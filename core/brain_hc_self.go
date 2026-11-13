package main

// `brain <lane> --reset --self` ON HERDR-CORE: a brainer in the herdr app resetting itself (hc-parity-before-v1).
//
// The problem is the TUI's (bin/brain's header): the process asking for the reset is the one the reset kills, so the reset
// is ARMED now and PERFORMED later, by a process in a session of its own that waits for this turn to end. What differs is
// what "reset" is: a herdr pane is typed into and keeps its id; an app terminal is ended (`hc kill`, `hc rm`) and a new one
// is started (brain_hc_reset.go), so the brainer's address changes and `repointRuns` hands the open runs the new one.
//
// MEASURED 2026-11-12 on an isolated app: a process that called setsid survives `hc kill` of the terminal it was started
// from (the kill is a SIGHUP to the terminal's program: `hc get` says signal 1), so the arm outlives the terminal it ends.
// Before this file the flag was refused: "--self has no herdr-core mapping yet: this step of foreman on the herdr app…".

import "time"

// hcArmSelfReset is armSelfReset on the app: measure while alive, reset from outside, later. addr is the lane's brainer
// terminal ("" when the lane has none).
func (b *brainRun) hcArmSelfReset(addr string) int {
	proj, own := b.proj, b.getenv("HC_TERMINAL_ID")
	if addr == "" || own != addr { // MUTATION-ANCHOR: hc-self-identity
		at := addr
		if at == "" {
			at = "not open"
		}
		b.errf("brain: --self means \"reset the terminal I am in\", but you are %s and the %s brainer is\n", own, proj)
		b.errf("brain: %s. Resetting somebody else is `brain %s --reset`, and that path refuses a\n", at, proj)
		b.errf("brain: WORKING brainer — --self does not widen it, it only defers your own.\n")
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return 1
	}
	// THE SAME GATE THE OTHER PATH ASKS, ASKED EARLY: executors that have not reported and are addressed to this terminal.
	if !b.hcBlockerGate(addr) { // MUTATION-ANCHOR: hc-self-blockers
		return 1
	}
	tmp := brOrStr(b.getenv("TMPDIR"), "/tmp")
	selfLog := tmp + "/brain-self-reset-" + sanitizePane(addr) + ".log"
	args := []string{proj}
	switch b.kind {
	case "opencode":
		args = append(args, "--opencode")
	case "claude":
		args = append(args, "--claude")
	}
	args = append(args, "--reset", "--await-idle", addr)
	if b.force {
		args = append(args, "--force")
	}
	if b.noCockpit {
		args = append(args, "--no-cockpit")
	}
	pid, err := b.detach(b.binDir+"/brain", args, b.env, selfLog)
	if err != nil || pid == 0 {
		b.errf("brain: the detached arm could not be started, so nothing is armed and nothing was\n")
		b.errf("brain: reset. This conversation is intact.\n")
		return 1
	}
	b.outf("armed: %s / %s (terminal %s, %s) resets itself when this turn ends — a new terminal, a new conversation\n", b.space, b.tabLabel, addr, b.kind)
	b.outf("  it runs detached as pid %d and writes what it did to %s\n", pid, selfLog)
	b.outf("  nothing has been reset yet; call it off with: kill %d\n", pid)
	return 0
}

// hcAwaitIdle is the detached arm, waiting out the turn that armed it: 0 once the brainer's terminal has left `working`
// (or is gone), 1 with nothing sent when it is not the arm of this terminal or the turn never ended.
func (b *brainRun) hcAwaitIdle(addr string) int {
	proj, own := b.proj, b.getenv("HC_TERMINAL_ID")
	if own == "" || own != b.awaitPane { // typed by anybody else, --await-idle would wait out a third party's turn and replace it
		b.errf("brain: --await-idle is the detached half of `brain %s --reset --self`: it resets the terminal that armed it, and\n", proj)
		b.errf("brain: you are %s and this names %s. Resetting somebody else is `brain %s --reset`.\n", brOrStr(own, "no app terminal"), b.awaitPane, proj)
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return 1
	}
	if addr != b.awaitPane {
		b.errf("brain: this reset was armed for %s, but the %s brainer is %s now — it moved between\n", b.awaitPane, proj, brOrStr(addr, "not open"))
		b.errf("brain: the arming and the reset. Nothing was sent and nothing was restarted.\n")
		return 1
	}
	awaitMax := atoiDefault(brOrStr(b.getenv("BRAIN_AWAIT_IDLE_MAX"), "900"), 0)
	awaitPoll := atoiDefault(brOrStr(b.getenv("BRAIN_AWAIT_POLL"), "3"), 0)
	waited, last := 0, "working"
	for {
		m, read := b.hcGet(addr)
		if read && hcStr(m, "state") != "live" { // a terminal that is already gone is not one still working: hcEnd re-reads it and decides
			break
		}
		// ONLY A STATUS THAT WAS READ ENDS THE WAIT.
		status, _ := m["status"].(jmap)
		if st := hcStr(status, "state"); !read || st == "" {
			last = "unreadable"
		} else if last = st; st != "working" { // MUTATION-ANCHOR: hc-self-wait
			break
		}
		if waited >= awaitMax {
			if last == "unreadable" {
				b.errf("brain: %s reported no status to `hc get` once in the %ds since this reset was armed, so whether\n", addr, waited)
				b.errf("brain: that turn ever ended is UNKNOWN. Nothing was sent and nothing was restarted.\n")
			} else {
				b.errf("brain: %s has been WORKING for %ds since this reset was armed. brain is not going to\n", addr, waited)
				b.errf("brain: replace a conversation that never stopped talking. Nothing was sent and nothing\n")
				b.errf("brain: was restarted; arm it again when that turn is over.\n")
			}
			return 1
		}
		b.sleep(time.Duration(awaitPoll) * time.Second)
		waited += awaitPoll
	}
	if last == "blocked" { // MUTATION-ANCHOR: hc-self-blocked — the TUI's livenessGate: a reset would discard the prompt a person is asked
		b.errf("brain: %s is BLOCKED: it is waiting for a person (a permission, a question), and a reset would discard\n", addr)
		b.errf("brain: that prompt. Nothing was sent and nothing was restarted; answer it there, then arm the reset again.\n")
		return 1
	}
	b.errf("brain: the turn that armed this reset is over (%ds) — replacing %s now.\n", waited, addr)
	return 0
}
