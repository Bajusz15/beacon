package errors

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// ErrorType represents different categories of errors
type ErrorType string

const (
	ErrorTypeConnection ErrorType = "connection"
	ErrorTypeConfig     ErrorType = "configuration"
	ErrorTypePlugin     ErrorType = "plugin"
	ErrorTypeSystem     ErrorType = "system"
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeAuth       ErrorType = "authentication"
	ErrorTypeTimeout    ErrorType = "timeout"
	ErrorTypeFile       ErrorType = "file"
)

// BeaconError represents a structured error with troubleshooting information
type BeaconError struct {
	Type            ErrorType
	Message         string
	OriginalError   error
	Troubleshooting []string
	NextSteps       []string
	Documentation   string
	Timestamp       time.Time
}

// Error implements the error interface
func (e *BeaconError) Error() string {
	if e.OriginalError != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.OriginalError)
	}
	return e.Message
}

// Unwrap returns the original error for error unwrapping
func (e *BeaconError) Unwrap() error {
	return e.OriginalError
}

// FormatError formats the error with troubleshooting information
func (e *BeaconError) FormatError() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "❌ Error: %s\n", e.Message)

	if e.OriginalError != nil {
		fmt.Fprintf(&sb, "   Original error: %v\n", e.OriginalError)
	}

	if len(e.Troubleshooting) > 0 {
		sb.WriteString("\n🔍 Possible causes:\n")
		for i, step := range e.Troubleshooting {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, step)
		}
	}

	if len(e.NextSteps) > 0 {
		sb.WriteString("\n📋 Troubleshooting steps:\n")
		for i, step := range e.NextSteps {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, step)
		}
	}

	if e.Documentation != "" {
		fmt.Fprintf(&sb, "\n📚 Documentation: %s\n", e.Documentation)
	}

	return sb.String()
}

// NewBeaconError creates a new BeaconError
func NewBeaconError(errorType ErrorType, message string, originalError error) *BeaconError {
	return &BeaconError{
		Type:          errorType,
		Message:       message,
		OriginalError: originalError,
		Timestamp:     time.Now(),
	}
}

// WithTroubleshooting adds troubleshooting information to the error
func (e *BeaconError) WithTroubleshooting(steps ...string) *BeaconError {
	e.Troubleshooting = append(e.Troubleshooting, steps...)
	return e
}

// WithNextSteps adds next steps to the error
func (e *BeaconError) WithNextSteps(steps ...string) *BeaconError {
	e.NextSteps = append(e.NextSteps, steps...)
	return e
}

// WithDocumentation adds documentation link to the error
func (e *BeaconError) WithDocumentation(url string) *BeaconError {
	e.Documentation = url
	return e
}

// Connection Errors

// NewConnectionError creates a connection error with troubleshooting info
func NewConnectionError(host string, port int, originalError error) *BeaconError {
	errorType := ErrorTypeConnection
	message := fmt.Sprintf("Failed to connect to %s:%d", host, port)

	err := NewBeaconError(errorType, message, originalError)

	// Add troubleshooting based on error type
	if netErr, ok := originalError.(*net.OpError); ok {
		if netErr.Timeout() {
			err.WithTroubleshooting(
				"Connection timeout - service may be slow to respond",
				"Network connectivity issues",
			).WithNextSteps(
				fmt.Sprintf("Verify port is open: netstat -tulpn | grep %d", port),
				fmt.Sprintf("Test connection: telnet %s %d", host, port),
				"Check firewall rules: ufw status or iptables -L",
			)
		} else if netErr.Op == "dial" {
			err.WithTroubleshooting(
				"Service is not running",
				"Wrong host or port number",
				"Network routing issues",
			).WithNextSteps(
				fmt.Sprintf("Verify port: netstat -tulpn | grep %d", port),
				fmt.Sprintf("Test connection: telnet %s %d", host, port),
				"Check service configuration",
			)
		}
	}

	return err.WithDocumentation("https://github.com/Bajusz15/beacon#troubleshooting-connection-errors")
}

// HTTP Errors

// NewHTTPError creates an HTTP error with troubleshooting info
func NewHTTPError(url string, statusCode int, originalError error) *BeaconError {
	errorType := ErrorTypeNetwork
	message := fmt.Sprintf("HTTP request failed for %s (status: %d)", url, statusCode)

	err := NewBeaconError(errorType, message, originalError)

	switch statusCode {
	case 404:
		err.WithTroubleshooting(
			"URL endpoint does not exist",
			"Service is not properly configured",
			"Wrong URL path",
		).WithNextSteps(
			"Verify the URL is correct",
			"Check if the service is running",
			"Test with curl: curl -v "+url,
		)
	case 500, 502, 503, 504:
		err.WithTroubleshooting(
			"Server internal error",
			"Service is overloaded",
			"Backend service is down",
		).WithNextSteps(
			"Check server logs for errors",
			"Verify backend services are running",
			"Check server resources (CPU, memory)",
		)
	case 401, 403:
		err.WithTroubleshooting(
			"Authentication required",
			"Insufficient permissions",
			"Invalid credentials",
		).WithNextSteps(
			"Check authentication configuration",
			"Verify API keys or tokens",
			"Check user permissions",
		)
	default:
		err.WithTroubleshooting(
			"Unexpected HTTP response",
			"Service may be misconfigured",
			"Network connectivity issues",
		).WithNextSteps(
			"Check service logs",
			"Verify service configuration",
			"Test with curl: curl -v "+url,
		)
	}

	return err.WithDocumentation("https://github.com/Bajusz15/beacon#troubleshooting-http-checks")
}

// Configuration Errors

