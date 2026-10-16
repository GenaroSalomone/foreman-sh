package main

// Train T5, area C (sdd, gentle): the parts of `--sdd gentle` (gentle-ai's ODD) a dry run reaches. bin/hw
// `gentle_home_dir`, `_gentle_home_refusal`, `_gentle_home_ready`, `_validate_sdd_gentle`, `_gentle_brief_gate`,
// `_gentle_rdd_word`, `_gentle_deployed_check`, `_brief_deployed_check`. Nothing is written: `_exclude_odd` is
// the launch's (`_apply_sdd`) and is not ported.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// gentlePin is GENTLE_VERSION: the `min` of lib/hw/deps.conf's gentle-ai row, the one place the pin lives. bin/hw dies at
// load when the row is absent, before any verb; the dry run says the same and ends.
func (d *dsp) gentlePin() string {
	r, ok := depsRow(d.depsRows(), "gentle-ai")
	if !ok {
		fmt.Fprint(d.err, "hw: lib/hw/deps.conf has no gentle-ai row\n")
		panic(exitNow{1})
	}
	return r.min
}

// gentleHomeDir is `gentle_home_dir`.
func (d *dsp) gentleHomeDir() string {
	if v := d.env("HW_GENTLE_HOME"); v != "" { // # MUTATION-ANCHOR: 844-R159
		return v
	}
	return d.env("HOME") + "/.local/share/hw/gentle-home" // # MUTATION-ANCHOR: 844-R160
}

// rtGit runs `git -C <dir> <args>` with stderr dropped: its stdout (newline-stripped) and whether it succeeded.
func rtGit(dir string, args ...string) (string, bool) {
	r := extRun(extCall{Name: "git", Args: append([]string{"-C", dir}, args...), Env: childEnv()})
	return bashSub(r.Out), r.RC == 0
}

// rtBashDirname is dirname(1) for the paths this file walks up.
func rtBashDirname(p string) string {
	if p == "" {
		return "."
	}
	t := strings.TrimRight(p, "/")
	if t == "" {
		return "/"
	}
	i := strings.LastIndexByte(t, '/')
	if i < 0 {
		return "."
	}
	t = strings.TrimRight(t[:i], "/")
	if t == "" {
		return "/"
	}
	return t
}

func rtHasSpace(s string) bool { return strings.ContainsAny(s, " \t\n\v\f\r") }

// gentleHomeRefusal is `_gentle_home_refusal`: why the destination is refused, and whether it is.
func (d *dsp) gentleHomeRefusal(dir string) (string, bool) {
	if !strings.HasPrefix(dir, "/") { // # MUTATION-ANCHOR: 844-R161
		return "not an absolute path: " + dir, true
	}
	if rtHasSpace(dir) {
		return "the path has whitespace, and hw passes it as one word: " + dir, true
	}
	dir = strings.TrimSuffix(dir, "/")
	home := strings.TrimSuffix(d.env("HOME"), "/")
	if dir == home { // # MUTATION-ANCHOR: 844-R162
		return "it is HOME itself", true
	}
	for _, cfg := range []string{".claude", ".claude-personal", ".claude.json", ".config/opencode", ".codex", ".gentle-ai"} {
		if strings.HasPrefix(dir+"/", home+"/"+cfg+"/") { // # MUTATION-ANCHOR: 844-R163
			return "it is inside ~/" + cfg + ", which another session reads", true
		}
	}
	p := dir
	for !isDir(p) && p != "/" {
		p = rtBashDirname(p)
	}
	if _, ok := rtGit(p, "rev-parse", "--is-inside-work-tree"); ok { // # MUTATION-ANCHOR: 844-R164
		if _, ignored := rtGit(p, "check-ignore", "-q", dir); !ignored {
			top, _ := rtGit(p, "rev-parse", "--show-toplevel")
			return "it is inside a git work tree (" + top + ") and not ignored there", true
		}
	}
	return "", false
}

// rtJqEach is `jq -r '<program>' file` run once per document of the file, with jq's own diagnostics (they go to
// stderr, where the bash lets them through): prog gives a document's output (has=false when `// empty` fell
// through) or the text of the runtime error. The result is the outputs, the stderr text and whether jq would have
// exited 0.
func rtJqEach(path string, prog func(v *jval) (out string, has bool, errMsg string)) (outs []string, diag string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "jq: error: Could not open " + path + ": " + rtStrerror(err) + "\n", false
	}
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	ok = true
	p := &jparser{d: data}
	for {
		p.ws()
		if p.i >= len(p.d) {
			return outs, diag, ok
		}
		v, perr := p.value()
		if perr != nil {
			msg := rtJqSyntax(data)
			if msg == "" {
				msg = perr.Error()
			}
			return outs, diag + "jq: parse error: " + msg + "\n", false // # MUTATION-ANCHOR: 844-R181
		}
		out, has, msg := prog(v)
		if msg != "" {
			// jq reads a line at a time: the position is the number of lines fetched up to the one the value ended on.
			from := max(p.i-1, 0)
			end := len(data)
			if nl := strings.IndexByte(string(data[from:]), '\n'); nl >= 0 {
				end = from + nl + 1
			}
			diag += fmt.Sprintf("jq: error (at %s:%d): %s\n", path, strings.Count(string(data[:end]), "\n"), msg)
			ok = false
			continue
		}
		if has {
			outs = append(outs, out)
		}
	}
}

