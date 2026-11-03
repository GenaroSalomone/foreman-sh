package main

// The lane's own executor material, installed into every worktree or work directory of that lane: `agents` and `skills`
// (the lane's specialists) and `CLAUDE.md` (the lane's executor rules). They live in brain/<lane>/.claude — local,
// excluded from git, never committed to a product repo — and Claude Code only reads them where its directory is under the
// lane, so an executor in work/<lane>/<task>/ or a product worktree saw none. They are linked, not copied: a copy goes
// stale. settings.json and hooks are the brainer's and are never linked.
//
// The same entries are named once in the checkout's `.git/info/exclude`, which every worktree of it shares: linked files
// must never be offered to git. A repo that tracks its own entry keeps it (the warning says so).

import (
	"os"
	"path/filepath"
	"strings"
)

// laneClaudeLinks are the entries of brain/<lane>/.claude an executor of the lane gets: directories or files.
var laneClaudeLinks = []string{"agents", "skills", "CLAUDE.md"}

// laneScope says what of brain/<lane>/.claude a lane's worktree gets, and where its .claude sits in the worktree.
type laneScope struct {
	agentsSkills bool   // agents and skills too; false: CLAUDE.md only (a lane with its own repo brings its own specialists)
	sub          string // the directory of the worktree whose .claude is the lane's ("" = the worktree root)
}

var (
	// laneScopeFull is a lane with no product repo: nothing of the lane's specialists lives in the checkout, so hw is their home.
	laneScopeFull = laneScope{agentsSkills: true}
	// laneScopeRules is a lane with its own repo: the repo's `.claude` is its own,
	// a tracked or a replicated one, and only the lane's executor rules are added, where the worktree has none.
	laneScopeRules = laneScope{}
)

func (sc laneScope) names() []string {
	if sc.agentsSkills {
		return laneClaudeLinks
	}
	return []string{"CLAUDE.md"}
}

// laneRules gives a worktree (or work directory) of the lane the lane's own executor material, and names what it linked in
// the checkout's info/exclude. main is the git checkout the worktree belongs to; "" for a repo-less lane, which has no
// exclude file to write.
func (l *lch) laneRules(main, wt string, sc laneScope) {
	owned := l.linkLaneClaude(wt, sc)
	if main != "" {
		l.excludeLaneClaude(main, wt, sc, owned)
	}
	l.receipt("lane_rules", l.laneRulesLine(), "brain/"+l.proj+"/.claude — what the lane's build does with it for the "+l.agent+" executor")
}

