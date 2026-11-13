package invokers

// Ports of two done-invoker gate blocks (bin/done-invoker, "A TASK WHOSE
// BRIEF DECLARED A BOUNDARY", "A BUILD CLOSES ON checks.md"). Pure functions: the receipt lookup
// stays with the caller, the refusal text is byte-identical to the bash and
// the caller adds the "done-invoker: " prefix.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// isRegular is bash's `[ -f path ]`: exists, follows symlinks, regular file.
func isRegular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// posixSpace is the [[:space:]] set.
func posixSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

func trimLeftFn(s string, f func(byte) bool) string {
	i := 0
	for i < len(s) && f(s[i]) {
		i++
	}
	return s[i:]
}

func trimRightFn(s string, f func(byte) bool) string {
	j := len(s)
	for j > 0 && f(s[j-1]) {
		j--
	}
	return s[:j]
}

// fileField is `sed -n "s/^KEY:[[:space:]]*//p" FILE | tail -1 | sed
// 's/[[:space:]]*$//'`: the last line that starts with "KEY:", prefix and
// surrounding whitespace removed. An unreadable file yields "".
func fileField(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + ":"
	last := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, prefix) {
			last = trimLeftFn(line[len(prefix):], posixSpace)
		}
	}
	return trimRightFn(last, posixSpace)
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// awkLower is awk's tolower under a UTF-8 locale (macOS awk 20200816 lowers
// whole characters, not just ASCII); bytes that are not valid UTF-8 are kept.
func awkLower(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && w == 1 {
			b.WriteByte(s[i])
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
		i += w
	}
	return b.String()
}

// resolveUnderRoot is `case "$x" in ""|/*) ;; *) x="$ROOT/$x" ;; esac`.
func resolveUnderRoot(root, p string) string {
	if p == "" || strings.HasPrefix(p, "/") {
		return p
	}
	return root + "/" + p
}

// DesignGate is the inside of `if [ -n "$_dz_boundary" ]`: the design judgment
// must name the declared boundary, end APPROVED and judge the design on disk.
func DesignGate(artifactsRoot, boundary string) error {
	file := artifactsRoot + "/design-judgment.md"
	how := fmt.Sprintf("Judge the design first (judgment-day, design mode: references/design-mode.md), then write %s with the lines  boundary: %s  /  design: <file>  /  design_sha256: <sha256>  /  JUDGMENT: APPROVED. An ESCALATED design is reported with --blocked. NOTHING was sent or marked.", file, boundary)
	if !isRegular(file) {
		return die("%s", fmt.Sprintf("this task's brief declared a boundary, and there is no %s. %s", file, how))
	}
	if got := fileField(file, "boundary"); got != boundary {
		return die("%s", fmt.Sprintf("design-judgment.md judges boundary '%s', and this task's brief declared '%s'. A verdict about another boundary is not this task's. %s", got, boundary, how))
	}
	verdict := fileField(file, "JUDGMENT")
	switch {
	case strings.HasPrefix(verdict, "APPROVED"):
	case strings.HasPrefix(verdict, "ESCALATED"):
		return die("%s", "the design judgment is ESCALATED. That is a blocker, not a done: report it with done-invoker --blocked, naming the bypass classes the judges found. NOTHING was sent or marked.")
	default:
		last := verdict
		if last == "" {
			last = "none"
		}
		return die("%s", fmt.Sprintf("design-judgment.md has no 'JUDGMENT: APPROVED' line (last verdict: '%s'). %s", last, how))
	}
	rawDesign := fileField(file, "design")
	design := resolveUnderRoot(artifactsRoot, rawDesign)
	if design == "" || !isRegular(design) {
		return die("%s", fmt.Sprintf("design-judgment.md names no design file that exists ('%s'). %s", rawDesign, how))
	}
	want := fileField(file, "design_sha256")
	have := ""
	if b, err := os.ReadFile(design); err == nil {
		sum := sha256.Sum256(b)
		have = hex.EncodeToString(sum[:])
	}
	if want == "" || want != have {
		haveShown, wantShown := have, want
		if haveShown == "" {
			haveShown = "<unreadable>"
		}
		if wantShown == "" {
			wantShown = "<no design_sha256>"
		}
		return die("%s", fmt.Sprintf("the design file %s hashes to %s, and the verdict judged %s. The approved design is not the one on disk: judge the current one. NOTHING was sent or marked.", design, haveShown, wantShown))
	}
	return nil
}

