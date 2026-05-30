package master

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"beacon/internal/audit"
	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/identity"
	"beacon/internal/ipc"
	"beacon/internal/keys"
	"beacon/internal/remoteaccess"
	"beacon/internal/terminal"
	"beacon/internal/tunnel"
)

const (
	actionTunnelConnect    = "tunnel_connect"
	actionVPNEnable        = "vpn_enable"
	actionVPNUse           = "vpn_use"
	actionVPNDisable       = "vpn_disable"
	actionTerminalOpen     = "terminal_open"
	actionUpdateCredential = "update_credential"

	commandTTL = 1 * time.Hour
)

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
	grants         *remoteaccess.Grants
	pendingResults []CommandResultReport
	mu             sync.Mutex

	seenMu       sync.Mutex
	seenCommands map[string]time.Time

	allowedMu       sync.RWMutex
	allowedOverride map[string]bool // nil = use defaultAllowedActions

	// metaMu guards cmdMeta, which remembers the action/source of in-flight
	// commands so recordResult can enrich the audit trail with the outcome.
	metaMu  sync.Mutex
	cmdMeta map[string]cmdMeta
}

// cmdMeta carries enough context about a dispatched command to audit its
// outcome when the result comes back (possibly from another goroutine).
type cmdMeta struct {
	action string
	source string
	at     time.Time
}

// NewCommandDispatcher creates a new command dispatcher.
func NewCommandDispatcher(pm *ProcessManager, tm *tunnel.TunnelManager) *CommandDispatcher {
	return &CommandDispatcher{
		pm:             pm,
		tm:             tm,
		grants:         remoteaccess.NewGrants(),
		pendingResults: make([]CommandResultReport, 0),
		seenCommands:   make(map[string]time.Time),
		cmdMeta:        make(map[string]cmdMeta),
	}
}

// Grants exposes the remote-access grant manager so the control socket can
// issue challenges and verify unlocks against the same in-memory state the
// dispatcher gates on.
func (d *CommandDispatcher) Grants() *remoteaccess.Grants { return d.grants }

// SetAllowedActions updates the action allowlist from user config. Pass nil to
// revert to the default built-in list.
func (d *CommandDispatcher) SetAllowedActions(actions []string) {
	d.allowedMu.Lock()
	defer d.allowedMu.Unlock()
	if len(actions) == 0 {
		d.allowedOverride = nil
		return
	}
	m := make(map[string]bool, len(actions))
	for _, a := range actions {
		a = strings.TrimSpace(a)
		if a != "" {
			m[a] = true
		}
	}
	d.allowedOverride = m
}

func (d *CommandDispatcher) isAllowed(action string) bool {
	d.allowedMu.RLock()
	defer d.allowedMu.RUnlock()
	if d.allowedOverride != nil {
		return d.allowedOverride[action]
	}
	return true
}

func (d *CommandDispatcher) isDuplicate(id string) bool {
	if id == "" {
		return false
	}
	d.seenMu.Lock()
	defer d.seenMu.Unlock()
	if t, ok := d.seenCommands[id]; ok && time.Since(t) < commandTTL {
		return true
	}
	d.seenCommands[id] = time.Now()
	// Prune stale entries periodically to avoid unbounded growth.
	if len(d.seenCommands) > 500 {
		d.pruneSeenLocked()
	}
	return false
}

func (d *CommandDispatcher) pruneSeenLocked() {
	now := time.Now()
	for id, t := range d.seenCommands {
		if now.Sub(t) > commandTTL {
			delete(d.seenCommands, id)
		}
	}
}

// DispatchCommands dispatches commands received via the heartbeat response.
func (d *CommandDispatcher) DispatchCommands(commands []HeartbeatCommand) {
	d.DispatchCommandsWithSource("heartbeat", commands)
}

