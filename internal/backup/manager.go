package backup

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"beacon/internal/identity"
	"beacon/internal/logging"

	"github.com/robfig/cron/v3"
)

var logger = logging.New("backup")

// Manager owns the lifecycle of the backup scheduler.
type Manager struct {
	mu       sync.Mutex
	cfg      *identity.BackupConfig
	runner   *resticRunner
	cron     *cron.Cron
	entryID  cron.EntryID
	status   Status
	events   EventAppender
	running  bool
	cancelFn context.CancelFunc
}

// NewManager constructs a Manager. Does not start scheduling until Reconcile is called.
func NewManager(events EventAppender) *Manager {
	return &Manager{events: events}
}

// Reconcile aligns the scheduler with the current config.
func (m *Manager) Reconcile(ctx context.Context, cfg *identity.BackupConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg == nil || !cfg.Enabled {
		m.stopLocked()
		m.status.Enabled = false
		return nil
	}

	if m.cfg != nil && m.runner != nil && configEqual(m.cfg, cfg) {
		return nil
	}

	m.stopLocked()
	m.cfg = cfg
	m.runner = nil
	m.status = Status{
		Enabled:  true,
		Schedule: effectiveSchedule(cfg.Schedule),
	}

	runner, err := newResticRunner(cfg)
	if err != nil {
		m.status.LastError = err.Error()
		return fmt.Errorf("backup setup: %w", err)
	}

	schedule := effectiveSchedule(cfg.Schedule)

	c := cron.New()
	entryID, err := c.AddFunc(schedule, func() {
		if err := m.RunNow(ctx); err != nil {
			logger.Infof("Scheduled backup skipped: %v", err)
		}
	})
	if err != nil {
		m.status.LastError = err.Error()
		return fmt.Errorf("invalid backup schedule %q: %w", schedule, err)
	}

	m.runner = runner
	m.cron = c
	m.entryID = entryID
	m.status.Enabled = true
	m.status.Schedule = schedule
	m.status.LastError = ""

	c.Start()
	m.updateNextRun()
	logger.Infof("Backup scheduled: %s → %s", schedule, cfg.Destination)
	return nil
}

// RunNow triggers an immediate backup. Returns error if already running.
func (m *Manager) RunNow(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("backup already in progress")
	}
	if m.runner == nil {
		m.mu.Unlock()
		return fmt.Errorf("backup not configured")
	}
	runner := m.runner
	cfg := m.cfg
	runCtx, cancel := context.WithCancel(ctx)
	m.running = true
	m.status.Running = true
	m.cancelFn = cancel
	m.mu.Unlock()

	go m.runBackup(runCtx, cancel, runner, cfg)
	return nil
}

// Status returns a copy of the current backup status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Stop gracefully stops the scheduler and waits for any in-progress backup.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	if m.cancelFn != nil {
		m.cancelFn()
		m.cancelFn = nil
	}
	if m.cron != nil {
		m.cron.Stop()
		m.cron = nil
	}
}

func (m *Manager) runBackup(ctx context.Context, cancel context.CancelFunc, runner *resticRunner, cfg *identity.BackupConfig) {
	defer cancel()
	start := time.Now()
	err := m.executeBackup(ctx, runner, cfg)
	duration := time.Since(start)

	m.mu.Lock()
	m.running = false
	m.status.Running = false
	m.status.LastRunAt = start
	m.status.LastDurationS = duration.Seconds()

	if err != nil {
		m.status.LastSuccess = false
		m.status.LastError = err.Error()
		logger.Infof("Backup failed (%.1fs): %v", duration.Seconds(), err)
		if m.events != nil {
			m.events.AppendBackupEvent(fmt.Sprintf("backup failed: %v", err), duration.Milliseconds())
		}
	} else {
		m.status.LastSuccess = true
		m.status.LastError = ""
		logger.Infof("Backup completed (%.1fs)", duration.Seconds())
		if m.events != nil {
			m.events.AppendBackupEvent("backup completed", duration.Milliseconds())
		}
	}

	if runner != nil {
		if count, err := runner.Snapshots(ctx); err == nil {
			m.status.SnapshotCount = count
		}
	}

	m.updateNextRun()
	m.cancelFn = nil
	m.mu.Unlock()
}

func (m *Manager) executeBackup(ctx context.Context, runner *resticRunner, cfg *identity.BackupConfig) error {
	if err := runner.Init(ctx); err != nil {
		return fmt.Errorf("init repo: %w", err)
	}
	if err := runner.Backup(ctx, cfg.Paths, cfg.Exclude); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if cfg.Retention != nil {
		if err := runner.Forget(ctx, cfg.Retention); err != nil {
			return fmt.Errorf("retention: %w", err)
		}
	}
	return nil
}

func (m *Manager) updateNextRun() {
	if m.cron == nil {
		m.status.NextRunAt = time.Time{}
		return
	}
	entry := m.cron.Entry(m.entryID)
	m.status.NextRunAt = entry.Next
}

func configEqual(a, b *identity.BackupConfig) bool {
	return reflect.DeepEqual(a, b)
}

func effectiveSchedule(schedule string) string {
	if schedule == "" {
		return "0 3 * * *"
	}
	return schedule
}
