package main

// Area A2 — the run's receipt, the ledger, the engram session, the stale-hook refusal (bin/hw: _receipt,
// _receipt_into, _receipt_verdict, _receipt_worktree, _receipt_session, _receipt_model, _engram_register_session,
// lib/hw/ledger.sh: _ledger_commit_brief, _ledger_dispatch, and bin/hw: _refuse_if_stale_hooks, _stale_hook_report).
//
// The receipt line is the same JSON `hw receipt`'s reader (snapshot.go, receipt.go) takes back: the object is
// built exactly as app.receiptInto builds it (key, value, source, at), but the write goes through l.fs so that
// shadow's first leg records it instead of doing it.

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type lrcState struct {
	hwEngramSession        string // HW_ENGRAM_SESSION: the id _engram_register_session registered ("" when none); _brief_preamble reads it
	ledgerBriefCommit      string // LEDGER_BRIEF_COMMIT: the commit _ledger_commit_brief made or found for the brief
	staleHooksAllowedFiles string // STALE_HOOKS_ALLOWED_FILES: set by _refuse_if_stale_hooks for a caller that persists it itself
}

// lrcNow is `date -u`; a variable so a test owns the clock.
var lrcNow = time.Now

// ── the receipt ─────────────────────────────────────────────────────────────

// receiptLine is the jq -nc object of _receipt_into plus its newline.
func receiptLine(key, val, src string, at time.Time) []byte {
	obj := &jval{kind: vObj, obj: []member{
		{"key", jstr(strings.ToValidUTF8(key, "�"))}, {"value", jstr(strings.ToValidUTF8(val, "�"))},
		{"source", jstr(strings.ToValidUTF8(src, "�"))},
		{"at", jstr(at.UTC().Format("2006-01-02T15:04:05Z"))},
	}}
	return []byte(obj.toJSON() + "\n")
}

// receiptInto is _receipt_into: append one fact, with its source, to <dir>/receipt.jsonl.
func (l *lch) receiptInto(rundir, key, val, src string) {
	if rundir == "" { // # MUTATION-ANCHOR: 847-A2-01
		return
	}
	// A fact with no stated source is an assertion; refuse it rather than record it.
	if src == "" { // # MUTATION-ANCHOR: 847-A2-02
		l.warn("internal: receipt '" + key + "' has no source; not recorded")
		return
	}
	if l.fs.mkdirAll(rundir) != nil {
		return
	}
	l.fs.appendTo(rundir+"/receipt.jsonl", receiptLine(key, val, src, lrcNow()), 0o666)
}

// receipt is _receipt: the launch-time caller, which knows the directory from $WT/$HW_RUN.
func (l *lch) receipt(key, val, src string) {
	if l.wt == "" || l.hwRun == "" { // # MUTATION-ANCHOR: 847-A2-03
		return
	}
	l.receiptInto(l.wt+"/.hw/"+l.hwRun, key, val, src)
	if key == "database" {
		l.dbLedgerAdd(l.proj, val, l.wt)
	}
}

// dbLedgerAdd is _db_ledger_add: the per-worktree database, recorded OUTSIDE the worktree, once.
func (l *lch) dbLedgerAdd(proj, db, wt string) {
	if proj == "" || db == "" || wt == "" {
		return
	}
	f := l.c.brain + "/.hw-db-ledger-" + proj
	line := db + "\t" + wt
	if data, err := os.ReadFile(f); err == nil { // grep -qxF
		for _, ln := range strings.Split(string(data), "\n") {
			if ln == line {
				return
			}
		}
	}
	l.fs.appendTo(f, []byte(line+"\n"), 0o666)
}

// receiptGet is _receipt_get: one measured value out of the current run's receipt ("" when unreadable).
func (l *lch) receiptGet(key string) string {
	f := l.wt + "/.hw/" + l.hwRun + "/receipt.jsonl"
	fh, err := os.Open(f) // [ -r ]
	if err != nil {
		return ""
	}
	fh.Close()
	return receiptLast(f, key)
}

// receiptWorktree is _receipt_worktree: what git says the worktree and its branch are, and the lock that says
// "somebody owns this" while the executor lives (`hw done` unlocks it; hw reap reads it).
func (l *lch) receiptWorktree(wt string) {
	top, _ := l.git(false, wt, "rev-parse", "--show-toplevel")
	top = bashSub(top)
	if top == "" {
		return
	}
	l.receipt("worktree", top, "git -C "+wt+" rev-parse --show-toplevel")
	br, _ := l.git(false, wt, "rev-parse", "--abbrev-ref", "HEAD")
	if br = bashSub(br); br != "" {
		l.receipt("branch", br, "git -C "+wt+" rev-parse --abbrev-ref HEAD")
	}
	// LOCKED WHILE THE EXECUTOR LIVES: a live executor has no process with its cwd in the worktree, so lsof
	// cannot see one; a lock is git's own "somebody owns this" and `git worktree remove` refuses it without -f -f.
	l.sh(cmdSpec{name: "git", args: []string{"-C", top, "worktree", "lock", "--reason",
		"hw " + lrcOr(l.proj, "?") + ":" + lrcOr(l.task, "?") + " run " + lrcOr(l.hwRun, "?") + ", executor live", top}, mut: true, quiet: true}) // # MUTATION-ANCHOR: 847-A2-05
}

func lrcOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// lrcMillis is `${VAR:-default}` read as the integer the bash `[ -ge ]` and `$(( / 1000))` make of it. A value
// that is not a number would loop bash forever in `[ "$waited" -ge "$budget" ]` (status 2, never true); here it
// is a budget of 0.
func lrcMillis(v string, def int64) int64 {
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func (l *lch) lrcSleep() { l.sh(cmdSpec{name: "sleep", args: []string{"1"}}) }

// ── the session id ──────────────────────────────────────────────────────────

// receiptSession is _receipt_session: the session id the agent published, recorded as a measurement (or as an
// absence, `none — …`, never as ""). Called by _send_brief after delivery; polls a short budget for the lazy vendors.
func (l *lch) receiptSession(pane string, drc int) {
	budget := lrcMillis(l.env("HW_SESSION_WAIT_MS"), 15000)
	var waited int64
	if l.wt == "" || l.hwRun == "" {
		return
	}
	if pane == "" {
		return
	}
	src := "herdr agent get " + pane + " | jq -r .result.agent.agent_session.value"
	var sid, vendor string
	for {
		info, _ := l.herdr(false, true, "", "agent", "get", pane)
		sid = jqRawOr(info, "", "result", "agent", "agent_session", "value")
		vendor = jqRawOr(info, "", "result", "agent", "agent_session", "agent")
		if vendor == "" {
			vendor = jqRawOr(info, "", "result", "agent", "agent")
		}
		if sid != "" {
			break
		}
		if waited >= budget { // # MUTATION-ANCHOR: 847-A2-06
			break
		}
		l.lrcSleep()
		waited += 1000
	}
	// THE DELIVERY ROUTE ALREADY READ THIS ID: opencode-tui asked the endpoint which session the brief's turn
	// landed in; herdr's agent_session is a second, slower publisher of the same fact.
	if tui := orDefaultStr(l.tuiSessionID, l.env("HW_TUI_SESSION_ID")); sid == "" && tui != "" { // # MUTATION-ANCHOR: 847-A2-07
		sid = tui
		vendor = "opencode"
		src = "channel-send opencode-tui: the session the brief's turn landed in, read from the executor's own endpoint (herdr published no agent_session within " + strconv.FormatInt(budget/1000, 10) + "s)"
	}
	if sid == "" {
		// NOT an empty string: a brief whose delivery failed never opened the turn that creates the session.
		l.receipt("session_id",
			"none — "+lrcOr(vendor, "the agent")+" published no agent_session within "+strconv.FormatInt(budget/1000, 10)+"s after brief delivery (delivery exit "+strconv.Itoa(drc)+")",
			src+" returned empty on every poll")
		l.warn("NO SESSION ID: this conversation is not addressable after the fact. If it needs recovering, nothing but its title identifies it.")
		return
	}
	l.receipt("session_id", sid, src+", "+strconv.FormatInt(waited, 10)+"ms after brief delivery")
	l.receipt("session_vendor", lrcOr(vendor, "unknown"), "herdr agent get "+pane+" | jq -r .result.agent.agent_session.agent")
	// The payoff, spelled out. The cwd matters: opencode resolves a session against the directory it ran in.
	var resume string
	switch vendor { // # MUTATION-ANCHOR: 847-A2-08
	case "claude":
		resume = "cd " + l.wt + " && claude --resume " + sid
	case "opencode":
		resume = "cd " + l.wt + " && opencode --session " + sid + "   # read-only: opencode export " + sid
	case "codex":
		resume = "cd " + l.wt + " && codex resume " + sid
	case "pi":
		resume = "cd " + l.wt + " && pi --session " + sid
	default:
		resume = "unknown vendor '" + vendor + "' — no resume command is claimed for it"
	}
	l.receipt("session_resume", resume, "the vendor's own documented resume for session_vendor="+lrcOr(vendor, "unknown"))
	l.info("session " + sid + " recorded — this conversation stays addressable: hw receipt " + l.proj + " " + l.task)
}

// ── the model that RUNS ─────────────────────────────────────────────────────

// lrcClaudeModel is `jq -r 'select(.type=="assistant") | .message.model // empty | select(. != "<synthetic>")' f | tail -1`.
func lrcClaudeModel(data []byte) string {
	vals, _ := parseStream(data)
	var outs []string
	for _, v := range vals {
		t, ok := v.get("type")
		if !ok || t.kind != vStr || t.s != "assistant" {
			continue
		}
		m, ok := v.get("message")
		if !ok {
			continue
		}
		mm, ok := m.get("model")
		if !ok || !mm.truthy() {
			continue
		}
		if r := mm.raw(); r != "<synthetic>" {
			outs = append(outs, r)
		}
	}
	return bashSub(lastLine(outs))
}

// lrcOpencodeModel is `jq -r '[.messages[]?.info | select(.role=="assistant" and .modelID != null)
// | "\(.providerID)/\(.modelID)"] | last // empty'`.
func lrcOpencodeModel(data []byte) string {
	vals, _ := parseStream(data)
	var outs []string
	for _, v := range vals {
		msgs, ok := v.get("messages")
		if !ok {
			continue
		}
		var items []*jval
		switch msgs.kind {
		case vArr:
			items = msgs.arr
		case vObj:
			for _, m := range msgs.obj {
				items = append(items, m.v)
			}
		}
		last := ""
		have := false
		failed := false
		for _, it := range items {
			info, ok := it.get("info")
			if !ok {
				failed = true
				break
			}
			role, ok := info.get("role")
			if !ok {
				failed = true
				break
			}
			mid, ok := info.get("modelID")
			if !ok {
				failed = true
				break
			}
			if role.kind == vStr && role.s == "assistant" && mid.kind != vNull {
				pid, _ := info.get("providerID")
				last, have = pid.tostring()+"/"+mid.tostring(), true
			}
		}
		if failed || !have {
			continue
		}
		outs = append(outs, last)
	}
	return bashSub(strings.Join(outs, "\n"))
}

// receiptModel is _receipt_model: the model the session's own record says answered, against the one handed to the
// vendor. Always 0. Follows receiptSession (the first assistant turn exists only after the brief).
func (l *lch) receiptModel(pane string, drc int) int {
	budget := lrcMillis(l.env("HW_MODEL_WAIT_MS"), 30000)
	var waited int64
	var running, why, src string
	if l.wt == "" || l.hwRun == "" {
		return 0
	}
	if drc != 0 { // # MUTATION-ANCHOR: 847-A2-09
		budget = 0
	}
	asked := l.modelPinned
	if asked == "" {
		asked = l.model
	}
	sid := l.receiptGet("session_id")
	// A vendor with no reader (codex) is not a fault: nothing was measured and nothing is claimed.
	if l.agent != "claude" && l.agent != "opencode" && l.agent != "pi" { // # MUTATION-ANCHOR: 847-A2-10
		ag := lrcOr(l.agent, "unknown")
		l.receipt("model_running", "n/a — hw has no model reader for vendor "+ag, "_receipt_model: no reader for this vendor")
		l.receipt("model_verdict", "n/a — nothing measured for vendor "+ag+"; asked: "+lrcOr(asked, "<agent default>"), "_receipt_model: no reader for this vendor")
		return 0
	}
	if l.agent == "pi" {
		sid = "pi" // pi publishes its own record (setup/pi/hw-executor.ts): no session id is needed to read it, even under --sandbox
	}
	piThinking := ""
	if sid == "" || strings.HasPrefix(sid, "none") { // # MUTATION-ANCHOR: 847-A2-11
		why = "no session id was recorded, so there is no session to read the model from"
	} else {
		for {
			if l.agent == "pi" {
				running, piThinking, src = l.piRuntimeRead()
			} else if l.agent == "claude" {
				cfg := l.accountConfigDir
				if cfg == "" {
					cfg = l.env("CLAUDE_CONFIG_DIR")
				}
				if cfg == "" {
					cfg = l.env("HOME") + "/.claude"
				}
				f := ""
				matches, _ := filepath.Glob(lrcGlobEscape(cfg) + "/projects/*/" + lrcGlobEscape(sid) + ".jsonl")
				sort.Strings(matches)
				for _, m := range matches {
					if fh, err := os.Open(m); err == nil { // [ -r ]
						fh.Close()
						f = m
						break
					}
				}
				if f != "" {
					data, _ := os.ReadFile(f)
					running = lrcClaudeModel(data)
					src = "the last assistant message.model in " + f
				} else {
					src = "no " + cfg + "/projects/*/" + sid + ".jsonl"
				}
			} else { // opencode: the export goes to a file in bash (piped, it truncates a large session)
				dir := ""
				if isDir(l.wt) {
					dir = l.wt
				}
				out, _ := l.sh(cmdSpec{name: "opencode", args: []string{"export", sid}, dir: dir, quiet: true})
				running = lrcOpencodeModel([]byte(out))
				src = "opencode export " + sid + ", the last assistant providerID/modelID"
			}
			if running != "" {
				break
			}
			if waited >= budget {
				break
			}
			l.lrcSleep()
			waited += 1000
		}
		if running == "" {
			why = lrcOr(src, "nothing read") + " — no assistant turn carried a model within " + strconv.FormatInt(budget/1000, 10) + "s"
		}
	}
	if running == "" { // # MUTATION-ANCHOR: 847-A2-12
		l.receipt("model_running", "none — "+why, lrcOr(src, "_receipt_session recorded no session"))
		l.receipt("model_verdict", "UNESTABLISHED — the model that runs could not be read ("+why+"); asked: "+lrcOr(asked, "<agent default>"), "no session record to compare")
		l.warn("UNESTABLISHED  model: could not read the model this executor runs — asked " + lrcOr(asked, "<agent default>") + ". " + why)
		return 0
	}
	l.receipt("model_running", running, src)
	var verdict string
	pinned := ""
	if l.modelPinned != "" {
		pinned = " (pinned from the alias " + l.model + ")"
	}
	if asked == "" {
		verdict = "recorded — no --model was given, so the vendor's default ran"
	} else {
		switch asked {
		case "fable", "opus", "sonnet", "haiku":
			if strings.HasPrefix(running, "claude-"+asked) { // # MUTATION-ANCHOR: 847-A2-13
				verdict = "unpinned — the alias " + asked + " resolved to " + running + "; " + lrcOr(l.proj, "this lane") + " does not pin it (model_pins in projects.json), so the same command can run another model tomorrow"
				l.warn("model: alias " + asked + " is not pinned by " + lrcOr(l.proj, "this lane") + " — it resolved to " + running + " today, and nothing keeps it there")
			} else {
				verdict = "DIVERGENCE — asked the alias " + asked + ", the session runs " + running
			}
		default:
			if running == asked { // # MUTATION-ANCHOR: 847-A2-14
				verdict = "match — asked " + asked + pinned + ", running " + running
			} else {
				verdict = "DIVERGENCE — asked " + asked + pinned + ", the session runs " + running
			}
		}
	}
	l.receipt("model_verdict", verdict, "compared MODEL_PINNED/MODEL with model_running")
	if strings.HasPrefix(verdict, "DIVERGENCE") {
		l.warn("DIVERGENCE  model: " + strings.TrimPrefix(verdict, "DIVERGENCE — "))
	}
	if l.agent == "pi" {
		l.piReceiptThinking(piThinking)
	}
	return 0
}

// lrcGlobEscape keeps a literal path from being read as a pattern (bash quotes "$cfg" and "$sid" in the glob).
func lrcGlobEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(s)
}

