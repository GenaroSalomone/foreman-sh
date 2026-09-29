#!/usr/bin/env bash
# Every declared OpenCode GPT subagent must pin deliberate reasoning effort.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# THE SUBJECT IS A FIXTURE, NOT THIS MACHINE'S CONFIG (since 2026-09-22). The
# live ~/.config/opencode/opencode.json is checked by setup/check-machine, which
# runs this same file with OPENCODE_CONFIG_FILE pointed at it; the suite judges
# the policy against setup/fixtures/opencode.json, so editing the live config
# can no longer turn it red.
CONFIG="${OPENCODE_CONFIG_FILE:-$FIXTURES/opencode.json}"

out="$(python3 - "$CONFIG" <<'PY'
import json
import shutil
import sys
import tempfile
from pathlib import Path

path = Path(sys.argv[1])
allowed = {"none", "low", "medium", "high", "xhigh", "max"}

def policy_effort(name, model):
    tier = model.rsplit("-", 1)[-1]
    if tier == "luna":
        return "max"
    if tier == "terra":
        return "high" if name.startswith("review-") or name == "sdd-verify" else "medium"
    if tier == "sol":
        return "max" if name.startswith("jd-") or name == "review-refuter" else "high"
    return None

def load(subject):
    try:
        with subject.open(encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot read valid config {subject}: {error}") from error

def validate(subject):
    config = load(subject)
    agents = config.get("agent", {})
    gpt = {
        name: agent for name, agent in agents.items()
        if agent.get("mode") == "subagent"
        and isinstance(agent.get("model"), str)
        and agent["model"].startswith("openai/gpt-")
    }
    errors = []
    for name, agent in sorted(gpt.items()):
        effort = agent.get("reasoningEffort")
        if not isinstance(effort, str) or not effort.strip():
            errors.append(f"missing reasoningEffort: {name}")
        elif effort not in allowed:
            errors.append(f"invalid reasoningEffort: {name}={effort!r}")
            continue
        expected = policy_effort(name, agent["model"])
        if expected is None:
            errors.append(f"unsupported GPT model tier: {name}={agent['model']!r}")
        elif effort != expected:
            errors.append(f"policy reasoningEffort: {name}={expected!r}, got {effort!r}")
    return errors

def write_mutation(subject, mutate):
    config = load(subject)
    mutate(config)
    with subject.open("w", encoding="utf-8") as handle:
        json.dump(config, handle)

try:
    errors = validate(path)
except ValueError as error:
    print(f"not ok - OpenCode subagent effort: {error}")
    raise SystemExit(1)
if errors:
    print("not ok - OpenCode subagent effort: " + "; ".join(errors))
    raise SystemExit(1)

# The probes name agents the live config need not carry, so they run on a copy
# that declares them: the live config is validated above, the mutations below.
probe_agents = {
    "explore": {"mode": "subagent", "model": "openai/gpt-5.6-terra", "reasoningEffort": "medium"},
    "jd-judge-a": {"mode": "subagent", "model": "openai/gpt-5.6-sol", "reasoningEffort": "max"},
    "explore-luna": {"mode": "subagent", "model": "openai/gpt-5.6-luna", "reasoningEffort": "max"},
}

with tempfile.TemporaryDirectory() as directory:
    base = Path(directory) / "base.json"
    config = load(path)
    config.setdefault("agent", {}).update(probe_agents)
    with base.open("w", encoding="utf-8") as handle:
        json.dump(config, handle)
    path = base
    fixture = Path(directory) / "opencode.json"
    shutil.copyfile(path, fixture)

    write_mutation(fixture, lambda config: config["agent"]["explore"].pop("reasoningEffort"))
    if "missing reasoningEffort: explore" not in validate(fixture):
        print("not ok - OpenCode subagent effort: mutation did not name missing explore effort")
        raise SystemExit(1)

    shutil.copyfile(path, fixture)
    write_mutation(fixture, lambda config: config["agent"]["explore"].__setitem__("reasoningEffort", "invalid"))
    if "invalid reasoningEffort: explore='invalid'" not in validate(fixture):
        print("not ok - OpenCode subagent effort: mutation did not name invalid explore='invalid'")
        raise SystemExit(1)

    shutil.copyfile(path, fixture)
    write_mutation(fixture, lambda config: config["agent"]["jd-judge-a"].__setitem__("reasoningEffort", "high"))
    if "policy reasoningEffort: jd-judge-a='max', got 'high'" not in validate(fixture):
        print("not ok - OpenCode subagent effort: mutation did not reject jd-judge-a max-to-high flattening")
        raise SystemExit(1)

    shutil.copyfile(path, fixture)
    write_mutation(fixture, lambda config: config["agent"]["explore-luna"].__setitem__("model", "openai/gpt-5.6-terra"))
    if "policy reasoningEffort: explore-luna='medium', got 'max'" not in validate(fixture):
        print("not ok - OpenCode subagent effort: mutation did not bind effort to the configured model tier")
        raise SystemExit(1)

count = sum(
    agent.get("mode") == "subagent" and isinstance(agent.get("model"), str)
    and agent["model"].startswith("openai/gpt-")
    for agent in load(Path(sys.argv[1])).get("agent", {}).values()
)
print(f"ok - OpenCode GPT subagent reasoning effort: {count} agents match the documented tier policy")
PY
)" || {
  printf '%s\n' "$out" >&2
  exit 1
}
printf '%s\n' "$out"
