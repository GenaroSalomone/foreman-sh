package main

// hw outbox flush (bin/hw cmd_outbox; the verb of hw-core is `outbox-flush`, the shim hands it what follows `flush`): redeliver the reports that never reached a
// brainer. `outbox list` is a read verb and is not ported here. A report whose
// delivery is UNCERTAIN (channel-send exit 5) is never redelivered: it may already
// be in the brainer's session, and a second copy is the duplicate the exit exists to prevent.

import (
	"hwcore/internal/mux"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var t3ReChainingOn = regexp.MustCompile(`(?m)^HW_CHAINING_ENABLED='?1`)

var t3ReOutboxMeta = regexp.MustCompile(`/\.hw/.*/outbox/`)

// mkOutboxRoots is _outbox_roots: the directories a run's outbox can sit under.
func (t *t3) mkOutboxRoots(proj string) []string {
	var out []string
	if proj != "" {
		if isDir(t.c.work + "/" + proj) {
			out = append(out, t.c.work+"/"+proj)
		}
	} else {
		out = append(out, t.c.work)
	}
	for _, p := range t.c.t.names {
		if t.c.t.get(p, "product_repo") != "1" {
			continue
		}
		root := t.c.lanePath(p, "worktree_root", "")
		if root == "" || (proj != "" && p != proj) {
			continue
		}
		if isDir(root) {
			out = append(out, root)
		}
	}
	return out
}

// mkOutboxMetas is _outbox_metas: every `<run>/outbox/*.meta`, oldest first. The walk prunes
// what cannot lead to one (a task's source tree), as the bash does.
func (t *t3) mkOutboxMetas(proj string) []string {
	seen := map[string]bool{}
	var all []string
	for _, r := range t.mkOutboxRoots(proj) {
		var walk func(dir string, depth int, inHW bool)
		walk = func(dir string, depth int, inHW bool) {
			if depth > 7 { // find -maxdepth 8: a file in this directory is at depth+1
				return
			}
			ents, err := os.ReadDir(dir)
			if err != nil {
				return
			}
			for _, e := range ents {
				name := e.Name()
				p := dir + "/" + name
				if e.IsDir() {
					if name == "node_modules" || name == ".git" {
						continue
					}
					d := depth + 1
					nowHW := inHW || name == ".hw"
					if d >= 3 && name != ".hw" && !inHW {
						continue
					}
					walk(p, d, nowHW)
					continue
				}
				if e.Type().IsRegular() && strings.HasSuffix(name, ".meta") && t3ReOutboxMeta.MatchString(p) {
					if !seen[p] {
						seen[p] = true
						all = append(all, p)
					}
				}
			}
		}
		walk(r, 0, false)
	}
	sort.Strings(all)
	type mm struct {
		m  string
		mt int64
	}
	var ms []mm
	for _, p := range all {
		var mt int64
		if st, err := os.Stat(p); err == nil {
			mt = st.ModTime().Unix()
		}
		ms = append(ms, mm{p, mt})
	}
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].mt != ms[j].mt {
			return ms[i].mt < ms[j].mt
		}
		return ms[i].m < ms[j].m
	})
	out := make([]string, len(ms))
	for i, x := range ms {
		out[i] = x.m
	}
	return out
}

// t3OutboxField is _outbox_field: the first `key=` line's value.
func t3OutboxField(meta, key string) string {
	b, err := os.ReadFile(meta)
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, key+"=") {
			return l[len(key)+1:]
		}
	}
	return ""
}

