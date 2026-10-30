package main

// What hw revive hands the agent: the framework gate, the machine's permissions policy, the env file read back,
// the workspace the tab goes into, and the args the recorded `resume` never carried because they are POLICY and
// not identity (the permissions flag, the brain guard, the engram label proxy, the artifact tools).

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// reviveSpeckitSkills is _speckit_skills_in: whether <dir>/.claude/skills holds a speckit-* skill.
func reviveSpeckitSkills(dir string) bool {
	d := dir + "/.claude/skills"
	if !isDir(d) {
		return false
	}
	ms, _ := filepath.Glob(d + "/speckit-*")
	for _, m := range ms {
		if _, err := os.Stat(m); err == nil {
			return true
		}
	}
	return false
}

// reviveValidateSDD is _validate_sdd_vendor_pair (bin/hw:5698): it dies, before anything is built, for a
// vendor that cannot run the framework in force.
func (a *app) reviveValidateSDD(vendor, requested, surface string) {
	mode := requested
	if mode == "" && reviveSpeckitSkills(surface) {
		mode = "speckit"
	}
	switch vendor + ":" + mode {
	case "codex:", "codex:none", "opencode:", "opencode:none":
		return
	case "codex:speckit":
		a.die("Codex framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd " + mode + " requires a demonstrated native entry and completed flow, but that capability is unverified. Nothing was built. Use --sdd none for direct delivery, or validate Codex framework execution before re-dispatching.") // # MUTATION-ANCHOR: 863-G10
	case "opencode:speckit":
		a.die("OpenCode framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd " + mode + " is Spec Kit, but this machine exposes Spec Kit only as Claude Code skills. Nothing was built. Use --agent claude for Spec Kit, or --sdd none for direct delivery.")
	}
}

// reviveResolvePermissions is hw_permissions_resolve "" (project-spaces.sh:711) followed by its caller's
// `|| _revive_die "$PERMISSIONS_ERR"`: ask|skip, from HW_PERMISSIONS, the machine file, or the default.
func (a *app) reviveResolvePermissions(st *reviveRunState) {
	cfg := a.getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = a.getenv("HOME") + "/.config"
	}
	f := cfg + "/hw/permissions"
	v, src := "", ""
	if p := a.getenv("HW_PERMISSIONS"); p != "" {
		v, src = p, "HW_PERMISSIONS"
	} else if isFile(f) {
		if b, err := os.ReadFile(f); err == nil {
			v = strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, string(b))
		}
		src = "machine config " + f
	} else {
		v, src = "skip", "default (recommended)"
	}
	switch v {
	case "ask", "skip":
		st.permissions, st.permSrc = v, src
		return
	}
	st.permissions, st.permSrc = "skip", src
	a.reviveDie("permissions must be ask or skip (got: '" + v + "', from " + src + ")")
}

// reviveEnvRecords is `runenv dump --nul` (strict): the run's env as KEY=value records, or the diagnostic
// runenv prints for a file outside the grammar.
func (a *app) reviveEnvRecords(file string) ([]KV, string) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, "runenv: cannot read " + file + ": " + reviveErrno(err)
	}
	kvs, perr := ParseEnv(data, false, false)
	if perr == nil {
		return kvs, ""
	}
	// the words are runenv's (a Python repr of the line, the codec's own account of a bad byte): ask it
	var stderr bytes.Buffer
	cmd := exec.Command(a.reviveBinDir()+"/runenv", "dump", "--nul", file)
	cmd.Stderr = &stderr
	if cmd.Run() != nil && stderr.Len() > 0 {
		return nil, bashSub(stderr.String())
	}
	return nil, "runenv: " + perr.Error()
}

// reviveWorkspaceByLabel is _ws_id_by_label: herdr workspace list | jq … | head -1.
func (a *app) reviveWorkspaceByLabel(label string) string {
	out, _ := herdrClient{getenv: a.getenv}.stdout("workspace", "list")
	vals, _ := parseStream([]byte(out))
	var outs []string
docs:
	for _, v := range vals {
		res, ok := reviveJqPath(v, "result", "workspaces")
		if !ok {
			continue
		}
		var items []*jval
		switch res.kind {
		case vArr:
			items = res.arr
		case vObj:
			for _, m := range res.obj {
				items = append(items, m.v)
			}
		default:
			continue docs // Cannot iterate
		}
		for _, w := range items {
			l, ok := reviveJqPath(w, "label")
			if !ok {
				continue docs
			}
			if l.kind == vStr && l.s == label {
				id, ok := reviveJqPath(w, "workspace_id")
				if !ok {
					continue docs
				}
				outs = append(outs, reviveRaw(id))
			}
		}
	}
	if len(outs) == 0 {
		return ""
	}
	first := outs[0]
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	return first
}

// reviveProjectWorkspace is _project_workspace: the project's workspace, created on demand and never closed.
func (a *app) reviveProjectWorkspace(proj, workdir string) (string, bool) {
	if !a.c.t.known(proj) {
		a.warn("no workspace label for project '" + proj + "'")
		return "", false
	}
	label, _, _ := a.reviveLaneFields(proj)
	if ws := a.reviveWorkspaceByLabel(label); ws != "" {
		return ws, true
	}
	raw, rc := herdrClient{getenv: a.getenv}.capture("workspace", "create", "--cwd", workdir, "--label", label, "--no-focus")
	raw = bashSub(raw)
	if rc != 0 {
		a.warn("herdr workspace create failed for " + label + ": " + reviveHead(raw, 1))
		return "", false
	}
	// jq -r '.result.workspace.workspace_id // .result.workspace_id': the last alternative prints even a null
	vals, _ := parseStream([]byte(raw))
	var outs []string
docs:
	for _, v := range vals {
		for i, p := range [][]string{{"result", "workspace", "workspace_id"}, {"result", "workspace_id"}} {
			cur, ok := reviveJqPath(v, p...)
			if !ok {
				continue docs
			}
			if cur.truthy() || i == 1 {
				outs = append(outs, reviveRaw(cur))
				continue docs
			}
		}
	}
	ws := bashSub(strings.Join(outs, "\n"))
	if ws == "" || ws == "null" {
		a.warn("herdr workspace create returned no id for " + label)
		return "", false
	}
	return ws, true
}

// reviveEngramProxy is _engram_proxy_args (bin/hw:4641) for a revive (never a dry run): the args that put the
// engram label proxy in front of `engram mcp`, written for the vendor. It starts from an empty AGENT_ARGS.
func (a *app) reviveEngramProxy(proj, agent string) string {
	label := a.c.t.get(proj, "engram")
	if label == "" {
		label = "brain"
	}
	proxy := a.reviveBrainRoot() + "/bin/engram-label-proxy"
	eng := "engram"
	if p, err := exec.LookPath("engram"); err == nil {
		eng = p
	}
	cache := a.getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = a.getenv("HOME") + "/.cache"
	}
	file := cache + "/hw/engram-mcp-" + label + ".json"
	if label == "" || strings.Trim(label, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") != "" {
		a.die("engram label '" + label + "' cannot travel as one word to the engram proxy")
	}
	if strings.IndexFunc(proxy+eng+file, unicode.IsSpace) >= 0 {
		a.die("the engram proxy cannot be passed as one word: a path has whitespace (" + proxy + ", " + eng + ", " + file + ")")
	}
	if st, err := os.Stat(proxy); err != nil || st.Mode()&0o111 == 0 {
		a.die("the engram label proxy is not on disk at " + proxy + ", and an executor is not launched with an unguarded engram")
	}
	switch agent {
	case "claude":
		doc := "{\n  \"mcpServers\": {\n    \"engram\": {\n      \"command\": " + reviveJSONString(proxy) + ",\n      \"args\": [\n        \"--label\",\n        " +
			reviveJSONString(label) + ",\n        \"--\",\n        " + reviveJSONString(eng) + ",\n        \"mcp\",\n        \"--tools=agent\"\n      ]\n    }\n  }\n}\n"
		if os.MkdirAll(filepath.Dir(file), 0o777) != nil || os.WriteFile(file, []byte(doc), 0o666) != nil {
			a.die("could not write the engram proxy's --mcp-config at " + file)
		}
		return "--mcp-config " + file
	case "codex":
		return "-c mcp_servers.engram.command=\"" + proxy + "\" -c mcp_servers.engram.args=[\"--label\",\"" + label + "\",\"--\",\"" + eng + "\",\"mcp\",\"--tools=agent\"]" // # MUTATION-ANCHOR: 863-G11
	}
	return ""
}

