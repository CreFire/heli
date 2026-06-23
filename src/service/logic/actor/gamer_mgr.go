package actor

import (
	"game/deps/xlog"
	"game/deps/xsync"
)

var GamerMgr = NewGamerManager()

type GamerManager struct {
	gamerBySessId *xsync.Map[int64, *Gamer]
	gamerByGid    *xsync.Map[int64, *Gamer]
}

func (mgr *GamerManager) AddGamer(sessId, gid int64, gamer *Gamer) {
	gamer.mgr = mgr
	mgr.gamerBySessId.Set(sessId, gamer)
	mgr.gamerByGid.Set(gid, gamer)
	gamer.Start()
}

func (mgr *GamerManager) DelGamer(sessId, gid int64, reason string) {
	cb := func(gamer *Gamer) bool {
		gamer.Stop(reason)
		return true
	}
	mgr.gamerBySessId.Delete(sessId, cb)
	mgr.gamerByGid.Delete(gid, nil)
}

func (mgr *GamerManager) ReplaceGamerSessId(oldSessId, newSessId int64, gamer *Gamer) {
	mgr.gamerBySessId.Set(newSessId, gamer)
	mgr.gamerBySessId.Delete(oldSessId, nil)
}

func (mgr *GamerManager) GetGamerBySessId(sessId int64) *Gamer {
	gamer, _ := mgr.gamerBySessId.Get(sessId)
	return gamer
}

func (mgr *GamerManager) GetGamerByGid(gid int64) *Gamer {
	gamer, _ := mgr.gamerByGid.Get(gid)
	return gamer
}

func (mgr *GamerManager) Foreach(f func(int64, *Gamer) bool) {
	mgr.gamerBySessId.Range(f)
}

func (mgr *GamerManager) DrainOfflineForGray(reason string) int {
	type gamerTarget struct {
		sessId int64
		gid    int64
	}

	targets := make([]gamerTarget, 0)
	mgr.Foreach(func(sessId int64, gamer *Gamer) bool {
		if gamer.IsOnline() {
			return true
		}
		targets = append(targets, gamerTarget{sessId: sessId, gid: gamer.GamerId})
		return true
	})

	for _, target := range targets {
		mgr.DelGamer(target.sessId, target.gid, reason)
	}
	if len(targets) > 0 {
		xlog.Infof("drain offline gamers for gray. count:%d reason:%s", len(targets), reason)
	}
	return len(targets)
}

func (mgr *GamerManager) Stop() {
	xlog.Infof("gamer mgr stop... gamer count:%d", mgr.gamerBySessId.Len())

	gids := make([]int64, 0, 1024*8)
	mgr.Foreach(func(sessId int64, gamer *Gamer) bool {
		gids = append(gids, gamer.GetGamerId())
		if !gamer.IsOnline() {
			return true
		}
		//todo 修改
		// playerSessId, gateSessId := gamer.GetSessIds()
		// req := msg.NewMsgWithCode(pb.MSG_ID_S2C_LOGIN_KICK_SESSION, errorpb.ERROR_KICK_SERVER_FIX, nil).SetUserInfo(playerSessId, gamer.GamerId)
		// server.MS.NetMgr.SendMsg2Sess(gateSessId, req, nil)

		gamer.Offline("server stop", false)
		return true
	})

	mgr.Foreach(func(sessId int64, gamer *Gamer) bool {
		gamer.Stop("server stop")
		return true
	})
}

func (mgr *GamerManager) GamerCount() int {
	return mgr.gamerBySessId.Len()
}

func (mgr *GamerManager) GamerOnlineCount() int {
	count := 0
	mgr.Foreach(func(sessId int64, gamer *Gamer) bool {
		if gamer.IsOnline() {
			count++
		}
		return true
	})
	return count
}

func NewGamerManager() *GamerManager {
	return &GamerManager{
		gamerBySessId: xsync.NewMap[int64, *Gamer](),
		gamerByGid:    xsync.NewMap[int64, *Gamer](),
	}
}
