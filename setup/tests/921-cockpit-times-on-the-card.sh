#!/usr/bin/env bash
# The card says how long an executor has worked and marks one that stopped moving
#
# cockpit/hooks/view.tsx draws, on a WORKING card, `working 3h12m · turn 41m`; when the card's
# last_progress_at is older than rules.no_progress_after_ms (the writer's one constant, read from the
# file) it adds `no progress 47m` in the attention colour and colours the card's border; and a queued
# ruling says how old it is (`1 ruling queued 25m`). The tests are in cockpit/hooks/register.test.ts
# and run with 801's `claude plugin test`; this subject proves they are not decoration: each mutant of
# the mod below must turn red THE TEST THAT NAMES ITS RULE (a mutant that only trips the 40-row
# timing test, which is a load-dependent budget, is not a kill).
#
# Run alone while working on this subject:
#     bash setup/tests/921-cockpit-times-on-the-card.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

C="$ROOT/cockpit"
[ -d "$C/hooks" ] || { pass "921: this tree carries no cockpit mod"; exit 0; }
command -v claude >/dev/null 2>&1 || { pass "921: claude is not installed — the mod is not exercised"; exit 0; }
ver="$(claude --version 2>/dev/null | rg -o '^[0-9]+\.[0-9]+\.[0-9]+' || true)"
[ -n "$ver" ] && [ "$(printf '%s\n2.1.289\n' "$ver" | sort -V | head -1)" = 2.1.289 ] \
  || { pass "921: claude ${ver:-?} is below 2.1.289 — the mod is not exercised"; exit 0; }
TMP="$(mktemp -d "${TMPDIR:-/tmp}/t921.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

copy() { rm -rf "$1"; mkdir -p "$1"; cp -R "$C/.claude-plugin" "$C/hooks" "$C/schema.json" "$1/"; }
run_tests() { ( cd "$1" && claude plugin test . ) >"$2" 2>&1; }

# name @@ file @@ from @@ to @@ the test that must go red
cat > "$TMP/mutants" <<'M'
threshold ignored (any age is no progress) @@ hooks/view.tsx @@ return age > thresholdMs ? age : null @@ return age > 0 ? age : null @@ whose last progress is older than hw
threshold copied into the mod instead of read from the file @@ hooks/view.tsx @@ thresholdMs: m.state?.rules.no_progress_after_ms ?? Infinity @@ thresholdMs: 1_800_000 @@ the threshold is the one in the file
a stalled card keeps the plain colour @@ hooks/view.tsx @@ color={dim ? undefined : STALL_COLOR} @@ color={undefined} @@ drawn in the attention colour
a stalled card keeps its border @@ hooks/view.tsx @@ stalled !== null ? STALL_COLOR : badge.color @@ badge.color @@ drawn in the attention colour
a card with no progress time is marked @@ hooks/view.tsx @@ r.last_progress_at === null) return null @@ false) return null @@ with no progress time is not marked
the ruling's age is lost @@ hooks/view.tsx @@ r.rulings_pending.oldest_at === null ? '' : @@ true ? '' : @@ a queued ruling says how old
the turn is not said @@ hooks/view.tsx @@ ` · turn ${dur(now - r.turn_started_at)}` @@ '' @@ says how long it has worked
every card says it works @@ hooks/view.tsx @@ if (r.attention === 'working') { @@ if (true as boolean) { @@ carries no working line
no progress is drawn for a card that does not work @@ hooks/view.tsx @@ if (r.attention !== 'working' || @@ if (false as boolean || @@ is never marked no progress
a card that does not know the window shows n/a instead of its tokens @@ hooks/view.tsx @@ return r.ctx.pct === null ? `ctx ${tokensText(r.ctx.tokens)}` : @@ return r.ctx.pct === null ? 'ctx n/a' : @@ shows the tokens alone
a card with tokens but no source shows n/a @@ hooks/view.tsx @@ if (r.ctx.tokens === null) return 'ctx n/a' @@ if (false as boolean) return 'ctx n/a' @@ says n/a
the file's new fields are not required @@ hooks/state.ts @@ if (!isInt(r.dispatched_at)) return `${at}.dispatched_at` @@  @@ a file without dispatched_at is dead
M
n=0
while IFS= read -r line; do
  [ -n "$line" ] || continue
  copy "$TMP/m"
  want="${line##* @@ }"
  name="$(python3 -I - "$TMP/m" "$line" <<'PY'
import sys
root, line = sys.argv[1:3]
name, file, a, b, _want = [x.strip() for x in line.split(" @@ ")]
p = root + "/" + file
s = open(p).read()
if a not in s: sys.exit(1)
open(p, "w").write(s.replace(a, b, 1))
print(name)
PY
)" || fail "921: a mutant's target text is not in its file — it tests nothing: ${line%% @@*}"
  if run_tests "$TMP/m" "$TMP/m.out"; then fail "921: mutant '$name' survived: claude plugin test stayed green"; fi
  rg -q "^\(fail\) .*${want}" "$TMP/m.out" || fail "921: mutant '$name' went red, but not on the test that names its rule ('$want'): $(rg '^\(fail\)' "$TMP/m.out" | head -3 | tr '\n' ' ')"
  pass "mutant killed: 921 $name (saw: (fail) …${want}…)"
  n=$((n + 1))
done < "$TMP/mutants"
pass "921: $n mutants of the card, each killed by the test that names its rule"
