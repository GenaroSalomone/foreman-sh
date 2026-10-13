package main

// Area B1 — where the executor lives: workspace, tab, layout, the pane beside the brainer (bin/hw: _build_space,
// _layout_apply, _render_layout, _layout_env_union, _project_workspace, _layout_failed_advice, _pane_by_label,
// _rightmost_pane_in_tab, _run_here, _ws_id_by_label, _tab_id_by_label, _live_agent_panes_in_tab/_workspace,
// _offset). OWNER: the B1 track.
//
// Every herdr lookup the bash pipes through `jq` is read here with the jval reader (jsonjq.go), with the jq
// behaviours the bash relied on: `//` swallows an error on its left, `-r` prints null as "null", a value that
// errors stops that input only, and `2>/dev/null || true` turns every failure into "nothing printed".

import (
	"math"
	"os"
	"strconv"
	"strings"
)

type lspState struct {
	projectWorkspace            string // PROJECT_WORKSPACE
	projectWorkspaceCreateReply string // PROJECT_WORKSPACE_CREATE_REPLY
}

// ── small jq stand-ins ──────────────────────────────────────────────────────

// spPath is `.a.b.c` over one value: the member, or ok == false where jq would raise.
func spPath(v *jval, keys ...string) (*jval, bool) {
	cur := v
	for _, k := range keys {
		var ok bool
		if cur, ok = cur.get(k); !ok {
			return nil, false
		}
	}
	return cur, true
}

// spIter is `.[]`: the elements of an array, the values of an object, ok == false for anything else.
func spIter(v *jval) ([]*jval, bool) {
	switch v.kind {
	case vArr:
		return v.arr, true
	case vObj:
		out := make([]*jval, 0, len(v.obj))
		for _, m := range v.obj {
			out = append(out, m.v)
		}
		return out, true
	}
	return nil, false
}

// spAlt is `jq -r 'p1 // p2 // … // <final>'` over a stream, final being "empty" or the last path's own value
// (null prints as "null"). Errors on the left of a `//` fall through to the next alternative.
func spAlt(text string, finalEmpty bool, paths ...[]string) string {
	vals, _ := parseStream([]byte(text))
	var outs []string
	for _, v := range vals {
		for i, p := range paths {
			cur, ok := spPath(v, p...)
			last := i == len(paths)-1
			if ok && (cur.truthy() || (last && !finalEmpty)) {
				outs = append(outs, cur.raw())
				break
			}
		}
	}
	return strings.Join(outs, "\n")
}

// spHeadLines is `head -n`: whole lines, the last one with its newline when it had one.
func spHeadLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "")
}

// spFirstLine is `| head -1` of a jq output.
func spFirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ── _offset ─────────────────────────────────────────────────────────────────

// offset is `_offset`: POSIX cksum of the task name (no newline) modulo 40.
func (l *lch) offset(task string) string {
	return strconv.FormatUint(uint64(spCksum([]byte(task)))%40, 10)
}

// spCksum is cksum(1): the CRC of the bytes then of the length (least significant byte first, until zero),
// complemented. Polynomial 0x04C11DB7, most significant bit first.
func spCksum(data []byte) uint32 {
	var table [256]uint32
	for i := 0; i < 256; i++ {
		c := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04C11DB7
			} else {
				c <<= 1
			}
		}
		table[i] = c
	}
	var crc uint32
	for _, b := range data {
		crc = crc<<8 ^ table[byte(crc>>24)^b]
	}
	for n := uint64(len(data)); n != 0; n >>= 8 {
		crc = crc<<8 ^ table[byte(crc>>24)^byte(n)]
	}
	return ^crc
}

// ── herdr lookups ───────────────────────────────────────────────────────────

// spListIDs is `herdr <kind> list 2>/dev/null | jq -r '.result.<arr>[] | select(.label == $l) | .<idKey>' | head -1`.
func (l *lch) spListIDs(kind, arr, idKey, label string) string {
	out, _ := l.herdr(false, true, "", kind, "list")
	vals, _ := parseStream([]byte(out))
	for _, v := range vals {
		list, ok := spPath(v, "result", arr)
		if !ok {
			continue
		}
		elems, ok := spIter(list)
		if !ok {
			continue
		}
		for _, e := range elems {
			lab, ok := e.get("label")
			if !ok {
				break
			}
			if lab.kind == vStr && lab.s == label {
				id, _ := e.get(idKey)
				return spFirstLine(id.raw())
			}
		}
	}
	return ""
}