// rtJqPath is the program `.a.b[0].c // empty` (a string step is a key, an int an index).
func rtJqPath(steps ...any) func(v *jval) (string, bool, string) {
	return func(v *jval) (string, bool, string) {
		cur, msg := rtJqWalk(v, steps)
		if msg != "" {
			return "", false, msg
		}
		if !cur.truthy() {
			return "", false, ""
		}
		return cur.raw(), true, ""
	}
}

// rtJqWalk is rtJqSteps with jq's error text.
func rtJqWalk(v *jval, steps []any) (*jval, string) {
	cur := v
	for _, s := range steps {
		switch x := s.(type) {
		case string:
			n, ok := cur.get(x)
			if !ok {
				return nil, "Cannot index " + kindName(cur) + " with string " + jstr(x).toJSON()
			}
			cur = n
		case int:
			switch cur.kind {
			case vNull:
				cur = jnull
			case vArr:
				if x < len(cur.arr) {
					cur = cur.arr[x]
				} else {
					cur = jnull
				}
			default:
				return nil, "Cannot index " + kindName(cur) + " with number"
			}
		}
	}
	return cur, ""
}

// rtJqOut is `jq -r '<steps> // empty' file`: the outputs of every document, jq's diagnostics, and whether it
// would have exited 0.
func rtJqOut(path string, steps ...any) ([]string, string, bool) {
	return rtJqEach(path, rtJqPath(steps...))
}

// rtJqSteps walks .a.b[0].c: a string is a key, an int an index. null indexes to null; a value that cannot be
// indexed that way is an error.
func rtJqSteps(v *jval, steps []any) (*jval, bool) {
	cur, msg := rtJqWalk(v, steps)
	return cur, msg == ""
}

func rtJoinOuts(outs []string) string { return bashSub(strings.Join(outs, "\n")) }

// rtSha256 is `shasum -a 256 <f> | cut -d' ' -f1` ("" when the file cannot be read).
func rtSha256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// rtHooksLength is `jq -r '.hooks // {} | length'` over one document.
func rtHooksLength(v *jval) (string, bool, string) {
	h, ok := v.get("hooks")
	if !ok {
		return "", false, "Cannot index " + kindName(v) + ` with string "hooks"`
	}
	if !h.truthy() {
		return "0", true, ""
	}
	switch h.kind {
	case vObj:
		return strconv.Itoa(len(h.obj)), true, "" // # MUTATION-ANCHOR: 844-R180
	case vArr:
		return strconv.Itoa(len(h.arr)), true, ""
	case vStr:
		return strconv.Itoa(utf8.RuneCountInString(h.s)), true, ""
	case vNum:
		f, err := strconv.ParseFloat(canonNumber(h.s), 64)
		if err != nil {
			return "", false, "number has no length"
		}
		if f < 0 {
			f = -f
		}
		return strconv.FormatFloat(f, 'f', -1, 64), true, ""
	}
	return "", false, "boolean (true) has no length"
}

