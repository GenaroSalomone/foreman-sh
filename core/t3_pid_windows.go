//go:build windows

package main

import "os"

// t3PidAlive: on Windows FindProcess opens the process, so it fails for one that is gone.
func t3PidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}
