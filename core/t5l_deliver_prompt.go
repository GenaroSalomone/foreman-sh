package main

// Area C — the text of the executor's prompt (bin/hw: _brief_preamble, _brief_epistemic, _brief_authorization,
// _gentle_preamble, _gentle_rdd_bullet, _rdd_only_review_line, _speckit_phase_note, _judgment_day_note,
// _design_judgment_note, _rdd_closing_note, _checks_note, _specialist_agents, _brief_qa_override, _brief_fm_value).
// The prompt is the executor's whole contract and its sha256 is receipted: every byte is bash's.

import (
	"fmt"
	"os"
	"strings"
)

// pjoin is `$(printf '%s\n\n%s' "$a" "$b")`.
func pjoin(a, b string) string { return bashSub(a + "\n\n" + b) }

// briefFmValue is _brief_fm_value: the frontmatter value of one key, "" when there is none.
func briefFmValue(key, path string) string {
	if !isFile(path) {
		return ""
	}
	out, _ := brExec("", "awk", "-v", "want="+key, `NR==1 { if ($0 != "---") exit; next }
       $0 == "---" { exit }
       tolower($0) ~ "^[[:space:]]*" want "[[:space:]]*:" {
         sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]+$/, "");
         if ($0 ~ /^".*"$/ || $0 ~ /^'.*'$/) $0 = substr($0, 2, length($0) - 2);
         print; exit }`, path)
	return bashSub(out)
}

// judgmentDayApplies is _judgment_day_applies: a base was computed and the task is a build.
func (l *lch) judgmentDayApplies() bool {
	if l.env("HW_REVIEW_BASE_REF") == "" { // # MUTATION-ANCHOR: 847-C-01
		return false
	}
	switch l.briefKindForClose() {
	case "", "build":
		return true // # MUTATION-ANCHOR: 847-C-02
	}
	return false
}

// rddIsTheReview is _rdd_is_the_review.
func (l *lch) rddIsTheReview() bool { return l.gentleReview == "rdd" } // # MUTATION-ANCHOR: 847-C-03

const checksNote = `Before working, write $HW_ARTIFACTS/checks.md from the brief's ## Proof and
## Verification sections: a table, one row per requirement,

    requirement | verdict | evidence | correction

verdict is pass, fail or unresolved; evidence is a command plus its result, or
a file:line. Fill it in as you go. done-invoker refuses a done report when the
file is missing, has no rows, or has a row with an empty verdict.`

const behaviourFixNote = `A behaviour fix ships the test that fails on the old code. Prove it: revert
the fix, watch that test go red, restore it, watch it go green, and say so in
the commit. An existing expectation that asserts the old behaviour is the
defect written a second time: change it. The one exception is a refactor — it
changes no expectation, and a refactor that does has broken something.

Where the repo carries setup/test-hw, its fast gate (HW_TEST_GATE=fast bash
setup/test-hw) runs the fast subjects AND the slow ones that name a path your
branch changed against main (capped, the cut ones printed). It is still not the
full suite and caches no full verdict, but a red it prints is yours to fix now,
not after the merge.`

const judgmentDayNote = `Closing this code task: Judgment Day, then done-invoker, in this order:

1. Commit per work unit as you go, on this branch, with Conventional Commit
   messages — not one commit at the end. The review target is what you
   committed; uncommitted work is not judged and is not delivered.
2. Before done-invoker, run the judgment-day skill over
   "$(hw review-range)"..HEAD — the branch point computed NOW, so a rebase
   onto the base since dispatch does not pull other people's commits into the
   target. If that range is empty there is nothing to judge: say so in the
   report instead. Record the judged range (base SHA..head SHA).
   Judgment Day is optional. If the judgment-day skill is not among your
   available skills, it is not installed: there is nothing to judge with and
   nothing ESCALATED. Say "Judgment Day: not installed (optional)" in the
   report and close normally. ESCALATED means judges ran and could not converge.
   If you rebased after judging, do not re-judge the whole range. Run
   hw review-delta <judged-base> <judged-head>
   Exit 0: your own diff is the one judged, only moved — the verdict stands.
   Exit 3: it prints the files whose own diff changed; re-judge ONLY those, as
   a scoped re-judgment (it does not reset the fix-round budget). Exit 1: it
   cannot tell — judge the full range again.
3. The skill asks before its first fix round. Nobody answers here, and nobody
   has to: the brief that authorized this implementation is that
   authorization. Apply the fixes both judges confirmed without asking. Judges
   that contradict each other, or anything left after round two, is ESCALATED —
   report it, do not ask and do not wait.
4. The done-invoker report carries the verdict, APPROVED or ESCALATED (or
   "not installed (optional)", above), and,
   separately, every severe finding only ONE judge raised (unconfirmed, so not
   fixed), each with its file:line.`

