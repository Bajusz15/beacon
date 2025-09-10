package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// MockSystemMetricsCollector implements SystemMetricsCollector for testing
type MockSystemMetricsCollector struct {
	Hostname      string
	IPAddress     string
	CPUUsage      float64
	MemoryUsage   float64
	DiskUsage     float64
	LoadAverage   float64
	UptimeSeconds int64
	Errors        map[string]error
}

func (m *MockSystemMetricsCollector) GetHostname() (string, error) {
	if err, exists := m.Errors["hostname"]; exists {
		return "", err
	}
	return m.Hostname, nil
}

func (m *MockSystemMetricsCollector) GetIPAddress() (string, error) {
	if err, exists := m.Errors["ip_address"]; exists {
		return "", err
	}
	return m.IPAddress, nil
}

func (m *MockSystemMetricsCollector) GetCPUUsage() (float64, error) {
	if err, exists := m.Errors["cpu"]; exists {
		return 0, err
	}
	return m.CPUUsage, nil
}

func (m *MockSystemMetricsCollector) GetMemoryUsage() (float64, error) {
	if err, exists := m.Errors["memory"]; exists {
		return 0, err
	}
	return m.MemoryUsage, nil
}

func (m *MockSystemMetricsCollector) GetDiskUsage(path string) (float64, error) {
	if err, exists := m.Errors["disk"]; exists {
		return 0, err
	}
	return m.DiskUsage, nil
}

func (m *MockSystemMetricsCollector) GetLoadAverage() (float64, error) {
	if err, exists := m.Errors["load_average"]; exists {
		return 0, err
	}
	return m.LoadAverage, nil
}

func (m *MockSystemMetricsCollector) GetUptime() (int64, error) {
	if err, exists := m.Errors["uptime"]; exists {
		return 0, err
	}
	return m.UptimeSeconds, nil
}

