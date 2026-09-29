#!/usr/bin/env bash
# The `unmeasured` hazard: a discarded stderr deciding something.
#
# `[ -n "$(cmd ... 2>/dev/null)" ]` cannot tell "the command answered, and the
# answer is nothing" from "the command failed". Both are an empty string, and
# the test decides on it anyway. That is the shape behind the 2026-09-07
# worktree fixes: `git status --porcelain 2>/dev/null` inside a test read a
# corrupted index as a clean tree, and `herdr agent list 2>/dev/null | jq … ||
# true` read a dead daemon as "nobody is working here" — one gate away from
# `git worktree remove`.
#
# WHY THE RULE IS THIS NARROW, measured over `bin/` on 2026-09-07 before it
# shipped: the naive version produced 111 findings and a variable-flow version
# 142. Nearly all were reads that are legitimately optional. A rule at that
# volume is one people learn to scroll past, which is worse than not having it.
# Three exclusions took it to 4, and all four were triaged one at a time:
#
#     bin/hw:5379                REAL and severe — an `fd` failure dropped a
#                                git-ignored tree off the irreplaceable list,
#                                and `hw reap --apply` removes on `safe`. Fixed.
#     bin/invoker-common.sh:870  REAL — malformed JSON read as "not blocked".
#     bin/invoker-common.sh:871  same. Both fixed by validating the reply once.
#     bin/channel-send:1448      REAL by shape, safe by direction — a failed
#                                `find` keeps the lock LIVE and the sender
#                                waits. Allowlisted, with the reason in the code.
#
# WHAT THIS FILE PINS. The rule fires on the shape, stays silent on each of the
# three guarded forms, and the allowlist works — driven against fixtures, never
# read out of `bin/lint-shell`'s own source.
#
# Run alone while working on this subject:
#     bash setup/tests/95-a-discarded-stderr-does-not-decide.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LS="$ROOT/bin/lint-shell"
[ -x "$LS" ] || fail "unmeasured: bin/lint-shell is missing or not executable"

probe() { # <body line(s)> → "exit=<rc>|<output>"
  local f="$TMP/probe.sh" out rc
  { printf '#!/usr/bin/env bash\nset -euo pipefail\n'; printf '%s\n' "$1"; } > "$f"
  out="$("$LS" "$f" 2>&1)" && rc=0 || rc=$?
  printf 'exit=%s|%s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}
flags()   { case "$(probe "$1")" in *discarded\ stderr*) return 0 ;; esac; return 1; }
silent()  { case "$(probe "$1")" in *discarded\ stderr*) return 1 ;; esac; return 0; }

# ── 1. IT FIRES ON THE SHAPE ───────────────────────────────────────────────
#
# The three real specimens from this codebase, reduced to one line each.
flags '[ -n "$(git -C "$wt" status --porcelain 2>/dev/null | head -1)" ] && dirty=1' \
  || fail "unmeasured: the rule missed a test on a substitution whose stderr is discarded — the git-status shape that read a corrupted index as a clean tree"
flags 'if [ -n "$(fd -H -t f . "$entry" 2>/dev/null | head -1)" ]; then keep=1; fi' \
  || fail "unmeasured: the rule missed the fd shape, which is the one it actually found in bin/hw"
flags '[ "$(printf "%s" "$info" | jq -r ".x // empty" 2>/dev/null)" = blocked ] || return 1' \
  || fail "unmeasured: the rule missed a jq read whose parse failure reads as a negative answer"
pass "the rule fires on a test whose subject had its stderr discarded"

# `while` and `until` count: a loop decides on the value just as a test does.
flags 'while [ -z "$(ls "$d" 2>/dev/null)" ]; do sleep 1; done' \
  || fail "unmeasured: the rule only looks at \`if\`/\`[\`, not at loops deciding on the same value"
pass "a loop condition counts as a decision too"

# ── 2. IT IS SILENT ON EACH GUARDED FORM ───────────────────────────────────
#
# These are the exclusions that took the count from 142 to 4. Each is a case
# where the failure IS observed, or where empty is genuinely the answer.
silent 'if top="$(git rev-parse --show-toplevel 2>/dev/null)"; then printf ok; fi' \
  || fail "unmeasured: flagged a substitution whose EXIT CODE is tested — the failure is observed there, only the stderr is hidden"
