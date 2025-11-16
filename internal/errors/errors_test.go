package errors

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestBeaconError_Error(t *testing.T) {
	tests := []struct {
		name          string
		errorType     ErrorType
		message       string
		originalError error
		expected      string
	}{
		{
			name:          "Error without original error",
			errorType:     ErrorTypeConfig,
			message:       "Configuration error",
			originalError: nil,
			expected:      "Configuration error",
		},
		{
			name:          "Error with original error",
			errorType:     ErrorTypeConnection,
			message:       "Connection failed",
			originalError: &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrNotExist},
			expected:      "Connection failed: dial tcp: file does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewBeaconError(tt.errorType, tt.message, tt.originalError)
			if err.Error() != tt.expected {
				t.Errorf("BeaconError.Error() = %v, want %v", err.Error(), tt.expected)
			}
		})
	}
}

func TestBeaconError_Unwrap(t *testing.T) {
	originalErr := &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrNotExist}
	err := NewBeaconError(ErrorTypeConnection, "Connection failed", originalErr)

	if err.Unwrap() != originalErr {
		t.Errorf("BeaconError.Unwrap() = %v, want %v", err.Unwrap(), originalErr)
	}
}

func TestBeaconError_WithTroubleshooting(t *testing.T) {
	err := NewBeaconError(ErrorTypeConfig, "Config error", nil)
	err = err.WithTroubleshooting("Step 1", "Step 2")

	expected := []string{"Step 1", "Step 2"}
	if len(err.Troubleshooting) != len(expected) {
		t.Errorf("Expected %d troubleshooting steps, got %d", len(expected), len(err.Troubleshooting))
	}

	for i, step := range expected {
		if err.Troubleshooting[i] != step {
			t.Errorf("Troubleshooting[%d] = %v, want %v", i, err.Troubleshooting[i], step)
		}
	}
}

func TestBeaconError_WithNextSteps(t *testing.T) {
	err := NewBeaconError(ErrorTypeConfig, "Config error", nil)
	err = err.WithNextSteps("Next 1", "Next 2")

	expected := []string{"Next 1", "Next 2"}
	if len(err.NextSteps) != len(expected) {
		t.Errorf("Expected %d next steps, got %d", len(expected), len(err.NextSteps))
	}

	for i, step := range expected {
		if err.NextSteps[i] != step {
			t.Errorf("NextSteps[%d] = %v, want %v", i, err.NextSteps[i], step)
		}
	}
}

func TestBeaconError_WithDocumentation(t *testing.T) {
	err := NewBeaconError(ErrorTypeConfig, "Config error", nil)
	err = err.WithDocumentation("https://example.com/docs")

	if err.Documentation != "https://example.com/docs" {
		t.Errorf("Documentation = %v, want %v", err.Documentation, "https://example.com/docs")
	}
}

func TestNewConnectionError(t *testing.T) {
	originalErr := &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrNotExist}
	err := NewConnectionError("localhost", 5432, originalErr)

	if err.Type != ErrorTypeConnection {
		t.Errorf("ErrorType = %v, want %v", err.Type, ErrorTypeConnection)
	}

	if err.Message != "Failed to connect to localhost:5432" {
		t.Errorf("Message = %v, want %v", err.Message, "Failed to connect to localhost:5432")
	}

	if err.OriginalError != originalErr {
		t.Errorf("OriginalError = %v, want %v", err.OriginalError, originalErr)
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Expected troubleshooting steps to be added")
	}

	if len(err.NextSteps) == 0 {
		t.Error("Expected next steps to be added")
	}

	if err.Documentation == "" {
		t.Error("Expected documentation link to be added")
	}
}

func TestNewHTTPError(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		statusCode  int
		originalErr error
		expectSteps bool
	}{
		{
			name:        "404 error",
			url:         "http://localhost:8080/api",
			statusCode:  404,
			originalErr: nil,
			expectSteps: true,
		},
		{
			name:        "500 error",
			url:         "http://localhost:8080/api",
			statusCode:  500,
			originalErr: nil,
			expectSteps: true,
		},
		{
			name:        "401 error",
			url:         "http://localhost:8080/api",
			statusCode:  401,
			originalErr: nil,
			expectSteps: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewHTTPError(tt.url, tt.statusCode, tt.originalErr)

			if err.Type != ErrorTypeNetwork {
				t.Errorf("ErrorType = %v, want %v", err.Type, ErrorTypeNetwork)
			}

			expectedMessage := fmt.Sprintf("HTTP request failed for %s (status: %d)", tt.url, tt.statusCode)
			if err.Message != expectedMessage {
				t.Errorf("Message = %v, want %v", err.Message, expectedMessage)
			}

			if tt.expectSteps && len(err.Troubleshooting) == 0 {
				t.Error("Expected troubleshooting steps to be added")
			}

			if tt.expectSteps && len(err.NextSteps) == 0 {
				t.Error("Expected next steps to be added")
			}
		})
	}
}

func TestNewConfigError(t *testing.T) {
	originalErr := os.ErrNotExist
	err := NewConfigError("/path/to/config.yml", originalErr)

	if err.Type != ErrorTypeConfig {
		t.Errorf("ErrorType = %v, want %v", err.Type, ErrorTypeConfig)
	}

	if err.Message != "Configuration error in /path/to/config.yml" {
		t.Errorf("Message = %v, want %v", err.Message, "Configuration error in /path/to/config.yml")
	}

	if err.OriginalError != originalErr {
		t.Errorf("OriginalError = %v, want %v", err.OriginalError, originalErr)
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Expected troubleshooting steps to be added")
	}

	if len(err.NextSteps) == 0 {
		t.Error("Expected next steps to be added")
	}
}

