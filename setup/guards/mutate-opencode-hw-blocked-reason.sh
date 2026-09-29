#!/usr/bin/env bash
# Mutate adapter copies only; tracked source is never edited in place.
set -u

ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REAL="$ROOT/setup/opencode-hw-blocked-reason.js"
ART="${HW_ARTIFACTS:-$(mktemp -d "${TMPDIR:-/tmp}/hw-blocked-mutants.XXXXXX")}"
WORK="$ART/opencode-hw-blocked-reason-mutants"
OWN_ART=0
[ -n "${HW_ARTIFACTS:-}" ] || OWN_ART=1
rm -rf "$WORK"
mkdir -p "$WORK"
trap '[ "$OWN_ART" -eq 0 ] || rm -rf "$ART"' EXIT
killed=0
survived=0

mutant() {
  local name="$1" from="$2" to="$3" file="$WORK/$1.mjs" rc
  cp "$REAL" "$file"
  python3 - "$file" "$from" "$to" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
s = p.read_text()
old, new = sys.argv[2:]
if s.count(old) != 1:
    raise SystemExit(f"expected one mutation site, found {s.count(old)}")
p.write_text(s.replace(old, new, 1))
PY
  rc=$?
  if [ "$rc" -ne 0 ]; then
    printf 'not ok - mutant %s mutation site missing\n' "$name"
    survived=$((survived + 1))
  elif ADAPTER_SRC="$file" node "$ROOT/setup/guards/test-opencode-hw-blocked-reason.mjs" >"$WORK/$name.txt" 2>&1; then
    printf 'not ok - mutant %s survived\n' "$name"
    survived=$((survived + 1))
  else
    printf 'ok - mutant %s killed\n' "$name"
    killed=$((killed + 1))
  fi
}

mutant child-scope-never-detected 'const scope = sessionID && childSessions.has(sessionID) ? "child" : "root";' 'const scope = "root";'
mutant child-error-published 'if (type === "session.error" && scope === "child") return;' ''
mutant settle-always-clears 'if (status === "blocked") {' 'if (false) {'
mutant settle-always-stuck $'if (status === undefined) return; // could not ask; leave the last word standing\n  await publish(CLEAR);' 'await publish({ blocked_reason: "stuck", blocked_scope: scope });'
mutant root-idle-publishes-stuck 'await settle("root", false);' 'await settle("root", true);'
mutant clears-with-prompts-outstanding 'if (outstanding.size > 0) {' 'if (false) {'
mutant child-outranks-root 'if (!best || (best.scope === "child" && entry.scope === "root")) best = entry;' 'best = entry;'
mutant runs-outside-herdr 'if (!isHerdrPane()) return {};' ''
mutant permission-not-blocking '["permission.asked", "permission"],' ''

printf '\n%d killed, %d survived\n' "$killed" "$survived"
[ "$survived" -eq 0 ]
