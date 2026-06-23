package configdoc

import (
	"game/deps/xlog"
	"sync"
)

// Plugin 是所有插件需要实现的接口
type Plugin interface {
	Name() string                     // 插件名称
	Init(config map[string]any) error // 初始化插件
	Start() error                     // 启动插件
	Stop() error                      // 停止插件
}

// PluginManager 管理所有注册的插件
type PluginManager struct {
	plugins map[string]Plugin
	mu      sync.Mutex
}

func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins: make(map[string]Plugin),
	}
}

// Register 注册一个插件
func (pm *PluginManager) Register(p Plugin) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.plugins[p.Name()] = p
	xlog.Infof("Plugin registered : %v", p.Name())
}

// Init 初始化插件
func (pm *PluginManager) Init(configs map[string]map[string]any) {
	for name, cfg := range configs {
		if plugin, exists := pm.plugins[name]; exists {
			if err := plugin.Init(cfg); err != nil {
				xlog.Infof("Failed to initialize plugin %s: %v\n", name, err)
			}
		}
	}
}

// Start 启动所有插件
func (pm *PluginManager) Start() {
	for _, plugin := range pm.plugins {
		if err := plugin.Start(); err != nil {
			xlog.Infof("Failed to start plugin %s: %v\n", plugin.Name(), err)
		} else {
			xlog.Infof("Plugin started : %v", plugin.Name())
		}
	}
}

// Stop 停止所有插件
func (pm *PluginManager) Stop() {
	for _, plugin := range pm.plugins {
		if err := plugin.Stop(); err != nil {
			xlog.Infof("Failed to stop plugin %s: %v\n", plugin.Name(), err)
		}
	}
}
