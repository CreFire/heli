package controller

import (
	"game/deps/basal"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/deps/xsync"
	"game/deps/xtime"
	"game/src/cache"
	"game/src/common"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var LoginQueue = NewAuthQueue()

type UserLoginInfo struct {
	GamerId        int64 //玩家ID
	StartQueueTime int64 //开始排队时间
	LastCheckTime  *basal.NoCacheLineData[int64]
}

type AuthQUeue struct {
	CurEnterTime  *basal.NoCacheLineData[int64]
	CurQueueCount *basal.NoCacheLineData[int]
	UserLoginMap  *xsync.Map[int64, *UserLoginInfo]
	quit          chan struct{}
	wg            *sync.WaitGroup
}

func NewAuthQueue() *AuthQUeue {
	return &AuthQUeue{
		UserLoginMap:  xsync.NewMap[int64, *UserLoginInfo](),
		quit:          make(chan struct{}),
		wg:            &sync.WaitGroup{},
		CurEnterTime:  basal.NewNoCacheLineData(xtime.NowUnixMs()),
		CurQueueCount: basal.NewNoCacheLineData(0),
	}
}

func (q *AuthQUeue) LocalQueueSize() int {
	return q.UserLoginMap.Len()
}

func (q *AuthQUeue) GetUserLoginInfo(id int64) *UserLoginInfo {
	return q.UserLoginMap.GetOrNew(id, func() *UserLoginInfo {
		ts := xtime.NowUnixMs()
		if _, err := cache.AddUserToLoginQueue(id, ts); err != nil {
			xlog.Warnf("add login queue failed. gid:%d startQueueTime:%d localQueueSize:%d err:%v", id, ts, q.LocalQueueSize(), err)
		}
		return &UserLoginInfo{
			GamerId:        id,
			StartQueueTime: ts,
			LastCheckTime:  basal.NewNoCacheLineData(ts),
		}
	})
}
func (q *AuthQUeue) DelUserLoginInfo(gid int64) error {
	q.UserLoginMap.Delete(gid, nil)
	if err := cache.RemoveUserFromLoginQueue([]int64{gid}); err != nil {
		xlog.Warnf("remove login queue failed. gid:%d localQueueSize:%d err:%v", gid, q.LocalQueueSize(), err)
		return err
	}
	return nil
}

func (q *AuthQUeue) GetUserPosition(gid int64) int64 {
	n, err := cache.GetUserLoginQueueRank(gid)
	if err != nil && err != redis.Nil {
		xlog.Errorf("get user login queue rank failed. gid:%d err:%v", gid, err)
		return -1
	}
	return n
}

func (q *AuthQUeue) CheckUserCanEnter(gid int64) bool {
	if q.UserLoginMap.Len() > 0 {
		li := q.GetUserLoginInfo(gid)
		if li.StartQueueTime <= q.CurEnterTime.Get() {
			q.DelUserLoginInfo(gid)
			return true
		}
		li.LastCheckTime.UpdateInt(xtime.NowUnixMs())
		xlog.Debugf("enter login queue %d  start wait %d  cur %d", gid, li.StartQueueTime, q.CurEnterTime.Get())
		return false
	}

	min, err := server.MS.SvrMgr.PickMinOnline(common.InnerServerTypeGate, false)
	if min == nil {
		xlog.Warnf("no gate server valid. gid:%d localQueueSize:%d err:%v", gid, q.LocalQueueSize(), err)
		return false
	}

	if min.OnlineCount() >= server.MS.ConfBase.Server.Auth.GateQueueSize {
		li := q.GetUserLoginInfo(gid)
		li.LastCheckTime.UpdateInt(xtime.NowUnixMs())
		xlog.Infof("gate over cap, enter login queue gid:%d start:%d cur:%d", gid, li.StartQueueTime, q.CurEnterTime.Get())
		return false
	}

	return true
}

func (q *AuthQUeue) Tick() {
	removeUser := make([]int64, 0, 16)

	q.UserLoginMap.RangeDelete(0, func(key int64, value *UserLoginInfo) bool {
		if xtime.NowUnixMs()-value.LastCheckTime.Get() > 30*1000 {
			removeUser = append(removeUser, key)
			return true
		}
		return false
	})

	if len(removeUser) > 0 {
		if err := cache.RemoveUserFromLoginQueue(removeUser); err != nil {
			xlog.Warnf("batch remove login queue failed. removeCount:%d curEnterTime:%d err:%v",
				len(removeUser), q.CurEnterTime.Get(), err)
		}
	}

	ins := server.MS.SvrMgr.List(common.InnerServerTypeGate, func(si *servicemgr.ServiceInstance) bool {
		return si.Healthy == servicemgr.ServiceStatusHealth
	})

	validCount := int32(0)
	for _, ins := range ins {
		if ins.OnlineCount() < server.MS.ConfBase.Server.Auth.GateQueueSize {
			validCount += server.MS.ConfBase.Server.Auth.GateQueueSize - ins.OnlineCount()
		}
	}
	if validCount > 0 {
		if curTime, err := cache.GetTopNTimeFromLoginQueue(int(validCount)); err == nil {
			q.CurEnterTime.UpdateInt(curTime)
		} else {
			xlog.Infof("get login queue data failed, %s", err.Error())
			return
		}
	}
	ts := q.CurEnterTime.Get() - 30*1000
	if err := cache.RemoveUserLessTimeFromLoginQueue(ts); err != nil {
		xlog.Errorf("remove user less time from login queue failed. ts:%d err:%v", ts, err)
	}

	curSize, err := cache.GetCurQUeueSize()
	if err != nil {
		xlog.Errorf("get current queue size failed. err:%v", err)
	}
	q.CurQueueCount.UpdateInt(int(curSize))
}

func (q *AuthQUeue) Start() {

	basal.SafeGo(func() {
		ticker := time.NewTicker(time.Second * 1)
		defer ticker.Stop()
		q.wg.Add(1)
		defer q.wg.Done()
		for {
			select {
			case <-ticker.C:
				basal.SafeRun(func() {
					q.Tick()
				})
			case <-q.quit:
				removeUser := make([]int64, 0, q.LocalQueueSize())
				q.UserLoginMap.RangeDelete(0, func(key int64, value *UserLoginInfo) bool {
					removeUser = append(removeUser, key)
					return true
				})
				if len(removeUser) > 0 {
					if err := cache.RemoveUserFromLoginQueue(removeUser); err != nil {
						xlog.Warnf("stop remove login queue failed. removeCount:%d localQueueSize:%d err:%v",
							len(removeUser), q.LocalQueueSize(), err)
					}
				}

				return
			}
		}
	})
}

func (q *AuthQUeue) Stop() {
	close(q.quit)
	q.wg.Wait()
}
