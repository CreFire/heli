package async

import (
	"fmt"
	"game/deps/basal"
	"game/deps/xlog"
	"game/deps/xtime"
	"sync"
	"sync/atomic"
	"time"
)

type task struct {
	start   int64
	tagInfo string
	fun     func()
}

//go:norace
func (t *task) SetStart(unixMill int64) {
	t.start = unixMill
}

type Async struct {
	size  int32
	index int64
	ch    []chan *task
	mon   []basal.NoCacheLineData[*task]
	// monitor 用于监控异步任务的执行情况
	quit chan struct{}
	wg   *sync.WaitGroup
}

func NewAsync(size int32, queueSize int32) (*Async, error) {
	if size <= 64 {
		size = 64
	}

	prime := basal.NextPrime(size)
	if prime == 0 {
		return nil, fmt.Errorf("async size is not in prime array, size: %d", size)
	}
	size = prime

	if queueSize <= 0 {
		queueSize = 8 * 1024
	}

	size = min(1024*16, size)
	queueSize = min(queueSize, 1024*128)

	ch := make([]chan *task, size)
	mon := make([]basal.NoCacheLineData[*task], size)
	for i := 0; i < int(size); i++ {
		ch[i] = make(chan *task, queueSize)
		mon[i] = *basal.NewNoCacheLineData[*task](nil)
	}

	xlog.Infof("async pool init with size: %d", size)
	return &Async{
		size: size,
		ch:   ch,
		mon:  mon,
		quit: make(chan struct{}),
		wg:   &sync.WaitGroup{},
	}, nil
}

func (a *Async) Start() {
	// 启动工作协程处理任务
	for i := int32(0); i < a.size; i++ {
		a.wg.Add(1)
		go a.startWorker(i)
	}

	// 启动监控协程
	a.wg.Add(1)
	go a.startMonitor()
}

// startWorker 启动指定索引的工作协程
func (a *Async) startWorker(index int32) {
	defer func() {
		if r := recover(); r != nil {
			xlog.Errorf("async worker panic: %v", r)
		}
	}()

	defer a.wg.Done()

	for {
		select {
		case t := <-a.ch[index]:
			a.executeTask(index, t)
		case <-a.quit:
			// 处理剩余任务
			a.processRemainingTasks(index)
			return
		}
	}
}

// executeTask 执行单个任务
func (a *Async) executeTask(i int32, t *task) {
	t.SetStart(xtime.NowUnixMs())
	a.mon[i].UpdatePtr(t)

	basal.SafeRun(t.fun)

	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		cost := xtime.NowUnixMs() - t.start
		clen := len(a.ch[i])
		funName := basal.GetFuncFullName(t.fun)
		xlog.Debugf("async task done,chan index: %d tag: [%s], func: %s, cost: %d ms , channel len: %d", i, t.tagInfo, funName, cost, clen)
	}

	a.mon[i].UpdatePtr(nil)
}

// processRemainingTasks 处理退出时的剩余任务
func (a *Async) processRemainingTasks(workerIndex int32) {
	remainingCount := len(a.ch[workerIndex])
	for range remainingCount {
		t := <-a.ch[workerIndex]
		basal.SafeRun(t.fun)
	}
}

const longRunningThreshold = 30 * 1000 // 30秒
// startMonitor 启动监控协程，定期检查任务执行情况
func (a *Async) startMonitor() {
	defer func() {
		if r := recover(); r != nil {
			xlog.Errorf("async monitor panic: %v", r)
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer func() {
		a.wg.Done()
		ticker.Stop()
	}()

	for {
		select {
		case <-a.quit:
			return
		case <-ticker.C:
			for i := int32(0); i < a.size; i++ {
				m := a.mon[i].Get()
				if m == nil || m.start == 0 {
					continue
				}

				if xtime.NowUnixMs()-m.start > longRunningThreshold {
					funName := basal.GetFuncFullName(m.fun)
					xlog.Warnf("async monitor %d, run time: %d, func: %s name: %s", i, xtime.NowUnixMs()-m.start, funName, m.tagInfo)
				}
			}
		}
	}
}

func (a *Async) Stop() {
	close(a.quit)
	a.wg.Wait()
}

func (a *Async) post(i uint64, tag string, f func()) error {
	select {
	case a.ch[i] <- &task{fun: f, tagInfo: tag}:
		return nil
	default:
		funName := ""
		if m := a.mon[i].Get(); m != nil && m.fun != nil {
			funName = basal.GetFuncFullName(m.fun)
		}
		xlog.Errorf("async post timeout, func: %v, task queue: %d is running: %s", basal.GetFuncFullName(f), i, funName)
		return fmt.Errorf("task queue is full and post timeout")
	}
}

func (a *Async) Post(tag string, f func()) error {
	if f == nil {
		return fmt.Errorf("post task is nil")
	}
	i := atomic.AddInt64(&a.index, 1) % int64(a.size)
	return a.post(uint64(i), tag, f)
}

func (a *Async) PostFixed(key uint64, tag string, f func()) error {
	if f == nil {
		return fmt.Errorf("post task is nil")
	}
	hash := basal.SimpleHash(key)
	i := hash % uint64(a.size)
	return a.post(i, tag, f)
}

func (a *Async) PostFixedOrRand(key uint64, tag string, f func()) error {
	if f == nil {
		return fmt.Errorf("post task is nil")
	}

	if key == 0 {
		return a.Post(tag, f)
	}

	return a.PostFixed(key, tag, f)
}

func (a *Async) PostFixedByString(key string, f func()) error {
	if f == nil {
		return fmt.Errorf("post task is nil")
	}
	hash := basal.SimpleStrHash(key)
	i := hash % uint64(a.size)
	return a.post(i, key, f)
}
