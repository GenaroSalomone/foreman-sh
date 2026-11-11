package main

// hw-reconcile in Go (bin/hw-reconcile): report finished hw work whose push delivery never reached home. READ-ONLY: it reads run
// records and live pane state, prints a recovery queue and exits 3 when that queue is non-empty; it never prompts, types into, closes,
// restarts or otherwise changes a pane. The reasons the checks are what they are are the bash header's and the comments below.
//
// Disk supplies durable run identity, report markers, turn counters and holds; it does not supply live truth. Live candidates are
// cross-checked through the state witness (wait.go) rather than by interpreting agent_status here.
//
// What it reads from the lane table (every lane's work directory and worktree root are surveyed, and a lane has a repository when the
// table gives it a checkout) it reads as hw-core reads it. bin/holdfacts stays the one validator of a hold and is run (python3 bin/holdfacts check);
// a run's env file is read through ParseEnv, the Go port of bin/runenv, which is no longer run from here.

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	hrRunRe  = regexp.MustCompile(`^2[0-9]{7}-[0-9]{6}-[0-9]+$`)
	hrHoldRe = regexp.MustCompile(`^pending-reply-[1-9][0-9]*$`)
	hrSkip   = map[string]bool{".hw": true, ".git": true, "node_modules": true, ".venv": true, ".next": true, "dist": true, "__pycache__": true}
)

type hrHold struct {
	file, intent string
	age          int64
}

type hrCandidate struct {
	run, project, task, pane string
	turns                    int64
	holds                    []hrHold
	artifacts                int64
	countable                bool
	artDir                   string
	reportWorthy             bool
}

// hrIsDir is os.path.isdir: a link is followed.
func hrIsDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// hrRunEnv is env_fields: KEY -> value of the run's env file as bin/runenv --lenient --bare reads it. A run with no env file has no
// fields; a file runenv cannot read is named on stderr and also has none, so it is never mistaken for a run that sets nothing.
func hrRunEnv(w *wenv, run string) map[string]string {
	p := run + "/env"
	data, err := os.ReadFile(p)
	if err != nil {
		if !os.IsNotExist(err) && !errorsIsNotExist(err) {
			w.errf("hw-reconcile: cannot read %s: %s\n", p, rtStrerror(err))
		}
		return map[string]string{}
	}
	kvs, perr := ParseEnv(data, true, true)
	if perr != nil {
		w.errf("hw-reconcile: %s is not UTF-8: %s\n", p, rtUTF8Error(data))
		return map[string]string{}
	}
	m := map[string]string{}
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
	return m
}

func errorsIsNotExist(err error) bool {
	return os.IsNotExist(err) || strings.Contains(err.Error(), "no such file")
}

// hrCurrentDone is current_done: the done marker of the run's current task (the run dir, or t<N> from task 2 on).
func hrCurrentDone(run string) bool {
	task := int64(1)
	if b, err := os.ReadFile(run + "/task"); err == nil {
		if n, ok := pyInt64(string(b)); ok {
			task = n
		}
	}
	marker := run + "/done"
	if task != 1 {
		marker = run + "/t" + strconv.FormatInt(task, 10) + "/done"
	}
	st, err := os.Stat(marker)
	return err == nil && st.Mode().IsRegular()
}

