package timermgr

import (
	"fmt"
	"game/deps/misc"
	"game/deps/xlog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sasha-s/go-deadlock"
)

const (
	numTimersForBench = 100000
	future            = int64(3600) // 1 hour in the future
	defaultBenchBuf   = 16 * 128 * 128
)

// noOpTimerFunc is a timer function that does nothing, for benchmarking.
func noOpTimerFunc(name string, now int64, value any) {}

func prepareBenchmarkEnv(b *testing.B) {
	b.Helper()

	oldDisable := deadlock.Opts.Disable
	oldDisableLockOrderDetection := deadlock.Opts.DisableLockOrderDetection
	oldLevel := xlog.GetLogLevel()

	deadlock.Opts.Disable = true
	deadlock.Opts.DisableLockOrderDetection = true
	xlog.SetLogLevel("WARN")

	b.Cleanup(func() {
		deadlock.Opts.Disable = oldDisable
		deadlock.Opts.DisableLockOrderDetection = oldDisableLockOrderDetection
		switch oldLevel {
		case xlog.LOG_LEVEL_DEBUG:
			xlog.SetLogLevel("DEBUG")
		case xlog.LOG_LEVEL_INFO:
			xlog.SetLogLevel("INFO")
		case xlog.LOG_LEVEL_WARN:
			xlog.SetLogLevel("WARN")
		case xlog.LOG_LEVEL_ERROR:
			xlog.SetLogLevel("ERROR")
		default:
			xlog.SetLogLevel("INFO")
		}
	})
}

// setupMgrWithTimers creates a new TimerMgr, starts it, and populates it with a given number of timers.
// These timers are set to fire far in the future so they don't interfere with the benchmarked operation.
func setupMgrWithTimers(b *testing.B, numTimers, channelBuffer int) *TimerMgr {
	b.Helper()
	prepareBenchmarkEnv(b)
	if channelBuffer < defaultBenchBuf {
		channelBuffer = defaultBenchBuf
	}

	cfg := DefaultConfig()
	cfg.TickInterval = 100 * time.Millisecond
	cfg.ChannelBuffer = channelBuffer
	mgr := NewTimerMgrWithConfig(cfg)
	mgr.Start()
	b.Cleanup(func() {
		mgr.Stop()
	})
	// Pre-populate with timers
	for i := range numTimers {
		_, err := mgr.AddTimer("pre-existing"+misc.IntToStr(i), future, future, 1, nil, false, noOpTimerFunc)
		if err != nil {
			b.Fatalf("Failed to add pre-existing timer: %v", err)
		}
	}

	// Wait for all timers to be added, since AddTimer is asynchronous.
	for mgr.GetActiveTimerCount() < numTimers {
		time.Sleep(10 * time.Millisecond)
	}
	return mgr
}

// setupBenchMgrWithTimers creates a new TimerMgr for benchmarking without starting it.
// This is used for internal method benchmarks that don't need the full goroutine machinery.
func setupBenchMgrWithTimers(b *testing.B, numTimers int) *TimerMgr {
	b.Helper()
	prepareBenchmarkEnv(b)
	cfg := DefaultConfig()
	cfg.ChannelBuffer = defaultBenchBuf
	mgr := NewTimerMgrWithConfig(cfg)

	// Manually add timers to simulate a populated state for internal method benchmarking
	for i := range numTimers {
		timer := &CircTimer{
			Name:         "pre-existing" + misc.IntToStr(i),
			Id:           int64(i + 1),
			MaxTickCount: 1,
			Interval:     future,
			NextTimeMs:   future * millisPerSecond,
			Func:         noOpTimerFunc,
		}
		mgr.addTimerInternal(timer)
	}

	return mgr
}

