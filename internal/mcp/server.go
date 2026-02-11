package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"beacon/internal/config"
	"beacon/internal/version"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// InventoryInput has no args
type InventoryInput struct{}

// StatusInput for beacon_status
type StatusInput struct {
	Project string `json:"project,omitempty" jsonschema:"description=Project name (optional, omit for all)"`
}

// LogsInput for beacon_logs
type LogsInput struct {
	Project string `json:"project" jsonschema:"description=Project name,required"`
	Since   string `json:"since,omitempty" jsonschema:"description=E.g. 1h, 24h"`
	Grep    string `json:"grep,omitempty" jsonschema:"description=Filter lines containing this string"`
}

// DiffInput for beacon_diff
type DiffInput struct {
	Project string `json:"project" jsonschema:"description=Project name,required"`
	From    string `json:"from" jsonschema:"description=Git ref (tag/branch/commit),required"`
	To      string `json:"to" jsonschema:"description=Git ref,required"`
}

// DeployInput for beacon_deploy
type DeployInput struct {
	Project            string `json:"project" jsonschema:"description=Project name,required"`
	Ref                string `json:"ref,omitempty" jsonschema:"description=Git tag or ref to deploy"`
	ConfirmationToken  string `json:"confirmation_token,omitempty" jsonschema:"description=Token from previous call to confirm"`
}

// RestartInput for beacon_restart
type RestartInput struct {
	Project            string `json:"project" jsonschema:"description=Project name,required"`
	Service            string `json:"service,omitempty" jsonschema:"description=deploy or monitor (default: deploy)"`
	ConfirmationToken  string `json:"confirmation_token,omitempty" jsonschema:"description=Token from previous call to confirm"`
}

// ServeOptions configures the MCP server transport
type ServeOptions struct {
	Transport string // "stdio" or "http"
	Listen    string // for http: e.g. "127.0.0.1:7766"
	TokenEnv  string // env var name for bearer token (e.g. "BEACON_MCP_TOKEN")
}

// RunServe starts the MCP server with the given options
func RunServe(ctx context.Context, opts ServeOptions) error {
	if opts.Transport == "" {
		opts.Transport = "stdio"
	}
	switch opts.Transport {
	case "stdio":
		return Run(ctx)
	case "http":
		return runHTTP(ctx, opts)
	default:
		return fmt.Errorf("unsupported transport %q (use stdio or http)", opts.Transport)
	}
}

// Run starts the MCP server over stdio and blocks until the client disconnects
func Run(ctx context.Context) error {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirectories(); err != nil {
		return err
	}

	cfg := LoadConfig(filepath.Join(paths.BaseDir, "mcp.yml"))
	backend := &ToolBackend{
		Paths:     paths,
		Config:    cfg,
		Confirm:   NewConfirmationTokenStore(),
		RateLimit: NewToolRateLimiter(10 * time.Second),
	}

	auditPath := cfg.GetAuditLogPath()
	if auditPath != "" {
		_ = os.MkdirAll(filepath.Dir(auditPath), 0755)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "beacon",
		Version: version.GetVersion(),
	}, nil)

	audit := func(tool string, args map[string]any, err error) {
		if auditPath == "" {
			return
		}
		entry := map[string]any{
			"ts":    time.Now().Format(time.RFC3339),
			"tool":  tool,
			"args":  redactArgs(args),
			"error": err != nil,
		}
		if err != nil {
			entry["error_msg"] = err.Error()
		}
		data, _ := json.Marshal(entry)
		f, _ := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			f.Write(append(data, '\n'))
			f.Close()
		}
	}

	wrap := func(tool string, fn func() (any, error)) (any, error) {
		out, err := fn()
		audit(tool, nil, err)
		return out, err
	}

	registerTools(server, cfg, backend, wrap)
	return server.Run(ctx, &mcp.StdioTransport{})
}

