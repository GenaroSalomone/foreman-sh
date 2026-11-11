package main

// The text hw puts before the brief in every executor prompt: ONE renderer for the launch (t5l_deliver.go buildBriefText)
// and for `hw next` (t4_next_msg.go nextMessage). A caller fills promptFacts from what hw knew at dispatch and calls
// renderPrompt; a difference between the two paths is a fact the caller supplies (retask, piNote, secrets, ...), never a
// branch in the caller. The prompt is the executor's whole contract and its sha256 is receipted.

import (
	"fmt"
	"regexp"
	"strings"
)

// promptFacts is everything renderPrompt reads. Text fields hold what only a file or a table can say (a lane note, the
// specialists on disk); everything else is a dispatch fact.
type promptFacts struct {
	// `hw next`: the message opens with the counter and the entry line it builds.
	retask    bool
	retaskSeq string
	entryLine string

	// the launch's frame, above the preamble
	assetsNote string

	// grants and the SDD entry
	secrets     string
	requestedBy string
	authorizes  []string // "<destination> :: <operation> :: <handle>"
	sdd         string   // the framework mode in play ("" = none)
	sddEntry    string   // its single entry command ("" = none)
	operator    string   // who a speckit decision belongs to

	// the preamble
	lane, task                    string
	binDir, brief, label, session string
	vendor                        string
	specialists                   string        // the lane's specialist subagents, one line
	piNote                        string        // a pi executor's specialists note ("" = not pi)
	model                         string        // the model the executor was asked to run (pi: the one it runs on)
	explorer                      *explorerTier // the cheap explorer on this executor (nil = not there)
	laneNote                      string
	laneQA                        string // "requires: browser", "deployed_check: required": why the brief overrides the lane's QA
	qaTarget                      bool
	portWeb                       string
	build                         bool // kind build or unset: closes with the behaviour rule and checks.md
	boundary                      string
	designGate                    bool
	reviewMode                    string
	jd                            bool   // a Judgment Day closes this build
	jdRequired                    bool   // the brief DECLARED it
	jdJudges                      string // "agents": jd-judge-a/b are registered for this vendor; "reviewers": they are not; "": hw cannot tell
	jdSkill                       string // "yes"/"no": the judgment-day skill is on this vendor's path; "": hw cannot tell
	jdSkillPath                   string // where a pi executor reads it from when it is not among its skills
	fastGate                      bool   // the work directory carries setup/test-hw
	plugins                       []string
	pluginsOnly                   bool // `review: plugins` closes this build

	briefText string
}

// promptBlock is one piece of the prompt. grp is the section it belongs to: blocks of one section are joined by a blank
// line, a rule stands between sections. when says whether this dispatch carries the block.
type promptBlock struct {
	id   string
	grp  int
	when func(*promptFacts) bool
	text func(*promptFacts) string
}

// the sections, in the order the executor reads them
const (
	grpContract = iota // what this is and how to report, who to ask, what to remember, whom to delegate to
	grpGranted         // what this brief grants, the framework's entry
	grpClose           // how a build closes
	grpBrief           // the brief itself
)

func pAlways(*promptFacts) bool { return true }

func hasRetask(f *promptFacts) bool    { return f.retask }
func hasAssets(f *promptFacts) bool    { return f.assetsNote != "" }
func hasSecrets(f *promptFacts) bool   { return f.secrets != "" }
func hasSession(f *promptFacts) bool   { return f.session != "" }
func hasPiNote(f *promptFacts) bool    { return f.piNote != "" }
func hasTier(f *promptFacts) bool      { return tierText(f) != "" }
func isBuild(f *promptFacts) bool      { return f.build }
func closesWithJD(f *promptFacts) bool { return f.jd } // # MUTATION-ANCHOR: 847-C-09

func hasAuthorization(f *promptFacts) bool  { return len(f.authorizes) > 0 } // # MUTATION-ANCHOR: 862-G18
func hasGrants(f *promptFacts) bool         { return hasSecrets(f) || hasAuthorization(f) }
func hasLaunchEntry(f *promptFacts) bool    { return !f.retask && f.sddEntry != "" }
func hasSpecialists(f *promptFacts) bool    { return f.piNote == "" && f.specialists != "" }
func hasLaneQAOverride(f *promptFacts) bool { return f.laneNote != "" && f.laneQA != "" }
func closesWithPlugins(f *promptFacts) bool { return !f.jd && f.pluginsOnly }

