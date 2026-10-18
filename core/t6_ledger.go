package main

// hw ledger — the port of cmd_ledger and its Python reader (lib/hw/ledger.sh), plus _picker_rows.
//
// The rest of lib/hw/ledger.sh is ALREADY in Go and is reused, not copied: _ledger_dispatch and
// _ledger_commit_brief (t5l_receipt.go: ledgerDispatch / ledgerCommitBrief), _ledger_next (t4_next_run.go:
// nextLedger), _briefless_gate / _nearest / _archive_hint (t5_brief.go). What `hw ledger` needs and Go lacked
// is the reader: for each brief, never-dispatched, in-progress or done, from the ledger plus the run's done
// marker, with the done marker written back the first time it is seen.
//
// The reader is Python in the bash, so what it does with a JSON line, a number, a path or a missing key is
// what this does (json.loads, json.dumps with compact separators, os.path.*, glob, str.splitlines).

import (
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const ledgerHelp = `hw ledger [<lane>] [--brief <path> | --task <task>] [--state <state>] [--json]

  One row per brief (and per dispatch with no brief): never-dispatched, in-progress or done,
  from the dispatch ledger ($WORK/.hw-ledger/<lane>.jsonl) plus the run's done marker.
  --state never-dispatched|in-progress|done   keep only that state
  --json                                      one JSON object per row
`

// ledgerNow is time.time(); the tests move it.
var ledgerNow = time.Now

func (a *app) cmdLedger() int {
	lane, brief, task, state := "", "", "", ""
	asJSON := false
	args := a.args
	for len(args) > 0 {
		switch args[0] {
		case "--brief", "--task", "--state":
			if len(args) < 2 { // _need_val
				a.die(args[0] + " needs a value")
			}
			switch args[0] {
			case "--brief":
				brief = args[1]
			case "--task":
				task = args[1]
			default:
				state = args[1]
			}
			args = args[2:]
		case "--json":
			asJSON = true
			args = args[1:]
		case "-h", "--help":
			a.printf("%s", ledgerHelp)
			return 0
		default:
			if strings.HasPrefix(args[0], "-") {
				a.die("unknown option: " + args[0] + " (usage: hw ledger [<lane>] [--brief <path>|--task <t>] [--state <s>] [--json])")
			}
			if lane != "" {
				a.die("hw ledger takes one lane (got: " + lane + ", " + args[0] + ")")
			}
			lane = a.c.t.canonProject(args[0])
			args = args[1:]
		}
	}
	switch state {
	case "", "never-dispatched", "in-progress", "done":
	default:
		a.die("--state must be never-dispatched, in-progress or done (got: " + state + ")")
	}
	if brief != "" && task != "" {
		a.die("--brief and --task name the same row two ways; pass one")
	}
	if brief != "" {
		if !isFile(brief) {
			a.die("no such brief: " + brief)
		}
		d, _ := lrcPhys(nextDirname(brief)) // $(cd "$(dirname "$brief")" && pwd -P)
		brief = d + "/" + nextBasename(brief)
	}
	lanes := a.c.t.names
	if lane != "" {
		lanes = []string{lane}
	}
	ldir := a.nextEnvOr("HW_LEDGER_DIR", nextOr(a.c.work, a.c.home+"/work")+"/.hw-ledger")
	rows := ledgerCollect(pyRealpath(a.c.brain), ldir, lanes, brief, task, state)
	for _, r := range rows {
		if asJSON {
			a.printf("%s\n", r.json())
		} else {
			a.printf("%s\n", r.text())
		}
	}
	return 0
}

// ── the ledger's lines, as json.loads reads them ─────────────────────────────

// ledgerRecs is load(): the dispatch lines and the done events of one lane, in file order. ok is false where
// the file cannot be opened (Python returns path None, so nothing is written back).
func ledgerRecs(path string) (rows []*jval, seen map[string]*jval, ok bool) {
	seen = map[string]*jval{}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, seen, false
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n") // universal newlines
	for _, ln := range strings.Split(text, "\n") {
		r, good := ledgerLoads(ln)
		if !good || r.kind != vObj { // a line that is not an object crashes the Python; it is skipped here
			continue
		}
		if ev := ledgerGet(r, "event"); ev.kind == vStr && ev.s == "done" {
			da := ledgerGet(r, "done_at")
			if !ledgerTruthy(da) {
				da = ledgerGet(r, "ts")
			}
			seen[ledgerRunSeqKey(r)] = da
		} else if ledgerHas(r, "task") {
			rows = append(rows, r)
		}
	}
	return rows, seen, true
}

func ledgerHas(o *jval, k string) bool {
	for _, m := range o.obj {
		if m.k == k {
			return true
		}
	}
	return false
}

// ledgerGet is r.get(k): None where the key is missing.
func ledgerGet(o *jval, k string) *jval {
	for _, m := range o.obj {
		if m.k == k {
			return m.v
		}
	}
	return jnull
}

func ledgerSeq(o *jval) *jval { // r.get("seq", 1)
	if ledgerHas(o, "seq") {
		return ledgerGet(o, "seq")
	}
	return &jval{kind: vNum, s: "1"}
}

func ledgerRunSeqKey(o *jval) string {
	return ledgerKey(ledgerGet(o, "run")) + "|" + ledgerKey(ledgerSeq(o))
}

// ledgerKey makes a value usable as a dict key: equal Python values give equal keys (1 == 1.0 == True).
func ledgerKey(v *jval) string {
	switch v.kind {
	case vNull:
		return "z"
	case vBool:
		if v.b {
			return "n1"
		}
		return "n0"
	case vNum:
		if f, err := strconv.ParseFloat(v.s, 64); err == nil || math.IsInf(f, 0) {
			return "n" + strconv.FormatFloat(f, 'g', -1, 64)
		}
		return "n" + v.s // NaN, Infinity
	case vStr:
		return "s" + v.s
	}
	return "j" + ledgerDump(v)
}

// ledgerTruthy is Python's truthiness of a JSON value.
func ledgerTruthy(v *jval) bool {
	switch v.kind {
	case vNull:
		return false
	case vBool:
		return v.b
	case vNum:
		f, err := strconv.ParseFloat(v.s, 64)
		return err != nil || f != 0
	case vStr:
		return v.s != ""
	case vArr:
		return len(v.arr) > 0
	}
	return len(v.obj) > 0
}

// ledgerPyStr is str(v) for what a "%s" prints.
func ledgerPyStr(v *jval) string {
	switch v.kind {
	case vNull:
		return "None"
	case vBool:
		if v.b {
			return "True"
		}
		return "False"
	case vNum:
		if !strings.ContainsAny(v.s, ".eE") {
			if strings.Trim(v.s, "-0") == "" {
				return "0"
			}
			return v.s
		}
		f, _ := strconv.ParseFloat(v.s, 64)
		return pyFloatRepr(f)
	case vStr:
		return v.s
	}
	var sb strings.Builder
	spPyJSON(&sb, v)
	return sb.String()
}

// ledgerDump is json.dumps(v, separators=(",", ":")): ensure_ascii, compact.
func ledgerDump(v *jval) string {
	var sb strings.Builder
	ledgerDumpTo(&sb, v)
	return sb.String()
}

func ledgerDumpTo(sb *strings.Builder, v *jval) {
	switch v.kind {
	case vArr:
		sb.WriteByte('[')
		for i, e := range v.arr {
			if i > 0 {
				sb.WriteByte(',')
			}
			ledgerDumpTo(sb, e)
		}
		sb.WriteByte(']')
	case vObj:
		sb.WriteByte('{')
		for i, m := range v.obj {
			if i > 0 {
				sb.WriteByte(',')
			}
			spPyString(sb, m.k)
			sb.WriteByte(':')
			ledgerDumpTo(sb, m.v)
		}
		sb.WriteByte('}')
	case vNum:
		switch v.s {
		case "NaN":
			sb.WriteString("NaN")
		case "Infinity", "-Infinity":
			sb.WriteString(v.s)
		default:
			sb.WriteString(spPyNumber(v.s))
		}
	default:
		spPyJSON(sb, v)
	}
}

// ledgerLoads is json.loads(s): false where Python raises ValueError. Like Python it takes NaN, Infinity and
// -Infinity, refuses control characters inside a string and anything after the value, and the last of two
// equal keys wins at the first one's place.
func ledgerLoads(s string) (*jval, bool) {
	p := &ledgerParser{s: s}
	p.ws()
	v, ok := p.value()
	if !ok {
		return nil, false
	}
	p.ws()
	if p.i != len(p.s) {
		return nil, false
	}
	return v, true
}

type ledgerParser struct {
	s string
	i int
}

func (p *ledgerParser) ws() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\n' || p.s[p.i] == '\r') {
		p.i++
	}
}

