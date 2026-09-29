#!/usr/bin/env bash
# The ask cap must leave a reportable exit instead of forcing a silent turn.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HARNESS="$TMP/harness"; WORK="$TMP/work"; mkdir -p "$HARNESS/bin" "$WORK/.hw/run"
cp "$ROOT/bin/ask-invoker" "$ROOT/bin/invoker-common.sh" "$HARNESS/bin/"
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
  python3 - "$MUT" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old='Your legal exit is: done-invoker --blocked \\"<the unresolved question and what would unblock it>\\". The cap never requires a silent turn.'
new='Stop in this pane. The cap is final.'
assert s.count(old) == 1
open(p,"w").write(s.replace(old,new))
PY
  chmod +x "$MUT"
  mout="$(run_cap "$MUT" || true)"
  case "$mout" in *'done-invoker --blocked'*) fail "M01 SURVIVED: capped ask still named the legal exit: $mout" ;; esac
  case "$mout" in
    *'Stop in this pane. The cap is final.'*) pass "mutant killed: M01 removes the ask-cap blocked-report exit" ;;
    *) fail "M01 VACUOUS: the mutant neither named the legal exit nor emitted the mutated text, so the mutated line never ran: $mout" ;;
  esac
  printf 'mapping - C01↔M01 ask-cap legal exit\n'
  printf 'coverage - 1 behavior claim, 1 dedicated ask-cap mutant killed\n'
