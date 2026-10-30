package main

// Area C — the brief as the executor receives it and its delivery (bin/hw: _send_brief, _deliver_brief,
// _resolve_strong_sender, _vendor_has_native_transport, the codex admission helpers, _turn_opened,
// _brief_undelivered; invoker-common.sh: invoker_resolve_sender). The prompt text is t5l_deliver_prompt.go's.
// OWNER: the C track.

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type ldlState struct {
	hwCodexThreadID string // HW_CODEX_THREAD_ID: the thread codex published after delivery

	// INVOKER_SENDER_VENDOR / _SESSION / _ENDPOINT, as invoker_resolve_sender leaves them
	invVendor, invSession, invEndpoint string
}

// dlSleep is `sleep 1`; a variable so a test does not wait.
var dlSleep = func() { time.Sleep(time.Second) }

// _send_brief "$pane": the status it returns is the delivery's (0 = delivered or no brief).
func (l *lch) sendBrief(pane string) int {
	// Before the brief guard: a brief-less run saves to engram too.
	l.engramRegisterSession()
	l.ledgerCommitBrief()
	if l.brief == "" { // # MUTATION-ANCHOR: 847-C-11
		l.ledgerDispatch(pane, 0)
		return 0
	}
	l.buildBriefText()
	// THE SESSION ID IS RECORDED ON BOTH PATHS, and the delivery status is passed through untouched.
	drc := l.deliverBrief(pane)
	l.ledgerDispatch(pane, drc)
	l.receiptSession(pane, drc)
	l.receiptModel(pane, drc) // || true
	return drc                // # MUTATION-ANCHOR: 847-C-12
}

// buildBriefText is the prompt assembly of _send_brief: BRIEF_TEXT, wrapped in everything the executor is told.
func (l *lch) buildBriefText() {
	preamble := l.briefPreamble()
	l.briefText = psep(preamble, l.briefText)
	epistemic := l.briefEpistemic()

	if l.sdd == "none" { // # MUTATION-ANCHOR: 847-C-13
		if l.dispatchFramework != "" && l.dispatchFramework != "<undetermined>" {
			l.briefText = bashSub("This task runs outside the framework flow, by choice: it was dispatched with\n--sdd none, so the brief is the whole contract. The directory is in " + l.dispatchFramework + " mode and\nits skills are loaded; leave their phase commands, review or verify flows and\npreflights unused here, and a skill that asks for a blocking preflight does not\napply to this task. Do the work directly and close with done-invoker.\n\n---\n\n" + l.briefText)
		} else {
			l.briefText = bashSub("This task runs outside the framework flow, by choice: it was dispatched with\n--sdd none, so the brief is the whole contract. The directory framework mode\ncould not be determined and framework skills may still be loaded; leave their\nphase commands, review or verify flows and preflights unused here, and a skill\nthat asks for a blocking preflight does not apply to this task. Do the work\ndirectly and close with done-invoker.\n\n---\n\n" + l.briefText)
		}
		l.info("prompt states the flow is deliberately not entered (" + l.dispatchFramework + " mode, --sdd none)")
	}

	if l.dispatchInPlay {
		entry := l.frameworkEntry(l.dispatchFramework)
		if entry != "" { // # MUTATION-ANCHOR: 847-C-15
			// The phase note goes AFTER the entry line, inside the same block.
			phaseNote := ""
			if l.dispatchFramework == "speckit" {
				phaseNote = bashSub("\n\n" + l.speckitPhaseNote())
			}
			l.briefText = bashSub("FIRST ACTION, before anything else: run " + entry + " for this task.\nThe framework mode here is " + l.dispatchFramework + " and this brief is its input, not a licence to\nstart editing. Do not read code, write files or run tests until " + entry + " has run." + phaseNote + "\n\n---\n\n" + l.briefText)
			l.info("prompt prefixed with " + entry + " (framework " + l.dispatchFramework + " is in play)")
		} else {
			l.warn("framework is '" + l.dispatchFramework + "' — no single entry command, so the prompt names none")
		}
	}

	// Deliberately last among prompt construction: before the original brief in every mode.
	if authorization := l.briefAuthorization(); authorization != "" {
		l.briefText = psep(authorization, l.briefText)
	}
	if note := l.briefSecretsNote(); note != "" {
		l.briefText = psep(note, l.briefText)
	}
	if note := l.copyBriefAssets(); note != "" {
		l.briefText = psep(note, l.briefText)
	}
	l.briefText = psep(epistemic, l.briefText)
}

