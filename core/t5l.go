package main

// Train T5b: the spine of the launch. bin/hw "BEFORE anything is built" … "esac" for a dispatch that is not a dry
// run: the route forecast, the manifest, the idempotence check, the repoless arm and the rollback. The lanes with
// a worktree are t5l_lanes.go; everything the arms share (the space, the agent, the brief) is the other t5l_*.go.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// routeForecastCheck is `_route_forecast_check`: a launch compares the route it resolved with the one the dry
// run printed, and refuses when they differ.
func (d *dsp) routeForecastCheck(proj, task, routeNow, detailNow string) {
	f := d.routeForecastPath(proj, task)
	data, err := os.ReadFile(f)
	if err != nil {
		return
	}
	field := func(k string) string {
		for _, ln := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(ln, k+"=") {
				return ln[len(k)+1:]
			}
		}
		return ""
	}
	recorded, recDetail := field("route"), field("detail")
	var recAt string
	for _, r := range field("at") {
		if r >= '0' && r <= '9' {
			recAt += string(r)
		}
	}
	if !d.readonly {
		os.Remove(f)
	}
	if recorded != "fresh" && recorded != "reuse" {
		return
	}
	if recAt == "" {
		d.info("a route forecast for " + proj + ":" + task + " had no readable timestamp; it was discarded rather than trusted")
		return
	}
	at, _ := strconv.ParseInt(recAt, 10, 64)
	age := time.Now().Unix() - at
	ttl, _ := strconv.ParseInt(d.routeForecastTTL, 10, 64)
	if age >= ttl {
		d.info("a route forecast for " + proj + ":" + task + " existed but is " + strconv.FormatInt(age, 10) + "s old (limit " + d.routeForecastTTL + "s); it was discarded, and this launch is not bound by it")
		return
	}
	if recorded == routeNow { // # MUTATION-ANCHOR: 847-L01
		d.info("route matches the dry run " + strconv.FormatInt(age, 10) + "s ago: " + recorded)
		return
	}
	d.warn("the dry run " + strconv.FormatInt(age, 10) + "s ago resolved: route=" + recorded + " — " + recDetail)
	d.warn("this launch resolves:            route=" + routeNow + " — " + detailNow)
	if routeNow == "reuse" {
		d.die("ROUTE CHANGED since your dry run. A reusable pane appeared in the meantime, so this launch would hand the brief to a LIVE pane instead of building the executor the dry run described. NOTHING was built and NOTHING was sent. Decide again: re-read it with --dry-run, or pass --fresh to bind the fresh route this command originally described.")
	}
	d.die("ROUTE CHANGED since your dry run. The pane the dry run would have reused is no longer reusable (its chaining lease expired, it closed, or it was re-tasked), so this launch would build a whole new executor instead. NOTHING was built and NOTHING was sent. Decide again: re-read it with --dry-run. There is no flag that forces the reuse route — a pane can only be re-tasked while its lease is live, and that is the fact that changed.")
}

// launch is everything after the manifest on the fresh route. `d.run` calls it when the dispatch is not a dry run.
func (d *dsp) launch() {
	l := &lch{dsp: d}
	l.fs = d.sharedFs()
	l.label = d.proj + ":" + d.task
	l.branch = d.c.lanePath(d.proj, "branch", d.task)
	if !d.c.t.known(d.proj) {
		l.branch = "task/" + d.task
	}
	l.herdr(true, true, "", "notification", "show", "dispatch "+d.proj+":"+d.task, "--body", d.notifyBody)
	l.setEnv("HW_RUN", d.hwRun) // bash: `export HW_RUN` right after the manifest
	l.run()
}

// run is bin/hw from "# ── --here" to the end.
func (l *lch) run() {
	// ── 9. Idempotent: an existing space is focused, not rebuilt ──
	if l.tab {
		if existing := l.tabIDByLabel(l.label); existing != "" {
			if l.liveAgentPanesInTab(existing) == "0" {
				l.herdr(true, true, "", "tab", "focus", existing)
				l.die("tab " + l.label + " exists (" + existing + ") but holds NO agent pane — herdr reports no agent in any of its panes, so there is nothing to reuse and nothing was started. This is not a running task. Close it and dispatch again:  hw done " + l.proj + " " + l.task + "  &&  <this same hw command>")
			}
			l.herdr(true, true, "", "tab", "focus", existing)
			l.ok("tab " + l.label + " already exists (" + existing + ") — focused, nothing rebuilt (an agent is live in it)")
			return
		}
	} else if !l.here {
		if existing := l.wsIDByLabel(l.label); existing != "" {
			if l.liveAgentPanesInWorkspace(existing) == "0" {
				l.herdr(true, false, "", "workspace", "focus", existing)
				l.die("space " + l.label + " exists (" + existing + ") but holds NO agent pane — herdr reports no agent in any of its panes, so there is nothing to reuse and nothing was started. Close it and dispatch again:  hw done " + l.proj + " " + l.task + " --space  &&  <this same hw command>")
			}
			l.herdr(true, false, "", "workspace", "focus", existing)
			l.ok("space " + l.label + " already exists (" + existing + ") — focused, nothing rebuilt (an agent is live in it)")
			return
		}
	}
	l.off = l.offset(l.task)

	lane := l.proj
	if !l.wantWT {
		lane = "repoless"
	}
	if lane == "repoless" {
		l.repolessArm()
		return
	}
	l.buildLane()
}

