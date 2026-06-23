package controller

import (
	"context"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/src/common"
	"game/src/proto/eventpb"
	"sync/atomic"
	"time"

	"github.com/sasha-s/go-deadlock"
	"golang.org/x/time/rate"
)

// 定义一个全局的限速器map，用于存储每个IP的限速器
var (
	ipLimiters      = make(map[string]*rate.Limiter)
	limitersMutex   deadlock.RWMutex
	gateServerCount = int32(1)
	loginLimiter    *rate.Limiter
)

var whiteListSwitch = false // 是否开启白名单

func Start() {
	LoginQueue.Start()
	UpdateLoginLimiter()

	server.MS.EventBus.Subscribe(eventpb.EVENT_TYPE_ERVER_WHITE_LIST_SWITCH, nil, false, func(ctx context.Context, e *eventpb.Event, a any) error {
		s := e.GetServerWhiteListSwitch()
		if s != nil {
			old := whiteListSwitch
			whiteListSwitch = s.Enable
			if old != whiteListSwitch {
				xlog.Infof("white list switch changed. old:%v new:%v", old, whiteListSwitch)
			}
		}

		e.Result = &eventpb.Event_ServerWhiteListSwitchResult{
			ServerWhiteListSwitchResult: &eventpb.ServerWhiteListSwitchEventResult{
				Enable: whiteListSwitch,
			},
		}

		return nil
	})
}

func Stop() {
	LoginQueue.Stop()
}

func GetLoginLimiter() *rate.Limiter {
	return loginLimiter
}

//go:norace
func UpdateLoginLimiter() {
	limit := min(server.MS.ConfBase.Server.Auth.RatePerGate*gateServerCount, server.MS.ConfBase.Server.Auth.RateMax)
	limit = max(limit, 200)
	xlog.Infof("login rate limit %d", limit)
	ins := server.MS.SvrMgr.List(common.InnerServerTypeAuth, func(si *servicemgr.ServiceInstance) bool {
		return si.Enable && si.Healthy == servicemgr.ServiceStatusHealth
	})
	authCountPerAuth := int(limit) / max(len(ins), 1)
	if authCountPerAuth <= 0 {
		xlog.Warnf("login rate calc authCountPerAuth=0, limit:%d authInstances:%d", limit, len(ins))
		authCountPerAuth = 1
	}
	interval := time.Second / time.Duration(authCountPerAuth)
	loginLimiter = rate.NewLimiter(rate.Every(interval), authCountPerAuth)
}

//go:norace
func UpdateGateServerCount() {
	ins := server.MS.SvrMgr.List(common.InnerServerTypeGate, func(si *servicemgr.ServiceInstance) bool {
		return si.Enable && si.Healthy == servicemgr.ServiceStatusHealth
	})
	count := int32(len(ins))
	atomic.StoreInt32(&gateServerCount, count)
}

// 获取或创建一个IP对应的限速器
func getIPLimiter(ip string) *rate.Limiter {
	limitersMutex.RLock()
	limiter, exists := ipLimiters[ip]
	limitersMutex.RUnlock()

	if exists {
		return limiter
	}

	limitersMutex.Lock()
	// 双重检查，防止重复创建
	limiter, exists = ipLimiters[ip]
	if !exists {
		limit := server.MS.ConfBase.Server.Auth.RatePerIp
		if limit <= 0 {
			xlog.Warnf("RatePerIp invalid: %d, fallback to 1", limit)
			limit = 1
		}
		interval := 5 * time.Second / time.Duration(limit)
		limiter = rate.NewLimiter(rate.Every(interval), int(limit))
		ipLimiters[ip] = limiter
	}
	limitersMutex.Unlock()

	return limiter
}
