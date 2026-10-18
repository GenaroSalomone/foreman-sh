package main

// channel-send's herdr route and the gate it shares with the claude route: the wait budget, the awaiting-child
// probe, the ruling gate, the witness wait, the delivery lock and the blocked-pane fallbacks (bin/channel-send
// lines 462-780 and 1553-2103).

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func sleepSeconds(s float64) { time.Sleep(time.Duration(s * float64(time.Second))) }

// csMS is a budget read from the environment as bash arithmetic reads it: junk is a number only to `$(( ))`, so
// ok says whether `[ "$x" -gt N ]` would have worked.
func csMS(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n, err == nil
}

func (c *chanSend) lastResort(pane string) string {
	return fmt.Sprintf(" THE ONE ROUTE LEFT, and it is not a retry: raw keystrokes — `herdr pane send-text %s \"<your text>\"` then `herdr pane send-keys %s enter`. It BYPASSES this gate: no ordering, no receipt, and no proof the receiver acted, so read the pane afterwards instead of assuming. Use it only for a message someone is already waiting on; never to hand over a task, which is `hw next`. MEASURED 2026-08-27: it opens a turn on an IDLE claude pane; whether it opens one from the state you just hit is NOT established.", pane, pane)
}

// waitBudgetFor is _wait_budget_for: the budget in ms as the text bash would print (HW_INVOKER_WAIT_MS verbatim).
func (c *chanSend) waitBudgetFor(t string) string {
	if v := c.getenv("HW_INVOKER_WAIT_MS"); v != "" {
		return v
	}
	info, _ := c.herdr().stdout("agent", "get", t)
	task := jqRawOr(info, "", "result", "agent", "tokens", "hw_task")
	ms := 90000
	if task != "" {
		ms = 540000
		c.printf("channel-send: %s is an hw executor (task %s), so the wait budget defaults to %ds instead of 90s — an executor with background work stays `working` for most of its task.\n", t, task, ms/1000)
		c.printf("channel-send: THIS CALL CAN THEREFORE TAKE UP TO %ds. Give it a caller timeout of at least %d ms, or shorten the wait with HW_INVOKER_WAIT_MS=<ms>. If your harness kills it first you will get a diagnosis, not a bare `Terminated`.\n", ms/1000, ms+15000)
	}
	return strconv.Itoa(ms)
}

// turnHasEnded is _turn_has_ended: turn_state, when it says the turn ended.
func (c *chanSend) turnHasEnded(t string) (string, bool) {
	info, _ := c.herdr().stdout("agent", "get", t)
	state := jqRawOr(info, "", "result", "agent", "tokens", "turn_state")
	if strings.HasPrefix(state, "ended") {
		return state, true
	}
	return "", false
}

// awaitingChildWarning is _awaiting_child_warning: "<turn_state>\t<children_running>\t<turn_ended_at>", or not.
func (c *chanSend) awaitingChildWarning(t string) (string, bool) {
	info, _ := c.herdr().stdout("agent", "get", t)
	if info == "" {
		return "", false
	}
	// A pane already at idle/done is at the boundary the gate wants: nothing to shorten.
	status := jqRawOr(info, "", "result", "agent", "agent_status")
	if status == "idle" || status == "done" { // # MUTATION-ANCHOR: 111-G02
		return "", false
	}
	state := jqRawOr(info, "", "result", "agent", "tokens", "turn_state")
	if !strings.HasPrefix(state, "ended") { // # MUTATION-ANCHOR: 111-G04
		return "", false
	}
	children := jqRawOr(info, "", "result", "agent", "tokens", "children_running")
	if children == "" { // # MUTATION-ANCHOR: 111-G03
		return "", false
	}
	at := jqRawOr(info, "", "result", "agent", "tokens", "turn_ended_at")
	if at == "" {
		at = "unknown"
	}
	return state + "\t" + children + "\t" + at, true
}

