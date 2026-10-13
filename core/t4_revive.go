package main

// hw revive <project> <task> [--run <id>] [--dry-run] — a port of cmd_revive (bin/hw:7985-8328) and of the
// argument block that calls it (_usage_revive and `if [ "${1:-}" = revive ]`, bin/hw:12106-12137), byte for
// byte: the same words, the same refusals in the same order, and the same calls to herdr, in the same order.
//
// What it does: repeats a run's RECORDED cwd, env and session resume in a new tab, refusing whatever was not
// recorded. A fact that is not on disk is a refusal, never an inference. It touches the run directory (the
// `tab` file, receipt lines, the `reopened` marker) and creates one tab through herdr.
//
// Every top-level name of the verb starts with `revive` (the packages of the four writers of train T4 merge).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const reviveUsage = "usage: hw revive <project> <task> [--run <id>] [--dry-run]  (hw revive --help)"

const reviveHelp = `hw revive <project> <task> [--run <id>] [--dry-run]

  Repeat a run's recorded cwd, env and session resume in a new tab; refuses
  anything not recorded (cwd gone, no session, no env).

  --run <id>   revive that run (a directory name under the task's .hw/, e.g.
               20261006-153531-98770) instead of the task's latest one. Refused
               for an unknown id (the runs that exist are named) and for a run
               whose session is still live in a pane (the pane is named).
  --dry-run    print what would be repeated; the output says which run was
               chosen and whether --run chose it or it was defaulted to the latest.
`

// reviveRun is the state cmd_revive keeps in its locals and in the globals its helpers read.
type reviveRunState struct {
	proj, task   string
	rundir       string
	cwd          string
	agentArgs    string // AGENT_ARGS
	sandbox      bool   // SANDBOX
	sandboxRun   string // _SANDBOX_RUNDIR
	sandboxArmed string // _SANDBOX_ARMED
	permissions  string // PERMISSIONS
	permSrc      string
}

// reviveDie is _revive_die: the refusal, on stderr, and status 1.
func (a *app) reviveDie(msg string) {
	a.err.Write([]byte(a.cErr + "hw revive REFUSED:" + a.c0 + " " + msg + "\n"))
	panic(exitNow{1})
}

func (a *app) cmdRevive() int {
	// bin/hw runs `need herdr; need git; need jq` before anything
	for _, c := range []string{"herdr", "git", "jq"} {
		if _, err := exec.LookPath(c); err != nil {
			a.die("missing required command: " + c)
		}
	}
	args := a.args
	dry, wantRun := false, ""
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			a.printf("%s", reviveHelp)
			return 0
		}
	}
	if len(args) < 2 {
		a.die(reviveUsage)
	}
	proj, task := args[0], args[1]
	rest := args[2:]
	for len(rest) > 0 {
		switch {
		case rest[0] == "--dry-run":
			dry = true
		case rest[0] == "--run":
			if len(rest) < 2 || rest[1] == "" {
				a.die("--run needs a run id. " + reviveUsage)
			}
			wantRun = rest[1]
			rest = rest[1:]
		case strings.HasPrefix(rest[0], "--run="):
			wantRun = strings.TrimPrefix(rest[0], "--run=")
			if wantRun == "" {
				a.die("--run needs a run id. " + reviveUsage)
			}
		default:
			a.die(reviveUsage)
		}
		rest = rest[1:]
	}
	a.reviveCmd(a.c.t.canonProject(proj), task, dry, wantRun)
	return 0
}

// reviveTaskRundirs is _task_rundirs: the run directories of a task, newest first. Unlike the work tree the
// verb resolves first, it has no fallback to the work directory.
func (a *app) reviveTaskRundirs(proj, task string) []string {
	base := a.c.lanePath(proj, "worktree", task)
	if base == "" {
		base = a.c.work + "/" + proj + "/" + task
	}
	if !isDir(base + "/.hw") {
		return nil
	}
	var out []string
	for _, r := range RunIDs(base + "/.hw") {
		out = append(out, base+"/.hw/"+r)
	}
	return out
}

// reviveDoneMarker is _run_done_marker: the done marker of the run's CURRENT task.
func reviveDoneMarker(rundir string) string {
	seq := ""
	if readableFile(rundir + "/task") {
		if b, err := os.ReadFile(rundir + "/task"); err == nil {
			seq = digitsOnly(string(b))
		}
	}
	switch seq {
	case "", "0", "1":
		return rundir + "/done"
	}
	return rundir + "/t" + seq + "/done"
}

