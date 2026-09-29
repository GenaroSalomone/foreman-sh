#!/usr/bin/env bash
# `bin/lint-shell` printed a clean verdict for files it never opened.
#
# The loop is `[ -f "$f" ] || continue` and the summary counted `$#` — the ARGV
# count, not the files read. Measured 2026-09-07:
#
#     $ lint-shell nope1.sh nope2.sh
#     clean: no silent-exit shapes in 2 file(s)        exit 0
#
# having opened nothing. A tool whose whole subject is "ways for this script to
# die without saying anything" died without saying anything, on the number it
# prints as its verdict.
#
# ITS OWN HEADER ALREADY ARGUED THE POINT and stopped one level short: the
# ZERO-argument case was hardened for exactly this reason ("a glob that matches
# nothing must fail loudly, not pass"), and the same hole one level down was
# left. That is the recurring shape in this codebase — the rule applied to the
# call site that bit, and not to its sibling.
#
# THREE VALUES:
#     0   every argument was read, and none was flagged
#     1   something was flagged
#    65   at least one argument could NOT be read. Not 1, because nothing was
#         flagged; emphatically not 0. "I checked these and they are clean" and
#         "I could not open these" are different claims.
#
# Run alone while working on this subject:
#     bash setup/tests/92-a-clean-bill-covers-only-what-was-read.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LS="$ROOT/bin/lint-shell"
[ -x "$LS" ] || fail "clean-bill: bin/lint-shell is missing or not executable"

# A file with no hazards, and one with a known hazard, so "clean" and "flagged"
# are both real rather than assumed.
CLEAN="$TMP/clean.sh"
printf '#!/usr/bin/env bash\nset -euo pipefail\nx="$(printf hi || true)"\nprintf %%s "$x"\n' > "$CLEAN"
DIRTY="$TMP/dirty.sh"
printf '#!/usr/bin/env bash\nset -euo pipefail\nx="$(grep nope /dev/null)"\nprintf %%s "$x"\n' > "$DIRTY"

lint() { # <args...> → "exit=<rc>|<stdout>|<stderr>"
  local out err rc
  out="$("$LS" "$@" 2>"$TMP/e")" && rc=0 || rc=$?
  err="$(cat "$TMP/e")"
  printf 'exit=%s|%s|%s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')" "$(printf '%s' "$err" | tr '\n' ' ')"
}

# ── 1. THE BASELINE, so the rest means something ───────────────────────────
r="$(lint "$CLEAN")"
case "$r" in
  exit=0*clean:\ no\ silent-exit\ shapes\ in\ 1\ file\(s\)*) pass "one readable clean file is clean, and the count is 1" ;;
  *) fail "clean-bill: a single clean file gave: $r" ;;
esac

r="$(lint "$DIRTY")"
case "$r" in
  exit=1*) pass "a file with a real hazard is still flagged, exit 1" ;;
  *) fail "clean-bill: the hazard fixture is not flagged, so the flagged/unread distinction below proves nothing: $r" ;;
esac

# ── 2. AN UNREADABLE ARGUMENT IS NOT A CLEAN FILE ──────────────────────────
r="$(lint "$TMP/does-not-exist-1.sh" "$TMP/does-not-exist-2.sh")"
case "$r" in
  exit=0*) fail "clean-bill: two files that do not exist produced EXIT 0. That is a green light for having read zero bytes — the linter's own subject, in the linter: $r" ;;
  exit=65*) : ;;
  *) fail "clean-bill: two missing files gave neither 0 nor 65: $r" ;;
esac
case "$r" in
  *could\ NOT\ be\ read*) : ;;
  *) fail "clean-bill: the refusal does not say the arguments could not be read: $r" ;;
esac
# AND IT MUST NAME THEM. "something was unreadable" sends the reader to guess
# which, and a glob that silently matched one wrong path is the case this is for.
case "$r" in
  *does-not-exist-1.sh*does-not-exist-2.sh*) pass "unreadable arguments are exit 65 and each one is named" ;;
  *) fail "clean-bill: the refusal does not name WHICH arguments were unread: $r" ;;
esac