silent 'if ! top="$(git rev-parse --show-toplevel 2>/dev/null)"; then printf no; fi' \
  || fail "unmeasured: flagged a negated exit-code test"
pass "a substitution whose exit code is tested is not flagged"

silent '[ -r "$f/task" ] && seq="$(tr -dc "0-9" < "$f/task" 2>/dev/null || true)"' \
  || fail "unmeasured: flagged a read gated on the file existing — absent is the expected case and the guard says so"
silent '[ ! -d "$wt" ] || dirty="$(git -C "$wt" status --porcelain 2>/dev/null | grep -c "^" || true)"' \
  || fail "unmeasured: flagged a read gated on a negated existence test"
pass "a read gated on an existence test on the same line is not flagged"

silent '[ "$AGENT" != "$(project_default_kind "$P" 2>/dev/null || printf "claude")" ] && x=1' \
  || fail "unmeasured: flagged a substitution whose fallback supplies a real value — the empty case never reaches the test"
pass "a fallback that supplies a real value is not flagged"

# ── 3. THE KEYWORD MUST BE AT A COMMAND POSITION ───────────────────────────
#
# `until` is an ordinary variable name in this codebase (bin/hw:4424), and
# matching it anywhere turned two plain assignments into phantom loops on the
# first pass. This is the difference between reading shell and grepping it.
silent 'local until; until="$(sed -n "s/^until=//p" "$f" 2>/dev/null | head -1)"' \
  || fail "unmeasured: a variable NAMED until was read as a loop keyword — the rule is matching text, not command positions"
silent 'printf "%s" "$until" "$(date -r "$until" +%s 2>/dev/null || printf 0)"' \
  || fail "unmeasured: a substitution merely mentioning \$until was flagged as a loop condition"
pass "a variable named 'until' is not mistaken for a loop keyword"

# ── 4. THE ALLOWLIST ───────────────────────────────────────────────────────
#
# For a read whose failure is genuinely observed elsewhere, or whose empty
# branch is the safe one — and the marker is not enough on its own, the code
# has to say WHY. That second half is a convention this cannot enforce; what it
# can enforce is that the marker works and that it is specific.
silent '[ -n "$(find "$l" -mmin +1 2>/dev/null || true)" ] && dead=1  # lint-shell: measured-elsewhere' \
  || fail "unmeasured: the allowlist marker does not suppress the finding, so a deliberate exception has nowhere to go"
pass "the allowlist marker suppresses the finding on the line that carries it"

flags '[ -n "$(find "$l" -mmin +1 2>/dev/null || true)" ] && dead=1  # lint-shell: something-else' \
  || fail "unmeasured: ANY trailing lint-shell comment suppresses the rule — an allowlist that broad is an off switch"
pass "an unrelated lint-shell comment does not suppress it"

# ── 5. THE OTHER HAZARDS STILL FIRE ────────────────────────────────────────
#
# A new rule that shadows an existing one trades one blind spot for another.
case "$(probe 'x="$(grep nope /dev/null)"')" in
  *inherit\ a\ non-zero\ status*) pass "the assignment hazard still fires alongside the new rule" ;;
  *) fail "unmeasured: adding this rule silenced the assignment hazard: $(probe 'x="$(grep nope /dev/null)"')" ;;
esac
case "$(probe 'shift 2')" in
  *shift\ 2\ returns\ 1*) pass "the shift hazard still fires" ;;
  *) fail "unmeasured: the shift hazard stopped firing" ;;
esac

# ── 6. THE TRIAGED SET IS AT ZERO, AND THAT IS THE POINT ───────────────────
#
# Every finding the rule produced over `bin/` was looked at and resolved — three
# fixed, one allowlisted with its reason. So the steady state is zero, and a
# NEW one appearing means somebody wrote the shape again.
out="$("$LS" "$ROOT/bin"/* 2>&1 || true)"
n="$(printf '%s\n' "$out" | grep -c 'discarded stderr' || true)"
[ "$n" = 0 ] \
  || fail "unmeasured: bin/ carries $n unmeasured finding(s). Each one is triaged individually — fix it, or allowlist it with the reason in the code. Do not raise a baseline to make them fit: $(printf '%s\n' "$out" | grep -A1 'discarded stderr' | head -4 | tr '\n' ' ')"
