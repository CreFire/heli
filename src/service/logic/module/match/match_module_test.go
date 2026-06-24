package match

import (
	"testing"

	"game/src/proto/errorpb"
	"game/src/proto/pb"
)

func TestMatchJoinCreatesBattleRoomAndReturnsConnectInfo(t *testing.T) {
	module := NewMatchModule(nil, nil)
	module.SetCreateRoomFunc(func(req CreateRoomRequest) (*CreateRoomResult, error) {
		if req.PlayerID != 1001 || req.LevelID != 2 || req.CombatType != 1 || len(req.TowerDeck) != 2 {
			t.Fatalf("unexpected create room req: %+v", req)
		}
		return &CreateRoomResult{RoomID: req.RoomID, BattleAddr: "127.0.0.1:7002", BattleToken: "token", PlayerIDs: []int64{1001}}, nil
	})

	code, msg := module.Join(1001, &pb.C2SMatchJoin{LevelId: 2, CombatType: 1, TowerDeck: []int32{101, 102}})
	if code != errorpb.ERROR_SUCCESS {
		t.Fatalf("code=%v", code)
	}
	rsp := msg.(*pb.S2CMatchJoin)
	if rsp.GetRoomId() == "" || rsp.GetBattleAddr() != "127.0.0.1:7002" || rsp.GetBattleToken() != "token" || len(rsp.GetPlayerIds()) != 1 {
		t.Fatalf("unexpected rsp: %+v", rsp)
	}
}

func TestMatchJoinRejectsNilRequest(t *testing.T) {
	module := NewMatchModule(nil, nil)
	code, _ := module.Join(1001, nil)
	if code != errorpb.ERROR_REQUEST_PARAMS {
		t.Fatalf("code=%v, want %v", code, errorpb.ERROR_REQUEST_PARAMS)
	}
}
