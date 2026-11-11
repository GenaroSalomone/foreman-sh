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
	"sort"
	"strings"
	"unicode/utf8"
)

// piState is what the pi arms keep of the dispatch (bash: PI_EFFORT_SRC).
type piState struct {
	// effortSrc is what supplied a level nobody asked for, and "" when --effort chose it (bash: PI_EFFORT_SRC). A
	// field of the dispatch, not of pi: the manifest's effort line names it in the same words for every vendor
	// (t5_manifest.go effortShownAndMark). pi is the only vendor hw declares a default level for.
	effortSrc string
	// piRunDone/piRun/piReal memoize piRunDirs: the env block and the pane split are one answer, whichever runs first
	piRunDone     bool
	piRun, piReal string
}

// THE pi ROSTER (opencode-go), the operator's approval of 2026-10-11, moved by the brainer's ruling of 2026-11-01
// (setup/decisions.md): build and design/audit run deepseek-v4.1-flash, and fall to v4-pro on a MODEL error (never on a
// spent quota: the cap is shared dollars); review keeps glm-5.3-flash with that same v4-pro fallback; explore is
// unchanged and mechanical, on the build model, moves with it but keeps the flash fallback. Thinking stays by kind
// (explore off, everything else low). MEASURED 2026-11-04, pi's get_state over RPC mode, per requested level on
// deepseek-v4.1-flash: off→low, minimal→low, low→low, medium→high, high→high, xhigh→max, max→max. THREE rungs are real —
// low, high and max — not two: the 2026-11-01 ruling measured only up to high, and xhigh/max are not clamped away. `off`
// and `minimal` are still clamped up to low. See piThinkingMeasured.
const (
	piModelDesign      = "opencode-go/deepseek-v4.1-flash"
	piModelBuild       = "opencode-go/deepseek-v4.1-flash"
	piModelReview      = "opencode-go/glm-5.3-flash"
	piModelCheap       = "opencode-go/deepseek-v4.1-flash"
	piFallbackFlash    = "opencode-go/glm-5.3-flash"
	piFallbackPro      = "opencode-go/deepseek-v4-pro"
	piFallbackThinking = "low"

	// piExplorerModel is HW_EXPLORER_MODEL_PI: the roster's `explore` model.
	piExplorerModel = "opencode-go/deepseek-v4.1-flash"

	piEngramProxyLine = "pi extension hw_engram (setup/pi/hw-executor.ts; raw mcp__engram__* refused)"
)

// THE SUBAGENTS' TIERS. A Claude agent file names an alias (haiku, sonnet, opus), never a pi model, and pi-subagents'
// runtime-agent API takes a model and a thinking level per agent. These are the pi models an alias resolves to when
// the lane names no profile: the roster's own ladder, cheapest first. Costs measured in the model catalog 2026-11-04,
// per Mtok in/out: deepseek-v4.1-flash 0.15/0.60, glm-5.3-flash 0.15/0.50, deepseek-v4-pro 0.66/1.98 — cost alone
// does not separate the two flashes, so the split follows the roster's roles (build/explore flash, review glm,
// fallback pro) and the thinking levels follow the shape of 2026-11-04: explore off/low, build and review medium/high.
const (
	piSubagentHaiku  = "opencode-go/deepseek-v4.1-flash"
	piSubagentSonnet = "opencode-go/glm-5.3-flash"
	piSubagentOpus   = "opencode-go/deepseek-v4-pro"
)

// THE TWO NAMED ROSTERS. `go` is the paid one above, unchanged; `free` is opencode's free pair (operator, 2026-11-09:
// the paid quota is spent and pi is used only to test pi itself). projects.json's top-level `pi_roster` picks one, and
// returning to the paid roster is changing that word, no code. MEASURED 2026-11-09, one `pi -p` per level: both free
// models accept off/minimal/low/medium/high/xhigh and run a `read` tool call; what pi clamps each level to was NOT
// measured on them. iceman-contributor-free is out (400: it trains on request data). The free pair back each other up.
// A lane's own pi_subagent_models rows name paid models, so they apply to `go` only.
type piRoster struct {
	name                                 string
	design, build, review, cheap         string
	fallbackFlash, fallbackPro, explorer string
	haiku, sonnet, opus                  string
	agents                               map[string]piTier // per-agent rows that outrank the agent's file (free: the explorer's file pins a paid model)
	fallbackSwap                         string            // used when the kind's fallback is the model that runs (free: the pair back each other up; go: none)
	laneRows                             bool
}

