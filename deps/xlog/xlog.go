package xlog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func logLevelResolve(level string) (zap.AtomicLevel, zapcore.Level) {
	level = strings.ToUpper(level)
	atomicLevel := zap.NewAtomicLevel()
	lvl, ok := parseLogLevel(level)
	if !ok {
		lvl = zapcore.InfoLevel
	}
	atomicLevel.SetLevel(lvl)
	return atomicLevel, lvl
}

func parseLogLevel(level string) (zapcore.Level, bool) {
	switch level {
	case "DEBUG":
		return zapcore.DebugLevel, true
	case "INFO":
		return zapcore.InfoLevel, true
	case "WARN":
		return zapcore.WarnLevel, true
	case "ERROR":
		return zapcore.ErrorLevel, true
	case "DPANIC":
		return zapcore.DPanicLevel, true
	case "PANIC":
		return zapcore.PanicLevel, true
	case "FATAL":
		return zapcore.FatalLevel, true
	default:
		return 0, false
	}
}

var (
	DefaultLogger   *MyLogger
	defaultLoggerMu sync.Mutex
)

const (
	LOG_LEVEL_DEBUG = -1
	LOG_LEVEL_INFO  = iota
	LOG_LEVEL_WARN
	LOG_LEVEL_ERROR
	LOG_LEVEL_DPANIC
	LOG_LEVEL_PANIC
	LOG_LEVEL_FATAL
)

const (
	defaultFilePath      = "./logs/log"
	defaultLevel         = "info"
	defaultRotationDaily = "daily"
	rotationHourly       = "hourly"
	defaultRetentionDays = 14
	defaultFileSizeMB    = 10
	bytesPerMB           = 1024 * 1024
	layoutDaily          = "2006-01-02"
	layoutHourly         = "2006-01-02-15"
)

type Options struct {
	FilePath      string
	Level         string // debug, info, warn, error
	Rotation      string // daily or hourly, default daily
	MaxFileSizeMB int    // default 10
	RetentionDays int    // default 14
	Skip          int    // default 2
	Sync          bool   // default false， use buffered writer
	StdOut        bool
	FileOut       bool
}

func init() {
	if DefaultLogger == nil {
		InitDefaultLogger("./logs/log", "debug")
	}
}

// 适配到标准Log的logger, level为适配后的logger输出的日志等级
func StdLogger(logger *MyLogger, level zapcore.Level, skip int) *log.Logger {
	level = effectiveChildLevel(logger, level)
	stdlogger := zap.New(logger.zapcore, zap.AddCaller(), zap.AddCallerSkip(skip), zap.AddStacktrace(zap.ErrorLevel))
	log, err := zap.NewStdLogAt(stdlogger, level)
	if err != nil {
		panic(err)
	}
	return log
}

func ZapLogger(logger *MyLogger, level zapcore.Level, skip int) *zap.Logger {
	level = effectiveChildLevel(logger, level)
	stdlogger := zap.New(logger.zapcore,
		zap.AddCaller(),
		zap.AddCallerSkip(skip),
		zap.AddStacktrace(zap.ErrorLevel),
		zap.IncreaseLevel(level),
	)

	return stdlogger
}

func effectiveChildLevel(logger *MyLogger, level zapcore.Level) zapcore.Level {
	if logger == nil {
		return level
	}
	if level < logger.level {
		return logger.level
	}
	return level
}

func InitDefaultLogger(path string, level string) {
	InitDefaultLoggerWithOptions(Options{
		FilePath: path,
		Level:    level,
		Skip:     2,
		FileOut:  true,
	})
}

func InitDefaultLoggerWithOptions(opts Options) {
	if opts.Skip <= 0 {
		opts.Skip = 2
	}
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	if DefaultLogger != nil {
		DefaultLogger.Close()
	}
	DefaultLogger = NewMyLoggerWithOptions(opts)
}

func Debugw(msg string, keysAndValues ...any) {
	if GetLogLevel() > LOG_LEVEL_DEBUG {
		return
	}
	DefaultLogger.Debugw(msg, keysAndValues...)
}

func Debugf(format string, args ...any) {
	if GetLogLevel() > LOG_LEVEL_DEBUG {
		return
	}
	DefaultLogger.Debugf(format, args...)
}

func Infow(msg string, keysAndValues ...any) {
	DefaultLogger.Infow(msg, keysAndValues...)
}

func Infof(format string, args ...any) {
	DefaultLogger.Infof(format, args...)
}

func Warnw(msg string, keysAndValues ...any) {
	DefaultLogger.Warnw(msg, keysAndValues...)
}

func Warnf(format string, args ...any) {
	DefaultLogger.Warnf(format, args...)
}

func Errorw(msg string, keysAndValues ...any) {
	DefaultLogger.Errorw(msg, keysAndValues...)
	DefaultLogger.Sync()
}

func Errorf(format string, args ...any) {
	DefaultLogger.Errorf(format, args...)
	DefaultLogger.Sync()
}

func GetLogLevel() int32 {
	return DefaultLogger.GetLevel()
}

func SetLogLevel(level string) {
	DefaultLogger.SetLevel(level)
}

func Sync() error {
	return DefaultLogger.Sync()
}

func Close() {
	DefaultLogger.Infof("logger close!")
	DefaultLogger.Sync()
	DefaultLogger.Close()
}

