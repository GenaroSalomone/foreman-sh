package main

// `--agent pi`, the launch half: what bin/hw does for the vendor pi after the manifest — the environment its children
// share, the startup screens read off the pane, the proof that the brief entered pi's conversation, the specialists note
// of its prompt and the receipts it can read. The dispatch half is t6_pi.go.
//
// bin/hw: _pi_startup_dialog, _pi_startup_ready, _pi_project_skills_prepare, the `pi` arms of _export_hw_env, _hw_env,
// _agent_start, _deliver_brief, _receipt_session, _receipt_model, _receipt_pi_thinking, _brief_preamble;
// _pi_inputs_file, _pi_inputs_count, _pi_normalized_sha, _pi_processed, _pi_admit, _pi_ops_procedure_file,
// _pi_delegable_agents, _pi_specialist_note.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ── the screens pi can hold before the brief ─────────────────────────────────

// piStartupDialog is _pi_startup_dialog: which startup screen of pi's, if any, the pane shows. The text it matches is
// pi's own (MEASURED against pi 1.1.0), so a rewording makes this blind rather than wrong.
func piStartupDialog(screen string) string {
	switch {
	case strings.Contains(screen, "Trust project folder?"):
		return "project-trust"
	case strings.Contains(screen, "cwd from session file does not exist"):
		return "missing-session-cwd"
	case strings.Contains(screen, "Welcome to pi, the minimal coding agent"):
		return "first-time-setup"
	case strings.Contains(screen, "Press any key to continue"):
		return "press-any-key"
	case strings.Contains(screen, "No API key for"), strings.Contains(screen, "re-authenticate"):
		return "login"
	}
	return ""
}

// piStartupReady is _pi_startup_ready: the one screen answered is the deprecation notice (any key dismisses it, enter
// changes nothing); every other one is a person's — a permission decision, a moved directory, a first-run setup, a login.
func (l *lch) piStartupReady(target string) bool {
	dialog := piStartupDialog(l.agentRead(target))
	switch dialog {
	case "":
		return true
	case "press-any-key":
		l.info("pi opened on a deprecation notice (\"Press any key to continue\") — answering enter, which changes nothing")
		if _, rc := l.herdr(true, true, "", "agent", "send-keys", target, "enter"); rc != 0 {
			return false
		}
		lSleep(2 * time.Second)
		return true
	}
	l.errf("pi is waiting on a startup screen (%s) that only a person may answer: clear it on %s, then re-deliver\n", dialog, target)
	return false
}

// piProjectSkillsPrepare is _pi_project_skills_prepare: creates the directory of symlinks (never by a dry run) and one
// link per project skill; true also when there are none. A second launch preparing the same set never follows an existing
// link into the skill's own directory (`ln -sfn`), and a link is never removed.
func (l *lch) piProjectSkillsPrepare() bool {
	dir, ok := l.piProjectSkillDir()
	if !ok {
		return true
	}
	if err := l.fs.mkdirAll(dir); err != nil {
		l.warn("pi project skills: cannot create " + dir)
		return false
	}
	for _, s := range l.piProjectSkills(true) {
		link := dir + "/" + s.name
		if to, err := os.Readlink(link); err == nil && to == s.dir {
			continue
		}
		if _, rc := l.sh(cmdSpec{name: "ln", args: []string{"-sfn", s.dir, link}, mut: true}); rc != 0 {
			l.warn("pi project skills: cannot link " + s.dir + " into " + dir)
			return false
		}
	}
	return true
}

// ── the environment pi's children share ──────────────────────────────────────

