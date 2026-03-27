//go:build windows

package main

import "syscall"

// daemonSysProcAttr returns SysProcAttr for Windows (no Setsid equivalent).
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
