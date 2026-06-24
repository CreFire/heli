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
		HandshakeTimeout: 15 * time.Second,
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

// readFromConn 从 websocket 连接中读取原始字节并追加到 state.buf。
//
// 和 tcp conn.Read 不同，websocket 底层存在 message/frame 边界，因此这里分两层处理：
// 1. 先通过 NextReader() 获取当前二进制 frame 的 reader；
// 2. 再从该 reader 中连续读取 payload，直到读满、超时或 frame 结束。
//
// 注意：这里把单个 websocket frame 的 io.EOF 视为“当前 frame 读完”，而不是“连接断开”。
// 如果本次 EOF 之前已经读到字节，则先把有效数据交给上层拆包；若本次未读到字节，则继续切到下一帧。
func (r *wsMsgQue) readFromConn(state *readState) (int, error) {
	for {
		if r.reader == nil {
			// 当前没有正在消费的 websocket frame，需要先切到下一条 binary message。
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

		// 对当前 frame 的实际读取也单独刷新读超时，避免长时间阻塞在 Read 上。
		timeout := r.opt.Timeout
		if timeout > 0 {
			_ = r.conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout)))
		}
		n, err := r.reader.Read(state.buf[state.offset:])
		if n > 0 {
			// 把本次读取到的字节追加到累计缓冲区尾部。
			state.offset += n
		}
		if errors.Is(err, io.EOF) {
			// EOF 在 websocket reader 语义下表示“当前 frame 读完”，不是整个连接断开。
			r.reader = nil
			if n > 0 {
				// 当前 frame 最后一段数据依然有效，先返回给上层做拆包。
				return n, nil
			}
			// 当前 frame 恰好空读结束，继续获取下一帧。
			continue
		}
		return n, err
	}
}

// ensureFrameCapacity 在消息头解析完成后，校验当前完整业务包所需的缓冲区容量。
//
// total = 固定 2 字节 headLen + 消息头长度 + 消息体长度。
// 对外部客户端连接，包长超过配置读缓冲上限时直接拒绝；
// 对内部服务连接，允许在 4 倍上限以内按需扩容，兼顾联机服务间传输弹性。
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

// handleReadMessage 把缓冲区中已经收全的一条消息组装成 msg.Message 并交给上层处理。
//
// 这里会把 body 拷贝到新的切片中，避免后续 read()/搬移缓冲区时覆盖当前消息内容。
// 当 bodyLen 为 0 时，说明这是一个仅包含消息头的空包，可直接进入 processMsg。
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

// consumeReadBuffer 会从累计读缓冲中尽可能多地拆出完整业务包并顺序处理。
//
// 当前项目的 websocket 传输虽然底层有 frame 边界，但这里仍按统一的自定义包格式解析：
// 1. 先读固定 2 字节的 headLen；
// 2. 再按 headLen 解析消息头；
// 3. 再按消息头中的 BodyLen 读取消息体；
// 4. 每组装出一条完整消息后，交给 handleReadMessage/processMsg 处理。
//
// 这个函数只消费 state.buf[0:state.offset] 中“已经收到”的数据：
// - 数据不足一个完整字段时直接 break，等待后续 read() 继续补数据；
// - 成功消费的前缀数据会在函数末尾整体左移；
// - 返回 false 表示拆包或处理过程中出现致命异常，调用方应停止当前 msgque。
func (r *wsMsgQue) consumeReadBuffer(state *readState) bool {
	// ptr 表示本轮已经从缓冲区前部消费了多少字节。
	// 注意它和 state.offset 不同：
	// - ptr 是“已解析游标”；
	// - state.offset 是“当前缓冲区累计有效字节数”。
	ptr := 0
	for {
		if state.headLen == 0 {
			// 连固定 2 字节 headLen 都不够，说明还不能开始解析下一帧，等后续数据到达。
			if state.offset-ptr < HEAD_SIZE {
				break
			}
			// 读取消息头长度。协议使用大端编码。
			state.headLen = int(binary.BigEndian.Uint16(state.buf[ptr : ptr+HEAD_SIZE]))
			if state.headLen > MAX_HEAD_LEN {
				xlog.Warnf("[%d] websocket packet head len invalid: %d, stop msgque.", r.sessId, state.headLen)
				return false
			}

			// 固定长度字段已消费，游标跳过 HEAD_SIZE。
			ptr += HEAD_SIZE
		}

		if state.bodyLen == 0 {
			// 已经知道 headLen，但当前缓冲区还放不下完整消息头，继续等后续数据。
			if state.offset-ptr < state.headLen {
				break
			}
			message := &msg.Message{}
			// 用项目统一的 MessageHead 解析逻辑，从缓冲区中还原消息头。
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
			// 消息头已经消费完，游标继续跳过 headLen。
			ptr += state.headLen
			state.bodyLen = int(head.BodyLen)
			// 检查整个包（固定头 + 消息头 + 消息体）是否超出允许上限；
			// 对内部连接允许适度扩容，对外部连接则严格限制。
			if !r.ensureFrameCapacity(state) {
				return false
			}
		}

		// 消息头已齐，但消息体还没收全，等待下次 read() 继续补字节。
		if state.offset-ptr < state.bodyLen {
			break
		}

		// 到这里说明一整条消息已经完整落在缓冲区中，可以进入上层处理。
		if !r.handleReadMessage(state, ptr) {
			xlog.Warnf("websocket msgque:%v process msg failed, stop msgque. msgId:%v", r.sessId, state.msg.MsgId())
			return false
		}
		// 消费完 body，游标后移，继续尝试解析缓冲区里后续可能连在一起的消息。
		ptr += state.bodyLen
		// 清空当前帧状态，准备解析下一条消息。
		state.resetFrame()
	}

	// 正常情况下已消费游标不能超过有效数据边界；超出说明内部状态错乱。
	if ptr > state.offset {
		xlog.Errorf("something wrong for websocket sess = %d, stop msgque", r.sessId)
		return false
	}
	if state.offset > ptr {
		// 把未消费的残留字节左移到缓冲区起始处，供下次 read() 继续拼包。
		copy(state.buf, state.buf[ptr:state.offset])
	}
	// 更新缓冲区当前有效长度。
	state.offset -= ptr
	return true
}

// read 是 websocket 收包主循环：负责持续读取底层字节、累计到缓冲区，并驱动统一拆包流程。
//
// 整体流程：
// 1. 检查连接是否已关闭；
// 2. 确保读缓冲区仍有空间，必要时仅对内部连接扩容；
// 3. 调用 readFromConn 读取 websocket binary frame 数据；
// 4. 调用 consumeReadBuffer 按项目协议拆出完整消息并处理。
//
// 一旦出现连接关闭、读错误、拆包失败或消息处理失败，read goroutine 会退出；
// defer 中再统一负责关闭底层资源并触发断线事件。
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
				// 内部连接允许按需扩容，避免大包直接把链路打断。
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
