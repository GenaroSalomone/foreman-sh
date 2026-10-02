#!/usr/bin/env bash
# THE RUNNER'S HEADLINE MUST NOT SILENTLY SPAN COMMITTED AND UNCOMMITTED WORK.
#
# THE INSTRUMENT HAD THE DISEASE IT WAS MEASURING, and that is why this subject
# exists. On 2026-09-02 this suite closed four instances of one failure — a green
# that only held for whoever measured it:
#
#   · an uncommitted `${AGENT:-}` fix that only one working tree had;
#   · a leaked pane environment (HW_CHAINING_LEASE_SECONDS) that made the same
#     tree answer 1140 ok to a brainer and abort at 406 to an executor;
#   · fifteen mutation arms `setup/mutation-coverage` could not see, so they
#     passed because nothing looked;
#   · an untracked file a tracked subject required.
#
# All four were fixed. The result was then reported as a single integer produced
# by `setup/test-hw`, which globs `[0-9][0-9]-*.sh` and never asks git anything.
# Measured by the second-vendor reviewer: `47-unattended-gate-stall.sh` and
# `48-plain-opencode-is-not-plain.sh` are UNTRACKED and absent from e8f7d23, and
# the 1187 everyone quoted — the executor, the brainer and both reviewers —
# silently included their 20 ok.
#
# WHAT THIS SUBJECT PINS, and the order matters because the second half is what
# makes the first half safe:
#   1. the headline counts TRACKED subjects only, because that is the only number
#      a reader can re-derive from the SHA;
#   2. untracked subjects STILL RUN and are counted apart and NAMED — refusing
#      them would push work-in-progress to a filename outside the glob, which is
#      the silent-skip failure the glob contract exists to prevent;
#   3. the untracked line appears even when the count is ZERO, because a
#      qualification that disappears when it is not needed is exactly how the
#      unqualified number comes back.
#
# HERMETIC, AND IT HAS TO BE: every case builds its own throwaway git repository
# with its own tests/ directory and runs the REAL runner against it. It never
# reads this repository's tracking state, so it cannot go green or red because of
# what happens to be uncommitted here today.
#
# Run it alone while working on this subject:
#
#     bash setup/tests/55-the-runner-number-says-what-it-measured.sh
#
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

CLAIMS=0
claim() { CLAIMS=$((CLAIMS + 1)); pass "$1"; }

RUNNER="$ROOT/setup/test-hw"
[ -x "$RUNNER" ] || fail "C00 setup/test-hw is missing or not executable"

# ── the fixture: a throwaway repo whose tests/ we control completely ────────
#
# `_common.sh` is copied because every subject sources it, and it resolves ROOT
# two levels up from its own location — so the fixture mirrors that shape. The
# guards are stubbed to emit nothing: this subject is about attribution, and four
# real node guards would add their own ok lines to every expected number.
make_repo() { # <name> → sets REPO
  local name="$1"
  local repo="$TMP/$name"
  mkdir -p "$repo/setup/tests" "$repo/setup/guards" "$repo/bin"
  cp "$ROOT/setup/test-hw" "$ROOT/setup/test-hw-snapshot.py" "$ROOT/setup/mutation-coverage" "$ROOT/setup/fast-gate-budget.sh" "$repo/setup/"
  cp "$ROOT/setup/tests/_common.sh" "$repo/setup/tests/"
  chmod +x "$repo/setup/test-hw" "$repo/setup/mutation-coverage"
  for g in test-deny-repo-writes.mjs test-deny-repo-writes-filesystem.mjs \
           test-opencode-hw-blocked-reason.mjs test-herdr-opencode-background-state.mjs; do
    printf '%s\n' '// stub: emits no ok lines, so the arithmetic below is the subject only' \
      > "$repo/setup/guards/$g"
  done
  # The real runner also executes this mutation suite. Keep the fixture's guard
  # surface complete while preserving its zero-line arithmetic contract.
  printf '%s\n' '#!/usr/bin/env bash' 'exit 0' > "$repo/setup/guards/mutate-deny-repo-writes.sh"
  chmod +x "$repo/setup/guards/mutate-deny-repo-writes.sh"
  # A git identity of its own: this must not depend on the caller's config.
  ( cd "$repo" && git init -q . \
      && git config user.email t@example.invalid && git config user.name t \
      && git config commit.gpgsign false ) >/dev/null 2>&1 \
    || fail "fixture: could not init the throwaway repo for $name"
  REPO="$repo"
}

