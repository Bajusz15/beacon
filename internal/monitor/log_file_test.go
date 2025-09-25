package monitor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFileLogCollection tests file-based log collection
func TestFileLogCollection(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create initial log content
	initialContent := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z INFO Database connected
2024-01-15T10:30:02Z ERROR Connection timeout
`
	err = os.WriteFile(logFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write initial log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-file",
				Type:       "file",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				FilePath:   logFile,
				FollowFile: false,
				MaxLines:   10,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify logs were collected
	lm.logsMux.RLock()
	logCount := len(lm.logs)
	lm.logsMux.RUnlock()

	if logCount == 0 {
		t.Error("Expected logs to be collected")
	}

	// Verify log content
	lm.logsMux.RLock()
	foundStart := false
	foundError := false
	for _, log := range lm.logs {
		if strings.Contains(log.Content, "Application started") {
			foundStart = true
		}
		if strings.Contains(log.Content, "Connection timeout") {
			foundError = true
		}
	}
	lm.logsMux.RUnlock()

	if !foundStart {
		t.Error("Expected to find 'Application started' log")
	}

	if !foundError {
		t.Error("Expected to find 'Connection timeout' log")
	}
}

// TestFileLogCollectionWithTail tests file log collection using tail command
func TestFileLogCollectionWithTail(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-tail-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create initial log content
	initialContent := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z INFO Database connected
2024-01-15T10:30:02Z ERROR Connection timeout
`
	err = os.WriteFile(logFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write initial log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-file-tail",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				UseTail:  true, // Force tail command
				MaxLines: 10,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify logs were collected
	lm.logsMux.RLock()
	logCount := len(lm.logs)
	lm.logsMux.RUnlock()

	if logCount == 0 {
		t.Error("Expected logs to be collected with tail command")
	}
}

// TestFileLogCollectionWithFollow tests file log collection with follow mode
func TestFileLogCollectionWithFollow(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-follow-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create initial log content
	initialContent := `2024-01-15T10:30:00Z INFO Application started
`
	err = os.WriteFile(logFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write initial log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-file-follow",
				Type:       "file",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				FilePath:   logFile,
				FollowFile: true,
				MaxLines:   10,
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
	time.Sleep(time.Millisecond * 200)

	// Add new content to the log file
	newContent := `2024-01-15T10:30:01Z INFO New log entry
2024-01-15T10:30:02Z ERROR New error
`
	err = os.WriteFile(logFile, []byte(initialContent+newContent), 0644)
	if err != nil {
		t.Fatalf("Failed to append new log content: %v", err)
	}

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify logs were collected
	lm.logsMux.RLock()
	logCount := len(lm.logs)
	lm.logsMux.RUnlock()

	if logCount == 0 {
		t.Error("Expected logs to be collected in follow mode")
	}
}

// TestFileLogCollectionWithFiltering tests file log collection with filtering
func TestFileLogCollectionWithFiltering(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-filter-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content with various levels
	logContent := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z DEBUG Verbose output
2024-01-15T10:30:02Z ERROR Critical failure
2024-01-15T10:30:03Z WARN Memory usage high
2024-01-15T10:30:04Z INFO Service healthy
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-file-filter",
				Type:            "file",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				FilePath:        logFile,
				IncludePatterns: []string{"ERROR", "WARN"}, // Only include errors and warnings
				MaxLines:        10,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify only filtered logs were collected
	lm.logsMux.RLock()
	hasInfo := false
	hasError := false
	hasWarn := false
	for _, log := range lm.logs {
		if strings.Contains(log.Content, "Application started") {
			hasInfo = true
		}
		if strings.Contains(log.Content, "Critical failure") {
			hasError = true
		}
		if strings.Contains(log.Content, "Memory usage high") {
			hasWarn = true
		}
	}
	lm.logsMux.RUnlock()

	if hasInfo {
		t.Error("Expected INFO logs to be filtered out")
	}

	if !hasError {
		t.Error("Expected ERROR logs to be included")
	}

	if !hasWarn {
		t.Error("Expected WARN logs to be included")
	}
}

// TestFileLogCollectionWithExclusion tests file log collection with exclusion patterns
func TestFileLogCollectionWithExclusion(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-exclude-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content
	logContent := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z DEBUG Verbose output
2024-01-15T10:30:02Z ERROR Critical failure
2024-01-15T10:30:03Z INFO Service healthy
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-file-exclude",
				Type:            "file",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				FilePath:        logFile,
				ExcludePatterns: []string{"DEBUG"}, // Exclude debug logs
				MaxLines:        10,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify debug logs were excluded
	lm.logsMux.RLock()
	hasDebug := false
	for _, log := range lm.logs {
		if strings.Contains(log.Content, "Verbose output") {
			hasDebug = true
		}
	}
	lm.logsMux.RUnlock()

	if hasDebug {
		t.Error("Expected DEBUG logs to be excluded")
	}
}

// TestFileLogCollectionWithDeduplication tests file log collection with deduplication
func TestFileLogCollectionWithDeduplication(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-dedup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content with duplicates
	logContent := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z INFO Application started
2024-01-15T10:30:02Z ERROR Critical failure
2024-01-15T10:30:03Z ERROR Critical failure
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:        "test-file-dedup",
				Type:        "file",
				Enabled:     true,
				Interval:    time.Millisecond * 100,
				FilePath:    logFile,
				Deduplicate: true,
				MaxLines:    10,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify duplicates were removed
	lm.logsMux.RLock()
	startCount := 0
	errorCount := 0
	for _, log := range lm.logs {
		if strings.Contains(log.Content, "Application started") {
			startCount++
		}
		if strings.Contains(log.Content, "Critical failure") {
			errorCount++
		}
	}
	lm.logsMux.RUnlock()

	if startCount > 1 {
		t.Errorf("Expected 1 'Application started' log, got %d", startCount)
	}

	if errorCount > 1 {
		t.Errorf("Expected 1 'Critical failure' log, got %d", errorCount)
	}
}

// TestFileLogCollectionWithNonExistentFile tests file log collection with non-existent file
func TestFileLogCollectionWithNonExistentFile(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-file-nonexistent",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: "/tmp/nonexistent.log",
				MaxLines: 10,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify no logs were collected
	lm.logsMux.RLock()
	logCount := len(lm.logs)
	lm.logsMux.RUnlock()

	if logCount > 0 {
		t.Error("Expected no logs to be collected from non-existent file")
	}
}

// TestFileLogCollectionWithPermissionDenied tests file log collection with permission denied
func TestFileLogCollectionWithPermissionDenied(t *testing.T) {
	// Create a temporary log file with restricted permissions
	tempDir, err := os.MkdirTemp("", "beacon-log-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "restricted.log")

	// Create file with no read permissions
	err = os.WriteFile(logFile, []byte("test content"), 0000)
	if err != nil {
		t.Fatalf("Failed to create restricted file: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-file-perm",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 10,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify no logs were collected due to permission denied
	lm.logsMux.RLock()
	logCount := len(lm.logs)
	lm.logsMux.RUnlock()

	if logCount > 0 {
		t.Error("Expected no logs to be collected due to permission denied")
	}
}

// TestFileLogCollectionWithLargeFile tests file log collection with large file
func TestFileLogCollectionWithLargeFile(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-large-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "large.log")

	// Create a large log file (simulate with many lines)
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, fmt.Sprintf("2024-01-15T10:30:%02dZ INFO Log entry %d", i%60, i))
	}

	logContent := strings.Join(lines, "\n") + "\n"
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write large log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-file-large",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 50, // Limit to 50 lines
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify logs were collected (should be limited by MaxLines)
	lm.logsMux.RLock()
	logCount := len(lm.logs)
	lm.logsMux.RUnlock()

	if logCount == 0 {
		t.Error("Expected logs to be collected from large file")
	}

	// Verify we didn't collect all 1000 lines
	if logCount >= 1000 {
		t.Error("Expected log collection to be limited by MaxLines")
	}
}

// TestFileLogCollectionWithRotation tests file log collection with log rotation
func TestFileLogCollectionWithRotation(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-rotation-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "rotated.log")

	// Create initial log content
	initialContent := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z INFO Database connected
`
	err = os.WriteFile(logFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write initial log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-file-rotation",
				Type:       "file",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				FilePath:   logFile,
				FollowFile: true,
				MaxLines:   10,
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
	time.Sleep(time.Millisecond * 200)

	// Simulate log rotation by truncating the file
	err = os.WriteFile(logFile, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to truncate log file: %v", err)
	}

	// Add new content after rotation
	newContent := `2024-01-15T10:31:00Z INFO Application restarted
2024-01-15T10:31:01Z INFO Database reconnected
`
	err = os.WriteFile(logFile, []byte(newContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write new log content: %v", err)
	}

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify logs were collected after rotation
	lm.logsMux.RLock()
	logCount := len(lm.logs)
	hasRestarted := false
	for _, log := range lm.logs {
		if strings.Contains(log.Content, "Application restarted") {
			hasRestarted = true
		}
	}
	lm.logsMux.RUnlock()

	if logCount == 0 {
		t.Error("Expected logs to be collected after rotation")
	}

	if !hasRestarted {
		t.Error("Expected to find 'Application restarted' log after rotation")
	}
}

// Benchmark tests for file log collection
func BenchmarkFileLogCollection(b *testing.B) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "bench.log")

	// Create log content
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("2024-01-15T10:30:%02dZ INFO Log entry %d", i%60, i))
	}

	logContent := strings.Join(lines, "\n") + "\n"
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		b.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "bench-file",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 10,
				FilePath: logFile,
				MaxLines: 100,
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
		lm.StartLogCollection(ctx)
		time.Sleep(time.Millisecond * 10)
		lm.StopLogCollection()
		cancel()
	}
}
