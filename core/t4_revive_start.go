package main

// Starting the agent in the revived pane: _agent_start and what it calls (bin/hw:1869-2038, and
// claude_session_env_prep / claude_startup_dialog / claude_dialog_advice, bin/project-spaces.sh), with the
// sandboxed variant a run launched in the sandbox comes back in. Every herdr call keeps its argv and its place.

import (
	"os"
	"os/exec"
	"strings"
	"time"
)

func (a *app) reviveHerdr() herdrClient { return herdrClient{getenv: a.getenv} }

// reviveAgentIs is `.result.agent | .pane_id == $pane and .name == $name and .agent == $kind`.
func reviveAgentIs(text, pane, name, kind string) bool {
	return reviveJqTrue(bashSub(text), func(v *jval) (bool, bool) {
		ag, ok := reviveJqPath(v, "result", "agent")
		if !ok {
			return false, false
		}
		for _, kv := range [][2]string{{"pane_id", pane}, {"name", name}, {"agent", kind}} {
			m, ok := ag.get(kv[0])
			if !ok {
				return false, false
			}
			if !reviveStrEq(m, kv[1]) {
				return false, true
			}
		}
		return true, true
	})
}

// reviveAgentReady is `.result.agent as $a | $a.interactive_ready == true and (["idle","done"] | index($a.agent_status) != null)`.
func reviveAgentReady(text string) bool {
	return reviveJqTrue(bashSub(text), func(v *jval) (bool, bool) {
		ag, ok := reviveJqPath(v, "result", "agent")
		if !ok {
			return false, false
		}
		r, ok := ag.get("interactive_ready")
		if !ok {
			return false, false
		}
		if !(r.kind == vBool && r.b) {
			return false, true
		}
		s, ok := ag.get("agent_status")
		if !ok {
			return false, false
		}
		return reviveStrEq(s, "idle") || reviveStrEq(s, "done"), true
	})
}

// reviveAgentIdle is the recheck after a failed start (bin/hw:8298). BASH BUG, ported as it is: the filter there is
// `… and (["idle", "done"] | index(.agent_status) != null)`, and `index(.agent_status)` is evaluated against the
// ARRAY it is piped from, not against the agent, so jq fails with "Cannot index array with string" for every
// reply and `jq -e` exits 5: the recheck can never recover, and a pane that did come up in the meantime is refused
// all the same. Contract cases revive-start-failrecover and the revive-screen-* ones hold it.
func (a *app) reviveAgentIdle(text, pane, name, kind string) bool {
	return false
}

// reviveDetection is `herdr agent read <target> --source detection --lines 80 2>/dev/null || true`.
func (a *app) reviveDetection(target string) string {
	out, _ := a.reviveHerdr().stdout("agent", "read", target, "--source", "detection", "--lines", "80")
	return bashSub(out)
}

// reviveHerdrQuiet runs `herdr <args> >/dev/null 2>&1`: whether it succeeded.
func (a *app) reviveHerdrQuiet(args ...string) bool {
	_, rc := a.reviveHerdr().capture(args...)
	return rc == 0
}

// reviveClaudeSessionPrep is claude_session_env_prep: a claude launched by hw is never a child session.
func (a *app) reviveClaudeSessionPrep(pane string) {
	if pane == "" {
		return
	}
	if out, ok := a.reviveHerdr().stdout("agent", "get", pane); ok && reviveJqTrue(bashSub(out), func(v *jval) (bool, bool) {
		ag, k := reviveJqPath(v, "result", "agent")
		return k && ag.kind != vNull, k
	}) {
		return
	}
	prep := "unset CLAUDE_CODE_CHILD_SESSION" // # MUTATION-ANCHOR: 863-G14
	prep += "; export CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1"
	if !a.reviveHerdrQuiet("pane", "run", pane, prep) {
		a.err.Write([]byte("claude: could not clear CLAUDE_CODE_CHILD_SESSION in " + pane + "; Claude may report 'Transcript saving is off'.\n"))
	}
}

