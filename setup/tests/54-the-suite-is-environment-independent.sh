#!/usr/bin/env bash
# A SUBJECT WHOSE RESULT CHANGES WITH THE CALLER'S ENVIRONMENT IS NOT
# MEASURING THE TREE.
#
# THE CLAIM, and it is bigger than any list of variable names: `./test-hw` must
# give the same answer to a brainer and to an executor. A green that holds only
# for whoever happened to run it is not a green — the same disease as a green
# that held only in one working tree (0fc237f, 2026-09-02).
#
# FOUR INSTANCES, all the same shape — the first three in one day:
#   · HW_INVOKER_PANE      — hw honours an exported pane ahead of every
#                            heuristic, so the suite passed for a brainer and
#                            died for every executor.
#   · HW_CHAINING_ENABLED  — read environment-first, so 23's fixture was
#                            overridden by whatever the running pane exported.
#   · HW_CHAINING_LEASE_*  — measured 2026-09-02. `hw` exports FOUR chaining
#                            variables into a `--keep-pane` executor and
#                            _common.sh named ONE. With the lease variable still
#                            set, `done-invoker` starts a bounded lease by
#                            calling `hw chaining-lease-start`, and
#                            24-done-closes-delivered.sh — which asserts that
#                            chaining invokes `hw` for NOTHING, and is right to —
#                            failed. Result: 1140 ok / exit 0 for a brainer,
#                            abort at 406 ok for a chained executor, same tree.
#                            A second-vendor reviewer returned NO SHIP for 24 by
#                            the same mechanism, and a five-commit bisect
#                            measured the runner's environment instead of any
#                            commit.
#
#   · A SECOND RUNNER, and the fourth instance — measured 2026-09-08.
#                            `setup/test-channel-send` is not globbed by
#                            ../test-hw; pre-commit runs it separately whenever a
#                            commit touches channel-send or an invoker. It had
#                            the ENUMERATION this file exists to have replaced —
#                            `unset HW_CHAINING_ENABLED HW_DONE_KEEP_PANE`, two
#                            of the four chaining names. With
#                            HW_CHAINING_LEASE_SECONDS still set, done-invoker's
#                            chaining branch takes CHAINING_LEASE=1, which owes a
#                            lease and therefore needs `hw`, so its "chaining
#                            reports with no hw at all" case REFUSED — correctly.
#                            38 ok for a brainer, `not ok` for every executor,
#                            same tree. It was found when an executor's own
#                            commit was blocked by it, and the red looked like a
#                            regression in that executor's work.
#
# So the first two were fixed by adding a name, the third happened anyway, and
# the fourth happened in the file nobody had swept.
# This subject exists because the missing name was never the defect: enumerating
# what to REMOVE has to track a file in another directory that other agents
# edit. _common.sh now sweeps the whole `HW_*` prefix and keeps a short list of
# names this directory OWNS, which is the same list inverted — and this file is
# what fails if that inversion is ever undone.
#
# HOW IT PROVES IT, and it is the method every claim today rested on: run the
# same thing twice, once under a deliberately polluted environment, and require
# the two to be IDENTICAL. Not "polluted still passes" — identical, because a
# subject that passes for a different reason has still stopped measuring the
# tree.
#
# THE POLLUTION INCLUDES A NAME NOBODY HAS INVENTED. `HW_CHAINING_FUTURE_KNOB`
# is what separates this from a fourth round of the same bug: an allowlist
# cannot know it, a prefix sweep does not need to.
#
# Run it alone while working on this subject:
#
#     bash setup/tests/54-the-suite-is-environment-independent.sh
#
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

CLAIMS=0
claim() { CLAIMS=$((CLAIMS + 1)); pass "$1"; }

TESTS_DIR="$ROOT/setup/tests"
COMMON="$TESTS_DIR/_common.sh"

