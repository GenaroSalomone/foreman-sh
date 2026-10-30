package main

// Train T5: `hw <lane> <task> … --dry-run`, the dispatch's read half — everything bin/hw does between its arguments
// and the line "dry run — nothing created, no agent started". The launch (a worktree, a database, a pane, the
// brief delivered) stays bash: bin/hw hands hw-core a dispatch ONLY when --dry-run is among its arguments, and
// hw-core refuses one that is not (see cmdDispatch). The order of the calls below is the order of bin/hw's, because
// the order is the contract: a refusal that comes earlier is a refusal that costs less.
//
// The manifest (t5_manifest.go) is the text of `_dispatch_manifest` and never changes by a byte; the four areas
// of the port are t5_brief*.go, t5_vendor*.go and t5_reuse*.go / t5_sdd*.go (their bash functions are named in
// the first comment of each file).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) cmdDispatch() int {
	if len(a.args) < 2 {
		// bin/hw `[ $# -ge 2 ] || { _usage_short >&2; exit 64; }`: one word (or `-x`) is the usage, not a launch
		fmt.Fprint(a.err, helpText("short-1"))
		return 64
	}
	t5FillLaneFields(a.c.t, a.c.home)
	d := &dsp{app: a}
	d.readonly = a.getenv("HW_CORE_READONLY") == "1"
	d.run(a.args)
	return 0
}

// needVal is bash's `_need_val "$@"`: an option that takes a value and has none.
func (d *dsp) needVal(rest []string) {
	if len(rest) < 2 {
		d.die(rest[0] + " needs a value")
	}
}

