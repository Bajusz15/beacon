package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"beacon/internal/config"
)

const (
	healthFile        = "health.json"
	commandFile       = "command.json"
	commandResultFile = "command_result.json"
)

// Writer handles writing IPC files for child agents.
type Writer struct {
	dir string
}

// NewWriter creates a new IPC writer for the given directory.
// Creates the directory if it doesn't exist.
func NewWriter(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create IPC directory: %w", err)
	}
	return &Writer{dir: dir}, nil
}

// WriteHealth writes the health report to health.json atomically.
func (w *Writer) WriteHealth(report *HealthReport) error {
	return atomicWriteJSON(filepath.Join(w.dir, healthFile), report)
}

// WriteCommandResult writes the command result to command_result.json atomically.
func (w *Writer) WriteCommandResult(result *CommandResult) error {
	return atomicWriteJSON(filepath.Join(w.dir, commandResultFile), result)
}

// ReadCommand reads and deletes the command.json file if it exists.
// Returns nil, nil if no command file exists.
func (w *Writer) ReadCommand() (*Command, error) {
	path := filepath.Join(w.dir, commandFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read command file: %w", err)
	}

	var cmd Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		// Delete malformed command file
		_ = os.Remove(path)
		return nil, fmt.Errorf("parse command: %w", err)
	}

	// Delete command file so it's not processed again
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("delete command file: %w", err)
	}

	return &cmd, nil
}

// Reader handles reading IPC files for the master agent.
type Reader struct {
	dir string
}

// NewReader creates a new IPC reader for the given directory.
func NewReader(dir string) *Reader {
	return &Reader{dir: dir}
}

// ReadHealth reads the health.json file.
// Returns nil, nil if the file doesn't exist.
func (r *Reader) ReadHealth() (*HealthReport, error) {
	path := filepath.Join(r.dir, healthFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read health file: %w", err)
	}

	var report HealthReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse health report: %w", err)
	}
	return &report, nil
}

// ReadHealthIfFresh reads the health.json file only if it was modified within maxAge.
// Returns nil, nil if the file doesn't exist or is stale.
func (r *Reader) ReadHealthIfFresh(maxAge time.Duration) (*HealthReport, error) {
	path := filepath.Join(r.dir, healthFile)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat health file: %w", err)
	}

	if time.Since(info.ModTime()) > maxAge {
		return nil, nil // stale
	}

	return r.ReadHealth()
}

// WriteCommand writes a command to command.json for the child to process.
func (r *Reader) WriteCommand(cmd *Command) error {
	return atomicWriteJSON(filepath.Join(r.dir, commandFile), cmd)
}

// ReadCommandResult reads and deletes the command_result.json file.
// Returns nil, nil if no result file exists.
func (r *Reader) ReadCommandResult() (*CommandResult, error) {
	path := filepath.Join(r.dir, commandResultFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read command result: %w", err)
	}

	var result CommandResult
	if err := json.Unmarshal(data, &result); err != nil {
		// Delete malformed result file
		_ = os.Remove(path)
		return nil, fmt.Errorf("parse command result: %w", err)
	}

	// Delete result file after reading
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("delete command result file: %w", err)
	}

	return &result, nil
}

// atomicWriteJSON writes data to a file atomically (write to temp, then rename).
func atomicWriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// IPCDir returns the base IPC directory for a user.
func IPCDir() (string, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ipc"), nil
}

// ProjectIPCDir returns the IPC directory for a specific project.
func ProjectIPCDir(projectID string) (string, error) {
	base, err := IPCDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, projectID), nil
}
