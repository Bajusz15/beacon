package monitor

import (
	"context"
	"net/http"
	"os/exec"
	"testing"
	"time"
)

// TestDockerLogCollection tests Docker container log collection
func TestDockerLogCollection(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:          "test-docker",
				Type:          "docker",
				Enabled:       true,
				Interval:      time.Millisecond * 100,
				AllContainers: true,
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
		t.Error("Expected Docker log collector to be created")
	}

	collector, exists := lm.logCollectors["test-docker"]
	if !exists {
		t.Error("Expected collector for test-docker to be created")
	}

	if collector == nil {
		t.Error("Expected collector to be non-nil")
	}
}

// TestDockerLogCollectionWithSpecificContainers tests Docker log collection with specific containers
func TestDockerLogCollectionWithSpecificContainers(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	// Create a test container
	containerName := "beacon-test-container"
	err := createTestContainer(containerName)
	if err != nil {
		t.Fatalf("Failed to create test container: %v", err)
	}
	defer removeTestContainer(containerName)

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-docker-specific",
				Type:       "docker",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				Containers: []string{containerName},
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
		t.Error("Expected Docker log collector to be created")
	}

	collector, exists := lm.logCollectors["test-docker-specific"]
	if !exists {
		t.Error("Expected collector for test-docker-specific to be created")
	}

	if collector == nil {
		t.Error("Expected collector to be non-nil")
	}
}

// TestDockerLogCollectionWithOptions tests Docker log collection with additional options
func TestDockerLogCollectionWithOptions(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:          "test-docker-options",
				Type:          "docker",
				Enabled:       true,
				Interval:      time.Millisecond * 100,
				AllContainers: true,
				DockerOptions: "--details", // Additional Docker options
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
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithFiltering tests Docker log collection with filtering
func TestDockerLogCollectionWithFiltering(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-docker-filter",
				Type:            "docker",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				AllContainers:   true,
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
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithExclusion tests Docker log collection with exclusion patterns
func TestDockerLogCollectionWithExclusion(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:            "test-docker-exclude",
				Type:            "docker",
				Enabled:         true,
				Interval:        time.Millisecond * 100,
				AllContainers:   true,
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
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithDeduplication tests Docker log collection with deduplication
func TestDockerLogCollectionWithDeduplication(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:          "test-docker-dedup",
				Type:          "docker",
				Enabled:       true,
				Interval:      time.Millisecond * 100,
				AllContainers: true,
				Deduplicate:   true,
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
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithNonExistentContainer tests Docker log collection with non-existent container
func TestDockerLogCollectionWithNonExistentContainer(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-docker-nonexistent",
				Type:       "docker",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				Containers: []string{"nonexistent-container"},
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

	// Verify collector was created (should handle non-existent containers gracefully)
	if len(lm.logCollectors) == 0 {
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithEmptyContainerList tests Docker log collection with empty container list
func TestDockerLogCollectionWithEmptyContainerList(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-docker-empty",
				Type:       "docker",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				Containers: []string{}, // Empty container list
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
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithInvalidOptions tests Docker log collection with invalid options
func TestDockerLogCollectionWithInvalidOptions(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:          "test-docker-invalid",
				Type:          "docker",
				Enabled:       true,
				Interval:      time.Millisecond * 100,
				AllContainers: true,
				DockerOptions: "--invalid-option", // Invalid Docker option
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

	// Verify collector was created (should handle invalid options gracefully)
	if len(lm.logCollectors) == 0 {
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithConflictingOptions tests Docker log collection with conflicting options
func TestDockerLogCollectionWithConflictingOptions(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:          "test-docker-conflict",
				Type:          "docker",
				Enabled:       true,
				Interval:      time.Millisecond * 100,
				AllContainers: true,
				DockerOptions: "--tail 100 --since 2024-01-01", // Conflicting options that should be filtered
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
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithMultipleContainers tests Docker log collection with multiple containers
func TestDockerLogCollectionWithMultipleContainers(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	// Create test containers
	container1 := "beacon-test-container-1"
	container2 := "beacon-test-container-2"

	err := createTestContainer(container1)
	if err != nil {
		t.Fatalf("Failed to create test container 1: %v", err)
	}
	defer removeTestContainer(container1)

	err = createTestContainer(container2)
	if err != nil {
		t.Fatalf("Failed to create test container 2: %v", err)
	}
	defer removeTestContainer(container2)

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-docker-multiple",
				Type:       "docker",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				Containers: []string{container1, container2},
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
		t.Error("Expected Docker log collector to be created")
	}
}

// TestDockerLogCollectionWithStoppedContainer tests Docker log collection with stopped container
func TestDockerLogCollectionWithStoppedContainer(t *testing.T) {
	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping Docker log collection tests")
	}

	// Create and stop a test container
	containerName := "beacon-test-stopped"
	err := createTestContainer(containerName)
	if err != nil {
		t.Fatalf("Failed to create test container: %v", err)
	}
	defer removeTestContainer(containerName)

	// Stop the container
	err = exec.Command("docker", "stop", containerName).Run()
	if err != nil {
		t.Fatalf("Failed to stop test container: %v", err)
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:       "test-docker-stopped",
				Type:       "docker",
				Enabled:    true,
				Interval:   time.Millisecond * 100,
				Containers: []string{containerName},
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
		t.Error("Expected Docker log collector to be created")
	}
}

// Helper functions for Docker tests

// isDockerAvailable checks if Docker is available
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}

// createTestContainer creates a test Docker container
func createTestContainer(name string) error {
	// Create a simple test container that runs for a short time
	cmd := exec.Command("docker", "run", "-d", "--name", name, "alpine", "sleep", "30")
	return cmd.Run()
}

// removeTestContainer removes a test Docker container
func removeTestContainer(name string) error {
	// Force remove the container
	cmd := exec.Command("docker", "rm", "-f", name)
	return cmd.Run()
}

// Benchmark tests for Docker log collection
func BenchmarkDockerLogCollection(b *testing.B) {
	// Check if Docker is available
	if !isDockerAvailable() {
		b.Skip("Docker not available, skipping Docker log collection benchmark")
	}

	config := &Config{
		LogSources: []LogSource{
			{
				Name:          "bench-docker",
				Type:          "docker",
				Enabled:       true,
				Interval:      time.Millisecond * 10,
				AllContainers: true,
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
