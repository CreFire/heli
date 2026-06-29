package xlog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sasha-s/go-deadlock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// newTestLogger creates a logger instance that writes to a buffer for testing purposes.
func newTestLogger(level string) (*MyLogger, *bytes.Buffer) {
	var buf bytes.Buffer
	con := zap.NewProductionEncoderConfig()
	con.EncodeTime = CustomTimeEncoder
	con.EncodeCaller = CustomCallerEncoder
	con.EncodeLevel = zapcore.CapitalLevelEncoder
	con.CallerKey = "line"

	al, _ := logLevelResolve(level)
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(con),
		zapcore.AddSync(&buf),
		al,
	)
	// skipLevel 0 for direct calls from test functions
	sugaredLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0)).Sugar()

	logger := &MyLogger{
		LogPath: "",
		Logger:  sugaredLogger,
		LogAl:   al,
		zapcore: core,
	}
	return logger, &buf
}

func TestLogLevelResolve(t *testing.T) {
	testCases := []struct {
		levelStr string
		expected zapcore.Level
	}{
		{"DEBUG", zapcore.DebugLevel},
		{"INFO", zapcore.InfoLevel},
		{"WARN", zapcore.WarnLevel},
		{"ERROR", zapcore.ErrorLevel},
		{"DPANIC", zapcore.DPanicLevel},
		{"PANIC", zapcore.PanicLevel},
		{"FATAL", zapcore.FatalLevel},
		{"UNKNOWN", zapcore.InfoLevel}, // Default case in switch
	}

	for _, tc := range testCases {
		t.Run(tc.levelStr, func(t *testing.T) {
			atomicLevel, _ := logLevelResolve(tc.levelStr)
			if atomicLevel.Level() != tc.expected {
				t.Errorf("logLevelResolve(%q) = %v, want %v", tc.levelStr, atomicLevel.Level(), tc.expected)
			}
		})
	}
}

func TestMyLogger_Logging(t *testing.T) {
	logger, buf := newTestLogger("DEBUG")

	tests := []struct {
		name     string
		logFunc  func()
		expected string
		level    string
	}{
		{"Debugf", func() { logger.Debugf("hello %s", "world") }, "hello world", "DEBUG"},
		{"Infof", func() { logger.Infof("info message") }, "info message", "INFO"},
		{"Warnf", func() { logger.Warnf("warn message") }, "warn message", "WARN"},
		{"Errorf", func() { logger.Errorf("error message") }, "error message", "ERROR"},
		{"Debugw", func() { logger.Debugw("structured debug", "key", "value") }, `structured debug	{"key": "value"}`, "DEBUG"},
		{"Infow", func() { logger.Infow("structured info", "key", "value") }, `structured info	{"key": "value"}`, "INFO"},
		{"Warnw", func() { logger.Warnw("structured warn", "key", "value") }, `structured warn	{"key": "value"}`, "WARN"},
		{"Errorw", func() { logger.Errorw("structured error", "key", "value") }, `structured error	{"key": "value"}`, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()
			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("%s did not write expected message. Got: %s, want contains: %s", tt.name, output, tt.expected)
			}
			if !strings.Contains(output, tt.level) {
				t.Errorf("%s did not have correct level. Got: %s, want contains: %s", tt.name, output, tt.level)
			}
		})
	}
}

func TestMyLogger_SetLevel(t *testing.T) {
	logger, buf := newTestLogger("INFO")

	// Should not be logged
	logger.Debugf("should not be logged")
	if buf.Len() > 0 {
		t.Errorf("Should not have logged DEBUG message at INFO level. Got: %s", buf.String())
	}
	buf.Reset()

	// Change level to DEBUG
	logger.SetLevel("DEBUG")
	if logger.GetLevel() != int32(zapcore.DebugLevel) {
		t.Errorf("GetLevel() after SetLevel(DEBUG) failed. Got: %v, want: %v", logger.GetLevel(), int32(zapcore.DebugLevel))
	}

	// Should be logged now
	logger.Debugf("should be logged")
	if !strings.Contains(buf.String(), "should be logged") {
		t.Errorf("Should have logged DEBUG message after level change. Got: %s", buf.String())
	}
	buf.Reset()

	// Change level to WARN
	logger.SetLevel("WARN")
	if logger.GetLevel() != int32(zapcore.WarnLevel) {
		t.Errorf("GetLevel() after SetLevel(WARN) failed. Got: %v, want: %v", logger.GetLevel(), int32(zapcore.WarnLevel))
	}

	// Should not be logged
	logger.Infof("should not be logged")
	if buf.Len() > 0 {
		t.Errorf("Should not have logged INFO message at WARN level. Got: %s", buf.String())
	}
	buf.Reset()
}

