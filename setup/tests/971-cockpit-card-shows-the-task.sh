#!/usr/bin/env bash
# The cockpit card shows how a task is going, not hw's internals
#
# cockpit/hooks/view.tsx draws each executor as below (task/cockpit-rediseno: a capture of 2026-10-08
# showed four of a card's seven lines given to refused buttons' reasons, with full paths):
#
#     task-05 · Go T4: verbos escritores           ● working 3h12m · turn 41m
#     claude · claude-sonnet-5-5 · high · ctx 41% ▰▰▱▱▱▱
#     2 commits · last 37m ago · gate running 12m
#     ⚠ no progress 47m · 1 ruling queued 25m          (only when there is a reason)
#                                        [ receipt ]  (only the actions hw would run)
#
# The tests are in cockpit/hooks/register.test.ts and actions.test.ts and run with 801's
# `claude plugin test`; this subject proves they are not decoration: each mutant below must turn red
# THE TEST THAT NAMES ITS RULE. The last mutant is of the 40-row draw test itself: each of its seven
# draws must read and parse the file again (a draw that reuses the parsed state measures less than a
# writer's beat costs), so a test that stops rewriting the file must go red.
#
# Run alone while working on this subject:
#     bash setup/tests/971-cockpit-card-shows-the-task.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

C="$ROOT/cockpit"
[ -d "$C/hooks" ] || { pass "971: this tree carries no cockpit mod"; exit 0; }
command -v claude >/dev/null 2>&1 || { pass "971: claude is not installed — the mod is not exercised"; exit 0; }
ver="$(claude --version 2>/dev/null | rg -o '^[0-9]+\.[0-9]+\.[0-9]+' || true)"
[ -n "$ver" ] && [ "$(printf '%s\n2.1.289\n' "$ver" | sort -V | head -1)" = 2.1.289 ] \
  || { pass "971: claude ${ver:-?} is below 2.1.289 — the mod is not exercised"; exit 0; }
TMP="$(mktemp -d "${TMPDIR:-/tmp}/t971.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

copy() { rm -rf "$1"; mkdir -p "$1"; cp -R "$C/.claude-plugin" "$C/hooks" "$C/schema.json" "$1/"; }
run_tests() { ( cd "$1" && claude plugin test . ) >"$2" 2>&1; }

copy "$TMP/base"
run_tests "$TMP/base" "$TMP/base.out" || fail "971: claude plugin test is red on the mod itself: $(tail -n 12 "$TMP/base.out")"
pass "971: claude plugin test passes ($(rg -o '[0-9]+ pass' "$TMP/base.out" | tail -1))"

# name @@ file @@ from @@ to @@ the test that must go red
cat > "$TMP/mutants" <<'M'
a refused action is a button again @@ hooks/view.tsx @@ verbs.filter(([v]) => a[v]).map( @@ verbs.map( @@ an action hw refuses is not on the card
a refused action's reason is back on the card @@ hooks/view.tsx @@     if (buttons.length > 0) { @@     for (const [v] of verbs.filter(([v]) => !a[v])) lines.push(<Text key={`why:${v}`} dimColor>{cut(`[${v}] ${a.why_not?.[v] ?? ''}`, 200)}</Text>)\n    if (buttons.length > 0) { @@ an action hw refuses is not on the card
the attention line is drawn with no reason @@ hooks/view.tsx @@ if (attention.length > 0) { @@ if (true as boolean) { @@ the attention line is there only with a reason
the reasons are cut to the first @@ hooks/view.tsx @@ {`⚠ ${attention.join(' · ')}`} @@ {`⚠ ${attention[0]}`} @@ says both on one attention line
the brief's title is not drawn @@ hooks/view.tsx @@ r.title === null ? slug : @@ true ? slug : @@ a card says the task and its brief
the project is always named @@ hooks/view.tsx @@ slug: oneProject ? r.task : r.id @@ slug: r.id @@ a card says the task and its brief
the gate is not said @@ hooks/view.tsx @@ if (r.gate !== null) parts.push( @@ if (false as boolean) parts.push( @@ a card says its commits, the last one
the last commit's age is lost @@ hooks/view.tsx @@ if (r.commits.last_at !== null) parts.push( @@ if (false as boolean) parts.push( @@ a card says its commits, the last one
an empty button row is drawn @@ hooks/view.tsx @@ if (buttons.length > 0) { @@ if (true as boolean) { @@ draws no button row at all
the state goes on one line, not beside the title @@ hooks/view.tsx @@ <Box key="head" justifyContent="space-between" columnGap={2}> @@ <Box key="head" columnGap={2}> @@ its state with how long it has held it
the title field is not required @@ hooks/state.ts @@ if (!nullOr(r.title, isStr)) return `${at}.title` @@  @@ a file without the card’s title is dead
the 40-row test reuses the parsed state @@ hooks/register.test.ts @@     w.mtime += 1\n    const reads0 @@     const reads0 @@ 40 rows draw in under 50 ms
M
n=0
while IFS= read -r line; do
  [ -n "$line" ] || continue
  copy "$TMP/m"
  want="${line##* @@ }"
  name="$(python3 -I - "$TMP/m" "$line" <<'PY'
import sys
root, line = sys.argv[1:3]
name, file, a, b, _want = [x.strip(" ") for x in line.split(" @@ ")]
a, b = a.replace("\\n", "\n"), b.replace("\\n", "\n")
p = root + "/" + file
s = open(p).read()
if a not in s: sys.exit(1)
open(p, "w").write(s.replace(a, b, 1))
print(name)
PY
)" || fail "971: a mutant's target text is not in its file — it tests nothing: ${line%% @@*}"
  if run_tests "$TMP/m" "$TMP/m.out"; then fail "971: mutant '$name' survived: claude plugin test stayed green"; fi
  rg '^\(fail\) ' "$TMP/m.out" | rg -qF -- "$want" || fail "971: mutant '$name' went red, but not on the test that names its rule ('$want'): $(rg '^\(fail\)' "$TMP/m.out" | head -3 | tr '\n' ' ')"
  pass "mutant killed: 971 $name (saw: (fail) …${want}…)"
  n=$((n + 1))
done < "$TMP/mutants"
pass "971: $n mutants of the card, each killed by the test that names its rule"
