package master

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"beacon/internal/config"
	"beacon/internal/util"
)

// instanceLockFile lives under BEACON_HOME and is held (flock) for the lifetime of the
// running master, so a second `beacon start` against the same home is detected instead of
// silently spawning a duplicate daemon. Because the lock is advisory and tied to the open
// file, it is released automatically when the process exits — even on SIGKILL — so there is
// no stale-PID problem.
const instanceLockFile = "master.lock"

// ErrAlreadyRunning reports that another beacon master already holds the instance lock.
type ErrAlreadyRunning struct{ PID int }

func (e ErrAlreadyRunning) Error() string {
	if e.PID > 0 {
		return fmt.Sprintf("beacon is already running (pid %d)", e.PID)
	}
	return "beacon is already running"
}

// InstanceLock is the held single-instance lock; Release drops it.
type InstanceLock struct{ f *os.File }

func instanceLockPath() (string, error) {
	dir, err := config.BeaconHomeDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, instanceLockFile), nil
}

// AcquireInstanceLock takes the exclusive single-instance lock, or returns
// ErrAlreadyRunning (carrying the holder's pid when known). The returned lock must be
// released (or the process must exit) to free it.
func AcquireInstanceLock() (*InstanceLock, error) {
	path, err := instanceLockPath()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockExclusiveNB(int(f.Fd())); err != nil {
		pid := readPIDFile(path)
		util.Close(f, "instance lock probe")
		return nil, ErrAlreadyRunning{PID: pid}
	}
	// Record our pid for diagnostics and the soft pre-check.
	if err := f.Truncate(0); err != nil {
		util.LogError(err, "truncate instance lock")
	}
	if _, err := f.Seek(0, 0); err != nil {
		util.LogError(err, "seek instance lock")
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		util.LogError(err, "write instance pid")
	}
	return &InstanceLock{f: f}, nil
}

// Release drops the lock (closing the descriptor releases the flock).
func (l *InstanceLock) Release() {
	if l != nil && l.f != nil {
		util.Close(l.f, "instance lock")
		l.f = nil
	}
}

// RunningInstancePID returns the pid of a live beacon master recorded in the lock file, or
// 0 if none. It is a soft, racy check used to print a friendly message before detaching;
// the authoritative guard is AcquireInstanceLock.
func RunningInstancePID() int {
	path, err := instanceLockPath()
	if err != nil {
		return 0
	}
	pid := readPIDFile(path)
	if pid > 0 && processAlive(pid) {
		return pid
	}
	return 0
}

func readPIDFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}
