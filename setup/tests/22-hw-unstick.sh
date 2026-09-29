#!/usr/bin/env bash
# Drive cmd_unstick against a Herdr-shaped process. The live owned probe proves
# terminal behavior; this covers every refusal and byte-preserving restart argv.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HW_UNSTICK_BIN="${HW_UNSTICK_BIN:-$ROOT/bin/hw}"
fixture_lock_dir=""
cleanup_fixture_lock() {
  local rc=$?
  [ -z "$fixture_lock_dir" ] || rm -rf "$fixture_lock_dir" || true
  rm -rf "$TMP" || true
  trap - EXIT
  exit "$rc"
}
trap cleanup_fixture_lock EXIT
awk '/^cmd_unstick\(\) \(/,/^\)/' "$HW_UNSTICK_BIN" > "$TMP/unstick.sh"
[ -s "$TMP/unstick.sh" ] || fail "hw unstick: cmd_unstick was not found"

cat > "$TMP/herdr" <<'STUB'
#!/usr/bin/env bash
log="${STUB_LOG:?}"; kind="${STUB_KIND:-stuck}"; pane="${STUB_PANE:-wX:p1}"
agent_get() {
  # OPTIONAL, absent by default, so every pre-existing kind is unchanged: the
  # session id and cwd `hw unstick` records before it disconnects a
  # conversation. A real stuck opencode pane may publish either, both or
  # neither — w3Z:p1T was measured blocked with agent_session null on
  # 2026-08-26, which is why absence is a case and not an error.
  local sess='' cwd='' run_tail='' run_object='' active_session="${STUB_SESSION:-}"
  [ -f "$STUB_RESTARTED" ] && active_session="${STUB_AFTER_SESSION-${STUB_SESSION:-}}"
  [ -n "$active_session" ] && sess=',"agent_session":{"agent":"opencode","kind":"id","value":"'"$active_session"'"}'
  [ -n "${STUB_CWD:-}" ] && cwd=',"cwd":"'"$STUB_CWD"'"'
  if [ -n "${STUB_RUN:-}" ]; then
    run_tail=',"hw_run":"'"$STUB_RUN"'"'
    run_object='"hw_run":"'"$STUB_RUN"'"'
  fi
  if [ -f "$STUB_RESTARTED" ]; then
    printf '{"result":{"agent":{"agent":"opencode","agent_status":"%s","tokens":{%s}%s%s}}}\n' "${STUB_AFTER:-idle}" "$run_object" "$sess" "$cwd"
  elif [ "$kind" = recheck-change ] && [ -f "$STUB_CAPTURED" ]; then
    printf '{"result":{"agent":{"agent":"opencode","agent_status":"working","tokens":{"blocked_reason":"stuck","blocked_scope":"child"}}}}\n'
  else
    case "$kind" in
      nonblocked) printf '{"result":{"agent":{"agent":"opencode","agent_status":"working","tokens":{"blocked_reason":"stuck","blocked_scope":"child"}}}}\n' ;;
      question) printf '{"result":{"agent":{"agent":"opencode","agent_status":"blocked","tokens":{"blocked_reason":"question","blocked_scope":"child"}}}}\n' ;;
      permission) printf '{"result":{"agent":{"agent":"opencode","agent_status":"blocked","tokens":{"blocked_reason":"permission","blocked_scope":"root"}}}}\n' ;;
      error) printf '{"result":{"agent":{"agent":"opencode","agent_status":"blocked","tokens":{"blocked_reason":"error","blocked_scope":"root"}}}}\n' ;;
      missing-scope) printf '{"result":{"agent":{"agent":"opencode","agent_status":"blocked","tokens":{"blocked_reason":"stuck"}}}}\n' ;;
      invalid-scope) printf '{"result":{"agent":{"agent":"opencode","agent_status":"blocked","tokens":{"blocked_reason":"stuck","blocked_scope":"sideways"}}}}\n' ;;
      root) printf '{"result":{"agent":{"agent":"opencode","agent_status":"blocked","tokens":{"blocked_reason":"stuck","blocked_scope":"root"%s}%s%s}}}\n' "$run_tail" "$sess" "$cwd" ;;
      *) printf '{"result":{"agent":{"agent":"opencode","agent_status":"blocked","tokens":{"blocked_reason":"stuck","blocked_scope":"child"%s}%s%s}}}\n' "$run_tail" "$sess" "$cwd" ;;
    esac
  fi
}
case "$1 $2" in
  "agent get") agent_get ;;
   "agent list")
     case "$kind" in
       name-collision) printf '{"result":{"agents":[{"name":"%s","pane_id":"wOther:p1"}]}}\n' "$STUB_EXPECT_NAME" ;;
       malformed-list) printf '{"result":{"agents":{}}}\n' ;;
       unparseable-list) printf 'not json\n' ;;
       *) printf '{"result":{"agents":[]}}\n' ;;
     esac ;;
  "agent send-keys") printf 'keys:%s\n' "$*" >> "$log"; case " $* " in *" ctrl+c "*) : > "$STUB_STOPPED" ;; esac; printf '{"type":"ok"}\n' ;;
  "agent read")
    reads=0; [ -f "$STUB_READS" ] && reads="$(cat "$STUB_READS")"; reads=$((reads + 1)); printf '%s' "$reads" > "$STUB_READS"
    case "$kind" in
      screen-unreadable) exit 1 ;;
      working-subagents) printf 'ctrl+x down view subagents\n' ;;
      generic-enter) printf 'enter submit\n' ;;
      generic-deny) printf 'Deny\n' ;;
      generic-allow-once) printf 'Allow once\n' ;;
      generic-always-allow) printf 'Always allow\n' ;;
      live-question) printf 'esc dismiss\n' ;;
      live-permission-once) printf 'Allow once\nDeny\n' ;;
      live-permission-always) printf 'Always allow\nDeny\n' ;;
      final-question) [ "$reads" -lt 2 ] || printf 'esc dismiss\n' ;;
      final-permission) [ "$reads" -lt 2 ] || printf 'Allow once\nDeny\n' ;;
    esac ;;
  "pane process-info")
    if [ -f "$STUB_STOPPED" ]; then
      case "$kind" in
        stop-read-error) exit 1 ;;
        stop-malformed) printf 'not json\n' ;;
        stop-missing-list) printf '{"result":{"process_info":{}}}\n' ;;
        stop-invalid-argv) printf '{"result":{"process_info":{"foreground_processes":[{"argv":null}]}}}\n' ;;
        stop-absolute-live) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["/usr/local/bin/opencode"]}]}}}\n' ;;
        *) printf '{"result":{"process_info":{"foreground_processes":[{"argv":["-zsh"]}]}}}\n' ;;
      esac
    elif [ "$kind" = bad-wrapper ]; then : > "$STUB_CAPTURED"; printf '{"result":{"process_info":{"foreground_processes":[{"argv":["env","opencode","--model","x","--port","57231"]}]}}}\n'
    elif [ "$kind" = whitespace ]; then : > "$STUB_CAPTURED"; printf '{"result":{"process_info":{"foreground_processes":[{"argv":["/usr/local/bin/opencode","--title","two words","--prompt","line one\\nline two","--port","57231"]}]}}}\n'
    elif [ "$kind" = existing-session ]; then : > "$STUB_CAPTURED"; printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--session","ses_OLD","--model","openai/gpt-5.6-luna","--port","57231"]}]}}}\n'
    elif [ "$kind" = existing-continue ]; then : > "$STUB_CAPTURED"; printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--continue","--model","openai/gpt-5.6-luna","--port","57231"]}]}}}\n'
    else : > "$STUB_CAPTURED"; printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--model","openai/gpt-5.6-luna","--auto","--port","57231"]}]}}}\n'; fi ;;
  "agent start") python3 - "$STUB_ARG_LOG" "$@" <<'PY'
