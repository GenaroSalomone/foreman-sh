package main

// The body of bin/brain once a lane is chosen: the flags, the lookup of the space and the tab, the three things that
// can happen to a brainer that is already there (focus it, replace its agent, revive a bare shell), and the creation
// and start of one that is not.

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// launch is everything after `proj=…` in bin/brain. args are the arguments after the lane.
func (b *brainRun) launch(args []string) int {
	proj := b.proj
	b.kind = b.tab.get(proj, "vendor")
	if b.kind == "" {
		b.kind = "claude"
	}
	// A lane whose vendor is opencode names who runs its EXECUTORS: the brainer is opencode only where the lane ships one.
	if b.kind == "opencode" && !brFileExists(b.brain+"/"+proj+"/opencode.json") { // # MUTATION-ANCHOR: go-206-M01
		b.kind = "claude"
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--opencode", "-o":
			b.kind = "opencode"
		case "--claude", "-c":
			b.kind = "claude"
		case "--no-cockpit":
			b.noCockpit = true
		case "--resume":
			b.kind = "claude"
			i++
			if i < len(args) {
				b.resume = args[i]
			} else {
				b.resume = ""
			}
			if b.resume == "" {
				b.errf("--resume needs a session id\n")
				return 64
			}
		case "--in-pane":
			i++
			if i < len(args) {
				b.inPane = args[i]
			} else {
				b.inPane = ""
			}
			if b.inPane == "" {
				b.errf("--in-pane needs a pane id\n")
				return 64
			}
		case "--reset":
			b.reset = true
		case "--force":
			b.force = true
		case "--self":
			b.self = true
		case "--await-idle":
			i++
			if i < len(args) {
				b.awaitPane = args[i]
			} else {
				b.awaitPane = ""
			}
			if b.awaitPane == "" {
				b.errf("--await-idle needs a pane id\n")
				return 64
			}
		default:
			b.errf("unknown flag: %s  (try: --opencode, --claude, --no-cockpit, --resume, --reset, --self, --force)\n", args[i])
			return 64
		}
	}
	if b.resume != "" && b.kind != "claude" {
		b.errf("--resume is a Claude Code conversation: it does not apply to a %s brainer\n", b.kind)
		return 64
	}
	if b.resume != "" && (b.reset || b.self || b.awaitPane != "") {
		b.errf("--resume replaces the agent keeping its conversation; it does not combine with --reset, --self or --await-idle\n")
		return 64
	}
	if b.inPane != "" && b.resume == "" {
		b.errf("--in-pane means nothing without --resume\n")
		return 64
	}
	if b.force && !b.reset {
		b.errf("--force means nothing without --reset\n")
		return 64
	}
	if b.self && !b.reset {
		b.errf("--self means nothing without --reset\n")
		return 64
	}
	if b.awaitPane != "" && !b.reset {
		b.errf("--await-idle means nothing without --reset\n")
		return 64
	}
	// THE DETACHED ARM IS NOT ITSELF A SELF-RESET.
	if b.self && b.awaitPane != "" {
		b.errf("--self and --await-idle are the two halves of one reset, not one call\n")
		return 64
	}

	// A SESSION WITH NO TRANSCRIPT HAS NO CONVERSATION: decided before any key is sent.
	if b.resume != "" {
		// # MUTATION-ANCHOR: go-896-M01
		_, has := b.sessionTranscript(b.resume)
		if !has {
			b.noConversation = true
		}
		// # MUTATION-ANCHOR-END: go-896-M01
		if b.noConversation {
			b.errf("brain: session %s has no transcript under any account on this machine (it never had a turn), so there is no conversation to resume — starting a NEW one in the same pane.\n", b.resume)
		}
	}

	b.dir = b.brain + "/" + proj
	if !brFileExists(b.dir + "/CLAUDE.md") {
		b.errf("no brainer at %s\n", b.dir)
		return 1
	}
	if b.kind == "opencode" {
		if !brFileExists(b.dir + "/opencode.json") {
			b.errf("no opencode brainer at %s (missing opencode.json)\n", b.dir)
			return 1
		}
	}

	// THE ENGRAM LABEL IS PINNED HERE: the lane's `engram` field, `brain` when it has none.
	b.engramLabel = b.tab.get(proj, "engram")
	if b.engramLabel == "" {
		b.engramLabel = "brain"
	}
	b.engramRegister()

	b.space = b.tab.get(proj, "space")
	b.tabLabel = "brainer"
	if b.kind == "opencode" {
		b.tabLabel = "brainer:oc"
	}

	ws := b.mustList("a workspace labelled "+b.space, []string{"result", "workspaces"},
		func(m jmap) (string, bool) {
			if l, ok := m["label"].(string); ok && l == b.space {
				return bjRaw(m["workspace_id"]), true
			}
			return "", false
		}, "workspace", "list")

	// `--in-pane <pane>` (brain relaunch): the workspace and tab are the pane's own, read from herdr.
	inTab := ""
	if b.inPane != "" {
		inJSON := b.mustJSON("the panes of "+b.inPane, []string{"result", "panes"}, "pane", "list")
		v, _ := bjParse(inJSON)
		ids := ""
		list, _ := bjGet(v, "result", "panes").([]any)
		for _, e := range list {
			p, isMap := e.(jmap)
			if !isMap {
				continue
			}
			if id, _ := p["pane_id"].(string); id == b.inPane {
				ids = bjRaw(p["workspace_id"]) + "\t" + bjRaw(p["tab_id"])
				break
			}
		}
		if ids == "" {
			b.errf("brain: %s is not a pane herdr knows. Nothing was sent and nothing was restarted.\n", b.inPane)
			return 1
		}
		i := strings.Index(ids, "\t")
		ws, inTab = ids[:i], ids[i+1:]
	}

	var tab, pane, resetPane, resetTab string
	revive := false
	resetCwdUnfixed := false
	var paneAgent, paneStatus string

	// Already open? Focus the brainer's TAB, not the space.
	if ws != "" {
		if inTab != "" {
			tab = inTab
		} else {
			tab = b.mustList("the "+b.tabLabel+" tab in "+ws, []string{"result", "tabs"},
				func(m jmap) (string, bool) {
					w, _ := m["workspace_id"].(string)
					l, _ := m["label"].(string)
					if w == ws && l == b.tabLabel {
						return bjRaw(m["tab_id"]), true
					}
					return "", false
				}, "tab", "list")
		}
		if tab != "" {
			panesJSON := b.mustJSON("the panes of "+tab, []string{"result", "panes"}, "pane", "list")
			tp, _ := brainTabPane(panesJSON, tab)
			pane, b.paneCwd, paneAgent, paneStatus = tp.pane, tp.cwd, tp.agent, tp.status
			b.verdict = ""
			if pane != "" {
				b.verdict = brainerVerdict(panesJSON, pane, b.dir)
			}
			// A RESUME OF AN OPEN BRAINER IS A REPLACEMENT OF ITS AGENT, in its own pane.
			if b.resume != "" && pane != "" {
				if b.inPane != "" && b.inPane != pane {
					b.errf("brain: %s is not the brainer pane of its tab (that is %s). Nothing was sent and nothing was restarted.\n", b.inPane, pane)
					return 1
				}
				b.reset = true
			}

			switch {
			case b.reset:
				if pane == "" {
					b.errf("brain: %s / %s (%s) has no single pane labelled `brain`, so there is no one agent to\n", b.space, b.tabLabel, tab)
					b.errf("brain: replace. Nothing was sent and nothing was restarted. Look at that tab first.\n")
					return 1
				}
				if b.awaitPane != "" {
					var rc int
					panesJSON, tp, rc = b.awaitIdle(tab, pane, panesJSON)
					if rc != 0 {
						return rc
					}
					pane, b.paneCwd, paneAgent, paneStatus = tp.pane, tp.cwd, tp.agent, tp.status
				}
				if b.self {
					return b.armSelfReset(pane, panesJSON)
				}
				if rc := b.livenessGate(pane, paneAgent, paneStatus); rc != 0 {
					return rc
				}
				// A RESUME KEEPS THE CONVERSATION the executors report to, so the gate has nothing to refuse.
				if b.resume == "" && !b.blockerGate(proj, pane, "--reset", panesJSON) {
					return 1
				}
				// CAPTURE IS NOT AUTHORITY: the status is re-read immediately before the first key.
				// # MUTATION-ANCHOR: go-137-M06
				now := b.agentGet(pane)
				nowStatus := bjRawOr(now, "result", "agent", "agent_status")
				switch {
				case agentNotFound(now):
					paneAgent = ""
				case nowStatus != "":
					if nowStatus != "idle" && nowStatus != "done" {
						ps := paneStatus
						if ps == "" {
							ps = "absent"
						}
						b.errf("brain: %s was %s when the pane list was read and is %s now — brain --reset refuses\n", pane, ps, nowStatus)
						b.errf("brain: on the fresher reading. Nothing was sent and nothing was restarted.\n")
						return 1
					}
					if paneAgent == "" {
						paneAgent = "agent"
					}
				default:
					b.errf("brain: %s could not be read by `herdr agent get` immediately before the reset, so\n", pane)
					b.errf("brain: whether anything is running in it is UNKNOWN. An unreachable reader is not\n")
					b.errf("brain: evidence that a restart is safe.\n")
					b.errf("brain: Nothing was sent and nothing was restarted.\n")
					return 1
				}
				// # MUTATION-ANCHOR-END: go-137-M06
				// A DRAFT IN THE INPUT BOX TURNS /exit INTO TEXT: refused before any key is sent.
				if paneAgent != "" && b.kind == "claude" {
					// # MUTATION-ANCHOR: go-894-M01
					draft, ok := b.inputDraft(pane)
					if !ok {
						b.errf("brain: the input box of %s could not be read, so whether it holds an unsent draft is UNKNOWN —\n", pane)
						b.errf("brain: /exit would be appended to it. Nothing was sent and nothing was restarted.\n")
						return 1
					}
					if draft != "" {
						b.errf("brain: %s has an unsent draft in its input box («%s») and /exit typed after it would be SUBMITTED with it as one message, not run.\n", pane, brainCutChars(draft, 60))
						b.errf("brain: send or clear that draft in the pane, then relaunch. Nothing was sent and nothing was restarted.\n")
						return 1
					}
					// # MUTATION-ANCHOR-END: go-894-M01
				}
				if paneAgent != "" {
					if rc := b.replaceAgent(pane, paneAgent); rc != 0 {
						return rc
					}
				} else {
					b.ownPaneGate(pane)
					b.errf("brain: %s is a bare shell (no agent) — starting the %s brainer in it.\n", pane, b.kind)
				}
				resetCwdUnfixed = b.relocate(pane, tab)
				// THE HANDLES, RE-ASSERTED.
				b.herdrOK("tab", "rename", tab, b.tabLabel)
				b.herdrOK("pane", "rename", pane, "brain")
				resetPane, resetTab = pane, tab
			case pane != "" && b.verdict == "ok" && agentNotFound(b.agentGet(pane)): // # MUTATION-ANCHOR: go-188-M02
				// THE TAB IS THERE AND THE BRAINER IS NOT.
				b.ownPaneGate(pane)
				b.errf("brain: %s is a bare shell — the %s brainer's agent has exited. Starting a new one in the same pane.\n", pane, proj)
				resetPane, resetTab = pane, tab
				revive = true
			default:
				if r := b.herdr(brainCmd{NoOut: true, ErrTo: true}, "tab", "focus", tab); r.RC != 0 {
					return 1
				}
				b.outf("focused %s / %s (%s)\n", b.space, b.tabLabel, tab)
				// # MUTATION-ANCHOR: go-137-M05
				if strings.HasPrefix(b.verdict, "mismatch") {
					b.errf("brain: WARNING — %s is NOT the %s brainer: %s\n", pane, proj, afterTab(b.verdict))
					b.errf("brain: expected label=brain cwd=%s. It loads the wrong lane's CLAUDE.md and guard\n", b.dir)
					b.errf("brain: rules, and `hw` refuses to dispatch from it. Fix it with: brain %s --reset\n", proj)
					return 3
				}
				// # MUTATION-ANCHOR-END: go-137-M05
				b.cockpitWriter(b.kind, pane)
				b.flushOutbox(proj, pane) // # MUTATION-ANCHOR: go-243-M01
				return 0
			}
		}
	}
	// A refusal that has already acted is worse than no refusal.
	if b.reset && resetPane == "" { // # MUTATION-ANCHOR: go-137-M04
		wsText := ws
		if wsText == "" {
			wsText = "the " + b.space + " space, which does not exist"
		}
		b.errf("brain: there is no %s brainer to reset (no %s tab in %s). Open one with: brain %s\n", proj, b.tabLabel, wsText, proj)
		b.errf("brain: Nothing was sent and nothing was restarted.\n")
		return 1
	}

	// THE ACCOUNT, FOR THE BRAINER, from the same table `hw` reads for executors.
	accountFlag := ""
	if b.inPane == "" {
		f, ok := b.accountEnv(proj, b.kind)
		if !ok {
			return 1
		}
		accountFlag = f
	}
	acctDir := strings.TrimPrefix(accountFlag, "CLAUDE_CONFIG_DIR=")
	// THE PANE'S ACCOUNT, NOT THE CALLER'S.
	if b.inPane != "" && b.inproc {
		acctDir = b.getenv("CLAUDE_CONFIG_DIR")
	} else if b.inPane != "" {
		d, ok := b.paneConfigDir(b.inPane)
		if !ok {
			d = ""
			b.errf("brain: could not read %s's CLAUDE_CONFIG_DIR; its folder-trust and first-run checks used the default account.\n", b.inPane)
		}
		acctDir = d
	}
	workspaceEnv := []string{"--env", "ENGRAM_PROJECT=" + b.engramLabel}
	engramProxy := b.binDir + "/engram-label-proxy"
	if !isExecutable(engramProxy) { // # MUTATION-ANCHOR: go-422-M02
		b.errf("brain: the engram label proxy is not on disk at %s; a brainer is not opened with an unguarded engram\n", engramProxy)
		return 1
	}
	engramMCPBin := b.a.lookPath("engram")
	if engramMCPBin == "" {
		engramMCPBin = "engram"
	}
	if b.kind == "opencode" {
		workspaceEnv = append(workspaceEnv, "--env", "OPENCODE_CONFIG_CONTENT="+jqCompact(map[string]any{}, engramProxy, b.engramLabel, engramMCPBin)) // # MUTATION-ANCHOR: go-422-M03
	}
	if b.engramSession != "" {
		workspaceEnv = append(workspaceEnv, "--env", "BRAIN_ENGRAM_SESSION="+b.engramSession)
	}
	if accountFlag != "" {
		workspaceEnv = append(workspaceEnv, "--env", accountFlag) // # MUTATION-ANCHOR: go-122-M02
	}
	// THE COCKPIT MOD FINDS ITS STATE FILE FROM THE PANE'S OWN ENVIRONMENT.
	if b.kind == "claude" && b.work != "" { // # MUTATION-ANCHOR: go-803-M08
		workspaceEnv = append(workspaceEnv, "--env", "HW_COCKPIT_WORK="+b.work)
	}
	if b.kind == "opencode" {
		workspaceEnv = append(workspaceEnv, "--env", "OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true")
	}

	var ok bool
	switch {
	case b.reset || revive:
		tab, pane = resetTab, resetPane
		// THE ENV ABOVE IS NOT RE-APPLIED, and HW_COCKPIT_WORK is the one the mod cannot do without.
		if b.kind == "claude" && b.work != "" {
			if b.inproc {
				b.setenv("HW_COCKPIT_WORK", b.work) // # MUTATION-ANCHOR: go-892-M02
			} else if !b.herdrOK("pane", "run", pane, "export HW_COCKPIT_WORK="+reviveQuote(b.work)) { // # MUTATION-ANCHOR: go-804-M01
				b.errf("brain: could not give %s HW_COCKPIT_WORK; the cockpit mod may not find its state file there.\n", pane)
			}
		}
		// THE ACCOUNT TOO, FOR THE SAME REASON.
		if b.kind == "claude" && accountFlag != "" && b.inproc {
			b.setenv("CLAUDE_CONFIG_DIR", acctDir)
			b.errf("brain: %s runs under account CLAUDE_CONFIG_DIR=%s (the lane's default)\n", pane, acctDir)
		} else if b.kind == "claude" && accountFlag != "" { // # MUTATION-ANCHOR: go-829-M02
			if b.herdrOK("pane", "run", pane, "export CLAUDE_CONFIG_DIR="+reviveQuote(acctDir)) { // # MUTATION-ANCHOR: go-829-M01
				b.errf("brain: %s runs under account CLAUDE_CONFIG_DIR=%s (the lane's default)\n", pane, acctDir)
			} else {
				b.errf("brain: could not give %s CLAUDE_CONFIG_DIR=%s; refusing to start the brainer under another account.\n", pane, acctDir)
				return 1
			}
		}
	case ws != "":
		args := append([]string{"tab", "create", "--workspace", ws, "--cwd", b.dir}, workspaceEnv...)
		args = append(args, "--no-focus")
		raw := brTrimNL(b.herdr(brainCmd{Merge: true}, args...).Out)
		v, _ := bjParse(raw)
		tab = rawOrEmpty(v, bjAlt(bjGet(v, "result", "tab", "tab_id"), bjGet(v, "result", "tab_id")))
		pane = rawOrEmpty(v, bjAlt(bjGet(v, "result", "pane", "pane_id"), bjGet(v, "result", "root_pane", "pane_id")))
		if tab == "" || tab == "null" {
			b.errf("herdr tab create failed in %s: %s\n", ws, brainFirstLines(raw, 2))
			return 1
		}
	default:
		args := append([]string{"workspace", "create", "--cwd", b.dir, "--label", b.space}, workspaceEnv...)
		args = append(args, "--no-focus")
		raw := brTrimNL(b.herdr(brainCmd{Merge: true}, args...).Out)
		v, _ := bjParse(raw)
		ws = rawOrEmpty(v, bjGet(v, "result", "workspace", "workspace_id"))
		if ws == "" || ws == "null" {
			b.errf("herdr workspace create failed: %s\n", brainFirstLines(raw, 2))
			return 1
		}
		tab = rawOrEmpty(v, bjAlt(bjGet(v, "result", "tab", "tab_id"), bjGet(v, "result", "root_pane", "tab_id")))
		pane = rawOrEmpty(v, bjAlt(bjGet(v, "result", "root_pane", "pane_id"), bjGet(v, "result", "pane", "pane_id")))
		if pane == "" || pane == "null" {
			pl := b.herdr(brainCmd{}, "pane", "list", "--workspace", ws)
			pv, _ := bjParse(pl.Out)
			pane = ""
			if l, isArr := bjGet(pv, "result", "panes").([]any); isArr && len(l) > 0 {
				pane = bjRaw(bjGet(l[0], "pane_id"))
			}
		}
	}
	_ = ok
	if pane == "" || pane == "null" {
		b.errf("no pane to start the brainer in (%s)\n", ws)
		return 1
	}
	// The TAB label is the handle.
	// # MUTATION-ANCHOR: go-102-M02
	if tab != "" && tab != "null" {
		r := b.herdr(brainCmd{Merge: true}, "tab", "rename", tab, b.tabLabel)
		if r.RC != 0 {
			b.errf("brain: `herdr tab rename %s %s` failed, so this tab has no queryable label and the\n", tab, b.tabLabel)
			b.errf("brain: next `brain %s` will not find it — it will build a second brainer beside it.\n", proj)
			b.errf("brain: herdr said: %s\n", headLinesSpaced(brTrimNL(r.Out), 2))
			return 1
		}
	}
	// # MUTATION-ANCHOR-END: go-102-M02
	if r := b.herdr(brainCmd{NoOut: true, ErrTo: true}, "pane", "rename", pane, "brain"); r.RC != 0 {
		return 1
	}
	// skip or ask, resolved as hw resolves it.
	if !b.permissionsResolve() {
		b.errf("brain: %s\n", b.permissionsErr)
		return 1
	}
	if b.permissions != "skip" { // # MUTATION-ANCHOR: go-561-M02
		b.errf("brain: permissions: ask (%s) — the brainer will stop on permission prompts; answer them in its pane.\n", b.permissionsSrc)
	}
	var agentArgs []string
	switch {
	case b.kind != "claude":
	case b.inproc: // the same settings claude_session_env_prep types, set where they take effect
		b.unsetenv("CLAUDE_CODE_CHILD_SESSION")
		b.setenv("CLAUDE_CODE_FORCE_SESSION_PERSISTENCE", "1") // # MUTATION-ANCHOR: go-892-M03
		b.setenv("CLAUDE_CODE_BASH_EDIT_DIFF", "0")            // # MUTATION-ANCHOR: go-892-M05
	default:
		b.sessionEnvPrep(pane) // # MUTATION-ANCHOR: go-810-M01
	}
	if b.kind == "opencode" {
		port := freePort()
		agentArgs = []string{"--", "--port", port}
		if b.permissions == "skip" {
			agentArgs = []string{"--", "--auto", "--port", port} // # MUTATION-ANCHOR: go-803-M05
		}
	} else {
		// THE FOLDER-TRUST QUESTION, ANSWERED THE WAY `hw` ANSWERS IT FOR EXECUTORS.
		b.trustDir(b.dir, acctDir) // # MUTATION-ANCHOR: go-122-M03
		// AND WHAT IS THE PERSON'S, SAID BEFORE THE PANE STOPS ON IT.
		firstRun := strings.Join(b.firstRunPending(acctDir), " ")
		if firstRun != "" { // # MUTATION-ANCHOR: go-188-M01
			b.errf("brain: Claude Code's first run is not finished in this account (%s).\n", firstRun)
			b.errf("brain: the brainer's pane opens on it and answering it is yours, there: finish the welcome and\n")
			b.errf("brain: login, and accept the Bypass Permissions warning. Nothing here answers it for you.\n")
			b.herdr(brainCmd{NoOut: true}, "tab", "focus", tab)
		}
		cache := b.getenv("XDG_CACHE_HOME")
		if cache == "" {
			cache = b.home + "/.cache"
		}
		mcpFile := cache + "/hw/engram-mcp-" + b.engramLabel + ".json"
		if err := b.writeEngramMCP(mcpFile, engramProxy, engramMCPBin); err != nil {
			b.errf("brain: could not write the engram proxy's --mcp-config at %s\n", mcpFile)
			return 1
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
	}
	// IN THIS PANE: no `agent start` (it types the command into the shell brain is holding).
	if b.inproc {
		b.herdr(brainCmd{NoOut: true}, "tab", "focus", tab)
		b.cockpitWriter(b.kind, pane)
		b.spawn(b.selfPath(), []string{"hw-core", "brain", "__flush", proj, pane}, b.env)
		how := "resuming " + b.resume
		if b.resume == "" {
			how = "resuming a new conversation"
		}
		if b.noConversation {
			how = "starting a new conversation (" + b.resume + " had no transcript)"
		}
		b.errf("%s in %s / %s (pane %s, %s) — exec in this pane — cwd %s\n", how, b.space, b.tabLabel, pane, b.kind, b.dir)
		if err := os.Chdir(b.dir); err != nil {
			return 1
		}
		path := b.a.lookPath("claude")
		if path == "" {
			b.errf("brain: exec: claude: not found\n")
			return 127
		}
		err := b.execve(path, append([]string{"claude"}, agentArgs[1:]...), b.env) // # MUTATION-ANCHOR: go-892-M04
		b.errf("brain: exec: claude: %v\n", err)
		return 126
	}
	var out string
	for tries := 0; tries < 15; tries++ {
		cmd := append([]string{"agent", "start", proj, "--kind", b.kind, "--pane", pane, "--timeout", "60000"}, agentArgs...)
		r := b.herdr(brainCmd{Merge: true}, cmd...)
		out = brTrimNL(r.Out)
		if r.RC == 0 && interactiveReady(out) {
			break
		}
		if !strings.Contains(out, "agent_pane_busy") {
			break
		}
		b.sleep(2 * time.Second)
	}
	startFailed := false
	// LOUD, not a note.
	if !interactiveReady(out) {
		// A FIRST-RUN SCREEN IS NOT A BARE SHELL.
		screen := ""
		if b.kind == "claude" {
			screen = brTrimNL(b.herdr(brainCmd{}, "agent", "read", pane, "--source", "detection", "--lines", "80").Out)
		}
		dialog := claudeStartupDialog(screen) // # MUTATION-ANCHOR: go-188-M03
		if dialog != "" {
			b.errf("brain: claude IS running in %s, stopped on a first-run screen (%s): %s.\n", pane, dialog, claudeDialogAdvice(dialog, acctDir, b.home))
			b.errf("brain: answer it in that pane — it is focused — and the brainer continues from there.\n")
		} else {
			b.errf("brain: %s did NOT start in %s — the tab is a bare shell, not a brainer.\n", b.kind, pane)
			last := out
			if last == "" {
				last = "<none>"
			}
			b.errf("brain: last reply from herdr: %s\n", headBytes(last, 200))
			if screen != "" {
				b.errf("brain: what the pane was showing (last lines):\n")
				for _, l := range lastNonBlank(screen, 8) {
					b.errf("    %s\n", l)
				}
			}
			b.errf("brain: start it by hand in that pane, or re-run once the pane is at its prompt.\n")
			startFailed = true
		}
	}
	if tab != "" && tab != "null" {
		if !b.herdrOK("tab", "focus", tab) {
			if r := b.herdr(brainCmd{NoOut: true, ErrTo: true}, "workspace", "focus", ws); r.RC != 0 {
				return 1
			}
		}
	} else if r := b.herdr(brainCmd{NoOut: true, ErrTo: true}, "workspace", "focus", ws); r.RC != 0 {
		return 1
	}
	// A RESUME THAT DID NOT START IS NOT "resumed".
	tabText := tab
	if tabText == "" {
		tabText = "?"
	}
	if startFailed && b.resume != "" {
		if !b.noConversation {
			b.errf("FAILED resuming %s in %s / %s (%s tab %s, pane %s, %s) — the pane is a bare shell; the conversation is untouched, and `brain %s --resume %s` retries\n", b.resume, b.space, b.tabLabel, ws, tabText, pane, b.kind, proj, b.resume)
			return 75
		}
		b.errf("FAILED starting a new conversation in %s / %s (%s tab %s, pane %s, %s) — the pane is a bare shell; `brain %s` starts a brainer in it\n", b.space, b.tabLabel, ws, tabText, pane, b.kind, proj)
		return 75
	}
	switch {
	case revive:
		b.outf("restarted %s / %s (%s tab %s, pane %s, %s) — same pane, new conversation, the old one had already exited — cwd %s\n", b.space, b.tabLabel, ws, tabText, pane, b.kind, b.dir)
	case b.noConversation && b.reset:
		b.outf("restarted %s / %s (%s tab %s, pane %s, %s) — same pane, NEW conversation: %s has no transcript (it never had a turn), so there was nothing to resume — cwd %s\n", b.space, b.tabLabel, ws, tabText, pane, b.kind, b.resume, b.dir)
	case b.resume != "" && b.reset:
		b.outf("resumed %s / %s (%s tab %s, pane %s, %s) — same pane, conversation %s — cwd %s\n", b.space, b.tabLabel, ws, tabText, pane, b.kind, b.resume, b.dir)
	case b.reset:
		b.outf("reset %s / %s (%s tab %s, pane %s, %s) — same pane, new conversation — cwd %s\n", b.space, b.tabLabel, ws, tabText, pane, b.kind, b.dir)
		if resetCwdUnfixed {
			b.errf("brain: but it is STILL in the wrong lane — see the warning above. `hw` will refuse to dispatch from it.\n")
			return 3
		}
	default:
		b.outf("opened %s / %s (%s tab %s, pane %s, %s) — cwd %s\n", b.space, b.tabLabel, ws, tabText, pane, b.kind, b.dir)
	}
	b.cockpitWriter(b.kind, pane)
	b.flushOutbox(proj, pane) // # MUTATION-ANCHOR: go-243-M02
	return 0
}

