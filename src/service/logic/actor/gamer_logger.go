package actor

import (
	"game/deps/xlog"

	"go.uber.org/zap"
)

var gamerLogger = xlog.ZapLogger(xlog.DefaultLogger, zap.DebugLevel, 1).Sugar()

func InitGamerLogger(logger *xlog.MyLogger) {
	gamerLogger = xlog.ZapLogger(logger, zap.DebugLevel, 1).Sugar()
}

func (r *Gamer) Info(format string, args ...any) {
	gamerLogger.Infof(prefixFormat(r.GamerId, format), prefixArgs(r.GamerId, args)...)
}

func (r *Gamer) Warn(format string, args ...any) {
	gamerLogger.Warnf(prefixFormat(r.GamerId, format), prefixArgs(r.GamerId, args)...)
}

func (r *Gamer) Error(format string, args ...any) {
	gamerLogger.Errorf(prefixFormat(r.GamerId, format), prefixArgs(r.GamerId, args)...)
}

func (r *Gamer) Debug(format string, args ...any) {
	if xlog.GetLogLevel() > xlog.LOG_LEVEL_DEBUG {
		return
	}
	gamerLogger.Debugf(prefixFormat(r.GamerId, format), prefixArgs(r.GamerId, args)...)
}

func prefixFormat(_ int64, format string) string {
	return "[gid=%d] " + format
}

func prefixArgs(gid int64, args []any) []any {
	merged := make([]any, 0, len(args)+1)
	merged = append(merged, gid)
	merged = append(merged, args...)
	return merged
}
