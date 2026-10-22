package main

// wait, wait-one and wait-monitor (train T2): the witness-backed way to wait for an executor, with the exit
// contract of bin/hw:11066 — 0 reached idle/done, 2 the budget was spent, 5 settled, 6 a person is waited on,
// 7 ended without delivering its report, 8 handback refused, and herdr-rpc's own 3 and 4 passed through.
//
// Ports of cmd_wait_one, cmd_wait_monitor, cmd_wait (bin/hw:10932-11120) and of the bash state witness (the cross-check:
// can this state still change?). Nothing here writes to a pane. `hw wait` asks herdr for one tab and one command in it
// (the monitor), which is the only thing hw-core does to herdr in T2.
//
// Two programs are still run as the bash runs them, because they ARE the contract and not code to port: herdr-rpc
// (`wait-agent`, which opens one events.wait per status, and `call pane.read`) and curl (the pane's own OpenCode
// endpoint). They are found beside the hw that started hw-core (HW_CORE_BIN_DIR), exactly where the bash state witness finds
// them. Their exit codes are the ones the verbs report.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// the exit contract of the wait family (bin/hw cmd_wait_one): one code per fact. 3 and 4 are herdr-rpc's own
// (could not connect / rpc error) and pass through unchanged.
const (
	rcTimeout  = 2 // # MUTATION-ANCHOR: 871-G24
	rcSettled  = 5 // # MUTATION-ANCHOR: 871-G22
	rcHuman    = 6 // # MUTATION-ANCHOR: 871-G23
	rcStranded = 7 // # MUTATION-ANCHOR: 871-G21
	rcRefused  = 8 // # MUTATION-ANCHOR: 871-G20
)

// binDir is HW_BIN_DIR: where the hw that started this process lives (the shim hands it down).
func (a *app) binDir() string {
	if d := a.getenv("HW_CORE_BIN_DIR"); d != "" {
		return d
	}
	return filepath.Join(a.c.brain, "bin")
}

// operator is ${HW_OPERATOR:-<the table's operator>}.
func (a *app) operator() string {
	if o := a.getenv("HW_OPERATOR"); o != "" {
		return o
	}
	return a.c.t.operator
}

// nowSec is `date +%s`; a variable so the loop of witness.wait can be driven by a clock a test owns.
var nowSec = func() int64 { return time.Now().Unix() }

// envInt is `: "${NAME:=default}"` read as a number; anything else is the default.
func (a *app) envInt(name string, def int64) int64 {
	v := a.getenv(name)
	if v == "" {
		return def
	}
	n, ok := pyInt64(v)
	if !ok {
		return def
	}
	return n
}

// ── herdr agent get ─────────────────────────────────────────────────────────

// agentGet is `herdr agent get <pane> 2>/dev/null || true`.
func (a *app) agentGet(pane string) string {
	out, _ := herdrOut(0, "agent", "get", pane)
	return bashSub(out)
}

// jqE is `printf '%s' text | jq -e <path>` as a condition: the last document's value at the path is neither
// null nor false, and every document parsed.
func jqE(text string, path ...string) bool {
	vals, err := parseStream([]byte(text))
	if err != nil || len(vals) == 0 {
		return false
	}
	cur := vals[len(vals)-1]
	for _, p := range path {
		next, ok := cur.get(p)
		if !ok {
			return false
		}
		cur = next
	}
	return cur.truthy()
}

func tokenOf(info, key string) string { return jqRawOr(info, "", "result", "agent", "tokens", key) }

// waitFalsePair is _wait_false_pair: a description when the pane carries BOTH halves of the broken state.
func waitFalsePair(info string) string {
	if tokenOf(info, "turn_state") != "ended_unreported" || tokenOf(info, "done_state") != "undelivered" {
		return ""
	}
	doneStatus, turnAt := tokenOf(info, "done_status"), tokenOf(info, "turn_ended_at")
	if doneStatus == "" {
		doneStatus = "absent"
	}
	at := ""
	if turnAt != "" {
		at = " at " + turnAt
	}
	return "it published done_status=" + doneStatus + " with done_state=undelivered — so the report exists and did NOT reach the brainer — and its turn-end adapter published turn_state=ended_unreported" + at + ", so the turn that would have retried the delivery is over"
}

