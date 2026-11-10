package main

// hw revive of a run that lives in the herdr app (a run dispatched as an `hc run` terminal: <run>/hc exists).
//
// herdr-app-final-gaps G3: dispatch on hc now records session_resume, but revive spoke only to the herdr CLI (`agent list`,
// `tab create`), so on the app it refused ("herdr agent list did not answer") or, with a TUI herdr running, looked at the wrong
// agents. The same recorded facts (cwd, env, session resume) are repeated in a new `hc run` terminal instead of a tab.
//
// shortcut: claude only, as dispatch on hc is; a sandboxed run is refused (dispatch refuses it on hc too).

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
		a.reviveDie("this run lives in the herdr app, where only claude executors are launched; session_vendor is " + vendor)
	}
	// One task, one terminal. AN UNREAD LIST IS NOT AN EMPTY ONE.
	raw, ok := hcOut(20*time.Second, "ls", "--json", "--label", "hw_project="+proj, "--label", "hw_task="+task)
	v, parsed := bjParse(brTrimNL(raw))
	list, isList := bjGet(v, "terminals").([]any)
	if !ok || !parsed || !isList {
		a.reviveDie("could not read whether " + proj + ":" + task + " is already live — `hc ls` did not answer. Refusing rather than resuming a session twice.")
	}
	for _, e := range list {
		if m, isMap := e.(jmap); isMap && hcStr(m, "address") != "" {
			a.reviveDie("session " + sid + " of run " + filepath.Base(rundir) + " is still live in " + hcStr(m, "address") + " — a live session is never resumed twice")
		}
	}
	if exitStatus(a.reviveSandboxArmed(proj, task)) != 1 { // FAIL CLOSED: anything but a clean "never armed"
		a.reviveDie("this run was launched in the sandbox, and an app terminal cannot be armed with it; hw revive will not start it unconfined")
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
	args = append(args, hcPathWord(a.reviveBinDir(), a.getenv("PATH")), "claude")
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
	// proved by the terminal's own `hc get`, as dispatch proves a brief: live, and the session claude wrote for its pid
	var got string
	live := false
	budget, _ := strconv.Atoi(orStr(a.getenv("HW_HC_PROOF_MS"), "30000"))
	for waited := 0; ; waited += 1000 {
		info, read := hcOut(20*time.Second, "get", addr, "--json")
		if m, isMap := func() (jmap, bool) { x, p := bjParse(brTrimNL(info)); mm, k := x.(jmap); return mm, p && k }(); read && isMap && hcStr(m, "state") == "live" {
			live = true
			got = claudeSessionOf(bjRaw(m["pid"]), hcStr(m, "cwd"), a.reviveConfigDir(envShown), a.getenv("CLAUDE_CONFIG_DIR"), a.getenv("HOME")+"/.claude")
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
