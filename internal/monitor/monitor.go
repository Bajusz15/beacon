package monitor

import (
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

type Config struct {
	Checks []CheckConfig `yaml:"checks"`
	Report ReportConfig  `yaml:"report"`
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

type ReportConfig struct {
	SendTo           string `yaml:"send_to"`
	Token            string `yaml:"token"`
	PrometheusEnable bool   `yaml:"prometheus_metrics"`
	PrometheusPort   int    `yaml:"prometheus_port"`
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
}

type Monitor struct {
	config     *Config
	results    map[string]*CheckResult
	resultsMux sync.RWMutex
	httpClient *http.Client
	ctx        context.Context
	cancel     context.CancelFunc
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
	return nil
}

func NewMonitor(configPath string) (*Monitor, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Monitor{
		config:  cfg,
		results: make(map[string]*CheckResult),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (m *Monitor) Start() error {
	log.Println("[Beacon] Starting monitoring system...")

	// Start Prometheus metrics server if enabled
	if m.config.Report.PrometheusEnable {
		go m.startPrometheusServer()
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

	// Store result
	m.resultsMux.Lock()
	m.results[check.Name] = &result
	m.resultsMux.Unlock()

	// Log result
	log.Printf("[Beacon] Check %s: %s (%.2fs)", check.Name, result.Status, result.Duration.Seconds())

	// Report to external API if configured
	if m.config.Report.SendTo != "" {
		go m.reportResult(result)
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

	address := fmt.Sprintf("%s:%d", check.Host, check.Port)
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

	// Split command into command and arguments
	parts := strings.Fields(check.Cmd)
	if len(parts) == 0 {
		result.Status = "error"
		result.Error = "empty command"
		return result
	}

	cmd := exec.CommandContext(m.ctx, parts[0], parts[1:]...)
	err := cmd.Run()

	if err != nil {
		result.Status = "down"
		result.Error = fmt.Sprintf("command failed: %v", err)
		return result
	}

	result.Status = "up"
	return result
}

func (m *Monitor) reportResult(result CheckResult) {
	if m.config.Report.SendTo == "" || m.config.Report.Token == "" {
		return
	}

	payload := map[string]interface{}{
		"check": result,
		"token": m.config.Report.Token,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Beacon] Failed to marshal result: %v", err)
		return
	}

	req, err := http.NewRequest("POST", m.config.Report.SendTo, strings.NewReader(string(jsonData)))
	if err != nil {
		log.Printf("[Beacon] Failed to create report request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.config.Report.Token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		log.Printf("[Beacon] Failed to send report: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Beacon] Successfully reported result for %s", result.Name)
	} else {
		log.Printf("[Beacon] Failed to report result for %s: HTTP %d", result.Name, resp.StatusCode)
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

		fmt.Fprintf(w, "beacon_check_status{name=\"%s\",type=\"%s\"} %d\n",
			result.Name, result.Type, status)

		// Duration metric
		fmt.Fprintf(w, "beacon_check_duration_seconds{name=\"%s\",type=\"%s\"} %.3f\n",
			result.Name, result.Type, result.Duration.Seconds())

		// Response time for HTTP checks
		if result.Type == "http" && result.ResponseTime > 0 {
			fmt.Fprintf(w, "beacon_check_response_time_seconds{name=\"%s\",type=\"%s\"} %.3f\n",
				result.Name, result.Type, result.ResponseTime.Seconds())
		}

		// Last check timestamp
		fmt.Fprintf(w, "beacon_check_last_check_timestamp{name=\"%s\",type=\"%s\"} %d\n",
			result.Name, result.Type, result.Timestamp.Unix())
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

	// Create and start monitor
	monitor, err := NewMonitor(configPath)
	if err != nil {
		cmd.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if err := monitor.Start(); err != nil {
		cmd.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
