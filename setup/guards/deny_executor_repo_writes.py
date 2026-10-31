#!/usr/bin/env python3
"""The executor's half of the repo-write guard: a claude executor does not write in a product repo it is not working in.

`deny_repo_writes` keeps every BRAINER read-only over the product repos, and loads by directory: under `brain/<lane>`.
An executor runs in a work directory or in a product worktree, loads none of that, and until 2026-10-30 nothing else
stood between it and another lane's repo (probed with a real claude run from a repo-less work directory: a `touch`
into a product checkout landed). This is the missing half, registered the way the brain guard is: `hw` passes
`--plugin-dir setup/guards/brain-guard` to every claude executor whose lane does not say `brain_guard: false`, whatever
the lane's shape, and that plugin's hooks.json runs this file beside deny_brain_writes.py.

WHAT IT PROTECTS: every `product_repos` root of guards.json, by the same detection `decide` gives a brainer (Bash:
redirects, rm, in-place edits, git mutations, interpreters), plus the Write/Edit tools by path.

WHAT IT LEAVES ALONE: a root that holds the executor's own work directory (`HW_WORKDIR`, set by hw on every executor):
an executor of a product lane writes in its repo's worktree, a repo-less one in its work directory. A product-repo
executor may therefore also write its own repo's checkout root; that is the stated limit, not a goal. Without
`HW_WORKDIR` nothing is left alone.

FAILS CLOSED: an import failure, a crash while deciding and an unreadable payload all deny (the hook command also ends in
`|| exit 2`).
"""
import json
import os
import sys

try:
    sys.path.insert(0, os.path.dirname(os.path.realpath(__file__)))
    import deny_repo_writes as drw  # noqa: E402
    LOAD_ERROR = None
except Exception as exc:  # noqa: BLE001 - ANY failure here must deny
    drw, LOAD_ERROR = None, exc

WRITE_TOOLS = {"Write": "file_path", "Edit": "file_path", "MultiEdit": "file_path", "NotebookEdit": "notebook_path"}


def deny(reason):
    print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny",
                                             "permissionDecisionReason": reason}}))
    sys.exit(0)


def executor_config(workdir):
    """The brain lane's config with `roots` cut to the product repos that do not hold `workdir`."""
    cfg = drw.config("brain")
    home = None
    if workdir:
        try:
            home = drw._real(workdir)
        except OSError:
            home = drw._canon(workdir)
    roots = []
    for root in drw.PRODUCT_REPOS:
        for cand in {root, drw._real(root)}:
            if home and (home == cand or home.startswith(cand + "/")):
                break
        else:
            roots.append(root)
    cfg["roots"] = tuple(roots)
    cfg["resolved_roots"] = drw._resolve_roots(cfg["roots"])
    cfg["root_ids"] = drw._root_ids(cfg["roots"])
    cfg["worktree_roots"] = ()
    cfg["where"] = "a product repo this executor is not working in"
    cfg["write_here"] = "your own worktree or work directory"
    cfg["git_tail"] = ("Only the executor of that repo changes its state, in its own worktree. Read-only git "
                       "(log/status/diff/show/rev-list) is allowed.")
    return cfg


def main(stream=None):
    try:
        payload = json.load(stream or sys.stdin)
    except Exception:
        sys.exit(0)  # not JSON: it names no tool
    if LOAD_ERROR is not None:
        if isinstance(payload, dict) and payload.get("tool_name") in ("Bash", *WRITE_TOOLS):
            deny("Blocked: the executor repo-write guard could not load its policy (%s: %s). It reached no verdict, "
                 "and a guard that reached no verdict has not established that this is safe."
                 % (type(LOAD_ERROR).__name__, LOAD_ERROR))
        sys.exit(0)
    if not isinstance(payload, dict):
        deny("Blocked: the executor repo-write guard was handed a payload that is not a JSON object (%s). Refused "
             "rather than guessed." % type(payload).__name__)
    tool = payload.get("tool_name", "")
    if tool != "Bash" and tool not in WRITE_TOOLS:
        sys.exit(0)
    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        deny("Blocked: the executor repo-write guard was handed a %s call whose tool_input is not an object (%s). "
             "Refused rather than guessed." % (tool, type(tool_input).__name__))
    cwd = payload.get("cwd", "") or ""
    try:
        cfg = executor_config(os.environ.get("HW_WORKDIR", ""))
        if tool == "Bash":
            verdict = drw.decide(cfg, tool_input.get("command", "") or "", cwd)
            if verdict:
                deny(verdict[1])
        else:
            path = tool_input.get(WRITE_TOOLS[tool], "") or ""
            if path and not drw._is_abs(path):
                path = os.path.join(cwd, path)
            if path and drw._inside_any(cfg, path):
                deny("Blocked: %s writes %s, inside a product repo this executor is not working in. Write in your "
                     "own worktree or work directory." % (tool, path))
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 - ANY failure here must deny
        deny("Blocked: the executor repo-write guard CRASHED while deciding (%s: %s). It reached no verdict, and a "
             "guard that reached no verdict has not established that this is safe." % (type(exc).__name__, exc))
    sys.exit(0)


if __name__ == "__main__":
    main()
