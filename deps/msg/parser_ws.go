package msg

import (
	"encoding/binary"
	"fmt"
	"game/deps/proto/msgbase"
	"game/src/proto/pb"
	"math"
	"strings"
	"unicode"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const (
	wsLenMsgLen    = 2
	wsMinMsgLen    = 1
	wsMaxMsgLen    = 4096
	wsHeaderMinLen = wsLenMsgLen + 1 + 1 + 4
	WSMsgTypeNone  = uint8(0)
	WSMsgTypeRPC   = uint8(0x01)
)

type RecvMsg struct {
	RpcCallId uint32
	MsgId     uint32
	Msg       any
	MsgType   uint8
}

var DefaultRecvMsg = &RecvMsg{0, 0, nil, WSMsgTypeNone}

func FlagSet(value uint8, flag uint8) uint8 { return value | flag }

func FlagUnset(value uint8, flag uint8) uint8 { return value & (^flag) }

func FlagGet(value uint8, flag uint8) bool { return (value & flag) != 0 }

type WsParser struct {
}

func NewWsParser() *WsParser {
	parser := &WsParser{}
	return parser
}

func (p *WsParser) ParseC2S(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("ws parser: nil message")
	}
	frameLen := len(msg.Data)
	if frameLen < wsHeaderMinLen {
		return fmt.Errorf("ws parser: frame too short: %d", frameLen)
	}

	declaredLen := int(binary.LittleEndian.Uint16(msg.Data[:wsLenMsgLen]))
	if declaredLen < wsMinMsgLen {
		return fmt.Errorf("ws parser: invalid len: %d", declaredLen)
	}
	if declaredLen > wsMaxMsgLen {
		return fmt.Errorf("ws parser: packet too large: %d", declaredLen)
	}
	actualLen := frameLen - wsLenMsgLen
	if actualLen < declaredLen {
		return fmt.Errorf("ws parser: incomplete frame: declared=%d actual=%d", declaredLen, actualLen)
	}

	packet := msg.Data[wsLenMsgLen : wsLenMsgLen+declaredLen]
	if !validWSCheckCode(packet) {
		return fmt.Errorf("ws parser: checkCode invalid")
	}

	checkCode := packet[0]
	payload := packet[1:]
	if len(payload) < 1+4 {
		return fmt.Errorf("ws parser: payload too short: %d", len(payload))
	}

	msgType := payload[0]
	cmd := binary.LittleEndian.Uint32(payload[1:5])
	ptr := 5
	rpcCallID := uint32(0)
	if FlagGet(msgType, WSMsgTypeRPC) {
		if len(payload)-ptr < 4 {
			return fmt.Errorf("ws parser: rpc payload too short: %d", len(payload))
		}
		rpcCallID = binary.LittleEndian.Uint32(payload[ptr : ptr+4])
		ptr += 4
	}
	if msgType != WSMsgTypeNone && rpcCallID == 0 {
		mainCmdID, subCmdID := GetCmd(cmd)
		return fmt.Errorf("ws parser: msgType[%d]!=0 but rpcCallId[%d]==0 cmd[%d,%d]", msgType, rpcCallID, mainCmdID, subCmdID)
	}

	mainCmdID, subCmdID := GetCmd(cmd)
	body := payload[ptr:]
	msg.Head = &msgbase.MsgHead{
		BodyLen:   int32(len(body)),
		CheckCode: uint32(checkCode),
		Cmd:       cmd,
		MainCmdId: uint32(mainCmdID),
		SubCmdId:  uint32(subCmdID),
		MsgType:   uint32(msgType),
		RpcCallId: rpcCallID,
	}
	msg.Data = append(msg.Data[:0], body...)
	return nil
}

