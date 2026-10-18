# invoker-go.sh — the Go implementation of ask-invoker and done-invoker (core/cmd/invokers).
# SOURCED by bin/ask-invoker and bin/done-invoker on the line that resolves INVOKER_BIN_DIR, never
# executed: no shebang, not executable. It sits in a file of its own so that the two scripts do not
# move a single line — the contract pages cite them by line (setup/tests/815).
#
# THE DEFAULT IS BASH. `go` execs the invokers binary — HW_IMPL_<THIS_VERB> or HW_IMPL in the
# environment, else the `<verb>=` line or the default line of the switch file
# (${HW_IMPL_FILE:-~/.config/hw/impl}), the same file `hw` reads. `shadow` is not offered here:
# the invokers write, and a second run is a second report. A binary that is not built, or older
# than core/'s sources, leaves the bash to run; so does this file's absence (a fixture that
# copies the invokers without it). The bash is the rollback and stays until the Go has soaked;
# a fix to one goes to both.
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
      *=*) [ "$(printf '%s' "${_inv_line%%=*}" | tr 'a-z-' 'A-Z_')" != "$_inv_up" ] || _inv_mode="${_inv_line#*=}" ;;
      *) _inv_def="$_inv_line" ;;
    esac
  done < "$_inv_file"
  [ -n "$_inv_mode" ] || _inv_mode="$_inv_def"
fi
if [ "$_inv_mode" = go ]; then
  _inv_os="$(uname -s | tr 'A-Z' 'a-z')"; _inv_arch="$(uname -m)"
  case "$_inv_arch" in x86_64) _inv_arch=amd64 ;; aarch64) _inv_arch=arm64 ;; esac
  _inv_go="${HW_INVOKERS:-$INVOKER_BIN_DIR/../core/dist/$_inv_os-$_inv_arch/invokers}"
  if [ -x "$_inv_go" ] && [ -z "$(find "$INVOKER_BIN_DIR/../core" -path '*/dist' -prune -o \( -name '*.go' -o -name go.mod \) -newer "$_inv_go" -print -quit 2>/dev/null)" ]; then
    exec "$_inv_go" --bin-dir "$INVOKER_BIN_DIR" "$_inv_prog" "$@"
  fi
fi
