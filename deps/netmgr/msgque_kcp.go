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
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	kcp "github.com/xtaci/kcp-go/v5"
)

type kcpMsgQue struct {
	msgQue
	conn           net.Conn
	listener       net.Listener
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

func listenKCP(addr string) (net.Listener, error) {
	return kcp.ListenWithOptions(addr, nil, 0, 0)
}

func configureKCPConn(conn net.Conn) {
	if sess, ok := conn.(interface{ SetStreamMode(bool) }); ok {
		sess.SetStreamMode(true)
	}
}

func (r *kcpMsgQue) ensureOpt() *options.NetOptions {
	if r.opt == nil {
		r.opt = options.NewMsgQueOptions()
		r.opt.Transport = options.TransportKCP
	}
	return r.opt
}

func (r *kcpMsgQue) listen(netmgr *NetMgr) {
	basal.SafeGo(func() {
		r.acceptLoop(netmgr)
	})
}

func (r *kcpMsgQue) acceptLoop(netmgr *NetMgr) {
	for {
		conn, err := r.listener.Accept()
		if err != nil || conn == nil {
			if r.isClosed() {
				return
			}
			xlog.Errorf("kcp msgque accept err:%v", err)
			continue
		}
		configureKCPConn(conn)
		basal.SafeGo(func() {
			r.handleAcceptConn(netmgr, conn)
		})
	}
}

func (r *kcpMsgQue) handleAcceptConn(netmgr *NetMgr, conn net.Conn) {
	listenAddr := netmgr.getOpt().ListenParams.ListenAddr
	mLoad := netmgr.getOpt().ListenParams.MaxConn

	if mLoad > 0 && netmgr.GetAcceptSessNum() >= mLoad {
		basal.SafeGo(func() { _ = conn.Close() })
		xlog.Warnf("[%v] kcp conn overload with limit=%v", listenAddr, mLoad)
		return
	}

	dhKey, err := r.serverHandshake(netmgr, conn)
	if err != nil {
		_ = conn.Close()
		xlog.Warnf("kcp dhKeyExchange err:%v", err)
		return
	}

	if !netmgr.addTask(func() {
		mq := newKCPAccept(conn, r.handler, netmgr.getOpt())
		mq.discEvt = netmgr.sessOverEvt
		mq.setDhKey(dhKey)

		netmgr.addSess(mq)
		if mq.handler != nil && !mq.handler.OnNewMsgQue(mq) {
			netmgr.deleteSess(mq.SessId())
			basal.SafeGo(func() { mq.stop() })
			return
		}

		mq.Start()
	}) {
		_ = conn.Close()
		xlog.Warnf("[%v] kcp accept task dropped; conn closed", listenAddr)
	}
}

func (r *kcpMsgQue) serverHandshake(netmgr *NetMgr, conn net.Conn) ([]byte, error) {
	if netmgr == nil || !netmgr.getOpt().EnableDH {
		return nil, nil
	}
	return (&tcpMsgQue{msgQue: msgQue{opt: r.opt}}).dhKeyExchange(conn)
}

func (r *kcpMsgQue) connect(netmgr *NetMgr) {
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

func (r *kcpMsgQue) connectOnce(netmgr *NetMgr) {
	opt := r.ensureOpt()
	if opt == nil || opt.ConnectParams == nil {
		return
	}

	addr := opt.ConnectParams.ConnectAddr
	svrType, svrId := opt.ConnectParams.SvrType, opt.ConnectParams.SvrId
	conn, err := kcp.DialWithOptions(addr, nil, 0, 0)
	if err != nil {
		if svrType != "" {
			if netmgr.CanReconnect(opt.ConnectParams) {
				xlog.Warnf("connect [%v-%v-%v][kcp] failed : %s", svrType, svrId, addr, err.Error())
				r.scheduleReconnect(netmgr, svrType, svrId)
			} else {
				xlog.Infof("connect [%v-%v-%v][kcp] skipped (target unavailable).", svrType, svrId, addr)
			}
		}
		return
	}
	configureKCPConn(conn)

	dhkey, err := r.clientHandshake(conn)
	if err != nil {
		_ = conn.Close()
		if netmgr.CanReconnect(opt.ConnectParams) {
			xlog.Warnf("kcp dhKeyExchange err:%v", err)
			r.scheduleReconnect(netmgr, svrType, svrId)
		} else {
			xlog.Infof("connect [%v-%v-%v][kcp] skipped (target unavailable).", svrType, svrId, addr)
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

		xlog.Infof("[kcp] connect to %s-%v[%v] successfully.", svrType, svrId, addr)
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

func (r *kcpMsgQue) clientHandshake(conn net.Conn) ([]byte, error) {
	if !r.opt.EnableDH {
		return nil, nil
	}
	return (&tcpMsgQue{msgQue: msgQue{opt: r.opt}}).dhKeyExchangeC(conn)
}

func (r *kcpMsgQue) scheduleReconnect(netmgr *NetMgr, svrType string, svrId int32) {
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

func (r *kcpMsgQue) stop() {
	r.stopOnce.Do(func() {
		r.stopping.Store(true)
		r.notifyStop()
		r.signalQuit()
		r.closeListener()
		r.writeWg.Wait()
		r.closeRead()
		r.readWg.Wait()
		r.closeConn()
	})
}

func (r *kcpMsgQue) Send(m *msg.Message) (re bool) {
	if r.stopping.Load() {
		return false
	}
	return r.msgQue.Send(m)
}

func (r *kcpMsgQue) notifyStop() {
	if r.handler != nil {
		r.handler.OnMsgQueStop(r)
	}
}

func (r *kcpMsgQue) forceClose() {
	r.signalQuit()
	r.closeConn()
	r.closeListener()
}

func (r *kcpMsgQue) signalQuit() {
	r.quitOnce.Do(func() {
		if r.quit != nil {
			close(r.quit)
		}
	})
}

func (r *kcpMsgQue) closeListener() {
	if r.listener != nil {
		_ = r.listener.Close()
	}
}

func (r *kcpMsgQue) closeRead() {
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

func (r *kcpMsgQue) isClosed() bool {
	select {
	case <-r.quit:
		return true
	default:
		return false
	}
}

func (r *kcpMsgQue) closeConn() {
	r.connOnce.Do(func() {
		if r.conn != nil {
			_ = r.conn.Close()
		}
	})
}

func (r *kcpMsgQue) remoteAddr() string {
	if r.conn != nil && r.conn.RemoteAddr() != nil {
		return r.conn.RemoteAddr().String()
	}
	return ""
}

func (r *kcpMsgQue) remoteIP() string {
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

func (r *kcpMsgQue) isInternalConn() bool {
	if r.connTyp == ConnTypeConn {
		return true
	}
	if r.opt != nil && r.opt.IsGate {
		return false
	}
	return true
}

func (r *kcpMsgQue) onDiscEvt(flag bool) {
	if !flag || r.stopping.Load() || r.discEvt == nil {
		return
	}
	r.discOnce.Do(func() {
		r.discEvt(r)
	})
}

func (r *kcpMsgQue) recordDisconnectErr(op string, err error) {
	if err == nil {
		return
	}
	r.discReasonMu.Lock()
	if r.discReason == "" {
		r.discReason = fmt.Sprintf("%s: %v", op, err)
	}
	r.discReasonMu.Unlock()
}

func (r *kcpMsgQue) getDisconnectReason() string {
	r.discReasonMu.RLock()
	reason := r.discReason
	r.discReasonMu.RUnlock()
	return reason
}

func (r *kcpMsgQue) readFromConn(state *readState) (int, error) {
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

func (r *kcpMsgQue) consumeReadBuffer(state *readState) bool {
	ptr := 0
	for {
		if state.headLen == 0 {
			if state.offset-ptr < HEAD_SIZE {
				break
			}
			state.headLen = int(binary.BigEndian.Uint16(state.buf[ptr : ptr+HEAD_SIZE]))
			if state.headLen > MAX_HEAD_LEN {
				xlog.Warnf("[%d] kcp packet head len invalid: %d, stop msgque.", r.sessId, state.headLen)
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
				xlog.Warnf("[%d] kcp packet head err. stop msgque.", r.sessId)
				return false
			}
			if head.BodyLen < 0 {
				xlog.Warnf("[%d] kcp packet body len invalid: %d, stop msgque.", r.sessId, head.BodyLen)
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
			xlog.Warnf("kcp msgque:%v process msg failed, stop msgque. msgId:%v", r.sessId, state.msg.MsgId())
			return false
		}
		ptr += state.bodyLen
		state.resetFrame()
	}

	if ptr > state.offset {
		xlog.Errorf("something wrong for kcp sess = %d, stop msgque", r.sessId)
		return false
	}
	if state.offset > ptr {
		copy(state.buf, state.buf[ptr:state.offset])
	}
	state.offset -= ptr
	return true
}

func (r *kcpMsgQue) ensureFrameCapacity(state *readState) bool {
	total := HEAD_SIZE + state.headLen + state.bodyLen
	max := int(r.opt.ReadSize)
	if max >= total {
		return true
	}

	if r.isInternalConn() {
		if total > max*4 {
			xlog.Warnf("[%d] internal kcp packet too large head:%d body:%d max:%d", r.sessId, state.headLen, state.bodyLen, max*4)
			return false
		}
		state.grow(total)
	} else {
		xlog.Warnf("[%d] kcp packet too large head:%d body:%d max:%d", r.sessId, state.headLen, state.bodyLen, max)
		return false
	}
	return true
}

func (r *kcpMsgQue) handleReadMessage(state *readState, ptr int) bool {
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

func (r *kcpMsgQue) read() {
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
			xlog.Warnf("[%d] kcp conn read failed : %s", r.sessId, err.Error())
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

func (r *kcpMsgQue) appendWriteBuffer(buffer *bytes.Buffer, m *msg.Message) error {
	if m == nil {
		return nil
	}
	mx := r.CompressOrEncrypt(m)
	_, err := mx.Bytes(buffer)
	return err
}

func (r *kcpMsgQue) newWriteBuffer() *bytes.Buffer {
	buffer := &bytes.Buffer{}
	if r.opt.WriteSize > 0 {
		buffer.Grow(int(r.opt.WriteSize))
	}
	return buffer
}

func (r *kcpMsgQue) drainWriteQueue(buffer *bytes.Buffer, count int) error {
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

func (r *kcpMsgQue) adaptiveWriteDelay(lastBatch int, allowDelay bool) time.Duration {
	return r.delayCtrl.nextDelay(lastBatch, allowDelay)
}

func (r *kcpMsgQue) collectWriteBatch(buffer *bytes.Buffer, first *msg.Message, allowDelay bool) error {
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

func (r *kcpMsgQue) flushWriteBuffer(buffer *bytes.Buffer) error {
	return r.flushWriteBufferWithTimeout(buffer, 0)
}

func (r *kcpMsgQue) flushWriteBufferWithTimeout(buffer *bytes.Buffer, timeout time.Duration) error {
	if buffer.Len() == 0 {
		return nil
	}

	opt := r.ensureOpt()
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

func (r *kcpMsgQue) write() {
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
					xlog.Warnf("[%d] kcp packet write failed : %s", r.sessId, err.Error())
					return
				}
				if err := r.flushWriteBufferWithTimeout(buffer, closeTimeout); err != nil {
					r.recordDisconnectErr("write", err)
					xlog.Warnf("[%d] kcp conn send failed : %s", r.sessId, err.Error())
					return
				}
			}
			if err := r.flushWriteBufferWithTimeout(buffer, closeTimeout); err != nil {
				r.recordDisconnectErr("write", err)
				xlog.Warnf("[%d] kcp conn send failed : %s", r.sessId, err.Error())
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
				xlog.Warnf("[%d] kcp packet write failed : %s", r.sessId, err.Error())
				return
			}
			if err := r.flushWriteBuffer(buffer); err != nil {
				if r.isClosed() {
					disconnected = false
					return
				}
				r.recordDisconnectErr("write", err)
				xlog.Warnf("[%d] kcp conn send failed : %s", r.sessId, err.Error())
				return
			}
		}
	}
}

func (r *kcpMsgQue) CompressOrEncrypt(m *msg.Message) *msg.Message {
	return (&tcpMsgQue{msgQue: r.msgQue}).CompressOrEncrypt(m)
}

func (r *kcpMsgQue) Start() {
	r.ensureOpt()
	r.readWg.Add(1)
	go r.read()
	r.writeWg.Add(1)
	go r.write()
}

func newKCPAccept(conn net.Conn, handler INetEventHandler, opt *options.NetOptions) *kcpMsgQue {
	mq := &kcpMsgQue{
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
	xlog.Debugf("sessId:[%d] new kcp accept from %s", mq.sessId, mq.remoteAddr())
	return mq
}

func newKCPConnect(handler INetEventHandler, opt *options.NetOptions) IMsgQue {
	mq := &kcpMsgQue{
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
	xlog.Debugf("sessId:[%d] new kcp connect to %s", mq.sessId, mq.remoteAddr())
	return mq
}

func newKCPListen(listener net.Listener, opt *options.NetOptions, handler INetEventHandler) IMsgQue {
	mq := &kcpMsgQue{
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
	return mq
}
