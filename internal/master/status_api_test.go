package master

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"beacon/internal/audit"
	"beacon/internal/config"
	"beacon/internal/projects"
)

func TestStatusServerDashboardAPIs(t *testing.T) {
	server := setupDashboardAPITest(t)

	t.Run("projects list redacts sensitive config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		rr := httptest.NewRecorder()
		server.handleAPIProjects(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if strings.Contains(body, "super-secret") || strings.Contains(body, "should-not-appear") {
			t.Fatalf("response leaked sensitive value: %s", body)
		}
		if !strings.Contains(body, `"repo_url":"https://redacted@example.com/repo.git"`) {
			t.Fatalf("response missing redacted repo url: %s", body)
		}
	})

	t.Run("project detail includes configured checks and deploy state", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/projects/myapp", nil)
		rr := httptest.NewRecorder()
		server.handleAPIProject(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
		}
		var detail projectDetail
		if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		if detail.Project.LastDeploy.LastTag != "v1.2.3" {
			t.Fatalf("LastTag = %q, want v1.2.3", detail.Project.LastDeploy.LastTag)
		}
		if len(detail.Checks) != 1 || detail.Checks[0].Name != "web" {
			t.Fatalf("configured checks = %#v, want web", detail.Checks)
		}
		if detail.Project.Alerts.Rules != 1 || detail.Project.Alerts.MonitorRules != 1 {
			t.Fatalf("alerts = %#v, want simple and monitor rules", detail.Project.Alerts)
		}
	})

	t.Run("project logs tails deploy log", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/projects/myapp/logs?limit=1", nil)
		rr := httptest.NewRecorder()
		server.handleAPIProject(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
		}
		var logs projectLogsResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &logs); err != nil {
			t.Fatal(err)
		}
		if len(logs.Lines) != 1 || logs.Lines[0] != "second" {
			t.Fatalf("logs = %#v, want tail line second", logs.Lines)
		}
	})

	t.Run("audit APIs list and verify chain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit?limit=10", nil)
		rr := httptest.NewRecorder()
		server.handleAPIAudit(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "deploy_requested") {
			t.Fatalf("audit response missing event: %s", rr.Body.String())
		}

		req = httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil)
		rr = httptest.NewRecorder()
		server.handleAPIAuditVerify(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"OK":true`) && !strings.Contains(rr.Body.String(), `"ok":true`) {
			t.Fatalf("verify response not OK: %s", rr.Body.String())
		}
	})
}

func setupDashboardAPITest(t *testing.T) *StatusServer {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BEACON_HOME", filepath.Join(home, ".beacon"))
	audit.SetPathForTesting(filepath.Join(home, ".beacon", "logs", "audit.jsonl"))
	t.Cleanup(func() { audit.SetPathForTesting("") })

	paths, err := config.NewBeaconPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.CreateProjectStructure("myapp"); err != nil {
		t.Fatal(err)
	}
	writeDashboardAPIProjectFiles(t, home, paths)
	writeDashboardAPIState(t, paths)
	writeDashboardAPIInventory(t, home, paths)
	if err := audit.Record(audit.Event{Action: "deploy_requested", Source: "local", Status: "executed", Project: "myapp", Detail: "done"}); err != nil {
		t.Fatal(err)
	}

	cache := &StatusCache{snapshot: StatusSnapshot{Children: []ChildStatus{{
		Name:   "myapp",
		Status: "healthy",
		Checks: CheckSummary{Total: 1, Passing: 1, Details: []CheckDetail{{
			Name:   "web",
			Type:   "http",
			Status: "passing",
		}}},
	}}}}
	return NewStatusServer(cache, 0)
}

func writeDashboardAPIProjectFiles(t *testing.T, home string, paths *config.BeaconPaths) {
	t.Helper()
	envPath := paths.GetProjectEnvFile("myapp")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"BEACON_PROJECT_NAME=myapp",
		"BEACON_PROJECT_ENV=prod",
		"BEACON_DEPLOYMENT_TYPE=git",
		"BEACON_REPO_URL=https://token:super-secret@example.com/repo.git",
		"BEACON_LOCAL_PATH=" + filepath.Join(home, "app"),
		"BEACON_DEPLOY_CMD=./deploy.sh",
		"BEACON_GIT_TOKEN=should-not-appear",
	}, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GetProjectMonitorFile("myapp"), []byte(`checks:
  - name: web
    type: http
    url: http://localhost:8080/health
    interval: 30s
plugins:
  - name: webhook
    enabled: true
alert_rules:
  - check: web
    severity: critical
    plugins: [webhook]
`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GetProjectAlertsFile("myapp"), []byte(`alert_channels:
  email:
    enabled: true
alert_routing:
  - severity: critical
    channels: [email]
`), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeDashboardAPIState(t *testing.T, paths *config.BeaconPaths) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(paths.StateDir, "myapp"), 0755); err != nil {
		t.Fatal(err)
	}
	deployedAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	if err := os.WriteFile(filepath.Join(paths.StateDir, "myapp", "status.json"), []byte(`{"last_tag":"v1.2.3","last_deployed":"`+deployedAt.Format(time.RFC3339)+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(paths.GetProjectLogsDir("myapp"), "deploy.log")
	if err := os.WriteFile(logPath, []byte("first\nsecond\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeDashboardAPIInventory(t *testing.T, home string, paths *config.BeaconPaths) {
	t.Helper()
	inv := &projects.Inventory{}
	projects.AddProject(inv, "myapp", filepath.Join(home, "app"), paths.GetProjectConfigDir("myapp"))
	if err := projects.SaveInventory(paths.GetProjectsFilePath(), inv); err != nil {
		t.Fatal(err)
	}
}
