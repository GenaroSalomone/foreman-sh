#!/usr/bin/env python3
"""What specialist agents exist for THIS brainer's project — read live, every
session, from wherever they actually live.

WHY THIS FILE EXISTS. `hw` puts the specialist inventory in the EXECUTOR's
prompt. A brainer gets nothing: its picture of what exists was whatever
someone once symlinked by hand into `brain/<project>/.claude/agents/`, and
that picture goes stale in silence. Measured 2026-09-10:
`brain/lane-a/.claude/agents/` held `ops-investigator.md` and
`repo-auditor.md`; the product repo's own `.claude/agents/` also held
`qa-tester.md`, which documents the whole browser-QA mechanism and had existed
for weeks. A brainer reported QA as blocked on a login wall it never checked,
because it did not know the specialist that reads the credential itself
existed. That is the incident this file closes: read the product repo's
`.claude/agents/` live, every session, instead of trusting a hand-made mirror.

WHAT COUNTS AS A "SPECIALIST" VS FRAMEWORK NOISE. The product repos each carry
roughly twenty agent files, and most of them are Judgment Day / Spec Kit
machinery (`jd-*`, `review-*`, `speckit-*`), plus whatever `sdd-*` agents the
now-retired Gentle AI install left behind on this machine — dispatched by an
orchestrator, never picked by name from a menu a brainer reads. Listing all of
them buries the two or three specialists that actually matter.

A NAME-PREFIX FILTER WAS THE OBVIOUS FIRST IDEA AND IT IS WRONG: a project
that later adds a real specialist named e.g. `review-costos` would vanish
under any filter keyed on the string "review-". So this file does not pattern
-match names at all. Instead it asks a question with a real, machine-checkable
answer: is this exact agent name part of the machine-wide framework install?
`~/.claude/agents/` is where Spec Kit and Judgment Day put the agents THEY
ship — it is not a guess about naming conventions, it is the actual manifest
of what the framework installed on this machine, plus any Gentle AI residue
never swept out of it. A specialist a project author writes — whatever they name it —
only ever lives in the product repo's own `.claude/agents/`, so it is never in
that directory and never gets filtered out, regardless of what it is called.
The corresponding false positive (a framework agent that is not currently
mirrored into `~/.claude/agents/`, e.g. because the global machine mode
differs from the target repo's own mode) is a known, accepted limitation:
it can let a little framework noise through, but it never hides a real
specialist, which is the failure this file exists to prevent.

TWO SOURCES, MERGED, PRODUCT WINS ON A NAME COLLISION. A brainer sees its own
lane's `.claude/agents/` (if it has one) AND the product repo's, live. Where
both name the same agent, the product repo's copy wins — it is the one repo
whoever wrote the specialist actually maintains; a lane-local file with the
same name is exactly the kind of hand-made mirror this file replaces.
"""
import os
import re

FRAMEWORK_AGENTS_DIR = os.path.expanduser("~/.claude/agents")

# The lanes with an associated product repo, read from `guards.json`
# (`specialists_from`) at brain's root. The lanes with none say so by omission,
# and callers must treat a missing lane as "no product repo", not as an error.
#
# A repo lane is listed even when its `.claude/agents/` holds ONLY framework
# agents, so its roster is empty today. Listing it anyway is the whole point of
# reading LIVE instead of mirroring: the day someone writes a specialist in that
# repo, its brainer sees it with no change here. A lane left out of the table
# cannot discover one, and that silent staleness is the incident this file
# exists to close.
#
# NOT `projects.json`'s `product_repo` flag, on purpose: that flag also marks a
# lane whose repository does not exist yet, and deriving from it would start
# reading a directory this roster has never read.
#
# `~` stays unexpanded here and goes through `os.path.expanduser` at use, the
# way this file always expanded it. A policy that cannot be read raises at
# import, and the lane shim already turns a failed import into silence: a
# SessionStart hook that crashes would cost the session, not protect anything.
def _load_roster_policy():
    import json

    path = os.path.join(
        os.path.dirname(os.path.dirname(os.path.dirname(os.path.realpath(__file__)))),
        "guards.json")
    with open(path, encoding="utf-8") as f:
        doc = json.load(f)
    repos = doc["specialists_from"]
    brain = doc["brain_root"]
    if not isinstance(brain, str) or not isinstance(repos, dict) or not all(
            isinstance(k, str) and isinstance(v, str) for k, v in repos.items()):
        raise ValueError("guards.json: brain_root or specialists_from is malformed")
    return brain, dict(repos)


