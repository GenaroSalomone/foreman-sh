# impl-retired.sh — the retired HW_IMPL switch (go-060 S3). SOURCED by bin/hw, bin/brain, bin/channel-send, bin/ask-invoker and
# bin/done-invoker on their first lines, never executed: no shebang, not executable.
#
# Until 0.5.x `HW_IMPL=bash`, `HW_IMPL_<VERB>=bash`, `shadow`, and the same words in the switch file
# (${HW_IMPL_FILE:-~/.config/hw/impl}: a bare line, `default=` or `<verb>=`) chose the bash twin of these five tools. 0.6.0 has no bash
# for them: the word is read for ONE thing, to say so, and Go runs. NEVER a refusal: a hook calls `hw` every turn, and a dying `hw`
# would stop every pane. One line per state, keyed in ${XDG_STATE_HOME:-~/.local/state}/hw/warned (shared with hw's other warn-once
# lines), so a profile that still says HW_IMPL=bash is told once on the machine, not once per tool or per turn.
#
# NOT the eight small tools (hw-stop-hook.sh, hw-secret, hw-sandbox and the five wrappers): HW_IMPL_<TOOL> and a `<tool>=` line keep
# their meaning for them until 0.7.0, so those names are not read here. HW_BASH_BODY=1, the seam that ran a bash body, is not
# read here either: it is gone (S4 for hw, S5 for the other four), so it does nothing.
#
# impl_retired_warn <tool> — prints nothing and returns 0 when nothing retired is set.
impl_retired_warn() {
  local tool="$1" f line key val v n=0
  # HW_IMPL_FILE names the switch file, it is not a switch; the eight keep theirs
  _impl_retired_tool() {  # <KEY in capitals> — succeeds for the eight small tools
    case "$1" in STOP_HOOK|HW_SECRET|HW_SANDBOX|BROWSER_CLOSE|BRAIN_ARCHIVE|SECRET_UNLOCK|OPENCODE_AUTO|CODEX_CHANNEL_SESSION_HOOK) return 0 ;; esac
    return 1
  }
  _impl_retired_say() {  # <value> <where> — one line, once per state
    local stamp="${XDG_STATE_HOME:-${HOME:-/nonexistent}/.local/state}/hw/warned" k="retired:$2:$1" m
    case "$1" in
      bash|shadow) m="bash was removed in 0.6.0; running Go — delete this line" ;;
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
    _impl_retired_tool "${key#HW_IMPL_}" && continue
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
        *=*) key="$(printf '%s' "${line%%=*}" | tr 'a-z-' 'A-Z_')"; val="${line#*=}"
             _impl_retired_tool "$key" && continue ;;
        *) key=default; val="$line" ;;
      esac
      case "$val" in ""|go) ;; *) _impl_retired_say "$val" "$f line $n (${line%%=*})" ;; esac
    done <"$f"
  fi
  unset -f _impl_retired_tool _impl_retired_say
  return 0
}
