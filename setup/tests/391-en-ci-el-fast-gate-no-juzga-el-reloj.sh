#!/usr/bin/env bash
# ON A CI HOST THE FAST GATE RELAXES ITS WALL-CLOCK BUDGETS, AND SAYS SO.
#
# Measured 2026-09-30: the first public CI run (36747604152, windows-latest)
# refused 104 at 3276ms against a 3000ms budget, "load-unknown": a shared
# runner, not the subject. Decided the same day (brief ci-publica-verde): in CI
# — CI=true or GITHUB_ACTIONS=true — every fast-gate ceiling is multiplied
# (setup/fast-gate-budget.sh, point 6; x3 by default, HW_TEST_BUDGET_FACTOR
# overrides). It relaxes, it does not switch off, and the runner prints it.
#
# And the leniency does not leak into a subject: _common.sh unsets CI and
# GITHUB_ACTIONS, so a nested runner (217, this file) judges its fixtures on
# the budgets it wrote, on a laptop and on a runner alike.
#
#     bash setup/tests/391-en-ci-el-fast-gate-no-juzga-el-reloj.sh
#
# SUBJECT_SETUP runs every claim against another setup/ (test-hw, its budget
# library and _common.sh), for the old/new evidence.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SUB="${SUBJECT_SETUP:-$ROOT/setup}"

# ── E: the subject's environment carries no CI leniency ─────────────────────
got="$(env CI=true GITHUB_ACTIONS=true bash -c '. "$1"; printf "%s|%s" "${CI-unset}" "${GITHUB_ACTIONS-unset}"' _ "$SUB/tests/_common.sh")"
[ "$got" = "unset|unset" ] || fail "E01: CI/GITHUB_ACTIONS survive _common.sh ($got): a nested runner in a subject would judge its fixtures leniently on a CI host"
pass "E01: under _common.sh a subject sees neither CI nor GITHUB_ACTIONS"

# ── U: the factor ───────────────────────────────────────────────────────────
factor() { env -u CI -u GITHUB_ACTIONS -u HW_TEST_BUDGET_FACTOR "$@" bash -c '. "$1"; fg_ci_factor' _ "$SUB/fast-gate-budget.sh" 2>&1; }
eq() { [ "$2" = "$3" ] && pass "$1" || fail "$1: expected '$3', got '$2'"; }
eq "U01: no CI: factor 1" "$(factor)" 1
eq "U02: CI=true: factor 3" "$(factor CI=true)" 3
eq "U03: GITHUB_ACTIONS=true: factor 3" "$(factor GITHUB_ACTIONS=true)" 3
eq "U04: CI=false is not a CI host" "$(factor CI=false)" 1
eq "U05: HW_TEST_BUDGET_FACTOR overrides, on a CI host too" "$(factor CI=true HW_TEST_BUDGET_FACTOR=1)" 1
eq "U06: an unusable HW_TEST_BUDGET_FACTOR is ignored" "$(factor HW_TEST_BUDGET_FACTOR=0) $(factor HW_TEST_BUDGET_FACTOR=x2)" "1 1"
eq "U07: the factor multiplies the ceiling, relative or absolute" \
  "$(bash -c '. "$1"; fg_budget_ms 2 0.4 3; printf " "; fg_budget_ms 2 1.97 3; printf " "; fg_budget_ms 2 0.4' _ "$SUB/fast-gate-budget.sh" 2>&1)" "9000 11820 3000"

# ── R: the real runner, on a fixture subject that waits past its budget ─────
mk_gate_repo() {
  local d; d="$(mktemp -d "$TMP/gaterepo-XXXXXX")/root"
  mkdir -p "$d/setup/tests"
  cp "$SUB/test-hw" "$SUB/test-hw-snapshot.py" "$SUB/mutation-coverage" "$SUB/fast-gate-budget.sh" "$d/setup/"
  cp "$SUB/tests/_common.sh" "$d/setup/tests/_common.sh"
  # threshold 0.3s → budget 1300 ms; the subject waits 1.6s
  printf '#!/usr/bin/env bash\nsleep 1.6\necho "ok - x"\n' > "$d/setup/tests/01-x.sh"; chmod +x "$d/setup/tests/01-x.sh"
  printf '{"fast_gate_threshold_seconds": 0.3, "files": {"01-x.sh": {"seconds": 0.1, "exit": 0}}}\n' > "$d/setup/test-budgets.json"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git init -q && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git add -A \
      && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git -c user.email=t@t -c user.name=t commit -q -m f )
  printf '%s' "$d"
}
gate() { # <dir> [VAR=value...] → "<rc>|<output>"; load pinned unsaturated
  local d="$1" rc=0 out; shift
  out="$(cd "$d" && env HW_TEST_LOAD1=0.1 HW_TEST_NCPU=8 HW_TEST_GATE=fast "$@" bash ./setup/test-hw 2>&1)" || rc=$?
  printf '%s|%s' "$rc" "$out"
}
d="$(mk_gate_repo)"

res="$(gate "$d")"
case "$res" in
  [!0]*"took"*"over the 1300ms budget"*) pass "R01: control — off CI the subject is refused over its 1300ms budget" ;;
  *) fail "R01: the control did not refuse a subject past its budget, so R02-R03 would prove nothing: $res" ;;
esac
res="$(gate "$d" CI=true)"
case "$res" in
  0\|*"every fast-gate time budget is x3 here (CI host)"*) pass "R02: CI=true: the same subject passes under x3, and the runner says so" ;;
  *) fail "R02: a CI host did not relax the wall budget out loud: $res" ;;
esac
res="$(gate "$d" GITHUB_ACTIONS=true HW_TEST_BUDGET_FACTOR=1)"
case "$res" in
  [!0]*"over the 1300ms budget"*) pass "R03: HW_TEST_BUDGET_FACTOR=1 on a CI host restores the budget: it relaxes, it does not switch the check off" ;;
  *) fail "R03: the factor override did not restore the budget: $res" ;;
esac
