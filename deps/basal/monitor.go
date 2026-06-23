package basal

import (
	"game/deps/xlog"
	"runtime"
	"sync"
	"time"
)

// monitorElapsed 用于记录某个函数的执行耗时统计
type monitorElapsed struct {
	funcName    string // 被监控的函数名称
	totalTime   int64  // 累计总耗时（纳秒）
	longestTime int64  // 单次最长耗时（纳秒）
	longestAt   time.Time
	totalNum    int64 // 调用总次数
}

// newMonitorElapsed 创建一个新的函数耗时统计记录
func newMonitorElapsed(funcName string, totalTime int64, longestAt time.Time) *monitorElapsed {
	return &monitorElapsed{
		funcName:    funcName,
		totalTime:   totalTime,
		longestTime: totalTime,
		longestAt:   longestAt,
		totalNum:    1,
	}
}

// MonitorMgr 管理多个函数的耗时监控
type MonitorMgr struct {
	lock    sync.Mutex                 // 保护 timeMap 的互斥锁
	timeMap map[string]*monitorElapsed // 保存所有函数的耗时统计数据
	quit    chan struct{}              // 结束信号
	ticker  *time.Ticker               // 定时器，用于定时输出统计信息
}

// Start 启动监控循环
func (m *MonitorMgr) Start() {
	if m != nil {
		go m.loop() // 启动独立 goroutine 循环统计
	}
}

// Stop 停止监控
func (m *MonitorMgr) Stop() {
	if m != nil {
		close(m.quit)
	}
}

// RecordTime 返回一个闭包函数，用于记录目标函数执行的耗时
//
// 使用方式：
// defer monitorMgr.RecordTime("FuncName")()
//
// 调用 RecordTime 时会记录开始时间，闭包函数执行时计算耗时并存入 timeMap
func (m *MonitorMgr) RecordTime(funcName string) func() {
	start := time.Now()
	return func() {
		now := time.Now()
		duration := now.Sub(start).Nanoseconds()
		m.lock.Lock()
		defer m.lock.Unlock()
		elapsed := m.timeMap[funcName]
		if elapsed == nil {
			// 首次记录该函数
			m.timeMap[funcName] = newMonitorElapsed(funcName, duration, now)
			return
		}
		// 累计耗时
		elapsed.totalTime += duration
		if duration > elapsed.longestTime {
			elapsed.longestTime = duration
			elapsed.longestAt = now
		}
		elapsed.totalNum++
	}
}

// statisticTime 输出当前所有监控函数的耗时统计
// - total time：总耗时（ms）
// - ave time：平均耗时（ms）
// - longest time：最长耗时（ms）
// - longest at：最长耗时发生时间
// - atom num / atom time / atom ave：本统计周期内的数据
func (m *MonitorMgr) statisticTime() {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, v := range m.timeMap {
		tTime := float32(v.totalTime) / float32(1000000) // 总耗时（ms）
		num := v.totalNum
		aveTime := tTime / float32(num) // 平均耗时（ms）
		lTime := float32(v.longestTime) / float32(1000000)
		longestAt := ""
		if !v.longestAt.IsZero() {
			longestAt = v.longestAt.Format("2006-01-02 15:04:05.000")
		}

		xlog.Infof("-- monitor -- elapsed time:%s total num: %d total time:%.2fms ave time: %.2fms longest time: %.2fms longest at: %s",
			v.funcName, num, tTime, aveTime, lTime, longestAt)
	}
}

func (m *MonitorMgr) memAndGoroutine() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	xlog.Infof("-- monitor -- current goroutine num:%d gc num:%v stw time:%v", runtime.NumGoroutine(), memStats.NumGC, time.Duration(memStats.PauseTotalNs))
	memUse := float32(memStats.Alloc) / float32(1024) / float32(1024)
	xlog.Infof("-- monitor -- current use mem:%.2f MB", memUse)
}

// loop 定时循环输出耗时统计
func (m *MonitorMgr) loop() {
	for {
		select {
		case <-m.quit: // 收到退出信号
			m.ticker.Stop()
			return
		case <-m.ticker.C: // 每分钟触发一次
			m.statisticTime()
			m.memAndGoroutine()
		}
	}
}

// NewMonitorMgr 创建一个新的 MonitorMgr
// 默认每分钟输出一次统计结果
func NewMonitorMgr() *MonitorMgr {
	return &MonitorMgr{
		timeMap: map[string]*monitorElapsed{},
		quit:    make(chan struct{}),
		ticker:  time.NewTicker(time.Minute),
	}
}