# subject <repo> <basename> <n>  — a subject file printing exactly n ok lines
subject() {
  local repo="$1" base="$2" n="$3" i=1
  {
    printf '%s\n' '#!/usr/bin/env bash'
    printf '. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"\n'
    while [ "$i" -le "$n" ]; do
      printf 'pass "%s claim %s"\n' "$base" "$i"
      i=$((i + 1))
    done
  } > "$repo/setup/tests/$base"
  chmod +x "$repo/setup/tests/$base"
}

commit_all() { ( cd "$1" && git add -A . && git commit -q -m fixture ) >/dev/null 2>&1; }
run_runner() { ( cd "$1/setup" && ./test-hw 2>&1 ); }

# ── C01/C02/C03. tracked committed, untracked in flight ────────────────────
make_repo mixed
subject "$REPO" 10-committed.sh 3
subject "$REPO" 11-also-committed.sh 2
commit_all "$REPO"
# …and now two that nobody committed, exactly the 47/48 situation.
subject "$REPO" 47-in-flight.sh 6
subject "$REPO" 48-also-in-flight.sh 14
MIXED_OUT="$(run_runner "$REPO" || true)"

case "$MIXED_OUT" in
  *'5 tests passed  (TRACKED only'*)
    claim "C01 the headline counts only the subjects the commit contains (5, not 25)" ;;
  *) fail "C01: the headline is not the tracked count: $(printf '%s' "$MIXED_OUT" | tail -6 | tr '\n' ' ')" ;;
esac
case "$MIXED_OUT" in
  *'+ 20 ok from UNTRACKED subjects'*)
    claim "C02 the untracked ok lines are counted apart, and stated" ;;
  *) fail "C02: the untracked total is not reported: $(printf '%s' "$MIXED_OUT" | tail -6 | tr '\n' ' ')" ;;
esac
# NAMED, not just totalled. A count nobody can act on sends the reader hunting.
for n in 47-in-flight.sh 48-also-in-flight.sh; do
  case "$MIXED_OUT" in
    *"$n"*) : ;;
    *) fail "C03: the untracked subject $n was counted but not named: $(printf '%s' "$MIXED_OUT" | tail -6 | tr '\n' ' ')" ;;
  esac
done
claim "C03 every untracked subject is named, so the reader can act on the number"

# AND THEY STILL RAN. The whole point of counting them apart rather than
# refusing them: their assertions were executed and are visible.
case "$MIXED_OUT" in
  *'ok - 47-in-flight.sh claim 1'*) claim "C04 untracked subjects are still RUN, not skipped" ;;
  *) fail "C04: an untracked subject did not run at all: $(printf '%s' "$MIXED_OUT" | tail -8 | tr '\n' ' ')" ;;
esac

# ── C05. the qualification is stated even when there is nothing to qualify ──
make_repo clean
subject "$REPO" 10-committed.sh 4
commit_all "$REPO"
CLEAN_OUT="$(run_runner "$REPO" || true)"
case "$CLEAN_OUT" in
  *'4 tests passed  (TRACKED only'*'no untracked subjects ran, so this is the whole suite'*)
    claim "C05 with nothing untracked it says so, so the qualification cannot quietly vanish" ;;
  *) fail "C05: a fully tracked run did not state that it was whole: $(printf '%s' "$CLEAN_OUT" | tail -5 | tr '\n' ' ')" ;;
esac

# ── C06. tracking that cannot be established is its own answer ─────────────
#
# An exported tarball or a copied directory is not a work tree. Claiming
# `tracked` there is the unverified-absence mistake this whole day was about, so
# it is a third bucket that says so on stderr.
make_repo nogit
subject "$REPO" 10-somesubject.sh 3
rm -rf "$REPO/.git"
NOGIT_OUT="$(run_runner "$REPO" || true)"
case "$NOGIT_OUT" in
  *'NO TRACKED NUMBER'*'no usable git work tree'*)
    claim "C06 outside a git work tree the split is refused rather than guessed" ;;
  *) fail "C06: a non-repo run claimed a tracked/untracked split anyway: $(printf '%s' "$NOGIT_OUT" | tail -5 | tr '\n' ' ')" ;;
esac

# ── C07/C08/C09. WHAT SURVIVES A PIPE MUST NOT OVERCLAIM ───────────────────
#
# THE CLAIM AND ITS QUALIFICATION MUST TRAVEL ON ONE STREAM. Found by the opus
# reviewer in 0d2f129 — the commit whose entire thesis is that a number must say
# what it measured. Outside a work tree, stdout carried `0 tests passed (TRACKED
# only …)` and `no untracked subjects ran, so this is the whole suite`, while the
# correction `neither number above is complete` went to STDERR. So
# `./test-hw 2>/dev/null | tail` showed a confident, complete-sounding number,
# and it was not merely unqualified but FALSE: subjects had run and passed, the
# count said 0, and completeness was asserted while nothing had been classified.
#
# It is the SAME defect as 04efe7c, this session's first commit: a caller pipes,
# the caveat goes to the dropped stream, and what survives reads as
# authoritative. Twice in one day, the second time inside the fix for the first.
#
# So these three read ONLY STDOUT — that is the whole point of the arm, and
# `2>/dev/null` here is the reader being modelled, not a convenience.
stdout_only() { ( cd "$1/setup" && ./test-hw 2>/dev/null ); }

make_repo nogit-pipe
subject "$REPO" 10-somesubject.sh 3
rm -rf "$REPO/.git"
PIPED="$(stdout_only "$REPO" || true)"

# The number itself must be gone. Not qualified — GONE. A reader who sees a
# number quotes the number, not the sentence beneath it.
case "$PIPED" in
  *'tests passed'*) fail "C07: a quotable 'tests passed' number survived on stdout with tracking unestablished: $(printf '%s' "$PIPED" | tail -4 | tr '\n' ' ')" ;;
  *) claim "C07 with tracking unestablished, stdout carries NO 'tests passed' number to quote" ;;
esac
case "$PIPED" in
  *'whole suite'*) fail "C08: stdout still claimed the run was the whole suite while nothing was classified" ;;
  *) claim "C08 stdout does not claim completeness when nothing could be classified" ;;
esac
# And the qualification is ON STDOUT, not merely absent from it.
case "$PIPED" in
  *'NO TRACKED NUMBER'*'quotable as a release number'*)
    claim "C09 the qualification rides on the SAME stream as the claim, so a pipe cannot separate them" ;;
  *) fail "C09: the stdout-only reader got no qualification at all: $(printf '%s' "$PIPED" | tail -4 | tr '\n' ' ')" ;;
esac

# ── C10. an aborted run does not read as a finished green one on stdout ────
#
# Beyond the reported finding. On an abort the diagnosis is on stderr and no
# summary prints, so there is no number to quote — verified by the reviewer. What
# a stdout-only reader still saw was a tail of `ok -` lines and nothing marking
# the end, which reads like a completed run. Each of those lines is true, so this
# is not the same defect; it is one line of insurance against the same misreading.
make_repo aborted
subject "$REPO" 10-fine.sh 2
{ printf '%s\n' '#!/usr/bin/env bash' \
                 '. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"' \
                 'fail "deliberate red"'; } > "$REPO/setup/tests/11-red.sh"
chmod +x "$REPO/setup/tests/11-red.sh"
commit_all "$REPO"
ABORT_OUT="$(stdout_only "$REPO" || true)"
case "$ABORT_OUT" in
  *'tests passed'*) fail "C10: an aborted run still printed a number on stdout: $(printf '%s' "$ABORT_OUT" | tail -4 | tr '\n' ' ')" ;;
