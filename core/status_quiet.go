package main

// The quiet checks `hw status` prints after its table: ports of bin/hw's _release_quiet, _gate_quiet, _outbox_quiet,
// _tabs_quiet, _hygiene_quiet, _rules_quiet, _decisions_quiet, _orphans_quiet (bin/hw:12889-13340) and lib/hw/status.sh's
// _agent_tools_quiet. Each prints exactly what its bash function printed, on the same stream, and ends with the
// status the function ended with (all of them 0). The helpers they ask — release-check, decisions, suite-gate,
// agent-tools-check — are programs of bin/, found where the shim says (HW_CORE_BIN_DIR), run through extRun.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// subst is `$(cmd)`: the output without its trailing newlines.
func subst(s string) string { return strings.TrimRight(s, "\n") }

// ── _release_quiet ──────────────────────────────────────────────────────────

// releaseQuiet: A NEWER RELEASE IS ONE LINE, AND ASKED ABOUT ONCE A DAY (bin/release-check): silent offline, silent in
// a brain with no install record, never a failed status.
func (a *app) releaseQuiet() int {
	r := extRun(extCall{Name: "python3", Args: []string{filepath.Join(a.binDir(), "release-check"), a.c.brain}})
	if r.RC != 0 {
		return 0
	}
	if l := subst(r.Out); l != "" {
		a.printf("\n")
		a.warn(l)
	}
	return 0
}

// ── _gate_quiet ─────────────────────────────────────────────────────────────

// gateQuiet: ONE LINE, the heavy-run slots in use and who waits (bin/suite-gate's sg_status_line). A brain without the
// library prints nothing, as the bash did when `declare -F sg_status_line` found nothing.
func (a *app) gateQuiet() int {
	lib := filepath.Join(a.binDir(), "suite-gate")
	if !readable(lib) {
		return 0
	}
	env := append(os.Environ(), "SG_WORK="+a.c.work, "WORK="+a.c.work)
	r := extRun(extCall{Name: "bash", Args: []string{lib, "status"}, Env: env, KeepErr: true})
	a.errf("%s", r.Err) // what sg_slots says of a bad HW_SUITE_SLOTS comes before the line, as it did in the held job
	a.info(subst(r.Out))
	return 0
}

// ── _outbox_quiet ───────────────────────────────────────────────────────────

// outboxCount is _outbox_count: the reports waiting, and the ones held back as uncertain.
func (a *app) outboxCount(proj string) (pending, uncertain int) {
	for _, m := range a.outboxMetas(proj) {
		if !isFile(strings.TrimSuffix(m, ".meta") + ".msg") {
			continue
		}
		if outboxField(m, "state") == "uncertain" {
			uncertain++
		} else {
			pending++
		}
	}
	return
}

// outboxQuiet: one warned block when something is waiting.
func (a *app) outboxQuiet(proj string) int {
	p, u := a.outboxCount(proj)
	if p <= 0 && u <= 0 {
		return 0
	}
	a.printf("\n")
	sfx := ""
	if proj != "" {
		sfx = " " + proj
	}
	if p > 0 {
		a.warn(itoa(p) + " report(s) never reached a brainer and are waiting in an outbox — hw outbox" + sfx + "   (`brain <lane>` redelivers them on open)")
	}
	if u > 0 {
		a.warn(itoa(u) + " report(s) are UNCERTAIN (they may already be in a brainer's session) and are held back — hw outbox" + sfx)
	}
	return 0
}

// ── _tabs_quiet ─────────────────────────────────────────────────────────────

// jqLines is a jq program that prints one string per element of `.result.<key>[]`: the lines it printed before an
// element it could not read, as jq prints them before its error (stderr and the status are dropped by the caller).
func jqLines(text, key string, line func(el *jval) (string, bool)) []string {
	vals, err := parseStream([]byte(text))
	if err != nil && len(vals) == 0 {
		return nil
	}
	var out []string
	for _, v := range vals {
		res, ok := v.get("result")
		if !ok {
			return out
		}
		list, ok := res.get(key)
		if !ok || list.kind != vArr {
			return out
		}
		for _, el := range list.arr {
			s, ok := line(el)
			if !ok {
				return out
			}
			out = append(out, s)
		}
	}
	return out
}

// jqField is `.f` of an object (or null) element; anything else is a jq error.
func jqField(el *jval, f string) (*jval, bool) { return el.get(f) }

func jqDefault(v *jval, def string) string {
	if !v.truthy() {
		return def
	}
	return v.tostring()
}

