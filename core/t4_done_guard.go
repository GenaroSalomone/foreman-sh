package main

// `hw done`'s in-use guard (_done_in_use_guard, lib/hw/done.sh) with what it reads — _report_evidence and its
// explanation (bin/hw), hw-actions' done_reasons (bin/hw-actions, ported here: the same text, the same
// stranded verdict) — and the preview lease `--keep-preview` writes (_lease_seconds, _lease_write).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var doneReSessionResume = regexp.MustCompile(`"key": *"session_resume"`)

// doneWalkFiles is `find <dir> -type f`: every regular file under dir, the root not followed when it is a link.
func doneWalkFiles(dir string, fn func(string)) {
	filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.Type().IsRegular() {
			fn(p)
		}
		return nil
	})
}

// reportEvidence is _report_evidence: `marker`, `token <status> <state>`, or `none`.
func (d *doneRun) reportEvidence(pane, rundir string) string {
	if isFile(doneMarker(rundir)) {
		return "marker"
	}
	info, _ := doneTool("", nil, "herdr", "agent", "get", pane)
	info = bashSub(info)
	tok := doneJqRawEmpty(info, "result", "agent", "tokens", "done_status")
	if tok == "done" || tok == "blocked" {
		state := doneJqRawOr(info, "unknown", "result", "agent", "tokens", "done_state")
		if state == "" {
			state = "unknown"
		}
		return "token " + tok + " " + state
	}
	return "none"
}

// explainEvidence is _report_evidence_explain: the two lines owed when the tokens prove a report the marker lacks.
func (d *doneRun) explainEvidence(pane, ev, marker string) {
	if !strings.HasPrefix(ev, "token") {
		return
	}
	f := strings.Fields(ev)
	f2, f3 := "", ""
	if len(f) > 1 {
		f2 = f[1]
	}
	if len(f) > 2 {
		f3 = f[2]
	}
	d.warn("no " + marker + ", but " + pane + "'s own tokens say the executor reported: done_status=" + f2 + ", done_state=" + f3)
	d.warn("the report was made and its delivery to you failed — that is a transport fact, not an unfinished task")
}

// postReportTurns is _done_post_report_turns: disk first, the token when the run has no record; "" for none.
func (d *doneRun) postReportTurns(rundir, seq, info string) string {
	post := ""
	if readableFile(rundir + "/turns-post-report") {
		rec, _ := doneReadTrim(rundir + "/turns-post-report")
		recSeq := rec
		if i := strings.IndexByte(rec, ' '); i >= 0 {
			recSeq = rec[:i]
		}
		if recSeq == seq {
			last := rec
			if i := strings.LastIndexByte(rec, ' '); i >= 0 {
				last = rec[i+1:]
			}
			post = doneDigits(last)
		}
	} else {
		tok := doneJqRawEmpty(info, "result", "agent", "tokens", "turns_post_report")
		tokSeq := tok
		if i := strings.IndexByte(tok, ' '); i >= 0 {
			tokSeq = tok[:i]
		}
		if tok != "" && tokSeq == seq {
			last := tok
			if i := strings.LastIndexByte(tok, ' '); i >= 0 {
				last = tok[i+1:]
			}
			post = doneDigits(last)
		}
	}
	if post == "0" {
		post = ""
	}
	return post
}

var doneReStranded = regexp.MustCompile(`^token .* undelivered\n?$`) // Python's re.match: `$` also before a final newline

// doneReasons is hw-actions' done_reasons: the refusal lines, and whether the report is STRANDED.
func doneReasons(ev, status string, selfClose bool, post, seq, pane, marker string) (string, bool) {
	var reasons []string
	stranded := doneReStranded.MatchString(ev)
	if ev == "none" { // # MUTATION-ANCHOR: 861-G14
		reasons = append(reasons, "  · task "+seq+" has NOT reported: no "+marker+", and "+pane+" publishes no done token either. "+
			"Closing it now files finished-or-unfinished work as neither.")
	}
	if stranded {
		reasons = append(reasons, "  · task "+seq+" is STRANDED: "+pane+" published its report (done_state=undelivered) and the brainer never received it — no "+marker+". "+
			"Closing the pane discards the only place that report can still be redelivered from.")
	}
	if status == "working" && !selfClose { // # MUTATION-ANCHOR: 861-G15
		reasons = append(reasons, "  · "+pane+" is WORKING right now — a turn is in progress and this close would cut it mid-turn. "+
			"hw dispatched no turn into it: after a report, hw next is the only re-task route and it advances the task counter first.")
	}
	if post != "" && !selfClose { // # MUTATION-ANCHOR: 861-G16
		reasons = append(reasons, "  · "+post+" turn(s) ended in "+pane+" AFTER its report, and hw dispatched none of them. Somebody is talking to this pane.")
	}
	return strings.Join(reasons, "\n"), stranded
}

