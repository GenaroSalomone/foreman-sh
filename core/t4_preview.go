package main

// hw preview [list] | hw preview release <project> <task> — a port of cmd_preview (bin/hw:8836-8890) and of the
// two dispatch lines that reach it (`hw preview` alone lists, `hw preview <action> …`, bin/hw:12646 and 12659),
// byte for byte. What a preview lease is, who holds it, until when, and how to give it up. It reads the lease
// file `hw done --keep-preview` writes (`<run>/lease`, bin/hw:8547-8570) and, for `release`, removes it.
//
// Every top-level name of the verb starts with `preview` (the packages of the four writers of train T4 merge).

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (a *app) cmdPreview() int {
	// bin/hw runs `need herdr; need git; need jq` before anything
	for _, c := range []string{"herdr", "git", "jq"} {
		if _, err := exec.LookPath(c); err != nil {
			a.die("missing required command: " + c)
		}
	}
	// the question "what are my options" must not require already knowing the answer: -h/--help ANYWHERE
	for _, x := range a.args {
		if x == "-h" || x == "--help" { // # MUTATION-ANCHOR: 864-G04
			a.usageShort()
			return 0
		}
	}
	action := "list"
	if len(a.args) > 0 {
		action = a.args[0]
	}
	switch action {
	case "list":
		a.previewList()
	case "release":
		proj, task := "", ""
		if len(a.args) > 1 {
			proj = a.args[1]
		}
		if len(a.args) > 2 {
			task = a.args[2]
		}
		a.previewRelease(a.c.t.canonProject(proj), task) // # MUTATION-ANCHOR: 864-G06
	default:
		a.die("usage: hw preview [list] | hw preview release <project> <task>") // # MUTATION-ANCHOR: 864-G05
	}
	return 0
}

// previewNow is `date +%s`. It asks `date`, as bash does, so the clock of the contract world (a stub that answers
// HW_T4_NOW) is the clock of both implementations.
func previewNow() int64 {
	out, err := exec.Command("date", "+%s").Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return n
}

// previewField is _lease_field: `sed -n "s/^<key>=//p" <run>/lease | head -1`.
func previewField(rundir, key string) string {
	data, err := os.ReadFile(rundir + "/lease")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, key+"=") {
			return strings.TrimPrefix(l, key+"=")
		}
	}
	return ""
}

// previewLeaseState is _lease_state: 0 a lease exists and has not expired, 1 none, 2 expired.
func previewLeaseState(rundir string) int {
	if !readableFile(rundir + "/lease") {
		return 1
	}
	until := previewField(rundir, "until")
	if until == "" || strings.Trim(until, "0123456789") != "" {
		return 2
	}
	n, err := strconv.ParseInt(until, 10, 64)
	if err != nil || previewNow() >= n { // # MUTATION-ANCHOR: 864-G01
		return 2
	}
	return 0
}

// previewRuns is `fd -H -t d -d 1 '^2[0-9]{7}-[0-9]{6}-[0-9]+$' <task>/.hw | sort -r`, as full paths.
func previewRuns(task string) []string {
	var out []string
	for _, r := range RunIDs(task + "/.hw") {
		out = append(out, task+"/.hw/"+r)
	}
	return out
}

// previewLeasedRun is _leased_rundir_for: the newest run of a worktree whose lease is live.
func previewLeasedRun(task string) (string, bool) {
	if !isDir(task + "/.hw") {
		return "", false
	}
	for _, r := range previewRuns(task) {
		if previewLeaseState(r) == 0 { // # MUTATION-ANCHOR: 864-G02
			return r, true
		}
	}
	return "", false
}

// previewPad is printf's `%-Ns`, which counts BYTES (the bash on a Mac is 3.2).
func previewPad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s)) // # MUTATION-ANCHOR: 864-G07
}

// previewSubdirs is `fd -H -t d -d 1 . <base>`: the real directories one level down. fd lists them in no order
// it promises; sorted here, and the world holds one leased task a lane, so the order is never in question.
func previewSubdirs(base string) []string {
	ents, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.Type().IsDir() {
			out = append(out, base+"/"+e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// previewBase is `base="$(_wt_canonical_root "$lane")" || base="$WORK/$lane"`.
func (a *app) previewBase(lane string) string {
	d := a.c.lanePath(lane, "worktree", "__probe__")
	if d == "" {
		return a.c.work + "/" + lane
	}
	return filepath.Dir(d)
}

func (a *app) previewList() {
	found := false
	a.printf("\n")
	for _, lane := range a.c.t.names {
		base := a.previewBase(lane)
		if !isDir(base) {
			continue
		}
		for _, t := range previewSubdirs(base) {
			name := filepath.Base(t)
			if r, ok := previewLeasedRun(t); ok { // # MUTATION-ANCHOR: 864-G08
				found = true
				a.printf("  %s%s%s %sheld%s     %s\n", a.cB, previewPad(lane+":"+name, 34), a.c0, a.cOK, a.c0, previewField(r, "why"))
				a.printf("      %suntil %s · by %s · release: hw preview release %s %s%s\n",
					a.cDim, previewField(r, "until_h"), previewField(r, "by"), lane, name, a.c0)
				continue
			}
			// expired leases are worth showing too: they are the ones to release
			var listing bytes.Buffer
			for _, rr := range previewRuns(t) {
				listing.WriteString(rr + "\n")
			}
			for _, rr := range wordSplit([]string{listing.String()}) { // `for rr in $(…)` splits on blanks # MUTATION-ANCHOR: 864-G09
				if readableFile(rr + "/lease") { // # MUTATION-ANCHOR: 864-G10
					found = true
					a.printf("  %s%s%s %sEXPIRED%s  %s\n", a.cB, previewPad(lane+":"+name, 34), a.c0, a.cWarn, a.c0, previewField(rr, "why"))
					a.printf("      %sexpired %s · release: hw preview release %s %s%s\n", a.cDim, previewField(rr, "until_h"), lane, name, a.c0)
					break
				}
			}
		}
	}
	if !found { // # MUTATION-ANCHOR: 864-G11
		a.info("no preview leases")
	}
	a.printf("\n")
}

func (a *app) previewRelease(proj, task string) {
	if task == "" { // # MUTATION-ANCHOR: 864-G12
		a.die("usage: hw preview release <project> <task>")
	}
	n := 0
	for _, r := range a.previewTaskRuns(proj, task) {
		if readableFile(r + "/lease") {
			if os.Remove(r+"/lease") == nil { // # MUTATION-ANCHOR: 864-G03
				n++
			}
		}
	}
	if n == 0 {
		a.info("no lease on " + proj + ":" + task + " — nothing to release")
	} else {
		a.ok("released " + strconv.Itoa(n) + " lease(s) on " + proj + ":" + task) // # MUTATION-ANCHOR: 864-G13
		a.info("the preview is still RUNNING: this only removed the protection. Close it with: hw done " + proj + " " + task + " --drop-db")
	}
}

// previewTaskRuns is _task_rundirs: the run directories of a task, newest first.
func (a *app) previewTaskRuns(proj, task string) []string {
	base := a.c.lanePath(proj, "worktree", task)
	if base == "" {
		base = a.c.work + "/" + proj + "/" + task
	}
	if !isDir(base + "/.hw") {
		return nil
	}
	return previewRuns(base)
}
