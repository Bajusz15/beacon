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
	"strings"
	"sync"
	"syscall"
	"time"

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
	Device        DeviceConfig        `yaml:"device"`
	Checks        []CheckConfig       `yaml:"checks"`
	SystemMetrics SystemMetricsConfig `yaml:"system_metrics,omitempty"`
	LogSources    []LogSource         `yaml:"log_sources,omitempty"`
	Report        ReportConfig        `yaml:"report"`
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
	Token            string          `yaml:"token"`
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
	results                   map[string]*CheckResult
	resultsMux                sync.RWMutex
	logManager                *LogManager
	httpClient                *http.Client
	ctx                       context.Context
	cancel                    context.CancelFunc
	heartbeatTicker           *time.Ticker
	reportSystemMetricsTicker *time.Ticker
	metricsCollector          SystemMetricsCollector
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
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func NewMonitor(cfg *Config) (*Monitor, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &Monitor{
		config:           cfg,
		results:          make(map[string]*CheckResult),
		logManager:       NewLogManager(cfg, httpClient),
		httpClient:       httpClient,
		ctx:              ctx,
		cancel:           cancel,
		metricsCollector: &LinuxSystemMetricsCollector{},
	}, nil
}

func (m *Monitor) Start() error {
	log.Printf("[Beacon] Starting monitoring system for device: %s", m.config.Device.Name)

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

	// Start log collection if configured
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
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		return result
	}

	start := time.Now()
	resp, err := m.httpClient.Do(req)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = "down"
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatusCode = resp.StatusCode

	if check.ExpectStatus > 0 && resp.StatusCode != check.ExpectStatus {
		result.Status = "down"
		result.Error = fmt.Sprintf("expected status %d, got %d", check.ExpectStatus, resp.StatusCode)
	} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = "up"
	} else {
		result.Status = "down"
		result.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
	}

	return result
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
		result.Error = fmt.Sprintf("connection failed: %v", err)
		return result
	}
	defer conn.Close()

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
		result.Error = fmt.Sprintf("command failed: %v", err)
		return result
	}

	result.Status = "up"
	return result
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
		AgentVersion: "1.0.0", // TODO: Get from build info
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
	req.Header.Set("X-API-Key", m.config.Report.Token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		log.Printf("[Beacon] Failed to send to API: %v", err)
		return
	}
	defer resp.Body.Close()

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

		fmt.Fprintf(w, "beacon_check_status{%s} %d\n", deviceLabels, status)

		// Duration metric
		fmt.Fprintf(w, "beacon_check_duration_seconds{%s} %.3f\n",
			deviceLabels, result.Duration.Seconds())

		// Response time for HTTP checks
		if result.Type == "http" && result.ResponseTime > 0 {
			fmt.Fprintf(w, "beacon_check_response_time_seconds{%s} %.3f\n",
				deviceLabels, result.ResponseTime.Seconds())
		}

		// Last check timestamp
		fmt.Fprintf(w, "beacon_check_last_check_timestamp{%s} %d\n",
			deviceLabels, result.Timestamp.Unix())
	}
}

func Run(cmd *cobra.Command, args []string) {
	// Determine config file path
	configPath := "beacon.monitor.yml"
	if len(args) > 0 {
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
	monitor, err := NewMonitor(cfg)
	if err != nil {
		cmd.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if err := monitor.Start(); err != nil {
		cmd.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
