package netmgr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"game/deps/basal"
	"game/deps/fastid"
	"game/deps/msg"
	"game/deps/netmgr/options"
	"game/deps/xlog"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type wsMsgQue struct {
	msgQue
	conn           *websocket.Conn
	listener       net.Listener
	server         *http.Server
	upgrader       websocket.Upgrader
	reader         io.Reader
	writeMu        sync.Mutex
	discEvt        func(IMsgQue)
	readWg         sync.WaitGroup
	writeWg        sync.WaitGroup
	quit           chan struct{}
	stopOnce       sync.Once
	quitOnce       sync.Once
	connOnce       sync.Once
	discOnce       sync.Once
	stopping       atomic.Bool
	connecting     atomic.Bool
	discReasonMu   sync.RWMutex
	discReason     string
	lastWriteBatch int
	delayCtrl      *tcpWriteDelayCtrl
}

func (r *wsMsgQue) ensureOpt() *options.NetOptions {
	if r.opt == nil {
		r.opt = options.NewMsgQueOptions()
	}
	if r.opt.WSPath == "" {
		r.opt.WSPath = options.DefaultWSPath
	}
	return r.opt
}

func (r *wsMsgQue) listen(netmgr *NetMgr) {
	opt := r.ensureOpt()
	mux := http.NewServeMux()
	mux.HandleFunc(opt.WSPath, func(w http.ResponseWriter, req *http.Request) {
		if r.isOverload(netmgr) {
			xlog.Warnf("[%v] websocket conn overload with limit=%v", r.listenAddr(netmgr), netmgr.getOpt().ListenParams.MaxConn)
			http.Error(w, "websocket conn overload", http.StatusServiceUnavailable)
			return
		}

		conn, err := r.upgrader.Upgrade(w, req, nil)
		if err != nil {
			xlog.Warnf("upgrade websocket failed: %v", err)
			return
		}
		basal.SafeGo(func() {
			r.handleAcceptConn(netmgr, conn)
		})
	})

	r.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: r.headerTimeout(),
	}
	basal.SafeGo(func() {
		err := r.server.Serve(r.listener)
		if err == nil || errors.Is(err, http.ErrServerClosed) || r.isClosed() {
			return
		}
		xlog.Errorf("websocket listen failed: %v", err)
	})
}

func (r *wsMsgQue) handleAcceptConn(netmgr *NetMgr, conn *websocket.Conn) {
	acceptMq := newWsAccept(conn, r.handler, r.ensureOpt())

	if !netmgr.addTask(func() {
		acceptMq.discEvt = netmgr.sessOverEvt

		netmgr.addSess(acceptMq)
		if acceptMq.handler != nil && !acceptMq.handler.OnNewMsgQue(acceptMq) {
			netmgr.deleteSess(acceptMq.SessId())
			basal.SafeGo(func() { acceptMq.stop() })
			return
		}

		acceptMq.Start()
	}) {
		_ = conn.Close()
		xlog.Warnf("[%v] websocket accept task dropped; conn closed", r.listenAddr(netmgr))
	}
}

func (r *wsMsgQue) listenAddr(netmgr *NetMgr) string {
	if netmgr == nil || netmgr.getOpt() == nil || netmgr.getOpt().ListenParams == nil {
		return ""
	}
	return netmgr.getOpt().ListenParams.ListenAddr
}

func (r *wsMsgQue) isOverload(netmgr *NetMgr) bool {
	if netmgr == nil || netmgr.getOpt() == nil || netmgr.getOpt().ListenParams == nil {
		return false
	}
	mLoad := netmgr.getOpt().ListenParams.MaxConn
	return mLoad > 0 && netmgr.GetAcceptSessNum() >= mLoad
}

func (r *wsMsgQue) headerTimeout() time.Duration {
	timeout := 5 * time.Second
	if r.opt != nil && r.opt.Timeout > 0 {
		timeout = time.Second * time.Duration(r.opt.Timeout)
	}
	return timeout
}

func (r *wsMsgQue) connect(netmgr *NetMgr) {
	if r == nil || netmgr == nil {
		return
	}
	if r.conn != nil {
		return
	}
	if r.stopping.Load() {
		return
	}
	if !r.connecting.CompareAndSwap(false, true) {
		return
	}
	basal.SafeGo(func() {
		defer r.connecting.Store(false)
		r.connectOnce(netmgr)
	})
}

