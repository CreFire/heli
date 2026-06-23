package main

import (
	"context"
	"fmt"
	"game/deps/basal"
	"game/deps/misc"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/netmgr/options"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xgrpc"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/cache"
	"game/src/common"
	"game/src/configdoc"
	"game/src/msghandler"
	"game/src/persist"
	"game/src/proto/errorpb"
	"game/src/proto/eventpb"
	"game/src/proto/pb"
	"game/src/service/logic/actor"
	"game/src/service/logic/module"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
)

var logicSvr = NewLogicSvr()

func NewLogicSvr() *LogicSvr {
	return &LogicSvr{
		gamerMgr:  actor.GamerMgr,
		monitor:   basal.NewMonitorMgr(),
		moduleMgr: module.NewModuleMgr(),
	}
}

type LogicSvr struct {
	gamerMgr  *actor.GamerManager
	monitor   *basal.MonitorMgr
	moduleMgr *module.ModuleMgr
	sh        *msghandler.ServerNetEventHandler
}

func (g *LogicSvr) OnInit() error {
	// if err := backend.Init(server.MS.BackendServe, common.InnerServerTypeLogic); err != nil {
	// 	return err
	// }
	actor.InitGamerLogger(xlog.DefaultLogger)
	return nil
}

func (g *LogicSvr) BeforeStart() error {
	server.MS.EventBus.Subscribe(eventpb.EVENT_TYPE_SERVER_GRAY, nil, false, func(ctx context.Context, e *eventpb.Event, a any) error {
		drained := g.gamerMgr.DrainOfflineForGray("gray switch")
		xlog.Infof("logic server gray drained offline gamers. count:%d", drained)
		g.gamerMgr.Foreach(func(_ int64, gamer *actor.Gamer) bool {
			if !gamer.IsOnline() {
				return true
			}
			// gamer.Post(func() {
			// 	if gamer.IsOnline() {
			// 		gamer.SendMsg(&pb.SwitchServerNTF{})
			// 	}
			// })
			return true
		})
		return nil
	})
	return nil
}

func (g *LogicSvr) OnStart() error {
	var err error
	if server.MS.ConfBase.Global.IsDebug {
		g.monitor.Start()
	}

	persist.InitCollections()

	if err := g.moduleMgr.OnStart(server.MS.Rpc, server.MS.Router); err != nil {
		return err
	}

	g.sh = msghandler.NewServerNetEventHandler(server.MS, g.ServerRouteHandler)

	prewarmBattleGrpcConn := func(serviceName string, instance *servicemgr.ServiceInstance) error {
		basal.SafeGo(func() {
			connTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := xgrpc.GrpcConnMgr.GetConn(connTimeout, fmt.Sprintf("%s:%d", instance.Host, instance.Port))
			if err != nil {
				xlog.Warnf("get grpc conn addr: %s:%d failed: %v", instance.Host, instance.Port, err)
			}
		})
		return nil
	}

	battleListen := servicemgr.ListenSpec{
		Cluster:     server.MS.ConfBase.Server.Cluster,
		ServiceName: common.InnerServerTypeBattle,
		Handler: servicemgr.HandlerFunc{
			OnlineFn: prewarmBattleGrpcConn,
		},
	}

	onlineFunc := func(serviceName string, instance *servicemgr.ServiceInstance) error {
		if serviceName == common.InnerServerTypeLogic {
			g.moduleMgr.OnLogicInstanceChange(instance, nil)
		}
		return nil
	}

	offlineFunc := func(serviceName string, instance *servicemgr.ServiceInstance) error {
		if serviceName == common.InnerServerTypeLogic {
			g.moduleMgr.OnLogicInstanceChange(nil, instance)
		}
		return nil
	}

	logicListen := servicemgr.ListenSpec{
		Cluster:     server.MS.ConfBase.GetSvrCfg().Cluster,
		ServiceName: common.InnerServerTypeLogic,
		Handler: servicemgr.HandlerFunc{
			OnlineFn:  onlineFunc,
			OfflineFn: offlineFunc,
		},
	}

	err = server.MS.SvrMgr.Watch(battleListen, logicListen)
	if err != nil {
		xlog.Warnf("listen service %v error: %v", common.InnerServerTypeBattle, err)
		return err
	}

	listenOpt := options.NewMsgQueOptions()
	listenOpt.SetListenParams(options.NewListenParams(fmt.Sprintf(":%v", server.MS.ConfBase.Server.Port)))
	listenOpt.SetWriteChanSize(options.WRITE_CHAN_SIZE_S)

	err = server.MS.NetMgr.StartListen(listenOpt, g.sh)
	if err != nil {
		return err
	}

	if _, err = server.MS.TimerMgr.AddTimer(
		"DailyTaskReset",
		xtime.NextLocalDayZeroSec(),
		xtime.DaySec, // interval
		0,            // no limit
		nil,          // payload
		true,         // callback in goroutine
		func(name string, now int64, value any) {
			g.onDailyReset(now) // on tick: refresh signal, daily pass, etc
		},
	); err != nil {
		return err
	}

	if _, err := server.MS.TimerMgr.AddSimpleTimer("report_server_info", 1, true, g.ReportServerInfo); err != nil {
		return err
	}
	if _, err := server.MS.TimerMgr.AddSimpleTimer("prewarm_battle_grpc_conn", xtime.MinSec, true, g.PrewarmBattleGrpcConn); err != nil {
		return err
	}
	if _, err := server.MS.TimerMgr.AddSimpleTimer("print_status", 20, true, g.printStatus); err != nil {
		return err
	}

	return nil
}

