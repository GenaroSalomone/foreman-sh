package main

// `--agent pi`, the dispatch half: everything bin/hw does for the vendor pi between its arguments and the line "dry run —
// nothing created, no agent started" (and the launch words the launch hands herdr). Until this file a pi dispatch was
// refused by hw-core (core/t5.go) and bin/hw kept it on bash (`_core_pi_argv`); now it is answered here, byte for byte.
//
// bin/hw: _apply_pi_model_default, _pi_fallback_model, _pi_agent_dir, _pi_subagents_dir, _pi_subagents_ready,
// _pi_extra_agent_dir, _pi_agent_dirs, _pi_extension_args, _pi_skill_dir, _pi_project_skills, _pi_decided_skills,
// _pi_skills, _pi_project_skill_dir, _pi_skill_args, _plugins_pi_line_check, the `pi)` arms of _build_agent_args,
// _engram_proxy_args, _validate_sdd_vendor_pair and _dispatch_manifest (PI_LINES), and
// lib/hw/explorer.sh's pi arms. The launch half (environment, startup screens, the brief's processed proof, the receipts)
// is t6_pi_launch.go. The pi arms that other files carry (permissions line, brain-guard line, plugins line, sandbox line,
// capabilities) stay where the other vendors' are, as one more `case`.
//
// No `# MUTATION-ANCHOR` comments here: the bash ids of pi (984, 986, 989, 991, 979-M09, 781-M07, 1003-M03) are named in
// the port's report, and each of them is held below by a test that goes red when its line is mutated.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"unicode/utf8"
)

// piState is what the pi arms keep of the dispatch (bash: PI_EFFORT_SRC).
type piState struct {
	piEffortSrc string // PI_EFFORT_SRC: where a defaulted --thinking came from; the manifest's effort line says it
}

// THE pi ROSTER (opencode-go), the operator's approval of 2026-10-11: the model by KIND and the one it falls to on a MODEL
// error (never on a spent quota: the cap is shared dollars).
const (
	piModelDesign      = "opencode-go/deepseek-v4-pro"
	piModelBuild       = "opencode-go/deepseek-v4-pro"
	piModelReview      = "opencode-go/glm-5.3-flash"
	piModelCheap       = "opencode-go/deepseek-v4.1-flash"
	piFallbackFlash    = "opencode-go/glm-5.3-flash"
	piFallbackPro      = "opencode-go/deepseek-v4-pro"
	piFallbackThinking = "low"

	// piExplorerModel is HW_EXPLORER_MODEL_PI: the roster's `explore` model.
	piExplorerModel = "opencode-go/deepseek-v4.1-flash"

	piEngramProxyLine = "pi extension hw_engram (setup/pi/hw-executor.ts; raw mcp__engram__* refused)"
)

// piApplyModelDefault is _apply_pi_model_default: no lane pins a pi model (a lane's own pin is its vendor's id, cleared
// for an explicit --agent pi), so the roster decides by the brief's kind.
func (d *dsp) piApplyModelDefault() {
	kind := d.vdBriefKind()
	switch kind {
	case "design", "audit":
		d.model = piModelDesign
	case "review":
		d.model = piModelReview
	case "explore":
		d.model = piModelCheap
	default:
		d.model = piModelBuild
	}
	d.modelSrc = "default; pi roster, kind " + orDefaultStr(kind, "none")
}

// piFallbackModel is _pi_fallback_model: the model a pi executor of this dispatch falls to on a MODEL error, "" when the
// model already is the fallback. Decided by the KIND, also for an explicit --model.
func (d *dsp) piFallbackModel() string {
	fb := piFallbackFlash
	if d.vdBriefKind() == "review" {
		fb = piFallbackPro
	}
	if fb == d.model {
		return ""
	}
	return fb
}

// ── where pi lives ───────────────────────────────────────────────────────────

// piExtraAgentDir is _pi_extra_agent_dir: "<dir>/.pi/agents" when the lane's checkout tracks one (a product lane only).
func (d *dsp) piExtraAgentDir(work string) string {
	if d.c.t.get(d.proj, "product_repo") != "1" {
		return ""
	}
	if !isDir(d.c.checkout(d.proj) + "/.pi/agents") {
		return ""
	}
	if work == "" || strings.ContainsAny(work, " \t\n\v\f\r:") {
		return ""
	}
	return work + "/.pi/agents"
}

