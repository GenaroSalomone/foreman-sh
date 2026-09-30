#!/usr/bin/env bash
# A --sdd speckit dispatch runs /speckit-analyze in a fresh subagent between
# /speckit-tasks and /speckit-implement; a --sdd none dispatch is told nothing
# about it.
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process. Run it alone while working on this subject:
#
#     bash setup/tests/185-el-analyze-corre-en-contexto-limpio.sh
#
# THE PROMPT IS RENDERED, NOT GREPPED — the same preamble fragment and the same
# stubbed delivery edge as 166-el-cierre-es-judgment-day.sh. Then the same
# render is run against two mutants of bin/hw, and each must be caught: one
# with the analyze paragraph deleted, one with the phase note leaked into the
# --sdd none branch of the launch route. A check that passes against its mutant
# proves nothing.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SRC="${SUBJECT_HW:-$ROOT/bin/hw}"

extract() {  # $1 = hw source, $2 = fragment out
  python3 - "$1" "$2" <<'PY'
import sys
src = open(sys.argv[1], encoding="utf-8").read()
start = src.index("_specialist_agents() {\n")
end = src.index("\n# ONE DELIVERY PATH FOR BRIEFS AND RE-TASKS.", start)
open(sys.argv[2], "w", encoding="utf-8").write(src[start:end])
PY
}

# _speckit_phase_note lives outside the fragment; lift it in beside it.
lift_note() {  # $1 = hw source, $2 = fragment to append to
  python3 - "$1" "$2" <<'PY'
import sys
src = open(sys.argv[1], encoding="utf-8").read()
start = src.index("_speckit_phase_note() {\n")
end = src.index("\n}\n", start) + 3
open(sys.argv[2], "a", encoding="utf-8").write("\n" + src[start:end])
PY
}

printf -- '---\nkind: build\n---\n\n# t\n' > "$TMP/build.md"

# render <fragment> <sdd>
render() {
  G="$1" S="$2" B="$TMP/build.md" bash -c '
    set -euo pipefail
    BRIEF="$B"; BRIEF_TEXT="$(<"$BRIEF")"
    SDD="$S"; DISPATCH_FRAMEWORK="$S"
    if [ "$S" = speckit ]; then DISPATCH_IN_PLAY=1; else DISPATCH_IN_PLAY=0; fi
    TASK=probe; PROJ=setup; TASK_KIND=""
    HW_BIN_DIR=/fixture/bin; HW_WORKDIR=/fixture/work; ENGRAM_PROJECT=brain
    HW_REVIEW_BASE_REF=abc123; HW_REVIEW_BASE_BRANCH=main
    source "$G"
    info() { :; }; warn() { :; }; _specialist_agents() { :; }; lane_get() { :; }
    _framework_entry() { [ "$1" != speckit ] || printf "/speckit-specify"; }
    AGENT=claude
    _deliver_brief() { printf "%s" "$BRIEF_TEXT"; }
    _receipt_session() { :; }; _receipt_model() { :; }
    _send_brief fixture-pane
  '
}

has_gate() {  # $1 = rendered prompt; the gate, its position and its scope
  case "$1" in
    *"BETWEEN /speckit-tasks AND /speckit-implement, A FRESH /speckit-analyze."*"new subagent"*"only spec.md, plan.md, tasks.md"*"not this conversation"*"CRITICAL finding is corrected once"*"without re-running it"*) return 0 ;;
    *) return 1 ;;
  esac
}

# verdict <hw source> -> "speckit=<y|n> none=<y|n>"
verdict() {
  local frag="$TMP/frag.$RANDOM.sh" sk=n nn=n
  extract "$1" "$frag"
  if ! grep -q '^_speckit_phase_note() {' "$frag"; then lift_note "$1" "$frag"; fi
  has_gate "$(render "$frag" speckit)" && sk=y
  has_gate "$(render "$frag" none)" && nn=y
  printf 'speckit=%s none=%s' "$sk" "$nn"
}

# ── 1. the real binary ──────────────────────────────────────────────────────
v="$(verdict "$SRC")"
case "$v" in
  "speckit=y none=n") pass "preamble: --sdd speckit carries the fresh analyze gate after /speckit-tasks; --sdd none does not ($v)" ;;
  *) fail "preamble: the fresh analyze gate is not scoped to --sdd speckit ($v)" ;;
esac

# The re-task route appends the same note, so it carries the gate by
# construction — held by 166's grep on the entry line.
grep -q '\[ "\$next_sdd" != speckit \] || entry_line=.*_speckit_phase_note' "$SRC" \
  || fail "hw next: a --sdd speckit re-task does not append the phase note that carries the gate"
pass "hw next: a --sdd speckit re-task appends the phase note that carries the gate"

# ── 2. mutant: the paragraph deleted ────────────────────────────────────────
M1="$TMP/hw.no-analyze"
python3 - "$SRC" "$M1" <<'PY'
import re, sys
src = open(sys.argv[1], encoding="utf-8").read()
out, n = re.subn(r"\n\nBETWEEN /speckit-tasks AND /speckit-implement.*?does not stop the flow\.", "", src, count=1, flags=re.S)
assert n == 1, "mutant 1: anchor not found"
open(sys.argv[2], "w", encoding="utf-8").write(out)
PY
v="$(verdict "$M1")"
case "$v" in
  "speckit=n none=n") pass "mutant: with the analyze paragraph deleted, the speckit check fails ($v)" ;;
  *) fail "mutant: deleting the analyze paragraph went unnoticed ($v)" ;;
esac

# ── 3. mutant: the phase note leaked into the --sdd none branch ────────────
M2="$TMP/hw.leaked"
python3 - "$SRC" "$M2" <<'PY'
import sys
src = open(sys.argv[1], encoding="utf-8").read()
old = '    info "prompt states the flow is deliberately not entered'
assert src.count(old) == 1, "mutant 2: anchor not found exactly once"
new = '    BRIEF_TEXT="$BRIEF_TEXT$(_speckit_phase_note)"\n' + old
open(sys.argv[2], "w", encoding="utf-8").write(src.replace(old, new))
PY
v="$(verdict "$M2")"
case "$v" in
  *"none=y") pass "mutant: with the phase note leaked into --sdd none, the none check fails ($v)" ;;
  *) fail "mutant: the phase note leaking into --sdd none went unnoticed ($v)" ;;
esac
