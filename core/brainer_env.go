package main

// WHAT A BRAINER OF A LANE STARTS WITH, resolved once for both backends
// (herdr-app-brain): the environment pairs, the account its claude runs under,
// and the engram proxy its claude is given. The TUI passes the pairs as
// `--env K=V` to `workspace create`/`tab create`; an herdr-core terminal gets
// them as `env K=V …` in its argv, because the core runs the program with ITS
// environment and not the caller's (MEASURED 2026-11-08, P3). One resolution,
// two spellings, so the two backends cannot drift.

import "strings"

// brainerLaunch is a lane's brainer setup.
type brainerLaunch struct {
	pairs                []string // K=V, in today's order
	accountFlag, acctDir string
	engramProxy, mcpBin  string
}

// planBrainer is the account and the environment block of `brain <lane>`.
func (b *brainRun) planBrainer(proj string) (brainerLaunch, bool) {
	var p brainerLaunch
	// THE ACCOUNT, FOR THE BRAINER, from the same table `hw` reads for executors.
	// A relaunch (--in-pane) IS THE SAME LAUNCH: the lane's declared account is carried like `brain <lane> --resume`
	// carries it, because a pane shell can lack the CLAUDE_CONFIG_DIR its claude ran under (MEASURED 2026-10-31: `brain
	// relaunch --all` left two personal-account brainers bare shells, "No conversation found").
	accountFlag, acctOK := b.accountEnv(proj, b.kind) // # MUTATION-ANCHOR: go-1231-M01
	if !acctOK {
		return p, false
	}
	p.accountFlag = accountFlag
	p.acctDir = strings.TrimPrefix(accountFlag, "CLAUDE_CONFIG_DIR=")
	// A LANE WITH THE DEFAULT ACCOUNT KEEPS THE PANE'S, NOT THE CALLER'S.
	if b.inPane != "" && accountFlag != "" {
		// the lane's account, resolved above, is the pane's too
	} else if b.inPane != "" && b.inproc {
		p.acctDir = b.getenv("CLAUDE_CONFIG_DIR")
	} else if b.inPane != "" { // # MUTATION-ANCHOR: go-806-M03
		d, ok := b.paneConfigDir(b.inPane)
		if !ok {
			d = ""
			b.errf("brain: could not read %s's CLAUDE_CONFIG_DIR; its folder-trust and first-run checks used the default account.\n", b.inPane)
		}
		p.acctDir = d
	}
	p.pairs = []string{"ENGRAM_PROJECT=" + b.engramLabel}
	p.engramProxy = b.binDir + "/engram-label-proxy"
	if !isExecutable(p.engramProxy) { // # MUTATION-ANCHOR: go-422-M02
		b.errf("brain: the engram label proxy is not on disk at %s; a brainer is not opened with an unguarded engram\n", p.engramProxy)
		return p, false
	}
	p.mcpBin = b.a.lookPath("engram")
	if p.mcpBin == "" {
		p.mcpBin = "engram"
	}
	if b.kind == "opencode" {
		p.pairs = append(p.pairs, "OPENCODE_CONFIG_CONTENT="+jqCompact(map[string]any{}, p.engramProxy, b.engramLabel, p.mcpBin)) // # MUTATION-ANCHOR: go-422-M03
	}
	if accountFlag != "" {
		p.pairs = append(p.pairs, accountFlag) // # MUTATION-ANCHOR: go-122-M02
	}
	// THE COCKPIT MOD FINDS ITS STATE FILE FROM THE PANE'S OWN ENVIRONMENT.
	if b.kind == "claude" && b.work != "" { // # MUTATION-ANCHOR: go-803-M08
		p.pairs = append(p.pairs, "HW_COCKPIT_WORK="+b.work)
	}
	if b.kind == "opencode" {
		p.pairs = append(p.pairs, "OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true")
	}
	return p, true
}

// withEngramSession registers the brainer's engram session and hands the pane its id (BRAIN_ENGRAM_SESSION). It is called
// once the launch cannot be refused any more: a refusal before it posts no session and prints no engram line.
func (b *brainRun) withEngramSession(p brainerLaunch) brainerLaunch {
	b.engramRegister()
	if b.engramSession != "" {
		// right after ENGRAM_PROJECT (before OPENCODE_CONFIG_CONTENT, which an opencode lane carries: env order has no effect)
		p.pairs = append(p.pairs[:1], append([]string{"BRAIN_ENGRAM_SESSION=" + b.engramSession}, p.pairs[1:]...)...)
	}
	return p
}