import json, sys
open(sys.argv[1], 'w').write(json.dumps(sys.argv[2:]))
PY
                 : > "$STUB_RESTARTED"; printf '{"result":{"agent":{}}}\n' ;;
  "pane report-metadata") printf 'metadata:%s\n' "$*" >> "$log"; printf '{"type":"ok"}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
chmod +x "$TMP/herdr"

# THE WITNESS IS NOT STUBBED — IT IS DRIVEN.
#
# cmd_unstick now refuses unless an independent cross-check positively confirms
# nothing is moving and nothing is outstanding, because the token signature it
# used to trust is one a HEALTHY pane can publish (measured 2026-08-27,
# w4C:pAH: blocked/stuck/child while visibly running a task it went on to
# finish). Stubbing `witness_state` out would make that refusal unfalsifiable
# and this file would go green on a binary that had lost the check entirely.
#
# So the real bin/state-witness.sh is sourced, and its two readers are given
# stubs instead: pane.read for the screen, curl for the endpoint. STUB_WITNESS
# selects what they report. It defaults to a static screen and empty prompt
# arrays — `settled` — so every pre-existing case above still recovers.
cat > "$TMP/herdr-rpc" <<'RPCSTUB'
#!/usr/bin/env bash
# `call pane.read` only; anything else is not this stub's business.
[ "$1 $2" = "call pane.read" ] || { printf '{"result":{}}
'; exit 0; }
case "${STUB_WITNESS:-settled}" in
  unknown-screen) exit 1 ;;
  screen-no-text) printf '{"read":{"pane_id":"x"}}\n' ;;
  live)
    # A different body on every call: this is what a working pane looks like to
    # a hash-of-the-screen witness.
    n=$(( $(cat "$STUB_WITNESS_TICK" 2>/dev/null || echo 0) + 1 ))
    printf '%s' "$n" > "$STUB_WITNESS_TICK"
    printf '{"read":{"text":"working, tick %s"}}\n' "$n" ;;
  live-at-second-check)
    # SETTLED at the first cross-check, LIVE at the second. WITNESS_SAMPLES is 2
    # here, so reads 1-2 belong to the check that runs before any keystroke and
    # reads 3+ to the one immediately before the stop. Keyed on the read COUNT,
    # not on $STUB_CAPTURED: the first cross-check calls process-info itself to
    # resolve the endpoint, so it trips the capture marker before the argv
    # capture ever happens. Nothing else in this file can tell the two checks
    # apart, and without that a binary keeping only the first looks identical to
    # one keeping both.
    n=$(( $(cat "$STUB_WITNESS_TICK" 2>/dev/null || echo 0) + 1 ))
    printf '%s' "$n" > "$STUB_WITNESS_TICK"
    if [ "$n" -le 2 ]; then printf '{"read":{"text":"a screen that does not change"}}\n'
    else printf '{"read":{"text":"now working, tick %s"}}\n' "$n"; fi ;;
  *) printf '{"read":{"text":"a screen that does not change"}}\n' ;;
