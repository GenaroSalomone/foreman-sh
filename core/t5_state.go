package main

// Train T5: the dispatch `hw <lane> <task> … --dry-run`. State of the dispatch, as bin/hw keeps it in globals
// (the block at bin/hw "PROJ=$(_canon_project …)" through `_dispatch_manifest`). One struct, one field per bash
// global, named after it in lowerCamel (ACCOUNT_SRC → accountSrc). The areas of the port keep what is private to
// them in their own embedded struct (brState, vdState, rtState); anything two areas read is a field here.
//
// Conventions of the port, so the files of four authors read as one:
//   · a bash function `_check_brief_contract` is the method `checkBriefContract`: the name without its leading
//     underscore, in lowerCamel. A function that prints a value for `$(…)` returns it; a predicate returns bool;
//     one that dies calls d.die (a panic that unwinds to main with status 1, as `die` exits).
//   · `warn`, `info`, `ok`, `step` and `die` are the embedded term's; the text is bash's, byte for byte.
//   · the environment is read through d.env(k), never os.Getenv: `--report-to` assigns HW_INVOKER_PANE inside the
//     process in bash, and d.env sees that, as bash's `${HW_INVOKER_PANE}` did.
//   · nothing here writes except the route forecast (rtState), and that only when !d.readonly: HW_CORE_READONLY=1
//     is how shadow runs Go first, and shadow never writes.

type dsp struct {
	fsShared *fsx     // see sharedFs
	dispArgv []string // the dispatch argv, for the launch (handBackToBash)
	*app
	brState
	vdState
	rtState

	readonly bool              // HW_CORE_READONLY=1: shadow's first leg writes nothing
	envOver  map[string]string // what the dispatch assigned to the environment (HW_INVOKER_PANE by --report-to)

	// identity
	proj, task string

	// the vendor, model and account (bin/hw "AGENT=…")
	agent, agentSrc                                              string
	agentExplicit                                                bool
	model, modelSrc                                              string
	modelWhy, modelWhySrc                                        string
	modelPinned                                                  string
	modelFloorLine                                               string
	belowFloorWhy                                                string
	topTierWhy, topTierLine                                      string // TOP_TIER_WHY, TOP_TIER_LINE (_check_top_tier_ceiling)
	explorerLine                                                 string // EXPLORER_LINE (_explorer_resolve)
	explorerInjected                                             bool
	explorerArm                                                  bool // EXPLORER_ARM: claude's launch writes the explorer's file and arms the pane
	effort                                                       string
	account, accountSrc                                          string
	subagentModel, subagentModelSrc, subagentModelLine           string // SUBAGENT_MODEL, _SRC, _LINE
	permissionsFlag                                              string
	permissions, permissionsSrc, permissionsLine, permissionsErr string
	agentArgs                                                    string
	sdd                                                          string
	sddEffectiveMode                                             string

	// flags
	brief, briefText                          string
	noBrief                                   bool
	wantDB                                    bool
	here                                      bool
	hereSrc                                   string
	wantDevserver                             bool
	devserverSrc                              string
	wantReport                                bool
	keepPane                                  bool
	keepPaneFor, keepPaneSeconds, keepPaneWhy string
	wantWT                                    bool
	wtSrc                                     string
	wtExplicit                                bool
	dbSrc                                     string
	tab                                       bool
	tabExplicit, spaceExplicit                bool
	dispatchDry                               bool
	sandbox                                   bool
	sandboxSrc, sandboxLine                   string
	checkHandles                              bool
	allowStaleHooks                           bool
	checkReuse                                bool
	freshSource                               string
	baseOverride, baseSrc                     string
	requestedBy, requestedBySrc               string
	reportToChosen                            bool

	// the brief's declarations (brState owns the readers)
	briefDeclared         string // BRIEF_DECLARED
	taskKind, taskKindSrc string

	// the account verdict (_resolve_account)
	accountLine, accountVerdict, accountRefusal, accountConfigDir string
	accountApplies                                                bool

	// the lines the vendor step hands the manifest
	artifactsLine, engramProxyLine string
	opencodePrimaryAgent           string
	opencodeAgentPort              string
	opencodeExecutorConfig         string
	opencodeEffortOverlay          string
	hwAgentEndpoint                string

	// the invoker (the brainer pane reports go to), resolved lazily as bash does
	invokerDone                                          bool
	invokerResolved, invokerHow                          string
	hwInvokerVendor, hwInvokerSession, hwInvokerEndpoint string

	// the reuse route (_check_reusable_executor and the reuse refusals)
	reusePane, reuseRundir, reuseWhy, reuseTask, reuseLeaseReason string
	reuseLine                                                     string
	reuseDiscarded                                                string
	reuseModel, reuseEffort, reuseVendor                          string
	reuseAccountCfg                                               string
	reuseAccountMeasured                                          bool
	reuseFlagVerdict                                              string
	routeNow                                                      string
	dispatchRoute                                                 string // fresh | reuse

	// ODD (--sdd gentle), read by the manifest
	gentleVersion, gentleDelivery, gentleDeployedCheck string

	// the manifest
	dispatchText      string
	dispatchFramework string
	dispatchInPlay    bool
	verifyFullSuite   bool
	depsLine          string
	hwRun             string
	routeForecastTTL  string // HW_ROUTE_FORECAST_TTL, as the text prints it
	notifyBody        string // the operator notification of a launch (t5l.go)
}

// env is bash's ${VAR:-}: what the dispatch assigned, else the process's environment.
func (d *dsp) env(k string) string {
	if v, ok := d.envOver[k]; ok {
		return v
	}
	return d.getenv(k)
}

// exportedEnv is the dispatch whose exports every herdr child inherits (bash `export`s are process-wide; the
// overrides here are not the process's, so the herdr helpers below ask for them).
var exportedEnv *dsp

func childEnv() []string {
	if exportedEnv == nil || len(exportedEnv.envOver) == 0 {
		return nil
	}
	return exportedEnv.environ()
}

func (d *dsp) setEnv(k, v string) {
	exportedEnv = d
	if d.envOver == nil {
		d.envOver = map[string]string{}
	}
	d.envOver[k] = v
}