// DispatchCommandsWithSource dispatches commands to the appropriate children
// via IPC, recording each consequential action to the local audit trail. The
// source identifies how the command arrived (e.g. "heartbeat", "control_ws").
func (d *CommandDispatcher) DispatchCommandsWithSource(source string, commands []HeartbeatCommand) {
	if len(commands) == 0 {
		return
	}

	var readers map[string]*ipc.Reader
	if d.pm != nil {
		readers = d.pm.GetIPCReaders()
	}

	for _, cmd := range commands {
		if !d.isAllowed(cmd.Action) {
			logger.Infof("Command %s: action %q rejected (not in allowlist)", cmd.ID, cmd.Action)
			d.auditCmd(audit.Event{
				Action:    cmd.Action,
				Source:    source,
				Status:    "denied",
				CommandID: cmd.ID,
				Project:   cmd.TargetProject,
				Detail:    "action not in allowed_remote_commands",
				Payload:   cmd.Payload,
			})
			d.recordResult(cmd.ID, ipc.ResultFailed, fmt.Sprintf("action %q not allowed", cmd.Action))
			continue
		}
		if d.isDuplicate(cmd.ID) {
			logger.Infof("Command %s: duplicate (already executed), skipping", cmd.ID)
			continue
		}

		// Remember the command so its outcome (reported later, possibly from
		// another goroutine via recordResult) can be audited, and record that
		// the command was accepted for execution.
		d.rememberCmd(cmd.ID, cmd.Action, source)
		d.auditCmd(audit.Event{
			Action:    cmd.Action,
			Source:    source,
			Status:    "received",
			CommandID: cmd.ID,
			Project:   cmd.TargetProject,
			Payload:   cmd.Payload,
		})

		if cmd.Action == actionTunnelConnect {
			d.dispatchTunnelConnect(cmd)
			continue
		}
		if cmd.Action == actionTerminalOpen {
			d.dispatchTerminalOpen(cmd)
			continue
		}
		if cmd.Action == actionUpdateCredential {
			d.dispatchUpdateCredential(cmd)
			continue
		}
		if isVPNAction(cmd.Action) {
			d.dispatchVPNCommand(cmd)
			continue
		}
		if d.pm == nil {
			d.recordResult(cmd.ID, ipc.ResultFailed, "Process manager not available")
			continue
		}
		if cmd.TargetProject == "" {
			d.recordResult(cmd.ID, ipc.ResultFailed, "Device-level commands not supported for action: "+cmd.Action)
			continue
		}

		reader, exists := readers[cmd.TargetProject]
		if !exists {
			logger.Infof("Command %s: project %s not found", cmd.ID, cmd.TargetProject)
			d.recordResult(cmd.ID, ipc.ResultFailed, "Project not found: "+cmd.TargetProject)
			continue
		}

		ipcCmd := &ipc.Command{
			ID:        cmd.ID,
			Action:    cmd.Action,
			Payload:   cmd.Payload,
			Timestamp: time.Now(),
		}

		if err := reader.WriteCommand(ipcCmd); err != nil {
			logger.Infof("Failed to dispatch command %s to %s: %v", cmd.ID, cmd.TargetProject, err)
			d.recordResult(cmd.ID, ipc.ResultFailed, "Failed to dispatch: "+err.Error())
			continue
		}

		logger.Infof("Dispatched command %s (%s) to %s", cmd.ID, cmd.Action, cmd.TargetProject)
	}
}

func isVPNAction(action string) bool {
	return action == actionVPNEnable || action == actionVPNUse || action == actionVPNDisable
}

func (d *CommandDispatcher) dispatchVPNCommand(cmd HeartbeatCommand) {
	switch cmd.Action {
	case actionVPNEnable:
		listenPort := 0
		if p, ok := cmd.Payload["listen_port"].(float64); ok {
			listenPort = int(p)
		}
		if err := identity.SetVPNExitNode(listenPort, ""); err != nil {
			d.recordResult(cmd.ID, ipc.ResultFailed, err.Error())
			return
		}
		d.recordResult(cmd.ID, ipc.ResultSuccess, "VPN exit-node config written; master will reconcile")

	case actionVPNUse:
		peerDevice, _ := cmd.Payload["peer_device"].(string)
		if strings.TrimSpace(peerDevice) == "" {
			d.recordResult(cmd.ID, ipc.ResultFailed, "peer_device required in payload")
			return
		}
		if err := identity.SetVPNClient(peerDevice, ""); err != nil {
			d.recordResult(cmd.ID, ipc.ResultFailed, err.Error())
			return
		}
		d.recordResult(cmd.ID, ipc.ResultSuccess, fmt.Sprintf("VPN client config written (peer: %s); master will reconcile", peerDevice))

	case actionVPNDisable:
		if err := identity.ClearVPN(); err != nil {
			d.recordResult(cmd.ID, ipc.ResultFailed, err.Error())
			return
		}
		d.recordResult(cmd.ID, ipc.ResultSuccess, "VPN config cleared; master will reconcile")
	}
}

