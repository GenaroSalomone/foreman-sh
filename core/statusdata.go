package main

// What `hw status` reads off the disk about a run: the readers of lib/hw/status.sh
// (_status_runs_source and _status_recovery_source), one Go function for each
// Python one, in the same order, answering the same on every file shape the
// Python answers on. The docstrings of the Python say WHY each fact is read the
// way it is; they are not copied here, the names are the pointers.

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type closedInfo struct {
	at       float64
	hasAt    bool
	by       string
	reported bool
	legacy   bool
}

type leaseInfo struct {
	vals          map[string]string
	live, expired bool
}

// runRec is one entry of runs_of(): a run directory with what status reads of it.
type runRec struct {
	name, dir     string
	legacy        bool
	done          bool
	task          int
	reopened      bool
	closed        *closedInfo
	priorReported int // 0 is None
	blockedSince  int
	hasBlocked    bool
	resumed       bool
	started       float64
	hasStarted    bool
	here          bool
	mode          string
	turns         int
	turnsAt       float64
	hasTurnsAt    bool
	invoker       string
	noReport      bool
	hasEnv        bool
	execPane      string
	modelRunning  string
	modelVerdict  string
	lease         *leaseInfo
	ownTab, ownWs string
}

func (r *runRec) blockedTruthy() bool { return r != nil && r.hasBlocked && r.blockedSince != 0 }

// statusReader holds what the readers need: the clock, and where a runenv
// diagnostic goes.
type statusReader struct {
	stderr func(string)
	now    func() float64
}

func wallNow() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func readText(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// runIsHere is run_is_here: the shape of the run, from its env file.
func runIsHere(rundir string) bool {
	body, ok := readText(filepath.Join(rundir, "env"))
	if !ok {
		return false
	}
	lines := strings.Split(body, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "HW_LAUNCH_MODE=") {
			return strings.Contains(l, "here")
		}
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "HW_INVOKER_PANE=") {
			return true
		}
	}
	return false
}

// runOwns is run_owns: the tab or workspace this run created.
func runOwns(rundir, kind string) string {
	body, ok := readText(filepath.Join(rundir, kind))
	if !ok {
		return ""
	}
	return pyStrip(body)
}

// taskSeq is task_seq: which task the executor is on.
func taskSeq(rundir string) int {
	body, ok := readText(filepath.Join(rundir, "task"))
	if !ok {
		return 1
	}
	var digits []byte
	for i := 0; i < len(body); i++ {
		if body[i] >= '0' && body[i] <= '9' {
			digits = append(digits, body[i])
		}
	}
	if len(digits) == 0 {
		return 1
	}
	n, ok := pyInt(string(digits))
	if !ok || n <= 0 {
		return 1
	}
	return n
}

func taskStateDir(rundir string) string {
	n := taskSeq(rundir)
	if n == 1 {
		return rundir
	}
	return filepath.Join(rundir, "t"+itoa(n))
}

func itoa(n int) string { return strconv.Itoa(n) }

func taskReopened(rundir string) bool {
	d := taskStateDir(rundir)
	return pyIsFile(filepath.Join(d, "done")) && pyIsFile(filepath.Join(d, "reopened"))
}

func taskDone(rundir string) bool {
	d := taskStateDir(rundir)
	return pyIsFile(filepath.Join(d, "done")) && !pyIsFile(filepath.Join(d, "reopened"))
}