func TestDefaultLogger_GlobalFunctions(t *testing.T) {
	// Temporarily replace DefaultLogger for test
	originalLogger := DefaultLogger
	defer func() { DefaultLogger = originalLogger }()

	var buf bytes.Buffer
	con := zap.NewProductionEncoderConfig()
	con.EncodeTime = CustomTimeEncoder
	con.EncodeCaller = CustomCallerEncoder
	con.CallerKey = "line"

	al, _ := logLevelResolve("DEBUG")
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(con),
		zapcore.AddSync(&buf),
		al,
	)
	DefaultLogger = &MyLogger{
		LogPath: "",
		Logger:  zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar(),
		LogAl:   al,
		level:   zapcore.DebugLevel,
		zapcore: core,
	}
	SetLogLevel("DEBUG")

	Debugf("global debug")
	if !strings.Contains(buf.String(), "global debug") {
		t.Errorf("Global Debugf failed. Got: %s", buf.String())
	}
	buf.Reset()

	Infof("global info")
	if !strings.Contains(buf.String(), "global info") {
		t.Errorf("Global Infof failed. Got: %s", buf.String())
	}
	buf.Reset()

	SetLogLevel("WARN")
	if GetLogLevel() != int32(zapcore.WarnLevel) {
		t.Errorf("Global GetLogLevel/SetLogLevel failed. Got: %v, want: %v", GetLogLevel(), int32(zapcore.WarnLevel))
	}

	Infof("should not be logged")
	if buf.Len() > 0 {
		t.Errorf("Global SetLogLevel did not work as expected. Got: %s", buf.String())
	}
	buf.Reset()
}

func TestStdLogger(t *testing.T) {
	originalLogger := DefaultLogger
	defer func() { DefaultLogger = originalLogger }()

	var buf bytes.Buffer
	con := zap.NewProductionEncoderConfig()
	con.EncodeTime = CustomTimeEncoder
	con.EncodeCaller = CustomCallerEncoder
	con.CallerKey = "line"

	al, _ := logLevelResolve("DEBUG")
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(con),
		zapcore.AddSync(&buf),
		al,
	)
	DefaultLogger = &MyLogger{
		LogPath: "",
		Logger:  zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0)).Sugar(),
		LogAl:   al,
		zapcore: core,
	}

	stdlog := StdLogger(DefaultLogger, zapcore.InfoLevel, 0)
	stdlog.Printf("hello from stdlog")

	output := buf.String()
	if !strings.Contains(output, "hello from stdlog") {
		t.Errorf("StdLogger did not write expected output. Got: %s", output)
	}
	if !strings.Contains(strings.ToUpper(output), "INFO") {
		t.Errorf("StdLogger did not have correct level. Got: %s", output)
	}

	test_log := NewMyLogger(filepath.Join(filepath.Dir(defaultResolvedFilePath()), "test.log"), "DEBUG", 1)
	test_log.Debugf("test debug log message")
	test_log.Infof("test info log message")
	test_log.Warnf("test warn log message")
	test_log.Errorf("test error og message")
	test_log.Sync()
	test_log.Debugw("test debugw", "key", "value")
	test_log.Infow("test infow", "key", "value")
	test_log.Warnw("test warnw", "key", "value")
	test_log.Errorw("test errorw", "key", "value")
	test_log.Sync()
	test_log.Close()
}

