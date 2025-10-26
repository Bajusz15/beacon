package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"beacon/internal/errors"
	"beacon/internal/keys"
	"beacon/internal/plugins"
	"beacon/internal/plugins/discord"
	"beacon/internal/plugins/email"
	"beacon/internal/plugins/telegram"
	"beacon/internal/plugins/webhook"
	"beacon/internal/ratelimit"
	"beacon/internal/util"
	"beacon/internal/version"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const maxOutputLength = 200 // Truncate output longer than 200 chars

type DeviceConfig struct {
	Name        string   `yaml:"name"`
	Location    string   `yaml:"location,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Environment string   `yaml:"environment,omitempty"`
}

type Config struct {
	Device        DeviceConfig           `yaml:"device"`
	Checks        []CheckConfig          `yaml:"checks"`
	SystemMetrics SystemMetricsConfig    `yaml:"system_metrics,omitempty"`
	LogSources    []LogSource            `yaml:"log_sources,omitempty"`
	Plugins       []plugins.PluginConfig `yaml:"plugins,omitempty"`
	AlertRules    []plugins.AlertRule    `yaml:"alert_rules,omitempty"`
	Report        ReportConfig           `yaml:"report"`
}

type CheckConfig struct {
	Name         string        `yaml:"name"`
	Type         string        `yaml:"type"` // "http", "port", "command"
	URL          string        `yaml:"url,omitempty"`
	Host         string        `yaml:"host,omitempty"`
	Port         int           `yaml:"port,omitempty"`
	Cmd          string        `yaml:"cmd,omitempty"`
	Interval     time.Duration `yaml:"interval"`
	ExpectStatus int           `yaml:"expect_status,omitempty"`
	AlertCommand string        `yaml:"alert_command,omitempty"`
}

type SystemMetricsConfig struct {
	Enabled     bool          `yaml:"enabled"`
	Interval    time.Duration `yaml:"interval,omitempty"`
	CPU         bool          `yaml:"cpu,omitempty"`
	Memory      bool          `yaml:"memory,omitempty"`
	Disk        bool          `yaml:"disk,omitempty"`
	LoadAverage bool          `yaml:"load_average,omitempty"`
	DiskPath    string        `yaml:"disk_path,omitempty"` // Default: "/"
}

type ReportConfig struct {
	SendTo           string          `yaml:"send_to"`
	Token            string          `yaml:"token,omitempty"`
	TokenName        string          `yaml:"token_name,omitempty"`
	PrometheusEnable bool            `yaml:"prometheus_metrics"`
	PrometheusPort   int             `yaml:"prometheus_port"`
	Heartbeat        HeartbeatConfig `yaml:"heartbeat,omitempty"`
}

type HeartbeatConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval,omitempty"`
}

type CheckResult struct {
	Name           string        `json:"name"`
	Type           string        `json:"type"`
	Status         string        `json:"status"` // "up", "down", "error"
	Duration       time.Duration `json:"duration"`
	Timestamp      time.Time     `json:"timestamp"`
	Error          string        `json:"error,omitempty"`
	HTTPStatusCode int           `json:"http_status_code,omitempty"`
	ResponseTime   time.Duration `json:"response_time,omitempty"`
	CommandOutput  string        `json:"command_output,omitempty"`
	CommandError   string        `json:"command_error,omitempty"`
	Device         DeviceConfig  `json:"device,omitempty"`
}

// AgentMetrics represents system metrics for the /agent/metrics endpoint
type AgentMetrics struct {
	Hostname      string                  `json:"hostname"`
	IPAddress     string                  `json:"ip_address"`
	CPUUsage      float64                 `json:"cpu_usage"`
	MemoryUsage   float64                 `json:"memory_usage"`
	DiskUsage     float64                 `json:"disk_usage"`
	UptimeSeconds int64                   `json:"uptime_seconds"`
	LoadAverage   float64                 `json:"load_average"`
	CustomMetrics map[string]*CheckResult `json:"custom_metrics"`
	Timestamp     time.Time               `json:"timestamp"`
}

