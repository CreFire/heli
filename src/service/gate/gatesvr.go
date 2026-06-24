package main

import (
	"context"
	"fmt"
	"game/deps/misc"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/netmgr/options"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
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
	"game/src/proto/pbrpc"
	"game/src/service/gate/gateuser"
	"game/src/service/gate/module"
	"time"
)

var gateSvr = NewGateSvr()

func NewGateSvr() *GateSvr {
	return &GateSvr{
		GateUserMgr: gateuser.UserMgr,
		ModuleMgr:   module.NewModuleMgr(),
	}
}

// GateSvr 是 Gate 服务的核心对象，负责管理 Gate 侧用户会话、模块生命周期和路由逻辑。
// Gate 负责以下职责：
// 1. 接收客户端连接并维护在线会话
// 2. 将客户端请求转发到逻辑服或公共服
// 3. 监听服务注册/下线事件并同步路由信息
// 4. 处理客户端断开、服务器停服等生命周期事件
type GateSvr struct {
	GateUserMgr *gateuser.GateUserMgr
	ModuleMgr   *module.ModuleMgr
	sh          *msghandler.ServerNetEventHandler
	ch          *msghandler.ClientNetEventHandler
}

func (g *GateSvr) OnInit() error {
	// Gate 启动前初始化后端服务依赖，例如服务发现、配置、日志等。

	return nil
}
func (g *GateSvr) BeforeStart() error {
	// 在 Gate 启动前订阅灰度停服事件，用于通知所有在线客户端服务器即将关闭。
	server.MS.EventBus.Subscribe(eventpb.EVENT_TYPE_SERVER_GRAY, nil, false, func(ctx context.Context, e *eventpb.Event, a any) error {
		s := e.GetServerGray()
		unix, err := xtime.StringToTime(s.ShutdownTime, time.DateTime)
		if err != nil {
			xlog.Warnf("server gray shutdown time parse failed. shutdownTime:%s layout:%s err:%v",
				s.ShutdownTime, time.DateTime, err)
			return nil
		}
		ntf := pb.ServerCloseNTF{
			ShutTime: unix.Unix(),
		}
		m := msg.NewMsgWithProto(pb.MSG_ID_SERVER_CLOSE_NTF, &ntf)
		server.MS.NetMgr.SendMsg2AllUser(m, nil)
		return nil
	})
	return nil
}

func (g *GateSvr) OnStart() error {
	// Gate 启动时完成持久层准备、定时任务调度、服务发现监听及网络监听。
	persist.InitCollections()

	if _, err := server.MS.TimerMgr.AddSimpleTimer("report_server_info", 3, true, g.ReportServerInfo); err != nil {
		return err
	}
	if _, err := server.MS.TimerMgr.AddSimpleTimer("print_status", 20, true, g.printStatus); err != nil {
		return err
	}
	if _, err := server.MS.TimerMgr.AddSimpleTimer("server-ping", 60, true, g.PingServer); err != nil {
		return err
	}

	gateSvr.sh = msghandler.NewServerNetEventHandler(server.MS, gateSvr.ServerRouteHandler)

	var err error
	logicListenInfo := servicemgr.ListenSpec{
		Cluster:     server.MS.ConfBase.Server.Cluster,
		ServiceName: common.InnerServerTypeLogic,
		Handler:     &serverListenInfo{},
	}

	err = server.MS.SvrMgr.Watch(logicListenInfo)
	if err != nil {
		return err
	}

	listenOpt := options.NewMsgQueOptions()
	listenOpt.SetTransport(options.TransportWebSocket) // 设置ws 作为连接点
	listenOpt.SetListenParams(options.NewListenParams(fmt.Sprintf(":%v", server.MS.ConfBase.Server.Port)))
	listenOpt.SetIsGate(true)
	listenOpt.SetNetCfg(server.MS.ConfBase.Server.Net)

	g.ch = msghandler.NewClientNetEventHandler(server.MS, g.ClientRouteHandler, g.ClientDisconnectHandler)
	err = server.MS.NetMgr.StartListen(listenOpt, g.ch)
	if err != nil {
		return err
	}

	if err := g.ModuleMgr.OnStart(server.MS.Rpc, server.MS.Router); err != nil {
		return err
	}
	return nil
}

func (g *GateSvr) BeforeStop() error {
	// Gate 停服前：从缓存中删除自身注册信息，通知客户端断开，并调用模块前置停止逻辑。
	inst := server.MS.SvrMgr.SelfCopy()
	if err := cache.DelServerInfo(inst); err != nil {
		xlog.Warnf("gate before stop delete server info failed. service:%s instanceId:%d err:%v",
			inst.ServiceName, inst.InstanceId, err)
	}

	g.notifyClientsServerStopping()
	g.ModuleMgr.OnBeforeStop()
	return nil
}

func (g *GateSvr) OnStop() error {
	// Gate 停服完成后释放网络事件处理器和模块资源。
	g.sh.Stop()
	g.ModuleMgr.OnStop()
	return nil
}

