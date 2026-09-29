#!/usr/bin/env bash
# MEASURED 2026-09-11, and found by accident. A comment was inserted into
# 124-an-entry-command-that-does-not-exist-is-refused.sh in the middle of a
# command substitution, leaving the file with an unterminated quote. It was
# committed, and `setup/test-budgets.json` received `0.05s` for it — a file that
# had been taking 2.08s. 0.05s is not a fast test; it is the duration of not
# running.
#
# suite-lane: exclusive — it drives a NESTED fast-gate runner, whose per-subject
#   budget check refuses on a busy machine (the same measurement as 117's, on
#   2026-09-16).
#
# TWO HOLES, AND THE SECOND IS THE ONE THAT MATTERS.
#
#   1. Neither harness asked whether a subject PARSES before running or
#      measuring it, so a broken file reached both.
#   2. measure-tests has always written a per-subject `exit` beside the seconds,
#      and NOTHING HAS EVER READ IT — not this runner, not 117, not anything.
#      A subject that failed while being measured was committed as its own
#      budget, and the manifest carried "this file does not pass" as a fact no
#      gate objected to. That is the durable defect; the typo was only its
#      occasion.
#
# WHAT IS NOT CLAIMED HERE. I read rc=0 from my own hand checks of the broken
# file at the time and could not reproduce it afterwards: the reconstructed file
# exits 2. So this file does not assert what a broken subject's exit status is,
# and neither gate below depends on one. `bash -n` is a verdict on the whole
# file in one pass; reading a status after the fact is the reckoning that failed.
#
# Run alone while working on this subject:
#     bash setup/tests/131-a-subject-that-does-not-parse-did-not-pass.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── a fixture that really is broken ─────────────────────────────────────────
broken="$TMP/broken-subject.sh"
{ printf '#!/usr/bin/env bash\n'
  printf 'printf "ok - a line that would look like evidence\\n"\n'
  printf 'f() {\n  out="$(echo unterminated\n}\n'
  printf 'printf "ok - and another\\n"\n'; } > "$broken"
chmod +x "$broken"
bash -n "$broken" 2>/dev/null && fail "gate: the fixture parses, so it is not a broken subject"
pass "gate: the fixture does not parse, which is the condition both gates must catch"

# A WHOLE setup/ IS THE FIXTURE, not a hand-picked list of helpers: test-hw
# reaches for its own snapshot tool, its coverage checker and the shared
# _common.sh, and a fixture that guesses which of those matter answers a
# different question every time one of them moves.
root="$TMP/fakeroot"; mkdir -p "$root/setup"
cp -R "$ROOT/setup/." "$root/setup/"
rm -f "$root/setup/tests"/[0-9]*.sh
cp "$broken" "$root/setup/tests/01-broken.sh"
budget='{"fast_gate_threshold_seconds": 2.0, "files": {"01-broken.sh": {"exit": 0, "seconds": 0.05}}}'
printf '%s\n' "$budget" > "$root/setup/test-budgets.json"

# ── the runner refuses it, before running it ────────────────────────────────
set +e
out="$(cd "$root" && HW_TEST_GATE=fast bash "$root/setup/test-hw" 2>&1)"; rc=$?
set -e
[ "$rc" -ne 0 ] || fail "the runner accepted a subject that does not parse: $out"
case "$out" in
  *"does not parse, so it was never run"*"NOT evidence"*) pass "the runner refuses an unparseable subject and says its assertions are not evidence" ;;
  *) fail "the runner's refusal does not name what is wrong: $out" ;;
esac
case "$out" in
  *"ok - a line that would look like evidence"*) fail "the runner ran the broken subject before judging it — the check must come FIRST: $out" ;;
esac
pass "the parse check runs BEFORE the subject, so no false ok- line reaches the tally"

# ── measure-tests refuses to write a budget for it ──────────────────────────
set +e
out="$(cd "$root" && bash "$root/setup/measure-tests.sh" 01-broken.sh 2>&1)"; rc=$?
set -e
[ "$rc" -ne 0 ] || fail "measure-tests measured a subject that does not parse: $out"
case "$out" in
  *"does not parse, so it cannot be measured"*"no budget was written"*) pass "measure-tests refuses an unparseable subject instead of timing it" ;;
  *) fail "measure-tests' refusal does not name what is wrong: $out" ;;
esac
[ "$(cat "$root/setup/test-budgets.json")" = "$budget" ] \
  || fail "measure-tests wrote a budget despite refusing — the manifest must record runs that happened"
pass "the budget manifest is unchanged by a refused measurement"

