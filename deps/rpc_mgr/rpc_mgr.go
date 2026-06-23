package rpcmgr

import (
	"context"
	"errors"
	"fmt"
	"game/deps/basal"
	"game/deps/fastid"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/proto/msgbase"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sasha-s/go-deadlock"
	"google.golang.org/protobuf/proto"
)

type RpcRequestHandler func(msgque netmgr.IMsgQue, msg *msg.Message) *pbrpc.S2SRpcRSP

const RPC_SERVER_HANDLE_TIMEOUT = 30
const RPC_CLIENT_WAIT_TIMEOUT = 40

type RpcResponseHandler func(error errorpb.ERROR, data *pbrpc.S2SRpcRSP)

type RpcCallInfo struct {
	rpcId     int64 //rpc id
	expireAt  int64 //过期时间
	handler   RpcResponseHandler
	receiveCh chan *pbrpc.S2SRpcRSP
	failed    func()
	free      bool
}

type RpcMgr struct {
	netmgr     *netmgr.NetMgr
	mutex      deadlock.Mutex
	rpcm       map[int64]*RpcCallInfo
	rpcRequest map[int32]RpcRequestHandler
	stopped    atomic.Bool
	cancel     context.CancelFunc
	wg         *sync.WaitGroup
}

func NewRpcMgr(nm *netmgr.NetMgr) *RpcMgr {
	return &RpcMgr{
		netmgr:     nm,
		rpcm:       make(map[int64]*RpcCallInfo),
		rpcRequest: make(map[int32]RpcRequestHandler),
		mutex:      deadlock.Mutex{},
		wg:         &sync.WaitGroup{},
	}
}

func (mgr *RpcMgr) removeRpcCall(rpcId int64, markFree bool) *RpcCallInfo {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	rpc := mgr.rpcm[rpcId]
	if rpc != nil {
		delete(mgr.rpcm, rpcId)
		if markFree {
			rpc.free = true
		}
		if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
			xlog.Debugf("RpcMgr remove rpcId:%d", rpcId)
		}
	}
	return rpc
}

func (mgr *RpcMgr) RpcRegister(msgId pb.MSG_ID, handler RpcRequestHandler) {
	mgr.rpcRequest[int32(msgId)] = handler
}

func (mgr *RpcMgr) SendRequestWithHandler(svrType string, svrId int32, m *msg.Message, handler RpcResponseHandler, failed func()) {
	if mgr.stopped.Load() {
		return
	}

	id := fastid.GenInt64ID()
	expireAt := time.Now().UnixMilli() + RPC_CLIENT_WAIT_TIMEOUT*1000

	mgr.mutex.Lock()
	if mgr.stopped.Load() {
		mgr.mutex.Unlock()
		return
	}
	mgr.rpcm[id] = &RpcCallInfo{
		rpcId:    id,
		expireAt: expireAt,
		handler:  handler,
		failed:   failed,
	}
	mgr.mutex.Unlock()

	m.SetTraceId(id)
	m.SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_REQ)
	if m.HashKey() == 0 {
		m.SetHashKey(id)
	}

	sendFail := func() {
		rpc := mgr.removeRpcCall(id, true)
		if rpc == nil {
			return
		}
		if rpc.failed != nil {
			rpc.failed()
		}
	}

	if svrId == 0 {
		mgr.netmgr.SendMsg2One(svrType, m, sendFail)
	} else {
		mgr.netmgr.SendMsg2Fix(svrType, svrId, m, sendFail)
	}
}

