#!/usr/bin/env bash
# A mutation arm nobody runs is not coverage.
#
# WHY THIS SUBJECT EXISTS. On 2026-08-28 the brainer re-measured a delivered task
# that reported "killed 18 committed-byte mutants". All 18 existed and all 18
# died — and none of them ran. Every one of the five new subject files wrapped
# its arms in `if [ "${<SUBJECT>_MUTATION_TEST:-0}" = 1 ]`, and `setup/test-hw`
# sets no such variable, so a full working-tree run — 909 ok, 0 not ok — printed
# zero mutant lines. Seven subject files were in that shape (39, 40, 41, 42, 43,
# 44, 45): every mutation-carrying subject added since the convention appeared,
# one session earlier, inherited it.
#
# The stated reason was that arms must run "only against committed bytes". The
# arms do not do that: they copy from $ROOT, the working tree, exactly as the
# ungated arms in 26, 27 and 35 do. The gate bought nothing and cost the whole of
# the coverage it guarded.
#
# THE SHAPE, and it is this repository's defining one: a proof that lives outside
# the gate does not exist. `ok -` lines are not evidence that the claims behind
# them are pinned; only a dead mutant is, and only if something runs it.
#
# WHY IT CHECKS OUTPUT AND NOT SOURCE SHAPE. The first attempt scanned for the
# gate idiom and flagged four innocents — 31, 34 and 36 gate an optional LIVE arm
# and kill their mutants regardless, and a fixture heredoc matched too. A check
# that cries wolf is ignored within a day, which is worse than none. What a
# subject PRINTED has no false positives and no idiom to keep up with: any future
# way of skipping an arm fails it.
#
# This subject drives `setup/mutation-coverage`, which the RUNNER applies to
# every subject file it runs. One copy of the predicate, two callers.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

COV="$ROOT/setup/mutation-coverage"
[ -x "$COV" ] || fail "C00 setup/mutation-coverage is missing or not executable"

FIX="$TMP/fix"; mkdir -p "$FIX"
run_cov() { "$COV" "$1" "$2" >/dev/null 2>&1; printf '%s' "$?"; }

# The three kill-marker conventions this suite actually uses, as subjects…
printf '%s\n' '#!/usr/bin/env bash' 'pass "mutant killed: M01 something"'  > "$FIX/says-spaced.sh"
printf '%s\n' '#!/usr/bin/env bash' 'mutant_killed "M01 something"'        > "$FIX/calls-helper.sh"
printf '%s\n' '#!/usr/bin/env bash' "printf 'mutant-killed - M01 x\\n'"    > "$FIX/prints-hyphen.sh"
# …and as the output a run produces.
printf '%s\n' 'ok - C01 a claim' 'ok - mutant killed: M01 something' > "$FIX/out-spaced"
printf '%s\n' 'ok - C01 a claim' 'mutant-killed - M01 something'     > "$FIX/out-hyphen"
printf '%s\n' 'ok - C01 a claim'                                    > "$FIX/out-silent"
# A subject that pins nothing, and one that only talks about mutants.
printf '%s\n' '#!/usr/bin/env bash' 'pass "C01 a claim with nothing pinned"' > "$FIX/declares-not.sh"
printf '%s\n' '#!/usr/bin/env bash' '# one mutant killed here would be nice'  > "$FIX/prose.sh"

# ── C01: the finding this whole subject exists for ───────────────────────────
[ "$(run_cov "$FIX/says-spaced.sh" "$FIX/out-silent")" = 1 ] \
  || fail "C01 a subject that declares arms and exercised none was not reported"
pass "C01 declared mutation arms that no run exercised are a failure, not a pass"

# ── C02: and it must not fire on a run that did exercise one ─────────────────
[ "$(run_cov "$FIX/says-spaced.sh" "$FIX/out-spaced")" = 0 ] \
  || fail "C02 an exercised mutation arm was reported as missing"
pass "C02 a run that exercised an arm passes"

# ── C03: all three conventions are recognised, in both directions ────────────
bad=""
for s in says-spaced calls-helper prints-hyphen; do
  [ "$(run_cov "$FIX/$s.sh" "$FIX/out-silent")" = 1 ] || bad="$bad declaration:$s"
done
for o in out-spaced out-hyphen; do
  [ "$(run_cov "$FIX/says-spaced.sh" "$FIX/$o")" = 0 ] || bad="$bad evidence:$o"
