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
#      the judgment stays on wall time with the re-run, and says so. Native
#      Windows (OSTYPE msys*/cygwin*) is such a platform for CPU: bash's `time`
#      there does not count a native child's CPU (a 0.4s spin read 92 ms), so
#      fg_time_cmd reports CPU as 0 — unmeasurable — rather than a number too
#      small that would let CPU drift through a saturated gate.
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
#   6. ON A CI HOST EVERY CEILING IS MULTIPLIED (fg_ci_factor, default x3 when
#      CI=true or GITHUB_ACTIONS=true). A shared runner cannot judge wall-clock:
#      measured 2026-09-30, run 36747604152 on windows-latest refused 104 at
#      3276ms against 3000ms, "load-unknown" — the runner, not the subject.
#      The factor relaxes, it does not switch off: a subject 3x over its
#      ceiling is still refused there. HW_TEST_BUDGET_FACTOR overrides it.
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
# Returns the command's own status. On native Windows FG_CPU_MS is 0 (point 4).
fg_time_cmd() {
  local o="$1" e="$2" tf rc r u s; shift 2
  tf="$(mktemp)"
  { TIMEFORMAT='%3R %3U %3S'; time "$@" > "$o" 2> "$e" < /dev/null; } 2> "$tf"
  rc=$?
  read -r r u s < "$tf" || true
  rm -f "$tf"
  FG_WALL_MS="$(fg_ms "${r:-0}")"
  FG_CPU_MS="$(( $(fg_ms "${u:-0}") + $(fg_ms "${s:-0}") ))"
  case "${OSTYPE:-}" in msys*|cygwin*) FG_CPU_MS=0 ;; esac
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

# fg_ci_factor → the multiplier on every ceiling: HW_TEST_BUDGET_FACTOR when it
# is a positive integer, else 3 on a CI host (CI or GITHUB_ACTIONS = true), else 1
fg_ci_factor() {
  case "${HW_TEST_BUDGET_FACTOR:-}" in
    ''|*[!0-9]*|0|00*) ;;
    *) printf '%s' "$HW_TEST_BUDGET_FACTOR"; return 0 ;;
  esac
  if [ "${CI:-}" = true ] || [ "${GITHUB_ACTIONS:-}" = true ]; then printf 3; else printf 1; fi
}

# fg_budget_ms <threshold_s> <own_s> [factor] → the ceiling in ms (own empty
# reads as 0; factor defaults to 1)
fg_budget_ms() {
  local base own2 m
  base=$(( $(fg_ms "$1") + 1000 )); own2=$(( $(fg_ms "${2:-0}") * 2 ))
  if [ "$own2" -gt "$base" ]; then m="$own2"; else m="$base"; fi
  printf '%s' "$(( m * ${3:-1} ))"
}

# fg_drift_warn <wall_ms> <own_ms> <threshold_s> — true when the relative ceiling
# is the one in force (2 x own > threshold + 1s) and the wall is over 1.5 x own.
fg_drift_warn() {
  local base=$(( $(fg_ms "$3") + 1000 ))
  [ "$2" -gt 0 ] && [ $(( $2 * 2 )) -gt "$base" ] && [ $(( $1 * 2 )) -gt $(( $2 * 3 )) ]
}

# THE PER-TEST CEILING (decided 2026-10-01, setup/decisions.md). A subject that
# hangs used to hang the whole suite: the runner timed things and killed
# nothing, so test 481 held `verify-for-push` for 1h50 and a suite-pool slot.
# The ceiling is the subject's committed budget (setup/test-budgets.json) x
# HW_TEST_TIMEOUT_FACTOR (default 5), never under HW_TEST_TIMEOUT_FLOOR seconds
# (default 60: it is also the whole ceiling of a subject with no budget, and
# keeps a 0.2s subject from being cut at 1s on a busy machine). Factor 0 turns
# the ceiling off.
#
# THE CEILING IS SPENT IN LOADED SECONDS, NOT WALL SECONDS. The budgets were
# measured on an idle machine; under a train's load (load1 30-70 on 14 cpus)
# a healthy subject stretches past budget x 5 — 136, budget 15s, was killed
# while working. So each second of wall counts 1 / max(1, load1 / cpus): on an
# oversubscribed machine the subject gets the wall its share of the cpus needs.
# The stretch is capped at HW_TEST_TIMEOUT_LOAD_MAX (default 4), so a hung
# subject is still killed, at most 4x later. HW_TEST_CAP_LOAD=<load1> pins the
# load the ceiling reads (the subjects that prove this use it); 1 per cpu or
# less is no stretch at all, and so is a python with no load average (Git
# Bash): there the ceiling stays the plain wall one.
#
# WHAT IS KILLED IS ONE PROCESS GROUP. The subject runs in a session of its own
# (start_new_session), so killpg reaches it and everything it spawned and
# nothing else — no pkill by pattern, which setup/guards/deny-blind-process-kill
# refuses and which would also take the sibling subjects of a parallel run.
# TERM first, KILL after 2s, then a reap so the CPU the wrapper reports is the
# subject's. On a cut the wrapper writes "<elapsed_s> <cap_s>" to <status-file>
# and exits 124; the runner turns that into the red line.
#
# NATIVE WINDOWS (Git Bash: native python, sys.platform win32) has neither
# killpg nor SIGKILL, and start_new_session makes no group there: the cut is
# `taskkill /T /F`, which ends the subject and the tree it spawned. And there
# every process start costs ~10x (the HW_TEST_SLOW of setup/tests/_common.sh),
# so the ceiling is x10: measured 2026-10-02 (windows.yml run 37011929022),
# subject 01 (budget 2.4s) ran past the 60s floor working, and the cut then
# died on `signal.SIGKILL` instead of killing it.
#
# NO APOSTROPHES in the program below: it lives in a single-quoted variable.
_FG_CAP_PY='
import json, os, signal, subprocess, sys, time
budgets, name, factor, floor, status, loadmax = sys.argv[1:7]
cmd = sys.argv[7:]
def num(v, d):
    try:
        v = float(v)
    except ValueError:
        return d
    return v if v >= 0 else d
