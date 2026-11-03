package main

// The executor's per-run TMPDIR lives OUTSIDE every git repo: <cache root>/hw-run-tmp/<run> (inside the worktree, a "plain"
// directory made under $TMPDIR was in a repo — rest-limpio-070). hw creates it at dispatch (exportHwEnv). Every step that removes a
// work dir (hw done, hw reap --apply, loose dirs) removes the runs' TMPDIRs too, via removeWithRunTmps; a kept work dir keeps its
// scratch. The path removed is the one the run RECORDED in <dir>/.hw/<run>/env, else the one derived from the closer's environment.

import (
	"os"
	"path/filepath"
)

// runTmpFallbackRoot is the root when neither cache variable is usable; it sits in shared space, so it is made 0700.
const runTmpFallbackRoot = "/tmp/hw-run-tmp"

// runTmpRoot is the parent of every per-run TMPDIR: $XDG_CACHE_HOME (else $HOME/.cache) + /hw-run-tmp; /tmp when neither is set.
func runTmpRoot(getenv func(string) string) string {
	// ABSOLUTE only: a relative root would land back inside the worktree (and RemoveAll would resolve it against hw done's cwd)
	if x := getenv("XDG_CACHE_HOME"); filepath.IsAbs(x) {
		return x + "/hw-run-tmp"
	}
	if h := getenv("HOME"); filepath.IsAbs(h) {
		return h + "/.cache/hw-run-tmp"
	}
	return runTmpFallbackRoot
}

// secureRunTmpRoot makes the /tmp fallback root 0700 (a cache root is already private). Call after the run's dir exists.
func secureRunTmpRoot(runTmp string, warn func(string)) {
	if root := filepath.Dir(runTmp); root == runTmpFallbackRoot {
		if err := os.Chmod(root, 0o700); err != nil {
			warn("could not make " + root + " private (0700): " + err.Error())
		}
	}
}

// runTmpDir is one run's TMPDIR.
func runTmpDir(getenv func(string) string, run string) string {
	if run == "" {
		run = "norun"
	}
	return runTmpRoot(getenv) + "/" + run
}

// recordedRunTmpdir is the TMPDIR recorded in <rundir>/env, "" unless it is a per-run TMPDIR hw could have made: absolute,
// its base is the run id and its parent's base is hw-run-tmp. Nothing else is ever removed on the strength of a file.
func recordedRunTmpdir(rundir, run string) string {
	data, err := os.ReadFile(rundir + "/env")
	if err != nil {
		return ""
	}
	kvs, perr := ParseEnv(data, false, false)
	if perr != nil {
		return ""
	}
	for _, kv := range kvs {
		if kv.Key != "TMPDIR" {
			continue
		}
		t := filepath.Clean(kv.Value)
		if filepath.IsAbs(t) && filepath.Base(t) == run && run != "" && filepath.Base(filepath.Dir(t)) == "hw-run-tmp" {
			return t
		}
	}
	return ""
}

// runTmpDirs is the TMPDIR of every run recorded under <wt>/.hw/ — read before the work dir is removed.
func runTmpDirs(getenv func(string) string, wt string) []string {
	if !isDir(wt + "/.hw") {
		return nil
	}
	var out []string
	for _, id := range RunIDs(wt + "/.hw") {
		d := recordedRunTmpdir(wt+"/.hw/"+id, id)
		if d == "" {
			d = filepath.Clean(runTmpDir(getenv, id))
		}
		out = append(out, d)
	}
	return out
}

// removeWithRunTmps reads the runs' TMPDIRs under wt, calls rm (which removes wt), and clears those TMPDIRs only if rm succeeded
// (a kept work dir keeps its scratch). Go's module cache is read-only: each one is made writable first, and a failure is warned.
func removeWithRunTmps(getenv func(string) string, wt string, warn func(string), rm func() bool) bool {
	dirs := runTmpDirs(getenv, wt)
	if !rm() {
		return false
	}
	for _, d := range dirs {
		if !doneRmWorkdir(d) {
			warn("could not remove the run's TMPDIR " + d + " — remove it by hand")
		}
	}
	return true
}
