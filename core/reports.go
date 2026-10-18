package main

// reports --no-repair (train T2): what a lane's executors said, read from disk. Port of cmd_reports (bin/hw:8345-8485)
// with repair off. Reading WITHOUT --no-repair re-reads live panes into receipt.jsonl (`_snapshot_done_report`), which
// writes, so that stays bash: the shim sends only `--no-repair` here, and hw-core refuses the rest by name.

import (
	"fmt"
	"os"
	"strings"
)

// bashWords is what an unquoted `for x in $var` splits a value into: on blanks, tabs and newlines.
func bashWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// workBases is lane_work_bases: the directories a lane's task work directories sit in.
func (a *app) workBases(lane string) []string {
	root := a.c.lanePath(lane, "worktree_root", "")
	if root == "" || root == a.c.work+"/"+lane {
		return []string{a.c.work + "/" + lane}
	}
	return []string{root, a.c.work + "/" + lane}
}

// runInvokerPane is run_invoker_pane (bin/project-spaces.sh): where a run's report was sent, from its env file,
// else from the `invoker` line of its dispatch manifest.
func (a *app) runInvokerPane(rundir string) string {
	pane := ""
	if readableFile(rundir + "/env") {
		data, err := os.ReadFile(rundir + "/env")
		if err == nil {
			if kvs, perr := ParseEnv(data, true, false); perr == nil {
				for _, kv := range kvs {
					if kv.Key == "HW_INVOKER_PANE" {
						pane = kv.Value
					}
				}
			} else {
				a.errf("runenv: %s is not UTF-8: %s\nrun_invoker_pane: %s/env could not be read (runenv exit 2)\n", rundir+"/env", pyUTF8Reason(data), rundir)
			}
		}
	}
	if pane == "" && readableFile(rundir+"/dispatch") {
		if body, ok := readText(rundir + "/dispatch"); ok {
			pane = invokerFromDispatch(body)
		}
	}
	return pane
}

// invokerFromDispatch is
//
//	sed -n 's/^ *invoker  *\([A-Za-z0-9]*:[A-Za-z0-9]*\).*$/\1/p' | tail -1
func invokerFromDispatch(body string) string {
	last := ""
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimLeft(l, " ")
		if !strings.HasPrefix(t, "invoker ") {
			continue
		}
		rest := strings.TrimLeft(t[len("invoker"):], " ")
		if len(rest) == len(t)-len("invoker") { // `invoker  *` needs at least one blank
			continue
		}
		i := 0
		for i < len(rest) && isAlnum(rest[i]) {
			i++
		}
		if i >= len(rest) || rest[i] != ':' {
			continue
		}
		j := i + 1
		for j < len(rest) && isAlnum(rest[j]) {
			j++
		}
		last = rest[:j]
	}
	return last
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// brainerVerdict is lane_brainer_verdict: "ok", "mismatch\tlabel=… cwd=…", or "" when the pane is not in the list.
func brainerVerdict(panesJSON, paneID, wantCwd string) string {
	v, ok := pyLoads(panesJSON)
	if !ok {
		return ""
	}
	res, isMap := v.(jmap)
	if !isMap {
		return ""
	}
	pm, isMap := res["result"].(jmap)
	if !isMap {
		return ""
	}
	list, isList := pm["panes"].([]any)
	if !isList {
		return ""
	}
	canon := func(p string) string { return pyCasefold(strings.TrimRight(pyRealpath(p), "/")) }
	for _, e := range list {
		p, isMap := e.(jmap)
		if !isMap || pyStr(p["pane_id"]) != paneID || p["pane_id"] == nil {
			continue
		}
		label, cwd := "<none>", "<none>"
		if pyTruthy(p["label"]) {
			label = pyStr(p["label"])
		}
		cwdText := ""
		if pyTruthy(p["cwd"]) {
			cwd = pyStr(p["cwd"])
			cwdText = cwd
		}
		if l, isStr := p["label"].(string); isStr && l == "brain" && canon(cwdText) == canon(wantCwd) {
			return "ok"
		}
		return "mismatch\tlabel=" + label + " cwd=" + cwd
	}
	return ""
}

