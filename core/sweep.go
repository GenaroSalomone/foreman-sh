package main

// sweep, without --apply (train T2): the runs whose tab or workspace hw could close, and why each is kept. Port of
// cmd_sweep's dry run (bin/hw:11647-11723) and of _hw_run_dirs. `--apply` closes herdr objects, so it stays bash.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hwRunDirs is _hw_run_dirs: every run directory sweep can see. $WORK is asked of `fd -H -t d <run id> $WORK`, which
// is what bash runs (a missing fd lists nothing there, as in bash); each lane's
// worktree root outside $WORK is globbed `<root>/*/.hw/2*`, because a worktree's `.hw/` sits in the repo's exclude
// file and fd honours that. Roots in survey order, except a lane whose `sweep` is false (every=true lists those too).
func (a *app) hwRunDirs(every bool) []string {
	work := a.c.work
	var roots []string
	for _, l := range a.c.t.survey {
		if !every && a.c.t.get(l, "sweep") == "0" {
			continue
		}
		r := a.c.lanePath(l, "worktree_root", "")
		if r == "" || r == work || strings.HasPrefix(r, work+"/") {
			continue
		}
		roots = append(roots, r)
	}
	// fd itself, as bash runs it: it honours the .gitignore rules of what it walks, and the order it prints is the contract
	var found []string
	if fdOut, _ := toolOut("fd", "-H", "-t", "d", RunIDPattern.String(), work); fdOut != "" {
		for _, l := range strings.Split(strings.TrimRight(fdOut, "\n"), "\n") {
			if l != "" {
				found = append(found, l)
			}
		}
	}
	out := fdSortedOrder(found)
	for _, r := range roots {
		paths, _ := filepath.Glob(escapeGlob(r) + "/*/.hw/2*")
		paths = sortLocale(paths, false) // a shell glob is in the locale's order, a Go one in bytes
		for _, rd := range paths {
			if isDir(rd) && RunIDPattern.MatchString(filepath.Base(rd)) {
				out = append(out, rd)
			}
		}
	}
	return out
}

// fdSortedOrder puts what fd printed in the order fd prints it when its walk is quick: it holds results for ~100 ms and sorts them by
// path, component by component, and a walk that takes longer (a loaded machine, a cold cache) streams them in thread order instead.
// Taking the order here makes the list the same either way (870 tty-sw-dry, Linux full suite, 0600b run 4).
func fdSortedOrder(paths []string) []string {
	sorted := append([]string(nil), paths...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := strings.Split(strings.TrimSuffix(sorted[i], "/"), "/"), strings.Split(strings.TrimSuffix(sorted[j], "/"), "/")
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return len(a) < len(b)
	})
	return sorted
}

// escapeGlob quotes the characters filepath.Glob would read as a pattern, so a root is taken literally.
func escapeGlob(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`*?[\`, r) {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func (a *app) cmdSweep() int {
	apply := len(a.args) > 0 && a.args[0] == "--apply"
	if apply {
		a.errf("hw-core: sweep --apply closes herdr tabs and workspaces, which writes; the bash implementation answers it\n")
		return 64
	}
	found := 0
	for _, rundir := range a.hwRunDirs(false) {
		kind, id := "", ""
		if readableFile(rundir + "/tab") {
			kind = "tab"
			id = squeezeBlanks(rundir + "/tab")
		} else if readableFile(rundir + "/workspace") {
			kind = "workspace"
			id = squeezeBlanks(rundir + "/workspace")
		}
		if kind == "" || id == "" {
			continue
		}
		found++
		task := shBasename(shDirname(shDirname(rundir)))
		if !isFile(runDoneMarker(rundir)) {
			a.info("keeping " + kind + " " + id + " — " + task + " never reported")
			continue
		}
		// Reported, then revived: the marker says a live pane is expected and the task is open again.
		if isFile(runReopenedMarker(rundir)) {
			a.info("keeping " + kind + " " + id + " — " + task + " was reopened by hw revive and has not reported since")
			continue
		}
		if bm := runBlockedMarker(rundir); isFile(bm) && a.blockedSecondsLeft(bm) > 0 {
			a.info("keeping " + kind + " " + id + " — " + task + " reported BLOCKED and is waiting for a ruling (hw ruling resumes it; hw reap closes it after " + blockedWaitHoursBash(a.getenv) + "h)")
			continue
		}
		if leaseLive(rundir) { // # MUTATION-ANCHOR: 871-G10
			a.info("keeping " + kind + " " + id + " — chaining lease until " + leaseField(rundir, "until_h") + ", holder " + leaseField(rundir, "holder") + ": " + leaseField(rundir, "reason"))
			continue
		}
		a.info("would close " + kind + " " + id + " (reported done)")
	}
	if found == 0 {
		a.info("no run recorded a workspace id — nothing hw can prove it owns")
	} else {
		a.info("dry run. Add --apply to actually close them.")
	}
	return 0
}

// squeezeBlanks is `tr -d '[:space:]' < file`.
func squeezeBlanks(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range data {
		switch c {
		case ' ', '\t', '\n', '\v', '\f', '\r':
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// shDirname and shBasename are dirname(1) and basename(1): a trailing slash (fd prints a directory with one) is not part
// of the name, where filepath.Dir and filepath.Base would read it as an empty last element.
func shDirname(p string) string {
	t := strings.TrimRight(p, "/")
	if t == "" {
		if p == "" {
			return "."
		}
		return "/"
	}
	i := strings.LastIndexByte(t, '/')
	if i < 0 {
		return "."
	}
	d := strings.TrimRight(t[:i], "/")
	if d == "" {
		return "/"
	}
	return d
}

func shBasename(p string) string {
	t := strings.TrimRight(p, "/")
	if t == "" {
		if p == "" {
			return ""
		}
		return "/"
	}
	return t[strings.LastIndexByte(t, '/')+1:]
}
