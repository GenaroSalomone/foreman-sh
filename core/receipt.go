package main

// `hw receipt <project> <task>` and `hw log <project> <task>`: the run
// directory read back. Ported from cmd_receipt and cmd_log (bin/hw:7727-7795).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// listVisible is `ls -1`'s entries: everything but dotfiles.
func listVisible(dir string) ([]os.DirEntry, bool) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	out := ents[:0]
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e)
		}
	}
	return out, true
}

// wordSplit is what `for r in $runs` does to the captured listing.
func wordSplit(names []string) []string {
	var out []string
	for _, n := range names {
		out = append(out, strings.Fields(n)...)
	}
	return out
}

// runsNewestFirst is `ls -1t`: by modification time, newest first, a tie by name.
func runsNewestFirst(dir string) []string {
	ents, _ := listVisible(dir)
	type ent struct {
		name string
		ns   int64
	}
	es := make([]ent, 0, len(ents))
	for _, e := range ents {
		var ns int64
		if info, err := e.Info(); err == nil {
			ns = info.ModTime().UnixNano()
		}
		es = append(es, ent{e.Name(), ns})
	}
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].ns != es[j].ns {
			return es[i].ns > es[j].ns // # MUTATION-ANCHOR: 816-G04
		}
		return es[i].name < es[j].name // # MUTATION-ANCHOR: 816-G05
	})
	names := make([]string, len(es))
	for i, e := range es {
		names[i] = e.name
	}
	return wordSplit(names)
}

// runsOldestFirst is `ls -1`: by name.
func runsByName(dir string) []string {
	ents, _ := listVisible(dir)
	names := make([]string, len(ents))
	for i, e := range ents {
		names[i] = e.Name()
	}
	sort.Strings(names)
	return wordSplit(names) // # MUTATION-ANCHOR: 816-G09
}

// receiptRefusal is bin/hw-actions' receipt_refusal (hw-actions:119-127): the
// text of the three states, shared with the cockpit writer, which still reads
// it from Python. setup/contracts/hw.md pins both to the same strings.
func receiptRefusal(project, task, wt string, hasHW, hasRuns, hasReceipt bool) string {
	switch {
	case !hasHW:
		return "no hw runs for " + project + ":" + task + " (looked in " + wt + "/.hw)"
	case !hasRuns:
		return "no runs under " + wt + "/.hw"
	case !hasReceipt:
		return "runs exist under " + wt + "/.hw but none carries a receipt"
	}
	return ""
}

func (a *app) cmdReceiptArgs() {
	if len(a.args) < 2 { // # MUTATION-ANCHOR: 816-G18
		a.die("usage: hw receipt <project> <task>")
	}
	a.receipt(a.c.t.canonProject(a.args[0]), a.args[1])
}

// workTree is the `wt` both verbs compute: the lane's worktree path, else the
// work directory, and the work directory again when the first has no .hw.
func (a *app) workTree(proj, task string) string {
	wt := a.c.lanePath(proj, "worktree", task)
	if wt == "" {
		wt = a.c.work + "/" + proj + "/" + task
	}
	if !isDir(wt + "/.hw") {
		wt = a.c.work + "/" + proj + "/" + task // # MUTATION-ANCHOR: 816-G15
	}
	return wt
}

