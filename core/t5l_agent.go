package main

// Area B2 — starting the agent (bin/hw: _agent_start, _agent_start_sandboxed, _sandbox_arm_pane, _codex_startup_ready,
// _claude_startup_ready, _claude_start_failure_screen, _agent_step, _agent_ready_state, _verify_opencode_effort,
// _dispatch_publish; bin/project-spaces.sh: claude_startup_dialog, claude_dialog_advice). OWNER: the B2 track.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type lagState struct {
	sandboxArmed  string // _SANDBOX_ARMED: the pane _agent_start_sandboxed already armed (a per-call prefix assignment in bash)
	sandboxRundir string // _SANDBOX_RUNDIR: set by revive for a run launched in the sandbox
}

// lSleep is bash's `sleep`; a package variable so a test can set it to nothing. bash has no scale knob, so the
// contract world runs with real sleeps.
var lSleep = time.Sleep

// lHwRun is ${HW_RUN:-}: the global bash keeps, else the environment it started from.
func (l *lch) lHwRun() string {
	if l.hwRun != "" {
		return l.hwRun
	}
	return l.env("HW_RUN")
}

// lHwWorkdir is ${HW_WORKDIR:-}.
func (l *lch) lHwWorkdir() string {
	if l.hwWorkdir != "" {
		return l.hwWorkdir
	}
	return l.env("HW_WORKDIR")
}

// ── claude_startup_dialog / claude_dialog_advice (bin/project-spaces.sh:801) ────────────────────────────────

// claudeStartupDialog: what a Claude Code pane shows, matched on the dialog's own title; "" when none.
func claudeStartupDialog(screen string) string {
	screen = bashSub(screen)
	switch {
	case strings.Contains(screen, "Try the new fullscreen renderer?"):
		return "fullscreen-offer"
	case strings.Contains(screen, "running in Bypass Permissions mode"):
		return "bypass"
	case strings.Contains(screen, "Is this a project you created or one you trust"):
		return "trust"
	case strings.Contains(screen, "Choose the text style"), strings.Contains(screen, "Select login method"):
		return "onboarding"
	}
	return ""
}

func claudeFirstRunCommand(cfgDir string) string {
	if cfgDir != "" {
		return "CLAUDE_CONFIG_DIR=" + cfgDir + " claude --dangerously-skip-permissions"
	}
	return "claude --dangerously-skip-permissions"
}

func claudeAccountJSON(cfgDir, home string) string {
	if cfgDir != "" {
		return cfgDir + "/.claude.json"
	}
	return home + "/.claude.json"
}

// claudeDialogAdvice: what the person does about each screen, in one line.
func claudeDialogAdvice(dialog, cfgDir, home string) string {
	switch dialog {
	case "bypass":
		return "the Bypass Permissions warning is waiting, and accepting it is yours: run `" + claudeFirstRunCommand(cfgDir) + "` once in a terminal, accept it, /exit, and try again"
	case "onboarding":
		return "Claude Code's welcome (theme, then login) has never been completed in this account: run `" + claudeFirstRunCommand(cfgDir) + "` once in a terminal, finish it and accept the Bypass Permissions warning, /exit, and try again — `claude auth login` alone does not complete it"
	case "trust":
		return "the folder-trust question is waiting: this directory is not marked trusted in " + claudeAccountJSON(cfgDir, home)
	case "fullscreen-offer":
		return "the fullscreen-renderer offer is waiting; \"Not now\" (Esc) keeps the renderer as it is"
	}
	return ""
}

// ── startup readiness ───────────────────────────────────────────────────────────────────────────────────────

// agentRead is `herdr agent read <t> --source detection --lines 80 2>/dev/null || true`.
func (l *lch) agentRead(target string) string {
	out, _ := l.herdr(false, true, "", "agent", "read", target, "--source", "detection", "--lines", "80")
	return bashSub(out)
}

// codexStartupReady is `_codex_startup_ready`: herdr can report idle while a Codex dialog still owns stdin.
func (l *lch) codexStartupReady(name string) bool {
	screen := l.agentRead(name)
	switch {
	case strings.Contains(screen, "Do you trust the contents of this directory"):
		if _, rc := l.herdr(true, true, "", "agent", "send-keys", name, "enter"); rc != 0 { // # MUTATION-ANCHOR: 847-B2-01
			return false
		}
		lSleep(2 * time.Second)
	case strings.Contains(screen, "Hooks need review"):
		l.errf("%s\n", "codex is waiting for hook trust; review that dialog before delivering a brief")
		return false // # MUTATION-ANCHOR: 847-B2-02
	case strings.Contains(screen, "Choose how you'd like Codex to proceed"):
		l.errf("%s\n", "codex is waiting on a model migration dialog; choose a supported model before delivering a brief")
		return false // # MUTATION-ANCHOR: 847-B2-03
	}
	return true
}

