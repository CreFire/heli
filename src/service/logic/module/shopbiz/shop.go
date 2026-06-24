package shopbiz

import (
	"game/deps/mongoclient"
	"game/src/persist"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata/base"
)

type GamerShop struct {
	*base.GamerModBase
	*mongoclient.DataPersister[*pb.GamerShopData]
}

func NewGamerShop(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) *GamerShop {
	return &GamerShop{
		GamerModBase:  base.NewGamerModBase(modIndex, doc, docExt),
		DataPersister: persist.GetGamerModData[*pb.GamerShopData](data, persist.GamerShopModIndex),
	}
}

func (m *GamerShop) EnsureShopTabs() map[int32]*pb.ShopTabData {
	if m.Data().ShopTabs == nil {
		m.Data().ShopTabs = make(map[int32]*pb.ShopTabData)
	}
	return m.Data().ShopTabs
}
