package task

import (
	"game/deps/msg"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/iface"

	"google.golang.org/protobuf/proto"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) HandleTaskReward(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.TaskRewardREQ)
	if !ok || req == nil || ctx == nil || ctx.Task() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return ctx.Task().Reward(req)
}

func (h *Handler) HandleTaskRefresh(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.TaskRefreshREQ)
	if !ok || req == nil || ctx == nil || ctx.Task() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return ctx.Task().Refresh(req)
}