// settledAlreadyReportedNote is _settled_already_reported_note.
func (a *app) settledAlreadyReportedNote(info string) {
	doneStatus := tokenOf(info, "done_status")
	if doneStatus != "done" && doneStatus != "blocked" {
		return
	}
	doneState := tokenOf(info, "done_state")
	if doneState == "" {
		doneState = "unknown"
	}
	a.warn("it already reported (done_status=" + doneStatus + ", done_state=" + doneState + ") — whatever blocked/stuck token herdr is still showing is stale, published on a pane that has finished. Do not `hw unstick` it. `hw next` to reuse it, or `hw done` to close it.")
}

// ── the witness (the bash state witness) ──────────────────────────────────────

type witness struct {
	a       *app
	rpc     string
	verdict string
	detail  string
	// onVerdict is WITNESS_ON_VERDICT: told what each round decided, never able to end the wait
	onVerdict func(verdict, detail string, elapsed, remaining int64)
	// rpcFn and stateFn stand in for herdr-rpc and the witness's observation in a unit test of the loop
	rpcFn   func(pane string, ms int64) int
	stateFn func(w *witness, pane string)
}

func (a *app) newWitness() *witness {
	return &witness{a: a, rpc: filepath.Join(a.binDir(), "herdr-rpc")}
}

// screenHash is witness_screen_hash: one hash of the pane's visible screen, or false when the read did not succeed.
func (w *witness) screenHash(pane string) (string, bool) {
	params := fmt.Sprintf(`{"pane_id":"%s","source":"visible","lines":%s}`, pane, orStr(w.a.getenv("WITNESS_READ_LINES"), "40"))
	cmd := exec.Command(w.rpc, "call", "pane.read", params)
	outb, err := cmd.Output()
	if err != nil {
		return "", false
	}
	out := bashSub(string(outb))
	if out == "" {
		return "", false
	}
	vals, perr := parseStream([]byte(out))
	if perr != nil || len(vals) == 0 {
		return "", false
	}
	root := vals[len(vals)-1]
	if root.kind != vObj {
		return "", false
	}
	read, ok := root.get("read")
	if !ok || read.kind != vObj {
		return "", false
	}
	for _, m := range root.obj {
		if m.k == "read" {
			read = m.v
		}
	}
	var text *jval
	for _, m := range read.obj {
		if m.k == "text" {
			text = m.v
		}
	}
	if text == nil || text.kind != vStr {
		return "", false
	}
	// `jq -j '.read.text'` prints the text of EVERY document of the stream, run together: a first
	// document that moved while the last stayed put is still a screen that moved. A document jq
	// would error on contributes nothing, as there.
	var all strings.Builder
	for _, doc := range vals {
		r, ok := doc.get("read")
		if !ok {
			continue
		}
		t, ok := r.get("text")
		if !ok {
			continue
		}
		all.WriteString(t.raw())
	}
	sum := sha256.Sum256([]byte(all.String()))
	return hex.EncodeToString(sum[:])[:16], true
}

// screen is witness_screen: "moved <hash>" or "static <hash>"; false when ANY sample failed to read.
func (w *witness) screen(pane string) (string, string, bool) {
	samples := w.a.envInt("WITNESS_SAMPLES", 3)
	first, ok := w.screenHash(pane)
	if !ok {
		return "", "", false
	}
	for i := int64(1); i < samples; i++ {
		if s, err := strconv.ParseFloat(orStr(w.a.getenv("WITNESS_INTERVAL_S"), "2"), 64); err == nil && s > 0 {
			time.Sleep(time.Duration(s * float64(time.Second)))
		}
		now, ok := w.screenHash(pane)
		if !ok {
			return "", "", false
		}
		if now != first {
			return "moved", first, true
		}
	}
	return "static", first, true
}

