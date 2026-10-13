package main

// The verdict on a loose work dir (what `git worktree list` cannot see) and the retention of what a reported task
// left: bin/hw's _workdir_disposition, _run_reported, _retention_expired, _artifacts_dir_for. THE BIAS IS RETAIN: every gate
// that cannot be measured answers `undetermined`, never `safe`, because `safe` is an `rm -rf` of the only copy.

import (
	"os"
	"path/filepath"
	"strings"
)

// reapApplyGlob is a shell glob: in the locale's order (`sort`), where filepath.Glob is in bytes.
func reapApplyGlob(pattern string) []string {
	m, _ := filepath.Glob(pattern)
	return sortLocale(m, false)
}

func (r *reapApplyRun) wdSet(verdict, why string) { r.wdVerdict, r.wdWhy = verdict, why }

// artifactsDirFor is _artifacts_dir_for: where a task's artifacts are, resolved from the lane AND the directory.
func (r *reapApplyRun) artifactsDirFor(proj, wd string) string {
	a := r.a
	if a.c.checkout(proj) == "" { // _lane_has_repo
		return wd
	}
	root := a.c.lanePath(proj, "worktree_root", "")
	if root != "" && root == a.c.work+"/"+proj && !reapApplyExists(wd+"/.git") { // lane_worktree_is_workdir
		return wd
	}
	return wd + "/.artifacts"
}

// runReported is _run_reported: the newest run of the directory carries its current task's done marker, was not reopened
// and is not a BLOCKED report still waiting for a ruling.
func (r *reapApplyRun) runReported(dir string) bool {
	newest := ""
	for _, rd := range reapApplyGlob(dir + "/.hw/2[0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]-*") {
		if !isDir(rd) {
			continue
		}
		if newest == "" || rd > newest {
			newest = rd
		}
	}
	if newest == "" {
		return false
	}
	if !isFile(reapApplyDoneMarker(newest)) {
		return false
	}
	if isFile(reapApplyReopenedMarker(newest)) {
		return false
	}
	if isFile(reapApplyBlockedMarker(newest)) {
		return false
	}
	return true
}

func reapApplyIsBackup(p string) bool { // _artifact_is_backup
	b := strings.ToLower(reapApplyBasename(p))
	return strings.HasSuffix(b, ".tgz") || strings.HasSuffix(b, ".tar.gz") || strings.HasSuffix(b, ".tar") || strings.Contains(b, "backup")
}

// retentionExpired is _retention_expired: is EVERY artifact past its own window? Keep (false) on anything unmeasured.
func (r *reapApplyRun) retentionExpired(files string) bool {
	r.retAge, r.retWin, r.retKind = "", "", ""
	art := r.windowDays("HW_ARTIFACT_RETENTION_DAYS", r.ex.retArt, "30")
	bak := r.windowDays("HW_BACKUP_RETENTION_DAYS", r.ex.retBak, "60")
	now := reapApplyNow()
	best := int64(-1)
	for _, f := range reapApplyLines(files) {
		win, kind := art, "artifact"
		if reapApplyIsBackup(f) { // # MUTATION-ANCHOR: 865-G29
			win, kind = bak, "backup"
		}
		if win == "0" {
			return false
		}
		st, err := os.Stat(f)
		if err != nil {
			return false
		}
		age := (now - st.ModTime().Unix()) / 86400
		if !(age > reapApplyAtoi(win)) { // # MUTATION-ANCHOR: 865-G30
			return false
		}
		slack := age - reapApplyAtoi(win)
		if best < 0 || slack < best {
			best = slack
			r.retAge, r.retWin, r.retKind = itoa(int(age)), win, kind
		}
	}
	return best >= 0
}