BRAIN_ROOT, LANE_PRODUCT_REPOS = _load_roster_policy()


def brainer_dir(lane):
    if lane == "brain":
        return os.path.expanduser(BRAIN_ROOT)
    return os.path.expanduser(f"{BRAIN_ROOT}/{lane}")


def product_repo_dir(lane):
    repo = LANE_PRODUCT_REPOS.get(lane)
    return os.path.expanduser(repo) if repo else None


_TOP_KEY_RE = re.compile(r"^([A-Za-z_][A-Za-z0-9_-]*):\s*(.*)$")


def _parse_frontmatter(text):
    """Best-effort YAML-frontmatter reader for exactly the two fields this
    file needs. Not a YAML parser — a project's `.claude/agents/*.md` is
    written by hand for Claude Code's own agent-frontmatter convention, and
    depending on a non-stdlib YAML library here would make a SessionStart
    hook's success depend on what happens to be installed in whichever
    python3 the calling agent's PATH resolves to."""
    if not text.startswith("---"):
        return None
    end = text.find("\n---", 3)
    if end == -1:
        return None
    body = text[3:end]
    lines = body.split("\n")
    fields = {}
    i = 0
    while i < len(lines):
        m = _TOP_KEY_RE.match(lines[i])
        if not m:
            i += 1
            continue
        key, inline_value = m.group(1), m.group(2).strip()
        if inline_value and inline_value[0] not in (">", "|"):
            fields[key] = inline_value
            i += 1
            continue
        # Folded/literal block scalar: collect indented continuation lines.
        block = []
        i += 1
        while i < len(lines) and (lines[i].startswith((" ", "\t")) or lines[i] == ""):
            block.append(lines[i].strip())
            i += 1
        fields[key] = " ".join(l for l in block if l)
    return fields


def _read_agent(path):
    try:
        with open(path, "r", encoding="utf-8") as fh:
            text = fh.read()
    except OSError:
        return None
    fields = _parse_frontmatter(text)
    if not fields:
        return None
    name = fields.get("name") or os.path.splitext(os.path.basename(path))[0]
    description = fields.get("description", "").strip()
    return name, description


def _iter_agent_dir(dir_path):
    try:
        entries = sorted(os.listdir(dir_path))
    except OSError:
        return
    for entry in entries:
        if not entry.endswith(".md"):
            continue
        parsed = _read_agent(os.path.join(dir_path, entry))
        if parsed:
            yield parsed


def framework_agent_names():
    return {name for name, _ in _iter_agent_dir(FRAMEWORK_AGENTS_DIR)}


def _agents_dirs(lane):
    dirs = [os.path.join(brainer_dir(lane), ".claude", "agents")]
    product = product_repo_dir(lane)
    if product:
        dirs.append(os.path.join(product, ".claude", "agents"))
    return dirs


def specialists(lane):
    """The real entry point: merges the lane's own agents dir with the
    product repo's, product-repo entries winning on a name collision."""
    framework = framework_agent_names()
    roster = {}
    for source_dir in _agents_dirs(lane):
        for name, description in _iter_agent_dir(source_dir):
            if name in framework:
                continue
            roster[name] = description
    return sorted(roster.items())


def render_context(lane, roster):
    """Returns the SessionStart additionalContext string, or None when there
    is nothing to say — a lane with no specialists must stay silent, not
    print an empty section."""
    if not roster:
        return None
    lines = [
        f"## Especialistas disponibles para este proyecto ({lane})",
        "",
        "Leído en vivo esta sesión desde `.claude/agents/` — antes de reportar algo "
        "bloqueado por falta de un mecanismo, revisá esta lista.",
        "",
    ]
    for name, description in roster:
        description = re.sub(r"\s+", " ", description).strip()
        if len(description) > 240:
            description = description[:237].rstrip() + "..."
        lines.append(f"- **{name}** — {description}" if description else f"- **{name}**")
    return "\n".join(lines)


def main(lane):
    import json

    context = render_context(lane, specialists(lane))
    if context is None:
        return
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "SessionStart",
            "additionalContext": context,
        }
    }))


if __name__ == "__main__":
    import sys

    main(sys.argv[1] if len(sys.argv) > 1 else "")