// _vendor_has_native_transport: claude, codex and pi have none that hw can address.
func vendorHasNativeTransport(v string) bool {
	switch v {
	case "", "claude", "codex", "pi":
		return false // # MUTATION-ANCHOR: 847-C-16
	}
	return true
}

// invokerResolveSender is invoker_resolve_sender "$pane" (bin/invoker-common.sh): the executor's own address.
func (l *lch) invokerResolveSender(pane string) {
	named := pane != ""
	if !named {
		pane = l.env("HERDR_PANE_ID")
		l.invVendor = l.env("HW_EXECUTOR_VENDOR")
	} else {
		l.invVendor = "" // A named pane is somebody else's: nothing about OUR vendor applies to it.
	}
	l.invSession, l.invEndpoint = "", ""
	if pane == "" {
		return
	}
	info, _ := l.herdr(false, true, "", "agent", "get", pane)
	l.invSession = jqRawOr(info, "", "result", "agent", "agent_session", "value")
	if l.invVendor == "" {
		l.invVendor = jqRawOr(info, "", "result", "agent", "agent")
	}
	switch l.invVendor {
	case "opencode":
		process, _ := l.herdr(false, true, "", "pane", "process-info", "--pane", pane)
		if port := rtOpencodePort(bashSub(process)); port != "" {
			l.invEndpoint = "http://127.0.0.1:" + port
		}
	case "claude", "codex", "pi": // pi: herdr is its route too — no address is invented
		l.invEndpoint = "-"
	}
}

// resolveStrongSender is _resolve_strong_sender: re-ask until the executor's own transport resolves, or the budget runs out.
func (l *lch) resolveStrongSender(pane string) {
	waited := 0
	budget := 12000
	if v := l.env("HW_SENDER_WAIT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			budget = n
		}
	}
	for {
		l.invokerResolveSender(pane)
		if !vendorHasNativeTransport(l.invVendor) {
			return
		}
		if l.invSession != "" && l.invEndpoint != "" {
			if waited > 0 {
				l.info(l.invVendor + " transport resolved after " + strconv.Itoa(waited) + "ms — using its causal route, not the terminal buffer")
			}
			return
		}
		if waited >= budget { // # MUTATION-ANCHOR: 847-C-17
			l.warn(l.invVendor + " has no resolvable session/endpoint after " + strconv.Itoa(budget/1000) + "s")
			l.warn("falling back to the terminal buffer for this FIRST delivery: OpenCode has no session until a turn opens, so the causal route cannot resolve yet — expected, not a fault. State tokens are unaffected. If the buffer stalls, hw replays the same bytes once over the causal route, which resolves from the first turn on.")
			return
		}
		dlSleep()
		waited += 1000
	}
}

// dlCodex is the codex_* locals of _deliver_brief, which the _codex_* helpers read by dynamic scope.
type dlCodex struct {
	before         string // codex_before
	proof          bool   // codex_proof=1: the pane had no thread id before delivery, so one appearing proves a prompt
	unprovenReason string // codex_unproven_reason
}

func (l *lch) agentGet(pane string) string {
	out, _ := l.herdr(false, true, "", "agent", "get", pane)
	return out
}

// codexThreadID is _codex_thread_id.
func (l *lch) codexThreadID(pane string) string {
	return jqRawOr(l.agentGet(pane), "", "result", "agent", "agent_session", "value")
}

// codexThreadPublisherWired is _codex_thread_publisher_wired: hooks.json registers codex-channel-session-hook on UserPromptSubmit.
func (l *lch) codexThreadPublisherWired() bool {
	home := l.env("CODEX_HOME")
	if home == "" {
		home = l.env("HOME") + "/.codex"
	}
	hooks := home + "/hooks.json"
	if !isFile(hooks) {
		return false
	}
	data, err := os.ReadFile(hooks)
	if err != nil {
		return false
	}
	vals, perr := parseStream(data)
	if perr != nil || len(vals) == 0 {
		return false
	}
	// jq -e '[.hooks.UserPromptSubmit[]?.hooks[]?.command // empty] | map(select(test("codex-channel-session-hook"))) | length > 0'
	result := false
	for _, v := range vals {
		res, ok := dlHookRegistered(v)
		if !ok {
			return false // jq errored: exit 5
		}
		result = res
	}
	return result
}

