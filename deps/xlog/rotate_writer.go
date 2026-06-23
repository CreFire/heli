package xlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rotateLogWriter struct {
	basePath        string
	baseName        string
	dir             string
	rotation        string
	maxSize         int64
	retention       time.Duration
	currentFile     *os.File
	currentFilePath string
	currentPeriod   time.Time
	currentIndex    int
	currentSize     int64
	now             func() time.Time
	mu              sync.Mutex
}

func buildRotateWriter(opts Options) *rotateLogWriter {
	writer, err := newRotateLogWriter(opts)
	if err != nil {
		panic(err)
	}
	return writer
}

func newRotateLogWriter(opts Options) (*rotateLogWriter, error) {
	return newRotateLogWriterWithClock(opts, time.Now)
}

func newRotateLogWriterWithClock(opts Options, now func() time.Time) (*rotateLogWriter, error) {
	maxSize := int64(opts.MaxFileSizeMB) * bytesPerMB
	if maxSize <= 0 {
		maxSize = int64(defaultFileSizeMB) * bytesPerMB
	}

	retention := max(time.Duration(opts.RetentionDays)*24*time.Hour, 0)

	dir := filepath.Dir(opts.FilePath)
	if dir == "" {
		dir = "."
	}

	w := &rotateLogWriter{
		basePath:  filepath.Join(dir, filepath.Base(opts.FilePath)),
		baseName:  filepath.Base(opts.FilePath),
		dir:       dir,
		rotation:  opts.Rotation,
		maxSize:   maxSize,
		retention: retention,
		now:       now,
	}

	if err := w.init(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotateLogWriter) init() error {
	// Try to continue from the most recent log file if it matches our pattern.
	if path, slot, idx, size, ok := w.findLatestFile(); ok {
		if err := w.useFile(path, slot, idx, size); err != nil {
			return err
		}
	} else {
		now := w.now()
		slot := w.timeSlot(now)
		idx, err := w.nextIndexForSlot(slot)
		if err != nil {
			return err
		}
		if err := w.startNewFile(slot, idx); err != nil {
			return err
		}
	}

	w.cleanupOldFiles()
	return nil
}

func (w *rotateLogWriter) timeLayout() string {
	if w.rotation == rotationHourly {
		return layoutHourly
	}
	return layoutDaily
}

func (w *rotateLogWriter) timeSlot(t time.Time) time.Time {
	if w.rotation == rotationHourly {
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (w *rotateLogWriter) fileName(slot time.Time, index int) string {
	name := fmt.Sprintf("%s.%s", w.baseName, slot.Format(w.timeLayout()))
	if index > 0 {
		name = fmt.Sprintf("%s.%s.%d", w.baseName, slot.Format(w.timeLayout()), index)
	}
	return filepath.Join(w.dir, name)
}

func (w *rotateLogWriter) useFile(path string, slot time.Time, index int, size int64) error {
	if w.currentFile != nil {
		if err := w.currentFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "xlog: failed to close current log file %s: %v\n", w.currentFilePath, err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.currentFile = file
	w.currentFilePath = path
	w.currentPeriod = slot
	w.currentIndex = index
	w.currentSize = size
	if err := w.updateBaseLink(); err != nil {
		fmt.Fprintf(os.Stderr, "xlog: update base link failed: %v\n", err)
	}
	return nil
}

func (w *rotateLogWriter) startNewFile(slot time.Time, index int) error {
	path := w.fileName(slot, index)
	return w.useFile(path, slot, index, 0)
}

func (w *rotateLogWriter) findLatestFile() (string, time.Time, int, int64, bool) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return "", time.Time{}, 0, 0, false
	}

	var (
		latestPath string
		latestSlot time.Time
		latestIdx  int
		size       int64
		found      bool
	)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		slot, idx, ok := w.parseLogFileName(entry.Name())
		if !ok {
			continue
		}

		if !found || slot.After(latestSlot) || (slot.Equal(latestSlot) && idx > latestIdx) {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			latestPath = filepath.Join(w.dir, entry.Name())
			latestSlot = slot
			latestIdx = idx
			size = info.Size()
			found = true
		}
	}

	return latestPath, latestSlot, latestIdx, size, found
}

func (w *rotateLogWriter) parseLogFileName(name string) (time.Time, int, bool) {
	prefix := w.baseName + "."
	if !strings.HasPrefix(name, prefix) {
		return time.Time{}, 0, false
	}
	rest := strings.TrimPrefix(name, prefix)
	parts := strings.Split(rest, ".")
	if len(parts) == 0 {
		return time.Time{}, 0, false
	}

	timeStr := strings.Join(parts, ".")
	index := 0
	if len(parts) > 1 {
		if idx, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			index = idx
			timeStr = strings.Join(parts[:len(parts)-1], ".")
		}
	}

	slot, err := time.ParseInLocation(w.timeLayout(), timeStr, time.Local)
	if err != nil {
		return time.Time{}, 0, false
	}
	return slot, index, true
}

