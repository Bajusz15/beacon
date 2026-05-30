package master

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"beacon/internal/audit"
	"beacon/internal/config"
	"beacon/internal/monitor"
	"beacon/internal/plugins"
	"beacon/internal/projects"
	"beacon/internal/state"
	"beacon/internal/util"

	"gopkg.in/yaml.v3"
)

const (
	defaultAPILimit = 100
	maxAPILimit     = 1000
)

type dashboardProject struct {
	Name        string            `json:"name"`
	Location    string            `json:"location,omitempty"`
	ConfigDir   string            `json:"config_dir,omitempty"`
	Status      string            `json:"status,omitempty"`
	Version     string            `json:"version,omitempty"`
	PID         int               `json:"pid,omitempty"`
	DeployedAt  *time.Time        `json:"deployed_at,omitempty"`
	Checks      CheckSummary      `json:"checks"`
	Config      projectConfigView `json:"config"`
	Files       projectFilesView  `json:"files"`
	Alerts      projectAlertsView `json:"alerts"`
	LastDeploy  projectDeployView `json:"last_deploy"`
	ConfigError string            `json:"config_error,omitempty"`
	StateError  string            `json:"state_error,omitempty"`
}

type projectFilesView struct {
	Env     fileView `json:"env"`
	Monitor fileView `json:"monitor"`
	Alerts  fileView `json:"alerts"`
	Log     fileView `json:"deploy_log"`
}

type fileView struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type projectConfigView struct {
	DeploymentType string            `json:"deployment_type,omitempty"`
	RepoURL        string            `json:"repo_url,omitempty"`
	LocalPath      string            `json:"local_path,omitempty"`
	DeployCommand  string            `json:"deploy_command,omitempty"`
	ProjectEnv     string            `json:"project_env,omitempty"`
	SecureEnvPath  string            `json:"secure_env_path,omitempty"`
	DockerImages   []dockerImageView `json:"docker_images,omitempty"`
}

type dockerImageView struct {
	Image              string   `json:"image,omitempty"`
	Registry           string   `json:"registry,omitempty"`
	DeployCommand      string   `json:"deploy_command,omitempty"`
	DockerComposeFiles []string `json:"docker_compose_files,omitempty"`
}

