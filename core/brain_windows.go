//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func isWindows() bool { return true }

// realExecve: Windows has no exec; the child runs and brain ends with its status.
func realExecve(path string, argv, env []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = env, os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	os.Exit(exitStatus(err))
	return err
}

func realDetach(path string, args []string, env []string, log string) (int, error) {
	out, err := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	cmd := exec.Command(path, args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = env, out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	return pid, nil
}

func realSpawn(path string, argv []string, env []string) {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}