type MyLogger struct {
	LogPath string
	Logger  *zap.SugaredLogger
	LogAl   zap.AtomicLevel
	level   zapcore.Level
	zapcore zapcore.Core
	closer  io.Closer
	buffer  *zapcore.BufferedWriteSyncer
}

func NewMyLogger(path string, level string, skipLevel int) *MyLogger {
	return NewMyLoggerWithOptions(Options{
		FilePath: path,
		Level:    level,
		Skip:     skipLevel,
	})
}

func NewMyLoggerWithOptions(opts Options) *MyLogger {
	if opts.Skip < 0 {
		opts.Skip = 0
	}
	opts.normalize()

	con := zap.NewProductionEncoderConfig()
	con.EncodeTime = CustomTimeEncoder
	con.EncodeCaller = CustomCallerEncoder
	con.EncodeLevel = zapcore.CapitalLevelEncoder
	con.CallerKey = "line"

	al, lvl := logLevelResolve(opts.Level)
	skip := zap.AddCallerSkip(opts.Skip)
	var (
		coreList []zapcore.Core
		closer   io.Closer
		buffer   *zapcore.BufferedWriteSyncer
	)

	if opts.FileOut {
		ensureLogDir(opts.FilePath)
		fileWriter := buildRotateWriter(opts)
		closer = fileWriter

		var ws zapcore.WriteSyncer = fileWriter
		if !opts.Sync {
			buffer = &zapcore.BufferedWriteSyncer{
				WS:            fileWriter,
				FlushInterval: time.Second,
				Size:          1024 * 1024,
			}
			ws = buffer
		}

		fileCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(con),
			ws,
			al,
		)
		coreList = append(coreList, fileCore)
	}

	con.EncodeLevel = zapcore.CapitalLevelEncoder
	stdoutCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(con),
		zapcore.Lock(os.Stdout),
		al,
	)

	if opts.StdOut {
		coreList = append(coreList, stdoutCore)
	}
	core := zapcore.NewTee(coreList...)

	log := zap.New(core, zap.AddCaller(), skip, zap.AddStacktrace(zap.ErrorLevel)).Sugar()

	return &MyLogger{
		LogPath: opts.FilePath,
		Logger:  log,
		LogAl:   al,
		level:   lvl,
		zapcore: core,
		closer:  closer,
		buffer:  buffer,
	}
}

func (l *MyLogger) Debugf(format string, args ...any) {
	l.Logger.Debugf(format, args...)
}

func (l *MyLogger) Infof(format string, args ...any) {
	l.Logger.Infof(format, args...)
}
func (l *MyLogger) Warnf(format string, args ...any) {
	l.Logger.Warnf(format, args...)
}

func (l *MyLogger) Errorf(format string, args ...any) {
	l.Logger.Errorf(format, args...)
}
func (l *MyLogger) Debugw(msg string, args ...any) {
	l.Logger.Debugw(msg, args...)
}

func (l *MyLogger) Infow(msg string, args ...any) {
	l.Logger.Infow(msg, args...)
}
func (l *MyLogger) Warnw(msg string, args ...any) {
	l.Logger.Warnw(msg, args...)
}

func (l *MyLogger) Errorw(msg string, args ...any) {
	l.Logger.Errorw(msg, args...)
}

//go:norace
func (l *MyLogger) GetLevel() int32 {
	return int32(l.level)
}

//go:norace
func (l *MyLogger) SetLevel(level string) {
	level = strings.ToUpper(level)
	lvl, ok := parseLogLevel(level)
	if !ok {
		l.Warnf("no log level %s", level)
		return
	}
	l.LogAl.SetLevel(lvl)
	l.level = lvl
}

func (l *MyLogger) Sync() error {
	return l.Logger.Sync()
}

func (l *MyLogger) Close() {
	l.Sync()
	if l.buffer != nil {
		_ = l.buffer.Stop()
	}
	if l.closer != nil {
		l.closer.Close()
	}
}

func ensureLogDir(path string) {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(fmt.Errorf("create log directory %s: %w", dir, err))
	}
}

func (o *Options) normalize() {
	o.FilePath = strings.TrimSpace(o.FilePath)
	if o.FilePath == "" {
		o.FilePath = defaultFilePath
	} else {
		o.FilePath = filepath.Clean(o.FilePath)
	}

	o.Level = resolveString(o.Level, defaultLevel)
	if rot := normalizeRotation(o.Rotation); rot != "" {
		o.Rotation = rot
	} else {
		o.Rotation = defaultRotationDaily
	}
	if o.MaxFileSizeMB <= 0 {
		o.MaxFileSizeMB = defaultFileSizeMB
	}
	if o.RetentionDays <= 0 {
		o.RetentionDays = defaultRetentionDays
	}

	if !o.FileOut && !o.StdOut {
		o.FileOut = false
		o.StdOut = true
	}
	// 默认日志配置
}

func resolveString(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}

func normalizeRotation(rotation string) string {
	switch strings.ToLower(strings.TrimSpace(rotation)) {
	case defaultRotationDaily, "day":
		return defaultRotationDaily
	case rotationHourly, "hour":
		return rotationHourly
	default:
		return ""
	}
}

func CustomTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}

func CustomCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(fmt.Sprintf("%40s", caller.TrimmedPath()))
}
