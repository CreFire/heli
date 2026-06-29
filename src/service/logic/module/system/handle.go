package system

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
	"game/src/service/logic/actor"
	"game/src/service/logic/iface"
	"game/src/service/logic/module/player"
	"time"

	"google.golang.org/protobuf/proto"
)

const gamerRpcWaitTimeout = 20 * time.Second

type SystemHandler struct {
}

func notifySwitchServerNTFIfGray(send func(proto.Message), status servicemgr.ServiceStatus) {
	if send == nil || status != servicemgr.ServiceStatusGray {
		return
	}
	send(&pb.SwitchServerNTF{})
}

func (m *SystemHandler) HandleRpcGamerLogin(msgque netmgr.IMsgQue, req *msg.Message) *pbrpc.S2SRpcRSP {
	msg := req.Message().(*pbrpc.S2SUserLoginREQ)
	rpcRsp := &pbrpc.S2SRpcRSP{
		RspType: &pbrpc.S2SRpcRSP_UserLoginRsp{UserLoginRsp: &pbrpc.S2SUserLoginRSP{}},
	}
	if server.MS.Stopping {
		xlog.Infof("reject gamer login while stopping. gid:%d sessId:%d", msg.Gid, msg.PlayerSession)
		rsp := &pbrpc.S2SRpcRSP{
			Error: &errorpb.RpcError{ErrCode: errorpb.ERROR_KICK_SERVER_FIX, ErrDesc: "server is stopping"},
		}
		return rsp
	}

	gid := msg.Gid
	playerSessId := msg.PlayerSession
	xlog.Infof("gamer[gid=%d & sessId=%d] start to login", gid, playerSessId)
	// 重连
	if msg.IsReconnect {
		gamer := actor.GamerMgr.GetGamerByGid(gid)
		if gamer == nil || gamer.IsStop() {
			xlog.Infof("gamer[gid=%d] reconnect but gamer not exist or is stop", gid)
			rpcRsp.GetUserLoginRsp().ReconnectOk = false
			return rpcRsp
		}
		// 断开旧连接
		actor.GamerMgr.ReplaceGamerSessId(gamer.GetPlayerSessId(), msg.PlayerSession, gamer)
		if err := gamer.AddMsgTask(0, func() {
			gamer.Online(msgque.SessId(), playerSessId)
			// 服务器灰度中，通知客户端切换服务器
			notifySwitchServerNTFIfGray(gamer.SendMsg, server.MS.SvrMgr.SelfCopy().Healthy)
		}); err != nil {
			xlog.Errorf("gamer[gid=%d] reconnect but add msg task failed: %v", gid, err)
			rpcRsp.GetUserLoginRsp().ReconnectOk = false
			return rpcRsp
		}
		rpcRsp.GetUserLoginRsp().ReconnectOk = true
		return rpcRsp
	}
	gamer := m.checkLoginOnline(playerSessId, gid, msgque)
	if gamer == nil {
		xlog.Warnf("gamer[gid=%d] login but gamer not found", gid)
		return rpcRsp
	}
	now := xtime.NowUnix()
	err := gamer.AddMsgTask(pb.MSG_ID_LOGIN_BY_SESSION_REQ, func() {
		recordModel := player.GetRecordModel(gamer.Model)
		if !recordModel.Has(int32(pb.TIME_RECORD_TYPE_LAST_LOGIN)) {
			gamer.LoginFirst(now)
		}
		recordModel.SetRecord(int32(pb.TIME_RECORD_TYPE_LAST_LOGIN), xtime.NowUnix())
		lastLogin := recordModel.GetRecord(int32(pb.TIME_RECORD_TYPE_LAST_LOGIN))
		dayFirstLogin := xtime.CheckDailyFresh(lastLogin, now)
		// deviceChange := !player.GetPl(gamer.Model).SetDeviceIDIfChanged(msg.DeviceId)
		// 登录
		gamer.OnLogin(now, dayFirstLogin, false)
		// // 登录之后
		// gamer.LoginAfter(false, false)
		// 灰度
		notifySwitchServerNTFIfGray(gamer.SendMsg, server.MS.SvrMgr.SelfCopy().Healthy)
	})
	if err != nil {
		rpcRsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_GAMER_TASK_ADD_FAILED, ErrDesc: "gamer task enqueue failed"}
		return rpcRsp
	}
	rpcRsp.GetUserLoginRsp().Gid = gamer.GamerId
	return rpcRsp
}
func (m *SystemHandler) HandleRpcSwitchServer(msgque netmgr.IMsgQue, msg *msg.Message) *pbrpc.S2SRpcRSP {
	req := msg.Message().(*pbrpc.S2SSwitchServerREQ)
	rsp := &pbrpc.S2SRpcRSP{
		RspType: &pbrpc.S2SRpcRSP_SwitchServerRsp{SwitchServerRsp: &pbrpc.S2SSwitchServerRSP{}},
	}
	if server.MS.Stopping {
		xlog.Infof("switch server while stopping. gid:%d sessId:%d isOut:%v", req.Gid, req.GamerSession, req.IsSwitchOut)
		rsp := &pbrpc.S2SRpcRSP{
			Error: &errorpb.RpcError{ErrCode: errorpb.ERROR_KICK_SERVER_FIX, ErrDesc: "server is stopping"},
		}
		return rsp
	}
	if req.IsSwitchOut {
		g := actor.GamerMgr.GetGamerByGid(req.Gid)
		if g != nil {
			if err := g.AddMsgTask(pb.MSG_ID_S2S_SWITCH_SERVER_REQ, func() {
				player.GetMainModel(g.Model).SetConfVersion(g.Doc().ExcelVersion)
				g.SetOnlineStatus(actor.GamerStatus_Offline)
			}); err != nil {
				xlog.Warnf("switch out enqueue prepare failed. gid:%d sessId:%d err:%v", req.Gid, req.GamerSession, err)
				rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_GAMER_TASK_ADD_FAILED, ErrDesc: "switch out gamer task enqueue failed"}
				return rsp
			}
			actor.GamerMgr.DelGamer(g.GetPlayerSessId(), req.Gid, "switch to out")
		} else {
			xlog.Infof("switch out gamer not found. gid:%d sessId:%d", req.Gid, req.GamerSession)
			rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_GM_FORBID_ROLE, ErrDesc: "switch out role not exist"}
		}
	} else {
		xlog.Warnf("switch in request not implemented. gid:%d sessId:%d", req.Gid, req.GamerSession)
	}
	return nil
}