esac
RPCSTUB
chmod +x "$TMP/herdr-rpc"

cat > "$TMP/curl" <<'CURLSTUB'
#!/usr/bin/env bash
url="${!#}"
case "${STUB_WITNESS:-settled}" in
  unknown-endpoint) exit 7 ;;
  endpoint-garbage) printf 'not json
'; exit 0 ;;
  endpoint-not-array) printf '{"data":{}}
'; exit 0 ;;
esac
case "$url" in
  */question)
    case "${STUB_WITNESS:-settled}" in
      awaiting-question) printf '[{"id":"q1"}]
' ;;
      *) printf '[]
' ;;
    esac ;;
  */permission)
    case "${STUB_WITNESS:-settled}" in
      awaiting-permission) printf '[{"id":"p1"}]
' ;;
      *) printf '[]
' ;;
    esac ;;
  *) printf '[]
' ;;
esac
CURLSTUB
chmod +x "$TMP/curl"

run_unstick() { # kind [pane] [after]
  : > "$TMP/log"; rm -f "$TMP/stopped" "$TMP/restarted" "$TMP/captured" "$TMP/argv.json" "$TMP/receipts.jsonl" "$TMP/reads" "$TMP/witness-tick"
  mkdir -p "$TMP/locks"
  local pane="${2:-wX:p1}"
  local name="unstick-$(printf '%s' "$pane" | shasum -a 256 | cut -c1-24)"
  # `warn`, `_receipt_into` and `_next_run_dir` are stubbed for the same reason
  # `die`/`info`/`ok` already are: cmd_unstick is extracted from bin/hw and driven
  # alone, so anything it calls from the surrounding file has to be supplied here.
  # `_receipt_into` writes a real line to a real file, because "what did it record
  # before it disconnected the conversation" is the claim under test — a no-op
  # stub would make every session assertion below unfalsifiable.
  env PATH="$TMP:$PATH" TMPDIR="$TMP/locks" STUB_KIND="$1" STUB_PANE="$pane" STUB_AFTER="${3:-idle}" \
    STUB_LOG="$TMP/log" STUB_STOPPED="$TMP/stopped" STUB_RESTARTED="$TMP/restarted" \
    STUB_CAPTURED="$TMP/captured" STUB_ARG_LOG="$TMP/argv.json" STUB_EXPECT_NAME="$name" STUB_READS="$TMP/reads" \
    STUB_SESSION="${STUB_SESSION:-}" STUB_AFTER_SESSION="${STUB_AFTER_SESSION-${STUB_SESSION:-}}" STUB_CWD="${STUB_CWD:-}" STUB_RUN="${STUB_RUN:-}" RECEIPTS="$TMP/receipts.jsonl" \
    RUNDIR="${RUNDIR:-}" EXPECT_CWD="${STUB_CWD:-}" \
    STUB_WITNESS="${STUB_WITNESS:-settled}" STUB_WITNESS_TICK="$TMP/witness-tick" \
    WITNESS_SAMPLES=2 WITNESS_INTERVAL_S=0 WITNESS_LIB="$ROOT/bin/state-witness.sh" \
    WITNESS_STUB_RPC="$TMP/herdr-rpc" \
    bash -c 'die() { printf "%s\n" "$1" >&2; exit 1; }; info(){ printf "%s\n" "$*"; }; ok(){ printf "%s\n" "$*"; }
             warn(){ printf "WARN:%s\n" "$*"; }; sleep(){ :; }
             _receipt_into(){ printf "%s\t%s\t%s\t%s\n" "$1" "$2" "$3" "${4:-}" >> "$RECEIPTS"; }
             _next_run_dir(){ [ "$1" = "$EXPECT_CWD" ] || return 1
                              [ -n "$RUNDIR" ] && printf "%s" "$RUNDIR" || return 1; }
             _load_state_witness(){ WITNESS_RPC="$WITNESS_STUB_RPC"; . "$WITNESS_LIB"; }
             source "'$TMP'/unstick.sh"; cmd_unstick "$1"' _ "$pane" 2>&1
}