func reviveJSONString(s string) string {
	var sb strings.Builder
	writeJSONString(&sb, s)
	return sb.String()
}

// reviveBrainGuardOn is _brain_guard_on: the lane is guarded AND the guard is on disk.
func (a *app) reviveBrainGuardOn(proj string) bool {
	if _, _, guard := a.reviveLaneFields(proj); !guard {
		return false
	}
	d := a.reviveBrainRoot() + "/setup/guards"
	return isFile(d+"/brain-guard/hooks/hooks.json") && isFile(d+"/brain-guard.opencode.json") && isFile(d+"/deny-brain-writes.js") && isFile(d+"/deny_brain_writes.py")
}

// revivePluginDirs is pluginsArgs for a run that already exists: one directory per plugin the run's receipt says the
// brief declared (`plugins`, written at dispatch), as <brain>/plugins/<name>, only where the pinned copy is installed.
func (a *app) revivePluginDirs(rundir string) []string {
	v, rc := reviveReceiptLast(rundir+"/receipt.jsonl", "plugins")
	if rc != 0 || v == "" || strings.HasPrefix(v, "NOT APPLIED") {
		return nil
	}
	var out []string
	for _, n := range strings.Split(v, ",") {
		dir := a.reviveBrainRoot() + "/plugins/" + n
		if n != "" && isFile(dir+"/.claude-plugin/plugin.json") {
			out = append(out, dir)
		}
	}
	return out
}

// reviveFrameworkArgs is the block of cmd_revive that, after the tab exists, adds to AGENT_ARGS what the
// recorded resume could not carry: INJECTED, NOT REPLAYED.
func (a *app) reviveFrameworkArgs(st *reviveRunState, vendor, cmdArgs, rundir string) {
	args := st.agentArgs
	// pi's guard, turn-end hook and engram proxy are its extensions: policy, appended exactly as a fresh launch does.
	if vendor == "pi" && !reviveHasWord(args, "-e "+a.reviveBrainRoot()+"/setup/pi/hw-executor.ts") { // # MUTATION-ANCHOR: 863-G26
		ext, skills := a.revivePiArgs(st.proj)
		args += " " + ext + " " + skills
	}
	if vendor == "codex" && st.permissions == "ask" {
		if !reviveHasWord(args, "-a") {
			args += " -a on-request" // # MUTATION-ANCHOR: 863-G25
		}
	}
	if vendor == "codex" {
		if !strings.Contains(" "+args+" ", " -c mcp_servers.engram.") {
			rev := args
			args = rev + " " + a.reviveEngramProxy(st.proj, "codex")
		}
	}
	if vendor == "claude" {
		if !reviveHasWord(cmdArgs, "--dangerously-skip-permissions") {
			if st.permissions == "skip" { // # MUTATION-ANCHOR: 863-G12
				args += " --dangerously-skip-permissions"
			}
		}
		if a.reviveBrainGuardOn(st.proj) {
			plugin := a.reviveBrainRoot() + "/setup/guards/brain-guard"
			if !reviveHasWord(args, "--plugin-dir "+plugin) {
				args += " --plugin-dir " + plugin
			}
		}
		// the brief's review plugins: the recorded `--resume` line carried them, and a hand-edited one may not
		for _, plugin := range a.revivePluginDirs(rundir) { // # MUTATION-ANCHOR: 1150-G06
			if !reviveHasWord(args, "--plugin-dir "+plugin) {
				args += " --plugin-dir " + plugin
			}
		}
		if !reviveHasWord(args, "--mcp-config") {
			rev := args
			args = rev + " " + a.reviveEngramProxy(st.proj, "claude")
		}
		// its artifact tools: the account is the one the revived pane will actually run under
		acct := "default"
		if cfg := a.reviveEnvValue(rundir, "CLAUDE_CONFIG_DIR"); cfg != "" {
			if cfg == a.getenv("HOME")+"/.claude-personal" {
				acct = "personal"
			} else {
				acct = "unknown:" + cfg
			}
		}
		_, want, _ := a.reviveLaneFields(st.proj)
		if want == "deny" || want != acct { // # MUTATION-ANCHOR: 863-G13
			if !reviveHasWord(args, "--disallowedTools Artifact,ArtifactComments,ArtifactData") {
				args += " --disallowedTools Artifact,ArtifactComments,ArtifactData"
			}
		}
		// And its cheap explorer: policy too, the recorded `--resume` never carried it. It goes to the revived run's own
		// directory by file and function (reviveAgentStart), not by argument; a recorded line that already carries --agents
		// (hand-edited) is left alone rather than given the explorer twice.
		st.explorerArm = false
		if !reviveHasWord(args, "--agents") && a.explorerFor(st.proj).claudeJSON() != "" { // # MUTATION-ANCHOR: 1007-G02
			st.explorerArm = true
		}
	}
	if vendor == "codex" {
		if !strings.Contains(" "+args+" ", " -c agents."+explorerName+".") {
			args += a.explorerFor(st.proj).codexArgs()
		}
	}
	st.agentArgs = args
}

