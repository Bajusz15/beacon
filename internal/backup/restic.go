package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"beacon/internal/identity"
)

type resticRunner struct {
	bin          string
	repo         string
	passwordFile string
	env          []string
}

func newResticRunner(cfg *identity.BackupConfig) (*resticRunner, error) {
	if cfg == nil {
		return nil, fmt.Errorf("backup config is required")
	}
	if len(cfg.Paths) == 0 {
		return nil, fmt.Errorf("at least one backup path is required")
	}
	for _, p := range cfg.Paths {
		if strings.TrimSpace(p) == "" {
			return nil, fmt.Errorf("backup paths cannot contain empty values")
		}
	}
	if strings.TrimSpace(cfg.Destination) == "" {
		return nil, fmt.Errorf("backup destination is required")
	}
	pwFile := expandHome(cfg.PasswordFile)
	if pwFile == "" {
		return nil, fmt.Errorf("backup password_file is required")
	}
	bin, err := exec.LookPath("restic")
	if err != nil {
		return nil, fmt.Errorf("restic not found in PATH: %w", err)
	}

	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	return &resticRunner{
		bin:          bin,
		repo:         cfg.Destination,
		passwordFile: pwFile,
		env:          env,
	}, nil
}

func (r *resticRunner) baseArgs() []string {
	return []string{"--repo", r.repo, "--password-file", r.passwordFile}
}

func (r *resticRunner) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = r.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("restic %s: %w\nstderr: %s", args[0], err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// Init initializes the restic repo if it doesn't exist. Idempotent.
func (r *resticRunner) Init(ctx context.Context) error {
	args := append(r.baseArgs(), "snapshots", "--json")
	_, err := r.run(ctx, args...)
	if err == nil {
		return nil
	}
	initArgs := append(r.baseArgs(), "init")
	_, err = r.run(ctx, initArgs...)
	if err != nil {
		return fmt.Errorf("restic init: %w", err)
	}
	logger.Infof("Initialized restic repo at %s", r.repo)
	return nil
}

// Backup runs restic backup with the given paths and exclude patterns.
func (r *resticRunner) Backup(ctx context.Context, paths []string, excludes []string) error {
	args := r.baseArgs()
	args = append(args, "backup")
	for _, e := range excludes {
		args = append(args, "--exclude", e)
	}
	for _, p := range paths {
		args = append(args, expandHome(p))
	}
	_, err := r.run(ctx, args...)
	return err
}

// Forget applies the retention policy and prunes unreferenced data.
func (r *resticRunner) Forget(ctx context.Context, ret *identity.RetentionPolicy) error {
	if ret == nil {
		return nil
	}
	args := r.baseArgs()
	args = append(args, "forget", "--prune")
	if ret.KeepDaily > 0 {
		args = append(args, "--keep-daily", fmt.Sprintf("%d", ret.KeepDaily))
	}
	if ret.KeepWeekly > 0 {
		args = append(args, "--keep-weekly", fmt.Sprintf("%d", ret.KeepWeekly))
	}
	if ret.KeepMonthly > 0 {
		args = append(args, "--keep-monthly", fmt.Sprintf("%d", ret.KeepMonthly))
	}
	_, err := r.run(ctx, args...)
	return err
}

// Snapshots returns the number of snapshots in the repo.
func (r *resticRunner) Snapshots(ctx context.Context) (int, error) {
	args := append(r.baseArgs(), "snapshots", "--json")
	out, err := r.run(ctx, args...)
	if err != nil {
		return 0, err
	}
	var snaps []json.RawMessage
	if err := json.Unmarshal(out, &snaps); err != nil {
		return 0, err
	}
	return len(snaps), nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + path[1:]
	}
	return path
}
