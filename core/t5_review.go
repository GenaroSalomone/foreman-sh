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

// reviewModeOf is _brief_review_mode: the mode, or the raw value and false when it is not one hw knows.
func reviewModeOf(raw string) (string, bool) {
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

// reviewSkillsOfPlugin is _review_skills_of_plugin: the review skills each pinned plugin carries.
func reviewSkillsOfPlugin(n string) []string {
	switch n {
	case "thermo":
		return []string{"thermo-nuclear-code-quality-review"}
	case "ponytail":
		return []string{"ponytail-review"}
	case "pstack":
		return []string{"thermo-nuclear-review", "thermo-nuclear-code-quality-review"}
	}
	return nil
}

// reviewSkillsOf is _review_skills_declared: the skills of the declared plugins, in the order they are declared,
// once each.
func reviewSkillsOf(plugins []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range plugins {
		for _, sk := range reviewSkillsOfPlugin(n) {
			if !seen[sk] {
				seen[sk] = true
				out = append(out, sk)
			}
		}
	}
	return out
}

// reviewGateOf is _review_gate over what the brief says: it dies (die is the caller's) on a value that is not
// one of the three, warns on the retired `rdd`, and refuses `plugins` with no plugin to name. The mode returned
// is the one the brief chose.
func reviewGateOf(raw string, plugins []string, die, warn func(string)) string {
	mode, ok := reviewModeOf(raw)
	if !ok {
		die("the brief declares review: " + mode + " — it takes judgment-day, plugins or none. NOTHING was built.")
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
	return "the Judgment Day closing"
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
   ones of the plugins this brief declares: ` + strings.Join(reviewSkillsOf(plugins), ", ") + `. A skill that is not among your
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
