#!/usr/bin/env bash
# A rotation that fails partway leaves no staged temp files, and a temp left by
# a KILLED rotation is named by a command.
#
# WHY. `stage()` cleans up only its OWN temp, so a multi-quarter rotation that
# failed while staging the second file left the first one beside the archives.
# Measured against the binary at 4514544 on 2026-09-08, by injecting a failure
# into the second stage() call:
#
#     leftover:  setup/decisions/.decisions-u5r5llb_.tmp
#     `decisions check`:  NO COMMAND NAMES THEM
#
# and the operator got a raw Python traceback where every other refusal in that
# file is written in its own words.
#
# THE INVISIBILITY IS THE INTERESTING HALF, and it is one line of cause:
# `archive_files()` filters on `.md`, so a `.decisions-*.tmp` is not merely
# unreported — it is unreachable by every command that walks a project's files.
#
# TWO FIXES, TWO REASONS, and they are not redundant. Cleanup covers a rotation
# that FAILS. It cannot cover one that is KILLED — SIGKILL, a full disk, the
# machine going down — because nothing runs afterwards. So something has to be
# able to name what a dead process left, and that is a separate arm below.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

tree() {   # tree <name> [<binary>]
  local name="$1" bin="${2:-$ROOT/bin/decisions}"
  rm -rf "$TMP/$name"
  mkdir -p "$TMP/$name/bin" "$TMP/$name/setup"
  cp "$bin" "$TMP/$name/bin/decisions"
  chmod +x "$TMP/$name/bin/decisions"
  printf '%s\n' "$TMP/$name"
}
dec() { local t="$1"; shift; (cd "$t" && python3 bin/decisions "$@" 2>&1) ; }
M() { mkdir -p "$TMP/mut"; printf '%s\n' "$TMP/mut/$1"; }
temps() {  # temps <root> -> how many staged temps are lying about
  { find "$1/setup" -name '.decisions-*' 2>/dev/null || true; } | wc -l | tr -d ' '
}
# THREE quarters, so the rotation stages more than one archive file. With one
# quarter there is nothing for a mid-loop failure to leave behind, and every arm
# here would pass on a tool that cleans up nothing.
multi_quarter() {
  cat > "$1/setup/decisions.md" <<'MD'
# setup — decisions

---

## 2026-01-15 first quarter entry

Body.

## 2026-05-20 second quarter entry

Body.

## 2026-09-01 third quarter entry

Body.

## 2026-09-08 the entry that stays

Body.
MD
}
# The injection: make the Nth stage() call raise. Deterministic, and it fails
# INSIDE the staging loop rather than at its edges, which is where the leak was.
inject_stage_failure() { # inject_stage_failure <which-call> <out>
  python3 - "$ROOT/bin/decisions" "$1" "$2" <<'PYMUT'
import sys
src, which, dst = sys.argv[1], int(sys.argv[2]), sys.argv[3]
s = open(src).read()
old = "def stage(path, text):"
assert old in s, "stage anchor missing"
new = ("_STAGE_CALLS = [0]\n\n\ndef stage(path, text):\n"
       "    _STAGE_CALLS[0] += 1\n"
       "    if _STAGE_CALLS[0] == %d:\n"
       "        raise IOError('INJECTED: stage call %d fails')\n" % (which, which))
open(dst, "w").write(s.replace(old, new, 1))
PYMUT
}

# ── 1. the fixture must actually stage more than one file ──────────────────
T="$(tree plan)"; multi_quarter "$T"
PLAN="$(dec "$T" archive setup --keep 1)"
q_count="$(printf '%s\n' "$PLAN" | grep -c 'decisions/2026-Q' || true)"
[ "$q_count" -ge 2 ] \
  && pass "the fixture rotates into $q_count quarter files, so a mid-loop failure has something to leave behind" \
  || fail "the fixture only touches $q_count quarter file(s) — a leak arm over it would prove nothing: $PLAN"

# ── 2. a failure inside the staging loop leaves NOTHING ───────────────────
inject_stage_failure 2 "$(M fail2)" || fail "could not build the injected binary"
python3 -c "import ast,sys; ast.parse(open(sys.argv[1]).read())" "$(M fail2)" \
  || fail "the injected binary is not valid python, so it tests nothing"
T2="$(tree failmid "$(M fail2)")"; multi_quarter "$T2"
OUT="$(dec "$T2" archive setup --keep 1 --apply || true)"
left="$(temps "$T2")"
[ "$left" = 0 ] \
  && pass "a rotation that fails while staging leaves 0 staged temps behind" \
  || fail "the failed rotation left $left staged temp(s): $(find "$T2/setup" -name '.decisions-*')"
# It must also refuse in this file's own words, not with a stack trace.
case "$OUT" in
  *Traceback*) fail "the failed rotation printed a Python traceback at the operator: $OUT" ;;
esac
case "$OUT" in
  *"could not stage the rotation"*"no temporary files were left behind"*)
    pass "and it refuses in this file's own words, saying nothing was written and nothing left" ;;
  *) fail "the failed rotation neither refused clearly nor traced: $OUT" ;;
esac
# And it wrote nothing: the live file still holds every entry it started with.
[ "$(grep -c 'first quarter entry' "$T2/setup/decisions.md" || true)" = 1 ] \
  && pass "the failed rotation installed nothing — the live file is as it was" \
  || fail "the failed rotation damaged the live file"