func (p *WsParser) Unmarshal(data []byte) (*RecvMsg, error) {
	if len(data) < 5 {
		return &RecvMsg{0, 0, nil, WSMsgTypeNone}, fmt.Errorf("ws parser: data too short")
	}

	msgType := data[0]
	cmd := binary.LittleEndian.Uint32(data[1:5])
	idx := 5
	rpcCallID := uint32(0)
	if FlagGet(msgType, WSMsgTypeRPC) {
		if len(data)-idx < 4 {
			mainCmdID, subCmdID := GetCmd(cmd)
			return &RecvMsg{0, cmd, nil, msgType}, fmt.Errorf("ws parser: rpc data too short cmd[%d,%d]", mainCmdID, subCmdID)
		}
		rpcCallID = binary.LittleEndian.Uint32(data[idx : idx+4])
		idx += 4
	}
	if msgType != WSMsgTypeNone && rpcCallID == 0 {
		mainCmdID, subCmdID := GetCmd(cmd)
		return &RecvMsg{rpcCallID, cmd, nil, msgType}, fmt.Errorf("ws parser: msgType[%d]!=0 but rpcCallId[%d]==0 cmd[%d,%d]", msgType, rpcCallID, mainCmdID, subCmdID)
	}

	message := &Message{
		Head: &msgbase.MsgHead{
			BodyLen:   int32(len(data) - idx),
			Cmd:       cmd,
			MainCmdId: uint32(uint16(cmd)),
			SubCmdId:  uint32(uint16(cmd >> 16)),
			MsgType:   uint32(msgType),
			RpcCallId: rpcCallID,
		},
		Data: append([]byte(nil), data[idx:]...),
	}

	return &RecvMsg{rpcCallID, cmd, message, msgType}, nil
}

func (p *WsParser) MarshalCmd(recv *RecvMsg, cmdId uint16, actId uint16) ([]byte, error) {
	if recv == nil {
		recv = DefaultRecvMsg
	}
	cmd := MakeMSGID(cmdId, actId)
	if !FlagGet(recv.MsgType, WSMsgTypeRPC) {
		header := make([]byte, 5)
		header[0] = recv.MsgType
		binary.LittleEndian.PutUint32(header[1:], cmd)
		return header, nil
	}

	if recv.RpcCallId == 0 {
		return nil, fmt.Errorf("ws parser: rpc call id is zero cmd[%d,%d]", cmdId, actId)
	}
	header := make([]byte, 9)
	header[0] = recv.MsgType
	binary.LittleEndian.PutUint32(header[1:], cmd)
	binary.LittleEndian.PutUint32(header[5:], recv.RpcCallId)
	return header, nil
}

func (p *WsParser) Marshal(recv *RecvMsg, mainCmdID uint16, subCmdID uint16, body []byte) ([][]byte, error) {
	header, err := p.MarshalCmd(recv, mainCmdID, subCmdID)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return [][]byte{header}, nil
	}
	return [][]byte{header, body}, nil
}

func (p *WsParser) Pack(args ...[]byte) ([]byte, error) {
	var msgLen uint32
	for i := range args {
		msgLen += uint32(len(args[i]))
	}
	msgLen++

	if msgLen > wsMaxMsgLen {
		return nil, fmt.Errorf("message too long: msgLen[%d]", msgLen)
	}
	if msgLen < wsMinMsgLen {
		return nil, fmt.Errorf("message too short")
	}
	if msgLen > math.MaxUint16 {
		return nil, fmt.Errorf("message too long for uint16 len: msgLen[%d]", msgLen)
	}

	frame := make([]byte, wsLenMsgLen+msgLen)
	binary.LittleEndian.PutUint16(frame[:wsLenMsgLen], uint16(msgLen))

	var checkCode byte
	for i := range args {
		for j := 0; j < len(args[i]); j++ {
			checkCode += args[i][j]
		}
	}

	pos := wsLenMsgLen
	frame[pos] = ^checkCode + 1
	pos++
	for i := range args {
		copy(frame[pos:], args[i])
		pos += len(args[i])
	}

	return frame, nil
}

func (p *WsParser) GetType() ParserType { return ParserTypeWS }

