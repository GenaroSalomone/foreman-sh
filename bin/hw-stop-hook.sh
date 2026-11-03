#!/usr/bin/env bash
#
# hw-stop-hook.sh — a Claude Code/Codex `Stop` hook that makes the harness carry
# turn-end facts an agent keeps forgetting to act on.
#
#   ~/.claude/settings.json  → hooks.Stop → bash '<this file>' stop
#   ~/.codex/hooks.json      → hooks.Stop → bash '<this file>' stop
#
# WHY THIS EXISTS
#
# `done-invoker` is opt-in: the agent has to choose to call it. Twice now an
# executor has finished its work, saved its findings to engram, and never
# called it — the pane stayed alive and idle while the brainer, which cannot
# poll, went on believing the task was running. The instruction side of this is
# already as loud as prose gets (CLAUDE.shared.md, "Closing is not optional").
# Prose was not enough. So the signal now comes from the harness, which fires
# whether or not the agent remembered anything.
#
# WHAT THIS HOOK HONESTLY CLAIMS — read this before extending it
#
# A `Stop` hook fires at the end of a TURN. It cannot tell "the task is
# finished" from "this turn is finished": an executor that has just asked the
# brainer a question, or that stopped to think, or that is one turn into a
# ten-turn job, ends a turn exactly like one that has quietly finished. So this
# hook does NOT write a done marker, does NOT prompt the brainer, and does not
# claim completion anywhere.
#
# What it establishes, and this is all it establishes:
#
#     this executor's turn ended at time T, and it has still not reported done
#
# That is precisely the signal that was missing. "Idle and silent" is a state a
# human or a brainer can act on — it is not "failed", and `hw status` words it
# that way on purpose.
#
# WHERE THE SIGNAL GOES
#
# Into `pane.report_metadata` tokens on the executor's own pane, through
# invoker-common.sh's `invoker_publish` — the same publishing path the invokers
# use, not a second copy of it. Tokens survive without a keystroke and are
# readable with `herdr agent list`; `hw status` reads them from there.
#
# THE TOKEN BUDGET IS THE REASON THIS IS A SEPARATE PUBLISH. herdr caps tokens
# at 16 keys PER REQUEST and merges into whatever the pane already has.
# ask-invoker's publish is 15/16 and done-invoker's is 16/16 —
# both exactly full, so a fixed key cannot be added inside either. A small
# publish of our own merges alongside them and takes nothing away. The three
# keys written here (`turn_state`, `turn_ended_at`, `turns`) appear in neither
# invoker's key set, so no publish deletes another's keys.
#
# GUARDS. Every wired Claude Code/Codex session on this machine runs this file.
# It has two narrow jobs: publish an unreported executor turn, and refuse an
# explicit handback from a brainer. Everything else is inert.
#
# IT MUST NEVER BREAK A SESSION: `set -e` is deliberately NOT set and an EXIT
# trap forces status 0 on every exit of this file (herdr absent, hw-core not
# built); once it execs hw-core, the status is hw-core's.
#
set -uo pipefail
trap 'exit 0' EXIT

# ── guard 1: the action this file was wired for ────────────────────────────
# Mirrors ~/.claude/hooks/herdr-agent-state.sh, which takes its event as argv[1]
# rather than trusting the payload. Anything else is not ours.
case "${1:-}" in
  stop) ;;
  *) exit 0 ;;
esac

# ── guard 2: are we inside a herdr pane at all ─────────────────────────────
[ "${HERDR_ENV:-}" = "1" ]          || exit 0
[ -n "${HERDR_SOCKET_PATH:-}" ]     || exit 0
[ -n "${HERDR_PANE_ID:-}" ]         || exit 0

# ── the Go implementation: hw-core stop-hook (core/stop_hook.go) ───────────
# 0.7.0 removed the bash body: this file stays the registered hook (install.sh, ~/.claude/settings.json, ~/.codex/hooks.json) and
# execs `hw-core stop-hook` with this hook's stdin untouched. HW_IMPL_STOP_HOOK or a `stop-hook=bash` line is retired: bin/impl-retired.sh
# says so once and Go runs. hw-core is HW_CORE (a seam of the suite, never rebuilt), else beside this file, else
# core/dist/<os>-<arch>/ (setup/build-core builds it); one not built or older than core/'s sources is rebuilt by bin/core-rebuild
# when a Go toolchain is on PATH. A hook that cannot run Go says so and exits 0: it must never break a session.
_t_src="${BASH_SOURCE[0]}"
while [ -L "$_t_src" ]; do
  _t_dir="$(cd -P "$(dirname "$_t_src")" && pwd)" || break
  _t_src="$(readlink "$_t_src")" || break
  case "$_t_src" in /*) ;; *) _t_src="$_t_dir/$_t_src" ;; esac
done
_t_bin="$(cd -P "$(dirname "$_t_src")" 2>/dev/null && pwd)"
[ ! -r "$_t_bin/impl-retired.sh" ] || { . "$_t_bin/impl-retired.sh"; impl_retired_warn stop-hook; }
_t_core=""
if [ -n "${HW_CORE:-}" ]; then
  [ ! -x "$HW_CORE" ] || _t_core="$HW_CORE"
else
  _t_os="$(uname -s | tr 'A-Z' 'a-z')"; _t_arch="$(uname -m)"
  case "$_t_arch" in x86_64) _t_arch=amd64 ;; aarch64|arm64) _t_arch=arm64 ;; esac
  for _t_c in "$_t_bin/hw-core" "$_t_bin/../core/dist/$_t_os-$_t_arch/hw-core"; do
    if [ -x "$_t_c" ]; then _t_core="$_t_c"; break; fi
  done
  if [ -n "$_t_core" ] && [ -d "$_t_bin/../core" ] \
     && [ -n "$(find "$_t_bin/../core" -path '*/dist' -prune -o \( -name '*.go' -o -name '*.txt' -o -name go.mod \) -newer "$_t_core" -print -quit 2>/dev/null)" ]; then
    _t_msg="$("$_t_bin/core-rebuild" "$_t_bin" "$_t_core" stop-hook 2>&1)" || :   # stale: rebuilt (bin/core-rebuild), or the old one runs with one line
    [ -z "$_t_msg" ] || printf 'hw-stop-hook.sh: %s\n' "$_t_msg" >&2
  fi
fi
if [ -n "$_t_core" ]; then
  export HW_CORE_BIN_DIR="$_t_bin" HW_CORE_ROOT="${_t_bin%/*}"
  export HW_BRAIN_ROOT="${HW_BRAIN_ROOT:-${_t_bin%/*}}"
  exec "$_t_core" stop-hook "$@" # MUTATION-ANCHOR: 43-M06
fi
printf 'hw-stop-hook.sh: hw-core is not built: run setup/build-core --native (it needs Go 1.22 or newer, https://go.dev/dl)\n' >&2
exit 0   # a hook must never break a session
