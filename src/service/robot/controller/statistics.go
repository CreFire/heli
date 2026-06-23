package controller

import (
	"game/deps/basal"
	"game/deps/xlog"
	"maps"
	"math"
	"sort"
	"time"
)

var bucketBoundaries = []int64{
	5, 10, 15, 20, 25, 30, 35, 40, 45, 50, // 5-50ms，每5ms一个桶
	60, 70, 80, 90, 100, // 50-100ms，每10ms一个桶
	120, 140, 160, 180, 200, // 100-200ms，每20ms一个桶
	250, 300, 350, 400, 450, 500, // 200-500ms，每50ms一个桶
	600, 700, 800, 900, 1000, // 500ms-1s，每100ms一个桶
	1200, 1400, 1600, 1800, 2000, // 1s-2s，每200ms一个桶
	2500, 3000, 3500, 4000, 4500, 5000, // 2s-5s，每500ms一个桶
	6000, 7000, 8000, 9000, 10000, // 5s-10s，每1s一个桶
	15000, 20000, 30000, 60000, // 10s以上的长尾桶
}

const battleRoundHistoryLimit = 100
const defaultRecentReqLimit = 200

type ReqTiming struct {
	RobotName  string `json:"robotName"`
	ReqName    string `json:"reqName"`
	DurationMs int64  `json:"durationMs"`
	Success    bool   `json:"success"`
	At         int64  `json:"at"`
}

type Statistics struct {
	TimeOut         int64 // 请求超时时间
	RobotNum        int64 // 机器人数量
	SpawnRate       int64 // 每秒启动多少个用户
	CurRobot        int
	SecRPS          int64
	_SecRPS         int64 // 每秒请求数量
	TotalRPS        int64 // 总请求数量
	SuccessNum      int64 // 成功数量
	ErrorNum        int64 // 失败数量
	TimeoutNum      int64 // 超时数量
	SecSuccessNum   int64
	_SecSuccessNum  int64
	SecErrorNum     int64
	_SecErrorNum    int64
	SecTimeoutNum   int64
	_SecTimeoutNum  int64
	StateMap        map[string]*StateInfo
	Overall         *StateInfo
	Battle          *BattleInfo
	ReqMap          map[string]map[string]time.Time
	RecentReqLimit  int64
	RecentReqs      []*ReqTiming
	task            chan func()
	quit            chan struct{}
	lastBattleLogAt int64
}

type BattleInfo struct {
	TotalNum       int64 // 总战斗数
	SuccessNum     int64 // 成功战斗数
	FailNum        int64 // 失败战斗数
	PerMinuteNum   int64 // 最近一分钟战斗数
	MinTime        int64 // 最小整场耗时
	MaxTime        int64 // 最大整场耗时
	AvgTime        int64 // 平均整场耗时
	P95Time        int64 // 95分位整场耗时
	TotalTime      int64 // 总整场耗时
	Buckets        map[int64]int64
	RecentFinished []int64            // 最近一分钟完成时间点
	RecentRounds   []*BattleRoundInfo // 最近若干局战斗时间
}

type BattleRoundInfo struct {
	Name       string
	StartedAt  int64
	FinishedAt int64
}

type StateInfo struct {
	ReqNum         int64 //请求数量
	SuccessNum     int64 //成功数量
	ErrorNum       int64 //失败数量
	TimeoutNum     int64 //超时数量
	SecRPS         int64 //每秒请求数量
	_SecReqNum     int64
	SecSuccNum     int64
	_SecSuccNum    int64
	SecErrNum      int64
	_SecErrNum     int64
	SecTimeoutNum  int64
	_SecTimeoutNum int64
	MinTime        int64           //最小时间
	MaxTime        int64           //最大时间
	AvgTime        int64           //平均时间
	TotalTime      int64           //总时间
	Buckets        map[int64]int64 // 响应时间分桶，键是桶的上限，值是该桶中的请求数量
	P95Time        int64           // 95%响应时间
	P99Time        int64           // 99%响应时间
}

