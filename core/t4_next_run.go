package main

// The two ends of `hw next`: the dry run (everything except the send and the counter) and the delivery (the
// task directory, the SDD surface, the lease, channel-send, the receipts, the counter, the ledger) — and the
// refusals and readings on the way there that live outside next.sh: the missing pane's diagnosis
// (_next_missing_pane_message, _next_autoclosed_run_for_pane), stale hooks (_refuse_if_stale_hooks), the SDD
// gates (_validate_sdd_vendor_pair, _validate_sdd_model_pair, _apply_sdd), the context line
// (_report_pane_context), the report evidence and the ledger (_ledger_next). Dispatch helpers (T5), copied.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ── the dry run ─────────────────────────────────────────────────────────────

func (a *app) nextDryRun(s *nextState, nextSeq string, waitMS int64) int {
	seconds, _ := a.nextRunEnv(s.rundir, "HW_CHAINING_LEASE_SECONDS", true)
	seconds = bashSub(seconds)
	if seconds != "" && !s.reopened {
		if st := nextLeaseEligibility(s.rundir, s.seq); st != "live" { // # MUTATION-ANCHOR: 862-G20
			a.die(nextLeaseIneligible(st, s.seq, s.rundir))
		}
	}
	a.ok("dry run: " + s.pane + " is reachable, on task " + s.seq + " of " + nextBasename(s.rundir) + " — NOTHING was sent and the pane was not touched")
	ep := ""
	if s.endpoint != "" {
		ep = " (endpoint " + s.endpoint + ")"
	}
	a.info("route " + s.route + " -> " + s.target + ep)
	msg := bashSub(a.nextMessage(s, nextSeq, ""))
	a.info("would create " + s.rundir + "/t" + nextSeq + " and deliver " + nextWc(msg) + " bytes as task " + nextSeq + " (" + nextWc(s.text) + " of them the task text; the rest is the preamble hw prepends, minus any framework entry line)")
	if s.reopened {
		a.info("run was revived (reopened): hw revive is the authority for this re-task, so its spent chaining lease is not consulted; task " + nextSeq + " gets its own done-invoker with no --retask")
	}
	if seconds != "" && !s.reopened {
		a.info("this one use would be consumed only after successful delivery; dry-run lease state is volatile and was not touched")
	}
	budget := strconv.FormatInt(waitMS, 10)
	if nextFinishedStaleStuck(s.info) {
		a.ok("the delivery gate WOULD OPEN: " + s.pane + " has authoritative delivered completion tokens; its blocked/stuck lifecycle token is stale (budget " + s.waitMS + "ms, unspent)")
	} else {
		switch s.status {
		case "idle", "done":
			a.ok("the delivery gate WOULD OPEN: herdr reports " + s.pane + " " + s.status + ", so a prompt would be read now (budget " + s.waitMS + "ms, unspent)")
		default:
			a.info("herdr reports " + s.pane + " " + s.status + ", so the gate would wait (budget " + s.waitMS + "ms). Cross-checking whether that state can still change — this does not send, dismiss or wait:")
			var w nextWitness
			a.nextWitnessState(s.pane, &w)
			switch w.verdict {
			case "live":
				a.info("gate verdict: WOULD WAIT, and correctly. " + w.detail)
			case "awaiting-human":
				a.warn("gate verdict: WOULD NOT OPEN. " + w.detail)
			case "settled":
				a.warn("gate verdict: WOULD NOT OPEN, and no budget would help. " + w.detail)
				a.warn("a real run would refuse here after confirming that twice, instead of spending " + s.waitMS + "ms on it")
			default:
				a.info("gate verdict: UNDETERMINED. " + w.detail)
			}
		}
	}
	_ = budget
	if s.nextSDD != "" {
		e := nextFrameworkEntry(s.nextSDD)
		if e == "" {
			e = "<none for this mode>"
		}
		a.info("would set SDD mode " + s.nextSDD + " on the worktree, entry " + e)
	} else {
		a.info("SDD surface: unchanged — compatibility checked using the effective mode; live surface takes precedence, with the launch record as fallback when no live mode is obtained")
	}
	return 0
}

// nextFrameworkEntry is _framework_entry.
func nextFrameworkEntry(mode string) string {
	if mode == "speckit" {
		return "/speckit-specify"
	}
	return ""
}

// ── the delivery ────────────────────────────────────────────────────────────

