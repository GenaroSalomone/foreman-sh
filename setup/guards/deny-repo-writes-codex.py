#!/usr/bin/env python3
"""The read-only guard for Codex, on the PreToolUse point that already exists.

WHAT WAS TRUE BEFORE THIS FILE, measured 2026-09-06 and recorded in
`setup/decisions.md` ("Codex corre sin sandbox"):

  * `~/.codex/config.toml` carries `sandbox_mode = "danger-full-access"` and
    `approval_policy = "never"`, so nothing in Codex's own sandboxing intercepts
    a write.
  * `~/.codex/hooks.json` had exactly one `PreToolUse` hook —
    `block-keychain-secret-read.py` — and nothing playing the role of
    `deny-repo-writes` for any of the four lanes.

So this was a guard that was MISSING, not a capability that was ABSENT: the hook
point works, and the keychain hook proves it live. This file fills it.

HOW IT DIFFERS FROM THE OTHER TWO VENDORS, and it is the only interesting part.
Claude Code registers the guard in each lane's `.claude/settings.json`, and
opencode in each lane's `.opencode/plugin/` — both are per-directory, so the
guard loads for a BRAINER standing in `~/brain/<lane>` and does not
load for an EXECUTOR in its worktree. That distinction is the whole design:
executors write, that is their job.

`~/.codex/hooks.json` is GLOBAL. It loads for every Codex session there is. So
the scoping the other two vendors get from the filesystem has to be done here,
explicitly: resolve the session's directory, and guard only when it is inside a
brain lane. Outside one — a worktree, a repo checkout, anywhere else — this
exits 0 and stays out of the way, which is exactly what the per-directory
registration does for the other two.

THE REFUSAL PROTOCOL IS CODEX'S, NOT CLAUDE'S. Claude Code's PreToolUse takes a
JSON `permissionDecision` on stdout and exit 0. Codex blocks on EXIT 2 with the
reason on stderr — the shape `block-keychain-secret-read.py` already uses on
this machine. Emitting Claude's JSON here would print a blob and allow the
command.

THE PATCH TOOL IS GUARDED TOO (captured 2026-10-01, codex-cli 0.153.4, with a
logging PreToolUse hook in a throwaway `CODEX_HOME`): Codex sends `apply_patch`
through PreToolUse as

    {"tool_name": "apply_patch", "tool_input": {"command": "*** Begin Patch\n*** Add File: <path>\n+hi\n*** End Patch"}, "cwd": ...}

so the targets are the `*** Add File:`, `*** Update File:`, `*** Delete File:`
and `*** Move to:` headers. A patch that lands one inside a protected tree is
refused with exit 2, in the spellings the tests drive (indented header, `~`, `..`, relative to
the payload cwd; symlinks through the shared module). The hook only sees it where `~/.codex/hooks.json` registers a
matcher for it (`apply_patch`, with `Edit`/`Write` as aliases of the same tool).
"""
import json
import os
import pathlib
import re
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

# WHERE BRAIN IS COMES FROM THE SAME POLICY AS WHAT IT PROTECTS (`guards.json`,
# `brain_root`), and so does the answer when that policy cannot be read. The
# shared module loads it at import. This import used to be bare, and a module
# that failed to import made this hook exit 1, which is not Codex's refusal
# (exit 2 is). A guard that cannot load cannot tell a brainer from an executor
# either, because it no longer knows where brain is, so `main` refuses every
# Bash call in every Codex session until it loads. Loud, and closed.
try:
    from deny_repo_writes import BRAIN_ROOT as BRAIN, LANES, config, decide  # noqa: E402
    # The shared module's path spelling (identity off Windows; see `_canon`).
    from deny_repo_writes import _is_abs, _norm, _real  # noqa: E402
    LOAD_ERROR = None
except Exception as exc:  # noqa: BLE001 — ANY failure here must refuse
    BRAIN, LANES, config, decide = None, {}, None, None
    LOAD_ERROR = exc


def lane_for(directory):
    """The brain lane `directory` sits in, or None.

    `..` and symlinks are resolved first for the same reason every other
    boundary question in this guard resolves: a lane reached under another name
    is still that lane, and the shared CLAUDE.md in the brain root's parent
    directory being a symlink into this repo is the local idiom, not an exotic
    case.
    """
    if not directory or not _is_abs(directory):
        return None
    resolved = _real(directory)
    brain = _real(BRAIN)
    for candidate in (resolved, _norm(directory)):
        for root in (brain, BRAIN):
            if not (candidate == root or candidate.startswith(root + "/")):
                continue
            rest = candidate[len(root):].lstrip("/")
            # THE ROOT ITSELF IS A LANE. `rest` is empty for `~/brain`,
            # and an empty lane name matched nothing, so a Codex session sitting
            # at the root fell through to "not a brainer" and was unguarded —
            # the same hole measured on 2026-09-07 for the Claude registration.
            lane = rest.split("/")[0] if rest else "brain"
            if lane in LANES:
                return lane
            # `work/` is the brain's reserved directory for executors'
            # worktrees (install.sh refuses it as a lane name), not a brainer's
            # place: a Codex executor in `<brain>/work/<lane>/<task>` is not a
            # brainer standing inside brain. It is no lane, and stays unguarded
            # as every executor is.
            if lane == "work":
                return None
            # A brain subdirectory that is NOT a lane (`bin/`, `_skills/`, a
            # stray path) is still a brainer standing inside brain, so it gets
            # the root lane rather than no guard.
            return "brain"
    return None