func (g *LogicSvr) BeforeStop() error {
	inst := server.MS.SvrMgr.SelfCopy()
	_ = cache.DelServerInfo(inst)
	g.moduleMgr.OnBeforeStop()
	g.gamerMgr.Stop()
	return nil
}

func (g *LogicSvr) OnStop() error {
	g.sh.Stop()
	g.monitor.Stop()
	g.moduleMgr.OnStop()

	return nil
}

func (g *LogicSvr) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	// g.gamerMgr.Foreach(
	// func(_ int64, gamer *actor.Gamer) bool {
	// gamer.Post(func() {
	// 	gamer.OnDocReload(newCfg.Doc, newCfg.DocExtend)
	// 	result, err := versionfix.Run(gamer)
	// 	if err != nil {
	// 		xlog.Errorf("reload version fix failed: gid=%d err=%v", gamer.GetGamerId(), err)
	// 		return
	// 	}
	// 	if gamer.IsOnline() {
	// 		versionfix.SyncChangedModules(gamer, result)
	// 	}
	// })
	// return true
	// })
	return nil
}

func (g *LogicSvr) OnHeart(now int64) error {
	g.moduleMgr.OnHeart(now)
	return nil
}

func (g *LogicSvr) OnEventHandle(_ *eventpb.Event) {}

func (g *LogicSvr) ReportServerInfo(name string, now int64, value any) {
	inst := server.MS.SvrMgr.SelfCopy()
	onlineCount := g.gamerMgr.GamerOnlineCount()
	// totalCount := g.gamerMgr.GamerCount()
	inst.UpdateOnlineCount(int32(onlineCount))
	if err := cache.UpdateServerInfoWithOnline(inst); err != nil {
		xlog.Warnf("update logic server info failed: %v", err)
	}

	m, err := cache.GetServiceAllBattleLoadInfo(common.InnerServerTypeBattle)
	if err != nil {
		xlog.Warnf("get battle load info failed: %v", err)
		return
	}

	updates := buildBattleLoadUpdates(server.MS.SvrMgr.List(common.InnerServerTypeBattle, nil), m)
	_ = server.MS.SvrMgr.UpdateLoads(updates)
}

func (g *LogicSvr) PrewarmBattleGrpcConn(_ string, _ int64, _ any) {
	ints := server.MS.SvrMgr.List(common.InnerServerTypeBattle, func(si *servicemgr.ServiceInstance) bool {
		return si != nil && si.Enable && si.Healthy == servicemgr.ServiceStatusHealth
	})
	for _, inst := range ints {
		connTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := xgrpc.GrpcConnMgr.GetConn(connTimeout, fmt.Sprintf("%s:%d", inst.Host, inst.Port))
		if err != nil {
			xlog.Warnf("prewarm grpc conn failed: %v", err)
		}
	}
}