// reviveReopenedMarker is _run_reopened_marker: beside the done marker.
func reviveReopenedMarker(rundir string) string {
	return strings.TrimSuffix(reviveDoneMarker(rundir), "/done") + "/reopened"
}

func (a *app) reviveCmd(proj, task string, dry bool, wantRun string) {
	chosen := "defaulted: the task's latest run"
	if !a.c.t.known(proj) {
		a.die("unknown project: " + proj + " (expected: " + a.c.t.namesBar() + ")")
	}
	wt := a.c.lanePath(proj, "worktree", task)
	if wt == "" {
		wt = a.c.work + "/" + proj + "/" + task
	}
	// A --worktree none task in a repo lane recorded itself in the work directory.
	if !isDir(wt + "/.hw") {
		wt = a.c.work + "/" + proj + "/" + task // # MUTATION-ANCHOR: 863-G01
	}
	if !isDir(wt + "/.hw") {
		a.reviveDie("no hw runs for " + proj + ":" + task + " (looked in " + wt + "/.hw) — nothing recorded, nothing to repeat")
	}
	runs := a.reviveTaskRundirs(proj, task)
	if len(runs) == 0 {
		a.reviveDie("no run directory under " + wt + "/.hw")
	}
	rundir := runs[0]
	if wantRun != "" { // # MUTATION-ANCHOR: 863-G02
		found := ""
		for _, r := range runs {
			if filepath.Base(r) == wantRun {
				found = r
			}
		}
		if found == "" {
			var names strings.Builder
			for _, r := range runs {
				names.WriteString(filepath.Base(r) + " ")
			}
			a.reviveDie("no run '" + wantRun + "' for " + proj + ":" + task + " — the runs that exist: " + names.String())
		}
		rundir = found
		chosen = "chosen: --run " + wantRun
	}
	receipt := rundir + "/receipt.jsonl"
	if !readableFile(receipt) {
		a.reviveDie("the selected run " + filepath.Base(rundir) + " has no receipt.jsonl, so no session was ever recorded for it. Receipts start with the run that introduced them.")
	}

	// the three facts of the receipt; a jq that fails ends bash, silently, with its own status (pipefail + set -e)
	sid, rc := reviveReceiptLast(receipt, "session_id")
	if rc != 0 {
		panic(exitNow{rc})
	}
	vendor, rc := reviveReceiptLast(receipt, "session_vendor")
	if rc != 0 {
		panic(exitNow{rc})
	}
	resume, rc := reviveReceiptLast(receipt, "session_resume")
	if rc != 0 {
		panic(exitNow{rc})
	}
	if resume == "" { // # MUTATION-ANCHOR: 863-G03
		a.reviveDie("run " + filepath.Base(rundir) + " recorded no session_resume — its executor published no session id in time (see the session_id receipt line, if any), so the conversation is not addressable and hw will not guess at it by title. 58 of setup's 143 receipts are in this state; this is one of them.")
	}
	switch vendor {
	case "claude", "opencode", "codex":
	case "pi":
		// bash revives a pi session; hw-core does not know pi yet, and refusing says so where a half-answer would not
		a.reviveDie("session_vendor is 'pi', which hw-core does not revive yet: run it with HW_IMPL=bash")
	default:
		shown := vendor
		if shown == "" {
			shown = "absent"
		}
		a.reviveDie("session_vendor is '" + shown + "', not one of claude|opencode|codex|pi — no agent kind is claimed for it")
	}
	// Revival bypasses the normal launch router, so replay the recorded framework contract before it can
	// create a tab or start the recorded vendor.
	framework := reviveFramework(rundir)
	a.reviveValidateSDD(vendor, framework, wt)
	// A root dispatch can predate an explicit re-task that changed this live surface: measure separately so a
	// recorded `none` cannot mask the revived pane's incompatible effective framework.
	a.reviveValidateSDD(vendor, "", wt)

	// FACT 1 AND FACT 3 FROM ONE RECORD: `cd <dir> && <vendor> <args>[   # comment]`
	if !strings.HasPrefix(resume, "cd ") || !strings.Contains(resume[3:], " && ") {
		a.reviveDie("session_resume is not in the shape hw writes (cd <dir> && <vendor> …): " + resume)
	}
	restS := resume[3:]
	i := strings.Index(restS, " && ")
	cwd := restS[:i]
	restS = restS[i+len(" && "):]
	if j := strings.Index(restS, "   #"); j >= 0 { // the opencode line carries a read-only hint after three spaces
		restS = restS[:j]
	}
	cmdBin, cmdArgs := restS, restS
	if k := strings.IndexByte(restS, ' '); k >= 0 {
		cmdBin, cmdArgs = restS[:k], restS[k+1:]
	}
	if cmdBin != vendor { // # MUTATION-ANCHOR: 863-G22
		a.reviveDie("session_resume runs '" + cmdBin + "' but session_vendor says '" + vendor + "' — two recorded facts disagree, and hw will not pick one")
	}
	if cmdArgs == restS {
		a.reviveDie("session_resume has no arguments after '" + cmdBin + "': " + resume)
	}
	gentleArgs := ""
	if vendor == "claude" {
		g, ok := a.reviveGentleArgs(rundir)
		if !ok {
			a.reviveDie(g)
		}
		gentleArgs = g
	}
	if cwd == "" {
		a.reviveDie("session_resume has an empty cd target: " + resume)
	}
	if !isDir(cwd) { // # MUTATION-ANCHOR: 863-G21
		have := sid
		if have == "" {
			have = "it"
		}
		a.reviveDie("the session survives — " + vendor + " keeps " + have + " — but its directory does not: " + cwd + " is gone. That is what `git worktree remove` produces, and hw revive does not rebuild a worktree: a rebuilt directory would give the session a tree it never worked in.")
	}

	// FACT 2. Refused when absent; refused when a line is outside the grammar.
	if !readableFile(rundir + "/env") {
		a.reviveDie("no " + rundir + "/env — this run's environment was never persisted, and hw will not reassemble one from flags")
	}
	envKVs, envErr := a.reviveEnvRecords(rundir + "/env")
	if envErr != "" {
		a.reviveDie(rundir + "/env is not in the grammar _write_run_env writes: " + envErr)
	}
	var envShown []string
	var envArgs []string
	for _, kv := range envKVs {
		rec := kv.Key + "=" + kv.Value
		envArgs = append(envArgs, "--env", rec)
		envShown = append(envShown, rec)
	}
	if len(envArgs) == 0 { // # MUTATION-ANCHOR: 863-G20
		a.reviveDie(rundir + "/env holds no variables")
	}
	// The env file's own idea of the work directory must be the receipt's cwd.
	envWorkdir := ""
	for _, rec := range envShown {
		if strings.HasPrefix(rec, "HW_WORKDIR=") {
			envWorkdir = strings.TrimPrefix(rec, "HW_WORKDIR=")
		}
	}
	if envWorkdir != "" && envWorkdir != cwd { // # MUTATION-ANCHOR: 863-G19
		a.reviveDie("the env file says HW_WORKDIR=" + envWorkdir + " and the receipt's resume says cd " + cwd + " — two recorded facts disagree, and hw will not pick one")
	}

	a.printf("\n  %srevive %s:%s%s  run %s  (%s)\n", a.cB, proj, task, a.c0, filepath.Base(rundir), chosen)
	a.printf("    cwd      %s\n", cwd)
	a.printf("    vendor   %s\n", vendor)
	shownSid := sid
	if shownSid == "" {
		shownSid = "<not in receipt>"
	}
	a.printf("    session  %s\n", shownSid)
	a.printf("    resume   %s %s\n", cmdBin, cmdArgs)
	if gentleArgs != "" {
		a.printf("    gentle   %s   (recomputed from the current gentle-home)\n", gentleArgs)
	}
	a.printf("    env      %d variables from %s\n", len(envShown), rundir+"/env")
	for _, rec := range envShown {
		a.printf("      %s\n", rec)
	}
	a.printf("\n")

	st := &reviveRunState{proj: proj, task: task, rundir: rundir, cwd: cwd}
	marker := reviveDoneMarker(rundir)
	if dry {
		if isFile(marker) {
			a.info("would mark the run reopened (" + reviveReopenedMarker(rundir) + "): it reported done, so a revived executor reports again without --retask")
		} else {
			a.info("the run never reported, so nothing to reopen")
		}
		if vendor == "claude" {
			a.reviveResolvePermissions(st)
			if reviveHasWord(cmdArgs, "--dangerously-skip-permissions") {
				a.info("the recorded resume already carries --dangerously-skip-permissions")
			} else if st.permissions == "ask" {
				a.info("permissions: ask (" + st.permSrc + ") — would NOT inject --dangerously-skip-permissions")
			} else {
				a.info("would inject --dangerously-skip-permissions (today's machine policy, not part of the recorded resume identity): " + cmdBin + " " + cmdArgs + " --dangerously-skip-permissions")
			}
		}
		a.ok("dry run — nothing created, no agent started")
		return
	}

	// One directory, one agent. AND AN UNREAD LIST IS NOT AN EMPTY ONE: `_wt_occupants` answers with three values.
	occ := (&occupantsReader{h: herdrClient{getenv: a.getenv}, win: windowsPaths(a.getenv)}).of(cwd)
	if !occ.observed {
		a.reviveDie("could not read who is already working in " + cwd + " — " + occ.why + ". Refusing rather than reviving into a directory that may already have an agent: one directory with two agents is the shared-tree collision the worktree model exists to prevent, and it leaves no trace in any diff.")
	}
	if sid != "" { // # MUTATION-ANCHOR: 863-G04
		sp, ok := a.reviveSessionPanes(sid)
		if !ok {
			a.reviveDie("could not read whether session " + sid + " is live — " + sp + ". Refusing rather than resuming a session twice.")
		}
		if sp != "" {
			a.reviveDie("session " + sid + " of run " + filepath.Base(rundir) + " is still live in " + strings.ReplaceAll(sp, "\n", " ") + " — a live session is never resumed twice")
		}
	}
	if occ.list != "" { // # MUTATION-ANCHOR: 863-G05
		a.reviveDie("an agent is already running in " + cwd + " (" + strings.ReplaceAll(occ.list, "\n", " ") + ") — revive would give one directory two agents")
	}

	// A run launched in the sandbox comes back in it. FAIL CLOSED: anything but a clean "never armed" revives it armed.
	sbxRC := 0
	{
		cmd := exec.Command(a.reviveBinDir()+"/hw-sandbox", "armed", proj, task)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		sbxRC = exitStatus(cmd.Run())
	}
	if sbxRC != 1 { // # MUTATION-ANCHOR: 863-G07
		st.sandbox, st.sandboxRun = true, rundir
	}
	pw, ok := a.reviveProjectWorkspace(proj, cwd)
	if !ok {
		a.reviveDie("could not find or create the workspace for " + proj)
	}
	targs := append([]string{"tab", "create", "--workspace", pw, "--cwd", cwd, "--label", proj + ":" + task, "--no-focus"}, envArgs...)
	raw, trc := herdrClient{getenv: a.getenv}.capture(targs...)
	raw = bashSub(raw)
	if trc != 0 {
		a.reviveDie("herdr tab create failed: " + reviveHead(raw, 3))
	}
	tab := reviveFirstTruthy(raw, []string{"result", "tab", "tab_id"}, []string{"result", "root_pane", "tab_id"}, []string{"result", "tab_id"})
	pane := reviveFirstTruthy(raw, []string{"result", "root_pane", "pane_id"}, []string{"result", "pane", "pane_id"})
	if tab == "" || pane == "" {
		a.reviveDie("herdr created no usable tab/pane: " + reviveHead(raw, 3))
	}
	// THE TAB RECORD IS REWRITTEN, not reconstructed: the old tab is the one `hw done` closed.
	if err := os.WriteFile(rundir+"/tab", []byte(tab+"\n"), 0o666); err != nil { // # MUTATION-ANCHOR: 863-G24
		a.err.Write([]byte("hw: " + rundir + "/tab: " + reviveErrno(err) + "\n"))
		panic(exitNow{1})
	}
	a.receiptInto(rundir, "revived_tab", tab, "herdr tab create --cwd "+cwd+" --label "+proj+":"+task+" with "+itoa(len(envShown))+" --env from "+rundir+"/env")
	a.receiptInto(rundir, "revived_pane", pane, "herdr tab create reply .result.root_pane.pane_id")

	st.agentArgs = cmdArgs
	if gentleArgs != "" {
		st.agentArgs += " " + gentleArgs
	}
	a.reviveResolvePermissions(st)
	a.reviveFrameworkArgs(st, vendor, cmdArgs, rundir)
	if !a.reviveAgentStart(st, pane, task, vendor) {
		// _agent_start giving up is not proof the pane is dead: ask once more before recommending a close
		recheck, _ := herdrClient{getenv: a.getenv}.stdout("agent", "get", pane)
		if a.reviveAgentIdle(recheck, pane, task, vendor) { // # MUTATION-ANCHOR: 863-G08
			a.receiptInto(rundir, "revived_agent", "yes (recovered after a startup-refusal recheck)", "herdr agent get "+pane+", interactive_ready=true")
			a.warn("hw's own start budget gave up, but " + pane + " is alive and idle — continuing rather than closing a working pane")
		} else {
			a.receiptInto(rundir, "revived_agent", "failed", "herdr agent start "+task+" --kind "+vendor+" --pane "+pane+" -- "+st.agentArgs)
			a.reviveDie("tab " + tab + " (pane " + pane + ") exists with the recorded environment, but `" + vendor + " " + st.agentArgs + "` did not come up in it (rechecked with herdr agent get, still not ready). The tab is left for you to read; close it with: herdr tab close " + tab)
		}
	} else {
		a.receiptInto(rundir, "revived_agent", "yes", "herdr agent start "+task+" --kind "+vendor+" --pane "+pane+" -- "+st.agentArgs+", interactive_ready=true")
	}
	gotOut, _ := herdrClient{getenv: a.getenv}.stdout("agent", "get", pane)
	got := reviveFirstTruthy(gotOut, []string{"result", "agent", "agent_session", "value"})
	if sid != "" && got != "" && got != sid { // # MUTATION-ANCHOR: 863-G23
		a.warn("the agent came up with session " + got + ", not the recorded " + sid + " — reattachment is NOT claimed; read the pane")
		a.receiptInto(rundir, "revived_session", got+" (recorded: "+sid+") — MISMATCH", "herdr agent get "+pane+" | jq .result.agent.agent_session.value")
	} else if got != "" {
		a.receiptInto(rundir, "revived_session", got, "herdr agent get "+pane+" | jq .result.agent.agent_session.value")
	}

	if isFile(marker) { // # MUTATION-ANCHOR: 863-G09
		if f, err := os.Create(reviveReopenedMarker(rundir)); err == nil {
			f.Close()
		}
		a.receiptInto(rundir, "reopened", "yes", "the run had "+filepath.Base(marker)+" and hw revive gave it a live pane; cleared by the next done-invoker or hw done")
		a.ok("run marked reopened — its earlier report stands, a new done-invoker is expected (no --retask), and hw status shows it as reopened")
	}
	a.ok("revived " + proj + ":" + task + " in tab " + tab + ", pane " + pane + " — " + vendor + " " + st.agentArgs + " in " + cwd)
	a.info("nothing was rebuilt: worktree, ports and database are whatever the directory still holds")
}

