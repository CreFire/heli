package timermgr

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"game/deps/basal"
	"game/deps/xlog"
	"game/deps/xtime"
	"sync/atomic"
	"time"

	"github.com/sasha-s/go-deadlock"
)

const catchUpThresholdSeconds int64 = 30
const millisPerSecond int64 = 1000

var (
	ErrTimerManagerStopped = errors.New("timer manager is stopped")
	ErrInvalidInterval     = errors.New("invalid timer interval")
	ErrInvalidTickCount    = errors.New("invalid tick count")
	ErrTimerNotFound       = errors.New("timer not found")
)

// TimerFunc 定时器回调函数类型
type TimerFunc func(name string, now int64, value any)

// CircTimer 循环定时器结构
type CircTimer struct {
	Name         string    // 定时器名称
	Id           int64     // 唯一标识
	MaxTickCount int32     // 最大执行次数，0表示无限循环
	Interval     int64     // 执行间隔（秒）
	Value        any       // 传递给回调函数的值
	Func         TimerFunc // 回调函数
	NextTimeMs   int64     // 下次执行时间戳（毫秒）
	TickCount    int32     // 已执行次数
	RunInGo      bool      // 是否在新的goroutine中执行
	index        int       // 在堆中的索引
	cancelled    int32     // 原子标记是否已取消
}

// IsCancelled 检查定时器是否已被取消
func (t *CircTimer) IsCancelled() bool {
	return atomic.LoadInt32(&t.cancelled) == 1
}

// Cancel 标记定时器为已取消状态
func (t *CircTimer) Cancel() {
	atomic.StoreInt32(&t.cancelled, 1)
}

// timerHeap 实现 heap.Interface 的定时器堆
type timerHeap []*CircTimer

func (h timerHeap) Len() int { return len(h) }

func (h timerHeap) Less(i, j int) bool {
	return h[i].NextTimeMs < h[j].NextTimeMs
}

func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *timerHeap) Push(x any) {
	n := len(*h)
	item := x.(*CircTimer)
	item.index = n
	*h = append(*h, item)
}

func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // 避免内存泄漏
	item.index = -1 // 安全标记
	*h = old[0 : n-1]
	return item
}

// TimerMgrConfig 定时器管理器配置
type TimerMgrConfig struct {
	TickInterval   time.Duration // ticker间隔，默认1秒
	ChannelBuffer  int           // 通道缓冲区大小，默认128
	MaxConcurrency int           // 最大并发执行数，默认128
	EnableMetrics  bool          // 是否启用指标统计
}

// DefaultConfig 返回默认配置
func DefaultConfig() *TimerMgrConfig {
	return &TimerMgrConfig{
		TickInterval:   time.Second,
		ChannelBuffer:  512,
		MaxConcurrency: 1000,
		EnableMetrics:  false,
	}
}

// TimerMgr 定时器管理器
type TimerMgr struct {
	config          *TimerMgrConfig
	timers          timerHeap
	timerMap        map[int64]*CircTimer
	ticker          *time.Ticker
	lastProcessTime int64

	// 通道
	addChan    chan *CircTimer
	cancelChan chan int64

	// 状态管理
	ctx     context.Context
	cancel  context.CancelFunc
	wg      deadlock.WaitGroup
	mu      deadlock.RWMutex
	running int32

	// 自增ID生成器
	TimerId int64

	// 指标统计
	metrics struct {
		totalAdded    int64
		totalCanceled int64
		totalExecuted int64
		activeTimers  int64
	}
}

// NewTimerMgr 创建新的定时器管理器
func NewTimerMgr() *TimerMgr {
	return NewTimerMgrWithConfig(DefaultConfig())
}

// NewTimerMgrWithConfig 使用指定配置创建定时器管理器
func NewTimerMgrWithConfig(config *TimerMgrConfig) *TimerMgr {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &TimerMgr{
		config:     config,
		timers:     make(timerHeap, 0),
		timerMap:   make(map[int64]*CircTimer),
		addChan:    make(chan *CircTimer, config.ChannelBuffer),
		cancelChan: make(chan int64, config.ChannelBuffer),
		ctx:        ctx,
		cancel:     cancel,
	}

	heap.Init(&m.timers)
	return m
}

// IsRunning 检查管理器是否正在运行
func (m *TimerMgr) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

// Start 启动定时器管理器
func (m *TimerMgr) Start() error {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return nil // 已经在运行
	}

	m.ticker = time.NewTicker(m.config.TickInterval)

	m.wg.Add(1)
	go m.mainLoop()

	xlog.Infof("timer manager started with tick interval: %v", m.config.TickInterval)
	return nil
}

// Stop 停止定时器管理器
func (m *TimerMgr) Stop() error {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return nil // 已经停止
	}

	// 取消上下文
	m.cancel()
	// 等待主循环退出
	m.wg.Wait()

	// 停止ticker
	if m.ticker != nil {
		m.ticker.Stop()
		m.ticker = nil
	}

	xlog.Infof("timer manager stopped")
	return nil
}