// pi loads no ~/.claude/CLAUDE.md, so for pi this line is the only carrier of the rule that a delegate inherits no remote access.
func hasNoRemoteLine(f *promptFacts) bool { return f.vendor == "pi" && !hasAuthorization(f) }
func noRemoteText(*promptFacts) string {
	return "No remote operation is authorized for this task, for you or a child: stay local, and report what needs one as blocked."
}

// THE GUARD A PI EXECUTOR IS JUDGED BY IS NOT THE ONE ITS BRANCH CARRIES. MEASURED 2026-11-04 (retro of
// pi-plugins-parity): hw loads `<lane root>/setup/pi/hw-executor.ts` — `piExtensionArgs` builds it from `vdRoot()`,
// core/t6_pi.go, which is HW_CORE_ROOT, the brain root bin/hw:64 exports — so a worktree's newer adapter never judges
// its own executor, and the adapter a session loaded is the one it keeps (measured: a session kept refusing `sleep 45`
// with the code it had loaded after the file on disk was patched, while a NEW session on that file ran it). In that run
// the branch had gained the sleep-poll call by rebase and a `sleep 540` still ran unrefused. Two ages, and Judgment Day
// round 1 is why the line states both: the ADAPTER is from session start, while the `.py` rules it asks
// (`sleepPollVerdict`, the two global hooks) are spawned per call and read then — from the LANE ROOT, which a branch's
// copy is not. The line tells the executor to report which guard judged it instead of re-running the command.
func hasPiGuardLine(f *promptFacts) bool { return f.vendor == "pi" }
func piGuardText(*promptFacts) string {
	return "Your guards are the LANE ROOT's: the adapter from this session's start (a rebase or ruling reaches a NEW " +
		"session only), the `.py` rules it asks on every call. Report which guard judged you; do not re-run a refused " +
		"command to prove one."
}

func noSpecialists(f *promptFacts) bool { return f.piNote == "" && f.specialists == "" }
const noSpecialistsLine = "No specialist subagents in this lane (no ops-investigator, no qa-tester)."

func noSpecialistsText(*promptFacts) string { return noSpecialistsLine } // # MUTATION-ANCHOR: 847-C-05

func isCodex(f *promptFacts) bool { return f.vendor == "codex" } // # MUTATION-ANCHOR: 847-C-06 # MUTATION-ANCHOR: 862-G19

func hasLaneNote(f *promptFacts) bool { return f.laneNote != "" } // # MUTATION-ANCHOR: 847-C-07

func hasQATarget(f *promptFacts) bool { return f.qaTarget && f.portWeb != "" } // # MUTATION-ANCHOR: 847-C-08

func hasDesignGate(f *promptFacts) bool { return f.designGate } // # MUTATION-ANCHOR: 847-C-10

// promptBlocks, in this order, IS the prompt.
var promptBlocks = []promptBlock{
	{"retask-open", grpContract, hasRetask, retaskOpenText},
	{"contract", grpContract, pAlways, contractText},
	{"ask", grpContract, pAlways, func(*promptFacts) string { return askNote }},
	{"memory", grpContract, pAlways, memoryText},
	{"epistemic", grpContract, pAlways, func(*promptFacts) string { return epistemicNote }},
	{"specialists-pi", grpContract, hasPiNote, func(f *promptFacts) string { return f.piNote }},
	{"specialists", grpContract, hasSpecialists, specialistsText},
	{"specialists-none", grpContract, noSpecialists, noSpecialistsText},
	{"remote-none", grpContract, hasNoRemoteLine, noRemoteText},
	{"pi-guard-session", grpContract, hasPiGuardLine, piGuardText},
	{"tier", grpContract, hasTier, tierText},
	{"codex-rules", grpContract, isCodex, codexRulesText},
	{"lane-note", grpContract, hasLaneNote, func(f *promptFacts) string { return f.laneNote }},
	{"lane-qa-override", grpContract, hasLaneQAOverride, laneQAOverrideText},
	{"qa-target", grpContract, hasQATarget, qaTargetText},
	{"assets", grpGranted, hasAssets, func(f *promptFacts) string { return f.assetsNote }},
	{"grants", grpGranted, hasGrants, grantsText},
	{"sdd-entry", grpGranted, hasLaunchEntry, sddEntryLaunchText},
	{"behaviour-fix", grpClose, isBuild, behaviourFixText},
	{"checks", grpClose, isBuild, func(*promptFacts) string { return checksNote }},
	{"design-gate", grpClose, hasDesignGate, designJudgmentText},
	{"close-judgment-day", grpClose, closesWithJD, closeJudgmentDayText},
	{"close-plugins", grpClose, closesWithPlugins, func(f *promptFacts) string { return reviewPluginsNote(f.plugins) }},
	{"brief", grpBrief, pAlways, func(f *promptFacts) string { return f.briefText }},
}

