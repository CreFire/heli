package login

import (
	"context"
	"game/deps/kit"
	"game/deps/msg"
	"game/deps/netmgr"
	redisclient "game/deps/redis"
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/cache"
	"game/src/common"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
	"game/src/service/gate/gateuser"
	"math/rand/v2"
	"time"

	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
)

type LoginModule struct {
}

func NewLoginModule() *LoginModule {
	return &LoginModule{}
}

func (l *LoginModule) RegisterHandler(rpc *rpcmgr.RpcMgr, r *router.Router) error {
	r.SSRegister(pb.MSG_ID_LOGIC_2_GATE_KICK_SESSION_REQ, reqKickSession)

	r.CSRegister(pb.MSG_ID_LOGIN_BY_SESSION_REQ, reqGamerLogin)
	r.CSRegister(pb.MSG_ID_LOGIN_RECONNECT_REQ, reqGamerLoginReconnect)
	r.CSRegister(pb.MSG_ID_SWITCH_SERVER_REQ, reqGamerSwitchLogic)
	r.CSRegister(pb.MSG_ID_LOGIN_OUT_REQ, reqGamerLogout)
	return nil
}

func reqKickSession(msgque netmgr.IMsgQue, data *msg.Message) {
	pSessionId, gid := data.GetUserInfo()
	kickSessionWithErrCode(gid, pSessionId, data.ErrorCode(), "logic_kick_req")
}

func kickSessionWithErrCode(gid int64, pSessionId int64, errCode errorpb.ERROR, source string) {

	xlog.Infof("kick session source:%s gid:%d sessId:%d errCode:%v", source, gid, pSessionId, errCode)
	if pSessionId <= 0 {
		xlog.Debugf("skip invalid kick session source:%s gid:%d sessId:%d errCode:%v", source, gid, pSessionId, errCode)
		return
	}
	gateuser.UserMgr.SetLogoutReason(gid, pSessionId, source)

	msg2 := msg.NewMsgWithCode(pb.MSG_ID_NOTIFY_KICK_NTF, errCode, nil)
	server.MS.NetMgr.SendMsg2Sess(pSessionId, msg2, nil)
	server.MS.NetMgr.KickSession(pSessionId, gid)
}