// reviveErrno is the strerror text bash prints for a failed redirect.
func reviveErrno(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		err = pe.Err
	}
	s := err.Error()
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// reviveSessionPanes is _revive_session_panes: "<pane> (<agent>, <status>)" for each herdr agent whose
// agent_session.value is sid, or why the list could not be read (an unread list is not an empty one).
func (a *app) reviveSessionPanes(sid string) (string, bool) {
	raw, ok := herdrClient{getenv: a.getenv}.stdout("agent", "list")
	vals, perr := parseStream([]byte(bashSub(raw)))
	if !ok || perr != nil || len(vals) == 0 || !reviveJqTrue(bashSub(raw), func(v *jval) (bool, bool) {
		r, k := reviveJqPath(v, "result", "type")
		return k && reviveStrEq(r, "agent_list"), k
	}) {
		return "herdr agent list did not answer", false
	}
	var lines []string
	for _, v := range vals {
		res, k := reviveJqPath(v, "result", "agents")
		if !k {
			break
		}
		var items []*jval
		switch res.kind {
		case vArr:
			items = res.arr
		case vObj:
			for _, m := range res.obj {
				items = append(items, m.v)
			}
		case vNull:
		default:
			break
		}
		for _, ag := range items {
			val, k := reviveJqPath(ag, "agent_session", "value")
			if !k {
				break
			}
			if val.kind != vStr || val.s != sid { // (.agent_session.value // "") == $s
				continue
			}
			pid, _ := reviveJqPath(ag, "pane_id")
			ag1, _ := reviveJqPath(ag, "agent")
			as, _ := reviveJqPath(ag, "agent_status")
			lines = append(lines, reviveText(pid)+" ("+reviveOr(ag1, "?")+", "+reviveOr(as, "?")+")")
		}
	}
	return strings.Join(lines, "\n"), true
}

// reviveText is "\(x)" in a jq string interpolation: a string as itself, anything else as JSON.
func reviveText(v *jval) string {
	if v == nil {
		return "null"
	}
	if v.kind == vStr {
		return v.s
	}
	return v.toJSON()
}

// reviveOr is "\(x // def)".
func reviveOr(v *jval, def string) string {
	if v == nil || !v.truthy() {
		return def
	}
	return reviveText(v)
}
