package main

// TWO LIVE TASKS EDITING THE SAME FILE. 2026-11-04: two live tasks edited core/invokers/transcript_pi.go and
// setup/pi/hw-executor.ts at the same time — that cost rulings, a rebase and retired code. At dispatch hw now WARNS
// when what this brief names, or what this task's own branch has already changed, overlaps the paths another live
// task's unmerged `task/*` branch has changed. It never refuses: a warning is a fact the brainer reads before launching,
// and two tasks may legitimately share a file.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// brPathRe is a repo path as a brief spells it: a known top-level directory and at least one component under it.
var brPathRe = regexp.MustCompile(`\b(?:bin|core|setup|docs|lanes|lib|scripts|shell|tools|guards|_skills|_agents)/[A-Za-z0-9._@+-]+(?:/[A-Za-z0-9._@+-]+)*`)

// brNamedPaths is the repo paths a brief body names, deduplicated and in order (a trailing `.`, `,`, `:` or `)` is
// prose, not a path).
func brNamedPaths(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range brPathRe.FindAllString(body, -1) {
		m = strings.TrimRight(m, ".,;:)]>`'\"")
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// taskOverlap is one other live task and the paths it shares with this one.
type taskOverlap struct {
	task  string
	paths []string
}

// liveTaskOverlaps is the fact the dispatch warns about: every OTHER live task worktree of the lane (a worktree under
// <work>/<proj>/<task> on branch task/<task>) whose branch changed a path in `mine`, against `base`. A worktree whose
// branch cannot be read as a diff is skipped, never guessed at.
func liveTaskOverlaps(repo, work, proj, task, base string, mine []string) []taskOverlap {
	if work == "" || proj == "" || len(mine) == 0 {
		return nil
	}
	out, rc := gitRC(repo, "worktree", "list", "--porcelain")
	if rc != 0 {
		return nil
	}
	want := map[string]bool{}
	for _, p := range mine {
		want[p] = true
	}
	var found []taskOverlap
	// git prints a worktree's REAL path; a work root reached through a symlink (/tmp on macOS) must be compared by its real one too
	root := filepath.Join(work, proj)
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	for _, wt := range parseWorktreeList(out) {
		rel, err := filepath.Rel(root, wt.path)
		if err != nil || rel == "" || strings.HasPrefix(rel, "..") || strings.Contains(rel, string(filepath.Separator)) {
			continue // not a task worktree of this lane
		}
		if rel == task || !strings.HasPrefix(wt.branch, "task/") {
			continue
		}
		changed, rc := gitRC(wt.path, "diff", "--name-only", base+"..."+wt.branch)
		if rc != 0 {
			continue
		}
		var shared []string
		for _, p := range strings.Split(changed, "\n") {
			p = strings.TrimSpace(p)
			if p != "" && want[p] {
				shared = append(shared, p)
			}
		}
		if len(shared) > 0 {
			sort.Strings(shared)
			found = append(found, taskOverlap{task: rel, paths: shared})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].task < found[j].task })
	return found
}

type worktreeRef struct{ path, branch string }

// parseWorktreeList reads `git worktree list --porcelain`.
func parseWorktreeList(out string) []worktreeRef {
	var refs []worktreeRef
	var cur worktreeRef
	flush := func() {
		if cur.path != "" {
			refs = append(refs, cur)
		}
		cur = worktreeRef{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			cur.branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	flush()
	return refs
}

// warnTaskOverlap is the dispatch-time warning: this brief's named paths, plus what this task's own branch already
// changed, against every other live task's branch. It never refuses and never fails a dispatch.
func (d *dsp) warnTaskOverlap() {
	work := d.c.work
	if work == "" || d.proj == "" || d.task == "" {
		return
	}
	mine := map[string]bool{}
	var mineList []string
	add := func(p string) {
		if p != "" && !mine[p] {
			mine[p] = true
			mineList = append(mineList, p)
		}
	}
	if d.brief != "" && brReadable(d.brief) {
		if b, err := os.ReadFile(d.brief); err == nil {
			for _, p := range brNamedPaths(string(b)) {
				add(p)
			}
		}
	}
	base := d.taskBase()
	if base == "" {
		base = "main"
	}
	// any worktree of the lane's repository lists them all; ours may not exist yet at dispatch
	repo := ""
	for _, c := range append([]string{d.intendedWorkdir()}, siblingWorktrees(work, d.proj)...) {
		if isRegularDir(c) && hasGitEntry(c) {
			repo = c
			break
		}
	}
	if repo == "" {
		return // nothing live to overlap with, or not a repository layout
	}
	// this task's OWN worktree, when it already exists: what it changed is what it will collide with
	if own := filepath.Join(work, d.proj, d.task); isRegularDir(own) {
		if changed, rc := gitRC(own, "diff", "--name-only", base+"...task/"+d.task); rc == 0 {
			for _, p := range strings.Split(changed, "\n") {
				add(strings.TrimSpace(p))
			}
		}
	} else if changed, rc := gitRC(work, "diff", "--name-only", base+"...task/"+d.task); rc == 0 {
		for _, p := range strings.Split(changed, "\n") {
			add(strings.TrimSpace(p))
		}
	}
	if len(mineList) == 0 {
		return
	}
	for _, o := range liveTaskOverlaps(repo, work, d.proj, d.task, base, mineList) {
		shown := o.paths
		more := ""
		if len(shown) > 8 {
			more = " (+" + itoa(len(shown)-8) + " more)"
			shown = shown[:8]
		}
		d.warn("OVERLAP with live task " + o.task + ": " + strings.Join(shown, " ") + more + " — its unmerged branch changes " + plural(len(o.paths), "path") + " this brief or this task's branch touches too. Two live tasks in one file cost rulings, a rebase and retired code on 2026-11-04: launch it anyway if that is intended, or sequence them.")
	}
}

// hasGitEntry is true for a worktree (.git is a FILE there) and for a normal checkout.
func hasGitEntry(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// siblingWorktrees is every directory under <work>/<proj>, for finding the repository when ours is not there yet.
func siblingWorktrees(work, proj string) []string {
	ents, err := os.ReadDir(filepath.Join(work, proj))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, filepath.Join(work, proj, e.Name()))
		}
	}
	return out
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + one + "s"
}
