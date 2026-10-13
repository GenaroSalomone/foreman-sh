//go:build !windows

package main

import "syscall"

// pidAlive is `kill -0 <pid>`: the shell's answer, so a process of another user (EPERM) is not alive.
func pidAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }
