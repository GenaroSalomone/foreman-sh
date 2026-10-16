package main

// Train T5, area A — the agent-name checks of the brief contract (bin/hw): _check_capabilities,
// _check_required_agents, _agent_markdown_mode, _agent_mode_matches, _claude_agent_index, _claude_agent_source,
// _claude_agent_reachable, _opencode_project_config_agent, _opencode_project_agent, _known_agent_names,
// _warn_brief_body_agents. git, jq and sort are called as bash calls them; the python snippets are in Go.

import (
	"os"
	"regexp"
	"strings"
)

// brIFSCollapse is what an unquoted expansion becomes inside one `warn` argument: each run of space, tab and
// newline is a field break, and the fields are joined by a single space again.
func brIFSCollapse(v string) string {
	var sb strings.Builder
	inRun := false
	for i := 0; i < len(v); i++ {
		if c := v[i]; c == ' ' || c == '\t' || c == '\n' {
			if !inRun {
				sb.WriteByte(' ')
			}
			inRun = true
		} else {
			sb.WriteByte(c)
			inRun = false
		}
	}
	return sb.String()
}

// checkCapabilities is _check_capabilities: a WARNING, never a refusal.
func (d *dsp) checkCapabilities(list string) {
	bad := ""
	rest := list
	for rest != "" {
		want := rest
		if i := strings.IndexByte(rest, ','); i >= 0 {
			want, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		want = strings.ReplaceAll(want, " ", "")
		if want == "" {
			continue
		}
		switch want {
		case "browser", "claude-in-chrome":
			if d.agent != "claude" { // # MUTATION-ANCHOR: 844-A123
				bad += " " + want + "(needs claude; " + d.agent + " reaches a browser only through agent-browser, which is a CLI and not the same instrument)"
			}
		case "subagents":
			if d.agent != "opencode" && d.agent != "claude" && !(d.agent == "pi" && d.piSubagentsReady()) { // # MUTATION-ANCHOR: 844-A124
				bad += " " + want + "(hw enables OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS only for opencode, pi needs pi-subagents installed under " + d.piSubagentsDir() + " with its required-child-extension API, and " + d.agent + " has no subagent tool hw can name)"
			}
		case "repo":
			if !d.wantWT { // # MUTATION-ANCHOR: 844-A125
				bad += " " + want + "(this dispatch has --worktree none, so there is no checkout)"
			}
		case "database":
			if !d.wantDB { // # MUTATION-ANCHOR: 844-A126
				bad += " " + want + "(this dispatch has --no-db, so it shares the main checkout database)"
			}
		default: // # MUTATION-ANCHOR: 844-A127
			// the bash writes '"'$want'"' with $want unquoted: IFS runs in it become one space (globs: UNESTABLISHED)
			d.warn("brief requires '\"" + brIFSCollapse(want) + "\"', which hw cannot check. Known: browser subagents repo database")
		}
	}
	if bad != "" {
		d.warn("the brief requires capabilities this target may not have:" + bad)
		d.warn("this is a WARNING, not a refusal — hw knows the vendor, not what the session can reach. If it matters, change --agent or the brief before the work reaches minute 40.")
	} else {
		d.brAppendDeclared("requires") // # MUTATION-ANCHOR: 844-A128
	}
}

// ── markdown agent definitions ──────────────────────────────────────────────

var brReMode = regexp.MustCompile(`(?i)^[` + brS + `]*mode[` + brS + `]*:[` + brS + `]*['"]?([^'"` + brS + `#]+)`)

// brAgentModeOf is the python of _agent_markdown_mode over its text: the mode, or false for the exit 1.
func brAgentModeOf(text string) (string, bool) {
	lines := pySplitlines(text)
	mode := "all"
	if len(lines) > 0 && pyStrip(lines[0]) == "---" { // # MUTATION-ANCHOR: 844-A129
		for _, line := range lines[1:] {
			if pyStrip(line) == "---" {
				break
			}
			if m := brReMode.FindStringSubmatch(line); m != nil {
				mode = strings.ToLower(m[1])
				break
			}
		}
	}
	switch mode {
	case "primary", "subagent", "all":
		return mode, true
	}
	return "", false
}

// brAgentMarkdownMode is _agent_markdown_mode <path>.
func brAgentMarkdownMode(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.ReplaceAll(strings.ReplaceAll(brDecodeReplace(data), "\r\n", "\n"), "\r", "\n")
	return brAgentModeOf(text)
}

// brAgentModeMatches is _agent_mode_matches.
func brAgentModeMatches(role, mode string) bool {
	return role == "any" || mode == "all" || role == mode
}

// ── the claude agent registry ───────────────────────────────────────────────

var brReName = regexp.MustCompile(`^name[` + brS + `]*:[` + brS + `]*(.*?)[` + brS + `]*$`)

// brClaudeDeclared is `declared(text)` of _claude_agent_index: the frontmatter `name:`.
func brClaudeDeclared(text string) string {
	lines := pySplitlines(text)
	if len(lines) == 0 || pyStrip(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if pyStrip(line) == "---" {
			break
		}
		if m := brReName.FindStringSubmatch(line); m != nil {
			return pyStrip(strings.Trim(pyStrip(m[1]), "'\""))
		}
	}
	return ""
}

// brClaudeIndexDir is `_claude_agent_index dir <root>`: "name<TAB>path" per definition.
func brClaudeIndexDir(root string) []string {
	if !pyIsDir(root) {
		return nil
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	brSortBytes(names)
	var out []string
	for _, f := range names {
		path := root + "/" + f
		if !strings.HasSuffix(f, ".md") || !pyIsFile(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := brClaudeDeclared(brDecodeReplace(data))
		if name == "" { // # MUTATION-ANCHOR: 844-A130
			name = f[:len(f)-3]
		}
		out = append(out, name+"\t"+path)
	}
	return out
}

// brClaudeIndexGit is `_claude_agent_index git <repo> <ref> <sub>`: the committed tree a new worktree receives.
func brClaudeIndexGit(repo, ref, sub string) []string {
	ls, rc := brExec("", "git", "-C", repo, "ls-tree", "--name-only", ref+":"+sub)
	if rc != 0 {
		return nil
	}
	names := strings.Split(ls, "\n")
	brSortBytes(names)
	var out []string
	for _, f := range names {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		spec := ref + ":" + sub + "/" + f
		show, rc := brExec("", "git", "-C", repo, "show", spec)
		if rc != 0 {
			continue
		}
		name := brClaudeDeclared(brDecodeReplace([]byte(strings.ReplaceAll(strings.ReplaceAll(show, "\r\n", "\n"), "\r", "\n"))))
		if name == "" {
			name = f[:len(f)-3]
		}
		out = append(out, name+"\t"+spec)
	}
	return out
}

// brIndexLookup is `| awk -F '\t' -v n="$name" '$1 == n { print $2; exit }'`.
func brIndexLookup(entries []string, name string) string {
	for _, l := range entries {
		f := strings.Split(l, "\t")
		if f[0] == name {
			if len(f) > 1 {
				return f[1]
			}
			return ""
		}
	}
	return ""
}

// claudeAgentSource is _claude_agent_source: where an agent named `name` is defined, or false.
func (d *dsp) claudeAgentSource(name, projectDir string) (string, bool) {
	taskDir := d.c.work + "/" + d.proj + "/" + d.task
	hit := brIndexLookup(brClaudeIndexDir(taskDir+"/.claude/agents"), name)
	if hit == "" && projectDir != "" && d.wantWT { // # MUTATION-ANCHOR: 844-A131
		switch d.brLaneGet(d.proj, "agents_from") {
		case "checkout": // # MUTATION-ANCHOR: 844-A132
			hit = brIndexLookup(brClaudeIndexDir(projectDir+"/.claude/agents"), name)
		case "base": // # MUTATION-ANCHOR: 844-A133
			hit = brIndexLookup(brClaudeIndexGit(projectDir, d.brAgentsRef(), ".claude/agents"), name)
		}
	}
	if hit == "" { // # MUTATION-ANCHOR: 844-A134
		hit = brIndexLookup(brClaudeIndexDir(d.c.home+"/.claude/agents"), name)
	}
	return hit, hit != ""
}

// claudeAgentReachable is _claude_agent_reachable.
func (d *dsp) claudeAgentReachable(name string) bool {
	_, ok := d.claudeAgentSource(name, d.c.checkout(d.proj))
	return ok
}

// ── the opencode agent registry ─────────────────────────────────────────────

// brOpencodeConfigAgent is _opencode_project_config_agent: "mode\tsource", status 0; 1 not defined; 2 invalid mode.
func brOpencodeConfigAgent(name, conf, repo string) (string, int) {
	var json string
	if repo != "" {
		out, rc := brExec("", "git", "-C", repo, "show", conf)
		if rc != 0 {
			return "", 1
		}
		json = brSub(out)
	} else {
		if !isFile(conf) {
			return "", 1
		}
		data, err := os.ReadFile(conf)
		if err != nil {
			return "", 1
		}
		json = brSub(string(data))
	}
	if _, rc := brExec(json, "jq", "-e", "--arg", "a", name, `.agent[$a] | type == "object" and has("mode")`); rc != 0 {
		return "", 1
	}
	out, rc := brExec(json, "jq", "-r", "--arg", "a", name, ".agent[$a].mode")
	if rc != 0 {
		return "", 2
	}
	switch mode := brSub(out); mode {
	case "primary", "subagent", "all":
		return mode + "\t" + conf, 0
	}
	return "", 2
}

// opencodeProjectAgent is _opencode_project_agent: "mode\tsource" and status 0, 1 when no definition, 2 invalid.
func (d *dsp) opencodeProjectAgent(name string) (string, int) {
	taskDir := d.c.work + "/" + d.proj + "/" + d.task
	ocProjectDir := ""
	if d.brIsProductRepo() {
		ocProjectDir = d.c.checkout(d.proj)
	}
	for _, path := range []string{taskDir + "/.opencode/agent/" + name + ".md", taskDir + "/.opencode/agents/" + name + ".md"} {
		if !isFile(path) {
			continue
		}
		mode, ok := brAgentMarkdownMode(path)
		if !ok {
			return "", 2
		}
		return mode + "\t" + path, 0
	}
	agentsFrom := d.brLaneGet(d.proj, "agents_from")
	agentsRef := d.brAgentsRef()
	if agentsFrom == "checkout" && d.wantWT { // # MUTATION-ANCHOR: 844-A167
		for _, path := range []string{ocProjectDir + "/.opencode/agent/" + name + ".md", ocProjectDir + "/.opencode/agents/" + name + ".md"} {
			if !isFile(path) {
				continue
			}
			mode, ok := brAgentMarkdownMode(path)
			if !ok {
				return "", 2
			}
			return mode + "\t" + path, 0
		}
	}
	if agentsFrom == "base" && d.wantWT { // # MUTATION-ANCHOR: 844-A135
		for _, rel := range []string{".opencode/agent/" + name + ".md", ".opencode/agents/" + name + ".md"} {
			if _, rc := brExec("", "git", "-C", ocProjectDir, "cat-file", "-e", agentsRef+":"+rel); rc == 0 {
				// `git show … | _agent_markdown_mode -`: the function's own heredoc is python's stdin, so the piped text
				// is never read and the mode is the default.
				mode := "all" // # MUTATION-ANCHOR: 844-A136
				return mode + "\t" + agentsRef + ":" + rel, 0
			}
		}
	}
	res, st := brOpencodeConfigAgent(name, taskDir+"/opencode.json", "")
	if st != 1 {
		return res, st
	}
	if agentsFrom == "checkout" && d.wantWT && ocProjectDir != "" {
		res, st = brOpencodeConfigAgent(name, ocProjectDir+"/opencode.json", "")
		if st != 1 {
			return res, st
		}
	}
	if agentsFrom == "base" && d.wantWT && ocProjectDir != "" { // # MUTATION-ANCHOR: 844-A137
		res, st = brOpencodeConfigAgent(name, agentsRef+":opencode.json", ocProjectDir)
		if st != 1 {
			return res, st
		}
	}
	for _, path := range []string{d.c.home + "/.config/opencode/agent/" + name + ".md", d.c.home + "/.config/opencode/agents/" + name + ".md"} {
		if !isFile(path) {
			continue
		}
		mode, ok := brAgentMarkdownMode(path)
		if !ok {
			return "", 2
		}
		return mode + "\t" + path, 0
	}
	return "", 1
}

// brSplitTab is `${v%%$(printf '\t')*}` and `${v#*$(printf '\t')}`.
func brSplitTab(v string) (string, string) {
	a, b, _ := strings.Cut(v, "\t")
	return a, b
}

// brNameOK is `case "$required" in ""|*[!A-Za-z0-9._-]*|.|..) bad`.
func brNameOK(n string) bool {
	if n == "" || n == "." || n == ".." {
		return false
	}
	for _, r := range n {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// checkRequiredAgents is _check_required_agents: false is bash's `return 1`.
func (d *dsp) checkRequiredAgents(list string) bool {
	bad := false
	rest := list
	ocConf := d.c.home + "/.config/opencode/opencode.json"
	for rest != "" {
		spec := rest
		if i := strings.IndexByte(rest, ','); i >= 0 {
			spec, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		spec = strings.ReplaceAll(spec, " ", "")
		if spec == "" {
			continue
		}
		role, required := "any", spec
		switch {
		case strings.HasPrefix(spec, "primary:"): // # MUTATION-ANCHOR: 844-A139
			role, required = "primary", strings.TrimPrefix(spec, "primary:")
		case strings.HasPrefix(spec, "subagent:"): // # MUTATION-ANCHOR: 844-A140
			role, required = "subagent", strings.TrimPrefix(spec, "subagent:")
		}
		if !brNameOK(required) { // # MUTATION-ANCHOR: 844-A138
			d.warn("brief requires agent '" + spec + "', but the accepted shape is [primary:|subagent:]name and names may only contain letters, digits, dot, underscore and dash")
			bad = true
			continue
		}
		// THE EXPLORER IS INJECTED BY hw, NOT FOUND ON DISK: met when this dispatch put it in the launch, as a subagent.
		if required == explorerName && d.explorerInjected {
			if role == "primary" {
				d.warn("brief requires agent '" + required + "' as primary, but the cheap explorer hw injects is a subagent")
				bad = true
			}
			continue
		}
		switch d.agent {
		case "opencode":
			projectAgent, st := d.opencodeProjectAgent(required)
			if st == 2 { // # MUTATION-ANCHOR: 844-A169
				d.warn("brief requires agent '" + spec + "', but its project definition has no valid mode (primary, subagent or all)")
				bad = true
				continue
			}
			if st != 0 {
				projectAgent = ""
			}
			if projectAgent != "" {
				projectMode, projectSource := brSplitTab(projectAgent)
				if !brAgentModeMatches(role, projectMode) { // # MUTATION-ANCHOR: 844-A141
					d.warn("brief requires agent '" + required + "' as " + role + ", but " + projectSource + " declares mode: " + projectMode)
					bad = true
				}
			} else if !brReadable(ocConf) || brJqFails(ocConf, "-e", ".agent | type == \"object\"") {
				d.warn("brief requires agent '" + spec + "' for opencode, but hw cannot read a valid .agent registry from " + ocConf + " and found no reachable project agent definition")
				bad = true
			} else if brJqFails(ocConf, "-e", "--arg", "a", required, ".agent[$a]") { // # MUTATION-ANCHOR: 844-A142
				d.warn("brief requires agent '" + spec + "', but opencode has no .agent[" + required + "] in " + ocConf + " and no reachable .opencode/agent(s)/" + required + ".md")
				bad = true
			} else if role != "any" { // # MUTATION-ANCHOR: 844-A143
				out, _ := brExec("", "jq", "-r", "--arg", "a", required, `.agent[$a].mode // "all"`, ocConf)
				configMode := brSub(out)
				switch configMode {
				case "primary", "subagent", "all": // # MUTATION-ANCHOR: 844-A168
				default:
					configMode = "invalid"
				}
				if !brAgentModeMatches(role, configMode) { // # MUTATION-ANCHOR: 844-A144
					d.warn("brief requires agent '" + required + "' as " + role + ", but " + ocConf + " declares mode: " + configMode)
					bad = true
				}
			}
		case "claude":
			projectDir := ""
			if d.brIsProductRepo() {
				projectDir = d.c.checkout(d.proj)
			}
			_, found := d.claudeAgentSource(required, projectDir)
			if !found { // # MUTATION-ANCHOR: 844-A145
				d.warn("brief requires agent '" + spec + "', but claude cannot reach an agent named '" + required + "' (frontmatter name:, or the file name when name: is absent) in .claude/agents from the intended workdir")
				bad = true
			} else if role == "primary" { // # MUTATION-ANCHOR: 844-A146
				d.warn("brief requires agent '" + required + "' as primary, but Claude .claude/agents/*.md definitions are Task-only subagents")
				bad = true
			}
		default: // # MUTATION-ANCHOR: 844-A147
			d.warn("brief requires agent '" + required + "', but hw cannot verify named agents for vendor " + d.agent)
			bad = true
		}
	}
	if bad { // # MUTATION-ANCHOR: 844-A148
		return false
	}
	d.brAppendDeclared("requires-agents") // # MUTATION-ANCHOR: 844-A149
	return true
}

// brJqFails runs `jq <flags…> <file>` the way bash tests it (`! jq -e … file >/dev/null 2>&1`): true when it fails.
func brJqFails(file string, args ...string) bool {
	_, rc := brExec("", "jq", append(args, file)...)
	return rc != 0
}

// ── the agents a brief names in prose ───────────────────────────────────────

// knownAgentNames is _known_agent_names: every name some agent registry defines, one per line, `sort -u`.
func (d *dsp) knownAgentNames() []string {
	taskDir := d.c.work + "/" + d.proj + "/" + d.task
	projectDir := d.c.checkout(d.proj)
	var names []string
	add := func(l string) {
		if l != "" {
			names = append(names, l)
		}
	}
	for _, dir := range []string{taskDir + "/.opencode/agent", taskDir + "/.opencode/agents", projectDir + "/.opencode/agent", projectDir + "/.opencode/agents", d.c.home + "/.config/opencode/agent", d.c.home + "/.config/opencode/agents"} {
		if !isDir(dir) {
			continue
		}
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			n := e.Name()
			if strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".md") || !isFile(dir+"/"+n) {
				continue
			}
			add(n[:len(n)-3])
		}
	}
	cut1 := func(entries []string) {
		for _, l := range entries {
			n, _ := brSplitTab(l)
			add(n)
		}
	}
	for _, dir := range []string{taskDir + "/.claude/agents", projectDir + "/.claude/agents", d.c.home + "/.claude/agents"} {
		if !isDir(dir) {
			continue
		}
		cut1(brClaudeIndexDir(dir))
	}
	if d.brLaneGet(d.proj, "agents_from") == "base" && projectDir != "" {
		cut1(brClaudeIndexGit(projectDir, d.brAgentsRef(), ".claude/agents"))
		out, _ := brExec("", "git", "-C", projectDir, "ls-tree", "--name-only", d.brAgentsRef()+":.opencode/agent")
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			add(strings.TrimSuffix(l, ".md"))
		}
	}
	if out, _ := brExec("", "jq", "-r", ".agent // {} | keys[]", d.c.home+"/.config/opencode/opencode.json"); out != "" { // # MUTATION-ANCHOR: 844-A156
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			add(l)
		}
	}
	return brSortU(names)
}

// warnBriefBodyAgents is _warn_brief_body_agents: warns, never refuses.
func (d *dsp) warnBriefBodyAgents() {
	if d.brief == "" || !brReadable(d.brief) {
		return
	}
	data, err := os.ReadFile(d.brief)
	if err != nil {
		return
	}
	lines := pySplitlines(brDecodeReplace(data))
	if len(lines) > 0 && pyStrip(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if pyStrip(lines[i]) == "---" {
				lines = lines[i+1:]
				break
			}
		}
	}
	body := brSub(strings.Join(lines, "\n") + "\n")
	if body == "" {
		return
	}
	names := d.knownAgentNames()
	if len(names) == 0 {
		return
	}
	conf := d.c.home + "/.config/opencode/opencode.json"
	for _, name := range names {
		if name == "" || !strings.Contains(body, name) { // # MUTATION-ANCHOR: 844-A150
			continue
		}
		hit := ""
		switch d.agent {
		case "opencode":
			var mode, source string
			if src, st := d.opencodeProjectAgent(name); st == 0 && src != "" { // # MUTATION-ANCHOR: 844-A155
				mode, source = brSplitTab(src)
			} else {
				out, _ := brExec("", "jq", "-r", "--arg", "a", name, ".agent[$a].mode // empty", conf)
				if mode = brSub(out); mode != "" {
					source = conf
				} else {
					mode, source = "", ""
				}
			}
			if mode == "" || mode != "subagent" {
				continue
			}
			hit = source + " declares mode: subagent, so it is reachable ONLY as a subagent — `opencode run --agent " + name + "` selects a PRIMARY and the runtime will reject it"
		case "claude":
			if d.claudeAgentReachable(name) { // # MUTATION-ANCHOR: 844-A151
				continue
			}
			hit = "claude cannot reach .claude/agents/" + name + ".md from the intended workdir, so the Task tool will not find it"
		default:
			continue
		}
		if hit == "" {
			continue
		}
		d.warn("the brief's TEXT names the agent '" + name + "': " + hit)                                                                          // # MUTATION-ANCHOR: 844-A152
		d.warn("hw is NOT refusing this launch — prose carries no role, so it cannot prove the brief uses '" + name + "' the way that fails.")     // # MUTATION-ANCHOR: 844-A153
		d.warn("say the ROLE and the FALLBACK in the brief, or declare 'requires-agents: subagent:" + name + "' to be refused instead of warned.") // # MUTATION-ANCHOR: 844-A154
	}
}

// piAgentDir, piSubagentsDir and piSubagentsReady are _pi_agent_dir, _pi_subagents_dir and _pi_subagents_ready: the
// text of a `requires: subagents` refusal names where pi-subagents would have to be.
func (d *dsp) piAgentDir() string {
	return orDefaultStr(d.env("PI_CODING_AGENT_DIR"), d.env("HOME")+"/.pi/agent")
}

func (d *dsp) piSubagentsDir() string { return d.piAgentDir() + "/npm/node_modules/pi-subagents" }

func (d *dsp) piSubagentsReady() bool {
	for _, f := range []string{"/index.js", "/src/api/required-child-extensions.js"} {
		fh, err := os.Open(d.piSubagentsDir() + f)
		if err != nil {
			return false
		}
		fh.Close()
	}
	return true
}
