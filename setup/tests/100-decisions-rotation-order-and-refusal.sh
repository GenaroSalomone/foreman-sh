#!/usr/bin/env bash
# `decisions archive` must keep the NEWEST entries, refuse what it does not
# recognise, and prove its result by re-reading what it wrote.
#
# WHY. All three were measured failing on the real files, and all three exited 0
# while failing.
#
#   ORDER. The files are append-only, so the newest entry is at the BOTTOM, and
#   the rotator selected `entries[:n]` in file order. Run on setup/decisions.md
#   2026-09-08 it kept 09-01/03/06 live and moved 09-07/08 to the archive — the
#   exact inverse of what a live file is for, and it breaks every CLAUDE.md's
#   "seek it before proposing" by hiding today's rulings. Reverted in 2d831ff.
#   Its own check, 136 entries in and 136 out, could not see it: a verdict about
#   an ordered file has to look at the ORDER, which is why this file asserts on
#   which heading is where and never on a total.
#
#   THE THIRD VALUE. `## <title>` with the date on a separate `**Fecha:**` line
#   was not an entry to the rotator and not its pointer bookkeeping either, so
#   fourteen of them were written out of BOTH files under
#   `25 entries stay, 18 move / wrote / rewrote`, exit 0. Recovered from git,
#   formats normalised in efae689 — the tool destroying what it does not
#   recognise is what this asserts against. Worst below a rotation pointer,
#   where the sentinel skip also made them invisible to `index`, the only path
#   any CLAUDE.md points an agent at.
#
#   MEASUREMENT, NOT ESTIMATE. Retention was chosen from an in-memory cost
#   estimate whose escape hatch was a hardcoded `len(entries) // 5`, and the
#   counts were printed before the write with no re-read. One oversized entry
#   got `nothing to archive`, exit 0, on a file `check` called OVER in the same
#   breath.
#
# Every arm below drives the COMPLETE CLI against a disposable tree, and each
# mutant is one isolated `sed` against a copy of the real binary.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── fixtures ────────────────────────────────────────────────────────────────
# tree <name> [<binary>] -> a disposable ROOT with bin/decisions in it.
tree() {
  local name="$1" bin="${2:-$ROOT/bin/decisions}"
  rm -rf "$TMP/$name"
  mkdir -p "$TMP/$name/bin" "$TMP/$name/setup"
  cp "$bin" "$TMP/$name/bin/decisions"
  chmod +x "$TMP/$name/bin/decisions"
  printf '%s\n' "$TMP/$name"
}
dec() { local t="$1"; shift; (cd "$t" && python3 bin/decisions "$@" 2>&1) ; }
# Mutants live in their own directory: a mutant file sharing a name with a
# fixture tree gets rm -rf'd by `tree`, the mutated binary never runs, and an
# arm reading the untouched fixture then reports a kill it did not observe.
# That happened on the first run of this file.
mkdir -p "$TMP/mut"
M() { printf '%s\n' "$TMP/mut/$1"; }

# Two entries, oldest first, exactly as an append-only file holds them.
two_entries() {
  cat > "$1/setup/decisions.md" <<'MD'
# setup — decisions

Preamble that must survive verbatim.

---

## 2026-01-01 the OLDEST decision

Body of the oldest.

## 2026-08-08 the NEWEST decision

Body of the newest.
MD
}

# ── 1. --keep 1 keeps the NEWEST, not the first in the file ─────────────────
T="$(tree keep1)"; two_entries "$T"
OUT="$(dec "$T" archive setup --keep 1 --apply)" || fail "archive --keep 1 failed: $OUT"
LIVE="$(cat "$T/setup/decisions.md")"
ARCH="$(cat "$T"/setup/decisions/*.md 2>/dev/null || true)"
case "$LIVE" in
  *"the NEWEST decision"*) pass "archive --keep 1: the NEWEST entry is the one left live" ;;
  *) fail "archive --keep 1 kept the wrong entry — live file is: $LIVE" ;;
esac
case "$LIVE" in
  *"the OLDEST decision"*) fail "archive --keep 1 left the OLDEST entry live as well" ;;
  *) pass "archive --keep 1: the OLDEST entry is not still live" ;;
esac
case "$ARCH" in
  *"the OLDEST decision"*) pass "archive --keep 1: the OLDEST entry is the one archived" ;;
  *) fail "archive --keep 1 did not archive the oldest entry — archive is: $ARCH" ;;
esac
# Conservation, stated separately from order, because a correct total was the
# signal that missed the real incident.
case "$LIVE$ARCH" in
  *"the NEWEST decision"*) : ;; *) fail "rotation lost the newest entry entirely" ;;
esac
pass "archive --keep 1: both entries are still present across the two files"

# MUTANT M01: select from the FRONT of the chronological list — the shipped bug.
sed 's|keep = ordered\[len(ordered) - keep_n:\] if keep_n < len(ordered) else list(ordered)|keep = ordered[:keep_n]|; s|move = ordered\[:len(ordered) - keep_n\] if keep_n < len(ordered) else \[\]|move = ordered[keep_n:]|' \
  "$ROOT/bin/decisions" > "$(M m01)"
