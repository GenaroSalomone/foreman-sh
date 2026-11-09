package main

// THE EXECUTOR IN THE HERDR APP (herdr-app-dispatch, step 2 of «foreman en la herdr app»).
//
// When the dispatching shell's nearest multiplexer ancestor is herdr-core (core/internal/mux, design §3),
// `hw` launches the executor as ONE `hc run` terminal instead of a herdr workspace/tab/pane. It is a second
// branch at the two existing seams — `buildSpace` (nothing to build) and `agentStep` (the agent start and the
// brief) — and no new flag: the environment already says where the process runs. A TUI dispatch takes the
// path it always took, byte for byte: `hcRoute` is false with no HC_TERMINAL_ID.
//
// shortcut: claude executors on the plain two-pane lanes (setup, git-worktree, repoless) only. A lane with
// dev servers needs `hc run` terminals of its own, and another vendor needs its own argv; both die by name
// here instead of launching half a task. `hw done`/reap on hc and `channel-send` to an hc brainer are the
// next slices.

import (
	"strconv"
	"strings"
	"time"
)

// hcRoute is the one question this file asks (cached by dsp.onHC: the answer walks `ps`).
func (l *lch) hcRoute() bool { return l.onHC() }

// hcLayoutDoc stands where `layout.apply` answered: the launch asks it for the labelled agent and shell panes.
const hcLayoutDoc = `{"layout":{"panes":[{"type":"pane","label":"agent","pane_id":"hc"},{"type":"pane","label":"shell","pane_id":"hc"}]}}`

// hcLaneGate refuses, BEFORE anything is built, what this step does not launch: a lane that provisions dev
// servers or a database (they need `hc run` terminals of their own), and any vendor but claude. A refusal at
// buildSpace would come after the worktree and the database exist.
func (l *lch) hcLaneGate(build string) {
	if !l.hcRoute() {
		return
	}
	switch build {
	case "setup.sh", "git-worktree.sh", "repoless":
	default:
		l.die("this dispatch runs under herdr-core (the herdr app), and " + l.proj + " builds through " + build + ": dev servers and databases need `hc run` terminals of their own, which this step does not launch. NOTHING was built. Dispatch from a herdr TUI pane.")
	}
	if l.sandbox {
		l.die("this dispatch runs under herdr-core (the herdr app), and the sandbox is armed on a herdr pane this route does not have: the executor would run UNCONFINED. NOTHING was built. Dispatch from a herdr TUI pane, or without --sandbox.")
	}
	if l.agent != "claude" {
		l.die("this dispatch runs under herdr-core (the herdr app), and its executors are claude only in this step (this one is " + l.agent + "). NOTHING was built. Dispatch from a herdr TUI pane.")
	}
}

// hcSpace is buildSpace on herdr-core: there is no workspace, tab or layout to build. The run env file is
// still written, because `hw status` is judged against it.
func (l *lch) hcSpace() {
	l.writeRunEnv(l.workdir(), "{}", l.invokerPane())
	l.layoutOut = hcLayoutDoc
	l.ok("herdr-core: no herdr space, tab or layout is built — the executor is one `hc run` terminal")
}

// hcLive is the address of the live terminal that already carries this task, "" when none. It keys on
// hw_project+hw_task, which are stable: hw_run is new on every invocation, so a second `hw` never matched it.
// A failed `hc ls` is not "none": reading it so launches a duplicate.
func (l *lch) hcLive() (addr string, ok bool) {
	raw, rc := l.sh(cmdSpec{name: "hc", args: []string{"ls", "--json", "--label", "hw_project=" + l.proj, "--label", "hw_task=" + l.task}, quiet: true, synth: `{"terminals":[]}`})
	v, parsed := bjParse(raw)
	list, isList := bjGet(v, "terminals").([]any)
	if rc != 0 || !parsed || !isList {
		return "", false
	}
	for _, e := range list {
		if m, isMap := e.(jmap); isMap && hcStr(m, "address") != "" {
			return hcStr(m, "address"), true
		}
	}
	return "", true
}

// hcRecord is the run's mark as an hc run, <rundir>/hc: what `hw done`, reap and status key their hc calls on.
func (l *lch) hcRecord(addr string) {
	if wd, run := l.workdir(), l.hwRun; wd != "" && run != "" {
		l.fs.mkdirAll(wd + "/.hw/" + run)
		l.fs.write(wd+"/.hw/"+run+"/hc", []byte(l.proj+" "+l.task+" "+addr+"\n"), 0o666)
	}
}

// hcEnvWords is hwEnvArgs as the `K=V` words `env` takes in an hc argv (`--env K=V` is herdr's grammar).
func (l *lch) hcEnvWords(cwd string) []string {
	var words []string
	f := strings.Fields(strings.Join(l.hwEnvArgs(cwd), " "))
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "--env" {
			words = append(words, f[i+1])
			i++
		}
	}
	return words
}