// PluginSkipGate refuses a `review-skip: <skill>` for a plugin the brain vendors in setup/vendor/plugins (vendored: nil when
// there is none to protect). A declared plugin that did not run is never accepted as skipped: the closing's own words used to
// say "a skill that is not among your available skills is not installed", so an executor whose pane was launched without the
// plugin wrote exactly that and closed (MEASURED 2026-11-10, a lane pane (w7H:p1VT), task 4: thermo, pstack,
// blast-radius and comment-sicko, "not installed", accepted). The honest exit for a plugin that cannot run is --blocked.
func PluginSkipGate(skills, summary string, vendored func(skill string) bool) error {
	var refused []string
	for _, sk := range strings.Split(skills, ",") {
		if sk != "" && vendored != nil && reviewSkipped(summary, sk) && vendored(sk) { // # MUTATION-ANCHOR: 1150-G18
			refused = append(refused, sk)
		}
	}
	if len(refused) == 0 {
		return nil
	}
	return die("%s", fmt.Sprintf("this task's closing named the plugin review skills %s, and the report skips %s with review-skip:, but those plugins are vendored in setup/vendor/plugins: a declared plugin that did not run is not a skip, and \"not installed\" is not an outcome (a pane launched without a plugin is a dispatch error, not yours to skip). Run each over your committed range under its full name (plugin:skill); if you cannot — the skill is not among your available skills, the pane was not launched with the plugin — close with done-invoker --blocked naming it, so the brainer dispatches a fresh executor that loads it. NOTHING was sent or marked.", skills, strings.Join(refused, ", ")))
}

// PluginReviewGate holds a done report to the plugin review skills the closing named: comma-joined, each one
// `plugin:piece` (a skill or an agent; a bare name, from an older receipt, is the skill part of any plugin). A piece
// is released by the executor's own transcript — a recorded invocation of exactly that name, whatever the vendor
// records one as (each reader says how) — or by `review-skip: <skill> — <why>` in the report. With no transcript
// (proof nil) the report text must name the skill as a whole token, and the note returned says the check was
// unverified.
func PluginReviewGate(skills, summary string, proof *TranscriptProof, why string) (note string, err error) {
	var missing, shown []string
	for _, sk := range strings.Split(skills, ",") {
		if sk == "" {
			continue
		}
		shown = append(shown, sk)
		switch {
		case reviewSkipped(summary, sk):
		case proof != nil && proof.Has(sk): // # MUTATION-ANCHOR: 1150-G11
		case proof == nil && mentionsToken(summary, sk): // # MUTATION-ANCHOR: 1150-G05
		default:
			missing = append(missing, sk)
		}
	}
	if len(missing) == 0 {
		if proof == nil {
			return "\nplugin review: unverified: transcript not reachable (" + why + "); the report text names each skill", nil
		}
		return "", nil
	}
	proofIs := "your transcript (and your subagents') has no recorded invocation of that exact name"
	if proof == nil {
		proofIs = "unverified: transcript not reachable (" + why + "), and the report does not name it as a whole token"
	}
	how := "the way your vendor records an invocation"
	if proof != nil && proof.Vendor == "pi" {
		how = "the way pi records one: read the skill's SKILL.md (plugins/<plugin>/skills/<skill>/SKILL.md), or run the agent with the `subagent` tool, agent that full name AND a task AND async:false — a pstack agent, with pstack's `agent` tool, subagent_type that name and no run_in_background (a management call such as action:get, an async launch, and workflow:true or a script run nothing foreman can see)"
	}
	return "", die("%s", fmt.Sprintf("this task's closing named the plugin review skills %s, and there is no proof that %s ran: %s. Invoke each over your committed range under its full name (plugin:skill), "+how+". A plugin vendored in setup/vendor/plugins is never skipped: if you cannot run one, close with done-invoker --blocked naming it; a line  review-skip: <skill> — <why>  is accepted only for a piece no vendored plugin ships. A look-alike (pstack:…-subagent) is not the skill, and Judgment Day does not stand in for them. NOTHING was sent or marked.", strings.Join(shown, ", "), strings.Join(missing, ", "), proofIs))
}

