package main

// A pi run's transcript is not in its worktree: pi keeps the session jsonl under PI_CODING_AGENT_DIR/sessions, so
// `hw done`'s `git worktree remove` deletes the tree that POINTS at it and leaves the jsonl orphaned — the 2026-11-04
// vault-stays-unlocked retro found no session to read after the close. Before anything is removed, the run's newest pi
// session jsonl is copied to archive/<lane>/<task>/.artifacts/transcript.jsonl.
//
// A CLAUDE run is not touched: its transcript is already under the account's projects dir and the receipt names it, and
// this copies only when the adapter's own record (<run>/pi-runtime.json) says the run was pi.

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// archivePiTranscript copies the run's pi session jsonl into <archiveDir>/.artifacts/ and returns the path it wrote ("" when
// the run was not pi, or there was nothing to copy). archiveDir is where the task's OUTPUTS are archived (the caller's
// archive/<lane>/<task>, or the stamped sibling it chose), so the transcript lands beside them.
//
// pi-adopt-gentle (2026-11-05), measured on the contract world (`done zeta done-z-arch` with a pi run): the destination used
// to be built from the worktree's own path (three directories up, then `archive`, then the PARENT directory's name as the
// lane), which is right only for work/<lane>/<task>. A lane whose worktrees live in `<repo>/.worktrees/<task>` archived its
// artifacts under archive/<lane>/<task> and wrote the transcript to <repos>/archive/.worktrees/<task>/.artifacts, where nobody
// looks: the copy "skipped it", silently, because success printed nothing. It now (1) lands beside the artifacts, (2) takes the
// session the run RECORDED (pi-runtime.json's session_file, found by its name when the private agent dir that held the
// path is already gone) and only then the newest session of the run's own cwd, and (3) says where it wrote it.
// It never fails the removal: a copy that does not land is warned about, because a transcript is evidence.
func archivePiTranscript(getenv func(string) string, wt, archiveDir string, warn, note func(string)) string {
	runs, _ := filepath.Glob(wt + "/.hw/*/pi-runtime.json")
	if len(runs) == 0 {
		return "" // not a pi run
	}
	if archiveDir == "" {
		warn("no archive directory for this task: the run's pi transcript is not archived")
		return ""
	}
	roots := piSessionRoots(getenv)
	if len(roots) == 0 {
		warn("no pi session root: the run's transcript is not archived")
		return ""
	}
	// the run's own window and its recorded session: newest run record first
	earliest, recorded, newestRec := time.Time{}, "", time.Time{}
	for _, f := range runs {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if earliest.IsZero() || st.ModTime().Before(earliest) {
			earliest = st.ModTime()
		}
		if st.ModTime().After(newestRec) {
			if b, e := os.ReadFile(f); e == nil {
				var rt struct {
					SessionFile string `json:"session_file"`
				}
				if json.Unmarshal(b, &rt) == nil && rt.SessionFile != "" {
					recorded, newestRec = rt.SessionFile, st.ModTime()
				}
			}
		}
	}
	best := ""
	if recorded != "" {
		// session_file is written by the executor (<wt>/.hw/<run>/pi-runtime.json is in its own worktree): it names a pi SESSION or
		// nothing. A path that is not a .jsonl under a session root (a planted ~/.ssh/id_ed25519, a worktree's .env) is never copied.
		if inSessionRoot(roots, recorded) {
			best = recorded
		} else if strings.HasSuffix(recorded, ".jsonl") {
			best = findSessionByName(roots, filepath.Base(recorded))
		}
	}
	if best == "" { // the newest session of this run's own cwd, then (the old rule) the newest of the root inside the run's window
		best = newestSession(roots, wt, earliest, true)
	}
	if best == "" {
		best = newestSession(roots, wt, earliest, false)
	}
	if best == "" {
		warn("the pi session roots " + strings.Join(roots, ", ") + " hold no session of this run: its transcript is not archived")
		return ""
	}
	destDir := archiveDir + "/.artifacts"
	if err := os.MkdirAll(destDir, 0o777); err != nil {
		warn("cannot create " + destDir + ": the run's pi transcript is not archived")
		return ""
	}
	dest := destDir + "/transcript.jsonl"
	if _, err := os.Lstat(dest); err == nil { // an earlier task of the same name: keep both
		dest = destDir + "/transcript." + time.Now().Format("20060102-150405") + ".jsonl"
	}
	if err := copyFile(best, dest); err != nil {
		warn("could not copy " + best + " to " + dest + ": the run's pi transcript is not archived")
		return ""
	}
	note("archived the pi transcript (" + filepath.Base(best) + ") → " + dest)
	return dest
}