// NewConfigError creates a configuration error with troubleshooting info
func NewConfigError(configPath string, originalError error) *BeaconError {
	errorType := ErrorTypeConfig
	message := fmt.Sprintf("Configuration error in %s", configPath)

	err := NewBeaconError(errorType, message, originalError)

	err = err.WithTroubleshooting(
		"Invalid YAML syntax",
		"Missing required fields",
		"Invalid field values",
		"File permissions issue",
	).WithNextSteps(
		"Validate YAML syntax with online validator",
		"Check file permissions: ls -la "+configPath,
		"Compare with example configuration",
		"Run configuration wizard: beacon setup-wizard",
	)

	err = err.WithDocumentation("https://github.com/Bajusz15/beacon#configuration-reference")
	return err
}

// Plugin Errors

// NewPluginError creates a plugin error with troubleshooting info
func NewPluginError(pluginName string, originalError error) *BeaconError {
	errorType := ErrorTypePlugin
	message := fmt.Sprintf("Plugin '%s' error", pluginName)

	err := NewBeaconError(errorType, message, originalError)

	err.WithTroubleshooting(
		"Plugin configuration is invalid",
		"Missing required credentials",
		"Plugin service is unavailable",
		"Network connectivity issues",
	).WithNextSteps(
		fmt.Sprintf("Check plugin configuration for '%s'", pluginName),
		"Verify environment variables are set",
		"Test plugin connectivity manually",
		"Check plugin logs for detailed errors",
	)

	err.WithDocumentation("https://github.com/Bajusz15/beacon#plugin-configuration")
	return err
}

// File Errors

// NewFileError creates a file error with troubleshooting info
func NewFileError(filePath string, originalError error) *BeaconError {
	errorType := ErrorTypeFile
	message := fmt.Sprintf("File operation failed for %s", filePath)

	err := NewBeaconError(errorType, message, originalError)

	if os.IsNotExist(originalError) {
		err.WithTroubleshooting(
			"File does not exist",
			"Wrong file path",
			"File was deleted",
		).WithNextSteps(
			"Check if file exists: ls -la "+filePath,
			"Verify the file path is correct",
			"Create the file if it should exist",
		)
	} else if os.IsPermission(originalError) {
		err.WithTroubleshooting(
			"Insufficient file permissions",
			"File is owned by different user",
			"Directory permissions issue",
		).WithNextSteps(
			"Check file permissions: ls -la "+filePath,
			"Fix permissions: chmod 644 "+filePath,
			"Check directory permissions",
		)
	} else {
		err.WithTroubleshooting(
			"File system error",
			"Disk space issue",
			"File is locked by another process",
		).WithNextSteps(
			"Check disk space: df -h",
			"Check if file is in use: lsof "+filePath,
			"Restart the service if needed",
		)
	}

	err.WithDocumentation("https://github.com/Bajusz15/beacon#troubleshooting-file-errors")
	return err
}

// Timeout Errors

// NewTimeoutError creates a timeout error with troubleshooting info
func NewTimeoutError(operation string, timeout time.Duration, originalError error) *BeaconError {
	errorType := ErrorTypeTimeout
	message := fmt.Sprintf("Operation '%s' timed out after %v", operation, timeout)

	err := NewBeaconError(errorType, message, originalError)

	err.WithTroubleshooting(
		"Service is slow to respond",
		"Network latency issues",
		"Service is overloaded",
		"Timeout value is too low",
	).WithNextSteps(
		"Increase timeout value in configuration",
		"Check service performance metrics",
		"Test network connectivity",
		"Check service logs for errors",
	)

	err.WithDocumentation("https://github.com/Bajusz15/beacon#troubleshooting-timeouts")
	return err
}

// Authentication Errors

// NewAuthError creates an authentication error with troubleshooting info
func NewAuthError(service string, originalError error) *BeaconError {
	errorType := ErrorTypeAuth
	message := fmt.Sprintf("Authentication failed for %s", service)

	err := NewBeaconError(errorType, message, originalError)

	err.WithTroubleshooting(
		"Invalid credentials",
		"Expired token or key",
		"Account is locked or disabled",
		"Wrong authentication method",
	).WithNextSteps(
		"Verify credentials are correct",
		"Check if token/key has expired",
		"Test credentials manually",
		"Contact service administrator",
	)

	err.WithDocumentation("https://github.com/Bajusz15/beacon#troubleshooting-authentication")
	return err
}

// FormatError formats any error with enhanced information
func FormatError(err error) string {
	if beaconErr, ok := err.(*BeaconError); ok {
		return beaconErr.FormatError()
	}

	// Try to wrap common error types
	if netErr, ok := err.(*net.OpError); ok {
		if netErr.Op == "dial" {
			// Extract host and port from the error
			addr := netErr.Addr
			if addr != nil {
				host, port, _ := net.SplitHostPort(addr.String())
				if port != "" {
					// This is a simplified version - in practice you'd parse the port
					return NewConnectionError(host, 0, err).FormatError()
				}
			}
		}
	}

	// Default formatting for unknown errors
	return fmt.Sprintf("❌ Error: %v\n\n📋 General troubleshooting:\n  1. Check the error message above\n  2. Verify your configuration\n  3. Check service logs\n  4. Consult documentation: https://github.com/Bajusz15/beacon#troubleshooting", err)
}

// GetServiceName returns a likely service name based on port
func GetServiceName(port int) string {
	serviceMap := map[int]string{
		22:   "ssh",
		80:   "nginx",
		443:  "nginx",
		3306: "mysql",
		5432: "postgresql",
		6379: "redis",
		8080: "tomcat",
		9200: "elasticsearch",
	}

	if service, exists := serviceMap[port]; exists {
		return service
	}
	return "unknown-service"
}
