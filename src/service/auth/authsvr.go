package main

import (
	"fmt"
	"game/deps/misc"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xhttp"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/cache"
	"game/src/common"
	"game/src/configdoc"
	"game/src/persist"
	"game/src/proto/eventpb"
	"game/src/service/auth/controller"
	"time"
)

var authSvr = NewAuthSvr()

func NewAuthSvr() *AuthSvr {
	return &AuthSvr{}
}

type AuthSvr struct {
}

func (a *AuthSvr) OnInit() error {
	return RegisterHandlers()
}
func (a *AuthSvr) BeforeStart() error {
	return nil
}

func (a *AuthSvr) OnStart() error {
	persist.InitCollections()

	xlog.Infof("auth service start at Ip:%v port:%v", server.MS.ConfBase.GetSvrCfg().Ip, server.MS.ConfBase.GetSvrCfg().Port)
	err := server.MS.HttpServe.StartServe(fmt.Sprintf(":%d", server.MS.ConfBase.GetSvrCfg().Port))
	if err != nil {
		return err
	}
	xhttp.InitProxy(server.MS.SvrMgr)

	gateListenInfo := servicemgr.ListenSpec{
		Cluster:     server.MS.ConfBase.GetSvrCfg().Cluster,
		ServiceName: common.InnerServerTypeGate,
		Handler: servicemgr.HandlerFunc{
			OnlineFn: func(serviceName string, instance *servicemgr.ServiceInstance) error {
				controller.UpdateGateServerCount()
				controller.UpdateLoginLimiter()
				return nil
			},
		},
	}

	authListenInfo := servicemgr.ListenSpec{
		Cluster:     server.MS.ConfBase.GetSvrCfg().Cluster,
		ServiceName: common.InnerServerTypeAuth,
		Handler: servicemgr.HandlerFunc{
			OnlineFn: func(serviceName string, instance *servicemgr.ServiceInstance) error {
				controller.UpdateLoginLimiter()
				return nil
			},
		},
	}

	err = server.MS.SvrMgr.Watch(gateListenInfo, authListenInfo)
	if err != nil {
		return err
	}

	if _, err := server.MS.TimerMgr.AddSimpleTimer("report_server_info", 3, true, a.ReportServerInfo); err != nil {
		return err
	}
	if _, err := server.MS.TimerMgr.AddSimpleTimer("print_status", 20, true, a.printStatus); err != nil {
		return err
	}
	controller.Start()
	return nil
}

func (a *AuthSvr) BeforeStop() error {
	inst := server.MS.SvrMgr.SelfCopy()
	if err := cache.DelServerInfo(inst); err != nil {
		xlog.Warnf("auth before stop delete server info failed. service:%s instanceId:%d err:%v",
			inst.ServiceName, inst.InstanceId, err)
	}
	controller.Stop()
	return nil
}
func (a *AuthSvr) OnStop() error {
	return nil
}

func (a *AuthSvr) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	return nil
}

func (a *AuthSvr) OnHeart(now int64) error {

	return nil
}

func (a *AuthSvr) OnEventHandle(evt *eventpb.Event) {

}

func (a *AuthSvr) ReportServerInfo(name string, now int64, value any) {
	inst := server.MS.SvrMgr.SelfCopy()
	if err := cache.UpdateServerInfo(inst); err != nil {
		xlog.Warnf("auth report server info failed. service:%s instanceId:%d err:%v",
			inst.ServiceName, inst.InstanceId, err)
	}
	m, err := cache.GetServiceAllOnline(common.InnerServerTypeGate)
	if err != nil {
		xlog.Errorf("get gate server online info err:%v", err.Error())
		return
	}
	insts := make([]*servicemgr.ServiceInstance, 0, len(m))
	for k, v := range m {
		insts = append(insts, &servicemgr.ServiceInstance{
			InstanceId:   int32(k),
			ServiceName:  common.InnerServerTypeGate,
			OnlineCount_: int32(v),
		})
	}
	_ = server.MS.SvrMgr.UpdateLoads(insts)
}

func (a *AuthSvr) printStatus(name string, now int64, value any) {
	xlog.Infof("[run-status] buildTime=%v | progVer=%v | excelVer=%v | launchTime=%v | gmTime=%v",
		misc.BuildTime,
		misc.ProgVer,
		misc.ExcelVer,
		server.MS.LaunchTime.Format(time.DateTime),
		xtime.Now().Format(time.DateTime),
	)
}
