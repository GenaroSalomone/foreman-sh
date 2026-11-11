package main

// THE BRAINER IN THE HERRD APP (herdr-app-brain, step 2 of «foreman en la herdr app»).
//
// `brain <lane>` run from an herdr-core (`hc`) terminal builds the brainer as an
// hc terminal instead of a herdr workspace tab, and that terminal is a valid
// return address for the executors the brainer dispatches:
//
//   - the label `label=brain` — the TUI's `pane rename … brain`, written as a
//     key=value because hc labels are a map;
//   - its cwd is the lane's brain directory, so one lane cannot be mistaken for
//     another;
//   - `herdr.space=<the lane's space>` opts it into the app's window (MEASURED
//     2026-11-09: an id the app does not know becomes a new space; `herdr-app do
//     agent.focus --terminal <t>` answers `{"ok":true}`, and `herdr-app status`
//     then names that space, tab and pane).
//
// The backend is core/internal/mux's choice — the same one the turn-end tokens
// make, not a second one. A TUI pane takes the path it always took, byte for
// byte: nothing here runs with no HC_TERMINAL_ID.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hwcore/internal/mux"
)

// hcBackend is the one question this file asks.
func (b *brainRun) hcBackend() bool { return mux.Backend(b.getenv) == mux.HC }

// hc runs `hc <args>` through the same seam as herdr.
func (b *brainRun) hc(c brainCmd, args ...string) brainRes {
	c.Name, c.Args = "hc", args
	return b.run(c)
}

// herdrApp runs `herdr-app <args>`: the app's own commands (showing a terminal).
func (b *brainRun) herdrApp(c brainCmd, args ...string) brainRes {
	c.Name, c.Args = "herdr-app", args
	return b.run(c)
}

// hcStr is one string field of an hc reply, "" when absent.
func hcStr(m jmap, k string) string {
	if pyTruthy(m[k]) {
		return pyStr(m[k])
	}
	return ""
}

// hcTerm is the part of `hc ls` TerminalInfo the brainer lookup reads.
type hcTerm struct{ addr, cwd string }

// hcTerminals is `hc ls --json --label <k=v>…` — `hc ls` lists the live terminals
// (careful: `--all` would add finished ones) carrying every one of those labels.
// false is a refusal and not an empty list — a failed `hc ls`
// is not evidence that no brainer exists, and reading it as one builds a second.
func (b *brainRun) hcTerminals(labels ...string) ([]hcTerm, bool) {
	args := []string{"ls", "--json"}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	r := b.hc(brainCmd{KeepErr: true}, args...)
	raw := brTrimNL(r.Out)
	if r.RC != 0 {
		b.errf("brain: `hc ls` failed (exit %d), so whether the %s brainer is open is UNKNOWN.\n", r.RC, b.proj)
		b.errf("brain: refusing to continue — reading a failed query as \"nothing found\" is how one lane\n")
		b.errf("brain: ends up with two brainers. hc said: %s\n", headLinesSpaced(r.Err+raw, 2))
		return nil, false
	}
	v, ok := bjParse(raw)
	list, isList := bjGet(v, "terminals").([]any)
	if !ok || !isList {
		b.errf("brain: `hc ls --json` exited 0 but its reply has no terminals array, so it cannot be read as\n")
		b.errf("brain: \"none\". Refusing to continue. It said: %s\n", headLinesSpaced(raw, 2))
		return nil, false
	}
	out := make([]hcTerm, 0, len(list))
	for _, e := range list {
		m, isMap := e.(jmap)
		if !isMap {
			continue
		}
		out = append(out, hcTerm{addr: hcStr(m, "address"), cwd: hcStr(m, "cwd")})
	}
	return out, true
}

// hcUnsupported names the first flag hc has no mapping for yet, "" when none.
func (b *brainRun) hcUnsupported() string {
	switch {
	case b.inPane != "":
		return "--in-pane (brain relaunch)"
	case b.self:
		return "--self"
	case b.awaitPane != "":
		return "--await-idle"
	}
	return ""
}

// hcName is the terminal's name. `hc run --name` takes 1-32 of a-z 0-9 - _ and
// refuses a name already live; both are the core's to enforce, so nothing here
// truncates into a name another lane could collide with.
func (b *brainRun) hcName() string { return "brain-" + b.proj }