// closedByHand is closed_by_hand: the closed-by-hand mark of the current task,
// or the verify_run line a close left in receipt.jsonl before the mark existed.
func closedByHand(rundir string) *closedInfo {
	if body, ok := readText(filepath.Join(taskStateDir(rundir), "closed-by-hand")); ok {
		kv := map[string]string{}
		for _, l := range pySplitlinesKeepEnds(body) {
			l = pyStrip(l)
			if i := strings.IndexByte(l, '='); i >= 0 {
				kv[l[:i]] = l[i+1:]
			}
		}
		atS := kv["at"]
		at := 0
		bad := false
		if atS != "" {
			n, ok := pyInt(atS)
			if !ok {
				bad = true
			}
			at = n
		}
		if !bad {
			by := kv["by"]
			if by == "" {
				by = "none"
			}
			ci := &closedInfo{by: by, reported: kv["reported"] == "yes"}
			if at != 0 {
				ci.at, ci.hasAt = float64(at), true
			}
			return ci
		}
	}
	floor := 0.0
	if taskSeq(rundir) > 1 {
		if m, ok := pyMtime(filepath.Join(rundir, "task")); ok {
			floor = m
		} else {
			return nil
		}
	}
	body, ok := readText(filepath.Join(rundir, "receipt.jsonl"))
	if !ok {
		return nil
	}
	found := false
	var at float64
	for _, line := range pySplitlinesKeepEnds(body) {
		v, ok := pyLoads(line)
		if !ok {
			continue
		}
		m, isMap := v.(jmap)
		if !isMap {
			continue
		}
		if k, _ := m["key"].(string); k != "verify_run" {
			continue
		}
		atStr, _ := m["at"].(string)
		tm, err := time.Parse("2006-01-02T15:04:05Z", atStr)
		if err != nil {
			continue
		}
		t := float64(tm.Unix())
		if t >= floor { // # MUTATION-ANCHOR: 826-G05
			at, found = t, true
		}
	}
	if found {
		return &closedInfo{at: at, hasAt: true, by: "unknown", legacy: true}
	}
	return nil
}