func TestZapLoggerDoesNotPanicWhenRequestedLevelIsBelowCoreLevel(t *testing.T) {
	var buf bytes.Buffer
	con := zap.NewProductionEncoderConfig()
	con.EncodeTime = CustomTimeEncoder
	con.EncodeCaller = CustomCallerEncoder
	con.CallerKey = "line"

	al, lvl := logLevelResolve("INFO")
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(con),
		zapcore.AddSync(&buf),
		al,
	)
	logger := &MyLogger{
		LogPath: "",
		Logger:  zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0)).Sugar(),
		LogAl:   al,
		level:   lvl,
		zapcore: core,
	}

	require.NotPanics(t, func() {
		zl := ZapLogger(logger, zap.DebugLevel, 0)
		zl.Debug("debug should still be filtered by core")
		zl.Info("info should be logged")
	})
	output := buf.String()
	require.Contains(t, output, "info should be logged")
	require.NotContains(t, output, "debug should still be filtered by core")
}

func TestStdLoggerDoesNotPanicWhenRequestedLevelIsBelowCoreLevel(t *testing.T) {
	var buf bytes.Buffer
	con := zap.NewProductionEncoderConfig()
	con.EncodeTime = CustomTimeEncoder
	con.EncodeCaller = CustomCallerEncoder
	con.CallerKey = "line"

	al, lvl := logLevelResolve("WARN")
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(con),
		zapcore.AddSync(&buf),
		al,
	)
	logger := &MyLogger{
		LogPath: "",
		Logger:  zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0)).Sugar(),
		LogAl:   al,
		level:   lvl,
		zapcore: core,
	}

	require.NotPanics(t, func() {
		StdLogger(logger, zap.DebugLevel, 0).Printf("std logger uses effective level")
	})
	require.Contains(t, buf.String(), "std logger uses effective level")
}

func TestNewMyLoggerStdOutOnlyDoesNotCreateFileWriter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	logPath := filepath.Join(dir, "stdout-only.log")

	logger := NewMyLoggerWithOptions(Options{
		FilePath: logPath,
		Level:    "info",
		FileOut:  false,
		StdOut:   true,
	})
	defer logger.Close()

	if logger.closer != nil {
		t.Fatalf("stdout-only logger should not create file closer")
	}
	if logger.buffer != nil {
		t.Fatalf("stdout-only logger should not create file buffer")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("stdout-only logger should not create log directory, stat err=%v", err)
	}
}

func TestRotateDailyWriter(t *testing.T) {
	dir := filepath.Dir(defaultResolvedFilePath())

	logPath := filepath.Join(dir, "testlog")
	opts := Options{
		FilePath:      logPath,
		Level:         "debug",
		Rotation:      rotationHourly,
		MaxFileSizeMB: 1,
		RetentionDays: 1,
	}
	opts.normalize()
	ensureLogDir(opts.FilePath)
	writer := buildRotateWriter(opts)
	t.Cleanup(func() { writer.Close() })

	if _, err := writer.Write([]byte("test log entry\n")); err != nil {
		t.Fatalf("Failed to write to log: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}
	var rotatedPath string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == "testlog" {
			continue
		}
		if strings.HasPrefix(entry.Name(), "testlog.") {
			rotatedPath = filepath.Join(dir, entry.Name())
			break
		}
	}
	if rotatedPath == "" {
		t.Fatalf("expected rotated log file in %s", dir)
	}
	data, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("read rotated file: %v", err)
	}
	if !strings.Contains(string(data), "test log entry") {
		t.Fatalf("rotated file missing log content: %s", data)
	}
}

type durationStats struct {
	min time.Duration
	p50 time.Duration
	p95 time.Duration
	max time.Duration
	avg time.Duration
}

func sampleLockWaitStats(iterations int, lock func(), unlock func()) durationStats {
	samples := make([]time.Duration, iterations)
	var total time.Duration
	for i := range iterations {
		start := time.Now()
		lock()
		wait := time.Since(start)
		unlock()
		samples[i] = wait
		total += wait
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})
	return durationStats{
		min: samples[0],
		p50: samples[len(samples)/2],
		p95: samples[(len(samples)*95)/100],
		max: samples[len(samples)-1],
		avg: total / time.Duration(len(samples)),
	}
}