# The word `clean` must not appear on stdout at all in that case: a caller that
# greps stdout for it — and `76-close-guard-holes.sh` does exactly that against
# bin/* — would read the refusal as a pass.
case "$r" in
  exit=65\|\|*) pass "an unread argument prints NOTHING on stdout, so a stdout grep cannot read it as a pass" ;;
  *) fail "clean-bill: stdout was not empty for the unreadable case, so a caller grepping stdout can still see a pass: $r" ;;
esac

# ── 3. A PARTIAL READ IS NOT A CLEAN BILL EITHER ───────────────────────────
#
# The dangerous shape: most of the glob matched, one path did not, and the
# summary covered all of them.
r="$(lint "$CLEAN" "$TMP/vanished.sh")"
case "$r" in
  exit=65*) : ;;
  *) fail "clean-bill: one readable clean file plus one missing file gave: $r. The clean half does not extend over the half that was never opened" ;;
esac
case "$r" in
  *vanished.sh*1\ file\(s\)\ read\ and\ clean*) pass "a partial read reports both halves and still exits 65" ;;
  *) fail "clean-bill: the partial-read message does not report both halves: $r" ;;
esac

# ── 4. THE COUNT IS WHAT WAS READ, NEVER \$# ───────────────────────────────
#
# The specific defect. Three arguments, one of them a directory, one missing:
# only one file is actually opened, and the number must say one.
#
# A NOTE ON WHAT THIS CAN AND CANNOT KILL, so nobody chases it later. Swapping
# `$read_count` back to `$#` on the CLEAN line alone is now an EQUIVALENT
# mutant: an unread argument routes to the exit-65 branch above and never
# reaches that line, so on the clean path `$# == $read_count` by construction.
# The defect was closed by the ROUTING, and section 3 is what pins that. The
# operand still says `$read_count` because it is the honest one and stays
# correct if a future skip is ever added — but the test that matters is the one
# below, which reads the count out of the exit-65 message.
mkdir -p "$TMP/adir"
r="$(lint "$CLEAN" "$TMP/adir" "$TMP/gone.sh")"
case "$r" in
  *3\ file\(s\)*) fail "clean-bill: the summary still counts \$# (3) rather than the files read (1): $r" ;;
esac
case "$r" in
  *1\ file\(s\)\ read*) pass "the count is the number of files read, not the number of arguments given" ;;
  *) fail "clean-bill: the summary does not report the read count: $r" ;;
esac

# A directory is an argument that was not read, like any other.
case "$r" in
  *adir*) pass "a directory argument is reported as unread rather than silently skipped" ;;
  *) fail "clean-bill: a directory argument was skipped without being named: $r" ;;
esac

# ── 5. FLAGGED STILL OUTRANKS UNREAD ───────────────────────────────────────
#
# When both are true the exit code has to be 1: a finding is actionable now,
# and collapsing it into 65 would bury it.
r="$(lint "$DIRTY" "$TMP/missing-too.sh")"
case "$r" in
  exit=1*) : ;;
  *) fail "clean-bill: a flagged file plus an unread argument must exit 1 — a real finding must not be downgraded to 'could not read': $r" ;;
esac
case "$r" in
  *could\ NOT\ be\ read*missing-too.sh*) pass "with both a finding and an unread argument, the exit is 1 and the unread one is still named" ;;
  *) fail "clean-bill: the unread argument stopped being reported once something was flagged: $r" ;;
esac

# ── 6. THE ZERO-ARGUMENT CASE ITS HEADER ALREADY HARDENED ──────────────────
r="$(lint)"
case "$r" in
  exit=64*nothing\ was\ checked*) pass "zero arguments still fails loudly with its own exit code" ;;
  *) fail "clean-bill: the zero-argument guard regressed: $r" ;;
esac