// mainLoop 主事件循环
func (m *TimerMgr) mainLoop() {
	defer m.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			xlog.Errorf("timer manager main loop panic: %v", r)
		}
	}()

	for {
		select {
		case <-m.ticker.C:
			nowMs := xtime.NowUnixMs()
			m.processTickers(nowMs)

		case timer := <-m.addChan:
			m.addTimerInternal(timer)

		case id := <-m.cancelChan:
			m.cancelTimerInternal(id)

		case <-m.ctx.Done():
			xlog.Infof("timer manager main loop exiting")
			return
		}
	}
}

// processTickers 处理到期的定时器
func (m *TimerMgr) processTickers(nowMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.detectTimeJump(nowMs)

	for m.timers.Len() > 0 {
		nextTimer := m.timers[0]
		nextTimeMs := nextTimer.NextTimeMs
		if nextTimeMs > nowMs {
			break
		}

		// 弹出到期的定时器
		timer := heap.Pop(&m.timers).(*CircTimer)

		// 检查是否已被取消
		if timer.IsCancelled() {
			delete(m.timerMap, timer.Id)
			continue
		}

		scheduleTimeMs := timer.NextTimeMs
		intervalMs := timer.Interval * millisPerSecond

		//实际需要执行数
		runtime := (nowMs-scheduleTimeMs)/intervalMs + 1
		if runtime > 1 {
			scheduleTimeMs += (runtime - 1) * intervalMs
			xlog.Warnf("timer %s id:%d catch-up detected, drift=%dms", timer.Name, timer.Id, nowMs-timer.NextTimeMs)
		}
		// 追帧逻辑，补齐执行时间，只追一帧
		m.executeTimer(timer, scheduleTimeMs, false)
		// 处理循环定时器
		m.handleRecurringTimer(timer, scheduleTimeMs)
	}
}

// executeTimer 执行定时器回调
func (m *TimerMgr) executeTimer(timer *CircTimer, scheduleTimeMs int64, forceSync bool) {
	if m.config.EnableMetrics {
		atomic.AddInt64(&m.metrics.totalExecuted, 1)
	}

	executeFunc := func() {
		startTimeMs := xtime.NowUnixMs()
		timer.Func(timer.Name, scheduleTimeMs/millisPerSecond, timer.Value)

		// 检查执行时间
		executionTimeMs := xtime.NowUnixMs() - startTimeMs
		if executionTimeMs > 60*millisPerSecond {
			xlog.Warnf("timer %s execution took too long: %dms", timer.Name, executionTimeMs)
		}
	}

	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		xlog.Debugf("execute timer id: %d name: %s args: %v count: %d, max count: %d,  at: %d,  next at: %d ",
			timer.Id, timer.Name, timer.Value, timer.TickCount, timer.MaxTickCount, scheduleTimeMs/millisPerSecond, (scheduleTimeMs+timer.Interval*millisPerSecond)/millisPerSecond)
	}

	if timer.RunInGo && !forceSync {
		basal.SafeGo(executeFunc)
	} else {
		basal.SafeRun(executeFunc)
	}
}

// handleRecurringTimer 处理循环定时器
func (m *TimerMgr) handleRecurringTimer(timer *CircTimer, scheduleTimeMs int64) {
	timer.TickCount++

	// 检查是否达到最大执行次数
	if timer.MaxTickCount > 0 && timer.TickCount >= timer.MaxTickCount {
		delete(m.timerMap, timer.Id)
		if m.config.EnableMetrics {
			atomic.AddInt64(&m.metrics.activeTimers, -1)
		}
		return
	}

	// 重新添加到堆中
	timer.NextTimeMs = scheduleTimeMs + timer.Interval*millisPerSecond
	heap.Push(&m.timers, timer)
}

// addTimerInternal 内部添加定时器方法
func (m *TimerMgr) addTimerInternal(timer *CircTimer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if timer == nil {
		return
	}
	if timer.NextTimeMs == 0 {
		timer.NextTimeMs = xtime.NowUnixMs() + timer.Interval*millisPerSecond
	}

	// 检查是否已存在
	if _, exists := m.timerMap[timer.Id]; exists {
		xlog.Errorf("timer with id %d already exists", timer.Id)
		return
	}

	heap.Push(&m.timers, timer)
	m.timerMap[timer.Id] = timer

	if m.config.EnableMetrics {
		atomic.AddInt64(&m.metrics.activeTimers, 1)
	}

	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		xlog.Debugf("add timer id: %d name: %s args: %v count: %d max count: %d ", timer.Id, timer.Name, timer.Value, timer.TickCount, timer.MaxTickCount)
	}
}