func TestNewPluginError(t *testing.T) {
	originalErr := os.ErrPermission
	err := NewPluginError("discord", originalErr)

	if err.Type != ErrorTypePlugin {
		t.Errorf("ErrorType = %v, want %v", err.Type, ErrorTypePlugin)
	}

	if err.Message != "Plugin 'discord' error" {
		t.Errorf("Message = %v, want %v", err.Message, "Plugin 'discord' error")
	}

	if err.OriginalError != originalErr {
		t.Errorf("OriginalError = %v, want %v", err.OriginalError, originalErr)
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Expected troubleshooting steps to be added")
	}

	if len(err.NextSteps) == 0 {
		t.Error("Expected next steps to be added")
	}
}

func TestNewFileError(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		originalErr error
		expectSteps bool
	}{
		{
			name:        "File not found",
			filePath:    "/path/to/file.txt",
			originalErr: os.ErrNotExist,
			expectSteps: true,
		},
		{
			name:        "Permission denied",
			filePath:    "/path/to/file.txt",
			originalErr: os.ErrPermission,
			expectSteps: true,
		},
		{
			name:        "Other file error",
			filePath:    "/path/to/file.txt",
			originalErr: os.ErrInvalid,
			expectSteps: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewFileError(tt.filePath, tt.originalErr)

			if err.Type != ErrorTypeFile {
				t.Errorf("ErrorType = %v, want %v", err.Type, ErrorTypeFile)
			}

			expectedMessage := "File operation failed for " + tt.filePath
			if err.Message != expectedMessage {
				t.Errorf("Message = %v, want %v", err.Message, expectedMessage)
			}

			if err.OriginalError != tt.originalErr {
				t.Errorf("OriginalError = %v, want %v", err.OriginalError, tt.originalErr)
			}

			if tt.expectSteps && len(err.Troubleshooting) == 0 {
				t.Error("Expected troubleshooting steps to be added")
			}

			if tt.expectSteps && len(err.NextSteps) == 0 {
				t.Error("Expected next steps to be added")
			}
		})
	}
}

func TestNewTimeoutError(t *testing.T) {
	originalErr := &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("timeout")}
	err := NewTimeoutError("HTTP request", 30*time.Second, originalErr)

	if err.Type != ErrorTypeTimeout {
		t.Errorf("ErrorType = %v, want %v", err.Type, ErrorTypeTimeout)
	}

	if err.Message != "Operation 'HTTP request' timed out after 30s" {
		t.Errorf("Message = %v, want %v", err.Message, "Operation 'HTTP request' timed out after 30s")
	}

	if err.OriginalError != originalErr {
		t.Errorf("OriginalError = %v, want %v", err.OriginalError, originalErr)
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Expected troubleshooting steps to be added")
	}

	if len(err.NextSteps) == 0 {
		t.Error("Expected next steps to be added")
	}
}

func TestNewAuthError(t *testing.T) {
	originalErr := os.ErrPermission
	err := NewAuthError("API", originalErr)

	if err.Type != ErrorTypeAuth {
		t.Errorf("ErrorType = %v, want %v", err.Type, ErrorTypeAuth)
	}

	if err.Message != "Authentication failed for API" {
		t.Errorf("Message = %v, want %v", err.Message, "Authentication failed for API")
	}

	if err.OriginalError != originalErr {
		t.Errorf("OriginalError = %v, want %v", err.OriginalError, originalErr)
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Expected troubleshooting steps to be added")
	}

	if len(err.NextSteps) == 0 {
		t.Error("Expected next steps to be added")
	}
}

func TestGetServiceName(t *testing.T) {
	tests := []struct {
		port     int
		expected string
	}{
		{22, "ssh"},
		{80, "nginx"},
		{443, "nginx"},
		{3306, "mysql"},
		{5432, "postgresql"},
		{6379, "redis"},
		{8080, "tomcat"},
		{9200, "elasticsearch"},
		{9999, "unknown-service"},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.port)), func(t *testing.T) {
			result := GetServiceName(tt.port)
			if result != tt.expected {
				t.Errorf("GetServiceName(%d) = %v, want %v", tt.port, result, tt.expected)
			}
		})
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains []string
	}{
		{
			name:     "BeaconError",
			err:      NewBeaconError(ErrorTypeConfig, "Test error", nil).WithTroubleshooting("Step 1").WithNextSteps("Next 1"),
			contains: []string{"❌ Error: Test error", "🔍 Possible causes:", "📋 Troubleshooting steps:"},
		},
		{
			name:     "Regular error",
			err:      os.ErrNotExist,
			contains: []string{"❌ Error:", "📋 General troubleshooting:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatError(tt.err)
			for _, expected := range tt.contains {
				if !contains(result, expected) {
					t.Errorf("FormatError() result does not contain %v", expected)
				}
			}
		})
	}
}

func TestBeaconError_FormatError(t *testing.T) {
	err := NewBeaconError(ErrorTypeConfig, "Test error", os.ErrNotExist).
		WithTroubleshooting("Cause 1", "Cause 2").
		WithNextSteps("Step 1", "Step 2").
		WithDocumentation("https://example.com/docs")

	result := err.FormatError()

	expectedParts := []string{
		"❌ Error: Test error",
		"Original error: file does not exist",
		"🔍 Possible causes:",
		"1. Cause 1",
		"2. Cause 2",
		"📋 Troubleshooting steps:",
		"1. Step 1",
		"2. Step 2",
		"📚 Documentation: https://example.com/docs",
	}

	for _, expected := range expectedParts {
		if !contains(result, expected) {
			t.Errorf("FormatError() result does not contain %v", expected)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
