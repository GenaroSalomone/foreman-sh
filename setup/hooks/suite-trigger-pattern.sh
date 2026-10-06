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
SUITE_TRIGGER_PATTERN='^(bin/|lib/|setup/test-hw|setup/gate-select|setup/tests/|setup/guards/|([^/]+/)?\.claude/hooks/deny-repo-writes\.py|([^/]+/)?\.opencode/plugin/deny-repo-writes\.js)'

# LANE DOCUMENTATION THE SUITE DOES NOT READ — the ONE copy, shared by
# setup/hooks/pre-push (a push) and bin/hw (`hw done`'s cached verification).
# Two copies of this rule would drift, and the one that accepts more lets a
# change the suite reads ride on another tree's verdict. The rule itself, and
# why each path is on or off the list, is explained at pre-push's
# docs_only_verdict_covers; it was measured there, not here.
#
# Sourced, never executed. Needs SUITE_TRIGGER_PATTERN (above) and git.
#   docs_only_init <toplevel>     reads <toplevel>/projects.json for the lane names
#   docs_only_path <path>         0 when the path is lane documentation
#   docs_only_diff_covers <verdict-tree> <tree>
#                                 0 when EVERY path in the difference is lane
#                                 documentation; prints those paths on stdout.
#                                 Identical trees, an unreadable git, a lane
#                                 `decisions.md` whose header changed or that was
#                                 created or deleted: 1 (does not cover).
# A table that cannot be read, or a key that is not a lowercase word, leaves NO
# lane: nothing is documentation and every candidate fails closed.
docs_only_init() { # <toplevel>
  local lanes
  lanes="$(jq -r '.lanes | keys_unsorted[]' "$1/projects.json" 2>/dev/null || true)"
  if [ -z "$lanes" ] || printf '%s\n' "$lanes" | grep -vqE '^[a-z][a-z0-9-]*$'; then
    DOCS_ONLY_PATTERN='^/'   # no lane: no pushed path starts with /
  else
    DOCS_ONLY_PATTERN="^($(printf '%s\n' "$lanes" | paste -sd'|' -))/(briefs/.+|decisions\.md|decisions/.+)\$"
  fi
}
docs_only_path() { # <path> → 0 when the path is lane documentation the suite does not read
  case "$1" in
    setup/decisions.md|setup/decisions/*) return 1 ;;
  esac
  printf '%s\n' "$1" | grep -qE "$SUITE_TRIGGER_PATTERN" && return 1
  printf '%s\n' "$1" | grep -qE "$DOCS_ONLY_PATTERN"
}
decisions_header() { # <tree> <path> → the header, or fails if the blob is unreadable
  local blob
  blob="$(git cat-file blob "$1:$2" 2>/dev/null)" || return 1
  printf '%s\n' "$blob" | awk 'NR > 1 && $0 == "---" { exit } { print }'
}
docs_only_diff_covers() { # <verdict-tree> <tree>
  local vtree="$1" tree="$2" diff status p h_old h_new paths=""
  diff="$(git diff-tree -r --no-renames --name-status "$vtree" "$tree" 2>/dev/null)" || return 1
  [ -n "$diff" ] || return 1   # identical trees are the exact-tree check's, not this one's
  while IFS="$(printf '\t')" read -r status p; do
    [ -n "$p" ] || return 1
    docs_only_path "$p" || return 1
    case "$p" in
      */decisions.md)
        [ "$status" = M ] || return 1   # created or deleted: the header changed
        h_old="$(decisions_header "$vtree" "$p")" || return 1
        h_new="$(decisions_header "$tree" "$p")" || return 1
        [ "$h_old" = "$h_new" ] || return 1
        ;;
    esac
    paths="${paths:+$paths, }$p"
  done <<EOF2
$diff
EOF2
  printf '%s' "$paths"
}
