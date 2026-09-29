#!/usr/bin/env bash
# `hw wait` — the witness-backed wait, and the one broken state it refuses to
# start a wait against.
#
# WHAT THE LIVE PROOF IS AND WHY A FIXTURE CANNOT BE IT. The measurement is in
# the commit: a REAL parked executor (setup:stranded-pair-probe, w4C:pBJ) driven
# into `done_state=undelivered` + `turn_state=ended_unreported` by reporting to
# a pane that does not exist, and `hw wait` refusing it in under a second where
# `herdr agent wait --until idle --timeout 1800000` would have sat for thirty
# minutes. A stub cannot stand in for that: the bug is that the WAIT believes
# `agent_status`, and a fixture that stubs `agent_status` is not the path.
#
# What a fixture CAN pin down, and what nothing else in this suite does, is the
# exit-code contract. `hw` documents ONE failure code — 1, every refusal — and
# this command adds four verdict codes on top of it. That distinction is the
# whole interface: a caller that cannot tell "hw refused" from "the executor is
# settled" is back to reading prose, which is exactly what the raw herdr call
# left everyone doing.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── a bin dir hw resolves as its own ─────────────────────────────────────────
# hw derives HW_BIN_DIR from its own path and loads state-witness.sh, herdr-rpc
# and the rest from beside itself. Copying the real bin/ and replacing ONLY
# herdr-rpc gives the real hw, the real cmd_wait and the real witness against
# stubbed readers — rather than a re-implementation that can drift from both.
BIN="$TMP/bin2"
cp -R "$ROOT/bin" "$BIN"
[ -x "$BIN/hw" ] || fail "hw wait: the copied bin/ has no executable hw"

STUB_LOG="$TMP/log"; : > "$STUB_LOG"
export STUB_LOG

cat > "$BIN/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
printf '%s %s\n' "$1" "${2:-}" >> "$STUB_LOG"
if [ "$1 ${2:-}" = "call pane.read" ]; then
  case "${W_SCREEN:-static}" in
    fail) exit 1 ;;
    moving)
      n=$(( $(cat "$W_TICK" 2>/dev/null || echo 0) + 1 ))
      printf '%s' "$n" > "$W_TICK"
      printf '{"read":{"text":"tick %s"}}\n' "$n" ;;
    *) printf '{"read":{"text":"an unchanging screen"}}\n' ;;
  esac
  exit 0
fi
if [ "$1" = wait-agent ]; then
  printf '%s' "$(( $(cat "$W_WAITS" 2>/dev/null || echo 0) + 1 ))" > "$W_WAITS"
  exit "${W_WAIT_RC:-2}"
fi
printf '{"result":{}}\n'
STUB
chmod +x "$BIN/herdr-rpc"

# `herdr` on PATH, not in BIN — hw calls it by bare name. Overwrites _common's.
cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
printf 'herdr %s %s\n' "$1" "${2:-}" >> "$STUB_LOG"
case "$1 ${2:-}" in
  "agent get")
    if [ "${W_NOAGENT:-0}" = 1 ]; then printf '{"result":{}}\n'; exit 0; fi
    _t="${W_TOKENS:-}"; [ -n "$_t" ] || _t='{}'
    printf '{"result":{"agent":{"agent":"opencode","agent_status":"%s","tokens":%s}}}\n' \
      "${W_STATUS:-blocked}" "$_t" ;;
  "pane process-info")
    case "${W_PROCINFO:-port}" in
      none) printf '{"result":{"process_info":{"foreground_processes":[]}}}\n' ;;
      *) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","%s"]}]}}}\n' "${W_PORT:-49999}" ;;
    esac ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
chmod +x "$TMP/bin/herdr"

cat > "$TMP/bin/curl" <<'STUB'
#!/usr/bin/env bash
url="${!#}"
case "${W_PROMPTS:-none}" in
  unreadable) printf 'not json' ;;
  question) case "$url" in *"/question") printf '[{"id":"q1"}]' ;; *) printf '[]' ;; esac ;;
  *) printf '[]' ;;
esac
STUB
chmod +x "$TMP/bin/curl"

export W_TICK="$TMP/tick" W_WAITS="$TMP/waits"
# Small slices: the assertions are about WHICH verdict ends a wait, never about
# the real cadence, and a 20s slice would make this file take minutes.
export WITNESS_SLICE_MS=200 WITNESS_INTERVAL_S=0 WITNESS_SAMPLES=2

