package msghandler

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/server"
	"game/deps/xlog"
	"game/src/common"
	"game/src/proto/pb"
)

type OnDisconnect func(gamerId, sessId int64)

type ClientNetEventHandler struct {
	*NetEventBaseHandler
	s          *server.Server
	route      ClientRoutMessageHandler
	disconnect OnDisconnect
}

func NewClientNetEventHandler(s *server.Server, routeHandler ClientRoutMessageHandler, disconnectHandler OnDisconnect) *ClientNetEventHandler {
	return &ClientNetEventHandler{
		NetEventBaseHandler: &NetEventBaseHandler{},
		s:                   s,
		route:               routeHandler,
		disconnect:          disconnectHandler,
	}
}

func (c *ClientNetEventHandler) OnNewMsgQue(msgque netmgr.IMsgQue) bool {
	return true
}

func (c *ClientNetEventHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool {
	return true
}

func (c *ClientNetEventHandler) OnMsgQueStop(msgque netmgr.IMsgQue) {
	if msgque == nil || msgque.GetAgent() == nil {
		return
	}
	gamerId := msgque.GetAgent().GetCltUser()
	msgque.GetAgent().DelCltSess(func(gid int64, svrType string, svrId int32) {
		if svrType == common.InnerServerTypeLogic {
			data := msg.NewMsg(pb.MSG_ID_GATE_2_LOGIC_KICK_SESSION_REQ, nil)
			data.SetUserInfo(msgque.SessId(), gamerId)
			server.MS.NetMgr.SendMsg2Fix(svrType, svrId, data, nil) // todo
		}
	})

	c.disconnect(gamerId, msgque.SessId())
}

func (c *ClientNetEventHandler) OnProcessMsg(msgque netmgr.IMsgQue, data *msg.Message) bool {
	if msgque == nil || data == nil {
		return false
	}
	msgId := pb.MSG_ID(data.MsgId())

	svrType := common.CheckSvrType(msgId)
	if svrType == "" {
		xlog.Infof("drop client message invalid msgId=%v sessId=%d", msgId, msgque.SessId())
		return false
	}

	// if svrType == common.InnerServerTypeGate {
	// 	return c.handleC2GateMessage(msgId, msgque, data)
	// }

	if msgque.GetAgent().GetCltUser() <= 0 {
		xlog.Infof("drop client message because gamerId <= 0, sessId:%d msgId:%v", msgque.SessId(), msgId)
		return false
	}

	c.route(msgque, data)
	return true
}

// 有gate 分流调用
func (c *ClientNetEventHandler) handleC2GateMessage(msgId pb.MSG_ID, msgque netmgr.IMsgQue, data *msg.Message) bool {
	f, ok := c.s.Router.GetHandler(msgId)
	if !ok {
		xlog.Infof("drop gate message handler not found, msgId:%v sessId:%d", msgId, msgque.SessId())
		return false
	}

	runHandler := func() {
		f(msgque, data)
	}

	if msgId == pb.MSG_ID_LOGIN_BY_SESSION_REQ || msgId == pb.MSG_ID_LOGIN_RECONNECT_REQ || msgId == pb.MSG_ID_SWITCH_SERVER_REQ {
		runHandler()
	} else {
		err := server.MS.PostAsyncTask(uint64(msgque.SessId()), data.MessageTag(), runHandler)
		if err != nil {
			xlog.Warnf("post main task failed. msgId:%d msgTag:%s sessId:%d err:%v", data.MsgId(), data.MessageTag(), msgque.SessId(), err)
			return false
		}
	}
	return true
}