func reqGamerLogin(msgque netmgr.IMsgQue, req *msg.Message) (errCode errorpb.ERROR, rsp proto.Message) {
	// 玩家通过 session 登录 Gate 的入口处理。
	// 1. 参数校验
	// 2. 会话令牌验证
	// 3. 防重复登录与账号锁
	// 4. 选择 Logic 服务并发起登录请求
	// 5. 返回登录结果并更新在线数据
	c2s := req.Message().(*pb.LoginBySessionREQ)
	s2c := &pb.LoginBySessionRSP{}

	// 设置状态和时区。
	_, offset := xtime.Now().Zone()
	s2c.TimeZone = int32(offset / xtime.HourSec)

	// 获取游戏ID并检查是否有效。
	gid := c2s.GetGid()
	if gid < common.GID_MIN || c2s.Session == "" {
		xlog.Infof("req gamer login param invalid. gid:%v session:%s", gid, c2s.Session)
		return
	}

	if msgque.GetAgent().GetCltUser() > 0 {
		kickSessionWithErrCode(msgque.GetAgent().GetCltUser(), msgque.SessId(), errorpb.ERROR_LOGIN_REPEAT, "already_login")
		return
	}

	ip := msgque.GetAgent().GetCltRemote()

	onlineData, err := cache.GetGamerOnlineData(gid)
	if err != nil {
		xlog.Warnf("userole reject login session load failed. gid:%d ip:%s err:%v", gid, ip, err)
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_DATA_EXCEPTION, "load_online_data")
		return
	}

	if onlineData.AuthToken != c2s.Session {
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_SESSION_INVALID, "session_mismatch")
		return
	}
	msgque.GetAgent().AddCltUser(gid)

	cancelFunc, err := redisclient.RedLock(server.MS.RedisDB.Client, context.Background(), cache.AccountLoginLockKey(onlineData.Account))
	if err != nil {
		xlog.Warnf("login lock failed. gid:%d sessId:%d account:%s ip:%s err:%v", gid, msgque.SessId(), onlineData.Account, ip, err)
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_REPEAT, "account_lock")
		return
	}
	defer cancelFunc()

	if user, ok := gateuser.UserMgr.Get(gid); ok {
		if oldSessId := user.GetSessId(); oldSessId > 0 {
			kickSessionWithErrCode(gid, oldSessId, errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "new_device_login")
			time.Sleep(20 * time.Millisecond) //try wait for old session exit
		}
	}

	gateuser.UserMgr.AddGateUser(gid, msgque.SessId(), true)

	// get logic server
	var ins *servicemgr.ServiceInstance
	if onlineData.LogicSvrId != 0 {
		inss := server.MS.SvrMgr.List(common.InnerServerTypeLogic,
			func(si *servicemgr.ServiceInstance) bool {
				return onlineData.LogicSvrId == si.InstanceId
			},
		)

		//存在id相等的logic服务器，服务注册中心不存在说明服务已经下线
		if len(inss) != 0 {
			if inss[0].NetState() == servicemgr.NetConnect {
				ins = inss[0]
			} else { //只是gate logic断线，避免数据冲突，暂时不让玩家上线，等待服务完全下线
				kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIC_SERVER_NOT_FOUND, "logic_unavailable")
				return
			}
		}
	}

	if ins == nil {
		ins, err = server.MS.SvrMgr.PickMinOnline(common.InnerServerTypeLogic, true)
		if ins == nil || err != nil {
			kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIC_SERVER_NOT_FOUND, "logic_not_found")
			return
		}
		ins.IncOnlineCount(1)
	}
	msgque.GetAgent().AddCltRoute(common.InnerServerTypeLogic, ins.InstanceId)

	xlog.Debugf("login to logic[svrId=%d] [sessId=%d]  [gid=%d] [ip=%v]", ins.InstanceId, msgque.SessId(), gid, ip)

	rpcReq := &pbrpc.S2SUserLoginREQ{
		Gid:           gid,
		PlayerSession: msgque.SessId(),
		DeviceId:      c2s.GetDeviceInfo().GetDeviceId(),
	}
	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_USER_LOGIN_REQ, rpcReq).SetHashKey(gid)

	rpcRsp, err := server.MS.Rpc.SendRequestWithBlock(common.InnerServerTypeLogic, ins.InstanceId, m, nil)
	if err != nil {
		errCode = lo.Ternary(rpcRsp != nil && rpcRsp.Error != nil, rpcRsp.Error.ErrCode, errorpb.ERROR_UNEXPECTED)
		xlog.Warnf("login to logic failed. gid:%d ip:%s logicSvrId:%d rpcErrCode:%v err:%v", gid, ip, ins.InstanceId, errCode, err)
		kickSessionWithErrCode(gid, msgque.SessId(), errCode, "login_rpc_failed")
		return
	}
	onlineData.LoginTime = xtime.NowUnix()
	onlineData.GateSession = msgque.SessId()
	onlineData.GateSvrId = server.MS.ConfBase.Server.Id
	onlineData.LogicSvrId = ins.InstanceId
	if err := cache.SetGamerOnlineData(gid, onlineData); err != nil {
		xlog.Warnf("set gamer online data failed after login. gid:%d err:%v", gid, err)
	}

	if userLoginRSP := rpcRsp.GetUserLoginRsp(); userLoginRSP != nil {
		s2c.Guides = userLoginRSP.Guides
	}

	msgque.Send(msg.NewMsgWithProto(pb.MSG_ID_LOGIN_BY_SESSION_RSP, s2c))
	xlog.Infof("user %d login success. ip:%s", gid, ip)
	return
}

