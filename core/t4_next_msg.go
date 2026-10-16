package main

// The message `hw next` delivers: _next_message (lib/hw/next.sh) around the launch's own blocks, copied from
// bin/hw byte for byte — _brief_preamble with its notes (_judgment_day_note, _rdd_closing_note, _checks_note,
// _design_judgment_note, _brief_qa_override, _specialist_agents), _brief_epistemic, _speckit_phase_note and
// gentle's lead block (_gentle_preamble, _gentle_rdd_bullet, _rdd_only_review_line). They are the dispatch
// path's (train T5), which owns them; these are `hw next`'s copies until T5 dedupes them.

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// nextLaneField is lane_get for the fields the Go lane table does not keep: brief_note, qa_target
// (true -> "1"), base_ref_prefix (default "origin/") and base, read from the table file as
// project-spaces.sh emits them. "" for a lane the table does not know.
func (a *app) nextLaneField(lane, key string) string {
	if !a.c.t.known(lane) {
		return ""
	}
	data, err := os.ReadFile(a.c.t.file)
	if err != nil {
		return ""
	}
	vals, _ := parseStream(data)
	if len(vals) == 0 {
		return ""
	}
	v, ok := nextPath(vals[0], "lanes", lane, key)
	switch key {
	case "qa_target":
		if ok && v.kind == vBool && v.b {
			return "1"
		}
		return ""
	case "base_ref_prefix":
		if !ok || !v.truthy() {
			return "origin/"
		}
	default:
		if !ok || !v.truthy() {
			return ""
		}
	}
	if v.kind == vStr {
		return v.s
	}
	return v.toJSON()
}

// nextMsgEnv is what _next_message's subshell rebuilds from the RUN, never from the brainer's shell.
type nextMsgEnv struct {
	brief, workdir, engram, baseRef, baseBranch, vendor, portWeb, proj, session, rundir string
	gentlePane                                                                          bool
	gentleReview                                                                        string
}

// nextMessage is _next_message: the counter, the entry line, the safeguard, the preamble and the task.
func (a *app) nextMessage(s *nextState, nextSeq, entryLine string) string {
	r := s.rundir
	e := nextMsgEnv{brief: s.brief, workdir: s.wtdir, gentleReview: s.gReview, rundir: r}
	if v, rc := a.nextRunEnv(r, "ENGRAM_PROJECT", false); rc == 0 {
		e.engram = bashSub(v) // # MUTATION-ANCHOR: 862-G17
	} else {
		e.engram = "brain"
	}
	e.baseRef = a.nextEnvVal(r, "HW_REVIEW_BASE_REF")
	e.baseBranch = a.nextEnvVal(r, "HW_REVIEW_BASE_BRANCH")
	e.vendor = a.nextEnvVal(r, "HW_EXECUTOR_VENDOR")
	e.portWeb = a.nextEnvVal(r, "HW_PORT_WEB")
	e.proj = a.nextEnvVal(r, "HW_PROJECT")
	e.gentlePane = nextDispatchFramework(r) == "gentle"
	e.session = nextEngramSession(r + "/receipt.jsonl")
	if strings.HasPrefix(e.session, "none") {
		e.session = ""
	}
	preamble := bashSub(a.nextBriefPreamble(e))
	text := s.text
	if s.auth != "" {
		text = bashSub(s.auth + "\n\n---\n\n" + text) // # MUTATION-ANCHOR: 862-G18
	}
	msg := "NEXT TASK (task " + nextSeq + " of this session — the previous one is closed and reported).\nYour ask budget and your done-invoker are reset for it: report this one with\ndone-invoker when it is finished, exactly as you did the last.\n\n" +
		entryLine + "---\n\n" + nextEpistemic + "\n\n---\n\n" + preamble + "\n\n---\n\n" + text
	return msg
}