func (a *app) nextDeliver(s *nextState, nextSeq string) int {
	tdir := s.rundir + "/t" + nextSeq
	if os.MkdirAll(tdir, 0o777) != nil {
		a.die("cannot create " + tdir)
	}
	entry := ""
	if s.nextSDD != "" {
		e := nextFrameworkEntry(s.nextSDD)
		if !a.nextApplySDD(s.wtdir, s.nextSDD) {
			a.die("--sdd " + s.nextSDD + " has no speckit-* skill on disk for " + s.wtdir + ", so nothing was sent and the task counter did not move. Install Spec Kit where it is used, or re-task with --sdd none.")
		}
		if e != "" {
			entry = bashSub("FIRST ACTION, before anything else: run " + e + " for this task. The framework mode\nfor it is " + s.nextSDD + " and what follows is its input, not a licence to start editing.\n\n")
			if s.nextSDD == "speckit" {
				entry = entry + "\n\n" + bashSub(a.nextSpeckitNote()) + "\n\n"
			}
		} else {
			entry = bashSub("Framework mode for this task: " + s.nextSDD + ". hw forces no entry command.\n\n")
		}
	}
	msg := bashSub(a.nextMessage(s, nextSeq, entry))

	// The lease: held under the run's lock across delivery, consumption and the counter.
	seconds, _ := a.nextRunEnv(s.rundir, "HW_CHAINING_LEASE_SECONDS", true)
	seconds = bashSub(seconds)
	if s.reopened {
		seconds = ""
	}
	lock := ""
	release := func() {
		if lock != "" {
			os.Remove(lock)
			lock = ""
		}
	}
	if seconds != "" {
		l, ok := a.nextLockTake(s.rundir)
		if !ok {
			a.die("could not acquire the run/task lock before consuming its chaining lease")
		}
		lock = l
		if st := nextLeaseEligibility(s.rundir, s.seq); st != "live" {
			release()
			a.die(nextLeaseIneligible(st, s.seq, s.rundir))
		}
	}

	bin := a.nextBinDir()
	var args []string
	if s.route == "opencode" {
		args = []string{"--require", "processed", "--id", nextBasename(s.rundir) + ":t" + nextSeq, s.route, s.target, s.endpoint, msg}
	} else {
		args = []string{s.route, s.target, s.endpoint, msg}
	}
	cmd := exec.Command(bin+"/channel-send", args...)
	cmd.Stderr = a.err
	sendRC := exitStatus(cmd.Run())           // stdout to /dev/null, as bash sends it
	if sendRC == 4 && s.route == "opencode" { // # MUTATION-ANCHOR: 862-G22
		a.warn(s.pane + " ACCEPTED task " + nextSeq + " — it is in its session — but the turn it started did not complete (see the message above).")
		a.warn("that is a turn failure, not a delivery failure, so the counter advances: rolling it back would leave the executor holding a task its invokers do not know about.")
		a.info("check what it did with: herdr agent read " + s.pane + " --source recent-unwrapped")
	} else if sendRC != 0 {
		os.Remove(tdir) // rmdir: only an empty directory goes
		release()
		a.die("delivery to " + s.pane + " failed via " + s.route + " (exit " + itoa(sendRC) + "). The task counter was NOT advanced, so the executor is still on task " + s.seq + " and its next done-invoker still refers to the task it actually ran. The pane is untouched and still usable — a failed delivery is not a reason to close an executor; fix the transport and retry.")
	}

	if lock != "" {
		by := a.nextEnvOr("HERDR_PANE_ID", "unknown")
		f, err := os.OpenFile(s.rundir+"/chaining-lease", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
		if err == nil {
			_, err = f.WriteString("state=consumed\nconsumed_at=" + nextDateUTC() + "\nconsumed_by=" + by + "\n") // # MUTATION-ANCHOR: 862-G23
			f.Close()
		}
		if err != nil {
			release()
			a.die("task delivered, but its chaining lease could not be marked consumed; refusing to advance the counter")
		}
	}
	if s.brief != "" && nextDesignGate(s.brief) {
		a.receiptInto(s.rundir, "design_boundary_t"+nextSeq, nextFmValue("boundary", s.brief),
			"brief frontmatter boundary: in "+s.brief+" at hw next — done-invoker requires an APPROVED design judgment")
	}
	if s.brief != "" && nextChecksGate(s.brief) {
		a.receiptInto(s.rundir, "checks_required_t"+nextSeq, "required",
			"kind build (declared or undeclared) at hw next — done-invoker requires $HW_ARTIFACTS/checks.md")
	}
	// THE REVIEW THE NEW BRIEF CHOSE, keyed by task like the boundary above.
	if s.brief != "" {
		a.nextReviewReceipt(s.rundir, "review_t"+nextSeq, s.brief)
	}
	if err := os.WriteFile(s.rundir+"/task", []byte(nextSeq+"\n"), 0o666); err != nil { // # MUTATION-ANCHOR: 862-G24
		release()
		a.die("delivered, but could not write " + s.rundir + "/task — the executor is on task " + nextSeq + " and its invokers still think " + s.seq + ". Write that file by hand before it reports.")
	}
	if s.reopened {
		os.Remove(s.reopenedMarker) // # MUTATION-ANCHOR: 862-G25
	}
	release()
	a.nextLedger(s.pane, s.rundir, nextSeq, s.brief)
	v := s.vendor
	if v == "" {
		v = "unknown"
	}
	a.ok("task " + nextSeq + " dispatched to " + s.pane + " via " + s.route + " (" + v + " executor, " + nextWc(msg) + " bytes)")
	a.info("state for it: " + tdir + " — fresh ask budget, no done marker")
	return 0
}

// nextApplySDD is _apply_sdd for the two modes `hw next` can apply.
func (a *app) nextApplySDD(dir, mode string) bool {
	if mode != "speckit" {
		return true
	}
	if where, ok := nextSpeckitOnDisk(dir, a.c.home); ok {
		a.ok("SDD mode: speckit (skills in " + where + ")")
		return true
	}
	a.warn("--sdd speckit, but no speckit-* skill is on disk under " + dir + "/.claude/skills or " + a.c.home + "/.claude/skills")
	return false
}

// nextSpeckitIn is _speckit_skills_in: <dir>/.claude/skills when a speckit-* entry is in it.
func nextSpeckitIn(dir string) (string, bool) {
	d := dir + "/.claude/skills"
	if !isDir(d) {
		return "", false
	}
	ents, err := os.ReadDir(d)
	if err != nil {
		return "", false
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "speckit-") {
			if _, err := os.Stat(d + "/" + e.Name()); err == nil { // [ -e ]: a dangling link does not count
				return d, true
			}
		}
	}
	return "", false
}

