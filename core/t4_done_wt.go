package main

// What `hw done` decides about the task's worktree and does with it: _wt_disposition and the gates it asks
// (_wt_occupants, _leased_rundir_for, _wt_lock_reason, _wt_branch_merged, _wt_pr_merged, _wt_irreplaceable,
// _wt_copy_of_main; bin/hw), and the removal it shares with `hw reap --apply` (_reap_worktree, _reap_db,
// _reap_delete_branch, _wt_shed; lib/hw/reap.sh). Every git, psql and pg_dump call is the bash's, in order.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

const doneWtRegenerable = ".agents .atl .claude .codex .opencode .hw .specify .venv .ruff_cache .pytest_cache .mypy_cache __pycache__ node_modules dist build .next .turbo .DS_Store AGENTS.md PROGRESS.md opencode.json local next-env.d.ts tsconfig.tsbuildinfo tsconfig.build.tsbuildinfo .astro"
const doneWtArchivable = ".artifacts qa-report test-results playwright-report"

// doneLockReason is _wt_lock_reason: whether <main>'s worktree list has <wt> locked, and the reason it gives.
func doneLockReason(main, wt string) (string, bool) {
	real := wt
	if p, ok := donePhysical(wt); ok {
		real = p
	}
	out, _ := doneGit(main, "worktree", "list", "--porcelain")
	here, found, reason := false, false, ""
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if strings.HasPrefix(l, "worktree ") {
			w := l[len("worktree "):]
			here = w == wt || w == real
		}
		if here && strings.HasPrefix(l, "locked") {
			found = true
			reason = ""
			if len(l) > 7 {
				reason = l[7:]
			}
			if len(reason) >= 2 && reason[0] == '"' && reason[len(reason)-1] == '"' {
				reason = reason[1 : len(reason)-1]
			}
		}
	}
	return reason, found
}

// leasedRundir is _leased_rundir_for: a run under <wt>/.hw whose preview lease has not expired.
func (d *doneRun) leasedRundir(wt string) (string, bool) {
	if !isDir(wt + "/.hw") {
		return "", false
	}
	for _, id := range RunIDs(wt + "/.hw") {
		r := wt + "/.hw/" + id + "/"
		f := r + "/lease"
		if !readableFile(f) {
			continue
		}
		until := doneFirstKey(f, "until")
		if !doneAllDigits(until) {
			continue
		}
		if doneNow() < doneAtoi(until) { // # MUTATION-ANCHOR: 861-G31
			return r, true
		}
	}
	return "", false
}

