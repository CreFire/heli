package controller

import (
	"encoding/json"
	"fmt"
	"game/deps/xjson"
	"game/deps/xlog"
	"game/src/proto/pb"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type modelStruct struct {
	CmdId   pb.MSG_ID
	CmdName string
	C2sList []messageStruct
}

type messageStruct struct {
	C2sName   string
	ActId     pb.MSG_ID
	Structure map[string]any
}

type modeStateInfo struct {
	Active int32 `json:"active"`
	Weight int32 `json:"weight"`
	HitNum int64 `json:"hitNum"`
}

type webTableHeader struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

type smokeResultsResponse struct {
	Error   int                  `json:"error"`
	Summary smokeSummary         `json:"summary"`
	Modules []smokeModuleSummary `json:"modules"`
	Robots  []smokeRobotResult   `json:"robots"`
}

type smokeSummary struct {
	RobotNum int       `json:"robotNum"`
	Total    int       `json:"total"`
	Success  int       `json:"success"`
	Failed   int       `json:"failed"`
	Pending  int       `json:"pending"`
	Updated  time.Time `json:"updated"`
}

type smokeModuleSummary struct {
	Module  string `json:"module"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
	Pending int    `json:"pending"`
	Done    bool   `json:"done"`
}

type smokeRobotResult struct {
	Name     string    `json:"name"`
	GID      int64     `json:"gid"`
	Module   string    `json:"module"`
	Step     string    `json:"step"`
	Received bool      `json:"received"`
	Success  bool      `json:"success"`
	Summary  string    `json:"summary"`
	Updated  time.Time `json:"updated"`
}

func getSmokeResults(w http.ResponseWriter, r *http.Request) {
	setupCors(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ret := buildSmokeResults()
	buffer, _ := json.MarshalIndent(ret, "", "  ")
	_, _ = w.Write(buffer)
}

func buildSmokeResults() smokeResultsResponse {
	ret := smokeResultsResponse{Error: 0}
	if RobotSvr == nil || RobotSvr.robotMgr == nil {
		ret.Modules = []smokeModuleSummary{}
		ret.Robots = []smokeRobotResult{}
		return ret
	}

	moduleMap := make(map[string]*smokeModuleSummary)
	ensureModule := func(module string) *smokeModuleSummary {
		module = strings.ToUpper(strings.TrimSpace(module))
		if module == "" {
			module = "UNKNOWN"
		}
		item := moduleMap[module]
		if item == nil {
			item = &smokeModuleSummary{Module: module}
			moduleMap[module] = item
		}
		return item
	}

	ret.Summary.RobotNum = RobotSvr.robotMgr.GamerCount()
	seenDetails := make(map[string]struct{})
	addDetail := func(result smokeRobotResult) {
		result.Module = strings.ToUpper(strings.TrimSpace(result.Module))
		if result.Module == "" {
			result.Module = "UNKNOWN"
		}
		item := ensureModule(result.Module)
		item.Total++
		ret.Summary.Total++
		if result.Success {
			item.Success++
			ret.Summary.Success++
		} else {
			item.Failed++
			ret.Summary.Failed++
		}
		if result.Updated.After(ret.Summary.Updated) {
			ret.Summary.Updated = result.Updated
		}
		ret.Robots = append(ret.Robots, result)
		seenDetails[result.Name+":"+result.Module+":"+result.Step] = struct{}{}
	}
	RobotSvr.robotMgr.foreach(func(_ int64, robot *Robot) bool {
		if robot == nil {
			return true
		}
		for _, module := range robot.enabledSmokeModules() {
			module = strings.ToUpper(strings.TrimSpace(module))
			item := ensureModule(module)
			if !robot.smokeModuleDone(module) {
				item.Pending++
				ret.Summary.Pending++
			}
		}

		results := robot.smokeResultSnapshot()
		for key, result := range results {
			module := strings.ToUpper(strings.TrimSpace(result.Module))
			step := result.Step
			if module == "" || step == "" {
				parts := strings.SplitN(key, ":", 2)
				if module == "" && len(parts) > 0 {
					module = parts[0]
				}
				if step == "" && len(parts) > 1 {
					step = parts[1]
				}
			}
			addDetail(smokeRobotResult{
				Name:     robot.name,
				GID:      robot.gid,
				Module:   module,
				Step:     step,
				Received: result.Received,
				Success:  result.Success,
				Summary:  result.Summary,
				Updated:  result.Updated,
			})
		}
		return true
	})
	for _, result := range RobotSvr.smokeHistorySnapshot() {
		key := result.Name + ":" + strings.ToUpper(strings.TrimSpace(result.Module)) + ":" + result.Step
		if _, ok := seenDetails[key]; ok {
			continue
		}
		addDetail(result)
	}

	ret.Modules = make([]smokeModuleSummary, 0, len(moduleMap))
	for _, item := range moduleMap {
		item.Done = item.Total > 0 && item.Failed == 0 && item.Pending == 0
		ret.Modules = append(ret.Modules, *item)
	}
	sort.Slice(ret.Modules, func(i, j int) bool {
		return ret.Modules[i].Module < ret.Modules[j].Module
	})
	sort.Slice(ret.Robots, func(i, j int) bool {
		left, right := ret.Robots[i], ret.Robots[j]
		if left.Success != right.Success {
			return !left.Success
		}
		if left.Module != right.Module {
			return left.Module < right.Module
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Step < right.Step
	})
	return ret
}
func (r *Robot) smokeSuccess(module, step string) bool {
	if r == nil {
		return false
	}
	r.smokeMu.RLock()
	defer r.smokeMu.RUnlock()
	if r.SmokeResults == nil {
		return false
	}
	result := r.SmokeResults[module+":"+step]
	return result != nil && result.Received && result.Success
}
func (r *Robot) smokeModuleDone(module string) bool {
	switch strings.ToUpper(strings.TrimSpace(module)) {
	case "LOGIN":
		return r.smokeSuccess("LOGIN", "main")
	}
	return false
}
func setRobotNum(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"error": 0}
	setupCors(w)
	defer HttpSendResponse(w, result)
	if RobotSvr == nil || RobotSvr.robotMgr == nil {
		result["error"] = 1
		result["message"] = "robot server not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_ = r.ParseForm()
	targetNum := parseFormInt32(r, "robotNum", -1)
	loginRate := parseFormInt32(r, "robotFrameNum", 0)
	robotNumOneFrame := parseFormInt32(r, "robotNumOneFrame", 0)
	robotAddOneFrame := parseFormInt32(r, "robotAddOneFrame", 0)
	cfg := robotServerCfg()
	if cfg == nil || cfg.Robot == nil {
		result["error"] = 1
		result["message"] = "robot config not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if targetNum < 0 {
		result["error"] = 1
		result["message"] = "robotNum must be >= 0"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if loginRate <= 0 {
		loginRate = cfg.Robot.LoginRate
		if loginRate <= 0 {
			loginRate = 1
		}
	}
	cfg.Robot.LoginRate = loginRate
	cfg.Robot.Count = targetNum
	current := int32(RobotSvr.robotMgr.GamerCount())
	switch {
	case targetNum > current:
		go spawnRobots(targetNum-current, loginRate)
	case targetNum < current:
		stopRobots(current - targetNum)
	}
	if RobotSvr.stat != nil {
		RobotSvr.stat.UpdateRobotMeta(int64(targetNum), int64(loginRate))
	}
	result["robotNum"] = targetNum
	result["robotFrameNum"] = loginRate
	result["robotNumOneFrame"] = loOrDefaultInt32(robotNumOneFrame, loginRate)
	result["robotAddOneFrame"] = loOrDefaultInt32(robotAddOneFrame, 0)
}

func setModeWeight(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"error": 0}
	setupCors(w)
	defer HttpSendResponse(w, result)
	_ = r.ParseForm()
	raw := strings.TrimSpace(r.FormValue("data"))
	if raw == "" {
		result["error"] = 1
		result["message"] = "data is empty"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var items []struct {
		Key    string `json:"key"`
		Weight int32  `json:"weight"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		result["error"] = 1
		result["message"] = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	cfg := robotServerCfg()
	if cfg == nil || cfg.Robot == nil {
		result["error"] = 1
		result["message"] = "robot config not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if cfg.Robot.StressWeights == nil {
		cfg.Robot.StressWeights = make(map[string]int32)
	}
	for _, item := range items {
		module := canonicalStressModuleName(item.Key)
		if module == "" {
			module = canonicalStressModuleName(strings.ToLower(strings.TrimSpace(item.Key)))
		}
		if module == "" {
			continue
		}
		cfg.Robot.StressWeights[module] = item.Weight
	}
	// applyStressWeightsToRobots(cfg.Robot.StressWeights)
	result["data"] = buildModeStateMap()
}
func canonicalStressModuleName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	default:
		return ""
	}
}
func getAllMsg(w http.ResponseWriter, r *http.Request) {
	setupCors(w)
	result := map[string]any{"error": 0}
	if RobotSvr == nil || RobotSvr.stat == nil {
		result["status"] = -1
		HttpSendResponse(w, result)
		return
	}
	stat := RobotSvr.stat.Copy()
	headers := []webTableHeader{
		{Text: "消息号", Value: "消息号"},
		{Text: "请求数量", Value: "请求数量"},
		{Text: "响应数量", Value: "响应数量"},
		{Text: "响应率", Value: "响应率"},
		{Text: "最小响应", Value: "最小响应"},
		{Text: "最大响应", Value: "最大响应"},
		{Text: "平均响应", Value: "平均响应"},
		{Text: "50%分位", Value: "50%分位"},
		{Text: "75%分位", Value: "75%分位"},
		{Text: "90%分位", Value: "90%分位"},
		{Text: "TPS", Value: "TPS"},
		{Text: "成功率", Value: "成功率"},
	}
	rows := make([]map[string]any, 0, len(stat.StateMap))
	for name, info := range stat.StateMap {
		if info == nil {
			continue
		}
		respNum := info.SuccessNum + info.ErrorNum
		row := map[string]any{
			"消息号":   name,
			"请求数量":  info.ReqNum,
			"响应数量":  respNum,
			"响应率":   formatPercent(respNum, info.ReqNum),
			"最小响应":  safeMinTime(info.MinTime),
			"最大响应":  info.MaxTime,
			"平均响应":  info.AvgTime,
			"50%分位": percentileOrZero(info, 0.50),
			"75%分位": percentileOrZero(info, 0.75),
			"90%分位": percentileOrZero(info, 0.90),
			"TPS":   info.SecRPS,
			"成功率":   formatPercent(info.SuccessNum, respNum),
		}
		rows = append(rows, row)
	}
	result["data"] = map[string]any{
		"headers":   headers,
		"stat":      rows,
		"total_tps": stat.SecRPS,
	}
	HttpSendResponse(w, result)
}

