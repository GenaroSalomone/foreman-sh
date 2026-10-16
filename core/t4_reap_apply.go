package main

// What `--apply` does once the verdict is in: the removal of a worktree (and the archive, the branch, the database
// that go with it), the shed of a kept one, the retention log, the databases whose worktree is gone, and the closing of an
// executor that reported BLOCKED and was never answered. lib/hw/reap.sh, function by function.

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ── the removal ────────────────────────────────────────────────────────────────

func (r *reapApplyRun) archiveDest(proj, wt string) string { // _reap_archive_dest: a path that does not exist yet
	d := r.archiveRoot() + "/" + proj + "/" + reapApplyBasename(wt)
	if reapApplyExists(d) {
		d += "." + reapApplyDate("+%Y%m%d-%H%M%S", "20060102-150405")
	}
	return d
}

// archiveEntries is _reap_archive_entries (and `hw done`'s doneArchiveEntries, which is the same function): each copy is
// verified, not assumed.
func (r *reapApplyRun) archiveEntries(wt, dest string, entries []string) bool {
	return archiveVerified(wt, dest, entries, r.a.warn, func(src, dst string) bool {
		return reapApplyRC("diff", "-q", src, dst) == 0 // # MUTATION-ANCHOR: 865-G17
	})
}

// archiveVerified verifies as the worktree HOLDS the tree, never as a path resolves it (`diff -rq` follows links: an
// absolute link back into the worktree is a loop, a dangling one is missing, a socket never compares equal). A link is
// copied as a link and compared by its target text; a socket, fifo or device cannot be archived, so it is named and left
// out of both sides. `same` is the arbiter of the two manifests (`diff -q`), so a stub that fails it refuses the copy.
func archiveVerified(wt, dest string, entries []string, warn func(string), same func(src, dst string) bool) bool {
	m, err := os.MkdirTemp("", "hw-reap-manifest.")
	if err != nil {
		return false
	}
	defer os.RemoveAll(m)
	for _, rel := range entries {
		if rel == "" {
			continue
		}
		if reapApplyRC("mkdir", "-p", dest+"/"+reapApplyDirname(rel)) != 0 {
			return false
		}
		specials, _ := reapApplyTree(wt, rel)
		for _, sp := range specials {
			warn("skipped " + sp.path + " — a " + sp.kind + " cannot be archived, it goes with its entry")
		}
		// -P: a link is copied AS a link. A special file may make cp say so; the manifests are the judge of the copy.
		if reapApplyRC("cp", "-pPR", wt+"/"+rel, dest+"/"+rel) != 0 && len(specials) == 0 {
			return false
		}
		if len(specials) > 0 {
			copied, _ := reapApplyTree(dest, rel)
			for _, sp := range copied {
				os.Remove(dest + "/" + sp.path)
			}
		}
		_, src := reapApplyTree(wt, rel)
		_, dst := reapApplyTree(dest, rel)
		if src == nil || dst == nil || os.WriteFile(m+"/src", []byte(strings.Join(src, "\n")+"\n"), 0o600) != nil ||
			os.WriteFile(m+"/dst", []byte(strings.Join(dst, "\n")+"\n"), 0o600) != nil {
			return false
		}
		if !same(m+"/src", m+"/dst") {
			return false
		}
	}
	return true
}

type reapApplySpecial struct{ path, kind string }

// reapApplyTree walks root/rel WITHOUT following a link: the special files it holds (sorted, named by their path from
// root) and the manifest of everything else — a directory, a link WITH its target text, a file with its digest —
// sorted bytewise (LC_ALL=C). A nil manifest is a walk that failed.
func reapApplyTree(root, rel string) ([]reapApplySpecial, []string) {
	var specials []reapApplySpecial
	lines := []string{}
	err := filepath.WalkDir(root+"/"+rel, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, _ := filepath.Rel(root, p)
		mode := d.Type()
		switch {
		case mode&os.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			lines = append(lines, "l "+name+" -> "+t)
		case mode.IsDir():
			lines = append(lines, "d "+name)
		case mode.IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return err
			}
			lines = append(lines, fmt.Sprintf("f %s %x", name, h.Sum(nil)))
		default:
			kind := "device"
			if mode&os.ModeSocket != 0 {
				kind = "socket"
			} else if mode&os.ModeNamedPipe != 0 {
				kind = "fifo"
			}
			specials = append(specials, reapApplySpecial{name, kind})
		}
		return nil
	})
	if err != nil {
		return specials, nil
	}
	sort.Strings(lines)
	sort.Slice(specials, func(i, j int) bool { return specials[i].path < specials[j].path })
	return specials, lines
}

