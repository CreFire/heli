package server

import (
	"game/deps/xlog"
	"runtime/debug"

	"github.com/sasha-s/go-deadlock"
	"go.uber.org/zap/zapcore"
)

func SetUpDeadlockChecker(enableLockCheck bool) {
	deadlock.Opts.Disable = !enableLockCheck
	deadlock.Opts.DisableLockOrderDetection = !enableLockCheck
	deadlock.Opts.LogBuf = xlog.StdLogger(xlog.DefaultLogger, zapcore.ErrorLevel, 0).Writer()
	// Deadlock checker is only enabled for debugging. Once triggered, fail fast so the crash reason is explicit.
	deadlock.Opts.OnPotentialDeadlock = func() {
		xlog.Errorf("deadlock checker triggered, process will panic intentionally for failfast, process may be crash, stack info: %s", string(debug.Stack()))
		panic("deadlock checker triggered: intentional failfast")
	}
}
