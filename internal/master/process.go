package master

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"beacon/internal/identity"
	"beacon/internal/ipc"
)

const (
	maxRestarts    = 5
	maxBackoff     = 60 * time.Second
	shutdownWait   = 10 * time.Second
)

// ChildProcess represents a spawned child agent process.
type ChildProcess struct {
	ProjectID  string
	ConfigPath string
	IPCDir     string
	Cmd        *exec.Cmd
	StartedAt  time.Time
	Restarts   int
	Failed     bool // Set to true if max restarts exceeded
}

// ProcessManager manages child agent processes.
type ProcessManager struct {
	mu       sync.RWMutex
	children map[string]*ChildProcess
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	ipcBase  string
}

// NewProcessManager creates a new process manager.
func NewProcessManager(ctx context.Context) (*ProcessManager, error) {
	ipcBase, err := ipc.IPCDir()
	if err != nil {
		return nil, fmt.Errorf("get IPC dir: %w", err)
	}

	childCtx, cancel := context.WithCancel(ctx)

	return &ProcessManager{
		children: make(map[string]*ChildProcess),
		ctx:      childCtx,
		cancel:   cancel,
		ipcBase:  ipcBase,
	}, nil
}

// SpawnAll spawns child agents for all enabled projects.
func (pm *ProcessManager) SpawnAll(projects []identity.ProjectConfig) {
	for _, project := range projects {
		if !pm.isProjectEnabled(project) {
			log.Printf("[Beacon master] Project %s is disabled, skipping", project.ID)
			continue
		}

		if err := pm.Spawn(project); err != nil {
			log.Printf("[Beacon master] Failed to spawn child for %s: %v", project.ID, err)
		}
	}
}

// isProjectEnabled returns true if the project should be spawned.
// Projects are enabled by default if Enabled is not explicitly set.
func (pm *ProcessManager) isProjectEnabled(project identity.ProjectConfig) bool {
	// If Enabled is explicitly set to false, skip
	// Note: Go's zero value for bool is false, so we need a way to distinguish
	// "not set" from "explicitly false". For now, we treat it as: if config_path is set, it's enabled
	// unless the ID is empty.
	if project.ID == "" || project.ConfigPath == "" {
		return false
	}
	return true // In YAML, if enabled is omitted we assume true
}

// Spawn starts a child agent for the given project.
func (pm *ProcessManager) Spawn(project identity.ProjectConfig) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already running
	if existing, ok := pm.children[project.ID]; ok && !existing.Failed {
		return fmt.Errorf("child for project %s already running", project.ID)
	}

	// Create IPC directory
	ipcDir := filepath.Join(pm.ipcBase, project.ID)
	if err := os.MkdirAll(ipcDir, 0755); err != nil {
		return fmt.Errorf("create IPC dir: %w", err)
	}

	child := &ChildProcess{
		ProjectID:  project.ID,
		ConfigPath: project.ConfigPath,
		IPCDir:     ipcDir,
	}

	if err := pm.spawnChild(child); err != nil {
		return err
	}

	pm.children[project.ID] = child

	// Start watcher goroutine
	pm.wg.Add(1)
	go pm.watchChild(child, project)

	log.Printf("[Beacon master] Spawned child for project %s (PID %d)", project.ID, child.Cmd.Process.Pid)
	return nil
}

// spawnChild starts the child process.
func (pm *ProcessManager) spawnChild(child *ChildProcess) error {
	// Get the path to the current executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	cmd := exec.CommandContext(pm.ctx, execPath, "agent",
		"--project-id", child.ProjectID,
		"--config", child.ConfigPath,
		"--ipc-dir", child.IPCDir,
	)

	// Inherit stdout/stderr for logging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start child: %w", err)
	}

	child.Cmd = cmd
	child.StartedAt = time.Now()

	return nil
}

