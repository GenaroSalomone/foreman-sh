package main

// Train T5, area C (invoker): the brainer pane a dispatch reports to. bin/hw `_invoker_pane_init`,
// `_invoker_pane`, `_invoker_how`, `_invoker_pane_resolve`, `_validate_invoker_lane`, `_resolve_invoker_context`
// and `_check_agent_name_free`; bin/project-spaces.sh `lane_brainer_verdict`. The two Python programs of the
// bash (the resolver and the verdict) are ported with Python's own behaviour: json.load, truthiness, and a
// traceback outside their `try`. The resolver runs inside `$( … )`, where errexit is off, so a traceback there
// is an empty answer (UNRESOLVED); the verdict is read by a plain assignment in the main shell, where it ends hw
// with status 1 and no word.

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// rtPyTraceback is a Python exception outside the script's own try: the program prints nothing and exits 1.
type rtPyTraceback struct{}

func rtPyCrash() { panic(rtPyTraceback{}) }

// rtPyRun runs one of the two Python programs: its stdout, and whether it died of a traceback.
func rtPyRun(f func() string) (out string, crashed bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(rtPyTraceback); !ok {
				panic(r)
			}
			out, crashed = "", true
		}
	}()
	return f(), false
}

// rtPanes is `json.load(stdin)["result"]["panes"]` followed by iterating it calling .get on every element:
// the panes (all dicts), false when json.load or the indexing raises (inside the script's try), and a crash
// when the iteration itself raises (outside it).
func rtPanes(text string) ([]jmap, bool) {
	if !utf8Valid(text) {
		return nil, false
	}
	v, ok := pyLoads(text)
	if !ok {
		return nil, false
	}
	res, isMap := v.(jmap)
	if !isMap {
		return nil, false
	}
	rv, has := res["result"]
	if !has {
		return nil, false
	}
	rm, isMap := rv.(jmap)
	if !isMap {
		return nil, false
	}
	pv, has := rm["panes"]
	if !has {
		return nil, false
	}
	switch x := pv.(type) {
	case []any:
		out := make([]jmap, 0, len(x))
		for _, e := range x {
			m, isMap := e.(jmap)
			if !isMap {
				rtPyCrash()
			}
			out = append(out, m)
		}
		return out, true
	case jmap:
		if len(x) == 0 {
			return nil, true
		}
	case string:
		if x == "" {
			return nil, true
		}
	}
	rtPyCrash()
	return nil, false
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "\x00") == s }

// rtHashable is whether Python can use the value as a dict key (a list or dict cannot).
func rtHashable(v any) bool {
	switch v.(type) {
	case []any, jmap:
		return false
	}
	return true
}

// rtSame is the script's `same(a, b)`: string equality, or the realpath comparison on native Windows.
func rtSame(a any, b string) bool {
	if runtime.GOOS != "windows" {
		s, isStr := a.(string)
		return isStr && s == b // # MUTATION-ANCHOR: 844-R106
	}
	s, isStr := a.(string)
	if !isStr {
		if pyTruthy(a) {
			rtPyCrash()
		}
		return false
	}
	if s == "" || b == "" {
		return false
	}
	k := func(x string) string { return pyCasefold(strings.TrimRight(pyRealpath(x), "/")) }
	return k(s) == k(b)
}

// rtPaneID is `cands[0] + "\t<how>"`: the pane id must be a string.
func rtPaneID(v any) string {
	s, ok := v.(string)
	if !ok {
		rtPyCrash()
	}
	return s
}