// hcGet is `hc get <addr> --json`: the terminal's own fields (labels, cwd), and
// false when it cannot be read.
func (b *brainRun) hcGet(addr string) (jmap, bool) {
	raw := brTrimNL(b.hc(brainCmd{}, "get", addr, "--json").Out)
	v, ok := bjParse(raw)
	m, isMap := v.(jmap)
	return m, ok && isMap
}

// sameDir is two paths naming one directory, and "" names none of them:
// pyRealpath("") is the process's own cwd, which would make a reply without a cwd
// compare equal to whatever brain was started from.
func sameDir(a, b string) bool {
	return a != "" && b != "" && pyRealpath(a) == pyRealpath(b)
}

// hcRefuse is what this step cannot do on herdr-core, in one place: a flag or a
// verb hc has no mapping for is refused by name, and nothing is sent.
func (b *brainRun) hcRefuse(what string) int {
	only := "a brainer"
	if b.proj != "" {
		only = "the " + b.proj + " brainer"
	}
	b.errf("brain: %s has no herdr-core mapping yet: this step of foreman on the herdr app opens and\n", what)
	b.errf("brain: focuses %s, and nothing else. Nothing was sent and nothing was restarted.\n", only)
	b.errf("brain: run it from a herdr TUI pane.\n")
	return 1
}

// launchHC is `brain <lane>` on herdr-core: focus this lane's brainer terminal or
// start one, then show it. What the TUI needs a workspace and a tab for, hc states
// as labels on one terminal.
//
// Reports that never reached the lane's brainer wait in `.hw/<run>/outbox/`; once the terminal is up they are handed to it
// through `hw outbox flush`, which delivers with channel-send's hc route.
func (b *brainRun) launchHC() int {
	if flag := b.hcUnsupported(); flag != "" {
		return b.hcRefuse(flag)
	}
	// `brain` is a brainer's verb: a run identity in its environment came from the app's launcher (QA5 G2), and the `hw outbox flush`
	// below would be refused as an executor's. The brainer it starts is cleared of it too (hcStartBrainer).
	for _, k := range hcRunIdentity {
		b.unsetenv(k)
	}
	terms, ok := b.hcTerminals("label=brain")
	if !ok {
		return 1
	}
	addr := ""
	for _, t := range terms {
		if sameDir(t.cwd, b.dir) {
			addr = t.addr
			break
		}
	}
	// --reset and --resume REPLACE the brainer: its terminal is ended and started again (brain_hc_reset.go).
	// ONLY A START needs the account and the environment: focusing the brainer that is already open must not fail
	// on them (the TUI focus path does not). A replacement resolves them BEFORE the old terminal is ended, so a
	// refusal there leaves the lane's brainer alive.
	replaced := false // # MUTATION-ANCHOR: go-1340-M02
	var plan brainerLaunch
	if addr == "" || b.reset || b.resume != "" {
		var planOK bool
		if plan, planOK = b.planBrainer(b.proj); !planOK {
			return 1
		}
	}
	replacing := addr != "" && (b.reset || b.resume != "")
	if replacing {
		if !b.permissionsResolve() {
			b.errf("brain: %s\n", b.permissionsErr)
			return 1
		}
		if b.resume == "" && !b.hcBlockerGate(addr) { // a resume keeps the conversation: only a reset is gated, as on herdr
			return 1
		}
		if !b.hcEnd(addr) {
			return 1
		}
		addr, replaced = "", true
	}
	if addr != "" && !replaced {
		b.engramRegister() // focusing the open brainer: no refusal is left, and the session is renewed as it always was
	}
	opened := false
	if addr == "" {
		var rc int
		if addr, rc = b.hcStartBrainer(plan); rc != 0 {
			return rc
		}
		opened = true
	}
	// agent.focus races a terminal `hc run` has just started (measured 3/3 on 0.8.0-rc.1): it is retried, and a
	// focus that never lands does not skip the outbox flush — the report is owed to the brainer whether or not it is shown.
	focused := b.hcFocus(addr)
	b.repointRuns(addr)
	b.cockpitWriter(b.kind, addr) // one per brainer (its lock): a re-run, a focus or a reset never stacks writers; it leaves when `hc get` says the terminal is gone
	if replaced {
		how := "NEW conversation"
		if b.resume != "" && !b.noConversation {
			how = "resuming " + b.resume
		}
		b.outf("restarted %s / %s (%s, %s) — %s — cwd %s\n", b.space, b.tabLabel, addr, b.kind, how, b.dir)
		b.hcWaitReady(addr)
	} else if opened {
		b.outf("opened %s / %s (%s, %s) — cwd %s\n", b.space, b.tabLabel, addr, b.kind, b.dir)
		b.hcWaitReady(addr)
	} else {
		b.outf("focused %s / %s (%s, %s) — cwd %s\n", b.space, b.tabLabel, addr, b.kind, b.dir)
	}
	b.flushOutbox(b.proj, addr)
	if !focused {
		b.errf("brain: %s is running but the app could not be told to show it; it is in the %s space.\n", addr, b.space)
		return 1
	}
	return 0
}