func NewStateInfo() *StateInfo {
	buckets := make(map[int64]int64)
	// 初始化所有桶
	for _, boundary := range bucketBoundaries {
		buckets[boundary] = 0
	}
	return &StateInfo{
		MinTime: math.MaxInt64,
		Buckets: buckets,
	}
}

func NewBattleInfo() *BattleInfo {
	buckets := make(map[int64]int64)
	for _, boundary := range bucketBoundaries {
		buckets[boundary] = 0
	}
	return &BattleInfo{
		MinTime: math.MaxInt64,
		Buckets: buckets,
	}
}

// 找到响应时间对应的桶
func findBucket(responseTime int64) int64 {
	// 使用二分查找高效找到对应的桶
	idx := sort.Search(len(bucketBoundaries), func(i int) bool {
		return bucketBoundaries[i] >= responseTime
	})

	if idx < len(bucketBoundaries) {
		return bucketBoundaries[idx]
	}
	// 如果超过最大桶，使用最大桶
	return bucketBoundaries[len(bucketBoundaries)-1]
}

func (s *StateInfo) latencySampleCount() int64 {
	if s == nil {
		return 0
	}
	return s.SuccessNum + s.ErrorNum
}

// 使用分桶数据计算95%响应时间
func (s *StateInfo) calculatePercentile(percent float64) int64 {
	sampleCount := s.latencySampleCount()
	if sampleCount == 0 {
		return 0
	}
	target := int64(math.Ceil(float64(sampleCount) * percent))
	if target <= 0 {
		return s.MinTime
	}
	cumulative := int64(0)
	for _, boundary := range bucketBoundaries {
		cumulative += s.Buckets[boundary]
		if cumulative >= target {
			return boundary
		}
	}
	return s.MaxTime
}

func (s *StateInfo) CalculatePercentiles() {
	if s.latencySampleCount() == 0 {
		s.P95Time = 0
		s.P99Time = 0
		return
	}
	s.P95Time = s.calculatePercentile(0.95)
	s.P99Time = s.calculatePercentile(0.99)
}

func (b *BattleInfo) CalculatePercentiles() {
	if b.SuccessNum == 0 {
		b.P95Time = 0
		return
	}
	target := int64(float64(b.SuccessNum) * 0.95)
	if target <= 0 {
		b.P95Time = b.MinTime
		return
	}
	cumulative := int64(0)
	for _, boundary := range bucketBoundaries {
		cumulative += b.Buckets[boundary]
		if cumulative >= target {
			b.P95Time = boundary
			return
		}
	}
	b.P95Time = bucketBoundaries[len(bucketBoundaries)-1]
}

