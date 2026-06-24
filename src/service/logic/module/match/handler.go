package match

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
	r.CSRegister(pb.MSG_ID_MATCH_JOIN_REQ, actor.WrapC2S(h.reqMatchJoin))
	return nil
}

func (h *Handler) reqMatchJoin(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.C2SMatchJoin)
	if !ok || req == nil || ctx == nil || ctx.Match() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return ctx.Match().Join(ctx.GetGamerId(), req)
}
