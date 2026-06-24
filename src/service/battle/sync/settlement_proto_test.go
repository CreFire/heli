package sync

import (
	"testing"

	"game/src/proto/pb"
)

func TestSettlementToProtoBuildsLogicRequest(t *testing.T) {
	settlement := Settlement{
		RoomID:       "room-1",
		BattleID:     "battle-1",
		Win:          true,
		StartTick:    10,
		EndTick:      500,
		FinishReason: FinishWin,
		Players: []PlayerSettlement{{
			PlayerID:          1,
			Gold:              120,
			Mana:              30,
			KillCount:         20,
			SummonBountyCount: 2,
			RewardGold:        50,
		}},
	}

	got := SettlementToProto(settlement)
	if got.GetRoomId() != "room-1" || got.GetBattleId() != "battle-1" || !got.GetWin() {
		t.Fatalf("unexpected settle header: %+v", got)
	}
	if got.GetFinishReason() != pb.BattleFinishReason_BATTLE_FINISH_WIN {
		t.Fatalf("finish reason = %v", got.GetFinishReason())
	}
	if len(got.GetPlayers()) != 1 || got.GetPlayers()[0].GetKillCount() != 20 || got.GetPlayers()[0].GetRewardGold() != 50 {
		t.Fatalf("unexpected players: %+v", got.GetPlayers())
	}
}
