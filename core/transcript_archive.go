package main

// A pi run's transcript is not in its worktree: pi keeps the session jsonl under PI_CODING_AGENT_DIR/sessions, so
// `hw done`'s `git worktree remove` deletes the tree that POINTS at it and leaves the jsonl orphaned — the 2026-11-04
// vault-stays-unlocked retro found no session to read after the close. Before anything is removed, the run's newest pi
// session jsonl is copied to archive/<lane>/<task>/.artifacts/transcript.jsonl.
//
// A CLAUDE run is not touched: its transcript is already under the account's projects dir and the receipt names it, and
// this copies only when the adapter's own record (<run>/pi-runtime.json) says the run was pi.

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// archivePiTranscript copies the run's newest pi session jsonl into the task's archive, and returns the path it wrote
// ("" when the run was not pi, or there was nothing to copy). It never fails the removal: a copy that does not land is
// warned about, because a transcript is evidence and a missing one must be said out loud.
func archivePiTranscript(getenv func(string) string, wt string, warn func(string)) string {
	runs, _ := filepath.Glob(wt + "/.hw/*/pi-runtime.json")
	if len(runs) == 0 {
		return "" // not a pi run
	}
	root := piSessionsRootOf(getenv)
	if root == "" || !isRegularDir(root) {
		warn("no pi session root (" + root + "): the run's transcript is not archived")
		return ""
	}
	// the run's own window: the oldest run dir's mtime, so a session of ANOTHER run does not win by being newest
	earliest := time.Time{}
	for _, f := range runs {
		if st, err := os.Stat(f); err == nil && (earliest.IsZero() || st.ModTime().Before(earliest)) {
			earliest = st.ModTime()
		}
	}
	best, bestMod := "", time.Time{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		st, e := d.Info()
		if e != nil {
			return nil
		}
		if !earliest.IsZero() && st.ModTime().Before(earliest.Add(-time.Hour)) {
			return nil
		}
		if st.ModTime().After(bestMod) {
			best, bestMod = p, st.ModTime()
		}
		return nil
	})
	if best == "" {
		warn("the pi session root " + root + " holds no session of this run: its transcript is not archived")
		return ""
	}
	destDir := archiveRootOf(getenv, wt) + "/" + filepath.Base(filepath.Dir(wt)) + "/" + filepath.Base(wt) + "/.artifacts"
	if err := os.MkdirAll(destDir, 0o777); err != nil {
		warn("cannot create " + destDir + ": the run's pi transcript is not archived")
		return ""
	}
	dest := destDir + "/transcript.jsonl"
	if err := copyFile(best, dest); err != nil {
		warn("could not copy " + best + " to " + dest + ": the run's pi transcript is not archived")
		return ""
	}
	return dest
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

// archiveRootOf is _archive_root for a worktree path: ${HW_ARCHIVE_ROOT:-<parent of the work dir>/archive}.
func archiveRootOf(getenv func(string) string, wt string) string {
	if v := getenv("HW_ARCHIVE_ROOT"); v != "" {
		return v
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(wt))) + "/archive"
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