# ── 3. failing on the LAST stage (the live file) is the same promise ───────
# The live file is staged after the loop, so a failure there exercises a
# different path with more temps already in hand.
inject_stage_failure 4 "$(M fail4)" || fail "could not build the second injected binary"
T3="$(tree faillive "$(M fail4)")"; multi_quarter "$T3"
OUT="$(dec "$T3" archive setup --keep 1 --apply || true)"
left="$(temps "$T3")"
[ "$left" = 0 ] \
  && pass "a failure staging the LIVE file also leaves 0 temps, with three already staged" \
  || fail "failing on the live file left $left staged temp(s)"

# ── 4. THE KILLED CASE, which cleanup can never cover ─────────────────────
# Nothing runs after SIGKILL, so the only possible answer is that a command can
# NAME what was left. Without this arm the fix is a promise about the one
# failure mode that is easy to handle.
T4="$(tree killed)"; multi_quarter "$T4"
mkdir -p "$T4/setup/decisions"
printf 'half a rendered file\n' > "$T4/setup/decisions/.decisions-orphan.tmp"
CHK="$(dec "$T4" check || true)"
case "$CHK" in
  *".decisions-orphan.tmp"*)
    pass "check NAMES a staged temp left by a killed rotation, with its path" ;;
  *) fail "check does not name a stray staged temp — it is still invisible: $CHK" ;;
esac
case "$CHK" in
  *"KILLED"*) pass "and says why it is there, so nobody reads it as a rotation still running" ;;
  *) fail "check reports the file without saying what it is" ;;
esac
# Its own exit code, for the same reason 1 and 2 were split: hw prints one
# sentence per code, so a code meaning two things names the wrong remedy.
rc=0
dec "$T4" check >/dev/null 2>&1 || rc=$?
[ "$rc" = 3 ] \
  && pass "a stray temp is exit 3 — not 1 (over budget) and not 2 (bad headings)" \
  || fail "a stray temp exited $rc, which hw would label as some other problem"
grep -q 'killed rotation' "$ROOT/bin/hw" \
  || fail "hw has no sentence for exit 3, so it would label a stray temp as a budget or heading problem"
pass "hw has its own sentence for exit 3"
# A clean tree is NOT exit 3, or the code says nothing.
T5="$(tree clean)"
printf '# setup — decisions\n\n---\n\n## 2026-09-08 one entry\n\nBody.\n' > "$T5/setup/decisions.md"
rc=0
dec "$T5" check >/dev/null 2>&1 || rc=$?
[ "$rc" = 0 ] \
  && pass "and a tree with no strays, no bad headings and no budget problem is exit 0" \
  || fail "a clean tree exited $rc"

# ── the mutant: the pre-fix staging, with no cleanup boundary ──────────────
# "It dies with the binary at 4514544", in the form this suite can run: the
# snapshot runner excludes .git, so a test cannot `git show` the old file.
# Removing the cleanup call is exactly what that binary did.
python3 - "$(M fail2)" "$(M m01)" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = """    except BaseException as exc:
        drop_staged()"""
assert old in s, "M01 anchor missing"
new = """    except BaseException as exc:
        pass"""
open(dst, "w").write(s.replace(old, new, 1))
PYMUT
python3 -c "import ast,sys; ast.parse(open(sys.argv[1]).read())" "$(M m01)" \
  || fail "M01 produced invalid python, which kills nothing"
T6="$(tree m01 "$(M m01)")"; multi_quarter "$T6"
dec "$T6" archive setup --keep 1 --apply >/dev/null 2>&1 || true
m_left="$(temps "$T6")"
if [ "$m_left" -ge 1 ]; then
  pass "mutant killed: M01 without the cleanup boundary a failed rotation leaves $m_left staged temp(s) (the fixed binary left 0)"
else
  fail "M01 did not die: it left $m_left temps, same as the fix"
fi
# M02 — the strays are found but reported under an existing code, so hw names
# the wrong remedy. Dies on the exit-code arm.
python3 - "$ROOT/bin/decisions" "$(M m02)" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = "    return 3 if strays else 0"
assert old in s, "M02 anchor missing"
open(dst, "w").write(s.replace(old, "    return 2 if strays else 0", 1))
PYMUT
T7="$(tree m02 "$(M m02)")"; multi_quarter "$T7"
mkdir -p "$T7/setup/decisions"
printf 'half a rendered file\n' > "$T7/setup/decisions/.decisions-orphan.tmp"
rc=0
dec "$T7" check >/dev/null 2>&1 || rc=$?
[ "$rc" = 2 ] \
  && pass "mutant killed: M02 a stray temp reports as exit 2, the code hw labels as unreadable headings (saw rc=$rc)" \
  || fail "M02 did not die: it exited $rc"
# M03 — the finder keeps archive_files's `.md` filter, which is the one line
# that made these invisible in the first place. Dies on arm 4.
python3 - "$ROOT/bin/decisions" "$(M m03)" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = '''        if f.startswith(".decisions-") and f.endswith(".tmp")'''
assert old in s, "M03 anchor missing"
open(dst, "w").write(s.replace(old, '''        if f.endswith(".md")''', 1))
PYMUT
T8="$(tree m03 "$(M m03)")"; multi_quarter "$T8"
mkdir -p "$T8/setup/decisions"
printf 'half a rendered file\n' > "$T8/setup/decisions/.decisions-orphan.tmp"
rc=0
dec "$T8" check >/dev/null 2>&1 || rc=$?
CHK="$(dec "$T8" check || true)"
case "$CHK" in
  *".decisions-orphan.tmp"*) fail "M03 still named the stray, so arm 4 does not rest on the filter" ;;
esac
[ "$rc" != 3 ] \
  && pass "mutant killed: M03 keeping the .md filter makes the stray invisible again (exit $rc, not 3)" \
  || fail "M03 did not die: it exited 3"
rm -rf "$TMP/mut"
