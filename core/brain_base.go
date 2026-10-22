package main

// `hw-core brain`: bin/brain (and the parts of bin/project-spaces.sh it uses), ported. The bash stays beside it
// (HW_IMPL=bash is the way back); every message, every herdr call and its order, and every exit status are the
// bash's. This file is the floor: what a run of the command holds, the seam every outside program goes through,
// and the small readers of JSON the bash did with jq.

import (
	"bytes"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// brainCmd is one run of an outside program as bin/brain asked for it.
type brainCmd struct {
	Name  string
	Args  []string
	Dir   string
	Stdin string
	// what becomes of its output: by default stdout is captured and stderr dropped (`2>/dev/null`).
	Merge   bool     // `2>&1`: stderr goes into the captured stdout
	KeepErr bool     // stderr is captured apart
	ErrTo   bool     // stderr goes to brain's own stderr (a redirection of stdout only: `>/dev/null`)
	AllErr  bool     // stdout AND stderr go to brain's stderr (`>&2`)
	NoOut   bool     // stdout is dropped (`>/dev/null`)
	Extra   []string // variables set for this one command (`VAR=x cmd`)
}

type brainRes struct {
	Out, Err string
	RC       int
}

// brainRun is what bin/brain keeps in shell variables, and the seams a test replaces.
type brainRun struct {
	out, errw io.Writer
	getenv    func(string) string
	env       []string // the environment its children get: the process's, plus what brain exports
	stdin     io.Reader
	exec      func(b *brainRun, c brainCmd) brainRes
	sleep     func(time.Duration)
	isTTY     func() bool                                 // stdin, stdout and stderr are all terminals ([ -t 0 ] && [ -t 1 ] && [ -t 2 ])
	execve    func(path string, argv, env []string) error // `exec claude …`
	detach    func(path string, argv []string, env []string, log string) (int, error)
	spawn     func(path string, argv []string, env []string) // `nohup … &` / `( … ) &`: start and forget
	now       func() time.Time

	tab    *table
	a      *app
	brain  string // BRAIN
	binDir string // BRAIN_BIN
	home   string
	work   string

	proj, kind, space, tabLabel, dir string
	reset, noCockpit, force, self    bool
	awaitPane, resume, inPane        string
	noConversation                   bool
	engramLabel, engramSession       string
	inproc                           bool
	trap75                           bool
	verdict, paneCwd                 string
	permissions, permissionsSrc      string
	permissionsErr                   string
}

func (b *brainRun) errf(f string, a ...any) { b.a.errf(f, a...) }
func (b *brainRun) outf(f string, a ...any) { b.a.printf(f, a...) }

// exit is bash's `exit n`: it unwinds to the entry point.
func (b *brainRun) exit(rc int) { panic(exitNow{rc}) }

func (b *brainRun) env1(k string) string { return b.getenv(k) }

// setenv is `export k=v` in brain: the children that follow inherit it.
func (b *brainRun) setenv(k, v string) {
	out := b.env[:0:0]
	for _, e := range b.env {
		if !strings.HasPrefix(e, k+"=") {
			out = append(out, e)
		}
	}
	b.env = append(out, k+"="+v)
	prev := b.getenv
	b.getenv = func(name string) string {
		if name == k {
			return v
		}
		return prev(name)
	}
}

// unsetenv is `unset k`.
func (b *brainRun) unsetenv(k string) {
	out := b.env[:0:0]
	for _, e := range b.env {
		if !strings.HasPrefix(e, k+"=") {
			out = append(out, e)
		}
	}
	b.env = out
	prev := b.getenv
	b.getenv = func(name string) string {
		if name == k {
			return ""
		}
		return prev(name)
	}
}

func (b *brainRun) run(c brainCmd) brainRes { return b.exec(b, c) }

// herdr runs `herdr <args>` the way the bash call site did.
func (b *brainRun) herdr(c brainCmd, args ...string) brainRes {
	c.Name, c.Args = "herdr", args
	return b.run(c)
}

// herdrOK is `herdr … >/dev/null 2>&1`: whether it succeeded.
func (b *brainRun) herdrOK(args ...string) bool {
	return b.herdr(brainCmd{NoOut: true}, args...).RC == 0
}

// realBrainExec is the seam's default: a process.
func realBrainExec(b *brainRun, c brainCmd) brainRes {
	cmd := exec.Command(c.Name, c.Args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}
	cmd.Env = append(append([]string(nil), b.env...), c.Extra...)
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	var out, errb bytes.Buffer
	switch {
	case c.AllErr:
		cmd.Stdout, cmd.Stderr = b.errw, b.errw
	case c.NoOut:
		if c.ErrTo {
			cmd.Stderr = b.errw
		}
	default:
		cmd.Stdout = &out
		switch {
		case c.Merge:
			cmd.Stderr = &out
		case c.KeepErr:
			cmd.Stderr = &errb
		case c.ErrTo:
			cmd.Stderr = b.errw
		}
	}
	err := cmd.Run()
	return brainRes{Out: out.String(), Err: errb.String(), RC: exitStatus(err)}
}

// brTrimNL is what `$( )` does: it drops the trailing newlines.
func brTrimNL(s string) string { return strings.TrimRight(s, "\n") }

// headLines is `head -n | tr '\n' ' '`.
func headLinesSpaced(s string, n int) string {
	i, cut := 0, 0
	for ; i < n; i++ {
		j := strings.IndexByte(s[cut:], '\n')
		if j < 0 {
			cut = len(s)
			break
		}
		cut += j + 1
	}
	return strings.ReplaceAll(s[:cut], "\n", " ")
}

// ── jq, for the few shapes bin/brain asks of herdr's replies ─────────────────────────────────────────────────────

// bjGet walks object keys; nil when anything on the way is not an object or the key is missing.
func bjGet(v any, path ...string) any {
	for _, k := range path {
		m, ok := v.(jmap)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// bjParse is jq's read of a document: false when it is not JSON.
func bjParse(s string) (any, bool) { return pyLoads(s) }

// bjRaw is what `jq -r` prints for a value.
func bjRaw(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return pyStr(v)
}

// bjAlt is `a // b`: the first of the values that is neither null nor false.
func bjAlt(vs ...any) any {
	for _, v := range vs {
		if v == nil {
			continue
		}
		if b, ok := v.(bool); ok && !b {
			continue
		}
		return v
	}
	return nil
}

// bjTest is `jq -e <path> == "<want>"` on a reply's text: success only when the document says exactly that.
func bjTestEq(text string, want string, path ...string) bool {
	v, ok := bjParse(text)
	if !ok {
		return false
	}
	s, isStr := bjGet(v, path...).(string)
	return isStr && s == want
}

// bjRawOr is `jq -r '<path> // empty'`.
func bjRawOr(text string, path ...string) string {
	v, ok := bjParse(text)
	if !ok {
		return ""
	}
	r := bjGet(v, path...)
	if r == nil {
		return ""
	}
	if b, isB := r.(bool); isB && !b {
		return ""
	}
	return bjRaw(r)
}

func atoiDefault(s string, d int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return d
	}
	return n
}

func brFileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// listenLoopback is _free_port: a port the kernel just handed out on 127.0.0.1.
func listenLoopback() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), nil
}