grep -q 'keep = ordered\[:keep_n\]' "$(M m01)" || fail "M01 did not apply — the selection line moved"
T="$(tree m01 "$(M m01)")"; two_entries "$T"
# The needle is the mutant's OWN plan line naming what it kept. The fixture's
# untouched text also says "the OLDEST decision", so reading the file back
# would certify a mutant that never ran.
saw_mutant "M01 --keep selects the leading entries in file order" \
  "$(dec "$T" archive setup --keep 1 --apply || true)" "keeping   2026-01-01 the OLDEST decision"

# An archive file is written newest-first, so its own position order is the
# reverse of the live file's. Two same-day untimed rows there carry their
# chronology in position alone.
T="$(tree arcorder)"
mkdir -p "$T/setup/decisions"
cat > "$T/setup/decisions/2026-Q3.md" <<'MD'
# setup — decisions, 2026-Q3 (archive)

---

## 2026-07-01 archived LATER on the day

Body.

## 2026-07-01 archived EARLIER on the day

Body.
MD
printf '# setup — decisions\n\n---\n\n## 2026-09-01 live\n\nBody.\n' > "$T/setup/decisions.md"
ROWS="$(dec "$T" index setup | grep '2026-07-01' | tr '\n' ' ' | tr -s ' ')"
case "$ROWS" in
  *"archived LATER on the day"*"archived EARLIER on the day"*)
    pass "index reads an archive file newest-first, so its same-day rows are not inverted" ;;
  *) fail "index inverted an archive file's same-day rows — got: $ROWS" ;;
esac

# MUTANT M02: drop the archive-file reversal, so an archive's newest-first
# serialization is read as if it were chronological.
sed 's|seq = list(reversed(entries)) if is_archive(path) else list(entries)|seq = list(entries)|' \
  "$ROOT/bin/decisions" > "$(M m02)"
grep -q 'seq = list(entries)$' "$(M m02)" || fail "M02 did not apply"
T="$(tree m02 "$(M m02)")"
mkdir -p "$T/setup/decisions"
# Same day, no clock, written newest-first as archives are. Chronology here is
# carried by position alone, so the reversal is the only thing that recovers it.
cat > "$T/setup/decisions/2026-Q3.md" <<'MD'
# setup — decisions, 2026-Q3 (archive)

---

## 2026-07-01 archived LATER on the day

Body.

## 2026-07-01 archived EARLIER on the day

Body.
MD
printf '# setup — decisions\n\n---\n\n## 2026-09-01 live\n\nBody.\n' > "$T/setup/decisions.md"
saw_mutant "M02 an archive file's newest-first order is read as chronological" \
  "$(dec "$T" index setup | tr '\n' ' ' | tr -s ' ')" \
  "archived EARLIER on the day setup/decisions/2026-Q3.md 2026-07-01 archived LATER"

# ── 2. a same-day clock outranks append position ────────────────────────────
# Two entries on one day, appended in the WRONG order — which is what happens
# when someone writes up the morning's ruling after the evening's. Append
# position and the clock disagree here, so this is the fixture where the clock
# has to be the thing that decides. Without one, ties fall back to position,
# which for an append-only file is still the best evidence available.
clock_fixture() {
  cat > "$1/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-08-08 20:30 evening ruling that supersedes it

Body.

## 2026-08-08 09:00 morning ruling, written up afterwards

Body.
MD
}
T="$(tree clock)"; clock_fixture "$T"
OUT="$(dec "$T" index setup)"
FIRST="$(printf '%s\n' "$OUT" | grep 'decisions.md ' | head -1)"
case "$FIRST" in
  *"evening ruling"*) pass "index: a same-day clock outranks append position" ;;
  *) fail "index ordered two same-day entries by append position — first row was: $FIRST" ;;
esac

# MUTANT M03: make the clock not reach the sort key — date only, ties left in
# append order, which is how the shipped version ordered them. The mutation is
# on the ORDERING use of the clock rather than on the regex: deleting the regex
# groups renumbers the ones after them, which crashes the parser instead of
# changing its order, and a crash is not this assertion's subject.
sed 's|^    hh = int(m.group(4)) if m.group(4) else 0$|    hh = 0|; s|^    mm = int(m.group(5)) if m.group(5) else 0$|    mm = 0|; s|^    ss = int(m.group(6)) if m.group(6) else 0$|    ss = 0|' \
  "$ROOT/bin/decisions" > "$(M m03)"
grep -q '^    hh = 0$' "$(M m03)" || fail "M03 did not apply — the clock fields of entry_key moved"
T="$(tree m03 "$(M m03)")"; clock_fixture "$T"
saw_mutant "M03 the clock does not reach the sort key, so same-day rows fall back to append position" \
  "$(dec "$T" index setup | grep 'decisions.md ' | head -1)" "morning ruling, written up afterwards"