// endpoint is witness_endpoint: the pane's own OpenCode HTTP address, from its argv; false when there is none to ask.
func (w *witness) endpoint(pane string) (string, bool) {
	out, _ := herdrOut(0, "pane", "process-info", "--pane", pane)
	info := bashSub(out)
	if info == "" {
		return "", false
	}
	vals, err := parseStream([]byte(info))
	if err != nil {
		return "", false
	}
	port := ""
	for _, doc := range vals {
		cur := doc
		ok := true
		for _, p := range []string{"result", "process_info", "foreground_processes"} {
			if cur, ok = cur.get(p); !ok {
				break
			}
		}
		if !ok || cur.kind != vArr {
			continue
		}
		for _, fp := range cur.arr {
			argv, ok := fp.get("argv")
			if !ok || argv.kind != vArr {
				continue
			}
			isOpencode := false
			for _, e := range argv.arr {
				if e.kind == vStr && (e.s == "opencode" || strings.HasSuffix(e.s, "/opencode")) {
					isOpencode = true
				}
			}
			if !isOpencode {
				continue
			}
			for i, e := range argv.arr { // index("--port"), then the element after it
				if e.kind == vStr && e.s == "--port" {
					if i+1 < len(argv.arr) && argv.arr[i+1].truthy() {
						port = argv.arr[i+1].raw()
					}
					break
				}
			}
			break // first(...): only the first opencode process answers
		}
		if port != "" {
			break
		}
	}
	if !allDigits(port) {
		return "", false
	}
	return "http://127.0.0.1:" + port, true
}

// prompts is witness_prompts: "<question_count> <permission_count>"; false unless BOTH answered and BOTH parsed as arrays.
func (w *witness) prompts(endpoint string) (int, int, bool) {
	timeout := orStr(w.a.getenv("WITNESS_CURL_TIMEOUT_S"), "5")
	count := func(url string) (int, bool) {
		out, err := exec.Command("curl", "-s", "-m", timeout, url).Output()
		if err != nil {
			return 0, false
		}
		vals, perr := parseStream([]byte(bashSub(string(out))))
		if perr != nil || len(vals) != 1 || vals[0].kind != vArr {
			return 0, false
		}
		return len(vals[0].arr), true
	}
	q, ok := count(endpoint + "/question")
	if !ok {
		return 0, 0, false
	}
	p, ok := count(endpoint + "/permission")
	if !ok {
		return 0, 0, false
	}
	return q, p, true
}