func (p *ledgerParser) lit(w string) bool {
	if strings.HasPrefix(p.s[p.i:], w) {
		p.i += len(w)
		return true
	}
	return false
}

func (p *ledgerParser) value() (*jval, bool) {
	if p.i >= len(p.s) {
		return nil, false
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		str, ok := p.str()
		return jstr(str), ok
	case c == 'n':
		return jnull, p.lit("null")
	case c == 't':
		return &jval{kind: vBool, b: true}, p.lit("true")
	case c == 'f':
		return &jval{kind: vBool}, p.lit("false")
	case c == 'N':
		return &jval{kind: vNum, s: "NaN"}, p.lit("NaN")
	case c == 'I':
		return &jval{kind: vNum, s: "Infinity"}, p.lit("Infinity")
	case c == '-' && strings.HasPrefix(p.s[p.i:], "-Infinity"):
		p.i += len("-Infinity")
		return &jval{kind: vNum, s: "-Infinity"}, true
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return nil, false
}

func (p *ledgerParser) digits() int {
	n := 0
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
		n++
	}
	return n
}

// number is `(-?(?:0|[1-9]\d*))(\.\d+)?([eE][-+]?\d+)?`.
func (p *ledgerParser) number() (*jval, bool) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	if p.i >= len(p.s) {
		return nil, false
	}
	switch {
	case p.s[p.i] == '0':
		p.i++
	case p.s[p.i] >= '1' && p.s[p.i] <= '9':
		p.digits()
	default:
		return nil, false
	}
	if p.i+1 < len(p.s) && p.s[p.i] == '.' && p.s[p.i+1] >= '0' && p.s[p.i+1] <= '9' {
		p.i++
		p.digits()
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		save := p.i
		p.i++
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		if p.digits() == 0 {
			p.i = save
		}
	}
	return &jval{kind: vNum, s: p.s[start:p.i]}, true
}