// inUseGuard is _done_in_use_guard: a pane has an owner, and hw done establishes it before closing.
func (d *doneRun) inUseGuard(proj, task, rundir string) {
	a := d.app
	if d.force { // # MUTATION-ANCHOR: 861-G17
		a.warn("--force: closing " + proj + ":" + task + " WITHOUT establishing that its pane is free")
		return
	}
	if rundir == "" {
		a.info("no hw run record for " + proj + ":" + task + ", so there is no pane to establish ownership of")
		return
	}
	pane := ""
	if readableFile(rundir + "/receipt.jsonl") {
		pane, _ = doneReceiptLast(rundir+"/receipt.jsonl", "pane")
	}
	if pane == "" {
		a.info("run " + doneBasename(rundir) + " recorded no pane receipt, so hw cannot tell whose pane this was")
		return
	}
	selfClose := false
	if hp := a.getenv("HERDR_PANE_ID"); hp != "" && pane == hp { // # MUTATION-ANCHOR: 861-G18
		selfClose = true
	}
	info, _ := doneTool("", nil, "herdr", "agent", "get", pane)
	info = bashSub(info)
	if info == "" || !doneJqE(info, "result", "agent") {
		a.warn("could NOT establish whether pane " + pane + " is free: herdr returned nothing usable for it. It may be long gone, or herdr may simply not have answered — this close cannot tell those apart, and is proceeding WITHOUT the check.")
		return
	}
	seq := doneCurrentTask(rundir)
	ev := d.reportEvidence(pane, rundir)
	status := doneJqRawEmpty(info, "result", "agent", "agent_status")
	focus := doneJqRawEmpty(info, "result", "agent", "focused")
	post := d.postReportTurns(rundir, seq, info)
	if post == "1" && status == "working" && !selfClose {
		post = ""
	}
	reasons, stranded := doneReasons(ev, status, selfClose, post, seq, pane, doneMarker(rundir))
	if reasons == "" {
		return
	}
	finishing := ev != "none" && !stranded && status == "working" && !selfClose && post == "" && focus != "true"
	if finishing { // # MUTATION-ANCHOR: 861-G19
		capS, pollS := a.getenv("HW_DONE_TURN_WAIT"), a.getenv("HW_DONE_TURN_POLL")
		if capS == "" {
			capS = "60"
		}
		if pollS == "" {
			pollS = "1"
		}
		if !doneAllDigits(capS) {
			capS = "60"
		}
		if pollS == "" || strings.Trim(pollS, "0123456789.") != "" || strings.Count(pollS, ".") > 1 {
			pollS = "1"
		}
		capN := doneAtoi(capS)
		poll, perr := strconv.ParseFloat(pollS, 64)
		t0 := doneNow()
		a.info(pane + " reported and is finishing the turn that sent it — waiting up to " + capS + "s for it to end before closing " + proj + ":" + task)
		for doneNow()-t0 < capN {
			if perr != nil {
				panic(exitNow{1}) // `sleep .` fails, and set -e ends hw done there
			}
			time.Sleep(time.Duration(poll * float64(time.Second)))
			ninfo, _ := doneTool("", nil, "herdr", "agent", "get", pane)
			ninfo = bashSub(ninfo)
			if !doneJqE(ninfo, "result", "agent") {
				a.warn("herdr stopped answering for " + pane + " while hw waited for its last turn — it may be gone; closing WITHOUT the rest of the check.")
				return
			}
			info = ninfo
			status = doneJqRawEmpty(info, "result", "agent", "agent_status")
			post = d.postReportTurns(rundir, seq, info)
			if post != "" && doneAtoi(post) > 1 {
				reasons = "  · " + post + " turn(s) ended in " + pane + " AFTER its report, and hw dispatched none of them. Somebody is talking to this pane."
				finishing = false
				break
			}
			if status != "working" {
				a.info(pane + " finished the turn that sent the report — closing " + proj + ":" + task)
				return
			}
		}
	}
	if finishing {
		a.warn("hw done REFUSED for " + proj + ":" + task + " — its pane has REPORTED and is STILL FINISHING its last turn:")
	} else {
		a.warn("hw done REFUSED for " + proj + ":" + task + " — this pane is in use:")
	}
	for _, r := range strings.Split(reasons, "\n") {
		if r != "" {
			a.warn(r)
		}
	}
	d.explainEvidence(pane, ev, doneMarker(rundir))
	if focus == "true" {
		a.warn("  · and it is FOCUSED: a human has this pane on screen right now. (Evidence only — an unfocused pane can be just as much in use, so this never decides on its own.)")
	}
	if note := d.reviveLine(proj, task, rundir); note != "" {
		a.info(note)
	}
	if finishing {
		a.die("NOT CLOSED YET — nothing was closed: the report is in and hw sees no turn after it, so this is most likely the turn that sent it ending (hw already waited HW_DONE_TURN_WAIT seconds for it and it is still working, so somebody is talking to it — hw status). Run the same command again: hw done " + proj + " " + task + "   (--force would cut that turn)")
	}
	if stranded {
		a.die("nothing was closed: the report is STRANDED. Redeliver it first (`hw outbox flush " + proj + " --to <brainer-pane>`; do NOT re-run done-invoker if its exit was 5 — the report may already be there); close it deliberately, discarding the undelivered report, with: hw done " + proj + " " + task + " --force")
	}
	a.die("nothing was closed. Establish who owns that pane — look at it — and then either let it finish, or close it deliberately with: hw done " + proj + " " + task + " --force")
}

