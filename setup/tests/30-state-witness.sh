#!/usr/bin/env bash
# bin/state-witness.sh — the cross-check every gate runs before it gives up.
#
# WHAT THIS FILE IS FOR, and what it deliberately cannot prove. The live proof
# is in the commit: the two preserved stale panes (w4C:p9D `working` eight hours
# after it delivered, w4C:p97 `blocked/permission:child` with nothing to answer)
# against the old binary and then the new one. A fixture cannot stand in for
# that — the bug is that the gate BELIEVES agent_status, and a fixture that
# stubs agent_status is not the path.
#
# What a fixture CAN pin down is the decision table, exhaustively, including the
# combinations that did not happen to exist on the machine today: every reader
# failing in every way, and the asymmetry between "read it and it said no" and
# "could not read it". That asymmetry is the whole design and it is one typo
# away from becoming the bug it fixes.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

WITNESS_LIB="$ROOT/bin/state-witness.sh"
[ -r "$WITNESS_LIB" ] || fail "state-witness: $WITNESS_LIB is missing"

# ── the readers, stubbed; the verdict, real ──────────────────────────────────
#
# `herdr` (agent get, pane process-info), `herdr-rpc` (pane.read) and `curl`
# (/question, /permission) are the witness's three inputs. Each stub can be told
# to answer, to answer nonsense, or to fail — because "it failed" must produce a
# different verdict from "it said no", and nothing else in this suite proves it.
mkdir -p "$TMP/bin"

cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
case "$1 $2" in
  "agent get")
    printf '{"result":{"agent":{"agent":"opencode","agent_status":"%s","tokens":{"blocked_reason":"%s","blocked_scope":"%s","done_status":"%s"}}}}\n' \
      "${W_STATUS:-blocked}" "${W_REASON:-stuck}" "${W_SCOPE:-child}" "${W_DONE:-}" ;;
  "pane process-info")
    case "${W_PROCINFO:-port}" in
      fail) exit 1 ;;
      empty) printf '{"result":{"process_info":{"foreground_processes":[]}}}\n' ;;
      noport) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--model","m"]}]}}}\n' ;;
      notopencode) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["claude","--model","m"]}]}}}\n' ;;
      badport) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","not-a-number"]}]}}}\n' ;;
      abspath) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["/usr/local/bin/opencode","--port","49999"]}]}}}\n' ;;
      *) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","%s"]}]}}}\n' "${W_PORT:-49999}" ;;
    esac ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB

cat > "$TMP/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
if [ "$1 $2" = "call pane.read" ]; then
  case "${W_SCREEN:-static}" in
    fail) exit 1 ;;
    empty-output) printf '' ;;
    no-text) printf '{"read":{"pane_id":"p"}}\n' ;;
    text-not-string) printf '{"read":{"text":{"nested":true}}}\n' ;;
    unparseable) printf 'not json at all\n' ;;
    blank) printf '{"read":{"text":""}}\n' ;;
    moving)
      n=$(( $(cat "$W_TICK" 2>/dev/null || echo 0) + 1 ))
      printf '%s' "$n" > "$W_TICK"
      printf '{"read":{"text":"tick %s"}}\n' "$n" ;;
    alternate)
      # Keyed off the WAIT COUNT, so the behaviour changes once per gate round
      # rather than once per sample: round 1 reads fine (settled), round 2 fails
      # (unknown), and so on. That is the only way the streak's reset becomes
      # observable — a counter that never resets looks identical to one that
      # does until the verdicts alternate.
      if [ $(( $(cat "$W_WAITS" 2>/dev/null || echo 0) % 2 )) = 0 ]; then exit 1; fi
      printf '{"read":{"text":"an unchanging screen"}}\n' ;;
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

cat > "$TMP/bin/curl" <<'STUB'
#!/usr/bin/env bash
url="${!#}"
case "${W_ENDPOINT:-empty}" in
  fail) exit 7 ;;
  garbage) printf 'not json\n'; exit 0 ;;
  object) printf '{"data":{}}\n'; exit 0 ;;
  question) case "$url" in */question) printf '[{"id":"q"}]\n' ;; *) printf '[]\n' ;; esac ;;
  permission) case "$url" in */permission) printf '[{"id":"p"}]\n' ;; *) printf '[]\n' ;; esac ;;
  question-fails) case "$url" in */question) exit 7 ;; *) printf '[]\n' ;; esac ;;
  permission-fails) case "$url" in */permission) exit 7 ;; *) printf '[]\n' ;; esac ;;
  *) printf '[]\n' ;;
