#!/usr/bin/env bash
# A plain message reaching a herdr-blocked pane must not die unconditionally.
#
# MEASURED against a real opencode executor: herdr agent get said
# agent_status=blocked, blocked_reason=stuck, blocked_scope=root, while the
# target's OWN opencode endpoint answered /question -> [] and /permission -> []
# — no human was actually being waited on. `herdr agent prompt` refused with
# agent_blocked anyway, and channel-send died with "NOTHING was sent" even
# though `herdr pane send-text` + `herdr pane send-keys enter` demonstrably
# delivers and submits in that exact state.
#
# The proved-report and proved-hold arms already had this look via
# `witness_state` (see 42-held-ruling-delivery.sh). This subject covers the
# plain message with none of those proofs: it must get the same look, not a
# different and weaker one.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BIN="$TMP/bin2"; STUB="$TMP/stub"; mkdir -p "$BIN" "$STUB"
cp "$ROOT"/bin/* "$BIN/" 2>/dev/null || true
LOG="$TMP/log"; SENT="$TMP/sent"; export LOG SENT

cat > "$STUB/herdr" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$LOG"
case "$1 $2" in
  "agent get") printf '{"result":{"agent":{"agent":"claude","agent_status":"working","tokens":{"turn_state":"ended_unreported"}}}}\n' ;;
  "pane process-info") printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","43111"]}]}}}\n' ;;
  "agent prompt")
    printf '{"error":{"code":"agent_blocked","message":"agent wT:p1 is blocked and requires interactive input"},"id":"cli:agent:prompt"}\n' >&2
    exit 1
    ;;
  "pane send-text") printf '%s\n' "$4" >> "$SENT"; printf '{"result":{}}\n' ;;
  "pane send-keys") printf '{"result":{}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
cat > "$BIN/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
printf '%s %s\n' "$1" "${2:-}" >> "$LOG"
[ "$1" != wait-agent ] || exit 2
if [ "$1 $2" = "call pane.read" ]; then
  count_file="${WITNESS_COUNT_FILE:?}"
  count=0; [ ! -r "$count_file" ] || count="$(cat "$count_file")"
  count=$((count + 1)); printf '%s\n' "$count" > "$count_file"
  if [ "${MSG_WITNESS:-static}" = live ]; then text="screen-$count"; else text="static"; fi
  printf '{"read":{"text":"%s"}}\n' "$text"
  exit 0
fi
printf '{"result":{}}\n'
STUB
cat > "$STUB/curl" <<'STUB'
#!/usr/bin/env bash
case "$*" in
  */question) [ "${MSG_HUMAN:-0}" = 1 ] && printf '[{"id":"permission"}]\n' || printf '[]\n' ;;
  */permission) printf '[]\n' ;;
  *) exit 1 ;;
esac
STUB
chmod +x "$STUB/herdr" "$STUB/curl" "$BIN/herdr-rpc"

# A genuinely plain message (no --report, no --ruling, no proven hold) only
# ever reaches herdr's agent_blocked response if it skips the idle gate first
# — which for a plain message means --allow-turn-ended against a target whose
# own Stop hook says a turn has ended (see channel-send's ALLOW_TURN_ENDED
# handling). Without that flag the wait gate itself refuses first and
# agent_blocked is never reached at all, which is a different, already-tested
# code path (see setup/tests/33-*.sh and friends).
run_plain() {
  local human="${1:-0}" witness="${2:-static}"
  : > "$LOG"; : > "$SENT"; : > "$TMP/witness-count"
  PATH="$STUB:$PATH" HW_INVOKER_WAIT_MS=300 MSG_HUMAN="$human" MSG_WITNESS="$witness" \
    WITNESS_SAMPLES=2 WITNESS_INTERVAL_S=0 WITNESS_COUNT_FILE="$TMP/witness-count" \
    "$BIN/channel-send" --allow-turn-ended herdr wT:p1 - "a plain message" 2>&1
}

# ── C01: no human prompt outstanding → falls back to raw pane input ─────────
out="$(run_plain 0 static)" && rc=0 || rc=$?
[ "$rc" = 0 ] || fail "C01 plain message with no human prompt expected fallback delivery, got $rc: $out"
[ "$(grep -c '^pane send-text ' "$LOG" || true)" = 1 ] || fail "C01 plain message did not use pane send-text exactly once"
[ "$(grep -c '^pane send-keys ' "$LOG" || true)" = 1 ] || fail "C01 plain message did not submit exactly once"
[ "$(cat "$SENT" 2>/dev/null)" = "a plain message" ] || fail "C01 wrong text reached pane send-text: $(cat "$SENT" 2>/dev/null)"
case "$out" in
  *"agent_blocked"*"confirms nothing is pending"*"falling back to raw pane input"*"message submitted to"*"blocked-pane fallback"*) pass "C01 a plain message reaches a blocked pane whose endpoint confirms nothing is pending" ;;
  *) fail "C01 fallback delivery omitted its diagnosis or accounting: $out" ;;
esac

# ── C02: endpoint confirms a real human prompt → refused, not overwritten ───
out="$(run_plain 1 static)" && rc=0 || rc=$?
[ "$rc" = 1 ] || fail "C02 awaiting-human plain message expected refusal, got $rc: $out"
[ ! -s "$SENT" ] || fail "C02 awaiting-human plain message overwrote the receiver prompt"
case "$out" in
  *"CONFIRMS a real prompt is outstanding"*"NOTHING was sent"*"raw pane input would overwrite a real human prompt"*) pass "C02 a plain message still refuses raw input when the endpoint confirms a real prompt" ;;
  *) fail "C02 awaiting-human refusal omitted the reason or remedy: $out" ;;
esac

# ── the mutant of the bash went with it ────────────────────────────────────
# THE ONE BASH MUTANT WENT WITH THE BASH (go-060 S5: bin/channel-send is a shim). It rewrote the `else` of the blocked-pane fallback into
# `elif false` with a python edit (no anchor id), so a plain message never reached raw pane input. In Go that branch is the `default:` of
# the agent_blocked switch in core/channel_send_herdr.go; disabling `if qn == "0" && pn == "0"` there turns
# TestChannelSendBlockedPlainMessageEndpointSaysClear (core/channel_send_test.go) red (measured 2026-10-24, go-060 S5), and C01 here
# asserts the same delivery through the shim. It carries no `<n>-G<nn>` anchor of setup/tests/1016.

printf 'mapping - C01 no-human fallback; C02 awaiting-human refusal (shares the witness wiring with 42-held-ruling-delivery.sh)\n'
printf 'coverage - 61: 2 behavior claims, 0 mutants (the 1 bash mutant is retired; the Go test TestChannelSendBlockedPlainMessageEndpointSaysClear goes red on its Go equivalent)\n'
