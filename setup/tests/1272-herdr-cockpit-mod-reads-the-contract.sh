#!/usr/bin/env bash
# The herdr app's cockpit mod (herdr-app/mods/foreman-cockpit) reads the cockpit
# state through its contract, cockpit/schema.json v1, and the contract ships a
# table of what a reader must make of each example file: cockpit/fixtures/index.json
# names the class and the number of drawn rows per fixture, including a missing
# file, a >1 MiB file and a file from an older writer.
#
# The mod's `state.js` is the reader and imports nothing from the app, so it runs
# under plain node: this subject pins the mod to that table. Without it, the mod
# could drift from the writer silently — schema.json is enforced on the writer,
# and a reader that draws a dead file as fresh has no other gate.
#
# Run alone while working on this subject:
#     bash setup/tests/1272-herdr-cockpit-mod-reads-the-contract.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

READER="$ROOT/herdr-app/mods/foreman-cockpit/state.js"
[ -f "$READER" ] || fail "the mod's reader is missing: $READER"

command -v node >/dev/null || { pass "node is not installed here: the reader's check is skipped"; exit 0; }
# state.js is an ES module in a .js file, and an .mjs importer only loads it on a
# node with module-syntax detection (22.7+, 20.19+). Older node is a loud skip,
# not a red on a SyntaxError that says nothing about the reader.
node -e 'const [a, b] = process.versions.node.split(".").map(Number); process.exit(a > 22 || (a === 22 && b >= 7) || (a === 20 && b >= 19) ? 0 : 1)' ||
  { pass "node $(node -v) has no module-syntax detection: the reader's check is skipped"; exit 0; }

out="$(node "$ROOT/herdr-app/mods/foreman-cockpit/state.test.mjs" 2>&1)" || fail "the mod disagrees with cockpit/fixtures/index.json: $out"
pass "$out"

# Every other mod's own node test (a mod's test with no runner of its own).
# A mod the export excludes is simply absent there, so the glob skips it.
for t in "$ROOT"/herdr-app/mods/*/*.test.mjs; do
  [ -f "$t" ] && [ "$t" != "$ROOT/herdr-app/mods/foreman-cockpit/state.test.mjs" ] || continue
  out="$(node "$t" 2>&1)" || fail "${t#"$ROOT"/} failed: $out"
  pass "${t#"$ROOT"/}: $out"
done