# ── 3. an unrecognised heading is CARRIED, never dropped ───────────────────
# The shape that actually happened: undated headings BELOW a rotation pointer,
# which the sentinel skip also hid from `index`.
#
# Refusing to rotate on one was the first fix here and it was reverted the same
# day: 77 undated `## ` headings sit in the four projects' archive files, every
# project's merge target is the same quarter, so the refusal blocked ALL FOUR
# rotations while `hw status` went on telling brainers to rotate. What makes
# carrying safe is the staged conservation check, so that is what is asserted.
below_pointer() {
  cat > "$1/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 old entry

Body.

<!-- decisions:archive-pointer -->

## Older quarters are archived

Closed quarters live in `setup/decisions/`.

## Una entrada sin fecha en el heading

**Fecha:** 2026-08-08

CONTENT-THAT-MUST-NOT-VANISH

## 2026-08-09 a later dated entry

Body.
MD
}
T="$(tree third)"; below_pointer "$T"
OUT="$(dec "$T" archive setup --keep 1 --apply)" \
  || fail "archive refused a file carrying an undated heading instead of carrying it: $OUT"
grep -rq 'CONTENT-THAT-MUST-NOT-VANISH' "$T/setup" \
  || fail "rotation deleted the content under the undated heading — the measured incident"
pass "rotation carries the content under an unrecognised heading into the file its entry landed in"
grep -rq 'Una entrada sin fecha en el heading' "$T/setup" \
  || fail "rotation deleted the unrecognised heading itself"
pass "rotation carries the unrecognised heading itself, not just the text under it"
# Exactly one place. An entry duplicated across live and archive is as wrong as
# a lost one, and it is what an interrupted install looks like.
N="$(grep -rc 'CONTENT-THAT-MUST-NOT-VANISH' "$T/setup" 2>/dev/null | awk -F: '{n+=$2} END {print n}')"
[ "$N" = "1" ] || fail "the carried content is in $N places, not exactly 1"
pass "the carried content is in exactly one file"

# And it is reachable by the one command every CLAUDE.md points at.
T="$(tree third_index)"; below_pointer "$T"
OUT="$(dec "$T" index setup)"
case "$OUT" in
  *"UNREACHABLE by index"*"Una entrada sin fecha"*)
    pass "index names the heading it cannot list instead of showing a short list" ;;
  *) fail "index silently omitted the unrecognised heading — said: $OUT" ;;
esac
# `check` must fail on it too, or nothing surfaces it at all.
dec "$T" check >/dev/null 2>&1 && fail "check passed a file with an unrecognised heading"
pass "check fails while an unrecognised heading exists"

# MUTANT M04: let the pointer region swallow everything below it until the next
# DATED heading, as it shipped. The content is then this tool's own bookkeeping,
# so it is dropped at PARSE time — which is why the conservation check cannot
# see it and why the region boundary has to be the guard.
#
# The kill is the mutant's `check` EXIT CODE. The real binary fails on this
# fixture because it can see an unrecognised heading; the mutant cannot see one,
# so it exits 0. That is text only the mutant produces, and it cannot be printed
# by a mutant that failed to run.
sed 's|            if in_pointer and line.strip() == POINTER_HEADING:|            if in_pointer:|' \
  "$ROOT/bin/decisions" > "$(M m04)"
grep -q '            if in_pointer:$' "$(M m04)" || fail "M04 did not apply"
T="$(tree m04 "$(M m04)")"; below_pointer "$T"
OUT="$(dec "$T" check >/dev/null 2>&1; echo "check-exit=$?")"
saw_mutant "M04 the pointer region swallows any heading below it, so nothing sees it" \
  "$OUT" "check-exit=0"
# AND THE SECOND HALF OF THIS ARM WAS RESTATED ON 2026-09-08, because the fix
# for the pointer-region loss made its old claim FALSE — by making the tool
# safer, not by breaking it.
#
# It used to assert that M04's swallowing DELETED the content: "swallowing
# implies loss". That held while the region was dropped at parse time. The
# region is now CARRIED (see 103-text-below-the-archive-pointer-survives.sh),
# so swallowing no longer costs the content, and the two facts are separate:
#
#   M04 still causes a real defect  — the heading is invisible to `check` and
#                                     `index`, which is the kill above.
#   M04 no longer causes the loss   — the carry protects it now.
#
# Asserting the old sentence here would demand the loss back. So this asserts
# the guarantee that replaced it, which is the stronger claim.
dec "$T" archive setup --keep 1 --apply >/dev/null 2>&1 || true
grep -rq 'CONTENT-THAT-MUST-NOT-VANISH' "$T/setup" \
  && pass "M04's swallowing costs visibility but no longer costs the content — the carry survives even a mutated region boundary" \
  || fail "the swallowed content was DELETED: the pointer region is dropping text again, which is the 2026-09-08 loss"