// ── intended vs measured ────────────────────────────────────────────────────

// receiptVerdict is _receipt_verdict, printed at the end of a launch: divergence here says the dispatch manifest,
// already printed and toasted, describes something other than what exists.
func (l *lch) receiptVerdict() {
	diverged, unestablished := false, false
	// worktree: the axis said whether there should be one; git says whether there is
	m := l.receiptGet("worktree")
	if l.wantWT && m == "" { // # MUTATION-ANCHOR: 847-A2-15
		l.warn("DIVERGENCE  worktree: the manifest said one would be created, and none was measured")
		diverged = true
	} else if !l.wantWT && m != "" {
		l.warn("DIVERGENCE  worktree: the manifest said none, and " + m + " exists")
		diverged = true
	}
	// database: same question, for the field that used to say `yes` everywhere
	if l.wantWT && l.wantDB && l.c.t.get(l.proj, "db_provisioned") == "1" {
		if l.receiptGet("database") == "" {
			l.warn("DIVERGENCE  database: the manifest said own-per-worktree, and none was measured")
			diverged = true
		}
	}
	// the agent: hw used to report a launch as fine when the CLI never started
	m = l.receiptGet("agent_ready")
	switch {
	case m == "yes":
	case strings.HasPrefix(m, "unknown") || strings.HasPrefix(m, "unreadable"): // # MUTATION-ANCHOR: 847-A2-16
		l.warn("UNESTABLISHED  agent: " + l.agent + " (" + m + ")")
		unestablished = true
	case m == "":
		l.warn("DIVERGENCE  agent: nothing measured whether " + l.agent + " ever became ready")
		diverged = true
	default:
		l.warn("DIVERGENCE  agent: " + l.agent + " did not reach interactive_ready (" + m + ")")
		diverged = true
	}
	// the model: what the session's own record says ran, against what was handed to the vendor
	m = l.receiptGet("model_verdict")
	switch {
	case strings.HasPrefix(m, "DIVERGENCE"):
		l.warn("DIVERGENCE  model: " + strings.TrimPrefix(m, "DIVERGENCE — "))
		diverged = true
	case strings.HasPrefix(m, "UNESTABLISHED"):
		l.warn("UNESTABLISHED  model: " + strings.TrimPrefix(m, "UNESTABLISHED — "))
		unestablished = true
	}
	// the effort: an opencode --effort is a claim until the live config says so
	if l.agent == "opencode" && l.effort != "" {
		m = l.receiptGet("effort_applied")
		switch {
		case strings.HasPrefix(m, "yes"):
		case strings.HasPrefix(m, "no"):
			l.warn("DIVERGENCE  effort: the manifest carried " + l.effort + " and the live executor does not have it (" + m + ")")
			diverged = true
		default:
			l.warn("UNESTABLISHED  effort: " + lrcOr(m, "never read back from the live executor"))
			unestablished = true
		}
	}
	switch {
	case !diverged && !unestablished: // # MUTATION-ANCHOR: 847-A2-17
		l.ok("receipt: every measured value matches the dispatch manifest")
	case diverged:
		l.warn("the dispatch manifest above describes something other than what exists")
		l.warn("read the measurements: hw receipt " + l.proj + " " + l.task)
	default:
		l.warn("receipt: incomplete — runtime readiness was not established; no runtime rejection was observed by this check")
	}
	l.info("receipt: " + l.wt + "/.hw/" + l.hwRun + "/receipt.jsonl")
}

