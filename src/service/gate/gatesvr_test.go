package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"game/deps/mongoclient"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/netmgr/options"
	redisclient "game/deps/redis"
	"game/deps/router"
	rpcmgr "game/deps/rpc_mgr"
	"game/deps/server"
	servicemgr "game/deps/service_mgr"
	timermgr "game/deps/timer_mgr"
	"game/deps/xtime"
	"game/src/configdoc"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/gate/gateuser"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func newGateRouteTestMessage(sessId int64, gid int64, msgId pb.MSG_ID) *msg.Message {
	return msg.NewMsg(msgId, []byte("payload")).SetUserInfo(sessId, gid)
}

func setGateUserClientRequestHistory(user *gateuser.GateUser, times []int64) {
	user.ClientReqHead = 0
	user.ClientReqCount = len(times)
	for i := range user.ClientReqTimes {
		user.ClientReqTimes[i] = 0
	}
	copy(user.ClientReqTimes[:], times)
}

type gateStopTestHandler struct {
	newCh  chan netmgr.IMsgQue
	stopCh chan netmgr.IMsgQue
}

func newGateStopTestHandler() *gateStopTestHandler {
	return &gateStopTestHandler{
		newCh:  make(chan netmgr.IMsgQue, 4),
		stopCh: make(chan netmgr.IMsgQue, 4),
	}
}

func (h *gateStopTestHandler) OnConnectSuccess(msgque netmgr.IMsgQue) bool { return true }
func (h *gateStopTestHandler) OnNewMsgQue(msgque netmgr.IMsgQue) bool {
	h.newCh <- msgque
	return true
}
func (h *gateStopTestHandler) OnMsgQueStop(msgque netmgr.IMsgQue) { h.stopCh <- msgque }
func (h *gateStopTestHandler) OnProcessMsg(msgque netmgr.IMsgQue, m *msg.Message) bool {
	return true
}

func waitGateStopConn(t *testing.T, ch <-chan netmgr.IMsgQue, desc string) netmgr.IMsgQue {
	t.Helper()
	select {
	case mq := <-ch:
		return mq
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for conn: %s", desc)
		return nil
	}
}

func waitGateStopEvent(t *testing.T, ch <-chan netmgr.IMsgQue, desc string) netmgr.IMsgQue {
	t.Helper()
	select {
	case mq := <-ch:
		return mq
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for stop: %s", desc)
		return nil
	}
}

func readGateTestMessage(t *testing.T, conn net.Conn) *msg.Message {
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

type gateTestDiscovery struct{}
type gateTestRegistry struct{}

func (d *gateTestDiscovery) Watch(ctx context.Context, servicesToWatch ...servicemgr.ListenSpec) (<-chan *servicemgr.WatchEvent, error) {
	ch := make(chan *servicemgr.WatchEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (d *gateTestDiscovery) Close() {}

func (r *gateTestRegistry) Register() error                               { return nil }
func (r *gateTestRegistry) Update(inst *servicemgr.ServiceInstance) error { return nil }
func (r *gateTestRegistry) Close()                                        {}

func allocGateTestPort(t *testing.T) int32 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloc listen addr failed: %v", err)
	}
	defer ln.Close()
	return int32(ln.Addr().(*net.TCPAddr).Port)
}

func writeGateTestFrames(t *testing.T, conn net.Conn, msgs ...*msg.Message) {
	t.Helper()
	buffer := &bytes.Buffer{}
	for _, m := range msgs {
		if _, err := m.Bytes(buffer); err != nil {
			t.Fatalf("build frame failed: %v", err)
		}
	}
	if _, err := conn.Write(buffer.Bytes()); err != nil {
		t.Fatalf("write frame failed: %v", err)
	}
}

func setupGateServerTest(t *testing.T) (*GateSvr, func()) {
	t.Helper()

	oldMS := server.MS
	oldGateSvr := gateSvr

	timerMgr := timermgr.NewTimerMgr()
	if err := timerMgr.Start(); err != nil {
		t.Fatalf("start timer mgr: %v", err)
	}
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	server.MS = &server.Server{
		NetMgr:   netmgr.NewNetMgr(),
		TimerMgr: timerMgr,
		RedisDB:  &redisclient.RedisClient{Client: rc},
	}

	g := &GateSvr{
		GateUserMgr: gateuser.NewGateUserMgr(),
	}
	gateSvr = g

	cleanup := func() {
		_ = timerMgr.Stop()
		_ = rc.Close()
		mr.Close()
		server.MS = oldMS
		gateSvr = oldGateSvr
	}
	return g, cleanup
}

func TestGateServerRouteHandlerDropsWhenSessCleared(t *testing.T) {
	g, cleanup := setupGateServerTest(t)
	defer cleanup()

	user := g.GateUserMgr.AddGateUser(1001, 2002, true)
	if !g.GateUserMgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected disconnect clear to succeed")
	}

	data := newGateRouteTestMessage(2002, 1001, pb.MSG_ID_NOTIFY_KICK_NTF)
	g.ServerRouteHandler(nil, data)

	if got := user.GetAllSendMessage().Len(); got != 0 {
		t.Fatalf("expected offline message to be dropped, got queue len %d", got)
	}
}