// hcRunArgs is the whole `hc run` argv, exactly design §6b. Each `-u` is its own word: the P12 accident was an
// unset list passed as one string, and it let the launcher's HERDR_PANE_ID into a terminal that wrote onto it.
func (l *lch) hcRunArgs(cwd string) []string {
	args := []string{"run", "--bg", "--json", "--no-parent", "--no-notify",
		"--name", l.task, "--cwd", cwd,
		"--label", "hw_project=" + l.proj, "--label", "hw_task=" + l.task, "--label", "hw_run=" + l.hwRun,
		"--label", "herdr.space=" + l.c.t.get(l.proj, "space"),
		"--", "env",
		"-u", "HERDR_ENV", "-u", "HERDR_PANE_ID", "-u", "HERDR_SOCKET_PATH",
		"-u", "HERDR_TAB_ID", "-u", "HERDR_WORKSPACE_ID", "-u", "HW_INVOKER_PANE",
		"-u", "CLAUDE_CODE_CHILD_SESSION",
		"CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1", "CLAUDE_CODE_BASH_EDIT_DIFF=0"}
	args = append(args, l.hcEnvWords(cwd)...)
	args = append(args, "claude")
	args = append(args, strings.Fields(l.agentArgs)...)
	if l.briefText != "" { // the brief is the launch task: claude's first message, no keystrokes to race
		// `--` ends the options: --mcp-config is variadic and would otherwise read the brief as one more config file
		args = append(args, "--", l.briefText)
	}
	return args
}

// hcAgentStep is agentStep on herdr-core: reuse the task's live terminal, or launch one with the brief, and
// prove the brief arrived by the terminal's own `status`/`turn`.
func (l *lch) hcAgentStep() {
	if addr, ok := l.hcLive(); !ok {
		l.die("`hc ls` could not answer, so whether " + l.proj + ":" + l.task + " is already live is UNKNOWN — launching now could start a second executor. NOTHING was launched; the worktree is intact. Dispatch again when `hc ls` answers.")
	} else if addr != "" {
		l.hcRecord(addr)                                                      // the newest run is the one `hw done` reads
		l.receipt("pane", addr, "hc ls --label hw_project= --label hw_task=") // the in-use guard reads the pane from it
		l.ok(l.proj + ":" + l.task + " is already live on herdr-core as " + addr + " — reused, nothing launched")
		return
	}
	l.engramRegisterSession()
	l.ledgerCommitBrief()
	if l.brief != "" {
		l.buildBriefText()
		nbytes := strconv.Itoa(len(l.briefText))
		l.receipt("brief_sha256", l.dlSha256(l.briefText), "shasum -a 256 of the "+nbytes+" bytes passed to `hc run` as the launch task")
		l.receipt("brief_bytes", nbytes, "wc -c of BRIEF_TEXT at launch")
	}
	if l.explorerArm {
		l.warn("the cheap explorer is NOT armed on herdr-core: its launch function is typed into a pane shell, and an hc terminal has none")
	}
	raw, rc := l.sh(cmdSpec{name: "hc", args: l.hcRunArgs(l.workdir()), mut: true, merge: true, synth: `{"address":"shadow:t1"}`})
	v, _ := bjParse(brTrimNL(raw))
	addr := ""
	if m, isMap := v.(jmap); isMap {
		addr = hcStr(m, "address")
	}
	if rc != 0 || addr == "" {
		l.receipt("agent_ready", "no — `hc run` did not start the executor", "hc run exit status")
		l.receiptVerdict()
		l.die("`hc run` did not start " + l.agent + " for " + l.label + " (exit " + strconv.Itoa(rc) + "): " + head3(raw) + " — NOTHING is running. The worktree is intact; dispatch again.")
	}
	l.info(l.agent + " started in " + addr + " (herdr-core); vendor runtime readiness is unestablished")
	l.receipt("pane", addr, "hc run --json address")
	l.hcRecord(addr)
	l.receipt("agent_ready", "unknown — hc run answered with "+addr+"; runtime readiness unestablished", "hc run --json")
	l.dispatchPublishHC(addr)
	if l.brief != "" && !l.hcBriefProved(addr) {
		l.receipt("brief_admitted", "no — "+addr+" showed neither status=working nor a turn within the wait", "hc get "+addr+" --json")
		l.ledgerDispatch(addr, 1)
		l.receiptVerdict()
		l.die("the brief was passed to " + addr + " as its launch task, but `hc get " + addr + "` shows neither status=working nor a turn after the wait: it may be holding a dialog. Read it with `hc read " + addr + "`. Do NOT re-send the brief.")
	}
	if l.brief != "" {
		l.receipt("brief_admitted", "yes — "+addr+" reports status=working or a turn past 0", "hc get "+addr+" --json")
		l.ok("brief IN THE CONVERSATION on " + addr + " (" + strconv.Itoa(len(l.briefText)) + " bytes, from " + l.brief + ") — the launch task, proved by hc get")
	}
	l.ledgerDispatch(addr, 0)
	l.receiptVerdict()
}

// dispatchPublishHC is dispatchPublish's twin: the labels `hc run` already set are the executor's identity on
// the cockpit; the brainer to report to is the one fact left to write.
func (l *lch) dispatchPublishHC(addr string) {
	if inv := l.invokerPane(); inv != "" {
		l.sh(cmdSpec{name: "hc", args: []string{"label", addr, "hw_invoker=" + inv}, mut: true, quiet: true})
	}
}

// hcBriefProved polls `hc get` until the terminal is working or its turn left 0. false is "not proved".
func (l *lch) hcBriefProved(addr string) bool {
	budget := lrcMillis(l.env("HW_HC_PROOF_MS"), 30000)
	for waited := int64(0); ; waited += 1000 {
		out, rc := l.sh(cmdSpec{name: "hc", args: []string{"get", addr, "--json"}, quiet: true, synth: `{"state":"live","turn":1}`})
		v, ok := bjParse(brTrimNL(out))
		if m, isMap := v.(jmap); rc == 0 && ok && isMap {
			turn := pyStr(m["turn"])
			if hcStr(m, "state") == "live" && (bjRaw(bjGet(m, "status", "state")) == "working" || (turn != "" && turn != "None" && turn != "0")) {
				return true
			}
		}
		if waited >= budget {
			return false
		}
		lSleep(time.Second)
	}
}
