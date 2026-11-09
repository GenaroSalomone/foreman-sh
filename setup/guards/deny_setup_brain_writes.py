#!/usr/bin/env python3
"""A setup EXECUTOR does not write in the brainer's checkout of brain.

The setup lane edits brain itself, in a worktree of its own (`<work root>/setup/<task>`). The brainer's `main`
checkout (`brain_root` in guards.json) is a different tree, and nothing refused a setup executor
that `cd`'d into it: in pi-background-wait one made 30 Write/Edit calls there and its recovery `git restore` erased the
brainer's uncommitted edits (2026-11-04); in jd-env-trust-check one committed onto `main` outside any merge. The setup
lane says `brain_guard: false` (the inverse guard, `deny_brain_writes.py`, would stop it running its own repo's tools),
`deny_executor_repo_writes.py` protects only the product roots, so this is the missing piece.

WHAT IT REFUSES, for an executor of the setup lane only (`HW_TASK` set and `HW_PROJECT=setup`): the Write/Edit tools on a
path inside `brain_root`, and any Bash that `deny_repo_writes.decide` calls a write there (redirects, rm, in-place
edits, git mutations, a shell standing in brain) — the same detection a brainer gets over a product repo, aimed at brain.

WHAT IT LEAVES ALONE: reads (`cat`, `rg`, `git log/status/diff/show`), the programs of brain's `bin/` run by path
(`done-invoker`, `ask-invoker`, `hw`), and everything under the executor's own `HW_WORKDIR`. A setup executor whose
workdir IS inside brain (`--here`: a pane in brain/setup, no worktree) has no other tree to write, so it is not
judged. The brainer has no `HW_TASK` and is never judged. Not a sandbox: code that COMPUTES a path at run time is
beyond this check, as for the other guards.

Registered by the tracked `.claude/settings.json` (Claude, so a worktree executor and the brainer both load it) and by
`setup/pi/hw-executor.ts` (pi, which calls this file with the same payload). FAILS CLOSED: an import failure, a crash
while deciding and an unreadable payload all deny, as the hook command ends in `|| exit 2`.
"""
import ast
import json
import os
import re
import sys

try:
    sys.path.insert(0, os.path.dirname(os.path.realpath(__file__)))
    import deny_repo_writes as drw  # noqa: E402
    LOAD_ERROR = None
except Exception as exc:  # noqa: BLE001 - ANY failure here must deny
    drw, LOAD_ERROR = None, exc

WRITE_TOOLS = {"Write": "file_path", "Edit": "file_path", "MultiEdit": "file_path", "NotebookEdit": "notebook_path"}
LANE = "setup"


def applies(env=None):
    env = os.environ if env is None else env
    return bool(env.get("HW_TASK")) and env.get("HW_PROJECT") == LANE


def deny(reason):
    print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny",
                                             "permissionDecisionReason": reason}}))
    sys.exit(0)


def brain_config(workdir):
    """The brain lane's config with `roots` cut to brain_root, or None when the executor's workdir is inside it."""
    root = drw.BRAIN_ROOT
    if workdir:
        try:
            home = drw._real(workdir)
        except OSError:
            home = drw._canon(workdir)
        for cand in {root, drw._real(root)}:
            if home == cand or home.startswith(cand + "/"):
                return None
    cfg = drw.config("brain")
    cfg["roots"] = (root,)
    cfg["resolved_roots"] = drw._resolve_roots(cfg["roots"])
    cfg["root_ids"] = drw._root_ids(cfg["roots"])
    cfg["worktree_roots"] = ()
    cfg["where"] = "the brainer's checkout of brain (%s)" % root
    cfg["write_here"] = "your own worktree (%s)" % (workdir or "$HW_WORKDIR")
    cfg["git_tail"] = ("A setup task changes brain only in its own worktree and branch; the brainer merges it. "
                       "Read-only git (log/status/diff/show/rev-list) and brain's bin/ tools by path are allowed.")
    return cfg


_CD_KW = re.compile(r"(?:^|[;&|(`\n])\s*(?:builtin\s+)?(?:cd|pushd)\s+(?:--\s+)?")
_CD_WORD = re.compile(r"""\s*("[^"]*"|'[^']*'|[^\s;&|)`]+)""")