// PluginEvidenceGate holds a done report to what each plugin review LEFT: a skill loaded and never applied still shows in
// the transcript (MEASURED 2026-11-09, pi-quota-stop-reports-finish: "loaded; I reviewed own diff inline" passed). Each
// named skill needs a row in <artifacts>/checks.md that names it (full name or its bare skill part, as a whole token),
// with a verdict cell (pass, fail or unresolved) and a non-empty evidence cell: its findings, or "no findings". It does
// not depend on how the skill was invoked. A `review-skip: <skill>` line releases that skill, as in PluginReviewGate.
func PluginEvidenceGate(skills, summary, artifactsRoot string) error {
	file := artifactsRoot + "/checks.md"
	b, _ := os.ReadFile(file)
	var rows [][]string
	if utf8.Valid(b) {
		for _, line := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "|") && !isSeparatorRow(l) {
				rows = append(rows, strings.Split(strings.TrimPrefix(l, "|"), "|"))
			}
		}
	}
	var missing []string
	for _, sk := range strings.Split(skills, ",") {
		if sk == "" || reviewSkipped(summary, sk) {
			continue
		}
		bare := sk[strings.LastIndex(sk, ":")+1:]
		found := false
		for _, c := range rows {
			if len(c) < 3 || strings.TrimSpace(c[2]) == "" {
				continue
			}
			switch strings.Trim(strings.ToLower(strings.TrimSpace(c[1])), "` ") {
			case "pass", "fail", "unresolved":
			default:
				continue
			}
			if mentionsToken(c[0], sk) || mentionsToken(c[0], bare) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, sk)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return die("%s", fmt.Sprintf("this task's closing named the plugin review skills %s, and %s has no row recording that %s ran on the diff. Loading a skill is not applying it: add one row per skill,  <skill> | pass|fail|unresolved | <its findings, or \"no findings\"> | <correction>  — or, for a piece no vendored plugin ships, a line  review-skip: <skill> — <why>.  NOTHING was sent or marked.", skills, file, strings.Join(missing, ", ")))
}