// AgentHeartbeatRequest represents a heartbeat for the /agent/heartbeat endpoint
type AgentHeartbeatRequest struct {
	Hostname     string            `json:"hostname"`
	IPAddress    string            `json:"ip_address"`
	Tags         []string          `json:"tags"`
	AgentVersion string            `json:"agent_version"`
	DeviceName   string            `json:"device_name"`
	Metadata     map[string]string `json:"metadata"`
}

// SystemMetricsCollector interface for dependency injection
type SystemMetricsCollector interface {
	GetHostname() (string, error)
	GetIPAddress() (string, error)
	GetCPUUsage() (float64, error)
	GetMemoryUsage() (float64, error)
	GetDiskUsage(path string) (float64, error)
	GetLoadAverage() (float64, error)
	GetUptime() (int64, error)
}

type Monitor struct {
	config                    *Config
	logManager                *LogManager
	results                   map[string]*CheckResult
	resultsMux                sync.RWMutex
	httpClient                *ratelimit.HTTPClient
	ctx                       context.Context
	cancel                    context.CancelFunc
	heartbeatTicker           *time.Ticker
	reportSystemMetricsTicker *time.Ticker
	metricsCollector          SystemMetricsCollector
	keyManager                *keys.KeyManager
	currentToken              string
	configWatcher             *fsnotify.Watcher
	configPath                string
	pluginManager             *plugins.Manager
}

// LinuxSystemMetricsCollector implements SystemMetricsCollector for Linux systems
type LinuxSystemMetricsCollector struct{}

func (c *LinuxSystemMetricsCollector) GetHostname() (string, error) {
	return getHostname()
}

func (c *LinuxSystemMetricsCollector) GetIPAddress() (string, error) {
	return getIPAddress()
}

func (c *LinuxSystemMetricsCollector) GetCPUUsage() (float64, error) {
	return getCPUUsage()
}

func (c *LinuxSystemMetricsCollector) GetMemoryUsage() (float64, error) {
	return getMemoryUsage()
}

func (c *LinuxSystemMetricsCollector) GetDiskUsage(path string) (float64, error) {
	return getDiskUsage(path)
}

func (c *LinuxSystemMetricsCollector) GetLoadAverage() (float64, error) {
	return getLoadAverage()
}

func (c *LinuxSystemMetricsCollector) GetUptime() (int64, error) {
	return getUptime()
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		beaconErr := errors.NewFileError(path, err)
		return nil, beaconErr
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		beaconErr := errors.NewConfigError(path, err).
			WithTroubleshooting(
				"Invalid YAML syntax",
				"Missing required fields",
				"Invalid field values",
			).WithNextSteps(
			"Validate YAML syntax",
			"Check configuration schema",
			"Compare with example configuration",
		)
		return nil, beaconErr
	}

	return &cfg, nil
}

func NewMonitor(cfg *Config, configPath string) (*Monitor, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize rate limiter with sensible defaults
	httpClient := ratelimit.NewHTTPClient(ratelimit.DefaultConfig())
	logManager := NewLogManager(cfg, httpClient.Client)

	// Initialize key manager
	configDir := getConfigDir()
	keyManager, err := keys.NewKeyManager(configDir)
	if err != nil {
		cancel() // Ensure context is canceled on error
		return nil, fmt.Errorf("failed to initialize key manager: %w", err)
	}

	// Get current token
	currentToken, err := getCurrentToken(cfg, keyManager)
	if err != nil {
		cancel() // Ensure context is canceled on error
		return nil, fmt.Errorf("failed to get current token: %w", err)
	}

	// Initialize file watcher for config hot-reload
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Initialize plugin manager
	pluginManager := plugins.NewManager()

	// Register built-in plugins
	if err := registerBuiltinPlugins(pluginManager); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to register built-in plugins: %w", err)
	}

	return &Monitor{
		config:           cfg,
		logManager:       logManager,
		results:          make(map[string]*CheckResult),
		httpClient:       httpClient,
		ctx:              ctx,
		cancel:           cancel,
		metricsCollector: &LinuxSystemMetricsCollector{},
		keyManager:       keyManager,
		currentToken:     currentToken,
		configWatcher:    watcher,
		configPath:       configPath,
		pluginManager:    pluginManager,
	}, nil
}