// state is witness_state: sets verdict and detail. It never fails.
func (w *witness) state(pane string) {
	a := w.a
	info := a.agentGet(pane)
	status := jqRawOr(info, "", "result", "agent", "agent_status")
	reason := jqRawOr(info, "", "result", "agent", "tokens", "blocked_reason")
	scope := jqRawOr(info, "", "result", "agent", "tokens", "blocked_scope")
	claim := "agent_status=" + orStr(status, "unreadable")
	if reason != "" {
		claim += "/" + reason
		if scope != "" {
			claim += ":" + scope
		}
	}
	samples, interval := itoa(int(a.envInt("WITNESS_SAMPLES", 3))), orStr(a.getenv("WITNESS_INTERVAL_S"), "2")
	// the endpoint first, because an outstanding prompt outranks everything else
	endpoint, haveEndpoint := w.endpoint(pane)
	qn, pn, havePrompts := 0, 0, false
	if haveEndpoint {
		qn, pn, havePrompts = w.prompts(endpoint)
	}
	if havePrompts && (qn > 0 || pn > 0) {
		w.verdict = "awaiting-human"
		w.detail = pane + " says " + claim + ", and its own endpoint " + endpoint + " CONFIRMS a real prompt is outstanding (/question " + itoa(qn) + ", /permission " + itoa(pn) + "). A person has to answer that; no budget reaches idle without one."
		return
	}
	screenState, screenHash, ok := w.screen(pane)
	if !ok {
		screenState, screenHash = "unreadable", ""
	}
	if screenState == "moved" {
		w.verdict = "live"
		w.detail = pane + " says " + claim + ", but its own visible screen CHANGED between samples " + interval + "s apart (pane.read source=visible), so that pane is doing work whatever the status field says. This is a real wait and it keeps its full budget."
		return
	}
	// From here the screen is static or unreadable, and no prompt was positively read.
	if screenState == "unreadable" {
		w.verdict = "unknown"
		w.detail = pane + " says " + claim + ", and the cross-check could NOT be made: its visible screen could not be read (pane.read source=visible failed or returned no text field). That is not evidence the pane is live and not evidence it is stale, so the gate keeps its normal budget."
		return
	}
	if !haveEndpoint {
		w.verdict = "unknown"
		w.detail = pane + " says " + claim + " and its visible screen was STATIC across " + samples + " samples " + interval + "s apart (hash " + screenHash + ") — but it exposes no OpenCode HTTP endpoint (not opencode, or no --port in its argv), so whether a prompt is outstanding could not be established. A static screen alone does not distinguish a stale state from a pane holding a modal this check cannot see, so the gate keeps its normal budget."
		return
	}
	if !havePrompts {
		w.verdict = "unknown"
		w.detail = pane + " says " + claim + " and its visible screen was STATIC across " + samples + " samples " + interval + "s apart (hash " + screenHash + ") — but its endpoint " + endpoint + " did not answer /question and /permission with parseable arrays, so an outstanding prompt could not be ruled out. An authority that cannot be asked is not an authority that said no; the gate keeps its normal budget."
		return
	}
	w.verdict = "settled"
	w.detail = pane + " says " + claim + ", and every independent check CONTRADICTS it: its visible screen did not change across " + samples + " samples " + interval + "s apart (hash " + screenHash + ", pane.read source=visible), and its own endpoint " + endpoint + " reports nothing outstanding (/question " + itoa(qn) + ", /permission " + itoa(pn) + "). Nothing is moving and there is no prompt for anyone to answer, so that state cannot change on its own and no budget reaches idle from here."
}

// rpcWait is witness_rpc_wait: one `wait-agent`, its status.
func (w *witness) rpcWait(pane string, ms int64) int {
	if w.rpcFn != nil {
		return w.rpcFn(pane, ms)
	}
	cmd := exec.Command(w.rpc, "wait-agent", pane, "idle,done", "--timeout-ms", strconv.FormatInt(ms, 10))
	return exitStatus(cmd.Run())
}

// wait is witness_wait: 0 idle/done, 2 the budget was spent, 5 settled, 6 a person is waited on, else herdr-rpc's own.
func (w *witness) wait(pane string, totalMS int64) int {
	a := w.a
	w.verdict, w.detail = "", ""
	sliceMS := a.envInt("WITNESS_SLICE_MS", 20000)
	confirmations := a.envInt("WITNESS_CONFIRMATIONS", 2)
	// a budget smaller than one slice is a caller that has already decided it will not wait long
	if totalMS <= sliceMS {
		return w.rpcWait(pane, totalMS)
	}
	deadline := nowSec() + totalMS/1000
	var settled, human int64
	for {
		now := nowSec()
		remaining := (deadline - now) * 1000
		if remaining <= 0 {
			return rcTimeout
		}
		slice := sliceMS
		if slice > remaining {
			slice = remaining
		}
		rc := w.rpcWait(pane, slice)
		// ONLY A TIMEOUT IS WORTH ANOTHER SLICE: 3 and 4 are facts about the transport, passed straight back
		switch rc {
		case 0:
			return 0
		case 2:
		default:
			return rc
		}
		// spend nothing on a witness whose answer can no longer change the outcome
		if nowSec() >= deadline {
			return rcTimeout
		}
		if w.stateFn != nil {
			w.stateFn(w, pane)
		} else {
			w.state(pane)
		}
		if w.onVerdict != nil {
			w.onVerdict(w.verdict, w.detail, nowSec()-(deadline-totalMS/1000), deadline-nowSec())
		}
		switch w.verdict {
		case "settled":
			settled++
			human = 0
			if settled >= confirmations {
				return rcSettled
			}
		case "awaiting-human":
			human++
			settled = 0
			if human >= confirmations {
				return rcHuman
			}
		default:
			settled, human = 0, 0
		}
	}
}

