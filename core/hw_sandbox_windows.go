//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
)

func sbUname() string { return "Windows_NT" }

func sbAccess(path string, mode uint32) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func sbOwnerAlive(owner string) bool {
	pid, err := strconv.Atoi(owner)
	if err != nil {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}

func sbIgnoreHupTerm() { signal.Ignore(syscall.SIGHUP, syscall.SIGTERM) }

func sbStartBroker(s *sbT, rundir, st string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	log, err := os.OpenFile(st+"/broker.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return
	}
	defer log.Close()
	cmd := exec.Command(exe, "hw-sandbox", "broker", rundir, strconv.Itoa(os.Getpid()))
	cmd.Env = append(os.Environ(), "HW_WRAPPER_ARGV0="+s.self())
	cmd.Stdout, cmd.Stderr = log, log
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}

func sbWaitChild(s *sbT, cmd *exec.Cmd) int {
	if err := cmd.Start(); err != nil {
		s.errf("%s: line %d: %s: %s\n", s.argv0, 0, cmd.Path, sbErrText(err))
		return 127
	}
	if err := cmd.Wait(); err != nil {
		return sbExitStatus(cmd.ProcessState)
	}
	return 0
}

func sbExitStatus(ps *os.ProcessState) int {
	if ps == nil {
		return 127
	}
	return ps.ExitCode()
}

func sbMkdirPrivate(d string) error    { return os.Mkdir(d, 0o700) }
func sbMkdirAllPrivate(d string) error { return os.MkdirAll(d, 0o700) }

// The outbox where the platform cannot open a directory O_NOFOLLOW and act relative to its descriptor: the pin is checked by lstat
// on the path right before each act. A window the descriptor closes, open only where no agent is sandboxed (the sandbox is
// macOS-only); the outbox is still served there, as setup/tests/322 requires.
type sbOutbox struct {
	path string
	pin  os.FileInfo
}

func sbPin(path string) (*sbOutbox, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("outbox %s is not a plain directory", path)
	}
	return &sbOutbox{path: path, pin: fi}, nil
}

func (o *sbOutbox) at(fn func(dir string, name func(string) string) error) error {
	fi, err := os.Lstat(o.path)
	if err != nil || !fi.IsDir() || !os.SameFile(fi, o.pin) {
		return fmt.Errorf("outbox %s is no longer the directory the broker started with", o.path)
	}
	return fn(o.path, func(n string) string { return filepath.Join(o.path, n) })
}

func sbUnlink(p string) {
	if fi, err := os.Lstat(p); err == nil && !fi.IsDir() {
		os.Remove(p)
	}
}

func sbRename(a, b string) error { return os.Rename(a, b) }
