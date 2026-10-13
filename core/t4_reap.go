package main

// `hw reap` — lib/hw/reap.sh (cmd_reap and the removal `hw done` shares with it), with the verdicts of bin/hw
// (_wt_disposition, _workdir_disposition, _wt_branch_merged, the retention windows). The shim sends `reap … --apply`
// here; the plan half (no --apply) is the read of another train, and the code that prints it is the same code, so a
// direct `hw-core reap` prints the plan the way bash does.
//
// WHAT IS KEPT FROM BASH ON PURPOSE, bugs and orderings included: every git, herdr, psql, pg_dump, pg_restore and
// fd call is made with bash's argv in bash's order (the contract records them in `calls`); `safe` stays an
// allowlist (nothing is removed unless its verdict is `safe` or `archivable`, and an archived worktree is judged
// again); the retention log is written BEFORE a removal by retention, never after.
//
// Everything a case needs to tell the two apart is in contract-hw/cases-t4-reap; every guard that must not move
// carries a mutation-anchor marker (865-G<nn>) held by setup/tests/865-las-anclas-de-reap.sh.

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type reapApplyRun struct {
	a     *app
	apply bool
	occ   *occupantsReader
	ex    *reapApplyExtras

	// WT_VERDICT, WT_WHY, WT_MERGE_EVIDENCE, WT_ARCHIVE
	verdict, why, evidence, archive string
	// WD_VERDICT, WD_WHY, WD_RET_AGE
	wdVerdict, wdWhy, wdRetAge string
	// RET_AGE, RET_WIN, RET_KIND
	retAge, retWin, retKind string
	// WT_BUILD_DIRS, WT_BUILD_KB, WT_BUILD_AGE
	buildDirs string
	buildKB   int64
	buildAge  int64
	// REAP_ARCHIVE_DIR
	archiveDir string
	// _WT_PRS_MAIN, _WT_PRS_OK, _WT_PRS
	prsMain string
	prsOK   bool
	prs     string
}

// reapApplyExtras are the lane fields lanes.go does not carry: `reap` (copies, copies_from) and the retention windows,
// read from the lane table the way project-spaces.sh's jq program reads them.
type reapApplyExtras struct {
	copies     map[string][]string
	copiesFrom map[string]string
	retArt     string
	retBak     string
	retBuild   string
}

func reapApplyLoadExtras(file string) *reapApplyExtras {
	ex := &reapApplyExtras{copies: map[string][]string{}, copiesFrom: map[string]string{}, retArt: "30", retBak: "60", retBuild: "7"}
	data, err := os.ReadFile(file)
	if err != nil {
		return ex
	}
	vals, _ := parseStream(data)
	if len(vals) == 0 {
		return ex
	}
	root := vals[0]
	if ret := field(root, "retention"); ret != nil {
		ex.retArt = orDefault(field(ret, "artifact_retention_days"), "30")
		ex.retBak = orDefault(field(ret, "backup_retention_days"), "60")
		ex.retBuild = orDefault(field(ret, "build_output_days"), "7")
	}
	if lanes := field(root, "lanes"); lanes != nil && lanes.kind == vObj {
		for _, m := range lanes.obj {
			reap := field(m.v, "reap")
			if reap == nil {
				continue
			}
			ex.copies[m.k] = strList(field(reap, "copies"))
			ex.copiesFrom[m.k] = orDefault(field(reap, "copies_from"), "")
		}
	}
	return ex
}

func (a *app) cmdReapApply() int {
	// bin/hw: `need herdr; need git` come before any verb
	for _, tool := range []string{"herdr", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			a.die("missing required command: " + tool)
		}
	}
	r := &reapApplyRun{a: a}
	r.occ = &occupantsReader{h: herdrClient{getenv: a.getenv}, win: windowsPaths(a.getenv)}
	r.ex = reapApplyLoadExtras(a.c.t.file)
	return r.cmdReap()
}

// ── the paths bin/hw spells ────────────────────────────────────────────────────

func (r *reapApplyRun) archiveRoot() string { // _archive_root
	if v := r.a.getenv("HW_ARCHIVE_ROOT"); v != "" {
		return v
	}
	return reapApplyDirname(r.a.c.work) + "/archive"
}

