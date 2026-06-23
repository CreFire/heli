package controller

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/src/proto/pb"
	"reflect"

	"google.golang.org/protobuf/proto"
)

type handlerFunc func(*Robot, *msg.Message)

type MsgHandler struct {
	robot *Robot
	netmgr.INetEventHandler
	Parser      msg.IParser
	msgMap      map[pb.MSG_ID]handlerFunc
	rspIdByType map[reflect.Type]pb.MSG_ID
	msgIdMap    map[pb.MSG_ID]pb.MSG_ID
}

func (mgr *MsgHandler) RegisterNtf(req pb.MSG_ID, data proto.Message, f handlerFunc) {
	mgr.msgMap[req] = f
}

func (mgr *MsgHandler) Register(req, rsp pb.MSG_ID, c2s, s2c proto.Message, f handlerFunc) {
	mgr.rspIdByType[reflect.TypeOf(c2s)] = req
	mgr.rspIdByType[reflect.TypeOf(s2c)] = rsp
	mgr.msgMap[rsp] = f
	mgr.msgIdMap[rsp] = req
}
func (mgr *MsgHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool {
	mgr.robot.msgque = msgque
	mgr.robot.SetState(STATE_LOGIN)
	return true
}

func NewMsgHandler(robot *Robot) *MsgHandler {
	handler := &MsgHandler{
		Parser:      msg.NewPBParser(),
		rspIdByType: map[reflect.Type]pb.MSG_ID{},
		robot:       robot,
		msgMap:      make(map[pb.MSG_ID]handlerFunc),
		msgIdMap:    map[pb.MSG_ID]pb.MSG_ID{},
	}
	return handler
}

func (mgr *MsgHandler) GetRspIdByType(msg proto.Message) pb.MSG_ID {
	data := reflect.TypeOf(msg)
	rspId, ok := mgr.rspIdByType[data]
	if !ok {
		return -1
	}
	return rspId
}