# EXACTLY WHAT A CHAINED EXECUTOR CARRIES, taken from hw's own run env, plus one
# invented name. Kept as a function rather than a list of `-u` flags because it
# is used both to pollute and to name what pollution means.
pollute() { # pollute <command...>
  env HW_CHAINING_ENABLED=1 \
      HW_CHAINING_LEASE_SECONDS=14400 \
      HW_CHAINING_LEASE_REASON='a lease reason from the running pane' \
      HW_CHAINING_LEASE_HOLDER=wPOLLUTE:pB \
      HW_CHAINING_FUTURE_KNOB=1 \
      HW_DONE_KEEP_PANE=1 \
      HW_INVOKER_PANE=wPOLLUTE:pB \
      HW_WORKDIR=/nonexistent/pollution \
      HW_RUN=pollution-run \
      HW_PROJECT=pollution \
      HW_TASK=pollution \
      HW_ARTIFACTS=/nonexistent/pollution/artifacts \
      HW_NEXT_WAIT_MS=1 \
      HW_INVOKER_WAIT_MS=1 \
      ENGRAM_PROJECT=pollution \
      AGENT_BROWSER_NAMESPACE=hw-pollution \
      "$@"
}
# The brainer-shaped side: none of the above present at all.
clean() { # clean <command...>
  env -u HW_CHAINING_ENABLED -u HW_CHAINING_LEASE_SECONDS \
      -u HW_CHAINING_LEASE_REASON -u HW_CHAINING_LEASE_HOLDER \
      -u HW_CHAINING_FUTURE_KNOB -u HW_DONE_KEEP_PANE -u HW_INVOKER_PANE \
      -u HW_WORKDIR -u HW_RUN -u HW_PROJECT -u HW_TASK -u HW_ARTIFACTS \
      -u HW_NEXT_WAIT_MS -u HW_INVOKER_WAIT_MS \
      -u ENGRAM_PROJECT -u AGENT_BROWSER_NAMESPACE \
      "$@"
}

# ── C01. the sweep reaches every member of the family, invented ones included ─
#
# Driven through a probe that sources the REAL _common.sh the way a subject
# does, then reports what survived. This is the direct claim about the harness;
# the subject-level claims below are the consequence.
cat > "$TMP/probe.sh" <<PROBE
#!/usr/bin/env bash
. "$COMMON"
for n in HW_CHAINING_ENABLED HW_CHAINING_LEASE_SECONDS HW_CHAINING_LEASE_REASON \\
         HW_CHAINING_LEASE_HOLDER HW_CHAINING_FUTURE_KNOB HW_DONE_KEEP_PANE \\
         HW_INVOKER_PANE HW_WORKDIR HW_RUN HW_PROJECT HW_TASK HW_ARTIFACTS \\
         ENGRAM_PROJECT AGENT_BROWSER_NAMESPACE; do
  eval "v=\\\${\$n:-<swept>}"
  printf '%s=%s\n' "\$n" "\$v"
done
printf 'KEPT_UNSTICK=%s\n' "\${HW_UNSTICK_BIN:-<swept>}"
printf 'KEPT_SOURCE=%s\n' "\${HW_SOURCE:-<swept>}"
printf 'KEPT_DRIFT=%s\n' "\${HW_CHECK_LIVE_REVIEW_DRIFT:-<swept>}"
printf 'KEPT_BINDING=%s\n' "\${HW_CHECK_LIVE_REVIEW_BINDING:-<swept>}"
PROBE

PROBE_OUT="$(pollute HW_UNSTICK_BIN=/keep/unstick HW_SOURCE=/keep/source \
              HW_CHECK_LIVE_REVIEW_DRIFT=1 HW_CHECK_LIVE_REVIEW_BINDING=1 \
              bash "$TMP/probe.sh" 2>&1)"
survivors="$(printf '%s\n' "$PROBE_OUT" | grep -v '=<swept>$' | grep -v '^KEPT_' || true)"
[ -z "$survivors" ] || fail "C01: pane state survived _common.sh: $(printf '%s' "$survivors" | tr '\n' ' ')"
claim "C01 every HW_* the pane exported is swept, including an invented HW_CHAINING_FUTURE_KNOB"

# ── C02. and the sweep does NOT eat the harness's own inputs ────────────────
#
# NOT A NICETY. HW_CHECK_LIVE_REVIEW_DRIFT and HW_CHECK_LIVE_REVIEW_BINDING gate
# an OPTIONAL LIVE ARM in 31 and 36. Sweeping them would silently turn a
# deliberate opt-in off — "an arm nobody runs is not coverage", which is the
# failure setup/mutation-coverage was written for. A fix that caused that would
# be worse than the bug it fixed.
for k in "KEPT_UNSTICK=/keep/unstick" "KEPT_SOURCE=/keep/source" \
         "KEPT_DRIFT=1" "KEPT_BINDING=1"; do
  case "$PROBE_OUT" in
    *"$k"*) : ;;
    *) fail "C02: the sweep ate a deliberate harness input ($k): $PROBE_OUT" ;;
  esac
done
claim "C02 the four names this directory owns as inputs survive, so no live arm is silently disabled"