func (w *rotateLogWriter) nextIndexForSlot(slot time.Time) (int, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return 0, err
	}

	maxIdx := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		s, idx, ok := w.parseLogFileName(entry.Name())
		if !ok {
			continue
		}
		if !s.Equal(slot) {
			continue
		}
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	return maxIdx + 1, nil
}

func (w *rotateLogWriter) Write(p []byte) (int, error) {
	now := w.now()
	extra := int64(len(p))

	w.mu.Lock()

	if err := w.rotateLocked(now, extra); err != nil {
		w.mu.Unlock()
		return 0, err
	}

	file := w.currentFile
	n, err := file.Write(p)
	if err == nil {
		w.currentSize += int64(n)
	}
	w.mu.Unlock()
	return n, err
}

func (w *rotateLogWriter) rotate(now time.Time, extra int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.rotateLocked(now, extra)
}

func (w *rotateLogWriter) rotateLocked(now time.Time, extra int64) error {
	slotNow := w.timeSlot(now)
	if w.currentFile != nil && w.timeSlot(w.currentPeriod).Equal(slotNow) && (w.maxSize <= 0 || w.currentSize+extra <= w.maxSize) {
		return nil
	}

	if w.currentFile != nil {
		_ = w.currentFile.Sync()
		_ = w.currentFile.Close()
		w.currentFile = nil
	}

	nextIdx, err := w.nextIndexForSlot(slotNow)
	if err != nil {
		return err
	}

	if err := w.startNewFile(slotNow, nextIdx); err != nil {
		return err
	}

	w.cleanupOldFiles()
	return nil
}

func (w *rotateLogWriter) updateBaseLink() error {
	if w.currentFilePath == "" {
		return nil
	}

	linkPath := filepath.Join(w.dir, w.baseName)
	_ = os.Remove(linkPath)
	// 用户期望 basePath 是当前日志文件的软链接；若软链接不可用，再退化到硬链接。
	target := filepath.Base(w.currentFilePath) // 相对路径，确保在同一目录下
	if err := os.Symlink(target, linkPath); err == nil {
		return nil
	}
	if err := os.Link(w.currentFilePath, linkPath); err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "xlog: base log link failed for %s -> %s\n", linkPath, w.currentFilePath)
	return nil
}

func (w *rotateLogWriter) cleanupOldFiles() {
	if w.retention <= 0 {
		return
	}

	threshold := w.now().Add(-w.retention)
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == w.baseName {
			continue
		}
		slot, _, ok := w.parseLogFileName(entry.Name())
		if !ok {
			continue
		}
		if slot.Before(threshold) {
			path := filepath.Join(w.dir, entry.Name())
			if w.currentFilePath == path {
				continue
			}
			_ = os.Remove(path)
		}
	}
}

func (w *rotateLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentFile != nil {
		_ = w.currentFile.Sync()
		if err := w.currentFile.Close(); err != nil {
			return err
		}
		w.currentFile = nil
	}
	return nil
}

func (w *rotateLogWriter) Sync() error {
	return nil
}