func interactiveReady(text string) bool {
	v, ok := bjParse(text)
	if !ok {
		return false
	}
	r, _ := bjGet(v, "result", "agent", "interactive_ready").(bool)
	return r
}

// rawOrEmpty is `jq -r <expr> 2>/dev/null || true` of a reply that may not be JSON at all.
func rawOrEmpty(doc any, v any) string {
	if doc == nil {
		return ""
	}
	return bjRaw(v)
}

func afterTab(s string) string {
	if i := strings.Index(s, "\t"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func brainCutChars(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func headBytes(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func brainFirstLines(s string, n int) string {
	l := strings.SplitN(s, "\n", n+1)
	if len(l) > n {
		l = l[:n]
	}
	return strings.Join(l, "\n")
}

// lastNonBlank is `sed '/^[[:space:]]*$/d' | tail -n`.
func lastNonBlank(s string, n int) []string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return keep
}

func cleanJoin(dir, name string) string {
	// `cd -P "$BRAIN_BIN/.." && pwd` is the physical path of the directory above bin/.
	d := pyRealpath(dir)
	return strings.TrimRight(d, "/") + "/" + name
}

// jqCompact is `jq -nc` of the opencode config: the engram server through the label proxy.
func jqCompact(_ map[string]any, proxy, label, engramBin string) string {
	type server struct {
		Type    string   `json:"type"`
		Command []string `json:"command"`
	}
	doc := struct {
		MCP struct {
			Engram server `json:"engram"`
		} `json:"mcp"`
	}{}
	doc.MCP.Engram = server{"local", []string{proxy, "--label", label, "--", engramBin, "mcp", "--tools=agent"}}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(doc)
	return strings.TrimRight(buf.String(), "\n")
}

// writeEngramMCP writes the claude brainer's --mcp-config the way `jq -n … > file.$$ && mv` does: pretty, with a
// closing newline, atomically.
func (b *brainRun) writeEngramMCP(file, proxy, engramBin string) error {
	type server struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	doc := struct {
		McpServers struct {
			Engram server `json:"engram"`
		} `json:"mcpServers"`
	}{}
	doc.McpServers.Engram = server{proxy, []string{"--label", b.engramLabel, "--", engramBin, "mcp", "--tools=agent"}} // # MUTATION-ANCHOR: go-422-M01
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := os.MkdirAll(file[:strings.LastIndex(file, "/")], 0o777); err != nil {
		return err
	}
	tmp := file + "." + itoa(os.Getpid())
	if err := os.WriteFile(tmp, buf.Bytes(), 0o666); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// selfPath is the program that runs brain: what the in-pane start spawns for its detached flush.
func (b *brainRun) selfPath() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "hw-core"
}

// waitThenFlush is the detached half of an in-pane start: wait (up to 60s) for claude to be up in the pane, then
// hand the pane its undelivered reports.
func (b *brainRun) waitThenFlush(proj, pane string) {
	for i := 0; i < 60; i++ {
		v, ok := bjParse(b.agentGet(pane))
		if ok {
			if r, _ := bjGet(v, "result", "agent", "interactive_ready").(bool); r || bjGet(v, "result", "agent", "agent_status") != nil {
				break
			}
		}
		b.sleep(1 * time.Second)
	}
	b.flushOutbox(proj, pane)
}
