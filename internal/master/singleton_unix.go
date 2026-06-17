//go:build unix

package master

import "syscall"

// lockExclusiveNB takes a non-blocking exclusive advisory lock on the descriptor. It fails
// (EWOULDBLOCK) when another open file description already holds it — i.e. another running
// master. The lock is released when the descriptor is closed or the process exits.
func lockExclusiveNB(fd int) error {
	return syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
}

// processAlive reports whether a pid refers to a live process. Signal 0 performs the
// permission/existence check without delivering a signal; EPERM means it exists but is
// owned by another user.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
