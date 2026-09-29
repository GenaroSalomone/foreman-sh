#!/usr/bin/env bash
# Repair/check the managed Herdr OpenCode integration's background-child state.
#
# Why this exists: `herdr integration install opencode` replaces the managed
# plugin. The tracked repair below is intentionally exact and fail-closed: a
# changed upstream v12 shape is not silently patched. `--check` is called by
# setup/test-hw against the LIVE installed file, so a reinstall removes the
# marker and fails the baseline before a false-idle executor can ship.
set -euo pipefail

FILE="${OPENCODE_HERDR_STATE_PLUGIN:-$HOME/.config/opencode/plugins/herdr-agent-state.js}"
MODE="${1:---check}"

is_patched() {
  [ -f "$FILE" ] && grep -Fq 'BACKGROUND_CHILD_LIFECYCLE_V1' "$FILE" \
    && grep -Fq 'const blockingSessions = new Set();' "$FILE" \
    && grep -Fq 'const completedChildSessions = new Set();' "$FILE" \
    && grep -Fq 'function reportLifecycleState()' "$FILE" \
    && grep -Fq 'info?.id === sessionID' "$FILE"
}

case "$MODE" in
  --check)
    is_patched || {
      printf 'not ok - OpenCode Herdr background-child patch missing: %s\n' "$FILE" >&2
      printf 'repair with: %s --apply\n' "$0" >&2
      exit 1
    }
    printf 'ok - OpenCode Herdr background-child patch is installed: %s\n' "$FILE"
    ;;
  --apply)
    if is_patched; then
      printf 'ok - OpenCode Herdr background-child patch already installed: %s\n' "$FILE"
      exit 0
    fi
    python3 - "$FILE" <<'PY'
from pathlib import Path
import re
import sys

p = Path(sys.argv[1])
s = p.read_text()
replacements = [
    ("// HERDR_INTEGRATION_VERSION=12\n", """// HERDR_INTEGRATION_VERSION=12
// BACKGROUND_CHILD_LIFECYCLE_V1
//
// This managed integration is intentionally patched by this harness's setup lane.
// The canonical, reproducible repair and drift check live at:
//   <brain>/setup/install-opencode-background-state.sh
// `herdr integration install opencode` overwrites this file; `--check` must
// fail immediately afterward until `--apply` repairs it.
"""),
    ("const childSessions = new Map();\n", """const childSessions = new Map();
// A root may become idle while a background child still runs. Child creation
// and completion are the durable lifecycle boundary; child status chatter is
// not. Real human prompts outrank child liveness so `working` never masks a
// question or permission modal.
// OpenCode emits a final session.updated after a child's session.idle. Keep
// completed ids for this plugin lifetime so that post-completion summary event
// cannot resurrect a child that was already removed.
const completedChildSessions = new Set();
const blockingSessions = new Set();
"""),
    ("""  return request("pane.report_agent", params);
}

export const HerdrAgentStatePlugin""", """  return request("pane.report_agent", params);
}

function reportLifecycleState() {
  if (blockingSessions.size > 0) return reportState("blocked");
  if (childSessions.size > 0) return reportState("working");
  return reportState("idle");
}

export const HerdrAgentStatePlugin"""),
    ("""      if (info?.id && info.parentID) {
        childSessions.set(info.id, info.parentID);
      }
      if (sessionID && childSessions.has(sessionID)) {
        const state = CHILD_EVENT_STATES.get(type);
        if (state) {
          let rootSessionID = sessionID;
          while (childSessions.has(rootSessionID)) {
            rootSessionID = childSessions.get(rootSessionID);
          }
          await reportState(state, rootSessionID);
        }
        return;
      }
""", """      if (
        (type === "session.created" || type === "session.updated") &&
        info?.id === sessionID &&
        info.parentID
      ) {
        if (completedChildSessions.has(sessionID)) return;
        childSessions.set(info.id, info.parentID);
        await reportLifecycleState();
        return;
      }
      if (sessionID && childSessions.has(sessionID)) {
        if (type === "session.idle" || type === "session.deleted") {
          childSessions.delete(sessionID);
          completedChildSessions.add(sessionID);
          blockingSessions.delete(sessionID);
          await reportLifecycleState();
          return;
        }
        if (type === "permission.asked" || type === "question.asked") {
          blockingSessions.add(sessionID);
          await reportLifecycleState();
          return;
        }
        if (type === "permission.replied" || type === "question.replied" || type === "question.rejected") {
          blockingSessions.delete(sessionID);
          await reportLifecycleState();
        }
        return;
      }
"""),
    ("""          if (state) {
            await reportState(state, sessionID);
          } else {
""", """          if (state) {
            if (state === "idle") await reportLifecycleState();
            else await reportState(state, sessionID);
          } else {
"""),
    ("""        case "session.compacted":
          await reportState("working", sessionID);
          break;
        case "permission.asked":
        case "question.asked":
        case "session.error":
          await reportState("blocked", sessionID);
          break;
        case "session.idle":
          await reportState("idle", sessionID);
""", """        case "session.compacted":
          blockingSessions.delete(sessionID);
          if (childSessions.size > 0 || blockingSessions.size > 0) await reportLifecycleState();
          else await reportState("working", sessionID);
          break;
        case "permission.asked":
        case "question.asked":
        case "session.error":
          blockingSessions.add(sessionID ?? type);
          await reportLifecycleState();
          break;
        case "session.idle":
          await reportLifecycleState();
"""),
]
for index, (old, new) in enumerate(replacements, start=1):
    if s.count(old) != 1:
        raise SystemExit(f"refusing unmanaged upstream shape at patch site {index}: expected one exact match, found {s.count(old)}")
    s = s.replace(old, new)
p.write_text(s)
PY
    "$0" --check
    ;;
  *)
    printf 'usage: %s [--check|--apply]\n' "$0" >&2
    exit 2
    ;;
esac
