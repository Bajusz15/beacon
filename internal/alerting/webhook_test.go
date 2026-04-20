package alerting

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSendWebhookAlert_payload(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type %q", ct)
		}
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	sam := NewSimpleAlertManager()
	sam.LoadChannels(map[string]interface{}{
		"webhook": map[string]interface{}{
			"url":     ts.URL + "/hook",
			"enabled": true,
		},
	})

	ctx := AlertContext{
		AlertID:     "a1",
		ProjectID:   "myproj",
		DeviceName:  "dev1",
		Service:     "db",
		Severity:    SeverityCritical,
		Message:     "down",
		Timestamp:   time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		Source:      "beacon",
		Environment: "prod",
	}

	err := sam.sendWebhookAlert(ctx)
	require.NoError(t, err)

	var payload WebhookPayloadV1
	require.NoError(t, json.Unmarshal(gotBody, &payload))
	require.Equal(t, webhookSchemaVersion, payload.SchemaVersion)
	require.Equal(t, "a1", payload.AlertID)
	require.Equal(t, "myproj", payload.ProjectID)
	require.Equal(t, "dev1", payload.DeviceName)
	require.Equal(t, "critical", payload.Severity)
	require.Equal(t, "down", payload.Message)
	require.Equal(t, "db", payload.Service)
	require.Contains(t, payload.Summary, "myproj")
	require.Contains(t, payload.Summary, "dev1")
	require.Contains(t, payload.Summary, "down")
}

func TestSendWebhookAlert_skipsWhenDisabled(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	sam := NewSimpleAlertManager()
	sam.LoadChannels(map[string]interface{}{
		"webhook": map[string]interface{}{
			"url":     ts.URL,
			"enabled": false,
		},
	})

	err := sam.sendWebhookAlert(AlertContext{
		AlertID: "x", Severity: SeverityInfo, Message: "m", Timestamp: time.Now(),
	})
	require.NoError(t, err)
	require.False(t, called)
}