refuses_without_keys() { # kind needle
  local out; out="$(run_unstick "$1" || true)"
  case "$out" in *"$2"*) pass "hw unstick: $1 is refused" ;; *) fail "hw unstick: $1 was not refused: $out" ;; esac
  [ ! -s "$TMP/log" ] && pass "hw unstick: $1 refusal sends no control keys" || fail "hw unstick: $1 touched pane: $(cat "$TMP/log")"
}
refuses_without_keys nonblocked 'not blocked'
refuses_without_keys question 'not stuck'
refuses_without_keys permission 'not stuck'
refuses_without_keys error 'not stuck'
refuses_without_keys missing-scope 'required scope metadata'
refuses_without_keys invalid-scope 'required scope metadata'

for kind in stop-read-error stop-malformed stop-missing-list stop-invalid-argv stop-absolute-live; do
  rc=0; out="$(run_unstick "$kind")" || rc=$?
  [ "$rc" != 0 ] || fail "hw unstick restarted without observing process exit ($kind): $out"
  [ ! -e "$TMP/restarted" ] || fail "hw unstick started a second process ($kind)"
  case "$out" in *"no second process was started"*) ;; *) fail "hw unstick failed for an unrelated reason ($kind): $out" ;; esac
  pass "hw unstick: $kind cannot authorize a second OpenCode process"
done

for kind in working-subagents generic-enter generic-deny generic-allow-once generic-always-allow; do
  out="$(run_unstick "$kind")"
  case "$out" in *'RECOVERED:'*) pass "hw unstick: $kind visible text is not prompt evidence at either read gate" ;;
    *) fail "hw unstick: $kind caused a false refusal: $out" ;; esac
done

prompt_refusal() { # kind exact evidence
  local out
  out="$(run_unstick "$1" || true)"
  case "$out" in *"$2"*) pass "hw unstick: $1 is refused with exact evidence" ;;
    *) fail "hw unstick: $1 did not name matched evidence '$2': $out" ;; esac
  ! grep -q 'ctrl+c' "$TMP/log" && pass "hw unstick: $1 refusal precedes Ctrl-C" \
    || fail "hw unstick: $1 reached Ctrl-C"
}
prompt_refusal live-question 'question footer: esc dismiss'
prompt_refusal live-permission-once 'permission choices: Allow once + Deny'
prompt_refusal live-permission-always 'permission choices: Always allow + Deny'
prompt_refusal final-question 'question footer: esc dismiss'
prompt_refusal final-permission 'permission choices: Allow once + Deny'

out="$(run_unstick screen-unreadable || true)"
case "$out" in *'visible screen could not be read'*'fails closed'*) pass "hw unstick: unreadable screen fails closed" ;;
  *) fail "hw unstick: unreadable screen did not fail closed: $out" ;; esac
! grep -q 'ctrl+c' "$TMP/log" && pass "hw unstick: unreadable screen refusal precedes Ctrl-C" || fail "hw unstick: unreadable screen reached Ctrl-C"

