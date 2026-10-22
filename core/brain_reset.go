package main

// The reset path of bin/brain: the wait of the detached arm, the self-reset that arms it, the liveness refusals, the
// replacement of the agent in place (/exit and the proof it is gone), the relocation of a brainer that stands in the
// wrong lane, and the gate that keeps brain from typing into the pane it runs in.

import (
	"strings"
	"time"
)

// ownPaneGate is brain_own_pane_gate: the pane brain is running in is not a pane it can type into.
func (b *brainRun) ownPaneGate(pane string) {
	// THE DETACHED `--reset --await-idle` ARM carries the pane's HERDR_PANE_ID but is no foreground holder.
	if b.awaitPane != "" { // # MUTATION-ANCHOR: go-893-M02
		return
	}
	if hp := b.getenv("HERDR_PANE_ID"); hp == "" || hp != pane { // # MUTATION-ANCHOR: go-892-M01
		return
	}
	why, cmd := "", "brain "+b.proj
	if b.resume != "" {
		cmd += " --resume " + b.resume
	}
	switch {
	case b.kind != "claude":
		why = "only a claude brainer is started in place"
	case !b.isTTY():
		why = "this shell has no terminal on stdin/stdout/stderr to hand to claude"
	case strings.HasPrefix(b.verdict, "mismatch"):
		cwd := b.paneCwd
		if cwd == "" {
			cwd = "<unknown>"
		}
		why = "its shell stands in " + cwd + ", not " + b.dir + ", and starting in place cannot move it"
	}
	if why != "" {
		b.errf("brain: %s is the pane brain is running in, and brain does not type into its own pane (%s).\n", pane, why)
		b.errf("brain: Nothing was sent and nothing was restarted. Run this from ANOTHER pane: %s\n", cmd)
		b.exit(1)
	}
	b.inproc = true
	b.errf("brain: %s is this very pane — starting the %s brainer in-process (exec), not by typing into it.\n", pane, b.kind)
}