def lane_candidates(payload):
    """Directories that might say WHICH LANE this session belongs to.

    Ordered most-specific first, and every source is tried because the payload
    key Codex uses is not established here — a first launch needs a human to
    clear two dialogs, so the shape has not been captured. `os.getcwd()` is the
    backstop: a PreToolUse hook is spawned by the agent, so its own cwd is the
    session's unless Codex deliberately changes it.

    THIS ANSWERS ONE QUESTION ONLY. An earlier draft used the directory that
    resolved the lane as the SHELL's cwd as well, and that conflation was a
    hole, measured 2026-09-06:

        payload cwd = ~/repo   (an earlier `cd`)
        CODEX_PROJECT_DIR = ~/brain/lane-a
        command = `rm -rf ./x`
            codex guard   ALLOW
            claude hook   DENY   (same command, same cwd)

    The payload's cwd is not under `brain/`, so it resolved no lane; the loop
    fell through to the launch-time env var, which did — and that stale brain
    directory was then handed to `decide` as the cwd. `stands_in_protected` was
    computed against a directory the shell had long left, so the cwd axis was
    silently off for exactly the case it exists to catch. The two questions are
    now answered separately: this one picks the lane, `shell_cwd` picks the cwd.

    THE SOURCE IS PART OF THE ANSWER. Each candidate is yielded as
    `(directory, from_payload)`: a payload key is something this invocation
    observed, an environment variable is a value fixed at launch by whoever
    started the process. `main` uses the distinction for exactly one case — see
    the `brain` root note there.
    """
    for value in (
        payload.get("cwd"),
        payload.get("working_directory"),
        payload.get("workdir"),
    ):
        if value:
            yield value, True
    for value in (
        os.environ.get("CODEX_PROJECT_DIR"),
        os.environ.get("CLAUDE_PROJECT_DIR"),
        os.environ.get("PWD"),
    ):
        if value:
            yield value, False
    try:
        yield os.getcwd(), False
    except OSError:
        return


def shell_cwd(payload):
    """Where the command will actually RUN — the cwd axis, and nothing else.

    Freshest first and never an env var fixed at session launch: a stale value
    here does not mis-name the lane, it silently disables the axis. `os.getcwd()`
    is the last resort because the hook is spawned by the agent, so it at least
    tracks the session rather than its launch.
    """
    for value in (
        payload.get("cwd"),
        payload.get("working_directory"),
        payload.get("workdir"),
    ):
        if value and _is_abs(value):
            return value
    try:
        return os.getcwd()
    except OSError:
        return ""


# Codex trims each line before reading a header, so an indented one is applied
# (measured 2026-10-01: " *** Add File: x" created x); leading blanks must match.
PATCH_HEADER = re.compile(r"^[^\S\n]*\*\*\* (?:Add File|Update File|Delete File|Move to): (.+?)\s*$", re.M)


def patch_targets(patch, cwd):
    """Absolute paths every header of an `apply_patch` names.

    A `~` path yields BOTH readings, `$HOME/..` and the literal `~` joined to the
    cwd, since which one Codex applies is not established here: both are checked.
    """
    from deny_repo_writes import _join  # lazily: only a patch needs it
    out = []
    for raw in PATCH_HEADER.findall(patch):
        spellings = [raw]
        if raw.startswith("~"):
            spellings.insert(0, os.path.expanduser(raw))
        for path in spellings:
            if not _is_abs(path):
                path = _join(cwd, path) if cwd else path
            out.append((raw, path))
    return out


def decide_patch(cfg, patch, cwd):
    """None to allow, or (rule, reason): the Bash answer for a write, per target.

    A write into a protected tree is refused whoever's tool performs it, so this
    asks the shared module's own boundary question of each header's path.
    """
    from deny_repo_writes import _inside_any
    for raw, path in patch_targets(patch, cwd):
        if _inside_any(cfg, path):
            return ("patch",
                    "Blocked: this patch writes %s (written as `%s`), inside %s. "
                    "The brainer is read-only there; the same write through Bash "
                    "is refused too." % (path, raw, cfg["where"]))
    return None