esac
case "$ABORT_OUT" in
  *'RUN ABORTED — no number for this run'*)
    claim "C10 an aborted run says so on stdout, so a tail of ok lines cannot read as a finished green run" ;;
  *) fail "C10: stdout ended on ok lines with nothing marking the abort: $(printf '%s' "$ABORT_OUT" | tail -4 | tr '\n' ' ')" ;;
esac

# ── MUTANTS ────────────────────────────────────────────────────────────────
#
# Each patches a COPY of the runner inside its own fixture repo, never the real
# one, and is killed by text ONLY THE MUTANT PRODUCES.
# Each mutant edits the line or block a `# MUTATION-ANCHOR: 55-Mnn` marker in
# setup/test-hw declares, not the prose it mutates; see mutate_anchor in _common.sh.
mutant_runner() { # <name> → sets REPO to a fresh fixture whose runner the next mutate_anchor edits
  make_repo "mutant-$1"
}

# BACKSLASHES COME FROM A VARIABLE, NOT FROM ESCAPING. The replacements carry
# the runner's `printf '\n…\n'` text, and this file is written inside a QUOTED
# heredoc where every `\` is literal. `BS` plus single-quoted bash strings makes
# what is written what bash sees.
BS='\'

# M01 — THE PRE-FIX RUNNER, restored exactly: one integer over everything, with
# no idea what is in the commit. The mutant prints its own headline text, so a
# run that never reached the summary is VACUOUS rather than dead.
mutant_runner M01; mutate_anchor 55-M01 "$REPO/setup/test-hw" \
  "printf '${BS}nM01-UNSPLIT-HEADLINE %s tests passed${BS}n' "'"$TOTAL_OK"'
subject "$REPO" 10-committed.sh 3
subject "$REPO" 11-also-committed.sh 2
commit_all "$REPO"
subject "$REPO" 47-in-flight.sh 6
subject "$REPO" 48-also-in-flight.sh 14
out="$(run_runner "$REPO" || true)"
case "$out" in
  *'M01-UNSPLIT-HEADLINE 25 tests passed'*) : ;;
  *) fail "M01 VACUOUS: the mutant did not produce the old single-integer headline over all 25 ok, so it may never have reached the summary — tail: $(printf '%s' "$out" | tail -4 | tr '\n' ' ')" ;;
esac
saw_mutant "M01 restores one integer spanning tracked and untracked, the number everyone quoted" "$out" \
  'M01-UNSPLIT-HEADLINE 25 tests passed'

# M02 — the split kept, the untracked line silenced. The plausible half-fix: a
# correct headline with the qualification dropped, which is a tracked number
# that reads as the whole suite.
mutant_runner M02; mutate_anchor 55-M02 "$REPO/setup/test-hw" \
  "printf '  M02-UNTRACKED-LINE-SILENCED${BS}n'"
subject "$REPO" 10-committed.sh 3
commit_all "$REPO"
subject "$REPO" 47-in-flight.sh 6
out="$(run_runner "$REPO" || true)"
case "$out" in
  *'47-in-flight.sh  (6 ok)'*) fail "M02 SURVIVED: the untracked subjects were still named" ;;
esac
saw_mutant "M02 silences the untracked line, so a tracked count reads as the whole suite" "$out" \
  'M02-UNTRACKED-LINE-SILENCED'

# M03 — `unknown` folded into `tracked`: the unverified-absence mistake in its
# purest form, a number claiming the commit contains work nothing checked.
mutant_runner M03; mutate_anchor 55-M03 "$REPO/setup/test-hw" \
  'if [ "$GIT_USABLE" != 1 ]; then printf '"'"'M03-UNKNOWN-IS-TRACKED'"'"' >&2; printf '"'"'tracked'"'"'; return 0; fi'
