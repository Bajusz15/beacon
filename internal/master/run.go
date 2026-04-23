package master

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"beacon/internal/cloud"
	"beacon/internal/identity"
	"beacon/internal/logging"
	"beacon/internal/tunnel"
	"beacon/internal/version"
	"beacon/internal/vpn"
)

type tunnelHeartbeatReport struct {
	ID               string `json:"id"`
	LocalPort        int    `json:"local_port"`
	UpstreamProtocol string `json:"upstream_protocol,omitempty"`
	UpstreamHost     string `json:"upstream_host,omitempty"`
	Enabled          bool   `json:"enabled"`
	Autostart        *bool  `json:"autostart,omitempty"`
	Connected        *bool  `json:"connected,omitempty"`
}

type vpnHeartbeatReport struct {
	Enabled    bool   `json:"enabled"`
	Role       string `json:"role,omitempty"`
	VPNAddress string `json:"vpn_address,omitempty"`
	PeerDevice string `json:"peer_device,omitempty"`
	Connected  bool   `json:"connected"`
	BytesRx    uint64 `json:"bytes_rx,omitempty"`
	BytesTx    uint64 `json:"bytes_tx,omitempty"`
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
	VPN            *vpnHeartbeatReport     `json:"vpn,omitempty"`
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
		autostart := true
		proto, host, port, _ := t.EffectiveUpstream()
		r := tunnelHeartbeatReport{
			ID:               t.ID,
			LocalPort:        port,
			UpstreamProtocol: proto,
			UpstreamHost:     host,
			Enabled:          tunnel.ConfigTunnelEnabled(t),
			Autostart:        &autostart,
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
		logger.Infof("Failed to load config: %v", err)
		uc = nil
	}
	if uc != nil && uc.LogLevel != "" {
		logging.SetLevel(uc.LogLevel)
		// Propagate to spawned children via env inheritance (only if env var not already set).
		if os.Getenv("BEACON_LOG_LEVEL") == "" {
			_ = os.Setenv("BEACON_LOG_LEVEL", uc.LogLevel)
		}
	}
	interval := 60 * time.Second
	if uc != nil {
		interval = uc.HeartbeatIntervalDuration()
	}

	pm, err := NewProcessManager(ctx)
	if err != nil {
		logger.Infof("Failed to create process manager: %v", err)
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
		logger.Infof("Spawning %d project(s)...", len(uc.Projects))
		pm.SpawnAll(uc.Projects)
	}

	tm := initTunnelManager(ctx, uc)
	statusCache.SetTunnelManager(tm)

	vm := initVPNManager(ctx, uc)
	statusCache.SetVPNManager(vm)

	dispatcher := NewCommandDispatcher(pm, tm)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Infof("Started (interval=%s)", interval)

	beat := &heartbeatLoop{
		ctx:         ctx,
		pm:          pm,
		tm:          tm,
		vm:          vm,
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
			logger.Infof("Stopping")
			if vm != nil {
				vm.Stop()
			}
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
			logger.Infof("Status server: %v", err)
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

// vpnCloudAdapter adapts cloud.VPNClient to the vpn.PeerResolver interface.
// The two packages can't import each other directly (cycle via identity), so the
// adapter lives in master where both are already imported.
type vpnCloudAdapter struct {
	c *cloud.VPNClient
}

func (a *vpnCloudAdapter) RegisterVPN(ctx context.Context, publicKey, role string, listenPort int, endpoint string) (string, error) {
	return a.c.RegisterVPN(ctx, publicKey, role, listenPort, endpoint)
}

func (a *vpnCloudAdapter) GetPeer(ctx context.Context, deviceName string) (*vpn.PeerInfo, error) {
	p, err := a.c.GetPeer(ctx, deviceName)
	if err != nil {
		return nil, err
	}
	return &vpn.PeerInfo{
		DeviceName: p.DeviceName,
		PublicKey:  p.PublicKey,
		Endpoint:   p.Endpoint,
		VPNAddress: p.VPNAddress,
		AllowedIPs: p.AllowedIPs,
	}, nil
}

func (a *vpnCloudAdapter) DeregisterVPN(ctx context.Context) error {
	return a.c.DeregisterVPN(ctx)
}

// initVPNManager constructs a VPN manager. Cloud reporting must be enabled —
// without an API key the manager has no way to register the public key.
// The manager is created in "stopped" state; the heartbeat loop's first
// Reconcile() will bring the interface up if cfg.VPN.Enabled is true.
func initVPNManager(_ context.Context, uc *identity.UserConfig) *vpn.Manager {
	if uc == nil || !uc.CloudReportingEnabled || strings.TrimSpace(uc.APIKey) == "" {
		return nil
	}
	deviceName := strings.TrimSpace(uc.DeviceName)
	if deviceName == "" {
		deviceName = getHostname()
	}
	client := cloud.NewVPNClient(uc.EffectiveCloudAPIBase(), uc.APIKey, deviceName)
	return vpn.NewManager(&vpnCloudAdapter{c: client})
}

// initTunnelManager creates and auto-starts enabled tunnels if cloud reporting is configured.
func initTunnelManager(ctx context.Context, uc *identity.UserConfig) *tunnel.TunnelManager {
	if uc == nil || len(uc.Tunnels) == 0 || !uc.CloudReportingEnabled || strings.TrimSpace(uc.APIKey) == "" {
		return nil
	}
	tm, err := tunnel.NewTunnelManager(ctx)
	if err != nil {
		logger.Infof("Failed to create tunnel manager: %v", err)
		return nil
	}
	apiKey := strings.TrimSpace(uc.APIKey)
	deviceName := strings.TrimSpace(uc.DeviceName)
	if deviceName == "" {
		deviceName = getHostname()
	}
	started := 0
	for _, t := range uc.Tunnels {
		if !tunnel.ConfigTunnelEnabled(t) {
			continue
		}
		if started >= tunnel.MaxActiveTunnels {
			logger.Infof("Tunnel %s skipped (limit: %d active tunnels)", t.ID, tunnel.MaxActiveTunnels)
			continue
		}
		if err := tm.EnsureStarted(t, uc.EffectiveCloudAPIBase(), apiKey, deviceName); err != nil {
			logger.Infof("Tunnel %s failed to start: %v", t.ID, err)
		} else {
			started++
		}
	}
	logger.Infof("Tunnel manager started (%d/%d tunnel(s) active)", started, len(uc.Tunnels))
	return tm
}

// heartbeatLoop holds state for the recurring cloud heartbeat.
type heartbeatLoop struct {
	ctx                     context.Context
	pm                      *ProcessManager
	tm                      *tunnel.TunnelManager
	vm                      *vpn.Manager
	dispatcher              *CommandDispatcher
	statusCache             *StatusCache
	eventLog                *EventLog
	lastSystemMetricsSentAt time.Time
}

func (h *heartbeatLoop) tryBeat() {
	uc, err := identity.LoadUserConfig()
	if err != nil {
		logger.Infof("Reload config: %v", err)
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
	h.dispatcher.SetAllowedActions(uc.AllowedRemoteCommands)
	h.dispatcher.CollectResults()

	// Reconcile VPN state with the (possibly hot-reloaded) config. Reconcile is
	// idempotent — a no-op if nothing changed — so it's cheap to call every tick.
	if h.vm != nil {
		if err := h.vm.Reconcile(h.ctx, uc.VPN); err != nil {
			logger.Infof("VPN reconcile: %v", err)
		}
	}

	if err := sendCloudHeartbeat(h.ctx, h, uc, name, h.pm, h.dispatcher, h.tm); err != nil {
		logger.Infof("Heartbeat: %v", err)
	} else {
		h.statusCache.UpdateCloudSync()
		h.eventLog.Append(Event{
			Timestamp: time.Now(),
			Type:      EventSync,
			Message:   "cloud heartbeat OK",
		})
	}
}

func buildVPNHeartbeatReport(vm *vpn.Manager) *vpnHeartbeatReport {
	if vm == nil {
		return nil
	}
	s := vm.Status()
	if !s.Enabled {
		return nil
	}
	return &vpnHeartbeatReport{
		Enabled:    true,
		Role:       string(s.Role),
		VPNAddress: s.VPNAddress,
		PeerDevice: s.PeerDevice,
		Connected:  s.Connected,
		BytesRx:    s.BytesRx,
		BytesTx:    s.BytesTx,
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
		payload.VPN = buildVPNHeartbeatReport(h.vm)
	}

	if h != nil {
		if sm, ok := buildSystemMetricsForCloud(cfg, h.lastSystemMetricsSentAt); ok {
			payload.SystemMetrics = sm
		}
	}

	if len(payload.Projects) > 0 {
		logger.Infof("Heartbeat includes %d project(s)", len(payload.Projects))
	}
	if len(payload.CommandResults) > 0 {
		logger.Infof("Heartbeat includes %d command result(s)", len(payload.CommandResults))
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
		logger.Infof("Failed to parse heartbeat response: %v", err)
	} else {
		// Save device_id if changed
		if hr.DeviceID != "" && cfg.DeviceID != hr.DeviceID {
			cfg.DeviceID = hr.DeviceID
			if err := cfg.Save(); err != nil {
				logger.Infof("Could not save device_id: %v", err)
			}
		}

		// Dispatch any commands from the response
		if dispatcher != nil && len(hr.Commands) > 0 {
			logger.Infof("Received %d command(s) from server", len(hr.Commands))
			dispatcher.DispatchCommands(hr.Commands)
		}
	}

	return nil
}