// invokerPaneResolve is `_invoker_pane_resolve`: "<pane>\t<how>", "!refuse <message>", or "" when nothing
// resolved. Its warnings go to stderr as they do in bash.
func (d *dsp) invokerPaneResolve() string {
	panesText, ok := herdrOut(0, "pane", "list")
	if !ok {
		panesText = "" // `|| panes=''`
	}
	panesText = bashSub(panesText)
	if panesText == "" { // # MUTATION-ANCHOR: 844-R84
		unverified := d.env("HW_INVOKER_PANE")
		if unverified == "" {
			unverified = d.env("HERDR_PANE_ID")
		}
		if unverified != "" { // # MUTATION-ANCHOR: 844-R85
			d.warn("HW_INVOKER_PANE unverified (herdr pane list unreachable): " + unverified) // # MUTATION-ANCHOR: 844-R86
			return unverified + "\tunverified"
		}
		return ""
	}
	resolved, _ := rtPyRun(func() string { return d.rtResolveScript(panesText) })
	pane, how := resolved, resolved
	if i := strings.Index(resolved, "\t"); i >= 0 {
		pane = resolved[:i]
	}
	if i := strings.LastIndex(resolved, "\t"); i >= 0 {
		how = resolved[i+1:]
	}
	if pane == "!refuse" { // # MUTATION-ANCHOR: 844-R87
		return "!refuse " + how
	}
	if pane == "" { // # MUTATION-ANCHOR: 844-R88
		d.warn("HW_INVOKER_PANE UNRESOLVED — ask-invoker and done-invoker will refuse, so the executor cannot report back. Inherited HERDR_PANE_ID=" + orDefaultStr(d.env("HERDR_PANE_ID"), "unset") + " is not a live pane.") // # MUTATION-ANCHOR: 844-R89
		d.warn("if you know your own pane id, export it and re-run: HW_INVOKER_PANE=$(herdr pane list | jq -r ...) hw …  — an explicit id is honoured first, and still checked for liveness.")                                 // # MUTATION-ANCHOR: 844-R90
		return ""
	}
	if how == "explicit" { // # MUTATION-ANCHOR: 844-R91
		// info … >&2: the announcement must not be captured as part of the pane id
		fmt.Fprintf(d.err, "  %s·%s %s\n", d.cDim, d.c0, "invoker pane taken from HW_INVOKER_PANE="+pane+" (named explicitly; verified live and not an executor pane)") // # MUTATION-ANCHOR: 844-R92
	}
	if how == "cwd" {
		d.warn("invoker pane resolved by " + how + " to " + pane + "; inherited HERDR_PANE_ID=" + orDefaultStr(d.env("HERDR_PANE_ID"), "unset") + " is STALE (this pane was moved)")
	}
	return pane + "\t" + how
}

// rtResolveScript is the python3 program of `_invoker_pane_resolve`, its stdout as `$( … )` returns it.
func (d *dsp) rtResolveScript(panesText string) string {
	panes, ok := rtPanes(panesText)
	if !ok {
		return ""
	}
	envID := d.env("HERDR_PANE_ID")
	byID := map[string]jmap{}
	for _, p := range panes {
		id := p["pane_id"]
		if !pyTruthy(id) {
			continue
		}
		if !rtHashable(id) {
			rtPyCrash()
		}
		if s, isStr := id.(string); isStr {
			byID[s] = p
		}
	}
	// HW_INVOKER_PANE reaches the script only when the caller's environment exported it; --report-to
	// assigns the shell variable without exporting it, which the script therefore never sees
	wantID := ""
	if d.getenv("HW_INVOKER_PANE") != "" {
		wantID = pyStrip(d.env("HW_INVOKER_PANE"))
	}
	if wantID != "" { // # MUTATION-ANCHOR: 844-R94
		p, live := byID[wantID]
		if !live { // # MUTATION-ANCHOR: 844-R95
			return "!refuse\tHW_INVOKER_PANE=" + wantID + " was named explicitly but is not a live pane (herdr pane list does not know it), so every executor launched here would have nowhere to report. Export the id `herdr pane list` actually shows for your brainer, or unset HW_INVOKER_PANE and let hw resolve it."
		}
		if p["label"] == "agent" { // # MUTATION-ANCHOR: 844-R96
			return "!refuse\tHW_INVOKER_PANE=" + wantID + " is an EXECUTOR pane (hw labels every executor it builds `agent`), not a brainer. Reports delivered there land on an agent that cannot act on them and will not say so. Name the brainer pane, or unset HW_INVOKER_PANE."
		}
		return wantID + "\texplicit" // # MUTATION-ANCHOR: 844-R97
	}
	if envID != "" {
		if _, live := byID[envID]; live { // # MUTATION-ANCHOR: 844-R98
			return envID + "\tenv" // # MUTATION-ANCHOR: 844-R99
		}
	}
	var agents []jmap
	for _, p := range panes {
		if pyTruthy(p["agent"]) && pyTruthy(p["pane_id"]) {
			agents = append(agents, p)
		}
	}
	here := d.env("PWD")
	var cands []any
	for _, p := range agents {
		if rtSame(p["cwd"], here) && p["label"] != "agent" {
			cands = append(cands, p["pane_id"])
		}
	}
	if len(cands) == 1 {
		return rtPaneID(cands[0]) + "\tcwd"
	}
	want := d.c.brain + "/" + d.proj
	cands = nil
	for _, p := range agents {
		if p["label"] == "brain" && rtSame(p["cwd"], want) { // # MUTATION-ANCHOR: 844-R104
			cands = append(cands, p["pane_id"])
		}
	}
	if len(cands) == 1 { // # MUTATION-ANCHOR: 844-R103
		return rtPaneID(cands[0]) + "\tlabel" // # MUTATION-ANCHOR: 844-R105
	}
	return ""
}

