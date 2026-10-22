package main

// hw next — re-task a live executor (lib/hw/next.sh, cmd_next and its _next_* helpers), byte for byte.
//
// The helpers cmd_next reaches in bin/hw, lib/hw/ledger.sh, bin/invoker-common.sh and the bash state witness are
// in t4_next_*.go under next… names (see t4_next_util.go): the gate (witness_wait, the question modal, the
// route), the message (the preamble, the epistemic safeguard, the authorization, the brief
// (its frontmatter), the run (env, markers, lease, receipt, ledger) and the refusals. The external commands
// are the bash's, in the bash's order, with the bash's argv: herdr, herdr-rpc, curl, git, channel-send,
// security, python3 (the two walks bash runs in Python, run verbatim), bin/runenv for a damaged env.

import (
	"os"
	"strconv"
	"strings"
)

// nextState is cmd_next's locals, and the globals its helpers set.
type nextState struct {
	pane, brief, text, waitMS, nextRun, nextSDD string
	force, dry, allowStale                      bool

	info, cwd, vendor, status string
	rundir, wtdir, prior      string
	reopened                  bool
	reopenedMarker            string
	auth                      string
	seq, marker               string

	route, target, endpoint string
	snd                     nextSender
}

func (a *app) cmdNext() int {
	// `hw next -h|--help` is the `commands` topic of help (bin/hw: exec "$0" help commands).
	if len(a.args) > 0 && (a.args[0] == "-h" || a.args[0] == "--help") {
		a.args = []string{"commands"}
		a.cmdHelp()
		return 0
	}
	s := &nextState{}
	args := a.args
	if len(args) > 0 {
		s.pane, args = args[0], args[1:]
	}
	if s.pane == "" {
		a.die("usage: hw next <executor-pane> [--run <revived-run-id>] [--sdd <mode>] [--force] [--dry-run] \"<next task>\" | --brief <path>  (flags may appear before or after the task text). Exit 0 means DELIVERED and the counter advanced; every refusal — a gate that would not open, an unknown pane, a task that has not reported — exits 1 and moves nothing.")
	}
	needVal := func() string {
		if len(args) < 2 {
			a.die(args[0] + " needs a value")
		}
		v := args[1]
		args = args[2:]
		return v
	}
	addText := func(t string) {
		if s.text != "" {
			s.text += " " + t
		} else {
			s.text = t
		}
	}
loop:
	for len(args) > 0 {
		switch w := args[0]; {
		case w == "--brief":
			s.brief = needVal()
		case w == "--force":
			s.force = true
			args = args[1:]
		case w == "--dry-run":
			s.dry = true
			args = args[1:]
		case w == "--wait-ms":
			s.waitMS = needVal()
		case w == "--sdd":
			s.nextSDD = needVal()
		case w == "--run":
			s.nextRun = needVal()
		case w == "--allow-stale-hooks":
			s.allowStale = true
			args = args[1:]
		case w == "--":
			if len(args) > 1 {
				addText(strings.Join(args[1:], " "))
			}
			break loop
		case strings.HasPrefix(w, "-"):
			a.die("unknown option: " + w)
		default: // a flag is valid in ANY position relative to the task text
			addText(w) // # MUTATION-ANCHOR: 862-G04
			args = args[1:]
		}
	}

	if s.brief != "" {
		if s.text != "" {
			a.die("pass a brief or a task text, not both")
		}
		if !nextReadable(s.brief) {
			a.die("cannot read brief: " + s.brief)
		}
		b, err := os.ReadFile(s.brief)
		if err != nil { // `text="$(cat "$brief")"` under set -e: cat's own words, and the run ends
			a.nextCatFailed(s.brief, err)
		}
		s.text = bashSub(string(b))
	}
	if s.text == "" {
		a.die("the next task is empty — there is nothing to send")
	}
	auth, ok := a.nextAuthorization(s.brief)
	if !ok {
		a.die("the re-task brief's authorization was refused above. NOTHING was sent.")
	}
	s.auth = auth
	// THE REVIEW THE NEW BRIEF CHOSE is checked here too, before anything is sent: a bad value, or `plugins` with
	// nothing to name, refuses the re-task exactly as the launch does.
	if s.brief != "" {
		a.nextReviewGate(s.brief)
	}
	switch s.nextSDD {
	case "", "speckit", "none":
	case "gentle": // RETIRED SPELLING (gentle-ai left hw): warn and run none, as the launch does.
		a.warn("hw next --sdd gentle is retired: gentle-ai is no longer part of hw. Running --sdd none.")
		s.nextSDD = "none"
	default:
		a.die("--sdd must be speckit or none (got: " + s.nextSDD + ")")
	}

	// _next_load_invoker_lib: the invoker library and the state witness, beside bin/hw
	bin := a.nextBinDir()
	if !nextReadable(bin + "/invoker-common.sh") {
		a.die("cannot find invoker-common.sh beside " + bin + " — this brain/bin is incomplete")
	}

	s.info = nextHerdrSub("agent", "get", s.pane)
	if s.info == "" {
		a.die(a.nextMissingPaneMessage(s.pane))
	}
	s.cwd = jqRawOr(s.info, "", "result", "agent", "cwd")
	s.vendor = jqRawOr(s.info, "", "result", "agent", "agent")
	s.status = jqRawOr(s.info, "", "result", "agent", "agent_status")
	if s.cwd == "" {
		a.die("herdr reports no cwd for " + s.pane + ", so hw cannot find its run directory")
	}
	rundir, found := nextRunDir(s.cwd)
	if !found {
		a.die("no hw run directory at or above " + s.cwd + " — that pane was not launched by hw, so it has no task state to advance")
	}
	s.rundir = rundir

	// A REVIVED RUN THAT REPORTED CAN BE RE-TASKED, and only a revived one (`hw revive` marks it reopened).
	if s.nextRun != "" {
		if strings.Contains(s.nextRun, "/") || strings.HasPrefix(s.nextRun, ".") {
			a.die("--run takes a run id (a directory name under .hw/), not a path: " + s.nextRun)
		}
		hw := nextDirname(s.rundir)
		if !isDir(hw + "/" + s.nextRun) {
			a.die("no run '" + s.nextRun + "' beside " + nextBasename(s.rundir) + " under " + hw + ". NOTHING was sent.")
		}
		s.rundir = hw + "/" + s.nextRun
		lease := nextLeaseEligibility(s.rundir, nextCurrentTask(s.rundir))
		if isFile(nextDoneMarker(s.rundir)) && !isFile(nextReopenedMarker(s.rundir)) && lease != "live" { // # MUTATION-ANCHOR: 862-G05
			proj, rc := a.nextRunEnv(s.rundir, "HW_PROJECT", false)
			if rc != 0 {
				proj = "<project>"
			}
			task, rc2 := a.nextRunEnv(s.rundir, "HW_TASK", false)
			if rc2 != 0 {
				task = "<task>"
			}
			a.die("run " + s.nextRun + " of " + s.pane + " reported and was not revived (no reopened marker), so `hw next --run` has no authority to re-task it. NOTHING was sent and the counter did NOT move. Revive it first: hw revive " + bashSub(proj) + " " + bashSub(task) + " --run " + s.nextRun + " — then re-run this. A run still on its chaining lease needs no --run.")
		}
	}
	s.reopenedMarker = nextReopenedMarker(s.rundir)
	if isFile(s.reopenedMarker) && isFile(nextDoneMarker(s.rundir)) {
		s.reopened = true
	}

	s.wtdir = nextDirname(nextDirname(s.rundir))
	if !isDir(s.wtdir) {
		s.wtdir = s.cwd
	}
	if files := a.nextRefuseIfStaleHooks(s.wtdir, s.allowStale); files != "" {
		a.receiptInto(s.rundir, "stale_hooks_allowed", files, "--allow-stale-hooks (hw next)")
	}
	s.prior = nextDispatchFramework(s.rundir)
	// A pane launched --sdd gentle before gentle-ai left hw still carries ODD in its system prompt for the whole
	// session; hw no longer adds anything to it and says so once.
	if s.prior == "gentle" {
		a.warn(s.pane + " was launched --sdd gentle, which is retired: its session still carries ODD, and hw adds nothing of it to this task.")
	}
	effective := a.nextValidateSDDVendor(s.vendor, s.nextSDD, s.wtdir, s.prior)
	a.nextValidateSDDModel(nextDispatchModel(s.rundir), effective, "this pane was launched with that model; hw next cannot change it")

	a.nextReportPaneContext(s.pane)

	if s.waitMS == "" {
		s.waitMS = a.getenv("HW_NEXT_WAIT_MS")
		if s.waitMS == "" {
			s.waitMS = a.nextWaitFromRun(s.rundir)
		}
		if s.waitMS == "" {
			s.waitMS = "90000"
		}
	}
	if s.waitMS == "" || strings.Trim(s.waitMS, "0123456789") != "" {
		a.die("--wait-ms/HW_NEXT_WAIT_MS must be a positive integer in milliseconds (got: " + s.waitMS + ")")
	}
	waitMS, _ := nextArith(s.waitMS)

	s.seq = ""
	if nextReadable(s.rundir + "/task") {
		s.seq = nextTaskDigits(s.rundir)
	}
	if s.seq == "" || s.seq == "0" {
		s.seq = "1"
	}
	s.marker = nextDoneMarker(s.rundir)

	// An unreported task is not a finished one (_report_evidence: the marker, or the pane's done tokens).
	if !isFile(s.marker) && !s.force { // # MUTATION-ANCHOR: 862-G07
		ev := a.nextReportEvidence(s.pane, s.rundir)
		if strings.HasPrefix(ev, "token") {
			a.nextReportEvidenceExplain(s.pane, ev, s.marker)
			a.info("proceeding without --force; read its report with: herdr agent get " + s.pane)
		} else {
			a.die("task " + s.seq + " of " + nextBasename(s.rundir) + " has not reported: no " + s.marker + ", and " + s.pane + " publishes no done token either — so there is no evidence it finished, from either source. Wait for its done-invoker, or override with --force if you are deliberately re-tasking an executor that will never report.")
		}
	}

	// The route, resolved BEFORE the gate so a dry run proves it too.
	a.nextResolveRoute(s, s.info)

	nextSeq, _ := nextArith(s.seq)
	nextSeqS := strconv.FormatInt(nextSeq+1, 10)

	if s.dry {
		return a.nextDryRun(s, nextSeqS, waitMS)
	}

	// Gate BEFORE the counter moves.
	gateRC := 0
	if nextFinishedStaleStuck(s.info) {
		a.info(s.pane + " already reported with delivered completion tokens; ignoring its stale blocked/stuck lifecycle token and re-tasking without --force.")
	} else {
		if a.nextClearQuestion(s.pane) == 0 {
			a.info(s.pane + " was blocked on its own opencode question modal; dismissed it (the executor sees question.rejected) and the pane reached idle. Re-tasking now.")
		}
		var w nextWitness
		gateRC = a.nextWitnessWait(s.pane, waitMS, &w) // # MUTATION-ANCHOR: 862-G08
		s.snd.witness = w
	}
	if gateRC != 0 {
		if a.nextClearQuestion(s.pane) == 0 {
			a.info(s.pane + " was blocked on its own opencode question modal; dismissed it (the executor sees question.rejected) and the pane reached idle. Re-tasking now.")
		} else {
			busy := nextHerdrSub("agent", "get", s.pane)
			if nextFinishedStaleStuck(busy) {
				a.info(s.pane + " reported while the gate was checking it; delivered completion tokens now prove blocked/stuck is stale. Re-tasking without --force.")
				a.nextResolveRoute(s, "")
			} else {
				detail := s.snd.witness.detail
				if gateRC == 6 {
					a.warn(detail)
					a.die(s.pane + " is waiting for a PERSON, not for time: its own endpoint confirms a prompt is outstanding (see above). The task counter did NOT move and nothing was sent. Answer that prompt on the pane — or have " + a.nextOperator() + " answer it — and re-run this identical hw next. Do not raise the budget and do not reach for --force: neither answers a question.")
				}
				if gateRC == 5 {
					a.warn(detail)
					a.die(nextSettledMessage(s.pane, waitMS, busy))
				}
				if detail != "" {
					a.warn("cross-check: " + detail)
				}
				a.die(nextBusyMessage(s.pane, waitMS, busy))
			}
		}
	}
	return a.nextDeliver(s, nextSeqS)
}

// nextReadable is `[ -r <path> ]`.
func nextReadable(p string) bool {
	_, err := os.Stat(p)
	if err != nil {
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// nextCatFailed is `text="$(cat "$brief")"` failing under set -e: cat's message, and bin/hw ends with 1.
func (a *app) nextCatFailed(path string, err error) {
	msg := "Is a directory"
	if pe, ok := err.(*os.PathError); ok && !isDir(path) {
		msg = nextStrerror(pe.Err)
	}
	a.err.Write([]byte("cat: " + path + ": " + msg + "\n"))
	panic(exitNow{1})
}

func nextStrerror(e error) string {
	s := e.Error()
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func nextDirname(p string) string {
	if i := strings.LastIndexByte(strings.TrimRight(p, "/"), '/'); i > 0 {
		return strings.TrimRight(p[:i], "/")
	} else if i == 0 {
		return "/"
	}
	return "."
}

func nextBasename(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// nextHerdrSub is `$(herdr <args> 2>/dev/null || true)`.
func nextHerdrSub(args ...string) string {
	out, _ := herdrOut(0, args...)
	return bashSub(out)
}
