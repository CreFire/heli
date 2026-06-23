package netmgr

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"game/deps/basal"
	"game/deps/encrypt"
	"game/deps/fastid"
	"game/deps/msg"
	"game/deps/netmgr/options"
	"game/deps/proto/msgbase"
	"game/deps/xlog"
	"game/src/proto/pb"

	"github.com/gorilla/websocket"
)

type recordHandler struct {
	newCh     chan IMsgQue
	connectCh chan IMsgQue
	stopCh    chan IMsgQue
	msgCh     chan *msg.Message
}

var benchDelay time.Duration

func newRecordHandler() *recordHandler {
	return &recordHandler{
		newCh:     make(chan IMsgQue, 4),
		connectCh: make(chan IMsgQue, 4),
		stopCh:    make(chan IMsgQue, 4),
		msgCh:     make(chan *msg.Message, 8),
	}
}

func (h *recordHandler) OnConnectSuccess(msgque IMsgQue) bool {
	h.connectCh <- msgque
	return true
}

func (h *recordHandler) OnNewMsgQue(msgque IMsgQue) bool {
	h.newCh <- msgque
	return true
}

func (h *recordHandler) OnMsgQueStop(msgque IMsgQue) {
	h.stopCh <- msgque
}

func (h *recordHandler) OnProcessMsg(msgque IMsgQue, m *msg.Message) bool {
	h.msgCh <- m
	return true
}

func waitForMessage(t *testing.T, ch <-chan *msg.Message, desc string) *msg.Message {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for message: %s", desc)
		return nil
	}
}

func waitForMatchingMessage(t *testing.T, ch <-chan *msg.Message, desc string, match func(*msg.Message) bool) *msg.Message {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case m := <-ch:
			if match(m) {
				return m
			}
		case <-deadline:
			t.Fatalf("timeout waiting for message: %s", desc)
			return nil
		}
	}
}

func waitForConn(t *testing.T, ch <-chan IMsgQue, desc string) IMsgQue {
	t.Helper()
	select {
	case mq := <-ch:
		return mq
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for conn: %s", desc)
		return nil
	}
}

func waitForWebSocketConn(t *testing.T, ch <-chan *websocket.Conn, desc string) *websocket.Conn {
	t.Helper()
	select {
	case conn := <-ch:
		return conn
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for websocket conn: %s", desc)
		return nil
	}
}

func waitForClosed(t *testing.T, ch <-chan struct{}, desc string) {
	t.Helper()
	select {
	case <-ch:
		return
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for close: %s", desc)
	}
}

func waitForCondition(t *testing.T, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for condition: %s", desc)
}

func setupMsgQueWithConn(t *testing.T, mgr *NetMgr, handler INetEventHandler, conn net.Conn) *tcpMsgQue {
	t.Helper()
	opt := options.NewMsgQueOptions()
	opt.WriteChanSize = 8
	mq := newTcpConnect(handler, opt)
	mq.conn = conn
	mq.discEvt = mgr.sessOverEvt

	if !mgr.runTaskSync(func() {
		mgr.addSess(mq)
		mq.Start()
	}) {
		t.Fatalf("failed to setup msgque")
	}
	return mq
}

type partialWriteConn struct {
	mu       sync.Mutex
	maxWrite int
	writes   [][]byte
}

func (c *partialWriteConn) Write(p []byte) (int, error) {
	n := len(p)
	if c.maxWrite > 0 && n > c.maxWrite {
		n = c.maxWrite
	}
	buf := make([]byte, n)
	copy(buf, p[:n])
	c.mu.Lock()
	c.writes = append(c.writes, buf)
	c.mu.Unlock()
	return n, nil
}

func (c *partialWriteConn) Read(p []byte) (int, error) { return 0, io.EOF }
func (c *partialWriteConn) Close() error               { return nil }
func (c *partialWriteConn) LocalAddr() net.Addr        { return dummyAddr("local") }
func (c *partialWriteConn) RemoteAddr() net.Addr       { return dummyAddr("remote") }
func (c *partialWriteConn) SetDeadline(t time.Time) error {
	return nil
}
func (c *partialWriteConn) SetReadDeadline(t time.Time) error {
	return nil
}
func (c *partialWriteConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type dummyAddr string

func (d dummyAddr) Network() string { return "dummy" }
func (d dummyAddr) String() string  { return string(d) }

type blockingWriteConn struct {
	closeCh chan struct{}
	wroteCh chan struct{}
	once    sync.Once
}

func newBlockingWriteConn() *blockingWriteConn {
	return &blockingWriteConn{
		closeCh: make(chan struct{}),
		wroteCh: make(chan struct{}),
	}
}

func captureDefaultLog(t *testing.T) func() string {
	t.Helper()

	originalLogger := xlog.DefaultLogger
	logPath := filepath.Join(t.TempDir(), "netmgr.log")
	xlog.DefaultLogger = xlog.NewMyLoggerWithOptions(xlog.Options{
		FilePath: logPath,
		Level:    "debug",
		Skip:     1,
		Sync:     true,
		FileOut:  true,
		StdOut:   false,
	})
	t.Cleanup(func() {
		xlog.DefaultLogger.Close()
		xlog.DefaultLogger = originalLogger
	})
	return func() string {
		_ = xlog.Sync()
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read log file failed: %v", err)
		}
		return string(data)
	}
}

func (c *blockingWriteConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.wroteCh) })
	<-c.closeCh
	return 0, io.ErrClosedPipe
}

func (c *blockingWriteConn) Read(p []byte) (int, error) {
	<-c.closeCh
	return 0, io.EOF
}

func (c *blockingWriteConn) Close() error {
	select {
	case <-c.closeCh:
	default:
		close(c.closeCh)
	}
	return nil
}

func (c *blockingWriteConn) LocalAddr() net.Addr  { return dummyAddr("local") }
func (c *blockingWriteConn) RemoteAddr() net.Addr { return dummyAddr("remote") }
func (c *blockingWriteConn) SetDeadline(t time.Time) error {
	return nil
}
func (c *blockingWriteConn) SetReadDeadline(t time.Time) error {
	return nil
}
func (c *blockingWriteConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type writeReasonConn struct {
	closeCh     chan struct{}
	readUnblock chan struct{}
	wroteCh     chan struct{}
	writeOnce   sync.Once
	readOnce    sync.Once
	closeOnce   sync.Once
}

func newWriteReasonConn() *writeReasonConn {
	return &writeReasonConn{
		closeCh:     make(chan struct{}),
		readUnblock: make(chan struct{}),
		wroteCh:     make(chan struct{}),
	}
}

func (c *writeReasonConn) Write(p []byte) (int, error) {
	c.writeOnce.Do(func() { close(c.wroteCh) })
	<-c.closeCh
	return 0, io.ErrClosedPipe
}

func (c *writeReasonConn) Read(p []byte) (int, error) {
	<-c.readUnblock
	return 0, io.EOF
}

func (c *writeReasonConn) Close() error {
	c.closeOnce.Do(func() { close(c.closeCh) })
	return nil
}

func (c *writeReasonConn) CloseRead() error {
	c.readOnce.Do(func() { close(c.readUnblock) })
	return nil
}

func (c *writeReasonConn) LocalAddr() net.Addr  { return dummyAddr("local") }
func (c *writeReasonConn) RemoteAddr() net.Addr { return dummyAddr("remote") }

func (c *writeReasonConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *writeReasonConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *writeReasonConn) SetWriteDeadline(t time.Time) error { return nil }

type blockingReadConn struct {
	started     chan struct{}
	unblock     chan struct{}
	startOnce   sync.Once
	unblockOnce sync.Once
}

func newBlockingReadConn() *blockingReadConn {
	return &blockingReadConn{
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
}

func (c *blockingReadConn) Read(p []byte) (int, error) {
	c.startOnce.Do(func() { close(c.started) })
	<-c.unblock
	return 0, io.EOF
}

func (c *blockingReadConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *blockingReadConn) Close() error {
	c.unblockOnce.Do(func() { close(c.unblock) })
	return nil
}

func (c *blockingReadConn) CloseRead() error { return nil }

func (c *blockingReadConn) LocalAddr() net.Addr  { return dummyAddr("local") }
func (c *blockingReadConn) RemoteAddr() net.Addr { return dummyAddr("remote") }

func (c *blockingReadConn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}

func (c *blockingReadConn) SetReadDeadline(t time.Time) error {
	c.unblockOnce.Do(func() { close(c.unblock) })
	return nil
}

func (c *blockingReadConn) SetWriteDeadline(t time.Time) error { return nil }

func buildMsg(msgID pb.MSG_ID, data []byte, flags int32) *msg.Message {
	m := msg.NewMsg(msgID, data)
	m.FlagBits = flags
	return m
}

func writeFrames(t *testing.T, conn net.Conn, msgs ...*msg.Message) {
	t.Helper()
	buffer := &bytes.Buffer{}
	for _, m := range msgs {
		if _, err := m.Bytes(buffer); err != nil {
			t.Fatalf("build frame failed: %v", err)
		}
	}
	if _, err := conn.Write(buffer.Bytes()); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write frame failed: %v", err)
	}
}

func msgFrameBytes(t *testing.T, msgs ...*msg.Message) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	for _, m := range msgs {
		if _, err := m.Bytes(buffer); err != nil {
			t.Fatalf("build frame failed: %v", err)
		}
	}
	return buffer.Bytes()
}

func websocketPair(t *testing.T, path string) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	if path == "" {
		path = options.DefaultWSPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen websocket pair failed: %v", err)
	}

	serverConnCh := make(chan *websocket.Conn, 1)
	errCh := make(chan error, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(req *http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			errCh <- err
			return
		}
		serverConnCh <- conn
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	clientConn, _, err := websocket.DefaultDialer.Dial("ws://"+ln.Addr().String()+path, nil)
	if err != nil {
		_ = server.Close()
		_ = ln.Close()
		t.Fatalf("dial websocket pair failed: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case err := <-errCh:
		_ = clientConn.Close()
		_ = server.Close()
		_ = ln.Close()
		t.Fatalf("accept websocket pair failed: %v", err)
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		_ = server.Close()
		_ = ln.Close()
		t.Fatalf("timeout accepting websocket pair")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = server.Close()
		_ = ln.Close()
	}
	return serverConn, clientConn, cleanup
}

func setupWSMsgQue(t *testing.T, connTyp ConnType, opt *options.NetOptions, handler INetEventHandler) (*wsMsgQue, *websocket.Conn) {
	t.Helper()
	if opt == nil {
		opt = options.NewMsgQueOptions()
	}
	if opt.WriteChanSize == 0 {
		opt.WriteChanSize = 8
	}

	serverConn, clientConn, cleanup := websocketPair(t, opt.WSPath)
	var mq *wsMsgQue
	if connTyp == ConnTypeConn {
		mq = newWsConnect(handler, opt)
		mq.conn = serverConn
	} else {
		mq = newWsAccept(serverConn, handler, opt)
	}
	mq.Start()
	t.Cleanup(func() {
		mq.stop()
		cleanup()
	})
	return mq, clientConn
}

func listenAddrForTest(t *testing.T, mq IMsgQue) string {
	t.Helper()
	switch v := mq.(type) {
	case *tcpMsgQue:
		return v.listener.Addr().String()
	case *wsMsgQue:
		return v.listener.Addr().String()
	default:
		t.Fatalf("msgque has no test listener address: %T", mq)
		return ""
	}
}

func writeWSFrameBytes(t *testing.T, conn *websocket.Conn, data []byte) {
	t.Helper()
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write websocket frame failed: %v", err)
	}
}

func wsMsgFrameBytes(t *testing.T, msgs ...*msg.Message) []byte {
	t.Helper()
	parser := msg.NewWsParser()
	buf := &bytes.Buffer{}
	for _, mx := range msgs {
		frame := parser.PackMsg(mx)
		if len(frame) == 0 {
			t.Fatalf("build websocket frame failed: msgId=%d", mx.MsgId())
		}
		buf.Write(frame)
	}
	return buf.Bytes()
}

func writeWSFrames(t *testing.T, conn *websocket.Conn, msgs ...*msg.Message) {
	t.Helper()
	writeWSFrameBytes(t, conn, wsMsgFrameBytes(t, msgs...))
}

