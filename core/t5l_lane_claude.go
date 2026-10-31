package main

// linkLaneClaude gives the work directory of a lane with no product repo the lane's own specialists. The lane's
// `.claude/agents` and `.claude/skills` live in brain/<lane>/ (local, excluded from git) and Claude Code only reads
// them where its directory is under that lane, so an executor in work/<lane>/<task>/ saw none, and the prompt said so.
// They are linked, not copied: a copy goes stale. settings.json and hooks are the brainer's and are never linked.

import (
	"os"
	"path/filepath"
)

// laneClaudeLinks are the entries of brain/<lane>/.claude an executor of a repo-less lane gets.
var laneClaudeLinks = []string{"agents", "skills"}

func (l *lch) linkLaneClaude(wt string) {
	src := filepath.Join(l.c.brain, l.proj, ".claude")
	for _, name := range laneClaudeLinks {
		target := filepath.Join(src, name)
		if st, err := os.Stat(target); err != nil || !st.IsDir() {
			continue
		}
		link := filepath.Join(wt, ".claude", name)
		if cur, err := os.Readlink(link); err == nil {
			if cur == target {
				continue
			}
			l.fs.remove(link) // a link this function made earlier, now pointing elsewhere
		} else if _, err := os.Lstat(link); err == nil {
			l.warn(".claude/" + name + " already exists in " + wt + " — kept, NOT linked to the lane's (a repo that tracks its own wins)")
			continue
		}
		l.fs.mkdirAll(filepath.Dir(link))
		if err := l.fs.symlink(target, link); err != nil {
			l.warn("could not link " + link + " → " + target + ": " + err.Error())
			continue
		}
		l.ok(".claude/" + name + " → " + target + " (the lane's specialists, linked, never copied)")
	}
}