func TestRotateWriterLockWaitStats(t *testing.T) {
	const iterations = 20000

	var mu sync.Mutex
	rawStats := sampleLockWaitStats(iterations, mu.Lock, mu.Unlock)

	writer := &rotateLogWriter{}
	writerStats := sampleLockWaitStats(iterations, writer.mu.Lock, writer.mu.Unlock)

	t.Logf("raw mutex lockWait stats over %d iterations: min=%s p50=%s p95=%s max=%s avg=%s", iterations, rawStats.min, rawStats.p50, rawStats.p95, rawStats.max, rawStats.avg)
	t.Logf("rotate writer mutex lockWait stats over %d iterations: min=%s p50=%s p95=%s max=%s avg=%s", iterations, writerStats.min, writerStats.p50, writerStats.p95, writerStats.max, writerStats.avg)
}

func BenchmarkRotateWriterLockWait(b *testing.B) {
	var mu sync.Mutex

	var total time.Duration
	var maxWait time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		mu.Lock()
		wait := time.Since(start)
		mu.Unlock()
		total += wait
		if wait > maxWait {
			maxWait = wait
		}
	}
	b.ReportMetric(float64(total.Nanoseconds())/float64(b.N), "lock_wait_ns/op")
	b.ReportMetric(float64(maxWait.Nanoseconds()), "max_lock_wait_ns")
}

func BenchmarkLockWaitMeasurementOverhead(b *testing.B) {
	var total time.Duration
	var maxWait time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		wait := time.Since(start)
		total += wait
		if wait > maxWait {
			maxWait = wait
		}
	}
	b.ReportMetric(float64(total.Nanoseconds())/float64(b.N), "measured_ns/op")
	b.ReportMetric(float64(maxWait.Nanoseconds()), "max_measured_ns")
}

func BenchmarkMutexLockUnlock(b *testing.B) {
	var mu sync.Mutex
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.Lock()
		mu.Unlock()
	}
}

func TestMyLogger_Close(t *testing.T) {
	// This is hard to test without exposing internal state of zap's BufferedWriteSyncer.
	// We can at least call it and ensure it doesn't panic.
	logger, _ := newTestLogger("INFO")
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("The code panicked: %v", r)
		}
	}()
	logger.Close()
}

func TestRotateWriterSize(t *testing.T) {
	dir := filepath.Dir(defaultResolvedFilePath())
	logPath := filepath.Join(dir, "rotate.log")
	opts := Options{
		FilePath:      logPath,
		Rotation:      defaultRotationDaily,
		MaxFileSizeMB: 1,
		RetentionDays: 2,
	}
	opts.normalize()
	writer := buildRotateWriter(opts)
	t.Cleanup(func() { writer.Close() })

	chunk := bytes.Repeat([]byte("0123456789"), 12000) // ~120KB
	for range 10 {
		if _, err := writer.Write(append(chunk, '\n')); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var rotatedFiles int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "rotate.log.") && entry.Name() != "rotate.log" {
			rotatedFiles++
		}
	}
	if rotatedFiles < 2 {
		t.Fatalf("expected size-based rotation to create multiple files, got %d", rotatedFiles)
	}
}

func TestRotateWriterRestartAndTimeRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "resume.log")
	opts := Options{
		FilePath:      logPath,
		Rotation:      defaultRotationDaily,
		MaxFileSizeMB: 5,
		RetentionDays: 3,
	}
	opts.normalize()

	current := time.Date(2025, 1, 1, 10, 0, 0, 0, time.Local)
	now := func() time.Time { return current }

	writer, err := newRotateLogWriterWithClock(opts, now)
	if err != nil {
		t.Fatalf("init writer: %v", err)
	}
	if _, err := writer.Write([]byte("first\n")); err != nil {
		t.Fatalf("write first: %v", err)
	}
	writer.Close()

	writer, err = newRotateLogWriterWithClock(opts, now)
	if err != nil {
		t.Fatalf("restart writer: %v", err)
	}
	if _, err := writer.Write([]byte("second\n")); err != nil {
		t.Fatalf("write second: %v", err)
	}
	writer.Close()

	dayOne := filepath.Join(dir, fmt.Sprintf("resume.log.%s", current.Format(layoutDaily)))
	content, err := os.ReadFile(dayOne)
	if err != nil {
		t.Fatalf("read day one log: %v", err)
	}
	if !strings.Contains(string(content), "first") || !strings.Contains(string(content), "second") {
		t.Fatalf("expected both entries in day one log, got %s", content)
	}

	current = current.Add(24 * time.Hour)
	writer, err = newRotateLogWriterWithClock(opts, now)
	if err != nil {
		t.Fatalf("restart second day: %v", err)
	}
	if _, err := writer.Write([]byte("third\n")); err != nil {
		t.Fatalf("write third: %v", err)
	}
	writer.Close()

	dayTwo := filepath.Join(dir, fmt.Sprintf("resume.log.%s", current.Format(layoutDaily)))
	if _, err := os.Stat(dayTwo); err != nil {
		t.Fatalf("expected rotated file for new day: %v", err)
	}
}