func nextSpeckitOnDisk(dir, home string) (string, bool) {
	if w, ok := nextSpeckitIn(dir); ok {
		return w, true
	}
	return nextSpeckitIn(home)
}

// nextValidateSDDVendor is _validate_sdd_vendor_pair (with _resolve_effective_sdd_mode): the effective mode.
func (a *app) nextValidateSDDVendor(vendor, requested, surface, inherited string) string {
	mode := requested
	if mode == "" {
		if _, ok := nextSpeckitIn(surface); ok {
			mode = "speckit"
		}
		if mode == "" {
			mode = inherited
		}
	}
	switch vendor + ":" + mode {
	case "codex:speckit":
		a.die("Codex framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd " + mode + " requires a demonstrated native entry and completed flow, but that capability is unverified. Nothing was built. Use --sdd none for direct delivery, or validate Codex framework execution before re-dispatching.")
	case "opencode:speckit": // # MUTATION-ANCHOR: 862-G26
		a.die("OpenCode framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd " + mode + " is Spec Kit, but this machine exposes Spec Kit only as Claude Code skills. Nothing was built. Use --agent claude for Spec Kit, or --sdd none for direct delivery.")
	}
	return mode
}

// nextValidateSDDModel is _validate_sdd_model_pair: a sonnet or haiku session cannot orchestrate Spec Kit.
func (a *app) nextValidateSDDModel(model, mode, where string) {
	if mode != "speckit" || model == "" {
		return
	}
	last := model
	if i := strings.LastIndexByte(last, '/'); i >= 0 {
		last = last[i+1:]
	}
	if !strings.Contains(last, "sonnet") && !strings.Contains(last, "haiku") { // # MUTATION-ANCHOR: 862-G27
		return
	}
	declared := ""
	if b := a.getenv("BRIEF"); b != "" { // the shell's BRIEF: `hw next` never sets it on this route
		declared = nextDecl(nextBriefDeclares(b), "sdd")
	}
	exits := "Relaunch with --model opus --effort medium, or dispatch outside the flow with --sdd none."
	if declared == "none" {
		exits = "The brief already declared `sdd: none`, and this dispatch did not carry it: relaunch with --sdd none, which is what the brief asked for. Only if the brief is what is wrong, relaunch with --model opus --effort medium instead."
	}
	w := ""
	if where != "" {
		w = " (" + where + ")"
	}
	a.die("SDD orchestrator model REFUSED BEFORE BUILDING ANYTHING: --model " + model + " with --sdd " + mode + w + ". Under " + mode + " this session is the ORCHESTRATOR -- it decides what the change is, partitions it and sequences the phases, and the phase agents pin their own models regardless of --model. A sonnet or haiku orchestrator over opus phase agents is inverted. Nothing was built. " + exits)
}