const rddClosingNote = `Closing this code task: native RDD is its only review, then done-invoker, in this order:

1. Commit per work unit as you go, on this branch, with Conventional Commit
   messages — not one commit at the end. The review target is what you
   committed; uncommitted work is not reviewed and is not delivered.
2. Run the native RDD review of the RDD bullet above, on the clone, and act on
   what it returns. That is the one review of this task: do NOT run the
   judgment-day skill, hw review-range or any second reviewer over the same
   candidate. Two verdicts on one candidate is the cost this mode removes.
3. The done-invoker report carries the RDD outcome (approved, or what
   correction_required left open) and no Judgment Day verdict.`

// designJudgmentNote is _design_judgment_note "$boundary".
func (l *lch) designJudgmentNote(boundary string) string {
	end := "the code-mode Judgment Day"
	if l.rddIsTheReview() {
		end = "the native RDD review"
	}
	return fmt.Sprintf(`Judge the design before the code. This brief declares a boundary:

    boundary: %s

Before your first code commit, write your design to one file under
$HW_ARTIFACTS — the boundary, the mechanism, the input space it decides over,
and what an input nobody anticipated gets — and run the judgment-day skill in
DESIGN MODE over it (references/design-mode.md). Then write
$HW_ARTIFACTS/design-judgment.md with these lines:

    boundary: <the line above, exactly>
    design: <the design file>
    design_sha256: <its sha256>
    JUDGMENT: APPROVED            (or ESCALATED)

An APPROVED verdict lists, one amendment: line each, the bypass classes both
judges confirmed and the approach can close; your code closes them, and the
%s at the end is where that is checked. done-invoker
refuses a done report without an APPROVED verdict whose design still hashes to
design_sha256. An ESCALATED design is a blocker, not a
prompt to code anyway: report it with done-invoker --blocked, naming the
bypass classes the judges found. Two rejected designs mean the approach is
your brainer's call.`, boundary, end)
}

// speckitPhaseNote is _speckit_phase_note.
func (l *lch) speckitPhaseNote() string {
	return `A SPEC KIT PHASE IS NOT THE TASK. Finishing /speckit-specify — or plan, tasks,
or any other speckit-* phase — is not finishing this task: go straight on to the
next phase, through implementation, and close with done-invoker only when the
flow and the brief are both done. A [NEEDS CLARIFICATION] marker is yours to
resolve from the brief and the code: decide, write the decision and its reason
into the spec, and continue. Do not wait for a human for it. Only a decision
that is genuinely ` + l.brOperator() + `'s — money, a third party, an irreversible or outward
action — goes through ask-invoker, and the task stays open while it waits.

BETWEEN /speckit-tasks AND /speckit-implement, A FRESH /speckit-analyze. Run it
in a new subagent that sees only spec.md, plan.md, tasks.md and the project
constitution — not this conversation, not your reasoning. Its report is
read-only. A CRITICAL finding is corrected once, in the artifact it names, and
then you go on to /speckit-implement without re-running it. Anything below
CRITICAL does not stop the flow.`
}

// specialistAgents is _specialist_agents: the specialist subagents on disk in the work directory.
func (l *lch) specialistAgents(dir string) string {
	if st, err := os.Stat(dir + "/.claude/agents"); err != nil || !st.IsDir() {
		return ""
	}
	out, _ := l.sh(cmdSpec{name: "fd", args: []string{"-t", "f", "-t", "l", "-e", "md", ".", dir + "/.claude/agents"}, quiet: true})
	var names []string
	for _, ln := range strings.Split(out, "\n") {
		if ln == "" {
			continue
		}
		if i := strings.LastIndexByte(ln, '/'); i >= 0 { // sd '.*/' ''
			ln = ln[i+1:]
		}
		ln = strings.TrimSuffix(ln, ".md") // sd '\.md$' ''
		switch {                           // rg -v '^(sdd-|speckit-|review-|jd-)'
		case strings.HasPrefix(ln, "sdd-"), strings.HasPrefix(ln, "speckit-"), strings.HasPrefix(ln, "review-"), strings.HasPrefix(ln, "jd-"):
			continue
		}
		names = append(names, ln)
	}
	if len(names) > 1 {
		names = sortLocale(names, false)
	} else if len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Join(names, " "), " ")
}

