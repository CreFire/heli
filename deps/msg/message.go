package msg

import (
	"bytes"
	"fmt"
	"game/deps/proto/msgbase"
	"game/deps/xlog"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"math"
	"strconv"
	"strings"
	"unsafe"

	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	FlagEncrypt  = 1 << 0 //数据是经过加密的
	FlagCompress = 1 << 1 //数据是经过压缩的
	FlagContinue = 1 << 2 //消息还有后续
	FlagNeedAck  = 1 << 3 //消息需要确认
	FlagAck      = 1 << 4 //确认消息
	FlagReSend   = 1 << 5 //重发消息
	FlagClient   = 1 << 6 //消息来自客服端，用于判断index来之服务器还是其他玩家
	FlagNoParse  = 1 << 7 //消息不解析
	FlagBroad    = 1 << 8 //广播消息
)

type Message struct {
	Head *msgbase.MsgHead //消息头，可能为nil
	Data []byte           //消息数据
	Body proto.Message    // 消息体
}

func (r *Message) PlayerSessId() int64 {
	if r.Head == nil {
		return 0
	}
	return r.Head.GSessId
}

func (r *Message) GId() int64 {
	if r.Head == nil {
		return 0
	}
	return r.Head.Gid
}

func (r *Message) NewMessageHead(headLen int, buffer []byte) (*msgbase.MsgHead, error) {
	if len(buffer) < headLen {
		return nil, nil
	}
	head := &msgbase.MsgHead{}
	err := proto.Unmarshal(buffer[:headLen], head)
	if err != nil {
		return nil, err
	}
	r.Head = head
	return head, nil
}

func (r *Message) GetUserInfo() (int64, int64) {
	if r.Head == nil {
		return 0, 0
	}
	return r.Head.GSessId, r.Head.Gid
}

func (r *Message) SetUserInfo(sessId int64, gid int64) *Message {
	if r.Head == nil {
		r.Head = &msgbase.MsgHead{}
	}
	r.Head.GSessId = sessId
	r.Head.Gid = gid
	return r
}

func (r *Message) Copy() *Message {
	newHead := proto.Clone(r.Head).(*msgbase.MsgHead)
	msg := &Message{
		Head: newHead,
		//IMsgParser: r.IMsgParser,
	}
	ld := len(r.Data)
	if ld > 0 {
		msg.Data = make([]byte, ld)
		copy(msg.Data, r.Data)
	}
	return msg
}

func (r *Message) Bytes(buffer *bytes.Buffer) (int, error) {
	if r.Head == nil {
		return buffer.Write(r.Data)
	}

	bodyLen := len(r.Data)
	r.Head.BodyLen = int32(bodyLen)
	headBytes, err := proto.Marshal(r.Head)
	if err != nil {
		return 0, fmt.Errorf("msgId:%v marshal head err: %v", r.Head.MsgId, err)
	}
	headLen := len(headBytes)
	if headLen > math.MaxUint16 {
		return 0, fmt.Errorf("msgId:%v marshal head len err: %v", r.Head.MsgId, headLen)
	}

	if headLen+bodyLen+HEAD_SIZE > buffer.Available() {
		buffer.Grow(headLen + bodyLen + HEAD_SIZE)
	}

	n := 0
	buffer.WriteByte(byte(headLen >> 8))
	buffer.WriteByte(byte(headLen))

	n += HEAD_SIZE
	// head body
	buffer.Write(headBytes)
	n += headLen
	// body
	buffer.Write(r.Data)
	n += bodyLen
	return n, nil
}

func (r *Message) Len() int32 {
	if r.Head != nil {
		return r.Head.BodyLen
	}
	return 0
}

func (r *Message) MsgId() int32 {
	if r.Head != nil {
		return int32(r.Head.MsgId)
	}
	return 0
}

func (r *Message) Flags() int32 {
	if r.Head != nil {
		return r.Head.Flags
	}
	return 0
}

func (r *Message) ErrorCode() errorpb.ERROR {
	if r.Head != nil {
		return r.Head.ErrCode
	}
	return errorpb.ERROR_SUCCESS
}

func (r *Message) SetSeq(seq int32) *Message {
	if r.Head != nil {
		r.Head.Flags |= FlagNeedAck
		r.Head.Seq = seq
	}
	return r
}
func (r *Message) Seq() int32 {
	return r.Head.Seq
}

func (r *Message) SetAck(ack int32) *Message {
	if r.Head != nil {
		r.Head.Flags |= FlagAck
		r.Head.Ack = ack
	}
	return r
}

func (r *Message) Ack() int32 {
	return r.Head.Ack
}

func (r *Message) SetTraceId(id int64) *Message {
	if r.Head != nil {
		r.Head.TraceId = id
	}
	return r
}
func (r *Message) TraceId() int64 {
	return r.Head.TraceId
}
func (r *Message) SetMsgType(mtype msgbase.MsgType) *Message {
	if r.Head != nil {
		r.Head.MsgType = int32(mtype)
	}
	return r
}

