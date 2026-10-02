#!/usr/bin/env bash
# Report Codex's thread id to Herdr as soon as a user prompt is submitted.
set -uo pipefail  # MUTATION-ANCHOR: 102-M04
# NATIVE WINDOWS (Git Bash): bin/msys-compat.sh makes the native programs this
# file runs (Python, jq, fd, git) answer in bash's path spelling and with LF,
# and asks msys for real symlinks. Elsewhere OSTYPE never matches. A copy of this
# file with no msys-compat.sh beside it (a test fixture) runs on the layer its parent exported.
case "${OSTYPE:-}" in msys*|cygwin*) HW_MSYS_COMPAT="$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/msys-compat.sh"; [ ! -f "$HW_MSYS_COMPAT" ] || . "$HW_MSYS_COMPAT" ;; esac

# NO `trap 'exit 0' EXIT`, and its absence is the fix.
#
# This file used to open with one, and close with
# `herdr-rpc call pane.report_agent_session … >/dev/null 2>&1 || true`. Between
# the two, every path exited 0 — including the path where the pane→session-id
# mapping never landed. That mapping is the whole reason the hook exists: it is
# what lets anything address this Codex session at all. A hook that reports
# success whether or not it published is a hook whose signal carries no
# information, which is the failure this whole toolchain is about.
#
# NOT APPLICABLE and FAILED are now two different exits. The guards below are
# "this invocation is not ours" — a different hook event, no herdr, no pane —
# and those are genuinely exit 0. A publication that was attempted and cannot be
# proved is exit 1, and it says which half failed.
_hook_fail() {
  printf 'codex-channel-session-hook: %s\n' "$*" >&2
  # DURABLE, because a hook's stderr is not something anyone reads later. This
  # is the only place the failure survives the invocation.
  if [ -n "${HERDR_PANE_ID:-}" ]; then
    printf '%s\t%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" \
      >> "${TMPDIR:-/tmp}/hw-codex-session-$(printf '%s' "$HERDR_PANE_ID" | tr -c 'A-Za-z0-9_.-' '_').err" \
      2>/dev/null || true
  fi
  exit 1
}

[ "${1:-}" = session ] || exit 0
[ "${HERDR_ENV:-}" = 1 ] || exit 0
[ -n "${HERDR_PANE_ID:-}" ] || exit 0
[ -n "${HERDR_SOCKET_PATH:-}" ] || exit 0

INPUT="$(head -c 262144 2>/dev/null || true)"
SESSION_ID="$(printf '%s' "$INPUT" | jq -r '.session_id // empty' 2>/dev/null || true)"
[ -n "$SESSION_ID" ] || exit 0
SEQ="$(python3 -c 'import time; print(time.time_ns())' 2>/dev/null || true)"
[ -n "$SEQ" ] || exit 0

SCRIPT_DIR="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)" \
  || _hook_fail "cannot resolve my own directory, so herdr-rpc could not be found"
if ! PARAMS="$(jq -nc \
  --arg pane "$HERDR_PANE_ID" \
  --arg session "$SESSION_ID" \
  --argjson seq "$SEQ" \
  '{pane_id:$pane,source:"hw:codex-channel",agent:"codex",seq:$seq,agent_session_id:$session}' \
  2>/dev/null)"; then
  _hook_fail "could not build the report_agent_session params for pane $HERDR_PANE_ID"
fi
[ -n "$PARAMS" ] || _hook_fail "built empty report_agent_session params for pane $HERDR_PANE_ID"

RPC_OUT="$("$SCRIPT_DIR/herdr-rpc" call pane.report_agent_session "$PARAMS" 2>&1)" \
  || _hook_fail "pane.report_agent_session failed for pane $HERDR_PANE_ID session $SESSION_ID: $(printf '%s' "$RPC_OUT" | head -2 | tr '\n' ' ')"

# AND READ IT BACK, because "the RPC returned 0" is not "the pane carries the
# mapping". `agent.get` exposes `agent_session.value`, so the fact this hook
# exists to establish is directly checkable — verified on the live daemon
# 2026-09-08: `herdr-rpc call agent.get '{"target":"<pane>"}'` answers with
# `.agent.agent_session = {source, agent, kind, value}`, unwrapped.
READ_OUT="$("$SCRIPT_DIR/herdr-rpc" call agent.get \
  "$(jq -nc --arg t "$HERDR_PANE_ID" '{target:$t}')" 2>&1)" \
  || _hook_fail "published the session for pane $HERDR_PANE_ID but could not read it back: $(printf '%s' "$READ_OUT" | head -2 | tr '\n' ' ')"
LANDED="$(printf '%s' "$READ_OUT" | jq -r '.agent.agent_session.value // empty' 2>/dev/null || true)"
[ "$LANDED" = "$SESSION_ID" ] \
  || _hook_fail "pane $HERDR_PANE_ID reports session '${LANDED:-none}', not the '$SESSION_ID' just published — the mapping did not land"