func (l *lch) wsIDByLabel(label string) string {
	return l.spListIDs("workspace", "workspaces", "workspace_id", label)
}

// tabIDByLabel: a tab label is a real handle, unlike a pane's (`tab list` returns it).
func (l *lch) tabIDByLabel(label string) string {
	return l.spListIDs("tab", "tabs", "tab_id", label)
}

// spLiveAgents is `herdr pane list 2>/dev/null | jq -r '[.result.panes[] | select(.<key> == $id and .agent != null)] | length' 2>/dev/null || printf '0'`.
func (l *lch) spLiveAgents(key, id string) string {
	out, rc := l.herdr(false, true, "", "pane", "list")
	failed := rc != 0 // pipefail
	vals, perr := parseStream([]byte(out))
	if perr != nil {
		failed = true
	}
	var printed []string
	for _, v := range vals {
		list, ok := spPath(v, "result", "panes")
		var elems []*jval
		if ok {
			elems, ok = spIter(list)
		}
		n := 0
		for _, e := range elems {
			if !ok {
				break
			}
			k, kok := e.get(key)
			if !kok {
				ok = false
				break
			}
			if k.kind == vStr && k.s == id {
				if a, _ := e.get("agent"); a.kind != vNull {
					n++
				}
			}
		}
		if !ok {
			failed = true
			continue
		}
		printed = append(printed, strconv.Itoa(n))
	}
	res := strings.Join(printed, "\n")
	if len(printed) > 0 {
		res += "\n"
	}
	if failed {
		res += "0"
	}
	return bashSub(res)
}

// liveAgentPanesInTab counts the panes herdr reports an `agent` for inside a tab: a tab is not an agent.
func (l *lch) liveAgentPanesInTab(id string) string { return l.spLiveAgents("tab_id", id) }

func (l *lch) liveAgentPanesInWorkspace(id string) string { return l.spLiveAgents("workspace_id", id) }

