package shop

import (
	"game/src/persist"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/gamedata/base"
)

func init() {
	gamedata.RegisterMod(persist.GamerShopModIndex, func(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
		return NewGamerShop(modIndex, data, doc, docExt)
	})
}

func GetShopModel(m *gamedata.GamerModel) *GamerShop {
	return gamedata.GetGamerModel[*GamerShop](m, persist.GamerShopModIndex)
}
