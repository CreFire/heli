package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"game/deps/fastid"
	"game/deps/netmgr"
	"game/deps/server"
	"game/deps/xlog"
	"game/src/configdoc"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var currentRobotRuntime *RobotRuntime

type RobotRuntime struct {
	cfg            *configdoc.ConfigBase
	netMgr         *netmgr.NetMgr
	customShutdown func(int)
	done           chan struct{}

	stopOnce sync.Once
	stopping atomic.Bool
	stopErr  error
}

func NewRobotRuntime() *RobotRuntime {
	return &RobotRuntime{
		done: make(chan struct{}),
	}
}

func (rt *RobotRuntime) Init() error {
	confPath, err := robotConfPathFromArgs()
	if err != nil {
		return err
	}

	cfg, err := configdoc.LoadRobotYamlConfig(confPath)
	if err != nil {
		return err
	}

	logOpts := configdoc.LogCfgToOptions(cfg.GetSvrCfg().Log)
	xlog.InitDefaultLoggerWithOptions(logOpts)
	xlog.Infof("")
	xlog.Infof("----------robot start %s----------", time.Now().Format("2006-01-02 15:04:05.000"))
	js, _ := json.Marshal(cfg.GetSvrCfg())
	xlog.Infof("robot conf: %s", string(js))
	xlog.Infof("robot init config loaded path=%s", confPath)

	if err := cfg.LoadTableConfig(); err != nil {
		xlog.Errorf("robot init load tables failed: %v", err)
		return err
	}
	xlog.Infof("robot init table config loaded")

	fastid.InitWithMachineID(cfg.GetSvrCfg().Id)

	rt.cfg = cfg
	rt.netMgr = netmgr.NewNetMgr()
	currentRobotRuntime = rt
	xlog.Infof("robot init runtime ready")
	if err := RobotSvr.OnInit(); err != nil {
		xlog.Errorf("robot init server failed: %v", err)
		return err
	}
	xlog.Infof("robot init server ready")
	return nil
}

func (rt *RobotRuntime) Start() error {
	xlog.Infof("robot runtime start net manager")
	rt.netMgr.Start()
	xlog.Infof("robot runtime net manager started")
	if err := RobotSvr.OnStart(); err != nil {
		xlog.Errorf("robot runtime start robot server failed: %v", err)
		return err
	}
	xlog.Infof("robot runtime start complete")
	return nil
}

func (rt *RobotRuntime) WaitStop() error {
	xlog.Infof("robot runtime waiting for stop signal")
	notify := make(chan os.Signal, 1)
	signal.Notify(notify, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(notify)
	sig := <-notify
	xlog.Warnf("robot runtime received stop signal: %v", sig)
	return rt.Stop()
}

func (rt *RobotRuntime) Stop() error {
	rt.stopOnce.Do(func() {
		rt.stopping.Store(true)
		if rt.done != nil {
			close(rt.done)
		}
		var errs []error

		if rt.netMgr != nil {
			rt.netMgr.StopFront()
		}

		if rt.netMgr != nil {
			rt.netMgr.Stop()
		}
		if len(errs) > 0 {
			rt.stopErr = errorsJoin(errs...)
			xlog.Warnf("robot stop finished with errors: %v", rt.stopErr)
		} else {
			xlog.Infof("robot stop success")
		}
		xlog.Close()
		currentRobotRuntime = nil
	})
	return rt.stopErr
}

func (rt *RobotRuntime) Shutdown(exitCode int) {
	go func() {
		if err := rt.Stop(); err != nil && exitCode == 0 {
			exitCode = 1
		}
		if isGoTestProcess() {
			return
		}
		os.Exit(exitCode)
	}()
}

func (rt *RobotRuntime) requestShutdown(exitCode int) {
	if rt == nil || rt.stopping.Load() {
		return
	}
	if rt.customShutdown != nil {
		rt.customShutdown(exitCode)
		return
	}
	rt.Shutdown(exitCode)
}

func robotConfPathFromArgs() (string, error) {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "-f" || arg == "--f" {
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", fmt.Errorf("config file path is empty")
			}
			return strings.TrimSpace(args[i+1]), nil
		}
		if strings.HasPrefix(arg, "-f=") {
			val := strings.TrimSpace(strings.TrimPrefix(arg, "-f="))
			if val == "" {
				return "", fmt.Errorf("config file path is empty")
			}
			return val, nil
		}
		if strings.HasPrefix(arg, "--f=") {
			val := strings.TrimSpace(strings.TrimPrefix(arg, "--f="))
			if val == "" {
				return "", fmt.Errorf("config file path is empty")
			}
			return val, nil
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exePath), "robot.yaml"), nil
}

func robotConfig() *configdoc.ConfigBase {
	if currentRobotRuntime != nil && currentRobotRuntime.cfg != nil {
		return currentRobotRuntime.cfg
	}
	return server.MS.ConfBase
}

func robotServerCfg() *configdoc.ServerCfg {
	cfg := robotConfig()
	if cfg == nil {
		return nil
	}
	return cfg.Server
}

func robotGlobalCfg() *configdoc.GlobalCfg {
	cfg := robotConfig()
	if cfg == nil {
		return nil
	}
	return cfg.Global
}

func robotDoc() *configdoc.DocPbConfig {
	cfg := robotConfig()
	if cfg == nil {
		return nil
	}
	return cfg.Doc
}

func robotDocExtend() *configdoc.DocExtendConfig {
	cfg := robotConfig()
	if cfg == nil {
		return nil
	}
	return cfg.DocExtend
}

func robotNetMgr() *netmgr.NetMgr {
	if currentRobotRuntime == nil {
		return nil
	}
	return currentRobotRuntime.netMgr
}

func robotRuntimeStopping() bool {
	return currentRobotRuntime != nil && currentRobotRuntime.stopping.Load()
}

func robotRuntimeDone() <-chan struct{} {
	if currentRobotRuntime == nil {
		return nil
	}
	return currentRobotRuntime.done
}

func errorsJoin(errs ...error) error {
	filtered := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err.Error())
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return errors.New(strings.Join(filtered, "; "))
}