out="$(run_unstick bad-wrapper || true)"
case "$out" in *"refuses wrappers"*) pass "hw unstick: wrapper argv is refused" ;; *) fail "hw unstick: wrapper was accepted: $out" ;; esac
! grep -q 'ctrl+c' "$TMP/log" && pass "hw unstick: wrapper refusal precedes Ctrl-C" || fail "hw unstick: wrapper reached Ctrl-C"

out="$(run_unstick recheck-change || true)"
case "$out" in *"changed after argv capture"*) pass "hw unstick: post-capture state recheck blocks termination" ;; *) fail "hw unstick: stale state reached restart: $out" ;; esac
! grep -q 'ctrl+c' "$TMP/log" && pass "hw unstick: recheck is before Ctrl-C" || fail "hw unstick: Ctrl-C preceded recheck"

# ── the token signature is necessary and NO LONGER SUFFICIENT ────────────────
#
# Property under test: `hw unstick` must stop accepting a signature a healthy
# pane can publish. Measured 2026-08-27 — w4C:pAH was publishing
# agent_status=blocked, blocked_reason=stuck, blocked_scope=child, with
# /question and /permission both empty, while running a task it went on to
# finish 22 minutes later; w4C:p9W did the same three minutes after dispatch.
# Every check that existed before today accepts both of those, and what it does
# next is kill an OpenCode process and replace a live conversation.
#
# So each case below feeds the STUCK TOKENS — the accepted signature, unchanged
# — and varies only what the independent cross-check finds. A refusal has to
# come from the cross-check alone.
#
# `precedes ctrl+x`, not just ctrl+c: for child scope the old code's first act
# was `ctrl+x down` on somebody's live session to bring the modal into view. A
# refusal that has already typed into a working pane is not a refusal.
witness_refusal() { # witness-kind needle label
  local out
  out="$(STUB_WITNESS="$1" run_unstick stuck || true)"
  case "$out" in *"$2"*) pass "hw unstick: $3 is refused on the cross-check, not the token" ;;
    *) fail "hw unstick: $3 was not refused with '$2': $out" ;; esac
  [ ! -s "$TMP/log" ]     && pass "hw unstick: $3 refusal sends no keys at all — not even child navigation"     || fail "hw unstick: $3 touched the pane: $(cat "$TMP/log")"
}
witness_refusal live               'it is WORKING'            'a working pane publishing blocked/stuck'
witness_refusal awaiting-question  'WAITING FOR A PERSON'     'an outstanding question behind a stuck token'
witness_refusal awaiting-permission 'WAITING FOR A PERSON'    'an outstanding permission behind a stuck token'

# UNKNOWN IS A REFUSAL TOO, and that is the deliberate part. This command's
# mistake is unrecoverable, so the absence of evidence that something is running
# is not evidence that a restart is safe. Fail closed, and name the reader that
# went quiet.
witness_refusal unknown-screen     'could NOT be corroborated' 'a screen that cannot be read'
witness_refusal unknown-endpoint   'could NOT be corroborated' 'an endpoint that does not answer'
witness_refusal endpoint-garbage   'could NOT be corroborated' 'an endpoint answering non-json'
witness_refusal endpoint-not-array 'could NOT be corroborated' 'an endpoint answering the wrong shape'
witness_refusal screen-no-text     'could NOT be corroborated' 'a pane.read payload with no text field'

# CAPTURE IS NOT AUTHORITY — the same rule the token re-read already follows,
# applied to the cross-check. The first check happens before the subagent
# navigation and the argv capture: seconds and several keystrokes earlier. A pane
# that woke up in between must not be killed on a stale reading, and the only
# thing standing between it and a Ctrl-C is the second check.
out="$(STUB_WITNESS=live-at-second-check run_unstick stuck || true)"
case "$out" in
  *"started WORKING between the first cross-check and the restart"*)
    pass "hw unstick: a pane that wakes up after the first cross-check is re-checked before the stop" ;;
  *) fail "hw unstick: the pre-stop re-check did not catch a pane that woke up: $out" ;;
esac
! grep -q 'ctrl+c' "$TMP/log" \
  && pass "hw unstick: the pre-stop re-check refusal precedes Ctrl-C" \
  || fail "hw unstick: a pane that woke up still reached Ctrl-C"
