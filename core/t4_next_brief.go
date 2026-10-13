package main

// The brief `hw next` delivers, read the way bin/hw reads a launch's: its frontmatter (_brief_declares, the
// Python reader, and the awk readers _brief_fm_value, _brief_kind_for_close, _brief_boundary), the
// authorization it cites (_check_authorizes, _brief_authorization, _next_authorization), and gentle's gate
// for a gentle pane (_gentle_brief_gate, _gentle_home_ready, gentle_home_dir). Dispatch helpers (T5), copied.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var nextKnownKeys = map[string]bool{"agent": true, "model": true, "sdd": true, "placement": true, "worktree": true,
	"database": true, "invoker": true, "requires": true, "requires-agents": true, "kind": true, "base": true,
	"decision": true, "delivery": true, "boundary": true, "deployed_check": true, "sandbox": true, "review": true,
	"requested_by": true, "asked": true, "authorizes": true}

// nextBriefDeclares is _brief_declares: "key\tvalue" per declared key, "__malformed__\t…" and
// "__unknown__\t<key>" for what it cannot take, nothing for a brief with no frontmatter.
func nextBriefDeclares(path string) []string {
	if path == "" || !isFile(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := strings.ToValidUTF8(string(data), "�")
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n") // text mode: universal newlines
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || pyStrip(lines[0]) != "---" {
		return nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return []string{"__malformed__\tunterminated frontmatter: no closing --- found"}
	}
	var out []string
	for _, raw := range lines[1:end] {
		line := pyStrip(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			out = append(out, "__malformed__\tnot a key: value pair: "+line)
			continue
		}
		key := strings.ToLower(pyStrip(line[:i]))
		value := strings.Trim(pyStrip(line[i+1:]), "\"'")
		if key == "kind" {
			value = strings.ToLower(value)
		}
		if value == "" {
			continue
		}
		if !nextKnownKeys[key] { // # MUTATION-ANCHOR: 862-G15
			out = append(out, "__unknown__\t"+key)
			continue
		}
		out = append(out, key+"\t"+value)
	}
	return out
}

// nextDecl is `printf '%s\n' "$BRIEF_DECLS" | awk -F '\t' '$1 == "<key>" { print $2; exit }'`.
func nextDecl(decls []string, key string) string {
	for _, l := range decls {
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

// nextAuthTrim is _auth_trim: leading and trailing [:space:] off.
func nextAuthTrim(v string) string { return strings.Trim(v, " \t\n\v\f\r") }

var nextReHandle = regexp.MustCompile(`^[A-Za-z0-9._-]*$`)

// nextAuthorization is _next_authorization: the block a brief's `authorizes:` puts in the prompt, after the
// same validation the launch runs; false when it refused (the refusal is on stderr already).
func (a *app) nextAuthorization(brief string) (string, bool) {
	if brief == "" || !isFile(brief) {
		return "", true
	}
	decls := nextBriefDeclares(brief)
	requested := nextDecl(decls, "requested_by")
	var entries []string
	n := 0
	refuse := func(msg string) (string, bool) {
		a.nextDieSub(msg)
		return "", false
	}
	for _, l := range decls {
		key, val, _ := strings.Cut(l, "\t")
		if key != "authorizes" {
			continue
		}
		n++
		ns := itoa(n)
		first := strings.Index(val, " :: ")
		if first < 0 || !strings.Contains(val[first+4:], " :: ") {
			return refuse("the brief's authorizes: entry " + ns + " is malformed (" + val + "). Shape: authorizes: <destination> :: <operation> :: <handle|none>. NOTHING was built.")
		}
		dest := nextAuthTrim(val[:first])
		rest := val[first+4:]
		i := strings.Index(rest, " :: ")
		op := nextAuthTrim(rest[:i])
		handle := nextAuthTrim(rest[i+4:])
		if strings.Contains(handle, " :: ") {
			return refuse("the brief's authorizes: entry " + ns + " has more than three fields (" + val + "). NOTHING was built.")
		}
		if dest == "" || op == "" || handle == "" {
			return refuse("the brief's authorizes: entry " + ns + " has an empty field (" + val + "). Name the destination, the operation, and a handle or none. NOTHING was built.")
		}
		if handle != "none" && !nextReHandle.MatchString(handle) {
			return refuse("the brief's authorizes: entry " + ns + " names the handle '" + handle + "', which is not a Keychain service name (letters, digits, . _ -) nor none. NOTHING was built.")
		}
		entries = append(entries, dest+" :: "+op+" :: "+handle)
	}
	if n == 0 {
		return "", true
	}
	if requested == "" { // # MUTATION-ANCHOR: 862-G16
		return refuse("the brief carries authorizes: but cites no request (requested_by:). An authorization nobody asked for is not delivered. NOTHING was built. Add `requested_by: <who, when, and the request in their words>` (or pass --requested-by).")
	}
	check := a.nextEnvOr("CHECK_HANDLES", "1")
	if check == "1" && a.keychainAvailable() {
		var hs []string
		for _, e := range entries {
			f := strings.Split(e, " :: ")
			if len(f) > 2 {
				hs = append(hs, f[2])
			} else {
				hs = append(hs, "")
			}
		}
		missing := ""
		for _, h := range nextSortedUnique(hs) {
			if h == "" || h == "none" {
				continue
			}
			if !handleExists(h) {
				missing += " " + h
			}
		}
		if missing != "" {
			user := a.getenv("USER")
			if user == "" {
				user = whoami()
			}
			return refuse("the authorization names Keychain handles that do not resolve:" + missing + ". NOTHING was built. Store each with security add-generic-password -s <handle> -a " + user + " -w '<value>' (check: hw handle <handle>), or pass --no-handle-check.")
		}
	} else if check == "1" {
		a.warn("the authorization's handles were NOT checked: no keychain backend here")
	}
	var b strings.Builder
	b.WriteString("AUTHORIZATION — delivered by hw from the brief frontmatter (authorizes:), quoted:\n\n")
	b.WriteString("> requested_by: " + requested + "\n")
	for _, e := range entries {
		f := strings.Split(e, " :: ")
		for len(f) < 3 {
			f = append(f, "")
		}
		b.WriteString("> authorized: destination: " + f[0] + " | operation: " + f[1] + " | credential handle: " + f[2] + "\n")
	}
	b.WriteString("\nOnly these destinations and operations are authorized, and nothing beyond them:\nanother destination, another operation, or a credential or session not named\nhere stays unauthorized. Within them, act without asking; for anything else\nask-invoker, or report blocked. A handle is a resolver, never a value.")
	return bashSub(b.String()), true
}

// nextDieSub is a `die` inside `$( … )`: its line on stderr, and only the subshell ends.
func (a *app) nextDieSub(msg string) {
	a.err.Write([]byte("  " + a.cErr + "✗" + a.c0 + " " + msg + "\n"))
}

// ── the awk readers of one frontmatter key ──────────────────────────────────

// nextFmLines are the lines awk's `NR==1 { if ($0 != "---") exit; next } $0 == "---" { exit }` reads.
func nextFmLines(path string) []string {
	if path == "" || !isFile(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return nil
	}
	var out []string
	for _, l := range lines[1:] {
		if l == "---" {
			break
		}
		out = append(out, l)
	}
	return out
}

var nextReAwkLead = regexp.MustCompile(`^[^:]*:[ \t\n\v\f\r]*`)

// nextFmValue is _brief_fm_value / _brief_boundary / _brief_deployed_check: the first line whose key
// matches, value trimmed and unquoted once.
func nextFmValue(key, path string) string {
	re := regexp.MustCompile(`^[ \t\n\v\f\r]*` + regexp.QuoteMeta(key) + `[ \t\n\v\f\r]*:`)
	for _, l := range nextFmLines(path) {
		if !re.MatchString(strings.ToLower(l)) {
			continue
		}
		v := nextReAwkLead.ReplaceAllString(l, "")
		v = strings.TrimRight(v, " \t\n\v\f\r")
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		return v
	}
	return ""
}

var nextReKindKey = regexp.MustCompile(`^[ \t\n\v\f\r]*kind[ \t\n\v\f\r]*:`)
var nextReComment = regexp.MustCompile(`[ \t\n\v\f\r]*#.*$`)

// nextKindForClose is _brief_kind_for_close for `hw next` (TASK_KIND is never set on this route).
func nextKindForClose(path string) string {
	for _, l := range nextFmLines(path) {
		if !nextReKindKey.MatchString(strings.ToLower(l)) {
			continue
		}
		v := nextReAwkLead.ReplaceAllString(l, "")
		v = nextReComment.ReplaceAllString(v, "")
		v = strings.Map(func(r rune) rune {
			switch r {
			case '"', '\'', ' ', '\t', '\n', '\v', '\f', '\r':
				return -1
			}
			return r
		}, v)
		return strings.ToLower(v)
	}
	return ""
}

func nextIsBuild(path string) bool {
	k := nextKindForClose(path)
	return k == "" || k == "build"
}

// nextDesignGate is _design_gate_applies; nextChecksGate is _checks_gate_applies.
func nextDesignGate(path string) bool {
	return nextFmValue("boundary", path) != "" && nextIsBuild(path)
}
func nextChecksGate(path string) bool { return nextIsBuild(path) }

// ── gentle ──────────────────────────────────────────────────────────────────

// nextGentleRead is _next_gentle_read: the NEW brief through gentle's per-task gate, and the home checked.
func (a *app) nextGentleRead(s *nextState) {
	if s.brief == "" {
		a.die("hw next on a gentle pane REFUSED: ODD needs the next task as a brief (--brief <path>) that fixes its delivery strategy, because nobody answers in an executor's pane. Nothing was sent.")
	}
	decls := nextBriefDeclares(s.brief)
	s.gDelivery = nextDecl(decls, "delivery")
	switch s.gDelivery {
	case "single-pr", "stacked-to-main", "feature-branch-chain":
	case "":
		a.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: the brief does not declare `delivery:` in its frontmatter. ODD asks for the delivery strategy before it slices work, and nobody answers in an executor's pane. Declare one: delivery: single-pr | stacked-to-main | feature-branch-chain. Nothing was built.")
	default:
		a.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: delivery: " + s.gDelivery + " is not one of single-pr, stacked-to-main, feature-branch-chain. Nothing was built.")
	}
	s.gReview = nextDecl(decls, "review")
	switch s.gReview {
	case "", "rdd":
	default:
		a.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: review: " + s.gReview + " is not rdd. RDD is the only opt-in (omit the key for Judgment Day). Nothing was built.")
	}
	dc, ok := nextGentleDeployedCheck(s.brief)
	s.gDeployed = dc
	if !ok {
		a.die("gentle-ai ODD dispatch REFUSED BEFORE BUILDING ANYTHING: deployed_check: " + dc + " is not a value hw accepts. The accepted forms are exactly three: the key absent or empty (the default, same as required); 'deployed_check: required'; or 'deployed_check: out-of-scope — <why>' with a non-empty reason (the separator may be —, – , - or :), e.g. 'deployed_check: out-of-scope — a docs-only change, nothing is deployed'. A bare 'out-of-scope' with no reason is refused too. Nothing was built.")
	}
	d := a.nextGentleHome()
	if why, ready := a.nextGentleHomeReady(d); !ready {
		a.die("hw next on a gentle pane REFUSED: gentle-ai's own home is not ready (" + why + "). Nothing was sent. Provision it with: hw gentle-home")
	}
}

var nextReOOS = regexp.MustCompile(`^[ \t\n\v\f\r]*(:|—|–|-)?[ \t\n\v\f\r]*`)

// nextGentleDeployedCheck is _gentle_deployed_check.
func nextGentleDeployedCheck(brief string) (string, bool) {
	v := nextFmValue("deployed_check", brief)
	switch {
	case v == "" || v == "required":
		return "required", true
	case strings.HasPrefix(v, "out-of-scope"):
		reason := nextReOOS.ReplaceAllString(strings.TrimPrefix(v, "out-of-scope"), "")
		if reason == "" {
			return v, false
		}
		return "out-of-scope: " + reason, true
	}
	return v, false
}

// nextGentleHome is gentle_home_dir.
func (a *app) nextGentleHome() string {
	return a.nextEnvOr("HW_GENTLE_HOME", a.c.home+"/.local/share/hw/gentle-home")
}

// nextGentleHomeRefusal is _gentle_home_refusal: why the destination may not be used, or "".
func (a *app) nextGentleHomeRefusal(d string) (string, bool) {
	if !strings.HasPrefix(d, "/") {
		return "not an absolute path: " + d, true
	}
	if strings.ContainsAny(d, " \t\n\v\f\r") {
		return "the path has whitespace, and hw passes it as one word: " + d, true
	}
	d = strings.TrimSuffix(d, "/")
	home := strings.TrimSuffix(a.c.home, "/")
	if d == home {
		return "it is HOME itself", true
	}
	for _, cfg := range []string{".claude", ".claude-personal", ".claude.json", ".config/opencode", ".codex", ".gentle-ai"} {
		if strings.HasPrefix(d+"/", home+"/"+cfg+"/") {
			return "it is inside ~/" + cfg + ", which another session reads", true
		}
	}
	p := d
	for !isDir(p) && p != "/" {
		p = nextDirname(p)
	}
	if nextGit(p, "rev-parse", "--is-inside-work-tree") == nil && nextGit(p, "check-ignore", "-q", d) != nil {
		top, _ := nextGitOut(p, "rev-parse", "--show-toplevel")
		return "it is inside a git work tree (" + bashSub(top) + ") and not ignored there", true
	}
	return "", false
}

func nextGit(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	return cmd.Run()
}

func nextGitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

// nextGentleHomeReady is _gentle_home_ready: "" and true for a complete home of the pinned version.
func (a *app) nextGentleHomeReady(d string) (string, bool) {
	if why, refused := a.nextGentleHomeRefusal(d); refused {
		return why, false
	}
	m := d + "/manifest.json"
	if !isFile(m) {
		return "no " + m, false
	}
	v, ok := nextJqFile(m, "version")
	if !ok {
		return m + " is not readable JSON", false
	}
	if v != "4.0.0" {
		return m + " is not version 4.0.0", false
	}
	f := d + "/odd-block.md"
	st, err := os.Stat(f)
	if err != nil || st.Size() == 0 {
		return "no ODD block at " + f, false
	}
	body, _ := os.ReadFile(f)
	if !bytes.Contains(body, []byte("<!-- gentle-ai:agent-routing -->")) {
		return f + " carries no gentle-ai agent-routing block", false
	}
	if bytes.Contains(body, []byte("<!-- gentle-ai:orchestrator -->")) {
		return f + " carries the orchestrator block, and only the ODD protocol travels", false
	}
	sha, _ := nextJqFile(m, "odd_block_sha256")
	if nextSha256(body) != sha {
		return f + " changed after provisioning", false
	}
	if !isFile(d + "/plugin/gentle-odd/.claude-plugin/plugin.json") {
		return "no plugin at " + d + "/plugin/gentle-odd", false
	}
	if !isFile(d + "/guard-settings.json") {
		return "no " + d + "/guard-settings.json", false
	}
	task := d + "/bin/gentle-ai-task"
	tb, terr := os.ReadFile(task)
	if !nextExecutable(task) || terr != nil || !bytes.Contains(tb, []byte("rdd-log.jsonl")) {
		return "no bin/gentle-ai-task that runs the native reviewer and logs it in " + d + " (re-provision: rm -rf " + d + " && hw gentle-home)", false
	}
	guard, ok := nextJqGuard(d + "/guard-settings.json")
	if !ok {
		return d + "/guard-settings.json is not readable JSON", false
	}
	if i := strings.LastIndexByte(guard, ' '); i >= 0 {
		guard = guard[i+1:]
	}
	if guard == "" || !isFile(guard) {
		g := guard
		if g == "" {
			g = "<none>"
		}
		return "the guard " + d + "/guard-settings.json names does not exist: " + g + " (re-provision from the brain checkout: rm -rf " + d + " && hw gentle-home)", false
	}
	if hs := d + "/home/.claude/settings.json"; isFile(hs) {
		n, ok := nextJqHooksLen(hs)
		if !ok {
			return d + "/home/.claude/settings.json is not readable JSON", false
		}
		if n != "0" {
			return d + "/home/.claude/settings.json carries hooks, and this mode runs none of gentle-ai's", false
		}
	}
	return "", true
}

func nextExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// nextSha256 is `shasum -a 256 <file> | cut -d' ' -f1`.
func nextSha256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// nextJqFile is `jq -r '.<key> // empty' <file>`: false when jq would fail on the file.
func nextJqFile(path, key string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	vals, perrE := parseStream(data)
	perr := perrE != nil
	var outs []string
	for _, v := range vals {
		if v.kind != vObj && v.kind != vNull {
			return "", false
		}
		if c, ok := v.get(key); ok && c.truthy() {
			outs = append(outs, c.raw())
		}
	}
	if perr {
		return "", false
	}
	return bashSub(strings.Join(outs, "\n")), true
}

// nextJqGuard is `jq -r '.hooks.PreToolUse[0].hooks[0].command // empty' guard-settings.json`.
func nextJqGuard(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	vals, perrE := parseStream(data)
	perr := perrE != nil
	if perr {
		return "", false
	}
	var outs []string
	for _, v := range vals {
		c, ok := nextPath(v, "hooks", "PreToolUse")
		if !ok || c.kind != vArr || len(c.arr) == 0 {
			continue
		}
		c, ok = c.arr[0].get("hooks")
		if !ok || c.kind != vArr || len(c.arr) == 0 {
			continue
		}
		if c, ok = c.arr[0].get("command"); ok && c.truthy() {
			outs = append(outs, c.raw())
		}
	}
	return bashSub(strings.Join(outs, "\n")), true
}

// nextJqHooksLen is `jq -r '.hooks // {} | length'`.
func nextJqHooksLen(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	vals, perrE := parseStream(data)
	perr := perrE != nil
	if perr || len(vals) == 0 {
		return "", false
	}
	v := vals[len(vals)-1]
	h, ok := v.get("hooks")
	if !ok || !h.truthy() {
		return "0", true
	}
	switch h.kind {
	case vObj:
		return itoa(len(h.obj)), true
	case vArr:
		return itoa(len(h.arr)), true
	case vStr:
		return itoa(len([]rune(h.s))), true
	}
	return "", false
}
