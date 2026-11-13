package main

// The review a build closes with is the brief's choice (`review:`), in every lane and every --sdd mode.
// bin/hw: _brief_review_mode, _review_skills_of_plugin, _review_skills_declared, _review_gate, _review_declared,
// _review_effect, _review_lines, _review_receipt, _plugins_review_note.
//
//	review: judgment-day   the Judgment Day closing
//	review: plugins        the review skills of the brief's `plugins:`, and no Judgment Day
//	review: none           no review note at all (ALSO when `review:` is absent, on any kind: a review is opt-in, and
//	                       choosing none needs no review-skip: line. 2026-11-05, the operator: «some executor tasks don't
//	                       require it … not needed in the brief»)
//
// One reader, so the launch, the manifest, the receipt and `hw next` read ONE answer. `rdd` is the retired
// spelling of the gentle-ai review: it warns and falls back to judgment-day. The pure halves are here; the
// dispatch (dsp, lch) and `hw next` (t4_next_*.go) each hand them the brief's declarations.

import (
	"regexp"
	"strings"
)

var reviewCommentRe = regexp.MustCompile(`[[:space:]]*#.*$`)

// reviewRawOf is `_brief_fm_value review | tr 'A-Z' 'a-z' | sed -E 's/[[:space:]]*#.*$//'`, from the frontmatter value.
func reviewRawOf(fm string) string {
	lower := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, fm)
	return bashSub(reviewCommentRe.ReplaceAllString(lower, ""))
}

// reviewListOf reads `review: [judgment-day, thermo, ponytail]`: the items, and whether the value is a list at all.
// A list is the one field that says what the close runs AND what loads: each plugin item is loaded like a `plugins:` entry.
func reviewListOf(raw string) ([]string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(raw), "[") {
		return nil, false
	}
	return pluginsOfValue(raw), true
}

// reviewListMode is the mode a list stands for: judgment-day when it names it (the plugin skills are named beside it), plugins when
// it names only plugins, none when it is empty. false when an item is neither judgment-day nor a plugin with a review skill.
func reviewListMode(items []string) (string, bool) {
	jd := false
	for _, it := range items {
		if it == "judgment-day" || it == "rdd" { // rdd is the retired spelling: it adds the same Judgment Day (design 8 S4)
			jd = true
		} else if reviewSkillsOfPlugin(it) == nil {
			return it, false
		}
	}
	switch {
	case jd:
		return "judgment-day", true // # MUTATION-ANCHOR: 1150-G09
	case len(items) == 0:
		return "none", true
	}
	return "plugins", true
}

// reviewPluginsOfList is the plugin items of a `review:` list: what the brief asks hw to load for its review.
func reviewPluginsOfList(raw string) []string {
	items, _ := reviewListOf(raw)
	var out []string
	for _, it := range items {
		if it != "judgment-day" && it != "rdd" {
			out = append(out, it)
		}
	}
	return out
}

// pluginsWithReview is the brief's `plugins:` plus the plugin items of its `review:` list, once each, `plugins:` first.
func pluginsWithReview(declared []string, reviewRaw string) []string {
	out := append([]string{}, declared...)
	for _, n := range reviewPluginsOfList(reviewRaw) {
		if !has(declared, n) { // # MUTATION-ANCHOR: 1150-G10
			out = append(out, n)
		}
	}
	return out
}

// reviewModeOf is _brief_review_mode: the mode, or the raw value and false when it is not one hw knows.
func reviewModeOf(raw string) (string, bool) {
	if items, isList := reviewListOf(raw); isList {
		if m, ok := reviewListMode(items); ok {
			return m, true
		}
		return raw, false
	}
	switch raw {
	case "rdd", "judgment-day":
		return "judgment-day", true // # MUTATION-ANCHOR: 1020-G01
	case "plugins":
		return "plugins", true // # MUTATION-ANCHOR: 1020-G02
	case "", "none":
		return "none", true // # MUTATION-ANCHOR: 1020-G03
	}
	return raw, false
}

