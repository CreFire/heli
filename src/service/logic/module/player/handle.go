package player

import (
	"game/deps/msg"
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/actor"
	"game/src/service/logic/iface"

	"google.golang.org/protobuf/proto"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) RegisterHandler(rpc *rpcmgr.RpcMgr, r *router.Router) error {
	r.CSRegister(pb.MSG_ID_PLAYER_LOAD_USER_REQ, actor.WrapC2S(h.reqPlayerLoadUser))
	return nil
}

func (h *Handler) reqPlayerLoadUser(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	if ctx == nil || ctx.Player() == nil {
		return errorpb.ERROR_FAILED, nil
	}
	return errorpb.ERROR_SUCCESS, &pb.PlayerInfoNTF{Base: ctx.Player().GetPlayerBase()}
}
