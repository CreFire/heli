package configdoc

import (
	"encoding/json"
	"fmt"
	"game/deps/misc"
	"game/deps/xlog"
	"path/filepath"
	"strconv"
)

// ConfigBase aggregates YAML configuration along with loaded doc tables.
type ConfigBase struct {
	Server              *ServerCfg       // 服务器配置
	Global              *GlobalCfg       // 全局配置
	Doc                 *DocPbConfig     // 配置表
	DocExtend           *DocExtendConfig // 额外配置
	reloadFinishedHooks []func(old *ConfigBase, new *ConfigBase)
}

func (m *ConfigBase) AfterReload(f func(old *ConfigBase, new *ConfigBase)) {
	m.reloadFinishedHooks = append(m.reloadFinishedHooks, f)
}

// ReadConfig loads YAML configuration and doc tables with hook points for consumers.
func (m *ConfigBase) LoadYamlConfig(confPath string) error {
	svrCfg, err := LoadServerCfg(confPath)
	if err != nil {
		return err
	}
	m.Server = svrCfg

	dir := filepath.Dir(confPath)
	globalCfg, err := LoadGlobalCfg(dir)
	if err != nil {
		return err
	}
	m.Global = globalCfg

	m.applyLogCfg()

	if m.Global == nil || m.Global.GameDataPath == "" {
		return fmt.Errorf("gameDataPath is empty")
	}
	return nil
}

func (m *ConfigBase) LoadTableConfig() error {
	doc, docExt, err := loadTables(m.Global.GameDataPath)
	if err != nil {
		return err
	}
	m.Doc = doc
	m.DocExtend = docExt
	checkExcelVersion(m.Doc)
	misc.ExcelVer = strconv.FormatInt(int64(m.Doc.ExcelVersion), 10)
	return nil
}

// Reload reloads YAML and doc configuration asynchronously.
func (m *ConfigBase) Reload(confPath string) (newCfg *ConfigBase, err error) {
	xlog.Infof("start to reload config...")

	newSvrCfg, err := LoadServerCfg(confPath)
	if err != nil {
		return nil, fmt.Errorf("load server cfg, path %s, failed: err %w", confPath, err)
	}
	js, _ := json.Marshal(newSvrCfg)
	xlog.Infof("reload server cfg success. confPath:%s value: %s", confPath, string(js))

	dir := filepath.Dir(confPath)
	newGlobalCfg, err := LoadGlobalCfg(dir)
	if err != nil {
		return nil, fmt.Errorf("load global cfg , path:%s failed: err %w", dir, err)
	}
	js, _ = json.Marshal(newGlobalCfg)
	xlog.Infof("reload global cfg success. confPath:%s value:%s ", confPath, string(js))
	cfg := &ConfigBase{
		Server: newSvrCfg,
		Global: newGlobalCfg,
	}
	cfg.applyLogCfg()

	if m.Global == nil || m.Global.GameDataPath == "" {
		return nil, fmt.Errorf("reload doc cfg failed. confPath: %s gameDataPath is empty", confPath)
	}
	doc, docExt, err := loadTables(m.Global.GameDataPath)
	if err != nil {
		return nil, err
	}

	cfg.Doc = doc
	cfg.DocExtend = docExt
	cfg.Doc.Changed = calcChangedTables(m.Doc, cfg.Doc)
	checkExcelVersion(cfg.Doc)

	for _, hook := range m.reloadFinishedHooks {
		hook(m, cfg)
	}
	xlog.Infof("confPath:%v reload finished.", confPath)

	return cfg, nil

}

////////////////////////////////////////////

func (m *ConfigBase) GetSvrCfg() *ServerCfg {
	return m.Server
}

func (m *ConfigBase) GetGlobalCfg() *GlobalCfg {
	return m.Global
}

func (m *ConfigBase) GetDoc() *DocPbConfig {
	return m.Doc
}

func (m *ConfigBase) GetDocExtend() *DocExtendConfig {
	return m.DocExtend
}

func (m *ConfigBase) applyLogCfg() {
	base := MergeLogCfg(DefaultLogCfg(), m.Global.Log)
	m.Server.Log = MergeLogCfg(base, m.Server.Log)
}
