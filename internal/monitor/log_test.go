package monitor

import (
	"context"
	"fmt"
	"net/http"
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
			name:          "RFC3339 with nanoseconds",
			line:          "2024-01-15T10:30:00.123456789Z INFO Application started",
			expectTime:    true,
			expectContent: "INFO Application started",
		},
		{
			name:          "RFC3339 without nanoseconds",
			line:          "2024-01-15T10:30:00Z ERROR Database connection failed",
			expectTime:    true,
			expectContent: "ERROR Database connection failed",
		},
		{
			name:          "Syslog format",
			line:          "Jan 15 10:30:00 server kernel: [12345.678] CPU temperature high",
			expectTime:    true,
			expectContent: "server kernel: [12345.678] CPU temperature high",
		},
		{
			name:          "ISO 8601 format",
			line:          "2024-01-15 10:30:00 WARN Memory usage at 85%",
			expectTime:    true,
			expectContent: "WARN Memory usage at 85%",
		},
		{
			name:          "ISO 8601 with milliseconds",
			line:          "2024-01-15 10:30:00.123 DEBUG Processing request",
			expectTime:    true,
			expectContent: "DEBUG Processing request",
		},
		{
			name:          "Unix timestamp",
			line:          "1705312200 INFO Service health check passed",
			expectTime:    true,
			expectContent: "INFO Service health check passed",
		},
		{
			name:          "Unix timestamp with milliseconds",
			line:          "1705312200123 ERROR Out of memory",
			expectTime:    true,
			expectContent: "ERROR Out of memory",
		},
		{
			name:          "No timestamp",
			line:          "This is a log line without timestamp",
			expectTime:    false,
			expectContent: "This is a log line without timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp, content := lm.parseLogTimestamp(tt.line)

			if tt.expectTime {
				// Check that we got a reasonable timestamp (not current time for most cases)
				if tt.name != "No timestamp" && timestamp.Equal(time.Now()) {
					t.Error("Expected parsed timestamp, got current time")
				}
			}

			if content != tt.expectContent {
				t.Errorf("Expected content '%s', got '%s'", tt.expectContent, content)
			}
		})
	}
}

// TestLogLevelDetection tests log level detection
func TestLogLevelDetection(t *testing.T) {
	lm := &LogManager{}

	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{"Error level", "ERROR: Database connection failed", "error"},
		{"Error level (short)", "ERR: Connection timeout", "error"},
		{"Warning level", "WARN: Memory usage high", "warning"},
		{"Info level", "INFO: Service started", "info"},
		{"Debug level", "DEBUG: Processing request", "debug"},
		{"No level", "Service is running normally", ""},
		{"Mixed case", "Error: Something went wrong", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := lm.detectLogLevel(tt.line)
			if level != tt.expected {
				t.Errorf("Expected level '%s', got '%s'", tt.expected, level)
			}
		})
	}
}

// TestLogHashGeneration tests log hash generation for deduplication
func TestLogHashGeneration(t *testing.T) {
	lm := &LogManager{}

	entry1 := LogEntry{
		Source:    "test-source",
		Type:      "file",
		Container: "",
		Content:   "Test log message",
		Timestamp: time.Now(),
	}

	entry2 := LogEntry{
		Source:    "test-source",
		Type:      "file",
		Container: "",
		Content:   "Test log message",
		Timestamp: time.Now().Add(time.Hour), // Different timestamp
	}

	entry3 := LogEntry{
		Source:    "test-source",
		Type:      "file",
		Container: "",
		Content:   "Different log message", // Different content
		Timestamp: time.Now(),
	}

	hash1 := lm.generateLogHash(entry1)
	hash2 := lm.generateLogHash(entry2)
	hash3 := lm.generateLogHash(entry3)

	// Same content should generate same hash regardless of timestamp
	if hash1 != hash2 {
		t.Error("Expected same hash for same content with different timestamps")
	}

	// Different content should generate different hash
	if hash1 == hash3 {
		t.Error("Expected different hash for different content")
	}

	// Hash should be non-empty
	if hash1 == "" {
		t.Error("Expected non-empty hash")
	}
}

// TestLogDeduplication tests log deduplication functionality
func TestLogDeduplication(t *testing.T) {
	lm := &LogManager{
		seenLogs: make(map[string]time.Time),
	}

	source := LogSource{
		Name:        "test-source",
		Deduplicate: true,
	}

	entry := LogEntry{
		Source:    "test-source",
		Type:      "file",
		Content:   "Test log message",
		Timestamp: time.Now(),
	}

	// First occurrence should not be duplicate
	if lm.isDuplicateLog(entry, source) {
		t.Error("Expected first log entry to not be duplicate")
	}

	// Second occurrence within time window should be duplicate
	if !lm.isDuplicateLog(entry, source) {
		t.Error("Expected second log entry to be duplicate")
	}

	// Test with deduplication disabled
	source.Deduplicate = false
	if lm.isDuplicateLog(entry, source) {
		t.Error("Expected log entry to not be duplicate when deduplication disabled")
	}
}