# ── 7. A FILE THAT EXISTS AND CANNOT BE OPENED ─────────────────────────────
#
# Both Judgment Day judges killed this file on 2026-09-07 with the same mutant:
# delete the `[ ! -r "$f" ]` arm. All nine assertions survived, and the mutant's
# behaviour on a chmod-000 file was the ORIGINAL DEFECT in its purest form —
#     PermissionError: [Errno 13] ...        (swallowed, on stderr)
#     clean: no silent-exit shapes in 1 file(s)   exit 0
# Sections 1-6 only ever produced paths that do not exist, or a directory.
# WHAT THIS CAN AND CANNOT KILL, so nobody chases it later. Deleting the `-r`
# arm is now an EQUIVALENT mutant: with the analyser's exit status checked
# (section 8), a chmod-000 file makes `_hazards` die and lands in the
# unanalysable list, so the verdict is still exit 65 and still not clean —
# measured 2026-09-07. The `-r` arm survives because it is the CHEAPER and
# better-diagnosed path to the same answer: it names the file plainly instead of
# showing the reader a PermissionError traceback. This section pins the VERDICT,
# which is the thing; which of the two branches produced it is not.
LOCKED="$TMP/locked.sh"
printf '#!/usr/bin/env bash\nx="$(grep nope /dev/null)"\n' > "$LOCKED"
chmod 000 "$LOCKED"
# Measured, not assumed: Git Bash mounts NTFS without ACLs, so chmod 000 leaves
# the file as readable as root's is.
locked=1; head -c1 "$LOCKED" >/dev/null 2>&1 && locked=0
r="$(lint "$LOCKED")"
chmod 644 "$LOCKED"
if [ "$(id -u)" = 0 ]; then
  printf 'ok - clean-bill: running as root, so an unopenable file cannot be produced here — this case is UNVERIFIED on this machine\n'
elif [ "$locked" = 0 ]; then
  printf 'ok - clean-bill: chmod 000 left the file readable (no POSIX permissions on this filesystem), so an unopenable file cannot be produced here — this case is UNVERIFIED on this machine\n'
else
  case "$r" in
    exit=0*) fail "clean-bill: a file that exists but cannot be OPENED produced exit 0. Its bytes were never read: $r" ;;
    exit=65*locked.sh*) pass "a file that exists but cannot be opened is exit 65 and is named" ;;
    *) fail "clean-bill: an unopenable file gave: $r" ;;
  esac
fi

# ── 8. THE ANALYSER'S OWN FAILURE IS NOT A CLEAN FILE ──────────────────────
#
# `read_count` used to be bumped after two predicates, before a byte was read,
# and `done < <(_hazards "$f")` threw the analyser's exit status away. Measured
# 2026-09-07 with `python3` stubbed to exit 7, against a file that really does
# carry a flagged shape: `clean: no silent-exit shapes in 1 file(s)`, exit 0.
mkdir -p "$TMP/brokenpy"
printf '#!/usr/bin/env bash\nexit 7\n' > "$TMP/brokenpy/python3"
chmod +x "$TMP/brokenpy/python3"
out="$(PATH="$TMP/brokenpy:$PATH" "$LS" "$DIRTY" 2>&1)" && rc=0 || rc=$?
case "$rc" in
  0) fail "clean-bill: with the analyser broken, lint-shell exited 0 on a file that DOES carry a hazard. An empty pipe from a failed reader is not a clean file: $out" ;;
  1) fail "clean-bill: with the analyser broken, lint-shell reported a FINDING. Nothing was analysed; there is no finding to report: $out" ;;
esac
case "$out" in
  *analyser\ FAILED*) pass "a crashed analyser is reported as unchecked, not as clean and not as flagged" ;;
  *) fail "clean-bill: a crashed analyser gave rc=$rc without saying the analyser failed: $out" ;;
esac

# ── 9. THE COUNT IN THE REFUSAL MATCHES THE LIST UNDER IT ──────────────────
#
# `grep -c .` skips empty lines, so `lint-shell ""` said "0 of 1 argument(s)
# could NOT be read" — a zero inside the sentence naming one. Cosmetic, and the
# subject of this whole file is numbers that match what happened.
r="$(lint "")"
case "$r" in
  *0\ of\ 1\ argument*) fail "clean-bill: the refusal counts 0 of 1 while refusing one argument: $r" ;;
  *1\ of\ 1\ argument*) pass "an empty-string argument is counted as the one unread argument it is" ;;
  *) fail "clean-bill: an empty-string argument gave: $r" ;;
esac