func TestReportSystemMetrics(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		mockCollector  *MockSystemMetricsCollector
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name: "successful metrics collection with all enabled",
			config: &Config{
				Device: DeviceConfig{
					Name:        "test-device",
					Location:    "test-location",
					Environment: "test",
					Tags:        []string{"test", "device"},
				},
				SystemMetrics: SystemMetricsConfig{
					Enabled:     true,
					CPU:         true,
					Memory:      true,
					Disk:        true,
					LoadAverage: true,
					DiskPath:    "/test",
				},
				Report: ReportConfig{
					SendTo: "http://test.example.com",
					Token:  "test-token",
				},
			},
			mockCollector: &MockSystemMetricsCollector{
				Hostname:      "test-host",
				IPAddress:     "192.168.1.100",
				CPUUsage:      45.2,
				MemoryUsage:   67.8,
				DiskUsage:     23.4,
				LoadAverage:   1.2,
				UptimeSeconds: 86400,
				Errors:        make(map[string]error),
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var metrics AgentMetrics
				if err := json.Unmarshal(body, &metrics); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				// Check basic info
				if metrics.Hostname != "test-host" {
					t.Errorf("Expected hostname 'test-host', got '%s'", metrics.Hostname)
				}
				if metrics.IPAddress != "192.168.1.100" {
					t.Errorf("Expected IP '192.168.1.100', got '%s'", metrics.IPAddress)
				}

				// Check enabled metrics
				if metrics.CPUUsage != 45.2 {
					t.Errorf("Expected CPU usage 45.2, got %f", metrics.CPUUsage)
				}
				if metrics.MemoryUsage != 67.8 {
					t.Errorf("Expected memory usage 67.8, got %f", metrics.MemoryUsage)
				}
				if metrics.DiskUsage != 23.4 {
					t.Errorf("Expected disk usage 23.4, got %f", metrics.DiskUsage)
				}
				if metrics.LoadAverage != 1.2 {
					t.Errorf("Expected load average 1.2, got %f", metrics.LoadAverage)
				}

				// Check basic metrics (always collected)
				if metrics.UptimeSeconds != 86400 {
					t.Errorf("Expected uptime 86400, got %d", metrics.UptimeSeconds)
				}
			},
		},
		{
			name: "partial metrics collection with some disabled",
			config: &Config{
				Device: DeviceConfig{
					Name: "test-device",
				},
				SystemMetrics: SystemMetricsConfig{
					Enabled:     true,
					CPU:         true,
					Memory:      false, // Disabled
					Disk:        true,
					LoadAverage: false, // Disabled
				},
				Report: ReportConfig{
					SendTo: "http://test.example.com",
					Token:  "test-token",
				},
			},
			mockCollector: &MockSystemMetricsCollector{
				Hostname:      "test-host",
				IPAddress:     "192.168.1.100",
				CPUUsage:      45.2,
				MemoryUsage:   67.8, // Should be ignored
				DiskUsage:     23.4,
				LoadAverage:   1.2, // Should be ignored
				UptimeSeconds: 86400,
				Errors:        make(map[string]error),
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var metrics AgentMetrics
				if err := json.Unmarshal(body, &metrics); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				// Check enabled metrics
				if metrics.CPUUsage != 45.2 {
					t.Errorf("Expected CPU usage 45.2, got %f", metrics.CPUUsage)
				}
				if metrics.DiskUsage != 23.4 {
					t.Errorf("Expected disk usage 23.4, got %f", metrics.DiskUsage)
				}

				// Check disabled metrics should be zero
				if metrics.MemoryUsage != 0 {
					t.Errorf("Expected memory usage 0 (disabled), got %f", metrics.MemoryUsage)
				}
				if metrics.LoadAverage != 0 {
					t.Errorf("Expected load average 0 (disabled), got %f", metrics.LoadAverage)
				}
			},
		},
		{
			name: "hostname error should return early",
			config: &Config{
				Device: DeviceConfig{
					Name: "test-device",
				},
				SystemMetrics: SystemMetricsConfig{
					Enabled: true,
				},
				Report: ReportConfig{
					SendTo: "http://test.example.com",
					Token:  "test-token",
				},
			},
			mockCollector: &MockSystemMetricsCollector{
				Errors: map[string]error{
					"hostname": &MockError{message: "hostname error"},
				},
			},
			expectedStatus: http.StatusOK, // Server should return 200, but no request should be made
			expectedBody: func(t *testing.T, body []byte) {
				// Should not receive any request due to early return
				if len(body) > 0 {
					t.Errorf("Expected no request body due to hostname error, got: %s", string(body))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			var receivedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the endpoint
				if r.URL.Path != "/agent/metrics" {
					t.Errorf("Expected path '/agent/metrics', got '%s'", r.URL.Path)
				}

				// Verify the method
				if r.Method != "POST" {
					t.Errorf("Expected method POST, got '%s'", r.Method)
				}

				// Verify the API key header
				if r.Header.Get("X-API-Key") != "test-token" {
					t.Errorf("Expected X-API-Key 'test-token', got '%s'", r.Header.Get("X-API-Key"))
				}

				// Verify content type
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
				}

				// Read the body
				receivedBody = make([]byte, r.ContentLength)
				r.Body.Read(receivedBody)

				w.WriteHeader(tt.expectedStatus)
			}))
			defer server.Close()

			// Update config with test server URL
			tt.config.Report.SendTo = server.URL

			// Create monitor with mock collector
			monitor := &Monitor{
				config:           tt.config,
				results:          make(map[string]*CheckResult),
				httpClient:       &http.Client{Timeout: 5 * time.Second},
				metricsCollector: tt.mockCollector,
			}

			// Call the function
			monitor.reportSystemMetrics()

			// Verify the request body
			if tt.expectedBody != nil {
				tt.expectedBody(t, receivedBody)
			}
		})
	}
}

// Mock error type
type MockError struct {
	message string
}

func (e *MockError) Error() string {
	return e.message
}
