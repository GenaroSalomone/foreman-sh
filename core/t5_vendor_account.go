package main

// Train T5, area B: the account the executor runs under.
// bin/hw: _resolve_account; bin/project-spaces.sh: account_config_dir, account_verifier, account_verify,
// claude_account_json, claude_first_run_pending, claude_first_run_command.

import (
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// accountConfigDirOf is account_config_dir: the CLAUDE_CONFIG_DIR an account names ("" for the default), false for
// an account hw does not know.
func (d *dsp) accountConfigDirOf(name string) (string, bool) {
	switch name {
	case "default":
		return "", true
	case "personal":
		return d.env("HOME") + "/.claude-personal", true // # MUTATION-ANCHOR: 844-B87
	}
	return "", false
}

// vdAccountVerifier is account_verifier.
func vdAccountVerifier(name string) (string, bool) {
	switch name {
	case "default":
		return "", true
	case "personal":
		return "claude-personal", true
	}
	return "", false
}

// vdAccountVerify is account_verify: its rc and what it printed, stdout and stderr together (2>&1).
func (d *dsp) vdAccountVerify(acct string) (string, bool) {
	verifier, ok := vdAccountVerifier(acct)
	if !ok {
		return "unknown account: " + acct + " (known: default, personal)\n", false
	}
	if verifier == "" {
		return "", true
	}
	path := d.vdCommandV(verifier)
	if path == "" { // # MUTATION-ANCHOR: 844-B88
		return "account " + acct + " is verified by `" + verifier + "`, which is not on PATH — so capability parity cannot be established and the account is refused.\n", false
	}
	cmd := exec.Command(path, "--check")
	cmd.Env = childEnv()
	cmd.Env = os.Environ()
	for k, v := range d.envOver {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err == nil // # MUTATION-ANCHOR: 844-B89
}

// resolveAccount is _resolve_account: the account line, the verdict and the config dir the executor gets.
func (d *dsp) resolveAccount() {
	d.accountConfigDir, d.accountApplies, d.accountVerdict, d.accountRefusal = "", false, "n/a", ""
	dir, ok := d.accountConfigDirOf(d.account)
	if !ok { // # MUTATION-ANCHOR: 844-B90
		d.accountVerdict = "refused" // # MUTATION-ANCHOR: 844-B91
		d.accountRefusal = "unknown account '" + d.account + "' (known: default, personal)"
		d.accountLine = d.account + "  (" + d.accountSrc + ") — REFUSED: " + d.accountRefusal
		return
	}
	if dir == "" { // # MUTATION-ANCHOR: 844-B92
		d.accountLine = "default  (" + d.accountSrc + ") — no CLAUDE_CONFIG_DIR is set, and an inherited one is cleared"
		return
	}
	if d.agent != "claude" { // # MUTATION-ANCHOR: 844-B93
		d.accountLine = d.account + "  (" + d.accountSrc + ") — NOT APPLIED: this executor is " + d.agent + ", and only Claude Code reads CLAUDE_CONFIG_DIR. Its account is whatever " + d.agent + " is signed into."
		return
	}
	out, good := d.vdAccountVerify(d.account)
	if !good { // # MUTATION-ANCHOR: 844-B94
		out = strings.TrimRight(out, "\n")
		lines := strings.Split(out, "\n")
		if len(lines) > 3 {
			lines = lines[len(lines)-3:]
		}
		d.accountVerdict = "refused"
		d.accountRefusal = strings.Join(lines, " ")
		d.accountLine = d.account + " → " + dir + "  (" + d.accountSrc + ") — REFUSED: capability parity is not established"
		return
	}
	d.accountConfigDir = dir // # MUTATION-ANCHOR: 844-B95
	d.accountApplies = true
	d.accountVerdict = "ok"
	verifier, _ := vdAccountVerifier(d.account)
	d.accountLine = d.account + " → CLAUDE_CONFIG_DIR=" + dir + "  (" + d.accountSrc + "; parity verified by `" + verifier + " --check`)"
}

// vdLoadDict is the python helper of claude_first_run_pending: a JSON object from a file, {} otherwise.
func vdLoadDict(path string) jmap {
	data, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(data) {
		return jmap{}
	}
	v, ok := pyLoads(string(data))
	if !ok {
		return jmap{}
	}
	return pyDict(v)
}

// claudeFirstRunPending is claude_first_run_pending: the once-per-account steps still ahead, one word each.
func (d *dsp) claudeFirstRunPending(cfgDir string) []string {
	home := d.env("HOME")
	jsonPath := home + "/.claude.json" // claude_account_json
	if cfgDir != "" {
		jsonPath = cfgDir + "/.claude.json" // # MUTATION-ANCHOR: 844-B96
	}
	settings := home + "/.claude/settings.json"
	if cfgDir != "" {
		settings = cfgDir + "/settings.json" // # MUTATION-ANCHOR: 844-B97
	}
	g, s := vdLoadDict(jsonPath), vdLoadDict(settings)
	var out []string
	if g["hasCompletedOnboarding"] != true { // # MUTATION-ANCHOR: 844-B98
		out = append(out, "onboarding")
	}
	if g["bypassPermissionsModeAccepted"] != true && s["skipDangerousModePermissionPrompt"] != true { // # MUTATION-ANCHOR: 844-B99
		out = append(out, "bypass")
	}
	return out
}

// claudeFirstRunCommand is claude_first_run_command: the one command that clears both steps.
func (d *dsp) claudeFirstRunCommand(cfgDir string) string {
	if cfgDir != "" {
		return "CLAUDE_CONFIG_DIR=" + cfgDir + " claude --dangerously-skip-permissions"
	}
	return "claude --dangerously-skip-permissions"
}

// validSubagentModel is _valid_subagent_model: inherit, an alias, or claude-<[a-z0-9.-]+>.
func validSubagentModel(v string) bool {
	switch v {
	case "inherit", "haiku", "sonnet", "opus", "fable":
		return true
	}
	rest, ok := strings.CutPrefix(v, "claude-")
	if !ok || rest == "" { // # MUTATION-ANCHOR: 844-232-M09
		return false
	}
	for _, r := range rest {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-') { // # MUTATION-ANCHOR: 844-232-M10
			return false
		}
	}
	return true
}

// resolveSubagentModel is _resolve_subagent_model: the manifest line for the model the executor's subagents run on.
func (d *dsp) resolveSubagentModel() {
	m, src := orDefaultStr(d.subagentModel, "inherit"), orDefaultStr(d.subagentModelSrc, "default")
	switch {
	case d.agent != "claude": // # MUTATION-ANCHOR: 844-232-M06
		agent := orDefaultStr(d.agent, "unknown")
		d.subagentModelLine = m + "  (" + src + ") — NOT APPLIED: this executor is " + agent + ", and only Claude Code reads CLAUDE_CODE_SUBAGENT_MODEL. Its subagents run on whatever " + agent + " gives them."
	case m == "inherit":
		d.subagentModelLine = "inherit  (" + src + ") — no CLAUDE_CODE_SUBAGENT_MODEL is set, and an inherited one is cleared: a subagent that names no model runs on the executor's"
	default:
		d.subagentModelLine = m + " → CLAUDE_CODE_SUBAGENT_MODEL=" + m + "  (" + src + ") — a subagent that names no model runs on it; one that pins its own keeps it"
	}
}