// splitTabs is `IFS=<tab> read -r f1 … fn`: a tab is IFS whitespace, so runs of tabs are one separator, leading ones
// vanish, and the last field keeps the rest of the line (minus its trailing separators).
func splitTabs(line string, n int) []string {
	out := make([]string, n)
	rest := strings.TrimLeft(line, "\t")
	for i := 0; i < n; i++ {
		if i == n-1 {
			out[i] = strings.TrimRight(rest, "\t")
			break
		}
		j := strings.IndexByte(rest, '\t')
		if j < 0 {
			out[i] = rest
			rest = ""
			continue
		}
		out[i] = rest[:j]
		rest = strings.TrimLeft(rest[j:], "\t")
	}
	return out
}

// padRight is printf's `%-<n>s`: bytes, not characters.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// knownSpaceLabels is _known_space_labels_json: every lane's space label, in table order.
func (a *app) knownSpaceLabels() []string {
	var out []string
	for _, p := range a.c.t.names {
		out = append(out, a.c.t.get(p, "space"))
	}
	return out
}

// tabsQuiet lists, under each project workspace herdr knows, the tabs it holds.
func (a *app) tabsQuiet() int {
	rows := jqLines(extRun(extCall{Name: "herdr", Args: []string{"tab", "list"}}).Out, "tabs", func(el *jval) (string, bool) {
		ws, ok1 := jqField(el, "workspace_id")
		id, ok2 := jqField(el, "tab_id")
		lb, ok3 := jqField(el, "label")
		st, ok4 := jqField(el, "agent_status")
		pc, ok5 := jqField(el, "pane_count")
		if !(ok1 && ok2 && ok3 && ok4 && ok5) {
			return "", false
		}
		return ws.tostring() + "\t" + id.tostring() + "\t" + jqDefault(lb, "-") + "\t" + jqDefault(st, "-") + "\t" + pc.tostring(), true
	})
	known := a.knownSpaceLabels()
	wss := jqLines(extRun(extCall{Name: "herdr", Args: []string{"workspace", "list"}}).Out, "workspaces", func(el *jval) (string, bool) {
		lb, ok := jqField(el, "label")
		if !ok {
			return "", false
		}
		if lb.kind == vNull {
			return "", true // select(.label != null …) drops it; the marker below says "no line"
		}
		if lb.kind != vStr { // `[.label] | inside($known)` of a number or a boolean: not contained, and jq goes on
			return "", true
		}
		inside := false
		for _, k := range known { // inside() is contains(): a string contains another as a substring
			if strings.Contains(k, lb.s) {
				inside = true
			}
		}
		if !inside {
			return "", true
		}
		ws, ok := jqField(el, "workspace_id")
		if !ok {
			return "", false
		}
		return ws.tostring() + "\t" + lb.s, true
	})
	var lines []string
	for _, l := range wss {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return 0
	}
	a.printf("\n  %sproject workspaces%s\n", a.cB, a.c0)
	for _, l := range lines {
		f := splitTabs(l, 2)
		wid, wlabel := f[0], f[1]
		if wid == "" {
			continue
		}
		a.printf("  %s  %s\n", wlabel, wid)
		for _, r := range rows {
			g := splitTabs(r, 5)
			if g[0] != wid {
				continue
			}
			a.printf("      %s%s %s %s pane(s)  %s%s\n", a.cDim, padRight(g[2], 28), padRight(g[3], 8), g[4], g[1], a.c0)
		}
	}
	return 0
}

// ── _orphans_quiet ──────────────────────────────────────────────────────────

var reBrainerGone = regexp.MustCompile(`brainer.s pane .* is gone`)

// orphansQuiet: FINISHED, NEVER REPORTED, AND NOBODY LEFT TO TELL — one line, the count that cannot scroll away.
// It reads the queue the table just worked out; when the case names its own orphans file (HW_STATUS_ORPHANS_JSON) the
// bash read that file back, so a file that is not there to read says nothing.
func (a *app) orphansQuiet(orphans []orphanLine) {
	if f := a.getenv("HW_STATUS_ORPHANS_JSON"); f != "" && !readable(f) {
		return
	}
	var keys []string
	for _, o := range orphans {
		if reBrainerGone.MatchString(o.why) {
			keys = append(keys, o.key)
		}
	}
	if len(keys) == 0 {
		return
	}
	a.warn(itoa(len(keys)) + " task(s) finished and never reported, their brainer's pane gone: " + strings.Join(keys, " ") + " — hw reports <lane> reads each, hw done <lane> <task> closes it")
}

// ── _hygiene_quiet ──────────────────────────────────────────────────────────

