# shellcheck shell=bash
# suite-trigger-pattern.sh — the ONE place that names which paths can
# invalidate setup/test-hw's verdict.
#
# WHY THIS FILE EXISTS. The same `grep -E` pattern used to be written twice
# inside setup/hooks/pre-commit (once to decide whether to run the fast gate
# at all, once again inside its failure branch to name what triggered it),
# and setup/hooks/pre-push had no such pattern at all — it required a cached
# verdict for every push regardless of what the push actually touched. Three
# copies of a pattern joined by `|` drift the moment one is edited and the
# others are not; the copy that relaxes LEAST is merely annoying, but the
# copy that relaxes MOST lets a change to bin/ (or to the suite, the guards,
# or either deny-repo-writes hook) reach a remote with no verdict behind it.
# So there is exactly one copy, sourced by every hook that needs to answer
# "can this path invalidate the suite's verdict".
#
# Sourced, never executed. Defines SUITE_TRIGGER_PATTERN only.
#
# THE LANE PREFIX ON THE TWO deny-repo-writes ALTERNATIVES IS OPTIONAL,
# `([^/]+/)?`, not `[^/]+/`. This repo's own CLAUDE.md is explicit that "the
# root is a lane" — `<brain>/{,<lane>/}.claude/hooks/deny-repo-writes.py`
# includes the EMPTY brace option, and the
# root-level file exists on disk at `.claude/hooks/deny-repo-writes.py` (no
# directory before `.claude/`). A mandatory-prefix `[^/]+/` cannot match a path
# with no prefix at all, so a push touching ONLY the root guard — e.g.
# loosening it — would have been classified as "nothing triggers" by this
# pattern's first version, found by adversarial review 2026-09-09 once this
# pattern started gating a mandatory push, not just a local fast-gate hint.
# setup/gate-select since 2026-09-30: it decides which subjects a commit runs,
# so a commit that touches ONLY it would otherwise run no gate at all (Judgment
# Day, tests/361 S08).
SUITE_TRIGGER_PATTERN='^(bin/|setup/test-hw|setup/gate-select|setup/tests/|setup/guards/|([^/]+/)?\.claude/hooks/deny-repo-writes\.py|([^/]+/)?\.opencode/plugin/deny-repo-writes\.js)'