// workdirDisposition is _workdir_disposition: it sets wdVerdict, wdWhy and wdRetAge.
func (r *reapApplyRun) workdirDisposition(proj, dir string) {
	r.wdSet("", "")
	r.wdRetAge = ""
	expired := false

	if !isDir(dir) {
		r.wdSet("undetermined", dir+" is not a directory")
		return
	}
	// GIT FIRST: anything git can see belongs to the pass above and is not ruled on twice
	phys, ok := reapApplyPwdP(dir)
	if !ok {
		phys = dir
	}
	topOut, _, rc := reapApplyGit(dir, "rev-parse", "--show-toplevel")
	top := bashSub(topOut)
	if rc == 0 {
		if reapApplyExists(dir+"/.git") || top == dir || top == phys {
			r.wdSet("worktree", "a git worktree — the worktree pass owns it and this one does not reclassify it")
		} else {
			r.wdSet("inside-worktree", "not a loose work dir: it sits inside the git worktree "+top+", and what happens to it is git's question, not this one's")
		}
		return
	}
	if rc != 128 {
		r.wdSet("undetermined", "git rev-parse --show-toplevel exited "+itoa(rc)+" in "+dir+", so whether git owns this directory was never established — 128 is the only exit that means 'not a repository'")
		return
	}

	// THE SAME THREE-VALUED OCCUPANCY GATE AS A WORKTREE'S
	occ := r.occ.of(dir)
	if !occ.observed {
		r.wdSet("undetermined", "could not read who is working in it — "+occ.why)
		return
	}
	if occ.list != "" { // # MUTATION-ANCHOR: 865-G36
		r.wdSet("held", "held by "+strings.ReplaceAll(occ.list, "\n", " "))
		return
	}
	if lr, ok := r.leasedRundirFor(dir); ok { // # MUTATION-ANCHOR: 865-G37
		r.wdSet("leased", "preview leased until "+reapApplyLeaseField(lr, "until_h")+" — "+reapApplyLeaseField(lr, "why"))
		return
	}

	// WHAT THE TASK PRODUCED, read from where the task was TOLD to put it
	adir := r.artifactsDirFor(proj, dir) // # MUTATION-ANCHOR: 865-G32
	if isDir(adir) {
		files, frc := reapApplyFd("-H", "-t", "f", ".", adir)
		if frc != 0 && frc != 1 {
			r.wdSet("undetermined", "could not read the artifact store "+adir+" — fd exited "+itoa(frc)+". An unreadable directory is not an empty one, and this is the gate that decides whether anything is being destroyed")
			return
		}
		files = bashSub(files)
		if adir == dir { // .hw is not an artifact, even when it sits inside the artifact store
			var keep []string
			for _, l := range strings.Split(files, "\n") {
				if !strings.HasPrefix(l, dir+"/.hw/") {
					keep = append(keep, l)
				}
			}
			files = strings.Join(keep, "\n")
		}
		n := 0
		for _, l := range strings.Split(files, "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
		if n != 0 {
			// RETENTION: reported AND every artifact past its window; anything else is the old verdict
			if r.runReported(dir) && r.retentionExpired(files) { // # MUTATION-ANCHOR: 865-G33
				expired = true
			} else {
				r.wdSet("artifacts", itoa(n)+" file(s) under its artifact store "+adir+" — artifacts are the one thing a work dir exists to hold, and nothing here can rebuild them")
				return
			}
		}
	}

	// STILL MOVING. A dispatch seconds old looks exactly like abandoned garbage from the outside.
	if st, err := os.Stat(dir); err == nil {
		if st.ModTime().UnixNano() > (reapApplyNow()-86400)*1e9 { // # MUTATION-ANCHOR: 865-G34
			r.wdSet("recent", "modified within the last 24h — too young to call abandoned")
			return
		}
	}
	// AND THE TREE, not just the root
	recent, rrc := reapApplyFd("-H", "--changed-within", "24h", ".", dir)
	if rrc != 0 && rrc != 1 {
		r.wdSet("undetermined", "could not read the directory's contents to date them — fd exited "+itoa(rrc))
		return
	}
	if strings.TrimSpace(recent) != "" {
		r.wdSet("recent", "something under it was written within the last 24h — too young to call abandoned")
		return
	}

	r.wdSet("safe", "")
	if expired {
		r.wdRetAge = r.retAge
		r.wdWhy = "retention: " + r.retAge + "d > " + r.retWin + "d (" + r.retKind + " window); its task reported, nobody in it, not a git worktree"
		return
	}
	r.wdWhy = "no artifacts, not a git worktree, nothing under it touched in 24h, nobody in it"
}
