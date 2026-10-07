#!/usr/bin/env bash
# a tab is not an agent, and a launch with no agent is not a launch
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/21-agent-vs-container.sh
#
# WHAT THIS SUBJECT IS. Two measurements from 2026-08-26, one bug:
#
#   * a relaunch onto a tab whose agent pane had been closed printed
#     `✓ tab … already exists — focused, nothing rebuilt` and exited 0 having
#     started nothing. "Nothing rebuilt" was true and useless: the question is
#     whether anything is left to reuse.
#   * a launch whose `_agent_start` failed printed two warnings, then the
#     success banner, and exited 0 — no agent, no brief delivered, and the
#     caller told everything was fine.
#
# THE ONE EXCEPTION TO "EVERY hw CALL IS --dry-run". The reuse gate runs BEFORE
# the dry-run exit, so it is unreachable that way. It is still hermetic: hw
# derives its work root from $HOME, so $HOME=$TMP keeps every path this makes
# inside the fixture, and herdr is the stub.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# The shared stub answers `tab list`/`pane list` with empty arrays. This subject
# needs a container that EXISTS, with and without an agent inside it, so it
# brings its own.
cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
tab="${STUB_TAB_LABEL:-none}"; ws="${STUB_WS_LABEL:-none}"
if [ "${STUB_TAB_AGENT:-0}" = 1 ]; then
  P='{"pane_id":"wX:p1","tab_id":"wX:t1","workspace_id":"wX","agent":"claude","label":"agent","cwd":"/w"},{"pane_id":"wX:p2","tab_id":"wX:t1","workspace_id":"wX","agent":null,"label":"shell","cwd":"/w"}'
else
  P='{"pane_id":"wX:p2","tab_id":"wX:t1","workspace_id":"wX","agent":null,"label":"shell","cwd":"/w"}'
fi
case "$1 $2" in
  "tab list")       printf '{"result":{"tabs":[{"tab_id":"wX:t1","label":"%s","workspace_id":"wX","pane_count":2,"agent_status":"-"}]}}\n' "$tab" ;;
  "pane list")      printf '{"result":{"panes":[%s]}}\n' "$P" ;;
  "workspace list") printf '{"result":{"workspaces":[{"workspace_id":"wX","label":"%s"}]}}\n' "$ws" ;;
  "agent list")     printf '{"result":{"agents":[]}}\n' ;;
  "pane split")     printf '{"result":{"pane":{"pane_id":"wX:p9","tab_id":"wX:t1","workspace_id":"wX"}}}\n' ;;
  "pane layout")    printf '{"result":{"panes":[{"pane_id":"wX:p2"}]}}\n' ;;
  "agent start")    printf '{"result":{}}\n' ;;
  "agent get")      printf '{"result":{"agent":{"cwd":"/w","agent":"claude","agent_status":"idle","tokens":{}}}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
chmod +x "$TMP/bin/herdr"

T=zz-agent-vs-container
hw_live() {  # hw_live <rc-var-is-echoed> — runs the real hw, hermetically
  local rc=0
  env HOME="$TMP" HW_INVOKER_PANE= "$@" "$ROOT/bin/hw" setup "$T" \
    --no-brief --no-report > "$TMP/out" 2>&1 || rc=$?
  printf '%s' "$rc"
}
tail_of() { sed 's/\x1b\[[0-9;]*m//g' "$TMP/out" | tail -1; }

rc="$(hw_live STUB_TAB_LABEL="setup:$T" STUB_TAB_AGENT=0)"
case "$rc:$(tail_of)" in
  1:*"holds NO agent pane"*) pass "container: a tab that exists with no agent pane is REFUSED, and hw exits 1" ;;
  *) fail "container: agentless tab -> exit $rc: $(tail_of)" ;;
esac
rc="$(hw_live STUB_TAB_LABEL="setup:$T" STUB_TAB_AGENT=1)"
case "$rc:$(tail_of)" in
  0:*"an agent is live in it"*) pass "container: a tab that exists WITH a live agent is still reused, exit 0" ;;
  *) fail "container: intact tab -> exit $rc: $(tail_of)" ;;
esac

sp_live() {
  local rc=0
  env HOME="$TMP" HW_INVOKER_PANE= "$@" "$ROOT/bin/hw" setup "$T" --space \
    --no-brief --no-report > "$TMP/out" 2>&1 || rc=$?
  printf '%s' "$rc"
}
rc="$(sp_live STUB_WS_LABEL="setup:$T" STUB_TAB_AGENT=0)"
case "$rc:$(tail_of)" in
  1:*"holds NO agent pane"*) pass "container: a legacy space with no agent pane is REFUSED the same way" ;;
  *) fail "container: agentless space -> exit $rc: $(tail_of)" ;;
esac
rc="$(sp_live STUB_WS_LABEL="setup:$T" STUB_TAB_AGENT=1)"
case "$rc:$(tail_of)" in
  0:*"an agent is live in it"*) pass "container: a legacy space WITH a live agent is still reused, exit 0" ;;
  *) fail "container: intact space -> exit $rc: $(tail_of)" ;;
esac

# The count must come from the PANE list, which carries the vendor per pane —
# not from `pane_count` (a lone shell counts) and not from the tab's
# `agent_status`, which herdr prints as "-" for both "no agent" and "not yet
# classified".
sed -n '/^_live_agent_panes_in_tab() {/,/^}/p' "$ROOT/bin/hw" | grep -q 'agent != null' \
  || fail "container: the agent count no longer asks the pane list for a vendor"
pass "container: the check counts panes herdr reports an agent for, not pane_count or a tab status"

# ── a launch whose agent never starts is a FAILED launch ───────────────────
# The stub's `agent start` returns {} — no interactive_ready — so _agent_start
# fails on its first pass, which is the shape of the real failure.
rc=0
env HOME="$TMP" HW_INVOKER_PANE= HERDR_PANE_ID=wX:p2 STUB_TAB_LABEL=none STUB_WS_LABEL=none \
  "$ROOT/bin/hw" setup "$T" --here --no-brief --no-report > "$TMP/out" 2>&1 || rc=$?
out="$(sed 's/\x1b\[[0-9;]*m//g' "$TMP/out" || true)"
case "$rc:$out" in
  1:*"did NOT start"*"NOT running"*) pass "launch: an agent that fails to start exits 1 and says the task is not running" ;;
  *) fail "launch: _agent_start failure -> exit $rc: $(printf '%s' "$out" | tail -1)" ;;
esac

# THE `&&` LIST THAT WAS TWO BUGS IN ONE LINE. `[ -n "$BRIEF" ] && warn …` is an
# && list whose exit status is the test's, so under `set -e` a launch with NO
# brief died on that line — before the agent_ready receipt was written — while a
# launch WITH one carried on to exit 0.
grep -q '\[ -n "\$BRIEF" \] && warn "brief NOT delivered' "$ROOT/bin/hw" \
  && fail "launch: the '[ -n \$BRIEF ] && warn' form is back — set -e reads its status as the block's"
pass "launch: the brief-not-delivered warning is an if, not an && list whose status set -e will read"
