package login

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/netmgr/options"
	"game/deps/proto/msgbase"
	redisclient "game/deps/redis"
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	"game/src/cache"
	"game/src/common"
	"game/src/configdoc"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
	"game/src/service/gate/gateuser"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type loginTestHandler struct {
	newCh  chan netmgr.IMsgQue
	stopCh chan netmgr.IMsgQue
}

func newLoginTestHandler() *loginTestHandler {
	return &loginTestHandler{
		newCh:  make(chan netmgr.IMsgQue, 8),
		stopCh: make(chan netmgr.IMsgQue, 8),
	}
}

func (h *loginTestHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool { return true }
func (h *loginTestHandler) OnNewMsgQue(msgque netmgr.IMsgQue) bool {
	h.newCh <- msgque
	return true
}
func (h *loginTestHandler) OnMsgQueStop(msgque netmgr.IMsgQue) {
	h.stopCh <- msgque
}
func (h *loginTestHandler) OnProcessMsg(msgque netmgr.IMsgQue, m *msg.Message) bool { return true }

type loginTestServiceRegistry struct{}

func (loginTestServiceRegistry) Register() error                               { return nil }
func (loginTestServiceRegistry) Update(inst *servicemgr.ServiceInstance) error { return nil }
func (loginTestServiceRegistry) Close()                                        {}

type loginTestServiceWatcher struct {
	ch chan *servicemgr.WatchEvent
}

func (w *loginTestServiceWatcher) Watch(ctx context.Context, _ ...servicemgr.ListenSpec) (<-chan *servicemgr.WatchEvent, error) {
	go func() {
		<-ctx.Done()
		close(w.ch)
	}()
	return w.ch, nil
}

func (w *loginTestServiceWatcher) Close() {}

func newLoginTestServiceMgr(t *testing.T, instances ...*servicemgr.ServiceInstance) *servicemgr.Manager {
	t.Helper()

	watcher := &loginTestServiceWatcher{ch: make(chan *servicemgr.WatchEvent, len(instances))}
	mgr, err := servicemgr.NewWithComponents(&servicemgr.ServiceInstance{
		ServiceName: common.InnerServerTypeGate,
		InstanceId:  1,
	}, loginTestServiceRegistry{}, watcher)
	require.NoError(t, err)
	require.NoError(t, mgr.Watch(servicemgr.ListenSpec{
		Cluster:     "test",
		ServiceName: common.InnerServerTypeLogic,
		Handler:     servicemgr.HandlerFunc{},
	}))

	for _, inst := range instances {
		watcher.ch <- &servicemgr.WatchEvent{
			Type:        servicemgr.EventTypePut,
			ServiceName: inst.ServiceName,
			InstanceID:  inst.InstanceId,
			Instance:    inst,
		}
	}
	require.Eventually(t, func() bool {
		return len(mgr.List(common.InnerServerTypeLogic, nil)) == len(instances)
	}, time.Second, 10*time.Millisecond)
	t.Cleanup(mgr.Close)
	return mgr
}

func waitLoginTestConn(t *testing.T, ch <-chan netmgr.IMsgQue, desc string) netmgr.IMsgQue {
	t.Helper()
	select {
	case mq := <-ch:
		return mq
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for conn: %s", desc)
		return nil
	}
}

func waitLoginTestStop(t *testing.T, ch <-chan netmgr.IMsgQue, desc string) netmgr.IMsgQue {
	t.Helper()
	select {
	case mq := <-ch:
		return mq
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for stop: %s", desc)
		return nil
	}
}

func readLoginTestMessage(t *testing.T, conn net.Conn) *msg.Message {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline failed: %v", err)
	}

	headSize := make([]byte, 2)
	if _, err := io.ReadFull(conn, headSize); err != nil {
		t.Fatalf("read head size failed: %v", err)
	}

	headLen := int(binary.BigEndian.Uint16(headSize))
	headBuf := make([]byte, headLen)
	if _, err := io.ReadFull(conn, headBuf); err != nil {
		t.Fatalf("read head failed: %v", err)
	}

	m := &msg.Message{}
	head, err := m.NewMessageHead(headLen, headBuf)
	if err != nil {
		t.Fatalf("parse head failed: %v", err)
	}

	if head.BodyLen > 0 {
		m.Data = make([]byte, head.BodyLen)
		if _, err := io.ReadFull(conn, m.Data); err != nil {
			t.Fatalf("read body failed: %v", err)
		}
	}
	return m
}