// claudeStartupReady is `_claude_startup_ready`: answers only the renderer offer ("Not now", Esc); names every
// other first-run screen and fails.
func (l *lch) claudeStartupReady(target string) bool {
	dialog := claudeStartupDialog(l.agentRead(target))
	switch dialog {
	case "":
		return true
	case "fullscreen-offer":
		l.info("claude opened on its fullscreen-renderer offer — answering \"Not now\" (Esc), which keeps the renderer as it is")
		if _, rc := l.herdr(true, true, "", "agent", "send-keys", target, "esc"); rc != 0 { // # MUTATION-ANCHOR: 847-B2-04
			return false
		}
		lSleep(2 * time.Second)
		return true
	}
	l.errf("claude is waiting on a first-run screen (%s): %s\n", dialog, claudeDialogAdvice(dialog, l.accountConfigDir, l.env("HOME")))
	return false // # MUTATION-ANCHOR: 847-B2-05
}

// claudeStartFailureScreen is `_claude_start_failure_screen`: what the pane was SHOWING when the start was given up.
func (l *lch) claudeStartFailureScreen(target string) {
	screen := l.agentRead(target)
	if screen == "" {
		return
	}
	l.errf("what the pane was showing when hw gave up (last lines):\n")
	var keep []string
	for _, ln := range strings.Split(screen, "\n") {
		if strings.Trim(ln, " \t\n\v\f\r") == "" { // sed '/^[[:space:]]*$/d'
			continue
		}
		keep = append(keep, ln)
	}
	if len(keep) > 8 { // tail -8
		keep = keep[len(keep)-8:]
	}
	for _, ln := range keep {
		l.errf("    %s\n", ln)
	}
}

// ── the sandbox ─────────────────────────────────────────────────────────────────────────────────────────────