type projectAlertsView struct {
	ConfigExists bool     `json:"config_exists"`
	Channels     []string `json:"channels,omitempty"`
	Rules        int      `json:"rules"`
	MonitorRules int      `json:"monitor_rules"`
	Plugins      []string `json:"plugins,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type projectDeployView struct {
	LastTag      string     `json:"last_tag,omitempty"`
	LastDeployed *time.Time `json:"last_deployed,omitempty"`
	Command      string     `json:"command,omitempty"`
	LogPath      string     `json:"log_path,omitempty"`
	LogExists    bool       `json:"log_exists"`
}

type projectCheckConfigView struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Target          string `json:"target,omitempty"`
	Interval        string `json:"interval,omitempty"`
	AlertConfigured bool   `json:"alert_configured"`
}

type projectDetail struct {
	Project dashboardProject         `json:"project"`
	Checks  []projectCheckConfigView `json:"configured_checks"`
}

type projectLogsResponse struct {
	Project string   `json:"project"`
	Path    string   `json:"path"`
	Lines   []string `json:"lines"`
}

func (s *StatusServer) handleAPIProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/api/projects" {
		http.NotFound(w, r)
		return
	}
	projects, err := s.loadDashboardProjects()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *StatusServer) handleAPIProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name, err := url.PathUnescape(parts[0])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateProjectParam(name); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	if len(parts) == 1 {
		s.handleAPIProjectDetail(w, name)
		return
	}
	switch parts[1] {
	case "deployments":
		s.handleAPIProjectDeployments(w, name)
	case "logs":
		s.handleAPIProjectLogs(w, r, name)
	default:
		http.NotFound(w, r)
	}
}

func (s *StatusServer) handleAPIProjectDetail(w http.ResponseWriter, name string) {
	project, checks, err := s.loadDashboardProject(name)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err)
		return
	}
	util.WriteJSON(w, http.StatusOK, projectDetail{Project: project, Checks: checks})
}

func (s *StatusServer) handleAPIProjectDeployments(w http.ResponseWriter, name string) {
	project, _, err := s.loadDashboardProject(name)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err)
		return
	}
	util.WriteJSON(w, http.StatusOK, project.LastDeploy)
}

func (s *StatusServer) handleAPIProjectLogs(w http.ResponseWriter, r *http.Request, name string) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	if !paths.ProjectExists(name) {
		writeAPIError(w, http.StatusNotFound, fmt.Errorf("project %q not found", name))
		return
	}
	limit := parseLimit(r, 200)
	path := projectDeployLogPath(paths, name)
	lines, err := tailFile(path, limit)
	if err != nil {
		if os.IsNotExist(err) {
			lines = []string{}
		} else {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
	}
	util.WriteJSON(w, http.StatusOK, projectLogsResponse{Project: name, Path: path, Lines: lines})
}

func (s *StatusServer) handleAPIAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/api/audit" {
		http.NotFound(w, r)
		return
	}
	entries, err := audit.ReadAll()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	filtered := make([]audit.Entry, 0, len(entries))
	for _, entry := range entries {
		if action != "" && entry.Action != action {
			continue
		}
		if status != "" && entry.Status != status {
			continue
		}
		filtered = append(filtered, entry)
	}
	limit := parseLimit(r, defaultAPILimit)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{"entries": filtered})
}

func (s *StatusServer) handleAPIAuditVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/api/audit/verify" {
		http.NotFound(w, r)
		return
	}
	result, err := audit.Verify()
	if err != nil && result.Count == 0 && !result.OK {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	util.WriteJSON(w, http.StatusOK, result)
}

func (s *StatusServer) loadDashboardProjects() ([]dashboardProject, error) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return nil, err
	}
	names, err := paths.ListProjects()
	if err != nil {
		return nil, err
	}
	inv, _ := projects.LoadInventory(paths.GetProjectsFilePath())
	out := make([]dashboardProject, 0, len(names))
	for _, name := range names {
		project, _, err := s.loadDashboardProjectWithPaths(paths, inv, name)
		if err != nil {
			out = append(out, dashboardProject{Name: name, ConfigError: err.Error()})
			continue
		}
		out = append(out, project)
	}
	return out, nil
}

func (s *StatusServer) loadDashboardProject(name string) (dashboardProject, []projectCheckConfigView, error) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return dashboardProject{}, nil, err
	}
	inv, _ := projects.LoadInventory(paths.GetProjectsFilePath())
	return s.loadDashboardProjectWithPaths(paths, inv, name)
}

func (s *StatusServer) loadDashboardProjectWithPaths(paths *config.BeaconPaths, inv *projects.Inventory, name string) (dashboardProject, []projectCheckConfigView, error) {
	if err := validateProjectParam(name); err != nil {
		return dashboardProject{}, nil, err
	}
	if !paths.ProjectExists(name) {
		return dashboardProject{}, nil, fmt.Errorf("project %q not found", name)
	}

	configDir := paths.GetProjectConfigDir(name)
	location := paths.GetProjectWorkingDir(name)
	if inv != nil {
		if entry := projects.GetProject(inv, name); entry != nil {
			configDir = entry.ConfigDir
			location = entry.Location
		}
	}

	envFile := paths.GetProjectEnvFile(name)
	monitorFile := paths.GetProjectMonitorFile(name)
	alertsFile := paths.GetProjectAlertsFile(name)
	logPath := projectDeployLogPath(paths, name)

	project := dashboardProject{
		Name:      name,
		Location:  location,
		ConfigDir: configDir,
		Status:    "unknown",
		Files: projectFilesView{
			Env:     fileStatus(envFile),
			Monitor: fileStatus(monitorFile),
			Alerts:  fileStatus(alertsFile),
			Log:     fileStatus(logPath),
		},
	}

	if child, ok := s.childStatusByName(name); ok {
		project.Status = child.Status
		project.Version = child.Version
		project.PID = child.PID
		project.DeployedAt = child.DeployedAt
		project.Checks = child.Checks
	}

	envValues, err := readProjectEnv(envFile)
	if err != nil && !os.IsNotExist(err) {
		project.ConfigError = err.Error()
	}
	project.Config = projectConfigFromEnv(envValues)
	if project.Config.LocalPath == "" {
		project.Config.LocalPath = location
	}
	if project.Config.DeploymentType == "docker" {
		project.Config.DockerImages = readDockerImages(paths, name)
	}

	var checks []projectCheckConfigView
	var monitorRules int
	var pluginNames []string
	if fileExists(monitorFile) {
		var monitorErr error
		checks, monitorRules, pluginNames, monitorErr = readMonitorSummary(monitorFile)
		if monitorErr != nil {
			project.ConfigError = monitorErr.Error()
		}
	}
	project.Alerts = readAlertSummary(alertsFile)
	project.Alerts.MonitorRules = monitorRules
	project.Alerts.Plugins = pluginNames

	lastTag, deployedAt, stateErr := readDeployState(paths, name)
	if stateErr != nil && !os.IsNotExist(stateErr) {
		project.StateError = stateErr.Error()
	}
	project.LastDeploy = projectDeployView{
		LastTag:   lastTag,
		Command:   project.Config.DeployCommand,
		LogPath:   logPath,
		LogExists: project.Files.Log.Exists,
	}
	if !deployedAt.IsZero() {
		project.LastDeploy.LastDeployed = &deployedAt
	}
	return project, checks, nil
}

func (s *StatusServer) childStatusByName(name string) (ChildStatus, bool) {
	if s.cache == nil {
		return ChildStatus{}, false
	}
	snap := s.cache.Get()
	for _, child := range snap.Children {
		if child.Name == name {
			return child, true
		}
	}
	return ChildStatus{}, false
}

func readProjectEnv(path string) (map[string]string, error) {
	base := util.EnvSliceToMap(os.Environ())
	return util.ReadEnvFileMap(path, base)
}

func projectConfigFromEnv(values map[string]string) projectConfigView {
	deploymentType := values["BEACON_DEPLOYMENT_TYPE"]
	if deploymentType == "" {
		deploymentType = "git"
	}
	return projectConfigView{
		DeploymentType: deploymentType,
		RepoURL:        redactString(values["BEACON_REPO_URL"]),
		LocalPath:      os.ExpandEnv(values["BEACON_LOCAL_PATH"]),
		DeployCommand:  values["BEACON_DEPLOY_CMD"],
		ProjectEnv:     values["BEACON_PROJECT_ENV"],
		SecureEnvPath:  os.ExpandEnv(values["BEACON_SECURE_ENV_PATH"]),
	}
}

func readDockerImages(paths *config.BeaconPaths, project string) []dockerImageView {
	path := filepath.Join(paths.GetProjectConfigDir(project), "docker-images.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var images []config.DockerImageConfig
	if err := yaml.Unmarshal(data, &images); err != nil {
		return nil
	}
	out := make([]dockerImageView, 0, len(images))
	for _, image := range images {
		out = append(out, dockerImageView{
			Image:              image.Image,
			Registry:           image.Registry,
			DeployCommand:      image.DeployCommand,
			DockerComposeFiles: image.DockerComposeFiles,
		})
	}
	return out
}

func readMonitorSummary(path string) ([]projectCheckConfigView, int, []string, error) {
	cfg, err := monitor.LoadConfig(path)
	if err != nil {
		return nil, 0, nil, err
	}
	checks := make([]projectCheckConfigView, 0, len(cfg.Checks))
	for _, check := range cfg.Checks {
		checks = append(checks, projectCheckConfigView{
			Name:            check.Name,
			Type:            check.Type,
			Target:          checkTarget(check),
			Interval:        check.Interval.String(),
			AlertConfigured: check.AlertCommand != "",
		})
	}
	plugins := make([]string, 0, len(cfg.Plugins))
	for _, plugin := range cfg.Plugins {
		if plugin.Name != "" {
			plugins = append(plugins, plugin.Name)
		}
	}
	return checks, len(cfg.AlertRules), plugins, nil
}

func readAlertSummary(path string) projectAlertsView {
	summary := projectAlertsView{ConfigExists: fileExists(path)}
	if !summary.ConfigExists {
		return summary
	}
	data, err := os.ReadFile(path)
	if err != nil {
		summary.Error = err.Error()
		return summary
	}
	var raw struct {
		Channels map[string]any         `yaml:"alert_channels"`
		Routing  []map[string]any       `yaml:"alert_routing"`
		Rules    []map[string]any       `yaml:"alert_rules"`
		Plugins  []plugins.PluginConfig `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		summary.Error = err.Error()
		return summary
	}
	for name := range raw.Channels {
		summary.Channels = append(summary.Channels, name)
	}
	for _, plugin := range raw.Plugins {
		if plugin.Name != "" {
			summary.Plugins = append(summary.Plugins, plugin.Name)
		}
	}
	summary.Rules = len(raw.Routing) + len(raw.Rules)
	return summary
}