func (p *ledgerParser) str() (string, bool) {
	p.i++ // the opening quote
	var sb strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return sb.String(), true
		case c < 0x20:
			return "", false
		case c != '\\':
			sb.WriteByte(c)
			p.i++
			continue
		}
		p.i++
		if p.i >= len(p.s) {
			return "", false
		}
		e := p.s[p.i]
		p.i++
		switch e {
		case '"', '\\', '/':
			sb.WriteByte(e)
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'n':
			sb.WriteByte('\n')
		case 'r':
			sb.WriteByte('\r')
		case 't':
			sb.WriteByte('\t')
		case 'u':
			r, ok := ledgerHex4(p.s, p.i)
			if !ok {
				return "", false
			}
			p.i += 4
			if r >= 0xd800 && r < 0xdc00 && strings.HasPrefix(p.s[p.i:], `\u`) {
				if lo, ok := ledgerHex4(p.s, p.i+2); ok && lo >= 0xdc00 && lo < 0xe000 {
					r = 0x10000 + (r-0xd800)<<10 + (lo - 0xdc00)
					p.i += 6
				}
			}
			if r >= 0xd800 && r < 0xe000 { // a lone surrogate has no UTF-8 form
				r = utf8.RuneError
			}
			sb.WriteRune(rune(r))
		default:
			return "", false
		}
	}
	return "", false
}

func ledgerHex4(s string, i int) (int, bool) {
	if i+4 > len(s) {
		return 0, false
	}
	n := 0
	for _, c := range []byte(s[i : i+4]) {
		switch {
		case c >= '0' && c <= '9':
			n = n<<4 | int(c-'0')
		case c >= 'a' && c <= 'f':
			n = n<<4 | int(c-'a'+10)
		case c >= 'A' && c <= 'F':
			n = n<<4 | int(c-'A'+10)
		default:
			return 0, false
		}
	}
	return n, true
}

