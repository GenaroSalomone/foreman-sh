# impl-retired.sh — the retired HW_IMPL switch (go-060 S3, completed in 0.7.0). SOURCED by bin/hw, bin/brain, bin/channel-send,
# bin/ask-invoker, bin/done-invoker and the eight small tools on their first lines, never executed: no shebang, not executable.
#
# Until 0.5.x `HW_IMPL=bash`, `HW_IMPL_<VERB>=bash`, `shadow`, and the same words in the switch file
# (${HW_IMPL_FILE:-~/.config/hw/impl}: a bare line, `default=` or `<verb>=`) chose the bash twin of these tools. 0.6.0 removed the bash
# of hw and its invokers; 0.7.0 removed the bash of the eight small tools. A word is read for ONE thing, to say so, and Go runs.
# NEVER a refusal: a hook calls `hw` every turn, and a dying `hw` would stop every pane. One line per state, keyed in
# ${XDG_STATE_HOME:-~/.local/state}/hw/warned (shared with hw's other warn-once lines), so a profile that still says HW_IMPL=bash
# is told once on the machine, not once per tool or per turn.
#
# The eight small tools are read here too since 0.7.0: HW_IMPL_STOP_HOOK, HW_IMPL_HW_SECRET, HW_IMPL_HW_SANDBOX, HW_IMPL_BROWSER_CLOSE,
# HW_IMPL_BRAIN_ARCHIVE, HW_IMPL_SECRET_UNLOCK, HW_IMPL_OPENCODE_AUTO, HW_IMPL_CODEX_CHANNEL_SESSION_HOOK and their `<tool>=` lines
# name a bash that no longer exists. The way back is `foreman-sh upgrade --to 0.6.3`. HW_BASH_BODY=1, the seam that ran a bash body,
# is gone too, so it does nothing.
#
# impl_retired_warn <tool> — prints nothing and returns 0 when nothing retired is set.
impl_retired_warn() {
  local tool="$1" f line key val v n=0
  # HW_IMPL_FILE names the switch file, it is not a switch
  _impl_retired_say() {  # <value> <where> — one line, once per state
    local stamp="${XDG_STATE_HOME:-${HOME:-/nonexistent}/.local/state}/hw/warned" k="retired:$2:$1" m
    case "$1" in
      bash|shadow) m="bash was removed; running Go — delete this line" ;;
      *) m="«$1» is not an implementation (Go is the only one); running Go — delete this line" ;;
    esac
    if [ -r "$stamp" ] && grep -qxF -- "$k" "$stamp" 2>/dev/null; then return 0; fi  # MUTATION-ANCHOR: 1127-M02
    printf '%s: %s names «%s»: %s\n' "$tool" "$2" "$1" "$m" >&2
    { mkdir -p "${stamp%/*}" && printf '%s\n' "$k" >>"$stamp"; } 2>/dev/null || true
  }
  v="${HW_IMPL:-}"
  case "$v" in ""|go) ;; *) _impl_retired_say "$v" "HW_IMPL" ;; esac
  for key in $(compgen -e HW_IMPL_ 2>/dev/null || true); do
    case "$key" in HW_IMPL_FILE|*[!A-Za-z0-9_]*) continue ;; esac
    val="${!key:-}"
    case "$val" in ""|go) ;; *) _impl_retired_say "$val" "$key" ;; esac
  done
  f="${HW_IMPL_FILE:-${XDG_CONFIG_HOME:-${HOME:-/nonexistent}/.config}/hw/impl}"
  if [ -r "$f" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      n=$((n + 1))
      line="${line%%#*}"; line="${line//[[:space:]]/}"
      case "$line" in
        "") continue ;;
        *=*) key="$(printf '%s' "${line%%=*}" | tr 'a-z-' 'A-Z_')"; val="${line#*=}" ;;
        *) key=default; val="$line" ;;
      esac
      case "$val" in ""|go) ;; *) _impl_retired_say "$val" "$f line $n (${line%%=*})" ;; esac
    done <"$f"
  fi
  unset -f _impl_retired_say
  return 0
}
