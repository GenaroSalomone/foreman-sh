#!/usr/bin/env bash
# `Reverses:` names its target by ENGRAM ID, and nothing resolves by title.
#
# WHY THIS IS A SEPARATE FILE FROM "supersession works". Supersession keyed on
# the entry TITLE first, and the mechanism worked — the wrong entries just moved.
# A title is prose: it is edited after the ruling settles, it is not unique
# across a project, and `—` and `-` are two different characters in a string
# nobody proofreads. So the arms below are not about moving an entry to an
# archive; they are about WHICH entry, and every one of them is satisfiable by a
# title-keyed implementation in the wrong way.
#
# Measured against the title-keyed binary on 2026-09-08, arm 1's fixture:
#
#     decisions supersede setup --apply
#     -> nothing to supersede: no live ruling reverses another live ruling
#
# exit 0, file untouched, and the reversal the author wrote silently did
# nothing. That is the failure this file exists to keep out: not a crash, a
# no-op wearing a success exit.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

tree() {   # tree <name> -> a disposable ROOT with bin/decisions in it
  local name="$1"
  rm -rf "$TMP/$name"
  mkdir -p "$TMP/$name/bin" "$TMP/$name/setup"
  cp "$ROOT/bin/decisions" "$TMP/$name/bin/decisions"
  chmod +x "$TMP/$name/bin/decisions"
  printf '%s\n' "$TMP/$name"
}
dec() { local t="$1"; shift; (cd "$t" && python3 bin/decisions "$@" 2>&1) ; }
live() { cat "$1/setup/decisions.md"; }
# The live sequence BY IDENTITY, one id per line, in file order. Not a count:
# the 2026-09-08 incident passed every count it was given and had moved the
# newest entries instead of the oldest. If an assertion below can be satisfied
# with the same entries in another order, it is not asserting conservation.
ids() { grep -o '^\*\*Evidence:\*\* engram #[0-9]*' "$1/setup/decisions.md" | grep -o '[0-9]*$' | tr '\n' ' '; }
arch_ids() { cat "$1"/setup/decisions/*.md 2>/dev/null | grep -o '^\*\*Evidence:\*\* engram #[0-9]*' | grep -o '[0-9]*$' | tr '\n' ' '; }

# ── 1. THE ARM THE TITLE KEY CANNOT PASS ───────────────────────────────────
# Two live entries share a title on purpose, and they are DIFFERENT rulings.
# Keyed on the title, #100 and #200 are one name and the reversal is ambiguous
# or empty. Keyed on the id they are two entries and exactly one is reversed.
T="$(tree byid)"
cat > "$T/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — a ruling whose title is not unique
**Ruling:** the old answer.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-02-01 — a ruling whose title is not unique
**Ruling:** a different ruling that happens to share the title.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #200

## 2026-03-01 — the ruling that overturns exactly one of them
**Ruling:** the new answer.
**Rules out:** the old answer.
**Reverses:** #100
**Evidence:** engram #300
MD
[ "$(ids "$T")" = "100 200 300 " ] \
  || fail "the fixture is not what this file thinks it is: [$(ids "$T")]"
OUT="$(dec "$T" supersede setup --apply)" || fail "supersede refused the id fixture: $OUT"
case "$OUT" in
  *"nothing to supersede"*)
    fail "the reversal resolved to nothing — Reverses was read as a title, not an id: $OUT" ;;
esac
[ "$(ids "$T")" = "200 300 " ] \
  && pass "the reversed ruling left the live file, and by ID: the entry sharing its TITLE stayed" \
  || fail "the live sequence is [$(ids "$T")], expected [200 300 ]: $OUT"
[ "$(arch_ids "$T")" = "100 " ] \
  && pass "and the loser is in the archive, exactly once, by identity" \
  || fail "the archive holds [$(arch_ids "$T")], expected [100 ]"
case "$OUT" in
  *"VERIFIED by re-reading"*) pass "and the command certified the result by re-reading it" ;;
  *) fail "no re-read verdict was printed: $OUT" ;;
esac

# ── 2. A TITLE IN `Reverses:` IS SKIPPED, NAMED, AND NEVER GUESSED AT ───────
# The old format's own syntax must not keep half-working: it is TOLD, because
# the alternative is that it looks accepted and reverses nothing. It used to
# refuse the WHOLE lane (and so every other, unambiguous reversal in it, for as
# long as nobody fixed one line: `none (see the archive)` did exactly that to
# setup for weeks, 2026-10-05). Now the entry is skipped with a warning that
# names it, and the lane's other reversals go ahead.
T2="$(tree bytitle)"
cat > "$T2/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — the old ruling
**Ruling:** the old answer.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-03-01 — the new ruling
**Ruling:** the new answer.
**Rules out:** the old answer.
**Reverses:** the old ruling
**Evidence:** engram #300
MD
OUT="$(dec "$T2" supersede setup --apply || true)"
case "$OUT" in
  *"warning: skipped"*"the new ruling"*"not an engram id"*)
    pass "a \`Reverses\` naming a title is skipped with a warning that names the entry and says it needs an id" ;;
  *"nothing to supersede"*)
    fail "a title-shaped Reverses was silently ignored, which is the no-op-with-exit-0 failure: $OUT" ;;
  *) fail "the title-shaped Reverses neither warned nor explained itself: $OUT" ;;
esac
[ "$(ids "$T2")" = "100 300 " ] \
  && pass "and nothing moved on its word: both entries are still live, in order" \
  || fail "the skipped entry changed the file: [$(ids "$T2")]"

# ── 2b. ONE UNREADABLE LINE DOES NOT REFUSE THE LANE ────────────────────────
# The measured case: a `Reverses: none (precisa ...)` on one entry refused every
# supersession in the project. Old: exit 1, "nothing was written", the good
# reversal (#100 by #300) stayed live. New: that one is warned about by name and
# the good one moves.
T2B="$(tree onebad)"
cat > "$T2B/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — the old ruling
**Ruling:** the old answer.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-02-01 — a ruling with a note where the id goes
**Ruling:** it narrows another, it does not reverse it.
**Rules out:** nothing yet.
**Reverses:** none (precisa el alcance de otro)
**Evidence:** engram #200

## 2026-03-01 — the ruling that overturns the first
**Ruling:** the new answer.
**Rules out:** the old answer.
**Reverses:** #100
**Evidence:** engram #300
MD
cp "$T2B/setup/decisions.md" "$TMP/onebad.md"
OUT="$(dec "$T2B" supersede setup --apply)" || fail "one unreadable Reverses refused the lane: $OUT"
case "$OUT" in *"warning: skipped"*"a ruling with a note where"*) ;; *) fail "the unreadable entry was not named in a warning: $OUT" ;; esac
[ "$(ids "$T2B")" = "200 300 " ] && [ "$(arch_ids "$T2B")" = "100 " ] \
  && pass "an unreadable Reverses is named and skipped, and the unambiguous reversal still moves" \
  || fail "the good reversal did not move beside the skipped one: live [$(ids "$T2B")] archive [$(arch_ids "$T2B")]"
M1="$TMP/mut109"; rm -rf "$M1"; mkdir -p "$M1/bin" "$M1/setup"
cp "$ROOT/bin/decisions" "$M1/bin/decisions"; cp "$TMP/onebad.md" "$M1/setup/decisions.md"
# M01: the unreadable Reverses refuses the lane again.
mutate_anchor 109-M01 "$M1/bin/decisions" 'problems = unreadable_reverses(ordered) + problems'
OUT="$(dec "$M1" supersede setup --apply || true)"
saw_mutant "109 M01 an unreadable Reverses refuses the whole lane" "$OUT" "nothing was written"

# ── 3. AN ID THAT NAMES NOTHING ANYWHERE IS A TYPO, AND IT REFUSES ────────
# THIS ARM ASSERTED THE OPPOSITE UNTIL 2026-09-08 and was wrong in the most
# embarrassing available way: it required the silent no-op that this file's own
# header condemns. Two blind reviewers caught it independently. An entry with
# `Evidence: none` has no identity, so `Reverses: #100` names nothing — and
# "nothing to supersede", exit 0, is a success wearing the author's belief that
# a reversal happened.
T3="$(tree noid)"
cat > "$T3/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — a legacy entry from before the ruling format
**Ruling:** the old answer.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** none

## 2026-03-01 — the new ruling
**Ruling:** the new answer.
**Rules out:** the old answer.
**Reverses:** #100
**Evidence:** engram #300
MD
OUT="$(dec "$T3" supersede setup --apply || true)"
case "$OUT" in
  *"no entry of this project"*)
    pass "a Reverses naming an id that is in no file at all is REFUSED as a typo" ;;
  *"nothing to supersede"*)
    fail "the dangling id was a silent no-op with exit 0 — the failure this file exists to prevent: $OUT" ;;
  *) fail "the dangling-id case neither refused nor explained itself: $OUT" ;;
esac
[ "$(grep -c '^## ' "$T3/setup/decisions.md")" = 2 ] \
  && pass "and the refusal wrote nothing" \
  || fail "the refusal changed the file: $(live "$T3")"
OUT="$(dec "$T3" check || true)"
case "$OUT" in
  *"no engram id"*)
    pass "and \`check\` names the entries that no future ruling can reverse" ;;
  *) fail "check is silent about entries with no engram id: $OUT" ;;
esac

# ── 3b. BUT AN ALREADY-ARCHIVED TARGET STAYS SILENT ────────────────────────
# The other half, and it is why the refusal above had to be narrowed to "in no
# file at all" rather than "not live". Superseding twice must not refuse the
# second time: an operation that cannot be repeated is one nobody can safely
# re-run after an interruption.
T3B="$(tree rerun)"
cat > "$T3B/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — the ruling that gets retired
**Ruling:** the old answer.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-03-01 — the new ruling
**Ruling:** the new answer.
**Rules out:** the old answer.
**Reverses:** #100
**Evidence:** engram #300
MD
dec "$T3B" supersede setup --apply >/dev/null 2>&1 || fail "the first supersession failed"
[ "$(ids "$T3B")" = "300 " ] || fail "the first supersession did not retire #100: [$(ids "$T3B")]"
OUT="$(dec "$T3B" supersede setup --apply)" || fail "the SECOND run refused: $OUT"
case "$OUT" in
  *"nothing to supersede"*)
    pass "running supersede again is silent and exits 0, so an interrupted run is safe to repeat" ;;
  *) fail "the re-run did not report itself as a no-op: $OUT" ;;
esac

# ── 4. TWO LIVE ENTRIES CITING ONE ID IS AMBIGUOUS, SO IT REFUSES ──────────
# engram will not issue an id twice, but a person types it into the file. There
# is no safe reading of a reversal that names two entries, and this command
# moves files — picking one buries the typo under a conservation verdict.
T4="$(tree dupid)"
cat > "$T4/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — first entry
**Ruling:** one.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-02-01 — second entry with a mistyped id
**Ruling:** two.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-03-01 — the reverser
**Ruling:** three.
**Rules out:** one of the two above, and nobody can say which.
**Reverses:** #100
**Evidence:** engram #300
MD
OUT="$(dec "$T4" supersede setup --apply || true)"
case "$OUT" in
  *"live entries cite that id"*)
    pass "a reversal matching two live entries is refused and names the ambiguity" ;;
  *"VERIFIED by re-reading"*)
    fail "the ambiguous reversal picked one and certified it: $OUT" ;;
  *) fail "the duplicate-id case neither refused nor explained itself: $OUT" ;;
esac
[ "$(ids "$T4")" = "100 100 300 " ] \
  && pass "and nothing was written" \
  || fail "the refusal still changed the file: [$(ids "$T4")]"

# ── 5. A RULING THAT REVERSES ITS OWN EVIDENCE IS REFUSED ──────────────────
T5="$(tree selfref)"
cat > "$T5/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — a standing ruling
**Ruling:** one.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-03-01 — a ruling that eats itself
**Ruling:** three.
**Rules out:** itself, apparently.
**Reverses:** #300
**Evidence:** engram #300
MD
OUT="$(dec "$T5" supersede setup --apply || true)"
case "$OUT" in
  *"names its own evidence"*)
    pass "a ruling reversing its own engram id is refused" ;;
  *) fail "the self-reference was not caught: $OUT" ;;
esac

# ── 6. THE SPELLINGS OF ONE ID ARE ONE ID ──────────────────────────────────
# `Evidence:` writes `engram #NNNNN` and `Reverses:` writes `#NNNNN`. If those
# were read as different strings no reversal would ever match its target, and
# the whole feature would be a no-op that never fails a test that counts.
T6="$(tree spelling)"
cat > "$T6/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 — the old ruling
**Ruling:** one.
**Rules out:** nothing yet.
**Reverses:** none
**Evidence:** engram #100

## 2026-03-01 — the new ruling, naming the target the long way
**Ruling:** three.
**Rules out:** one.
**Reverses:** engram #100
**Evidence:** engram #300
MD
OUT="$(dec "$T6" supersede setup --apply)" || fail "the long spelling was refused: $OUT"
[ "$(ids "$T6")" = "300 " ] \
  && pass "\`Reverses: engram #100\` and \`Reverses: #100\` name the same ruling" \
  || fail "the long spelling did not resolve: [$(ids "$T6")] — $OUT"