func (p *ledgerParser) array() (*jval, bool) {
	p.i++
	v := &jval{kind: vArr}
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return v, true
	}
	for {
		p.ws()
		e, ok := p.value()
		if !ok {
			return nil, false
		}
		v.arr = append(v.arr, e)
		p.ws()
		if p.i >= len(p.s) {
			return nil, false
		}
		c := p.s[p.i]
		p.i++
		if c == ']' {
			return v, true
		}
		if c != ',' {
			return nil, false
		}
	}
}

func (p *ledgerParser) object() (*jval, bool) {
	p.i++
	v := &jval{kind: vObj}
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return v, true
	}
	for {
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return nil, false
		}
		k, ok := p.str()
		if !ok {
			return nil, false
		}
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, false
		}
		p.i++
		p.ws()
		e, ok := p.value()
		if !ok {
			return nil, false
		}
		dup := false
		for i := range v.obj {
			if v.obj[i].k == k {
				v.obj[i].v, dup = e, true
				break
			}
		}
		if !dup {
			v.obj = append(v.obj, member{k, e})
		}
		p.ws()
		if p.i >= len(p.s) {
			return nil, false
		}
		c := p.s[p.i]
		p.i++
		if c == '}' {
			return v, true
		}
		if c != ',' {
			return nil, false
		}
	}
}

// ── the rows ─────────────────────────────────────────────────────────────────

type ledgerRow struct {
	state, lane string
	task        *jval
	m           []member // the JSON object, in the order the Python builds it
	archived    bool
}

func (r *ledgerRow) get(k string) *jval {
	for _, m := range r.m {
		if m.k == k {
			return m.v
		}
	}
	return jnull
}

func (r *ledgerRow) json() string { return ledgerDump(&jval{kind: vObj, obj: r.m}) }

// text is `"%-16s %-14s %s%s%s"`.
func (r *ledgerRow) text() string {
	extra := ""
	if r.state != "never-dispatched" {
		ts := ""
		if t := r.get("ts"); ledgerTruthy(t) {
			ts = ledgerPyStr(t)
		}
		if rs := []rune(ts); len(rs) > 16 {
			ts = string(rs[:16])
		}
		mv := "-"
		if m := r.get("model"); ledgerTruthy(m) {
			mv = ledgerPyStr(m)
		} else if v := r.get("vendor"); ledgerTruthy(v) {
			mv = ledgerPyStr(v)
		}
		run := ""
		if v := r.get("run"); ledgerTruthy(v) {
			run = ledgerPyStr(v)
		}
		extra = "  " + ts + "  " + mv + "  " + run
	}
	suffix := ""
	if r.archived {
		suffix = "  (archived)"
	}
	return pyLjust(r.state, 16) + " " + pyLjust(r.lane, 14) + " " + ledgerPyStr(r.task) + extra + suffix
}

func ledgerIso(sec int64) string { return time.Unix(sec, 0).UTC().Format("2006-01-02T15:04:05Z") }

// ledgerJudge is judge(): the state of the lane's last dispatch of a task, from the run directory's markers and
// from what the ledger recorded the first time it saw one. A done marker seen for the first time is queued for
// write-back.
func ledgerJudge(r *jval, seen map[string]*jval, lane string, pending *[]*jval) (state, evidence string, at *jval) {
	key := ledgerRunSeqKey(r)
	seq := ledgerSeq(r)
	rd := ""
	if v := ledgerGet(r, "rundir"); ledgerTruthy(v) {
		rd = ledgerPyStr(v)
	}
	d := rd
	sf := 1.0
	switch seq.kind {
	case vNum:
		sf, _ = strconv.ParseFloat(seq.s, 64)
	case vBool:
		if !seq.b {
			sf = 0
		}
	}
	if sf > 1 {
		d = joinPath(rd, "t"+strconv.FormatInt(int64(sf), 10))
	}
	if rd != "" && pyIsFile(joinPath(d, "reopened")) {
		return "in-progress", "reopened", jnull
	}
	if rd != "" && pyIsFile(joinPath(d, "done")) {
		mt, _ := pyMtime(joinPath(d, "done"))
		atS := ledgerIso(int64(math.Floor(mt)))
		if _, ok := seen[key]; !ok {
			*pending = append(*pending, &jval{kind: vObj, obj: []member{
				{"v", &jval{kind: vNum, s: "1"}}, {"event", jstr("done")}, {"lane", jstr(lane)},
				{"task", ledgerGet(r, "task")}, {"run", ledgerGet(r, "run")}, {"seq", seq},
				{"ts", jstr(ledgerIso(ledgerNow().Unix()))}, {"done_at", jstr(atS)},
			}})
			seen[key] = jstr(atS)
		}
		return "done", "done marker", jstr(atS)
	}
	if v, ok := seen[key]; ok {
		return "done", "reported (recorded earlier)", v
	}
	if rd != "" && !pyIsDir(rd) {
		return "done", "workdir removed", jnull
	}
	return "in-progress", "no done marker", jnull
}