func stopRobot(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"error": 0}
	setupCors(w)
	defer HttpSendResponse(w, result)
	if currentRobotRuntime == nil {
		result["error"] = 1
		result["message"] = "runtime not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	go currentRobotRuntime.requestShutdown(0)
	result["message"] = "stopping"
}

func runMode(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"error": 0}
	setupCors(w)
	defer HttpSendResponse(w, result)
	_ = r.ParseForm()
	cmd := strings.TrimSpace(r.FormValue("cmd"))
	runType := strings.ToLower(strings.TrimSpace(r.FormValue("runType")))
	if cmd == "" || (runType != "run" && runType != "stop") {
		result["error"] = 1
		result["message"] = "invalid cmd or runType"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	cfg := robotServerCfg()
	if cfg == nil || cfg.Robot == nil {
		result["error"] = 1
		result["message"] = "robot config not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if cfg.Robot.StressWeights == nil {
		cfg.Robot.StressWeights = make(map[string]int32)
	}
	for _, part := range strings.Split(cmd, ",") {
		module := canonicalStressModuleName(strings.TrimSpace(part))
		if module == "" {
			continue
		}
		switch runType {
		case "run":
			if cfg.Robot.StressWeights[module] <= 0 {
				cfg.Robot.StressWeights[module] = 1
			}
		case "stop":
			cfg.Robot.StressWeights[module] = 0
		}
	}
	// applyStressWeightsToRobots(cfg.Robot.StressWeights)
	result["data"] = buildModeStateMap()
}

