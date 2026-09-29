#!/usr/bin/env python3
"""Lane shim: the brain ROOT's read-only guard.

THE LOGIC IS NOT HERE. It is in `setup/guards/deny_repo_writes.py`, once, for
all four lanes and both runtimes — read that file's docstring for why eight
copies became two and what the 11.5 KB of drift between them actually was.

WHY THE ROOT IS A LANE AT ALL. Claude Code resolves the project directory
from the cwd, not from the git root: a session started in a lane's directory
loads that lane's settings and is blocked writing into a product repository,
while the same command from the brain root would be ALLOWED if the root carried
no settings and no hook.

THIS FILE STAYS AT THIS PATH because `.claude/settings.json` registers the hook
as `$CLAUDE_PROJECT_DIR/.claude/hooks/deny-repo-writes.py`. That settings file
is the guard's OTHER half — its `permissions.deny` list covers `Edit()`/`Write()`,
which this hook cannot see, and `0c51247` emptied it once for three days. Do not
edit it as a side effect of touching this.

The lane name below is the only thing this file decides. Everything the lane
configures — protected roots, prose, exemptions — lives in the shared table.

AND THE IMPORT IS GUARDED, which is the whole cost of having a shared module.
Measured 2026-09-06 on the first draft of this shim: with the shared module
absent, `from deny_repo_writes import main` raised ModuleNotFoundError, python
exited 1, and Claude Code treats a PreToolUse hook exiting non-zero with
anything other than 2 as a NON-BLOCKING error — so the write went through with
no deny anywhere. Eight self-contained copies could not fail that way; two
shared ones can, and the failure is silent. So an unusable module DENIES, in
the only shape Claude Code honours: the deny JSON, on stdout, exit 0.
"""
import json
import pathlib
import sys

LANE = "brain"


def _deny(reason):
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": reason,
        }
    }))
    raise SystemExit(0)


try:
    sys.path.insert(
        0, str(pathlib.Path(__file__).resolve().parents[2] / "setup" / "guards"))
    from deny_repo_writes import main
except Exception as exc:  # noqa: BLE001 — ANY failure here must deny, not crash
    # Only Bash is this hook's business; anything else is none of it, and
    # refusing it would be a regression on top of a broken import.
    try:
        payload = json.load(sys.stdin)
    except Exception:
        raise SystemExit(0)
    if payload.get("tool_name") != "Bash":
        raise SystemExit(0)
    _deny(
        "Blocked: the brain-root read-only guard could not load its shared module "
        "(setup/guards/deny_repo_writes.py): %s: %s. Every Bash command is "
        "refused until it loads, because a guard that is not running cannot "
        "tell a protected tree from any other directory."
        % (type(exc).__name__, exc))

if __name__ == "__main__":
    main(LANE)