func (s *Statistics) Copy() *Statistics {
	c := make(chan *Statistics)
	s.addTask(func() {
		newStat := &Statistics{}
		newStat.RobotNum = s.RobotNum
		newStat.SpawnRate = s.SpawnRate
		newStat.SecRPS = s.SecRPS
		if RobotSvr != nil && RobotSvr.robotMgr != nil {
			newStat.CurRobot = RobotSvr.robotMgr.GamerCount()
		}
		newStat.TotalRPS = s.TotalRPS
		newStat.SuccessNum = s.SuccessNum
		newStat.ErrorNum = s.ErrorNum
		newStat.TimeoutNum = s.TimeoutNum
		newStat.SecSuccessNum = s.SecSuccessNum
		newStat.SecErrorNum = s.SecErrorNum
		newStat.SecTimeoutNum = s.SecTimeoutNum
		newStat.RecentReqLimit = s.RecentReqLimit
		newStat.StateMap = make(map[string]*StateInfo)
		if s.Overall != nil {
			newOverall := NewStateInfo()
			*newOverall = *s.Overall
			newOverall.Buckets = make(map[int64]int64)
			maps.Copy(newOverall.Buckets, s.Overall.Buckets)
			newStat.Overall = newOverall
		}
		for _, req := range s.RecentReqs {
			if req == nil {
				continue
			}
			copyReq := *req
			newStat.RecentReqs = append(newStat.RecentReqs, &copyReq)
		}
		if s.Battle != nil {
			newBattle := NewBattleInfo()
			newBattle.TotalNum = s.Battle.TotalNum
			newBattle.SuccessNum = s.Battle.SuccessNum
			newBattle.FailNum = s.Battle.FailNum
			newBattle.PerMinuteNum = s.Battle.PerMinuteNum
			newBattle.MinTime = s.Battle.MinTime
			newBattle.MaxTime = s.Battle.MaxTime
			newBattle.AvgTime = s.Battle.AvgTime
			newBattle.P95Time = s.Battle.P95Time
			newBattle.TotalTime = s.Battle.TotalTime
			maps.Copy(newBattle.Buckets, s.Battle.Buckets)
			newBattle.RecentFinished = append(newBattle.RecentFinished, s.Battle.RecentFinished...)
			for _, round := range s.Battle.RecentRounds {
				if round == nil {
					continue
				}
				copyRound := *round
				newBattle.RecentRounds = append(newBattle.RecentRounds, &copyRound)
			}
			newStat.Battle = newBattle
		}
		for name, info := range s.StateMap {
			newInfo := NewStateInfo()
			newInfo.ReqNum = info.ReqNum
			newInfo.SuccessNum = info.SuccessNum
			newInfo.ErrorNum = info.ErrorNum
			newInfo.TimeoutNum = info.TimeoutNum
			newInfo.SecRPS = info.SecRPS
			newInfo.SecSuccNum = info.SecSuccNum
			newInfo.SecErrNum = info.SecErrNum
			newInfo.SecTimeoutNum = info.SecTimeoutNum
			newInfo.MinTime = info.MinTime
			newInfo.MaxTime = info.MaxTime
			newInfo.AvgTime = info.AvgTime
			newInfo.TotalTime = info.TotalTime
			newInfo.P95Time = info.P95Time
			newInfo.P99Time = info.P99Time

			// 复制桶数据
			maps.Copy(newInfo.Buckets, info.Buckets)

			newStat.StateMap[name] = newInfo
		}
		c <- newStat
	})
	ret := <-c
	close(c)
	return ret
}

func (s *Statistics) update() {
	s.addTask(func() {
		nowMs := time.Now().UnixMilli()
		s.SecRPS = s._SecRPS
		s._SecRPS = 0
		s.SecSuccessNum = s._SecSuccessNum
		s._SecSuccessNum = 0
		s.SecErrorNum = s._SecErrorNum
		s._SecErrorNum = 0
		s.SecTimeoutNum = s._SecTimeoutNum
		s._SecTimeoutNum = 0
		for robotName, m := range s.ReqMap {
			for name, t := range m {
				if time.Since(t).Milliseconds() > s.TimeOut {
					delete(s.ReqMap[robotName], name)
					info, exist := s.StateMap[name]
					if !exist {
						info = NewStateInfo()
						s.StateMap[name] = info
					}
					// 超时请求也计入最大时间
					if s.TimeOut > info.MaxTime {
						info.MaxTime = s.TimeOut
					}
					info.TotalTime += s.TimeOut
					info.TimeoutNum++
					info._SecTimeoutNum++
					s.TimeoutNum++
					s._SecTimeoutNum++
				}
			}
		}
		for _, info := range s.StateMap {
			info.SecRPS = info._SecReqNum
			info._SecReqNum = 0
			info.SecSuccNum = info._SecSuccNum
			info._SecSuccNum = 0
			info.SecErrNum = info._SecErrNum
			info._SecErrNum = 0
			info.SecTimeoutNum = info._SecTimeoutNum
			info._SecTimeoutNum = 0
			if info.ReqNum > 0 {
				info.AvgTime = info.TotalTime / info.ReqNum
			}
			info.CalculatePercentiles()
		}
		if s.Overall != nil {
			if s.Overall.ReqNum > 0 {
				s.Overall.AvgTime = s.Overall.TotalTime / s.Overall.ReqNum
			}
			s.Overall.CalculatePercentiles()
		}
		s.logPerfSummary()
		if s.Battle != nil {
			cutoff := nowMs - int64(time.Minute/time.Millisecond)
			kept := s.Battle.RecentFinished[:0]
			for _, ts := range s.Battle.RecentFinished {
				if ts >= cutoff {
					kept = append(kept, ts)
				}
			}
			s.Battle.RecentFinished = kept
			s.Battle.PerMinuteNum = int64(len(s.Battle.RecentFinished))
			if s.Battle.SuccessNum > 0 {
				s.Battle.AvgTime = s.Battle.TotalTime / s.Battle.SuccessNum
			}
			s.Battle.CalculatePercentiles()
			if s.Battle.TotalNum > 0 && (s.lastBattleLogAt == 0 || nowMs-s.lastBattleLogAt >= int64(time.Minute/time.Millisecond)) {
				s.lastBattleLogAt = nowMs
				xlog.Infof("battle stats total=%d success=%d fail=%d perMinute=%d avgMs=%d p95Ms=%d minMs=%d maxMs=%d",
					s.Battle.TotalNum,
					s.Battle.SuccessNum,
					s.Battle.FailNum,
					s.Battle.PerMinuteNum,
					s.Battle.AvgTime,
					s.Battle.P95Time,
					loTernaryInt64(s.Battle.MinTime == math.MaxInt64, 0, s.Battle.MinTime),
					s.Battle.MaxTime,
				)
			}
		}
	})
}