// nextEngramSession is `jq -r 'select(.key=="engram_session") | .value' receipt.jsonl | tail -1`.
func nextEngramSession(path string) string { return nextReceiptValue(path, "engram_session") }

// nextReceiptValue is `jq -r 'select(.key==<key>) | .value' receipt.jsonl | tail -1`.
func nextReceiptValue(path, key string) string {
	vals, _ := readStream(path)
	var outs []string
	for _, v := range vals {
		if v.kind != vObj {
			if v.kind == vNull {
				continue
			}
			break // .key of a non-object: jq stops there
		}
		if k, ok := v.get("key"); ok && k.kind == vStr && k.s == key {
			val, ok := v.get("value")
			if !ok {
				outs = append(outs, "null")
				continue
			}
			outs = append(outs, val.raw())
		}
	}
	return bashSub(lastLine(outs))
}

const nextEpistemic = "BRIEF EPISTEMIC SAFEGUARD — apply this before acting on the brief:\n\nMEASURED: a heading or citation alone is not proof; treat a claim as established\nonly when its named evidence/source supports the exact claim being prescribed. A reader need not independently re-derive already measured work merely because it is prescribed.\n\nUNESTABLISHED: keep open causal or design questions visibly open; do not turn\nthem into facts.\n\nCONSTRAINTS: preserve the stated boundaries, non-regressions and verification\nrequirements.\n\nA prescribed solution is valid only when it rests on MEASURED evidence. If a\nproposed remedy rests on an UNESTABLISHED cause, surface that contradiction and\ndo not ratify the remedy. Legitimate measured prescriptions remain permitted:\npreserve and execute them when their evidence supports them."

