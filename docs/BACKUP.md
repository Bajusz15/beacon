# Backup

Beacon can run scheduled [restic](https://restic.net) backups of configured directories. Backups run inside the master agent process (`beacon start`) and are configured in `~/.beacon/config.yaml`.

## Prerequisites

- **restic** must be installed and available in `$PATH`.
- A password file for the restic repository (e.g. `~/.beacon/restic-password`).

## Configuration

Add a `backup:` block to `~/.beacon/config.yaml`:

```yaml
backup:
  enabled: true
  schedule: "0 3 * * *"          # cron expression (default: daily at 03:00)
  paths:
    - ~/
    - /etc/
  exclude:
    - "*.tmp"
    - node_modules
    - .cache
  destination: "/mnt/backup/beacon"
  password_file: ~/.beacon/restic-password
  retention:
    keep_daily: 7
    keep_weekly: 4
    keep_monthly: 6
  env:                            # extra environment variables passed to restic
    RESTIC_COMPRESSION: "auto"
```

### Fields

| Field | Required | Description |
|---|---|---|
| `enabled` | yes | Enable or disable the backup scheduler. |
| `schedule` | no | Cron expression. Defaults to `0 3 * * *` (daily at 03:00). |
| `paths` | yes | List of directories to back up. `~/` is expanded to the user's home directory. |
| `exclude` | no | Glob patterns passed to `restic backup --exclude`. |
| `destination` | yes | Restic repository path (local path or any restic-supported URL). |
| `password_file` | yes | Path to a file containing the restic repository password. |
| `retention` | no | Retention policy applied via `restic forget --prune` after each backup. |
| `retention.keep_daily` | no | Number of daily snapshots to keep. |
| `retention.keep_weekly` | no | Number of weekly snapshots to keep. |
| `retention.keep_monthly` | no | Number of monthly snapshots to keep. |
| `env` | no | Key-value map of extra environment variables passed to restic. |

## Usage

### Manual backup

Trigger a backup immediately (requires `beacon start` to be running):

```sh
beacon backup
```

### Check status

```sh
beacon backup status
```

Displays: enabled state, schedule, whether a backup is running, last run time/result/duration, next scheduled run, and snapshot count.

### How it works

1. When `beacon start` launches, the backup manager reads the `backup:` config and starts a cron scheduler.
2. At each scheduled tick (or on `beacon backup`), the manager:
   - Initializes the restic repository if it doesn't exist yet (`restic init`).
   - Runs `restic backup` with the configured paths and exclusions.
   - Applies the retention policy with `restic forget --prune` (if configured).
   - Counts remaining snapshots.
3. Only one backup can run at a time — concurrent triggers return a "backup already in progress" error.
4. Backup events (success/failure with duration) are logged to the master event log.
5. Config changes are picked up via hot-reload — the scheduler is reconciled without restarting the master.

### API

The master exposes a local HTTP endpoint for the CLI:

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/backup/run` | Trigger a manual backup. Returns `200` (started), `409` (already running), or `400` (not configured). |

The status endpoint (`beacon backup status`) reads from the master's `/api/status` snapshot.
