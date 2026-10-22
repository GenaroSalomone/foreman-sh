//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

// execReplace on Windows runs the program and leaves with its status: there is no exec to become.
func execReplace(path string, argv, env []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}

func accessOK(path string, mode uint32) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func kill0(pid int) (bool, string) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false, "No such process"
	}
	p.Release()
	return true, ""
}

func exitCodeOf(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 127
}
