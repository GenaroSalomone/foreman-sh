package main

// stall-watch in Go (bin/stall-watch): the timer for an executor that ended its turn legally and went quiet. `hw executor-turn-end`
// (t3_stall.go) starts one, detached, at each such turn end. It sleeps HW_STALL_POLL seconds at a time and ends silently when the
// wait is over (a new turn, the task reported, the pane gone); still the same turn after HW_STALL_MIN minutes it writes the
// `stalled` marker, one line in the alerts log and, with HW_INVOKER_PANE set, a `channel-send --report` to that pane. ONE alert
// per stall: the marker names the turn it alerted for. The reasons it exists are the bash header's, kept in bin/stall-watch.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var swLiveRe = regexp.MustCompile(`"state": *"live"`)

// swDigits is `tr -dc '0-9'`: every digit of the file, in order, and nothing else; "" when it cannot be read.
func swDigits(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var d []byte
	for _, c := range b {
		if c >= '0' && c <= '9' {
			d = append(d, c)
		}
	}
	return string(d)
}

// swInt is bash's `$(( x ))` for the settings: a number, or 0 for what is not one (an unset name evaluates to 0).
func swInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// swReported is `reported`: `done` in the current task's dir (the run dir, or t<N> from task 2 on) and no `reopened` beside it.
func swReported(state string) bool {
	dir := state
	switch n := swDigits(state + "/task"); n {
	case "", "0", "1":
	default:
		dir = state + "/t" + n
	}
	_, errDone := os.Stat(dir + "/done")
	_, errReopened := os.Stat(dir + "/reopened")
	return errDone == nil && errReopened != nil // MUTATION-ANCHOR: s6-sw-reopened
}

// swPaneAlive is `pane_alive`: herdr's agent get for HERDR_PANE_ID; with only an hc terminal, hc's state is live. No hc to ask is
// not "gone".
func swPaneAlive(w *wenv) bool {
	pane, hcID := w.getenv("HERDR_PANE_ID"), w.getenv("HC_TERMINAL_ID")
	if pane == "" && hcID != "" {
		if _, err := exec.LookPath("hc"); err != nil {
			return true
		}
		out, err := exec.Command("hc", "get", hcID, "--json").Output()
		return err == nil && swLiveRe.Match(out)
	}
	if pane == "" {
		return true
	}
	return exec.Command("herdr", "agent", "get", pane).Run() == nil
}

func swStamp(t time.Time) string { return t.Format("2006-01-02T15:04:05") }

// swRun is bin/stall-watch <run-state-dir> <turn-count> <what-it-waits-on>.
func swRun(w *wenv, args []string) int {
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	state, turns, what := arg(0), arg(1), arg(2)
	if what == "" {
		what = "a legal wait"
	}
	if st, err := os.Stat(state); err != nil || !st.IsDir() || turns == "" {
		return 0
	}
	dir := w.getenv("HW_STALL_DIR")
	if dir == "" {
		dir = w.getenv("HOME") + "/.cache/hw"
	}
	alerts := dir + "/stall-alerts.log"
	poll := 30.0
	if v := w.getenv("HW_STALL_POLL"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			poll = f
		}
	}
	var limit int64
	if v := w.getenv("HW_STALL_AFTER_SEC"); v != "" {
		limit = swInt(v)
	} else {
		mins := int64(20)
		if m := w.getenv("HW_STALL_MIN"); m != "" {
			mins = swInt(m)
		}
		limit = mins * 60
	}
	bin := w.ownDir()
	os.MkdirAll(dir, 0o755)
	start := time.Now()
	for {
		time.Sleep(time.Duration(poll * float64(time.Second)))
		if swDigits(state+"/turns") != turns { // MUTATION-ANCHOR: s6-sw-turn (a new turn: the wait is over)
			return 0
		}
		if swReported(state) {
			return 0
		}
		now := time.Now()
		if now.Unix()-start.Unix() < limit {
			continue
		}
		if !swPaneAlive(w) {
			return 0
		}
		// the marker is the claim: O_EXCL makes exactly one of several watchers of one stall win it. A marker of another turn is
		// stale, and so is one that cannot be read: it goes and the claim below is made again (one duplicate alert at worst,
		// never a stall left unsaid).
		marker := state + "/stalled"
		if b, err := os.ReadFile(marker); err == nil {
			f := strings.Split(strings.TrimRight(string(b), "\n"), " ")
			if len(f) < 2 || f[1] != turns { // MUTATION-ANCHOR: s6-sw-stale
				os.Remove(marker)
			}
		}
		f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // MUTATION-ANCHOR: s6-sw-claim
		if err != nil {
			return 0 // this stall was alerted
		}
		f.WriteString(strconv.FormatInt(start.Unix(), 10) + " " + turns + "\n")
		f.Close()
		mins := (now.Unix() - start.Unix()) / 60
		pane := w.getenv("HERDR_PANE_ID")
		if pane == "" {
			pane = w.getenv("HC_TERMINAL_ID")
		}
		or := func(v, d string) string {
			if v == "" {
				return d
			}
			return v
		}
		msg := "stall: " + or(w.getenv("HW_PROJECT"), "?") + ":" + or(w.getenv("HW_TASK"), "?") + " (pane " + or(pane, "?") +
			") ended its turn at " + start.Format("15:04") + ", waiting on " + what + ", and has had no new turn for " +
			strconv.FormatInt(mins, 10) + " min (HW_STALL_MIN=" + strconv.FormatInt(limit/60, 10) +
			"). `hw status` shows it STALLED. It may be legitimate (a long heavy run, `setup/heavy --status`; an ask you have not answered) or stuck (an orphaned lock, a stopped holder): look at the pane, then rule it, or `hw unstick`/`hw done` it."
		swAppend(alerts, swStamp(now)+" "+msg+"\n")
		invoker := w.getenv("HW_INVOKER_PANE")
		if invoker == "" {
			return 0
		}
		cs := w.getenv("HW_STALL_CHANNEL_SEND")
		if cs == "" {
			cs = filepath.Join(bin, "channel-send")
		}
		if err := exec.Command(cs, "--report", "herdr", invoker, "-", msg).Run(); err != nil { // MUTATION-ANCHOR: s6-sw-sendrc
			rc := 127
			if ee, ok := err.(*exec.ExitError); ok {
				rc = ee.ExitCode()
			}
			swAppend(alerts, swStamp(time.Now())+" stall-watch: the alert above did not reach pane "+invoker+" (channel-send exit "+strconv.Itoa(rc)+")\n")
		}
		return 0
	}
}

func swAppend(path, line string) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	f.WriteString(line)
	f.Close()
}
