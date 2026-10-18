# hw-core, train T6: the verbs that were bash-only (train, suite, unstick, ledger, briefs, cockpit-state, secret, -V).
# Sourced by bin/hw right after the T2 switch, before the lane table loads (so -V answers from a broken table). It lives
# here, not in bin/hw, so that adding it moves no line of bin/hw that setup/contracts/hw.md cites (test 815).
# Same switch as the other trains. These verbs write (a merge, a pane restarted, a brief archived): under `shadow` bash
# answers alone. An executor goes to bash for the verbs `_executor_refuse` words (train, unstick, briefs archive/restore).
# Only hw-core's own `-V` (HW_IMPL_VERSION) is decided here; bin/hw prints the bash one when this does not exec.
case "${1:-}" in
  -V|--version|train|suite|unstick|ledger|briefs|cockpit-state|secret)
    _core_v="$1"; case "$1" in -V|--version) _core_v=version ;; esac
    _core_mode="$(_core_impl_for "$_core_v")"
    if [ -n "${HW_TASK:-}${HW_RUN:-}" ]; then
      case "$1" in train|unstick|briefs) _core_mode=bash ;; esac
    fi
    [ "$_core_mode" != shadow ] || _core_mode=bash
    if [ "$_core_mode" = go ] && _core_usable "$_core_v"; then
      export HW_BRAIN_ROOT="$BRAIN" HW_CORE_BIN_DIR="$HW_BIN_DIR" HW_CORE_ROOT="$_brain_root"
      exec "$_core_bin" "$@"
    fi
    ;;
esac