func TestGateServerRouteHandlerDropsWhenSessMismatch(t *testing.T) {
	g, cleanup := setupGateServerTest(t)
	defer cleanup()

	user := g.GateUserMgr.AddGateUser(1001, 3003, true)

	data := newGateRouteTestMessage(2002, 1001, pb.MSG_ID_NOTIFY_KICK_NTF)
	g.ServerRouteHandler(nil, data)

	if got := user.GetAllSendMessage().Len(); got != 0 {
		t.Fatalf("expected mismatched session message to be dropped, got queue len %d", got)
	}
}

func TestGateServerRouteHandlerSendsWhenSessMatches(t *testing.T) {
	g, cleanup := setupGateServerTest(t)
	defer cleanup()

	user := g.GateUserMgr.AddGateUser(1001, 2002, true)

	data := newGateRouteTestMessage(2002, 1001, pb.MSG_ID_NOTIFY_KICK_NTF)
	g.ServerRouteHandler(nil, data)

	if got := user.GetAllSendMessage().Len(); got != 1 {
		t.Fatalf("expected matched session message to be tracked, got queue len %d", got)
	}
}

func TestGateServerRouteHandlerNoNeedSeqAckStillDropsOffline(t *testing.T) {
	g, cleanup := setupGateServerTest(t)
	defer cleanup()

	user := g.GateUserMgr.AddGateUser(1001, 2002, true)
	g.GateUserMgr.SetNoNeedSeqAckMsgId(int(pb.MSG_ID_NOTIFY_KICK_NTF))
	if !g.GateUserMgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected disconnect clear to succeed")
	}

	data := newGateRouteTestMessage(2002, 1001, pb.MSG_ID_NOTIFY_KICK_NTF)
	g.ServerRouteHandler(nil, data)

	if got := user.GetAllSendMessage().Len(); got != 0 {
		t.Fatalf("expected offline no-ack message to be dropped, got queue len %d", got)
	}
}

func TestClientDisconnectHandlerClearsSessImmediately(t *testing.T) {
	g, cleanup := setupGateServerTest(t)
	defer cleanup()

	user := g.GateUserMgr.AddGateUser(1001, 2002, true)

	g.ClientDisconnectHandler(1001, 2002)

	if got := user.GetSessId(); got != 0 {
		t.Fatalf("expected session to be cleared immediately, got %d", got)
	}
}

