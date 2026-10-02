#!/usr/bin/env bash
# A behaviour fix ships the test that fails on the old code — and the rule
# reaches every build executor through its preamble, not through a file every
# session of every lane pays for.
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process. Run it alone while working on this subject:
#
#     bash setup/tests/183-un-fix-prueba-el-rojo.sh
#
# THE PROMPT IS RENDERED, NOT GREPPED, with the same extraction 166 uses.
# `SUBJECT_HW` runs the same assertions against the pre-change binary, which is
# how the old/new evidence in the commit was produced.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SRC="${SUBJECT_HW:-$ROOT/bin/hw}"
FRAG="$TMP/preamble-fragment.sh"
python3 - "$SRC" "$FRAG" <<'PY'
import sys
src = open(sys.argv[1], encoding="utf-8").read()
start = src.index("_specialist_agents() {\n")
end = src.index("\n# ONE DELIVERY PATH FOR BRIEFS AND RE-TASKS.", start)
open(sys.argv[2], "w", encoding="utf-8").write(src[start:end])
PY

brief() {  # $1 = file, $2 = kind (empty = no frontmatter)
  if [ -n "$2" ]; then printf -- '---\nkind: %s\n---\n\n# t\n' "$2" > "$1"
  else printf '# t\n' > "$1"; fi
}

# render <brief> <base_ref> <task_kind>
render() {
  B="$1" R="$2" K="$3" FRAG="$FRAG" bash -c '
    set -euo pipefail
    BRIEF="$B"; TASK=probe; PROJ=setup; TASK_KIND="$K"
    HW_BIN_DIR=/fixture/bin; HW_WORKDIR=/fixture/work; ENGRAM_PROJECT=brain
    HW_REVIEW_BASE_REF="$R"; HW_REVIEW_BASE_BRANCH=main
    source "$FRAG"
    info() { :; }; warn() { :; }; _specialist_agents() { :; }; lane_get() { :; }
    _brief_preamble
  '
}

RED='A behaviour fix ships the test that fails on the old code.'

brief "$TMP/build.md" build
brief "$TMP/plain.md" ""
brief "$TMP/audit.md" audit
brief "$TMP/explore.md" explore

# ── 1. a build carries it: declared, inferred, undeclared, with or without a base
for c in "build.md||" "build.md|abc123|" "plain.md|abc123|" "plain.md||build" "plain.md||"; do
  IFS='|' read -r f r k <<< "$c"
  out="$(render "$TMP/$f" "$r" "$k")"
  case "$out" in *"$RED"*) ;; *) fail "red test: a build preamble ($c) omits the rule" ;; esac
  case "$out" in *"revert"*"red"*"green"*) ;; *) fail "red test: the rule ($c) does not say how to prove it — revert, red, green" ;; esac
  case "$out" in *"refactor"*) ;; *) fail "red test: the rule ($c) lost the refactor exception" ;; esac
done
pass "red test: every build preamble carries the rule, its proof and the refactor exception"

# ── 2. an audit does not: its output is a report, there is no fix to prove ──
for c in "audit.md|abc123|" "explore.md|abc123|" "plain.md|abc123|audit" "plain.md||review"; do
  IFS='|' read -r f r k <<< "$c"
  out="$(render "$TMP/$f" "$r" "$k")"
  case "$out" in
    *"$RED"*) fail "red test: a non-build preamble ($c) carries the rule" ;;
    *"Close with done-invoker"*) ;;
    *) fail "red test: the non-build control ($c) rendered no preamble at all" ;;
  esac
done
pass "red test: audit, explore and review preambles do not carry it"

# ── 3. and it costs the shared rules nothing ────────────────────────────────
# The export carries no CLAUDE.shared.md (setup/export/manifest): said, not a
# grep error that reads as a pass.
if [ -f "$ROOT/CLAUDE.shared.md" ]; then
  grep -q 'FAILS ON THE OLD CODE' "$ROOT/CLAUDE.shared.md" \
    && fail "red test: the rule was copied into CLAUDE.shared.md, which every session pays for"
  pass "red test: the rule lives only where it applies"
else
  pass "red test: this tree carries no CLAUDE.shared.md, so no session pays for the rule"
fi
