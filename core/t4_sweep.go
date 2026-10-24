package main

// `hw sweep --apply` — cmd_sweep in bin/hw (and _hw_run_dirs, _close_herdr_object, _herdr_object_exists, the markers it
// reads). It closes the herdr tab or workspace of a run that REPORTED; every other run is kept, and says why. The plan half
// (no --apply) is the read of another train and stays with bash; this is the same loop, so a direct `hw-core sweep`
// prints the dry run the way bash does.
//
// KIND IS DECIDED BY WHICH RECORD EXISTS, and it decides which close runs: a `tab` record never reaches `workspace
// close` (in tab mode the workspace holds other agents' tabs, and closing it would be the 2026-08-21 incident with
// another verb). A failed close is not evidence of absence: herdr's own list says whether the object is gone.

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type sweepApplyRun struct {
	a        *app
	noSweep  map[string]bool // lanes whose `sweep` is false in the lane table
	closeWhy string          // _HERDR_CLOSE_REASON
}

func (a *app) cmdSweepApply() int {
	for _, tool := range []string{"herdr", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			a.die("missing required command: " + tool)
		}
	}
	s := &sweepApplyRun{a: a, noSweep: sweepApplyNoSweepLanes(a.c.t.file)}
	return s.run()
}

// sweepApplyNoSweepLanes reads `sweep: false` per lane, as project-spaces.sh's jq program does.
func sweepApplyNoSweepLanes(file string) map[string]bool {
	off := map[string]bool{}
	data, err := os.ReadFile(file)
	if err != nil {
		return off
	}
	vals, _ := parseStream(data)
	if len(vals) == 0 {
		return off
	}
	if lanes := field(vals[0], "lanes"); lanes != nil && lanes.kind == vObj {
		for _, m := range lanes.obj {
			if v, ok := m.v.get("sweep"); ok && v.kind == vBool && !v.b {
				off[m.k] = true
			}
		}
	}
	return off
}

var sweepApplyRunName = regexp.MustCompile(`^2[0-9]{7}-[0-9]{6}-[0-9]+$`)

// runDirs is `_hw_run_dirs`: $WORK (what fd finds there) and each lane's worktree root outside it, in survey order, except a
// lane whose `sweep` is false.
func (s *sweepApplyRun) runDirs() []string {
	a := s.a
	work := a.c.work
	var roots []string
	for _, lane := range a.c.t.survey {
		if s.noSweep[lane] { // # MUTATION-ANCHOR: 866-G01
			continue
		}
		sr := a.c.lanePath(lane, "worktree_root", "")
		if sr == "" || sr == work || strings.HasPrefix(sr, work+"/") {
			continue
		}
		roots = append(roots, sr)
	}
	listed, _ := reapApplyFd("-H", "-t", "d", `^2[0-9]{7}-[0-9]{6}-[0-9]+$`, work)
	out := fdSortedOrder(reapApplyLines(listed))
	for _, sr := range roots {
		for _, rd := range reapApplyGlob(sr + "/*/.hw/2*") {
			if isDir(rd) && sweepApplyRunName.MatchString(rd[strings.LastIndexByte(rd, '/')+1:]) {
				out = append(out, rd)
			}
		}
	}
	return out
}

// sweepApplyIDOf is `tr -d '[:space:]' < file`: the id a record holds, without any white space.
func sweepApplyIDOf(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return -1
		}
		return r
	}, string(data))
}

func sweepApplyReadable(p string) bool { // [ -r p ]
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// leaseField is _chaining_lease_field: the LAST `key=` line of the run's chaining lease.
func sweepApplyLeaseField(rundir, key string) string {
	data, err := os.ReadFile(rundir + "/chaining-lease")
	if err != nil {
		return ""
	}
	val := ""
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, key+"=") {
			val = l[len(key)+1:]
		}
	}
	return val
}

// leaseLive is _chaining_lease_live.
func sweepApplyLeaseLive(rundir string) bool {
	if sweepApplyLeaseField(rundir, "state") != "live" {
		return false
	}
	until := sweepApplyLeaseField(rundir, "until")
	if !reapApplyAllDigits(until) {
		return false
	}
	return reapApplyNow() < reapApplyAtoi(until)
}

