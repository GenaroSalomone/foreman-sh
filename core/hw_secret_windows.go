//go:build windows

package main

import (
	"os"
	"os/exec"
	"strings"
)

func hsUmask077() {}

func hsIsExec(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func hsLookPath(name string, env []string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// hsExecProcess: Windows has no exec(2), so the command runs as a child and this process ends with its status.
func hsExecProcess(argv, env []string) (int, string) {
	p, err := exec.LookPath(argv[0])
	if err != nil {
		return 127, "not found"
	}
	c := exec.Command(p, argv[1:]...)
	c.Env = env
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return 126, "cannot execute: " + strings.TrimSpace(err.Error())
	}
	os.Exit(0)
	return 0, ""
}
