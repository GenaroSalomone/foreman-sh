package main

// The manifest of the dispatch (bin/hw `_dispatch_manifest`) and the helpers only it reads: `_deps_line`,
// `_deps_contending_others`, `_surface_dir`, `_intended_workdir`, `_task_base`. The text is bash's, byte for byte;
// it is the contract the suite and the brainers read (CLAUDE.md: "the manifest text never changes").

import (
	"path/filepath"
	"strings"
)

func trimNL(s string) string { return strings.TrimRight(s, "\n") }

// laneBase is `_lane_base`.
func (d *dsp) laneBase() string { return d.c.t.get(d.proj, "base") }

// laneBaseRefPrefix is `_lane_base_ref_prefix`: lane_get succeeds for a known lane, whose loader defaults the field.
func (d *dsp) laneBaseRefPrefix() string {
	if !d.c.t.known(d.proj) {
		return "origin/" // `v="$(lane_get …)" || v='origin/'`
	}
	return d.c.t.get(d.proj, "base_ref_prefix")
}

// taskBase is `_task_base`: an explicit --base wins, the lane's otherwise.
func (d *dsp) taskBase() string {
	if d.baseOverride != "" { // # MUTATION-ANCHOR: 844-M36
		return d.baseOverride
	}
	return d.laneBase()
}

func (d *dsp) laneHasRepo() bool { return d.c.checkout(d.proj) != "" }

// intendedWorkdir is `_intended_workdir`.
func (d *dsp) intendedWorkdir() string {
	if !d.wantWT {
		return d.c.work + "/" + d.proj + "/" + d.task
	}
	if wt := d.c.lanePath(d.proj, "worktree", d.task); wt != "" {
		return wt
	}
	return d.c.work + "/" + d.proj + "/" + d.task
}

// surfaceDir is `_surface_dir`.
func (d *dsp) surfaceDir() string {
	if d.c.t.get(d.proj, "product_repo") == "1" {
		return d.c.checkout(d.proj)
	}
	return d.intendedWorkdir()
}

// depsContendingOthers is `_deps_contending_others`: the live, unreported executors of the lane, one line each;
// ok=false is bash's `return 1` (herdr or jq failed).
func (d *dsp) depsContendingOthers(proj string) (string, bool) {
	h := herdrClient{getenv: d.getenv, env: d.environ()}
	agents, okList := h.stdout("agent", "list")
	if !okList {
		return "", false
	}
	if agents == "" || trimNL(agents) == "" {
		return "", true // `[ -n "$agents" ] || return 0` ($(…) strips the newlines)
	}
	vals, err := parseStream([]byte(agents))
	if err != nil && len(vals) == 0 {
		return "", false
	}
	type cand struct{ pane, task string }
	var cands []cand
	for _, v := range vals {
		list, okIter := depsIter(v, "result", "agents")
		if !okIter {
			return "", false
		}
		for _, a := range list {
			tk, _ := a.get("tokens")
			hp := ""
			if tk != nil {
				if x, ok := tk.get("hw_project"); ok && x.truthy() {
					hp = x.raw()
				}
			}
			if hp != proj {
				continue
			}
			var elems []string
			if x, ok := a.get("pane_id"); ok && x.truthy() {
				elems = append(elems, tsvEscape(x.raw()))
			}
			if tk != nil {
				if x, ok := tk.get("hw_task"); ok && x.truthy() {
					elems = append(elems, tsvEscape(x.raw()))
				}
			}
			line := strings.Join(elems, "\t")
			parts := strings.SplitN(line, "\t", 2)
			c := cand{pane: parts[0]}
			if len(parts) == 2 {
				c.task = parts[1]
			}
			cands = append(cands, c)
		}
	}
	var out []string
	for _, c := range cands {
		if c.pane == "" {
			continue
		}
		info, _ := h.stdout("agent", "get", c.pane)
		if info == "" {
			continue
		}
		cwd := jqRawOr(info, "", "result", "agent", "cwd")
		if cwd == "" {
			continue
		}
		rundir := d.nextRunDir(cwd)
		if rundir == "" {
			continue
		}
		if isFile(d.runDoneMarker(rundir)) {
			continue
		}
		task := c.task
		if task == "" {
			task, _ = d.runEnvValue(rundir, "HW_TASK")
		}
		if task == "" {
			task = "unknown"
		}
		out = append(out, "pane "+c.pane+" (task "+task+")")
	}
	return strings.Join(out, "\n"), true
}