// reviewSkillsOfPlugin is _review_skills_of_plugin: the review pieces each pinned plugin carries, one focus per plugin:
// thermo the maintainability review, ponytail the senior review, pstack the diff's security and correctness, what a change breaks
// outside the diff (blast-radius), needless comments and workaround code (comment-sicko, an agent) and maintainability, tests the tests
// of the diff: each behaviour change has a test that fails on the old code and passes on the new (test-review, foreman's own).
func reviewSkillsOfPlugin(n string) []string {
	switch n {
	case "thermo":
		return []string{"thermo-nuclear-code-quality-review"}
	case "ponytail":
		return []string{"ponytail-review"}
	case "pstack":
		return []string{"thermo-nuclear-review", "blast-radius", "comment-sicko", "thermo-nuclear-code-quality-review"}
	case "tests":
		return []string{"test-review"}
	}
	return nil
}

// reviewPieceIsAgent is true for a review piece that is launched with the Agent tool, not the Skill tool.
func reviewPieceIsAgent(piece string) bool { return piece == "comment-sicko" }

// reviewSkillsOf is _review_skills_declared: the review pieces of the declared plugins, in the order they are declared,
// once each.
func reviewSkillsOf(plugins []string) []string {
	var out []string
	for _, full := range reviewSkillsQualified(plugins) {
		out = append(out, full[strings.Index(full, ":")+1:])
	}
	return out
}

// reviewSkillsQualified is the review pieces of the declared plugins by full name, `plugin:piece`, each piece once: a
// piece two plugins carry (thermo and pstack both carry thermo-nuclear-code-quality-review) runs from the first plugin
// declared. It is what the receipt holds and what done-invoker proves from the transcript, a Skill call or an Agent's
// subagent_type: a bare name matches a look-alike by substring.
func reviewSkillsQualified(plugins []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range plugins {
		for _, sk := range reviewSkillsOfPlugin(n) {
			if !seen[sk] {
				seen[sk] = true
				out = append(out, n+":"+sk)
			}
		}
	}
	return out
}

// reviewPiecesToInvoke is the qualified names of the pieces launched with the Skill tool (agents false) or the Agent tool.
func reviewPiecesToInvoke(plugins []string, agents bool) string {
	var out []string
	for _, full := range reviewSkillsQualified(plugins) {
		if reviewPieceIsAgent(full[strings.Index(full, ":")+1:]) == agents {
			out = append(out, full)
		}
	}
	return strings.Join(out, ", ")
}

// reviewSkillsToInvoke is the skills as the closing's prose says them: the Skill tool takes the full name.
func reviewSkillsToInvoke(plugins []string) string { return reviewPiecesToInvoke(plugins, false) }

// reviewAgentsClause is the sentence that sends the agents (comment-sicko) through the Agent tool: empty when no declared
// plugin carries one. The subagent_type a plugin agent records is its full name, `plugin:agent`. FOREGROUND is said out
// loud, on every vendor: done-invoker proves a review piece only from a call that ran and waited, and a background launch
// proves the start alone (measured 2026-11-09: an executor launched pstack:comment-sicko with run_in_background, the proof
// was refused, and it cost 8.5 min and two more cycles).
func reviewAgentsClause(plugins []string) string {
	ag := reviewPiecesToInvoke(plugins, true)
	if ag == "" {
		return ""
	}
	return " Launch the review agent " + ag + " with the Agent tool over the same range, in the foreground — no background launch (`run_in_background`, `async:true`); where the call takes an `async` field, pass `async:false`: a background call proves the launch, not the run. Its subagent_type is that full name."
}

// reviewGateOf is _review_gate over what the brief says: it dies (die is the caller's) on a value that is not
// one of the three, warns on the retired `rdd`, and refuses `plugins` with no plugin to name. The mode returned
// is the one the brief chose.
func reviewGateOf(raw string, plugins []string, die, warn func(string)) string {
	mode, ok := reviewModeOf(raw)
	if !ok {
		if _, isList := reviewListOf(raw); isList {
			die("the brief declares review: " + mode + " — a list takes judgment-day and plugins with a review skill (thermo, ponytail, pstack, tests). NOTHING was built.")
		} else {
			die("the brief declares review: " + mode + " — it takes judgment-day, plugins or none. NOTHING was built.")
		}
	}
	if raw == "rdd" {
		warn("brief declares review: rdd, which is retired (gentle-ai left hw): running review: judgment-day instead")
	}
	if mode == "plugins" && len(reviewSkillsOf(plugins)) == 0 {
		die("the brief declares review: plugins, but its plugins: names no plugin with a review skill (thermo, ponytail, pstack, tests). NOTHING was built.")
	}
	return mode
}

