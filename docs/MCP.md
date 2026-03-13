# Beacon MCP Server

The Beacon MCP (Model Context Protocol) server exposes Beacon projects, status, logs, and actions as tools for AI assistants like Claude Desktop and Cursor.

## Requirements

| Item               | Required?                                                          |
| ------------------ | ------------------------------------------------------------------ |
| **beacon** binary  | Yes — build with `go build -o beacon ./cmd/beacon` or `make build` |
| **Cursor**         | Optional — if using Cursor with MCP                                |
| **Claude Desktop** | Optional — if using Claude with MCP                                |
| **jq**             | Optional — only if scripts use it for JSON checks                  |

## Transports

- **stdio** — Recommended for local Cursor/Claude Desktop. Uses stdin/stdout.
- **http** — For remote access. `beacon mcp serve --transport http --listen 127.0.0.1:7766`

## Claude Desktop

**Config file**:
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux**: `~/.config/Claude/claude_desktop_config.json`

Edit via Claude menu → Settings → Developer → Edit Config.

**Add Beacon** (stdio — recommended for local):

```json
{
  "mcpServers": {
    "beacon": {
      "command": "/absolute/path/to/beacon",
      "args": ["mcp", "serve", "--transport", "stdio"]
    }
  }
}
```

Replace `/absolute/path/to/beacon` with the full path to your beacon binary (e.g. `$(which beacon)`).

Restart Claude Desktop fully (quit and reopen). Beacon tools (e.g. `beacon_inventory`) should appear. Try: "List my Beacon projects" or "What's the status of my Beacon projects?"

## Cursor

**Config**: `~/.cursor/mcp.json` or Cursor Settings → MCP.

**Example** (stdio):

```json
{
  "mcpServers": {
    "beacon": {
      "command": "/absolute/path/to/beacon",
      "args": ["mcp", "serve", "--transport", "stdio"]
    }
  }
}
```

Path must be absolute. Restart Cursor if needed, then use the tools in chat.

## ChatGPT

ChatGPT (web/app) does not support local MCP servers or stdio in the same way as Claude Desktop or Cursor. For local MCP, use **Claude Desktop** or **Cursor**.

## Testing

**Unit tests**:

```bash
go test ./internal/mcp/...
```

**E2E test** (requires beacon in PATH or builds it):

```bash
./scripts/test-e2e-mcp.sh
```

The E2E script builds beacon, starts the MCP server on HTTP, runs a client that calls `beacon_inventory`, and verifies the response.

## Tools

| Tool              | Description                                |
| ----------------- | ------------------------------------------ |
| `beacon_inventory` | List all Beacon projects                   |
| `beacon_status`    | Get check status for a project             |
| `beacon_logs`      | Tail logs for a project                    |
| `beacon_diff`      | Git diff between refs                      |
| `beacon_deploy`    | Deploy a project (with confirmation)       |
| `beacon_restart`   | Restart deploy/monitor service             |