# MUTANT M05: skip the staged conservation check. Paired with a write that
# loses the carried heading, so the check is what has to catch it.
sed 's|^    if problems:$|    if False:|' "$ROOT/bin/decisions" > "$(M m05)"
grep -q '^    if False:$' "$(M m05)" || fail "M05 did not apply"
T="$(tree m05 "$(M m05)")"; below_pointer "$T"
# With the check skipped AND the pointer swallowing, nothing stops the loss. The
# pointer half is the real binary's, so mutate only the check and drive the loss
# through a body-dropping render instead.
# python3, not sed: the live-file staging is now a two-line call inside
# stage_everything(), so a single-line sed pattern cannot reach it — and a
# mutation that silently fails to apply is a mutant that kills nothing.
python3 - "$ROOT/bin/decisions" "$(M m05)" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
check = "    if problems:\n"
assert check in s, "M05: the staged-check anchor is gone"
s = s.replace(check, "    if False:\n", 1)
render = "render(preamble, keep,"
assert render in s, "M05: the live-render anchor is gone"
s = s.replace(render, "render(preamble, [(k, h, []) for k, h, _b in keep],", 1)
open(dst, "w").write(s)
PYMUT
grep -q 'for k, h, _b in keep' "$(M m05)" || fail "M05 did not apply (body-dropping render)"
python3 -c "import ast,sys; ast.parse(open(sys.argv[1]).read())" "$(M m05)" \
  || fail "M05 produced invalid python, which kills nothing"
T="$(tree m05 "$(M m05)")"; below_pointer "$T"
# THE NEEDLE WAS VACUOUS AND JUDGMENT DAY CAUGHT IT (2026-09-08). It was
# "VERIFIED by re-reading", which bin/decisions prints on EVERY successful
# rotation — including the healthy run twenty lines above, on this same fixture.
# The arm passed with no mutation applied at all.
#
# What actually distinguishes this mutant is the INSTALLED FILE: with the staged
# check skipped, a body-dropping render reaches disk. So observe that, and
# observe it against the healthy binary on the same fixture rather than against
# the absence of anything — a body that is gone cannot be a positive needle, but
# a body that SURVIVED the healthy run and is MISSING after the mutant is.
# The mutation drops the bodies of the KEPT entries, so the needle is the kept
# entry's body in the LIVE file. Measured: that one line is the entire
# difference between the healthy and the mutated rotation of this fixture.
BODY_NEEDLE='Body.'
T_OK="$(tree m05ok "$ROOT/bin/decisions")"; below_pointer "$T_OK"
dec "$T_OK" archive setup --keep 1 --apply >/dev/null 2>&1 || true
ok_bodies="$(grep -c "^$BODY_NEEDLE$" "$T_OK/setup/decisions.md" 2>/dev/null || true)"
[ "$ok_bodies" -ge 1 ] \
  || fail "M05 the fixture does not carry the body needle through a HEALTHY rotation, so its absence after the mutant would prove nothing (found $ok_bodies)"
T="$(tree m05 "$(M m05)")"; below_pointer "$T"
dec "$T" archive setup --keep 1 --apply >/dev/null 2>&1 || true
mut_bodies="$(grep -c "^$BODY_NEEDLE$" "$T/setup/decisions.md" 2>/dev/null || true)"
if [ "$mut_bodies" -lt "$ok_bodies" ]; then
  pass "mutant killed: M05 a body-dropping write installs when the staged check is skipped (healthy kept $ok_bodies body line(s), mutant kept $mut_bodies)"
else
  fail "M05 did not die: the mutant kept $mut_bodies body line(s) and the healthy binary kept $ok_bodies, so skipping the staged check changed nothing observable"
fi

# MUTANT M06: keep the check, drop the bodies. The check must be what stops it,
# and it must stop it BEFORE anything is installed.
python3 - "$ROOT/bin/decisions" "$(M m06)" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
render = "render(preamble, keep,"
assert render in s, "M06: the live-render anchor is gone"
open(dst, "w").write(
    s.replace(render, "render(preamble, [(k, h, []) for k, h, _b in keep],", 1))
PYMUT
grep -q 'for k, h, _b in keep' "$(M m06)" || fail "M06 did not apply"
python3 -c "import ast,sys; ast.parse(open(sys.argv[1]).read())" "$(M m06)" \
  || fail "M06 produced invalid python, which kills nothing"
T="$(tree m06 "$(M m06)")"; below_pointer "$T"
BEFORE="$(cat "$T/setup/decisions.md")"
saw_mutant "M06 the staged check names what a losing write would drop" \
  "$(dec "$T" archive setup --keep 1 --apply || true)" "refusing to install"
[ "$(cat "$T/setup/decisions.md")" = "$BEFORE" ] \
  || fail "the staged check refused but the live file was rewritten anyway"
# The directory itself is created before staging; what must not exist is an
# installed archive FILE.
ARCHMD="$(ls "$T/setup/decisions"/*.md 2>/dev/null || true)"
[ -z "$ARCHMD" ] || fail "the staged check refused but an archive file was installed: $ARCHMD"
pass "a refused install writes nothing: both files are byte-untouched"
LEFT="$( (ls -A "$T/setup"; ls -A "$T/setup/decisions" 2>/dev/null) | grep '^\.decisions-' || true)"
[ -z "$LEFT" ] || fail "the refused install left staged temp files behind: $LEFT"
pass "a refused install leaves no staged temp files"