// JudgmentDayGate holds a done report to the Judgment Day its closing declared: one call to EACH judge the receipt
// names, proved by the executor's own transcript. It exists because the two names in the prompt were not a proof: the
// measured pi session in testdata/transcripts/pi_judges.jsonl runs the builtin `reviewer` in a judge's slot, so a
// report (or a proof) that says jd-judge-a says a specialist of that name ran, not that two blind judges did. A vendor
// where the judges are not registered never writes the receipt row, so its closing is not held to this.
func JudgmentDayGate(judges, summary string, proof *TranscriptProof, why string) (note string, err error) {
	var missing, shown []string
	for _, n := range strings.Split(judges, ",") {
		if n == "" {
			continue
		}
		shown = append(shown, n)
		switch {
		case proof != nil && proof.Has(n): // a call that ran and was answered
		case proof == nil && mentionsToken(summary, n): // no transcript: the report's words, and the note says unverified
		default:
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		if proof == nil {
			return "\njudgment day: unverified: transcript not reachable (" + why + "); the report text names each judge", nil
		}
		return "", nil
	}
	proofIs := "your transcript (and your subagents') has no call to " + strings.Join(missing, " or ")
	if proof == nil {
		proofIs = "unverified: transcript not reachable (" + why + "), and the report does not name " + strings.Join(missing, " or ") + " as a whole token"
	}
	how := "the way your vendor records one"
	if proof != nil && proof.Vendor == "pi" {
		how = "the way pi records one: the `subagent` tool with agent that name, a task, and async:false — or, for a pstack agent, pstack's `agent` tool with subagent_type that name and no run_in_background (a management call such as action:get, an async launch, and workflow:true or a script run nothing foreman can see)"
	}
	return "", die("%s", fmt.Sprintf("this task's closing declared Judgment Day with its judges %s, and %s. Launch each judge over `git diff \"$(hw review-range)\"..HEAD`, %s. The builtin `reviewer` is not a judge where those agents are registered — a report is not a run. If a judge cannot be launched, that is a blocker: report it with done-invoker --blocked. NOTHING was sent or marked.", strings.Join(shown, " and "), proofIs, how))
}

// SerialJudgesWarning is the WARNING (never a refusal) a done-invoker prints when a declared Judgment Day was proved
// by the right judges in the wrong SHAPE: both names are in the transcript, but no single call launched them together.
// MEASURED 2026-11-04 (retro of no-notifications): the two judges ran one after the other, twice, ~13 min lost each
// time. The proof already read for the gate carries the groups; this only asks whether one group holds every judge.
// Empty when the judges ran together, when fewer than two are declared, or when the proof does not hold them all (the
// gate itself refuses that case).
func SerialJudgesWarning(judges string, proof *TranscriptProof) string {
	if proof == nil {
		return ""
	}
	var names []string
	for _, n := range strings.Split(judges, ",") {
		if n == "" {
			continue
		}
		if !proof.Has(n) {
			return ""
		}
		names = append(names, n)
	}
	if len(names) < 2 || proof.Together(names...) {
		return ""
	}
	return fmt.Sprintf("the declared Judgment Day judges (%s) were proved one call after another, not in ONE parallel call. Two blind judges must be launched in the same call — one `subagent({workflow:\"parallel\",args:{tasks:[…]}})` in pi, one assistant message with both Agent/Task tool_uses in Claude — or the second waits for the first and the minutes are paid twice (measured 2026-11-04, no-notifications: ~13 min). This is a WARNING, not a refusal: the report is delivered. Distil it into decisions.md and run the next Judgment Day in one call.", strings.Join(names, ", "))
}

// reviewSkipped is a `review-skip: <skill>` line naming the skill (full name or its bare skill part).
func reviewSkipped(summary, skill string) bool {
	bare := skill[strings.LastIndex(skill, ":")+1:]
	for _, seg := range strings.Split(summary, "review-skip:")[1:] {
		seg = strings.TrimLeft(seg, " \t`*")
		for _, n := range []string{skill, bare} {
			if strings.HasPrefix(seg, n) && !isNameChar(seg, len(n)) { // # MUTATION-ANCHOR: 1150-G14
				return true
			}
		}
	}
	return false
}

// mentionsToken is name as a whole token of s: a longer name around it (`…-subagent`, a `plugin:` prefix) is not it.
func mentionsToken(s, name string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], name)
		if j < 0 {
			return false
		}
		j += i
		if (j == 0 || !isNameChar(s, j-1)) && !isNameChar(s, j+len(name)) {
			return true
		}
		i = j + 1
	}
}