done
[ -z "$bad" ] || fail "C03 a kill-marker convention is not recognised:$bad"
pass "C03 all three kill-marker conventions in this suite are recognised as declaration and as evidence"

# ── C04/C05: no crying wolf — the two innocents ──────────────────────────────
[ "$(run_cov "$FIX/declares-not.sh" "$FIX/out-silent")" = 0 ] \
  || fail "C04 a subject declaring no mutation arm was required to have one"
pass "C04 a subject that pins nothing is left alone, so the check cannot cry wolf"

[ "$(run_cov "$FIX/prose.sh" "$FIX/out-silent")" = 0 ] \
  || fail "C05 a comment mentioning a killed mutant was read as a declaration"
pass "C05 a comment is not a declaration, so this file's own header is not a promise"

# ── C06: unreadable input is never a silent pass ─────────────────────────────
[ "$(run_cov "$FIX/says-spaced.sh" "$FIX/no-such-output")" = 2 ] \
  || fail "C06 an unreadable output file did not produce the distinct usage status"
pass "C06 an unreadable input is its own status, never a pass"

# ── C07–C10: a kill certified by a catch-all is refused, and only that ───────
#
# THE SECOND WAY AN ARM LIES: a mutant that dies on an unrelated environment
# error before ever reaching the mutated line can still land in the "kill"
# arm of a catch-all —
# `*'original'*) fail ;; *) pass "mutant killed"` — read the silence as a kill.
# The output check above cannot see that: the marker WAS printed. So the
# predicate reads the source for that one shape, in its three spellings, and
# refuses it with its own status (3), whatever the output says.
# The catch-all token is assembled from a variable so that THIS file's source
# never spells the refused shape itself — the predicate reads this file too,
# and a fixture that looked like the defect was the first false positive it
# produced (the same trap the header describes for the heredoc in 2026-08-28).
CA='*)'
printf '%s\n' '#!/usr/bin/env bash' \
  'case "$out" in *"original"*) fail "M01 SURVIVED" ;; '"$CA"' pass "mutant killed: M01 x" ;; esac' \
  > "$FIX/catchall-oneline.sh"
printf '%s\n' '#!/usr/bin/env bash' 'case "$out" in' '  *"original"*) fail "M01 SURVIVED" ;;' \
  '  '"$CA"' pass "mutant killed: M01 x" ;;' 'esac' > "$FIX/catchall-sameline.sh"
printf '%s\n' '#!/usr/bin/env bash' 'case "$out" in' '  *"original"*) fail "M01 SURVIVED" ;;' \
  '  '"$CA" '    mutant_killed "M01 x" ;;' 'esac' > "$FIX/catchall-nextline.sh"
printf '%s\n' '#!/usr/bin/env bash' \
  'case "$out" in *"original"*) fail "SURVIVED" ;; '"$CA"' pass "mutation: reverting the fix reproduces the defect" ;; esac' \
  > "$FIX/catchall-mutation-word.sh"
# …and the two shapes that must NOT be refused: the positive arm, and a
# catch-all that FAILS while mentioning the mutant.
printf '%s\n' '#!/usr/bin/env bash' \
  'case "$out" in *"what only the mutant says"*) pass "mutant killed: M01 x" ;; *) fail "M01 VACUOUS: mutant killed nothing visible" ;; esac' \
  > "$FIX/positive.sh"
printf '%s\n' '#!/usr/bin/env bash' 'saw_mutant "M01 x" "$out" "what only the mutant says"' > "$FIX/positive-helper.sh"

bad=""
for s in catchall-oneline catchall-sameline catchall-nextline catchall-mutation-word; do
  # out-spaced, not out-silent: the output DID carry a marker, so only the shape
  # check can be what refuses these.
  [ "$(run_cov "$FIX/$s.sh" "$FIX/out-spaced")" = 3 ] || bad="$bad $s"
done
[ -z "$bad" ] || fail "C07 a kill certified by a catch-all branch was not refused:$bad"
pass "C07 a kill certified by a \`*)\` catch-all is refused with its own status, in all three spellings and for the 'mutation:' wording"

[ "$(run_cov "$FIX/positive.sh" "$FIX/out-spaced")" = 0 ] \
  || fail "C08 a positive arm — the mutant's own text in a matching branch, the catch-all failing — was refused"
pass "C08 a positive arm whose catch-all FAILS is not refused, so the check cannot cry wolf on the correct shape"

