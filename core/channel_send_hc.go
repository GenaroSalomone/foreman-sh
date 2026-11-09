package main

// channel-send's hc route: delivery to an agent in an herdr-core terminal, with the herdr route's contract (exits 0/1/4/5).
// Measured facts it rests on (design 0b P11): `hc send --if idle` refuses a finished Claude (`done`, not `idle`), so the gate
// is foreman's; `--force` may answer a dialog, so it is never used. Delivery is proven from the receiver, never from the send.

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func (c *chanSend) agentGet(t string) (string, bool) {
	if c.vendor == "hc" {
		return hcAgentGetJSON(t)
	}
	return c.herdr().stdout("agent", "get", t)
}

type hcState struct{ status, turn, turns, endedAt string }

func (c *chanSend) hcState(t string) (hcState, bool) {
	info, ok := c.agentGet(t)
	if !ok {
		return hcState{}, false
	}
	return hcState{
		status:  jqRawOr(info, "", "result", "agent", "agent_status"),
		turn:    jqRawOr(info, "", "result", "agent", "turn"),
		turns:   jqRawOr(info, "", "result", "agent", "tokens", "hw_turns"),
		endedAt: jqRawOr(info, "", "result", "agent", "tokens", "hw_turn_ended_at"),
	}, true
}

// hcMoved: the receiver did something after the state `before` that a delivered prompt explains.
// shortcut: a turn that was already ending when the send went in also moves turn/labels, so a mid-turn send can be
// proved by the old turn's end; the text is still typed and acted on, only WHERE it landed is unproven (the caller is told).
func hcMoved(before, now hcState) bool {
	if now.status == "working" && before.status != "working" {
		return true
	}
	return now.turn != before.turn || now.turns != before.turns || now.endedAt != before.endedAt
}

// hcWaitIdle returns 0 at a turn boundary, 2 budget spent, 3 terminal gone, 6 blocked on a person.
func (c *chanSend) hcWaitIdle(t string, totalMS int64) int {
	return hcWaitBoundary(
		func() (string, bool) { st, ok := c.hcState(t); return st.status, ok },
		func(secs int64) int {
			return c.csChildWait(exec.Command("hc", "wait", t, "--timeout", strconv.FormatInt(secs, 10)))
		},
		func() { c.sleep(1) }, totalMS, "idle", "done", "error")
}

func (c *chanSend) routeHC() {
	if c.deliveryKey != "" {
		c.die("hc does not expose idempotent caller-supplied delivery ids")
	}
	if c.required != "admitted" {
		c.die("hc does not expose causal processed receipt proof")
	}
	if c.tgt == "" {
		c.die("hc target terminal is empty")
	}
	if c.isRuling && !c.holdProven {
		c.rulingGate(c.tgt, c.tgt)
	}
	waitS := c.waitBudgetFor(c.tgt)
	waitMS, _ := csMS(waitS)
	c.setBudgetText(waitS)
	skipWait := false
	if c.isReport {
		skipWait = true
		c.printf("channel-send: --report: not waiting for %s to be idle. A report is not a redirect — the receiver is waiting for it, and the idle gate is what deadlocks a brainer against its own executor.\n", c.tgt)
	}
	if c.isRuling && c.holdProven {
		skipWait = true
		c.printf("channel-send: --ruling: not waiting for %s to become idle because its delivered challenge is still outstanding in %s. This reply is the work it is holding for, not a redirect into work in flight.\n", c.tgt, c.replyHold)
	}
	if !c.isReport && c.allowTurnEnded {
		if st, ok := c.turnHasEnded(c.tgt); ok {
			skipWait = true
			c.printf("channel-send: --allow-turn-ended: %s is `working`, but its own Stop hook published turn_state=%s, so a turn has ENDED there. Sending now instead of waiting.\n", c.tgt, st)
		} else {
			c.printf("channel-send: --allow-turn-ended: %s publishes no ended turn_state, so the gate applies as normal.\n", c.tgt)
		}
	}
	var lockS int64
	if c.isReport || c.holdProven {
		lockS = 15
		if v := c.getenv("HW_REPORT_LOCK_WAIT_S"); v != "" {
			lockS, _ = csMS(v)
		}
	} else {
		lockS = waitMS/1000 + 5
	}
	c.takeLock(lockS)

	if !skipWait {
		switch rc := c.hcWaitIdle(c.tgt, waitMS); rc {
		case 0:
		case 6:
			c.releaseLock()
			c.die(fmt.Sprintf("%s is BLOCKED on a person (a permission, question or auth prompt). NOTHING was sent, so nothing was lost — and `hc send --force` would type into that prompt, so this route never uses it. Answer it on that terminal (or have %s answer it), then re-run this identical command.", c.tgt, c.operator()))
		case 3:
			c.releaseLock()
			c.die(fmt.Sprintf("hc knows no live terminal %s (or `hc get` could not answer). Nothing was sent.", c.tgt))
		default:
			c.releaseLock()
			c.die(fmt.Sprintf("%s was still working after %ds, so NOTHING was sent. A prompt delivered mid-turn IS acted on — it is not dropped — but not necessarily where you meant it: it lands wherever the receiver's loop is, which may be after the work you were trying to change. This gate is what keeps that from happening silently. Nothing was lost; retry, or give it a longer budget with a matching Bash timeout: HW_INVOKER_WAIT_MS=540000 channel-send …", c.tgt, waitMS/1000))
		}
	}
	before, ok := c.hcState(c.tgt)
	if !ok {
		c.releaseLock()
		c.die(fmt.Sprintf("hc knows no live terminal %s (or `hc get` could not answer). Nothing was sent.", c.tgt))
	}
	c.setSent(csInflight)
	r := extRun(extCall{Name: "hc", Args: []string{"send", "--", c.tgt, c.message}, Env: childEnv(), Merge: true})
	if r.RC != 0 {
		c.setSent(csNotSent)
		if out := bashSub(r.Out); out != "" {
			c.printf("channel-send: hc send rejected delivery to %s (exit %d): %s\n", c.tgt, r.RC, out)
		}
		c.die(fmt.Sprintf("hc send failed before delivery to %s. NOTHING was sent.", c.tgt))
	}
	c.releaseLock()
	proofMS := int64(15000)
	if n, ok := csMS(c.env("HW_HC_PROOF_MS", "15000")); ok {
		proofMS = n
	}
	deadline := nextEpoch()*1000 + proofMS
	for {
		now, got := c.hcState(c.tgt)
		if got && hcMoved(before, now) {
			c.setSent(csSent)
			if before.status == "working" {
				c.printf("channel-send: %s was MID-TURN when this was sent, so WHERE it landed was not your choice: either a new turn interleaved with the work in flight, or queued until that turn ended. To re-task properly use `hw next`.\n", c.tgt)
			}
			if now.status == "error" {
				c.printf("channel-send: %s took the message (its turn counter moved) but its turn ended in error. Do NOT resend.\n", c.tgt)
				c.exit(4)
			}
			return
		}
		if nextEpoch()*1000 >= deadline {
			break
		}
		c.sleep(0.5)
	}
	c.printf("channel-send: hc send to %s went in (exit 0), but none of its own signals moved within %dms (status was %q, turn %s, hw_turns %q): ADMITTED, receiver ingestion UNCONFIRMED. It may yet have the message — read it with `hc read %s` before sending anything again; do NOT blind-retry.\n",
		c.tgt, proofMS, strings.TrimSpace(before.status), before.turn, before.turns, c.tgt)
	c.exit(5)
}
