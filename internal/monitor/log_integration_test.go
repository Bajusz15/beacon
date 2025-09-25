package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLogReportingIntegration tests the complete log reporting flow
func TestLogReportingIntegration(t *testing.T) {
	// Create a test server to receive log reports
	var receivedLogs []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/logs" && r.Method == "POST" {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Failed to decode log payload: %v", err)
				return
			}

			if logs, ok := payload["logs"].([]interface{}); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]interface{}); ok {
						receivedLogs = append(receivedLogs, logMap)
					}
				}
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content
	logContent := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z ERROR Database connection failed
2024-01-15T10:30:02Z WARN Memory usage high
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-integration",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 10,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Verify logs were reported
	if len(receivedLogs) == 0 {
		t.Error("Expected logs to be reported to server")
	}

	// Verify log content
	foundStart := false
	foundError := false
	foundWarn := false

	for _, log := range receivedLogs {
		if content, ok := log["content"].(string); ok {
			if strings.Contains(content, "Application started") {
				foundStart = true
			}
			if strings.Contains(content, "Database connection failed") {
				foundError = true
			}
			if strings.Contains(content, "Memory usage high") {
				foundWarn = true
			}
		}
	}

	if !foundStart {
		t.Error("Expected 'Application started' log to be reported")
	}

	if !foundError {
		t.Error("Expected 'Database connection failed' log to be reported")
	}

	if !foundWarn {
		t.Error("Expected 'Memory usage high' log to be reported")
	}
}

// TestLogReportingWithMultipleSources tests log reporting with multiple log sources
func TestLogReportingWithMultipleSources(t *testing.T) {
	// Create a test server to receive log reports
	var receivedLogs []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/logs" && r.Method == "POST" {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Failed to decode log payload: %v", err)
				return
			}

			if logs, ok := payload["logs"].([]interface{}); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]interface{}); ok {
						receivedLogs = append(receivedLogs, logMap)
					}
				}
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create temporary log files
	tempDir, err := os.MkdirTemp("", "beacon-log-multi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile1 := filepath.Join(tempDir, "app.log")
	logFile2 := filepath.Join(tempDir, "error.log")

	// Create log content
	logContent1 := `2024-01-15T10:30:00Z INFO Application started
2024-01-15T10:30:01Z INFO Service healthy
`
	logContent2 := `2024-01-15T10:30:00Z ERROR Database connection failed
2024-01-15T10:30:01Z ERROR Service unavailable
`

	err = os.WriteFile(logFile1, []byte(logContent1), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content 1: %v", err)
	}

	err = os.WriteFile(logFile2, []byte(logContent2), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content 2: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "app-logs",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile1,
				MaxLines: 10,
			},
			{
				Name:     "error-logs",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile2,
				MaxLines: 10,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Verify logs from both sources were reported
	if len(receivedLogs) == 0 {
		t.Error("Expected logs to be reported to server")
	}

	// Verify we have logs from both sources
	appLogs := 0
	errorLogs := 0

	for _, log := range receivedLogs {
		if source, ok := log["source"].(string); ok {
			if source == "app-logs" {
				appLogs++
			} else if source == "error-logs" {
				errorLogs++
			}
		}
	}

	if appLogs == 0 {
		t.Error("Expected logs from app-logs source to be reported")
	}

	if errorLogs == 0 {
		t.Error("Expected logs from error-logs source to be reported")
	}
}

// TestLogReportingWithFiltering tests log reporting with filtering
func TestLogReportingWithFiltering(t *testing.T) {
	// Create a test server to receive log reports
	var receivedLogs []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/logs" && r.Method == "POST" {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Failed to decode log payload: %v", err)
				return
			}

			if logs, ok := payload["logs"].([]interface{}); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]interface{}); ok {
						receivedLogs = append(receivedLogs, logMap)
					}
				}
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

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
2024-01-15T10:30:02Z ERROR Database connection failed
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
				Name:            "test-filter",
				Type:            "file",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				FilePath:        logFile,
				IncludePatterns: []string{"ERROR", "WARN"}, // Only include errors and warnings
				MaxLines:        10,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Verify only filtered logs were reported
	if len(receivedLogs) == 0 {
		t.Error("Expected filtered logs to be reported to server")
	}

	// Verify no INFO or DEBUG logs were reported
	for _, log := range receivedLogs {
		if content, ok := log["content"].(string); ok {
			if strings.Contains(content, "Application started") {
				t.Error("Expected INFO logs to be filtered out")
			}
			if strings.Contains(content, "Verbose output") {
				t.Error("Expected DEBUG logs to be filtered out")
			}
			if strings.Contains(content, "Service healthy") {
				t.Error("Expected INFO logs to be filtered out")
			}
		}
	}
}

