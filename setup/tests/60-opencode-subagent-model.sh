#!/usr/bin/env bash
# Every declared OpenCode subagent must resolve its own model instead of inheriting the primary.
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
    print(f"not ok - OpenCode subagent models: cannot read valid config {path}: {error}")
    raise SystemExit(1)

missing = []
invalid = []
for name, agent in sorted(config.get("agent", {}).items()):
    if agent.get("mode") != "subagent":
        continue
    model = agent.get("model")
    if not isinstance(model, str) or not model.strip():
        missing.append(name)
    elif "/" not in model or model.startswith("/") or model.endswith("/"):
        invalid.append(f"{name}={model!r}")

if missing:
    print("not ok - OpenCode subagent models: missing explicit model: " + ", ".join(missing))
if invalid:
    print("not ok - OpenCode subagent models: invalid provider/model: " + ", ".join(invalid))
if missing or invalid:
    raise SystemExit(1)

print(f"ok - OpenCode subagent models: {sum(1 for agent in config.get('agent', {}).values() if agent.get('mode') == 'subagent')} declared subagents pin provider/model")
PY
)" || {
  printf '%s\n' "$out" >&2
  exit 1
}
printf '%s\n' "$out"

# THE CHECKER MUST BE ABLE TO SAY NO. Against the live config the only failure
# this file could ever show was a real drift; against a fixture that always
# passes, a checker that accepts anything would stay green. So one subagent
# loses its model on a copy, and another gets a provider-less one, and each must
# be named for its own reason.
# This block judges the fixture only: it hardcodes explore-luna/general-terra,
# which a valid live config is free to rename or drop, so it must not run when
# OPENCODE_CONFIG_FILE is set (check-machine's live-config pass, and the
# recursive child run below, which sets it too and so is covered by the same
# check).
[ -n "${OPENCODE_CONFIG_FILE:-}" ] && exit 0
MUT="$TMP/opencode-mutant.json"
python3 - "$CONFIG" "$MUT" <<'PY'
import json, sys
c = json.load(open(sys.argv[1], encoding="utf-8"))
c["agent"]["explore-luna"].pop("model")
c["agent"]["general-terra"]["model"] = "gpt-5.6-terra"
json.dump(c, open(sys.argv[2], "w", encoding="utf-8"))
PY
mut_out="$(OPENCODE_MODEL_MUTANT_RUN=1 OPENCODE_CONFIG_FILE="$MUT" bash "$0" 2>&1 || true)"
saw_mutant "M01 a subagent with no model is named" "$mut_out" "missing explicit model: explore-luna"
saw_mutant "M02 a model with no provider is named" "$mut_out" "invalid provider/model: general-terra='gpt-5.6-terra'"