// piAgentDirs is _pi_agent_dirs: every extra agent directory a pi executor gets, PATH-style — the lane's own `.pi/agents`
// and hw's cheap-explorer directory (the brain's, never the product repo's), the latter only when pi-subagents is loaded.
func (d *dsp) piAgentDirs(work string) string {
	lane := d.piExtraAgentDir(work)
	exp := ""
	if d.piSubagentsReady() {
		exp = d.explorer().piAgentDirOK()
	}
	out := lane
	if lane != "" && exp != "" {
		out += ":"
	}
	return out + exp
}

// piClaudeAgentDirs is the PATH-style list of `.claude/agents` directories hw-executor.ts registers with pi-subagents
// (HW_PI_CLAUDE_AGENT_DIRS): the one a Claude executor of this lane sees, the worktree's. A lane with no product repo
// has it linked at launch from its brain directory (linkLaneClaude), so before the worktree exists the brain's own
// directory stands for it. "" when pi-subagents is not loaded (nothing to register with) or the path cannot travel in
// a PATH-style list.
func (d *dsp) piClaudeAgentDirs(work string) string {
	if !d.piSubagentsReady() {
		return ""
	}
	dir := ""
	if work != "" && isDir(work+"/.claude/agents") {
		dir = work + "/.claude/agents"
	} else if d.c.t.get(d.proj, "product_repo") != "1" && d.proj != "" && isDir(d.c.brain+"/"+d.proj+"/.claude/agents") {
		dir = d.c.brain + "/" + d.proj + "/.claude/agents"
	}
	if dir == "" || strings.ContainsAny(dir, " \t\n\v\f\r:") {
		return ""
	}
	return dir
}

// piDir is _explorer_pi_dir; piAgentDirOK is _explorer_pi_agent_dir: the directory pi-subagents reads as an extra
// user-scope agent dir, "" when the explorer's role is not there or the path cannot travel in a PATH-style list.
func (x explorerSrc) piDir() string { return x.root + "/setup/pi/agents" }

func (x explorerSrc) piAgentDirOK() string {
	d := x.piDir()
	if !isReadable(d + "/" + explorerName + ".md") {
		return ""
	}
	if d == "" || strings.ContainsAny(d, " \t\n\v\f\r:") {
		return ""
	}
	return d
}

// piSubagentsReady is _pi_subagents_ready for the explorer's source (a re-task holds no dispatch).
func (x explorerSrc) piSubagentsReady() bool {
	base := x.piHome + "/npm/node_modules/pi-subagents"
	return isReadable(base+"/index.js") && isReadable(base+"/src/api/required-child-extensions.js")
}

// piExplorerResolve is the `pi)` arm of _explorer_resolve, which checks the explorer is IN the directory list
// (`_pi_agent_dirs ""`) rather than that it could have been.
func (d *dsp) piExplorerResolve(m string) {
	if !d.piSubagentsReady() {
		d.explorerLine = "NOT INJECTED — pi-subagents is not loaded in this pi executor (see the pi subagents line): it has no subagent tool, so there is nothing to delegate to"
		return
	}
	if strings.Contains(":"+d.piAgentDirs("")+":", ":"+d.explorer().piDir()+":") {
		d.explorerInjected = true
		d.explorerLine = explorerName + " on " + m + " — setup/pi/agents via PI_SUBAGENT_EXTRA_AGENT_DIRS (pi-subagents user scope)"
		return
	}
	d.explorerLine = "NOT INJECTED — setup/pi/agents/" + explorerName + ".md is missing"
}

// ── the launch words ─────────────────────────────────────────────────────────

