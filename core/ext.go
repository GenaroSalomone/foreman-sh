package main

// The one place core's helpers hand a run of an outside program to a seam.
//
// In the binary extHook is nil and extRun is exec: nothing here changes what hw-core does. The in-process
// contract tests (inproc_*_test.go) set extHook to answer a READ of a program (git rev-parse, merge-base…) from a
// memo of an identical state, without a process, and to run everything else for real. A helper that routes
// through extRun states what it keeps of the run (stdout, stderr, both merged), so what a memo stores is exactly
// what the call site could have seen.

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
)

// extCall is one run of an outside program as a helper asks for it.
type extCall struct {
	Dir     string   // the working directory ("" is the process's)
	Env     []string // the child's environment (nil is the process's)
	Stdin   string
	Name    string
	Args    []string
	Merge   bool // stderr goes into stdout (`2>&1`)
	KeepErr bool // stderr is kept apart (otherwise it is dropped, `2>/dev/null`)
	Signal  bool // a killed child's status is 128+signal (bash's) rather than 128+9
}

// extResult is what the call site sees of it. RC is already the helper's status (127: it could not be run).
type extResult struct {
	Out, Err string
	RC       int
}

// extHook, when not nil, answers a call (live runs the real thing). Set only by tests.
var extHook func(c extCall, live func(extCall) extResult) extResult

func extRun(c extCall) extResult {
	if h := extHook; h != nil {
		return h(c, extExec)
	}
	return extExec(c)
}

// extExec runs the program.
func extExec(c extCall) extResult {
	cmd := exec.Command(c.Name, c.Args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}
	if c.Env != nil {
		cmd.Env = c.Env
	}
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	switch {
	case c.Merge:
		cmd.Stderr = &out
	case c.KeepErr:
		cmd.Stderr = &errb
	}
	err := cmd.Run()
	rc := exitStatus(err)
	if c.Signal {
		rc = doneExit(err)
	}
	return extResult{Out: out.String(), Err: errb.String(), RC: rc}
}

// errOf is the error a helper that returns `error` hands back for a status.
func (r extResult) errOf() error {
	if r.RC == 0 {
		return nil
	}
	return extStatusError(r.RC)
}

type extStatusError int

func (e extStatusError) Error() string { return "exit status " + itoa(int(e)) }

// writeErr gives a helper that was handed an stderr writer the stderr of the run.
func (r extResult) writeErr(w io.Writer) {
	if w != nil && r.Err != "" {
		io.WriteString(w, r.Err)
	}
}
