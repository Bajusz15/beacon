package monitor

import (
	"beacon/internal/util"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultLogBatchSize     = 50
	defaultLogFlushInterval = 15 * time.Second
	logPositionsFilename    = "log_positions.json"
)

// logCursor is persisted per source+identifier (container or file path)
type logCursor struct {
	LastTimestamp string `json:"last_timestamp,omitempty"` // RFC3339
	LastOffset    int64  `json:"last_offset,omitempty"`
}

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
	UseTail    bool   `yaml:"use_tail,omitempty"`    // force tail command instead of direct file access

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

	// Deduplication
	Deduplicate bool `yaml:"deduplicate,omitempty"` // enable log deduplication for this source
}

// LogEntry represents a single log entry
type LogEntry struct {
	Source    string    `json:"source"`
	Type      string    `json:"type"`
	Container string    `json:"container,omitempty"` // for docker logs
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level,omitempty"` // parsed log level if detected
	Hash      string    `json:"hash,omitempty"`  // hash for deduplication
}

// LogCollector manages log collection for a specific source
type LogCollector struct {
	source        LogSource
	lastPosition  int64 // for file following
	lastTimestamp time.Time
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
	fileHandle    *os.File // for file-based collection
}

// LogManager handles all log collection and forwarding
type LogManager struct {
	config          *Config
	logs            []LogEntry
	logsMux         sync.RWMutex
	logCollectors   map[string]*LogCollector
	httpClient      *http.Client
	getAuthToken    func() string
	seenLogs        map[string]time.Time // hash -> last seen timestamp for deduplication
	seenLogsMux     sync.RWMutex
	stateDir        string
	logPositions    map[string]logCursor
	logPositionsMux sync.RWMutex
	pendingEntries  []LogEntry
	lastFlush       time.Time
	pendingMux      sync.Mutex
	flushTicker     *time.Ticker
	flushStop       chan struct{}
}

// getStateDir returns ~/.beacon/state for cursor persistence
func getStateDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".beacon/state"
	}
	return filepath.Join(home, ".beacon", "state")
}

// NewLogManager creates a new log manager. getAuthToken returns the bearer/API secret (e.g. dtk_ or bci_live_);
// if nil, report.token from config is used (tests and legacy).
func NewLogManager(config *Config, httpClient *http.Client, getAuthToken func() string) *LogManager {
	stateDir := getStateDir()
	_ = os.MkdirAll(stateDir, 0755)
	lm := &LogManager{
		config:         config,
		logs:           make([]LogEntry, 0),
		logCollectors:  make(map[string]*LogCollector),
		httpClient:     httpClient,
		getAuthToken:   getAuthToken,
		seenLogs:       make(map[string]time.Time),
		stateDir:       stateDir,
		logPositions:   make(map[string]logCursor),
		pendingEntries: make([]LogEntry, 0),
		lastFlush:      time.Now(),
	}
	return lm
}

func (lm *LogManager) authSecret() string {
	if lm.getAuthToken != nil {
		return lm.getAuthToken()
	}
	return lm.config.Report.Token
}

func (lm *LogManager) canForwardLogs() bool {
	if lm.config.Report.SendTo == "" {
		return false
	}
	if lm.getAuthToken != nil {
		return lm.getAuthToken() != ""
	}
	return lm.config.Report.Token != "" || lm.config.Report.TokenName != ""
}

func (lm *LogManager) loadLogPositions() {
	path := filepath.Join(lm.stateDir, logPositionsFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[Beacon] Failed to load log positions: %v", err)
		}
		return
	}
	var positions map[string]logCursor
	if err := json.Unmarshal(data, &positions); err != nil {
		log.Printf("[Beacon] Failed to parse log positions: %v", err)
		return
	}
	lm.logPositionsMux.Lock()
	lm.logPositions = positions
	lm.logPositionsMux.Unlock()
}

