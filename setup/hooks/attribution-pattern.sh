# shellcheck shell=bash
# attribution-pattern.sh — the ONE place that names what counts as AI
# attribution in a commit message, and the ONE place that formats the refusal.
#
# WHY THIS FILE EXISTS. The standing rule is "Never add Co-Authored-By or AI
# attribution to commits" (~/.claude/CLAUDE.md, ## Rules). Until 2026-09-16 it
# was enforced in exactly one place, setup/hooks/pre-push, with the regex
# written inline there — and a SECOND gate now enforces it at the moment the
# message is written, setup/hooks/commit-msg. Two copies of a detection regex
# drift the moment one is edited and the other is not, and here the drift is
# not symmetric: the copy that matches LESS lets attribution through the gate
# that is supposed to be the earliest one, and the offending commit is then
# only caught days later at push time — or, if the second copy is the push
# one, never.
#
# This is the same reasoning, and the same idiom, as
# setup/hooks/suite-trigger-pattern.sh: one definition, sourced by every hook
# that needs to answer the question, resolved through
# `git rev-parse --show-toplevel` so a hook still works as a bare symlink with
# nothing beside it in .git/hooks.
#
# Sourced, never executed. Defines ATTRIBUTION_PATTERN and
# attribution_offending_lines.
#
# ── A TRAILER IS NOT A QUOTATION, AND THE ANCHOR IS WHAT TELLS THEM APART ──
#
# THE DEFECT THIS ANCHOR FIXES, measured 2026-09-17 by running the detection
# over the 55 commits this repository had not yet pushed. Three were refused;
# exactly one deserved it:
#
#     222f9d7  a REAL trailer, at column 0.                  refused, rightly
#     c70dbe7  the same text quoted, indented two spaces.    refused, WRONGLY
#     9dc5ab8  the same text quoted inside a shell example.  refused, WRONGLY
#
# c70dbe7 and 9dc5ab8 are the two commits that BUILT this guard. They paste the
# trailer into their own messages as evidence that the detection works, because
# this lane's rules require the evidence to travel inside the commit message —
# and the detection then bit the evidence. The release sat blocked behind two
# false positives of our own making.
#
# SO THE ANCHOR IS `^`, NOT `^[[:space:]]*`. A git trailer is a line of its own
# beginning at column 0: that is what `git interpret-trailers` writes, what
# every agent harness emits, and what `git log --format=%B` hands back
# unmodified. A quotation is indented, or sits inside a fenced block, or is
# embedded mid-line in a shell example. Column 0 is the whole discriminator,
# and it is why the refusal below can state the column as a constant.
#
# THE OLD `[[:space:]]*` WAS NEVER JUSTIFIED BY A MEASUREMENT, AND ONE WAS
# LOOKED FOR BEFORE IT WAS REMOVED. It arrived with the first inline version of
# this regex in 684b760 and was carried into this file verbatim by 9dc5ab8;
# neither commit message, nor setup/decisions.md, records a case it was there
# to catch. The search: every commit in this history, grepped for an attribution
# line with LEADING WHITESPACE — `^[[:space:]]+(…)`. It returns exactly three
# hits, and all three are the two false positives above (c70dbe7 twice,
# 9dc5ab8 once). No real indented trailer exists anywhere in this history.
# UNESTABLISHED, and named rather than assumed away: whether some tool
# somewhere emits an indented trailer token. None was found here. A folded
# RFC822 continuation line IS indented, but it carries only the VALUE — its
# `Co-Authored-By:` token line is still at column 0 and still refused.
#
# ERRING TOWARD REFUSAL, deliberately. A false negative is worse than a false
# positive, so `^` still refuses a quotation that happens to sit at column 0 —
# an unindented trailer inside a fenced code block is refused exactly like a
# real one. The escape hatch for that case is the same as every other gate
# here: --no-verify. What the anchor buys is that the ordinary way to quote
# evidence in this repo — indent it — stops being an offense.
#
# WHAT IT MATCHES, and why each alternative is there. It is applied with
# `grep -iE`, so it is case-insensitive by the caller.
#
#   co-authored-by:.*(claude|anthropic|gpt|codex|opencode|copilot)
#       The trailer form every agent harness emits. The vendor list is what
#       this machine actually runs; a human co-author trailer is NOT refused,
#       because co-authorship with a person is legitimate.
#   generated with \[?claude
#       "🤖 Generated with [Claude Code](https://claude.com/claude-code)" with
#       or without the markdown link bracket.
#   🤖 generated
#       The same line when the emoji leads and the wording differs.
#
# A KNOWN GAP THAT LIVES IN THIS ANCHOR, AND IT IS OPEN — UNCHANGED BY THE
# NARROWING ABOVE. A trailer written as `# Co-Authored-By: Claude …` matches
# NEITHER gate, and it did not match the old anchor either: `#` is not
# whitespace. Under the non-interactive `whitespace` cleanup git keeps such a
# line in the commit (measured 2026-09-16, git 2.50.1); that the anchor
# excludes it is readable straight off the regex and needed no measuring.
# Closing it would mean matching a `#`-commented quotation of the rule, which
# this repo's own files write constantly, so it is named rather than closed.
# Pinned by the `hash-gap` arm in
# setup/tests/152-la-atribucion-muere-en-commit-msg.sh.
ATTRIBUTION_PATTERN='^(co-authored-by:.*(claude|anthropic|gpt|codex|opencode|copilot)|generated with \[?claude|🤖 generated)'

# attribution_offending_lines [<indent>] — reads a commit message on stdin and
# prints one human-facing line per offense, nothing at all when it is clean.
#
# WHY THE FORMATTING LIVES HERE TOO, beside the pattern rather than in each
# hook. The refusal used to say only "attribution" and quote the text, and a
# reader holding both a real trailer and a pasted piece of evidence could not
# tell which one had been refused — that ambiguity is half of what cost this
# release a day. Naming the line and the column answers it, and the answer must
# read identically from both gates, so it is defined once, here.
#
# THE COLUMN IS 1, AND IT IS A CONSTANT BY CONSTRUCTION, not a value anyone
# measured per line: ATTRIBUTION_PATTERN is anchored at `^`, so a match can
# begin nowhere else. That is the point worth telling the reader — column 1 IS
# the rule. If the anchor is ever widened, this constant becomes a lie; the
# `anchor` arm in setup/tests/152 asserts the anchor's shape so that widening
# it fails a test that points straight back at this comment.
attribution_offending_lines() {
  local indent="${1:-    }" hit
  # `|| true`: grep exits 1 on a clean message, which is the ordinary case and
  # must not kill a caller running under `set -e`.
  # Bytes, not characters: under a UTF-8 locale Git Bash's grep did not match
  # the literal 🤖 (outside the BMP) and let the robot line through. Every
  # other alternative is ASCII, which -i folds the same in C.
  LC_ALL=C grep -niE "$ATTRIBUTION_PATTERN" | while IFS= read -r hit; do
    [ -n "$hit" ] || continue
    printf '%sline %s, column 1: %s\n' "$indent" "${hit%%:*}" "${hit#*:}"
  done || true
}
