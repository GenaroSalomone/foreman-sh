#!/usr/bin/env bash
# hw must actually DELIVER a brief to a codex executor, and must not claim a
# delivery codex itself contradicts.
#
# WHAT THIS SUBJECT IS ABOUT, measured 2026-09-06 against codex-cli 0.153.4.
# Every `hw … --agent codex` dispatch ended in "codex refused to process the
# brief (exit 1)" and "NO SESSION ID". The vendor was never the blocker:
#
#   · `invoker_resolve_sender` gives a codex pane an endpoint
#     (~/.codex/app-server-control/app-server-control.sock) that DOES NOT EXIST
#     — `codex queue --remote unix://` resolves to that exact path and answers
#     "No such file or directory" — and, before herdr publishes an
#     agent_session, a thread GUESSED from the work directory's basename, which
#     `codex queue` answers with "No active session found matching …" because a
#     TUI started by `herdr agent start` registers with no app-server daemon.
#   · hw therefore chose a native route to an unreachable address, and
#     channel-send refused it one step earlier over the `--id` and
#     `--require processed` hw sends with every brief.
#   · the recovery command hw then printed carried those same two flags, so the
#     one instruction a stuck brainer was given could not run.
#
# The herdr route DOES work on codex: the same brief prompted into a live pane
# opened a turn and Codex answered with the brief's own word. So codex joins
# claude as a vendor with no native transport hw can address.
#
# AND ONE MORE THING HERDR CANNOT SEE. With codex's "Hooks need review" dialog
# owning stdin, `herdr agent prompt --wait --until working` reported a turn,
# went to done, and the brief was NOT in the conversation. Codex's own
# UserPromptSubmit hook is the signal that cannot be faked, so a first delivery
# is proven by the thread id it publishes, not by herdr's state.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SOURCE="${HW_SOURCE:-$ROOT/bin/hw}"

# ── the harness ─────────────────────────────────────────────────────────────
# The real functions, extracted and driven directly, in the style
# 75-codex-framework-refusal.sh already uses for _codex_startup_ready. Nothing
# here re-implements hw's logic: every decision under test is hw's own text.
build_harness() {
  local src="$1" out="$2"
  {
    printf 'set -uo pipefail\n'
    for fn in _vendor_has_native_transport _resolve_strong_sender \
              _codex_thread_id _codex_thread_publisher_wired _codex_prompt_landed \
              _codex_dialog_blocking _codex_admit _codex_unproven \
              _codex_startup_ready _turn_opened _brief_undelivered _deliver_brief; do
      awk -v f="^${fn}\\\\(\\\\) \\\\{" '$0 ~ f, /^}/' "$src"
      printf '\n'
    done
  } > "$out"
  [ -s "$out" ] || fail "harness extraction produced nothing from $src"
  for fn in _vendor_has_native_transport _codex_prompt_landed _codex_admit _deliver_brief; do
    grep -q "^${fn}() {" "$out" || fail "harness is missing $fn — the extraction ranges drifted"
  done
  # NAMES ARE NOT BODIES. `awk` stops at the first column-0 `}`, so a future
  # edit that puts one inside a function would truncate it silently and this
  # subject would keep passing against half the code it claims to drive.
  bash -n "$out" || fail "harness does not parse — an extraction range truncated a body"
  grep -q 'DO NOT re-send blindly' "$out" \
    || fail "harness lost _codex_unproven's body — the extraction truncated it"
}

mkdir -p "$TMP/bin"
cat > "$TMP/bin/channel-send" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CS_LOG"
exit "${CS_RC:-0}"
STUB
chmod +x "$TMP/bin/channel-send"

