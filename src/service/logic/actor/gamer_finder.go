package actor

import (
	"game/src/service/logic/iface"
)

var GamerFinderWithGid GamerFinder
var GamerFinderWithSess GamerFinder

type GamerFinder func(Gid int64) iface.IGamerContext

func SetGidGamerFinder(r GamerFinder) {
	GamerFinderWithGid = r
}

func SetSessGamerFinder(r GamerFinder) {
	GamerFinderWithSess = r
}

func FindGamerWithSess(sessId int64) iface.IGamerContext {
	return GamerFinderWithSess(sessId)
}

func FindGamerWithGid(Gid int64) iface.IGamerContext {
	return GamerFinderWithGid(Gid)
}
