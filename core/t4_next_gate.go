package main

// The delivery gate of `hw next`: the sender's route (invoker_resolve_sender, bin/invoker-common.sh), the
// question modal (invoker_clear_opencode_question), the state witness (witness_state and witness_wait,
// the bash state witness) and the refusals that name what the gate read (_next_busy_message,
// _next_settled_message, _next_finished_stale_stuck, lib/hw/next.sh). The witness and the invoker library
// are shared with channel-send, hw unstick and hw ruling, which stay bash; these are `hw next`'s copies.

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type nextWitness struct{ verdict, detail string }

type nextSender struct {
	vendor, session, endpoint string
	witness                   nextWitness
}

// nextRPC runs "$HW_BIN_DIR/herdr-rpc" <args>: stdout, and the exit status.
func (a *app) nextRPC(args ...string) (string, int) {
	cmd := exec.Command(a.nextBinDir()+"/herdr-rpc", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	rc := exitStatus(cmd.Run())
	return out.String(), rc
}

// nextEnvInt is `: "${VAR:=def}"` read as a number.
func (a *app) nextEnvStr(k, def string) string { return a.nextEnvOr(k, def) }

func nextSleep(s string) {
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		time.Sleep(time.Duration(f * float64(time.Second)))
	}
}

// ── the route ───────────────────────────────────────────────────────────────

// nextResolveSender is invoker_resolve_sender <pane>: a named pane's vendor is read from herdr.
func (a *app) nextResolveSender(pane string) (vendor, session, endpoint string) {
	if pane == "" {
		return "", "", ""
	}
	info := nextHerdrSub("agent", "get", pane)
	session = jqRawOr(info, "", "result", "agent", "agent_session", "value")
	vendor = jqRawOr(info, "", "result", "agent", "agent")
	switch vendor {
	case "opencode":
		proc := nextHerdrSub("pane", "process-info", "--pane", pane)
		if port := nextOpencodePort(proc, false); port != "" {
			endpoint = "http://127.0.0.1:" + port
		}
	case "claude", "codex":
		endpoint = "-"
	}
	return
}

