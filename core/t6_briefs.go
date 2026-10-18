package main

// hw briefs — finished briefs leave briefs/ for briefs/archive/<yyyy-mm>/, and come back (lib/hw/briefs.sh:
// cmd_briefs, _briefs_usage, _briefs_archive, _briefs_restore, _briefs_unarchive, _briefs_move_back; the plan is in
// t6_briefs_plan.go, the commit in t6_briefs_commit.go). `_archived_brief_path`/`_gate` are the dispatch's, already in
// t5_brief.go (brArchivedPath, archivedBriefGate), and are reused here. The executor refusal of `briefs
// archive|restore|unarchive` is not here: the shim keeps an executor on bash.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const briefsUsageText = `hw briefs archive <lane> [--apply] [--list]
    Move every finished brief of the lane to <lane>/briefs/archive/<yyyy-mm>/ (the month it
    reported), in one commit, with a MANIFEST.tsv per month. Finished = ` + "`hw ledger`" + ` says done,
    or — for a brief the ledger never saw, because it is older than the ledger — the history
    says so (` + "`backfill: …`" + ` in the reason): a run reported in $WORK/<lane>/<task>/.hw, or, in a
    lane whose checkout is the brain repo, a merge of its task branch on the base; and no live
    worktree. A brief with neither a ledger line nor such evidence whose last commit in the brain
    is more than 21 days old is ` + "`stale: …`" + ` and moves too, to the month of that commit. --list names the in-progress and never-dispatched briefs instead of counting them. Never moves a
    never-dispatched or in-progress brief, a ` + "`_`" + ` document, or one that is untracked or modified.
    Without --apply it prints the plan and changes nothing. The commit lands on the branch the
    checkout has checked out.
hw briefs restore <lane> <task>
    Move one archived brief back to <lane>/briefs/<task>.md (one commit; refuses to overwrite).
hw briefs unarchive <lane> --manifest <file> [--manifest <file> ...]
    Move back every brief a manifest lists, all or nothing, and delete the manifest.

