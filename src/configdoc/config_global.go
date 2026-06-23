package configdoc

import (
	"game/src/common"
	"os"
	"strings"

	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

// 全局配置
type GlobalCfg struct {
	IsDebug         bool      `yaml:"isDebug"` //是否开启debug模式
	Mongo           *MongoCfg `yaml:"mongo"`
	RedisDsn        string    `yaml:"redisDsn"`
	EtcdDsn         string    `yaml:"etcdDsn"`
	GameDataPath    string    `yaml:"gameDataPath"`
	GatePublicHost  string    `yaml:"gatePublicHost"`
	Log             *LogCfg   `yaml:"log"`
	BI              *BICfg    `yaml:"bi"`
	GameArea        string    `yaml:"gameArea"`
	EnableLockCheck bool      `yaml:"enableLockCheck"`
}

type MongoCfg struct {
	Dsn    string `yaml:"dsn"`
	DbName string `yaml:"dbName"`
}

type KafkaCfg struct {
	Brokers          []string `yaml:"brokers"`
	Topics           []string `yaml:"topics"`
	FlushBytes       int      `yaml:"flushBytes"`
	FlushMessages    int      `yaml:"flushMessages"`
	FlushFrequency   int      `yaml:"flushFrequency"`
	FlushMaxMessages int      `yaml:"flushMaxMessages"`
}

type BICfg struct {
	Enabled         bool       `yaml:"enabled"`
	QueueSize       int        `yaml:"queueSize"`
	FlushCount      int        `yaml:"flushCount"`
	FlushIntervalMs int        `yaml:"flushIntervalMs"`
	Sink            string     `yaml:"sink"`
	Http            *BIHttpCfg `yaml:"http"`
	Mq              *BIMQCfg   `yaml:"mq"`
}

type BIHttpCfg struct {
	URL       string            `yaml:"url"`
	Path      string            `yaml:"path"`
	TimeoutMs int               `yaml:"timeoutMs"`
	Headers   map[string]string `yaml:"headers"`
}

type BIMQCfg struct {
	Brokers          []string `yaml:"brokers"`
	Topic            string   `yaml:"topic"`
	Acks             string   `yaml:"acks"`
	FlushBytes       int      `yaml:"flushBytes"`
	FlushMessages    int      `yaml:"flushMessages"`
	FlushFrequencyMs int      `yaml:"flushFrequencyMs"`
	FlushMaxMessages int      `yaml:"flushMaxMessages"`
}

const (
	defaultBIQueueSize       = 32 * 1024
	defaultBIFlushCount      = 500
	defaultBIFlushIntervalMs = 1000
	defaultBISink            = "log"
	defaultBIHttpTimeoutMs   = 3000
	defaultBIMQAcks          = "all"
	defaultBIMQTopic         = "bi"
	defaultBIMQFlushMessages = 200
	defaultBIMQFlushFreqMs   = 1000
)

func LoadGlobalCfg(path string) (*GlobalCfg, error) {
	v := viper.New()
	v.SetConfigName("global")
	v.SetConfigType("yaml")
	v.AddConfigPath(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	conf := NewGlobalCfg()

	if err := v.Unmarshal(conf); err != nil {
		return nil, err
	}
	conf.applyEnv()
	conf.applyDefaults()
	return conf, nil
}

func NewGlobalCfg() *GlobalCfg {
	return &GlobalCfg{
		GameArea:       common.GAME_AREA_PROD,
		BI:             defaultBICfg(),
		GatePublicHost: os.Getenv("GATE_PUBLIC_HOST"),
	}
}

func (c *GlobalCfg) applyEnv() {
	if c.Mongo == nil {
		c.Mongo = &MongoCfg{}
	}
	if dsn := strings.TrimSpace(os.Getenv("MONGO_DSN")); dsn != "" {
		c.Mongo.Dsn = dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("REDIS_DSN")); dsn != "" {
		c.RedisDsn = dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("ETCD_DSN")); dsn != "" {
		c.EtcdDsn = dsn
	}
	if dbName := mongoDBNameFromDSN(c.Mongo.Dsn); dbName != "" {
		c.Mongo.DbName = dbName
	}
}

func mongoDBNameFromDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	cfg, err := connstring.ParseAndValidate(dsn)
	if err != nil {
		return ""
	}
	return cfg.Database
}

func defaultBICfg() *BICfg {
	return &BICfg{
		Enabled:         true,
		QueueSize:       defaultBIQueueSize,
		FlushCount:      defaultBIFlushCount,
		FlushIntervalMs: defaultBIFlushIntervalMs,
		Sink:            defaultBISink,
		Http: &BIHttpCfg{
			TimeoutMs: defaultBIHttpTimeoutMs,
			Headers:   map[string]string{},
		},
		Mq: &BIMQCfg{
			Topic:            defaultBIMQTopic,
			Acks:             defaultBIMQAcks,
			FlushMessages:    defaultBIMQFlushMessages,
			FlushFrequencyMs: defaultBIMQFlushFreqMs,
		},
	}
}

func (c *GlobalCfg) applyDefaults() {
	if c.BI == nil {
		c.BI = defaultBICfg()
		return
	}
	if c.BI.QueueSize <= 0 {
		c.BI.QueueSize = defaultBIQueueSize
	}
	if c.BI.FlushCount <= 0 {
		c.BI.FlushCount = defaultBIFlushCount
	}
	if c.BI.FlushIntervalMs <= 0 {
		c.BI.FlushIntervalMs = defaultBIFlushIntervalMs
	}
	if c.BI.Sink == "" {
		c.BI.Sink = defaultBISink
	}
	if c.BI.Http == nil {
		c.BI.Http = &BIHttpCfg{TimeoutMs: defaultBIHttpTimeoutMs, Headers: map[string]string{}}
	} else if c.BI.Http.TimeoutMs <= 0 {
		c.BI.Http.TimeoutMs = defaultBIHttpTimeoutMs
	}
	if c.BI.Http.Headers == nil {
		c.BI.Http.Headers = map[string]string{}
	}
	if c.BI.Mq == nil {
		c.BI.Mq = &BIMQCfg{}
	}
	if c.BI.Mq.Topic == "" {
		c.BI.Mq.Topic = defaultBIMQTopic
	}
	if c.BI.Mq.Acks == "" {
		c.BI.Mq.Acks = defaultBIMQAcks
	}
	if c.BI.Mq.FlushMessages <= 0 {
		c.BI.Mq.FlushMessages = defaultBIMQFlushMessages
	}
	if c.BI.Mq.FlushFrequencyMs <= 0 {
		c.BI.Mq.FlushFrequencyMs = defaultBIMQFlushFreqMs
	}
}
