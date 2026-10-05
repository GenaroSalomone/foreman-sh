#!/usr/bin/env bash
# The live herdr path, driven for real, with no paid model.
#
# Every other subject runs with herdr stubbed (KNOWN-LIMITATIONS L9), so the
# path hw takes through a real herdr server was only ever exercised by the paid
# e2e at release cut. The defects that reached that cut or the maintainer sat on this
# path: "✓ brief delivered" while the executor held the text unsent
# (decisions 2026-08-24), a report lost with exit 0, a wait on a turn state the
# parse dropped — and a flag green on 167 tests that printed nothing live
# (setup/CLAUDE.md, "The stubbed path is not the path").
#
# What is real here: a private herdr server (its own XDG_CONFIG_HOME, so its
# own socket and session list), `hw <lane> <task>` building the tab and typing
# the agent into it, the brief going through `herdr agent prompt`, the Stop
# hook's turn tokens through pane.report_metadata, `done-invoker` →
# `channel-send --report` into a brainer pane, and `hw done` closing the tab.
# What is fake: the agent. `claude` on the panes' PATH is a shell loop that
# reports working/idle with `herdr pane report-agent` and runs the same Stop
# hook Claude Code runs. No model is called.
#
# Nothing here touches the operator's herdr: the server, its workspaces and its
# session live under this run's own directories and are closed by the EXIT trap,
# on failure too. Engram is never reached (port 9, from _common.sh).
#
# Without herdr the subject skips with the reason; it never passes.
#
# HW650_HW: run against another bin/hw. The mutation arm at the end re-runs this
# file with a copy whose `hw done` cannot close the tab, and must see it red.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# _common.sh put herdr and herdr-rpc stubs first on PATH. This subject is the
# one that must reach the real ones.
rm -f "$TMP/bin/herdr" "$TMP/bin/herdr-rpc"
hash -r

# On native Windows a missing tool is a failure, not a skip: there the gate
# exempts 650's arm by name (setup/mutation-exempt-native-windows.tsv) for the
# stop below, and a skip here would ride that into a pass.
for _p in herdr jq python3; do
  command -v "$_p" >/dev/null 2>&1 && continue
  case "${OSTYPE:-}" in
    msys*|cygwin*) fail "650: $_p is not installed; on native Windows that is not a skip" ;;
  esac
  printf 'skip - 650: %s is not installed; the live herdr path cannot run here\n' "$_p"; exit 0
done

# ── native Windows stops here, by name ─────────────────────────────────────
# hw's own dispatch does not run against a live herdr there: in the executor
# pane hw built, the agent was never typed (_agent_start failed) and the pane
# opened in the runner's home, not the workdir (windows.yml 37088978624);
# KNOWN-LIMITATIONS L1b lists it as unverified. The brainer side that used to
# run before this stop needed three Windows-only workarounds and then timed out
# starting its fake in the full run (37090376222) while it started in the
# survey: it proved nothing about hw, so it no longer runs there. What the
# layers were: this file at 339ce50.
case "${OSTYPE:-}" in
  msys*|cygwin*)
    printf 'skip - 650: hw against a live herdr does not run on native Windows: in the pane hw builds the agent is never started and the pane opens outside the workdir (KNOWN-LIMITATIONS L1b)\n'
    exit 0 ;;
esac

# A unix socket path is capped at 104 bytes on macOS; $TMP under TMPDIR is
# already most of that. herdr puts its sessions under XDG_CONFIG_HOME.
SHORT="$(mktemp -d /tmp/h650.XXXXXX)"
export XDG_CONFIG_HOME="$SHORT"
SESSION="t650-$$"
SOCK="$SHORT/herdr/sessions/$SESSION/herdr.sock"
# _common.sh points this at a socket that never exists; the herdr CLI and every
# absolute-path herdr-rpc caller must reach the private server instead.
export HERDR_SOCKET_PATH="$SOCK"
SERVER_PID=""
LANE=h650
TASK=h650-probe