subject "$REPO" 10-somesubject.sh 3
rm -rf "$REPO/.git"
out="$(run_runner "$REPO" || true)"
# THE SURVIVAL NEEDLE TRACKS THE CURRENT WORDING. It used to read `could NOT be
# classified`, which this commit renamed — a guard matching text that can no
# longer appear never fires, so the arm would have passed on a surviving mutant.
case "$out" in
  *'NO TRACKED NUMBER'*) fail "M03 SURVIVED: unclassifiable subjects were still reported apart" ;;
esac
case "$out" in
  *'3 tests passed  (TRACKED only'*) : ;;
  *) fail "M03 VACUOUS: the mutant did not claim the unclassifiable subject as tracked, so the mutated branch may never have run — tail: $(printf '%s' "$out" | tail -4 | tr '\n' ' ')" ;;
esac
saw_mutant "M03 folds unclassifiable subjects into the tracked count, claiming the commit holds unchecked work" "$out" \
  'M03-UNKNOWN-IS-TRACKED'

# M04 — the arithmetic self-check removed. A split that does not add up would put
# a confident wrong number in the headline, which is worse than no split.
mutant_runner M04; mutate_anchor 55-M04 "$REPO/setup/test-hw" \
  'printf "M04-SUM-CHECK-REMOVED'"$BS"'n" >&2; if false; then'
subject "$REPO" 10-committed.sh 3
commit_all "$REPO"
out="$(run_runner "$REPO" || true)"
saw_mutant "M04 removes the arithmetic self-check that stops a mis-attributed headline being printed" "$out" \
  'M04-SUM-CHECK-REMOVED'

# M05 — THE PRE-FIX SPLIT RESTORED EXACTLY: an unqualified headline on stdout
# with the correction on stderr. This is the reported defect, and the arm reads
# ONLY stdout, which is what makes it the defect rather than a wording change.
mutant_runner M05; mutate_anchor 55-M05 "$REPO/setup/test-hw" \
  "printf '${BS}nM05-UNQUALIFIED %s tests passed  (TRACKED only)${BS}n' "'"$TRACKED_OK"; printf '"'"'  no untracked subjects ran, so this is the whole suite'"$BS"'n'"'"'; printf '"'"'  ! could NOT be classified'"$BS"'n'"'"' >&2'
subject "$REPO" 10-somesubject.sh 3
rm -rf "$REPO/.git"
out="$(stdout_only "$REPO" || true)"
case "$out" in
  *'could NOT be classified'*) fail "M05 VACUOUS: the stderr correction leaked onto stdout, so this arm is not modelling a caller that dropped stderr" ;;
esac
saw_mutant "M05 puts the headline on stdout and its correction on stderr, so a caller that drops stderr quotes a wrong complete-sounding number" "$out" \
  'M05-UNQUALIFIED 0 tests passed  (TRACKED only)'

# M06 — the abort marker removed, so a stdout-only reader sees a tail of ok
# lines and nothing saying the run died.
mutant_runner M06; mutate_anchor 55-M06 "$REPO/setup/test-hw" \
  "abort_marker() { printf 'M06-ABORT-MARKER-REMOVED${BS}n' >&2; }"
subject "$REPO" 10-fine.sh 2
{ printf '%s\n' '#!/usr/bin/env bash' \
                 '. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"' \
                 'fail "deliberate red"'; } > "$REPO/setup/tests/11-red.sh"
chmod +x "$REPO/setup/tests/11-red.sh"
commit_all "$REPO"
out="$( ( cd "$REPO/setup" && ./test-hw 2>&1 || true ) )"
case "$out" in
  *'RUN ABORTED'*) fail "M06 SURVIVED: the abort marker was still printed" ;;
esac
saw_mutant "M06 removes the stdout abort marker, so an aborted run ends on ok lines that read as green" "$out" \
  'M06-ABORT-MARKER-REMOVED'

[ "$CLAIMS" -ge 1 ] || fail "no behaviour claims were made"
printf 'coverage - %s behaviour claims, %s dedicated production mutants\n' "$CLAIMS" 6
