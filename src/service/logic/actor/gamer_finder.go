package actor

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/router"
	"game/deps/xlog"
	"game/src/proto/errorpb"
	"game/src/service/logic/iface"

	"google.golang.org/protobuf/proto"
)

var GamerFinderWithGid GamerFinder
var GamerFinderWithSess GamerFinder

type GamerFinder func(Gid int64) iface.IGamerContext

func SetGidGamerFinder(r GamerFinder) {
	GamerFinderWithGid = r
}

func SetSessGamerFinder(r GamerFinder) {
	GamerFinderWithSess = r
}

func FindGamerWithSess(sessId int64) iface.IGamerContext {
	return GamerFinderWithSess(sessId)
}

func FindGamerWithGid(Gid int64) iface.IGamerContext {
	return GamerFinderWithGid(Gid)
}
func WrapC2S(h func(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message)) router.C2SHandlerFunc {
	return func(_ netmgr.IMsgQue, data *msg.Message) (errorpb.ERROR, proto.Message) {
		if data == nil {
			return errorpb.ERROR_REQUEST_PARAMS, nil
		}
		ctx := FindGamerWithGid(data.GId())
		if ctx == nil {
			xlog.Warnf("handler ctx missing msgId:%v gid:%v sess:%v", data.MsgId(), data.GId(), data.PlayerSessId())
			return errorpb.ERROR_LOGIN_SESSION_INVALID, nil
		}
		return h(ctx, data)
	}
}