# The stub herdr. Every answer is driven by the environment so one harness can
# reproduce each measured pane state without a live multiplexer.
cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "agent get")
    sid=""
    if [ "${SESSION_AFTER_PROMPT:-0}" = 1 ]; then
      [ -f "$PROMPT_MARK" ] && sid="$FAKE_SESSION"
    else
      sid="${FAKE_SESSION:-}"
    fi
    if [ -n "$sid" ]; then
      printf '{"result":{"agent":{"agent":"%s","agent_status":"%s","agent_session":{"value":"%s","agent":"%s"}}}}\n' \
        "$FAKE_VENDOR" "${FAKE_STATUS:-idle}" "$sid" "$FAKE_VENDOR"
    else
      printf '{"result":{"agent":{"agent":"%s","agent_status":"%s","agent_session":null}}}\n' \
        "$FAKE_VENDOR" "${FAKE_STATUS:-idle}"
    fi
    ;;
  "agent read")
    if [ "${DIALOG_AFTER_PROMPT:-0}" = 1 ] && [ -s "$HERDR_LOG" ]; then printf 'Hooks need review';
    else printf '%s' "${FAKE_SCREEN:-normal codex prompt}"; fi ;;
  "agent prompt")
    printf '%s\n' "prompt $*" >> "$HERDR_LOG"
    if [ "${PROMPT_RC:-0}" = 0 ] || [ "${PROMPT_LANDED:-0}" = 1 ]; then : > "$PROMPT_MARK"; fi
    printf '%s\n' "${PROMPT_OUT:-{}}"
    exit "${PROMPT_RC:-0}"
    ;;
  "agent send-keys") printf '%s\n' "send-keys $*" >> "$HERDR_LOG"; exit 0 ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
chmod +x "$TMP/bin/herdr"

build_harness "$SOURCE" "$TMP/harness.sh"

# Drive one real _deliver_brief. Prints hw's own output; the caller asserts on it.
deliver() {
  DELIVER_RC=0
  DELIVER_OUT="$(
    CS_LOG="$TMP/cs.log" HERDR_LOG="$TMP/herdr.log" PROMPT_MARK="$TMP/prompted" \
    PATH="$TMP/bin:$PATH" HW_BIN_DIR="$TMP/bin" \
    HW_SENDER_WAIT_MS=0 HW_CODEX_PROMPT_WAIT_MS=2000 HW_TURN_WAIT_MS=1000 \
    HW_RUN=testrun BRIEF="$TMP/brief.md" BRIEF_TEXT='the brief body' \
    CODEX_HOME="${CODEX_HOME_ARG:-$TMP/codex-home}" \
    C_DIM='' C_0='' AGENT="${AGENT_ARG:-codex}" \
    FAKE_VENDOR="${FAKE_VENDOR:-codex}" FAKE_SESSION="${FAKE_SESSION:-}" \
    FAKE_SCREEN="${FAKE_SCREEN:-normal codex prompt}" \
    SESSION_AFTER_PROMPT="${SESSION_AFTER_PROMPT:-0}" \
    PROMPT_RC="${PROMPT_RC:-0}" \
    PROMPT_LANDED="${PROMPT_LANDED:-0}" \
    DIALOG_AFTER_PROMPT="${DIALOG_AFTER_PROMPT:-0}" \
    SENDER_VENDOR="${SENDER_VENDOR:-codex}" \
    SENDER_SESSION="${SENDER_SESSION:-}" \
    SENDER_ENDPOINT="${SENDER_ENDPOINT:-}" \
    bash -c '
      warn() { printf "warn: %s\n" "$*"; }
      ok()   { printf "ok: %s\n" "$*"; }
      info() { printf "info: %s\n" "$*"; }
      _receipt() { printf "receipt: %s=%s\n" "$1" "$2" >> "$RECEIPT_LOG"; }
      _next_load_invoker_lib() { :; }
      invoker_resolve_sender() {
        INVOKER_SENDER_VENDOR="$SENDER_VENDOR"
        INVOKER_SENDER_SESSION="$SENDER_SESSION"
        INVOKER_SENDER_ENDPOINT="$SENDER_ENDPOINT"
        return 0
      }
      . "$1"
      _deliver_brief pTEST
    ' _ "$TMP/harness.sh" 2>&1
  )" || DELIVER_RC=$?
}

reset_logs() { : > "$TMP/cs.log"; : > "$TMP/herdr.log"; : > "$TMP/receipt.log"; rm -f "$TMP/prompted"; }

# The publisher fixture. hw asks $CODEX_HOME/hooks.json whether the hook that
# would publish a thread id is registered at all, so this suite supplies both
# answers rather than reading the machine it happens to run on.
mkdir -p "$TMP/codex-home" "$TMP/codex-home-unwired"
cat > "$TMP/codex-home/hooks.json" <<'JSON'
{"hooks":{"UserPromptSubmit":[{"hooks":[{"command":"bash '/x/bin/codex-channel-session-hook.sh' session","type":"command"}]}]}}
JSON
cat > "$TMP/codex-home-unwired/hooks.json" <<'JSON'
{"hooks":{"UserPromptSubmit":[{"hooks":[{"command":"some-other-hook","type":"command"}]}]}}
JSON
export RECEIPT_LOG="$TMP/receipt.log"
printf 'the brief body\n' > "$TMP/brief.md"

