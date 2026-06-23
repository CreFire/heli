package controller

import (
	"game/deps/xsync"
)

type RobotMgr struct {
	gamerBySessId *xsync.Map[int64, *Robot]
	gamerByGid    *xsync.Map[int64, *Robot]
}

func (mgr *RobotMgr) addRobot(sessId, gid int64, robot *Robot) {
	mgr.gamerBySessId.Set(sessId, robot)
	mgr.gamerByGid.Set(gid, robot)
}

func (mgr *RobotMgr) delGamer(sessId, gid int64, reason string) {
	cb := func(gamer *Robot) bool {
		return true
	}
	mgr.gamerBySessId.Delete(sessId, cb)
	mgr.gamerByGid.Delete(gid, nil)
}

func (mgr *RobotMgr) getGamerBySessId(sessId int64) *Robot {
	robot, _ := mgr.gamerBySessId.Get(sessId)
	return robot
}

func (mgr *RobotMgr) getGamerByGid(gid int64) *Robot {
	robot, _ := mgr.gamerByGid.Get(gid)
	return robot
}

func (mgr *RobotMgr) foreach(f func(int64, *Robot) bool) {
	mgr.gamerBySessId.Range(f)
}

func (mgr *RobotMgr) GamerCount() int {
	return mgr.gamerBySessId.Len()
}

func NewRobotMgr() *RobotMgr {
	return &RobotMgr{
		gamerBySessId: xsync.NewMap[int64, *Robot](),
		gamerByGid:    xsync.NewMap[int64, *Robot](),
	}
}