// brainerAgentArgs is the brainer's own argv, minus the program: `--` and its
// flags, shared by the herdr `agent start` and the herdr-core argv. hc says the
// first-run advice but cannot type into a tab that does not exist, so the tab
// focus of that advice is the TUI's alone.
func (b *brainRun) brainerAgentArgs(p brainerLaunch, pane, tab string, hc bool) ([]string, int) {
	var agentArgs []string
	switch {
	case b.kind == "claude" && b.inproc: // the same settings claude_session_env_prep types, set where they take effect
		b.unsetenv("CLAUDE_CODE_CHILD_SESSION")
		b.setenv("CLAUDE_CODE_FORCE_SESSION_PERSISTENCE", "1") // # MUTATION-ANCHOR: go-892-M03
		b.setenv("CLAUDE_CODE_BASH_EDIT_DIFF", "0")            // # MUTATION-ANCHOR: go-892-M05
	case b.kind == "claude" && !hc:
		b.sessionEnvPrep(pane) // # MUTATION-ANCHOR: go-810-M01
	}
	if b.kind == "opencode" {
		port := freePort()
		agentArgs = []string{"--", "--port", port}
		if b.permissions == "skip" {
			agentArgs = []string{"--", "--auto", "--port", port} // # MUTATION-ANCHOR: go-803-M05
		}
		return agentArgs, 0
	}
	// THE FOLDER-TRUST QUESTION, ANSWERED THE WAY `hw` ANSWERS IT FOR EXECUTORS.
	b.trustDir(b.dir, p.acctDir) // # MUTATION-ANCHOR: go-122-M03
	// AND WHAT IS THE PERSON'S, SAID BEFORE THE PANE STOPS ON IT.
	firstRun := strings.Join(b.firstRunPending(p.acctDir), " ")
	if firstRun != "" { // # MUTATION-ANCHOR: go-188-M01
		b.errf("brain: Claude Code's first run is not finished in this account (%s).\n", firstRun)
		b.errf("brain: the brainer's pane opens on it and answering it is yours, there: finish the welcome and\n")
		b.errf("brain: login, and accept the Bypass Permissions warning. Nothing here answers it for you.\n")
		if !hc {
			b.herdr(brainCmd{NoOut: true}, "tab", "focus", tab)
		}
	}
	cache := b.getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = b.home + "/.cache"
	}
	mcpFile := cache + "/hw/engram-mcp-" + b.engramLabel + ".json"
	if err := b.writeEngramMCP(mcpFile, p.engramProxy, p.mcpBin); err != nil {
		b.errf("brain: could not write the engram proxy's --mcp-config at %s\n", mcpFile)
		return nil, 1
	}
	agentArgs = []string{"--", "--mcp-config", mcpFile}
	if b.permissions == "skip" { // # MUTATION-ANCHOR: go-561-M01
		agentArgs = []string{"--", "--dangerously-skip-permissions", "--mcp-config", mcpFile}
	}
	// THE COCKPIT MOD rides in as a session-only plugin folder.
	if b.resume != "" && !b.noConversation {
		agentArgs = append(agentArgs, "--resume", b.resume)
	}
	cockpitOn := !b.noCockpit                                   // # MUTATION-ANCHOR: go-803-M03
	cockpitOn = cockpitOn && b.getenv("FOREMAN_COCKPIT") != "0" // # MUTATION-ANCHOR: go-803-M02
	// UNDER THE APP THE SIDE PANEL IS THE COCKPIT: the plugin folder paints the same panel inside the Claude pane, so an
	// app (hc) brainer shows two (herdr-app-gaps-fix G-A, measured 2026-11-10). The writer the side panel reads is kept
	// (cockpitWriter does not look at this), and no knob is needed to tell them apart.
	cockpitOn = cockpitOn && !hc
	if cockpitOn {
		cockpitDir := cleanJoin(b.binDir+"/..", "cockpit")
		floor, _ := b.cockpitFloor()
		ver := b.claudeVersion()
		switch {
		case !brFileExists(cockpitDir + "/.claude-plugin/plugin.json"):
			b.errf("brain: no cockpit mod at %s; running without it\n", cockpitDir)
		case floor == "":
			b.errf("brain: no version floor for the cockpit (lib/hw/deps.conf is missing); running without it\n")
		case ver == "" || !versionAtLeast(ver, floor): // # MUTATION-ANCHOR: go-803-M01
			v := ver
			if v == "" {
				v = "none"
			}
			b.errf("brain: cockpit needs claude >= %s (found %s); running without it\n", floor, v) // # MUTATION-ANCHOR: go-803-M04
		default:
			agentArgs = append(agentArgs, "--plugin-dir", cockpitDir) // # MUTATION-ANCHOR: go-803-M07
		}
	}
	return agentArgs, 0
}
