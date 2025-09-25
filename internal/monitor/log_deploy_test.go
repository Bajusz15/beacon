package monitor

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeployLogCollection tests deploy log collection
func TestDeployLogCollection(t *testing.T) {
	// Create a temporary deploy log file
	tempDir, err := os.MkdirTemp("", "beacon-deploy-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "deploy.log")

	// Create initial deploy log content
	initialContent := `2024-01-15T10:30:00Z INFO Deploy started
2024-01-15T10:30:01Z INFO Building application
2024-01-15T10:30:02Z ERROR Build failed
`
	err = os.WriteFile(deployLogFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write initial deploy log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: deployLogFile,
				MaxLines:     10,
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

	// Verify collector was created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}

	collector, exists := lm.logCollectors["test-deploy"]
	if !exists {
		t.Error("Expected collector for test-deploy to be created")
	}

	if collector == nil {
		t.Error("Expected collector to be non-nil")
	}
}

// TestDeployLogCollectionWithNonExistentFile tests deploy log collection with non-existent file
func TestDeployLogCollectionWithNonExistentFile(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy-nonexistent",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: "/tmp/nonexistent-deploy.log",
				MaxLines:     10,
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

	// Verify collector was created (should handle non-existent files gracefully)
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithEmptyFile tests deploy log collection with empty file
func TestDeployLogCollectionWithEmptyFile(t *testing.T) {
	// Create a temporary deploy log file
	tempDir, err := os.MkdirTemp("", "beacon-deploy-empty-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "empty-deploy.log")

	// Create empty file
	err = os.WriteFile(deployLogFile, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to create empty deploy log file: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy-empty",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: deployLogFile,
				MaxLines:     10,
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

	// Verify collector was created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithFiltering tests deploy log collection with filtering
func TestDeployLogCollectionWithFiltering(t *testing.T) {
	// Create a temporary deploy log file
	tempDir, err := os.MkdirTemp("", "beacon-deploy-filter-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "filter-deploy.log")

	// Create deploy log content with various levels
	logContent := `2024-01-15T10:30:00Z INFO Deploy started
2024-01-15T10:30:01Z DEBUG Verbose output
2024-01-15T10:30:02Z ERROR Build failed
2024-01-15T10:30:03Z WARN Memory usage high
2024-01-15T10:30:04Z INFO Deploy completed
`
	err = os.WriteFile(deployLogFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write deploy log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-deploy-filter",
				Type:            "deploy",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				DeployLogFile:   deployLogFile,
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

	// Verify collector was created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithExclusion tests deploy log collection with exclusion patterns
func TestDeployLogCollectionWithExclusion(t *testing.T) {
	// Create a temporary deploy log file
	tempDir, err := os.MkdirTemp("", "beacon-deploy-exclude-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "exclude-deploy.log")

	// Create deploy log content
	logContent := `2024-01-15T10:30:00Z INFO Deploy started
2024-01-15T10:30:01Z DEBUG Verbose output
2024-01-15T10:30:02Z ERROR Build failed
2024-01-15T10:30:03Z INFO Deploy completed
`
	err = os.WriteFile(deployLogFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write deploy log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-deploy-exclude",
				Type:            "deploy",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				DeployLogFile:   deployLogFile,
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

	// Verify collector was created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithDeduplication tests deploy log collection with deduplication
func TestDeployLogCollectionWithDeduplication(t *testing.T) {
	// Create a temporary deploy log file
	tempDir, err := os.MkdirTemp("", "beacon-deploy-dedup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "dedup-deploy.log")

	// Create deploy log content with duplicates
	logContent := `2024-01-15T10:30:00Z INFO Deploy started
2024-01-15T10:30:01Z INFO Deploy started
2024-01-15T10:30:02Z ERROR Build failed
2024-01-15T10:30:03Z ERROR Build failed
`
	err = os.WriteFile(deployLogFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write deploy log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy-dedup",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: deployLogFile,
				Deduplicate:  true,
				MaxLines:     10,
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

	// Verify collector was created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithLargeFile tests deploy log collection with large file
func TestDeployLogCollectionWithLargeFile(t *testing.T) {
	// Create a temporary deploy log file
	tempDir, err := os.MkdirTemp("", "beacon-deploy-large-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "large-deploy.log")

	// Create a large deploy log file (simulate with many lines)
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, "2024-01-15T10:30:00Z INFO Deploy step "+string(rune(i)))
	}

	logContent := strings.Join(lines, "\n") + "\n"
	err = os.WriteFile(deployLogFile, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write large deploy log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy-large",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: deployLogFile,
				MaxLines:     50, // Limit to 50 lines
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

	// Verify collector was created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithPermissionDenied tests deploy log collection with permission denied
func TestDeployLogCollectionWithPermissionDenied(t *testing.T) {
	// Create a temporary deploy log file with restricted permissions
	tempDir, err := os.MkdirTemp("", "beacon-deploy-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "restricted-deploy.log")

	// Create file with no read permissions
	err = os.WriteFile(deployLogFile, []byte("test content"), 0000)
	if err != nil {
		t.Fatalf("Failed to create restricted deploy log file: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy-perm",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: deployLogFile,
				MaxLines:     10,
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

	// Verify collector was created (should handle permission denied gracefully)
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithEmptyDeployLogFile tests deploy log collection with empty deploy log file path
func TestDeployLogCollectionWithEmptyDeployLogFile(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy-empty-path",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: "", // Empty deploy log file path
				MaxLines:     10,
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

	// Verify collector was created (should handle empty path gracefully)
	if len(lm.logCollectors) == 0 {
		t.Error("Expected deploy log collector to be created")
	}
}

// TestDeployLogCollectionWithMultipleDeployLogs tests deploy log collection with multiple deploy log sources
func TestDeployLogCollectionWithMultipleDeployLogs(t *testing.T) {
	// Create temporary deploy log files
	tempDir, err := os.MkdirTemp("", "beacon-deploy-multiple-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile1 := filepath.Join(tempDir, "deploy1.log")
	deployLogFile2 := filepath.Join(tempDir, "deploy2.log")

	// Create deploy log content
	logContent1 := `2024-01-15T10:30:00Z INFO Deploy 1 started
2024-01-15T10:30:01Z INFO Deploy 1 completed
`
	logContent2 := `2024-01-15T10:30:00Z INFO Deploy 2 started
2024-01-15T10:30:01Z INFO Deploy 2 completed
`

	err = os.WriteFile(deployLogFile1, []byte(logContent1), 0644)
	if err != nil {
		t.Fatalf("Failed to write deploy log content 1: %v", err)
	}

	err = os.WriteFile(deployLogFile2, []byte(logContent2), 0644)
	if err != nil {
		t.Fatalf("Failed to write deploy log content 2: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "test-deploy-1",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: deployLogFile1,
				MaxLines:     10,
			},
			{
				Name:         "test-deploy-2",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 100,
				DeployLogFile: deployLogFile2,
				MaxLines:     10,
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

	// Verify both collectors were created
	if len(lm.logCollectors) != 2 {
		t.Errorf("Expected 2 deploy log collectors, got %d", len(lm.logCollectors))
	}

	_, exists1 := lm.logCollectors["test-deploy-1"]
	if !exists1 {
		t.Error("Expected collector for test-deploy-1 to be created")
	}

	_, exists2 := lm.logCollectors["test-deploy-2"]
	if !exists2 {
		t.Error("Expected collector for test-deploy-2 to be created")
	}
}

// Benchmark tests for deploy log collection
func BenchmarkDeployLogCollection(b *testing.B) {
	// Create a temporary deploy log file
	tempDir, err := os.MkdirTemp("", "beacon-deploy-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deployLogFile := filepath.Join(tempDir, "bench-deploy.log")

	// Create deploy log content
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "2024-01-15T10:30:00Z INFO Deploy step "+string(rune(i)))
	}

	logContent := strings.Join(lines, "\n") + "\n"
	err = os.WriteFile(deployLogFile, []byte(logContent), 0644)
	if err != nil {
		b.Fatalf("Failed to write deploy log content: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:         "bench-deploy",
				Type:         "deploy",
				Enabled:      true,
				Interval:     time.Millisecond * 10,
				DeployLogFile: deployLogFile,
				MaxLines:     100,
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