// ── readings on the way ─────────────────────────────────────────────────────

// nextWaitFromRun is _next_wait_from_run: the run's own HW_NEXT_WAIT_MS, when it is a decimal.
func (a *app) nextWaitFromRun(rundir string) string {
	v, rc := a.nextRunEnv(rundir, "HW_NEXT_WAIT_MS", true)
	v = bashSub(v)
	if rc != 0 || v == "" || strings.Trim(v, "0123456789") != "" {
		return ""
	}
	return v
}

// nextReportPaneContext is _report_pane_context.
func (a *app) nextReportPaneContext(pane string) {
	state, tok, pct := a.paneContext(pane)
	warnAt := a.nextEnvOr("HW_NEXT_CTX_WARN_TOKENS", "300000")
	switch state {
	case "measured":
		t, err1 := strconv.ParseInt(tok, 10, 64)
		w, err2 := strconv.ParseInt(warnAt, 10, 64)
		if err1 == nil && err2 == nil && t >= w { // # MUTATION-ANCHOR: 862-G28
			ps := ""
			if pct != "" {
				ps = " (" + pct + "% of its window)"
			}
			a.warn("this session has already burned " + tok + " tokens of context" + ps + ". " + a.nextOperator() + "'s rule is not to re-task past " + warnAt + ": the next task would start compacted, which costs more than a fresh executor. Re-tasking anyway is a deliberate choice, not a default.")
		} else {
			ps := ""
			if pct != "" {
				ps = " (" + pct + "% of the window)"
			}
			a.info("context: " + tok + " tokens used" + ps + ", under the " + warnAt + " re-task threshold")
		}
	case "no-statusline":
		a.warn("context: NOT MEASURED — the pane responded but rendered no `Ctx:` statusline row (no statusline configured, or it is scrolled out of the viewport). This is not 'a fresh session'; it is no reading at all, and the +" + warnAt + " rule cannot be applied to this pane from here.")
	default:
		a.warn("context: NOT MEASURED — `herdr agent read " + pane + " --source visible` returned nothing readable. This is not 'a fresh session'; it is no reading at all.")
	}
}

// nextReportEvidence is _report_evidence: "marker", "token <status> <state>" or "none".
func (a *app) nextReportEvidence(pane, rundir string) string {
	if isFile(nextDoneMarker(rundir)) {
		return "marker"
	}
	info := nextHerdrSub("agent", "get", pane)
	switch tok := jqRawOr(info, "", "result", "agent", "tokens", "done_status"); tok {
	case "done", "blocked":
		st := jqRawOr(info, "unknown", "result", "agent", "tokens", "done_state")
		if st == "" {
			st = "unknown"
		}
		return "token " + tok + " " + st
	}
	return "none"
}

// nextReportEvidenceExplain is _report_evidence_explain.
func (a *app) nextReportEvidenceExplain(pane, verdict, marker string) {
	if !strings.HasPrefix(verdict, "token") {
		return
	}
	f := strings.Fields(verdict)
	for len(f) < 3 {
		f = append(f, "")
	}
	a.warn("no " + marker + ", but " + pane + "'s own tokens say the executor reported: done_status=" + f[1] + ", done_state=" + f[2])
	a.warn("the report was made and its delivery to you failed — that is a transport fact, not an unfinished task")
}

// ── the missing pane ────────────────────────────────────────────────────────

// nextMissingPaneMessage is _next_missing_pane_message.
func (a *app) nextMissingPaneMessage(pane string) string {
	generic := "herdr does not know pane " + pane + " (or herdr is unreachable on $HERDR_SOCKET_PATH)"
	panes, _ := herdrOut(0, "pane", "list")
	vals, perrE := parseStream([]byte(panes))
	perr := perrE != nil
	if perr || len(vals) == 0 {
		return generic
	}
	isArr := false
	found := false
	for _, v := range vals { // jq -e: the status is the last output's
		arr, ok := nextPath(v, "result", "panes")
		isArr = ok && arr.kind == vArr
		if !isArr {
			continue
		}
		for _, p := range arr.arr {
			if id, ok := p.get("pane_id"); ok && id.kind == vStr && id.s == pane {
				found = true
			}
		}
	}
	if !isArr {
		return generic
	}
	if found {
		return "herdr pane " + pane + " exists but has no agent record, so hw cannot re-task it"
	}
	if ev := a.nextAutoclosedRun(pane); ev != "" {
		return "pane " + pane + " belonged to completed " + ev + " and was launched with automatic close. Chaining is a launch-time decision: launch with `hw ... --keep-pane` when this executor must survive its report for `hw next`."
	}
	return generic
}