# hw_wait <args...> — runs the real command, strips colour, records the status.
#
# THE STATUS GOES THROUGH A FILE, and that is not a style choice. Every call
# site here is `out="$(... hw_wait ...)"`, which runs the function in a
# COMMAND-SUBSTITUTION SUBSHELL: a variable assigned inside it is discarded on
# the way out, so a plain `WAIT_RC=` would read as whatever the previous case
# left behind. Caught while writing this file — the first assertion "failed"
# against a command that was already returning the right code.
WAIT_RC_FILE="$TMP/rc"
hw_wait() {
  : > "$W_WAITS"; : > "$W_TICK"
  # The public command now launches a detached set monitor. This subject pins
  # the unchanged one-target verdict engine that each monitor worker executes.
  "$BIN/hw" wait-one "$@" 2>&1 | sed 's/\x1b\[[0-9;]*m//g'
  printf '%s' "${PIPESTATUS[0]}" > "$WAIT_RC_FILE"
}
expect_rc() {
  local label="$1" want="$2" got
  got="$(cat "$WAIT_RC_FILE" 2>/dev/null || echo missing)"
  [ "$got" = "$want" ] && pass "$label" || fail "$label — wanted exit $want, got $got"
}
expect_says() {
  local label="$1" pat="$2" out="$3"
  case "$out" in *"$pat"*) pass "$label" ;; *) fail "$label — no '$pat' in: $out" ;; esac
}
expect_silent() {
  local label="$1" pat="$2" out="$3"
  case "$out" in *"$pat"*) fail "$label — '$pat' should NOT appear in: $out" ;; *) pass "$label" ;; esac
}

# ── THE NAMED BROKEN STATE ───────────────────────────────────────────────────
#
# `turn_state=ended_unreported` WITH `done_state=undelivered`. The report exists,
# it did not reach the brainer, and the turn that would have retried it is over.
# No budget reaches idle from there, so no budget is spent finding that out.
STRANDED='{"turn_state":"ended_unreported","done_state":"undelivered","done_status":"done","turn_ended_at":"2026-08-27T18:51:48Z"}'

out="$(W_TOKENS="$STRANDED" W_STATUS=working hw_wait wX:p1)"
expect_rc   "hw wait: the stranded pair is its own verdict, exit 7" 7
expect_says "hw wait: it names the state rather than timing out on it" "ENDED AND NEVER REPORTED" "$out"
expect_says "hw wait: it says both halves, so the reader can check them" "done_state=undelivered" "$out"
expect_says "hw wait: and names the turn half too" "turn_state=ended_unreported" "$out"
# THE MUTANT THIS KILLS: moving the pair check below the wait. The command would
# still exit 7 and still print the right words — after burning the whole budget,
# which is the entire failure being fixed. Only the absence of a wait-agent call
# distinguishes the fix from the bug wearing its clothes.
w="$(cat "$W_WAITS" 2>/dev/null || echo 0)"
[ "${w:-0}" = 0 ] \
  && pass "hw wait: NO wait was started against a stranded pane — the check is before the budget, not after it" \
  || fail "hw wait: a stranded pane still spent $w wait-agent call(s); the pair check ran too late to be the fix"
expect_says "hw wait: it forbids the two moves that cannot help here" "hw unstick" "$out"

# EITHER HALF ALONE IS ORDINARY, and calling it stranded would be a false alarm
# on a healthy executor. Measured 2026-08-27: w30:p54 published
# `turn_state=ended_unreported` with turns=6 while visibly mid-work, because a
# background-subagent notification opens a turn with no prompt behind it.
out="$(W_TOKENS='{"turn_state":"ended_unreported","turns":"6"}' W_SCREEN=moving hw_wait wX:p1 --dry-run)"
expect_rc   "hw wait: turn_state alone is NOT the stranded pair" 0
expect_silent "hw wait: and a healthy mid-work pane is never accused of it" "ENDED AND NEVER REPORTED" "$out"

out="$(W_TOKENS='{"done_state":"undelivered","done_status":"done"}' W_SCREEN=moving hw_wait wX:p1 --dry-run)"
expect_rc   "hw wait: done_state alone is NOT the stranded pair — a delivery may still be in flight" 0
expect_silent "hw wait: an in-flight delivery is not reported as abandoned" "ENDED AND NEVER REPORTED" "$out"