func TestRotateWriterRetention(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "retention.log")
	opts := Options{
		FilePath:      logPath,
		Rotation:      defaultRotationDaily,
		MaxFileSizeMB: 1,
		RetentionDays: 1,
	}
	opts.normalize()

	current := time.Date(2025, 1, 10, 10, 0, 0, 0, time.Local)
	oldSlot := current.Add(-48 * time.Hour)
	oldFile := filepath.Join(dir, fmt.Sprintf("retention.log.%s", oldSlot.Format(layoutDaily)))
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatalf("prepare old log: %v", err)
	}

	writer, err := newRotateLogWriterWithClock(opts, func() time.Time { return current })
	if err != nil {
		t.Fatalf("init writer: %v", err)
	}
	if _, err := writer.Write([]byte("next\n")); err != nil {
		t.Fatalf("write next: %v", err)
	}
	writer.Close()

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old log to be removed, stat err=%v", err)
	}
}

func TestParseLogFileName(t *testing.T) {
	tests := []struct {
		name      string
		writer    rotateLogWriter
		file      string
		wantTime  time.Time
		wantIndex int
		wantOK    bool
	}{
		{
			name:      "daily index zero explicit",
			writer:    rotateLogWriter{baseName: "logic", rotation: defaultRotationDaily},
			file:      "logic.2025-11-21.0",
			wantTime:  time.Date(2025, 11, 21, 0, 0, 0, 0, time.Local),
			wantIndex: 0,
			wantOK:    true,
		},
		{
			name:      "daily index implicit",
			writer:    rotateLogWriter{baseName: "logic", rotation: defaultRotationDaily},
			file:      "logic.2025-11-21",
			wantTime:  time.Date(2025, 11, 21, 0, 0, 0, 0, time.Local),
			wantIndex: 0,
			wantOK:    true,
		},
		{
			name:      "daily index nonzero",
			writer:    rotateLogWriter{baseName: "logic", rotation: defaultRotationDaily},
			file:      "logic.2025-11-21.2",
			wantTime:  time.Date(2025, 11, 21, 0, 0, 0, 0, time.Local),
			wantIndex: 2,
			wantOK:    true,
		},
		{
			name:      "hourly index nonzero",
			writer:    rotateLogWriter{baseName: "logic", rotation: rotationHourly},
			file:      "logic.2025-11-21-10.3",
			wantTime:  time.Date(2025, 11, 21, 10, 0, 0, 0, time.Local),
			wantIndex: 3,
			wantOK:    true,
		},
		{
			name:   "prefix mismatch",
			writer: rotateLogWriter{baseName: "logic", rotation: defaultRotationDaily},
			file:   "other.2025-11-21.1",
			wantOK: false,
		},
		{
			name:   "bad time string",
			writer: rotateLogWriter{baseName: "logic", rotation: defaultRotationDaily},
			file:   "logic.badtime.1",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTime, gotIdx, ok := tc.writer.parseLogFileName(tc.file)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if !gotTime.Equal(tc.wantTime) {
				t.Fatalf("time = %v, want %v", gotTime, tc.wantTime)
			}
			if gotIdx != tc.wantIndex {
				t.Fatalf("index = %d, want %d", gotIdx, tc.wantIndex)
			}
		})
	}
}

func BenchmarkMyLoggerMemory(b *testing.B) {
	logger, _ := newTestLogger("INFO")
	payload := "benchmark message 0123456789"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Infof(payload)
	}
}