esac
STUB
chmod +x "$TMP/bin/herdr" "$TMP/bin/herdr-rpc" "$TMP/bin/curl"

# verdict <name=value ...> -- runs witness_state in a clean subshell and prints
# "<verdict>|<detail>". A subshell per case because the library caches nothing
# and must not be allowed to start.
verdict() {
  env PATH="$TMP/bin:$PATH" W_TICK="$TMP/tick" W_WAITS="$TMP/waits" \
      WITNESS_RPC="$TMP/bin/herdr-rpc" WITNESS_SAMPLES=3 WITNESS_INTERVAL_S=0 \
      "$@" bash -c '
        . "'"$WITNESS_LIB"'"
        witness_state wX:p1
        printf "%s|%s" "$WITNESS_VERDICT" "$WITNESS_DETAIL"'
}
v_only() { rm -f "$TMP/tick"; verdict "$@" | cut -d'|' -f1; }
v_detail() { rm -f "$TMP/tick"; verdict "$@" | cut -d'|' -f2-; }

# ── settled: every reader answered, and every answer was negative ────────────
[ "$(v_only W_SCREEN=static W_ENDPOINT=empty)" = settled ] \
  && pass "witness: a static screen plus an endpoint that answers no prompts is settled" \
  || fail "witness: the fully-corroborated stale case was not settled: $(v_only W_SCREEN=static W_ENDPOINT=empty)"

# A screen that is legitimately BLANK is a screen that was read. Confusing it
# with a failed read would turn a real finding into `unknown` — the same class
# of error as the reverse, and easier to write by accident.
[ "$(v_only W_SCREEN=blank W_ENDPOINT=empty)" = settled ] \
  && pass "witness: a blank-but-readable screen is a reading, not a failure" \
  || fail "witness: a blank screen was treated as unreadable: $(v_only W_SCREEN=blank W_ENDPOINT=empty)"

# ── live: something positively moved ────────────────────────────────────────
[ "$(v_only W_SCREEN=moving W_ENDPOINT=empty)" = live ] \
  && pass "witness: a screen that changes between samples is live" \
  || fail "witness: a moving screen was not live: $(v_only W_SCREEN=moving W_ENDPOINT=empty)"

# THE MEASURED CASE THIS WHOLE DESIGN TURNS ON. w4C:pAH published
# agent_status=blocked, blocked_reason=stuck, blocked_scope=child — the exact
# signature `hw unstick` accepts — while running a task it went on to finish,
# with /question and /permission both empty. A moving screen has to outrank that
# signature, or the recovery kills working executors.
[ "$(v_only W_STATUS=blocked W_REASON=stuck W_SCOPE=child W_SCREEN=moving W_ENDPOINT=empty)" = live ] \
  && pass "witness: blocked/stuck/child with a moving screen is live, not stuck (w4C:pAH)" \
  || fail "witness: the healthy-pane-publishing-stuck case was not live"

# And a moving screen must beat a `working` that is real, too — that is the
# no-regression half: a genuinely busy executor keeps its budget.
[ "$(v_only W_STATUS=working W_SCREEN=moving W_ENDPOINT=empty)" = live ] \
  && pass "witness: a genuinely working pane is live, so a real wait stays a wait" \
  || fail "witness: a working pane with a moving screen was not live"

# ── awaiting-human: a prompt was positively READ as outstanding ──────────────
[ "$(v_only W_SCREEN=static W_ENDPOINT=question)" = awaiting-human ] \
  && pass "witness: an outstanding /question is awaiting-human, never settled" \
  || fail "witness: an outstanding question was not awaiting-human: $(v_only W_SCREEN=static W_ENDPOINT=question)"
[ "$(v_only W_SCREEN=static W_ENDPOINT=permission)" = awaiting-human ] \
  && pass "witness: an outstanding /permission is awaiting-human, never settled" \
  || fail "witness: an outstanding permission was not awaiting-human"

# A real prompt outranks a moving screen: the pane may well be repainting, but
# what it needs is a person, and "live" would send the caller back to waiting.
[ "$(v_only W_SCREEN=moving W_ENDPOINT=question)" = awaiting-human ] \
  && pass "witness: a real prompt outranks a moving screen" \
  || fail "witness: a prompt behind a moving screen was misreported"