# ── 4. a dated heading inside a fenced block is example text ────────────────
T="$(tree fence)"
cat > "$T/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 real entry

The format is:

```markdown
## 2099-03-01 an EXAMPLE, not a decision
```

More body.
MD
OUT="$(dec "$T" archive setup --apply)" || fail "archive failed on a fenced example: $OUT"
[ -e "$T/setup/decisions/2099-Q1.md" ] \
  && fail "a fenced markdown example was rotated into an archive file: $(ls "$T/setup/decisions")"
pass "a dated heading inside a fenced block is not an entry"
case "$(dec "$T" index setup)" in
  *"an EXAMPLE, not a decision"*) fail "the fenced example is listed as a decision" ;;
  *) pass "index does not list a fenced example as a decision" ;;
esac

# MUTANT M11: no fence state — the shipped parser had none.
sed 's|^        if fence is None and H2_RE.match(line):$|        if H2_RE.match(line):|' \
  "$ROOT/bin/decisions" > "$(M m11)"
grep -q '^        if H2_RE.match(line):$' "$(M m11)" || fail "M11 did not apply"
T="$(tree m11 "$(M m11)")"
cat > "$T/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-01 real entry

```markdown
## 2099-03-01 an EXAMPLE, not a decision
```
MD
dec "$T" archive setup --keep 1 --apply >/dev/null 2>&1 || true
saw_mutant "M11 the parser has no fence state, so a markdown example becomes an entry" \
  "$(ls "$T/setup/decisions" 2>/dev/null || true)$(dec "$T" index setup)" \
  "2099-Q1.md" "an EXAMPLE, not a decision"

# ── 5. one oversized entry: a refusal, not "nothing to archive", exit 0 ─────
T="$(tree oversized)"
{ printf '# setup — decisions\n\n---\n\n## 2026-08-08 one enormous entry\n\n'
  for i in $(seq 1 1500); do echo "line $i of the single enormous entry"; done
} > "$T/setup/decisions.md"
BEFORE="$(wc -l < "$T/setup/decisions.md")"
OUT="$(dec "$T" archive setup --apply)" && \
  fail "archive exited 0 on a file it left over budget: $OUT"
case "$OUT" in
  *"newest entry alone renders"*"1200-line budget"*)
    pass "archive refuses when no retention it can write fits the budget, and says the numbers" ;;
  *) fail "the over-budget refusal does not name the measured line counts — said: $OUT" ;;
esac
[ "$(wc -l < "$T/setup/decisions.md")" = "$BEFORE" ] \
  || fail "the refusal still rewrote the live file"
pass "the over-budget refusal wrote nothing"

# MUTANT M07: report the old success line instead of refusing.
sed 's|^        if current_lines > WARN_LINES:$|        if False:|' "$ROOT/bin/decisions" > "$(M m07)"
grep -q '^        if False:$' "$(M m07)" || fail "M07 did not apply"
T="$(tree m07 "$(M m07)")"
{ printf '# setup — decisions\n\n---\n\n## 2026-08-08 a\n\nBody.\n\n## 2026-08-09 b\n\n'
  for i in $(seq 1 1500); do echo "line $i"; done
} > "$T/setup/decisions.md"
# --keep 2 leaves nothing to move on a file that is over budget: the old code
# printed the kept-range line and exited 0 there.
saw_mutant "M07 an over-budget file with nothing movable reports success" \
  "$(dec "$T" archive setup --keep 2 --apply || true)" "nothing to archive: all 2 entries"

# ── 6. the result is re-read, and the budget is the file's real line count ──
T="$(tree budget)"
{ printf '# setup — decisions\n\n---\n\n'
  for e in $(seq 1 60); do
    printf '## 2026-%02d-%02d entry %d\n\n' $(( (e % 12) + 1 )) $(( (e % 28) + 1 )) "$e"
    for i in $(seq 1 40); do echo "body line $i of entry $e"; done
    echo
  done
} > "$T/setup/decisions.md"
OUT="$(dec "$T" archive setup --apply)" || fail "archive failed on a 60-entry file: $OUT"
case "$OUT" in
  *"VERIFIED by re-reading"*"accounted for exactly once"*)
    pass "archive reports from a re-read of the installed files, not from its plan" ;;
  *) fail "archive did not re-read what it wrote — said: $OUT" ;;
esac
AFTER="$(wc -l < "$T/setup/decisions.md" | tr -d " ")"
[ "$AFTER" -le 1200 ] || fail "archive left the live file at $AFTER lines, over the 1200 budget"
pass "archive leaves the live file inside the line budget ($AFTER of 1200 lines)"
dec "$T" check >/dev/null 2>&1 || fail "check still flags the rotated file"
pass "check and archive now agree about the rotated file"