func BenchmarkMyLoggerFileSingle(b *testing.B) {
	dir := b.TempDir()
	logPath := filepath.Join(dir, "bench.log")
	opts := Options{
		FilePath:      logPath,
		Level:         "info",
		MaxFileSizeMB: 128,
		RetentionDays: 1,
		FileOut:       true,
		StdOut:        false,
	}
	opts.normalize()
	logger := NewMyLoggerWithOptions(opts)
	defer logger.Close()

	payload := "benchmark file single message 0123456789"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Infof(payload)
	}
}

func BenchmarkMyLoggerFileParallel(b *testing.B) {
	dir := b.TempDir()
	logPath := filepath.Join(dir, "bench_parallel.log")
	opts := Options{
		FilePath:      logPath,
		Level:         "info",
		MaxFileSizeMB: 128,
		RetentionDays: 1,
		FileOut:       true,
		StdOut:        false,
		Sync:          false,
	}
	opts.normalize()
	logger := NewMyLoggerWithOptions(opts)
	defer logger.Close()

	payload := "benchmark file parallel message 0123456789"
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Infof(payload)
		}
	})
}

func BenchmarkSyncMutexMeasuredLockWait(b *testing.B) {
	var mu sync.Mutex

	var total time.Duration
	var maxWait time.Duration

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lockStart := time.Now()
		mu.Lock()
		lockWait := time.Since(lockStart)
		mu.Unlock()

		total += lockWait
		if lockWait > maxWait {
			maxWait = lockWait
		}
	}
	b.StopTimer()

	b.ReportMetric(float64(total.Nanoseconds())/float64(b.N), "avg_lock_wait_ns")
	b.ReportMetric(float64(maxWait.Nanoseconds()), "max_lock_wait_ns")
}

func BenchmarkRotateWriterMeasuredLockWait(b *testing.B) {
	oldDisable := deadlock.Opts.Disable
	oldDisableLockOrderDetection := deadlock.Opts.DisableLockOrderDetection
	deadlock.Opts.Disable = true
	deadlock.Opts.DisableLockOrderDetection = true
	b.Cleanup(func() {
		deadlock.Opts.Disable = oldDisable
		deadlock.Opts.DisableLockOrderDetection = oldDisableLockOrderDetection
	})

	dir := b.TempDir()
	logPath := filepath.Join(dir, "bench_lockwait.log")
	opts := Options{
		FilePath:      logPath,
		Rotation:      defaultRotationDaily,
		MaxFileSizeMB: 128,
		RetentionDays: 1,
	}
	opts.normalize()

	writer, err := newRotateLogWriter(opts)
	if err != nil {
		b.Fatalf("newRotateLogWriter: %v", err)
	}
	b.Cleanup(func() { _ = writer.Close() })

	payload := []byte("bench lock wait message\n")
	var total time.Duration
	var maxWait time.Duration

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lockStart := time.Now()
		writer.mu.Lock()
		lockWait := time.Since(lockStart)
		total += lockWait
		if lockWait > maxWait {
			maxWait = lockWait
		}

		if err := writer.rotateLocked(writer.now(), int64(len(payload))); err != nil {
			writer.mu.Unlock()
			b.Fatalf("rotateLocked: %v", err)
		}
		n, err := writer.currentFile.Write(payload)
		if err != nil {
			writer.mu.Unlock()
			b.Fatalf("write: %v", err)
		}
		writer.currentSize += int64(n)
		writer.mu.Unlock()
	}
	b.StopTimer()

	b.ReportMetric(float64(total.Nanoseconds())/float64(b.N), "avg_lock_wait_ns")
	b.ReportMetric(float64(maxWait.Nanoseconds()), "max_lock_wait_ns")
}