func loTernaryInt64(cond bool, a, b int64) int64 {
	if cond {
		return a
	}
	return b
}

func (s *Statistics) AddReq(robotName, name string) {
	s.addTask(func() {
		if s.ReqMap[robotName] == nil {
			s.ReqMap[robotName] = make(map[string]time.Time)
		}
		s.ReqMap[robotName][name] = time.Now()
		info, exist := s.StateMap[name]
		if !exist {
			info = NewStateInfo()
			s.StateMap[name] = info
		}
		info.ReqNum++
		info._SecReqNum++
		s._SecRPS++
		s.TotalRPS++
	})
}

func (s *Statistics) AddRsp(robotName, name string, success bool) {
	s.addTask(func() {
		t, exist := s.ReqMap[robotName][name]
		if !exist {
			return
		}
		delete(s.ReqMap[robotName], name)
		end := time.Now()
		if success {
			s.SuccessNum++
			s._SecSuccessNum++
		} else {
			s.ErrorNum++
			s._SecErrorNum++
		}
		inv := end.Sub(t).Milliseconds()
		info, exist := s.StateMap[name]
		if !exist {
			info = NewStateInfo()
			s.StateMap[name] = info
		}
		if success {
			info.SuccessNum++
			info._SecSuccNum++
		} else {
			info.ErrorNum++
			info._SecErrNum++
		}
		if inv < info.MinTime {
			info.MinTime = inv
		}
		if inv > info.MaxTime {
			info.MaxTime = inv
		}
		info.TotalTime += inv

		// 将响应时间放入对应的桶中
		bucket := findBucket(inv)
		info.Buckets[bucket]++
		if s.Overall == nil {
			s.Overall = NewStateInfo()
		}
		s.Overall.ReqNum++
		if success {
			s.Overall.SuccessNum++
		} else {
			s.Overall.ErrorNum++
		}
		if inv < s.Overall.MinTime {
			s.Overall.MinTime = inv
		}
		if inv > s.Overall.MaxTime {
			s.Overall.MaxTime = inv
		}
		s.Overall.TotalTime += inv
		s.Overall.Buckets[bucket]++
		s.appendRecentReq(&ReqTiming{
			RobotName:  robotName,
			ReqName:    name,
			DurationMs: inv,
			Success:    success,
			At:         time.Now().UnixMilli(),
		})
	})
}

