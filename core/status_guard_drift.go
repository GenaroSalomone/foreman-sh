package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// guardDriftLines names every guard installed under HOME/.claude/hooks whose bytes differ from its copy in the tree's
// setup/guards/, and the installer that fixes it. After the pi-background-wait merge ~/.claude/hooks/deny-executor-
// sleep-poll.py still differed from setup/guards/ and Claude executors kept running the old guard (2026-11-04): the tree
// was right, the installed copy was what ran, and nothing said so. A guard with no installed copy is not drift (not
// every machine installs every guard), and nothing here writes under ~/.claude*: the install is the brainer's step.
func guardDriftLines(root, home string) []string {
	treeDir := filepath.Join(root, "setup", "guards")
	hooksDir := filepath.Join(home, ".claude", "hooks")
	entries, err := os.ReadDir(treeDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		installed, err := os.ReadFile(filepath.Join(hooksDir, n))
		if err != nil {
			continue
		}
		tree, err := os.ReadFile(filepath.Join(treeDir, n))
		if err != nil || bytes.Equal(installed, tree) {
			continue
		}
		out = append(out, "guards: ~/.claude/hooks/"+n+" differs from setup/guards/"+n+" — Claude executors run the INSTALLED copy, not the tree's; "+
			guardInstallHint(root, n))
	}
	return out
}

// guardInstallHint is the command that installs guard n: the setup/install/*.sh script that names it, or the generic one.
func guardInstallHint(root, n string) string {
	dir := filepath.Join(root, "setup", "install")
	scripts, _ := filepath.Glob(filepath.Join(dir, "*.sh"))
	sort.Strings(scripts)
	for _, s := range scripts {
		if b, err := os.ReadFile(s); err == nil && bytes.Contains(b, []byte(n)) {
			return "run `bash setup/install/" + filepath.Base(s) + "` (the brainer's step; an executor never writes ~/.claude*)"
		}
	}
	return "no installer under setup/install/ names it: copy it by hand (the brainer's step; an executor never writes ~/.claude*)"
}

// guardDriftOutput is what `hw status` prints after the audits: one line per drifted guard, nothing when none.
func (a *app) guardDriftOutput() []byte {
	root := a.getenv("HW_CORE_ROOT")
	if root == "" {
		root = a.c.brain
	}
	home := a.getenv("HOME")
	if root == "" || home == "" {
		return nil
	}
	lines := guardDriftLines(root, home)
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
