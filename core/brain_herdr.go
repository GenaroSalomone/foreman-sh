package main

// What bin/brain asks of herdr, and how it refuses to read a failed call as an empty answer (herdr_json,
// herdr_list, brain_agent_get, brain_tab_pane, brain_live_agent_in, brain_reset_blockers, brain_blocker_gate).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// herdrJSON is herdr_json: the whole reply of `herdr <args>`, only when herdr answered AND the reply holds an
// array at arr. ok=false says it said so on stderr (and the caller stops: `return 1` under `set -e`).
func (b *brainRun) herdrJSON(what string, arr []string, args ...string) (string, bool) {
	r := b.herdr(brainCmd{KeepErr: true}, args...)
	raw := brTrimNL(r.Out)
	said := headLinesSpaced(r.Err+raw, 2)
	joined := strings.Join(args, " ")
	if r.RC != 0 {
		b.errf("brain: `herdr %s` failed (exit %d), so it is not known whether %s exists.\n", joined, r.RC, what)
		b.errf("brain: refusing to continue — treating a failed query as \"nothing found\" is how one\n")
		b.errf("brain: project ended up with five workspaces. herdr said: %s\n", said)
		return "", false
	}
	v, ok := bjParse(raw)
	if _, isArr := bjGet(v, arr...).([]any); !ok || !isArr {
		b.errf("brain: `herdr %s` exited 0 but its reply has no %s array, so the answer cannot be\n", joined, "."+strings.Join(arr, "."))
		b.errf("brain: read as \"none\". Refusing to continue. It said: %s\n", said)
		return "", false
	}
	return raw, true
}

// herdrList is herdr_list: the first line `pick` selects out of the array, "" when nothing matches.
func (b *brainRun) herdrList(what string, arr []string, pick func(elem jmap) (string, bool), args ...string) (string, bool) {
	raw, ok := b.herdrJSON(what, arr, args...)
	if !ok { // # MUTATION-ANCHOR: go-102-M01
		return "", false
	}
	v, _ := bjParse(raw)
	list, _ := bjGet(v, arr...).([]any)
	for _, e := range list {
		m, isMap := e.(jmap)
		if !isMap {
			continue
		}
		if s, hit := pick(m); hit {
			return strings.SplitN(s, "\n", 2)[0], true
		}
	}
	return "", true
}

// mustJSON is the call site under `set -e`: a refusal said, then the end of the command.
func (b *brainRun) mustJSON(what string, arr []string, args ...string) string {
	raw, ok := b.herdrJSON(what, arr, args...)
	if !ok {
		b.exit(1)
	}
	return raw
}

func (b *brainRun) mustList(what string, arr []string, pick func(elem jmap) (string, bool), args ...string) string {
	s, ok := b.herdrList(what, arr, pick, args...)
	if !ok {
		b.exit(1)
	}
	return s
}

// agentGet is brain_agent_get: whichever stream carried the body (stdout wins whenever it said anything).
func (b *brainRun) agentGet(pane string) string {
	r := b.herdr(brainCmd{KeepErr: true}, "agent", "get", pane)
	if out := brTrimNL(r.Out); out != "" { // # MUTATION-ANCHOR: go-153-M05
		return out
	}
	return r.Err
}

func agentNotFound(reply string) bool { return bjTestEq(reply, "agent_not_found", "error", "code") }

// tabPane is brain_tab_pane: the pane of a tab that is the brainer — "" when it is ambiguous or unreadable.
// Fields: pane id, label, cwd, agent, agent_status.
type tabPane struct{ pane, label, cwd, agent, status string }

func brainTabPane(panesJSON, tab string) (tabPane, bool) {
	v, ok := bjParse(panesJSON)
	if !ok {
		return tabPane{}, false
	}
	list, isList := bjGet(v, "result", "panes").([]any)
	if !isList {
		return tabPane{}, false
	}
	var mine, named []jmap
	for _, e := range list {
		p, isMap := e.(jmap)
		if !isMap {
			continue
		}
		if t, isStr := p["tab_id"].(string); isStr && t == tab {
			mine = append(mine, p)
			if l, isStr := p["label"].(string); isStr && l == "brain" {
				named = append(named, p)
			}
		}
	}
	var pick jmap
	switch {
	case len(named) == 1:
		pick = named[0]
	case len(mine) == 1:
		pick = mine[0]
	default:
		return tabPane{}, false
	}
	s := func(k string) string {
		if pyTruthy(pick[k]) {
			return pyStr(pick[k])
		}
		return ""
	}
	return tabPane{s("pane_id"), s("label"), s("cwd"), s("agent"), s("agent_status")}, true
}