type loginRPCRecord struct {
	gid           int64
	playerSession int64
	deviceID      string
	isReconnect   bool
}

type loginRPCServerHandler struct {
	newCh chan netmgr.IMsgQue
	reqCh chan *loginRPCRecord
}

func newLoginRPCServerHandler() *loginRPCServerHandler {
	return &loginRPCServerHandler{
		newCh: make(chan netmgr.IMsgQue, 2),
		reqCh: make(chan *loginRPCRecord, 4),
	}
}

func (h *loginRPCServerHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool { return true }
func (h *loginRPCServerHandler) OnNewMsgQue(msgque netmgr.IMsgQue) bool {
	h.newCh <- msgque
	return true
}
func (h *loginRPCServerHandler) OnMsgQueStop(msgque netmgr.IMsgQue) {}

func (h *loginRPCServerHandler) OnProcessMsg(msgque netmgr.IMsgQue, m *msg.Message) bool {
	req := &pbrpc.S2SUserLoginREQ{}
	if err := proto.Unmarshal(m.Data, req); err != nil {
		return false
	}
	h.reqCh <- &loginRPCRecord{
		gid:           req.Gid,
		playerSession: req.PlayerSession,
		deviceID:      req.DeviceId,
		isReconnect:   req.IsReconnect,
	}

	rsp := &pbrpc.S2SRpcRSP{
		MsgId: pb.MSG_ID_S2S_USER_LOGIN_REQ,
		RspType: &pbrpc.S2SRpcRSP_UserLoginRsp{
			UserLoginRsp: &pbrpc.S2SUserLoginRSP{
				Gid:         req.Gid,
				ReconnectOk: true,
			},
		},
	}
	msgque.Send(
		msg.NewMsgWithProto(pb.MSG_ID_S2S_RPC_RSP, rsp).
			SetTraceId(m.TraceId()).
			SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_RES).
			SetHashKey(m.HashKey()),
	)
	return true
}

func startLoginLogicRPCConn(t *testing.T, mgr *netmgr.NetMgr, logicID int32, handler *loginRPCServerHandler) netmgr.IMsgQue {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	_ = ln.Close()

	listenOpt := options.NewMsgQueOptions()
	listenOpt.SetListenParams(options.NewListenParams(addr))
	require.NoError(t, mgr.StartListen(listenOpt, handler))

	connectHandler := newSwitchConnectHandler(server.MS.Rpc)
	opt := options.NewMsgQueOptions()
	opt.SetConnectParams(options.NewConnectParams(addr, common.InnerServerTypeLogic, logicID))
	require.NoError(t, mgr.StartConnect(opt, connectHandler))

	sess := waitLoginTestConn(t, connectHandler.connectCh, "logic connect")
	waitLoginTestConn(t, handler.newCh, "logic accept")
	mgr.RegisterSess(common.InnerServerTypeLogic, logicID, sess.SessId())
	time.Sleep(50 * time.Millisecond)
	return sess
}

