#!/usr/bin/env bash
# The pre-commit hook carried its own cost as a prose constant, and it was wrong.
#
# `setup/hooks/pre-commit` said: "~19s as of 2026-08-25, at 158 tests —
# measured, not estimated, because a hook's cost is what decides whether people
# reach for --no-verify." That comment had ALREADY corrected an earlier "~4s".
# On 2026-09-07, at 1856 tests, the same suite took ~13 minutes run alone and
# ~40 from the hook on a loaded machine, and that session reached for
# `--no-verify` six times. The comment predicted its own failure and then became
# an instance of it.
#
# A NUMBER MEASURED ONCE AND PRINTED AS A PROPERTY IS THE SAME DEFECT IN PROSE:
# an assertion about a subject printed from something other than a signal that
# subject emitted, and a prose constant cannot emit anything. So the hook now
# times its own run and prints what it actually took.
#
# WHAT THIS FILE PINS. The cost is reported from the clock on THIS run, on
# success as well as failure, and no fixed number is written down as the cost.
# It also pins that the versioned hook and the installed one are the same file —
# a hook nobody installed is a check nobody runs.
#
# Run alone while working on this subject:
#     bash setup/tests/94-the-hook-measures-its-own-cost.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HOOK="$ROOT/setup/hooks/pre-commit"
[ -x "$HOOK" ] || fail "hook-cost: setup/hooks/pre-commit is missing or not executable"

# ── 1. THE REPORTED COST COMES FROM A CLOCK ───────────────────────────────
#
# NOT A REGEX OVER THE PROSE. The first version of this section hunted for
# "~<n>s" in the file and failed it — on the file's own account of having been
# wrong twice, which is the argument, not the defect. Policing prose with a
# pattern is the same mistake one level up: it reads the text instead of the
# thing. What matters is where the PRINTED number comes from, and section 2
# settles that by running the hook twice at different speeds.
#
# Here, only the structural half: the clock is read, and the line the operator
# sees is built from what it returned rather than from a literal.
grep -q 'date +%s' "$HOOK" \
  || fail "hook-cost: the hook does not read a clock at all, so it cannot be reporting a measured cost"
python3 - "$HOOK" <<'PY' || exit 1
import re, sys
src = open(sys.argv[1], encoding="utf-8").read()
lines = [l for l in src.split("\n")
         if "measured on this run" in l and l.lstrip().startswith(("printf", "'"))]
if not lines:
    print("not ok - hook-cost: no line reports a cost 'measured on this run'", file=sys.stderr)
    raise SystemExit(1)
# The seconds in that report must be a substituted value, never a literal.
for l in lines:
    if re.search(r"\bin \d+s\b", l):
        print("not ok - hook-cost: the cost line carries a literal duration: %r" % l.strip(),
              file=sys.stderr)
        raise SystemExit(1)
if "hook_elapsed" not in src:
    print("not ok - hook-cost: nothing named hook_elapsed exists, so the printed seconds "
          "are not the measured ones", file=sys.stderr)
    raise SystemExit(1)
print("ok - the cost line is built from a clock reading, not from a literal")
PY

# ── 2. DRIVE IT, in a throwaway repo with a stubbed suite ──────────────────
#
# GIT_* is stripped for the reason the hook's own comment gives: a hook exports
# GIT_DIR and GIT_INDEX_FILE, and a child `git init` would otherwise land on the
# real repository. That happened here once, on 2026-08-24.
mk_repo() { # <name> <suite exit> <suite sleep> <suite stdout>
  local d="$TMP/$1"
  mkdir -p "$d/bin" "$d/setup/hooks"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git init -q )
  # pre-commit sources its sibling setup/hooks/suite-trigger-pattern.sh via
  # $(git rev-parse --show-toplevel) — unconditionally — so a copy of
  # pre-commit at the fixture's own toplevel needs that sibling too.
  cp "$ROOT/setup/hooks/suite-trigger-pattern.sh" "$d/setup/hooks/suite-trigger-pattern.sh"
  cp "$HOOK" "$d/hook"
  cat > "$d/setup/test-hw" <<STUB
#!/usr/bin/env bash
sleep $3
printf '%s\n' "$4"
exit $2
STUB
  chmod +x "$d/setup/test-hw"
  # A bin/ change is what arms the suite branch.
  printf 'x\n' > "$d/bin/tool"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git add bin/tool )
  printf '%s' "$d"
}

run_hook() { # <repo> → "exit=<rc>|<stderr>"
  local d="$1" err rc=0
  err="$( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE \
            bash ./hook 2>&1 >/dev/null )" || rc=$?
  printf 'exit=%s|%s' "$rc" "$(printf '%s' "$err" | tr '\n' ' ')"
}