// reviveClaudeDialog is claude_startup_dialog: the first-run screen a Claude Code pane sits on, or "".
func reviveClaudeDialog(screen string) string {
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

// reviveDialogAdvice is claude_dialog_advice for the default account (a revive resolves none).
func (a *app) reviveDialogAdvice(dialog string) string {
	switch dialog {
	case "bypass":
		return "the Bypass Permissions warning is waiting, and accepting it is yours: run `claude --dangerously-skip-permissions` once in a terminal, accept it, /exit, and try again"
	case "onboarding":
		return "Claude Code's welcome (theme, then login) has never been completed in this account: run `claude --dangerously-skip-permissions` once in a terminal, finish it and accept the Bypass Permissions warning, /exit, and try again — `claude auth login` alone does not complete it"
	case "trust":
		return "the folder-trust question is waiting: this directory is not marked trusted in " + a.getenv("HOME") + "/.claude.json"
	case "fullscreen-offer":
		return `the fullscreen-renderer offer is waiting; "Not now" (Esc) keeps the renderer as it is`
	}
	return ""
}

// reviveClaudeReady is _claude_startup_ready.
func (a *app) reviveClaudeReady(target string) bool {
	dialog := reviveClaudeDialog(a.reviveDetection(target))
	switch dialog {
	case "":
		return true
	case "fullscreen-offer":
		a.info("claude opened on its fullscreen-renderer offer — answering \"Not now\" (Esc), which keeps the renderer as it is")
		if !a.reviveHerdrQuiet("agent", "send-keys", target, "esc") {
			return false
		}
		time.Sleep(2 * time.Second)
		return true
	}
	a.err.Write([]byte("claude is waiting on a first-run screen (" + dialog + "): " + a.reviveDialogAdvice(dialog) + "\n"))
	return false
}

// reviveCodexReady is _codex_startup_ready.
func (a *app) reviveCodexReady(name string) bool {
	screen := a.reviveDetection(name)
	switch {
	case strings.Contains(screen, "Do you trust the contents of this directory"):
		if !a.reviveHerdrQuiet("agent", "send-keys", name, "enter") {
			return false
		}
		time.Sleep(2 * time.Second)
	case strings.Contains(screen, "Hooks need review"):
		a.err.Write([]byte("codex is waiting for hook trust; review that dialog before delivering a brief\n"))
		return false
	case strings.Contains(screen, "Choose how you'd like Codex to proceed"):
		a.err.Write([]byte("codex is waiting on a model migration dialog; choose a supported model before delivering a brief\n"))
		return false
	}
	return true
}

// reviveStartFailureScreen is _claude_start_failure_screen: what the pane was showing when hw gave up.
func (a *app) reviveStartFailureScreen(target string) {
	screen := a.reviveDetection(target)
	if screen == "" {
		return
	}
	var w strings.Builder
	w.WriteString("what the pane was showing when hw gave up (last lines):\n")
	var keep []string
	for _, l := range strings.Split(screen, "\n") {
		if strings.TrimLeft(l, " \t\r\v\f") != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > 8 {
		keep = keep[len(keep)-8:]
	}
	for _, l := range keep {
		w.WriteString("    " + l + "\n")
	}
	a.err.Write([]byte(w.String()))
}

// reviveSandboxArm is _sandbox_arm_pane: write the shim hw-sandbox makes and put it first on the pane's PATH.
func (a *app) reviveSandboxArm(st *reviveRunState, pane, kind, rundir string) bool {
	if !strings.HasPrefix(rundir, "/") || !strings.Contains(rundir, "/.hw/") || strings.HasSuffix(rundir, "/.hw/") {
		a.warn("sandbox: no run directory to arm (" + rundir + ")")
		return false
	}
	cmd := exec.Command(a.reviveBinDir()+"/hw-sandbox", "arm", rundir, kind, st.proj, st.task)
	cmd.Env = append(os.Environ(), "HW_WORKDIR="+st.cwd, "HW_ARTIFACTS="+a.getenv("HW_ARTIFACTS"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		a.warn("sandbox: could not arm " + rundir + " — the agent is NOT started unconfined")
		return false
	}
	shim := bashSub(string(out))
	if !a.reviveHerdrQuiet("pane", "run", pane, "export PATH="+reviveQuote(shim)+":$PATH; hash -r 2>/dev/null; clear") {
		a.warn("sandbox: could not put the shim on " + pane + "'s PATH — the agent is NOT started unconfined")
		return false
	}
	dir := shim
	if i := strings.LastIndexByte(dir, '/'); i >= 0 {
		dir = dir[:i]
	}
	a.receiptInto(rundir, "sandbox", dir+"/profile.sb", "hw-sandbox arm ("+kind+"); shim "+shim+" exported in "+pane)
	return true
}

// reviveAgentStartSandboxed is _agent_start_sandboxed: arm, start through the ordinary path, then PROVE it.
func (a *app) reviveAgentStartSandboxed(st *reviveRunState, pane, name, kind string) bool {
	rundir := st.sandboxRun
	if !a.reviveSandboxArm(st, pane, kind, rundir) {
		return false
	}
	cmd := exec.Command(a.reviveBinDir()+"/hw-sandbox", "statedir", rundir)
	cmd.Stderr = os.Stderr
	out, _ := cmd.Output()
	state := bashSub(string(out))
	st.sandboxArmed = pane
	ok := a.reviveAgentStart(st, pane, name, kind)
	st.sandboxArmed = ""
	if !ok {
		return false
	}
	// AN AGENT THAT STARTED IS NOT PROOF IT STARTED IN THE SANDBOX: the broker's heartbeat is.
	for n := 0; n < 20; n++ {
		if isFile(state + "/broker.alive") {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	a.warn("sandbox: " + kind + " started in " + pane + " but the broker never came up (" + state + "/broker.alive) — it may be running UNCONFINED. Close it: hw done " + st.proj + " " + st.task)
	return false
}

// reviveAgentStart is _agent_start: start an agent, retrying while the pane is still coming up.
func (a *app) reviveAgentStart(st *reviveRunState, pane, name, kind string) bool {
	if st.sandbox && st.sandboxArmed != pane {
		return a.reviveAgentStartSandboxed(st, pane, name, kind)
	}
	h := a.reviveHerdr()
	if kind == "claude" {
		a.reviveClaudeSessionPrep(pane) // # MUTATION-ANCHOR: 863-G15
	}
	tries, registered := 0, false
	for tries < 15 {
		if registered {
			// agent_not_ready keeps the registration: poll that exact agent, a second creation would collide
			probe, _ := h.stdout("agent", "get", pane)
			probe = bashSub(probe)
			if !reviveAgentIs(probe, pane, name, kind) {
				a.err.Write([]byte(probe + "\n"))
				return false
			}
			if kind == "codex" && !a.reviveCodexReady(pane) {
				return false
			}
			if kind == "claude" && !a.reviveClaudeReady(pane) {
				return false
			}
			if reviveAgentReady(probe) {
				return true
			}
			time.Sleep(2 * time.Second)
			tries++
			continue
		}
		argv := []string{"agent", "start", name, "--kind", kind, "--pane", pane, "--timeout", "60000"}
		if st.agentArgs != "" {
			argv = append(argv, "--")
			argv = append(argv, reviveSplitWords(st.agentArgs)...)
		}
		out, rc := h.capture(argv...)
		out = bashSub(out)
		if rc == 0 && reviveJqTrue(out, func(v *jval) (bool, bool) {
			r, ok := reviveJqPath(v, "result", "agent", "interactive_ready")
			return ok && r.kind == vBool && r.b, ok
		}) {
			// ready can be reported with a dialog still on screen, and the brief would land in it
			if kind == "codex" && !a.reviveCodexReady(name) {
				return false
			}
			if kind == "claude" && !a.reviveClaudeReady(pane) {
				return false
			}
			return true
		}
		switch {
		case strings.Contains(out, "agent_pane_busy"):
		case strings.Contains(out, "agent_not_ready"):
			registered = true
		// A startup timeout is a busted BUDGET, not a busted agent: the name stays available.
		case strings.Contains(out, "timed out waiting for agent startup"): // # MUTATION-ANCHOR: 863-G16
			registered = true
		default:
			a.err.Write([]byte(out + "\n"))
			return false
		}
		time.Sleep(2 * time.Second)
		tries++
	}
	if kind == "claude" {
		a.reviveStartFailureScreen(pane)
	}
	return false
}
