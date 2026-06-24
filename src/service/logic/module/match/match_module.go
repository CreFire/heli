package match

import (
	"game/src/service/logic/gamedata"
	"game/src/service/logic/iface"
	matchbiz "game/src/service/logic/module/matchbiz"
)

type CreateRoomRequest = matchbiz.CreateRoomRequest
type CreateRoomResult = matchbiz.CreateRoomResult
type CreateRoomFunc = matchbiz.CreateRoomFunc
type MatchModule = matchbiz.MatchModule

func NewMatchModule(ctx iface.IGamerContext, model *gamedata.GamerModel) *MatchModule {
	return matchbiz.NewMatchModule(ctx, model)
}
