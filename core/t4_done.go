package main

// hw done <project> <task> (train T4): the port of lib/hw/done.sh — cmd_done, the in-use guard (_done_*), the
// close verification (_verify_*) — and of what it reaches in bin/hw and lib/hw/reap.sh (_close_herdr_object,
// _wt_disposition, _reap_worktree, _wt_shed, _lease_write, _brain_leak_at_close, …), byte for byte, with the
// same git, herdr and psql calls in the same order (setup/tests/contract-hw/cases-t4-done holds both).
//
// The argument block is bin/hw's (`if [ "$1" = done ]`), with what bin/hw does before it for every verb: -h or
// --help anywhere is the short usage on stdout, and fewer than two words the same usage on stderr with 64.

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// doneRun is cmd_done's state: its arguments, the flags bin/hw parsed, and the globals the bash shares between
// its helpers (_HERDR_CLOSE_RESULT, WT_VERDICT, WT_WHY, WT_ARCHIVE, WT_MERGE_EVIDENCE, REAP_ARCHIVE_DIR).
type doneRun struct {
	*app
	x           *doneLaneExtra
	binDir      string // HW_BIN_DIR
	agentsBlock bool   // WT_AGENTS_BLOCK: the verdict excused a tracked AGENTS.md that is only next dev's block
	root        string // _brain_root
	occ         occupantsReader

	dropDB, force, blocked, keepPreview bool
	keepFor, keepWhy, verifySkip        string
	acceptReview                        string // --accept-review: a brainer's reason to merge without an approved review

	t0, verifySecs int64
	closeResult    string // _HERDR_CLOSE_RESULT
	closeReason    string // _HERDR_CLOSE_REASON

	verdict, why, evidence string // WT_VERDICT, WT_WHY, WT_MERGE_EVIDENCE
	archive                []string
	archiveDir             string // REAP_ARCHIVE_DIR
	prsMain                string
	prsOK                  bool
	prs                    string
}

func (a *app) cmdDone() int {
	// bin/hw, before any verb: -h/--help anywhere is the short page; fewer than two words is a usage error
	for _, x := range a.args {
		if x == "-h" || x == "--help" {
			a.printf("%s", helpText("short-1"))
			return 0
		}
	}
	if len(a.args) < 1 {
		fmt.Fprint(a.err, helpText("short-1"))
		return 64
	}
	d := &doneRun{app: a, x: doneLoadExtras(a.c.t.file), binDir: a.getenv("HW_CORE_BIN_DIR"), root: a.getenv("HW_CORE_ROOT")}
	if d.binDir == "" {
		d.binDir = a.c.brain + "/bin"
	}
	if d.root == "" {
		d.root = a.c.brain
	}
	d.occ = occupantsReader{h: herdrClient{getenv: a.getenv}, win: windowsPaths(a.getenv)}
	args := a.args
	if len(args) < 2 { // # MUTATION-ANCHOR: 861-G01
		a.die("usage: hw done <project> <task> [--drop-db] [--force] [--accept-review <why>] [--keep-preview [<duration>] --why <reason>]")
	}
	dp, dt := args[0], args[1]
	args = args[2:]
	for len(args) > 0 {
		switch args[0] {
		case "--drop-db":
			d.dropDB = true
			args = args[1:]
		case "--force":
			d.force = true
			args = args[1:]
		case "--blocked":
			d.blocked = true
			args = args[1:]
		case "--keep-preview":
			d.keepPreview = true
			args = args[1:]
			if len(args) > 0 && args[0] != "" && !strings.HasPrefix(args[0], "--") {
				d.keepFor = args[0]
				args = args[1:]
			}
		case "--why":
			if len(args) < 2 {
				a.die(args[0] + " needs a value")
			}
			d.keepWhy = args[1]
			args = args[2:]
		case "--accept-review":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				a.die("--accept-review needs a reason: --accept-review \"<why this merge needs no approved review>\"")
			}
			if a.getenv("HW_TASK") != "" || a.getenv("HW_RUN") != "" {
				a.die("--accept-review is refused inside an executor (HW_TASK or HW_RUN is set): only a brainer overrides the review gate. Report blocked with done-invoker --blocked instead.")
			}
			d.acceptReview = args[1]
			args = args[2:]
		default:
			a.die("unknown option: " + args[0] + " (hw done takes --drop-db, --force, --blocked, --accept-review <why>, --keep-preview [<duration>], --why <reason>)")
		}
	}
	if d.keepPreview {
		if d.dropDB { // # MUTATION-ANCHOR: 861-G02
			a.die("--drop-db and --keep-preview contradict each other: the preview reads that database. Pick one.")
		}
		if d.keepWhy == "" { // # MUTATION-ANCHOR: 861-G03
			op := a.getenv("HW_OPERATOR")
			if op == "" {
				op = d.x.operator
			}
			a.die("--keep-preview needs --why \"<what is still going to look at this>\" — e.g. --why \"opus judge, then " + op + " approves the exact preview\". A retained preview with no stated reader is the thing this flag exists to stop.")
		}
	}
	if d.blocked {
		d.verifySkip = "blocked"
	}
	if d.force {
		d.verifySkip = "--force" // # MUTATION-ANCHOR: 861-G04
	}
	return d.run(a.c.t.canonProject(dp), dt)
}

