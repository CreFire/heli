package options

import (
	"fmt"
	"game/src/configdoc"
	"strings"
)

type Transport string

const (
	TransportTCP       Transport = "tcp"
	TransportWebSocket Transport = "websocket"
	TransportKCP       Transport = "kcp"
	DefaultWSPath                = "/ws"
)

type ConnectParams struct {
	ConnectAddr  string
	ReConnectInv int64
	SvrType      string
	SvrId        int32
}

func (param *ConnectParams) SetConnectAddr(addr string) *ConnectParams {
	param.ConnectAddr = addr
	return param
}

func (param *ConnectParams) SetSvrType(svrType string) *ConnectParams {
	param.SvrType = svrType
	return param
}

func (param *ConnectParams) SetSvrId(svrId int32) *ConnectParams {
	param.SvrId = svrId
	return param
}

func (param *ConnectParams) SetReConnectInv(reConnectInv int64) *ConnectParams {
	param.ReConnectInv = reConnectInv
	return param
}

func (param *ConnectParams) Check() error {
	if param.ConnectAddr == "" {
		return fmt.Errorf("connection address cannot be empty")
	}
	if param.SvrType == "" {
		return fmt.Errorf("connection server type cannot be empty")
	}
	if param.SvrId == 0 {
		return fmt.Errorf("connection server id cannot be empty")
	}
	return nil
}

func NewConnectParams(addr, svrType string, svrId int32) *ConnectParams {
	return &ConnectParams{
		ConnectAddr:  addr,
		SvrType:      svrType,
		SvrId:        svrId,
		ReConnectInv: NET_RECONNECT_INV,
	}
}

type ListenParams struct {
	ListenAddr string
	MaxConn    int // 允许最大连接数
}

func (param *ListenParams) Check() error {
	if param.ListenAddr == "" {
		return fmt.Errorf("listen address cannot be empty")
	}
	return nil
}

func (param *ListenParams) SetMaxConn(maxConn int) *ListenParams {
	param.MaxConn = maxConn
	return param
}

func NewListenParams(listenAddr string) *ListenParams {
	return &ListenParams{ListenAddr: listenAddr}
}

type NetOptions struct {
	// not support reload
	ConnectParams *ConnectParams
	ListenParams  *ListenParams
	Transport     Transport
	WSPath        string
	// support reload

	Timeout       int32 // 超时时间
	ReadSize      int32 // 读取缓冲区大小
	WriteSize     int32 // 写入缓冲区大小
	WriteChanSize int32 // 写入通道大小
	IsGate        bool  // gate 对外监听连接
	Compress      bool  // 是否压缩
	CompressMode  int32 // 压缩方式
	CompressLimit int32
	EnableDH      bool // 是否启用Diffie-Hellman密钥交换
	DelayWrite    int32
}

func NewMsgQueOptions() *NetOptions {
	return &NetOptions{
		ReadSize:      DEFAULT_BUFF_SIZE,
		WriteChanSize: WRITE_CHAN_SIZE_S,
		WriteSize:     DEFAULT_BUFF_SIZE,
		DelayWrite:    DELAY_WRITE_MS,
		Transport:     TransportTCP,
		WSPath:        DefaultWSPath,
	}
}

func (opt *NetOptions) CheckOptions() error {
	if opt.ConnectParams != nil {
		if err := opt.ConnectParams.Check(); err != nil {
			return err
		}
	}
	if opt.ListenParams != nil {
		if err := opt.ListenParams.Check(); err != nil {
			return err
		}
	}
	if opt.ReadSize > 0 && opt.ReadSize < MIN_READ_SIZE {
		return fmt.Errorf("read buffer size too small: %d (min %d)", opt.ReadSize, MIN_READ_SIZE)
	}
	if opt.Transport == "" {
		opt.Transport = TransportTCP
	}
	if opt.Transport == TransportWebSocket && opt.WSPath == "" {
		return fmt.Errorf("websocket path cannot be empty")
	}
	return nil
}

func (opt *NetOptions) SetListenParams(params *ListenParams) *NetOptions {
	opt.ListenParams = params
	return opt
}

func (opt *NetOptions) SetConnectParams(params *ConnectParams) *NetOptions {
	opt.ConnectParams = params
	return opt
}

func (opt *NetOptions) SetTimeout(timeout int32) *NetOptions {
	opt.Timeout = timeout
	return opt
}