func (r *wsMsgQue) connectOnce(netmgr *NetMgr) {
	opt := r.ensureOpt()
	if opt == nil || opt.ConnectParams == nil {
		return
	}

	addr := opt.ConnectParams.ConnectAddr
	svrType, svrId := opt.ConnectParams.SvrType, opt.ConnectParams.SvrId
	dialer := websocket.Dialer{
		HandshakeTimeout: 3 * time.Second,
		ReadBufferSize:   int(opt.ReadSize),
		WriteBufferSize:  int(opt.WriteSize),
	}
	conn, _, err := dialer.Dial(wsConnectURL(opt), nil)
	if err != nil {
		if svrType != "" {
			if netmgr.CanReconnect(opt.ConnectParams) {
				xlog.Warnf("connect [%v-%v-%v][websocket] failed : %s", svrType, svrId, addr, err.Error())
				r.scheduleReconnect(netmgr, svrType, svrId)
			} else {
				xlog.Infof("connect [%v-%v-%v][websocket] skipped (target unavailable).", svrType, svrId, addr)
			}
		}
		return
	}

	if !netmgr.addTask(func() {
		r.sessId = fastid.GenInt64ID()
		r.conn = conn
		r.discEvt = netmgr.sessOverEvt
		netmgr.addSess(r)
		r.Start()

		xlog.Infof("[websocket] connect to %s-%v[%v] successfully.", svrType, svrId, addr)
		if r.handler != nil {
			r.handler.OnConnectSuccess(r)
		}
	}) {
		_ = conn.Close()
		if svrType != "" && !netmgr.isStopping() {
			r.scheduleReconnect(netmgr, svrType, svrId)
		}
	}
}

func (r *wsMsgQue) scheduleReconnect(netmgr *NetMgr, svrType string, svrId int32) {
	if svrType == "" || netmgr == nil || netmgr.timerMgr == nil {
		return
	}
	if !netmgr.CanReconnect(r.opt.ConnectParams) {
		return
	}
	_, _ = netmgr.timerMgr.AddOneShotTimer(fmt.Sprintf("connect_%v_%v", svrType, svrId),
		r.opt.ConnectParams.ReConnectInv, false, func(name string, now int64, value any) {
			r.connect(netmgr)
		})
}

func (r *wsMsgQue) stop() {
	r.stopOnce.Do(func() {
		r.stopping.Store(true)
		r.notifyStop()
		r.signalQuit()
		r.closeRead()
		r.closeListener()
		r.waitIO()
		r.closeConn()
	})
}

func (r *wsMsgQue) Send(m *msg.Message) (re bool) {
	if r.stopping.Load() {
		return false
	}
	return r.msgQue.Send(m)
}

func (r *wsMsgQue) notifyStop() {
	if r.handler != nil {
		r.handler.OnMsgQueStop(r)
	}
}

func (r *wsMsgQue) waitIO() {
	r.writeWg.Wait()
	r.readWg.Wait()
}

func (r *wsMsgQue) forceClose() {
	r.signalQuit()
	r.closeConn()
	r.closeListener()
}

func (r *wsMsgQue) signalQuit() {
	r.quitOnce.Do(func() {
		if r.quit != nil {
			close(r.quit)
		}
	})
}

func (r *wsMsgQue) closeListener() {
	if r.server != nil {
		_ = r.server.Close()
	}
	if r.listener != nil {
		_ = r.listener.Close()
	}
}

func (r *wsMsgQue) closeRead() {
	if r.conn == nil {
		return
	}
	_ = r.conn.SetReadDeadline(time.Now())
}

func (r *wsMsgQue) isClosed() bool {
	select {
	case <-r.quit:
		return true
	default:
		return false
	}
}

func (r *wsMsgQue) closeConn() {
	r.connOnce.Do(func() {
		if r.conn == nil {
			return
		}
		_ = r.conn.Close()
	})
}

func (r *wsMsgQue) remoteAddr() string {
	if r.conn != nil && r.conn.RemoteAddr() != nil {
		return r.conn.RemoteAddr().String()
	}
	return ""
}

func (r *wsMsgQue) remoteIP() string {
	addr := strings.TrimSpace(r.remoteAddr())
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}

func (r *wsMsgQue) isInternalConn() bool {
	if r.connTyp == ConnTypeConn {
		return true
	}
	if r.opt != nil && r.opt.IsGate {
		return false
	}
	return true
}

func (r *wsMsgQue) onDiscEvt(flag bool) {
	if !flag || r.stopping.Load() || r.discEvt == nil {
		return
	}
	r.discOnce.Do(func() {
		r.discEvt(r)
	})
}

func (r *wsMsgQue) recordDisconnectErr(op string, err error) {
	if err == nil {
		return
	}
	r.discReasonMu.Lock()
	if r.discReason == "" {
		r.discReason = fmt.Sprintf("%s: %v", op, err)
	}
	r.discReasonMu.Unlock()
}