// invokerPaneInit is `_invoker_pane_init`: resolve once, as a statement, and die on a refusal.
func (d *dsp) invokerPaneInit() {
	if d.invokerDone {
		return
	}
	d.invokerDone = true
	if !d.wantReport { // # MUTATION-ANCHOR: 844-R107
		d.invokerResolved = ""
		d.invokerHow = "no-report"
		return
	}
	r := d.invokerPaneResolve()
	if strings.HasPrefix(r, "!refuse ") { // # MUTATION-ANCHOR: 844-R108
		d.die(strings.TrimPrefix(r, "!refuse "))
	}
	if r == "" { // # MUTATION-ANCHOR: 844-R109
		d.die("HW_INVOKER_PANE is UNRESOLVED — every executor launched here would have nowhere to report. Export the id `herdr pane list` actually shows for your brainer, or pass --no-report for a deliberate fire-and-forget dispatch.") // # MUTATION-ANCHOR: 844-R110
	}
	if i := strings.Index(r, "\t"); i >= 0 { // # MUTATION-ANCHOR: 844-R111
		d.invokerResolved, d.invokerHow = r[:i], r[i+1:]
	} else {
		d.invokerResolved, d.invokerHow = r, ""
	}
	d.validateInvokerLane() // # MUTATION-ANCHOR: 844-R112
}

// invokerPane is `_invoker_pane`.
func (d *dsp) invokerPane() string {
	if !d.invokerDone {
		d.invokerPaneInit()
	}
	return d.invokerResolved
}

// invokerHowWord is `_invoker_how`.
func (d *dsp) invokerHowWord() string {
	if !d.invokerDone {
		d.invokerPaneInit()
	}
	return d.invokerHow
}

// laneBrainerVerdict is `lane_brainer_verdict <pane> <expected cwd>` over a pane list: "ok",
// "mismatch<TAB>label=… cwd=…", or "" when the pane is not in the list (or the list is unreadable).
func (d *dsp) laneBrainerVerdict(panesText, pane, expected string) string {
	out, crashed := rtPyRun(func() string { return d.rtVerdictScript(panesText, pane, expected) })
	if crashed {
		panic(exitNow{1}) // verdict="$( … )" is a plain assignment: set -e ends hw
	}
	return out
}