// rmWorkdir is _reap_rm_workdir: a read-only candidate included, and its own failure reported.
func (r *reapApplyRun) rmWorkdir(dir string) bool {
	reapApplyRC("chmod", "-R", "u+w", dir)
	if reapApplyRC("rm", "-rf", dir) != 0 {
		return false
	}
	return !reapApplyExists(dir)
}

// reapWorktree is _reap_worktree: it acts on the verdict the caller JUST computed, and only on `safe` or `archivable`.
func (r *reapApplyRun) reapWorktree(proj, main, wt, branch, base string) bool {
	a := r.a
	dest := ""
	r.archiveDir = ""
	switch r.verdict { // # MUTATION-ANCHOR: 865-G18
	case "safe", "archivable":
	default:
		return false
	}
	if r.verdict == "archivable" {
		entries := r.archive
		dest = r.archiveDest(proj, wt)
		if !r.archiveEntries(wt, dest, reapApplyLines(entries)) {
			// The partial copy goes: the originals are untouched
			r.rmWorkdir(dest)
			a.warn("kept " + reapApplyBasename(wt) + " — the copy into " + dest + " could not be verified, so nothing was removed")
			return false
		}
		r.archiveDir = dest
		a.ok("archived " + strings.ReplaceAll(entries, "\n", " ") + "→ " + dest)
		for _, rel := range reapApplyLines(entries) {
			r.rmWorkdir(wt + "/" + rel)
		}
		r.disposition(main, wt, branch, base)
		if r.verdict != "safe" {
			a.warn("kept " + reapApplyBasename(wt) + " — with its outputs archived it reads " + r.verdict + ": " + r.why)
			return false
		}
	}
	// the verdict excused a tracked AGENTS.md that is only next dev's block; `worktree remove` does not (lib/hw/reap.sh)
	if r.agentsBlock { // # MUTATION-ANCHOR: 865-G41
		reapApplyGitRC(wt, "checkout", "--", "AGENTS.md")
	}
	if reapApplyGitRC(main, "worktree", "remove", wt) != 0 {
		a.warn("could not remove " + wt + " — run it by hand and read what git says")
		return false
	}
	a.ok("removed " + reapApplyBasename(wt))
	if r.deleteBranch(main, branch, base) {
		a.info("deleted " + branch)
	} else {
		a.warn("kept " + branch + " — git refused a safe delete, so something is not merged after all")
	}
	r.reapDB(proj, main, wt, dest)
	return true
}

// deleteBranch is _reap_delete_branch: `-d` first, `-D` only on the squash/PR evidence asked again right here.
func (r *reapApplyRun) deleteBranch(main, branch, base string) bool {
	if reapApplyGitRC(main, "branch", "-d", branch) == 0 {
		return true
	}
	r.evidence = ""
	if !(r.branchMerged(main, branch, base) && r.evidence != "") {
		return false
	}
	return reapApplyGitRC(main, "branch", "-D", branch) == 0
}

// orphanBranches is _reap_orphan_branches: the lane's task branches no worktree holds, merged into the base.
func (r *reapApplyRun) orphanBranches(proj, main, base string) []string {
	a := r.a
	branchOf := func(task string) string {
		if !a.c.t.known(proj) {
			return "task/" + task
		}
		return a.c.lanePath(proj, "branch", task)
	}
	whole := branchOf("X")
	prefix := strings.TrimSuffix(whole, "X")
	if prefix == "" || prefix == whole {
		return nil
	}
	listing, _, lrc := reapApplyGit(main, "worktree", "list", "--porcelain")
	if lrc != 0 { // `held="$(git … | sed …)" || return 0` under pipefail: a failed list ends the pass before the for-each-ref
		return nil
	}
	held := map[string]bool{}
	for _, l := range strings.Split(listing, "\n") {
		if strings.HasPrefix(l, "branch refs/heads/") {
			held[l[len("branch refs/heads/"):]] = true
		}
	}
	refs, _, _ := reapApplyGit(main, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+prefix)
	var out []string
	for _, b := range reapApplyLines(refs) {
		if held[b] {
			continue
		}
		if r.branchMerged(main, b, base) {
			out = append(out, b)
		}
	}
	return out
}

