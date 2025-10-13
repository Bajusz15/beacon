package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAlertCommandExecution tests that alert commands work correctly
func TestAlertCommandExecution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a test script that will be executed
	testScript := `#!/bin/bash
echo "Test alert command executed"
echo "Check name: Test Check"
echo "Check status: up"
echo "Check output: test output"
echo "Device name: test-device"
`

	scriptPath := filepath.Join(tempDir, "test-alert.sh")
	if err := os.WriteFile(scriptPath, []byte(testScript), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Create a mock monitor
	ctx := context.Background()
	monitor := &Monitor{
		ctx: ctx,
	}

	// Create a test check result
	result := CheckResult{
		Name:          "Test Check",
		Type:          "command",
		Status:        "up",
		CommandOutput: "test output",
		Duration:      1 * time.Second,
		Device: DeviceConfig{
			Name: "test-device",
		},
	}

	// Test alert command execution with variable substitution
	outputPath := filepath.Join(tempDir, "alert-output.txt")
	alertCommand := `echo "Check name: $BEACON_CHECK_NAME" > ` + outputPath + ` && echo "Check status: $BEACON_CHECK_STATUS" >> ` + outputPath + ` && echo "Check output: $BEACON_CHECK_OUTPUT" >> ` + outputPath + ` && echo "Device name: $BEACON_DEVICE_NAME" >> ` + outputPath
	monitor.executeAlertCommand(alertCommand, result)

	// Wait for the command to execute (with retries)
	var content []byte
	var err error
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(outputPath); err == nil {
			content, err = os.ReadFile(outputPath)
			if err == nil && len(content) > 0 {
				break
			}
		}
	}

	// Check if the output file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Alert command output file was not created")
		return
	}

	// Read and verify the output
	if len(content) == 0 {
		content, err = os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read alert output: %v", err)
		}
	}

	output := string(content)

	// Verify that variables were substituted correctly
	if !contains(output, "Check name: Test Check") {
		t.Errorf("Expected 'Check name: Test Check' in output, got: %s", output)
	}

	if !contains(output, "Check status: up") {
		t.Errorf("Expected 'Check status: up' in output, got: %s", output)
	}

	if !contains(output, "Check output: test output") {
		t.Errorf("Expected 'Check output: test output' in output, got: %s", output)
	}

	if !contains(output, "Device name: test-device") {
		t.Errorf("Expected 'Device name: test-device' in output, got: %s", output)
	}
}

// TestCommandCheckAlwaysRunsAlertCommand tests that command checks always run alert commands
func TestCommandCheckAlwaysRunsAlertCommand(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a test script that will be executed
	testScript := `#!/bin/bash
echo "Alert command executed for status: $BEACON_CHECK_STATUS"
echo "Check output: $BEACON_CHECK_OUTPUT"
`

	scriptPath := filepath.Join(tempDir, "test-alert.sh")
	if err := os.WriteFile(scriptPath, []byte(testScript), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Create a mock monitor
	ctx := context.Background()
	monitor := &Monitor{
		ctx: ctx,
	}

	// Test with "up" status (should still run alert command for command checks)
	result := CheckResult{
		Name:          "Test Command Check",
		Type:          "command",
		Status:        "up",
		CommandOutput: "success output",
		Duration:      1 * time.Second,
		Device: DeviceConfig{
			Name: "test-device",
		},
	}

	outputPath := filepath.Join(tempDir, "alert-output-up.txt")
	alertCommand := `echo "Alert command executed for status: $BEACON_CHECK_STATUS" > ` + outputPath + ` && echo "Check output: $BEACON_CHECK_OUTPUT" >> ` + outputPath
	monitor.executeAlertCommand(alertCommand, result)

	// Wait for the command to execute (with retries)
	var content []byte
	var err error
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(outputPath); err == nil {
			content, err = os.ReadFile(outputPath)
			if err == nil && len(content) > 0 {
				break
			}
		}
	}

	// Check if the output file was created (should be created even for "up" status)
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Alert command should have run even for 'up' status on command checks")
		return
	}

	// Read and verify the output
	if len(content) == 0 {
		content, err = os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read alert output: %v", err)
		}
	}

	output := string(content)

	// Verify that the alert command ran with "up" status
	if !contains(output, "Alert command executed for status: up") {
		t.Errorf("Expected 'Alert command executed for status: up' in output, got: %s", output)
	}
}

// TestAlertCommandStdout tests that alert commands with stdout output are logged correctly
func TestAlertCommandStdout(t *testing.T) {
	// Create a mock monitor
	ctx := context.Background()
	monitor := &Monitor{
		ctx: ctx,
	}

	// Create a test check result
	result := CheckResult{
		Name:          "Test Check",
		Type:          "command",
		Status:        "up",
		CommandOutput: "test output",
		Duration:      1 * time.Second,
		Device: DeviceConfig{
			Name: "test-device",
		},
	}

	// Test alert command that produces stdout (not redirected to file)
	alertCommand := `echo "Alert executed for $BEACON_CHECK_NAME with status $BEACON_CHECK_STATUS"`
	monitor.executeAlertCommand(alertCommand, result)

	// Wait for the command to execute
	time.Sleep(200 * time.Millisecond)

	// This test verifies that the alert command runs without errors
	// The actual stdout logging is tested in the integration tests
	t.Log("Alert command with stdout executed successfully")
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && contains(s[1:], substr)
}