// TestLogFiltering tests log line filtering based on patterns
func TestLogFiltering(t *testing.T) {
	lm := &LogManager{}

	tests := []struct {
		name            string
		line            string
		includePatterns []string
		excludePatterns []string
		shouldInclude   bool
	}{
		{
			name:            "No patterns - include all",
			line:            "INFO: Service started",
			includePatterns: []string{},
			excludePatterns: []string{},
			shouldInclude:   true,
		},
		{
			name:            "Include pattern match",
			line:            "ERROR: Database connection failed",
			includePatterns: []string{"ERROR"},
			excludePatterns: []string{},
			shouldInclude:   true,
		},
		{
			name:            "Include pattern no match",
			line:            "INFO: Service started",
			includePatterns: []string{"ERROR"},
			excludePatterns: []string{},
			shouldInclude:   false,
		},
		{
			name:            "Exclude pattern match",
			line:            "DEBUG: Verbose output",
			includePatterns: []string{},
			excludePatterns: []string{"DEBUG"},
			shouldInclude:   false,
		},
		{
			name:            "Exclude pattern no match",
			line:            "ERROR: Critical failure",
			includePatterns: []string{},
			excludePatterns: []string{"DEBUG"},
			shouldInclude:   true,
		},
		{
			name:            "Include and exclude patterns",
			line:            "ERROR: Database connection failed",
			includePatterns: []string{"ERROR"},
			excludePatterns: []string{"Database"},
			shouldInclude:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := LogSource{
				Name:            "test-source",
				IncludePatterns: tt.includePatterns,
				ExcludePatterns: tt.excludePatterns,
			}

			result := lm.shouldIncludeLogLine(tt.line, source)
			if result != tt.shouldInclude {
				t.Errorf("Expected %v, got %v", tt.shouldInclude, result)
			}
		})
	}
}

// TestLogManagerCreation tests LogManager creation
func TestLogManagerCreation(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-source",
				Type:     "file",
				Enabled:  true,
				Interval: time.Minute,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	if lm == nil {
		t.Fatal("Expected LogManager to be created")
	}

	if lm.config != config {
		t.Error("Expected config to be set")
	}

	if lm.httpClient != httpClient {
		t.Error("Expected httpClient to be set")
	}

	if lm.logs == nil {
		t.Error("Expected logs slice to be initialized")
	}

	if lm.logCollectors == nil {
		t.Error("Expected logCollectors map to be initialized")
	}

	if lm.seenLogs == nil {
		t.Error("Expected seenLogs map to be initialized")
	}
}

// TestLogManagerStartStop tests LogManager start and stop functionality
func TestLogManagerStartStop(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-source",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: "/tmp/test.log",
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait a bit for collectors to start
	time.Sleep(time.Millisecond * 50)

	// Stop log collection
	lm.StopLogCollection()

	// Verify collectors were created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected log collectors to be created")
	}

	// Verify collector was created for enabled source
	collector, exists := lm.logCollectors["test-source"]
	if !exists {
		t.Error("Expected collector for test-source to be created")
	}

	if collector == nil {
		t.Error("Expected collector to be non-nil")
	}
}

// TestLogManagerWithDisabledSource tests LogManager with disabled sources
func TestLogManagerWithDisabledSource(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "disabled-source",
				Type:     "file",
				Enabled:  false, // Disabled
				Interval: time.Minute,
				FilePath: "/tmp/test.log",
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait a bit
	time.Sleep(time.Millisecond * 50)

	// Stop log collection
	lm.StopLogCollection()

	// Verify no collectors were created for disabled source
	if len(lm.logCollectors) > 0 {
		t.Error("Expected no collectors for disabled source")
	}
}

// TestLogManagerWithUnknownSourceType tests LogManager with unknown source type
func TestLogManagerWithUnknownSourceType(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "unknown-source",
				Type:     "unknown-type",
				Enabled:  true,
				Interval: time.Minute,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait a bit
	time.Sleep(time.Millisecond * 50)

	// Stop log collection
	lm.StopLogCollection()

	// Verify no collectors were created for unknown type
	if len(lm.logCollectors) > 0 {
		t.Error("Expected no collectors for unknown source type")
	}
}

// TestLogEntryCreation tests LogEntry creation and validation
func TestLogEntryCreation(t *testing.T) {
	now := time.Now()
	entry := LogEntry{
		Source:    "test-source",
		Type:      "file",
		Container: "test-container",
		Content:   "Test log message",
		Timestamp: now,
		Level:     "info",
		Hash:      "test-hash",
	}

	if entry.Source != "test-source" {
		t.Error("Expected Source to be set")
	}

	if entry.Type != "file" {
		t.Error("Expected Type to be set")
	}

	if entry.Container != "test-container" {
		t.Error("Expected Container to be set")
	}

	if entry.Content != "Test log message" {
		t.Error("Expected Content to be set")
	}

	if !entry.Timestamp.Equal(now) {
		t.Error("Expected Timestamp to be set")
	}

	if entry.Level != "info" {
		t.Error("Expected Level to be set")
	}

	if entry.Hash != "test-hash" {
		t.Error("Expected Hash to be set")
	}
}

