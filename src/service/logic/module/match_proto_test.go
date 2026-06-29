package module_test

import (
	"testing"

	"game/src/proto/pb"
)

func TestLogicMatchProtocolCarriesBattleConnectInfo(t *testing.T) {
	req := &pb.C2SMatchJoin{LevelId: 1, CombatType: 1, TowerDeck: []int32{101, 102}}
	if req.GetLevelId() != 1 || len(req.GetTowerDeck()) != 2 {
		t.Fatalf("unexpected match req: %+v", req)
	}

	rsp := &pb.S2CMatchJoin{
		RoomId:      "room-1",
		BattleAddr:  "127.0.0.1:7002",
		BattleToken: "token",
		PlayerIds:   []int64{1, 2},
	}
	if rsp.GetRoomId() != "room-1" || rsp.GetBattleAddr() == "" || rsp.GetBattleToken() == "" || len(rsp.GetPlayerIds()) != 2 {
		t.Fatalf("unexpected match rsp: %+v", rsp)
	}
}

func TestLogicMatchMessageIDsCompile(t *testing.T) {
	ids := []pb.MSG_ID{pb.MSG_ID_MATCH_JOIN_REQ, pb.MSG_ID_MATCH_JOIN_RSP}
	for _, id := range ids {
		if id == pb.MSG_ID_NONE {
			t.Fatalf("match msg id should not be NONE")
		}
	}
}

func TestLogicSayHelloProtocolCarriesEchoPayload(t *testing.T) {
	req := &pb.SayHelloREQ{Id: 7, Type: "logic"}
	if req.GetId() != 7 || req.GetType() != "logic" {
		t.Fatalf("unexpected say hello req: %+v", req)
	}

	rsp := &pb.SayHelloRSP{Id: req.GetId(), Type: req.GetType()}
	if rsp.GetId() != 7 || rsp.GetType() != "logic" {
		t.Fatalf("unexpected say hello rsp: %+v", rsp)
	}
}

func TestLogicSayHelloMessageIDsCompile(t *testing.T) {
	ids := []pb.MSG_ID{pb.MSG_ID_SAY_HELLO_REQ, pb.MSG_ID_SAY_HELLO_RSP}
	for _, id := range ids {
		if id == pb.MSG_ID_NONE {
			t.Fatalf("say hello msg id should not be NONE")
		}
	}
}
