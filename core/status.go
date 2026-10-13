package main

// `hw status`: the table half (cmd_status and the Python it feeds, lib/hw/status.sh).
// bin/hw still prints the rest of `hw status` — the quiet checks after the table —
// and calls this in place of cmd_status. The Python is the spec: each Go function
// below carries the name of the Python function it ports, in the same order, and
// the output is the same bytes (setup/tests/contract-hw/cases-status).

import (
	"bufio"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type taskRow struct {
	project, task, workdir string
}

type agentRec = jmap

type statusData struct {
	herdrOK    bool
	workspaces []jmap
	panes      []jmap
	agents     []jmap

	projects      map[string]bool
	wsLabelOrder  []string
	wsByLabel     map[string]any
	deadByWs      map[string][]string
	sessionStart  float64
	hasSession    bool
	tasks         map[string]*taskRow
	taskOrder     []string
	agentsByTask  map[string][]jmap
	panesByTask   map[string][]jmap
	agentByPane   map[string]jmap
	taskByName    map[string]string
	engramByLane  map[string]string
	allLivePanes  map[string]bool
	livePaneOrder []string
}

type statusRun struct {
	*app
	rd      *statusReader
	d       *statusData
	project string
	all     bool
	colour  struct{ ok, warn, err, dim, b, z string }
}

// ── cmd_status: what is asked of herdr ──────────────────────────────────────

func (a *app) cmdStatusArgs() int {
	project, all := a.getenv("HW_STATUS_PROJECT"), a.getenv("HW_STATUS_ALL") == "1"
	args := a.args
	for len(args) > 0 {
		switch {
		case args[0] == "--all":
			all = true
		case strings.HasPrefix(args[0], "-"):
			a.die("unknown option: " + args[0] + " (usage: hw status [<project>] [--all])")
		default:
			project = a.c.t.canonProject(args[0])
			if !a.c.t.known(project) {
				a.die("unknown project: " + args[0] + " (expected: " + a.laneNamesHint() + ")")
			}
		}
		args = args[1:]
	}
	return a.cmdStatus(project, all)
}

// laneNamesHint is lane_names_hint: "a|x | b | c".
func (a *app) laneNamesHint() string {
	var parts []string
	for _, l := range a.c.t.names {
		s := l
		if h := a.c.t.get(l, "hint_aliases"); h != "" {
			s += "|" + h
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " | ")
}

func (a *app) cmdStatus(project string, all bool) int {
	h := herdrClient{getenv: a.getenv}
	d := &statusData{}
	// what does not depend on herdr's lists starts now and is joined where it is used: the process holding
	// the socket (its start time), and the two read-only audits printed after the table
	startedCh := make(chan string, 1)
	go func() { startedCh <- a.herdrServerStarted() }()
	auditCh := a.startAudits()
	wsOut, rc := h.capture("workspace", "list")
	wsOut = bashSub(wsOut)
	var panesJSON, agentsJSON = "{}", "{}"
	var exited []jmap
	d.herdrOK = rc == 0
	if !d.herdrOK { // # MUTATION-ANCHOR: 826-G18
		first := wsOut
		if i := strings.IndexByte(first, '\n'); i >= 0 {
			first = first[:i]
		}
		a.warn("herdr unreachable: " + first)
		wsOut = "{}"
	} else {
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			o, ok := h.stdout("pane", "list")
			if ok {
				panesJSON = bashSub(o)
			}
		}()
		go func() {
			defer wg.Done()
			o, ok := h.stdout("agent", "list")
			if ok {
				agentsJSON = bashSub(o)
			}
		}()
		go func() { defer wg.Done(); exited = h.drainExited(600*time.Millisecond, 8*time.Second) }()
		wg.Wait()
	}
	started := <-startedCh
	rd := &statusReader{stderr: func(s string) { os.Stderr.WriteString(s) }, now: wallNow}
	s := &statusRun{app: a, rd: rd, d: d, project: project, all: all}
	s.colour.ok, s.colour.warn, s.colour.err, s.colour.dim, s.colour.b, s.colour.z = a.cOK, a.cWarn, a.cErr, a.cDim, a.cB, a.c0
	s.load(wsOut, panesJSON, agentsJSON, exited, started)
	out := bufio.NewWriter(a.term.out)
	a.reportSpaces(out, d)
	limits, orphans, cleanup := s.reportTasks(out)
	s.reportCleanup(out, cleanup)
	s.reportOrphans(out, orphans)
	if p := a.getenv("HW_STATUS_ORPHANS_JSON"); p != "" && a.getenv("HW_CORE_READONLY") == "" {
		s.writeOrphansJSON(p, orphans)
	}
	s.reportCaveats(out, limits)
	out.Flush()
	for _, o := range auditCh() {
		a.term.out.Write(o)
	}
	return 0
}

// herdrServerStarted is _herdr_server_started: ps's lstart of the process holding the socket.
func (a *app) herdrServerStarted() string {
	sock := a.getenv("HERDR_SOCKET_PATH")
	if sock == "" {
		return ""
	}
	pid := ""
	if _, err := exec.LookPath("lsof"); err == nil {
		o, _ := runBounded(30*time.Second, "lsof", "-t", sock)
		pid = firstLine(o)
	} else {
		o, _ := runBounded(30*time.Second, "fuser", sock)
		if f := strings.Fields(firstLine(o)); len(f) > 0 {
			pid = f[0]
		}
	}
	if pid == "" {
		return ""
	}
	o, _ := runBounded(30*time.Second, "ps", "-o", "lstart=", "-p", pid)
	return firstLine(o)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// startAudits starts the two read-only audits cmd_status ends with — the engram label proxy's and the
// always-loaded memory indexes' (one line each, never a failure) — and returns what joins them: their
// output, in the order bash prints it. Each is a Python process, so they run beside the herdr reads
// instead of after the table.
func (a *app) startAudits() func() [][]byte {
	bin := a.getenv("HW_CORE_BIN_DIR")
	root := a.getenv("HW_CORE_ROOT")
	if bin == "" {
		return func() [][]byte { return nil }
	}
	if root == "" {
		root = a.c.brain
	}
	scripts := [][]string{{filepath.Join(root, "bin", "engram-label-proxy"), "--audit"}, {filepath.Join(bin, "memory-audit")}}
	outs := make([][]byte, len(scripts))
	var wg sync.WaitGroup
	for i, sc := range scripts {
		wg.Add(1)
		go func(i int, sc []string) {
			defer wg.Done()
			outs[i], _ = exec.Command("python3", sc...).CombinedOutput()
		}(i, sc)
	}
	return func() [][]byte { wg.Wait(); return outs }
}

// ── the inputs, joined (_status_inputs_source, _status_liveness_source) ─────

func (s *statusRun) load(wsJSON, panesJSON, agentsJSON string, exited []jmap, serverStarted string) {
	d := s.d
	blob := func(text string) jmap {
		v, ok := pyLoads(text)
		if !ok {
			return jmap{}
		}
		return pyDict(v)
	}
	d.workspaces = pyDicts(pyDict(blob(wsJSON)["result"])["workspaces"])
	d.panes = pyDicts(pyDict(blob(panesJSON)["result"])["panes"])
	d.agents = pyDicts(pyDict(blob(agentsJSON)["result"])["agents"])

	order, exitedWs := exitedPanes(exited)
	live := map[string]bool{}
	for _, p := range d.panes {
		live[pyStr(p["pane_id"])] = true
	}
	d.deadByWs = map[string][]string{}
	for _, pid := range order {
		ws := exitedWs[pid]
		if live[pid] || !pyTruthy(ws) {
			continue
		}
		k := pyStr(ws)
		d.deadByWs[k] = append(d.deadByWs[k], pid) // # MUTATION-ANCHOR: 826-G02
	}
	d.wsByLabel = map[string]any{}
	for _, w := range d.workspaces {
		if lab := w["label"]; pyTruthy(lab) {
			l := pyStr(lab)
			if _, seen := d.wsByLabel[l]; !seen {
				d.wsLabelOrder = append(d.wsLabelOrder, l)
			}
			d.wsByLabel[l] = w["workspace_id"]
		}
	}
	d.projects = map[string]bool{}
	for _, p := range s.c.t.names {
		d.projects[p] = true
	}
	if raw := pyStrip(serverStarted); raw != "" {
		if t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(raw), " "), time.Local); err == nil {
			d.sessionStart, d.hasSession = float64(t.Unix()), true
		}
	}

	d.tasks = map[string]*taskRow{}
	row := func(project, task string) *taskRow {
		k := project + ":" + task
		if r, ok := d.tasks[k]; ok {
			return r
		}
		r := &taskRow{project: project, task: task}
		d.tasks[k] = r
		d.taskOrder = append(d.taskOrder, k)
		return r
	}
	work := s.c.work
	for _, dir := range pyGlobStar2(work) {
		if pyIsDir(dir) {
			row(filepath.Base(filepath.Dir(dir)), filepath.Base(dir)).workdir = dir
		}
	}
	for _, spec := range strings.Split(s.worktreeRoots(), "\n") {
		if !strings.Contains(spec, ":") {
			continue
		}
		i := strings.LastIndexByte(spec, ':')
		root, project := spec[:i], spec[i+1:]
		for _, dir := range pyGlobStarRoot(root) {
			if pyIsDir(dir) {
				row(project, filepath.Base(dir)).workdir = dir
			}
		}
	}

	d.agentsByTask = map[string][]jmap{}
	for _, a := range d.agents {
		tok := pyDict(a["tokens"])
		key := ""
		hp, _ := tok["hw_project"].(string)
		if _, isProject := d.projects[hp]; isProject && pyTruthy(tok["hw_task"]) {
			key = hp + ":" + pyStr(tok["hw_task"])
			row(hp, pyStr(tok["hw_task"]))
		} else {
			for _, label := range d.wsLabelOrder {
				if pyEq(d.wsByLabel[label], a["workspace_id"]) && s.hwLabel(label) {
					key = label
					parts := strings.SplitN(label, ":", 2)
					row(parts[0], parts[1])
					break
				}
			}
		}
		if key != "" {
			d.agentsByTask[key] = append(d.agentsByTask[key], a)
		}
	}

	// liveness: a pane in a task's work directory IS that task
	type idx struct {
		root, key string
	}
	var workIndex []idx
	for _, k := range d.taskOrder {
		if t := d.tasks[k]; t.workdir != "" {
			workIndex = append(workIndex, idx{canon(t.workdir), k})
		}
	}
	sort.SliceStable(workIndex, func(i, j int) bool {
		return utf8.RuneCountInString(workIndex[i].root) > utf8.RuneCountInString(workIndex[j].root)
	})
	d.taskByName = map[string]string{}
	for _, k := range d.taskOrder {
		d.taskByName[d.tasks[k].task] = k
	}
	d.agentByPane = map[string]jmap{}
	for _, a := range d.agents {
		if pyTruthy(a["pane_id"]) {
			d.agentByPane[pyStr(a["pane_id"])] = a
		}
	}
	d.panesByTask = map[string][]jmap{}
	for _, p := range d.panes {
		if !pyTruthy(p["pane_id"]) {
			continue
		}
		key := ""
		for _, cand := range []any{p["cwd"], p["foreground_cwd"]} {
			if !pyTruthy(cand) {
				continue
			}
			c := canon(pyStr(cand))
			for _, w := range workIndex {
				if c == w.root || strings.HasPrefix(c, w.root+"/") {
					key = w.key
					break
				}
			}
			if key != "" {
				break
			}
		}
		if key == "" {
			if label := p["label"]; pyTruthy(label) {
				if k, ok := d.taskByName[pyStr(label)]; ok {
					key = k
				}
			}
		}
		if key != "" {
			d.panesByTask[key] = append(d.panesByTask[key], p)
		}
	}
	d.engramByLane = map[string]string{}
	for _, l := range s.c.t.names {
		lab := s.c.t.get(l, "engram")
		if lab == "" {
			lab = "brain"
		}
		d.engramByLane[l] = lab
	}
}