# ── unknown: an unreachable authority is NOT EVIDENCE OF ANYTHING ────────────
#
# Nine ways a reader can go quiet. Every one must be `unknown` — not live, not
# settled. This is the block that stops the fix from becoming a new instance of
# the bug it fixes.
for case_desc in \
  "screen rpc exits nonzero:W_SCREEN=fail:W_ENDPOINT=empty" \
  "screen rpc prints nothing:W_SCREEN=empty-output:W_ENDPOINT=empty" \
  "screen payload has no text field:W_SCREEN=no-text:W_ENDPOINT=empty" \
  "screen text is not a string:W_SCREEN=text-not-string:W_ENDPOINT=empty" \
  "screen payload is not json:W_SCREEN=unparseable:W_ENDPOINT=empty" \
  "endpoint curl fails:W_SCREEN=static:W_ENDPOINT=fail" \
  "endpoint returns non-json:W_SCREEN=static:W_ENDPOINT=garbage" \
  "endpoint returns an object not an array:W_SCREEN=static:W_ENDPOINT=object" \
  "only /question fails:W_SCREEN=static:W_ENDPOINT=question-fails" \
  "only /permission fails:W_SCREEN=static:W_ENDPOINT=permission-fails" \
; do
  desc="${case_desc%%:*}"; rest="${case_desc#*:}"
  a="${rest%%:*}"; b="${rest#*:}"
  got="$(v_only "$a" "$b")"
  [ "$got" = unknown ] \
    && pass "witness: $desc is unknown, not a verdict" \
    || fail "witness: $desc produced '$got' instead of unknown"
done

# The endpoint can be ABSENT rather than broken — a claude pane, or an opencode
# started without --port. Absent is still unknown: a static screen alone cannot
# tell a stale state from a pane holding a modal this check cannot see.
for pi in fail empty noport notopencode badport; do
  got="$(v_only W_SCREEN=static W_PROCINFO="$pi")"
  [ "$got" = unknown ] \
    && pass "witness: no endpoint to ask (process-info $pi) is unknown" \
    || fail "witness: process-info $pi produced '$got' instead of unknown"
done

# An opencode launched by ABSOLUTE PATH is still an opencode. Its argv[0] is
# /usr/local/bin/opencode, not the bare name, and a selector matching only the
# bare string silently downgrades every such pane to `unknown` — safe but blind,
# and this check is the only reason a caller may act at all.
[ "$(v_only W_SCREEN=static W_PROCINFO=abspath W_ENDPOINT=empty)" = settled ] \
  && pass "witness: an opencode launched by absolute path still resolves its endpoint" \
  || fail "witness: an absolute-path opencode resolved no endpoint: $(v_only W_SCREEN=static W_PROCINFO=abspath W_ENDPOINT=empty)"

# ── the diagnosis reaches the human ─────────────────────────────────────────
#
# "timed out" is what this class of failure looked like from outside for eight
# hours. A verdict that does not name its reader, its source and the
# disagreement is not an improvement on it.
d="$(v_detail W_STATUS=working W_SCREEN=static W_ENDPOINT=empty)"
case "$d" in
  *"agent_status=working"*) : ;;
  *) fail "witness: the settled detail does not name the state it contradicts: $d" ;;
esac
case "$d" in
  *"pane.read source=visible"*) : ;;
  *) fail "witness: the settled detail does not name where the screen came from: $d" ;;
esac
case "$d" in
  *"/question 0"*"/permission 0"*) : ;;
  *) fail "witness: the settled detail does not name the endpoint facts: $d" ;;
esac
case "$d" in
  *"http://127.0.0.1:49999"*) : ;;
  *) fail "witness: the settled detail does not name the endpoint it asked: $d" ;;
esac
pass "witness: a settled verdict names the state, the reader, the source and the disagreement"

d="$(v_detail W_SCREEN=fail W_ENDPOINT=empty)"
case "$d" in
  *"could NOT be made"*|*"could not be read"*) pass "witness: an unknown verdict says the cross-check could not be made" ;;
  *) fail "witness: the unknown detail does not say the check failed: $d" ;;
esac
case "$d" in
  *"not evidence"*) pass "witness: an unknown verdict refuses to be read as either answer" ;;
  *) fail "witness: the unknown detail does not disclaim both answers: $d" ;;
esac

