#!/usr/bin/env bash
# The cockpit writer says how long an executor has worked and whether it still moves
#
# bin/cockpit-state publishes, per row, `dispatched_at`, and for a working row `turn_started_at` and
# `last_progress_at`, plus `rules.no_progress_after_ms` (the one threshold). The checks are in
# _cockpit_times_checks.py, each on real files (run dirs, git repositories, transcripts):
#
#   · times_published         dispatched_at is the receipt's first `at`; turn_started_at is the
#                             previous turn's end (dispatched_at in a first turn); a row that is not
#                             working carries neither a turn start nor a progress time
#   · dispatched_fallbacks    no `at` on the receipt: the dispatch file's mtime, then env's
#   · last_progress           the newest of the last commit (files untouched), the newest file in the
#                             worktree and the claude transcript; `.git`, `.hw` and node_modules
#                             are not progress; with no sign at all it is the dispatch
#   · threshold_is_one_constant  30 min, published once; the test environment can lower it
#   · progress_cache          a worktree walked inside its TTL is not walked again
#   · progress_cache_is_shared_by_writers  the cache is a file beside the state file, so a kick and
#                             the heartbeat share it; `--stdout` creates nothing
#   · walk_cost               what a walk of 3000 files costs (printed)
#
# and each rule has a mutant of the writer that the checks above must kill, naming the check that does.
# The card's text and colour, and the mod's mutants, are 921's.
#
# Run alone while working on this subject:
#     bash setup/tests/920-cockpit-times-in-the-writer.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

[ -x "$ROOT/bin/cockpit-state" ] || { pass "920: this tree carries no cockpit writer"; exit 0; }
command -v git >/dev/null 2>&1 || { pass "920: git is not installed — the worktrees cannot be built"; exit 0; }
CHECKS="$ROOT/setup/tests/_cockpit_times_checks.py"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/t920.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

out="$(python3 -I "$CHECKS" "$ROOT/bin" 2>&1)" || { printf '%s\n' "$out" >&2; fail "920: the times checks failed on the real tree (above)"; }
printf '%s\n' "$out"
pass "920: dispatched_at, turn_started_at and last_progress_at are what the disk says; the threshold is one constant; the walk is cached"

mutant_dir() {  # <id> — a COPY of the writer and the tools it loads; sets MD
  MD="$TMP/m-$1"
  mkdir -p "$MD"
  cp "$ROOT"/bin/cockpit-state "$ROOT"/bin/runenv "$ROOT"/bin/holdfacts "$ROOT"/bin/hw-actions "$MD/"
}
kill_mutant() {  # <id> <check> <needle>… — the check, run on the mutated copy, must fail with the mutant's own evidence
  local id="$1" check="$2" res rc=0; shift 2
  res="$(python3 -I "$CHECKS" "$MD" "$check" 2>&1)" || rc=$?
  [ "$rc" -ne 0 ] || fail "920: mutant $id SURVIVED — check $check passed on a writer with the rule broken"
  saw_mutant "$id $check" "$res" "$@"
}
mutant_dir 920-M01; mutate_anchor 920-M01 "$MD/cockpit-state" 'if False:'; kill_mutant 920-M01 last_progress "a commit on the branch is progress"
mutant_dir 920-M02; mutate_anchor 920-M02 "$MD/cockpit-state" 'return 0'; kill_mutant 920-M02 threshold_is_one_constant "'no_progress_after_ms': 0"
mutant_dir 920-M03; mutate_anchor 920-M03 "$MD/cockpit-state" 'if False:'; kill_mutant 920-M03 progress_cache "cache ignored"
mutant_dir 920-M04; mutate_anchor 920-M04 "$MD/cockpit-state" 'if True:'; kill_mutant 920-M04 last_progress ".git, .hw and node_modules are not progress" "though no file moved"
mutant_dir 920-M05; mutate_anchor 920-M05 "$MD/cockpit-state" 'turn_started = turn_at'; kill_mutant 920-M05 times_published "a first turn began with the run"
mutant_dir 920-M06; mutate_anchor 920-M06 "$MD/cockpit-state" 'if False:'; kill_mutant 920-M06 last_progress "the transcript's last write is progress"
mutant_dir 920-M07; mutate_anchor 920-M07 "$MD/cockpit-state" 'pass'; kill_mutant 920-M07 progress_cache_is_shared_by_writers "second write inside the TTL"
mutant_dir 920-M08; mutate_anchor 920-M08 "$MD/cockpit-state" 'progress = None'; kill_mutant 920-M08 last_progress "wanted about"
pass "920: 8 mutants of the writer, each killed by the check that names its rule"