type stubMsgQue struct {
	sessId    int64
	connTyp   ConnType
	transport options.Transport
	agt       *ConnAgt
	opt       *options.NetOptions
	handler   INetEventHandler
	stopped   atomic.Bool
	sent      []*msg.Message
	sendRet   bool
	hasRet    bool
}

func (s *stubMsgQue) SessId() int64      { return s.sessId }
func (s *stubMsgQue) GetAgent() *ConnAgt { return s.agt }
func (s *stubMsgQue) Send(m *msg.Message) (re bool) {
	s.sent = append(s.sent, m)
	if s.hasRet {
		return s.sendRet
	}
	return true
}
func (s *stubMsgQue) listen(mgr *NetMgr)           {}
func (s *stubMsgQue) connect(mgr *NetMgr)          {}
func (s *stubMsgQue) stop()                        { s.stopped.Store(true) }
func (s *stubMsgQue) GetConnectType() ConnType     { return s.connTyp }
func (s *stubMsgQue) RemoteAddr() string           { return "stub" }
func (s *stubMsgQue) remoteAddr() string           { return "stub" }
func (s *stubMsgQue) remoteIP() string             { return "stub" }
func (s *stubMsgQue) getOpt() *options.NetOptions  { return s.opt }
func (s *stubMsgQue) getHandler() INetEventHandler { return s.handler }
func (s *stubMsgQue) getTransportType() options.Transport {
	if s.transport != "" {
		return s.transport
	}
	if s.opt == nil || s.opt.Transport == "" {
		return options.TransportTCP
	}
	return s.opt.Transport
}
func (s *stubMsgQue) getDisconnectReason() string    { return "" }
func (s *stubMsgQue) getDhKey() []byte               { return nil }
func (s *stubMsgQue) setDhKey(key []byte)            {}
func (s *stubMsgQue) setOpt(opt *options.NetOptions) { s.opt = opt }

func drainTasks(mgr *NetMgr) {
	for {
		select {
		case f := <-mgr.taskChan:
			f()
		default:
			return
		}
	}
}

func TestTcpMsgQueHandshakeSharedSecret(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	opt := options.NewMsgQueOptions()
	server := &tcpMsgQue{msgQue: msgQue{opt: opt}, quit: make(chan struct{})}
	client := &tcpMsgQue{msgQue: msgQue{opt: opt}, quit: make(chan struct{})}

	type result struct {
		key []byte
		err error
	}
	serverCh := make(chan result, 1)
	clientCh := make(chan result, 1)

	go func() {
		key, err := server.dhKeyExchange(c1)
		serverCh <- result{key: key, err: err}
	}()
	go func() {
		key, err := client.dhKeyExchangeC(c2)
		clientCh <- result{key: key, err: err}
	}()

	sr := <-serverCh
	cr := <-clientCh

	if sr.err != nil || cr.err != nil {
		t.Fatalf("handshake failed: server=%v client=%v", sr.err, cr.err)
	}
	if !bytes.Equal(sr.key, cr.key) {
		t.Fatalf("shared secret mismatch")
	}
}

func TestReadStateDefaultSize(t *testing.T) {
	state := newReadState(0)
	if state == nil {
		t.Fatalf("state is nil")
	}
	if len(state.buf) != options.DEFAULT_BUFF_SIZE {
		t.Fatalf("default buffer size mismatch: got %d", len(state.buf))
	}
}

func TestReadStateResetFrame(t *testing.T) {
	state := newReadState(16)
	state.headLen = 8
	state.bodyLen = 4
	state.msg = &msg.Message{}

	state.resetFrame()
	if state.headLen != 0 || state.bodyLen != 0 || state.msg != nil {
		t.Fatalf("resetFrame did not clear state")
	}
}

func TestReadStateGrowPreservesData(t *testing.T) {
	state := &readState{buf: make([]byte, 4)}
	copy(state.buf, []byte{1, 2, 3, 4})
	state.offset = 4

	state.grow(8)
	if len(state.buf) != 8 {
		t.Fatalf("buffer not grown")
	}
	if !bytes.Equal(state.buf[:4], []byte{1, 2, 3, 4}) {
		t.Fatalf("buffer data not preserved")
	}
}

func TestReadFromConnAppendsToBuffer(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	opt := options.NewMsgQueOptions()
	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, conn: c1, quit: make(chan struct{})}
	state := newReadState(8)
	state.buf[0] = 'x'
	state.buf[1] = 'y'
	state.offset = 2

	type result struct {
		n   int
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		n, err := mq.readFromConn(state)
		resCh <- result{n: n, err: err}
	}()

	if _, err := c2.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	res := <-resCh
	if res.err != nil {
		t.Fatalf("read failed: %v", res.err)
	}
	if res.n != 3 {
		t.Fatalf("unexpected read size: %d", res.n)
	}
	if state.offset != 5 {
		t.Fatalf("unexpected offset: %d", state.offset)
	}
	if string(state.buf[:5]) != "xyabc" {
		t.Fatalf("unexpected buffer data: %q", string(state.buf[:5]))
	}
}

func TestTcpMsgQueReadMultipleMessages(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	opt := options.NewMsgQueOptions()
	opt.ReadSize = 256
	opt.WriteChanSize = 8

	handler := newRecordHandler()
	mq := newTcpAccept(c1, handler, opt)
	mq.Start()
	defer mq.stop()

	msg1 := buildMsg(pb.MSG_ID(1), []byte("one"), 0)
	msg2 := buildMsg(pb.MSG_ID(2), []byte("two"), 0)
	writeFrames(t, c2, msg1, msg2)

	got1 := waitForMessage(t, handler.msgCh, "first")
	if got1.MsgId() != int32(msg1.MsgId()) || string(got1.Data) != "one" {
		t.Fatalf("first message mismatch")
	}
	got2 := waitForMessage(t, handler.msgCh, "second")
	if got2.MsgId() != int32(msg2.MsgId()) || string(got2.Data) != "two" {
		t.Fatalf("second message mismatch")
	}
}

func TestTcpMsgQueReadAcceptAllowsGrowthByDefault(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8

	handler := newRecordHandler()
	mq := newTcpAccept(c1, handler, opt)
	mq.Start()
	defer mq.stop()

	payload := bytes.Repeat([]byte("a"), 100)
	writeFrames(t, c2, buildMsg(pb.MSG_ID(1), payload, 0))

	got := waitForMessage(t, handler.msgCh, "accept growth")
	if got.MsgId() != int32(pb.MSG_ID(1)) || len(got.Data) != len(payload) {
		t.Fatalf("accept message mismatch")
	}
}

func TestTcpMsgQueReadGateExternalTooLarge(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8
	opt.SetIsGate(true)

	mq := newTcpAccept(c1, nil, opt)
	mq.Start()
	defer mq.stop()

	payload := bytes.Repeat([]byte("a"), 100)
	writeFrames(t, c2, buildMsg(pb.MSG_ID(1), payload, 0))

	waitForClosed(t, mq.quit, "gate external too large")
}

func TestTcpMsgQueReadInternalAllowsGrowth(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8

	handler := newRecordHandler()
	mq := newTcpConnect(handler, opt)
	mq.conn = c1
	mq.Start()
	defer mq.stop()

	payload := bytes.Repeat([]byte("b"), 150)
	writeFrames(t, c2, buildMsg(pb.MSG_ID(1), payload, 0))

	got := waitForMessage(t, handler.msgCh, "internal growth")
	if got.MsgId() != int32(pb.MSG_ID(1)) || len(got.Data) != len(payload) {
		t.Fatalf("internal message mismatch")
	}
}

func TestTcpMsgQueReadInternalTooLarge(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8

	mq := newTcpConnect(nil, opt)
	mq.conn = c1
	mq.Start()
	defer mq.stop()

	payload := bytes.Repeat([]byte("c"), 300)
	writeFrames(t, c2, buildMsg(pb.MSG_ID(1), payload, 0))

	waitForClosed(t, mq.quit, "internal too large")
}

func TestTcpMsgQueHeadLenTooLarge(t *testing.T) {
	opt := options.NewMsgQueOptions()
	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, quit: make(chan struct{})}
	state := newReadState(opt.ReadSize)

	headLen := uint16(MAX_HEAD_LEN + 1)
	state.buf[0] = byte(headLen >> 8)
	state.buf[1] = byte(headLen)
	state.offset = HEAD_SIZE

	if mq.consumeReadBuffer(state) {
		t.Fatalf("expected head len validation to fail")
	}
}

func TestTcpMsgQueConsumeReadBufferPartial(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 128
	handler := newRecordHandler()
	mq := &tcpMsgQue{msgQue: msgQue{opt: opt, handler: handler, connTyp: ConnTypeAccept, agt: newConnAgt()}, quit: make(chan struct{})}

	msg1 := buildMsg(pb.MSG_ID(1), []byte("hello"), 0)
	buf := &bytes.Buffer{}
	if _, err := msg1.Bytes(buf); err != nil {
		t.Fatalf("build frame failed: %v", err)
	}
	frame := buf.Bytes()

	state := newReadState(opt.ReadSize)
	copy(state.buf, frame[:len(frame)/2])
	state.offset = len(frame) / 2

	if !mq.consumeReadBuffer(state) {
		t.Fatalf("partial consume should not fail")
	}
	select {
	case <-handler.msgCh:
		t.Fatalf("message should not be processed yet")
	default:
	}

	copy(state.buf[state.offset:], frame[len(frame)/2:])
	state.offset += len(frame) - len(frame)/2

	if !mq.consumeReadBuffer(state) {
		t.Fatalf("full consume should succeed")
	}
	got := waitForMessage(t, handler.msgCh, "partial complete")
	if string(got.Data) != "hello" {
		t.Fatalf("message data mismatch")
	}
}

func TestTcpMsgQueFlushWriteBufferPartialWrites(t *testing.T) {
	conn := &partialWriteConn{maxWrite: 3}
	opt := options.NewMsgQueOptions()
	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, conn: conn, quit: make(chan struct{})}

	buffer := &bytes.Buffer{}
	buffer.WriteString("abcdef")

	if err := mq.flushWriteBuffer(buffer); err != nil {
		t.Fatalf("flush failed: %v", err)
	}
	if buffer.Len() != 0 {
		t.Fatalf("buffer not reset")
	}

	var combined []byte
	for _, w := range conn.writes {
		combined = append(combined, w...)
	}
	if string(combined) != "abcdef" {
		t.Fatalf("partial writes did not flush full data")
	}
}

func TestTcpMsgQueAdaptiveWriteDelayDisabled(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.DelayWrite = 10
	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, delayCtrl: newTcpWriteDelayCtrl(opt.DelayWrite, WriteChanLowWatermark), quit: make(chan struct{})}

	if d := mq.adaptiveWriteDelay(0, false); d != 0 {
		t.Fatalf("expected no delay when allowDelay is false, got %v", d)
	}

	optZero := options.NewMsgQueOptions()
	optZero.DelayWrite = 0
	mqZero := &tcpMsgQue{msgQue: msgQue{opt: optZero}, delayCtrl: newTcpWriteDelayCtrl(optZero.DelayWrite, WriteChanLowWatermark), quit: make(chan struct{})}
	if d := mqZero.adaptiveWriteDelay(0, true); d != 0 {
		t.Fatalf("expected no delay when DelayWrite is disabled, got %v", d)
	}
}

