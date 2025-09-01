package monitor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_executeHTTPCheck(t *testing.T) {

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	monitor, err := NewMonitor(&Config{})
	if err != nil {
		t.Fatalf("Failed to create monitor: %v", err)
	}

	result := monitor.executeHTTPCheck(CheckConfig{
		Name: "Homepage",
		URL:  mockServer.URL,
	})
	assert.Equal(t, "up", result.Status)
}
