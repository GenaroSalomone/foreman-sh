#!/usr/bin/env bash
# Retrying readiness must not repeat a creation whose name remains registered.
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
      if [ -f "$START_LOG" ]; then
        echo '{"error":{"code":"agent_name_taken"}}'; return 1
      fi
      echo start > "$START_LOG"
      echo '{"error":{"code":"agent_not_ready"}}'; return 1 ;;
    'agent get')
      echo get >> "$GET_LOG"
      status=working
      if [ "$(wc -l < "$GET_LOG")" -gt 1 ] && [ "$CASE" != timeout ]; then status=idle; fi
      pane=pTEST; vendor=codex; name=probe
      case "$CASE" in foreign) pane=pOTHER ;; vendor) vendor=claude ;; name) name=other ;; esac
      printf '{"result":{"agent":{"pane_id":"%s","name":"%s","agent":"%s","agent_status":"%s","interactive_ready":true}}}\n' "$pane" "$name" "$vendor" "$status" ;;
  esac
}
AGENT_ARGS=''
_agent_start pTEST probe codex
SH
run_case() {
  rm -f "$TMP/starts" "$TMP/gets"
  rc=0
  out="$(START_SOURCE="$TMP/start.sh" START_LOG="$TMP/starts" GET_LOG="$TMP/gets" CASE="$1" bash "$TMP/drive.sh" 2>&1)" || rc=$?
}
run_case ready
[ "$rc" = 0 ] || fail "registered Codex readiness retried creation: $out"
[ "$(wc -l < "$TMP/starts" | tr -d ' ')" = 1 ] || fail "registered agent was started twice"
[ "$(wc -l < "$TMP/gets" | tr -d ' ')" -ge 2 ] || fail "readiness never polled past the initial working state"
pass "a registered but not-ready Codex agent is polled, never recreated"
for state in foreign vendor name timeout; do
  run_case "$state"
  [ "$rc" != 0 ] || fail "readiness adopted $state agent state"
  pass "readiness refuses $state without adopting another agent or waiting forever"
done
