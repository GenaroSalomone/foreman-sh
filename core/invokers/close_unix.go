//go:build !windows

package invokers

import "syscall"

func detachedAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
