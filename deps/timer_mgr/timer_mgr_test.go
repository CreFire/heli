package timermgr

import (
	"container/heap"
	"fmt"
	"game/deps/xlog"
	"game/deps/xtime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func swapTestLogger(t *testing.T, level string) string {
	t.Helper()

	logPath := filepath.Join(t.TempDir(), "timer")
	xlog.InitDefaultLoggerWithOptions(xlog.Options{
		FilePath: logPath,
		Level:    level,
		Skip:     1,
		FileOut:  true,
		StdOut:   false,
		Sync:     true,
	})
	t.Cleanup(func() {
		xlog.Close()
		xlog.InitDefaultLoggerWithOptions(xlog.Options{
			FilePath: "./logs/log",
			Level:    "debug",
			Skip:     2,
			FileOut:  false,
			StdOut:   true,
		})
	})
	return logPath
}

func TestImprovedTimerMgr_DoesNotCatchUpForSubIntervalDelay(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	var callTimes []int64
	timerFunc := func(name string, now int64, value any) {
		callTimes = append(callTimes, now)
	}

	timer := &CircTimer{
		Name:         "ms-precision",
		Id:           1,
		MaxTickCount: 0,
		Interval:     1,
		NextTimeMs:   20_924,
		Value:        nil,
		Func:         timerFunc,
		RunInGo:      false,
	}

	mgr.mu.Lock()
	heap.Push(&mgr.timers, timer)
	mgr.timerMap[timer.Id] = timer
	mgr.mu.Unlock()

	mgr.processTickers(21_024)

	if len(callTimes) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(callTimes))
	}
	if got := callTimes[0]; got != 20 {
		t.Fatalf("expected scheduled callback second 20, got %d", got)
	}
	if timer.NextTimeMs != 21_924 {
		t.Fatalf("expected next trigger at 21924ms, got %d", timer.NextTimeMs)
	}
}