const (
	piFreeCrouton = "opencode-go/crouton-alpha-free"
	piFreeBeam    = "opencode-go/beam-free"
)

var piRosters = map[string]piRoster{
	"go": {"go", piModelDesign, piModelBuild, piModelReview, piModelCheap, piFallbackFlash, piFallbackPro, piExplorerModel,
		piSubagentHaiku, piSubagentSonnet, piSubagentOpus, nil, "", true},
	"free": {"free", piFreeCrouton, piFreeCrouton, piFreeBeam, piFreeCrouton, piFreeBeam, piFreeBeam, piFreeCrouton,
		piFreeCrouton, piFreeBeam, piFreeCrouton, map[string]piTier{explorerName: {piFreeCrouton, "off"}}, piFreeCrouton, false},
}

// piRosterName is the roster projects.json's `pi_roster` names; absent means `go`, what hw did before the key existed.
const piRosterDefault = "go"

// piTier is what one alias resolves to: the pi model and the thinking level its child is started with ("inherit" =
// pi's own default for that model).
type piTier struct{ Model, Thinking string }

// piTierNames are the Claude aliases a pi dispatch resolves; `fable` and a claude-* id have no pi mapping and are said
// so in the receipt rather than guessed.
var piTierNames = []string{"haiku", "sonnet", "opus"}

func piDefaultTiers(r piRoster) map[string]piTier {
	return map[string]piTier{
		"haiku":  {r.haiku, "off"},
		"sonnet": {r.sonnet, "medium"},
		"opus":   {r.opus, "high"},
	}
}

// piSubagentProfile is the active roster's tiers, then (roster `go`) the lane's `pi_subagent_models` over them: `alias=model|thinking` pairs, keyed by
// tier or by an agent's name (an agent entry outranks its file).
func (d *dsp) piSubagentProfile() map[string]piTier {
	r := d.c.t.piRoster()
	prof := piDefaultTiers(r)
	for k, v := range r.agents {
		prof[k] = v
	}
	laneRows := ""
	if r.laneRows {
		laneRows = d.c.t.get(d.proj, "pi_subagent_models")
	}
	for _, pair := range vdIFSFields(laneRows) {
		k, v, _ := strings.Cut(pair, "=")
		if k == "" || v == "" {
			continue
		}
		model, thinking, _ := strings.Cut(v, "|")
		prof[k] = piTier{orDefaultStr(model, "inherit"), orDefaultStr(thinking, "inherit")}
	}
	return prof
}

// piSubagentResolve is the resolution order of 2026-11-04, per agent: the lane's entry for the agent's name, then the
// entry for the alias its file names, then a pi id the file names itself, then the lane's `subagent_model` (the
// default). ok=false means the specialist runs on the executor's model, and why names what had no pi mapping.
func (d *dsp) piSubagentResolve(agent, fileModel string) (tier piTier, why string, ok bool) {
	prof := d.piSubagentProfile()
	if t, found := prof[agent]; found && agent != "" && !has(piTierNames, agent) {
		return t, "the lane's pi_subagent_models entry for " + agent, t.Model != "" && t.Model != "inherit"
	}
	alias := strings.ToLower(strings.TrimSpace(fileModel))
	if alias != "" {
		if t, found := prof[alias]; found && has(piTierNames, alias) {
			return t, "the file's model: " + alias, t.Model != "" && t.Model != "inherit"
		}
		if strings.Contains(alias, "/") && rePiID.MatchString(alias) {
			return piTier{Model: alias, Thinking: "inherit"}, "the pi model the file names", true
		}
		return piTier{}, "the file's model: " + alias + " has no pi mapping", false
	}
	// no model: in the file — the lane's subagent default, Claude's CLAUDE_CODE_SUBAGENT_MODEL
	sm := orDefaultStr(d.subagentModel, orDefaultStr(d.c.t.get(d.proj, "subagent_model"), "inherit"))
	if t, found := prof[sm]; found && has(piTierNames, sm) {
		return t, "the lane's subagent_model: " + sm, t.Model != "" && t.Model != "inherit"
	}
	return piTier{}, "the lane's subagent_model: " + sm + " has no pi mapping", false
}