// If there is an atomicity requirement, check err and handle based on rsp.Error
func (mgr *RpcMgr) SendRequestWithBlock(svrType string, svrId int32, m *msg.Message, failed func()) (*pbrpc.S2SRpcRSP, error) {
	if mgr.stopped.Load() {
		return nil, errors.New("rpc mgr stop")
	}

	receiveCh := make(chan *pbrpc.S2SRpcRSP, 1)
	id := fastid.GenInt64ID()
	expireAt := time.Now().UnixMilli() + RPC_CLIENT_WAIT_TIMEOUT*1000

	mgr.mutex.Lock()
	if mgr.stopped.Load() {
		mgr.mutex.Unlock()
		return nil, errors.New("rpc mgr stop")
	}
	mgr.rpcm[id] = &RpcCallInfo{
		rpcId:     id,
		expireAt:  expireAt,
		receiveCh: receiveCh,
		failed:    failed,
	}
	mgr.mutex.Unlock()
	m.SetTraceId(id).SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_REQ)
	if m.Head.RpcHashKey == 0 {
		m.SetHashKey(id)
		xlog.Debugf("RpcMgr SendRequestWithBlock rpcId:%d use trace as rpc hash", id)
	}
	sendFail := func() {
		rpc := mgr.removeRpcCall(id, true)
		if rpc == nil {
			return
		}
		if rpc.failed != nil {
			rpc.failed()
		}
		mgr.notifyReceiveCh(id, rpc.receiveCh, &pbrpc.S2SRpcRSP{
			MsgId: m.Head.MsgId,
			Error: &errorpb.RpcError{
				ErrCode: errorpb.ERROR_RPC_SEND_FAILED,
				ErrDesc: "rpc SendRequestWithBlock failed ",
			},
		})
	}

	if svrId == 0 {
		mgr.netmgr.SendMsg2One(svrType, m, sendFail)
	} else {
		mgr.netmgr.SendMsg2Fix(svrType, svrId, m, sendFail)
	}

	select {
	case rsp, ok := <-receiveCh:
		if !ok {
			return nil, fmt.Errorf("rpc close receive channel, req msg: %v", m.ToString())
		}
		if rsp.Error != nil {
			return rsp, fmt.Errorf("rpc error, err code: %d,  err msg: %s", rsp.Error.ErrCode, rsp.Error.ErrDesc)
		}

		return rsp, nil
	case <-time.After(time.Second * RPC_CLIENT_WAIT_TIMEOUT):
		rpc := mgr.removeRpcCall(id, true)
		xlog.Warnf("RpcClient WaitTimeOut serverType: %s serverId: %d  req msg :%s", svrType, svrId, m.ToString())
		if rpc != nil && rpc.failed != nil {
			rpc.failed()
		}
		rsp := &pbrpc.S2SRpcRSP{
			MsgId: m.Head.MsgId,
			Error: &errorpb.RpcError{
				ErrCode: errorpb.ERROR_RPC_WAIT_TIMEOUT,
				ErrDesc: "rpc SendRequestWithBlock failed ",
			},
		}
		return rsp, errors.New("RPC_TIMEOUT")
	}
}

func (mgr *RpcMgr) RpcResponseHandler(rpcId int64, rsp *msg.Message) {
	mgr.mutex.Lock()
	rpc := mgr.rpcm[rpcId]
	if rpc == nil {
		xlog.Warnf("rpc call not found. rpcId:%d ,rsp %v", rpcId, rsp.ToString())
		mgr.mutex.Unlock()
		return
	}
	delete(mgr.rpcm, rpcId)
	mgr.mutex.Unlock()

	if rpc.expireAt < time.Now().UnixMilli() {
		xlog.Warnf("rpc call timeout. rpcId:%d ,rsp %v", rpcId, rsp)
		return
	}

	rpcRsp := mgr.parseRpcResponse(rsp)
	if rpc.handler != nil {
		errCode := errorpb.ERROR(rsp.Head.ErrCode)
		if rpcRsp != nil && rpcRsp.Error != nil {
			errCode = rpcRsp.Error.ErrCode
		}
		rpc.handler(errCode, rpcRsp)
	} else if rpc.receiveCh != nil {
		if rpc.free {
			xlog.Warnf("rpc call free. rpcId:%d ,rsp %v", rpcId, rsp)
			return
		}
		mgr.notifyReceiveCh(rpcId, rpc.receiveCh, rpcRsp)
	}
}

