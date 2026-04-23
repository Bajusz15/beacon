// Package logging provides a thin logger with per-component prefixes.
//
// Every logger is created with a free-form name that becomes the prefix:
//
//	logging.New("master")            -> [Beacon master] ...
//	logging.New(cfg.ProjectID)       -> [Beacon home-assistant] ...
//	logging.New("tunnel " + id)      -> [Beacon tunnel abc123] ...
//
// Output format matches the old stdlib log: "YYYY/MM/DD HH:MM:SS [Beacon name] message".
// Levels are filtered globally via SetLevel; BEACON_LOG_LEVEL env var overrides any
// programmatic setting.
package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level represents a log severity level.
type Level int32

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	currentLevel atomic.Int32 // stores Level
	envOverride  bool         // true if BEACON_LOG_LEVEL was set at startup

	outMu sync.Mutex
	out   io.Writer = os.Stderr
)

func init() {
	currentLevel.Store(int32(LevelInfo))
	if v := strings.TrimSpace(os.Getenv("BEACON_LOG_LEVEL")); v != "" {
		if lvl, ok := parseLevel(v); ok {
			currentLevel.Store(int32(lvl))
			envOverride = true
		}
	}
}

// SetLevel sets the global minimum log level. Values: "debug", "info", "warn", "error".
// A value of "" or an unrecognized level is ignored. BEACON_LOG_LEVEL env var takes
// precedence — if it was set at process start, SetLevel is a no-op.
func SetLevel(level string) {
	if envOverride {
		return
	}
	if lvl, ok := parseLevel(level); ok {
		currentLevel.Store(int32(lvl))
	}
}

// SetOutput redirects log output. Used by tests.
func SetOutput(w io.Writer) {
	outMu.Lock()
	defer outMu.Unlock()
	if w == nil {
		out = os.Stderr
	} else {
		out = w
	}
}

func parseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn", "warning":
		return LevelWarn, true
	case "error", "err":
		return LevelError, true
	}
	return 0, false
}

// Logger carries a baked-in prefix and writes to the package output.
type Logger struct {
	prefix string // e.g. "[Beacon master]"
}

// New creates a logger whose prefix is "[Beacon <name>]".
// The name is free-form — callers pass whatever reads best for their component.
func New(name string) *Logger {
	name = strings.TrimSpace(name)
	if name == "" {
		return &Logger{prefix: "[Beacon]"}
	}
	return &Logger{prefix: "[Beacon " + name + "]"}
}

func (l *Logger) enabled(lvl Level) bool {
	return int32(lvl) >= currentLevel.Load()
}

func (l *Logger) write(lvl Level, format string, args ...any) {
	if !l.enabled(lvl) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	ts := time.Now().Format("2006/01/02 15:04:05")
	var line string
	if lvl == LevelInfo {
		line = fmt.Sprintf("%s %s %s\n", ts, l.prefix, msg)
	} else {
		line = fmt.Sprintf("%s %s %s: %s\n", ts, l.prefix, levelTag(lvl), msg)
	}
	outMu.Lock()
	_, _ = io.WriteString(out, line)
	outMu.Unlock()
}

func levelTag(lvl Level) string {
	switch lvl {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Debugf logs at debug level.
func (l *Logger) Debugf(format string, args ...any) { l.write(LevelDebug, format, args...) }

// Infof logs at info level (the default).
func (l *Logger) Infof(format string, args ...any) { l.write(LevelInfo, format, args...) }

// Warnf logs at warn level.
func (l *Logger) Warnf(format string, args ...any) { l.write(LevelWarn, format, args...) }

// Errorf logs at error level.
func (l *Logger) Errorf(format string, args ...any) { l.write(LevelError, format, args...) }

// Fatalf logs at error level and then calls os.Exit(1).
func (l *Logger) Fatalf(format string, args ...any) {
	l.write(LevelError, format, args...)
	os.Exit(1)
}

// CloseAndLog closes c and logs a warning if Close returns an error.
// Intended for use with defer: defer logger.CloseAndLog(c, "smtp client")
func (l *Logger) CloseAndLog(c io.Closer, label string) {
	if err := c.Close(); err != nil {
		l.Warnf("close %s: %v", label, err)
	}
}
