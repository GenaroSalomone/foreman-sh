package main

// Area D — the lane builders: a worktree and its branch, the local config, ports, database, dependencies and the
// space of one lane (lanes/*.sh, sourced by bin/hw
// where its `case` arm was). OWNER: the D tracks; each lane in its own file (t5l_lane_<lane>.go).
// This file is D1's: the dispatcher (the `*)` arm of bin/hw's `case "$_LANE"`) and what every lane file shares
// (the lane getters, lane_checkout, _pick_port, the worktree/space steps the plain lanes repeat).

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type llnState struct {
	portRemoved map[string]bool // read-only: lock directories a stale-lock reap "removed"
}

// ── the lane getters (bin/hw _lane_*, bin/project-spaces.sh lane_*) ──

// laneCheckout is `lane_checkout "$1"`.
func (l *lch) laneCheckout(proj string) string { return l.c.checkout(proj) }

// laneWtDir is `_lane_wt_dir <project> <task>`.
func (l *lch) laneWtDir(proj, task string) string { return l.c.lanePath(proj, "worktree", task) }

// laneBranch is `_lane_branch <project> <task>`.
func (l *lch) laneBranch(proj, task string) string {
	if !l.c.t.known(proj) {
		return "task/" + task
	}
	return l.c.lanePath(proj, "branch", task)
}

// laneBaseRefPrefixOf is `_lane_base_ref_prefix <project>` (for the dispatch's own project: d.laneBaseRefPrefix()).
func (l *lch) laneBaseRefPrefixOf(proj string) string {
	if !l.c.t.known(proj) {
		return "origin/"
	}
	return l.c.t.get(proj, "base_ref_prefix")
}

// laneBaseOf is `_lane_base <project>`.
func (l *lch) laneBaseOf(proj string) string { return l.c.t.get(proj, "base") }

// laneAgentsRef is `_lane_agents_ref <project>`.
func (l *lch) laneAgentsRef(proj string) string {
	return l.laneBaseRefPrefixOf(proj) + l.laneBaseOf(proj)
}

// laneMainCheckout is `_lane_main_checkout`.
func (l *lch) laneMainCheckout(proj string) string { return l.c.checkout(proj) }

// laneIsProductRepo is `_lane_is_product_repo`.
func (l *lch) laneIsProductRepo(proj string) bool { return l.c.t.get(proj, "product_repo") == "1" }

// laneBuildHook is `lane_build_hook <lane>`: the file that builds the lane's worktree, or "".
func (l *lch) laneBuildHook(proj string) string {
	b := l.c.t.get(proj, "build")
	if b == "" {
		return ""
	}
	if strings.HasPrefix(b, "/") {
		return b
	}
	return filepath.Dir(l.c.t.file) + "/" + b
}

// ── the dispatcher ──

// buildLane is the `*)` arm of bin/hw's `case "$_LANE"`: it runs the lane's hook (projects.json `build`) and
// returns when the lane's build is done (the sourced file's `exit 0` and the end of the file are both a return).
// A lane with no hook dies as bin/hw does.
func (l *lch) buildLane() {
	hook := l.laneBuildHook(l.proj)
	if hook == "" { // # MUTATION-ANCHOR: 847-D1-01
		l.die(l.proj + " has a worktree in projects.json but no build path in bin/hw yet — its dry run is all this version can do. Nothing was created. Dispatch with --worktree none for a plain work directory.")
	}
	if !readable(hook) { // # MUTATION-ANCHOR: 847-D1-02
		l.die(l.proj + " builds through " + hook + " (its `build` in projects.json), and that file is not readable. Nothing was created.")
	}
	if filepath.Base(filepath.Dir(hook)) == "lanes" { // # MUTATION-ANCHOR: 847-D1-03
		switch filepath.Base(hook) {
		case "git-worktree.sh":
			l.laneBuildGitWorktree()
			return
		}
		if l.laneBuildProduct(filepath.Base(hook)) { // the maintainer's own lanes: t5l_lanes_product.go, which the export replaces by a stub
			return
		}
	}
	l.refuseForeignHook(hook) // a hook someone wrote: hw no longer has the bash to source it into
}