# ── witness_wait: the gate ──────────────────────────────────────────────────
#
# `wait_for` drives the real loop with the stub rpc. W_WAIT_RC is what
# `wait-agent` returns, and $TMP/waits counts how many times it was called —
# because "did not spend the budget" is a claim about the number of slices, and
# a message alone cannot prove it.
wait_for() { # <total_ms> <env...>
  local total="$1"; shift
  rm -f "$TMP/tick" "$TMP/waits"
  local rc=0
  env PATH="$TMP/bin:$PATH" W_TICK="$TMP/tick" W_WAITS="$TMP/waits" \
      WITNESS_RPC="$TMP/bin/herdr-rpc" WITNESS_SAMPLES=2 WITNESS_INTERVAL_S=0 \
      WITNESS_SLICE_MS=1000 \
      "$@" bash -c '
        . "'"$WITNESS_LIB"'"
        witness_wait wX:p1 '"$total"'
        printf "rc=%s\n" "$?"' || rc=$?
  return $rc
}
waits() { tr -dc '0-9' < "$TMP/waits" 2>/dev/null || printf 0; }

settled_started="$(date +%s)"
out="$(wait_for 30000 W_WAIT_RC=2 W_SCREEN=static W_ENDPOINT=empty)"
settled_elapsed=$(( $(date +%s) - settled_started ))
case "$out" in
  *"rc=5"*) pass "witness_wait: a proved-settled state exits 5 instead of spending the budget" ;;
  *) fail "witness_wait: settled did not exit 5: $out" ;;
esac
# THE EXIT CODE ALONE IS NOT THE CLAIM. "Did not spend the budget" is a claim
# about wall clock, and a gate could return 5 having waited the whole time.
[ "$settled_elapsed" -lt 15 ] \
  && pass "witness_wait: it gave up in ${settled_elapsed}s, well inside the 30s budget" \
  || fail "witness_wait: it returned 5 but took ${settled_elapsed}s of a 30s budget"
# TWO CONFIRMATIONS, NOT ONE. A single 4-second window is not enough to abandon
# an executor on: a working pane can be silent that long. The streak is the
# whole safety margin, so its size is asserted, not assumed.
[ "$(waits)" = 2 ] \
  && pass "witness_wait: settled needs two consecutive confirmations, so it waits twice first" \
  || fail "witness_wait: gave up after $(waits) slices, not 2"

out="$(wait_for 900000 W_WAIT_RC=0 W_SCREEN=static W_ENDPOINT=empty)"
case "$out" in
  *"rc=0"*) pass "witness_wait: a pane that reaches idle,done opens the gate and is never witnessed" ;;
  *) fail "witness_wait: an idle pane did not open the gate: $out" ;;
esac
[ "$(waits)" = 1 ] \
  && pass "witness_wait: the normal path costs exactly one wait and no cross-check" \
  || fail "witness_wait: the idle path took $(waits) waits"

# UNKNOWN MUST COST THE CALLER NOTHING IT DID NOT ALREADY PAY. If a broken
# cross-check shortened a wait, the fix would be worse than the bug.
out="$(wait_for 3000 W_WAIT_RC=2 W_SCREEN=fail W_ENDPOINT=empty)"
case "$out" in
  *"rc=2"*) pass "witness_wait: an unknown cross-check falls back to an ordinary timeout" ;;
  *) fail "witness_wait: unknown did not time out normally: $out" ;;
esac

# A REAL WAIT IS STILL A WAIT. A live pane is re-witnessed every slice and must
# never be cut short; it can only ever end in the ordinary timeout.
out="$(wait_for 3000 W_WAIT_RC=2 W_SCREEN=moving W_ENDPOINT=empty)"
case "$out" in
  *"rc=2"*) pass "witness_wait: a live pane keeps its budget and ends in a plain timeout, never exit 5" ;;
  *) fail "witness_wait: a live pane was cut short: $out" ;;
esac

# AWAITING-HUMAN IS ITS OWN OUTCOME — 6, never 5 and no longer 2.
#
# This arm used to assert rc=2, pinning the behaviour that a positively-read
# outstanding prompt was folded in with `unknown` and cost the caller its whole
# budget. That was the gap: the verdict was computed, was positive, and was
# discarded, so a gate that had just had a pane's own endpoint CONFIRM a
# question told its caller 540s later that the pane "was still working" —
# advice to retry, for a state only a person can move.
#
# What the old arm was protecting is still protected, and by the same line: 6 is
# not 5. Collapsing the two would send a caller to "look at that pane and decide
# what it needs" when the pane has already said what it needs.
out="$(wait_for $((3000 * HW_TEST_SLOW)) W_WAIT_RC=2 W_SCREEN=static W_ENDPOINT=question)"
case "$out" in
  *"rc=6"*) pass "witness_wait: a pane awaiting a person exits 6, its own code" ;;
  *"rc=5"*) fail "witness_wait: awaiting-human was collapsed into settled: $out" ;;
  *) fail "witness_wait: awaiting-human did not exit 6: $out" ;;