# A dry run is a plan and says so.
T="$(tree dryrun)"; two_entries "$T"
OUT="$(dec "$T" archive setup --keep 1)" || fail "dry run failed: $OUT"
case "$OUT" in
  *"PLAN, not a result"*) pass "a dry run calls itself a plan" ;;
  *) fail "the dry run does not distinguish itself from a result — said: $OUT" ;;
esac
[ -e "$T/setup/decisions" ] && fail "the dry run created an archive directory"
grep -q 'the OLDEST decision' "$T/setup/decisions.md" || fail "the dry run rewrote the live file"
pass "a dry run writes nothing"

# ── 7. an archive file's own preamble is not regenerated over ───────────────
T="$(tree arcpre)"
mkdir -p "$T/setup/decisions"
cat > "$T/setup/decisions/2026-Q3.md" <<'MD'
# setup — decisions, 2026-Q3 (archive)

HAND-WRITTEN-ARCHIVE-NOTE that a human added here.

---

## 2026-07-01 an already archived entry

Body.
MD
printf '# setup — decisions\n\n---\n\n## 2026-08-01 a\n\nBody a.\n\n## 2026-09-01 b\n\nBody b.\n' \
  > "$T/setup/decisions.md"
OUT="$(dec "$T" archive setup --keep 1 --apply)" || fail "archive into an existing quarter failed: $OUT"
grep -q 'HAND-WRITTEN-ARCHIVE-NOTE' "$T/setup/decisions/2026-Q3.md" \
  || fail "the archive file's own preamble was regenerated over: $(cat "$T/setup/decisions/2026-Q3.md")"
pass "merging into an existing archive keeps that file's preamble verbatim"
grep -q 'an already archived entry' "$T/setup/decisions/2026-Q3.md" \
  || fail "merging into an existing archive dropped the entry that was already in it"
pass "merging into an existing archive keeps the entries already in it"

# ── 8. an archive file this rotation does not open ──────────────────────────
# Measured 2026-09-08 on the real files: 77 `## ` headings across the four
# projects' archive files carry no date — mostly section headings inside long
# entries. A rotation must not be blocked by them, must not rewrite the quarter
# files it is not merging into, and must still count those files when it asks
# whether every entry ended up in exactly one place.
scoped() {
  local t="$1"
  mkdir -p "$t/setup/decisions"
  cat > "$t/setup/decisions/2026-Q1.md" <<'MD'
# setup — decisions, 2026-Q1 (archive)

---

## 2026-02-01 an archived entry

Body.

## A section heading inside that entry, undated

Body.
MD
  printf '# setup — decisions\n\n---\n\n## 2026-07-01 a\n\nBody a.\n\n## 2026-09-01 b\n\nBody b.\n' \
    > "$t/setup/decisions.md"
}
T="$(tree scoped_ok)"; scoped "$T"
Q1="$(cat "$T/setup/decisions/2026-Q1.md")"
# The move lands in 2026-Q3, so 2026-Q1 is never opened.
OUT="$(dec "$T" archive setup --keep 1 --apply)" \
  || fail "a rotation into 2026-Q3 was blocked by an undated heading in 2026-Q1: $OUT"
pass "archive is not blocked by an unrecognised heading in a quarter it does not rewrite"
[ "$(cat "$T/setup/decisions/2026-Q1.md")" = "$Q1" ] \
  || fail "the untouched quarter file was rewritten: $(cat "$T/setup/decisions/2026-Q1.md")"
pass "the quarter file it does not rewrite is left byte-identical"

# MUTANT M10: leave the untouched archives out of the conservation scope, paired
# with a write that duplicates an entry into the live file. An entry in two
# places is what an interrupted install looks like, and the count is only able
# to see it if every file is in the comparison.
python3 - "$ROOT/bin/decisions" "$(M m10)" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
scope = "    untouched = [a for a in archive_files(project) if a not in targets.values()]"
assert scope in s, "M10: the staged-scope anchor is gone"
s = s.replace(scope, "    untouched = []", 1)
render = "render(preamble, keep,"
assert render in s, "M10: the live-render anchor is gone"
s = s.replace(
    render,
    "render(preamble, keep + [e for e in split(archive_files(project)[-1])[1][:1]],",
    1)
open(dst, "w").write(s)
PYMUT
grep -q '^    untouched = \[\]$' "$(M m10)" || fail "M10 did not apply (scope)"
grep -q 'keep + \[e for e in split' "$(M m10)" || fail "M10 did not apply (duplicating write)"
T="$(tree m10 "$(M m10)")"; scoped "$T"
# The mutant still gets caught — by the POST-install re-read, which reads every
# archive back off disk. That is the difference the arm is about: with the
# untouched file outside the STAGED scope, the files are written first and the
# problem is reported afterwards. Only the mutant reaches that text.
OUT="$(dec "$T" archive setup --keep 1 --apply || true)"
# M10'S KILL MOVED EARLIER ON 2026-09-08, and that is the guard improving
# rather than the test weakening. It used to be caught by the POST-install
# re-read, after the files were written. The line-conservation check added with
# the pointer-region fix runs on the STAGED files and knows nothing about entry
# scope, so a narrowed scope now shows up as a line it can no longer see —
# before anything is installed.
#
# What the arm asserts is therefore: the narrowed scope is still caught, it is
# caught PRE-install, and nothing was written. Asserting the old late catch
# would be asking for the install back.
saw_mutant "M10 a narrowed staged scope is caught before installing, not after" \
  "$OUT" "refusing to install"
