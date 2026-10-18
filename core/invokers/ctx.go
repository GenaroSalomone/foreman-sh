package invokers

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Ctx is the process an invoker runs as: its environment, streams, clock and
// the state the bash kept in shell variables. A test builds one with a map for
// an environment and buffers for the streams; the binary builds it from os.*.
type Ctx struct {
	Prog      string // INVOKER_PROG
	BinDir    string // INVOKER_BIN_DIR: the real brain/bin
	Env       map[string]string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	StdoutTTY bool
	StdinTTY  bool
	Cwd       string
	Pid       int
	Now       func() time.Time
	Sleep     func(time.Duration)
	LockWait  time.Duration // the 120 s a waiter gives a busy holder
	Exit      func(int)     // what a signal handler ends the process with

	exitSpoken      bool
	runLock         string // INVOKER_RUN_LOCK
	envFile         string // INVOKER_ENV_FILE
	paneFromFile    bool   // INVOKER_ENV_PANE_FROM_FILE
	envRoots        string // INVOKER_ENV_ROOTS
	operatorDefault string // HW_OPERATOR as project-spaces.sh's lane table gave it
	cleanups        []func()
}

// NewCtx is the context of the running process.
func NewCtx(prog, binDir string) *Ctx {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	cwd, _ := os.Getwd()
	return &Ctx{
		Prog: prog, BinDir: binDir, Env: env,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		StdoutTTY: isTerminal(os.Stdout), StdinTTY: isTerminal(os.Stdin),
		Cwd: cwd, Pid: os.Getpid(), Now: time.Now, Sleep: time.Sleep,
		LockWait: 120 * time.Second, Exit: os.Exit,
	}
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0 && ttyDevice(f)
}

func (c *Ctx) get(k string) string { return c.Env[k] }
func (c *Ctx) has(k string) bool   { _, ok := c.Env[k]; return ok }

// or is `${k:-def}`.
func (c *Ctx) or(k, def string) string {
	if v := c.Env[k]; v != "" {
		return v
	}
	return def
}

func (c *Ctx) tmpdir() string { return c.or("TMPDIR", "/tmp") }

func (c *Ctx) operator() string {
	if v := c.get("HW_OPERATOR"); v != "" {
		return v
	}
	if c.operatorDefault != "" {
		return c.operatorDefault
	}
	return "the operator"
}

func (c *Ctx) stamp() string { return c.Now().UTC().Format("2006-01-02T15:04:05Z") }

func (c *Ctx) errf(format string, a ...any) { c.printf(c.Stderr, format, a...) }

func (c *Ctx) printf(w io.Writer, format string, a ...any) {
	if w == nil {
		return
	}
	io.WriteString(w, sprintf(format, a...))
}

func (c *Ctx) environ() []string {
	out := make([]string, 0, len(c.Env))
	for k, v := range c.Env {
		out = append(out, k+"="+v)
	}
	return out
}

// lookPath is `command -v` against the PATH this context carries.
func (c *Ctx) lookPath(name string) (string, bool) {
	if strings.Contains(name, "/") {
		st, err := os.Stat(name)
		return name, err == nil && !st.IsDir() && st.Mode()&0o111 != 0
	}
	for _, d := range filepath.SplitList(c.Env["PATH"]) {
		if d == "" {
			d = "."
		}
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, true
		}
	}
	return "", false
}

type runOpts struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	extra  []*os.File
	env    []string
}

// run starts name and waits; the status is the shell's: 127 when it cannot be
// found, 128+n when a signal ended it.
func (c *Ctx) run(o runOpts, name string, args ...string) int {
	p, ok := c.lookPath(name)
	if !ok {
		return 127
	}
	cmd := exec.Command(p, args...)
	cmd.Dir = c.Cwd
	cmd.Env = c.environ()
	if o.env != nil {
		cmd.Env = o.env
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.stdin, o.stdout, o.stderr
	cmd.ExtraFiles = o.extra
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	return 126
}

// quiet runs name with every stream discarded.
func (c *Ctx) quiet(name string, args ...string) int {
	return c.run(runOpts{}, name, args...)
}

// out is `$(name args 2>/dev/null)`: stdout without its trailing newlines.
func (c *Ctx) out(name string, args ...string) (string, int) {
	var b bytes.Buffer
	rc := c.run(runOpts{stdout: &b}, name, args...)
	return strings.TrimRight(b.String(), "\n"), rc
}

// finish is the EXIT trap: what the command registered to run when it ends,
// then the swallowed-exit-code guard. It returns rc.
func (c *Ctx) finish(rc int) int {
	for i := len(c.cleanups) - 1; i >= 0; i-- {
		c.cleanups[i]()
	}
	c.cleanups = nil
	c.releaseRunLock()
	c.exitGuard(rc)
	return rc
}

// signalExit is the INT/TERM handler of a command holding the run lock.
func (c *Ctx) signalExit(code int) {
	c.releaseRunLock()
	c.finish(code)
	c.Exit(code)
}

// exitCode turns what a step returned into the status the command ends with,
// printing a refusal the way `die` does.
func (c *Ctx) exitCode(err error) int {
	if err == nil {
		return 0
	}
	var d *Die
	if errors.As(err, &d) {
		if d.Msg != "" {
			c.errf("%s: %s\n", c.Prog, d.Msg)
		}
		return d.Code
	}
	c.errf("%s: %s\n", c.Prog, err.Error())
	return 1
}

// SignalExit is the INT/TERM handler: release the lock, run the exit path, end.
func (c *Ctx) SignalExit(code int) { c.signalExit(code) }