func SendMessage(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"error": 0}
	setupCors(w)
	defer HttpSendResponse(w, result)
	r.ParseForm()
	xlog.Debugf("send message: %v", r.Form)
	c2s := r.FormValue("c2s")
	params := r.FormValue("params")
	req, err := CreateInstanceFromMessageName(fmt.Sprintf("pb.%v", ConvertToGoStructName(c2s)))

	err = xjson.LoadStringTo(params, req)
	if err != nil {
		return
	}
	RobotSvr.robotMgr.foreach(func(i int64, robot *Robot) bool {
		robot.Send(req)
		return true
	})
	xlog.Debugf("send message: %v", c2s)
}

func QueryInfo(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"error": 0}
	setupCors(w)
	defer HttpSendResponse(w, result)
	cfg := robotServerCfg()
	loginRate := int32(0)
	if cfg != nil && cfg.Robot != nil {
		loginRate = cfg.Robot.LoginRate
	}
	result["robotNum"] = RobotSvr.robotMgr.GamerCount()
	result["robotFrameNum"] = loginRate
	result["robotNumOneFrame"] = loginRate
	result["robotAddOneFrame"] = 0
	result["info"] = buildModeStateMap()
}

func setupCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

func GetAllC2S(w http.ResponseWriter, r *http.Request) {
	setupCors(w)
	result := map[string]any{"error": 0}
	data := map[string]*modelStruct{}
	for msgId, name := range pb.MSG_ID_name {
		parts := strings.Split(name, "_")
		l := len(parts)
		if l == 0 || parts[l-1] != "REQ" {
			continue
		}
		req, err := CreateInstanceFromMessageName(fmt.Sprintf("pb.%v", ConvertToGoStructName(name)))
		if err != nil {
			continue
		}
		sInfo, ok := data[parts[0]]
		if !ok {
			data[parts[0]] = &modelStruct{
				CmdId:   pb.MSG_ID(msgId),
				CmdName: parts[0],
				C2sList: make([]messageStruct, 0),
			}
			sInfo = data[parts[0]]
		}
		sInfo.C2sList = append(sInfo.C2sList, messageStruct{
			C2sName:   name,
			ActId:     pb.MSG_ID(msgId),
			Structure: getFieldStruct(reflect.TypeOf(req).Elem()),
		})
	}
	xlog.Warnf("")
	result["data"] = data
	HttpSendResponse(w, result)
}