// disposition is _wt_disposition: sets verdict and why. Everything that is not `safe` is a reason to KEEP.
func (d *doneRun) disposition(main, wt, branch, base string) {
	d.verdict, d.why, d.evidence = "", "", ""
	occ := d.occ.of(wt)
	if !occ.observed {
		d.verdict = "undetermined"
		d.why = "could not read who is working in it — " + occ.why + ". An unread occupant list is not an empty one, and occupancy is the gate that outranks every other here."
		return
	}
	if occ.list != "" { // # MUTATION-ANCHOR: 861-G32
		d.verdict = "held"
		d.why = "held by " + strings.ReplaceAll(occ.list, "\n", " ")
		return
	}
	if lr, ok := d.leasedRundir(wt); ok {
		d.verdict = "leased"
		d.why = "preview leased until " + doneFirstKey(lr+"/lease", "until_h") + " — " + doneFirstKey(lr+"/lease", "why")
		return
	}
	if lockr, ok := doneLockReason(main, wt); ok {
		d.verdict = "locked"
		paren := ""
		if lockr != "" {
			paren = " (" + lockr + ")"
		}
		d.why = "locked" + paren + " — hw locks a worktree while its executor lives, and an idle one is invisible to lsof; `hw done` unlocks its own, `git worktree unlock` any other"
		return
	}
	if branch == "" || branch == "detached" {
		d.verdict = "detached"
		d.why = "no branch — a detached worktree is not something this tool decides about"
		return
	}
	statusOut, statusErr, rc := doneGitStatusErr(wt)
	if rc != 0 {
		if _, grc := doneGit(wt, "rev-parse", "--git-dir"); grc != 0 {
			if branch != "" {
				if _, src := doneGit(main, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); src == 0 {
					d.verdict = "undetermined"
					d.why = wt + " is not readable as a git worktree, but " + branch + " EXISTS in " + main + " — so this is a worktree whose .git was lost, not a directory that never was one. What it holds has not been measured and its branch has not been merged."
					return
				}
			}
			d.verdict = "not-a-worktree"
			d.why = wt + " is not a git worktree at all and no branch " + branch + " exists — it is a plain directory, which is what a --worktree none task and every pre-2026-09-16 setup task leaves behind. There is nothing here for git to dispose of."
			return
		}
		d.verdict = "undetermined"
		d.why = "git status exited " + itoa(rc) + " in " + wt + " — " + trimTR(statusErr, 200) + ". Empty output from a FAILED status is the same bytes as a clean tree; it is not a clean tree."
		return
	}
	statusOut, d.agentsBlock = statusWithoutAgentsBlock(statusOut, func() string {
		o, _ := doneGit(wt, "diff", "--no-color", "--no-ext-diff", "--unified=0", "--", "AGENTS.md")
		return o
	})
	if statusOut != "" { // # MUTATION-ANCHOR: 861-G33
		d.verdict = "dirty"
		d.why = itoa(strings.Count(statusOut, "\n")+1) + " uncommitted tracked change(s)"
		return
	}
	if !d.branchMerged(main, branch, base) {
		out1, rc1 := doneGit(main, "rev-list", "--count", "origin/"+base+".."+branch)
		all := out1
		if rc1 != 0 {
			out2, rc2 := doneGit(main, "rev-list", "--count", base+".."+branch)
			all += out2
			if rc2 != 0 {
				all += "?\n"
			}
		}
		d.verdict = "unmerged"
		d.why = branch + " has " + bashSub(all) + " commit(s) not in " + base
		return
	}
	keep := d.irreplaceable(wt)
	d.archive = nil
	if len(keep) > 0 {
		var arch []string
		other := false
		for _, rel := range keep {
			if rel == "" {
				continue
			}
			first := rel
			if i := strings.Index(rel, "/"); i >= 0 {
				first = rel[:i]
			}
			if strings.Contains(rel, " (") || !strings.Contains(" "+doneWtArchivable+" ", " "+first+" ") {
				other = true
				break
			}
			arch = append(arch, rel)
		}
		if !other && len(arch) > 0 { // # MUTATION-ANCHOR: 861-G34
			d.archive = arch
			d.verdict = "archivable"
			ev := d.evidence
			if ev == "" {
				ev = "merged into " + base
			}
			d.why = ev + " and clean; its only ignored content is task output, archived before removal: " + strings.Join(arch, " ")
			return
		}
		d.archive = nil
	}
	if len(keep) > 0 {
		d.verdict = "irreplaceable"
		d.why = "git-ignored and unreproducible: " + strings.Join(keep, " ")
		return
	}
	d.verdict = "safe"
	if d.evidence != "" {
		d.why = d.evidence + "; clean, nothing ignored that git cannot rebuild"
	} else {
		d.why = "merged into " + base + ", clean, nothing ignored that git cannot rebuild"
	}
}

// doneGitStatusErr is `git -C wt status --porcelain 2>file`: stdout as `$( )` keeps it, stderr, status.
func doneGitStatusErr(wt string) (string, string, int) {
	cmd := doneCmd("git", "-C", wt, "status", "--porcelain")
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	rc := doneExit(cmd.Run())
	return bashSub(out.String()), errb.String(), rc
}