// hcFocus is `herdr-app do agent.focus`, tried up to 5 times a second apart: true once the app showed the terminal.
func (b *brainRun) hcFocus(addr string) bool {
	for i := 0; i < 5; i++ {
		if i > 0 {
			b.sleep(1 * time.Second)
		}
		if r := b.herdrApp(brainCmd{NoOut: true, ErrTo: true}, "do", "agent.focus", "--terminal", addr); r.RC == 0 {
			return true
		}
	}
	return false
}

// hcWaitReady holds a started brainer until its program has said a status (up to 60s), as waitThenFlush does on the TUI:
// a report typed into claude before it is up is lost. A brainer that never answers in that time is flushed to anyway (the report may then be typed in before claude is up, and a flush that fails keeps it in the outbox).
func (b *brainRun) hcWaitReady(addr string) {
	for i := 0; i < 60; i++ {
		if info, ok := hcAgentGetJSON(addr); ok && jqRawOr(info, "unknown", "result", "agent", "agent_status") != "unknown" {
			return
		}
		b.sleep(1 * time.Second)
	}
}

// hcStartBrainer is `hc run`: the terminal is started in the lane's directory with
// the environment in its argv (the core runs the program with its own, P3) and
// without the inherited herdr variables (P12: they name somebody else's pane).
func (b *brainRun) hcStartBrainer(plan brainerLaunch) (string, int) {
	// skip or ask, resolved as hw resolves it.
	if !b.permissionsResolve() {
		b.errf("brain: %s\n", b.permissionsErr)
		return "", 1
	}
	if b.permissions != "skip" {
		b.errf("brain: permissions: ask (%s) — the brainer will stop on permission prompts; answer them in its terminal.\n", b.permissionsSrc)
	}
	agentArgs, argRC := b.brainerAgentArgs(plan, "", "", true)
	if argRC != 0 {
		return "", argRC
	}
	// THE LAST REFUSAL IS BEHIND US (hcEnd's, the permissions', the agent args'): the session is registered for a terminal that starts now.
	plan = b.withEngramSession(plan)
	argv := []string{"env",
		"-u", "HERDR_ENV", "-u", "HERDR_PANE_ID", "-u", "HERDR_SOCKET_PATH",
		"-u", "HERDR_TAB_ID", "-u", "HERDR_WORKSPACE_ID", "-u", "HW_INVOKER_PANE",
		// what sessionEnvPrep types into a TUI pane shell: there is no shell here to type into
		"-u", "CLAUDE_CODE_CHILD_SESSION",
	}
	// env stops reading options at its first NAME=VALUE word: every -u goes before the assignments
	if plan.accountFlag == "" { // the default account: trust and first-run were read from $HOME/.claude.json, so an inherited CLAUDE_CONFIG_DIR must not move the brainer elsewhere (hw clears it for executors too)
		argv = append(argv, "-u", "CLAUDE_CONFIG_DIR")
	}
	for _, k := range hcRunIdentity {
		argv = append(argv, "-u", k)
	}
	argv = append(argv, "CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1", "CLAUDE_CODE_BASH_EDIT_DIFF=0")
	argv = append(argv, plan.pairs...)
	if id := hcCoreID(b.getenv); id != "" { // the cockpit plugin in this terminal names its state file <core>.<address>.json
		argv = append(argv, "HW_COCKPIT_CORE="+id)
	}
	argv = append(argv, hcPathWord(b.binDir, b.getenv("PATH")))
	argv = append(argv, b.kind)
	argv = append(argv, agentArgs[1:]...) // agentArgs[0] is `agent start`'s `--`
	args := []string{"run", "--bg", "--json", "--no-parent", "--no-notify", "--keep",
		"--name", b.hcName(), "--cwd", b.dir,
		"--label", "label=brain", "--label", "herdr.space=" + b.space,
		"--"}
	args = append(args, argv...)
	app := func(a ...string) (string, int) { r := b.herdrApp(brainCmd{}, a...); return r.Out, r.RC }
	spaceNew := hcSpaceIsNew(app, b.space)
	raw := brTrimNL(b.hc(brainCmd{Merge: true}, args...).Out)
	v, _ := bjParse(raw)
	addr := ""
	if m, isMap := v.(jmap); isMap {
		addr = hcStr(m, "address")
	}
	if addr == "" {
		b.errf("brain: `hc run` did not start the brainer: %s\n", brainFirstLines(raw, 2))
		return "", 1
	}
	// AN ADDRESS IS NOT A RETURN ADDRESS. `hc run` answering with one proves the
	// program started, not that the terminal is findable and provable by the two
	// facts the resolver checks. The TUI path refuses in the same place, when its
	// tab rename fails, because the next `brain <lane>` would build a second
	// brainer beside this one.
	m, got := b.hcGet(addr)
	labels, _ := m["labels"].(jmap)
	if got && hcStr(labels, "label") == "brain" && sameDir(hcStr(m, "cwd"), b.dir) {
		if spaceNew {
			hcNameSpace(app, b.sleep, b.space)
		}
		return addr, 0
	}
	b.errf("brain: %s started but is not provably this lane's brainer (label=brain, cwd %s), so the next\n", addr, b.dir)
	b.errf("brain: `brain %s` would build a second one and `hw` could not report to it. End it with\n", b.proj)
	b.errf("brain: `hc kill %s` and re-run.\n", addr)
	return "", 1
}

