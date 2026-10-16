package main

// Train T5, area A — the brief and its contract (prefix `br`). Ports, from bin/hw and lib/hw/{briefs,ledger}.sh:
// _infer_brief, _archived_brief_path/_gate, _briefless_gate/_nearest/_archive_hint, _check_task,
// _task_kind_from_name, _check_task_kind, _load_brief_decls/_brief_declares, _dispatch_actual,
// _ensure_sdd_effective_for_contract, _check_brief_contract, _check_requested, _requested_line,
// _check_authorizes, _authorizes_line, _auth_trim, _check_brief_measured_evidence, the credential preflight
// (_keychain_available, _no_keychain_message, _brief_handles, _handle_exists), _brief_verify_cmds,
// _verify_names_full_suite, _VERIFY_TIMEOUT, _brief_boundary, _brief_kind_for_close, _design_gate_applies.
// The agent-name checks of the contract are in t5_brief_agents.go.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// brState is area A's private state: the frontmatter read once (BRIEF_DECLS), ASKED, AUTHORIZES, and whether the
// effective sdd mode has been resolved (SDD_EFFECTIVE_MODE+set).
type brState struct {
	brDeclsRead   bool
	brDecls       string
	brAsked       string
	brAuthorizes  string
	brAuthMarker  string
	brSddResolved bool
	brLaneTried   bool
	brLaneRoot    *jval
}

// ── small helpers ────────────────────────────────────────────────────────────

// brExec runs a program as bash would, stderr discarded, and returns its stdout and exit status (127: not run).
func brExec(stdin, name string, args ...string) (string, int) {
	cmd := exec.Command(name, args...)
	cmd.Env = childEnv()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out.String(), ee.ExitCode()
		}
		return "", 127
	}
	return out.String(), 0
}

// brExecErr is brExec with stderr kept.
func brExecErr(name string, args ...string) (string, string, int) {
	cmd := exec.Command(name, args...)
	cmd.Env = childEnv()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out.String(), errb.String(), ee.ExitCode()
		}
		return "", errb.String(), 127
	}
	return out.String(), errb.String(), 0
}

// brSub is `$(…)`: trailing newlines stripped.
func brSub(s string) string { return strings.TrimRight(s, "\n") }

// brAppendDeclared is BRIEF_DECLARED="${BRIEF_DECLARED}${BRIEF_DECLARED:+ }$w".
func (d *dsp) brAppendDeclared(w string) {
	if d.briefDeclared != "" {
		d.briefDeclared += " "
	}
	d.briefDeclared += w
}

// brReadable is `[ -r path ]`.
func brReadable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// brReadValid is Python's open(path, encoding="utf-8", errors="replace").read().
func brDecodeReplace(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for i := 0; i < len(b); {
		r, n := utf8.DecodeRune(b[i:])
		if r != utf8.RuneError || n != 1 {
			sb.WriteRune(r)
			i += n
			continue
		}
		// an ill-formed sequence: one U+FFFD for the maximal subpart of a valid sequence (what CPython does)
		sb.WriteRune(utf8.RuneError)
		i += brIllFormedLen(b[i:])
	}
	return sb.String()
}

func brIllFormedLen(b []byte) int {
	lo, hi := byte(0x80), byte(0xbf)
	need := 0
	switch c := b[0]; {
	case c >= 0xc2 && c <= 0xdf:
		need = 1
	case c == 0xe0:
		need, lo = 2, 0xa0
	case c >= 0xe1 && c <= 0xec, c == 0xee, c == 0xef:
		need = 2
	case c == 0xed:
		need, hi = 2, 0x9f
	case c == 0xf0:
		need, lo = 3, 0x90
	case c >= 0xf1 && c <= 0xf3:
		need = 3
	case c == 0xf4:
		need, hi = 3, 0x8f
	default:
		return 1
	}
	n := 1
	for k := 0; k < need && n < len(b); k++ {
		if b[n] < lo || b[n] > hi {
			break
		}
		lo, hi = 0x80, 0xbf
		n++
	}
	return n
}

// ── lane values the Go table does not hold (project-spaces.sh lane_config_load) ──

// brTable is the lane table file, parsed once (the fields lanes.go does not keep are read from it).
func (d *dsp) brTable() *jval {
	if !d.brLaneTried {
		d.brLaneTried = true
		if data, err := os.ReadFile(d.c.t.file); err == nil {
			if vals, perr := parseStream(data); perr == nil && len(vals) > 0 {
				d.brLaneRoot = vals[0]
			}
		}
	}
	return d.brLaneRoot
}

// brLaneGet is lane_get for the fields area A reads that lanes.go does not keep: requested_by, agents_from,
// base_ref_prefix and repoless, as lane_config_load emits them (project-spaces.sh).
func (d *dsp) brLaneGet(lane, key string) string {
	if !d.c.t.known(lane) {
		return ""
	}
	l := field(field(d.brTable(), "lanes"), lane)
	switch key {
	case "requested_by", "agents_from":
		if l == nil {
			return d.c.t.get(lane, key)
		}
		return orDefault(field(l, key), "")
	case "base_ref_prefix":
		if l == nil {
			return "origin/"
		}
		return orDefault(field(l, key), "origin/")
	case "repoless":
		if l == nil {
			return d.c.t.get(lane, key)
		}
		if v := field(l, key); v != nil && v.kind == vBool && v.b {
			return "1"
		}
		return ""
	}
	return d.c.t.get(lane, key)
}

// brOperator is ${HW_OPERATOR:-the operator}: the environment, else the table's `operator`.
func (d *dsp) brOperator() string {
	if v := d.env("HW_OPERATOR"); v != "" {
		return v
	}
	if v := orDefault(field(d.brTable(), "operator"), "the operator"); v != "" {
		return v
	}
	return "the operator"
}

func (d *dsp) brIsProductRepo() bool { return d.c.t.get(d.proj, "product_repo") == "1" } // _lane_is_product_repo
func (d *dsp) brLaneBase() string    { return d.c.t.get(d.proj, "base") }                // _lane_base
func (d *dsp) brAgentsRef() string { // _lane_agents_ref
	return d.brLaneGet(d.proj, "base_ref_prefix") + d.brLaneBase()
}

// brTaskBase is _task_base: --base, else the lane's.
func (d *dsp) brTaskBase() string {
	if d.baseOverride != "" {
		return d.baseOverride
	}
	return d.brLaneBase()
}

// brSurfaceDir is _surface_dir: where the framework mode is read from.
func (d *dsp) brSurfaceDir() string {
	if d.brIsProductRepo() {
		return d.c.checkout(d.proj)
	}
	return d.intendedWorkdir()
}

// ── the brief: inferred, archived, absent ───────────────────────────────────

// inferBrief is _infer_brief: brain/<lane>/briefs/<task>.md when it is a file.
func (d *dsp) inferBrief() string {
	cand := d.c.brain + "/" + d.proj + "/briefs/" + d.task + ".md"
	if isFile(cand) { // # MUTATION-ANCHOR: 844-A01
		return cand
	}
	return ""
}

