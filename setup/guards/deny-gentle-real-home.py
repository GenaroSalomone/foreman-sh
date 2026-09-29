#!/usr/bin/env python3
"""gentle-ai never installs or syncs anywhere but its own HOME.

`--sdd gentle` (see setup/decisions.md for the ruling and its history) keeps
gentle-ai in one directory, GENTLE_HOME, provisioned by `hw gentle-home`. Its
wrapper, GENTLE_HOME/bin/gentle-ai, fixes HOME to GENTLE_HOME/home whatever it
is asked, so it cannot reach the machine's agent configs. The binary behind it,
GENTLE_HOME/libexec/gentle-ai, or any other gentle-ai, can: run with the real
HOME, `install` and `sync` rewrite ~/.claude, ~/.claude.json,
~/.config/opencode and ~/.codex. That is what retired gentle-ai; see
setup/decisions.md for the incident.

So this PreToolUse hook refuses a Bash command whose program is gentle-ai and
whose verb mutates configuration (install, sync, uninstall, upgrade, restore),
unless the program is the wrapper itself. Reading, `--help`, `version` and the
wrapper pass. hw registers it for every `--sdd gentle` executor through
Claude's `--settings`, and the brainer of each lane that lists the mode carries
it in its settings.

Detection is by program position, not substring: `rg gentle-ai install` names
the word and is not a run of it. `sh -c '<cmd>'` is read recursively.
"""
import json
import os
import re
import shlex
import sys

VERBS = {"install", "sync", "uninstall", "upgrade", "restore"}
PREFIXES = {"exec", "command", "nohup", "time", "sudo", "builtin"}
SHELLS = {"sh", "bash", "zsh", "dash"}


def gentle_home():
    return os.environ.get("HW_GENTLE_HOME") or os.path.join(
        os.path.expanduser("~"), ".local/share/hw/gentle-home")


def simple_commands(command):
    """Split on the shell's list and pipe operators, outside quotes."""
    parts, buf, quote, i = [], [], None, 0
    while i < len(command):
        c = command[i]
        if quote:
            buf.append(c)
            if c == quote:
                quote = None
            elif c == "\\" and quote == '"' and i + 1 < len(command):
                buf.append(command[i + 1]); i += 1
        elif c in "'\"":
            quote = c; buf.append(c)
        elif c in ";|&\n()`":
            parts.append("".join(buf)); buf = []
        else:
            buf.append(c)
        i += 1
    parts.append("".join(buf))
    return [p.strip() for p in parts if p.strip()]


def words(part):
    try:
        return shlex.split(part)
    except ValueError:
        return part.split()


def offending(command, depth=0):
    """The first gentle-ai run that mutates config outside the wrapper, or None."""
    wrapper = os.path.join(gentle_home(), "bin", "gentle-ai")
    for part in simple_commands(command):
        w = words(part)
        # Skip assignments and the prefixes that run what follows them.
        while w and (re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", w[0]) or w[0] in PREFIXES):
            w = w[1:]
        if w and w[0] == "env":
            w = w[1:]
            while w and (w[0].startswith("-") or re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", w[0])):
                w = w[1:]
        if not w:
            continue
        prog = w[0]
        if os.path.basename(prog) in SHELLS and depth < 3:
            for j, a in enumerate(w[1:], 1):
                if a == "-c" and j + 1 < len(w):
                    hit = offending(w[j + 1], depth + 1)
                    if hit:
                        return hit
                    break
            continue
        if os.path.basename(prog) != "gentle-ai":
            continue
        if not VERBS.intersection(a for a in w[1:] if not a.startswith("-")):
            continue
        if os.path.normpath(prog) == os.path.normpath(wrapper):
            continue
        return part
    return None


def deny(reason):
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": reason,
        }
    }))
    sys.exit(0)


def main():
    try:
        payload = json.load(sys.stdin)
    except (ValueError, OSError):
        sys.exit(0)
    if payload.get("tool_name") != "Bash":
        sys.exit(0)
    command = (payload.get("tool_input") or {}).get("command") or ""
    hit = offending(command)
    if hit:
        deny("Blocked: `%s` runs gentle-ai against a HOME that is not its own, and gentle-ai "
             "install/sync rewrites the machine's agent configs (~/.claude, ~/.claude.json, "
             "~/.config/opencode, ~/.codex). gentle-ai only runs through its wrapper, %s, "
             "which is provisioned and kept by `hw gentle-home`."
             % (hit, os.path.join(gentle_home(), "bin", "gentle-ai")))
    sys.exit(0)


if __name__ == "__main__":
    main()