# ── 1. the classifier ───────────────────────────────────────────────────────
classify() {
  PATH="$TMP/bin:$PATH" bash -c '. "$1"; if _vendor_has_native_transport "$2"; then echo native; else echo herdr; fi' _ "$TMP/harness.sh" "$1"
}
[ "$(classify opencode)" = native ] || fail "opencode lost its native transport"
[ "$(classify codex)" = herdr ] || fail "codex is still classed as having a native transport"
[ "$(classify claude)" = herdr ] || fail "claude is no longer classed as herdr-only"
[ "$(classify '')" = herdr ] || fail "an unresolved vendor is not classed as herdr-only"
pass "codex joins claude as a vendor with no native transport hw can address"

# ── 2. a codex brief goes over herdr, never over channel-send ───────────────
# This is the measured dispatch: invoker_resolve_sender DID resolve a session
# and an endpoint for codex, which is exactly why hw used to pick that route.
reset_logs
SENDER_VENDOR=codex SENDER_SESSION=codex-probe-before \
SENDER_ENDPOINT="$HOME/.codex/app-server-control/app-server-control.sock" \
SESSION_AFTER_PROMPT=1 FAKE_SESSION=01a078cd-969c-7913-9bf7-00cdc0bc0367 deliver
[ "$DELIVER_RC" = 0 ] || fail "codex delivery failed on the herdr route: $DELIVER_OUT"
[ ! -s "$TMP/cs.log" ] || fail "codex delivery still called channel-send: $(cat "$TMP/cs.log")"
grep -q 'prompt agent prompt pTEST' "$TMP/herdr.log" || fail "codex delivery did not use herdr agent prompt: $(cat "$TMP/herdr.log")"
case "$DELIVER_OUT" in
  *"brief IN THE CONVERSATION"*"codex published thread 01a078cd-969c-7913-9bf7-00cdc0bc0367"*) ;;
  *) fail "codex delivery did not report the published thread: $DELIVER_OUT" ;;
esac
grep -q 'brief_admitted=yes — codex published thread' "$TMP/receipt.log" \
  || fail "codex delivery recorded no thread-backed admission: $(cat "$TMP/receipt.log")"
pass "a codex brief is delivered over herdr and admitted only against codex's own thread id"

# A transport timeout can race a successful UserPromptSubmit publication.
reset_logs
PROMPT_RC=1 PROMPT_OUT='timeout waiting for working' PROMPT_LANDED=1 \
SESSION_AFTER_PROMPT=1 FAKE_SESSION=timeout-thread deliver
[ "$DELIVER_RC" = 0 ] || fail "timeout overrode Codex's delivery proof: $DELIVER_OUT"
case "$DELIVER_OUT" in
  *"THE BRIEF IS NOT IN"*|*"herdr could not put"*) fail "timeout asserted false loss: $DELIVER_OUT" ;;
esac
grep -q 'brief_admitted=yes — codex published thread timeout-thread' "$TMP/receipt.log" \
  || fail "timeout did not record the successful thread proof"
pass "a herdr timeout cannot override a published Codex thread"

for proof_case in absent existing unwired; do
  reset_logs
  case "$proof_case" in
    absent) PROMPT_RC=1 PROMPT_OUT=timeout deliver ;;
    existing) PROMPT_RC=1 PROMPT_OUT=timeout FAKE_SESSION=old-thread deliver ;;
    unwired) PROMPT_RC=1 PROMPT_OUT=timeout CODEX_HOME_ARG="$TMP/codex-home-unwired" deliver ;;
  esac
  [ "$DELIVER_RC" != 0 ] || fail "timeout with $proof_case proof claimed success: $DELIVER_OUT"
  case "$DELIVER_OUT" in
    *"THE BRIEF IS NOT IN"*|*"herdr reported a turn"*|*"brief delivered"*) fail "timeout with $proof_case proof invented evidence: $DELIVER_OUT" ;;
  esac
  case "$DELIVER_OUT" in *"DO NOT re-send blindly"*) ;; *) fail "timeout omitted duplicate warning: $DELIVER_OUT" ;; esac
  [ ! -s "$TMP/cs.log" ] || fail "timeout replayed the brief"
  pass "timeout with $proof_case thread proof stays unproven without replay"