func dlHookRegistered(doc *jval) (found, ok bool) {
	if doc.kind != vObj && doc.kind != vNull {
		return false, false
	}
	hk, _ := doc.get("hooks")
	if hk == nil || hk.kind == vNull {
		return false, true
	}
	if hk.kind != vObj {
		return false, true // `.hooks.UserPromptSubmit[]?` swallows the error
	}
	ups, _ := hk.get("UserPromptSubmit")
	if ups == nil {
		return false, true
	}
	for _, entry := range rtIter(ups) {
		if entry.kind != vObj && entry.kind != vNull {
			return false, false // `.hooks` on a scalar: not covered by the `?` that came before it
		}
		var inner *jval
		if entry.kind == vObj {
			inner, _ = entry.get("hooks")
		}
		if inner == nil {
			continue
		}
		for _, h := range rtIter(inner) {
			if h.kind != vObj && h.kind != vNull {
				return false, false
			}
			if h.kind != vObj {
				continue
			}
			cmd, has := h.get("command")
			if !has || !cmd.truthy() {
				continue
			}
			if cmd.kind != vStr {
				return false, false // test() on a non-string
			}
			if strings.Contains(cmd.s, "codex-channel-session-hook") {
				found = true
			}
		}
	}
	return found, true
}

// codexPromptLanded is _codex_prompt_landed: wait for the thread id codex publishes on UserPromptSubmit.
func (l *lch) codexPromptLanded(pane string) bool {
	waited := 0
	budget := 40000
	if v := l.env("HW_CODEX_PROMPT_WAIT_MS"); v != "" && strings.Trim(v, "0123456789") == "" {
		budget, _ = strconv.Atoi(v) // a budget that is not a number is not a budget
	}
	for {
		if now := l.codexThreadID(pane); now != "" {
			l.hwCodexThreadID = now
			return true
		}
		if waited >= budget {
			return false
		}
		dlSleep()
		waited += 1000
	}
}

// codexDialogBlocking is _codex_dialog_blocking: READ-ONLY, answers with the dialog's name when one owns the screen.
func (l *lch) codexDialogBlocking(pane string) (string, bool) {
	screen, _ := l.herdr(false, true, "", "agent", "read", pane, "--source", "detection", "--lines", "80")
	switch {
	case strings.Contains(screen, "Hooks need review"):
		return `hook trust ("Hooks need review")`, true
	case strings.Contains(screen, "Choose how you'd like Codex to proceed"):
		return "a model migration choice", true
	case strings.Contains(screen, "Do you trust the contents of this directory"):
		return "directory trust", true
	}
	return "", false
}

// codexAdmit is _codex_admit: one admission decision for every codex success branch.
func (l *lch) codexAdmit(pane, how, bytes string, c *dlCodex) bool {
	c.unprovenReason = ""
	reason := ""
	if !c.proof {
		before := c.before
		if before == "" {
			before = "<none recorded>"
		}
		reason = "this pane had already published thread " + before + " before delivery, so a thread id proves nothing about THIS brief"
	} else if !l.codexThreadPublisherWired() {
		home := l.env("CODEX_HOME")
		if home == "" {
			home = l.env("HOME") + "/.codex"
		}
		reason = home + "/hooks.json registers no codex-channel-session-hook on UserPromptSubmit, so no thread id can be published for any prompt"
	} else {
		if l.codexPromptLanded(pane) {
			l.receipt("brief_admitted", "yes — codex published thread "+l.hwCodexThreadID+" after delivery",
				how+", then ~/.codex/hooks.json UserPromptSubmit -> bin/codex-channel-session-hook.sh")
			l.ok("brief IN THE CONVERSATION on " + pane + " (" + bytes + " bytes, from " + l.brief + ") — " + how + ", and codex published thread " + l.hwCodexThreadID)
			return true
		}
		return false
	}
	// The caller records one final unproven outcome.
	c.unprovenReason = reason
	l.warn("codex's own delivery proof was UNAVAILABLE here: " + reason)
	return false
}

