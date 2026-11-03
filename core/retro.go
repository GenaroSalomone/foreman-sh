package main

// The retro reminder: `hw done` on a setup run that reported done leaves a record
// under $XDG_CACHE_HOME/hw/retro-pending (else ~/.cache/hw/retro-pending);
// `hw status` lists it until engram holds an observation with topic_key
// setup/retro/<task>, and `hw retro-done <task>` clears it by hand.
// It is a REMINDER: an unreachable engram or a missing record never fails
// `hw done` or `hw status`.

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"hwcore/internal/engramhealth"
)

const retroLane = "setup"

type retroRecord struct {
	task, at string
}

func (a *app) retroPendingDir() string {
	if x := a.getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "hw", "retro-pending")
	}
	home := a.getenv("HOME")
	if home == "" {
		home = a.getenv("USERPROFILE")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".cache", "hw", "retro-pending")
}

func retroPendingName(proj, task string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return r.Replace(proj + "__" + task)
}

// markRetroPending is the write `hw done` makes: only setup, only a run whose
// current task carries a done marker (reported done, not blocked).
func (a *app) markRetroPending(proj, task, rundir string) {
	if proj != retroLane || rundir == "" || !isFile(doneMarker(rundir)) {
		return
	}
	// done-invoker writes the done marker for a BLOCKED report too; that run owes no retro.
	if isFile(doneTaskDir(rundir)+"/blocked-closed") || isFile(doneTaskDir(rundir)+"/blocked-waiting") {
		return
	}
	dir := a.retroPendingDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return
	}
	body := "project=" + proj + "\ntask=" + task + "\nrun=" + doneBasename(rundir) + "\nat=" + strconv.FormatInt(doneNow(), 10) + "\n"
	_ = os.WriteFile(filepath.Join(dir, retroPendingName(proj, task)), []byte(body), 0o666)
}

func parseRetroRecord(path string) (retroRecord, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return retroRecord{}, false
	}
	r := retroRecord{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "task":
			r.task = v
		case "at":
			r.at = v
		}
	}
	return r, r.task != ""
}

func (a *app) retroPending() []retroRecord {
	dir := a.retroPendingDir()
	if dir == "" {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []retroRecord
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if r, ok := parseRetroRecord(filepath.Join(dir, e.Name())); ok {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at > out[j].at })
	return out
}

func (a *app) clearRetroPending(task string) int {
	dir := a.retroPendingDir()
	if dir == "" || task == "" {
		return 0
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if r, ok := parseRetroRecord(p); ok && r.task == task {
			if os.Remove(p) == nil {
				n++
			}
		}
	}
	return n
}

// engramRetroKeys asks the serve ONCE for every setup/retro key, across all
// project labels (labels are not durable: without all_projects the live serve
// answers [] and nothing ever clears). known is false when the serve did not
// answer as a search result: the reminders then stay, because "could not check"
// is not "already done".
func (a *app) engramRetroKeys() (map[string]bool, bool) {
	base := a.getenv("HW_ENGRAM_URL")
	if base == "" {
		base = "http://127.0.0.1:" + orDefaultStr(a.getenv("ENGRAM_PORT"), "7437")
	}
	budget := 2 * time.Second
	if v := a.getenv("HW_RETRO_TIMEOUT"); v != "" {
		budget = engramhealth.Budget(v)
	}
	r := engramhealth.Get(base+"/search?q="+url.QueryEscape("setup/retro")+"&limit=500&all_projects=true", budget)
	if !r.OK {
		return nil, false
	}
	var rows []struct {
		TopicKey string `json:"topic_key"`
	}
	if json.Unmarshal([]byte(r.Body), &rows) != nil {
		return nil, false
	}
	keys := map[string]bool{}
	for _, row := range rows {
		keys[row.TopicKey] = true
	}
	return keys, true
}

// retroObservationExists is the clearing condition: exactly setup/retro/<task>.
// A key that merely ends with the task name (setup/retro/windows-cierre-070 for
// cierre-070) is another task's.
func retroObservationExists(keys map[string]bool, task string) bool {
	return keys["setup/retro/"+task]
}

// cmdRetroDone is `hw retro-done <task>`: clear the reminder by hand.
func (a *app) cmdRetroDone() int {
	if len(a.args) != 1 || a.args[0] == "" || strings.HasPrefix(a.args[0], "-") {
		a.die("usage: hw retro-done <task>")
	}
	task := a.args[0]
	if n := a.clearRetroPending(task); n == 0 {
		a.info("no retro-pending record for " + task)
		return 0
	}
	a.ok("retro-pending cleared for " + task)
	return 0
}

// retroQuiet is the `hw status` section. One engram check clears the records whose
// observation exists; an unreachable engram leaves every record listed (said once)
// and never fails the status.
func (a *app) retroQuiet() {
	recs := a.retroPending()
	if len(recs) == 0 {
		return
	}
	keys, checked := a.engramRetroKeys()
	var open []retroRecord
	for _, r := range recs {
		if checked && retroObservationExists(keys, r.task) {
			a.clearRetroPending(r.task)
			continue
		}
		open = append(open, r)
	}
	if len(open) == 0 {
		return
	}
	a.printf("\n")
	a.warn(itoa(len(open)) + " setup run(s) reported done and still owe a retro — the report was accepted, the retro was not run")
	for _, r := range open {
		at := ""
		if secs, err := strconv.ParseInt(r.at, 10, 64); err == nil {
			at = ", closed " + pyStrftimeDHM(float64(secs))
		}
		a.printf("  %s!%s setup:%s%s — run the retro skill, then `hw retro-done %s` (or save it in engram with topic_key setup/retro/%s)\n",
			a.cWarn, a.c0, r.task, at, r.task, r.task)
	}
	if !checked {
		a.info("engram could not be checked (no answer or an HTTP error), so these reminders are unchanged")
	}
}
