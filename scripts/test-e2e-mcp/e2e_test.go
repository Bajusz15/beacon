package e2emcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2EMCPTools(t *testing.T) {
	endpoint := os.Getenv("BEACON_MCP_ENDPOINT")
	if endpoint == "" {
		t.Skip("BEACON_MCP_ENDPOINT not set; run via scripts/test-e2e-mcp.sh")
	}
	t.Logf("connecting to MCP server at %s", endpoint)

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "1.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Logf("calling %s with args %v", name, args)
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("CallTool %s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("tool %s returned error", name)
		}
		t.Logf("  %s OK", name)
		return result
	}

	// beacon_inventory
	t.Log("testing beacon_inventory")
	invResult := call("beacon_inventory", map[string]any{})
	invText := extractText(t, invResult)
	var inv struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	mustUnmarshal(t, invText, &inv)
	if len(inv.Projects) < 1 || inv.Projects[0].Name != "e2e-mcp-proj" {
		t.Fatalf("beacon_inventory: expected e2e-mcp-proj, got %+v", inv.Projects)
	}

	// beacon_status
	t.Log("testing beacon_status")
	statusResult := call("beacon_status", map[string]any{"project": "e2e-mcp-proj"})
	statusText := extractText(t, statusResult)
	var status struct {
		Project string `json:"project"`
		Checks  []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	mustUnmarshal(t, statusText, &status)
	if status.Project != "e2e-mcp-proj" || len(status.Checks) < 2 {
		t.Fatalf("beacon_status: expected project e2e-mcp-proj with 2+ checks, got %+v", status)
	}

	// beacon_logs
	t.Log("testing beacon_logs")
	logsResult := call("beacon_logs", map[string]any{"project": "e2e-mcp-proj"})
	logsText := extractText(t, logsResult)
	var logs struct {
		Project string   `json:"project"`
		Lines   []string `json:"lines"`
	}
	mustUnmarshal(t, logsText, &logs)
	if logs.Project != "e2e-mcp-proj" || len(logs.Lines) < 1 {
		t.Fatalf("beacon_logs: expected lines, got %+v", logs)
	}

	// beacon_logs with grep
	t.Log("testing beacon_logs (grep=Done)")
	logsGrepResult := call("beacon_logs", map[string]any{"project": "e2e-mcp-proj", "grep": "Done"})
	logsGrepText := extractText(t, logsGrepResult)
	var logsGrep struct {
		Lines []string `json:"lines"`
	}
	mustUnmarshal(t, logsGrepText, &logsGrep)
	if len(logsGrep.Lines) < 1 || !strings.Contains(logsGrep.Lines[0], "Done") {
		t.Fatalf("beacon_logs grep: expected line containing Done, got %+v", logsGrep.Lines)
	}

	// beacon_diff
	t.Log("testing beacon_diff (v1.0.0..v2.0.0)")
	diffResult := call("beacon_diff", map[string]any{"project": "e2e-mcp-proj", "from": "v1.0.0", "to": "v2.0.0"})
	diffText := extractText(t, diffResult)
	var diff struct {
		Project string `json:"project"`
		From    string `json:"from"`
		To      string `json:"to"`
		Diff    string `json:"diff"`
	}
	mustUnmarshal(t, diffText, &diff)
	if diff.Project != "e2e-mcp-proj" || diff.From != "v1.0.0" || diff.To != "v2.0.0" || diff.Diff == "" {
		t.Fatalf("beacon_diff: expected diff output, got %+v", diff)
	}

	// beacon_deploy (disabled by default)
	t.Log("testing beacon_deploy (expect disabled)")
	deployResult := call("beacon_deploy", map[string]any{"project": "e2e-mcp-proj"})
	deployText := extractText(t, deployResult)
	var deploy struct {
		Message string `json:"message"`
	}
	mustUnmarshal(t, deployText, &deploy)
	if !strings.Contains(deploy.Message, "disabled") {
		t.Fatalf("beacon_deploy: expected disabled message, got %q", deploy.Message)
	}

	// beacon_restart (disabled by default)
	t.Log("testing beacon_restart (expect disabled)")
	restartResult := call("beacon_restart", map[string]any{"project": "e2e-mcp-proj"})
	restartText := extractText(t, restartResult)
	var restart struct {
		Message string `json:"message"`
	}
	mustUnmarshal(t, restartText, &restart)
	if !strings.Contains(restart.Message, "disabled") {
		t.Fatalf("beacon_restart: expected disabled message, got %q", restart.Message)
	}
}

func extractText(t *testing.T, result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no TextContent in result")
	return ""
}

func mustUnmarshal(t *testing.T, text string, v any) {
	if err := json.Unmarshal([]byte(text), v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