// pySplitlinesKeepEnds is the line iteration of `for line in fh`: split after
// every \n only (a file opened in text mode also folds \r\n and \r to \n).
func pySplitlinesKeepEnds(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

// priorReported is prior_reported: the last task before the current one with a done marker.
func priorReported(rundir string) int {
	for k := taskSeq(rundir) - 1; k > 0; k-- {
		d := rundir
		if k != 1 {
			d = filepath.Join(rundir, "t"+itoa(k))
		}
		if pyIsFile(filepath.Join(d, "done")) {
			return k
		}
	}
	return 0
}

// blockedSince is blocked_since: when the --blocked report landed, if the executor is still kept waiting.
func blockedSince(rundir string) (int, bool) {
	p := filepath.Join(taskStateDir(rundir), "blocked-waiting")
	if !pyIsFile(p) {
		return 0, false
	}
	if body, ok := readText(p); ok {
		for _, l := range pySplitlinesKeepEnds(body) {
			if strings.HasPrefix(l, "since=") {
				if n, ok := pyInt(l[len("since="):]); ok {
					return n, true
				}
				break // a ValueError ends the try: the mtime answers
			}
		}
	}
	if m, ok := pyMtime(p); ok {
		return int(m), true
	}
	return 0, false
}

func blockedWaitHours(getenv func(string) string) int {
	h := getenv("HW_BLOCKED_WAIT_HOURS")
	if h == "" {
		return 24
	}
	for i := 0; i < len(h); i++ {
		if h[i] < '0' || h[i] > '9' {
			return 24
		}
	}
	n, ok := pyInt(h)
	if !ok {
		return 24
	}
	return n
}

func taskResumed(rundir string) bool {
	d := taskStateDir(rundir)
	return pyIsFile(filepath.Join(d, "blocked-resumed")) && !taskDone(rundir)
}

// turnsOf is turns_of: how many turns the harness watched end.
func turnsOf(rundir string) int {
	body, ok := readText(filepath.Join(rundir, "turns"))
	if !ok {
		return 0
	}
	var digits []byte
	for i := 0; i < len(body); i++ {
		if body[i] >= '0' && body[i] <= '9' {
			digits = append(digits, body[i])
		}
	}
	if len(digits) == 0 {
		return 0
	}
	n, ok := pyInt(string(digits))
	if !ok || n <= 0 {
		return 0
	}
	return n
}

func hasEnv(rundir string) bool { return pyIsFile(filepath.Join(rundir, "env")) }

// envField is _env_field: one field of the env file as bin/runenv --lenient --bare reads it.
func (s *statusReader) envField(rundir, key string) string {
	path := filepath.Join(rundir, "env")
	if !pyIsDir(rundir) {
		path = rundir
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		msg := err.Error()
		if pe, ok := err.(*os.PathError); ok {
			msg = pe.Err.Error()
			if msg != "" {
				msg = strings.ToUpper(msg[:1]) + msg[1:]
			}
		}
		s.stderr("hw status: cannot read " + path + ": " + msg + "\n")
		return ""
	}
	kvs, perr := ParseEnv(data, true, true)
	if perr != nil {
		s.stderr("hw status: " + path + " is not UTF-8\n")
		return ""
	}
	for _, kv := range kvs {
		if kv.Key == key {
			return kv.Value
		}
	}
	return ""
}

// receiptRecords is the lines of receipt.jsonl a reader walks: each parsed, a
// line that is not JSON skipped.
func receiptRecords(rundir string, skipBlank bool) ([]jmap, bool) {
	body, ok := readText(filepath.Join(rundir, "receipt.jsonl"))
	if !ok {
		return nil, false
	}
	var out []jmap
	for _, line := range pySplitlinesKeepEnds(body) {
		if skipBlank {
			line = pyStrip(line)
			if line == "" {
				continue
			}
		}
		v, ok := pyLoads(line)
		if !ok {
			continue
		}
		if m, ok := v.(jmap); ok {
			out = append(out, m)
		}
	}
	return out, true
}

// receiptPane is receipt_pane.
func receiptPane(rundir string) string {
	recs, ok := receiptRecords(rundir, true)
	if !ok {
		return ""
	}
	pane := ""
	for _, r := range recs {
		if k, _ := r["key"].(string); k == "pane" && pyTruthy(r["value"]) {
			pane = pyStr(r["value"])
		}
	}
	return pane
}

// receiptValue is receipt_value: the last non-empty value recorded for key.
func receiptValue(rundir, key string) string {
	recs, ok := receiptRecords(rundir, false)
	if !ok {
		return ""
	}
	found := ""
	for _, r := range recs {
		if k, _ := r["key"].(string); k == key && pyTruthy(r["value"]) {
			found = pyStr(r["value"])
		}
	}
	return found
}

// chainingLeaseOf is chaining_lease_of.
func chainingLeaseOf(rundir string) *leaseInfo {
	body, ok := readText(filepath.Join(rundir, "chaining-lease"))
	if !ok {
		return nil
	}
	vals := map[string]string{}
	for _, l := range pySplitlinesKeepEnds(body) {
		if strings.Contains(l, "=") {
			l = strings.TrimRight(l, "\n")
			k, v, _ := strings.Cut(l, "=")
			vals[k] = v
		}
	}
	until, _ := pyInt(vals["until"])
	now := float64(time.Now().UnixNano()) / 1e9
	st := vals["state"]
	li := &leaseInfo{vals: vals}
	li.live = st == "live" && now < float64(until)
	li.expired = st == "live" && now >= float64(until) || st == "expired-cleanup"
	return li
}

var reRunStart = regexp.MustCompile(`^(\d{8}-\d{6})`)

// runsOf is runs_of: every .hw run directory, newest name first.
func (s *statusReader) runsOf(workdir string) []*runRec {
	var out []*runRec
	if workdir == "" {
		return out
	}
	hw := filepath.Join(workdir, ".hw")
	if pyIsFile(filepath.Join(hw, "done")) {
		out = append(out, &runRec{name: "(legacy)", legacy: true, done: true, task: 1})
	}
	dirs := pyGlobStar(hw)
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		if !pyIsDir(d) {
			continue
		}
		name := filepath.Base(d)
		r := &runRec{name: name, dir: d}
		if m := reRunStart.FindStringSubmatch(name); m != nil {
			if t, err := time.ParseInLocation("20060102-150405", m[1], time.Local); err == nil {
				r.started, r.hasStarted = float64(t.Unix()), true
			}
		}
		if !r.hasStarted {
			if m, ok := pyMtime(d); ok {
				r.started, r.hasStarted = m, true
			}
		}
		r.done = taskDone(d)
		r.task = taskSeq(d)
		r.reopened = taskReopened(d)
		r.closed = closedByHand(d)
		r.priorReported = priorReported(d)
		r.blockedSince, r.hasBlocked = blockedSince(d)
		r.resumed = taskResumed(d)
		r.here = runIsHere(d)
		r.mode = s.envField(d, "HW_LAUNCH_MODE")
		r.turns = turnsOf(d)
		if m, ok := pyMtime(filepath.Join(d, "turns")); ok {
			r.turnsAt, r.hasTurnsAt = m, true
		}
		r.invoker = s.envField(d, "HW_INVOKER_PANE")
		r.noReport = s.envField(d, "HW_NO_REPORT") == "1"
		r.hasEnv = hasEnv(d)
		r.execPane = receiptPane(d)
		r.modelRunning = receiptValue(d, "model_running")
		r.modelVerdict = receiptValue(d, "model_verdict")
		r.lease = chainingLeaseOf(d)
		r.ownTab = runOwns(d, "tab")
		r.ownWs = runOwns(d, "workspace")
		out = append(out, r)
	}
	return out
}

