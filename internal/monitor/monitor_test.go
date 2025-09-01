package monitor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_executeHTTPCheck(t *testing.T) {
	monitor, err := NewMonitor(&Config{
		Checks: []CheckConfig{
			{
				Name: "Homepage",
				Type: "http",
				URL:  "https://mestertkeresek.hu/",
			},
		},
		// Report: ReportConfig{
		// 	SendTo: "https://beaconinfra.dev/api/monitor",
		// 	Token:  "YOUR_API_TOKEN",
		// 	PrometheusEnable: false,
		// },
	})
	if err != nil {
		t.Fatalf("Failed to create monitor: %v", err)
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	result := monitor.executeHTTPCheck(CheckConfig{
		Name: "Homepage",
		URL:  mockServer.URL,
	})
	assert.Equal(t, "up", result.Status)

}

func Test_executeCommandCheck(t *testing.T) {
	monitor, err := NewMonitor(&Config{
		Checks: []CheckConfig{
			{
				Name: "Test Command",
				Type: "command",
				Cmd:  "echo 'Hello World'",
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create monitor: %v", err)
	}

	result := monitor.executeCommandCheck(CheckConfig{
		Name: "Test Command",
		Type: "command",
		Cmd:  "echo 'Hello World'",
	})

	assert.Equal(t, "up", result.Status)
	assert.Equal(t, "Hello World", result.CommandOutput)
	assert.Equal(t, "", result.CommandError)
}

func Test_executeCommandCheckWithError(t *testing.T) {
	monitor, err := NewMonitor(&Config{
		Checks: []CheckConfig{
			{
				Name: "Test Command Error",
				Type: "command",
				Cmd:  "nonexistentcommand",
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create monitor: %v", err)
	}

	result := monitor.executeCommandCheck(CheckConfig{
		Name: "Test Command Error",
		Type: "command",
		Cmd:  "nonexistentcommand",
	})

	assert.Equal(t, "down", result.Status)
	assert.Contains(t, result.Error, "command failed")
}