func (m *SystemHandler) HandleSsKickSession(_ netmgr.IMsgQue, msg *msg.Message) {
	_ = server.MS.PostAsyncTask(uint64(msg.Head.Gid), "HandleSsKickSession", func() {
		gamer := actor.GamerMgr.GetGamerByGid(msg.Head.Gid)
		if gamer == nil {
			return
		}
		if !gamer.OfflineIfSessionMatch(msg.PlayerSessId(), "gate kick", false) {
			xlog.Debugf("HandleSsKickSession: user session changed gid:%v", msg.Head.Gid)
		}
	})
}

func (m *SystemHandler) HandleHeart(ctx iface.IGamerContext, data *msg.Message) (code errorpb.ERROR, rsp proto.Message) {
	req := data.Message().(*pb.HeartREQ)
	rsp = &pb.HeartRSP{CltTs: req.CltTs, SvrTs: xtime.NowUnixMs()}
	gamer, ok := ctx.(*actor.Gamer)
	if !ok {
		return errorpb.ERROR_FAILED, rsp
	}

	gamer.SetLastHeart(xtime.NowUnix())

	return errorpb.ERROR_SUCCESS, rsp
}

func (m *SystemHandler) HandleStress(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req := data.Message().(*pb.StressREQ)
	rsp := &pb.StressRSP{Val: req.Val}
	return errorpb.ERROR_SUCCESS, rsp
}

func (m *SystemHandler) HandleSayHello(ctx iface.IGamerContext, data *msg.Message) (errorpb.ERROR, proto.Message) {
	req, ok := data.Message().(*pb.SayHelloREQ)
	if !ok || req == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	rsp := &pb.SayHelloRSP{Id: req.Id, Type: req.Type}
	return errorpb.ERROR_SUCCESS, rsp
}

func (m *SystemHandler) checkLoginOnline(playerSession, gid int64, msgque netmgr.IMsgQue) *actor.Gamer {
	gamer := actor.GamerMgr.GetGamerByGid(gid)
	gateSessId := int64(0)
	if msgque != nil {
		gateSessId = msgque.SessId()
	}
	if gamer != nil {
		if gamer.IsStop() {
			xlog.Warnf("gamer[gid=%d] login but gamer stopped", gid)
			return nil
		}
		gamer.Online(gateSessId, playerSession)
		return gamer
	}

	newGamer, err := actor.NewGamer(gid, gateSessId, playerSession)
	if err != nil {
		xlog.Errorf("create gamer failed. gid:%d gateSessId:%d playerSessId:%d err:%v", gid, gateSessId, playerSession, err)
		return nil
	}
	actor.GamerMgr.AddGamer(playerSession, gid, newGamer)
	return newGamer
}