var rePendingSuffix = regexp.MustCompile(`^\.[0-9]+$`)

// pendingRulingsOf is pending_rulings_of: the count of rulings queued and not
// delivered, and the oldest mtime.
func pendingRulingsOf(rundir string) (int, float64) {
	if rundir == "" {
		return 0, 0
	}
	ents, err := os.ReadDir(rundir)
	if err != nil {
		return 0, 0
	}
	var ages []float64
	for _, e := range ents {
		n := e.Name()
		if !strings.HasPrefix(n, "pending-ruling") {
			continue
		}
		suffix := n[len("pending-ruling"):]
		if (suffix == "" || rePendingSuffix.MatchString(suffix)) && pyIsFile(filepath.Join(rundir, n)) {
			if m, ok := pyMtime(filepath.Join(rundir, n)); ok {
				ages = append(ages, m)
			}
		}
	}
	if len(ages) == 0 {
		return 0, 0
	}
	oldest := ages[0]
	for _, a := range ages {
		oldest = math.Min(oldest, a)
	}
	return len(ages), oldest
}

// ── the work directory, as a shape and as signs of life ─────────────────────

var skipDirs = map[string]bool{".hw": true, ".git": true, "node_modules": true, ".venv": true, ".next": true, "dist": true, "__pycache__": true}

// walkFiles is os.walk(top) with SKIP_DIRS pruned: fn is called with the path of
// every file (a symlink to a directory is a directory, so not a file, and is not
// entered), and stops the walk when it returns true.
func walkFiles(top string, fn func(path string) bool) {
	var rec func(dir string) bool
	rec = func(dir string) bool {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		var subdirs []string
		for _, e := range ents {
			p := filepath.Join(dir, e.Name())
			isDir := e.IsDir()
			if e.Type()&os.ModeSymlink != 0 {
				isDir = pyIsDir(p)
			}
			if isDir {
				if !skipDirs[e.Name()] {
					subdirs = append(subdirs, p)
				}
				continue
			}
			if fn(p) {
				return true
			}
		}
		for _, d := range subdirs {
			if e, err := os.Lstat(d); err == nil && e.Mode()&os.ModeSymlink != 0 {
				continue // os.walk does not follow links
			}
			if rec(d) {
				return true
			}
		}
		return false
	}
	rec(top)
}

// shapeOf is shape_of.
func shapeOf(workdir string) string {
	if workdir == "" {
		return "no work directory"
	}
	if _, err := os.Stat(filepath.Join(workdir, ".git")); err == nil {
		return "worktree"
	}
	n := 0
	walkFiles(workdir, func(string) bool { n++; return false })
	if n == 1 {
		return "1 artifact"
	}
	return itoa(n) + " artifacts"
}

func (s *statusReader) staleMinutes(getenv func(string) string) float64 {
	v := getenv("HW_STALE_MINUTES")
	if v == "" {
		return 30
	}
	m, ok := pyFloat(v)
	if !ok {
		return 30
	}
	if m > 0 {
		return m
	}
	return 30
}

func statusNow(getenv func(string) string) float64 {
	if v := getenv("HW_ST_NOW"); v != "" {
		if f, ok := pyFloat(v); ok {
			return f
		}
	}
	return wallNow()
}

// runStateActivity is run_state_activity.
func runStateActivity(rundir string) (float64, bool) {
	dirs := []string{rundir}
	ents, _ := os.ReadDir(rundir)
	var ts []string
	for _, e := range ents {
		n := e.Name()
		if len(n) >= 2 && n[0] == 't' && n[1] >= '0' && n[1] <= '9' {
			ts = append(ts, filepath.Join(rundir, n))
		}
	}
	sort.Strings(ts)
	dirs = append(dirs, ts...)
	var newest float64
	have := false
	for _, d := range dirs {
		names, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range names {
			st, err := os.Lstat(filepath.Join(d, e.Name()))
			if err != nil {
				continue
			}
			m := mtimeOf(st)
			if !have || m > newest {
				newest, have = m, true
			}
		}
	}
	return newest, have
}

