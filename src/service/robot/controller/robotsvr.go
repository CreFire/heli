package controller

import (
	"fmt"
	"game/deps/basal"
	"game/deps/xlog"
	"game/src/configdoc"
	"game/src/proto/eventpb"
	"net/http"
	"strings"
	"sync"
	"time"
)

var RobotSvr = NewRobotSvr()

func NewRobotSvr() *RobotServer {
	return &RobotServer{
		robotMgr: NewRobotMgr(),
	}
}

type RobotServer struct {
	client             *http.Client
	stat               *Statistics
	robotMgr           *RobotMgr
	webSvr             *WebSvr
	smokeMu            sync.RWMutex
	smokeHistory       map[string]smokeRobotResult
	smokeKnownFailures map[string]map[string]struct{}
}

func (r *RobotServer) OnInit() error {
	xlog.Infof("robot server init start")
	r.client = &http.Client{Transport: &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}, Timeout: 10 * time.Second}
	r.RegisterWebHandler()
	xlog.Infof("robot server init complete")
	return nil
}

func (r *RobotServer) BeforeStart() error {
	return nil
}

func (r *RobotServer) OnStart() error {
	svrCfg := robotServerCfg()
	xlog.Infof("robot server start web addr=:%d robotCount=%d loginRate=%d", svrCfg.Port, svrCfg.Robot.Count, svrCfg.Robot.LoginRate)
	if svrCfg.RobotGUIEnabled() {
		if err := r.webSvr.Start(fmt.Sprintf(":%d", svrCfg.Port)); err != nil {
			return fmt.Errorf("start web server failed: %w", err)
		}
	} else {
		xlog.Infof("robot gui disabled by config")
	}

	robotCount := robotSpawnCount(svrCfg)
	r.stat = NewStatistics(robotCount, svrCfg.Robot.LoginRate, 5000)

	r.stat.Start()
	xlog.Infof("robot server statistics started")

	sleep := time.Second / time.Duration(svrCfg.Robot.LoginRate)
	runtimeDone := robotRuntimeDone()
	xlog.Infof("robot server spawning robots total=%d interval=%s", robotCount, sleep)
	for i := int32(0); i < robotCount; i++ {
		select {
		case <-runtimeDone:
			xlog.Warnf("robot server stop spawning robots because runtime is stopping")
			return nil
		default:
		}
		robot := newRobotForSpawn(i)
		robot.Start()
		if (i+1)%100 == 0 || i == robotCount-1 {
			xlog.Infof("robot server spawned=%d/%d", i+1, robotCount)
		}
		if sleep > 0 {
			select {
			case <-runtimeDone:
				xlog.Warnf("robot server stop sleep because runtime is stopping")
				return nil
			case <-time.After(sleep):
			}
		}
	}
	xlog.Infof("robot server start complete")

	return nil
}

func (r *RobotServer) BeforeStop() error {
	if r.robotMgr != nil {
		// 先快照后停止，避免在 Map.Range(RLock) 回调里调用 robot.Stop()
		// 触发同一张表 Delete(Lock) 的锁升级死锁。
		robots := make([]*Robot, 0, r.robotMgr.GamerCount())
		r.robotMgr.foreach(func(_ int64, robot *Robot) bool {
			if robot != nil {
				robots = append(robots, robot)
			}
			return true
		})
		for _, robot := range robots {
			robot.Stop()
		}
	}
	return nil

}

func (r *RobotServer) OnStop() error {

	if r.stat != nil {
		r.stat.Stop()
	}
	if r.webSvr != nil {
		r.webSvr.Stop()
	}
	return nil

}

func (r *RobotServer) OnReload(oldCfg, newCfg *configdoc.ConfigBase) error {
	return nil

}

func (r *RobotServer) OnHeart(now int64) error {
	return nil
}

func (r *RobotServer) OnEventHandle(evt *eventpb.Event) {
}

func robotSpawnCount(cfg *configdoc.ServerCfg) int32 {
	if cfg == nil || cfg.Robot == nil {
		return 0
	}
	if cfg.Robot.EnableSmoke && !cfg.Robot.EnableStress {
		modules := collectRobotModules(cfg.Robot.SmokeModules, false)
		if len(modules) == 0 {
			modules = defaultSmokeModules()
		}
		return int32(len(modules))
	}
	return cfg.Robot.Count
}

func newRobotForSpawn(index int32) *Robot {
	cfg := robotServerCfg()
	if cfg != nil && cfg.Robot != nil && cfg.Robot.EnableSmoke && !cfg.Robot.EnableStress {
		modules := collectRobotModules(cfg.Robot.SmokeModules, false)
		if len(modules) == 0 {
			modules = defaultSmokeModules()
		}
		if int(index) < len(modules) {
			return NewSmokeRobot(modules[index])
		}
	}
	return NewRobot()
}