// piExportEnv is the `pi` half of _export_hw_env: pi's HALVES of the guard and the roster travel as environment, because
// its children (pi-subagents sessions) share it and an extension's flags would not reach them. THE UNSET IS AS
// LOAD-BEARING AS THE EXPORT: base_env copies this process, so a brainer that carries one must not hand it to another vendor.
func (l *lch) piExportEnv(cwd string) {
	if l.agent != "pi" {
		l.unsetEnv("HW_PI_BRAIN_GUARD", "HW_PI_FALLBACK_MODEL", "HW_PI_FALLBACK_THINKING", "HW_PI_ASKED_MODEL", "HW_PI_SUBAGENTS_DIR", "PI_SUBAGENT_EXTRA_AGENT_DIRS")
		return
	}
	if l.brainGuardOn() {
		l.setEnv("HW_PI_BRAIN_GUARD", "1")
	} else {
		l.unsetEnv("HW_PI_BRAIN_GUARD")
	}
	if fb := l.piFallbackModel(); fb != "" {
		l.setEnv("HW_PI_FALLBACK_MODEL", fb)
		l.setEnv("HW_PI_FALLBACK_THINKING", piFallbackThinking)
	} else {
		l.unsetEnv("HW_PI_FALLBACK_MODEL", "HW_PI_FALLBACK_THINKING")
	}
	if l.model != "" {
		l.setEnv("HW_PI_ASKED_MODEL", l.model)
	} else {
		l.unsetEnv("HW_PI_ASKED_MODEL")
	}
	l.setEnv("HW_PI_SUBAGENTS_DIR", l.piSubagentsDir())
	if dirs := l.piAgentDirs(cwd); dirs != "" {
		l.setEnv("PI_SUBAGENT_EXTRA_AGENT_DIRS", dirs)
	} else {
		l.unsetEnv("PI_SUBAGENT_EXTRA_AGENT_DIRS")
	}
}

// piHwEnvFlags is the `pi` block of _hw_env: the same, in the `--env K=V` form of a pane split (`--here`).
func (l *lch) piHwEnvFlags(cwd string) string {
	if l.agent != "pi" {
		return ""
	}
	s := ""
	if l.brainGuardOn() {
		s += " --env HW_PI_BRAIN_GUARD=1"
	}
	if fb := l.piFallbackModel(); fb != "" {
		s += " --env HW_PI_FALLBACK_MODEL=" + fb + " --env HW_PI_FALLBACK_THINKING=" + piFallbackThinking
	}
	if l.model != "" {
		s += " --env HW_PI_ASKED_MODEL=" + l.model
	}
	s += " --env HW_PI_SUBAGENTS_DIR=" + l.piSubagentsDir()
	if dirs := l.piAgentDirs(cwd); dirs != "" {
		s += " --env PI_SUBAGENT_EXTRA_AGENT_DIRS=" + dirs
	}
	return s
}

// ── the brief entered pi's conversation ──────────────────────────────────────

// piInputsFile is _pi_inputs_file: where the adapter writes the sha256 of every message that reaches the session.
func (l *lch) piInputsFile() string {
	return l.wt + "/.hw/" + l.lHwRun() + "/pi-inputs.jsonl"
}

// piInputsCount is _pi_inputs_count: the lines the adapter has recorded so far (the baseline of THIS message).
func (l *lch) piInputsCount() int {
	data, err := os.ReadFile(l.piInputsFile())
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "\n")
}

var piCRLF = regexp.MustCompile(`\r\n?`)
var piTrailingNL = regexp.MustCompile(`\n+$`)

