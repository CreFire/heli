package msg

import (
	"game/src/proto/pb"

	"google.golang.org/protobuf/proto"
)

type IMsgParser interface {
	C2S() any
	C2SData() []byte
	C2SString() string
	//setC2S(any)
	C2SJsonString() string
}

type MsgParser struct {
	c2s     any
	c2sFunc ParseFunc
	parser  IParser
}

//func (r *MsgParser) setC2S(c2s interface{}) {
//	r.c2s = c2s
//}

func (r *MsgParser) C2S() any {
	if r.c2s == nil && r.c2sFunc != nil {
		r.c2s = r.c2sFunc()
	}
	return r.c2s
}

func (r *MsgParser) C2SData() []byte {
	return r.parser.PackMsg(r.C2S())
}

func (r *MsgParser) C2SString() string {
	return string(r.C2SData())
}

func (r *MsgParser) C2SJsonString() string {
	return r.parser.JsonString(r.C2S())
}

type ParserType int

const (
	ParserTypePB ParserType = iota //protobuf类型，用于和客户端交互
	ParserTypeWS                   //WebSocket二进制协议
)

type ParseFunc func() any

type IParser interface {
	GetType() ParserType
	ParseC2S(msg *Message) error
	PackMsg(v any) []byte
	JsonString(v any) string
	GetRspIdByType(msg proto.Message) (pb.MSG_ID, bool)
}
