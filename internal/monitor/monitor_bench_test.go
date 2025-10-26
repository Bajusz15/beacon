package monitor

import (
	"testing"
	"time"

	"beacon/internal/plugins"
)

func BenchmarkMonitor_ExecuteCheck(b *testing.B) {
	// Create a mock monitor for benchmarking
	cfg := &Config{
		Device: DeviceConfig{
			Name: "test-device",
		},
		Checks: []CheckConfig{
			{
				Name:     "test-check",
				Type:     "command",
				Cmd:      "echo 'test'",
				Interval: 30 * time.Second,
			},
		},
		Report: ReportConfig{
			SendTo: "http://localhost:8080",
			Token:  "test-token",
		},
	}

	monitor, err := NewMonitor(cfg, "test-config.yml")
	if err != nil {
		b.Fatalf("Failed to create monitor: %v", err)
	}

	check := cfg.Checks[0]

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		monitor.executeCheck(check)
	}
}

func BenchmarkMonitor_ExecuteHTTPCheck(b *testing.B) {
	cfg := &Config{
		Device: DeviceConfig{
			Name: "test-device",
		},
		Checks: []CheckConfig{
			{
				Name:         "test-http",
				Type:         "http",
				URL:          "https://httpbin.org/status/200",
				Interval:     30 * time.Second,
				ExpectStatus: 200,
			},
		},
		Report: ReportConfig{
			SendTo: "http://localhost:8080",
			Token:  "test-token",
		},
	}

	monitor, err := NewMonitor(cfg, "test-config.yml")
	if err != nil {
		b.Fatalf("Failed to create monitor: %v", err)
	}

	check := cfg.Checks[0]

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		monitor.executeHTTPCheck(check)
	}
}

func BenchmarkMonitor_ExecutePortCheck(b *testing.B) {
	cfg := &Config{
		Device: DeviceConfig{
			Name: "test-device",
		},
		Checks: []CheckConfig{
			{
				Name:     "test-port",
				Type:     "port",
				Host:     "127.0.0.1",
				Port:     22,
				Interval: 30 * time.Second,
			},
		},
		Report: ReportConfig{
			SendTo: "http://localhost:8080",
			Token:  "test-token",
		},
	}

	monitor, err := NewMonitor(cfg, "test-config.yml")
	if err != nil {
		b.Fatalf("Failed to create monitor: %v", err)
	}

	check := cfg.Checks[0]

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		monitor.executePortCheck(check)
	}
}

func BenchmarkMonitor_ExecuteCommandCheck(b *testing.B) {
	cfg := &Config{
		Device: DeviceConfig{
			Name: "test-device",
		},
		Checks: []CheckConfig{
			{
				Name:     "test-command",
				Type:     "command",
				Cmd:      "echo 'test output'",
				Interval: 30 * time.Second,
			},
		},
		Report: ReportConfig{
			SendTo: "http://localhost:8080",
			Token:  "test-token",
		},
	}

	monitor, err := NewMonitor(cfg, "test-config.yml")
	if err != nil {
		b.Fatalf("Failed to create monitor: %v", err)
	}

	check := cfg.Checks[0]

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		monitor.executeCommandCheck(check)
	}
}

func BenchmarkMonitor_ReportSystemMetrics(b *testing.B) {
	cfg := &Config{
		Device: DeviceConfig{
			Name: "test-device",
		},
		SystemMetrics: SystemMetricsConfig{
			Enabled:     true,
			Interval:    60 * time.Second,
			CPU:         true,
			Memory:      true,
			Disk:        true,
			LoadAverage: true,
		},
		Report: ReportConfig{
			SendTo: "http://localhost:8080",
			Token:  "test-token",
		},
	}

	monitor, err := NewMonitor(cfg, "test-config.yml")
	if err != nil {
		b.Fatalf("Failed to create monitor: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		monitor.reportSystemMetrics()
	}
}

func BenchmarkMonitor_PluginIntegration(b *testing.B) {
	cfg := &Config{
		Device: DeviceConfig{
			Name: "test-device",
		},
		Checks: []CheckConfig{
			{
				Name:     "test-check",
				Type:     "command",
				Cmd:      "echo 'test'",
				Interval: 30 * time.Second,
			},
		},
		Plugins: []plugins.PluginConfig{
			{
				Name:    "discord",
				Enabled: true,
				Config: map[string]interface{}{
					"webhook_url": "https://discord.com/api/webhooks/test",
				},
			},
		},
		AlertRules: []plugins.AlertRule{
			{
				Check:    "test-check",
				Severity: "critical",
				Plugins:  []string{"discord"},
			},
		},
		Report: ReportConfig{
			SendTo: "http://localhost:8080",
			Token:  "test-token",
		},
	}

	monitor, err := NewMonitor(cfg, "test-config.yml")
	if err != nil {
		b.Fatalf("Failed to create monitor: %v", err)
	}

	check := cfg.Checks[0]

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		monitor.executeCheck(check)
	}
}

func BenchmarkCheckResultCreation(b *testing.B) {
	device := DeviceConfig{
		Name:        "test-device",
		Location:    "test-location",
		Tags:        []string{"test", "benchmark"},
		Environment: "test",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result := CheckResult{
			Name:           "test-check",
			Type:           "http",
			Status:         "up",
			Duration:       time.Second,
			Timestamp:      time.Now(),
			Error:          "",
			HTTPStatusCode: 200,
			ResponseTime:   time.Millisecond * 100,
			CommandOutput:  "test output",
			CommandError:   "",
			Device:         device,
		}
		_ = result
	}
}
