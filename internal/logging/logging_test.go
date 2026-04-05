package logging

import (
	"bytes"
	"strings"
	"testing"
)

// resetForTest restores default state between tests.
func resetForTest(t *testing.T) {
	t.Helper()
	currentLevel.Store(int32(LevelInfo))
	envOverride = false
	SetOutput(nil)
}

func captureOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	SetOutput(buf)
	return buf
}

func TestPrefixFormat(t *testing.T) {
	resetForTest(t)
	buf := captureOutput(t)

	cases := []struct {
		name   string
		want   string
	}{
		{"master", "[Beacon master]"},
		{"home-assistant", "[Beacon home-assistant]"},
		{"tunnel abc123", "[Beacon tunnel abc123]"},
		{"", "[Beacon]"},
		{"  deploy  ", "[Beacon deploy]"},
	}
	for _, tc := range cases {
		buf.Reset()
		l := New(tc.name)
		l.Infof("hello %s", "world")
		got := buf.String()
		if !strings.Contains(got, tc.want) {
			t.Errorf("New(%q).Infof: want prefix %q in output, got %q", tc.name, tc.want, got)
		}
		if !strings.Contains(got, "hello world") {
			t.Errorf("New(%q).Infof: missing formatted message in output %q", tc.name, got)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	resetForTest(t)
	buf := captureOutput(t)
	l := New("test")

	// Default is info: debug suppressed, info/warn/error pass.
	l.Debugf("debug1")
	l.Infof("info1")
	l.Warnf("warn1")
	l.Errorf("error1")
	out := buf.String()
	if strings.Contains(out, "debug1") {
		t.Errorf("debug should be filtered at info level, got %q", out)
	}
	for _, want := range []string{"info1", "warn1", "error1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output %q", want, out)
		}
	}

	// Raise to warn: info also filtered.
	buf.Reset()
	SetLevel("warn")
	l.Debugf("debug2")
	l.Infof("info2")
	l.Warnf("warn2")
	out = buf.String()
	if strings.Contains(out, "debug2") || strings.Contains(out, "info2") {
		t.Errorf("debug/info should be filtered at warn level, got %q", out)
	}
	if !strings.Contains(out, "warn2") {
		t.Errorf("warn should pass, got %q", out)
	}
}

func TestEnvOverrideBlocksSetLevel(t *testing.T) {
	resetForTest(t)
	envOverride = true
	currentLevel.Store(int32(LevelError))

	SetLevel("debug") // should be ignored
	if currentLevel.Load() != int32(LevelError) {
		t.Errorf("env override should block SetLevel, level is %v", currentLevel.Load())
	}
}

func TestLevelTagInOutput(t *testing.T) {
	resetForTest(t)
	buf := captureOutput(t)
	l := New("x")
	l.Warnf("careful")
	l.Errorf("boom")
	out := buf.String()
	if !strings.Contains(out, "WARN: careful") {
		t.Errorf("want WARN tag, got %q", out)
	}
	if !strings.Contains(out, "ERROR: boom") {
		t.Errorf("want ERROR tag, got %q", out)
	}
}

func TestInfoHasNoLevelTag(t *testing.T) {
	resetForTest(t)
	buf := captureOutput(t)
	l := New("x")
	l.Infof("plain")
	out := buf.String()
	if strings.Contains(out, "INFO:") {
		t.Errorf("info lines should not carry a level tag, got %q", out)
	}
	if !strings.Contains(out, "[Beacon x] plain") {
		t.Errorf("want formatted info line, got %q", out)
	}
}
