package sync

import (
	"fmt"
	"sort"
)

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

	BaseHP          int64
	MonsterDamage   int64
	WaveTimeoutTick int64
	MonsterWaves    []MonsterWaveConfig
}

type RoomState string

const (
	RoomStateCreated RoomState = "CREATED"
	RoomStateRunning RoomState = "RUNNING"
	RoomStateClosed  RoomState = "CLOSED"
)

type Room struct {
	cfg           RoomConfig
	state         RoomState
	serverTick    int64
	startTick     int64
	endTick       int64
	nextTowerID   int64
	nextMinerID   int64
	nextWaveID    int32
	nextMonsterID int64
	randomIndex   int

	finishReason FinishReason
	settled      bool
	baseHP       int64

	players   map[int64]*PlayerState
	towers    map[int64]*TowerState
	gridTower map[int32]int64
	monsters  map[int64]*MonsterState
	waves     []MonsterWaveState
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

type MonsterWaveConfig struct {
	StartTick    int64
	RouteID      int32
	MonsterType  int32
	MonsterCount int32
	SpawnGapTick int64
	Speed        int64
	HP           int64
	RewardGold   int64
	PathLength   int64
}

type MonsterWaveState struct {
	WaveID       int32
	Config       MonsterWaveConfig
	SpawnedCount int32
	Finished     bool
}

type MonsterState struct {
	MonsterID    int64
	WaveID       int32
	MonsterType  int32
	RouteID      int32
	SpawnTick    int64
	Progress     int64
	Speed        int64
	HP           int64
	MaxHP        int64
	RewardGold   int64
	PathLength   int64
	Status       MonsterStatus
	FinishedTick int64
}

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
	ServerTick   int64
	State        RoomState
	StartTick    int64
	EndTick      int64
	BaseHP       int64
	FinishReason FinishReason
	Players      map[int64]PlayerSnapshot
	Towers       map[int64]TowerSnapshot
	Monsters     map[int64]MonsterSnapshot
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
	if cfg.BaseHP <= 0 {
		cfg.BaseHP = 3
	}
	if cfg.MonsterDamage <= 0 {
		cfg.MonsterDamage = 1
	}
	if cfg.WaveTimeoutTick <= 0 {
		cfg.WaveTimeoutTick = 600
	}
	if cfg.MonsterWaves == nil {
		cfg.MonsterWaves = defaultMonsterWaves()
	}
	waves := make([]MonsterWaveState, 0, len(cfg.MonsterWaves))
	for idx, wave := range cfg.MonsterWaves {
		if wave.MonsterCount <= 0 {
			continue
		}
		if wave.SpawnGapTick <= 0 {
			wave.SpawnGapTick = 1
		}
		if wave.Speed <= 0 {
			wave.Speed = 1
		}
		if wave.HP <= 0 {
			wave.HP = 1
		}
		if wave.PathLength <= 0 {
			wave.PathLength = 100
		}
		waves = append(waves, MonsterWaveState{WaveID: int32(idx + 1), Config: wave})
	}
	return &Room{
		cfg:           cfg,
		state:         RoomStateCreated,
		baseHP:        cfg.BaseHP,
		nextTowerID:   1,
		nextMinerID:   1,
		nextMonsterID: 1,
		players:       make(map[int64]*PlayerState),
		towers:        make(map[int64]*TowerState),
		gridTower:     make(map[int32]int64),
		monsters:      make(map[int64]*MonsterState),
		waves:         waves,
	}
}

