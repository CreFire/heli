package main

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
