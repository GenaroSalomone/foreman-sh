//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"syscall"
)

// execReplace is bash's `exec`: this process becomes the program. It returns only when that failed.
func execReplace(path string, argv, env []string) error { return syscall.Exec(path, argv, env) }

// accessOK is `[ -r ]`, `[ -w ]`, `[ -x ]`: the kernel's answer for this process, bits R_OK=4, W_OK=2, X_OK=1.
func accessOK(path string, mode uint32) bool { return syscall.Access(path, mode) == nil }

// kill0 is `kill -0 pid`: alive, gone, or an error whose text is the shell's.
func kill0(pid int) (alive bool, err string) {
	e := syscall.Kill(pid, 0)
	switch {
	case e == nil:
		return true, ""
	case errors.Is(e, syscall.ESRCH):
		return false, "No such process"
	case errors.Is(e, syscall.EPERM):
		return false, "Operation not permitted"
	default:
		return false, e.Error()
	}
}

func exitCodeOf(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 127
}
