package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LogSource represents a log collection source
type LogSource struct {
	Name     string        `yaml:"name"`
	Type     string        `yaml:"type"` // "file", "docker", "deploy", "command"
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
	MaxLines int           `yaml:"max_lines,omitempty"`
	MaxSize  string        `yaml:"max_size,omitempty"` // e.g., "10MB"

	// File-based logging
	FilePath   string `yaml:"file_path,omitempty"`
	FollowFile bool   `yaml:"follow_file,omitempty"` // tail -f behavior

	// Docker logging
	Containers    []string `yaml:"containers,omitempty"`     // specific containers
	AllContainers bool     `yaml:"all_containers,omitempty"` // all running containers
	DockerOptions string   `yaml:"docker_options,omitempty"` // additional docker logs options

	// Deploy logging (captures deploy command output)
	DeployLogFile string `yaml:"deploy_log_file,omitempty"` // file to write deploy output

	// Command logging
	Command string `yaml:"command,omitempty"`

	// Filtering
	IncludePatterns []string `yaml:"include_patterns,omitempty"` // regex patterns to include
	ExcludePatterns []string `yaml:"exclude_patterns,omitempty"` // regex patterns to exclude
}

// LogEntry represents a single log entry
type LogEntry struct {
	Source    string    `json:"source"`
	Type      string    `json:"type"`
	Container string    `json:"container,omitempty"` // for docker logs
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level,omitempty"` // parsed log level if detected
}

// LogCollector manages log collection for a specific source
type LogCollector struct {
	source        LogSource
	lastPosition  int64 // for file following
	lastTimestamp time.Time
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
}

// LogManager handles all log collection and forwarding
type LogManager struct {
	config        *Config
	logs          []LogEntry
	logsMux       sync.RWMutex
	logCollectors map[string]*LogCollector
	httpClient    *http.Client
}

// NewLogManager creates a new log manager
func NewLogManager(config *Config, httpClient *http.Client) *LogManager {
	return &LogManager{
		config:        config,
		logs:          make([]LogEntry, 0),
		logCollectors: make(map[string]*LogCollector),
		httpClient:    httpClient,
	}
}

// StartLogCollection starts all configured log sources
func (lm *LogManager) StartLogCollection(ctx context.Context) {
	log.Printf("[Beacon] Starting log collection for %d sources", len(lm.config.LogSources))

	for _, source := range lm.config.LogSources {
		if !source.Enabled {
			continue
		}

		logCollector := &LogCollector{
			source:  source,
			running: true,
		}
		logCollector.ctx, logCollector.cancel = context.WithCancel(ctx)

		lm.logCollectors[source.Name] = logCollector

		switch source.Type {
		case "file":
			go lm.runFileLogCollection(logCollector)
		case "docker":
			go lm.runDockerLogCollection(logCollector)
		case "deploy":
			go lm.runDeployLogCollection(logCollector)
		case "command":
			go lm.runCommandLogCollection(logCollector)
		default:
			log.Printf("[Beacon] Unknown log source type: %s", source.Type)
		}
	}
}

// StopLogCollection stops all log collectors
func (lm *LogManager) StopLogCollection() {
	for _, collector := range lm.logCollectors {
		if collector.cancel != nil {
			collector.cancel()
		}
	}
}

// runFileLogCollection handles file-based log collection
func (lm *LogManager) runFileLogCollection(collector *LogCollector) {
	source := collector.source
	log.Printf("[Beacon] Starting file log collection: %s -> %s", source.Name, source.FilePath)

	ticker := time.NewTicker(source.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-collector.ctx.Done():
			return
		case <-ticker.C:
			entries := lm.collectFileLog(source)
			if len(entries) > 0 {
				lm.addLogEntries(entries)
			}
		}
	}
}