// watchChild watches a child process and restarts it on crash.
func (pm *ProcessManager) watchChild(child *ChildProcess, project identity.ProjectConfig) {
	defer pm.wg.Done()

	for {
		// Wait for the process to exit
		err := child.Cmd.Wait()

		// Check if we're shutting down
		select {
		case <-pm.ctx.Done():
			log.Printf("[Beacon master] Child %s exited during shutdown", child.ProjectID)
			return
		default:
		}

		// Child crashed or exited unexpectedly
		exitCode := -1
		if child.Cmd.ProcessState != nil {
			exitCode = child.Cmd.ProcessState.ExitCode()
		}
		log.Printf("[Beacon master] Child %s exited (code=%d, err=%v)", child.ProjectID, exitCode, err)

		pm.mu.Lock()
		child.Restarts++

		if child.Restarts > maxRestarts {
			log.Printf("[Beacon master] Child %s exceeded max restarts (%d), giving up", child.ProjectID, maxRestarts)
			child.Failed = true
			pm.mu.Unlock()
			return
		}

		// Calculate backoff: 2^restarts seconds, capped at maxBackoff
		backoff := time.Duration(1<<uint(child.Restarts)) * time.Second
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		pm.mu.Unlock()

		log.Printf("[Beacon master] Restarting child %s in %v (attempt %d/%d)", child.ProjectID, backoff, child.Restarts, maxRestarts)

		// Wait for backoff
		select {
		case <-pm.ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Respawn
		pm.mu.Lock()
		if err := pm.spawnChild(child); err != nil {
			log.Printf("[Beacon master] Failed to respawn child %s: %v", child.ProjectID, err)
			child.Failed = true
			pm.mu.Unlock()
			return
		}
		log.Printf("[Beacon master] Respawned child %s (PID %d)", child.ProjectID, child.Cmd.Process.Pid)
		pm.mu.Unlock()
	}
}

// Shutdown gracefully stops all child processes.
func (pm *ProcessManager) Shutdown() {
	log.Printf("[Beacon master] Stopping all children...")

	// Cancel context to stop respawn attempts
	pm.cancel()

	pm.mu.RLock()
	children := make([]*ChildProcess, 0, len(pm.children))
	for _, child := range pm.children {
		if child.Cmd != nil && child.Cmd.Process != nil {
			children = append(children, child)
		}
	}
	pm.mu.RUnlock()

	// Send SIGTERM to all children
	for _, child := range children {
		log.Printf("[Beacon master] Sending SIGTERM to child %s (PID %d)", child.ProjectID, child.Cmd.Process.Pid)
		_ = child.Cmd.Process.Signal(os.Interrupt)
	}

	// Wait for children with timeout
	done := make(chan struct{})
	go func() {
		pm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[Beacon master] All children stopped gracefully")
	case <-time.After(shutdownWait):
		log.Printf("[Beacon master] Timeout waiting for children, sending SIGKILL")
		for _, child := range children {
			if child.Cmd.ProcessState == nil {
				_ = child.Cmd.Process.Kill()
			}
		}
	}

	// Cleanup IPC directories (optional - could keep for debugging)
	// for _, child := range children {
	// 	_ = os.RemoveAll(child.IPCDir)
	// }
}

// GetChildren returns a snapshot of all child processes.
func (pm *ProcessManager) GetChildren() map[string]*ChildProcess {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string]*ChildProcess, len(pm.children))
	for k, v := range pm.children {
		// Copy to avoid race conditions
		result[k] = &ChildProcess{
			ProjectID:  v.ProjectID,
			ConfigPath: v.ConfigPath,
			IPCDir:     v.IPCDir,
			StartedAt:  v.StartedAt,
			Restarts:   v.Restarts,
			Failed:     v.Failed,
		}
	}
	return result
}

// GetIPCReaders returns IPC readers for all children (for health aggregation).
func (pm *ProcessManager) GetIPCReaders() map[string]*ipc.Reader {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	readers := make(map[string]*ipc.Reader, len(pm.children))
	for projectID, child := range pm.children {
		readers[projectID] = ipc.NewReader(child.IPCDir)
	}
	return readers
}