// bashLE is `[ "$a" -le "$b" ]`: whether it holds, and whether `[` could compare them at all (it errors, status 2,
// on a word that is not an integer, which `|| why=…` reads as "no").
func bashLE(a, b string) (le, valid bool) {
	x, e1 := strconv.ParseInt(a, 10, 64)
	y, e2 := strconv.ParseInt(b, 10, 64)
	if e1 != nil || e2 != nil {
		return false, false
	}
	return x <= y, true
}

// agoOf is `_ago`: "12m ago" under an hour, "5h ago" after.
func agoOf(epoch, now int64) string {
	m := (now - epoch) / 60
	if m < 60 {
		return itoa(int(m)) + "m ago"
	}
	return itoa(int(m/60)) + "h ago"
}

// dfPercent is `df -kP <dir> | awk 'NR == 2 { sub("%", "", $5); print $5 }'`.
func dfPercent(dir string) string {
	r := extRun(extCall{Name: "df", Args: []string{"-kP", dir}})
	lines := strings.Split(r.Out, "\n")
	if len(lines) < 2 {
		return ""
	}
	f := strings.Fields(lines[1])
	if len(f) < 5 {
		return ""
	}
	return strings.Replace(f[4], "%", "", 1)
}

// hygieneQuiet: A LANE PAST ITS SIZE, ONE LINE, LIVE. A clean lane prints nothing.
func (a *app) hygieneQuiet() int {
	for _, proj := range a.c.t.survey {
		main := a.c.checkout(proj)
		if main == "" || !isDir(main) { // _repo_lanes, then `[ -d "$main" ]`
			continue
		}
		maxn := a.getenv("HW_HYGIENE_MAX_WORKTREES")
		if maxn == "" {
			maxn = a.c.t.get(proj, "reap_max_worktrees")
		}
		if maxn == "" {
			maxn = "40"
		}
		maxd := a.getenv("HW_HYGIENE_MAX_DISK_PCT")
		if maxd == "" {
			maxd = a.c.t.get(proj, "reap_max_disk_pct")
		}
		if maxd == "" {
			maxd = "85"
		}
		count := 0
		listing, _ := gitOut(main, "worktree", "list", "--porcelain")
		for _, l := range strings.Split(listing, "\n") {
			if strings.HasPrefix(l, "worktree ") {
				count++
			}
		}
		n := count - 1
		pct := dfPercent(main)
		why := ""
		if le, _ := bashLE(itoa(n), maxn); !le {
			why = itoa(n) + " worktrees (threshold " + maxn + ")"
		}
		if pct != "" {
			if le, _ := bashLE(pct, maxd); !le {
				if why != "" {
					why += ", "
				}
				why += "disk " + pct + "% used (threshold " + maxd + "%)"
			}
		}
		maxg := a.getenv("HW_HYGIENE_MAX_BUILD_GB")
		if maxg == "" {
			maxg = a.c.t.get(proj, "reap_max_build_gb")
		}
		if maxg == "" {
			maxg = "30"
		}
		gb, at, safe := "", "", ""
		if body, ok := readText(filepath.Join(a.c.brain, ".hw-reap-"+proj)); ok {
			f := strings.SplitN(body, "\n", 2)[0]
			w := splitSpace(f, 3) // `read -r gb at safe`: the default IFS
			gb, at, safe = w[0], w[1], w[2]
		}
		if strings.ContainsFunc(gb+at+safe, func(r rune) bool { return r < '0' || r > '9' }) {
			gb, at, safe = "", "", ""
		}
		if gb != "" {
			v, _ := strconv.ParseInt(gb, 10, 64)
			gb = strconv.FormatInt(v/1048576, 10)
			if le, _ := bashLE(gb, maxg); !le {
				if why != "" {
					why += ", "
				}
				why += gb + " GB of .next/.turbo at the last reap (threshold " + maxg + ")"
			}
		}
		if safe != "" && safe != "0" {
			if why != "" {
				why += ", "
			}
			t, _ := strconv.ParseInt(at, 10, 64)
			why += safe + " disposable at the last reap (" + agoOf(t, time.Now().Unix()) + ")"
		}
		if why != "" {
			a.warn(proj + ": " + why + " — hw reap " + proj + " --apply")
		}
	}
	return 0
}

// splitSpace is `read -r f1 … fn` under the default IFS (space, tab, newline): runs are one separator, the last field
// keeps the rest of the line.
func splitSpace(line string, n int) []string {
	out := make([]string, n)
	rest := strings.Trim(line, " \t\n")
	for i := 0; i < n; i++ {
		if i == n-1 {
			out[i] = rest
			break
		}
		j := strings.IndexAny(rest, " \t\n")
		if j < 0 {
			out[i] = rest
			rest = ""
			continue
		}
		out[i] = rest[:j]
		rest = strings.TrimLeft(rest[j:], " \t\n")
	}
	return out
}