// ── hw wait-one ─────────────────────────────────────────────────────────────

type waitProgress struct {
	a        *app
	pane     string
	printed  bool
	lastText string
}

// progress is _wait_progress: the evidence is said once, then short heartbeats.
func (p *waitProgress) progress(verdict, detail string, elapsed, remaining int64) {
	a, s, r := p.a, strconv.FormatInt(elapsed, 10), strconv.FormatInt(remaining, 10)
	target := orStr(p.pane, "target")
	if p.printed {
		switch verdict {
		case "live":
			a.info(target + ": still LIVE after " + s + "s (" + r + "s left)")
		case "settled":
			a.info(target + ": still SETTLED after " + s + "s; confirming")
		case "awaiting-human":
			a.info(target + ": still awaiting a PERSON after " + s + "s; confirming")
		case "unknown":
			a.info(target + ": cross-check still unavailable after " + s + "s (" + r + "s left)")
		}
		return
	}
	p.lastText = detail
	p.printed = true
	switch verdict {
	case "live":
		a.info("still LIVE after " + s + "s (" + r + "s of budget left) — its screen moved between samples, so this is a real wait. " + detail)
	case "settled":
		a.info("cross-check says SETTLED after " + s + "s; confirming once more before ending the wait. " + detail)
	case "awaiting-human":
		a.info("cross-check says a PERSON is being waited on, after " + s + "s; confirming once more. " + detail)
	case "unknown":
		a.info("cross-check UNAVAILABLE after " + s + "s (" + r + "s left) — keeping the ordinary budget. " + detail)
	}
}

// timeoutSummary is _wait_timeout_summary.
func timeoutSummary(w *witness, p *waitProgress, pane string, waitMS int64) string {
	secs := strconv.FormatInt(waitMS/1000, 10)
	if w.detail != "" && p.printed {
		switch w.verdict {
		case "live":
			return "budget of " + secs + "s spent and " + pane + " never reached idle/done. Last cross-check: LIVE — its screen was still moving; full witness reasoning is above."
		case "unknown":
			return "budget of " + secs + "s spent and " + pane + " never reached idle/done. Last cross-check: UNAVAILABLE — a required reader still could not be trusted; full witness reasoning is above."
		}
		return "budget of " + secs + "s spent and " + pane + " never reached idle/done. Last cross-check: " + orStr(w.verdict, "unknown") + " — full witness reasoning is above."
	}
	last := ""
	if w.detail != "" {
		last = " Last cross-check: " + w.detail
	}
	return "budget of " + secs + "s spent and " + pane + " never reached idle/done." + last
}

func (a *app) waitMS() string { return orStr(a.getenv("HW_WAIT_MS"), "900000") }