// refuseForeignHook ends the dispatch for a build hook that is not one of the builders under lanes/. hw used to source such
// a file into its own bash, at top level, with every variable and function hw had set (`step`, `ok`, `die`, `_agent_step`, …);
// that bash was removed in 0.6.0 and nothing in Go can source into it, so the hook cannot run, and half of its contract
// (a child process with no hw functions) would fail later and louder. Nothing was created.
func (l *lch) refuseForeignHook(hook string) {
	l.die(l.proj + " builds through " + hook + " (its `build` in projects.json), which is not one of hw's own builders under lanes/. hw used to source such a file into its own bash, and that bash was removed in 0.6.0: a builder is Go now. Nothing was created. Dispatch with --worktree none for a plain work directory.")
}

// ── what the plain lanes (setup, git-worktree) repeat ──

// laneIsDir is `[ -d p ]`; laneExists is `[ -e p ]` (both follow symlinks).
func laneIsDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
func laneExists(p string) bool { _, err := os.Stat(p); return err == nil }

// laneHead is `git -C "$wt" symbolic-ref --short HEAD 2>/dev/null || echo detached`.
func (l *lch) laneHead(wt string) string {
	out, rc := l.git(false, wt, "symbolic-ref", "--short", "HEAD")
	if rc != 0 {
		return "detached"
	}
	return trimNL(out)
}

// laneWorktreeAdd is the `git worktree add` block both plain lanes carry: the branch is used if it already
// exists (and the rollback then leaves it alone), created off base otherwise. It reports whether it added one.
func (l *lch) laneWorktreeAdd(main, wt, base string) (added bool) {
	_, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", main, "show-ref", "--verify", "--quiet", "refs/heads/" + l.branch}, merge: true})
	branchExisted := rc == 0
	if err := l.fs.mkdirAll(filepath.Dir(wt)); err != nil {
		panic(exitNow{1})
	}
	if branchExisted { // # MUTATION-ANCHOR: 847-D1-04
		if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", main, "worktree", "add", wt, l.branch}, mut: true}); rc != 0 {
			panic(exitNow{rc})
		}
		l.launchWT, l.launchWTCreated = wt, true
		l.info("branch " + l.branch + " already existed — worktree added on it, so a rollback will not delete it")
	} else {
		if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", main, "worktree", "add", "-b", l.branch, wt, base}, mut: true}); rc != 0 {
			panic(exitNow{rc})
		}
		l.launchWT, l.launchWTCreated = wt, true
		l.launchBranch, l.launchBranchCreated = l.branch, true
	}
	l.ok("worktree " + wt + " on " + l.branch + " (from " + base + ")")
	return true
}

// laneSpaceTwoPane is steps "_trust_dir … _agent_step" of the plain lanes up to the agent: trust, sdd, the env, the
// artifacts directory, the repoless layout and its two panes. The caller prints the step headers around it.
func (l *lch) laneTrustSdd(wt string) {
	l.trustDir(wt)
	l.applySdd(wt, l.sdd) // `|| true`
}

// laneSpaceTwoPane is `_export_hw_env … ok "agent=… shell=…"`: the artifacts directory, repoless.json and the panes.
func (l *lch) laneSpaceTwoPane(wt string) {
	l.exportHwEnv(wt)
	if l.artifactsDirFor(l.proj, wt) != wt {
		l.ensureArtifactsDir(wt)
	}
	l.buildSpace(l.layoutDir() + "/repoless.json")
	l.pAgent = l.paneByLabel(l.layoutOut, "agent")
	l.pShell = l.paneByLabel(l.layoutOut, "shell")
	if l.pAgent == "" || l.pShell == "" { // # MUTATION-ANCHOR: 847-D1-05
		l.die("layout applied but a labelled pane is missing (agent=" + l.pAgent + " shell=" + l.pShell + ") — check " + l.layoutDir() + "/repoless.json")
	}
	l.ok("agent=" + l.pAgent + "  shell=" + l.pShell)
}