// nextBriefPreamble is _brief_preamble, for a re-task (TASK_KIND unset; AGENT and SDD as the shell has them).
func (a *app) nextBriefPreamble(e nextMsgEnv) string {
	bin := a.nextBinDir()
	brief := e.brief
	if brief == "" {
		brief = "<no brief file>"
	}
	eng := e.engram
	if eng == "" {
		eng = "brain"
	}
	p := "Close with done-invoker. Finishing the work is not reporting it, and neither is\nanswering in your own pane — your brainer cannot see this pane and will not poll\nit. Until done-invoker runs, this task reads as open to everyone else.\n\nThis pane is unattended: no human reads it, and an interactive question tool has\nno answerer here — where such a tool exists at all, its answer never arrives.\nask-invoker is the only human channel. A missing tool or runtime limitation is a\nfact to report with done-invoker --blocked, never a reason to end the turn silently.\n\ndone-invoker and ask-invoker are shell commands in your PATH, run in a terminal.\nThey are not tools, and no tool of those names exists. Run them the way you run\nany other command:\n\n    done-invoker \"<one paragraph: what you established, and the engram ids>\"\n    done-invoker --blocked \"<what stopped you, and what would unblock it>\"  # keeps this pane\n    ask-invoker  \"<one question, at most 600 characters>\"   # 3 per task; your brainer answers\n    ask-invoker --challenge \"<what the brief forbids> :: <what you observed>\"\n\nIf the brief forbids something you have evidence is wrong, challenge it instead\nof stopping — obey-or-stop are not your only two moves.\nA challenge does not spend an ask; it has its own budget. It needs the evidence,\nnot the opinion. Until your brainer rules, hold.\nA --blocked report does not close you: wait. A ruling that answers it resumes\nTHIS task — act on it, then report again with done-invoker (no --retask).\n\nIf your shell cannot find them, they are at " + bin + "/done-invoker and\n" + bin + "/ask-invoker — run them by absolute path rather than concluding they do not\nexist.\n\nYour brief is also on disk at " + brief + ". If anything above arrived truncated or you\nlost it, read that file rather than asking for it again.\n\nIf it ever answers \"already notified the brainer for this task\" and you were\nre-tasked after that report, the task counter did not move — add --retask and\nreport anyway. A finished task that cannot be reported is the failure; a report\nyour brainer has to arbitrate is not.\n\nSave to engram with project: " + eng + " — your engram server is the label proxy hw\nlaunched: omit `project` or pass exactly that; any other value is refused, and\nevery mem_search already reads all labels (all_projects is forced)."
	p = bashSub(p)
	if e.session != "" {
		p = bashSub(p + "\n\nYour engram session is " + e.session + ", registered by hw under " + eng + " for this work\ndirectory. Pass session_id: " + e.session + " with project: " + eng + " on every mem_save; without\nit, engram may file the save into manual-save-" + eng + ", which it can refuse.\ndone-invoker checks it: a done report must name at least one observation as\n#<id>, and that id must exist under " + eng + ", or the report is refused.")
	}
	if sp := nextSpecialists(e.workdir); sp != "" {
		p = bashSub(p + "\n\nSpecialist subagents available here: " + sp + ". If one of them is the job, delegate\nrather than doing it inline — they carry the access and the conventions for it,\nand reconstructing those in your own session is how a mechanism gets improvised.")
	} else {
		p = bashSub(p + "\n\nno specialists in this lane; ops-investigator / qa-tester are not available here")
	}
	vendor := e.vendor
	if vendor == "" {
		vendor = a.getenv("AGENT")
	}
	// DELEGATE BY MODEL TIER: only when the session being re-tasked was LAUNCHED with the explorer (its receipt says
	// so), so the prompt never names an agent that session does not have.
	if strings.HasPrefix(nextReceiptValue(e.rundir+"/receipt.jsonl", "explorer"), explorerName+" on ") {
		if note := a.explorerFor(e.proj).tierNote(vendor); note != "" {
			p = pjoin(p, note)
		}
	}
	if vendor == "codex" { // # MUTATION-ANCHOR: 862-G19
		p = bashSub(p + "\n\n" + "RULES CODEX DOES NOT LOAD (shared rules, one line each):\n- Secrets: never print, copy into a new file, paste or commit .env, *.db, *.credentials or secrets/; report key names, never values.\n- A brief names a Keychain handle for a credential; no handle is a gap, report it with done-invoker --blocked.\n- Browser: close it with " + bin + "/browser-close, and never retry a failed launch (a failed launch is a blocked report).\n- Artifacts (audio, exports, captures, generated files) go to $HW_ARTIFACTS, never into git status.")
	}
	note := ""
	if e.proj != "" {
		note = bashSub(a.nextLaneField(e.proj, "brief_note"))
	}
	if note != "" {
		p = bashSub(p + "\n\n" + note)
		if qa := a.nextQAOverride(e); qa != "" {
			p = bashSub(p + "\n\nTHIS BRIEF OVERRIDES THE LANE NOTE ABOVE ON BROWSERS AND QA: it declares " + qa + ". What the brief asks you to verify in a browser or against what is deployed IS your task, not the brainer's; the sentence about staging-only QA describes the lane default and does not apply to it.")
		}
	}
	if e.proj != "" && a.nextLaneField(e.proj, "qa_target") != "" && e.portWeb != "" {
		p = bashSub(p + "\n\nQA target: http://localhost:" + e.portWeb + " — pass it as `pnpm qa <flow> --target <url>`, to qa-tester in its prompt, and to `harness open <url>`. Never staging, never :3001, unless the brief says so.")
	}
	jd := e.baseRef != "" && nextIsBuild(e.brief)
	rdd := e.gentleReview == "rdd"
	if jd && !rdd {
		bb := e.baseBranch
		if bb == "" {
			bb = "<unnamed>"
		}
		base := bashSub("The base a Judgment Day target is built against, computed by hw at dispatch:\n\n    HW_REVIEW_BASE_REF=" + e.baseRef + "\n    HW_REVIEW_BASE_BRANCH=" + bb + "   (the branch POINT off it — git merge-base, not its tip)\n\nThat value is frozen at dispatch. If you have rebased onto the base since, it\nnow includes commits that are not yours. A Judgment Day target is the branch\npoint computed at judging time, to HEAD:\n\n    git diff \"$(hw review-range)\"..HEAD\n\nCheck the count before you trust any verdict. The frozen target must hold the\nsame number of files as\n\n    git diff --name-only \"$(hw review-range)\"..HEAD | wc -l\n\n`hw review-range` exits 1 naming the ref when it cannot compute the branch\npoint; then report that with done-invoker instead of judging a guessed range.\n\nIf the two numbers differ, the target is not your work: report that with\ndone-invoker instead of judging it.")
		p = bashSub(p + "\n\n" + base)
	}
	if nextIsBuild(e.brief) {
		p = bashSub(p + "\n\n" + "A behaviour fix ships the test that fails on the old code. Prove it: revert\nthe fix, watch that test go red, restore it, watch it go green, and say so in\nthe commit. An existing expectation that asserts the old behaviour is the\ndefect written a second time: change it. The one exception is a refactor — it\nchanges no expectation, and a refactor that does has broken something.\n\nWhere the repo carries setup/test-hw, its fast gate (HW_TEST_GATE=fast bash\nsetup/test-hw) runs the fast subjects AND the slow ones that name a path your\nbranch changed against main (capped, the cut ones printed). It is still not the\nfull suite and caches no full verdict, but a red it prints is yours to fix now,\nnot after the merge.")
		// _checks_gate_applies: the same kinds
		p = bashSub(p + "\n\n" + "Before working, write $HW_ARTIFACTS/checks.md from the brief's ## Proof and\n## Verification sections: a table, one row per requirement,\n\n    requirement | verdict | evidence | correction\n\nverdict is pass, fail or unresolved; evidence is a command plus its result, or\na file:line. Fill it in as you go. done-invoker refuses a done report when the\nfile is missing, has no rows, or has a row with an empty verdict.")
	}
	if nextDesignGate(e.brief) {
		end := "the code-mode Judgment Day"
		if rdd {
			end = "the native RDD review"
		}
		p = bashSub(p + "\n\n" + bashSub("Judge the design before the code. This brief declares a boundary:\n\n    boundary: "+nextFmValue("boundary", e.brief)+"\n\nBefore your first code commit, write your design to one file under\n$HW_ARTIFACTS — the boundary, the mechanism, the input space it decides over,\nand what an input nobody anticipated gets — and run the judgment-day skill in\nDESIGN MODE over it (references/design-mode.md). Then write\n$HW_ARTIFACTS/design-judgment.md with these lines:\n\n    boundary: <the line above, exactly>\n    design: <the design file>\n    design_sha256: <its sha256>\n    JUDGMENT: APPROVED            (or ESCALATED)\n\nAn APPROVED verdict lists, one amendment: line each, the bypass classes both\njudges confirmed and the approach can close; your code closes them, and the\n"+end+" at the end is where that is checked. done-invoker\nrefuses a done report without an APPROVED verdict whose design still hashes to\ndesign_sha256. An ESCALATED design is a blocker, not a\nprompt to code anyway: report it with done-invoker --blocked, naming the\nbypass classes the judges found. Two rejected designs mean the approach is\nyour brainer's call."))
	}
	if jd {
		if rdd {
			p = bashSub(p + "\n\n" + "Closing this code task: native RDD is its only review, then done-invoker, in this order:\n\n1. Commit per work unit as you go, on this branch, with Conventional Commit\n   messages — not one commit at the end. The review target is what you\n   committed; uncommitted work is not reviewed and is not delivered.\n2. Run the native RDD review of the RDD bullet above, on the clone, and act on\n   what it returns. That is the one review of this task: do NOT run the\n   judgment-day skill, hw review-range or any second reviewer over the same\n   candidate. Two verdicts on one candidate is the cost this mode removes.\n3. The done-invoker report carries the RDD outcome (approved, or what\n   correction_required left open) and no Judgment Day verdict.")
		} else {
			p = bashSub(p + "\n\n" + nextJudgmentDayNote)
		}
	}
	return p
}

