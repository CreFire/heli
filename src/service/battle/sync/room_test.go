package sync

import "testing"

func TestBuildTowerCreatesLevelOneTowerAndEmitsSnapshotDelta(t *testing.T) {
	room := NewRoom(RoomConfig{
		InitialGold:      100,
		InitialMana:      10,
		BuildCostGold:    20,
		RerollCostMana:   3,
		TowerMaxLevel:    5,
		DefaultTowerDeck: []int32{101},
		MonsterWaves:     []MonsterWaveConfig{},
		BuildGridsByPlayer: map[int64][]int32{
			1: {1001},
		},
	})

	if err := room.AddPlayer(1, []int32{101}); err != nil {
		t.Fatalf("add player: %v", err)
	}

	result := room.BuildTower(1, "op-build-1", 1001)
	if !result.OK {
		t.Fatalf("build tower failed: %v", result.Code)
	}

	snapshot := room.Snapshot()
	player := snapshot.Players[1]
	if player.Gold != 80 {
		t.Fatalf("gold after build = %d, want 80", player.Gold)
	}
	if len(snapshot.Towers) != 1 {
		t.Fatalf("tower count = %d, want 1", len(snapshot.Towers))
	}
	tower := snapshot.Towers[result.TowerID]
	if tower.OwnerPlayerID != 1 || tower.GridID != 1001 || tower.TypeID != 101 || tower.Level != 1 {
		t.Fatalf("unexpected tower: %+v", tower)
	}

	deltas := room.FlushDeltas()
	if len(deltas) != 2 {
		t.Fatalf("delta count = %d, want 2", len(deltas))
	}
	if deltas[0].Type != DeltaResourceChanged || deltas[1].Type != DeltaTowerBuilt {
		t.Fatalf("unexpected deltas: %+v", deltas)
	}
}

func TestBuildTowerRejectsGridOutsideOwnBuildArea(t *testing.T) {
	room := NewRoom(RoomConfig{
		InitialGold:      100,
		BuildCostGold:    20,
		TowerMaxLevel:    5,
		DefaultTowerDeck: []int32{101},
		MonsterWaves:     []MonsterWaveConfig{},
		BuildGridsByPlayer: map[int64][]int32{
			1: {1001},
		},
	})
	_ = room.AddPlayer(1, []int32{101})

	result := room.BuildTower(1, "op-build-outside", 2001)
	if result.OK {
		t.Fatalf("build outside area should fail")
	}
	if result.Code != ErrGridNotInBuildArea {
		t.Fatalf("code = %v, want %v", result.Code, ErrGridNotInBuildArea)
	}
	if len(room.Snapshot().Towers) != 0 {
		t.Fatalf("tower should not be created")
	}
}

func TestRerollOwnTowerKeepsLevelAndCostsMana(t *testing.T) {
	room := NewRoom(RoomConfig{
		InitialGold:        100,
		InitialMana:        10,
		BuildCostGold:      20,
		RerollCostMana:     4,
		TowerMaxLevel:      5,
		DefaultTowerDeck:   []int32{101, 202},
		MonsterWaves:       []MonsterWaveConfig{},
		BuildGridsByPlayer: map[int64][]int32{1: {1001}},
		RandomTowerTypes:   []int32{101, 202},
	})
	_ = room.AddPlayer(1, []int32{101, 202})
	built := room.BuildTower(1, "op-build", 1001)
	room.FlushDeltas()

	result := room.RerollTower(1, "op-reroll", built.TowerID)
	if !result.OK {
		t.Fatalf("reroll failed: %v", result.Code)
	}
	snapshot := room.Snapshot()
	if got := snapshot.Players[1].Mana; got != 6 {
		t.Fatalf("mana after reroll = %d, want 6", got)
	}
	tower := snapshot.Towers[built.TowerID]
	if tower.Level != 1 {
		t.Fatalf("tower level after reroll = %d, want 1", tower.Level)
	}
	if tower.TypeID != 202 {
		t.Fatalf("tower type after reroll = %d, want 202", tower.TypeID)
	}
}

func TestRerollRejectsOtherPlayersTower(t *testing.T) {
	room := NewRoom(RoomConfig{
		InitialGold:        100,
		InitialMana:        10,
		BuildCostGold:      20,
		RerollCostMana:     4,
		TowerMaxLevel:      5,
		DefaultTowerDeck:   []int32{101},
		MonsterWaves:       []MonsterWaveConfig{},
		BuildGridsByPlayer: map[int64][]int32{1: {1001}, 2: {2001}},
	})
	_ = room.AddPlayer(1, []int32{101})
	_ = room.AddPlayer(2, []int32{101})
	built := room.BuildTower(1, "op-build", 1001)

	result := room.RerollTower(2, "op-reroll-other", built.TowerID)
	if result.OK {
		t.Fatalf("reroll other player's tower should fail")
	}
	if result.Code != ErrTowerNotOwned {
		t.Fatalf("code = %v, want %v", result.Code, ErrTowerNotOwned)
	}
}

func TestMergeConsumesMaterialAndCreatesRandomHigherLevelAtMainGrid(t *testing.T) {
	room := NewRoom(RoomConfig{
		InitialGold:        100,
		BuildCostGold:      10,
		TowerMaxLevel:      5,
		DefaultTowerDeck:   []int32{101, 202},
		MonsterWaves:       []MonsterWaveConfig{},
		BuildGridsByPlayer: map[int64][]int32{1: {1001, 1002}},
		RandomTowerTypes:   []int32{101, 101, 202},
	})
	_ = room.AddPlayer(1, []int32{101, 202})
	main := room.BuildTower(1, "op-build-main", 1001)
	mat := room.BuildTower(1, "op-build-mat", 1002)
	room.FlushDeltas()

	result := room.MergeTower(1, "op-merge", main.TowerID, mat.TowerID)
	if !result.OK {
		t.Fatalf("merge failed: %v", result.Code)
	}
	snapshot := room.Snapshot()
	if len(snapshot.Towers) != 1 {
		t.Fatalf("tower count after merge = %d, want 1", len(snapshot.Towers))
	}
	if _, ok := snapshot.Towers[mat.TowerID]; ok {
		t.Fatalf("material tower should be removed")
	}
	tower := snapshot.Towers[main.TowerID]
	if tower.GridID != 1001 || tower.Level != 2 || tower.TypeID != 202 {
		t.Fatalf("unexpected merged tower: %+v", tower)
	}
}