// reviewEffectOf is _review_effect: the words after the mode, for the manifest and the receipt.
func reviewEffectOf(mode string, plugins []string) string {
	switch mode {
	case "plugins":
		return "the closing names the review skills of the brief's plugins (" + strings.Join(reviewSkillsOf(plugins), ", ") + ") and no Judgment Day"
	case "none":
		return "no review note at the close"
	}
	if sk := reviewSkillsOf(plugins); len(sk) > 0 {
		return "the Judgment Day closing, preceded by the review skills of the brief's plugins (" + strings.Join(sk, ", ") + ")"
	}
	return "the Judgment Day closing"
}

// pluginReviewSkillsNamed is what the closing of a build names as plugin review skills: the plugins' skills for
// `review: plugins`, and for `review: judgment-day` the same ones beside the Judgment Day note (which a base ref
// gates). It is the receipt's value and what done-invoker holds the report to, so both read ONE answer.
func pluginReviewSkillsNamed(mode string, baseRef string, build bool, plugins []string) []string {
	if !build {
		return nil
	}
	switch mode {
	case "plugins":
		return reviewSkillsOf(plugins) // # MUTATION-ANCHOR: 1150-G01
	case "judgment-day":
		if baseRef != "" { // # MUTATION-ANCHOR: 1150-G02
			return reviewSkillsOf(plugins)
		}
	}
	return nil
}

// reviewDiffFileClause hands every review child the diff as a file: some have no shell (pi-subagents' reviewer declares
// read, grep, find, ls only) and cannot run git. Same text on every vendor.
const reviewDiffFileClause = " Write that range's diff to `$HW_ARTIFACTS/review-range.diff` and give that path to every review subagent and judge: some have no shell (pi's reviewer reads files only) and cannot run git."

// judgmentDayWithPlugins is the Judgment Day closing plus the step that names the plugin review skills: run over
// the same committed range, before Judgment Day, and the report says each one ran or was skipped.
func judgmentDayWithPlugins(note string, plugins []string) string {
	sk := reviewSkillsOf(plugins)
	if len(sk) == 0 { // # MUTATION-ANCHOR: 1150-G03
		return note
	}
	return bashSub(note + `

This brief also declares review plugins, so the closing has one more step, BEFORE
the Judgment Day of step 2: run these review skills over the same committed range
("$(hw review-range)"..HEAD), each with the Skill tool under its full name: ` + reviewSkillsToInvoke(plugins) + `.` + reviewAgentsClause(plugins) + reviewDiffFileClause + ` Judgment Day does not replace them.
A skill that is not among your available skills means this pane was launched without
its plugin: close with done-invoker --blocked naming it, never as "not installed". The
report names each skill you ran and what it left open. done-invoker reads your
transcript for each Skill call and refuses a done report without one; a plugin vendored
in setup/vendor/plugins is never skipped, and a line "review-skip: <skill> — <why>" is
accepted only for a piece no vendored plugin ships.`)
}

// reviewReceiptSource is the receipt's third column.
func reviewReceiptSource(brief, mode string, plugins []string) string {
	return "brief frontmatter review: in " + orDefaultStr(brief, "<no brief>") + " — " + reviewEffectOf(mode, plugins)
}

// reviewPluginsNote is _plugins_review_note: the closing of a build that chose `review: plugins`.
func reviewPluginsNote(plugins []string) string {
	return bashSub(`Closing this code task: the review this brief chose, then done-invoker, in this order:

1. Commit per work unit as you go, on this branch, with Conventional Commit
   messages — not one commit at the end. The review target is what you
   committed; uncommitted work is not reviewed and is not delivered.
2. Before done-invoker, run these review skills over your committed diff, the
   ones of the plugins this brief declares, each with the Skill tool under its full name: ` + reviewSkillsToInvoke(plugins) + `.` + reviewAgentsClause(plugins) + reviewDiffFileClause + ` A skill that is not among your
   available skills means this pane was launched without its plugin: close with
   done-invoker --blocked naming it; a vendored plugin is never skipped.
   Do NOT run the judgment-day skill: this brief chose its review, and two
   verdicts on one candidate is the cost it removes.
3. The done-invoker report names each review skill you ran and what it left
   open.`)
}

// reviewDesignEnd is the `_dz_end` of _design_judgment_note: what the end of the task is called, by the review chosen.
func reviewDesignEnd(mode string) string {
	switch mode {
	case "plugins":
		return "the closing review at the end"
	case "none":
		return "the end of the task"
	}
	return "the code-mode Judgment Day at the end"
}

// ── the dispatch (dsp, and lch which embeds it) ─────────────────────────────

// reviewRaw is the brief's review value as _brief_review_mode reads it.
func (d *dsp) reviewRaw() string { return reviewRawOf(nextFmValue("review", d.brief)) }

// briefReviewMode is _brief_review_mode over $BRIEF.
func (d *dsp) briefReviewMode() (string, bool) { return reviewModeOf(d.reviewRaw()) }

// reviewMode is `$(_brief_review_mode 2>/dev/null)` where the caller compares the mode: "" when it is not known.
func (d *dsp) reviewMode() string {
	m, ok := d.briefReviewMode()
	if !ok {
		return ""
	}
	return m
}

// reviewGate is _review_gate: raised where the contract is checked, before anything is built.
func (d *dsp) reviewGate() {
	mode := reviewGateOf(d.reviewRaw(), d.pluginsDeclared(), d.die, d.warn)
	k := d.briefKindForClose()
	if reviewBareNone(k == "" || k == "build", d.reviewDeclared(), mode, nextFmValue("review-skip", d.brief)) {
		d.warn(reviewBareNoneWarning)
	}
}

// reviewDeclared is _review_declared: only when the brief DECLARES review:, like plugins.
func (d *dsp) reviewDeclared() bool { return nextFmValue("review", d.brief) != "" }

// reviewIndependentEffect is the words after the mode for a build that owes the independent review: what the builder does, what
// the reviewer adds for what the brief declared.
func reviewIndependentEffect(raw, mode string, declaredPlugins []string) string {
	e := "an independent review after the report (hw review <lane> <task>); the builder closes with /simplify on claude and runs no Judgment Day and no review skill"
	if mode == "judgment-day" {
		e += "; the reviewer runs Judgment Day (high risk, declared)"
	}
	if extra := reviewExtraPlugins(raw, declaredPlugins); len(extra) > 0 {
		e += "; the reviewer's tier adds " + strings.Join(extra, ", ")
	}
	return e
}

// reviewLines is _review_lines: the manifest's review field, leading newline included. A build that owes the independent review always
// prints it (declared or the default); any other task prints it only when the brief declares review:.
func (d *dsp) reviewLines() string {
	mode, ok := d.briefReviewMode()
	if d.independentReview() && ok {
		src := "default: every build owes it"
		if d.reviewDeclared() {
			src = "brief review:"
		}
		return "\n  review      independent  (" + src + ") — " + reviewIndependentEffect(d.reviewRaw(), mode, d.declaredPlugins())
	}
	if !d.reviewDeclared() || !ok {
		return ""
	}
	return "\n  review      " + mode + "  (brief review:) — " + reviewEffectOf(mode, d.pluginsDeclared())
}

// declaredPlugins is the brief's `plugins:` alone, without the plugin items of its review: list.
func (d *dsp) declaredPlugins() []string { return pluginsOfValue(d.briefDecl("plugins")) }

// reviewReceipt is _review_receipt <run dir> [<key>].
func (l *lch) reviewReceipt(dir, key string) {
	if !l.reviewDeclared() {
		return
	}
	mode, ok := l.briefReviewMode()
	if !ok {
		return
	}
	if key == "" {
		key = "review"
	}
	src := reviewReceiptSource(l.brief, mode, l.pluginsDeclared())
	if l.independentReview() {
		src = "brief frontmatter review: in " + orDefaultStr(l.brief, "<no brief>") + " — " + reviewIndependentEffect(l.reviewRaw(), mode, l.declaredPlugins())
	}
	l.receiptInto(dir, key, mode, src)
}

// ── the independent review: what a build owes (design §3.1, §8 C1, S4) ──────

// reviewOwedBy is the one answer to «does this build owe the independent review»: every `review:` value owes it — absent, a
// list, judgment-day, rdd, plugins and a BARE none — except `review: none` WITH a `review-skip:` reason, the one recorded
// opt-out (S4). A brief of any other kind never asks (the callers pass build).
func reviewOwedBy(build, declared bool, mode, skip string) bool {
	if !build {
		return false
	}
	return !(declared && mode == "none" && skip != "")
}

