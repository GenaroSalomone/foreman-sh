#!/usr/bin/env bash
# A REAL pi in a throwaway herdr workspace, working on a long foreground command (a local scripted provider, no quota), and
# `hw ruling` aimed at its pane: the ruling must reach pi's session AS STEERING, right after the tool call in flight, with no
# wait for the turn end (retro-fixes-rc1 item 7). Prints `ok - …` / `not ok - …`; exit 1 if any check failed.
#
#   setup/pi/test-live-steer.sh            # the tree's hw
#   HW_CORE=<an older hw-core> setup/pi/test-live-steer.sh   # OLD: the ruling only queues, pi never sees it mid-turn
#
# The workspace is created here and closed on exit; nothing is written outside a temp dir and the workspace.
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NODE=(node --experimental-strip-types --no-warnings)
T="$(mktemp -d "${TMPDIR:-/tmp}/live-steer.XXXXXX")"; W="$T/wd"; mkdir -p "$W/.hw/20261105-100000-1" "$T/home"; printf '1\n' > "$W/.hw/20261105-100000-1/task"
FAILS=0
ok() { if [ "$1" = 0 ]; then printf 'ok - %s\n' "$2"; else printf 'not ok - %s\n' "$2"; FAILS=$((FAILS + 1)); fi; }
WS=""; STUB_PID=""
cleanup() { [ -z "$WS" ] || herdr workspace close "$WS" >/dev/null 2>&1; [ -z "$STUB_PID" ] || kill "$STUB_PID" 2>/dev/null; rm -rf "$T"; }
trap cleanup EXIT
waitfor() { local n="$1"; shift; local i=0; while [ "$i" -lt "$n" ]; do "$@" && return 0; i=$((i + 1)); /bin/sleep 1; done; return 1; }

"${NODE[@]}" "$ROOT/setup/pi/stub-provider-serve.mjs" "sleep 30; echo LIVE-DONE" "$T/stub.json" "$T/requests.jsonl" &
STUB_PID=$!
waitfor 20 test -s "$T/stub.json" || { ok 1 "the local provider started"; exit 1; }
AGENT_DIR="$(jq -r .agentDir "$T/stub.json")"

created="$(herdr workspace create --cwd "$W" --label hw-steer-probe --no-focus \
  --env HW_TASK=probe --env HW_RUN=20261105-100000-1 --env HW_PROJECT=setup --env "HW_WORKDIR=$W" --env "HW_BRAIN_ROOT=$ROOT" \
  --env "PI_CODING_AGENT_DIR=$AGENT_DIR" --env ENGRAM_PORT=9 --env "HOME=$T/home" 2>&1)" || { ok 1 "a probe workspace was created: $created"; exit 1; }
WS="$(printf '%s' "$created" | jq -r '.result.workspace.workspace_id // empty' 2>/dev/null)"
PANE="$(printf '%s' "$created" | jq -r '.result.root_pane.pane_id // .result.pane.pane_id // empty' 2>/dev/null)"
if [ -z "$WS" ] || [ -z "$PANE" ]; then ok 1 "the workspace answer names a workspace and a pane: $created"; exit 1; fi
ok 0 "a throwaway herdr workspace was created ($WS, pane $PANE)"

herdr agent start probe --kind pi --pane "$PANE" --timeout 60000 -- --no-session -ne -na -ns -nc -e "$ROOT/setup/pi/hw-executor.ts" \
  --model stub/stub --thinking off >"$T/start.log" 2>&1 || { ok 1 "a real pi started in the pane: $(head -c 300 "$T/start.log")"; exit 1; }
ok 0 "a real pi started in the pane"
herdr agent prompt "$PANE" "run the command" >/dev/null 2>&1 || { ok 1 "the first prompt reached pi"; exit 1; }

# pi is now inside the 30 s foreground command: its provider has been asked once, and herdr calls the pane working
req1() { [ "$(wc -l < "$T/requests.jsonl" | tr -d ' ')" -ge 1 ]; }
waitfor 40 req1 || { ok 1 "pi asked the provider for its first turn"; exit 1; }
status() { [ "$(herdr agent get "$PANE" 2>/dev/null | jq -r '.result.agent.agent_status // empty')" = working ]; }
waitfor 20 status; ok $? "herdr reports the pi pane as working during the long command"

# `hw ruling` is a BRAINER verb: it runs here from a clean environment, the one a brainer's shell has, not from this
# script's own (an executor shell sets HW_TASK/HW_RUN and the gate refuses it, which is right).
out="$(cd "$ROOT" && env -i HOME="${HOME:-/}" PATH="$PATH" USER="${USER:-x}" ${HW_CORE:+HW_CORE="$HW_CORE"} bin/hw ruling "$PANE" "PROBE-RULING: use the staging base, not main" 2>&1)"
printf '%s\n' "$out" | sed 's/^/# hw ruling: /' | head -4
case "$out" in *"NOW through pi's native steering"*) ok 0 "hw ruling says it delivered through pi's native steering" ;; *) ok 1 "hw ruling did not steer (it queued): $(printf '%s' "$out" | head -c 200)" ;; esac

# the second provider request is made when the tool call ends: the steered text must be IN it, with the tool result
req2() { [ "$(wc -l < "$T/requests.jsonl" | tr -d ' ')" -ge 2 ]; }
waitfor 60 req2 || { ok 1 "pi made its second request after the long command"; exit 1; }
second="$(sed -n 2p "$T/requests.jsonl")"
printf '%s' "$second" | grep -q 'LIVE-DONE'; ok $? "the second request carries the command's result (the tool call ended)"
printf '%s' "$second" | grep -q 'PROBE-RULING'; ok $? "the second request ALREADY carries the ruling: it landed as steering after the tool call, not at a turn end"
[ -z "$(ls "$W/.hw/20261105-100000-1"/pending-ruling* 2>/dev/null)" ]; ok $? "nothing is left pending for the turn end"
[ -n "$(ls "$W/.hw/20261105-100000-1"/ruling-delivered.* 2>/dev/null)" ]; ok $? "the delivery is recorded"
[ "$FAILS" = 0 ]
