package main

// The launch's one way to run a program. bin/hw runs herdr, git, jq, psql, node and the agent's own binary as
// children; hw-core runs them as children too, through `sh`, so that:
//   · the stubs of the contract world (PATH) see the same argv from both implementations — the calls are part of
//     the contract (test 847 compares them), and
//   · HW_CORE_READONLY=1, the first leg of shadow, RECORDS a mutating call (`mut`) instead of doing it and answers
//     with `synth`. A read still runs: shadow's Go leg sees the world bash is about to change.

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

type cmdSpec struct {
	name  string
	args  []string
	env   []string // KEY=VALUE, added to the process environment (and d.envOver)
	dir   string   // working directory ("" = the process's)
	stdin string   // ""  = the terminal's stdin is NOT passed on (bash's `< /dev/null` is the default of the helpers)
	mut   bool     // changes the world (a pane, a worktree, a database, a file outside the fsx): never run read-only
	merge bool     // `2>&1` into the returned text (bin/hw `_capture`)
	pass  bool     // stdout and stderr go to the dispatch's own (the child prints what the user sees)
	quiet bool     // stderr discarded (`2>/dev/null`); ignored with merge
	synth string   // what a read-only run answers for a mut call (stdout), status 0
}

// sh runs one program and returns its stdout (and stderr with merge) with the status. The environment is the
// process's plus what the dispatch exported (d.envOver, as bash's `export`) plus c.env.
func (l *lch) sh(c cmdSpec) (string, int) {
	if l.readonly {
		l.fs.rec(fsOp{Op: "exec", Path: c.name, Data: t3B64([]byte(strings.Join(c.args, "\x00"))), To: c.dir})
		if c.mut {
			return c.synth, 0
		}
		// a read in a directory this very launch has only PLANNED (the worktree it would have added): there is nothing to run in
		if c.dir != "" {
			if _, err := os.Stat(c.dir); err != nil {
				return c.synth, 0
			}
		}
	}
	cmd := exec.Command(c.name, c.args...)
	cmd.Dir = c.dir
	cmd.Env = append(l.environ(), c.env...)
	if c.stdin != "" {
		cmd.Stdin = strings.NewReader(c.stdin)
	}
	var buf bytes.Buffer
	switch {
	case c.pass:
		cmd.Stdout, cmd.Stderr = l.out, l.err
	case c.merge:
		cmd.Stdout, cmd.Stderr = &buf, &buf
	case c.quiet:
		cmd.Stdout = &buf
	default:
		cmd.Stdout, cmd.Stderr = &buf, l.err
	}
	err := cmd.Run()
	return buf.String(), exitStatus(err)
}

// environ is os.Environ() with what the dispatch assigned (d.setEnv) laid over it.
func (d *dsp) environ() []string {
	if len(d.envOver) == 0 {
		return os.Environ()
	}
	out := make([]string, 0, len(os.Environ())+len(d.envOver))
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			if _, over := d.envOver[kv[:i]]; over && kv[:i] != "HW_INVOKER_PANE" {
				continue
			}
		}
		out = append(out, kv)
	}
	for k, v := range d.envOver {
		if k == "HW_INVOKER_PANE" { // bash assigns it (--report-to) without exporting it
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

// herdr is `herdr <args…>` with stdout only (`2>/dev/null` when quiet), the shape of most of bin/hw's calls.
func (l *lch) herdr(mut, quiet bool, synth string, args ...string) (string, int) {
	return l.sh(cmdSpec{name: "herdr", args: args, mut: mut, quiet: quiet, synth: synth})
}

// herdrCapture is `_capture herdr <args…>`: both streams in one text.
func (l *lch) herdrCapture(mut bool, synth string, args ...string) (string, int) {
	return l.sh(cmdSpec{name: "herdr", args: args, mut: mut, merge: true, synth: synth})
}

// git is `git -C dir <args…>`, stdout only.
func (l *lch) git(mut bool, dir string, args ...string) (string, int) {
	return l.sh(cmdSpec{name: "git", args: append([]string{"-C", dir}, args...), mut: mut, quiet: true})
}

// childCmd is exec.Command whose child inherits the dispatch's exports (see exportedEnv).
func childCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Env = childEnv()
	return c
}
