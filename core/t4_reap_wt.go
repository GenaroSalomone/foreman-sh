package main

// The verdict on a worktree: bin/hw's _wt_disposition and what it asks (_wt_lock_reason, _wt_irreplaceable,
// _wt_copy_of_main, _wt_branch_merged, _wt_pr_merged, _leased_rundir_for). A verdict that is not `safe` is a reason
// to KEEP; `safe` is the allowlist and everything that could not be measured is `undetermined`.

import (
	"os"
	"os/exec"
	"strings"
)

const reapApplyRegenerable = ".agents .atl .claude .codex .opencode .hw .specify .venv .ruff_cache .pytest_cache .mypy_cache __pycache__ node_modules dist build .next .turbo .DS_Store AGENTS.md PROGRESS.md opencode.json local next-env.d.ts tsconfig.tsbuildinfo tsconfig.build.tsbuildinfo .astro"
const reapApplyArchivable = ".artifacts qa-report test-results playwright-report"

func (r *reapApplyRun) regenerable() string {
	return reapApplyRegenerable + " " + r.a.getenv("HW_WT_REGENERABLE")
}

func (r *reapApplyRun) set(verdict, why string) { r.verdict, r.why = verdict, why }

// disposition is _wt_disposition: it sets verdict, why and the merge evidence.
func (r *reapApplyRun) disposition(main, wt, branch, base string) {
	if base == "" {
		base = "main"
	}
	r.set("", "")
	r.evidence = ""

	occ := r.occ.of(wt)
	if !occ.observed { // # MUTATION-ANCHOR: 865-G07
		r.set("undetermined", "could not read who is working in it — "+occ.why+". An unread occupant list is not an empty one, and occupancy is the gate that outranks every other here.")
		return
	}
	if occ.list != "" { // # MUTATION-ANCHOR: 865-G08
		r.set("held", "held by "+strings.ReplaceAll(occ.list, "\n", " "))
		return
	}
	if lr, ok := r.leasedRundirFor(wt); ok { // # MUTATION-ANCHOR: 865-G09
		r.set("leased", "preview leased until "+reapApplyLeaseField(lr, "until_h")+" — "+reapApplyLeaseField(lr, "why"))
		return
	}
	if lock, ok := r.lockReason(main, wt); ok { // # MUTATION-ANCHOR: 865-G10
		reason := ""
		if lock != "" {
			reason = " (" + lock + ")"
		}
		r.set("locked", "locked"+reason+" — hw locks a worktree while its executor lives, and an idle one is invisible to lsof; `hw done` unlocks its own, `git worktree unlock` any other")
		return
	}
	if branch == "" || branch == "detached" {
		r.set("detached", "no branch — a detached worktree is not something this tool decides about")
		return
	}
	statusOut, statusErr, rc := reapApplyGit(wt, "status", "--porcelain")
	if rc != 0 {
		if reapApplyGitRC(wt, "rev-parse", "--git-dir") != 0 {
			if branch != "" && reapApplyGitRC(main, "show-ref", "--verify", "--quiet", "refs/heads/"+branch) == 0 {
				r.set("undetermined", wt+" is not readable as a git worktree, but "+branch+" EXISTS in "+main+" — so this is a worktree whose .git was lost, not a directory that never was one. What it holds has not been measured and its branch has not been merged.")
				return
			}
			r.set("not-a-worktree", wt+" is not a git worktree at all and no branch "+branch+" exists — it is a plain directory, which is what a --worktree none task and every pre-2026-09-16 setup task leaves behind. There is nothing here for git to dispose of.")
			return
		}
		r.set("undetermined", "git status exited "+itoa(rc)+" in "+wt+" — "+trimTR(statusErr, 200)+". Empty output from a FAILED status is the same bytes as a clean tree; it is not a clean tree.")
		return
	}
	if status := bashSub(statusOut); status != "" { // # MUTATION-ANCHOR: 865-G11
		r.set("dirty", itoa(strings.Count(status, "\n")+1)+" uncommitted tracked change(s)")
		return
	}
	if !r.branchMerged(main, branch, base) { // # MUTATION-ANCHOR: 865-G12
		n := ""
		if o, _, rc := reapApplyGit(main, "rev-list", "--count", "origin/"+base+".."+branch); rc == 0 {
			n = bashSub(o)
		} else if o, _, rc := reapApplyGit(main, "rev-list", "--count", base+".."+branch); rc == 0 {
			n = bashSub(o)
		} else {
			n = "?"
		}
		r.set("unmerged", branch+" has "+n+" commit(s) not in "+base)
		return
	}
	keep := bashSub(r.irreplaceable(wt))
	// ARCHIVABLE: every kept entry is a task output under a WT_ARCHIVABLE name; an unread one (" (") never is
	r.archive = ""
	if keep != "" {
		other := false
		var entries []string
		for _, rel := range reapApplyLines(keep) {
			first := rel
			if i := strings.IndexByte(rel, '/'); i >= 0 {
				first = rel[:i]
			}
			if strings.Contains(rel, " (") {
				other = true
				break
			}
			if strings.Contains(" "+reapApplyArchivable+" ", " "+first+" ") {
				entries = append(entries, rel)
			} else {
				other = true
				break
			}
		}
		if !other && len(entries) > 0 { // # MUTATION-ANCHOR: 865-G13
			r.archive = strings.Join(entries, "\n")
			ev := r.evidence
			if ev == "" {
				ev = "merged into " + base
			}
			r.set("archivable", ev+" and clean; its only ignored content is task output, archived before removal: "+strings.ReplaceAll(r.archive, "\n", " "))
			return
		}
		r.archive = ""
	}
	if keep != "" {
		r.set("irreplaceable", "git-ignored and unreproducible: "+strings.ReplaceAll(keep, "\n", " "))
		return
	}
	if r.evidence != "" {
		r.set("safe", r.evidence+"; clean, nothing ignored that git cannot rebuild")
	} else {
		r.set("safe", "merged into "+base+", clean, nothing ignored that git cannot rebuild")
	}
}