// awaitingChildShorten is _awaiting_child_shorten: the warning to cite when the wait is to be cut to a probe.
func (c *chanSend) awaitingChildShorten(t string, full string) (string, bool) {
	w, ok := c.awaitingChildWarning(t)
	if !ok {
		return "", false
	}
	fullMS, okN := csMS(full)
	if !okN || fullMS <= c.probeMS {
		return "", false
	}
	f := strings.Split(w, "\t")
	c.printf("channel-send: %s has WARNED that idle may be far off — its own tokens say turn_state=%s with children_running=%s. Waiting %ds instead of %ds: if it is between turns this still delivers at once, and if it is not you get a diagnosis instead of your own harness killing this call.\n",
		t, f[0], f[1], c.probeMS/1000, fullMS/1000)
	return w, true
}

// awaitingChildRefusal is _awaiting_child_refusal: it exits 1.
func (c *chanSend) awaitingChildRefusal(target, pane, warning string, fullMS int64, escape string) {
	f := strings.Split(warning, "\t")
	for len(f) < 3 {
		f = append(f, "")
	}
	c.printf("channel-send: REFUSED AFTER A BOUNDED PROBE, and NOTHING was sent: %s did not reach idle in %ds, and it had already said why it might not. Its own herdr tokens say turn_state=%s, children_running=%s, turn_ended_at=%s — its turn is over, and what has to finish before it can be idle is a background child, which runs for minutes. So this call spent %ds on the question rather than the %ds it was allowed: the receiver named the reason a long wait would be spent for nothing, and the wait was cut to a probe instead of to your caller timeout.\n",
		target, c.probeMS/1000, f[0], f[1], f[2], c.probeMS/1000, fullMS/1000)
	c.printf("channel-send: DO NOT raise HW_INVOKER_WAIT_MS and do not lengthen the probe. No number is both longer than a background child and shorter than the caller timeout you have — every raise so far moved the ceiling without removing it, and the advice it printed (\"a caller timeout of at least 555000 ms\") is written for a caller that can raise one. Measured 2026-09-09: an OpenCode caller may pass at most 600000, so that advice is followable there only just, and would stop being followable at the next raise.\n")
	c.printf("channel-send: THE ORDERED ROUTE, and it is affordable now: re-run this identical command. Each attempt costs the probe, not your budget, so re-checking is a bounded poll — and a pane awaiting a child oscillates, so an attempt that lands while it is briefly idle simply delivers. To look without sending at all: `herdr agent get %s | jq -r \".result.agent.tokens | {turn_state, children_running, turn_ended_at}\"`; the reason above is gone once that pane publishes a turn end with no child running.\n", pane)
	c.printf("channel-send: WHAT THIS DOES NOT BUY, said plainly rather than left to be discovered: waiting for the child does not guarantee your redirect lands before the receiver acts on that child. The child finishing opens a turn of its own. This gate only ever promised a turn BOUNDARY, never priority over work the receiver is about to do — if you need that, the receiver has to be stopped, not messaged.\n")
	c.printf("channel-send: %s\n", escape)
	c.exit(1)
}

// rulingRefusal is _ruling_refusal: it exits 1.
func (c *chanSend) rulingRefusal(target, state, pane string) {
	if state == "" {
		state = "in an unreadable state"
	}
	c.printf("channel-send: --ruling REFUSED before delivery: %s is %s, not at a clean idle/done boundary. A ruling changes the premise of work, so injecting it now would land wherever the current loop happens to be and could arrive only after the work it was meant to correct. NOTHING was sent.\n\nTHREE EXITS, in the order to consider them:\n  1. `hw ruling %s \"<the correction>\"` queues it, and the executor receives it at its NEXT TURN END. Nothing is stopped, nothing is killed, and you do not wait. This is almost always the right one.\n  2. Let it finish and report, then hand the correction to its next task with `hw next`.\n  3. Only if the correction cannot wait for a turn end: stop the work deliberately.\n\nDO NOT run `hw done` to reach exit 3. `hw done` closes the executor and its whole session context goes with it, which is a permanent cost for a temporary correction. Do not substitute --report either: reports do not steer work, rulings do.\n",
		target, state, pane)
	c.exit(1)
}

