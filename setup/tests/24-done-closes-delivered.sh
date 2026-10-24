#!/usr/bin/env bash
# done-invoker: a delivered report closes its owned task container
# A delivered SUCCESS report. Since 2026-10-01 a --blocked one keeps its pane
# waiting for a ruling instead of closing (590); every case here is a success.

. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
need_invokers   # this subject runs the invokers from a copy of bin/, which has no core/dist beside it: they are Go (go-060 S2)

make_case() {
  local name="$1" dir
  dir="$TMP/$name"
  mkdir -p "$dir/bin" "$dir/work/.hw/run" "$dir/artifacts"
  cp "$ROOT/bin/done-invoker" "$ROOT/bin/invoker-common.sh" "$ROOT/bin/runenv" "$ROOT/bin/state-witness.sh" "$dir/bin/"
cat > "$dir/bin/channel-send" <<'STUB'
#!/usr/bin/env bash
printf 'deliver\n' >> "$STUB_EVENTS"
printf '%s\n' "$*" > "$STUB_DELIVERY_ARGS"
printf '%s' "${*: -1}" > "$STUB_MESSAGE"
case " $* " in
  *" --report --report-state $HW_WORKDIR/.hw/$HW_RUN herdr $HW_INVOKER_PANE "*) ;;
  *) exit 2 ;;
esac
exit "${STUB_DELIVERY_RC:-0}"
STUB
  cat > "$dir/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
state="$(dirname "$0")/metadata.json"
case "$2" in
  pane.report_metadata) printf '%s' "$3" > "$state"; printf '{"type":"ok"}\n' ;;
  agent.list) jq '{agents:[{pane_id:.pane_id,tokens:(.tokens | with_entries(select(.value != null)))}]}' "$state" ;;
  *) exit 0 ;;
esac
STUB
  cat > "$dir/bin/hw" <<'STUB'
#!/usr/bin/env bash
printf 'close %s\n' "$*" >> "$STUB_EVENTS"
printf 'artifacts %s\n' "${HW_ARTIFACTS:-<unset>}" >> "$STUB_EVENTS"
if [ -f "$HW_WORKDIR/.hw/$HW_RUN/done" ]; then
  printf 'marker-present\n' >> "$STUB_EVENTS"
else
  printf 'marker-absent\n' >> "$STUB_EVENTS"
fi
# The closer exports the pid of the invoker it waited for: is it still there?
if kill -0 "$HW_DONE_INVOKER_PID" 2>/dev/null; then
  printf 'invoker-alive\n' >> "$STUB_EVENTS"
else
  printf 'invoker-gone\n' >> "$STUB_EVENTS"
fi
exit "${STUB_HW_RC:-0}"
STUB
  cat > "$dir/bin/herdr" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
  chmod +x "$dir/bin/"*
}

run_case() { # name [extra env assignments...]
  local name="$1" dir
  dir="$TMP/$name"
  shift
  set +e
  env PATH="$dir/bin:$PATH" STUB_EVENTS="$dir/events" STUB_MESSAGE="$dir/message" STUB_DELIVERY_ARGS="$dir/delivery-args" \
    HW_INVOKER_PANE=wT:p1 HERDR_PANE_ID=wT:p2 HW_TASK=task HW_PROJECT=setup \
    HW_RUN=run HW_WORKDIR="$dir/work" HW_ARTIFACTS="$dir/artifacts" HW_CLOSE_GRACE_SECONDS=0 \
    "$@" "$dir/bin/done-invoker" "completed; findings in engram #1" \
    > "$dir/out" 2>&1
  CASE_RC=$?
  set -e
}

# The close is no longer a child of the invoker: it starts after the invoker has
# exited, in a process of its own (invoker_close_after_exit). Wait for its record
# — close-rc is written when `hw done` returns, whatever it returned.
wait_close() { # name
  local end=$((SECONDS + 15))
  while [ ! -e "$TMP/$1/work/.hw/run/close-rc" ]; do
    [ "$SECONDS" -lt "$end" ] || fail "done close: $1: the detached close never finished: $(tr '\n' ' ' < "$TMP/$1/events" 2>/dev/null)"
    sleep 0.1
  done
}

run_summary_case() { # name summary
  local name="$1" summary="$2" dir
  dir="$TMP/$name"
  set +e
  env PATH="$dir/bin:$PATH" STUB_EVENTS="$dir/events" STUB_MESSAGE="$dir/message" STUB_DELIVERY_ARGS="$dir/delivery-args" \
    HW_INVOKER_PANE=wT:p1 HERDR_PANE_ID=wT:p2 HW_TASK=task HW_PROJECT=setup \
    HW_RUN=run HW_WORKDIR="$dir/work" HW_ARTIFACTS="$dir/artifacts" HW_CLOSE_GRACE_SECONDS=0 \
    "$dir/bin/done-invoker" "$summary" > "$dir/out" 2>&1
  CASE_RC=$?
  set -e
}