// cancelTimerInternal 内部取消定时器方法
func (m *TimerMgr) cancelTimerInternal(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	timer, exists := m.timerMap[id]
	if !exists {
		return
	}

	// 标记为已取消
	timer.Cancel()

	// 从堆中移除
	if timer.index >= 0 && timer.index < len(m.timers) {
		heap.Remove(&m.timers, timer.index)
	}

	delete(m.timerMap, id)

	if m.config.EnableMetrics {
		atomic.AddInt64(&m.metrics.totalCanceled, 1)
		atomic.AddInt64(&m.metrics.activeTimers, -1)
	}
}

// AddTimer 添加定时器
func (m *TimerMgr) AddTimer(name string, firstSeconds, intervalSeconds int64, tickCount int32, value any, runInGo bool, f TimerFunc) (int64, error) {
	if !m.IsRunning() {
		return 0, ErrTimerManagerStopped
	}

	// 参数验证
	if intervalSeconds <= 0 {
		return 0, ErrInvalidInterval
	}

	if tickCount < 0 {
		return 0, ErrInvalidTickCount
	}

	if f == nil {
		return 0, errors.New("timer function cannot be nil")
	}

	// 使用自增ID替代fastid
	id := atomic.AddInt64(&m.TimerId, 1)
	timer := &CircTimer{
		Name:         name,
		Id:           id,
		MaxTickCount: tickCount,
		TickCount:    0,
		Interval:     intervalSeconds,
		Value:        value,
		Func:         f,
		RunInGo:      runInGo,
	}
	timer.NextTimeMs = xtime.NowUnixMs() + firstSeconds*millisPerSecond

	select {
	case m.addChan <- timer:
		if m.config.EnableMetrics {
			atomic.AddInt64(&m.metrics.totalAdded, 1)
		}
		return id, nil
	case <-m.ctx.Done():
		return 0, ErrTimerManagerStopped
	default:
		return 0, errors.New("add timer channel is full")
	}
}

// AddSimpleTimer 添加简单定时器
func (m *TimerMgr) AddSimpleTimer(name string, intervalSeconds int64, runInGo bool, f TimerFunc) (int64, error) {
	return m.AddTimer(name, intervalSeconds, intervalSeconds, 0, nil, runInGo, f)
}

func (m *TimerMgr) AddOneShotTimer(name string, intervalSeconds int64, runInGo bool, f TimerFunc) (int64, error) {
	return m.AddTimer(name, intervalSeconds, intervalSeconds, 1, nil, runInGo, f)
}

// CancelTimer 取消定时器
func (m *TimerMgr) CancelTimer(id int64) error {
	if !m.IsRunning() {
		return ErrTimerManagerStopped
	}

	select {
	case m.cancelChan <- id:
		return nil
	case <-m.ctx.Done():
		return ErrTimerManagerStopped
	default:
		return errors.New("cancel timer channel is full")
	}
}

// GetMetrics 获取指标统计
func (m *TimerMgr) GetMetrics() map[string]int64 {
	if !m.config.EnableMetrics {
		return nil
	}

	return map[string]int64{
		"total_added":    atomic.LoadInt64(&m.metrics.totalAdded),
		"total_canceled": atomic.LoadInt64(&m.metrics.totalCanceled),
		"total_executed": atomic.LoadInt64(&m.metrics.totalExecuted),
		"active_timers":  atomic.LoadInt64(&m.metrics.activeTimers),
	}
}

// GetActiveTimerCount 获取活跃定时器数量
func (m *TimerMgr) GetActiveTimerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.timerMap)
}

// GetTimerInfo 获取定时器信息
func (m *TimerMgr) GetTimerInfo(id int64) (*CircTimer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	timer, exists := m.timerMap[id]
	if !exists {
		return nil, ErrTimerNotFound
	}

	// 返回副本以避免并发修改
	return &CircTimer{
		Name:         timer.Name,
		Id:           timer.Id,
		MaxTickCount: timer.MaxTickCount,
		Interval:     timer.Interval,
		NextTimeMs:   timer.NextTimeMs,
		TickCount:    timer.TickCount,
		RunInGo:      timer.RunInGo,
	}, nil
}

// String 返回管理器状态的字符串表示
func (m *TimerMgr) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return fmt.Sprintf("TimerMgr{running: %v, active_timers: %d, heap_size: %d}",
		m.IsRunning(), len(m.timerMap), m.timers.Len())
}
func (m *TimerMgr) detectTimeJump(nowMs int64) {
	if m.lastProcessTime != 0 && xlog.GetLogLevel() != xlog.LOG_LEVEL_DEBUG {
		deltaMs := nowMs - m.lastProcessTime
		thresholdMs := catchUpThresholdSeconds * millisPerSecond
		if deltaMs > thresholdMs {
			xlog.Warnf("timer manager detected forward time jump: delta=%dms prev=%d now=%d", deltaMs, m.lastProcessTime, nowMs)
		} else if deltaMs < -thresholdMs {
			xlog.Warnf("timer manager detected backward time jump: delta=%dms prev=%d now=%d", deltaMs, m.lastProcessTime, nowMs)
		}
	}
	m.lastProcessTime = nowMs
}