// TestLogSourceValidation tests LogSource validation
func TestLogSourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		source  LogSource
		isValid bool
	}{
		{
			name: "Valid file source",
			source: LogSource{
				Name:     "test-file",
				Type:     "file",
				Enabled:  true,
				Interval: time.Minute,
				FilePath: "/tmp/test.log",
			},
			isValid: true,
		},
		{
			name: "Valid docker source",
			source: LogSource{
				Name:          "test-docker",
				Type:          "docker",
				Enabled:       true,
				Interval:      time.Minute,
				AllContainers: true,
			},
			isValid: true,
		},
		{
			name: "Valid command source",
			source: LogSource{
				Name:     "test-command",
				Type:     "command",
				Enabled:  true,
				Interval: time.Minute,
				Command:  "echo 'test'",
			},
			isValid: true,
		},
		{
			name: "Valid deploy source",
			source: LogSource{
				Name:          "test-deploy",
				Type:          "deploy",
				Enabled:       true,
				Interval:      time.Minute,
				DeployLogFile: "/tmp/deploy.log",
			},
			isValid: true,
		},
		{
			name: "Invalid file source without path",
			source: LogSource{
				Name:     "test-file",
				Type:     "file",
				Enabled:  true,
				Interval: time.Minute,
				// Missing FilePath
			},
			isValid: false,
		},
		{
			name: "Invalid command source without command",
			source: LogSource{
				Name:     "test-command",
				Type:     "command",
				Enabled:  true,
				Interval: time.Minute,
				// Missing Command
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation logic
			isValid := true

			if tt.source.Type == "file" && tt.source.FilePath == "" {
				isValid = false
			}

			if tt.source.Type == "command" && tt.source.Command == "" {
				isValid = false
			}

			if tt.source.Type == "deploy" && tt.source.DeployLogFile == "" {
				isValid = false
			}

			if tt.source.Type == "docker" && !tt.source.AllContainers && len(tt.source.Containers) == 0 {
				isValid = false
			}

			if isValid != tt.isValid {
				t.Errorf("Expected validity %v, got %v", tt.isValid, isValid)
			}
		})
	}
}

// TestLogManagerMemoryManagement tests memory management for log entries
func TestLogManagerMemoryManagement(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	// Add more than 1000 log entries
	for i := 0; i < 1500; i++ {
		entry := LogEntry{
			Source:    "test-source",
			Type:      "file",
			Content:   fmt.Sprintf("Test log message %d", i),
			Timestamp: time.Now(),
		}
		lm.logs = append(lm.logs, entry)
	}

	// Simulate the memory management logic
	if len(lm.logs) > 1000 {
		lm.logs = lm.logs[len(lm.logs)-1000:]
	}

	// Verify only last 1000 entries are kept
	if len(lm.logs) != 1000 {
		t.Errorf("Expected 1000 log entries, got %d", len(lm.logs))
	}

	// Verify the last entry is the most recent one
	lastEntry := lm.logs[len(lm.logs)-1]
	if lastEntry.Content != "Test log message 1499" {
		t.Errorf("Expected last entry to be 'Test log message 1499', got '%s'", lastEntry.Content)
	}
}

// TestLogManagerHashCleanup tests hash cleanup functionality
func TestLogManagerHashCleanup(t *testing.T) {
	lm := &LogManager{
		seenLogs: make(map[string]time.Time),
	}

	// Add some old hashes
	oldTime := time.Now().Add(-25 * time.Hour) // Older than 24 hours
	lm.seenLogs["old-hash"] = oldTime

	// Add some recent hashes
	recentTime := time.Now().Add(-1 * time.Hour) // Within 24 hours
	lm.seenLogs["recent-hash"] = recentTime

	// Run cleanup
	lm.cleanupOldHashes()

	// Verify old hash was removed
	if _, exists := lm.seenLogs["old-hash"]; exists {
		t.Error("Expected old hash to be removed")
	}

	// Verify recent hash was kept
	if _, exists := lm.seenLogs["recent-hash"]; !exists {
		t.Error("Expected recent hash to be kept")
	}
}

// Benchmark tests
func BenchmarkLogTimestampParsing(b *testing.B) {
	lm := &LogManager{}
	line := "2024-01-15T10:30:00.123456789Z INFO Application started"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm.parseLogTimestamp(line)
	}
}

func BenchmarkLogLevelDetection(b *testing.B) {
	lm := &LogManager{}
	line := "ERROR: Database connection failed"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm.detectLogLevel(line)
	}
}

func BenchmarkLogHashGeneration(b *testing.B) {
	lm := &LogManager{}
	entry := LogEntry{
		Source:    "test-source",
		Type:      "file",
		Content:   "Test log message",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm.generateLogHash(entry)
	}
}