# ── THE SIX LONG RUNS START HERE, TOGETHER ─────────────────────────────────
#
# C03, C04 and C05 each drive a real subject or runner twice, once per
# environment, and every one of those six runs owns its mktemp directory and
# binds its own port — none reads another. One after another they were most of
# this file's 76s (measured 2026-09-23); launched together they cost the
# longest of them. Each lands in $BG/<name> with its exit status in
# $BG/<name>.rc, and is judged below in the original order, unchanged.
BG="$TMP/bg"; mkdir -p "$BG"
bg_run() { # bg_run <name> <normaliser> <env wrapper> <command...>
  local _n="$1" _norm="$2" _wrap="$3"; shift 3
  # EACH RUN GETS ITS OWN TMPDIR, because the invokers these subjects drive take
  # a lock at $TMPDIR/hw-invoker-<HW_RUN>.lock, and 24 names every case
  # HW_RUN=run, a name other subjects in the same pool also use. On a shared
  # TMPDIR the two runs of 24 here, or 24 and any other holder, serialise on one
  # lock, and a waiter that reads the holder's pid just before a clean release
  # prints "breaking an invoker lock whose holder is gone" into the case's
  # output. Measured 2026-09-23: that line lands first in close-failed's out and
  # 24's `head -1` assertion goes red, so this subject reported an environment
  # difference that was lock traffic. TMPDIR is not pane state, it is identical
  # in shape on both sides, and tmp_norm/cs_norm already erase the paths under it.
  mkdir -p "$BG/$_n.tmp"
  # The status is written from INSIDE the pipeline's left side (a subshell of
  # its own) to a part file, and renamed once the normaliser has finished, so
  # bg_get never reads a status whose output is still being written.
  ( { _rc=0; "$_wrap" env TMPDIR="$BG/$_n.tmp" "$@" 2>&1 || _rc=$?; printf '%s' "$_rc" > "$BG/$_n.rc.part"; } | "$_norm" > "$BG/$_n"
    mv -f "$BG/$_n.rc.part" "$BG/$_n.rc" ) &
}
bg_get() { # bg_get <name> — its output; a run that exited non-zero fails here
  while [ ! -f "$BG/$1.rc" ]; do sleep 0.2; done
  [ "$(cat "$BG/$1.rc")" = 0 ] || fail "$1 exited $(cat "$BG/$1.rc"), so its output is not an answer: $(tail -5 "$BG/$1")"
  cat "$BG/$1"
}
tmp_norm() { sed 's|/[^ ]*hw-test\.[A-Za-z0-9]*|<TMP>|g'; }
# FOUR things legitimately differ between two runs of the second runner, and
# the fourth is why this normalisation is not cosmetic: the mktemp path, the
# port the stub receiver binds, the ids channel-send mints, and a MEASURED
# ELAPSED TIME that one subject puts inside its own assertion text ("refused in
# 3s" against "refused in 4s"). 16-status.sh calls out the same shape in this
# suite. Time is not environment, so it is normalised rather than allowed to
# fail here. `|` inside a sed -E group needs no escaping, but it IS the
# delimiter of the first expression, so the alternation gets its own with a
# different one.
cs_norm() {
  sed -E 's|/[^ ]*channel-send-test\.[A-Za-z0-9]*|<TMP>|g; s|127\.0\.0\.1:[0-9]+|127.0.0.1:<PORT>|g' \
    | sed -E 's#(msg|ses|thread)_[A-Za-z0-9]+#\1_<ID>#g' \
    | sed -E 's#in [0-9]+s rather than#in <N>s rather than#g'
}
CS_RUNNER="$ROOT/setup/test-channel-send"
for _s in 24-done-closes-delivered.sh 23-invoker-env-file.sh; do
  bg_run "${_s%%-*}-clean"   tmp_norm clean   bash "$TESTS_DIR/$_s"
  bg_run "${_s%%-*}-polluted" tmp_norm pollute bash "$TESTS_DIR/$_s"
done
if [ -x "$CS_RUNNER" ]; then
  bg_run cs-clean    cs_norm clean   bash "$CS_RUNNER"
  bg_run cs-polluted cs_norm pollute bash "$CS_RUNNER"
fi

