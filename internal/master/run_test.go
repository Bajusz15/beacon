package master

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"beacon/internal/cloud"
	"beacon/internal/identity"

	"github.com/stretchr/testify/require"
)

// setTestCloudURL overrides the compile-time cloud URL for the duration of a test.
func setTestCloudURL(t *testing.T, url string) {
	t.Helper()
	old := cloud.DefaultBeaconInfraAPIURL
	cloud.DefaultBeaconInfraAPIURL = url
	t.Cleanup(func() { cloud.DefaultBeaconInfraAPIURL = old })
}

func TestGetHostname(t *testing.T) {
	hostname := getHostname()
	require.NotEmpty(t, hostname)
	// Should not be "unknown" on a normal system
	if hostname == "unknown" {
		t.Log("Warning: hostname returned 'unknown', may indicate os.Hostname() issue")
	}
}

func TestGetOutboundIP(t *testing.T) {
	ip := getOutboundIP()
	require.NotEmpty(t, ip)
	// Could be "unknown" if no network, but usually returns an IP
	t.Logf("outbound IP: %s", ip)
}

func TestSetAuthHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/api", nil)
	setAuthHeaders(req, "test_token_123")

	require.Equal(t, "Bearer test_token_123", req.Header.Get("Authorization"))
	require.Equal(t, "test_token_123", req.Header.Get("X-API-Key"))
}

func TestHeartbeatRequest_structure(t *testing.T) {
	hr := heartbeatRequest{
		Hostname:     "test-host",
		IPAddress:    "192.168.1.100",
		Tags:         []string{"beacon-master"},
		AgentVersion: "1.0.0",
		DeviceName:   "my-device",
		OS:           "linux",
		Arch:         "amd64",
		Metadata: map[string]string{
			"role": "beacon-master",
		},
	}

	data, err := json.Marshal(hr)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "test-host", decoded["hostname"])
	require.Equal(t, "192.168.1.100", decoded["ip_address"])
	require.Equal(t, "my-device", decoded["device_name"])
	require.Equal(t, "linux", decoded["os"])
	require.Equal(t, "amd64", decoded["arch"])
}

func TestSendCloudHeartbeat_success(t *testing.T) {
	var receivedRequest heartbeatRequest
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/agent/heartbeat", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		receivedAuth = r.Header.Get("Authorization")

		decoder := json.NewDecoder(r.Body)
		require.NoError(t, decoder.Decode(&receivedRequest))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"device_id":"new-device-uuid"}`))
	}))
	defer server.Close()

	// Create temp home for config
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	setTestCloudURL(t, server.URL)

	cfg := &identity.UserConfig{
		APIKey: "usr_test_api_key",
	}

	ctx := context.Background()
	err := sendCloudHeartbeat(ctx, cfg, "test-device", nil, nil, nil)
	require.NoError(t, err)

	// Verify request
	require.Equal(t, "test-device", receivedRequest.DeviceName)
	require.Contains(t, receivedRequest.Tags, "beacon-master")
	require.Equal(t, "beacon-master", receivedRequest.Metadata["role"])
	require.NotEmpty(t, receivedRequest.Hostname)
	require.Equal(t, "Bearer usr_test_api_key", receivedAuth)
}

func TestSendCloudHeartbeat_savesDeviceID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"device_id":"returned-device-uuid"}`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	setTestCloudURL(t, server.URL)

	cfg := &identity.UserConfig{
		APIKey:   "usr_test_key",
		DeviceID: "", // initially empty
	}
	require.NoError(t, cfg.Save())

	ctx := context.Background()
	err := sendCloudHeartbeat(ctx, cfg, "device", nil, nil, nil)
	require.NoError(t, err)

	// Verify device_id was saved
	require.Equal(t, "returned-device-uuid", cfg.DeviceID)

	// Verify it was persisted to disk
	loaded, err := identity.LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, "returned-device-uuid", loaded.DeviceID)
}