// gentleHomeReady is `_gentle_home_ready <dir>`: "" when the directory is a complete home of the pinned
// version, else what is missing.
func (d *dsp) gentleHomeReady(dir string) (string, bool) {
	if why, refused := d.gentleHomeRefusal(dir); refused {
		return why, false
	}
	m := dir + "/manifest.json"
	if !isFile(m) { // # MUTATION-ANCHOR: 844-R165
		return "no " + m, false
	}
	vouts, vdiag, ok := rtJqOut(m, "version")
	io.WriteString(d.err, vdiag) // # MUTATION-ANCHOR: 844-R166
	if !ok {
		return m + " is not readable JSON", false // # MUTATION-ANCHOR: 844-R167
	}
	if rtJoinOuts(vouts) != d.gentlePin() { // # MUTATION-ANCHOR: 844-R168
		return m + " is not version " + d.gentlePin(), false
	}
	f := dir + "/odd-block.md"
	if st, err := os.Stat(f); err != nil || st.Size() <= 0 {
		return "no ODD block at " + f, false
	}
	block, rerr := os.ReadFile(f)
	if rerr != nil || !strings.Contains(string(block), "<!-- gentle-ai:agent-routing -->") {
		return f + " carries no gentle-ai agent-routing block", false
	}
	if rerr == nil && strings.Contains(string(block), "<!-- gentle-ai:orchestrator -->") { // # MUTATION-ANCHOR: 844-R169
		return f + " carries the orchestrator block, and only the ODD protocol travels", false
	}
	shouts, sdiag, _ := rtJqOut(m, "odd_block_sha256")
	io.WriteString(d.err, sdiag)
	if rtSha256(f) != rtJoinOuts(shouts) { // # MUTATION-ANCHOR: 844-R170
		return f + " changed after provisioning", false
	}
	if !isFile(dir + "/plugin/gentle-odd/.claude-plugin/plugin.json") {
		return "no plugin at " + dir + "/plugin/gentle-odd", false
	}
	if !isFile(dir + "/guard-settings.json") {
		return "no " + dir + "/guard-settings.json", false
	}
	task := dir + "/bin/gentle-ai-task"
	taskOK := false
	if st, err := os.Stat(task); err == nil && st.Mode()&0o111 != 0 {
		if b, err := os.ReadFile(task); err == nil && strings.Contains(string(b), "rdd-log.jsonl") {
			taskOK = true
		}
	}
	if !taskOK { // # MUTATION-ANCHOR: 844-R171
		return "no bin/gentle-ai-task that runs the native reviewer and logs it in " + dir + " (re-provision: rm -rf " + dir + " && hw gentle-home)", false
	}
	gouts, gdiag, gok := rtJqOut(dir+"/guard-settings.json", "hooks", "PreToolUse", 0, "hooks", 0, "command")
	io.WriteString(d.err, gdiag) // # MUTATION-ANCHOR: 844-R172
	if !gok {
		return dir + "/guard-settings.json is not readable JSON", false // # MUTATION-ANCHOR: 844-R173
	}
	guard := rtJoinOuts(gouts)
	if i := strings.LastIndexByte(guard, ' '); i >= 0 { // # MUTATION-ANCHOR: 844-R174
		guard = guard[i+1:]
	}
	if guard == "" || !isFile(guard) { // # MUTATION-ANCHOR: 844-R175
		g := guard
		if g == "" {
			g = "<none>"
		}
		return "the guard " + dir + "/guard-settings.json names does not exist: " + g + " (re-provision from the brain checkout: rm -rf " + dir + " && hw gentle-home)", false
	}
	if isFile(dir + "/home/.claude/settings.json") { // # MUTATION-ANCHOR: 844-R176
		ns, hdiag, hok := rtJqEach(dir+"/home/.claude/settings.json", func(v *jval) (string, bool, string) {
			n, _, msg := rtHooksLength(v)
			return n, msg == "", msg
		})
		io.WriteString(d.err, hdiag) // # MUTATION-ANCHOR: 844-R177
		if !hok {
			return dir + "/home/.claude/settings.json is not readable JSON", false // # MUTATION-ANCHOR: 844-R178
		}
		if rtJoinOuts(ns) != "0" { // # MUTATION-ANCHOR: 844-R179
			return dir + "/home/.claude/settings.json carries hooks, and this mode runs none of gentle-ai's", false
		}
	}
	return "", true
}

var rtDeployedKeyRe = regexp.MustCompile(`^[ \t\n\v\f\r]*deployed_check[ \t\n\v\f\r]*:`)
var rtSpacesTail = regexp.MustCompile(`[ \t\n\v\f\r]+$`)
var rtKeyPrefix = regexp.MustCompile(`^[^:]*:[ \t\n\v\f\r]*`)

// briefDeployedCheck is `_brief_deployed_check`: the raw value of the brief's `deployed_check:` key.
func (d *dsp) briefDeployedCheck() string {
	text, ok := rtReadable(d.brief)
	if d.brief == "" || !isFile(d.brief) || !ok {
		return ""
	}
	for n, line := range rtRecords(text) {
		if n == 0 {
			if line != "---" {
				return ""
			}
			continue
		}
		if line == "---" {
			return ""
		}
		if rtDeployedKeyRe.MatchString(strings.ToLower(line)) { // # MUTATION-ANCHOR: 844-701-M03
			line = rtKeyPrefix.ReplaceAllString(line, "")
			line = rtSpacesTail.ReplaceAllString(line, "")
			if len(line) >= 2 && ((line[0] == '"' && line[len(line)-1] == '"') || (line[0] == '\'' && line[len(line)-1] == '\'')) {
				line = line[1 : len(line)-1]
			}
			return line
		}
	}
	return ""
}

