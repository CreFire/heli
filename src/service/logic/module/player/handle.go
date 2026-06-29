package player

import (
	"game/deps/msg"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/iface"

	"google.golang.org/protobuf/proto"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) HandlePlayerLoadUser(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	if ctx == nil || ctx.Player() == nil {
		return errorpb.ERROR_FAILED, nil
	}
	return errorpb.ERROR_SUCCESS, &pb.PlayerInfoNTF{Base: ctx.Player().GetPlayerBase()}
}
