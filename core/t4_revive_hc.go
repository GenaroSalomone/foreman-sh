package main

// hw revive of a run that lives in the herdr app (a run dispatched as an `hc run` terminal: <run>/hc exists).
//
// herdr-app-final-gaps G3: dispatch on hc now records session_resume, but revive spoke only to the herdr CLI (`agent list`,
// `tab create`), so on the app it refused ("herdr agent list did not answer") or, with a TUI herdr running, looked at the wrong
// agents. The same recorded facts (cwd, env, session resume) are repeated in a new `hc run` terminal instead of a tab.
//
// shortcut: claude only (the other vendors' resume flags are unmeasured on hc). A run launched in the sandbox is armed again
// and its shim goes first on the new terminal's PATH, as dispatch does on hc.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// reviveReopen is the end of a revive of a run that had reported: the reopened marker replaces the done marker.
func (a *app) reviveReopen(rundir, marker string) {
	if !isFile(marker) { // # MUTATION-ANCHOR: 863-G09
		return
	}
	if f, err := os.Create(reviveReopenedMarker(rundir)); err == nil {
		f.Close()
	}
	// THE DONE MARKER GOES WITH THE REOPEN. channel-send's --report-state refuses EVERY report while it stands ("task
	// state already has a done marker", not even the blocked-pane bypass), so a reopened run could not report at all
	// and the executor moved the marker by hand (2026-11-04). The earlier report stays on record — it is in the
	// receipt and in the ledger — and the reopened marker is what says this task is open again.
	os.Remove(marker)
	a.receiptInto(rundir, "reopened", "yes", "the run had "+filepath.Base(marker)+" and hw revive gave it a live pane; its done marker was cleared so the next done-invoker can deliver (the earlier report stays in the receipt and the ledger)")
	a.ok("run marked reopened — its earlier report stands, a new done-invoker is expected (no --retask), and hw status shows it as reopened")
}

