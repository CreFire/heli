package msg

import (
	"fmt"
	"game/deps/basal"
	"game/deps/xlog"
	"game/src/proto/pb"
	"reflect"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type PBParser struct {
	msgMap      map[pb.MSG_ID]MsgParser
	rspIdByType map[protoreflect.MessageType]pb.MSG_ID
	protoMap    map[pb.MSG_ID]proto.Message
	req2Rsp     map[pb.MSG_ID]pb.MSG_ID
}

func NewPBParser() *PBParser {
	parser := &PBParser{
		msgMap:      make(map[pb.MSG_ID]MsgParser),
		rspIdByType: map[protoreflect.MessageType]pb.MSG_ID{},
		protoMap:    make(map[pb.MSG_ID]proto.Message),
	}
	parser.autoRegisterPb()
	if err := parser.checkREQRSPIdMatch(); err != nil {
		panic(err)
	}
	return parser
}

func (p *PBParser) autoRegisterPb() {
	for msgId, name := range pb.MSG_ID_name {
		if !filterMsgId(name) {
			continue
		}

		var body protoreflect.MessageType
		var err error
		for _, pkgName := range PACKAGE_NAME_LIST {
			body, err = createInstanceFromMessageName(fmt.Sprintf("%s.%v", pkgName, convertToGoStructName(name)))
			if err == nil {
				break
			}
		}

		if body == nil || err != nil {
			xlog.Errorf("msgId:%v bind msg err:%v", name, err)
			continue
		}
		pbMsgId := pb.MSG_ID(msgId)
		p.msgMap[pbMsgId] = MsgParser{
			c2sFunc: func() any {
				return body.New().Interface()
			},
			parser: p,
		}

		p.rspIdByType[body.New().Type()] = pbMsgId
		p.protoMap[pb.MSG_ID(msgId)] = body.New().Interface()
	}
}

func (p *PBParser) checkREQRSPIdMatch() error {
	for msgId := range p.msgMap {
		if strings.HasSuffix(msgId.String(), "REQ") {
			rsp := strings.Replace(msgId.String(), "REQ", "RSP", 1)
			rspId, ok := pb.MSG_ID_value[rsp]
			if !ok {
				return fmt.Errorf("req:%v id:%v not found rsp", msgId, msgId)
			}
			if msgId+1 != pb.MSG_ID(rspId) {
				return fmt.Errorf("req:%v id:%v miss match rsp:%v id:%v", msgId, int32(msgId), rsp, rspId)
			}
		}
	}
	return nil
}

func (p *PBParser) GetMessage(msgId int32) proto.Message {
	x := p.protoMap[pb.MSG_ID(msgId)]
	if x == nil {
		return nil
	}
	return proto.Clone(x)
}

func (p *PBParser) GetRspIdByType(msg proto.Message) (pb.MSG_ID, bool) {
	id, exist := p.rspIdByType[msg.ProtoReflect().Type()]
	return id, exist
}

func (p *PBParser) GetType() ParserType {
	return ParserTypePB
}

func (p *PBParser) ParseC2S(msg *Message) error {
	if msg == nil || msg.MsgId() == 0 {
		return fmt.Errorf("ParseC2S message is nil or msgId is 0")
	}
	if msg.Head.Flags&FlagNoParse > 0 {
		return nil
	}
	msgP, ok := p.msgMap[pb.MSG_ID(msg.MsgId())]
	if !ok {
		return nil
	}
	if msgP.C2S() == nil {
		return nil
	}
	// msg.IMsgParser = &msgP
	if err := PBUnPack(msg.Data, msgP.C2S()); err != nil {
		return err
	}
	return nil
}

func (p *PBParser) PackMsg(v any) []byte {
	data, _ := PBPack(v)
	return data
}

func (p *PBParser) JsonString(v any) string {
	json, _ := protojson.Marshal(v.(proto.Message))
	return string(json)
}

func PBUnPack(data []byte, msg any) error {
	if msg == nil {
		return fmt.Errorf("PBUnPack msg is nil")
	}
	m, ok := msg.(proto.Message)
	if !ok {
		return fmt.Errorf("PBUnPack failed: not proto.Message, actual type: %v", reflect.TypeOf(msg))
	}
	err := proto.Unmarshal(data, m)
	if err != nil {
		return fmt.Errorf("PBUnPack Unmarshal err: %s, msg type: %v, len: %d", err.Error(), basal.Type(msg), len(data))
	}
	return nil
}

func PBPack(msg any) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("PBPack msg is nil")
	}

	m, ok := msg.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("PBPack failed: not proto.Message, actual type: %v", reflect.TypeOf(msg))
	}

	data, err := proto.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("PBPack Marshal err: %v msg:%v", err, msg)
	}

	return data, nil
}