// ── the database ───────────────────────────────────────────────────────────────

func reapApplyUser() string { // id -un
	o, rc := reapApplyOut("id", "-un")
	if rc != 0 {
		return ""
	}
	return bashSub(o)
}

// dbExists is `psql … "select 1 from pg_database where datname = '<db>'" | grep -q 1`.
func (r *reapApplyRun) dbExists(db string) bool {
	o, _ := reapApplyOut("psql", "-U", reapApplyUser(), "-d", "postgres", "-Atc", "select 1 from pg_database where datname = '"+db+"'")
	return strings.Contains(o, "1")
}

var reapApplyPgMajor1 = regexp.MustCompile(`^.* ([0-9]+)[. ].*$`)
var reapApplyPgMajor2 = regexp.MustCompile(`^.* ([0-9]+)$`)

// pgMajor is _pg_major: the major its --version names, empty when unreadable.
func reapApplyPgMajor(tool string) string {
	o, _ := reapApplyOut(tool, "--version")
	for _, l := range strings.Split(o, "\n") {
		if m := reapApplyPgMajor1.FindStringSubmatch(l); m != nil {
			return m[1]
		}
		if m := reapApplyPgMajor2.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// pgTool is _pg_tool: a tool is taken from the major the server runs.
func (r *reapApplyRun) pgTool(tool, want string) string {
	if want != "" {
		dirs := r.a.getenv("HW_PG_BIN_DIRS")
		if dirs == "" {
			dirs = "/opt/homebrew/opt/postgresql@" + want + "/bin:/usr/local/opt/postgresql@" + want + "/bin:/usr/lib/postgresql/" + want + "/bin"
		}
		if c, err := exec.LookPath(tool); err == nil && c != "" && reapApplyPgMajor(c) == want {
			return c
		}
		for dirs != "" {
			d := dirs
			if i := strings.IndexByte(dirs, ':'); i >= 0 {
				d, dirs = dirs[:i], dirs[i+1:]
			} else {
				dirs = ""
			}
			if reapApplyExecutable(d+"/"+tool) && reapApplyPgMajor(d+"/"+tool) == want { // # MUTATION-ANCHOR: 865-G21
				return d + "/" + tool
			}
		}
	}
	return tool
}

// dropDB is _reap_drop_db: DROP DATABASE, once. "" and true when it is gone; the reason and false otherwise.
func (r *reapApplyRun) dropDB(u, db string) (string, bool) {
	o, e, rc := reapApplyExec("", "", "psql", "-U", u, "-d", "postgres", "-q", "-c", `DROP DATABASE "`+db+`"`)
	if rc == 0 {
		return "", true
	}
	// 2>&1: the diagnostic and the output are one text
	errText := bashSub(o + e)
	if strings.Contains(errText, "does not exist") { // # MUTATION-ANCHOR: 865-G22
		return "", true
	}
	who, _ := reapApplyOut("psql", "-U", u, "-d", "postgres", "-Atc", "select coalesce(nullif(usename,''),'?') || '/' || coalesce(nullif(application_name,''),'-') || ' x' || count(*) from pg_stat_activity where datname = '"+db+"' group by usename, application_name")
	who = strings.ReplaceAll(who, "\n", " ")
	if who != "" {
		return "open connections: " + who + "— close them and reap again", false
	}
	first := errText
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	return first, false
}

// reapDB is _reap_db: the worktree's database is dumped, the dump verified, and only then dropped.
func (r *reapApplyRun) reapDB(proj, main, wt, dest string) {
	a := r.a
	if a.c.t.get(proj, "db_provisioned") == "" {
		return
	}
	db := reapApplyDBName(reapApplyBasename(wt))
	mainDB := reapApplyDBName(reapApplyBasename(main))
	if db == mainDB {
		a.warn("kept database " + db + " — it is the main checkout's name")
		return
	}
	u := reapApplyUser()
	if !r.dbExists(db) {
		return
	}
	if dest == "" {
		dest = r.archiveDest(proj, wt)
	}
	if reapApplyRC("mkdir", "-p", dest) != 0 {
		a.warn("kept database " + db + " — could not create " + dest)
		return
	}
	pgv := ""
	if o, _ := reapApplyOut("psql", "-U", u, "-d", "postgres", "-Atc", "show server_version_num"); o != "" {
		var majors []string
		for _, l := range strings.Split(o, "\n") {
			if reapApplyAllDigits(l) {
				majors = append(majors, itoa(int(reapApplyAtoi(l)/10000)))
			}
		}
		pgv = strings.Join(majors, "\n")
	}
	pgd, pgr := r.pgTool("pg_dump", pgv), r.pgTool("pg_restore", pgv)
	_, derrRaw, drc := reapApplyExec("", "", pgd, "-U", u, "-Fc", "-f", dest+"/"+db+".dump", db)
	derr := bashSub(derrRaw)
	st, serr := os.Stat(dest + "/" + db + ".dump")
	verified := drc == 0 && serr == nil && st.Size() > 0 && reapApplyRC(pgr, "-l", dest+"/"+db+".dump") == 0 // # MUTATION-ANCHOR: 865-G23
	if verified {
		r.archiveDir = dest
		if why, dropped := r.dropDB(u, db); dropped {
			a.ok("database " + db + " dumped to " + dest + "/" + db + ".dump and dropped")
		} else {
			a.warn("database " + db + " dumped to " + dest + "/" + db + ".dump but NOT dropped — " + why)
		}
		return
	}
	reapApplyRC("rm", "-f", dest+"/"+db+".dump")
	tail := ""
	if derr != "" {
		last := derr
		if i := strings.LastIndexByte(derr, '\n'); i >= 0 {
			last = derr[i+1:]
		}
		tail = ": " + last
	}
	a.warn("kept database " + db + " — its pg_dump into " + dest + " could not be verified, so it was not dropped" + tail)
}

// ── shed and build output ──────────────────────────────────────────────────────

func (r *reapApplyRun) sheddable() bool { // _wt_sheddable: the verdicts about git
	switch r.verdict {
	case "dirty", "unmerged", "irreplaceable":
		return true
	}
	return false
}

func (r *reapApplyRun) duKB(path string) int64 { // du -sk path | awk '{print $1 + 0}'
	o, _ := reapApplyOut("du", "-sk", path)
	f := strings.Fields(o)
	if len(f) == 0 {
		return 0
	}
	return reapApplyLeadingInt(f[0])
}

// processInside is the awk of _wt_shed and _wt_build_idle: a cwd that is the directory or under it.
func processInside(cwds, real string) bool {
	d := "n" + real
	for _, l := range strings.Split(cwds, "\n") {
		if l == d || strings.HasPrefix(l, d+"/") {
			return true
		}
	}
	return false
}

// shed is _wt_shed: what a KEPT worktree loses is only what its install or build rebuilds, never under a live process.
func (r *reapApplyRun) shed(proj, wt string) (string, bool) {
	real, ok := reapApplyPwdP(wt)
	if !ok {
		return "", false
	}
	root, ok := r.canonicalRoot(proj)
	if !ok {
		return "", false
	}
	if root, ok = reapApplyPwdP(root); !ok {
		return "", false
	}
	if !strings.HasPrefix(real, root+"/") {
		return "", false
	}
	cwds, rc := reapApplyOut("lsof", "-a", "-d", "cwd", "-Fn")
	cwds = bashSub(cwds)
	if rc != 0 && cwds == "" {
		return "", false
	}
	if processInside(cwds, real) { // # MUTATION-ANCHOR: 865-G24
		return "", false
	}
	ignored, _, rc := reapApplyGit(wt, "status", "--porcelain", "--ignored")
	if rc != 0 {
		return "", false
	}
	shedNames := " .next .turbo " + r.a.getenv("HW_WT_SHED") + " "
	var kb int64
	gone := ""
	for _, l := range strings.Split(ignored, "\n") {
		if !strings.HasPrefix(l, "!! ") {
			continue
		}
		rel := strings.TrimSuffix(l[3:], "/")
		leaf := rel
		if i := strings.LastIndexByte(rel, '/'); i >= 0 {
			leaf = rel[i+1:]
		}
		if !strings.Contains(shedNames, " "+leaf+" ") {
			continue
		}
		if !isDir(wt+"/"+rel) || reapApplyIsLink(wt+"/"+rel) {
			continue
		}
		k := r.duKB(wt + "/" + rel)
		if reapApplyRC("rm", "-rf", wt+"/"+rel) != 0 {
			continue
		}
		kb += k
		gone += " " + rel
	}
	if gone == "" {
		return "", false
	}
	return strings.TrimPrefix(gone, " ") + " (" + itoa(int(kb/1024)) + " MB)", true
}

// buildIdle is _wt_build_idle: the build output of a kept worktree idle past its window (sets buildDirs, buildKB, buildAge).
func (r *reapApplyRun) buildIdle(proj, wt string) bool {
	r.buildDirs, r.buildKB, r.buildAge = "", 0, 0
	win := r.buildWindow()
	if win == "0" {
		return false
	}
	switch r.verdict { // # MUTATION-ANCHOR: 865-G25
	case "dirty", "unmerged", "irreplaceable":
	default:
		return false
	}
	real, ok := reapApplyPwdP(wt)
	if !ok {
		return false
	}
	root, ok := r.canonicalRoot(proj)
	if !ok {
		return false
	}
	if root, ok = reapApplyPwdP(root); !ok {
		return false
	}
	if !strings.HasPrefix(real, root+"/") {
		return false
	}
	cwds, rc := reapApplyOut("lsof", "-Fn", "-a", "-d", "cwd")
	cwds = bashSub(cwds)
	if rc != 0 && cwds == "" { // # MUTATION-ANCHOR: 865-G26
		return false
	}
	if processInside(cwds, real) { // # MUTATION-ANCHOR: 865-G35
		return false
	}
	listed, _ := reapApplyFd("-H", "-I", "-t", "d", "-d", "4", "-E", "node_modules", `^\.(next|turbo)$`, wt)
	dirs := ""
	var kb, newest int64
	for _, d := range reapApplyLines(listed) {
		d = strings.TrimSuffix(d, "/")
		if d == "" || !isDir(d) || reapApplyIsLink(d) {
			continue
		}
		if reapApplyGitRC(wt, "check-ignore", "-q", d) != 0 {
			continue
		}
		act, frc := reapApplyOut("find", d, "-type", "f", "-mtime", "-"+win, "-print", "-quit")
		if frc != 0 || act != "" { // # MUTATION-ANCHOR: 865-G27
			return false
		}
		dirs += d + "\n"
		kb += r.duKB(d)
		st, err := os.Stat(d)
		if err != nil {
			return false
		}
		if m := st.ModTime().Unix(); m > newest {
			newest = m
		}
	}
	if dirs == "" {
		return false
	}
	// git's own last touch counts too: the reflog, not the index
	fo, _, _ := reapApplyGit(wt, "rev-parse", "--git-path", "logs/HEAD")
	if f := bashSub(fo); f != "" && reapApplyExists(f) { // `[ -e "$f" ]` as the shell asks it: where hw is, not in the worktree
		if !strings.HasPrefix(f, "/") {
			f = wt + "/" + f
		}
		act, frc := reapApplyOut("find", f, "-mtime", "-"+win, "-print")
		if frc != 0 || act != "" {
			return false
		}
	}
	age := (reapApplyNow() - newest) / 86400
	if w := reapApplyAtoi(win); age < w {
		age = w
	}
	r.buildDirs, r.buildKB, r.buildAge = dirs, kb, age
	return true
}

// buildDirsShown is `printf '%s' "$WT_BUILD_DIRS" | sed "s|^$wt/||" | tr '\n' ' '`.
func (r *reapApplyRun) buildDirsShown(wt string) string {
	re, err := regexp.Compile("^" + regexp.QuoteMeta(wt) + "/")
	var parts []string
	for _, d := range strings.Split(strings.TrimSuffix(r.buildDirs, "\n"), "\n") {
		if err == nil {
			d = re.ReplaceAllString(d, "")
		}
		parts = append(parts, d)
	}
	return strings.Join(parts, " ") + " "
}

// logRetention is _reap_log_retention: one tab-separated line per removal by retention (age and reason from WD_*).
func (r *reapApplyRun) logRetention(lane, path string, kb int64) bool {
	line := reapApplyDate("+%Y-%m-%dT%H:%M:%S%z", "2006-01-02T15:04:05-0700") + "\t" + lane + "\t" + path + "\t" + reapApplyBasename(path) + "\t" +
		r.wdRetAge + "d\t" + itoa(int(kb)) + "KB\t" + r.wdWhy + "\n"
	return reapApplyAppend(r.logFile(), line)
}

func reapApplyAppend(file, text string) bool {
	h, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
	if err != nil {
		return false
	}
	defer h.Close()
	_, err = h.WriteString(text)
	return err == nil
}

// ── blocked executors ──────────────────────────────────────────────────────────

func reapApplyDoneMarker(rundir string) string { // _run_done_marker: the current task's
	seq := ""
	if data, err := os.ReadFile(rundir + "/task"); err == nil {
		for _, c := range string(data) {
			if c >= '0' && c <= '9' {
				seq += string(c)
			}
		}
	}
	switch seq {
	case "", "0", "1":
		return rundir + "/done"
	}
	return rundir + "/t" + seq + "/done"
}

func reapApplyBlockedMarker(rundir string) string { // _run_blocked_marker
	m := reapApplyDoneMarker(rundir)
	return strings.TrimSuffix(m, "/done") + "/blocked-waiting"
}

func reapApplyReopenedMarker(rundir string) string { // _run_reopened_marker
	m := reapApplyDoneMarker(rundir)
	return strings.TrimSuffix(m, "/done") + "/reopened"
}

// reapApplyBlockedField is _blocked_field: awk -F= '$1==k {print substr($0, index($0,"=")+1); exit}'.
func reapApplyBlockedField(marker, key string) string {
	data, err := os.ReadFile(marker)
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		f1 := l
		if i := strings.IndexByte(l, '='); i >= 0 {
			f1 = l[:i]
		}
		if f1 == key {
			if i := strings.IndexByte(l, '='); i >= 0 {
				return l[i+1:]
			}
			return l
		}
	}
	return ""
}

// blockedSecondsLeft is _blocked_seconds_left; 0 = expired.
func (r *reapApplyRun) blockedSecondsLeft(marker string) int64 {
	since := reapApplyBlockedField(marker, "since")
	var s int64
	if reapApplyAllDigits(since) {
		s = reapApplyAtoi(since)
	} else if st, err := os.Stat(marker); err == nil {
		s = st.ModTime().Unix()
	} else {
		s = reapApplyNow()
	}
	// the clock is HW_ST_NOW when it is a whole number of seconds (bash: `now="${HW_ST_NOW:-}"`, else `date +%s`): the
	// shadow gives its two runs one reading, and a run that reads it does not call `date`
	now := int64(0)
	if v := r.a.getenv("HW_ST_NOW"); reapApplyAllDigits(v) {
		now = reapApplyAtoi(v)
	} else {
		now = reapApplyNow()
	}
	left := s + reapApplyAtoi(r.blockedWaitHours())*3600 - now
	if left <= 0 {
		return 0
	}
	return left
}

var reapApplyRunName = regexp.MustCompile(`^2[0-9]{7}-[0-9]{6}-[0-9]+$`)

// runDirsEveryLane is `_hw_run_dirs --every-lane`: $WORK (what fd finds) and each lane's worktree root outside it.
func (r *reapApplyRun) runDirsEveryLane() []string {
	a := r.a
	work := a.c.work
	var roots []string
	for _, lane := range a.c.t.survey {
		sr := a.c.lanePath(lane, "worktree_root", "")
		if sr == "" || sr == work || strings.HasPrefix(sr, work+"/") {
			continue
		}
		roots = append(roots, sr)
	}
	listed, _ := reapApplyFd("-H", "-t", "d", `^2[0-9]{7}-[0-9]{6}-[0-9]+$`, work)
	out := reapApplyLines(listed)
	for _, sr := range roots {
		for _, rd := range reapApplyGlob(sr + "/*/.hw/2*") {
			if isDir(rd) && reapApplyRunName.MatchString(rd[strings.LastIndexByte(rd, '/')+1:]) {
				out = append(out, rd)
			}
		}
	}
	return out
}

// closeBlocked is `"$HW_BIN_DIR/hw" done <project> <task> --blocked`: the same entry point, the same command line, with
// the child's own output going where it goes in bash.
func (r *reapApplyRun) closeBlocked(proj, task string) bool {
	bin := r.a.getenv("HW_CORE_BIN_DIR")
	if bin == "" {
		bin = r.a.c.brain + "/bin"
	}
	cmd := exec.Command(bin+"/hw", "done", proj, task, "--blocked")
	cmd.Stdout, cmd.Stderr = r.a.term.out, r.a.term.err
	cmd.Stdin = r.a.stdin
	return cmd.Run() == nil
}
