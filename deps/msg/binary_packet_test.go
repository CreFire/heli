package msg

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestPackMessageReadPacketRoundTrip(t *testing.T) {
	body := &TestPacketBody{
		Name:  "heli",
		Value: 7,
	}

	frame, err := PackMessage(MsgTypeRPC, 101, 3001, 42, body)
	require.NoError(t, err)

	packet, err := ReadPacket(bytes.NewReader(frame))
	require.NoError(t, err)
	require.Equal(t, MsgTypeRPC, packet.MsgType)
	require.Equal(t, uint16(101), packet.MainCmd)
	require.Equal(t, uint16(3001), packet.SubCmd)
	require.Equal(t, uint32(42), packet.RpcCallID)
	require.NotEmpty(t, packet.Body)

	decoded := &TestPacketBody{}
	require.NoError(t, UnmarshalPacketBody(packet.Body, decoded))
	require.True(t, proto.Equal(body, decoded))
}

func TestPackMessageReadPacketWithoutRPC(t *testing.T) {
	frame, err := PackMessage(MsgTypeNone, 9, 10, 999, nil)
	require.NoError(t, err)

	packet, err := ReadPacketBytes(frame)
	require.NoError(t, err)
	require.Equal(t, MsgTypeNone, packet.MsgType)
	require.Equal(t, uint16(9), packet.MainCmd)
	require.Equal(t, uint16(10), packet.SubCmd)
	require.Equal(t, uint32(0), packet.RpcCallID)
	require.Empty(t, packet.Body)
}

func TestReadPacketRejectsShortPayload(t *testing.T) {
	_, err := ReadPacketBytes([]byte{5, 0, 0, 0, 0, 0, 0})
	require.ErrorContains(t, err, "payload too short")
}

func TestReadPacketRejectsBadCheckCode(t *testing.T) {
	frame, err := PackMessage(MsgTypeNone, 101, 3001, 0, nil)
	require.NoError(t, err)
	frame[len(frame)-1]++

	_, err = ReadPacketBytes(frame)
	require.ErrorContains(t, err, "checkCode verify failed")
}

func TestUnmarshalPacketBodyNilMessage(t *testing.T) {
	err := UnmarshalPacketBody([]byte{1, 2, 3}, nil)
	require.ErrorContains(t, err, "nil proto message")
}

func TestUnmarshalPacketBodyEmptyBody(t *testing.T) {
	decoded := &TestPacketBody{}
	require.NoError(t, UnmarshalPacketBody(nil, decoded))
	require.Equal(t, "", decoded.Name)
	require.Equal(t, int32(0), decoded.Value)
}

func TestPackMessageRejectsOversizedPayload(t *testing.T) {
	body := &TestPacketBody{
		Name: strings.Repeat("a", 65535),
	}

	_, err := PackMessage(MsgTypeNone, 1, 2, 0, body)
	require.ErrorContains(t, err, "payload too large")
}

func TestDumpPacketNil(t *testing.T) {
	require.Equal(t, "<nil>", DumpPacket(nil))
}
