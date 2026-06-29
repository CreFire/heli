package configdoc

import (
	"encoding/json"
	"errors"
	"fmt"
	"game/deps/xlog"
	cfg "game/src/proto/docpb"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var errExtend = errors.New("newConfigExtend failed")
var errCheck = errors.New("check config failed")

// 加载配置文档
func loadTables(confDir string) (*DocPbConfig, *DocExtendConfig, error) {
	xlog.Infof("docpb config path: %v", confDir)
	tables, err := cfg.NewTables(func(fileName string) ([]map[string]any, error) {
		//假设配置文件存储在 "config/" 目录下
		filePath := filepath.Join(confDir, fileName+".json")
		pwd, _ := os.Getwd()
		xlog.Infof("docpb config pwd:%v path: %v", pwd, filePath)
		// 读取文件内容
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %v", filePath, err)
		}

		// 解析 JSON 数据
		var result []map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON for %s: %v", fileName, err)
		}
		return result, nil
	})
	if err != nil {
		xlog.Infof("loadTables: err:%v", err)
		return nil, nil, err
	}

	pbConf := &DocPbConfig{
		Tables: tables,
	}
	if battle, err := loadBattleTables(confDir); err != nil {
		xlog.Warnf("load battle tables failed: %v", err)
	} else {
		pbConf.Battle = battle
	}
	if ver, err := loadDocVersion(confDir); err != nil {
		xlog.Warnf("load doc version failed: %v", err)
	} else {
		pbConf.ExcelVersion = ver
	}
	// 加载扩展配置
	docExtend := newConfigExtend(pbConf)
	if docExtend == nil {
		return nil, nil, errExtend
	}
	//检测配置合法性
	state := true
	for _, checkFunc := range checkers.handlers {
		if !checkFunc(pbConf, docExtend) {
			state = false
		}
	}
	if !state {
		return nil, nil, errCheck
	}
	return pbConf, docExtend, nil
}

type docVersion struct {
	Version  string `json:"version"`
	DateTime string `json:"dateTime"`
}

func loadDocVersion(confDir string) (int32, error) {
	path := filepath.Join(confDir, "version.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var info docVersion
	if err := json.Unmarshal(data, &info); err != nil {
		return 0, err
	}
	if strings.TrimSpace(info.Version) == "" {
		return 0, fmt.Errorf("version is empty")
	}
	val, err := strconv.ParseInt(strings.TrimSpace(info.Version), 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(val), nil
}
