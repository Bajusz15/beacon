//go:build unix

package audit

import "syscall"

// lockFD takes an exclusive advisory lock on the open file descriptor. The lock
// is associated with the open file description and is released on unlock or when
// the descriptor is closed, so it coordinates appends both across goroutines and
// across separate Beacon processes (master + per-project monitors).
func lockFD(fd int) error { return syscall.Flock(fd, syscall.LOCK_EX) }

func unlockFD(fd int) error { return syscall.Flock(fd, syscall.LOCK_UN) }