func TestTcpMsgQueAdaptiveWriteDelayAimd(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.DelayWrite = 8
	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, delayCtrl: newTcpWriteDelayCtrl(opt.DelayWrite, WriteChanLowWatermark), quit: make(chan struct{})}
	highThreshold := WriteChanLowWatermark / 3
	if highThreshold <= 0 {
		highThreshold = 1
	}

	if d := mq.adaptiveWriteDelay(0, true); d != 8*time.Millisecond {
		t.Fatalf("unexpected initial delay: %v", d)
	}

	highBatch := WriteChanLowWatermark + 1
	for range highThreshold - 1 {
		if d := mq.adaptiveWriteDelay(highBatch, true); d != 8*time.Millisecond {
			t.Fatalf("unexpected delay before high streak threshold: %v", d)
		}
	}
	if d := mq.adaptiveWriteDelay(highBatch, true); d != 4*time.Millisecond {
		t.Fatalf("expected delay to halve after high streak, got %v", d)
	}

	for range highThreshold - 1 {
		if d := mq.adaptiveWriteDelay(highBatch, true); d != 4*time.Millisecond {
			t.Fatalf("unexpected delay before second high streak threshold: %v", d)
		}
	}
	if d := mq.adaptiveWriteDelay(highBatch, true); d != 2*time.Millisecond {
		t.Fatalf("expected delay to halve again after high streak, got %v", d)
	}

	lowBatch := WriteChanLowWatermark
	for range WriteChanLowWatermark - 1 {
		if d := mq.adaptiveWriteDelay(lowBatch, true); d != 2*time.Millisecond {
			t.Fatalf("unexpected delay before low streak threshold: %v", d)
		}
	}
	if d := mq.adaptiveWriteDelay(lowBatch, true); d != 3*time.Millisecond {
		t.Fatalf("expected delay to increase slowly after low streak, got %v", d)
	}
}

func TestTcpMsgQueCollectWriteBatchTracksLastBatch(t *testing.T) {
	opt := options.NewMsgQueOptions()
	mq := &tcpMsgQue{
		msgQue: msgQue{
			opt:    opt,
			cwrite: make(chan *msg.Message, 4),
		},
		delayCtrl: newTcpWriteDelayCtrl(opt.DelayWrite, WriteChanLowWatermark),
		quit:      make(chan struct{}),
	}
	buffer := &bytes.Buffer{}

	first := buildMsg(pb.MSG_ID(1), []byte("a"), 0)
	mq.cwrite <- buildMsg(pb.MSG_ID(2), []byte("b"), 0)
	mq.cwrite <- buildMsg(pb.MSG_ID(3), []byte("c"), 0)

	if err := mq.collectWriteBatch(buffer, first, false); err != nil {
		t.Fatalf("collectWriteBatch failed: %v", err)
	}
	if mq.lastWriteBatch != 3 {
		t.Fatalf("unexpected lastWriteBatch: %d", mq.lastWriteBatch)
	}
	if len(mq.cwrite) != 0 {
		t.Fatalf("expected write queue to be drained, got %d", len(mq.cwrite))
	}
}

func TestTcpMsgQueIsInternalConn(t *testing.T) {
	internal := &tcpMsgQue{msgQue: msgQue{connTyp: ConnTypeConn, agt: newConnAgt()}, quit: make(chan struct{})}
	if !internal.isInternalConn() {
		t.Fatalf("ConnTypeConn should be internal")
	}

	acceptInternal := &tcpMsgQue{msgQue: msgQue{connTyp: ConnTypeAccept, agt: newConnAgt()}, quit: make(chan struct{})}
	if !acceptInternal.isInternalConn() {
		t.Fatalf("non-gate accept should be internal")
	}

	gateExternal := &tcpMsgQue{
		msgQue: msgQue{
			connTyp: ConnTypeAccept,
			agt:     newConnAgt(),
			opt:     (&options.NetOptions{}).SetIsGate(true),
		},
		quit: make(chan struct{}),
	}
	gateExternal.agt.AddSvrAgt("logic", 1)
	if gateExternal.isInternalConn() {
		t.Fatalf("gate listen connections should stay external")
	}
}

func TestMsgQueSendChannelFull(t *testing.T) {
	mq := &msgQue{
		cwrite: make(chan *msg.Message, 1),
		agt:    newConnAgt(),
	}

	if !mq.Send(msg.NewMsg(pb.MSG_ID(1), []byte("a"))) {
		t.Fatalf("first send should succeed")
	}
	if mq.Send(msg.NewMsg(pb.MSG_ID(2), []byte("b"))) {
		t.Fatalf("second send should fail when channel is full")
	}
}

func TestMsgQueCompressOrEncryptCompresses(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.IsGate = true
	opt.Compress = true
	opt.CompressMode = NET_COMPRESS_MODE_GZIP
	opt.CompressLimit = 1

	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, quit: make(chan struct{})}
	originalData := bytes.Repeat([]byte("a"), 1024)
	m := msg.NewMsg(pb.MSG_ID(1), originalData)

	out := mq.CompressOrEncrypt(m)
	if out == m {
		t.Fatalf("expected new message when compression is effective")
	}
	if out.FlagBits&msg.FlagCompress == 0 {
		t.Fatalf("compress flag not set")
	}
	if len(out.Data) >= len(originalData) {
		t.Fatalf("compressed data not smaller")
	}
	if !bytes.Equal(m.Data, originalData) {
		t.Fatalf("original data mutated")
	}
}

func TestMsgQueCompressOrEncryptNoBenefit(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.IsGate = true
	opt.Compress = true
	opt.CompressMode = NET_COMPRESS_MODE_GZIP
	opt.CompressLimit = 1

	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, quit: make(chan struct{})}
	data := []byte("abcdefg")
	m := msg.NewMsg(pb.MSG_ID(1), data)

	out := mq.CompressOrEncrypt(m)
	if out != m {
		t.Fatalf("expected original message when compression is not effective")
	}
}

func TestMsgQueCompressOrEncryptSkipAlreadyCompressed(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.IsGate = true
	opt.Compress = true
	opt.CompressMode = NET_COMPRESS_MODE_GZIP
	opt.CompressLimit = 1

	mq := &tcpMsgQue{msgQue: msgQue{opt: opt}, quit: make(chan struct{})}
	originalData := bytes.Repeat([]byte("a"), 1024)
	compressedData := basal.GZipCompress(originalData)
	m := msg.NewMsg(pb.MSG_ID(1), compressedData)
	m.FlagBits |= msg.FlagCompress

	out := mq.CompressOrEncrypt(m)
	if out != m {
		t.Fatalf("expected already compressed message to be sent as-is")
	}
	if !bytes.Equal(out.Data, compressedData) {
		t.Fatalf("compressed payload should stay unchanged")
	}
}

func TestMsgQueProcessMsgDecompress(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.CompressMode = NET_COMPRESS_MODE_GZIP

	handler := newRecordHandler()
	mq := &tcpMsgQue{
		msgQue: msgQue{
			connTyp: ConnTypeAccept,
			handler: handler,
			agt:     newConnAgt(),
			opt:     opt,
		},
		quit: make(chan struct{}),
	}

	original := bytes.Repeat([]byte("a"), 256)
	compressed := basal.GZipCompress(original)

	m := &msg.Message{
		Head: &msgbase.MsgHead{
			Cmd:     uint32(pb.MSG_ID(1)),
			BodyLen: int32(len(compressed)),
		},
		FlagBits: msg.FlagCompress,
		Data:     compressed,
	}

	if !mq.processMsg(mq, m) {
		t.Fatalf("process should succeed")
	}
	got := waitForMessage(t, handler.msgCh, "decompress")
	if !bytes.Equal(got.Data, original) {
		t.Fatalf("decompressed data mismatch")
	}
	if got.Head.BodyLen != int32(len(original)) {
		t.Fatalf("body len not updated")
	}
}

func TestMsgQueProcessMsgDecrypt(t *testing.T) {
	opt := options.NewMsgQueOptions()
	handler := newRecordHandler()
	mq := &tcpMsgQue{
		msgQue: msgQue{
			connTyp: ConnTypeAccept,
			handler: handler,
			agt:     newConnAgt(),
			opt:     opt,
		},
		quit: make(chan struct{}),
	}

	key := bytes.Repeat([]byte{1}, 32)
	plain := []byte("secret")
	encrypted, err := encrypt.AesEncodeData(plain, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	mq.dhKey = key

	m := &msg.Message{
		Head: &msgbase.MsgHead{
			Cmd:     uint32(pb.MSG_ID(1)),
			BodyLen: int32(len(encrypted)),
		},
		FlagBits: msg.FlagEncrypt,
		Data:     encrypted,
	}

	if !mq.processMsg(mq, m) {
		t.Fatalf("process should succeed")
	}
	got := waitForMessage(t, handler.msgCh, "decrypt")
	if !bytes.Equal(got.Data, plain) {
		t.Fatalf("decrypted data mismatch")
	}
	if got.FlagBits&msg.FlagEncrypt != 0 {
		t.Fatalf("encrypt flag not cleared")
	}
}

func TestNetMgrRegisterSess(t *testing.T) {
	mgr := NewNetMgr()
	mq := &stubMsgQue{
		sessId:  101,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
		opt:     options.NewMsgQueOptions(),
	}

	mgr.sessMap[mq.sessId] = mq
	mgr.loginPending[mq.sessId] = struct{}{}

	mgr.RegisterSess("logic", 2, mq.sessId)
	drainTasks(mgr)

	if _, ok := mgr.sessAgent["logic"][2]; !ok {
		t.Fatalf("session agent not registered")
	}
	if _, ok := mgr.loginPending[mq.sessId]; ok {
		t.Fatalf("login pending not cleared")
	}
	svrType, svrId := mq.agt.GetSvrAgt()
	if svrType != "logic" || svrId != 2 {
		t.Fatalf("agent not updated")
	}
}

func TestNetMgrHandleLoginTimeout(t *testing.T) {
	mgr := NewNetMgr()
	opt := options.NewMsgQueOptions()

	oldMillis := time.Now().Add(-loginTimeout - time.Second).UnixMilli()
	sessId := fastid.MinFastIdAt(oldMillis)
	mq := &stubMsgQue{
		sessId:  sessId,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
		opt:     opt,
	}

	mgr.addSess(mq)

	mgr.handleLoginTimeout(time.Now())

	if _, ok := mgr.sessMap[sessId]; ok {
		t.Fatalf("session not removed on timeout")
	}
	if _, ok := mgr.loginPending[sessId]; ok {
		t.Fatalf("login pending not removed on timeout")
	}
	waitForCondition(t, "session stopped on timeout", func() bool { return mq.stopped.Load() })
}

func TestNetMgrHandleLoginTimeoutWithinLimit(t *testing.T) {
	mgr := NewNetMgr()
	opt := options.NewMsgQueOptions()

	recentMillis := time.Now().Add(-time.Second).UnixMilli()
	sessId := fastid.MinFastIdAt(recentMillis)
	mq := &stubMsgQue{
		sessId:  sessId,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
		opt:     opt,
	}

	mgr.addSess(mq)

	mgr.handleLoginTimeout(time.Now())

	if _, ok := mgr.sessMap[sessId]; !ok {
		t.Fatalf("session removed unexpectedly")
	}
	if _, ok := mgr.loginPending[sessId]; !ok {
		t.Fatalf("login pending removed unexpectedly")
	}
	if mq.stopped.Load() {
		t.Fatalf("session stopped unexpectedly")
	}
}

func TestNetMgrStopFrontKeepsConnSessions(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	acceptSess := &stubMsgQue{
		sessId:  1,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
	}
	connSess := &stubMsgQue{
		sessId:  2,
		connTyp: ConnTypeConn,
		agt:     newConnAgt(),
	}

	mgr.sessMap[acceptSess.sessId] = acceptSess
	mgr.sessMap[connSess.sessId] = connSess
	mgr.incAcceptSessNum()

	mgr.StopFront()

	if !acceptSess.stopped.Load() {
		t.Fatalf("accept session should be stopped")
	}
	if connSess.stopped.Load() {
		t.Fatalf("conn session should not be stopped")
	}
	if _, ok := mgr.sessMap[acceptSess.sessId]; ok {
		t.Fatalf("accept session should be removed")
	}
	if _, ok := mgr.sessMap[connSess.sessId]; !ok {
		t.Fatalf("conn session should remain")
	}
	if mgr.GetAcceptSessNum() != 0 {
		t.Fatalf("accept session count should be zero")
	}
}

func TestNetMgrStopFrontLogsUnifiedDisconnect(t *testing.T) {
	readLog := captureDefaultLog(t)

	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	acceptSess := &stubMsgQue{
		sessId:  1,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
	}
	mgr.sessMap[acceptSess.sessId] = acceptSess
	mgr.incAcceptSessNum()

	mgr.StopFront()

	logOutput := readLog()
	if !strings.Contains(logOutput, "conn disconnect [") ||
		!strings.Contains(logOutput, "sessid=1") ||
		!strings.Contains(logOutput, "cause=stop front") ||
		!strings.Contains(logOutput, "action=local close") {
		t.Fatalf("stop front disconnect log missing unified fields, logs: %s", logOutput)
	}
}

func TestNetMgrStopFrontRejectsNewConnections(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.SetListenParams(options.NewListenParams("127.0.0.1:0"))

	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start listen failed: %v", err)
	}
	mqListen := mgr.listenMap[opt.ListenParams.ListenAddr].(*tcpMsgQue)
	addr := mqListen.listener.Addr().String()

	mgr.StopFront()
	time.Sleep(50 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("expected connection to be rejected after StopFront")
	}
}

