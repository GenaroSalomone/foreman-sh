//go:build windows

package main

import "os/exec"

// t3Detach: nothing to do; there is no session to leave, and the watcher is bash.
func t3Detach(cmd *exec.Cmd) {}