// ledgerGlobMD is sorted(glob.glob(dir + "/*.md")): no dotfiles, code point order.
func ledgerGlobMD(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".md") {
			continue
		}
		out = append(out, joinPath(dir, n))
	}
	sort.Strings(out)
	return out
}

func ledgerDispatchedRow(lane string, task *jval, st, ev string, brief *jval, hist []*jval, at *jval) *ledgerRow {
	last := hist[len(hist)-1]
	return &ledgerRow{state: st, lane: lane, task: task, m: []member{
		{"lane", jstr(lane)}, {"task", task}, {"state", jstr(st)}, {"evidence", jstr(ev)}, {"brief", brief},
		{"dispatches", &jval{kind: vNum, s: strconv.Itoa(len(hist))}},
		{"run", ledgerGet(last, "run")}, {"ts", ledgerGet(last, "ts")}, {"done_at", at},
		{"model", ledgerGet(last, "model")}, {"vendor", ledgerGet(last, "vendor")},
		{"account", ledgerGet(last, "account")}, {"effort", ledgerGet(last, "effort")},
		{"brief_sha", ledgerGet(last, "brief_sha")}, {"brief_commit", ledgerGet(last, "brief_commit")},
		{"subagent_model", ledgerGet(last, "subagent_model")}, {"pane", ledgerGet(last, "pane")},
	}}
}

