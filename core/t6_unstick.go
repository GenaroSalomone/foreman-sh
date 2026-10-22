package main

// hw unstick <pane> — port of cmd_unstick (bin/hw:12559). Deliberate recovery for an OpenCode pane that carries
// blocked_reason=stuck and blocked_scope=root|child: restart OpenCode in the SAME pane, reattaching the exact
// persisted session when the pane published one. The token signature is only necessary; the state witness
// (wait.go, the state witness) must positively confirm nothing is moving and nothing is outstanding, twice.
//
// The bash runs in a subshell (its EXIT trap releases only this command's lock); here the lock is released by a
// defer, which also runs while a die unwinds. The executor refusal (`_executor_refuse unstick`) is not here: the
// shim keeps executors on bash.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// unstickSleep is `sleep <seconds>`; a variable so a test does not spend the settle and the process wait.
var unstickSleep = func(a *app, secs string) {
	f, err := strconv.ParseFloat(secs, 64)
	if err != nil || f < 0 {
		a.errf("usage: sleep seconds\n") // bash's `sleep` fails and set -e ends the subshell
		panic(exitNow{1})
	}
	time.Sleep(time.Duration(f * float64(time.Second)))
}

// unstickHerdr is `herdr <args> 2>/dev/null`: what it printed and its status.
func unstickHerdr(args ...string) (string, int) {
	r := extRun(extCall{Name: "herdr", Args: args, Env: childEnv()})
	return r.Out, r.RC
}

func unstickHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:24]
}

// unstickPromptEvidence is prompt_evidence: the measured modal footer of a question, or both sides of a permission
// choice. Generic terminal text (`enter submit`, a lone `Deny`) is not evidence.
func unstickPromptEvidence(visible string) string {
	if strings.Contains(visible, "esc dismiss") {
		return "question footer: esc dismiss"
	}
	if strings.Contains(visible, "Allow once") && strings.Contains(visible, "Deny") {
		return "permission choices: Allow once + Deny"
	}
	if strings.Contains(visible, "Always allow") && strings.Contains(visible, "Deny") {
		return "permission choices: Always allow + Deny"
	}
	return ""
}

// unstickTokens reads the four fields every gate reads out of `herdr agent get`.
func unstickTokens(info string) (state, agent, reason, scope string) {
	return jqRawOr(info, "", "result", "agent", "agent_status"),
		jqRawOr(info, "", "result", "agent", "agent"),
		jqRawOr(info, "", "result", "agent", "tokens", "blocked_reason"),
		jqRawOr(info, "", "result", "agent", "tokens", "blocked_scope")
}

func unstickGet(pane string) string {
	out, _ := unstickHerdr("agent", "get", pane)
	return bashSub(out)
}

// unstickReadScreen is `screen="$(herdr agent read <pane> --source visible --lines 100 2>/dev/null)"`.
func unstickReadScreen(pane string) (string, bool) {
	out, rc := unstickHerdr("agent", "read", pane, "--source", "visible", "--lines", "100")
	return bashSub(out), rc == 0
}

// unstickWalk is jq's `.a.b.c` over one document: the value, or false when a step errors.
func unstickWalk(v *jval, path ...string) (*jval, bool) {
	cur := v
	for _, p := range path {
		var ok bool
		if cur, ok = cur.get(p); !ok {
			return nil, false
		}
	}
	return cur, true
}

// unstickLastDoc parses text as a jq input stream and returns its last document.
func unstickLastDoc(text string) (*jval, bool) {
	vals, err := parseStream([]byte(text))
	if err != nil || len(vals) == 0 {
		return nil, false
	}
	return vals[len(vals)-1], true
}