// isNameChar is true when s[i] exists and continues a skill name.
func isNameChar(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c == '-' || c == '_' || c == ':' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// awkSpace is awk's [ \t].
func awkSpace(c byte) bool { return c == ' ' || c == '\t' }

// awkSpaceMark is awk's [ \t`*].
func awkSpaceMark(c byte) bool { return c == ' ' || c == '\t' || c == '`' || c == '*' }

// isSeparatorRow is /^[|: \t-]+$/.
func isSeparatorRow(l string) bool {
	if l == "" {
		return false
	}
	for i := 0; i < len(l); i++ {
		switch l[i] {
		case '|', ':', ' ', '\t', '-':
		default:
			return false
		}
	}
	return true
}

// ChecksGate is the inside of `if [ -n "$_ck" ]`: checks.md needs at least one
// row and a pass/fail/unresolved verdict on every row, and a plain done carries no `fail` row: a task whose own
// checks.md says a requirement failed is not done, it closes --blocked (setup:bash-justify-or-port closed done over
// 2 fail rows, 2026-11-10). It returns CHECKS_NOTE.
func ChecksGate(artifactsRoot string) (note string, err error) {
	return checksGate(artifactsRoot, false)
}

// checksGate is ChecksGate; fnTable is a reviewer's close (review_of), whose checks.md also carries the one-thing-per-function
// table `function | what it does | verdict | action` (design §3.5): its rows are that table's, with their own four verdicts, and
// are read by ReviewerGate, not by this one. The table runs from its header to the first line without a pipe.
func checksGate(artifactsRoot string, fnTable bool) (note string, err error) {
	file := artifactsRoot + "/checks.md"
	how := fmt.Sprintf("Write %s as a table, one row per requirement of the brief: its Verification section and each numbered goal:  requirement | verdict | evidence | correction  (verdict: pass, fail or unresolved). NOTHING was sent or marked.", file)
	if !isRegular(file) {
		return "", die("%s", fmt.Sprintf("this is a build and there is no %s. %s", file, how))
	}
	var rows, pass, fail, unres, bad, firstFail int
	failWhy := ""
	why := ""
	hdr := false
	inFn := false
	// awk (macOS, UTF-8 locale) aborts on the first record with an invalid
	// byte sequence, END never runs and the bash sees an empty summary: no rows.
	if b, rerr := os.ReadFile(file); rerr == nil && utf8.Valid(b) {
		for _, line := range strings.Split(string(b), "\n") {
			l := trimRightFn(trimLeftFn(line, awkSpace), awkSpace)
			if !strings.Contains(l, "|") {
				inFn = false
				continue
			}
			if isSeparatorRow(l) {
				continue
			}
			l = strings.TrimPrefix(l, "|")
			c := splitUnescapedPipes(l)
			if fnTable && (inFn || isFunctionHeader(c)) {
				inFn = true
				continue
			}
			v := ""
			if len(c) > 1 {
				v = trimRightFn(trimLeftFn(awkLower(c[1]), awkSpaceMark), awkSpaceMark)
			}
			c1 := trimRightFn(trimLeftFn(c[0], awkSpace), awkSpace)
			if !hdr && v == "verdict" {
				hdr = true
				continue
			}
			rows++
			switch v {
			case "pass":
				pass++
			case "fail":
				fail++
				if firstFail == 0 {
					firstFail = rows
					failWhy = c1
				}
			case "unresolved":
				unres++
			default:
				if bad == 0 {
					bad = rows
					if v == "" {
						why = "an empty verdict"
					} else {
						why = "the verdict \"" + v + "\""
					}
					why += " (" + c1 + ")"
				}
			}
		}
	}
	if rows <= 0 {
		return "", die("%s", fmt.Sprintf("%s has no rows: a table of requirement | verdict | evidence | correction with nothing under its header proves nothing. %s", file, how))
	}
	vocab := "Accepted verdicts, exactly and alone in the cell: pass, fail, unresolved (case and surrounding spaces or backticks do not matter; a qualifier such as \"pass (n/a)\" does). Put the qualifier in the evidence or correction column."
	if bad != 0 {
		return "", die("%s", fmt.Sprintf("row %d of %s has %s. %s %s", bad, file, why, vocab, how))
	}
	if fail != 0 {
		return "", die("%s", fmt.Sprintf("row %d of %s says fail (%s); fail rows in all: %d. A task that failed its own requirements is not done. Close it as blocked, so the executor stays alive and a ruling resumes this task: done-invoker --blocked \"partial: <the failed rows>; <what would unblock them>\". Or fix the work and set the row to pass. NOTHING was sent or marked.", firstFail, file, failWhy, fail))
	}
	parts := ""
	if pass != 0 {
		parts += fmt.Sprintf(", %d pass", pass)
	}
	if unres != 0 {
		parts += fmt.Sprintf(", %d unresolved", unres)
	}
	return "\nchecks.md: " + strings.TrimPrefix(parts, ", ") + " (" + file + ")", nil
}

var _ = filepath.Join

// splitUnescapedPipes splits a checks.md row on its cell separators only: a `\|` (inside a code span too) is cell text, not a
// column. 2026-11-09: strings.Split on every pipe shifted the verdict column of a row quoting `a \| b`.
func splitUnescapedPipes(l string) []string {
	var cells []string
	start := 0
	for i := 0; i < len(l); i++ {
		if l[i] == '\\' {
			i++
		} else if l[i] == '|' {
			cells = append(cells, l[start:i])
			start = i + 1
		}
	}
	return append(cells, l[start:])
}

// pluginVendored says whether a plugin review piece (`plugin:piece`, or a bare piece from an older receipt) belongs to a plugin
// the brain vendors in setup/vendor/plugins: the directory holds a .claude-plugin/plugin.json, and a bare name is a skill or an
// agent one of them ships. nil (nothing protected) when the run's own `plugins` receipt says its vendor loads none (codex,
// opencode: "NOT APPLIED — …"): there a skip is the only honest exit, and a fresh dispatch would load nothing either.
func (c *Ctx) pluginVendored(receipt string) func(string) bool {
	if strings.HasPrefix(receiptValue(receipt, "plugins"), "NOT APPLIED") { // # MUTATION-ANCHOR: 1150-G20
		return nil
	}
	root := c.brainRoot() + "/setup/vendor/plugins"
	isPlugin := func(n string) bool {
		_, err := os.Stat(root + "/" + n + "/.claude-plugin/plugin.json")
		return err == nil
	}
	return func(skill string) bool {
		if plugin, _, qualified := strings.Cut(skill, ":"); qualified {
			return isPlugin(plugin)
		}
		ents, _ := os.ReadDir(root)
		for _, e := range ents {
			if !isPlugin(e.Name()) {
				continue
			}
			for _, f := range []string{"/skills/" + skill + "/SKILL.md", "/agents/" + skill + ".md"} {
				if _, err := os.Stat(root + "/" + e.Name() + f); err == nil {
					return true
				}
			}
		}
		return false
	}
}

// ── the independent reviewer's close (design §3.4, §3.5, §8 C3 S3) ──────────────────────────────────────────────────────────
//
// Active when the run's receipt holds `review_of_t<n>` (`<lane>:<task>@<sha>`, written at dispatch for a `kind: review` brief
// with `review_of:`). The review's pieces are the plugin gates' (the same receipt carries `plugin_review_t<n>`); this one holds
// what only a reviewer leaves: review.md bound to the SHA it judged, rows that agree with its verdict, and the function table.

var (
	reviewHeadRe = regexp.MustCompile(`^verdict:[ \t]*(approve|changes-needed)[ \t]+head:[ \t]*([0-9a-fA-F]{7,40})[ \t]*$`)
	reviewLocRe  = regexp.MustCompile(`[^\s:` + "`" + `]:[0-9]+`)
)

// reviewSeverities are review.md's severities; `follow-up` is a defect that was already there and never blocks.
var reviewSeverities = map[string]bool{"blocker": true, "should": true, "nit": true, "follow-up": true}

// functionVerdicts are the one-thing-per-function verdicts of checks.md's function table.
var functionVerdicts = map[string]bool{"simple": true, "split": true, "merge": true, "inline": true}

// isFunctionHeader is the function table's header row, `function | what it does | verdict | action`, as split cells.
func isFunctionHeader(c []string) bool {
	cell := func(i int) string { return strings.Trim(strings.ToLower(strings.TrimSpace(c[i])), "` *") }
	return len(c) >= 3 && cell(0) == "function" && cell(2) == "verdict"
}

// sameSHA is two spellings of one commit: equal, or one a prefix of the other of at least 7 hex digits.
func sameSHA(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if len(a) < 7 || len(b) < 7 {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// ReviewerGate holds a reviewer's done to its review: of is the receipt's `<lane>:<task>@<sha>`, workHead the `git rev-parse HEAD`
// of the reviewer's own worktree (C3: the bytes it read are the bytes the SHA names). review.md's first line is
// `verdict: approve|changes-needed  head: <sha>` with that SHA, every row is `id | severity | file:line | …`, `approve` has no
// blocker row and `changes-needed` at least one (S3), and checks.md's function table has a verdict row for every function in
// functions.txt (a name hw could not extract is not owed).
func ReviewerGate(artifactsRoot, of, workHead string) (note string, err error) {
	review := artifactsRoot + "/review.md"
	how := "Write " + review + " with the first line  verdict: approve | changes-needed  head: <the SHA you judged>  and one row per finding,  id | blocker|should|nit|follow-up | file:line | failure scenario | fix direction  (approve has no blocker row; changes-needed has at least one; a defect that was already there is a follow-up and never blocks). NOTHING was sent or marked."
	want := of[strings.LastIndex(of, "@")+1:]
	if !strings.Contains(of, "@") || len(want) < 7 {
		return "", die("%s", fmt.Sprintf("this reviewer's receipt names the review as '%s', which is not <lane>:<task>@<sha>: hw review wrote it wrong. Report it with done-invoker --blocked. NOTHING was sent or marked.", of))
	}
	if !sameSHA(workHead, want) {
		got := workHead
		if got == "" {
			got = "<unreadable>"
		}
		return "", die("%s", fmt.Sprintf("this review is of %s and the worktree you read is at %s: what you judged is not the SHA the verdict will name. Do not commit in a review worktree; if it moved, close with done-invoker --blocked naming both SHAs, so the brainer dispatches a fresh review of the right head. NOTHING was sent or marked.", want, got))
	}
	b, rerr := os.ReadFile(review)
	if rerr != nil || !utf8.Valid(b) {
		return "", die("%s", fmt.Sprintf("this task is the independent review of %s and there is no readable %s. %s", of, review, how))
	}
	lines := strings.Split(string(b), "\n")
	m := reviewHeadRe.FindStringSubmatch(strings.TrimRight(lines[0], "\r"))
	if m == nil {
		return "", die("%s", fmt.Sprintf("the first line of %s is '%s', not  verdict: approve | changes-needed  head: <sha>. %s", review, strings.TrimSpace(lines[0]), how))
	}
	verdict, head := m[1], m[2]
	if !sameSHA(head, want) {
		return "", die("%s", fmt.Sprintf("%s judges head %s, and this review is of %s: a verdict for another SHA does not approve these bytes. Write the SHA you reviewed (%s). NOTHING was sent or marked.", review, head, want, want))
	}
	rows, blockers := 0, 0
	for i, line := range lines[1:] {
		l := strings.TrimSpace(line)
		if !strings.Contains(l, "|") || isSeparatorRow(l) {
			continue
		}
		c := splitUnescapedPipes(strings.TrimPrefix(l, "|"))
		sev := ""
		if len(c) > 1 {
			sev = strings.Trim(strings.ToLower(strings.TrimSpace(c[1])), "` *")
		}
		if sev == "severity" { // the header row
			continue
		}
		rows++
		switch {
		case !reviewSeverities[sev]:
			return "", die("%s", fmt.Sprintf("row at line %d of %s ('%s') has the severity '%s'. Severities are exactly blocker, should, nit or follow-up, in the second cell. %s", i+2, review, strings.TrimSpace(c[0]), sev, how))
		case len(c) < 3 || !reviewLocRe.MatchString(c[2]):
			return "", die("%s", fmt.Sprintf("row at line %d of %s ('%s') has no file:line in its third cell. Every finding names the place it is about. %s", i+2, review, strings.TrimSpace(c[0]), how))
		}
		if sev == "blocker" {
			blockers++
		}
	}
	if verdict == "approve" && blockers > 0 {
		return "", die("%s", fmt.Sprintf("%s says approve and carries %d blocker row(s): an approval with a blocker is a verdict that disagrees with its own findings. Say changes-needed, or re-rate the row (a defect that was already there is a follow-up). NOTHING was sent or marked.", review, blockers))
	}
	if verdict == "changes-needed" && blockers == 0 {
		return "", die("%s", fmt.Sprintf("%s says changes-needed and carries no blocker row: name the blocker (id | blocker | file:line | failure scenario | fix direction) or approve. NOTHING was sent or marked.", review))
	}
	fnNote, err := reviewerFunctionsCovered(artifactsRoot)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("review.md: %s at %s, %d finding row(s), %d blocker(s); %s", verdict, head, rows, blockers, fnNote), nil
}

// reviewerFunctionsCovered is the function table's coverage of functions.txt (`name<TAB>path` per line, written by hw review next
// to checks.md): a verdict row for every name. No functions.txt, or an empty one, owes nothing.
func reviewerFunctionsCovered(artifactsRoot string) (string, error) {
	fb, ferr := os.ReadFile(artifactsRoot + "/functions.txt")
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(fb), "\n") {
		if n, _, _ := strings.Cut(line, "\t"); strings.TrimSpace(n) != "" && !seen[strings.TrimSpace(n)] {
			seen[strings.TrimSpace(n)] = true
			names = append(names, strings.TrimSpace(n))
		}
	}
	if ferr != nil || len(names) == 0 {
		return "no functions listed, none owed", nil
	}
	file := artifactsRoot + "/checks.md"
	how := fmt.Sprintf("Add to %s the table  function | what it does | verdict | action  with one row per function in %s (verdict exactly simple, split, merge or inline; action none for simple), after a blank line. NOTHING was sent or marked.", file, artifactsRoot+"/functions.txt")
	var rows [][]string
	header, inFn := false, false
	if cb, err := os.ReadFile(file); err == nil && utf8.Valid(cb) {
		for _, line := range strings.Split(string(cb), "\n") {
			l := strings.TrimSpace(line)
			if !strings.Contains(l, "|") {
				inFn = false
				continue
			}
			if isSeparatorRow(l) {
				continue
			}
			c := splitUnescapedPipes(strings.TrimPrefix(l, "|"))
			if isFunctionHeader(c) {
				header, inFn = true, true
				continue
			}
			if inFn && len(c) >= 3 {
				rows = append(rows, c)
			}
		}
	}
	if !header {
		return "", die("%s", fmt.Sprintf("functions.txt lists %d changed function(s) and %s has no function table. %s", len(names), file, how))
	}
	var missing, bad []string
	for _, n := range names {
		found, ok := false, false
		for _, c := range rows {
			if !mentionsToken(c[0], n) {
				continue
			}
			found = true
			if functionVerdicts[strings.Trim(strings.ToLower(strings.TrimSpace(c[2])), "` *")] {
				ok = true
			}
		}
		switch {
		case !found:
			missing = append(missing, n)
		case !ok:
			bad = append(bad, n)
		}
	}
	shown := func(l []string) string {
		if len(l) > 8 {
			return strings.Join(l[:8], ", ") + fmt.Sprintf(" and %d more", len(l)-8)
		}
		return strings.Join(l, ", ")
	}
	if len(missing) > 0 {
		return "", die("%s", fmt.Sprintf("the function table of %s has no row for %d function(s) the diff changed: %s. %s", file, len(missing), shown(missing), how))
	}
	if len(bad) > 0 {
		return "", die("%s", fmt.Sprintf("the function table of %s has no valid verdict for %s. The third cell is exactly simple, split, merge or inline. %s", file, shown(bad), how))
	}
	return fmt.Sprintf("function table covers %d function(s)", len(names)), nil
}

// ReviewerGate is the Ctx side of ReviewerGate: it reads the reviewer's own worktree HEAD (C3), `git -C $HW_WORKDIR rev-parse HEAD`.
func (c *Ctx) ReviewerGate(of, artifactsRoot, workdir string) (string, error) {
	head := ""
	if workdir != "" {
		head, _ = c.out("git", "-C", workdir, "rev-parse", "HEAD")
	}
	return ReviewerGate(artifactsRoot, of, head)
}
