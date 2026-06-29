package controller

import (
	"fmt"
	"game/deps/basal"
	"game/deps/fastid"
	"game/deps/kit"
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/xlog"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

type Robot struct {
	name    string
	gid     int64
	Session string

	curState   IState
	states     map[string]IState
	taskChan   chan func()
	msgque     netmgr.IMsgQue
	msgHandler *MsgHandler
	quit       chan struct{}
	stopOnce   sync.Once
	seq        int32

	smokeMu            sync.RWMutex
	SmokeResults       map[string]*SmokeResult
	SmokeModuleStarted map[string]time.Time

	picker             *WeightPicker
	smokeFailureStop   bool
	smokeFailureModule string
	shutdownRequested  bool
	cleanupRequested   bool

	smokeModules []string

	packLoaded bool
	taskLoaded bool
}

func (r *Robot) RegisterStates() {
	r.RegisterState(&StateAuth{}, 1)
	r.RegisterState(&StateLogin{}, 1)
}
func isGoTestProcess() bool {
	name := strings.ToLower(strings.TrimSpace(os.Args[0]))
	return strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".test.exe")
}

func (r *Robot) RegisterState(state IState, weight int32) {
	r.states[state.Name()] = state
	r.picker.Update(state.Name(), weight)
}

func (r *Robot) SetState(name string) {
	state, ok := r.states[name]
	if !ok {
		return
	}
	if r.curState != nil {
		r.curState.onLeave(r)
	}

	r.curState = state
	err := r.curState.OnEnter(r)
	if err != nil {
		WarnfLimited("set_state_failed:"+name, "robot %v set state %v failed : %v", r.name, name, err)
		r.Stop()
	}
}

func (r *Robot) Stop() {
	r.stopOnce.Do(func() {
		smokeFailureStop := r.smokeFailureStop
		smokeFailureModule := r.smokeFailureModule
		if RobotSvr == nil || RobotSvr.robotMgr == nil {
			WarnfLimited("robot_stop_failed", "robot %v stop failed", r.name)
		} else {
			RobotSvr.rememberSmokeResults(r)
			if r.msgque != nil {
				RobotSvr.robotMgr.delGamer(r.msgque.SessId(), r.gid, "stop")
			}
			if smokeFailureStop {
				RobotSvr.scheduleSmokeReplacement(smokeFailureModule)
			}
			if RobotSvr.robotMgr.GamerCount() == 0 && !robotSmokeOnlyMode() {
				r.requestProcessShutdown("all robots stopped")
			}
		}
		if r.quit != nil {
			close(r.quit)
			r.quit = nil
		}
	})
}
func robotSmokeOnlyMode() bool {
	cfg := robotServerCfg()
	return cfg != nil && cfg.Robot != nil && cfg.Robot.EnableSmoke && !cfg.Robot.EnableStress
}

func (r *Robot) requestProcessShutdown(reason string) {
	if r == nil || r.shutdownRequested {
		return
	}
	r.shutdownRequested = true
	WarnfLimited("request_process_shutdown:"+reason, "robot[%s] request process shutdown: %s", r.name, reason)
	exitCode := 1
	if !strings.Contains(strings.ToLower(reason), "timeout") {
		exitCode = 0
	}
	if currentRobotRuntime != nil {
		currentRobotRuntime.requestShutdown(exitCode)
		return
	}
	if isGoTestProcess() {
		xlog.Debugf("robot[%s] skip process shutdown in go test process", r.name)
		return
	}
	if runtime.GOOS == "windows" {
		go func() {
			time.Sleep(200 * time.Millisecond)
			os.Exit(exitCode)
		}()
		return
	}
	pid := os.Getpid()
	proc, err := os.FindProcess(pid)
	if err != nil {
		ErrorfLimited("find_current_process_failed", "robot[%s] find current process failed: %v", r.name, err)
		go func() {
			time.Sleep(200 * time.Millisecond)
			os.Exit(exitCode)
		}()
		return
	}
	go func() {
		if err := proc.Signal(os.Interrupt); err != nil {
			ErrorfLimited("signal_current_process_failed", "robot[%s] signal current process failed: %v", r.name, err)
			time.Sleep(200 * time.Millisecond)
			os.Exit(exitCode)
		}
	}()
}