// brArchivedPath is _archived_brief_path: briefs/archive/*/<task>.md, the last of the sorted months.
func (d *dsp) brArchivedPath(lane, task string) string {
	root := d.c.brain + "/" + lane + "/briefs/archive"
	ents, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	brSortBytes(names)
	hit := ""
	for _, n := range names {
		if f := root + "/" + n + "/" + task + ".md"; isFile(f) {
			hit = f // # MUTATION-ANCHOR: 844-A03
		}
	}
	return hit
}

// archivedBriefGate is _archived_brief_gate.
func (d *dsp) archivedBriefGate(lane, task string) {
	hit := d.brArchivedPath(lane, task)
	if hit == "" { // # MUTATION-ANCHOR: 844-A02
		return
	}
	d.die(lane + ":" + task + " is ARCHIVED — its brief is " + hit + "\n    restore it:  hw briefs restore " + lane + " " + task + "\n    or run it as it is:  --brief " + hit + "\n    hw never launches an archived task without a brief (--no-brief does not apply to it)")
}

// brieflessGate is _briefless_gate.
func (d *dsp) brieflessGate() {
	if d.brief != "" {
		return
	}
	if d.noBrief { // # MUTATION-ANCHOR: 844-A04
		d.warn("launching " + d.proj + ":" + d.task + " WITHOUT a brief (--no-brief): the executor has no contract, no verification and no authorization to quote")
		return
	}
	want := d.c.brain + "/" + d.proj + "/briefs/" + d.task + ".md"
	near := ""
	for _, n := range d.brBrieflessNearest(d.proj, d.task) {
		near += n + " " // `| tr '\n' ' '`
	}
	hint := d.brBrieflessArchiveHint(d.proj, d.task)
	msg := "no brief for " + d.proj + ":" + d.task + " — looked for " + want
	if near != "" { // # MUTATION-ANCHOR: 844-A05
		msg += "\n    nearest names in " + d.proj + "/briefs: " + near
	}
	if hint != "" { // # MUTATION-ANCHOR: 844-A06
		msg += "\n    " + hint
	}
	d.die(msg + "\n    point at one with --brief <path>, or launch on purpose without a contract with --no-brief")
}

// brBrieflessNearest is _briefless_nearest: difflib.get_close_matches(task, names, n=5, cutoff=0.5).
func (d *dsp) brBrieflessNearest(lane, task string) []string {
	set := map[string]bool{}
	for _, dir := range []string{d.c.brain + "/" + lane + "/briefs"} {
		for _, f := range brGlobMd(dir) {
			set[f] = true
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			sub := dir + "/" + e.Name()
			if !pyIsDir(sub) { // # MUTATION-ANCHOR: 844-A157
				continue
			}
			for _, f := range brGlobMd(sub) {
				set[f] = true
			}
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	brSortBytes(names)
	return brGetCloseMatches(task, names, 5, 0.5) // # MUTATION-ANCHOR: 844-A07 // # MUTATION-ANCHOR: 844-A08
}

// brGlobMd is glob.glob(dir + "/*.md") reduced to the names the caller keeps: basename minus ".md", no `_` prefix.
func brGlobMd(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".md") {
			continue
		}
		n = n[:len(n)-3]
		if strings.HasPrefix(n, "_") { // # MUTATION-ANCHOR: 844-A09
			continue
		}
		out = append(out, n)
	}
	return out
}

// brBrieflessArchiveHint is _briefless_archive_hint (fd and git, called as bash calls them).
func (d *dsp) brBrieflessArchiveHint(lane, task string) string {
	out, _ := brExec("", "fd", "-t", "f", "-g", task+".md", d.c.brain+"/"+lane+"/briefs")
	if hit := brFirstLine(out); hit != "" { // # MUTATION-ANCHOR: 844-A10
		return "it exists at " + hit + " (moved into a subfolder?): pass it with --brief"
	}
	out, _ = brExec("", "git", "-C", d.c.brain, "log", "-1", "--format=%h", "--diff-filter=D", "--name-only", "--", lane+"/briefs/"+task+".md", lane+"/briefs/*/"+task+".md")
	if hit := brFirstLine(out); hit != "" { // # MUTATION-ANCHOR: 844-A11
		return "it was deleted or moved in git history: git -C " + d.c.brain + " log --all --stat -- " + lane + "/briefs/*" + task + ".md"
	}
	return ""
}

