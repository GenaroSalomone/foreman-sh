#!/usr/bin/env bash
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
LIB="${HW_SOURCE:-$ROOT/bin/invoker-common.sh}"
route() {
  HW_EXECUTOR_VENDOR="$1" HW_INVOKER_VENDOR="$2" HW_INVOKER_SESSION="${3-ses}" HW_INVOKER_ENDPOINT="${4--}" \
    bash -c '. "$1"; invoker_route' _ "$LIB"
}
[ "$(route codex codex)" = herdr ] || fail "same-vendor Codex report still selects native transport: $(route codex codex)"
pass "same-vendor Codex reports use herdr even with session and endpoint fields"
[ "$(route opencode opencode ses http://localhost:4567)" = opencode ] || fail "OpenCode lost native reports"
[ "$(route claude claude)" = herdr ] || fail "Claude report changed route"
[ "$(route codex opencode)" = herdr ] || fail "mixed vendor report changed route"
[ "$(route opencode opencode '')" = herdr ] || fail "absent session admitted native report"
pass "native OpenCode, Claude, mixed vendors and missing sessions retain their routing"