// piExtensionArgs is _pi_extension_args: an explicit extension set, nothing discovered. It dies (bash: in the caller's
// subshell, which `|| exit 1` turns into the launch's own end) when the adapter or, on a launch, herdr's pi integration
// is not on disk. pi-subagents is part of the set when it can carry the guard to its children.
func (d *dsp) piExtensionArgs() string {
	piHerdr := d.piAgentDir() + "/extensions/herdr-agent-state.ts"
	piExt := d.vdRoot() + "/setup/pi/hw-executor.ts"
	piSub := d.piSubagentsDir() + "/index.js"
	if vdSpace(piExt + piHerdr + piSub) {
		d.die("pi's extension paths cannot travel as one word each: " + piExt + ", " + piHerdr + ", " + piSub)
	}
	if !isReadable(piExt) {
		d.die("hw's pi adapter is not on disk at " + piExt + " — a pi executor is not launched without its write guard and turn-end hook. NOTHING was built.")
	}
	subArgs := ""
	if d.piSubagentsReady() {
		subArgs = " -e " + piSub
	} else if isReadable(piSub) {
		// bash's `_PI_SUBAGENTS_WARNED` is set inside a `$(…)`, so it never survives: the warning is said every call.
		d.warn("pi-subagents (" + piSub + ") has no src/api/required-child-extensions.js: its children could not carry the write guard, so it is NOT loaded and this pi executor has no subagent tool. Update it with: pi install npm:pi-subagents")
	} else {
		d.warn("pi-subagents is not installed (" + piSub + "): this pi executor has no subagent tool. Install it with: pi install npm:pi-subagents")
	}
	switch {
	case isReadable(piHerdr):
		return "-ne -na -e builtin:mcp -e builtin:codemode -e builtin:tool-search -e " + piHerdr + subArgs + " -e " + piExt + " --exclude-tools mcp__engram__*"
	case d.dispatchDry:
		d.warn("herdr's pi integration is not installed (" + piHerdr + "): a launch would refuse. Install it with: herdr integration install pi")
		return "-ne -na -e builtin:mcp -e builtin:codemode -e builtin:tool-search" + subArgs + " -e " + piExt + " --exclude-tools mcp__engram__*"
	}
	d.die("herdr's pi integration is not installed (" + piHerdr + "), so the pane would never report working/idle and no delivery could be proved. Install it with: herdr integration install pi. NOTHING was built.")
	return ""
}

// piSkill is one "<name> <dir>" line of _pi_skills.
type piSkill struct{ name, dir string }

// piSkillDirFor is _pi_skill_dir: the first directory holding <name>/SKILL.md.
func (d *dsp) piSkillDirFor(n string) (string, bool) {
	for _, base := range []string{d.env("HOME") + "/.claude/skills", d.env("HOME") + "/.agents/skills", d.vdRoot() + "/setup/.claude/skills"} {
		if isReadable(base + "/" + n + "/SKILL.md") {
			return base + "/" + n, true
		}
	}
	return "", false
}

