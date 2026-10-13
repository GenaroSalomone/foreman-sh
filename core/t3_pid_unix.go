//go:build !windows

package main

import "syscall"

// t3PidAlive is `kill -0 <pid>`: the process exists and this one may signal it.
func t3PidAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }
