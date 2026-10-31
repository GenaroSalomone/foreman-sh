package main

// The review a build closes with is the brief's choice (`review:`), in every lane and every --sdd mode.
// bin/hw: _brief_review_mode, _review_skills_of_plugin, _review_skills_declared, _review_gate, _review_declared,
// _review_effect, _review_lines, _review_receipt, _plugins_review_note.
//
//	review: judgment-day   the Judgment Day closing (DEFAULT, also when absent)
//	review: plugins        the review skills of the brief's `plugins:`, and no Judgment Day
//	review: none           no review note at all
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
		if it == "judgment-day" {
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
		if it != "judgment-day" {
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
	case "", "rdd", "judgment-day":
		return "judgment-day", true // # MUTATION-ANCHOR: 1020-G01
	case "plugins":
		return "plugins", true // # MUTATION-ANCHOR: 1020-G02
	case "none":
		return "none", true // # MUTATION-ANCHOR: 1020-G03
	}
	return raw, false
}

// reviewSkillsOfPlugin is _review_skills_of_plugin: the review pieces each pinned plugin carries, one focus per plugin:
// thermo the maintainability review, ponytail the senior review, pstack the diff's security and correctness, what a change breaks
// outside the diff (blast-radius), needless comments and workaround code (comment-sicko, an agent) and maintainability.
func reviewSkillsOfPlugin(n string) []string {
	switch n {
	case "thermo":
		return []string{"thermo-nuclear-code-quality-review"}
	case "ponytail":
		return []string{"ponytail-review"}
	case "pstack":
		return []string{"thermo-nuclear-review", "blast-radius", "comment-sicko", "thermo-nuclear-code-quality-review"}
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
// plugin carries one. The subagent_type a plugin agent records is its full name, `plugin:agent`.
func reviewAgentsClause(plugins []string) string {
	ag := reviewPiecesToInvoke(plugins, true)
	if ag == "" {
		return ""
	}
	return " Launch the review agent " + ag + " with the Agent tool over the same range (its subagent_type is that full name)."
}

// reviewGateOf is _review_gate over what the brief says: it dies (die is the caller's) on a value that is not
// one of the three, warns on the retired `rdd`, and refuses `plugins` with no plugin to name. The mode returned
// is the one the brief chose.
func reviewGateOf(raw string, plugins []string, die, warn func(string)) string {
	mode, ok := reviewModeOf(raw)
	if !ok {
		if _, isList := reviewListOf(raw); isList {
			die("the brief declares review: " + mode + " — a list takes judgment-day and plugins with a review skill (thermo, ponytail, pstack). NOTHING was built.")
		} else {
			die("the brief declares review: " + mode + " — it takes judgment-day, plugins or none. NOTHING was built.")
		}
	}
	if raw == "rdd" {
		warn("brief declares review: rdd, which is retired (gentle-ai left hw): running review: judgment-day instead")
	}
	if mode == "plugins" && len(reviewSkillsOf(plugins)) == 0 {
		die("the brief declares review: plugins, but its plugins: names no plugin with a review skill (thermo, ponytail, pstack). NOTHING was built.")
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
("$(hw review-range)"..HEAD), each with the Skill tool under its full name: ` + reviewSkillsToInvoke(plugins) + `.` + reviewAgentsClause(plugins) + ` Judgment Day does not replace them.
A skill that is not among your available skills is not installed. The report names
each skill you ran and what it left open; a skill you did not run gets a line
"review-skip: <skill> — <why>". done-invoker reads your transcript for each
Skill call and refuses a done report that has neither.`)
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
   ones of the plugins this brief declares, each with the Skill tool under its full name: ` + reviewSkillsToInvoke(plugins) + `.` + reviewAgentsClause(plugins) + ` A skill that is not among your
   available skills is not installed: say so in the report and close normally.
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
	reviewGateOf(d.reviewRaw(), d.pluginsDeclared(), d.die, d.warn)
}

// reviewDeclared is _review_declared: only when the brief DECLARES review:, like plugins.
func (d *dsp) reviewDeclared() bool { return nextFmValue("review", d.brief) != "" }

// reviewLines is _review_lines: the manifest's review field, leading newline included; nothing when the brief
// declares none.
func (d *dsp) reviewLines() string {
	if !d.reviewDeclared() {
		return ""
	}
	mode, ok := d.briefReviewMode()
	if !ok {
		return ""
	}
	return "\n  review      " + mode + "  (brief review:) — " + reviewEffectOf(mode, d.pluginsDeclared())
}

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
	l.receiptInto(dir, key, mode, reviewReceiptSource(l.brief, mode, l.pluginsDeclared()))
}

// judgmentDayApplies is _judgment_day_applies: a base was computed, the brief did not choose plugins or none, and the task is a build.
func (l *lch) judgmentDayApplies() bool {
	if l.env("HW_REVIEW_BASE_REF") == "" { // # MUTATION-ANCHOR: 847-C-01
		return false
	}
	if m := l.reviewMode(); m == "plugins" || m == "none" { // # MUTATION-ANCHOR: 1020-G04
		return false
	}
	switch l.briefKindForClose() {
	case "", "build":
		return true // # MUTATION-ANCHOR: 847-C-02
	}
	return false
}

// pluginsReviewApplies is _plugins_review_applies: `review: plugins` closes a build with the plugins' review
// skills, base or no base.
func (l *lch) pluginsReviewApplies() bool {
	if l.reviewMode() != "plugins" { // # MUTATION-ANCHOR: 1020-G05
		return false
	}
	switch l.briefKindForClose() {
	case "", "build":
		return true
	}
	return false
}
