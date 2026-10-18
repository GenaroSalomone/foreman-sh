package main

// `hw status`, the whole of it: the table (status.go) and every check bin/hw printed after it — the quiet checks
// (_release_quiet … _agent_tools_quiet, status_quiet.go) held in the order the bash replayed them. Reached as
// `hw-core status-full`, from the shim under HW_IMPL=go; `hw-core status` stays the table alone, which the shim's
// bash/shadow composition still calls.
//
// The bash starts its independent sections beside the table and prints each where it always was (lib/hw/status.sh,
// "what `hw status` runs side by side"): a section's stdout, then its stderr, then its status, whatever order they
// finish in. Here each section is a goroutine writing into its own buffers, replayed in the same order; when stdout
// and stderr are one file (a terminal) a section's two streams go to ONE buffer so the order between its `info`
// and its `warn` is kept (`_status_par_run`, MUTATION-ANCHOR 827-M03 in the bash).

import (
	"bytes"
	"io"
	"os"
	"strings"
)

// heldJob is one section of `hw status`, run beside the table with its output held for the replay.
type heldJob struct {
	out, err bytes.Buffer
	rc       int
	done     chan struct{}
	one      bool // stdout and stderr are one stream: err is empty, out holds both in the order they were written
}

// sameStream is bash's `[ /dev/fd/1 -ef /dev/fd/2 ]`.
func sameStream(out, err io.Writer) bool {
	fo, ok1 := out.(*os.File)
	fe, ok2 := err.(*os.File)
	if !ok1 || !ok2 {
		return false
	}
	so, e1 := fo.Stat()
	se, e2 := fe.Stat()
	return e1 == nil && e2 == nil && os.SameFile(so, se)
}

// startJob runs fn in its own goroutine against an app whose two streams are the job's buffers. A `die` inside it ends
// the job with its status, as the bash subshell ended its own, and not the process.
func (a *app) startJob(one bool, args []string, fn func(*app) int) *heldJob {
	j := &heldJob{done: make(chan struct{}), one: one}
	t := *a.term
	t.out = &j.out
	if one {
		t.err = &j.out // # MUTATION-ANCHOR: 826-G28
	} else {
		t.err = &j.err
	}
	b := *a
	b.term = &t
	b.args = args
	go func() {
		defer close(j.done)
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(exitNow); ok {
					j.rc = e.code
					return
				}
				panic(r)
			}
		}()
		j.rc = fn(&b)
	}()
	return j
}

// replay is `_status_par_replay`: the job finished, its stdout, then its stderr, and its status.
func (a *app) replay(j *heldJob) int {
	<-j.done
	a.term.out.Write(j.out.Bytes())
	if j.err.Len() > 0 {
		a.term.err.Write(j.err.Bytes())
	}
	return j.rc
}

// cmdStatusFull is the `status` block of bin/hw's main: its words, its sections, its order.
func (a *app) cmdStatusFull() int {
	project, all := "", false
	args := a.args
	for len(args) > 0 {
		switch {
		case args[0] == "--all":
			all = true
		case strings.HasPrefix(args[0], "-"):
			a.die("unknown option: " + args[0] + " (usage: hw status [<project>] [--all])")
		default:
			project = a.c.t.canonProject(args[0])
			if !a.c.t.known(project) {
				a.die("unknown project: " + args[0] + " (expected: " + a.laneNamesHint() + ")")
			}
		}
		args = args[1:]
	}
	one := sameStream(a.term.out, a.term.err)
	// the sections that read nothing the table writes start now
	release := a.startJob(one, nil, (*app).releaseQuiet)
	gate := a.startJob(one, nil, (*app).gateQuiet)
	outbox := a.startJob(one, nil, func(b *app) int { return b.outboxQuiet(project) })
	worktrees := a.startJob(one, []string{"--quiet"}, (*app).cmdWorktrees)
	agentTools := a.startJob(one, nil, func(b *app) int { return b.agentToolsQuiet(project) })
	var tabs, hygiene, rules, decisions *heldJob
	if project == "" {
		tabs = a.startJob(one, nil, (*app).tabsQuiet)
		hygiene = a.startJob(one, nil, (*app).hygieneQuiet)
		rules = a.startJob(one, nil, (*app).rulesQuiet)
		decisions = a.startJob(one, nil, (*app).decisionsQuiet)
	}
	rc, orphans := a.statusTable(project, all)
	if rc != 0 {
		return rc
	}
	a.orphansQuiet(orphans)
	for _, j := range []*heldJob{release, gate, outbox, agentTools} {
		if rc := a.replay(j); rc != 0 {
			return rc
		}
	}
	if project == "" {
		for _, j := range []*heldJob{worktrees, tabs, hygiene, rules, decisions} { // # MUTATION-ANCHOR: 826-G27
			if rc := a.replay(j); rc != 0 {
				return rc
			}
		}
	} else {
		if rc := a.replay(worktrees); rc != 0 {
			return rc
		}
		a.info("scoped to " + project + " — `hw status` with no project adds the cross-lane checks")
	}
	// A NEW VERB NOBODY FINDS IS A VERB NOBODY READS: the command that says what those runs REPORTED
	p := project
	if p == "" {
		p = "<project>"
	}
	a.info("what those runs actually REPORTED, from disk: hw reports " + p)
	return 0
}