// depsIter is `(.a.b // [])[]`: the elements jq would iterate, false for what it cannot iterate.
func depsIter(v *jval, path ...string) ([]*jval, bool) {
	cur := v
	for _, p := range path {
		x, ok := cur.get(p)
		if !ok || x == nil {
			return nil, true
		}
		cur = x
	}
	if !cur.truthy() {
		return nil, true
	}
	switch cur.kind {
	case vArr:
		return cur.arr, true
	case vObj:
		var vs []*jval
		for _, m := range cur.obj {
			vs = append(vs, m.v)
		}
		return vs, true
	}
	return nil, false
}

// tsvEscape is jq's @tsv for a string.
func tsvEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r")
	return r.Replace(s)
}

// depsLineFor is `_deps_line`: DEPS_LINE, from what the lane declares and who else is live on it.
func (d *dsp) depsLineFor(proj string) {
	base := d.c.t.get(proj, "deps_line")
	if base == "" {
		if d.c.t.get(proj, "repoless") == "1" {
			d.depsLine = "none (" + proj + " is repo-less)"
		} else {
			d.depsLine = "not declared for " + proj + " — bin/hw names no dependency sharing for this lane"
		}
		return
	}
	if d.c.t.get(proj, "deps_contention") != "1" {
		d.depsLine = base
		return
	}
	others, ok := d.depsContendingOthers(proj)
	others = trimNL(others)
	switch {
	case !ok:
		d.depsLine = base + " Other live executors on this project could NOT be established (herdr/jq lookup failed) — check by hand before installing anything."
	case others != "":
		j := strings.ReplaceAll(strings.ReplaceAll(others, "\n", ";"), ";", ", ")
		j = strings.TrimSuffix(j, ", ")
		d.depsLine = base + " Currently working (not yet reported), so an install there breaks this task's suite too: " + j
	default:
		d.depsLine = base + " No other live executor found on this project right now."
	}
}

// cutChars is `cut -c1-N` per line: the system's own cut, so a UTF-8 locale counts characters and C counts bytes.
func (d *dsp) cutChars(s string, n int) string { return brCutChars(s, n, d.vdUTF8()) }