func runHTTP(ctx context.Context, opts ServeOptions) error {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirectories(); err != nil {
		return err
	}

	cfg := LoadConfig(filepath.Join(paths.BaseDir, "mcp.yml"))
	backend := &ToolBackend{
		Paths:     paths,
		Config:    cfg,
		Confirm:   NewConfirmationTokenStore(),
		RateLimit: NewToolRateLimiter(10 * time.Second),
	}

	auditPath := cfg.GetAuditLogPath()
	if auditPath != "" {
		_ = os.MkdirAll(filepath.Dir(auditPath), 0755)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "beacon",
		Version: version.GetVersion(),
	}, nil)

	audit := func(tool string, args map[string]any, err error) {
		if auditPath == "" {
			return
		}
		entry := map[string]any{
			"ts":    time.Now().Format(time.RFC3339),
			"tool":  tool,
			"args":  redactArgs(args),
			"error": err != nil,
		}
		if err != nil {
			entry["error_msg"] = err.Error()
		}
		data, _ := json.Marshal(entry)
		f, _ := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			f.Write(append(data, '\n'))
			f.Close()
		}
	}

	wrap := func(tool string, fn func() (any, error)) (any, error) {
		out, err := fn()
		audit(tool, nil, err)
		return out, err
	}

	registerTools(server, cfg, backend, wrap)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	var handler http.Handler = mcpHandler
	if opts.TokenEnv != "" {
		expectToken := os.Getenv(opts.TokenEnv)
		if expectToken == "" {
			return fmt.Errorf("token-env %q is set but %s is empty; set the env var or omit --token-env", opts.TokenEnv, opts.TokenEnv)
		}
		verifier := auth.TokenVerifier(func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
			if token != expectToken {
				return nil, auth.ErrInvalidToken
			}
			return &auth.TokenInfo{}, nil
		})
		handler = auth.RequireBearerToken(verifier, nil)(mcpHandler)
	}

	listen := opts.Listen
	if listen == "" {
		listen = "127.0.0.1:7766"
	}

	srv := &http.Server{Addr: listen, Handler: handler}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	log.Printf("MCP server listening on http://%s", listen)
	if opts.TokenEnv != "" {
		log.Printf("Bearer token required (from %s)", opts.TokenEnv)
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func registerTools(server *mcp.Server, cfg *Config, backend *ToolBackend, wrap func(string, func() (any, error)) (any, error)) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "beacon_inventory",
		Description: "List all Beacon projects (name, location, config_dir)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ InventoryInput) (*mcp.CallToolResult, InventoryOutput, error) {
		out, err := wrap("beacon_inventory", func() (any, error) {
			if !cfg.IsToolAllowed("beacon_inventory") {
				return nil, errToolNotAllowed
			}
			return backend.ToolInventory()
		})
		if err != nil {
			return nil, InventoryOutput{}, err
		}
		return nil, out.(InventoryOutput), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "beacon_status",
		Description: "Get check health status for a project (or all projects)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StatusInput) (*mcp.CallToolResult, StatusOutput, error) {
		out, err := wrap("beacon_status", func() (any, error) {
			if !cfg.IsToolAllowed("beacon_status") {
				return nil, errToolNotAllowed
			}
			return backend.ToolStatus(in.Project)
		})
		if err != nil {
			return nil, StatusOutput{}, err
		}
		return nil, out.(StatusOutput), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "beacon_logs",
		Description: "Get recent logs for a project",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in LogsInput) (*mcp.CallToolResult, LogsOutput, error) {
		out, err := wrap("beacon_logs", func() (any, error) {
			if !cfg.IsToolAllowed("beacon_logs") {
				return nil, errToolNotAllowed
			}
			return backend.ToolLogs(in.Project, in.Since, in.Grep)
		})
		if err != nil {
			return nil, LogsOutput{}, err
		}
		return nil, out.(LogsOutput), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "beacon_diff",
		Description: "Show git diff between two refs for a project",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in DiffInput) (*mcp.CallToolResult, DiffOutput, error) {
		out, err := wrap("beacon_diff", func() (any, error) {
			if !cfg.IsToolAllowed("beacon_diff") {
				return nil, errToolNotAllowed
			}
			return backend.ToolDiff(in.Project, in.From, in.To)
		})
		if err != nil {
			return nil, DiffOutput{}, err
		}
		return nil, out.(DiffOutput), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "beacon_deploy",
		Description: "Trigger deploy (gated; requires BEACON_MCP_DEPLOY_ENABLED=1 and confirmation)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in DeployInput) (*mcp.CallToolResult, DeployOutput, error) {
		out, err := wrap("beacon_deploy", func() (any, error) {
			if !cfg.IsToolAllowed("beacon_deploy") {
				return nil, errToolNotAllowed
			}
			return backend.ToolDeploy(in.Project, in.Ref, in.ConfirmationToken)
		})
		if err != nil {
			return nil, DeployOutput{}, err
		}
		return nil, out.(DeployOutput), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "beacon_restart",
		Description: "Restart deploy or monitor service (gated; requires BEACON_MCP_RESTART_ENABLED=1 and confirmation)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RestartInput) (*mcp.CallToolResult, RestartOutput, error) {
		out, err := wrap("beacon_restart", func() (any, error) {
			if !cfg.IsToolAllowed("beacon_restart") {
				return nil, errToolNotAllowed
			}
			return backend.ToolRestart(in.Project, in.Service, in.ConfirmationToken)
		})
		if err != nil {
			return nil, RestartOutput{}, err
		}
		return nil, out.(RestartOutput), nil
	})
}

var errToolNotAllowed = fmt.Errorf("tool not allowed by configuration")

func redactArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any)
	for k, v := range args {
		if k == "confirmation_token" {
			out[k] = "[redacted]"
		} else {
			out[k] = v
		}
	}
	return out
}
