#!/usr/bin/env bash
# pi's delivery route is herdr, both directions (setup/briefs/pi-vendor-v1.md,
# piece 2; docs/pi-vendor.md). pi has no daemon or HTTP endpoint an invoker can
# address, so a pi pane is classed with claude and codex: hw delivers the brief
# with `herdr agent prompt --wait --until working` (the turn-opened proof herdr's
# pi integration publishes), and a reply to a pi executor is the herdr command.
# OLD: pi fell to `*) return 0` — "native" — and the reply command named a
# `channel-send pi …` route that does not exist.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LIB="${PI_INVOKER_LIB_SOURCE:-$ROOT/bin/invoker-common.sh}"
HW_SRC="${PI_HW_SOURCE:-$ROOT/bin/hw}"
[ -r "$LIB" ] || fail "no invoker library at $LIB"

# ── the harness ─────────────────────────────────────────────────────────────
# The REAL library, sourced. herdr is a stub driven by the environment so one
# harness reproduces each measured pane state; channel-send is a recorder, so
# "the command runs" is measured by running it, not by reading it.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "agent get")
    if [ -n "${FAKE_SESSION:-}" ]; then
      printf '{"result":{"agent":{"agent":"%s","cwd":"%s","agent_status":"idle","agent_session":{"value":"%s","agent":"%s"}}}}\n' \
        "$FAKE_VENDOR" "${FAKE_CWD:-/w/x}" "$FAKE_SESSION" "$FAKE_VENDOR"
    else
      printf '{"result":{"agent":{"agent":"%s","cwd":"%s","agent_status":"idle","agent_session":null}}}\n' \
        "$FAKE_VENDOR" "${FAKE_CWD:-/w/x}"
    fi
    ;;
  "pane process-info")
    if [ -n "${FAKE_PORT:-}" ]; then
      printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","%s"]}]}}}\n' "$FAKE_PORT"
    else
      printf '{"result":{"process_info":{"foreground_processes":[]}}}\n'
    fi
    ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
cat > "$TMP/bin/channel-send" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CS_LOG"
exit 0
STUB
chmod +x "$TMP/bin/herdr" "$TMP/bin/channel-send"
export CS_LOG="$TMP/cs.log"

# drive <lib> <shell snippet> — the snippet runs after the library is sourced.
# The invoker environment is the executor's own: HERDR_PANE_ID is the pane the
# functions belong to, HW_EXECUTOR_VENDOR is what hw baked into it.
drive() {
  local lib="$1"; shift
  PATH="$TMP/bin:$PATH" \
  INVOKER_PROG=probe INVOKER_BIN_DIR="$TMP/bin" \
  HERDR_PANE_ID="${PANE:-wC:p9}" HW_EXECUTOR_VENDOR="${EXEC_VENDOR:-codex}" HW_RUN="${RUN:-testrun}" \
  FAKE_VENDOR="${FAKE_VENDOR:-codex}" FAKE_SESSION="${FAKE_SESSION-01a0790e-7cf5-7183-a594-9006e28ca563}" \
  FAKE_CWD="${FAKE_CWD:-/w/work/setup/guessable-basename}" FAKE_PORT="${FAKE_PORT:-}" \
  bash -c '
    set -uo pipefail
    die() { printf "die: %s\n" "$*" >&2; exit 1; }
    . "$1"; shift
    eval "$1"
  ' _ "$lib" "$1" 2>&1
}
resolve() { # <pane-argument> — prints the three variables one per line
  drive "$LIB" 'invoker_resolve_sender '"$1"'; printf "%s\n%s\n%s\n" "$INVOKER_SENDER_VENDOR" "$INVOKER_SENDER_SESSION" "$INVOKER_SENDER_ENDPOINT"'
}

classify() { drive "$LIB" 'if invoker_vendor_has_native_transport "'"$1"'"; then echo native; else echo herdr; fi'; }
[ "$(classify pi)" = herdr ] || fail "the invoker library classes pi as having a native transport"
pass "the invoker library classes pi with claude and codex: herdr only"

HOLD="$TMP/pending-reply-1"; rm -f "$HOLD"
CMD="$(EXEC_VENDOR=pi FAKE_VENDOR=pi FAKE_SESSION=/w/.pi/agent/sessions/s.jsonl drive "$LIB" 'invoker_reply_command testrun:ask:1:reply answer '"$HOLD")"
case "$CMD" in
  *"herdr wC:p9 - "*) pass "a pi executor hands its brainer a herdr reply command addressed to its pane" ;;
  *) fail "a pi executor's reply command is not the herdr one: $CMD" ;;
esac
case "$CMD" in *" pi /w/"*|*"--require processed"*) fail "a pi reply command carries a native route: $CMD" ;; esac

OUT="$(FAKE_VENDOR=pi FAKE_SESSION=/w/.pi/agent/sessions/s.jsonl resolve wC:p9)"
E="$(printf '%s\n' "$OUT" | sed -n 3p)"
[ "$E" = - ] || fail "resolving a pi pane invented an endpoint: $OUT"
pass "resolving a pi pane keeps its session and invents no endpoint"

# hw's own classifier, extracted and driven alone.
fn="$(sed -n '/^_vendor_has_native_transport() {/,/^}/p' "$HW_SRC")"
[ -n "$fn" ] || fail "could not extract _vendor_has_native_transport from bin/hw"
v="$(bash -c "$fn"'; _vendor_has_native_transport pi && echo native || echo herdr')"
[ "$v" = herdr ] || fail "hw classes pi as native: the brief would go to a channel-send route that does not exist"
pass "hw delivers a pi brief over herdr (agent prompt --wait --until working)"