// ── the run directories ─────────────────────────────────────────────────────

// taskBase is the directory _task_rundirs and _latest_run_record look in.
func (d *doneRun) taskBase(proj, task string) string {
	base := d.c.lanePath(proj, "worktree", task)
	if base == "" || !isDir(base+"/.hw") {
		// a --worktree none task in a repo lane recorded itself in the work directory, not in the lane's worktree path
		// (the same fallback receipt and revive make); without it no run record is found and an hc terminal is never closed
		base = d.c.work + "/" + proj + "/" + task
	}
	return base
}

// taskRundirs is _task_rundirs: `fd … "$base/.hw" | sort -r` — newest first, each with fd's trailing slash.
func (d *doneRun) taskRundirs(proj, task string) []string {
	base := d.taskBase(proj, task)
	if !isDir(base + "/.hw") {
		return nil
	}
	var out []string
	for _, id := range RunIDs(base + "/.hw") {
		out = append(out, base+"/.hw/"+id+"/")
	}
	return out
}

func (d *doneRun) newestRundir(proj, task string) string {
	if r := d.taskRundirs(proj, task); len(r) > 0 {
		return r[0]
	}
	return ""
}

// latestRunRecord is _latest_run_record: the newest run's ownership record of that kind, blanks removed.
func (d *doneRun) latestRunRecord(proj, task, kind string) string {
	for _, r := range d.taskRundirs(proj, task) {
		f := r + "/" + kind
		if !readableFile(f) {
			continue
		}
		b, _ := os.ReadFile(f)
		v := strings.Map(func(c rune) rune {
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f' {
				return -1
			}
			return c
		}, string(b))
		if v != "" {
			return v
		}
	}
	return ""
}

// doneMarker is _run_done_marker: the done marker of the run's CURRENT task.
func doneMarker(rundir string) string {
	seq := ""
	if b, err := os.ReadFile(rundir + "/task"); err == nil {
		seq = doneDigits(string(b))
	}
	switch seq {
	case "", "0", "1":
		return rundir + "/done"
	}
	return rundir + "/t" + seq + "/done"
}

func doneTaskDir(rundir string) string { return strings.TrimSuffix(doneMarker(rundir), "/done") }

// doneCurrentTask is _chaining_current_task.
func doneCurrentTask(rundir string) string {
	n := ""
	if b, err := os.ReadFile(rundir + "/task"); err == nil && readableFile(rundir+"/task") {
		n = doneDigits(string(b))
	}
	if n == "" || n == "0" {
		n = "1"
	}
	return n
}

// ── cmd_done ────────────────────────────────────────────────────────────────

// recordRetro: a `--blocked` close owes no retro. The markers inside markRetroPending cover the rest, but the
// chaining-lease fallback and the legacyKeep path never write them, so the flag itself is the guard.
func (d *doneRun) recordRetro(proj, task, rundir string) {
	if !d.blocked {
		d.markRetroPending(proj, task, rundir)
	}
}