// unstickForegroundArgs is the `jq -c 'first(.result.process_info.foreground_processes[]? | select(argv[0] is
// opencode or ends in /opencode) | .argv[1:]) // empty'`, then the NUL-separated read of its elements. found is
// "args_json is not empty"; args are the leading strings (jq stops at the first element that is not a string or
// null, as `. + "\u0000"` errors on it).
func unstickForegroundArgs(text string) (args []string, found bool) {
	vals, _ := parseStream([]byte(text))
	for _, doc := range vals {
		ps, ok := unstickWalk(doc, "result", "process_info", "foreground_processes")
		if !ok || ps.kind != vArr {
			continue
		}
		for _, p := range ps.arr {
			if p.kind != vNull && p.kind != vObj {
				break // an error inside `?` ends the whole scan
			}
			argv, _ := p.get("argv")
			if argv.kind != vArr || len(argv.arr) == 0 || argv.arr[0].kind != vStr {
				continue
			}
			n := argv.arr[0].s
			if n != "opencode" && !strings.HasSuffix(n, "/opencode") {
				continue
			}
			for _, e := range argv.arr[1:] {
				if e.kind == vStr {
					args = append(args, e.s)
				} else if e.kind == vNull {
					args = append(args, "")
				} else {
					break
				}
			}
			return args, true
		}
	}
	return nil, false
}

// unstickProcessState is the jq -er filter run after Ctrl-C: "present", "absent", or false when the observation
// is unreadable (no list, an error, an element that is not an object with a non-empty argv of strings).
func unstickProcessState(text string) (string, bool) {
	vals, err := parseStream([]byte(text))
	if err != nil || len(vals) == 0 {
		return "", false
	}
	var outs []string
	for _, doc := range vals {
		if doc.kind != vObj {
			return "", false
		}
		if e, _ := doc.get("error"); e.kind != vNull { // `.error != null`
			return "", false
		}
		ps, ok := unstickWalk(doc, "result", "process_info", "foreground_processes")
		if !ok || ps.kind != vArr {
			return "", false
		}
		present := false
		for _, p := range ps.arr {
			if p.kind != vObj {
				return "", false
			}
			argv, _ := p.get("argv")
			if argv.kind != vArr || len(argv.arr) == 0 {
				return "", false
			}
			for _, e := range argv.arr {
				if e.kind != vStr {
					return "", false
				}
			}
		}
		for _, p := range ps.arr {
			argv, _ := p.get("argv")
			for _, e := range argv.arr {
				base := e.s
				if i := strings.LastIndexByte(base, '/'); i >= 0 {
					base = base[i+1:]
				}
				if base == "opencode" {
					present = true
				}
			}
		}
		if present {
			outs = append(outs, "present")
		} else {
			outs = append(outs, "absent")
		}
	}
	return strings.Join(outs, "\n"), true
}

// unstickRegistryCollision is `jq -e --arg n name --arg p pane 'any(.result.agents[]?; .name == $n and .pane_id != $p)'`.
func unstickRegistryCollision(text, name, pane string) bool {
	doc, ok := unstickLastDoc(text)
	if !ok {
		return false
	}
	agents, ok := unstickWalk(doc, "result", "agents")
	if !ok || agents.kind != vArr {
		return false
	}
	for _, e := range agents.arr {
		if e.kind != vNull && e.kind != vObj {
			return false // `.name` on a scalar errors, and the error is the answer
		}
		n, _ := e.get("name")
		pid, _ := e.get("pane_id")
		if n.kind == vStr && n.s == name && !(pid.kind == vStr && pid.s == pane) {
			return true
		}
	}
	return false
}

// unstickReceiptLast is `jq -sr '[.[] | select(.key == "<key>")][-1].value // empty' receipt.jsonl 2>/dev/null || true`:
// one unreadable line empties the whole answer (slurp).
func unstickReceiptLast(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	vals, perr := parseStream(b)
	if perr != nil {
		return ""
	}
	var last *jval
	for _, v := range vals {
		if v.kind != vNull && v.kind != vObj {
			return ""
		}
		k, _ := v.get("key")
		if k.kind == vStr && k.s == key {
			last = v
		}
	}
	if last == nil {
		return ""
	}
	val, _ := last.get("value")
	if !val.truthy() {
		return ""
	}
	return bashSub(val.raw())
}

