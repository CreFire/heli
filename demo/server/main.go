package main

import (
	"fmt"
	"game/demo/pb"
	"game/deps/msg"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	if err := startServer(":9000"); err != nil {
		fmt.Println("server error:", err)
	}
}

func startServer(addr string) error {
	http.HandleFunc("/ws", handleWS)
	fmt.Println("websocket server listen:", addr, "path: /ws")
	return http.ListenAndServe(addr, nil)
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("upgrade websocket error:", err)
		return
	}
	defer conn.Close()

	fmt.Println("client connected:", conn.RemoteAddr())

	for {
		msgType, frame, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				fmt.Println("client closed:", conn.RemoteAddr())
			} else {
				fmt.Println("read websocket message error:", err)
			}
			return
		}
		if msgType != websocket.BinaryMessage {
			fmt.Println("unexpected websocket message type:", msgType)
			return
		}

		packet, err := msg.ReadPacketBytes(frame)
		if err != nil {
			fmt.Println("read packet error:", err)
			return
		}

		fmt.Println("recv packet:", msg.DumpPacket(packet))

		req := &pb.BaseReq{}
		if err := msg.UnmarshalPacketBody(packet.Body, req); err != nil {
			fmt.Println("unmarshal body error:", err)
			return
		}

		fmt.Println("recv pb body:", req.Gid, req.Session)
		message := fmt.Sprintf("%v+%v+%v", req.Gid, req.Session, time.Now())
		rsp := &pb.BaseRsp{
			Msg: message,
		}
		respFrame, err := msg.PackMessage(
			msg.MsgTypeNone,
			200,
			2,
			0,
			rsp,
		)
		if err != nil {
			fmt.Println("pack response error:", err)
			return
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, respFrame); err != nil {
			fmt.Println("write websocket response error:", err)
			return
		}
	}
}