# ── C03/C04. real subjects give the SAME answer either way ──────────────────
#
# The claim is identity, not success. Two subjects are driven: the one the
# incident was found in, and the one whose header documents the previous
# instance of it. Output is compared byte for byte after stripping the one thing
# that legitimately differs between two runs — the mktemp path each run gets.
#
# WHY NOT THE WHOLE SUITE: it takes minutes and would make this subject the
# slowest thing in the run. These two carry the two documented instances, and
# C01 above is what generalises to the family.
same_both_ways() { # <subject file> <label>
  local subj="$1" label="$2" a b
  a="$(bg_get "${subj%%-*}-clean")"
  b="$(bg_get "${subj%%-*}-polluted")"
  if [ "$a" = "$b" ]; then
    claim "C0x $label gives byte-identical output to a brainer and to a chained executor"
    return 0
  fi
  fail "C0x $label DIFFERS by environment — clean vs polluted: $(diff <(printf '%s\n' "$a") <(printf '%s\n' "$b") | head -6 | tr '\n' ' ')"
}
same_both_ways 24-done-closes-delivered.sh "24 (where the lease leak was found)"
same_both_ways 23-invoker-env-file.sh       "23 (where the previous instance was found)"

# ── C05. AND THE OTHER RUNNER, because the family is not one directory ──────
#
# `setup/test-channel-send` is a second runner: ../test-hw does not glob it, and
# pre-commit invokes it on its own whenever a commit touches the delivery path.
# It carries the same contract and had none of the sweep, which is the fourth
# instance in this file's header. Driven whole rather than sampled — it takes
# ~13s, and it is the only thing that exercises the delivery contract.
if [ -x "$CS_RUNNER" ]; then
  # (its normaliser, cs_norm, is defined with the launch above)
  cs_a="$(bg_get cs-clean)"
  cs_b="$(bg_get cs-polluted)"
  if [ "$cs_a" = "$cs_b" ]; then
    claim "C05 setup/test-channel-send gives identical output to a brainer and to a chained executor"
  else
    fail "C05 setup/test-channel-send DIFFERS by environment — clean vs polluted: $(diff <(printf '%s\n' "$cs_a") <(printf '%s\n' "$cs_b") | head -6 | tr '\n' ' ')"
  fi
  # A TALLY FLOOR, NOT THE ABSENCE OF "not ok". Judgment Day caught this on
  # 2026-09-08: a runner that died before its first assertion produces empty
  # output for BOTH runs — identical, and free of "not ok" — so both C05 claims
  # passed on nothing. That is the 8f5c8ef shape, and it is the shape the M04
  # arm below was rewritten to remove IN THE SAME COMMIT that added this one.
  # The floor is deliberately below the current count (measured 38): it exists
  # to catch a runner that did not run, not to pin a number that legitimately
  # grows.
  cs_b_ok="$(printf '%s\n' "$cs_b" | grep -c '^ok ' || true)"
  cs_a_ok="$(printf '%s\n' "$cs_a" | grep -c '^ok ' || true)"
  case "$cs_b" in
    *"not ok"*) fail "C05 setup/test-channel-send is red for a chained executor: $(printf '%s' "$cs_b" | grep 'not ok' | head -2)" ;;
  esac
  if [ "${cs_b_ok:-0}" -ge 20 ] && [ "${cs_a_ok:-0}" -ge 20 ]; then
    pass "C05 and it is GREEN for the chained executor by its own tally ($cs_a_ok ok clean, $cs_b_ok ok polluted), not identically empty"
  else
    fail "C05 setup/test-channel-send reported only $cs_a_ok/$cs_b_ok passing assertions — it did not run, so its identical output proves nothing"
  fi
else
  fail "C05 setup/test-channel-send is not executable, so the second runner's contract is unchecked"
fi

