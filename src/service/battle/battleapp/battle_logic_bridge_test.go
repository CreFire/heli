package battleapp

import (
	"testing"

	battlesync "game/src/service/battle/sync"
)

func TestBattleAddrAndTokenHelpers(t *testing.T) {
	old := battleSvr
	battleSvr = &BattleSvr{roomMgr: newRoomManager()}
	defer func() { battleSvr = old }()

	room, err := battleSvr.roomMgr.createRoom("room-1", []int64{1001, 1002}, []int32{101, 102}, "battle:room-1:1001,1002")
	if err != nil {
		t.Fatalf("createRoom err: %v", err)
	}
	if room == nil || room.room == nil {
		t.Fatalf("room not created")
	}
	if room.roomSnapshot().State != battlesync.RoomStateCreated {
		t.Fatalf("room state = %s, want CREATED", room.roomSnapshot().State)
	}

	token := battleSvr.buildBattleToken(room.id, room.playerIDs)
	if token == "" {
		t.Fatalf("empty battle token")
	}
}

func TestSettleProtoBuildForRoom(t *testing.T) {
	settle := battlesync.Settlement{RoomID: "room-1", BattleID: "battle-1", Win: true, StartTick: 1, EndTick: 100, FinishReason: battlesync.FinishWin}
	protoReq := battlesync.SettlementToProto(settle)
	if protoReq.GetRoomId() != "room-1" || !protoReq.GetWin() {
		t.Fatalf("unexpected settlement proto: %+v", protoReq)
	}
}

func TestVerifyBattleToken(t *testing.T) {
	room := &battleRoom{id: "room-1", playerIDs: []int64{1001}, allowedToken: "battle:room-1:1001"}
	if err := battleSvr.verifyBattleToken(room, 1001, "room-1", "battle:room-1:1001"); err != nil {
		t.Fatalf("verifyBattleToken err: %v", err)
	}
	if err := battleSvr.verifyBattleToken(room, 1001, "room-1", "bad-token"); err == nil {
		t.Fatalf("expected invalid token error")
	}
}

func TestBattleRoomAdvanceStopsAfterFinish(t *testing.T) {
	mgr := newRoomManager()
	room, err := mgr.createRoom("room-finish", []int64{1001}, []int32{101}, "battle:room-finish:1001")
	if err != nil {
		t.Fatalf("create room err: %v", err)
	}
	room.withRoom(func(syncRoom *battlesync.Room) {
		syncRoom.Abort()
	})
	before := room.roomSnapshot().ServerTick
	after := room.advanceLoopTick()
	if after.ServerTick != before {
		t.Fatalf("closed room should not advance, before=%d after=%d", before, after.ServerTick)
	}
	if after.State != battlesync.RoomStateClosed {
		t.Fatalf("state = %s, want CLOSED", after.State)
	}
}