done

# ── 3. herdr claims a turn, codex publishes nothing ─────────────────────────
# The hook-dialog case: the keystrokes went into a dialog, herdr's state machine
# said working then done, and the conversation never saw a word.
reset_logs
SENDER_VENDOR=codex SENDER_SESSION=codex-probe-before \
SENDER_ENDPOINT="$HOME/.codex/app-server-control/app-server-control.sock" \
SESSION_AFTER_PROMPT=0 FAKE_SESSION='' deliver
[ "$DELIVER_RC" != 0 ] || fail "hw called a codex delivery successful with no thread id: $DELIVER_OUT"
case "$DELIVER_OUT" in
  *"brief IN THE CONVERSATION"*) fail "hw admitted a brief codex did not confirm: $DELIVER_OUT" ;;
esac
case "$DELIVER_OUT" in
  *"DELIVERY UNPROVEN on pTEST"*) ;;
  *) fail "hw did not name the missing thread id as an unproven delivery: $DELIVER_OUT" ;;
esac
grep -q 'brief_admitted=unproven — herdr reported a turn and codex published no thread id' "$TMP/receipt.log" \
  || fail "the contradicted turn was not recorded as unproven: $(cat "$TMP/receipt.log")"
pass "herdr claiming a turn does not admit a brief codex never confirmed"

# AND IT DOES NOT CLAIM THE OPPOSITE EITHER. herdr measured a turn, so the
# bytes may be in the conversation and only the receipt missing; telling the
# operator to re-send would then deliver the brief twice.
case "$DELIVER_OUT" in
  *"DO NOT re-send blindly"*"Read the pane"*) ;;
  *) fail "an unproven codex delivery told the operator to retry blindly: $DELIVER_OUT" ;;
esac
case "$DELIVER_OUT" in
  *"THE BRIEF IS NOT IN"*) fail "hw asserted non-delivery it cannot establish: $DELIVER_OUT" ;;
esac
pass "an unproven codex delivery claims neither delivery nor loss, and warns against a duplicate"

# ── 4. the recovery command has to be runnable ──────────────────────────────
case "$DELIVER_OUT" in
  *"channel-send herdr pTEST -"*) ;;
  *) fail "the codex recovery command is not the herdr one: $DELIVER_OUT" ;;
esac
case "$DELIVER_OUT" in
  *--require\ processed*|*--id\ *) fail "the codex recovery command still carries flags its route refuses: $DELIVER_OUT" ;;
esac
pass "a failed codex delivery prints a recovery command its own route accepts"

# ── 5. a startup dialog refuses the delivery instead of feeding it ──────────
reset_logs
FAKE_SCREEN='Hooks need review' SENDER_VENDOR=codex deliver
[ "$DELIVER_RC" != 0 ] || fail "hw delivered into a codex hook-trust dialog: $DELIVER_OUT"
[ ! -s "$TMP/herdr.log" ] || fail "hw typed into a pane holding a startup dialog: $(cat "$TMP/herdr.log")"
[ ! -s "$TMP/cs.log" ] || fail "hw sent a brief through channel-send while a dialog owned stdin"
grep -q 'brief_admitted=no — codex was holding hook trust' "$TMP/receipt.log" \
  || fail "the dialog refusal was not recorded by name: $(cat "$TMP/receipt.log")"
case "$DELIVER_OUT" in
  *"re-sending it is NOT the fix"*"Clear it on that"*) ;;
  *) fail "the dialog refusal told the operator to re-send into the same dialog: $DELIVER_OUT" ;;
esac
pass "a codex startup dialog refuses the brief before anything is typed, and says a retry will not answer it"

