//go:build !windows

package main

import "syscall"

// daemonSysProcAttr returns SysProcAttr that detaches the child from the terminal session.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