esac
out="$(wait_for $((3000 * HW_TEST_SLOW)) W_WAIT_RC=2 W_SCREEN=static W_ENDPOINT=permission)"
case "$out" in
  *"rc=6"*) pass "witness_wait: an outstanding /permission exits 6 as well as /question" ;;
  *) fail "witness_wait: an outstanding permission did not exit 6: $out" ;;
esac

# SAME BAR AS settled: two CONSECUTIVE confirmations. One 4-second window is not
# enough to refuse on — a person may answer while this is watching — so the
# streak size is asserted rather than assumed, exactly as it is for settled.
[ "$(waits)" = 2 ] \
  && pass "witness_wait: awaiting-human needs two consecutive confirmations too" \
  || fail "witness_wait: awaiting-human refused after $(waits) slices, not 2"

# AND THE TWO STREAKS MUST NOT ADD UP. A pane that alternates between a real
# prompt and a proved-settled state has never been either thing twice in a row.
# With one shared counter it reaches a refusal anyway — and reports whichever
# verdict happened to land last, which is a fact it never established.
cat > "$TMP/bin/curl.alt" <<'STUB'
#!/usr/bin/env bash
url="${!#}"
n="$(cat "$W_WAITS" 2>/dev/null || echo 0)"
if [ $(( n % 2 )) = 0 ]; then printf '[]\n'; exit 0; fi
case "$url" in */question) printf '[{"id":"q"}]\n' ;; *) printf '[]\n' ;; esac
STUB
chmod +x "$TMP/bin/curl.alt"
cp "$TMP/bin/curl" "$TMP/bin/curl.plain"
cp "$TMP/bin/curl.alt" "$TMP/bin/curl"
out="$(wait_for 8000 W_WAIT_RC=2 W_SCREEN=static W_ENDPOINT=empty)"
cp "$TMP/bin/curl.plain" "$TMP/bin/curl"
case "$out" in
  *"rc=2"*) pass "witness_wait: alternating settled/awaiting-human never accumulates into a refusal" ;;
  *) fail "witness_wait: two different verdicts were added into one streak: $out" ;;
esac

# A STREAK MUST RESET, or `settled` accumulates across verdicts that were never
# settled. Here the rounds alternate settled / unknown forever: two settled
# verdicts never land consecutively, so exit 5 must never happen. Without the
# reset this reaches 5 on the third round and abandons a pane on one real
# observation and one failed read.
out="$(wait_for 8000 W_WAIT_RC=2 W_SCREEN=alternate W_ENDPOINT=empty)"
case "$out" in
  *"rc=2"*) pass "witness_wait: alternating verdicts never accumulate into a settled refusal" ;;
  *) fail "witness_wait: a non-consecutive streak reached a refusal: $out" ;;
esac

# ONLY A TIMEOUT IS WORTH ANOTHER SLICE. herdr-rpc exits 3 when it cannot
# connect and 4 when the pane is gone. Looping on those would spin as fast as
# the socket can refuse and burn the whole budget in a hot loop, and would
# return a code the caller could no longer tell from a real timeout.
for rc_in in 3 4; do
  out="$(wait_for 900000 W_WAIT_RC="$rc_in" W_SCREEN=static W_ENDPOINT=empty || true)"
  case "$out" in
    *"rc=$rc_in"*) pass "witness_wait: wait-agent exit $rc_in is passed straight back, not retried" ;;
    *) fail "witness_wait: exit $rc_in was not preserved: $out" ;;
  esac
  [ "$(waits)" = 1 ] \
    && pass "witness_wait: exit $rc_in does not become a hot loop" \
    || fail "witness_wait: exit $rc_in looped $(waits) times"
done

# A budget at or below one slice is a caller that has already decided not to
# wait long; it must not have witness time added on top of it.
out="$(wait_for 1 W_WAIT_RC=2 W_SCREEN=static W_ENDPOINT=empty)"
case "$out" in
  *"rc=2"*) pass "witness_wait: a sub-slice budget stays a single plain wait" ;;
  *) fail "witness_wait: a tiny budget was not a plain wait: $out" ;;
esac
[ "$(waits)" = 1 ] \
  && pass "witness_wait: a sub-slice budget costs exactly one wait" \
  || fail "witness_wait: a tiny budget took $(waits) waits"

exit 0