// getConfigDir returns the beacon configuration directory
func getConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".beacon"
	}
	return filepath.Join(homeDir, ".beacon")
}

// getCurrentToken retrieves the current API token from config or key manager
func getCurrentToken(cfg *Config, keyManager *keys.KeyManager) (string, error) {
	// If token is directly specified in config, use it
	if cfg.Report.Token != "" {
		return cfg.Report.Token, nil
	}

	// If token_name is specified, get it from key manager
	if cfg.Report.TokenName != "" {
		storedKey, err := keyManager.GetKey(cfg.Report.TokenName)
		if err != nil {
			return "", fmt.Errorf("failed to get token '%s': %w", cfg.Report.TokenName, err)
		}
		return storedKey.Key, nil
	}

	return "", fmt.Errorf("no token or token_name specified in report configuration")
}

// registerBuiltinPlugins registers all built-in plugins with the manager
func registerBuiltinPlugins(manager *plugins.Manager) error {
	// Register Discord plugin
	if err := manager.RegisterPlugin(discord.NewDiscordPlugin()); err != nil {
		return fmt.Errorf("failed to register Discord plugin: %w", err)
	}

	// Register Telegram plugin
	if err := manager.RegisterPlugin(telegram.NewTelegramPlugin()); err != nil {
		return fmt.Errorf("failed to register Telegram plugin: %w", err)
	}

	// Register Email plugin
	if err := manager.RegisterPlugin(email.NewEmailPlugin()); err != nil {
		return fmt.Errorf("failed to register Email plugin: %w", err)
	}

	// Register Webhook plugin
	if err := manager.RegisterPlugin(webhook.NewWebhookPlugin()); err != nil {
		return fmt.Errorf("failed to register Webhook plugin: %w", err)
	}

	return nil
}

func (m *Monitor) Start() error {
	log.Printf("[Beacon] Starting monitoring system for device: %s", m.config.Device.Name)

	// Load plugin configurations
	if err := m.pluginManager.LoadConfigs(m.config.Plugins, m.config.AlertRules); err != nil {
		log.Printf("[Beacon] Warning: failed to load plugin configurations: %v", err)
	}

	// Start config hot-reload monitoring
	m.startConfigHotReload()

	// Start Prometheus metrics server if enabled
	if m.config.Report.PrometheusEnable {
		go m.startPrometheusServer()
	}

	// Start heartbeat if enabled
	if m.config.Report.Heartbeat.Enabled {
		interval := m.config.Report.Heartbeat.Interval
		if interval == 0 {
			interval = 30 * time.Second // Default 30 seconds
		}
		m.heartbeatTicker = time.NewTicker(interval)
		go m.runHeartbeatLoop()
	}

	// Start system metrics collection if enabled
	if m.config.SystemMetrics.Enabled && m.config.Report.SendTo != "" && m.config.Report.Token != "" {
		interval := m.config.SystemMetrics.Interval
		if interval == 0 {
			interval = 1 * time.Minute // Default 1 minute
		}
		m.reportSystemMetricsTicker = time.NewTicker(interval)
		go m.reportSystemMetricsLoop()
	}

	// Start log collection if enabled
	if len(m.config.LogSources) > 0 && m.config.Report.SendTo != "" && m.config.Report.Token != "" {
		m.logManager.StartLogCollection(m.ctx)
	}

	// Start all health checks
	var wg sync.WaitGroup
	for _, check := range m.config.Checks {
		wg.Add(1)
		go func(c CheckConfig) {
			defer wg.Done()
			m.runCheckLoop(c)
		}(check)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("[Beacon] Shutdown signal received, stopping monitoring...")
	m.cancel()
	wg.Wait()

	return nil
}

func (m *Monitor) runHeartbeatLoop() {
	defer m.heartbeatTicker.Stop()

	// Send initial heartbeat
	m.sendHeartbeat()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.heartbeatTicker.C:
			m.sendHeartbeat()
		}
	}
}