func reqGamerLoginReconnect(msgque netmgr.IMsgQue, data *msg.Message) (errCode errorpb.ERROR, rsp proto.Message) {
	req := data.Message().(*pb.LoginReconnectREQ)
	res := &pb.LoginReconnectRSP{}
	gid := req.Gid
	if gid < common.GID_MIN || req.Session == "" {
		xlog.Infof("req gamer reconnect param invalid. gid:%v session:%s", gid, req.Session)
		return
	}

	ip := msgque.GetAgent().GetCltRemote()

	god, err := cache.GetGamerOnlineData(gid)
	if err != nil || god == nil || god.LogicSvrId == 0 {
		logicSvrId := lo.Ternary(god == nil, 0, god.LogicSvrId)
		xlog.Warnf("reconnect online data invalid. gid:%d sessId:%d logicSvrId:%d ip:%s err:%v",
			gid, msgque.SessId(), logicSvrId, ip, err)
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_RECON_SESSION_INVALID, "reconnect_online_data")
		return
	}

	if god.AuthToken != req.Session {
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_RECON_SESSION_INVALID, "reconnect_session_mismatch")
		return
	}

	cancelFunc, err := redisclient.RedLock(server.MS.RedisDB.Client, context.Background(), cache.AccountLoginLockKey(god.Account))
	if err != nil {
		xlog.Warnf("reconnect lock failed. gid:%d sessId:%d account:%s ip:%s err:%v", gid, msgque.SessId(), god.Account, ip, err)
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_REPEAT, "reconnect_account_lock")
		return
	}
	defer cancelFunc()

	user, ok := gateuser.UserMgr.Get(gid)
	if !ok || user == nil {
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_DATA_EXCEPTION, "reconnect_gate_user_missing")
		return
	}

	oldSessId := user.GetSessId()

	if !user.CanReconnect() {
		// 已经超过最大重连次数
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_RECON_SESSION_INVALID, "reconnect_too_many")
		if err := cache.DelGamerToken(gid); err != nil {
			xlog.Warnf("del gamer token failed. source:reconnect_too_many gid:%d sessId:%d err:%v", gid, msgque.SessId(), err)
		}
		return
	}

	xlog.Infof("gamer reconnect to logic[svrId=%d] [sessId=%d]  [gid=%d] [ip=%v]", god.LogicSvrId, msgque.SessId(), gid, ip)

	rpcReq := &pbrpc.S2SUserLoginREQ{
		Gid:           gid,
		PlayerSession: msgque.SessId(),
		IsReconnect:   true,
	}
	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_USER_LOGIN_REQ, rpcReq).SetHashKey(gid)

	rpcRsp, err := server.MS.Rpc.SendRequestWithBlock(common.InnerServerTypeLogic, god.LogicSvrId, m, nil)
	if err != nil {
		errCode = lo.Ternary(rpcRsp != nil && rpcRsp.Error != nil, rpcRsp.Error.ErrCode, errorpb.ERROR_UNEXPECTED)
		xlog.Warnf("reconnect rpc failed. gid:%d sessId:%d logicSvrId:%d msgId:%d rpcErrCode:%v err:%v",
			gid, msgque.SessId(), god.LogicSvrId, pb.MSG_ID_S2S_USER_LOGIN_REQ, errCode, err)

		kickSessionWithErrCode(gid, msgque.SessId(), errCode, "reconnect_rpc_failed")
		return
	}

	if !rpcRsp.GetUserLoginRsp().ReconnectOk {
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_KICK_DATA_EXCEPTION, "reconnect_logic_reject")
		return
	}
	msgque.GetAgent().AddCltUser(gid)
	msgque.GetAgent().AddCltRoute(common.InnerServerTypeLogic, god.LogicSvrId)

	if !gateuser.UserMgr.ReplaceSess(gid, oldSessId, msgque.SessId()) {
		kickSessionWithErrCode(gid, msgque.SessId(), errorpb.ERROR_LOGIN_DATA_EXCEPTION, "reconnect_replace_session_failed")
		return
	}
	user.ResendUnAckMessages(msgque, uint16(req.Ack))

	god.GateSvrId = server.MS.ConfBase.Server.Id
	god.GateSession = msgque.SessId()
	god.LoginTime = xtime.NowUnix()
	if err := cache.SetGamerOnlineData(gid, god); err != nil {
		xlog.Errorf("set gamer online data failed after reconnect. gid:%d err:%v", gid, err)
	}

	if oldSessId != msgque.SessId() && oldSessId > 0 {
		kickSessionWithErrCode(gid, oldSessId, errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "reconnect_replace_old_session")
	}

	m = msg.NewMsgWithProto(pb.MSG_ID_LOGIN_RECONNECT_RSP, res)
	msgque.Send(m)

	return
}