func (r *Robot) enabledSmokeModules() []string {
	if r != nil && len(r.smokeModules) > 0 {
		return append([]string(nil), r.smokeModules...)
	}
	cfgBase := robotServerCfg()
	if r == nil || cfgBase == nil || cfgBase.Robot == nil {
		return defaultSmokeModules()
	}
	cfg := cfgBase.Robot
	if !cfg.EnableSmoke && len(cfg.SmokeModules) == 0 {
		return nil
	}
	modules := cfg.SmokeModules
	if len(modules) == 0 {
		return defaultSmokeModules()
	}
	enabled := collectRobotModules(modules, false)
	if len(enabled) == 0 {
		return defaultSmokeModules()
	}
	return enabled
}
func defaultSmokeModules() []string {
	// 默认仅执行登录冒烟；其他模块需在配置 smokeModules 中显式声明。
	return []string{STATE_LOGIN}
}

func collectRobotModules(modules []string, allowStress bool) []string {
	enabled := make([]string, 0, len(modules))
	seen := make(map[string]struct{}, len(modules))
	for _, module := range modules {
		canonical := canonicalRobotModuleName(module, allowStress)
		if canonical == "" {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		enabled = append(enabled, canonical)
	}
	return enabled
}

func canonicalRobotModuleName(module string, allowStress bool) string {
	raw := strings.ToLower(strings.TrimSpace(module))
	if raw == "" {
		return ""
	}
	switch raw {
	case "login":
		return STATE_LOGIN
	case "item", "bag", "pack":
		return "ITEM"
	case "task", "tasks":
		return "TASK"
	}
	return raw
}

func (r *Robot) Start() {
	basal.SafeGo(func() {
		r.SetState(STATE_AUTH)
		timer := time.NewTimer(nextRobotActionInterval())
		defer timer.Stop()
		runtimeDone := robotRuntimeDone()
		for {
			select {
			case <-runtimeDone:
				WarnfLimited("robot_runtime_quit", "robot %v quit by runtime stop", r.name)
				return
			case <-r.quit:
				WarnfLimited("robot_quit", "robot %v quit", r.name)
				return
			case f := <-r.taskChan:
				if f == nil {
					return
				}

				basal.SafeRun(func() {
					f()
				})
			case <-timer.C:
				if r.curState != nil {
					r.curState.onUpdate(r)
				}
				if r.curState == nil {
					// 随机权重
				}
				timer.Reset(nextRobotActionInterval())
			}
		}
	})
}

func robotActionInterval() time.Duration {
	cfgBase := robotServerCfg()
	if cfgBase == nil || cfgBase.Robot == nil {
		return time.Second
	}
	ms := cfgBase.Robot.ActionIntervalMs
	if ms <= 0 {
		return time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

func nextRobotActionInterval() time.Duration {
	return jitterDuration(robotActionInterval())
}

func jitterDuration(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	delta := base / 10
	if delta <= 0 {
		return base
	}
	return base - delta + time.Duration(rand.Int63n(int64(delta*2)+1))
}

func NewRobot() *Robot {
	name := robotServerCfg().Robot.Name
	if name == "" {
		name = fmt.Sprintf("robot_%d", fastid.GenInt64ID())
	}
	robot := &Robot{
		name:     name,
		states:   map[string]IState{},
		taskChan: make(chan func(), 1000),
		quit:     make(chan struct{}),
	}
	xlog.Debugf("new robot %v", robot.name)
	robot.msgHandler = NewMsgHandler(robot)
	robot.RegisterStates()

	for _, state := range robot.states {
		state.register(robot)
	}
	return robot
}

func (r *Robot) recordSmokeResult(module, step string, success bool, summary string) {
	stopOnFailure := false
	module = strings.ToUpper(strings.TrimSpace(module))
	step = strings.TrimSpace(step)
	if !success && r.shouldHandleSingleModuleSmokeFailure(module) && RobotSvr != nil {
		shouldStop, signature := RobotSvr.shouldStopSmokeOnFailure(module, step, summary)
		if shouldStop {
			stopOnFailure = true
			r.smokeFailureStop = true
			r.smokeFailureModule = module
			if signature != "" {
				summary = summary + " signature=" + signature
			}
		} else {
			success = true
			if signature != "" {
				summary = "ignored_known_error signature=" + signature + " original=" + summary
			} else {
				summary = "ignored_known_error original=" + summary
			}
		}
	}
	r.smokeMu.Lock()
	if r.SmokeResults == nil {
		r.SmokeResults = make(map[string]*SmokeResult)
	}
	if r.SmokeModuleStarted == nil {
		r.SmokeModuleStarted = make(map[string]time.Time)
	}
	key := module + ":" + step
	if _, ok := r.SmokeModuleStarted[module]; !ok {
		r.SmokeModuleStarted[module] = time.Now()
	}
	r.SmokeResults[key] = &SmokeResult{
		Module:   module,
		Step:     step,
		Received: true,
		Success:  success,
		Summary:  summary,
		Updated:  time.Now(),
	}
	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		xlog.Debugf("robot[%s] [SMOKE][%s][%s] success=%v summary=%s", r.name, module, step, success, summary)
	}
	r.smokeMu.Unlock()
	if stopOnFailure {
		WarnfLimited("smoke_new_failure:"+module+":"+step, "robot[%s] smoke module=%s step=%s found new failure, stop and keep account scene: %s", r.name, module, step, summary)
		r.Stop()
	}
}

func (r *Robot) Send(data proto.Message) {
	if robotRuntimeStopping() || r == nil || r.msgque == nil {
		return
	}
	rspId := r.msgHandler.GetRspIdByType(data)
	if rspId <= 0 {
		WarnfLimited("robot_send_rsp_id_failed", "robot %v send rsp id %v failed", r.name, rspId)
		//r.Stop()
		return
	}
	RobotSvr.stat.AddReq(r.name, rspId.String())
	r.seq++
	send := msg.NewMsg(rspId, kit.PbData(data)).SetSeq(r.seq)
	r.msgque.Send(send)
}

func (r *Robot) shouldHandleSingleModuleSmokeFailure(module string) bool {
	if r == nil || r.shutdownRequested || r.cleanupRequested || r.isContinuousMode() || !robotSmokeOnlyMode() {
		return false
	}
	if !r.smokeModuleEnabled(module) {
		return false
	}
	modules := r.enabledSmokeModules()
	return len(modules) == 1 && strings.EqualFold(modules[0], module)
}

func (r *Robot) isContinuousMode() bool {
	if r == nil {
		return false
	}
	modules := r.enabledRuntimeModules()
	if len(modules) != 1 {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(modules[0])) {
	case STATE_BATTLE, STATE_STRESS:
		return true
	default:
		return false
	}
}

func (r *Robot) smokeModuleEnabled(module string) bool {
	name := strings.ToLower(strings.TrimSpace(module))
	for _, enabled := range r.enabledRuntimeModules() {
		if enabled == name {
			return true
		}
	}
	return false
}

func (r *Robot) enabledRuntimeModules() []string {
	cfgBase := robotServerCfg()
	if r == nil || cfgBase == nil || cfgBase.Robot == nil {
		return defaultSmokeModules()
	}
	cfg := cfgBase.Robot
	enabled := make([]string, 0, 1+len(cfg.SmokeModules))
	seen := make(map[string]struct{}, 1+len(cfg.SmokeModules))
	if cfg.EnableStress {
		enabled = append(enabled, STATE_STRESS)
		seen[STATE_STRESS] = struct{}{}
	}
	for _, module := range r.enabledSmokeModules() {
		if _, ok := seen[module]; ok {
			continue
		}
		seen[module] = struct{}{}
		enabled = append(enabled, module)
	}
	if len(enabled) == 0 {
		return defaultSmokeModules()
	}
	return enabled
}
