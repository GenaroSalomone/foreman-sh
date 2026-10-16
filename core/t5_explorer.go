package main

// The cheap explorer every executor is handed, and the tier table the prompt states: the Go half of
// lib/hw/explorer.sh (_explorer_role_json, _explorer_claude_args, _explorer_opencode_overlay, _explorer_codex_args,
// _explorer_resolve, _explorer_tier_note), plus _check_top_tier_ceiling and model_is_top_tier. Byte for byte what
// bash prints: the contract goldens come from bash, and a launch under HW_IMPL=go must hand the same words to the
// vendor. pi is not ported to hw-core (core/t5.go refuses it), so it has no arm here.

import (
	"os"
	"regexp"
	"strings"
)

const (
	// explorerSpace is a space spelt the way JSON and TOML read back as a space: the launch words are split on whitespace.
	explorerSpace          = "\\u0020"
	explorerName           = "cheap-explorer"
	explorerModelClaude    = "haiku"
	explorerModelOpencode  = "openai/gpt-5.6-luna"
	explorerModelCodex     = "gpt-5.6-luna"
	explorerEffortOpencode = "low"
)

// explorerRole is the parsed `_agents/cheap-explorer.md`: its frontmatter and its body.
type explorerRole struct {
	name, description, prompt string
	tools                     []string
}

// explorerSrc is what the explorer functions read: the brain's root (bash's _brain_root) and the lane's pin of
// haiku (model_pins, "" when none). Shared by the dispatch, the launch and the re-task, which hold different types.
type explorerSrc struct{ root, haikuPin string }

func (d *dsp) explorer() explorerSrc {
	return explorerSrc{root: d.vdRoot(), haikuPin: d.modelPinFor(explorerModelClaude)}
}

func (x explorerSrc) roleFile() string { return x.root + "/_agents/" + explorerName + ".md" }
func (x explorerSrc) codexFile() string {
	return x.root + "/setup/codex/agents/" + explorerName + ".toml"
}

var explorerFmKV = regexp.MustCompile(`^([A-Za-z_-]+):\s*(.*)$`)

// roleParse is _explorer_role_json: ok is false on a role file that is missing or malformed.
func (x explorerSrc) roleParse() (explorerRole, bool) {
	data, err := os.ReadFile(x.roleFile())
	if err != nil {
		return explorerRole{}, false
	}
	src := string(data)
	if !strings.HasPrefix(src, "---\n") {
		return explorerRole{}, false
	}
	rest := src[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return explorerRole{}, false
	}
	fm, body := rest[:end], strings.TrimSpace(rest[end+len("\n---\n"):])
	vals := map[string]string{}
	key := ""
	for _, line := range strings.Split(fm, "\n") {
		if kv := explorerFmKV.FindStringSubmatch(line); kv != nil && !strings.HasPrefix(line, " ") {
			key = kv[1]
			v := strings.TrimSpace(kv[2])
			if v == ">" || v == "|" {
				v = ""
			}
			vals[key] = v
		} else if key != "" && strings.TrimSpace(line) != "" {
			vals[key] = strings.TrimSpace(vals[key] + " " + strings.TrimSpace(line))
		}
	}
	for _, need := range []string{"name", "description", "tools"} {
		if vals[need] == "" {
			return explorerRole{}, false
		}
	}
	var tools []string
	for _, t := range strings.Split(vals["tools"], ",") {
		if t = strings.TrimSpace(t); t != "" {
			tools = append(tools, t)
		}
	}
	return explorerRole{name: vals["name"], description: vals["description"], tools: tools, prompt: body}, true
}

// explorerJSONString is python's json.dumps for one string (ensure_ascii), then every space spelt  .
func explorerJSONString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			sb.WriteString(`\"`)
		case r == '\\':
			sb.WriteString(`\\`)
		case r == '\n':
			sb.WriteString(`\n`)
		case r == '\r':
			sb.WriteString(`\r`)
		case r == '\t':
			sb.WriteString(`\t`)
		case r == '\b':
			sb.WriteString(`\b`)
		case r == '\f':
			sb.WriteString(`\f`)
		case r < 0x20 || r > 0x7e && r < 0xa0 || r == 0x7f:
			sb.WriteString(explorerUEsc(r))
		case r > 0x7e:
			if r > 0xffff {
				r -= 0x10000
				sb.WriteString(explorerUEsc(0xd800 + (r >> 10)))
				sb.WriteString(explorerUEsc(0xdc00 + (r & 0x3ff)))
			} else {
				sb.WriteString(explorerUEsc(r))
			}
		case r == ' ':
			sb.WriteString(explorerSpace)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func explorerUEsc(r rune) string {
	const hexd = "0123456789abcdef"
	return `\u` + string([]byte{hexd[r>>12&0xf], hexd[r>>8&0xf], hexd[r>>4&0xf], hexd[r&0xf]})
}