func (d *dsp) run(argv []string) {
	defer func() {
		if d.fsShared != nil {
			d.fsShared.flush()
		}
	}()
	// bin/hw "_need_val "$@"" / PROJ=… / TASK=…
	d.proj = d.c.t.canonProject(argv[0])
	d.task = argv[1]
	rest := argv[2:]

	d.agent = orDefaultStr(d.c.t.get(d.proj, "vendor"), "claude")
	d.model = d.c.t.get(d.proj, "model")
	d.modelSrc = "default"
	d.agentExplicit = false
	if d.model != "" {
		d.modelSrc = "lane default for " + d.proj
	}
	d.account = orDefaultStr(d.c.t.get(d.proj, "account"), "default")
	d.accountSrc = "lane default for " + d.proj
	d.subagentModel = orDefaultStr(d.c.t.get(d.proj, "subagent_model"), "inherit") // # MUTATION-ANCHOR: 844-232-M08
	d.subagentModelSrc = "lane default for " + d.proj
	d.wantDB = true
	d.wantReport = true
	d.wantWT = true
	d.wtSrc = "default"
	d.dbSrc = "default"
	d.hereSrc = "default"
	d.devserverSrc = "default"
	d.agentSrc = "lane default for " + d.proj
	d.tab = true
	d.checkHandles = true
	d.checkReuse = true
	d.baseSrc = "lane default"
	d.dispatchRoute = "fresh"
	d.reuseLine = "no live reusable executor found in this lane (default)"
	d.routeForecastTTL = d.env("HW_ROUTE_FORECAST_TTL")
	if d.routeForecastTTL == "" {
		d.routeForecastTTL = "3600"
	}

	d.parseFlags(rest)

	if d.keepPane {
		if d.keepPaneWhy == "" { // # MUTATION-ANCHOR: 844-M01
			d.die("--keep-pane needs --why \"<who or what will re-task it>\". Retention without a named holder is an unbounded leak with nicer syntax.") // # MUTATION-ANCHOR: 844-37-M03
		}
		if !d.wantReport { // # MUTATION-ANCHOR: 844-M02
			d.die("--keep-pane and --no-report contradict each other: a chaining lease starts only after receiver-proved report delivery.")
		}
		if strings.Contains(d.keepPaneWhy, "\n") {
			d.die("--why must be one line so the run env and lease records remain parseable")
		}
		d.keepPaneSeconds = d.leaseSeconds(d.keepPaneFor) // # MUTATION-ANCHOR: 844-M03
	} else if d.keepPaneWhy != "" && !d.ignoreAfter {
		d.die("--why is only valid with --keep-pane or --ignore-after")
	}
	if d.ignoreAfter {
		if d.keepPaneWhy == "" {
			d.die("--ignore-after needs --why \"<why this dispatch must not wait for the brief's after: tasks>\". An override with no stated reason is the same as no check.")
		}
		if strings.Contains(d.keepPaneWhy, "\n") {
			d.die("--why must be one line so the manifest and the receipt stay parseable")
		}
		// --why is the --keep-pane lease reason too: with --ignore-after alone it is the override's, and must not reach the lease.
		d.ignoreAfterWhy = d.keepPaneWhy
		if !d.keepPane {
			d.keepPaneWhy = ""
		}
	}

	// An explicit --brief wins; otherwise brain/<project>/briefs/<task>.md when it exists.
	if d.brief == "" {
		d.brief = d.inferBrief()
		if d.brief != "" {
			d.info("brief inferred: " + d.brief)
		} else {
			d.archivedBriefGate(d.proj, d.task)
		}
	}
	d.brieflessGate()
	if d.brief != "" {
		if !isFile(d.brief) {
			d.die("brief not found: " + d.brief)
		}
		data, err := os.ReadFile(d.brief)
		if err != nil {
			d.die("brief not found: " + d.brief)
		}
		d.briefText = strings.TrimRight(string(data), "\n") // BRIEF_TEXT="$(cat "$BRIEF")"
		if d.briefText == "" {
			d.die("brief is empty: " + d.brief)
		}
	}
	if d.here && d.tabExplicit { // # MUTATION-ANCHOR: 844-M04
		d.die("--here and --tab are two different placements: --here splits your pane and drops the worktree, --tab gives the task its own tab and keeps it. Pick one.")
	}
	if d.here {
		d.tab = false
	}
	switch d.agent {
	case "claude", "opencode", "codex", "pi": // # MUTATION-ANCHOR: 844-M06
	default:
		d.die("--agent must be claude, opencode, codex or pi (got: " + d.agent + ")")
	}
	switch d.sdd {
	case "", "speckit", "none":
	case "gentle": // RETIRED SPELLING: gentle-ai left hw (setup/decisions.md, 2026-10-16). Warn, run none.
		d.warn("--sdd gentle is retired: gentle-ai is no longer part of hw. Running --sdd none.")
		d.sdd = "none"
	default:
		d.die("--sdd must be speckit or none (got: " + d.sdd + ")")
	}
	d.checkReusableExecutor(d.proj)
	d.routeNow = "fresh"
	if d.reusePane != "" {
		d.routeNow = "reuse"
	}
	// a dry run never checks or drops the forecast: that is the launch's (bin/hw "if [ "$DISPATCH_DRY" != 1 ]")
	if !d.dispatchDry {
		if !d.checkReuse {
			d.routeForecastDrop(d.proj, d.task)
		} else {
			d.routeForecastCheck(d.proj, d.task, d.routeNow, d.reuseLine)
		}
	}
	if d.reusePane == "" {
		d.validateSddVendorPair(d.agent, d.sdd, d.codexValidationSurface())
		d.validateSddModelPair(d.model)
	}
	d.setEnv("HW_EXECUTOR_VENDOR", d.agent)
	d.resolveInvokerContext()
	if d.agentExplicit && d.modelSrc != "chosen" && d.agent != orDefaultStr(d.c.t.get(d.proj, "vendor"), "claude") {
		d.model, d.modelSrc = "", "default" // # MUTATION-ANCHOR: 844-M09
	}
	d.applyModelDefault()
	d.checkModelFloor()
	d.checkBriefMeasuredEvidence()
	if !d.permissionsResolve(d.permissionsFlag) {
		d.die(d.permissionsErr)
	}
	d.buildAgentArgs()
	d.brainGuardCheck()
	d.brainGuardArgs()
	d.pluginsArgs()
	d.engramProxyArgs()
	d.writeEngramMcp()
	d.artifactsArgs()
	d.checkBriefContract()
	d.checkSandbox()
	d.checkRequested()
	d.checkAuthorizes()
	d.checkSecrets()
	d.checkBriefAssets()
	d.checkTask(d.task)
	d.checkAgentNameFree(d.task)
	if !d.c.t.known(d.proj) {
		d.die("unknown project: " + d.proj + " (expected: " + d.c.t.namesBar() + ")")
	}
	if d.here && !d.wtExplicit { // # MUTATION-ANCHOR: 844-M10
		d.wantWT, d.wtSrc = false, "implied by --here"
	}
	if d.c.t.get(d.proj, "repoless") == "1" {
		d.wantWT, d.wtSrc = false, "n/a (repo-less lane)"
	}

	d.dispatchRoute = "fresh"
	if d.reusePane != "" {
		// THE MANIFEST FIRST, ON THIS PATH TOO (bin/hw "BEFORE anything is built"): printed, then the refusals
		d.dispatchRoute = "reuse"
		d.dispatchManifest(filepath.Dir(filepath.Dir(d.reuseRundir)))
		d.reuseRoute()
		return
	}
	d.credentialPreflight()
	d.hwRun = time.Now().Format("20060102-150405") + "-" + fmt.Sprint(os.Getpid())
	d.dispatchManifest(d.intendedWorkdir())
	if !d.dispatchDry {
		d.launch()
	}
}

func orDefaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// parseFlags is the `while [ $# -gt 0 ]` loop of bin/hw, option for option.
func (d *dsp) parseFlags(rest []string) {
	for len(rest) > 0 {
		switch rest[0] {
		case "--agent":
			d.needVal(rest)
			d.agent, d.agentSrc, d.agentExplicit = rest[1], "chosen", true
			rest = rest[2:]
		case "--account":
			d.needVal(rest)
			d.account, d.accountSrc = rest[1], "chosen"
			rest = rest[2:]
		case "--subagent-model":
			d.needVal(rest)
			if !validSubagentModel(rest[1]) {
				d.die("--subagent-model '" + rest[1] + "' is not a model: inherit, an alias (haiku, sonnet, opus, fable) or a claude-* id. Nothing was built.")
			}
			d.subagentModel, d.subagentModelSrc = rest[1], "chosen" // # MUTATION-ANCHOR: 844-232-M07
			rest = rest[2:]
		case "--permissions":
			d.needVal(rest)
			d.permissionsFlag = rest[1]
			rest = rest[2:]
		case "--brief":
			d.needVal(rest)
			d.brief = rest[1]
			rest = rest[2:]
		case "--no-brief":
			d.noBrief = true
			rest = rest[1:]
		case "--model":
			d.needVal(rest)
			d.model, d.modelSrc = rest[1], "chosen"
			rest = rest[2:]
		case "--model-why":
			d.needVal(rest)
			d.modelWhy, d.modelWhySrc = rest[1], "flag"
			rest = rest[2:]
		case "--requested-by":
			d.needVal(rest)
			d.requestedBy, d.requestedBySrc = rest[1], "flag"
			rest = rest[2:]
		case "--below-floor-why":
			d.needVal(rest)
			d.belowFloorWhy = rest[1]
			rest = rest[2:]
		case "--ignore-after":
			d.ignoreAfter = true
			rest = rest[1:]
		case "--top-tier-why":
			d.needVal(rest)
			d.topTierWhy = rest[1]
			rest = rest[2:]
		case "--effort":
			d.needVal(rest)
			d.effort = rest[1]
			rest = rest[2:]
		case "--sdd":
			d.needVal(rest)
			d.sdd = rest[1]
			rest = rest[2:]
		case "--dry-run":
			d.dispatchDry = true
			rest = rest[1:]
		case "--sandbox":
			d.sandbox, d.sandboxSrc = true, "--sandbox"
			rest = rest[1:]
		case "--no-db":
			d.wantDB, d.dbSrc = false, "chosen"
			rest = rest[1:]
		case "--dev-server":
			d.wantDevserver, d.devserverSrc = true, "chosen"
			rest = rest[1:]
		case "--no-report":
			d.wantReport = false
			rest = rest[1:]
		case "--report-to":
			d.needVal(rest)
			d.setEnv("HW_INVOKER_PANE", rest[1])
			d.reportToChosen = true
			rest = rest[2:]
		case "--keep-pane":
			d.keepPane = true
			rest = rest[1:]
			if len(rest) > 0 && rest[0] != "" && !strings.HasPrefix(rest[0], "--") {
				d.keepPaneFor = rest[0]
				rest = rest[1:]
			}
		case "--why":
			d.needVal(rest)
			d.keepPaneWhy = rest[1]
			rest = rest[2:]
		case "--here":
			d.here, d.hereSrc = true, "chosen"
			rest = rest[1:]
		case "--placement":
			d.needVal(rest)
			switch rest[1] {
			case "project-tab":
				d.tab, d.tabExplicit, d.here = true, true, false
			case "legacy-space":
				d.tab, d.spaceExplicit, d.here = false, true, false
			case "here":
				d.here = true
			default:
				d.die("--placement must be project-tab, legacy-space or here (got: " + rest[1] + ")")
			}
			d.hereSrc = "chosen"
			rest = rest[2:]
		case "--base":
			d.needVal(rest)
			if rest[1] == "" || strings.HasPrefix(rest[1], "-") {
				d.die("--base takes a branch name (got: " + rest[1] + ")")
			}
			d.baseOverride, d.baseSrc = rest[1], "chosen"
			rest = rest[2:]
		case "--worktree":
			d.needVal(rest)
			switch {
			case rest[1] == "new":
				d.wantWT = true
			case rest[1] == "none":
				d.wantWT = false
			case strings.HasPrefix(rest[1], "reuse:"):
				d.die("--worktree reuse:<task> is not implemented. It is the one axis value that needs an ownership check first — reusing another task's worktree binds this task to that branch and database, which is how a remediation task once ended up on the wrong lineage. Launch on its own worktree, or say what you actually need.")
			default:
				d.die("--worktree must be new or none (got: " + rest[1] + ")")
			}
			d.wtSrc, d.wtExplicit = "chosen", true
			rest = rest[2:]
		case "--tab":
			d.tab, d.tabExplicit, d.hereSrc = true, true, "chosen"
			rest = rest[1:]
		case "--space":
			d.tab, d.spaceExplicit, d.hereSrc = false, true, "chosen"
			rest = rest[1:]
		case "--no-handle-check":
			d.checkHandles = false
			rest = rest[1:]
		case "--allow-stale-hooks":
			d.allowStaleHooks = true
			rest = rest[1:]
		case "--fresh":
			d.checkReuse, d.freshSource = false, "--fresh" // # MUTATION-ANCHOR: 844-71-M02
			rest = rest[1:]
		case "--no-reuse-check":
			d.checkReuse, d.freshSource = false, "--no-reuse-check (legacy alias for --fresh)"
			rest = rest[1:]
		default:
			d.die("unknown option: " + rest[0])
		}
	}
}