# THE DELIVERY-TIME CHECK ONLY LOOKS. `_codex_startup_ready` answers the
# directory-trust dialog with an Enter, which is right at `agent start` and
# wrong here: hw must not both answer a dialog and claim it typed nothing.
reset_logs
FAKE_SCREEN='Do you trust the contents of this directory' SENDER_VENDOR=codex deliver
[ "$DELIVER_RC" != 0 ] || fail "hw delivered into a codex directory-trust dialog: $DELIVER_OUT"
[ ! -s "$TMP/herdr.log" ] || fail "hw answered a dialog at delivery time while claiming it typed nothing: $(cat "$TMP/herdr.log")"
pass "the delivery-time dialog check reads the screen and never answers it"

# ── 6. the proof is scoped to a FIRST delivery, and says so by behaviour ────
# A pane that already published a thread id (a re-task) cannot produce a causal
# proof, so hw attempts delivery but cannot report admission as established.
reset_logs
SENDER_VENDOR=codex FAKE_SESSION=01a078c5-2a79-7082-87a1-3e1f6371d5a8 \
SESSION_AFTER_PROMPT=0 deliver
[ "$DELIVER_RC" != 0 ] || fail "existing Codex thread became proof of this delivery: $DELIVER_OUT"
case "$DELIVER_OUT" in
  *"codex published thread"*) fail "hw claimed a causal thread proof it could not have: $DELIVER_OUT" ;;
esac
# AND IT MUST NOT QUIETLY FALL BACK TO THE UNQUALIFIED CLAIM. Printing the same
# "brief IN THE CONVERSATION … a turn opened" every other vendor gets would
# reinstate, for the one vendor whose prompt herdr cannot see, exactly the
# unbacked sentence this subject exists to remove.
case "$DELIVER_OUT" in
  *"brief IN THE CONVERSATION"*|*"brief delivered"*|*"THE BRIEF IS NOT IN"*) fail "hw made a delivery/loss claim on a codex pane with no usable proof: $DELIVER_OUT" ;;
esac
grep -q "brief_admitted=unproven — this pane had already published thread" "$TMP/receipt.log" \
  || fail "an unavailable proof was omitted from the receipt instead of recorded: $(cat "$TMP/receipt.log")"
case "$DELIVER_OUT" in
  *"had already published thread 01a078c5-2a79-7082-87a1-3e1f6371d5a8"*) ;;
  *) fail "hw did not name WHY the proof was unavailable: $DELIVER_OUT" ;;
esac
pass "a reused Codex pane is prompted but admission remains unproven, not success or loss"

# ── 7. opencode is untouched ────────────────────────────────────────────────
reset_logs
AGENT_ARG=opencode FAKE_VENDOR=opencode SENDER_VENDOR=opencode \
SENDER_SESSION=ses_abc SENDER_ENDPOINT=http://127.0.0.1:5555 deliver
grep -q -- '--require processed --id testrun:brief opencode ses_abc' "$TMP/cs.log" \
  || fail "opencode no longer uses its native causal route: $(cat "$TMP/cs.log")"
pass "opencode keeps its native route and its processed receipt"

# ── 8. the replay arm goes through the same door ────────────────────────────
# The first attempt stalls, hw replays the exact bytes once, and `_turn_opened`
# says a turn is there. For codex that signal is worth nothing on its own: it is
# herdr's agent_status, which is what a dialog fools. A proof one branch can
# walk around is not a proof.
reset_logs
PROMPT_RC=1 PROMPT_OUT='agent_prompt_stalled' SENDER_VENDOR=codex \
FAKE_STATUS=working SESSION_AFTER_PROMPT=0 FAKE_SESSION='' deliver
[ "$DELIVER_RC" != 0 ] || fail "the replay arm admitted a codex brief on herdr's state alone: $DELIVER_OUT"
case "$DELIVER_OUT" in
  *"brief IN THE CONVERSATION"*) fail "the replay arm printed the unqualified claim for codex: $DELIVER_OUT" ;;
esac
case "$DELIVER_OUT" in
  *"DELIVERY UNPROVEN on pTEST"*) ;;
  *) fail "the replay arm did not report the delivery as unproven: $DELIVER_OUT" ;;
esac
pass "the stalled-then-replayed arm admits a codex brief only through the same thread-id door"