case "$OUT" in
  *"a line would be LOST"*) pass "M10 is named by the classifier-independent line check, which sees the archive the narrowed scope dropped" ;;
  *) fail "M10 refused but not through the line check, so the guard is not the one this arm describes: $OUT" ;;
esac
case "$OUT" in
  *"wrote setup/decisions.md"*) fail "M10 installed the files despite refusing: $OUT" ;;
  *) pass "M10 wrote nothing, so the late-catch window this arm used to assert is closed" ;;
esac

# With the untouched archive back in scope, the same duplicating write is caught.
python3 - "$ROOT/bin/decisions" "$(M m12)" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
render = "render(preamble, keep,"
assert render in s, "M12: the live-render anchor is gone"
open(dst, "w").write(s.replace(
    render,
    "render(preamble, keep + [e for e in split(archive_files(project)[-1])[1][:1]],",
    1))
PYMUT
grep -q 'keep + \[e for e in split' "$(M m12)" || fail "M12 did not apply"
T="$(tree m12 "$(M m12)")"; scoped "$T"
# Unmutated scope, same duplicating write: caught BEFORE anything is installed.
T="$(tree m12 "$(M m12)")"; scoped "$T"
LIVE_BEFORE="$(cat "$T/setup/decisions.md")"
OUT="$(dec "$T" archive setup --keep 1 --apply || true)"
saw_mutant "M12 the staged count spans the untouched archives and names the duplicated entry" \
  "$OUT" "refusing to install"
case "$OUT" in
  *"would be in 2"*) pass "M12's refusal names the entry that would be in two places" ;;
  *) fail "M12's refusal does not name the duplicated entry: $OUT" ;;
esac
[ "$(cat "$T/setup/decisions.md")" = "$LIVE_BEFORE" ] \
  || fail "M12 refused but the live file was written anyway"
pass "M12: the pre-install refusal left the live file byte-identical"

# `check` names them wherever they are — otherwise 77 of them stay invisible.
T="$(tree scoped_check)"; scoped "$T"
OUT="$(dec "$T" check || true)"
case "$OUT" in
  *"2026-Q1.md:9"*"A section heading inside that entry, undated"*)
    pass "check names an unrecognised heading inside an archive file, with its line" ;;
  *) fail "check does not report unrecognised headings in archive files — said: $OUT" ;;
esac
case "$OUT" in
  *'demote'*'### '*) pass "check says how to fix it, including the section-heading case" ;;
  *) fail "check reports the heading without saying what to do about it" ;;
esac

# ── the three defects Judgment Day found IN THIS FIX, 2026-09-08 ────────────
#
# All three are the same lesson twice over: this file was written to stop a tool
# from silently reordering and mislabelling, and the fix shipped with a
# reordering bug, a mislabelled exit code, and a crash where a refusal belonged.

# 1. TIES. Two entries sharing a timestamp — the common case, since every
#    undated entry collapses to 00:00 — must come back out of the archive in
#    the order they went in. `sorted(reverse=True)` is stable, so it kept ties
#    in INPUT order (oldest-first) inside a NEWEST-first file, and reading it
#    back inverted them. This is the bug the whole file exists to refuse.
tie_tree() {
  cat > "$1/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-03-01 TIE-FIRST-IN

Body.

## 2026-03-01 TIE-SECOND-IN

Body.

## 2026-08-09 the entry that stays

Body.
MD
}
T="$(tree ties)"; tie_tree "$T"
OUT="$(dec "$T" archive setup --keep 1 --apply)" || fail "the tie fixture would not rotate: $OUT"
arch="$(ls "$T"/setup/decisions/*.md 2>/dev/null | head -1)"
[ -n "$arch" ] || fail "the tie fixture produced no archive file"
# The archive file is newest-first, so within a tie the LAST one in must appear
# FIRST in the file. Read the two positions rather than trusting the sort.
first_line="$(grep -n 'TIE-FIRST-IN' "$arch" | cut -d: -f1)"
second_line="$(grep -n 'TIE-SECOND-IN' "$arch" | cut -d: -f1)"
[ -n "$first_line" ] && [ -n "$second_line" ] \
  || fail "a tied entry did not survive the rotation (first=$first_line second=$second_line)"
if [ "$second_line" -lt "$first_line" ]; then
  pass "ties are written newest-first like the rest of the archive file (SECOND-IN at $second_line precedes FIRST-IN at $first_line)"
else
  fail "ties were written oldest-first into a newest-first file, so index reads them backwards: FIRST-IN at $first_line, SECOND-IN at $second_line"