const nextJudgmentDayNote = "Closing this code task: Judgment Day, then done-invoker, in this order:\n\n1. Commit per work unit as you go, on this branch, with Conventional Commit\n   messages — not one commit at the end. The review target is what you\n   committed; uncommitted work is not judged and is not delivered.\n2. Before done-invoker, run the judgment-day skill over\n   \"$(hw review-range)\"..HEAD — the branch point computed NOW, so a rebase\n   onto the base since dispatch does not pull other people's commits into the\n   target. If that range is empty there is nothing to judge: say so in the\n   report instead. Record the judged range (base SHA..head SHA).\n   Judgment Day is optional. If the judgment-day skill is not among your\n   available skills, it is not installed: there is nothing to judge with and\n   nothing ESCALATED. Say \"Judgment Day: not installed (optional)\" in the\n   report and close normally. ESCALATED means judges ran and could not converge.\n   If you rebased after judging, do not re-judge the whole range. Run\n   hw review-delta <judged-base> <judged-head>\n   Exit 0: your own diff is the one judged, only moved — the verdict stands.\n   Exit 3: it prints the files whose own diff changed; re-judge ONLY those, as\n   a scoped re-judgment (it does not reset the fix-round budget). Exit 1: it\n   cannot tell — judge the full range again.\n3. The skill asks before its first fix round. Nobody answers here, and nobody\n   has to: the brief that authorized this implementation is that\n   authorization. Apply the fixes both judges confirmed without asking. Judges\n   that contradict each other, or anything left after round two, is ESCALATED —\n   report it, do not ask and do not wait.\n4. The done-invoker report carries the verdict, APPROVED or ESCALATED (or\n   \"not installed (optional)\", above), and,\n   separately, every severe finding only ONE judge raised (unconfirmed, so not\n   fixed), each with its file:line."