def main():
    try:
        payload = json.load(sys.stdin)
    except Exception:
        sys.exit(0)  # Never block on a payload that is not JSON: it names no tool.

    # A PAYLOAD OF THE WRONG SHAPE REFUSES, where this session is a brainer.
    # Measured 2026-09-29 (audit F12): a JSON array or string, or a Bash
    # `tool_input` that is null, a string or a list, raised AttributeError and
    # exited 1 — non-blocking, so the command ran. Codex's payload has not been
    # captured, so a shape this guard cannot read is the likeliest way it meets
    # one. The lane is still resolved first: an executor is never guarded, and
    # a payload that is not an object says nothing, so only the environment can
    # name the lane for it.
    malformed = None
    if not isinstance(payload, dict):
        malformed = "a payload that is not a JSON object (%s)" % type(payload).__name__
        payload = {}
    elif payload.get("tool_name") not in ("Bash", "apply_patch"):
        sys.exit(0)
    elif not isinstance(payload.get("tool_input"), dict):
        malformed = ("a %s call whose tool_input is not an object (%s)"
                     % (payload.get("tool_name"), type(payload.get("tool_input")).__name__))
    elif not isinstance(payload["tool_input"].get("command", ""), (str, type(None))):
        malformed = ("a %s call whose command is not a string (%s)"
                     % (payload.get("tool_name"), type(payload["tool_input"]["command"]).__name__))

    is_patch = not malformed and payload.get("tool_name") == "apply_patch"
    command = "" if malformed else (payload["tool_input"].get("command", "") or "")
    if is_patch and not command:
        # A patch whose text is not under `command` is a payload shape this guard
        # cannot read; a brainer refuses it rather than let every patch through.
        malformed = "an apply_patch call with no patch text in tool_input.command"
    if not command and not malformed:
        sys.exit(0)

    if LOAD_ERROR is not None:
        print(
            "BLOCKED by deny-repo-writes (codex): the shared guard could not load "
            "(setup/guards/deny_repo_writes.py and guards.json): %s: %s. Every "
            "Bash command is refused until it loads, because a guard that is not "
            "running cannot tell a protected tree from any other directory."
            % (type(LOAD_ERROR).__name__, LOAD_ERROR),
            file=sys.stderr,
        )
        raise SystemExit(2)

    # A MORE SPECIFIC LANE OUTRANKS THE ROOT, whichever candidate produced it.
    # Adding `brain` as a lane must not silently RE-LABEL the other four: a lane-a
    # brainer whose payload cwd is `~/brain/bin` resolves "brain" on
    # the first candidate, and returning it there would have dropped Cowork's
    # store and the team's docs — lane-a's own extra trees — from the guard, while
    # the launch-time `CODEX_PROJECT_DIR` still said `brain/lane-a`. So the root is
    # held as a fallback and the search continues.
    lane = None
    root_fallback = False
    for directory, from_payload in lane_candidates(payload):
        found = lane_for(directory)
        if not found:
            continue
        # THE BARE `brain` ROOT IS NOT RESOLVED FROM THE ENVIRONMENT. It is the
        # least specific lane and the likeliest stale value: an operator's shell
        # or a launcher standing in `~/brain` puts that path in `PWD`,
        # and every process it spawns inherits it — including an EXECUTOR in its
        # own worktree, which must never be guarded (a guarded executor cannot
        # do its job at all). Measured while adding this lane: a
        # payload whose cwd was `~/repo-scratch/some-task`
        # was blocked, resolved as lane=brain from the harness shell's `PWD`.
        #
        # A lane SUBDIRECTORY in an env var stays trusted, because the earlier
        # measurement in `lane_candidates` above depends on it: a brainer that
        # `cd`'d out of its lane is still a brainer, and only the launch-time
        # variable still says so.
        if found == "brain" and not from_payload:
            continue
        if found == "brain":
            root_fallback = True
            continue
        lane = found
        break
    if lane is None and root_fallback:
        lane = "brain"
    if lane is None:
        # Not a brainer. An executor's worktree, a repo checkout, anywhere else
        # — the same answer the per-directory registration gives the other two
        # vendors.
        sys.exit(0)

    if malformed:
        print(
            "BLOCKED by deny-repo-writes (codex, lane=%s): the guard was handed %s, "
            "so it cannot read the command. Refused rather than guessed." % (lane, malformed),
            file=sys.stderr,
        )
        sys.exit(2)

    # THE CWD AXIS GETS THE REAL CWD, resolved independently of which directory
    # happened to name the lane. See `lane_candidates` for the measurement that
    # forced this apart.
    #
    # A CRASH HERE REFUSES. An exception out of `config` or `decide` used to
    # leave with Python's exit 1, which Codex reads as a non-blocking error: the
    # command ran, unguarded, exactly when the guard had reached no verdict. Only
    # exit 2 blocks. (The lane is already known, so an executor is never reached.)
    try:
        if is_patch:
            verdict = decide_patch(config(lane), command, shell_cwd(payload))
        else:
            verdict = decide(config(lane), command, shell_cwd(payload))
    except Exception as exc:  # noqa: BLE001 — ANY failure while deciding must refuse
        print(
            "BLOCKED by deny-repo-writes (codex, lane=%s): the guard CRASHED while "
            "deciding (%s: %s). It reached no verdict, and a guard that reached no "
            "verdict has not established that this command is safe. Every Bash "
            "command is refused until this is fixed." % (lane, type(exc).__name__, exc),
            file=sys.stderr,
        )
        sys.exit(2)
    if verdict:
        print(
            "BLOCKED by deny-repo-writes (codex, lane=%s): %s" % (lane, verdict[1]),
            file=sys.stderr,
        )
        sys.exit(2)
    sys.exit(0)


if __name__ == "__main__":
    main()