// CreateInstanceFromMessageName 根据消息名称创建消息实例
func CreateInstanceFromMessageName(messageName string) (proto.Message, error) {
	fullName := protoreflect.FullName(messageName)

	messageType, err := protoregistry.GlobalTypes.FindMessageByName(fullName)
	if err != nil {
		return nil, fmt.Errorf("找不到消息类型 %s: %v", messageName, err)
	}

	message := messageType.New().Interface()
	return message, nil
}

// ConvertToGoStructName 将下划线命名转换为 Go 结构体名称
func ConvertToGoStructName(input string) string {
	parts := strings.Split(input, "_")

	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}

		if isAbbreviation(part) {
			result.WriteString(part)
		} else {
			if len(part) > 0 {
				result.WriteString(strings.ToUpper(part[:1]))
				if len(part) > 1 {
					result.WriteString(strings.ToLower(part[1:]))
				}
			}
		}
	}

	return result.String()
}

// isAbbreviation 判断字符串是否为缩写词
func isAbbreviation(s string) bool {
	if len(s) < 2 || len(s) > 4 {
		return false
	}

	for _, r := range s {
		if !unicode.IsUpper(r) {
			return false
		}
	}

	commonAbbreviations := map[string]bool{"REQ": true, "RSP": true, "NTF": true}

	return commonAbbreviations[s]
}

func getFieldStruct(c2s reflect.Type) map[string]any {
	result := make(map[string]any)
	for i := 0; i < c2s.NumField(); i++ {
		field := c2s.Field(i)
		kind := field.Type.Kind()
		if field.Tag.Get("protobuf") == "" {
			continue
		}
		if kind == reflect.Pointer {
			kind = field.Type.Elem().Kind()
		}
		if kind == reflect.Struct {
			if field.Type.Kind() == reflect.Pointer {
				result[field.Name] = getFieldStruct(field.Type.Elem())
			} else {
				result[field.Name] = getFieldStruct(field.Type)
			}
		} else {
			switch kind {
			case reflect.String:
				result[field.Name] = ""
			case reflect.Bool:
				result[field.Name] = false
			case reflect.Slice:
				dataType := field.Type.Elem()
				if dataType.Kind() == reflect.Pointer {
					dataType = dataType.Elem()
				}
				if dataType.Kind() == reflect.Struct {
					result[field.Name] = []any{getFieldStruct(dataType)}
				} else if dataType.Kind() == reflect.String {
					result[field.Name] = ""
				} else if dataType.Kind() == reflect.Bool {
					result[field.Name] = false
				} else {
					result[field.Name] = []int32{0}
				}
			default:
				result[field.Name] = 0
			}
		}
	}
	return result
}