func (a *app) cmdReports() int {
	proj, task, repair, full := "", "", true, false
	for _, arg := range a.args {
		switch {
		case arg == "--no-repair":
			repair = false
		case arg == "--full":
			full = true
		case strings.HasPrefix(arg, "-"):
			a.die("unknown option: " + arg + " (usage: hw reports [<project>] [<task>] [--full] [--no-repair])")
		case proj == "":
			proj = a.c.t.canonProject(arg)
		default:
			task = arg
		}
	}
	if task != "" { // naming a task IS the filter: you asked for one thing, so you get all of it
		full = true
	}
	var lanes []string
	if proj != "" {
		if !a.c.t.known(proj) {
			a.die("unknown project: " + proj + " (expected: " + a.laneNamesHint() + ")")
		}
		lanes = []string{proj}
	} else {
		lanes = a.c.t.names
	}
	h := herdrClient{getenv: a.getenv}
	panes, ok := h.stdout("pane", "list")
	if !ok {
		panes = ""
	}
	n, stranger, recovered, missing, unreported := 0, 0, 0, 0, 0
	for _, lane := range lanes {
		expected := a.c.brain + "/" + lane
		a.printf("\n%sreports%s  %s — read from disk, never from a brainer's context\n", a.cB, a.c0, lane)
		laneRows := 0
		for _, base := range a.workBases(lane) {
			if !isDir(base) {
				continue
			}
			for _, dir := range globStar(base) {
				if !isDir(dir + "/.hw") {
					continue
				}
				tname := dir[strings.LastIndex(dir, "/")+1:]
				if task != "" && tname != task {
					continue
				}
				tRuns, tText, tFlag, tOpen, tStr := 0, 0, 0, 0, 0
				for _, run := range RunIDs(dir + "/.hw") {
					rundir := dir + "/.hw/" + run
					if !readableFile(rundir+"/receipt.jsonl") && !isFile(rundir+"/done") {
						continue
					}
					// READING REPAIRS. While the executor pane is still alive its tokens are the only copy of a report
					// that went to the wrong brainer, and this is idempotent: it returns once a summary exists.
					if repair {
						a.snapshotDoneReport(rundir)
					}
					n++
					laneRows++
					tRuns++
					rc := rundir + "/receipt.jsonl"
					status, summary := receiptLastJQ(rc, "report_status"), receiptLastJQ(rc, "report_summary")
					artifacts, engram := receiptLastJQ(rc, "report_artifacts"), receiptLastJQ(rc, "report_engram")
					invoker := a.runInvokerPane(rundir)
					// the verdict is computed in BOTH modes: a report that left its lane is the one thing a
					// summary must never hide
					verdict := ""
					if invoker != "" {
						verdict = brainerVerdict(panes, invoker, expected)
					}
					if strings.HasPrefix(verdict, "mismatch") { // # MUTATION-ANCHOR: 871-G60
						stranger++
						tStr++
					}
					switch {
					case summary != "":
						recovered++
						tText++
					case isFile(rundir + "/done"):
						missing++
						tFlag++
					default:
						unreported++
						tOpen++
					}
					if !full {
						continue
					}
					a.printf("\n  %s%s:%s%s  %s%s%s\n", a.cB, lane, tname, a.c0, a.cDim, run, a.c0)
					if summary != "" {
						a.printf("      report     %s\n", summary)
						a.printf("      status     %s · artifacts %s · engram %s\n", orStr(status, "unknown"), orStr(artifacts, "unknown"), orStr(engram, "unknown"))
					} else if isFile(rundir + "/done") {
						// THE SENTENCE THAT REPLACES AN EMPTY FIELD. Never print nothing here: nothing is what got read as failure.
						a.printf("      report     NOT ON DISK. This run has a `done` marker, and that marker is a\n")
						a.printf("                 ZERO-BYTE FLAG BY DESIGN (`: > .hw/%s/done`). It records THAT\n", run)
						a.printf("                 delivery was proved, never WHAT was said. Empty is not evidence\n")
						a.printf("                 of failure, of an empty report, or of a task that did nothing.\n")
						a.printf("      recover    the text is in the receiving session and in engram:\n")
						a.printf("                 engram search \"%s\" --project brain --all-projects\n", tname)
					} else {
						a.printf("      report     none — this run has no `done` marker either, so nothing here\n")
						a.printf("                 says it ever reported. That is an open task, not a failed one.\n")
					}
					switch {
					case invoker == "": // # MUTATION-ANCHOR: 871-G61
						a.printf("      routed to  no report channel was set for this run (--no-report, or unresolved)\n")
					case verdict == "ok":
						a.printf("      routed to  %s — the %s brainer\n", invoker, lane)
					case strings.HasPrefix(verdict, "mismatch"):
						a.printf("      routed to  %s — %sNOT the %s brainer%s (%s); expected label=brain cwd=%s\n",
							invoker, a.cWarn, lane, a.c0, verdict[strings.Index(verdict, "\t")+1:], expected)
						a.printf("                 the brainer that owns this lane never received this report\n")
					default:
						a.printf("      routed to  %s — that pane is gone, so hw cannot prove now whether it was\n", invoker)
						a.printf("                 the %s brainer. If the report is not above, it left no copy here.\n", lane)
					}
				}
				// ONE LINE PER TASK, and it says which of the three things each run is.
				if !full && tRuns != 0 {
					detail := ""
					if tText != 0 {
						detail += " · " + itoa(tText) + " with text"
					}
					if tFlag != 0 {
						detail += " · " + itoa(tFlag) + " flag only"
					}
					if tOpen != 0 {
						detail += " · " + itoa(tOpen) + " never reported"
					}
					if tStr != 0 {
						detail += " · " + a.cWarn + itoa(tStr) + " outside lane" + a.c0
					}
					var mark string
					switch {
					case tStr != 0:
						mark = a.cWarn + "!" + a.c0
					case tText == tRuns:
						mark = a.cOK + "✓" + a.c0
					default:
						mark = a.cDim + "·" + a.c0
					}
					plural := "s"
					if tRuns == 1 {
						plural = ""
					}
					a.printf("  %s %-46s %s%d run%s%s%s\n", mark, tname, a.cDim, tRuns, plural, detail, a.c0)
				}
			}
		}
		if laneRows == 0 {
			a.info("no runs on disk for " + lane)
		}
	}
	a.printf("\n")
	a.info(itoa(n) + " run(s) read · " + itoa(recovered) + " with the report text on disk · " + itoa(missing) + " delivered but text not on disk · " + itoa(unreported) + " never reported · " + itoa(stranger) + " routed outside their lane")
	if !full {
		if missing != 0 {
			// THE LEGEND, ONCE. This used to be six lines per run, 76 times in one lane.
			a.printf("  %sflag only%s = the run has a `done` marker, and that marker is a ZERO-BYTE FLAG BY\n", a.cDim, a.c0)
			a.printf("              DESIGN (`: > .hw/<run>/done`). It records THAT delivery was proved,\n")
			a.printf("              never WHAT was said. Empty is not evidence of failure, of an empty\n")
			a.printf("              report, or of a task that did nothing. The text is in the receiving\n")
			a.printf("              session and in engram:  engram search \"<task>\" --project brain --all-projects\n")
		}
		p := proj
		if p == "" {
			p = "<project>"
		}
		a.info("one line per task. For the report text: hw reports " + p + " <task>, or --full for the whole lane")
	}
	if !repair {
		a.info("--no-repair: nothing was re-read from live panes, so a still-live stranded report was not rescued")
	}
	if stranger != 0 {
		a.warn("a report routed outside its lane is only in the receiving session; `hw reports` cannot invent it, and this is what the launch guard now refuses")
	}
	return 0
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// globStar is bash's `"$base"/*`: every entry that does not start with a dot, in the locale's order.
func globStar(base string) []string {
	ents, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	names = sortLocale(names, false)
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = base + "/" + n
	}
	return out
}

