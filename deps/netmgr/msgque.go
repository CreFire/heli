package netmgr

import (
	"game/deps/basal"
	"game/deps/encrypt"
	"game/deps/msg"
	"game/deps/netmgr/options"
	"game/deps/xlog"
	"game/src/proto/pb"

	"github.com/samber/lo"
)

const (
	HEAD_SIZE                = 2
	MAX_HEAD_LEN             = 1024
	NET_COMPRESS_MODE_GZIP   = 0
	NET_COMPRESS_MODE_SNAPPY = 1
)

type MsgType int

const (
	MsgTypeMsg MsgType = iota //消息基于确定的消息头
)

type ConnType int

const (
	ConnTypeListen ConnType = iota //监听
	ConnTypeConn                   //连接产生的
	ConnTypeAccept                 //Accept产生的
)

type IMsgQue interface {
	SessId() int64
	GetAgent() *ConnAgt
	Send(m *msg.Message) (re bool)
	GetConnectType() ConnType

	listen(mgr *NetMgr)
	connect(mgr *NetMgr)
	stop()
	remoteAddr() string
	remoteIP() string
	getOpt() *options.NetOptions
	getHandler() INetEventHandler
	getTransportType() options.Transport
	getDisconnectReason() string
	getDhKey() []byte
	setDhKey(key []byte)
	setOpt(opt *options.NetOptions)
}

type msgQue struct {
	sessId  int64             //唯一标示
	cwrite  chan *msg.Message //写入通道
	connTyp ConnType          //通道类型
	handler INetEventHandler  //处理者
	agt     *ConnAgt
	opt     *options.NetOptions
	dhKey   []byte
}

func (r *msgQue) SessId() int64 {
	return r.sessId
}
func (r *msgQue) GetAgent() *ConnAgt {
	return r.agt
}
func (r *msgQue) SendLen() int                   { return len(r.cwrite) }
func (r *msgQue) GetConnectType() ConnType       { return r.connTyp }
func (r *msgQue) setOpt(opt *options.NetOptions) { r.opt = opt }
func (r *msgQue) getOpt() *options.NetOptions    { return r.opt }
func (r *msgQue) getHandler() INetEventHandler   { return r.handler }
func (r *msgQue) getTransportType() options.Transport {
	if r.opt == nil || r.opt.Transport == "" {
		return options.TransportTCP
	}
	return r.opt.Transport
}
func (r *msgQue) getDisconnectReason() string { return "" }
func (r *msgQue) getDhKey() []byte            { return r.dhKey }
func (r *msgQue) setDhKey(key []byte)         { r.dhKey = key }
func (r *msgQue) Send(m *msg.Message) (re bool) {
	if m == nil {
		return
	}
	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		gid := r.agt.GetCltUser()
		gid = lo.Ternary(gid <= 0, m.GId(), gid)
		svrType, svrId := r.GetAgent().GetSvrAgt()
		xlog.Debugf("msgque send msgId: %v, sessId: %v, gid: %d, svr: %s %d,  send_que_len: %v, msg:[ %s ]",
			pb.MSG_ID(m.MsgId()), r.sessId, gid, svrType, svrId, len(r.cwrite), m.ToString())
	}

	select {
	case r.cwrite <- m:
	default:
		xlog.Warnf("msgque cwrite full sessId: %v, msgId: %v, len: %v", r.sessId, m.MsgId(), len(r.cwrite))
		return false
	}

	return true
}

func (r *msgQue) processMsg(msgque IMsgQue, data *msg.Message) bool {
	if data == nil || data.Head == nil {
		// No head means we cannot safely process flags / msg id.
		// Treat it as invalid frame.
		return false
	}

	//作为客户端时--robot
	if r.connTyp == ConnTypeConn && data.MsgId() == 0 {
		return true
	}

	if data.FlagBits&msg.FlagEncrypt > 0 && data.Data != nil {
		newData, err := encrypt.AesDecodeData(data.Data, r.getDhKey())
		if err != nil {
			xlog.Warnf("AesDecodeData failed sessId: %v msgId: %v err: %v", r.sessId, data.MsgId(), err)
			return false
		}
		data.FlagBits &^= msg.FlagEncrypt
		data.Data = newData
		data.Head.BodyLen = int32(len(data.Data))
	}

	if data.FlagBits&msg.FlagCompress > 0 && data.Data != nil {
		var newBuffer []byte
		var err error
		mode := NET_COMPRESS_MODE_GZIP
		if r.opt != nil {
			mode = int(r.opt.CompressMode)
		}
		switch int32(mode) {
		case NET_COMPRESS_MODE_GZIP:
			newBuffer, err = basal.GZipDecompress(data.Data)
		case NET_COMPRESS_MODE_SNAPPY:
			newBuffer, err = basal.SnappyDecompress(data.Data)
		}

		if err != nil {
			xlog.Errorf("msgque uncompress failed msgque:%v msgId:%v len:%v err:%v", msgque.SessId(), data.MsgId(), data.Head.BodyLen, err)
			return false
		}
		data.Data = newBuffer
		data.Head.BodyLen = int32(len(data.Data))
		data.FlagBits &^= msg.FlagCompress
	}

	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		gid := r.agt.GetCltUser()
		gid = lo.Ternary(gid <= 0, data.GId(), gid)
		svrType, svrId := r.GetAgent().GetSvrAgt()
		xlog.Debugf("msgque recv msgId: %v, sessId: %v, gid: %d , svr: %s %d, msg:[ %v ]",
			pb.MSG_ID(data.MsgId()), r.sessId, gid, svrType, svrId, data.ToString())
	}

	// 处理消息
	if r.handler != nil {
		return r.handler.OnProcessMsg(msgque, data)
	}
	return true
}
