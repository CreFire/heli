package configdoc

import (
	"game/deps/xlog"
	"path/filepath"
	"strings"
)

const (
	defaultLogLevel         = "debug"
	defaultLogPath          = "./logs"
	defaultLogFileName      = "leog"
	defaultLogRotation      = "daily"
	defaultLogRetentionDays = 14
	defaultLogFileSizeMB    = 10
)

// LogCfg manages log related configuration for a service.
type LogCfg struct {
	Level         string `yaml:"level"`
	Path          string `yaml:"path"`
	FileName      string `yaml:"fileName"`
	Rotation      string `yaml:"rotation"`
	MaxFileSizeMB int    `yaml:"maxFileSizeMB"`
	Sync          bool   `yaml:"sync"`
	RetentionDays int    `yaml:"retentionDays"`
	FileOut       bool   `yaml:"fileOut"`
	StdOut        bool   `yaml:"stdOut"`
}

func (c *LogCfg) Clone() *LogCfg {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

// EnsureDefaults fills missing values with safe defaults.
func (c *LogCfg) EnsureDefaults() {
	if c == nil {
		return
	}
	c.Level = resolveString(c.Level, defaultLogLevel)
	c.Path = resolveString(c.Path, defaultLogPath)
	c.FileName = resolveString(c.FileName, defaultLogFileName)
	c.Rotation = normalizeRotation(c.Rotation)
	if c.Rotation == "" {
		c.Rotation = defaultLogRotation
	}
	if c.MaxFileSizeMB <= 0 {
		c.MaxFileSizeMB = defaultLogFileSizeMB
	}
	if c.RetentionDays <= 0 {
		c.RetentionDays = defaultLogRetentionDays
	}
}

// FilePath resolves the filesystem path (without extension) for the log file.
func (c *LogCfg) FilePath() string {
	if c == nil {
		return filepath.Clean(filepath.Join(defaultLogPath, defaultLogFileName))
	}
	dir := strings.TrimSpace(c.Path)
	name := strings.TrimSpace(c.FileName)
	switch {
	case dir == "" && name == "":
		return filepath.Clean(filepath.Join(defaultLogPath, defaultLogFileName))
	case dir == "":
		return filepath.Clean(filepath.Join(defaultLogPath, name))
	case name == "":
		return filepath.Clean(filepath.Join(dir, defaultLogFileName))
	default:
		return filepath.Clean(filepath.Join(dir, name))
	}
}

// EqualOutput determines whether two log configurations write to the same destination
// and share rotation/retention settings (excluding log level).
func (c *LogCfg) EqualOutput(other *LogCfg) bool {
	if c == nil || other == nil {
		return c == other
	}
	return strings.EqualFold(c.Path, other.Path) &&
		strings.EqualFold(c.FileName, other.FileName) &&
		normalizeRotation(c.Rotation) == normalizeRotation(other.Rotation) &&
		c.MaxFileSizeMB == other.MaxFileSizeMB &&
		c.RetentionDays == other.RetentionDays
}

// Equal compares every field including level.
func (c *LogCfg) Equal(other *LogCfg) bool {
	if c == nil || other == nil {
		return c == other
	}
	return c.EqualOutput(other) &&
		strings.EqualFold(c.Level, other.Level)
}

// DefaultLogCfg returns a LogCfg instance populated with default values.
func DefaultLogCfg() *LogCfg {
	cfg := &LogCfg{
		Level:         defaultLogLevel,
		Path:          defaultLogPath,
		FileName:      defaultLogFileName,
		Rotation:      defaultLogRotation,
		MaxFileSizeMB: defaultLogFileSizeMB,
		RetentionDays: defaultLogRetentionDays,
		FileOut:       false,
		StdOut:        false,
	}
	return cfg
}

// MergeLogCfg applies override onto base returning a new config instance that never mutates input pointers.
func MergeLogCfg(base, override *LogCfg) *LogCfg {
	var merged *LogCfg
	if base != nil {
		merged = base.Clone()
	} else {
		merged = DefaultLogCfg()
	}

	if override != nil {
		if lvl := strings.TrimSpace(override.Level); lvl != "" {
			merged.Level = lvl
		}
		if p := strings.TrimSpace(override.Path); p != "" {
			merged.Path = p
		}
		if name := strings.TrimSpace(override.FileName); name != "" {
			merged.FileName = name
		}
		if rot := normalizeRotation(override.Rotation); rot != "" {
			merged.Rotation = rot
		}
		if override.MaxFileSizeMB > 0 {
			merged.MaxFileSizeMB = override.MaxFileSizeMB
		}
		if override.RetentionDays > 0 {
			merged.RetentionDays = override.RetentionDays
		}
		if override.FileOut {
			merged.FileOut = override.FileOut
		}
		if override.StdOut {
			merged.StdOut = override.StdOut
		}
		if override.Sync {
			merged.Sync = override.Sync
		}
	}
	merged.EnsureDefaults()
	return merged
}

func resolveString(val, fallback string) string {
	v := strings.TrimSpace(val)
	if v == "" {
		return fallback
	}
	return v
}

func normalizeRotation(rot string) string {
	r := strings.ToLower(strings.TrimSpace(rot))
	switch r {
	case "daily", "day":
		return "daily"
	case "hourly", "hour":
		return "hourly"
	default:
		return ""
	}
}

func LogCfgToOptions(cfg *LogCfg) xlog.Options {
	if cfg == nil {
		cfg = DefaultLogCfg()
	}
	effective := cfg.Clone()
	if effective == nil {
		effective = DefaultLogCfg()
	}
	effective.EnsureDefaults()
	return xlog.Options{
		FilePath:      effective.FilePath(),
		Level:         effective.Level,
		Rotation:      effective.Rotation,
		MaxFileSizeMB: effective.MaxFileSizeMB,
		RetentionDays: effective.RetentionDays,
		StdOut:        effective.StdOut,
		FileOut:       effective.FileOut,
		Sync:          effective.Sync,
	}
}
