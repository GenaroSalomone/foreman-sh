//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func sbUname() string {
	out, err := exec.Command("uname", "-s").Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// sbAccess is `[ -r ]`, `[ -x ]`: the kernel's answer for this process (R_OK=4, X_OK=1).
func sbAccess(path string, mode uint32) bool { return syscall.Access(path, mode) == nil }

// sbOwnerAlive is `kill -0 "$owner"`: only a signal the kernel accepts counts, so a process of another user (EPERM) is gone.
func sbOwnerAlive(owner string) bool {
	pid, err := strconv.Atoi(owner)
	if err != nil {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// sbIgnoreHupTerm is `trap ” HUP TERM`: ignored here and, inherited, in the invoker the broker runs.
func sbIgnoreHupTerm() { signal.Ignore(syscall.SIGHUP, syscall.SIGTERM) }

// sbStartBroker is the python `os.setsid(); os.execv(SELF, broker rundir $$)` of the bash, on /dev/null and appending to broker.log.
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}

// sbWaitChild runs the agent as bash runs a foreground command: the terminal's ^C reaches both, the status is the child's, and a
// child that died of SIGINT or SIGQUIT is a status of 128+n rather than the end of this process first.
func sbWaitChild(s *sbT, cmd *exec.Cmd) int {
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(sig)
	if err := cmd.Start(); err != nil {
		s.errf("%s: line %d: %s: %s\n", s.argv0, 0, cmd.Path, sbErrText(err))
		return 127
	}
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	return sbExitStatus(cmd.ProcessState)
}

func sbExitStatus(ps *os.ProcessState) int {
	if ps == nil {
		return 127
	}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal()) // MUTATION-ANCHOR: s8-exitsig
	}
	return ps.ExitCode()
}

// sbMkdirPrivate is `( umask 077; mkdir d )`.
func sbMkdirPrivate(d string) error {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	return os.Mkdir(d, 0o777)
}

func sbMkdirAllPrivate(d string) error {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	return os.MkdirAll(d, 0o777)
}

// ── the outbox, pinned by identity and acted on relative to its descriptor ───

type sbOutbox struct {
	path     string
	dev, ino uint64
}

var sbCwdMu sync.Mutex // fchdir moves the whole process: one act at a time

// sbPin records the outbox's device and inode — before the agent runs.
func sbPin(path string) (*sbOutbox, error) {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return nil, fmt.Errorf("%s: %s", path, sbErrText(err))
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return nil, fmt.Errorf("outbox %s is not a plain directory", path)
	}
	return &sbOutbox{path: path, dev: uint64(st.Dev), ino: uint64(st.Ino)}, nil
}

// at opens the outbox O_NOFOLLOW, checks it is still the directory the broker started with, and runs fn with it as the working
// directory, so every bare name fn uses is resolved inside it — never through the path above it, which the agent can swap.
func (o *sbOutbox) at(fn func(dir string, name func(string) string) error) error {
	sbCwdMu.Lock()
	defer sbCwdMu.Unlock()
	fd, err := syscall.Open(o.path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return err
	}
	if uint64(st.Dev) != o.dev || uint64(st.Ino) != o.ino { // MUTATION-ANCHOR: s8-pin
		return fmt.Errorf("outbox %s is no longer the directory the broker started with", o.path)
	}
	back, err := syscall.Open(".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(back)
	if err := syscall.Fchdir(fd); err != nil {
		return err
	}
	defer syscall.Fchdir(back)
	return fn(".", func(n string) string { return n })
}

func sbUnlink(p string) { _ = syscall.Unlink(p) } // never rmdir: a directory planted as a name stays one

func sbRename(a, b string) error { return syscall.Rename(a, b) }