func (g *GateSvr) notifyClientsServerStopping() {
	// 通知所有在线客户端：服务器将停止，并主动断开会话。
	for gamerId, sessId := range g.GateUserMgr.OnlineSessions() {
		msg2 := msg.NewMsgWithCode(pb.MSG_ID_NOTIFY_KICK_NTF, errorpb.ERROR_KICK_SERVER_FIX, nil)
		server.MS.NetMgr.SendMsg2Sess(sessId, msg2, nil)
		server.MS.NetMgr.KickSession(sessId, gamerId)
		g.GateUserMgr.DelBySess(gamerId, sessId)
	}
}

func (g *GateSvr) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	// 配置热更入口，当前 Gate 不做动态配置处理。
	return nil
}

func (g *GateSvr) OnHeart(now int64) error {
	// 心跳周期入口，Gate 目前没有额外心跳处理。
	return nil
}

func (g *GateSvr) OnEventHandle(evt *eventpb.Event) {
	// 通用事件处理入口，Gate 目前不需要额外事件处理。
}

func (g *GateSvr) ReportServerInfo(name string, now int64, value any) {
	// 定时上报 Gate 在线人数和逻辑服负载信息，用于服务发现和调度。
	inst := server.MS.SvrMgr.SelfCopy()
	inst.UpdateOnlineCount(int32(server.MS.NetMgr.GetAcceptSessNum()))

	if err := cache.UpdateServerInfoWithOnline(inst); err != nil {
		xlog.Warnf("gate report server info failed. service:%s instanceId:%d online:%d err:%v",
			inst.ServiceName, inst.InstanceId, inst.OnlineCount(), err)
	}
	m, err := cache.GetServiceAllOnline(common.InnerServerTypeLogic)
	if err != nil {
		xlog.Errorf("get logic server online info failed. err:%v", err)
		return
	}

	insts := make([]*servicemgr.ServiceInstance, 0, len(m))
	for k, v := range m {
		insts = append(insts, &servicemgr.ServiceInstance{
			InstanceId:   int32(k),
			OnlineCount_: int32(v),
			ServiceName:  common.InnerServerTypeLogic,
		})
	}
	_ = server.MS.SvrMgr.UpdateLoads(insts)
}

func (g *GateSvr) printStatus(name string, now int64, value any) {
	// 定时打印 Gate 当前运行状态，用于运维监控和日志跟踪。
	xlog.Infof("[run-status] buildTime=%v | progVer=%v | excelVer=%v | launchTime=%v | gmTime=%v | netLinks=%v",
		misc.BuildTime,
		misc.ProgVer,
		misc.ExcelVer,
		server.MS.LaunchTime.Format(time.DateTime),
		xtime.Now().Format(time.DateTime),
		server.MS.NetMgr.GetAcceptSessNum(),
	)
}

func (g *GateSvr) PingServer(name string, now int64, value any) {
	// 定时向已知 Logic 和 Public 实例发送心跳请求，检测网络连通性与服务可用性。
	ins := server.MS.SvrMgr.List(common.InnerServerTypeLogic, func(si *servicemgr.ServiceInstance) bool {
		return true
	})
	ins = append(ins)

	for _, v := range ins {
		if v.NetState() == servicemgr.NetUnValid {
			xlog.Warnf("skip ping because instance net invalid. service:%s instanceId:%d", v.ServiceName, v.InstanceId)
			continue
		}
		req := &pbrpc.S2SPingREQ{
			SrcServerId: int64(server.MS.ConfBase.Server.Id),
			TimeStr:     xtime.Now().Format("2006-01-02 15:04:05.000"),
			TarServerId: int64(v.InstanceId),
		}

		m := msg.NewMsgWithProto(pb.MSG_ID_S2S_PING_REQ, req).SetHashKey(int64(v.InstanceId))
		rpcRsp, err := server.MS.Rpc.SendRequestWithBlock(v.ServiceName, v.InstanceId, m, nil)
		if err != nil {
			xlog.Warnf("ping server failed, server name: %s , instance id:%d  ping error:%v", v.ServiceName, v.InstanceId, err)
			continue
		}

		rsp := rpcRsp.GetPingRsp()
		xlog.Infof("ping server success, server name: %s , instance id:%d  ping rsp:%v", v.ServiceName, v.InstanceId, rsp.String())
	}
}

