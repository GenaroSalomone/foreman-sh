# invoker-go.sh — the Go implementation of ask-invoker and done-invoker (core/cmd/invokers).
# SOURCED by bin/ask-invoker and bin/done-invoker on the line that resolves INVOKER_BIN_DIR, never
# executed: no shebang, not executable. It sits in a file of its own so that the two scripts do not
# move a single line — the contract pages cite them by line (setup/tests/815).
#
# THE DEFAULT IS GO. `go` execs the invokers binary — HW_IMPL_<THIS_VERB> or HW_IMPL in the
# environment, else the `<verb>=` line or the default line of the switch file
# (${HW_IMPL_FILE:-~/.config/hw/impl}), the same file `hw` reads, else go. `bash` is the one-line
# rollback (HW_IMPL=bash, or `default=bash` in the file). `shadow` is not offered here:
# the invokers write, and a second run is a second report, so it runs bash. A binary that is not
# built, or older than core/'s sources, leaves the bash to run with ONE line saying so per state;
# so does this file's absence (a fixture that copies the invokers without it). The bash stays
# until 0.6.0; a fix to one goes to both.
#
# Callers set, before sourcing: _inv_prog (ask-invoker | done-invoker) and INVOKER_BIN_DIR.
_inv_up="$(printf '%s' "$_inv_prog" | tr 'a-z-' 'A-Z_')"
eval "_inv_mode=\"\${HW_IMPL_$_inv_up:-\${HW_IMPL:-}}\""
_inv_file="${HW_IMPL_FILE:-${XDG_CONFIG_HOME:-${HOME:-/nonexistent}/.config}/hw/impl}"
if [ -z "$_inv_mode" ] && [ -r "$_inv_file" ]; then
  _inv_def=""
  while IFS= read -r _inv_line || [ -n "$_inv_line" ]; do
    _inv_line="${_inv_line%%#*}"; _inv_line="${_inv_line//[[:space:]]/}"
    case "$_inv_line" in
      "") ;;
      *=*) _inv_k="$(printf '%s' "${_inv_line%%=*}" | tr 'a-z-' 'A-Z_')"; [ "$_inv_k" != "$_inv_up" ] || _inv_mode="${_inv_line#*=}"; [ "$_inv_k" != DEFAULT ] || _inv_def="${_inv_line#*=}" ;;  # MUTATION-ANCHOR: 1050-M08
      *) _inv_def="$_inv_line" ;;
    esac
  done < "$_inv_file"
  [ -n "$_inv_mode" ] || _inv_mode="$_inv_def"
fi
if [ "${_inv_mode:-go}" = go ]; then  # MUTATION-ANCHOR: 1050-M02
  _inv_os="$(uname -s | tr 'A-Z' 'a-z')"; _inv_arch="$(uname -m)"
  case "$_inv_arch" in x86_64) _inv_arch=amd64 ;; aarch64) _inv_arch=arm64 ;; esac
  _inv_go="${HW_INVOKERS:-$INVOKER_BIN_DIR/../core/dist/$_inv_os-$_inv_arch/invokers}"
  if [ -x "$_inv_go" ] && [ -z "$(find "$INVOKER_BIN_DIR/../core" -path '*/dist' -prune -o \( -name '*.go' -o -name go.mod \) -newer "$_inv_go" -print -quit 2>/dev/null)" ]; then
    exec "$_inv_go" --bin-dir "$INVOKER_BIN_DIR" "$_inv_prog" "$@"
  fi
  # the bash answers; a hook calls this every turn, so the line is said once per state of the binary (hw's own stamp)
  if [ -x "$_inv_go" ]; then _inv_why="stale:$(stat -c %Y "$_inv_go" 2>/dev/null || stat -f %m "$_inv_go" 2>/dev/null)"; else _inv_why=missing; fi
  _inv_stamp="${XDG_STATE_HOME:-${HOME:-/nonexistent}/.local/state}/hw/warned"
  if ! grep -qxF -- "invokers:$_inv_why" "$_inv_stamp" 2>/dev/null; then
    printf '%s: the Go invokers are %s, so the bash implementation runs — setup/build-core --native builds them (said once)\n' "$_inv_prog" "$([ "$_inv_why" = missing ] && echo 'not built' || echo 'older than the sources in core/')" >&2
    { mkdir -p "${_inv_stamp%/*}" && printf '%s\n' "invokers:$_inv_why" >>"$_inv_stamp"; } 2>/dev/null || true
  fi
  unset _inv_why _inv_stamp
fi
