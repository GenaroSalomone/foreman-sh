#!/usr/bin/env bash
# A framework-free Sol primary orchestrates pinned economical workers; the
# direct worker and every SDD phase keep their deliberately inline contracts.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# THE SUBJECT IS A FIXTURE, NOT THIS MACHINE'S CONFIG (since 2026-09-22). The
# live ~/.config/opencode/opencode.json is checked by setup/check-machine, which
# runs this same file with OPENCODE_CONFIG_FILE pointed at it; the suite judges
# the policy against setup/fixtures/opencode.json, so editing the live config
# can no longer turn it red.
CONFIG="${OPENCODE_CONFIG_FILE:-$FIXTURES/opencode.json}"

out="$(python3 - "$CONFIG" <<'PY'
import json
import sys

path = sys.argv[1]
try:
    with open(path, encoding="utf-8") as handle:
        config = json.load(handle)
except (OSError, json.JSONDecodeError) as error:
    print(f"not ok - Sol orchestrator policy: cannot read valid config {path}: {error}")
    raise SystemExit(1)

agents = config.get("agent", {})
sol = agents.get("sol-orchestrator", {})
direct = agents.get("direct-worker", {})
general = agents.get("general", {})

def require(condition, message):
    if not condition:
        print("not ok - " + message)
        raise SystemExit(1)

require(sol.get("mode") == "primary", "sol-orchestrator is not a primary")
# Delegation comes from permission.task below. A legacy `tools: {edit: true}`
# appends `edit * allow` LAST and voids every edit deny (test 552), so the
# primary must not grant tools that way.
require(sol.get("tools", {}).get("task") is not False, "sol-orchestrator cannot delegate")

task_policy = sol.get("permission", {}).get("task", {})
allowed = {name for name, action in task_policy.items() if action == "allow"}
expected = {
    "explore-luna", "explore-terra", "general-luna", "general-terra", "general-sol",
    "jd-fix-agent", "jd-judge-a", "jd-judge-b",
}
require(task_policy.get("*") == "deny", "sol-orchestrator task policy is not default-deny")
require(not any(name.startswith("sdd-") for name in allowed), "an SDD phase can run as a Sol Task")
require(allowed == expected, f"sol-orchestrator allowlist drifted: {sorted(allowed)}")

prompt = sol.get("prompt", "")
for phrase in ("background: true", "result is not needed for the next step", "Use Luna", "Use Terra", "Use Sol subagents only when strictly necessary", "general-sol", "demonstrably exceeds Terra", "SDD phase agents"):
    require(phrase in prompt, f"sol-orchestrator prompt omits policy phrase: {phrase}")

require(direct.get("mode") == "primary", "direct-worker lost primary mode")
require("direct executor, not an orchestrator" in direct.get("prompt", ""), "direct-worker lost its inline contract")
require(general.get("mode") == "subagent" and general.get("hidden") is True, "general became a visible primary")
require(general.get("tools", {}).get("task") is False, "general can now delegate")

for name in expected:
    agent = agents.get(name, {})
    require(agent.get("mode") == "subagent", f"allowed worker {name} is not a subagent")
    model = agent.get("model", "")
    require(model.startswith("openai/gpt-5.6-"), f"allowed worker {name} lacks an explicit tier model")

require(agents.get("general-sol", {}).get("tools", {}).get("task") is False, "general-sol can recursively delegate")

print("ok - C01 sol-orchestrator is a primary whose task tool is not disabled")
print("ok - C02 its task policy is default-deny and names pinned economical workers plus bounded Sol exceptions")
print("ok - C03 its prompt defines tier choice and dependency-based background use")
print("ok - C04 SDD phase agents are excluded from its task permission")
print("ok - C05 direct-worker keeps its inline primary contract")
print("ok - C06 general remains a hidden non-delegating subagent")
print("ok - C07 general-sol makes the strictly-necessary non-JD exception executable")
PY
)" || {
  printf '%s\n' "$out" >&2
  exit 1
}
printf '%s\n' "$out"

# Mutation: allowing an SDD phase must make the foreground-only guard fail for
# its own named reason, not because the config became unreadable.
MUTANT="$TMP/opencode-mutant.json"
python3 - "$CONFIG" "$MUTANT" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    config = json.load(handle)
config["agent"]["sol-orchestrator"]["permission"]["task"]["sdd-apply"] = "allow"
with open(sys.argv[2], "w", encoding="utf-8") as handle:
    json.dump(config, handle)
PY

mut_out="$(OPENCODE_CONFIG_FILE="$MUTANT" bash "$0" 2>&1 || true)"
saw_mutant "M01 SDD phases remain outside Sol Task routing" "$mut_out" "an SDD phase can run as a Sol Task"

printf 'mapping - C01 primary/tools · C02 allowlist/models · C03 prompt policy · C04↔M01 SDD foreground boundary · C05 direct role · C06 general role · C07 general-sol exception\n'
printf 'coverage - 7 behavior claims, 1 dedicated policy mutant killed\n'