// renderBlocks joins the blocks a dispatch carries: a blank line inside a section, a rule between sections.
func renderBlocks(f *promptFacts, only func(promptBlock) bool) string {
	var sb strings.Builder
	n, prev := 0, 0
	for _, b := range promptBlocks {
		if !b.when(f) || (only != nil && !only(b)) {
			continue
		}
		if n > 0 {
			if b.grp != prev {
				sb.WriteString("\n\n---\n\n")
			} else {
				sb.WriteString("\n\n")
			}
		}
		sb.WriteString(bashSub(b.text(f)))
		n, prev = n+1, b.grp
	}
	return bashSub(sb.String())
}

// renderPrompt is the whole message the executor receives.
func renderPrompt(f *promptFacts) string { return renderBlocks(f, nil) }

// renderPreamble is what hw says about the work itself, without the grants and the brief: the contract and the close.
func renderPreamble(f *promptFacts) string {
	return renderBlocks(f, func(b promptBlock) bool { return b.grp == grpContract || b.grp == grpClose })
}

func retaskOpenText(f *promptFacts) string {
	return "NEXT TASK (task " + f.retaskSeq + " of this session — the previous one is closed and reported).\nYour ask budget and your done-invoker are reset for it: report this one with\ndone-invoker when it is finished, exactly as you did the last.\n\n" + f.entryLine + "---"
}

// askNote stays whole on every vendor: the measured gap is a pi executor that ignored a prohibition the brief itself wrote
// (deepseek), and the challenge is the brainer's only instrument for a wrong one.
const askNote = `ASK OR CHALLENGE, NEVER GUESS. Only your brainer decides what the brief does
not; ask once, then end your turn: the reply arrives as your next prompt. A brief that forbids what you measured is
a --challenge — evidence, not opinion, and it spends no ask — and you hold until
the ruling arrives, then act on it in this same task. A workaround hides the gap
from the only person who could close it.`

const epistemicNote = `BRIEF EPISTEMIC SAFEGUARD.

MEASURED — act only on a claim whose evidence you can name: a file:line, a
command and its output, an engram id. A heading, a citation or the brief's own
confidence is not evidence.
UNESTABLISHED — a cause you cannot evidence stays labelled in your report; it
does not become a fact on the way into code.
CONSTRAINTS — the brief's Limits and Verification are the task, not advice
about it.

A remedy resting on an UNESTABLISHED cause is a contradiction to report, not a
fix to apply. A remedy the evidence supports is yours to execute without asking.
Where the brief's Limits restrict what you run, the brief wins; the checks
done-invoker makes do not move.`

// grantsText is everything the brief's frontmatter grants, under one heading: its Bitwarden items and the remote operations
// the operator authorized (quoted, with who asked). Both say "for you and every subagent": a delegate inherits no more.
func grantsText(f *promptFacts) string {
	var sb strings.Builder
	sb.WriteString("GRANTED BY THIS BRIEF (hw, from its frontmatter)")
	if hasSecrets(f) {
		sb.WriteString("\n\nSecrets — items of the Bitwarden folder agents: " + f.secrets + ". Reach a value only as\n" +
			"    hw secret run <item>[.<field>] --as VAR [--item <item> --as VAR ...] -- <cmd>\n" +
			"which puts it in <cmd>'s environment and nowhere else; never bw, never the\n" +
			"session file, never print or write a value. Vault locked: --blocked, naming the item.")
	}
	if hasAuthorization(f) {
		sb.WriteString("\n\nRemote — only this, quoted from the operator:\n> requested_by: " + orDefault2(f.requestedBy, "<none>") + "\n")
		for _, rec := range f.authorizes {
			p := strings.Split(rec, " :: ")
			for len(p) < 3 {
				p = append(p, "")
			}
			sb.WriteString("> destination: " + p[0] + " | operation: " + p[1] + " | credential handle: " + p[2] + "\n")
		}
		sb.WriteString("Inside it, act without asking. Any other destination, operation, credential or\nsession is unauthorized, for you and every subagent: ask-invoker, or --blocked.")
	}
	return sb.String()
}