An archived task does not launch: ` + "`hw <lane> <task>`" + ` dies naming the archived path and the
restore command — ` + "`--no-brief`" + ` does not override it; ` + "`--brief <archived path>`" + ` runs it as is.
`

// cmdBriefs is cmd_briefs.
func (a *app) cmdBriefs() int {
	args := a.args
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "archive":
		return a.briefsArchive(args)
	case "restore":
		return a.briefsRestore(args)
	case "unarchive":
		return a.briefsUnarchive(args)
	case "", "-h", "--help", "help":
		a.printf("%s", briefsUsageText)
		return 0
	}
	a.die("unknown: hw briefs " + sub + " (usage: hw briefs archive|restore|unarchive — hw briefs --help)")
	return 1
}

// briefsExit ends the verb with a status and no message (bash's `exit 1` after the helper already warned).
func briefsExit(rc int) { panic(exitNow{rc}) }

// briefsArchive is _briefs_archive.
func (a *app) briefsArchive(args []string) int {
	lane := ""
	apply, list := false, false
	for i := 0; i < len(args); i++ {
		switch s := args[i]; {
		case s == "--apply":
			apply = true
		case s == "--list":
			list = true
		case s == "-h" || s == "--help":
			a.printf("%s", briefsUsageText)
			return 0
		case strings.HasPrefix(s, "-"):
			a.die("unknown option: " + s + " (usage: hw briefs archive <lane> [--apply] [--list])")
		default:
			if lane != "" {
				a.die("hw briefs archive takes one lane (got: " + lane + ", " + s + ")")
			}
			lane = a.c.t.canonProject(s)
		}
	}
	if lane == "" {
		a.die("usage: hw briefs archive <lane> [--apply]")
	}
	brain := a.c.brain
	if !isDir(brain + "/" + lane + "/briefs") {
		a.die("no briefs directory for " + lane + ": " + brain + "/" + lane + "/briefs")
	}
	if _, ok := briefsGit(brain, "rev-parse", "--show-toplevel"); !ok {
		a.die("briefs: " + brain + " is not a git checkout; archive commits what it moves")
	}
	plan := a.briefsPlan(lane)
	nMove, nStale := 0, 0
	for _, r := range plan {
		if r.move {
			nMove++
			if strings.HasPrefix(r.reason, "stale:") {
				nStale++
			}
		}
	}
	if apply {
		a.printf("archive %s\n", lane)
	} else {
		a.printf("archive plan for %s (a dry run: add --apply to move)\n", lane)
	}
	for _, r := range plan {
		if r.move {
			a.printf("  move  %s  %s  (%s)\n", r.ym, r.task, r.reason)
		}
	}
	for _, why := range []string{"never-dispatched", "in-progress", "live-worktree", "untracked", "modified", "name-taken"} {
		var names []string
		for _, r := range plan {
			if !r.move && r.reason == why {
				names = append(names, r.task)
			}
		}
		if len(names) == 0 {
			continue
		}
		listed := ""
		for _, n := range names {
			listed += n + " "
		}
		if (why == "never-dispatched" || why == "in-progress") && !list {
			a.printf("  skip  %d %s\n", len(names), why)
		} else {
			a.printf("  skip  %d %s: %s\n", len(names), why, listed)
		}
	}
	if nStale > 0 {
		a.printf("  %d to move (%d of them stale)\n", nMove, nStale)
	} else {
		a.printf("  %d to move\n", nMove)
	}
	if nMove == 0 || !apply {
		return 0
	}

	top, ok := briefsGit(brain, "rev-parse", "--show-toplevel")
	top = strings.TrimRight(top, "\n")
	if !ok {
		a.die("briefs: " + brain + " has no git toplevel")
	}
	top, _ = briefsRealpath(top)
	tmp, err := os.MkdirTemp("", "hw-briefs-ops.")
	if err != nil {
		a.die("briefs: cannot make a temp directory")
	}
	defer os.RemoveAll(tmp)
	relLane, ok := briefsRel(top, brain+"/"+lane)
	if !ok {
		os.RemoveAll(tmp)
		a.die("briefs: " + brain + "/" + lane + " is outside the checkout at " + top)
	}
	var ops []briefsOp
	rowsBy := map[string]string{}
	var months []string
	for _, r := range plan {
		if !r.move {
			continue
		}
		if _, seen := rowsBy[r.ym]; !seen {
			rowsBy[r.ym] = ""
			months = append(months, r.ym)
		}
		src := relLane + "/briefs/" + r.task + ".md"
		dst := relLane + "/briefs/archive/" + r.ym + "/" + r.task + ".md"
		h := extRun(extCall{Name: "git", Args: []string{"-C", top, "hash-object", "--", src}})
		if h.RC != 0 {
			a.die("briefs: cannot hash " + src)
		}
		ops = append(ops, briefsOp{"mv", src, dst})
		rowsBy[r.ym] += src + "\t" + strings.TrimRight(h.Out, "\n") + "\t" + r.reason + "\n"
	}
	sort.Strings(months)
	var manifests []string
	for i, m := range months {
		mrel := relLane + "/briefs/archive/" + m + "/MANIFEST.tsv"
		body := ""
		if b, err := os.ReadFile(top + "/" + mrel); err == nil {
			body = string(b)
		}
		f := tmp + "/manifest." + itoa(i)
		os.WriteFile(f, []byte(body+rowsBy[m]), 0o644)
		ops = append(ops, briefsOp{"put", mrel, f})
		manifests = append(manifests, brain+"/"+lane+"/briefs/archive/"+m+"/MANIFEST.tsv")
	}
	msg := "docs(briefs): archive " + itoa(nMove) + " finished " + lane + " briefs (" + strings.Join(months, ",") + ")"
	commit, ok := a.briefsCommitOps(top, msg, ops)
	if !ok {
		os.RemoveAll(tmp)
		briefsExit(1)
	}
	a.ok("archived " + itoa(nMove) + " " + lane + " briefs: " + commit)
	undo := "hw briefs unarchive " + lane
	for _, m := range manifests {
		undo += " --manifest " + m
	}
	a.info("undo: " + undo + "   (returns EVERY brief those manifests list, earlier runs of the month included; to undo only this run: git revert " + commit + ")")
	return 0
}

// briefsRestore is _briefs_restore.
func (a *app) briefsRestore(args []string) int {
	lane, task := "", ""
	for _, s := range args {
		switch {
		case s == "-h" || s == "--help":
			a.printf("%s", briefsUsageText)
			return 0
		case strings.HasPrefix(s, "-"):
			a.die("unknown option: " + s + " (usage: hw briefs restore <lane> <task>)")
		case lane == "":
			lane = a.c.t.canonProject(s)
		case task == "":
			task = s
		default:
			a.die("hw briefs restore takes <lane> <task> (got extra: " + s + ")")
		}
	}
	if lane == "" || task == "" {
		a.die("usage: hw briefs restore <lane> <task>")
	}
	hit := (&dsp{app: a}).brArchivedPath(lane, task)
	if hit == "" {
		a.die("no archived brief for " + lane + ":" + task + " under " + a.c.brain + "/" + lane + "/briefs/archive/")
	}
	a.briefsMoveBack(lane, "restore "+lane+"/"+task+" from the archive", hit, nil)
	return 0
}

// briefsUnarchive is _briefs_unarchive.
func (a *app) briefsUnarchive(args []string) int {
	lane := ""
	var manifests []string
	for i := 0; i < len(args); i++ {
		switch s := args[i]; {
		case s == "--manifest":
			if i+1 >= len(args) {
				a.die("--manifest needs a value")
			}
			manifests = append(manifests, args[i+1])
			i++
		case s == "-h" || s == "--help":
			a.printf("%s", briefsUsageText)
			return 0
		case strings.HasPrefix(s, "-"):
			a.die("unknown option: " + s + " (usage: hw briefs unarchive <lane> --manifest <file>)")
		default:
			if lane != "" {
				a.die("hw briefs unarchive takes one lane (got: " + lane + ", " + s + ")")
			}
			lane = a.c.t.canonProject(s)
		}
	}
	if lane == "" || len(manifests) == 0 {
		a.die("usage: hw briefs unarchive <lane> --manifest <file> [--manifest <file> ...]")
	}
	a.briefsMoveBack(lane, "unarchive "+itoa(len(manifests))+" manifest(s) of "+lane, "", manifests)
	return 0
}

// briefsDirname/Base: the shell's.
func briefsRealDir(p string) string {
	d, ok := briefsRealpath(filepath.Dir(p))
	if !ok {
		return filepath.Dir(p)
	}
	return d
}

// briefsMoveBack is _briefs_move_back: the engine of restore and unarchive. ALL OR NOTHING: every problem is
// gathered before anything moves, and a problem refuses the whole run.
func (a *app) briefsMoveBack(lane, what, single string, manifests []string) {
	brain := a.c.brain
	if _, ok := briefsGit(brain, "rev-parse", "--show-toplevel"); !ok {
		a.die("briefs: " + brain + " is not a git checkout")
	}
	top, ok := briefsGit(brain, "rev-parse", "--show-toplevel")
	top = strings.TrimRight(top, "\n")
	if !ok {
		a.die("briefs: " + brain + " has no git toplevel")
	}
	top, _ = briefsRealpath(top)
	relLane, ok := briefsRel(top, brain+"/"+lane)
	if !ok {
		a.die("briefs: " + brain + "/" + lane + " is outside the checkout at " + top)
	}
	bd, ok := briefsRealpath(brain + "/" + lane + "/briefs")
	if !ok || !isDir(bd) {
		a.die("no briefs directory for " + lane)
	}
	adir := bd + "/archive"
	var problems []string
	type fileRow struct{ src, mf, blob string }
	var files []fileRow
	if single != "" {
		mdir := briefsRealDir(single)
		blob := ""
		if b, err := os.ReadFile(mdir + "/MANIFEST.tsv"); err == nil {
			want := relLane + "/briefs/" + filepath.Base(single)
			for _, ln := range strings.Split(string(b), "\n") {
				f := strings.Split(ln, "\t")
				if f[0] == want {
					if len(f) > 1 {
						blob = f[1]
					}
					break
				}
			}
		}
		files = append(files, fileRow{mdir + "/" + filepath.Base(single), mdir + "/MANIFEST.tsv", blob})
	} else {
		for _, mf0 := range manifests {
			if !isFile(mf0) {
				problems = append(problems, "  manifest not found: "+mf0)
				continue
			}
			mf := briefsRealDir(mf0) + "/" + filepath.Base(mf0)
			mid := strings.TrimSuffix(strings.TrimPrefix(mf, adir+"/"), "/MANIFEST.tsv")
			if !strings.HasPrefix(mf, adir+"/") || !strings.HasSuffix(mf, "/MANIFEST.tsv") || strings.Contains(mid, "/") {
				problems = append(problems, "  not a manifest of "+lane+"'s archive ("+adir+"/<yyyy-mm>/MANIFEST.tsv): "+mf)
				continue
			}
			body, _ := os.ReadFile(mf)
			for _, ln := range briefsReadLines(string(body)) {
				path, blob := briefsTabFields(ln)
				if path == "" {
					continue
				}
				if !strings.HasPrefix(path, relLane+"/briefs/") || !strings.HasSuffix(path, ".md") {
					problems = append(problems, "  "+mf+" lists a path outside "+relLane+"/briefs/: "+path)
					continue
				}
				files = append(files, fileRow{filepath.Dir(mf) + "/" + filepath.Base(path), mf, blob})
			}
		}
	}

	var ops []briefsOp
	for _, f := range files {
		base := filepath.Base(f.src)
		dst := relLane + "/briefs/" + base
		if !isFile(f.src) {
			problems = append(problems, "  archived file is missing: "+f.src)
			continue
		}
		rs, _ := briefsRel(top, f.src)
		if r := extRun(extCall{Name: "git", Args: []string{"-C", top, "ls-files", "--error-unmatch", "--", rs}}); r.RC != 0 {
			problems = append(problems, "  archived file is not committed: "+f.src)
			continue
		}
		if _, err := os.Lstat(top + "/" + dst); err == nil {
			problems = append(problems, "  would overwrite: "+top+"/"+dst)
			continue
		}
		if f.blob != "" {
			h := extRun(extCall{Name: "git", Args: []string{"-C", top, "hash-object", "--", f.src}})
			if strings.TrimRight(h.Out, "\n") != f.blob {
				a.warn("briefs: " + base + " changed since it was archived (blob differs from its manifest row); it is restored as it is now")
			}
		}
		ops = append(ops, briefsOp{"mv", rs, dst}) // bash anchor 812-M03 (registered on the bash source)
	}
	if len(problems) > 0 {
		n := 0
		for _, p := range problems {
			for _, l := range strings.Split(p, "\n") {
				if strings.TrimSpace(l) != "" {
					n++
				}
			}
		}
		a.die("briefs: nothing was moved back; " + itoa(n) + " problem(s):\n" + strings.Join(problems, "\n"))
	}
	if len(ops) == 0 {
		a.die("briefs: nothing to move back")
	}

	// the manifests: rows of what moved back are dropped; a manifest left empty is deleted
	var mpaths []string
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			mpaths = append(mpaths, p)
		}
	}
	if single != "" {
		add(briefsRealDir(single) + "/MANIFEST.tsv")
	} else {
		for _, m := range manifests {
			add(briefsRealDir(m) + "/" + filepath.Base(m))
		}
	}
	sort.Strings(mpaths)
	gone := map[string]bool{}
	for _, o := range ops {
		gone[o.b] = true
	}
	tmp, err := os.MkdirTemp("", "hw-briefs-ops.")
	if err != nil {
		a.die("briefs: cannot make a temp directory")
	}
	idx := 0
	nMv := len(ops)
	for _, m := range mpaths {
		mrel, ok := briefsRel(top, m)
		if !ok || !isFile(m) {
			continue
		}
		idx++
		body, _ := os.ReadFile(m)
		var keep strings.Builder
		lines := strings.Split(string(body), "\n") // awk reads a last line that has no newline too
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for _, ln := range lines {
			first, _, _ := strings.Cut(ln, "\t")
			if !gone[first] {
				keep.WriteString(ln + "\n")
			}
		}
		if keep.Len() > 0 {
			f := tmp + "/manifest." + itoa(idx)
			os.WriteFile(f, []byte(keep.String()), 0o644)
			ops = append(ops, briefsOp{"put", mrel, f})
		} else {
			ops = append(ops, briefsOp{"del", mrel, ""})
		}
	}
	commit, ok := a.briefsCommitOps(top, "docs(briefs): "+what, ops)
	os.RemoveAll(tmp)
	if !ok {
		briefsExit(1)
	}
	// month directories the move emptied are not part of any tree; leave none behind
	dirs := map[string]bool{}
	for _, m := range mpaths {
		dirs[filepath.Dir(m)] = true
	}
	for d := range dirs {
		os.Remove(d)
	}
	os.Remove(adir)
	a.ok("moved " + itoa(nMv) + " " + lane + " brief(s) back: " + commit)
}

// briefsReadLines is the lines `while read` would see: a last line with no newline is not read.
func briefsReadLines(s string) []string {
	parts := strings.Split(s, "\n")
	return parts[:len(parts)-1]
}

// briefsTabFields is `IFS=$'\t' read -r path blob reason` for the first two fields (tab is IFS whitespace: runs
// collapse and the ends are trimmed).
func briefsTabFields(ln string) (string, string) {
	f := strings.FieldsFunc(ln, func(r rune) bool { return r == '\t' })
	switch len(f) {
	case 0:
		return "", ""
	case 1:
		return f[0], ""
	}
	return f[0], f[1]
}