def cd_into(cfg, command, cwd):
    """The first `cd`/`pushd` target of `command` that lands inside brain, or None.

    A shell standing in brain is how a relative `setup/commit-tree` or `git restore .` reaches the brainer's checkout
    with no brain path in the command at all (jd-env-trust-check, pi-background-wait). The target is resolved the way
    the shell would: `~`, `$HOME`, `..`, symlinks, relative to the session's cwd. Reads stay possible by absolute
    path or `git -C`; it is the standing there that is refused.
    """
    # What is DATA is not a `cd`: a heredoc body, or the interior of a quoted string (a commit message, an `echo`),
    # is dropped by the same maskers `decide` uses. The target word is read from the unmasked text at the same offset.
    text = drw._strip_heredoc_bodies(command)
    masked, balanced = drw._mask_quotes(text)
    if not balanced or len(masked) != len(text):
        masked = text  # a quote that never closes cannot be reasoned about: scan it all, which is the stricter reading
    for m in _CD_KW.finditer(masked):
        w = _CD_WORD.match(text, m.end())
        if not w:
            continue
        tgt = w.group(1).strip("\"'")
        tgt = os.path.expandvars(drw._expand_tilde(tgt))
        if not tgt or tgt == "-":
            continue
        if not drw._is_abs(tgt):
            tgt = os.path.join(cwd or os.getcwd(), tgt)
        if drw._inside_any(cfg, tgt):
            return tgt
    return None


# A python heredoc that EDITS a file whose TEXT mentions brain (a note, a decisions entry, a doc) is not a write into brain,
# but `decide` keeps an interpreter's heredoc as code and so refuses the root it names (n=491, retro herdr-app-premium-pass).
# The body is parsed: a string that holds the root is TEXT only when it is an argument of a text method (replace, write,
# format, …) that is not itself inside a path-taking call (open, Path, os.*, shutil.*, subprocess.*, exec, eval). Any other
# use (a literal assigned, a call to open, an f-string, a body that does not parse) leaves the body in the scan: refused as before.
# A text method whose RESULT is assigned and then handed to a path sink (`dst = s.replace(…); open(dst, 'w')`) is followed one
# assignment deep; a result used inline as a path (`open('{}'.format(…))`) is refused by the sink walk above.
# ONLY A QUOTED DELIMITER: an unquoted one lets the shell expand $(…) and backticks in the body before python runs.
# Not a sandbox: a path built at run time (`'a' + 'b'`, a variable) is beyond this check, as for the other guards.
_PY_HEREDOC = re.compile(r"^[^\n]*?(?<![\w-])python3?\s+-[^\n]*<<-?\s*(['\"])(\w+)\1[^\n]*\n(.*?)^\t*\2[ \t]*$", re.M | re.S)
_TEXT_METHODS = {"replace", "write", "writelines", "format"}
_FILE_MODULES = {"os", "shutil", "subprocess", "pathlib", "sys"}  # os.replace(a, b) moves a file: not text
_PATH_SINKS = {"open", "Path", "PurePath", "exec", "eval", "compile", "__import__", "system", "popen", "run", "call",
               "check_call", "check_output", "Popen", "rename", "replace_file", "remove", "unlink", "rmtree", "copy",
               "copy2", "copyfile", "move", "write_text", "write_bytes", "mkdir", "makedirs", "chmod", "symlink", "link", "touch"}


def _called_name(call):
    f = call.func
    if isinstance(f, ast.Attribute) and isinstance(f.value, ast.Name) and f.value.id in _FILE_MODULES:
        return "os." + f.attr  # a file operation, never a text method
    return f.id if isinstance(f, ast.Name) else f.attr if isinstance(f, ast.Attribute) else ""


def _body_mentions_brain_as_text_only(body, roots):
    """True when `body` parses, and every string holding a root is text handed to a text method outside any path sink."""
    try:
        tree = ast.parse(body)
    except SyntaxError:
        return False
    parents = {}
    for node in ast.walk(tree):
        for child in ast.iter_child_nodes(node):
            parents[child] = node
    tainted = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.JoinedStr) and any(r in ast.unparse(node) for r in roots):
            return False
        if isinstance(node, ast.Constant) and isinstance(node.value, str) and any(r in node.value for r in roots):
            par = parents.get(node)
            ok = False
            while par is not None:
                if isinstance(par, ast.Call):
                    name = _called_name(par)
                    if name.startswith("os.") or (name in _PATH_SINKS and name not in _TEXT_METHODS):
                        return False
                    if name in _TEXT_METHODS and node in par.args:
                        ok = True
                    elif not ok:
                        return False
                par = parents.get(par)
            if not ok:
                return False
            stmt = parents.get(node)
            while stmt is not None and not isinstance(stmt, ast.stmt):
                stmt = parents.get(stmt)
            if isinstance(stmt, (ast.Assign, ast.AnnAssign, ast.AugAssign)):  # `dst = s.replace(…)`: followed into a path sink below
                tainted |= {n.id for t in (getattr(stmt, "targets", None) or [stmt.target]) for n in ast.walk(t) if isinstance(n, ast.Name)}

    def sunk(call):  # a path-taking call; write_text/write_bytes take CONTENT, which is the legitimate use of an edited text
        name = _called_name(call)
        return name.startswith("os.") or (name in _PATH_SINKS and name not in ("write_text", "write_bytes"))
    return not any(isinstance(c, ast.Call) and sunk(c) and any(isinstance(n, ast.Name) and n.id in tainted
                   for a in [*c.args, *(k.value for k in c.keywords)] for n in ast.walk(a)) for c in ast.walk(tree))


