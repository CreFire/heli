package battle

import (
	"testing"

	"game/src/proto/pb"
)

func TestValidateAndMarkRejectsInvalidSettle(t *testing.T) {
	h := NewHandler()
	rsp := h.validateAndMark(&pb.S2SBattleSettleREQ{RoomId: "", BattleId: "battle-1"})
	if rsp.GetAccepted() || rsp.GetMessage() == "" {
		t.Fatalf("unexpected rsp: %+v", rsp)
	}

	rsp = h.validateAndMark(&pb.S2SBattleSettleREQ{RoomId: "room-1", BattleId: "battle-1", StartTick: 10, EndTick: 5, Players: []*pb.BattlePlayerSettle{{PlayerId: 1}}})
	if rsp.GetAccepted() {
		t.Fatalf("expected invalid tick reject")
	}
}

func TestValidateAndMarkAcceptsDuplicateIdempotently(t *testing.T) {
	h := NewHandler()
	req := &pb.S2SBattleSettleREQ{RoomId: "room-1", BattleId: "battle-1", StartTick: 1, EndTick: 2, Players: []*pb.BattlePlayerSettle{{PlayerId: 1}}}
	first := h.validateAndMark(req)
	second := h.validateAndMark(req)
	if !first.GetAccepted() || !second.GetAccepted() {
		t.Fatalf("duplicate should stay accepted, first=%+v second=%+v", first, second)
	}
	if second.GetMessage() != "duplicate settle accepted" {
		t.Fatalf("duplicate message = %q", second.GetMessage())
	}
}