// laneSymlink is `ln -s to path`: through the fsx's plan when read-only.
func (l *lch) laneSymlink(to, path string) {
	if l.readonly {
		l.fs.rec(fsOp{Op: "symlink", Path: path, To: to})
		return
	}
	if err := os.Symlink(to, path); err != nil {
		panic(exitNow{1})
	}
}

// ── _pick_port ──

// lookPathEnv is `command -v name` against the dispatch's PATH.
func (l *lch) lookPathEnv(name string) bool {
	for _, dir := range strings.Split(l.env("PATH"), ":") {
		if dir == "" {
			dir = "."
		}
		if st, err := os.Stat(dir + "/" + name); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// portFree is `_port_free`: lsof where it exists, ss otherwise, and a port is free when neither can say it is not.
func (l *lch) portFree(p int) bool {
	ps := strconv.Itoa(p)
	if l.lookPathEnv("lsof") {
		_, rc := l.sh(cmdSpec{name: "lsof", args: []string{"-nP", "-iTCP:" + ps, "-sTCP:LISTEN"}, merge: true})
		return rc != 0
	} else if l.lookPathEnv("ss") {
		out, rc := l.sh(cmdSpec{name: "ss", args: []string{"-Hltn", "sport = :" + ps}})
		if rc != 0 { // ss failed: no answer, not "busy"
			return true
		}
		return trimNL(out) == ""
	}
	return true
}

func (l *lch) portLocks() string {
	t := l.env("TMPDIR")
	if t == "" {
		t = "/tmp"
	}
	return t + "/hw-ports"
}

// pickPort is `_pick_port <first>`: the first free port from there upward, claimed with a lock directory so
// that two concurrent launches never get the same one.
func (l *lch) pickPort(first int) int {
	p, tries := first, 0
	locks := l.portLocks()
	l.fs.mkdirAll(locks)
	for {
		lockdir := locks + "/" + strconv.Itoa(p)
		if l.portFree(p) && l.portClaim(lockdir) { // # MUTATION-ANCHOR: 847-D1-06
			return p
		}
		// A FAILED `find` IS NOT A STALE LOCK: only a find that succeeded and printed nothing reaps.
		if l.portLockIsDir(lockdir) {
			fresh, rc := l.sh(cmdSpec{name: "find", args: []string{lockdir, "-mmin", "-10"}, quiet: true})
			if rc == 0 && fresh == "" { // # MUTATION-ANCHOR: 847-D1-07
				if l.readonly {
					l.fs.rec(fsOp{Op: "remove", Path: lockdir})
					if l.portRemoved == nil {
						l.portRemoved = map[string]bool{}
					}
					l.portRemoved[lockdir] = true
				} else {
					os.RemoveAll(lockdir)
				}
				continue
			}
		}
		p++
		tries++
		if tries > 60 { // # MUTATION-ANCHOR: 847-D1-08
			l.die("no free port near " + strconv.Itoa(first))
		}
	}
}

func (l *lch) portLockIsDir(lockdir string) bool {
	return !l.portRemoved[lockdir] && laneIsDir(lockdir)
}

// portClaim is `mkdir "$lockdir" 2>/dev/null` and, on success, the pid file.
func (l *lch) portClaim(lockdir string) bool {
	if l.readonly {
		if l.portLockIsDir(lockdir) {
			return false
		}
		l.fs.rec(fsOp{Op: "mkdir", Path: lockdir})
		l.fs.write(lockdir+"/pid", []byte(strconv.Itoa(os.Getpid())), 0o666)
		return true
	}
	if err := os.Mkdir(lockdir, 0o777); err != nil {
		return false
	}
	if err := l.fs.write(lockdir+"/pid", []byte(strconv.Itoa(os.Getpid())), 0o666); err != nil {
		panic(exitNow{1})
	}
	return true
}

// laneIsLink is `[ -L p ]`.
func laneIsLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}
