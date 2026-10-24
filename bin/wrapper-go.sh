# wrapper-go.sh — the switch to the Go twin of five small tools (core/wrappers*.go).
# SOURCED by browser-close, brain-archive, secret-unlock, opencode-auto and codex-channel-session-hook.sh on a line of
# their own just after their header, never executed: no shebang, not executable. A copy of a tool with no wrapper-go.sh
# beside it (a test fixture) simply runs its bash.
#
# THE DEFAULT IS GO (0.6.0), and the bash of each tool is the rollback through 0.6.x. Unless it is told `bash`, the tool execs
# `hw-core <tool> "$@"` with the same argv, stdin and environment. The way back is HW_IMPL_<TOOL>=bash in the environment, else the
# tool's `<tool>=bash` line of the switch file (${HW_IMPL_FILE:-~/.config/hw/impl}). Only the word `bash` selects the bash: the
# global HW_IMPL and the file's default line retired with the switch (go-060 S3) and do not reach these tools, and any other word
# (`shadow`, `rust`) is Go, since there is one way back and it is spelled out.
# A hw-core that is not built, or older than core/'s sources, says so and the bash answers: every time when the environment asked
# for Go, once per state when only the file or the default did (the stamp is bin/hw's, ${XDG_STATE_HOME:-~/.local/state}/hw/warned).
# HW_CORE names a binary; an explicit one is never judged stale.
#
# Callers set, before sourcing: _wg_prog (the tool's name, no .sh) and _wg_bin (the directory the tool really lives in).
_wg_up="$(printf '%s' "$_wg_prog" | tr 'a-z-' 'A-Z_')"
eval "_wg_mode=\"\${HW_IMPL_$_wg_up:-}\""
_wg_from=env
_wg_file="${HW_IMPL_FILE:-${XDG_CONFIG_HOME:-${HOME:-/nonexistent}/.config}/hw/impl}"
if [ -z "$_wg_mode" ]; then
  _wg_from=file
  if [ -r "$_wg_file" ]; then
    while IFS= read -r _wg_line || [ -n "$_wg_line" ]; do
      _wg_line="${_wg_line%%#*}"; _wg_line="${_wg_line//[[:space:]]/}"
      case "$_wg_line" in
        *=*) [ "$(printf '%s' "${_wg_line%%=*}" | tr 'a-z-' 'A-Z_')" != "$_wg_up" ] || _wg_mode="${_wg_line#*=}" ;;
      esac
    done < "$_wg_file"
  fi
fi
if [ "$_wg_mode" != bash ]; then
  _wg_core=""; _wg_why=""
  if [ -n "${HW_CORE:-}" ]; then
    [ ! -x "$HW_CORE" ] || _wg_core="$HW_CORE"
  else
    _wg_os="$(uname -s | tr 'A-Z' 'a-z')"; _wg_arch="$(uname -m)"; _wg_ext=""
    case "$_wg_os" in mingw*|msys*|cygwin*) _wg_os=windows; _wg_ext=.exe ;; esac
    case "$_wg_arch" in x86_64) _wg_arch=amd64 ;; aarch64) _wg_arch=arm64 ;; esac
    for _wg_c in "$_wg_bin/hw-core$_wg_ext" "$_wg_bin/../core/dist/$_wg_os-$_wg_arch/hw-core$_wg_ext"; do
      if [ -x "$_wg_c" ]; then _wg_core="$_wg_c"; break; fi
    done
    if [ -n "$_wg_core" ] && [ -d "$_wg_bin/../core" ] \
       && [ -n "$(find "$_wg_bin/../core" -path '*/dist' -prune -o \( -name '*.go' -o -name '*.txt' -o -name go.mod \) -newer "$_wg_core" -print -quit 2>/dev/null)" ]; then
      _wg_core=""; _wg_why=stale
    fi
  fi
  if [ -n "$_wg_core" ]; then
    HW_WRAPPER_ARGV0="$0" exec "$_wg_core" "$_wg_prog" "$@"
  fi
  _wg_msg="$_wg_prog: hw-core is not built or is older than core/ (the default), so the bash implementation runs — setup/build-core builds it"
  _wg_stamp="${XDG_STATE_HOME:-${HOME:-/nonexistent}/.local/state}/hw/warned"
  _wg_key="wrapper:$_wg_prog:${_wg_why:-missing}"
  if [ "$_wg_from" = env ] || ! { [ -r "$_wg_stamp" ] && grep -qxF -- "$_wg_key" "$_wg_stamp" 2>/dev/null; }; then
    printf '%s\n' "$_wg_msg" >&2
    if [ "$_wg_from" = file ]; then { mkdir -p "${_wg_stamp%/*}" && printf '%s\n' "$_wg_key" >>"$_wg_stamp"; } 2>/dev/null || true; fi
  fi
fi
unset _wg_up _wg_mode _wg_from _wg_file _wg_line _wg_core _wg_why _wg_os _wg_arch _wg_ext _wg_c _wg_msg _wg_stamp _wg_key
