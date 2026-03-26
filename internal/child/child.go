// Package child implements the child agent that runs health checks and reports to the master via IPC.
// The child agent is spawned by the master with `beacon agent` and never communicates directly with the cloud.
package child

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

	"beacon/internal/config"
	"beacon/internal/ipc"
	"beacon/internal/monitor"
	"beacon/internal/state"
)

const (
	healthWriteInterval = 10 * time.Second
	commandPollInterval = 1 * time.Second
)

// Config holds the configuration for the child agent.
type Config struct {
	ProjectID  string
	ConfigPath string
	IPCDir     string
}

// Child represents a child agent process that runs health checks and reports via IPC.
type Child struct {
	cfg        *Config
	monitorCfg *monitor.Config
	ipcWriter  *ipc.Writer
	startedAt  time.Time

	results    map[string]*checkResult
	resultsMux sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

type checkResult struct {
	Name      string
	Passed    bool
	LatencyMs int64
	Error     string
	Timestamp time.Time
}

// getConfigDir returns the beacon configuration directory (~/.beacon or $BEACON_HOME).
func getConfigDir() string {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return ".beacon"
	}
	return base
}

// projectNameFromConfigPath derives project name from config path.
// For paths like ~/.beacon/config/projects/<name>/monitor.yml it returns <name>.
func projectNameFromConfigPath(configPath string, fallbackDeviceName string) string {
	dir := filepath.Dir(configPath)
	parent := filepath.Base(filepath.Dir(dir))
	if parent == "projects" {
		return filepath.Base(dir)
	}
	if fallbackDeviceName != "" {
		return fallbackDeviceName
	}
	return "default"
}

// readDeployedAt attempts to load last deployment time from ~/.beacon/state/<project>/status.json.
// If the file is missing/invalid (or last_deployed is zero), it returns nil.
func (c *Child) readDeployedAt() *time.Time {
	if c == nil || c.cfg == nil || c.cfg.ConfigPath == "" {
		return nil
	}

	fallbackDeviceName := ""
	if c.monitorCfg != nil {
		fallbackDeviceName = c.monitorCfg.Device.Name
	}

	projectName := projectNameFromConfigPath(c.cfg.ConfigPath, fallbackDeviceName)
	statusFile := filepath.Join(getConfigDir(), "state", projectName, "status.json")

	// Avoid creating directories during periodic health writes.
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil
	}

	var st state.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	if st.LastDeployed.IsZero() {
		return nil
	}

	t := st.LastDeployed.UTC()
	return &t
}

// New creates a new child agent with the given configuration.
func New(cfg *Config) (*Child, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("project-id is required")
	}
	if cfg.ConfigPath == "" {
		return nil, fmt.Errorf("config path is required")
	}
	if cfg.IPCDir == "" {
		return nil, fmt.Errorf("ipc-dir is required")
	}

	monitorCfg, err := monitor.LoadConfig(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load project config: %w", err)
	}

	ipcWriter, err := ipc.NewWriter(cfg.IPCDir)
	if err != nil {
		return nil, fmt.Errorf("create IPC writer: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Child{
		cfg:        cfg,
		monitorCfg: monitorCfg,
		ipcWriter:  ipcWriter,
		startedAt:  time.Now(),
		results:    make(map[string]*checkResult),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Run starts the child agent and blocks until shutdown.
func (c *Child) Run() error {
	log.Printf("[Beacon child] Starting for project: %s", c.cfg.ProjectID)
	log.Printf("[Beacon child] Config: %s", c.cfg.ConfigPath)
	log.Printf("[Beacon child] IPC dir: %s", c.cfg.IPCDir)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run all checks once synchronously before starting loops
	// This ensures we have initial data for the first health report
	for _, check := range c.monitorCfg.Checks {
		c.executeCheck(check)
	}
	// Write initial health report with check results
	c.writeHealthReport()

	// Start health check loops for each configured check
	var wg sync.WaitGroup
	for _, check := range c.monitorCfg.Checks {
		wg.Add(1)
		go func(chk monitor.CheckConfig) {
			defer wg.Done()
			c.runCheckLoop(chk)
		}(check)
	}

	// Start health report writer loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.runHealthWriteLoop()
	}()

	// Start command polling loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.runCommandPollLoop()
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Printf("[Beacon child] Shutdown signal received, stopping...")

	c.cancel()
	wg.Wait()

	log.Printf("[Beacon child] Stopped")
	return nil
}

// runCheckLoop runs a single health check on its configured interval.
// Note: initial check is run synchronously in Run() before this loop starts.
func (c *Child) runCheckLoop(check monitor.CheckConfig) {
	ticker := time.NewTicker(check.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.executeCheck(check)
		}
	}
}

// executeCheck runs a single health check and stores the result.
func (c *Child) executeCheck(check monitor.CheckConfig) {
	start := time.Now()
	var result checkResult
	result.Name = check.Name
	result.Timestamp = time.Now()

	switch check.Type {
	case "http":
		result = c.executeHTTPCheck(check)
	case "port":
		result = c.executePortCheck(check)
	case "command":
		result = c.executeCommandCheck(check)
	default:
		result.Passed = false
		result.Error = fmt.Sprintf("unknown check type: %s", check.Type)
	}

	result.LatencyMs = time.Since(start).Milliseconds()

	c.resultsMux.Lock()
	c.results[check.Name] = &result
	c.resultsMux.Unlock()

	status := "passed"
	if !result.Passed {
		status = "failed"
	}
	log.Printf("[Beacon child] Check %s (%s): %s (%dms)", check.Name, check.Type, status, result.LatencyMs)
}

