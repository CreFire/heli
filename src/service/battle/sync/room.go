package sync

import "fmt"

type ErrorCode string

const (
	OK ErrorCode = "OK"

	ErrPlayerExists       ErrorCode = "PLAYER_EXISTS"
	ErrPlayerNotFound     ErrorCode = "PLAYER_NOT_FOUND"
	ErrNoTowerInDeck      ErrorCode = "NO_TOWER_IN_DECK"
	ErrNotEnoughGold      ErrorCode = "NOT_ENOUGH_GOLD"
	ErrNotEnoughMana      ErrorCode = "NOT_ENOUGH_MANA"
	ErrGridNotInBuildArea ErrorCode = "GRID_NOT_IN_BUILD_AREA"
	ErrGridOccupied       ErrorCode = "GRID_OCCUPIED"
	ErrTowerNotFound      ErrorCode = "TOWER_NOT_FOUND"
	ErrTowerNotOwned      ErrorCode = "TOWER_NOT_OWNED"
	ErrTowerMergeMismatch ErrorCode = "TOWER_MERGE_MISMATCH"
	ErrTowerMaxLevel      ErrorCode = "TOWER_MAX_LEVEL"
)

type RoomConfig struct {
	InitialGold int64
	InitialMana int64

	BuildCostGold        int64
	RerollCostMana       int64
	MinerCostGold        int64
	MinerProduceMana     int64
	MinerProduceInterval int64

	TowerMaxLevel      int32
	DefaultTowerDeck   []int32
	RandomTowerTypes   []int32
	BuildGridsByPlayer map[int64][]int32
}

type Room struct {
	cfg         RoomConfig
	serverTick  int64
	nextTowerID int64
	nextMinerID int64
	randomIndex int

	players   map[int64]*PlayerState
	towers    map[int64]*TowerState
	gridTower map[int32]int64
	deltas    []Delta
}

type PlayerState struct {
	PlayerID  int64
	Gold      int64
	Mana      int64
	TowerDeck []int32
	Miners    map[int64]*MinerState
}

type TowerState struct {
	TowerID       int64
	OwnerPlayerID int64
	TypeID        int32
	Level         int32
	GridID        int32
}

type MinerState struct {
	MinerID         int64
	OwnerPlayerID   int64
	NextProduceTick int64
}

type MonsterStatus string

const (
	MonsterNone    MonsterStatus = "NONE"
	MonsterAlive   MonsterStatus = "ALIVE"
	MonsterDead    MonsterStatus = "DEAD"
	MonsterArrived MonsterStatus = "ARRIVED"
	MonsterPaused  MonsterStatus = "PAUSED"
)

type MonsterSnapshot struct {
	MonsterID   int64
	MonsterType int32
	RouteID     int32
	SpawnTick   int64
	Progress    int64
	Speed       int64
	HP          int64
	MaxHP       int64
	Status      MonsterStatus
}

type Snapshot struct {
	ServerTick int64
	Players    map[int64]PlayerSnapshot
	Towers     map[int64]TowerSnapshot
	Monsters   map[int64]MonsterSnapshot
}

type PlayerSnapshot struct {
	PlayerID int64
	Gold     int64
	Mana     int64
	Miners   []MinerSnapshot
}

type TowerSnapshot struct {
	TowerID       int64
	OwnerPlayerID int64
	TypeID        int32
	Level         int32
	GridID        int32
}

type MinerSnapshot struct {
	MinerID         int64
	NextProduceTick int64
}

type DeltaType string

const (
	DeltaResourceChanged      DeltaType = "RESOURCE_CHANGED"
	DeltaTowerBuilt           DeltaType = "TOWER_BUILT"
	DeltaTowerRerolled        DeltaType = "TOWER_REROLLED"
	DeltaTowerMerged          DeltaType = "TOWER_MERGED"
	DeltaMinerBought          DeltaType = "MINER_BOUGHT"
	DeltaMinerProduced        DeltaType = "MINER_PRODUCED"
	DeltaMonsterSpawned       DeltaType = "MONSTER_SPAWNED"
	DeltaMonsterHPChanged     DeltaType = "MONSTER_HP_CHANGED"
	DeltaMonsterStatusChanged DeltaType = "MONSTER_STATUS_CHANGED"
	DeltaMonsterDead          DeltaType = "MONSTER_DEAD"
	DeltaMonsterArrived       DeltaType = "MONSTER_ARRIVED"
	DeltaMonsterProgressFixed DeltaType = "MONSTER_PROGRESS_FIXED"
)

type Delta struct {
	Type            DeltaType
	ServerTick      int64
	PlayerID        int64
	OpID            string
	TowerID         int64
	MaterialTowerID int64
	MinerID         int64
	MonsterID       int64
	Gold            int64
	Mana            int64
	Tower           TowerSnapshot
	Monster         MonsterSnapshot
}

type OpResult struct {
	OK      bool
	Code    ErrorCode
	OpID    string
	TowerID int64
	MinerID int64
}