# Closes what this run created, and only that: every workspace of the private
# session, the session, the server. Idempotent — the body calls it to assert
# on the result, and the EXIT trap calls it again on any failure.
close_workspaces() {
  [ -S "$SOCK" ] || return 0
  for ws in $(herdr --session "$SESSION" workspace list 2>/dev/null | jq -r '.result.workspaces[].workspace_id' 2>/dev/null); do
    herdr --session "$SESSION" workspace close "$ws" >/dev/null 2>&1 || true
  done
}
teardown() {
  close_workspaces
  herdr session stop "$SESSION" --json >/dev/null 2>&1 || true
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    for _ in 1 2 3 4 5 6 7 8 9 10; do kill -0 "$SERVER_PID" 2>/dev/null || break; sleep 0.2; done
    kill -9 "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true   # reap it: a zombie still answers kill -0
    SERVER_PID=""   # the trap runs this again; a reaped pid may be someone else's by then
  fi
  herdr session delete "$SESSION" --json >/dev/null 2>&1 || true
}
cleanup() {
  local rc=$?
  # HW650_KEEP_EV=<dir>: keep what the fakes wrote, for diagnosis.
  [ -z "${HW650_KEEP_EV:-}" ] || cp -R "$TMP/ev" "$HW650_KEEP_EV" 2>/dev/null || true
  teardown
  rm -rf "$SHORT" "$TMP"
  exit "$rc"
}
trap cleanup EXIT

# ── a brain of our own, with one temporary repoless lane ────────────────────
BRAIN="$TMP/brain"
mkdir -p "$BRAIN/setup" "$BRAIN/$LANE/briefs" "$TMP/work" "$TMP/ev"
for d in bin layouts lanes; do cp -R "$ROOT/$d" "$BRAIN/$d"; done
cp -R "$ROOT/setup/guards" "$BRAIN/setup/guards"
for f in CLAUDE.md CLAUDE.shared.md guards.json; do [ ! -f "$ROOT/$f" ] || cp "$ROOT/$f" "$BRAIN/$f"; done
[ -z "${HW650_HW:-}" ] || cp "$HW650_HW" "$BRAIN/bin/hw"
python3 - "$ROOT/projects.json" "$BRAIN/projects.json" "$TMP/work" "$LANE" <<'PY'
import json, sys
t = json.load(open(sys.argv[1]))
t["work"] = sys.argv[3]
t["lanes"][sys.argv[4]] = {"hw_aliases": [], "brain_aliases": [], "space": "h650-space",
    "engram": "brain", "vendor": "claude", "model": "", "account": "default",
    "requested_by": "warn", "repoless": True}
json.dump(t, open(sys.argv[2], "w"), indent=2)
PY
export HW_BRAIN_ROOT="$BRAIN" HW_PROJECTS_JSON="$BRAIN/projects.json"
printf '# h650 — a temporary lane for setup/tests/650\n' > "$BRAIN/$LANE/CLAUDE.md"
cat > "$BRAIN/$LANE/briefs/$TASK.md" <<'EOF'
---
requested_by: "setup/tests/650, a disposable live probe"
asked: investigate
---
# h650-probe — BRIEF-650-MARKER

A disposable brief. The agent reading it is a shell loop, not a model.
EOF