// collectFileLog reads log entries from a file
func (lm *LogManager) collectFileLog(source LogSource) []LogEntry {
	maxLines := source.MaxLines
	if maxLines == 0 {
		maxLines = 100
	}

	var cmd *exec.Cmd
	if source.FollowFile {
		// Use tail -n for recent lines
		cmd = exec.Command("tail", "-n", fmt.Sprintf("%d", maxLines), source.FilePath)
	} else {
		// Use head for beginning of file or tail for end
		cmd = exec.Command("tail", "-n", fmt.Sprintf("%d", maxLines), source.FilePath)
	}

	output, err := cmd.Output()
	if err != nil {
		log.Printf("[Beacon] Error reading file log %s: %v", source.FilePath, err)
		return nil
	}

	lines := strings.Split(string(output), "\n")
	var entries []LogEntry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if lm.shouldIncludeLogLine(line, source) {
			entry := LogEntry{
				Source:    source.Name,
				Type:      source.Type,
				Content:   line,
				Timestamp: time.Now(),
				Level:     lm.detectLogLevel(line),
			}
			entries = append(entries, entry)
		}
	}

	return entries
}

// runDockerLogCollection handles Docker container log collection
func (lm *LogManager) runDockerLogCollection(collector *LogCollector) {
	source := collector.source
	log.Printf("[Beacon] Starting docker log collection: %s", source.Name)

	// Initialize lastTimestamp to avoid getting all historical logs on first run
	collector.lastTimestamp = time.Now().Add(-source.Interval)

	ticker := time.NewTicker(source.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-collector.ctx.Done():
			return
		case <-ticker.C:
			entries := lm.collectDockerLogSince(source, collector.lastTimestamp)
			if len(entries) > 0 {
				lm.addLogEntries(entries)
				// Update lastTimestamp to the most recent log entry
				for _, entry := range entries {
					if entry.Timestamp.After(collector.lastTimestamp) {
						collector.lastTimestamp = entry.Timestamp
					}
				}
			}
		}
	}
}

// collectDockerLogSince reads log entries from Docker containers since a specific timestamp
func (lm *LogManager) collectDockerLogSince(source LogSource, since time.Time) []LogEntry {
	var entries []LogEntry

	containers := source.Containers
	if source.AllContainers {
		// Get all running containers
		cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
		output, err := cmd.Output()
		if err != nil {
			log.Printf("[Beacon] Error getting docker containers: %v", err)
			return nil
		}
		containers = strings.Split(strings.TrimSpace(string(output)), "\n")
	}

	for _, container := range containers {
		if container == "" {
			continue
		}

		// Use --since with timestamp instead of --tail
		sinceStr := since.Format("2006-01-02T15:04:05")
		args := []string{"logs", "--since", sinceStr, "--timestamps"}

		// Add any additional docker options
		if source.DockerOptions != "" {
			optArgs := strings.Fields(source.DockerOptions)
			// Filter out --tail and --since options to avoid conflicts
			filteredOpts := []string{}
			skipNext := false
			for i, opt := range optArgs {
				if skipNext {
					skipNext = false
					continue
				}
				if opt == "--tail" || opt == "--since" {
					skipNext = true
					continue
				}
				if strings.HasPrefix(opt, "--tail=") || strings.HasPrefix(opt, "--since=") {
					continue
				}
				filteredOpts = append(filteredOpts, opt)
			}
			args = append(args, filteredOpts...)
		}
		args = append(args, container)

		cmd := exec.Command("docker", args...)
		output, err := cmd.Output()
		if err != nil {
			log.Printf("[Beacon] Error getting docker logs for %s: %v", container, err)
			continue
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Parse Docker timestamp format: 2024-01-15T10:30:00.123456789Z message
			var logTimestamp time.Time
			var logContent string

			parts := strings.SplitN(line, " ", 2)
			if len(parts) >= 2 {
				if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
					logTimestamp = ts
					logContent = parts[1]
				} else {
					// Fallback if timestamp parsing fails
					logTimestamp = time.Now()
					logContent = line
				}
			} else {
				logTimestamp = time.Now()
				logContent = line
			}

			// Only include logs newer than our last timestamp
			if logTimestamp.After(since) && lm.shouldIncludeLogLine(logContent, source) {
				entry := LogEntry{
					Source:    source.Name,
					Type:      source.Type,
					Container: container,
					Content:   logContent,
					Timestamp: logTimestamp,
					Level:     lm.detectLogLevel(logContent),
				}
				entries = append(entries, entry)
			}
		}
	}

	return entries
}