// redeliverLine is the dim line every undelivered branch prints for the herdr route.
func (l *lch) redeliverHerdrLine(pane string) {
	l.printf("  %s%s%s\n", l.cDim, "  "+l.binDir()+"/channel-send herdr "+pane+" - \"$(cat "+l.brief+")\"", l.c0)
}

// codexUnproven is _codex_unproven: not delivered is a claim hw is not entitled to here.
func (l *lch) codexUnproven(pane, reason string, c *dlCodex) {
	if reason == "" {
		reason = c.unprovenReason
	}
	if reason != "" {
		l.receipt("brief_admitted", "unproven — "+reason, "thread-id admission not established")
		l.warn("DELIVERY UNPROVEN on " + pane + ". " + reason + ".")
	} else {
		secs := int64(40000)
		if v := l.env("HW_CODEX_PROMPT_WAIT_MS"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				secs = n
			}
		}
		l.receipt("brief_admitted", "unproven — herdr reported a turn and codex published no thread id within "+strconv.FormatInt(secs/1000, 10)+"s",
			"herdr agent prompt (turn claimed) vs agent_session still absent, publisher wired")
		l.warn("DELIVERY UNPROVEN on " + pane + ". herdr reported a turn; codex published no thread id.")
	}
	l.warn("codex fires UserPromptSubmit on SUBMISSION, so a missing id usually means the bytes")
	l.warn("went somewhere that is not the conversation — a dialog, or an input line with no Enter.")
	l.warn("It can also mean the id was simply slow. hw cannot tell those apart, so it claims NEITHER.")
	l.warn("DO NOT re-send blindly: if the brief did land, a second delivery duplicates it.")
	l.warn("Read the pane. If it holds no brief, re-deliver it:")
	l.redeliverHerdrLine(pane)
	l.warn("if it is sitting on a dialog, clear that first — no route answers a dialog")
}

// turnOpened is _turn_opened: did a turn actually open, asked of herdr.
func (l *lch) turnOpened(pane string) bool {
	switch jqRawOr(l.agentGet(pane), "", "result", "agent", "agent_status") {
	case "working", "done":
		return true
	}
	return false
}

// briefUndelivered is _brief_undelivered: the brief is NOT in the executor's context; only re-delivering it helps.
func (l *lch) briefUndelivered(pane, route, target, endpoint string) {
	l.warn("THE BRIEF IS NOT IN " + pane + "'s CONTEXT. Nothing was given to this executor.")
	l.warn("do NOT tell it to start, and do NOT send a generic nudge — it has nothing")
	l.warn("to read, and no way to tell you so. Re-deliver the brief itself:")
	if route == "herdr" {
		l.redeliverHerdrLine(pane)
	} else {
		l.printf("  %s%s%s\n", l.cDim, "  "+l.binDir()+"/channel-send --require processed --id "+l.dlRunID()+":brief:retry "+route+" "+target+" "+endpoint+" \"$(cat "+l.brief+")\"", l.c0)
	}
	l.warn("if that fails too, close the run with `hw done` and dispatch again —")
	l.warn("an executor holding no brief is not behind, it is empty")
}

// dlRunID is .
func (l *lch) dlRunID() string {
	if v := l.runID(); v != "" {
		return v
	}
	return "norun"
}

// dlHead is `printf '%s\n' "$text" | head -N >&2`.
func (l *lch) dlHead(text string, n int) {
	lines := strings.Split(text+"\n", "\n")
	lines = lines[:len(lines)-1]
	if len(lines) > n {
		lines = lines[:n]
	}
	for _, ln := range lines {
		l.errf("%s\n", ln)
	}
}

// dlSha256 is `printf '%s' "$text" | shasum -a 256 | cut -d" " -f1`.
func (l *lch) dlSha256(text string) string {
	out, _ := l.sh(cmdSpec{name: "shasum", args: []string{"-a", "256"}, stdin: text})
	lines := strings.Split(out, "\n")
	for i, ln := range lines {
		if j := strings.IndexByte(ln, ' '); j >= 0 {
			lines[i] = ln[:j]
		}
	}
	return bashSub(strings.Join(lines, "\n"))
}

