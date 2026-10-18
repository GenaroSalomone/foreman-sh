package main

// hw briefs — the commit by private index (lib/hw/briefs.sh `_briefs_commit_ops`, `_briefs_undo`, `_briefs_rel`).
// The checkout is shared, so the moves go through a PRIVATE GIT_INDEX_FILE: git mv renames on disk and in that
// index only, commit-tree builds the commit, update-ref is given the OLD value, and the real index is reset for those
// paths alone afterwards.

import (
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// briefsOp is one line of the ops file: mv <a> <b> | put <a> <file b> | del <a>.
type briefsOp struct{ op, a, b string }

type briefsUndoStep struct{ op, a, b string }

// briefsRel is _briefs_rel: abs relative to the checkout top, or false when it is outside it.
func briefsRel(top, abs string) (string, bool) {
	dir, ok := briefsRealpath(filepath.Dir(abs))
	if !ok {
		return "", false
	}
	abs = dir + "/" + filepath.Base(abs)
	if !strings.HasPrefix(abs, top+"/") {
		return "", false
	}
	return abs[len(top)+1:], true
}

// briefsEnvWith is the process environment with one variable set.
func briefsEnvWith(k, v string) []string {
	env := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, k+"=") {
			env = append(env, e)
		}
	}
	return append(env, k+"="+v)
}

func briefsCopy(src, dst string, keepMode bool) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(src); err == nil {
		mode = st.Mode().Perm()
	}
	if !keepMode {
		if st, err := os.Stat(dst); err == nil { // cp onto an existing file keeps the destination's mode
			mode = st.Mode().Perm()
		}
	}
	if err := os.WriteFile(dst, in, mode); err != nil {
		return err
	}
	if keepMode {
		if st, err := os.Stat(src); err == nil {
			os.Chtimes(dst, st.ModTime(), st.ModTime())
			os.Chmod(dst, st.Mode().Perm())
		}
	}
	return nil
}

// briefsUndo puts the disk back, last operation first.
func (a *app) briefsUndo(top string, undo []briefsUndoStep) {
	for i := len(undo) - 1; i >= 0; i-- {
		u := undo[i]
		switch u.op {
		case "mv":
			os.MkdirAll(filepath.Dir(top+"/"+u.b), 0o755)
			if os.Rename(top+"/"+u.a, top+"/"+u.b) != nil {
				a.warn("briefs: could not put " + u.b + " back")
			}
		case "rm":
			os.Remove(top + "/" + u.a)
		case "restore":
			os.MkdirAll(filepath.Dir(top+"/"+u.a), 0o755)
			if briefsCopy(u.b, top+"/"+u.a, true) != nil {
				a.warn("briefs: could not put " + u.a + " back")
			}
		}
	}
}