// nextAutoclosedScript is _next_autoclosed_run_for_pane's walk, run as bash runs it: python3 over the roots,
// loading bin/runenv.
const nextAutoclosedScript = `import importlib.machinery, importlib.util, json, os, sys

sys.dont_write_bytecode = True  # never a __pycache__ beside bin/runenv
pane = os.environ["HW_NEXT_PANE"]
roots = [r for r in os.environ.get("HW_NEXT_ROOTS", "").split("\n") if r]

# bin/runenv is the one reader of a run's env file; loaded, not re-implemented.
_loader = importlib.machinery.SourceFileLoader("runenv", os.environ["HW_RUNENV"])
runenv = importlib.util.module_from_spec(importlib.util.spec_from_loader("runenv", _loader))
_loader.exec_module(runenv)

def env_value(path, key):
    try:
        values = runenv.read(path, lenient=True, bare=True)
    except runenv.Unusable:
        return None
    return None if values is None else values.get(key)

def receipt_pane(path):
    found = None
    try:
        lines = open(path, encoding="utf-8")
    except OSError:
        return None
    with lines:
        for line in lines:
            try:
                rec = json.loads(line)
            except ValueError:
                continue
            if rec.get("key") == "pane" and rec.get("value"):
                found = rec["value"]
    return found

for root in roots:
    if not os.path.isdir(root):
        continue
    for directory, dirs, files in os.walk(root):
        if "receipt.jsonl" not in files or os.path.basename(os.path.dirname(directory)) != ".hw":
            continue
        if receipt_pane(os.path.join(directory, "receipt.jsonl")) != pane:
            continue
        if env_value(os.path.join(directory, "env"), "HW_CHAINING_ENABLED") != "0":
            continue
        if not any(os.path.isfile(os.path.join(directory, k)) for k in ("tab", "workspace")):
            continue
        seq = "1"
        try:
            raw = open(os.path.join(directory, "task"), encoding="utf-8").read()
            digits = "".join(c for c in raw if c.isdigit())
            if digits and digits != "0":
                seq = digits
        except OSError:
            pass
        marker = (os.path.join(directory, "done") if seq == "1" else
                  os.path.join(directory, "t" + seq, "done"))
        if not os.path.isfile(marker):
            continue
        project = env_value(os.path.join(directory, "env"), "HW_PROJECT") or "unknown-project"
        task = env_value(os.path.join(directory, "env"), "HW_TASK") or "unknown-task"
        print("%s:%s (run %s, task %s)" % (project, task, os.path.basename(directory), seq))
        raise SystemExit(0)
`

// nextAutoclosedRun is _next_autoclosed_run_for_pane: the completed, auto-closing run that owned the pane.
func (a *app) nextAutoclosedRun(pane string) string {
	roots := a.getenv("HW_NEXT_EVIDENCE_ROOTS")
	if roots == "" {
		roots = a.c.work
		if derived := a.nextLaneRoots(); len(derived) > 0 {
			roots = a.c.work + "\n" + strings.Join(derived, "\n")
		}
	}
	cmd := exec.Command("python3", "-")
	cmd.Stdin = strings.NewReader(nextAutoclosedScript)
	cmd.Env = append(os.Environ(), "HW_NEXT_PANE="+pane, "HW_NEXT_ROOTS="+roots, "HW_RUNENV="+a.nextBinDir()+"/runenv")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()
	return bashSub(out.String())
}

// ── stale hooks ─────────────────────────────────────────────────────────────

