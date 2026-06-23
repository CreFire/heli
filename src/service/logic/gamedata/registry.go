package gamedata

import (
	"game/src/persist"
	"game/src/service/logic/gamedata/base"
)

type ModFactory func(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase

var modFactories = map[int]ModFactory{}

func RegisterMod(modIndex int, factory ModFactory) {
	if factory == nil {
		return
	}
	modFactories[modIndex] = factory
}