func defaultMonsterWaves() []MonsterWaveConfig {
	return []MonsterWaveConfig{
		{StartTick: 1, RouteID: 1, MonsterType: 101, MonsterCount: 2, SpawnGapTick: 2, Speed: 40, HP: 10, RewardGold: 5, PathLength: 100},
		{StartTick: 6, RouteID: 1, MonsterType: 102, MonsterCount: 2, SpawnGapTick: 2, Speed: 50, HP: 12, RewardGold: 6, PathLength: 100},
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

func (r *Room) Start() bool {
	if r.state != RoomStateCreated {
		return false
	}
	r.state = RoomStateRunning
	r.startTick = r.serverTick
	return true
}

func (r *Room) Abort() bool {
	return r.finish(FinishAbort)
}

func (r *Room) State() RoomState           { return r.state }
func (r *Room) StartTick() int64           { return r.startTick }
func (r *Room) EndTick() int64             { return r.endTick }
func (r *Room) FinishReason() FinishReason { return r.finishReason }
func (r *Room) Settled() bool              { return r.settled }
func (r *Room) MarkSettled() bool {
	if r.settled {
		return false
	}
	r.settled = true
	return true
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
		if r.state == RoomStateClosed {
			return
		}
		if r.state == RoomStateCreated {
			r.Start()
		}
		r.serverTick++
		r.produceMinerMana()
		r.spawnWaveMonsters()
		r.advanceMonsters()
		r.cleanupFinishedWaves()
		r.checkFinishConditions()
	}
}

func (r *Room) Snapshot() Snapshot {
	s := Snapshot{
		ServerTick:   r.serverTick,
		State:        r.state,
		StartTick:    r.startTick,
		EndTick:      r.endTick,
		BaseHP:       r.baseHP,
		FinishReason: r.finishReason,
		Players:      make(map[int64]PlayerSnapshot, len(r.players)),
		Towers:       make(map[int64]TowerSnapshot, len(r.towers)),
		Monsters:     make(map[int64]MonsterSnapshot, len(r.monsters)),
	}
	for id, p := range r.players {
		ps := PlayerSnapshot{PlayerID: p.PlayerID, Gold: p.Gold, Mana: p.Mana, Miners: make([]MinerSnapshot, 0, len(p.Miners))}
		for _, m := range p.Miners {
			ps.Miners = append(ps.Miners, MinerSnapshot{MinerID: m.MinerID, NextProduceTick: m.NextProduceTick})
		}
		sort.Slice(ps.Miners, func(i, j int) bool { return ps.Miners[i].MinerID < ps.Miners[j].MinerID })
		s.Players[id] = ps
	}
	for id, t := range r.towers {
		s.Towers[id] = towerSnapshot(t)
	}
	for id, monster := range r.monsters {
		s.Monsters[id] = monsterSnapshot(monster)
	}
	return s
}

func (r *Room) FlushDeltas() []Delta {
	out := append([]Delta(nil), r.deltas...)
	r.deltas = nil
	return out
}

func (r *Room) BuildSettlement(roomID string) Settlement {
	players := make([]PlayerSettlement, 0, len(r.players))
	ids := make([]int64, 0, len(r.players))
	for id := range r.players {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		player := r.players[id]
		players = append(players, PlayerSettlement{
			PlayerID:  player.PlayerID,
			Gold:      player.Gold,
			Mana:      player.Mana,
			KillCount: 0,
		})
	}
	return Settlement{
		RoomID:       roomID,
		BattleID:     roomID,
		Win:          r.finishReason == FinishWin,
		StartTick:    r.startTick,
		EndTick:      r.endTick,
		FinishReason: r.finishReason,
		Players:      players,
	}
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

func (r *Room) spawnWaveMonsters() {
	for idx := range r.waves {
		wave := &r.waves[idx]
		if wave.Finished || r.serverTick < wave.Config.StartTick {
			continue
		}
		for wave.SpawnedCount < wave.Config.MonsterCount {
			nextSpawnTick := wave.Config.StartTick + int64(wave.SpawnedCount)*wave.Config.SpawnGapTick
			if r.serverTick < nextSpawnTick {
				break
			}
			monster := &MonsterState{
				MonsterID:   r.allocMonsterID(),
				WaveID:      wave.WaveID,
				MonsterType: wave.Config.MonsterType,
				RouteID:     wave.Config.RouteID,
				SpawnTick:   r.serverTick,
				Speed:       wave.Config.Speed,
				HP:          wave.Config.HP,
				MaxHP:       wave.Config.HP,
				RewardGold:  wave.Config.RewardGold,
				PathLength:  wave.Config.PathLength,
				Status:      MonsterAlive,
			}
			r.monsters[monster.MonsterID] = monster
			wave.SpawnedCount++
			r.appendMonsterDelta(DeltaMonsterSpawned, monster)
		}
	}
}

func (r *Room) advanceMonsters() {
	for _, monster := range r.monsters {
		if monster.Status != MonsterAlive {
			continue
		}
		monster.Progress += monster.Speed
		if monster.Progress >= monster.PathLength {
			monster.Progress = monster.PathLength
			monster.Status = MonsterArrived
			monster.FinishedTick = r.serverTick
			r.baseHP -= r.cfg.MonsterDamage
			if r.baseHP < 0 {
				r.baseHP = 0
			}
			r.appendMonsterDelta(DeltaMonsterArrived, monster)
			continue
		}
		r.appendMonsterDelta(DeltaMonsterProgressFixed, monster)
	}
}

func (r *Room) cleanupFinishedWaves() {
	for idx := range r.waves {
		wave := &r.waves[idx]
		if wave.Finished || wave.SpawnedCount < wave.Config.MonsterCount {
			continue
		}
		finished := true
		for _, monster := range r.monsters {
			if monster.WaveID == wave.WaveID && monster.Status == MonsterAlive {
				finished = false
				break
			}
		}
		wave.Finished = finished
	}
}

func (r *Room) checkFinishConditions() {
	if r.state == RoomStateClosed {
		return
	}
	if r.baseHP <= 0 {
		r.finish(FinishLose)
		return
	}
	if r.cfg.WaveTimeoutTick > 0 && r.serverTick-r.startTick >= r.cfg.WaveTimeoutTick {
		r.finish(FinishTimeout)
		return
	}
	allFinished := len(r.waves) > 0
	for _, wave := range r.waves {
		if !wave.Finished {
			allFinished = false
			break
		}
	}
	if allFinished {
		r.finish(FinishWin)
	}
}

func (r *Room) finish(reason FinishReason) bool {
	if r.state == RoomStateClosed {
		return false
	}
	r.state = RoomStateClosed
	r.finishReason = reason
	r.endTick = r.serverTick
	return true
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

func (r *Room) allocMonsterID() int64 {
	id := r.nextMonsterID
	r.nextMonsterID++
	return id
}

func (r *Room) appendResourceDelta(playerID int64, opID string) {
	p := r.players[playerID]
	r.deltas = append(r.deltas, Delta{Type: DeltaResourceChanged, ServerTick: r.serverTick, PlayerID: playerID, OpID: opID, Gold: p.Gold, Mana: p.Mana})
}

func (r *Room) appendTowerDelta(deltaType DeltaType, playerID int64, opID string, tower *TowerState, materialTowerID int64) {
	r.deltas = append(r.deltas, Delta{Type: deltaType, ServerTick: r.serverTick, PlayerID: playerID, OpID: opID, TowerID: tower.TowerID, MaterialTowerID: materialTowerID, Tower: towerSnapshot(tower)})
}

func (r *Room) appendMonsterDelta(deltaType DeltaType, monster *MonsterState) {
	r.deltas = append(r.deltas, Delta{Type: deltaType, ServerTick: r.serverTick, MonsterID: monster.MonsterID, Monster: monsterSnapshot(monster)})
}

func towerSnapshot(t *TowerState) TowerSnapshot {
	return TowerSnapshot{TowerID: t.TowerID, OwnerPlayerID: t.OwnerPlayerID, TypeID: t.TypeID, Level: t.Level, GridID: t.GridID}
}

func monsterSnapshot(m *MonsterState) MonsterSnapshot {
	return MonsterSnapshot{MonsterID: m.MonsterID, MonsterType: m.MonsterType, RouteID: m.RouteID, SpawnTick: m.SpawnTick, Progress: m.Progress, Speed: m.Speed, HP: m.HP, MaxHP: m.MaxHP, Status: m.Status}
}

func fail(opID string, code ErrorCode) OpResult {
	return OpResult{OK: false, Code: code, OpID: opID}
}