// piNormalizedSha is _pi_normalized_sha: the text the way the adapter normalises it (CRLF and CR to LF, every TAB to four
// spaces — MEASURED: pi's editor expands them — and no trailing newlines).
func piNormalizedSha(text string) string {
	t := string([]rune(text)) // python's decode("utf-8", "replace")
	t = piCRLF.ReplaceAllString(t, "\n")
	t = strings.ReplaceAll(t, "\t", "    ")
	t = piTrailingNL.ReplaceAllString(t, "")
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// piMatchInputs is the jq of _pi_processed over the lines the adapter recorded after the baseline: "exact", "normalized"
// or "none <n interactive messages>". A line jq cannot read, or a record it cannot index, is "none 0".
func piMatchInputs(data string, base int, exact, norm string) string {
	lines := strings.SplitAfter(data, "\n")
	if base > len(lines) {
		base = len(lines)
	}
	vals, perr := parseStream([]byte(strings.Join(lines[base:], "")))
	if perr != nil {
		return "none 0"
	}
	n, isExact, isNorm := 0, false, false
	for _, v := range vals {
		if v.kind != vObj && v.kind != vNull {
			return "none 0" // .source on a number, a string or an array is an error: jq prints nothing
		}
		src, ok := v.get("source")
		if !ok || src.kind != vStr || src.s != "interactive" {
			continue
		}
		n++
		if h, ok := v.get("sha256"); ok && h.kind == vStr && h.s == exact {
			isExact = true
		}
		if h, ok := v.get("normalized_sha256"); ok && h.kind == vStr && h.s == norm {
			isNorm = true
		}
	}
	switch {
	case isExact: // # MUTATION-ANCHOR: 986-G07
		return "exact"
	case isNorm:
		return "normalized"
	}
	return "none " + strconv.Itoa(n)
}

// piProcessed is _pi_processed: "exact", "normalized" (true) or "none <seen>" (false), polling HW_PI_PROCESSED_WAIT_MS.
func (l *lch) piProcessed(text string, base int) (string, bool) {
	budget := lrcMillis(l.env("HW_PI_PROCESSED_WAIT_MS"), 20000)
	sum := sha256.Sum256([]byte(text))
	exact := hex.EncodeToString(sum[:])
	norm := piNormalizedSha(text)
	var waited int64
	seen := "0"
	for {
		if data, err := os.ReadFile(l.piInputsFile()); err == nil {
			r := piMatchInputs(string(data), base, exact, norm)
			if r == "exact" || r == "normalized" {
				return r, true
			}
			seen = strings.TrimPrefix(r, "none ")
		}
		if waited >= budget {
			break
		}
		l.lrcSleep()
		waited += 1000
	}
	return "none " + seen, false
}

// piAdmit is _pi_admit: the brief of a fresh launch, after herdr proved a turn opened, is `processed` only when the
// adapter saw THIS text. True when it did; otherwise it says why, and the brief is not proven in the pane's context.
func (l *lch) piAdmit(pane, how, nbytes string, base int) bool {
	got, ok := l.piProcessed(l.briefText, base)
	file := l.wt + "/.hw/" + l.lHwRun() + "/pi-inputs.jsonl"
	if ok {
		l.receipt("brief_admitted", "yes — pi processed it: the adapter saw a message with the brief's sha256 ("+got+" match; "+how+")",
			"setup/pi/hw-executor.ts input event -> "+file+" vs brief_sha256")
		l.receipt("brief_delivery_id", l.dlRunID()+":brief", "the id hw gives the brief's delivery on every vendor; for pi it is matched by hash, pi has no endpoint to key it on")
		l.ok("brief IN THE CONVERSATION on " + pane + " (" + nbytes + " bytes, from " + l.brief + ") — pi receipt: processed (" + got + " sha256 match)")
		return true
	}
	if got == "none 0" {
		l.receipt("brief_admitted", "no — a turn opened but pi's adapter recorded no message within "+strconv.FormatInt(lrcMillis(l.env("HW_PI_PROCESSED_WAIT_MS"), 20000)/1000, 10)+"s",
			file+" is empty or absent after delivery")
		l.warn("a turn opened on " + pane + ", but pi's adapter (setup/pi/hw-executor.ts) recorded no message: it may not be loaded, and then the write guard is not running either")
	} else {
		l.receipt("brief_admitted", "no — pi's adapter saw "+strings.TrimPrefix(got, "none ")+" message(s), none with the brief's sha256", file+" vs brief_sha256")
		l.warn("pi received something on " + pane + " that is NOT the brief the way hw sent it (the terminal may have cut or altered the paste)")
	}
	l.warn("THE BRIEF IS NOT PROVEN IN " + pane + "'s CONTEXT. Read what it holds: herdr agent read " + pane + " --source recent-unwrapped")
	l.briefUndelivered(pane, "herdr", pane, "-")
	return false
}

// piDialogBeforeDelivery is the pi block of _deliver_brief: the same second look as codex's, immediately before anything is
// typed. A dialog rendered after _agent_start's single read would swallow the brief, and these wait for a person. True
// when the delivery must stop.
func (l *lch) piDialogBeforeDelivery(pane string) bool {
	dialog := piStartupDialog(l.agentRead(pane))
	switch dialog {
	case "", "press-any-key":
		return false
	}
	l.warn("pi is holding a startup screen on " + pane + " (" + dialog + "): NOTHING was typed — the brief would have landed in it and been lost")
	l.receipt("brief_admitted", "no — pi was holding "+dialog+" at delivery time, nothing typed", "herdr agent read "+pane+" --source detection, read immediately before delivery")
	l.warn("THE BRIEF IS NOT IN " + pane + "'s CONTEXT, and re-sending it is NOT the fix: these screens wait for a PERSON. Clear it on that pane first, then re-deliver:")
	l.redeliverHerdrLine(pane)
	return true
}

// ── what the receipts can read of pi ─────────────────────────────────────────

// piRuntimeFile is the adapter's own record of the session (model_ref, thinking), rewritten at every turn.
func (l *lch) piRuntimeFile() string { return l.wt + "/.hw/" + l.hwRun + "/pi-runtime.json" }

// piRuntimeRead is the pi arm of _receipt_model's loop: the model and the thinking the session says it runs.
func (l *lch) piRuntimeRead() (model, thinking, src string) {
	f := l.piRuntimeFile()
	if data, err := os.ReadFile(f); err == nil {
		model = jqRawOr(string(data), "", "model_ref")
		thinking = jqRawOr(string(data), "", "thinking")
	}
	return model, thinking, "the model_ref in " + f + ", published by setup/pi/hw-executor.ts from the session's own ctx.model"
}

// piReceiptThinking is _receipt_pi_thinking: the thinking the session really has, against the one asked — pi CLAMPS a level
// the model does not support (MEASURED: --thinking off on opencode-go/deepseek-v4.1-flash runs low), so `off` is a request.
func (l *lch) piReceiptThinking(got string) {
	asked := l.effort
	src := "the thinking in " + l.piRuntimeFile() + ", from pi.getThinkingLevel() in the session"
	if got == "" {
		l.receipt("thinking_running", "none — pi-runtime.json carried no thinking level", src)
		l.receipt("thinking_verdict", "UNESTABLISHED — the thinking level that runs could not be read; asked: "+lrcOr(asked, "<pi default>"), src)
		l.warn("UNESTABLISHED  thinking: could not read the level this pi executor runs — asked " + lrcOr(asked, "<pi default>"))
		return
	}
	l.receipt("thinking_running", got, src)
	var verdict string
	switch {
	case asked == "":
		verdict = "recorded — no --thinking was given, so pi's default ran"
	case got == asked:
		verdict = "match — asked " + asked + ", running " + got
	default:
		verdict = "DIVERGENCE — asked thinking " + asked + ", the session runs " + got + " (pi clamps a level the model does not support)" // # MUTATION-ANCHOR: 986-G06
	}
	l.receipt("thinking_verdict", verdict, "compared EFFORT with thinking_running")
	if strings.HasPrefix(verdict, "DIVERGENCE") {
		l.warn("DIVERGENCE  thinking: " + strings.TrimPrefix(verdict, "DIVERGENCE — "))
	}
}

// ── the specialists note of the prompt ───────────────────────────────────────

// piPromptApplies is the condition of _brief_preamble's pi branch: a pi executor with specialists on disk, or with an
// ops-investigator procedure to hand it.
func (l *lch) piPromptApplies(specialists string) bool {
	vendor := orDefaultStr(l.env("HW_EXECUTOR_VENDOR"), l.agent)
	return vendor == "pi" && (specialists != "" || l.piOpsProcedureFile() != "")
}

// piSpecialistNote is _pi_specialist_note: pi-subagents does not read `.claude/agents`, so the specialists are told apart
// into the ones pi can delegate to (the worktree tracks a pi copy and hw made it reachable) and the ones it cannot, and
// ops-investigator, whose job is a PROCEDURE, travels as text.
func (l *lch) piSpecialistNote(list string) string {
	proc := l.piOpsProcedureFile()
	deleg := l.piDelegableAgents(l.workdir(), list)
	var rest []string
	for _, n := range strings.Fields(list) {
		if !has(strings.Fields(deleg), n) {
			rest = append(rest, n)
		}
	}
	model := orDefaultStr(l.env("HW_PI_ASKED_MODEL"), l.model)
	var b strings.Builder
	if deleg != "" {
		b.WriteString("This executor is pi. The specialist subagents of this lane (" + list + ") are Claude Code agents, and pi-subagents does\n" +
			"not read `.claude/agents`. DELEGABLE: " + deleg + " — this repo tracks a pi copy of each in `.pi/agents`, and hw has pointed\n" +
			"pi-subagents at this worktree's. Call the `subagent` tool with agent \"<name>\" and ALWAYS pass `model: \"" + orDefaultStr(model, "the opencode-go model you run on") + "\"`:\n" +
			"their files pin an openai-codex model this machine's login cannot refresh (measured: a 401 and a failed mission),\n" +
			"so a call without `model` fails. Your guards travel with each child (a child's write into a protected tree or read of\n" +
			"a secret path is refused like yours). Remote access is still NOT granted by delegating: ops-worker reaches remote hosts over\n" +
			"ssh, and runs only what your brief authorizes (destination and operation, quoted from the operator\n" +
			"with its requested_by line); otherwise stay local and report it blocked.")
		if len(rest) > 0 {
			b.WriteString("\nNOT delegable (only in `.claude/agents`): " + strings.Join(rest, " ") + " — do their job inline with your own tools, or report it blocked.")
		}
	} else {
		shown := ""
		if list != "" {
			shown = " (" + list + ")"
		}
		b.WriteString("This executor is pi. The specialist subagents of this lane" + shown + " are Claude Code agents, and pi-subagents does\n" +
			"not discover `.claude/agents`: you CANNOT delegate to them, so do not try, and do not report one of them\n" +
			"missing as a blocker. If one of their jobs is yours, do it inline with your own tools, or report it blocked.")
	}
	if proc == "" {
		return bashSub(b.String())
	}
	b.WriteString("\n\nops-investigator (read-only operations lookups) is the one whose job is a\n" +
		"procedure, so its definition is below and you follow it yourself, inline. Your tools are pi's\n" +
		"(bash, read, grep, find, the hw_engram memory tools), not the Claude tool list its header names.\n\n" +
		"--- ops-investigator: procedure (from " + proc + ") ---\n")
	if data, err := os.ReadFile(proc); err == nil {
		b.WriteString(piStripFrontmatter(string(data)))
	}
	b.WriteString("--- end of the ops-investigator procedure ---\n\n" + piOpsOverride + "\n")
	if extra, err := os.ReadFile(l.c.brain + "/" + l.proj + "/pi-ops-overrides.md"); err == nil && isFile(l.c.brain+"/"+l.proj+"/pi-ops-overrides.md") {
		b.WriteString("\n" + string(extra))
	}
	return bashSub(b.String())
}

// piStripFrontmatter is `awk 'NR==1 && $0=="---" { fm=1; next } fm && $0=="---" { fm=0; next } !fm'`: every record but
// the frontmatter's, each ended by a newline.
func piStripFrontmatter(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var b strings.Builder
	fm := false
	for i, ln := range lines {
		switch {
		case i == 0 && ln == "---":
			fm = true
		case fm && ln == "---":
			fm = false
		case !fm:
			b.WriteString(ln + "\n")
		}
	}
	return b.String()
}

// piOpsOverride is the text after the procedure: what changes for the executor under pi.
const piOpsOverride = `WHAT CHANGES FOR YOU UNDER pi (these override the procedure above where they differ):
- REMOTE ACCESS IS NOT GRANTED BY THIS PROCEDURE. ssh, scp, cloud CLIs and API calls to a host run only when your
  brief names the destination and the operation (an authorization quoted from the operator, with its requested_by
  line). Where it does not, stay local, report it blocked and do not probe the destination. Do not discover or reuse
  an ambient ssh agent, ControlMaster socket or authenticated session.
- Secrets: a read guard refuses your read, grep and bash any path under ~/.ssh, any .env* file, *.db, *.credentials,
  *.pem, *.key and secrets/. ` + "`ssh -i ~/.ssh/<key>`" + ` is allowed (the key is used, not read); under --sandbox ~/.ssh is
  unreadable to the OS, so ssh cannot load its key at all. A refused read is a blocker to report, never a thing to
  route around: a step that sources an .env file is unavailable to you, say so.
- Memory: your tools are mcp__hw_engram__mem_search / mem_get_observation / mem_save, not mcp__engram__*.`