func (m *Monitor) runCheckLoop(check CheckConfig) {
	ticker := time.NewTicker(check.Interval)
	defer ticker.Stop()

	// Run initial check
	m.executeCheck(check)

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.executeCheck(check)
		}
	}
}

func (m *Monitor) executeCheck(check CheckConfig) {
	start := time.Now()
	var result CheckResult

	switch check.Type {
	case "http":
		result = m.executeHTTPCheck(check)
	case "port":
		result = m.executePortCheck(check)
	case "command":
		result = m.executeCommandCheck(check)
	default:
		result = CheckResult{
			Name:      check.Name,
			Type:      check.Type,
			Status:    "error",
			Timestamp: time.Now(),
			Error:     fmt.Sprintf("unknown check type: %s", check.Type),
		}
	}

	result.Duration = time.Since(start)
	result.Timestamp = time.Now()
	result.Device = m.config.Device

	// Log result
	switch check.Type {
	case "http":
		log.Printf("[Beacon] Check (%s) %s: %s (%.2fs)", check.Type, check.Name, result.Status, result.Duration.Seconds())
	case "port":
		log.Printf("[Beacon] Check (%s) %s: %s (%.2fs)", check.Type, check.Name, result.Status, result.Duration.Seconds())
	case "command":
		// Format output with truncation and whitespace normalization
		output := strings.Join(strings.Fields(result.CommandOutput), " ")
		if len(output) > maxOutputLength {
			output = output[:maxOutputLength] + "..."
		}

		// Format error with truncation and whitespace normalization
		errorMsg := strings.Join(strings.Fields(result.CommandError), " ")
		if len(errorMsg) > maxOutputLength {
			errorMsg = errorMsg[:maxOutputLength] + "..."
		}

		log.Printf(
			"[Beacon] Check (%s) %s: (%.2fs) - Output: %s, Error: %s",
			check.Type,
			check.Name,
			result.Duration.Seconds(),
			output,
			errorMsg,
		)
	}

	// Store result
	m.resultsMux.Lock()
	m.results[check.Name] = &result
	m.resultsMux.Unlock()

	// Send alert via plugin system if check failed
	if result.Status != "up" {
		// Convert monitor.CheckResult to plugins.CheckResult
		pluginResult := &plugins.CheckResult{
			Name:           result.Name,
			Type:           result.Type,
			Status:         result.Status,
			Duration:       result.Duration,
			Timestamp:      result.Timestamp,
			Error:          result.Error,
			HTTPStatusCode: result.HTTPStatusCode,
			ResponseTime:   result.ResponseTime,
			CommandOutput:  result.CommandOutput,
			CommandError:   result.CommandError,
			Device: plugins.DeviceConfig{
				Name:        result.Device.Name,
				Location:    result.Device.Location,
				Tags:        result.Device.Tags,
				Environment: result.Device.Environment,
			},
		}

		if err := m.pluginManager.SendAlert(pluginResult); err != nil {
			log.Printf("[Beacon] Failed to send alert via plugins: %v", err)
		}
	}

	// Execute alert command for command checks (always run regardless of status)
	if check.Type == "command" && check.AlertCommand != "" {
		log.Printf("[Beacon] Executing alert command for command check: %s", result.Name)
		m.executeAlertCommand(check.AlertCommand, result)
	} else if result.Status != "up" && check.AlertCommand != "" {
		// For non-command checks, only run alert command on failure
		log.Printf("[Beacon] Executing alert command for failed check: %s", result.Name)
		m.executeAlertCommand(check.AlertCommand, result)
	}
}