const nextStaleHookScript = `import json, os, re, shlex, shutil, sys

wt = sys.argv[1]

def find_commands(obj):
    out = []
    if isinstance(obj, dict):
        if obj.get("type") == "command" and isinstance(obj.get("command"), str):
            out.append(obj["command"])
        for v in obj.values():
            out.extend(find_commands(v))
    elif isinstance(obj, list):
        for v in obj:
            out.extend(find_commands(v))
    return out

def expand_known(cmd):
    for name in ("CLAUDE_PROJECT_DIR", "PWD"):
        cmd = cmd.replace("${" + name + "}", wt).replace("$" + name, wt)
    return cmd

def missing_binary(cmd):
    cmd = expand_known(cmd)
    try:
        tokens = shlex.split(cmd)
    except ValueError:
        return None
    while tokens and re.match(r'^[A-Za-z_][A-Za-z0-9_]*=', tokens[0]):
        tokens = tokens[1:]
    if not tokens:
        return None
    b = tokens[0]
    if "$" in b:
        return None
    if b.startswith("/"):
        ok = os.path.isfile(b) and os.access(b, os.X_OK)
    elif "/" in b:
        p = os.path.join(wt, b)
        ok = os.path.isfile(p) and os.access(p, os.X_OK)
    else:
        ok = shutil.which(b) is not None
    return None if ok else b

for path in sys.argv[2:]:
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, json.JSONDecodeError):
        continue
    seen = set()
    for cmd in find_commands(data):
        if cmd in seen:
            continue
        seen.add(cmd)
        binname = missing_binary(cmd)
        if binname:
            print(f"{path}\t{cmd}\t{binname}")
`

// nextRefuseIfStaleHooks is _refuse_if_stale_hooks for `hw next` (no main checkout, no base, and nothing this
// route created to roll back): "" when the worktree's hook settings are sound, the files' list when
// --allow-stale-hooks lets it through; it dies otherwise.
func (a *app) nextRefuseIfStaleHooks(wt string, allow bool) string {
	var files []string
	for _, f := range []string{wt + "/.claude/settings.json", wt + "/.claude/settings.local.json", wt + "/.codex/hooks.json", wt + "/opencode.json"} {
		if isFile(f) {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return ""
	}
	cmd := exec.Command("python3", append([]string{"-", wt}, files...)...)
	cmd.Stdin = strings.NewReader(nextStaleHookScript)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = a.err
	if err := cmd.Run(); err != nil {
		panic(exitNow{exitStatus(err)}) // `report="$(…)"` under set -e
	}
	report := bashSub(out.String())
	if report == "" {
		return ""
	}
	lines := strings.Split(report, "\n")
	var fs []string
	for _, l := range lines {
		f, _, _ := strings.Cut(l, "\t")
		fs = append(fs, f)
	}
	list := strings.Join(nextSortedUnique(fs), " ")
	each := func() {
		for _, l := range lines {
			f := strings.SplitN(l, "\t", 3)
			for len(f) < 3 {
				f = append(f, "")
			}
			a.warn("  " + f[0] + ": '" + f[1] + "' -> '" + f[2] + "' not found")
		}
	}
	if allow || a.getenv("ALLOW_STALE_HOOKS") == "1" { // # MUTATION-ANCHOR: 862-G29
		a.warn("STALE HOOKS in " + wt + " — launching anyway (--allow-stale-hooks): " + list)
		each()
		return list
	}
	a.warn("this worktree's VERSIONED hook settings invoke a binary hw cannot find on this machine:")
	each()
	if _, err := os.Stat(wt + "/.git"); err != nil {
		a.die("refusing to hand a brief to " + wt + ": its versioned hook settings (" + list + ") invoke a binary that is not on PATH — see above. " + wt + " is not a git worktree (no .git), so there is no branch to merge and nothing to remove with `git worktree remove`. Fix the file(s) directly, or dispatch under a task name with no existing directory so hw builds a real worktree. Override with --allow-stale-hooks if you must launch onto it anyway (recorded in the receipt).")
	}
	branch, _ := nextGitOut(wt, "symbolic-ref", "--short", "HEAD")
	_ = branch
	a.die("refusing to hand a brief to " + wt + ": its versioned hook settings (" + list + ") invoke a binary that is not on PATH — see above. This branch predates the cleanup that dropped it. Fix: git -C '" + wt + "' merge <this lane's base branch> — brings the base's current settings into this branch. Or start over: remove the worktree AND its branch — both, or the next dispatch reuses the branch and hits this same refusal again — then dispatch again so hw builds a fresh worktree off the base. Override with --allow-stale-hooks if you must launch onto it anyway (recorded in the receipt).")
	return ""
}

// ── the ledger (lib/hw/ledger.sh) ───────────────────────────────────────────

// nextLedger is _ledger_next: one JSONL line for this re-task, the lane and task from the run, the brief from
// the call (and committed to the brain's main first, when it is one of its briefs and is not).
func (a *app) nextLedger(pane, rundir, seq, brief string) {
	run := nextBasename(rundir)
	proj := a.nextEnvVal(rundir, "HW_PROJECT")
	task := a.nextEnvVal(rundir, "HW_TASK")
	if brief != "" {
		task = strings.TrimSuffix(nextBasename(brief), ".md")
	}
	if proj == "" || task == "" { // # MUTATION-ANCHOR: 862-G30
		return
	}
	model := nextDispatchModel(rundir)
	effort := nextDispatchEffort(rundir)
	vendor := a.nextEnvVal(rundir, "HW_EXECUTOR_VENDOR")
	// the pane's own subagent model, from its env; no line there means it inherits (bash `SUBAGENT_MODEL=${…:-inherit}`)
	subagent := a.nextEnvVal(rundir, "CLAUDE_CODE_SUBAGENT_MODEL")
	if subagent == "" {
		subagent = "inherit"
	}
	commit := a.nextCommitBrief(brief, proj, task)
	// _ledger_dispatch
	sha := ""
	if brief != "" && isFile(brief) {
		if out, err := exec.Command("git", "hash-object", "--", brief).Output(); err == nil {
			sha = bashSub(string(out))
		}
	}
	base := a.c.t.get(proj, "base")
	dir := a.nextEnvOr("HW_LEDGER_DIR", nextOr(a.c.work, a.c.home+"/work")+"/.hw-ledger")
	f := dir + "/" + proj + ".jsonl"
	if os.MkdirAll(dir, 0o777) != nil {
		a.warn("ledger: cannot create " + dir + "; this dispatch is NOT recorded")
		return
	}
	n, _ := strconv.ParseInt(seq, 10, 64)
	nul := func(s string) *jval {
		if s == "" {
			return &jval{kind: vNull}
		}
		return jstr(s)
	}
	obj := &jval{kind: vObj, obj: []member{
		{"v", &jval{kind: vNum, s: "1"}}, {"ts", jstr(nextDateUTC())}, {"lane", jstr(proj)}, {"task", jstr(task)},
		{"run", jstr(run)}, {"seq", &jval{kind: vNum, s: strconv.FormatInt(n, 10)}},
		{"brief", nul(brief)}, {"brief_sha", nul(sha)}, {"brief_commit", nul(commit)},
		{"account", jstr(a.getenv("ACCOUNT"))}, {"model", jstr(model)}, {"effort", jstr(effort)}, {"vendor", jstr(vendor)},
		{"sdd", jstr(a.getenv("SDD"))}, {"base", jstr(base)}, {"subagent_model", jstr(nextSubagentModelReceipt(vendor, subagent))},
		{"pane", jstr(pane)}, {"rundir", jstr(rundir)},
		{"delivered", &jval{kind: vBool, b: true}},
	}}
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		a.warn("ledger: could not append to " + f + "; this dispatch is NOT recorded")
		return
	}
	defer fh.Close()
	fh.WriteString(obj.toJSON() + "\n")
}