func (a *app) cmdUnstick() int {
	if len(a.args) != 1 {
		a.die("usage: hw unstick <executor-pane> — deliberate recovery only for an OpenCode pane carrying blocked_reason=stuck and blocked_scope=root|child")
	}
	pane := a.args[0]
	if pane == "" {
		a.die("usage: hw unstick <executor-pane>")
	}

	info := unstickGet(pane)
	if info == "" {
		a.die("herdr does not know pane " + pane + " (or herdr is unreachable on $HERDR_SOCKET_PATH)")
	}
	state, agent, reason, scope := unstickTokens(info)
	if agent != "opencode" {
		a.die(pane + " is " + agent + ", not OpenCode; hw unstick will not send control keys to another agent")
	}
	if state != "blocked" {
		a.die(pane + " is " + state + ", not blocked; hw unstick refuses because agent_status alone is not the stuck condition")
	}
	if reason != "stuck" {
		a.die(pane + " is blocked_reason=" + orStr(reason, "absent") + ", not stuck; hw unstick will not dismiss a real question, permission, or session error")
	}
	if scope != "root" && scope != "child" {
		a.die(pane + " is blocked_reason=stuck but blocked_scope=" + orStr(scope, "absent") + "; required scope metadata is missing or invalid, so hw unstick refuses")
	}

	// The token signature is necessary, not sufficient: a healthy pane can publish it (measured 2026-08-27). An
	// independent witness must CONFIRM that nothing is moving and nothing is outstanding. Checked before any key.
	w := a.newWitness()
	w.state(pane)
	switch w.verdict {
	case "settled":
		a.info("cross-check CONFIRMS stuck: " + w.detail)
	case "live":
		a.die(pane + " publishes blocked_reason=stuck, but it is WORKING: " + w.detail + " — hw unstick refuses. The token signature alone is not the stuck condition, because a healthy pane can publish it (measured 2026-08-27, w4C:pAH and w4C:p9W). Nothing was sent, nothing was restarted, and that session is intact. Let it finish.")
	case "awaiting-human":
		a.die(pane + " publishes blocked_reason=stuck, but it is WAITING FOR A PERSON: " + w.detail + " — hw unstick refuses, because restarting the process would discard that prompt and the turn behind it. Answer it instead. Nothing was sent and nothing was restarted.")
	default:
		a.die(pane + " publishes blocked_reason=stuck, but that could NOT be corroborated: " + w.detail + " — hw unstick refuses. It restarts a process and replaces a live conversation, so it needs positive evidence that nothing is running, not merely the absence of evidence that something is. Fix the reader named above, or close the pane with `hw done` and dispatch again. Nothing was sent and nothing was restarted.")
	}

	// The lock: mkdir is atomic. A stale lock is removed only when its owner is this uid and its pid is proven
	// dead; a reused pid or another user's lock stays a loud refusal.
	uid := strconv.Itoa(os.Getuid())
	lockDir := "/tmp/hw-unstick-" + uid + "-" + unstickHash(pane) + ".lock"
	ownerFile := lockDir + "/owner"
	if os.Mkdir(lockDir, 0o777) != nil {
		ownerPid, ownerUID := "", ""
		if b, err := os.ReadFile(ownerFile); err == nil {
			line := string(b)
			if i := strings.IndexByte(line, '\n'); i >= 0 {
				line = line[:i]
			}
			f := strings.SplitN(line, " ", 3) // IFS=' ' read -r pid uid: the rest goes to uid
			if len(f) > 0 {
				ownerPid = f[0]
			}
			if len(f) > 1 {
				ownerUID = strings.Join(f[1:], " ")
			}
		}
		if allDigits(ownerPid) && ownerUID == uid && unstickPidDead(ownerPid) {
			stale := ownerFile + ".stale." + strconv.Itoa(os.Getpid())
			if os.Rename(ownerFile, stale) == nil {
				os.Remove(stale)
				os.Remove(lockDir)
			}
		}
		if os.Mkdir(lockDir, 0o777) != nil {
			a.die(pane + " already has an hw unstick recovery in progress (or an unproven stale lock at " + lockDir + "); refusing before any control key")
		}
	}
	if os.WriteFile(ownerFile, []byte(strconv.Itoa(os.Getpid())+" "+uid+"\n"), 0o666) != nil {
		os.Remove(lockDir)
		a.die("could not record hw unstick lock ownership for " + pane + "; refusing before any control key")
	}
	defer func() { // the bash EXIT trap
		os.Remove(ownerFile)
		os.Remove(lockDir)
	}()

	// A stale `stuck` token must not turn a genuine child question into a process restart. Navigation is harmless
	// and only after the strict token check; Escape is never sent. For root scope the modal is already visible.
	if scope == "child" {
		if _, rc := unstickHerdr("agent", "send-keys", pane, "ctrl+x", "down"); rc != 0 {
			a.die(pane + " was verified stuck/child but Herdr could not open its subagent view; nothing was restarted")
		}
		unstickSleep(a, orStr(a.getenv("HW_UNSTICK_VIEW_SETTLE_S"), "2"))
	}
	screen, ok := unstickReadScreen(pane)
	if !ok {
		a.die(pane + " visible screen could not be read after the stuck metadata check; hw unstick fails closed before any restart")
	}
	if ev := unstickPromptEvidence(screen); ev != "" {
		a.die(pane + " shows live prompt evidence after the stuck metadata check (" + ev + "). hw unstick did NOT dismiss, restart, or answer it; handle that human prompt instead.")
	}

	// Only a direct OpenCode executable is supported: a wrapper can carry its own state or positional contract.
	pout, _ := unstickHerdr("pane", "process-info", "--pane", pane)
	args, found := unstickForegroundArgs(pout)
	if !found {
		a.die(pane + " is stuck, but its foreground process is not a direct opencode executable; hw refuses wrappers and will not guess how to restart it")
	}

	// Herdr enforces globally unique agent names; the pane id is hashed, and the live registry is checked
	// before stopping. A collision is a loud refusal, never a takeover.
	restartName := "unstick-" + unstickHash(pane)
	agentsOut, _ := unstickHerdr("agent", "list")
	agents := bashSub(agentsOut)
	if agents == "" {
		a.die(pane + " is stuck, but Herdr's global agent registry could not be read; restart name was not reserved")
	}
	if !unstickIsArray(agents, "result", "agents") {
		a.die(pane + " is stuck, but Herdr's global agent registry was malformed; restart name was not reserved")
	}
	if unstickRegistryCollision(agents, restartName, pane) {
		a.die(pane + " is stuck, but restart name " + restartName + " is already owned by another pane; nothing was stopped")
	}

	// Capture is not authority: immediately before Ctrl-C re-read every token AND the visible UI.
	info = unstickGet(pane)
	state, agent, reason, scope = unstickTokens(info)
	if agent+":"+state+":"+reason != "opencode:blocked:stuck" {
		a.die(pane + " changed after argv capture (agent=" + agent + " status=" + state + " blocked_reason=" + orStr(reason, "absent") + "); nothing was restarted")
	}
	if scope != "root" && scope != "child" {
		a.die(pane + " lost valid blocked_scope metadata after argv capture; nothing was restarted")
	}
	if screen, ok = unstickReadScreen(pane); !ok {
		a.die(pane + " visible screen could not be read after argv capture; hw unstick fails closed before any restart")
	}
	if ev := unstickPromptEvidence(screen); ev != "" {
		a.die(pane + " shows live prompt evidence after argv capture (" + ev + "). hw unstick did NOT dismiss, restart, or answer it; handle that human prompt instead.")
	}

	// AND AGAIN, immediately before the stop: a pane that woke up in between must not be killed on a stale reading.
	w.state(pane)
	switch w.verdict {
	case "settled":
	case "live":
		a.die(pane + " started WORKING between the first cross-check and the restart: " + w.detail + " — nothing was stopped. That session is intact.")
	case "awaiting-human":
		a.die(pane + " raised a prompt between the first cross-check and the restart: " + w.detail + " — nothing was stopped. Answer it instead.")
	default:
		a.die(pane + " could no longer be corroborated as stuck immediately before the restart: " + w.detail + " — nothing was stopped. hw unstick fails closed here on purpose.")
	}

	// IDENTIFY WHAT IS ABOUT TO BE DISCONNECTED, BEFORE DISCONNECTING IT. A stuck pane can omit agent_session; the
	// launch receipt is then the fallback, only from the pane's exact hw_run and only when pane and vendor match.
	outSid := jqRawOr(info, "", "result", "agent", "agent_session", "value")
	outCwd := jqRawOr(info, "", "result", "agent", "cwd")
	outRun := jqRawOr(info, "", "result", "agent", "tokens", "hw_run")
	outRundir, receiptExact := "", false
	if outCwd != "" && outRun != "" {
		d := outCwd
		for hops := 0; hops < 12 && d != "" && d != "/"; hops++ {
			if isDir(d + "/.hw/" + outRun) {
				outRundir, receiptExact = d+"/.hw/"+outRun, true
				break
			}
			d = reapApplyDirname(d)
		}
	}
	if outRundir == "" && outCwd != "" {
		outRundir, _ = nextRunDir(outCwd)
	}

	sidSource := "herdr agent get " + pane + " | jq -r .result.agent.agent_session.value, read immediately before ctrl+c"
	if outSid == "" && receiptExact && readableFile(outRundir+"/receipt.jsonl") {
		rc := outRundir + "/receipt.jsonl"
		rsid := unstickReceiptLast(rc, "session_id")
		rvendor := unstickReceiptLast(rc, "session_vendor")
		rpane := unstickReceiptLast(rc, "pane")
		if strings.HasPrefix(rsid, "ses_") && rvendor == "opencode" && rpane == pane {
			outSid = rsid
			sidSource = outRundir + "/receipt.jsonl session_id, accepted only after session_vendor=opencode and pane=" + pane + " matched"
		}
	}
	var restartArgs []string
	if outSid != "" {
		if outRundir != "" {
			a.receiptInto(outRundir, "unstick_reattached_session", outSid, sidSource)
		}
		a.info("REATTACHING conversation " + outSid + " after the restart. Its persisted transcript stays readable with: cd " + orStr(outCwd, "<the pane cwd>") + " && opencode export " + outSid)
		if outRundir != "" {
			a.info("recorded as unstick_reattached_session in " + outRundir + "/receipt.jsonl")
		} else {
			a.warn("no .hw run directory found above " + orStr(outCwd, "the pane cwd") + ", so that id is in THIS output only — copy it now")
		}
		// The captured id is authority: every active continuation selector becomes one `--session <id>`, all other
		// argv bytes kept in order; after `--` everything is positional.
		boundary := false
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if boundary {
				restartArgs = append(restartArgs, arg)
				continue
			}
			switch {
			case arg == "--":
				restartArgs = append(restartArgs, "--session", outSid, "--")
				boundary = true
			case arg == "--session" || arg == "-s":
				i++
			case strings.HasPrefix(arg, "--session=") || strings.HasPrefix(arg, "-s=") || arg == "--continue" || arg == "-c":
			default:
				restartArgs = append(restartArgs, arg)
			}
		}
		if !boundary {
			restartArgs = append(restartArgs, "--session", outSid)
		}
	} else {
		a.warn("this pane published NO agent_session, so the conversation about to be replaced has no recoverable id — after the restart, only its title identifies it")
		if outRundir != "" {
			a.receiptInto(outRundir, "unstick_replaced_session",
				"none — the pane published no agent_session before the restart, so the replaced conversation is not addressable",
				"herdr agent get "+pane+" | jq -r .result.agent.agent_session.value returned empty before ctrl+c")
		}
		restartArgs = append(restartArgs, args...)
	}

	if outSid != "" {
		a.info("RECOVERY: " + pane + " is stuck/" + scope + " with no visible human prompt. Stopping only its OpenCode process; the pane, tab, cwd, work directory and exact session stay.")
	} else {
		a.info("RECOVERY: " + pane + " is stuck/" + scope + " with no visible human prompt. Stopping only its OpenCode process; the pane, tab, cwd and work directory stay. Its unaddressable active conversation will be replaced.")
	}
	if _, rc := unstickHerdr("agent", "send-keys", pane, "ctrl+c"); rc != 0 {
		a.die(pane + " was verified stuck but Herdr could not send Ctrl-C; no restart was attempted")
	}
	hasOpencode := true
	for i := 1; i <= 15; i++ {
		proc, rc := unstickHerdr("pane", "process-info", "--pane", pane)
		if rc != 0 {
			a.die(pane + " process exit is UNESTABLISHED: process-info failed after Ctrl-C; no second process was started")
		}
		ps, ok := unstickProcessState(proc)
		if !ok {
			a.die(pane + " process exit is UNESTABLISHED: process-info returned an invalid observation after Ctrl-C; no second process was started")
		}
		if ps == "absent" {
			hasOpencode = false
			break
		}
		unstickSleep(a, "1")
	}
	if hasOpencode {
		a.die(pane + " did not leave its OpenCode process within 15s; no second process was started")
	}

	startArgs := append([]string{"agent", "start", restartName, "--kind", "opencode", "--pane", pane, "--"}, restartArgs...)
	if _, rc := unstickHerdr(startArgs...); rc != 0 {
		a.die(pane + "'s stuck OpenCode process stopped, but its in-place restart failed. The pane/cwd/work directory remain; inspect the shell before starting anything else.")
	}
	info = unstickGet(pane)
	state = jqRawOr(info, "", "result", "agent", "agent_status")
	if state != "idle" && state != "working" {
		a.die(pane + "'s OpenCode process restarted but Herdr reports " + state + ", not idle/working; recovery is NOT claimed")
	}
	if outSid != "" {
		newSid := jqRawOr(info, "", "result", "agent", "agent_session", "value")
		if newSid != outSid {
			a.die(pane + " restarted, but Herdr reports session " + orStr(newSid, "absent") + ", not captured session " + outSid + "; stale stuck metadata was NOT cleared and session reattachment is NOT claimed")
		}
	}
	if _, rc := unstickHerdr("pane", "report-metadata", pane, "--source", "hw:unstick", "--clear-token", "blocked_reason", "--clear-token", "blocked_scope"); rc != 0 {
		a.die(pane + " restarted to " + state + ", but stale stuck metadata could not be cleared; recovery is NOT claimed")
	}
	a.ok("RECOVERED: " + pane + " restarted in place and Herdr now reports " + state + ". Cleared only stale blocked_reason/blocked_scope metadata; no question or permission was dismissed.")
	// Repeated after the restart on purpose: the line above scrolls past during a 15s process wait.
	if outSid != "" {
		a.info("reattached conversation " + outSid + " — its existing history is active in this pane and remains exportable with: cd " + orStr(outCwd, "<the pane cwd>") + " && opencode export " + outSid)
	}
	return 0
}

// unstickPidDead is `! kill -0 <pid>`: bash's kill fails for a missing process AND for one this user may not
// signal, and both read as "dead".
func unstickPidDead(pid string) bool {
	n, err := strconv.ParseInt(pid, 10, 32)
	if err != nil {
		return true
	}
	p, err := os.FindProcess(int(n))
	if err != nil {
		return true
	}
	return p.Signal(syscall.Signal(0)) != nil
}

// unstickIsArray is `jq -e '.a.b | type == "array"'` as a condition.
func unstickIsArray(text string, path ...string) bool {
	doc, ok := unstickLastDoc(text)
	if !ok {
		return false
	}
	v, ok := unstickWalk(doc, path...)
	return ok && v.kind == vArr
}