// treeActivity is tree_activity: the newest file mtime under workdir, returning
// as soon as one is at or past cutoff.
func treeActivity(workdir string, cutoff float64) (float64, bool) {
	var newest float64
	have := false
	walkFiles(workdir, func(p string) bool {
		st, err := os.Lstat(p)
		if err != nil {
			return false
		}
		m := mtimeOf(st)
		if !have || m > newest {
			newest, have = m, true
			if newest >= cutoff {
				return true
			}
		}
		return false
	})
	return newest, have
}

func canon(path string) string {
	return pyCasefold(strings.TrimRight(pyRealpath(path), "/"))
}

// processActivity is process_activity: when the newest process with its cwd
// under workdir was started.
func processActivity(workdir string, now float64) (float64, bool) {
	root := canon(workdir)
	out, ok1 := runBounded(10*time.Second, "lsof", "-a", "-d", "cwd", "-Fpn")
	ps, ok2 := runBounded(10*time.Second, "ps", "-axo", "pid=,etime=")
	if !ok1 || !ok2 {
		return 0, false
	}
	pids := map[string]bool{}
	pid := ""
	for _, line := range pySplitlines(out) {
		switch {
		case strings.HasPrefix(line, "p"):
			pid = line[1:]
		case strings.HasPrefix(line, "n") && pid != "":
			c := canon(line[1:])
			if c == root || strings.HasPrefix(c, root+"/") {
				pids[pid] = true
			}
		}
	}
	var newest float64
	have := false
	for _, line := range pySplitlines(ps) {
		parts := strings.Fields(line)
		if len(parts) != 2 || !pids[parts[0]] {
			continue
		}
		et, days := parts[1], 0
		if i := strings.IndexByte(et, '-'); i >= 0 {
			d, ok := pyInt(et[:i])
			if !ok {
				continue
			}
			days, et = d, et[i+1:]
		}
		var sec []int
		bad := false
		for _, x := range strings.Split(et, ":") {
			n, ok := pyInt(x)
			if !ok {
				bad = true
				break
			}
			sec = append(sec, n)
		}
		if bad {
			continue
		}
		for len(sec) < 3 {
			sec = append([]int{0}, sec...)
		}
		started := now - float64(days*86400+sec[0]*3600+sec[1]*60+sec[2])
		if !have || started > newest {
			newest, have = started, true
		}
	}
	return newest, have
}

type staleInfo struct {
	quiet, at float64
	what      string
}

// staleOf is stale_of.
func staleOf(newest *runRec, workdir string, now, thresholdMin float64) *staleInfo {
	if newest == nil || newest.dir == "" {
		return nil
	}
	cutoff := now - thresholdMin*60
	type sign struct {
		ts   float64
		what string
	}
	var signs []sign
	seen := func(ts float64, ok bool, what string) bool {
		if ok {
			signs = append(signs, sign{ts, what})
		}
		return ok && ts >= cutoff
	}
	if seen(newest.started, newest.hasStarted, "its launch") {
		return nil
	}
	if seen(newest.turnsAt, newest.hasTurnsAt, "its last turn end") {
		return nil
	}
	if ts, ok := runStateActivity(newest.dir); seen(ts, ok, "its run state (asks, rulings, transcript)") {
		return nil
	}
	if workdir != "" && pyIsDir(workdir) {
		if ts, ok := treeActivity(workdir, cutoff); seen(ts, ok, "a file in its work directory") {
			return nil
		}
		if ts, ok := processActivity(workdir, now); seen(ts, ok, "a process started in its work directory") {
			return nil
		}
	}
	if len(signs) == 0 {
		return nil
	}
	best := signs[0]
	for _, sg := range signs[1:] {
		if sg.ts > best.ts || sg.ts == best.ts && sg.what > best.what {
			best = sg
		}
	}
	return &staleInfo{quiet: now - best.ts, at: best.ts, what: best.what}
}
