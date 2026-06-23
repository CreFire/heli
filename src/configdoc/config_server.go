package configdoc

type ServerCfg struct {
	Id      int32   `yaml:"id"`
	Type    string  `yaml:"type"`
	Ip      string  `yaml:"ip"`
	Port    int32   `yaml:"port"`
	Cluster string  `yaml:"cluster"`
	Net     *Net    `yaml:"net"`
	Auth    *Auth   `yaml:"auth"`
	Log     *LogCfg `yaml:"log"`
}
type Net struct {
	Transport          string `yaml:"transport"`          // 网络传输类型，空值默认 TCP；可配置 tcp / websocket / kcp 等实现支持的类型。
	WSPath             string `yaml:"wsPath"`             // WebSocket 监听路径，仅在 websocket 传输类型下生效。
	MaxConn            int32  `yaml:"maxConn"`            // 最大连接数限制，用于控制服务端可接受的客户端连接规模。
	CltReadBufferSize  int32  `yaml:"cltReadBufferSize"`  // 客户端连接读缓冲区大小。
	CltWriteBufferSize int32  `yaml:"cltWriteBufferSize"` // 客户端连接写缓冲区大小。
	CltWriteChanSize   int32  `yaml:"cltWriteChanSize"`   // 客户端连接写队列长度，影响待发送消息排队能力。
	CltEnableDH        bool   `yaml:"cltEnableDH"`        // 是否启用客户端连接 Diffie-Hellman 密钥交换。
	CltReadTimeout     int32  `yaml:"cltReadTimeout"`     // 客户端连接读超时时间，单位按 netmgr 配置解析约定执行。
	Compress           bool   `yaml:"compress"`           // 是否启用消息压缩。
	CompressMode       int32  `yaml:"compressMode"`       // 消息压缩模式，具体含义由底层压缩实现定义。
	CompressLimit      int32  `yaml:"compressLimit"`      // 消息压缩阈值，超过该大小才触发压缩。
	DelayWrite         int32  `yaml:"delayWrite"`         // 延迟写入时间 / 开关配置，用于合并或延后发送，具体语义由 netmgr 实现定义。
}

type Auth struct {
	RatePerIp     int32  `yaml:"rate_per_ip"`     // 每5秒的同ip登录限制
	RatePerGate   int32  `yaml:"rate_per_gate"`   // 每个gate登录限制数量 用于计算最大限速数量
	RateMax       int32  `yaml:"rate_max"`        // 总登录限制数量
	GateQueueSize int32  `yaml:"gate_queue_size"` // auth开始排队，gate在线数量
	AppId         string `yaml:"appid"`           // 微信小程序 appid
	SC            string `yaml:"sc"`              // 微信小程序 secret
}
