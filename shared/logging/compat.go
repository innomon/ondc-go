package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

var (
	exitMu      = mu
	exitHandler = os.Exit
)

func setExitHandler(h func(int)) {
	exitMu.Lock()
	defer exitMu.Unlock()
	exitHandler = h
}

func resetExitHandler() {
	exitMu.Lock()
	defer exitMu.Unlock()
	exitHandler = os.Exit
}

func doExit(code int) {
	exitMu.RLock()
	h := exitHandler
	exitMu.RUnlock()
	h(code)
}

func logMsg(level slog.Level, msg string) {
	ctx := context.Background()
	logger := GetLogger()
	if !logger.Handler().Enabled(ctx, level) {
		return
	}

	pc := findCallerPC()
	r := slog.NewRecord(time.Now(), level, msg, pc)
	_ = logger.Handler().Handle(ctx, r)
}

func logFormat(level slog.Level, format string, args ...any) {
	ctx := context.Background()
	logger := GetLogger()
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

// Info logs using fmt.Sprint to level INFO.
func Info(args ...any) {
	logMsg(slog.LevelInfo, fmt.Sprint(args...))
}

// Infof logs formatted message to level INFO.
func Infof(format string, args ...any) {
	logFormat(slog.LevelInfo, format, args...)
}

// Infoln logs line message to level INFO.
func Infoln(args ...any) {
	logMsg(slog.LevelInfo, fmt.Sprintln(args...))
}

// Warning logs message to level WARN.
func Warning(args ...any) {
	logMsg(slog.LevelWarn, fmt.Sprint(args...))
}

// Warningf logs formatted message to level WARN.
func Warningf(format string, args ...any) {
	logFormat(slog.LevelWarn, format, args...)
}

// Warningln logs line message to level WARN.
func Warningln(args ...any) {
	logMsg(slog.LevelWarn, fmt.Sprintln(args...))
}

// Error logs message to level ERROR.
func Error(args ...any) {
	logMsg(slog.LevelError, fmt.Sprint(args...))
}

// Errorf logs formatted message to level ERROR.
func Errorf(format string, args ...any) {
	logFormat(slog.LevelError, format, args...)
}

// Errorln logs line message to level ERROR.
func Errorln(args ...any) {
	logMsg(slog.LevelError, fmt.Sprintln(args...))
}

// Exit logs message to ERROR and exits with code 1.
func Exit(args ...any) {
	logMsg(slog.LevelError, fmt.Sprint(args...))
	doExit(1)
}

// Exitf logs formatted message to ERROR and exits with code 1.
func Exitf(format string, args ...any) {
	logFormat(slog.LevelError, format, args...)
	doExit(1)
}

// Exitln logs line message to ERROR and exits with code 1.
func Exitln(args ...any) {
	logMsg(slog.LevelError, fmt.Sprintln(args...))
	doExit(1)
}

// Verbose represents a boolean check for verbosity levels (compatible with glog.V).
type Verbose bool

// V reports whether verbosity at the given level is enabled.
// level 1 or higher maps to Debug level in slog.
func V(level int) Verbose {
	ctx := context.Background()
	logger := GetLogger()
	if level <= 0 {
		return Verbose(logger.Handler().Enabled(ctx, slog.LevelInfo))
	}
	return Verbose(logger.Handler().Enabled(ctx, slog.LevelDebug))
}

// Info logs verbose message if enabled.
func (v Verbose) Info(args ...any) {
	if v {
		logMsg(slog.LevelDebug, fmt.Sprint(args...))
	}
}

// Infof logs verbose formatted message if enabled.
func (v Verbose) Infof(format string, args ...any) {
	if v {
		logFormat(slog.LevelDebug, format, args...)
	}
}

// Infoln logs verbose line message if enabled.
func (v Verbose) Infoln(args ...any) {
	if v {
		logMsg(slog.LevelDebug, fmt.Sprintln(args...))
	}
}