func (r *Message) MsgType() msgbase.MsgType {
	return msgbase.MsgType(r.Head.MsgType)
}

func (r *Message) SetHashKey(key int64) *Message {
	if r.Head != nil {
		r.Head.RpcHashKey = key
	}
	return r
}
func (r *Message) HashKey() int64 {
	if r.Head == nil {
		return 0
	}
	return r.Head.RpcHashKey
}

func (r *Message) CopyTag(old *Message) *Message {
	if r.Head != nil && old.Head != nil {
		r.Head.MsgId = old.Head.MsgId
	}
	return r
}

func (r *Message) Message() proto.Message {
	if r.Body != nil {
		return r.Body
	}
	m := PbParser.GetMessage(r.MsgId())
	if m == nil {
		xlog.Warnf("msgId:%v msg parse err", r.MsgId())
		return nil
	}
	err := proto.Unmarshal(r.Data, m)
	if err != nil {
		xlog.Warnf("C2S proto.Unmarshal failed:%s", err.Error())
		return nil
	}
	r.Body = m

	return r.Body
}

func (r *Message) ToString() string {
	var sb strings.Builder
	sb.Grow(256)

	jsonFormatter := protojson.MarshalOptions{
		Multiline: false,
	}

	dataStr := lo.Ternary(r.Message() != nil, jsonFormatter.Format(r.Message()), "nil")

	sb.WriteString("MsgId: ")
	sb.WriteString(strconv.FormatInt(int64(r.MsgId()), 10))
	sb.WriteString(", ErrCode: ")
	sb.WriteString(strconv.FormatInt(int64(r.ErrorCode()), 10))
	sb.WriteString(", MsgType: ")
	sb.WriteString(strconv.FormatInt(int64(r.MsgType()), 10))
	sb.WriteString(", Flags: ")
	sb.WriteString(strconv.FormatInt(int64(r.Flags()), 10))
	sb.WriteString(", Seq: ")
	sb.WriteString(strconv.FormatInt(int64(r.Seq()), 10))
	sb.WriteString(", Ack: ")
	sb.WriteString(strconv.FormatInt(int64(r.Ack()), 10))
	sb.WriteString(", TraceId: ")
	sb.WriteString(strconv.FormatInt(r.TraceId(), 10))
	sb.WriteString(", RpcHashKey: ")
	sb.WriteString(strconv.FormatInt(r.HashKey(), 10))
	sb.WriteString(", DataLen: ")
	sb.WriteString(strconv.FormatInt(int64(len(r.Data)), 10))
	sb.WriteString(", Data: [")
	sb.WriteString(dataStr)
	sb.WriteString("]")

	return sb.String()
}

func (r *Message) MessageTag() string {
	b := make([]byte, 0, 64)
	b = append(b, "msgid:"...)
	b = strconv.AppendInt(b, int64(r.MsgId()), 10)
	b = append(b, " gid:"...)
	b = strconv.AppendInt(b, int64(r.GId()), 10)
	b = append(b, " tid:"...)
	b = strconv.AppendInt(b, int64(r.TraceId()), 10)

	return unsafe.String(unsafe.SliceData(b), len(b))
}

func (r *Message) MessageString() string {
	jsonFormatter := protojson.MarshalOptions{
		Multiline: false,
	}
	dataStr := lo.Ternary(r.Message() != nil, jsonFormatter.Format(r.Message()), "nil")
	return dataStr
}

func NewMsg(msgId pb.MSG_ID, data []byte) *Message {
	message := &Message{
		Head: &msgbase.MsgHead{
			BodyLen: int32(len(data)),
			MsgId:   msgId,
		},
		Data: data,
	}
	return message
}

func NewMsgWithCode(msgId pb.MSG_ID, code errorpb.ERROR, data []byte) *Message {
	return &Message{
		Head: &msgbase.MsgHead{
			MsgId:   msgId,
			ErrCode: code,
			BodyLen: int32(len(data)),
		},
		Data: data,
	}
}

func NewMsgWithProto(msgid pb.MSG_ID, msg proto.Message) *Message {
	data, _ := proto.Marshal(msg)
	return NewMsg(msgid, data)
}

func NewRspMsgWithProtoAndCode(ReqMsgId pb.MSG_ID, code errorpb.ERROR, msg proto.Message) *Message {
	msgId := pb.MSG_ID(ReqMsgId + 1) // 固定规则rsp 消息id = req消息id + 1
	if msg != nil && xlog.GetLogLevel() <= xlog.LOG_LEVEL_DEBUG {
		rspId, ok := PbParser.GetRspIdByType(msg)
		if !ok || rspId != msgId {
			panic("break msg define rule .rspId = reqId +1")
		}
	}

	data, _ := proto.Marshal(msg)
	return NewMsgWithCode(msgId, code, data)
}