// awaitIdle is the detached arm, waiting out the turn that armed it, then re-reading every question from scratch.
func (b *brainRun) awaitIdle(tab, pane, panesJSON string) (string, tabPane, int) {
	proj := b.proj
	hp := b.getenv("HERDR_PANE_ID")
	if hp == "" || hp != b.awaitPane { // # MUTATION-ANCHOR: go-153-M06
		who := hp
		if who == "" {
			who = "nobody"
		}
		b.errf("brain: --await-idle is the second half of `--reset --self`, and it waits out a turn\n")
		b.errf("brain: instead of refusing it — so it is only ever legal from INSIDE the pane being\n")
		b.errf("brain: reset. You are %s and this names %s. Resetting somebody else is `brain %s\n", who, b.awaitPane, proj)
		b.errf("brain: --reset`, which refuses a WORKING brainer instead of outliving it.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return "", tabPane{}, 1
	}
	if b.awaitPane != pane {
		b.errf("brain: this reset was armed for %s, but the %s brainer is %s now — it moved between\n", b.awaitPane, proj, pane)
		b.errf("brain: the request and this attempt. brain will not reset a pane nobody asked about.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return "", tabPane{}, 1
	}
	awaitMax := atoiDefault(brOrStr(b.getenv("BRAIN_AWAIT_IDLE_MAX"), "900"), 0)
	awaitPoll := atoiDefault(brOrStr(b.getenv("BRAIN_AWAIT_POLL"), "3"), 0)
	waited := 0
	awaitLast := "working"
	sawWorking := false
	for {
		reply := b.agentGet(pane)
		// A PANE THAT IS ALREADY EMPTY IS NOT A PANE STILL WORKING.
		if agentNotFound(reply) {
			break
		}
		st := bjRawOr(reply, "result", "agent", "agent_status")
		// ONLY A STATUS THAT WAS READ ENDS THE WAIT.
		// # MUTATION-ANCHOR: go-153-M07
		if st == "" {
			awaitLast = "unreadable"
		} else {
			awaitLast = st
			if st == "working" {
				sawWorking = true
			}
			if st != "working" { // # MUTATION-ANCHOR: go-153-M02
				break
			}
		}
		// # MUTATION-ANCHOR-END: go-153-M07
		if waited >= awaitMax {
			if awaitLast == "unreadable" {
				b.errf("brain: %s could not be read by `herdr agent get` once in the %ds since this reset was\n", pane, waited)
				b.errf("brain: armed, so whether that turn ever ended is UNKNOWN. An unreachable reader is not\n")
				b.errf("brain: evidence that a restart is safe. Nothing was sent and nothing was restarted.\n")
			} else {
				b.errf("brain: %s has been WORKING for %ds since this reset was armed. brain is not going to\n", pane, waited)
				b.errf("brain: replace a conversation that never stopped talking. Nothing was sent and nothing\n")
				b.errf("brain: was restarted; arm it again when that turn is over.\n")
			}
			return "", tabPane{}, 1
		}
		b.sleep(time.Duration(awaitPoll) * time.Second)
		waited += awaitPoll
	}
	if sawWorking {
		b.errf("brain: the turn that armed this reset ended after %ds — re-reading everything before\n", waited)
	} else {
		b.errf("brain: no turn was in flight when this reset was reached (%ds) — re-reading everything\n", waited)
	}
	b.errf("brain: before touching the pane, because none of it was measured since.\n")
	panesJSON = b.mustJSON("the panes of "+tab, []string{"result", "panes"}, "pane", "list")
	tp, _ := brainTabPane(panesJSON, tab)
	if tp.pane != b.awaitPane {
		nothing := tp.pane
		if nothing == "" {
			nothing = "nothing"
		}
		b.errf("brain: after the wait, the %s brainer tab no longer holds %s (it holds %s). Nothing was\n", proj, b.awaitPane, nothing)
		b.errf("brain: sent and nothing was restarted.\n")
		return "", tabPane{}, 1
	}
	b.verdict = brainerVerdict(panesJSON, tp.pane, b.dir)
	return panesJSON, tp, 0
}

func brOrStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// armSelfReset is the --self branch: measure while alive, reset from outside, later.
func (b *brainRun) armSelfReset(pane, panesJSON string) int {
	proj := b.proj
	hp := b.getenv("HERDR_PANE_ID")
	if hp == "" {
		b.errf("brain: --self means \"reset the pane I am in\", and HERDR_PANE_ID is not set here, so\n")
		b.errf("brain: brain cannot tell which pane that is. It will not guess: the guess is a live\n")
		b.errf("brain: conversation. Nothing was sent and nothing was restarted.\n")
		return 1
	}
	if hp != pane { // # MUTATION-ANCHOR: go-153-M01
		b.errf("brain: --self means \"reset the pane I am in\", but you are %s and the %s brainer is\n", hp, proj)
		b.errf("brain: %s. Resetting somebody else is `brain %s --reset`, and that path refuses a\n", pane, proj)
		b.errf("brain: WORKING brainer — --self does not widen it, it only defers your own.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return 1
	}
	// THE SAME GATE THE OTHER PATH ASKS, ASKED EARLY.
	if !b.blockerGate(proj, pane, "--reset --self", panesJSON) { // # MUTATION-ANCHOR: go-153-M03
		return 1
	}
	blockers := b.resetBlockers(proj, pane, panesJSON)
	b.errf("brain: measured before arming: %d executor(s) of %s have not reported and their return\n", len(blockers), proj)
	b.errf("brain: address is this pane (%s). The pane id survives, so their reports still land —\n", pane)
	b.errf("brain: in a conversation that will know nothing about them.\n")
	for _, r := range blockers {
		b.errf("brain:   %s  (run %s)\n", r.task, r.run)
	}
	tmp := brOrStr(b.getenv("TMPDIR"), "/tmp")
	selfLog := tmp + "/brain-self-reset-" + sanitizePane(pane) + ".log"
	selfArgs := []string{"brain", proj}
	if b.kind == "opencode" {
		selfArgs = append(selfArgs, "--opencode")
	}
	if b.kind == "claude" {
		selfArgs = append(selfArgs, "--claude")
	}
	selfArgs = append(selfArgs, "--reset", "--await-idle", pane)
	if b.force {
		selfArgs = append(selfArgs, "--force")
	}
	if b.noCockpit {
		selfArgs = append(selfArgs, "--no-cockpit")
	}
	pid, err := b.detach(b.binDir+"/brain", selfArgs[1:], b.env, selfLog)
	if err != nil {
		b.errf("brain: the detached arm could not be started, so nothing is armed and nothing was\n")
		b.errf("brain: reset. This conversation is intact.\n")
		return 1
	}
	if pid == 0 {
		b.errf("brain: the detached arm reported no pid, so brain cannot say it is running. Nothing\n")
		b.errf("brain: was sent and nothing was restarted.\n")
		return 1
	}
	b.outf("armed: %s / %s (pane %s, %s) resets itself when this turn ends — same pane, new conversation\n", b.space, b.tabLabel, pane, b.kind)
	b.outf("  it runs detached as pid %d and writes what it did to %s\n", pid, selfLog)
	if isWindows() {
		b.outf("  nothing has been reset yet; call it off with: taskkill /PID %d /F  (a Windows pid)\n", pid)
	} else {
		b.outf("  nothing has been reset yet; call it off with: kill %d\n", pid)
	}
	return 0
}

// sanitizePane is `tr -c 'A-Za-z0-9_.-' '_'`.
func sanitizePane(s string) string {
	out := []byte(s)
	for i, c := range out {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-') {
			out[i] = '_'
		}
	}
	return string(out)
}

// livenessGate is the case on the pane's status: nothing is sent before a positive reading that nothing is running.
func (b *brainRun) livenessGate(pane, paneAgent, paneStatus string) int {
	agent := paneAgent
	if agent == "" {
		agent = "agent"
	}
	// # MUTATION-ANCHOR: go-137-M01
	switch paneStatus {
	case "working":
		b.errf("brain: %s is WORKING (%s) — a turn is in flight, and a reset would discard it.\n", pane, agent)
		b.errf("brain: Let it finish, then reset.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return 1
	case "blocked":
		b.errf("brain: %s is BLOCKED (%s) — it is waiting for a person, and a reset would discard\n", pane, agent)
		b.errf("brain: that prompt and the turn behind it. Answer it instead.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return 1
	case "idle", "done", "": // # MUTATION-ANCHOR: go-153-M04
	case "unknown": // # MUTATION-ANCHOR: go-143-M01
		probe := b.agentGet(pane)
		if agentNotFound(probe) {
			b.errf("brain: %s reports agent_status=unknown and `herdr agent get` says agent_not_found —\n", pane)
			b.errf("brain: that is a bare shell, which is what --reset is for. Continuing.\n")
		} else {
			b.errf("brain: %s reports agent_status=unknown and `herdr agent get` did NOT answer\n", pane)
			b.errf("brain: agent_not_found, so something may still be running there. brain --reset\n")
			b.errf("brain: replaces a live conversation and needs positive evidence that nothing is.\n")
			b.errf("brain: Nothing was sent and nothing was restarted.\n")
			return 1
		}
	default:
		b.errf("brain: %s reports agent_status=%s, which brain --reset does not know how to read as\n", pane, paneStatus)
		b.errf("brain: safe. It replaces a live conversation, so it needs positive evidence that\n")
		b.errf("brain: nothing is running, not the absence of evidence that something is.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return 1
	}
	// # MUTATION-ANCHOR-END: go-137-M01
	return 0
}

// replaceAgent sends /exit and proves the agent is gone before anything is started over it. A non-zero status ends
// the command with it.
func (b *brainRun) replaceAgent(pane, paneAgent string) int {
	b.errf("brain: replacing the %s in %s — the space, tab, pane and cwd all stay.\n", paneAgent, pane)
	if !b.herdrOK("pane", "send-text", pane, "/exit") {
		b.errf("brain: could not send /exit to %s; nothing was restarted.\n", pane)
		return 1
	}
	b.sleep(1 * time.Second)
	// FROM HERE THE PANE IS TOUCHED. A resume that dies after this point exits 75, not 1.
	if b.resume != "" {
		b.trap75 = true
	}
	if !b.herdrOK("pane", "send-keys", pane, "enter") {
		b.errf("brain: /exit was typed into %s but could not be submitted; look at that pane before starting anything else.\n", pane)
		return 1
	}
	gone, bgMoved := false, false
	var bgPids []string
	for i := 0; i < 15; i++ {
		b.sleep(1 * time.Second)
		reply := b.agentGet(pane)
		if agentNotFound(reply) {
			gone = true
			break
		}
		// CLAUDE ASKS BEFORE IT EXITS WHEN IT HAS BACKGROUND WORK.
		// # MUTATION-ANCHOR: go-891-M01
		if b.exitConfirmation(pane, reply) {
			if !bgMoved {
				bgPids = b.claudeDescendants(pane)
				if b.moveToBackground(pane) {
					bgMoved = true
					b.errf("brain: %s asked to confirm /exit — it has background work; chose «Move to background and exit».\n", pane)
					continue
				}
			}
			b.herdrOK("pane", "send-keys", pane, "escape")
			b.sleep(2 * time.Second)
			b.trap75 = false // nothing was stopped: a refusal (rc 1, a skip for `brain relaunch`), not the 75 of a pane left empty
			b.errf("brain: %s asked to confirm /exit — it has background work and «Move to background and exit» could not be chosen.\n", pane)
			st := bjRawOr(b.agentGet(pane), "result", "agent", "agent_status")
			if st == "" {
				st = "unreadable"
			}
			b.errf("brain: Esc was sent (stay): the agent is still there, now %s. Nothing was restarted.\n", st)
			b.errf("brain: %s has background tasks — relaunch would kill them; stop or finish them in that pane, then relaunch.\n", pane)
			return 1
		}
		// # MUTATION-ANCHOR-END: go-891-M01
	}
	// THE TASKS MOVED TO THE BACKGROUND ARE LOOKED FOR, not assumed.
	if gone && bgMoved {
		total, alive := 0, 0
		for _, p := range bgPids {
			total++
			if pidAlive(atoiDefault(p, -1)) {
				alive++
			}
		}
		if total == 0 {
			b.errf("brain: background-kept unverified (claude's process tree could not be read)\n")
		} else {
			b.errf("brain: background-kept %d/%d\n", alive, total)
		}
		if total != 0 && alive == 0 {
			b.errf("brain: WARNING — none of the %d background processes survived the exit (they may have finished meanwhile).\n", total)
		}
	}
	if !gone {
		b.errf("brain: 15s after /exit, %s has NOT been proved agent-free — herdr either still reports an agent there or could not be asked. brain will NOT start a second one over it. Look at that pane.\n", pane)
		return 1
	}
	return 0
}

// relocate is the relocation of a brainer that stands in the wrong lane: sent only after the agent is gone, so it
// lands on the pane's shell. It returns RESET_CWD_UNFIXED.
func (b *brainRun) relocate(pane, tab string) bool {
	if !strings.HasPrefix(b.verdict, "mismatch") {
		return false
	}
	cwd := b.paneCwd
	if cwd == "" {
		cwd = "<unknown>"
	}
	b.errf("brain: this brainer was standing in %s, not %s — moving it.\n", cwd, b.dir)
	if b.herdrOK("pane", "send-text", pane, "cd "+b.dir) {
		b.sleep(1 * time.Second)
		if !b.herdrOK("pane", "send-keys", pane, "enter") {
			b.errf("brain: the cd was typed into %s but could not be submitted.\n", pane)
		}
	} else {
		b.errf("brain: the cd could not be sent to %s.\n", pane)
	}
	b.sleep(2 * time.Second)
	// NOT AN UNGUARDED ASSIGNMENT: an unverifiable move is treated as a move that did not happen.
	moved, _ := b.herdrJSON("the panes of "+tab, []string{"result", "panes"}, "pane", "list")
	if moved == "" {
		b.errf("brain: the pane list could not be re-read, so the move could not be verified.\n")
	}
	verdict := brainerVerdict(moved, pane, b.dir)
	// A RELOCATION THAT DID NOT HAPPEN IS NOT A WARNING, IT IS A STATUS.
	if verdict != "ok" {
		b.errf("brain: WARNING — %s still reports the wrong cwd (%s); the brainer below is in the wrong lane.\n", pane, verdict)
		return true
	}
	return false
}
