package main

// lanes/git-worktree.sh — the build for a lane whose repo has no worktree tooling of its own: a plain
// `git worktree add`, no ports, no database, no dev server, and the two-pane layout. NO `git fetch`: the base is
// the table's `base_ref_prefix` + `base`.

// laneBuildGitWorktree is lanes/git-worktree.sh, sourced at the `*)` arm.
func (l *lch) laneBuildGitWorktree() {
	main := l.laneCheckout(l.proj)
	base := l.taskBase()
	wt := l.laneWtDir(l.proj, l.task)
	l.wt = wt
	if main == "" || !laneIsDir(main) { // # MUTATION-ANCHOR: 847-D1-20
		m := main
		if m == "" {
			m = "<none in projects.json>"
		}
		l.die("checkout not found: " + m)
	}
	if _, rc := l.sh(cmdSpec{name: "git", args: []string{"-C", main, "rev-parse", "--git-dir"}, merge: true}); rc != 0 { // # MUTATION-ANCHOR: 847-D1-21
		l.die(main + " is not a git repository — this build adds a git worktree to it")
	}

	l.step("1/4  worktree")
	created := false
	if laneExists(wt + "/.git") { // # MUTATION-ANCHOR: 847-D1-22
		l.ok("worktree already exists, reusing: " + wt + " (HEAD " + l.laneHead(wt) + ")")
	} else if laneExists(wt) { // # MUTATION-ANCHOR: 847-D1-23
		l.die(wt + " exists and is not a git worktree — dispatch under another task name, or move it aside. Nothing was created.")
	} else {
		created = l.laneWorktreeAdd(main, wt, base)
	}
	if !laneExists(wt+"/.git") && !(l.readonly && created) {
		l.die("worktree creation reported success but " + wt + " is not a worktree")
	}
	l.receiptWorktree(wt)
	l.refuseIfStaleHooks(wt, main, base)

	l.step("2/4  local config")
	l.info("plain git worktree: only what git tracks is here — an ignored .env, .venv or node_modules in " + main + " was NOT carried over")
	if !l.wantDB {
		l.info("--no-db has no effect here: this lane provisions no database")
	}

	if l.here {
		l.runHere(wt)
		return
	}

	l.laneTrustSdd(wt)

	l.step("3/4  space + layout")
	l.laneSpaceTwoPane(wt)

	l.step("4/4  agent")
	l.agentStep()

	l.printf("\n  %s%s%s  %s\n  branch %s off %s  ·  no ports, no database\n\n", l.cB, l.label, l.c0, wt, l.branch, base)
}
