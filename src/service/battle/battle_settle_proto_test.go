package main_test

import (
	"testing"

	"game/src/proto/pb"
)

func TestBattleSettleS2SProtocolTypesCompile(t *testing.T) {
	req := &pb.S2SBattleSettleREQ{
		RoomId:       "room-1",
		BattleId:     "battle-1",
		Win:          true,
		StartTick:    10,
		EndTick:      500,
		FinishReason: pb.BattleFinishReason_BATTLE_FINISH_WIN,
		Players: []*pb.BattlePlayerSettle{
			{PlayerId: 1, Gold: 120, Mana: 30, KillCount: 20, SummonBountyCount: 2, RewardGold: 50},
		},
	}
	if req.GetRoomId() != "room-1" || !req.GetWin() || req.GetPlayers()[0].GetPlayerId() != 1 {
		t.Fatalf("unexpected settle req: %+v", req)
	}

	rsp := &pb.S2SBattleSettleRSP{RoomId: "room-1", Accepted: true}
	if !rsp.GetAccepted() || rsp.GetRoomId() != req.GetRoomId() {
		t.Fatalf("unexpected settle rsp: %+v", rsp)
	}
}

func TestBattleSettleMessageIDsCompile(t *testing.T) {
	ids := []pb.MSG_ID{
		pb.MSG_ID_S2S_BATTLE_SETTLE_REQ,
		pb.MSG_ID_S2S_BATTLE_SETTLE_RSP,
	}
	for _, id := range ids {
		if id == pb.MSG_ID_NONE {
			t.Fatalf("settle msg id should not be NONE")
		}
	}
}