reset_logs
PROMPT_RC=1 PROMPT_OUT='agent_prompt_stalled' DIALOG_AFTER_PROMPT=1 SENDER_VENDOR=codex \
FAKE_STATUS=working FAKE_SESSION=old-thread deliver
[ "$DELIVER_RC" != 0 ] && [ ! -s "$TMP/cs.log" ] || fail "a newly appeared Codex dialog received a replay: $DELIVER_OUT"
case "$DELIVER_OUT" in *"DELIVERY UNPROVEN"*"replay was not attempted"*) ;; *) fail "late dialog was not reported as uncertain first delivery: $DELIVER_OUT" ;; esac
pass "a Codex dialog appearing after the first submission blocks replay without claiming loss"

reset_logs
PROMPT_RC=1 PROMPT_OUT='agent_prompt_stalled' CS_RC=1 SENDER_VENDOR=codex \
FAKE_STATUS=idle FAKE_SESSION=old-thread deliver
[ "$DELIVER_RC" != 0 ] || fail "failed Codex replay was admitted"
case "$DELIVER_OUT" in *"THE BRIEF IS NOT IN"*|*"brief delivered"*) fail "failed replay claimed delivery or loss: $DELIVER_OUT" ;; esac
case "$DELIVER_OUT" in *"DELIVERY UNPROVEN"*"DO NOT re-send blindly"*) ;; *) fail "failed replay omitted uncertainty/duplicate warning: $DELIVER_OUT" ;; esac
pass "a failed Codex replay remains unproven instead of asserting non-delivery"

# The full delivery caller owns exactly ONE final admission row. Terminal
# uncertainty cannot repair a false observation already appended to a receipt.
for proof in reused unwired; do
  for replay_rc in 0 1; do
    reset_logs
    if [ "$proof" = reused ]; then
      PROMPT_RC=1 PROMPT_OUT='agent_prompt_stalled' CS_RC="$replay_rc" SENDER_VENDOR=codex \
        FAKE_STATUS=idle FAKE_SESSION=old-thread deliver
    else
      PROMPT_RC=1 PROMPT_OUT='agent_prompt_stalled' CS_RC="$replay_rc" SENDER_VENDOR=codex \
        FAKE_STATUS=idle FAKE_SESSION='' CODEX_HOME_ARG="$TMP/codex-home-unwired" deliver
    fi
    [ "$DELIVER_RC" != 0 ] || fail "no-turn replay was admitted ($proof/$replay_rc): $DELIVER_OUT"
    receipt="$(cat "$TMP/receipt.log")"
    [ "$(grep -c '^receipt: brief_admitted=' "$TMP/receipt.log")" = 1 ] || fail "replay wrote duplicate admission rows ($proof/$replay_rc): $receipt"
    case "$receipt" in *"herdr measured a turn"*|*"herdr reported a turn"*|*"brief_admitted=yes"*) fail "replay invented a measured turn ($proof/$replay_rc): $receipt" ;; esac
    case "$receipt" in *"brief_admitted=unproven"*) ;; *) fail "replay lost its final unproven receipt: $receipt" ;; esac
    pass "full Codex replay $proof/$replay_rc writes one unproven receipt without inventing a turn"
  done
done

# ── 9. no publisher, no evidence — and no verdict either ────────────────────
# The proof rests on a hook a person can decline to install. Absence of an id is
# evidence only when the thing that would have produced it exists; otherwise a
# delivery that landed perfectly would be reported as lost, and the operator
# told to send it a second time.
reset_logs
CODEX_HOME_ARG="$TMP/codex-home-unwired" SENDER_VENDOR=codex \
SESSION_AFTER_PROMPT=0 FAKE_SESSION='' deliver
[ "$DELIVER_RC" != 0 ] || fail "unwired publisher was treated as successful admission: $DELIVER_OUT"
case "$DELIVER_OUT" in
  *"registers no codex-channel-session-hook"*) ;;
  *) fail "hw did not name the missing publisher as the reason the proof was unavailable: $DELIVER_OUT" ;;
esac
case "$DELIVER_OUT" in
  *"THE BRIEF IS NOT IN"*|*"brief delivered"*) fail "an absent publisher became a delivery or loss claim: $DELIVER_OUT" ;;
esac
grep -q "brief_admitted=unproven — .*registers no codex-channel-session-hook" "$TMP/receipt.log" \
  || fail "an unwired publisher was not recorded as an unavailable proof: $(cat "$TMP/receipt.log")"
pass "an unwired publisher makes the proof unavailable, never a verdict against a delivery that happened"

