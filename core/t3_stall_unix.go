//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// t3Detach gives the watcher its own session and no stdio: the Stop hook that starts it waits for its pipes to close, and the
// harness may signal the hook's process group when the hook returns.
func t3Detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
