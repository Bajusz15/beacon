package master

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"beacon/internal/identity"
	"beacon/internal/ipc"
	"beacon/internal/tunnel"
)

const actionTunnelConnect = "tunnel_connect"

// HeartbeatCommand represents a command received from the heartbeat response.
type HeartbeatCommand struct {
	ID            string         `json:"id"`
	Action        string         `json:"action"`
	TargetProject string         `json:"target_project"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// CommandResultReport represents a command result to include in the next heartbeat.
type CommandResultReport struct {
	CommandID string    `json:"command_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// CommandDispatcher handles dispatching commands to children and collecting results.
type CommandDispatcher struct {
	pm             *ProcessManager
	tm             *tunnel.TunnelManager
	pendingResults []CommandResultReport
	mu             sync.Mutex
}

// NewCommandDispatcher creates a new command dispatcher.
func NewCommandDispatcher(pm *ProcessManager, tm *tunnel.TunnelManager) *CommandDispatcher {
	return &CommandDispatcher{
		pm:             pm,
		tm:             tm,
		pendingResults: make([]CommandResultReport, 0),
	}
}

// DispatchCommands dispatches commands to the appropriate children via IPC.
func (d *CommandDispatcher) DispatchCommands(commands []HeartbeatCommand) {
	if len(commands) == 0 {
		return
	}

	var readers map[string]*ipc.Reader
	if d.pm != nil {
		readers = d.pm.GetIPCReaders()
	}

	for _, cmd := range commands {
		if cmd.Action == actionTunnelConnect {
			d.dispatchTunnelConnect(cmd)
			continue
		}
		if d.pm == nil {
			d.recordResult(cmd.ID, ipc.ResultFailed, "Process manager not available")
			continue
		}
		if cmd.TargetProject == "" {
			d.recordResult(cmd.ID, ipc.ResultFailed, "Device-level commands not supported")
			continue
		}

		reader, exists := readers[cmd.TargetProject]
		if !exists {
			log.Printf("[Beacon master] Command %s: project %s not found", cmd.ID, cmd.TargetProject)
			d.recordResult(cmd.ID, ipc.ResultFailed, "Project not found: "+cmd.TargetProject)
			continue
		}

		// Write command to child's IPC directory
		ipcCmd := &ipc.Command{
			ID:        cmd.ID,
			Action:    cmd.Action,
			Payload:   cmd.Payload,
			Timestamp: time.Now(),
		}

		if err := reader.WriteCommand(ipcCmd); err != nil {
			log.Printf("[Beacon master] Failed to dispatch command %s to %s: %v", cmd.ID, cmd.TargetProject, err)
			d.recordResult(cmd.ID, ipc.ResultFailed, "Failed to dispatch: "+err.Error())
			continue
		}

		log.Printf("[Beacon master] Dispatched command %s (%s) to %s", cmd.ID, cmd.Action, cmd.TargetProject)
	}
}

func (d *CommandDispatcher) dispatchTunnelConnect(cmd HeartbeatCommand) {
	if d.tm == nil {
		d.recordResult(cmd.ID, ipc.ResultFailed, "Tunnel manager not available")
		return
	}
	tid, _ := cmd.Payload["tunnel_id"].(string)
	tid = strings.TrimSpace(tid)
	if tid == "" {
		d.recordResult(cmd.ID, ipc.ResultFailed, "tunnel_id missing in payload")
		return
	}
	uc, err := identity.LoadUserConfig()
	if err != nil || uc == nil {
		d.recordResult(cmd.ID, ipc.ResultFailed, "config unavailable")
		return
	}
	var found *identity.TunnelConfig
	for i := range uc.Tunnels {
		if uc.Tunnels[i].ID == tid {
			found = &uc.Tunnels[i]
			break
		}
	}
	if found == nil {
		d.recordResult(cmd.ID, ipc.ResultFailed, "tunnel not found in config: "+tid)
		return
	}
	name := strings.TrimSpace(uc.DeviceName)
	if name == "" {
		h, err := os.Hostname()
		if err != nil || strings.TrimSpace(h) == "" {
			name = "unknown"
		} else {
			name = strings.TrimSpace(h)
		}
	}
	apiKey := strings.TrimSpace(uc.APIKey)
	if apiKey == "" {
		d.recordResult(cmd.ID, ipc.ResultFailed, "API key not configured")
		return
	}
	err = d.tm.EnsureStarted(*found, uc.EffectiveCloudAPIBase(), apiKey, name)
	if err != nil {
		d.recordResult(cmd.ID, ipc.ResultFailed, err.Error())
		return
	}
	d.recordResult(cmd.ID, ipc.ResultSuccess, fmt.Sprintf("tunnel %q started", tid))
}

// CollectResults collects command results from all children.
func (d *CommandDispatcher) CollectResults() {
	if d.pm == nil {
		return
	}

	readers := d.pm.GetIPCReaders()

	for projectID, reader := range readers {
		result, err := reader.ReadCommandResult()
		if err != nil {
			log.Printf("[Beacon master] Error reading command result from %s: %v", projectID, err)
			continue
		}
		if result == nil {
			continue // No result
		}

		log.Printf("[Beacon master] Collected result for command %s from %s: %s", result.CommandID, projectID, result.Status)
		d.recordResult(result.CommandID, result.Status, result.Message)
	}
}

// GetPendingResults returns and clears the pending command results for the next heartbeat.
func (d *CommandDispatcher) GetPendingResults() []CommandResultReport {
	d.mu.Lock()
	defer d.mu.Unlock()

	results := d.pendingResults
	d.pendingResults = make([]CommandResultReport, 0)
	return results
}

// recordResult adds a command result to the pending results list.
func (d *CommandDispatcher) recordResult(commandID, status, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pendingResults = append(d.pendingResults, CommandResultReport{
		CommandID: commandID,
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
	})
}