# ── C09. THE HELPER FORM MUST BE ENFORCED, NOT MERELY ACCEPTED ─────────────
#
# THIS ASSERTION USED TO BE THE MASK, and that is worse than the bug it hid.
# It was one line — `run_cov positive-helper.sh out-spaced` must be 0 — and it
# passed for the wrong reason for as long as it existed. Exit 0 is ALSO what the
# predicate returns for a subject whose arms it cannot SEE: `saw_mutant` puts no
# marker in the source, the declaration gate found nothing to enforce, and the
# early `exit 0` satisfied this check. So the recommended shape was unenforced,
# sixteen arms across subjects 66, 67 and 68 were never evaluated, and the
# dashboard said green. Found 2026-09-02 by the opus Judgment Day reviewer; a
# masked check is more dangerous than a missing one for exactly this reason.
#
# THE FIX IS THAT IT IS NOW TWO-SIDED, because "accepted" and "enforced" are
# different facts and only the pair can tell them apart:
#   · with evidence in the output → 0, the shape is not refused (no wolf);
#   · with NO evidence            → 1, the promise IS being enforced.
# One-sided, either half is satisfiable by a predicate that never looked.
[ "$(run_cov "$FIX/positive-helper.sh" "$FIX/out-spaced")" = 0 ] \
  || fail "C09a the saw_mutant helper form was refused even with its evidence present"
[ "$(run_cov "$FIX/positive-helper.sh" "$FIX/out-silent")" = 1 ] \
  || fail "C09b the saw_mutant helper form is NOT ENFORCED: a subject declaring it, whose run exercised no arm, was not refused (got $(run_cov "$FIX/positive-helper.sh" "$FIX/out-silent"), wanted 1). This is the masking defect — exit 0 here is what a predicate that cannot see the helper returns."
pass "C09 the saw_mutant helper form is both accepted with its evidence and ENFORCED without it, so an unseen arm cannot pass as a seen one"

# ── C11. an EMPTY output file is never a pass ──────────────────────────────
#
# The same disease as C09's mask, one level down: every other branch reads the
# output only to look for evidence IN it, so "nothing was printed" fell out of
# the bottom as a pass. A subject that printed nothing did not run.
printf '' > "$FIX/out-empty"
printf '   \n\n' > "$FIX/out-whitespace"
[ "$(run_cov "$FIX/says-spaced.sh" "$FIX/out-empty")" = 4 ] \
  || fail "C11 an EMPTY output file was not refused (got $(run_cov "$FIX/says-spaced.sh" "$FIX/out-empty"))"
[ "$(run_cov "$FIX/says-spaced.sh" "$FIX/out-whitespace")" = 4 ] \
  || fail "C11 a whitespace-only output file was not refused"
# AND IT IS REFUSED FOR A SUBJECT THAT DECLARES NOTHING TOO. An empty capture is
# a statement about the RUN, not about the arms, so the declaration gate must not
# be able to excuse it — that gate returning 0 early is what made this reachable.
[ "$(run_cov "$FIX/declares-not.sh" "$FIX/out-empty")" = 4 ] \
  || fail "C11 an empty output was excused for a subject that declares no arm"
pass "C11 an empty or whitespace-only output file is refused with its own status, declaration or not"

[ "$(run_cov "$FIX/catchall-oneline.sh" "$FIX/out-silent")" = 3 ] \
  || fail "C10 the shape refusal did not take precedence over the exercised-arms check"
pass "C10 the shape refusal is decided before the exercised-arms check, so a vacuous arm is named as vacuous and not as unexercised"

# THE LIVE ASSERTION IS THE RUNNER'S, NOT THIS FILE'S, and deliberately so.
# `setup/test-hw` already captures each subject's output, so applying the
# predicate there costs nothing and covers every subject it runs. An earlier
# draft looped over the whole directory here instead — correct, and it re-ran the
# entire suite inside one subject file, roughly doubling its wall clock for a
# second copy of an answer the runner already has. What this file owns is the
# predicate's behaviour and the mutants that pin it.