func (mgr *RpcMgr) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	mgr.cancel = cancel
	mgr.wg.Add(1)
	basal.SafeGo(func() {
		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()
		defer mgr.wg.Done()
		for {
			select {
			case <-ctx.Done():
				xlog.Infof("RpcMgr stop")
				return

			case <-ticker.C:
				mgr.mutex.Lock()
				rs := make([]*RpcCallInfo, 0, 32)
				for rpcId, rpc := range mgr.rpcm {
					if rpc.expireAt < time.Now().UnixMilli() {
						delete(mgr.rpcm, rpcId)
						xlog.Debugf("RpcMgr Update delete rpcId:%v", rpcId)
						rs = append(rs, rpc)
						rpc.free = true
					}
				}
				mgr.mutex.Unlock()

				for _, rpc := range rs {
					if rpc.failed != nil {
						rpc.failed()
					}
				}
			}
		}
	})
}
func (mgr *RpcMgr) Stop() {
	if mgr.stopped.Swap(true) {
		return
	}

	mgr.mutex.Lock()
	cancel := mgr.cancel
	rs := make([]*RpcCallInfo, 0, len(mgr.rpcm))
	for _, rpc := range mgr.rpcm {
		rpc.free = true
		rs = append(rs, rpc)
	}
	mgr.rpcm = make(map[int64]*RpcCallInfo)
	mgr.mutex.Unlock()

	for _, rpc := range rs {
		if rpc.failed != nil {
			rpc.failed()
		}
		mgr.notifyReceiveCh(rpc.rpcId, rpc.receiveCh, &pbrpc.S2SRpcRSP{
			Error: &errorpb.RpcError{
				ErrCode: errorpb.ERROR_RPC_SEND_FAILED,
				ErrDesc: "rpc mgr stop",
			},
		})
	}

	if cancel != nil {
		cancel()
	}
	mgr.wg.Wait()
}

func (mgr *RpcMgr) RpcMessageHandler(que netmgr.IMsgQue, req *msg.Message) {
	if req.MsgType() == msgbase.MsgType_MSG_TYPE_RPC_RES {
		mgr.RpcResponseHandler(req.TraceId(), req)
		return
	}

	msgid := req.MsgId()
	fn, ok := mgr.rpcRequest[msgid]
	if !ok {
		xlog.Errorf("RpcRequestMessageHandler msgId:%d no register handler", msgid)
		mgr.sendRpcErrorResponse(que, req, errorpb.ERROR_RPC_LOGIC_FAILED, "rpc req no register handler")
		return
	}
	now := xtime.NowUnixMs()
	sendTime := fastid.GetTimeMillFromFastID(req.TraceId())
	if sendTime+RPC_SERVER_HANDLE_TIMEOUT*1000 < now {
		xlog.Errorf("RpcRequestMessageHandler msgId:%d gid:%d traceId:%d  timeout", msgid, req.GId(), req.TraceId())
		mgr.sendRpcErrorResponse(que, req, errorpb.ERROR_RPC_WAIT_TIMEOUT, "rpc req server handle timeout")
		return
	}

	rsp := fn(que, req)
	if rsp == nil {
		xlog.Warnf("RpcRequestMessageHandler msgId:%d gid:%d traceId:%d rsp nil", msgid, req.GId(), req.TraceId())
		mgr.sendRpcErrorResponse(que, req, errorpb.ERROR_RPC_LOGIC_FAILED, "rpc req rsp nil")
		return
	}
	data, err := proto.Marshal(rsp)
	if err != nil {
		xlog.Warnf("RpcRequestMessageHandler proto.Marshal failed msgId:%d gid:%d traceId:%d err:%v", msgid, req.GId(), req.TraceId(), err)
		mgr.sendRpcErrorResponse(que, req, errorpb.ERROR_RPC_SERVER_RSP_ERROR, "rpc rsp marshal failed")
		return
	}
	m := msg.NewMsg(pb.MSG_ID_S2S_RPC_RSP, data)
	m.SetTraceId(req.TraceId())
	m.SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_RES)
	m.SetHashKey(req.HashKey())
	if m.Head != nil && rsp.Error != nil {
		m.Head.ErrCode = rsp.Error.ErrCode
	}

	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		xlog.Debugf("RpcRequestMessageHandler msgId:%d gid:%d traceId:%d handle time:%d ms", msgid, req.GId(), req.TraceId(), xtime.NowUnixMs()-now)
	}

	if xtime.NowUnixMs()-now > 50 {
		xlog.Warnf("RpcRequestMessageHandler msgId:%d gid:%d traceId:%d handle time:%d ms", msgid, req.GId(), req.TraceId(), xtime.NowUnixMs()-now)
	}

	que.Send(m)
}

