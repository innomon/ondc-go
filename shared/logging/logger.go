package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Config defines the configuration for the hybrid logger.
type Config struct {
	Level          string
	ConsoleEnabled bool
	FileEnabled    bool
	Dir            string
	FileName       string
	MaxSizeMB      int
	MaxBackups     int
}

var (
	mu  sync.RWMutex
	Log *slog.Logger = slog.Default()
)

// SetLogger sets the active global logger.
func SetLogger(l *slog.Logger) {
	mu.Lock()
	defer mu.Unlock()
	Log = l
	slog.SetDefault(l)
}

// GetLogger returns the active global logger.
func GetLogger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return Log
}

// ParseLevel converts a string representation to slog.Level.
func ParseLevel(lvl string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(lvl)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Init configures and sets the global structured logger.
func Init(cfg Config) (*slog.Logger, error) {
	level := ParseLevel(cfg.Level)

	var handlers []slog.Handler

	if cfg.ConsoleEnabled {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		}))
	}

	if cfg.FileEnabled {
		dir := cfg.Dir
		if dir == "" {
			dir = "./data/logs"
		}
		filename := cfg.FileName
		if filename == "" {
			filename = "ondc-node.jsonl"
		}

		rotator, err := NewLogRotator(dir, filename, cfg.MaxSizeMB, cfg.MaxBackups)
		if err != nil {
			return nil, fmt.Errorf("failed to init log rotator: %w", err)
		}

		handlers = append(handlers, slog.NewJSONHandler(rotator, &slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		}))
	}

	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}

	logger := slog.New(NewMultiHandler(handlers...))
	SetLogger(logger)
	return logger, nil
}

// CustomLogger wraps slog.Logger with caller PC frame recovery.
type CustomLogger struct {
	logger *slog.Logger
}

//go:noinline
func NewCustomLogger(l *slog.Logger) *CustomLogger {
	if l == nil {
		l = GetLogger()
	}
	return &CustomLogger{logger: l}
}

func (c *CustomLogger) log(level slog.Level, format string, args ...any) {
	ctx := context.Background()
	logger := c.logger
	if logger == nil {
		logger = GetLogger()
	}

	if !logger.Handler().Enabled(ctx, level) {
		return
	}

	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}

	pc := findCallerPC()

	r := slog.NewRecord(time.Now(), level, msg, pc)
	_ = logger.Handler().Handle(ctx, r)
}

// findCallerPC inspects the call stack to locate the first frame outside logger.go and compat.go.
func findCallerPC() uintptr {
	var pcs [32]uintptr
	n := runtime.Callers(1, pcs[:])
	if n == 0 {
		return 0
	}

	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		baseFile := filepath.Base(frame.File)
		if baseFile != "logger.go" && baseFile != "compat.go" &&
			!strings.Contains(frame.Function, "runtime.") {
			return frame.PC
		}
		if !more {
			break
		}
	}
	return 0
}

// Infof logs formatted info message preserving caller frame.
func (c *CustomLogger) Infof(format string, args ...any) {
	c.log(slog.LevelInfo, format, args...)
}

// Errorf logs formatted error message preserving caller frame.
func (c *CustomLogger) Errorf(format string, args ...any) {
	c.log(slog.LevelError, format, args...)
}

// Warningf logs formatted warning message preserving caller frame.
func (c *CustomLogger) Warningf(format string, args ...any) {
	c.log(slog.LevelWarn, format, args...)
}

// Debugf logs formatted debug message preserving caller frame.
func (c *CustomLogger) Debugf(format string, args ...any) {
	c.log(slog.LevelDebug, format, args...)
}