func (d *dsp) rtVerdictScript(panesText, pane, expected string) string {
	if !utf8Valid(panesText) {
		return ""
	}
	v, ok := pyLoads(panesText)
	if !ok {
		return ""
	}
	top, isMap := v.(jmap)
	if !isMap {
		return ""
	}
	res, isMap := top["result"].(jmap)
	if !isMap {
		return ""
	}
	pv, has := res["panes"]
	if !has {
		return ""
	}
	var list []any
	switch x := pv.(type) {
	case []any:
		list = x
	case jmap:
		if len(x) != 0 {
			rtPyCrash()
		}
	case string:
		if x != "" {
			rtPyCrash()
		}
	default:
		rtPyCrash()
	}
	canon := func(p string) string { return pyCasefold(strings.TrimRight(pyRealpath(p), "/")) }
	for _, e := range list {
		p, isMap := e.(jmap)
		if !isMap {
			rtPyCrash()
		}
		if id, isStr := p["pane_id"].(string); !isStr || id != pane {
			continue
		}
		cwd := p["cwd"]
		cwdStr := ""
		if pyTruthy(cwd) {
			s, isStr := cwd.(string)
			if !isStr {
				rtPyCrash()
			}
			cwdStr = s
		}
		labelOK := p["label"] == "brain"                 // # MUTATION-ANCHOR: 844-R120
		if labelOK && canon(cwdStr) == canon(expected) { // # MUTATION-ANCHOR: 844-R121
			return "ok"
		}
		label, cw := "<none>", "<none>" // # MUTATION-ANCHOR: 844-R122
		if pyTruthy(p["label"]) {
			label = pyStr(p["label"])
		}
		if pyTruthy(cwd) {
			cw = pyStr(cwd)
		}
		return "mismatch\tlabel=" + label + " cwd=" + cw
	}
	return ""
}

// validateInvokerLane is `_validate_invoker_lane`: the resolved pane must be THIS lane's brainer.
func (d *dsp) validateInvokerLane() {
	if !d.wantReport {
		return
	}
	pane := d.invokerResolved
	expected := d.c.brain + "/" + d.proj
	if pane == "" {
		return
	}
	panesText, _ := herdrOut(0, "pane", "list")
	panesText = bashSub(panesText)
	how := orDefaultStr(d.invokerHow, "unknown")
	if panesText == "" { // # MUTATION-ANCHOR: 844-R113
		if d.reportToChosen {
			return
		}
		d.die("invoker resolved to " + pane + " by " + how + ", but herdr pane list is unreachable, so hw cannot prove it is the " + d.proj + " brainer (label brain, cwd " + expected + "). Run `brain " + d.proj + "` and launch from that pane, or deliberately route elsewhere with `--report-to " + pane + "`. NOTHING was built.")
	}
	verdict := d.laneBrainerVerdict(panesText, pane, expected)
	if verdict == "ok" { // # MUTATION-ANCHOR: 844-R114
		return
	}
	why := verdict
	if i := strings.Index(verdict, "\t"); i >= 0 { // # MUTATION-ANCHOR: 844-R115
		why = verdict[i+1:]
	}
	if d.reportToChosen { // # MUTATION-ANCHOR: 844-R116
		d.warn("--report-to deliberately routes " + d.proj + " reports to " + pane + " despite lane mismatch (" + why + ")") // # MUTATION-ANCHOR: 844-R117
		return
	}
	d.die("invoker resolved to " + pane + " by " + how + ", but it is not the " + d.proj + " brainer: " + why + "; expected label=brain cwd=" + expected + ". Run `brain " + d.proj + "` and re-run the launch there. If a human deliberately wants this foreign target, add `--report-to " + pane + "`; that choice is recorded in the manifest and receipt. NOTHING was built.") // # MUTATION-ANCHOR: 844-R118
}

