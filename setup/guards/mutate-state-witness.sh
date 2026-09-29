#!/usr/bin/env bash
# Mutate a COPY of bin/, never the live tree.
#
# WHY THIS ARM EXISTS. Verde no es cobertura. 67 new assertions went green the
# first time they ran, and a green assertion proves only that the code and the
# expectation agree — not that the assertion would notice if the code changed.
# The claims being defended here are the kind that fail silently: a verdict that
# quietly turns "I could not ask" into "nothing is happening" reads exactly like
# the fix and behaves exactly like the bug.
#
# HOW IT ISOLATES. Each subject test file derives its own $ROOT from
# $BASH_SOURCE, so running the copy at $WORK/setup/tests/<f> makes $WORK the
# root and $WORK/bin the binaries under test. Nothing reaches ~/brain.
set -euo pipefail
ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ART="${HW_ARTIFACTS:-${TMPDIR:-/tmp}}"
WORK="$ART/state-witness-mutants"
rm -rf "$WORK"; mkdir -p "$WORK/setup"
cp -R "$ROOT/bin" "$WORK/bin"
cp -R "$ROOT/setup/tests" "$WORK/setup/tests"
cp -R "$ROOT/setup/guards" "$WORK/setup/guards"
cp "$ROOT/setup/test-hw" "$ROOT/setup/test-channel-send" "$WORK/setup/"
PRISTINE="$WORK/pristine"; mkdir -p "$PRISTINE"
cp "$ROOT/bin/state-witness.sh" "$ROOT/bin/hw" "$ROOT/bin/channel-send" "$PRISTINE/"

killed=0; survived=0

# BASELINE FIRST. A mutant "killed" by a test file that was already red proves
# nothing at all, so every subject is run unmutated before anything is mutated.
for subject in 30-state-witness.sh 22-hw-unstick.sh 12-hw-next.sh; do
  if bash "$WORK/setup/tests/$subject" >"$WORK/baseline-$subject.txt" 2>&1; then
    printf 'ok - baseline %s is green before mutation\n' "$subject"
  else
    printf 'not ok - baseline %s is ALREADY RED, so no mutant it kills would mean anything\n' "$subject"
    exit 1
  fi
done

# mutant <name> <bin-file> <subject-test> <from> <to>
mutant() {
  local name="$1" binfile="$2" subject="$3" from="$4" to="$5"
  cp "$PRISTINE/$binfile" "$WORK/bin/$binfile"
  python3 - "$WORK/bin/$binfile" "$from" "$to" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text(); old, new = sys.argv[2:]
n = s.count(old)
if n != 1:
    raise SystemExit("expected exactly one mutation site, found %d" % n)
p.write_text(s.replace(old, new, 1))
PY
  chmod +x "$WORK/bin/$binfile"
  if bash "$WORK/setup/tests/$subject" >"$WORK/$name.txt" 2>&1; then
    printf 'not ok - mutant %s SURVIVED %s\n' "$name" "$subject"; survived=$((survived + 1))
  else
    printf 'ok - mutant %s killed by %s\n' "$name" "$subject"; killed=$((killed + 1))
  fi
  cp "$PRISTINE/$binfile" "$WORK/bin/$binfile"; chmod +x "$WORK/bin/$binfile"
}

# ── THE VERDICT'S ASYMMETRY ─────────────────────────────────────────────────
# `unknown` must never collapse into an answer. These four are the fix becoming
# the bug it fixes, and each is a one-line edit somebody could make believing it
# was a simplification.
mutant unreadable-screen-becomes-settled state-witness.sh 30-state-witness.sh \
  '  if [ "$screen_state" = unreadable ]; then
    WITNESS_VERDICT=unknown' \
  '  if [ "$screen_state" = unreadable ]; then
    WITNESS_VERDICT=settled'
mutant absent-endpoint-becomes-settled state-witness.sh 30-state-witness.sh \
  '  if [ -z "$endpoint" ]; then
    WITNESS_VERDICT=unknown' \
  '  if [ -z "$endpoint" ]; then
    WITNESS_VERDICT=settled'
mutant unaskable-endpoint-becomes-settled state-witness.sh 30-state-witness.sh \
  '  if [ -z "$prompts" ]; then
    WITNESS_VERDICT=unknown' \
  '  if [ -z "$prompts" ]; then
    WITNESS_VERDICT=settled'
# `unknown` reaching the exit-5 path at all. This is the fix becoming the bug.
mutant unknown-treated-as-settled state-witness.sh 30-state-witness.sh \
  '    case "$WITNESS_VERDICT" in
      settled)
        streak=$((streak + 1))' \
  '    case "$WITNESS_VERDICT" in
      settled|unknown)
        streak=$((streak + 1))'
# The streak never resetting, so two verdicts that were never consecutive add up.
mutant streak-never-resets state-witness.sh 30-state-witness.sh \
  '      *) streak=0 ;;
    esac
  done
}' \
  '      *) : ;;
    esac
  done
}'

