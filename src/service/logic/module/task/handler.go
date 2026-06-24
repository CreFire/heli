package task

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
	r.CSRegister(pb.MSG_ID_TASK_REWARD_REQ, actor.WrapC2S(h.reqTaskReward))
	r.CSRegister(pb.MSG_ID_TASK_REFRESH_REQ, actor.WrapC2S(h.reqTaskRefresh))
	return nil
}

func (h *Handler) reqTaskReward(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.TaskRewardREQ)
	if !ok || req == nil || ctx == nil || ctx.Task() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return ctx.Task().Reward(req)
}

func (h *Handler) reqTaskRefresh(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.TaskRefreshREQ)
	if !ok || req == nil || ctx == nil || ctx.Task() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return ctx.Task().Refresh(req)
}