func (p *WsParser) PackMsg(v any) []byte {
	if v == nil {
		return nil
	}

	var (
		msgId     uint32
		msgType   uint8
		rpcCallID uint32
		body      []byte
		err       error
	)

	switch m := v.(type) {
	case *Message:
		if m == nil {
			return nil
		}
		if m.Head != nil {
			msgId = m.Head.Cmd
			msgType = uint8(m.Head.MsgType)
			rpcCallID = m.Head.RpcCallId
			if rpcCallID > 0 {
				msgType |= WSMsgTypeRPC
			}
		}
		body = m.Data
		if len(body) == 0 && m.Body != nil {
			body, err = proto.Marshal(m.Body)
		}
	case proto.Message:
		body, err = proto.Marshal(m)
		if msgID, ok := p.GetRspIdByType(m); ok {
			msgId = uint32(msgID)
		}
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	recv := &RecvMsg{RpcCallId: rpcCallID, MsgId: msgId, MsgType: msgType}
	cmdId, actId := GetCmd(msgId)
	parts, err := p.Marshal(recv, cmdId, actId, body)
	if err != nil {
		return nil
	}
	frame, err := p.Pack(parts...)
	if err != nil {
		return nil
	}
	return frame
}

func (p *WsParser) JsonString(v any) string {
	switch m := v.(type) {
	case nil:
		return ""
	case *Message:
		return m.MessageString()
	case proto.Message:
		return protojson.MarshalOptions{Multiline: false}.Format(m)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (p *WsParser) GetRspIdByType(msg proto.Message) (pb.MSG_ID, bool) {
	if PbParser == nil {
		return pb.MSG_ID_NONE, false
	}
	return PbParser.GetRspIdByType(msg)
}

func validWSCheckCode(packet []byte) bool {
	var sum uint8
	for _, b := range packet {
		sum += b
	}
	return sum == 0
}

func makeWSCheckCode(payload []byte) uint8 {
	var sum uint8
	for _, b := range payload {
		sum += b
	}
	return ^sum + 1
}

type PBParser struct {
	msgFactories map[int32]func() proto.Message
	typeMsgIDs   map[protoreflect.FullName]pb.MSG_ID
}

func NewPBParser() *PBParser {
	p := &PBParser{
		msgFactories: make(map[int32]func() proto.Message),
		typeMsgIDs:   make(map[protoreflect.FullName]pb.MSG_ID),
	}

	for id, enumName := range pb.MSG_ID_name {
		msgID := pb.MSG_ID(id)
		for _, name := range messageNameCandidates(enumName) {
			mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName("pb." + name))
			if err != nil {
				continue
			}
			fullName := mt.Descriptor().FullName()
			p.msgFactories[id] = func(mt protoreflect.MessageType) func() proto.Message {
				return func() proto.Message { return mt.New().Interface() }
			}(mt)
			p.typeMsgIDs[fullName] = msgID
			break
		}
	}

	return p
}

func (p *PBParser) GetMessage(msgID int32) proto.Message {
	if p == nil {
		return nil
	}
	if f := p.msgFactories[msgID]; f != nil {
		return f()
	}
	return nil
}

func (p *PBParser) GetRspIdByType(msg proto.Message) (pb.MSG_ID, bool) {
	if p == nil || msg == nil {
		return pb.MSG_ID_NONE, false
	}
	fullName := msg.ProtoReflect().Descriptor().FullName()
	if msgID, ok := p.typeMsgIDs[fullName]; ok {
		return msgID, true
	}

	shortName := string(fullName.Name())
	name := camelToEnumName(shortName)
	if id, ok := pb.MSG_ID_value[name]; ok {
		return pb.MSG_ID(id), true
	}
	return pb.MSG_ID_NONE, false
}

func messageNameCandidates(enumName string) []string {
	base := strings.TrimPrefix(enumName, "MSG_ID_")
	return []string{
		enumToCamel(base),
		enumToCamel(strings.TrimSuffix(base, "_REQ")),
		enumToCamel(strings.TrimSuffix(base, "_RESP")),
		enumToCamel(strings.TrimSuffix(base, "_RSP")),
		enumToCamel(strings.TrimSuffix(base, "_NTF")),
	}
}

func enumToCamel(s string) string {
	parts := strings.Split(strings.ToLower(s), "_")
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

func camelToEnumName(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()

}