# Delivery is first; only then may the durable marker exist and hw's existing
# owner-aware close path run. This catches both a close-before-delivery race and
# a return to the old mark-and-wait-for-sweep backlog.
make_case delivered
run_case delivered
[ "$CASE_RC" = 0 ] || fail "done close: delivered fixture failed: $(cat "$TMP/delivered/out")"
wait_close delivered
[ "$(cat "$TMP/delivered/events")" = $'deliver\nclose done setup task\nartifacts '"$TMP"$'/delivered/artifacts\nmarker-present\ninvoker-gone' ] \
  || fail "done close: order was not receiver delivery -> done marker -> hw done, run AFTER the invoker exited: $(tr '\n' ' ' < "$TMP/delivered/events")"
case "$(cat "$TMP/delivered/message")" in
  *"receiver admitted the report"*"hw done"*"--keep-pane"*"chaining"*)
    pass "done close: the report reaches the brainer before the existing hw close path runs" ;;
  *) fail "done close: delivered envelope does not explain automatic close and its escape hatch" ;;
esac
case "$(cat "$TMP/delivered/delivery-args")" in
  *"--report --report-state $TMP/delivered/work/.hw/run herdr wT:p1"*) pass "done close: the completion carries its dispatch-bound unreported task state to channel-send" ;;
  *) fail "done close: completion omitted its blocked-report authority: $(cat "$TMP/delivered/delivery-args")" ;;
esac

# Refusal is still recovery state: no done marker, no close attempt, and the
# established timeout wording survives. The pane copy is not sacrificed merely
# because publishing the undelivered tokens succeeded.
make_case refused
run_case refused STUB_DELIVERY_RC=2
[ "$CASE_RC" -ne 0 ] || fail "done close: refused delivery returned success"
[ ! -e "$TMP/refused/work/.hw/run/done" ] || fail "done close: refused delivery wrote the done marker"
[ "$(cat "$TMP/refused/events")" = deliver ] \
  || fail "done close: refused delivery attempted a close: $(tr '\n' ' ' < "$TMP/refused/events")"
case "$(cat "$TMP/refused/out")" in
  *"was still working after"*"so it was NOT told"*) pass "done close: refused delivery preserves the pane and existing refusal result" ;;
  *) fail "done close: refused delivery wording changed: $(cat "$TMP/refused/out")" ;;
esac

# Chaining is an explicit launch decision, not the default. Delivery and accounting
# still complete, but no close command is issued.
make_case kept
run_case kept HW_CHAINING_ENABLED=1
[ "$CASE_RC" = 0 ] || fail "done close: keep escape hatch failed: $(cat "$TMP/kept/out")"
[ "$(cat "$TMP/kept/events")" = deliver ] || fail "done close: keep escape hatch still invoked hw"
[ -f "$TMP/kept/work/.hw/run/done" ] || fail "done close: keep escape hatch lost delivered accounting"
grep -q 'pane kept because chaining is enabled' "$TMP/kept/out" \
  || fail "done close: chaining did not state why the pane survived"
pass "done close: HW_CHAINING_ENABLED=1 preserves a delivered pane for hw next"

# Compatibility only: old running executors may still carry the legacy name,
# but no user-facing output should teach it as the current contract.
make_case legacy-kept
run_case legacy-kept HW_DONE_KEEP_PANE=1
[ "$CASE_RC" = 0 ] || fail "done close: legacy keep variable no longer works"
[ "$(cat "$TMP/legacy-kept/events")" = deliver ] || fail "done close: legacy keep variable invoked hw"
grep -q 'chaining is enabled' "$TMP/legacy-kept/out" \
  || fail "done close: legacy compatibility leaked old debug wording"
pass "done close: legacy HW_DONE_KEEP_PANE=1 still enables chaining"

# hw owns the close dialect and its exit status is authoritative. A failure may
# follow a delivered report, but it must never become a close-success line —
# and, since 2026-08-26, it must not become a DELIVERY-failure line either.
# Exit 1 means "nothing was reported, retry"; a caller that reads it about a
# report the brainer is already holding retries and double-reports. Exit 4 is
# the delivered-but-not-closed fact, and the message leads with it.
make_case close-failed
run_case close-failed STUB_HW_RC=9
# SINCE 2026-10-14 THE CLOSE IS NOT THE INVOKER'S TO REPORT. It runs after the
# invoker has exited (a close inside the invoker SIGKILLed it: "Exit code 137"
# on every delivered report), so the invoker's own exit is the delivery's: 0.
# The close's failure is a fact on disk — close-failed — never a lie about the
# report, and never silent. The old expectation here (exit 4 on a failed close)
# asserted the defect: it could only hold where the close was a child of the
# invoker, which is what made every SUCCESSFUL close read as 137.
[ "$CASE_RC" = 0 ] || fail "done close: a delivered report exited $CASE_RC although only the close failed — the close is not the report's exit: $(cat "$TMP/close-failed/out")"
wait_close close-failed
[ -f "$TMP/close-failed/work/.hw/run/close-failed" ] \
  || fail "done close: hw close failure left no close-failed record — a failed close would be silent"