func (c *Child) executeHTTPCheck(check monitor.CheckConfig) checkResult {
	result := checkResult{
		Name:      check.Name,
		Timestamp: time.Now(),
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(check.URL)
	if err != nil {
		result.Passed = false
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	if check.ExpectStatus > 0 {
		result.Passed = resp.StatusCode == check.ExpectStatus
		if !result.Passed {
			result.Error = fmt.Sprintf("expected status %d, got %d", check.ExpectStatus, resp.StatusCode)
		}
	} else {
		result.Passed = resp.StatusCode >= 200 && resp.StatusCode < 300
		if !result.Passed {
			result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	return result
}

func (c *Child) executePortCheck(check monitor.CheckConfig) checkResult {
	result := checkResult{
		Name:      check.Name,
		Timestamp: time.Now(),
	}

	var address string
	ip := net.ParseIP(check.Host)
	if ip != nil && ip.To4() == nil {
		address = fmt.Sprintf("[%s]:%d", check.Host, check.Port)
	} else {
		address = fmt.Sprintf("%s:%d", check.Host, check.Port)
	}

	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		result.Passed = false
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	result.Passed = true
	return result
}

func (c *Child) executeCommandCheck(check monitor.CheckConfig) checkResult {
	result := checkResult{
		Name:      check.Name,
		Timestamp: time.Now(),
	}

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", check.Cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("%v: %s", err, strings.TrimSpace(stderr.String()))
		return result
	}

	result.Passed = true
	return result
}

// runHealthWriteLoop writes health reports to IPC every healthWriteInterval.
// Note: initial health report is written synchronously in Run() before this loop starts.
func (c *Child) runHealthWriteLoop() {
	ticker := time.NewTicker(healthWriteInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.writeHealthReport()
		}
	}
}

// writeHealthReport builds and writes the health report to IPC.
func (c *Child) writeHealthReport() {
	c.resultsMux.RLock()
	defer c.resultsMux.RUnlock()

	checks := make([]ipc.CheckResult, 0, len(c.results))
	allPassing := true
	allFailing := len(c.results) > 0
	hasChecks := len(c.results) > 0

	for _, r := range c.results {
		checks = append(checks, ipc.CheckResult{
			Name:      r.Name,
			Passed:    r.Passed,
			LatencyMs: r.LatencyMs,
			Error:     r.Error,
		})
		if !r.Passed {
			allPassing = false
		} else {
			allFailing = false
		}
	}

	// Determine status
	var status string
	switch {
	case !hasChecks:
		status = ipc.StatusUnknown
	case allPassing:
		status = ipc.StatusHealthy
	case allFailing:
		status = ipc.StatusDown
	default:
		status = ipc.StatusDegraded
	}

	report := &ipc.HealthReport{
		ProjectID:     c.cfg.ProjectID,
		Timestamp:     time.Now(),
		Status:        status,
		UptimeSeconds: int64(time.Since(c.startedAt).Seconds()),
		DeployedAt:    c.readDeployedAt(),
		Checks:        checks,
	}

	if err := c.ipcWriter.WriteHealth(report); err != nil {
		log.Printf("[Beacon child] Failed to write health report: %v", err)
	}
}

// runCommandPollLoop polls for commands from the master via IPC.
func (c *Child) runCommandPollLoop() {
	ticker := time.NewTicker(commandPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.checkForCommand()
		}
	}
}

// checkForCommand reads and executes any pending command.
func (c *Child) checkForCommand() {
	cmd, err := c.ipcWriter.ReadCommand()
	if err != nil {
		log.Printf("[Beacon child] Failed to read command: %v", err)
		return
	}
	if cmd == nil {
		return // No command
	}

	log.Printf("[Beacon child] Received command: %s (id=%s)", cmd.Action, cmd.ID)
	result := c.executeCommand(cmd)

	if err := c.ipcWriter.WriteCommandResult(result); err != nil {
		log.Printf("[Beacon child] Failed to write command result: %v", err)
	}
}

// executeCommand executes a command and returns the result.
func (c *Child) executeCommand(cmd *ipc.Command) *ipc.CommandResult {
	result := &ipc.CommandResult{
		CommandID: cmd.ID,
		Timestamp: time.Now(),
	}

	switch cmd.Action {
	case ipc.ActionHealthCheck:
		c.runAllChecksNow()
		result.Status = ipc.ResultSuccess
		result.Message = "Health checks executed"

	case ipc.ActionFetchLogs:
		lines := 100
		if l, ok := cmd.Payload["lines"].(float64); ok {
			lines = int(l)
		}
		result.Status = ipc.ResultSuccess
		result.Message = fmt.Sprintf("Fetched %d log lines", lines)
		result.Data = c.fetchLogs(lines)

	case ipc.ActionRestart:
		result.Status = ipc.ResultFailed
		result.Message = "Restart not implemented - no restart command configured"

	case ipc.ActionStop:
		result.Status = ipc.ResultFailed
		result.Message = "Stop not implemented - no stop command configured"

	default:
		result.Status = ipc.ResultFailed
		result.Message = fmt.Sprintf("Unknown action: %s", cmd.Action)
	}

	log.Printf("[Beacon child] Command %s completed: %s - %s", cmd.ID, result.Status, result.Message)
	return result
}

// runAllChecksNow triggers all health checks immediately.
func (c *Child) runAllChecksNow() {
	for _, check := range c.monitorCfg.Checks {
		c.executeCheck(check)
	}
	c.writeHealthReport()
}

// fetchLogs returns the last N lines from configured log sources.
func (c *Child) fetchLogs(lines int) []string {
	// For now, return empty - log tailing can be added later
	// This would read from c.monitorCfg.LogSources
	return []string{}
}