// mirror test: keep catch-up warning text aligned with real behavior; catch-up does not force sync execution.
func TestImprovedTimerMgr_CatchUpLogDoesNotClaimForceSync(t *testing.T) {
	logPath := swapTestLogger(t, "WARN")

	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	timer := &CircTimer{
		Name:         "catch-up-log",
		Id:           1,
		MaxTickCount: 0,
		Interval:     1,
		NextTimeMs:   20_000,
		Func:         func(name string, now int64, value any) {},
		RunInGo:      true,
	}

	mgr.mu.Lock()
	heap.Push(&mgr.timers, timer)
	mgr.timerMap[timer.Id] = timer
	mgr.mu.Unlock()

	mgr.processTickers(21_001)

	if err := xlog.Sync(); err != nil {
		t.Fatalf("sync log failed: %v", err)
	}
	data, err := os.ReadFile(logPath + "." + time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	output := string(data)
	if !strings.Contains(output, "catch-up detected") {
		t.Fatalf("expected catch-up warning, got %q", output)
	}
	if strings.Contains(output, "forceSync") {
		t.Fatalf("catch-up warning should not claim forceSync, got %q", output)
	}
}

func TestImprovedTimerMgr_NewTimerMgr(t *testing.T) {
	mgr := NewTimerMgr()
	defer mgr.Stop()

	if mgr == nil {
		t.Fatal("NewTimerMgr 返回了 nil")
	}

	if mgr.config == nil {
		t.Error("配置未初始化")
	}

	if mgr.timers == nil {
		t.Error("定时器堆未初始化")
	}

	if mgr.timerMap == nil {
		t.Error("定时器映射未初始化")
	}

	if !mgr.IsRunning() {
		// 管理器创建后默认未启动
	}
}

func TestImprovedTimerMgr_StartStop(t *testing.T) {
	mgr := NewTimerMgr()

	// 测试启动
	err := mgr.Start()
	if err != nil {
		t.Fatalf("启动管理器失败: %v", err)
	}

	if !mgr.IsRunning() {
		t.Error("管理器应该处于运行状态")
	}

	// 测试重复启动
	err = mgr.Start()
	if err != nil {
		t.Error("重复启动不应该返回错误")
	}

	// 测试停止
	err = mgr.Stop()
	if err != nil {
		t.Fatalf("停止管理器失败: %v", err)
	}

	if mgr.IsRunning() {
		t.Error("管理器应该已停止")
	}

	// 测试重复停止
	err = mgr.Stop()
	if err != nil {
		t.Error("重复停止不应该返回错误")
	}
}

func TestImprovedTimerMgr_AddTimerValidation(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	timerFunc := func(name string, now int64, value any) {}

	// 测试无效间隔
	_, err := mgr.AddTimer("test", 1, 0, 1, nil, false, timerFunc)
	if err != ErrInvalidInterval {
		t.Errorf("期望 ErrInvalidInterval，得到 %v", err)
	}

	_, err = mgr.AddTimer("test", 1, -1, 1, nil, false, timerFunc)
	if err != ErrInvalidInterval {
		t.Errorf("期望 ErrInvalidInterval，得到 %v", err)
	}

	// 测试无效执行次数
	_, err = mgr.AddTimer("test", 1, 1, -1, nil, false, timerFunc)
	if err != ErrInvalidTickCount {
		t.Errorf("期望 ErrInvalidTickCount，得到 %v", err)
	}

	// 测试空回调函数
	_, err = mgr.AddTimer("test", 1, 1, 1, nil, false, nil)
	if err == nil {
		t.Error("空回调函数应该返回错误")
	}

	// 测试有效参数
	id, err := mgr.AddTimer("test", 1, 1, 1, nil, false, timerFunc)
	if err != nil {
		t.Errorf("有效参数不应该返回错误: %v", err)
	}

	if id <= 0 {
		t.Error("应该返回正数ID")
	}
}

func TestImprovedTimerMgr_AddTimerWhenStopped(t *testing.T) {
	mgr := NewTimerMgr()
	// 不启动管理器

	timerFunc := func(name string, now int64, value any) {}

	_, err := mgr.AddTimer("test", 1, 1, 1, nil, false, timerFunc)
	if err != ErrTimerManagerStopped {
		t.Errorf("期望 ErrTimerManagerStopped，得到 %v", err)
	}
}

func TestImprovedTimerMgr_TimerExecution(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	var callCount int32
	var lastValue any
	var lastName string
	var mu sync.Mutex

	timerFunc := func(name string, now int64, value any) {
		mu.Lock()
		defer mu.Unlock()
		atomic.AddInt32(&callCount, 1)
		lastValue = value
		lastName = name
	}

	testValue := "测试值"
	id, err := mgr.AddTimer("测试定时器", 0, 1, 2, testValue, false, timerFunc)
	if err != nil {
		t.Fatalf("添加定时器失败: %v", err)
	}

	// 等待执行
	time.Sleep(2500 * time.Millisecond)

	count := atomic.LoadInt32(&callCount)
	if count != 2 {
		t.Errorf("期望执行2次，实际执行%d次", count)
	}

	mu.Lock()
	if lastName != "测试定时器" {
		t.Errorf("期望名称'测试定时器'，得到'%s'", lastName)
	}

	if lastValue != testValue {
		t.Errorf("期望值'%v'，得到'%v'", testValue, lastValue)
	}
	mu.Unlock()

	// 验证定时器已自动清理
	time.Sleep(500 * time.Millisecond)
	_, err = mgr.GetTimerInfo(id)
	if err != ErrTimerNotFound {
		t.Error("定时器应该已被清理")
	}
}

func TestImprovedTimerMgr_InfiniteTimer(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	var callCount int32
	timerFunc := func(name string, now int64, value any) {
		atomic.AddInt32(&callCount, 1)
	}

	id, err := mgr.AddTimer("无限定时器", 0, 1, 0, nil, false, timerFunc)
	if err != nil {
		t.Fatalf("添加无限定时器失败: %v", err)
	}

	// 等待几次执行
	time.Sleep(3 * time.Second)

	count1 := atomic.LoadInt32(&callCount)
	if count1 < 2 {
		t.Error("无限定时器应该执行多次")
	}

	// 取消定时器
	err = mgr.CancelTimer(id)
	if err != nil {
		t.Errorf("取消定时器失败: %v", err)
	}

	time.Sleep(2 * time.Second)
	count2 := atomic.LoadInt32(&callCount)

	// 取消后不应该继续执行
	if count2 > count1+1 {
		t.Error("定时器取消后不应该继续执行")
	}
}

func TestImprovedTimerMgr_CancelTimer(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	var callCount int32
	timerFunc := func(name string, now int64, value any) {
		atomic.AddInt32(&callCount, 1)
	}

	id, err := mgr.AddTimer("取消测试", 1, 1, 0, nil, false, timerFunc)
	if err != nil {
		t.Fatalf("添加定时器失败: %v", err)
	}

	// 等待定时器进入管理器，避免 AddTimer 的异步入队与 CancelTimer 竞争。
	time.Sleep(100 * time.Millisecond)

	// 立即取消
	err = mgr.CancelTimer(id)
	if err != nil {
		t.Errorf("取消定时器失败: %v", err)
	}

	// 等待确保不会执行
	time.Sleep(2 * time.Second)

	count := atomic.LoadInt32(&callCount)
	if count > 0 {
		t.Errorf("已取消的定时器不应该执行，但执行了%d次", count)
	}

	// 验证定时器信息已清理
	_, err = mgr.GetTimerInfo(id)
	if err != ErrTimerNotFound {
		t.Error("已取消的定时器信息应该被清理")
	}
}

func TestImprovedTimerMgr_RunInGo(t *testing.T) {
	config := DefaultConfig()
	config.TickInterval = 300 * time.Millisecond
	mgr := NewTimerMgrWithConfig(config)
	mgr.Start()
	defer mgr.Stop()

	var callCount int32
	var executionDurations []time.Duration
	var mu sync.Mutex

	timerFunc := func(name string, now int64, value any) {
		start := time.Now()
		fmt.Printf("execute start %s at %d", name, time.Now().UnixMilli())
		atomic.AddInt32(&callCount, 1)

		// 模拟长时间运行的任务
		time.Sleep(500 * time.Millisecond)

		mu.Lock()
		executionDurations = append(executionDurations, time.Since(start))
		mu.Unlock()
		t.Logf("execute %s at %d, %d", name, time.Now().UnixMilli(), time.Since(start).Milliseconds())
	}
	t.Logf("execute add at %d", time.Now().UnixMilli())
	// 添加两个定时器，一个在goroutine中运行，一个不在
	_, err := mgr.AddTimer("同步定时器", 0, 1, 1, nil, false, timerFunc)
	if err != nil {
		t.Fatalf("添加同步定时器失败: %v", err)
	}

	_, err = mgr.AddTimer("异步定时器", 0, 1, 1, nil, true, timerFunc)
	if err != nil {
		t.Fatalf("添加异步定时器失败: %v", err)
	}

	// 等待执行完成
	time.Sleep(2 * time.Second)
	t.Logf("execute end at %d", time.Now().UnixMilli())
	count := atomic.LoadInt32(&callCount)
	if count != 2 {
		t.Errorf("期望执行2次，实际执行%d次", count)
	}

	mu.Lock()
	if len(executionDurations) != 2 {
		t.Errorf("期望记录2个执行时长，实际记录%d个", len(executionDurations))
	}
	mu.Unlock()
}

func TestImprovedTimerMgr_GetTimerInfo(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	timerFunc := func(name string, now int64, value any) {}

	id, err := mgr.AddTimer("信息测试", 5, 10, 3, "测试数据", true, timerFunc)
	if err != nil {
		t.Fatalf("添加定时器失败: %v", err)
	}

	// 等待定时器被添加到内部结构
	time.Sleep(100 * time.Millisecond)

	info, err := mgr.GetTimerInfo(id)
	if err != nil {
		t.Fatalf("获取定时器信息失败: %v", err)
	}

	if info.Name != "信息测试" {
		t.Errorf("期望名称'信息测试'，得到'%s'", info.Name)
	}

	if info.Id != id {
		t.Errorf("期望ID %d，得到 %d", id, info.Id)
	}

	if info.MaxTickCount != 3 {
		t.Errorf("期望最大执行次数3，得到%d", info.MaxTickCount)
	}

	if info.Interval != 10 {
		t.Errorf("期望间隔10，得到%d", info.Interval)
	}

	if !info.RunInGo {
		t.Error("期望RunInGo为true")
	}

	// 测试不存在的定时器
	_, err = mgr.GetTimerInfo(99999)
	if err != ErrTimerNotFound {
		t.Errorf("期望 ErrTimerNotFound，得到 %v", err)
	}
}

func TestImprovedTimerMgr_GetActiveTimerCount(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	timerFunc := func(name string, now int64, value any) {}

	// 初始应该为0
	count := mgr.GetActiveTimerCount()
	if count != 0 {
		t.Errorf("初始活跃定时器数量应该为0，得到%d", count)
	}

	// 添加几个定时器
	id1, _ := mgr.AddTimer("定时器1", 10, 10, 1, nil, false, timerFunc)
	id2, _ := mgr.AddTimer("定时器2", 10, 10, 1, nil, false, timerFunc)
	id3, _ := mgr.AddTimer("定时器3", 10, 10, 1, nil, false, timerFunc)

	// 等待添加完成
	time.Sleep(100 * time.Millisecond)

	count = mgr.GetActiveTimerCount()
	if count != 3 {
		t.Errorf("期望活跃定时器数量为3，得到%d", count)
	}

	// 取消一个定时器
	mgr.CancelTimer(id2)
	time.Sleep(100 * time.Millisecond)

	count = mgr.GetActiveTimerCount()
	if count != 2 {
		t.Errorf("取消后期望活跃定时器数量为2，得到%d", count)
	}

	// 取消剩余定时器
	mgr.CancelTimer(id1)
	mgr.CancelTimer(id3)
	time.Sleep(100 * time.Millisecond)

	count = mgr.GetActiveTimerCount()
	if count != 0 {
		t.Errorf("全部取消后期望活跃定时器数量为0，得到%d", count)
	}
}

func TestImprovedTimerMgr_Metrics(t *testing.T) {
	config := DefaultConfig()
	config.EnableMetrics = true
	config.TickInterval = 300 * time.Millisecond

	mgr := NewTimerMgrWithConfig(config)
	mgr.Start()
	defer mgr.Stop()

	var callCount int32
	timerFunc := func(name string, now int64, value any) {
		atomic.AddInt32(&callCount, 1)
	}

	// 添加定时器
	id1, _ := mgr.AddTimer("指标测试1", 0, 1, 10, nil, false, timerFunc)
	mgr.AddTimer("指标测试2", 0, 1, 1, nil, false, timerFunc)

	// 等待执行
	time.Sleep(1500 * time.Millisecond)

	// 取消一个定时器
	mgr.CancelTimer(id1)
	time.Sleep(300 * time.Millisecond)

	metrics := mgr.GetMetrics()
	if metrics == nil {
		t.Fatal("指标应该不为nil")
	}

	if metrics["total_added"] != 2 {
		t.Errorf("期望添加2个定时器，指标显示%d", metrics["total_added"])
	}

	if metrics["total_canceled"] != 1 {
		t.Errorf("期望取消1个定时器，指标显示%d", metrics["total_canceled"])
	}

	if metrics["total_executed"] < 2 {
		t.Errorf("期望至少执行2次，指标显示%d", metrics["total_executed"])
	}
}

func TestImprovedTimerMgr_ConcurrentOperations(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	var totalCalls int32
	timerFunc := func(name string, now int64, value any) {
		atomic.AddInt32(&totalCalls, 1)
	}

	// 并发添加定时器
	var wg sync.WaitGroup
	numTimers := 50
	timerIds := make([]int64, numTimers)

	for i := range numTimers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			id, err := mgr.AddTimer("并发测试", 0, 1, 1, index, false, timerFunc)
			if err != nil {
				t.Errorf("并发添加定时器失败: %v", err)
				return
			}
			timerIds[index] = id
		}(i)
	}

	wg.Wait()

	// 等待执行
	time.Sleep(2 * time.Second)

	calls := atomic.LoadInt32(&totalCalls)
	if calls != int32(numTimers) {
		t.Errorf("期望执行%d次，实际执行%d次", numTimers, calls)
	}

	// 并发取消定时器
	for i := 0; i < numTimers/2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			err := mgr.CancelTimer(timerIds[index])
			if err != nil {
				t.Errorf("并发取消定时器失败: %v", err)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// 验证活跃定时器数量
	activeCount := mgr.GetActiveTimerCount()
	if activeCount != 0 { // 所有定时器都应该已执行完毕或被取消
		t.Logf("剩余活跃定时器: %d", activeCount)
	}
}