func (d *doneRun) run(proj, task string) int {
	a := d.app
	d.t0 = doneNow()
	if !a.c.t.known(proj) {
		a.die("unknown project: " + proj + " (expected: " + a.c.t.namesBar() + ")")
	}
	main := a.c.checkout(proj)
	wt := a.c.lanePath(proj, "worktree", task)
	if wt == "" {
		wt = a.c.work + "/" + proj + "/" + task
	}
	branch := "task/" + task
	if a.c.t.known(proj) {
		branch = a.c.lanePath(proj, "branch", task) // # MUTATION-ANCHOR: 861-G05
	}
	closeFailed := 0

	// capture first: the live pane is the only source for an old report
	rundir := d.newestRundir(proj, task)
	if d.alreadyClosed(rundir, wt) {
		a.info(proj + ":" + task + " is already closed (its own done-invoker ran `hw done`; " + wt + " holds no checkout) — nothing to verify, nothing to merge")
		return 0
	}
	if rundir != "" {
		a.snapshotDoneReport(rundir)
	}
	d.inUseGuard(proj, task, rundir)
	if rundir != "" && isFile(doneTaskDir(rundir)+"/reopened") {
		os.Remove(doneTaskDir(rundir) + "/reopened")
		a.info("run was reopened by hw revive; closing it clears that marker")
	}

	v0 := doneNow()
	d.verifyAtClose(d.newestRundir(proj, task), wt, proj, task)
	d.verifySecs = doneNow() - v0
	d.brainLeakAtClose(proj, wt, branch)
	if rundir != "" && isFile(doneTaskDir(rundir)+"/blocked-waiting") {
		os.Rename(doneTaskDir(rundir)+"/blocked-waiting", doneDirname(doneTaskDir(rundir)+"/blocked-waiting")+"/blocked-closed")
		a.info("it had reported BLOCKED and was waiting for a ruling; closing it ends the wait")
	}
	d.cancelLease(rundir)
	d.markClosedByHand(rundir)
	d.recordRetro(proj, task, rundir)

	tab := d.latestRunRecord(proj, task, "tab")
	_, _, _, onHC := hcRunRecord(rundir)
	if tab == "" && !onHC {
		out, _ := doneTool("", nil, "herdr", "tab", "list")
		tab = doneJqSelectFirst(out, "tabs", proj+":"+task, "tab_id")
	}
	if d.keepPreview {
		return d.keepLease(proj, task, tab)
	}
	if note := d.reviveLine(proj, task, rundir); note != "" {
		a.info(note) // # MUTATION-ANCHOR: 861-G06
	}
	d.closeResult = ""
	if onHC {
		if !d.closeHC(proj, task) {
			closeFailed = 1
		}
		d.outcome(proj, task, "terminal on herdr-core")
	} else if tab != "" {
		if !d.closeObject("tab", tab, "tab "+tab+" closed ("+proj+":"+task+")", "tab "+tab+" is already gone") {
			closeFailed = 1
		}
		d.outcome(proj, task, "tab "+tab)
	} else {
		out, _ := doneTool("", nil, "herdr", "workspace", "list")
		ws := doneJqSelectFirst(out, "workspaces", proj+":"+task, "workspace_id")
		if ws != "" {
			if !d.closeObject("workspace", ws, "space "+ws+" closed", "space "+ws+" is already gone") {
				closeFailed = 1
			}
			d.outcome(proj, task, "space "+ws)
		} else {
			a.info("no open tab or space labelled " + proj + ":" + task)
			if a.getenv("HW_DONE_INVOKER_PID") != "" && !(rundir != "" && runIsHere(rundir)) { // a --here pane is no tab and no space: hw done closes nothing for it, by design
				// the close that follows a report (done-invoker's own detached closer): the report just landed, so the run owns a
				// pane, and finding none means hw looked in the wrong place. That is a failed close, not a quiet exit 0.
				d.closeResult = "unfound"
				closeFailed = 1
			}
			d.outcome(proj, task, "")
		}
	}
	if rundir != "" && closeFailed == 0 {
		os.WriteFile(doneTaskDir(rundir)+"/closed-ok", nil, 0o666) // what alreadyClosed trusts: the pane close itself succeeded
	}
	if rundir != "" {
		doneTool("", nil, d.binDir+"/cockpit-state", "--kick", "--rundir", rundir)
	}

	if d.x.get(proj, "repoless") == "1" {
		d.repoless(proj, wt)
		d.took()
		return closeFailed
	}
	a.info("ports released with the panes")

	dbLive := ""
	if a.c.t.get(proj, "db_provisioned") != "" {
		dbLive = d.database(wt)
	} else {
		a.info(proj + " has no per-worktree database to release")
	}

	base := a.c.t.get(proj, "base")
	if base == "" {
		base = "main"
	}
	ahead, aheadRef, aheadNote := int64(0), base, ""
	if _, rc := doneGit(main, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); rc == 0 {
		if _, rc := doneGit(main, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+base); rc == 0 {
			aheadRef = "origin/" + base
			fh, _ := doneGit(main, "rev-parse", "--git-path", "FETCH_HEAD")
			fh = bashSub(fh)
			if fh != "" {
				fh = main + "/" + fh
			}
			if fh != "" && isFile(fh) {
				epoch := int64(0)
				if st, err := os.Stat(fh); err == nil {
					epoch = st.ModTime().Unix()
				}
				if epoch > 0 {
					aheadNote = " (origin fetched " + fmt.Sprint((doneNow()-epoch)/3600) + "h ago)"
				}
			}
		} else {
			aheadNote = " (no origin/" + base + " — counted against the local checkout, which may be behind origin)"
		}
		out, rc := doneGit(main, "rev-list", "--count", aheadRef+".."+branch)
		s := bashSub(out)
		if rc != 0 {
			s = bashSub(out + "0\n")
		}
		if doneAllDigits(s) {
			ahead = doneAtoi(s)
		}
	}
	dirty := "0"
	if isDir(wt) {
		out, rc := doneGit(wt, "status", "--porcelain")
		if rc != 0 {
			dirty = "unknown"
		} else if s := bashSub(out); s != "" {
			dirty = fmt.Sprint(strings.Count(s, "\n") + 1)
		}
	}
	syncLines := ""
	if strings.HasPrefix(aheadRef, "origin/") {
		syncLines = "  git fetch origin\n  git checkout " + base + "\n  git pull"
	} else {
		syncLines = "  git checkout " + base + "\n  " + a.cDim + "# no origin in " + main + " — nothing to fetch or pull" + a.c0
	}
	if _, rc := doneGit(main, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); rc == 0 && ahead > 0 {
		if gate := d.reviewGate(proj, task, main, wt, branch, rundir, d.acceptReview); gate.allow {
			a.printf("\n%sMerge it:%s\n\n  cd %s\n%s\n  git merge --no-ff %s          # %d commit(s) against %s%s — or open a PR and squash-merge it\n\n",
				a.cB, a.c0, main, syncLines, branch, ahead, aheadRef, aheadNote)
			for _, n := range gate.notes {
				a.info(n)
			}
			if gate.readyPR > 0 {
				a.printf("  gh pr ready %d          # the PR is a draft; this head is the one the review covers\n\n", gate.readyPR)
			}
		} else {
			a.printf("\n%s%s%s\n", a.cB, gate.deny, a.c0)
			for _, l := range append(gate.why, gate.how...) {
				a.printf("  %s\n", l)
			}
			a.printf("\n")
		}
	} else if _, rc := doneGit(main, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); rc == 0 {
		a.printf("\n")
		a.warn("NOTHING TO MERGE: " + branch + " exists but is level with " + aheadRef + " — 0 commits." + aheadNote)
		switch {
		case dirty == "unknown":
			a.warn("and whether the worktree holds uncommitted changes could NOT be read (git status failed). That is not the same as none — look before you remove anything.")
		case doneAtoi(dirty) > 0: // # MUTATION-ANCHOR: 861-G07
			a.warn("and the worktree holds " + dirty + " uncommitted change(s). THE WORK IS NOT ON THE BRANCH.")
			a.warn("It exists in exactly one place: " + wt)
			a.warn("Commit it there before anything else, or it is one `git worktree remove` from gone:")
			a.printf("\n  git -C %s add -A && git -C %s commit -m \"<what this is>\"\n\n", wt, wt)
			a.warn("`hw reap` already refuses this worktree while it is dirty, so nothing is lost right now.")
		default:
			a.info("the worktree is clean too, so this task produced no committed change")
		}
	} else {
		a.printf("\n")
		a.info("no branch " + branch + " in " + main + " — nothing to merge")
		a.info("that is expected for a --worktree none task: it ran in a work directory, not a worktree")
	}
	if !isDir(wt) && branch != "" {
		out, _ := doneGit(main, "worktree", "list", "--porcelain")
		// awk -v b="refs/heads/$branch" '/^worktree /{w=$2} /^branch /{if ($2==b) print w}' | head -1
		found, w, printed := "", "", false
		for _, l := range strings.Split(out, "\n") {
			fs := strings.Fields(l)
			f2 := ""
			if len(fs) > 1 {
				f2 = fs[1]
			}
			if strings.HasPrefix(l, "worktree ") {
				w = f2
			}
			if strings.HasPrefix(l, "branch ") && f2 == "refs/heads/"+branch && !printed {
				found, printed = w, true
			}
		}
		if found != "" && isDir(found) {
			wt = found
			a.warn("its worktree is not at the canonical root: " + wt)
		}
	}
	if !isDir(wt) {
		a.printf("  %sno worktree for %s — already removed, or it never had one%s\n\n", a.cDim, branch, a.c0)
		if dbLive != "" {
			a.warn("database " + dbLive + " is STILL LIVE — drop it with: hw done " + proj + " " + task + " --drop-db")
		}
		d.took()
		return closeFailed
	}
	wtBranch := branch
	if _, rc := doneGit(wt, "rev-parse", "--git-dir"); rc == 0 {
		if out, rc := doneGit(wt, "symbolic-ref", "--short", "HEAD"); rc == 0 {
			wtBranch = bashSub(out)
		} else {
			wtBranch = bashSub(out + "detached\n")
		}
	}
	if lr, ok := doneLockReason(main, wt); ok && strings.HasPrefix(lr, "hw ") { // # MUTATION-ANCHOR: 861-G08
		if _, rc := doneTool("", nil, "git", "-C", main, "worktree", "unlock", wt); rc == 0 {
			a.info("unlocked " + doneBasename(wt) + " — its executor is closed")
		}
	}
	d.disposition(main, wt, wtBranch, base)
	if d.verdict == "safe" || d.verdict == "archivable" { // # MUTATION-ANCHOR: 861-G09
		if rundir != "" && readableFile(rundir+"/receipt.jsonl") {
			if v, _ := doneReceiptLast(rundir+"/receipt.jsonl", "session_resume"); v != "" {
				a.info("removing the worktree also ends `hw revive " + proj + " " + task + "` for this session")
			}
		}
		a.printf("%sReaping it — %s%s\n", a.cB, a.c0, d.why)
		if d.reapWorktree(proj, main, wt, wtBranch, base) {
			if d.archiveDir != "" {
				a.info("its outputs are in " + d.archiveDir)
			}
			if strings.HasPrefix(aheadRef, "origin/") {
				a.info("a pushed branch stays on origin: git -C " + main + " push origin --delete " + branch)
			}
			dbLive = ""
		}
	} else {
		a.printf("  %s%s DO NOT REMOVE THIS WORKTREE YET%s\n", a.cWarn, "!", a.c0)
		a.printf("      %s%s%s\n", a.cDim, d.why, a.c0)
		if d.sheddable() {
			if shed, ok := d.shed(proj, wt); ok {
				a.printf("      %sKept, and shed what its install rebuilds: %s%s\n", a.cDim, shed, a.c0)
			}
		}
		switch d.verdict {
		case "irreplaceable":
			a.printf("      %sgit status reports this worktree clean and it is not — those paths are%s\n", a.cDim, a.c0)
			a.printf("      %signored, so `git worktree remove` would delete them with no warning.%s\n", a.cDim, a.c0)
			a.printf("      %sMove what you need out first, then re-run `hw done` or `hw reap`.%s\n", a.cDim, a.c0)
		case "unmerged":
			a.printf("      %sMerge it first (above), then `hw reap %s`.%s\n", a.cDim, proj, a.c0)
		case "dirty":
			a.printf("      %sCommit or discard those changes, then `hw reap %s`.%s\n", a.cDim, proj, a.c0)
		case "held":
			a.printf("      %sThat pane is still sitting in it. Nothing to do until it moves.%s\n", a.cDim, a.c0)
		case "not-a-worktree":
			a.printf("      %sNothing to remove and nothing to merge: no branch was ever created for it.%s\n", a.cDim, a.c0)
			a.printf("      %sIts contents are the task output. Move what is worth keeping, then delete it by hand.%s\n", a.cDim, a.c0)
		case "undetermined":
			a.printf("      %sThis is NOT a verdict that the worktree is needed — it is the absence of one.%s\n", a.cDim, a.c0)
			a.printf("      %sFix what could not be read, then re-run. Do not remove it on the strength of%s\n", a.cDim, a.c0)
			a.printf("      %sa check that did not run: that is how 588K went once.%s\n", a.cDim, a.c0)
		case "locked":
			a.printf("      %sSomebody holds it; unlock it once nobody does, then `hw reap %s`.%s\n", a.cDim, proj, a.c0)
		case "leased":
			a.printf("      %sSomebody is still going to look at this preview. `hw preview` shows who and until when;%s\n", a.cDim, a.c0)
			a.printf("      %s`hw preview release %s <task>` ends it early.%s\n", a.cDim, proj, a.c0)
		}
		a.printf("      %sSurvey every worktree the same way: hw reap %s%s\n\n", a.cDim, proj, a.c0)
	}
	if dbLive != "" {
		a.warn("database " + dbLive + " is STILL LIVE — drop it with: hw done " + proj + " " + task + " --drop-db")
	}
	d.took()
	return closeFailed
}

