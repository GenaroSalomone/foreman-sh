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

# The one mutant this file ran (44-M01, removing the handback verdict from bash `hw wait-one` so that it starts the wait it was
# supposed to refuse) edited bin/hw's bash and went with it in go-060 S4. The claim is held on the Go side by the contract-hw
# golden wo-handback (cases-t2: rc 8, the whole refusal text) and by anchor 871-G20 (core/wait.go rcRefused = 8); the condition
# at core/wait.go `turn_state == handback_refused` itself carries no anchor.
printf 'coverage - 1 behavior claim (C01 vendor-neutral wait enforcement)\n'