# ── mutation arms, ungated on purpose — this file is the reason why ──────────
# `mut <name>` prints the path of a copy of the predicate; the caller then edits it
# with `mutate_anchor <id> "$path" <replacement>`. The edit lands on the line (or block) a `# MUTATION-ANCHOR: 46-Mnn`
# marker in setup/mutation-coverage declares, not on the prose of that line; see
# mutate_anchor in _common.sh, which fails loudly when its anchor is gone, so a
# harness that cannot build its mutant never reads as a surviving one.
mut() {
  # THREE STATEMENTS, NOT ONE `local`. `local a="$1" b="$TMP/x.$a"` reads as
  # left-to-right and is not: under `set -u` bash 3.2 evaluates the whole
  # declaration before binding any of it, so `$a` is unbound. This suite runs
  # under bash 3.2 on purpose (see _common.sh) and this is one of its traps.
  local name="$1"
  local out="$TMP/cov.$name"
  cp "$COV" "$out" || return 1
  chmod +x "$out" || return 1
  printf '%s' "$out"
}

# rc <mutant> <subject> <output>  → the mutant's exit status, nothing else
rc() { "$1" "$2" "$3" >/dev/null 2>&1; printf '%s' "$?"; }

M1="$(mut m01)" || fail "M01 could not be built"
mutate_anchor 46-M01 "$M1" 'exit 0' || fail "M01 could not be built"
[ "$(rc "$M1" "$FIX/says-spaced.sh" "$FIX/out-silent")" = 0 ] \
  || fail "M01 SURVIVED: short-circuiting the declaration check did not turn C01 into a pass"
pass "mutant killed: M01 short-circuiting the declaration check turns the C01 finding into a pass"

M2="$(mut m02)" || fail "M02 could not be built"
mutate_anchor 46-M02 "$M2" 'exit 0' || fail "M02 could not be built"
[ "$(rc "$M2" "$FIX/says-spaced.sh" "$FIX/out-silent")" = 0 ] \
  || fail "M02 SURVIVED: ignoring the evidence marker did not let a silent run pass"
pass "mutant killed: M02 accepting any output as evidence lets a silent run pass C01"

M3="$(mut m03)" || fail "M03 could not be built"
mutate_anchor 46-M03 "$M3" 'if [ -r "$OUTPUT" ] && false; then' || fail "M03 could not be built"
[ "$(rc "$M3" "$FIX/says-spaced.sh" "$FIX/no-such-output")" != 2 ] \
  || fail "M03 SURVIVED: dropping the unreadable-output guard still produced the usage status"
pass "mutant killed: M03 dropping the unreadable-output guard stops an absent run being its own status"

M4="$(mut m04)" || fail "M04 could not be built"
mutate_anchor 46-M04 "$M4" 'COMMENT='\''^\$a^'\''' || fail "M04 could not be built"
[ "$(rc "$M4" "$FIX/prose.sh" "$FIX/out-silent")" = 1 ] \
  || fail "M04 SURVIVED: dropping the comment exclusion did not make prose a declaration"
pass "mutant killed: M04 dropping the comment exclusion makes a comment a declaration, which C05 refuses"

M5="$(mut m05)" || fail "M05 could not be built"
mutate_anchor 46-M05 "$M5" 'MARKER='\''mutant killed'\''' || fail "M05 could not be built"
[ "$(rc "$M5" "$FIX/calls-helper.sh" "$FIX/out-silent")" = 0 ] \
  || fail "M05 SURVIVED: narrowing the marker to one convention still saw the helper form"
pass "mutant killed: M05 narrowing the marker to one convention blinds it to the helper form, which C03 refuses"

# ── mutants of the shape check — each judged by the exit the MUTANT produces ──
# `= 0` is positive evidence here: the correct predicate answers 3, and a mutant
# that failed to build or crashed answers 2, 126 or 127 — nothing but the
# mutated logic can answer 0 on these fixtures.
M6="$(mut m06)" || fail "M06 could not be built"
mutate_anchor 46-M06 "$M6" 'exit 0' || fail "M06 could not be built"
[ "$(rc "$M6" "$FIX/catchall-oneline.sh" "$FIX/out-spaced")" = 0 ] \
  || fail "M06 SURVIVED: turning the shape refusal into a pass did not let a catch-all kill through"
pass "mutant killed: M06 turning the catch-all refusal into a pass lets a vacuous arm through, which C07 refuses"

M7="$(mut m07)" || fail "M07 could not be built"
mutate_anchor 46-M07 "$M7" 'else if (0) print NR ": " line' || fail "M07 could not be built"
[ "$(rc "$M7" "$FIX/catchall-nextline.sh" "$FIX/out-spaced")" = 0 ] \
  || fail "M07 SURVIVED: dropping the previous-line branch still saw a catch-all on its own line"
pass "mutant killed: M07 dropping the previous-line spelling blinds the check to a catch-all on its own line, which C07 refuses"