// TestLogReportingWithDeduplication tests log reporting with deduplication
func TestLogReportingWithDeduplication(t *testing.T) {
	// Create a test server to receive log reports
	var receivedLogs []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/logs" && r.Method == "POST" {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Failed to decode log payload: %v", err)
				return
			}

			if logs, ok := payload["logs"].([]interface{}); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]interface{}); ok {
						receivedLogs = append(receivedLogs, logMap)
					}
				}
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

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
2024-01-15T10:30:02Z ERROR Database connection failed
2024-01-15T10:30:03Z ERROR Database connection failed
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:        "test-dedup",
				Type:        "file",
				Enabled:     true,
				Interval:    time.Millisecond * 100,
				FilePath:    logFile,
				Deduplicate: true,
				MaxLines:    10,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Verify logs were reported
	if len(receivedLogs) == 0 {
		t.Error("Expected logs to be reported to server")
	}

	// Count occurrences of each log message
	startCount := 0
	errorCount := 0

	for _, log := range receivedLogs {
		if content, ok := log["content"].(string); ok {
			if strings.Contains(content, "Application started") {
				startCount++
			}
			if strings.Contains(content, "Database connection failed") {
				errorCount++
			}
		}
	}

	// Verify duplicates were removed
	if startCount > 1 {
		t.Errorf("Expected 1 'Application started' log, got %d", startCount)
	}

	if errorCount > 1 {
		t.Errorf("Expected 1 'Database connection failed' log, got %d", errorCount)
	}
}

// TestLogReportingWithServerError tests log reporting when server returns error
func TestLogReportingWithServerError(t *testing.T) {
	// Create a test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-error-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content
	logContent := `2024-01-15T10:30:00Z INFO Application started
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-error",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 10,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Test should complete without crashing (error handling should be graceful)
}

// TestLogReportingWithInvalidURL tests log reporting with invalid URL
func TestLogReportingWithInvalidURL(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-invalid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content
	logContent := `2024-01-15T10:30:00Z INFO Application started
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-invalid",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 10,
			},
		},
		Report: ReportConfig{
			SendTo: "http://invalid-url-that-should-fail",
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Test should complete without crashing (error handling should be graceful)
}

// TestLogReportingWithMissingToken tests log reporting with missing token
func TestLogReportingWithMissingToken(t *testing.T) {
	// Create a test server to receive log reports
	var receivedLogs []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/logs" && r.Method == "POST" {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Failed to decode log payload: %v", err)
				return
			}

			if logs, ok := payload["logs"].([]interface{}); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]interface{}); ok {
						receivedLogs = append(receivedLogs, logMap)
					}
				}
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-no-token-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content
	logContent := `2024-01-15T10:30:00Z INFO Application started
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-no-token",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 10,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "", // Missing token
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Verify no logs were reported (due to missing token)
	if len(receivedLogs) > 0 {
		t.Error("Expected no logs to be reported due to missing token")
	}
}

// TestLogReportingWithEmptySendTo tests log reporting with empty SendTo
func TestLogReportingWithEmptySendTo(t *testing.T) {
	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-empty-sendto-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content
	logContent := `2024-01-15T10:30:00Z INFO Application started
`
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-empty-sendto",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 10,
			},
		},
		Report: ReportConfig{
			SendTo: "", // Empty SendTo
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Test should complete without crashing (no reporting should occur)
}

// TestLogReportingWithLargePayload tests log reporting with large payload
func TestLogReportingWithLargePayload(t *testing.T) {
	// Create a test server to receive log reports
	var receivedLogs []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/logs" && r.Method == "POST" {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Failed to decode log payload: %v", err)
				return
			}

			if logs, ok := payload["logs"].([]interface{}); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]interface{}); ok {
						receivedLogs = append(receivedLogs, logMap)
					}
				}
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-large-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create log content with many lines
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("2024-01-15T10:30:%02dZ INFO Log entry %d", i%60, i))
	}

	logContent := strings.Join(lines, "\n") + "\n"
	err = os.WriteFile(logFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-large",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				FilePath: logFile,
				MaxLines: 100,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "test-token",
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection and reporting
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Wait a bit more for async reporting
	time.Sleep(time.Millisecond * 100)

	// Verify logs were reported
	if len(receivedLogs) == 0 {
		t.Error("Expected logs to be reported to server")
	}

	// Verify we got a reasonable number of logs
	if len(receivedLogs) < 50 {
		t.Errorf("Expected at least 50 logs to be reported, got %d", len(receivedLogs))
	}
}

// Benchmark tests for log reporting
func BenchmarkLogReporting(b *testing.B) {
	// Create a test server to receive log reports
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a temporary log file
	tempDir, err := os.MkdirTemp("", "beacon-log-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

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
				Name:     "bench-log",
				Type:     "file",
				Enabled:  true,
				Interval: time.Millisecond * 10,
				FilePath: logFile,
				MaxLines: 100,
			},
		},
		Report: ReportConfig{
			SendTo: server.URL,
			Token:  "test-token",
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
