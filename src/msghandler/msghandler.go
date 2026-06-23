package msghandler

import (
	"game/deps/msg"
	"game/deps/netmgr"
)

type NetEventBaseHandler struct{}

func (mgr *NetEventBaseHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool { return true }
func (mgr *NetEventBaseHandler) OnNewMsgQue(msgque netmgr.IMsgQue) bool      { return true }
func (mgr *NetEventBaseHandler) OnMsgQueStop(msgque netmgr.IMsgQue)          {}
func (mgr *NetEventBaseHandler) OnProcessMsg(msgque netmgr.IMsgQue, msg *msg.Message) bool {
	return true
}


