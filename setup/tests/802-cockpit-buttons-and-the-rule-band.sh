#!/usr/bin/env bash
# The cockpit's buttons start only the contract's verbs, and its rule band only looks
#
# cockpit/ is a Claude Code mod. T4 gives it hands: a button per verb and a band
# that shows hw's preflight verdict. Claims:
#
#   · hooks/verbs.ts spells the argv templates of cockpit/verbs.txt exactly, with
#     the same confirm and stdin attributes: the table the mod runs from IS the
#     contract (verbs.txt is not read at run time, so this is where they meet);
#   · `claude plugin validate cockpit` has no error and now names process.run and
#     prompt.fill among its calls, still inside the allowlist;
#   · every `process.run` in the source takes an argv from argvOf (never a string
#     built in place);
#   · `claude plugin test cockpit` passes: each button starts one verb with the
#     row's own names, `done` asks first, a refusal is shown verbatim and the card
#     stays, the ruling goes on stdin, the band shows warn and block and never
#     denies, and an unavailable preflight fails open;
#   · the tests are not decoration: each mutant of the mod below turns them red.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

C="$ROOT/cockpit"
[ -f "$C/hooks/verbs.ts" ] || { pass "802: this tree carries no cockpit actions"; exit 0; }

python3 -I - "$C" <<'PY' || fail "802: verbs.ts and verbs.txt disagree (see above)"
import os, re, sys
C = sys.argv[1]
txt = {}
for l in open(os.path.join(C, "verbs.txt")):
    l = l.strip()
    if not l or l.startswith("#"): continue
    v, tpl, confirm, stdin = [x.strip() for x in l.split(" | ")]
    txt[v] = (tpl, confirm == "confirm=yes", stdin == "stdin=text")
ts = open(os.path.join(C, "hooks/verbs.ts")).read()
code = {}
for m in re.finditer(r"^  '?([\w-]+)'?: \{ template: '([^']+)', confirm: (true|false), stdin: (true|false) \},$", ts, re.M):
    code[m.group(1)] = (m.group(2), m.group(3) == "true", m.group(4) == "true")
if code != txt:
    for k in sorted(set(code) | set(txt)):
        if code.get(k) != txt.get(k): print("  ✗", k, "verbs.ts:", code.get(k), "verbs.txt:", txt.get(k))
    sys.exit(1)
PY
pass "802: verbs.ts carries exactly the lines of verbs.txt"

# Every process the mod starts takes its argv from the contract's table.
bad="$(rg -n 'process\.run\(' "$C/hooks" --glob '!*.test.ts' | rg -v 'process\.run\((argv|cmd as string\[\])' || true)"
[ -z "$bad" ] || fail "802: a process.run in the source does not take an argv from argvOf: $bad"
pass "802: every process.run takes an argv built from the table"

command -v claude >/dev/null 2>&1 || { pass "802: claude is not installed — the mod is not exercised"; exit 0; }
ver="$(claude --version 2>/dev/null | rg -o '^[0-9]+\.[0-9]+\.[0-9]+' || true)"
[ -n "$ver" ] && [ "$(printf '%s\n2.1.289\n' "$ver" | sort -V | head -1)" = 2.1.289 ] \
  || { pass "802: claude ${ver:-?} is below 2.1.289 — the mod is not exercised"; exit 0; }

copy() { rm -rf "$1"; mkdir -p "$1"; cp -R "$C/.claude-plugin" "$C/hooks" "$C/schema.json" "$1/"; }

json="$TMP/validate.json"
claude plugin validate "$C" --json > "$json" 2>"$TMP/validate.err" || fail "802: claude plugin validate failed: $(head -c 400 "$json" "$TMP/validate.err")"
python3 -I - "$json" <<'PY' || fail "802: the mod's call set is wrong (see above)"
import json, re, sys
d = json.load(open(sys.argv[1]))
calls = set()
for c in d["contents"]:
    for n in c["notes"]:
        m = re.search(r"calls: (.*)$", n)
        if m: calls |= {x.strip().removeprefix("$.") for x in re.sub(r" \(via [^)]*\)", "", m.group(1)).split(", ")}
ALLOW = {"fs.stat", "fs.read", "fs.write", "fs.exists", "env.get", "clock.every", "clock.now", "command.register", "prompt.fill", "process.run"}
bad = sorted(x for x in calls if not (x in ALLOW or x.startswith("ui.")))
if bad: print("  ✗ calls outside the allowlist:", bad); sys.exit(1)
miss = sorted({"process.run", "prompt.fill", "ui.ask"} - calls)
if miss: print("  ✗ the buttons are not wired, missing:", miss); sys.exit(1)
PY
pass "802: validate has no error; the calls are inside the allowlist and include the buttons'"

run_tests() { ( cd "$1" && claude plugin test . ) >"$2" 2>&1; }
copy "$TMP/base"
run_tests "$TMP/base" "$TMP/base.out" || fail "802: claude plugin test is red on the mod itself: $(tail -n 12 "$TMP/base.out")"
pass "802: claude plugin test passes ($(rg -o '[0-9]+ pass' "$TMP/base.out" | tail -1))"

