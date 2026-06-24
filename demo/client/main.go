package main

import (
	"fmt"
	"game/demo/pb"
	"game/deps/msg"

	"github.com/gorilla/websocket"
)

const serverURL = "ws://127.0.0.1:9000/ws"

func main() {
	if err := startClient(serverURL); err != nil {
		fmt.Println("client error:", err)
	}
}

func startClient(url string) error {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("client connected to:", url)
	data := &pb.BaseReq{
		Gid:     122,
		Session: "12345",
	}
	reqFrame, err := msg.PackMessage(
		msg.MsgTypeNone,
		101,
		3001,
		0,
		data,
	)
	if err != nil {
		return err
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, reqFrame); err != nil {
		return err
	}
	fmt.Println("client send ok")

	msgType, respFrame, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	if msgType != websocket.BinaryMessage {
		return fmt.Errorf("unexpected websocket message type: %d", msgType)
	}

	packet, err := msg.ReadPacketBytes(respFrame)
	if err != nil {
		return err
	}

	fmt.Println("client recv packet:", msg.DumpPacket(packet))

	resp := &pb.BaseRsp{}
	if err := msg.UnmarshalPacketBody(packet.Body, resp); err != nil {
		return err
	}

	fmt.Println("client recv pb body:", resp.Msg)
	return nil
}
