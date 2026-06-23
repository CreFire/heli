package msg

import (
	"testing"

	"game/deps/proto/msgbase"
	"game/src/proto/pb"
)

func TestNewMsgUsesRawMsgIDAndSplitsHeader(t *testing.T) {
	m := NewMsg(pb.MSG_ID(MakeMSGID(2, 1)), []byte("body"))
	if m == nil || m.Head == nil {
		t.Fatalf("NewMsg returned nil message/header")
	}
	if got := uint32(m.MsgId()); got != MakeMSGID(2, 1) {
		t.Fatalf("MsgId mismatch: got %d want %d", got, MakeMSGID(2, 1))
	}
	if m.Head.MainCmdId != 2 || m.Head.SubCmdId != 1 {
		t.Fatalf("header split mismatch: main=%d sub=%d", m.Head.MainCmdId, m.Head.SubCmdId)
	}
	if m.Head.BodyLen != 4 || string(m.Data) != "body" {
		t.Fatalf("body mismatch: len=%d data=%q", m.Head.BodyLen, string(m.Data))
	}
}

func TestPBParserMapsClientMessagesByCompatMsgID(t *testing.T) {
	p := NewPBParser()

	msgID, ok := p.GetRspIdByType(&pb.LoginReq{})
	if !ok || msgID != pb.MSG_ID_LOGIN_REQ {
		t.Fatalf("LoginReq msg id mismatch: got %d ok=%v", msgID, ok)
	}

	body := []byte{0x0a, 0x03, '1', '2', '3'}
	m := &Message{Head: &msgbase.MsgHead{Cmd: uint32(pb.MSG_ID_LOGIN_REQ)}, Data: body}
	got := m.Message()
	if _, ok := got.(*pb.LoginReq); !ok {
		t.Fatalf("Message parsed type mismatch: got %T", got)
	}
}