# ── the fake agent: a shell loop that speaks herdr's lifecycle ──────────────
FAKE="$TMP/fakebin"
mkdir -p "$FAKE"
cat > "$FAKE/claude" <<EOF
#!/bin/bash
# setup/tests/650's stand-in for Claude Code. Reports its state to herdr the
# way an integration does; an executor (HW_RUN set) runs the real Stop hook at
# each turn end, and done-invoker when told REPORT-650.
# It publishes NO session id: measured on herdr 0.9.1, once a pane carries a
# herdr:claude session herdr reads Claude's screen for the state and ignores
# report-agent, so the brief "never opens a turn". Real Claude is covered there
# by its own screen; this stand-in has none to offer.
EV='$TMP/ev'
BIN='$BRAIN/bin'
role=brainer; [ -n "\${HW_RUN:-}" ] && role=executor
rep() { herdr pane report-agent --source t650:fake --agent claude --state "\$1" "\$HERDR_PANE_ID" >/dev/null 2>&1; }
printf '%s %s\n' "\$role" "\$HERDR_PANE_ID" >> "\$EV/started"
echo "FAKE-CLAUDE-650 \$role ready"
rep idle
n=0
while IFS= read -r line; do
  turn="\$line"
  while IFS= read -r -t 1 more; do turn="\$turn
\$more"; done
  rep working; n=\$((n + 1))
  if [ "\$role" = brainer ]; then
    printf '%s\n--- end of turn %s ---\n' "\$turn" "\$n" >> "\$EV/brainer.log"
    sleep 1; rep idle; continue
  fi
  printf '%s\n' "\$turn" > "\$EV/executor-turn-\$n"
  case "\$turn" in
    *REPORT-650*)
      "\$BIN/done-invoker" "650 live probe complete: REPORT-650-SUMMARY" > "\$EV/done-invoker.out" 2>&1
      : > "\$EV/done-invoker.returned" ;;  # only reached if hw done did NOT close this pane
    *) sleep 1 ;;
  esac
  printf '{"hook_event_name":"Stop"}' | bash "\$BIN/hw-stop-hook.sh" stop > "\$EV/stop-hook-\$n.out" 2>&1
  rep idle
done
EOF
chmod +x "$FAKE/claude"
# Panes get this shell: no profile and no rc, so nothing reorders PATH and the
# fake is the only `claude` the pane can resolve.
printf '#!/bin/sh\nexec /bin/bash --noprofile --norc -i\n' > "$FAKE/sh650"
chmod +x "$FAKE/sh650"
export PATH="$FAKE:$PATH"
export SHELL="$FAKE/sh650"

h() { herdr --session "$SESSION" "$@"; }
waitfile() {  # $1=path $2=seconds
  local end=$((SECONDS + $2)); while [ ! -e "$1" ]; do [ "$SECONDS" -lt "$end" ] || return 1; sleep 0.2; done
}

# ── the private server ──────────────────────────────────────────────────────
herdr --session "$SESSION" server >/dev/null 2>&1 &
SERVER_PID=$!
_end=$((SECONDS + 20))
until h workspace list >/dev/null 2>&1; do
  [ "$SECONDS" -lt "$_end" ] || fail "650 the private herdr server did not come up in 20s"
  kill -0 "$SERVER_PID" 2>/dev/null || fail "650 the private herdr server exited during startup"
  sleep 0.1
done

created="$(h workspace create --cwd "$BRAIN/$LANE" --label h650-brain --no-focus)"
BRAINER="$(printf '%s' "$created" | jq -r '.result.root_pane.pane_id')"
[ -n "$BRAINER" ] && [ "$BRAINER" != null ] || fail "650 could not create the brainer workspace: $created"
h pane rename "$BRAINER" brain >/dev/null
for _ in $(seq 1 15); do
  out="$(h agent start brain650 --kind claude --pane "$BRAINER" --timeout 20000 2>&1 || true)"
  case "$out" in *agent_pane_busy*) sleep 1 ;; *) break ;; esac
done
# A start that never gets ready says why only on the pane's screen: on Git Bash
# it timed out with nothing else to read (windows.yml 37084055355).
printf '%s' "$out" | jq -e '.result.agent.interactive_ready == true' >/dev/null 2>&1 \
  || fail "650 the fake brainer did not start: $out
  pane process: $(h pane process-info --pane "$BRAINER" 2>&1 | head -c 800)
  pane screen: $(h pane read "$BRAINER" --source recent --lines 40 2>&1 | tail -n 40)
  fake started: $(cat "$TMP/ev/started" 2>/dev/null || echo never)"
