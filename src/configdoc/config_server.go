package configdoc

import (
	"encoding/json"
	"fmt"
	"game/src/common"
	"hash/crc32"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/samber/lo"
	"github.com/spf13/viper"
)

type ServerCfg struct {
	Id             int32     `yaml:"id"`
	Type           string    `yaml:"type"`
	Ip             string    `yaml:"ip"`
	Port           int32     `yaml:"port"`
	Cluster        string    `yaml:"cluster"`
	AsyncPoolSize  int32     `yaml:"asyncPoolSize"`
	ASyncQueueSize int32     `yaml:"aSyncQueueSize"`
	GopsAddr       string    `yaml:"gopsAddr"`
	BackendAddr    string    `yaml:"backendAddr"`
	Net            *Net      `yaml:"net"`
	Robot          *Robot    `yaml:"robot"`
	CombatPath     string    `yaml:"combatPath"`
	CombatLogPath  string    `yaml:"combatLogPath"`
	Auth           *Auth     `yaml:"auth"`
	Log            *LogCfg   `yaml:"log"`
	SPR            *RobotSPR `yaml:"spr"` // 兼容旧/误配的根节点 spr；优先使用 robot.spr。
	GUI            *bool     `yaml:"gui"` // 兼容根节点 gui；优先使用 robot.gui。
	NotEtcd        bool      `yaml:"notEtcd"`
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

func LoadServerCfg(path string) (*ServerCfg, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	conf := &ServerCfg{}
	if err := v.Unmarshal(conf, func(config *mapstructure.DecoderConfig) {
		config.TagName = "yaml"
	}); err != nil {
		return nil, err
	}

	if conf.Auth == nil {
		conf.Auth = &Auth{
			RatePerIp:     10,
			RatePerGate:   200,
			RateMax:       500,
			GateQueueSize: 5000,
		}
	} else {
		conf.Auth.RatePerIp = lo.Ternary(conf.Auth.RatePerIp == 0, 10, conf.Auth.RatePerIp)
		conf.Auth.RatePerGate = lo.Ternary(conf.Auth.RatePerGate == 0, 200, conf.Auth.RatePerGate)
		conf.Auth.RateMax = lo.Ternary(conf.Auth.RateMax == 0, 500, conf.Auth.RateMax)
		conf.Auth.GateQueueSize = lo.Ternary(conf.Auth.GateQueueSize == 0, 5000, conf.Auth.GateQueueSize)
	}

	if err := conf.tidyServerConfigWithEnv(); err != nil {
		return nil, err
	}
	conf.normalizeRobotConfig()
	if err := conf.loadRobotBattleSmokeConfig(filepath.Dir(path)); err != nil {
		return nil, err
	}

	return conf, nil
}

func (c *ServerCfg) normalizeRobotConfig() {
	if c == nil || c.Robot == nil {
		return
	}
	robot := c.Robot
	if robot.GUI == nil && c.GUI != nil {
		robot.GUI = c.GUI
	}
	if robot.EnableSmoke || robot.EnableStress {
		return
	}
	if len(robot.SmokeModules) == 0 {
		robot.EnableSmoke = true
		return
	}
	hasSmokeModule := false
	hasStressModule := false
	for _, module := range robot.SmokeModules {
		switch strings.ToLower(strings.TrimSpace(module)) {
		case "":
		case "stress":
			hasStressModule = true
		default:
			hasSmokeModule = true
		}
	}
	if hasStressModule && !hasSmokeModule {
		robot.EnableStress = true
		return
	}
	robot.EnableSmoke = true
}

func (c *ServerCfg) RobotGUIEnabled() bool {
	if c == nil || c.Robot == nil || c.Robot.GUI == nil {
		return true
	}
	return *c.Robot.GUI
}

func (c *ServerCfg) loadRobotBattleSmokeConfig(confDir string) error {
	if c == nil || c.Robot == nil || c.Robot.BattleSmoke == nil {
		return nil
	}
	cfg := c.Robot.BattleSmoke
	configPath := strings.TrimSpace(cfg.ConfigPath)
	if configPath == "" {
		return nil
	}
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(confDir, configPath)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read battle smoke config failed, path=%s err=%w", configPath, err)
	}
	fileCfg := &RobotBattleSmokeCfg{}
	if err := json.Unmarshal(data, fileCfg); err != nil {
		return fmt.Errorf("unmarshal battle smoke config failed, path=%s err=%w", configPath, err)
	}
	if fileCfg.IntervalSeconds > 0 {
		cfg.IntervalSeconds = fileCfg.IntervalSeconds
	}
	if len(fileCfg.Battles) > 0 {
		cfg.Battles = fileCfg.Battles
	}
	if fileCfg.Start != nil {
		cfg.Start = fileCfg.Start
	}
	if fileCfg.End != nil {
		cfg.End = fileCfg.End
	}
	cfg.ConfigPath = configPath
	return nil
}

func (c *ServerCfg) tidyServerConfigWithEnv() error {
	serverType := common.GetServerType(c.Type)
	if serverType == 0 && c.Type != "robot" {
		return fmt.Errorf("server type is not valid %s", c.Type)
	}

	// 判断是否为K8S环境 - 检查是否有K8S特定环境变量
	ip := os.Getenv("SERVICE_IP")
	port := os.Getenv("SERVICE_PORT")
	podName := os.Getenv("POD_NAME")
	gopsAddr := os.Getenv("GOPS_ADDR")
	backendAddr := os.Getenv("BACKEND_ADDR")
	// 如果没有设置K8S环境变量，则认为是非K8S环境，直接返回使用配置文件中的配置
	if ip == "" && port == "" && podName == "" {
		return nil
	}

	// K8S环境下验证必要环境变量
	if ip == "" || port == "" || podName == "" {
		return fmt.Errorf("env is not all set, SERVICE_IP: %s, SERVICE_PORT: %s, POD_NAME: %s", ip, port, podName)
	}

	iport, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("env SERVICE_PORT is not valid, %s", port)
	}
	index := -1
	//Deployment server
	if strings.Contains(podName, "auth-") || strings.Contains(podName, "query-") {
		str := podName + strconv.FormatInt(time.Now().UnixNano(), 10)
		index = int(crc32.ChecksumIEEE([]byte(str))) % common.ServerOffset
	} else { // StatefulSet server
		ss := strings.Split(podName, "-")
		podIndex := lo.Ternary(len(ss) > 1, ss[len(ss)-1], "")
		index, err = strconv.Atoi(podIndex)
		if err != nil {
			return fmt.Errorf("env POD_NAME is not valid, %s", podName)
		}
	}

	if index < 0 || index >= common.ServerOffset {
		return fmt.Errorf("env POD_NAME is not valid, %s", podName)
	}

	serverId := serverType*common.ServerOffset + int32(index)

	if serverId >= 8192 {
		return fmt.Errorf("server id is too large, %d", serverId)
	}

	c.Ip = ip
	c.Port = int32(iport)
	c.Id = serverId

	if c.GopsAddr == "" {
		c.GopsAddr = lo.Ternary(gopsAddr == "", ":6060", gopsAddr)
	}
	if c.BackendAddr == "" {
		c.BackendAddr = lo.Ternary(backendAddr == "", ":6080", backendAddr)
	}
	return nil
}
