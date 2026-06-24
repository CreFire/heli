package sync

import (
	"testing"

	"game/src/proto/pb"
)

func TestSnapshotToProtoMapsPlayersTowersAndMiners(t *testing.T) {
	snapshot := Snapshot{
		ServerTick: 7,
		Players: map[int64]PlayerSnapshot{
			1: {PlayerID: 1, Gold: 100, Mana: 20, Miners: []MinerSnapshot{{MinerID: 3, NextProduceTick: 9}}},
		},
		Towers: map[int64]TowerSnapshot{
			2: {TowerID: 2, OwnerPlayerID: 1, TypeID: 101, Level: 4, GridID: 1001},
		},
	}

	got := SnapshotToProto("room-1", snapshot)
	if got.GetRoomId() != "room-1" || got.GetServerTick() != 7 {
		t.Fatalf("unexpected snapshot header: %+v", got)
	}
	if len(got.GetPlayers()) != 1 || got.GetPlayers()[0].GetGold() != 100 || got.GetPlayers()[0].GetMiners()[0].GetMinerId() != 3 {
		t.Fatalf("unexpected players: %+v", got.GetPlayers())
	}
	if len(got.GetTowers()) != 1 || got.GetTowers()[0].GetTowerType() != 101 || got.GetTowers()[0].GetLevel() != 4 {
		t.Fatalf("unexpected towers: %+v", got.GetTowers())
	}
}

func TestDeltasToProtoMapsDeltaTypes(t *testing.T) {
	deltas := []Delta{
		{Type: DeltaResourceChanged, ServerTick: 1, PlayerID: 1, OpID: "op-r", Gold: 80, Mana: 20},
		{Type: DeltaTowerBuilt, ServerTick: 2, PlayerID: 1, OpID: "op-b", TowerID: 2, Tower: TowerSnapshot{TowerID: 2, OwnerPlayerID: 1, TypeID: 101, Level: 1, GridID: 1001}},
		{Type: DeltaTowerRerolled, ServerTick: 3, PlayerID: 1, OpID: "op-rr", TowerID: 2},
		{Type: DeltaTowerMerged, ServerTick: 4, PlayerID: 1, OpID: "op-m", TowerID: 2, MaterialTowerID: 3},
		{Type: DeltaMinerBought, ServerTick: 5, PlayerID: 1, OpID: "op-mb", MinerID: 4},
		{Type: DeltaMinerProduced, ServerTick: 6, PlayerID: 1, MinerID: 4, Mana: 25},
	}

	got := DeltasToProto("room-1", 6, deltas)
	wantTypes := []pb.BattleDeltaType{
		pb.BattleDeltaType_BATTLE_DELTA_RESOURCE_CHANGED,
		pb.BattleDeltaType_BATTLE_DELTA_TOWER_BUILT,
		pb.BattleDeltaType_BATTLE_DELTA_TOWER_REROLLED,
		pb.BattleDeltaType_BATTLE_DELTA_TOWER_MERGED,
		pb.BattleDeltaType_BATTLE_DELTA_MINER_BOUGHT,
		pb.BattleDeltaType_BATTLE_DELTA_MINER_PRODUCED,
	}
	if got.GetRoomId() != "room-1" || got.GetServerTick() != 6 {
		t.Fatalf("unexpected delta header: %+v", got)
	}
	if len(got.GetEvents()) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d", len(got.GetEvents()), len(wantTypes))
	}
	for i, want := range wantTypes {
		if got.GetEvents()[i].GetType() != want {
			t.Fatalf("event[%d] type = %v, want %v", i, got.GetEvents()[i].GetType(), want)
		}
	}
}