// nextOpencodePort is the jq both copies run over `herdr pane process-info`: the argv of the first
// foreground process that is opencode (bare name; the witness's copy also takes a path ending /opencode),
// and the word after its --port.
func nextOpencodePort(proc string, orPath bool) string {
	vals, _ := parseStream([]byte(proc))
	var outs []string
	for _, v := range vals {
		procs, ok := nextPath(v, "result", "process_info", "foreground_processes")
		if !ok || procs.kind != vArr {
			continue
		}
		var argv *jval
		for _, p := range procs.arr {
			av, ok := p.get("argv")
			if !ok || av.kind != vArr {
				continue
			}
			hit := false
			for _, x := range av.arr {
				if x.kind != vStr {
					continue
				}
				if x.s == "opencode" || (orPath && strings.HasSuffix(x.s, "/opencode")) {
					hit = true
					break
				}
			}
			if hit {
				argv = av
				break
			}
		}
		if argv == nil {
			continue
		}
		for i, x := range argv.arr {
			if x.kind == vStr && x.s == "--port" {
				if i+1 < len(argv.arr) && argv.arr[i+1].truthy() {
					outs = append(outs, argv.arr[i+1].raw())
				}
				break
			}
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

func nextPath(v *jval, path ...string) (*jval, bool) {
	cur := v
	for _, p := range path {
		var ok bool
		if cur, ok = cur.get(p); !ok {
			return nil, false
		}
	}
	return cur, true
}

// nextVendorNative is _vendor_has_native_transport.
func nextVendorNative(v string) bool { return v != "" && v != "claude" && v != "codex" }

// nextResolveRoute is cmd_next's route block: the native route when the vendor has one and both its session
// and its endpoint are known; an OpenCode pane whose session herdr omits while blocked/stuck-but-finished takes
// the session its launch receipt bound to this pane. info is the snapshot the stale-stuck test reads, or ""
// for the re-resolution after the gate (which tests the vendor alone, as the bash does there).
func (a *app) nextResolveRoute(s *nextState, info string) {
	v, ses, ep := a.nextResolveSender(s.pane)
	cond := v == "opencode" && ses == "" && ep != ""
	if info != "" {
		cond = nextFinishedStaleStuck(info) && cond
	}
	if cond {
		ses = nextSessionFromReceipt(s.rundir, s.pane)
	}
	s.snd.vendor, s.snd.session, s.snd.endpoint = v, ses, ep
	if nextVendorNative(v) && ses != "" && ep != "" { // # MUTATION-ANCHOR: 862-G09
		s.route, s.target, s.endpoint = v, ses, ep
	} else {
		s.route, s.target, s.endpoint = "herdr", s.pane, "-"
	}
}

// nextSessionFromReceipt is _next_opencode_session_from_receipt: the launch receipt's last session_id,
// when it is ses_*, its vendor opencode and its pane this one.
func nextSessionFromReceipt(rundir, pane string) string {
	rc := rundir + "/receipt.jsonl"
	if !nextReadable(rc) {
		return ""
	}
	sid, vendor, rp := nextReceiptLast(rc, "session_id"), nextReceiptLast(rc, "session_vendor"), nextReceiptLast(rc, "pane")
	if strings.HasPrefix(sid, "ses_") && vendor == "opencode" && rp == pane {
		return sid
	}
	return ""
}

// nextReceiptLast is `jq -sr '[.[] | select(.key == K)][-1].value // empty' receipt.jsonl`: slurped, so a
// line jq cannot parse makes the whole answer empty.
func nextReceiptLast(path, key string) string {
	vals, bad := readStream(path)
	if bad {
		return ""
	}
	var last *jval
	for _, v := range vals {
		if v.kind != vObj {
			if v.kind == vNull {
				continue
			}
			return "" // .key of a non-object is an error
		}
		if k, ok := v.get("key"); ok && k.kind == vStr && k.s == key {
			last = v
		}
	}
	if last == nil {
		return ""
	}
	val, ok := last.get("value")
	if !ok || !val.truthy() {
		return ""
	}
	return bashSub(val.raw())
}

// ── the reads of one agent snapshot ─────────────────────────────────────────

func nextAgentField(info string, path ...string) string {
	return jqRawOr(info, "", append([]string{"result", "agent"}, path...)...)
}

// nextFinishedStaleStuck is _next_finished_stale_stuck: blocked/stuck WITH delivered completion tokens.
func nextFinishedStaleStuck(info string) bool {
	vals, perrE := parseStream([]byte(info))
	perr := perrE != nil
	if len(vals) == 0 {
		return false
	}
	st := false
	for _, v := range vals {
		get := func(p ...string) *jval {
			c, ok := nextPath(v, p...)
			if !ok {
				return nil
			}
			return c
		}
		is := func(c *jval, s string) bool { return c != nil && c.kind == vStr && c.s == s }
		ds := get("result", "agent", "tokens", "done_status")
		st = is(get("result", "agent", "agent_status"), "blocked") &&
			is(get("result", "agent", "tokens", "blocked_reason"), "stuck") &&
			(is(ds, "done") || is(ds, "blocked")) &&
			is(get("result", "agent", "tokens", "done_state"), "delivered") // # MUTATION-ANCHOR: 862-G10
	}
	if perr {
		return false
	}
	return st
}

// nextBusyMessage is _next_busy_message.
func nextBusyMessage(pane string, waitMS int64, info string) string {
	st := nextAgentField(info, "agent_status")
	reason := nextAgentField(info, "tokens", "blocked_reason")
	scope := nextAgentField(info, "tokens", "blocked_scope")
	askState := nextAgentField(info, "tokens", "ask_state")
	doneStatus := nextAgentField(info, "tokens", "done_status")
	doneState := nextAgentField(info, "tokens", "done_state")
	or := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	switch {
	case st == "blocked" && askState == "delivered" && doneStatus != "done" && doneStatus != "blocked":
		seq := jqRawOr(info, "?", "result", "agent", "tokens", "ask_seq")
		at := jqRawOr(info, "an unknown time", "result", "agent", "tokens", "ask_at")
		text := nextAskText(info)
		return pane + " is blocked on its own ask " + seq + ", delivered " + at + " and still unanswered: \"" + text + "\". Re-tasking cannot reach idle,done from here no matter how large a budget you pass — hw next gates on idle,done, and a pane parked on its own ask never gets there until the ask is answered. Answer it, then retry: herdr agent get " + pane
	case st == "blocked" && askState == "delivered":
		return pane + " already reported done (done_status=" + doneStatus + ", done_state=" + or(doneState, "unknown") + ") — the ask token you see is stale, published before that report, and there is nothing left to answer. Do not answer it: read the actual report with `herdr agent get " + pane + "` and re-task from that, or override with --force if you are deliberately replacing it."
	case st == "blocked" && reason == "permission":
		return pane + " is blocked on a PERMISSION prompt (" + or(scope, "root") + " session), not on a turn that will end. Nothing was sent and the counter did not move. hw will not dismiss it for you: dismissing a permission prompt means DENY, and that is a decision about what the executor may do, not a stuck pane to rescue. Answer it — `herdr agent get " + pane + "`, and for a child prompt press ctrl+x then down to reach it — then retry."
	case st == "blocked" && reason == "stuck" && (doneStatus == "done" || doneStatus == "blocked"):
		return pane + " already reported (done_status=" + doneStatus + ", done_state=" + or(doneState, "unknown") + ") — the blocked/stuck token you see is stale, published on a pane that has finished. Do not `hw unstick` it: that restarts OpenCode for nothing left to do. Read its report with `herdr agent get " + pane + "`, then `hw next " + pane + " \"<next task>\"` to reuse it or `hw done <project> <task>` to close it."
	case st == "blocked" && reason == "stuck":
		return pane + " is blocked with NOTHING LEFT TO ANSWER: every prompt it raised has been answered or dismissed (last one in its " + or(scope, "root") + " session) and herdr still reports blocked. Nothing was sent and the counter did not move. No budget reaches idle from here and there is no modal for a human to clear, so do not retry with a larger HW_NEXT_WAIT_MS. A deliberate in-place recovery is available: `hw unstick " + pane + "`; it first refuses any visible question/permission, then restarts only OpenCode in the same pane (the active conversation is replaced; tab, cwd and worktree stay). If that is not acceptable, close it with `hw done` and dispatch again."
	case st == "blocked" && reason == "error":
		return pane + " is blocked on a SESSION ERROR, not on input — there is no modal to dismiss and no budget that reaches idle. Nothing was sent and the counter did not move. Read what failed with `herdr agent get " + pane + "`; if that session is unrecoverable this is a relaunch, not a re-task."
	}
	return pane + " was still busy after " + strconv.FormatInt(waitMS/1000, 10) + "s (agent_status=" + st + "), so nothing was sent and the task counter did not move. A mid-turn prompt is NOT dropped — it is acted on — but task N+1 answered inside task N is answered against the wrong state, with the counter already advanced, which is what this gate is for. Retry, or raise the budget: HW_NEXT_WAIT_MS=540000 hw next …" // # MUTATION-ANCHOR: 862-G11
}

// nextAskText is `jq -r '[.result.agent.tokens | .ask, .ask2, … .ask8] | map(select(. != null)) | join("")'`.
func nextAskText(info string) string {
	vals, _ := parseStream([]byte(info))
	var outs []string
	for _, v := range vals {
		tk, ok := nextPath(v, "result", "agent", "tokens")
		if !ok {
			continue
		}
		var sb strings.Builder
		bad := false
		for _, k := range []string{"ask", "ask2", "ask3", "ask4", "ask5", "ask6", "ask7", "ask8"} {
			if tk.kind == vNull {
				continue
			}
			c, ok := tk.get(k)
			if !ok {
				bad = true
				break
			}
			switch c.kind {
			case vNull:
			case vStr, vNum, vBool:
				sb.WriteString(c.tostring())
			default:
				bad = true
			}
		}
		if !bad {
			outs = append(outs, sb.String())
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

// nextSettledMessage is _next_settled_message.
func nextSettledMessage(pane string, waitMS int64, info string) string {
	st := nextAgentField(info, "agent_status")
	reason := nextAgentField(info, "tokens", "blocked_reason")
	scope := nextAgentField(info, "tokens", "blocked_scope")
	doneStatus := nextAgentField(info, "tokens", "done_status")
	doneState := nextAgentField(info, "tokens", "done_state")
	or := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	m := pane + ": NOTHING WAS SENT and the task counter did not move. "
	switch {
	case st == "working" && (doneStatus == "done" || doneStatus == "blocked"):
		m += "herdr calls it `working`, but it already reported (done_status=" + doneStatus + ") and the cross-check above finds nothing running and no prompt outstanding. That `working` is STALE — the lifecycle producer never cleared it — so the state you are waiting on will not change, at any budget. This executor is finished: read its report with `herdr agent get " + pane + "` and either re-task with --force (which files the unreported task as abandoned — check first whether the report simply failed to reach you) or close it with `hw done`."
	case st == "working":
		m += "herdr calls it `working` and the cross-check above finds nothing running and no prompt outstanding, so that `working` is not a turn in progress. Do NOT raise HW_NEXT_WAIT_MS: the budget is not what is missing. Look at the pane, and if it has genuinely finished without reporting, close it with `hw done` or re-task with --force."
	case st == "blocked" && reason == "permission":
		m += "herdr calls it blocked on a PERMISSION prompt (" + or(scope, "root") + " session), but its own endpoint reports no permission outstanding — that token is STALE, and there is no modal for anyone to answer. Do not go looking for a prompt to approve. `hw unstick` will not touch this either, because the signature is `permission`, not `stuck`. Read what it last did with `herdr agent get " + pane + "`, then close it with `hw done` and dispatch again."
	case st == "blocked" && reason == "question":
		m += "herdr calls it blocked on a QUESTION (" + or(scope, "root") + " session), but its own endpoint reports no question outstanding, and the dismissal attempt above found no modal on screen. That token is STALE. Read what it last did with `herdr agent get " + pane + "`, then close it with `hw done` and dispatch again."
	case st == "blocked" && reason == "error":
		m += "herdr calls it blocked on a SESSION ERROR and the cross-check confirms nothing is running and nothing is outstanding — so there is no modal, and no budget reaches idle. Read what failed with `herdr agent get " + pane + "`; an unrecoverable session is a relaunch, not a re-task."
	case st == "blocked" && reason == "stuck" && (doneStatus == "done" || doneStatus == "blocked"):
		m += "herdr calls it blocked/stuck, but it already reported (done_status=" + doneStatus + ", done_state=" + or(doneState, "unknown") + ") — that token is STALE, published on a pane that has finished, and the cross-check above only confirms nothing is CURRENTLY moving, not why. Do not `hw unstick` it. Read its report with `herdr agent get " + pane + "`, then `hw next " + pane + " \"<next task>\"` to reuse it or `hw done <project> <task>` to close it."
	case st == "blocked" && reason == "stuck":
		m += "herdr calls it blocked/stuck (" + or(scope, "root") + " session) and the cross-check CONFIRMS it: nothing is moving and nothing is outstanding. This is the one case `hw unstick " + pane + "` is for — it re-runs this same cross-check before it touches anything, and refuses if the pane turns out to be working. If an in-place restart is not acceptable, close it with `hw done` and dispatch again."
	default:
		m += "herdr calls it `" + or(st, "unknown") + "`, and the cross-check above finds nothing moving and no prompt outstanding, so that state cannot change on its own. Raising the budget past " + strconv.FormatInt(waitMS/1000, 10) + "s cannot help. Look at the pane and decide: `hw done` and dispatch again, or --force if you are deliberately replacing an executor that will never report."
	}
	return m
}

// ── the question modal (invoker_clear_opencode_question) ────────────────────

// nextClearQuestion: 0 dismissed, 1 nothing of that shape (or the key did not move it), 2 a reason that
// must not be auto-dismissed.
func (a *app) nextClearQuestion(pane string) int {
	info, _ := a.nextRPC("call", "agent.get", `{"target":"`+pane+`"}`)
	info = bashSub(info)
	if info == "" {
		return 1
	}
	state := jqRawOr(info, "", "agent", "agent_status")
	agent := jqRawOr(info, "", "agent", "agent")
	if state != "blocked" || agent != "opencode" {
		return 1
	}
	reason := jqRawOr(info, "", "agent", "tokens", "blocked_reason")
	scope := jqRawOr(info, "", "agent", "tokens", "blocked_scope")
	if scope == "" {
		scope = "root"
	}
	switch reason {
	case "permission", "error", "stuck":
		return 2
	case "question", "":
	default:
		return 1
	}
	if scope == "child" && reason == "question" {
		if _, rc := a.nextRPC("call", "pane.send_keys", `{"pane_id":"`+pane+`","keys":["ctrl+x","down"]}`); rc != 0 {
			return 1
		}
		nextSleep(a.nextEnvStr("INVOKER_SUBAGENT_VIEW_SETTLE_S", "2"))
	}
	out, _ := a.nextRPC("call", "pane.read", `{"pane_id":"`+pane+`","source":"visible","lines":`+a.nextEnvStr("INVOKER_MODAL_READ_LINES", "80")+`}`)
	text := jqRawOr(out, "", "read", "text")
	if !strings.Contains(text, "esc dismiss") { // # MUTATION-ANCHOR: 862-G12
		return 1
	}
	if _, rc := a.nextRPC("call", "pane.send_keys", `{"pane_id":"`+pane+`","keys":["escape"]}`); rc != 0 {
		return 1
	}
	if _, rc := a.nextRPC("wait-agent", pane, "idle,done", "--timeout-ms", a.nextEnvStr("INVOKER_UNBLOCK_WAIT_MS", "15000")); rc != 0 {
		return 1
	}
	return 0
}

// ── the witness (the bash state witness) ──────────────────────────────────────

// nextScreenHash is witness_screen_hash: the first 16 hex of sha256 of the visible text, or false.
func (a *app) nextScreenHash(pane string) (string, bool) {
	out, rc := a.nextRPC("call", "pane.read", `{"pane_id":"`+pane+`","source":"visible","lines":`+a.nextEnvStr("WITNESS_READ_LINES", "40")+`}`)
	out = bashSub(out)
	if rc != 0 || out == "" {
		return "", false
	}
	vals, perrE := parseStream([]byte(out))
	perr := perrE != nil
	if perr || len(vals) == 0 {
		return "", false
	}
	v := vals[len(vals)-1]
	if v.kind != vObj {
		return "", false
	}
	rd, ok := v.get("read")
	if !ok || rd.kind != vObj {
		return "", false
	}
	t, ok := rd.get("text")
	if !ok || t.kind != vStr {
		return "", false
	}
	var sb strings.Builder
	for _, x := range vals { // jq -j '.read.text' over every value
		if r, ok := nextPath(x, "read", "text"); ok {
			sb.WriteString(r.raw())
		}
	}
	return nextSha16(sb.String()), true
}

// nextScreen is witness_screen: "moved <hash>" / "static <hash>", or false when any sample failed.
func (a *app) nextScreen(pane string) (string, string, bool) {
	first, ok := a.nextScreenHash(pane)
	if !ok {
		return "", "", false
	}
	samples, _ := strconv.Atoi(a.nextEnvStr("WITNESS_SAMPLES", "3"))
	for i := 1; i < samples; i++ {
		nextSleep(a.nextEnvStr("WITNESS_INTERVAL_S", "2"))
		now, ok := a.nextScreenHash(pane)
		if !ok {
			return "", "", false
		}
		if now != first {
			return "moved", first, true
		}
	}
	return "static", first, true
}

// nextWitnessEndpoint is witness_endpoint.
func (a *app) nextWitnessEndpoint(pane string) string {
	info := nextHerdrSub("pane", "process-info", "--pane", pane)
	if info == "" {
		return ""
	}
	port := nextOpencodePort(info, true)
	if port == "" || strings.Trim(port, "0123456789") != "" {
		return ""
	}
	return "http://127.0.0.1:" + port
}

// nextWitnessPrompts is witness_prompts: the two counts, or false unless both answered as JSON arrays.
func (a *app) nextWitnessPrompts(endpoint string) (string, string, bool) {
	t := a.nextEnvStr("WITNESS_CURL_TIMEOUT_S", "5")
	curl := func(path string) (string, bool) {
		cmd := exec.Command("curl", "-s", "-m", t, endpoint+path)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return "", false
		}
		return bashSub(out.String()), true
	}
	q, ok := curl("/question")
	if !ok {
		return "", "", false
	}
	p, ok := curl("/permission")
	if !ok {
		return "", "", false
	}
	n := func(s string) (string, bool) {
		vals, perrE := parseStream([]byte(s))
		perr := perrE != nil
		if perr || len(vals) == 0 {
			return "", false
		}
		var outs []string
		for _, v := range vals {
			if v.kind == vArr {
				outs = append(outs, strconv.Itoa(len(v.arr)))
			}
		}
		if len(outs) == 0 { // jq -e: no output at all is exit 4
			return "", false
		}
		return bashSub(strings.Join(outs, "\n")), true
	}
	qn, ok := n(q)
	if !ok {
		return "", "", false
	}
	pn, ok := n(p)
	if !ok {
		return "", "", false
	}
	return qn, pn, true
}

// nextWitnessState is witness_state.
func (a *app) nextWitnessState(pane string, w *nextWitness) {
	info := nextHerdrSub("agent", "get", pane)
	status := nextAgentField(info, "agent_status")
	reason := nextAgentField(info, "tokens", "blocked_reason")
	scope := nextAgentField(info, "tokens", "blocked_scope")
	st := status
	if st == "" {
		st = "unreadable"
	}
	claim := "agent_status=" + st
	if reason != "" {
		claim += "/" + reason
		if scope != "" {
			claim += ":" + scope
		}
	}
	samples, interval := a.nextEnvStr("WITNESS_SAMPLES", "3"), a.nextEnvStr("WITNESS_INTERVAL_S", "2")
	endpoint := a.nextWitnessEndpoint(pane)
	var qn, pn string
	prompts := false
	if endpoint != "" {
		qn, pn, prompts = a.nextWitnessPrompts(endpoint)
	}
	if prompts {
		q, _ := strconv.Atoi(qn)
		p, _ := strconv.Atoi(pn)
		if q > 0 || p > 0 { // # MUTATION-ANCHOR: 862-G13
			w.verdict = "awaiting-human"
			w.detail = pane + " says " + claim + ", and its own endpoint " + endpoint + " CONFIRMS a real prompt is outstanding (/question " + qn + ", /permission " + pn + "). A person has to answer that; no budget reaches idle without one."
			return
		}
	}
	state, hash, ok := a.nextScreen(pane)
	if !ok {
		state = "unreadable"
	}
	if state == "moved" {
		w.verdict = "live"
		w.detail = pane + " says " + claim + ", but its own visible screen CHANGED between samples " + interval + "s apart (pane.read source=visible), so that pane is doing work whatever the status field says. This is a real wait and it keeps its full budget."
		return
	}
	if state == "unreadable" {
		w.verdict = "unknown"
		w.detail = pane + " says " + claim + ", and the cross-check could NOT be made: its visible screen could not be read (pane.read source=visible failed or returned no text field). That is not evidence the pane is live and not evidence it is stale, so the gate keeps its normal budget."
		return
	}
	if endpoint == "" {
		w.verdict = "unknown"
		w.detail = pane + " says " + claim + " and its visible screen was STATIC across " + samples + " samples " + interval + "s apart (hash " + hash + ") — but it exposes no OpenCode HTTP endpoint (not opencode, or no --port in its argv), so whether a prompt is outstanding could not be established. A static screen alone does not distinguish a stale state from a pane holding a modal this check cannot see, so the gate keeps its normal budget."
		return
	}
	if !prompts {
		w.verdict = "unknown"
		w.detail = pane + " says " + claim + " and its visible screen was STATIC across " + samples + " samples " + interval + "s apart (hash " + hash + ") — but its endpoint " + endpoint + " did not answer /question and /permission with parseable arrays, so an outstanding prompt could not be ruled out. An authority that cannot be asked is not an authority that said no; the gate keeps its normal budget."
		return
	}
	w.verdict = "settled"
	w.detail = pane + " says " + claim + ", and every independent check CONTRADICTS it: its visible screen did not change across " + samples + " samples " + interval + "s apart (hash " + hash + ", pane.read source=visible), and its own endpoint " + endpoint + " reports nothing outstanding (/question " + qn + ", /permission " + pn + "). Nothing is moving and there is no prompt for anyone to answer, so that state cannot change on its own and no budget reaches idle from here."
}

// nextWitnessWait is witness_wait: 0 idle,done; 2 budget spent; 5 proved unable to change; 6 a person is
// waited on; 3/4 herdr-rpc's own.
func (a *app) nextWitnessWait(pane string, totalMS int64, w *nextWitness) int {
	w.verdict, w.detail = "", ""
	slice, _ := strconv.ParseInt(a.nextEnvStr("WITNESS_SLICE_MS", "20000"), 10, 64)
	confirm, _ := strconv.Atoi(a.nextEnvStr("WITNESS_CONFIRMATIONS", "2"))
	wait := func(ms int64) int {
		_, rc := a.nextRPC("wait-agent", pane, "idle,done", "--timeout-ms", strconv.FormatInt(ms, 10))
		return rc
	}
	if totalMS <= slice {
		return wait(totalMS)
	}
	deadline := nextEpoch() + totalMS/1000
	settled, human := 0, 0
	for {
		remaining := (deadline - nextEpoch()) * 1000
		if remaining <= 0 {
			return 2
		}
		ms := slice
		if ms > remaining {
			ms = remaining
		}
		switch rc := wait(ms); rc {
		case 0:
			return 0
		case 2:
		default:
			return rc
		}
		if nextEpoch() >= deadline {
			return 2
		}
		a.nextWitnessState(pane, w)
		switch w.verdict {
		case "settled":
			settled++
			human = 0
			if settled >= confirm { // # MUTATION-ANCHOR: 862-G14
				return 5
			}
		case "awaiting-human":
			human++
			settled = 0
			if human >= confirm {
				return 6
			}
		default:
			settled, human = 0, 0
		}
	}
}