// sddEntryLaunchText is the launch's FIRST ACTION line (the framework entry).
func sddEntryLaunchText(f *promptFacts) string {
	phase := ""
	if f.sdd == "speckit" {
		phase = "\n\n" + speckitPhaseNote(f.operator)
	}
	return "FIRST ACTION, before anything else: run " + f.sddEntry + " for this task.\nThe framework mode here is " + f.sdd + " and this brief is its input, not a licence to\nstart editing. Do not read code, write files or run tests until " + f.sddEntry + " has run." + phase
}

// sddEntryRetaskText is `hw next`'s entry line, the text that opens the re-task message under the counter.
func sddEntryRetaskText(f *promptFacts) string {
	if f.sdd == "" {
		return ""
	}
	if f.sddEntry == "" {
		return bashSub("Framework mode for this task: " + f.sdd + ". hw forces no entry command.\n\n")
	}
	entry := bashSub("FIRST ACTION, before anything else: run " + f.sddEntry + " for this task. The framework mode\nfor it is " + f.sdd + " and what follows is its input, not a licence to start editing.\n\n")
	if f.sdd == "speckit" {
		entry += "\n\n" + bashSub(speckitPhaseNote(f.operator)) + "\n\n"
	}
	return entry
}

func speckitPhaseNote(operator string) string {
	return `A SPEC KIT PHASE IS NOT THE TASK. Finishing /speckit-specify — or plan, tasks,
or any other speckit-* phase — is not finishing this task: go straight on to the
next phase, through implementation, and close with done-invoker only when the
flow and the brief are both done. A [NEEDS CLARIFICATION] marker is yours to
resolve from the brief and the code: decide, write the decision and its reason
into the spec, and continue. Do not wait for a human for it. Only a decision
that is genuinely ` + operator + `'s — money, a third party, an irreversible or outward
action — goes through ask-invoker, and the task stays open while it waits.

BETWEEN /speckit-tasks AND /speckit-implement, A FRESH /speckit-analyze. Run it
in a new subagent that sees only spec.md, plan.md, tasks.md and the project
constitution — not this conversation, not your reasoning. Its report is
read-only. A CRITICAL finding is corrected once, in the artifact it names, and
then you go on to /speckit-implement without re-running it. Anything below
CRITICAL does not stop the flow.`
}

func contractText(f *promptFacts) string {
	brief := f.brief
	if brief == "" {
		brief = "<no brief file>"
	}
	return `HW TASK ` + f.lane + `/` + f.task + ` — ` + orDefault2(f.vendor, "an") + ` executor, unattended: nobody reads this pane.
Your brief is below, and on disk at ` + brief + ` if you lose it.

The task is open until you run done-invoker.
done-invoker and ask-invoker are shell commands in ` + f.binDir + `, not tools:

    ` + f.binDir + `/done-invoker "<what you established; the engram #ids; 900 chars max>"   # rehearse: done-invoker --check
    ` + f.binDir + `/done-invoker --blocked "<what stopped you; what would unblock it>"   # you stay; a ruling resumes this task — never --retask it
    ` + f.binDir + `/ask-invoker "<one question, 600 chars max>"   # 3 per task; then END YOUR TURN: the reply arrives as your next prompt (never sleep or poll)
    ` + f.binDir + `/ask-invoker --challenge "<what the brief forbids> :: <your evidence against it>"   # own budget; hold until ruled

No question tool reaches a person. A missing tool, capability or credential is a
--blocked report, never a silent stop.

A friction (a tool, hook or doc that cost you time): mem_save with topic_key
`+f.lane+`/`+f.task+`/friction, its #id in your done summary; never channel-send it.`
}