// brFirstLine is `| head -1` inside `$(…)`.
func brFirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func brSortBytes(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// brSortU is `sort -u` as the dispatch's own environment runs it (the collation is the locale's).
func brSortU(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out, rc := brExec(strings.Join(lines, "\n")+"\n", "sort", "-u")
	if rc != 0 {
		set := map[string]bool{}
		var u []string
		for _, l := range lines {
			if !set[l] {
				set[l] = true
				u = append(u, l)
			}
		}
		brSortBytes(u)
		return u
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

// ── the task name ───────────────────────────────────────────────────────────

// brBashEval runs a bash snippet with arguments and returns its stdout and status: the locale-dependent classes of
// bash's own patterns ([A-Za-z] ranges, [[:space:]]) are bash's to answer for characters outside ASCII.
func brBashEval(script string, args ...string) (string, int) {
	return brExec("", "bash", append([]string{"-c", script, "bash"}, args...)...)
}

func brIsASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// brTaskHasOtherChars is `case "$1" in *[!A-Za-z0-9._-]*)`: ASCII decided here, anything else asked of bash
// (under a UTF-8 locale a range like A-Z holds accented letters, and which ones is the platform's).
func brTaskHasOtherChars(task string) bool {
	for i := 0; i < len(task); i++ {
		c := task[i]
		if c < 0x80 && !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return true
		}
	}
	if brIsASCII(task) {
		return false
	}
	_, rc := brBashEval(`case "$1" in *[!A-Za-z0-9._-]*) exit 1 ;; esac`, task)
	return rc != 0
}

// checkTask is _check_task.
func (d *dsp) checkTask(task string) {
	if task == "" {
		d.die("<task> is empty")
	}
	if strings.HasPrefix(task, "-") { // # MUTATION-ANCHOR: 844-A12
		d.die("<task> must not start with a dash: " + task)
	}
	if brTaskHasOtherChars(task) { // case "$1" in *[!A-Za-z0-9._-]*) // # MUTATION-ANCHOR: 844-A13
		d.die("<task> may only contain letters, digits, dot, underscore and dash: " + task)
	}
	if task == "." || task == ".." { // # MUTATION-ANCHOR: 844-A14
		d.die("<task> must not be . or ..")
	}
	const lc = "abcdefghijklmnopqrstuvwxyz"
	if !strings.ContainsRune(lc, rune(task[0])) { // # MUTATION-ANCHOR: 844-A15
		d.die("<task> must start with a lowercase letter (herdr agent-name rule): " + task)
	}
	for _, r := range task {
		if !strings.ContainsRune(lc+"0123456789_-", r) { // # MUTATION-ANCHOR: 844-A16
			d.die("<task> may only contain lowercase letters, digits, underscore and dash (herdr agent-name rule): " + task)
		}
	}
	if len(task) > 32 { // # MUTATION-ANCHOR: 844-A17
		d.die("<task> is " + brItoa(len(task)) + " characters; herdr agent names cap at 32. Shorten it: " + task)
	}
}

func brItoa(n int) string { return strconv.Itoa(n) }

// brTaskKindFromName is _task_kind_from_name.
func brTaskKindFromName(t string) string {
	for _, s := range []string{"diagnosis", "diagnose", "audit", "explore", "research", "review", "investigation"} { // # MUTATION-ANCHOR: 844-A18
		if strings.Contains(t, s) {
			return "explore"
		}
	}
	if strings.HasSuffix(t, "-diag") || strings.Contains(t, "-diag-") {
		return "explore"
	}
	return ""
}

// checkTaskKind is _check_task_kind (area B provides modelIsBuildOnly).
func (d *dsp) checkTaskKind(kind, how string) {
	switch kind {
	case "build", "explore", "audit", "review", "design": // # MUTATION-ANCHOR: 844-A19
	default:
		d.warn("brief declares kind=" + kind + ", which hw does not know. Known: build explore audit review design") // # MUTATION-ANCHOR: 844-A20
		return
	}
	d.taskKind, d.taskKindSrc = kind, how
	if kind == "build" {
		if how == "declared" {
			d.brAppendDeclared("kind") // # MUTATION-ANCHOR: 844-A21
			d.checkTopTierCeiling()
		}
		return
	}
	if d.model != "" && d.modelIsBuildOnly(d.model) { // # MUTATION-ANCHOR: 844-A23
		if how == "declared" { // # MUTATION-ANCHOR: 844-A24
			d.die("this task is declared kind=" + kind + " and " + d.model + " is a build model — its own runner labels every turn \"Build\". Exploration decides WHAT to build; handing it to the model meant for building the decision is how ten browser diagnoses ran on it on 2026-08-24/25 while the same lane ran the same class of task on opus. Nothing was built. Relaunch with a model that reasons: --agent claude --model opus, or --model opus if you keep this vendor.")
		}
		src := d.modelSrc
		if src == "" {
			src = "lane default"
		}
		d.warn("task name suggests this is exploration (" + kind + ") but the model is " + d.model + ", which is a build model (" + src + ").")
		d.warn("the lane pin is right for building and wrong for deciding what to build. If this really is exploration: --agent claude --model opus.")
		d.warn("if it is not, say so in the brief (kind: build) and this stops asking.") // # MUTATION-ANCHOR: 844-A25
	} else if how == "declared" {
		d.brAppendDeclared("kind") // # MUTATION-ANCHOR: 844-A22
	}
}

// ── the frontmatter ─────────────────────────────────────────────────────────

var brKnownDecl = map[string]bool{"agent": true, "model": true, "sdd": true, "placement": true, "worktree": true, "database": true,
	"invoker": true, "requires": true, "requires-agents": true, "kind": true, "base": true, "decision": true, "delivery": true,
	"boundary": true, "deployed_check": true, "sandbox": true, "review": true, "plugins": true, "requested_by": true, "asked": true, "authorizes": true, "secrets": true,
	"top_tier_why": true}

// brNearestKnown is the `dist`/`near`/`listed` of _brief_declares' python: the known key nearest to <key> (edit
// distance <= 2, "" when none is that close; the first of the sorted keys on a tie) and every known key, sorted,
// space-separated.
func brNearestKnown(key string) (near, listed string) {
	keys := make([]string, 0, len(brKnownDecl))
	for k := range brKnownDecl {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kr := []rune(key)
	best := -1
	for _, k := range keys {
		if dd := brEditDistance(kr, []rune(k)); best < 0 || dd < best {
			best, near = dd, k
		}
	}
	if best > 2 { // # MUTATION-ANCHOR: 844-910-M02
		near = ""
	}
	return near, strings.Join(keys, " ") // # MUTATION-ANCHOR: 844-910-M01
}

// brEditDistance is the Levenshtein distance of the python helper `dist`.
func brEditDistance(a, b []rune) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i, ca := range a {
		prev := row[0]
		row[0] = i + 1
		for j, cb := range b {
			cost := 0
			if ca != cb {
				cost = 1
			}
			cur := min(row[j+1]+1, row[j]+1, prev+cost)
			prev, row[j+1] = row[j+1], cur
		}
	}
	return row[len(b)]
}

// brHasKey is _brief_has_key: <decls> declares <key> by the first column only.
func brHasKey(decls, key string) bool {
	for _, line := range strings.Split(decls, "\n") {
		first, _, _ := strings.Cut(line, "\t")
		if first == key { // # MUTATION-ANCHOR: 844-910-M03
			return true
		}
	}
	return false
}

// brBriefDeclares is _brief_declares: "key<TAB>value" per line, as the python of bin/hw prints them.
func brBriefDeclares(path string) string {
	if !isFile(path) {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := brDecodeReplace(data)
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n") // universal newlines
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || pyStrip(lines[0]) != "---" {
		return ""
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 { // # MUTATION-ANCHOR: 844-A26
		return "__malformed__\tunterminated frontmatter: no closing --- found"
	}
	var out []string
	for _, raw := range lines[1:end] {
		line := pyStrip(raw)
		if line == "" || strings.HasPrefix(line, "#") { // # MUTATION-ANCHOR: 844-A28
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found { // # MUTATION-ANCHOR: 844-A27
			out = append(out, "__malformed__\tnot a key: value pair: "+line)
			continue
		}
		key = strings.ToLower(pyStrip(key))
		value = strings.Trim(pyStrip(value), "\"'") // # MUTATION-ANCHOR: 844-A29
		if key == "kind" {
			value = strings.ToLower(value) // # MUTATION-ANCHOR: 844-A30
		}
		if value == "" {
			continue
		}
		if !brKnownDecl[key] { // # MUTATION-ANCHOR: 844-A31
			near, listed := brNearestKnown(key)
			out = append(out, "__unknown__\t"+key+"\x1f"+near+"\x1f"+listed)
			continue
		}
		out = append(out, key+"\t"+value)
	}
	return strings.Join(out, "\n")
}

// loadBriefDecls is _load_brief_decls: the frontmatter read once, BRIEF_DECLS returned (no trailing newline).
// Area C reads it for `delivery`, `review` and `sdd` (the gentle gate and the sdd model gate).
func (d *dsp) loadBriefDecls() string {
	if !d.brDeclsRead {
		d.brDecls = brBriefDeclares(d.brief)
		d.brDeclsRead = true
	}
	return d.brDecls
}

// briefDecl is `printf '%s\n' "$BRIEF_DECLS" | awk -F'\t' '$1 == KEY { print $2; exit }'`.
func (d *dsp) briefDecl(key string) string {
	d.loadBriefDecls()
	for _, l := range strings.Split(d.brDecls, "\n") {
		f := strings.Split(l, "\t")
		if f[0] == key {
			if len(f) > 1 {
				return f[1]
			}
			return ""
		}
	}
	return ""
}

// brReadKV is `IFS=$'\t' read -r key val`.
func brReadKV(line string) (string, string) {
	line = strings.TrimLeft(line, "\t")
	key, rest, found := strings.Cut(line, "\t")
	if !found {
		return key, ""
	}
	return key, strings.TrimRight(strings.TrimLeft(rest, "\t"), "\t")
}

// brDispatchActual is _dispatch_actual: the resolved value of one declared key.
func (d *dsp) brDispatchActual(key string) string {
	switch key {
	case "agent":
		return d.agent // # MUTATION-ANCHOR: 844-A32
	case "model":
		return orDefaultStr(d.model, "<agent default>")
	case "sdd":
		if d.brSddIsSet() {
			return orDefaultStr(d.sddEffectiveMode, "none")
		}
		return orDefaultStr(d.sdd, "none")
	case "placement":
		if d.here { // # MUTATION-ANCHOR: 844-A33
			return "here"
		} else if d.tab { // # MUTATION-ANCHOR: 844-A34
			return "project-tab"
		}
		return "legacy-space"
	case "worktree":
		if !d.wantWT { // # MUTATION-ANCHOR: 844-A35
			return "none"
		}
		return "new"
	case "database":
		if !d.wantDB { // # MUTATION-ANCHOR: 844-A36
			return "main"
		}
		return "own"
	case "invoker":
		return d.invokerPane() // # MUTATION-ANCHOR: 844-A37
	case "base":
		return d.brTaskBase() // # MUTATION-ANCHOR: 844-A38
	}
	return ""
}

// brSddIsSet is [ -n "${SDD_EFFECTIVE_MODE+set}" ]: the launch route resolves it (_validate_sdd_vendor_pair) before
// the contract; the reuse route does not, until _ensure_sdd_effective_for_contract does.
func (d *dsp) brSddIsSet() bool { return d.reusePane == "" || d.brSddResolved }

// brResolveEffectiveSdd is _resolve_effective_sdd_mode, which area C owns (adapter: the one place that names it).
func (d *dsp) brResolveEffectiveSdd(requested, surface, inherited string) {
	d.resolveEffectiveSddMode(requested, surface, inherited)
	d.brSddResolved = true
}

// ensureSddEffectiveForContract is _ensure_sdd_effective_for_contract.
func (d *dsp) ensureSddEffectiveForContract() {
	if d.brSddIsSet() {
		return
	}
	surface, inherited := "", ""
	if d.reusePane != "" {
		if d.brIsProductRepo() {
			surface = d.brSurfaceDir()
		} else {
			surface = filepath.Dir(filepath.Dir(d.reuseRundir))
		}
		inherited = d.runDispatchFramework(d.reuseRundir)
	} else {
		surface = d.codexValidationSurface()
	}
	d.brResolveEffectiveSdd(d.sdd, surface, inherited)
}

// checkBriefContract is _check_brief_contract.
func (d *dsp) checkBriefContract() {
	bad, requiredAgentBad := false, false
	decls := d.loadBriefDecls()
	inferred := brTaskKindFromName(d.task)
	if decls == "" {
		d.briefDeclared = "none"
		if inferred != "" { // # MUTATION-ANCHOR: 844-A39
			d.checkTaskKind(inferred, "inferred")
		}
		d.warnBriefBodyAgents() // # MUTATION-ANCHOR: 844-A158
		return
	}
	if !brHasKey(decls, "kind") && inferred != "" {
		d.checkTaskKind(inferred, "inferred")
	}
	for _, line := range strings.Split(decls, "\n") {
		key, val := brReadKV(line)
		if key == "" {
			continue
		}
		switch key {
		case "__malformed__": // # MUTATION-ANCHOR: 844-A40
			d.die("the brief's frontmatter is malformed (" + val + "). Fix it or remove the fence entirely — a half-parsed contract is worse than none.")
		case "__unknown__":
			uk, rest, _ := strings.Cut(val, "\x1f")
			near, known, _ := strings.Cut(rest, "\x1f")
			suggest := ""
			if near != "" {
				suggest = " — did you mean '" + near + "'?"
			}
			d.warn("brief declares '" + uk + "', which hw does not check" + suggest + ". Known keys: " + known)
			continue // # MUTATION-ANCHOR: 844-A160
		}
		if key == "requires" { // # MUTATION-ANCHOR: 844-A41
			d.checkCapabilities(val)
			continue
		}
		if key == "plugins" { // # MUTATION-ANCHOR: 844-P08
			d.checkPlugins()
			continue
		}
		if key == "requires-agents" { // # MUTATION-ANCHOR: 844-A42
			if !d.checkRequiredAgents(val) {
				bad, requiredAgentBad = true, true
			}
			continue
		}
		if key == "kind" { // # MUTATION-ANCHOR: 844-A43
			d.checkTaskKind(val, "declared")
			continue
		}
		if key == "sandbox" {
			switch val {
			case "true", "yes": // # MUTATION-ANCHOR: 844-A44
				d.sandbox = true // # MUTATION-ANCHOR: 844-A45
				if d.sandboxSrc == "" {
					d.sandboxSrc = "brief frontmatter"
				}
			case "false", "no": // # MUTATION-ANCHOR: 844-A46
			default:
				d.die("the brief declares sandbox: " + val + " — it takes true or false. NOTHING was built.") // # MUTATION-ANCHOR: 844-A47
			}
			d.brAppendDeclared("sandbox") // # MUTATION-ANCHOR: 844-A48
			continue
		}
		if key == "requested_by" || key == "asked" || key == "authorizes" || key == "secrets" { // # MUTATION-ANCHOR: 844-A49
			continue
		}
		if key == "delivery" {
			if d.sdd == "gentle" { // # MUTATION-ANCHOR: 844-A50
				d.brAppendDeclared("delivery")
			}
			continue
		}
		if key == "review" {
			if d.sdd == "gentle" { // # MUTATION-ANCHOR: 844-A51
				d.brAppendDeclared("review")
			} else {
				d.warn("brief declares review: on a --sdd " + orDefaultStr(d.sdd, "<lane default>") + " dispatch. Only --sdd gentle reads it, so it is NOT recorded here")
			}
			continue
		}
		if key == "deployed_check" {
			if d.sdd == "gentle" { // # MUTATION-ANCHOR: 844-A52
				d.brAppendDeclared("deployed_check")
			} else {
				d.warn("brief declares deployed_check: on a --sdd " + orDefaultStr(d.sdd, "<lane default>") + " dispatch. Only --sdd gentle holds a report to it, so it is NOT recorded here")
			}
			continue
		}
		if key == "boundary" {
			if d.designGateApplies() { // # MUTATION-ANCHOR: 844-A53
				d.brAppendDeclared("boundary")
			} else {
				d.warn("brief declares boundary: on a kind: " + d.briefKindForClose() + " task. A design judgment gates code, and this task writes none, so it is NOT recorded and done-invoker will not ask for one")
			}
			continue
		}
		if key == "top_tier_why" { // a reason, read by checkTopTierCeiling: here it only counts as declared
			d.brAppendDeclared("top_tier_why")
			continue
		}
		if key == "decision" {
			if d.modelWhy == "" {
				d.modelWhy, d.modelWhySrc = val, "brief"
			}
			d.brAppendDeclared("decision") // # MUTATION-ANCHOR: 844-A161
			continue
		}
		if key == "base" && d.brLaneBase() == "" && d.baseOverride == "" { // # MUTATION-ANCHOR: 844-A54
			d.warn("brief declares base=" + val + " but " + d.proj + " has no repository, so there is no base branch to build on")
			bad = true
			continue
		}
		if key == "sdd" {
			d.ensureSddEffectiveForContract()
		}
		actual := d.brDispatchActual(key)
		if val != actual { // # MUTATION-ANCHOR: 844-A55
			d.warn("brief declares " + key + "=" + val + " but this dispatch resolves " + key + "=" + actual)
			bad = true
		} else {
			d.brAppendDeclared(key)
		}
	}
	if bad { // # MUTATION-ANCHOR: 844-A57
		if requiredAgentBad { // # MUTATION-ANCHOR: 844-A56
			d.die("the brief requires a named agent this " + d.agent + " runtime cannot invoke (see above). NOTHING was built — no worktree, no database, no pane. Choose a vendor that defines that agent, or change the brief if the mechanism requirement is wrong.")
		}
		d.die("the brief and the flags disagree (see above). NOTHING was built — no worktree, no database, no pane. Either fix the flags, or change the brief if the brief is what is wrong. This check exists because on 2026-08-24 a brief asking for OpenCode in a project tab was dispatched as Claude in a legacy space, and nothing compared the two.")
	}
	d.warnBriefBodyAgents() // # MUTATION-ANCHOR: 844-A159
	if d.briefDeclared == "" {
		d.briefDeclared = "none"
	}
}

// ── what the operator asked for ─────────────────────────────────────────────

// checkRequested is _check_requested.
func (d *dsp) checkRequested() {
	mode := d.brLaneGet(d.proj, "requested_by")
	decls := d.loadBriefDecls()
	kindDeclared := ""
	for _, line := range strings.Split(decls, "\n") {
		key, val := brReadKV(line)
		switch key {
		case "requested_by":
			if d.requestedBy == "" { // # MUTATION-ANCHOR: 844-A58
				d.requestedBy, d.requestedBySrc = val, "brief"
			}
		case "asked":
			d.brAsked = val
		case "kind":
			kindDeclared = val
		}
	}
	switch d.brAsked {
	case "", "build":
	case "investigate":
		if kindDeclared == "build" { // # MUTATION-ANCHOR: 844-A60
			d.die("the brief says " + d.brOperator() + " asked to INVESTIGATE (asked: investigate) and declares kind: build. An investigation that implements is the failure of 2026-09-28 this check exists for (setup/decisions.md). NOTHING was built. Make the brief investigate, or cite the request that authorized building (asked: build).")
		}
		if kindDeclared == "" { // # MUTATION-ANCHOR: 844-A61
			d.checkTaskKind("explore", "declared")
		}
	default: // # MUTATION-ANCHOR: 844-A62
		d.die("the brief declares asked=" + d.brAsked + "; hw knows investigate and build. NOTHING was built — a gate field hw cannot read is not one it can hold.")
	}
	if d.requestedBy == "" { // # MUTATION-ANCHOR: 844-A59
		switch mode {
		case "required": // # MUTATION-ANCHOR: 844-A63
			d.die(d.proj + " requires every dispatch to cite the request it answers, and this one cites none. NOTHING was built. Add `requested_by: <who, when, and the request in their words>` to the brief's frontmatter (or pass --requested-by \"...\"). If nobody asked for this work, it is a proposal: say it to " + d.brOperator() + " and do not launch it. Fiscal, legal and billing work is a proposal until it is asked for, too.")
		case "warn": // # MUTATION-ANCHOR: 844-A64
			d.warn("this dispatch cites no request (requested_by:). Work " + d.brOperator() + " did not ask for is proposed, not launched — see brain/CLAUDE.md, \"Decide, then act\".")
		}
		return
	}
	if strings.Contains(d.requestedBy, "\n") {
		d.die("--requested-by must be one line so the manifest and the receipt stay parseable")
	}
	if d.requestedBySrc == "brief" { // # MUTATION-ANCHOR: 844-A66
		if d.briefDeclared == "none" { // # MUTATION-ANCHOR: 844-A82
			d.briefDeclared = ""
		}
		d.brAppendDeclared("requested_by")
		if d.brAsked != "" { // # MUTATION-ANCHOR: 844-A67
			d.briefDeclared += " asked"
		}
	}
}

// requestedLine is _requested_line: the manifest's `requested` field.
func (d *dsp) requestedLine() string {
	if d.requestedBy != "" {
		asked := ""
		if d.brAsked != "" { // # MUTATION-ANCHOR: 844-A68
			asked = ", asked: " + d.brAsked
		}
		return brCutChars(d.requestedBy, 96) + "  (" + d.requestedBySrc + asked + ")" // # MUTATION-ANCHOR: 844-A69
	}
	switch d.brLaneGet(d.proj, "requested_by") {
	case "required":
		return "none — REQUIRED in " + d.proj
	case "warn": // # MUTATION-ANCHOR: 844-A65
		return "none — warned (" + d.proj + " does not require it)"
	}
	return "none — not checked in " + d.proj // # MUTATION-ANCHOR: 844-A70
}

// brCutChars is `printf '%s' "$s" | cut -c1-N` inside `$(…)`, as the system's cut counts characters.
func brCutChars(s string, n int) string {
	out, rc := brExec(s, "cut", "-c1-"+brItoa(n))
	if rc == 0 {
		return brSub(out)
	}
	if r := []rune(s); len(r) > n {
		s = string(r[:n])
	}
	return s
}

// brAuthTrim is _auth_trim: [[:space:]] off both ends (bash's own answer for anything outside ASCII).
func brAuthTrim(v string) string {
	if brIsASCII(v) {
		return strings.Trim(v, " \t\n\v\f\r")
	}
	out, _ := brBashEval(`v="$1"; v="${v#"${v%%[![:space:]]*}"}"; v="${v%"${v##*[![:space:]]}"}"; printf '%s' "$v"`, v)
	return out
}

// checkAuthorizes is _check_authorizes.
func (d *dsp) checkAuthorizes() {
	d.brAuthorizes = ""
	d.brAuthMarker = ""
	decls := d.loadBriefDecls()
	n := 0
	var entries []string
	for _, line := range strings.Split(decls, "\n") {
		key, val := brReadKV(line)
		if key != "authorizes" {
			continue
		}
		n++
		ns := brItoa(n)
		if strings.Count(val, " :: ") < 2 { // # MUTATION-ANCHOR: 844-A71
			d.die("the brief's authorizes: entry " + ns + " is malformed (" + val + "). Shape: authorizes: <destination> :: <operation> :: <handle|none>. NOTHING was built.")
		}
		dest, rest, _ := strings.Cut(val, " :: ")
		dest = brAuthTrim(dest)
		op, handle, _ := strings.Cut(rest, " :: ")
		op, handle = brAuthTrim(op), brAuthTrim(handle)
		if strings.Contains(handle, " :: ") { // # MUTATION-ANCHOR: 844-A72
			d.die("the brief's authorizes: entry " + ns + " has more than three fields (" + val + "). NOTHING was built.")
		}
		if dest == "" || op == "" || handle == "" { // # MUTATION-ANCHOR: 844-A73
			d.die("the brief's authorizes: entry " + ns + " has an empty field (" + val + "). Name the destination, the operation, and a handle or none. NOTHING was built.")
		}
		if handle != "none" && strings.IndexFunc(handle, func(r rune) bool { // # MUTATION-ANCHOR: 844-A74
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-')
		}) >= 0 {
			d.die("the brief's authorizes: entry " + ns + " names the handle '" + handle + "', which is not a Keychain service name (letters, digits, . _ -) nor none. NOTHING was built.")
		}
		entries = append(entries, dest+" :: "+op+" :: "+handle)
		d.brAuthorizes = strings.Join(entries, "\n")
	}
	if n == 0 {
		d.warnAuthorizesMissing()
		return
	}
	if d.requestedBy == "" { // # MUTATION-ANCHOR: 844-A75
		d.die("the brief carries authorizes: but cites no request (requested_by:). An authorization nobody asked for is not delivered. NOTHING was built. Add `requested_by: <who, when, and the request in their words>` (or pass --requested-by).")
	}
	if d.checkHandles && d.keychainAvailable() { // # MUTATION-ANCHOR: 844-A76
		var hs []string
		for _, e := range entries {
			parts := strings.Split(e, " :: ")
			hs = append(hs, parts[2])
		}
		missing := ""
		for _, h := range brSortU(hs) {
			if h == "" || h == "none" { // # MUTATION-ANCHOR: 844-A77
				continue
			}
			if !handleExists(h) { // # MUTATION-ANCHOR: 844-A78
				missing += " " + h
			}
		}
		if missing != "" { // # MUTATION-ANCHOR: 844-A79
			d.die("the authorization names Keychain handles that do not resolve:" + missing + ". NOTHING was built. Store each with security add-generic-password -s <handle> -a " + d.brUser() + " -w '<value>' (check: hw handle <handle>), or pass --no-handle-check.")
		}
	} else if d.checkHandles { // # MUTATION-ANCHOR: 844-A80
		d.warn("the authorization's handles were NOT checked: no keychain backend here")
	}
	if d.briefDeclared == "none" {
		d.briefDeclared = ""
	}
	d.brAppendDeclared("authorizes") // # MUTATION-ANCHOR: 844-A81
}

// brUser is ${USER:-$(id -un)}.
func (d *dsp) brUser() string {
	if u := d.env("USER"); u != "" {
		return u
	}
	out, _ := brExec("", "id", "-un")
	return brSub(out)
}

// brRemoteWords are the generic remote-access markers of _brief_remote_marker; the ones that name this machine's
// hosts and tools are read from <bin>/../setup/remote-markers.txt, which the export leaves out.
var brRemoteWords = []string{"ops-investigator", "prod", "producción", "produccion", "production", "ssh", "VPS"}

// remoteMarker is _brief_remote_marker: the first remote-access marker the brief's body (frontmatter excluded) names,
// word-bounded and case-insensitive, as the brief writes it; "" when none.
func (d *dsp) remoteMarker() string {
	if d.brief == "" {
		return ""
	}
	data, err := os.ReadFile(d.brief)
	if err != nil {
		return ""
	}
	words := append([]string{}, brRemoteWords...)
	if f, err := os.ReadFile(d.env("HW_CORE_BIN_DIR") + "/../setup/remote-markers.txt"); err == nil {
		for _, l := range strings.Split(string(f), "\n") {
			l, _, _ = strings.Cut(l, "#")
			if w := strings.TrimSpace(l); w != "" {
				words = append(words, w)
			}
		}
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				lines = lines[i+1:]
				break
			}
		}
	}
	text := []rune(strings.Join(lines, "\n"))
	for i := range text {
		if i > 0 && brIsWord(text[i-1]) {
			continue
		}
		for _, w := range words {
			wr := []rune(w)
			if i+len(wr) > len(text) {
				continue
			}
			ok := true
			for k, c := range wr {
				if text[i+k] != c && !strings.EqualFold(string(text[i+k]), string(c)) {
					ok = false
					break
				}
			}
			if ok && (i+len(wr) == len(text) || !brIsWord(text[i+len(wr)])) {
				return string(text[i : i+len(wr)])
			}
		}
	}
	return ""
}

// warnAuthorizesMissing is _warn_authorizes_missing: a warning, never a refusal.
func (d *dsp) warnAuthorizesMissing() {
	d.brAuthMarker = d.remoteMarker()
	if d.brAuthMarker == "" {
		return
	}
	d.warn("the brief names '" + d.brAuthMarker + "' but carries no authorizes: entry, so no AUTHORIZATION block reaches the executor.")
	d.warn("the request cited in requested_by: does not authorize anything by itself. If it covers the operation, add `authorizes: <destination> :: <operation> :: <handle|none>`; otherwise the executor will block and ask.")
	d.warn("hw is NOT refusing this launch.")
}

// authorizesLine is _authorizes_line: the manifest's `authorizes` field.
func (d *dsp) authorizesLine() string {
	if d.brAuthorizes == "" {
		if d.brAuthMarker != "" {
			return "none — but the brief names " + d.brAuthMarker
		}
		return "none"
	}
	var sb strings.Builder
	for i, l := range strings.Split(d.brAuthorizes, "\n") {
		if i > 0 {
			sb.WriteString("; ") // # MUTATION-ANCHOR: 844-A83
		}
		sb.WriteString("[" + l + "]") // # MUTATION-ANCHOR: 844-A84
	}
	return sb.String()
}

// ── the measured-evidence lint ──────────────────────────────────────────────

// brS is Python's `\s` for str patterns (str.isspace), as the inside of a character class.
const brS = `\x09-\x0d\x1c-\x20\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`

var (
	brReHeading = regexp.MustCompile(`^##(?:[` + brS + `]|$)`)
	brReClaim   = regexp.MustCompile(`^Claim:[` + brS + `]+[^` + brS + `]`)
	brReReport  = regexp.MustCompile(`^Evidence-Report:[` + brS + `]+engram[` + brS + `]+#[0-9]+$`)
	brReReceipt = regexp.MustCompile(`^Evidence-Receipt:[` + brS + `]+[^` + brS + `:]+:[^` + brS + `:]+[` + brS + `]+key=[A-Za-z0-9_.-]+$`)
	brReCommand = regexp.MustCompile(`^Evidence-Command:[` + brS + `]+[^` + brS + `].*$`)
	brReFile    = regexp.MustCompile(`^Evidence-File:[` + brS + `]+.+:[0-9]+(?:-[0-9]+)?$`)
	brReFence   = regexp.MustCompile("^```[^`]*$")
	brReDollar  = regexp.MustCompile(`^\$[` + brS + `]+[^` + brS + `]`)
	brReDollarS = regexp.MustCompile(`^\$[` + brS + `]+`)
)

type brLn struct {
	n int
	t string
}

// brMeasuredFail runs the python of _check_brief_measured_evidence: "" when the block is fine, else the line it
// prints to stderr ("hw: <path>:<n>: <msg>").
func brMeasuredFail(path string, data []byte) string {
	text := string(data)
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	lines := pySplitlines(text)
	fail := func(n int, msg string) string { return "hw: " + path + ":" + brItoa(n) + ": " + msg }
	i := 0
	for i < len(lines) {
		if pyStrip(lines[i]) != "## MEASURED" {
			i++
			continue
		}
		sectionLine := i + 1
		i++
		var body []brLn
		for i < len(lines) && !brReHeading.MatchString(lines[i]) {
			body = append(body, brLn{i + 1, lines[i]})
			i++
		}
		for len(body) > 0 && pyStrip(body[0].t) == "" {
			body = body[1:]
		}
		for len(body) > 0 && pyStrip(body[len(body)-1].t) == "" {
			body = body[:len(body)-1]
		}
		if len(body) == 0 { // # MUTATION-ANCHOR: 844-A85
			return fail(sectionLine, "empty ## MEASURED section")
		}
		p := 0
		for p < len(body) {
			for p < len(body) && pyStrip(body[p].t) == "" {
				p++
			}
			if p >= len(body) {
				break
			}
			n, claim := body[p].n, body[p].t
			if !brReClaim.MatchString(claim) { // # MUTATION-ANCHOR: 844-A86
				return fail(n, "expected Claim: ...; prose, headings, citations, bare commands, and bare paths do not count")
			}
			p++
			if p >= len(body) || pyStrip(body[p].t) == "" { // # MUTATION-ANCHOR: 844-A87
				return fail(n, "Claim needs an immediately adjacent Evidence-* carrier")
			}
			en, ev := body[p].n, body[p].t
			p++
			if brReReport.MatchString(ev) { // # MUTATION-ANCHOR: 844-A88
				continue
			}
			if brReReceipt.MatchString(ev) { // # MUTATION-ANCHOR: 844-A89
				continue
			}
			kind := ""
			if brReCommand.MatchString(ev) { // # MUTATION-ANCHOR: 844-A90
				kind = "command"
			} else if brReFile.MatchString(ev) { // # MUTATION-ANCHOR: 844-A91
				kind = "file"
			} else if strings.HasPrefix(ev, "Evidence-") { // # MUTATION-ANCHOR: 844-A92
				return fail(en, "unknown Evidence-* carrier")
			} else {
				return fail(en, "Claim needs an immediately adjacent recognized Evidence-* carrier")
			}
			if p >= len(body) || !brReFence.MatchString(body[p].t) { // # MUTATION-ANCHOR: 844-A93
				return fail(en, "Evidence-"+strings.ToUpper(kind[:1])+kind[1:]+" needs an immediately attached fenced block")
			}
			fn := body[p].n
			p++
			var quoted []brLn
			for p < len(body) && body[p].t != "```" {
				quoted = append(quoted, body[p])
				p++
			}
			if p >= len(body) { // # MUTATION-ANCHOR: 844-A94
				return fail(fn, "unterminated evidence fence")
			}
			p++
			if kind == "command" {
				any := false
				for _, q := range quoted {
					if brReDollar.MatchString(q.t) {
						any = true
					}
				}
				if !any { // # MUTATION-ANCHOR: 844-A95
					return fail(fn, "transcript needs a `$ command` line")
				}
				any = false
				for _, q := range quoted {
					if pyStrip(q.t) != "" && !brReDollarS.MatchString(q.t) {
						any = true
					}
				}
				if !any { // # MUTATION-ANCHOR: 844-A96
					return fail(fn, "transcript needs at least one literal output line")
				}
			} else {
				any := false
				for _, q := range quoted {
					if pyStrip(q.t) != "" {
						any = true
					}
				}
				if !any { // # MUTATION-ANCHOR: 844-A97
					return fail(fn, "file quote is empty")
				}
			}
		}
	}
	return ""
}

// checkBriefMeasuredEvidence is _check_brief_measured_evidence.
func (d *dsp) checkBriefMeasuredEvidence() {
	if d.brief == "" { // # MUTATION-ANCHOR: 844-A99
		return
	}
	data, err := os.ReadFile(d.brief)
	bad := err != nil || !utf8.Valid(data) // a traceback in bash; UNESTABLISHED text
	if !bad {
		if msg := brMeasuredFail(d.brief, data); msg != "" { // # MUTATION-ANCHOR: 844-A98
			d.err.Write([]byte(msg + "\n"))
			bad = true
		}
	}
	if bad { // # MUTATION-ANCHOR: 844-A100
		d.die("brief " + d.brief + " has an invalid ## MEASURED block. Accepted: Claim: ... then exactly one of Evidence-Command: ... plus an immediate fenced transcript with a $ command and literal output; Evidence-Report: engram #<numeric-id>; Evidence-Receipt: <project>:<task> key=<named-key>; Evidence-File: <path>:<line-or-range> plus an immediate fenced quote. This lint checks shape, adjacency, and auditable identity only — not authenticity, freshness, semantic support, causality, completeness, or omitted contradictions.")
	}
}

// ── credential handles ──────────────────────────────────────────────────────

var brReHandle = regexp.MustCompile(`security[` + brS + `]+(find-generic-password)([^\n]*?-s[` + brS + `]+(?:"([^"\n]+)"|'([^'\n]+)'|([^` + brS + "`'\"" + `]+)))`)

func brIsWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) }

// brBriefHandles is _brief_handles: the handles the brief names in the one resolver shape.
func brBriefHandles(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(data) {
		return nil
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	var seen []string
	pos := 0
	for pos <= len(text) {
		m := brReHandle.FindStringSubmatchIndex(text[pos:])
		if m == nil {
			break
		}
		endPw := pos + m[3] // end of "find-generic-password": \b follows it
		if endPw < len(text) {
			if r, _ := utf8.DecodeRuneInString(text[endPw:]); brIsWord(r) {
				pos += m[0] + 1
				continue
			}
		}
		handle := ""
		for _, g := range []int{6, 8, 10} {
			if m[g] >= 0 {
				handle = text[pos+m[g] : pos+m[g+1]]
				break
			}
		}
		pos += m[1]
		handle = strings.TrimRight(pyStrip(handle), "`,.;")
		if handle == "" || strings.ContainsAny(handle, "<>{") { // # MUTATION-ANCHOR: 844-A101
			continue
		}
		dup := false
		for _, s := range seen {
			if s == handle {
				dup = true
			}
		}
		if !dup {
			seen = append(seen, handle)
		}
	}
	return seen
}

func (d *dsp) noKeychainMessage() {
	d.warn("no keychain backend on " + unameS() + ": hw cannot tell whether a handle exists")                           // # MUTATION-ANCHOR: 844-A164
	d.warn("the macOS Keychain is the only store hw checks. Here, keep a credential in a file or an env var")           // # MUTATION-ANCHOR: 844-A162
	d.warn("only its consumer can read (chmod 600, outside every repo) and name that in the brief instead of a handle") // # MUTATION-ANCHOR: 844-A163
}

// credentialPreflight is the block of bin/hw before `LABEL="$PROJ:$TASK"`.
func (d *dsp) credentialPreflight() {
	if d.brief != "" && d.checkHandles && !d.keychainAvailable() { // # MUTATION-ANCHOR: 844-A103
		if len(brBriefHandles(d.brief)) > 0 { // # MUTATION-ANCHOR: 844-A102
			d.noKeychainMessage()
			d.warn("the brief's handles were NOT checked; the executor may block on one") // # MUTATION-ANCHOR: 844-A109
		}
	} else if d.brief != "" && d.checkHandles { // # MUTATION-ANCHOR: 844-A104
		missing, seen := "", 0
		for _, h := range brBriefHandles(d.brief) {
			if h == "" {
				continue
			}
			seen++
			if !handleExists(h) { // # MUTATION-ANCHOR: 844-A105
				missing += " " + h
			}
		}
		if missing != "" { // # MUTATION-ANCHOR: 844-A106
			d.warn("the brief names credential handles that do not resolve:" + missing)
			d.warn("store each one:  security add-generic-password -s <handle> -a " + d.brUser() + " -w '<value>'") // # MUTATION-ANCHOR: 844-A166
			d.warn("then re-run. Check a handle any time with: hw handle <handle>")                                 // # MUTATION-ANCHOR: 844-A108
			d.die("not launching " + d.proj + ":" + d.task + " — the executor would be blocked on this. Override with --no-handle-check if the handle is about to exist or that line is an example.")
		}
		if seen > 0 { // # MUTATION-ANCHOR: 844-A107
			d.info("credential handles in the brief: " + brItoa(seen) + ", all present (presence only, no value read)") // # MUTATION-ANCHOR: 844-A165
		}
	}
}

// ── what the manifest reads from the brief ──────────────────────────────────

// brAwkVerify is the awk program of _brief_verify_cmds, as awk reads it. The system's awk runs it: its
// [[:space:]], tolower and ranges are the locale's, and so are its errors on bytes that are not text.
const brAwkVerify = `
    /^#+[ \t]/ {
      if (tolower($0) ~ /(^|[^a-z])(testing|tests|verification|verificaci|verificar|pruebas|how to verify|qa)/) { insec=1; next } # MUTATION-ANCHOR: 844-A114
      insec=0; fence=0; next
    }
    !insec { next } # MUTATION-ANCHOR: 844-A115
    /^[ \t]*` + "```" + `/ { fence = !fence; next } # MUTATION-ANCHOR: 844-A116
    {
      line=$0
      if (!fence && line !~ /^    /) next # MUTATION-ANCHOR: 844-A117
      sub(/^[ \t]+/, "", line) # MUTATION-ANCHOR: 844-A118
      sub(/^\$[ \t]+/, "", line) # MUTATION-ANCHOR: 844-A119
      if (line == "" || line ~ /^#/) next # MUTATION-ANCHOR: 844-A120
      print line
    }
  `

// briefVerifyCmds is _brief_verify_cmds: the commands of the Testing/Verification fences (or indented code), one per
// line. awk failing ends the dispatch the way `set -e` ends bash on `verify_cmds="$(…)"`.
func (d *dsp) briefVerifyCmds(brief string) string {
	if brief == "" || !brReadable(brief) {
		return ""
	}
	out, errText, rc := brExecErr("awk", brAwkVerify, brief)
	if rc != 0 {
		d.err.Write([]byte(errText))
		panic(exitNow{rc})
	}
	return brSub(out)
}

// verifyNamesFullSuite is _verify_names_full_suite.
func (d *dsp) verifyNamesFullSuite(cmd string) bool {
	return strings.Contains(cmd, "setup/test-hw") && !strings.Contains(cmd, "HW_TEST_GATE=fast") // # MUTATION-ANCHOR: 844-A110 // # MUTATION-ANCHOR: 844-A111
}

// brVerifyTimeout is _VERIFY_TIMEOUT="${HW_VERIFY_TIMEOUT_SECONDS:-900}", as the manifest prints it.
func brVerifyTimeout() string {
	if v := os.Getenv("HW_VERIFY_TIMEOUT_SECONDS"); v != "" { // # MUTATION-ANCHOR: 844-A112
		return v
	}
	return "900" // # MUTATION-ANCHOR: 844-A113
}

// brAwkKind and brAwkBoundary are the awk programs of _brief_kind_for_close and _brief_boundary, as awk reads them.
const brAwkKind = `NR==1 { if ($0 != "---") exit; next }
       $0 == "---" { exit }
       tolower($0) ~ /^[[:space:]]*kind[[:space:]]*:/ {
         sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]*#.*$/, ""); gsub(/["'[:space:]]/, "")
         print tolower($0); exit
       }`

const brAwkBoundary = `NR==1 { if ($0 != "---") exit; next }
       $0 == "---" { exit }
       tolower($0) ~ /^[[:space:]]*boundary[[:space:]]*:/ {
         sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]+$/, "");
         if ($0 ~ /^".*"$/ || $0 ~ /^'.*'$/) $0 = substr($0, 2, length($0) - 2);
         print; exit }`

// briefBoundaryOf is _brief_boundary <path>.
func briefBoundaryOf(path string) string {
	if !isFile(path) {
		return ""
	}
	out, _ := brExec("", "awk", brAwkBoundary, path)
	return brSub(out)
}

// briefBoundary is _brief_boundary with $BRIEF.
func (d *dsp) briefBoundary() string { return briefBoundaryOf(d.brief) }

// briefKindForClose is _brief_kind_for_close: the resolved kind, else the frontmatter's own `kind:`.
func (d *dsp) briefKindForClose() string {
	if d.taskKind != "" {
		return d.taskKind
	}
	if !isFile(d.brief) {
		return ""
	}
	out, _ := brExec("", "awk", brAwkKind, d.brief)
	return brSub(out)
}

// designGateApplies is _design_gate_applies.
func (d *dsp) designGateApplies() bool {
	if d.briefBoundary() == "" { // # MUTATION-ANCHOR: 844-A121
		return false
	}
	switch d.briefKindForClose() {
	case "", "build": // # MUTATION-ANCHOR: 844-A122
		return true
	}
	return false
}
