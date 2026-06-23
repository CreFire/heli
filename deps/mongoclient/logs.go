package mongoclient

import "game/deps/xlog"

// logAdapter 实现了 go.mongodb.org/mongo-driver/v2/mongo/options.LogSink 接口
type logAdapter struct {
	logger *xlog.MyLogger
}

// New 创建一个新的适配器实例
func newMongoLogAdapter(logger *xlog.MyLogger) *logAdapter {
	return &logAdapter{logger: logger}
}

// Info 处理非错误日志，将 MongoDB Driver 的 verbosity level 转换为 logrus level。
func (log *logAdapter) Info(verbosity int, msg string, keysAndValues ...any) {
	if verbosity >= 1 {
		log.logger.Debugw(msg, keysAndValues...)
	}
}

func (log *logAdapter) Error(err error, msg string, keysAndValues ...any) {
	kvs := append(keysAndValues, "error", err)
	log.logger.Errorw(msg, kvs...)
}