factor, floor, loadmax = num(factor, 5.0), num(floor, 60.0), max(1.0, num(loadmax, 4.0))
cpus = float(os.cpu_count() or 1)
def stretch():
    pinned = os.environ.get("HW_TEST_CAP_LOAD", "")
    try:
        load = float(pinned) if pinned else os.getloadavg()[0]
    except (ValueError, OSError, AttributeError):  # AttributeError: no os.getloadavg (Git Bash)
        load = 0.0
    return min(loadmax, max(1.0, load / cpus))
own = None
try:
    own = float(json.load(open(budgets))["files"][name]["seconds"])
except (OSError, ValueError, KeyError, TypeError):
    pass
windows = sys.platform == "win32"
cap = None if factor == 0 else max(floor, (own or 0) * factor) * (10 if windows else 1)
# On native Windows the subject starts SUSPENDED inside a Job Object, so every
# process it ever spawns is in the job whatever its parent chain: taskkill /T
# walks parent pids, and an msys exec chain leaves a grandchild outside that
# tree, alive, holding the pipe and its directory (585-588 hung 2400s on
# windows.yml 37032873682). No WinDLL (the simulation in 609): taskkill only.
job = None
k32 = getattr(__import__("ctypes"), "WinDLL", None) if windows else None
if k32:
    import ctypes
    k32 = ctypes.WinDLL("kernel32", use_last_error=True)
    k32.CreateJobObjectW.restype = ctypes.c_void_p
    k32.AssignProcessToJobObject.argtypes = (ctypes.c_void_p, ctypes.c_void_p)
    k32.TerminateJobObject.argtypes = (ctypes.c_void_p, ctypes.c_uint)
    job = k32.CreateJobObjectW(None, None)
if job:
    p = subprocess.Popen(cmd, creationflags=0x4)  # CREATE_SUSPENDED
    ntdll = ctypes.WinDLL("ntdll")
    ntdll.NtResumeProcess.argtypes = (ctypes.c_void_p,)
    if not k32.AssignProcessToJobObject(job, int(p._handle)):
        job = None
    ntdll.NtResumeProcess(int(p._handle))
else:
    p = subprocess.Popen(cmd, start_new_session=True)
t0 = last = time.time()
spent, rc = 0.0, None
while rc is None:
    try:
        rc = p.wait(timeout=None if cap is None else max(0.05, min(1.0, (cap - spent) * stretch())))
    except subprocess.TimeoutExpired:
        now = time.time()
        spent += (now - last) / stretch()
        last = now
        if spent >= cap:
            break
if rc is None:
    elapsed = time.time() - t0
    if windows:
        if job:
            k32.TerminateJobObject(job, 1)
        subprocess.run(["taskkill", "/T", "/F", "/PID", str(p.pid)],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    else:
        for sig, grace in ((signal.SIGTERM, 2), (signal.SIGKILL, None)):
            try:
                os.killpg(p.pid, sig)
            except ProcessLookupError:
                break
            try:
                p.wait(timeout=grace)
                break
            except subprocess.TimeoutExpired:
                pass
    p.wait()
    with open(status, "w") as f:
        f.write("%.1f %.1f\n" % (elapsed, cap))
    sys.exit(124)
sys.exit(128 - rc if rc < 0 else rc)
'

# fg_capped <budgets.json> <subject-file-name> <status-file> <cmd...> — the
# command under the per-test ceiling above. Returns its status, 124 when cut.
fg_capped() {
  local b="$1" n="$2" s="$3"; shift 3
  python3 -c "$_FG_CAP_PY" "$b" "$n" "${HW_TEST_TIMEOUT_FACTOR:-5}" "${HW_TEST_TIMEOUT_FLOOR:-60}" "$s" \
    "${HW_TEST_TIMEOUT_LOAD_MAX:-4}" "$@"
}