func (t *t3) outboxFlush() int {
	args := t.args // `hw outbox flush <args>`: the shim drops the two words
	proj, to := "", ""
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--to":
			if len(args) < 2 {
				t.die("--to needs a value")
			}
			to, args = args[1], args[2:]
		case strings.HasPrefix(a, "-"):
			t.die("unknown option: " + a + " (usage: hw outbox [list|flush] [<project>] [--to <brainer-pane>])")
		default:
			proj = t.c.t.canonProject(a)
			if !t.c.t.known(proj) {
				t.die("unknown project: " + a + " (expected: " + t.laneNamesHint() + ")")
			}
			args = args[1:]
		}
	}
	if to == "" {
		t.die("hw outbox flush needs --to <brainer-pane>: a report is redelivered to a pane you name, never to a guessed one (brain <lane> passes its own)")
	}
	fails := 0
	closeBy := time.Now().Add(time.Duration(t3AtoiOr(t.getenv("HW_FLUSH_CLOSE_WAIT_MS"), 5000)) * time.Millisecond)
	for _, m := range t.mkOutboxMetas(proj) {
		key := strings.TrimSuffix(m, ".meta")
		t.fs.watch(filepath.Dir(m))
		t.fs.watch(filepath.Dir(filepath.Dir(key)))
		if !isFile(key + ".msg") {
			continue
		}
		st, p, task := t3OutboxField(m, "state"), t3OutboxField(m, "project"), t3OutboxField(m, "task")
		status, envelope := t3OutboxField(m, "status"), t3OutboxField(m, "envelope")
		if why := t.outboxForeign(m, proj); why != "" { // another lane's, another run's or another core's report is not this brainer's to be told
			t.printf("  %s:%s  skipped — %s; left where it is\n", p, task, why)
			continue
		}
		if st == "uncertain" { // # MUTATION-ANCHOR: 841-O01
			t.printf("  %s:%s  %s  uncertain — NOT redelivered: it may already be in the brainer's session (channel-send exit 5); read that pane, then remove %s\n",
				p, task, t3OutboxField(m, "status"), key+".msg")
			continue
		}
		if pid := t3OutboxField(m, "pid"); pid != "" {
			if n, err := strconv.Atoi(pid); err == nil && n > 0 && (t3PidAlive(n)) { // # MUTATION-ANCHOR: 841-O02
				t.printf("  %s:%s  still being sent by done-invoker (pid %s) — left alone\n", p, task, pid)
				continue
			}
		}
		msg := "[redelivered from the outbox: the pane this report was first sent to was gone or unreachable]\n" + mkReadSub(key+".msg")
		vendor := "herdr"
		if mux.IsHCAddress(t.getenv, to) {
			vendor = "hc"
		}
		cmd := exec.Command(t.binDir()+"/channel-send", "--report", vendor, to, "-", msg)
		rc := exitStatus(cmd.Run())
		switch rc {
		case 0:
			t.fs.remove(key + ".msg") // # MUTATION-ANCHOR: 841-O05
			t.fs.remove(m)            // # MUTATION-ANCHOR: 841-O06
			// THE REPORT REACHED THE BRAINER: say so where `hw done` and a re-run of done-invoker look.
			state := filepath.Dir(filepath.Dir(key))
			t.fs.write(state+"/done", nil, 0o666) // # MUTATION-ANCHOR: 841-O03
			// a blocked report that landed leaves the marker done-invoker writes for it: `hw ruling` resumes only from it
			if status == "blocked" {
				lines := []string{"since=" + strconv.FormatInt(time.Now().Unix(), 10), "project=" + p, "task=" + task, "envelope=" + envelope}
				if q, err := os.ReadFile(state + "/queued"); err == nil { // the report turn was already told apart: keep it so
					for _, l := range strings.Split(string(q), "\n") {
						if strings.HasPrefix(l, "report_turn_ended=") {
							lines = append(lines, l)
						}
					}
				}
				t.fs.write(state+"/blocked-waiting", t3Lines(lines...), 0o666)
			}
			t.fs.remove(state + "/reopened") // # MUTATION-ANCHOR: 841-O07
			t.fs.remove(state + "/queued")
			t.transcriptAppend(state, "REPORT", envelope, msg) // done-invoker's own line: the transcript says the report landed
			if vendor == "hc" {
				t.hcMarkDelivered(p, task, to)
			}
			t.printf("  redelivered %s:%s to %s\n", p, task, to)
			t.outboxClose(state, status, p, task, closeBy)
		case 5:
			t.fs.write(m, t3Lines(
				"envelope="+t3OutboxField(m, "envelope"), // # MUTATION-ANCHOR: 841-O10
				"project="+p,                         // # MUTATION-ANCHOR: 841-O11
				"task="+task,                         // # MUTATION-ANCHOR: 841-O12
				"run="+t3OutboxField(m, "run"),       // # MUTATION-ANCHOR: 841-O13
				"status="+t3OutboxField(m, "status"), // # MUTATION-ANCHOR: 841-O14
				"state=uncertain",                    // # MUTATION-ANCHOR: 841-O15
				"at="+t3IsoNow(),                     // # MUTATION-ANCHOR: 841-O16
			), 0o666)
			t.warn("redelivering " + p + ":" + task + " to " + to + " is UNCERTAIN (channel-send exit 5): it may already be there; held, not retried")
			fails++
		default:
			t.warn("could not redeliver " + p + ":" + task + " to " + to + " (channel-send exit " + strconv.Itoa(rc) + "); it stays in the outbox")
			fails++
		}
	}
	if fails != 0 {
		return 1
	}
	return 0
}

