package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/projects"
	"beacon/internal/state"
	"beacon/internal/systemd"
)

// ToolBackend provides data access for MCP tools
type ToolBackend struct {
	Paths     *config.BeaconPaths
	Config    *Config
	Confirm   *ConfirmationTokenStore
	RateLimit *ToolRateLimiter
}

// InventoryOutput is the result of beacon_inventory
type InventoryOutput struct {
	Projects []projects.ProjectEntry `json:"projects"`
}

// StatusOutput is the result of beacon_status
type StatusOutput struct {
	Project   string    `json:"project"`
	UpdatedAt time.Time `json:"updated_at"`
	Checks    []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	} `json:"checks"`
}

// LogsOutput is the result of beacon_logs
type LogsOutput struct {
	Project string   `json:"project"`
	Lines   []string `json:"lines"`
}

// DiffOutput is the result of beacon_diff
type DiffOutput struct {
	Project string `json:"project"`
	From    string `json:"from"`
	To      string `json:"to"`
	Diff    string `json:"diff"`
}

// DeployOutput is the result of beacon_deploy
type DeployOutput struct {
	Message           string `json:"message"`
	ConfirmationToken string `json:"confirmation_token,omitempty"`
	Deployed          bool   `json:"deployed,omitempty"`
}

// RestartOutput is the result of beacon_restart
type RestartOutput struct {
	Message           string `json:"message"`
	ConfirmationToken string `json:"confirmation_token,omitempty"`
	Restarted         bool   `json:"restarted,omitempty"`
}

func (b *ToolBackend) getInventory() (*projects.Inventory, error) {
	invPath := b.Paths.GetProjectsFilePath()
	inv, err := projects.LoadInventory(invPath)
	if err != nil {
		return nil, err
	}
	dirProjects, _ := b.Paths.ListProjects()
	for _, name := range dirProjects {
		if projects.GetProject(inv, name) == nil {
			configDir := b.Paths.GetProjectConfigDir(name)
			location := b.Paths.GetProjectWorkingDir(name)
			projects.AddProject(inv, name, location, configDir)
		}
	}
	return inv, nil
}

func (b *ToolBackend) projectNames() ([]string, error) {
	inv, err := b.getInventory()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range inv.Projects {
		names = append(names, p.Name)
	}
	return names, nil
}

const maxDiffBytes = 50 * 1024

func (b *ToolBackend) ToolInventory() (InventoryOutput, error) {
	inv, err := b.getInventory()
	if err != nil {
		return InventoryOutput{}, err
	}
	return InventoryOutput{Projects: inv.Projects}, nil
}

func (b *ToolBackend) ToolStatus(project string) (StatusOutput, error) {
	names, err := b.projectNames()
	if err != nil {
		return StatusOutput{}, err
	}
	if project != "" {
		if err := ValidateProjectName(project, names); err != nil {
			return StatusOutput{}, err
		}
		names = []string{project}
	}

	var out StatusOutput
	for _, name := range names {
		path := filepath.Join(b.Paths.StateDir, name, "checks.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var st state.ChecksState
		if err := json.Unmarshal(data, &st); err != nil {
			continue
		}
		out.Project = name
		out.UpdatedAt = st.UpdatedAt
		for _, c := range st.Checks {
			out.Checks = append(out.Checks, struct {
				Name   string `json:"name"`
				Status string `json:"status"`
				Error  string `json:"error,omitempty"`
			}{c.Name, c.Status, c.Error})
		}
		if project != "" {
			break
		}
	}
	return out, nil
}

func (b *ToolBackend) ToolLogs(project, since, grep string) (LogsOutput, error) {
	names, err := b.projectNames()
	if err != nil {
		return LogsOutput{}, err
	}
	if err := ValidateProjectName(project, names); err != nil {
		return LogsOutput{}, err
	}
	if grep != "" {
		if _, err := ValidateGrepPattern(grep); err != nil {
			return LogsOutput{}, err
		}
	}

	logsDir := b.Paths.GetProjectLogsDir(project)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return LogsOutput{Project: project, Lines: []string{}}, nil
	}

	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(logsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if grep != "" && !strings.Contains(line, grep) {
				continue
			}
			lines = append(lines, line)
		}
	}
	if len(lines) > 500 {
		lines = lines[len(lines)-500:]
	}
	return LogsOutput{Project: project, Lines: lines}, nil
}

