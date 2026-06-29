package shop

import (
	"game/src/common"
	"game/src/configdoc"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/iface"
)

type ShopModule struct {
	ctx   iface.IGamerContext
	model *gamedata.GamerModel
	store *GamerShop
}

func NewShopModule(ctx iface.IGamerContext, model *gamedata.GamerModel) *ShopModule {
	return &ShopModule{ctx: ctx, model: model, store: GetShopModel(model)}
}

func (m *ShopModule) SendShopInfo(shopId int32) {
	m.ctx.SendMsg(m.BuildShopInfoRsp(&pb.ShopInfoREQ{ShopTabId: []int32{shopId}}))
}

func (m *ShopModule) BuildShopInfoRsp(req *pb.ShopInfoREQ) *pb.ShopInfoRSP {
	rsp := &pb.ShopInfoRSP{}
	if req == nil {
		return rsp
	}
	if req.GetAll() || len(req.GetShopTabId()) == 0 {
		for tabId := range m.store.EnsureShopTabs() {
			if info := m.buildShopInfo(tabId); info != nil {
				rsp.ShopInfoList = append(rsp.ShopInfoList, info)
			}
		}
		if len(rsp.ShopInfoList) == 0 {
			if info := m.buildShopInfo(1); info != nil {
				rsp.ShopInfoList = append(rsp.ShopInfoList, info)
			}
		}
		return rsp
	}
	for _, shopId := range req.GetShopTabId() {
		if info := m.buildShopInfo(shopId); info != nil {
			rsp.ShopInfoList = append(rsp.ShopInfoList, info)
		}
	}
	return rsp
}

func (m *ShopModule) BuyGoods(shopId, goodsId int32, num int32) ([]*pb.Item, errorpb.ERROR) {
	if shopId <= 0 || goodsId <= 0 || num <= 0 {
		return nil, errorpb.ERROR_REQUEST_PARAMS
	}
	// conf := m.ctx.Doc().Tb
	// if m.ctx.Item() == nil {
	// 	return nil, errorpb.ERROR_FAILED
	// }
	goldConfID := int32(configdoc.COIN_GOLD)
	cost := []*pb.Item{{ConfId: goldConfID, Num: int64(1) * int64(num)}}
	if _, code := m.ctx.Item().SubItems(cost, common.BuildReason(common.ReasonShop.Id, "", "shop buy")); code != errorpb.ERROR_SUCCESS {
		return nil, code
	}
	rewards := []*pb.Item{{ConfId: goldConfID, Num: int64(1+2) * int64(num)}}
	items, code := m.ctx.Item().AddItems(rewards, common.BuildReason(common.ReasonShop.Id, "", "shop buy reward"))
	if code != errorpb.ERROR_SUCCESS {
		return nil, code
	}
	tab := m.ensureShopTab(shopId)
	if tab.BuyCount == nil {
		tab.BuyCount = make(map[int32]int32)
	}
	tab.BuyCount[goodsId] += num
	m.store.AddUpdateOp("shop_tabs", m.store.Data().ShopTabs)
	return items, errorpb.ERROR_SUCCESS
}

func (m *ShopModule) RefreshShop(shopId int32) errorpb.ERROR {
	if shopId <= 0 {
		return errorpb.ERROR_REQUEST_PARAMS
	}
	tab := m.ensureShopTab(shopId)
	tab.RefreshTime = m.ctx.Now()
	m.store.AddUpdateOp("shop_tabs", m.store.Data().ShopTabs)
	return errorpb.ERROR_SUCCESS
}

func (m *ShopModule) GetBuyCount(shopId, goodsId int32) int32 {
	tab := m.store.EnsureShopTabs()[shopId]
	if tab == nil || tab.BuyCount == nil {
		return 0
	}
	return tab.BuyCount[goodsId]
}

func (m *ShopModule) ensureShopTab(shopId int32) *pb.ShopTabData {
	tabs := m.store.EnsureShopTabs()
	if shopId <= 0 {
		shopId = 1
	}
	if tabs[shopId] == nil {
		tabs[shopId] = &pb.ShopTabData{BuyCount: make(map[int32]int32), RefreshTime: m.ctx.Now()}
		m.store.AddUpdateOp("shop_tabs", m.store.Data().ShopTabs)
	}
	return tabs[shopId]
}

func (m *ShopModule) buildShopInfo(shopId int32) *pb.ShopInfo {
	tab := m.ensureShopTab(shopId)
	return &pb.ShopInfo{ShopTabId: shopId, BuyCount: tab.BuyCount}
}