// took is _done_took: where the seconds went, once the close took five or more.
func (d *doneRun) took() {
	total := doneNow() - d.t0
	if total >= 5 {
		d.info(fmt.Sprintf("hw done took %ds: verification %ds, everything else %ds", total, d.verifySecs, total-d.verifySecs))
	}
}

// alreadyClosed: the run reported (done marker), its own close SUCCEEDED (closed-ok, written after the pane close), it was not
// reopened by `hw revive` (and no newer report followed the close), and its worktree is no checkout any more (no .git: a reaped worktree leaves `.hw/`). Then a second
// `hw done` would verify and `git status` the PARENT repo (measured 2026-11-09). Any other state is closed the long way.
func (d *doneRun) alreadyClosed(rundir, wt string) bool {
	if rundir == "" || d.force || d.keepPreview || d.dropDB {
		return false
	}
	dir := doneTaskDir(rundir)
	if !isFile(doneMarker(rundir)) || !isFile(dir+"/closed-ok") || isFile(dir+"/reopened") {
		return false
	}
	// a revived run that reported again wrote a NEWER done marker: the closed-ok of its first close is stale
	dn, e1 := os.Stat(doneMarker(rundir))
	ok, e2 := os.Stat(dir + "/closed-ok")
	if e1 != nil || e2 != nil || ok.ModTime().Before(dn.ModTime()) {
		return false
	}
	_, err := os.Lstat(wt + "/.git")
	return err != nil
}