func (a *app) cmdWaitOne() int {
	pane, waitMS, dry := "", a.waitMS(), false
	usageOne := "usage: hw wait <pane> [--timeout-ms <ms>] [--dry-run]"
	args := a.args
	for len(args) > 0 {
		switch {
		case args[0] == "--dry-run":
			dry = true
			args = args[1:]
		case args[0] == "--timeout-ms":
			args = args[1:]
			if len(args) == 0 {
				a.die(usageOne)
			}
			waitMS = args[0]
			args = args[1:]
		case strings.HasPrefix(args[0], "-"):
			a.die("unknown option: " + args[0] + " (" + usageOne + ")")
		default:
			if pane != "" {
				a.die(usageOne + " — one pane at a time")
			}
			pane = args[0]
			args = args[1:]
		}
	}
	if pane == "" {
		a.die(usageOne + " — the pane of the executor you are waiting on, e.g. `hw wait w41:p7B`")
	}
	if !allDigits(waitMS) {
		a.die("--timeout-ms must be a whole number of milliseconds, got: " + waitMS)
	}
	ms, _ := pyInt64(waitMS)
	info := a.agentGet(pane)
	// an unreadable pane is a REFUSAL, not a verdict: hw never waited, so it has nothing to report about the target
	if !jqE(info, "result", "agent") {
		a.die(pane + " is not an agent pane herdr can read (herdr agent get returned nothing usable). Nothing was waited on. Check the id with `herdr agent list`.")
	}
	if tokenOf(info, "turn_state") == "handback_refused" {
		a.warn(pane + " ended a turn with no report, no delivered ask/challenge outstanding, and no named human-boundary exception.")
		a.dieRC(rcRefused, "HAND-BACK REFUSED: no wait was started. The executor has every legal exit in front of it: continue working; report with `done-invoker`; report the blocking question with `done-invoker --blocked \"<question and what would unblock it>\"`; or, only for money/third-party/irreversible-outward/priority decisions, record `hw human-boundary \"<specific boundary and why only "+a.operator()+" can decide>\"` before ending the turn.")
	}
	// BEFORE ANY BUDGET IS SPENT: the one state where waiting is provably pointless
	if pair := waitFalsePair(info); pair != "" {
		a.warn(pane + " ENDED AND NEVER REPORTED: " + pair + ".")
		a.dieRC(rcStranded, "no wait was started, because none can end well: nothing inside that pane is going to deliver the report. Read what it established with `herdr agent get "+pane+"` (tokens sum/sum2/…) and its engram observations, then close it with `hw done`. Do NOT reach for a larger budget and do NOT hw unstick it — the work is finished, only the delivery failed.")
	}
	w := a.newWitness()
	if dry {
		w.state(pane)
		switch w.verdict {
		case "live":
			a.info("WOULD WAIT, and correctly. " + w.detail)
			return 0
		case "settled":
			a.warn("WOULD NOT WAIT: " + w.detail)
			a.settledAlreadyReportedNote(info)
			return rcSettled
		case "awaiting-human":
			a.warn("WOULD NOT WAIT: " + w.detail)
			return rcHuman
		}
		a.info("UNDETERMINED, so a real `hw wait` would keep its ordinary " + waitMS + "ms budget and say the cross-check was unavailable. " + w.detail)
		return rcTimeout
	}
	slice := a.getenv("WITNESS_SLICE_MS")
	if slice == "" {
		slice = "20000"
	}
	sliceN, _ := pyInt64(slice)
	a.info("waiting on " + pane + " for up to " + strconv.FormatInt(ms/1000, 10) + "s, cross-checked every " + strconv.FormatInt(sliceN/1000, 10) + "s. Ctrl-C is safe: nothing is sent to that pane, ever.")
	prog := &waitProgress{a: a, pane: pane}
	w.onVerdict = prog.progress
	rc := w.wait(pane, ms)
	w.onVerdict = nil
	switch rc {
	case 0:
		a.ok(pane + " reached idle/done. It is ready for `hw next`, or for you to read its report.")
		return 0
	case rcSettled:
		if !prog.printed {
			a.warn(w.detail)
		}
		a.warn(pane + " is SETTLED: that state was proved unable to change, so the remaining budget would have bought nothing. Look at the pane and decide what it needs — `hw done` and dispatch again is usually it. A larger budget is never the answer here.")
		a.settledAlreadyReportedNote(info)
		return rcSettled
	case rcHuman:
		if !prog.printed {
			a.warn(w.detail)
		}
		a.warn(pane + " is waiting for a PERSON, not for time: its own endpoint confirms an outstanding prompt. Answer it on the pane — or have " + a.operator() + " answer it — and re-run this identical command. No budget answers a question.")
		return rcHuman
	case 2:
		a.warn(timeoutSummary(w, prog, pane, ms))
		return rcTimeout
	}
	// herdr-rpc's own transport codes, passed through exactly as the raw wait would: facts about the multiplexer
	a.warn("the wait itself failed (herdr-rpc exit " + itoa(rc) + ": 3 = could not connect, 4 = rpc error). That is a fact about herdr, NOT about " + pane + ", and nothing was established about that executor.")
	return rc
}