func (r *wsMsgQue) getDisconnectReason() string {
	r.discReasonMu.RLock()
	reason := r.discReason
	r.discReasonMu.RUnlock()
	return reason
}

func (r *wsMsgQue) readFromConn(state *readState) (int, error) {
	for {
		if r.reader == nil {
			timeout := r.opt.Timeout
			if timeout > 0 {
				_ = r.conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout)))
			}

			msgType, reader, err := r.conn.NextReader()
			if err != nil {
				return 0, err
			}
			if msgType != websocket.BinaryMessage {
				return 0, fmt.Errorf("unexpected websocket message type: %d", msgType)
			}
			r.reader = reader
		}

		timeout := r.opt.Timeout
		if timeout > 0 {
			_ = r.conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout)))
		}
		n, err := r.reader.Read(state.buf[state.offset:])
		if n > 0 {
			state.offset += n
		}
		if errors.Is(err, io.EOF) {
			r.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (r *wsMsgQue) ensureFrameCapacity(state *readState) bool {
	total := HEAD_SIZE + state.headLen + state.bodyLen
	max := int(r.opt.ReadSize)
	if max >= total {
		return true
	}

	if r.isInternalConn() {
		if total > max*4 {
			xlog.Warnf("[%d] internal websocket packet too large head:%d body:%d max:%d", r.sessId, state.headLen, state.bodyLen, max*4)
			return false
		}
		state.grow(total)
	} else {
		xlog.Warnf("[%d] websocket packet too large head:%d body:%d max:%d", r.sessId, state.headLen, state.bodyLen, max)
		return false
	}

	return true
}

func (r *wsMsgQue) handleReadMessage(state *readState, ptr int) bool {
	if state.msg == nil {
		return false
	}
	if state.bodyLen == 0 {
		return r.processMsg(r, state.msg)
	}
	data := make([]byte, state.bodyLen)
	copy(data, state.buf[ptr:ptr+state.bodyLen])
	state.msg.Data = data
	return r.processMsg(r, state.msg)
}

func (r *wsMsgQue) consumeReadBuffer(state *readState) bool {
	ptr := 0
	for {
		if state.headLen == 0 {
			if state.offset-ptr < HEAD_SIZE {
				break
			}
			state.headLen = int(binary.BigEndian.Uint16(state.buf[ptr : ptr+HEAD_SIZE]))
			if state.headLen > MAX_HEAD_LEN {
				xlog.Warnf("[%d] websocket packet head len invalid: %d, stop msgque.", r.sessId, state.headLen)
				return false
			}

			ptr += HEAD_SIZE
		}

		if state.bodyLen == 0 {
			if state.offset-ptr < state.headLen {
				break
			}
			message := &msg.Message{}
			head, err := message.NewMessageHead(state.headLen, state.buf[ptr:])
			if head == nil || err != nil {
				xlog.Warnf("[%d] websocket packet head err. stop msgque.", r.sessId)
				return false
			}
			if head.BodyLen < 0 {
				xlog.Warnf("[%d] websocket packet body len invalid: %d, stop msgque.", r.sessId, head.BodyLen)
				return false
			}
			state.msg = message
			ptr += state.headLen
			state.bodyLen = int(head.BodyLen)
			if !r.ensureFrameCapacity(state) {
				return false
			}
		}

		if state.offset-ptr < state.bodyLen {
			break
		}

		if !r.handleReadMessage(state, ptr) {
			xlog.Warnf("websocket msgque:%v process msg failed, stop msgque. msgId:%v", r.sessId, state.msg.MsgId())
			return false
		}
		ptr += state.bodyLen
		state.resetFrame()
	}

	if ptr > state.offset {
		xlog.Errorf("something wrong for websocket sess = %d, stop msgque", r.sessId)
		return false
	}
	if state.offset > ptr {
		copy(state.buf, state.buf[ptr:state.offset])
	}
	state.offset -= ptr
	return true
}

func (r *wsMsgQue) read() {
	disconnected := true
	defer func() {
		r.readWg.Done()
		if err := recover(); err != nil {
			xlog.Errorf("%v : %s", err, debug.Stack())
			disconnected = true
		}
		if disconnected {
			r.forceClose()
		}
		r.onDiscEvt(disconnected)
	}()

	state := newReadState(r.opt.ReadSize)

	for {
		if r.isClosed() {
			disconnected = false
			return
		}

		if state.offset == len(state.buf) {
			if r.isInternalConn() {
				state.grow(len(state.buf) * 2)
			} else {
				xlog.Warnf("[%d] websocket read buffer full", r.sessId)
				return
			}
		}

		n, err := r.readFromConn(state)
		if err != nil {
			if r.isClosed() {
				disconnected = false
				return
			}
			r.recordDisconnectErr("read", err)
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			xlog.Warnf("[%d] websocket conn read failed : %s", r.sessId, err.Error())
			return
		}
		if n <= 0 {
			continue
		}
		if !r.consumeReadBuffer(state) {
			return
		}
	}
}

func (r *wsMsgQue) appendWriteBuffer(buffer *bytes.Buffer, m *msg.Message) error {
	if m == nil {
		return nil
	}
	mx := r.CompressOrEncrypt(m)
	_, err := mx.Bytes(buffer)
	return err
}

func (r *wsMsgQue) newWriteBuffer() *bytes.Buffer {
	buffer := &bytes.Buffer{}
	if r.opt.WriteSize > 0 {
		buffer.Grow(int(r.opt.WriteSize))
	}
	return buffer
}

func (r *wsMsgQue) drainWriteQueue(buffer *bytes.Buffer, count int) error {
	for range count {
		m := <-r.cwrite
		if m == nil {
			continue
		}
		if err := r.appendWriteBuffer(buffer, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *wsMsgQue) adaptiveWriteDelay(lastBatch int, allowDelay bool) time.Duration {
	return r.delayCtrl.nextDelay(lastBatch, allowDelay)
}

func (r *wsMsgQue) collectWriteBatch(buffer *bytes.Buffer, first *msg.Message, allowDelay bool) error {
	if err := r.appendWriteBuffer(buffer, first); err != nil {
		return err
	}
	shouldDelay := allowDelay && len(r.cwrite) <= options.DELAY_SEND_QUEUE_LEN
	if delay := r.adaptiveWriteDelay(r.lastWriteBatch, shouldDelay); delay > 0 {
		time.Sleep(delay)
	}
	queued := min(len(r.cwrite), options.MAX_SEND_QUEUE_LEN)
	if err := r.drainWriteQueue(buffer, queued); err != nil {
		return err
	}
	r.lastWriteBatch = queued + 1
	return nil
}

func (r *wsMsgQue) flushWriteBuffer(buffer *bytes.Buffer) error {
	return r.flushWriteBufferWithTimeout(buffer, 0)
}

func (r *wsMsgQue) flushWriteBufferWithTimeout(buffer *bytes.Buffer, timeout time.Duration) error {
	if buffer.Len() == 0 {
		return nil
	}

	opt := r.ensureOpt()
	if timeout > 0 {
		_ = r.conn.SetWriteDeadline(time.Now().Add(timeout))
	} else if opt.Timeout > 0 {
		_ = r.conn.SetWriteDeadline(time.Now().Add(time.Second * time.Duration(opt.Timeout)))
	}

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	writer, err := r.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}

	data := buffer.Bytes()
	wn := 0
	for wn < len(data) {
		n, err := writer.Write(data[wn:])
		if err != nil {
			_ = writer.Close()
			return err
		}
		wn += n
	}
	if err := writer.Close(); err != nil {
		return err
	}

	buffer.Reset()
	return nil
}

func (r *wsMsgQue) write() {
	disconnected := true
	defer func() {
		r.writeWg.Done()
		if err := recover(); err != nil {
			xlog.Errorf("%v : %s", err, debug.Stack())
			disconnected = true
		}
		if disconnected {
			r.forceClose()
		}
		r.onDiscEvt(disconnected)
	}()

	r.delayCtrl = newTcpWriteDelayCtrl(r.opt.DelayWrite, WriteChanLowWatermark)
	buffer := r.newWriteBuffer()

	for {
		select {
		case <-r.quit:
			disconnected = false
			closeTimeout := externalStopWriteTimeout
			if r.isInternalConn() {
				closeTimeout = internalStopWriteTimeout
			}

			for {
				queued := len(r.cwrite)
				if queued == 0 {
					break
				}
				if err := r.drainWriteQueue(buffer, queued); err != nil {
					r.recordDisconnectErr("write", err)
					xlog.Warnf("[%d] websocket packet write failed : %s", r.sessId, err.Error())
					return
				}
				if err := r.flushWriteBufferWithTimeout(buffer, closeTimeout); err != nil {
					r.recordDisconnectErr("write", err)
					xlog.Warnf("[%d] websocket conn send failed : %s", r.sessId, err.Error())
					return
				}
			}
			if err := r.flushWriteBufferWithTimeout(buffer, closeTimeout); err != nil {
				r.recordDisconnectErr("write", err)
				xlog.Warnf("[%d] websocket conn send failed : %s", r.sessId, err.Error())
			}
			return

		case m := <-r.cwrite:
			if m == nil {
				continue
			}
			if err := r.collectWriteBatch(buffer, m, true); err != nil {
				if r.isClosed() {
					disconnected = false
					return
				}
				r.recordDisconnectErr("write", err)
				xlog.Warnf("[%d] websocket packet write failed : %s", r.sessId, err.Error())
				return
			}
			if err := r.flushWriteBuffer(buffer); err != nil {
				if r.isClosed() {
					disconnected = false
					return
				}
				r.recordDisconnectErr("write", err)
				xlog.Warnf("[%d] websocket conn send failed : %s", r.sessId, err.Error())
				return
			}
		}
	}
}

func (r *wsMsgQue) CompressOrEncrypt(m *msg.Message) *msg.Message {
	if m.Head.Flags&msg.FlagBroad > 0 {
		return m
	}
	if m.Head.Flags&msg.FlagCompress > 0 {
		return m
	}
	if r.opt == nil || r.isInternalConn() || !r.opt.Compress || m.Data == nil {
		return m
	}

	compressLimit := r.opt.CompressLimit
	if int32(len(m.Data)) < compressLimit {
		return m
	}

	var data []byte
	switch r.opt.CompressMode {
	case NET_COMPRESS_MODE_SNAPPY:
		data = basal.SnappyCompress(m.Data)
	default:
		data = basal.GZipCompress(m.Data)
	}
	if len(data) >= len(m.Data) {
		return m
	}

	newMsg := m.Copy()
	newMsg.Head.Flags |= msg.FlagCompress
	newMsg.Data = data
	newMsg.Head.BodyLen = int32(len(data))
	return newMsg
}

func (r *wsMsgQue) Start() {
	r.ensureOpt()
	r.readWg.Add(1)
	go r.read()
	r.writeWg.Add(1)
	go r.write()
}

func newWsAccept(conn *websocket.Conn, handler INetEventHandler, opt *options.NetOptions) *wsMsgQue {
	mq := &wsMsgQue{
		msgQue: msgQue{
			sessId:  fastid.GenInt64ID(),
			cwrite:  make(chan *msg.Message, opt.WriteChanSize),
			handler: handler,
			connTyp: ConnTypeAccept,
			agt:     newConnAgt(),
			opt:     opt,
		},
		conn: conn,
		quit: make(chan struct{}),
	}

	if host, _, err := net.SplitHostPort(strings.TrimSpace(mq.remoteAddr())); err == nil {
		mq.agt.AddCltRemote(strings.TrimSpace(host))
	} else {
		mq.agt.AddCltRemote(strings.TrimSpace(mq.remoteAddr()))
	}
	xlog.Debugf("sessId:[%d] new websocket accept from %s", mq.sessId, mq.remoteAddr())
	return mq
}

func newWsConnect(handler INetEventHandler, opt *options.NetOptions) *wsMsgQue {
	mq := &wsMsgQue{
		msgQue: msgQue{
			sessId:  fastid.GenInt64ID(),
			cwrite:  make(chan *msg.Message, opt.WriteChanSize),
			connTyp: ConnTypeConn,
			agt:     newConnAgt(),
			handler: handler,
			opt:     opt,
		},
		quit: make(chan struct{}),
	}
	xlog.Debugf("sessId:[%d] new websocket connect to %s", mq.sessId, wsConnectURL(opt))
	return mq
}

func newWSListen(listener net.Listener, opt *options.NetOptions, handler INetEventHandler) *wsMsgQue {
	mq := &wsMsgQue{
		msgQue: msgQue{
			sessId:  fastid.GenInt64ID(),
			connTyp: ConnTypeListen,
			agt:     newConnAgt(),
			handler: handler,
			opt:     opt,
		},
		listener: listener,
		quit:     make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  int(opt.ReadSize),
			WriteBufferSize: int(opt.WriteSize),
			CheckOrigin: func(req *http.Request) bool {
				return true
			},
		},
	}
	return mq
}

func wsConnectURL(opt *options.NetOptions) string {
	if opt == nil || opt.ConnectParams == nil {
		return ""
	}
	addr := strings.TrimSpace(opt.ConnectParams.ConnectAddr)
	if strings.HasPrefix(addr, "ws://") || strings.HasPrefix(addr, "wss://") {
		u, err := url.Parse(addr)
		if err != nil {
			return addr
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = wsPath(opt)
		}
		return u.String()
	}
	return "ws://" + addr + wsPath(opt)
}

func wsPath(opt *options.NetOptions) string {
	path := opt.WSPath
	if path == "" {
		path = options.DefaultWSPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