# ── THE READERS ─────────────────────────────────────────────────────────────
# A failed read must not be indistinguishable from a successful one.
mutant screen-read-failure-is-a-hash state-witness.sh 30-state-witness.sh \
  "  printf '%s' \"\$out\" | jq -e 'has(\"read\") and (.read | has(\"text\")) and (.read.text | type == \"string\")' \\
    >/dev/null 2>&1 || return 1" \
  '  : # any payload will do'
# BOTH LINES, because either one alone is MASKED by the other: the two shape
# checks are redundant on purpose (defence in depth), so mutating one still
# leaves the other returning 1 and the verdict still comes out `unknown`. A
# masked mutant surviving says nothing about the tests, only about the mutant —
# so the mutant has to remove the whole guard.
_ps_from="  qn=\"\$(printf '%s' \"\$q\" | jq -e 'if type == \"array\" then length else empty end' 2>/dev/null)\" || return 1
  pn=\"\$(printf '%s' \"\$p\" | jq -e 'if type == \"array\" then length else empty end' 2>/dev/null)\" || return 1"
_ps_to="  qn=\"\$(printf '%s' \"\$q\" | jq -r 'length // 0' 2>/dev/null || printf 0)\"
  pn=\"\$(printf '%s' \"\$p\" | jq -r 'length // 0' 2>/dev/null || printf 0)\""
mutant prompts-shape-not-checked state-witness.sh 30-state-witness.sh "$_ps_from" "$_ps_to"

# ── LIVENESS AND THE HUMAN ──────────────────────────────────────────────────
mutant moving-screen-read-as-static state-witness.sh 30-state-witness.sh \
  '    if [ "$cur" != "$first" ]; then printf '"'"'moved %s'"'"' "$first"; return 0; fi' \
  '    if [ "$cur" = "$first" ]; then printf '"'"'moved %s'"'"' "$first"; return 0; fi'
mutant outstanding-prompt-ignored state-witness.sh 30-state-witness.sh \
  '    if [ "$qn" -gt 0 ] || [ "$pn" -gt 0 ]; then' \
  '    if [ "$qn" -gt 99 ] || [ "$pn" -gt 99 ]; then'
mutant absolute-path-opencode-unmatched state-witness.sh 30-state-witness.sh \
  '                   and (. == "opencode" or endswith("/opencode")))) | .argv) as $argv' \
  '                   and (. == "opencode"))) | .argv) as $argv'

# ── THE GATE ────────────────────────────────────────────────────────────────
mutant one-confirmation-is-enough state-witness.sh 30-state-witness.sh \
  ': "${WITNESS_CONFIRMATIONS:=2}"' \
  ': "${WITNESS_CONFIRMATIONS:=1}"'
mutant transport-errors-retried-in-a-hot-loop state-witness.sh 30-state-witness.sh \
  '      0) return 0 ;;
      2) ;;
      *) return "$rc" ;;' \
  '      0) return 0 ;;
      *) ;;'
mutant settled-never-cuts-the-wait state-witness.sh 30-state-witness.sh \
  '        [ "$streak" -lt "$WITNESS_CONFIRMATIONS" ] || return 5' \
  '        [ "$streak" -lt "$WITNESS_CONFIRMATIONS" ] || :'

# ── hw next --dry-run MUST NOT ENTER THE GATE ───────────────────────────────
# Unreachable dry-run block: execution falls through to the delivery gate, which
# is precisely the measured exit-144 shape.
mutant dry-run-falls-through-to-the-gate hw 12-hw-next.sh \
  '  if [ "$dry" = 1 ]; then
    local nextseq_dry' \
  '  if [ "$dry" = 2 ]; then
    local nextseq_dry'
# The gate itself, gone: hw next would wait with the raw wait again.
mutant hw-next-loses-its-cross-check hw 12-hw-next.sh \
  '  witness_wait "$pane" "$wait_ms" || gate_rc=$?' \
  '  invoker_wait_for_brainer "$pane" "$wait_ms" || gate_rc=$?'

# ── hw unstick MUST NOT ACCEPT A SIGNATURE A HEALTHY PANE CAN PUBLISH ───────
mutant unstick-accepts-a-working-pane hw 22-hw-unstick.sh \
  '  case "$WITNESS_VERDICT" in
    settled) info "cross-check CONFIRMS stuck: $WITNESS_DETAIL" ;;' \
  '  case "$WITNESS_VERDICT" in
    settled|live) info "cross-check CONFIRMS stuck: $WITNESS_DETAIL" ;;'
mutant unstick-accepts-an-unavailable-cross-check hw 22-hw-unstick.sh \
  '    *)
      die "$pane publishes blocked_reason=stuck, but that could NOT be corroborated' \
  '    *)
      info "proceeding anyway: $pane publishes blocked_reason=stuck, but that could NOT be corroborated'
mutant unstick-drops-the-pre-stop-recheck hw 22-hw-unstick.sh \
  '  witness_state "$pane"
  case "$WITNESS_VERDICT" in
    settled) : ;;
    live)
      die "$pane started WORKING between the first cross-check and the restart' \
  '  WITNESS_VERDICT=settled
  case "$WITNESS_VERDICT" in
    settled) : ;;
    live)
      die "$pane started WORKING between the first cross-check and the restart'

printf '\n%d killed, %d survived\n' "$killed" "$survived"
[ "$survived" -eq 0 ]
