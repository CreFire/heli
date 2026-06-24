package main_test

import (
	"testing"

	"game/src/proto/pb"
)

func TestBattleClientProtocolTypesCompile(t *testing.T) {
	join := &pb.C2SBattleJoinREQ{RoomId: "room-1", BattleToken: "token"}
	if join.GetRoomId() != "room-1" || join.GetBattleToken() != "token" {
		t.Fatalf("unexpected join req: %+v", join)
	}

	op := &pb.C2SBattleOpREQ{
		RoomId: "room-1",
		OpId:   "op-1",
		Op: &pb.BattlePlayerOp{
			Type:       pb.BattleOpType_BATTLE_OP_BUILD_TOWER,
			BuildTower: &pb.BattleBuildTowerOp{GridId: 1001},
		},
	}
	if op.GetOp().GetBuildTower().GetGridId() != 1001 {
		t.Fatalf("unexpected build op: %+v", op)
	}

	merge := &pb.BattlePlayerOp{
		Type:       pb.BattleOpType_BATTLE_OP_MERGE_TOWER,
		MergeTower: &pb.BattleMergeTowerOp{MainTowerId: 1, MaterialTowerId: 2},
	}
	if merge.GetMergeTower().GetMainTowerId() != 1 || merge.GetMergeTower().GetMaterialTowerId() != 2 {
		t.Fatalf("unexpected merge op: %+v", merge)
	}

	snapshot := &pb.S2CBattleSnapshotNTF{
		RoomId:     "room-1",
		ServerTick: 10,
		Players:    []*pb.BattlePlayerState{{PlayerId: 1, Gold: 100, Mana: 20}},
		Towers:     []*pb.BattleTowerState{{TowerId: 1, OwnerPlayerId: 1, TowerType: 101, Level: 1, GridId: 1001}},
	}
	if snapshot.GetPlayers()[0].GetGold() != 100 || snapshot.GetTowers()[0].GetTowerType() != 101 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	delta := &pb.S2CBattleDeltaNTF{
		RoomId:     "room-1",
		ServerTick: 11,
		Events:     []*pb.BattleStateDelta{{Type: pb.BattleDeltaType_BATTLE_DELTA_TOWER_BUILT, PlayerId: 1, Tower: snapshot.GetTowers()[0]}},
	}
	if delta.GetEvents()[0].GetType() != pb.BattleDeltaType_BATTLE_DELTA_TOWER_BUILT {
		t.Fatalf("unexpected delta: %+v", delta)
	}
}

func TestBattleClientMessageIDsCompile(t *testing.T) {
	ids := []pb.MSG_ID{
		pb.MSG_ID_BATTLE_JOIN_REQ,
		pb.MSG_ID_BATTLE_JOIN_RSP,
		pb.MSG_ID_BATTLE_OP_REQ,
		pb.MSG_ID_BATTLE_OP_RSP,
		pb.MSG_ID_BATTLE_SNAPSHOT_NTF,
		pb.MSG_ID_BATTLE_DELTA_NTF,
	}
	for _, id := range ids {
		if id == pb.MSG_ID_NONE {
			t.Fatalf("battle msg id should not be NONE")
		}
	}
}
