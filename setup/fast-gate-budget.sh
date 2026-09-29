#!/usr/bin/env bash
# The fast gate's per-subject budget, as functions so the runner and its test
# read one definition. Sourced by setup/test-hw; not run on its own.
#
# WHAT THE BUDGET MEASURES. A subject is refused when it overruns
# `fast_gate_threshold_seconds` + 1s of grace. Wall-clock alone measured the
# MACHINE: on 2026-09-29 test 02 ran 2.7-3.7s at load 7 against a manifest of
# 1.51s (same bytes on main), and five executors in a row could not finish the
# gate at load 20-100. So a reading is judged like this:
#
#   1. in milliseconds, not `date +%s` seconds;
#   2. a subject over budget is RE-RUN up to twice and the MINIMUM is judged —
#      genuine drift is slow in every run, a load spike is not;
#   3. only if the minimum is still over AND the machine is saturated
#      (load1 > cpus/2: measured 2026-09-29, five subjects ran 2x slow at load 7 on 14 cpus, whose hardware threads are not full cores) is the budget applied to the
#      subject's CPU time (user+sys) instead of its wall time, at TWICE the
#      wall budget — measured the same day: CPU time inflates too under load
#      (95: 3.45s vs 1.97s at load 7; 4.16s at load 26 on 14 cpus) — and the gate
#      SAYS so, per subject, with the load it measured. What this stops catching, only under
#      saturation, is drift made of waiting (sleeps: high wall, low CPU); the
#      full suite in verify-for-push still runs the timing subjects.
#   4. where CPU time or the load average cannot be read (a platform whose
#      `time` reports zero, or whose python has no os.getloadavg — Git Bash),
#      the judgment stays on wall time with the re-run, and says so.
#
#   5. THE CEILING IS max(threshold + 1s, 2 x THE SUBJECT'S OWN NUMBER in
#      test-budgets.json). Measured 2026-09-29: subject 95 (single-threaded, its
#      file untouched since 2026-09-07, manifest 1.97s) ran 3.4-4.1s with CPU
#      close to wall at load 7 on 14 physical cores — the machine ran ~1.9x slow
#      without the load explaining it (NOT investigated; UNESTABLISHED cause). A
#      subject near the threshold has no absolute margin for that, so its margin
#      is relative: real drift (> 2x its own baseline) is still refused. Between
#      1.5x and 2x of its own number the gate WARNS, per subject, without failing.
#
# HW_TEST_LOAD1 / HW_TEST_NCPU replace the measured load (a simulated one, for
# setup/tests/217-*.sh).

# fg_ms <S.mmm> → integer milliseconds ('' and junk read as 0)
fg_ms() {
  local x="${1//,/.}" i f
  x="${x%%[!0-9.]*}"
  case "$x" in
    *.*) i="${x%%.*}"; f="${x#*.}000"; f="${f:0:3}"; printf '%s' "$(( 10#${i:-0} * 1000 + 10#$f ))" ;;
    *)   printf '%s' "$(( 10#${x:-0} * 1000 ))" ;;
  esac
}

# fg_load_ncpu → "<load1 x100> <cpus>", or nothing when this platform cannot say
fg_load_ncpu() {
  if [ -n "${HW_TEST_LOAD1:-}" ] && [ -n "${HW_TEST_NCPU:-}" ]; then
    printf '%s %s' "$(( $(fg_ms "$HW_TEST_LOAD1") / 10 ))" "$HW_TEST_NCPU"
    return 0
  fi
  python3 -c 'import os;print(int(os.getloadavg()[0]*100), os.cpu_count() or 0)' 2>/dev/null || true
}

# fg_time_cmd <stdout-file> <stderr-file> <cmd...> — runs it with stdin from
# /dev/null and sets FG_WALL_MS / FG_CPU_MS (CPU = user+sys of the whole tree).
# Returns the command's own status.
fg_time_cmd() {
  local o="$1" e="$2" tf rc r u s; shift 2
  tf="$(mktemp)"
  { TIMEFORMAT='%3R %3U %3S'; time "$@" > "$o" 2> "$e" < /dev/null; } 2> "$tf"
  rc=$?
  read -r r u s < "$tf" || true
  rm -f "$tf"
  FG_WALL_MS="$(fg_ms "${r:-0}")"
  FG_CPU_MS="$(( $(fg_ms "${u:-0}") + $(fg_ms "${s:-0}") ))"
  return "$rc"
}

# fg_decide <wall_ms> <cpu_ms> <budget_ms> <load1x100> <ncpu>
#   → "<pass|refuse> <wall|cpu> <reason>"
fg_decide() {
  local wall="$1" cpu="$2" budget="$3" load="${4:-}" ncpu="${5:-}"
  if [ "$wall" -le "$budget" ]; then echo "pass wall within-budget"; return 0; fi
  if [ -z "$load" ] || [ -z "$ncpu" ] || [ "$ncpu" -le 0 ]; then echo "refuse wall load-unknown"; return 0; fi
  if [ "$cpu" -le 0 ]; then echo "refuse wall cpu-unmeasurable"; return 0; fi
  if [ $(( load * 2 )) -gt $(( 100 * ncpu )) ]; then
    if [ "$cpu" -le $(( 2 * budget )) ]; then echo "pass cpu saturated"; else echo "refuse cpu saturated"; fi
    return 0
  fi
  echo "refuse wall unsaturated"
}

# fg_budget_ms <threshold_s> <own_s> → the ceiling in ms (own empty reads as 0)
fg_budget_ms() {
  local base own2
  base=$(( $(fg_ms "$1") + 1000 )); own2=$(( $(fg_ms "${2:-0}") * 2 ))
  if [ "$own2" -gt "$base" ]; then printf '%s' "$own2"; else printf '%s' "$base"; fi
}

# fg_drift_warn <wall_ms> <own_ms> <threshold_s> — true when the relative ceiling
# is the one in force (2 x own > threshold + 1s) and the wall is over 1.5 x own.
fg_drift_warn() {
  local base=$(( $(fg_ms "$3") + 1000 ))
  [ "$2" -gt 0 ] && [ $(( $2 * 2 )) -gt "$base" ] && [ $(( $1 * 2 )) -gt $(( $2 * 3 )) ]
}