func NewRoom(cfg RoomConfig) *Room {
	if cfg.TowerMaxLevel <= 0 {
		cfg.TowerMaxLevel = 5
	}
	return &Room{
		cfg:         cfg,
		nextTowerID: 1,
		nextMinerID: 1,
		players:     make(map[int64]*PlayerState),
		towers:      make(map[int64]*TowerState),
		gridTower:   make(map[int32]int64),
	}
}

func (r *Room) AddPlayer(playerID int64, deck []int32) error {
	if _, ok := r.players[playerID]; ok {
		return fmt.Errorf("%s", ErrPlayerExists)
	}
	if len(deck) == 0 {
		deck = append([]int32(nil), r.cfg.DefaultTowerDeck...)
	}
	if len(deck) == 0 {
		return fmt.Errorf("%s", ErrNoTowerInDeck)
	}
	r.players[playerID] = &PlayerState{
		PlayerID:  playerID,
		Gold:      r.cfg.InitialGold,
		Mana:      r.cfg.InitialMana,
		TowerDeck: append([]int32(nil), deck...),
		Miners:    make(map[int64]*MinerState),
	}
	return nil
}

func (r *Room) BuildTower(playerID int64, opID string, gridID int32) OpResult {
	p, ok := r.players[playerID]
	if !ok {
		return fail(opID, ErrPlayerNotFound)
	}
	if !r.canBuildOn(playerID, gridID) {
		return fail(opID, ErrGridNotInBuildArea)
	}
	if _, occupied := r.gridTower[gridID]; occupied {
		return fail(opID, ErrGridOccupied)
	}
	if p.Gold < r.cfg.BuildCostGold {
		return fail(opID, ErrNotEnoughGold)
	}
	typeID, ok := r.nextTowerType(p)
	if !ok {
		return fail(opID, ErrNoTowerInDeck)
	}
	p.Gold -= r.cfg.BuildCostGold
	towerID := r.allocTowerID()
	tower := &TowerState{TowerID: towerID, OwnerPlayerID: playerID, TypeID: typeID, Level: 1, GridID: gridID}
	r.towers[towerID] = tower
	r.gridTower[gridID] = towerID
	r.appendResourceDelta(playerID, opID)
	r.appendTowerDelta(DeltaTowerBuilt, playerID, opID, tower, 0)
	return OpResult{OK: true, Code: OK, OpID: opID, TowerID: towerID}
}

func (r *Room) RerollTower(playerID int64, opID string, towerID int64) OpResult {
	p, ok := r.players[playerID]
	if !ok {
		return fail(opID, ErrPlayerNotFound)
	}
	tower, ok := r.towers[towerID]
	if !ok {
		return fail(opID, ErrTowerNotFound)
	}
	if tower.OwnerPlayerID != playerID {
		return fail(opID, ErrTowerNotOwned)
	}
	if p.Mana < r.cfg.RerollCostMana {
		return fail(opID, ErrNotEnoughMana)
	}
	typeID, ok := r.nextTowerType(p)
	if !ok {
		return fail(opID, ErrNoTowerInDeck)
	}
	p.Mana -= r.cfg.RerollCostMana
	tower.TypeID = typeID
	r.appendResourceDelta(playerID, opID)
	r.appendTowerDelta(DeltaTowerRerolled, playerID, opID, tower, 0)
	return OpResult{OK: true, Code: OK, OpID: opID, TowerID: towerID}
}

func (r *Room) MergeTower(playerID int64, opID string, mainTowerID, materialTowerID int64) OpResult {
	p, ok := r.players[playerID]
	if !ok {
		return fail(opID, ErrPlayerNotFound)
	}
	mainTower, ok := r.towers[mainTowerID]
	if !ok {
		return fail(opID, ErrTowerNotFound)
	}
	materialTower, ok := r.towers[materialTowerID]
	if !ok {
		return fail(opID, ErrTowerNotFound)
	}
	if mainTower.OwnerPlayerID != playerID || materialTower.OwnerPlayerID != playerID {
		return fail(opID, ErrTowerNotOwned)
	}
	if mainTower.TypeID != materialTower.TypeID || mainTower.Level != materialTower.Level || mainTowerID == materialTowerID {
		return fail(opID, ErrTowerMergeMismatch)
	}
	if mainTower.Level >= r.cfg.TowerMaxLevel {
		return fail(opID, ErrTowerMaxLevel)
	}
	typeID, ok := r.nextTowerType(p)
	if !ok {
		return fail(opID, ErrNoTowerInDeck)
	}
	delete(r.towers, materialTowerID)
	delete(r.gridTower, materialTower.GridID)
	mainTower.TypeID = typeID
	mainTower.Level++
	r.appendTowerDelta(DeltaTowerMerged, playerID, opID, mainTower, materialTowerID)
	return OpResult{OK: true, Code: OK, OpID: opID, TowerID: mainTowerID}
}

