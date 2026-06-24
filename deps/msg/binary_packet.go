package msg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"google.golang.org/protobuf/proto"
)

const (
	MsgTypeNone uint8 = 0
	MsgTypeRPC  uint8 = 0x01
)

type Packet struct {
	MsgType   uint8
	MainCmd   uint16
	SubCmd    uint16
	RpcCallID uint32
	Body      []byte
}

func PackMessage(msgType uint8, mainCmd uint16, subCmd uint16, rpcCallID uint32, bodyMsg proto.Message) ([]byte, error) {
	var body []byte
	var err error
	if bodyMsg != nil {
		body, err = proto.Marshal(bodyMsg)
		if err != nil {
			return nil, err
		}
	}

	payloadLen := 1 + 1 + 2 + 2 + len(body)
	if msgType&MsgTypeRPC != 0 {
		payloadLen += 4
	}
	if payloadLen > math.MaxUint16 {
		return nil, fmt.Errorf("payload too large: %d", payloadLen)
	}

	frame := make([]byte, 2+payloadLen)
	binary.LittleEndian.PutUint16(frame[:2], uint16(payloadLen))

	index := 2
	checkCodeIndex := index
	index++

	frame[index] = msgType
	index++

	binary.LittleEndian.PutUint16(frame[index:index+2], mainCmd)
	index += 2

	binary.LittleEndian.PutUint16(frame[index:index+2], subCmd)
	index += 2

	if msgType&MsgTypeRPC != 0 {
		binary.LittleEndian.PutUint32(frame[index:index+4], rpcCallID)
		index += 4
	}

	copy(frame[index:], body)

	var sum byte
	for i := 3; i < len(frame); i++ {
		sum += frame[i]
	}
	frame[checkCodeIndex] = ^sum + 1

	return frame, nil
}

func ReadPacket(reader io.Reader) (*Packet, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(reader, lenBuf[:]); err != nil {
		return nil, err
	}

	payloadLen := binary.LittleEndian.Uint16(lenBuf[:])
	if payloadLen < 6 {
		return nil, errors.New("payload too short")
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}

	var sum byte
	for _, value := range payload {
		sum += value
	}
	if sum != 0 {
		return nil, errors.New("checkCode verify failed")
	}

	index := 0
	index++

	msgType := payload[index]
	index++

	if len(payload) < index+4 {
		return nil, errors.New("missing mainCmd/subCmd")
	}

	mainCmd := binary.LittleEndian.Uint16(payload[index : index+2])
	index += 2

	subCmd := binary.LittleEndian.Uint16(payload[index : index+2])
	index += 2

	var rpcCallID uint32
	if msgType&MsgTypeRPC != 0 {
		if len(payload) < index+4 {
			return nil, errors.New("missing rpcCallId")
		}
		rpcCallID = binary.LittleEndian.Uint32(payload[index : index+4])
		index += 4
	}

	body := payload[index:]
	return &Packet{
		MsgType:   msgType,
		MainCmd:   mainCmd,
		SubCmd:    subCmd,
		RpcCallID: rpcCallID,
		Body:      body,
	}, nil
}

func ReadPacketBytes(data []byte) (*Packet, error) {
	return ReadPacket(bytes.NewReader(data))
}

func UnmarshalPacketBody(body []byte, msg proto.Message) error {
	if msg == nil {
		return errors.New("nil proto message")
	}
	if len(body) == 0 {
		return nil
	}
	return proto.Unmarshal(body, msg)
}

func DumpPacket(packet *Packet) string {
	if packet == nil {
		return "<nil>"
	}
	return fmt.Sprintf("msgType=%d mainCmd=%d subCmd=%d rpcCallID=%d bodyLen=%d",
		packet.MsgType, packet.MainCmd, packet.SubCmd, packet.RpcCallID, len(packet.Body))
}
