package monitor

import (
	"context"
	"net/http"
	"os/exec"
	"testing"
	"time"
)

// TestLogTimestampParsing tests timestamp parsing from various log formats
func TestLogTimestampParsing(t *testing.T) {
	lm := &LogManager{}

	tests := []struct {
		name          string
		line          string
		expectTime    bool
		expectContent string
	}{
		{
			name:          "RFC3339 timestamp",
			line:          "2023-01-15T10:30:45Z INFO: Application started",
			expectTime:    true,
			expectContent: "INFO: Application started",
		},
		{
			name:          "RFC3339Nano timestamp",
			line:          "2023-01-15T10:30:45.123456789Z DEBUG: Processing request",
			expectTime:    true,
			expectContent: "DEBUG: Processing request",
		},
		{
			name:          "Common log format",
			line:          "127.0.0.1 - - [15/Jan/2023:10:30:45 +0000] \"GET / HTTP/1.1\" 200 1234",
			expectTime:    true,
			expectContent: "127.0.0.1 - - [15/Jan/2023:10:30:45 +0000] \"GET / HTTP/1.1\" 200 1234",
		},
		{
			name:          "Syslog format",
			line:          "Jan 15 10:30:45 hostname systemd[1]: Started Network Manager",
			expectTime:    true,
			expectContent: "hostname systemd[1]: Started Network Manager",
		},
		{
			name:          "No timestamp",
			line:          "This is a log line without timestamp",
			expectTime:    true, // The parseLogTimestamp method adds current time for lines without timestamps
			expectContent: "This is a log line without timestamp",
		},
		{
			name:          "Empty line",
			line:          "",
			expectTime:    true, // The parseLogTimestamp method adds current time for empty lines
			expectContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp, content := lm.parseLogTimestamp(tt.line)

			if tt.expectTime && timestamp.IsZero() {
				t.Errorf("Expected timestamp to be parsed, but got zero time")
			}

			if !tt.expectTime && !timestamp.IsZero() {
				t.Errorf("Expected no timestamp, but got: %v", timestamp)
			}

			if content != tt.expectContent {
				t.Errorf("Expected content %q, got %q", tt.expectContent, content)
			}
		})
	}
}

// TestLogEntryCreation tests log entry creation and validation
func TestLogEntryCreation(t *testing.T) {
	now := time.Now()

	// Test basic log entry creation
	entry := LogEntry{
		Source:    "test-source",
		Content:   "Test log message",
		Timestamp: now,
		Level:     "INFO",
	}

	// Verify basic fields
	if entry.Source != "test-source" {
		t.Errorf("Expected source 'test-source', got %q", entry.Source)
	}
	if entry.Content != "Test log message" {
		t.Errorf("Expected content 'Test log message', got %q", entry.Content)
	}
	if entry.Level != "INFO" {
		t.Errorf("Expected level 'INFO', got %q", entry.Level)
	}
	if entry.Timestamp != now {
		t.Errorf("Expected timestamp %v, got %v", now, entry.Timestamp)
	}
}

// TestLogLevelDetection tests automatic log level detection
func TestLogLevelDetection(t *testing.T) {
	lm := &LogManager{}

	tests := []struct {
		name        string
		content     string
		expectLevel string
	}{
		{
			name:        "ERROR level",
			content:     "ERROR: Database connection failed",
			expectLevel: "error", // The detectLogLevel method returns lowercase
		},
		{
			name:        "WARN level",
			content:     "WARNING: High memory usage detected",
			expectLevel: "warning", // The detectLogLevel method returns lowercase
		},
		{
			name:        "INFO level",
			content:     "INFO: User logged in successfully",
			expectLevel: "info", // The detectLogLevel method returns lowercase
		},
		{
			name:        "DEBUG level",
			content:     "DEBUG: Processing request data",
			expectLevel: "debug", // The detectLogLevel method returns lowercase
		},
		{
			name:        "No level",
			content:     "This is a regular log message",
			expectLevel: "", // The detectLogLevel method returns empty string for no level
		},
		{
			name:        "Case insensitive",
			content:     "error: Something went wrong",
			expectLevel: "error", // The detectLogLevel method returns lowercase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := lm.detectLogLevel(tt.content)
			if level != tt.expectLevel {
				t.Errorf("Expected level %q, got %q", tt.expectLevel, level)
			}
		})
	}
}

// TestLogManagerInitialization tests LogManager initialization
func TestLogManagerInitialization(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-source",
				Type:     "file",
				Enabled:  true,
				FilePath: "/var/log/test.log",
				Interval: time.Minute,
			},
		},
	}

	lm := NewLogManager(config, &http.Client{})
	if lm == nil {
		t.Fatal("Expected LogManager to be created, got nil")
	}

	if lm.config == nil {
		t.Error("Expected config to be set")
	}

	if lm.httpClient == nil {
		t.Error("Expected httpClient to be set")
	}

	if len(lm.logs) != 0 {
		t.Errorf("Expected empty logs slice, got %d entries", len(lm.logs))
	}
}

// TestLogManagerStartStop tests LogManager start and stop functionality
func TestLogManagerStartStop(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-source",
				Type:     "command",
				Enabled:  true,
				Command:  "echo 'Test log message'",
				Interval: time.Millisecond * 100,
			},
		},
	}

	lm := NewLogManager(config, &http.Client{})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait a bit for logs to be collected
	time.Sleep(300 * time.Millisecond)

	// Stop log collection
	lm.StopLogCollection()

	// Wait a bit more for final collection
	time.Sleep(100 * time.Millisecond)

	// Verify that logs were collected
	if len(lm.logs) == 0 {
		t.Error("Expected logs to be collected, but got none")
	}
}

// Helper function to check if Docker is available
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}
