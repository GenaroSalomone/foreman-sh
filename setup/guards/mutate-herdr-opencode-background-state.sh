#!/usr/bin/env bash
# Mutate copied managed-plugin sources only; never touch the live integration.
set -euo pipefail
ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# The frozen patched v12 plugin, not the live one: a herdr update is a machine event, and
# setup/check-machine is what checks the live file.
REAL="${OPENCODE_HERDR_STATE_PLUGIN:-$ROOT/setup/fixtures/herdr-agent-state.js}"
ART="${HW_ARTIFACTS:?HW_ARTIFACTS is required}"
WORK="$ART/herdr-opencode-background-mutants"
rm -rf "$WORK"; mkdir -p "$WORK"
killed=0; survived=0
mutant() {
  local name="$1" from="$2" to="$3" file="$WORK/$1.mjs"
  cp "$REAL" "$file"
  python3 - "$file" "$from" "$to" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text(); old, new = sys.argv[2:]
if s.count(old) != 1: raise SystemExit(f"expected one mutation site, found {s.count(old)}")
p.write_text(s.replace(old, new, 1))
PY
  if MANAGED_SRC="$file" node "$ROOT/setup/guards/test-herdr-opencode-background-state.mjs" >"$WORK/$name.txt" 2>&1; then
    printf 'not ok - mutant %s survived\n' "$name"; survived=$((survived + 1))
  else
    printf 'ok - mutant %s killed\n' "$name"; killed=$((killed + 1))
  fi
}
mutant child-create-not-working $'childSessions.set(info.id, info.parentID);\n        await reportLifecycleState();\n        return;' $'childSessions.set(info.id, info.parentID);\n        await reportState("idle");\n        return;'
mutant child-completion-never-removes 'childSessions.delete(sessionID);' '// child kept live'
mutant child-completion-update-resurrects 'if (completedChildSessions.has(sessionID)) return;' '// completed child may be re-added'
mutant child-question-not-blocked 'blockingSessions.add(sessionID);' '// child question ignored'
mutant root-idle-ignores-liveness 'if (state === "idle") await reportLifecycleState();' 'if (state === "idle") await reportState("idle", sessionID);'
mutant root-idle-ignores-question $'case "session.idle":\n          await reportLifecycleState();' $'case "session.idle":\n          await reportState("idle", sessionID);'
printf '\n%d killed, %d survived\n' "$killed" "$survived"
[ "$survived" -eq 0 ]