// nextQAOverride is _brief_qa_override.
func (a *app) nextQAOverride(e nextMsgEnv) string {
	f := e.brief
	if f == "" || !isFile(f) {
		return ""
	}
	req := strings.ReplaceAll(nextFmValue("requires", f), " ", "")
	why := ""
	if strings.Contains(","+req+",", ",browser,") || strings.Contains(","+req+",", ",claude-in-chrome,") {
		why = "requires: " + req
	}
	if why == "" {
		dc := nextFmValue("deployed_check", f)
		switch {
		case dc == "required":
			why = "deployed_check: required"
		case strings.HasPrefix(dc, "out-of-scope"):
		case dc == "":
			if a.getenv("SDD") == "gentle" || e.gentlePane {
				why = "a deployed check (the --sdd gentle default, deployed_check: required)"
			}
		}
	}
	return why
}

// nextSpecialists is _specialist_agents: the *.md (any case) files and links under <dir>/.claude/agents, not
// hidden, by name without .md, minus the framework's own (sdd-, speckit-, review-, jd-), sorted, one line.
func nextSpecialists(dir string) string {
	root := dir + "/.claude/agents"
	if !isDir(root) {
		return ""
	}
	var names []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") || len(d.Name()) <= 3 {
			return nil
		}
		n := strings.TrimSuffix(d.Name(), ".md")
		for _, pre := range []string{"sdd-", "speckit-", "review-", "jd-"} {
			if strings.HasPrefix(n, pre) {
				return nil
			}
		}
		names = append(names, n)
		return nil
	})
	sort.Strings(names)
	return strings.Join(names, " ")
}

// ── Spec Kit and gentle ─────────────────────────────────────────────────────

