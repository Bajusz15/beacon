package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"beacon/internal/config"
	"beacon/internal/projects"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPTools_Inventory(t *testing.T) {
	homeDir := t.TempDir()
	paths := config.NewBeaconPathsFromBase(homeDir)
	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	invPath := paths.GetProjectsFilePath()
	inv := &projects.Inventory{
		Projects: []projects.ProjectEntry{
			{Name: "testproj", Location: "/tmp/testproj", ConfigDir: paths.GetProjectConfigDir("testproj")},
		},
	}
	if err := projects.SaveInventory(invPath, inv); err != nil {
		t.Fatalf("SaveInventory: %v", err)
	}

	server, _, err := NewServerAndBackend(homeDir)
	if err != nil {
		t.Fatalf("NewServerAndBackend: %v", err)
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "beacon_inventory",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error")
	}

	var found bool
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			var out InventoryOutput
			if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(out.Projects) != 1 {
				t.Errorf("expected 1 project, got %d", len(out.Projects))
			}
			if len(out.Projects) > 0 && out.Projects[0].Name != "testproj" {
				t.Errorf("expected testproj, got %s", out.Projects[0].Name)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no TextContent in result")
	}
}

func TestMCPTools_Status(t *testing.T) {
	homeDir := t.TempDir()
	paths := config.NewBeaconPathsFromBase(homeDir)
	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	projects.AddProject(
		&projects.Inventory{Projects: []projects.ProjectEntry{}},
		"testproj",
		filepath.Join(homeDir, "beacon", "testproj"),
		paths.GetProjectConfigDir("testproj"),
	)
	if err := projects.SaveInventory(paths.GetProjectsFilePath(), &projects.Inventory{
		Projects: []projects.ProjectEntry{
			{Name: "testproj", Location: filepath.Join(homeDir, "beacon", "testproj"), ConfigDir: paths.GetProjectConfigDir("testproj")},
		},
	}); err != nil {
		t.Fatalf("SaveInventory: %v", err)
	}

	stateDir := paths.GetProjectStateDir("testproj")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("MkdirAll state: %v", err)
	}
	checksPath := filepath.Join(stateDir, "checks.json")
	checksData := `{"updated_at":"2024-01-01T00:00:00Z","checks":[{"name":"http","status":"up","timestamp":"2024-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(checksPath, []byte(checksData), 0644); err != nil {
		t.Fatalf("WriteFile checks: %v", err)
	}

	server, _, err := NewServerAndBackend(homeDir)
	if err != nil {
		t.Fatalf("NewServerAndBackend: %v", err)
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "beacon_status",
		Arguments: map[string]any{
			"project": "testproj",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error")
	}

	var found bool
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			var out StatusOutput
			if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if out.Project != "testproj" {
				t.Errorf("expected project testproj, got %s", out.Project)
			}
			if len(out.Checks) != 1 || out.Checks[0].Name != "http" || out.Checks[0].Status != "up" {
				t.Errorf("unexpected checks: %+v", out.Checks)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no TextContent in result")
	}
}