func (opt *NetOptions) SetReadSize(readSize int32) *NetOptions {
	if readSize <= 0 {
		readSize = DEFAULT_BUFF_SIZE
	}
	if readSize < MIN_READ_SIZE {
		readSize = MIN_READ_SIZE
	}
	opt.ReadSize = readSize
	return opt
}

func (opt *NetOptions) SetWriteSize(writeSize int32) *NetOptions {
	if writeSize <= 0 {
		writeSize = DEFAULT_BUFF_SIZE
	}
	opt.WriteSize = writeSize
	return opt
}

func (opt *NetOptions) SetWriteChanSize(writeChanSize int32) *NetOptions {
	if writeChanSize <= 0 {
		writeChanSize = WRITE_CHAN_SIZE_S
	}
	opt.WriteChanSize = writeChanSize
	return opt
}

func (opt *NetOptions) SetTransport(transport Transport) *NetOptions {
	opt.Transport = transport
	return opt
}

func (opt *NetOptions) SetWSPath(path string) *NetOptions {
	opt.WSPath = path
	return opt
}

func (opt *NetOptions) SetIsGate(isGate bool) *NetOptions {
	opt.IsGate = isGate
	return opt
}

func (opt *NetOptions) SetCompress(compress bool) *NetOptions {
	opt.Compress = compress
	return opt
}

func (opt *NetOptions) SetCompressMode(mode int32) *NetOptions {
	opt.CompressMode = mode
	return opt
}

func (opt *NetOptions) SetCompressLimit(compressLimit int32) *NetOptions {
	if compressLimit <= 0 {
		compressLimit = COMPRESS_LIMIT
	}
	opt.CompressLimit = compressLimit
	return opt
}

func (opt *NetOptions) SetEnableDH(enableDH bool) *NetOptions {
	opt.EnableDH = enableDH
	return opt
}

func (opt *NetOptions) SetDelayWrite(delayWrite int32) *NetOptions {
	opt.DelayWrite = delayWrite
	return opt
}

func (opt *NetOptions) SetNetCfg(cfg *configdoc.Net) *NetOptions {
	if cfg == nil {
		return opt
	}
	if transport := normalizeNetCfgTransport(cfg.Transport); transport != "" {
		opt.SetTransport(transport)
	}
	if cfg.WSPath != "" {
		opt.SetWSPath(cfg.WSPath)
	}
	if opt.ListenParams != nil {
		opt.ListenParams.SetMaxConn(int(cfg.MaxConn))
	}
	opt.SetTimeout(cfg.CltReadTimeout)
	opt.SetReadSize(cfg.CltReadBufferSize)
	opt.SetWriteSize(cfg.CltWriteBufferSize)
	opt.SetWriteChanSize(cfg.CltWriteChanSize)
	opt.SetEnableDH(cfg.CltEnableDH)
	opt.SetCompress(cfg.Compress)
	opt.SetCompressMode(cfg.CompressMode)
	opt.SetCompressLimit(cfg.CompressLimit)
	opt.SetDelayWrite(cfg.DelayWrite)
	return opt
}

func normalizeNetCfgTransport(transport string) Transport {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "", string(TransportTCP):
		return TransportTCP
	case "ws", string(TransportWebSocket):
		return TransportWebSocket
	case string(TransportKCP):
		return TransportKCP
	default:
		return Transport(transport)
	}
}

func MergeOptions(opts ...*NetOptions) *NetOptions {
	opt := NewMsgQueOptions()
	for _, o := range opts {
		if o == nil {
			continue
		}
		if o.Transport != "" {
			opt.Transport = o.Transport
		}
		if o.WSPath != "" {
			opt.WSPath = o.WSPath
		}
		break
	}

	for _, o := range opts {
		if o == nil {
			continue
		}
		if o.Timeout != 0 {
			opt.Timeout = o.Timeout
		}
		if o.ReadSize != 0 {
			opt.ReadSize = o.ReadSize
		}
		if o.WriteSize != 0 {
			opt.WriteSize = o.WriteSize
		}
		if o.WriteChanSize != 0 {
			opt.WriteChanSize = o.WriteChanSize
		}
		opt.CompressMode = o.CompressMode
		if o.CompressLimit != 0 {
			opt.CompressLimit = o.CompressLimit
		}
		// Bool fields: allow turning features OFF during reload.
		opt.IsGate = o.IsGate
		opt.Compress = o.Compress
		opt.EnableDH = o.EnableDH
		if o.DelayWrite != 0 {
			opt.DelayWrite = o.DelayWrite
		}
	}
	return opt
}