func (s *sweepApplyRun) run() int {
	a := s.a
	apply := len(a.args) > 0 && a.args[0] == "--apply" // `[ "${1:-}" = --apply ] && apply=1`
	found, closed, failed := 0, 0, 0
	r := &reapApplyRun{a: a} // the markers and the blocked wait are reap's, spelled once
	for _, rundir := range s.runDirs() {
		if rundir == "" {
			continue
		}
		id, kind := "", ""
		if sweepApplyReadable(rundir + "/tab") {
			kind, id = "tab", sweepApplyIDOf(rundir+"/tab")
		} else if sweepApplyReadable(rundir + "/workspace") { // # MUTATION-ANCHOR: 866-G10
			kind, id = "workspace", sweepApplyIDOf(rundir+"/workspace")
		}
		if kind == "" || id == "" { // # MUTATION-ANCHOR: 866-G11
			continue
		}
		found++
		task := reapApplyBasename(reapApplyDirname(reapApplyDirname(rundir)))
		if !isFile(reapApplyDoneMarker(rundir)) { // # MUTATION-ANCHOR: 866-G02
			a.info("keeping " + kind + " " + id + " — " + task + " never reported")
			continue
		}
		// Reported, then revived: the marker says a live pane is expected and the task is open again
		if isFile(reapApplyReopenedMarker(rundir)) { // # MUTATION-ANCHOR: 866-G03
			a.info("keeping " + kind + " " + id + " — " + task + " was reopened by hw revive and has not reported since")
			continue
		}
		if mk := reapApplyBlockedMarker(rundir); isFile(mk) && r.blockedSecondsLeft(mk) > 0 { // # MUTATION-ANCHOR: 866-G04
			a.info("keeping " + kind + " " + id + " — " + task + " reported BLOCKED and is waiting for a ruling (hw ruling resumes it; hw reap closes it after " + r.blockedWaitHours() + "h)")
			continue
		}
		if sweepApplyLeaseLive(rundir) { // # MUTATION-ANCHOR: 866-G05
			a.info("keeping " + kind + " " + id + " — chaining lease until " + sweepApplyLeaseField(rundir, "until_h") + ", holder " + sweepApplyLeaseField(rundir, "holder") + ": " + sweepApplyLeaseField(rundir, "reason"))
			continue
		}
		if apply { // # MUTATION-ANCHOR: 866-G06
			if s.closeObject(kind, id, "closed "+kind+" "+id+" (reported done)", kind+" "+id+" is already gone") {
				closed++
			} else {
				failed++
			}
		} else {
			a.info("would close " + kind + " " + id + " (reported done)")
		}
	}
	if found == 0 { // # MUTATION-ANCHOR: 866-G12
		a.info("no run recorded a workspace id — nothing hw can prove it owns")
	} else if !apply {
		a.info("dry run. Add --apply to actually close them.")
	} else {
		a.ok("closed " + itoa(closed) + " of " + itoa(found))
	}
	if failed != 0 { // # MUTATION-ANCHOR: 866-G07
		return 1
	}
	return 0
}

// head3 is `printf '%s' "$out" | head -3 | tr '\n' ' '`.
func sweepApplyHead3(out string) string {
	lines := strings.Split(out, "\n")
	text := strings.Join(lines, "\n")
	if len(lines) > 3 {
		text = strings.Join(lines[:3], "\n") + "\n"
	}
	return strings.ReplaceAll(text, "\n", " ")
}

// objectExists is _herdr_object_exists: 0 the object is in herdr's list, 1 it is not, 2 herdr's list could not be read
// (closeWhy says why), 5 jq could not evaluate the list.
func (s *sweepApplyRun) objectExists(kind, id string) int {
	var array, fld string
	switch kind {
	case "tab":
		array, fld = "tabs", "tab_id"
	case "workspace":
		array, fld = "workspaces", "workspace_id"
	default:
		s.closeWhy = "unknown herdr object kind: " + kind
		return 2
	}
	h := herdrClient{getenv: s.a.getenv}
	out, rc := h.capture(kind, "list")
	out = bashSub(out)
	if rc != 0 {
		s.closeWhy = "list exited " + itoa(rc) + ": " + sweepApplyHead3(out)
		return 2
	}
	vals, perr := parseStream([]byte(out))
	notArray := func() int {
		s.closeWhy = "list returned no parseable .result." + array + " array"
		return 2
	}
	if perr != nil || len(vals) == 0 {
		return notArray()
	}
	last := vals[len(vals)-1]
	res, ok := last.get("result")
	if !ok {
		return notArray()
	}
	list, ok := res.get(array)
	if !ok || list.kind != vArr {
		return notArray()
	}
	for _, e := range list.arr { // any(.result[$a][]; .[$f] == $id)
		v, ok := e.get(fld)
		if !ok {
			return 5
		}
		if v.kind == vStr && v.s == id {
			return 0
		}
	}
	return 1
}

// closeObject is _close_herdr_object: closed, absent (herdr's own list proves it gone) or failed (it may still be running).
func (s *sweepApplyRun) closeObject(kind, id, success, absent string) bool {
	a := s.a
	h := herdrClient{getenv: a.getenv}
	out, rc := h.capture(kind, "close", id)
	if rc == 0 {
		a.ok(success)
		return true
	}
	reason := sweepApplyHead3(bashSub(out))
	if reason == "" {
		reason = "no reason returned"
	}
	s.closeWhy = ""
	switch probe := s.objectExists(kind, id); probe {
	case 0: // # MUTATION-ANCHOR: 866-G08
		a.warn("CLOSE FAILED: " + kind + " " + id + " still exists (close exit " + itoa(rc) + ": " + reason + ")")
		return false
	case 1: // # MUTATION-ANCHOR: 866-G09
		a.info(absent)
		return true
	default:
		a.warn("CLOSE FAILED: " + kind + " " + id + " could not be closed (exit " + itoa(rc) + ": " + reason + ")")
		a.warn("absence could not be verified: " + s.closeWhy)
		return false
	}
}