// piProjectSkills is _pi_project_skills: a product lane's OWN skills, named one by one from the lane's checkout (the
// worktree does not exist yet when the arguments are built). loud is whether bash's warning reaches the terminal:
// every caller but the launch's prepare step discards it (`2>/dev/null`).
func (d *dsp) piProjectSkills(loud bool) []piSkill {
	if d.c.t.get(d.proj, "product_repo") != "1" {
		return nil
	}
	dir := d.c.checkout(d.proj) + "/.claude/skills"
	if !isDir(dir) {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range ents { // the glob "$d"/*/ : directories (symlinks to them too), nothing hidden
		if strings.HasPrefix(e.Name(), ".") || !isDir(dir+"/"+e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	var out []piSkill
	for _, n := range sortLocale(names, false) {
		if strings.HasPrefix(n, "speckit-") || n == "agent-browser" {
			continue
		}
		s := dir + "/" + n + "/"
		if !isReadable(s + "SKILL.md") {
			continue
		}
		if vdSpace(s) {
			if loud {
				d.warn("pi project skill '" + n + "' lives at a path with whitespace (" + s + "): it cannot travel as one word, skipped")
			}
			continue
		}
		out = append(out, piSkill{n, strings.TrimSuffix(s, "/")})
	}
	return out
}

// piDecidedSkills is _pi_decided_skills: the skills hw decides from the brief (judgment-day always, agent-browser when the
// brief declares `requires: browser`). The brief is read HERE, not from BRIEF_REQUIRES: the argument list is built before
// the contract check sets it.
func (d *dsp) piDecidedSkills(loud bool) []piSkill {
	names := []string{"judgment-day"}
	req := ""
	if d.brief != "" {
		var vals []string
		for _, l := range strings.Split(brBriefDeclares(d.brief), "\n") {
			if f := strings.Split(l, "\t"); f[0] == "requires" {
				v := ""
				if len(f) > 1 {
					v = f[1]
				}
				vals = append(vals, v)
			}
		}
		req = strings.Join(vals, "\n")
	}
	if strings.Contains(strings.ReplaceAll(req, " ", ""), "browser") {
		names = append(names, "agent-browser")
	}
	var out []piSkill
	for _, n := range names {
		dir, ok := d.piSkillDirFor(n)
		if !ok {
			if loud {
				d.warn("pi skill '" + n + "' is not on disk (looked in ~/.claude/skills, ~/.agents/skills, " + d.vdRoot() + "/setup/.claude/skills): this executor goes without it")
			}
			continue
		}
		if vdSpace(dir) {
			if loud {
				d.warn("pi skill '" + n + "' lives at a path with whitespace (" + dir + "): it cannot travel as one word, skipped")
			}
			continue
		}
		out = append(out, piSkill{n, dir})
	}
	return out
}

// piSkills is _pi_skills: the decided ones, then the lane's own.
func (d *dsp) piSkills(loud bool) []piSkill {
	return append(d.piDecidedSkills(loud), d.piProjectSkills(loud)...)
}

// piProjectSkillDir is _pi_project_skill_dir: the directory of symlinks one `--skill` argument names (a launch line holds
// 1024 bytes: 13 paths cut pi's launch in the middle of the eleventh). Named by the hash of its contents; nothing is created.
func (d *dsp) piProjectSkillDir() (string, bool) {
	skills := d.piProjectSkills(false)
	if len(skills) == 0 {
		return "", false
	}
	var b strings.Builder
	for _, s := range skills {
		b.WriteString(s.name + " " + s.dir + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	root := orDefaultStr(d.env("HW_PI_SKILLS_ROOT"), d.env("HOME")+"/.local/share/hw")
	return root + "/pi-skills/" + hex.EncodeToString(sum[:])[:16], true
}

// piSkillArgs is _pi_skill_args: `-ns` (discovery off), each decided skill, the project skills' one directory, and each
// installed plugin the brief declares as `-e <plugin dir>`.
func (d *dsp) piSkillArgs() string {
	out := ""
	for _, s := range d.piDecidedSkills(true) {
		out += " --skill " + s.dir
	}
	if dir, ok := d.piProjectSkillDir(); ok {
		out += " --skill " + dir
	}
	for _, n := range d.pluginsDeclaredInstalled() {
		out += " -e " + d.pluginsDir() + "/" + n
	}
	return "-ns" + out
}

// piPluginsLineCheck is _plugins_pi_line_check: a launch line is typed into a tty that holds 1024 bytes, and a truncated
// line is a pi that never starts. Refused with the number, before anything is built.
func (d *dsp) piPluginsLineCheck() {
	if len(d.pluginsDeclaredInstalled()) > 0 && utf8.RuneCountInString(d.agentArgs) > 1000 {
		d.die("pi's launch line would be " + itoa(utf8.RuneCountInString(d.agentArgs)) + " bytes with the brief's plugins, and the tty herdr types it into holds 1024: the end would be dropped and pi would not start. Declare fewer plugins. NOTHING was built.")
	}
}

// piBuildArgs is the `pi)` arm of _build_agent_args: the vendor's command line, as one string of words (with the leading
// space bash leaves, which the caller strips).
func (d *dsp) piBuildArgs(perm string) {
	// pi has NO permission prompts to skip or keep: its tools run unasked, and the only gate is an extension's tool_call veto.
	if perm != "skip" {
		d.die("--permissions ask cannot be honoured on pi: pi has no permission prompts, only the tool_call veto hw's guard already uses. Drop --permissions ask, or use --agent claude.")
	}
	if d.model != "" {
		switch {
		case strings.HasPrefix(d.model, "-") || strings.ContainsAny(d.model, " \t\n*?["):
			d.die("hw cannot safely pass this pi model as one literal argument (got: " + d.model + ")")
		case strings.Contains(d.model, "/"):
			d.agentArgs += " --model " + d.model
		default:
			d.die("pi needs provider/model, e.g. opencode-go/deepseek-v4-pro (got: " + d.model + "). List them with: pi --list-models opencode-go")
		}
	}
	// THINKING BY KIND when --effort is not given: explore off, everything else low; never high by default. A fresh launch
	// only: a reused pane keeps its own, and a defaulted EFFORT would read as a requested one a live session cannot honour.
	if d.effort == "" && d.reusePane == "" {
		kind := d.vdBriefKind()
		d.effort = "low"
		if kind == "explore" {
			d.effort = "off"
		}
		d.piEffortSrc = "default; pi roster, kind " + kind
	}
	switch d.effort {
	case "":
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
		d.agentArgs += " --thinking " + d.effort
	default:
		d.die("pi --effort must be one of: off minimal low medium high xhigh max (got: " + d.effort + ")")
	}
	ext := d.piExtensionArgs()
	d.agentArgs += " " + ext + " " + d.piSkillArgs()
	d.piPluginsLineCheck()
}

// ── the refusals ─────────────────────────────────────────────────────────────

// piRefuseSdd is the `pi:*` arm of _validate_sdd_vendor_pair: pi is none-mode only. An UNTYPED --sdd resolves to speckit
// from the speckit-* skills on disk in a product lane; the refusal then says so, or it names a flag nobody passed.
func (d *dsp) piRefuseSdd(requested, surface string) {
	why := ""
	if requested == "" {
		why = " (not typed: resolved from the speckit-* skills on disk in " + surface + ")"
	}
	d.die("pi framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd " + d.sddEffectiveMode + why + " has no pi entry (pi runs --sdd none only). Nothing was built. Use --sdd none, or --agent claude for a framework flow.")
}

// ── the manifest ─────────────────────────────────────────────────────────────

// piSpecialistNames is the tail of _specialist_agents: `fd` output to the names the specialists go by.
func piSpecialistNames(out string) string {
	var names []string
	for _, ln := range strings.Split(out, "\n") {
		if ln == "" {
			continue
		}
		if i := strings.LastIndexByte(ln, '/'); i >= 0 {
			ln = ln[i+1:]
		}
		ln = strings.TrimSuffix(ln, ".md")
		switch {
		case strings.HasPrefix(ln, "sdd-"), strings.HasPrefix(ln, "speckit-"), strings.HasPrefix(ln, "review-"), strings.HasPrefix(ln, "jd-"):
			continue
		}
		names = append(names, ln)
	}
	if len(names) == 0 {
		return ""
	}
	if len(names) > 1 {
		names = sortLocale(names, false)
	}
	return strings.Join(names, " ")
}

// piSpecialistAgents is _specialist_agents for the dry run: the specialist subagents on disk in <dir>/.claude/agents.
func (d *dsp) piSpecialistAgents(dir string) string {
	if !isDir(dir + "/.claude/agents") {
		return ""
	}
	r := extRun(extCall{Name: "fd", Args: []string{"-t", "f", "-t", "l", "-e", "md", ".", dir + "/.claude/agents"}, Env: childEnv()})
	return piSpecialistNames(r.Out)
}

// piDelegableAgents is _pi_delegable_agents: which of the specialists pi CAN delegate to — the ones this worktree also
// tracks as a pi agent AND that hw made reachable (pi-subagents loaded, PI_SUBAGENT_EXTRA_AGENT_DIRS pointing at them).
func (d *dsp) piDelegableAgents(work, list string) string {
	if work == "" || d.piExtraAgentDir(work) == "" || !d.piSubagentsReady() {
		return ""
	}
	var out []string
	for _, n := range strings.Fields(list) {
		if isFile(work + "/.pi/agents/" + n + ".md") {
			out = append(out, n)
		}
	}
	return strings.Join(out, " ")
}

// piRegisteredAgents: the specialists hw-executor.ts registers with pi-subagents itself — every one of the list that no
// tracked `.pi/agents` copy provides (a copy is loaded by pi-subagents and wins), when hw passed it the directory.
func (d *dsp) piRegisteredAgents(work, list string) string {
	if d.piClaudeAgentDirs(work) == "" {
		return ""
	}
	have := strings.Fields(d.piDelegableAgents(work, list))
	var out []string
	for _, n := range strings.Fields(list) {
		if !has(have, n) {
			out = append(out, n)
		}
	}
	return strings.Join(out, " ")
}

// piOpsProcedureFile is _pi_ops_procedure_file: the brainer's copy of ops-investigator's definition, "" when absent.
func (d *dsp) piOpsProcedureFile() string {
	f := d.c.brain + "/" + d.proj + "/.claude/agents/ops-investigator.md"
	if d.proj != "" && isFile(f) {
		return f
	}
	return ""
}

// piManifestLines is PI_LINES of _dispatch_manifest, leading newline included: pi's own fields, only for pi, so every
// other vendor's manifest stays byte-identical. dir is the manifest's own argument.
func (d *dsp) piManifestLines(dir string) string {
	fb := d.piFallbackModel()
	var names []string
	for _, s := range d.piSkills(false) {
		names = append(names, s.name)
	}
	var sub string
	if d.piSubagentsReady() {
		sub = "pi-subagents loaded (-e " + d.piSubagentsDir() + "/index.js); the write guard is a REQUIRED child extension, so a subagent is guarded like its parent"
	} else {
		sub = "NOT LOADED — pi-subagents is not installed under " + d.piSubagentsDir() + ", or lacks src/api/required-child-extensions.js, so a child could not carry the write guard (pi install npm:pi-subagents)"
	}
	proc := d.piOpsProcedureFile()
	// A dry run has no worktree yet: the lane's checkout is the stand-in for what the worktree will carry.
	co := orDefaultStr(d.c.checkout(d.proj), dir)
	specs := d.piSpecialistAgents(co)
	deleg := d.piDelegableAgents(co, specs)
	reg := d.piRegisteredAgents(co, specs)
	var spec string
	switch {
	case deleg == "" && reg == "" && specs == "":
		spec = "the lane has no .claude/agents specialists to delegate to"
	case deleg == "" && reg == "":
		spec = "the lane's .claude/agents are NOT delegable: pi-subagents is not loaded (see the pi subagents line), and the preamble says so"
	default:
		var parts []string
		if reg != "" {
			parts = append(parts, "REGISTERED with pi-subagents by hw-executor.ts at session start: "+reg+" (read from "+d.piClaudeAgentDirs(co)+" via HW_PI_CLAUDE_AGENT_DIRS; Claude's tool names mapped to pi's in memory, the executor's model instead of the file's model:, nothing copied or rewritten)")
		}
		if deleg != "" {
			parts = append(parts, "DELEGABLE through pi-subagents: "+deleg+" (the worktree's tracked .pi/agents, reached by PI_SUBAGENT_EXTRA_AGENT_DIRS because -na leaves the project untrusted; every call must pass model, their files pin openai-codex)")
		}
		spec = strings.Join(parts, "; ")
	}
	if proc != "" {
		shown := proc
		if home := d.env("HOME"); home != "" && strings.HasPrefix(proc, home) {
			shown = "~" + proc[len(home):]
		}
		spec += "; the ops-investigator procedure is injected inline (from " + shown + ")"
	} else {
		spec += "; no ops-investigator procedure in this lane's brain directory, so none is injected"
	}
	fbLine := "none (the model already is the roster fallback)"
	if fb != "" {
		fbLine = fb + "  (HW_PI_FALLBACK_MODEL, thinking " + piFallbackThinking + ": a terminal MODEL error switches to it by itself and writes the switch in the receipt; a spent QUOTA does not — the cap is shared dollars)"
	}
	skills := orDefaultStr(strings.Join(names, ", "), "none")
	return "\n  pi fallback  " + fbLine + "\n  pi skills    " + skills + "  (-ns: discovery off; only what hw decides, not ~/.pi/agent/skills)\n  pi subagents " + sub + "\n  pi specialists " + spec
}
