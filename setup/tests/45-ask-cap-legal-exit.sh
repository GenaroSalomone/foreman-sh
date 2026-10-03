#!/usr/bin/env bash
# The ask cap must leave a reportable exit instead of forcing a silent turn.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HARNESS="$TMP/harness"; WORK="$TMP/work"; mkdir -p "$HARNESS/bin" "$WORK/.hw/run"
cp "$ROOT/bin/ask-invoker" "$ROOT/bin/invoker-common.sh" "$ROOT/bin/runenv" "$HARNESS/bin/"
cat > "$HARNESS/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
cat > "$HARNESS/bin/channel-send" <<'STUB'
#!/usr/bin/env bash
exit 99
STUB
chmod +x "$HARNESS/bin/ask-invoker" "$HARNESS/bin/herdr-rpc" "$HARNESS/bin/channel-send"
printf '3' > "$WORK/.hw/run/ask-count"

run_cap() {
  HW_INVOKER_PANE=wB:p1 HERDR_PANE_ID=wT:p1 HW_TASK=probe HW_PROJECT=setup \
    HW_WORKDIR="$WORK" HW_RUN=run "$1" "one more question" 2>&1
}

out="$(run_cap "$HARNESS/bin/ask-invoker")" && rc=0 || rc=$?
[ "$rc" = 1 ] || fail "C01 capped ask expected exit 1, got $rc: $out"
case "$out" in *'done-invoker --blocked "<the unresolved question and what would unblock it>"'*'cap never requires a silent turn'*) pass "C01 the ask cap names the blocked-report legal exit" ;; *) fail "C01 ask cap created a silent dead end: $out" ;; esac

# MUTATION ARMS RUN UNCONDITIONALLY. They used to sit behind
# `if [ "${ASK_CAP_MUTATION_TEST:-0}" = 1 ]`, which `setup/test-hw` never set, so a 909-ok run
# exercised none of them. The stated reason was "committed bytes only"; the arms
# copy from $ROOT, the working tree, like every ungated arm here. See
# setup/tests/46-mutation-arms-are-not-gated.sh and setup/mutation-coverage.
# THE MUTANT IS CO-LOCATED WITH ITS LIBRARY, and the arm proves the mutated
# line RAN. Copying ask-invoker alone made this arm VACUOUS: `bin/ask-invoker`
# resolves invoker-common.sh from its OWN directory via BASH_SOURCE, a plain cp
# is not a symlink, so the copy died with "cannot find invoker-common.sh beside
# …" BEFORE reaching the mutated string — and the old catch-all `*) pass` filed
# that environment error as "mutant killed". Verified 2026-09-01 by executing
# the copy directly. So: copy the whole bin (the pattern 41-ruling-refusal.sh
# already uses), and assert POSITIVELY on the mutated text, because a mutant
# that cannot run must FAIL this arm rather than satisfy it by dying.
  MUTDIR="$TMP/m01"; mkdir -p "$MUTDIR"; cp "$HARNESS/bin"/* "$MUTDIR/"
  MUT="$MUTDIR/ask-invoker"
  mutate_anchor 45-M01 "$MUT" 'die "Stop in this pane. The cap is final."'
  chmod +x "$MUT"
  mout="$(run_cap "$MUT" || true)"
  case "$mout" in *'done-invoker --blocked'*) fail "M01 SURVIVED: capped ask still named the legal exit: $mout" ;; esac
  case "$mout" in
    *'Stop in this pane. The cap is final.'*) pass "mutant killed: M01 removes the ask-cap blocked-report exit" ;;
    *) fail "M01 VACUOUS: the mutant neither named the legal exit nor emitted the mutated text, so the mutated line never ran: $mout" ;;
  esac
  printf 'mapping - C01↔M01 ask-cap legal exit\n'
  printf 'coverage - 1 behavior claim, 1 dedicated ask-cap mutant killed\n'

# Under gentle the phase-1 approval rides an ask, so the cap is 4 there and
# stays 3 everywhere else. OLD binary (cap 3 always) refuses the 4th under
# gentle; the new one accepts it. The stub channel-send exits 99, so an
# accepted ask surfaces as something other than the cap message.
printf '  framework   gentle  (chosen)\n' > "$WORK/.hw/run/dispatch"
gout="$(run_cap "$HARNESS/bin/ask-invoker")" && grc=0 || grc=$?
case "$gout" in *'already asked the brainer'*) fail "G01 gentle run refused its 4th ask: $gout" ;; *) pass "G01 gentle run accepts the 4th ask (rc=$grc)" ;; esac
printf '4' > "$WORK/.hw/run/ask-count"
gout="$(run_cap "$HARNESS/bin/ask-invoker")" && grc=0 || grc=$?
case "$gout" in *'already asked the brainer 4 times on this task (limit 4)'*) pass "G02 gentle run is capped at 4" ;; *) fail "G02 gentle 5th ask not capped: $gout" ;; esac
printf '  framework   none  (chosen)\n' > "$WORK/.hw/run/dispatch"
printf '3' > "$WORK/.hw/run/ask-count"
nout="$(run_cap "$HARNESS/bin/ask-invoker")" && nrc=0 || nrc=$?
case "$nout" in *'already asked the brainer 3 times on this task (limit 3)'*) pass "G03 non-gentle cap stays 3" ;; *) fail "G03 non-gentle cap moved: $nout" ;; esac