func reqGamerLogout(msgque netmgr.IMsgQue, req *msg.Message) (errCode errorpb.ERROR, rsp proto.Message) {
	gid := msgque.GetAgent().GetCltUser()
	sessID := msgque.SessId()
	xlog.Infof("gamer logout [sessId=%d] [gid=%d]", msgque.SessId(), gid)
	if err := cache.DelGamerToken(gid); err != nil {
		xlog.Warnf("del gamer token failed. source:logout gid:%d sessId:%d err:%v",
			gid, sessID, err)
	}
	msgque.Send(msg.NewMsg(pb.MSG_ID_LOGIN_OUT_RSP, kit.PbData(&pb.LoginOutRSP{})))
	gateuser.UserMgr.SetLogoutReason(gid, sessID, "active_logout")
	xlog.Infof("close session source:logout gid:%d sessId:%d", gid, sessID)
	server.MS.NetMgr.KickSession(sessID, 0)
	return
}

func reqGamerSwitchLogic(msgque netmgr.IMsgQue, req *msg.Message) (errCode errorpb.ERROR, resp proto.Message) {
	//req := req.C2S().(*pb.SwitchServerREQ)
	res := &pb.SwitchServerRSP{}
	gid := msgque.GetAgent().GetCltUser()
	sessID := msgque.SessId()

	defer func() {
		xres := msg.NewMsg(pb.MSG_ID_SWITCH_SERVER_RSP, kit.PbData(res))
		server.MS.NetMgr.SendMsg2Sess(sessID, xres, nil)
		if !res.SwitchSuccess {
			server.MS.NetMgr.KickSession(sessID, 0)
		}
	}()

	if gid < common.GID_MIN {
		xlog.Infof("req gamer switch logic gid err. gid:%v", gid)
		return
	}

	xreq := &pbrpc.S2SSwitchServerREQ{
		IsSwitchOut:  true,
		Gid:          gid,
		GamerSession: msgque.SessId(),
	}
	svrId := msgque.GetAgent().GetCltRoute(common.InnerServerTypeLogic)
	if svrId < 0 {
		xlog.Warnf("switch logic failed, no logic route. gid:%d sessId:%d msgId:%d", gid, msgque.SessId(), req.MsgId())
		return
	}
	oldLogic := server.MS.SvrMgr.List(common.InnerServerTypeLogic, func(si *servicemgr.ServiceInstance) bool {
		return svrId == si.InstanceId
	})

	if len(oldLogic) == 0 {
		xlog.Warnf("switch logic failed, old logic instance missing. gid:%d sessId:%d oldLogic:%d", gid, msgque.SessId(), svrId)
		return
	}
	if oldLogic[0].Healthy == servicemgr.ServiceStatusHealth {
		xlog.Infof("switch logic skipped, old logic healthy. gid:%d sessId:%d oldLogic:%d", gid, msgque.SessId(), svrId)
		return
	}
	god, err := cache.GetGamerOnlineData(gid)
	if err != nil || god == nil {
		xlog.Warnf("switch logic failed, load online data failed. gid:%d sessId:%d err:%v", gid, msgque.SessId(), err)
		return
	}
	cancelFunc, err := redisclient.RedLock(server.MS.RedisDB.Client, context.Background(), cache.AccountLoginLockKey(god.Account))
	if err != nil {
		xlog.Warnf("switch logic lock failed. gid:%d sessId:%d err:%v", gid, msgque.SessId(), err)
		return
	}
	defer cancelFunc()

	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_SWITCH_SERVER_REQ, xreq)
	m.SetHashKey(gid)

	rsp, err := server.MS.Rpc.SendRequestWithBlock("logic", svrId, m, func() {
		xlog.Warnf("switch logic timeout on switch out. gid:%d oldLogic:%d", gid, svrId)
	})

	if err != nil {
		xlog.Warnf("switch logic failed on switch out request. gid:%d oldLogic:%d err:%v", gid, svrId, err)
		return
	}
	if rsp.GetSwitchServerRsp().Ok != 0 {
		xlog.Infof("switch logic rejected by old logic. gid:%d oldLogic:%d ret:%d", gid, svrId, rsp.GetSwitchServerRsp().Ok)
		return
	}

	// nil filter may return the shared internal slice; downstream must keep it read-only.
	newLogic := pickSwitchLogicTarget(oldLogic[0], server.MS.SvrMgr.List(common.InnerServerTypeLogic, nil))
	if newLogic == nil {
		xlog.Errorf("switch logic failed selecting new logic. gid:%d oldLogic:%d reason:no_higher_healthy_logic", gid, svrId)
		return
	}
	xreq.IsSwitchOut = false

	m = msg.NewMsgWithProto(pb.MSG_ID_S2S_SWITCH_SERVER_REQ, xreq)
	m.SetHashKey(gid)

	_, err = server.MS.Rpc.SendRequestWithBlock("logic", newLogic.InstanceId, m, func() {
		xlog.Warnf("switch logic timeout on switch in. gid:%d oldLogic:%d newLogic:%d", gid, svrId, newLogic.InstanceId)
	})
	if err != nil {
		xlog.Warnf("switch logic failed on switch in request. gid:%d oldLogic:%d newLogic:%d err:%v", gid, svrId, newLogic.InstanceId, err)
		return
	}
	if err := cache.SetLogicSvrId(gid, newLogic.InstanceId); err != nil {
		xlog.Warnf("switch logic set logic server id failed. gid:%d oldLogic:%d newLogic:%d err:%v", gid, svrId, newLogic.InstanceId, err)
		return
	}

	res.SwitchSuccess = true
	xlog.Infof("switch logic success. gid:%d sessId:%d oldLogic:%d newLogic:%d", gid, msgque.SessId(), svrId, newLogic.InstanceId)
	msgque.GetAgent().AddCltRoute(common.InnerServerTypeLogic, newLogic.InstanceId)

	return
}