func TestMinerProducesManaOnTick(t *testing.T) {
	room := NewRoom(RoomConfig{
		InitialGold:          100,
		InitialMana:          1,
		MinerCostGold:        30,
		MinerProduceMana:     5,
		MinerProduceInterval: 3,
		TowerMaxLevel:        5,
		DefaultTowerDeck:     []int32{101},
		MonsterWaves:         []MonsterWaveConfig{},
	})
	_ = room.AddPlayer(1, []int32{101})

	buy := room.BuyMiner(1, "op-miner")
	if !buy.OK {
		t.Fatalf("buy miner failed: %v", buy.Code)
	}
	room.FlushDeltas()

	room.AdvanceToTick(2)
	if got := room.Snapshot().Players[1].Mana; got != 1 {
		t.Fatalf("mana before interval = %d, want 1", got)
	}
	room.AdvanceToTick(3)
	if got := room.Snapshot().Players[1].Mana; got != 6 {
		t.Fatalf("mana after interval = %d, want 6", got)
	}
	deltas := room.FlushDeltas()
	if len(deltas) != 1 || deltas[0].Type != DeltaMinerProduced {
		t.Fatalf("unexpected miner deltas: %+v", deltas)
	}
}

func TestAdvanceToTickSpawnsAndMovesMonsters(t *testing.T) {
	room := NewRoom(RoomConfig{
		DefaultTowerDeck: []int32{101},
		MonsterWaves: []MonsterWaveConfig{{
			StartTick:    1,
			RouteID:      1,
			MonsterType:  1001,
			MonsterCount: 2,
			SpawnGapTick: 3,
			Speed:        25,
			HP:           10,
			PathLength:   100,
		}},
	})
	_ = room.AddPlayer(1, []int32{101})
	room.Start()
	room.AdvanceToTick(2)

	snapshot := room.Snapshot()
	if snapshot.ServerTick != 2 {
		t.Fatalf("server tick = %d, want 2", snapshot.ServerTick)
	}
	if snapshot.State != RoomStateRunning {
		t.Fatalf("state = %s, want RUNNING", snapshot.State)
	}
	if len(snapshot.Monsters) != 1 {
		t.Fatalf("monster count = %d, want 1", len(snapshot.Monsters))
	}
	for _, monster := range snapshot.Monsters {
		if monster.Status != MonsterAlive {
			t.Fatalf("monster status = %s, want ALIVE", monster.Status)
		}
		if monster.Progress <= 0 {
			t.Fatalf("monster progress = %d, want > 0", monster.Progress)
		}
	}
}

func TestRoomFinishWinAfterAllWavesProcessed(t *testing.T) {
	room := NewRoom(RoomConfig{
		DefaultTowerDeck: []int32{101},
		MonsterWaves: []MonsterWaveConfig{{
			StartTick:    1,
			RouteID:      1,
			MonsterType:  1001,
			MonsterCount: 1,
			SpawnGapTick: 1,
			Speed:        100,
			HP:           10,
			PathLength:   100,
		}},
		BaseHP:        5,
		MonsterDamage: 0,
	})
	_ = room.AddPlayer(1, []int32{101})
	room.Start()
	room.AdvanceToTick(2)

	snapshot := room.Snapshot()
	if snapshot.State != RoomStateClosed {
		t.Fatalf("room state = %s, want CLOSED", snapshot.State)
	}
	if snapshot.FinishReason != FinishWin {
		t.Fatalf("finish reason = %s, want WIN", snapshot.FinishReason)
	}
	if snapshot.EndTick == 0 {
		t.Fatalf("end tick should be set")
	}
}

func TestRoomFinishLoseWhenBaseHPDepleted(t *testing.T) {
	room := NewRoom(RoomConfig{
		DefaultTowerDeck: []int32{101},
		MonsterWaves: []MonsterWaveConfig{{
			StartTick:    1,
			RouteID:      1,
			MonsterType:  1001,
			MonsterCount: 1,
			SpawnGapTick: 1,
			Speed:        100,
			HP:           10,
			PathLength:   100,
		}},
		BaseHP:        1,
		MonsterDamage: 1,
	})
	_ = room.AddPlayer(1, []int32{101})
	room.Start()
	room.AdvanceToTick(1)

	snapshot := room.Snapshot()
	if snapshot.State != RoomStateClosed {
		t.Fatalf("room state = %s, want CLOSED", snapshot.State)
	}
	if snapshot.FinishReason != FinishLose {
		t.Fatalf("finish reason = %s, want LOSE", snapshot.FinishReason)
	}
	if snapshot.BaseHP != 0 {
		t.Fatalf("base hp = %d, want 0", snapshot.BaseHP)
	}
}

func TestMarkSettledOnlyOnce(t *testing.T) {
	room := NewRoom(RoomConfig{DefaultTowerDeck: []int32{101}, MonsterWaves: []MonsterWaveConfig{}})
	if room.MarkSettled() != true {
		t.Fatalf("first mark settled should succeed")
	}
	if room.MarkSettled() != false {
		t.Fatalf("second mark settled should fail")
	}
}