// hrTurns is turns: the durable turn counter, 0 when absent or not a whole number.
func hrTurns(run string) int64 {
	b, err := os.ReadFile(run + "/turns")
	if err != nil {
		return 0
	}
	v := strings.TrimSpace(string(b))
	if v == "" || strings.Trim(v, "0123456789") != "" {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// hrReceiptFacts is receipt_facts: the pane the receipt recorded (the last one) and whether the brief's delivery was measured.
func hrReceiptFacts(run string) (string, bool) {
	b, err := os.ReadFile(run + "/receipt.jsonl")
	if err != nil {
		return "", false
	}
	pane, brief := "", false
	for _, line := range pyLines(strings.ToValidUTF8(string(b), "�")) {
		var fact map[string]any
		if json.Unmarshal([]byte(line), &fact) != nil {
			continue
		}
		if v, ok := fact["value"].(string); ok {
			switch fact["key"] {
			case "pane":
				pane = v
			case "brief_sha256":
				brief = true
			}
		}
	}
	return pane, brief
}

// hrArtifactsDir is artifacts_dir: where this run's artifacts actually are. `hw` EXPORTS HW_ARTIFACTS ALREADY RESOLVED
// (<workdir>/.artifacts on a lane with a repository, <workdir> elsewhere), so the exported value is used as it is; the derivation
// stays for the fallback only, when the run record carries none (measured 2026-09-07: appending `.artifacts` again looked in
// <workdir>/.artifacts/.artifacts and counted a worktree with three real artifacts as zero).
func hrArtifactsDir(env map[string]string, workdir, project string, repoLanes map[string]bool) string {
	if v := env["HW_ARTIFACTS"]; v != "" {
		return v
	}
	if repoLanes[project] {
		return workdir + "/.artifacts"
	}
	return workdir
}

// hrArtifactCount is artifact_count: how many artifact files are on disk, or ok false when that is not knowable. THREE VALUES, and the
// third is the point: 0 used to mean the directory holds nothing, is not there and could not be read, and report_worthy reads 0 as "no
// work". Only ENOENT is an observation of absence (a zero); every other failure is a failure to look.
func hrArtifactCount(dir string) (int64, bool) {
	if dir == "" {
		return 0, false
	}
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) { // hw creates this directory, so its genuine absence says the run never got that far
			return 0, true
		}
		return 0, false
	}
	if !st.IsDir() {
		return 0, false
	}
	var count int64
	failed := false
	var walk func(d string)
	walk = func(d string) {
		ents, err := os.ReadDir(d)
		if err != nil {
			failed = true
			return
		}
		for _, e := range ents {
			isDir := e.IsDir()
			if e.Type()&fs.ModeSymlink != 0 {
				isDir = hrIsDir(d + "/" + e.Name())
				if isDir { // os.walk lists a link to a directory among the directories and never enters it
					continue
				}
			}
			if !isDir {
				count++
			} else if !hrSkip[e.Name()] {
				walk(d + "/" + e.Name())
			}
		}
	}
	walk(dir)
	if failed {
		return 0, false
	}
	return count, true
}

// hrValidHold is valid_hold: the hold's facts when it is a valid hold, old enough, else ok false. The live pane and vendor are not
// knowable for a run that is no longer live: the hold's own pane and route stand in, so those two comparisons say nothing and every
// other rule of bin/holdfacts still applies.
func hrValidHold(w *wenv, binDir, path, runID string, now, holdSeconds int64) (hrHold, bool) {
	b, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(b) {
		return hrHold{}, false
	}
	facts := map[string]string{}
	for _, row := range hrSplitLines(string(b)) {
		i := strings.IndexByte(row, '=')
		if i < 0 {
			return hrHold{}, false
		}
		k, v := row[:i], row[i+1:]
		if _, dup := facts[k]; k == "" || dup {
			return hrHold{}, false
		}
		facts[k] = v
	}
	cmd := exec.Command("python3", binDir+"/holdfacts", "check", path, "--run", runID, "--pane", facts["pane"], "--vendor", facts["route"])
	cmd.Env = os.Environ()
	if cmd.Run() != nil {
		return hrHold{}, false
	}
	st, err := os.Stat(path)
	if err != nil {
		return hrHold{}, false
	}
	age := now - st.ModTime().Unix()
	if age < holdSeconds { // # MUTATION-ANCHOR: bpr4-holdage
		return hrHold{}, false
	}
	return hrHold{file: filepath.Base(path), intent: facts["intent"], age: age}, true
}

// hrSplitLines is str.splitlines for the separators a hold file can hold.
func hrSplitLines(s string) []string {
	f := func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == 0x1c || r == 0x1d || r == 0x1e || r == 0x85 || r == 0x2028 || r == 0x2029
	}
	var out []string
	start := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if f(rs[i]) {
			out = append(out, string(rs[start:i]))
			if rs[i] == '\r' && i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(rs) {
		out = append(out, string(rs[start:]))
	}
	return out
}