fi
# And the round trip, which is the property that actually matters.
IDX="$(dec "$T" index setup --all)"
i_second="$(printf '%s\n' "$IDX" | grep -n 'TIE-SECOND-IN' | cut -d: -f1)"
i_first="$(printf '%s\n' "$IDX" | grep -n 'TIE-FIRST-IN' | cut -d: -f1)"
if [ -n "$i_second" ] && [ -n "$i_first" ] && [ "$i_second" -lt "$i_first" ]; then
  pass "index reads the tie back newest-first, so the round trip preserves the order it was appended in"
else
  fail "index reads the tie back in the wrong order (SECOND-IN at $i_second, FIRST-IN at $i_first)"
fi
# M13 — the reverse=True form. Dies on the two arms above.
sed 's|merged = list(reversed(sorted(existing + buckets\[q\], key=lambda e: e\[0\])))|merged = sorted(buckets[q] + existing, key=lambda e: e[0], reverse=True)|' \
  "$ROOT/bin/decisions" > "$(M m13)"
grep -q 'reverse=True' "$(M m13)" || fail "M13 did not apply"
T="$(tree m13 "$(M m13)")"; tie_tree "$T"
dec "$T" archive setup --keep 1 --apply >/dev/null 2>&1 || true
m_arch="$(ls "$T"/setup/decisions/*.md 2>/dev/null | head -1)"
m_first="$(grep -n 'TIE-FIRST-IN' "$m_arch" 2>/dev/null | cut -d: -f1)"
m_second="$(grep -n 'TIE-SECOND-IN' "$m_arch" 2>/dev/null | cut -d: -f1)"
if [ -n "$m_first" ] && [ -n "$m_second" ] && [ "$m_first" -lt "$m_second" ]; then
  pass "mutant killed: M13 a stable reverse sort keeps ties oldest-first in a newest-first file (FIRST-IN at $m_first precedes SECOND-IN at $m_second)"
else
  fail "M13 did not die: the tie landed at first=$m_first second=$m_second, which is the fixed order"
fi

# 2. THE EXIT CODE IS THE LABEL. `hw status`'s only consumer prints one
#    sentence per nonzero exit, so "over the budget" and "headings I cannot
#    read" cannot share a code — a file inside the budget was announced as over
#    it on every single hw status.
small_broken() {
  cat > "$1/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-08-09 a dated entry

Body.

## an undated heading, and the file is small

Body.
MD
}
T="$(tree exitcodes)"; small_broken "$T"
# `|| rc=$?`, never a bare assignment: check now exits 2 here, and under
# `set -e` a failing assignment kills the file with no `not ok` at all. That
# is how this section vanished silently on its first run.
rc=0
OUT="$(dec "$T" check)" || rc=$?
case "$OUT" in
  *'UNRECOGNISED HEADING'*) : ;;
  *) fail "check did not report the unrecognised heading at all: $OUT" ;;
esac
case "$OUT" in
  *OVER*) fail "the fixture is over the seek budget, so it cannot test the inside-budget case: $OUT" ;;
esac
[ "$rc" = 2 ] \
  && pass "check exits 2 for a file INSIDE the budget with headings it cannot read — not 1, which means over budget" \
  || fail "check exited $rc for an inside-budget file with a bad heading; hw would report it as past the seek budget"
# And hw must have words for 2 that are not the budget's words.
grep -q 'headings the tool cannot read' "$ROOT/bin/hw" \
  || fail "hw has no separate sentence for exit 2, so it still labels a heading problem as a budget problem"
pass "hw distinguishes the two, so the warning names the remedy it actually has"
# M14 — the two answers share code 1 again. Dies on the exit-code arm.
sed 's|^    if over:$|    if over or broken:|' "$ROOT/bin/decisions" > "$(M m14)"
grep -q '^    if over or broken:$' "$(M m14)" || fail "M14 did not apply"
T="$(tree m14 "$(M m14)")"; small_broken "$T"
m_rc=0
dec "$T" check >/dev/null 2>&1 || m_rc=$?
[ "$m_rc" = 1 ] \
  && pass "mutant killed: M14 an inside-budget heading problem exits 1, the code hw reads as past the seek budget (saw rc=$m_rc)" \
  || fail "M14 did not die: it exited $m_rc"

# 3. `--before` had no floor, so a cutoff later than every entry crashed on
#    keep[0] instead of refusing. --keep has had that floor all along; the two
#    cutoffs now enforce the same invariant.
T="$(tree beforefloor)"; below_pointer "$T"
OUT="$(dec "$T" archive setup --before 2099-01-01 2>&1 || true)"
case "$OUT" in
  *Traceback*) fail "--before later than every entry still dies with a traceback: $OUT" ;;
esac
case "$OUT" in
  *'an empty live file is not a rotation'*) pass "--before refuses an empty live file in this file's own words, like --keep" ;;
  *) fail "--before neither refused nor crashed, which means it wrote something: $OUT" ;;
esac
