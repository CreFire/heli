package battleapp

import (
	"fmt"
	"sync"
	"time"

	battlesync "game/src/service/battle/sync"
)

type battleRoom struct {
	mu                sync.Mutex
	id                string
	playerIDs         []int64
	room              *battlesync.Room // 局内权威状态，所有战斗操作最终都下沉到 sync.Room。
	allowedToken      string
	joinedSess        map[int64]int64 // battle join 成功后绑定 player -> session，后续 op 用它做最小会话校验。
	loop              *roomLoop       // 房间独立 tick loop；P0 采用 battle 服本地 ticker 自动推进。
	settleAcked       bool
	settleAckMessage  string
	lastSettlement    battlesync.Settlement // 最近一次发往 logic 的结算快照，便于联调排查。
	lastSettlementErr string
}

type roomManager struct {
	mu    sync.RWMutex
	rooms map[string]*battleRoom
}

type roomLoop struct {
	ticker     *time.Ticker
	stopCh     chan struct{}
	stoppedCh  chan struct{}
	interval   time.Duration
	settleOnce sync.Once
}

func newRoomManager() *roomManager {
	return &roomManager{
		rooms: make(map[string]*battleRoom),
	}
}

func (m *roomManager) createRoom(roomID string, playerIDs []int64, towerDeck []int32, battleToken string) (*battleRoom, error) {
	if roomID == "" {
		return nil, fmt.Errorf("room id is empty")
	}
	if len(playerIDs) == 0 {
		return nil, fmt.Errorf("player ids is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if room := m.rooms[roomID]; room != nil {
		return nil, fmt.Errorf("room already exists")
	}

	// 当前配置先写死，目标是尽快打通 battle P0 闭环。
	// 后续若做配置表化，再把这些值迁移到 battle 配置或表结构。
	cfg := battlesync.RoomConfig{
		InitialGold:          100,
		InitialMana:          0,
		BuildCostGold:        10,
		RerollCostMana:       1,
		MinerCostGold:        20,
		MinerProduceMana:     1,
		MinerProduceInterval: 3,
		TowerMaxLevel:        5,
		BaseHP:               3,
		MonsterDamage:        1,
		WaveTimeoutTick:      600,
		DefaultTowerDeck:     append([]int32(nil), towerDeck...),
	}
	if len(cfg.DefaultTowerDeck) == 0 {
		cfg.DefaultTowerDeck = []int32{101}
	}

	room := battlesync.NewRoom(cfg)
	for _, playerID := range playerIDs {
		if err := room.AddPlayer(playerID, towerDeck); err != nil {
			return nil, err
		}
	}

	ret := &battleRoom{
		id:           roomID,
		playerIDs:    append([]int64(nil), playerIDs...),
		room:         room,
		allowedToken: battleToken,
		joinedSess:   make(map[int64]int64, len(playerIDs)),
	}
	m.rooms[roomID] = ret
	return ret, nil
}

func (m *roomManager) getRoom(roomID string) *battleRoom {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rooms[roomID]
}

func (m *roomManager) roomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

func (r *battleRoom) hasPlayer(playerID int64) bool {
	for _, id := range r.playerIDs {
		if id == playerID {
			return true
		}
	}
	return false
}

func (r *battleRoom) bindPlayerSession(playerID, sessID int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.joinedSess == nil {
		r.joinedSess = map[int64]int64{}
	}
	// 允许重复 join 覆盖 session，便于最小重连/重进房场景下继续联调。
	r.joinedSess[playerID] = sessID
}

func (r *battleRoom) matchPlayerSession(playerID, sessID int64) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bound, ok := r.joinedSess[playerID]
	return ok && bound == sessID
}

func (r *battleRoom) snapshotJoinedSessions() map[int64]int64 {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ret := make(map[int64]int64, len(r.joinedSess))
	for playerID, sessID := range r.joinedSess {
		ret[playerID] = sessID
	}
	return ret
}

func (r *battleRoom) withRoom(fn func(room *battlesync.Room)) {
	if r == nil || r.room == nil || fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(r.room)
}

func (r *battleRoom) roomSnapshot() battlesync.Snapshot {
	var snapshot battlesync.Snapshot
	r.withRoom(func(room *battlesync.Room) {
		snapshot = room.Snapshot()
	})
	return snapshot
}

func (r *battleRoom) flushRoomDeltas() []battlesync.Delta {
	var deltas []battlesync.Delta
	r.withRoom(func(room *battlesync.Room) {
		deltas = room.FlushDeltas()
	})
	return deltas
}

func (r *battleRoom) advanceLoopTick() battlesync.Snapshot {
	var snapshot battlesync.Snapshot
	r.withRoom(func(room *battlesync.Room) {
		// battle loop 逐 tick 推进，由 sync.Room 负责怪物、矿工、结束条件等内部演进。
		room.AdvanceToTick(room.Snapshot().ServerTick + 1)
		snapshot = room.Snapshot()
	})
	return snapshot
}

func (r *battleRoom) state() battlesync.RoomState {
	if r == nil || r.room == nil {
		return battlesync.RoomStateClosed
	}
	return r.roomSnapshot().State
}

func (r *battleRoom) markSettled() bool {
	if r == nil || r.room == nil {
		return false
	}
	var ok bool
	r.withRoom(func(room *battlesync.Room) {
		ok = room.MarkSettled()
	})
	return ok
}

func (r *battleRoom) buildSettlement() battlesync.Settlement {
	if r == nil || r.room == nil {
		return battlesync.Settlement{}
	}
	settlement := battlesync.Settlement{}
	r.withRoom(func(room *battlesync.Room) {
		// 结算内容统一从权威房间快照构造，避免 handler 层自行拼字段导致不一致。
		settlement = room.BuildSettlement(r.id)
	})
	r.mu.Lock()
	r.lastSettlement = settlement
	r.mu.Unlock()
	return settlement
}

func (r *battleRoom) markSettleAck(acked bool, message string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settleAcked = acked
	r.settleAckMessage = message
	if acked {
		r.lastSettlementErr = ""
	} else {
		r.lastSettlementErr = message
	}
}