func NewSmokeRobot(module string) *Robot {
	robot := NewRobot()
	module = strings.ToUpper(strings.TrimSpace(module))
	if module != "" {
		robot.smokeModules = []string{module}
		if strings.TrimSpace(robotServerCfg().Robot.Name) != "" {
			robot.name = fmt.Sprintf("%s_%s", strings.TrimRight(robot.name, "_"), strings.ToLower(module))
		}
	}
	return robot
}

func (r *Robot) smokeResultSnapshot() map[string]SmokeResult {
	if r == nil {
		return nil
	}
	r.smokeMu.RLock()
	defer r.smokeMu.RUnlock()
	if len(r.SmokeResults) == 0 {
		return nil
	}
	results := make(map[string]SmokeResult, len(r.SmokeResults))
	for key, result := range r.SmokeResults {
		if result == nil {
			continue
		}
		results[key] = *result
	}
	return results
}
func (r *RobotServer) rememberSmokeResults(robot *Robot) {
	if r == nil || robot == nil {
		return
	}
	results := robot.smokeResultSnapshot()
	if len(results) == 0 {
		return
	}
	r.smokeMu.Lock()
	defer r.smokeMu.Unlock()
	if r.smokeHistory == nil {
		r.smokeHistory = make(map[string]smokeRobotResult)
	}
	for key, result := range results {
		module := strings.ToUpper(strings.TrimSpace(result.Module))
		step := result.Step
		if module == "" || step == "" {
			parts := strings.SplitN(key, ":", 2)
			if module == "" && len(parts) > 0 {
				module = strings.ToUpper(strings.TrimSpace(parts[0]))
			}
			if step == "" && len(parts) > 1 {
				step = parts[1]
			}
		}
		historyKey := robot.name + ":" + module + ":" + step
		r.smokeHistory[historyKey] = smokeRobotResult{
			Name:     robot.name,
			GID:      robot.gid,
			Module:   module,
			Step:     step,
			Received: result.Received,
			Success:  result.Success,
			Summary:  result.Summary,
			Updated:  result.Updated,
		}
	}
}

func (r *RobotServer) smokeHistorySnapshot() []smokeRobotResult {
	if r == nil {
		return nil
	}
	r.smokeMu.RLock()
	defer r.smokeMu.RUnlock()
	if len(r.smokeHistory) == 0 {
		return nil
	}
	results := make([]smokeRobotResult, 0, len(r.smokeHistory))
	for _, result := range r.smokeHistory {
		results = append(results, result)
	}
	return results
}

func (r *RobotServer) shouldStopSmokeOnFailure(module, step, summary string) (bool, string) {
	if r == nil {
		return false, ""
	}
	module = strings.ToUpper(strings.TrimSpace(module))
	signature := smokeFailureSignature(step, summary)
	if module == "" || signature == "" {
		return true, signature
	}
	r.smokeMu.Lock()
	defer r.smokeMu.Unlock()
	if r.smokeKnownFailures == nil {
		r.smokeKnownFailures = make(map[string]map[string]struct{})
	}
	failures := r.smokeKnownFailures[module]
	if failures == nil {
		failures = make(map[string]struct{})
		r.smokeKnownFailures[module] = failures
	}
	if _, ok := failures[signature]; ok {
		return false, signature
	}
	failures[signature] = struct{}{}
	return true, signature
}

func (r *RobotServer) scheduleSmokeReplacement(module string) {
	if r == nil || robotRuntimeStopping() || !robotSmokeOnlyMode() {
		return
	}
	module = strings.ToLower(strings.TrimSpace(module))
	if module == "" {
		return
	}
	loginRate := int32(1)
	if cfg := robotServerCfg(); cfg != nil && cfg.Robot != nil && cfg.Robot.LoginRate > 0 {
		loginRate = cfg.Robot.LoginRate
	}
	delay := time.Second / time.Duration(loginRate)
	basal.SafeGo(func() {
		if delay > 0 {
			select {
			case <-robotRuntimeDone():
				return
			case <-time.After(delay):
			}
		}
		if robotRuntimeStopping() {
			return
		}
		robot := NewSmokeRobot(module)
		robot.Start()
		xlog.Infof("robot smoke replacement spawned module=%s name=%s", strings.ToUpper(module), robot.name)
	})
}

func smokeFailureSignature(step, summary string) string {
	step = strings.ToLower(strings.TrimSpace(step))
	summary = strings.TrimSpace(summary)
	lower := strings.ToLower(summary)
	if idx := strings.Index(lower, "err="); idx >= 0 {
		errPart := summary[idx+len("err="):]
		if fields := strings.Fields(errPart); len(fields) > 0 {
			summary = "err=" + fields[0]
		} else {
			summary = "err=" + strings.TrimSpace(errPart)
		}
	}
	if step == "" {
		return summary
	}
	if summary == "" {
		return step
	}
	return step + "|" + summary
}