// lockReason is _wt_lock_reason: the reason a worktree is locked (maybe empty), and whether it is locked at all.
func (r *reapApplyRun) lockReason(main, wt string) (string, bool) {
	real, ok := reapApplyPwdP(wt)
	if !ok {
		real = wt
	}
	listing, _, _ := reapApplyGit(main, "worktree", "list", "--porcelain")
	here, found, reason := false, false, ""
	for _, l := range strings.Split(listing, "\n") {
		switch {
		case strings.HasPrefix(l, "worktree "):
			w := l[len("worktree "):]
			here = w == wt || w == real
		case here && strings.HasPrefix(l, "locked"):
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

// irreplaceable is _wt_irreplaceable: the ignored top-level entries that are not on the allowlist and hold something.
func (r *reapApplyRun) irreplaceable(wt string) string {
	ignored, _, rc := reapApplyGit(wt, "status", "--porcelain", "--ignored")
	if rc != 0 {
		return "<the ignored-file list could not be read — git status --ignored exited " + itoa(rc) + ">\n"
	}
	var rows []string
	for _, l := range strings.Split(ignored, "\n") {
		if strings.HasPrefix(l, "!! ") {
			rows = append(rows, strings.TrimSuffix(l[3:], "/"))
		}
	}
	var out strings.Builder
	regen := " " + r.regenerable() + " "
	for _, rel := range reapApplyLines(reapApplySort(strings.Join(rows, "\n")+"\n", "-u")) {
		skip := false
		for _, comp := range strings.Fields(strings.ReplaceAll(rel, "/", " ")) {
			if strings.Contains(regen, " "+comp+" ") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if r.copyOfMain(wt, rel) {
			continue
		}
		entry := wt + "/" + rel
		if reapApplyIsLink(entry) { // A symlink loses nothing when removed
			continue
		}
		if isDir(entry) {
			fdOut, frc := reapApplyFd("-H", "-t", "f", ".", entry)
			if frc != 0 {
				out.WriteString(rel + " (could not be read — fd exited " + itoa(frc) + ")\n")
			} else if bashSub(fdOut) != "" {
				out.WriteString(rel + "\n")
			}
		} else if reapApplyExists(entry) {
			out.WriteString(rel + "\n")
		}
	}
	return out.String()
}

// copyOfMain is _wt_copy_of_main: a copy whose original the main checkout still holds is not unreproducible.
func (r *reapApplyRun) copyOfMain(wt, rel string) bool {
	lane, main := "", ""
	for _, c := range r.a.c.t.names {
		if len(r.ex.copies[c]) == 0 {
			continue
		}
		m := r.a.c.checkout(c)
		if m == "" {
			continue
		}
		if strings.HasPrefix(wt, m+"/") {
			lane, main = c, m
			break
		}
	}
	if lane == "" {
		return false
	}
	matched := false
	for _, c := range r.ex.copies[lane] {
		if c == "" {
			continue
		}
		if reapApplyShellMatch(c, rel) {
			matched = true
			break
		}
	}
	if from := r.ex.copiesFrom[lane]; !matched && from != "" {
		_, pats, _ := r.seedPatterns(main + "/" + from)
		for _, pat := range pats {
			if pat == "" {
				continue
			}
			if reapApplyShellMatch(pat, rel) {
				matched = true
				break
			}
		}
	}
	if !matched {
		return false
	}
	return isFile(main + "/" + rel)
}

// branchMerged is _wt_branch_merged: ancestry first, then the CONTENT of what the branch contributed, then a merged PR.
func (r *reapApplyRun) branchMerged(main, branch, base string) bool {
	if base == "" {
		base = "main"
	}
	if reapApplyGitRC(main, "merge-base", "--is-ancestor", branch, "origin/"+base) == 0 {
		return true
	}
	if reapApplyGitRC(main, "merge-base", "--is-ancestor", branch, base) == 0 { // # MUTATION-ANCHOR: 865-G15
		return true
	}
	ref := ""
	if reapApplyGitRC(main, "rev-parse", "--verify", "--quiet", "origin/"+base) == 0 {
		ref = "origin/" + base
	} else if reapApplyGitRC(main, "rev-parse", "--verify", "--quiet", base) == 0 {
		ref = base
	} else {
		return false
	}
	mbOut, _, _ := reapApplyGit(main, "merge-base", ref, branch)
	mb := bashSub(mbOut)
	if mb == "" {
		return false
	}
	// A branch that changed nothing is not evidence of a merge.
	names, _, _ := reapApplyGit(main, "diff", "-z", "--name-only", mb, branch)
	if strings.ReplaceAll(names, "\x00", "") == "" {
		return false
	}
	names2, _, _ := reapApplyGit(main, "diff", "-z", "--name-only", mb, branch)
	var paths []string
	for _, p := range strings.Split(names2, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	if len(paths) > 0 {
		args := append([]string{"diff", "--quiet", ref, branch, "--"}, paths...)
		if reapApplyGitRC(main, args...) == 0 { // # MUTATION-ANCHOR: 865-G16
			r.evidence = "squash-merged: every path it contributed is already identical in " + base
			return true
		}
	}
	return r.prMerged(main, branch, base)
}

// prList is _wt_pr_list: every PR of the repository, once per repository.
func (r *reapApplyRun) prList(main string) bool {
	if r.prsMain == main {
		return r.prsOK
	}
	r.prsMain, r.prsOK, r.prs = main, false, ""
	gh := r.a.getenv("HW_GH")
	if gh == "" {
		gh = "gh"
	}
	if _, err := lookPath(gh); err != nil {
		return false
	}
	url, _, rc := reapApplyGit(main, "remote", "get-url", "origin")
	if rc != 0 {
		return false
	}
	if !strings.Contains(bashSub(url), "github.com") {
		return false
	}
	args := []string{"pr", "list", "--state", "all", "--limit", "1000", "--json", "headRefName,state,headRefOid,baseRefName", "--jq", `.[] | [.headRefName, .state, .headRefOid, .baseRefName] | @tsv`}
	name := gh
	for _, to := range []string{"timeout", "gtimeout"} {
		if _, err := lookPath(to); err == nil {
			name, args = to, append([]string{"60", gh}, args...)
			break
		}
	}
	out, _, rc := reapApplyExec(main, "", name, args...)
	if rc != 0 {
		return false
	}
	r.prs, r.prsOK = bashSub(out), true
	return true
}

// prMerged is _wt_pr_merged: the branch tip is in the head of a PR merged into the lane's base, and no PR of it is open.
func (r *reapApplyRun) prMerged(main, branch, base string) bool {
	if !r.prList(main) {
		return false
	}
	tipOut, _, rc := reapApplyGit(main, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if rc != 0 {
		return false
	}
	tip := bashSub(tipOut)
	var rows [][]string
	for _, l := range strings.Split(r.prs, "\n") {
		f := strings.Split(l, "\t")
		if f[0] == branch {
			rows = append(rows, f)
		}
	}
	if len(rows) == 0 {
		return false
	}
	for _, f := range rows {
		if len(f) > 1 && f[1] == "OPEN" {
			return false
		}
	}
	for _, f := range rows {
		for len(f) < 4 {
			f = append(f, "")
		}
		state, oid, prbase := f[1], f[2], f[3]
		if !(state == "MERGED" && oid != "" && prbase == base) {
			continue
		}
		if oid == tip || reapApplyGitRC(main, "merge-base", "--is-ancestor", tip, oid) == 0 {
			r.evidence = "PR-merged: its tip is in the head of a PR merged into " + base
			return true
		}
	}
	return false
}

func lookPath(name string) (string, error) { return exec.LookPath(name) }

// ── leases ─────────────────────────────────────────────────────────────────────

func reapApplyLeaseField(rundir, key string) string { // _lease_field: the first `key=` line, sed 's/^key=//p' | head -1
	data, err := os.ReadFile(rundir + "/lease")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, key+"=") {
			return l[len(key)+1:]
		}
	}
	return ""
}

// leaseState is _lease_state: 0 live, 1 none, 2 expired.
func (r *reapApplyRun) leaseState(rundir string) int {
	f, err := os.Open(rundir + "/lease")
	if err != nil {
		return 1
	}
	f.Close()
	until := reapApplyLeaseField(rundir, "until")
	if !reapApplyAllDigits(until) {
		return 2
	}
	if reapApplyNow() < reapApplyAtoi(until) {
		return 0
	}
	return 2
}

// leasedRundirFor is _leased_rundir_for: the run of a worktree that holds a live preview lease.
func (r *reapApplyRun) leasedRundirFor(wt string) (string, bool) {
	if !isDir(wt + "/.hw") {
		return "", false
	}
	listed, _ := reapApplyFd("-H", "-t", "d", "-d", "1", `^2[0-9]{7}-[0-9]{6}-[0-9]+$`, wt+"/.hw")
	for _, rd := range reapApplyLines(reapApplySort(listed, "-r")) {
		if r.leaseState(rd) == 0 {
			return rd, true
		}
	}
	return "", false
}