// nextSpeckitNote is _speckit_phase_note.
func (a *app) nextSpeckitNote() string {
	return "A SPEC KIT PHASE IS NOT THE TASK. Finishing /speckit-specify — or plan, tasks,\nor any other speckit-* phase — is not finishing this task: go straight on to the\nnext phase, through implementation, and close with done-invoker only when the\nflow and the brief are both done. A [NEEDS CLARIFICATION] marker is yours to\nresolve from the brief and the code: decide, write the decision and its reason\ninto the spec, and continue. Do not wait for a human for it. Only a decision\nthat is genuinely " + a.nextOperator() + "'s — money, a third party, an irreversible or outward\naction — goes through ask-invoker, and the task stays open while it waits.\n\nBETWEEN /speckit-tasks AND /speckit-implement, A FRESH /speckit-analyze. Run it\nin a new subagent that sees only spec.md, plan.md, tasks.md and the project\nconstitution — not this conversation, not your reasoning. Its report is\nread-only. A CRITICAL finding is corrected once, in the artifact it names, and\nthen you go on to /speckit-implement without re-running it. Anything below\nCRITICAL does not stop the flow."
}

// nextGentlePreamble is _gentle_preamble, for the run's lane (PROJ = the run's HW_PROJECT).
func (a *app) nextGentlePreamble(s *nextState, proj string) string {
	var b strings.Builder
	b.WriteString("THIS TASK RUNS IN GENTLE-AI ODD MODE (--sdd gentle) — chosen by your brainer.\nThe Organic Driven Development (ODD) protocol is in your system prompt: follow\nit, in its order, for this task. What this mode fixes for you:\n\n")
	if s.gReview == "rdd" {
		gh := a.nextGentleHome()
		base := a.nextLaneField(proj, "base_ref_prefix") + a.nextLaneField(proj, "base")
		b.WriteString("- RECEIPT-DRIVEN DEVELOPMENT (RDD) IS ON for this task, natively, and only\n  this way. NEVER run gentle-ai with --cwd on this worktree or any repository\n  of the product: its review store is <git-common-dir>/gentle-ai, inside the\n  product repo, and that is forbidden. The binary is " + gh + "/bin/gentle-ai-task (its\n  wrapper fixes its HOME to a private one); never any other, never install,\n  sync or upgrade.\n  1. When the work is committed, make a clone and review THAT:\n     git clone --no-hardlinks <this worktree> $HW_ARTIFACTS/rdd-clone ;\n     in the clone, git fetch origin refs/remotes/" + base + ":refs/remotes/" + base + "\n     (the review base is the lane's base, " + base + ", never main). The clone's\n     HEAD is the candidate; commit nothing new in the product worktree for it.\n  2. " + gh + "/bin/gentle-ai-task review mode enable --scope global --cwd\n     $HW_ARTIFACTS/rdd-home — the task's own copy of gentle-ai's home, made on\n     first use: RDD is on for this task only and there is nothing to turn off.\n     NEVER enable it through " + gh + "/bin/gentle-ai (every executor shares that\n     home), and never --scope clone to turn it on: a clone setting can only\n     turn RDD off.\n  3. Drive the lifecycle, always through " + gh + "/bin/gentle-ai-task, with\n     --cwd <the clone> --base-ref " + base + " --committed-only: review status\n     --contract gentle-ai.review-integration/v2 --agent claude-code\n     --next-transition. Run EXACTLY the operation and tokens each transition\n     returns, never rebuilt: START first (answer consent with its granted\n     invocation), then status again. THE REVIEW IS NATIVE: a returned\n     review capture-result carries --agent and no --input, and running it makes\n     gentle-ai materialize the immutable context and run its own tool-free\n     reviewer in process, then admit the result. NEVER launch a lens or any\n     subagent for it, NEVER assemble a reviewer prompt, NEVER add --input,\n     NEVER hand candidate bytes over through a file: the context can be over\n     100 KB and relaying it by hand was cut by an API safeguard. When one\n     collect carries several reviewer slots, start every returned capture\n     concurrently in the order returned (one Bash call, backgrounded) and wait\n     for all of them. A review can outlast a tool timeout: run them with\n     run_in_background (or a long timeout), and a timeout is not a failed\n     capture: ask STATUS again before concluding anything. Run the\n     acknowledge-approved transition it returns once\n     approved. On correction_required, re-enter only through the\n     status_continuation it returns.\n  4. If a capture fails (provider, budget, lens_context_budget_exceeded, no\n     login), report done-invoker --blocked with its exact output. Do not\n     relay, do not retry with another command shape.\n  5. " + gh + "/bin/gentle-ai-task writes rdd-log.jsonl in $HW_ARTIFACTS for you,\n     locally, from each capture's own answer: role, verdict, findings,\n     duration. Telemetry is off; nothing leaves this machine.\n  6. NATIVE RDD IS THE ONLY REVIEW OF THIS TASK. Do NOT run the judgment-day\n     skill or any second reviewer over the same candidate, and do not report a\n     Judgment Day verdict: two reviews of one candidate are two verdicts and\n     twice the cost, and this brief chose RDD instead. (A design judgment the brief\n     asks for under `boundary:` is a separate gate, before the code, not this.)\n")
	} else {
		b.WriteString("- Receipt-driven development (RDD) is OFF for this task. gentle-ai is NOT\n  installed for you and must not be run: no `gentle-ai review ...`, no\n  `gentle-ai install`, no `gentle-ai sync`. Where ODD says \"when RDD is\n  enabled\", it is not. Review is Judgment Day, as this dispatch says below.\n")
	}
	delivery := ""
	switch s.gDelivery {
	case "single-pr":
		delivery = "single-pr — one pull request; do not slice it into a chain"
	case "stacked-to-main":
		delivery = "a chain of pull requests, stacked-to-main"
	case "feature-branch-chain":
		delivery = "a chain of pull requests, feature-branch-chain"
	}
	b.WriteString("- SDD is not selected. Do not enter SDD phases or create SDD artifacts.\n- The feature document is `odd/tasks/<feature-name>.md`, as ODD says. `odd/`\n  is ignored in this worktree by an untracked odd/.gitignore: never commit it,\n  never force-add it, never delete that file. Mirror it to engram as ODD says,\n  passing project explicitly.\n- NOBODY ANSWERS IN THIS PANE. Every question ODD tells you to ask the user —\n  an ambiguous authorization, a real product decision, a delivery or chain\n  strategy — goes through `ask-invoker \"<one question>\"`, and you act on its\n  reply. Never end your turn waiting for an answer here.\n- Delivery strategy is fixed by the brief: " + delivery + ". Do not ask for it.\n")
	dc := s.gDeployed
	if dc == "" {
		dc = "required"
	}
	if strings.HasPrefix(dc, "out-of-scope:") {
		b.WriteString("- DEPLOYED CHECK OUT OF SCOPE, declared by the brief (" + strings.TrimPrefix(dc, "out-of-scope: ") + ").\n  done-invoker opens your report with that line; do not claim a check against\n  what is deployed that you did not run.\n")
	} else {
		b.WriteString("- THE CHECK AGAINST WHAT IS DEPLOYED IS PART OF DONE, not a pending line in\n  the feature document. done-invoker refuses a done until\n  $HW_ARTIFACTS/deployed-check.md holds three lines: `target: <the deployed\n  https URL you verified>` (never localhost), `evidence: <a file under\n  $HW_ARTIFACTS with the run's result>`, and `RESULT: PASSED`.\n  If it cannot run — no QA account, no seed, no session — that is a HOLD:\n  `ask-invoker` naming exactly what is missing, then wait for the reply. If it\n  ran and failed, or the hold cannot be lifted, report `done-invoker --blocked`.\n")
	}
	return b.String()
}

// nextGentleRDDWord is _gentle_rdd_word.
func nextGentleRDDWord(review string) string {
	if review == "rdd" {
		return "RDD on (native)"
	}
	return "RDD off"
}