// briefQaOverride is _brief_qa_override: what, if anything, in this brief asks for a browser or a deployed check.
func (l *lch) briefQaOverride() string {
	f := l.brief
	if !isFile(f) {
		return ""
	}
	why := ""
	req := strings.ReplaceAll(briefFmValue("requires", f), " ", "")
	if strings.Contains(","+req+",", ",browser,") || strings.Contains(","+req+",", ",claude-in-chrome,") {
		why = "requires: " + req
	}
	dc := briefFmValue("deployed_check", f)
	if why == "" {
		switch {
		case dc == "required":
			why = "deployed_check: required"
		case strings.HasPrefix(dc, "out-of-scope"):
		case dc == "":
			if l.sdd == "gentle" || l.env("HW_GENTLE_PANE") == "1" {
				why = "a deployed check (the --sdd gentle default, deployed_check: required)" // # MUTATION-ANCHOR: 847-C-04
			}
		}
	}
	return why
}

// briefPreamble is _brief_preamble.
func (l *lch) briefPreamble() string {
	binDir := l.binDir()
	brief := l.brief
	if brief == "" {
		brief = "<no brief file>"
	}
	label := orDefault2(l.env("ENGRAM_PROJECT"), "brain")
	specialists := l.specialistAgents(l.workdir())
	preamble := bashSub(fmt.Sprintf(`Close with done-invoker. Finishing the work is not reporting it, and neither is
answering in your own pane — your brainer cannot see this pane and will not poll
it. Until done-invoker runs, this task reads as open to everyone else.

This pane is unattended: no human reads it, and an interactive question tool has
no answerer here — where such a tool exists at all, its answer never arrives.
ask-invoker is the only human channel. A missing tool or runtime limitation is a
fact to report with done-invoker --blocked, never a reason to end the turn silently.

done-invoker and ask-invoker are shell commands in your PATH, run in a terminal.
They are not tools, and no tool of those names exists. Run them the way you run
any other command:

    done-invoker "<one paragraph: what you established, and the engram ids>"
    done-invoker --blocked "<what stopped you, and what would unblock it>"  # keeps this pane
    ask-invoker  "<one question, at most 600 characters>"   # 3 per task; your brainer answers
    ask-invoker --challenge "<what the brief forbids> :: <what you observed>"

If the brief forbids something you have evidence is wrong, challenge it instead
of stopping — obey-or-stop are not your only two moves.
A challenge does not spend an ask; it has its own budget. It needs the evidence,
not the opinion. Until your brainer rules, hold.
A --blocked report does not close you: wait. A ruling that answers it resumes
THIS task — act on it, then report again with done-invoker (no --retask).

If your shell cannot find them, they are at %s/done-invoker and
%s/ask-invoker — run them by absolute path rather than concluding they do not
exist.

Your brief is also on disk at %s. If anything above arrived truncated or you
lost it, read that file rather than asking for it again.

If it ever answers "already notified the brainer for this task" and you were
re-tasked after that report, the task counter did not move — add --retask and
report anyway. A finished task that cannot be reported is the failure; a report
your brainer has to arbitrate is not.

Save to engram with project: %s — your engram server is the label proxy hw
launched: omit `+"`project`"+` or pass exactly that; any other value is refused, and
every mem_search already reads all labels (all_projects is forced).`, binDir, binDir, brief, label))
	if s := l.hwEngramSession; s != "" {
		preamble = bashSub(preamble + "\n\n" + fmt.Sprintf("Your engram session is %s, registered by hw under %s for this work\ndirectory. Pass session_id: %s with project: %s on every mem_save; without\nit, engram may file the save into manual-save-%s, which it can refuse.\ndone-invoker checks it: a done report must name at least one observation as\n#<id>, and that id must exist under %s, or the report is refused.",
			s, label, s, label, label, label))
	}
	if specialists != "" {
		preamble = bashSub(preamble + "\n\n" + fmt.Sprintf("Specialist subagents available here: %s. If one of them is the job, delegate\nrather than doing it inline — they carry the access and the conventions for it,\nand reconstructing those in your own session is how a mechanism gets improvised.", specialists))
	} else {
		preamble = pjoin(preamble, "no specialists in this lane; ops-investigator / qa-tester are not available here") // # MUTATION-ANCHOR: 847-C-05
	}
	vendor := l.env("HW_EXECUTOR_VENDOR")
	if vendor == "" {
		vendor = l.agent
	}
	// DELEGATE BY MODEL TIER, ON EVERY VENDOR: the explorer's exact name and which job goes to which tier.
	if note := l.explorer().tierNote(vendor); note != "" {
		preamble = pjoin(preamble, note)
	}
	if vendor == "codex" { // # MUTATION-ANCHOR: 847-C-06
		preamble = pjoin(preamble, `RULES CODEX DOES NOT LOAD (shared rules, one line each):
- Secrets: never print, copy into a new file, paste or commit .env, *.db, *.credentials or secrets/; report key names, never values.
- A brief names a Keychain handle for a credential; no handle is a gap, report it with done-invoker --blocked.
- Browser: close it with `+binDir+`/browser-close, and never retry a failed launch (a failed launch is a blocked report).
- Artifacts (audio, exports, captures, generated files) go to $HW_ARTIFACTS, never into git status.`)
	}
	briefNote := ""
	if l.proj != "" {
		briefNote = bashSub(l.c.t.get(l.proj, "brief_note"))
	}
	if briefNote != "" { // # MUTATION-ANCHOR: 847-C-07
		preamble = pjoin(preamble, briefNote)
		if qa := l.briefQaOverride(); qa != "" {
			preamble = bashSub(preamble + "\n\n" + "THIS BRIEF OVERRIDES THE LANE NOTE ABOVE ON BROWSERS AND QA: it declares " + qa + ". What the brief asks you to verify in a browser or against what is deployed IS your task, not the brainer's; the sentence about staging-only QA describes the lane default and does not apply to it.")
		}
	}
	if l.proj != "" && l.c.t.get(l.proj, "qa_target") != "" && l.env("HW_PORT_WEB") != "" { // # MUTATION-ANCHOR: 847-C-08
		preamble = bashSub(preamble + "\n\n" + "QA target: http://localhost:" + l.env("HW_PORT_WEB") + " — pass it as `pnpm qa <flow> --target <url>`, to qa-tester in its prompt, and to `harness open <url>`. Never staging, never :3001, unless the brief says so.")
	}
	if l.judgmentDayApplies() && !l.rddIsTheReview() { // # MUTATION-ANCHOR: 847-C-09
		branch := orDefault2(l.env("HW_REVIEW_BASE_BRANCH"), "<unnamed>")
		baseNote := bashSub(`The base a Judgment Day target is built against, computed by hw at dispatch:

    HW_REVIEW_BASE_REF=` + l.env("HW_REVIEW_BASE_REF") + `
    HW_REVIEW_BASE_BRANCH=` + branch + `   (the branch POINT off it — git merge-base, not its tip)

That value is frozen at dispatch. If you have rebased onto the base since, it
now includes commits that are not yours. A Judgment Day target is the branch
point computed at judging time, to HEAD:

    git diff "$(hw review-range)"..HEAD

Check the count before you trust any verdict. The frozen target must hold the
same number of files as

    git diff --name-only "$(hw review-range)"..HEAD | wc -l

` + "`hw review-range`" + ` exits 1 naming the ref when it cannot compute the branch
point; then report that with done-invoker instead of judging a guessed range.

If the two numbers differ, the target is not your work: report that with
done-invoker instead of judging it.`)
		preamble = pjoin(preamble, baseNote)
	}
	switch l.briefKindForClose() {
	case "", "build":
		preamble = pjoin(preamble, behaviourFixNote)
	}
	if l.checksGateApplies() {
		preamble = pjoin(preamble, checksNote)
	}
	if l.designGateApplies() { // # MUTATION-ANCHOR: 847-C-10
		preamble = pjoin(preamble, l.designJudgmentNote(l.briefBoundary()))
	}
	if l.judgmentDayApplies() {
		if l.rddIsTheReview() {
			preamble = pjoin(preamble, rddClosingNote)
		} else {
			preamble = pjoin(preamble, judgmentDayNote)
		}
	}
	return preamble
}