func HttpSendResponse(w http.ResponseWriter, data map[string]any) error {
	if data == nil {
		return nil
	}
	res, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = w.Write(res)
	return err
}

func getStatistics(w http.ResponseWriter, r *http.Request) {
	setupCors(w)
	ret := RobotSvr.stat.Copy()
	buffer, _ := json.MarshalIndent(ret, "", "  ")
	_, _ = w.Write(buffer)
}

func buildModeStateMap() map[string]modeStateInfo {
	result := make(map[string]modeStateInfo)
	cfg := robotServerCfg()
	if cfg == nil || cfg.Robot == nil {
		return result
	}
	hitData := buildModuleHitData()
	for _, module := range []string{} {
		upperName := strings.ToUpper(module)
		weight := cfg.Robot.StressWeights[module]
		active := int32(0)
		if weight > 0 {
			active = 1
		}
		result[upperName] = modeStateInfo{
			Active: active,
			Weight: weight,
			HitNum: hitData[upperName],
		}
	}
	return result
}

func buildModuleHitData() map[string]int64 {
	result := make(map[string]int64)
	if RobotSvr == nil || RobotSvr.stat == nil {
		return result
	}
	stat := RobotSvr.stat.Copy()
	for reqName, info := range stat.StateMap {
		if info == nil {
			continue
		}
		module := strings.ToUpper(strings.TrimSpace(strings.Split(reqName, "_")[0]))
		if module == "" {
			continue
		}
		result[module] += info.ReqNum
	}
	return result
}

// func applyStressWeightsToRobots(weights map[string]int32) {
// 	if RobotSvr == nil || RobotSvr.robotMgr == nil {
// 		return
// 	}
// 	allModules := []string{}
// 	RobotSvr.robotMgr.foreach(func(_ int64, robot *Robot) bool {
// 		state, _ := robot.states[STATE_STRESS].(*StateStress)
// 		if state == nil {
// 			return true
// 		}
// 		state.ensurePicker()
// 		if state.modulePicker == nil {
// 			return true
// 		}
// 		for _, module := range allModules {
// 			state.modulePicker.Update(module, weights[module])
// 		}
// 		return true
// 	})
// }

func spawnRobots(count, loginRate int32) {
	if count <= 0 {
		return
	}
	if loginRate <= 0 {
		loginRate = 1
	}
	interval := time.Second / time.Duration(loginRate)
	for i := int32(0); i < count; i++ {
		if robotRuntimeStopping() {
			return
		}
		robot := newRobotForSpawn(i)
		robot.Start()
		if interval > 0 {
			time.Sleep(interval)
		}
	}
}

func stopRobots(count int32) {
	if count <= 0 || RobotSvr == nil || RobotSvr.robotMgr == nil {
		return
	}
	robots := make([]*Robot, 0, count)
	RobotSvr.robotMgr.foreach(func(_ int64, robot *Robot) bool {
		if robot == nil {
			return true
		}
		robots = append(robots, robot)
		return int32(len(robots)) < count
	})
	for _, robot := range robots {
		robot.Stop()
	}
}

func percentileOrZero(info *StateInfo, percent float64) int64 {
	if info == nil || info.latencySampleCount() == 0 {
		return 0
	}
	return info.calculatePercentile(percent)
}

func safeMinTime(v int64) int64 {
	if v == math.MaxInt64 {
		return 0
	}
	return v
}

func formatPercent(num, denom int64) string {
	return fmt.Sprintf("%.2f%%", ratioPercent(num, denom))
}

func parseFormInt32(r *http.Request, key string, defaultVal int32) int32 {
	if r == nil {
		return defaultVal
	}
	raw := strings.TrimSpace(r.FormValue(key))
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return defaultVal
	}
	return int32(v)
}

func loOrDefaultInt32(v, defaultVal int32) int32 {
	if v == 0 {
		return defaultVal
	}
	return v
}
