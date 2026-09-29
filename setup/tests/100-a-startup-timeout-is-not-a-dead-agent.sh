#!/usr/bin/env bash
# `herdr agent start` timing out at its own 30s budget is not proof the agent
# died: herdr's own --help says an unfinished start "keeps the name available
# for `agent read` and `agent send-keys`" — the same guarantee agent_not_ready
# gives. The old code read the timeout message as a terminal failure and gave
# up without ever polling `agent get`; the pane in the measured case (2026-09-18,
# w7H:pMZ) came up idle ~20s later, unseen.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
SOURCE="${HW_SOURCE:-$ROOT/bin/hw}"
awk '/^_agent_start\(\) \{/,/^}/' "$SOURCE" > "$TMP/start.sh"
cat > "$TMP/drive.sh" <<'SH'
set -uo pipefail
. "$START_SOURCE"
sleep() { :; }
_codex_startup_ready() { return 0; }
_claude_startup_ready() { return 0; }        # first-run screens: 188-el-primer-uso-no-se-traba.sh
_claude_start_failure_screen() { :; }
herdr() {
  case "$1 $2" in
    'agent start')
      echo start >> "$START_LOG"
      echo '{"error":{"code":"timeout","message":"timed out waiting for agent startup"}}'
      return 1 ;;
    'agent get')
      echo get >> "$GET_LOG"
      status=working
      # comes up idle on the SECOND poll, simulating the pane that was still
      # starting when herdr's own budget gave up on it
      if [ "$(wc -l < "$GET_LOG")" -gt 1 ]; then status=idle; fi
      printf '{"result":{"agent":{"pane_id":"pTEST","name":"probe","agent":"claude","agent_status":"%s","interactive_ready":true}}}\n' "$status" ;;
  esac
}
AGENT_ARGS=''
_agent_start pTEST probe claude
SH
run_case() {
  rm -f "$TMP/starts" "$TMP/gets"
  rc=0
  out="$(START_SOURCE="$TMP/start.sh" START_LOG="$TMP/starts" GET_LOG="$TMP/gets" bash "$TMP/drive.sh" 2>&1)" || rc=$?
}
run_case
[ "$rc" = 0 ] \
  || fail "a startup-timeout response was still treated as terminal: rc=$rc out=$out"
[ "$(wc -l < "$TMP/starts" | tr -d ' ')" = 1 ] \
  || fail "a registered-but-timed-out start was retried as a fresh creation"
[ "$(wc -l < "$TMP/gets" | tr -d ' ')" -ge 2 ] \
  || fail "a startup timeout never polled agent get for the pane it left registered"
pass "a herdr agent-start timeout is polled via agent get instead of killing the agent outright"