// reviewBareNone is `review: none` on a build with no `review-skip:` line: it still owes the review (reviewOwedBy), and
// this release only warns about the missing reason. The release after refuses it.
func reviewBareNone(build, declared bool, mode, skip string) bool {
	return build && declared && mode == "none" && skip == ""
}

const reviewBareNoneWarning = "the brief declares review: none with no review-skip: line. On a build that no longer opts out: the independent review is still owed. Add  review-skip: <why>  to the frontmatter to opt out (a bare none is refused from the release after this one)."

// reviewExtraPlugins is what a build's `review:` adds to the reviewer's tier (S4): a list's plugin items, `plugins` (scalar) = the
// brief's `plugins:` that carry a review piece. judgment-day and rdd add Judgment Day, not a plugin (the plan reads the mode).
func reviewExtraPlugins(raw string, declared []string) []string {
	if raw == "plugins" {
		var out []string
		for _, n := range declared {
			if reviewSkillsOfPlugin(n) != nil {
				out = append(out, n)
			}
		}
		return out
	}
	return reviewPluginsOfList(raw)
}

// independentReview is what a build owes at its dispatch: see reviewOwedBy.
func (d *dsp) independentReview() bool {
	k := d.briefKindForClose()
	mode, _ := d.briefReviewMode()
	return reviewOwedBy(k == "" || k == "build", d.reviewDeclared(), mode, nextFmValue("review-skip", d.brief))
}

// closeSimplifyPiece is the one piece a claude builder proves at its close: /simplify, the Skill's own name (MEASURED in a transcript,
// design 1.2). The receipt row is the plugin review's: done-invoker proves a name from the transcript, or takes a review-skip: line.
func closeSimplifyPiece(vendor string) string {
	if vendor == "claude" {
		return "simplify"
	}
	return ""
}

// ── the independent review: what the dispatch writes (design §8 C1, S1) ─────

// reviewOwedReceipt is C1: a build owes an independent review from its dispatch, and the receipt says so. Not written for
// `review: none` WITH a `review-skip:` reason (the one recorded opt-out), nor for a kind that is not a build. A failed or
// forgotten `hw review` therefore leaves `review_owed=1` standing: the absence of a verdict is never read as «nothing owed».
func (l *lch) reviewOwedReceipt(dir string) {
	if !l.independentReview() {
		return
	}
	l.receiptInto(dir, "review_owed", "1",
		"kind build (declared or undeclared) at dispatch, and no `review: none` with a review-skip: reason in "+orDefaultStr(l.brief, "<no brief>")+" — `hw review` measures the diff; until a verdict at the head, a recorded review-skip or --accept-review, a merge is not offered")
}

// reviewerOf is the `review_of:` of a kind: review brief — `<lane>:<task>@<sha>` — and "" for every other brief.
func (l *lch) reviewerOf() string {
	if l.briefKindForClose() != "review" {
		return ""
	}
	return nextFmValue("review_of", l.brief)
}

// reviewerReceipts is S1: the gate keys a build gets from its kind and its review plugins are inert on `kind: review`, so a
// reviewer's dispatch writes its own — the pieces it must prove from its transcript (`plugin_review_t1`), the checks.md it
// must leave (`checks_required_t1`) and what it judges (`review_of_t1`). The judges' row is jdAgentsReceipt's.
func (l *lch) reviewerReceipts(dir string) {
	of := l.reviewerOf()
	if of == "" {
		return
	}
	src := "brief review_of: and review: in " + l.brief + " at dispatch"
	l.receiptInto(dir, "review_of_t1", of, src+" — the build and the SHA this run judges: its review.md must name this head")
	list, _ := reviewListOf(l.reviewRaw())
	if pieces := reviewerPieces(list); len(pieces) > 0 {
		l.receiptInto(dir, "plugin_review_t1", strings.Join(pieces, ","), src+" — done-invoker proves each piece from the transcript, or takes a review-skip: line")
	}
	l.receiptInto(dir, "checks_required_t1", "required", "kind review with review_of: at dispatch — done-invoker requires $HW_ARTIFACTS/checks.md")
}