// passRemoteAccessGate enforces the device-verified passphrase for a gated
// action. When a passphrase is configured, the command must carry a session_id
// that has a valid, single-use, in-memory unlock (established via the
// challenge/unlock round trip on the control socket). With no passphrase
// configured the gate is a no-op and behavior is unchanged.
//
// It returns true when the command may proceed. On denial it records a failed
// result and a "denied" audit entry, then returns false.
func (d *CommandDispatcher) passRemoteAccessGate(cmd HeartbeatCommand) bool {
	if d.grants == nil || !d.grants.IsConfigured() {
		return true
	}
	sid, _ := cmd.Payload["session_id"].(string)
	sid = strings.TrimSpace(sid)
	if sid == "" || !d.grants.Consume(cmd.Action, sid) {
		d.auditCmd(audit.Event{
			Action:    cmd.Action,
			Source:    "local",
			Status:    "denied",
			CommandID: cmd.ID,
			Project:   cmd.TargetProject,
			Detail:    "remote-access passphrase required",
			Payload:   cmd.Payload,
		})
		d.recordResult(cmd.ID, ipc.ResultFailed, "remote-access passphrase required")
		return false
	}
	return true
}

func (d *CommandDispatcher) dispatchTunnelConnect(cmd HeartbeatCommand) {
	if !d.passRemoteAccessGate(cmd) {
		return
	}
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
			logger.Infof("Error reading command result from %s: %v", projectID, err)
			continue
		}
		if result == nil {
			continue
		}

		logger.Infof("Collected result for command %s from %s: %s", result.CommandID, projectID, result.Status)
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

// recordResult adds a command result to the pending results list and records
// the outcome to the audit trail.
func (d *CommandDispatcher) recordResult(commandID, status, message string) {
	d.mu.Lock()
	d.pendingResults = append(d.pendingResults, CommandResultReport{
		CommandID: commandID,
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
	})
	d.mu.Unlock()

	d.auditOutcome(commandID, status, message)
}

// rememberCmd stores context for an in-flight command so its later outcome can
// be attributed to the right action/source in the audit trail.
func (d *CommandDispatcher) rememberCmd(id, action, source string) {
	if id == "" {
		return
	}
	d.metaMu.Lock()
	defer d.metaMu.Unlock()
	d.cmdMeta[id] = cmdMeta{action: action, source: source, at: time.Now()}
	if len(d.cmdMeta) > 500 {
		for k, v := range d.cmdMeta {
			if time.Since(v.at) > commandTTL {
				delete(d.cmdMeta, k)
			}
		}
	}
}

// auditOutcome writes the result of a previously-seen command to the audit
// trail. Commands with no remembered context (e.g. denied before execution,
// already audited inline) are skipped to avoid duplicate records.
func (d *CommandDispatcher) auditOutcome(commandID, status, message string) {
	d.metaMu.Lock()
	meta, ok := d.cmdMeta[commandID]
	if ok {
		delete(d.cmdMeta, commandID)
	}
	d.metaMu.Unlock()
	if !ok {
		return
	}

	auditStatus := status
	switch status {
	case ipc.ResultSuccess:
		auditStatus = "executed"
	case ipc.ResultFailed:
		auditStatus = "failed"
	}
	d.auditCmd(audit.Event{
		Action:    meta.action,
		Source:    meta.source,
		Status:    auditStatus,
		CommandID: commandID,
		Detail:    message,
	})
}