M8="$(mut m08)" || fail "M08 could not be built"
mutate_anchor 46-M08 "$M8" 'else if (0) print NR ": " line' || fail "M08 could not be built"
[ "$(rc "$M8" "$FIX/catchall-oneline.sh" "$FIX/out-spaced")" = 0 ] \
  || fail "M08 SURVIVED: dropping the one-line-case branch still saw an inline catch-all"
pass "mutant killed: M08 dropping the one-line-case spelling blinds the check to an inline catch-all, which C07 refuses"

M9="$(mut m09)" || fail "M09 could not be built"
mutate_anchor 46-M09 "$M9" 'if (line ~ /mutant_killed[[:space:]]+["'\''"'\''"'\'']/ || line ~ /pass[[:space:]]+["'\''"'\''"'\''][^"'\''"'\''"'\'']*[Mm]utant killed/) {' || fail "M09 could not be built"
[ "$(rc "$M9" "$FIX/catchall-mutation-word.sh" "$FIX/out-spaced")" = 0 ] \
  || fail "M09 SURVIVED: narrowing the arm wording to the marker still caught a 'mutation:' arm"
pass "mutant killed: M09 narrowing the shape net to the marker convention misses the 'mutation:' wording five subjects use, which C07 refuses"

# ── M10/M11. the two halves the reviewer found, each with its own mutant ───
#
# C09b and C11 are new claims, so they get new mutants. Without these the fix
# could be reverted and 46 would stay green — which is exactly the history being
# closed here.
M10="$(mut M10)" || fail "M10 could not be built"
mutate_anchor 46-M10 "$M10" 'DECLARE_MARKER="$MARKER"' || fail "M10 could not be built"
[ "$(rc "$M10" "$FIX/positive-helper.sh" "$FIX/out-silent")" = 0 ] \
  || fail "M10 SURVIVED: with saw_mutant removed from the declaration pattern the helper form was still enforced"
pass "mutant killed: M10 drops saw_mutant from the declaration pattern, and the helper form goes unenforced again — the reviewer's finding"

M11="$(mut M11)" || fail "M11 could not be built"
mutate_anchor 46-M11 "$M11" 'if false; then' || fail "M11 could not be built"
[ "$(rc "$M11" "$FIX/says-spaced.sh" "$FIX/out-empty")" = 1 ] \
  || fail "M11 SURVIVED: an empty output was still given its own status with the emptiness gate removed"
[ "$(rc "$M11" "$FIX/declares-not.sh" "$FIX/out-empty")" = 0 ] \
  || fail "M11 VACUOUS: the mutant did not restore the old pass-on-empty behaviour for a declaration-free subject"
pass "mutant killed: M11 removes the emptiness gate, and an empty output passes again for a subject that declares nothing"

# ── C12-C16 / M12-M13. the native-Windows exemption, and how narrow it is ────
#
# A copy of the predicate with a list of its own beside it (it reads the one in
# its own directory), and the platform as `uname -s` says it, stubbed first on
# PATH: both platforms are asked on every platform.
cp "$COV" "$TMP/cov.healthy" && chmod +x "$TMP/cov.healthy" || fail "C12 the predicate could not be copied"
printf '%s\t%s\t%s\n' \
  one-arm.sh M01 "a reason of one line" \
  two-arms.sh M01 "only the first arm is listed" \
  no-reason.sh M01 "" \
  wild.sh '*' "a wildcard, which matches no arm" \
  half-named.sh M01 "its other arm has no name to list" > "$TMP/mutation-exempt-native-windows.tsv"
printf '%s\n' '#!/usr/bin/env bash' 'saw_mutant "M01 a thing" "$x" "y"' 'pass "mutant killed: an arm with no id"' > "$FIX/half-named.sh"
for p in one-arm no-reason wild; do
  printf '%s\n' '#!/usr/bin/env bash' 'saw_mutant "M01 a thing" "$x" "y"' > "$FIX/$p.sh"
done
printf '%s\n' '#!/usr/bin/env bash' 'saw_mutant "M01 a thing" "$x" "y"' 'saw_mutant "M02 another" "$x" "y"' > "$FIX/two-arms.sh"
mkdir -p "$TMP/os-win" "$TMP/os-mac"
printf '#!/bin/sh\necho MINGW64_NT-10.0-20348\n' > "$TMP/os-win/uname"
printf '#!/bin/sh\necho Darwin\n' > "$TMP/os-mac/uname"
chmod +x "$TMP/os-win/uname" "$TMP/os-mac/uname"
# on <win|mac> <gate> <subject> → "<rc> <stdout+stderr>"
on() { local o rc=0; o="$(PATH="$TMP/os-$1:$PATH" "$2" "$3" "$FIX/out-silent" 2>&1)" || rc=$?; printf '%s %s' "$rc" "$o"; }
on_rc() { local r; r="$(on "$@")"; printf '%s' "${r%% *}"; }

r="$(on win "$TMP/cov.healthy" "$FIX/one-arm.sh")"
case "$r" in "0 exempt - one-arm.sh M01 on native Windows: a reason of one line") ;; *) fail "C12 a listed arm on native Windows was not exempt, or not named with its reason: $r" ;; esac
pass "C12 on native Windows an arm listed by name with its reason is exempt, and the run prints it"