func (b *ToolBackend) ToolDiff(project, from, to string) (DiffOutput, error) {
	inv, err := b.getInventory()
	if err != nil {
		return DiffOutput{}, err
	}
	p := projects.GetProject(inv, project)
	if p == nil {
		return DiffOutput{}, fmt.Errorf("project %q not in inventory", project)
	}
	if err := ValidateGitRef(from); err != nil {
		return DiffOutput{}, err
	}
	if err := ValidateGitRef(to); err != nil {
		return DiffOutput{}, err
	}

	cmd := exec.Command("git", "diff", from+".."+to)
	cmd.Dir = p.Location
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return DiffOutput{}, fmt.Errorf("git diff: %w: %s", err, string(out))
	}
	diff := string(out)
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes] + "\n... (truncated)"
	}
	return DiffOutput{Project: project, From: from, To: to, Diff: diff}, nil
}

func (b *ToolBackend) ToolDeploy(project, ref, confirmationToken string) (DeployOutput, error) {
	if !b.Config.DeployEnabled {
		return DeployOutput{Message: "deploy is disabled; set BEACON_MCP_DEPLOY_ENABLED=1 to enable"}, nil
	}
	if !b.RateLimit.Allow("beacon_deploy") {
		return DeployOutput{}, fmt.Errorf("rate limited; try again later")
	}

	names, err := b.projectNames()
	if err != nil {
		return DeployOutput{}, err
	}
	if err := ValidateProjectName(project, names); err != nil {
		return DeployOutput{}, err
	}
	if ref != "" {
		if err := ValidateGitRef(ref); err != nil {
			return DeployOutput{}, err
		}
	}

	if confirmationToken != "" {
		tool, args, err := b.Confirm.Confirm(confirmationToken)
		if err != nil {
			return DeployOutput{}, err
		}
		if tool != "beacon_deploy" {
			return DeployOutput{}, fmt.Errorf("token is for %s, not deploy", tool)
		}
		project = args["project"].(string)
		refVal, _ := args["ref"].(string)

		envPath := b.Paths.GetProjectEnvFile(project)
		if err := configLoadEnv(envPath); err != nil {
			return DeployOutput{}, err
		}
		_ = os.Setenv("BEACON_PROJECT_NAME", project)
		cfg := config.Load()
		statusPath := filepath.Join(os.Getenv("HOME"), ".beacon", cfg.ProjectDir)
		st := state.NewStatus(statusPath)
		if err := deployProject(cfg, refVal, st); err != nil {
			return DeployOutput{}, err
		}
		return DeployOutput{Message: "deploy completed", Deployed: true}, nil
	}

	token, err := b.Confirm.CreateToken("beacon_deploy", map[string]any{"project": project, "ref": ref})
	if err != nil {
		return DeployOutput{}, err
	}
	return DeployOutput{
		Message:           "deploy requires confirmation; call again with confirmation_token",
		ConfirmationToken: token,
	}, nil
}

func (b *ToolBackend) ToolRestart(project, service, confirmationToken string) (RestartOutput, error) {
	if !b.Config.RestartEnabled {
		return RestartOutput{Message: "restart is disabled; set BEACON_MCP_RESTART_ENABLED=1 to enable"}, nil
	}
	if !b.RateLimit.Allow("beacon_restart") {
		return RestartOutput{}, fmt.Errorf("rate limited; try again later")
	}

	names, err := b.projectNames()
	if err != nil {
		return RestartOutput{}, err
	}
	if err := ValidateProjectName(project, names); err != nil {
		return RestartOutput{}, err
	}
	if service == "" {
		service = "deploy"
	}

	if confirmationToken != "" {
		tool, args, err := b.Confirm.Confirm(confirmationToken)
		if err != nil {
			return RestartOutput{}, err
		}
		if tool != "beacon_restart" {
			return RestartOutput{}, fmt.Errorf("token is for %s, not restart", tool)
		}
		project = args["project"].(string)
		_, _ = args["service"].(string)

		if err := restartSystemdService(project); err != nil {
			return RestartOutput{}, err
		}
		return RestartOutput{Message: "restart requested", Restarted: true}, nil
	}

	token, err := b.Confirm.CreateToken("beacon_restart", map[string]any{"project": project, "service": service})
	if err != nil {
		return RestartOutput{}, err
	}
	return RestartOutput{
		Message:           "restart requires confirmation; call again with confirmation_token",
		ConfirmationToken: token,
	}, nil
}

func configLoadEnv(envPath string) error {
	f, err := os.Open(envPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		value = os.ExpandEnv(value)
		_ = os.Setenv(key, value)
	}
	return scanner.Err()
}

func deployProject(cfg *config.Config, tag string, st *state.Status) error {
	return deploy.Deploy(cfg, tag, st)
}

func restartSystemdService(projectName string) error {
	sm := systemd.NewServiceManager(systemd.UserService)
	return sm.RestartService(projectName)
}