// markClosedByHand is _done_mark_closed_by_hand: written once, beside the task's done marker.
func (d *doneRun) markClosedByHand(rundir string) {
	rundir = strings.TrimSuffix(rundir, "/")
	if rundir == "" || !isDir(rundir) {
		return
	}
	mark := doneDirname(doneMarker(rundir)) + "/closed-by-hand"
	if _, err := os.Lstat(mark); err == nil {
		return
	}
	by, reported := "none", "no"
	if d.blocked {
		by = "--blocked"
	}
	if d.force {
		by = "--force"
	}
	if isFile(doneMarker(rundir)) {
		reported = "yes"
	}
	if os.MkdirAll(doneDirname(mark), 0o777) != nil {
		return
	}
	os.WriteFile(mark, []byte(fmt.Sprintf("at=%d\nby=%s\nreported=%s\n", doneNow(), by, reported)), 0o666) // # MUTATION-ANCHOR: 861-G10
}

// reviveLine is _done_revive_line: whether `hw revive` can bring the session back.
func (d *doneRun) reviveLine(proj, task, rundir string) string {
	if rundir != "" && readableFile(rundir+"/receipt.jsonl") {
		if b, err := os.ReadFile(rundir + "/receipt.jsonl"); err == nil && doneReSessionResume.Match(b) {
			return "if this closes and you wanted it back: hw revive " + proj + " " + task + " — run " + doneBasename(rundir) + " recorded a session_resume, so the conversation is addressable"
		}
	}
	return "NOTE: hw revive " + proj + " " + task + " will NOT bring this back — " + doneBasename(rundir) + " recorded no session_resume, so the conversation is not addressable and closing is final for it"
}