// ── the engram session ──────────────────────────────────────────────────────

// lrcEngramLib runs one function of bin/engram-serve.sh in a bash of its own (the lib is sourced there, as bash
// sourced it here; it stays the owner of its three small verbs).
func (l *lch) lrcEngramLib(lib string, mut bool, synth string, fn string, args ...string) (string, int) {
	argv := append([]string{"-c", `. "$1"; shift; "$@"`, "hw", lib, fn}, args...)
	return l.sh(cmdSpec{name: "bash", args: argv, mut: mut, synth: synth})
}

// engramRegisterSession is _engram_register_session. BEST EFFORT, AND SAID SO: engram down is a receipt line and
// a warning, never a failed launch; and never into a store that is not the executor's (the serve is identified
// first). Always returns.
func (l *lch) engramRegisterSession() {
	if l.wt == "" || l.hwRun == "" {
		return
	}
	project := l.env("ENGRAM_PROJECT")
	dir := l.hwWorkdir
	if dir == "" {
		dir = l.env("HW_WORKDIR")
	}
	if dir == "" {
		dir = l.wt
	}
	if project == "" { // # MUTATION-ANCHOR: 847-A2-18
		return
	}
	id := "hw-" + l.hwRun
	binDir := l.env("HW_BIN_DIR")
	if binDir == "" {
		binDir = l.binDir()
	}
	lib := binDir + "/engram-serve.sh"
	// A `[ -r ]` and not `. … || return`: bash exits outright on a `.` whose file is missing.
	if fh, err := os.Open(lib); err != nil {
		l.receipt("engram_session", "none — "+lib+" is missing, so the serve could not be identified", "[ -r "+lib+" ]")
		l.warn("engram session NOT registered: " + lib + " is missing (incomplete bin/)")
		return
	} else {
		fh.Close()
	}
	url, _ := l.lrcEngramLib(lib, false, "", "engram_serve_url")
	url = bashSub(url)
	verdict, rc := l.lrcEngramLib(lib, false, "", "engram_serve_verdict", url)
	verdict = bashSub(verdict)
	if rc == 2 { // # MUTATION-ANCHOR: 847-A2-19
		store, _ := l.lrcEngramLib(lib, false, "", "engram_store_dir")
		l.receipt("engram_session", "none — "+verdict, "GET "+url+"/health vs "+bashSub(store)+"/.instance-id")
		l.warn("engram session NOT registered: " + verdict + ". Nothing was written to it. Stop that serve and start the real one (HOME=" + l.env("HOME") + " engram serve); until then an executor's mem_save has no session of its own and may fall into manual-save-" + project)
		return
	}
	posted := false
	var out string
	if rc == 0 {
		var prc int
		out, prc = l.lrcEngramLib(lib, true, `{"id":"`+id+`"}`, "engram_session_post", id, project, dir, url)
		out = bashSub(out)
		posted = prc == 0
	}
	if posted { // # MUTATION-ANCHOR: 847-A2-20
		l.hwEngramSession = id
		l.receipt("engram_session", id, "POST "+url+"/sessions project="+project+" directory="+dir)
		l.info("engram session " + id + " registered under " + project + " for " + dir)
		return
	}
	if rc != 0 {
		out = verdict
	}
	l.receipt("engram_session", "none — POST "+url+"/sessions failed: "+lrcOr(out, "no answer"), "curl -sf -m 5 POST "+url+"/sessions")
	l.warn("engram session NOT registered (" + url + " unreachable or refused): an unlabelled mem_save may still fall into manual-save-" + project)
}