func TestNetMgrTrackLoginPending(t *testing.T) {
	mgr := NewNetMgr()

	acceptSess := &stubMsgQue{sessId: 1, connTyp: ConnTypeAccept}
	mgr.addSess(acceptSess)

	if _, ok := mgr.loginPending[acceptSess.sessId]; !ok {
		t.Fatalf("accept session should be login pending")
	}

	connSess := &stubMsgQue{sessId: 2, connTyp: ConnTypeConn, agt: newConnAgt()}
	mgr.addSess(connSess)

	if _, ok := mgr.loginPending[connSess.sessId]; ok {
		t.Fatalf("conn session should not be login pending")
	}
}

func TestNetMgrKickSessionMatchGid(t *testing.T) {
	mgr := NewNetMgr()
	agt := newConnAgt()
	agt.AddCltUser(10)

	mq := &stubMsgQue{
		sessId:  100,
		connTyp: ConnTypeAccept,
		agt:     agt,
	}
	mgr.addSess(mq)

	mgr.KickSession(mq.sessId, 10)
	drainTasks(mgr)

	if _, ok := mgr.sessMap[mq.sessId]; ok {
		t.Fatalf("session should be removed")
	}
	waitForCondition(t, "session stopped after kick", func() bool { return mq.stopped.Load() })
}

func TestNetMgrKickSessionLogsUnifiedDisconnect(t *testing.T) {
	readLog := captureDefaultLog(t)

	mgr := NewNetMgr()
	mgr.SetLocalServer("gate", 1001)
	agt := newConnAgt()
	agt.AddCltUser(10)

	mq := &stubMsgQue{
		sessId:  100,
		connTyp: ConnTypeAccept,
		agt:     agt,
	}
	mgr.addSess(mq)

	mgr.KickSession(mq.sessId, 10)
	drainTasks(mgr)

	logOutput := readLog()
	if !strings.Contains(logOutput, "conn disconnect [") ||
		!strings.Contains(logOutput, "sessid=100 gid=10 connType=accept") ||
		!strings.Contains(logOutput, "self=gate-1001") ||
		!strings.Contains(logOutput, "cause=kick session") ||
		!strings.Contains(logOutput, "action=local close") {
		t.Fatalf("kick session disconnect log missing unified fields, logs: %s", logOutput)
	}
}

func TestNetMgrKickSessionMismatchGid(t *testing.T) {
	mgr := NewNetMgr()
	agt := newConnAgt()
	agt.AddCltUser(10)

	mq := &stubMsgQue{
		sessId:  100,
		connTyp: ConnTypeAccept,
		agt:     agt,
	}
	mgr.addSess(mq)

	mgr.KickSession(mq.sessId, 11)
	drainTasks(mgr)

	if _, ok := mgr.sessMap[mq.sessId]; !ok {
		t.Fatalf("session should remain")
	}
	if mq.stopped.Load() {
		t.Fatalf("session should not be stopped")
	}
}

func TestNetMgrKickSessionZeroGid(t *testing.T) {
	mgr := NewNetMgr()
	agt := newConnAgt()
	agt.AddCltUser(10)

	mq := &stubMsgQue{
		sessId:  100,
		connTyp: ConnTypeAccept,
		agt:     agt,
	}
	mgr.addSess(mq)

	mgr.KickSession(mq.sessId, 0)
	drainTasks(mgr)

	if _, ok := mgr.sessMap[mq.sessId]; ok {
		t.Fatalf("session should be removed")
	}
	waitForCondition(t, "session stopped after zero gid kick", func() bool { return mq.stopped.Load() })
}

func TestNetMgrRemoveSessionTwiceDoesNotOverDecrementAcceptSessNum(t *testing.T) {
	mgr := NewNetMgr()
	mq := &stubMsgQue{
		sessId:  100,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
	}
	mgr.addSess(mq)

	mgr.RemoveSession(mq.sessId)
	drainTasks(mgr)
	if mgr.GetAcceptSessNum() != 0 {
		t.Fatalf("accept session count mismatch after first remove: %d", mgr.GetAcceptSessNum())
	}

	mgr.RemoveSession(mq.sessId)
	drainTasks(mgr)
	if mgr.GetAcceptSessNum() != 0 {
		t.Fatalf("accept session count mismatch after second remove: %d", mgr.GetAcceptSessNum())
	}
}

func TestNetMgrRemoveSessionLogsUnifiedDisconnect(t *testing.T) {
	readLog := captureDefaultLog(t)

	mgr := NewNetMgr()
	mq := &stubMsgQue{
		sessId:  100,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
	}
	mgr.addSess(mq)

	mgr.RemoveSession(mq.sessId)
	drainTasks(mgr)

	logOutput := readLog()
	if !strings.Contains(logOutput, "conn disconnect [") ||
		!strings.Contains(logOutput, "sessid=100") ||
		!strings.Contains(logOutput, "cause=remove session") ||
		!strings.Contains(logOutput, "action=local close") {
		t.Fatalf("remove session disconnect log missing unified fields, logs: %s", logOutput)
	}
}

func TestNetMgrHandleLoginTimeoutConnSessClearsPendingOnly(t *testing.T) {
	mgr := NewNetMgr()
	opt := options.NewMsgQueOptions()

	oldMillis := time.Now().Add(-loginTimeout - time.Second).UnixMilli()
	connSessId := fastid.MinFastIdAt(oldMillis)
	acceptSessId := fastid.MinFastIdAt(time.Now().UnixMilli())
	acceptSess := &stubMsgQue{
		sessId:  acceptSessId,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
		opt:     opt,
	}
	connSess := &stubMsgQue{
		sessId:  connSessId,
		connTyp: ConnTypeConn,
		agt:     newConnAgt(),
		opt:     opt,
	}

	mgr.addSess(acceptSess)
	mgr.addSess(connSess)
	mgr.loginPending[connSess.sessId] = struct{}{}

	mgr.handleLoginTimeout(time.Now())

	if mgr.GetAcceptSessNum() != 1 {
		t.Fatalf("accept session count mismatch after conn timeout: %d", mgr.GetAcceptSessNum())
	}
	if _, ok := mgr.sessMap[acceptSess.sessId]; !ok {
		t.Fatalf("accept session should remain")
	}
	if _, ok := mgr.sessMap[connSess.sessId]; !ok {
		t.Fatalf("conn session should remain")
	}
	if _, ok := mgr.loginPending[connSess.sessId]; ok {
		t.Fatalf("stale conn pending entry should be cleared")
	}
	if connSess.stopped.Load() {
		t.Fatalf("conn session should not be stopped")
	}
}

func TestNetMgrLoginTimeoutLogsUnifiedDisconnect(t *testing.T) {
	readLog := captureDefaultLog(t)

	mgr := NewNetMgr()
	opt := options.NewMsgQueOptions()
	oldMillis := time.Now().Add(-loginTimeout - time.Second).UnixMilli()
	sessId := fastid.MinFastIdAt(oldMillis)
	mq := &stubMsgQue{
		sessId:  sessId,
		connTyp: ConnTypeAccept,
		agt:     newConnAgt(),
		opt:     opt,
	}
	mgr.addSess(mq)

	mgr.handleLoginTimeout(time.Now())

	logOutput := readLog()
	if !strings.Contains(logOutput, "conn disconnect [") ||
		!strings.Contains(logOutput, "cause=login timeout") ||
		!strings.Contains(logOutput, "action=local close") {
		t.Fatalf("login timeout disconnect log missing unified fields, logs: %s", logOutput)
	}
}

func TestNetMgrRemoveSvr(t *testing.T) {
	mgr := NewNetMgr()
	agt := newConnAgt()
	agt.AddSvrAgt("logic", 2)

	mq := &stubMsgQue{
		sessId:  200,
		connTyp: ConnTypeConn,
		agt:     agt,
	}
	mgr.sessMap[mq.sessId] = mq
	mgr.sessAgent["logic"] = map[int32]IMsgQue{2: mq}

	mgr.RemoveSvr("logic", 2)
	drainTasks(mgr)

	if _, ok := mgr.sessMap[mq.sessId]; ok {
		t.Fatalf("session should be removed")
	}
	waitForCondition(t, "session stopped after remove svr", func() bool { return mq.stopped.Load() })
	if _, ok := mgr.sessAgent["logic"][2]; ok {
		t.Fatalf("sessAgent entry should be removed")
	}
}

func TestNetMgrRemoveSvrLogsUnifiedDisconnect(t *testing.T) {
	readLog := captureDefaultLog(t)

	mgr := NewNetMgr()
	agt := newConnAgt()
	agt.AddSvrAgt("logic", 2)

	mq := &stubMsgQue{
		sessId:  200,
		connTyp: ConnTypeConn,
		agt:     agt,
	}
	mgr.sessMap[mq.sessId] = mq
	mgr.sessAgent["logic"] = map[int32]IMsgQue{2: mq}

	mgr.RemoveSvr("logic", 2)
	drainTasks(mgr)

	logOutput := readLog()
	if !strings.Contains(logOutput, "conn disconnect [") ||
		!strings.Contains(logOutput, "peer=logic-2") ||
		!strings.Contains(logOutput, "cause=remove server") ||
		!strings.Contains(logOutput, "action=local close") {
		t.Fatalf("remove server disconnect log missing unified fields, logs: %s", logOutput)
	}
}

func TestNetMgrAddTaskFull(t *testing.T) {
	mgr := &NetMgr{
		taskChan: make(chan func(), 1),
		stopCh:   make(chan struct{}),
	}
	mgr.taskChan <- func() {}

	if mgr.addTask(func() {}) {
		t.Fatalf("addTask should fail when channel is full")
	}
}

func TestNetMgrRunTaskSyncAfterStop(t *testing.T) {
	mgr := &NetMgr{
		taskChan: make(chan func(), 1),
		stopCh:   make(chan struct{}),
	}
	mgr.Stop()

	var called atomic.Bool
	ok := mgr.runTaskSync(func() { called.Store(true) })
	if !ok {
		t.Fatalf("runTaskSync should return true after stop")
	}
	if !called.Load() {
		t.Fatalf("task should run after stop")
	}
}

func TestNetMgrAddTaskAfterStopDoesNotQueue(t *testing.T) {
	const attempts = 128

	mgr := &NetMgr{
		taskChan: make(chan func(), attempts),
		stopCh:   make(chan struct{}),
	}
	mgr.Stop()

	for i := 0; i < attempts; i++ {
		if mgr.addTask(func() {}) {
			t.Fatalf("addTask should fail after stop")
		}
	}
	if got := len(mgr.taskChan); got != 0 {
		t.Fatalf("addTask queued %d tasks after stop", got)
	}
}

func TestNetMgrStopStateSemantics(t *testing.T) {
	msgData := msg.NewMsg(pb.MSG_ID(1), []byte("a"))
	tests := []struct {
		name string
		send func(mgr *NetMgr, fail func())
	}{
		{
			name: "SendMsg2One",
			send: func(mgr *NetMgr, fail func()) {
				mgr.SendMsg2One("logic", msgData, fail)
			},
		},
		{
			name: "SendMsg2All",
			send: func(mgr *NetMgr, fail func()) {
				mgr.SendMsg2All("logic", msgData, fail)
			},
		},
		{
			name: "SendMsg2Fix",
			send: func(mgr *NetMgr, fail func()) {
				mgr.SendMsg2Fix("logic", 1, msgData, fail)
			},
		},
		{
			name: "SendMsg2Sess",
			send: func(mgr *NetMgr, fail func()) {
				mgr.SendMsg2Sess(1, msgData, fail)
			},
		},
		{
			name: "SendMsg2AllUser",
			send: func(mgr *NetMgr, fail func()) {
				mgr.SendMsg2AllUser(msgData, fail)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewNetMgr()
			if mgr.isStopping() {
				t.Fatalf("mgr should not be stopping before Stop")
			}

			mgr.Stop()
			if !mgr.isStopping() {
				t.Fatalf("mgr should report stopping after Stop")
			}

			called := make(chan struct{}, 1)
			tt.send(mgr, func() {
				called <- struct{}{}
			})

			select {
			case <-called:
			default:
				t.Fatalf("fail callback should be called after Stop")
			}

			if got := len(mgr.taskChan); got != 0 {
				t.Fatalf("send queued %d tasks after Stop", got)
			}
		})
	}
}

