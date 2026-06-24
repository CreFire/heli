package shop

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
	r.CSRegister(pb.MSG_ID_SHOP_INFO_REQ, actor.WrapC2S(h.reqShopInfo))
	r.CSRegister(pb.MSG_ID_SHOP_BUY_REQ, actor.WrapC2S(h.reqShopBuy))
	return nil
}

func (h *Handler) reqShopInfo(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.ShopInfoREQ)
	if !ok || req == nil || ctx == nil || ctx.Shop() == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	return errorpb.ERROR_SUCCESS, ctx.Shop().BuildShopInfoRsp(req)
}

func (h *Handler) reqShopBuy(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
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