// ── the preview lease ───────────────────────────────────────────────────────

const doneLeaseMaxH = 24

// leaseSeconds is _lease_seconds: 90m | 2h | 4 (hours) → seconds; empty is 4h.
func (d *doneRun) leaseSeconds(v string) int64 {
	orig := v
	if v == "" {
		v = "4h"
	}
	n, u := v, int64(3600)
	switch {
	case strings.HasSuffix(v, "h"):
		n = strings.TrimSuffix(v, "h")
	case strings.HasSuffix(v, "m"):
		n, u = strings.TrimSuffix(v, "m"), 60
	}
	if !doneAllDigits(n) {
		d.die("--keep-preview takes a duration like 90m, 4h or a bare number of hours (got: " + orig + ")")
	}
	if doneAtoi(n) <= 0 {
		d.die("--keep-preview needs a duration greater than zero")
	}
	secs := doneAtoi(n) * u
	if secs > doneLeaseMaxH*3600 { // # MUTATION-ANCHOR: 861-G20
		d.die(fmt.Sprintf("a preview lease may not exceed %dh (asked for %s). A preview nobody has looked at in a day is not being kept, it is being forgotten.", doneLeaseMaxH, orig))
	}
	return secs
}

// keepLease is the `--keep-preview` close: the agent pane goes, the tab, ports and database stay, under a lease.
func (d *doneRun) keepLease(proj, task, tab string) int {
	a := d.app
	leaseRun := d.newestRundir(proj, task)
	if leaseRun == "" {
		a.die("no hw run directory for " + proj + ":" + task + ", so there is nothing to write a lease against.")
	}
	secs := d.leaseSeconds(d.keepFor)
	agentPane, rc := doneReceiptLast(leaseRun+"/receipt.jsonl", "pane")
	if rc != 0 {
		panic(exitNow{rc}) // jq … | tail -1 under pipefail, unguarded: a run with no receipt ends hw done there
	}
	if agentPane != "" {
		if !d.closeObject("pane", agentPane, "agent pane "+agentPane+" closed — the executor is released", "agent pane "+agentPane+" is already gone") {
			return 1
		}
	} else {
		a.warn("no pane receipt for this run, so the agent pane could not be identified; nothing was closed")
	}
	until := doneNow() + secs
	by := a.getenv("HERDR_PANE_ID")
	if by == "" {
		by = "unknown"
	}
	untilH, ok := doneDate("-r", strconv.FormatInt(until, 10), "+%Y-%m-%dT%H:%M:%S%z")
	if !ok { // GNU date reads -r as a file: the epoch goes through -d @N there
		untilH, ok = doneDate("-d", "@"+strconv.FormatInt(until, 10), "+%Y-%m-%dT%H:%M:%S%z")
	}
	if !ok {
		untilH = strconv.FormatInt(until, 10)
	}
	at, _ := doneDate("-u", "+%Y-%m-%dT%H:%M:%SZ")
	body := "until=" + strconv.FormatInt(until, 10) + "\nuntil_h=" + untilH + "\nwhy=" + d.keepWhy + "\nby=" + by + "\nat=" + at + "\n"
	if err := os.WriteFile(leaseRun+"/lease", []byte(body), 0o666); err != nil { // # MUTATION-ANCHOR: 861-G21
		a.err.Write([]byte("hw: line 1: " + leaseRun + "/lease: " + err.Error() + "\n"))
		panic(exitNow{1})
	}
	a.ok("preview LEASED until " + doneFirstKey(leaseRun+"/lease", "until_h") + " — tab " + tab + ", database and ports kept")
	a.info("why: " + d.keepWhy)
	if p := a.getenv("HW_PORT_WEB"); p != "" {
		a.info("preview: http://127.0.0.1:" + p)
	}
	a.info("release it with: hw preview release " + proj + " " + task + "   (then hw done " + proj + " " + task + " --drop-db)")
	a.info("hw reap will refuse this worktree while the lease holds, and hw status shows it expiring")
	d.took()
	return 0
}