// piSubagentModelsEnv is HW_PI_SUBAGENT_MODELS: the whole profile as `key=model|thinking` pairs plus `default=<tier>`
// (the tier an agent with no model: runs on; `inherit` for none). hw-executor.ts registers each specialist with the
// model and thinking this resolves, so the split Claude has by alias holds on pi. Sorted, so the same lane exports the
// same bytes.
func (d *dsp) piSubagentModelsEnv() string {
	prof := d.piSubagentProfile()
	keys := make([]string, 0, len(prof))
	for k := range prof {
		if k == "default" { // reserved: the lane's subagent_model decides it, never a profile key
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+orDefaultStr(prof[k].Model, "inherit")+"|"+orDefaultStr(prof[k].Thinking, "inherit"))
	}
	sm := orDefaultStr(d.subagentModel, orDefaultStr(d.c.t.get(d.proj, "subagent_model"), ""))
	def := "inherit"
	if has(piTierNames, sm) {
		def = sm
	}
	return strings.Join(append(parts, "default="+def), ",") // no blanks: hwEnvArgs word-splits its --env values
}

// piSubagentTiersLine is the profile as the manifest and the preamble say it: every tier with the model and thinking it
// resolves, every agent the lane classifies by name (the explore class cheap, the build/review class strong), and what an
// agent with no model: runs on.
func (d *dsp) piSubagentTiersLine() string {
	prof := d.piSubagentProfile()
	var parts []string
	for _, k := range piTierNames {
		if t, ok := prof[k]; ok {
			parts = append(parts, k+"→"+t.Model+"|"+t.Thinking)
		}
	}
	var agents []string
	for k := range prof {
		if !has(piTierNames, k) && k != "default" {
			agents = append(agents, k)
		}
	}
	sort.Strings(agents)
	var byAgent []string
	for _, k := range agents {
		byAgent = append(byAgent, k+"→"+prof[k].Model+"|"+prof[k].Thinking)
	}
	def := "the executor's"
	if sm := orDefaultStr(d.subagentModel, orDefaultStr(d.c.t.get(d.proj, "subagent_model"), "")); has(piTierNames, sm) {
		def = "subagent_model " + sm
	}
	out := strings.Join(parts, ", ") + "; no model: → " + def
	if len(byAgent) > 0 {
		out += "; by agent " + strings.Join(byAgent, ", ")
	}
	return out
}

// piThinkingMeasured is what pi REALLY runs per requested level, per model: MEASURED 2026-11-04 with pi's own get_state in
// RPC mode (no prompt, so no model call), the reading the 2026-11-01 ruling took with pi.getThinkingLevel(). A level the
// model does not expose is not refused — pi runs the nearest one it has, which is why `off` is a REQUEST and the receipt
// compares it with what ran (t6_pi_launch.go piReceiptThinking).
var piThinkingMeasured = map[string]map[string]string{
	"deepseek-v4.1-flash": {"off": "low", "minimal": "low", "low": "low", "medium": "high", "high": "high", "xhigh": "max", "max": "max"},
}

// piThinkingRuns is the level pi runs when asked for `asked` on `model`: the level, and whether it is measured at all
// (an unmeasured model is not guessed at).
func piThinkingRuns(model, asked string) (string, bool) {
	if asked == "" {
		return "", false
	}
	name := model
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, ":"); i >= 0 {
		name = name[:i]
	}
	levels, ok := piThinkingMeasured[name]
	if !ok {
		return "", false
	}
	runs, ok := levels[asked]
	return runs, ok
}

// piThinkingAboveRuling is a level above the pi-roster ruling of 2026-11-01 (build low, design/audit high): nothing has
// measured xhigh or max for this lane's work, and the ruling's numbers — 120 runs, 18 models (setup/decisions.md) — do not
// cover them. They run, as max; the warning says the choice is unmeasured, not forbidden.
func piThinkingAboveRuling(level string) bool {
	switch level {
	case "xhigh", "max":
		return true
	}
	return false
}