func (mgr *RpcMgr) GetPlayLoadMessage(rsp *pbrpc.S2SRpcRSP) proto.Message {
	if rsp == nil {
		return nil
	}
	if rsp.MsgId == 0 {
		return nil
	}

	xm := msg.PbParser.GetMessage(int32(rsp.MsgId))
	if err := proto.Unmarshal(rsp.PayLoad, xm); err != nil {
		xlog.Warnf("RpcRspPlayLoadMessage proto.Unmarshal failed:%s", err.Error())
		return nil
	}
	return xm
}

func RpcTaskAddFailed(rsp *pbrpc.S2SRpcRSP) bool {
	if rsp.Error != nil {
		switch rsp.Error.ErrCode {
		case errorpb.ERROR_RPC_GAMER_TASK_ADD_FAILED:
			return true
		case errorpb.ERROR_RPC_GAMER_NOT_FOUND:
			return true
		case errorpb.ERROR_RPC_SEND_FAILED:
			return true
		}
	}

	return false
}

func (mgr *RpcMgr) notifyReceiveCh(rpcId int64, ch chan *pbrpc.S2SRpcRSP, rsp *pbrpc.S2SRpcRSP) {
	if ch == nil || rsp == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			xlog.Warnf("rpc notify channel panic rpcId:%d, panic:%v", rpcId, r)
		}
	}()
	select {
	case ch <- rsp:
	default:
		xlog.Warnf("rpc notify channel full rpcId:%d", rpcId)
	}
}

func (mgr *RpcMgr) parseRpcResponse(rsp *msg.Message) *pbrpc.S2SRpcRSP {
	if rsp == nil {
		return &pbrpc.S2SRpcRSP{
			Error: &errorpb.RpcError{
				ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR,
				ErrDesc: "rpc rsp nil",
			},
		}
	}
	rpcRsp, ok := rsp.Message().(*pbrpc.S2SRpcRSP)
	if ok && rpcRsp != nil {
		return rpcRsp
	}
	xlog.Warnf("RpcResponseHandler parse rsp failed. rpcId:%d msgId:%d", rsp.TraceId(), rsp.MsgId())
	return &pbrpc.S2SRpcRSP{
		MsgId: pb.MSG_ID(rsp.MsgId()),
		Error: &errorpb.RpcError{
			ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR,
			ErrDesc: "rpc rsp decode failed",
		},
	}
}

func (mgr *RpcMgr) sendRpcErrorResponse(que netmgr.IMsgQue, req *msg.Message, errCode errorpb.ERROR, errDesc string) {
	m := mgr.buildRpcErrorResponse(req, errCode, errDesc)
	if m == nil {
		return
	}
	que.Send(m)
}

func (mgr *RpcMgr) buildRpcErrorResponse(req *msg.Message, errCode errorpb.ERROR, errDesc string) *msg.Message {
	if req == nil {
		return nil
	}
	rsp := &pbrpc.S2SRpcRSP{
		MsgId: pb.MSG_ID(req.MsgId()),
		Error: &errorpb.RpcError{
			ErrCode: errCode,
			ErrDesc: errDesc,
		},
	}
	data, err := proto.Marshal(rsp)
	if err != nil {
		xlog.Warnf("sendRpcErrorResponse proto.Marshal failed msgId:%d traceId:%d err:%v", req.MsgId(), req.TraceId(), err)
		return nil
	}
	m := msg.NewMsg(pb.MSG_ID_S2S_RPC_RSP, data)
	m.SetTraceId(req.TraceId())
	m.SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_RES)
	m.SetHashKey(req.HashKey())
	if m.Head != nil {
		m.Head.ErrCode = errCode
	}
	return m
}