func (a *app) receipt(proj, task string) {
	if !a.c.t.known(proj) {
		a.die("unknown project: " + proj + " (expected: " + a.c.t.namesBar() + ")")
	}
	wt := a.workTree(proj, task)
	if !isDir(wt + "/.hw") {
		a.die(receiptRefusal(proj, task, wt, false, false, false))
	}
	runs := runsNewestFirst(wt + "/.hw")
	if len(runs) == 0 {
		a.die(receiptRefusal(proj, task, wt, true, false, false))
	}
	a.printf("\n")
	n := 0
	for _, r := range runs {
		rundir := wt + "/.hw/" + r
		rc := rundir + "/receipt.jsonl"
		if !readableFile(rc) {
			continue // # MUTATION-ANCHOR: 816-G06
		}
		// If this old executor still carries a report that landed at the wrong
		// brainer, reading the lane receipt saves it before rendering.
		a.snapshotDoneReport(rundir)
		n++
		a.printf("  %s%s%s  %s\n", a.cB, r, a.c0, rc)
		a.printf("%s", receiptBody(rc))
		a.printf("\n")
		if n == 1 && !isFile(rundir+"/done") { // # MUTATION-ANCHOR: 816-G07
			a.receiptContextLine(rc)
		}
		if disp, err := os.ReadFile(rundir + "/dispatch"); err == nil && readableFile(rundir+"/dispatch") {
			a.printf("    %sintended (from the dispatch manifest)%s\n", a.cDim, a.c0) // # MUTATION-ANCHOR: 816-G08
			a.printf("%s", dimIndent(disp, a.cDim, a.c0))
			a.printf("\n")
		}
	}
	if n == 0 {
		a.warn(receiptRefusal(proj, task, wt, true, true, false)) // # MUTATION-ANCHOR: 816-G32
		a.info("receipts start with the run that introduced them; older runs only have .hw/<run>/dispatch")
	}
}

func readableFile(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// receiptBody is
//
//	jq -r '"    \(.key)\u0009\(.value)\n      \u001b[2mmeasured by: \(.source)\u001b[0m"' receipt.jsonl
//
// A value jq cannot index is skipped and the next one is read; a syntax error
// ends the stream. The escapes are literal, whatever the terminal.
func receiptBody(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	vals, _ := parseStream(data)
	var sb strings.Builder
	for _, v := range vals {
		k, ok1 := v.get("key")
		val, ok2 := v.get("value")
		src, ok3 := v.get("source")
		if !(ok1 && ok2 && ok3) {
			continue
		}
		sb.WriteString("    " + k.tostring() + "\t" + val.tostring() + "\n      \x1b[2mmeasured by: " + src.tostring() + "\x1b[0m\n")
	}
	return sb.String()
}

// dimIndent is  sed 's/^/      /' f | sed "s/^/$DIM/;s/$/$C0/" : every line
// indented and wrapped in the dim escape, a missing final newline kept (BSD and
// GNU sed both keep it).
func dimIndent(data []byte, dim, c0 string) string {
	var sb strings.Builder
	s := string(data)
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		line, nl := s, ""
		if i >= 0 {
			line, nl, s = s[:i], "\n", s[i+1:]
		} else {
			s = ""
		}
		sb.WriteString(dim + "      " + line + c0 + nl)
	}
	return sb.String()
}

func (a *app) cmdLogArgs() {
	if len(a.args) != 2 {
		a.die("usage: hw log <project> <task>")
	}
	a.log(a.c.t.canonProject(a.args[0]), a.args[1])
}

func (a *app) log(proj, task string) {
	if !a.c.t.known(proj) {
		a.die("unknown project: " + proj + " (expected: " + a.c.t.namesBar() + ")")
	}
	wt := a.workTree(proj, task)
	if !isDir(wt + "/.hw") {
		a.die("no hw runs for " + proj + ":" + task + " (looked in " + wt + "/.hw)")
	}
	n := 0
	for _, r := range runsByName(wt + "/.hw") {
		tl := filepath.Join(wt, ".hw", r, "transcript.log")
		st, err := os.Stat(tl)
		if err != nil || st.Size() == 0 { // # MUTATION-ANCHOR: 816-G10
			continue
		}
		n++
		a.printf("%s%s%s  %s\n", a.cB, r, a.c0, wt+"/.hw/"+r+"/transcript.log")
		if b, err := os.ReadFile(tl); err == nil {
			a.out.Write(b)
		}
	}
	if n == 0 {
		a.warn("no transcript for " + proj + ":" + task + " — nothing was asked, ruled or reported through the invokers yet (runs from before this log existed have none)")
	}
}
