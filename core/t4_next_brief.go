package main

// The brief `hw next` delivers, read the way bin/hw reads a launch's: its frontmatter (_brief_declares, the
// Python reader, and the awk readers _brief_fm_value, _brief_kind_for_close, _brief_boundary), the
// authorization it cites (_check_authorizes, _brief_authorization, _next_authorization), and the review it
// chose (_brief_review_mode, _review_gate, _review_receipt). Dispatch helpers (T5), copied.

import (
	"os"
	"regexp"
	"strings"
)

var nextKnownKeys = map[string]bool{"agent": true, "model": true, "effort": true, "sdd": true, "placement": true, "worktree": true,
	"database": true, "invoker": true, "requires": true, "requires-agents": true, "kind": true, "base": true,
	"decision": true, "delivery": true, "boundary": true, "deployed_check": true, "sandbox": true, "review": true, "plugins": true,
	"requested_by": true, "asked": true, "authorizes": true, "secrets": true}

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

// nextAuthorization is _next_authorization: what a brief's `authorizes:` puts in the prompt (who asked, and the
// "<destination> :: <operation> :: <handle>" records), after the same validation the launch runs; false when it
// refused (the refusal is on stderr already). The block's text is the renderer's.
func (a *app) nextAuthorization(brief string) (requested string, entries []string, ok bool) {
	if brief == "" || !isFile(brief) {
		return "", nil, true
	}
	decls := nextBriefDeclares(brief)
	requested = nextDecl(decls, "requested_by")
	n := 0
	refuse := func(msg string) (string, []string, bool) {
		a.nextDieSub(msg)
		return "", nil, false
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
			return refuse("the brief's authorizes: entry " + ns + " has more than three fields (" + val + "). Use one authorizes: line per entry (\";\" does not separate entries). NOTHING was built.")
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
		return "", nil, true
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
			return refuse("the authorization names Keychain handles that do not resolve:" + missing + ". NOTHING was built. The third field is a Keychain handle: a Bitwarden item belongs in secrets: [item], not here. Store each handle with security add-generic-password -s <handle> -a " + user + " -w '<value>' (check: hw handle <handle>), or pass --no-handle-check.")
		}
	} else if check == "1" {
		a.warn("the authorization's handles were NOT checked: no keychain backend here")
	}
	return requested, entries, true
}