// collectDockerLog reads log entries from Docker containers (legacy method)
func (lm *LogManager) collectDockerLog(source LogSource) []LogEntry {
	var entries []LogEntry

	containers := source.Containers
	if source.AllContainers {
		// Get all running containers
		cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
		output, err := cmd.Output()
		if err != nil {
			log.Printf("[Beacon] Error getting docker containers: %v", err)
			return nil
		}
		containers = strings.Split(strings.TrimSpace(string(output)), "\n")
	}

	for _, container := range containers {
		if container == "" {
			continue
		}

		maxLines := source.MaxLines
		if maxLines == 0 {
			maxLines = 50
		}

		args := []string{"logs", "--tail", fmt.Sprintf("%d", maxLines)}
		if source.DockerOptions != "" {
			optArgs := strings.Fields(source.DockerOptions)
			args = append(args, optArgs...)
		}
		args = append(args, container)

		cmd := exec.Command("docker", args...)
		output, err := cmd.Output()
		if err != nil {
			log.Printf("[Beacon] Error getting docker logs for %s: %v", container, err)
			continue
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if lm.shouldIncludeLogLine(line, source) {
				entry := LogEntry{
					Source:    source.Name,
					Type:      source.Type,
					Container: container,
					Content:   line,
					Timestamp: time.Now(),
					Level:     lm.detectLogLevel(line),
				}
				entries = append(entries, entry)
			}
		}
	}

	return entries
}

// runDeployLogCollection handles deploy log collection
func (lm *LogManager) runDeployLogCollection(collector *LogCollector) {
	source := collector.source
	log.Printf("[Beacon] Starting deploy log collection: %s", source.Name)

	// Deploy logs are captured during deployment
	// This function monitors the deploy log file if it exists
	if source.DeployLogFile == "" {
		return
	}

	ticker := time.NewTicker(source.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-collector.ctx.Done():
			return
		case <-ticker.C:
			entries := lm.collectDeployLog(source)
			if len(entries) > 0 {
				lm.addLogEntries(entries)
			}
		}
	}
}

// collectDeployLog reads log entries from deploy log file
func (lm *LogManager) collectDeployLog(source LogSource) []LogEntry {
	if _, err := os.Stat(source.DeployLogFile); os.IsNotExist(err) {
		return nil
	}

	maxLines := source.MaxLines
	if maxLines == 0 {
		maxLines = 100
	}

	cmd := exec.Command("tail", "-n", fmt.Sprintf("%d", maxLines), source.DeployLogFile)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[Beacon] Error reading deploy log %s: %v", source.DeployLogFile, err)
		return nil
	}

	lines := strings.Split(string(output), "\n")
	var entries []LogEntry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if lm.shouldIncludeLogLine(line, source) {
			entry := LogEntry{
				Source:    source.Name,
				Type:      source.Type,
				Content:   line,
				Timestamp: time.Now(),
				Level:     lm.detectLogLevel(line),
			}
			entries = append(entries, entry)
		}
	}

	return entries
}

// runCommandLogCollection handles command-based log collection
func (lm *LogManager) runCommandLogCollection(collector *LogCollector) {
	source := collector.source
	log.Printf("[Beacon] Starting command log collection: %s -> %s", source.Name, source.Command)

	ticker := time.NewTicker(source.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-collector.ctx.Done():
			return
		case <-ticker.C:
			entries := lm.collectCommandLog(source)
			if len(entries) > 0 {
				lm.addLogEntries(entries)
			}
		}
	}
}