var rtOutOfScopeSep = regexp.MustCompile(`^[ \t\n\v\f\r]*(:|—|–|-)?[ \t\n\v\f\r]*`) // # MUTATION-ANCHOR: 844-R185

// rtGentleDeployedCheck is `_gentle_deployed_check`: required, or out-of-scope: <reason>; false (with the raw
// value) when it is neither.
func (d *dsp) rtGentleDeployedCheck() (string, bool) {
	v := d.briefDeployedCheck()
	switch {
	case v == "" || v == "required":
		return "required", true
	case strings.HasPrefix(v, "out-of-scope"): // # MUTATION-ANCHOR: 844-R182
		reason := rtOutOfScopeSep.ReplaceAllString(strings.TrimPrefix(v, "out-of-scope"), "")
		if reason == "" { // # MUTATION-ANCHOR: 844-R183
			return v, false
		}
		return "out-of-scope: " + reason, true // # MUTATION-ANCHOR: 844-R184
	}
	return v, false
}

// validateSddGentle is `_validate_sdd_gentle`: the launch gate for `--sdd gentle`.
func (d *dsp) validateSddGentle() {
	d.gentleVersion = d.gentlePin()                                            // # MUTATION-ANCHOR: 844-R186
	if !strings.Contains(" "+d.c.t.get(d.proj, "sdd_modes")+" ", " gentle ") { // # MUTATION-ANCHOR: 844-R187
		d.die("--sdd must be speckit or none (got: gentle)")
	}
	if d.agent != "claude" { // # MUTATION-ANCHOR: 844-R188
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd gentle runs only on Claude Code, because ODD enters the executor's system prompt through Claude's --append-system-prompt-file and " + d.agent + " has no per-launch equivalent here. Nothing was built. Add --agent claude.")
	}
	if d.here { // # MUTATION-ANCHOR: 844-R189
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: --here runs in your own directory, and ODD writes odd/ where it runs. Nothing was built. Give it its own worktree (drop --here).")
	}
	if d.brief == "" {
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: it needs a brief, and the brief must fix the delivery strategy ODD would otherwise stop to ask for. Nothing was built.")
	}
	d.gentleBriefGate()
	dir := d.gentleHomeDir()
	if why, ok := d.gentleHomeReady(dir); !ok { // # MUTATION-ANCHOR: 844-R191
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: gentle-ai's own home is not ready (" + why + "). Nothing was built. Provision it with: hw gentle-home")
	}
}

// gentleBriefGate is `_gentle_brief_gate`: the part of the gate that reads one brief's declarations.
func (d *dsp) gentleBriefGate() { d.rtGentleGate(d.loadBriefDecls()) }

func (d *dsp) rtGentleGate(decls string) {
	d.gentleDelivery = rtDeclValue(decls, "delivery") // # MUTATION-ANCHOR: 844-R197
	switch d.gentleDelivery {
	case "single-pr", "stacked-to-main", "feature-branch-chain": // # MUTATION-ANCHOR: 844-R192
	case "": // # MUTATION-ANCHOR: 844-R193
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: the brief does not declare `delivery:` in its frontmatter. ODD asks for the delivery strategy before it slices work, and nobody answers in an executor's pane. Declare one: delivery: single-pr | stacked-to-main | feature-branch-chain. Nothing was built.")
	default:
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: delivery: " + d.gentleDelivery + " is not one of single-pr, stacked-to-main, feature-branch-chain. Nothing was built.")
	}
	d.gentleReview = rtDeclValue(decls, "review") // # MUTATION-ANCHOR: 844-R194
	switch d.gentleReview {
	case "", "rdd": // # MUTATION-ANCHOR: 844-R195
	default:
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: review: " + d.gentleReview + " is not rdd. RDD is the only opt-in (omit the key for Judgment Day). Nothing was built.")
	}
	v, ok := d.rtGentleDeployedCheck()
	d.gentleDeployedCheck = v
	if !ok { // # MUTATION-ANCHOR: 844-R198
		d.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: deployed_check: " + v + " is not a value hw accepts. The accepted forms are exactly three: the key absent or empty (the default, same as required); 'deployed_check: required'; or 'deployed_check: out-of-scope — <why>' with a non-empty reason (the separator may be —, – , - or :), e.g. 'deployed_check: out-of-scope — a docs-only change, nothing is deployed'. A bare 'out-of-scope' with no reason is refused too. Nothing was built.") // # MUTATION-ANCHOR: 844-781-M02
	}
}

// gentleRddWord is `_gentle_rdd_word`: what the dispatch says about RDD.
func (d *dsp) gentleRddWord() string { // # MUTATION-ANCHOR: 844-781-M01
	if d.gentleReview == "rdd" {
		return "RDD on (native)" // # MUTATION-ANCHOR: 844-R199
	}
	return "RDD off"
}
