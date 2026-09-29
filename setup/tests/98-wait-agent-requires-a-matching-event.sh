#!/usr/bin/env bash
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
python3 "$ROOT/setup/guards/probe-wait-agent.py" "${HW_SOURCE:-$ROOT/bin/herdr-rpc}"