func (m *Monitor) executeHTTPCheck(check CheckConfig) CheckResult {
	result := CheckResult{
		Name:      check.Name,
		Type:      "http",
		Timestamp: time.Now(),
	}

	req, err := http.NewRequest("GET", check.URL, nil)
	if err != nil {
		result.Status = "error"
		beaconErr := errors.NewBeaconError(errors.ErrorTypeConfig, "Failed to create HTTP request", err).
			WithTroubleshooting(
				"Invalid URL format",
				"Malformed URL in configuration",
			).WithNextSteps(
			"Check URL format in configuration",
			"Verify URL is properly formatted",
		)
		result.Error = errors.FormatError(beaconErr)
		return result
	}

	start := time.Now()
	resp, err := m.httpClient.Do(m.ctx, req)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = "down"
		beaconErr := errors.NewHTTPError(check.URL, 0, err).
			WithTroubleshooting(
				"Network connectivity issues",
				"Service is not running",
				"DNS resolution failed",
				"Firewall blocking connection",
			).WithNextSteps(
			"Check network connectivity",
			"Verify service is running",
			"Test with curl: curl -v "+check.URL,
			"Check DNS resolution: nslookup "+extractHostname(check.URL),
		)
		result.Error = errors.FormatError(beaconErr)
		return result
	}
	defer util.DeferClose(resp.Body, "HTTP response body")()

	result.HTTPStatusCode = resp.StatusCode

	if check.ExpectStatus > 0 && resp.StatusCode != check.ExpectStatus {
		result.Status = "down"
		beaconErr := errors.NewHTTPError(check.URL, resp.StatusCode, nil).
			WithTroubleshooting(
				"Service returned unexpected status code",
				"Configuration expects different status",
				"Service may be misconfigured",
			).WithNextSteps(
			fmt.Sprintf("Check if status %d is expected", resp.StatusCode),
			"Update configuration if needed",
			"Check service logs for errors",
		)
		result.Error = errors.FormatError(beaconErr)
	} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = "up"
	} else {
		result.Status = "down"
		beaconErr := errors.NewHTTPError(check.URL, resp.StatusCode, nil)
		result.Error = errors.FormatError(beaconErr)
	}

	return result
}

// extractHostname extracts hostname from URL for DNS troubleshooting
func extractHostname(url string) string {
	if strings.HasPrefix(url, "http://") {
		url = url[7:]
	} else if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}

	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	if idx := strings.Index(url, ":"); idx != -1 {
		url = url[:idx]
	}

	return url
}

func (m *Monitor) executePortCheck(check CheckConfig) CheckResult {
	result := CheckResult{
		Name:      check.Name,
		Type:      "port",
		Timestamp: time.Now(),
	}

	var address string
	ip := net.ParseIP(check.Host)
	if ip != nil && ip.To4() == nil {
		// IPv6 address, wrap in brackets
		address = fmt.Sprintf("[%s]:%d", check.Host, check.Port)
	} else {
		address = fmt.Sprintf("%s:%d", check.Host, check.Port)
	}

	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		result.Status = "down"
		beaconErr := errors.NewConnectionError(check.Host, check.Port, err)
		result.Error = errors.FormatError(beaconErr)
		return result
	}
	defer util.DeferClose(conn, "TCP connection")()

	result.Status = "up"
	return result
}

func (m *Monitor) executeCommandCheck(check CheckConfig) CheckResult {
	result := CheckResult{
		Name:      check.Name,
		Type:      "command",
		Timestamp: time.Now(),
	}

	cmd := exec.CommandContext(m.ctx, "sh", "-c", check.Cmd)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Always capture output, regardless of success/failure
	result.CommandOutput = strings.TrimSpace(stdout.String())
	result.CommandError = strings.TrimSpace(stderr.String())

	if err != nil {
		result.Status = "down"
		beaconErr := errors.NewBeaconError(errors.ErrorTypeSystem, "Command execution failed", err).
			WithTroubleshooting(
				"Command syntax error",
				"Required program not installed",
				"Insufficient permissions",
				"Command timeout",
			).WithNextSteps(
			"Test command manually: "+check.Cmd,
			"Check if required programs are installed",
			"Verify command syntax",
			"Check file permissions",
		)
		result.Error = errors.FormatError(beaconErr)
		return result
	}

	result.Status = "up"
	return result
}