# ── MUTANTS ─────────────────────────────────────────────────────────────────
#
# Killed by text ONLY THE MUTANT PRODUCES. The mutation target is _common.sh
# itself, so each mutant is a COPY of the tests directory with a patched
# _common.sh — the real one is never touched, which is the rule the herdr-rpc
# incident produced: a fixture goes in a temp dir, never over the file.
mutate_common() { # <name> <old> <new>
  # Split: bash 3.2 does not make `name` visible to a later assignment in the
  # SAME `local` statement, so the one-liner dies under set -u.
  local name="$1"
  local dir="$TMP/mutant-$name"
  mkdir -p "$dir"
  cp "$TESTS_DIR"/*.sh "$dir/"
  MUT_OLD="$2" MUT_NEW="$3" python3 - "$dir/_common.sh" <<'PY'
import os, sys
p = sys.argv[1]; s = open(p, encoding="utf-8").read()
old = os.environ["MUT_OLD"]; new = os.environ["MUT_NEW"]
n = s.count(old)
if n != 1:
    raise SystemExit("mutation anchor count %d, expected 1: %r" % (n, old[:90]))
open(p, "w", encoding="utf-8").write(s.replace(old, new))
PY
  MUTANT_DIR="$dir"
}

# M01 — the prefix sweep reverted to the three-name allowlist that shipped this
# morning. This is the pre-fix state exactly, and the mutant announces itself so
# a run that never reached the line is VACUOUS rather than dead.
mutate_common M01 \
  'for _v in ${!HW_@}; do' \
  'printf "M01-ALLOWLIST-RESTORED\n" >&2; unset HW_INVOKER_PANE HW_CHAINING_ENABLED HW_DONE_KEEP_PANE 2>/dev/null || true; for _v in ; do'
# The probe is re-pointed at the MUTANT's _common.sh, never the real one.
sed "s|$COMMON|$MUTANT_DIR/_common.sh|" "$TMP/probe.sh" > "$MUTANT_DIR/probe-m01.sh"
out="$(pollute bash "$MUTANT_DIR/probe-m01.sh" 2>&1 || true)"
case "$out" in
  *'HW_CHAINING_LEASE_SECONDS=14400'*) : ;;
  *) fail "M01 VACUOUS: the lease variable did not survive the reverted allowlist, so this arm is not reproducing the incident — out: $(printf '%s' "$out" | tail -3 | tr '\n' ' ')" ;;
esac
saw_mutant "M01 reverts the sweep to the three-name allowlist, and the lease variable survives again" "$out" \
  'M01-ALLOWLIST-RESTORED'

# M02 — the sweep kept, but the inverted list dropped, so it eats the harness's
# own inputs and silently disables 31's and 36's live arms. The mutant is the
# plausible over-correction, which is why it needs its own arm.
mutate_common M02 \
  '_HW_TEST_INPUTS=" HW_UNSTICK_BIN HW_SOURCE HW_CHECK_LIVE_REVIEW_DRIFT HW_CHECK_LIVE_REVIEW_BINDING "' \
  'printf "M02-INPUTS-NOT-SPARED\n" >&2; _HW_TEST_INPUTS=" "'
sed "s|$COMMON|$MUTANT_DIR/_common.sh|" "$TMP/probe.sh" > "$MUTANT_DIR/probe-m02.sh"
out="$(pollute HW_UNSTICK_BIN=/keep/unstick HW_CHECK_LIVE_REVIEW_DRIFT=1 \
        bash "$MUTANT_DIR/probe-m02.sh" 2>&1 || true)"
case "$out" in
  *'KEPT_DRIFT=<swept>'*) : ;;
  *) fail "M02 SURVIVED: the live-arm gate was still spared with the input list emptied: $(printf '%s' "$out" | grep KEPT_ | tr '\n' ' ')" ;;
esac
saw_mutant "M02 empties the spared-input list, so a deliberate live-arm opt-in is swept away" "$out" \
  'M02-INPUTS-NOT-SPARED'

# M03 — the two non-HW_ names dropped. They are outside the prefix, so nothing
# else covers them, and both are pane state that hw bakes in.
mutate_common M03 \
  'unset ENGRAM_PROJECT AGENT_BROWSER_NAMESPACE 2>/dev/null || true' \
  'printf "M03-NON-PREFIX-NAMES-DROPPED\n" >&2'
sed "s|$COMMON|$MUTANT_DIR/_common.sh|" "$TMP/probe.sh" > "$MUTANT_DIR/probe-m03.sh"
out="$(pollute bash "$MUTANT_DIR/probe-m03.sh" 2>&1 || true)"
case "$out" in
  *'ENGRAM_PROJECT=pollution'*) : ;;
  *) fail "M03 VACUOUS: ENGRAM_PROJECT did not survive, so the mutated line may never have run — out: $(printf '%s' "$out" | tail -3 | tr '\n' ' ')" ;;
esac
saw_mutant "M03 drops the two names outside the HW_ prefix, and pane state survives again" "$out" \
  'M03-NON-PREFIX-NAMES-DROPPED'

# M04 — THE SECOND RUNNER's sweep reverted to the two-name enumeration it
# shipped with, which is the fourth instance in this file's header. The mutation
# target is `setup/test-channel-send`, so it goes in a temp copy of bin/ plus the
# runner; the real file is never touched.
CS_MUT="$TMP/mutant-M04"
mkdir -p "$CS_MUT/setup"
cp -R "$ROOT/bin" "$CS_MUT/bin"
cp "$ROOT/setup/test-channel-send" "$CS_MUT/setup/"
chmod +x "$CS_MUT/setup/test-channel-send"
python3 - "$CS_MUT/setup/test-channel-send" <<'PYM04'
import sys
p = sys.argv[1]
s = open(p, encoding="utf-8").read()
old = 'for _v in ${!HW_@}; do unset "$_v" 2>/dev/null || true; done\n'
n = s.count(old)
if n != 1:
    raise SystemExit("M04 anchor count %d, expected 1" % n)
new = ("printf 'M04-ENUMERATION-RESTORED\\n' >&2\n"
       "unset HW_CHAINING_ENABLED HW_DONE_KEEP_PANE\n")
open(p, "w", encoding="utf-8").write(s.replace(old, new, 1))
PYM04
grep -q 'M04-ENUMERATION-RESTORED' "$CS_MUT/setup/test-channel-send" \
  || fail "M04 did not apply — the prefix sweep in test-channel-send moved"
# Both environments at once, for the same reason as the launch above; a red
# run is this mutant's expected answer, so neither exit status is judged.
( pollute bash "$CS_MUT/setup/test-channel-send" > "$BG/m04-dirty" 2>&1 || true ) & _m04_dirty=$!
( clean   bash "$CS_MUT/setup/test-channel-send" > "$BG/m04-clean" 2>&1 || true ) & _m04_clean=$!
wait "$_m04_dirty" "$_m04_clean" || true
m04_out="$(cat "$BG/m04-dirty")"
case "$m04_out" in
  *M04-ENUMERATION-RESTORED*) ;;
  *) fail "M04 VACUOUS: the mutated line never ran — out: $(printf '%s' "$m04_out" | tail -3 | tr '\n' ' ')" ;;
esac
saw_mutant "M04 the second runner enumerates two of four chaining names, so a chained executor gets a red a brainer does not" \
  "$m04_out" "chaining could not report without hw"
# And the same mutant is GREEN for a brainer, which is the whole disease: the red
# exists only for whoever runs it from an executor pane.
#
# COUNTED, NOT `case`d. The first version of this arm was
# `*"not ok"*) fail ;; *) pass` — a catch-all that passes on the ABSENCE of
# "not ok", so a mutant that died before its first assertion would satisfy it.
# setup/mutation-coverage refused it by name, and it was right: that is the
# 8f5c8ef shape this whole suite is built against. Both runs are now measured by
# their own tallies, and the marker proves each one reached the mutated line.
m04_clean="$(cat "$BG/m04-clean")"
case "$m04_clean" in
  *M04-ENUMERATION-RESTORED*) ;;
  *) fail "M04 VACUOUS on the brainer side: the mutated line never ran — out: $(printf '%s' "$m04_clean" | tail -3 | tr '\n' ' ')" ;;
esac
m04_clean_ok="$(printf '%s\n' "$m04_clean"     | grep -c '^ok ' || true)"
m04_clean_no="$(printf '%s\n' "$m04_clean"     | grep -c '^not ok' || true)"
m04_dirty_ok="$(printf '%s\n' "$m04_out"       | grep -c '^ok ' || true)"
m04_dirty_no="$(printf '%s\n' "$m04_out"       | grep -c '^not ok' || true)"
[ "$m04_clean_no" = 0 ] \
  || fail "M04 is red for a brainer too ($m04_clean_no not ok), so it is not reproducing the environment-dependent shape — the brainer's red: $(printf '%s\n' "$m04_clean" | grep -A3 '^not ok' | head -8 | tr '\n' ' ')"
[ "$m04_dirty_no" -ge 1 ] \
  || fail "M04 is green for the executor too ($m04_dirty_no not ok), so the enumeration is not what produced the split"
[ "$m04_clean_ok" -gt "$m04_dirty_ok" ] \
  || fail "M04 got at least as far for the executor ($m04_dirty_ok ok) as for the brainer ($m04_clean_ok ok), so the run did not diverge"
pass "mutant killed: M04 same tree, two answers — brainer ${m04_clean_ok} ok / ${m04_clean_no} not ok, chained executor ${m04_dirty_ok} ok / ${m04_dirty_no} not ok"

[ "$CLAIMS" -ge 1 ] || fail "no behaviour claims were made"
printf 'coverage - %s behaviour claims, %s dedicated production mutants\n' "$CLAIMS" 4