// branchMerged is _wt_branch_merged: ancestry, then content (a squash), then a merged PR.
func (d *doneRun) branchMerged(main, branch, base string) bool {
	if _, rc := doneGit(main, "merge-base", "--is-ancestor", branch, "origin/"+base); rc == 0 {
		return true
	}
	if _, rc := doneGit(main, "merge-base", "--is-ancestor", branch, base); rc == 0 {
		return true
	}
	var ref string
	if _, rc := doneGit(main, "rev-parse", "--verify", "--quiet", "origin/"+base); rc == 0 {
		ref = "origin/" + base
	} else if _, rc := doneGit(main, "rev-parse", "--verify", "--quiet", base); rc == 0 {
		ref = base
	} else {
		return false
	}
	mbOut, _ := doneGit(main, "merge-base", ref, branch)
	mb := bashSub(mbOut)
	if mb == "" {
		return false
	}
	names, _ := doneGit(main, "diff", "-z", "--name-only", mb, branch)
	if strings.ReplaceAll(names, "\x00", "") == "" {
		return false
	}
	names2, _ := doneGit(main, "diff", "-z", "--name-only", mb, branch)
	var paths []string
	for _, n := range strings.Split(names2, "\x00") {
		if n != "" {
			paths = append(paths, n)
		}
	}
	identical := true
	for i := 0; i < len(paths) && identical; i += 500 {
		end := i + 500
		if end > len(paths) {
			end = len(paths)
		}
		if _, rc := doneGit(main, append([]string{"diff", "--quiet", ref, branch, "--"}, paths[i:end]...)...); rc != 0 {
			identical = false
		}
	}
	if identical {
		d.evidence = "squash-merged: every path it contributed is already identical in " + base
		return true
	}
	return d.prMerged(main, branch, base)
}

// prList is _wt_pr_list: the pull requests of main's GitHub origin, asked once per process and repository.
func (d *doneRun) prList(main string) bool {
	if d.prsMain == main {
		return d.prsOK
	}
	d.prsMain, d.prsOK, d.prs = main, false, ""
	gh := d.getenv("HW_GH")
	if gh == "" {
		gh = "gh"
	}
	ghPath, err := doneLookPath(gh)
	if err != nil {
		return false
	}
	url, rc := doneGit(main, "remote", "get-url", "origin")
	if rc != 0 || !strings.Contains(bashSub(url), "github.com") {
		return false
	}
	var argv []string
	if p, err := doneLookPath("timeout"); err == nil {
		argv = []string{p, "60"}
	} else if p, err := doneLookPath("gtimeout"); err == nil {
		argv = []string{p, "60"}
	}
	argv = append(argv, ghPath, "pr", "list", "--state", "all", "--limit", "1000",
		"--json", "headRefName,state,headRefOid,baseRefName",
		"--jq", `.[] | [.headRefName, .state, .headRefOid, .baseRefName] | @tsv`)
	out, rc := doneTool(main, nil, argv[0], argv[1:]...)
	if rc != 0 {
		return false
	}
	d.prs, d.prsOK = bashSub(out), true
	return true
}