# (a) A PASSING SUITE REPORTS ITS COST. A cost that only appears when something
# is already broken is invisible exactly while it is being paid.
d="$(mk_repo pass 0 2 '1856 tests passed  (TRACKED only)')"
r="$(run_hook "$d")"
case "$r" in
  exit=0*) : ;;
  *) fail "hook-cost: a passing suite did not let the commit through: $r" ;;
esac
case "$r" in
  *passed\ in\ [0-9]*s\ \(measured\ on\ this\ run\)*1856\ tests\ passed*) pass "a passing suite reports its measured seconds and the suite's own headline" ;;
  *) fail "hook-cost: a passing run printed no measured cost: $r" ;;
esac

# AND THE NUMBER IS THE CLOCK, not a constant. Two runs of different length must
# report different numbers — this is what separates measuring from printing.
d2="$(mk_repo slower 0 5 '1856 tests passed')"
r2="$(run_hook "$d2")"
fast="$(printf '%s' "$r"  | sed -n 's/.*passed in \([0-9][0-9]*\)s.*/\1/p')"
slow="$(printf '%s' "$r2" | sed -n 's/.*passed in \([0-9][0-9]*\)s.*/\1/p')"
[ -n "$fast" ] && [ -n "$slow" ] \
  || fail "hook-cost: could not read a duration out of both runs (fast=$fast slow=$slow)"
[ "$slow" -gt "$fast" ] \
  || fail "hook-cost: a 5s suite reported ${slow}s and a 2s suite reported ${fast}s. The number does not track the clock, so it is not a measurement"
pass "the reported seconds track the actual run time (2s run -> ${fast}s, 5s run -> ${slow}s)"

# (b) A FAILING SUITE STILL REPORTS THE COST, and still blocks.
d="$(mk_repo failing 1 1 'not ok - something broke')"
r="$(run_hook "$d")"
case "$r" in
  exit=1*failed\ after\ [0-9]*s*) : ;;
  exit=1*) fail "hook-cost: a failing suite blocked but reported no cost: $r" ;;
  *) fail "hook-cost: a failing suite did not block the commit: $r" ;;
esac
# AND THAT NUMBER IS THE CLOCK TOO. A judge put the constant `19s` back into
# this exact line on 2026-09-07 and every assertion here stayed green: section
# 1's literal guard only scans lines containing "measured on this run", and the
# failure line does not. Two failing runs of different length settle it.
d_slowfail="$(mk_repo failing-slower 1 4 'not ok - something broke')"
r_sf="$(run_hook "$d_slowfail")"
fail_fast="$(printf '%s' "$r"    | sed -n 's/.*failed after \([0-9][0-9]*\)s.*/\1/p')"
fail_slow="$(printf '%s' "$r_sf" | sed -n 's/.*failed after \([0-9][0-9]*\)s.*/\1/p')"
[ -n "$fail_fast" ] && [ -n "$fail_slow" ] \
  || fail "hook-cost: could not read a duration out of both failing runs (fast=$fail_fast slow=$fail_slow)"
[ "$fail_slow" -gt "$fail_fast" ] \
  || fail "hook-cost: a 4s failing suite reported ${fail_slow}s and a 1s one reported ${fail_fast}s. The failure line's number is a constant, not a measurement — and a constant there is the very prose number this change deleted"
pass "a failing suite blocks the commit, and the cost it names also tracks the clock (1s -> ${fail_fast}s, 4s -> ${fail_slow}s)"

# (b2) THE SUITE'S NUMBER TRAVELS WITH ITS OWN CAVEAT, or it does not travel.
#
# `setup/test-hw` prints "N tests passed  (TRACKED only — the number this commit
# can reproduce)" and argues in its own source that the number must never be
# quoted without the sentence under it. The first version of this hook lifted
# the integer with `sed` and reprinted it bare: a judge stubbed the real
# two-line output on 2026-09-07 and the hook said "1856 tests" while 1900
# subjects had run. A new instance of the class, created inside the fix for it.
d="$(mk_repo caveat 0 1 '1856 tests passed  (TRACKED only — the number this commit can reproduce)')"
r="$(run_hook "$d")"
case "$r" in
  *TRACKED\ only*) pass "the suite's headline reaches the operator with its own qualification attached" ;;
  *) fail "hook-cost: the hook stripped the caveat off the suite's number — a reader quotes the number and not the sentence under it: $r" ;;
esac