// tierText renders the lines that apply to THIS executor: reading goes to the explorer only when it runs a different model than
// the executor's own; defined edits go to a mid-tier subagent only when
// the executor is top tier (and not pi, whose roster already runs builds on its build model). "" when no line survives.
func tierText(f *promptFacts) string {
	if f.explorer == nil {
		return ""
	}
	var lines []string
	reads := f.explorer.model != f.model
	if reads {
		lines = append(lines, "Delegate reading. Searches, code maps, call sites, docs -> "+explorerName+" ("+f.explorer.model+"; "+f.explorer.call+"), which returns file:line, not files.")
	}
	if f.vendor != "pi" && f.explorer.implement != "" && modelIsTopTier(f.model) {
		lines = append(lines, "Defined edits and test runs -> "+f.explorer.implement+".")
	}
	if len(lines) == 0 {
		return ""
	}
	end := "Design, audit and verdicts stay with you."
	if reads {
		end += " More than three files for one question is " + explorerName + "'s."
	}
	return strings.Join(lines, "\n") + "\n" + end
}

// memoryText: the proxy accepts the label (bin/engram-label-proxy), forces
// all_projects on a search, and refuses a decision/discovery/bugfix save without a topic_key: the executor is told the last
// because pi and codex load no rule saying so.
func memoryText(f *promptFacts) string {
	label := orDefault2(f.label, "brain")
	key := "a decision/discovery/bugfix save also needs a topic_key, e.g. " + f.lane + "/" + f.task + "/findings."
	if f.session == "" {
		return "Memory is engram, label " + label + " (another label is refused); " + key
	}
	return "Memory is engram, label " + label + ". Every mem_save passes session_id: " + f.session + "\nand project: " + label + " (another label is refused); " + key
}

func specialistsText(f *promptFacts) string {
	return "Specialists: " + strings.Join(strings.Fields(f.specialists), ", ") + ". When the job is theirs, delegate it: they carry the access and its conventions."
}

func codexRulesText(f *promptFacts) string {
	return `RULES CODEX DOES NOT LOAD (shared rules, one line each):
- Secrets: never print, copy into a new file, paste or commit .env, *.db, *.credentials or secrets/; report key names, never values.
- A credential comes as a ` + "`secrets:`" + ` item (` + "`hw secret run`" + `) or a Keychain handle; never ask for a value.
- Browser: close it with ` + f.binDir + `/browser-close, and never retry a failed launch (a failed launch is a blocked report).
- Artifacts (audio, exports, captures, generated files) go to $HW_ARTIFACTS, never into git status.`
}

func laneQAOverrideText(f *promptFacts) string {
	return "THIS BRIEF OVERRIDES THE LANE NOTE ABOVE ON BROWSERS AND QA: it declares " + f.laneQA + ". What the brief asks you to verify in a browser or against what is deployed IS your task, not the brainer's; the sentence about staging-only QA describes the lane default and does not apply to it."
}

func qaTargetText(f *promptFacts) string {
	return "QA target: http://localhost:" + f.portWeb + " — pass it as `pnpm qa <flow> --target <url>`, to qa-tester in its prompt, and to `harness open <url>`. Never staging, never :3001, unless the brief says so."
}

