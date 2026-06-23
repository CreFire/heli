package router

import (
	"game/deps/msg"
	"game/deps/netmgr"
	rpcmgr "game/deps/rpc_mgr"
	"game/deps/xlog"
	"game/src/proto/errorpb"
	"game/src/proto/pb"

	"google.golang.org/protobuf/proto"
)

type S2SHandlerFunc func(msgque netmgr.IMsgQue, msg *msg.Message)
type C2SHandlerFunc func(msgque netmgr.IMsgQue, msg *msg.Message) (errorpb.ERROR, proto.Message)

type Router struct {
	Chandlers map[pb.MSG_ID]C2SHandlerFunc
	Shandlers map[pb.MSG_ID]S2SHandlerFunc
	rpc       *rpcmgr.RpcMgr
}

func NewRouter(rpc *rpcmgr.RpcMgr) *Router {
	return &Router{
		Chandlers: make(map[pb.MSG_ID]C2SHandlerFunc),
		Shandlers: make(map[pb.MSG_ID]S2SHandlerFunc),
		rpc:       rpc,
	}
}

func (r *Router) RpcRegister(msgId pb.MSG_ID, handler rpcmgr.RpcRequestHandler) {
	r.rpc.RpcRegister(msgId, handler)
}

func (r *Router) SSRegister(msgId pb.MSG_ID, handler S2SHandlerFunc) {
	if _, ok := r.Shandlers[msgId]; ok {
		xlog.Warnf("router register repeated msgId:%v", msgId)
	}
	r.Shandlers[msgId] = handler
}

func (r *Router) CSRegister(msgId pb.MSG_ID, handler C2SHandlerFunc) {
	if _, ok := r.Chandlers[msgId]; ok {
		xlog.Warnf("router register repeated msgId:%v", msgId)
	}
	r.Chandlers[msgId] = handler
}

func (r *Router) GetHandler(msgId pb.MSG_ID) (handler C2SHandlerFunc, ok bool) {
	handler, ok = r.Chandlers[msgId]
	return handler, ok
}

func (r *Router) Dispatch(msgque netmgr.IMsgQue, message *msg.Message) bool {
	msgId := pb.MSG_ID(message.MsgId())
	handler, ok := r.Chandlers[msgId]
	if !ok {
		return false
	}
	handler(msgque, message)
	return true
}