// ── the ledger and the brief's commit ───────────────────────────────────────

func (l *lch) ledgerDir() string {
	if d := l.env("HW_LEDGER_DIR"); d != "" {
		return d
	}
	w := l.c.work
	if w == "" {
		w = l.env("HOME") + "/work"
	}
	return w + "/.hw-ledger"
}

// lrcSubagentModelValue is _subagent_model_receipt_value.
func (l *lch) lrcSubagentModelValue() string {
	sm := lrcOr(l.subagentModel, "inherit")
	switch {
	case l.agent != "claude":
		return "NOT APPLIED — " + lrcOr(l.agent, "unknown") + " does not read CLAUDE_CODE_SUBAGENT_MODEL (asked: " + sm + ")"
	case l.subagentModel != "" && l.subagentModel != "inherit": // _subagent_model_applies
		return l.subagentModel
	}
	return "inherit"
}

// ledgerDispatch is _ledger_dispatch for a launch (seq 1, rundir the run's): one JSONL line per real dispatch,
// in $WORK/.hw-ledger/<lane>.jsonl. A dry run writes nothing. Never fails the dispatch.
func (l *lch) ledgerDispatch(pane string, drc int) {
	if l.dispatchDry { // # MUTATION-ANCHOR: 847-A2-21
		return
	}
	if l.proj == "" || l.task == "" {
		return
	}
	rundir := ""
	if l.wt != "" {
		rundir = l.wt + "/.hw/" + l.hwRun
	}
	sha := ""
	if l.brief != "" && isFile(l.brief) {
		out, _ := l.sh(cmdSpec{name: "git", args: []string{"hash-object", "--", l.brief}, quiet: true})
		sha = bashSub(out)
	}
	base := l.taskBase()
	dir := l.ledgerDir()
	f := dir + "/" + l.proj + ".jsonl"
	if l.fs.mkdirAll(dir) != nil {
		l.warn("ledger: cannot create " + dir + "; this dispatch is NOT recorded")
		return
	}
	nz := func(s string) *jval {
		if s == "" {
			return jnull
		}
		return jstr(s)
	}
	delivered := &jval{kind: vBool, b: drc == 0}
	obj := &jval{kind: vObj, obj: []member{
		{"v", &jval{kind: vNum, s: "1"}}, {"ts", jstr(lrcNow().UTC().Format("2006-01-02T15:04:05Z"))},
		{"lane", jstr(l.proj)}, {"task", jstr(l.task)}, {"run", jstr(l.hwRun)}, {"seq", &jval{kind: vNum, s: "1"}},
		{"brief", nz(l.brief)}, {"brief_sha", nz(sha)}, {"brief_commit", nz(l.ledgerBriefCommit)},
		{"account", jstr(l.account)}, {"model", jstr(l.model)}, {"effort", jstr(l.effort)}, {"vendor", jstr(l.agent)},
		{"sdd", jstr(l.sdd)}, {"base", jstr(base)}, {"subagent_model", jstr(l.lrcSubagentModelValue())},
		{"pane", jstr(pane)}, {"rundir", jstr(rundir)}, {"delivered", delivered},
	}}
	if l.fs.appendTo(f, []byte(obj.toJSON()+"\n"), 0o666) != nil {
		l.warn("ledger: could not append to " + f + "; this dispatch is NOT recorded")
	}
}

// lrcPhys is `cd "$d" && pwd -P`: the physical path, or false where the cd fails.
func lrcPhys(d string) (string, bool) {
	abs, err := filepath.Abs(d)
	if err != nil {
		return "", false
	}
	p, err := filepath.EvalSymlinks(abs)
	if err != nil || !isDir(p) {
		return "", false
	}
	return p, true
}

// lrcBriefPathMatches is the `case "$abs" in "$brainphys"/*/briefs/*.md)`: both stars cross slashes and may be empty.
func lrcBriefPathMatches(abs, brain string) bool {
	if !strings.HasPrefix(abs, brain+"/") {
		return false
	}
	rest := abs[len(brain)+1:]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' && strings.HasPrefix(rest[i:], "/briefs/") && strings.HasSuffix(rest[i+len("/briefs/"):], ".md") {
			return true
		}
	}
	return false
}