// prMerged is _wt_pr_merged.
func (d *doneRun) prMerged(main, branch, base string) bool {
	if !d.prList(main) {
		return false
	}
	tipOut, rc := doneGit(main, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if rc != 0 {
		return false
	}
	tip := bashSub(tipOut)
	var rows [][]string
	for _, l := range strings.Split(d.prs, "\n") {
		f := strings.Split(l, "\t")
		if f[0] == branch {
			for len(f) < 4 {
				f = append(f, "")
			}
			rows = append(rows, f)
		}
	}
	if len(rows) == 0 {
		return false
	}
	for _, f := range rows {
		if f[1] == "OPEN" {
			return false
		}
	}
	for _, f := range rows {
		state, oid, prbase := f[1], f[2], f[3]
		if state != "MERGED" || oid == "" || prbase != base {
			continue
		}
		if oid == tip {
			d.evidence = "PR-merged: its tip is in the head of a PR merged into " + base
			return true
		}
		if _, rc := doneGit(main, "merge-base", "--is-ancestor", tip, oid); rc == 0 {
			d.evidence = "PR-merged: its tip is in the head of a PR merged into " + base
			return true
		}
	}
	return false
}

// irreplaceable is _wt_irreplaceable: the ignored entries git cannot rebuild, one per entry.
func (d *doneRun) irreplaceable(wt string) []string {
	ignored, rc := doneGit(wt, "status", "--porcelain", "--ignored")
	if rc != 0 {
		return []string{"<the ignored-file list could not be read — git status --ignored exited " + itoa(rc) + ">"}
	}
	var rows []string
	for _, l := range strings.Split(bashSub(ignored)+"\n", "\n") {
		if strings.HasPrefix(l, "!! ") {
			rows = append(rows, strings.TrimSuffix(l[3:], "/"))
		}
	}
	rows = doneSortUnique(rows)
	regen := " " + doneWtRegenerable + " " + d.getenv("HW_WT_REGENERABLE") + " "
	var keep []string
	for _, rel := range rows {
		if rel == "" {
			continue
		}
		isRegen := false
		for _, comp := range strings.Fields(strings.ReplaceAll(rel, "/", " ")) {
			if strings.Contains(regen, " "+comp+" ") {
				isRegen = true
				break
			}
		}
		if isRegen || d.copyOfMain(wt, rel) {
			continue
		}
		entry := wt + "/" + rel
		if st, err := os.Lstat(entry); err == nil && st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if isDir(entry) {
			out, rc := doneTool("", nil, "fd", "-H", "-t", "f", ".", entry)
			if rc != 0 {
				keep = append(keep, rel+" (could not be read — fd exited "+itoa(rc)+")")
			} else if bashSub(out) != "" {
				keep = append(keep, rel)
			}
		} else if doneExists(entry) {
			keep = append(keep, rel)
		}
	}
	return keep
}

// doneSortUnique is `sort -u` in the caller's locale.
func doneSortUnique(items []string) []string {
	if len(items) == 0 {
		return items
	}
	cmd := doneCmd("sort", "-u")
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return items
	}
	s := strings.TrimSuffix(string(out), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

var doneReCopyOne = regexp.MustCompile(`_copy_one "\$wt" "\$rel" "\$dry"`)

// seedPatterns is _wt_seed_patterns: the glob wt.sh copies with, read from wt.sh itself.
func doneSeedPatterns(wtSh string) []string {
	b, err := os.ReadFile(wtSh)
	if err != nil {
		return nil
	}
	for _, l := range strings.Split(string(b), "\n") {
		if doneReCopyOne.MatchString(l) {
			pat := strings.TrimLeft(l, " \t\r\n\f\v")
			if i := strings.Index(pat, ")"); i >= 0 {
				pat = pat[:i]
			}
			if pat == "" {
				return nil
			}
			return strings.Split(pat, "|")
		}
	}
	return nil
}

// copyOfMain is _wt_copy_of_main: a path the lane copies into every worktree whose original main still holds.
func (d *doneRun) copyOfMain(wt, rel string) bool {
	main, lane := "", ""
	for _, c := range d.c.t.names {
		if d.x.get(c, "reap_copies") == "" {
			continue
		}
		m := d.c.checkout(c)
		if m == "" {
			continue
		}
		if strings.HasPrefix(wt, m+"/") {
			main, lane = m, c
			break
		}
	}
	if lane == "" {
		return false
	}
	matched := false
	for _, c := range strings.Split(d.x.get(lane, "reap_copies"), "\n") {
		if c != "" && doneGlobMatch(c, rel) {
			matched = true
			break
		}
	}
	if from := d.x.get(lane, "reap_copies_from"); !matched && from != "" {
		for _, p := range doneSeedPatterns(main + "/" + from) {
			if p != "" && doneGlobMatch(p, rel) {
				matched = true
				break
			}
		}
	}
	return matched && isFile(main+"/"+rel)
}

// doneGlobMatch is `case $s in $pat)`: * and ? cross slashes, [..] is a class.
func doneGlobMatch(pat, s string) bool {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pat); i++ {
		switch c := pat[i]; c {
		case '*':
			sb.WriteString("(?s:.*)")
		case '?':
			sb.WriteString("(?s:.)")
		case '[':
			j := i + 1
			if j < len(pat) && (pat[j] == '!' || pat[j] == '^') {
				j++
			}
			if j < len(pat) && pat[j] == ']' {
				j++
			}
			for j < len(pat) && pat[j] != ']' {
				j++
			}
			if j >= len(pat) {
				sb.WriteString(regexp.QuoteMeta("["))
				continue
			}
			class := pat[i+1 : j]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			sb.WriteString("[" + class + "]")
			i = j
		case '\\':
			if i+1 < len(pat) {
				i++
				sb.WriteString(regexp.QuoteMeta(string(pat[i])))
			}
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	return err == nil && re.MatchString(s)
}

// ── the removal ─────────────────────────────────────────────────────────────

// archiveRoot is _archive_root.
func (d *doneRun) archiveRoot() string {
	if r := d.getenv("HW_ARCHIVE_ROOT"); r != "" {
		return r
	}
	return doneDirname(d.c.work) + "/archive"
}

// archiveDest is _reap_archive_dest: a path that does not exist yet.
func (d *doneRun) archiveDest(proj, wt string) string {
	dest := d.archiveRoot() + "/" + proj + "/" + doneBasename(wt)
	if _, err := os.Lstat(dest); err == nil || doneExists(dest) {
		stamp, _ := doneDate("+%Y%m%d-%H%M%S")
		dest += "." + stamp
	}
	return dest
}

// doneArchiveEntries is _reap_archive_entries: the same verified copy `hw reap --apply` makes (archiveVerified).
func doneArchiveEntries(wt, dest string, entries []string, warn func(string)) bool {
	return archiveVerified(wt, dest, entries, warn, func(src, dst string) bool {
		_, rc := doneTool("", nil, "diff", "-q", src, dst)
		return rc == 0 // # MUTATION-ANCHOR: 861-G36
	})
}

// doneRmWorkdir is _reap_rm_workdir: made writable, removed, and its absence checked.
func doneRmWorkdir(dir string) bool {
	doneTool("", nil, "chmod", "-R", "u+w", dir)
	if os.RemoveAll(dir) != nil {
		return false
	}
	_, err := os.Lstat(dir)
	return err != nil
}

// reapWorktree is _reap_worktree: acts on the verdict just computed, and only on safe or archivable.
func (d *doneRun) reapWorktree(proj, main, wt, branch, base string) bool {
	a := d.app
	d.archiveDir = ""
	dest := ""
	if d.verdict != "safe" && d.verdict != "archivable" {
		return false
	}
	if d.verdict == "archivable" {
		entries := d.archive
		dest = d.archiveDest(proj, wt)
		if !doneArchiveEntries(wt, dest, entries, a.warn) {
			doneRmWorkdir(dest)
			a.warn("kept " + doneBasename(wt) + " — the copy into " + dest + " could not be verified, so nothing was removed")
			return false
		}
		d.archiveDir = dest
		a.ok("archived " + strings.Join(entries, " ") + "→ " + dest)
		for _, rel := range entries {
			if rel != "" {
				doneRmWorkdir(wt + "/" + rel)
			}
		}
		d.disposition(main, wt, branch, base)
		if d.verdict != "safe" { // # MUTATION-ANCHOR: 861-G37
			a.warn("kept " + doneBasename(wt) + " — with its outputs archived it reads " + d.verdict + ": " + d.why)
			return false
		}
	}
	if d.agentsBlock {
		doneGit(wt, "checkout", "--", "AGENTS.md")
	}
	if _, rc := doneTool("", nil, "git", "-C", main, "worktree", "remove", wt); rc != 0 {
		a.warn("could not remove " + wt + " — run it by hand and read what git says")
		return false
	}
	a.ok("removed " + doneBasename(wt))
	if d.deleteBranch(main, branch, base) {
		a.info("deleted " + branch)
	} else {
		a.warn("kept " + branch + " — git refused a safe delete, so something is not merged after all")
	}
	d.reapDB(proj, main, wt, dest)
	return true
}

// deleteBranch is _reap_delete_branch: `-d`, and `-D` only on a squash- or PR-merge asked again here.
func (d *doneRun) deleteBranch(main, branch, base string) bool {
	if _, rc := doneGit(main, "branch", "-d", branch); rc == 0 {
		return true
	}
	d.evidence = ""
	if !d.branchMerged(main, branch, base) || d.evidence == "" {
		return false
	}
	_, rc := doneGit(main, "branch", "-D", branch)
	return rc == 0
}

var (
	donePgMajor1 = regexp.MustCompile(`^.* ([0-9][0-9]*)[. ].*$`)
	donePgMajor2 = regexp.MustCompile(`^.* ([0-9][0-9]*)$`)
)

// pgMajor is _pg_major: the major a tool's --version names.
func donePgMajor(tool string) string {
	out, _ := doneTool("", nil, tool, "--version")
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if m := donePgMajor1.FindStringSubmatch(l); m != nil {
			return m[1]
		}
		if m := donePgMajor2.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// pgTool is _pg_tool: the tool of the server's major, else PATH's own.
func (d *doneRun) pgTool(tool, want string) string {
	if want != "" {
		dirs := d.getenv("HW_PG_BIN_DIRS")
		if dirs == "" {
			dirs = "/opt/homebrew/opt/postgresql@" + want + "/bin:/usr/local/opt/postgresql@" + want + "/bin:/usr/lib/postgresql/" + want + "/bin"
		}
		if c, err := doneLookPath(tool); err == nil && donePgMajor(c) == want {
			return c
		}
		for _, dd := range strings.Split(dirs, ":") {
			if dd == "" {
				continue
			}
			p := dd + "/" + tool
			if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 && donePgMajor(p) == want {
				return p
			}
		}
	}
	return tool
}

// reapDB is _reap_db: on a lane with `db.provisioned`, the worktree's database dumped, checked, then dropped.
func (d *doneRun) reapDB(proj, main, wt, dest string) {
	a := d.app
	if a.c.t.get(proj, "db_provisioned") == "" {
		return
	}
	tr := strings.NewReplacer("-", "_", "/", "_")
	db, mainDB := tr.Replace(doneBasename(wt)), tr.Replace(doneBasename(main))
	if db == mainDB {
		a.warn("kept database " + db + " — it is the main checkout's name")
		return
	}
	u := doneWhoami()
	out, rc := doneTool("", nil, "psql", "-U", u, "-d", "postgres", "-Atc", "select 1 from pg_database where datname = '"+db+"'")
	if rc != 0 || !strings.Contains(out, "1") {
		return
	}
	if dest == "" {
		dest = d.archiveDest(proj, wt)
	}
	if os.MkdirAll(dest, 0o777) != nil {
		a.warn("kept database " + db + " — could not create " + dest)
		return
	}
	vout, _ := doneTool("", nil, "psql", "-U", u, "-d", "postgres", "-Atc", "show server_version_num")
	pgv := ""
	for _, l := range strings.Split(vout, "\n") {
		if doneAllDigits(l) {
			n, _ := strconv.ParseInt(l, 10, 64)
			pgv += strconv.FormatInt(n/10000, 10) + "\n"
		}
	}
	pgv = bashSub(pgv)
	pgd, pgr := d.pgTool("pg_dump", pgv), d.pgTool("pg_restore", pgv)
	dump := dest + "/" + db + ".dump"
	c := doneCmd(pgd, "-U", u, "-Fc", "-f", dump, db)
	var errb strings.Builder
	c.Stderr = &errb
	derrRC := doneExit(c.Run())
	derr := bashSub(errb.String())
	verified := false
	if derrRC == 0 {
		if st, err := os.Stat(dump); err == nil && st.Size() > 0 {
			if _, rc := doneTool("", nil, pgr, "-l", dump); rc == 0 {
				verified = true
			}
		}
	}
	if verified {
		d.archiveDir = dest
		if why, ok := doneDropDB(u, db); ok {
			a.ok("database " + db + " dumped to " + dump + " and dropped")
		} else {
			a.warn("database " + db + " dumped to " + dump + " but NOT dropped — " + why)
		}
		return
	}
	os.Remove(dump)
	tail := ""
	if derr != "" {
		ls := strings.Split(derr, "\n")
		tail = ": " + ls[len(ls)-1]
	}
	a.warn("kept database " + db + " — its pg_dump into " + dest + " could not be verified, so it was not dropped" + tail)
}

// doneDropDB is _reap_drop_db: DROP DATABASE once; why it did not go, naming who holds it open.
func doneDropDB(u, db string) (string, bool) {
	errOut, rc := doneCapture("psql", "-U", u, "-d", "postgres", "-q", "-c", "DROP DATABASE \""+db+"\"")
	if rc == 0 {
		return "", true
	}
	if strings.Contains(errOut, "does not exist") {
		return "", true
	}
	who, _ := doneTool("", nil, "psql", "-U", u, "-d", "postgres", "-Atc", "select coalesce(nullif(usename,''),'?') || '/' || coalesce(nullif(application_name,''),'-') || ' x' || count(*) from pg_stat_activity where datname = '"+db+"' group by usename, application_name")
	who = strings.ReplaceAll(who, "\n", " ")
	if who != "" {
		return "open connections: " + who + "— close them and reap again", false
	}
	return t4FirstLine(errOut), false
}

// ── shed: the weight of a worktree that stays ───────────────────────────────

func (d *doneRun) sheddable() bool {
	switch d.verdict {
	case "dirty", "unmerged", "irreplaceable":
		return true
	}
	return false
}

// shed is _wt_shed: the git-ignored build output of a kept worktree, removed when nothing runs in it.
func (d *doneRun) shed(proj, wt string) (string, bool) {
	real, ok := donePhysical(wt)
	if !ok {
		return "", false
	}
	probe := d.c.lanePath(proj, "worktree", "__probe__")
	if probe == "" {
		return "", false
	}
	root, ok := donePhysical(doneDirname(probe))
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(real, root+"/") {
		return "", false
	}
	cwds, rc := doneTool("", nil, "lsof", "-a", "-d", "cwd", "-Fn")
	cwds = bashSub(cwds)
	if rc != 0 && cwds == "" {
		return "", false
	}
	for _, l := range strings.Split(cwds, "\n") {
		if l == "n"+real || strings.HasPrefix(l, "n"+real+"/") { // # MUTATION-ANCHOR: 861-G38
			return "", false
		}
	}
	ignored, rc := doneGit(wt, "status", "--porcelain", "--ignored")
	if rc != 0 {
		return "", false
	}
	shedSet := " .next .turbo " + d.getenv("HW_WT_SHED") + " "
	var kb int64
	gone := ""
	for _, l := range strings.Split(bashSub(ignored)+"\n", "\n") {
		if !strings.HasPrefix(l, "!! ") {
			continue
		}
		rel := strings.TrimSuffix(l[3:], "/")
		if !strings.Contains(shedSet, " "+rel[strings.LastIndexByte(rel, '/')+1:]+" ") {
			continue
		}
		p := wt + "/" + rel
		if st, err := os.Lstat(p); err != nil || !st.IsDir() {
			continue
		}
		duOut, _ := doneTool("", nil, "du", "-sk", p)
		k := int64(0)
		if f := strings.Fields(t4FirstLine(duOut)); len(f) > 0 {
			k = doneAtoi(f[0])
		}
		if os.RemoveAll(p) != nil {
			continue
		}
		kb += k
		gone += " " + rel
	}
	if gone == "" {
		return "", false
	}
	return strings.TrimPrefix(gone, " ") + " (" + strconv.FormatInt(kb/1024, 10) + " MB)", true
}