func TestReqGamerLogin(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	gateuser.UserMgr = gateuser.NewGateUserMgr()

	mgr := netmgr.NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rpcMgr := rpcmgr.NewRpcMgr(mgr)
	svrMgr := newLoginTestServiceMgr(t, &servicemgr.ServiceInstance{
		ServiceName: common.InnerServerTypeLogic,
		InstanceId:  2,
		Enable:      true,
		Healthy:     servicemgr.ServiceStatusHealth,
		NetStatus:   servicemgr.NetConnect,
	})

	server.MS = &server.Server{
		NetMgr:  mgr,
		Rpc:     rpcMgr,
		SvrMgr:  svrMgr,
		RedisDB: &redisclient.RedisClient{Client: rc},
		Router:  router.NewRouter(rpcMgr),
		ConfBase: &configdoc.ConfigBase{
			Server: &configdoc.ServerCfg{Id: 1},
		},
	}
	defer func() {
		_ = rc.Close()
		mr.Close()
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	logicHandler := newLoginRPCServerHandler()
	_ = startLoginLogicRPCConn(t, mgr, 2, logicHandler)

	clientHandler := newLoginTestHandler()
	opt := options.NewMsgQueOptions()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	_ = ln.Close()
	opt.SetListenParams(options.NewListenParams(addr))
	require.NoError(t, mgr.StartListen(opt, clientHandler))

	clientConn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer clientConn.Close()

	clientMq := waitLoginTestConn(t, clientHandler.newCh, "login client")
	const gid int64 = common.GID_MIN + 1
	require.NoError(t, cache.SetGamerOnlineData(gid, &cache.GamerOnlineData{
		GamerId:    gid,
		Account:    "login_test_account",
		AuthToken:  "session-token",
		LogicSvrId: 2,
	}))

	code, _ := reqGamerLogin(clientMq, msg.NewMsgWithProto(pb.MSG_ID_LOGIN_BY_SESSION_REQ, &pb.LoginBySessionREQ{
		Gid:     gid,
		Session: "session-token",
		DeviceInfo: &pb.DeviceInfo{
			DeviceId: "dev-1",
		},
	}))
	require.Equal(t, errorpb.ERROR_SUCCESS, code)
}

func TestReqGamerLogout(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	gateuser.UserMgr = gateuser.NewGateUserMgr()

	mgr := netmgr.NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	server.MS = &server.Server{
		NetMgr:  mgr,
		RedisDB: &redisclient.RedisClient{Client: rc},
	}
	defer func() {
		_ = rc.Close()
		mr.Close()
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	clientHandler := newLoginTestHandler()
	opt := options.NewMsgQueOptions()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	_ = ln.Close()
	opt.SetListenParams(options.NewListenParams(addr))
	require.NoError(t, mgr.StartListen(opt, clientHandler))

	clientConn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer clientConn.Close()

	clientMq := waitLoginTestConn(t, clientHandler.newCh, "logout client")
	const gid int64 = common.GID_MIN + 1
	clientMq.GetAgent().AddCltUser(gid)
	gateuser.UserMgr.AddGateUser(gid, clientMq.SessId(), true)
	require.NoError(t, cache.SetGamerOnlineData(gid, &cache.GamerOnlineData{
		GamerId:   gid,
		Account:   "logout_test_account",
		AuthToken: "session-token",
	}))

	code, _ := reqGamerLogout(clientMq, msg.NewMsgWithProto(pb.MSG_ID_LOGIN_OUT_REQ, &pb.LoginOutREQ{}))
	require.Equal(t, errorpb.ERROR_SUCCESS, code)

	require.Equal(t, "active_logout", gateuser.UserMgr.TakeLogoutReason(gid, clientMq.SessId()))
}

func TestReqGamerReconnect(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	gateuser.UserMgr = gateuser.NewGateUserMgr()

	mgr := netmgr.NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rpcMgr := rpcmgr.NewRpcMgr(mgr)

	server.MS = &server.Server{
		NetMgr:  mgr,
		Rpc:     rpcMgr,
		RedisDB: &redisclient.RedisClient{Client: rc},
		Router:  router.NewRouter(rpcMgr),
		ConfBase: &configdoc.ConfigBase{
			Server: &configdoc.ServerCfg{Id: 1},
		},
	}
	defer func() {
		_ = rc.Close()
		mr.Close()
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	logicHandler := newLoginRPCServerHandler()
	_ = startLoginLogicRPCConn(t, mgr, 2, logicHandler)

	clientHandler := newLoginTestHandler()
	opt := options.NewMsgQueOptions()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	_ = ln.Close()
	opt.SetListenParams(options.NewListenParams(addr))
	require.NoError(t, mgr.StartListen(opt, clientHandler))

	oldConn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer oldConn.Close()
	oldMq := waitLoginTestConn(t, clientHandler.newCh, "reconnect old client")

	newConn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer newConn.Close()
	newMq := waitLoginTestConn(t, clientHandler.newCh, "reconnect new client")

	const gid int64 = common.GID_MIN + 1
	gateuser.UserMgr.AddGateUser(gid, oldMq.SessId(), true)
	require.NoError(t, cache.SetGamerOnlineData(gid, &cache.GamerOnlineData{
		GamerId:     gid,
		Account:     "reconnect_test_account",
		AuthToken:   "session-token",
		GateSession: oldMq.SessId(),
		GateSvrId:   1,
		LogicSvrId:  2,
		PubSvrId:    3,
	}))

	code, _ := reqGamerLoginReconnect(newMq, msg.NewMsgWithProto(pb.MSG_ID_LOGIN_RECONNECT_REQ, &pb.LoginReconnectREQ{
		Gid:     gid,
		Session: "session-token",
		Ack:     0,
	}))
	require.Equal(t, errorpb.ERROR_SUCCESS, code)
}

func TestKickSessionWithErrCodeKeepsLogoutReasonForKickedSession(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	server.MS = &server.Server{NetMgr: netmgr.NewNetMgr()}
	gateuser.UserMgr = gateuser.NewGateUserMgr()
	defer func() {
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	gateuser.UserMgr.AddGateUser(1001, 3003, true)

	kickSessionWithErrCode(1001, 3003, errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "new_device_login")

	require.Equal(t, "new_device_login", gateuser.UserMgr.TakeLogoutReason(1001, 3003))
}

func TestKickSessionWithErrCodeSkipsInvalidSession(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	server.MS = &server.Server{NetMgr: netmgr.NewNetMgr()}
	gateuser.UserMgr = gateuser.NewGateUserMgr()
	defer func() {
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	gateuser.UserMgr.AddGateUser(1001, 0, true)

	kickSessionWithErrCode(1001, 0, errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "test_offline_session")

	user, ok := gateuser.UserMgr.Get(1001)
	if !ok {
		t.Fatalf("expected gate user to remain cached")
	}
	if got := user.GetSessId(); got != 0 {
		t.Fatalf("expected invalid session kick to keep cached session, got %d", got)
	}
}

func TestKickSessionWithErrCodeSkipsInvalidSessionForOnlineUser(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	server.MS = &server.Server{NetMgr: netmgr.NewNetMgr()}
	gateuser.UserMgr = gateuser.NewGateUserMgr()
	defer func() {
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	gateuser.UserMgr.AddGateUser(1001, 3003, true)

	kickSessionWithErrCode(1001, 0, errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "test_invalid_online_session")

	user, ok := gateuser.UserMgr.Get(1001)
	if !ok {
		t.Fatalf("expected online gate user to remain cached")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected current session to remain, got %d", got)
	}
}

func TestKickSessionWithErrCodeKeepsGateUserUntilDisconnect(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	server.MS = &server.Server{NetMgr: netmgr.NewNetMgr()}
	gateuser.UserMgr = gateuser.NewGateUserMgr()
	defer func() {
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	gateuser.UserMgr.AddGateUser(1001, 3003, true)

	kickSessionWithErrCode(1001, 3003, errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "test_current_session")

	user, ok := gateuser.UserMgr.Get(1001)
	if !ok {
		t.Fatalf("expected current session kick to keep gate user until disconnect")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected kicked session to remain until disconnect callback, got %d", got)
	}
}

func TestKickSessionWithErrCodeClosesStaleSessionAfterTopLogin(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	mgr := netmgr.NewNetMgr()
	mgr.Start()
	defer mgr.Stop()
	server.MS = &server.Server{NetMgr: mgr}
	gateuser.UserMgr = gateuser.NewGateUserMgr()
	defer func() {
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	handler := newLoginTestHandler()
	opt := options.NewMsgQueOptions()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloc listen addr failed: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	opt.SetListenParams(options.NewListenParams(addr))
	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start listen failed: %v", err)
	}

	oldConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial old conn failed: %v", err)
	}
	defer oldConn.Close()
	oldMq := waitLoginTestConn(t, handler.newCh, "old session")
	oldMq.GetAgent().AddCltUser(1001)

	newConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial new conn failed: %v", err)
	}
	defer newConn.Close()
	newMq := waitLoginTestConn(t, handler.newCh, "new session")
	newMq.GetAgent().AddCltUser(1001)

	gateuser.UserMgr.AddGateUser(1001, newMq.SessId(), true)

	kickSessionWithErrCode(1001, oldMq.SessId(), errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "test_stale_top_login_close")

	stoppedMq := waitLoginTestStop(t, handler.stopCh, "stale session stop")
	if stoppedMq.SessId() != oldMq.SessId() {
		t.Fatalf("expected stale session %d to stop, got %d", oldMq.SessId(), stoppedMq.SessId())
	}
	user, ok := gateuser.UserMgr.Get(1001)
	if !ok {
		t.Fatalf("expected current gate user to remain cached")
	}
	if got := user.GetSessId(); got != newMq.SessId() {
		t.Fatalf("expected current session to remain, got %d", got)
	}
}

func TestKickSessionWithErrCodeSkipsStaleSessionAfterTopLogin(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	server.MS = &server.Server{NetMgr: netmgr.NewNetMgr()}
	gateuser.UserMgr = gateuser.NewGateUserMgr()
	defer func() {
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	gateuser.UserMgr.AddGateUser(1001, 3003, true)

	kickSessionWithErrCode(1001, 2002, errorpb.ERROR_KICK_OTHER_DEVICE_LOGIN, "test_stale_top_login")

	user, ok := gateuser.UserMgr.Get(1001)
	if !ok {
		t.Fatalf("expected current gate user to remain cached")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected current session to remain, got %d", got)
	}
}

func TestPickSwitchLogicTargetPrefersHighestHigherProVersion(t *testing.T) {
	target := pickSwitchLogicTargetByID(2, []*servicemgr.ServiceInstance{
		{InstanceId: 2, Enable: true, Healthy: servicemgr.ServiceStatusGray, NetStatus: servicemgr.NetConnect, ProVersion: 100},
		{InstanceId: 3, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, OnlineCount_: 1, ProVersion: 101},
		{InstanceId: 4, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, OnlineCount_: 30, ProVersion: 103},
		{InstanceId: 5, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, OnlineCount_: 5, ProVersion: 102},
	})
	if target == nil {
		t.Fatal("expected a switch target")
	}
	if target.InstanceId != 4 {
		t.Fatalf("expected instance 4, got %d", target.InstanceId)
	}
}

func TestPickSwitchLogicTargetUsesServiceInstanceProVersionField(t *testing.T) {
	target := pickSwitchLogicTargetByID(2, []*servicemgr.ServiceInstance{
		{InstanceId: 2, Enable: true, Healthy: servicemgr.ServiceStatusGray, NetStatus: servicemgr.NetConnect, ProVersion: 100},
		{InstanceId: 3, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, ProVersion: 101},
	})
	if target == nil {
		t.Fatal("expected a switch target")
	}
	if target.InstanceId != 3 {
		t.Fatalf("expected instance 3, got %d", target.InstanceId)
	}
}

func TestPickSwitchLogicTargetSkipsInvalidCandidates(t *testing.T) {
	target := pickSwitchLogicTargetByID(3, []*servicemgr.ServiceInstance{
		{InstanceId: 3, Enable: true, Healthy: servicemgr.ServiceStatusGray, NetStatus: servicemgr.NetConnect, ProVersion: 100},
		{InstanceId: 4, Enable: false, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, ProVersion: 101},
		{InstanceId: 5, Enable: true, Healthy: servicemgr.ServiceStatusGray, NetStatus: servicemgr.NetConnect, ProVersion: 101},
		{InstanceId: 6, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetUnValid, ProVersion: 101},
	})
	if target != nil {
		t.Fatalf("expected no valid target, got %d", target.InstanceId)
	}
}

func TestPickSwitchLogicTargetFallsBackToSameProVersionPeer(t *testing.T) {
	target := pickSwitchLogicTargetByID(2, []*servicemgr.ServiceInstance{
		{InstanceId: 1, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, ProVersion: 100},
		{InstanceId: 2, Enable: true, Healthy: servicemgr.ServiceStatusGray, NetStatus: servicemgr.NetConnect, ProVersion: 100},
		{InstanceId: 4, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, ProVersion: 99},
	})
	if target == nil {
		t.Fatal("expected a switch target")
	}
	if target.InstanceId != 1 {
		t.Fatalf("expected same-version peer instance 1, got %d", target.InstanceId)
	}
}

func TestPickSwitchLogicTargetUsesOldLogicProVersionAsCurrentVersion(t *testing.T) {
	target := pickSwitchLogicTargetByID(2, []*servicemgr.ServiceInstance{
		{InstanceId: 1, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, ProVersion: 101},
		{InstanceId: 2, Enable: true, Healthy: servicemgr.ServiceStatusGray, NetStatus: servicemgr.NetConnect, ProVersion: 200},
		{InstanceId: 3, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, ProVersion: 99},
	})
	if target != nil {
		t.Fatalf("expected no target when old logic version is highest, got %d", target.InstanceId)
	}
}

func TestPickSwitchLogicTargetUsesOldLogicSnapshotWhenCurrentListDroppedOldLogic(t *testing.T) {
	oldLogic := &servicemgr.ServiceInstance{InstanceId: 2, ProVersion: 100}
	target := pickSwitchLogicTarget(oldLogic, []*servicemgr.ServiceInstance{
		{InstanceId: 3, Enable: true, Healthy: servicemgr.ServiceStatusHealth, NetStatus: servicemgr.NetConnect, ProVersion: 101},
	})
	if target == nil {
		t.Fatal("expected a switch target")
	}
	if target.InstanceId != 3 {
		t.Fatalf("expected instance 3, got %d", target.InstanceId)
	}
}

func pickSwitchLogicTargetByID(oldLogicId int32, instances []*servicemgr.ServiceInstance) *servicemgr.ServiceInstance {
	for _, instance := range instances {
		if instance != nil && instance.InstanceId == oldLogicId {
			return pickSwitchLogicTarget(instance, instances)
		}
	}
	return pickSwitchLogicTarget(nil, instances)
}

type switchRPCRecord struct {
	targetLogic  int32
	isSwitchOut  bool
	gid          int64
	gamerSession int64
}

type switchRPCServerHandler struct {
	targetLogic int32
	newCh       chan netmgr.IMsgQue
	reqCh       chan *switchRPCRecord
}

func newSwitchRPCServerHandler(targetLogic int32) *switchRPCServerHandler {
	return &switchRPCServerHandler{
		targetLogic: targetLogic,
		newCh:       make(chan netmgr.IMsgQue, 2),
		reqCh:       make(chan *switchRPCRecord, 4),
	}
}

func (h *switchRPCServerHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool { return true }
func (h *switchRPCServerHandler) OnNewMsgQue(msgque netmgr.IMsgQue) bool {
	h.newCh <- msgque
	return true
}
func (h *switchRPCServerHandler) OnMsgQueStop(msgque netmgr.IMsgQue) {}

func (h *switchRPCServerHandler) OnProcessMsg(msgque netmgr.IMsgQue, m *msg.Message) bool {
	req := &pbrpc.S2SSwitchServerREQ{}
	if err := proto.Unmarshal(m.Data, req); err != nil {
		return false
	}
	h.reqCh <- &switchRPCRecord{
		targetLogic:  h.targetLogic,
		isSwitchOut:  req.IsSwitchOut,
		gid:          req.Gid,
		gamerSession: req.GamerSession,
	}

	rsp := &pbrpc.S2SRpcRSP{
		MsgId: pb.MSG_ID_S2S_SWITCH_SERVER_REQ,
		RspType: &pbrpc.S2SRpcRSP_SwitchServerRsp{
			SwitchServerRsp: &pbrpc.S2SSwitchServerRSP{},
		},
	}
	msgque.Send(
		msg.NewMsgWithProto(pb.MSG_ID_S2S_RPC_RSP, rsp).
			SetTraceId(m.TraceId()).
			SetMsgType(msgbase.MsgType_MSG_TYPE_RPC_RES).
			SetHashKey(m.HashKey()),
	)
	return true
}

type switchConnectHandler struct {
	connectCh chan netmgr.IMsgQue
	rpc       *rpcmgr.RpcMgr
}

func newSwitchConnectHandler(rpc *rpcmgr.RpcMgr) *switchConnectHandler {
	return &switchConnectHandler{
		connectCh: make(chan netmgr.IMsgQue, 2),
		rpc:       rpc,
	}
}

func (h *switchConnectHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool {
	h.connectCh <- msgque
	return true
}

func (h *switchConnectHandler) OnNewMsgQue(msgque netmgr.IMsgQue) bool { return true }
func (h *switchConnectHandler) OnMsgQueStop(msgque netmgr.IMsgQue)     {}
func (h *switchConnectHandler) OnProcessMsg(msgque netmgr.IMsgQue, m *msg.Message) bool {
	if h.rpc != nil {
		h.rpc.RpcMessageHandler(msgque, m)
	}
	return true
}

func waitSwitchRPCRecord(t *testing.T, ch <-chan *switchRPCRecord, desc string) *switchRPCRecord {
	t.Helper()
	select {
	case rec := <-ch:
		return rec
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for switch rpc: %s", desc)
		return nil
	}
}

func TestReqGamerSwitchLogicRoutesOldAndNewLogicInOneFlow(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	gateuser.UserMgr = gateuser.NewGateUserMgr()

	mgr := netmgr.NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	rpcMgr := rpcmgr.NewRpcMgr(mgr)
	svrMgr := newLoginTestServiceMgr(t,
		&servicemgr.ServiceInstance{
			ServiceName: common.InnerServerTypeLogic,
			InstanceId:  2,
			Enable:      true,
			Healthy:     servicemgr.ServiceStatusGray,
			NetStatus:   servicemgr.NetConnect,
			ProVersion:  100,
		},
		&servicemgr.ServiceInstance{
			ServiceName:  common.InnerServerTypeLogic,
			InstanceId:   4,
			Enable:       true,
			Healthy:      servicemgr.ServiceStatusHealth,
			NetStatus:    servicemgr.NetConnect,
			OnlineCount_: 1,
			ProVersion:   101,
		},
	)

	server.MS = &server.Server{
		NetMgr:   mgr,
		Rpc:      rpcMgr,
		SvrMgr:   svrMgr,
		RedisDB:  &redisclient.RedisClient{Client: rc},
		Router:   router.NewRouter(rpcMgr),
		Stopping: false,
	}
	defer func() {
		_ = rc.Close()
		mr.Close()
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	loginModule := NewLoginModule()
	if err := loginModule.RegisterHandler(rpcMgr, server.MS.Router); err != nil {
		t.Fatalf("register login handler failed: %v", err)
	}

	oldLogicHandler := newSwitchRPCServerHandler(2)
	newLogicHandler := newSwitchRPCServerHandler(4)
	oldLogicSess := startSwitchLogicRPCConn(t, mgr, 2, oldLogicHandler)
	newLogicSess := startSwitchLogicRPCConn(t, mgr, 4, newLogicHandler)
	_ = oldLogicSess
	_ = newLogicSess

	clientHandler := newLoginTestHandler()
	opt := options.NewMsgQueOptions()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloc listen addr failed: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	opt.SetListenParams(options.NewListenParams(addr))
	if err := mgr.StartListen(opt, clientHandler); err != nil {
		t.Fatalf("start client listen failed: %v", err)
	}

	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial client conn failed: %v", err)
	}
	defer clientConn.Close()

	clientMq := waitLoginTestConn(t, clientHandler.newCh, "switch client")
	const gid int64 = common.GID_MIN + 1
	clientMq.GetAgent().AddCltUser(gid)
	clientMq.GetAgent().AddCltRoute(common.InnerServerTypeLogic, 2)

	if err := cache.SetGamerOnlineData(gid, &cache.GamerOnlineData{
		GamerId:     gid,
		Account:     "switch_test_account",
		GateSession: clientMq.SessId(),
		GateSvrId:   1,
		LogicSvrId:  2,
	}); err != nil {
		t.Fatalf("seed gamer online data failed: %v", err)
	}

	handler, ok := server.MS.Router.GetHandler(pb.MSG_ID_SWITCH_SERVER_REQ)
	if !ok {
		t.Fatal("switch handler not registered")
	}

	req := msg.NewMsgWithProto(pb.MSG_ID_SWITCH_SERVER_REQ, &pb.SwitchServerREQ{}).SetUserInfo(clientMq.SessId(), gid)
	code, _ := handler(clientMq, req)
	if code != errorpb.ERROR_SUCCESS {
		t.Fatalf("expected switch handler success, got %v", code)
	}

	oldReq := waitSwitchRPCRecord(t, oldLogicHandler.reqCh, "old logic switch out")
	if oldReq.targetLogic != 2 || !oldReq.isSwitchOut {
		t.Fatalf("expected old logic switch out request, got %#v", oldReq)
	}
	if oldReq.gid != gid || oldReq.gamerSession != clientMq.SessId() {
		t.Fatalf("unexpected old logic request payload: %#v", oldReq)
	}

	newReq := waitSwitchRPCRecord(t, newLogicHandler.reqCh, "new logic switch in")
	if newReq.targetLogic != 4 || newReq.isSwitchOut {
		t.Fatalf("expected new logic switch in request, got %#v", newReq)
	}
	if newReq.gid != gid || newReq.gamerSession != clientMq.SessId() {
		t.Fatalf("unexpected new logic request payload: %#v", newReq)
	}

	online, err := cache.GetGamerOnlineData(gid)
	if err != nil {
		t.Fatalf("load gamer online data failed: %v", err)
	}
	if online.LogicSvrId != 4 {
		t.Fatalf("expected logic server switched to 4, got %d", online.LogicSvrId)
	}
}

func TestReqGamerSwitchLogicRespondsToCurrentSessionWithoutRequestSessHead(t *testing.T) {
	oldMS := server.MS
	oldUserMgr := gateuser.UserMgr
	gateuser.UserMgr = gateuser.NewGateUserMgr()

	mgr := netmgr.NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	svrMgr := newLoginTestServiceMgr(t,
		&servicemgr.ServiceInstance{
			ServiceName: common.InnerServerTypeLogic,
			InstanceId:  2,
			Enable:      true,
			Healthy:     servicemgr.ServiceStatusGray,
			NetStatus:   servicemgr.NetConnect,
			ProVersion:  100,
		},
		&servicemgr.ServiceInstance{
			ServiceName: common.InnerServerTypeLogic,
			InstanceId:  4,
			Enable:      true,
			Healthy:     servicemgr.ServiceStatusHealth,
			NetStatus:   servicemgr.NetConnect,
			ProVersion:  101,
		},
	)

	rpcMgr := rpcmgr.NewRpcMgr(mgr)
	server.MS = &server.Server{
		NetMgr:   mgr,
		Rpc:      rpcMgr,
		SvrMgr:   svrMgr,
		RedisDB:  &redisclient.RedisClient{Client: rc},
		Router:   router.NewRouter(rpcMgr),
		Stopping: false,
	}
	defer func() {
		_ = rc.Close()
		mr.Close()
		server.MS = oldMS
		gateuser.UserMgr = oldUserMgr
	}()

	loginModule := NewLoginModule()
	if err := loginModule.RegisterHandler(rpcMgr, server.MS.Router); err != nil {
		t.Fatalf("register login handler failed: %v", err)
	}

	_ = startSwitchLogicRPCConn(t, mgr, 2, newSwitchRPCServerHandler(2))
	_ = startSwitchLogicRPCConn(t, mgr, 4, newSwitchRPCServerHandler(4))

	clientHandler := newLoginTestHandler()
	opt := options.NewMsgQueOptions()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloc listen addr failed: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	opt.SetListenParams(options.NewListenParams(addr))
	if err := mgr.StartListen(opt, clientHandler); err != nil {
		t.Fatalf("start client listen failed: %v", err)
	}

	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial client conn failed: %v", err)
	}
	defer clientConn.Close()

	clientMq := waitLoginTestConn(t, clientHandler.newCh, "switch client")
	const gid int64 = common.GID_MIN + 2
	clientMq.GetAgent().AddCltUser(gid)
	clientMq.GetAgent().AddCltRoute(common.InnerServerTypeLogic, 2)

	if err := cache.SetGamerOnlineData(gid, &cache.GamerOnlineData{
		GamerId:     gid,
		Account:     "switch_rsp_test_account",
		GateSession: clientMq.SessId(),
		GateSvrId:   1,
		LogicSvrId:  2,
	}); err != nil {
		t.Fatalf("seed gamer online data failed: %v", err)
	}

	handler, ok := server.MS.Router.GetHandler(pb.MSG_ID_SWITCH_SERVER_REQ)
	if !ok {
		t.Fatal("switch handler not registered")
	}

	req := msg.NewMsgWithProto(pb.MSG_ID_SWITCH_SERVER_REQ, &pb.SwitchServerREQ{})
	code, _ := handler(clientMq, req)
	if code != errorpb.ERROR_SUCCESS {
		t.Fatalf("expected switch handler success, got %v", code)
	}

	rspMsg := readLoginTestMessage(t, clientConn)
	if got := pb.MSG_ID(rspMsg.MsgId()); got != pb.MSG_ID_SWITCH_SERVER_RSP {
		t.Fatalf("expected switch server rsp, got %v", got)
	}

	rsp := &pb.SwitchServerRSP{}
	if err := proto.Unmarshal(rspMsg.Data, rsp); err != nil {
		t.Fatalf("unmarshal switch server rsp failed: %v", err)
	}
	if !rsp.SwitchSuccess {
		t.Fatal("expected switch success rsp")
	}
}

func startSwitchLogicRPCConn(t *testing.T, mgr *netmgr.NetMgr, logicID int32, handler *switchRPCServerHandler) netmgr.IMsgQue {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen logic %d failed: %v", logicID, err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	listenOpt := options.NewMsgQueOptions()
	listenOpt.SetListenParams(options.NewListenParams(addr))
	if err := mgr.StartListen(listenOpt, handler); err != nil {
		t.Fatalf("start listen logic %d failed: %v", logicID, err)
	}

	connectHandler := newSwitchConnectHandler(server.MS.Rpc)
	opt := options.NewMsgQueOptions()
	opt.SetConnectParams(options.NewConnectParams(addr, common.InnerServerTypeLogic, logicID))
	if err := mgr.StartConnect(opt, connectHandler); err != nil {
		t.Fatalf("start connect logic %d failed: %v", logicID, err)
	}

	sess := waitLoginTestConn(t, connectHandler.connectCh, "logic connect")
	waitLoginTestConn(t, handler.newCh, "logic accept")
	mgr.RegisterSess(common.InnerServerTypeLogic, logicID, sess.SessId())
	time.Sleep(50 * time.Millisecond)
	return sess
}
