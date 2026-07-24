package master

import (
	"errors"
	"os"
	"testing"
)

func TestInstanceLock_ContendAndRelease(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	l1, err := AcquireInstanceLock()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// A second acquire (separate open file description) must be refused with the
	// holder's pid.
	_, err = AcquireInstanceLock()
	var running ErrAlreadyRunning
	if !errors.As(err, &running) {
		t.Fatalf("expected ErrAlreadyRunning, got %v", err)
	}
	if running.PID != os.Getpid() {
		t.Fatalf("expected holder pid %d, got %d", os.Getpid(), running.PID)
	}

	// The soft pre-check also sees a live instance.
	if got := RunningInstancePID(); got != os.Getpid() {
		t.Fatalf("RunningInstancePID = %d, want %d", got, os.Getpid())
	}

	// After release the lock is free to take again.
	l1.Release()
	l2, err := AcquireInstanceLock()
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	l2.Release()
}

func TestRunningInstancePID_NoneWhenUnlocked(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	if got := RunningInstancePID(); got != 0 {
		t.Fatalf("expected 0 with no lock file, got %d", got)
	}
}