case "$out" in
  *'RECOVERED:'*) fail "hw unstick: it claimed a recovery on a pane it had just found working: $out" ;;
  # The preceding arm positively observed the pre-stop working verdict; RECOVERED is the exhaustive contradictory claim.
  *) pass "hw unstick: no recovery is claimed for a pane found working before the stop" ;;
esac

# The refusals must name WHICH reader went quiet, or the operator is left with
# the same "it timed out" that made this class invisible for eight hours.
out="$(STUB_WITNESS=unknown-endpoint run_unstick stuck || true)"
case "$out" in *'/question and /permission'*) pass "hw unstick: an unknown refusal names the reader that failed" ;;
  *) fail "hw unstick: the unknown refusal does not name its failed reader: $out" ;; esac

# And the corroborated case still recovers — the check is a gate, not a veto.
out="$(STUB_WITNESS=settled run_unstick stuck)"
case "$out" in *'RECOVERED:'*) pass "hw unstick: a cross-check that confirms stuck still recovers" ;;
  *) fail "hw unstick: a confirmed stuck pane did not recover: $out" ;; esac
case "$out" in *'cross-check CONFIRMS stuck'*) pass "hw unstick: the recovery says the cross-check confirmed it" ;;
  *) fail "hw unstick: recovery did not report its cross-check: $out" ;; esac

out="$(run_unstick root)"; case "$out" in *'RECOVERED:'*) pass "hw unstick: root scope recovers without child navigation" ;; *) fail "hw unstick: root scope did not recover: $out" ;; esac
! grep -q 'ctrl+x' "$TMP/log" && pass "hw unstick: root scope does not navigate child view" || fail "hw unstick: root sent child navigation"

out="$(run_unstick stuck wX:p1 working)"; case "$out" in *'reports working'*) pass "hw unstick: working is accepted after restart" ;; *) fail "hw unstick: working restart rejected: $out" ;; esac
out="$(run_unstick stuck wX:p1 blocked || true)"; case "$out" in *'not idle/working'*) pass "hw unstick: blocked post-restart state is not claimed recovered" ;; *) fail "hw unstick: blocked post-restart state was accepted: $out" ;; esac

out="$(STUB_SESSION=ses_WS run_unstick whitespace)"; case "$out" in *'RECOVERED:'*) pass "hw unstick: direct executable argv restarts" ;; *) fail "hw unstick: whitespace argv did not restart: $out" ;; esac
jq -e 'index("two words") and index("line one\nline two")' "$TMP/argv.json" >/dev/null && pass "hw unstick: argv whitespace and newline survive NUL transfer" || fail "hw unstick: argv was mangled: $(cat "$TMP/argv.json")"

jq -e '[.[] | select(. == "--session")] | length == 1' "$TMP/argv.json" >/dev/null \
  && jq -e 'index("ses_WS")' "$TMP/argv.json" >/dev/null \
  || fail "hw unstick: outgoing session was not selected deterministically: $(cat "$TMP/argv.json")"

run_unstick stuck >/dev/null
jq -e 'index("--session") | not' "$TMP/argv.json" >/dev/null \
  && jq -e 'index("--model") and index("openai/gpt-5.6-luna") and index("--auto") and index("--port") and index("57231")' "$TMP/argv.json" >/dev/null \
  || fail "hw unstick: no-session restart argv changed: $(cat "$TMP/argv.json")"

fallback_wd="$TMP/fallback-wd"
fallback_run="$fallback_wd/.hw/run1"
mkdir -p "$fallback_run"
printf '%s\n' \
  '{"key":"pane","value":"wX:p1"}' \
  '{"key":"session_vendor","value":"opencode"}' \
  '{"key":"session_id","value":"ses_RECEIPT"}' > "$fallback_run/receipt.jsonl"
STUB_CWD="$fallback_wd" STUB_RUN=run1 STUB_AFTER_SESSION=ses_RECEIPT run_unstick root >/dev/null
jq -e 'index("--session") and index("ses_RECEIPT")' "$TMP/argv.json" >/dev/null \
  || fail "hw unstick: matching pane/vendor receipt did not recover the session id: $(cat "$TMP/argv.json")"

printf '%s\n' \
  '{"key":"pane","value":"wOther:p1"}' \
  '{"key":"session_vendor","value":"opencode"}' \
  '{"key":"session_id","value":"ses_WRONG_PANE"}' > "$fallback_run/receipt.jsonl"
STUB_CWD="$fallback_wd" STUB_RUN=run1 run_unstick root >/dev/null
jq -e 'index("--session") | not' "$TMP/argv.json" >/dev/null \
  || fail "hw unstick: a receipt for another pane was trusted: $(cat "$TMP/argv.json")"

