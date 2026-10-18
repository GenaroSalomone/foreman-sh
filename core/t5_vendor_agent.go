package main

// Train T5, area B: the permissions and the vendor's command line.
// bin/project-spaces.sh: hw_permissions_file, hw_permissions_resolve; bin/hw: _build_agent_args, _free_channel_port.

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// vdSpaceCut is `tr -d '[:space:]'`.
func vdSpaceCut(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return -1
		}
		return r
	}, s)
}

// vdEnvSet is `${VAR+x} = x`: set in the environment, even empty.
func (d *dsp) vdEnvSet(k string) bool {
	if _, ok := d.envOver[k]; ok {
		return true
	}
	if d.getenv(k) != "" {
		return true
	}
	_, ok := os.LookupEnv(k)
	return ok
}

// permissionsResolve is hw_permissions_resolve: sets permissions/permissionsSrc, false (with permissionsErr) when
// the value that won is neither ask nor skip.
func (d *dsp) permissionsResolve(flag string) bool {
	v, src := flag, ""
	d.permissionsErr = ""
	cfg := d.env("XDG_CONFIG_HOME")
	if cfg == "" { // # MUTATION-ANCHOR: 844-B32
		cfg = d.env("HOME") + "/.config"
	}
	f := cfg + "/hw/permissions" // hw_permissions_file
	switch {
	case v != "":
		src = "chosen (--permissions)"
	case d.env("HW_PERMISSIONS") != "": // # MUTATION-ANCHOR: 844-B33
		v, src = d.env("HW_PERMISSIONS"), "HW_PERMISSIONS"
	case isFile(f):
		data, _ := os.ReadFile(f)
		v, src = vdSpaceCut(string(data)), "machine config "+f // # MUTATION-ANCHOR: 844-B34
	default:
		v, src = "skip", "default (recommended)"
	}
	if v == "ask" || v == "skip" { // # MUTATION-ANCHOR: 844-B35
		d.permissions, d.permissionsSrc = v, src
		return true
	}
	d.permissions, d.permissionsSrc = "skip", src
	d.permissionsErr = "permissions must be ask or skip (got: '" + v + "', from " + src + ")"
	return false
}