// resolveInvokerContext is `_resolve_invoker_context`: the brainer's typed receiver address, from the pane that
// invoked hw (HERDR_PANE_ID). Missing data leaves all three empty.
func (d *dsp) resolveInvokerContext() {
	d.hwInvokerVendor, d.hwInvokerSession, d.hwInvokerEndpoint = "", "", ""
	d.rtInvokerExported = false
	pane := d.env("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	info, _ := herdrOut(0, "agent", "get", pane)
	info = bashSub(info)
	if info == "" {
		return
	}
	d.hwInvokerVendor = jqRawOr(info, "", "result", "agent", "agent")
	d.hwInvokerSession = jqRawOr(info, "", "result", "agent", "agent_session", "value")
	switch d.hwInvokerVendor {
	case "opencode":
		process, _ := herdrOut(0, "pane", "process-info", "--pane", pane)
		if port := rtOpencodePort(bashSub(process)); port != "" {
			d.hwInvokerEndpoint = "http://127.0.0.1:" + port
		}
	case "codex":
		if d.hwInvokerSession == "" {
			d.hwInvokerSession = rtCwdBase(info)
		}
		d.hwInvokerEndpoint = d.env("HOME") + "/.codex/app-server-control/app-server-control.sock"
	case "claude":
		d.hwInvokerEndpoint = "-"
	}
	d.rtInvokerExported = true // export HW_INVOKER_VENDOR HW_INVOKER_SESSION HW_INVOKER_ENDPOINT
	d.setEnv("HW_INVOKER_VENDOR", d.hwInvokerVendor)
	d.setEnv("HW_INVOKER_SESSION", d.hwInvokerSession)
	d.setEnv("HW_INVOKER_ENDPOINT", d.hwInvokerEndpoint)
}

// rtCwdBase is `jq -r '.result.agent.cwd // empty | split("/")[-1]'`.
func rtCwdBase(info string) string {
	vals, _ := parseStream([]byte(info))
	var outs []string
	for _, v := range vals {
		cur, ok := rtJqSteps(v, []any{"result", "agent", "cwd"})
		if !ok || !cur.truthy() {
			continue
		}
		if cur.kind != vStr {
			continue // split on a non-string is an error
		}
		parts := strings.Split(cur.s, "/")
		outs = append(outs, parts[len(parts)-1])
	}
	return bashSub(strings.Join(outs, "\n"))
}

// rtIter is jq's `.[]?` over a value: its elements (array order, object member order), none for a scalar.
func rtIter(v *jval) []*jval {
	switch v.kind {
	case vArr:
		return v.arr
	case vObj:
		out := make([]*jval, len(v.obj))
		for i, m := range v.obj {
			out[i] = m.v
		}
		return out
	}
	return nil
}

// rtOpencodePort is the jq program of `_resolve_invoker_context` for opencode: the value after `--port` in the
// argv of the first foreground process that has an argument exactly `opencode`.
func rtOpencodePort(process string) string {
	vals, _ := parseStream([]byte(process))
	var outs []string
	for _, v := range vals {
		pi, ok := rtJqSteps(v, []any{"result", "process_info", "foreground_processes"})
		if !ok {
			continue
		}
		var argv *jval
		errored := false
	find:
		for _, proc := range rtIter(pi) {
			// `.argv` on anything but an object or null is an error (the `?` of `.argv[]?` guards only the
			// iteration), and it ends this input unless an earlier process already matched (`first`).
			if proc.kind != vObj && proc.kind != vNull {
				errored = true
				break
			}
			a, ok := proc.get("argv")
			if !ok {
				continue
			}
			for _, w := range rtIter(a) {
				if w.kind == vStr && w.s == "opencode" {
					argv = a
					break find
				}
			}
		}
		if errored || argv == nil || argv.kind != vArr {
			continue
		}
		idx := -1
		for i, w := range argv.arr {
			if w.kind == vStr && w.s == "--port" {
				idx = i
				break
			}
		}
		if idx < 0 || idx+1 >= len(argv.arr) {
			continue
		}
		nx := argv.arr[idx+1]
		if nx.truthy() {
			outs = append(outs, nx.raw())
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

// checkAgentNameFree is `_check_agent_name_free`: herdr agent names are unique across every lane.
func (d *dsp) checkAgentNameFree(name string) {
	if d.reusePane != "" { // # MUTATION-ANCHOR: 844-R129
		return
	}
	text, _ := herdrClient{getenv: d.getenv, env: d.environ()}.stdout("agent", "list")
	text = bashSub(text)
	vals, perr := parseStream([]byte(text))
	// `jq -e` exits by its LAST input: it must have been read without error and have been an array.
	isArr := false
	if perr == nil && len(vals) > 0 {
		if ag, ok := rtJqSteps(vals[len(vals)-1], []any{"result", "agents"}); ok {
			isArr = ag.kind == vArr
		}
	}
	if !isArr { // # MUTATION-ANCHOR: 844-R131
		d.warn("UNESTABLISHED  agent name: herdr's agent registry could not be read, so whether '" + name + "' is free in every lane was not checked") // # MUTATION-ANCHOR: 844-R132
		return
	}
	// One `first(...)` per input; an error in an input (`.name` on a scalar) ends that input's output only.
	var holders []string
	for _, v := range vals {
		ag, ok := rtJqSteps(v, []any{"result", "agents"})
		if !ok {
			continue
		}
		for _, a := range rtIter(ag) {
			if a.kind != vObj && a.kind != vNull { // # MUTATION-ANCHOR: 844-R133
				break
			}
			n, ok := a.get("name")
			if !ok || n.kind != vStr || n.s != name { // # MUTATION-ANCHOR: 844-R134
				continue
			}
			pid, _ := a.get("pane_id")
			cwd, _ := a.get("cwd")
			cw := "unknown" // # MUTATION-ANCHOR: 844-R135
			if cwd != nil && cwd.truthy() {
				cw = cwd.tostring()
			}
			ps := "null"
			if pid != nil {
				ps = pid.tostring()
			}
			holders = append(holders, ps+" (cwd "+cw+")")
			break
		}
	}
	holder := bashSub(strings.Join(holders, "\n"))
	if holder != "" {
		d.die("herdr agent name '" + name + "' is already held by " + holder + ", and herdr requires agent names to be unique across EVERY lane — this launch would build its worktree and tab and then die at agent start with agent_name_taken. Rename the task (for example " + d.proj + "-" + name + "), or close that executor first. NOTHING was built.") // # MUTATION-ANCHOR: 844-R137
	}
}

// rtChildEnv is the environment bash's `cmd_next` would have had: the process's, with what hw exported before
// the call (HW_EXECUTOR_VENDOR, the invoker's vendor/session/endpoint, and HW_INVOKER_PANE when it was exported).
func (d *dsp) rtChildEnv() []string {
	// AGENT is a shell variable of the bash process, not an exported one, and `cmd_next` runs in that process: the
	// preamble it builds reads HW_EXECUTOR_VENDOR, else AGENT (the codex rules are added for a pane whose
	// run env records no vendor). A child process sees it only through the environment.
	over := map[string]string{"HW_EXECUTOR_VENDOR": d.agent, "AGENT": d.agent} // # MUTATION-ANCHOR: 844-R83
	// The same goes for what the dispatch's own flags and brief decided and `_next_authorization` / the preamble read
	// as shell variables: CHECK_HANDLES (--no-handle-check), REQUESTED_BY (--requested-by or the brief's) and
	// TASK_KIND (declared or inferred). Without them the child re-reads the brief alone and can answer differently.
	over["CHECK_HANDLES"] = "0"
	if d.checkHandles {
		over["CHECK_HANDLES"] = "1"
	}
	over["REQUESTED_BY"], over["REQUESTED_BY_SRC"] = d.requestedBy, d.requestedBySrc
	over["TASK_KIND"], over["TASK_KIND_SRC"] = d.taskKind, d.taskKindSrc
	if d.rtInvokerExported {
		over["HW_INVOKER_VENDOR"] = d.hwInvokerVendor
		over["HW_INVOKER_SESSION"] = d.hwInvokerSession
		over["HW_INVOKER_ENDPOINT"] = d.hwInvokerEndpoint
	}
	if d.getenv("HW_INVOKER_PANE") != "" {
		over["HW_INVOKER_PANE"] = d.env("HW_INVOKER_PANE")
	}
	var env []string
	for _, kv := range os.Environ() {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if _, replaced := over[k]; !replaced {
			env = append(env, kv)
		}
	}
	for k, v := range over {
		env = append(env, k+"="+v)
	}
	return env
}