func pyEq(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return pyStr(a) == pyStr(b)
}

func (s *statusRun) hwLabel(label string) bool {
	if strings.Count(label, ":") != 1 {
		return false
	}
	return s.d.projects[strings.SplitN(label, ":", 2)[0]]
}

// worktreeRoots is _status_worktree_roots: "<worktree root>:<lane>" per product-repo lane.
func (s *statusRun) worktreeRoots() string {
	var out []string
	for _, p := range s.c.t.names {
		if s.c.t.get(p, "product_repo") != "1" {
			continue
		}
		out = append(out, s.c.lanePath(p, "worktree_root", "")+":"+p)
	}
	return strings.Join(out, "\n")
}

// pyGlobStar2 is sorted(glob.glob(os.path.join(work, "*", "*"))).
func pyGlobStar2(work string) []string {
	var out []string
	for _, a := range pyGlobStar(work) {
		out = append(out, pyGlobStar(a)...)
	}
	sort.Strings(out)
	return out
}

// pyGlobStarRoot is sorted(glob.glob(os.path.join(root, "*"))): an empty root
// is the current directory, as os.path.join("", "*") is.
func pyGlobStarRoot(root string) []string {
	if root != "" {
		return pyGlobStar(root)
	}
	ents, err := os.ReadDir(".")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// ── the context line of each live executor (pane_ctx_*) ─────────────────────

var (
	reCtxRow   = regexp.MustCompile(`Ctx: *[0-9]+(\.[0-9]+)?[kKmM]? *\|`)
	reCtxNum   = regexp.MustCompile(`Ctx: *([0-9.]+)([kKmM]?)`)
	reCtxUsed2 = regexp.MustCompile(`Ctx Used: *([0-9.]+)`)
)

type ctxReading struct {
	tokens int
	pct    string
}

// paneCtxParse is pane_ctx_parse.
func paneCtxParse(text string) *ctxReading {
	var rows []string
	for _, l := range pySplitlines(strings.ReplaceAll(text, " ", " ")) {
		if reCtxRow.MatchString(l) { // # MUTATION-ANCHOR: 826-G03
			rows = append(rows, l)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	last := rows[len(rows)-1]
	m := reCtxNum.FindStringSubmatch(last)
	if m == nil {
		return nil
	}
	mult := 1.0
	switch strings.ToLower(m[2]) {
	case "k":
		mult = 1000
	case "m":
		mult = 1000000
	}
	f, ok := pyFloat(m[1])
	if !ok {
		return nil
	}
	r := &ctxReading{tokens: int(f * mult)}
	if u := reCtxUsed2.FindStringSubmatch(last); u != nil {
		r.pct = u[1]
	}
	return r
}

// paneCtxMany is pane_ctx_many: every pane read at once, so the whole status costs one timeout.
func (s *statusRun) paneCtxMany(panes []string) map[string]*ctxReading {
	set := map[string]bool{}
	for _, p := range panes {
		if p != "" {
			set[p] = true
		}
	}
	if len(set) == 0 {
		return map[string]*ctxReading{}
	}
	var list []string
	for p := range set {
		list = append(list, p)
	}
	sort.Strings(list)
	timeout := 2.0
	if v := s.getenv("HW_STATUS_CTX_TIMEOUT"); v != "" {
		if f, ok := pyFloat(v); ok {
			timeout = f
		}
	}
	h := herdrClient{getenv: s.getenv}
	res := make([]*ctxReading, len(list))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, p := range list {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if out, ok := h.readPane(p, time.Duration(timeout*float64(time.Second))); ok { // # MUTATION-ANCHOR: 826-G04
				res[i] = paneCtxParse(out)
			}
		}(i, p)
	}
	wg.Wait()
	m := map[string]*ctxReading{}
	for i, p := range list {
		m[p] = res[i]
	}
	return m
}

// ── the table (status_report_spaces, status_report_tasks) ────────────────────

func (a *app) reportSpaces(out *bufio.Writer, d *statusData) {
	pl := func(s string) { out.WriteString(s + "\n") }
	pl("\n" + a.cB + "spaces" + a.c0)
	if len(d.workspaces) == 0 {
		if d.herdrOK {
			pl("  " + a.cDim + "·" + a.c0 + " none")
		} else {
			pl("  " + a.cDim + "·" + a.c0 + " unknown — herdr unreachable")
		}
	}
	for _, w := range d.workspaces {
		label := "-"
		if pyTruthy(w["label"]) {
			label = pyStr(w["label"])
		}
		drift := ""
		if strings.Contains(label, ":") {
			drift = "  " + a.cWarn + "DRIFT" + a.c0 + " — legacy per-task workspace; expected a tab in the project's bare-label space"
		}
		pl("  " + pyStr(w["workspace_id"]) + "  " + label + drift)
	}
}

type orphanRec struct {
	key, workdir, run string
	turns             int
	at                float64
	hasAt             bool
	invoker           string
	noReport, hasEnv  bool
	execPane          string
	livePanes         []string
}

type cleanupRec struct {
	key, kind, id string
	panes         []string
}

func (s *statusRun) strOr(v any, def string) string {
	if pyTruthy(v) {
		return pyStr(v)
	}
	return def
}

func keyCell(key string) string {
	if utf8.RuneCountInString(key) < 38 {
		return pyLjust(key, 38)
	}
	return key + "  "
}

// placementGone is _placement_gone.
func placementGone(here bool, ownTab, ownWs string) string {
	switch {
	case ownTab != "":
		return "tab " + ownTab + " closed"
	case ownWs != "":
		return "workspace " + ownWs + " closed"
	case here:
		return "pane gone (--here)"
	}
	return "its placement record is gone, so what closed is not recorded"
}

// orphanOf is orphan_of: finished, never reported, nothing running.
func orphanOf(newest *runRec, hasAgent bool) *orphanRec {
	if newest == nil || hasAgent || newest.done || newest.turns < 1 {
		return nil
	}
	return &orphanRec{run: newest.name, turns: newest.turns, at: newest.turnsAt, hasAt: newest.hasTurnsAt,
		invoker: newest.invoker, noReport: newest.noReport, hasEnv: newest.hasEnv, execPane: newest.execPane}
}

// unreportedRow is unreported_row.
func unreportedRow(newest *runRec) (string, []string, bool) {
	if newest == nil {
		return "", nil, false
	}
	cl, prior, n := newest.closed, newest.priorReported, newest.task
	var notes []string
	if prior != 0 && n > 1 { // # MUTATION-ANCHOR: 826-G06
		verb := "ended"
		if cl != nil {
			verb = "closed"
		}
		notes = append(notes, "task "+itoa(prior)+" reported, task "+itoa(n)+" "+verb+" before reporting")
	}
	if cl == nil {
		if len(notes) == 0 {
			return "", nil, false
		}
		return "finished, unreported", notes, true
	}
	when := "an unrecorded time"
	if cl.hasAt {
		when = pyStrftimeDHM(cl.at)
	}
	if cl.legacy {
		notes = append(notes, "closed by hand: an `hw done` verification (receipt) at "+when+", and no report — read from the receipt, this run predates the closed-by-hand mark")
	} else {
		by := cl.by
		if by == "none" {
			by = "no flag"
		}
		after := "no report existed"
		if cl.reported {
			after = "after a report existed"
		}
		notes = append(notes, "closed by hand with `hw done` ("+by+") at "+when+", "+after)
	}
	notes = append(notes, "not a death: the task was closed on purpose without done-invoker — its work and engram findings may still be there")
	return "closed-unreported", notes, true // # MUTATION-ANCHOR: 826-G07
}

// orphanWhy is orphan_why.
func orphanWhy(o *orphanRec, live map[string]bool) string {
	switch {
	case !o.hasEnv:
		return "no run env file, so hw cannot say whether it had a return channel at all"
	case o.noReport:
		return "this dispatch deliberately used --no-report (fire-and-forget), so no brainer report was requested"
	case o.invoker == "":
		return "no HW_INVOKER_PANE in its run env — done-invoker refuses at the gate (`no HW_INVOKER_PANE` in bin/done-invoker) and publishes nothing, so this was never the executor's to fix"
	case !live[o.invoker]:
		return "its brainer's pane " + o.invoker + " is gone, so done-invoker had nowhere to deliver"
	}
	return "its brainer " + o.invoker + " is still live, so nothing stopped it — done-invoker was simply never called"
}

// cleanupBacklogOf is cleanup_backlog_of.
func cleanupBacklogOf(newest *runRec, hasAgent bool, live []jmap, ownTab, ownWs string) *cleanupRec {
	if newest == nil || !newest.done || hasAgent || len(live) == 0 {
		return nil
	}
	if l := newest.lease; l != nil && l.live {
		return nil
	}
	var panes []string
	for _, p := range live {
		if pyTruthy(p["pane_id"]) {
			panes = append(panes, pyStr(p["pane_id"]))
		}
	}
	switch {
	case ownTab != "":
		return &cleanupRec{kind: "tab", id: ownTab, panes: panes}
	case ownWs != "":
		return &cleanupRec{kind: "workspace", id: ownWs, panes: panes}
	}
	return nil
}

func eqStr(v any, s string) bool { x, ok := v.(string); return ok && x == s }

func (s *statusRun) reportTasks(out *bufio.Writer) (map[string]bool, []*orphanRec, []*cleanupRec) {
	c := s.colour
	d := s.d
	pl := func(line string) { out.WriteString(line + "\n") }
	pl("\n" + c.b + "tasks" + c.z)
	if len(d.tasks) == 0 {
		pl("  " + c.dim + "·" + c.z + " no work directories, no worktrees, no task panes")
	}
	limits := map[string]bool{}
	suppressed := 0
	var orphans []*orphanRec
	var cleanupBacklog []*cleanupRec
	inScope := func(k string) bool { return s.project == "" || strings.HasPrefix(k, s.project+":") }

	// the agents a row names: agents_by_task first, then the agent of a live pane in its directory
	agentsOf := func(k string) []jmap {
		mine := append([]jmap(nil), d.agentsByTask[k]...)
		seen := map[string]bool{}
		for _, a := range mine {
			seen[pyStr(a["pane_id"])] = true
		}
		for _, p := range d.panesByTask[k] {
			if a, ok := d.agentByPane[pyStr(p["pane_id"])]; ok && pyTruthy(a) && !seen[pyStr(a["pane_id"])] {
				mine = append(mine, a)
				seen[pyStr(a["pane_id"])] = true
			}
		}
		return mine
	}
	var ctxPanes []string
	for _, k := range d.taskOrder {
		if !inScope(k) {
			continue
		}
		if ags := agentsOf(k); len(ags) > 0 && pyTruthy(ags[0]["pane_id"]) {
			ctxPanes = append(ctxPanes, pyStr(ags[0]["pane_id"]))
		}
	}
	ctxByPane := map[string]*ctxReading{}
	if d.herdrOK {
		ctxByPane = s.paneCtxMany(ctxPanes)
	}
	ctxFresh := 150000
	if v := s.getenv("HW_CTX_PREFER_FRESH_TOKENS"); v != "" {
		if n, ok := pyInt(v); ok {
			ctxFresh = n
		}
	}
	keys := append([]string(nil), d.taskOrder...)
	sort.Strings(keys)
	for _, key := range keys {
		if !inScope(key) {
			continue
		}
		t := d.tasks[key]
		wsv, hasWs := d.wsByLabel[key]
		ws := ""
		wsTruthy := hasWs && pyTruthy(wsv)
		if hasWs {
			ws = pyStr(wsv)
		}
		mine := agentsOf(key)
		livePanes := d.panesByTask[key]
		runs := s.rd.runsOf(t.workdir)
		var newest *runRec
		if len(runs) > 0 {
			newest = runs[0]
		}
		hereRun, runMode, ownTab, ownWs := false, "", "", ""
		if newest != nil {
			hereRun, runMode, ownTab, ownWs = newest.here, newest.mode, newest.ownTab, newest.ownWs
		}
		var dead []string
		if wsTruthy {
			dead = d.deadByWs[ws]
		}

		var tok jmap
		if len(mine) > 0 {
			tok = pyDict(mine[0]["tokens"])
		} else {
			tok = jmap{}
		}
		doneStatus := tok["done_status"]
		doneState := tok["done_state"]
		doneTok := eqStr(doneStatus, "done") || pyTruthy(tok["hw_done"]) || pyTruthy(tok["done"])
		blockedTok := eqStr(doneStatus, "blocked")
		blockedNow := blockedTok || newest.blockedTruthy()
		doneDisk := false
		for _, r := range runs {
			if r.done {
				doneDisk = true
			}
		}
		doneNewest := newest != nil && newest.done
		turnEnded := eqStr(tok["turn_state"], "ended_unreported")
		turnAt := s.strOr(tok["turn_ended_at"], "?")
		turns := s.strOr(tok["turns"], "?")
		orph := orphanOf(newest, len(mine) > 0)
		if orph != nil {
			orph.key = key
			orph.workdir = t.workdir
			for _, p := range livePanes {
				orph.livePanes = append(orph.livePanes, pyStrOrEmpty(p["pane_id"]))
			}
			orphans = append(orphans, orph)
		}
		if cl := cleanupBacklogOf(newest, len(mine) > 0, livePanes, ownTab, ownWs); cl != nil {
			cl.key = key
			cleanupBacklog = append(cleanupBacklog, cl)
		}

		var notes []string
		var chain *leaseInfo
		if newest != nil {
			chain = newest.lease
		}
		var mark, state, tint string
		var a jmap
		if len(mine) > 0 {
			a = mine[0]
		}
		aPane := func() string { // (a or {}).get("pane_id") or "<pane>"
			if a != nil && pyTruthy(a["pane_id"]) {
				return pyStr(a["pane_id"])
			}
			return "<pane>"
		}
		keyArg := strings.Replace(key, ":", " ", 1)
		if len(mine) > 0 || len(livePanes) > 0 {
			mark, state, tint = "●", "process-live", c.ok
			var detail string
			if a != nil {
				agentStatus := s.strOr(a["agent_status"], "unknown")
				switch agentStatus {
				case "working", "retry":
					state = "working"
				case "idle", "done":
					state = "idle"
				case "blocked":
					state = "blocked"
				}
				detail = s.strOr(a["agent"], "agent") + " " + agentStatus + " in " + pyStr(a["pane_id"])
			} else {
				var ids []string
				for _, p := range livePanes {
					ids = append(ids, pyStr(p["pane_id"]))
				}
				detail = "pane " + strings.Join(ids, ", ") + " live, no agent detected in it"
			}
			if !wsTruthy {
				switch {
				case ownTab != "":
					detail += " (tab " + ownTab + " in the project's space — a tab has no workspace of its own, by design)"
				case hereRun:
					detail += " (--here: no space of its own, by design)"
				case runMode == "tab":
					detail += " (a tab in the project's space, not a workspace of its own)"
				default:
					detail += " (no workspace of its own, and no --here record either)"
				}
			}
			notes = append(notes, detail)
			if newest != nil && newest.modelRunning != "" {
				mv := newest.modelVerdict
				if strings.HasPrefix(mv, "DIVERGENCE") {
					notes = append(notes, "model "+newest.modelRunning+" — "+mv)
					limits["model"] = true
				} else {
					notes = append(notes, "model "+newest.modelRunning+" (measured)")
				}
			}
			if len(dead) > 0 {
				notes = append(notes, "another pane in "+ws+" exited: "+strings.Join(dead, ", "))
			}
			if pyTruthy(tok["ask_seq"]) {
				notes = append(notes, pyStrip("ask "+pyStr(tok["ask_seq"])+" "+s.strOr(tok["ask_state"], "")))
			}
			if a != nil {
				if cr, in := ctxByPane[pyStr(a["pane_id"])]; in {
					if cr == nil {
						notes = append(notes, "context: not measured (unreadable, no statusline, or timed out) — not a fresh session")
					} else {
						n := "context " + itoa(cr.tokens) + " tokens used"
						if cr.pct != "" {
							n += " (" + cr.pct + "% of the window)"
						}
						if cr.tokens > ctxFresh { // # MUTATION-ANCHOR: 826-G08
							n += " → prefer a fresh executor over hw next or a ruling"
						}
						notes = append(notes, n)
					}
				}
			}
			rundir := ""
			if newest != nil {
				rundir = newest.dir
			}
			if nr, oldest := pendingRulingsOf(rundir); nr != 0 { // # MUTATION-ANCHOR: 826-G09
				age := 0
				if oldest != 0 {
					age = int(statusNow(s.getenv) - oldest)
					if age < 0 {
						age = 0
					}
				}
				var human string
				switch {
				case age < 120:
					human = itoa(age) + "s"
				case age < 7200:
					human = itoa(age/60) + " min"
				default:
					human = itoa(age/3600) + " h"
				}
				plural := "s"
				if nr == 1 {
					plural = ""
				}
				notes = append(notes, itoa(nr)+" ruling"+plural+" queued, not delivered — oldest "+human+" old; it lands at the executor's next turn end")
			}
			stranded := turnEnded && eqStr(doneState, "undelivered") && !doneNewest
			strandNote := func() {
				notes = append(notes, "turn "+turns+" ended "+turnAt+" and the delivery had already failed — nothing in that pane is going to retry it")
				notes = append(notes, "ENDED AND NEVER REPORTED: read its tokens (sum/sum2/…) and its engram observations, then `hw done` it. A larger budget and `hw unstick` both answer a question nobody is asking — the work finished, the delivery did not")
				limits["turn"] = true
			}
			switch {
			case newest != nil && newest.reopened:
				mark, state, tint = "●", "reopened", c.ok
				notes = append(notes, "reopened by hw revive after reporting done — a new done-invoker is expected, without --retask")
			case newest != nil && newest.resumed:
				mark, state, tint = "●", "resumed", c.ok
				notes = append(notes, "resumed by hw ruling after a BLOCKED report — same task, same session; a new done-invoker is expected")
			case (doneTok || doneNewest) && !blockedNow: // # MUTATION-ANCHOR: 826-G22
				if eqStr(doneState, "undelivered") && !doneNewest {
					if stranded {
						mark, state, tint = "!", "done-STRANDED", c.warn
					} else {
						mark, state, tint = "!", "done-NOT-delivered", c.warn
					}
				} else {
					mark, state, tint = "✓", "reported-done", c.ok
					if chain != nil && chain.live {
						state = "chaining-retained"
					} else if chain != nil && chain.expired {
						mark, state, tint = "!", "chaining-expired", c.warn
					}
				}
				switch {
				case chain != nil && chain.live:
					notes = append(notes, "reported done; chaining retention is LIVE until "+leaseVal(chain, "until_h", "?")+" — holder "+leaseVal(chain, "holder", "?")+": "+leaseVal(chain, "reason", "?"))
				case chain != nil && chain.expired:
					notes = append(notes, "reported done; chaining lease EXPIRED, cleanup="+leaseVal(chain, "cleanup", "pending"))
				default:
					notes = append(notes, "reported done, pane still open")
				}
				if stranded {
					strandNote()
				}
			case blockedNow:
				switch {
				case eqStr(doneState, "undelivered") && !doneNewest:
					if stranded {
						mark, state, tint = "!", "blocked-STRANDED", c.warn
					} else {
						mark, state, tint = "!", "blocked-NOT-delivered", c.warn
					}
				case newest.blockedTruthy():
					mark, state, tint = "!", "blocked-waiting", c.warn
				default:
					mark, state, tint = "!", "reported-blocked", c.warn
				}
				if state == "blocked-waiting" {
					hours := blockedWaitHours(s.getenv)
					until := float64(newest.blockedSince + hours*3600)
					notes = append(notes, "reported BLOCKED and is WAITING for a ruling since "+pyStrftimeDHM(float64(newest.blockedSince))+" — pane and session kept")
					notes = append(notes, "answer it: `hw ruling "+aPane()+" \"<the unblock>\"` resumes the same task; `hw done "+keyArg+" --blocked` closes it; `hw reap --apply` closes it unanswered after "+pyStrftimeDHM(until)+" (HW_BLOCKED_WAIT_HOURS="+itoa(hours)+")")
				} else {
					notes = append(notes, "reported BLOCKED and stopped, pane still open")
				}
				if stranded {
					strandNote()
				}
			case turnEnded: // # MUTATION-ANCHOR: 826-G20
				status := ""
				if a != nil {
					status = s.strOr(a["agent_status"], "")
				}
				switch status {
				case "", "idle", "done", "unknown":
					mark, state, tint = "!", "idle, silent", c.warn
					notes = append(notes, "turn "+turns+" ended "+turnAt+" and done-invoker was never called")
					notes = append(notes, "idle and silent — that is not failed and not done; read its engram findings or ask it in its pane")
					limits["turn"] = true
				default:
					notes = append(notes, "turn "+turns+" ended "+turnAt+" unreported, and it is "+status+" again since")
				}
			case orph != nil: // # MUTATION-ANCHOR: 826-G21
				mark, state, tint = "!", "finished, unreported", c.warn
				notes = append(notes, "its agent is gone and the pane is a bare shell — see `finished, never reported` below")
				limits["orphan"] = true
				if st, ns, ok := unreportedRow(newest); ok {
					state = st
					notes = append(notes, ns...)
				}
			}
			if state == "working" && newest != nil && t.workdir != "" { // # MUTATION-ANCHOR: 826-G19
				mins := s.rd.staleMinutes(s.getenv)
				if q := staleOf(newest, t.workdir, statusNow(s.getenv), mins); q != nil {
					mark, state, tint = "!", "STALE", c.warn
					notes = append(notes, "working by herdr's word, but no turn end, file change or new process for "+itoa(int(math.Floor(q.quiet/60)))+" min (last sign: "+q.what+" at "+pyStrftimeDHM(q.at)+"); threshold "+pyG(mins)+" min, HW_STALE_MINUTES changes it")
					notes = append(notes, "look at: `herdr pane read "+aPane()+"` (what it is doing), `hw log "+keyArg+"` (what was said), its engram findings; it may be waiting on something, hung, or in one long step")
					limits["stale"] = true
				}
			}
		} else if wsTruthy && len(dead) > 0 { // # MUTATION-ANCHOR: 826-G10
			mark, state, tint = "✗", "DIED", c.err
			notes = append(notes, "space "+ws+" is open with no agent, and "+strings.Join(dead, ", ")+" exited")
			notes = append(notes, "herdr reports no exit code, so this says died, not failed")
			limits["exit"] = true
		} else if wsTruthy { // # MUTATION-ANCHOR: 826-G23
			mark, state, tint = "!", "no agent", c.warn
			notes = append(notes, "space "+ws+" is open but nothing in it is an agent — never started, or released")
		} else if newest != nil && newest.reopened {
			mark, state, tint = "!", "reopened, gone", c.warn
			notes = append(notes, "hw revive reopened this run and its pane is gone with no new report — `hw done` it, or `hw revive` it again")
		} else if blockedTok {
			mark, state, tint = "!", "blocked", c.warn
			notes = append(notes, "the executor reported BLOCKED and stopped, and its "+placementGone(hereRun, ownTab, ownWs))
		} else if doneTok || doneDisk {
			mark, state, tint = "✓", "done", c.ok
			notes = append(notes, "reported done, "+placementGone(hereRun, ownTab, ownWs))
		} else if orph != nil {
			mark, state, tint = "!", "finished, unreported", c.warn
			notes = append(notes, "its "+placementGone(hereRun, ownTab, ownWs)+" and no turn since — see `finished, never reported` below")
			limits["orphan"] = true
			if st, ns, ok := unreportedRow(newest); ok {
				state = st
				notes = append(notes, ns...)
			}
		} else if newest != nil && !newest.done && newest.turns < 1 && d.hasSession && newest.hasStarted && newest.started >= d.sessionStart {
			var ur []string
			urState, ok := "", false
			if newest.closed != nil { // # MUTATION-ANCHOR: 826-G11
				urState, ur, ok = unreportedRow(newest)
			}
			if ok {
				mark, state, tint = "!", urState, c.warn
				notes = append(notes, "its "+placementGone(hereRun, ownTab, ownWs)+", and no turn end was ever recorded")
				notes = append(notes, ur...)
				limits["first-turn"] = true
			} else {
				mark, state, tint = "✗", "DIED-BEFORE-FIRST-TURN", c.err
				notes = append(notes, "its "+placementGone(hereRun, ownTab, ownWs)+", and no turn end was ever recorded — it died or was closed before finishing one (`hw done` closes a task without a report too, and leaves no mark of it)")
				notes = append(notes, "look at: `hw log "+keyArg+"`, its engram findings, .hw/"+newest.name+"/ in its work directory; if it was closed on purpose, `hw done` leaves this row in place for the rest of this herdr session; otherwise dispatch it again")
				limits["first-turn"] = true
			}
		} else {
			mark, state, tint = "?", "no information", c.dim
			switch {
			case !d.hasSession:
				notes = append(notes, "no space, no done marker, and hw could not read the herdr session's start time — the backlog's silence proves nothing")
				limits["session"] = true
			case newest == nil || !newest.hasStarted:
				notes = append(notes, "no space, no done marker, and no .hw run record — nothing places this task in time, so the backlog cannot answer for it")
				limits["session"] = true
			case newest.started < d.sessionStart:
				notes = append(notes, "last run started "+pyStrftimeDHM(newest.started)+", before this herdr session began "+pyStrftimeDHM(d.sessionStart))
				notes = append(notes, "the pane_exited backlog is per session, so it holds nothing about this task")
				limits["session"] = true
			case hereRun:
				notes = append(notes, "a --here run, so it never had a space of its own — and no live pane is in its work directory")
				notes = append(notes, "it ended without reporting: no done marker, and nothing left to ask")
			default:
				what := "space"
				if ownTab != "" {
					what = "tab " + ownTab
				} else if ownWs != "" {
					what = "workspace " + ownWs
				}
				notes = append(notes, "started in this herdr session, and its "+what+" is gone with no done marker — closed without reporting")
				if ownTab != "" {
					notes = append(notes, "a done marker only appears once the brainer is TOLD; if this executor reported while you were mid-turn, the delivery failed and the work may be complete — check its engram findings before calling it abandoned")
				}
			}
		}

		if state == "no information" && !s.all { // # MUTATION-ANCHOR: 826-G16
			suppressed++
			continue
		}
		counts := []string{shapeOf(t.workdir)}
		if t.workdir != "" {
			word := "runs"
			if len(runs) == 1 {
				word = "run"
			}
			counts = append(counts, itoa(len(runs))+" "+word)
			if newest != nil && newest.task > 1 {
				counts = append(counts, "task "+itoa(newest.task))
			}
		}
		pl("  " + tint + mark + c.z + " " + keyCell(key) + tint + pyLjust(state, 16) + c.z + "  " + c.dim + strings.Join(counts, " · ") + c.z)
		for _, n := range notes {
			pl("      " + c.dim + n + c.z)
		}
	}
	if suppressed > 0 {
		pl("  " + c.dim + "·" + c.z + " " + itoa(suppressed) + " older task(s) with no information, hidden — run " + c.dim + "hw status --all" + c.z + " for them")
	}
	return limits, orphans, cleanupBacklog
}

func pyStrOrEmpty(v any) string {
	if !pyTruthy(v) {
		return ""
	}
	return pyStr(v)
}

func leaseVal(l *leaseInfo, k, def string) string {
	if v, ok := l.vals[k]; ok {
		return v
	}
	return def
}

// ── the sections after the table ─────────────────────────────────────────────

func (s *statusRun) reportCleanup(out *bufio.Writer, backlog []*cleanupRec) {
	if len(backlog) == 0 {
		return
	}
	c := s.colour
	pl := func(l string) { out.WriteString(l + "\n") }
	pl("\n" + c.b + "reported done, not closed" + c.z + "  " + c.dim + "— cleanup backlog; every row has a durable done marker" + c.z)
	for _, b := range backlog {
		panes := "no listed pane"
		if len(b.panes) > 0 {
			panes = strings.Join(b.panes, ", ")
		}
		command := strings.Replace(b.key, ":", " ", 1)
		pl("  " + c.warn + "!" + c.z + " " + keyCell(b.key) + c.warn + b.kind + " " + b.id + " still exists" + c.z)
		pl("      " + c.dim + "pane(s): " + panes + "; close this one with `hw done " + command + "`" + c.z)
	}
	pl("  " + c.dim + "·" + c.z + " " + c.dim + "batch recovery: `hw sweep --apply` closes only owned tabs/workspaces whose current task has a done marker" + c.z)
	pl("  " + c.dim + "·" + c.z + " " + c.dim + "finished-unreported panes are excluded by that marker gate and are never touched" + c.z)
}

func (s *statusRun) reportOrphans(out *bufio.Writer, orphans []*orphanRec) {
	if len(orphans) == 0 {
		return
	}
	c := s.colour
	pl := func(l string) { out.WriteString(l + "\n") }
	live := map[string]bool{}
	for _, p := range s.d.panes {
		if pyTruthy(p["pane_id"]) {
			live[pyStr(p["pane_id"])] = true
		}
	}
	pl("\n" + c.b + "finished, never reported" + c.z + "  " + c.dim + "— the turn ended, the agent is gone, no brainer was told" + c.z)
	sorted := append([]*orphanRec(nil), orphans...)
	sort.SliceStable(sorted, func(i, j int) bool { return atKey(sorted[i]) > atKey(sorted[j]) })
	for _, o := range sorted {
		when := "at an unrecorded time"
		if o.hasAt && o.at != 0 {
			when = pyStrftimeDHM(o.at)
		}
		word := "turns"
		if o.turns == 1 {
			word = "turn"
		}
		pl("  " + c.warn + "!" + c.z + " " + keyCell(o.key) + c.warn + pyLjust(itoa(o.turns)+" "+word+", last ended "+when, 16) + c.z)
		pl("      " + c.dim + "why it never reported: " + orphanWhy(o, live) + c.z)
		var alive []string
		for _, p := range o.livePanes {
			if p != "" {
				alive = append(alive, p)
			}
		}
		switch {
		case len(alive) > 0:
			pl("      " + c.dim + "report: pane " + strings.Join(alive, ", ") + " is still open — the summary it could not send is in that pane" + c.z)
		case o.execPane != "":
			pl("      " + c.dim + "report: its pane " + o.execPane + " (from receipt.jsonl) is gone and no session id was recorded, so the pane copy is lost" + c.z)
		default:
			pl("      " + c.dim + "report: no pane, and no pane recorded in receipt.jsonl" + c.z)
		}
		lane := strings.SplitN(o.key, ":", 2)[0]
		engram, ok := s.d.engramByLane[lane]
		if !ok {
			engram = lane
		}
		pl("      " + c.dim + "also: " + shapeOf(o.workdir) + " in " + o.workdir + ", and whatever it saved to engram under `" + engram + "`" + c.z)
	}
	pl("  " + c.dim + "·" + c.z + " " + c.dim + "recover or drop each one, then `hw done <project> <task>` takes it off the board" + c.z)
}

func atKey(o *orphanRec) float64 {
	if !o.hasAt {
		return 0
	}
	return o.at
}

// writeOrphansJSON is the machine-readable copy of that queue, for the brainer's
// SessionStart (setup/guards/lane_housekeeping.py).
func (s *statusRun) writeOrphansJSON(path string, orphans []*orphanRec) {
	live := map[string]bool{}
	for _, p := range s.d.panes {
		if pyTruthy(p["pane_id"]) {
			live[pyStr(p["pane_id"])] = true
		}
	}
	var list []jmapOrdered
	for _, o := range orphans {
		var at any
		if o.hasAt {
			at = o.at
		}
		list = append(list, jmapOrdered{{"key", o.key}, {"at", at}, {"turns", o.turns}, {"why", orphanWhy(o, live)}})
	}
	body := pyJSONDump(list)
	if list == nil {
		body = "[]"
	}
	os.WriteFile(path, []byte(body), 0o666)
}

func (s *statusRun) reportCaveats(out *bufio.Writer, limits map[string]bool) {
	c := s.colour
	pl := func(l string) { out.WriteString(l + "\n") }
	dim := func(l string) { pl("      " + c.dim + l + c.z) }
	if !s.d.herdrOK {
		pl("\n  " + c.warn + "!" + c.z + " herdr is unreachable, so every row above is disk evidence only:")
		dim("a done marker still means done; nothing else on this list means anything")
	} else if len(limits) > 0 {
		pl("\n  " + c.dim + "what this cannot tell you" + c.z)
		if limits["exit"] {
			dim("pane_exited carries a pane id and nothing else — no exit code, and a dead")
			dim("pane returns null from `pane get`. A DIED row means the process is gone.")
			dim("Whether it succeeded, failed or was killed is not recorded anywhere.")
		}
		if limits["session"] {
			dim("the event backlog starts when the herdr server does. For a task older")
			dim("than that, hw says `no information` rather than reporting it as fine.")
		}
		if limits["orphan"] {
			dim("`finished, unreported` is three disk facts agreeing: no done marker, at least")
			dim("one turn END watched by the harness, and no live agent. It cannot tell a task")
			dim("that finished from one killed after a turn — a Stop hook never could. What it")
			dim("does establish is that nothing is running and nobody was told.")
		}
		if limits["stale"] {
			dim("`STALE` is computed when you read it, from the last turn end, the run's")
			dim("state files, the newest file in its work directory and the newest process")
			dim("started there. herdr exposes no last-output time, so pane output is not a")
			dim("signal, and one long step (a test run that started earlier) is not seen.")
		}
		if limits["first-turn"] {
			dim("`DIED-BEFORE-FIRST-TURN` is: started in this herdr session, pane gone, no done")
			dim("marker, no turn end ever counted. A `hw done` with no report reads the same.")
		}
		if limits["model"] {
			dim("`model` is what the executor's own session record says answered, read at")
			dim("launch. A divergence is against what hw handed the vendor (the lane's pin")
			dim("when there is one). It is not re-read afterwards: `/model` inside the pane")
			dim("changes what runs and this row will not know.")
		}
		if limits["turn"] {
			dim("`idle, silent` comes from the agent's turn-end adapter, which fires at the end")
			dim("of a TURN. It cannot tell a finished task from a finished turn, so it says")
			dim("only that the turn ended and done-invoker was not called. The executor may")
			dim("be waiting on an ask-invoker reply, or thinking, or quietly finished.")
			dim("`done-STRANDED` / `blocked-STRANDED` is the OTHER shape and it is not")
			dim("ambiguous: the executor DID publish a report, the delivery did NOT land,")
			dim("and its turn is over — so nothing inside that pane is going to retry it.")
			dim("`hw wait` on such a pane exits 7 without starting a wait, for the same")
			dim("reason: no budget reaches idle from there.")
		}
	}
	pl("")
	pl("  " + c.dim + "restarting herdr? use `herdr update --handoff` — it keeps pane processes alive" + c.z)
	pl("  " + c.dim + "(environment, dev servers and --model intact). A cold restart resumes the" + c.z)
	pl("  " + c.dim + "agents only, from .hw/<run>/env; the dev servers come back as bare shells." + c.z)
	pl("")
}