// repointRuns gives the lane's runs the brainer's NEW address: an executor's run env names the terminal that dispatched it,
// and the cockpit keys its rows on that name, so a reopened brainer (t2 -> t7) showed no executors while the old one's
// rows sat in a dead file. Only an hc address that `hc get` says is not live is replaced; a live one is somebody's.
func (b *brainRun) repointRuns(addr string) {
	envs, _ := filepath.Glob(filepath.Join(b.work, b.proj, "*", ".hw", "*", "env"))
	gone := map[string]bool{}
	mine := hcCoreID(b.getenv)
	for _, f := range envs {
		// a run of ANOTHER core (a QA instance, a restart that recycles the numbers) names an address that means nothing here:
		// `hc get` would answer for whoever holds that number in THIS core, so its liveness is not this run's.
		if c, err := os.ReadFile(filepath.Dir(f) + "/hc-core"); err == nil && mine != "" && strings.TrimSpace(string(c)) != "" && strings.TrimSpace(string(c)) != mine {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		m := reInvokerRow.FindSubmatch(raw)
		if m == nil || string(m[1]) == addr || !mux.IsHCAddress(b.getenv, string(m[1])) {
			continue
		}
		old := string(m[1])
		if _, seen := gone[old]; !seen {
			info, ok := b.hcGet(old)
			gone[old] = !ok || hcStr(info, "state") != "live"
		}
		if gone[old] {
			tmp := f + ".repoint" // rename, so a reader never sees half a row
			if os.WriteFile(tmp, reInvokerRow.ReplaceAllLiteral(raw, []byte("HW_INVOKER_PANE='"+addr+"'")), 0o600) == nil && os.Rename(tmp, f) != nil {
				os.Remove(tmp)
			}
		}
	}
}

var reInvokerRow = regexp.MustCompile(`(?m)^HW_INVOKER_PANE='([^'\n]*)'`)
