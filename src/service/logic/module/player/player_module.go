package player

import (
	"game/src/persist"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/gamedata/base"
)

func init() {
	gamedata.RegisterMod(persist.GamerMainModIndex, func(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
		return NewGamerMain(modIndex, data, doc, docExt)
	})
	gamedata.RegisterMod(persist.GamerBaseModIndex, func(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
		return NewGamerPlayerData(modIndex, data, doc, docExt)
	})
}

// gamer 模块
func GetMainModel(m *gamedata.GamerModel) *GamerMain {
	return gamedata.GetGamerModel[*GamerMain](m, persist.GamerMainModIndex)
}

// Base模块
func GetPlayerModel(m *gamedata.GamerModel) *GamerPlayerData {
	return gamedata.GetGamerModel[*GamerPlayerData](m, persist.GamerBaseModIndex)
}