// hrCandidates is the python of bin/hw-reconcile: one candidate per work directory, from the newest run in each `.hw`.
func hrCandidates(w *wenv, binDir string, roots []string, repoLanes map[string]bool, now, holdSeconds int64) []hrCandidate {
	var out []hrCandidate
	seen := map[string]bool{}
	for _, root := range roots {
		if !hrIsDir(root) {
			continue
		}
		var walk func(base string)
		walk = func(base string) {
			if filepath.Base(base) == ".hw" {
				hrOneHW(w, binDir, base, repoLanes, now, holdSeconds, seen, &out)
				return
			}
			ents, err := os.ReadDir(base)
			if err != nil {
				return
			}
			for _, e := range ents {
				if !e.IsDir() && !(e.Type()&fs.ModeSymlink != 0 && hrIsDir(base+"/"+e.Name())) {
					continue
				}
				if e.Type()&fs.ModeSymlink != 0 { // os.walk does not follow links
					continue
				}
				if hrSkip[e.Name()] && e.Name() != ".hw" {
					continue
				}
				walk(base + "/" + e.Name())
			}
		}
		walk(root)
	}
	return out
}

// hrOneHW handles one `.hw` directory: ONE WORK DIRECTORY IS ONE TASK. Older runs are attempts superseded by the newest run;
// reporting each old attempt is how a recovered task becomes noise again after it eventually succeeds.
func hrOneHW(w *wenv, binDir, base string, repoLanes map[string]bool, now, holdSeconds int64, seen map[string]bool, out *[]hrCandidate) {
	workdir := filepath.Dir(base)
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	var names []string
	for _, e := range ents {
		if hrRunRe.MatchString(e.Name()) && hrIsDir(base+"/"+e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) == 0 {
		return
	}
	name := names[0]
	run := base + "/" + name
	ident := run
	if r, err := filepath.EvalSymlinks(run); err == nil {
		ident = r
	}
	if seen[ident] {
		return
	}
	seen[ident] = true
	env := hrRunEnv(w, run)
	project := env["HW_PROJECT"]
	if _, ok := env["HW_PROJECT"]; !ok {
		project = "unknown"
	}
	task, ok := env["HW_TASK"]
	if !ok {
		task = filepath.Base(workdir)
	}
	noReport := env["HW_NO_REPORT"] == "1"
	if hrCurrentDone(run) {
		return
	}
	artDir := hrArtifactsDir(env, workdir, project, repoLanes)
	count, countable := hrArtifactCount(artDir)
	pane, briefBacked := hrReceiptFacts(run)
	hasContract := env["HW_INVOKER_PANE"] != "" && !noReport
	var holds []hrHold
	if names, err := os.ReadDir(run); err == nil {
		for _, n := range names {
			if hrHoldRe.MatchString(n.Name()) {
				if h, ok := hrValidHold(w, binDir, run+"/"+n.Name(), name, now, holdSeconds); ok {
					holds = append(holds, h)
				}
			}
		}
	}
	// Work, not merely lifecycle noise. A repo-less task needs output on disk; a repo-lane task needs a measured brief-delivery receipt.
	// AN UNCOUNTABLE RUN IS REPORT-WORTHY, not unworthy: a run wrongly queued costs a human one look, a run wrongly dropped is
	// finished work nobody ever hears about.
	worthy := (!noReport && (!countable || count > 0)) || (hasContract && repoLanes[project] && briefBacked) // # MUTATION-ANCHOR: bpr4-brief
	c := hrCandidate{run: name, project: project, task: task, pane: pane, turns: hrTurns(run), holds: holds, artifacts: count, countable: countable, artDir: artDir, reportWorthy: worthy}
	// Runs with neither durable orphan evidence nor a pane to inspect are omitted before any live-state work is attempted.
	if c.turns > 0 || len(c.holds) > 0 || c.pane != "" {
		*out = append(*out, c)
	}
}

// hrAgentReadable is the test the live branch rests on: the reply is the record we ASKED FOR, not a truthy container. `jq -e
// '.result.agent'` is true for `{}`, so a reply with an empty or partial agent record took the live branch, found no tokens and
// suppressed the fallback built for exactly the panes it could not read (measured 2026-09-08: `herdr agent get <unknown pane>`
// exits 1 with an `agent_not_found` error and no `.result.agent`).
func hrAgentReadable(info, pane string) bool {
	if info == "" { // jq 1.6 exits 0 on EMPTY input even under -e; an empty reply is refused here rather than left to jq
		return false
	}
	vals, err := parseStream([]byte(info))
	if err != nil || len(vals) == 0 {
		return false
	}
	res, ok := vals[len(vals)-1].get("result")
	if !ok {
		return false
	}
	agent, ok := res.get("agent")
	if !ok || agent.kind != vObj {
		return false
	}
	id, _ := agent.get("pane_id")
	return id != nil && id.kind == vStr && id.s == pane // # MUTATION-ANCHOR: bpr4-readable
}

// hrRun is bin/hw-reconcile.
func hrRun(w *wenv, args []string) int {
	binDir := w.ownDir()
	brain := filepath.Dir(binDir) // the `{brain}` a lane's checkout may name
	home := w.getenv("HOME")
	file := w.getenv("HW_PROJECTS_JSON")
	if file == "" {
		file = brain + "/projects.json"
	}
	tab, errs := loadTable(file, home)
	if tab == nil {
		for _, l := range errs {
			w.errf("%s\n", l)
		}
		w.errf("hw-reconcile: the lane table did not load (the reason is above)\n")
		return 1
	}
	work := w.getenv("WORK")
	if work == "" {
		work = tab.work
	}
	c := &ctx{t: tab, brain: brain, work: work, home: home, getenv: w.getenv}
	var defaultRoots []string
	repoLanes := map[string]bool{}
	for _, lane := range tab.survey {
		root := c.lanePath(lane, "worktree_root", "")
		if root != "" && root != work+"/"+lane {
			defaultRoots = append(defaultRoots, root)
		}
		defaultRoots = append(defaultRoots, work+"/"+lane)
		if c.checkout(lane) != "" {
			repoLanes[lane] = true
		}
	}
	roots := defaultRoots
	if v := w.getenv("HW_RECONCILE_ROOTS"); v != "" {
		roots = nil
		for _, p := range strings.Split(v, "\n") {
			if p != "" {
				roots = append(roots, p)
			}
		}
	}
	holdStr := w.getenv("HW_RECONCILE_HOLD_SECONDS")
	if holdStr == "" {
		holdStr = "900"
	}
	nowStr := w.getenv("HW_RECONCILE_NOW")
	if nowStr == "" {
		nowStr = strconv.FormatInt(time.Now().Unix(), 10)
	}
	if strings.Trim(holdStr, "0123456789") != "" {
		w.errf("hw-reconcile: HW_RECONCILE_HOLD_SECONDS must be a whole number\n")
		return 1
	}
	if strings.Trim(nowStr, "0123456789") != "" {
		w.errf("hw-reconcile: HW_RECONCILE_NOW must be a Unix timestamp\n")
		return 1
	}
	holdSeconds, _ := strconv.ParseInt(holdStr, 10, 64)
	now, _ := strconv.ParseInt(nowStr, 10, 64)
	if st, err := os.Stat(binDir + "/herdr-rpc"); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		w.errf("hw-reconcile: herdr-rpc is missing beside this binary\n")
		return 1
	}
	// a validator that cannot RUN is a failure to look, not a hold that is not valid: without this every stale hold would be dropped in silence
	// and the queue would read "nothing found" (the old inline python loaded bin/holdfacts as a module, so a missing file or interpreter was fatal)
	if st, err := os.Stat(binDir + "/holdfacts"); err != nil || st.IsDir() {
		w.errf("hw-reconcile: holdfacts is missing beside this binary\n")
		return 1
	}
	if _, err := exec.LookPath("python3"); err != nil {
		w.errf("hw-reconcile: python3 is not on PATH: bin/holdfacts, which validates a hold, needs it\n")
		return 1
	}

	cands := hrCandidates(w, binDir, roots, repoLanes, now, holdSeconds)

	genv := func(k string) string {
		if k == "HW_CORE_BIN_DIR" {
			return binDir
		}
		return w.getenv(k)
	}
	a := &app{term: newTerm(w.stdout, w.stderr, false), getenv: genv, c: c}
	type finding struct{ project, task, run, pane, artifacts, reasons, evidence string }
	var findings []finding
	for _, cand := range cands {
		artifacts := strconv.FormatInt(cand.artifacts, 10)
		if !cand.countable {
			// THE THIRD VALUE KEEPS ITS NAME ON THE SURFACE A HUMAN READS, not the -1 sentinel.
			artifacts = "UNCOUNTED (" + cand.artDir + " could not be read)"
		}
		var reasons, evidence []string
		if len(cand.holds) > 0 {
			for _, h := range cand.holds {
				reasons = append(reasons, "stale-hold:"+h.file+":"+h.intent+":"+strconv.FormatInt(h.age, 10))
			}
			evidence = append(evidence, "disk hold state=delivered older than "+holdStr+"s")
		}
		info, infoOK := "", false
		if cand.pane != "" {
			if a.nextHC(cand.pane) {
				info, infoOK = a.nextAgentGet(cand.pane), true
			} else {
				info, infoOK = herdrOut(0, "agent", "get", cand.pane)
			}
		}
		if infoOK && hrAgentReadable(info, cand.pane) { // # MUTATION-ANCHOR: bpr4-live
			tok := func(k string) string { return tokenOf(info, k) }
			doneStatus, doneState, turnState := tok("done_status"), tok("done_state"), tok("turn_state")
			if doneStatus == "done" { // # MUTATION-ANCHOR: bpr4-done
				reasons = append(reasons, "done-token:"+orStr(doneState, "unknown"))
				evidence = append(evidence, "pane tokens say done_status=done while disk has no done marker")
			}
			if turnState == "ended_unreported" && doneStatus != "done" {
				wt := a.newWitness()
				wt.state(cand.pane)
				switch wt.verdict {
				case "settled", "awaiting-human":
					reasons = append(reasons, "ended-unreported:"+wt.verdict)
					evidence = append(evidence, wt.detail)
				} // live: a newer moving turn vetoes the stale turn token; unreadable state is not converted into a finding
			}
		} else if cand.turns > 0 && cand.reportWorthy { // # MUTATION-ANCHOR: bpr4-turns
			reasons = append(reasons, "ended-unreported:disk")
			evidence = append(evidence, "durable turns counter="+strconv.FormatInt(cand.turns, 10)+", no live agent readable, and no done marker")
		}
		if len(reasons) == 0 {
			continue
		}
		pane := cand.pane
		if pane == "" {
			pane = "-"
		}
		findings = append(findings, finding{cand.project, cand.task, cand.run, pane, artifacts, strings.Join(reasons, ","), strings.Join(evidence, "; ")})
	}
	if len(findings) == 0 {
		w.outf("hw-reconcile: no finished, undelivered work found\n")
		return 0
	}
	plural := "s"
	if len(findings) == 1 {
		plural = ""
	}
	w.outf("hw-reconcile: %d orphaned run%s need attention (read-only; no pane was changed)\n", len(findings), plural)
	w.outf("selection: exact done/hold evidence, or an ended turn with disk output (repo-less) / a brief-backed repo-lane task\n")
	for _, f := range findings {
		w.outf("! %s:%s  run %s  pane %s  artifacts %s\n", f.project, f.task, f.run, f.pane, f.artifacts)
		w.outf("  detected: %s\n", f.reasons)
		w.outf("  evidence: %s\n", f.evidence)
		w.outf("  action: inspect the pane/tokens, artifacts, and engram; this command deliberately takes no action\n")
	}
	w.outf("limit: report-only repo-less work saved solely to engram has no positive local work signal and is omitted\n")
	return 3
}