// liveAgentIn is brain_live_agent_in: a pane whose cwd is dir (or inside it) and which herdr says carries an agent.
func liveAgentIn(panesJSON, dir string) bool {
	v, ok := bjParse(panesJSON)
	if !ok {
		return false
	}
	list, isList := bjGet(v, "result", "panes").([]any)
	if !isList {
		return false
	}
	d := strings.TrimRight(pyRealpath(dir), "/")
	for _, e := range list {
		p, isMap := e.(jmap)
		if !isMap || !pyTruthy(p["agent"]) {
			continue
		}
		cwd := "/"
		if pyTruthy(p["cwd"]) {
			cwd = pyStr(p["cwd"])
		}
		c := strings.TrimRight(pyRealpath(cwd), "/")
		if c == d || strings.HasPrefix(c, d+"/") {
			return true
		}
	}
	return false
}

// unreportedRun is one line of lane_unreported_runs: <task> <run> <invoker pane> <work directory>.
type unreportedRun struct{ task, run, invoker, wdir string }

// laneUnreportedRuns is lane_unreported_runs: a run directory with a receipt on disk and no `done` marker.
func (b *brainRun) laneUnreportedRuns(proj string) []unreportedRun {
	var out []unreportedRun
	for _, base := range b.a.workBases(proj) {
		if !dirExists(base) {
			continue
		}
		dirs, _ := filepath.Glob(brEscapeGlob(base) + "/*")
		sort.Strings(dirs)
		for _, dir := range dirs {
			if !dirExists(dir + "/.hw") {
				continue
			}
			runs, _ := filepath.Glob(brEscapeGlob(dir) + "/.hw/*")
			sort.Strings(runs)
			for _, rundir := range runs {
				if !dirExists(rundir) || !runDirName(filepath.Base(rundir)) {
					continue
				}
				if !isReadable(rundir + "/receipt.jsonl") {
					continue
				}
				if st, err := os.Stat(rundir + "/done"); err == nil && st.Mode().IsRegular() {
					continue
				}
				out = append(out, unreportedRun{filepath.Base(dir), filepath.Base(rundir), b.a.runInvokerPane(rundir), dir})
			}
		}
	}
	return out
}

// brEscapeGlob keeps a path's own metacharacters literal in filepath.Glob.
func brEscapeGlob(p string) string {
	r := strings.NewReplacer("\\", "\\\\", "*", "\\*", "?", "\\?", "[", "\\[")
	return r.Replace(p)
}

// runDirName is the case pattern 2[0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]-*.
func runDirName(n string) bool {
	if len(n) < 17 || n[0] != '2' || n[8] != '-' || n[15] != '-' {
		return false
	}
	for i := 1; i < 8; i++ {
		if n[i] < '0' || n[i] > '9' {
			return false
		}
	}
	for i := 9; i < 15; i++ {
		if n[i] < '0' || n[i] > '9' {
			return false
		}
	}
	return true
}

// resetBlockers is brain_reset_blockers: the runs still going to report to THIS pane — never reported, naming the
// pane, and with a live agent in their own work directory.
func (b *brainRun) resetBlockers(proj, pane, panesJSON string) []unreportedRun {
	var out []unreportedRun
	for _, r := range b.laneUnreportedRuns(proj) {
		if r.invoker != pane {
			continue
		}
		if !liveAgentIn(panesJSON, r.wdir) { // # MUTATION-ANCHOR: go-137-M03
			continue
		}
		out = append(out, r)
	}
	return out
}

// blockerGate is brain_blocker_gate: the executor refusal, asked by both reset paths. false = refused (said).
func (b *brainRun) blockerGate(proj, pane, flags, panesJSON string) bool {
	if !b.laneWorkBasesKnown(proj) {
		b.errf("brain: bin/project-spaces.sh has no work roots for %s, so brain cannot tell whether\n", proj)
		b.errf("brain: this lane has executors that never reported. Add it to lane_work_bases.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return false
	}
	blockers := b.resetBlockers(proj, pane, panesJSON)
	if len(blockers) > 0 && !b.force { // # MUTATION-ANCHOR: go-137-M02
		b.errf("brain: %d executor(s) of %s have not reported yet and their return address is THIS\n", len(blockers), proj)
		b.errf("brain: pane (%s):\n", pane)
		for _, r := range blockers {
			b.errf("brain:   %s  (run %s)\n", r.task, r.run)
		}
		b.errf("brain: the pane id SURVIVES a reset (measured 2026-09-14), so their reports will still\n")
		b.errf("brain: be delivered — what dies is the conversation that knows what it dispatched and\n")
		b.errf("brain: why, so each report lands on a brainer with no context for it. Read them first\n")
		b.errf("brain: (hw status), or accept that cost with: brain %s %s --force\n", proj, flags)
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return false
	}
	if len(blockers) > 0 {
		b.errf("brain: --force: resetting with %d unreported executor(s) still addressed to %s.\n", len(blockers), pane)
	}
	return true
}

// laneWorkBasesKnown is `lane_work_bases "$proj" >/dev/null 2>&1`: it fails for a lane the table does not know.
func (b *brainRun) laneWorkBasesKnown(proj string) bool { return b.tab.known(proj) }