// bashPrintfQ is `printf '%q'` for the paths and words this file feeds it, as bash 3.2.57 (/bin/bash on macOS, the reference the
// goldens were captured with) prints it in a UTF-8 locale: it asks isprint of each BYTE, so the C1 range 0x80-0x9f (the
// middle bytes of most non-Latin runes: the snowman is e2 98 83) forces the $'...' form and prints those bytes as octal,
// while every other high byte stays raw. é (c3 a9) is plain; ☃ is $'\xe2' raw followed by \230\203.
func bashPrintfQ(s string) string {
	if s == "" {
		return "''"
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\t' || c < 0x20 || c == 0x7f || (c >= 0x80 && c <= 0x9f):
			return bashAnsiQ(s)
		case strings.IndexByte("\\'\"` !$&()*;<>?[]^{|}", c) >= 0, (c == '#' || c == '~') && i == 0:
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

func bashAnsiQ(s string) string {
	var sb strings.Builder
	sb.WriteString("$'")
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\n':
			sb.WriteString(`\n`)
		case c == '\t':
			sb.WriteString(`\t`)
		case c == '\\' || c == '\'':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		case c < 0x20 || c == 0x7f || (c >= 0x80 && c <= 0x9f):
			sb.WriteString("\\" + strconv.FormatInt(int64(c)+0o1000, 8)[1:])
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteString("'")
	return sb.String()
}

// rundirArmable is the `case "$rundir" in /*/.hw/?*)` pattern.
func rundirArmable(rundir string) bool {
	if !strings.HasPrefix(rundir, "/") {
		return false
	}
	rest := rundir[1:]
	for i := 0; i+len("/.hw/") < len(rest); i++ {
		if strings.HasPrefix(rest[i:], "/.hw/") && len(rest) > i+len("/.hw/") {
			return true
		}
	}
	return false
}

// sandboxArmPane is `_sandbox_arm_pane`: put the shim on the pane's PATH before `agent start` types the command.
func (l *lch) sandboxArmPane(pane, kind, rundir string) bool {
	if !rundirArmable(rundir) { // # MUTATION-ANCHOR: 847-B2-06
		l.warn("sandbox: no run directory to arm (" + rundir + ")")
		return false
	}
	out, rc := l.sh(cmdSpec{name: l.binDir() + "/hw-sandbox", args: []string{"arm", rundir, kind, l.proj, l.task},
		env: []string{"HW_WORKDIR=" + l.lHwWorkdir(), "HW_ARTIFACTS=" + l.env("HW_ARTIFACTS")}, mut: true})
	shimdir := bashSub(out)
	if rc != 0 { // # MUTATION-ANCHOR: 847-B2-07
		l.warn("sandbox: could not arm " + rundir + " — the agent is NOT started unconfined")
		return false
	}
	if _, rc := l.sh(cmdSpec{name: "herdr", args: []string{"pane", "run", pane, "export PATH=" + bashPrintfQ(shimdir) + ":$PATH; hash -r 2>/dev/null; clear"}, mut: true, quiet: true}); rc != 0 {
		l.warn("sandbox: could not put the shim on " + pane + "'s PATH — the agent is NOT started unconfined")
		return false
	}
	l.receiptInto(rundir, "sandbox", filepath.Dir(shimdir)+"/profile.sb", "hw-sandbox arm ("+kind+"); shim "+shimdir+" exported in "+pane)
	return true
}

// agentStartSandboxed is `_agent_start_sandboxed`: arm, start through the ordinary path, then PROVE it.
func (l *lch) agentStartSandboxed(pane, name, kind string) bool {
	rundir := l.sandboxRundir
	if rundir == "" {
		rundir = l.lHwWorkdir() + "/.hw/" + l.lHwRun()
	}
	if !l.sandboxArmPane(pane, kind, rundir) {
		return false
	}
	out, _ := l.sh(cmdSpec{name: l.binDir() + "/hw-sandbox", args: []string{"statedir", rundir}})
	st := bashSub(out)
	prev := l.sandboxArmed
	l.sandboxArmed = pane
	ok := l.agentStart(pane, name, kind)
	l.sandboxArmed = prev
	if !ok { // # MUTATION-ANCHOR: 847-B2-08
		return false
	}
	// An agent that started is not proof it started in the sandbox: the broker's heartbeat is.
	if l.readonly { // the broker is started by the helper a read-only leg only records: there is no heartbeat to wait for
		return true
	}
	for n := 0; n < 20; n++ {
		if fi, err := os.Stat(st + "/broker.alive"); err == nil && fi.Mode().IsRegular() {
			return true
		}
		lSleep(500 * time.Millisecond)
	}
	l.warn("sandbox: " + kind + " started in " + pane + " but the broker never came up (" + st + "/broker.alive) — it may be running UNCONFINED. Close it: hw done " + l.proj + " " + l.task)
	return false
}

// ── agent start ─────────────────────────────────────────────────────────────────────────────────────────────

// jqPath follows keys the way jq's `.a.b.c` does; ok is false when jq would have errored.
func jqPathVal(text string, path ...string) (*jval, bool) {
	vals, err := parseStream([]byte(text))
	if err != nil || len(vals) == 0 {
		return nil, false
	}
	cur := vals[len(vals)-1]
	for _, p := range path {
		next, ok := cur.get(p)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// startReady is `jq -e '.result.agent.interactive_ready == true'`.
func startReady(out string) bool {
	v, ok := jqPathVal(out, "result", "agent", "interactive_ready")
	return ok && v.kind == vBool && v.b
}

// probeIsAgent is `jq -e --arg pane --arg name --arg kind '.result.agent | .pane_id == $pane and .name == $name and .agent == $kind'`.
func probeIsAgent(probe, pane, name, kind string) bool {
	eq := func(key, want string) bool {
		v, ok := jqPathVal(probe, "result", "agent", key)
		return ok && v.kind == vStr && v.s == want
	}
	return eq("pane_id", pane) && eq("name", name) && eq("agent", kind)
}

// probeReady is `jq -e '.result.agent as $a | $a.interactive_ready == true and (["idle","done"] | index($a.agent_status) != null)'`.
func probeReady(probe string) bool {
	r, ok := jqPathVal(probe, "result", "agent", "interactive_ready")
	if !ok || r.kind != vBool || !r.b {
		return false
	}
	st, ok := jqPathVal(probe, "result", "agent", "agent_status")
	return ok && st.kind == vStr && (st.s == "idle" || st.s == "done")
}

// explorerClaudeArm is `_explorer_claude_arm "${EXPLORER_RUNDIR:-$HW_WORKDIR/.hw/$HW_RUN}"` when EXPLORER_ARM=1: the
// explorer's JSON goes to a file in the run directory and the pane is handed the function that reads it, so the
// launch line stays short. "" (and a warning) when it cannot be armed: the executor then starts without it.
func (l *lch) explorerClaudeArm(pane string) string {
	if !l.explorerArm { // # MUTATION-ANCHOR: 1007-G01
		return ""
	}
	rundir := l.lHwWorkdir() + "/.hw/" + l.lHwRun()
	json := l.explorer().claudeJSON()
	file := explorerClaudeFile(rundir)
	if !explorerRundirOK(rundir) || json == "" || l.fs.mkdirAll(rundir) != nil || l.fs.write(file, []byte(json+"\n"), 0o600) != nil {
		l.warn("the cheap explorer could not be armed for " + pane + " (no run directory, or the role is unreadable): this executor starts without it")
		return ""
	}
	return explorerClaudeFunction(file)
}

// agentStart is `_agent_start "$pane" "$name" "$kind"`: true when the agent is up and ready (bash's status 0).
// Retries while the pane comes up; every call here is inside an `if` in bash, so no status escapes under set -e.
func (l *lch) agentStart(pane, name, kind string) bool {
	if kind == "pi" && !l.piProjectSkillsPrepare() { // the project skills' one directory of links, before anything starts
		return false
	}
	if l.sandbox && l.sandboxArmed != pane { // # MUTATION-ANCHOR: 847-B2-09
		return l.agentStartSandboxed(pane, name, kind)
	}
	if kind == "claude" { // # MUTATION-ANCHOR: 847-B2-10
		l.claudeSessionEnvPrep(pane, l.explorerClaudeArm(pane))
	}
	tries, registered := 0, false
	for tries < 15 { // # MUTATION-ANCHOR: 847-B2-11
		if registered {
			// agent_not_ready keeps the registration. Poll that exact agent; a second creation would collide with our own name.
			out, _ := l.herdr(false, true, "", "agent", "get", pane)
			probe := bashSub(out)
			if !probeIsAgent(probe, pane, name, kind) {
				l.errf("%s\n", probe)
				return false
			}
			if kind == "codex" && !l.codexStartupReady(pane) {
				return false
			}
			if kind == "claude" && !l.claudeStartupReady(pane) {
				return false
			}
			if kind == "pi" && !l.piStartupReady(pane) {
				return false
			}
			if probeReady(probe) { // # MUTATION-ANCHOR: 847-B2-12
				return true
			}
			lSleep(2 * time.Second)
			tries++
			continue
		}
		// --timeout is explicit: herdr's own default (30000ms) is too short for a real worktree's cold start.
		args := []string{"agent", "start", name, "--kind", kind, "--pane", pane, "--timeout", "60000"}
		if l.agentArgs != "" {
			args = append(args, "--")
			args = append(args, strings.Fields(l.agentArgs)...) // unquoted $AGENT_ARGS: word-split
		}
		raw, rc := l.herdrCapture(true, `{"result":{"agent":{"interactive_ready":true}}}`, args...)
		out := bashSub(raw)
		if rc == 0 && startReady(out) { // # MUTATION-ANCHOR: 847-B2-13
			// Herdr can report idle + interactive_ready while a dialog still owns stdin: read the buffer first.
			if kind == "codex" && !l.codexStartupReady(name) {
				return false
			}
			if kind == "claude" && !l.claudeStartupReady(pane) {
				return false
			}
			// pi's pane reports ready while a startup screen can still own stdin.
			if kind == "pi" && !l.piStartupReady(pane) { // # MUTATION-ANCHOR: 986-G11
				return false
			}
			return true
		}
		switch {
		case strings.Contains(out, "agent_pane_busy"):
		case strings.Contains(out, "agent_not_ready"):
			registered = true // # MUTATION-ANCHOR: 847-B2-14
		case strings.Contains(out, "timed out waiting for agent startup"):
			// A busted BUDGET, not a busted agent: herdr keeps the name, exactly as agent_not_ready does.
			registered = true // # MUTATION-ANCHOR: 847-B2-15
		default:
			l.errf("%s\n", out)
			return false
		}
		lSleep(2 * time.Second)
		tries++
	}
	if kind == "claude" {
		l.claudeStartFailureScreen(pane)
	}
	return false
}

// agentReadyState is `_agent_ready_state`: herdr reports terminal recognition, not vendor runtime readiness.
func (l *lch) agentReadyState(pane string) string {
	j, _ := l.herdr(false, true, "", "agent", "get", pane)
	j = bashSub(j)
	ready := jqRawOr(j, "", "result", "agent", "interactive_ready")
	st := jqRawOr(j, "", "result", "agent", "agent_status")
	if ready+st == "" {
		return "unknown — herdr state unreadable; runtime readiness unestablished"
	}
	switch ready + ":" + st {
	case "true:idle", "true:working", "true:done": // # MUTATION-ANCHOR: 847-B2-16
		return "unknown — runtime readiness unestablished; herdr interactive_ready=" + ready + " agent_status=" + st
	}
	q := func(s string) string {
		if s == "" {
			return "?"
		}
		return s
	}
	return "no — interactive_ready=" + q(ready) + " agent_status=" + q(st)
}

// dispatchPublish is `_dispatch_publish`: the executor is on the brainer's cockpit from its first second. Best effort.
func (l *lch) dispatchPublish(pane string) {
	if pane == "" {
		return
	}
	invoker := l.invokerPane()
	or := func(s string) string {
		if s == "" {
			return "unknown"
		}
		return s
	}
	args := []string{"pane", "report-metadata", pane, "--source", "hw:dispatch", "--ttl-ms", "86400000",
		"--token", "hw_project=" + or(l.proj), "--token", "hw_task=" + or(l.task), "--token", "hw_run=" + or(l.lHwRun())}
	if invoker != "" {
		args = append(args, "--token", "hw_invoker="+invoker)
	}
	l.sh(cmdSpec{name: "herdr", args: args, mut: true, quiet: true})
	if invoker != "" { // # MUTATION-ANCHOR: 847-B2-17
		l.sh(cmdSpec{name: l.binDir() + "/cockpit-state", args: []string{"--kick", "--invoker", invoker}, mut: true, quiet: true})
	}
}

// ── the effort read back ────────────────────────────────────────────────────────────────────────────────────

// effortOf is `jq -r --arg a A '.agent[$a] | (.reasoningEffort // .options.reasoningEffort // "null")' || echo unreadable`.
func effortOf(body, agent string) string {
	vals, perr := parseStream([]byte(body))
	var outs []string
	failed := perr != nil
	for _, v := range vals {
		ag, ok := v.get("agent")
		var x *jval
		if ok {
			x, ok = ag.get(agent)
		}
		var res *jval
		if ok {
			if re, ok2 := x.get("reasoningEffort"); !ok2 {
				// `//` swallows an error on its left side
			} else if re.truthy() {
				res = re
			}
			if res == nil {
				op, ok2 := x.get("options")
				if ok2 {
					var re2 *jval
					if re2, ok2 = op.get("reasoningEffort"); ok2 && re2.truthy() {
						res = re2
					}
				}
				if !ok2 {
					failed = true
					break
				}
				if res == nil {
					res = jstr("null")
				}
			}
		} else {
			failed = true
			break
		}
		outs = append(outs, res.raw())
	}
	if failed {
		outs = append(outs, "unreadable")
	}
	return bashSub(strings.Join(outs, "\n"))
}

// verifyOpencodeEffort is `_verify_opencode_effort`: asked of the live executor, after start; warns, never dies.
func (l *lch) verifyOpencodeEffort() {
	if !(l.agent == "opencode" && l.effort != "") { // # MUTATION-ANCHOR: 847-B2-18
		return
	}
	endpoint := l.hwAgentEndpoint
	if endpoint == "" {
		endpoint = l.env("HW_AGENT_ENDPOINT")
	}
	shown := endpoint
	if shown == "" {
		shown = "<no endpoint>"
	}
	src := "GET " + shown + "/config (the executor's own server)"
	if endpoint == "" {
		l.receipt("effort_applied", "unverified — no executor endpoint to read /config from", src)
		l.warn("effort=" + l.effort + " UNVERIFIED: this executor has no endpoint to read its config back from")
		return
	}
	trS := l.env("HW_EFFORT_VERIFY_TRIES")
	if trS == "" {
		trS = "10"
	}
	tries, nerr := strconv.Atoi(strings.TrimSpace(trS))
	body := ""
	if nerr == nil {
		for i := 0; i < tries; i++ { // # MUTATION-ANCHOR: 847-B2-19
			out, _ := l.sh(cmdSpec{name: "curl", args: []string{"-fsS", "-m", "3", endpoint + "/config"}, quiet: true})
			body = bashSub(out)
			if body != "" {
				break
			}
			lSleep(time.Second)
		}
	}
	if body == "" {
		l.receipt("effort_applied", "unverified — "+endpoint+"/config did not answer in "+trS+" tries", src)
		l.warn("effort=" + l.effort + " UNVERIFIED: " + endpoint + "/config never answered, so nothing says it arrived")
		return
	}
	got := effortOf(body, l.opencodePrimaryAgent)
	if got == l.effort { // # MUTATION-ANCHOR: 847-B2-20
		l.receipt("effort_applied", "yes — agent \""+l.opencodePrimaryAgent+"\".reasoningEffort="+got, src)
		l.ok("effort=" + l.effort + " verified on the live executor (agent " + l.opencodePrimaryAgent + ")")
	} else {
		l.receipt("effort_applied", "no — agent \""+l.opencodePrimaryAgent+"\".reasoningEffort="+got+", wanted "+l.effort, src)
		l.warn("effort=" + l.effort + " NOT APPLIED: the live executor's config says reasoningEffort=" + got + " for " + l.opencodePrimaryAgent)
	}
}

// ── the step every build path ends with ─────────────────────────────────────────────────────────────────────

// agentStep is `_agent_step`: start the agent in l.pAgent, record, deliver the brief — or die. Ends with the
// workspace focused.
func (l *lch) agentStep() {
	if l.agentStart(l.pAgent, l.task, l.agent) { // # MUTATION-ANCHOR: 847-B2-21
		l.info(l.agent + " registered in " + l.pAgent + "; vendor runtime readiness is unestablished")
		// MEASURED AFTERWARDS, not inferred from the call returning 0.
		l.receipt("agent_ready", l.agentReadyState(l.pAgent), "herdr agent get "+l.pAgent)
		l.receipt("pane", l.pAgent, "layout.apply output")
		if l.modelFloorLine != "" {
			l.receipt("model_floor", l.modelFloorLine, "_check_model_floor against projects.json model_floor of "+l.proj)
		}
		if l.explorerLine != "" {
			l.receipt("explorer", l.explorerLine, "_explorer_resolve (lib/hw/explorer.sh)")
		}
		if l.topTierLine != "" {
			l.receipt("top_tier_why", l.topTierWhy, "--top-tier-why or the brief's top_tier_why (_check_top_tier_ceiling)")
		}
		if l.afterLine != "" {
			l.receipt("after", l.afterLine, "the brief's after: (_check_after), --ignore-after --why when overridden")
		}
		if l.belowFloorWhy != "" {
			l.receipt("below_floor_why", l.belowFloorWhy, "--below-floor-why")
		}
		src := l.requestedBySrc
		if src == "" {
			src = "no citation"
		}
		l.receipt("requested_by", trimNL(l.requestedLine()), "_check_requested: "+src)
		ep := l.hwAgentEndpoint
		if ep == "" {
			ep = l.env("HW_AGENT_ENDPOINT")
		}
		if ep != "" {
			l.receipt("endpoint", ep, "--port passed to opencode in _build_agent_args")
		}
		l.dispatchPublish(l.pAgent)
		if rc := l.sendBrief(l.pAgent); rc != 0 { // set -e: a failing _send_brief ends hw with its status
			panic(exitNow{rc}) // # MUTATION-ANCHOR: 847-B2-22
		}
		l.verifyOpencodeEffort()
	} else {
		// A launch with no agent is a failed launch. The receipt and its verdict are written first: what hw
		// measured is worth keeping precisely then.
		l.warn(l.agent + " did NOT start in " + l.pAgent + " — the pane is a plain shell; start it by hand")
		if l.brief != "" {
			l.warn("brief NOT delivered (no agent to receive it)")
		}
		l.receipt("agent_ready", "no — _agent_start failed", "hw _agent_start exit status")
		l.receiptVerdict()
		l.die(l.agent + " did NOT start in " + l.pAgent + ", so this task is NOT running: no agent, no brief delivered, and nothing to report back. The pane and its worktree are intact — read the receipt with `hw receipt " + l.proj + " " + l.task + "`, then close it with `hw done " + l.proj + " " + l.task + "` and dispatch again.") // # MUTATION-ANCHOR: 847-B2-23
	}
	l.receiptVerdict()
	if _, rc := l.herdr(true, false, "", "workspace", "focus", l.ws); rc != 0 { // `herdr workspace focus >/dev/null` under set -e
		panic(exitNow{rc}) // # MUTATION-ANCHOR: 847-B2-24
	}
}
