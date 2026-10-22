package main

// Train T5, area B: the brain guard, the engram label proxy, the artifact tools and the sandbox.
// bin/hw: _brain_guard_applies/_dir/_claude_plugin/_opencode_config/_present/_check/_args, _engram_label,
// _engram_proxy_bin/_mcp_bin/_mcp_claude_file, _engram_proxy_args, _artifacts_verdict, _artifacts_args,
// _check_sandbox.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// vdRoot is bash's $_brain_root: the install root the shim hands down.
func (d *dsp) vdRoot() string { return d.env("HW_CORE_ROOT") }

// vdSpace is `*[[:space:]]*`.
func vdSpace(s string) bool { return strings.ContainsAny(s, " \t\n\v\f\r") }

// vdPhysDir is `$(cd -P "$p" 2>/dev/null && pwd)`.
func vdPhysDir(p string) (string, bool) {
	if !isDir(p) {
		return "", false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	return abs, true
}

// vdCommandV is `command -v NAME` for a name that is not a builtin: the first executable file on PATH.
func (d *dsp) vdCommandV(name string) string {
	for _, dir := range strings.Split(d.getenv("PATH"), ":") {
		if dir == "" {
			dir = "."
		}
		p := dir + "/" + name
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0111 != 0 {
			return p
		}
	}
	return ""
}

func (d *dsp) brainGuardApplies() bool { return d.vdLane(d.proj, "brain_guard") != "0" } // # MUTATION-ANCHOR: 844-B68

func (d *dsp) brainGuardDir() string { return d.vdRoot() + "/setup/guards" }

func (d *dsp) brainGuardClaudePlugin() string { return d.brainGuardDir() + "/brain-guard" }

func (d *dsp) brainGuardOpencodeConfig() string {
	return d.brainGuardDir() + "/brain-guard.opencode.json"
}

// brainGuardPresent is _brain_guard_present: the four files of the guard are on disk.
func (d *dsp) brainGuardPresent() bool {
	g := d.brainGuardDir()
	return isFile(g+"/brain-guard/hooks/hooks.json") && isFile(g+"/brain-guard.opencode.json") && // # MUTATION-ANCHOR: 844-B69
		isFile(g+"/deny-brain-writes.js") && isFile(g+"/deny_brain_writes.py")
}

// brainGuardCheck is _brain_guard_check: a launch refuses what a dry run warns about.
func (d *dsp) brainGuardCheck() {
	if !d.brainGuardApplies() {
		return
	}
	work := d.c.work
	cdWork := orDefaultStr(work, "/nonexistent")
	bgWork, ok := vdPhysDir(cdWork)
	if !ok {
		bgWork = work // # MUTATION-ANCHOR: 844-B70
	}
	bgBrain, ok := vdPhysDir(d.c.brain)
	if !ok {
		bgBrain = d.c.brain
	}
	if strings.HasPrefix(bgWork+"/", bgBrain+"/") { // # MUTATION-ANCHOR: 844-B71
		if d.dispatchDry { // # MUTATION-ANCHOR: 844-B72
			d.warn("work root " + work + " is inside the brain " + d.c.brain + ": this dry run goes on, and a launch would refuse — the brain guard denies every command a product executor runs in its own worktree there. Set \"work\" in projects.json outside the brain")
		} else {
			d.die("work root " + work + " is inside the brain " + d.c.brain + ": the brain guard denies every command a product executor runs in its own worktree there, so it could execute nothing. Set \"work\" in projects.json to a directory outside the brain (install.sh puts it beside it), or declare the lane \"brain_guard\": false")
		}
	}
	if vdSpace(d.vdRoot()) {
		d.die("the brain guard cannot be passed as one word: the checkout path has whitespace (" + d.vdRoot() + ")")
	}
	if d.brainGuardPresent() { // # MUTATION-ANCHOR: 844-B73
		return
	}
	if !d.dispatchDry {
		d.die("the brain guard is not on disk under " + d.brainGuardDir() + ", and a product executor is not launched without it")
	}
	d.warn("the brain guard is not on disk under " + d.brainGuardDir() + ": this dry run shows no guard, and a launch would refuse")
}

// brainGuardArgs is _brain_guard_args: the Claude half, onto the agent args.
func (d *dsp) brainGuardArgs() {
	if d.agent == "claude" && d.brainGuardApplies() && d.brainGuardPresent() {
		d.agentArgs = vdJoinArg(d.agentArgs, "--plugin-dir "+d.brainGuardClaudePlugin())
	}
}

// vdJoinArg is `${AGENT_ARGS:+$AGENT_ARGS }word`.
func vdJoinArg(args, word string) string {
	if args == "" {
		return word
	}
	return args + " " + word
}

// engramLabel is _engram_label: the label of the lane's memory, `brain` unless the lane names one.
func (d *dsp) engramLabel(proj string) string {
	return orDefaultStr(d.vdLane(proj, "engram"), "brain") // # MUTATION-ANCHOR: 844-B74
}

// engramProxyArgs is _engram_proxy_args: each vendor's way to launch `engram mcp` behind the label proxy. A dry
// run writes nothing (the claude --mcp-config file is the launch's), and Go only ever runs a dry dispatch.
func (d *dsp) engramProxyArgs() {
	d.vdEngramOver, d.engramProxyLine = "", ""
	label := d.engramLabel(d.proj)
	proxy := d.vdRoot() + "/bin/engram-label-proxy"
	eng := d.vdCommandV("engram")
	if eng == "" {
		eng = "engram"
	}
	cache := d.env("XDG_CACHE_HOME")
	if cache == "" {
		cache = d.env("HOME") + "/.cache" // # MUTATION-ANCHOR: 844-B75
	}
	file := cache + "/hw/engram-mcp-" + label + ".json"
	if label == "" || strings.Trim(label, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") != "" { // # MUTATION-ANCHOR: 844-B76
		d.die("engram label '" + label + "' cannot travel as one word to the engram proxy")
	}
	if vdSpace(proxy + eng + file) {
		d.die("the engram proxy cannot be passed as one word: a path has whitespace (" + proxy + ", " + eng + ", " + file + ")")
	}
	if st, err := os.Stat(proxy); err != nil || st.Mode()&0111 == 0 { // # MUTATION-ANCHOR: 844-B77
		if !d.dispatchDry {
			d.die("the engram label proxy is not on disk at " + proxy + ", and an executor is not launched with an unguarded engram")
		}
		d.warn("the engram label proxy is not on disk at " + proxy + ": this dry run shows no proxy, and a launch would refuse")
		return
	}
	switch d.agent {
	case "claude":
		d.agentArgs = vdJoinArg(d.agentArgs, "--mcp-config "+file)
		d.engramProxyLine = "--mcp-config " + file // # MUTATION-ANCHOR: 844-B78
	case "opencode":
		cmd := &jval{kind: vArr}
		for _, w := range []string{proxy, "--label", label, "--", eng, "mcp", "--tools=agent"} {
			cmd.arr = append(cmd.arr, jstr(w))
		}
		engram := &jval{kind: vObj, obj: []member{{"type", jstr("local")}, {"command", cmd}}}
		over := &jval{kind: vObj, obj: []member{{"mcp", &jval{kind: vObj, obj: []member{{"engram", engram}}}}}}
		d.vdEngramOver = over.toJSON()
		d.engramProxyLine = "OPENCODE_CONFIG_CONTENT mcp.engram" // # MUTATION-ANCHOR: 844-B79
	case "codex":
		d.agentArgs = vdJoinArg(d.agentArgs, `-c mcp_servers.engram.command="`+proxy+`" -c mcp_servers.engram.args=["--label","`+label+`","--","`+eng+`","mcp","--tools=agent"]`)
		d.engramProxyLine = "-c mcp_servers.engram.*" // # MUTATION-ANCHOR: 844-B80
	case "pi":
		d.engramProxyLine = piEngramProxyLine // t6_pi.go: the extension registers hw_engram, nothing rides the launch line
	}
}

const vdArtifactTools = "Artifact,ArtifactComments,ArtifactData"

// vdArtifactsVerdict is _artifacts_verdict: `allow`, or the reason for deny.
func (d *dsp) vdArtifactsVerdict(lane, account string) string {
	want := d.projectArtifacts(lane)
	if want == "deny" { // # MUTATION-ANCHOR: 844-B81
		return "the lane declares artifacts: deny"
	}
	if want != account { // # MUTATION-ANCHOR: 844-B82
		return "the lane publishes only to '" + want + "', and this executor runs under '" + account + "'"
	}
	return "allow"
}

// artifactsArgs is _artifacts_args: whether the claude.ai Artifact tools stay on the executor.
func (d *dsp) artifactsArgs() {
	if d.agent != "claude" { // # MUTATION-ANCHOR: 844-B83
		d.artifactsLine = "not governed — hw removes the claude.ai Artifact tools from Claude Code only; this executor is " + d.agent
		return
	}
	v := d.vdArtifactsVerdict(d.proj, d.account)
	if v == "allow" {
		d.artifactsLine = "publish to '" + d.account + "' only (the account this executor runs under is the lane's)"
		return
	}
	d.agentArgs += " --disallowedTools " + vdArtifactTools
	d.artifactsLine = "DENIED — --disallowedTools " + vdArtifactTools + " (" + v + ")"
}

// checkSandbox is _check_sandbox: --sandbox is claude's and pi's, on a machine hw-sandbox says is supported.
func (d *dsp) checkSandbox() {
	d.sandboxLine = ""
	if !d.sandbox { // # MUTATION-ANCHOR: 844-B84
		return
	}
	if d.agent != "claude" && d.agent != "pi" { // # MUTATION-ANCHOR: 844-B85
		d.die("--sandbox refused: it is measured for claude and pi only, and this dispatch runs " + d.agent + ". Dispatch with --agent claude or --agent pi, or without --sandbox. NOTHING was built.")
	}
	bin := d.env("HW_CORE_BIN_DIR") + "/hw-sandbox"
	cmd := exec.Command(bin, "supported")
	cmd.Env = childEnv()
	cmd.Env = os.Environ()
	for k, v := range d.envOver {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	why := strings.TrimRight(string(out), "\n")
	if err != nil { // # MUTATION-ANCHOR: 844-B86
		if len(out) == 0 {
			why = bin + ": " + err.Error()
		}
		d.die("--sandbox refused: " + why + " NOTHING was built.")
	}
	d.sandboxLine = "on (" + d.sandboxSrc + ") — Seatbelt hard profile: writes only in the workdir, artifacts, TMPDIR and the agent's own state; no unix sockets (herdr, docker, ssh-agent); ~/.ssh, ~/.config/gh, ~/.docker unreadable. Reports leave through the outbox broker. TCP is NOT confined."
	if d.agent == "pi" { // pi's own state differs (bash: the line after _check_sandbox's claude one)
		d.sandboxLine = "on (" + d.sandboxSrc + ") — Seatbelt hard profile: writes only in the workdir, artifacts, TMPDIR and pi's session dir for this workdir (+ the settings/auth/models lock dirs); ~/.pi/agent/{extensions,mcp.json,settings.json,auth.json} stay read-only; no unix sockets (herdr, docker, ssh-agent), so herdr's pi integration publishes no agent_session from inside; ~/.ssh, ~/.config/gh, ~/.docker unreadable. Reports leave through the outbox broker. TCP is NOT confined."
	}
}