// closeJudgmentDayText is the close of a build with a Judgment Day. What hw knows decides what is written: a declared review has
// no optional exit; a default one on a vendor whose skill hw knows is absent says so itself; judges and the skill's path are
// stated for the vendor, not left to the executor to work out.
func closeJudgmentDayText(f *promptFacts) string {
	const commit = "1. Commit per work unit as you go, on this branch, Conventional Commits; uncommitted work is neither judged nor delivered."
	head := "Closing this code task: Judgment Day, then done-invoker, in this order:\n\n" + commit
	if !f.jdRequired && f.jdSkill == "no" {
		return judgmentDayWithPlugins(head+"\n2. Judgment Day: not installed for this executor — write \"Judgment Day: not installed (optional)\" in the report.", f.plugins)
	}
	skill := "the judgment-day skill"
	if f.jdSkillPath != "" {
		skill += " (if it is not among your skills, follow " + f.jdSkillPath + " yourself)"
	}
	var judges string
	switch f.jdJudges {
	case "agents":
		judges = "with its judges jd-judge-a and jd-judge-b"
	case "reviewers":
		judges = "with two blind `reviewer` subagents (read-only), judging the same candidate blind, one at a time if need be"
		if f.vendor == "pi" {
			// the dispatch says WHY it fell back: pi registers the account's judges, and hw found them missing
			judges += " (jd-judge-a and jd-judge-b are not on disk for pi, so this is what hw can promise)"
		}
	case "none":
		judges = "with no judge at all: pi-subagents is not loaded in this executor, so neither jd-judge-a/jd-judge-b nor a `reviewer` subagent can be launched — say so in the report (a REQUIRED Judgment Day is then --blocked, never skipped silently)"
	default:
		judges = "with its judges jd-judge-a and jd-judge-b; where no such agent is registered, with two blind builtin `reviewer` subagents (read-only), judging the same candidate blind, one at a time if need be"
	}
	if f.vendor == "pi" {
		// pi-subagents refuses a second subagent call in the same turn ("a subagent call is already in progress" —
		// measured 2026-11-04), so two independent judges travel in ONE call, the shipped `parallel` workflow resource. The close
		// counts only a call that ran and waited (async:false), and `reviewer` is not one of the two: a measured session
		// used it in a judge's slot. Paid for by the shorter specialist note (t6_pi_launch.go), inside the 5000 B budget.
		if f.jdJudges == "agents" {
			judges += ", both in ONE `subagent({workflow:\"parallel\",args:{tasks:[{agent:\"jd-judge-a\",…},{agent:\"jd-judge-b\",…}]},async:false})` call; one per turn. Plugins: async:false"
		} else if f.jdJudges != "none" {
			// "none" just said no subagent tool is loaded: an async:false rule for one would contradict it
			judges += ". Judge/plugin subagent calls need agent, task, async:false"
		}
	}
	var rule, report string
	switch {
	case f.jdRequired:
		rule = "REQUIRED: this brief declares it, and \"not installed\" is not an outcome. "
	case f.jdSkill == "yes":
	default:
		rule = "Judgment Day is optional: if the skill is not among your available skills, it is not installed, there is nothing to judge with and nothing ESCALATED; say \"Judgment Day: not installed (optional)\" in the report and close normally. "
		report = " (or \"not installed (optional)\")"
	}
	diffFile := reviewDiffFileClause // the plugin step already says it when the closing has one
	if len(reviewSkillsOf(f.plugins)) > 0 {
		diffFile = ""
	}
	return judgmentDayWithPlugins(head+"\n"+
		"2. Run "+skill+" over `git diff \"$(hw review-range)\"..HEAD` (the branch point computed now, never a range of your own) "+judges+"."+diffFile+" At most two rounds. "+rule+
		"If hw review-range fails: --blocked. An empty range is reported, not judged. After a rebase, run `hw review-delta <judged-base> <judged-head>` and do what it prints."+judgeSpecText(f.briefText)+"\n"+
		"3. The skill's fix-round question needs no answer here: this brief is the authorization. Apply the fixes both judges confirm; judges that contradict each other, or anything left after round two, is ESCALATED: report it, do not wait.\n"+
		"4. The report carries the judged range (base SHA..head SHA), the verdict APPROVED or ESCALATED"+report+", and, separately, each severe finding only ONE judge raised (unconfirmed: fix it only once reproduced), with its file:line.", f.plugins)
}

var (
	briefGoalHeading = regexp.MustCompile(`(?m)^## Goal[ \t]*$`)
	briefDoneHeading = regexp.MustCompile(`(?m)^## Done when[ \t]*$`)
)

// judgeSpecText: a judge that sees only the diff cannot judge intent (a release passed four reviews and broke a rule no judge
// was given, 2026-11-06). A brief with both `## Goal` and `## Done when` has its executor hand those two sections, verbatim,
// to each judge, and the judges answer per rule; any other brief gets "" and the close is unchanged.
func judgeSpecText(brief string) string {
	if !briefGoalHeading.MatchString(brief) || !briefDoneHeading.MatchString(brief) {
		return ""
	}
	return " Give each judge, beside the diff, the brief's `## Goal` and `## Done when` verbatim; each answers per Done-when rule: held / broken (file:line) / not judgeable from the diff. List every \"not judgeable\" rule in the report."
}

