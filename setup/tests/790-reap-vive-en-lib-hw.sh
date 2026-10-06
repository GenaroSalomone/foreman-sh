#!/usr/bin/env bash
# `hw reap` lives in lib/hw/reap.sh, and bin/hw sources it exactly once.
#
#     bash setup/tests/790-reap-vive-en-lib-hw.sh
#
# bin/hw was one 17,600-line file; it is being split by command into lib/hw/,
# one module per train. This subject pins the first: nothing of reap is defined
# in bin/hw any more (a function copied back would shadow the module's, or the
# module's would shadow it, depending on order — silently), the module is
# sourced once (twice re-runs its assignments), and it resolves through a
# symlink to bin/hw, which is how the installer puts `hw` on the PATH.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HW="$ROOT/bin/hw" MOD="$ROOT/lib/hw/reap.sh"
[ -r "$MOD" ] || fail "790: $MOD does not exist"

# Old = every one of these was defined in bin/hw. New = only in the module.
DEF='^(_reap_[A-Za-z0-9_]+|cmd_reap|_pg_major|_pg_tool|_wt_sheddable|_wt_shed|_wt_build_idle)\(\)'
left="$(rg -n "$DEF" "$HW" || true)"
[ -z "$left" ] || fail "790: bin/hw still defines reap functions — $left"
pass "790: bin/hw defines no _reap_* function, nor cmd_reap and its helpers"

for f in cmd_reap _reap_worktree _reap_db _reap_rm_workdir _reap_log_file _reap_orphan_branches _wt_shed _wt_build_idle; do
  [ "$(rg -c "^$f\\(\\)" "$MOD")" = 1 ] || fail "790: lib/hw/reap.sh does not define $f exactly once"
done
pass "790: lib/hw/reap.sh defines the reap functions once each"

n="$(rg -c '^[[:space:]]*(\.|source)[[:space:]]+"?\$HW_LIB/hw/reap\.sh"?([[:space:]]|$)' "$HW" || true)"
[ "$n" = 1 ] || fail "790: bin/hw sources lib/hw/reap.sh $n times, not once"
pass "790: bin/hw sources lib/hw/reap.sh exactly once"

# hw is symlinked into ~/bin: the module is found from the link's target.
mkdir -p "$TMP/link"; ln -s "$HW" "$TMP/link/hw"
out="$("$TMP/link/hw" reap --help 2>&1)" || fail "790: hw reap --help through a symlink failed — $out"
printf '%s' "$out" | rg -q 'usage: hw reap' || fail "790: hw reap --help through a symlink printed no usage — $out"
pass "790: hw reap resolves lib/hw/reap.sh through a symlink to bin/hw"

# A copy of bin/ with no lib/ beside it: loud without the seam, working with it,
# and the seam never overrides a lib/ that is there.
mkdir -p "$TMP/copy/bin"; cp -R "$ROOT/bin/." "$TMP/copy/bin/"
out="$(env -u HW_LIB_DIR "$TMP/copy/bin/hw" reap --help 2>&1)" && fail "790: hw with no lib/ beside it and no HW_LIB_DIR ran — $out"
printf '%s' "$out" | rg -q 'cannot load .*/hw/reap\.sh' || fail "790: a missing module did not say so — $out"
pass "790: a bin/ copy with no lib/ refuses loudly, naming the module"
out="$(HW_LIB_DIR="$ROOT/lib" "$TMP/copy/bin/hw" reap --help 2>&1)" || fail "790: HW_LIB_DIR did not stand in for a missing lib/ — $out"
printf '%s' "$out" | rg -q 'usage: hw reap' || fail "790: HW_LIB_DIR run printed no usage — $out"
pass "790: HW_LIB_DIR stands in for a lib/ that is not beside bin/"
mkdir -p "$TMP/copy/lib/hw"; printf 'cmd_reap() { echo BESIDE-WINS; }\n' > "$TMP/copy/lib/hw/reap.sh"
out="$(HW_LIB_DIR="$ROOT/lib" "$TMP/copy/bin/hw" reap 2>&1)" || true
[ "$out" = BESIDE-WINS ] || fail "790: HW_LIB_DIR overrode the lib/ beside bin/ — $out"
pass "790: the lib/ beside bin/ wins over HW_LIB_DIR"
