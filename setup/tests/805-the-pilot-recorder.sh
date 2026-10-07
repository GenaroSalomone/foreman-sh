#!/usr/bin/env bash
# THE PILOT RECORDER: time from envelope to action, and the reports nobody acted on
#
# The design's pilot metric. bin/cockpit-state appends one `{at,pane,task,kind}` line per NEW report,
# ask or challenge to <work>/.cockpit/cockpit-events.jsonl (deduplicated, 0600, rotated to .1 at
# 1 MiB); the mod appends `{at,pane,verb}` to cockpit-actions.jsonl beside it (held by
# cockpit/hooks/actions.test.ts, run by 801/802); `cockpit-state --pilot` prints the median and p90
# time from an envelope to the first action on its pane, and the envelopes with none after 30 min.
# Checks are in _cockpit_pilot_checks.py; each rule has a text mutant that must turn one red.
#
#     bash setup/tests/805-the-pilot-recorder.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

[ -x "$ROOT/bin/cockpit-state" ] || { pass "805: this tree carries no cockpit writer"; exit 0; }
CHECKS="$ROOT/setup/tests/_cockpit_pilot_checks.py"

out="$(python3 -I "$CHECKS" "$ROOT/bin" 2>&1)" || { printf '%s\n' "$out" >&2; fail "805: the pilot checks failed on the real tree (above)"; }
printf '%s\n' "$out"
pass "805: the writer records envelopes, rotates, and --pilot reports median, p90 and the missed"

mutant() {  # <id> <check> <from> <to>
  local d="$TMP/m-$1"
  mkdir -p "$d"
  cp "$ROOT"/bin/cockpit-state "$ROOT"/bin/runenv "$ROOT"/bin/holdfacts "$ROOT"/bin/hw-actions "$d/"
  python3 -I - "$d/cockpit-state" "$3" "$4" <<'PY' || fail "805: mutant $1 targets text that is not in bin/cockpit-state"
import sys
p, a, b = sys.argv[1:4]
s = open(p).read()
if a not in s: sys.exit(1)
open(p, "w").write(s.replace(a, b, 1))
PY
  if python3 -I "$CHECKS" "$d" "$2" >"$TMP/m.out" 2>&1; then fail "805: mutant $1 SURVIVED — check $2 passed with the rule broken"; fi
}
mutant M01 records 'not in seen]' 'not in set()]'
mutant M02 rotates 'if os.path.getsize(f) >= EVENTS_CAP' 'if False'
mutant M03 pilot 'later = [t for t in acts.get(e.get("pane"), []) if t >= e["at"]]' 'later = list(acts.get(e.get("pane"), []))'
mutant M04 pilot 'return times[max(0, -(-p * len(times) // 100) - 1)]' 'return times[0]'
mutant M05 pilot 'elif now - e["at"] > MISSED_MS:' 'elif True:'
mutant M06 records 'record_events(json.loads(body), path)' 'pass'
pass "805: 6 mutants of bin/cockpit-state, each killed"