// hcMarkDelivered is what done-invoker's admission writes on the executor's terminal, for a report that landed late: the
// labels the cockpit reads (`hw_done_state`, `hw_invoker`) otherwise say "undelivered" to a dead brainer for ever (QA5 G1).
func (t *t3) hcMarkDelivered(proj, task, to string) {
	addrs, ok := hcOwned(proj, task)
	if !ok {
		t.warn("the report for " + proj + ":" + task + " landed, but `hc ls` could not name its terminal: the cockpit may keep saying undelivered")
		return
	}
	for _, a := range addrs {
		if _, ok := hcOut(10*time.Second, "label", a, "hw_done_state=delivered", "hw_invoker="+to); !ok {
			t.warn("the report for " + proj + ":" + task + " landed, but `hc label " + a + "` failed: the cockpit may keep saying undelivered")
		}
	}
}

// outboxForeign says why a queued report is not this flush's to deliver, "" when it is: it names another lane than the one the
// brainer serves, a run other than the directory it sits in, or a herdr-core other than this one (an address means nothing there).
// An old entry that records none of these is delivered as before.
func (t *t3) outboxForeign(meta, lane string) string {
	run := t3RunDir(filepath.Dir(filepath.Dir(strings.TrimSuffix(meta, ".meta"))))
	if p := t3OutboxField(meta, "project"); lane != "" && p != "" && p != lane {
		return "it belongs to lane " + p + ", not " + lane
	}
	if r := t3OutboxField(meta, "run"); r != "" && r != filepath.Base(run) {
		return "its meta names run " + r + " but it sits in " + filepath.Base(run)
	}
	if b, err := os.ReadFile(run + "/hc-core"); err == nil {
		if theirs, mine := strings.TrimSpace(string(b)), hcCoreID(t.getenv); theirs != "" && mine != "" && theirs != mine {
			return "its run belongs to herdr-core " + theirs + ", this one is " + mine
		}
	}
	return ""
}

// outboxClose is what done-invoker does after an admitted done report, for the one that was queued: `hw done` closes the owned
// pane. A blocked report keeps its pane (it waits for a ruling) and so does a run launched with --keep-pane.
// `hw done` runs the pinned verification first (up to 15 minutes, more behind a suite lock) and `brain <lane>` waits for this
// flush on open, so the close runs in a process of its own and is waited for only until `by` (HW_FLUSH_CLOSE_WAIT_MS, shared by the
// whole flush, 5 s): a close still running then goes on without us, its output in close.log beside the done marker.
func (t *t3) outboxClose(state, status, proj, task string, by time.Time) {
	if status == "blocked" {
		return
	}
	if env, err := os.ReadFile(t3RunDir(state) + "/env"); err == nil && t3ReChainingOn.Match(env) {
		return
	}
	logf, err := os.CreateTemp(state, "close.log.")
	if err != nil {
		t.warn("the report for " + proj + ":" + task + " landed, but its close could not be started (" + err.Error() + "): `hw done " + proj + " " + task + "` closes the pane")
		return
	}
	cmd := exec.Command(t.binDir()+"/hw", "done", proj, task)
	cmd.Stdout, cmd.Stderr = logf, logf
	t3Detach(cmd)
	if err := cmd.Start(); err != nil {
		logf.Close()
		os.Remove(logf.Name())
		t.warn("the report for " + proj + ":" + task + " landed, but `hw done` did not start (" + err.Error() + "): close the pane by hand")
		return
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err := <-waited:
		logf.Close()
		if err == nil {
			os.Remove(logf.Name())
			return
		}
		os.Rename(logf.Name(), state+"/close.log")
		t.warn("the report for " + proj + ":" + task + " landed, but `hw done " + proj + " " + task + "` failed (" + err.Error() + "; " + state + "/close.log): close the pane by hand")
	case <-time.After(time.Until(by)):
		logf.Close() // the child keeps its own descriptor; the name is what we move
		os.Rename(logf.Name(), state+"/close.log")
		t.transcriptAppend(state, "CLOSE-PENDING", "-", "hw done "+proj+" "+task+" was still running when the flush ended; its outcome is in close.log")
		t.printf("  closing %s:%s in the background (hw done verifies first); outcome in %s/close.log\n", proj, task, state)
	}
}

// t3RunDir is the run directory of a state directory: a chained task keeps its state in `<run>/t<N>`, and the run's own files
// (env, hc-core) stay in the run directory itself.
func t3RunDir(state string) string {
	if b := filepath.Base(state); len(b) > 1 && b[0] == 't' && strings.Trim(b[1:], "0123456789") == "" {
		return filepath.Dir(state)
	}
	return state
}

func t3AtoiOr(v string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
		return n
	}
	return def
}
