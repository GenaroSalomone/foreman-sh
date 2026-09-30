#!/usr/bin/env bash
# done-invoker: a delivered report closes its owned task container

. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

make_case() {
  local name="$1" dir
  dir="$TMP/$name"
  mkdir -p "$dir/bin" "$dir/work/.hw/run" "$dir/artifacts"
  cp "$ROOT/bin/done-invoker" "$ROOT/bin/invoker-common.sh" "$ROOT/bin/state-witness.sh" "$dir/bin/"
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
    HW_RUN=run HW_WORKDIR="$dir/work" HW_ARTIFACTS="$dir/artifacts" \
    "$@" "$dir/bin/done-invoker" "completed; findings in engram #1" \
    > "$dir/out" 2>&1
  CASE_RC=$?
  set -e
}

run_summary_case() { # name summary
  local name="$1" summary="$2" dir
  dir="$TMP/$name"
  set +e
  env PATH="$dir/bin:$PATH" STUB_EVENTS="$dir/events" STUB_MESSAGE="$dir/message" STUB_DELIVERY_ARGS="$dir/delivery-args" \
    HW_INVOKER_PANE=wT:p1 HERDR_PANE_ID=wT:p2 HW_TASK=task HW_PROJECT=setup \
    HW_RUN=run HW_WORKDIR="$dir/work" HW_ARTIFACTS="$dir/artifacts" \
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
[ "$(cat "$TMP/delivered/events")" = $'deliver\nclose done setup task\nartifacts '"$TMP"$'/delivered/artifacts\nmarker-present' ] \
  || fail "done close: order was not receiver delivery -> done marker -> hw done: $(tr '\n' ' ' < "$TMP/delivered/events")"
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
[ "$CASE_RC" = 4 ] || fail "done close: hw close failure exited $CASE_RC, wanted 4 (delivered but not closed) — 1 would say the report was lost: $(cat "$TMP/close-failed/out")"
grep -q 'could not close the owned task tab/workspace' "$TMP/close-failed/out" \
  || fail "done close: hw close failure was not surfaced"
head -1 "$TMP/close-failed/out" | grep -q 'THE REPORT IS DELIVERED AND SAFE' \
  || fail "done close: close failure does not LEAD with the report being delivered and safe: $(cat "$TMP/close-failed/out")"
grep -q 'Do NOT re-run done-invoker' "$TMP/close-failed/out" \
  || fail "done close: close failure does not warn against the retry that would double-report"
[ -f "$TMP/close-failed/work/.hw/run/done" ] \
  || fail "done close: close failure discarded the done marker, so a retry would double-report"
case "$(cat "$TMP/close-failed/events")" in
  $'deliver\nclose done setup task\nartifacts '"$TMP"$'/close-failed/artifacts\nmarker-present') pass "done close: a failed owner-aware close inherits the executor artifact path and exits 4 after proven delivery" ;;
  *) fail "done close: close-failure fixture did not traverse the expected path" ;;
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

# Mutation: remove the content barrier. The old behavior must become visible as
# a delivery and done marker, proving this test is not satisfied by another
# precondition in the harness.
make_case empty-mutant
python3 - "$TMP/empty-mutant/bin/done-invoker" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old='  case "$SUMMARY" in\n    *[![:space:]]*) ;;\n    *) die "summary is empty. A completion report needs a substantive one-paragraph pointer to what was established and its engram ids; zero artifacts is valid, an empty summary is not. NOTHING was sent or marked." ;;\n  esac\n'
assert s.count(old) == 1
open(p,"w").write(s.replace(old,''))
PY
chmod +x "$TMP/empty-mutant/bin/done-invoker"
run_summary_case empty-mutant ""
[ "$CASE_RC" = 0 ] || fail "done summary mutant: removing the barrier did not reproduce old acceptance"
[ "$(cat "$TMP/empty-mutant/events")" = $'deliver\nclose done setup task\nartifacts '"$TMP"$'/empty-mutant/artifacts\nmarker-present' ] \
  || fail "done summary mutant: empty report did not traverse delivery and accounting"
pass "mutant killed: removing the non-empty summary barrier restores the hollow done report"

# Mutation: disconnect done-invoker from the task state proof. The ordinary
# success stub above now models the blocked route's minimum authority and must
# refuse before the marker is written.
make_case report-proof-mutant
python3 - "$TMP/report-proof-mutant/bin/done-invoker" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old='if invoker_deliver "$MSG" "$STATE_DIR"; then'
new='if invoker_deliver "$MSG"; then'
assert s.count(old) == 1
open(p,"w").write(s.replace(old,new))
PY
chmod +x "$TMP/report-proof-mutant/bin/done-invoker"
run_case report-proof-mutant
[ "$CASE_RC" != 0 ] || fail "M11 SURVIVED: done-invoker reported without passing its task state proof"
[ ! -e "$TMP/report-proof-mutant/work/.hw/run/done" ] || fail "M11 SURVIVED: proofless report wrote the done marker"
pass "mutant killed: M11 disconnects done-invoker from the dispatch-bound report proof"