func TestNetMgrStopDrainsQueuedTasks(t *testing.T) {
	const attempts = 32

	for i := 0; i < attempts; i++ {
		mgr := NewNetMgr()
		mgr.Start()

		blockStarted := make(chan struct{})
		blockRelease := make(chan struct{})
		if !mgr.addTask(func() {
			close(blockStarted)
			<-blockRelease
		}) {
			t.Fatalf("failed to queue blocking task")
		}
		waitForClosed(t, blockStarted, "blocking task started")

		taskRan := make(chan struct{})
		if !mgr.addTask(func() {
			close(taskRan)
		}) {
			t.Fatalf("failed to queue drain task")
		}

		waitForCondition(t, "drain task queued", func() bool {
			return len(mgr.taskChan) == 1
		})

		stopDone := make(chan struct{})
		go func() {
			mgr.Stop()
			close(stopDone)
		}()

		waitForCondition(t, "mgr stopping", func() bool {
			return mgr.isStopping()
		})

		close(blockRelease)
		waitForClosed(t, stopDone, "stop complete")
		waitForClosed(t, taskRan, "runTaskSync task ran")
	}
}

func TestNetMgrSendMsg2OneNoConn(t *testing.T) {
	mgr := NewNetMgr()
	called := make(chan struct{}, 1)

	mgr.SendMsg2One("logic", msg.NewMsg(pb.MSG_ID(1), []byte("x")), func() {
		called <- struct{}{}
	})
	drainTasks(mgr)

	select {
	case <-called:
	default:
		t.Fatalf("fail callback not called")
	}
}

func TestNetMgrSendMsg2All(t *testing.T) {
	mgr := NewNetMgr()
	s1 := &stubMsgQue{sessId: 1}
	s2 := &stubMsgQue{sessId: 2}
	mgr.sessAgent["logic"] = map[int32]IMsgQue{
		1: s1,
		2: s2,
	}

	mgr.SendMsg2All("logic", msg.NewMsg(pb.MSG_ID(1), []byte("a")), nil)
	drainTasks(mgr)

	if len(s1.sent) != 1 || len(s2.sent) != 1 {
		t.Fatalf("expected both sessions to receive message")
	}
}

func TestNetMgrSendMsg2One(t *testing.T) {
	mgr := NewNetMgr()
	s1 := &stubMsgQue{sessId: 1}
	s2 := &stubMsgQue{sessId: 2}
	mgr.sessAgent["logic"] = map[int32]IMsgQue{
		1: s1,
		2: s2,
	}

	mgr.SendMsg2One("logic", msg.NewMsg(pb.MSG_ID(1), []byte("a")), nil)
	drainTasks(mgr)

	got := len(s1.sent) + len(s2.sent)
	if got != 1 {
		t.Fatalf("expected one session to receive message, got %d", got)
	}
}

func TestNetMgrSendMsg2OneSendFail(t *testing.T) {
	mgr := NewNetMgr()
	s1 := &stubMsgQue{sessId: 1, hasRet: true, sendRet: false}
	mgr.sessAgent["logic"] = map[int32]IMsgQue{
		1: s1,
	}
	called := make(chan struct{}, 1)

	mgr.SendMsg2One("logic", msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})
	drainTasks(mgr)

	select {
	case <-called:
	default:
		t.Fatalf("fail callback should be called when Send returns false")
	}
}

func TestNetMgrSendMsg2OneAddTaskFail(t *testing.T) {
	mgr := &NetMgr{
		taskChan: make(chan func(), 1),
		stopCh:   make(chan struct{}),
	}
	mgr.taskChan <- func() {}
	called := make(chan struct{}, 1)

	mgr.SendMsg2One("logic", msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})

	select {
	case <-called:
	default:
		t.Fatalf("fail callback should be called when addTask fails")
	}
}

func TestNetMgrSendMsg2Fix(t *testing.T) {
	mgr := NewNetMgr()
	s1 := &stubMsgQue{sessId: 1}
	mgr.sessAgent["logic"] = map[int32]IMsgQue{
		1: s1,
	}

	mgr.SendMsg2Fix("logic", 1, msg.NewMsg(pb.MSG_ID(1), []byte("a")), nil)
	drainTasks(mgr)

	if len(s1.sent) != 1 {
		t.Fatalf("expected fixed session to receive message")
	}
}

func TestNetMgrSendMsg2FixSendFail(t *testing.T) {
	mgr := NewNetMgr()
	s1 := &stubMsgQue{sessId: 1, hasRet: true, sendRet: false}
	mgr.sessAgent["logic"] = map[int32]IMsgQue{
		1: s1,
	}
	called := make(chan struct{}, 1)

	mgr.SendMsg2Fix("logic", 1, msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})
	drainTasks(mgr)

	select {
	case <-called:
	default:
		t.Fatalf("fail callback should be called when fixed Send returns false")
	}
}

func TestNetMgrSendMsg2FixNoConn(t *testing.T) {
	mgr := NewNetMgr()
	called := make(chan struct{}, 1)

	mgr.SendMsg2Fix("logic", 1, msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})
	drainTasks(mgr)

	select {
	case <-called:
	default:
		t.Fatalf("fail callback not called")
	}
}

func TestNetMgrSendMsg2Sess(t *testing.T) {
	mgr := NewNetMgr()
	s1 := &stubMsgQue{sessId: 1}
	mgr.sessMap[1] = s1

	mgr.SendMsg2Sess(1, msg.NewMsg(pb.MSG_ID(1), []byte("a")), nil)
	drainTasks(mgr)

	if len(s1.sent) != 1 {
		t.Fatalf("expected session to receive message")
	}
}

func TestNetMgrSendMsg2SessNoConn(t *testing.T) {
	mgr := NewNetMgr()
	called := make(chan struct{}, 1)

	mgr.SendMsg2Sess(1, msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})
	drainTasks(mgr)

	select {
	case <-called:
	default:
		t.Fatalf("fail callback not called")
	}
}

func TestNetMgrSendMsg2AllUserNoPlayerNoFail(t *testing.T) {
	mgr := NewNetMgr()
	mgr.sessMap[1] = &stubMsgQue{sessId: 1}
	called := make(chan struct{}, 1)

	mgr.SendMsg2AllUser(msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})
	drainTasks(mgr)

	select {
	case <-called:
		t.Fatalf("fail callback should not be called when no player connection")
	default:
	}
}

func TestNetMgrSendMsg2AllUserAllSendFailNoFailCallback(t *testing.T) {
	mgr := NewNetMgr()
	agt := &ConnAgt{}
	agt.AddCltUser(10001)
	mgr.sessMap[1] = &stubMsgQue{
		sessId:  1,
		agt:     agt,
		hasRet:  true,
		sendRet: false,
	}
	called := make(chan struct{}, 1)

	mgr.SendMsg2AllUser(msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})
	drainTasks(mgr)

	select {
	case <-called:
		t.Fatalf("fail callback should not be called when all player sends fail")
	default:
	}
}

func TestNetMgrSendMsg2AllUserAddTaskFail(t *testing.T) {
	mgr := &NetMgr{
		taskChan: make(chan func(), 1),
		stopCh:   make(chan struct{}),
	}
	mgr.taskChan <- func() {}
	called := make(chan struct{}, 1)

	mgr.SendMsg2AllUser(msg.NewMsg(pb.MSG_ID(1), []byte("a")), func() {
		called <- struct{}{}
	})

	select {
	case <-called:
	default:
		t.Fatalf("fail callback should be called when addTask fails")
	}
}

func TestNetMgrStartListenAccept(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.SetListenParams(options.NewListenParams("127.0.0.1:0"))

	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start listen failed: %v", err)
	}
	mqListen := mgr.listenMap[opt.ListenParams.ListenAddr].(*tcpMsgQue)
	addr := mqListen.listener.Addr().String()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitForConn(t, handler.newCh, "accept")

	if mgr.GetAcceptSessNum() != 1 {
		t.Fatalf("accept session count mismatch")
	}
}

func TestNetMgrStartConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	addr := ln.Addr().String()
	opt := options.NewMsgQueOptions()
	opt.SetConnectParams(options.NewConnectParams(addr, "logic", 1))

	if err := mgr.StartConnect(opt, handler); err != nil {
		t.Fatalf("start connect failed: %v", err)
	}

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept failed: %v", err)
	}
	defer conn.Close()

	waitForConn(t, handler.connectCh, "connect")

	if mgr.GetSessNum() == 0 {
		t.Fatalf("connect session not registered")
	}
}

func TestNetMgrStartListenAcceptWebSocket(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.Transport = options.TransportWebSocket
	opt.WSPath = "/ws"
	opt.SetListenParams(options.NewListenParams("127.0.0.1:0"))

	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start websocket listen failed: %v", err)
	}

	mqListen := mgr.listenMap[opt.ListenParams.ListenAddr].(*wsMsgQue)
	addr := mqListen.listener.Addr().String()

	clientOpt := options.NewMsgQueOptions()
	clientOpt.Transport = options.TransportWebSocket
	clientOpt.WSPath = "/ws"
	clientOpt.SetConnectParams(options.NewConnectParams("ws://"+addr+"/ws", "gate", 1))

	if err := mgr.StartConnect(clientOpt, handler); err != nil {
		t.Fatalf("start websocket connect failed: %v", err)
	}

	waitForConn(t, handler.newCh, "websocket accept")
	waitForConn(t, handler.connectCh, "websocket connect")

	if mgr.GetAcceptSessNum() != 1 {
		t.Fatalf("accept session count mismatch")
	}
}

func TestNetMgrWebSocketRoundTrip(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	serverHandler := newRecordHandler()
	listenOpt := options.NewMsgQueOptions()
	listenOpt.Transport = options.TransportWebSocket
	listenOpt.WSPath = options.DefaultWSPath
	listenOpt.SetListenParams(options.NewListenParams("127.0.0.1:0"))
	if err := mgr.StartListen(listenOpt, serverHandler); err != nil {
		t.Fatalf("start websocket listen failed: %v", err)
	}

	mqListen := mgr.listenMap[listenOpt.ListenParams.ListenAddr].(*wsMsgQue)
	addr := mqListen.listener.Addr().String()

	clientHandler := newRecordHandler()
	connectOpt := options.NewMsgQueOptions()
	connectOpt.Transport = options.TransportWebSocket
	connectOpt.WSPath = options.DefaultWSPath
	connectOpt.SetConnectParams(options.NewConnectParams("ws://"+addr+"/ws", "gate", 1))
	if err := mgr.StartConnect(connectOpt, clientHandler); err != nil {
		t.Fatalf("start websocket connect failed: %v", err)
	}

	serverConn := waitForConn(t, serverHandler.newCh, "websocket accept")
	clientConn := waitForConn(t, clientHandler.connectCh, "websocket connect")

	if !clientConn.Send(msg.NewMsg(pb.MSG_ID(1), []byte("hello-ws"))) {
		t.Fatalf("client send failed")
	}
	got := waitForMessage(t, serverHandler.msgCh, "websocket server recv")
	if got.MsgId() != int32(pb.MSG_ID(1)) || string(got.Data) != "hello-ws" {
		t.Fatalf("unexpected websocket server message: msgId=%d data=%q", got.MsgId(), string(got.Data))
	}

	if !serverConn.Send(msg.NewMsg(pb.MSG_ID(2), []byte("hello-client"))) {
		t.Fatalf("server send failed")
	}
	got = waitForMessage(t, clientHandler.msgCh, "websocket client recv")
	if got.MsgId() != int32(pb.MSG_ID(2)) || string(got.Data) != "hello-client" {
		t.Fatalf("unexpected websocket client message: msgId=%d data=%q", got.MsgId(), string(got.Data))
	}
}

