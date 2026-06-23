package netmgr

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"game/deps/basal"
	"game/deps/fastid"
	"game/deps/msg"
	"game/deps/xlog"
	"io"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"game/deps/netmgr/options"
)

type tcpMsgQue struct {
	msgQue
	conn           net.Conn     //连接
	listener       net.Listener //监听
	network        string
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

func (r *tcpMsgQue) ensureOpt() *options.NetOptions {
	if r.opt == nil {
		r.opt = options.NewMsgQueOptions()
	}
	return r.opt
}

func (r *tcpMsgQue) listen(netmgr *NetMgr) {
	basal.SafeGo(func() {
		r.acceptLoop(netmgr)
	})
}

func (r *tcpMsgQue) acceptLoop(netmgr *NetMgr) {
	for {
		conn, err := r.listener.Accept()
		if err != nil || conn == nil {
			if r.isClosed() {
				return
			}
			xlog.Errorf("msgque accept err:%v", err)
			continue
		}
		basal.SafeGo(func() {
			r.handleAcceptConn(netmgr, conn)
		})
	}
}

func (r *tcpMsgQue) handleAcceptConn(netmgr *NetMgr, conn net.Conn) {
	listenAddr := netmgr.getOpt().ListenParams.ListenAddr
	mLoad := netmgr.getOpt().ListenParams.MaxConn

	if mLoad > 0 && netmgr.GetAcceptSessNum() >= mLoad {
		basal.SafeGo(func() { _ = conn.Close() })
		xlog.Warnf("[%v] conn overload with limit=%v", listenAddr, mLoad)
		return
	}

	dhKey, err := r.serverHandshake(netmgr, conn)
	if err != nil {
		_ = conn.Close()

		if errors.Is(err, ErrInvalidHandshakeProbe) {
			return
		}
		xlog.Warnf("dhKeyExchange err:%v", err)
		return
	}

	if !netmgr.addTask(func() {
		mq := newTcpAccept(conn, r.handler, netmgr.getOpt())
		mq.discEvt = netmgr.sessOverEvt
		mq.setDhKey(dhKey)

		// 先入表，避免 Start() 后立即断线导致断线事件丢失。
		netmgr.addSess(mq)
		if mq.handler != nil && !mq.handler.OnNewMsgQue(mq) {
			netmgr.deleteSess(mq.SessId())
			basal.SafeGo(func() { mq.stop() })
			return
		}

		mq.Start()
	}) {
		_ = conn.Close()
		xlog.Warnf("[%v] accept task dropped; conn closed", listenAddr)
	}
}

func (r *tcpMsgQue) serverHandshake(netmgr *NetMgr, conn net.Conn) ([]byte, error) {
	if netmgr == nil {
		return nil, nil
	}

	if !netmgr.getOpt().EnableDH {
		return nil, nil
	}
	return r.dhKeyExchange(conn)
}

func (r *tcpMsgQue) connect(netmgr *NetMgr) {
	if r == nil || netmgr == nil {
		return
	}
	if r.conn != nil {
		// already connected (or in an abnormal state); do not start a new dial on the same object.
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

func (r *tcpMsgQue) connectOnce(netmgr *NetMgr) {
	opt := r.ensureOpt()
	if opt == nil || opt.ConnectParams == nil {
		return
	}
	addr := opt.ConnectParams.ConnectAddr
	svrType, svrId := opt.ConnectParams.SvrType, opt.ConnectParams.SvrId
	conn, err := net.DialTimeout("tcp", addr, time.Second*3)
	if err != nil {
		if svrType != "" {
			if netmgr.CanReconnect(opt.ConnectParams) {
				xlog.Warnf("connect [%v-%v-%v] failed : %s", svrType, svrId, addr, err.Error())
				r.scheduleReconnect(netmgr, svrType, svrId)
			} else {
				xlog.Infof("connect [%v-%v-%v] skipped (target unavailable).", svrType, svrId, addr)
			}
		}
		return
	}

	dhkey, err := r.clientHandshake(conn)
	if err != nil {
		_ = conn.Close()
		if netmgr.CanReconnect(opt.ConnectParams) {
			xlog.Warnf("dhKeyExchange err:%v", err)
			r.scheduleReconnect(netmgr, svrType, svrId)
		} else {
			xlog.Infof("connect [%v-%v-%v] skipped (target unavailable).", svrType, svrId, addr)
		}
		return
	}

	if !netmgr.addTask(func() {
		r.sessId = fastid.GenInt64ID()
		r.conn = conn
		r.discEvt = netmgr.sessOverEvt
		r.setDhKey(dhkey)
		netmgr.addSess(r)
		r.Start()

		xlog.Infof("[tcp] connect to %s-%v[%v] successfully.", svrType, svrId, addr)
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

func (r *tcpMsgQue) clientHandshake(conn net.Conn) ([]byte, error) {
	if !r.opt.EnableDH {
		return nil, nil
	}
	return r.dhKeyExchangeC(conn)
}

func (r *tcpMsgQue) scheduleReconnect(netmgr *NetMgr, svrType string, svrId int32) {
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

func (r *tcpMsgQue) stop() {
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

func (r *tcpMsgQue) Send(m *msg.Message) (re bool) {
	if r.stopping.Load() {
		return false
	}
	return r.msgQue.Send(m)
}

func (r *tcpMsgQue) notifyStop() {
	if r.handler != nil {
		r.handler.OnMsgQueStop(r)
	}
}

func (r *tcpMsgQue) waitIO() {
	r.writeWg.Wait()
	r.readWg.Wait()
}

func (r *tcpMsgQue) forceClose() {
	r.signalQuit()
	r.closeConn()
	r.closeListener()
}

func (r *tcpMsgQue) signalQuit() {
	r.quitOnce.Do(func() {
		if r.quit != nil {
			close(r.quit)
		}
	})
}

func (r *tcpMsgQue) closeListener() {
	if r.listener != nil {
		_ = r.listener.Close()
	}
}

func (r *tcpMsgQue) closeRead() {
	if r.conn == nil {
		return
	}
	if cr, ok := r.conn.(interface{ CloseRead() error }); ok {
		_ = cr.CloseRead()
	}
	_ = r.conn.SetReadDeadline(time.Now())
}

func (r *tcpMsgQue) isClosed() bool {
	select {
	case <-r.quit:
		return true
	default:
		return false
	}
}

func (r *tcpMsgQue) closeConn() {
	r.connOnce.Do(func() {
		if r.conn == nil {
			return
		}
		if cr, ok := r.conn.(interface{ CloseRead() error }); ok {
			_ = cr.CloseRead()
		}
		if cw, ok := r.conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		_ = r.conn.Close()
	})
}

func (r *tcpMsgQue) remoteAddr() string {
	if r.conn != nil {
		return r.conn.RemoteAddr().String()
	}
	return ""
}

func (r *tcpMsgQue) remoteIP() string {
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

func (r *tcpMsgQue) isInternalConn() bool {
	if r.connTyp == ConnTypeConn {
		return true
	}
	if r.opt != nil && r.opt.IsGate {
		return false
	}
	return true
}

func (r *tcpMsgQue) onDiscEvt(flag bool) {
	if !flag || r.stopping.Load() || r.discEvt == nil {
		return
	}
	r.discOnce.Do(func() {
		r.discEvt(r)
	})
}

func (r *tcpMsgQue) recordDisconnectErr(op string, err error) {
	if err == nil {
		return
	}
	r.discReasonMu.Lock()
	if r.discReason == "" {
		r.discReason = fmt.Sprintf("%s: %v", op, err)
	}
	r.discReasonMu.Unlock()
}

func (r *tcpMsgQue) getDisconnectReason() string {
	r.discReasonMu.RLock()
	reason := r.discReason
	r.discReasonMu.RUnlock()
	return reason
}

type readState struct {
	buf     []byte
	offset  int
	headLen int
	bodyLen int
	msg     *msg.Message
}

func newReadState(size int32) *readState {
	if size <= 0 {
		size = options.DEFAULT_BUFF_SIZE
	}
	min := int32(HEAD_SIZE + MAX_HEAD_LEN)
	if size < min {
		size = min
	}
	return &readState{
		buf: make([]byte, size),
	}
}

func (s *readState) resetFrame() {
	s.headLen = 0
	s.bodyLen = 0
	s.msg = nil
}

func (s *readState) grow(required int) {
	if required <= len(s.buf) {
		return
	}
	buf := make([]byte, required)
	copy(buf, s.buf[:s.offset])
	s.buf = buf
}

func (r *tcpMsgQue) readFromConn(state *readState) (int, error) {
	timeout := r.opt.Timeout
	if timeout > 0 {
		_ = r.conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout)))
	}

	n, err := r.conn.Read(state.buf[state.offset:])
	if err != nil {
		return n, err
	}
	if n > 0 {
		state.offset += n
	}
	return n, nil
}

func (r *tcpMsgQue) consumeReadBuffer(state *readState) bool {
	ptr := 0
	for {
		if state.headLen == 0 {
			if state.offset-ptr < HEAD_SIZE {
				break
			}
			state.headLen = int(binary.BigEndian.Uint16(state.buf[ptr : ptr+HEAD_SIZE]))
			if state.headLen > MAX_HEAD_LEN {
				xlog.Warnf("[%d] packet head len invalid: %d, stop msgque.", r.sessId, state.headLen)
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
				xlog.Warnf("[%d] packet head err. stop msgque.", r.sessId)
				return false
			}
			if head.BodyLen < 0 {
				xlog.Warnf("[%d] packet body len invalid: %d, stop msgque.", r.sessId, head.BodyLen)
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
			xlog.Warnf("msgque:%v process msg  failed, stop msgque. msgId:%v", r.sessId, state.msg.MsgId())
			return false
		}
		ptr += state.bodyLen
		state.resetFrame()
	}

	if ptr > state.offset {
		xlog.Errorf("something wrong for sess = %d, stop msgque", r.sessId)
		return false
	}
	if state.offset > ptr {
		copy(state.buf, state.buf[ptr:state.offset])
	}
	state.offset -= ptr
	return true
}

func (r *tcpMsgQue) ensureFrameCapacity(state *readState) bool {

	total := HEAD_SIZE + state.headLen + state.bodyLen
	max := int(r.opt.ReadSize)
	if max >= total {
		return true
	}

	if r.isInternalConn() {
		if total > max*4 {
			xlog.Warnf("[%d] internal packet too large head:%d body:%d max:%d", r.sessId, state.headLen, state.bodyLen, max*4)
			return false
		}
		state.grow(total)
	} else {
		// 外部连接严格限制包大小
		xlog.Warnf("[%d] packet too large head:%d body:%d max:%d", r.sessId, state.headLen, state.bodyLen, max)
		return false
	}

	return true
}

func (r *tcpMsgQue) handleReadMessage(state *readState, ptr int) bool {
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

func (r *tcpMsgQue) read() {
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

		n, err := r.readFromConn(state)
		if err != nil {
			if r.isClosed() {
				disconnected = false
				return
			}
			r.recordDisconnectErr("read", err)
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			xlog.Warnf("[%d]conn read failed : %s", r.sessId, err.Error())
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

func (r *tcpMsgQue) appendWriteBuffer(buffer *bytes.Buffer, m *msg.Message) error {
	if m == nil {
		return nil
	}
	mx := r.CompressOrEncrypt(m)
	_, err := mx.Bytes(buffer)
	return err
}

func (r *tcpMsgQue) newWriteBuffer() *bytes.Buffer {
	buffer := &bytes.Buffer{}
	if r.opt.WriteSize > 0 {
		buffer.Grow(int(r.opt.WriteSize))
	}
	return buffer
}

func (r *tcpMsgQue) drainWriteQueue(buffer *bytes.Buffer, count int) error {
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

type tcpWriteDelayCtrl struct {
	maxDelayMs int32
	watermark  int
	curDelayMs int32
	highStreak int
	lowStreak  int
}

func newTcpWriteDelayCtrl(maxDelayMs int32, watermark int) *tcpWriteDelayCtrl {
	ctrl := &tcpWriteDelayCtrl{}
	ctrl.Reset(maxDelayMs, watermark)
	return ctrl
}

func (c *tcpWriteDelayCtrl) Reset(maxDelayMs int32, watermark int) {
	if watermark <= 0 {
		watermark = 1
	}
	if maxDelayMs < 0 {
		maxDelayMs = 0
	}

	c.maxDelayMs = maxDelayMs
	c.watermark = watermark
	c.curDelayMs = maxDelayMs
	c.highStreak = 0
	c.lowStreak = 0
}

func (c *tcpWriteDelayCtrl) nextDelay(lastBatch int, allowDelay bool) time.Duration {
	if c.maxDelayMs <= 0 {
		return 0
	}

	if lastBatch > c.watermark {
		c.highStreak++
		c.lowStreak = 0
		if c.highStreak >= c.watermark/3 {
			c.curDelayMs /= 2
			if c.curDelayMs < 1 {
				c.curDelayMs = 1
			}
			c.highStreak = 0
		}
	} else {
		c.lowStreak++
		c.highStreak = 0
		if c.lowStreak >= c.watermark {
			if c.curDelayMs < c.maxDelayMs {
				c.curDelayMs++
			}
			if c.curDelayMs > c.maxDelayMs {
				c.curDelayMs = c.maxDelayMs
			}
			c.lowStreak = 0
		}
	}

	if !allowDelay || c.curDelayMs <= 0 {
		return 0
	}

	return time.Duration(c.curDelayMs) * time.Millisecond
}

func (r *tcpMsgQue) adaptiveWriteDelay(lastBatch int, allowDelay bool) time.Duration {
	return r.delayCtrl.nextDelay(lastBatch, allowDelay)
}

func (r *tcpMsgQue) collectWriteBatch(buffer *bytes.Buffer, first *msg.Message, allowDelay bool) error {
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

func (r *tcpMsgQue) flushWriteBuffer(buffer *bytes.Buffer) error {
	return r.flushWriteBufferWithTimeout(buffer, 0)
}

func (r *tcpMsgQue) flushWriteBufferWithTimeout(buffer *bytes.Buffer, timeout time.Duration) error {
	if buffer.Len() == 0 {
		return nil
	}

	opt := r.ensureOpt()

	// Protect against peers that stop reading.
	if timeout > 0 {
		_ = r.conn.SetWriteDeadline(time.Now().Add(timeout))
	} else if opt.Timeout > 0 {
		_ = r.conn.SetWriteDeadline(time.Now().Add(time.Second * time.Duration(opt.Timeout)))
	}

	data := buffer.Bytes()
	wn := 0
	for wn < len(data) {
		n, err := r.conn.Write(data[wn:])
		if err != nil {
			return err
		}
		wn += n
	}

	buffer.Reset()
	return nil
}

func (r *tcpMsgQue) write() {
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
					xlog.Warnf("[%d] packet write failed : %s", r.sessId, err.Error())
					return
				}
				if err := r.flushWriteBufferWithTimeout(buffer, closeTimeout); err != nil {
					r.recordDisconnectErr("write", err)
					xlog.Warnf("[%d]conn send failed : %s", r.sessId, err.Error())
					return
				}
			}
			if err := r.flushWriteBufferWithTimeout(buffer, closeTimeout); err != nil {
				r.recordDisconnectErr("write", err)
				xlog.Warnf("[%d]conn send failed : %s", r.sessId, err.Error())
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
				xlog.Warnf("[%d] packet write failed : %s", r.sessId, err.Error())
				return
			}
			if err := r.flushWriteBuffer(buffer); err != nil {
				if r.isClosed() {
					disconnected = false
					return
				}
				r.recordDisconnectErr("write", err)
				xlog.Warnf("[%d]conn send failed : %s", r.sessId, err.Error())
				return
			}
		}
	}
}

func (r *tcpMsgQue) CompressOrEncrypt(m *msg.Message) *msg.Message {

	// Broadcast messages might be reused across sessions; keep them immutable here.
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

	// Keep the original opt as immutable: do not write back defaults.
	newMsg := m.Copy()
	newMsg.Head.Flags |= msg.FlagCompress
	newMsg.Data = data
	newMsg.Head.BodyLen = int32(len(data))
	return newMsg
}

func (r *tcpMsgQue) Start() {
	r.ensureOpt()
	r.readWg.Add(1)
	go r.read()
	r.writeWg.Add(1)
	go r.write()
}

func newTcpAccept(conn net.Conn, handler INetEventHandler, opt *options.NetOptions) *tcpMsgQue {
	msgque := tcpMsgQue{
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

	if host, _, err := net.SplitHostPort(strings.TrimSpace(msgque.remoteAddr())); err == nil {
		msgque.agt.AddCltRemote(strings.TrimSpace(host))
	} else {
		// fallback (unexpected format)
		msgque.agt.AddCltRemote(strings.TrimSpace(msgque.remoteAddr()))
	}
	xlog.Debugf("sessId:[%d] new tcp accept from %s", msgque.sessId, msgque.remoteAddr())
	return &msgque
}

func newTcpConnect(handler INetEventHandler, opt *options.NetOptions) *tcpMsgQue {
	mq := &tcpMsgQue{
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
	xlog.Debugf("sessId:[%d] new tcp connect to %s", mq.sessId, mq.remoteAddr())
	return mq
}

func newTcpListen(listener net.Listener, opt *options.NetOptions, handler INetEventHandler) *tcpMsgQue {
	msgque := tcpMsgQue{
		msgQue: msgQue{
			sessId:  fastid.GenInt64ID(),
			connTyp: ConnTypeListen,
			agt:     newConnAgt(),
			handler: handler,
			opt:     opt,
		},
		listener: listener,
		quit:     make(chan struct{}),
	}
	return &msgque
}

const (
	PublicKeyLength = 65 // P-256 曲线的公钥长度 （0x04 + X + Y)

	WriteChanLowWatermark = 10
)

var ErrInvalidHandshakeProbe = errors.New("invalid handshake probe")

func isHTTPProbe(b []byte) bool {
	return bytes.HasPrefix(b, []byte("GET ")) ||
		bytes.HasPrefix(b, []byte("POST ")) ||
		bytes.HasPrefix(b, []byte("HEAD ")) ||
		bytes.HasPrefix(b, []byte("OPTIONS ")) ||
		bytes.HasPrefix(b, []byte("PUT ")) ||
		bytes.HasPrefix(b, []byte("DELETE "))
}

func (r *tcpMsgQue) dhKeyExchange(conn net.Conn) ([]byte, error) {
	// Set handshake deadline to avoid hanging forever.
	handshakeTimeout := 5 * time.Second
	if r.opt != nil && r.opt.Timeout > 0 {
		handshakeTimeout = time.Second * time.Duration(r.opt.Timeout)
	}
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	// --- 密钥交换流程 ---
	// a. 从客户端接收其公钥
	clientPubKeyBytes := make([]byte, PublicKeyLength)
	if _, err := io.ReadFull(conn, clientPubKeyBytes); err != nil {
		return nil, fmt.Errorf("read public key err:%v. remote:%v", err, conn.RemoteAddr().String())
	}

	if isHTTPProbe(clientPubKeyBytes) {
		return nil, ErrInvalidHandshakeProbe
	}

	curve := ecdh.P256()
	serverPrivKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private key err:%v. remote:%v", err, conn.RemoteAddr().String())
	}

	// b. 向客户端发送服务器的公钥
	if _, err := conn.Write(serverPrivKey.PublicKey().Bytes()); err != nil {
		return nil, fmt.Errorf("write public key err:%v. remote:%v", err, conn.RemoteAddr().String())
	}

	// c. 解析客户端的公钥
	clientPubKey, err := curve.NewPublicKey(clientPubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("new public key err:%v. clientPubKeyBytes:%v str:%v remote:%v",
			err, clientPubKeyBytes, hex.EncodeToString(clientPubKeyBytes), conn.RemoteAddr().String())
	}

	// d. 使用服务器的私钥和客户端的公钥计算共享密钥 (核心步骤)
	sharedSecret, err := serverPrivKey.ECDH(clientPubKey)
	if err != nil {
		return nil, fmt.Errorf("ecdh err:%v. remote:%v", err, conn.RemoteAddr().String())
	}
	return sharedSecret, nil
}

func (r *tcpMsgQue) dhKeyExchangeC(conn net.Conn) ([]byte, error) {
	// Set handshake deadline to avoid hanging forever.
	handshakeTimeout := 5 * time.Second
	if r.opt != nil && r.opt.Timeout > 0 {
		handshakeTimeout = time.Second * time.Duration(r.opt.Timeout)
	}
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	curve := ecdh.P256()
	clientPrivKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private key err:%v. remote:%v", err, conn.RemoteAddr().String())
	}
	// Do not log key material.
	// 发送公钥
	if _, err := conn.Write(clientPrivKey.PublicKey().Bytes()); err != nil {
		return nil, fmt.Errorf("write public key err:%v. remote:%v", err, conn.RemoteAddr().String())
	}

	// 接收公钥
	clientPubKeyBytes := make([]byte, PublicKeyLength)
	if _, err := io.ReadFull(conn, clientPubKeyBytes); err != nil {
		return nil, fmt.Errorf("read public key err:%v. remote:%v", err, conn.RemoteAddr().String())
	}

	// 解析公钥
	clientPubKey, err := curve.NewPublicKey(clientPubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("new public key err:%v. clientPubKeyBytes:%v str:%v remote:%v", err, clientPubKeyBytes, hex.EncodeToString(clientPubKeyBytes), conn.RemoteAddr().String())
	}

	// 使用服务器的私钥和客户端的公钥计算共享密钥 (核心步骤)
	sharedSecret, err := clientPrivKey.ECDH(clientPubKey)
	if err != nil {
		return nil, fmt.Errorf("ecdh err:%v. remote:%v", err, conn.RemoteAddr().String())
	}
	return sharedSecret, nil
}