// csExecState is witness_executor_state's line, with the two tokens the refusal quotes.
type csExecState struct{ verdict, status, state, children string }

func (c *chanSend) executorState(pane string) csExecState {
	info, _ := c.herdr().stdout("agent", "get", pane)
	status := jqRawOr(info, "", "result", "agent", "agent_status")
	if info == "" || status == "" {
		return csExecState{verdict: "absent"}
	}
	state := jqRawOr(info, "", "result", "agent", "tokens", "turn_state")
	children := jqRawOr(info, "", "result", "agent", "tokens", "children_running")
	cwd := jqRawOr(info, "", "result", "agent", "cwd")
	run := jqRawOr(info, "", "result", "agent", "tokens", "hw_run")
	rundir, taskdir := "", ""
	if cwd != "" {
		rundir = t3WitnessRunDir(cwd, run)
	}
	if rundir != "" {
		taskdir = mkTaskDir(rundir)
	}
	var verdict string
	switch {
	case taskdir != "" && t3ReportingLive(taskdir+"/reporting"):
		verdict = "reporting"
	case taskdir != "" && isFile(taskdir+"/done") && !isFile(taskdir+"/reopened"):
		verdict = "reported"
	case state == "ended_awaiting_child" && children != "":
		verdict = "awaiting_child"
	case status == "idle" || status == "done":
		verdict = "idle"
	default:
		verdict = "working"
	}
	return csExecState{verdict, status, state, children}
}

// rulingGate is _ruling_gate: the classifier `hw ruling` asks; its answers are the complement of that verb's.
func (c *chanSend) rulingGate(pane, desc string) {
	if c.degraded {
		info, _ := c.herdr().stdout("agent", "get", pane)
		status := jqRawOr(info, "", "result", "agent", "agent_status")
		if status == "idle" || status == "done" {
			return
		}
		c.rulingRefusal(desc, status, pane)
	}
	es := c.executorState(pane)
	switch es.verdict {
	case "idle": // # MUTATION-ANCHOR: 41-G05
		return
	case "reported":
		if es.status == "idle" || es.status == "done" {
			return
		}
	}
	switch es.verdict {
	case "absent":
		c.rulingRefusal(desc, "", pane)
	case "awaiting_child":
		c.rulingRefusal(desc, fmt.Sprintf("awaiting_child (turn_state=%s, children_running=%s; herdr agent_status=%s flaps here, so it is not read)", es.state, es.children, es.status), pane)
	case "reporting":
		c.rulingRefusal(desc, "reporting (its done-invoker is running and has already checked for rulings, so hw ruling will refuse too — the correction belongs in hw next)", pane)
	default:
		c.rulingRefusal(desc, fmt.Sprintf("%s (herdr agent_status=%s)", es.verdict, es.status), pane)
	}
}

// ── the witness wait (bin/state-witness.sh witness_wait) ────────────────────