# ── the `exit` field is read, and a failed measurement is refused ───────────
# THIS IS THE DURABLE HALF. A subject can fail for reasons a parse check never
# sees — a missing fixture, a changed dependency, a real regression — and until
# today its failure could be committed as its own budget. Same manifest, one
# field changed, no broken syntax anywhere.
ok_subject="$root/setup/tests/02-fine.sh"
printf '#!/usr/bin/env bash\nprintf "ok - fine\\n"\n' > "$ok_subject"; chmod +x "$ok_subject"
rm -f "$root/setup/tests/01-broken.sh"
printf '{"fast_gate_threshold_seconds": 2.0, "files": {"02-fine.sh": {"exit": 1, "seconds": 0.42}}}\n' \
  > "$root/setup/test-budgets.json"
set +e
out="$(cd "$root" && HW_TEST_GATE=fast bash "$root/setup/test-hw" 2>&1)"; rc=$?
set -e
[ "$rc" -ne 0 ] || fail "the runner accepted a manifest whose budget came from a FAILED run: $out"
case "$out" in
  *"measured from a FAILED run is not a budget"*"02-fine.sh (exit 1, 0.42s)"*) pass "the runner reads the exit field, names the subject and what it recorded" ;;
  *) fail "the runner's refusal does not name the failed measurement: $out" ;;
esac
case "$out" in
  *"bash setup/measure-tests.sh 02-fine.sh"*) pass "and it names the exact command that fixes it" ;;
  *) fail "the refusal names no way out: $out" ;;
esac
printf '{"fast_gate_threshold_seconds": 2.0, "files": {"02-fine.sh": {"exit": 0, "seconds": 0.42}}}\n' \
  > "$root/setup/test-budgets.json"
set +e
out="$(cd "$root" && HW_TEST_GATE=fast bash "$root/setup/test-hw" 2>&1)"; rc=$?
set -e
[ "$rc" = 0 ] || fail "the same manifest with exit 0 was refused, so the gate is not keyed on the exit field: $out"
pass "the identical manifest passes with exit 0 — the exit field is what decided, not the seconds or the name"

# ── mutants ─────────────────────────────────────────────────────────────────
# M01 — drop the runner's exit-field check. The failed-measurement manifest must
# then be accepted, which is the 2026-09-11 behaviour: a field written and never
# read.
mut="$TMP/m01"; mkdir -p "$mut"; cp -R "$root/." "$mut/"
MUT="$mut/setup/test-hw" python3 - <<'PY'
import os
p = os.environ["MUT"]
s = open(p, encoding="utf-8").read()
start = s.index('broken = sorted(n for n in on_disk')
end = s.index('threshold = budgets["fast_gate_threshold_seconds"]', start)
open(p, "w", encoding="utf-8").write(s[:start] + s[end:])
PY
printf '{"fast_gate_threshold_seconds": 2.0, "files": {"02-fine.sh": {"exit": 1, "seconds": 0.42}}}\n' \
  > "$mut/setup/test-budgets.json"
set +e
out="$(cd "$mut" && HW_TEST_GATE=fast bash "$mut/setup/test-hw" 2>&1)"; rc=$?
set -e
if [ "$rc" = 0 ]; then
  pass "mutant killed: M01 without the exit-field check a budget measured from a failed run is accepted"
else
  fail "M01 survived or misfired: rc=$rc out=$out"
fi

# M02 — drop measure-tests' parse refusal. It must then time a file it cannot
# run and write that number as a budget, which is how 0.05s reached a commit.
mut2="$TMP/m02"; mkdir -p "$mut2"; cp -R "$root/." "$mut2/"
cp "$broken" "$mut2/setup/tests/01-broken.sh"
MUT="$mut2/setup/measure-tests.sh" python3 - <<'PY'
import os
p = os.environ["MUT"]
s = open(p, encoding="utf-8").read()
start = s.index('for f in "${TARGETS[@]}"; do\n  if ! parse_err=')
end = s.index('for f in "${TARGETS[@]}"; do\n  name=', start)
open(p, "w", encoding="utf-8").write(s[:start] + s[end:])
PY
set +e
out="$(cd "$mut2" && bash "$mut2/setup/measure-tests.sh" 01-broken.sh 2>&1)"; rc=$?
set -e
if grep -q '"01-broken.sh"' "$mut2/setup/test-budgets.json"; then
  pass "mutant killed: M02 without the refusal measure-tests writes a budget for a file it could not run"
else
  fail "M02 survived or misfired: rc=$rc out=$out"
fi