func (g *GateSvr) ServerRouteHandler(msgque netmgr.IMsgQue, data *msg.Message) bool {
	user, ok := gateSvr.GateUserMgr.Get(data.Head.Gid)
	if !ok {
		if g.GateUserMgr.NeedSetAckSeq(int(data.MsgId())) {
			xlog.Warnf("skip add send seq/ack because gate user missing. gid=%v sess=%v msgId=%v fromSvr=%v",
				data.Head.Gid, data.Head.GSessId, data.MsgId(), common.CheckSvrType(pb.MSG_ID(data.MsgId())))
		}
		return true
	}
	if !user.MatchSessId(data.PlayerSessId()) {
		xlog.Debugf("drop s2c message because session offline. gid=%v sess=%v msgId=%v currentSess=%v",
			data.Head.Gid, data.Head.GSessId, data.MsgId(), user.GetSessId())
		return true
	}

	if g.GateUserMgr.NeedSetAckSeq(int(data.MsgId())) {
		if !user.AddSendMessageSeqAck(data.PlayerSessId(), data) {
			xlog.Debugf("skip add send seq/ack: gid=%v sess=%v msgId=%v", data.Head.Gid, data.Head.GSessId, data.MsgId())
			return true
		}
	}

	// s -> c
	server.MS.NetMgr.SendMsg2Sess(data.Head.GSessId, data, nil)
	return true
}

// handler gate的handler
func (g *GateSvr) ClientRouteHandler(msgque netmgr.IMsgQue, data *msg.Message) bool {
	// 处理客户端发送到 Gate 的消息，根据 msgId 进行路由到逻辑服或公共服。
	msgId := pb.MSG_ID(data.Head.MsgId)
	svrType := common.CheckSvrType(msgId)
	if svrType == "" || msgId >= pb.MSG_ID_S2S_RPC_END {
		xlog.Debugf("drop invalid c2s msg id=%v sess=%d gid=%d", msgId, msgque.SessId(), msgque.GetAgent().GetCltUser())
		return false
	}

	limited, err := g.GateUserMgr.CheckAndUpdateSeqAckAndLimit(msgque.GetAgent().GetCltUser(), msgque.SessId(), uint16(data.Seq()), uint16(data.Ack()))
	if err != nil {
		xlog.Debugf("message dropped [svrType:%v]sessId:%v gamerId:%v msgId:%v seq:%v ack:%v err:%v", svrType, msgque.SessId(), msgque.GetAgent().GetCltUser(), msgId, data.Seq(), data.Ack(), err)
		return false
	}
	if limited {
		xlog.Warnf("client request rate limited. gid:%d sessId:%d msgId:%d", msgque.GetAgent().GetCltUser(), msgque.SessId(), msgId)
		msgque.Send(msg.NewRspMsgWithProtoAndCode(msgId, errorpb.ERROR_TCP_C2S_TOO_FAST, nil))
		return false
	}

	serTypeId := common.CheckRouter(msgId)
	switch serTypeId {
	case pb.SER_TYPE_ID_LOGIC:
		svrId := msgque.GetAgent().GetCltRoute(svrType)
		if svrId < 0 {
			xlog.Warnf("drop c2s message because no route. svrType=%v sessId=%d gid=%d msgId=%d",
				svrType, msgque.SessId(), msgque.GetAgent().GetCltUser(), msgId)
			return false
		}

		data.SetUserInfo(msgque.SessId(), msgque.GetAgent().GetCltUser())
		server.MS.NetMgr.SendMsg2Fix(svrType, svrId, data, nil)
	default:
		xlog.Warnf("unsupported router type. serTypeId:%v sessId:%d gid:%d msgId:%d", serTypeId, msgque.SessId(), msgque.GetAgent().GetCltUser(), msgId)
		return false
	}

	if xlog.LOG_LEVEL_DEBUG == xlog.GetLogLevel() {
		xlog.Debugf("c2%v sess=%v msgId=%v", svrType, msgque.SessId(), msgId)
	}

	return false
}

// gate的handler
func (g *GateSvr) ClientDisconnectHandler(gamerId, sessId int64) {
	// 客户端断开时清理 Gate 会话缓存，并可能延迟执行真正的登出处理。
	xlog.Debugf("gamer disconnect [gamerId:%d] [sessId:%d]", gamerId, sessId)
	cleared := gateSvr.GateUserMgr.ClearSessBySess(gamerId, sessId)
	if gamerId > 0 && sessId > 0 {
		gateSvr.GateUserMgr.TakeLogoutReason(gamerId, sessId)
	}
	if gamerId > 0 && sessId > 0 && cleared {
		if _, err := cache.ClearGateOnlineDataBySession(gamerId, sessId); err != nil {
			xlog.Warnf("clear gate online data on disconnect failed. gid:%d sessId:%d err:%v", gamerId, sessId, err)
		}
	}
	if server.MS.Stopping || !cleared {
		return
	}
	if _, err := server.MS.TimerMgr.AddOneShotTimer("gate_log_out", common.GATE_CACHE_USER_TIME, false, func(name string, now int64, value any) {
		if gateSvr.GateUserMgr.DelOffline(gamerId) {
			xlog.Debugf("user logout sessId:%v gamerId:%v logout:%v", sessId, gamerId, xtime.NowUnix())
		}
	}); err != nil {
		xlog.Warnf("add logout timer failed. gid:%d sessId:%d timer:gate_log_out delay:%d err:%v",
			gamerId, sessId, common.GATE_CACHE_USER_TIME, err)
	}
}