for kind in existing-session existing-continue; do
  STUB_SESSION=ses_OUT run_unstick "$kind" >/dev/null
  jq -e '[.[] | select(. == "--session")] | length == 1' "$TMP/argv.json" >/dev/null \
    && jq -e 'index("ses_OLD") | not' "$TMP/argv.json" >/dev/null \
    && jq -e 'index("--continue") | not' "$TMP/argv.json" >/dev/null \
    && jq -e 'index("ses_OUT")' "$TMP/argv.json" >/dev/null \
    || fail "hw unstick: $kind conflicted with exact reattachment: $(cat "$TMP/argv.json")"
done

out="$(STUB_SESSION=ses_OUT STUB_AFTER_SESSION=ses_WRONG run_unstick root || true)"
case "$out" in *'reports session ses_WRONG, not captured session ses_OUT'*'NOT cleared'*)
    : ;;
  *) fail "hw unstick: mismatched restarted session was accepted: $out" ;;
esac
! grep -q 'report-metadata' "$TMP/log" || fail "hw unstick: mismatch cleared stuck metadata: $(cat "$TMP/log")"

run_unstick stuck wA:p1 >/dev/null; name_a="$(jq -r '.[2]' "$TMP/argv.json")"
run_unstick stuck wB:p1 >/dev/null; name_b="$(jq -r '.[2]' "$TMP/argv.json")"
[ "$name_a" != "$name_b" ] && [ "${#name_a}" -le 32 ] && [ "${#name_b}" -le 32 ] && pass "hw unstick: full pane ids with same suffix derive distinct valid names" || fail "hw unstick: restart names collide: $name_a / $name_b"
out="$(run_unstick name-collision || true)"; case "$out" in *'already owned by another pane'*) pass "hw unstick: existing global restart name is checked before stop" ;; *) fail "hw unstick: name collision not refused: $out" ;; esac
! grep -q 'ctrl+c' "$TMP/log" && pass "hw unstick: name collision precedes Ctrl-C" || fail "hw unstick: name collision reached Ctrl-C"

for kind in malformed-list unparseable-list; do
  out="$(run_unstick "$kind" || true)"
  case "$out" in *'registry was malformed'*) pass "hw unstick: $kind registry is refused structurally" ;; *) fail "hw unstick: $kind registry was accepted: $out" ;; esac
  ! grep -q 'ctrl+c' "$TMP/log" && pass "hw unstick: $kind refusal precedes Ctrl-C" || fail "hw unstick: $kind registry reached Ctrl-C"
done

pane="wLocked:fixture-$$"; lock_name="$(printf '%s' "$pane" | shasum -a 256 | cut -c1-24)"
fixture_lock_dir="/tmp/hw-unstick-$(id -u)-${lock_name}.lock"
mkdir "$fixture_lock_dir"
printf '%s %s\n' "$$" "$(id -u)" > "$fixture_lock_dir/owner"
out="$(run_unstick stuck "$pane" || true)"
case "$out" in *'recovery in progress'*) pass "hw unstick: different TMPDIR callers contend on the same pane lock" ;; *) fail "hw unstick: different TMPDIR lock was accepted: $out" ;; esac
! grep -q 'ctrl+c' "$TMP/log" && pass "hw unstick: shared same-pane lock precedes Ctrl-C" || fail "hw unstick: shared lock reached Ctrl-C"
rm -rf "$fixture_lock_dir"

mkdir "$fixture_lock_dir"
printf '%s %s\n' '999999' "$(id -u)" > "$fixture_lock_dir/owner"
out="$(run_unstick stuck "$pane")"
case "$out" in *'RECOVERED:'*) pass "hw unstick: proven-dead same-user lock is reclaimed" ;; *) fail "hw unstick: stale lock was not reclaimed: $out" ;; esac
[ ! -e "$fixture_lock_dir" ] && pass "hw unstick: recovery releases its lock after metadata clearing" || fail "hw unstick: recovery left lock behind"

# ── the conversation it replaces is named before it is replaced ─────────────
#
# MEASURED 2026-08-26: `opencode export ses_fc0bac363ffeHqKj657cIJEair` returned
# the full transcript of a session whose pane had been restarted hours earlier.
# Replaced is not deleted — unaddressable was the whole of the loss, and this
# command was destroying the only handle that reaches it. So the id is read and
# recorded BEFORE Ctrl-C: after the restart there is nothing left to read it
# from.
recorded_session() { rg -N '\tunstick_(reattached|replaced)_session\t' "$TMP/receipts.jsonl" 2>/dev/null | tail -1 || true; }