// ledgerCommitBrief is _ledger_commit_brief: the dispatch is the moment a brief becomes a contract, so it is
// committed — ONE FILE, by the private-index recipe (commit-tree, no hook; update-ref given the OLD value).
// Only on `main`. Never fails the dispatch. Sets l.ledgerBriefCommit.
func (l *lch) ledgerCommitBrief() {
	l.ledgerBriefCommit = ""
	if l.dispatchDry { // # MUTATION-ANCHOR: 847-A2-22
		return
	}
	if l.brief == "" || !isFile(l.brief) {
		return
	}
	dirPhys, ok := lrcPhys(filepath.Dir(l.brief))
	if !ok {
		return
	}
	abs := dirPhys + "/" + filepath.Base(l.brief)
	brainPhys, _ := lrcPhys(l.c.brain)
	if !lrcBriefPathMatches(abs, brainPhys) { // # MUTATION-ANCHOR: 847-A2-23
		return
	}
	topOut, rc := l.git(false, l.c.brain, "rev-parse", "--show-toplevel")
	if rc != 0 {
		return
	}
	top, ok := lrcPhys(bashSub(topOut))
	if !ok {
		return
	}
	if !strings.HasPrefix(abs, top+"/") {
		return
	}
	rel := abs[len(top)+1:]
	// Tracked and unmodified: nothing to commit; its commit is the last one that touched it.
	dirty, rc := l.git(false, top, "status", "--porcelain", "--", rel)
	if rc != 0 { // a failed status is not "clean"
		return
	}
	if bashSub(dirty) == "" {
		c, _ := l.git(false, top, "log", "-1", "--format=%H", "--", rel)
		l.ledgerBriefCommit = bashSub(c)
		return
	}
	br, _ := l.git(false, top, "symbolic-ref", "--short", "-q", "HEAD")
	branch := bashSub(br)
	if branch != "main" { // # MUTATION-ANCHOR: 847-A2-24
		l.warn("brief " + rel + " is not committed and the brain checkout is on '" + lrcOr(branch, "a detached HEAD") + "', not main: left uncommitted")
		return
	}
	oldOut, rc := l.git(false, top, "rev-parse", "-q", "--verify", "refs/heads/main")
	if rc != 0 {
		return
	}
	old := bashSub(oldOut)
	tmp := l.env("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	idx := tmp + "/hw-brief-idx.private"
	if !l.readonly { // mktemp, then rm -f: only the name is kept
		fh, err := os.CreateTemp(tmp, "hw-brief-idx.*")
		if err != nil {
			return
		}
		idx = fh.Name()
		fh.Close()
		os.Remove(idx)
	}
	rmIdx := func() { // rm -f "$idx"
		if !l.readonly {
			os.Remove(idx)
		}
	}
	oldtree := old + "^{tree}"
	// the mutating git calls answer something plausible under HW_CORE_READONLY, so the sequence is the same
	blob, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", top, "hash-object", "-w", "--", rel}, mut: true, quiet: true, synth: "hw-readonly-blob\n"})
	if rc != 0 {
		rmIdx()
		l.warn("brief commit: could not hash " + rel)
		return
	}
	blob = bashSub(blob)
	mode := "100644"
	if fi, err := os.Stat(abs); err == nil && fi.Mode()&0o111 != 0 {
		mode = "100755"
	}
	built := false
	env := []string{"GIT_INDEX_FILE=" + idx}
	if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", top, "read-tree", old}, env: env, mut: true, quiet: true}); rc == 0 {
		if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", top, "update-index", "--add", "--cacheinfo", mode + "," + blob + "," + rel}, env: env, mut: true, quiet: true}); rc == 0 {
			built = true // # MUTATION-ANCHOR: 847-A2-25
		}
	}
	var newtree string
	if built {
		nt, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", top, "write-tree"}, env: env, mut: true, quiet: true, synth: "hw-readonly-tree\n"})
		if rc != 0 {
			built = false
		}
		newtree = bashSub(nt)
	}
	if !built {
		rmIdx()
		l.warn("brief commit: could not build the private index for " + rel + "; left uncommitted")
		return
	}
	rmIdx()
	// The one safety that matters: the new tree differs from HEAD's in that path and no other.
	chg, _ := l.git(false, top, "diff-tree", "-r", "--name-only", oldtree, newtree)
	changed := bashSub(chg)
	if l.readonly { // the trees above were never built; the check cannot be answered and is not what shadow compares
		changed = rel
	}
	if changed != rel { // # MUTATION-ANCHOR: 847-A2-26
		l.warn("brief commit REFUSED: the tree it built differs from HEAD in more than " + rel + "; left uncommitted")
		return
	}
	cOut, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", top, "commit-tree", newtree, "-p", old, "-m",
		"docs(briefs): " + lrcOr(l.proj, "lane") + "/" + lrcOr(l.task, "task") + " brief, committed at dispatch"}, mut: true, quiet: true, synth: "hw-readonly-commit\n"})
	if rc != 0 {
		l.warn("brief commit: commit-tree failed (is user.name/user.email set?); " + rel + " left uncommitted")
		return
	}
	c := bashSub(cOut)
	if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", top, "update-ref", "refs/heads/main", c, old}, mut: true, quiet: true}); rc != 0 {
		l.warn("brief commit: main moved while the commit was being written; " + rel + " left uncommitted (nothing was undone)")
		return
	}
	// The real index must not keep a stale idea of this path (it would show as staged-deleted).
	if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", top, "reset", "-q", "--", rel}, mut: true, quiet: true}); rc != 0 {
		l.warn("brief commit: committed, but 'git reset -- " + rel + "' failed on the real index; run it by hand")
	}
	l.ledgerBriefCommit = c
	short := c
	if len(short) > 8 {
		short = short[:8]
	}
	l.ok("brief committed at dispatch: " + short + " (" + rel + ")")
}

// ── stale hooks ─────────────────────────────────────────────────────────────

// worktreeHookFiles is _worktree_hook_files: the candidate hook-config files that exist.
func worktreeHookFiles(wt string) []string {
	var out []string
	for _, f := range []string{wt + "/.claude/settings.json", wt + "/.claude/settings.local.json", wt + "/.codex/hooks.json", wt + "/opencode.json"} {
		if fi, err := os.Stat(f); err == nil && fi.Mode().IsRegular() { // [ -f ]
			out = append(out, f)
		}
	}
	return out
}