// nextSecrets is _check_secrets' reading of the brief: the Bitwarden items `secrets:` names, "a, b" — what the launch
// renders in its grants block, so a re-task brief carrying them gets the same one. The vault is not asked here, and an
// item the launch would refuse is warned about and left out.
func (a *app) nextSecrets(brief string) string {
	var items []string
	for _, l := range nextBriefDeclares(brief) {
		key, val, _ := strings.Cut(l, "\t")
		if key != "secrets" {
			continue
		}
		got, bad := parseSecretsValue(val)
		for _, b := range bad {
			a.warn("the re-task brief's secrets: names \"" + b + "\", which is not a Bitwarden item name; it is left out of the grants block")
		}
		items = append(items, got...)
	}
	return strings.Join(items, ", ")
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

// ── the review the brief chose ──────────────────────────────────────────────

// nextReviewMode is `$(_brief_review_mode 2>/dev/null)` over the new brief: "" when it is not a mode hw knows.
func nextReviewMode(brief string) string {
	m, ok := reviewModeOf(reviewRawOf(nextFmValue("review", brief)))
	if !ok {
		return ""
	}
	return m
}

// nextJudgmentDayRequired is the launch's `reviewDeclared() && reviewMode() == "judgment-day"` for a re-task: a
// brief that DECLARES judgment-day, rdd or a list naming it is held to the REQUIRED close, never the optional one.
func nextJudgmentDayRequired(brief string) bool {
	return nextFmValue("review", brief) != "" && nextReviewMode(brief) == "judgment-day"
}

// nextPluginsDeclared is _plugins_declared over the NEW brief (hw next reads it fresh, never a memo of another).
func nextPluginsDeclared(brief string) []string {
	return pluginsWithReview(pluginsOfValue(nextDecl(nextBriefDeclares(brief), "plugins")), reviewRawOf(nextFmValue("review", brief)))
}

// nextReviewGate is `( BRIEF=<brief>; _review_gate )`: a bad value, or `plugins` with nothing to name, refuses the
// re-task before anything is sent.
func (a *app) nextReviewGate(brief string) {
	reviewGateOf(reviewRawOf(nextFmValue("review", brief)), nextPluginsDeclared(brief), a.die, a.warn)
}

// nextReviewReceipt is `BRIEF=<brief> _review_receipt <run dir> <key>`: only when the brief declares review:.
func (a *app) nextReviewReceipt(rundir, key, brief string) {
	if nextFmValue("review", brief) == "" {
		return
	}
	mode, ok := reviewModeOf(reviewRawOf(nextFmValue("review", brief)))
	if !ok {
		return
	}
	a.receiptInto(rundir, key, mode, reviewReceiptSource(brief, mode, nextPluginsDeclared(brief)))
}

// nextJdAgentsReceipt is jdAgentsReceipt for a re-task: the same row, the same condition (pi, a DECLARED
// Judgment Day, and both judges on disk), keyed by the task it belongs to.
func (a *app) nextJdAgentsReceipt(rundir, key, brief, vendor, workdir string) {
	// The same gates the re-task's closing has (`f.jd` in t4_next_msg.go): a resolved base ref and a build kind, or
	// there is no Judgment Day to hold the close to. Judgment Day, both judges, 2026-11-04.
	if (vendor != "pi" && vendor != "claude") || !nextJudgmentDayRequired(brief) || a.nextEnvVal(rundir, "HW_REVIEW_BASE_REF") == "" || !nextIsBuild(brief) {
		return
	}
	home := a.getenv("HOME")
	cfg := orDefaultStr(a.nextEnvVal(rundir, "CLAUDE_CONFIG_DIR"), home+"/.claude")
	if j, _, _ := jdFacts(vendor, cfg, home, workdir, "", vendor == "pi" && a.explorerFor("").piSubagentsReady()); j != "agents" {
		return
	}
	a.receiptInto(rundir, key, "jd-judge-a,jd-judge-b",
		"brief review: in "+brief+" at hw next, and the judges are registered — done-invoker requires a call to each judge, not `reviewer`, within the Judgment Day budget and frozen ledger")
}

// nextPluginReviewReceipt is the plugin review skills the NEW brief's closing names, keyed by task like the review above.
func (a *app) nextPluginReviewReceipt(rundir, key, brief string) {
	sk := pluginReviewSkillsNamed(nextReviewMode(brief), a.nextEnvVal(rundir, "HW_REVIEW_BASE_REF"), nextIsBuild(brief), nextPluginsDeclared(brief))
	if len(sk) == 0 { // # MUTATION-ANCHOR: 1150-G08
		return
	}
	a.receiptInto(rundir, key, strings.Join(reviewSkillsQualified(nextPluginsDeclared(brief)), ","),
		"brief plugins: and review: in "+brief+" at hw next — done-invoker proves each skill from the transcript, or takes a review-skip: line")
}

func nextGitOut(dir string, args ...string) (string, error) {
	r := extRun(extCall{Name: "git", Args: append([]string{"-C", dir}, args...)})
	return r.Out, r.errOf()
}

func nextGit(dir string, args ...string) error {
	return extRun(extCall{Name: "git", Args: append([]string{"-C", dir}, args...)}).errOf()
}

func nextExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// nextCheckBriefModel is the `model:` check of a re-task brief, the twin of nextCheckBriefEffort and the brief door of the same
// ONE check the dispatch runs (modelEffortSuffixRefusal, brDispatchActual): a level written inside the id is refused in the
// words of --effort, and a model the pane was not launched with is refused too, because --model is fixed at the pane's launch
// and hw next cannot change it. A check, never a source. Nothing is sent and the counter does not move.
func (a *app) nextCheckBriefModel(brief, pane, running string) {
	declared := nextDecl(nextBriefDeclares(brief), "model")
	if declared == "" {
		return
	}
	if msg := modelEffortSuffixRefusal(declared, "the brief's model:"); msg != "" {
		a.die(msg + ". NOTHING was sent and the counter did NOT move.")
	}
	if running == "" {
		running = "<unmeasured>"
	}
	if declared != running {
		a.die("the re-task brief declares model=" + declared + " but " + pane + " runs model=" + running + ", fixed at ITS launch: hw next cannot change it. NOTHING was sent and the counter did NOT move. Dispatch a fresh executor with --model " + declared + ", or change the brief if the brief is what is wrong.")
	}
}

// nextCheckBriefEffort is the `effort:` check of a re-task brief, the brief door of the same ONE check the dispatch runs
// (effortRefusal, brBriefDeclares): a level the vendor cannot take is refused in --effort's words, and a level the pane was
// not launched with is refused too, because --effort is fixed at the pane's launch and hw next cannot change it. A check,
// never a source. Nothing is sent and the counter does not move.
func (a *app) nextCheckBriefEffort(brief, vendor, pane, running string) {
	declared := nextDecl(nextBriefDeclares(brief), "effort")
	if declared == "" {
		return
	}
	if msg := effortRefusal(vendor, declared); msg != "" {
		a.die(msg + ". NOTHING was sent and the counter did NOT move.")
	}
	if running == "" {
		running = "<unmeasured>"
	}
	if declared != running {
		a.die("the re-task brief declares effort=" + declared + " but " + pane + " runs effort=" + running + ", fixed at ITS launch: hw next cannot change it. NOTHING was sent and the counter did NOT move. Dispatch a fresh executor with --effort " + declared + ", or change the brief if the brief is what is wrong.")
	}
}

// nextCheckBriefPlugins is the `plugins:` check of a re-task brief, the twin of nextCheckBriefEffort: a plugin is loaded when
// a session starts (claude --plugin-dir, pi -e), so hw next cannot add one to a live pane, and a re-task that declares a review
// plugin the pane did not launch with would be told "not installed" by its own executor and close on a review-skip (MEASURED
// 2026-11-10, a lane pane (w7H:p1VT), task 4). The pane's own launch is the receipt row `plugins` written at dispatch
// (absent when the brief asked for none, "NOT APPLIED — …" when the vendor loads none); only claude and pi load plugins, so
// any other vendor is not held to it. A check, never a source. Nothing is sent and the counter does not move.
func (a *app) nextCheckBriefPlugins(brief, vendor, pane, rundir string) {
	if vendor != "claude" && vendor != "pi" { // # MUTATION-ANCHOR: 1150-G16
		return
	}
	var loaded []string
	if v, rc := reviveReceiptLast(rundir+"/receipt.jsonl", "plugins"); rc == 0 && !strings.HasPrefix(v, "NOT APPLIED") {
		loaded = pluginsOfValue(v)
	}
	var missing []string
	for _, n := range nextPluginsDeclared(brief) {
		if !has(loaded, n) { // # MUTATION-ANCHOR: 1150-G17
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return
	}
	have := "none"
	if len(loaded) > 0 {
		have = strings.Join(loaded, ", ")
	}
	a.die("the re-task brief declares plugins " + strings.Join(missing, ", ") + " (plugins: or review:), but " + pane + " was launched with " + have + ", and a plugin is loaded when a session starts (" + map[string]string{"claude": "--plugin-dir", "pi": "-e"}[vendor] + "): hw next cannot add one to a live pane. NOTHING was sent and the counter did NOT move. Dispatch a fresh executor, which loads them: hw <project> <task> --fresh. Or change the brief if the brief is what is wrong — an executor that is not given a declared review plugin must not close on a review-skip.")
}

// nextPluginsReceipt records, for a re-task onto a vendor that loads no plugin (codex, opencode), the same `plugins` row its launch
// writes when the brief declares some: "NOT APPLIED — …". Without it a pane launched with no plugins has no row, done-invoker reads
// that as a pane that WAS meant to load them (the measured case) and refuses the review-skip that is the only exit a vendor with no
// plugin has, while a fresh dispatch of the same brief is let close (Judgment Day, both judges, 2026-11-10). claude and pi write nothing:
// their row is the launch's, and nextCheckBriefPlugins already held the re-task to it.
func (a *app) nextPluginsReceipt(rundir, brief, vendor string) {
	declared := nextPluginsDeclared(brief)
	if vendor == "claude" || vendor == "pi" || len(declared) == 0 { // # MUTATION-ANCHOR: 1150-G21
		return
	}
	// The vendor is herdr's live detection, an input: empty or unrecognised for a pane that may really be claude or pi. A pane whose
	// launch row says plugins WERE applied, or an unnamed vendor, never gets the row that would shadow it as the LAST one every reader
	// takes (done-invoker's exemption, hw revive's --plugin-dir) — Judgment Day round 2, both judges.
	if prior, rc := reviveReceiptLast(rundir+"/receipt.jsonl", "plugins"); vendor == "" || (rc == 0 && prior != "" && !strings.HasPrefix(prior, "NOT APPLIED")) { // # MUTATION-ANCHOR: 1150-G22
		return
	}
	a.receiptInto(rundir, "plugins", "NOT APPLIED — "+orDefaultStr(vendor, "unknown")+" does not load plugins (asked: "+strings.Join(declared, ",")+")",
		"brief plugins: and review: in "+brief+" at hw next; only claude (--plugin-dir) and pi (-e) load plugins")
}
