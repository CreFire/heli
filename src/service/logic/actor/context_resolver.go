package actor

import (
	"game/src/service/logic/iface"
)

func init() {
	SetSessGamerFinder(func(sessId int64) iface.IGamerContext {

		gamer := GamerMgr.GetGamerBySessId(sessId)
		if gamer == nil {
			gamer = GamerMgr.GetGamerBySessId(sessId)
		}
		return gamer
	})

	SetGidGamerFinder(func(gid int64) iface.IGamerContext {
		gamer := GamerMgr.GetGamerByGid(gid)
		return gamer
	})
}