// model is _explorer_model.
func (x explorerSrc) model(vendor string) string {
	switch vendor {
	case "claude":
		if x.haikuPin != "" {
			return x.haikuPin
		}
		return explorerModelClaude
	case "opencode":
		return explorerModelOpencode
	case "codex":
		return explorerModelCodex
	}
	return ""
}

// claudeJSON is _explorer_claude_json: the `--agents` JSON, or "" when the role cannot be read. It is NOT a launch
// word: the launch line is typed, and 2KB of JSON on it broke opus (see explorerClaudeFunction).
func (x explorerSrc) claudeJSON() string {
	r, ok := x.roleParse()
	if !ok {
		return ""
	}
	tools := make([]string, len(r.tools))
	for i, t := range r.tools {
		tools[i] = explorerJSONString(t)
	}
	return `{` + explorerJSONString(r.name) + `:{"description":` + explorerJSONString(r.description) + `,"prompt":` + explorerJSONString(r.prompt) +
		`,"tools":[` + strings.Join(tools, ",") + `],"model":` + explorerJSONString(x.model("claude")) + `}}`
}

// explorerRundirOK is bash's `case "$rundir" in /*/.hw/?*)`: an absolute path with a `/.hw/` segment and something after it.
func explorerRundirOK(rundir string) bool {
	if !strings.HasPrefix(rundir, "/") {
		return false
	}
	j := strings.Index(rundir[1:], "/.hw/")
	return j >= 0 && len(rundir) > 1+j+len("/.hw/")
}

// explorerClaudeFile is _explorer_claude_file: where the JSON lives for a run (rundir is .hw/<run>).
func explorerClaudeFile(rundir string) string { return rundir + "/explorer-agents.json" }

// explorerClaudeFunction is _explorer_claude_function: the shell text typed into the pane so the `claude` herdr types
// carries `--agents "$(cat <file>)"`. herdr single-quotes every word it types, so the substitution has to be the
// shell's own; `command claude` skips the function, so the sandbox's shim is still what runs.
func explorerClaudeFunction(file string) string {
	return `claude() { command claude --agents "$(cat '` + strings.ReplaceAll(file, `'`, `'\''`) + `')" "$@"; }`
}

// opencodeOverlay is _explorer_opencode_overlay's JSON, in jq -c's key order, spaces spelt  .
func (x explorerSrc) opencodeOverlay() string {
	r, ok := x.roleParse()
	if !ok {
		return ""
	}
	out := `{"agent":{` + explorerJSONString(r.name) + `:{"mode":"subagent","description":` + explorerJSONString(r.description) +
		`,"prompt":` + explorerJSONString(r.prompt) + `,"model":` + explorerJSONString(x.model("opencode")) +
		`,"reasoningEffort":` + explorerJSONString(explorerEffortOpencode) + `,"permission":{"edit":"deny"}}}}`
	if strings.ContainsAny(out, " \t\n") {
		return ""
	}
	return out
}

// codexArgs is _explorer_codex_args.
func (x explorerSrc) codexArgs() string {
	f := x.codexFile()
	if !isFile(f) || strings.ContainsAny(f, " \t\n\v\f\r") {
		return ""
	}
	r, ok := x.roleParse()
	if !ok {
		return ""
	}
	desc := strings.ReplaceAll(r.description, " ", explorerSpace)
	return " -c agents." + explorerName + `.description="` + desc + `" -c agents.` + explorerName + `.config_file="` + f + `"`
}

// explorerResolve is _explorer_resolve: what the manifest says, decided AFTER the launch words are built and
// checked against them. Sets d.explorerLine and d.explorerInjected.
func (d *dsp) explorerResolve() {
	d.explorerLine, d.explorerInjected, d.explorerArm = "", false, false
	x := d.explorer()
	m := x.model(d.agent)
	switch d.agent {
	case "claude":
		if x.claudeJSON() != "" {
			d.explorerInjected, d.explorerArm = true, true
			d.explorerLine = explorerName + " on " + m + " — claude --agents (JSON from _agents/" + explorerName + ".md in the run directory's explorer-agents.json, read by a shell function typed into the pane; nothing written into the repo)"
		} else {
			d.explorerLine = "NOT INJECTED — " + x.roleFile() + " is missing or malformed, so no --agents reached the launch"
		}
	case "opencode":
		if x.opencodeOverlay() != "" {
			d.explorerInjected = true
			d.explorerLine = explorerName + " on " + m + " (reasoningEffort " + explorerEffortOpencode + ") — OPENCODE_CONFIG_CONTENT agent overlay, this session only"
		} else {
			d.explorerLine = "NOT INJECTED — " + x.roleFile() + " is missing or malformed, so the overlay carries no agent"
		}
	case "codex":
		if strings.Contains(" "+d.agentArgs+" ", " -c agents."+explorerName+".config_file=") {
			d.explorerInjected = true
			d.explorerLine = explorerName + " on " + m + " — codex -c agents." + explorerName + " (config_file setup/codex/agents/" + explorerName + ".toml)"
		} else {
			d.explorerLine = "NOT INJECTED — setup/codex/agents/" + explorerName + ".toml or the role file is missing, so no -c agents.* reached the launch"
		}
	default:
		d.explorerLine = "not applicable — unknown vendor '" + d.agent + "'"
	}
}