# Each mutant: name, file, from, to. A `from` that is not in the file tests nothing.
cat > "$TMP/mutants.py" <<'PY'
import sys
M = [
 ("a button calls a verb outside the allowlist", "hooks/verbs.ts", "hw done <project> <task>", "hw reap <project> <task>"),
 ("a button runs on the wrong row", "hooks/register.tsx", "task: r.task,\n  pane: r.pane,", "task: r.project,\n  pane: r.pane,"),
 ("a challenge is answered with the ask's verb", "hooks/register.tsx", "r.ask.kind === 'challenge' ? 'challenge-reply' : 'reply'", "'reply'"),
 ("the reply drops its hold", "hooks/verbs.ts", "'channel-send --report --reply-hold <pending_reply> herdr", "'channel-send --report herdr"),
 ("the reply text goes in argv, not stdin", "hooks/register.tsx", "r.id, namesOf(r), text)\n    },\n    verify", "r.id, namesOf(r))\n    },\n    verify"),
 ("done skips its confirmation", "hooks/verbs.ts", "template: 'hw done <project> <task>', confirm: true", "template: 'hw done <project> <task>', confirm: false"),
 ("the ruling text goes in argv, not stdin", "hooks/register.tsx", "stdin === undefined ? { timeoutMs: VERB_MS } : { stdin, timeoutMs: VERB_MS }", "{ timeoutMs: VERB_MS }"),
 ("the argv is one shell string", "hooks/register.tsx", "await $.process.run(argv, stdin", "await $.process.run(['sh', '-c', argv.join(' ')], stdin"),
 ("hw's answer is dropped", "hooks/register.tsx", "S.notes[id] = note", "void note"),
 ("the mod decides: every action is a button", "hooks/view.tsx", "verbs.filter(([v]) => a[v]).map(", "verbs.filter(([v]) => true).map("),
 ("the mod decides: ruling always has its input", "hooks/view.tsx", "a.ruling ? (", "true ? ("),
 ("a stale state draws buttons", "hooks/view.tsx", "  if (!dim) {\n    const verbs", "  if (true as boolean) {\n    const verbs"),
 ("the band denies on a verdict", "hooks/register.tsx", "if (argv !== null) await checkDispatch($, argv)", "if (argv !== null) { await checkDispatch($, argv); if (S.rule !== null && S.rule.v !== 'unavailable' && S.rule.v.level !== 'ok') return { deny: 'rule' } }"),
 ("the band fails closed", "hooks/register.tsx", "if (argv !== null) await checkDispatch($, argv)", "if (argv !== null) { await checkDispatch($, argv); if (S.rule !== null && S.rule.v === 'unavailable') return { deny: 'no preflight' } }"),
 ("every Bash command is checked", "hooks/register.tsx", "if (argv !== null) await checkDispatch($, argv)", "await checkDispatch($, argv ?? ['x'])"),
 ("the verdict's schema is ignored", "hooks/rules.ts", "v.schema !== 1) return null", "false as boolean) return null"),
 ("a pipe is read as a dispatch", "hooks/rules.ts", "'|&;<>()$`\\\\*?[]{}!#~'.includes(c)", "'&;<>()$`\\\\*?[]{}!#~'.includes(c)"),
 ("a button press is not recorded for the pilot", "hooks/register.tsx", "await recordAction($, names.pane ?? '', verb)", "void 0"),
 ("the action file is overwritten, not appended to", "hooks/register.tsx", "`${old}${JSON.stringify", "`${JSON.stringify"),
 ("preflight has no time limit", "hooks/register.tsx", "const PREFLIGHT_MS = 3000", "const PREFLIGHT_MS = 3000000"),
 ("a card that costs milliseconds makes the 40-row draw slow in all seven tries, not only on a loaded machine", "hooks/view.tsx", "function RowCard({ els, r, now, dim, act, notes, thresholdMs }: { els: Els; r: Row; now: number; dim: boolean; act: Acts; notes: Notes; thresholdMs: number }) {", "function RowCard({ els, r, now, dim, act, notes, thresholdMs }: { els: Els; r: Row; now: number; dim: boolean; act: Acts; notes: Notes; thresholdMs: number }) {\n  let burn = 0\n  for (let i = 0; i < 3000000; i++) burn += i % 7\n  if (burn < 0) throw new Error('unreachable')"),
]
if sys.argv[1] == "count": print(len(M)); sys.exit(0)
name, f, a, b = M[int(sys.argv[1])]
p = sys.argv[2] + "/" + f
s = open(p).read()
if a not in s: print("MISSING " + name); sys.exit(1)
open(p, "w").write(s.replace(a, b, 1))
print(name)
PY
n="$(python3 -I "$TMP/mutants.py" count)"
i=0
while [ "$i" -lt "$n" ]; do
  copy "$TMP/m"
  name="$(python3 -I "$TMP/mutants.py" "$i" "$TMP/m")" || fail "802: mutant $i's target text is not in its file — it tests nothing: $name"
  if run_tests "$TMP/m" "$TMP/m.out"; then fail "802: mutant '$name' survived: claude plugin test stayed green"; fi
  i=$((i + 1))
done
pass "802: $n mutants of the mod, each killed by the tests"
