#!/usr/bin/env bash
# A code task closes with Judgment Day over its own branch, and a Spec Kit
# phase is not the end of the task.
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process. Run it alone while working on this subject:
#
#     bash setup/tests/166-el-cierre-es-judgment-day.sh
#
# THE PROMPT IS RENDERED, NOT GREPPED. The real preamble fragment is extracted
# from bin/hw (same anchor as 27-brief-epistemic-safeguard.sh) and executed;
# only the delivery edge is stubbed. `SUBJECT_HW` lets the same assertions
# run against the pre-change binary, which is how the old/new evidence in the
# commit was produced.
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

# render <brief> <sdd> <framework> <in_play> <base_ref> <task_kind>
render() {
  B="$1" S="$2" F="$3" P="$4" R="$5" K="$6" FRAG="$FRAG" bash -c '
    set -euo pipefail
    BRIEF="$B"; BRIEF_TEXT="$(<"$BRIEF")"
    SDD="$S"; DISPATCH_FRAMEWORK="$F"; DISPATCH_IN_PLAY="$P"
    TASK=probe; PROJ=setup; TASK_KIND="$K"
    HW_BIN_DIR=/fixture/bin; HW_WORKDIR=/fixture/work; ENGRAM_PROJECT=brain
    HW_REVIEW_BASE_REF="$R"; HW_REVIEW_BASE_BRANCH=main
    source "$FRAG"
    info() { :; }; warn() { :; }; _specialist_agents() { :; }
    _framework_entry() { [ "$1" != speckit ] || printf "/speckit-specify"; }
    AGENT=claude
    _deliver_brief() { printf "%s" "$BRIEF_TEXT"; }
    _receipt_session() { :; }; _receipt_model() { :; }
    _send_brief fixture-pane
  '
}

JD='CLOSING THIS CODE TASK: JUDGMENT DAY, THEN done-invoker.'
SK='A SPEC KIT PHASE IS NOT THE TASK.'

brief "$TMP/build.md" build
brief "$TMP/plain.md" ""
brief "$TMP/audit.md" audit
brief "$TMP/explore.md" explore

# ── 1. judgment day closes a build, over the branch hw computed ────────────
out="$(render "$TMP/build.md" none none 0 abc123 "")"
case "$out" in
  *"$JD"*"hw review-range"*"IS that"*"authorization"*"APPROVED or ESCALATED"*"only ONE judge"*) pass "preamble: a declared build closes with judgment-day over HW_REVIEW_BASE_REF..HEAD, pre-authorized, verdict + single-judge findings in the report" ;;
  *) fail "preamble: a declared build does not carry the judgment-day closing. got: $(printf '%s' "$out" | tail -c 600)" ;;
esac
case "$out" in
  *"Commit per work unit"*) pass "preamble: the build closing asks for a commit per work unit" ;;
  *) fail "preamble: the build closing does not ask for per-work-unit commits" ;;
esac
case "$(render "$TMP/plain.md" none none 0 abc123 "")" in
  *"$JD"*) pass "preamble: an undeclared kind is dispatched as a build and carries judgment-day" ;;
  *) fail "preamble: an undeclared kind lost the judgment-day closing" ;;
esac

# ── ...and not a task with no diff to judge ─────────────────────────────────
for k in audit explore; do
  case "$(render "$TMP/$k.md" none none 0 abc123 "")" in
    *"$JD"*) fail "preamble: a brief declaring kind: $k was told to run judgment-day on a diff it does not produce" ;;
    *) pass "preamble: kind: $k (read from the brief, the hw next route) carries no judgment-day" ;;
  esac
done
# An inline YAML comment after the kind is not part of it.
printf -- '---\nkind: build  # writes code\n---\n\n# t\n' > "$TMP/build-comment.md"
case "$(render "$TMP/build-comment.md" none none 0 abc123 "")" in
  *"$JD"*) pass "preamble: an inline comment after kind: build does not suppress judgment-day" ;;
  *) fail "preamble: kind: build with an inline comment lost the judgment-day closing" ;;
esac
# The base paragraph is JD's input, so it follows the same gate.
case "$(render "$TMP/audit.md" none none 0 abc123 "")" in
  *"THE BASE A JUDGMENT DAY TARGET"*) fail "preamble: kind: audit still gets the judgment-day base paragraph, with no diff to judge" ;;
  *) pass "preamble: kind: audit gets no base paragraph either" ;;
esac
case "$(render "$TMP/build.md" none none 0 abc123 "")" in
  *"THE BASE A JUDGMENT DAY TARGET"*) pass "preamble: a build still gets the base paragraph" ;;
  *) fail "preamble: the base paragraph vanished from a build" ;;
esac
# The executor flow never launches the fix agent; the closing does not name it.
case "$(render "$TMP/build.md" none none 0 abc123 "")" in
  *jd-fix-agent*) fail "preamble: the judgment-day closing names jd-fix-agent, which no recorded executor run uses" ;;
  *) pass "preamble: the judgment-day closing does not name jd-fix-agent" ;;
esac
# The launch route resolves the kind before the preamble, from the brief OR
# the task name; the resolved value wins over an undeclared brief.
case "$(render "$TMP/plain.md" none none 0 abc123 review)" in
  *"$JD"*) fail "preamble: a task resolved as kind review still carries judgment-day" ;;
  *) pass "preamble: a kind resolved at launch (review) suppresses judgment-day" ;;
esac
case "$(render "$TMP/build.md" none none 0 "" "")" in
  *"$JD"*) fail "preamble: judgment-day was prescribed with no base to diff against" ;;
  *) pass "preamble: no computed base, no judgment-day range to prescribe" ;;
esac

# ── 2. a spec kit phase is not the task ─────────────────────────────────────
out="$(render "$TMP/build.md" speckit speckit 1 abc123 "")"
case "$out" in
  *"FIRST ACTION"*"$SK"*"[NEEDS CLARIFICATION]"*"Do not wait for a human"*) pass "preamble: --sdd speckit says a phase is not the task and that NEEDS CLARIFICATION is the executor's to resolve" ;;
  *) fail "preamble: --sdd speckit does not say where the task ends. got: $(printf '%s' "$out" | head -c 900)" ;;
esac
case "$(render "$TMP/build.md" none none 0 abc123 "")" in
  *"$SK"*) fail "preamble: the spec kit phase note leaked into a --sdd none dispatch" ;;
  *) pass "preamble: --sdd none carries no spec kit phase note" ;;
esac

# The re-task route builds its own entry line; it must carry the note too.
grep -q '\[ "\$next_sdd" != speckit \] || entry_line=.*_speckit_phase_note' "$SRC" \
  || fail "hw next: a --sdd speckit re-task does not carry the phase note"
pass "hw next: a --sdd speckit re-task appends the phase note to its entry line"

