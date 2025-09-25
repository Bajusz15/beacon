package monitor

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestCommandLogCollection tests command-based log collection
func TestCommandLogCollection(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'Test log message'",
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
		t.Error("Expected command log collector to be created")
	}

	collector, exists := lm.logCollectors["test-command"]
	if !exists {
		t.Error("Expected collector for test-command to be created")
	}

	if collector == nil {
		t.Error("Expected collector to be non-nil")
	}
}

// TestCommandLogCollectionWithMultipleCommands tests command log collection with multiple commands
func TestCommandLogCollectionWithMultipleCommands(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-1",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'Command 1 output'",
			},
			{
				Name:     "test-command-2",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'Command 2 output'",
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
		t.Errorf("Expected 2 command log collectors, got %d", len(lm.logCollectors))
	}

	_, exists1 := lm.logCollectors["test-command-1"]
	if !exists1 {
		t.Error("Expected collector for test-command-1 to be created")
	}

	_, exists2 := lm.logCollectors["test-command-2"]
	if !exists2 {
		t.Error("Expected collector for test-command-2 to be created")
	}
}

// TestCommandLogCollectionWithComplexCommand tests command log collection with complex command
func TestCommandLogCollectionWithComplexCommand(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-complex",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'INFO: Application started' && echo 'ERROR: Database connection failed'",
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithSystemCommand tests command log collection with system command
func TestCommandLogCollectionWithSystemCommand(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-system",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "date", // System command that should work on most systems
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithFiltering tests command log collection with filtering
func TestCommandLogCollectionWithFiltering(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-command-filter",
				Type:            "command",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				Command:         "echo 'INFO: Service started' && echo 'ERROR: Critical failure' && echo 'DEBUG: Verbose output'",
				IncludePatterns: []string{"ERROR", "WARN"}, // Only include errors and warnings
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithExclusion tests command log collection with exclusion patterns
func TestCommandLogCollectionWithExclusion(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-command-exclude",
				Type:            "command",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				Command:         "echo 'INFO: Service started' && echo 'DEBUG: Verbose output' && echo 'ERROR: Critical failure'",
				ExcludePatterns: []string{"DEBUG"}, // Exclude debug logs
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithDeduplication tests command log collection with deduplication
func TestCommandLogCollectionWithDeduplication(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:        "test-command-dedup",
				Type:        "command",
				Enabled:     true,
				Interval:    time.Millisecond * 100,
				Command:     "echo 'INFO: Service started' && echo 'INFO: Service started'", // Duplicate output
				Deduplicate: true,
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithInvalidCommand tests command log collection with invalid command
func TestCommandLogCollectionWithInvalidCommand(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-invalid",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "nonexistent-command-that-should-fail", // Invalid command
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

	// Verify collector was created (should handle invalid commands gracefully)
	if len(lm.logCollectors) == 0 {
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithEmptyCommand tests command log collection with empty command
func TestCommandLogCollectionWithEmptyCommand(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-empty",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "", // Empty command
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

	// Verify collector was created (should handle empty commands gracefully)
	if len(lm.logCollectors) == 0 {
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithLongRunningCommand tests command log collection with long-running command
func TestCommandLogCollectionWithLongRunningCommand(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-long",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "sleep 0.1 && echo 'Long running command completed'", // Command that takes time
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithCommandWithOutput tests command log collection with command that produces output
func TestCommandLogCollectionWithCommandWithOutput(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-output",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo '2024-01-15T10:30:00Z INFO Application started'", // Command with timestamped output
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithCommandWithErrorOutput tests command log collection with command that produces error output
func TestCommandLogCollectionWithCommandWithErrorOutput(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-error",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'ERROR: Something went wrong' >&2", // Command with error output
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithCommandWithMultipleLines tests command log collection with command that produces multiple lines
func TestCommandLogCollectionWithCommandWithMultipleLines(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-multiline",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo -e 'Line 1\\nLine 2\\nLine 3'", // Command with multiple lines
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithCommandWithSpecialCharacters tests command log collection with command that produces special characters
func TestCommandLogCollectionWithCommandWithSpecialCharacters(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-special",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'Special chars: !@#$%^&*()_+-=[]{}|;:,.<>?'", // Command with special characters
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithCommandWithUnicode tests command log collection with command that produces unicode characters
func TestCommandLogCollectionWithCommandWithUnicode(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-unicode",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'Unicode: 你好世界 🌍'", // Command with unicode characters
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithCommandWithExitCode tests command log collection with command that has specific exit code
func TestCommandLogCollectionWithCommandWithExitCode(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-exit",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "echo 'Command with exit code' && exit 0", // Command with exit code
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
		t.Error("Expected command log collector to be created")
	}
}

// TestCommandLogCollectionWithCommandWithTimeout tests command log collection with command that times out
func TestCommandLogCollectionWithCommandWithTimeout(t *testing.T) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "test-command-timeout",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 100,
				Command:  "sleep 10", // Command that takes longer than test timeout
			},
		},
	}

	httpClient := &http.Client{}
	lm := NewLogManager(config, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()

	// Start log collection
	lm.StartLogCollection(ctx)

	// Wait for collection
	time.Sleep(time.Millisecond * 200)

	// Stop collection
	lm.StopLogCollection()

	// Verify collector was created
	if len(lm.logCollectors) == 0 {
		t.Error("Expected command log collector to be created")
	}
}

// Benchmark tests for command log collection
func BenchmarkCommandLogCollection(b *testing.B) {
	config := &Config{
		LogSources: []LogSource{
			{
				Name:     "bench-command",
				Type:     "command",
				Enabled:  true,
				Interval: time.Millisecond * 10,
				Command:  "echo 'Benchmark test'",
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