func (r *Room) BuyMiner(playerID int64, opID string) OpResult {
	p, ok := r.players[playerID]
	if !ok {
		return fail(opID, ErrPlayerNotFound)
	}
	if p.Gold < r.cfg.MinerCostGold {
		return fail(opID, ErrNotEnoughGold)
	}
	p.Gold -= r.cfg.MinerCostGold
	minerID := r.allocMinerID()
	interval := r.cfg.MinerProduceInterval
	if interval <= 0 {
		interval = 1
	}
	miner := &MinerState{MinerID: minerID, OwnerPlayerID: playerID, NextProduceTick: r.serverTick + interval}
	p.Miners[minerID] = miner
	r.appendResourceDelta(playerID, opID)
	r.deltas = append(r.deltas, Delta{Type: DeltaMinerBought, ServerTick: r.serverTick, PlayerID: playerID, OpID: opID, MinerID: minerID, Gold: p.Gold, Mana: p.Mana})
	return OpResult{OK: true, Code: OK, OpID: opID, MinerID: minerID}
}

func (r *Room) AdvanceToTick(targetTick int64) {
	for r.serverTick < targetTick {
		r.serverTick++
		r.produceMinerMana()
	}
}

func (r *Room) Snapshot() Snapshot {
	s := Snapshot{ServerTick: r.serverTick, Players: make(map[int64]PlayerSnapshot, len(r.players)), Towers: make(map[int64]TowerSnapshot, len(r.towers)), Monsters: make(map[int64]MonsterSnapshot)}
	for id, p := range r.players {
		ps := PlayerSnapshot{PlayerID: p.PlayerID, Gold: p.Gold, Mana: p.Mana, Miners: make([]MinerSnapshot, 0, len(p.Miners))}
		for _, m := range p.Miners {
			ps.Miners = append(ps.Miners, MinerSnapshot{MinerID: m.MinerID, NextProduceTick: m.NextProduceTick})
		}
		s.Players[id] = ps
	}
	for id, t := range r.towers {
		s.Towers[id] = towerSnapshot(t)
	}
	return s
}

func (r *Room) FlushDeltas() []Delta {
	out := append([]Delta(nil), r.deltas...)
	r.deltas = nil
	return out
}

func (r *Room) produceMinerMana() {
	interval := r.cfg.MinerProduceInterval
	if interval <= 0 {
		interval = 1
	}
	for _, p := range r.players {
		for _, m := range p.Miners {
			for r.serverTick >= m.NextProduceTick {
				p.Mana += r.cfg.MinerProduceMana
				m.NextProduceTick += interval
				r.deltas = append(r.deltas, Delta{Type: DeltaMinerProduced, ServerTick: r.serverTick, PlayerID: p.PlayerID, MinerID: m.MinerID, Gold: p.Gold, Mana: p.Mana})
			}
		}
	}
}

func (r *Room) canBuildOn(playerID int64, gridID int32) bool {
	grids := r.cfg.BuildGridsByPlayer[playerID]
	if len(grids) == 0 && len(r.cfg.BuildGridsByPlayer) == 0 {
		return true
	}
	for _, allowed := range grids {
		if allowed == gridID {
			return true
		}
	}
	return false
}

func (r *Room) nextTowerType(p *PlayerState) (int32, bool) {
	pool := r.cfg.RandomTowerTypes
	if len(pool) == 0 {
		pool = p.TowerDeck
	}
	if len(pool) == 0 {
		pool = r.cfg.DefaultTowerDeck
	}
	if len(pool) == 0 {
		return 0, false
	}
	typeID := pool[r.randomIndex%len(pool)]
	r.randomIndex++
	return typeID, true
}

func (r *Room) allocTowerID() int64 {
	id := r.nextTowerID
	r.nextTowerID++
	return id
}

func (r *Room) allocMinerID() int64 {
	id := r.nextMinerID
	r.nextMinerID++
	return id
}

func (r *Room) appendResourceDelta(playerID int64, opID string) {
	p := r.players[playerID]
	r.deltas = append(r.deltas, Delta{Type: DeltaResourceChanged, ServerTick: r.serverTick, PlayerID: playerID, OpID: opID, Gold: p.Gold, Mana: p.Mana})
}

func (r *Room) appendTowerDelta(deltaType DeltaType, playerID int64, opID string, tower *TowerState, materialTowerID int64) {
	r.deltas = append(r.deltas, Delta{Type: deltaType, ServerTick: r.serverTick, PlayerID: playerID, OpID: opID, TowerID: tower.TowerID, MaterialTowerID: materialTowerID, Tower: towerSnapshot(tower)})
}

func towerSnapshot(t *TowerState) TowerSnapshot {
	return TowerSnapshot{TowerID: t.TowerID, OwnerPlayerID: t.OwnerPlayerID, TypeID: t.TypeID, Level: t.Level, GridID: t.GridID}
}

func fail(opID string, code ErrorCode) OpResult {
	return OpResult{OK: false, Code: code, OpID: opID}
}