func TestClientDisconnectHandlerWrongSessDoesNotClearCurrentOnlineSess(t *testing.T) {
	g, cleanup := setupGateServerTest(t)
	defer cleanup()

	user := g.GateUserMgr.AddGateUser(1001, 2002, true)

	g.ClientDisconnectHandler(1001, 3003)

	if got := user.GetSessId(); got != 2002 {
		t.Fatalf("expected current session to remain online, got %d", got)
	}
}

func TestClientRouteHandlerReturnsTooFastAfterRateLimit(t *testing.T) {
	g, cleanup := setupGateServerTest(t)
	defer cleanup()

	const (
		gid    = int64(1001)
		sessId = int64(2002)
	)
	user := g.GateUserMgr.AddGateUser(gid, sessId, true)
	now := xtime.NowUnixMs()
	history := make([]int64, 8)
	for i := range history {
		history[i] = now - 1000
	}
	setGateUserClientRequestHistory(user, history)

	for seq := uint16(1); seq <= 7; seq++ {
		limited, err := g.GateUserMgr.CheckAndUpdateSeqAckAndLimit(gid, sessId, seq, 0)
		if err != nil {
			t.Fatalf("expected seq %d to pass, err=%v", seq, err)
		}
		if limited {
			t.Fatalf("expected seq %d to stay under limit", seq)
		}
	}

	limited, err := g.GateUserMgr.CheckAndUpdateSeqAckAndLimit(gid, sessId, 16, 0)
	if err != nil {
		t.Fatalf("expected over-limit request to keep seq valid, err=%v", err)
	}
	if !limited {
		t.Fatalf("expected request after rate limit to be rejected")
	}

	rsp := msg.NewRspMsgWithProtoAndCode(pb.MSG_ID_HEART_REQ, errorpb.ERROR_TCP_C2S_TOO_FAST, nil)
	if got := pb.MSG_ID(rsp.MsgId()); got != pb.MSG_ID_HEART_RSP {
		t.Fatalf("expected heart response msg id, got %v", got)
	}
	if got := rsp.ErrorCode(); got != errorpb.ERROR_TCP_C2S_TOO_FAST {
		t.Fatalf("expected too-fast err code, got %v", got)
	}
}