// paneByLabel is `jq -r 'first(.. | objects | select(.type == "pane" and .label == $l) | .pane_id) // empty'`:
// pane ids come back BY LABEL, so hw never has to know the shape of the tree it sent.
func (l *lch) paneByLabel(doc, label string) string {
	vals, _ := parseStream([]byte(doc))
	var outs []string
	for _, v := range vals {
		if p := spFirstPane(v, label); p != nil {
			if id, _ := p.get("pane_id"); id.truthy() {
				outs = append(outs, id.raw())
			}
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

// spFirstPane is the first object, in `..` order, with type "pane" and the label.
func spFirstPane(v *jval, label string) *jval {
	if v.kind == vObj {
		t, _ := v.get("type")
		lab, _ := v.get("label")
		if t.kind == vStr && t.s == "pane" && lab.kind == vStr && lab.s == label {
			return v
		}
		for _, m := range v.obj {
			if r := spFirstPane(m.v, label); r != nil {
				return r
			}
		}
	}
	if v.kind == vArr {
		for _, e := range v.arr {
			if r := spFirstPane(e, label); r != nil {
				return r
			}
		}
	}
	return nil
}

// rightmostPaneInTab: the BIGGEST pane of the brainer's tab, tie-broken by rightmost. Splitting that instead of
// the brainer's own pane is the whole fix for a shrinking brainer (see bin/hw for the measurements).
func (l *lch) rightmostPaneInTab(pane string) string {
	out, _ := l.herdr(false, true, "", "pane", "layout", "--pane", pane)
	if out == "" { // # MUTATION-ANCHOR: 847-B1-01
		return ""
	}
	vals, _ := parseStream([]byte(out))
	var outs []string
	for _, v := range vals {
		panes, ok := spPath(v, "result", "layout", "panes")
		var elems []*jval
		if ok && panes.kind == vArr { // `[…[]?]`: anything else is no panes
			elems = panes.arr
		}
		var best *jval
		var bestArea, bestX float64
		bestXNull, bad := false, false
		for _, e := range elems {
			rect, ok := e.get("rect")
			if !ok {
				bad = true
				break
			}
			w, wok := spPath(rect, "width")
			h, hok := spPath(rect, "height")
			x, xok := spPath(rect, "x")
			if !wok || !hok || !xok || w.kind != vNum || h.kind != vNum || (x.kind != vNum && x.kind != vNull) {
				bad = true
				break
			}
			area := spNum(w) * spNum(h)
			xn := x.kind == vNull
			var xv float64
			if !xn {
				xv = spNum(x)
			}
			if best != nil {
				c := spCmpKey(area, xn, xv, bestArea, bestXNull, bestX)
				if c < 0 { // # MUTATION-ANCHOR: 847-B1-02
					continue // max_by: a later pane wins a tie (jq replaces when cmp >= 0)
				}
			}
			best, bestArea, bestXNull, bestX = e, area, xn, xv
		}
		if bad || best == nil {
			continue
		}
		if id, _ := best.get("pane_id"); id.truthy() {
			outs = append(outs, id.raw())
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

func spNum(v *jval) float64 {
	f, _ := strconv.ParseFloat(v.s, 64)
	return f
}

// spCmpKey compares the jq keys [area, x] (x null sorts before every number).
func spCmpKey(a1 float64, n1 bool, x1, a2 float64, n2 bool, x2 float64) int {
	switch {
	case a1 < a2:
		return -1
	case a1 > a2:
		return 1
	case n1 && n2:
		return 0
	case n1:
		return -1
	case n2:
		return 1
	case x1 < x2:
		return -1
	case x1 > x2:
		return 1
	}
	return 0
}

// layoutEnvUnion is `jq -c '[ .. | objects | select(.type == "pane") | (.env // {}) ] | add // {}'`: the union
// of every pane's env in a RENDERED document ("" where jq prints nothing).
func (l *lch) layoutEnvUnion(doc string) string {
	vals, _ := parseStream([]byte(doc))
	var outs []string
	for _, v := range vals {
		var merged []member
		bad := false
		var walk func(n *jval)
		walk = func(n *jval) {
			if bad {
				return
			}
			switch n.kind {
			case vObj:
				if t, _ := n.get("type"); t.kind == vStr && t.s == "pane" {
					env, _ := n.get("env")
					if env.truthy() {
						if env.kind != vObj {
							bad = true
							return
						}
						for _, m := range env.obj {
							set := false
							for i := range merged {
								if merged[i].k == m.k {
									merged[i].v, set = m.v, true
								}
							}
							if !set {
								merged = append(merged, m)
							}
						}
					}
				}
				for _, m := range n.obj {
					walk(m.v)
				}
			case vArr:
				for _, e := range n.arr {
					walk(e)
				}
			}
		}
		walk(v)
		if bad {
			continue
		}
		outs = append(outs, (&jval{kind: vObj, obj: merged}).toJSON())
	}
	return bashSub(strings.Join(outs, "\n"))
}

// layoutFailedAdvice is what to close after a failed layout — ONLY what this task created. In tab mode the
// workspace is the lane's and its brainer lives there.
func (l *lch) layoutFailedAdvice() string {
	if l.tab { // # MUTATION-ANCHOR: 847-B1-03
		return "tab " + l.hwTab + " was created in " + l.ws + " but has no layout — close that tab with: herdr tab close " + l.hwTab + "  (NOT the workspace: " + l.ws + " is the lane's, and its brainer lives there)"
	}
	return "space " + l.ws + " was created but has no layout — close it with: herdr workspace close " + l.ws + "  (hw created it for this task; nothing else lives there)"
}

// projectWorkspaceFor is `_project_workspace "$1"`: the project's workspace, created on demand and NEVER closed
// by hw. The creation reply is retained because its forced first tab belongs to the first task.
func (l *lch) projectWorkspaceFor(proj string) bool {
	l.projectWorkspace, l.projectWorkspaceCreateReply = "", ""
	if !l.c.t.known(proj) {
		l.warn("no workspace label for project '" + proj + "'")
		return false
	}
	label := l.c.t.get(proj, "space")
	if ws := l.wsIDByLabel(label); ws != "" {
		l.projectWorkspace = ws
		return true
	}
	raw, rc := l.herdrCapture(true, spWsCreateSynth, "workspace", "create", "--cwd", l.workdir(), "--label", label, "--no-focus")
	raw = bashSub(raw)
	if rc != 0 { // # MUTATION-ANCHOR: 847-B1-04
		l.warn("herdr workspace create failed for " + label + ": " + spHead1(raw))
		return false
	}
	ws := spAlt(raw, false, []string{"result", "workspace", "workspace_id"}, []string{"result", "workspace_id"})
	if ws == "" || ws == "null" {
		l.warn("herdr workspace create returned no id for " + label)
		return false
	}
	l.projectWorkspace = ws
	l.projectWorkspaceCreateReply = raw
	return true
}

func spHead1(s string) string { return bashSub(spHeadLines(s, 1)) }

const spWsCreateSynth = `{"result":{"workspace":{"workspace_id":"shadow:w1"},"tab":{"tab_id":"shadow:t1"}}}`

// workdir is $HW_WORKDIR; the run id is $HW_RUN.
func (l *lch) workdir() string {
	if v := l.env("HW_WORKDIR"); v != "" {
		return v
	}
	return l.hwWorkdir
}

func (l *lch) runID() string {
	if v := l.env("HW_RUN"); v != "" {
		return v
	}
	return l.hwRun
}

// ── the layout ──────────────────────────────────────────────────────────────

// layoutApply is `_layout_apply`: render the lane's document and hand it to herdr in ONE call. It returns the
// reply and the status. The rendered document is returned too, but bash assigns LAYOUT_DOC inside the
// `$(…)` that calls this, so the caller's LAYOUT_DOC stays "" (see buildSpace).
func (l *lch) layoutApply(tpl string) (string, string, int) {
	doc, rc := l.renderLayout(tpl)
	if rc != 0 { // # MUTATION-ANCHOR: 847-B1-05
		return "", "", 1
	}
	out, rc := l.sh(cmdSpec{name: l.binDir() + "/herdr-rpc", args: []string{"layout-apply", "-"}, stdin: doc, mut: true, merge: true, synth: spLayoutSynth(doc)})
	return bashSub(out), doc, rc
}

// spLayoutSynth is the reply a read-only run answers for layout-apply: the panes of the document, by label, with
// ids a following `jq` lookup can use.
func spLayoutSynth(doc string) string {
	vals, _ := parseStream([]byte(doc))
	var panes []*jval
	n := 0
	var walk func(v *jval)
	walk = func(v *jval) {
		switch v.kind {
		case vObj:
			if t, _ := v.get("type"); t.kind == vStr && t.s == "pane" {
				lab, _ := v.get("label")
				n++
				panes = append(panes, &jval{kind: vObj, obj: []member{{"type", jstr("pane")}, {"label", lab}, {"pane_id", jstr("shadow:p" + strconv.Itoa(n))}}})
			}
			for _, m := range v.obj {
				walk(m.v)
			}
		case vArr:
			for _, e := range v.arr {
				walk(e)
			}
		}
	}
	for _, v := range vals {
		walk(v)
	}
	root := &jval{kind: vObj, obj: []member{{"tab_id", jstr("shadow:t1")}, {"panes", &jval{kind: vArr, arr: panes}}}}
	return (&jval{kind: vObj, obj: []member{{"layout", root}}}).toJSON()
}

// layoutBaseEnvKeys is the executor environment merged into every pane, in the order bin/hw's python lists it.
// Kept in code and NOT in the documents so that the day hw adds a variable, no lane silently misses it.
var layoutBaseEnvKeys = []string{"HW_PROJECT", "HW_TASK", "HW_WORKDIR", "HW_RUN", "HW_ARTIFACTS",
	"ENGRAM_PROJECT",
	"HW_EXECUTOR_VENDOR", "HW_INVOKER_VENDOR", "HW_INVOKER_SESSION",
	"HW_INVOKER_ENDPOINT", "HW_INVOKER_PANE", "HW_NO_REPORT",
	"HW_CHAINING_ENABLED", "HW_LAUNCH_MODE", "HW_NEXT_WAIT_MS",
	"HW_AGENT_PORT", "HW_AGENT_ENDPOINT",
	"HW_PORT_WEB", "HW_PORT_API",
	"HW_REVIEW_BASE_REF", "HW_REVIEW_BASE_BRANCH",
	"AGENT_BROWSER_NAMESPACE", "HW_BROWSER_PROFILE",
	"CLAUDE_CONFIG_DIR",               // # MUTATION-ANCHOR: 847-B1-06
	"CLAUDE_CODE_DISABLE_AUTO_MEMORY", // # MUTATION-ANCHOR: 847-B1-07
	"CLAUDE_CODE_SUBAGENT_MODEL",      // # MUTATION-ANCHOR: 847-B1-08
	"OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS",
	"OPENCODE_CONFIG_CONTENT", // # MUTATION-ANCHOR: 847-B1-09
	"HW_OPENCODE_EFFORT", "HW_OPENCODE_EFFORT_AGENT",
	"OPENCODE_CONFIG"}

// renderLayout is `_render_layout`: the template, with every `_`-prefixed key dropped, every ${VAR} replaced
// from the environment (after parsing, so a value with a quote cannot break the JSON), and the executor
// environment merged into every pane; written the way python's json.dump writes it. (bash runs `python3 -`;
// Go does the same work in-process.) Returns the document and the status.
func (l *lch) renderLayout(tpl string) (string, int) {
	data, err := os.ReadFile(tpl)
	if err != nil { // # MUTATION-ANCHOR: 847-B1-10
		l.warn("layout document not readable: " + tpl)
		return "", 1
	}
	vals, perr := parseStream(data)
	if perr != nil || len(vals) != 1 {
		msg := "Expecting value: line 1 column 1 (char 0)"
		if perr != nil {
			msg = perr.Error()
		} else if len(vals) > 1 {
			msg = "Extra data"
		}
		l.errf("%s: %s\n", tpl, msg)
		return "", 1
	}
	env := map[string]string{}
	for _, kv := range l.environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	r := &spLayoutRender{env: env}
	for _, k := range layoutBaseEnvKeys {
		if v := env[k]; v != "" { // os.environ.get(k): unset and empty are both left out
			r.baseEnv = append(r.baseEnv, member{k, jstr(v)})
		}
	}
	if v := env["OPENCODE_EXECUTOR_CONFIG"]; v != "" { // keyed on the lane's own declaration, not an inherited dir
		r.baseEnv = append(r.baseEnv, member{"OPENCODE_CONFIG_DIR", jstr(v)})
	}
	rendered := r.walk(vals[0])
	if r.fatal != "" {
		l.errf("%s\n", r.fatal)
		return "", 1
	}
	if len(r.missing) > 0 { // # MUTATION-ANCHOR: 847-B1-11
		names := make([]string, 0, len(r.missing))
		for k := range r.missing {
			names = append(names, k)
		}
		spSortStrings(names)
		l.errf("%s: unset placeholders: %s\n", tpl, strings.Join(names, ", "))
		return "", 1
	}
	var sb strings.Builder
	spPyJSON(&sb, rendered)
	return sb.String(), 0
}

func spSortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

type spLayoutRender struct {
	env     map[string]string
	baseEnv []member
	missing map[string]bool
	fatal   string
}

func (r *spLayoutRender) walk(n *jval) *jval {
	switch n.kind {
	case vObj:
		out := &jval{kind: vObj}
		for _, m := range n.obj {
			if strings.HasPrefix(m.k, "_") { // `_`-prefixed keys are this format's comments
				continue
			}
			spPut(out, m.k, r.walk(m.v))
		}
		if t, _ := out.get("type"); t.kind == vStr && t.s == "pane" {
			merged := &jval{kind: vObj, obj: append([]member(nil), r.baseEnv...)}
			if cur, _ := out.get("env"); spTruthy(cur) {
				if cur.kind != vObj {
					if r.fatal == "" {
						r.fatal = "TypeError: pane env is not an object"
					}
					return out
				}
				for _, m := range cur.obj {
					spPut(merged, m.k, m.v)
				}
			}
			spPut(out, "env", merged)
		}
		return out
	case vArr:
		out := &jval{kind: vArr}
		for _, e := range n.arr {
			out.arr = append(out.arr, r.walk(e))
		}
		return out
	case vStr:
		return jstr(r.subst(n.s))
	}
	return n
}

// spPut is a python dict assignment: an existing key keeps its place.
func spPut(o *jval, k string, v *jval) {
	for i := range o.obj {
		if o.obj[i].k == k {
			o.obj[i].v = v
			return
		}
	}
	o.obj = append(o.obj, member{k, v})
}

func spTruthy(v *jval) bool {
	switch v.kind {
	case vNull:
		return false
	case vBool:
		return v.b
	case vNum:
		f, _ := strconv.ParseFloat(v.s, 64)
		return f != 0
	case vStr:
		return v.s != ""
	case vArr:
		return len(v.arr) > 0
	}
	return len(v.obj) > 0
}

func spIDStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func spIDChar(c byte) bool  { return spIDStart(c) || c >= '0' && c <= '9' }

// subst is python's string.Template(s).substitute(os.environ), with a KeyError turned into "the string as it
// was" and a note of the name; an invalid placeholder is python's uncaught ValueError.
func (r *spLayoutRender) subst(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		switch {
		case j < len(s) && s[j] == '$':
			sb.WriteByte('$')
			i = j + 1
			continue
		case j < len(s) && spIDStart(s[j]):
			k := j
			for k < len(s) && spIDChar(s[k]) {
				k++
			}
			if !r.put(&sb, s[j:k]) {
				return s
			}
			i = k
			continue
		case j < len(s) && s[j] == '{' && j+1 < len(s) && spIDStart(s[j+1]):
			k := j + 1
			for k < len(s) && spIDChar(s[k]) {
				k++
			}
			if k < len(s) && s[k] == '}' {
				if !r.put(&sb, s[j+1:k]) {
					return s
				}
				i = k + 1
				continue
			}
		}
		if r.fatal == "" {
			line, col := 1, 1
			if pre := s[:j]; pre != "" { // python: splitlines(keepends=True) of template[:i]
				lines := strings.SplitAfter(pre, "\n")
				if lines[len(lines)-1] == "" {
					lines = lines[:len(lines)-1]
				}
				line, col = len(lines), j-len(strings.Join(lines[:len(lines)-1], ""))
			}
			r.fatal = "ValueError: Invalid placeholder in string: line " + strconv.Itoa(line) + ", col " + strconv.Itoa(col)
		}
		return s
	}
	return sb.String()
}

func (r *spLayoutRender) put(sb *strings.Builder, name string) bool {
	v, ok := r.env[name]
	if !ok {
		if r.missing == nil {
			r.missing = map[string]bool{}
		}
		r.missing[name] = true
		return false
	}
	sb.WriteString(v)
	return true
}

// spPyJSON is json.dump with python's defaults: ", " and ": " separators, ensure_ascii.
func spPyJSON(sb *strings.Builder, v *jval) {
	switch v.kind {
	case vNull:
		sb.WriteString("null")
	case vBool:
		if v.b {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case vNum:
		sb.WriteString(spPyNumber(v.s))
	case vStr:
		spPyString(sb, v.s)
	case vArr:
		sb.WriteByte('[')
		for i, e := range v.arr {
			if i > 0 {
				sb.WriteString(", ")
			}
			spPyJSON(sb, e)
		}
		sb.WriteByte(']')
	case vObj:
		sb.WriteByte('{')
		for i, m := range v.obj {
			if i > 0 {
				sb.WriteString(", ")
			}
			spPyString(sb, m.k)
			sb.WriteString(": ")
			spPyJSON(sb, m.v)
		}
		sb.WriteByte('}')
	}
}

func spPyString(sb *strings.Builder, s string) {
	const hex = "0123456789abcdef"
	sb.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			sb.WriteString(`\"`)
		case r == '\\':
			sb.WriteString(`\\`)
		case r == '\n':
			sb.WriteString(`\n`)
		case r == '\r':
			sb.WriteString(`\r`)
		case r == '\t':
			sb.WriteString(`\t`)
		case r == '\b':
			sb.WriteString(`\b`)
		case r == '\f':
			sb.WriteString(`\f`)
		case r >= ' ' && r <= '~':
			sb.WriteRune(r)
		case r < 0x10000:
			sb.WriteString(`\u` + string([]byte{hex[r>>12&15], hex[r>>8&15], hex[r>>4&15], hex[r&15]}))
		default:
			r -= 0x10000
			hi, lo := 0xd800+(r>>10), 0xdc00+(r&0x3ff)
			sb.WriteString(`\u` + string([]byte{hex[hi>>12&15], hex[hi>>8&15], hex[hi>>4&15], hex[hi&15]}))
			sb.WriteString(`\u` + string([]byte{hex[lo>>12&15], hex[lo>>8&15], hex[lo>>4&15], hex[lo&15]}))
		}
	}
	sb.WriteByte('"')
}

// spPyNumber is how python writes a JSON number it read: an int as an int, a float as repr(float).
func spPyNumber(lit string) string {
	if !strings.ContainsAny(lit, ".eE") {
		if strings.Trim(lit, "-0") == "" {
			return "0"
		}
		return lit
	}
	f, _ := strconv.ParseFloat(lit, 64)
	switch {
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	neg := ""
	if s[0] == '-' {
		neg, s = "-", s[1:]
	}
	i := strings.IndexByte(s, 'e')
	digits := strings.Replace(s[:i], ".", "", 1)
	e, _ := strconv.Atoi(s[i+1:])
	decpt := e + 1
	if decpt > -4 && decpt <= 16 {
		switch {
		case decpt <= 0:
			return neg + "0." + strings.Repeat("0", -decpt) + digits
		case len(digits) <= decpt:
			return neg + digits + strings.Repeat("0", decpt-len(digits)) + ".0"
		}
		return neg + digits[:decpt] + "." + digits[decpt:]
	}
	m := digits[:1]
	if len(digits) > 1 {
		m += "." + digits[1:]
	}
	sign, ae := "+", e
	if e < 0 {
		sign, ae = "-", -e
	}
	ex := strconv.Itoa(ae)
	if len(ex) < 2 {
		ex = "0" + ex
	}
	return neg + m + "e" + sign + ex
}

// ── _build_space ────────────────────────────────────────────────────────────

// buildSpace is `_build_space "$tpl"`: workspace (or the project's workspace and a tab), then the whole tab in
// one call. Sets WS, HW_TAB, LAYOUT_OUT. `workspace create` gets no --env: layout.apply replaces the tab and
// whatever it put on the first pane is discarded; --cwd stays (it is the workspace's identity_cwd).
func (l *lch) buildSpace(tpl string) {
	var wsRaw string
	if l.tab { // # MUTATION-ANCHOR: 847-B1-12
		// ONE TAB PER AGENT, one workspace per project.
		if !l.projectWorkspaceFor(l.proj) {
			l.die("could not find or create the workspace for " + l.proj)
		}
		l.ws = l.projectWorkspace
		if l.projectWorkspaceCreateReply != "" {
			// `workspace create` forces a first tab: use it for the first task, as bin/brain does for the first brainer.
			wsRaw = l.projectWorkspaceCreateReply
		} else {
			// No --label: layout.apply replaces this tab and destroys its label; the name is set after the layout.
			raw, rc := l.herdrCapture(true, `{"result":{"tab":{"tab_id":"shadow:t1"}}}`, "tab", "create", "--workspace", l.ws, "--cwd", l.workdir(), "--no-focus")
			wsRaw = bashSub(raw)
			if rc != 0 {
				l.die("herdr tab create failed in " + l.ws + ": " + head3(wsRaw))
			}
		}
	} else {
		raw, rc := l.herdrCapture(true, spWsCreateSynth, "workspace", "create", "--cwd", l.workdir(), "--label", l.label, "--no-focus")
		wsRaw = bashSub(raw)
		if rc != 0 {
			l.die("herdr workspace create failed: " + head3(wsRaw))
		}
		l.ws = spAlt(wsRaw, false, []string{"result", "workspace", "workspace_id"}, []string{"result", "workspace_id"})
		if l.ws == "" || l.ws == "null" { // # MUTATION-ANCHOR: 847-B1-13
			l.die("herdr workspace create returned no id: " + head3(wsRaw))
		}
	}
	// RECORD WHAT WE OWN, in space mode only: in tab mode hw does not own the workspace, and a `workspace`
	// record is exactly what would license a `workspace close` on it. The tab record is written after the layout.
	wd, run := l.workdir(), l.runID()
	if !l.tab && wd != "" && run != "" { // # MUTATION-ANCHOR: 847-B1-14
		l.fs.mkdirAll(wd + "/.hw/" + run)
		l.fs.write(wd+"/.hw/"+run+"/workspace", []byte(l.ws+"\n"), 0o666)
	}
	tab := spAlt(wsRaw, false, []string{"result", "tab", "tab_id"}, []string{"result", "root_pane", "tab_id"}, []string{"result", "tab_id"})
	if tab == "" || tab == "null" {
		l.die("no tab id in the create reply: " + head3(wsRaw))
	}
	l.hwTab = tab
	l.setEnv("HW_TAB", tab)

	out, _, rc := l.layoutApply(tpl)
	l.layoutOut = out
	// bash assigns LAYOUT_DOC inside the `$(_layout_apply …)` subshell, so the parent's stays "" and the run env
	// file below gets the empty union ('{}'). Reproduced as it is (see the report).
	if rc != 0 { // # MUTATION-ANCHOR: 847-B1-15
		// The space exists but has no layout: say so with its id, an empty space is recoverable by hand.
		l.warn("layout.apply failed for " + tpl + ":")
		l.err.Write([]byte(spHeadLines(l.layoutOut+"\n", 3)))
		l.die(l.layoutFailedAdvice())
	}

	// THE TAB ID CHANGED: layout.apply replaces the tab it is given and returns a new id, which is the one to
	// name and to record — recording the old one would make `hw done` read "already gone" and leave the real one open.
	if l.tab {
		live := spAlt(l.layoutOut, true, []string{"layout", "tab_id"}, []string{"result", "layout", "tab_id"}) // # MUTATION-ANCHOR: 847-B1-16
		if live == "" {
			live = l.hwTab
		}
		l.hwTab = live
		l.setEnv("HW_TAB", live)
		if _, rc := l.sh(cmdSpec{name: "herdr", args: []string{"tab", "rename", l.hwTab, l.label}, mut: true, merge: true}); rc != 0 {
			l.warn("could not name tab " + l.hwTab + " — `hw done` will fall back to the run record")
		}
		if wd != "" && run != "" {
			l.fs.mkdirAll(wd + "/.hw/" + run)
			l.fs.write(wd+"/.hw/"+run+"/tab", []byte(l.hwTab+"\n"), 0o666)
		}
	}

	// After the layout, not before: the file records what the panes were actually given.
	l.writeRunEnv(wd, l.layoutEnvUnion(l.layoutDoc), l.invokerPane())
	if l.tab {
		l.ok("layout applied in one call (" + spBaseName(tpl) + ") — tab " + l.hwTab + " in " + l.ws + " (" + l.label + ")")
	} else {
		l.ok("layout applied in one call (" + spBaseName(tpl) + ")")
	}
}

func spBaseName(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// ── --here ──────────────────────────────────────────────────────────────────

// runHere is `_run_here "$cwd"`: one pane beside this one, instead of a space. No dev servers; `hw done` cannot
// clean it up (a pane rename is a display label only), so hw prints its id and closing it is the person's.
func (l *lch) runHere(cwd string) {
	herdrPane := l.env("HERDR_PANE_ID")
	if herdrPane == "" { // # MUTATION-ANCHOR: 847-B1-17
		l.die("--here needs to run inside a herdr pane (HERDR_PANE_ID is unset)")
	}

	l.exportHwEnv(cwd)
	if l.artifactsDirFor(l.proj, cwd) != cwd {
		l.ensureArtifactsDir(cwd)
	}
	l.trustDir(cwd)
	l.applySdd(cwd, l.sdd) // `|| true`

	// Before the split, so the file is already there when the pane's shell starts: a --here pane is the one shape
	// that carries HW_INVOKER_PANE, which a restored executor cannot obtain any other way.
	l.writeRunEnv(cwd, "{}", l.invokerPane())

	l.step("space")
	// Split the RIGHTMOST pane, not the brainer's. Falls back to the brainer's own pane when the layout cannot be
	// read: degraded, but never a failed launch.
	target := l.rightmostPaneInTab(herdrPane)
	if target == "" { // # MUTATION-ANCHOR: 847-B1-18
		target = herdrPane
		l.warn("could not read the tab layout — splitting the brainer's own pane, so it will shrink")
	}
	args := []string{"pane", "split", target, "--direction", "right", "--ratio", "0.5", "--cwd", cwd}
	args = append(args, l.hwEnvArgs(cwd)...)
	args = append(args, "--no-focus")
	raw, rc := l.herdrCapture(true, `{"result":{"pane":{"pane_id":"shadow:p1"}}}`, args...)
	raw = bashSub(raw)
	if rc != 0 {
		l.die("herdr pane split failed: " + head3(raw))
	}
	pane := spAlt(raw, false, []string{"result", "pane", "pane_id"}, []string{"result", "pane_id"})
	if pane == "" || pane == "null" {
		l.die("herdr pane split returned no id: " + head3(raw))
	}
	if _, rc := l.herdr(true, false, "", "pane", "rename", pane, l.task); rc != 0 { // set -e: dies silently with herdr's status
		panic(exitNow{rc})
	}
	if target == herdrPane {
		l.ok("pane " + pane + ", split right of " + herdrPane)
	} else {
		l.ok("pane " + pane + ", split right of " + target + " (the brainer at " + herdrPane + " keeps its width)")
	}

	l.step("agent")
	if l.agentStart(pane, l.task, l.agent) { // # MUTATION-ANCHOR: 847-B1-19
		l.info(l.agent + " registered in " + pane + "; vendor runtime readiness is unestablished")
		l.dispatchPublish(pane)
		if rc := l.sendBrief(pane); rc != 0 { // set -e: the delivery's status ends the run
			panic(exitNow{rc})
		}
		l.verifyOpencodeEffort()
	} else {
		// No agent means no task.
		l.warn(l.agent + " did NOT start in " + pane + " — the pane is a plain shell; start it by hand")
		if l.brief != "" {
			l.warn("brief NOT delivered (no agent to receive it)")
		}
		l.herdr(true, true, "", "pane", "focus", pane)
		l.die(l.agent + " did NOT start in " + pane + ", so this task is NOT running: no agent, no brief delivered, and nothing to report back. Close that pane with `herdr pane close " + pane + "` and dispatch again.")
	}
	l.herdr(true, true, "", "pane", "focus", pane)

	l.printf("\n  %s%s%s  %s\n  pane %s — no dev servers; close it with: herdr pane close %s\n\n",
		l.cB, l.label, l.c0, cwd, pane, pane)
}
