package sync

import "game/src/proto/pb"

func SnapshotToProto(roomID string, snapshot Snapshot) *pb.S2CBattleSnapshotNTF {
	out := &pb.S2CBattleSnapshotNTF{
		RoomId:     roomID,
		ServerTick: snapshot.ServerTick,
		Players:    make([]*pb.BattlePlayerState, 0, len(snapshot.Players)),
		Towers:     make([]*pb.BattleTowerState, 0, len(snapshot.Towers)),
		Monsters:   make([]*pb.BattleMonsterState, 0, len(snapshot.Monsters)),
	}
	for _, player := range snapshot.Players {
		out.Players = append(out.Players, playerSnapshotToProto(player))
	}
	for _, tower := range snapshot.Towers {
		out.Towers = append(out.Towers, towerSnapshotToProto(tower))
	}
	for _, monster := range snapshot.Monsters {
		out.Monsters = append(out.Monsters, monsterSnapshotToProto(monster))
	}
	return out
}

func DeltasToProto(roomID string, serverTick int64, deltas []Delta) *pb.S2CBattleDeltaNTF {
	out := &pb.S2CBattleDeltaNTF{
		RoomId:     roomID,
		ServerTick: serverTick,
		Events:     make([]*pb.BattleStateDelta, 0, len(deltas)),
	}
	for _, delta := range deltas {
		out.Events = append(out.Events, deltaToProto(delta))
	}
	return out
}

func playerSnapshotToProto(player PlayerSnapshot) *pb.BattlePlayerState {
	out := &pb.BattlePlayerState{
		PlayerId: player.PlayerID,
		Gold:     player.Gold,
		Mana:     player.Mana,
		Miners:   make([]*pb.BattleMinerState, 0, len(player.Miners)),
	}
	for _, miner := range player.Miners {
		out.Miners = append(out.Miners, minerSnapshotToProto(player.PlayerID, miner))
	}
	return out
}

func towerSnapshotToProto(tower TowerSnapshot) *pb.BattleTowerState {
	return &pb.BattleTowerState{
		TowerId:       tower.TowerID,
		OwnerPlayerId: tower.OwnerPlayerID,
		TowerType:     tower.TypeID,
		Level:         tower.Level,
		GridId:        tower.GridID,
	}
}

func minerSnapshotToProto(ownerPlayerID int64, miner MinerSnapshot) *pb.BattleMinerState {
	return &pb.BattleMinerState{
		MinerId:         miner.MinerID,
		OwnerPlayerId:   ownerPlayerID,
		NextProduceTick: miner.NextProduceTick,
	}
}

func monsterSnapshotToProto(monster MonsterSnapshot) *pb.BattleMonsterState {
	return &pb.BattleMonsterState{
		MonsterId:   monster.MonsterID,
		MonsterType: monster.MonsterType,
		RouteId:     monster.RouteID,
		SpawnTick:   monster.SpawnTick,
		Progress:    monster.Progress,
		Speed:       monster.Speed,
		Hp:          monster.HP,
		MaxHp:       monster.MaxHP,
		Status:      monsterStatusToProto(monster.Status),
	}
}

func deltaToProto(delta Delta) *pb.BattleStateDelta {
	out := &pb.BattleStateDelta{
		Type:            deltaTypeToProto(delta.Type),
		ServerTick:      delta.ServerTick,
		PlayerId:        delta.PlayerID,
		OpId:            delta.OpID,
		TowerId:         delta.TowerID,
		MaterialTowerId: delta.MaterialTowerID,
		MinerId:         delta.MinerID,
		Gold:            delta.Gold,
		Mana:            delta.Mana,
	}
	if delta.Tower.TowerID != 0 {
		out.Tower = towerSnapshotToProto(delta.Tower)
	}
	if delta.Monster.MonsterID != 0 {
		out.Monster = monsterSnapshotToProto(delta.Monster)
	}
	return out
}

func deltaTypeToProto(deltaType DeltaType) pb.BattleDeltaType {
	switch deltaType {
	case DeltaResourceChanged:
		return pb.BattleDeltaType_BATTLE_DELTA_RESOURCE_CHANGED
	case DeltaTowerBuilt:
		return pb.BattleDeltaType_BATTLE_DELTA_TOWER_BUILT
	case DeltaTowerRerolled:
		return pb.BattleDeltaType_BATTLE_DELTA_TOWER_REROLLED
	case DeltaTowerMerged:
		return pb.BattleDeltaType_BATTLE_DELTA_TOWER_MERGED
	case DeltaMinerBought:
		return pb.BattleDeltaType_BATTLE_DELTA_MINER_BOUGHT
	case DeltaMinerProduced:
		return pb.BattleDeltaType_BATTLE_DELTA_MINER_PRODUCED
	case DeltaMonsterSpawned:
		return pb.BattleDeltaType_BATTLE_DELTA_MONSTER_SPAWNED
	case DeltaMonsterHPChanged:
		return pb.BattleDeltaType_BATTLE_DELTA_MONSTER_HP_CHANGED
	case DeltaMonsterStatusChanged:
		return pb.BattleDeltaType_BATTLE_DELTA_MONSTER_STATUS_CHANGED
	case DeltaMonsterDead:
		return pb.BattleDeltaType_BATTLE_DELTA_MONSTER_DEAD
	case DeltaMonsterArrived:
		return pb.BattleDeltaType_BATTLE_DELTA_MONSTER_ARRIVED
	case DeltaMonsterProgressFixed:
		return pb.BattleDeltaType_BATTLE_DELTA_MONSTER_PROGRESS_FIXED
	default:
		return pb.BattleDeltaType_BATTLE_DELTA_NONE
	}
}

func monsterStatusToProto(status MonsterStatus) pb.BattleMonsterStatus {
	switch status {
	case MonsterAlive:
		return pb.BattleMonsterStatus_BATTLE_MONSTER_ALIVE
	case MonsterDead:
		return pb.BattleMonsterStatus_BATTLE_MONSTER_DEAD
	case MonsterArrived:
		return pb.BattleMonsterStatus_BATTLE_MONSTER_ARRIVED
	case MonsterPaused:
		return pb.BattleMonsterStatus_BATTLE_MONSTER_PAUSED
	default:
		return pb.BattleMonsterStatus_BATTLE_MONSTER_NONE
	}
}