r="$(on win "$TMP/cov.healthy" "$FIX/two-arms.sh")"
case "$r" in "1 "*"the arms M02 are not in"*) ;; *) fail "C13 a subject with an unlisted arm was exempt: $r" ;; esac
pass "C13 on native Windows an arm that is not listed keeps the subject red, and is named"

[ "$(on_rc win "$TMP/cov.healthy" "$FIX/says-spaced.sh")" = 1 ] || fail "C14 an unlisted subject was exempt on native Windows"
pass "C14 on native Windows a subject the list does not name is red, as before"

r="$(on mac "$TMP/cov.healthy" "$FIX/one-arm.sh")"
case "$r" in "1 "*"exercised none"*) ;; *) fail "C15 the list exempted an arm off native Windows: $r" ;; esac
pass "C15 off native Windows the list exempts nothing"

[ "$(on_rc win "$TMP/cov.healthy" "$FIX/no-reason.sh")" = 1 ] || fail "C16 a row with no reason exempted its arm"
[ "$(on_rc win "$TMP/cov.healthy" "$FIX/wild.sh")" = 1 ] || fail "C16 a '*' row exempted an arm"
pass "C16 a row with no reason, or a '*' for its arm, exempts nothing: names only, each with its reason"

r="$(on win "$TMP/cov.healthy" "$FIX/half-named.sh")"
case "$r" in "1 "*"(1 declared with no arm name)"*) ;; *) fail "C17 an arm with no name rode its named sibling's row into an exemption: $r" ;; esac
pass "C17 on native Windows an arm declared with no name keeps the subject red: the list cannot name it"

M12="$(mut m12)" || fail "M12 could not be built"
mutate_anchor 46-M12 "$M12" '*)' || fail "M12 could not be built"
case "$(on mac "$M12" "$FIX/one-arm.sh")" in
  "0 exempt - one-arm.sh M01"*) pass "mutant killed: M12 dropping the platform test lets the list exempt on macOS and Linux, which C15 refuses" ;;
  *) fail "M12 SURVIVED: with no platform test the list still exempted nothing off native Windows" ;;
esac

M13="$(mut m13)" || fail "M13 could not be built"
mutate_anchor 46-M13 "$M13" ':' || fail "M13 could not be built"
case "$(on win "$M13" "$FIX/two-arms.sh")" in
  "0 exempt - two-arms.sh M01"*) pass "mutant killed: M13 not counting unlisted arms exempts a subject one listed arm covers, which C13 refuses" ;;
  *) fail "M13 SURVIVED: with unlisted arms ignored the two-arm subject was still red" ;;
esac

M14="$(mut m14)" || fail "M14 could not be built"
mutate_anchor 46-M14 "$M14" ':' || fail "M14 could not be built"
case "$(on win "$M14" "$FIX/half-named.sh")" in
  "0 exempt - half-named.sh M01"*) pass "mutant killed: M14 not counting unnamed declarations exempts a subject with an arm the list cannot name, which C17 refuses" ;;
  *) fail "M14 SURVIVED: with unnamed declarations ignored the half-named subject was still red" ;;
esac

printf 'mapping - C01↔M01 declaration · C01↔M02 evidence · C03↔M05 marker breadth · C05↔M04 comment exclusion · C06↔M03 unreadable input · C07↔M06,M07,M08,M09 catch-all shape · C08 no wolf · C09↔M10 helper form enforced · C11↔M11 empty output · C12,C15↔M12 native Windows only · C13↔M13 every arm listed · C17↔M14 an unnamed arm · C14,C16 names only\n'
printf 'coverage - 18 behavior claims, 14 dedicated mutants killed\n'
