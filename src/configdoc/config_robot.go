package configdoc

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

type robotYamlOnlyCfg struct {
	GameDataPath string `yaml:"gameDataPath"`
}

type Robot struct {
	Name             string               `yaml:"name"`             // 机器人账号名前缀；为空时运行时自动生成。
	Count            int32                `yaml:"count"`            // 启动机器人总数。
	Auth             string               `yaml:"auth"`             // 认证服 HTTP 地址。
	LoginRate        int32                `yaml:"loginRate"`        // 每秒启动/登录的机器人数量。
	ActionIntervalMs int32                `yaml:"actionIntervalMs"` // 机器人动作驱动间隔，单位毫秒。
	RecentReqLimit   int32                `yaml:"recentReqLimit"`   // 最近请求耗时明细保留条数。
	EnableSmoke      bool                 `yaml:"enableSmoke"`      // 是否开启冒烟模式。
	EnableStress     bool                 `yaml:"enableStress"`     // 是否开启压测模式。
	StressWeights    map[string]int32     `yaml:"stressWeights"`    // 压测模式模块权重；key 为 stress 模块名，value 为权重。
	StressCooldownMs map[string]int32     `yaml:"stressCooldownMs"` // 压测模式模块冷却；单位毫秒，冷却中不再触发该模块请求。
	SmokeModules     []string             `yaml:"smokeModules"`     // 冒烟模式下启用的模块列表。
	BattleSmoke      *RobotBattleSmokeCfg `yaml:"battleSmoke"`      // 战斗冒烟专用配置。
	SPR              *RobotSPR            `yaml:"spr"`              // SPRAdapter 上报配置。
	GUI              *bool                `yaml:"gui"`              // 是否启动 robot HTTP 图形面板；不配置默认开启。
}

type RobotSPR struct {
	Use              bool   `yaml:"use"`              // 是否上报到本机 SPRAdapter。
	AdapterURL       string `yaml:"adapterUrl"`       // SPRAdapter 地址。
	UploadIntervalMs int32  `yaml:"uploadIntervalMs"` // 上报间隔，单位毫秒。
}

type RobotBattleSmokeCfg struct {
	IntervalSeconds int32                 `yaml:"intervalSeconds"`
	ConfigPath      string                `yaml:"configPath"`
	Battles         []*RobotBattleCaseCfg `yaml:"battles"`
	Start           *RobotBattleStartCfg  `yaml:"start"`
	End             *RobotBattleEndCfg    `yaml:"end"`
}

type RobotBattleCaseCfg struct {
	Name  string               `yaml:"name"`
	Start *RobotBattleStartCfg `yaml:"start"`
	End   *RobotBattleEndCfg   `yaml:"end"`
}

type RobotBattleStartCfg struct {
	StageID   int32                 `yaml:"stageId"`
	FightType int32                 `yaml:"fightType"`
	Heroes    []*RobotBattleHeroCfg `yaml:"heroes"`
}

type RobotBattleHeroCfg struct {
	ConfID int32 `yaml:"confId"`
	X      int32 `yaml:"x"`
	Z      int32 `yaml:"z"`
}

type RobotBattleEndCfg struct {
	CombatCode     string                    `yaml:"combatCode"`
	Win            *bool                     `yaml:"win"`
	ProVersion     *int32                    `yaml:"proVersion"`
	ConfVersion    *int32                    `yaml:"confVersion"`
	StatisticsData *RobotBattleStatisticsCfg `yaml:"statisticsData"`
}

type RobotBattleStatisticsCfg struct {
	Units      []*RobotBattleUnitStatusCfg `yaml:"units"`
	EnemyUnits []*RobotBattleUnitStatusCfg `yaml:"enemyUnits"`
}

type RobotBattleUnitStatusCfg struct {
	CharacterID int32 `yaml:"characterID"`
	CurHp       int64 `yaml:"curHp"`
	MaxHp       int64 `yaml:"maxHp"`
	CurMp       int64 `yaml:"curMp"`
	MaxMp       int64 `yaml:"maxMp"`
	Damage      int64 `yaml:"damage"`
	Heal        int64 `yaml:"heal"`
	TakeDamage  int64 `yaml:"takeDamage"`
	Status      int32 `yaml:"status"`
	SkinID      int32 `yaml:"skinId"`
}


func LoadRobotYamlConfig(confPath string) (*ConfigBase, error) {
	svrCfg, err := LoadServerCfg(confPath)
	if err != nil {
		return nil, err
	}

	robotOnlyCfg, err := loadRobotYamlOnlyCfg(confPath)
	if err != nil {
		return nil, err
	}

	globalCfg := NewGlobalCfg()
	globalCfg.GameDataPath = resolveRobotGameDataPath(confPath, robotOnlyCfg.GameDataPath)
	if globalCfg.GameDataPath == "" {
		return nil, fmt.Errorf("gameDataPath is empty")
	}

	cfg := &ConfigBase{
		Server: svrCfg,
		Global: globalCfg,
	}
	cfg.applyLogCfg()
	return cfg, nil
}

func loadRobotYamlOnlyCfg(confPath string) (*robotYamlOnlyCfg, error) {
	v := viper.New()
	v.SetConfigFile(confPath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	cfg := &robotYamlOnlyCfg{}
	if err := v.Unmarshal(cfg, func(config *mapstructure.DecoderConfig) {
		config.TagName = "yaml"
	}); err != nil {
		return nil, err
	}
	return cfg, nil
}

func resolveRobotGameDataPath(confPath string, gameDataPath string) string {
	gameDataPath = strings.TrimSpace(gameDataPath)
	if gameDataPath == "" {
		return ""
	}
	if filepath.IsAbs(gameDataPath) {
		return filepath.Clean(gameDataPath)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(confPath), gameDataPath))
}
