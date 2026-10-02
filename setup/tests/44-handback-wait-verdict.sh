#!/usr/bin/env bash
# hw wait treats an illegal quiet turn as a verdict, not as readiness or a budget.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BIN="$TMP/bin2"; STUB="$TMP/stub"; mkdir -p "$BIN" "$STUB"
cp "$ROOT"/bin/* "$BIN/" 2>/dev/null || true
cat > "$STUB/herdr" <<'STUB'
#!/usr/bin/env bash
case "$1 $2" in
  "agent get") printf '{"result":{"agent":{"agent":"opencode","agent_status":"done","tokens":{"turn_state":"%s"}}}}\n' "${TURN_STATE:-handback_refused}" ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
chmod +x "$STUB/herdr"

run_wait() {
  PATH="$STUB:$PATH" TURN_STATE="${TURN_STATE:-handback_refused}" \
    "$1" wait-one wT:p1 --timeout-ms 10 2>&1
}

out="$(run_wait "$BIN/hw")" && rc=0 || rc=$?
[ "$rc" = 8 ] || fail "C01 handback wait expected verdict 8, got $rc: $out"
case "$out" in *"HAND-BACK REFUSED"*"no wait was started"*"done-invoker --blocked"*"hw human-boundary"*) pass "C01 hw wait refuses to proceed past a recorded handback" ;; *) fail "C01 verdict omitted its exits: $out" ;; esac

# MUTATION ARMS RUN UNCONDITIONALLY. They used to sit behind
# `if [ "${HANDBACK_WAIT_MUTATION_TEST:-0}" = 1 ]`, which `setup/test-hw` never set, so a 909-ok run
# exercised none of them. The stated reason was "committed bytes only"; the arms
# copy from $ROOT, the working tree, like every ungated arm here. See
# setup/tests/46-mutation-arms-are-not-gated.sh and setup/mutation-coverage.
  M1="$TMP/m1"; mkdir -p "$M1"; cp "$BIN"/* "$M1/" 2>/dev/null || true
  mutate_anchor 44-M01 "$M1/hw" 'if [ "$turn_state" = never_handback_refused ]; then'
  chmod +x "$M1/hw"
  mout="$(run_wait "$M1/hw" || true)"
  # KILLED BY WHAT THE MUTANT DID INSTEAD. Removing the handback verdict does not
  # silence hw wait — it makes it fall through and actually START the wait, so the
  # mutant prints the wait banner (`· waiting on wT:p1 for up to 0s, cross-checked
  # every 20s.`) the correct code never reaches. That banner is positive evidence
  # the mutated branch ran; the old shape `*"HAND-BACK REFUSED"*) fail ;; *) pass`
  # certified the kill from the ABSENCE of the verdict, which a mutant that could
  # not run produces too.
  case "$mout" in
    *"waiting on wT:p1 for up to"*) pass "mutant killed: M01 removes hw wait's handback verdict, so it starts the wait it was supposed to refuse" ;;
    *"HAND-BACK REFUSED"*) fail "M01 SURVIVED: mutated wait still emitted the handback verdict" ;;
    *) fail "M01 VACUOUS: the mutant neither refused nor started a wait, so the mutated condition may never have been evaluated — tail: $(printf '%s' "$mout" | tail -3 | tr '\n' ' ')" ;;
  esac
  printf 'mapping - C01↔M01 vendor-neutral wait enforcement\n'
  printf 'coverage - 1 behavior claim, 1 dedicated wait mutant killed\n'
