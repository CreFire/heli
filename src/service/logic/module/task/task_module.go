package task

import (
	"game/src/persist"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/gamedata/base"
)

func init() {
	gamedata.RegisterMod(persist.GamerTaskModIndex, func(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
		return NewGamerTask(modIndex, data, doc, docExt)
	})
}

func GetTaskModel(m *gamedata.GamerModel) *GamerTask {
	return gamedata.GetGamerModel[*GamerTask](m, persist.GamerTaskModIndex)
}