def without_text_only_heredocs(command, roots):
    """`command` with the body of each python heredoc that only quotes brain as text removed."""
    def cut(m):
        return m.group(0)[: m.start(3) - m.start(0)] + m.group(0)[m.end(3) - m.start(0):] \
            if _body_mentions_brain_as_text_only(m.group(3), roots) else m.group(0)
    return _PY_HEREDOC.sub(cut, command)


# brain's closing tools, run by path as the HEAD of a command (after a boundary, never as an operand). The docstring
# allows them; but `decide` refuses any command that has an interpreter (`python3 - <<EOF`) AND names brain anywhere, so
# `python3 - <<EOF … EOF` followed by `<brain>/bin/done-invoker` was denied.
_TOOL_HEAD = r"(^|(?<![>\\])[;&|(\n]\s*)(?:%s)/bin/(?:done-invoker|ask-invoker)(?=\s|$)"


def without_closing_tools(command):
    """`command` with brain's done-invoker/ask-invoker, when run by path as a command, replaced by a bare word."""
    home = drw._shell_home()
    text = command.replace("$HOME", home).replace("${HOME}", home)
    text = re.sub(r"(?<![\w~])~/", lambda _m: home + "/", text)
    roots = {drw.BRAIN_ROOT, drw._real(drw.BRAIN_ROOT)}
    text = without_text_only_heredocs(text, roots)
    return re.sub(_TOOL_HEAD % "|".join(re.escape(r) for r in roots), r"\1done-tool", text)


def main(stream=None):
    try:
        payload = json.load(stream or sys.stdin)
    except Exception:
        sys.exit(0)  # not JSON: it names no tool
    if not applies():
        sys.exit(0)
    if LOAD_ERROR is not None:
        if isinstance(payload, dict) and payload.get("tool_name") in ("Bash", *WRITE_TOOLS):
            deny("Blocked: the setup brain-write guard could not load its policy (%s: %s). It reached no verdict, "
                 "and a guard that reached no verdict has not established that this is safe."
                 % (type(LOAD_ERROR).__name__, LOAD_ERROR))
        sys.exit(0)
    if not isinstance(payload, dict):
        deny("Blocked: the setup brain-write guard was handed a payload that is not a JSON object (%s). Refused "
             "rather than guessed." % type(payload).__name__)
    tool = payload.get("tool_name", "")
    if tool != "Bash" and tool not in WRITE_TOOLS:
        sys.exit(0)
    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        deny("Blocked: the setup brain-write guard was handed a %s call whose tool_input is not an object (%s). "
             "Refused rather than guessed." % (tool, type(tool_input).__name__))
    cwd = payload.get("cwd", "") or ""
    try:
        cfg = brain_config(os.environ.get("HW_WORKDIR", ""))
        if cfg is None:
            sys.exit(0)
        if tool == "Bash":
            command = tool_input.get("command", "") or ""
            standing = cd_into(cfg, command, cwd)
            if standing:
                deny("Blocked: this command changes directory into %s, the brainer's checkout of brain. A shell "
                     "standing there makes every relative path and `git` call land in the brainer's tree "
                     "(setup/commit-tree, git restore, an editor). Stay in your worktree (%s); read brain by absolute "
                     "path or `git -C`, and run brain's bin/ tools by path."
                     % (standing, os.environ.get("HW_WORKDIR") or "$HW_WORKDIR"))
            verdict = drw.decide(cfg, without_closing_tools(command), cwd)
            if verdict:
                deny(verdict[1])
        else:
            path = tool_input.get(WRITE_TOOLS[tool], "") or ""
            if path and not drw._is_abs(path):
                path = os.path.join(cwd, path)
            if path and drw._inside_any(cfg, path):
                deny("Blocked: %s writes %s, inside the brainer's checkout of brain. A setup task writes only in its "
                     "own worktree (%s); the brainer merges it." % (tool, path, os.environ.get("HW_WORKDIR") or "$HW_WORKDIR"))
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 - ANY failure here must deny
        deny("Blocked: the setup brain-write guard CRASHED while deciding (%s: %s). It reached no verdict, and a "
             "guard that reached no verdict has not established that this is safe." % (type(exc).__name__, exc))
    sys.exit(0)


if __name__ == "__main__":
    main()