// lrcFindCommands is the python find_commands: every {"type":"command","command":"…"} node, in document order.
func lrcFindCommands(v *jval) []string {
	var out []string
	switch v.kind {
	case vObj:
		// Python's dict: a repeated key keeps its first position and its last value.
		var keys []string
		vals := map[string]*jval{}
		for _, m := range v.obj {
			if _, seen := vals[m.k]; !seen {
				keys = append(keys, m.k)
			}
			vals[m.k] = m.v
		}
		if t, ok := vals["type"]; ok && t.kind == vStr && t.s == "command" {
			if c, ok := vals["command"]; ok && c.kind == vStr {
				out = append(out, c.s)
			}
		}
		for _, k := range keys {
			out = append(out, lrcFindCommands(vals[k])...)
		}
	case vArr:
		for _, e := range v.arr {
			out = append(out, lrcFindCommands(e)...)
		}
	}
	return out
}

// lrcShlexSplit is python's shlex.split(s) (POSIX mode, no comments): ok is false where Python raises ValueError.
func lrcShlexSplit(s string) ([]string, bool) {
	rs := []rune(s)
	var toks []string
	i := 0
	for {
		var tok []rune
		quoted := false
		state := ' ' // ' ' between words, 'a' in a word, '\'' or '"' in quotes, '\\' after an escape
		var escapedState rune
		done := false
		for !done {
			var c rune
			eof := i >= len(rs)
			if !eof {
				c = rs[i]
				i++
			}
			switch {
			case state == ' ':
				switch {
				case eof:
					done = true
				case c == ' ' || c == '\t' || c == '\r' || c == '\n':
					if len(tok) > 0 || quoted {
						done = true
					}
				case c == '\\':
					escapedState, state = 'a', '\\'
				case c == '\'' || c == '"':
					state = c
				default:
					tok = append(tok, c)
					state = 'a'
				}
			case state == '\'' || state == '"':
				quoted = true
				switch {
				case eof:
					return nil, false
				case c == state:
					state = 'a'
				case c == '\\' && state == '"':
					escapedState, state = state, '\\'
				default:
					tok = append(tok, c)
				}
			case state == '\\':
				if eof {
					return nil, false
				}
				if (escapedState == '\'' || escapedState == '"') && c != state && c != escapedState {
					tok = append(tok, '\\')
				}
				tok = append(tok, c)
				state = escapedState
			default: // 'a'
				switch {
				case eof:
					done = true
				case c == ' ' || c == '\t' || c == '\r' || c == '\n':
					state = ' '
					if len(tok) > 0 || quoted {
						done = true
					}
				case c == '\'' || c == '"':
					state = c
				case c == '\\':
					escapedState, state = 'a', '\\'
				default:
					tok = append(tok, c)
				}
			}
		}
		if len(tok) == 0 && !quoted { // posix: an unquoted empty token is the end of input
			return toks, true
		}
		toks = append(toks, string(tok))
	}
}

