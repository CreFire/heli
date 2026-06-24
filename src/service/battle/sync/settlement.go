package sync

import "game/src/proto/pb"

type FinishReason string

const (
	FinishNone    FinishReason = "NONE"
	FinishWin     FinishReason = "WIN"
	FinishLose    FinishReason = "LOSE"
	FinishTimeout FinishReason = "TIMEOUT"
	FinishAbort   FinishReason = "ABORT"
)

type Settlement struct {
	RoomID       string
	BattleID     string
	Win          bool
	StartTick    int64
	EndTick      int64
	FinishReason FinishReason
	Players      []PlayerSettlement
}

type PlayerSettlement struct {
	PlayerID          int64
	Gold              int64
	Mana              int64
	KillCount         int32
	SummonBountyCount int32
	RewardGold        int64
}

func SettlementToProto(settlement Settlement) *pb.S2SBattleSettleREQ {
	out := &pb.S2SBattleSettleREQ{
		RoomId:       settlement.RoomID,
		BattleId:     settlement.BattleID,
		Win:          settlement.Win,
		StartTick:    settlement.StartTick,
		EndTick:      settlement.EndTick,
		FinishReason: finishReasonToProto(settlement.FinishReason),
		Players:      make([]*pb.BattlePlayerSettle, 0, len(settlement.Players)),
	}
	for _, player := range settlement.Players {
		out.Players = append(out.Players, &pb.BattlePlayerSettle{
			PlayerId:          player.PlayerID,
			Gold:              player.Gold,
			Mana:              player.Mana,
			KillCount:         player.KillCount,
			SummonBountyCount: player.SummonBountyCount,
			RewardGold:        player.RewardGold,
		})
	}
	return out
}

func finishReasonToProto(reason FinishReason) pb.BattleFinishReason {
	switch reason {
	case FinishWin:
		return pb.BattleFinishReason_BATTLE_FINISH_WIN
	case FinishLose:
		return pb.BattleFinishReason_BATTLE_FINISH_LOSE
	case FinishTimeout:
		return pb.BattleFinishReason_BATTLE_FINISH_TIMEOUT
	case FinishAbort:
		return pb.BattleFinishReason_BATTLE_FINISH_ABORT
	default:
		return pb.BattleFinishReason_BATTLE_FINISH_NONE
	}
}