func TestGateBeforeStopSendsKickNtfToOnlineClient(t *testing.T) {
	oldMS := server.MS
	oldGateSvr := gateSvr

	timerMgr := timermgr.NewTimerMgr()
	if err := timerMgr.Start(); err != nil {
		t.Fatalf("start timer mgr: %v", err)
	}
	mgr := netmgr.NewNetMgr()
	mgr.Start()

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svrMgr, err := servicemgr.NewWithComponents(&servicemgr.ServiceInstance{
		ServiceName: "gate",
		InstanceId:  1,
	}, &gateTestRegistry{}, nil)
	if err != nil {
		t.Fatalf("new service manager failed: %v", err)
	}

	server.MS = &server.Server{
		NetMgr:   mgr,
		TimerMgr: timerMgr,
		RedisDB:  &redisclient.RedisClient{Client: rc},
		SvrMgr:   svrMgr,
	}

	g := &GateSvr{
		GateUserMgr: gateuser.NewGateUserMgr(),
	}
	gateSvr = g

	defer func() {
		mgr.Stop()
		_ = timerMgr.Stop()
		_ = rc.Close()
		mr.Close()
		server.MS = oldMS
		gateSvr = oldGateSvr
	}()

	handler := newGateStopTestHandler()
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

	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial client failed: %v", err)
	}
	defer clientConn.Close()

	clientMq := waitGateStopConn(t, handler.newCh, "client")
	const (
		gid = int64(1001)
	)
	clientMq.GetAgent().AddCltUser(gid)
	g.GateUserMgr.AddGateUser(gid, clientMq.SessId(), true)

	if err := g.BeforeStop(); err != nil {
		t.Fatalf("before stop failed: %v", err)
	}

	kickMsg := readGateTestMessage(t, clientConn)
	if got := pb.MSG_ID(kickMsg.MsgId()); got != pb.MSG_ID_NOTIFY_KICK_NTF {
		t.Fatalf("expected kick ntf, got %v", got)
	}
	if got := kickMsg.ErrorCode(); got != errorpb.ERROR_KICK_SERVER_FIX {
		t.Fatalf("expected server fix err code, got %v", got)
	}

	stoppedMq := waitGateStopEvent(t, handler.stopCh, "gate stop client")
	if stoppedMq.SessId() != clientMq.SessId() {
		t.Fatalf("expected stopped sess %d, got %d", clientMq.SessId(), stoppedMq.SessId())
	}

	waitDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(waitDeadline) {
		if server.MS.NetMgr.GetSessNum() == 0 && server.MS.NetMgr.GetAcceptSessNum() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := server.MS.NetMgr.GetAcceptSessNum(); got != 0 {
		t.Fatalf("expected no accept sessions after stop, got %d", got)
	}
	if got := server.MS.NetMgr.GetSessNum(); got != 0 {
		t.Fatalf("expected no sessions to remain, got %d", got)
	}
	if _, ok := g.GateUserMgr.Get(gid); ok {
		t.Fatalf("expected gate user cache to be cleared")
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := clientConn.Read(buf); err == nil {
		t.Fatalf("expected kicked client connection to be closed")
	}
}

func TestGateOnStartTreatsClientConnAsExternal(t *testing.T) {
	oldMS := server.MS
	oldGateSvr := gateSvr

	timerMgr := timermgr.NewTimerMgr()
	if err := timerMgr.Start(); err != nil {
		t.Fatalf("start timer mgr: %v", err)
	}
	mgr := netmgr.NewNetMgr()
	mgr.Start()
	rpc := rpcmgr.NewRpcMgr(mgr)

	port := allocGateTestPort(t)
	svrMgr, err := servicemgr.NewWithComponents(
		&servicemgr.ServiceInstance{
			ClusterName: "test",
			ServiceName: "gate",
			InstanceId:  1,
			Host:        "127.0.0.1",
			Port:        port,
			Enable:      true,
			Healthy:     servicemgr.ServiceStatusHealth,
		},
		&gateTestRegistry{},
		&gateTestDiscovery{},
	)
	if err != nil {
		t.Fatalf("new service manager failed: %v", err)
	}
	server.MS = &server.Server{
		NetMgr:   mgr,
		Rpc:      rpc,
		TimerMgr: timerMgr,
		Router:   router.NewRouter(rpc),
		SvrMgr:   svrMgr,
		MongoDB:  &mongoclient.MongoClient{Clients: &mongo.Client{}},
		ConfBase: &configdoc.ConfigBase{
			Global: &configdoc.GlobalCfg{
				Mongo: &configdoc.MongoCfg{
					DbName: "gate_test",
				},
			},
			Server: &configdoc.ServerCfg{
				Id:      1,
				Type:    "gate",
				Ip:      "127.0.0.1",
				Port:    port,
				Cluster: "test",
				Net: &configdoc.Net{
					CltReadBufferSize:  64,
					CltWriteBufferSize: 128,
					CltWriteChanSize:   8,
				},
			},
		},
	}

	g := NewGateSvr()
	gateSvr = g

	defer func() {
		_ = g.OnStop()
		mgr.Stop()
		_ = timerMgr.Stop()
		server.MS = oldMS
		gateSvr = oldGateSvr
	}()

	if err := g.OnStart(); err != nil {
		t.Fatalf("gate on start failed: %v", err)
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial gate failed: %v", err)
	}
	defer clientConn.Close()

	payload := bytes.Repeat([]byte("a"), 2048)
	writeGateTestFrames(t, clientConn, msg.NewMsg(pb.MSG_ID_HEART_REQ, payload))

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := clientConn.Read(buf); err == nil {
		t.Fatalf("expected oversized gate client packet to close the connection")
	}
}