// ── hw wait-monitor ─────────────────────────────────────────────────────────

func (a *app) hwBin() string { return filepath.Join(a.binDir(), "hw") }

// waitMonitorOne is _wait_monitor_one: one target in the monitor tab, its own process.
func (a *app) waitMonitorOne(pane, waitMS string) int {
	cmd := exec.Command(a.hwBin(), "wait-one", pane, "--timeout-ms", waitMS)
	cmd.Stdout, cmd.Stderr = a.out, a.err
	rc := exitStatus(cmd.Run())
	label, body := "", ""
	switch rc {
	case 0:
		label, body = "ready", "reached idle/done"
	case 2:
		label, body = "timeout", "spent its wait budget"
	case 5:
		label, body = "settled", "proved unable to change"
	case 6:
		label, body = "awaiting-human", "confirmed a person is required"
	case 7:
		label, body = "stranded", "ended with an undelivered report"
	case 8:
		label, body = "handback-refused", "ended without a legal handoff fact"
	default:
		label, body = "wait-error-"+itoa(rc), "the monitor could not establish an executor verdict"
	}
	tab := orStr(a.getenv("HERDR_TAB_ID"), "unknown")
	herdrOut(0, "notification", "show", "wait "+label+" · "+pane, "--body", body+" — monitor tab "+tab, "--sound", "done")
	return rc
}

func (a *app) cmdWaitMonitor() int {
	waitMS := ""
	var panes []string
	usage := "usage: hw wait-monitor --timeout-ms <ms> <pane> [pane ...]"
	args := a.args
	for len(args) > 0 {
		switch {
		case args[0] == "--timeout-ms":
			args = args[1:]
			if len(args) == 0 {
				a.die(usage)
			}
			waitMS = args[0]
			args = args[1:]
		case strings.HasPrefix(args[0], "-"):
			a.die("unknown internal wait-monitor option: " + args[0])
		default:
			panes = append(panes, args[0])
			args = args[1:]
		}
	}
	if waitMS == "" || len(panes) == 0 {
		a.die(usage)
	}
	a.printf("Monitoring %d executor(s) concurrently. This tab owns the waits; the brainer remains reachable.\n", len(panes))
	rcs := make([]int, len(panes))
	var wg sync.WaitGroup
	for i, p := range panes {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			rcs[i] = a.waitMonitorOne(p, waitMS)
		}(i, p)
	}
	wg.Wait()
	rc := 0
	for _, c := range rcs {
		if c != 0 {
			rc = c
		}
	}
	a.printf("Monitor complete: every target reached a verdict. Close this tab when you have read it.\n")
	return rc
}

// ── hw wait ─────────────────────────────────────────────────────────────────

