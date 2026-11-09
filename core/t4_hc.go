package main

// hw done / hw reap / occupancy for an executor that runs on herdr-core (herdr-app-done-reap, step 4).
//
// An hc run says so on disk: dispatch writes <rundir>/hc ("<project> <task> <address>"), the way a TUI run
// writes <rundir>/tab. Every hc call here is behind that record, so a TUI run makes none. The terminals a run
// owns are found by their labels (hw_project + hw_task), never by the recorded address or the name alone: a
// terminal with other labels is somebody else's and is not touched.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// hcRunRecord is the run's hc record: its project, task and dispatch address, ok=false for a TUI run.
func hcRunRecord(rundir string) (proj, task, addr string, ok bool) {
	if rundir == "" {
		return "", "", "", false
	}
	b, err := os.ReadFile(strings.TrimSuffix(rundir, "/") + "/hc")
	f := strings.Fields(string(b))
	if err != nil || len(f) != 3 {
		return "", "", "", false
	}
	return f[0], f[1], f[2], true
}

// hcOwned lists the live terminals carrying this project+task. ok=false is "hc could not answer", which is
// not "none": reading it so reports a running executor as gone.
func hcOwned(proj, task string) (addrs []string, ok bool) {
	raw, rc := hcOut(10*time.Second, "ls", "--json", "--label", "hw_project="+proj, "--label", "hw_task="+task)
	v, parsed := bjParse(brTrimNL(raw))
	list, isList := bjGet(v, "terminals").([]any)
	if !rc || !parsed || !isList {
		return nil, false
	}
	for _, e := range list {
		if m, isMap := e.(jmap); isMap && hcStr(m, "address") != "" {
			addrs = append(addrs, hcStr(m, "address"))
		}
	}
	return addrs, true
}

// hcAgentInfo is `hc get` in the shape of `herdr agent get`, so the in-use guard stays one program: the
// terminal's status, and the done/post-report tokens its turn-end hook published as labels. "" when the
// terminal is not live.
func hcAgentInfo(addr string) string {
	raw, rc := hcOut(10*time.Second, "get", addr, "--json")
	v, parsed := bjParse(brTrimNL(raw))
	m, isMap := v.(jmap)
	if !rc || !parsed || !isMap || hcStr(m, "state") != "live" {
		return ""
	}
	labels, _ := m["labels"].(jmap)
	tokens := map[string]string{}
	for tok, label := range map[string]string{"done_status": "hw_done_status", "done_state": "hw_done_state", "turns_post_report": "hw_turns_post_report"} {
		if s := hcStr(labels, label); s != "" {
			tokens[tok] = s
		}
	}
	out, err := json.Marshal(map[string]any{"result": map[string]any{"agent": map[string]any{
		"agent_status": bjRaw(bjGet(m, "status", "state")), "focused": false, "tokens": tokens}}})
	if err != nil {
		return ""
	}
	return string(out)
}

// closeHC ends the run's hc terminals: kill, then remove. It answers like closeObject: a terminal `hc ls`
// no longer lists is closed, one it still lists after kill is a failure.
func (d *doneRun) closeHC(proj, task string) bool {
	addrs, ok := hcOwned(proj, task)
	if !ok {
		d.closeResult = "failed"
		d.warn("CLOSE FAILED: `hc ls` could not answer, so whether " + proj + ":" + task + " is still running on herdr-core is UNKNOWN")
		return false
	}
	d.closeResult = "absent"
	for _, a := range addrs {
		hcOut(10*time.Second, "kill", a)
		hcOut(10*time.Second, "rm", a)
		d.closeResult = "closed"
		d.ok("terminal " + a + " closed (" + proj + ":" + task + ")")
	}
	if left, ok := hcOwned(proj, task); !ok || len(left) > 0 {
		d.closeResult = "failed"
		d.warn("CLOSE FAILED: " + proj + ":" + task + " still has a live terminal on herdr-core")
		return false
	}
	if len(addrs) == 0 {
		d.info("terminal for " + proj + ":" + task + " is already gone")
	}
	return true
}

// hcOwnedIn is the live terminals of the hc runs recorded under dir. observed=false when an hc run is
// recorded and `hc ls` could not say.
func hcOwnedIn(dir string) (addrs []string, observed bool, why string) {
	recs, _ := filepath.Glob(dir + "/.hw/*/hc")
	seen := map[string]bool{}
	for _, r := range recs {
		proj, task, _, ok := hcRunRecord(filepath.Dir(r))
		if !ok || seen[proj+"/"+task] {
			continue
		}
		seen[proj+"/"+task] = true
		got, ok := hcOwned(proj, task)
		if !ok {
			return nil, false, "hc ls could not answer for " + proj + ":" + task
		}
		addrs = append(addrs, got...)
	}
	return addrs, true, ""
}

// hcOccupants is hcOwnedIn as occupantsIn's lines.
func hcOccupants(dir string) (list string, observed bool, why string) {
	addrs, observed, why := hcOwnedIn(dir)
	for i := range addrs {
		addrs[i] += " (hc, live)"
	}
	return strings.Join(addrs, "\n"), observed, why
}

// addHCPanes makes an hc run's live terminals the live panes of its task, as herdr's pane list does for a TUI run.
// An `hc ls` that cannot answer adds nothing: status then shows what herdr says, as before.
func (d *statusData) addHCPanes() {
	for _, key := range d.taskOrder {
		t := d.tasks[key]
		if addrs, ok, _ := hcOwnedIn(t.workdir); ok {
			for _, addr := range addrs {
				d.panesByTask[key] = append(d.panesByTask[key], jmap{"pane_id": addr, "label": t.task, "cwd": t.workdir})
			}
		}
	}
}