func TestSendCloudHeartbeat_httpError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer server.Close()

	setTestCloudURL(t, server.URL)

	cfg := &identity.UserConfig{
		APIKey: "invalid_key",
	}

	ctx := context.Background()
	err := sendCloudHeartbeat(ctx, cfg, "device", nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestSendCloudHeartbeat_networkError(t *testing.T) {
	setTestCloudURL(t, "http://localhost:59999") // unlikely to be listening

	cfg := &identity.UserConfig{
		APIKey: "key",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := sendCloudHeartbeat(ctx, cfg, "device", nil, nil, nil)
	require.Error(t, err)
}

func TestRun_withDisabledCloudReporting(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// Create config with cloud reporting disabled
	cfg := &identity.UserConfig{
		APIKey:                "key",
		CloudReportingEnabled: false,
		HeartbeatInterval:     1, // 1 second for fast test
	}
	require.NoError(t, cfg.Save())

	// Run should not make any HTTP requests
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run in goroutine
	done := make(chan struct{})
	go func() {
		Run(ctx)
		close(done)
	}()

	// Should complete without error when context is canceled
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not complete in time")
	}
}

func TestRun_sendsHeartbeats(t *testing.T) {
	var heartbeatCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/heartbeat" {
			atomic.AddInt32(&heartbeatCount, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	setTestCloudURL(t, server.URL)

	cfg := &identity.UserConfig{
		APIKey:                "usr_key",
		DeviceName:            "test-device",
		CloudReportingEnabled: true,
		HeartbeatInterval:     1, // 1 second
	}
	require.NoError(t, cfg.Save())

	// Run for a short time
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		Run(ctx)
		close(done)
	}()

	<-done

	// Should have sent at least 1 heartbeat (initial + possibly 1 more)
	count := atomic.LoadInt32(&heartbeatCount)
	require.GreaterOrEqual(t, count, int32(1), "expected at least 1 heartbeat")
}

func TestRun_noConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// No config file exists - Run should handle gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success - handled gracefully
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not complete in time")
	}
}

func TestRun_missingCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// Create config without API key or cloud URL
	cfg := &identity.UserConfig{
		CloudReportingEnabled: true,
		HeartbeatInterval:     1,
	}
	require.NoError(t, cfg.Save())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		Run(ctx)
		close(done)
	}()

	// Should complete without panicking (logs warning about missing credentials)
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not complete in time")
	}
}

func TestRun_reloadsConfigOnTick(t *testing.T) {
	var heartbeatCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&heartbeatCount, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	setTestCloudURL(t, server.URL)

	// Start with cloud reporting disabled
	cfg := &identity.UserConfig{
		APIKey:                "usr_key",
		DeviceName:            "test-device",
		CloudReportingEnabled: false,
		HeartbeatInterval:     1,
	}
	require.NoError(t, cfg.Save())

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		Run(ctx)
		close(done)
	}()

	// Wait a bit then enable cloud reporting
	time.Sleep(500 * time.Millisecond)
	cfg.CloudReportingEnabled = true
	require.NoError(t, cfg.Save())

	<-done

	// Should have sent heartbeats after config was updated
	count := atomic.LoadInt32(&heartbeatCount)
	require.GreaterOrEqual(t, count, int32(1), "expected heartbeats after enabling cloud reporting")
}

// Test that heartbeat includes OS and Arch
func TestHeartbeatRequest_includesSystemInfo(t *testing.T) {
	var receivedRequest heartbeatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		_ = decoder.Decode(&receivedRequest)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	setTestCloudURL(t, server.URL)

	cfg := &identity.UserConfig{
		APIKey: "key",
	}

	ctx := context.Background()
	_ = sendCloudHeartbeat(ctx, cfg, "device", nil, nil, nil)

	require.NotEmpty(t, receivedRequest.OS)
	require.NotEmpty(t, receivedRequest.Arch)
	require.NotEmpty(t, receivedRequest.AgentVersion)
}

// Ensure config directory is created
func TestUserConfigPath_createsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	p, err := identity.UserConfigPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmpDir, ".beacon", "config.yaml"), p)

	// Directory shouldn't exist yet (path is just computed)
	_, err = os.Stat(filepath.Dir(p))
	require.True(t, os.IsNotExist(err))

	// After saving, directory should exist
	cfg := &identity.UserConfig{APIKey: "key"}
	require.NoError(t, cfg.Save())

	_, err = os.Stat(filepath.Dir(p))
	require.NoError(t, err)
}