func buildBattleLoadUpdates(current []*servicemgr.ServiceInstance, loadInfo map[int]float64) []*servicemgr.ServiceInstance {
	updates := make([]*servicemgr.ServiceInstance, 0, len(current))
	index := make(map[int32]*servicemgr.ServiceInstance, len(current))
	for _, inst := range current {
		if inst == nil {
			continue
		}
		upd := &servicemgr.ServiceInstance{
			InstanceId:   inst.InstanceId,
			ServiceName:  common.InnerServerTypeBattle,
			OnlineCount_: inst.OnlineCount(),
			Load_:        -1,
		}
		updates = append(updates, upd)
		index[upd.InstanceId] = upd
	}
	for id, load := range loadInfo {
		upd := index[int32(id)]
		if upd == nil {
			continue
		}
		upd.Load_ = int32(math.Round(load))
	}
	return updates
}

func (g *LogicSvr) printStatus(name string, now int64, value any) {
	xlog.Infof("[run-status] buildTime=%v | progVer=%v | excelVer=%v | launchTime=%v | gmTime=%v | gamerOnline=%v | gamerTotal=%v",
		misc.BuildTime,
		misc.ProgVer,
		misc.ExcelVer,
		server.MS.LaunchTime.Format(time.DateTime),
		xtime.Now().Format(time.DateTime),
		g.gamerMgr.GamerOnlineCount(),
		g.gamerMgr.GamerCount(),
	)
}

func (g *LogicSvr) onDailyReset(now int64) {
}

func (g *LogicSvr) ServerRouteHandler(msgque netmgr.IMsgQue, data *msg.Message) bool {
	if data == nil {
		return true
	}
	msgId := pb.MSG_ID(data.MsgId())
	if common.CheckRouter(msgId) != pb.SER_TYPE_ID_LOGIC {
		xlog.Warnf("server route mismatch. target=%s got=logic msgId:%d sessId:%d gid:%d",
			common.CheckSvrType(msgId), msgId, data.PlayerSessId(), data.GId())
		return true
	}

	f, ok := server.MS.Router.GetHandler(msgId)
	if !ok || f == nil {
		xlog.Errorf("msgId:%d no register handler", msgId)
		return true
	}

	gamer := logicSvr.gamerMgr.GetGamerByGid(data.GId())
	if gamer == nil || gamer.GetPlayerSessId() != data.PlayerSessId() {
		sessId := 0
		if gamer != nil {
			sessId = int(gamer.GetPlayerSessId())
		}
		xlog.Infof("handle msg failed because gamer not found or sessid nor equal , gid:%d sessId:%d msgId:%d dataSessId:%d",
			data.GId(), sessId, msgId, data.PlayerSessId())
		return true
	}

	if err := gamer.AddMsgTask(msgId, func() {
		if server.MS.ConfBase.Global.IsDebug {
			defer logicSvr.monitor.RecordTime(msgId.String())()
		}

		if xlog.LOG_LEVEL_DEBUG == xlog.GetLogLevel() {
			xlog.Debugf("handle msg: %v  gid: %d  data: %v", msgId, data.GId(), data.Message())
		}

		errCode := errorpb.ERROR_UNEXPECTED
		var rsp proto.Message
		defer func() {
			rspMsg := msg.NewRspMsgWithProtoAndCode(data.Head.MsgId, errCode, rsp).SetUserInfo(data.PlayerSessId(), data.GId())
			server.MS.NetMgr.SendMsg2Sess(msgque.SessId(), rspMsg, nil)

		}()

		if !gamer.Function().CheckFunctionOpenByMsgId(msgId) {
			errCode = errorpb.ERROR_FUNCTION_NOT_OPEN
			return
		}

		errCode, rsp = f(msgque, data)
	}); err != nil {
		xlog.Warnf("handle msg enqueue failed, gid:%d sessId:%d msgId:%d err:%v", data.GId(), data.PlayerSessId(), msgId, err)
		errCode := errorpb.ERROR_LOGIC_SERVER_BUSY
		if err == actor.ErrGamerStopped {
			errCode = errorpb.ERROR_RPC_GAMER_NOT_FOUND
		}
		rspMsg := msg.NewRspMsgWithProtoAndCode(data.Head.MsgId, errCode, nil).SetUserInfo(data.PlayerSessId(), data.GId())
		server.MS.NetMgr.SendMsg2Sess(msgque.SessId(), rspMsg, nil)
	}

	return true
}