# ── 10. the PANE's vendor decides, not the flag ─────────────────────────────
# A dispatch into a reused pane can request one vendor and land on another. The
# guards have to follow what is measured there, or a codex pane gets delivered
# to as if it were a claude one.
reset_logs
AGENT_ARG=claude FAKE_VENDOR=codex SENDER_VENDOR=codex \
FAKE_SCREEN='Hooks need review' deliver
[ "$DELIVER_RC" != 0 ] || fail "a measured codex pane skipped the codex guards because --agent said claude: $DELIVER_OUT"
[ ! -s "$TMP/herdr.log" ] || fail "hw typed into a codex pane holding a dialog because the requested vendor differed"
pass "the codex guards follow the pane's measured vendor, not the vendor the flag asked for"

# ── 11. no hand-coded route test survives anywhere in the binary ────────────
# STRUCTURAL, and labelled as such. The first version of this change converted
# three route-selection sites and its own comment claimed there were three;
# there are five, and the two it missed are `cmd_next`'s — a separate delivery
# path with no coverage here, so `hw next` on codex stayed broken by the exact
# mechanism this subject exists to kill. A count in a comment is not a guard.
hand_coded="$(cat "$SOURCE" "$ROOT/lib/hw/next.sh" | grep -c '\[ "\$INVOKER_SENDER_VENDOR" != claude \]' || true)"
[ "$hand_coded" = 0 ] \
  || fail "$hand_coded route-selection site(s) still hand-code != claude instead of asking _vendor_has_native_transport"
# Sliced function-start to NEXT function-start, not to the first column-0 `}`:
# cmd_next is long enough to contain one before its route selection, and a
# range that stopped early would report a converted site as missing.
# CAPTURED, THEN MATCHED — never `awk … | grep -q`. Under `pipefail` grep -q
# exits on the first hit, awk takes SIGPIPE, and the pipeline reports failure
# for a slice that DID contain the string: the assertion would fail hardest on
# the longest function, which is cmd_next, the one this block exists to check.
for fn in _resolve_strong_sender _deliver_brief cmd_next; do
  slice="$(awk -v f="${fn}() {" '
    index($0, f) == 1 { inside = 1; print; next }
    inside && /^[a-zA-Z_][a-zA-Z0-9_]*\(\) \{/ { exit }
    inside { print }
  ' "$SOURCE" "$ROOT/lib/hw/next.sh")"
  case "$slice" in
    *_vendor_has_native_transport*) ;;
    *) fail "$fn does not ask _vendor_has_native_transport, so its route selection is not covered by this subject" ;;
  esac
done
pass "every route-selection site in hw, launch path and hw next alike, asks the one classifier"

# ── M01 ─────────────────────────────────────────────────────────────────────
# Give codex a native transport again. The measured dispatch must then leave
# herdr untouched and go back through channel-send, which is the exact
# "codex refused to process the brief" this subject exists to keep dead.
mut="$TMP/hw-native-codex"; cp "$SOURCE" "$mut"
MUT_HW="$mut" python3 - <<'PY'
import os
p = os.environ["MUT_HW"]
s = open(p, encoding="utf-8").read()
old = '''    ""|claude|codex) return 1 ;;'''
new = '''    ""|claude) return 1 ;;'''
assert s.count(old) == 1, s.count(old)
open(p, "w", encoding="utf-8").write(s.replace(old, new))
PY
build_harness "$mut" "$TMP/harness.sh"
reset_logs
SENDER_VENDOR=codex SENDER_SESSION=codex-probe-before \
SENDER_ENDPOINT="$HOME/.codex/app-server-control/app-server-control.sock" \
SESSION_AFTER_PROMPT=1 FAKE_SESSION=01a078cd-969c-7913-9bf7-00cdc0bc0367 deliver
if grep -q -- '--require processed --id testrun:brief codex codex-probe-before' "$TMP/cs.log" \
   && [ ! -s "$TMP/herdr.log" ]; then
  pass "mutant killed: M01 with a native codex transport the brief goes back to the route that refuses it"
else
  fail "M01 survived or misfired: cs=$(cat "$TMP/cs.log") herdr=$(cat "$TMP/herdr.log")"
fi