func (lm *LogManager) saveLogPositions() {
	lm.logPositionsMux.RLock()
	positions := make(map[string]logCursor, len(lm.logPositions))
	for k, v := range lm.logPositions {
		positions[k] = v
	}
	lm.logPositionsMux.RUnlock()

	path := filepath.Join(lm.stateDir, logPositionsFilename)
	data, err := json.MarshalIndent(positions, "", "  ")
	if err != nil {
		log.Printf("[Beacon] Failed to marshal log positions: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[Beacon] Failed to write log positions: %v", err)
	}
}

func (lm *LogManager) getCursor(key string) (since time.Time, offset int64) {
	lm.logPositionsMux.RLock()
	c, ok := lm.logPositions[key]
	lm.logPositionsMux.RUnlock()
	if !ok || c.LastTimestamp == "" {
		return time.Time{}, 0
	}
	t, _ := time.Parse(time.RFC3339, c.LastTimestamp)
	return t, c.LastOffset
}

func (lm *LogManager) saveCursor(key string, lastTimestamp time.Time, lastOffset int64) {
	lm.logPositionsMux.Lock()
	lm.logPositions[key] = logCursor{
		LastTimestamp: lastTimestamp.UTC().Format(time.RFC3339),
		LastOffset:    lastOffset,
	}
	lm.logPositionsMux.Unlock()
}