// channelSend is `"$HW_BIN_DIR/channel-send" <args…> 2>&1`: the text and the status.
func (l *lch) channelSend(args ...string) (string, int) {
	out, rc := l.sh(cmdSpec{name: l.binDir() + "/channel-send", args: args, mut: true, merge: true})
	return bashSub(out), rc
}

// nextLoadInvokerLib is _next_load_invoker_lib: the lib is sourced in bash; here only its presence is the contract.
func (l *lch) nextLoadInvokerLib() {
	if !brReadable(l.binDir() + "/invoker-common.sh") {
		l.die("cannot find invoker-common.sh beside " + l.binDir() + " — this brain/bin is incomplete")
	}
}

// deliverBrief is _deliver_brief: ONE DELIVERY PATH FOR BRIEFS AND RE-TASKS. 0 = delivered.
func (l *lch) deliverBrief(pane string) int {
	nbytes := strconv.Itoa(len(l.briefText))
	l.receipt("brief_sha256", l.dlSha256(l.briefText), "shasum -a 256 of the "+nbytes+" bytes passed to the delivery route")
	l.receipt("brief_bytes", nbytes, "wc -c of BRIEF_TEXT at delivery")

	// CODEX: READ THE SCREEN AGAIN, BECAUSE READINESS IS NOT A PROPERTY. The PANE's measured vendor, not $AGENT.
	c := &dlCodex{}
	paneVendor := jqRawOr(l.agentGet(pane), "", "result", "agent", "agent")
	if paneVendor == "" {
		paneVendor = l.agent
		if paneVendor == "" {
			l.warn("could not measure " + pane + "'s vendor and none was requested — vendor-specific delivery checks are SKIPPED for this delivery")
		}
	}
	if paneVendor == "codex" { // # MUTATION-ANCHOR: 847-C-18
		c.before = l.codexThreadID(pane)
		c.proof = c.before == ""
		if dialog, blocking := l.codexDialogBlocking(pane); blocking { // # MUTATION-ANCHOR: 847-C-19
			l.warn("codex is holding a startup dialog on " + pane + ": " + dialog)
			l.warn("NOTHING was typed — the brief would have landed in that dialog and been lost")
			l.receipt("brief_admitted", "no — codex was holding "+dialog+" at delivery time, nothing typed",
				"herdr agent read "+pane+" --source detection, read immediately before delivery")
			l.warn("THE BRIEF IS NOT IN " + pane + "'s CONTEXT, and re-sending it is NOT the fix.")
			l.warn("these dialogs wait for a PERSON: no route, no budget and no retry answers them,")
			l.warn("and channel-send would only push the brief into the same dialog. Clear it on that")
			l.warn("pane first, then re-deliver:")
			l.redeliverHerdrLine(pane)
			return 1
		}
	}

	// pi: the same second look as codex's, immediately before anything is typed.
	if paneVendor == "pi" && l.piDialogBeforeDelivery(pane) {
		return 1
	}

	l.nextLoadInvokerLib()
	// WAIT FOR THE STRONG ROUTE BEFORE USING THE WEAK ONE.
	l.resolveStrongSender(pane)
	var route, target, endpoint string
	if vendorHasNativeTransport(l.invVendor) && l.invSession != "" && l.invEndpoint != "" {
		route, target, endpoint = l.invVendor, l.invSession, l.invEndpoint
	} else {
		route, target, endpoint = "herdr", pane, "-"
	}

	// OPENCODE'S FIRST DELIVERY DOES NOT HAVE TO GO THROUGH THE BUFFER: its own TUI prompt endpoint, tried
	// BEFORE the buffer and never instead of the causal route.
	if route == "herdr" && l.invVendor == "opencode" && l.invEndpoint != "" { // # MUTATION-ANCHOR: 847-C-20
		tuiCwd := jqRawOr(l.agentGet(pane), "", "result", "agent", "cwd")
		if tuiCwd == "" {
			tuiCwd = l.wt
		}
		if tuiCwd != "" {
			l.info("opencode has no session yet, so the causal route cannot exist — delivering through its own TUI prompt endpoint instead of herdr's buffer")
			tuiOut, tuiRC := l.channelSend("--require", "processed", "opencode-tui", tuiCwd, l.invEndpoint, l.briefText)
			tuiSid := ""
			for _, ln := range strings.Split(tuiOut+"\n", "\n") { // sed -n 's/^CHANNEL_SESSION_ID=//p' | tail -1
				if strings.HasPrefix(ln, "CHANNEL_SESSION_ID=") {
					tuiSid = strings.TrimPrefix(ln, "CHANNEL_SESSION_ID=")
				}
			}
			if tuiSid != "" {
				l.tuiSessionID = tuiSid // a bash global (not exported) A2's receiptSession reads
				l.receipt("session_id_tui", tuiSid, "the session the brief's own turn created, read back from "+l.invEndpoint+"/session?directory="+tuiCwd)
			}
			if tuiRC == 0 {
				l.receipt("brief_admitted", "yes — opencode-tui, turn completed", "channel-send --require processed opencode-tui")
				sess := ""
				if tuiSid != "" {
					sess = ", session " + tuiSid
				}
				l.ok("brief IN THE CONVERSATION on " + pane + " (" + nbytes + " bytes, from " + l.brief + ") — opencode-tui receipt: processed" + sess)
				return 0
			}
			// 4 and 5 mean the brief IS in the receiver's session: a second delivery is worse than the failure.
			if tuiRC == 6 { // # MUTATION-ANCHOR: 847-C-21
				l.warn("opencode-tui: the receiver is UNUSABLE (exit 6) — NOT falling back to the buffer, the same process serves both and the buffer would only hide this as agent_prompt_stalled")
				l.dlHead(tuiOut, 4)
				l.receipt("brief_admitted", "no — receiver unusable, nothing typed on any route", "channel-send opencode-tui exit 6 (endpoint liveness)")
				l.briefUndelivered(pane, "opencode-tui", tuiCwd, l.invEndpoint)
				return 1
			}
			if tuiRC == 4 || tuiRC == 5 { // # MUTATION-ANCHOR: 847-C-22
				rc := strconv.Itoa(tuiRC)
				l.warn("opencode-tui: the brief reached the receiver and the turn did not complete (exit " + rc + ") — NOT retrying on the buffer, that would deliver it twice")
				l.dlHead(tuiOut, 3)
				l.receipt("brief_admitted", "yes — opencode-tui exit "+rc+": present in the session, turn unconfirmed", "channel-send --require processed opencode-tui")
				l.briefUndelivered(pane, "opencode-tui", tuiCwd, l.invEndpoint)
				return 1
			}
			l.warn("opencode-tui refused or could not deliver (exit " + strconv.Itoa(tuiRC) + ") — nothing was typed, falling back to herdr's buffer")
			l.dlHead(tuiOut, 3)
		}
	}

	if route != "herdr" { // # MUTATION-ANCHOR: 847-C-23
		out, rc := l.channelSend("--require", "processed", "--id", l.dlRunID()+":brief", route, target, endpoint, l.briefText)
		if rc == 0 {
			l.ok("brief IN THE CONVERSATION on " + pane + " (" + nbytes + " bytes, from " + l.brief + ") — " + route + " receipt: processed")
			return 0
		}
		l.warn(route + " refused to process the brief (exit " + strconv.Itoa(rc) + ")")
		l.dlHead(out, 3)
		l.briefUndelivered(pane, route, target, endpoint)
		return 1
	}

	// herdr route. --wait --until working turns "the text was typed" into "a turn opened".
	piBase := 0
	if paneVendor == "pi" {
		piBase = l.piInputsCount()
	}
	timeout := orDefault2(l.env("HW_TURN_WAIT_MS"), "45000")
	pout, prc := l.sh(cmdSpec{name: "herdr", args: []string{"agent", "prompt", pane, l.briefText, "--wait", "--until", "working", "--timeout", timeout}, mut: true, merge: true})
	pout = bashSub(pout)
	if prc == 0 {
		if paneVendor == "codex" {
			if l.codexAdmit(pane, "a turn opened", nbytes, c) {
				return 0
			}
			l.codexUnproven(pane, "", c)
			return 1
		}
		if paneVendor == "pi" { // # MUTATION-ANCHOR: 986-G08
			if l.piAdmit(pane, "a turn opened", nbytes, piBase) {
				return 0
			}
			return 1
		}
		l.ok("brief IN THE CONVERSATION on " + pane + " (" + nbytes + " bytes, from " + l.brief + ") — a turn opened")
		return 0
	}
	switch {
	case strings.Contains(pout, "agent_prompt_stalled"):
		if paneVendor == "codex" {
			// Re-read immediately before replay; never type into a newly opened startup dialog.
			if dialog, blocking := l.codexDialogBlocking(pane); blocking {
				l.codexUnproven(pane, "Codex dialog appeared after submission ("+dialog+"); replay was not attempted", c)
				return 1
			}
		}
		l.warn("herdr did not observe a turn after submission (agent_prompt_stalled)")
		l.warn("submission is not proof of admission; replay remains subject to the delivery checks below")
		// ONE EXACT REPLAY, AUTOMATICALLY: exactly once, and exactly the same bytes.
		l.resolveStrongSender(pane)
		var rRoute, rOut string
		var rRC int
		if vendorHasNativeTransport(l.invVendor) && l.invSession != "" && l.invEndpoint != "" {
			rRoute = l.invVendor
			l.warn("replaying the exact brief once over " + rRoute + " (a causal route, unlike the buffer)")
			rOut, rRC = l.channelSend("--require", "processed", "--id", l.dlRunID()+":brief:replay", rRoute, l.invSession, l.invEndpoint, l.briefText)
		} else {
			l.warn("replaying the exact brief once over herdr")
			rOut, rRC = l.channelSend("herdr", pane, "-", l.briefText)
			rRoute = "herdr"
		}
		if paneVendor == "codex" {
			if l.codexAdmit(pane, "replay over "+rRoute+" returned exit "+strconv.Itoa(rRC), nbytes, c) {
				return 0
			}
			l.codexUnproven(pane, orDefault2(c.unprovenReason, "replay outcome could not be established"), c)
			return 1
		}
		if paneVendor == "pi" && rRC == 0 && l.turnOpened(pane) {
			if l.piAdmit(pane, "replayed once over "+rRoute, nbytes, piBase) {
				return 0
			}
			return 1
		}
		if rRC == 0 && l.turnOpened(pane) { // # MUTATION-ANCHOR: 847-C-24
			l.receipt("brief_admitted", "yes — replayed once over "+rRoute, "channel-send "+rRoute+" + herdr agent get "+pane+" showed a turn")
			l.ok("brief IN THE CONVERSATION on " + pane + " (" + nbytes + " bytes, from " + l.brief + ") — replayed once over " + rRoute + ", turn confirmed")
			return 0
		}
		l.warn("the replay did not produce a turn either (exit " + strconv.Itoa(rRC) + ")")
		if rOut != "" {
			l.dlHead(rOut, 3)
		}
		l.receipt("brief_admitted", "no — first attempt stalled, replay over "+rRoute+" also failed", "herdr agent prompt --until working, then channel-send "+rRoute)
	case strings.Contains(pout, "agent_blocked"):
		l.warn(pane + " is blocked and refused the submission before any input was sent")
	default:
		if paneVendor == "codex" {
			// A timeout is not a negative receipt: only a fresh, wired thread publication can admit it.
			if c.proof && l.codexThreadPublisherWired() {
				if l.codexAdmit(pane, "herdr prompt failed or timed out", nbytes, c) {
					return 0
				}
			}
			l.codexUnproven(pane, "herdr prompt failed or timed out; no usable fresh thread proof", c)
			return 1
		}
		first := pout
		if i := strings.IndexByte(first, '\n'); i >= 0 {
			first = first[:i]
		}
		l.warn("herdr could not put the brief into " + pane + ": " + first)
	}
	l.briefUndelivered(pane, "herdr", pane, "-")
	return 1
}

// psep is `$(printf '%s\n\n---\n\n%s' "$a" "$b")`.
func psep(a, b string) string { return bashSub(a + "\n\n---\n\n" + b) }