func TestNetMgrWebSocketIgnoresDHOption(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	serverHandler := newRecordHandler()
	listenOpt := options.NewMsgQueOptions()
	listenOpt.Transport = options.TransportWebSocket
	listenOpt.WSPath = "/ws"
	listenOpt.SetEnableDH(true)
	listenOpt.SetListenParams(options.NewListenParams("127.0.0.1:0"))
	if err := mgr.StartListen(listenOpt, serverHandler); err != nil {
		t.Fatalf("start websocket listen failed: %v", err)
	}

	mqListen := mgr.listenMap[listenOpt.ListenParams.ListenAddr].(*wsMsgQue)
	addr := mqListen.listener.Addr().String()

	clientHandler := newRecordHandler()
	connectOpt := options.NewMsgQueOptions()
	connectOpt.Transport = options.TransportWebSocket
	connectOpt.WSPath = "/ws"
	connectOpt.SetEnableDH(true)
	connectOpt.SetConnectParams(options.NewConnectParams("ws://"+addr, "gate", 1))
	if err := mgr.StartConnect(connectOpt, clientHandler); err != nil {
		t.Fatalf("start websocket connect failed: %v", err)
	}

	clientConn := waitForConn(t, clientHandler.connectCh, "websocket connect")
	waitForConn(t, serverHandler.newCh, "websocket accept")

	if !clientConn.Send(msg.NewMsg(pb.MSG_ID(1), []byte("dh-offloaded"))) {
		t.Fatalf("client send failed")
	}
	got := waitForMessage(t, serverHandler.msgCh, "websocket server recv")
	if got.MsgId() != int32(pb.MSG_ID(1)) || string(got.Data) != "dh-offloaded" {
		t.Fatalf("unexpected websocket message: msgId=%d data=%q", got.MsgId(), string(got.Data))
	}
}

func TestNetMgrWebSocketRejectsOverloadBeforeUpgrade(t *testing.T) {
	mgr := NewNetMgr()
	mgr.acceptSessNum.Store(1)
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.Transport = options.TransportWebSocket
	opt.WSPath = "/ws"
	opt.SetListenParams(options.NewListenParams("127.0.0.1:0").SetMaxConn(1))

	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start websocket listen failed: %v", err)
	}

	mqListen := mgr.listenMap[opt.ListenParams.ListenAddr].(*wsMsgQue)
	addr := mqListen.listener.Addr().String()

	_, resp, err := websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err == nil {
		t.Fatalf("overload websocket dial should fail before upgrade")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected overload status: resp=%v err=%v", resp, err)
	}

	select {
	case mq := <-handler.newCh:
		t.Fatalf("overload connection should not create msgque: %v", mq.SessId())
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWSConnectURLAppliesPathToSchemeOnlyAddr(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.Transport = options.TransportWebSocket
	opt.WSPath = "/game"
	opt.SetConnectParams(options.NewConnectParams("ws://127.0.0.1:1234", "gate", 1))

	if got := wsConnectURL(opt); got != "ws://127.0.0.1:1234/game" {
		t.Fatalf("unexpected websocket url: %s", got)
	}

	opt.ConnectParams.ConnectAddr = "wss://example.com/custom"
	if got := wsConnectURL(opt); got != "wss://example.com/custom" {
		t.Fatalf("explicit websocket path should be kept: %s", got)
	}
}

func TestWSMsgQueReadMultipleMessagesInOneFrame(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 256
	opt.WriteChanSize = 8
	handler := newRecordHandler()
	_, conn := setupWSMsgQue(t, ConnTypeAccept, opt, handler)

	msg1 := buildMsg(pb.MSG_ID(1), []byte("one"), 0)
	msg2 := buildMsg(pb.MSG_ID(2), []byte("two"), 0)
	writeWSFrames(t, conn, msg1, msg2)

	got1 := waitForMessage(t, handler.msgCh, "first ws message")
	if got1.MsgId() != int32(msg1.MsgId()) || string(got1.Data) != "one" {
		t.Fatalf("first websocket message mismatch")
	}
	got2 := waitForMessage(t, handler.msgCh, "second ws message")
	if got2.MsgId() != int32(msg2.MsgId()) || string(got2.Data) != "two" {
		t.Fatalf("second websocket message mismatch")
	}
}

func TestWSMsgQueConnParsesNewHeaderAndBody(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 256
	opt.WriteChanSize = 8
	handler := newRecordHandler()
	_, conn := setupWSMsgQue(t, ConnTypeAccept, opt, handler)

	body := []byte("new-ws-body")
	want := buildMsg(pb.MSG_ID(14), body, 0)
	want.SetMsgType(msgbase.MsgType(msg.WSMsgTypeRPC))
	want.SetTraceId(12345)
	writeWSFrames(t, conn, want)

	got := waitForMessage(t, handler.msgCh, "websocket conn parse new header")
	mainCmdID, subCmdID := msg.GetCmd(uint32(want.MsgId()))
	if got.Head == nil {
		t.Fatalf("websocket message head is nil")
	}
	if got.MsgId() != int32(want.MsgId()) {
		t.Fatalf("websocket msgId mismatch: got %d want %d", got.MsgId(), want.MsgId())
	}
	if got.Head.Cmd != uint32(want.MsgId()) || got.Head.MainCmdId != uint32(mainCmdID) || got.Head.SubCmdId != uint32(subCmdID) {
		t.Fatalf("websocket cmd mismatch: head=%+v", got.Head)
	}
	if got.Head.BodyLen != int32(len(body)) || !bytes.Equal(got.Data, body) {
		t.Fatalf("websocket body mismatch: len=%d data=%q", got.Head.BodyLen, string(got.Data))
	}
	if got.Head.MsgType != uint32(msg.WSMsgTypeRPC) || got.Head.RpcCallId != 12345 {
		t.Fatalf("websocket rpc header mismatch: type=%d trace=%d", got.Head.MsgType, got.Head.RpcCallId)
	}
}

func TestWSMsgQueReadMessageSplitAcrossFrames(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 256
	opt.WriteChanSize = 8
	handler := newRecordHandler()
	_, conn := setupWSMsgQue(t, ConnTypeAccept, opt, handler)

	frame := wsMsgFrameBytes(t, buildMsg(pb.MSG_ID(1), []byte("split-message"), 0))
	writeWSFrameBytes(t, conn, frame[:len(frame)/2])
	select {
	case <-handler.msgCh:
		t.Fatalf("partial websocket frame should not produce message")
	case <-time.After(50 * time.Millisecond):
	}

	writeWSFrameBytes(t, conn, frame[len(frame)/2:])
	got := waitForMessage(t, handler.msgCh, "split ws message")
	if got.MsgId() != int32(pb.MSG_ID(1)) || string(got.Data) != "split-message" {
		t.Fatalf("split websocket message mismatch")
	}
}

func TestWSMsgQueReadAcceptAllowsGrowthByDefault(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8
	handler := newRecordHandler()
	_, conn := setupWSMsgQue(t, ConnTypeAccept, opt, handler)

	payload := bytes.Repeat([]byte("a"), 100)
	writeWSFrames(t, conn, buildMsg(pb.MSG_ID(1), payload, 0))

	got := waitForMessage(t, handler.msgCh, "websocket accept growth")
	if got.MsgId() != int32(pb.MSG_ID(1)) || len(got.Data) != len(payload) {
		t.Fatalf("websocket accept message mismatch")
	}
}

func TestWSMsgQueReadGateExternalTooLarge(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8
	opt.SetIsGate(true)
	mq, conn := setupWSMsgQue(t, ConnTypeAccept, opt, nil)

	payload := bytes.Repeat([]byte("a"), 100)
	writeWSFrames(t, conn, buildMsg(pb.MSG_ID(1), payload, 0))

	waitForClosed(t, mq.quit, "websocket gate external too large")
}

func TestWSMsgQueReadInternalAllowsGrowth(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8
	handler := newRecordHandler()
	_, conn := setupWSMsgQue(t, ConnTypeConn, opt, handler)

	payload := bytes.Repeat([]byte("b"), 150)
	writeWSFrames(t, conn, buildMsg(pb.MSG_ID(1), payload, 0))

	got := waitForMessage(t, handler.msgCh, "websocket internal growth")
	if got.MsgId() != int32(pb.MSG_ID(1)) || len(got.Data) != len(payload) {
		t.Fatalf("websocket internal message mismatch")
	}
}

func TestWSMsgQueReadInternalTooLarge(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 64
	opt.WriteChanSize = 8
	mq, conn := setupWSMsgQue(t, ConnTypeConn, opt, nil)

	payload := bytes.Repeat([]byte("c"), 300)
	writeWSFrames(t, conn, buildMsg(pb.MSG_ID(1), payload, 0))

	waitForClosed(t, mq.quit, "websocket internal too large")
}

func TestWSMsgQueHeadLenTooLarge(t *testing.T) {
	opt := options.NewMsgQueOptions()
	mq := &wsMsgQue{msgQue: msgQue{opt: opt, agt: newConnAgt()}, quit: make(chan struct{})}
	state := newReadState(opt.ReadSize)

	headLen := uint16(MAX_HEAD_LEN + 1)
	state.buf[0] = byte(headLen >> 8)
	state.buf[1] = byte(headLen)
	state.offset = HEAD_SIZE

	if mq.consumeReadBuffer(state) {
		t.Fatalf("expected websocket head len validation to fail")
	}
}

func TestWSMsgQueConsumeReadBufferPartial(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.ReadSize = 128
	handler := newRecordHandler()
	mq := &wsMsgQue{msgQue: msgQue{opt: opt, handler: handler, connTyp: ConnTypeAccept, agt: newConnAgt()}, quit: make(chan struct{})}

	frame := wsMsgFrameBytes(t, buildMsg(pb.MSG_ID(1), []byte("hello"), 0))
	state := newReadState(opt.ReadSize)
	copy(state.buf, frame[:len(frame)/2])
	state.offset = len(frame) / 2

	if !mq.consumeReadBuffer(state) {
		t.Fatalf("partial websocket consume should not fail")
	}
	select {
	case <-handler.msgCh:
		t.Fatalf("websocket message should not be processed yet")
	default:
	}

	copy(state.buf[state.offset:], frame[len(frame)/2:])
	state.offset += len(frame) - len(frame)/2

	if !mq.consumeReadBuffer(state) {
		t.Fatalf("full websocket consume should succeed")
	}
	got := waitForMessage(t, handler.msgCh, "websocket partial complete")
	if string(got.Data) != "hello" {
		t.Fatalf("websocket message data mismatch")
	}
}

func TestWSMsgQueReadRejectsNonBinaryFrame(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.WriteChanSize = 8
	mq, conn := setupWSMsgQue(t, ConnTypeAccept, opt, nil)

	if err := conn.WriteMessage(websocket.TextMessage, []byte("bad")); err != nil {
		t.Fatalf("write websocket text frame failed: %v", err)
	}

	waitForClosed(t, mq.quit, "websocket non-binary frame")
}

func TestWSMsgQueFlushWriteBufferWritesBinaryFrame(t *testing.T) {
	opt := options.NewMsgQueOptions()
	serverConn, clientConn, cleanup := websocketPair(t, opt.WSPath)
	defer cleanup()

	mq := newWsConnect(nil, opt)
	mq.conn = serverConn
	buffer := &bytes.Buffer{}
	buffer.WriteString("abcdef")

	if err := mq.flushWriteBuffer(buffer); err != nil {
		t.Fatalf("websocket flush failed: %v", err)
	}
	if buffer.Len() != 0 {
		t.Fatalf("websocket buffer not reset")
	}

	msgType, data, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket flushed frame failed: %v", err)
	}
	if msgType != websocket.BinaryMessage || string(data) != "abcdef" {
		t.Fatalf("unexpected websocket flushed frame: type=%d data=%q", msgType, string(data))
	}
}