// receiptLastJQ is receiptLast for the bash readers, which run plain `jq -r`: a compound value is printed over
// several lines (indented), so `tail -1` of it is the closing bracket. (The status readers go through Python, whose
// compact print is receiptLast's.)
func receiptLastJQ(receipt, key string) string {
	vals, _ := readStream(receipt)
	var outs []string
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok || k.kind != vStr || k.s != key {
			continue
		}
		val, ok := v.get("value")
		if !ok {
			continue
		}
		switch {
		case val.kind == vArr && len(val.arr) > 0:
			outs = append(outs, "[\n]")
		case val.kind == vObj && len(val.obj) > 0:
			outs = append(outs, "{\n}")
		default:
			outs = append(outs, val.raw())
		}
	}
	return bashSub(lastLine(outs))
}

// pyUTF8Reason is str(UnicodeDecodeError) for the first undecodable run of b, as Python's utf-8 codec words it.
func pyUTF8Reason(b []byte) string {
	n := len(b)
	for i := 0; i < n; {
		c := b[i]
		if c < 0x80 {
			i++
			continue
		}
		need, lo, hi := 0, byte(0x80), byte(0xBF)
		switch {
		case c >= 0xC2 && c <= 0xDF:
			need = 1
		case c >= 0xE0 && c <= 0xEF:
			need = 2
			if c == 0xE0 {
				lo = 0xA0
			} else if c == 0xED {
				hi = 0x9F
			}
		case c >= 0xF0 && c <= 0xF4:
			need = 3
			if c == 0xF0 {
				lo = 0x90
			} else if c == 0xF4 {
				hi = 0x8F
			}
		default:
			return fmt.Sprintf("'utf-8' codec can't decode byte 0x%02x in position %d: invalid start byte", c, i)
		}
		end := i + 1
		for k := 1; k <= need; k++ {
			if i+k >= n {
				return pyUTF8Span(b, i, n, "unexpected end of data")
			}
			d := b[i+k]
			l, h := byte(0x80), byte(0xBF)
			if k == 1 {
				l, h = lo, hi
			}
			if d < l || d > h {
				return pyUTF8Span(b, i, end, "invalid continuation byte")
			}
			end = i + k + 1
		}
		i = end
	}
	return ""
}

func pyUTF8Span(b []byte, from, to int, why string) string {
	if to-from == 1 {
		return fmt.Sprintf("'utf-8' codec can't decode byte 0x%02x in position %d: %s", b[from], from, why)
	}
	return fmt.Sprintf("'utf-8' codec can't decode bytes in position %d-%d: %s", from, to-1, why)
}