// reviveOnHC is reviveCmd's tail for an app run: nothing is created before the refusals, as on herdr.
func (a *app) reviveOnHC(st *reviveRunState, sid, vendor, cmdArgs string, envShown []string, marker string) {
	proj, task, rundir, cwd := st.proj, st.task, st.rundir, st.cwd
	if vendor != "claude" {
		// every vendor is LAUNCHED on the app since hc-dispatch-every-vendor; what is claude-only is the resume, and this says why
		a.reviveDie("this run lives in the herdr app, and hw revive resumes only claude there: the resumed session is proved by reading claude's <config>/sessions/<pid>.json for the new terminal, and " + vendor + " has no such read-back (pi and codex record no session id on the app; opencode's comes from its own server, and resuming it in an app terminal is unmeasured). session_vendor is " + vendor + ". Start it again with `hw " + proj + " " + task + "`: the worktree is intact")
	}
	// One task, one terminal. AN UNREAD LIST IS NOT AN EMPTY ONE.
	raw, ok := hcOut(20*time.Second, "ls", "--json", "--label", "hw_project="+proj, "--label", "hw_task="+task)
	v, parsed := bjParse(brTrimNL(raw))
	list, isList := bjGet(v, "terminals").([]any)
	if !ok || !parsed || !isList {
		a.reviveDie("could not read whether " + proj + ":" + task + " is already live — `hc ls` did not answer. Refusing rather than resuming a session twice.")
	}
	var addrs []string
	for _, e := range list {
		if m, isMap := e.(jmap); isMap && hcStr(m, "address") != "" {
			addrs = append(addrs, hcStr(m, "address"))
		}
	}
	// the task's dev-server terminals carry its labels too, and are not a session
	agents, readable := hcNotDev(addrs, func(addr string) (string, bool) { return hcOut(20*time.Second, "get", addr, "--json") })
	if !readable {
		a.reviveDie("could not read whether " + proj + ":" + task + " is already live — `hc get` did not answer. Refusing rather than resuming a session twice.")
	}
	for _, addr := range agents {
		a.reviveDie("session " + sid + " of run " + filepath.Base(rundir) + " is still live in " + addr + " — a live session is never resumed twice")
	}
	shimDir := ""
	if exitStatus(a.reviveSandboxArmed(proj, task)) != 1 { // FAIL CLOSED: anything but a clean "never armed" is armed again
		shim, armed := a.reviveSandboxShim(st, vendor, rundir)
		if !armed || shim == "" {
			a.reviveDie("this run was launched in the sandbox and it could not be armed again (see the warning above); hw revive will not start it unconfined")
		}
		shimDir = shim
		a.receiptInto(rundir, "sandbox", filepath.Dir(shim)+"/profile.sb", "hw-sandbox arm ("+vendor+"); shim "+shim+" first on the PATH of the `hc run` terminal")
	}
	st.agentArgs = cmdArgs
	a.reviveResolvePermissions(st)
	a.reviveFrameworkArgs(st, vendor, cmdArgs, rundir)

	args := []string{"run", "--bg", "--json", "--no-parent", "--no-notify", "--name", task, "--cwd", cwd,
		"--label", "hw_project=" + proj, "--label", "hw_task=" + task, "--label", "hw_run=" + filepath.Base(rundir),
		"--label", "herdr.space=" + a.c.t.get(proj, "space"),
		"--", "env", "-u", "HERDR_ENV", "-u", "HERDR_PANE_ID", "-u", "HERDR_SOCKET_PATH", "-u", "HERDR_TAB_ID", "-u", "HERDR_WORKSPACE_ID",
		"-u", "HW_INVOKER_PANE", "-u", "CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1", "CLAUDE_CODE_BASH_EDIT_DIFF=0"}
	args = append(args, envShown...)
	args = append(args, hcSandboxPathWord(shimDir, hcPathWord(a.reviveBinDir(), a.getenv("PATH"))), "claude")
	if st.explorerArm { // an hc terminal has no shell to type the launch function into: the JSON is a word of the argv
		if json := a.explorerFor(proj).claudeJSON(); json != "" {
			args = append(args, "--agents", json)
		}
	}
	args = append(args, strings.Fields(st.agentArgs)...)
	out, ok := hcOut(60*time.Second, args...)
	rv, _ := bjParse(brTrimNL(out))
	addr := ""
	if m, isMap := rv.(jmap); isMap {
		addr = hcStr(m, "address")
	}
	if !ok || addr == "" {
		a.reviveDie("`hc run` did not start " + vendor + " for " + proj + ":" + task + ": " + reviveHead(out, 3))
	}
	if err := os.WriteFile(rundir+"/hc", []byte(proj+" "+task+" "+addr+"\n"), 0o666); err != nil { // the newest record is the one `hw done` reads
		a.err.Write([]byte("hw: " + rundir + "/hc: " + reviveErrno(err) + "\n"))
		panic(exitNow{1})
	}
	// the address and the core that holds it travel as a pair (dispatch writes both): a revive in another core re-pairs them
	if id := hcCoreID(a.getenv); id != "" {
		os.WriteFile(rundir+"/hc-core", []byte(id+"\n"), 0o666)
	} else {
		os.Remove(rundir + "/hc-core")
	}
	a.receiptInto(rundir, "revived_pane", addr, "hc run --json address")
	if shimDir != "" && !hcBrokerAlive(filepath.Dir(shimDir), lSleep) { // the heartbeat is the proof it runs in the sandbox, as on a pane
		hcOut(20*time.Second, "kill", addr)
		a.receiptInto(rundir, "revived_agent", "failed", "no sandbox broker heartbeat in "+filepath.Dir(shimDir))
		a.reviveDie("sandbox: " + vendor + " started in " + addr + " but the broker never came up (" + filepath.Dir(shimDir) + "/broker.alive), so it may have been running UNCONFINED: " + addr + " was killed (`hc kill`).")
	}
	// proved by the terminal's own `hc get`, as dispatch proves a brief: live, and the session claude wrote for its pid
	var got string
	live := false
	budget, _ := strconv.Atoi(orStr(a.getenv("HW_HC_PROOF_MS"), "30000"))
	for waited := 0; ; waited += 1000 {
		info, read := hcOut(20*time.Second, "get", addr, "--json")
		if m, isMap := func() (jmap, bool) { x, p := bjParse(brTrimNL(info)); mm, k := x.(jmap); return mm, p && k }(); read && isMap && hcStr(m, "state") == "live" {
			live = true
			got = claudeSessionOf(bjRaw(m["pid"]), hcStr(m, "cwd"), a.reviveConfigDir(envShown), a.getenv("CLAUDE_CONFIG_DIR"), a.getenv("HOME")+"/.claude")
			if got == "" && shimDir != "" {
				got = claudeSessionSandboxed(bjRaw(m["pid"]), hcStr(m, "cwd"), a.reviveConfigDir(envShown), a.getenv("CLAUDE_CONFIG_DIR"), a.getenv("HOME")+"/.claude")
			}
			if got != "" {
				break
			}
		}
		if waited >= budget {
			break
		}
		time.Sleep(time.Second)
	}
	if !live {
		a.receiptInto(rundir, "revived_agent", "failed", "hc get "+addr+" --json")
		a.reviveDie(addr + " exists with the recorded environment, but `hc get " + addr + "` does not show it live: read it with `hc read " + addr + "`. It is not closed.")
	}
	a.receiptInto(rundir, "revived_agent", "yes", "hc get "+addr+" --json: state=live")
	switch {
	case sid != "" && got != "" && got != sid:
		a.warn("the agent came up with session " + got + ", not the recorded " + sid + " — reattachment is NOT claimed; read the terminal")
		a.receiptInto(rundir, "revived_session", got+" (recorded: "+sid+") — MISMATCH", "<config>/sessions/<pid>.json of the pid `hc get "+addr+"` names")
	case got != "":
		a.receiptInto(rundir, "revived_session", got, "<config>/sessions/<pid>.json of the pid `hc get "+addr+"` names")
	}
	a.reviveReopen(rundir, marker)
	a.ok("revived " + proj + ":" + task + " in " + addr + " (herdr-core) — " + vendor + " " + st.agentArgs + " in " + cwd)
	a.info("nothing was rebuilt: worktree, ports and database are whatever the directory still holds")
}

// reviveConfigDir is the CLAUDE_CONFIG_DIR the recorded env carries ("" for the default account).
func (a *app) reviveConfigDir(envShown []string) string {
	for _, rec := range envShown {
		if strings.HasPrefix(rec, "CLAUDE_CONFIG_DIR=") {
			return strings.TrimPrefix(rec, "CLAUDE_CONFIG_DIR=")
		}
	}
	return ""
}