// BenchmarkAddTimer measures the performance of adding a single timer to an empty manager.
func BenchmarkAddTimer(b *testing.B) {
	mgr := setupMgrWithTimers(b, 0, b.N+1024)

	b.ResetTimer()
	b.ReportAllocs()

	var benchErr error
	var once sync.Once
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := mgr.AddTimer("benchmark-add", future, future, 1, nil, false, noOpTimerFunc); err != nil {
				once.Do(func() {
					benchErr = err
				})
				return
			}
		}
	})

	if benchErr != nil {
		b.Fatalf("Failed to add timer: %v", benchErr)
	}
}

// BenchmarkAddTimerWith100kExisting measures the performance of adding a timer
// when 100,000 timers already exist. This tests the O(log N) complexity of heap insertion.
func BenchmarkAddTimerWith100kExisting(b *testing.B) {
	mgr := setupMgrWithTimers(b, numTimersForBench, numTimersForBench+b.N+1024)

	b.ResetTimer()
	b.ReportAllocs()

	var benchErr error
	var once sync.Once
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := mgr.AddTimer("benchmark-add", future, future, 1, nil, false, noOpTimerFunc); err != nil {
				once.Do(func() {
					benchErr = err
				})
				return
			}
		}
	})

	if benchErr != nil {
		b.Fatalf("Failed to add timer with existing heap: %v", benchErr)
	}
}

// BenchmarkCancelTimerWith100kExisting measures the performance of canceling a timer
// when 100,000 timers already exist. This measures the internal O(1) map lookup
// and O(log N) heap removal without channel/setup noise.
func BenchmarkCancelTimerWith100kExisting(b *testing.B) {
	mgr := setupBenchMgrWithTimers(b, numTimersForBench)

	// Add b.N timers to be canceled during the benchmark.
	idsToCancel := make([]int64, b.N)
	for i := 0; i < b.N; i++ {
		id := int64(numTimersForBench + i + 1)
		mgr.addTimerInternal(&CircTimer{
			Name:         fmt.Sprintf("to-cancel-%d", i),
			Id:           id,
			MaxTickCount: 1,
			Interval:     future,
			NextTimeMs:   future * millisPerSecond,
			Func:         noOpTimerFunc,
		})
		idsToCancel[i] = id
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mgr.cancelTimerInternal(idsToCancel[i])
	}
}

// BenchmarkAddTimerInternal measures the performance of the internal add timer algorithm
// when 100,000 timers already exist. This tests only the internal algorithm without
// the channel communication overhead.
func BenchmarkAddTimerInternal(b *testing.B) {
	// Create a manager with existing timers but don't start the goroutines
	mgr := setupBenchMgrWithTimers(b, numTimersForBench)

	// Pre-create timers to add during benchmark
	timersToAdd := make([]*CircTimer, b.N)
	for i := 0; i < b.N; i++ {
		timersToAdd[i] = &CircTimer{
			Name:         fmt.Sprintf("benchmark-add-%d", i),
			Id:           int64(numTimersForBench + i + 1),
			MaxTickCount: 1,
			Interval:     future,
			NextTimeMs:   future * millisPerSecond,
			Func:         noOpTimerFunc,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mgr.addTimerInternal(timersToAdd[i])
	}
}

// BenchmarkAddTimerInternalParallel measures the performance of the internal add timer algorithm
// with parallelism when 100,000 timers already exist.
func BenchmarkAddTimerInternalParallel(b *testing.B) {
	// Create a manager with existing timers but don't start the goroutines
	mgr := setupBenchMgrWithTimers(b, numTimersForBench)

	b.ResetTimer()
	b.ReportAllocs()

	// Use atomic to generate unique IDs for each operation
	var nextId int64 = numTimersForBench + 1

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Create a unique timer for each operation
			id := atomic.AddInt64(&nextId, 1)
			timer := &CircTimer{
				Name:         "benchmark-add",
				Id:           id,
				MaxTickCount: 1,
				Interval:     future,
				NextTimeMs:   future * millisPerSecond,
				Func:         noOpTimerFunc,
			}

			mgr.addTimerInternal(timer)
		}
	})
}