// witnessWait: 0 idle,done; 2 budget spent; 5 proved unable to change; 6 a person is waited on; 3/4 herdr-rpc's.
// c.witnessDetail carries what the cross-check read.
func (c *chanSend) witnessWait(pane string, totalMS int64, valid bool) int {
	c.witnessDetail = ""
	if c.degraded {
		c.witnessDetail = "no cross-check was available (state-witness.sh missing)"
		return c.csRPCWait(pane, totalMS)
	}
	slice := int64(20000)
	if n, ok := csMS(c.env("WITNESS_SLICE_MS", "20000")); ok {
		slice = n
	}
	confirm := 2
	if n, ok := csMS(c.env("WITNESS_CONFIRMATIONS", "2")); ok {
		confirm = int(n)
	}
	if valid && totalMS <= slice {
		return c.csRPCWait(pane, totalMS)
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
		switch rc := c.csRPCWait(pane, ms); rc {
		case 0:
			return 0
		case 2:
		default:
			return rc
		}
		if nextEpoch() >= deadline {
			return 2
		}
		var w nextWitness
		c.a.nextWitnessState(pane, &w)
		c.witnessDetail = w.detail
		switch w.verdict {
		case "settled":
			settled++
			human = 0
			if settled >= confirm {
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

// ── the delivery lock ───────────────────────────────────────────────────────

func (c *chanSend) releaseLock() {
	c.mu.Lock()
	held, lock := c.lockHeld, c.lock
	c.lockHeld = false
	c.mu.Unlock()
	if held {
		os.Remove(lock + "/owner.pid")
		os.Remove(lock + "/owner.start")
		os.Remove(lock)
	}
}

// procStart is _cs_proc_start: a pid's start time, whitespace squeezed; "" means "could not ask".
func procStart(pid string) string {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", pid).Output()
	if err != nil {
		return ""
	}
	return strings.Trim(t3Squeeze(bashSub(string(out))), " ")
}

// takeLock claims the per-target delivery lock (mkdir is the atomic claim), breaking a dead holder's.
func (c *chanSend) takeLock(budgetS int64) {
	key := make([]byte, 0, len(c.tgt))
	for i := 0; i < len(c.tgt); i++ { // tr -c 'A-Za-z0-9_.-' '_'
		b := c.tgt[i]
		if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '.' || b == '-' {
			key = append(key, b)
		} else {
			key = append(key, '_')
		}
	}
	tmp := c.getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	lock := tmp + "/hw-deliver-" + string(key) + ".lock"
	ownerFile := lock + "/owner.pid"
	deadline := nextEpoch() + budgetS
	announced := false
	me := strconv.Itoa(os.Getpid())
	for {
		if os.Mkdir(lock, 0o777) == nil {
			os.WriteFile(ownerFile, []byte(me+"\n"), 0o666)
			os.WriteFile(lock+"/owner.start", []byte(procStart(me)+"\n"), 0o666)
			c.mu.Lock()
			c.lock, c.lockHeld = lock, true
			c.mu.Unlock()
			return
		}
		ob, _ := os.ReadFile(ownerFile)
		owner := bashSub(string(ob))
		dead := 0
		startNow, startWas, ownerCmd := "", "", ""
		if owner == "" || strings.Trim(owner, "0123456789") != "" {
			// No owner recorded: a holder mid-claim, or a lock left by a version that wrote nothing.
			if r := extRun(extCall{Name: "find", Args: []string{lock, "-maxdepth", "0", "-mmin", "+1"}, Env: childEnv()}); bashSub(r.Out) != "" {
				dead = 1
			}
		} else {
			pid, _ := strconv.Atoi(owner)
			if pid > 0 && t3PidAlive(pid) {
				sb, _ := os.ReadFile(lock + "/owner.start")
				startWas = bashSub(string(sb))
				if startWas != "" { // # MUTATION-ANCHOR: 101-G03
					startNow = procStart(owner)
					if startNow != "" && startNow != startWas {
						dead = 3
					}
				} else {
					out, _ := exec.Command("ps", "-o", "command=", "-p", owner).Output()
					ownerCmd = bashSub(string(out))
					if ownerCmd != "" && !strings.Contains(ownerCmd, "channel-send") {
						dead = 2
					}
				}
			} else {
				dead = 1
			}
		}
		switch dead {
		case 3:
			c.printf("channel-send: breaking a delivery lock on %s whose holder pid was REUSED (%s, pid %s is alive but started %s, and the sender that took this lock started %s). The sender is gone and nothing is going to release it.\n", c.tgt, lock, owner, startNow, startWas)
		case 2:
			c.printf("channel-send: breaking a delivery lock on %s whose holder is NOT a sender (%s, pid %s is alive but its command is `%s`). A pid outlives the program that wrote it; this lock was left behind and nothing is going to release it.\n", c.tgt, lock, owner, ownerCmd)
		case 1:
			who := owner
			if who == "" {
				who = "none recorded"
			}
			c.printf("channel-send: breaking a delivery lock on %s whose holder is gone (%s, pid %s). This is a crashed or killed sender, not a busy one.\n", c.tgt, lock, who)
		}
		if dead != 0 {
			os.Remove(ownerFile)
			os.Remove(lock + "/owner.start")
			os.Remove(lock)
			continue
		}
		if !announced {
			announced = true
			who := owner
			if who == "" {
				who = "none recorded, still inside its claim window"
			}
			c.printf("channel-send: another sender holds the delivery lock on %s (%s, pid %s, alive). Waiting up to %ds for it; nothing has been sent yet.\n", c.tgt, lock, who, budgetS)
		}
		if nextEpoch() >= deadline {
			who := owner
			if who == "" {
				who = "unknown"
			}
			if !c.isReport && !c.holdProven {
				c.die(fmt.Sprintf("another sender has held the delivery lock on %s for over %ds (%s, pid %s, still alive). Nothing was sent, so nothing was lost — retry, or investigate that process.", c.tgt, budgetS, lock, who))
			}
			c.printf("channel-send: reply/report: another sender has held %s for %ds (pid %s), so this expected reply is being SENT WITHOUT THE LOCK. The lock only makes the idle gate atomic; waiting behind a gated sender would deadlock the receiver against the answer it is holding for.\n", lock, budgetS, who)
			return
		}
		c.sleep(1)
	}
}

// ── the herdr route ─────────────────────────────────────────────────────────

var agentBlockedRE = regexp.MustCompile(`"code"[[:space:]]*:[[:space:]]*"agent_blocked"`)

func (c *chanSend) routeHerdr() {
	if c.deliveryKey != "" {
		c.die("herdr does not expose idempotent caller-supplied delivery ids")
	}
	if c.required != "admitted" {
		c.die("herdr does not expose causal processed receipt proof")
	}
	if c.tgt == "" {
		c.die("Herdr target pane is empty")
	}
	if c.isRuling && !c.holdProven { // # MUTATION-ANCHOR: 41-G01
		c.rulingGate(c.tgt, c.tgt)
	}
	waitS := c.waitBudgetFor(c.tgt)
	waitMS, waitOK := csMS(waitS)
	c.setBudgetText(waitS)
	skipWait := false
	if c.isReport {
		skipWait = true
		c.printf("channel-send: --report: not waiting for %s to be idle. A report is not a redirect — the receiver is waiting for it, and mid-turn delivery on this route is acted on, never dropped (measured 2026-08-25). The idle gate is what deadlocked a brainer against its own executor on 2026-08-27; it is kept for redirects and `hw next`, which is why this is a declaration and not the default.\n", c.tgt)
	}
	if c.isRuling && c.holdProven { // # MUTATION-ANCHOR: 42-G01
		skipWait = true
		c.printf("channel-send: --ruling: not waiting for %s to become idle because its delivered challenge is still outstanding in %s. This reply is the work it is holding for, not a redirect into work in flight.\n", c.tgt, c.replyHold)
	}
	if !c.isReport && c.allowTurnEnded {
		if st, ok := c.turnHasEnded(c.tgt); ok {
			skipWait = true
			c.printf("channel-send: --allow-turn-ended: herdr calls %s `working`, but its own Stop hook published turn_state=%s, so a turn has ENDED there. Sending now instead of waiting.\n", c.tgt, st)
			c.printf("channel-send: what you traded: that token cannot prove no NEW turn started since (only Stop is wired, and a background-subagent notification opens a turn without one). If one did, this lands mid-turn — which is not a loss, but it is not ordered either. Do not use this to hand over a task; that is `hw next`.\n")
		} else {
			c.printf("channel-send: --allow-turn-ended: %s publishes no ended turn_state, so the gate applies as normal.\n", c.tgt)
		}
	}
	uwWarning := ""
	uwFullMS := waitMS
	uwFullS := waitS
	if !skipWait { // # MUTATION-ANCHOR: 111-G06
		if w, ok := c.awaitingChildShorten(c.tgt, waitS); ok { // # MUTATION-ANCHOR: 111-G01
			uwWarning = w
			waitMS, waitOK = c.probeMS, true
			c.setBudget(waitMS)
		}
	}
	var budgetS int64
	if c.isReport || c.holdProven {
		budgetS = 15
		if v := c.getenv("HW_REPORT_LOCK_WAIT_S"); v != "" {
			budgetS, _ = csMS(v)
		}
	} else {
		budgetS = uwFullMS/1000 + 5 // # MUTATION-ANCHOR: 111-G08
	}
	_ = uwFullS
	c.takeLock(budgetS)

	preStatus := c.preStatus(c.tgt)

	waitRC := 0
	if !skipWait {
		waitRC = c.witnessWait(c.tgt, waitMS, waitOK)
	}
	if waitRC != 0 {
		c.releaseLock()
		switch waitRC {
		case 6:
			c.die(fmt.Sprintf("%s is WAITING FOR A PERSON: %s NOTHING was sent, so nothing was lost — and there is no sender-side move here. Do not retry with a larger HW_INVOKER_WAIT_MS, do not reach for --allow-turn-ended, and do not fall back to raw keystrokes: none of them answer a prompt. Answer it on that pane (or have %s answer it), then re-run this identical command.", c.tgt, c.witnessDetail, c.operator()))
		case 5:
			c.die(fmt.Sprintf("%s is NOT going to become idle: %s NOTHING was sent, so nothing was lost — but do not retry with a larger HW_INVOKER_WAIT_MS, because no budget reaches idle from that state. This is the failure that used to look like an ordinary timeout while two `herdr agent wait` budgets burned on it (measured 2026-08-27, w41:p6H).%s", c.tgt, c.witnessDetail, c.lastResort(c.tgt)))
		}
		if waitRC != 2 {
			c.die(fmt.Sprintf("could not wait on %s (herdr-rpc exit %d — see its message above). Nothing was sent.", c.tgt, waitRC))
		}
		if c.witnessDetail != "" {
			c.printf("channel-send: cross-check: %s\n", c.witnessDetail)
		}
		if uwWarning != "" {
			c.awaitingChildRefusal(c.tgt, c.tgt, uwWarning, uwFullMS, "IF IT MUST LAND NOW, --allow-turn-ended is the declared trade, and it stays a declaration rather than becoming the default here for a reason: it sends immediately and gives up ordering. The price is sharper in this state than in the one it was written for — a child IS running, and its completion opens a new turn on its own, so a redirect sent now may be read before, during or after that turn. Take it only for a message whose value does not depend on where it lands, and never to hand over a task: that is `hw next`.")
		}
		c.die(fmt.Sprintf("%s was still working after %ds, so NOTHING was sent. A prompt delivered mid-turn IS acted on — it is not dropped — but not necessarily where you meant it: it lands wherever the receiver's loop is, which may be after the work you were trying to change. This gate is what keeps that from happening silently. Nothing was lost; retry, or give it a longer budget with a matching Bash timeout: HW_INVOKER_WAIT_MS=540000 channel-send …", c.tgt, waitMS/1000))
	}
	c.setSent(csInflight) // # MUTATION-ANCHOR: 42-G04
	prOut, promptRC := c.herdr().capture("agent", "prompt", c.tgt, c.message)
	promptOut := bashSub(prOut)
	if promptRC != 0 {
		c.setSent(csNotSent)
		fallback := ""
		if agentBlockedRE.MatchString(promptOut) {
			switch {
			case c.holdProven: // # MUTATION-ANCHOR: 42-G05
				if c.isRuling {
					fallback = "ruling"
					c.printf("channel-send: herdr agent prompt rejected the proved held ruling with agent_blocked (exit %d): %s\n", promptRC, promptOut)
				} else {
					fallback = "answer" // # MUTATION-ANCHOR: 42-G12
					c.printf("channel-send: herdr agent prompt rejected the proved held ask answer with agent_blocked (exit %d): %s\n", promptRC, promptOut)
				}
				c.printf("channel-send: falling back ONLY for this receiver-owned reply hold to raw pane input; generic reports, rulings and redirects cannot use this proof.\n")
			case c.isReport && c.reportProven: // # MUTATION-ANCHOR: 42-G08
				var w nextWitness
				c.a.nextWitnessState(c.tgt, &w) // # MUTATION-ANCHOR: 42-G09
				switch w.verdict {
				case "awaiting-human":
					c.die(fmt.Sprintf("herdr agent prompt rejected this proved run report because %s is blocked, and state-witness confirms it is WAITING FOR A PERSON: %s NOTHING was sent; raw pane input would overwrite a real human prompt. Answer that prompt, then retry the identical report.", c.tgt, w.detail))
				case "live", "settled":
					fallback = "report"
					c.printf("channel-send: herdr agent prompt rejected a report for a proved dispatched, still-unreported run with agent_blocked (exit %d): %s\n", promptRC, promptOut)
					c.printf("channel-send: state-witness classified %s as %s rather than awaiting-human: %s\n", c.tgt, w.verdict, w.detail)
					c.printf("channel-send: falling back ONLY for this dispatch-bound unreported run to raw pane input; an unproved --report cannot use this path.\n")
				default:
					c.die(fmt.Sprintf("herdr agent prompt rejected this proved run report because %s is blocked, but state-witness could not prove whether raw input is safe: %s NOTHING was sent; retry only after the receiver state is readable.", c.tgt, w.detail))
				}
			case c.isRuling || c.isReport:
				// A ruling without its proven hold, or a report without its proven dispatch record, gets no
				// generic fallback, ever.
			default:
				// A PLAIN MESSAGE: it asserts nothing about the receiver waiting for it, so it is gated on the
				// receiver's own endpoint instead of a disk-backed proof.
				ep := c.a.nextWitnessEndpoint(c.tgt)
				qn, pn, havePrompts := "", "", false
				if ep != "" {
					qn, pn, havePrompts = c.a.nextWitnessPrompts(ep)
				}
				if ep != "" && havePrompts {
					if qn == "0" && pn == "0" {
						fallback = "message"
						c.printf("channel-send: herdr agent prompt rejected delivery to %s with agent_blocked (exit %d: %s), but its own endpoint %s confirms nothing is pending (/question 0, /permission 0) — no human is actually being waited on.\n", c.tgt, promptRC, promptOut, ep)
						c.printf("channel-send: falling back to raw pane input for this message. This BYPASSES the gate entirely: no receipt, no ordering guarantee — read the pane afterward instead of assuming it landed.\n")
					} else {
						c.printf("channel-send: herdr agent prompt rejected delivery to %s with agent_blocked (exit %d), and its own endpoint %s CONFIRMS a real prompt is outstanding (/question %s, /permission %s). NOTHING was sent; raw pane input would overwrite a real human prompt. Answer that prompt, then retry the identical command.\n", c.tgt, promptRC, ep, qn, pn)
					}
				} else {
					c.printf("channel-send: herdr agent prompt rejected delivery to %s with agent_blocked (exit %d), and its endpoint could not be read to confirm whether a prompt is pending (not opencode, no --port, or an unreachable/unparseable response). An authority that cannot be asked is not an authority that said no, so this refuses rather than guessing: NOTHING was sent; retry only after the receiver state is readable.\n", c.tgt, promptRC)
				}
			}
		}
		if fallback != "" {
			stOut, stRC := c.herdr().capture("pane", "send-text", c.tgt, c.message)
			if stRC != 0 {
				c.die(fmt.Sprintf("agent_blocked was diagnosed, but the proved-%s fallback failed before input reached %s (pane send-text exit %d: %s). NOTHING was sent.", fallback, c.tgt, stRC, bashSub(stOut)))
			}
			c.setSent(csInflight)
			skOut, skRC := c.herdr().capture("pane", "send-keys", c.tgt, "enter")
			if skRC != 0 {
				c.printf("channel-send: pane send-text reached %s after agent_blocked, but pane send-keys enter failed (exit %d: %s). The %s is in the input buffer but submission is UNCONFIRMED; do not blind-retry.\n", c.tgt, skRC, bashSub(skOut), fallback)
				c.exit(5)
			}
			if fallback == "ruling" || fallback == "answer" {
				c.printf("channel-send: held %s was submitted to %s through the blocked-pane fallback after agent_blocked, but receiver ingestion is UNCONFIRMED. The reply hold remains; do NOT blind-retry, because the receiver may yet read this input.\n", fallback, c.tgt)
				c.exit(5)
			}
			c.setSent(csSent)
			if fallback == "report" {
				c.printf("channel-send: proved run report submitted to %s through the blocked-pane fallback after agent_blocked.\n", c.tgt)
			} else {
				c.printf("channel-send: message submitted to %s through the blocked-pane fallback after agent_blocked (its own endpoint confirmed nothing pending).\n", c.tgt)
			}
		} else {
			if promptOut != "" {
				c.printf("channel-send: herdr agent prompt rejected delivery to %s (exit %d): %s\n", c.tgt, promptRC, promptOut) // # MUTATION-ANCHOR: 42-G07
			}
			c.die(fmt.Sprintf("herdr agent prompt failed before delivery to %s. NOTHING was sent.", c.tgt))
		}
	} else {
		c.setSent(csSent)
	}
	c.releaseLock()
	switch preStatus {
	case "working":
		c.printf("channel-send: %s was MID-TURN when this was sent, so WHERE it landed was not your choice.\n", c.tgt)
		c.printf("channel-send: it was acted on — mid-turn delivery is not a loss — but either as a new turn interleaved with the work in flight, or queued until that turn ended. This cannot tell which. So do not assume it steered anything: check the target before concluding your redirect took effect, and to re-task properly use `hw next`.\n")
	case "idle", "done":
	default:
		c.printf("channel-send: could not read %s status before sending (got %s), so whether this steered or queued is unknown.\n", c.tgt, preStatus)
	}
}

// setBudgetText is _CS_BUDGET_MS = <the text>; the signal's message reads it as a number.
func (c *chanSend) setBudgetText(s string) {
	c.mu.Lock()
	c.budgetMS = s
	c.mu.Unlock()
}

// preStatus is `herdr agent get <t> | jq -r '.result.agent.agent_status // "unknown"' || echo unknown`: under
// pipefail a failed herdr or a jq that could not read it adds "unknown" to whatever jq printed — and an EMPTY answer
// that jq read fine prints nothing at all, which the caller then reports as `(got )`.
func (c *chanSend) preStatus(t string) string {
	info, ok := c.herdr().stdout("agent", "get", t)
	vals, perr := parseStream([]byte(info))
	var outs []string
	for _, v := range vals {
		cur, okp := v, true
		for _, p := range []string{"result", "agent", "agent_status"} {
			if cur, okp = cur.get(p); !okp {
				break
			}
		}
		if okp && cur.truthy() {
			outs = append(outs, cur.raw())
		} else if okp {
			outs = append(outs, "unknown")
		}
	}
	if !ok || perr != nil {
		outs = append(outs, "unknown")
	}
	return bashSub(strings.Join(outs, "\n"))
}