func (m *Monitor) executeAlertCommand(command string, result CheckResult) {
	if command == "" {
		return
	}

	log.Printf("[Beacon] Executing alert command for check: %s", result.Name)

	// Execute the alert command in a goroutine to avoid blocking
	go func() {
		// Replace variables in the command string directly
		expandedCommand := strings.ReplaceAll(command, "$BEACON_CHECK_NAME", result.Name)
		expandedCommand = strings.ReplaceAll(expandedCommand, "$BEACON_CHECK_TYPE", result.Type)
		expandedCommand = strings.ReplaceAll(expandedCommand, "$BEACON_CHECK_STATUS", result.Status)
		expandedCommand = strings.ReplaceAll(expandedCommand, "$BEACON_CHECK_ERROR", result.Error)
		expandedCommand = strings.ReplaceAll(expandedCommand, "$BEACON_CHECK_DURATION", fmt.Sprintf("%.2f", result.Duration.Seconds()))
		expandedCommand = strings.ReplaceAll(expandedCommand, "$BEACON_DEVICE_NAME", result.Device.Name)

		// Add command output for command-type checks
		if result.Type == "command" {
			expandedCommand = strings.ReplaceAll(expandedCommand, "$BEACON_CHECK_OUTPUT", result.CommandOutput)
		}

		cmd := exec.CommandContext(m.ctx, "sh", "-c", expandedCommand)

		// Capture output for logging
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		if err != nil {
			log.Printf("[Beacon] Alert command failed: %v, stderr: %s", err, stderr.String())
		} else {
			log.Printf("[Beacon] Alert command executed successfully: %s", stdout.String())
		}
	}()
}

func (m *Monitor) reportSystemMetricsLoop() {
	defer m.reportSystemMetricsTicker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.reportSystemMetricsTicker.C:
			m.reportSystemMetrics()
		}
	}
}

func (m *Monitor) reportSystemMetrics() {
	if m.config.Report.SendTo == "" || m.config.Report.Token == "" {
		return
	}

	hostname, err := m.metricsCollector.GetHostname()
	if err != nil {
		log.Printf("[Beacon] Failed to get hostname: %v", err)
		return
	}

	ipAddress, err := m.metricsCollector.GetIPAddress()
	if err != nil {
		log.Printf("[Beacon] Failed to get IP address: %v", err)
		ipAddress = "unknown"
	}

	// Initialize metrics with basic info
	metrics := AgentMetrics{
		Hostname:      hostname,
		IPAddress:     ipAddress,
		Timestamp:     time.Now(),
		CustomMetrics: make(map[string]*CheckResult),
	}

	// Collect only enabled system metrics
	if m.config.SystemMetrics.CPU {
		if cpuUsage, err := m.metricsCollector.GetCPUUsage(); err == nil {
			metrics.CPUUsage = cpuUsage
		}
	}

	if m.config.SystemMetrics.Memory {
		if memoryUsage, err := m.metricsCollector.GetMemoryUsage(); err == nil {
			metrics.MemoryUsage = memoryUsage
		}
	}

	if m.config.SystemMetrics.Disk {
		diskPath := m.config.SystemMetrics.DiskPath
		if diskPath == "" {
			diskPath = "/"
		}
		if diskUsage, err := m.metricsCollector.GetDiskUsage(diskPath); err == nil {
			metrics.DiskUsage = diskUsage
		}
	}

	if m.config.SystemMetrics.LoadAverage {
		if loadAverage, err := m.metricsCollector.GetLoadAverage(); err == nil {
			metrics.LoadAverage = loadAverage
		}
	}

	// Always collect uptime
	if uptime, err := m.metricsCollector.GetUptime(); err == nil {
		metrics.UptimeSeconds = uptime
	}

	// Add custom check results
	m.resultsMux.RLock()
	for _, result := range m.results {
		metrics.CustomMetrics[result.Name] = result
	}
	m.resultsMux.RUnlock()

	// Send to /agent/metrics endpoint
	metricsURL := strings.TrimSuffix(m.config.Report.SendTo, "/") + "/agent/metrics"
	m.sendToAPI(metricsURL, metrics)
}