waitfile "$TMP/ev/started" 10 && grep -q "^brainer $BRAINER" "$TMP/ev/started" \
  || fail "650 the brainer pane did not run the fake claude — refusing to go on (a real one would spend)"

# ── the dispatch, through the real hw ───────────────────────────────────────
export HERDR_ENV=1 HERDR_SOCKET_PATH="$SOCK" HERDR_PANE_ID="$BRAINER"
# No session id is published (see the fake), so hw's wait for one is cut short;
# the receipt then records `none — …` and the Stop hook does not narrow.
export HW_SESSION_WAIT_MS=2000
dispatch="$("$BRAIN/bin/hw" "$LANE" "$TASK" --sdd none --fresh 2>&1)" || {
  ap="$(printf '%s' "$dispatch" | sed -n 's/.*agent=\([^ ]*\).*/\1/p' | head -1)"
  fail "650 hw $LANE $TASK failed: $dispatch
  agent pane ${ap:-?} screen: $([ -z "$ap" ] || h pane read "$ap" --source recent --lines 30 2>&1 | tail -n 30)"
}
EXEC="$(grep -m1 "^executor " "$TMP/ev/started" 2>/dev/null | awk '{print $2}' || true)"
[ -n "$EXEC" ] || fail "650 hw dispatched but no fake executor started: $dispatch"
pass "650 hw $LANE $TASK built a real tab and started the agent in $EXEC"
# hw's own verdict on the delivery must agree with the executor's: the
# 2026-08-24 defect was a green "brief delivered" over an idle executor.
case "$dispatch" in
  *"THE BRIEF IS NOT IN"*|*agent_prompt_stalled*) fail "650 hw says the brief did not land: $dispatch" ;;
esac

waitfile "$TMP/ev/executor-turn-1" 30 || fail "650 the executor never received a turn: $dispatch"
grep -q BRIEF-650-MARKER "$TMP/ev/executor-turn-1" \
  || fail "650 the executor's first turn is not the brief: $(head -c 400 "$TMP/ev/executor-turn-1")"
pass "650 the brief reached the executor through herdr agent prompt"

waitfile "$TMP/ev/stop-hook-1.out" 30 || fail "650 the Stop hook never ran"
# A turn that ends with no report, ask or boundary on disk is a handback; the
# hook refuses it and publishes that verdict as the turn's state.
tok=""; _end=$((SECONDS + 15))
while [ "$SECONDS" -lt "$_end" ]; do
  tok="$(h agent get "$EXEC" 2>/dev/null | jq -r '.result.agent.tokens | "\(.turn_state // "") \(.turn_ended_at // "") \(.hw_run // "")"')"
  case "$tok" in " "*) sleep 0.3 ;; *) break ;; esac
done
set -- $tok
[ "${1:-}" = handback_refused ] && [ -n "${2:-}" ] && [ -n "${3:-}" ] \
  || fail "650 the Stop hook did not publish turn_state=handback_refused with turn_ended_at and hw_run on $EXEC (got '$tok'): $(cat "$TMP/ev/stop-hook-1.out")"
grep -q '"decision":"block"' "$TMP/ev/stop-hook-1.out" \
  || fail "650 the Stop hook did not refuse the unreported turn: $(cat "$TMP/ev/stop-hook-1.out")"
pass "650 the Stop hook refused the unreported turn and published turn_state=handback_refused on the live pane"

EXEC_TAB="$(h pane get "$EXEC" | jq -r '.result.pane.tab_id')"
h agent prompt "$EXEC" "REPORT-650 now" >/dev/null 2>&1 || fail "650 could not prompt the executor"
# done-invoker's own exit is not observable: after the report lands it runs
# `hw done`, which closes this very pane, as it does under a real agent. What
# is observable is where the report went and what it left on disk.
_end=$((SECONDS + 90))
until grep -q REPORT-650-SUMMARY "$TMP/ev/brainer.log" 2>/dev/null; do
  [ "$SECONDS" -lt "$_end" ] || fail "650 the report never reached the brainer pane: $(cat "$TMP/ev/done-invoker.out" 2>/dev/null)"
  sleep 0.3