func TestWSMsgQueAdaptiveWriteDelayDisabled(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.DelayWrite = 10
	mq := &wsMsgQue{msgQue: msgQue{opt: opt}, delayCtrl: newTcpWriteDelayCtrl(opt.DelayWrite, WriteChanLowWatermark), quit: make(chan struct{})}

	if d := mq.adaptiveWriteDelay(0, false); d != 0 {
		t.Fatalf("expected no websocket delay when allowDelay is false, got %v", d)
	}

	optZero := options.NewMsgQueOptions()
	optZero.DelayWrite = 0
	mqZero := &wsMsgQue{msgQue: msgQue{opt: optZero}, delayCtrl: newTcpWriteDelayCtrl(optZero.DelayWrite, WriteChanLowWatermark), quit: make(chan struct{})}
	if d := mqZero.adaptiveWriteDelay(0, true); d != 0 {
		t.Fatalf("expected no websocket delay when DelayWrite is disabled, got %v", d)
	}
}

func TestWSMsgQueAdaptiveWriteDelayAimd(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.DelayWrite = 8
	mq := &wsMsgQue{msgQue: msgQue{opt: opt}, delayCtrl: newTcpWriteDelayCtrl(opt.DelayWrite, WriteChanLowWatermark), quit: make(chan struct{})}
	highThreshold := WriteChanLowWatermark / 3
	if highThreshold <= 0 {
		highThreshold = 1
	}

	if d := mq.adaptiveWriteDelay(0, true); d != 8*time.Millisecond {
		t.Fatalf("unexpected websocket initial delay: %v", d)
	}

	highBatch := WriteChanLowWatermark + 1
	for range highThreshold - 1 {
		if d := mq.adaptiveWriteDelay(highBatch, true); d != 8*time.Millisecond {
			t.Fatalf("unexpected websocket delay before high streak threshold: %v", d)
		}
	}
	if d := mq.adaptiveWriteDelay(highBatch, true); d != 4*time.Millisecond {
		t.Fatalf("expected websocket delay to halve after high streak, got %v", d)
	}

	for range highThreshold - 1 {
		if d := mq.adaptiveWriteDelay(highBatch, true); d != 4*time.Millisecond {
			t.Fatalf("unexpected websocket delay before second high streak threshold: %v", d)
		}
	}
	if d := mq.adaptiveWriteDelay(highBatch, true); d != 2*time.Millisecond {
		t.Fatalf("expected websocket delay to halve again after high streak, got %v", d)
	}

	lowBatch := WriteChanLowWatermark
	for range WriteChanLowWatermark - 1 {
		if d := mq.adaptiveWriteDelay(lowBatch, true); d != 2*time.Millisecond {
			t.Fatalf("unexpected websocket delay before low streak threshold: %v", d)
		}
	}
	if d := mq.adaptiveWriteDelay(lowBatch, true); d != 3*time.Millisecond {
		t.Fatalf("expected websocket delay to increase slowly after low streak, got %v", d)
	}
}

func TestWSMsgQueCollectWriteBatchTracksLastBatch(t *testing.T) {
	opt := options.NewMsgQueOptions()
	mq := &wsMsgQue{
		msgQue: msgQue{
			opt:    opt,
			cwrite: make(chan *msg.Message, 4),
		},
		delayCtrl: newTcpWriteDelayCtrl(opt.DelayWrite, WriteChanLowWatermark),
		quit:      make(chan struct{}),
	}
	buffer := &bytes.Buffer{}

	first := buildMsg(pb.MSG_ID(1), []byte("a"), 0)
	mq.cwrite <- buildMsg(pb.MSG_ID(2), []byte("b"), 0)
	mq.cwrite <- buildMsg(pb.MSG_ID(3), []byte("c"), 0)

	if err := mq.collectWriteBatch(buffer, first, false); err != nil {
		t.Fatalf("websocket collectWriteBatch failed: %v", err)
	}
	if mq.lastWriteBatch != 3 {
		t.Fatalf("unexpected websocket lastWriteBatch: %d", mq.lastWriteBatch)
	}
	if len(mq.cwrite) != 0 {
		t.Fatalf("expected websocket write queue to be drained, got %d", len(mq.cwrite))
	}
}

func TestWSMsgQueIsInternalConn(t *testing.T) {
	internal := &wsMsgQue{msgQue: msgQue{connTyp: ConnTypeConn, agt: newConnAgt()}, quit: make(chan struct{})}
	if !internal.isInternalConn() {
		t.Fatalf("websocket ConnTypeConn should be internal")
	}

	acceptInternal := &wsMsgQue{msgQue: msgQue{connTyp: ConnTypeAccept, agt: newConnAgt()}, quit: make(chan struct{})}
	if !acceptInternal.isInternalConn() {
		t.Fatalf("websocket non-gate accept should be internal")
	}

	gateExternal := &wsMsgQue{
		msgQue: msgQue{
			connTyp: ConnTypeAccept,
			agt:     newConnAgt(),
			opt:     (&options.NetOptions{}).SetIsGate(true),
		},
		quit: make(chan struct{}),
	}
	gateExternal.agt.AddSvrAgt("logic", 1)
	if gateExternal.isInternalConn() {
		t.Fatalf("websocket gate listen connections should stay external")
	}
}

func TestWSMsgQueCompressOrEncryptMatchesTCPExternalGate(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.SetIsGate(true)
	opt.SetCompress(true)
	opt.SetCompressMode(NET_COMPRESS_MODE_SNAPPY)
	opt.SetCompressLimit(1)

	tcpMq := &tcpMsgQue{msgQue: msgQue{opt: opt, connTyp: ConnTypeAccept, agt: newConnAgt()}, quit: make(chan struct{})}
	wsMq := &wsMsgQue{msgQue: msgQue{opt: opt, connTyp: ConnTypeAccept, agt: newConnAgt()}, quit: make(chan struct{})}
	payload := bytes.Repeat([]byte("compressible-websocket-data"), 64)

	tcpMsg := tcpMq.CompressOrEncrypt(buildMsg(pb.MSG_ID(1), payload, 0))
	wsMsg := wsMq.CompressOrEncrypt(buildMsg(pb.MSG_ID(1), payload, 0))
	if tcpMsg.FlagBits&msg.FlagCompress == 0 || wsMsg.FlagBits&msg.FlagCompress == 0 {
		t.Fatalf("expected tcp and websocket messages to be compressed")
	}
	if tcpMsg.FlagBits != wsMsg.FlagBits || !bytes.Equal(tcpMsg.Data, wsMsg.Data) {
		t.Fatalf("websocket compression does not match tcp behavior")
	}
}

func TestWSMsgQueStopUnblocksBlockingRead(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.Timeout = 0
	opt.WriteChanSize = 1
	mq, _ := setupWSMsgQue(t, ConnTypeConn, opt, nil)

	done := make(chan struct{})
	go func() {
		mq.stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("websocket stop blocked on read")
	}
}

func TestNetMgrWebSocketDisconnectEventAccept(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.Transport = options.TransportWebSocket
	opt.WSPath = "/ws"
	opt.SetListenParams(options.NewListenParams("127.0.0.1:0"))

	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start websocket listen failed: %v", err)
	}
	mqListen := mgr.listenMap[opt.ListenParams.ListenAddr].(*wsMsgQue)
	addr := mqListen.listener.Addr().String()

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatalf("dial websocket failed: %v", err)
	}
	waitForConn(t, handler.newCh, "websocket accept")
	waitForCondition(t, "websocket accept session count", func() bool {
		return mgr.GetAcceptSessNum() == 1
	})

	_ = conn.Close()
	waitForConn(t, handler.stopCh, "websocket disconnect stop")
	waitForCondition(t, "websocket accept session removed", func() bool {
		return mgr.GetAcceptSessNum() == 0
	})
}

func TestNetMgrWebSocketActiveCloseTriggersStop(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.Transport = options.TransportWebSocket
	opt.WSPath = "/ws"
	opt.SetListenParams(options.NewListenParams("127.0.0.1:0"))

	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start websocket listen failed: %v", err)
	}
	mqListen := mgr.listenMap[opt.ListenParams.ListenAddr].(*wsMsgQue)
	addr := mqListen.listener.Addr().String()

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatalf("dial websocket failed: %v", err)
	}
	defer conn.Close()

	accepted := waitForConn(t, handler.newCh, "websocket accept")
	mgr.RemoveSession(accepted.SessId())

	stopped := waitForConn(t, handler.stopCh, "websocket active close stop")
	if stopped.SessId() != accepted.SessId() {
		t.Fatalf("websocket stop event session mismatch")
	}
	waitForCondition(t, "websocket accept session removed after active close", func() bool {
		return mgr.GetAcceptSessNum() == 0
	})
}

func TestNetMgrWebSocketReconnectAfterDisconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	acceptCh := make(chan *websocket.Conn, 4)
	upgrader := websocket.Upgrader{CheckOrigin: func(req *http.Request) bool { return true }}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		acceptCh <- conn
	})}
	go func() {
		_ = server.Serve(ln)
	}()
	defer server.Close()

	mgr := NewNetMgr()
	mgr.RegisterCanReconnect(func(params *options.ConnectParams) bool { return true })
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.Transport = options.TransportWebSocket
	opt.WSPath = "/ws"
	opt.SetConnectParams(options.NewConnectParams("ws://"+ln.Addr().String()+"/ws", "logic", 1))

	if err := mgr.StartConnect(opt, handler); err != nil {
		t.Fatalf("start websocket connect failed: %v", err)
	}

	firstConn := waitForConn(t, handler.connectCh, "websocket connect-1")
	serverConn1 := waitForWebSocketConn(t, acceptCh, "websocket server connect-1")

	_ = serverConn1.Close()
	waitForConn(t, handler.stopCh, "websocket disconnect stop")

	serverConn2 := waitForWebSocketConn(t, acceptCh, "websocket server connect-2")
	defer serverConn2.Close()

	secondConn := waitForConn(t, handler.connectCh, "websocket connect-2")
	if firstConn.SessId() == secondConn.SessId() {
		t.Fatalf("websocket reconnect should use a new session id")
	}
}

func TestNetMgrReconnectWithNewMqUsesTransportType(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	acceptCh := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(req *http.Request) bool { return true }}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		acceptCh <- conn
	})}
	go func() {
		_ = server.Serve(ln)
	}()
	defer server.Close()

	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.Transport = options.TransportWebSocket
	opt.WSPath = "/ws"
	opt.SetConnectParams(options.NewConnectParams("ws://"+ln.Addr().String()+"/ws", "logic", 1))

	mq := &stubMsgQue{
		sessId:    1,
		connTyp:   ConnTypeConn,
		transport: options.TransportWebSocket,
		agt:       newConnAgt(),
		opt:       opt,
		handler:   handler,
	}

	mgr.reconnectWithNewMq(mq)

	serverConn := waitForWebSocketConn(t, acceptCh, "transport reconnect")
	defer serverConn.Close()
	waitForConn(t, handler.connectCh, "transport reconnect handler")
}

func TestNetMgrKCPRoundTrip(t *testing.T) {
	t.Skip("kcp transport is declared but not implemented in netmgr")
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	serverHandler := newRecordHandler()
	listenOpt := options.NewMsgQueOptions()
	listenOpt.Transport = options.TransportKCP
	listenOpt.SetListenParams(options.NewListenParams("127.0.0.1:0"))
	if err := mgr.StartListen(listenOpt, serverHandler); err != nil {
		t.Fatalf("start kcp listen failed: %v", err)
	}
	addr := listenAddrForTest(t, mgr.listenMap[listenOpt.ListenParams.ListenAddr])

	clientHandler := newRecordHandler()
	connectOpt := options.NewMsgQueOptions()
	connectOpt.Transport = options.TransportKCP
	connectOpt.SetConnectParams(options.NewConnectParams(addr, "gate", 1))
	if err := mgr.StartConnect(connectOpt, clientHandler); err != nil {
		t.Fatalf("start kcp connect failed: %v", err)
	}

	clientConn := waitForConn(t, clientHandler.connectCh, "kcp connect")
	if !clientConn.Send(msg.NewMsg(pb.MSG_ID(1), []byte("hello-kcp"))) {
		t.Fatalf("kcp client send failed")
	}

	serverConn := waitForConn(t, serverHandler.newCh, "kcp accept")
	got := waitForMessage(t, serverHandler.msgCh, "kcp server recv")
	if got.MsgId() != int32(pb.MSG_ID(1)) || string(got.Data) != "hello-kcp" {
		t.Fatalf("unexpected kcp server message: msgId=%d data=%q", got.MsgId(), string(got.Data))
	}

	if !serverConn.Send(msg.NewMsg(pb.MSG_ID(2), []byte("hello-client"))) {
		t.Fatalf("kcp server send failed")
	}
	got = waitForMessage(t, clientHandler.msgCh, "kcp client recv")
	if got.MsgId() != int32(pb.MSG_ID(2)) || string(got.Data) != "hello-client" {
		t.Fatalf("unexpected kcp client message: msgId=%d data=%q", got.MsgId(), string(got.Data))
	}
}