// collectCommandLog executes a command and collects its output as log entries
func (lm *LogManager) collectCommandLog(source LogSource) []LogEntry {
	cmd := exec.Command("sh", "-c", source.Command)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[Beacon] Error executing command log %s: %v", source.Command, err)
		return nil
	}

	lines := strings.Split(string(output), "\n")
	var entries []LogEntry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if lm.shouldIncludeLogLine(line, source) {
			entry := LogEntry{
				Source:    source.Name,
				Type:      source.Type,
				Content:   line,
				Timestamp: time.Now(),
				Level:     lm.detectLogLevel(line),
			}
			entries = append(entries, entry)
		}
	}

	return entries
}

// shouldIncludeLogLine determines if a log line should be included based on patterns
func (lm *LogManager) shouldIncludeLogLine(line string, source LogSource) bool {
	// Check exclude patterns first
	for _, pattern := range source.ExcludePatterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return false
		}
	}

	// If include patterns are specified, line must match at least one
	if len(source.IncludePatterns) > 0 {
		for _, pattern := range source.IncludePatterns {
			if matched, _ := regexp.MatchString(pattern, line); matched {
				return true
			}
		}
		return false
	}

	return true
}

// detectLogLevel attempts to detect the log level from a log line
func (lm *LogManager) detectLogLevel(line string) string {
	line = strings.ToLower(line)

	if strings.Contains(line, "error") || strings.Contains(line, "err") {
		return "error"
	}
	if strings.Contains(line, "warn") {
		return "warning"
	}
	if strings.Contains(line, "info") {
		return "info"
	}
	if strings.Contains(line, "debug") {
		return "debug"
	}

	return ""
}

// addLogEntries adds new log entries to the collection and reports them
func (lm *LogManager) addLogEntries(entries []LogEntry) {
	if len(entries) == 0 {
		return
	}

	lm.logsMux.Lock()
	lm.logs = append(lm.logs, entries...)

	// Keep only last 1000 log entries to prevent memory issues
	if len(lm.logs) > 1000 {
		lm.logs = lm.logs[len(lm.logs)-1000:]
	}
	lm.logsMux.Unlock()

	// Report logs to external API
	if lm.config.Report.SendTo != "" && lm.config.Report.Token != "" {
		go lm.reportLogs(entries)
	}

	log.Printf("[Beacon] Collected %d log entries", len(entries))
}

// reportLogs sends log entries to the external API
func (lm *LogManager) reportLogs(logs []LogEntry) {
	if len(logs) == 0 {
		return
	}

	// Convert to the format expected by Beaconinfra
	beaconinfraLogs := make([]map[string]interface{}, 0, len(logs))
	for _, entry := range logs {
		beaconinfraLogs = append(beaconinfraLogs, map[string]interface{}{
			"source":    entry.Source,
			"type":      entry.Type,
			"container": entry.Container,
			"content":   entry.Content,
			"timestamp": entry.Timestamp,
			"level":     entry.Level,
		})
	}

	payload := map[string]interface{}{
		"logs":  beaconinfraLogs,
		"token": lm.config.Report.Token,
		"type":  "logs",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Beacon] Failed to marshal logs: %v", err)
		return
	}

	req, err := http.NewRequest("POST", lm.config.Report.SendTo+"/agent/logs", strings.NewReader(string(jsonData)))
	if err != nil {
		log.Printf("[Beacon] Failed to create logs report request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+lm.config.Report.Token)

	resp, err := lm.httpClient.Do(req)
	if err != nil {
		log.Printf("[Beacon] Failed to send logs report: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Beacon] Successfully reported %d log entries", len(logs))
	} else {
		log.Printf("[Beacon] Failed to report logs: HTTP %d", resp.StatusCode)
	}
}