# ── M02 ─────────────────────────────────────────────────────────────────────
# Drop the thread-id proof and keep everything else. The dialog-swallowed
# delivery must then be reported as a success, which is the false fact herdr's
# state alone produces.
mut2="$TMP/hw-no-proof"; cp "$SOURCE" "$mut2"
MUT_HW="$mut2" python3 - <<'PY'
import os
p = os.environ["MUT_HW"]
s = open(p, encoding="utf-8").read()
old = '''_codex_admit() {'''
new = '''_codex_admit() {
  ok "brief delivered to $1 (M02 no proof)"; return 0'''
assert s.count(old) == 1, s.count(old)
open(p, "w", encoding="utf-8").write(s.replace(old, new))
PY
build_harness "$mut2" "$TMP/harness.sh"
reset_logs
SENDER_VENDOR=codex SESSION_AFTER_PROMPT=0 FAKE_SESSION='' deliver
if [ "$DELIVER_RC" = 0 ]; then
  case "$DELIVER_OUT" in
    *"brief delivered to pTEST"*|*"brief IN THE CONVERSATION"*)
      pass "mutant killed: M02 without the thread-id proof a swallowed brief is reported as delivered" ;;
    *) fail "M02 misfired: rc=0 but no delivery claim: $DELIVER_OUT" ;;
  esac
else
  fail "M02 survived: the delivery still failed without the proof: $DELIVER_OUT"
fi

# ── M03 ─────────────────────────────────────────────────────────────────────
# Exempt the replay arm from the admission door, which is what the first
# version of this change actually did. The stalled delivery must then be
# reported as delivered on herdr's state alone.
mut3="$TMP/hw-replay-exempt"; cp "$SOURCE" "$mut3"
MUT_HW="$mut3" python3 - <<'PY'
import os
p = os.environ["MUT_HW"]
s = open(p, encoding="utf-8").read()
old = '''      if [ "$pane_vendor" = codex ]; then
        _codex_admit "$pane" "replay over $r_route returned exit $r_rc" "$bytes" && return 0
        _codex_unproven "$pane" "${codex_unproven_reason:-replay outcome could not be established}"
        return 1
      fi
'''
assert s.count(old) == 1, s.count(old)
open(p, "w", encoding="utf-8").write(s.replace(old, ""))
PY
build_harness "$mut3" "$TMP/harness.sh"
reset_logs
PROMPT_RC=1 PROMPT_OUT='agent_prompt_stalled' SENDER_VENDOR=codex \
FAKE_STATUS=working SESSION_AFTER_PROMPT=0 FAKE_SESSION='' deliver
if [ "$DELIVER_RC" = 0 ]; then
  case "$DELIVER_OUT" in
    *"brief IN THE CONVERSATION"*"replayed once over herdr, turn confirmed"*)
      pass "mutant killed: M03 an exempt replay arm admits a codex brief on herdr's state alone" ;;
    *) fail "M03 misfired: rc=0 but not the replay admission: $DELIVER_OUT" ;;
  esac
else
  fail "M03 survived: the replay arm still refused without its exemption: $DELIVER_OUT"
fi

# ── M04 ─────────────────────────────────────────────────────────────────────
# Pretend the publisher is wired. A thread appearing without the claimed
# producer must not be promoted into a receipt from that producer.
mut4="$TMP/hw-always-wired"; cp "$SOURCE" "$mut4"
MUT_HW="$mut4" python3 - <<'PY'
import os
p = os.environ["MUT_HW"]
s = open(p, encoding="utf-8").read()
old = '''  elif ! _codex_thread_publisher_wired; then'''
new = '''  elif false; then'''
assert s.count(old) == 1, s.count(old)
open(p, "w", encoding="utf-8").write(s.replace(old, new))
PY
build_harness "$mut4" "$TMP/harness.sh"
reset_logs
CODEX_HOME_ARG="$TMP/codex-home-unwired" SENDER_VENDOR=codex \
SESSION_AFTER_PROMPT=1 FAKE_SESSION=unwired-thread deliver
if [ "$DELIVER_RC" = 0 ]; then
  case "$DELIVER_OUT" in
    *"codex published thread unwired-thread"*) pass "mutant killed: M04 without the publisher check a thread is admitted without its claimed producer" ;;
    *) fail "M04 misfired: zero but no thread-backed claim: $DELIVER_OUT" ;;
  esac
else
  fail "M04 survived: the unwired thread still was not admitted: $DELIVER_OUT"
fi