// ── _rules_quiet ────────────────────────────────────────────────────────────

// pySpace is Python's str.isspace over a character.
func pySpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

// ruleBlocks is `re.split(r"\n\s*\n", text)` with each block stripped: the text cut at every whitespace run that holds
// two newlines or more.
func ruleBlocks(text string) []string {
	var blocks []string
	rs := []rune(text)
	start, i := 0, 0
	for i < len(rs) {
		if !pySpace(rs[i]) {
			i++
			continue
		}
		j, nl := i, 0
		for j < len(rs) && pySpace(rs[j]) {
			if rs[j] == '\n' {
				nl++
			}
			j++
		}
		if nl >= 2 {
			blocks = append(blocks, string(rs[start:i]))
			start = j
		}
		i = j
	}
	blocks = append(blocks, string(rs[start:]))
	for k, b := range blocks {
		blocks[k] = strings.TrimFunc(b, pySpace)
	}
	return blocks
}

// rulesQuiet: A paragraph copied into three or more project CLAUDE.md files gets corrected in some of them and left
// wrong in the rest. Three or more means it belongs in CLAUDE.shared.md; two is left alone.
func (a *app) rulesQuiet() int {
	seen := map[string]int{}
	for _, p := range a.c.t.names {
		body, ok := readText(filepath.Join(a.c.brain, p, "CLAUDE.md"))
		if !ok {
			continue
		}
		if !utf8.ValidString(body) { // the Python died decoding it, and `|| true` made that "0"
			return 0
		}
		body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n") // universal newlines
		for _, b := range ruleBlocks(body) {
			if utf8.RuneCountInString(b) > 80 {
				seen[b]++
			}
		}
	}
	n := 0
	for _, c := range seen {
		if c >= 3 {
			n++
		}
	}
	if n <= 0 {
		return 0
	}
	a.printf("\n")
	a.warn(itoa(n) + " paragraph(s) are copied into 3+ project CLAUDE.md files — they belong in CLAUDE.shared.md")
	a.info("a rule in four files gets corrected in two of them; that is how a stale rule survives")
	return 0
}

// ── _decisions_quiet ────────────────────────────────────────────────────────

// decisionsQuiet: THE EXIT CODE SAYS WHICH PROBLEM IT IS, so this says which problem it is.
func (a *app) decisionsQuiet() int {
	r := extRun(extCall{Name: filepath.Join(a.binDir(), "decisions"), Args: []string{"check"}})
	out := subst(r.Out)
	if r.RC == 0 || out == "" {
		return 0
	}
	a.printf("\n")
	switch r.RC {
	case 1:
		a.warn("a decisions.md is past the seek budget — an agent told to read it will not:")
	case 3:
		a.warn("a decisions.md directory holds staged temp files from a killed rotation — nothing reads them, and nothing else reports them:")
	case 2:
		a.warn("a decisions.md has headings the tool cannot read — `index` cannot list them, so an agent seeking them will not find them:")
	default:
		a.warn("`decisions check` exited " + itoa(r.RC) + ", which this function does not have words for — read its output rather than trusting a label:")
	}
	for _, l := range strings.Split(out, "\n") {
		a.printf("    %s\n", l)
	}
	return 0
}

// ── _agent_tools_quiet ──────────────────────────────────────────────────────

// agentToolsQuiet: A SUBAGENT THAT DECLARES A TOOL ITS SESSION DOES NOT HAVE STARTS WITHOUT IT, SILENTLY (MEASURED
// 2026-10-09; bin/agent-tools-check says how and why). One line per lane whose agents are broken, empty when none is.
func (a *app) agentToolsQuiet(lane string) int {
	if v := a.getenv("HW_AGENT_TOOLS"); v == "off" {
		return 0
	}
	tool := filepath.Join(a.binDir(), "agent-tools-check")
	if fi, err := os.Stat(tool); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
		return 0
	}
	args := []string{"--root", a.c.brain, "--cached", "--oneline"}
	if lane != "" {
		args = append(args, "--lane", lane)
	}
	r := extRun(extCall{Name: tool, Args: args})
	if r.RC != 0 && r.RC != 3 {
		who := lane
		if who == "" {
			who = "ninguna lane en particular"
		}
		a.warn("agent-tools-check no pudo verificar los agentes de " + who + " (¿claude?) — corré `agent-tools-check`")
		return 0
	}
	l := subst(r.Out)
	if l == "" {
		return 0
	}
	a.printf("\n")
	for _, l2 := range strings.Split(l, "\n") {
		a.warn(l2)
	}
	return 0
}
