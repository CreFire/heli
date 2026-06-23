package msghandler

import (
	"game/deps/async"
	"game/deps/basal"
	"game/deps/kit"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/proto/msgbase"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/deps/xlog"
	"game/deps/xtime"
	"time"

	"game/src/common"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
	"sync"

	"github.com/samber/lo"
)

type ClientRoutMessageHandler func(msgque netmgr.IMsgQue, msg *msg.Message) bool

const (
	rpcWorkerCount = 128
	rpcQueueSize   = 8 * 1024
)

type ServerNetEventHandler struct {
	*NetEventBaseHandler
	s            *server.Server
	route        ClientRoutMessageHandler
	rpcReqAsync  *async.Async
	rpcRespAsync *async.Async
	stopOnce     sync.Once
}

func NewServerNetEventHandler(server *server.Server, routeHandler ClientRoutMessageHandler) *ServerNetEventHandler {
	xlog.Infof("init server net event handler ...")
	rpcReqAsync, err := async.NewAsync(rpcWorkerCount, rpcQueueSize)
	if err != nil {
		panic(err)
	}
	rpcRespAsync, err := async.NewAsync(rpcWorkerCount, rpcQueueSize)
	if err != nil {
		panic(err)
	}
	rpcReqAsync.Start()
	rpcRespAsync.Start()

	svh := &ServerNetEventHandler{
		NetEventBaseHandler: &NetEventBaseHandler{},
		s:                   server,
		route:               routeHandler,
		rpcReqAsync:         rpcReqAsync,
		rpcRespAsync:        rpcRespAsync,
	}
	svh.RegisterServerBaseHandlers()
	return svh
}

func (h *ServerNetEventHandler) Stop() {
	h.stopOnce.Do(func() {
		h.rpcReqAsync.Stop()
		h.rpcRespAsync.Stop()
	})
}

func (h *ServerNetEventHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool {
	if msgque.GetConnectType() == netmgr.ConnTypeConn {
		serverInfo := &pb.SayHelloREQ{
			Id:   h.s.ConfBase.Server.Id,
			Type: h.s.ConfBase.Server.Type,
		}
		msgque.Send(msg.NewMsg(pb.MSG_ID_SAY_HELLO_REQ, kit.PbData(serverInfo)))
	}
	return true
}

func (h *ServerNetEventHandler) OnMsgQueStop(msgque netmgr.IMsgQue) {
	if msgque == nil || msgque.GetAgent() == nil {
		return
	}

	msgque.GetAgent().DelSvrSess(func(svrType string, svrId int32) {
		err := h.s.SvrMgr.UpdateNetStatus(svrType, svrId, servicemgr.NetUnValid)
		if err != nil {
			xlog.Infof("[svrType:%v | svrId:%v]skip net status update(invalid): %v", svrType, svrId, err)
		}
	})
}

func (h *ServerNetEventHandler) OnProcessMsg(msgque netmgr.IMsgQue, mes *msg.Message) bool {
	if mes == nil {
		return true
	}

	mtype := mes.MsgType()
	if mtype == msgbase.MsgType_MSG_TYPE_RPC_REQ {
		if err := h.rpcReqAsync.PostFixedOrRand(uint64(mes.Head.RpcHashKey), mes.MessageTag(), func() {
			h.s.Rpc.RpcMessageHandler(msgque, mes)
		}); err != nil {
			h.rpcPostHandleError(mtype, mes, msgque, err)
		}
		return true
	}

	if mtype == msgbase.MsgType_MSG_TYPE_RPC_RES {
		if err := h.rpcRespAsync.PostFixedOrRand(uint64(mes.Head.RpcHashKey), mes.MessageTag(), func() {
			h.s.Rpc.RpcMessageHandler(msgque, mes)
		}); err != nil {
			h.rpcPostHandleError(mtype, mes, msgque, err)
		}
		return true
	}

	if common.CheckRouter(mes.Head.MsgId) == pb.SER_TYPE_ID_S2S {
		return h.handleS2SMessage(msgque, mes)
	}

	if h.route == nil {
		xlog.Errorf("[%v]message handler not found, message dropped", mes.Head.MsgId)
		return true
	}

	return h.route(msgque, mes)
}

func (h *ServerNetEventHandler) handleS2SMessage(msgque netmgr.IMsgQue, mes *msg.Message) bool {
	msgId := mes.Head.MsgId
	if f, ok := h.s.Router.Shandlers[mes.Head.MsgId]; ok {
		key := lo.Ternary(mes.HashKey() != 0, mes.HashKey(), msgque.SessId())
		if err := h.s.PostAsyncTask(uint64(key), mes.MessageTag(), func() {
			if xlog.LOG_LEVEL_DEBUG == xlog.GetLogLevel() {
				xlog.Debugf("handle s2s msg msgId:%v traceId:%d sessId:%d", msgId, mes.TraceId(), msgque.SessId())
			}
			f(msgque, mes)
		}); err != nil {
			xlog.Warnf("post s2s main task failed. msgId:%d traceId:%d sessId:%d err:%v",
				mes.Head.MsgId, mes.TraceId(), msgque.SessId(), err)
		}
	} else {
		xlog.Errorf("[%v]message handler not found, message dropped", mes.Head.MsgId)
		return false
	}
	return true
}

func (h *ServerNetEventHandler) rpcPostHandleError(msgType msgbase.MsgType, mes *msg.Message, msgque netmgr.IMsgQue, err error) {
	xlog.Warnf("post async task failed, rpc message dropped. msgId:%d traceId:%d sessId:%d msgType:%v err:%v",
		mes.MsgId(), mes.TraceId(), msgque.SessId(), msgType, err)

	if msgType == msgbase.MsgType_MSG_TYPE_RPC_REQ {
		rsp := &pbrpc.S2SRpcRSP{Error: &errorpb.RpcError{
			ErrCode: errorpb.ERROR_FAILED,
			ErrDesc: "task queue full"},
		}
		rpcrsp := msg.NewMsg(pb.MSG_ID_S2S_RPC_RSP, kit.PbData(rsp))
		rpcrsp.SetTraceId(mes.TraceId())
		rpcrsp.SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_RES)
		rpcrsp.SetHashKey(mes.HashKey())
		msgque.Send(rpcrsp)

	} else {
		basal.SafeGo(func() {
			server.MS.Rpc.RpcMessageHandler(msgque, mes)
		})
	}
}

