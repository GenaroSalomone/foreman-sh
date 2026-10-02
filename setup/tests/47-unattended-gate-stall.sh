#!/usr/bin/env bash
# An outside-flow dispatch cannot be captured by a loaded framework gate, and
# every executor knows that its pane has no interactive human answerer.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SOURCE="${UNATTENDED_GATE_HW_SOURCE:-$ROOT/bin/hw}"
MUTANTS="$TMP/mutations-unattended-gate-stall"
mkdir -p "$MUTANTS"

# Extract and execute the real _send_brief implementation. Delivery and receipt
# are stubbed only after extraction, so each assertion reads the assembled first
# prompt rather than matching source text in bin/hw.
extract_send_brief() {
  local source="$1" fragment="$2"
  python3 - "$source" "$fragment" <<'PY'
import sys
src = open(sys.argv[1], encoding="utf-8").read()
# ANCHORED AT _specialist_agents, not at _send_brief: since 2026-09-21 the
# preamble and the safeguard live in _brief_preamble/_brief_epistemic so
# `hw next` can carry them too, and _send_brief alone no longer assembles a
# prompt. All three, plus the helper _brief_preamble reads, share one anchor.
start = src.index("_specialist_agents() {\n")
end = src.index("\n# ONE DELIVERY PATH FOR BRIEFS AND RE-TASKS.", start)
open(sys.argv[2], "w", encoding="utf-8").write(src[start:end])
PY
}

render() {
  local source="$1" sdd="$2" framework="$3" in_play="$4" output="$5"
  local fragment="$TMP/send-$(basename "$output").sh"
  extract_send_brief "$source" "$fragment"
  OUT="$output" FRAGMENT="$fragment" SDD_VALUE="$sdd" FRAMEWORK="$framework" \
    IN_PLAY="$in_play" bash -c '
      set -euo pipefail
      BRIEF="<fixture brief>"; BRIEF_TEXT="# unattended fixture"
      SDD="$SDD_VALUE"; DISPATCH_FRAMEWORK="$FRAMEWORK"; DISPATCH_IN_PLAY="$IN_PLAY"
      TASK=unattended-probe; PROJ=fixture-lane
      HW_BIN_DIR=/fixture/bin; HW_WORKDIR=/fixture/work; ENGRAM_PROJECT=brain
      source "$FRAGMENT"
      info() { :; }
      warn() { :; }
      _specialist_agents() { :; }
      lane_get() { :; }
      # `_send_brief` no longer checks the entry command against anything on
      # disk — `_framework_entry` just prints the fixed skill name for its
      # mode — so this fixture is about the brief TEXT alone, and the entry
      # command is stubbed to a stand-in rather than the real /speckit-specify.
      _framework_entry() { printf "/sdd-new"; }
      AGENT=claude
      _deliver_brief() { PROMPT="$BRIEF_TEXT"; }
      _receipt_session() { :; }; _receipt_model() { :; }
      _send_brief fixture-pane
      printf "%s" "$PROMPT" > "$OUT"
    '
}

contains() { case "$(<"$1")" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

assert_surface_excluded() {
  contains "$1" 'This task runs outside the framework flow' &&
    contains "$1" 'dispatched with' &&
    contains "$1" '--sdd none, so the brief is the whole contract' &&
    contains "$1" 'leave their phase commands, review or verify flows' &&
    contains "$1" 'preflights unused here' &&
    contains "$1" 'a skill that asks for a blocking' &&
    contains "$1" 'preflight does not' &&
    contains "$1" 'apply to this task'
}

assert_unattended() {
  contains "$1" 'This pane is unattended: no human reads it' &&
    contains "$1" 'interactive question tool has' &&
    contains "$1" 'no answerer here' &&
    contains "$1" 'ask-invoker is the only human channel' &&
    contains "$1" 'missing tool or runtime limitation is a' &&
    contains "$1" 'fact to report with done-invoker --blocked, never a reason to end the turn silently'
}

BASE="$MUTANTS/base-hw"
cp "$SOURCE" "$BASE"

SPECKIT="$TMP/speckit.prompt"
SPECKIT_FLOW="$TMP/speckit-flow.prompt"
UNKNOWN="$TMP/unknown-none.prompt"
render "$BASE" none speckit 0 "$SPECKIT"
render "$BASE" speckit speckit 1 "$SPECKIT_FLOW"
render "$BASE" none '<undetermined>' 0 "$UNKNOWN"

assert_surface_excluded "$SPECKIT" \
  || fail "A01 outside-flow prompt did not exclude phase, review/verify and orchestrator-preflight surfaces"
pass "A01 --sdd none excludes the whole loaded framework surface and rejects its blocking preflight"

for prompt in "$SPECKIT" "$SPECKIT_FLOW"; do
  assert_unattended "$prompt" \
    || fail "B01 unattended-pane rule was absent from one dispatch mode"
done
pass "B01 every dispatch says the pane is unattended, ask-invoker is the channel and limitations are reported"

contains "$UNKNOWN" 'This task runs outside the framework flow' &&
  contains "$UNKNOWN" 'mode' &&
  contains "$UNKNOWN" 'could not be determined' &&
  contains "$UNKNOWN" 'preflights unused here' &&
  ! contains "$UNKNOWN" 'in <undetermined> mode' \
  || fail "C01 undetermined framework suppressed or falsely named the outside-flow statement"
pass "C01 --sdd none still excludes the flow when framework mode is undetermined without naming a mode"

mutate() {
  local name="$1" old="$2" new="$3" output
  output="$MUTANTS/$name"
  cp "$BASE" "$output"
  python3 - "$output" "$old" "$new" <<'PY'
import sys
path, old, new = sys.argv[1:]
src = open(path, encoding="utf-8").read()
if src.count(old) != 1:
    raise SystemExit("mutation anchor count is not one: %r" % old[:90])
open(path, "w", encoding="utf-8").write(src.replace(old, new))
PY
  printf '%s' "$output"
}

# One ungated mutation per behavioural claim; setup/test-hw verifies each
# declared arm emitted its kill marker during the full run.
mutant="$(mutate surface-breadth 'review or verify flows and\npreflights unused here' 'review flows and\npreflights unused here')"
render "$mutant" none speckit 0 "$TMP/mutant-surface.prompt"
assert_surface_excluded "$TMP/mutant-surface.prompt" && fail "M01 SURVIVED: surface-breadth"
pass "mutant killed: M01 narrowing the outside-flow prohibition drops review/verify or preflight coverage"

mutant="$(mutate unattended-channel 'ask-invoker is the only human channel.' 'No human channel is named.')"
render "$mutant" none speckit 0 "$TMP/mutant-unattended.prompt"
assert_unattended "$TMP/mutant-unattended.prompt" && fail "M02 SURVIVED: unattended-channel"
pass "mutant killed: M02 removing ask-invoker from the unattended rule leaves the pane without a human channel"

mutant="$(mutate undetermined-branch 'The directory framework mode\ncould not be determined' 'The directory framework mode\nis hidden')"
render "$mutant" none '<undetermined>' 0 "$TMP/mutant-undetermined.prompt"
contains "$TMP/mutant-undetermined.prompt" 'could not be determined' \
  && fail "M03 SURVIVED: undetermined-branch"
pass "mutant killed: M03 hiding the undetermined-mode branch removes the mode-safe outside-flow statement"

printf 'mapping - A01↔M01 surface breadth · B01↔M02 unattended channel · C01↔M03 undetermined branch\n'
printf 'coverage - 3 behavior claims, 3 dedicated mutants killed\n'
