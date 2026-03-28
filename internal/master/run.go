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
	"beacon/internal/tunnel"
	"beacon/internal/version"
)

type tunnelHeartbeatReport struct {
	ID        string `json:"id"`
	LocalPort int    `json:"local_port"`
	Enabled   bool   `json:"enabled"`
	Autostart *bool  `json:"autostart,omitempty"`
	Connected *bool  `json:"connected,omitempty"`
}

type heartbeatRequest struct {
	Hostname       string                  `json:"hostname"`
	IPAddress      string                  `json:"ip_address"`
	Tags           []string                `json:"tags"`
	AgentVersion   string                  `json:"agent_version"`
	DeviceName     string                  `json:"device_name"`
	OS             string                  `json:"os,omitempty"`
	Arch           string                  `json:"arch,omitempty"`
	Metadata       map[string]string       `json:"metadata"`
	Projects       []ProjectHealth         `json:"projects,omitempty"`
	Tunnels        []tunnelHeartbeatReport `json:"tunnels,omitempty"`
	CommandResults []CommandResultReport   `json:"command_results,omitempty"`
	SystemMetrics  *heartbeatSystemMetrics `json:"system_metrics,omitempty"`
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

func buildTunnelHeartbeatReports(cfg *identity.UserConfig, tm *tunnel.TunnelManager) []tunnelHeartbeatReport {
	if cfg == nil || len(cfg.Tunnels) == 0 {
		return nil
	}
	connectedByID := make(map[string]bool)
	if tm != nil {
		for _, s := range tm.GetTunnelStatuses() {
			connectedByID[s.ID] = s.Status == "connected"
		}
	}
	out := make([]tunnelHeartbeatReport, 0, len(cfg.Tunnels))
	for _, t := range cfg.Tunnels {
		if strings.TrimSpace(t.ID) == "" {
			continue
		}
		noAutostart := false
		r := tunnelHeartbeatReport{
			ID:        t.ID,
			LocalPort: t.LocalPort,
			Enabled:   tunnel.ConfigTunnelEnabled(t),
			Autostart: &noAutostart,
		}
		if c, ok := connectedByID[t.ID]; ok {
			r.Connected = &c
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// Run blocks until ctx is canceled. Reads ~/.beacon/config.yaml (v2 identity).
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

	pm, err := NewProcessManager(ctx)
	if err != nil {
		log.Printf("[Beacon master] Failed to create process manager: %v", err)
	}

	eventLog := NewEventLog()
	if pm != nil {
		pm.eventLog = eventLog
	}

	port := defaultMetricsPort
	if uc != nil && uc.MetricsPort > 0 {
		port = uc.MetricsPort
	}
	listenAddr := ""
	if uc != nil {
		listenAddr = uc.MetricsListenAddr
	}

	statusCache := NewStatusCache(pm, eventLog, uc)
	statusCache.Refresh()

	startStatusServer(ctx, statusCache, port, listenAddr)
	startCacheRefresh(ctx, statusCache)

	if pm != nil && uc != nil && len(uc.Projects) > 0 {
		log.Printf("[Beacon master] Spawning %d project(s)...", len(uc.Projects))
		pm.SpawnAll(uc.Projects)
	}

	// Tunnel manager for on-demand tunnel_connect (no WebSocket at startup)
	var tm *tunnel.TunnelManager
	if uc != nil && len(uc.Tunnels) > 0 && uc.CloudReportingEnabled && strings.TrimSpace(uc.APIKey) != "" {
		var tmErr error
		tm, tmErr = tunnel.NewTunnelManager(ctx)
		if tmErr != nil {
			log.Printf("[Beacon master] Failed to create tunnel manager: %v", tmErr)
		} else {
			log.Printf("[Beacon master] Tunnel manager ready (%d tunnel(s); WebSocket opens from dashboard only)", len(uc.Tunnels))
		}
	}
	statusCache.SetTunnelManager(tm)

	dispatcher := NewCommandDispatcher(pm, tm)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[Beacon master] Started (interval=%s)", interval)

	beat := &heartbeatLoop{
		ctx:         ctx,
		pm:          pm,
		tm:          tm,
		dispatcher:  dispatcher,
		statusCache: statusCache,
		eventLog:    eventLog,
	}

	if uc != nil && uc.CloudReportingEnabled {
		beat.tryBeat()
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Beacon master] Stopping")
			if tm != nil {
				tm.Shutdown()
			}
			if pm != nil {
				pm.Shutdown()
			}
			return
		case <-ticker.C:
			beat.tryBeat()
		}
	}
}

func startStatusServer(ctx context.Context, cache *StatusCache, port int, listenAddr string) {
	var srv *StatusServer
	if listenAddr != "" {
		srv = NewStatusServerWithAddr(cache, port, listenAddr)
	} else {
		srv = NewStatusServer(cache, port)
	}
	go func() {
		if err := srv.Start(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[Beacon master] Status server: %v", err)
		}
	}()
}

func startCacheRefresh(ctx context.Context, cache *StatusCache) {
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cache.Refresh()
			}
		}
	}()
}

// heartbeatLoop holds state for the recurring cloud heartbeat.
type heartbeatLoop struct {
	ctx                      context.Context
	pm                       *ProcessManager
	tm                       *tunnel.TunnelManager
	dispatcher               *CommandDispatcher
	statusCache              *StatusCache
	eventLog                 *EventLog
	lastSystemMetricsSentAt time.Time
}

func (h *heartbeatLoop) tryBeat() {
	uc, err := identity.LoadUserConfig()
	if err != nil {
		log.Printf("[Beacon master] Reload config: %v", err)
		return
	}
	if uc == nil || !uc.CloudReportingEnabled {
		return
	}
	if strings.TrimSpace(uc.APIKey) == "" {
		return
	}
	name := strings.TrimSpace(uc.DeviceName)
	if name == "" {
		name = getHostname()
	}

	h.statusCache.UpdateConfig(uc)
	h.dispatcher.CollectResults()

	if err := sendCloudHeartbeat(h.ctx, h, uc, name, h.pm, h.dispatcher, h.tm); err != nil {
		log.Printf("[Beacon master] Heartbeat: %v", err)
	} else {
		h.statusCache.UpdateCloudSync()
		h.eventLog.Append(Event{
			Timestamp: time.Now(),
			Type:      EventSync,
			Message:   "cloud heartbeat OK",
		})
	}
}

func sendCloudHeartbeat(ctx context.Context, h *heartbeatLoop, cfg *identity.UserConfig, deviceName string, pm *ProcessManager, dispatcher *CommandDispatcher, tm *tunnel.TunnelManager) error {
	base := cfg.EffectiveCloudAPIBase()
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
		Tunnels:        buildTunnelHeartbeatReports(cfg, tm),
		CommandResults: commandResults,
	}

	if h != nil {
		if sm, ok := buildSystemMetricsForCloud(cfg, h.lastSystemMetricsSentAt); ok {
			payload.SystemMetrics = sm
		}
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
	if h != nil && payload.SystemMetrics != nil {
		h.lastSystemMetricsSentAt = time.Now()
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