// outcome is _done_outcome: which of three facts the close was.
func (d *doneRun) outcome(proj, task, what string) {
	switch d.closeResult {
	case "closed":
		d.ok("CLOSED: " + what + " — this call closed it (" + proj + ":" + task + ")")
	case "absent":
		d.info("NOTHING TO CLOSE: " + what + " was already gone when hw asked — this call closed nothing, and that is not a failure (" + proj + ":" + task + ")")
	case "unfound":
		d.warn("NOT CLOSED — NOTHING FOUND: the report of " + proj + ":" + task + " was just delivered, so its executor owns a tab, space or terminal, and hw found none labelled that way. This call closed nothing and exits non-zero (close-failed): look before anything else: hw status")
	case "failed":
		d.warn("NOT CLOSED — FAILED: " + what + " may still be RUNNING; this call did not close it and exits non-zero (" + proj + ":" + task + "). Look before anything else: hw status")
	default:
		d.info("NOTHING TO CLOSE: no open tab or space labelled " + proj + ":" + task + " — closed earlier or never opened, so this call closed nothing, and that is not a failure")
	}
}

// closeObject is _close_herdr_object: closed, absent (herdr's own list proves it gone), or failed.
func (d *doneRun) closeObject(kind, id, success, absent string) bool {
	d.closeResult = "failed"
	out, rc := doneCapture("herdr", kind, "close", id)
	if rc == 0 {
		d.closeResult = "closed"
		d.ok(success)
		return true
	}
	reason := doneHeadTr(out, 3)
	if reason == "" {
		reason = "no reason returned"
	}
	d.closeReason = ""
	switch d.objectExists(kind, id) { // # MUTATION-ANCHOR: 861-G11
	case 0:
		d.warn("CLOSE FAILED: " + kind + " " + id + " still exists (close exit " + itoa(rc) + ": " + reason + ")")
		return false
	case 1:
		d.closeResult = "absent"
		d.info(absent)
		return true
	}
	d.warn("CLOSE FAILED: " + kind + " " + id + " could not be closed (exit " + itoa(rc) + ": " + reason + ")")
	d.warn("absence could not be verified: " + d.closeReason)
	return false
}