func TestImprovedTimerMgr_String(t *testing.T) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	str := mgr.String()
	if str == "" {
		t.Error("String()方法不应该返回空字符串")
	}

	t.Logf("TimerMgr状态: %s", str)
}

func TestTimerMgrDetectTimeJump(t *testing.T) {
	mgr := NewTimerMgr()
	defer mgr.Stop()

	baseMs := int64(1_000)
	mgr.lastProcessTime = baseMs

	forwardMs := baseMs + catchUpThresholdSeconds*millisPerSecond + 1
	mgr.detectTimeJump(forwardMs)
	if mgr.lastProcessTime != forwardMs {
		t.Fatalf("expected lastProcessTime updated to %d, got %d", forwardMs, mgr.lastProcessTime)
	}

	backwardMs := baseMs - catchUpThresholdSeconds*millisPerSecond - 1
	mgr.detectTimeJump(backwardMs)
	if mgr.lastProcessTime != backwardMs {
		t.Fatalf("expected lastProcessTime updated to %d, got %d", backwardMs, mgr.lastProcessTime)
	}

	mgr.lastProcessTime = baseMs
	smallForwardMs := baseMs + catchUpThresholdSeconds*millisPerSecond - 1
	mgr.detectTimeJump(smallForwardMs)
	if mgr.lastProcessTime != smallForwardMs {
		t.Fatalf("expected lastProcessTime updated to %d, got %d", smallForwardMs, mgr.lastProcessTime)
	}

	smallBackwardMs := baseMs - catchUpThresholdSeconds*millisPerSecond + 1
	mgr.detectTimeJump(smallBackwardMs)
	if mgr.lastProcessTime != smallBackwardMs {
		t.Fatalf("expected lastProcessTime updated to %d, got %d", smallBackwardMs, mgr.lastProcessTime)
	}
}