// vdFreePort is _free_channel_port: the port the kernel picks for 127.0.0.1:0.
func vdFreePort() string {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return ""
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

// vdJqAgentDefined is `jq -e --arg a NAME '.agent[$a]' FILE >/dev/null 2>&1`: true when the last result of the
// last value is neither null nor false; an unreadable file, a syntax error, or an index that errors is false.
func vdJqAgentDefined(path, name string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	vals, perr := parseStream(data)
	if perr != nil {
		return false
	}
	ok := false // no output at all is exit 4
	for _, v := range vals {
		agent, good := v.get("agent")
		if !good {
			ok = false
			continue
		}
		r, good := agent.get(name)
		if !good {
			ok = false
			continue
		}
		ok = r.truthy() // # MUTATION-ANCHOR: 844-B36
	}
	return ok
}

// vdValidEffort is the `low|medium|high|xhigh|max` pattern.
func vdValidEffort(e string) bool {
	switch e {
	case "low", "medium", "high", "xhigh", "max":
		return true
	}
	return false
}

// buildAgentArgs is _build_agent_args: the vendor's command line, as one string of words.
func (d *dsp) buildAgentArgs() {
	d.agentArgs = ""
	d.modelPinned = ""
	d.opencodeExecutorConfig = ""
	d.opencodeEffortOverlay = ""
	d.opencodePrimaryAgent = ""
	perm := orDefaultStr(d.permissions, "skip")
	switch d.agent {
	case "claude":
		if perm == "skip" { // # MUTATION-ANCHOR: 844-B37
			d.agentArgs = "--dangerously-skip-permissions"
		}
		if d.model != "" {
			if strings.HasPrefix(d.model, "-") || strings.ContainsAny(d.model, " \t\r\n*?[") { // # MUTATION-ANCHOR: 844-B38
				d.die("hw cannot safely pass this Claude model as one literal argument (got: " + d.model + ")")
			}
			switch {
			case d.model == "fable" || d.model == "opus" || d.model == "sonnet" || d.model == "haiku": // # MUTATION-ANCHOR: 844-B39
			case strings.HasPrefix(d.model, "claude-"): // # MUTATION-ANCHOR: 844-B40
			case strings.Contains(d.model, "/"): // # MUTATION-ANCHOR: 844-B41
				d.die("that is a provider/model id, which is opencode's form, not claude's (got: " + d.model + "). Use an alias (opus, sonnet, haiku, fable) or a full claude-* name — or launch it with --agent opencode.")
			default:
				d.warn("claude model '" + d.model + "' is not recognized by hw; availability is unverified, passing it to the vendor unchanged.") // # MUTATION-ANCHOR: 844-B42
			}
			d.modelPinned = d.modelPinFor(d.model)
			d.agentArgs += " --model " + orDefaultStr(d.modelPinned, d.model) // # MUTATION-ANCHOR: 844-B43
		}
		if d.effort != "" {
			if !vdValidEffort(d.effort) { // # MUTATION-ANCHOR: 844-B44
				d.die("--effort must be one of: low medium high xhigh max (got: " + d.effort + ")")
			}
			d.agentArgs += " --effort " + d.effort // # MUTATION-ANCHOR: 844-B45
		}
	case "opencode":
		if d.model != "" {
			if !strings.Contains(d.model, "/") { // # MUTATION-ANCHOR: 844-B46
				d.die("opencode needs provider/model, not a bare alias (got: " + d.model + "). Try: opencode models")
			}
			d.agentArgs += " --model " + d.model // # MUTATION-ANCHOR: 844-B47
		}
		d.opencodeExecutorConfig = d.vdLane(d.proj, "opencode_config_dir")
		if d.opencodeExecutorConfig != "" && !isFile(d.opencodeExecutorConfig+"/plugin/deny-repo-writes.js") { // # MUTATION-ANCHOR: 844-B48
			d.die("lane " + d.proj + " names opencode_config_dir " + d.opencodeExecutorConfig + ", but it has no plugin/deny-repo-writes.js — the executor would run unguarded. Re-run install.sh for this lane, or remove the field from projects.json")
		}
		d.opencodeAgentPort = vdFreePort()
		if perm == "skip" { // # MUTATION-ANCHOR: 844-B49
			d.agentArgs += " --auto"
		}
		d.agentArgs += " --port " + d.opencodeAgentPort // # MUTATION-ANCHOR: 844-B50
		var defAgent, role string
		switch {
		case d.sdd == "none" && (strings.HasSuffix(d.model, "/gpt-5.6-sol") || strings.Contains(d.model, "/gpt-5.6-sol-")): // # MUTATION-ANCHOR: 844-B51
			defAgent, role = "sol-orchestrator", "Sol orchestration primary; delegates economical work"
		case d.sdd == "none" && (strings.HasSuffix(d.model, "/gpt-6-astra") || strings.Contains(d.model, "/gpt-6-astra-")): // # MUTATION-ANCHOR: 844-B52
			defAgent, role = "astra-orchestrator", "Astra orchestration primary; delegates economical work"
		default:
			defAgent, role = "direct-worker", "direct worker; no orchestration"
		}
		ocAgent := defAgent
		if d.vdEnvSet("HW_OPENCODE_AGENT") { // # MUTATION-ANCHOR: 844-B53
			ocAgent = d.env("HW_OPENCODE_AGENT")
			role = "explicit HW_OPENCODE_AGENT override" // # MUTATION-ANCHOR: 844-B54
		}
		home := d.env("HOME")
		if ocAgent != "" { // # MUTATION-ANCHOR: 844-B55
			ocConf := home + "/.config/opencode/opencode.json"
			if vdJqAgentDefined(ocConf, ocAgent) {
				d.agentArgs += " --agent " + ocAgent
				d.opencodePrimaryAgent = ocAgent // # MUTATION-ANCHOR: 844-B56
				d.info("opencode → --agent " + ocAgent + " (" + role + ")")
			} else {
				d.warn("opencode wanted --agent " + ocAgent + " (" + role + "), but it is not defined in " + ocConf) // # MUTATION-ANCHOR: 844-B57
				d.warn("continuing without --agent — opencode will use its configured default_agent")
			}
		} else {
			d.info("opencode → no --agent (HW_OPENCODE_AGENT is explicitly empty)")
		}
		d.opencodeEffortOverlay = ""
		if d.effort != "" {
			if !vdValidEffort(d.effort) { // # MUTATION-ANCHOR: 844-B58
				d.die("--effort must be one of: low medium high xhigh max (got: " + d.effort + ")")
			}
			if d.opencodePrimaryAgent == "" { // # MUTATION-ANCHOR: 844-B59
				d.die("--effort needs a named, resolved primary to attach it to, but none is being passed to opencode (either HW_OPENCODE_AGENT is explicitly empty, or '" + ocAgent + "' is not defined in " + home + "/.config/opencode/opencode.json — see the warning above). Fix the agent name, unset the override, or drop --effort.")
			}
			if strings.Trim(d.opencodePrimaryAgent, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") != "" { // # MUTATION-ANCHOR: 844-B60
				d.die("--effort cannot carry agent name '" + d.opencodePrimaryAgent + "': the overlay needs a name of [A-Za-z0-9._-] only, to survive the --here flag list and the crash restore. Rename the agent or drop --effort.")
			}
			d.opencodeEffortOverlay = `{"agent":{"` + d.opencodePrimaryAgent + `":{"reasoningEffort":"` + d.effort + `"}}}` // # MUTATION-ANCHOR: 844-B61
		}
	case "codex":
		if perm == "ask" { // # MUTATION-ANCHOR: 844-B62
			d.agentArgs += " -a on-request"
		}
		if d.model != "" {
			if strings.Contains(d.model, "/") { // # MUTATION-ANCHOR: 844-B63
				d.die("codex needs a bare model id, not opencode's provider/model form (got: " + d.model + "). Use --model " + d.model[strings.LastIndex(d.model, "/")+1:] + " with --agent codex.")
			}
			d.agentArgs += " --model " + d.model // # MUTATION-ANCHOR: 844-B64
		}
		if d.effort != "" {
			if !vdValidEffort(d.effort) && d.effort != "ultra" { // # MUTATION-ANCHOR: 844-B65
				d.die("codex --effort must be one of: low medium high xhigh max ultra (got: " + d.effort + ")")
			}
			d.agentArgs += " -c model_reasoning_effort=" + d.effort // # MUTATION-ANCHOR: 844-B66
		}
	}
	d.agentArgs = strings.TrimPrefix(d.agentArgs, " ") // # MUTATION-ANCHOR: 844-B67
	if d.agentArgs != "" {
		d.info("agent args: " + d.agentArgs)
	}
	// THE CHEAP EXPLORER (lib/hw/explorer.sh), AFTER the line above so it stays the flags the operator chose.
	switch d.agent {
	case "codex":
		d.agentArgs += d.explorer().codexArgs()
	}
	d.agentArgs = strings.TrimPrefix(d.agentArgs, " ")
	d.explorerResolve()
}