// underDir reports whether p is dir or inside it.
func underDir(dir, p string) bool {
	r, err := filepath.Rel(dir, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

// linkLaneClaude links each entry of the scope that exists in brain/<lane>/.claude into <wt>/<sub>/.claude, a file or a
// directory, and returns the names whose link is hw's (made now or earlier). A symlink is replaced only when it points under
// brain/<lane>/.claude, i.e. hw made it; any other entry (file, directory, a symlink elsewhere) is the repo's and is kept.
func (l *lch) linkLaneClaude(wt string, sc laneScope) (owned []string) {
	src := filepath.Join(l.c.brain, l.proj, ".claude")
	for _, name := range sc.names() {
		target := filepath.Join(src, name)
		st, err := os.Stat(target)
		if err != nil || (!st.IsDir() && !st.Mode().IsRegular()) {
			continue
		}
		link := filepath.Join(wt, sc.sub, ".claude", name)
		if cur, err := os.Readlink(link); err == nil {
			if cur == target {
				owned = append(owned, name)
				l.laneRulesOutcome = "linked"
				continue
			}
			if !underDir(src, cur) {
				l.warn(".claude/" + name + " in " + wt + " is a link to " + cur + " — kept, NOT linked to the lane's")
				l.noteKept(name)
				continue
			}
			l.fs.remove(link) // a link hw made earlier, now pointing elsewhere in the lane
		} else if _, err := os.Lstat(link); err == nil {
			l.warn(".claude/" + name + " already exists in " + wt + " — kept, NOT linked to the lane's (a repo that tracks its own wins)")
			l.noteKept(name)
			continue
		}
		l.fs.mkdirAll(filepath.Dir(link))
		if err := l.fs.symlink(target, link); err != nil {
			l.warn("could not link " + link + " → " + target + ": " + err.Error())
			continue
		}
		l.ok(".claude/" + name + " → " + target + " (the lane's own, linked, never copied)")
		owned = append(owned, name)
		if name == "CLAUDE.md" {
			l.laneRulesOutcome = "linked"
		}
	}
	return owned
}

// noteKept records that the worktree kept its own .claude/CLAUDE.md, which is what the receipt's lane_rules reports.
func (l *lch) noteKept(name string) {
	if name == "CLAUDE.md" {
		l.laneRulesOutcome = "kept"
	}
}

// excludeLaneClaude appends to the checkout's info/exclude the links hw owns in this worktree, once: a line already there is
// not repeated, and a path git tracks (in the checkout or in the worktree) is never excluded — new files there must show.
func (l *lch) excludeLaneClaude(main, wt string, sc laneScope, owned []string) {
	if len(owned) == 0 {
		return
	}
	out, rc := l.git(false, main, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if rc != 0 {
		l.warn("could not find " + main + "'s git directory — the lane's .claude links are not excluded from git")
		return
	}
	file := filepath.Join(strings.TrimSpace(out), "info", "exclude")
	have, _ := os.ReadFile(file)
	present := map[string]bool{}
	for _, ln := range strings.Split(string(have), "\n") {
		present[strings.TrimSpace(ln)] = true
	}
	var add strings.Builder
	for _, name := range owned {
		rel := filepath.Join(sc.sub, ".claude", name)
		pat := "/" + filepath.ToSlash(rel)
		if present[pat] {
			continue
		}
		tracked := false
		for _, dir := range []string{main, wt} {
			if o, rc := l.git(false, dir, "ls-files", "--", rel); rc == 0 && strings.TrimSpace(o) != "" {
				tracked = true
			}
		}
		if tracked {
			continue
		}
		add.WriteString(pat + "\n")
	}
	if add.Len() == 0 {
		return
	}
	prefix := ""
	if len(have) > 0 && !strings.HasSuffix(string(have), "\n") {
		prefix = "\n"
	}
	l.fs.mkdirAll(filepath.Dir(file))
	if err := l.fs.appendTo(file, []byte(prefix+add.String()), 0o644); err != nil {
		l.warn("could not write " + file + ": " + err.Error())
		return
	}
	l.ok("the lane's .claude links excluded from git in " + file)
}

// laneRulesFile is brain/<lane>/.claude/CLAUDE.md when it is a regular file, "" otherwise. A lane with no such file has
// no lane-local executor rules, and every vendor's line says `none`.
func laneRulesFile(brain, proj string) string {
	if brain == "" || proj == "" {
		return ""
	}
	f := filepath.Join(brain, proj, ".claude", "CLAUDE.md")
	if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return f
}

// laneRulesLine is the manifest's and the receipt's `lane_rules`: how THIS vendor gets the lane's executor rules.
func (d *dsp) laneRulesLine() string {
	f := laneRulesFile(d.c.brain, d.proj)
	if f == "" {
		return "none — this lane has no brain/" + d.proj + "/.claude/CLAUDE.md"
	}
	switch d.agent {
	case "claude":
		if d.laneRulesOutcome == "kept" {
			return "KEPT (repo has its own) — the worktree already has a .claude/CLAUDE.md, and the lane's " + f + " was NOT linked over it"
		}
		return "linked — " + f + " → .claude/CLAUDE.md in the worktree (a symlink; Claude Code reads ./.claude/CLAUDE.md)"
	case "pi":
		if strings.ContainsAny(f, " \t\n") {
			return "NOT APPLIED — " + d.agent + " would need --append-system-prompt " + f + ", and that path cannot travel as one word on the launch line"
		}
		return "appended — " + f + " via --append-system-prompt (pi reads no .claude/CLAUDE.md)"
	default:
		return "NOT APPLIED — " + d.agent + " reads neither .claude/CLAUDE.md nor an appended prompt, so the lane's executor rules do not reach it"
	}
}