// lrcWhich is shutil.which(b) for a name with no directory part.
func (l *lch) lrcWhich(b string) bool {
	path := l.env("PATH")
	if path == "" {
		return false
	}
	for _, d := range strings.Split(path, ":") {
		p := filepath.Join(d, b)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// lrcMissingBinary is the python missing_binary(cmd): the binary the hook invokes when hw cannot find it, or "".
func (l *lch) lrcMissingBinary(wt, cmd string) string {
	for _, name := range []string{"CLAUDE_PROJECT_DIR", "PWD"} {
		cmd = strings.ReplaceAll(strings.ReplaceAll(cmd, "${"+name+"}", wt), "$"+name, wt)
	}
	tokens, ok := lrcShlexSplit(cmd)
	if !ok {
		return ""
	}
	for len(tokens) > 0 && lrcIsAssign(tokens[0]) {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return ""
	}
	b := tokens[0]
	if strings.Contains(b, "$") { // an env var this call site cannot prove the value of
		return ""
	}
	var found bool
	switch {
	case strings.HasPrefix(b, "/"):
		found = lrcExecFile(b)
	case strings.Contains(b, "/"):
		found = lrcExecFile(filepath.Join(wt, b)) // a worktree-relative path: a hook always runs at the project root
	default:
		found = l.lrcWhich(b)
	}
	if found {
		return ""
	}
	return b
}

func lrcExecFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}

// lrcIsAssign is re.match(r'^[A-Za-z_][A-Za-z0-9_]*=', s).
func lrcIsAssign(s string) bool {
	if s == "" || !(s[0] == '_' || s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z') {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '=':
			return true
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return false
}

// staleHookReport is _stale_hook_report: file<TAB>command<TAB>missing-binary, one line per hook whose invoked
// binary hw cannot find, as `$( … )` returns it. GENERIC: it walks every command node of every hook-config file.
// A command it cannot parse is skipped rather than guessed at: silence there, not a false refusal.
func (l *lch) staleHookReport(wt string) string {
	var lines []string
	for _, path := range worktreeHookFiles(wt) {
		data, err := os.ReadFile(path)
		if err != nil || !utf8.Valid(data) {
			continue
		}
		vals, perr := parseStream(data)
		if perr != nil || len(vals) != 1 { // json.load: one document, nothing after it
			continue
		}
		seen := map[string]bool{}
		for _, cmd := range lrcFindCommands(vals[0]) {
			if seen[cmd] {
				continue
			}
			seen[cmd] = true
			if b := l.lrcMissingBinary(wt, cmd); b != "" {
				lines = append(lines, path+"\t"+cmd+"\t"+b)
			}
		}
	}
	return bashSub(strings.Join(lines, "\n") + "\n")
}

// lrcReadTab3 is `IFS=$'\t' read -r f cmd bin`: tab is IFS whitespace, so runs of it collapse and the last
// variable takes the rest of the line.
func lrcReadTab3(line string) (f, cmd, bin string) {
	rest := strings.TrimLeft(line, "\t")
	cut := func() string {
		i := strings.IndexByte(rest, '\t')
		if i < 0 {
			w := rest
			rest = ""
			return w
		}
		w := rest[:i]
		rest = strings.TrimLeft(rest[i:], "\t")
		return w
	}
	f = cut()
	cmd = cut()
	bin = strings.TrimRight(rest, "\t")
	return
}

// refuseIfStaleHooks is _refuse_if_stale_hooks: where a worktree is about to be handed to a live agent (new,
// reused as a directory, or reused as a pane via `hw next`), refuse when its VERSIONED hook settings invoke a
// binary hw cannot find. --allow-stale-hooks launches anyway, loudly, and records it. $2 is the main checkout (for
// the remedy text), $3 the base ref or "". Dies unless allowed.
func (l *lch) refuseIfStaleHooks(wt, main, base string) {
	l.staleHooksAllowedFiles = ""
	report := l.staleHookReport(wt)
	if report == "" { // # MUTATION-ANCHOR: 847-A2-27
		return
	}
	var files []string
	seenF := map[string]bool{}
	var rows [][3]string
	for _, ln := range strings.Split(report, "\n") {
		f, cmd, bin := lrcReadTab3(ln)
		rows = append(rows, [3]string{f, cmd, bin})
		first := ln
		if i := strings.IndexByte(ln, '\t'); i >= 0 {
			first = ln[:i]
		}
		if !seenF[first] {
			seenF[first] = true
			files = append(files, first)
		}
	}
	sort.Strings(files)
	fl := strings.Join(files, " ")
	if l.allowStaleHooks { // # MUTATION-ANCHOR: 847-A2-28
		l.warn("STALE HOOKS in " + wt + " — launching anyway (--allow-stale-hooks): " + fl)
		for _, r := range rows {
			l.warn("  " + r[0] + ": '" + r[1] + "' -> '" + r[2] + "' not found")
		}
		// Set for the CALLER to persist: on the `hw next` route $WT/$HW_RUN are gone, so it writes its own.
		l.staleHooksAllowedFiles = fl
		l.receipt("stale_hooks_allowed", fl, "--allow-stale-hooks")
		return
	}
	l.warn("this worktree's VERSIONED hook settings invoke a binary hw cannot find on this machine:")
	for _, r := range rows {
		l.warn("  " + r[0] + ": '" + r[1] + "' -> '" + r[2] + "' not found")
	}
	tail := " Override with --allow-stale-hooks if you must launch onto it anyway (recorded in the receipt)."
	head := "refusing to hand a brief to " + wt + ": its versioned hook settings (" + fl + ") invoke a binary that is not on PATH — see above."
	// NOT A GIT WORKTREE AT ALL — the setup lane's plain directories: the git remedies below do not hold there.
	if _, err := os.Stat(wt + "/.git"); err != nil { // # MUTATION-ANCHOR: 847-A2-29
		l.die(head + " " + wt + " is not a git worktree (no .git), so there is no branch to merge and nothing to remove with `git worktree remove`. Fix the file(s) directly, or dispatch under a task name with no existing directory so hw builds a real worktree." + tail)
	}
	bo, _ := l.git(false, wt, "symbolic-ref", "--short", "HEAD")
	branch := bashSub(bo)
	// WHETHER ROLLBACK IS ABOUT TO DELETE $wt, decided before any message is built: it is compared by path, because
	// on the `hw next` route the launch globals belong to whatever the last fresh launch created, never this one.
	if main != "" && l.launchWTCreated && l.launchWT == wt { // # MUTATION-ANCHOR: 847-A2-30
		l.launchRollback(main)
		switch {
		case l.launchBranchCreated:
			l.die(head + " This worktree and its branch were both created by THIS launch and have just been rolled back — there is nothing left to clean up. Dispatch again: hw builds a fresh worktree off the base, since none exists at that path any more." + tail)
		case branch != "":
			l.die(head + " This launch's worktree has just been rolled back, but its branch (" + branch + ") existed BEFORE this launch and was left alone — the next dispatch will re-add a worktree on that same stale branch and hit this same refusal again. Delete it first: git -C '" + main + "' branch -D '" + branch + "' — then dispatch again for a fresh worktree off the base. Or keep that branch's work: check it out somewhere else and merge the base into it before you delete anything here." + tail)
		default:
			l.die(head + " This launch's worktree has just been rolled back; its branch name could not be read. Dispatch again, or inspect " + main + "'s branches by hand if one was left behind." + tail)
		}
	}
	// THE ORDINARY CASE: $wt is an OLD worktree that THIS launch only found and reused.
	var fixMerge, fixFresh string
	if base != "" {
		fixMerge = "git -C '" + wt + "' merge '" + base + "'"
	} else {
		fixMerge = "git -C '" + wt + "' merge <this lane's base branch>"
	}
	// BOTH STEPS, NOT ONE: `git worktree add` on an existing branch reuses it as-is, same stale settings, forever.
	switch {
	case main != "" && branch != "":
		fixFresh = "git -C '" + main + "' worktree remove '" + wt + "' && git -C '" + main + "' branch -D '" + branch + "'  (BOTH — keeping the branch means the next dispatch reuses it and hits this same refusal again), then dispatch again so hw builds a fresh worktree off the base"
	case main != "":
		fixFresh = "git -C '" + main + "' worktree remove '" + wt + "', then delete this worktree's branch too (both, or the next dispatch reuses it and hits this same refusal again), then dispatch again so hw builds a fresh worktree off the base"
	default:
		fixFresh = "remove the worktree AND its branch — both, or the next dispatch reuses the branch and hits this same refusal again — then dispatch again so hw builds a fresh worktree off the base"
	}
	l.die(head + " This branch predates the cleanup that dropped it. Fix: " + fixMerge + " — brings the base's current settings into this branch. Or start over: " + fixFresh + "." + tail)
}
