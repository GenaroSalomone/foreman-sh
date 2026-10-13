package main

// The run-directory markers as bin/hw reads them in bash (`_run_done_marker`, `_run_reopened_marker`,
// `_run_blocked_marker`, `_blocked_*`, `_chaining_lease_*`, bin/hw:8894-8940, 8550-8590). statusdata.go holds the
// Python-flavoured readers `hw status` uses; these keep the shell's arithmetic and field rules, which differ at the
// edges (a task counter of `02`, a `since` that is not digits), and the verbs of train T2 are held to bash.

import (
	"os"
	"strings"
	"time"
)

// bashTaskSeq is the digits of the `task` counter file, as `tr -dc '0-9'` leaves them ("" when none).
func bashTaskSeq(rundir string) string {
	body, ok := readText(rundir + "/task")
	if !ok || !readableFile(rundir+"/task") {
		return ""
	}
	var d []byte
	for i := 0; i < len(body); i++ {
		if body[i] >= '0' && body[i] <= '9' {
			d = append(d, body[i])
		}
	}
	return string(d)
}

// runDoneMarker is _run_done_marker: the done marker of the run's CURRENT task.
func runDoneMarker(rundir string) string {
	switch seq := bashTaskSeq(rundir); seq {
	case "", "0", "1":
		return rundir + "/done"
	default:
		return rundir + "/t" + seq + "/done"
	}
}

func beside(marker, name string) string { return strings.TrimSuffix(marker, "/done") + "/" + name }

func runReopenedMarker(rundir string) string { return beside(runDoneMarker(rundir), "reopened") }
func runBlockedMarker(rundir string) string  { return beside(runDoneMarker(rundir), "blocked-waiting") }

// blockedWaitHoursBash is _blocked_wait_hours: HW_BLOCKED_WAIT_HOURS as written when it is all digits, else 24.
func blockedWaitHoursBash(getenv func(string) string) string {
	h := getenv("HW_BLOCKED_WAIT_HOURS")
	if h == "" {
		h = "24"
	}
	for i := 0; i < len(h); i++ {
		if h[i] < '0' || h[i] > '9' {
			return "24"
		}
	}
	return h
}

// blockedField is _blocked_field: the value of the first `key=` line.
func blockedField(marker, key string) string {
	body, ok := readText(marker)
	if !ok {
		return ""
	}
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, key+"=") {
			return l[len(key)+1:]
		}
	}
	return ""
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// blockedSecondsLeft is _blocked_seconds_left: seconds before an unanswered blocked executor may be closed, 0 =
// expired. A marker with no readable `since` counts from its own mtime.
func (a *app) blockedSecondsLeft(marker string) int64 {
	since := blockedField(marker, "since")
	var s int64
	if allDigits(since) {
		s, _ = pyInt64(since)
	} else if st, err := os.Stat(marker); err == nil {
		s = st.ModTime().Unix()
	} else {
		s = time.Now().Unix()
	}
	hours, _ := pyInt64(blockedWaitHoursBash(a.getenv))
	// the clock is HW_ST_NOW when it is a whole number of seconds: the shadow gives bash and Go one reading for the pair
	now := time.Now().Unix()
	if v := a.getenv("HW_ST_NOW"); allDigits(v) {
		now, _ = pyInt64(v) // # MUTATION-ANCHOR: 960-G01
	}
	left := s + hours*3600 - now
	if left < 0 {
		left = 0
	}
	return left
}

// leaseField is _chaining_lease_field: the value of the LAST `key=` line of the run's chaining lease.
func leaseField(rundir, key string) string {
	body, ok := readText(rundir + "/chaining-lease")
	if !ok {
		return ""
	}
	val := ""
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, key+"=") {
			val = l[len(key)+1:]
		}
	}
	return val
}

// leaseLive is _chaining_lease_live: state=live and `until` not yet reached.
func leaseLive(rundir string) bool {
	if leaseField(rundir, "state") != "live" {
		return false
	}
	until := leaseField(rundir, "until")
	if !allDigits(until) {
		return false
	}
	u, ok := pyInt64(until)
	return ok && time.Now().Unix() < u
}
