package battleapp

import (
	"testing"
	"time"

	"game/src/proto/errorpb"
	"game/src/proto/pb"
	battlesync "game/src/service/battle/sync"
)

type fakeSettleSender struct {
	calls      int
	lastRoomID string
	lastSettle battlesync.Settlement
	rsp        *pb.S2SBattleSettleRSP
	err        error
}

func (f *fakeSettleSender) Send(roomID string, settlement battlesync.Settlement) (*pb.S2SBattleSettleRSP, error) {
	f.calls++
	f.lastRoomID = roomID
	f.lastSettle = settlement
	if f.rsp == nil {
		f.rsp = &pb.S2SBattleSettleRSP{RoomId: roomID, Accepted: true, Message: "ok"}
	}
	return f.rsp, f.err
}

func TestApplyBattleOpRejectsEmptyOpID(t *testing.T) {
	room, err := newRoomManager().createRoom("room-op", []int64{1001}, []int32{101}, "battle:room-op:1001")
	if err != nil {
		t.Fatalf("create room err: %v", err)
	}
	rsp := battleSvr.applyBattleOp(room, 1001, &pb.C2SBattleOpREQ{RoomId: "room-op", Op: &pb.BattlePlayerOp{Type: pb.BattleOpType_BATTLE_OP_BUY_MINER}})
	if rsp.GetCode() != errorpb.ERROR_REQUEST_PARAMS {
		t.Fatalf("code = %v, want request params", rsp.GetCode())
	}
}

func TestApplyBattleOpRejectsClosedRoom(t *testing.T) {
	room, err := newRoomManager().createRoom("room-closed", []int64{1001}, []int32{101}, "battle:room-closed:1001")
	if err != nil {
		t.Fatalf("create room err: %v", err)
	}
	room.withRoom(func(syncRoom *battlesync.Room) {
		syncRoom.Abort()
	})
	rsp := battleSvr.applyBattleOp(room, 1001, &pb.C2SBattleOpREQ{RoomId: "room-closed", OpId: "op-1", Op: &pb.BattlePlayerOp{Type: pb.BattleOpType_BATTLE_OP_BUY_MINER}})
	if rsp.GetCode() != errorpb.ERROR_FAILED || rsp.GetMessage() != "battle already finished" {
		t.Fatalf("unexpected rsp: %+v", rsp)
	}
}

func TestFinishBattleRoomSendsSettlementOnce(t *testing.T) {
	old := battleSvr
	sender := &fakeSettleSender{}
	battleSvr = &BattleSvr{roomMgr: newRoomManager(), settleSender: sender}
	defer func() { battleSvr = old }()

	room, err := battleSvr.roomMgr.createRoom("room-settle", []int64{1001}, []int32{101}, "battle:room-settle:1001")
	if err != nil {
		t.Fatalf("create room err: %v", err)
	}
	room.loop = &roomLoop{stopCh: make(chan struct{}), stoppedCh: make(chan struct{})}
	close(room.loop.stoppedCh)
	room.withRoom(func(syncRoom *battlesync.Room) {
		syncRoom.AdvanceToTick(3)
		syncRoom.Abort()
	})
	snapshot := room.roomSnapshot()

	battleSvr.finishBattleRoom(room, snapshot)
	battleSvr.finishBattleRoom(room, snapshot)

	for i := 0; i < 50 && sender.calls == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if sender.calls != 1 {
		t.Fatalf("settle send calls = %d, want 1", sender.calls)
	}
	if sender.lastSettle.RoomID != "room-settle" || sender.lastSettle.BattleID == "" {
		t.Fatalf("unexpected settlement: %+v", sender.lastSettle)
	}
	if !room.settleAcked {
		t.Fatalf("room should mark settle acked")
	}
}