// mirror test: keep detectTimeJump threshold aligned with millisecond scheduling.
func TestImprovedTimerMgr_DetectTimeJumpUsesMillisecondThreshold(t *testing.T) {
	logPath := swapTestLogger(t, "WARN")

	mgr := NewTimerMgr()
	defer mgr.Stop()

	mgr.lastProcessTime = 1_000
	mgr.detectTimeJump(30_999)

	if err := xlog.Sync(); err != nil {
		t.Fatalf("sync log failed: %v", err)
	}
	data, err := os.ReadFile(logPath + "." + time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	if strings.Contains(string(data), "time jump") {
		t.Fatalf("did not expect warning below threshold, got %q", string(data))
	}

	mgr.lastProcessTime = 1_000
	mgr.detectTimeJump(31_001)

	if err := xlog.Sync(); err != nil {
		t.Fatalf("sync log failed: %v", err)
	}
	data, err = os.ReadFile(logPath + "." + time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	if !strings.Contains(string(data), "forward time jump") {
		t.Fatalf("expected forward jump warning, got %q", string(data))
	}
}

func TestTimerMgrServerTimeJumpWithXtime(t *testing.T) {
	originalGmAdd := xtime.NowUnix() - time.Now().Unix()
	defer func() {
		xtime.SetGmAdd(originalGmAdd)
	}()

	mgr := NewTimerMgr()
	defer mgr.Stop()

	baseMs := xtime.NowUnixMs()
	mgr.detectTimeJump(baseMs)
	if mgr.lastProcessTime != baseMs {
		t.Fatalf("expected base process time %d, got %d", baseMs, mgr.lastProcessTime)
	}

	xtime.SetGmAdd(originalGmAdd + 100)
	forwardMs := xtime.NowUnixMs()
	mgr.detectTimeJump(forwardMs)
	if mgr.lastProcessTime != forwardMs {
		t.Fatalf("expected forward process time %d, got %d", forwardMs, mgr.lastProcessTime)
	}

	xtime.SetGmAdd(originalGmAdd - 50)
	backwardMs := xtime.NowUnixMs()
	mgr.detectTimeJump(backwardMs)
	if mgr.lastProcessTime != backwardMs {
		t.Fatalf("expected backward process time %d, got %d", backwardMs, mgr.lastProcessTime)
	}
}

// 基准测试
func BenchmarkImprovedTimerMgr_AddTimer(b *testing.B) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	timerFunc := func(name string, now int64, value any) {}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mgr.AddTimer("基准测试", 1, 1, 1, nil, false, timerFunc)
		}
	})
}

func BenchmarkImprovedTimerMgr_CancelTimer(b *testing.B) {
	mgr := NewTimerMgr()
	mgr.Start()
	defer mgr.Stop()

	timerFunc := func(name string, now int64, value any) {}

	// 预先添加定时器
	ids := make([]int64, b.N)
	for i := 0; i < b.N; i++ {
		id, _ := mgr.AddTimer("基准测试", 10, 10, 1, nil, false, timerFunc)
		ids[i] = id
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.CancelTimer(ids[i])
	}
}