// dispatchManifest is `_dispatch_manifest`: builds the text, prints it, then runs every gate a launch would run
// before anything is built, and on a dry run says so and ends.
func (d *dsp) dispatchManifest(dir string) {
	live, sddSrc := "", "inherited"
	surface := d.surfaceDir()
	if d.dispatchRoute == "reuse" && d.c.t.get(d.proj, "product_repo") != "1" {
		surface = dir
	}
	if d.sdd != "" {
		live = d.sdd
		switch d.sdd {
		case "none":
			sddSrc = "chosen (--sdd none) — the directory keeps its own mode, hw forces no entry"
		default:
			sddSrc = "chosen (--sdd " + d.sdd + "), checked on disk below"
		}
	} else {
		live = d.sddLiveMode(surface)
		sddSrc = "inherited"
	}

	// what will be run at close
	d.verifyFullSuite = false
	verifyCmds := d.briefVerifyCmds(d.brief)
	var verifyLine string
	if verifyCmds != "" {
		verifyLine = strings.SplitN(verifyCmds, "\n", 2)[0]
		if n := len(strings.Split(verifyCmds, "\n")); n > 1 {
			verifyLine += "  (+" + itoa(n-1) + " more)" // # MUTATION-ANCHOR: 844-M20
		}
		verifyLine += "  (hw runs this at close; the exit code goes to the receipt)" // # MUTATION-ANCHOR: 844-M21
		for _, vc := range strings.Split(verifyCmds, "\n") {
			if vc == "" {
				continue
			}
			if d.verifyNamesFullSuite(vc) { // # MUTATION-ANCHOR: 844-141-M01
				d.verifyFullSuite = true
			}
		}
		if d.verifyFullSuite { // # MUTATION-ANCHOR: 844-M22
			verifyLine += "\n              WILL TIME OUT AT CLOSE: this names the full suite (~1000s) without HW_TEST_GATE=fast, against hw done's " + brVerifyTimeout() + "s timeout. Edit the brief now: HW_TEST_GATE=fast bash setup/test-hw"
		}
	} else if d.brief == "" {
		verifyLine = "none — no brief, so nothing to run and nothing measured at close"
	} else {
		verifyLine = "none — the brief has no runnable command in its Testing/Verification section"
	}

	// is the framework in play for this task?
	inPlay := !(live == "" || strings.HasSuffix(live, "(global)")) // # MUTATION-ANCHOR: 844-M23
	if d.sdd == "none" {
		inPlay = false
	} else if d.sdd != "" {
		inPlay = true
	}
	d.dispatchFramework = strings.SplitN(live, " ", 2)[0]
	d.dispatchInPlay = inPlay

	mark := "inherited"
	if d.model != "" {
		mark = d.modelSrc
		if mark == "" {
			mark = "chosen"
		}
	}
	modelShown := d.model
	if modelShown == "" {
		modelShown = "<agent default>"
	}
	modelLine := "model=" + modelShown + "  (" + mark + ")" + d.modelSuggestionNote()
	if d.modelPinned != "" {
		modelLine += "  — the lane pins the alias " + d.model + " to " + d.modelPinned + " (projects.json model_pins), so the CLI gets --model " + d.modelPinned
	}
	mark = "inherited"
	if d.effort != "" {
		mark = "chosen"
	}
	effortShown := d.effort
	if effortShown == "" {
		effortShown = "<agent default>"
	}
	effortLine := "effort=" + effortShown + "  (" + mark + ")" // # MUTATION-ANCHOR: 844-M26
	if d.agent == "opencode" && d.effort != "" {
		pa := d.opencodePrimaryAgent
		if pa == "" {
			pa = "?"
		}
		effortLine += "\n  carried      via OPENCODE_CONFIG_CONTENT in the pane env (tab, --here, .hw/<run>/env) → agent \"" + pa + "\".reasoningEffort\n  verified     UNVERIFIED at dispatch — read back from the live executor's GET /config after start (receipt: effort_applied)\n  overlay      " + d.opencodeEffortOverlay
	}

	var placement, dbLine, entryLine, chainingLine, devserverLine string
	if inPlay {
		entryLine = d.frameworkEntry(strings.SplitN(live, " ", 2)[0])
		if entryLine == "" {
			entryLine = "<none: '" + live + "' has no single entry>"
		}
	} else {
		entryLine = "none (no framework flow for this task)"
	}
	switch {
	case d.here:
		placement = "here (splits the brainer pane)"
	case d.tab:
		placement = "own tab in space " + d.c.t.get(d.proj, "space") // # MUTATION-ANCHOR: 844-M28
	default:
		placement = "own workspace (legacy model)"
	}
	placement += "  (" + d.hereSrc + ")"
	dbText := d.c.t.get(d.proj, "db_line")
	dbNoWT := d.c.t.get(d.proj, "db_no_worktree")
	switch {
	case d.c.t.get(d.proj, "db_provisioned") == "1":
		if !d.wantWT {
			dbLine = dbNoWT + "  (" + d.wtSrc + ")"
		} else if d.wantDB {
			dbLine = "own, provisioned per worktree  (" + d.dbSrc + ")" // # MUTATION-ANCHOR: 844-M29
		} else {
			dbLine = "NO — shares the main checkout's database  (" + d.dbSrc + ")" // # MUTATION-ANCHOR: 844-M30
		}
	case dbText != "":
		if !d.wantWT && dbNoWT != "" {
			dbLine = dbNoWT + "  (" + d.wtSrc + ")"
		} else {
			dbLine = dbText
			if !d.wantDB && d.c.t.get(d.proj, "db_no_db_inert") == "1" {
				dbLine += " (--no-db has no effect)"
			}
		}
	case d.c.t.get(d.proj, "repoless") == "1":
		dbLine = "none (" + d.proj + " is repo-less)"
	default:
		dbLine = "none — bin/hw provisions no database for " + d.proj
	}
	switch dev := d.c.t.get(d.proj, "devserver"); dev {
	case "always", "opt-in":
		switch {
		case !d.wantWT:
			devserverLine = "none — no worktree to run one in  (" + d.wtSrc + ")"
		case d.here:
			devserverLine = "none — --here exits before a dev server would start  (" + d.hereSrc + ")" // # MUTATION-ANCHOR: 844-171-M05
		case dev == "always":
			devserverLine = "always started (this lane has no opt-out)"
		case d.wantDevserver: // # MUTATION-ANCHOR: 844-M32
			devserverLine = d.c.t.get(d.proj, "devserver_line") + "  (" + d.devserverSrc + ")"
		default:
			devserverLine = d.c.t.get(d.proj, "devserver_off") + "  (" + d.devserverSrc + ")" // # MUTATION-ANCHOR: 844-M31
		}
	default:
		devserverLine = "n/a (" + d.proj + " has no dev server)"
	}
	d.depsLineFor(d.proj)
	d.resolveAccount()
	d.resolveSubagentModel()
	agentLine := d.agent + "  (" + d.agentSrc + ")"
	if d.agent == "opencode" {
		if d.opencodePrimaryAgent != "" {
			agentLine += "\n  primary     " + d.opencodePrimaryAgent + "  (explicit --agent passed by hw)"
		} else {
			agentLine += "\n  primary     <none passed — opencode default_agent applies>"
		}
	}
	if d.keepPane {
		kf := d.keepPaneFor
		if kf == "" {
			kf = "4h"
		}
		chainingLine = "enabled — leased for " + d.keepPaneSeconds + "s  (chosen by --keep-pane " + kf + "; expiry closes the pane)\n  chaining why  " + d.keepPaneWhy // # MUTATION-ANCHOR: 844-37-M02
	} else {
		chainingLine = "disabled  (default; a done report closes the owned pane, a --blocked one keeps it waiting)"
		if !d.here {
			chainingLine += " — ONE-WAY FROM HERE: HW_CHAINING_ENABLED is baked into the executor's environment at spawn, so `hw next` cannot re-task this pane afterwards" // # MUTATION-ANCHOR: 844-67-M04
		}
	}

	// the worktree
	var wtLine string
	if d.laneHasRepo() {
		switch {
		case !d.wantWT:
			wtLine = "none — a plain work directory, no branch, no ports, no database  (" + d.wtSrc + ")"
		case isDir(dir) && !isDir(dir+"/.git") && !isFile(dir+"/.git"): // # MUTATION-ANCHOR: 844-M34
			wtLine = dir + "  (REUSED as a PLAIN DIRECTORY — it exists and is not a git worktree, so this task gets no branch and no isolation; that is what a pre-worktree " + d.proj + " task left behind)"
		case isDir(dir+"/.git") || isFile(dir+"/.git"): // # MUTATION-ANCHOR: 844-M35
			head := "detached"
			if out, err := childCmd("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output(); err == nil {
				head = trimNL(string(out))
			}
			wtLine = dir + "  (REUSED — it already exists, with whatever branch and uncommitted state it holds; HEAD is " + head + ", and --base cannot rebase it)"
		default:
			wtLine = dir + "  (new, own branch off " + d.laneBaseRefPrefix() + d.taskBase() + ")"
		}
	} else {
		wtLine = "none (" + d.proj + " is repo-less: no worktree, no branch, no ports)"
	}

	var baseLine string
	switch {
	case d.laneBase() == "" && d.baseOverride == "":
		baseLine = "none (" + d.proj + " is repo-less)"
	case !d.wantWT:
		baseLine = d.laneBaseRefPrefix() + d.taskBase() + "  (" + d.baseSrc + ") — no worktree is built, so nothing is checked out from it" // # MUTATION-ANCHOR: 844-M37
	default:
		baseLine = d.laneBaseRefPrefix() + d.taskBase() + "  (" + d.baseSrc + ")"
	}

	permissionsLine := ""
	switch d.agent + ":" + d.permissions {
	case "claude:skip":
		permissionsLine = "skip  (" + d.permissionsSrc + ") — --dangerously-skip-permissions" // # MUTATION-ANCHOR: 844-M38
	case "opencode:skip":
		permissionsLine = "skip  (" + d.permissionsSrc + ") — --auto (identical to `opencode-auto`)"
	case "codex:skip":
		permissionsLine = "skip  (" + d.permissionsSrc + ") — codex's own approval_policy applies; hw sets none"
	case "claude:ask":
		permissionsLine = "ask  (" + d.permissionsSrc + ") — no skip flag; the executor WAITS on every permission dialog, and nothing answers it in an unattended pane"
	case "opencode:ask":
		permissionsLine = "ask  (" + d.permissionsSrc + ") — no --auto; a prompt is reported blocked reason=permission, and a person has to answer it"
	case "codex:ask":
		permissionsLine = "ask  (" + d.permissionsSrc + ") — -a on-request; the executor WAITS on an approval nobody answers in an unattended pane"
	}
	var providerLine string
	if d.agent == "opencode" {
		if d.model != "" {
			prov := d.model
			if i := strings.Index(prov, "/"); i >= 0 {
				prov = prov[:i]
			}
			if d.modelSrc == "" || d.modelSrc == "chosen" {
				providerLine = prov + "  (direct, from --model)"
			} else {
				providerLine = prov + "  (direct, " + d.modelSrc + ")"
			}
		} else {
			providerLine = "<opencode default — no --model given>"
		}
		endpoint := "<not allocated>"
		if d.opencodeAgentPort != "" {
			endpoint = "http://127.0.0.1:" + d.opencodeAgentPort
		}
		providerLine += "\n  background  OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true (as `opencode-auto` sets it)\n  endpoint    " + endpoint + "  (this executor's own; $HW_AGENT_ENDPOINT in its env)"
		if d.opencodeExecutorConfig != "" {
			providerLine += "\n  guard       " + d.opencodeExecutorConfig + "/plugin/deny-repo-writes.js  (OPENCODE_CONFIG_DIR, lane table: refuses writes into " + d.proj + "'s checkout)"
		}
	} else {
		providerLine = "n/a (" + d.agent + " resolves its own)"
	}

	d.invokerPaneInit()
	var invokerLine string
	switch {
	case !d.wantReport:
		invokerLine = "DISABLED by --no-report — deliberate fire-and-forget; status will classify an unreported finished task"
	case d.invokerPane() != "":
		if d.reportToChosen { // # MUTATION-ANCHOR: 844-M41
			invokerLine = d.invokerPane() + "  (chosen by --report-to; foreign routing deliberately permitted)"
		} else {
			invokerLine = d.invokerPane() + "  (resolved by " + d.invokerHowWord() + "; verified as label=brain cwd=" + d.c.brain + "/" + d.proj + ")"
		}
	default:
		invokerLine = "UNRESOLVED — ask-invoker and done-invoker will refuse, so this executor could not report back"
	}

	var routeLine string
	runLabel := d.hwRun
	if d.dispatchRoute == "reuse" {
		runLabel = basename(d.reuseRundir) + " (the reused session's run, not a new one)"
		routeLine = "REUSE — the brief goes to the LIVE executor on pane " + d.reusePane + " (prior task: " + orDefaultStr(d.reuseTask, "unknown") + "; " + d.reuseWhy + ")\n              NOTHING IS BUILT: no worktree, no branch, no database, no ports, no pane. Its prior context and worktree carry into this task.\n              Launch fresh instead with --fresh."
		placement = "n/a — reuse adds no pane; the work lands in " + d.reusePane + " where it already is"
		dbLine = "n/a — reuse provisions nothing; this task uses whatever database that session already has"
		devserverLine = "n/a — reuse starts nothing; this task uses whatever dev server that session already has (or lacks)"
		d.depsLine = "n/a — reuse builds nothing; this task runs in " + d.reusePane + "'s existing worktree and inherits whatever dependency contention already applies to it"
		wtLine = "n/a — reuse creates none; this task inherits the reused session's worktree and its uncommitted state"
		baseLine = "n/a — the reused session's worktree was built on its own base at ITS launch; --base cannot change it from here"
		chainingLine = "governed by the REUSED pane's own lease, baked at ITS launch — --keep-pane cannot change it from here"
		d.reuseMeasureAccount()
		d.accountLine = d.reuseAccountFieldLine()
		d.subagentModelLine = d.reuseSubagentModelFieldLine()
		d.explorerLine = d.reuseExplorerFieldLine()
		d.artifactsLine = "the REUSED session's own, fixed at ITS launch — hw next cannot add or remove a tool"
		permissionsLine = "the REUSED session's own, fixed at ITS launch — --permissions cannot change a live session"
		d.reuseModel = d.nextDispatchModel(d.reuseRundir)
		d.reuseEffort = d.nextDispatchEffort(d.reuseRundir)
		hv, _ := herdrClient{getenv: d.getenv, env: d.environ()}.stdout("agent", "get", d.reusePane)
		d.reuseVendor = jqRawOr(hv, "", "result", "agent", "agent")
		modelLine = d.reuseModelFieldLine()
		effortLine = d.reuseEffortFieldLine()
		agentLine = d.reuseAgentFieldLine()
	} else {
		routeLine = "fresh executor — a new one is built (" + d.reuseLine + ")"
		if d.dispatchDry && d.checkReuse { // # MUTATION-ANCHOR: 844-M50
			routeLine += "\n              FROZEN by this dry run: still a read of live state, but if a reusable pane appears before you launch, the launch REFUSES and names the change rather than taking the other route. --fresh binds it without a dry run."
		}
	}

	designLine := ""
	if d.designGateApplies() {
		designLine = "judged before code — boundary: " + trimNL(d.cutChars(d.briefBoundary(), 96)) + "  (done-invoker refuses done without an APPROVED design-judgment.md)"
	}

	guardLine := ""
	if d.brainGuardApplies() && !d.brainGuardPresent() {
		guardLine = "\n  brain guard MISSING under " + d.brainGuardDir() + " — a launch refuses"
	} else if d.brainGuardApplies() {
		why := "(a product executor does not write in brain, work from it, or commit a path into it)"
		var gl string
		switch d.agent {
		case "claude":
			gl = "--plugin-dir " + d.brainGuardClaudePlugin() + "  " + why
		case "opencode":
			gl = "OPENCODE_CONFIG=" + d.brainGuardOpencodeConfig() + "  " + why
		default:
			gl = "NOT APPLIED — " + d.agent + " loads none, so nothing stops this executor writing in brain"
		}
		guardLine = "\n  brain guard " + gl
	}

	briefShown := d.brief
	if briefShown == "" {
		briefShown = "<none>"
	}
	contract := d.briefDeclared
	if contract == "" {
		contract = "none"
	}
	liveShown := live
	if liveShown == "" {
		liveShown = "<undetermined>"
	}
	var sb strings.Builder
	sb.WriteString("dispatch " + d.proj + ":" + d.task + "  run=" + runLabel + "\n")
	sb.WriteString("  route       " + routeLine + "\n")
	sb.WriteString("  framework   " + liveShown + "  (" + sddSrc + ", from " + surface + ")\n")
	sb.WriteString("  " + modelLine)
	if d.modelFloorLine != "" {
		sb.WriteString("\n  floor       " + d.modelFloorLine)
	}
	if d.topTierLine != "" {
		sb.WriteString("\n  ceiling     " + d.topTierLine)
	}
	sb.WriteString("\n")
	sb.WriteString("  " + effortLine + "\n")
	sb.WriteString("  agent       " + agentLine + "\n")
	sb.WriteString("  account     " + d.accountLine + "\n")
	sb.WriteString("  subagents   " + d.subagentModelLine + "\n")
	if d.explorerLine != "" {
		sb.WriteString("  explorer    " + d.explorerLine + "\n")
	}
	if pl := d.pluginsLines() + d.reviewLines(); pl != "" {
		sb.WriteString(pl[1:] + "\n")
	}
	sb.WriteString("  permissions " + permissionsLine + "\n")
	sb.WriteString("  artifacts   " + d.artifactsLine + guardLine + "\n")
	sb.WriteString("  provider    " + providerLine + "\n")
	sb.WriteString("  placement   " + placement + "\n")
	sb.WriteString("  database    " + dbLine + "\n")
	sb.WriteString("  dev server  " + devserverLine + "\n")
	sb.WriteString("  dependencies " + d.depsLine + "\n")
	sb.WriteString("  worktree    " + wtLine + "\n")
	sb.WriteString("  base        " + baseLine + "\n")
	sb.WriteString("  brief       " + briefShown + "\n")
	sb.WriteString("  contract    " + contract + "  (which dispatch fields the brief declared, and hw verified)\n")
	sb.WriteString("  requested   " + trimNL(d.requestedLine()) + "\n")
	sb.WriteString("  authorizes  " + trimNL(d.authorizesLine()))
	if designLine != "" {
		sb.WriteString("\n  design      " + designLine)
	}
	sb.WriteString("\n")
	sb.WriteString("  verify      " + verifyLine + "\n")
	sb.WriteString("  entry       " + entryLine + "\n")
	sb.WriteString("  workdir     " + dir + "\n")
	sb.WriteString("  invoker     " + invokerLine)
	if d.sandboxLine != "" {
		sb.WriteString("\n  sandbox     " + d.sandboxLine)
	}
	sb.WriteString("\n")
	ep := d.engramProxyLine
	if ep == "" {
		ep = "nothing"
	}
	sb.WriteString("  engram      " + trimNL(d.engramLabel(d.proj)) + "  (the label of the lane, not of the brainer: --report-to routes reports, never memory; held by bin/engram-label-proxy via " + ep + ")\n")
	sb.WriteString("  chaining    " + chainingLine + "\n")
	sb.WriteString("  reuse check " + d.reuseCheckField())
	d.dispatchText = trimNL(sb.String()) // DISPATCH_TEXT="$(cat <<EOF … EOF)"

	if d.dispatchDry {
		d.step("0/3  dispatch (dry run)")
	} else {
		d.step("0/3  dispatch")
	}
	for _, l := range strings.Split(d.dispatchText, "\n") {
		d.printf("  %s%s%s\n", d.cDim, l, d.c0)
	}
	if d.verifyFullSuite {
		d.warn("this brief's verification names the full suite without HW_TEST_GATE=fast")
		d.warn("hw runs it itself at close, bounded at " + brVerifyTimeout() + "s; the full suite is ~1000s, so it will exit 124 and read as a FAILED verification, not a slow one")
		d.warn("full-suite coverage is already mandatory at push (setup/hooks/pre-push). Fix the brief now — it is free until this launches: HW_TEST_GATE=fast bash setup/test-hw")
	}
	if d.dispatchRoute == "reuse" && d.sandbox {
		d.die("--sandbox is a property of a LAUNCH, and this dispatch would re-task the executor that is already running for " + d.proj + ":" + d.task + " — whatever it was launched with. Add --fresh to launch a sandboxed one. NOTHING was built.")
	}
	if d.dispatchRoute == "reuse" {
		return
	}
	if d.agent == "opencode" && d.model == "" { // # MUTATION-ANCHOR: 844-M46
		d.warn("no --model given and " + d.proj + " pins no provider, so this opencode executor gets whatever opencode defaults to")
		d.warn("pin it if the work depends on which model runs: --model <provider>/<model>")
	}
	if d.spaceExplicit { // # MUTATION-ANCHOR: 844-M44
		d.warn("--space selects the LEGACY placement model")
		d.warn("this task consumes a whole workspace instead of a tab in the project's " + d.c.t.get(d.proj, "space") + " space")
		d.warn("prefer --tab unless this task specifically requires a separate workspace")
	}
	if d.sdd == "" && inPlay {
		d.warn("framework mode would be INHERITED from the directory, not chosen for this task")
		d.warn("the brief must name the flow and why — the executor cannot guess it")
		d.die("say which: --sdd speckit | none (none = outside the flow, for read-only or operational work; hw then forces no entry command)")
	}
	if d.sdd == "speckit" && !d.speckitOnDisk(surface) {
		d.die("--sdd speckit REFUSED BEFORE BUILDING ANYTHING: no speckit-* skill is on disk under " + surface + "/.claude/skills or " + d.c.home + "/.claude/skills. Nothing was built. Install Spec Kit where it is used, or dispatch with --sdd none.")
	}
	if d.accountVerdict == "refused" {
		d.warn(d.accountRefusal)
		d.die("account '" + d.account + "' was selected (" + d.accountSrc + ") and hw could not establish it. Nothing was built. Fix what the line above names, or dispatch under the default account with --account default.")
	}
	if d.account != "default" && !d.accountApplies && d.agent != "claude" {
		d.warn("account '" + d.account + "' does NOT apply to a " + d.agent + " executor — only Claude Code reads CLAUDE_CONFIG_DIR")
		d.warn("this executor runs under whatever account " + d.agent + " is signed into. If the account matters, launch it with --agent claude.")
	}
	if d.agent == "claude" && d.accountVerdict != "refused" {
		if pend := d.claudeFirstRunPending(d.accountConfigDir); len(pend) > 0 {
			d.warn("Claude Code's first run is not finished in this account (" + strings.Join(pend, " ") + ") — this executor will stop on it, and finishing it is yours:")
			d.warn("  run once in a terminal: " + d.claudeFirstRunCommand(d.accountConfigDir) + "  — finish the welcome and login, accept the Bypass Permissions warning, /exit")
		}
	}
	if inPlay && d.model == "" {
		switch strings.SplitN(live, " ", 2)[0] {
		case "speckit":
			if d.model != "opus" {
				d.warn("framework is speckit — this session is the ORCHESTRATOR, not the worker")
				d.warn("the phase agents pin their own models; --model only sets the judgment that")
				d.warn("sequences them. opus is suggested here: --model opus --effort medium")
			} else if d.effort == "" || d.effort == "low" {
				e := d.effort
				if e == "" {
					e = "<default>"
				}
				d.warn("opus with effort=" + e + " for an orchestrating run — medium is suggested")
			}
		}
	}
	if d.model == "" && d.agent == "claude" && d.reusePane == "" {
		m := d.sddEffectiveMode
		if m == "" {
			m = "?"
		}
		d.warn("no --model under --sdd " + m + ": this inherits claude's account default,")
		d.warn("not sonnet — an orchestrator may not be sonnet. --model opus --effort medium names it.")
	}
	if !d.keepPane && d.wantReport && !d.here { // # MUTATION-ANCHOR: 844-67-M01
		d.warn("no --keep-pane: this executor's pane CLOSES on its proved report, and that is IRREVERSIBLE")
		d.warn("HW_CHAINING_ENABLED is baked in at spawn — it cannot be turned on afterwards, so") // # MUTATION-ANCHOR: 844-67-M02
		d.warn("`hw next` will have no pane to re-task and new scope means a fresh session that")
		d.warn("has learned nothing about this repo. Decide it NOW, while it is still free:")
		d.warn("  --keep-pane [4h] --why \"<who or what will re-task it>\"") // # MUTATION-ANCHOR: 844-67-M03
	}
	if d.brief != "" {
		bb := strings.TrimSuffix(basename(d.brief), ".md")
		if bb != d.task { // # MUTATION-ANCHOR: 844-M47
			d.warn("the brief is named '" + bb + "' but the task is '" + d.task + "' — deliberate, or a copy-paste?")
			d.warn("everything else (worktree, branch, engram, hw done) will say '" + d.task + "'")
		}
	}
	if verifyCmds == "" && d.brief != "" { // # MUTATION-ANCHOR: 844-M48
		d.warn("nothing in this brief can be RUN to verify the work: hw will measure nothing at close")
		d.warn("put the command in a fenced block under Testing/Verification and hw runs it in hw done")
	}

	if d.dispatchDry {
		if !d.checkReuse { // # MUTATION-ANCHOR: 844-M49
			d.routeForecastDrop(d.proj, d.task)
			fs := d.freshSource
			if fs == "" {
				fs = "--fresh"
			}
			d.info("route BOUND as fresh by " + fs + " itself; nothing was recorded to bind a later launch, and any earlier forecast for " + d.proj + ":" + d.task + " was dropped")
		} else {
			if rf := d.routeForecastWrite(d.proj, d.task, "fresh", d.reuseLine); rf != "" {
				d.info("route FROZEN as fresh for the next " + d.routeForecastTTL + " s (" + rf + "). If a reusable pane appears before you launch, the launch REFUSES and names the change instead of switching routes.")
			}
		}
		d.ok("dry run — nothing created, no agent started (except the route forecast named above)")
		return
	}
	// Straight to the operator, not through the brainer (bin/hw "herdr notification show"): sent by launch() (t5l.go).
	// `${verify_cmds:+$(… | head -1)}${verify_cmds:-none}`: the first line, then the whole of it — or "none".
	verifyShown := "none"
	if verifyCmds != "" {
		verifyShown = strings.SplitN(verifyCmds, "\n", 2)[0] + verifyCmds
	}
	d.notifyBody = "framework " + orDefaultStr(live, "?") + " (" + sddSrc + ") · model " + orDefaultStr(d.model, "default") + " · verify " + verifyShown
}

func basename(p string) string { return filepath.Base(p) }