func (s *Statistics) logPerfSummary() {
	if s == nil {
		return
	}
	// model := buildStatsDashboardModel(s, defaultStatsDashboardTopN)
	// if statsTUIShouldRender() {
	// 	width, height := statsTUISize()
	// 	statsTUIDrawFrame(renderStatsDashboardFrame(model, width, height))
	// 	return
	// }
	// statsTUILogInfof("%s", buildStatsSummaryLogLine(model))
	// apiLine := buildStatsAPILogLine(model)
	// if apiLine != "" {
	// 	statsTUILogInfof("%s", apiLine)
	// }
}

var statsTUILogInfof = xlog.Infof

func ratioPercent(num, denom int64) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) * 100 / float64(denom)
}

func (s *Statistics) appendRecentReq(req *ReqTiming) {
	if s == nil || req == nil || s.RecentReqLimit <= 0 {
		return
	}
	s.RecentReqs = append(s.RecentReqs, req)
	if int64(len(s.RecentReqs)) > s.RecentReqLimit {
		s.RecentReqs = append([]*ReqTiming(nil), s.RecentReqs[len(s.RecentReqs)-int(s.RecentReqLimit):]...)
	}
}

func (s *Statistics) AddBattle(duration time.Duration, success bool) {
	s.addTask(func() {
		if s.Battle == nil {
			s.Battle = NewBattleInfo()
		}
		s.Battle.TotalNum++
		if !success {
			s.Battle.FailNum++
			return
		}
		inv := duration.Milliseconds()
		s.Battle.SuccessNum++
		s.Battle.TotalTime += inv
		s.Battle.RecentFinished = append(s.Battle.RecentFinished, time.Now().UnixMilli())
		s.Battle.AvgTime = s.Battle.TotalTime / s.Battle.SuccessNum
		if inv < s.Battle.MinTime {
			s.Battle.MinTime = inv
		}
		if inv > s.Battle.MaxTime {
			s.Battle.MaxTime = inv
		}
		bucket := findBucket(inv)
		s.Battle.Buckets[bucket]++
	})
}

func (s *Statistics) AddBattleRound(round *BattleRoundInfo) {
	s.addTask(func() {
		if s.Battle == nil {
			s.Battle = NewBattleInfo()
		}
		if round == nil {
			return
		}
		copyRound := *round
		s.Battle.RecentRounds = append(s.Battle.RecentRounds, &copyRound)
		if len(s.Battle.RecentRounds) > battleRoundHistoryLimit {
			s.Battle.RecentRounds = append([]*BattleRoundInfo(nil), s.Battle.RecentRounds[len(s.Battle.RecentRounds)-battleRoundHistoryLimit:]...)
		}
	})
}

func (s *Statistics) UpdateRobotMeta(robotNum, spawnRate int64) {
	if s == nil {
		return
	}
	s.addTask(func() {
		s.RobotNum = robotNum
		s.SpawnRate = spawnRate
	})
}

func (s *Statistics) Start() {
	basal.SafeGo(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.quit:
				return
			case f := <-s.task:
				f()
			case <-ticker.C:
				s.update()
			}
		}
	})
}

func (s *Statistics) Stop() {
	close(s.quit)
}

func (s *Statistics) addTask(f func()) {
	select {
	case s.task <- f:
	default:
		WarnfLimited("statistics_add_task_failed", "statistics add task failed")
	}
}

func NewStatistics(robotNum, spawnRate, timeout int32) *Statistics {
	return &Statistics{
		RobotNum:       int64(robotNum),
		SpawnRate:      int64(spawnRate),
		TimeOut:        int64(timeout),
		StateMap:       make(map[string]*StateInfo),
		Overall:        NewStateInfo(),
		Battle:         NewBattleInfo(),
		ReqMap:         make(map[string]map[string]time.Time),
		RecentReqLimit: defaultRecentReqLimit,
		task:           make(chan func(), 10000),
		quit:           make(chan struct{}),
	}
}
