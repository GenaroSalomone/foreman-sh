#!/usr/bin/env bash
# Report Codex's thread id to Herdr as soon as a user prompt is submitted.
set -uo pipefail

# ── the Go implementation: hw-core (core/wrappers_small.go)
# 0.7.0 removed the bash body: this file keeps the name (the interface, setup/release-standard.md «Versioning») and execs
# `hw-core codex-channel-session-hook` with the same argv, stdin and environment. A HW_IMPL_CODEX_CHANNEL_SESSION_HOOK or `codex-channel-session-hook=bash` word is retired: bin/impl-retired.sh says so
# once and Go runs. hw-core is HW_CORE (a seam of the suite, never rebuilt), else beside this file, else core/dist/<os>-<arch>/
# (setup/build-core builds it); one not built or older than core/'s sources is rebuilt by bin/core-rebuild when a toolchain is on PATH.
set -euo pipefail
_t_src="${BASH_SOURCE[0]}"
while [ -L "$_t_src" ]; do
  _t_dir="$(cd -P "$(dirname "$_t_src")" && pwd)" || break
  _t_src="$(readlink "$_t_src")" || break
  case "$_t_src" in /*) ;; *) _t_src="$_t_dir/$_t_src" ;; esac
done
_t_bin="$(cd -P "$(dirname "$_t_src")" 2>/dev/null && pwd)"
[ ! -r "$_t_bin/impl-retired.sh" ] || { . "$_t_bin/impl-retired.sh"; impl_retired_warn codex-channel-session-hook; }
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
    _t_msg="$("$_t_bin/core-rebuild" "$_t_bin" "$_t_core" codex-channel-session-hook 2>&1)" || :   # stale: rebuilt (bin/core-rebuild), or the old one runs with one line
    [ -z "$_t_msg" ] || printf 'codex-channel-session-hook: %s\n' "$_t_msg" >&2
  fi
fi
if [ -n "$_t_core" ]; then
  HW_WRAPPER_ARGV0="$0" HW_CORE_BIN_DIR="$_t_bin" exec "$_t_core" codex-channel-session-hook "$@"
fi
printf 'codex-channel-session-hook: hw-core is not built: run setup/build-core --native (it needs Go 1.22 or newer, https://go.dev/dl)\n' >&2
exit 1