grep -q '^rc=9$' "$TMP/close-failed/work/.hw/run/close-failed" \
  || fail "done close: close-failed does not carry hw's exit status: $(cat "$TMP/close-failed/work/.hw/run/close-failed")"
grep -q 'the report was delivered' "$TMP/close-failed/work/.hw/run/close-failed" \
  || fail "done close: close-failed does not lead with the report being delivered"
[ -f "$TMP/close-failed/work/.hw/run/done" ] \
  || fail "done close: close failure discarded the done marker, so a retry would double-report"
case "$(cat "$TMP/close-failed/events")" in
  $'deliver\nclose done setup task\nartifacts '"$TMP"$'/close-failed/artifacts\nmarker-present\ninvoker-gone') pass "done close: a failed close inherits the executor artifact path, runs after the invoker is gone, and is recorded, not reported as the invoker's exit" ;;
  *) fail "done close: close-failure fixture did not traverse the expected path: $(tr '\n' ' ' < "$TMP/close-failed/events")" ;;
esac

# THE LATE PRECONDITION, one layer up from where it was found. `hw` is a hard
# dependency of the delivered path; a brain/bin missing it must be refused
# before the first irreversible act, so the pane keeps the only copy and the
# identical command can be re-run. Discovered on 2026-08-26 as a missing `hw`
# in setup/test-channel-send's harness, hit AFTER the report had landed.
make_case no-hw
rm -f "$TMP/no-hw/bin/hw"
run_case no-hw
[ "$CASE_RC" = 1 ] || fail "done close: a missing adjacent hw exited $CASE_RC, wanted 1 (nothing reported): $(cat "$TMP/no-hw/out")"
grep -q 'REFUSING BEFORE REPORTING' "$TMP/no-hw/out" \
  || fail "done close: missing hw did not refuse before reporting: $(cat "$TMP/no-hw/out")"
[ ! -e "$TMP/no-hw/events" ] \
  || fail "done close: missing hw still attempted a delivery: $(tr '\n' ' ' < "$TMP/no-hw/events")"
[ ! -e "$TMP/no-hw/work/.hw/run/done" ] || fail "done close: missing hw wrote the done marker"
pass "done close: no adjacent hw refuses before reporting, so the pane keeps the only copy"

# …and the escape hatch that refusal names must work, or it strands finished
# work: chaining owes no close, so a broken install can still report.
make_case no-hw-chained
rm -f "$TMP/no-hw-chained/bin/hw"
run_case no-hw-chained HW_CHAINING_ENABLED=1
[ "$CASE_RC" = 0 ] || fail "done close: chaining could not report without hw: $(cat "$TMP/no-hw-chained/out")"
[ "$(cat "$TMP/no-hw-chained/events")" = deliver ] || fail "done close: chaining without hw attempted a close"
pass "done close: with chaining on, a brain/bin without hw still reports"

# An argument can exist while carrying no report. `done-invoker ""` used to
# pass every gate, publish done, write the marker and become indistinguishable
# from a substantive completion. Refuse both empty and whitespace-only text
# before environment adoption, metadata, delivery or accounting.
make_case empty-summary
run_summary_case empty-summary ""
[ "$CASE_RC" = 1 ] || fail "done summary: empty string exited $CASE_RC, wanted refusal exit 1"
[ ! -e "$TMP/empty-summary/events" ] || fail "done summary: empty string reached delivery or close"
[ ! -e "$TMP/empty-summary/work/.hw/run/done" ] || fail "done summary: empty string wrote a done marker"
grep -q 'summary is empty' "$TMP/empty-summary/out" || fail "done summary: refusal did not name the empty summary"
pass "done summary: an empty argument is refused before reporting"

make_case whitespace-summary
run_summary_case whitespace-summary $' \t\n '
[ "$CASE_RC" = 1 ] || fail "done summary: whitespace-only string exited $CASE_RC, wanted refusal exit 1"
[ ! -e "$TMP/whitespace-summary/events" ] || fail "done summary: whitespace-only string reached delivery or close"
pass "done summary: whitespace-only text is not substantive"

# Two bash mutants stood here, both inline edits of a copy of bin/done-invoker's bash (go-060 S5 deleted it): removing the non-empty summary
# barrier (an empty report traversing delivery and accounting) and M11, disconnecting done-invoker from the dispatch-bound report proof
# (invoker_deliver without the state dir). The Go holds both with goldens, not mutants: testdata/golden/done-invoker/empty_summary.json
# (TestDiffDone/empty_summary: refusal, no delivery, no marker) and delivered_closes.json (the channel-send stub's argv in the disk tree
# carries `--report --report-state <run dir>`); the two refusals and the "carries its dispatch-bound unreported task state to
# channel-send" claim above are the same assertions, driven on the shim and the binary.