// reviveEnvValue is `runenv --bare get <rundir> KEY || true`.
func (a *app) reviveEnvValue(rundir, key string) string {
	data, err := os.ReadFile(rundir + "/env")
	if err != nil {
		return ""
	}
	kvs, perr := ParseEnv(data, true, true)
	if perr != nil {
		return ""
	}
	for _, kv := range kvs {
		if kv.Key == key {
			return kv.Value
		}
	}
	return ""
}

// reviveLaneFields reads the three fields of a lane the lane table of hw-core does not keep, from the table the
// way lane_config_load emits them: `space` (tostring), `artifacts` (artifacts // account // "default") and
// `brain_guard` (off only when the table says false).
func (a *app) reviveLaneFields(proj string) (space, artifacts string, guard bool) {
	guard = true
	data, err := os.ReadFile(a.c.t.file)
	if err != nil {
		return
	}
	vals, _ := parseStream(data)
	if len(vals) == 0 {
		return
	}
	v := field(field(vals[0], "lanes"), proj)
	space = orDefault(field(v, "space"), "")
	artifacts = orDefault(field(v, "artifacts"), orDefault(field(v, "account"), "default"))
	if g := field(v, "brain_guard"); g != nil && g.kind == vBool && !g.b {
		guard = false
	}
	return
}

// revivePiArgs is `$(_pi_extension_args)` and `$(_pi_skill_args)` as cmd_revive calls them: the dispatch's own builders
// on a dispatch that holds the lane (cmd_revive sets PROJ) and no brief (BRIEF is never set). So a product lane's own
// skills ARE named, and the brief's plugins are not (`_plugins_declared` of no brief is empty). A die of the extension
// builder ends bash's subshell, which `|| _revive_die` then words; the builder's own message has already been said.
func (a *app) revivePiArgs(proj string) (ext, skills string) {
	d := &dsp{app: a, proj: proj} // # MUTATION-ANCHOR: 863-G32
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(exitNow); ok {
					a.reviveDie("could not build pi's extension set") // # MUTATION-ANCHOR: 863-G31
				}
				panic(r)
			}
		}()
		ext = d.piExtensionArgs()
	}()
	return ext, d.piSkillArgs()
}
