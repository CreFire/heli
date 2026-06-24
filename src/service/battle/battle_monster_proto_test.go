package main_test

import (
	"testing"

	"game/src/proto/pb"
)

func TestBattleMonsterProtocolTypesCompile(t *testing.T) {
	monster := &pb.BattleMonsterState{
		MonsterId:   1001,
		MonsterType: 11,
		RouteId:     1,
		SpawnTick:   20,
		Progress:    1500,
		Speed:       300,
		Hp:          80,
		MaxHp:       100,
		Status:      pb.BattleMonsterStatus_BATTLE_MONSTER_ALIVE,
	}
	if monster.GetMonsterId() != 1001 || monster.GetRouteId() != 1 || monster.GetStatus() != pb.BattleMonsterStatus_BATTLE_MONSTER_ALIVE {
		t.Fatalf("unexpected monster: %+v", monster)
	}

	snapshot := &pb.S2CBattleSnapshotNTF{
		RoomId:   "room-1",
		Monsters: []*pb.BattleMonsterState{monster},
	}
	if len(snapshot.GetMonsters()) != 1 || snapshot.GetMonsters()[0].GetProgress() != 1500 {
		t.Fatalf("unexpected snapshot monsters: %+v", snapshot)
	}

	delta := &pb.S2CBattleDeltaNTF{
		RoomId: "room-1",
		Events: []*pb.BattleStateDelta{{
			Type:    pb.BattleDeltaType_BATTLE_DELTA_MONSTER_SPAWNED,
			Monster: monster,
		}},
	}
	if delta.GetEvents()[0].GetType() != pb.BattleDeltaType_BATTLE_DELTA_MONSTER_SPAWNED {
		t.Fatalf("unexpected monster delta: %+v", delta)
	}
}