// repolessArm is the `repoless)` arm of bin/hw's `case "$_LANE"`.
func (l *lch) repolessArm() {
	l.wt = filepath.Join(l.c.work, l.proj, l.task)
	l.step("1/3  work directory")
	if st, err := os.Stat(l.wt); err == nil && st.IsDir() {
		l.ok("reusing " + l.wt + " (" + strconv.Itoa(countFiles(l.wt)) + " files already there)")
	} else {
		l.fs.mkdirAll(l.wt)
		l.ok("created " + l.wt)
	}
	l.info("artifacts belong here, not in brain/ and not in any repo")
	if l.here {
		l.runHere(l.wt)
		return
	}
	l.trustDir(l.wt)
	l.applySdd(l.wt, l.sdd)
	l.step("2/3  space + layout")
	l.exportHwEnv(l.wt)
	if l.artifactsDirFor(l.proj, l.wt) != l.wt {
		l.ensureArtifactsDir(l.wt)
	}
	l.buildSpace(l.layoutDir() + "/repoless.json")
	l.pAgent = l.paneByLabel(l.layoutOut, "agent")
	l.pShell = l.paneByLabel(l.layoutOut, "shell")
	if l.pAgent == "" || l.pShell == "" {
		l.die("layout applied but a labelled pane is missing (agent=" + l.pAgent + " shell=" + l.pShell + ") — check " + l.layoutDir() + "/repoless.json")
	}
	l.ok("agent=" + l.pAgent + "  shell=" + l.pShell)
	l.step("3/3  agent")
	l.agentStep()
	l.printf("\n  %s%s%s  %s\n  no repo, no branch, no ports, no database\n\n", l.cB, l.label, l.c0, l.wt)
}

// layoutDir is $LAYOUT_DIR ("$HW_BIN_DIR/../layouts", unnormalised, as bin/hw builds it).
func (l *lch) layoutDir() string {
	return l.binDir() + "/../layouts"
}

func countFiles(dir string) int {
	n := 0
	filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err == nil && e.Type().IsRegular() {
			n++
		}
		return nil
	})
	return n
}

// launchRollback is `_launch_rollback "$main"`: remove only what THIS launch created.
func (l *lch) launchRollback(main string) {
	if !l.launchWTCreated {
		if l.launchWT != "" {
			l.info("worktree " + l.launchWT + " existed before this command — left untouched")
		}
		return
	}
	l.warn("rolling back what this launch created")
	removed := false
	if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", main, "worktree", "remove", "-f", "-f", l.launchWT}, mut: true, merge: true}); rc == 0 {
		l.ok("removed worktree " + l.launchWT)
		removed = true
	} else {
		l.warn("could not remove " + l.launchWT + " — remove it by hand:")
		l.warn("  git -C " + main + " worktree remove --force " + l.launchWT)
	}
	if l.launchBranchCreated && l.launchBranch != "" {
		if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", main, "branch", "-D", l.launchBranch}, mut: true, merge: true}); removed && rc == 0 {
			l.ok("deleted branch " + l.launchBranch)
		} else {
			l.warn("branch " + l.launchBranch + " was created by this launch and is still there:")
			l.warn("  git -C " + main + " branch -D " + l.launchBranch)
		}
	}
}

// writeEngramMcp is the claude arm of _engram_proxy_args that a dry run skips: the --mcp-config file the engram
// label proxy is launched from. The dry run (T5a) already chose the flag; this writes what it names.
func (d *dsp) writeEngramMcp() {
	if d.dispatchDry {
		return
	}
	const flag = "--mcp-config "
	if d.agent != "claude" || !strings.HasPrefix(d.engramProxyLine, flag) {
		return
	}
	file := strings.TrimPrefix(d.engramProxyLine, flag)
	label := d.engramLabel(d.proj)
	proxy := d.vdRoot() + "/bin/engram-label-proxy"
	eng := d.vdCommandV("engram")
	if eng == "" {
		eng = "engram"
	}
	q := func(v string) string { b, _ := json.Marshal(v); return string(b) }
	doc := "{\n  \"mcpServers\": {\n    \"engram\": {\n      \"command\": " + q(proxy) + ",\n      \"args\": [\n        \"--label\",\n        " +
		q(label) + ",\n        \"--\",\n        " + q(eng) + ",\n        \"mcp\",\n        \"--tools=agent\"\n      ]\n    }\n  }\n}\n"
	dir := file[:strings.LastIndexByte(file, '/')]
	if err := d.sharedFs().mkdirAll(dir); err != nil {
		d.die("could not write the engram proxy's --mcp-config at " + file)
	}
	if err := d.sharedFs().write(file, []byte(doc), 0o666); err != nil {
		d.die("could not write the engram proxy's --mcp-config at " + file)
	}
}

// sharedFs is the one file-op recorder of the dispatch: the engram --mcp-config is written before the launch begins
// (bash writes it in _engram_proxy_args), so the pre-launch and the launch record into the same plan.
func (d *dsp) sharedFs() *fsx {
	if d.fsShared == nil {
		d.fsShared = newFsx(d.getenv)
	}
	return d.fsShared
}