func (m *Monitor) sendHeartbeat() {
	if !m.config.Report.Heartbeat.Enabled {
		return
	}

	hostname, err := getHostname()
	if err != nil {
		log.Printf("[Beacon] Failed to get hostname for heartbeat: %v", err)
		return
	}

	ipAddress, err := getIPAddress()
	if err != nil {
		log.Printf("[Beacon] Failed to get IP address for heartbeat: %v", err)
		ipAddress = "unknown"
	}

	heartbeat := AgentHeartbeatRequest{
		Hostname:     hostname,
		IPAddress:    ipAddress,
		Tags:         m.config.Device.Tags,
		AgentVersion: version.GetVersion(), // Get from build info
		DeviceName:   m.config.Device.Name,
		Metadata: map[string]string{
			"location":    m.config.Device.Location,
			"environment": m.config.Device.Environment,
			"status":      "alive",
		},
	}

	// Send to /agent/heartbeat endpoint
	heartbeatURL := strings.TrimSuffix(m.config.Report.SendTo, "/") + "/agent/heartbeat"
	m.sendToAPI(heartbeatURL, heartbeat)
}

func (m *Monitor) sendToAPI(url string, payload interface{}) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Beacon] Failed to marshal payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		log.Printf("[Beacon] Failed to create API request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", m.currentToken)

	resp, err := m.httpClient.Do(m.ctx, req)
	if err != nil {
		log.Printf("[Beacon] Failed to send to API: %v", err)
		return
	}
	defer util.DeferClose(resp.Body, "HTTP response body")()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Beacon] Successfully sent data to %s", url)
	} else {
		log.Printf("[Beacon] API request failed: HTTP %d", resp.StatusCode)
	}
}

func (m *Monitor) startPrometheusServer() {
	http.HandleFunc("/metrics", m.prometheusHandler)

	addr := fmt.Sprintf(":%d", m.config.Report.PrometheusPort)
	log.Printf("[Beacon] Prometheus metrics server listening on %s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("[Beacon] Prometheus server error: %v", err)
	}
}

func (m *Monitor) prometheusHandler(w http.ResponseWriter, r *http.Request) {
	m.resultsMux.RLock()
	defer m.resultsMux.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	for _, result := range m.results {
		// Status metric (1 = up, 0 = down/error)
		status := 0
		if result.Status == "up" {
			status = 1
		}

		// Build device labels for metrics
		deviceLabels := fmt.Sprintf("name=\"%s\",type=\"%s\"", result.Name, result.Type)
		if result.Device.Name != "" {
			deviceLabels += fmt.Sprintf(",device=\"%s\"", result.Device.Name)
		}
		if result.Device.Location != "" {
			deviceLabels += fmt.Sprintf(",location=\"%s\"", result.Device.Location)
		}
		if result.Device.Environment != "" {
			deviceLabels += fmt.Sprintf(",environment=\"%s\"", result.Device.Environment)
		}

		util.LogError(func() error {
			_, err := fmt.Fprintf(w, "beacon_check_status{%s} %d\n", deviceLabels, status)
			return err
		}(), "write metrics")

		// Duration metric
		util.LogError(func() error {
			_, err := fmt.Fprintf(w, "beacon_check_duration_seconds{%s} %.3f\n", deviceLabels, result.Duration.Seconds())
			return err
		}(), "write metrics")

		// Response time for HTTP checks
		if result.Type == "http" && result.ResponseTime > 0 {
			util.LogError(func() error {
				_, err := fmt.Fprintf(w, "beacon_check_response_time_seconds{%s} %.3f\n", deviceLabels, result.ResponseTime.Seconds())
				return err
			}(), "write metrics")
		}

		// Last check timestamp
		util.LogError(func() error {
			_, err := fmt.Fprintf(w, "beacon_check_last_check_timestamp{%s} %d\n", deviceLabels, result.Timestamp.Unix())
			return err
		}(), "write metrics")
	}
}