done
grep -q "^\[executor finished — $LANE:$TASK, pane $EXEC\]" "$TMP/ev/brainer.log" \
  || fail "650 the brainer received something other than the completion envelope: $(head -c 600 "$TMP/ev/brainer.log")"
ls "$TMP/work/$LANE/$TASK/.hw/"*/done >/dev/null 2>&1 \
  || fail "650 the report reached the brainer but the run has no done marker"
pass "650 done-invoker delivered the report into the brainer pane through channel-send --report"

gone=""; _end=$((SECONDS + 30))
while [ "$SECONDS" -lt "$_end" ]; do
  if ! h tab list 2>/dev/null | jq -e --arg t "$EXEC_TAB" '.result.tabs[] | select(.tab_id == $t)' >/dev/null 2>&1; then gone=1; break; fi
  # done-invoker returned in a pane that should be gone: the close is over and failed.
  [ ! -e "$TMP/ev/done-invoker.returned" ] || break
  sleep 0.3
done
[ -n "$gone" ] || fail "650 hw done left the executor's tab $EXEC_TAB open after the report — hw done said: $(grep -E 'CLOSED|NOTHING TO CLOSE|CLOSE FAILED|could not close' "$TMP/ev/done-invoker.out" 2>/dev/null | head -5 | tr '\n' ' ') — last lines: $(tail -3 "$TMP/ev/done-invoker.out" 2>/dev/null | tr '\n' ' ')"
pass "650 hw done closed the executor's tab $EXEC_TAB"

# ── nothing left behind ─────────────────────────────────────────────────────
close_workspaces
left="$(h workspace list 2>/dev/null | jq -r '[.result.workspaces[].label] | join(",")')"
[ -z "$left" ] || fail "650 workspaces survived their close: $left"
_srv="$SERVER_PID"
teardown
herdr session list --json 2>/dev/null | jq -e --arg s "$SESSION" '[.sessions[] | select(.name == $s)] | length == 0' >/dev/null \
  || fail "650 the private herdr session $SESSION survived its teardown"
case "$(ps -p "$_srv" -o command= 2>/dev/null)" in
  *herdr*"$SESSION"*) fail "650 the private herdr server ($_srv) survived its teardown" ;;
esac
pass "650 teardown left no workspace, session or server behind"

# ── the mutant: hw records the tab id it asked for, not the one it got ───────
# layout.apply REPLACES the tab it is given and answers with a new id (bin/hw,
# _build_space). Record the pre-layout id and `hw done` closes a tab that is
# already gone, calls it "nothing to close, not a failure", and leaves the
# executor's real tab running. Every stubbed subject writes its receipt by hand
# and never parses a layout.apply reply, so only a live herdr shows this.
[ -z "${HW650_HW:-}" ] || exit 0
mut="$(mktemp "${TMPDIR:-/tmp}/hw650-mutant.XXXXXX")"
cp "$ROOT/bin/hw" "$mut"
mutate_anchor 650-M01 "$mut" 'live_tab=""'
cmp -s "$ROOT/bin/hw" "$mut" && { rm -f "$mut"; fail "650 M01 the mutant did not change bin/hw"; }
mout="$(HW650_HW="$mut" bash "$0" 2>&1)" && { rm -f "$mut"; fail "650 M01 SURVIVED: a run that recorded the pre-layout tab passed: $mout"; }
rm -f "$mut"
saw_mutant "650 M01 hw done closes the pre-layout tab and leaves the live one open" "$mout" "NOTHING TO CLOSE: tab"