// tierNote is _explorer_tier_note: the paragraph the prompt carries, "" when the explorer is not there.
func (x explorerSrc) tierNote(vendor string) string {
	m := x.model(vendor)
	var call, implement string
	switch vendor {
	case "claude":
		if x.claudeJSON() == "" {
			return ""
		}
		call = `the Agent tool with subagent_type "` + explorerName + `"`
		implement = `a sonnet subagent (the Agent tool with model "sonnet"), when the edit is already defined`
	case "opencode":
		if x.opencodeOverlay() == "" {
			return ""
		}
		call = `the task tool with subagent_type "` + explorerName + `"`
		implement = "a mid-tier subagent (general-terra), when the edit is already defined"
	case "codex":
		if x.codexArgs() == "" {
			return ""
		}
		call = `the spawn_agent tool with agent_type "` + explorerName + `"`
		implement = "a mid-tier agent (terra), when the edit is already defined"
	default:
		return ""
	}
	return "DELEGATE BY MODEL TIER. Reading is the expensive part of this work, so the top tier is kept for deciding.\n" +
		"  searching, mapping code, finding call sites, pulling docs -> " + explorerName + " (" + m + "): " + call + ". It returns the answer with file:line, not the files.\n" +
		"  edits and running tests -> " + implement + ".\n" +
		"  design, audit and the verdict -> you.\n" +
		"Before you read more than three files to answer one question, hand that question to " + explorerName + "."
}

// modelIsTopTier is model_is_top_tier: opus and fable on the last path segment. Models hw cannot rank are not top tier.
func modelIsTopTier(model string) bool {
	last := model[strings.LastIndex(model, "/")+1:]
	return strings.Contains(last, "opus") || strings.Contains(last, "fable")
}

// checkTopTierCeiling is _check_top_tier_ceiling.
func (d *dsp) checkTopTierCeiling() {
	if d.model == "" || d.reusePane != "" || !modelIsTopTier(d.model) {
		return
	}
	why, src := d.topTierWhy, "--top-tier-why"
	if why == "" {
		why, src = briefFmValue("top_tier_why", d.brief), "the brief's top_tier_why"
	}
	if why == "" {
		d.die("this task is declared kind=build and " + d.model + " is the top tier. A build implements what was already decided, and the top tier reading files itself is the cost this ceiling exists to stop (road-pace-build ran on opus, kind build, 2026-10-14). NOTHING was built. Launch with --model sonnet (or no --model: a claude executor runs sonnet), or, if this build really needs " + d.model + ", say why with --top-tier-why \"<reason>\" or a top_tier_why: line in the brief's frontmatter — it is written to the manifest and the receipt.")
	}
	if strings.Contains(why, "\n") {
		d.die("--top-tier-why must be one line so the manifest and the receipt stay parseable")
	}
	d.topTierWhy = why
	d.topTierLine = d.model + " is top tier on kind build — allowed by " + src + ": " + why
	d.warn("model ceiling: " + d.model + " is the top tier on a kind=build task; launching on the stated reason: " + why)
}

// explorerFor is the explorer's source for a re-task or a revive, which hold no dispatch state: the brain's root and
// the lane's pin of haiku, read from the lane table the way lane_get reads model_pins ("" when the lane pins none).
func (a *app) explorerFor(proj string) explorerSrc {
	x := explorerSrc{root: a.reviveBrainRoot()}
	if proj == "" || !a.c.t.known(proj) {
		return x
	}
	data, err := os.ReadFile(a.c.t.file)
	if err != nil {
		return x
	}
	vals, _ := parseStream(data)
	if len(vals) == 0 {
		return x
	}
	if v, ok := nextPath(vals[0], "lanes", proj, "model_pins", explorerModelClaude); ok && v.kind == vStr {
		x.haikuPin = v.s
	}
	return x
}