// auditCmd enriches an event with this device's identity and appends it to the
// tamper-evident local audit log. Failures are non-fatal.
func (d *CommandDispatcher) auditCmd(ev audit.Event) {
	if ev.DeviceID == "" || ev.Device == "" {
		id, name := deviceIdentity()
		if ev.DeviceID == "" {
			ev.DeviceID = id
		}
		if ev.Device == "" {
			ev.Device = name
		}
	}
	audit.Log(ev)
}

// deviceIdentity returns the BeaconInfra device id and device name from local
// config, falling back to the hostname for the name.
func deviceIdentity() (id, name string) {
	if uc, err := identity.LoadUserConfig(); err == nil && uc != nil {
		id = strings.TrimSpace(uc.DeviceID)
		name = strings.TrimSpace(uc.DeviceName)
	}
	if name == "" {
		if h, err := os.Hostname(); err == nil {
			name = strings.TrimSpace(h)
		}
	}
	return id, name
}

func (d *CommandDispatcher) dispatchTerminalOpen(cmd HeartbeatCommand) {
	if !d.passRemoteAccessGate(cmd) {
		return
	}
	ws, _ := cmd.Payload["ws_url"].(string)
	if strings.TrimSpace(ws) == "" {
		d.recordResult(cmd.ID, ipc.ResultFailed, "ws_url required in payload")
		return
	}
	uc, err := identity.LoadUserConfig()
	if err != nil || uc == nil {
		d.recordResult(cmd.ID, ipc.ResultFailed, "config unavailable")
		return
	}
	apiKey := strings.TrimSpace(uc.APIKey)
	if apiKey == "" {
		d.recordResult(cmd.ID, ipc.ResultFailed, "API key not configured")
		return
	}
	name := strings.TrimSpace(uc.DeviceName)
	if name == "" {
		if h, e := os.Hostname(); e == nil {
			name = strings.TrimSpace(h)
		}
	}
	go func() {
		err := terminal.RunSession(terminal.RunConfig{WSURL: strings.TrimSpace(ws), APIKey: apiKey, DeviceName: name, CommandID: cmd.ID})
		if err != nil {
			d.recordResult(cmd.ID, ipc.ResultFailed, err.Error())
			return
		}
		d.recordResult(cmd.ID, ipc.ResultSuccess, "remote terminal session finished")
	}()
}

func (d *CommandDispatcher) dispatchUpdateCredential(cmd HeartbeatCommand) {
	credType, _ := cmd.Payload["credential_type"].(string)
	keyName, _ := cmd.Payload["key_name"].(string)
	credValue, _ := cmd.Payload["credential_value"].(string)
	projectID, _ := cmd.Payload["project_id"].(string)

	if credType == "" || credValue == "" {
		d.recordResult(cmd.ID, ipc.ResultFailed, "credential_type and credential_value required")
		return
	}

	if keyName == "" {
		keyName = credType + "_token"
	}

	configDir := credentialConfigDir()
	km, err := keys.NewKeyManager(configDir)
	if err != nil {
		d.recordResult(cmd.ID, ipc.ResultFailed, "key manager init: "+err.Error())
		return
	}

	if err := km.RotateKey(keyName, credValue); err != nil {
		if err := km.AddKey(keyName, credValue, credType, "Remote credential update"); err != nil {
			d.recordResult(cmd.ID, ipc.ResultFailed, "store credential: "+err.Error())
			return
		}
	}

	if projectID != "" {
		stateDir := credentialStateDir()
		deploy.ClearCredentialErrors(stateDir, projectID)
	}

	logger.Infof("Credential updated via remote command: %s/%s", credType, keyName)
	d.recordResult(cmd.ID, ipc.ResultSuccess, fmt.Sprintf("credential updated: %s/%s", credType, keyName))
}

func credentialConfigDir() string {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return ".beacon"
	}
	return base
}

func credentialStateDir() string {
	base, err := config.BeaconHomeDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return home + "/.beacon/state"
	}
	return base + "/state"
}