// piApplyModelDefault is _apply_pi_model_default: no lane pins a pi model (a lane's own pin is its vendor's id, cleared
// for an explicit --agent pi), so the roster decides by the brief's kind.
func (d *dsp) piApplyModelDefault() {
	r := d.c.t.piRoster()
	kind := d.vdBriefKind()
	switch kind {
	case "design", "audit":
		d.model = r.design
	case "review":
		d.model = r.review
	case "explore":
		d.model = r.cheap
	default:
		d.model = r.build
	}
	d.modelSrc = "default; pi roster, kind " + orDefaultStr(kind, "none")
}

// piFallbackModel is _pi_fallback_model: the model a pi executor of this dispatch falls to on a MODEL error, "" when the
// model already is the fallback. Decided by the KIND, also for an explicit --model.
func (d *dsp) piFallbackModel() string {
	r := d.c.t.piRoster()
	fb := r.fallbackFlash
	switch d.vdBriefKind() {
	// the kinds on the flash default (2026-11-01) fall to the pro, a kindless brief being a build; review has always
	// fallen to it; explore and mechanical keep the flash fallback they had.
	case "", "build", "design", "audit", "review":
		fb = r.fallbackPro
	}
	if fb == d.model {
		fb = r.fallbackSwap
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

// piLaneClaudeAgentDir is the `.claude/agents` directory a Claude executor of this lane sees, the worktree's. A lane with
// no product repo has it linked at launch from its brain directory (linkLaneClaude), so before the worktree exists the
// brain's own directory stands for it. "" when pi-subagents is not loaded (nothing to register with) or the path cannot
// travel in a PATH-style list.
func (d *dsp) piLaneClaudeAgentDir(work string) string {
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

// piUserClaudeAgentDir is the account's user-level agents directory ($CLAUDE_CONFIG_DIR/agents, default ~/.claude/agents):
// where a Claude executor finds jd-judge-a, jd-judge-b and jd-fix-agent. "" when it is absent or cannot travel in a
// PATH-style list.
func (d *dsp) piUserClaudeAgentDir() string {
	dir := orDefaultStr(d.env("CLAUDE_CONFIG_DIR"), d.env("HOME")+"/.claude") + "/agents"
	if !isDir(dir) || strings.ContainsAny(dir, " \t\n\v\f\r:") {
		return ""
	}
	return dir
}

// piClaudeAgentDirs is the PATH-style list of `.claude/agents` directories hw-executor.ts registers with pi-subagents
// (HW_PI_CLAUDE_AGENT_DIRS), in the order a Claude executor reads them: the lane's, then the account's user-level one.
// "" when pi-subagents is not loaded (nothing to register with).
func (d *dsp) piClaudeAgentDirs(work string) string {
	if !d.piSubagentsReady() {
		return ""
	}
	var dirs []string
	for _, dir := range []string{d.piLaneClaudeAgentDir(work), d.piUserClaudeAgentDir()} {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return strings.Join(dirs, ":")
}

// declaredPluginAgentDirs is every declared, installed plugin's agents/ directory, whatever pi-subagents' state: what hw
// would name, and what says the review's agent piece has no door here when pi-subagents is not loaded.
func (d *dsp) declaredPluginAgentDirs() []string {
	var out []string
	for _, n := range d.pluginsDeclaredInstalled() {
		if dir := d.pluginsDir() + "/" + n + "/agents"; isDir(dir) {
			out = append(out, n+"="+dir)
		}
	}
	return out
}

// piPluginAgentDirs is HW_PI_PLUGIN_AGENT_DIRS: `<plugin>=<agents dir>` for every plugin the brief declares (`plugins:`, or
// a `review:` list, which loads them too) that ships an agents/ directory. hw-executor.ts registers each file under
// `<plugin>:<name>`, the name a Claude executor reads from --plugin-dir — pstack's comment-sicko was `Unknown agent` on pi
// (2026-11-09). Empty when pi-subagents is not loaded (nothing to register with) or no declared plugin carries agents.
func (d *dsp) piPluginAgentDirs() []string {
	if !d.piSubagentsReady() {
		return nil
	}
	var out []string
	for _, one := range d.declaredPluginAgentDirs() {
		if strings.ContainsAny(one, " \t\n\v\f\r:") {
			continue
		}
		out = append(out, one)
	}
	return out
}

// piPluginAgentDirsEnv is piPluginAgentDirs as the child's environment carries it (`:` between entries); "" for none.
func (d *dsp) piPluginAgentDirsEnv() string { return strings.Join(d.piPluginAgentDirs(), ":") }

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

// piPluginSkills is what one installed plugin ships as skills: the entries of its `skills/` that hold a SKILL.md.
func piPluginSkills(dir string) []string {
	ents, err := os.ReadDir(dir + "/skills")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if isFile(dir + "/skills/" + e.Name() + "/SKILL.md") {
			out = append(out, e.Name())
		}
	}
	return out
}

// piPluginLoad is one declared plugin's `-e`: the directory pi is handed, the plugin it came from, and the skills a
// MIRROR of it leaves out because an earlier declared plugin already ships them.
type piPluginLoad struct {
	dir, src string
	shadow   []string
}

// piPluginLoads is the `-e` directory of every declared plugin. pi keeps the first copy of a skill NAME and prints a
// `[Skill warnings]` block naming the one it skipped (interactive-mode.js showLoadedResources), so a launch declaring
// thermo and pstack — both ship thermo-nuclear-code-quality-review — opens on a warning instead of on the task. A
// colliding plugin is handed as a per-run mirror, one `skills/` entry short, so nothing vendored changes and the
// plugin keeps its own copy for a launch that declares it alone. The mirror is named by the hash of its set AND its
// path, like piProjectSkillDir, and piPluginMirrorsPrepare makes it at launch (never by a dry run): a plugin that gains
// or loses a skill, or a checkout that moved, gets a new directory instead of a stale link.
//
// shortcut: the mirror leaves out an entry of `<plugin>/skills`, which is where the shipped three declare their skills
// (`package.json` `pi.skills`); a plugin of the operator's own that declares another directory keeps its duplicate and
// pi's warning stays. Read that field when a plugin of the operator's is seen colliding. The mirror lives under
// $HOME/.local/share/hw and a REVIVE does not rebuild it (cmd_revive carries no brief, so it names no plugins:
// t4_revive_start.go, `TestRevivePiNamesTheLanesSkillsAndNoPlugins`): a revive whose ~/.local/share/hw was wiped finds
// no mirror, and the next dispatch makes it.
func (d *dsp) piPluginLoads() []piPluginLoad {
	root := orDefaultStr(d.env("HW_PI_SKILLS_ROOT"), d.env("HOME")+"/.local/share/hw") + "/pi-plugins"
	seen := map[string]bool{}
	var out []piPluginLoad
	for _, n := range d.pluginsDeclaredInstalled() {
		src := d.pluginsDir() + "/" + n
		own := piPluginSkills(src)
		load := piPluginLoad{dir: src, src: src}
		var b strings.Builder
		b.WriteString(n + "\n" + src + "\n")
		for _, s := range own {
			if seen[s] {
				load.shadow = append(load.shadow, s)
				b.WriteString("!" + s + "\n")
				continue
			}
			b.WriteString(s + "\n")
		}
		for _, s := range own {
			seen[s] = true
		}
		if len(load.shadow) > 0 {
			sum := sha256.Sum256([]byte(b.String()))
			load.dir = root + "/" + n + "-" + hex.EncodeToString(sum[:])[:16]
		}
		out = append(out, load)
	}
	return out
}

// piSkillArgs is _pi_skill_args: `-ns` (discovery off), each decided skill, the project skills' one directory, and each
// installed plugin the brief declares as `-e <plugin dir>` (or the mirror of it, piPluginLoads).
func (d *dsp) piSkillArgs() string {
	out := ""
	for _, s := range d.piDecidedSkills(true) {
		out += " --skill " + s.dir
	}
	if dir, ok := d.piProjectSkillDir(); ok {
		out += " --skill " + dir
	}
	for _, pl := range d.piPluginLoads() {
		out += " -e " + pl.dir
	}
	return "-ns" + out
}

// piLaneRulesArgFor is the `--append-system-prompt <file>` a pi executor of this lane needs: pi reads only
// AGENTS.override.md|AGENTS.md|CLAUDE.md directly in each ancestor directory, never .claude/CLAUDE.md, and the flag
// takes a path when it names an existing file. MEASURED on pi 1.1.0 in a throwaway HOME/PI_*: the appended file's first
// heading was quoted with the flag and absent without it. "" when the lane has no such file, or its path cannot travel
// as one word on the launch line.
func piLaneRulesArgFor(brain, proj string) string {
	f := laneRulesFile(brain, proj)
	if f == "" || strings.ContainsAny(f, " \t\n") {
		return ""
	}
	return " --append-system-prompt " + f
}

// piLaneRulesArg is piLaneRulesArgFor for the dispatch's lane.
func (d *dsp) piLaneRulesArg() string { return piLaneRulesArgFor(d.c.brain, d.proj) }

// piPluginsLineCheck is _plugins_pi_line_check: a launch line is typed into a tty that holds 1024 bytes, and a truncated
// line is a pi that never starts. Refused with the number, before anything is built.
func (d *dsp) piPluginsLineCheck() {
	if len(d.pluginsDeclaredInstalled()) > 0 && utf8.RuneCountInString(d.agentArgs) > 1000 {
		d.die("pi's launch line would be " + itoa(utf8.RuneCountInString(d.agentArgs)) + " bytes with the brief's plugins, and the tty herdr types it into holds 1024: the end would be dropped and pi would not start. Declare fewer plugins. NOTHING was built.")
	}
}

// piTestingOnlyWarn: under the `free` roster pi is for testing pi itself (operator, 2026-11-09). A lane dispatch is
// warned, never refused; a task with a test or probe word in its name (hyphen-delimited) is that testing, so it stays quiet.
func (d *dsp) piTestingOnlyWarn() {
	if d.c.t.piRoster().name != "free" || has(strings.Split(d.task, "-"), "test") || has(strings.Split(d.task, "-"), "probe") {
		return
	}
	d.warn("pi runs on opencode's free models for testing only (roster free; the paid quota is spent): use it to test pi itself, not for real lane work")
}

// piBuildArgs is the `pi)` arm of _build_agent_args: the vendor's command line, as one string of words (with the leading
// space bash leaves, which the caller strips).
func (d *dsp) piBuildArgs(perm string) {
	// pi has NO permission prompts to skip or keep: its tools run unasked, and the only gate is an extension's tool_call veto.
	if perm != "skip" {
		d.die("--permissions ask cannot be honoured on pi: pi has no permission prompts, only the tool_call veto hw's guard already uses. Drop --permissions ask, or use --agent claude.")
	}
	d.piTestingOnlyWarn()
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
	// The level and the words come from effortDeclaredDefault, the one rule (t5_vendor_agent.go): the manifest prints
	// exactly what this chose.
	if d.effort == "" && d.reusePane == "" {
		if lvl, src, ok := d.effortDeclaredDefault(); ok {
			d.effort, d.effortSrc = lvl, src
		}
	}
	if msg := effortRefusal(d.agent, d.effort); msg != "" {
		d.die(msg)
	}
	if d.effort != "" {
		d.agentArgs += " --thinking " + d.effort
	}
	ext := d.piExtensionArgs()
	d.agentArgs += " " + ext + " " + d.piSkillArgs() + d.piLaneRulesArg()
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

// piFrameworkPrefixes are the agents hw's own specialist list leaves out: the framework's (sdd-, speckit-, review-) and the
// Judgment Day judges. hw-executor.ts's NOT_SPECIALIST copies this set WITHOUT `jd-` (it registers the judges); a Go test
// holds the two apart by exactly that prefix.
var piFrameworkPrefixes = []string{"sdd-", "speckit-", "review-", "jd-"}

// piIsFramework: name starts with a framework prefix; with keepJD the `jd-` judges are not framework.
func piIsFramework(name string, keepJD bool) bool {
	for _, p := range piFrameworkPrefixes {
		if keepJD && p == "jd-" {
			continue
		}
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// piSpecialistNames is the tail of _specialist_agents: `fd` output to the names the specialists go by.
func piSpecialistNames(out string) string { return piAgentNames(out, false) }

// piAgentNames is piSpecialistNames with the Judgment Day judges optional: hw's own specialist list drops `jd-` (the
// preamble owner reads it), but hw-executor.ts registers them (its NOT_SPECIALIST lacks `jd-`), so what a pi executor can
// call is listed with keepJD. 1230 holds the two sets apart by exactly this prefix.
func piAgentNames(out string, keepJD bool) string {
	var names []string
	for _, ln := range strings.Split(out, "\n") {
		if ln == "" {
			continue
		}
		if i := strings.LastIndexByte(ln, '/'); i >= 0 {
			ln = ln[i+1:]
		}
		ln = strings.TrimSuffix(ln, ".md")
		if piIsFramework(ln, keepJD) {
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
	if d.piLaneClaudeAgentDir(work) == "" {
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

// piAgentsIn is the agents hw-executor.ts can register from one `.claude/agents` directory (every file, the `jd-` judges
// included), "" when the directory is absent.
func (d *dsp) piAgentsIn(agentDir string) string {
	if agentDir == "" || !isDir(agentDir) {
		return ""
	}
	r := extRun(extCall{Name: "fd", Args: []string{"-t", "f", "-t", "l", "-e", "md", ".", agentDir}, Env: childEnv()})
	return piAgentNames(r.Out, true)
}

// piRegisteredByDir is what hw-executor.ts registers with pi-subagents, per directory it reads: the lane's agents (list,
// as piRegisteredAgents) and the account's user-level ones, minus the names a tracked `.pi/agents` copy provides and, for
// the account, the names the lane's directory already provides (the lane's file shadows the account's).
func (d *dsp) piRegisteredByDir(work, list string) (lane, account string) {
	lane = d.piRegisteredAgents(work, list)
	if !d.piSubagentsReady() {
		return lane, ""
	}
	laneAll := strings.Fields(d.piAgentsIn(d.piLaneClaudeAgentDir(work)))
	// the lane's own jd- judges are registered too, though the specialist list drops them
	var judges []string
	for _, n := range laneAll {
		if strings.HasPrefix(n, "jd-") && !has(strings.Fields(lane), n) && !has(strings.Fields(d.piDelegableAgents(work, n)), n) {
			judges = append(judges, n)
		}
	}
	lane = strings.TrimSpace(lane + " " + strings.Join(judges, " "))
	taken := laneAll
	taken = append(taken, strings.Fields(list)...)
	taken = append(taken, strings.Fields(d.piDelegableAgents(work, d.piAgentsIn(d.piUserClaudeAgentDir())))...)
	var out []string
	for _, n := range strings.Fields(d.piAgentsIn(d.piUserClaudeAgentDir())) {
		if !has(taken, n) {
			out = append(out, n)
		}
	}
	return lane, strings.Join(out, " ")
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
	// the tiers a specialist's model: resolves to, whether or not pi-subagents is loaded here
	sub += "; subagent tiers " + d.piSubagentTiersLine()
	proc := d.piOpsProcedureFile()
	// A dry run has no worktree yet: the lane's checkout is the stand-in for what the worktree will carry.
	co := orDefaultStr(d.c.checkout(d.proj), dir)
	specs := d.piSpecialistAgents(co)
	deleg := d.piDelegableAgents(co, specs)
	reg, acct := d.piRegisteredByDir(co, specs)
	var spec string
	switch {
	case deleg == "" && reg == "" && acct == "" && specs == "":
		spec = "the lane has no .claude/agents specialists to delegate to"
	case deleg == "" && reg == "" && acct == "":
		spec = "the lane's .claude/agents are NOT delegable: pi-subagents is not loaded (see the pi subagents line), and the preamble says so"
	default:
		var parts []string
		how := "Claude's tool names mapped to pi's in memory, the file's model: resolved to a pi model (see pi subagents), nothing copied or rewritten"
		switch {
		case reg != "" && acct == "":
			parts = append(parts, "REGISTERED with pi-subagents by hw-executor.ts at session start: "+reg+" (read from "+d.piLaneClaudeAgentDir(co)+" via HW_PI_CLAUDE_AGENT_DIRS; "+how+")")
		case reg != "" || acct != "":
			var from []string
			if reg != "" {
				from = append(from, reg+" (the lane's "+d.piLaneClaudeAgentDir(co)+")")
			}
			if acct != "" {
				from = append(from, acct+" (the account's "+d.piUserClaudeAgentDir()+")")
			}
			parts = append(parts, "REGISTERED with pi-subagents by hw-executor.ts at session start, via HW_PI_CLAUDE_AGENT_DIRS: "+strings.Join(from, "; ")+" ("+how+")")
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
	// the declared plugins' own agents, by the names a Claude executor reads from --plugin-dir: `pstack:comment-sicko`
	if dirs := d.piPluginAgentDirs(); len(dirs) > 0 {
		var parts []string
		for _, one := range dirs {
			plugin, dir, _ := strings.Cut(one, "=")
			var names []string
			if ents, err := os.ReadDir(dir); err == nil { // what hw-executor.ts registers: every .md, no filter
				for _, e := range ents {
					if strings.HasSuffix(e.Name(), ".md") {
						names = append(names, plugin+":"+strings.TrimSuffix(e.Name(), ".md"))
					}
				}
			}
			parts = append(parts, strings.Join(names, " ")+" (from "+dir+")")
		}
		spec += "; the declared plugins' agents, registered as <plugin>:<name> by hw-executor.ts at session start via HW_PI_PLUGIN_AGENT_DIRS: " + strings.Join(parts, "; ")
	} else if declared := d.declaredPluginAgentDirs(); len(declared) > 0 {
		// a declared agent with no door here is SAID, never dropped silently (the cross-vendor rule)
		why := "pi-subagents is not loaded (see the pi subagents line)"
		if d.piSubagentsReady() {
			why = "its agents directory path has a space or ':', which cannot travel in HW_PI_PLUGIN_AGENT_DIRS"
		}
		if pieces := reviewPiecesToInvoke(d.pluginsDeclared(), true); pieces != "" {
			spec += "; NOT LAUNCHABLE here: the review's agent " + pieces + " needs pi's subagent tool, and " + why
		}
	}
	fbLine := "none (the model already is the roster fallback)"
	if fb != "" {
		fbLine = fb + "  (HW_PI_FALLBACK_MODEL, thinking " + piFallbackThinking + ": a terminal MODEL error switches to it by itself and writes the switch in the receipt; a spent QUOTA does not — the cap is shared dollars)"
	}
	skills := orDefaultStr(strings.Join(names, ", "), "none")
	agentDir := "the ambient one (PI_CODING_AGENT_DIR, else ~/.pi/agent), untouched — no plugin is declared, so this session starts no child with discovery on"
	if len(d.pluginsDeclaredInstalled()) > 0 {
		agentDir = "PRIVATE (under $HW_ARTIFACTS/pi-agent): the ambient agent directory's files, symlinked, with a settings.json that declares no packages and no extensions — a plugin's children start pi with discovery ON (pstack's `agent`: setup/vendor/plugins/pstack/pi/agents.ts), and the ambient rpiv-ask-user-question package registers the same `ask_user_question` pstack's extension does. ~/.pi is not written"
	}
	usage := ""
	if u := d.piUsageManifest(); u != "" {
		usage = "\n  pi usage     opencode-go " + u
	}
	roster := ""
	if d.c.t.piRosterName != "" { // named only where projects.json sets it: a table without the key reads as before
		roster = "\n  pi roster    " + d.c.t.piRosterName + "  (projects.json pi_roster: go = the paid opencode-go roster, free = opencode's free models; --model still overrides the primary)"
	}
	return roster + "\n  pi fallback  " + fbLine + usage + "\n  pi skills    " + skills + "  (-ns: discovery off; only what hw decides, not ~/.pi/agent/skills)\n  pi agent dir " + agentDir + "\n  pi subagents " + sub + "\n  pi specialists " + spec
}
