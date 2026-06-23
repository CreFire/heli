package control

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/src/proto/pb"
)

type Func func(gamer *pb.Gamer, msgque netmgr.IMsgQue, msg *msg.Message) bool

func SetControlFunc(cmd pb.CMD, act pb.ACT, c2s, s2c any, fun Func, coolTime int, unlockId int32) {
}
