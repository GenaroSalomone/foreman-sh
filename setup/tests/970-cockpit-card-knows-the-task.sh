#!/usr/bin/env bash
# The cockpit writer says what the task is and how far it has come
#
# bin/cockpit-state publishes, per row, what the card's redesign (task/cockpit-rediseno) draws above hw's
# internals: `title` (the brief's first `# ` line after its frontmatter), `commits` (the branch over the
# base hw recorded at dispatch, and the newest one's time) and `gate` (the suite running for the task: its
# suite-gate slot, else a `setup/test-hw` whose cwd is in its worktree). The checks are in
# _cockpit_card_checks.py, each on real files and real processes:
#
#   · title_after_frontmatter  a YAML comment is not the title; no frontmatter, the first `# `; no brief, null
#   · commits_over_base        2 commits over review_base, the last 37 min ago; 0 and null on a fresh branch;
#                              null without a repository
#   · commits_cache            a commit inside the TTL is counted: the cache is keyed by the HEAD reflog
#   · gate_from_slot           the task's live slot (or hw done's verification of it); not another task's,
#                              not a dead holder's
#   · gate_from_process        a `bash setup/test-hw` in the worktree; not one in another tree
#
# and each rule has a mutant of the writer that the checks must kill, naming the check that does.
# The card itself, and the mod's mutants, are 971's.
#
# Run alone while working on this subject:
#     bash setup/tests/970-cockpit-card-knows-the-task.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

[ -x "$ROOT/bin/cockpit-state" ] || { pass "970: this tree carries no cockpit writer"; exit 0; }
command -v git >/dev/null 2>&1 || { pass "970: git is not installed — the worktrees cannot be built"; exit 0; }
command -v lsof >/dev/null 2>&1 || { pass "970: lsof is not installed — a process's cwd cannot be read"; exit 0; }
CHECKS="$ROOT/setup/tests/_cockpit_card_checks.py"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/t970.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

out="$(python3 -I "$CHECKS" "$ROOT/bin" 2>&1)" || { printf '%s\n' "$out" >&2; fail "970: the card checks failed on the real tree (above)"; }
printf '%s\n' "$out"
pass "970: title, commits and gate are what the brief, the branch and the running suite say"

mutant_dir() {  # <id> — a COPY of the writer and the tools it loads; sets MD
  MD="$TMP/m-$1"
  mkdir -p "$MD"
  cp "$ROOT"/bin/cockpit-state "$ROOT"/bin/runenv "$ROOT"/bin/holdfacts "$ROOT"/bin/hw-actions "$MD/"
}
kill_mutant() {  # <id> <check> <needle>… — the check, run on the mutated copy, must fail with the mutant's own evidence
  local id="$1" check="$2" res rc=0; shift 2
  res="$(python3 -I "$CHECKS" "$MD" "$check" 2>&1)" || rc=$?
  [ "$rc" -ne 0 ] || fail "970: mutant $id SURVIVED — check $check passed on a writer with the rule broken"
  saw_mutant "$id $check" "$res" "$@"
}
mutant_dir 970-M01; mutate_anchor 970-M01 "$MD/cockpit-state" 'i = 0'; kill_mutant 970-M01 title_after_frontmatter "the title read past the frontmatter: 'a YAML comment is not the title'"
mutant_dir 970-M02; mutate_anchor 970-M02 "$MD/cockpit-state" 'count = min(len(stamps), 1)'; kill_mutant 970-M02 commits_over_base "two commits on the branch over its base"
mutant_dir 970-M03; mutate_anchor 970-M03 "$MD/cockpit-state" 'if True:'; kill_mutant 970-M03 gate_from_slot "a slot of another task is not this one's gate"
mutant_dir 970-M04; mutate_anchor 970-M04 "$MD/cockpit-state" 'if hit and 0 <= now - hit[0] < 3600 * 1000:'; kill_mutant 970-M04 commits_cache "a new commit is counted within the TTL"
mutant_dir 970-M05; mutate_anchor 970-M05 "$MD/cockpit-state" 'if True:'; kill_mutant 970-M05 gate_from_process "a run in another tree is not this task's gate"
pass "970: 5 mutants of the writer, each killed by the check that names its rule"