// briefsCommitOps is _briefs_commit_ops: the commit id (8 chars) on success; ok false (the disk as it was found) on any failure.
func (a *app) briefsCommitOps(top, subject string, ops []briefsOp) (string, bool) {
	git := func(env []string, args ...string) extResult {
		return extRun(extCall{Name: "git", Args: append([]string{"-C", top}, args...), Env: env, KeepErr: true})
	}
	br := git(nil, "symbolic-ref", "--short", "-q", "HEAD")
	branch := strings.TrimRight(br.Out, "\n")
	if br.RC != 0 || branch == "" {
		a.warn("briefs: HEAD of " + top + " is detached; nothing was moved")
		return "", false
	}
	ov := git(nil, "rev-parse", "-q", "--verify", "refs/heads/"+branch)
	old := strings.TrimRight(ov.Out, "\n")
	if ov.RC != 0 {
		a.warn("briefs: cannot read " + branch)
		return "", false
	}
	oldtree := old + "^{tree}"
	idxDir, err := os.MkdirTemp("", "hw-briefs-idx.")
	if err != nil {
		return "", false
	}
	idx := idxDir + "/index" // a path that does not exist yet, as `mktemp; rm -f` leaves it
	bk, err := os.MkdirTemp("", "hw-briefs-bk.")
	if err != nil {
		os.RemoveAll(idxDir)
		return "", false
	}
	cleanup := func() { os.RemoveAll(idxDir); os.RemoveAll(bk) }
	ienv := briefsEnvWith("GIT_INDEX_FILE", idx)
	if r := git(ienv, "read-tree", old); r.RC != 0 {
		cleanup()
		a.warn("briefs: cannot read HEAD into a private index")
		return "", false
	}

	var mu sync.Mutex
	var undo []briefsUndoStep
	var paths []string
	committed := false
	// an interrupt part-way through leaves files moved and nothing committed: put the disk back
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	stop := make(chan struct{})
	go func() {
		select {
		case <-sig:
			mu.Lock()
			if !committed {
				a.briefsUndo(top, undo)
			}
			cleanup()
			os.Exit(130)
		case <-stop:
		}
	}()
	defer func() { signal.Stop(sig); close(stop) }()

	failed := false
	mu.Lock()
	for i, o := range ops {
		switch o.op {
		case "mv":
			os.MkdirAll(filepath.Dir(top+"/"+o.b), 0o755)
			r := git(ienv, "mv", "--", o.a, o.b)
			if r.RC == 0 {
				undo = append(undo, briefsUndoStep{"mv", o.b, o.a})
				paths = append(paths, o.a, o.b)
			} else {
				a.warn("briefs: git mv " + o.a + " failed: " + strings.TrimRight(r.Err, "\n"))
				failed = true
			}
		case "put":
			orig := bk + "/orig." + itoa(i+1)
			if _, err := os.Lstat(top + "/" + o.a); err == nil {
				briefsCopy(top+"/"+o.a, orig, true)
				undo = append(undo, briefsUndoStep{"restore", o.a, orig})
			} else {
				undo = append(undo, briefsUndoStep{"rm", o.a, ""})
			}
			os.MkdirAll(filepath.Dir(top+"/"+o.a), 0o755)
			if briefsCopy(o.b, top+"/"+o.a, false) == nil && git(ienv, "add", "--", o.a).RC == 0 {
				paths = append(paths, o.a)
			} else {
				a.warn("briefs: could not write " + o.a)
				failed = true
			}
		case "del":
			orig := bk + "/orig." + itoa(i+1)
			if briefsCopy(top+"/"+o.a, orig, true) == nil {
				undo = append(undo, briefsUndoStep{"restore", o.a, orig})
			}
			if git(ienv, "rm", "-q", "-f", "--", o.a).RC == 0 {
				paths = append(paths, o.a)
			} else {
				a.warn("briefs: could not remove " + o.a)
				failed = true
			}
		}
		if failed {
			break
		}
	}

	var newtree, c string
	if !failed {
		r := git(ienv, "write-tree")
		newtree = strings.TrimRight(r.Out, "\n")
		if r.RC != 0 {
			failed = true
		}
	}
	if !failed {
		// the one safety that matters: the tree differs from HEAD's in the named paths and no other
		expect := map[string]bool{}
		for _, p := range paths {
			expect[p] = true
		}
		r := git(nil, "diff-tree", "-r", "--no-renames", "--name-only", oldtree, newtree)
		var extra bool
		for _, p := range strings.Split(r.Out, "\n") {
			if p != "" && !expect[p] {
				extra = true
			}
		}
		if extra {
			a.warn("briefs: REFUSED — the tree it built differs from HEAD's in more than the paths it names; nothing was committed")
			failed = true
		}
	}
	if !failed {
		r := git(nil, "commit-tree", newtree, "-p", old, "-m", subject)
		c = strings.TrimRight(r.Out, "\n")
		if r.RC != 0 {
			a.warn("briefs: commit-tree failed (is user.name/user.email set?)")
			failed = true
		} else if git(nil, "update-ref", "refs/heads/"+branch, c, old).RC != 0 {
			a.warn("briefs: " + branch + " moved while the commit was being written; nothing was undone upstream, and the disk is put back")
			failed = true
		}
	}
	if failed {
		a.briefsUndo(top, undo)
		mu.Unlock()
		cleanup()
		return "", false
	}
	committed = true // committed: from here nothing is undone
	mu.Unlock()
	// the real index keeps no stale idea of these paths (it would show them as staged renames)
	seen := map[string]bool{}
	var uniq []string
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	sort.Strings(uniq)
	if git(nil, append([]string{"reset", "-q", "--"}, uniq...)...).RC != 0 {
		a.warn("briefs: committed, but 'git reset' of the moved paths failed on the real index; run it by hand")
	}
	cleanup()
	if len(c) > 8 {
		c = c[:8]
	}
	return c, true
}