// nextSubagentModelReceipt is _subagent_model_receipt_value as _ledger_next calls it, with AGENT the run's vendor and
// SUBAGENT_MODEL the run's CLAUDE_CODE_SUBAGENT_MODEL: only claude reads it, and `inherit` exports nothing.
func nextSubagentModelReceipt(vendor, model string) string {
	if vendor != "claude" {
		if vendor == "" {
			vendor = "unknown"
		}
		return "NOT APPLIED — " + vendor + " does not read CLAUDE_CODE_SUBAGENT_MODEL (asked: " + model + ")"
	}
	if model != "" && model != "inherit" {
		return model
	}
	return "inherit"
}

func nextOr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// nextDispatchEffort is _next_dispatch_effort's reading for the ledger: the `effort=` value, its trailing
// `  (mark)` taken off.
func nextDispatchEffort(rundir string) string {
	b, err := os.ReadFile(rundir + "/dispatch")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		t := strings.TrimLeft(l, " \t\v\f\r")
		if !strings.HasPrefix(t, "effort=") {
			continue
		}
		line := t[len("effort="):]
		if m := nextReMark.FindStringIndex(line); m != nil {
			line = line[:m[0]]
		}
		return line
	}
	return ""
}

// nextCommitBrief is _ledger_commit_brief: a brief of the brain's own (<brain>/*/briefs/*.md), committed to
// its main by the private-index recipe when it is not, or the last commit that touched it when it is clean.
func (a *app) nextCommitBrief(brief, proj, task string) string {
	if brief == "" || !isFile(brief) {
		return ""
	}
	dir, err := nextPhysDir(nextDirname(brief))
	if err != nil {
		return ""
	}
	abs := dir + "/" + nextBasename(brief)
	brain, err := nextPhysDir(a.c.brain)
	if err != nil {
		brain = ""
	}
	rel0 := strings.TrimPrefix(abs, brain+"/")
	if brain == "" || rel0 == abs || !nextBriefShape(rel0) {
		return ""
	}
	topOut, err := nextGitOut(a.c.brain, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	top, err := nextPhysDir(bashSub(topOut))
	if err != nil {
		return ""
	}
	rel := strings.TrimPrefix(abs, top+"/")
	if rel == abs {
		return ""
	}
	dirty, err := nextGitOut(top, "status", "--porcelain", "--", rel)
	if err != nil {
		return ""
	}
	if bashSub(dirty) == "" {
		c, _ := nextGitOut(top, "log", "-1", "--format=%H", "--", rel)
		return bashSub(c)
	}
	branch, _ := nextGitOut(top, "symbolic-ref", "--short", "-q", "HEAD")
	branch = bashSub(branch)
	if branch != "main" {
		b := branch
		if b == "" {
			b = "a detached HEAD"
		}
		a.warn("brief " + rel + " is not committed and the brain checkout is on '" + b + "', not main: left uncommitted")
		return ""
	}
	oldOut, err := nextGitOut(top, "rev-parse", "-q", "--verify", "refs/heads/main")
	if err != nil {
		return ""
	}
	old := bashSub(oldOut)
	idxf, err := os.CreateTemp(a.nextEnvOr("TMPDIR", "/tmp"), "hw-brief-idx.")
	if err != nil {
		return ""
	}
	idx := idxf.Name()
	idxf.Close()
	os.Remove(idx)
	blobOut, err := nextGitOut(top, "hash-object", "-w", "--", rel)
	if err != nil {
		os.Remove(idx)
		a.warn("brief commit: could not hash " + rel)
		return ""
	}
	blob := bashSub(blobOut)
	mode := "100644"
	if nextExecutable(abs) {
		mode = "100755"
	}
	gidx := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", top}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+idx)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		return out.String(), err
	}
	built := false
	if _, err := gidx("read-tree", old); err == nil {
		if _, err := gidx("update-index", "--add", "--cacheinfo", mode+","+blob+","+rel); err == nil {
			built = true
		}
	}
	newtree := ""
	if built {
		out, err := gidx("write-tree")
		if err != nil {
			built = false
		}
		newtree = bashSub(out)
	}
	if !built {
		os.Remove(idx)
		a.warn("brief commit: could not build the private index for " + rel + "; left uncommitted")
		return ""
	}
	os.Remove(idx)
	changed, _ := nextGitOut(top, "diff-tree", "-r", "--name-only", old+"^{tree}", newtree)
	if bashSub(changed) != rel {
		a.warn("brief commit REFUSED: the tree it built differs from HEAD in more than " + rel + "; left uncommitted")
		return ""
	}
	cOut, err := nextGitOut(top, "commit-tree", newtree, "-p", old, "-m", "docs(briefs): "+nextOr(proj, "lane")+"/"+nextOr(task, "task")+" brief, committed at dispatch")
	if err != nil {
		a.warn("brief commit: commit-tree failed (is user.name/user.email set?); " + rel + " left uncommitted")
		return ""
	}
	c := bashSub(cOut)
	if nextGit(top, "update-ref", "refs/heads/main", c, old) != nil {
		a.warn("brief commit: main moved while the commit was being written; " + rel + " left uncommitted (nothing was undone)")
		return ""
	}
	if nextGit(top, "reset", "-q", "--", rel) != nil {
		a.warn("brief commit: committed, but 'git reset -- " + rel + "' failed on the real index; run it by hand")
	}
	short := c
	if len(short) > 8 {
		short = short[:8]
	}
	a.ok("brief committed at dispatch: " + short + " (" + rel + ")")
	return c
}

// nextBriefShape is the case pattern */briefs/*.md under the brain (a `*` crosses slashes).
func nextBriefShape(rel string) bool {
	i := strings.Index(rel, "/briefs/")
	return i >= 0 && strings.HasSuffix(rel[i+len("/briefs/"):], ".md") && len(rel[i+len("/briefs/"):]) >= 3
}

// nextPhysDir is `cd <dir> && pwd -P`.
func nextPhysDir(d string) (string, error) {
	abs, err := filepath.Abs(d)
	if err != nil {
		return "", err
	}
	if !isDir(abs) {
		return "", os.ErrNotExist
	}
	return filepath.EvalSymlinks(abs)
}