# ── THE FOUR VERDICTS ────────────────────────────────────────────────────────
#
# settled: every reader answered and every one said no.
out="$(W_STATUS=working W_SCREEN=static W_PROMPTS=none hw_wait wX:p1 --timeout-ms $((4000 * HW_TEST_SLOW)))"
expect_rc   "hw wait: a state proved unable to change ends the wait, exit 5" 5
expect_says "hw wait: a settled verdict says a bigger budget is not the answer" "larger budget is never the answer" "$out"

# awaiting-human: the pane's own endpoint CONFIRMED an outstanding prompt. A
# distinct code because the advice is not the same — 5 says look at the pane,
# 6 says only a person can move this.
out="$(W_STATUS=working W_SCREEN=static W_PROMPTS=question hw_wait wX:p1 --timeout-ms $((4000 * HW_TEST_SLOW)))"
expect_rc   "hw wait: an outstanding prompt is its own verdict, exit 6" 6
expect_says "hw wait: it says a PERSON is being waited on, not time" "waiting for a PERSON" "$out"

# live: the screen moved, so this is a real wait and it keeps every millisecond.
# THE MUTANT: treating `live` like `settled` would cut a working executor short —
# the mirror of the bug, and the more expensive direction.
out="$(W_STATUS=blocked W_SCREEN=moving W_PROMPTS=none hw_wait wX:p1 --timeout-ms $((1200 * HW_TEST_SLOW)))"
expect_rc   "hw wait: a moving pane is never cut short — the budget runs out instead" 2
expect_says "hw wait: and it SAYS it is still live, so a long wait is not mistaken for a hang" "still LIVE after" "$out"

# unknown: fall back to exactly the old behaviour, and SAY the cross-check was
# unavailable. This property is why the other three may be acted on at all.
out="$(W_STATUS=working W_SCREEN=fail hw_wait wX:p1 --timeout-ms $((1200 * HW_TEST_SLOW)))"
expect_rc   "hw wait: an unreadable cross-check costs the caller nothing extra — the ordinary timeout, exit 2" 2
expect_says "hw wait: an unavailable cross-check is ANNOUNCED, never assumed" "cross-check UNAVAILABLE" "$out"
expect_says "hw wait: the FINAL timeout message carries the detail too, not just the progress lines" "Last cross-check:" "$out"
expect_silent "hw wait: and it is never rendered as a proved verdict" "SETTLED" "$out"

# reaching idle is still the normal path, and still returns immediately.
out="$(W_WAIT_RC=0 W_STATUS=idle hw_wait wX:p1 --timeout-ms $((4000 * HW_TEST_SLOW)))"
expect_rc   "hw wait: a pane that reaches idle/done returns 0" 0
expect_says "hw wait: and says the pane is ready to be re-tasked" "ready for \`hw next\`" "$out"

# ── TRANSPORT IS NOT A VERDICT ───────────────────────────────────────────────
# herdr-rpc's 3 and 4 are facts about the multiplexer. Dressing either up as a
# statement about the executor is the precise mistake this command exists to
# stop, so they are passed through unchanged and labelled.
out="$(W_WAIT_RC=3 W_STATUS=working hw_wait wX:p1 --timeout-ms $((4000 * HW_TEST_SLOW)))"
expect_rc   "hw wait: a transport failure keeps herdr-rpc's own code" 3
expect_says "hw wait: and is stated as a fact about herdr, NOT about the executor" "NOT about wX:p1" "$out"

# ── REFUSALS STAY 1 ──────────────────────────────────────────────────────────
# hw documents exactly one failure code. A verdict code leaking onto a refusal
# would make the whole contract unreadable.
out="$(hw_wait)"
expect_rc "hw wait: no pane is a refusal, exit 1" 1
out="$(hw_wait wX:p1 --timeout-ms later)"
expect_rc "hw wait: a non-numeric budget is a refusal, exit 1" 1
out="$(W_NOAGENT=1 hw_wait wX:p1)"
expect_rc   "hw wait: an unreadable pane is a REFUSAL, not a verdict about it" 1
expect_says "hw wait: and says plainly that nothing was waited on" "Nothing was waited on" "$out"

# ── IT SENDS NOTHING, EVER ───────────────────────────────────────────────────
# The one property that makes this safe to point at another brainer's executor.
# `hw wait` is read-only by construction; if that ever stops being true, every
# use of it against a pane you do not own becomes an intrusion.
: > "$STUB_LOG"
out="$(W_STATUS=working W_SCREEN=static hw_wait wX:p1 --timeout-ms 1200)"
if grep -qE 'agent prompt|pane send-text|pane send-keys|agent start' "$STUB_LOG"; then
  fail "hw wait: it sent something to the pane — $(grep -E 'agent prompt|send' "$STUB_LOG" | head -1)"