// orDefault2 is ${v:-def}.
func orDefault2(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// briefEpistemic is _brief_epistemic.
func (l *lch) briefEpistemic() string {
	return "BRIEF EPISTEMIC SAFEGUARD — apply this before acting on the brief:\n\nMEASURED: a heading or citation alone is not proof; treat a claim as established\nonly when its named evidence/source supports the exact claim being prescribed. A reader need not independently re-derive already measured work merely because it is prescribed.\n\nUNESTABLISHED: keep open causal or design questions visibly open; do not turn\nthem into facts.\n\nCONSTRAINTS: preserve the stated boundaries, non-regressions and verification\nrequirements.\n\nA prescribed solution is valid only when it rests on MEASURED evidence. If a\nproposed remedy rests on an UNESTABLISHED cause, surface that contradiction and\ndo not ratify the remedy. Legitimate measured prescriptions remain permitted:\npreserve and execute them when their evidence supports them."
}

// briefAuthorization is _brief_authorization: the authorization the brief cites, quoted, or "".
func (l *lch) briefAuthorization() string {
	if l.brAuthorizes == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("AUTHORIZATION — delivered by hw from the brief frontmatter (authorizes:), quoted:\n\n")
	sb.WriteString("> requested_by: " + orDefault2(l.requestedBy, "<none>") + "\n")
	// printf '%s\n' "$AUTHORIZES" | awk -F ' :: ': one line per record, the first three fields
	for _, rec := range strings.Split(l.brAuthorizes, "\n") {
		f := strings.Split(rec, " :: ")
		for len(f) < 3 {
			f = append(f, "")
		}
		sb.WriteString("> authorized: destination: " + f[0] + " | operation: " + f[1] + " | credential handle: " + f[2] + "\n")
	}
	sb.WriteString("\nOnly these destinations and operations are authorized, and nothing beyond them:\nanother destination, another operation, or a credential or session not named\nhere stays unauthorized. Within them, act without asking; for anything else\nask-invoker, or report blocked. A handle is a resolver, never a value.")
	return bashSub(sb.String())
}

// rddOnlyReviewLine is _rdd_only_review_line.
func rddOnlyReviewLine() string {
	return "NATIVE RDD IS THE ONLY REVIEW OF THIS TASK. Do NOT run the judgment-day" +
		"\n     skill or any second reviewer over the same candidate, and do not report a" +
		"\n     Judgment Day verdict: two reviews of one candidate are two verdicts and" +
		"\n     twice the cost, and this brief chose RDD instead. (A design judgment the brief" +
		"\n     asks for under `boundary:` is a separate gate, before the code, not this.)"
}

// gentleRddBullet is _gentle_rdd_bullet.
func (l *lch) gentleRddBullet() string {
	gh := l.gentleHomeDir()
	base := l.brLaneGet(l.proj, "base_ref_prefix") + l.c.t.get(l.proj, "base")
	return `- RECEIPT-DRIVEN DEVELOPMENT (RDD) IS ON for this task, natively, and only
  this way. NEVER run gentle-ai with --cwd on this worktree or any repository
  of the product: its review store is <git-common-dir>/gentle-ai, inside the
  product repo, and that is forbidden. The binary is ` + gh + `/bin/gentle-ai-task (its
  wrapper fixes its HOME to a private one); never any other, never install,
  sync or upgrade.
  1. When the work is committed, make a clone and review THAT:
     git clone --no-hardlinks <this worktree> $HW_ARTIFACTS/rdd-clone ;
     in the clone, git fetch origin refs/remotes/` + base + `:refs/remotes/` + base + `
     (the review base is the lane's base, ` + base + `, never main). The clone's
     HEAD is the candidate; commit nothing new in the product worktree for it.
  2. ` + gh + `/bin/gentle-ai-task review mode enable --scope global --cwd
     $HW_ARTIFACTS/rdd-home — the task's own copy of gentle-ai's home, made on
     first use: RDD is on for this task only and there is nothing to turn off.
     NEVER enable it through ` + gh + `/bin/gentle-ai (every executor shares that
     home), and never --scope clone to turn it on: a clone setting can only
     turn RDD off.
  3. Drive the lifecycle, always through ` + gh + `/bin/gentle-ai-task, with
     --cwd <the clone> --base-ref ` + base + ` --committed-only: review status
     --contract gentle-ai.review-integration/v2 --agent claude-code
     --next-transition. Run EXACTLY the operation and tokens each transition
     returns, never rebuilt: START first (answer consent with its granted
     invocation), then status again. THE REVIEW IS NATIVE: a returned
     review capture-result carries --agent and no --input, and running it makes
     gentle-ai materialize the immutable context and run its own tool-free
     reviewer in process, then admit the result. NEVER launch a lens or any
     subagent for it, NEVER assemble a reviewer prompt, NEVER add --input,
     NEVER hand candidate bytes over through a file: the context can be over
     100 KB and relaying it by hand was cut by an API safeguard. When one
     collect carries several reviewer slots, start every returned capture
     concurrently in the order returned (one Bash call, backgrounded) and wait
     for all of them. A review can outlast a tool timeout: run them with
     run_in_background (or a long timeout), and a timeout is not a failed
     capture: ask STATUS again before concluding anything. Run the
     acknowledge-approved transition it returns once
     approved. On correction_required, re-enter only through the
     status_continuation it returns.
  4. If a capture fails (provider, budget, lens_context_budget_exceeded, no
     login), report done-invoker --blocked with its exact output. Do not
     relay, do not retry with another command shape.
  5. ` + gh + `/bin/gentle-ai-task writes rdd-log.jsonl in $HW_ARTIFACTS for you,
     locally, from each capture's own answer: role, verdict, findings,
     duration. Telemetry is off; nothing leaves this machine.
  6. ` + rddOnlyReviewLine() + "\n"
}

// gentlePreamble is _gentle_preamble.
func (l *lch) gentlePreamble() string {
	var delivery string
	switch l.gentleDelivery {
	case "single-pr":
		delivery = "single-pr — one pull request; do not slice it into a chain"
	case "stacked-to-main":
		delivery = "a chain of pull requests, stacked-to-main"
	case "feature-branch-chain":
		delivery = "a chain of pull requests, feature-branch-chain"
	}
	var sb strings.Builder
	sb.WriteString("THIS TASK RUNS IN GENTLE-AI ODD MODE (--sdd gentle) — chosen by your brainer.\nThe Organic Driven Development (ODD) protocol is in your system prompt: follow\nit, in its order, for this task. What this mode fixes for you:\n\n")
	if l.gentleReview == "rdd" {
		sb.WriteString(l.gentleRddBullet())
	} else {
		sb.WriteString("- Receipt-driven development (RDD) is OFF for this task. gentle-ai is NOT\n  installed for you and must not be run: no `gentle-ai review ...`, no\n  `gentle-ai install`, no `gentle-ai sync`. Where ODD says \"when RDD is\n  enabled\", it is not. Review is Judgment Day, as this dispatch says below.\n")
	}
	sb.WriteString("- SDD is not selected. Do not enter SDD phases or create SDD artifacts.\n" +
		"- The feature document is `odd/tasks/<feature-name>.md`, as ODD says. `odd/`\n" +
		"  is ignored in this worktree by an untracked odd/.gitignore: never commit it,\n" +
		"  never force-add it, never delete that file. Mirror it to engram as ODD says,\n" +
		"  passing project explicitly.\n" +
		"- NOBODY ANSWERS IN THIS PANE. Every question ODD tells you to ask the user —\n" +
		"  an ambiguous authorization, a real product decision, a delivery or chain\n" +
		"  strategy — goes through `ask-invoker \"<one question>\"`, and you act on its\n" +
		"  reply. Never end your turn waiting for an answer here.\n" +
		"- Delivery strategy is fixed by the brief: " + delivery + ". Do not ask for it.\n")
	dv := orDefault2(l.gentleDeployedCheck, "required")
	if strings.HasPrefix(dv, "out-of-scope:") {
		sb.WriteString("- DEPLOYED CHECK OUT OF SCOPE, declared by the brief (" + strings.TrimPrefix(dv, "out-of-scope: ") + ").\n" +
			"  done-invoker opens your report with that line; do not claim a check against\n" +
			"  what is deployed that you did not run.\n")
	} else {
		sb.WriteString("- THE CHECK AGAINST WHAT IS DEPLOYED IS PART OF DONE, not a pending line in\n" +
			"  the feature document. done-invoker refuses a done until\n" +
			"  $HW_ARTIFACTS/deployed-check.md holds three lines: `target: <the deployed\n" +
			"  https URL you verified>` (never localhost), `evidence: <a file under\n" +
			"  $HW_ARTIFACTS with the run's result>`, and `RESULT: PASSED`.\n" +
			"  If it cannot run — no QA account, no seed, no session — that is a HOLD:\n" +
			"  `ask-invoker` naming exactly what is missing, then wait for the reply. If it\n" +
			"  ran and failed, or the hold cannot be lifted, report `done-invoker --blocked`.\n")
	}
	return bashSub(sb.String())
}