// ledgerCollect is the body of the Python: every lane's rows (briefs, archived briefs, then dispatches that
// point at no brief), the done markers written back, and --state applied last.
func ledgerCollect(brain, ldir string, lanes []string, onlyBrief, onlyTask, onlyState string) []*ledgerRow {
	var out []*ledgerRow
	for _, lane := range lanes {
		path := joinPath(ldir, lane+".jsonl")
		recs, seen, ok := ledgerRecs(path)
		var pending []*jval
		byTask := map[string][]*jval{}
		var order []string
		for _, r := range recs {
			k := ledgerKey(ledgerGet(r, "task"))
			if _, dup := byTask[k]; !dup {
				order = append(order, k)
			}
			byTask[k] = append(byTask[k], r)
		}
		listed := map[string]bool{}
		bdir := joinPath(brain, lane, "briefs")
		files := ledgerGlobMD(bdir)
		var afiles []string
		if ents, err := os.ReadDir(joinPath(bdir, "archive")); err == nil {
			for _, e := range ents {
				if !strings.HasPrefix(e.Name(), ".") {
					afiles = append(afiles, ledgerGlobMD(joinPath(bdir, "archive", e.Name()))...)
				}
			}
		}
		sort.Strings(afiles)
		isArch := map[string]bool{}
		for _, f := range afiles {
			isArch[f] = true
		}
		flat := map[string]bool{}
		for _, f := range files {
			flat[strings.TrimSuffix(nextBasename(f), ".md")] = true
		}
		for _, f := range append(append([]string{}, files...), afiles...) {
			name := strings.TrimSuffix(nextBasename(f), ".md")
			archived := isArch[f]
			if strings.HasPrefix(name, "_") || (archived && (flat[name] || listed[name])) {
				continue
			}
			if onlyBrief != "" && pyRealpath(f) != onlyBrief {
				continue
			}
			if onlyTask != "" && name != onlyTask {
				continue
			}
			hist := byTask[ledgerKey(jstr(name))]
			listed[name] = true
			if len(hist) == 0 {
				row := &ledgerRow{state: "never-dispatched", lane: lane, task: jstr(name), archived: archived, m: []member{
					{"lane", jstr(lane)}, {"task", jstr(name)}, {"state", jstr("never-dispatched")}, {"brief", jstr(f)},
				}}
				if archived {
					row.m = append(row.m, member{"archived", &jval{kind: vBool, b: true}})
				}
				out = append(out, row)
				continue
			}
			st, ev, at := ledgerJudge(hist[len(hist)-1], seen, lane, &pending)
			row := ledgerDispatchedRow(lane, jstr(name), st, ev, jstr(f), hist, at)
			if archived {
				row.archived = true
				row.m = append(row.m, member{"archived", &jval{kind: vBool, b: true}})
			}
			out = append(out, row)
		}
		// dispatches that point at no brief in this lane's briefs/ (--no-brief, --brief elsewhere)
		for _, k := range order {
			hist := byTask[k]
			task := ledgerGet(hist[0], "task")
			if (task.kind == vStr && listed[task.s]) || onlyBrief != "" {
				continue
			}
			if onlyTask != "" && !(task.kind == vStr && task.s == onlyTask) {
				continue
			}
			last := hist[len(hist)-1]
			st, ev, at := ledgerJudge(last, seen, lane, &pending)
			out = append(out, ledgerDispatchedRow(lane, task, st, ev, ledgerGet(last, "brief"), hist, at))
		}
		if len(pending) > 0 && ok {
			if fh, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o666); err == nil {
				for _, p := range pending {
					if _, err := fh.WriteString(ledgerDump(p) + "\n"); err != nil {
						break
					}
				}
				fh.Close()
			}
		}
	}
	if onlyState != "" {
		var kept []*ledgerRow
		for _, r := range out {
			if r.state == onlyState {
				kept = append(kept, r)
			}
		}
		out = kept
	}
	return out
}

// ── the picker's rows (_picker_rows) ─────────────────────────────────────────

// ledgerPickerRows is _picker_rows: lane TAB task TAB title for every brief of the lanes, from the `# title`,
// else the first prose line after the frontmatter. `_`-prefixed files are not tasks.
func ledgerPickerRows(brain string, lanes []string) string {
	var sb strings.Builder
	for _, lane := range lanes {
		for _, f := range ledgerGlobMD(joinPath(brain, lane, "briefs")) {
			name := strings.TrimSuffix(nextBasename(f), ".md")
			if strings.HasPrefix(name, "_") {
				continue
			}
			sb.WriteString(lane + "\t" + name + "\t" + strings.ReplaceAll(ledgerPickerTitle(f), "\t", " ") + "\n")
		}
	}
	return sb.String()
}

func ledgerClip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func ledgerPickerTitle(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(unreadable)"
	}
	lines := pySplitlines(string([]rune(string(b)))) // errors="replace", universal newlines, splitlines
	i := 0
	for i < len(lines) && pyStrip(lines[i]) == "" {
		i++
	}
	if i < len(lines) && pyStrip(lines[i]) == "---" { // bash anchor 811-M03 (registered on the bash source)
		j := i + 1
		for j < len(lines) && pyStrip(lines[j]) != "---" {
			j++
		}
		if j < len(lines) {
			i = j + 1
		} else {
			i = len(lines)
		}
	}
	prose, fence := "", false
	for _, ln := range lines[i:] {
		s := pyStrip(ln)
		if strings.HasPrefix(s, "```") {
			fence = !fence
			continue
		}
		if fence || s == "" {
			continue
		}
		if strings.HasPrefix(s, "#") {
			if t := pyStrip(strings.TrimLeft(s, "#")); t != "" {
				return ledgerClip(t, 60)
			}
			continue
		}
		if prose == "" && s != "---" {
			prose = s
		}
	}
	if prose == "" {
		prose = "(empty)"
	}
	return ledgerClip(prose, 60)
}
