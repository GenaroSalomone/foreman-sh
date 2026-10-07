#!/usr/bin/env bash
# The cockpit mod reads the contract's states right (and may start only hw verbs)
#
# cockpit/ is a Claude Code mod (plugin.json, hooks/). Claims:
#
#   · `claude plugin validate cockpit` has no error, and the mod's `$.` calls
#     are inside the allowlist: writes no file but the pilot's cockpit-actions.jsonl, reaches the network,
#     calls a model, sends a prompt or keeps a store (the buttons' verbs and the
#     rule band are held by 802);
#   · hooks/fixtures.gen.ts carries exactly the bytes of cockpit/fixtures/ (a
#     mod test can neither read a file nor import JSON, so it rides in there);
#   · `claude plugin test cockpit` passes: every fixture of fixtures/index.json
#     draws the class and the rows its bytes earn, the thresholds, the band,
#     the 12/40-row render, the disabled reply and the redraw budget;
#   · the tests are not decoration: each mutant of the mod below turns them red.
#
#     bash setup/tests/801-cockpit-mod-reads-the-state.sh           # check
#     bash setup/tests/801-cockpit-mod-reads-the-state.sh --write   # regenerate fixtures.gen.ts
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

C="$ROOT/cockpit"
[ -d "$C/hooks" ] || { pass "801: this tree carries no cockpit mod"; exit 0; }

GEN='
import json, os, sys
F = sys.argv[1]
names = sorted(n for n in os.listdir(F) if n.endswith((".json", ".txt")))
out = ["// Generated from cockpit/fixtures/ by setup/tests/801-cockpit-mod-reads-the-state.sh --write.",
       "// A mod test cannot read files or import JSON, so the fixtures'"'"' bytes ride in here;",
       "// that test fails when this file and the fixtures disagree.", "",
       "export const FIXTURES: Record<string, string> = {"]
for n in names:
    out.append("  %s: %s," % (json.dumps(n), json.dumps(open(os.path.join(F, n)).read())))
out.append("}")
print("\n".join(out))
'

if [ "${1:-}" = --write ]; then
  python3 -I -c "$GEN" "$C/fixtures" > "$C/hooks/fixtures.gen.ts"
  pass "801: $C/hooks/fixtures.gen.ts regenerated"; exit 0
fi

python3 -I -c "$GEN" "$C/fixtures" | cmp -s - "$C/hooks/fixtures.gen.ts" \
  || fail "801: hooks/fixtures.gen.ts differs from cockpit/fixtures/ — run: bash setup/tests/801-cockpit-mod-reads-the-state.sh --write"
pass "801: fixtures.gen.ts carries the fixtures' bytes"

# Past this point the real claude is needed (the test HOME is empty on purpose).
command -v claude >/dev/null 2>&1 || { pass "801: claude is not installed — the mod is not exercised"; exit 0; }
ver="$(claude --version 2>/dev/null | rg -o '^[0-9]+\.[0-9]+\.[0-9]+' || true)"
[ -n "$ver" ] && [ "$(printf '%s\n2.1.289\n' "$ver" | sort -V | head -1)" = 2.1.289 ] \
  || { pass "801: claude ${ver:-?} is below 2.1.289 — the mod is not exercised"; exit 0; }

# The mod under test lives in a copy: a mutant must never touch the tree.
copy() { rm -rf "$1"; mkdir -p "$1"; cp -R "$C/.claude-plugin" "$C/hooks" "$C/schema.json" "$1/"; }

json="$TMP/validate.json"
claude plugin validate "$C" --json > "$json" 2>"$TMP/validate.err" || fail "801: claude plugin validate failed: $(head -c 400 "$json" "$TMP/validate.err")"
python3 -I - "$json" <<'PY' || fail "801: the mod's validation or call set is outside the read-only allowlist (see above)"
import json, re, sys
d = json.load(open(sys.argv[1]))
errs = d["manifest"]["errors"] + [e for c in d["contents"] for e in c["errors"]]
if errs or not d["success"]: print("  ✗ errors:", errs); sys.exit(1)
calls = set()
for c in d["contents"]:
    for n in c["notes"]:
        m = re.search(r"calls: (.*)$", n)
        if m: calls |= {x.strip().removeprefix("$.") for x in re.sub(r" \(via [^)]*\)", "", m.group(1)).split(", ")}
ALLOW = {"fs.stat", "fs.read", "fs.write", "fs.exists", "env.get", "clock.every", "clock.now", "command.register", "prompt.fill", "process.run"}
def ok(x): return x in ALLOW or x.startswith("ui.")
bad = sorted(x for x in calls if not ok(x))
if bad: print("  ✗ calls outside the allowlist:", bad); sys.exit(1)
print("  calls:", ", ".join(sorted(calls)))
PY
pass "801: validate has no error; the calls are inside the allowlist"

if rg -n '\$\.(http|model|session\.send|prompt\.submit|store|state|tool\.check)' "$C/hooks" --glob '!*.test.ts' >"$TMP/forbidden.txt" 2>&1; then
  fail "801: the mod source names a call it must not make: $(head -3 "$TMP/forbidden.txt")"
