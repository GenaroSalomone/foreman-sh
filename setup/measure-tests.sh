#!/usr/bin/env bash
# Measures real wall-clock seconds per setup/tests/*.sh subject, run the same
# way test-hw runs them (own bash process, git env stripped), and writes
# setup/test-budgets.json — the file the fast/slow gate split is derived from.
#
# Usage:
#   bash setup/measure-tests.sh                 # re-measure EVERY subject file
#   bash setup/measure-tests.sh 42-foo.sh 9-bar.sh   # re-measure only these,
#                                                       merged into the existing
#                                                       manifest — this is the
#                                                       ~1s-per-file path a new
#                                                       or edited test takes,
#                                                       not a ~1000s full run.
#
# WHY THIS EXISTS SEPARATELY FROM test-hw. The fast gate must not derive its
# own classification by running the full suite to decide whether to run the
# full suite — the measurement is a value committed once (here) and read many
# times (by test-hw's own completeness+threshold check, every fast run). This
# script is the only place that WRITES setup/test-budgets.json; test-hw only
# reads it.
#
# fast_gate_threshold_seconds is PRESERVED across a partial (named-files) run
# and defaults to 2.0 — the value measured 2026-09-09 to keep the fast gate's
# own sum in the tens of seconds — on a full regeneration that has none yet.
# Change it by editing setup/test-budgets.json directly, or pass
# MEASURE_TESTS_THRESHOLD=<n> to this script.
set -uo pipefail
HERE="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TESTS="$HERE/tests"
OUT="$HERE/test-budgets.json"

declare -a TARGETS
if [ "$#" -gt 0 ]; then
  for name in "$@"; do
    f="$TESTS/$name"
    [ -f "$f" ] || { printf 'measure-tests: %s does not exist\n' "$f" >&2; exit 1; }
    TARGETS+=("$f")
  done
else
  shopt -s nullglob
  TARGETS=("$TESTS"/[0-9][0-9]*-*.sh)
  shopt -u nullglob
fi
[ "${#TARGETS[@]}" -gt 0 ] || { printf 'measure-tests: nothing to measure\n' >&2; exit 1; }

results_file="$(mktemp)"
trap 'rm -f "$results_file"' EXIT
echo "{}" > "$results_file"
# A FILE THAT DOES NOT PARSE HAS NO BUDGET, it has a missing measurement, and
# writing one anyway is worse than writing none. On 2026-09-11 this script
# recorded `exit=0` and `0.05s` for a subject with an unterminated quote --
# bash parses incrementally, so a break near the end still exits 0 -- and that
# number then became the committed budget for a file that never ran. Refusing
# here keeps the manifest a record of runs that happened.
for f in "${TARGETS[@]}"; do
  if ! parse_err="$(bash -n "$f" 2>&1)"; then
    printf 'measure-tests: %s does not parse, so it cannot be measured — no budget was written for it:\n%s\n' \
      "$(basename "$f")" "$parse_err" >&2
    exit 1
  fi
done
for f in "${TARGETS[@]}"; do
  name="$(basename "$f")"
  start=$(date +%s.%N)
  env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE -u GIT_OBJECT_DIRECTORY \
      -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR -u GIT_PREFIX \
      bash "$f" >/dev/null 2>&1
  rc=$?
  end=$(date +%s.%N)
  python3 -c "
import json
d = json.load(open('$results_file'))
d['$name'] = {'seconds': round($end - $start, 2), 'exit': $rc}
json.dump(d, open('$results_file', 'w'))
"
  printf '%-55s %8.2fs  exit=%s\n' "$name" "$(python3 -c "print($end-$start)")" "$rc" >&2
  # A FAILED RUN IS NOT A MEASUREMENT, and the number it produces is the
  # duration of whatever went wrong. It is still written -- the manifest is a
  # record of what happened, and setup/test-hw refuses a manifest carrying one,
  # which is the gate -- but it may not scroll past as an ordinary line.
  [ "$rc" = 0 ] || printf 'measure-tests: %s FAILED while being measured (exit %s) — %ss is the duration of that failure, not a budget. setup/test-hw will refuse this manifest until the subject passes.\n' "$name" "$rc" "$(python3 -c "print(round($end-$start,2))")" >&2
done

python3 - "$OUT" "$results_file" "${MEASURE_TESTS_THRESHOLD:-}" <<'PY'
import json, pathlib, sys, datetime
out_path, results_path, threshold_arg = sys.argv[1:4]
new = json.load(open(results_path))
try:
    existing = json.loads(pathlib.Path(out_path).read_text())
except FileNotFoundError:
    existing = {}
files = existing.get("files", {})
files.update(new)
threshold = existing.get("fast_gate_threshold_seconds", 2.0)
if threshold_arg:
    threshold = float(threshold_arg)
out = {
    "measured_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "fast_gate_threshold_seconds": threshold,
    "files": files,
}
# Owned by hand like the threshold: the cap on the slow subjects the executor's
# verify adds (setup/gate-select touched). A re-measure must not drop it.
if "touched_budget_seconds" in existing:
    out["touched_budget_seconds"] = existing["touched_budget_seconds"]
pathlib.Path(out_path).write_text(json.dumps(out, indent=2, sort_keys=True) + "\n")
print(f"wrote {out_path}: {len(files)} files classified, threshold {threshold}s")
PY
