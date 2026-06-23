package controller

import (
	"fmt"
	"game/deps/xlog"
	"sync"
	"time"
)

const robotLogLimitWindow = 10 * time.Second

type limitedLogState struct {
	nextAt     time.Time
	suppressed int64
}

var robotLimitedLogs sync.Map
var limitedLogMu sync.Mutex

func WarnfLimited(key string, format string, args ...any) {
	logfLimited("warn", key, format, args...)
}

func ErrorfLimited(key string, format string, args ...any) {
	logfLimited("error", key, format, args...)
}
func logfLimited(level string, key string, format string, args ...any) {
	now := time.Now()
	raw, _ := robotLimitedLogs.LoadOrStore(key, &limitedLogState{})
	state := raw.(*limitedLogState)

	var msg string

	limitedLogMu.Lock()
	defer limitedLogMu.Unlock()
	if now.Before(state.nextAt) {
		state.suppressed++
		return
	}
	msg = fmt.Sprintf(format, args...)
	if state.suppressed > 0 {
		msg = fmt.Sprintf("%s suppressed=%d window=%s", msg, state.suppressed, robotLogLimitWindow)
		state.suppressed = 0
	}
	state.nextAt = now.Add(robotLogLimitWindow)
	switch level {
	case "error":
		xlog.Errorf("%s", msg)
	default:
		xlog.Warnf("%s", msg)
	}
}
