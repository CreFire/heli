package sync

import (
	"testing"

	"game/src/proto/pb"
)

func TestSnapshotToProtoMapsMonsters(t *testing.T) {
	snapshot := Snapshot{
		ServerTick: 7,
		Players:    map[int64]PlayerSnapshot{},
		Towers:     map[int64]TowerSnapshot{},
		Monsters: map[int64]MonsterSnapshot{
			1001: {
				MonsterID:   1001,
				MonsterType: 11,
				RouteID:     1,
				SpawnTick:   2,
				Progress:    1500,
				Speed:       300,
				HP:          80,
				MaxHP:       100,
				Status:      MonsterAlive,
			},
		},
	}

	got := SnapshotToProto("room-1", snapshot)
	if len(got.GetMonsters()) != 1 {
		t.Fatalf("monster count = %d, want 1", len(got.GetMonsters()))
	}
	monster := got.GetMonsters()[0]
	if monster.GetMonsterId() != 1001 || monster.GetMonsterType() != 11 || monster.GetProgress() != 1500 || monster.GetStatus() != pb.BattleMonsterStatus_BATTLE_MONSTER_ALIVE {
		t.Fatalf("unexpected monster: %+v", monster)
	}
}

func TestDeltasToProtoMapsMonsterDelta(t *testing.T) {
	deltas := []Delta{{
		Type:       DeltaMonsterSpawned,
		ServerTick: 3,
		MonsterID:  1001,
		Monster: MonsterSnapshot{
			MonsterID:   1001,
			MonsterType: 11,
			RouteID:     1,
			SpawnTick:   3,
			Progress:    0,
			Speed:       300,
			HP:          100,
			MaxHP:       100,
			Status:      MonsterAlive,
		},
	}}

	got := DeltasToProto("room-1", 3, deltas)
	if len(got.GetEvents()) != 1 {
		t.Fatalf("event count = %d, want 1", len(got.GetEvents()))
	}
	event := got.GetEvents()[0]
	if event.GetType() != pb.BattleDeltaType_BATTLE_DELTA_MONSTER_SPAWNED {
		t.Fatalf("event type = %v", event.GetType())
	}
	if event.GetMonster().GetMonsterId() != 1001 || event.GetMonster().GetStatus() != pb.BattleMonsterStatus_BATTLE_MONSTER_ALIVE {
		t.Fatalf("unexpected monster event: %+v", event)
	}
}