// piSessionRoots are the places a run's session may live: the process's agent dir, then the account's own. A pi executor on
// a private agent dir keeps `sessions` there as a symlink to the account's, so the second is where it survives.
func piSessionRoots(getenv func(string) string) []string {
	var out []string
	add := func(r string) {
		if r == "" || !isRegularDir(r) {
			return
		}
		for _, x := range out {
			if x == r {
				return
			}
		}
		out = append(out, r)
	}
	add(piSessionsRootOf(getenv))
	if h := getenv("HOME"); h != "" {
		add(filepath.Join(h, ".pi", "agent", "sessions"))
	}
	return out
}

// inSessionRoot is a regular .jsonl file whose real path lies under one of the session roots (the private agent dir's `sessions`
// is a symlink to the account's: it is the resolved path that counts).
func inSessionRoot(roots []string, p string) bool {
	if !strings.HasSuffix(p, ".jsonl") {
		return false
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return false
	}
	if st, err := os.Stat(real); err != nil || !st.Mode().IsRegular() {
		return false
	}
	for _, r := range roots {
		rr, err := filepath.EvalSymlinks(r)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(rr, real); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// findSessionByName is the session file with this base name under any root, newest first.
func findSessionByName(roots []string, name string) string {
	best, bestMod := "", time.Time{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(p) != name {
				return nil
			}
			if st, e := d.Info(); e == nil && st.ModTime().After(bestMod) {
				best, bestMod = p, st.ModTime()
			}
			return nil
		})
	}
	return best
}

// piCwdDir is the directory name pi gives the sessions of a cwd: "--" + the path with its slashes as dashes + "--".
func piCwdDir(cwd string) string {
	return "--" + strings.ReplaceAll(strings.TrimPrefix(filepath.Clean(cwd), "/"), "/", "-") + "--"
}

// newestSession is the newest session jsonl no older than an hour before `earliest`; ownCwd restricts it to the sessions of
// the worktree's own directory.
func newestSession(roots []string, wt string, earliest time.Time, ownCwd bool) string {
	best, bestMod := "", time.Time{}
	for _, root := range roots {
		walk := root
		if ownCwd {
			walk = filepath.Join(root, piCwdDir(wt))
			if !isRegularDir(walk) {
				continue
			}
		}
		_ = filepath.WalkDir(walk, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") {
				return nil
			}
			st, e := d.Info()
			if e != nil || (!earliest.IsZero() && st.ModTime().Before(earliest.Add(-time.Hour))) {
				return nil
			}
			if st.ModTime().After(bestMod) {
				best, bestMod = p, st.ModTime()
			}
			return nil
		})
	}
	return best
}

// piSessionsRootOf is the invokers' piSessionsRoot for a getenv-shaped environment: <agent dir>/sessions, the agent dir
// being $PI_CODING_AGENT_DIR (a leading ~ expanded) else $HOME/.pi/agent.
func piSessionsRootOf(getenv func(string) string) string {
	home := getenv("HOME")
	d := getenv("PI_CODING_AGENT_DIR")
	switch {
	case d == "":
		if home == "" {
			return ""
		}
		d = filepath.Join(home, ".pi", "agent")
	case d == "~" && home != "":
		d = home
	case strings.HasPrefix(d, "~/") && home != "":
		d = filepath.Join(home, d[2:])
	}
	return filepath.Join(d, "sessions")
}

func isRegularDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