func readDeployState(paths *config.BeaconPaths, project string) (string, time.Time, error) {
	statusPath := filepath.Join(paths.StateDir, project, "status.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		legacyPath := filepath.Join(paths.BaseDir, project, "status.json")
		data, err = os.ReadFile(legacyPath)
		if err != nil {
			return "", time.Time{}, err
		}
	}
	var st state.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return "", time.Time{}, err
	}
	return st.LastTag, st.LastDeployed, nil
}

func checkTarget(check monitor.CheckConfig) string {
	switch check.Type {
	case "http":
		return check.URL
	case "port":
		if check.Host == "" && check.Port == 0 {
			return ""
		}
		return fmt.Sprintf("%s:%d", check.Host, check.Port)
	case "command":
		return check.Cmd
	default:
		return ""
	}
}

func fileStatus(path string) fileView {
	return fileView{Path: path, Exists: fileExists(path)}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func projectDeployLogPath(paths *config.BeaconPaths, project string) string {
	return filepath.Join(paths.GetProjectLogsDir(project), "deploy.log")
}

func tailFile(path string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer util.Close(file, "dashboard log file")
	lines := make([]string, 0, limit)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if len(lines) == limit {
			copy(lines, lines[1:])
			lines[len(lines)-1] = line
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func parseLimit(r *http.Request, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		return fallback
	}
	if limit > maxAPILimit {
		return maxAPILimit
	}
	return limit
}

func validateProjectParam(name string) error {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return err
	}
	return paths.ValidateProjectName(name)
}

func redactString(value string) string {
	if value == "" {
		return ""
	}
	if u, err := url.Parse(value); err == nil && u.User != nil {
		u.User = url.User(redactedValue)
		value = u.String()
	}
	for _, marker := range []string{"token=", "access_token=", "password=", "api_key="} {
		idx := strings.Index(strings.ToLower(value), marker)
		if idx == -1 {
			continue
		}
		start := idx + len(marker)
		end := start
		for end < len(value) && value[end] != '&' && value[end] != ' ' {
			end++
		}
		value = value[:start] + redactedValue + value[end:]
	}
	return value
}

const redactedValue = "redacted"

func writeAPIError(w http.ResponseWriter, status int, err error) {
	util.WriteJSON(w, status, map[string]string{"error": err.Error()})
}