func pickSwitchLogicTarget(oldLogic *servicemgr.ServiceInstance, instances []*servicemgr.ServiceInstance) *servicemgr.ServiceInstance {
	if oldLogic == nil {
		return nil
	}
	oldLogicId := oldLogic.InstanceId
	oldVersion := oldLogic.ProVersion
	highestHigherVersion := oldVersion
	higherVersionCandidates := make([]*servicemgr.ServiceInstance, 0, len(instances))
	sameVersionCandidates := make([]*servicemgr.ServiceInstance, 0, len(instances))
	for _, instance := range instances {
		if instance == nil || instance.InstanceId == oldLogicId {
			continue
		}
		if !instance.Enable || instance.Healthy != servicemgr.ServiceStatusHealth || instance.NetState() != servicemgr.NetConnect {
			continue
		}

		version := instance.ProVersion
		if version > oldVersion {
			if version > highestHigherVersion {
				highestHigherVersion = version
				higherVersionCandidates = higherVersionCandidates[:0]
			}
			if version == highestHigherVersion {
				higherVersionCandidates = append(higherVersionCandidates, instance)
			}
			continue
		}
		if version == oldVersion {
			sameVersionCandidates = append(sameVersionCandidates, instance)
		}
	}
	if len(higherVersionCandidates) > 0 {
		return higherVersionCandidates[rand.IntN(len(higherVersionCandidates))]
	}
	if len(sameVersionCandidates) > 0 {
		return sameVersionCandidates[rand.IntN(len(sameVersionCandidates))]
	}
	return nil
}
