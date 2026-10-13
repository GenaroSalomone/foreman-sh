//go:build windows

package main

import "os"

// pidAlive: under Git Bash `kill -0` asks the process table; os.FindProcess opens the process and fails when it is gone.
func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}