fi
# The one file the mod may write is the pilot's cockpit-actions.jsonl, from recordAction in register.tsx alone
# (its rotation and its append: two calls).
if rg -n '\$\.fs\.write' "$C/hooks" --glob '!*.test.ts' --glob '!register.tsx' >"$TMP/forbidden.txt" 2>&1; then
  fail "801: a hooks file other than register.tsx writes a file: $(head -3 "$TMP/forbidden.txt")"
fi
[ "$(rg -c '\$\.fs\.write' "$C/hooks/register.tsx")" = 2 ] || fail "801: register.tsx must write exactly the pilot's two lines (rotation and append)"
rg -q 'cockpit-actions\.jsonl' "$C/hooks/register.tsx" || fail "801: the mod's one write is not the pilot's cockpit-actions.jsonl"
pass "801: no write, network, model or store call in the source"

run_tests() { ( cd "$1" && claude plugin test . ) >"$2" 2>&1; }
copy "$TMP/base"
run_tests "$TMP/base" "$TMP/base.out" || fail "801: claude plugin test is red on the mod itself: $(tail -n 12 "$TMP/base.out")"
pass "801: claude plugin test passes ($(rg -o '[0-9]+ pass' "$TMP/base.out" | tail -1))"

# One mutant per line of $TMP/mutants: name @@ file @@ from @@ to. Each must turn
# the tests red, and each `from` must be in the file, or the mutant tests nothing.
cat > "$TMP/mutants" <<'M'
fresh window one second too long @@ hooks/state.ts @@ export const FRESH_MS = 15_000 @@ export const FRESH_MS = 16_000
dead window one second too long @@ hooks/state.ts @@ export const DEAD_MS = 60_000 @@ export const DEAD_MS = 61_000
dead treated as stale @@ hooks/state.ts @@ if (age > DEAD_MS) return { cls: 'dead', state: null, @@ if (age > DEAD_MS * 100) return { cls: 'dead', state: null,
future-dated accepted @@ hooks/state.ts @@ if (age < 0) return @@ if (age < -1e15) return
herdr.ok ignored @@ hooks/state.ts @@ if (!state.herdr.ok) return @@ if (false as boolean) return
schema ignored @@ hooks/state.ts @@ if (v.schema !== 1) return @@ if (false as boolean) return
missing file shown as an empty fresh state @@ hooks/state.ts @@ if (state === null) return { cls: 'dead', state: null, ageMs: null, why } @@ if (state === null) return { cls: 'fresh', ageMs: 0, why, state: { schema: 1, generated_at: now, herdr: { ok: true, error: null }, totals: { executors: 0, omitted: 0, working: 0, idle: 0, asks: 0, reports: 0, blocked: 0, challenges: 0, rulings_pending: 0 }, rules: { cut: { running: false, since_ms: null, holder: null } }, rows: [] } }
old rows kept when dead @@ hooks/state.ts @@ age > DEAD_MS) return { cls: 'dead', state: null, @@ age > DEAD_MS) return { cls: 'dead', state,
over-size file read @@ hooks/register.tsx @@ if (size > MAX_BYTES) { @@ if (size > MAX_BYTES * 1000) {
file read on every tick @@ hooks/register.tsx @@ if (S.loaded !== null && next === S.sig) return false @@ 
redraw on every tick @@ hooks/register.tsx @@ if (changed || m.cls !== S.lastClass || bucket !== S.lastBucket) { @@ if (true as boolean) {
the work root of the pane is ignored @@ hooks/register.tsx @@ (await $.env.get('HW_COCKPIT_WORK')) || @@ 
reply drawn without the field @@ hooks/view.tsx @@ typeof r.ask.pending_reply === 'string' @@ true
reply drawn on a stale state @@ hooks/view.tsx @@ if (!dim && r.ask !== null @@ if (r.ask !== null
reply drawn as dead text @@ hooks/view.tsx @@ <Input key={`reply:${r.id}`} label="reply " @@ <Text key={`reply:${r.id}`} label="reply "
M
n=0
while IFS= read -r line; do
  [ -n "$line" ] || continue
  copy "$TMP/m"
  name="$(python3 -I - "$TMP/m" "$line" <<'PY'
import sys
root, line = sys.argv[1:3]
name, file, a, b = [x.strip() for x in line.split(" @@ ")] if line.rstrip().endswith("@@") is False else [x.strip() for x in line.rstrip().rstrip("@").split(" @@ ")] + [""]
p = root + "/" + file
s = open(p).read()
if a not in s: sys.exit(1)
open(p, "w").write(s.replace(a, b, 1))
print(name)
PY
)" || fail "801: a mutant's target text is not in its file — it tests nothing: ${line%% @@*}"
  if run_tests "$TMP/m" "$TMP/m.out"; then fail "801: mutant '$name' survived: claude plugin test stayed green"; fi
  n=$((n + 1))
done < "$TMP/mutants"
pass "801: $n mutants of the mod, each killed by the tests"
