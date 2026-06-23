package msg

import (
	"testing"

	"game/src/proto/pb"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type testParser struct {
	last any
}

func (p *testParser) GetType() ParserType         { return ParserTypePB }
func (p *testParser) ParseC2S(msg *Message) error { return nil }
func (p *testParser) PackMsg(v any) []byte        { return nil }
func (p *testParser) JsonString(v any) string {
	p.last = v
	if v == nil {
		return "nil"
	}
	return "ok"
}
func (p *testParser) GetRspIdByType(_ proto.Message) (pb.MSG_ID, bool) {
	return 0, false
}

func TestMsgParserC2SJsonStringLoadsLazyMessage(t *testing.T) {
	parser := &testParser{}
	called := 0
	msgParser := MsgParser{
		c2sFunc: func() any {
			called++
			return "payload"
		},
		parser: parser,
	}

	got := msgParser.C2SJsonString()

	require.Equal(t, "ok", got)
	require.Equal(t, 1, called)
	require.Equal(t, "payload", parser.last)
}