// bashQ is printf's %q for the words a pane command carries: safe characters as they are, the rest backslashed.
func bashQ(s string) string {
	if s == "" {
		return "''"
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', strings.IndexByte("%+,-./:=@_^", c) >= 0, c >= 0x80:
			sb.WriteByte(c)
		default:
			sb.WriteByte('\\')
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func (a *app) cmdWait() int {
	waitMS, dry := a.waitMS(), false
	var panes []string
	usage := "usage: hw wait <pane> [pane ...] [--timeout-ms <ms>] [--dry-run]"
	args := a.args
	for len(args) > 0 {
		switch {
		case args[0] == "--dry-run":
			dry = true
			args = args[1:]
		case args[0] == "--timeout-ms":
			args = args[1:]
			if len(args) == 0 {
				a.die(usage)
			}
			waitMS = args[0]
			args = args[1:]
		case strings.HasPrefix(args[0], "-"):
			a.die("unknown option: " + args[0] + " (" + usage + ")")
		default:
			panes = append(panes, args[0])
			args = args[1:]
		}
	}
	if len(panes) == 0 {
		a.die(usage)
	}
	if !allDigits(waitMS) {
		a.die("--timeout-ms must be a whole number of milliseconds, got: " + waitMS)
	}
	if dry {
		rc := 0
		for _, p := range panes {
			cmd := exec.Command(a.hwBin(), "wait-one", p, "--timeout-ms", waitMS, "--dry-run")
			cmd.Stdout, cmd.Stderr = a.out, a.err
			if c := exitStatus(cmd.Run()); c != 0 {
				rc = c
			}
		}
		return rc
	}
	ws := a.getenv("HERDR_WORKSPACE_ID")
	if a.getenv("HERDR_ENV") != "1" || ws == "" {
		a.die("a non-blocking wait needs a Herdr-managed caller so hw can create a separate monitor tab. Run from the brainer pane, or use --dry-run for a synchronous one-shot verdict.")
	}
	// validate the complete set first: a typo must not leave a monitor watching N-1 targets
	for _, p := range panes {
		if !jqE(a.agentGet(p), "result", "agent") {
			a.die(p + " is not an agent pane herdr can read. No monitor tab was created; fix the set and retry.")
		}
	}
	cwd, _ := os.Getwd()
	h := herdrClient{getenv: a.getenv}
	raw, rc := h.capture("tab", "create", "--workspace", ws, "--cwd", cwd, "--label", "wait · "+itoa(len(panes))+" executors", "--no-focus")
	raw = bashSub(raw)
	if rc != 0 {
		a.die("could not create the wait monitor tab: " + head3(raw))
	}
	tab := firstOf(raw, [][]string{{"result", "tab", "tab_id"}, {"result", "tab_id"}})
	monitor := firstOf(raw, [][]string{{"result", "root_pane", "pane_id"}, {"result", "pane", "pane_id"}})
	if tab == "" || monitor == "" {
		a.die("Herdr created no usable monitor tab/pane: " + head3(raw))
	}
	command := bashQ(a.hwBin()) + " wait-monitor --timeout-ms " + bashQ(waitMS)
	for _, p := range panes {
		command += " " + bashQ(p)
	}
	if _, ok := h.stdout("pane", "run", monitor, command); !ok {
		herdrOut(0, "tab", "close", tab)
		a.die("created monitor tab " + tab + " but could not start its command, so hw closed the tab and started no wait")
	}
	a.ok("monitoring " + itoa(len(panes)) + " executor(s) in tab " + tab + " (pane " + monitor + "); this brainer is NOT waiting and remains reachable")
	a.info("targets: " + strings.Join(panes, " "))
	a.info("the monitor prints each witness explanation once, then short heartbeats; each verdict also sends a system notification")
	return 0
}

// head3 is `head -3`.
func head3(s string) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.TrimRight(strings.Join(lines, ""), "\n")
}

// firstOf is `jq -r '.a.b // .c.d // empty'` for paths of plain keys.
func firstOf(text string, paths [][]string) string {
	for _, p := range paths {
		if v := jqRawOr(text, "", p...); v != "" {
			return v
		}
	}
	return ""
}