func designJudgmentText(f *promptFacts) string {
	end := reviewDesignEnd(f.reviewMode)
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
judges confirmed and the approach can close; your code closes them, and
%s is where that is checked. done-invoker
refuses a done report without an APPROVED verdict whose design still hashes to
design_sha256. An ESCALATED design is a blocker, not a
prompt to code anyway: report it with done-invoker --blocked, naming the
bypass classes the judges found. Two rejected designs mean the approach is
your brainer's call.`, f.boundary, end)
}

const checksNote = `Before working, write $HW_ARTIFACTS/checks.md: one row per requirement of the
brief's Verification section and per numbered goal —

    requirement | verdict | evidence | correction

verdict is exactly pass, fail or unresolved; evidence is a command and its
result, or a file:line. Fill it in as you go: done-invoker refuses a missing
file, no rows, an empty verdict, or any fail row — a requirement you did not
meet closes --blocked, never done.`

// behaviourFixText is the red-test rule; the fast-gate clause only where the repo carries setup/test-hw, and it says the full gate is
// the brainer's (2026-11-09: executors kept running it and the brainer killed five). What done-invoker checks does not move.
func behaviourFixText(f *promptFacts) string {
	s := `A behaviour fix ships the test that fails on the old code. Prove it: revert
the fix, watch that test go red, restore it, watch it go green, and say so in
the commit. An existing expectation that asserts the old behaviour is the
defect written a second time: change it. The one exception is a refactor — it
changes no expectation, and a refactor that does has broken something.`
	if f.fastGate {
		s += `

Verify touched subjects only, in the background (a foreground call is cut at 600 s):
git diff --name-only "$(git merge-base HEAD main)" > "$HW_ARTIFACTS/changed"; HW_TEST_CHANGED="$HW_ARTIFACTS/changed" HW_TEST_GATE=fast bash setup/test-hw --keep-going
The full fast gate is the brainer's: do not run it.`
	}
	return s
}

// claudeDirsHas is a file under the two `.claude` roots an executor reads: the account's config dir (default
// ~/.claude) and the work directory's. A Claude executor reads both; pi registers the same `.claude/agents` with
// pi-subagents (piClaudeAgentDirs), so one predicate answers for both.
func claudeDirsHas(cfgDir, home, workdir, rel string) bool {
	if cfgDir == "" {
		cfgDir = home + "/.claude"
	}
	return isFile(cfgDir+"/"+rel) || isFile(workdir+"/.claude/"+rel)
}

// jdFacts is what hw KNOWS of the Judgment Day a vendor can run, so the prompt states it instead of asking the executor to work
// it out: claude has the skill and the two judge agents on its config dir's path (or the work directory's); pi has the skill
// where piSkillDirFor finds it and the judges registered (hw-executor.ts registers jd-*); for any other vendor hw cannot tell.
func jdFacts(vendor, cfgDir, home, workdir, piSkillDir string, piSubagents bool) (judges, skill, skillPath string) {
	switch vendor {
	case "claude":
		judges, skill = "reviewers", "no"
		if claudeDirsHas(cfgDir, home, workdir, "agents/jd-judge-a.md") && claudeDirsHas(cfgDir, home, workdir, "agents/jd-judge-b.md") {
			judges = "agents"
		}
		if claudeDirsHas(cfgDir, home, workdir, "skills/judgment-day/SKILL.md") {
			skill = "yes"
		}
	case "pi":
		// The judges pi can launch are the ones hw registers with pi-subagents (piClaudeAgentDirs: the lane's
		// `.claude/agents` then the account's), so hw can see whether they exist instead of handing over two names
		// that are not registered. Where they are absent the generic "reviewers" wording is the honest answer.
		// Without pi-subagents nothing can be launched at all — no judge and no `reviewer` — so the closing says that.
		judges, skill = "agents", "no"
		switch {
		case !piSubagents:
			judges = "none"
		default:
			if !claudeDirsHas(cfgDir, home, workdir, "agents/jd-judge-a.md") || !claudeDirsHas(cfgDir, home, workdir, "agents/jd-judge-b.md") {
				judges = "reviewers"
			}
		}
		if piSkillDir != "" {
			skill, skillPath = "yes", piSkillDir+"/SKILL.md"
		}
	}
	return judges, skill, skillPath
}