// objectExists is _herdr_object_exists: 0 present, 1 absent, 2 unknown (closeReason says why).
func (d *doneRun) objectExists(kind, id string) int {
	var array, fieldName string
	switch kind {
	case "pane":
		array, fieldName = "panes", "pane_id"
	case "tab":
		array, fieldName = "tabs", "tab_id"
	case "workspace":
		array, fieldName = "workspaces", "workspace_id"
	default:
		d.closeReason = "unknown herdr object kind: " + kind
		return 2
	}
	out, rc := doneCapture("herdr", kind, "list")
	if rc != 0 {
		d.closeReason = "list exited " + itoa(rc) + ": " + doneHeadTr(out, 3)
		return 2
	}
	r := doneListHas(out, array, fieldName, id)
	if r == 2 {
		d.closeReason = "list returned no parseable .result." + array + " array"
	}
	return r
}

// repoless is the close of a lane with no repository: its work directory is the output, reported, never removed.
func (d *doneRun) repoless(proj, wt string) {
	a := d.app
	if isDir(wt) {
		n := 0
		doneWalkFiles(wt, func(p string) {
			if !strings.Contains(p, "/.hw/") {
				n++
			}
		})
		szOut, _ := doneTool("", nil, "du", "-sh", wt)
		sz := ""
		if f := strings.Fields(t4FirstLine(szOut)); len(f) > 0 {
			sz = f[0]
		}
		if n == 0 { // # MUTATION-ANCHOR: 861-G12
			a.info("no artifacts in " + wt + " (correct when the output was a report)")
		} else {
			a.warn(fmt.Sprintf("artifacts KEPT in %s (%d files, %s) — hw never deletes them", wt, n, sz))
		}
	} else {
		a.info("no work directory at " + wt)
	}
	a.printf("\n  %sNothing to merge:%s %s has no repository.\n  Move anything worth keeping out of %s yourself.\n\n", a.cB, a.c0, proj, wt)
}