func (h *ServerNetEventHandler) RegisterServerBaseHandlers() {
	server.MS.Router.RpcRegister(pb.MSG_ID_S2S_PING_REQ, rpcPingHandler)

	server.MS.Router.SSRegister(pb.MSG_ID_SAY_HELLO_REQ, reqSayHello)
	server.MS.Router.SSRegister(pb.MSG_ID_SAY_HELLO_RSP, rspServerHello)
}

func reqSayHello(msgque netmgr.IMsgQue, req *msg.Message) {
	Req, _ := req.Message().(*pb.SayHelloREQ)
	if Req == nil {
		xlog.Errorf("say hello decode failed. sessId:%d msgId:%d traceId:%d", msgque.SessId(), req.MsgId(), req.TraceId())
		return
	}
	server.MS.NetMgr.RegisterSess(Req.Type, Req.Id, msgque.SessId())
	serverInfo := &pb.SayHelloRSP{Id: server.MS.ConfBase.Server.Id, Type: server.MS.ConfBase.Server.Type}
	msgque.Send(msg.NewMsg(pb.MSG_ID_SAY_HELLO_RSP, kit.PbData(serverInfo)))
}

func rpcPingHandler(msgque netmgr.IMsgQue, msg *msg.Message) *pbrpc.S2SRpcRSP {
	req := msg.Message().(*pbrpc.S2SPingREQ)
	rsp := &pbrpc.S2SRpcRSP{
		RspType: &pbrpc.S2SRpcRSP_PingRsp{
			PingRsp: &pbrpc.S2SPingRSP{
				SrcServerId: int64(server.MS.ConfBase.Server.Id),
				TarServerId: int64(req.SrcServerId),
				TimeStr:     xtime.Now().Format("2006-01-02 15:04:05.000"),
			},
		},
	}
	xlog.Debugf("server ping serverId:%d time:%s", req.SrcServerId, req.TimeStr)
	return rsp
}

func rspServerHello(msgque netmgr.IMsgQue, req *msg.Message) {
	c2s, _ := req.Message().(*pb.SayHelloRSP)
	if c2s == nil {
		xlog.Warnf("err msg data : %v", c2s)
		return
	}
	server.MS.NetMgr.RegisterSess(c2s.Type, c2s.Id, msgque.SessId())
	xlog.Infof("[svrType:%v | svrId:%v]connect success", c2s.Type, c2s.Id)
	err := server.MS.SvrMgr.UpdateNetStatus(c2s.Type, c2s.Id, servicemgr.NetConnect)
	if err != nil {
		xlog.Errorf("[svrType:%v | svrId:%v]update service instance net status error:%v", c2s.Type, c2s.Id, err)
	}
}

func reqSyncGmTime(msgque netmgr.IMsgQue, data *msg.Message) {
	req, _ := data.Message().(*pb.S2SSyncGmTimeREQ)
	gmTime := req.GmTime
	xtime.SetGmAdd(gmTime)

	xlog.Infof("gm set time. now : %v | gm : %v", time.Now().String(), xtime.Now().String())
}