func TestRotateWriterHourly(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "hourly.log")
	opts := Options{
		FilePath:      logPath,
		Rotation:      rotationHourly,
		MaxFileSizeMB: 10,
		RetentionDays: 2,
	}
	opts.normalize()

	current := time.Date(2025, 1, 1, 10, 0, 0, 0, time.Local)
	now := func() time.Time { return current }

	writer, err := newRotateLogWriterWithClock(opts, now)
	if err != nil {
		t.Fatalf("init writer: %v", err)
	}
	if _, err := writer.Write([]byte("first hour\n")); err != nil {
		t.Fatalf("write first: %v", err)
	}
	writer.Close()

	current = current.Add(time.Hour)
	writer, err = newRotateLogWriterWithClock(opts, now)
	if err != nil {
		t.Fatalf("init hour 2 writer: %v", err)
	}
	if _, err := writer.Write([]byte("second hour\n")); err != nil {
		t.Fatalf("write second: %v", err)
	}
	writer.Close()

	h1 := filepath.Join(dir, fmt.Sprintf("hourly.log.%s", time.Date(2025, 1, 1, 10, 0, 0, 0, time.Local).Format(layoutHourly)))
	h2 := filepath.Join(dir, fmt.Sprintf("hourly.log.%s", time.Date(2025, 1, 1, 11, 0, 0, 0, time.Local).Format(layoutHourly)))

	if _, err := os.Stat(h1); err != nil {
		t.Fatalf("expected first hour file: %v", err)
	}
	if _, err := os.Stat(h2); err != nil {
		t.Fatalf("expected second hour file: %v", err)
	}
}

func TestRotateWriterResumeIndex(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "resume-index.log")
	slot := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	baseName := filepath.Base(logPath)
	// prepare existing rotated files for the same slot
	existing0 := filepath.Join(dir, fmt.Sprintf("%s.%s", baseName, slot.Format(layoutDaily)))
	existing1 := filepath.Join(dir, fmt.Sprintf("%s.%s.%d", baseName, slot.Format(layoutDaily), 1))
	if err := os.WriteFile(existing0, []byte("old0\n"), 0o644); err != nil {
		t.Fatalf("prepare existing0: %v", err)
	}
	if err := os.WriteFile(existing1, []byte("old1\n"), 0o644); err != nil {
		t.Fatalf("prepare existing1: %v", err)
	}

	opts := Options{
		FilePath:      logPath,
		Rotation:      defaultRotationDaily,
		MaxFileSizeMB: 1,
		RetentionDays: 2,
	}
	opts.normalize()

	writer, err := newRotateLogWriterWithClock(opts, func() time.Time { return slot })
	if err != nil {
		t.Fatalf("init writer: %v", err)
	}
	// force small size threshold for test
	writer.maxSize = 32

	payload := bytes.Repeat([]byte("a"), 40)
	if _, err := writer.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	writer.Close()

	expectedNew := filepath.Join(dir, fmt.Sprintf("%s.%s.%d", baseName, slot.Format(layoutDaily), 2))
	if _, err := os.Stat(expectedNew); err != nil {
		t.Fatalf("expected new rotated file %s: %v", expectedNew, err)
	}
}

func TestRotateWriterDefersRotationUntilWrite(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "defer-rotate.log")
	slot := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	current := filepath.Join(dir, fmt.Sprintf("%s.%s", filepath.Base(logPath), slot.Format(layoutDaily)))
	if err := os.WriteFile(current, bytes.Repeat([]byte("a"), 11*bytesPerMB), 0o644); err != nil {
		t.Fatalf("prepare current log: %v", err)
	}

	opts := Options{
		FilePath:      logPath,
		Rotation:      defaultRotationDaily,
		MaxFileSizeMB: 10,
		RetentionDays: 3,
	}
	opts.normalize()
	writer, err := newRotateLogWriterWithClock(opts, func() time.Time { return slot })
	if err != nil {
		t.Fatalf("init writer: %v", err)
	}
	defer writer.Close()

	if writer.currentFilePath != current {
		t.Fatalf("expected writer to keep current file on init, got %s", writer.currentFilePath)
	}
	rotated := filepath.Join(dir, fmt.Sprintf("%s.%s.%d", filepath.Base(logPath), slot.Format(layoutDaily), 1))
	if _, err := os.Stat(rotated); !os.IsNotExist(err) {
		t.Fatalf("expected no rotated file before write, stat err=%v", err)
	}

	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if writer.currentFilePath != rotated {
		t.Fatalf("expected writer to rotate on write, got %s", writer.currentFilePath)
	}
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected rotated file after write: %v", err)
	}
}
