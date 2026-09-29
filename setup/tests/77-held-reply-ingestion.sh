#!/usr/bin/env bash
# A raw blocked-pane reply is terminal submission, not receiver ingestion.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BIN="$TMP/bin"; STUB="$TMP/stub"; mkdir -p "$BIN" "$STUB"
cp "$ROOT"/bin/* "$BIN/" 2>/dev/null || true
LOG="$TMP/herdr.log"; export LOG

cat > "$STUB/herdr" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$LOG"
case "$1 $2" in
  "agent get") printf '{"result":{"agent":{"agent_status":"idle"}}}\n' ;;
  "agent prompt")
    if [ "${PROMPT_BLOCKED:-0}" = 1 ]; then
      printf '{"error":{"code":"agent_blocked","message":"probe blocked"}}\n' >&2
      exit 1
    fi
    printf '{"result":{}}\n'
    ;;
  "pane send-text"|"pane send-keys") printf '{"result":{}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
chmod +x "$STUB/herdr"

help_out="$("$BIN/channel-send" --help)" && help_rc=0 || help_rc=$?
[ "$help_rc" = 0 ] || fail "channel-send --help exited $help_rc: $help_out"
case "$help_out" in
  *"ADMITTED or terminal-submitted"*"receiver may yet have the message"*"Do NOT blind-retry"*"keeps its hold"*) pass "help pins exit 5 raw-reply uncertainty and hold retention" ;;
  *) fail "channel-send --help omitted exit 5 raw-reply semantics: $help_out" ;;
esac

write_hold() { # path intent
  cat > "$1" <<EOF
version=1
state=delivered
intent=$2
route=herdr
target=wT:p1
logical_id=
pane=wT:p1
run=run-1
EOF
}

run_reply() { # intent hold blocked
  local intent="$1" hold="$2" blocked="$3"
  : > "$LOG"
  if [ "$intent" = ruling ]; then
    PATH="$STUB:$PATH" TMPDIR="$TMP" PROMPT_BLOCKED="$blocked" \
      "$BIN/channel-send" --ruling --reply-hold "$hold" herdr wT:p1 - 'CHANNEL_REPLY' 2>&1
  else
    PATH="$STUB:$PATH" TMPDIR="$TMP" PROMPT_BLOCKED="$blocked" \
      "$BIN/channel-send" --report --reply-hold "$hold" herdr wT:p1 - 'CHANNEL_REPLY' 2>&1
  fi
}

for intent in ruling answer; do
  hold="$TMP/${intent}-blocked"; write_hold "$hold" "$intent"
  out="$(run_reply "$intent" "$hold" 1)" && rc=0 || rc=$?
  [ "$rc" = 5 ] || fail "$intent blocked fallback exited $rc, expected uncertainty exit 5: $out"
  [ -e "$hold" ] || fail "$intent blocked fallback consumed its hold without ingestion proof"
  [ "$(grep -c '^pane send-text ' "$LOG" || true)" = 1 ] || fail "$intent blocked fallback did not submit terminal text"
  [ "$(grep -c '^pane send-keys ' "$LOG" || true)" = 1 ] || fail "$intent blocked fallback did not submit Enter"
  case "$out" in
    *"receiver ingestion is UNCONFIRMED"*"reply hold remains"*"do NOT blind-retry"*) pass "$intent blocked fallback exits uncertain and preserves its hold" ;;
    *) fail "$intent blocked fallback omitted uncertainty/hold diagnosis: $out" ;;
  esac
done

for intent in ruling answer; do
  hold="$TMP/${intent}-direct"; write_hold "$hold" "$intent"
  out="$(run_reply "$intent" "$hold" 0)" && rc=0 || rc=$?
  [ "$rc" = 0 ] || fail "$intent receipt-backed direct reply exited $rc: $out"
  [ ! -e "$hold" ] || fail "$intent receipt-backed direct reply left its hold"
  [ "$(grep -c '^agent prompt ' "$LOG" || true)" = 1 ] || fail "$intent direct reply did not use receiver-owned prompt"
  pass "$intent receipt-backed direct reply clears its hold"
done

printf 'coverage - blocked answer/ruling fallback uncertainty and direct answer/ruling hold clearing\n'
