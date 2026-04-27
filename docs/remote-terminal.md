# Remote terminal (browser)

Beacon can open a **browser-based shell** on the device, relayed through BeaconInfra. It is **not** SSH in the browser: the agent dials an outbound WebSocket, spawns a local PTY, and streams I/O.

## Flow

1. User creates a session in the dashboard (device page → **Open terminal**).
2. BeaconInfra stores a `terminal_sessions` row and enqueues a `terminal_open` command.
3. A connected Beacon agent receives the command immediately over `/api/agent/control/ws`. Older agents, or agents without the control socket connected, receive it on the next heartbeat.
4. The agent connects to the provided `wss://…/api/terminal/sessions/{id}/agent/ws?token=…` URL with the same credentials as the tunnel (`X-API-Key` + `X-Device-Name` or device token as configured).
5. A shell runs as the **same user** as the Beacon process (`$SHELL`, then `/bin/bash`, then `/bin/sh`).
6. The dashboard opens a second WebSocket to `…/browser/ws?access_token=…` and attaches xterm.js.

## Wire format

- **Binary WebSocket messages**: raw bytes to/from the PTY.
- **Text WebSocket messages**: JSON control, e.g. `{"type":"resize","cols":120,"rows":40}` (resize) from the browser path.

## Support

- Implemented on **Unix** (`internal/terminal/run_unix.go`). Other OS builds return a clear error at runtime when the command runs.

## Standard SSH over VPN

If the machine is reachable on the VPN, normal `ssh` from a client is still a separate, optional path; the browser terminal does not depend on VPN.