func TestNetMgrKCPReconnectAfterDisconnect(t *testing.T) {
	t.Skip("kcp transport is declared but not implemented in netmgr")
	serverMgr := NewNetMgr()
	serverMgr.Start()
	defer serverMgr.Stop()

	serverHandler := newRecordHandler()
	listenOpt := options.NewMsgQueOptions()
	listenOpt.Transport = options.TransportKCP
	listenOpt.SetListenParams(options.NewListenParams("127.0.0.1:0"))
	if err := serverMgr.StartListen(listenOpt, serverHandler); err != nil {
		t.Fatalf("start kcp listen failed: %v", err)
	}
	addr := listenAddrForTest(t, serverMgr.listenMap[listenOpt.ListenParams.ListenAddr])

	clientMgr := NewNetMgr()
	clientMgr.RegisterCanReconnect(func(params *options.ConnectParams) bool { return true })
	clientMgr.Start()
	defer clientMgr.Stop()

	clientHandler := newRecordHandler()
	connectOpt := options.NewMsgQueOptions()
	connectOpt.Transport = options.TransportKCP
	connectOpt.SetTimeout(1)
	connectOpt.SetConnectParams(options.NewConnectParams(addr, "logic", 1))
	if err := clientMgr.StartConnect(connectOpt, clientHandler); err != nil {
		t.Fatalf("start kcp connect failed: %v", err)
	}

	clientConn1 := waitForConn(t, clientHandler.connectCh, "kcp connect-1")
	if !clientConn1.Send(msg.NewMsg(pb.MSG_ID(1), []byte("connect-1"))) {
		t.Fatalf("kcp client send-1 failed")
	}
	serverConn1 := waitForConn(t, serverHandler.newCh, "kcp accept-1")
	waitForMessage(t, serverHandler.msgCh, "kcp recv-1")

	serverMgr.RemoveSession(serverConn1.SessId())
	waitForConn(t, clientHandler.stopCh, "kcp disconnect stop")

	clientConn2 := waitForConn(t, clientHandler.connectCh, "kcp connect-2")
	if clientConn1.SessId() == clientConn2.SessId() {
		t.Fatalf("kcp reconnect should use a new session id")
	}
	if !clientConn2.Send(msg.NewMsg(pb.MSG_ID(2), []byte("connect-2"))) {
		t.Fatalf("kcp client send-2 failed")
	}
	waitForConn(t, serverHandler.newCh, "kcp accept-2")
	waitForMatchingMessage(t, serverHandler.msgCh, "kcp recv-2", func(m *msg.Message) bool {
		return m.MsgId() == int32(pb.MSG_ID(2)) && string(m.Data) == "connect-2"
	})
}

func TestNetMgrCloseDuringReadTriggersStop(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	c1, c2 := net.Pipe()
	defer c1.Close()

	mq := setupMsgQueWithConn(t, mgr, handler, c1)

	_ = c2.Close()
	stopped := waitForConn(t, handler.stopCh, "stop on read close")
	if stopped.SessId() != mq.SessId() {
		t.Fatalf("stop event session mismatch")
	}
}

func TestTcpMsgQueStopUnblocksBlockingRead(t *testing.T) {
	opt := options.NewMsgQueOptions()
	opt.Timeout = 0
	opt.WriteChanSize = 1

	conn := newBlockingReadConn()
	mq := newTcpConnect(nil, opt)
	mq.conn = conn
	mq.Start()
	defer conn.Close()

	select {
	case <-conn.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("read did not start")
	}

	done := make(chan struct{})
	go func() {
		mq.stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("stop blocked on read")
	}
}

func TestNetMgrCloseDuringWriteTriggersStop(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	conn := newWriteReasonConn()
	mq := setupMsgQueWithConn(t, mgr, handler, conn)

	if !mq.Send(msg.NewMsg(pb.MSG_ID(1), []byte("data"))) {
		t.Fatalf("send failed")
	}

	select {
	case <-conn.wroteCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("write not started")
	}
	_ = conn.Close()

	stopped := waitForConn(t, handler.stopCh, "stop on write close")
	if stopped.SessId() != mq.SessId() {
		t.Fatalf("stop event session mismatch")
	}
}

func TestNetMgrActiveCloseTriggersStop(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	mq := setupMsgQueWithConn(t, mgr, handler, c1)
	mgr.RemoveSession(mq.SessId())

	stopped := waitForConn(t, handler.stopCh, "stop on remove session")
	if stopped.SessId() != mq.SessId() {
		t.Fatalf("stop event session mismatch")
	}
}

func TestNetMgrDisconnectEventAccept(t *testing.T) {
	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.SetListenParams(options.NewListenParams("127.0.0.1:0"))

	if err := mgr.StartListen(opt, handler); err != nil {
		t.Fatalf("start listen failed: %v", err)
	}
	mqListen := mgr.listenMap[opt.ListenParams.ListenAddr].(*tcpMsgQue)
	addr := mqListen.listener.Addr().String()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	waitForConn(t, handler.newCh, "accept")
	waitForCondition(t, "accept session count", func() bool {
		return mgr.GetAcceptSessNum() == 1
	})

	_ = conn.Close()
	waitForConn(t, handler.stopCh, "disconnect stop")
	waitForCondition(t, "accept session removed", func() bool {
		return mgr.GetAcceptSessNum() == 0
	})
}

func TestNetMgrDisconnectEventLogsTcpReason(t *testing.T) {
	readLog := captureDefaultLog(t)

	mgr := NewNetMgr()
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	c1, c2 := net.Pipe()
	defer c1.Close()

	mq := setupMsgQueWithConn(t, mgr, handler, c1)
	_ = c2.Close()

	stopped := waitForConn(t, handler.stopCh, "disconnect stop")
	if stopped.SessId() != mq.SessId() {
		t.Fatalf("stop event session mismatch")
	}
	waitForCondition(t, "session removed", func() bool {
		return mgr.GetSessNum() == 0
	})

	logOutput := readLog()
	if !strings.Contains(logOutput, "conn disconnect [") ||
		!strings.Contains(logOutput, "connType=conn") ||
		!strings.Contains(logOutput, "cause=read: EOF") ||
		!strings.Contains(logOutput, "action=target unavailable") {
		t.Fatalf("disconnect log missing tcp reason, logs: %s", logOutput)
	}
}

func TestNetMgrDisconnectCauseKeepsFirstTcpReason(t *testing.T) {
	mgr := NewNetMgr()
	opt := options.NewMsgQueOptions()
	mq := newTcpConnect(nil, opt)
	mq.recordDisconnectErr("write", io.ErrClosedPipe)
	mq.recordDisconnectErr("read", io.EOF)

	reason, action := mgr.disconnectReason(mq, true)
	if reason != "write: io: read/write on closed pipe" {
		t.Fatalf("unexpected disconnect cause: %s", reason)
	}
	if action != "reconnect scheduled" {
		t.Fatalf("unexpected disconnect action: %s", action)
	}
}

func TestNetMgrAcceptSessNumDisconnectScenarios(t *testing.T) {
	mgr := NewNetMgr()
	opt := options.NewMsgQueOptions()

	s1 := &stubMsgQue{sessId: 1, connTyp: ConnTypeAccept, agt: newConnAgt(), opt: opt}
	s2 := &stubMsgQue{sessId: 2, connTyp: ConnTypeAccept, agt: newConnAgt(), opt: opt}
	s3 := &stubMsgQue{sessId: 3, connTyp: ConnTypeAccept, agt: newConnAgt(), opt: opt}

	mgr.addSess(s1)
	mgr.addSess(s2)
	mgr.addSess(s3)

	if mgr.GetAcceptSessNum() != 3 {
		t.Fatalf("accept session count mismatch: %d", mgr.GetAcceptSessNum())
	}

	mgr.sessOverEvt(s1)
	drainTasks(mgr)
	if mgr.GetAcceptSessNum() != 2 {
		t.Fatalf("accept session count mismatch after disconnect: %d", mgr.GetAcceptSessNum())
	}
	waitForCondition(t, "session stopped after disconnect", func() bool { return s1.stopped.Load() })

	mgr.RemoveSession(s2.sessId)
	drainTasks(mgr)
	if mgr.GetAcceptSessNum() != 1 {
		t.Fatalf("accept session count mismatch after remove: %d", mgr.GetAcceptSessNum())
	}
	waitForCondition(t, "session stopped after remove", func() bool { return s2.stopped.Load() })

	s3.agt.AddCltUser(100)
	mgr.KickSession(s3.sessId, 100)
	drainTasks(mgr)
	if mgr.GetAcceptSessNum() != 0 {
		t.Fatalf("accept session count mismatch after kick: %d", mgr.GetAcceptSessNum())
	}
	waitForCondition(t, "session stopped after kick", func() bool { return s3.stopped.Load() })
}

func TestNetMgrReconnectAfterDisconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	acceptCh := make(chan net.Conn, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			acceptCh <- conn
		}
	}()

	mgr := NewNetMgr()
	mgr.RegisterCanReconnect(func(params *options.ConnectParams) bool { return true })
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.SetConnectParams(options.NewConnectParams(ln.Addr().String(), "logic", 1))

	if err := mgr.StartConnect(opt, handler); err != nil {
		t.Fatalf("start connect failed: %v", err)
	}

	firstConn := waitForConn(t, handler.connectCh, "connect-1")
	conn1 := <-acceptCh

	_ = conn1.Close()
	waitForConn(t, handler.stopCh, "disconnect stop")

	conn2 := <-acceptCh
	defer conn2.Close()

	secondConn := waitForConn(t, handler.connectCh, "connect-2")
	if firstConn.SessId() == secondConn.SessId() {
		t.Fatalf("reconnect should use a new session id")
	}
}

func TestNetMgrNoReconnectWhenDisabled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	acceptCh := make(chan net.Conn, 2)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			acceptCh <- conn
		}
	}()

	mgr := NewNetMgr()
	mgr.RegisterCanReconnect(func(params *options.ConnectParams) bool { return false })
	mgr.Start()
	defer mgr.Stop()

	handler := newRecordHandler()
	opt := options.NewMsgQueOptions()
	opt.SetConnectParams(options.NewConnectParams(ln.Addr().String(), "logic", 1))

	if err := mgr.StartConnect(opt, handler); err != nil {
		t.Fatalf("start connect failed: %v", err)
	}

	conn := <-acceptCh
	waitForConn(t, handler.connectCh, "connect-1")

	_ = conn.Close()
	waitForConn(t, handler.stopCh, "disconnect stop")

	select {
	case extra := <-acceptCh:
		_ = extra.Close()
		t.Fatalf("unexpected reconnect when disabled")
	case <-time.After(200 * time.Millisecond):
	}
}

func BenchmarkWriteDelayCtrlNextDelay(b *testing.B) {
	b.Run("HighBatch", func(b *testing.B) {
		ctrl := newTcpWriteDelayCtrl(8, WriteChanLowWatermark)
		var d time.Duration
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d = ctrl.nextDelay(WriteChanLowWatermark+1, true)
		}
		benchDelay = d
	})

	b.Run("LowBatch", func(b *testing.B) {
		ctrl := newTcpWriteDelayCtrl(8, WriteChanLowWatermark)
		var d time.Duration
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d = ctrl.nextDelay(WriteChanLowWatermark, true)
		}
		benchDelay = d
	})
}
