package master

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"beacon/internal/identity"
	"beacon/internal/version"
)

type heartbeatRequest struct {
	Hostname       string                `json:"hostname"`
	IPAddress      string                `json:"ip_address"`
	Tags           []string              `json:"tags"`
	AgentVersion   string                `json:"agent_version"`
	DeviceName     string                `json:"device_name"`
	OS             string                `json:"os,omitempty"`
	Arch           string                `json:"arch,omitempty"`
	Metadata       map[string]string     `json:"metadata"`
	Projects       []ProjectHealth       `json:"projects,omitempty"`
	CommandResults []CommandResultReport `json:"command_results,omitempty"`
}

// heartbeatResponse represents the server response to /agent/heartbeat.
type heartbeatResponse struct {
	Ack      bool               `json:"ack,omitempty"`
	DeviceID string             `json:"device_id,omitempty"`
	Commands []HeartbeatCommand `json:"commands,omitempty"`
}

func setAuthHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-API-Key", token)
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "unknown"
	}
	return strings.TrimSpace(h)
}

func getOutboundIP() string {
	c, err := net.DialTimeout("udp", "8.8.8.8:53", 2*time.Second)
	if err != nil {
		return "unknown"
	}
	defer func() { _ = c.Close() }()
	addr, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok || addr == nil {
		return "unknown"
	}
	return addr.IP.String()
}

// Run blocks until ctx is cancelled. Reads ~/.beacon/config.yaml (v2 identity).
// Spawns child agents for configured projects and sends heartbeats to the cloud.
func Run(ctx context.Context) {
	uc, err := identity.LoadUserConfig()
	if err != nil {
		log.Printf("[Beacon master] Failed to load config: %v", err)
		uc = nil
	}
	interval := 60 * time.Second
	if uc != nil {
		interval = uc.HeartbeatIntervalDuration()
	}

	// Create process manager for child agents
	pm, err := NewProcessManager(ctx)
	if err != nil {
		log.Printf("[Beacon master] Failed to create process manager: %v", err)
	}

	// Create event log and status infrastructure
	eventLog := NewEventLog()
	if pm != nil {
		pm.eventLog = eventLog
	}

	port := defaultMetricsPort
	if uc != nil && uc.MetricsPort > 0 {
		port = uc.MetricsPort
	}

	statusCache := NewStatusCache(pm, eventLog, uc)
	statusCache.Refresh()

	srv := NewStatusServer(statusCache, port)
	go func() {
		if err := srv.Start(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[Beacon master] Status server: %v", err)
		}
	}()

	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				statusCache.Refresh()
			}
		}
	}()

	// Spawn children for all configured projects
	if pm != nil && uc != nil && len(uc.Projects) > 0 {
		log.Printf("[Beacon master] Spawning %d project(s)...", len(uc.Projects))
		pm.SpawnAll(uc.Projects)
	}

	// Create command dispatcher for piggyback commands
	dispatcher := NewCommandDispatcher(pm)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[Beacon master] Started (interval=%s)", interval)

	tryBeat := func() {
		uc2, err := identity.LoadUserConfig()
		if err != nil {
			log.Printf("[Beacon master] Reload config: %v", err)
			return
		}
		if uc2 == nil || !uc2.CloudReportingEnabled {
			return
		}
		if strings.TrimSpace(uc2.APIKey) == "" || strings.TrimSpace(uc2.CloudURL) == "" {
			log.Printf("[Beacon master] Skipping heartbeat: set api_key and cloud_url (beacon init)")
			return
		}
		name := strings.TrimSpace(uc2.DeviceName)
		if name == "" {
			name = getHostname()
		}

		statusCache.UpdateConfig(uc2)

		// Collect any pending command results before heartbeat
		dispatcher.CollectResults()

		if err := sendCloudHeartbeat(ctx, uc2, name, pm, dispatcher); err != nil {
			log.Printf("[Beacon master] Heartbeat: %v", err)
		} else {
			statusCache.UpdateCloudSync()
			eventLog.Append(Event{
				Timestamp: time.Now(),
				Type:      EventSync,
				Message:   "cloud heartbeat OK",
			})
		}
	}

	if uc != nil && uc.CloudReportingEnabled {
		tryBeat()
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Beacon master] Stopping")
			if pm != nil {
				pm.Shutdown()
			}
			return
		case <-ticker.C:
			tryBeat()
		}
	}
}

func sendCloudHeartbeat(ctx context.Context, cfg *identity.UserConfig, deviceName string, pm *ProcessManager, dispatcher *CommandDispatcher) error {
	base := strings.TrimSuffix(strings.TrimSpace(cfg.CloudURL), "/")
	token := strings.TrimSpace(cfg.APIKey)

	// Get pending command results to include in heartbeat
	var commandResults []CommandResultReport
	if dispatcher != nil {
		commandResults = dispatcher.GetPendingResults()
	}

	payload := heartbeatRequest{
		Hostname:     getHostname(),
		IPAddress:    getOutboundIP(),
		Tags:         []string{"beacon-master"},
		AgentVersion: version.GetVersion(),
		DeviceName:   deviceName,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Metadata: map[string]string{
			"role": "beacon-master",
		},
		Projects:       AggregateProjectHealth(pm),
		CommandResults: commandResults,
	}

	if len(payload.Projects) > 0 {
		log.Printf("[Beacon master] Heartbeat includes %d project(s)", len(payload.Projects))
	}
	if len(payload.CommandResults) > 0 {
		log.Printf("[Beacon master] Heartbeat includes %d command result(s)", len(payload.CommandResults))
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := base + "/agent/heartbeat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req, token)

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Parse response
	var hr heartbeatResponse
	if err := json.Unmarshal(respBody, &hr); err != nil {
		log.Printf("[Beacon master] Failed to parse heartbeat response: %v", err)
	} else {
		// Save device_id if changed
		if hr.DeviceID != "" && cfg.DeviceID != hr.DeviceID {
			cfg.DeviceID = hr.DeviceID
			if err := cfg.Save(); err != nil {
				log.Printf("[Beacon master] Could not save device_id: %v", err)
			}
		}

		// Dispatch any commands from the response
		if dispatcher != nil && len(hr.Commands) > 0 {
			log.Printf("[Beacon master] Received %d command(s) from server", len(hr.Commands))
			dispatcher.DispatchCommands(hr.Commands)
		}
	}

	return nil
}