# And the untracked line, which says how many MORE ran than the number admits.
d="$(mk_repo untracked 0 1 '1856 tests passed  (TRACKED only)')"
cat > "$d/setup/test-hw" <<'STUB'
#!/usr/bin/env bash
sleep 1
printf '\n1856 tests passed  (TRACKED only — the number this commit can reproduce)\n'
printf '  + 44 ok from UNTRACKED subjects, absent from the commit and NOT in the number above: 95-x.sh\n'
STUB
chmod +x "$d/setup/test-hw"
r="$(run_hook "$d")"
case "$r" in
  *44\ ok\ from\ UNTRACKED*) pass "the untracked-subject line travels too, so the headline is never the whole story on its own" ;;
  *) fail "hook-cost: 44 subjects ran outside the number and the hook did not say so: $r" ;;
esac

# (c) A SUITE WHOSE COUNT LINE MOVED still reports the cost. Degrading to "no
# count" is honest; degrading to "no cost" would put the number back in prose.
d="$(mk_repo nocount 0 1 'everything is fine, trust me')"
r="$(run_hook "$d")"
case "$r" in
  *passed\ in\ [0-9]*s*test\ count\ line\ was\ not\ found*) pass "an unreadable test count still reports the measured cost, and says the count is missing" ;;
  *) fail "hook-cost: with no parseable count the hook reported: $r" ;;
esac

# (d) EXPENSIVE SAYS SO, and the branch is DRIVEN.
#
# This was `grep -q 'over two minutes' "$HOOK"` — a literal-presence read of
# the subject's own source — and both Judgment Day judges killed it on
# 2026-09-07: raising the threshold to 999999 and replacing the `if` with
# `false` both survived while the text stayed. The `pass` line even claimed the
# warning "exists to be triggered"; nothing had ever triggered it. That is the
# category this file's own section 1 disavows, committed three sections later.
#
# A branch nothing can execute is a branch nothing can test, so the threshold
# reads `HW_PRECOMMIT_SLOW_SECONDS` and a 1s run is driven against a 0s bar.
d="$(mk_repo slow 0 1 '10 tests passed')"
r="$(run_hook "$d")"
case "$r" in
  *calls\ expensive*) fail "hook-cost: a 1s run against the default 120s threshold claimed to be expensive: $r" ;;
esac

run_hook_slow() { # <repo> → stderr, with the bar on the floor
  ( cd "$1" && HW_PRECOMMIT_SLOW_SECONDS=0 env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE \
      bash ./hook 2>&1 >/dev/null ) | tr '\n' ' '
}
r2="$(run_hook_slow "$d")"
case "$r2" in
  *calls\ expensive*) : ;;
  *) fail "hook-cost: with the bar at 0s a 1s run said nothing about being expensive. The warning is unreachable, and an unreachable branch is one nobody can test: $r2" ;;
esac
# AND IT NAMES THE BAR IT CROSSED. "over two minutes" was hardcoded prose while
# the threshold became configurable — the same defect one more time, smaller.
case "$r2" in
  *past\ the\ 0s\ mark*) pass "the expensive warning fires when the measured time crosses the bar, and names the bar it crossed" ;;
  *) fail "hook-cost: the warning fired but states a threshold that is not the one in effect: $r2" ;;
esac

# ── 3. THE INSTALLED HOOK IS THE VERSIONED ONE ─────────────────────────────
#
# A fix to a hook nobody installed is a fix nobody runs. `.git/hooks` is not
# versioned; setup/hooks/README.md says to symlink it.
INSTALLED="$ROOT/.git/hooks/pre-commit"
if [ -e "$INSTALLED" ]; then
  if [ -L "$INSTALLED" ]; then
    [ "$(cd "$ROOT/.git/hooks" && cd "$(dirname "$(readlink pre-commit)")" && pwd)/$(basename "$(readlink "$INSTALLED")")" = "$HOOK" ] \
      || fail "hook-cost: .git/hooks/pre-commit is a symlink, but not to setup/hooks/pre-commit"
    pass "the installed pre-commit is a symlink to the versioned one, so this fix is live"
  else
    cmp -s "$INSTALLED" "$HOOK" \
      || fail "hook-cost: .git/hooks/pre-commit is a COPY and it has drifted from setup/hooks/pre-commit. The versioned fix is not what runs"
    pass "the installed pre-commit is a copy and still matches the versioned one"
  fi
else
  # Not a silent skip: say what was not established, on stdout.
  printf 'ok - hook-cost: no .git/hooks/pre-commit on this checkout, so whether the fix is INSTALLED is UNVERIFIED here (the versioned hook was exercised above)\n'
fi
