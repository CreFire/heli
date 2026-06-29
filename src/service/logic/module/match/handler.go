package match

import (
	"game/deps/msg"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/iface"

	"google.golang.org/protobuf/proto"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) HandleMatchJoin(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.C2SMatchJoin)
	if !ok || req == nil || ctx == nil || ctx.Match() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return ctx.Match().Join(ctx.GetGamerId(), req)
}