pass "bin/ carries no unmeasured findings — the four it had were each resolved, not absorbed into a number"

# ── 7. THE SHAPES THE FIRST DRAFT WALKED PAST ──────────────────────────────
#
# A judge ran seven shapes carrying the identical defect through the rule on
# 2026-09-07 and FIVE got through. `case` matters most: it is the dominant
# conditional idiom in this codebase, so a rule that cannot see it would not
# have held its line past the next author.
flags 'case "$(git status --porcelain 2>/dev/null)" in "") clean=1 ;; esac' \
  || fail "unmeasured: a case statement deciding on a discarded-stderr substitution slips past — and case is the dominant conditional idiom here"
flags 'test -n "$(fd -H . "$e" 2>/dev/null)" && keep=1' \
  || fail "unmeasured: \`test\` is \`[\` under another name and slips past"
flags '[ -n "`git status 2>/dev/null`" ] && x=1' \
  || fail "unmeasured: backticks are \$() under an older name and slip past"
flags '[ -n "$(git -C "$(pwd)" status 2>/dev/null)" ] && x=1' \
  || fail "unmeasured: a NESTED substitution defeats the match — the subject pattern stops at the first close paren"
pass "case, test, backticks and a nested substitution are all seen"

# ── 8. THE EXCLUSIONS ARE SUBJECT-SCOPED, NOT LINE-SCOPED ──────────────────
#
# Both were matched anywhere on the line at first, and a judge showed that
# swallows real defects.
flags '[ -n "$(fd -H . "$e" 2>/dev/null)" ] || printf "empty\n"' \
  || fail "unmeasured: a trailing \`|| printf\` silenced the finding, but that is the TEST'S ELSE BRANCH, not a fallback inside the substitution — the value is still empty and still decides"
pass "an else-branch printf is not mistaken for a fallback inside the substitution"

flags '[ -d "$wt/.git" ] && [ -n "$(git status 2>/dev/null)" ] && y=1' \
  || fail "unmeasured: an existence test on a DIFFERENT path silenced the finding. That .git exists proves nothing about a corrupted index, which is the motivating bug"
silent '[ -r "$f/task" ] && seq="$(tr -dc "0-9" < "$f/task" 2>/dev/null || true)"' \
  || fail "unmeasured: the existence guard stopped working for the case it is FOR — the guard names the very path being read"
pass "the existence guard excludes only when it names the path the substitution reads"

# ── 9. THE if/while/until LIMB IS EXERCISED ON ITS OWN ─────────────────────
#
# Every "should flag" fixture above also contains `[ `, so deleting the whole
# command-position alternative left the file green — measured by a judge. This
# one has no bracket in it at all.
flags 'if grep -q x "$(f 2>/dev/null)"; then z=1; fi' \
  || fail "unmeasured: a substitution feeding an \`if <command>\` with no bracket is missed, so the keyword limb of the pattern is dead weight"
pass "the command-position limb is exercised without a bracket on the line"

# ── 10. NO errexit GATE ────────────────────────────────────────────────────
#
# The other hazards only fire under `set -e`; this one must not. A discarded
# stderr deciding a test is wrong either way, and the gate would exempt every
# file before its `set -e` line and every region after a `set +e`.
f="$TMP/noerrexit.sh"
printf '#!/usr/bin/env bash\nset -uo pipefail\n[ -n "$(git status 2>/dev/null)" ] && x=1\n' > "$f"
out="$("$LS" "$f" 2>&1 || true)"
case "$out" in
  *discarded\ stderr*) pass "the rule fires in a file that never sets errexit" ;;
  *) fail "unmeasured: a file with no \`set -e\` is exempt from this rule, so every script is unchecked until its errexit line: $out" ;;
esac
