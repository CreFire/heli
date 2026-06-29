package shop

import (
	"game/deps/msg"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/iface"

	"google.golang.org/protobuf/proto"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) HandleShopInfo(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.ShopInfoREQ)
	if !ok || req == nil || ctx == nil || ctx.Shop() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return errorpb.ERROR_SUCCESS, ctx.Shop().BuildShopInfoRsp(req)
}

func (h *Handler) HandleShopBuy(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.ShopBuyREQ)
	if !ok || req == nil || ctx == nil || ctx.Shop() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	items, code := ctx.Shop().BuyGoods(req.GetShopTabId(), req.GetGoodsId(), req.GetGoodsNum())
	if code != errorpb.ERROR_SUCCESS {
		return code, nil
	}
	return errorpb.ERROR_SUCCESS, &pb.ShopBuyRSP{ShopTabId: req.GetShopTabId(), GoodsId: req.GetGoodsId(), Items: items, BuyCount: map[int32]int32{req.GetGoodsId(): ctx.Shop().GetBuyCount(req.GetShopTabId(), req.GetGoodsId())}}
}