func (r *reapApplyRun) logFile() string { // _reap_log_file
	if v := r.a.getenv("HW_REAP_LOG"); v != "" {
		return v
	}
	return r.a.c.brain + "/.hw-reap-retention.log"
}

func (r *reapApplyRun) logReady() bool { // _reap_log_ready: `: >> "$(_reap_log_file)"`
	f, err := os.OpenFile(r.logFile(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
	if err != nil {
		// `: >> file 2>/dev/null` — the redirections apply left to right, so the failed `>>` is reported BEFORE stderr is
		// silenced: bash names its own source line (reap.sh, line 513) and the errno, on the real stderr.
		var pe *fs.PathError
		why := err.Error()
		if errors.As(err, &pe) {
			why = pe.Err.Error()
		}
		if why != "" {
			why = strings.ToUpper(why[:1]) + why[1:]
		}
		bin := r.a.getenv("HW_CORE_BIN_DIR")
		if bin == "" {
			bin = r.a.c.brain + "/bin"
		}
		r.a.printErr(bin + "/../lib/hw/reap.sh: line 513: " + r.logFile() + ": " + why)
		return false
	}
	f.Close()
	return true
}

func (r *reapApplyRun) canonicalRoot(proj string) (string, bool) { // _wt_canonical_root
	d := r.a.c.lanePath(proj, "worktree", "__probe__")
	if d == "" {
		return "", false
	}
	return reapApplyDirname(d), true
}

func (r *reapApplyRun) retentionDays(def, value string) string { // _retention_days
	if !reapApplyAllDigits(value) {
		return def
	}
	if len(value) > 9 {
		value = value[:9]
	}
	return itoa(int(reapApplyAtoi(value)))
}

func (r *reapApplyRun) windowDays(envName, table, def string) string {
	v := r.a.getenv(envName)
	if v == "" { // `: "${VAR:=$_LC_RETENTION_...}"`
		v = table
	}
	if v == "" {
		v = def
	}
	return r.retentionDays(def, v)
}

func (r *reapApplyRun) buildWindow() string {
	return r.windowDays("HW_BUILD_OUTPUT_DAYS", r.ex.retBuild, "7")
}

func (r *reapApplyRun) blockedWaitHours() string { // _blocked_wait_hours
	h := r.a.getenv("HW_BLOCKED_WAIT_HOURS")
	if h == "" {
		h = "24"
	}
	if !reapApplyAllDigits(h) {
		h = "24"
	}
	return h
}

// ── cmd_reap ───────────────────────────────────────────────────────────────────

func (r *reapApplyRun) usage() {
	a := r.a
	a.printErr("usage: hw reap [<project>] [--apply] — surveys the lane's git worktrees, its merged task branches no worktree holds, AND the loose work dirs beside them (" + a.c.work + "/<lane>). --apply removes what is safe; a merged worktree whose only ignored content is .artifacts/qa-report/test-results/playwright-report has it copied to " + r.archiveRoot() + "/<lane>/<task>/ and verified first, and a db.provisioned lane's database is pg_dump'ed there before it is dropped. Dirty, unmerged and occupied stay. Retention: a loose work dir whose task REPORTED (done, or blocked and closed) and not occupied is safe once every artifact is older than artifact_retention_days (30), a backup (*.tgz, *.tar.gz, *.tar, or a name containing backup) older than backup_retention_days (60) — both in projects.json under retention, 0 disables a window, HW_ARTIFACT_RETENTION_DAYS/HW_BACKUP_RETENTION_DAYS override; the dry run says «retention: Nd > 30d», and every removal by retention appends a line (time, lane, path, task, age, size, reason) to " + r.logFile() + " (HW_REAP_LOG). A task that never reported or has a live pane is kept at any age. A kept worktree idle past build_output_days (7; 0 disables) has its .next/.turbo freed (dry run lists them with sizes; --apply logs each); never one with a live pane or process in it. It also closes an executor kept after a --blocked report and left unanswered past HW_BLOCKED_WAIT_HOURS (default 24).")
}

func (a *app) printErr(s string) { a.term.err.Write([]byte(s + "\n")) }

func (r *reapApplyRun) cmdReap() int {
	a := r.a
	apply := false
	projFilter := ""
	for _, arg := range a.args {
		switch {
		case arg == "--apply":
			apply = true
		case arg == "-h" || arg == "--help":
			r.usage()
			return 0
		case strings.HasPrefix(arg, "-"):
			a.die("unknown option: " + arg)
		default:
			projFilter = a.c.t.canonProject(arg)
		}
	}
	r.apply = apply

	var totalSafe, totalKept, totalRemoved, buildKBTotal int64
	for _, proj := range a.c.t.survey {
		main := a.c.checkout(proj)
		if main == "" { // _repo_lanes: a lane with a checkout
			continue
		}
		if projFilter != "" && projFilter != proj {
			continue
		}
		if !isDir(main) {
			continue
		}
		base := a.c.t.get(proj, "base")
		if base == "" {
			base = "main"
		}
		a.printf("\n  %s%s%s  (base %s)\n", a.cB, proj, a.c0, base)
		laneSafe0, laneRemoved0 := totalSafe, totalRemoved
		if seedFrom := r.ex.copiesFrom[proj]; seedFrom != "" {
			note, _, _ := r.seedPatterns(main + "/" + seedFrom)
			a.info(note)
		}

		listing, _, _ := reapApplyGit(main, "worktree", "list", "--porcelain")
		var wts []string
		for _, l := range strings.Split(listing, "\n") {
			if strings.HasPrefix(l, "worktree ") {
				wts = append(wts, l[len("worktree "):])
			}
		}
		if len(wts) > 0 { // tail -n +2: the main checkout is git's first entry
			wts = wts[1:]
		}
		for _, wt := range wts {
			if wt == "" {
				continue
			}
			if wt == main {
				continue
			}
			branch := "detached"
			if o, _, rc := reapApplyGit(wt, "symbolic-ref", "--short", "HEAD"); rc == 0 {
				branch = bashSub(o)
			}
			if a.c.t.get(proj, "db_provisioned") != "" {
				for _, rdb := range r.receiptDatabases(wt) {
					r.dbLedgerAdd(proj, rdb, wt)
				}
			}
			r.disposition(main, wt, branch, base)
			if r.verdict == "safe" || r.verdict == "archivable" { // # MUTATION-ANCHOR: 865-G02
				totalSafe++
				if apply {
					if r.reapWorktree(proj, main, wt, branch, base) {
						totalRemoved++
					}
				} else {
					a.ok(reapApplyBasename(wt) + "  " + a.cDim + r.why + a.c0)
					if r.verdict == "archivable" {
						a.printf("      %sarchive to %s/%s/%s, then remove%s\n", a.cDim, r.archiveRoot(), proj, reapApplyBasename(wt), a.c0)
					}
				}
				continue
			}
			totalKept++
			a.printf("  %s!%s %s %s%s: %s%s\n", a.cWarn, a.c0, reapApplyPad(reapApplyBasename(wt), 42), a.cDim, r.verdict, r.why, a.c0)
			if r.buildIdle(proj, wt) {
				bwin := r.buildWindow()
				r.wdRetAge = itoa(int(r.buildAge))
				r.wdWhy = "build output retention: " + r.wdRetAge + "d > " + bwin + "d"
				dirs := r.buildDirsShown(wt)
				if !apply {
					buildKBTotal += r.buildKB
					a.printf("      %sbuild output of %s idle %dd > %sd, would free %d MB: %s%s\n", a.cDim, reapApplyBasename(wt), r.buildAge, bwin, r.buildKB/1024, dirs, a.c0)
				} else if r.logReady() { // # MUTATION-ANCHOR: 865-G38
					buildKBTotal += r.buildKB
					for _, bd := range reapApplyLines(r.buildDirs) {
						bkb := r.duKB(bd)
						if r.logRetention(proj, bd, bkb) {
							reapApplyRC("rm", "-rf", bd)
						}
					}
					a.printf("      %sbuild output of %s idle %dd > %sd, freed %d MB: %s%s\n", a.cDim, reapApplyBasename(wt), r.buildAge, bwin, r.buildKB/1024, dirs, a.c0)
				} else {
					a.warn("kept the build output of " + reapApplyBasename(wt) + " — " + r.logFile() + " cannot be written, and an unlogged removal is the one this log forbids")
				}
			}
			if apply && r.sheddable() {
				if shed, ok := r.shed(proj, wt); ok {
					a.printf("      %skept, and shed %s%s\n", a.cDim, shed, a.c0)
				}
			}
		}

		// ── the merged task branches no worktree holds ──
		for _, ob := range r.orphanBranches(proj, main, base) {
			totalSafe++
			if apply {
				if r.deleteBranch(main, ob, base) {
					totalRemoved++
					a.ok("deleted " + ob + "  " + a.cDim + "(merged, no worktree)" + a.c0)
				} else {
					a.warn("kept " + ob + " — git refused a safe delete, so git does not see it as merged")
				}
			} else {
				a.ok(ob + "  " + a.cDim + "branch merged into " + base + ", no worktree holds it" + a.c0)
			}
		}

		// ── second pass: the loose work dirs ──
		wdRoot := a.c.work + "/" + proj
		looseSeen := 0
		if isDir(wdRoot) {
			listed, _ := reapApplyFd("-H", "-t", "d", "-d", "1", ".", wdRoot)
			var ds []string
			for _, l := range strings.Split(listed, "\n") {
				if l == "" {
					continue
				}
				ds = append(ds, strings.TrimSuffix(l, "/"))
			}
			sorted := reapApplySort(strings.Join(ds, "\n") + "\n")
			for _, wd := range reapApplyLines(sorted) {
				r.workdirDisposition(proj, wd)
				if r.wdVerdict == "worktree" {
					continue
				}
				looseSeen++
				if r.wdVerdict == "safe" {
					totalSafe++
					if apply {
						var retKB int64
						if r.wdRetAge != "" {
							if !r.logReady() { // # MUTATION-ANCHOR: 865-G04
								a.warn("kept " + reapApplyBasename(wd) + " — it is past retention but " + r.logFile() + " cannot be written, and a removal by retention is never unlogged")
								continue
							}
							retKB = r.duKB(wd)
						}
						if r.rmWorkdir(wd) {
							totalRemoved++
							if r.wdRetAge != "" {
								if !r.logRetention(proj, wd, retKB) {
									a.warn("removed " + wd + " by retention but could not append to " + r.logFile() + " — record it by hand: " + r.wdRetAge + "d, " + itoa(int(retKB)) + "KB, " + r.wdWhy)
								}
							}
							why := ""
							if r.wdRetAge != "" {
								why = ", " + r.wdWhy
							}
							a.ok("removed " + reapApplyBasename(wd) + "  " + a.cDim + "(loose work dir" + why + ")" + a.c0)
						} else {
							a.warn("could not remove " + wd + " — it read as safe and the delete failed, which is the one outcome worse than keeping it. Run it by hand and read what rm says.")
						}
					} else {
						a.ok(reapApplyBasename(wd) + "  " + a.cDim + "loose work dir: " + r.wdWhy + a.c0)
					}
				} else {
					totalKept++
					a.printf("  %s!%s %s %s%s: %s%s\n", a.cWarn, a.c0, reapApplyPad(reapApplyBasename(wd), 42), a.cDim, r.wdVerdict, r.wdWhy, a.c0)
				}
			}
		}
		if looseSeen == 0 {
			a.info("no loose work dirs under " + wdRoot)
		}

		// ── the databases hw recorded whose worktree is gone ──
		lf := r.dbLedger(proj)
		if a.c.t.get(proj, "db_provisioned") != "" && readable(lf) {
			keepRows := ""
			data, _ := os.ReadFile(lf)
			text := string(data)
			for len(text) > 0 { // `while read` runs a line only when it ends in a newline
				nl := strings.IndexByte(text, '\n')
				if nl < 0 {
					break
				}
				line := text[:nl]
				text = text[nl+1:]
				ldb, lwt := reapApplyTabFields(line)
				if ldb == "" || lwt == "" {
					continue
				}
				if isDir(lwt) {
					keepRows += ldb + "\t" + lwt + "\n"
					continue
				}
				if !r.dbExists(ldb) {
					continue // already gone: the row goes too
				}
				if ldb != reapApplyDBName(reapApplyBasename(lwt)) { // # MUTATION-ANCHOR: 865-G05
					a.warn("kept database " + ldb + " — recorded for " + lwt + ", whose name does not derive it; drop it by hand if it is disposable")
					keepRows += ldb + "\t" + lwt + "\n"
					continue
				}
				totalSafe++
				if apply {
					r.reapDB(proj, main, lwt, "")
					if r.dbExists(ldb) {
						keepRows += ldb + "\t" + lwt + "\n"
					} else {
						totalRemoved++
					}
				} else {
					a.ok("database " + ldb + "  " + a.cDim + "its worktree " + reapApplyBasename(lwt) + " is gone; pg_dump into the archive, then drop" + a.c0)
					keepRows += ldb + "\t" + lwt + "\n"
				}
			}
			if apply {
				_ = os.WriteFile(lf, []byte(keepRows), 0o666)
			}
		}

		// ── build output: the weight `df` actually gets back ──
		var bkb int64
		if broot, ok := r.canonicalRoot(proj); ok && isDir(broot) {
			listed, _ := reapApplyFd("-H", "-I", "-t", "d", "-d", "4", "-E", "node_modules", `^\.(next|turbo)$`, broot)
			for _, d := range reapApplyLines(listed) {
				o, _ := reapApplyOut("du", "-sk", d)
				f := strings.Fields(o)
				if len(f) > 0 {
					bkb += reapApplyLeadingInt(f[0])
				}
			}
			a.info("build output (.next/.turbo) under " + broot + ": " + itoa(int(bkb/1048576)) + " GB")
		}
		_ = os.WriteFile(a.c.brain+"/.hw-reap-"+proj, []byte(itoa(int(bkb))+" "+itoa(int(reapApplyNow()))+" "+itoa(int(totalSafe-laneSafe0-(totalRemoved-laneRemoved0)))+"\n"), 0o666)
	}

	// ── third pass: executors that reported BLOCKED and are still waiting ──
	hours := r.blockedWaitHours()
	seenB := false
	for _, rd := range r.runDirsEveryLane() {
		mk := reapApplyBlockedMarker(rd)
		if !isFile(mk) {
			continue
		}
		bproj, btask, bpane := reapApplyBlockedField(mk, "project"), reapApplyBlockedField(mk, "task"), reapApplyBlockedField(mk, "pane")
		if bproj == "" || btask == "" {
			continue
		}
		if projFilter != "" && projFilter != bproj {
			continue
		}
		if !seenB {
			a.printf("\n  %sblocked, waiting for a ruling%s  (closed after %sh unanswered — HW_BLOCKED_WAIT_HOURS)\n", a.cB, a.c0, hours)
			seenB = true
		}
		left := r.blockedSecondsLeft(mk)
		switch {
		case left > 0: // # MUTATION-ANCHOR: 865-G06
			pane := bpane
			if pane == "" {
				pane = "<pane>"
			}
			a.printf("  %s·%s %s %swaiting, %dh%02dm left — hw ruling %s \"<the unblock>\" resumes it%s\n", a.cDim, a.c0, reapApplyPad(bproj+":"+btask, 42), a.cDim, left/3600, left%3600/60, pane, a.c0)
		case apply:
			if r.closeBlocked(bproj, btask) {
				a.ok("closed " + bproj + ":" + btask + " — reported BLOCKED and unanswered for over " + hours + "h")
			} else {
				a.warn("could not close " + bproj + ":" + btask + " (blocked, unanswered for over " + hours + "h) — run hw done " + bproj + " " + btask + " --blocked by hand and read what it says")
			}
		default:
			a.ok(bproj + ":" + btask + "  " + a.cDim + "reported BLOCKED, unanswered for over " + hours + "h" + a.c0)
			a.printf("      %shw done %s %s --blocked%s\n", a.cDim, bproj, btask, a.c0)
		}
	}

	a.printf("\n")
	if buildKBTotal != 0 {
		verb := "would be freed by --apply"
		if apply {
			verb = "freed"
		}
		a.info("build output of idle kept worktrees: " + itoa(int(buildKBTotal/1024)) + " MB " + verb + " (build_output_days)")
	}
	if apply {
		suffix := ""
		if totalRemoved != totalSafe {
			suffix = " — " + itoa(int(totalSafe-totalRemoved)) + " disposable could not be removed, read the lines above"
		}
		a.ok(itoa(int(totalRemoved)) + " removed, " + itoa(int(totalKept)) + " kept" + suffix)
	} else {
		a.info(itoa(int(totalSafe)) + " safe to remove, " + itoa(int(totalKept)) + " kept. Add --apply to remove the safe ones.")
		if totalKept != 0 {
			a.info("nothing kept is ever removed by --apply, whatever the reason says")
			a.info("an ignored path that IS disposable here: HW_WT_REGENERABLE='<name> <name>' hw reap")
		}
	}
	return 0
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// reapApplyLeadingInt is awk's number from a field: its leading digits.
func reapApplyLeadingInt(s string) int64 {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return reapApplyAtoi(s[:i])
}

// reapApplyTabFields is `IFS=$'\t' read -r a b`: leading tabs dropped, a runs to the first tab, b is the rest minus the
// tabs around it.
func reapApplyTabFields(line string) (string, string) {
	line = strings.TrimLeft(line, "\t")
	i := strings.IndexByte(line, '\t')
	if i < 0 {
		return line, ""
	}
	return line[:i], strings.Trim(line[i:], "\t")
}

func reapApplyDBName(s string) string { // tr '\-/' '__'
	return strings.NewReplacer("-", "_", "/", "_").Replace(s)
}

func (r *reapApplyRun) dbLedger(proj string) string { return r.a.c.brain + "/.hw-db-ledger-" + proj } // _db_ledger

func (r *reapApplyRun) dbLedgerAdd(proj, db, wt string) { // _db_ledger_add
	if proj == "" || db == "" || wt == "" {
		return
	}
	f := r.dbLedger(proj)
	row := db + "\t" + wt
	if data, err := os.ReadFile(f); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			if l == row {
				return
			}
		}
	}
	h, err := os.OpenFile(f, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
	if err != nil {
		return
	}
	defer h.Close()
	_, _ = h.WriteString(row + "\n")
}

// receiptDatabases is `cat "$wt"/.hw/2*/receipt.jsonl | jq -r 'select(.key=="database") | .value' | sort -u`, word-split.
func (r *reapApplyRun) receiptDatabases(wt string) []string {
	files, _ := filepath.Glob(wt + "/.hw/2*/receipt.jsonl")
	var all []byte
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil {
			all = append(all, b...)
		}
	}
	vals, _ := parseStream(all)
	var outs []string
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok || k.kind != vStr || k.s != "database" {
			continue
		}
		val, _ := v.get("value")
		outs = append(outs, val.raw())
	}
	if len(outs) == 0 {
		return nil
	}
	return strings.Fields(reapApplySort(strings.Join(outs, "\n")+"\n", "-u"))
}

// seedPatterns is _wt_seed_patterns: the case labels of the worktree script that name a copy, and the note saying
// which line they came from or why none did.
func (r *reapApplyRun) seedPatterns(wtSh string) (string, []string, bool) {
	f, err := os.Open(wtSh)
	if err != nil {
		return "no " + wtSh + " — reap falls back to the hand-maintained allowlist", nil, false
	}
	f.Close()
	data, _ := os.ReadFile(wtSh)
	lineno, text := 0, ""
	for i, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, `_copy_one "$wt" "$rel" "$dry"`) {
			lineno, text = i+1, l
			break
		}
	}
	if lineno == 0 {
		return wtSh + ` has no _copy_one case line at the shape reap expects — falling back to the hand-maintained allowlist`, nil, false
	}
	pat := strings.TrimLeft(text, " \t\r\n\v\f")
	if i := strings.IndexByte(pat, ')'); i >= 0 {
		pat = pat[:i]
	}
	if pat == "" {
		return "could not parse the case pattern at " + wtSh + ":" + itoa(lineno) + " — falling back to the hand-maintained allowlist", nil, false
	}
	return "seeded from " + wtSh + ":" + itoa(lineno) + " (" + pat + ")", strings.Split(pat, "|"), true
}
