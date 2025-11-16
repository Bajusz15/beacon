package util

import (
	"bytes"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// mockCloser implements io.Closer for testing
type mockCloser struct {
	shouldError bool
	closed      bool
}

func (m *mockCloser) Close() error {
	m.closed = true
	if m.shouldError {
		return errors.New("mock close error")
	}
	return nil
}

// mockConn implements net.Conn for testing
type mockConn struct {
	mockCloser
}

func (m *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestCloser_Close(t *testing.T) {
	tests := []struct {
		name        string
		shouldError bool
		expectLog   bool
	}{
		{
			name:        "successful close",
			shouldError: false,
			expectLog:   false,
		},
		{
			name:        "error on close",
			shouldError: true,
			expectLog:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(os.Stderr)

			closer := NewCloser("[Test]")
			mock := &mockCloser{shouldError: tt.shouldError}

			closer.Close(mock, "test resource")

			if !mock.closed {
				t.Error("Expected resource to be closed")
			}

			logOutput := buf.String()
			if tt.expectLog && !strings.Contains(logOutput, "Failed to close test resource") {
				t.Errorf("Expected error log, got: %s", logOutput)
			}
			if !tt.expectLog && logOutput != "" {
				t.Errorf("Expected no log output, got: %s", logOutput)
			}
		})
	}
}

func TestCloser_CloseFile(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	closer := NewCloser("[Test]")
	mock := &mockCloser{shouldError: true}

	closer.Close(mock, "test.txt")

	if !mock.closed {
		t.Error("Expected file to be closed")
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "Failed to close test.txt") {
		t.Errorf("Expected error log, got: %s", logOutput)
	}
}

func TestCloser_CloseConn(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	closer := NewCloser("[Test]")
	mock := &mockConn{mockCloser{shouldError: true}}

	closer.Close(mock, "TCP")

	if !mock.closed {
		t.Error("Expected connection to be closed")
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "Failed to close TCP") {
		t.Errorf("Expected error log, got: %s", logOutput)
	}
}

func TestCloser_CloseResponse(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	closer := NewCloser("[Test]")

	// Create a test server to get a real response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to create test response: %v", err)
	}

	closer.Close(resp.Body, "test-endpoint")

	// Should not log anything for successful close
	logOutput := buf.String()
	if logOutput != "" {
		t.Errorf("Expected no log output for successful close, got: %s", logOutput)
	}
}

func TestDeferFunctions(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	mock := &mockCloser{shouldError: true}

	func() {
		defer DeferClose(mock, "deferred resource")()
	}()

	if !mock.closed {
		t.Error("Expected resource to be closed via defer")
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "Failed to close deferred resource") {
		t.Errorf("Expected defer error log, got: %s", logOutput)
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	mock := &mockCloser{shouldError: false}

	Close(mock, "package level test")

	if !mock.closed {
		t.Error("Expected resource to be closed via package function")
	}

	// Should not log anything for successful close
	logOutput := buf.String()
	if logOutput != "" {
		t.Errorf("Expected no log output for successful close, got: %s", logOutput)
	}
}

func TestLogError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		operation string
		expectLog bool
	}{
		{
			name:      "no error",
			err:       nil,
			operation: "test operation",
			expectLog: false,
		},
		{
			name:      "with error",
			err:       errors.New("test error"),
			operation: "test operation",
			expectLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(os.Stderr)

			LogError(tt.err, tt.operation)

			logOutput := buf.String()
			if tt.expectLog && !strings.Contains(logOutput, "Failed to test operation") {
				t.Errorf("Expected error log, got: %s", logOutput)
			}
			if !tt.expectLog && logOutput != "" {
				t.Errorf("Expected no log output, got: %s", logOutput)
			}
		})
	}
}