else
  pass "hw wait: nothing is ever sent to the target pane, on any path"
fi

# ── THE PROGRESS CALLBACK IS OPT-IN ──────────────────────────────────────────
#
# witness_wait's other callers are gates with fixed output contracts, and
# channel-send's is read inside `$(...)` — a stray progress line there is not a
# report, it is a corrupted return value. So the hook defaults to nothing.
# THE MUTANT: an unconditional printf inside witness_wait. Every assertion above
# still passes, and channel-send's captured output silently grows lines.
cat > "$TMP/optin.sh" <<'PROBE'
set -u
WITNESS_RPC="$BIN/herdr-rpc"
. "$BIN/state-witness.sh"
# STDOUT IS THE SUBJECT — do not redirect it. This line read
# `>/dev/null 2>&1` first, which made the assertion below unfalsifiable: an
# unconditional printf inside witness_wait survived the mutant untouched.
witness_wait wX:p1 1200 2>/dev/null || true
PROBE
noise="$(WITNESS_SLICE_MS=200 WITNESS_INTERVAL_S=0 WITNESS_SAMPLES=2 \
         W_STATUS=working W_SCREEN=static BIN="$BIN" \
         bash "$TMP/optin.sh" 2>/dev/null || true)"
[ -z "$noise" ] \
  && pass "state-witness: witness_wait prints nothing when no caller asked for progress" \
  || fail "state-witness: witness_wait emitted output with WITNESS_ON_VERDICT unset: $noise"

# ── hw status names the same pair ────────────────────────────────────────────
#
# THE LIVE PROOF IS IN THE COMMIT: the same real parked executor, read by HEAD's
# `hw status` and then by this one —
#
#   before   ! setup:stranded-pair-probe   done-NOT-delivered
#                reported done, pane still open
#   after    ! setup:stranded-pair-probe   done-STRANDED
#                turn 1 ended 2026-08-27T19:00:39Z and the delivery had already
#                failed — nothing in that pane is going to retry it
#
# `done-NOT-delivered` was never wrong, it was INCOMPLETE: it reads as a
# delivery that might still land, and this one cannot. The `elif turn_ended`
# branch that would have said so is unreachable from there — `done_tok` consumes
# the chain — so the second fact was computed and dropped.
#
# What a fixture adds is the truth table, which the live pane can only ever
# occupy one cell of. The classifier is EXTRACTED FROM bin/hw rather than
# restated, so an edit to the real expression is what runs here.
# MATCH THE ASSIGNMENT, NEVER THE OPERATORS. Grepping for the full expression
# meant a mutant that changed `and` to `or` was killed by the grep failing —
# so the truth table below never ran against the mutation it was written for,
# and would not have caught it. It now extracts whatever the line says.
STRAND_EXPR="$(grep -E '^[[:space:]]*stranded = ' "$ROOT/bin/hw" | sed 's/^[[:space:]]*//' || true)"
[ -n "$STRAND_EXPR" ] \
  && pass "hw status: the stranded classifier is present in bin/hw" \
  || fail "hw status: no 'stranded = ...' line in bin/hw — the pair is unnamed again"

# turn_ended, done_state, done_newest -> expected
# done_newest is the on-disk marker proving the brainer WAS told; with it, the
# stale `undelivered` token is simply out of date and nothing is stranded.
while IFS='|' read -r te ds dn want label; do
  [ -n "$te" ] || continue
  got="$(python3 - "$te" "$ds" "$dn" <<PY
import sys
turn_ended = sys.argv[1] == "1"
done_state = sys.argv[2] or None
done_newest = sys.argv[3] == "1"
$STRAND_EXPR
print("1" if stranded else "0")
PY
)"
  [ "$got" = "$want" ] && pass "hw status: $label" \
    || fail "hw status: $label — wanted stranded=$want, got $got"
done <<'TABLE'
1|undelivered|0|1|both halves and no delivery marker is STRANDED
1||0|0|a turn that ended with no report published is not stranded
0|undelivered|0|0|an undelivered report whose turn is still open is not stranded
1|delivered|0|0|a delivered report is not stranded, whatever the turn did
1|undelivered|1|0|an on-disk done marker outranks a stale undelivered token
TABLE