func t4FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// database is the per-worktree database of a lane with `db.provisioned`: reported, dropped only when asked.
// It returns what stays live ("name (size)") for the end of the close to name.
func (d *doneRun) database(wt string) string {
	a := d.app
	db := strings.NewReplacer("-", "_", "/", "_").Replace(doneBasename(wt))
	user := doneWhoami()
	out, rc := doneTool("", nil, "psql", "-U", user, "-d", "postgres", "-Atc", "select 1 from pg_database where datname = '"+db+"'")
	if rc != 0 || !strings.Contains(out, "1") {
		a.info("no database named " + db)
		return ""
	}
	sizeOut, src := doneTool("", nil, "psql", "-U", doneWhoami(), "-d", "postgres", "-Atc", "select pg_size_pretty(pg_database_size('"+db+"'))")
	if src != 0 {
		panic(exitNow{src}) // `size="$(psql …)"` under set -e: a failed size query ends hw done, silently
	}
	size := bashSub(sizeOut)
	if d.dropDB { // # MUTATION-ANCHOR: 861-G13
		if _, rc := doneTool("", nil, "psql", "-U", doneWhoami(), "-d", "postgres", "-q", "-c", "DROP DATABASE \""+db+"\""); rc == 0 {
			a.ok("database " + db + " dropped (was " + size + ")")
		} else {
			a.warn("could not drop " + db + " — open connections? try again once the panes are gone")
		}
		return ""
	}
	return db + " (" + size + ")"
}

// doneWhoami is `id -un`.
func doneWhoami() string {
	out, _ := doneTool("", nil, "id", "-un")
	return bashSub(out)
}

// cancelLease ends the chaining-lease timer a --keep-pane run left running. Its scheduler is
// `nohup bash -c 'sleep N; exec hw chaining-lease-expire …'`: 3600 s of sleeper that, after `hw done`, has nothing
// left to expire (QA 2026-11-09 found two). The pid comes from the lease file and is signalled only while its
// command line still names chaining-lease-expire — a recycled pid is somebody else's.
func (d *doneRun) cancelLease(rundir string) {
	pids := cancelLeaseTimers(rundir)
	for _, pid := range pids {
		d.app.info("cancelled the chaining lease timer (pid " + strconv.Itoa(pid) + "): this task is closed, nothing is left to expire")
	}
	if len(pids) == 0 {
		return
	}
	if f, err := os.OpenFile(rundir+"/chaining-lease", os.O_WRONLY|os.O_APPEND, 0o666); err == nil {
		f.WriteString("state=cancelled\n")
		f.Close()
	}
}

// cancelLeaseTimers signals every chaining-lease timer of the run and answers the pids it ended. The lease file keeps only the
// NEWEST scheduler (`hw next` writes task N's over task N-1's), so the run's other timers are found by their command line, which
// carries the run directory. A pid is signalled only while its command line still names chaining-lease-expire.
func cancelLeaseTimers(rundir string) (cancelled []int) {
	if rundir == "" || !isFile(rundir+"/chaining-lease") { // a run that never held a lease has no timer to look for
		return nil
	}
	pids := map[int]bool{}
	if pid, err := strconv.Atoi(t3LeaseField(rundir, "scheduler_pid")); err == nil && pid > 1 {
		pids[pid] = true
	}
	if out, err := exec.Command("pgrep", "-f", "chaining-lease-expire.*"+regexp.QuoteMeta(rundir)+" ").Output(); err == nil {
		for _, f := range strings.Fields(string(out)) {
			if pid, err := strconv.Atoi(f); err == nil && pid > 1 {
				pids[pid] = true
			}
		}
	}
	for pid := range pids {
		cmdline, _ := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
		if !strings.Contains(string(cmdline), "chaining-lease-expire") {
			continue
		}
		exec.Command("pkill", "-P", strconv.Itoa(pid)).Run() // the sleep: killing bash alone orphans it
		if syscall.Kill(pid, syscall.SIGTERM) == nil {
			cancelled = append(cancelled, pid)
		}
	}
	return cancelled
}
