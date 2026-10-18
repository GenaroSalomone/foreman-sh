package invokers

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	doneMaxChars = 900
	doneTTLMs    = "86400000"
	doneChunks   = 7

	exitDeliveredNotClosed = 4
	exitDeliveryUncertain  = 5
	exitHeldForRuling      = 6
)

// Done is bin/done-invoker.
func Done(c *Ctx, args []string) int {
	c.Prog = "done-invoker"
	return c.finish(c.exitCode(c.done(args)))
}

func notClosed(format string, a ...any) *Die {
	return &Die{Msg: sprintf(format, a...), Code: exitDeliveredNotClosed}
}

var engramIDRe = regexp.MustCompile(`#[0-9]+`)

func hasNonSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		if !posixSpace(s[i]) {
			return true
		}
	}
	return false
}

// laneEngram is `lane_get <project> engram` from project-spaces.sh ("" on failure).
func (c *Ctx) laneEngram(project string) string {
	const script = `[ -r "$1/project-spaces.sh" ] || exit 1
. "$1/project-spaces.sh" >/dev/null 2>&1 || true
command -v lane_get >/dev/null 2>&1 || exit 1
if command -v lane_config_load >/dev/null 2>&1; then lane_config_load "${HW_BRAIN_ROOT:-$(dirname "$1")}" >/dev/null 2>&1 || true; fi
lane_get "$2" engram 2>/dev/null`
	var b bytes.Buffer
	if rc := c.run(runOpts{stdout: &b}, "bash", "-c", script, "_", c.BinDir, project); rc != 0 {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

func (c *Ctx) done(args []string) error {
	if c.get("HW_SANDBOX") == "1" { // 322-M02
		sb := c.BinDir + "/hw-sandbox"
		if st, err := os.Stat(sb); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			return die("running in the sandbox, but %s is missing, so nothing can leave it. NOTHING was sent.", sb)
		}
		rc := c.run(runOpts{stdin: c.Stdin, stdout: c.Stdout, stderr: c.Stderr}, sb, append([]string{"request", "done-invoker"}, args...)...)
		c.exitSpoken = true
		return &Die{Code: rc}
	}
	c.loadRoots()

	status := "done"
	retask := false
loop:
	for len(args) > 0 {
		switch args[0] {
		case "--blocked":
			status = "blocked"
		case "--done":
		case "--retask":
			retask = true
		default:
			break loop
		}
		args = args[1:]
	}

	if len(args) > 0 {
		switch {
		case args[0] == "--":
			args = args[1:]
		case args[0] == "-":
		case strings.HasPrefix(args[0], "-"):
			return die("unknown option: %s — this invoker takes NO flags but [--blocked] and [--retask], and the status is that flag, not a value. Silently folding it into the summary is how a blocked task gets filed as done. Usage: done-invoker [--blocked] [--retask] \"<one paragraph>\" (artifacts are counted from $HW_ARTIFACTS, do not pass them)", args[0])
		}
	}

	var summary string
	if len(args) > 0 && args[0] == "-" {
		if len(args) != 1 { // 135-M02
			return die("the summary was given on argv AND as '-' for stdin — pass it ONE way, not both")
		}
		if c.StdinTTY {
			return die("'-' reads the summary from stdin, but stdin is a terminal — pipe or redirect the text in")
		}
		var b []byte
		var err error
		if c.Stdin != nil {
			b, err = io.ReadAll(c.Stdin)
		}
		if err != nil {
			return die("could not read the summary from stdin")
		}
		summary = strings.TrimRight(string(b), "\n")
		if !hasNonSpace(summary) {
			return die("stdin for '-' produced no bytes — an empty summary delivered would be worse than this refusal. NOTHING was sent or marked.")
		}
	} else {
		if len(args) < 1 {
			return die("usage: done-invoker [--blocked] [--retask] \"<one paragraph>\" | done-invoker [--blocked] [--retask] - (reads the paragraph from stdin)")
		}
		summary = strings.Join(args, " ")
		if !hasNonSpace(summary) {
			return die("summary is empty. A completion report needs a substantive one-paragraph pointer to what was established and its engram ids; zero artifacts is valid, an empty summary is not. NOTHING was sent or marked.")
		}
	}

	if c.has("HW_INVOKER_PANE") && c.get("HW_INVOKER_PANE") == "" { // 68-M01 68-M02
		return die("HW_INVOKER_PANE is set and EMPTY, so this call has been told there is no brainer to reach. REFUSING before the run lock, any counter and any delivery: nothing was sent, no ask was spent and no marker was written. That is deliberate — an explicitly empty address is how a test or a smoke run makes the live channel unreachable, and honouring it is why a probe can no longer reach a real brainer by accident (2026-09-02). To report for real, UNSET it (`unset HW_INVOKER_PANE`) so the run's .hw/<run>/env can supply the address, or export the pane id you mean.")
	}

	c.adoptEnvFile()
	if err := c.runLockAcquire(); err != nil { // 66-M06
		return err
	}
	var reportingMarker string
	c.cleanups = append(c.cleanups, func() {
		if reportingMarker != "" {
			os.Remove(reportingMarker)
		}
	})

	pane := c.get("HW_INVOKER_PANE")
	if pane == "" {
		return die("no HW_INVOKER_PANE — this pane was not launched by hw with --here, so there is no brainer to notify. Report to %s in this pane instead.", c.operator())
	}
	herdrPane := c.get("HERDR_PANE_ID")
	if herdrPane == "" {
		return die("no HERDR_PANE_ID — not running inside a herdr pane.")
	}

	task := c.or("HW_TASK", "unknown")
	project := c.or("HW_PROJECT", "unknown")
	workdir := c.or("HW_WORKDIR", "<unknown>")
	artifacts := c.or("HW_ARTIFACTS", workdir)

	if n := len(summary); n > doneMaxChars {
		return die("summary is %d chars, limit is %d. This is a pointer, not the report — mem_save the report and name its id here.", n, doneMaxChars)
	}
	if strings.Contains(summary, "```") {
		return die("no code fences. The findings belong in engram, not in this message.")
	}

	receipt := c.runDir() + "/receipt.jsonl"
	// -- a completion names an observation that exists, under this lane
	if status == "done" {
		if sess := receiptValue(receipt, "engram_session"); strings.HasPrefix(sess, "hw-") {
			label := c.get("ENGRAM_PROJECT")
			if label == "" {
				label = c.laneEngram(c.get("HW_PROJECT"))
			}
			if label == "" {
				label = project
			}
			if st, err := os.Stat(c.BinDir + "/engram-serve.sh"); err != nil || st.IsDir() {
				return die("cannot find engram-serve.sh beside %s — this brain/bin is incomplete. NOTHING was sent or marked.", c.BinDir)
			}
			var ids []string
			seen := map[string]bool{}
			for _, m := range engramIDRe.FindAllString(summary, -1) {
				id := m[1:]
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
			if len(ids) == 0 {
				return die("the summary names no engram observation. Name each one as #<id> (the id mem_save returned, saved with project: %s and session_id: %s). If every save was refused, that is a blocker: report it with --blocked and quote the refusal. NOTHING was sent or marked.", label, sess)
			}
			url := c.engramServeURL()
			verdict, vrc := c.engramVerdict(url)
			if vrc != 0 {
				return die("cannot verify the observations you named: %s. A report whose engram ids cannot be checked is not accepted as done — report it with --blocked, naming this line. NOTHING was sent or marked.", verdict)
			}
			ok := false
			seenNote := ""
			for _, id := range ids {
				proj := engramObservationProject(id, url)
				if proj == label {
					ok = true
					break
				}
				if proj == "" {
					proj = "missing"
				}
				seenNote += " #" + id + "=" + proj
			}
			if !ok {
				return die("none of the observations you named exists under project %s:%s. A save that landed elsewhere (manual-save-*, another project) is not this lane's report. Save again with project: %s and session_id: %s, name the returned id as #<id>, and re-run. NOTHING was sent or marked.", label, seenNote, label, sess)
			}
		}
	}

	// -- a task whose brief declared a boundary closes on an approved design
	if status == "done" {
		if boundary := receiptValue(receipt, "design_boundary_t"+c.taskSeq()); boundary != "" {
			if err := DesignGate(artifacts, boundary); err != nil {
				return err
			}
		}
	}
	// -- a build closes on checks.md
	checksNote := ""
	if status == "done" { // 818-M05
		if ck := receiptValue(receipt, "checks_required_t"+c.taskSeq()); ck != "" { // 818-M04
			n, err := ChecksGate(artifacts)
			if err != nil {
				return err
			}
			checksNote = n
		}
	}

	retaskNote := ""
	if retask {
		runDir := c.runDir()
		seq := c.taskSeq()
		if !isRegular(c.stateDir() + "/done") {
			return die("--retask claims the NEXT task number, and task %s has not reported yet — so there is nothing to move past. Drop the flag: plain done-invoker is what reports this task. (If you already ran --retask and only the delivery failed, the counter moved then; re-run WITHOUT it.)", seq)
		}
		next := itoa(atoi10(seq) + 1)
		if err := os.MkdirAll(runDir+"/t"+next, 0o777); err != nil {
			return die("cannot create %s/t%s, so --retask cannot claim task %s", runDir, next, next)
		}
		if err := os.WriteFile(runDir+"/task", []byte(next+"\n"), 0o666); err != nil {
			return die("cannot write %s/task, so --retask cannot claim task %s", runDir, next)
		}
		retaskNote = "NOTE — this executor claimed task " + next + " itself with `done-invoker --retask`,\n" +
			"because task " + seq + " had already reported and the counter had not moved. That means\n" +
			"the re-task did not go through `hw next`. If you did re-task it, this report is\n" +
			"task " + next + " and everything is consistent again. If you did NOT, this is a\n" +
			"duplicate report for task " + seq + " — say so and ignore it. Re-task with\n" +
			"`hw next <pane> \"…\"` and the counter moves on its own.\n\n"
	}

	stateDir := c.stateDir()
	if err := os.MkdirAll(stateDir, 0o777); err != nil {
		return &Die{Code: 1}
	}
	if isRegular(stateDir+"/done") && !isRegular(stateDir+"/reopened") {
		return die("already notified the brainer for this task. If your brainer re-tasked you AFTER that report and this is the new task's completion, the task counter never moved — only `hw next` advances it, and your brainer used something else. Claim it yourself and report: done-invoker --retask \"<same paragraph>\". Otherwise this is a second completion for one task: that is a message for %s, not a report.", c.operator())
	}

	// -- a queued ruling is delivered before the task can close
	reportingMarker = stateDir + "/reporting"
	body := itoa(c.Pid) + "\n"
	if st := c.procStart(itoa(c.Pid)); st != "" {
		body += st + "\n"
	}
	if err := os.WriteFile(reportingMarker, []byte(body), 0o666); err != nil {
		return die("could not write %s, so a ruling queued now could not be told this task is closing. Nothing was reported; retry.", reportingMarker)
	}
	rulingRun := c.runDir()
	if st, err := os.Stat(c.BinDir + "/state-witness.sh"); err != nil || st.IsDir() {
		return die("cannot find state-witness.sh beside %s, so queued rulings cannot be checked before this task closes. Nothing was reported; this brain/bin is incomplete.", c.BinDir)
	}
	var claimed bytes.Buffer
	crc := c.run(runOpts{stdout: &claimed, stderr: c.Stderr}, "bash", "-c",
		`set -euo pipefail
. "$1/state-witness.sh"
ruling_queue_claim_all "$2" done-invoker`, "_", c.BinDir, rulingRun)
	if crc == 0 { // 203-M01
		text := strings.TrimRight(claimed.String(), "\n")
		msg := "NOT REPORTED — RULING FROM YOUR BRAINER, queued with `hw ruling` while you were working. It changes the premise of your work, so your report was held and NOTHING was sent. Read it, act on it — and if it contradicts what you have already done, say so rather than quietly redoing it — then run done-invoker again.\n\n" + text
		c.printf(c.Stdout, "%s\n", msg)
		c.errf("done-invoker: %s\n", msg)
		return &Die{Code: exitHeldForRuling}
	}

	// -- the close precondition, checked before anything is reported
	chainingLease, legacyKeep, closeOwed := false, false, false
	switch {
	case c.or("HW_CHAINING_ENABLED", "0") == "1" && c.get("HW_CHAINING_LEASE_SECONDS") != "":
		chainingLease = true
	case c.or("HW_CHAINING_ENABLED", "0") == "1" || c.or("HW_DONE_KEEP_PANE", "0") == "1":
		legacyKeep = true
	default:
		closeOwed = true
	}
	hwBin := c.BinDir + "/hw"
	if st, err := os.Stat(hwBin); (closeOwed || chainingLease) && (err != nil || st.IsDir() || st.Mode()&0o111 == 0) {
		return die("REFUSING BEFORE REPORTING — nothing was sent, nothing was marked, this pane still holds the only copy of your report. A delivered report must either close its owned task tab/workspace or establish its bounded chaining lease, and there is no executable `hw` beside this invoker (%s). Fix the install so brain/bin is complete, then re-run this exact command.", hwBin)
	}

	files := countFiles(artifacts)
	filesNote := ""
	if files == 0 {
		filesNote = " (none, which is correct when the output is a report)"
	}

	engramLabel := c.get("ENGRAM_PROJECT")
	if engramLabel == "" {
		engramLabel = c.laneEngram(project)
	}
	if engramLabel == "" {
		engramLabel = project
	}

	var head, tail, sound string
	nfiles := itoa(files)
	switch {
	case status == "done":
		head = "[executor finished — " + project + ":" + task + ", pane " + herdrPane + "]"
		tail = "Report: in engram under project `" + engramLabel + "` — search it, or\n" +
			"mem_get_observation the ids named above.\n" +
			"Artifacts: " + nfiles + " file(s) in " + artifacts + filesNote + checksNote + "\n\n" +
			"Distil what is decision-grade into decisions.md; the raw report stays in engram.\n" +
			"After the receiver admitted the report, done-invoker exits 0 and then, in a process of\n" +
			"its own, closes the hw-owned task tab or workspace through `hw done`. If the brainer launched this run with\n" +
			"`hw ... --keep-pane [duration] --why ...`, a bounded chaining lease keeps the pane\n" +
			"for `hw next` and expiry closes it automatically.\n" +
			"A failed close leaves the pane open and a `close-failed` file (and `close.log`) beside the\n" +
			"run's done marker — this report is delivered and safe and only the close failed, NOT lost."
		sound = "done"
	case chainingLease || legacyKeep:
		kept := "the legacy keep-pane contract, and nothing expires it"
		if chainingLease {
			kept = "its bounded chaining lease, which closes it when it expires"
		}
		head = "[executor BLOCKED — " + project + ":" + task + ", pane " + herdrPane + "]"
		tail = "Partial output: " + nfiles + " file(s) in " + artifacts + "\n\n" +
			"It stopped rather than working around the blocker, which is correct. Decide\n" +
			"whether this is yours to unblock or " + c.operator() + "'s.\n" +
			"The pane is kept by " + kept + ". It is NOT marked waiting for a ruling, so\n" +
			"`hw ruling` will not resume it: re-task it with `hw next`, or close it with\n" +
			"`hw done " + project + " " + task + " --blocked`."
		sound = "request"
	default:
		head = "[executor BLOCKED and waiting — " + project + ":" + task + ", pane " + herdrPane + "]"
		tail = "Partial output: " + nfiles + " file(s) in " + artifacts + "\n\n" +
			"It stopped rather than working around the blocker, which is correct. Decide\n" +
			"whether this is yours to unblock or " + c.operator() + "'s.\n" +
			"The pane and its session are KEPT, waiting: `hw ruling " + herdrPane + " \"<the unblock>\"`\n" +
			"resumes THIS task in the same session, and it reports again with its own\n" +
			"done-invoker. `hw done " + project + " " + task + " --blocked` closes it now; unanswered, `hw reap`\n" +
			"closes it after " + c.or("HW_BLOCKED_WAIT_HOURS", "24") + "h (HW_BLOCKED_WAIT_HOURS)."
		sound = "request"
	}

	envelope := c.envelopePrefix() + ":done"
	if b, err := os.ReadFile(stateDir + "/blocked-resumed"); err == nil {
		if n := bytes.Count(b, []byte("\n")); n > 0 {
			envelope += ":r" + itoa(n)
		}
	}
	msg := head + "\nEnvelope: " + envelope + ". Process this envelope even if your inbox coalesced it\nwith other messages.\n\n" +
		retaskNote + summary + "\n\n" + tail

	publishDone := func(state string) (int, error) {
		tokens, err := tokensJSON(append([]pair{
			{"hw_project", str(project)}, {"hw_task", str(task)}, {"hw_run", str(c.or("HW_RUN", "norun"))},
			{"hw_invoker", str(pane)}, {"done_status", str(status)}, {"done_state", str(state)},
			{"done_at", str(c.stamp())}, {"artifacts", str(nfiles)}, {"engram", str(engramLabel)},
		}, chunkTokens("sum", doneChunks, summary)...))
		if err != nil {
			return 0, err
		}
		rc, err := c.publish(doneTTLMs, tokens,
			`{"idle":"`+status+`: `+task+`","working":"`+status+`: `+task+`","blocked":"`+status+`: `+task+`"}`)
		c.cockpitKick()
		return rc, err
	}
	pubRC, err := publishDone("undelivered")
	if err != nil {
		return err
	}
	if pubRC != 0 {
		if pubRC == 3 {
			return die("cannot reach herdr on $HERDR_SOCKET_PATH, so neither the record nor the message could be delivered. Nothing was marked; retry, or tell %s in this pane that %s:%s ended %s.", c.operator(), project, task, status)
		}
		c.errf("done-invoker: warning: completion metadata is unverified (herdr-rpc exit %d); continuing to the brainer without a verified record.\n", pubRC)
	}
	pubNote := publicationNote(pubRC)

	c.quiet("herdr", "notification", "show", "executor "+status+" · "+project+":"+task,
		"--body", nfiles+" file(s) in "+filepath.Base(artifacts)+" — notifying brainer in "+pane, "--sound", sound)

	route := c.route()
	if err := c.outboxPut(envelope, msg, status); err != nil {
		c.errf("done-invoker: warning: could not write the outbox copy under %s; the report is only in the message and the tokens.\n", c.outboxDir())
	}
	rc := c.deliver(msg, stateDir)
	if rc == 4 && route != "herdr" {
		c.errf("done-invoker: the brainer RECEIVED this (it is in its session) but the turn it started did not complete — see the message above. Counting it as delivered, because it was.\n")
		rc = 0
	}
	if rc == 5 {
		c.deliveryUncertain(envelope, route)
		return &Die{Code: exitDeliveryUncertain, Msg: "delivery UNCERTAIN via " + route + " (channel-send exit 5): envelope " + envelope + " was admitted but its landing is unconfirmed, so it MAY already be in the brainer's session. Do NOT re-run done-invoker — a retry may be a second report for one task. Read the brainer's pane " + pane + " first; if the envelope is there, run `hw done " + project + " " + task + " --force`. The done marker was not written and the outbox copy is held back from automatic redelivery (recorded in " + stateDir + "/delivery-uncertain). " + pubNote}
	}
	if rc != 0 && route != "herdr" {
		return die("%s delivery proof failed (exit %d). %s The done marker was not written.", route, rc, pubNote)
	}
	if rc == 3 {
		return die("could not wait on %s: either herdr is unreachable on $HERDR_SOCKET_PATH or that pane no longer exists — herdr answers both the same way. %s The brainer was not told.%s Tell %s in this pane.", pane, pubNote, c.paneSourceNote(), c.operator())
	}
	if rc == 4 {
		return die("the brainer's pane %s no longer exists, so no message was sent. %s The done marker was not written. Correct the invoker target, then retry.", pane, pubNote)
	}
	if rc == 2 && route == "herdr" {
		return die("the brainer at %s was still working after %ds, so it was NOT told. A prompt sent mid-turn is not dropped — it would be acted on inside the turn the brainer is already in, against context that has nothing to do with your report — which is what this gate prevents. %s Re-run the same command to reach the brainer, or give it a longer budget with a matching Bash timeout: HW_INVOKER_WAIT_MS=540000 done-invoker …", pane, c.waitMs()/1000, pubNote)
	}
	if rc != 0 {
		return die("Herdr delivery failed (exit %d). %s The done marker was not written.", rc, pubNote)
	}

	var receiptNote string
	if route == "herdr" {
		receiptNote = "receiver ADMITTED envelope " + envelope + " (admission only: the prompt was accepted by the brainer's pane, not proof it was read)"
	} else {
		receiptNote = "receiver proved delivery of envelope " + envelope
	}

	// -- account only after the receiver admitted it
	os.WriteFile(stateDir+"/done", nil, 0o666)
	c.outboxSettle(envelope)
	os.Remove(stateDir + "/delivery-uncertain")
	os.Remove(stateDir + "/reopened")
	c.transcriptAppend(stateDir, "REPORT", envelope, summary)
	publishDone("delivered")

	// -- close only after receiver-owned delivery proof
	closeNote := ""
	closeAfter := func(hwArgs ...string) int {
		vcmd := ""
		pin := c.runDir() + "/verify"
		if st, err := os.Stat(pin); status != "blocked" && err == nil && st.Size() > 0 {
			if b, err := os.ReadFile(pin); err == nil {
				vcmd = strings.SplitN(string(b), "\n", 2)[0]
			}
		}
		if c.closeAfterExit(stateDir, hwArgs...) { // 899-M06
			verify := ""
			if vcmd != "" {
				verify = ", after verifying: " + vcmd + " (up to " + c.or("HW_VERIFY_TIMEOUT_SECONDS", "900") + "s)"
			}
			closeNote = "the owned pane closes right after this command exits" + verify + "; the outcome is in close.log beside the done marker, and close-failed there says if it failed"
			return 0
		}
		if vcmd != "" {
			c.printf(c.Stdout, "reported — verifying before close: %s (up to %ss)\n", vcmd, c.or("HW_VERIFY_TIMEOUT_SECONDS", "900"))
		}
		return c.run(runOpts{stdin: c.Stdin, stdout: c.Stdout, stderr: c.Stderr}, hwBin, hwArgs...)
	}
	if chainingLease {
		lrc := c.run(runOpts{stdin: c.Stdin, stdout: c.Stdout, stderr: c.Stderr}, hwBin, "chaining-lease-start")
		if lrc != 0 {
			c.errf("done-invoker: THE REPORT IS DELIVERED AND SAFE, but bounded chaining lease setup failed (exit %d). Refusing silent unbounded retention; attempting the normal owner-aware close now.\n", lrc)
			doneArgs := []string{"done", project, task}
			if status == "blocked" {
				doneArgs = append(doneArgs, "--blocked")
			}
			if closeAfter(doneArgs...) != 0 {
				return notClosed("THE REPORT IS DELIVERED AND SAFE — bounded chaining lease setup failed and the fallback close also failed. The done marker exists, but no retention authority was created. Do NOT re-run done-invoker; close this pane with `hw done %s %s`.", project, task)
			}
			c.printf(c.Stdout, "brainer notified; chaining lease setup failed, so hw closes the owned pane instead of retaining it without a bound (%s).\n", closeNote)
			return nil
		}
		c.printf(c.Stdout, "brainer notified via %s (%s, %s file(s)); %s; bounded chaining lease recorded and scheduled.\n", route, status, nfiles, receiptNote)
		return nil
	}
	if legacyKeep {
		c.printf(c.Stdout, "brainer notified via %s (%s, %s file(s)); %s; pane kept because chaining is enabled for compatibility with an already-running legacy dispatch.\n", route, status, nfiles, receiptNote)
		return nil
	}

	if status == "blocked" {
		waiting := "since=" + itoa(int(c.Now().Unix())) + "\n" +
			"pane=" + herdrPane + "\nproject=" + project + "\ntask=" + task + "\nenvelope=" + envelope + "\n"
		if err := os.WriteFile(stateDir+"/blocked-waiting", []byte(waiting), 0o666); err != nil {
			return notClosed("THE REPORT IS DELIVERED AND SAFE — %s — but %s/blocked-waiting could not be written, so `hw ruling` cannot resume this task and nothing will expire it. The pane is still open at %s. Do NOT re-run done-invoker.", receiptNote, stateDir, herdrPane)
		}
		c.cockpitKick()
		c.printf(c.Stdout, "brainer notified via %s (blocked, %s file(s)); %s; pane KEPT, waiting: `hw ruling %s \"<the unblock>\"` resumes this task, `hw reap` closes it after %sh unanswered.\n",
			route, nfiles, receiptNote, herdrPane, c.or("HW_BLOCKED_WAIT_HOURS", "24"))
		return nil
	}

	if closeAfter("done", project, task) != 0 {
		return notClosed("THE REPORT IS DELIVERED AND SAFE — %s, and this run is marked done. What failed is the CLOSE: `hw done %s %s` could not close the owned task tab/workspace, so the pane is still open at %s (workdir %s). Do NOT re-run done-invoker — the report landed, and a second one is a second report for one task. Close it by hand with `hw done %s %s` once the cause above is fixed. (Exit %d means delivered-but-not-closed; exit 1 would have meant nothing was reported.)",
			receiptNote, project, task, herdrPane, workdir, project, task, exitDeliveredNotClosed)
	}
	if closeNote != "" {
		c.printf(c.Stdout, "brainer notified via %s (%s, %s file(s)); %s; %s.\n", route, status, nfiles, receiptNote, closeNote)
		return nil
	}
	c.printf(c.Stdout, "brainer notified via %s (%s, %s file(s)); %s; hw done returned without an owned task pane to close.\n", route, status, nfiles, receiptNote)
	return nil
}

// countFiles is `find <root> -type f -not -path '*/.hw/*' | wc -l`.
func countFiles(root string) int {
	n := 0
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() && !strings.Contains(p, "/.hw/") {
			n++
		}
		return nil
	})
	return n
}