func Run(cmd *cobra.Command, args []string) {
	// Determine config file path
	configPath := "beacon.monitor.yml"

	// Check for --config flag first
	if configFlag := cmd.Flag("config"); configFlag != nil && configFlag.Value.String() != "" {
		configPath = configFlag.Value.String()
	} else if len(args) > 0 {
		// Fall back to positional argument
		configPath = args[0]
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cmd.Printf("Error: Configuration file '%s' not found.\n", configPath)
		cmd.Printf("Please create a configuration file or use the example: beacon.monitor.example.yml\n")
		os.Exit(1)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		cmd.Printf("failed to load config: %v", err)
		os.Exit(1)
	}

	// Create and start monitor
	monitor, err := NewMonitor(cfg, configPath)
	if err != nil {
		cmd.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if err := monitor.Start(); err != nil {
		cmd.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// Stop stops the monitor and cleans up resources
func (m *Monitor) Stop() {
	log.Printf("[Beacon] Stopping monitoring system")

	if m.configWatcher != nil {
		util.Close(m.configWatcher, "config watcher")
	}

	if m.heartbeatTicker != nil {
		m.heartbeatTicker.Stop()
	}

	if m.reportSystemMetricsTicker != nil {
		m.reportSystemMetricsTicker.Stop()
	}

	if m.pluginManager != nil {
		if err := m.pluginManager.Close(); err != nil {
			log.Printf("[Beacon] Error closing plugin manager: %v", err)
		}
	}

	m.cancel()
}

// startConfigHotReload starts monitoring the configuration file for changes
func (m *Monitor) startConfigHotReload() {
	// Watch the config file
	err := m.configWatcher.Add(m.configPath)
	if err != nil {
		log.Printf("[Beacon] Failed to watch config file: %v", err)
		return
	}

	// Watch the keys directory for key changes
	keysDir := filepath.Join(getConfigDir(), "keys")
	if err := m.configWatcher.Add(keysDir); err != nil {
		log.Printf("[Beacon] Failed to watch keys directory: %v", err)
	}

	go func() {
		for {
			select {
			case event, ok := <-m.configWatcher.Events:
				if !ok {
					return
				}
				m.handleConfigChange(event)
			case err, ok := <-m.configWatcher.Errors:
				if !ok {
					return
				}
				log.Printf("[Beacon] Config watcher error: %v", err)
			case <-m.ctx.Done():
				return
			}
		}
	}()

	log.Printf("[Beacon] Started config hot-reload monitoring")
}

// handleConfigChange handles configuration file changes
func (m *Monitor) handleConfigChange(event fsnotify.Event) {
	// Debounce rapid changes
	time.Sleep(100 * time.Millisecond)

	switch {
	case event.Op&fsnotify.Write == fsnotify.Write:
		if event.Name == m.configPath {
			log.Printf("[Beacon] Config file changed, reloading...")
			m.reloadConfig()
		} else if strings.Contains(event.Name, "keys/") && strings.HasSuffix(event.Name, ".json") {
			log.Printf("[Beacon] Key file changed, reloading token...")
			m.reloadToken()
		}
	case event.Op&fsnotify.Create == fsnotify.Create:
		if strings.Contains(event.Name, "keys/") && strings.HasSuffix(event.Name, ".json") {
			log.Printf("[Beacon] New key file created, checking for token updates...")
			m.reloadToken()
		}
	}
}

// reloadConfig reloads the configuration file
func (m *Monitor) reloadConfig() {
	newConfig, err := LoadConfig(m.configPath)
	if err != nil {
		log.Printf("[Beacon] Failed to reload config: %v", err)
		return
	}

	// Update configuration
	m.config = newConfig

	// Reload plugin configurations
	if err := m.pluginManager.LoadConfigs(m.config.Plugins, m.config.AlertRules); err != nil {
		log.Printf("[Beacon] Failed to reload plugin configurations: %v", err)
	}

	log.Printf("[Beacon] Configuration reloaded successfully")
}

// reloadToken reloads the current token from the key manager
func (m *Monitor) reloadToken() {
	if m.config.Report.TokenName == "" {
		return // No token reload for hardcoded tokens
	}

	newToken, err := getCurrentToken(m.config, m.keyManager)
	if err != nil {
		log.Printf("[Beacon] Failed to reload token: %v", err)
		return
	}

	if newToken != m.currentToken {
		m.currentToken = newToken
		log.Printf("[Beacon] Token reloaded successfully")
	}
}
