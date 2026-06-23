package rpcmgr

import (
	"testing"
	"time"

	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/proto/msgbase"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"

	"github.com/stretchr/testify/require"
)

func TestNotifyReceiveChClosedChannelNoPanic(t *testing.T) {
	mgr := NewRpcMgr(nil)
	ch := make(chan *pbrpc.S2SRpcRSP, 1)
	close(ch)

	mgr.notifyReceiveCh(1, ch, &pbrpc.S2SRpcRSP{
		Error: &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SEND_FAILED},
	})
}

func TestStopFailedCallbackOutOfLock(t *testing.T) {
	mgr := NewRpcMgr(nil)
	mgr.Start()

	called := make(chan struct{}, 1)
	mgr.mutex.Lock()
	mgr.rpcm[1] = &RpcCallInfo{
		rpcId:    1,
		expireAt: time.Now().Add(time.Minute).UnixMilli(),
		failed: func() {
			mgr.removeRpcCall(1, false)
			called <- struct{}{}
		},
	}
	mgr.mutex.Unlock()

	stopDone := make(chan struct{}, 1)
	go func() {
		mgr.Stop()
		stopDone <- struct{}{}
	}()

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("failed callback should not be blocked by rpc mutex")
	}

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stop should finish quickly")
	}
}

func TestStopIdempotent(t *testing.T) {
	mgr := NewRpcMgr(nil)
	mgr.Start()

	done := make(chan struct{}, 1)
	go func() {
		mgr.Stop()
		mgr.Stop()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("start/stop should be idempotent")
	}
}

func TestRpcResponseHandlerUsePayloadErrorCode(t *testing.T) {
	mgr := NewRpcMgr(nil)
	resultCh := make(chan errorpb.ERROR, 1)
	dataCh := make(chan *pbrpc.S2SRpcRSP, 1)
	mgr.rpcm[100] = &RpcCallInfo{
		rpcId:    100,
		expireAt: time.Now().Add(time.Minute).UnixMilli(),
		handler: func(err errorpb.ERROR, data *pbrpc.S2SRpcRSP) {
			resultCh <- err
			dataCh <- data
		},
	}

	rpcRsp := &pbrpc.S2SRpcRSP{
		Error: &errorpb.RpcError{
			ErrCode: errorpb.ERROR_RPC_SEND_FAILED,
			ErrDesc: "send failed",
		},
	}
	rspMsg := msg.NewMsgWithProto(pb.MSG_ID_S2S_RPC_RSP, rpcRsp)
	rspMsg.SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_RES)
	if rspMsg.Head != nil {
		rspMsg.Head.ErrCode = errorpb.ERROR_SUCCESS
	}

	mgr.RpcResponseHandler(100, rspMsg)

	select {
	case err := <-resultCh:
		require.Equal(t, errorpb.ERROR_RPC_SEND_FAILED, err)
	case <-time.After(2 * time.Second):
		t.Fatal("rpc handler callback not called")
	}

	select {
	case data := <-dataCh:
		require.NotNil(t, data)
		require.NotNil(t, data.Error)
		require.Equal(t, errorpb.ERROR_RPC_SEND_FAILED, data.Error.ErrCode)
	case <-time.After(2 * time.Second):
		t.Fatal("rpc handler callback data not received")
	}
}

func TestRpcResponseHandlerBadPayloadFallbackError(t *testing.T) {
	mgr := NewRpcMgr(nil)
	resultCh := make(chan errorpb.ERROR, 1)
	dataCh := make(chan *pbrpc.S2SRpcRSP, 1)
	mgr.rpcm[200] = &RpcCallInfo{
		rpcId:    200,
		expireAt: time.Now().Add(time.Minute).UnixMilli(),
		handler: func(err errorpb.ERROR, data *pbrpc.S2SRpcRSP) {
			resultCh <- err
			dataCh <- data
		},
	}

	badRsp := msg.NewMsgWithProto(pb.MSG_ID_SAY_HELLO_RSP, &pb.SayHelloRSP{})
	badRsp.SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_RES)

	mgr.RpcResponseHandler(200, badRsp)

	select {
	case err := <-resultCh:
		require.Equal(t, errorpb.ERROR_RPC_SERVER_RSP_ERROR, err)
	case <-time.After(2 * time.Second):
		t.Fatal("rpc bad payload callback not called")
	}

	select {
	case data := <-dataCh:
		require.NotNil(t, data)
		require.NotNil(t, data.Error)
		require.Equal(t, errorpb.ERROR_RPC_SERVER_RSP_ERROR, data.Error.ErrCode)
	case <-time.After(2 * time.Second):
		t.Fatal("rpc bad payload callback data not received")
	}
}

func TestBuildRpcErrorResponse(t *testing.T) {
	mgr := NewRpcMgr(nil)
	req := msg.NewMsg(pb.MSG_ID_S2S_PING_REQ, nil)
	req.SetTraceId(345)
	req.SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_REQ)
	req.SetHashKey(777)

	rspMsg := mgr.buildRpcErrorResponse(req, errorpb.ERROR_RPC_LOGIC_FAILED, "rpc no handler")
	require.NotNil(t, rspMsg)
	require.Equal(t, req.TraceId(), rspMsg.TraceId())
	require.Equal(t, msgbase.MsgType_MSG_TYPE_RPC_RES, rspMsg.MsgType())
	require.Equal(t, errorpb.ERROR_RPC_LOGIC_FAILED, rspMsg.ErrorCode())
	require.Equal(t, req.HashKey(), rspMsg.HashKey())

	rsp := rspMsg.Message().(*pbrpc.S2SRpcRSP)
	require.NotNil(t, rsp.Error)
	require.Equal(t, errorpb.ERROR_RPC_LOGIC_FAILED, rsp.Error.ErrCode)
	require.Equal(t, "rpc no handler", rsp.Error.ErrDesc)
}

func TestSendRequestWithHandlerFillRpcHashKey(t *testing.T) {
	mgr := NewRpcMgr(netmgr.NewNetMgr())
	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_PING_REQ, &pbrpc.S2SPingREQ{})
	require.Equal(t, int64(0), m.HashKey())

	mgr.SendRequestWithHandler("logic", 1, m, nil, nil)

	require.NotEqual(t, int64(0), m.HashKey())
	mgr.removeRpcCall(m.TraceId(), true)
}

func TestSendRequestWithHandlerKeepRpcHashKey(t *testing.T) {
	mgr := NewRpcMgr(netmgr.NewNetMgr())
	m := msg.NewMsgWithProto(pb.MSG_ID_S2S_PING_REQ, &pbrpc.S2SPingREQ{})
	m.SetHashKey(999)

	mgr.SendRequestWithHandler("logic", 1, m, nil, nil)

	require.Equal(t, int64(999), m.HashKey())
	mgr.removeRpcCall(m.TraceId(), true)
}