// parseLogTimestamp attempts to parse timestamps from various log formats
func (lm *LogManager) parseLogTimestamp(line string) (time.Time, string) {
	// Common timestamp patterns
	patterns := []struct {
		regex   *regexp.Regexp
		layout  string
		example string
	}{
		// RFC3339: 2024-01-15T10:30:00.123456789Z
		{regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?)\s+(.*)`), time.RFC3339Nano, "2024-01-15T10:30:00.123456789Z"},
		// RFC3339 without nanoseconds: 2024-01-15T10:30:00Z
		{regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)\s+(.*)`), time.RFC3339, "2024-01-15T10:30:00Z"},
		// Syslog: Jan 15 10:30:00
		{regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+(.*)`), "Jan 2 15:04:05", "Jan 15 10:30:00"},
		// ISO 8601: 2024-01-15 10:30:00
		{regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+(.*)`), "2006-01-02 15:04:05", "2024-01-15 10:30:00"},
		// ISO 8601 with milliseconds: 2024-01-15 10:30:00.123
		{regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3})\s+(.*)`), "2006-01-02 15:04:05.000", "2024-01-15 10:30:00.123"},
		// Unix timestamp: 1705312200
		{regexp.MustCompile(`^(\d{10})\s+(.*)`), "unix", "1705312200"},
		// Unix timestamp with milliseconds: 1705312200123
		{regexp.MustCompile(`^(\d{13})\s+(.*)`), "unix_ms", "1705312200123"},
	}

	for _, pattern := range patterns {
		matches := pattern.regex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			timestampStr := matches[1]
			content := matches[2]

			var timestamp time.Time
			var err error

			switch pattern.layout {
			case "unix":
				if unix, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
					timestamp = time.Unix(unix, 0)
				}
			case "unix_ms":
				if unix, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
					timestamp = time.Unix(unix/1000, (unix%1000)*1000000)
				}
			default:
				timestamp, err = time.Parse(pattern.layout, timestampStr)
			}

			if err == nil {
				return timestamp, content
			}
		}
	}

	// No timestamp found, return current time
	return time.Now(), line
}

// StartLogCollection starts all configured log sources
func (lm *LogManager) StartLogCollection(ctx context.Context) {
	log.Printf("[Beacon] Starting log collection for %d sources", len(lm.config.LogSources))

	lm.loadLogPositions()

	// Start periodic cleanup of old hashes
	go lm.startHashCleanup(ctx)

	flushInterval := lm.config.Report.LogFlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultLogFlushInterval
	}
	lm.flushStop = make(chan struct{})
	lm.flushTicker = time.NewTicker(flushInterval)
	go lm.runFlushTicker()

	for _, source := range lm.config.LogSources {
		if !source.Enabled {
			continue
		}

		logCollector := &LogCollector{
			source:  source,
			running: true,
		}
		logCollector.ctx, logCollector.cancel = context.WithCancel(ctx)

		// Apply persisted cursor for this source
		switch source.Type {
		case "file":
			if source.FilePath != "" {
				key := "file:" + source.Name + ":" + source.FilePath
				since, offset := lm.getCursor(key)
				if !since.IsZero() {
					logCollector.lastTimestamp = since
				}
				if offset > 0 {
					logCollector.lastPosition = offset
				}
			}
		case "docker":
			key := "docker:" + source.Name
			since, _ := lm.getCursor(key)
			if !since.IsZero() {
				logCollector.lastTimestamp = since
			} else {
				logCollector.lastTimestamp = time.Now().Add(-source.Interval)
			}
		}

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

func (lm *LogManager) runFlushTicker() {
	for {
		select {
		case <-lm.flushStop:
			return
		case <-lm.flushTicker.C:
			lm.flushPending(false)
		}
	}
}

// startHashCleanup runs periodic cleanup of old hash entries
func (lm *LogManager) startHashCleanup(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour) // Cleanup every 6 hours
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lm.cleanupOldHashes()
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

// FlushAndStop cancels collectors, performs a final collect per source, flushes pending logs synchronously, then saves positions.
func (lm *LogManager) FlushAndStop(ctx context.Context) {
	// Stop flush ticker
	if lm.flushTicker != nil {
		lm.flushTicker.Stop()
		lm.flushTicker = nil
	}
	if lm.flushStop != nil {
		close(lm.flushStop)
		lm.flushStop = nil
	}

	// Cancel collectors so they exit
	lm.StopLogCollection()

	// Brief wait for collector goroutines to exit
	select {
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
	}

	// Final collect per source (one-shot)
	for _, source := range lm.config.LogSources {
		if !source.Enabled {
			continue
		}
		var entries []LogEntry
		switch source.Type {
		case "file":
			// Temporary collector to get last N lines
			tempCollector := &LogCollector{source: source}
			entries = lm.collectFileLogWithTail(source, tempCollector)
		case "docker":
			since, _ := lm.getCursor("docker:" + source.Name)
			if since.IsZero() {
				since = time.Now().Add(-source.Interval)
			}
			entries = lm.collectDockerLogSince(source, since)
		case "deploy":
			entries = lm.collectDeployLog(source)
		case "command":
			entries = lm.collectCommandLog(source)
		default:
			continue
		}
		if len(entries) > 0 {
			lm.addLogEntries(entries)
		}
	}

	// Flush any pending entries synchronously
	lm.flushPending(true)
	lm.saveLogPositions()
}

// runFileLogCollection handles file-based log collection
func (lm *LogManager) runFileLogCollection(collector *LogCollector) {
	source := collector.source
	log.Printf("[Beacon] Starting file log collection: %s -> %s", source.Name, source.FilePath)

	// If user explicitly wants tail or we can't open the file directly, use tail
	if source.UseTail || !lm.canAccessFileDirectly(source.FilePath) {
		log.Printf("[Beacon] Using tail command for %s", source.FilePath)
		lm.runFileLogCollectionWithTail(collector)
		return
	}

	// Try direct file access
	file, err := os.Open(source.FilePath)
	if err != nil {
		log.Printf("[Beacon] Cannot open file %s directly (%v), falling back to tail", source.FilePath, err)
		lm.runFileLogCollectionWithTail(collector)
		return
	}
	defer util.DeferClose(file, "log file")()

	collector.fileHandle = file

	// Get initial file size and position
	stat, err := file.Stat()
	if err != nil {
		log.Printf("[Beacon] Error getting file stats for %s: %v", source.FilePath, err)
		lm.runFileLogCollectionWithTail(collector)
		return
	}

	// If we didn't load a cursor, set initial position from file
	if collector.lastPosition == 0 {
		if source.FollowFile {
			collector.lastPosition = stat.Size()
		} else {
			collector.lastPosition = 0 // Start from beginning to read existing content
		}
	}

	log.Printf("[Beacon] Using direct file access for %s", source.FilePath)
	ticker := time.NewTicker(source.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-collector.ctx.Done():
			return
		case <-ticker.C:
			entries := lm.collectFileLogFromPosition(collector)
			if len(entries) > 0 {
				lm.addLogEntries(entries)
			}
			key := "file:" + source.Name + ":" + source.FilePath
			ts := collector.lastTimestamp
			if ts.IsZero() {
				ts = time.Now()
			}
			lm.saveCursor(key, ts, collector.lastPosition)
			lm.saveLogPositions()
		}
	}
}

// collectFileLogFromPosition reads new log entries from file since last position
func (lm *LogManager) collectFileLogFromPosition(collector *LogCollector) []LogEntry {
	source := collector.source
	file := collector.fileHandle

	// Get current file size
	stat, err := file.Stat()
	if err != nil {
		log.Printf("[Beacon] Error getting file stats for %s: %v", source.FilePath, err)
		return nil
	}

	currentSize := stat.Size()

	// If file was truncated (log rotation), reset position
	if currentSize < collector.lastPosition {
		log.Printf("[Beacon] File %s was truncated, resetting position", source.FilePath)
		collector.lastPosition = 0
	}

	// If no new content, return empty
	if currentSize <= collector.lastPosition {
		return nil
	}

	// Seek to last position
	_, err = file.Seek(collector.lastPosition, 0)
	if err != nil {
		log.Printf("[Beacon] Error seeking in file %s: %v", source.FilePath, err)
		return nil
	}

	// Read new content
	reader := bufio.NewReader(file)
	var entries []LogEntry
	var line string

	for {
		line, err = reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[Beacon] Error reading file %s: %v", source.FilePath, err)
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if lm.shouldIncludeLogLine(line, source) {
			timestamp, content := lm.parseLogTimestamp(line)

			entry := LogEntry{
				Source:    source.Name,
				Type:      source.Type,
				Content:   content,
				Timestamp: timestamp,
				Level:     lm.detectLogLevel(content),
			}
			entries = append(entries, entry)
		}
	}

	// Update position
	collector.lastPosition = currentSize

	return entries
}

// canAccessFileDirectly checks if we can read the file without permission issues
func (lm *LogManager) canAccessFileDirectly(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	util.LogError(file.Close(), "close log file")
	return true
}

// runFileLogCollectionWithTail handles file-based log collection using tail command
func (lm *LogManager) runFileLogCollectionWithTail(collector *LogCollector) {
	source := collector.source

	ticker := time.NewTicker(source.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-collector.ctx.Done():
			return
		case <-ticker.C:
			entries := lm.collectFileLogWithTail(source, collector)
			if len(entries) > 0 {
				lm.addLogEntries(entries)
			}
			key := "file:" + source.Name + ":" + source.FilePath
			ts := collector.lastTimestamp
			if ts.IsZero() {
				ts = time.Now()
			}
			lm.saveCursor(key, ts, 0)
			lm.saveLogPositions()
		}
	}
}

// collectFileLogWithTail reads log entries using tail command with position tracking
func (lm *LogManager) collectFileLogWithTail(source LogSource, collector *LogCollector) []LogEntry {
	maxLines := source.MaxLines
	if maxLines == 0 {
		maxLines = 100
	}

	var cmd *exec.Cmd
	if source.FollowFile && collector.lastTimestamp.IsZero() {
		// First run with follow - get recent lines
		cmd = exec.Command("tail", "-n", fmt.Sprintf("%d", maxLines), source.FilePath)
		collector.lastTimestamp = time.Now()
	} else if source.FollowFile {
		// Subsequent runs - try to get only new lines since last check
		// Use tail with grep to filter by timestamp (if possible)
		cmd = exec.Command("tail", "-n", fmt.Sprintf("%d", maxLines*2), source.FilePath)
	} else {
		// Not following - get end of file
		cmd = exec.Command("tail", "-n", fmt.Sprintf("%d", maxLines), source.FilePath)
	}

	output, err := cmd.Output()
	if err != nil {
		log.Printf("[Beacon] Error reading file log %s with tail: %v", source.FilePath, err)
		return nil
	}

	lines := strings.Split(string(output), "\n")
	var entries []LogEntry
	var newestTimestamp time.Time

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if lm.shouldIncludeLogLine(line, source) {
			timestamp, content := lm.parseLogTimestamp(line)

			// Skip logs older than our last timestamp when following
			if source.FollowFile && !collector.lastTimestamp.IsZero() && timestamp.Before(collector.lastTimestamp) {
				continue
			}

			entry := LogEntry{
				Source:    source.Name,
				Type:      source.Type,
				Content:   content,
				Timestamp: timestamp,
				Level:     lm.detectLogLevel(content),
			}
			entries = append(entries, entry)

			// Track newest timestamp
			if timestamp.After(newestTimestamp) {
				newestTimestamp = timestamp
			}
		}
	}

	// Update last timestamp for next run
	if source.FollowFile && !newestTimestamp.IsZero() {
		collector.lastTimestamp = newestTimestamp
	}

	return entries
}

// runDockerLogCollection handles Docker container log collection
func (lm *LogManager) runDockerLogCollection(collector *LogCollector) {
	source := collector.source
	log.Printf("[Beacon] Starting docker log collection: %s", source.Name)

	// lastTimestamp set from persisted cursor in StartLogCollection, or from initial interval

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
			key := "docker:" + source.Name
			lm.saveCursor(key, collector.lastTimestamp, 0)
			lm.saveLogPositions()
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
			for _, opt := range optArgs {
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

			logTimestamp, logContent := lm.parseLogTimestamp(line)

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
			timestamp, content := lm.parseLogTimestamp(line)

			entry := LogEntry{
				Source:    source.Name,
				Type:      source.Type,
				Content:   content,
				Timestamp: timestamp,
				Level:     lm.detectLogLevel(content),
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
			timestamp, content := lm.parseLogTimestamp(line)

			entry := LogEntry{
				Source:    source.Name,
				Type:      source.Type,
				Content:   content,
				Timestamp: timestamp,
				Level:     lm.detectLogLevel(content),
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

// generateLogHash creates a hash for log deduplication
func (lm *LogManager) generateLogHash(entry LogEntry) string {
	// Create a hash based on source, type, container, and content
	// Timestamp is excluded to allow same log content at different times
	hashInput := fmt.Sprintf("%s|%s|%s|%s", entry.Source, entry.Type, entry.Container, entry.Content)
	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])
}

// isDuplicateLog checks if a log entry is a duplicate
func (lm *LogManager) isDuplicateLog(entry LogEntry, source LogSource) bool {
	if !source.Deduplicate {
		return false
	}

	hash := lm.generateLogHash(entry)

	lm.seenLogsMux.RLock()
	lastSeen, exists := lm.seenLogs[hash]
	lm.seenLogsMux.RUnlock()

	if !exists {
		// New log entry
		lm.seenLogsMux.Lock()
		lm.seenLogs[hash] = entry.Timestamp
		lm.seenLogsMux.Unlock()
		return false
	}

	// Check if this is the same log within a reasonable time window (e.g., 1 hour)
	// This prevents the same log from being sent multiple times
	timeWindow := time.Hour
	if entry.Timestamp.Sub(lastSeen) < timeWindow {
		return true
	}

	// Update the timestamp for this hash
	lm.seenLogsMux.Lock()
	lm.seenLogs[hash] = entry.Timestamp
	lm.seenLogsMux.Unlock()
	return false
}

// cleanupOldHashes removes old hash entries to prevent memory leaks
func (lm *LogManager) cleanupOldHashes() {
	cutoff := time.Now().Add(-24 * time.Hour) // Keep hashes for 24 hours

	lm.seenLogsMux.Lock()
	defer lm.seenLogsMux.Unlock()

	for hash, lastSeen := range lm.seenLogs {
		if lastSeen.Before(cutoff) {
			delete(lm.seenLogs, hash)
		}
	}
}

// flushPending sends pending entries to the API synchronously and clears the buffer
func (lm *LogManager) flushPending(_ bool) {
	lm.pendingMux.Lock()
	pending := lm.pendingEntries
	lm.pendingEntries = nil
	lm.lastFlush = time.Now()
	lm.pendingMux.Unlock()

	if len(pending) == 0 {
		return
	}
	if !lm.canForwardLogs() {
		return
	}
	lm.reportLogs(pending)
	log.Printf("[Beacon] Flushed %d log entries", len(pending))
}

// addLogEntries adds new log entries to the collection and reports them (batched)
func (lm *LogManager) addLogEntries(entries []LogEntry) {
	if len(entries) == 0 {
		return
	}

	// Filter out duplicates and add hashes
	var filteredEntries []LogEntry
	for _, entry := range entries {
		// Generate hash for the entry
		entry.Hash = lm.generateLogHash(entry)

		// Find the source configuration for this entry
		var source LogSource
		for _, s := range lm.config.LogSources {
			if s.Name == entry.Source {
				source = s
				break
			}
		}

		// Check if this is a duplicate
		if lm.isDuplicateLog(entry, source) {
			continue
		}

		filteredEntries = append(filteredEntries, entry)
	}

	if len(filteredEntries) == 0 {
		return
	}

	lm.logsMux.Lock()
	lm.logs = append(lm.logs, filteredEntries...)

	// Keep only last 1000 log entries to prevent memory issues
	if len(lm.logs) > 1000 {
		lm.logs = lm.logs[len(lm.logs)-1000:]
	}
	lm.logsMux.Unlock()

	batchSize := lm.config.Report.LogBatchSize
	if batchSize <= 0 {
		batchSize = defaultLogBatchSize
	}

	if lm.canForwardLogs() {
		lm.pendingMux.Lock()
		lm.pendingEntries = append(lm.pendingEntries, filteredEntries...)
		if len(lm.pendingEntries) >= batchSize {
			toSend := lm.pendingEntries
			lm.pendingEntries = nil
			lm.lastFlush = time.Now()
			lm.pendingMux.Unlock()
			lm.reportLogs(toSend)
			log.Printf("[Beacon] Flushed %d log entries (batch full)", len(toSend))
		} else {
			lm.pendingMux.Unlock()
		}
	}

	log.Printf("[Beacon] Collected %d log entries (filtered from %d)", len(filteredEntries), len(entries))
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

	secret := lm.authSecret()
	if secret == "" {
		return
	}

	payload := map[string]interface{}{
		"logs": beaconinfraLogs,
		"type": "logs",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Beacon] Failed to marshal logs: %v", err)
		return
	}

	baseURL := strings.TrimSuffix(lm.config.Report.SendTo, "/")
	req, err := http.NewRequest("POST", baseURL+"/agent/logs", strings.NewReader(string(jsonData)))
	if err != nil {
		log.Printf("[Beacon] Failed to create logs report request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	setAgentAuthHeaders(req, secret)

	resp, err := lm.httpClient.Do(req)
	if err != nil {
		log.Printf("[Beacon] Failed to send logs report: %v", err)
		return
	}
	defer util.DeferClose(resp.Body, "HTTP response body")()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Beacon] Successfully reported %d log entries", len(logs))
	} else {
		log.Printf("[Beacon] Failed to report logs: HTTP %d", resp.StatusCode)
	}
}