out="$(STUB_SESSION=ses_OUT STUB_CWD=/tmp/wd RUNDIR="$TMP/run" run_unstick root)"
line="$(recorded_session)"
[ -n "$line" ] || fail "hw unstick: the replaced session was never recorded: $out"
case "$line" in
  "$TMP/run"$'\t'unstick_reattached_session$'\t'ses_OUT$'\t'*"before ctrl+c"*)
    pass "hw unstick: the outgoing session id is recorded into the run it belongs to, sourced to a read before Ctrl-C" ;;
  *) fail "hw unstick: wrong record: $line" ;;
esac
case "$out" in *"REATTACHING conversation ses_OUT"*) pass "hw unstick: the outgoing session is named in the output, not only filed" ;;
  *) fail "hw unstick: output does not name the reattached conversation: $out" ;; esac
case "$out" in *"opencode export ses_OUT"*) pass "hw unstick: the output carries the command that still reads the replaced transcript" ;;
  *) fail "hw unstick: no way to read the replaced transcript was given: $out" ;; esac
case "$out" in *"persisted transcript stays readable"*) pass "hw unstick: says the conversation survives the process restart" ;;
  *) fail "hw unstick: still implies the conversation is gone: $out" ;; esac
# ORDER IS THE WHOLE CLAIM. Recorded before the keys, or it is recorded from a
# process that no longer exists.
before_keys="$(rg -n 'unstick_reattached_session' "$TMP/receipts.jsonl" >/dev/null 2>&1 && echo yes || echo no)"
[ "$before_keys" = yes ] && grep -q 'ctrl+c' "$TMP/log" \
  && pass "hw unstick: the record exists and Ctrl-C was still sent — the two are not exclusive" \
  || fail "hw unstick: record/Ctrl-C ordering broke"

# The id is repeated after the 15s process wait, because the pre-restart line
# has scrolled by then and this id is the only route to the replaced work.
case "$out" in *"RECOVERED:"*"reattached conversation ses_OUT"*)
    pass "hw unstick: the reattached id is repeated after the restart, below the RECOVERED line" ;;
  *) fail "hw unstick: the id was printed only before a 15s wait: $out" ;; esac

# A pane that published no agent_session: an absence, recorded as an absence.
out="$(STUB_CWD=/tmp/wd RUNDIR="$TMP/run" run_unstick root)"
line="$(recorded_session)"
case "$line" in *$'\t'none*"no agent_session"*) pass "hw unstick: a pane with no session records 'none — <why>', not an empty value" ;;
  *) fail "hw unstick: absent session recorded as: $line" ;; esac
case "$out" in *"WARN:this pane published NO agent_session"*) pass "hw unstick: an unrecoverable conversation is warned about BEFORE the restart" ;;
  *) fail "hw unstick: silently replaced an unaddressable conversation: $out" ;; esac
case "$out" in *"the replaced conversation is "*) fail "hw unstick: claimed a replaced-conversation id when there was none" ;;
  # The two preceding arms observed the explicit no-session warning and receipt; the forbidden id phrase is exhaustive here.
  *) pass "hw unstick: claims no id after the restart when none was published" ;; esac

# No run directory above the pane's cwd: the id must not vanish into a file
# nobody can find. It is in the output, and the output says so.
out="$(STUB_SESSION=ses_NORUN STUB_CWD=/tmp/wd run_unstick root)"
[ -z "$(recorded_session)" ] || fail "hw unstick: wrote a receipt with no run directory resolved"
case "$out" in *"THIS output only"*|*"this output only"*) pass "hw unstick: with no run dir, it says the id exists only in the output" ;;
  *) fail "hw unstick: silently dropped the id when no run dir resolved: $out" ;; esac
case "$out" in *"ses_NORUN"*) pass "hw unstick: the id is still printed when it cannot be filed" ;;
  *) fail "hw unstick: id neither filed nor printed: $out" ;; esac

# And a REFUSAL must not record anything: nothing was replaced.
out="$(STUB_SESSION=ses_NO STUB_CWD=/tmp/wd RUNDIR="$TMP/run" run_unstick question || true)"
[ -z "$(recorded_session)" ] && pass "hw unstick: a refused recovery records no replaced session, because it replaced nothing" \
  || fail "hw unstick: a refusal wrote a replaced-session record: $(recorded_session)"
