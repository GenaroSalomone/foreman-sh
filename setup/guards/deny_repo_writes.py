#!/usr/bin/env python3
"""The read-only guard, once, for every lane and every vendor.

WHY THIS FILE EXISTS. Until 2026-09-06 this logic lived in EIGHT files — four
`.claude/hooks/deny-repo-writes.py` and four `.opencode/plugin/deny-repo-writes.js`
— and the four Python copies had drifted 11.5 KB apart. The drift was not
cosmetic and it was not one-directional:

  * one lane's copy had heredoc-body stripping, quoted-string masking and
    the spent-worktree teardown exception. The other three had none of it.
  * one lane's copy had LOST `_inside_protected()`, replacing a
    boundary-correct, `..`-normalising cwd test with a bare `str.startswith`.
    Measured, with IDENTICAL constants in both files:

        cwd=~/docs/../app/src  rm -rf ./x
            setup/.claude/hooks   DENY      lane-a/.claude/hooks   ALLOW
        cwd=~/app-scratch                      rm -rf ./x
            setup/.claude/hooks   ALLOW     lane-a/.claude/hooks   DENY

    A false negative and a false positive, in the lane the roadmap called the
    baseline. So "level up to lane-a" was not, by itself, correct:
    the target is the UNION, and this file is it.

  * The eight-copy conformance suite in `setup/guards/` was green across all of
    that. It proves py-vs-js parity WITHIN a lane on 15 vectors; it never
    compared one lane against another, so an 11.5 KB spread was invisible to it.
    That is the "confidence without protection" shape, one level up from the
    string-matching hazard this guard is about.

WHAT IS SHARED AND WHAT IS NOT. Detection is shared — every check every lane had
now runs in every lane. Configuration is not: the protected roots, the prose, and
the EXEMPTIONS stay per lane, in `guards.json` at brain's root, loaded into
`LANES` below. An exemption is not a check, and
handing one lane's spent-worktree teardown to another would widen
what that lane permits. Widening is a decision, not a refactor; lanes without a
`worktree_roots` entry cannot reach the exception at all.

WHY IT IS NOT A PATH SUBSTRING TEST. Permission rules match tool *paths*. A shell
redirect is not a path — `echo x > repo/file.txt` is a single Bash call whose
argument merely contains a filename, so `Edit()`/`Write()` deny rules never see
it. That hole is not theoretical: the first enforcement test of this setup wrote
a probe file into the repo with exactly that command while every deny rule was in
place. And matching the string is not enough either — `..`, a symlink, or a
sibling directory whose name merely begins with a protected root all defeat a
substring test. Every boundary question in this file therefore RESOLVES, then
compares, and resolves against both the raw and the realpath'd form of both the
candidate and the root, so neither a symlinked root nor a symlinked candidate can
slip between them.

The brainer is read-only over the protected trees. Executors write, in their own
worktrees, and they do not load this guard.

The JavaScript half is `deny-repo-writes.js` beside this file, and it is the same
decision on the same table. If you change one, change both — `setup/guards/`
holds the conformance suite that proves you did.
"""
import json
import os
import re
import shlex
import sys

# ── NATIVE WINDOWS: ONE SPELLING OF A PATH ──────────────────────────────────
#
# Every boundary question below compares strings, and on Windows one directory
# has many: Git Bash writes `/c/Data/x`, Claude Code's payload `C:\Data\x`,
# Windows Python answers `C:\DATA\X`, and NTFS does not care about case. A
# comparison between two of those spellings is false, and a false boundary
# answer here is an ALLOW. So on native Windows every root, cwd and path token
# is folded to ONE canonical spelling before it is compared — `/c/data/x`:
# drive as a leading segment, forward slashes, lower case (NTFS is case-
# insensitive) — and realpath runs on the native spelling, then folds back.
# Folding only ever makes two spellings equal that were different, so it can
# add matches, never remove one. `~` is the profile directory of the account
# the process runs as, read from its token (GetUserProfileDirectoryW, Windows'
# password database), never USERPROFILE or HOME.
#
# Everywhere else each helper is the identity (or os.path's own), so a POSIX
# decision cannot move. `DENY_REPO_WRITES_AS_NATIVE_WINDOWS=1` folds on any
# platform, so the folding is testable where Windows is not — it only adds
# matches there too, and realpath stays POSIX's.
#
# A Cygwin or MSYS2 Python (sys.platform cygwin/msys) is NOT this: it answers
# in its own POSIX spelling over a Windows filesystem, and nothing here was
# measured against it. It refuses at import, which every shim turns into a deny.
NON_NATIVE_WINDOWS_REFUSAL = (
    "this read-only guard does not run under a Cygwin or MSYS2 Python: its "
    "Windows path model was measured with Windows' own Python. Point python3 "
    "at a python.org install, or run the brain under WSL2 (INSTALL.md, Windows)")
if sys.platform.startswith(("cygwin", "msys")):
    raise ImportError(NON_NATIVE_WINDOWS_REFUSAL)

import posixpath  # noqa: E402

_NATIVE_WINDOWS = os.name == "nt"
_WINPATHS = _NATIVE_WINDOWS or os.environ.get("DENY_REPO_WRITES_AS_NATIVE_WINDOWS") == "1"
_DRIVE = re.compile(r"^([A-Za-z]):(?:[\\/]|$)")
_TEXT_DRIVE = re.compile(r"(?<![\w])([A-Za-z]):/")


def _canon(p):
    """The one spelling of a path on Windows; the path itself elsewhere."""
    if not _WINPATHS or not isinstance(p, str) or not p:
        return p
    q = p.replace("\\", "/")
    m = _DRIVE.match(q)
    if m:
        q = "/" + m.group(1) + q[2:]
    if q.startswith("/"):
        # One leading slash, as the JavaScript half's normPath leaves it.
        q = posixpath.normpath("/" + q.lstrip("/"))
    return q.lower()


def _canon_text(text):
    """A command's text with every drive path in the canonical spelling, for
    the literal gates that ask whether a root appears in it at all."""
    if not _WINPATHS or not isinstance(text, str):
        return text
    return _TEXT_DRIVE.sub(lambda m: "/" + m.group(1) + "/", text.replace("\\", "/")).lower()


def _is_abs(p):
    if not isinstance(p, str):
        return False
    if _WINPATHS:
        return p.startswith(("/", "\\")) or bool(_DRIVE.match(p))
    return p.startswith("/")


def _norm(p):
    return _canon(p) if _WINPATHS else os.path.normpath(p)


def _join(base, rel):
    if _WINPATHS:
        return _canon(posixpath.join(_canon(base), rel.replace("\\", "/")))
    return os.path.join(base, rel)


def _msys_root():
    """Where Git Bash's `/` is, for a `/usr/...`-style path: its own mount
    table when msys-compat.sh exported it, else beside the bash on PATH."""
    r = os.environ.get("HW_MSYS_ROOT", "")
    if not r:
        import shutil
        b = (shutil.which("bash") or "").replace("\\", "/")
        r = b[: -len("/usr/bin/bash.exe")] if b.lower().endswith("/usr/bin/bash.exe") else ""
    return r.replace("\\", "/").rstrip("/")


def _native(c):
    """Canonical spelling -> one Windows can open, or None."""
    m = re.match(r"^/([a-z])(/.*)?$", c)
    if m:
        return m.group(1) + ":" + (m.group(2) or "/")
    if c == "/tmp" or c.startswith("/tmp/"):
        t = os.environ.get("TEMP") or os.environ.get("TMP")
        return t.replace("\\", "/").rstrip("/") + c[4:] if t else None
    r = _msys_root() if c.startswith("/") else ""
    return r + c if r else None


def _real(p):
    """realpath, answered in the canonical spelling."""
    if _NATIVE_WINDOWS:
        import ntpath
        c = _canon(p)
        n = _native(c) if isinstance(c, str) else None
        if n is None:
            return c
        try:
            return _canon(ntpath.realpath(n))
        except (OSError, ValueError):
            return c
    return _canon(os.path.realpath(p))


def _shell_home():
    """What the shell expands `$HOME` and `~/` to, spelled the way bash spells
    it: without bin/'s layer msys hands a native Python HOME as a backslash drive path,
    and a path built on that would not match the patterns that follow."""
    if _NATIVE_WINDOWS:
        import ntpath
        h = (os.environ.get("HOME") or ntpath.expanduser("~")).replace("\\", "/")
        m = re.match(r"^([A-Za-z]):(/|$)", h)
        return "/" + m.group(1).lower() + h[2:] if m else h
    return os.path.expanduser("~")


if _NATIVE_WINDOWS:
    def _account_home():
        import ctypes
        from ctypes import wintypes
        advapi32 = ctypes.WinDLL("advapi32", use_last_error=True)
        userenv = ctypes.WinDLL("userenv", use_last_error=True)
        kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel32.GetCurrentProcess.restype = wintypes.HANDLE
        advapi32.OpenProcessToken.argtypes = [wintypes.HANDLE, wintypes.DWORD, ctypes.POINTER(wintypes.HANDLE)]
        userenv.GetUserProfileDirectoryW.argtypes = [wintypes.HANDLE, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
        token = wintypes.HANDLE()
        if not advapi32.OpenProcessToken(kernel32.GetCurrentProcess(), 0x0008, ctypes.byref(token)):
            raise PolicyError("cannot open this process's token to read its profile directory "
                              "(error %d)" % ctypes.get_last_error())
        try:
            size = wintypes.DWORD(0)
            userenv.GetUserProfileDirectoryW(token, None, ctypes.byref(size))
            buf = ctypes.create_unicode_buffer(max(size.value, 1))
            if not userenv.GetUserProfileDirectoryW(token, buf, ctypes.byref(size)) or not buf.value:
                raise PolicyError("GetUserProfileDirectoryW failed (error %d)" % ctypes.get_last_error())
            return buf.value
        finally:
            kernel32.CloseHandle(token)
else:
    import pwd  # noqa: E402

    def _account_home():
        return pwd.getpwuid(os.getuid()).pw_dir

# ── THE LANE TABLE ──────────────────────────────────────────────────────────
#
# THE VALUES LIVE IN `guards.json`, BESIDE `projects.json` AT BRAIN'S ROOT, and
# this file and `deny-repo-writes.js` read the same copy. Until 2026-09-23 each
# half carried its own literal table. They agreed, but only because someone kept
# them in step by hand, and every path in them spelled one machine. The lane
# data moved to `projects.json` in stage 2 of the harness opening. This is the
# guards' half of stage 3: what the policy protects now comes from a file, and
# this code is only the mechanism. What each key means:
#
# `repo` and `worktrees` are the lane's OWN pair of protected roots, and the
# prose and the teardown exemption key on them. Every lane with a product repo
# names its OWN here, so its brainer is read-only over it. A lane with no
# central checkout, whose repo lives only as the clones inside each task's work
# dir, names that work root as both. Executors write there and do not load this
# guard. On a lane with no product repo (brain, setup) the pair is NOMINAL and
# every protected tree arrives through `product_repos`. `setup` protecting
# the product repos is deliberate: the setup lane owns `bin/` and the rules,
# and the protected trees are not its to change either.
#
# `product_repos` IS THE FLOOR, AND IT IS NOT OPTIONAL. Measured 2026-09-07 with
# a real payload against the four hooks registered then: a brainer in
# brain/setup, brain/lane-a or brain/lane-b writing to lane-c's repo was
# ALLOWED, because only lane-c's own hook guarded it. A brainer does not become
# entitled to another product repo by standing in a different brain directory,
# so every lane protects every entry. It is not a per-lane key on purpose: a
# lane that could opt out of it is how that hole came about.
#
# `worktree_roots` is the ONLY key that grants an exemption (the spent-worktree
# teardown), and a lane without it cannot reach that branch. An exemption is a
# widening, and a widening is the operator's call, not a refactor.
#
# ADDING A ROOT IS A DECISION ABOUT EVERY LANE, not about the new one. a new root
# joined on 2026-09-14 and that single line made every existing lane read-only
# over it, which is why each lane's `.claude/settings.json` needed its own
# `Edit()` deny in the same commit. The hook half and the `permissions.deny`
# half do not see the same things (a shell redirect is not a path; an `Edit()`
# is not a Bash call), so a root added to one half and not the other is half a
# guard.
#
# A repo whose worktrees live INSIDE it needs no second entry. The separate
# outside worktree root is listed only because it predates that convention and
# is still live.
#
# DELIBERATELY NOT PROTECTED: brain itself. It is where every brainer writes, and
# guarding it would deny the lane's own purpose. `write_here` names it for
# exactly that reason.
#
# `~` IS THE HOME IN THE PASSWORD DATABASE, NOT `$HOME`. The literal table this
# replaces did not move when a process exported another HOME, and a guard whose
# roots follow an environment variable the guarded process can set is one
# `HOME=/tmp` away from protecting nothing.
#
# A MISSING OR MALFORMED FILE FAILS CLOSED. Loading happens at import, so a
# policy that cannot be read raises here, and every shim already turns a failed
# import into a refusal (see `.claude/hooks/deny-repo-writes.py` and the Codex
# guard). Every field is checked for shape, and an unknown key is refused
# rather than ignored. A typo in `worktree_roots` that got ignored would drop an
# exemption silently, and one in `product_repos` would drop a root.
POLICY_PATH = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.realpath(__file__)))),
    "guards.json")

_TOP_KEYS = {"comment", "brain_root", "product_repos", "defaults", "lanes",
             "specialists_from"}
_LANE_REQUIRED = ("repo", "worktrees", "write_here", "where")
_LANE_OPTIONAL = ("worktree_roots", "git_tail", "redirect_tail")
_LANE_NAME = re.compile(r"^[a-z][a-z-]*$")


class PolicyError(Exception):
    """`guards.json` is missing or malformed. Nothing is protected until fixed."""


def _policy_home():
    return _canon(_account_home())


def _policy_path(value, where, home):
    if not isinstance(value, str) or not value:
        raise PolicyError("%s must be a non-empty path string, got %r" % (where, value))
    if value == "~":
        value = home
    elif value.startswith("~/"):
        value = home + value[1:]
    value = _canon(value)
    if not _is_abs(value):
        raise PolicyError("%s must be absolute or start with ~/, got %r" % (where, value))
    return value


def _policy_text(value, where):
    if not isinstance(value, str) or not value:
        raise PolicyError("%s must be a non-empty string, got %r" % (where, value))
    return value


def load_policy(path=None):
    """Read and check `guards.json`, returning (brain_root, product_repos, lanes).

    Raises PolicyError on anything short of a complete, well-formed policy.
    """
    path = path or POLICY_PATH
    try:
        with open(path, encoding="utf-8") as f:
            doc = json.load(f)
    except (OSError, ValueError) as exc:
        raise PolicyError("cannot read %s: %s: %s" % (path, type(exc).__name__, exc))
    if not isinstance(doc, dict):
        raise PolicyError("%s is not a JSON object" % path)
    unknown = sorted(set(doc) - _TOP_KEYS)
    missing = sorted({"brain_root", "product_repos", "defaults", "lanes"} - set(doc))
    if unknown or missing:
        raise PolicyError("%s: unknown keys %s, missing keys %s" % (path, unknown, missing))
    home = _policy_home()
    brain_root = _policy_path(doc["brain_root"], "brain_root", home)
    repos = doc["product_repos"]
    if not isinstance(repos, list) or not repos:
        raise PolicyError("product_repos must be a non-empty list")
    product = tuple(_policy_path(p, "product_repos[%d]" % i, home)
                    for i, p in enumerate(repos))
    defaults = doc["defaults"]
    if not isinstance(defaults, dict) or set(defaults) != {"git_tail", "redirect_tail"}:
        raise PolicyError("defaults must hold exactly git_tail and redirect_tail")
    for key in defaults:
        _policy_text(defaults[key], "defaults." + key)
    raw_lanes = doc["lanes"]
    if not isinstance(raw_lanes, dict) or "brain" not in raw_lanes:
        raise PolicyError("lanes must be an object that includes the brain root lane")
    lanes = {}
    for name, raw in raw_lanes.items():
        if not _LANE_NAME.match(name):
            raise PolicyError("lane name %r is not a lowercase word" % name)
        if not isinstance(raw, dict):
            raise PolicyError("lanes.%s is not an object" % name)
        unknown = sorted(set(raw) - set(_LANE_REQUIRED) - set(_LANE_OPTIONAL))
        missing = sorted(set(_LANE_REQUIRED) - set(raw))
        if unknown or missing:
            raise PolicyError("lanes.%s: unknown keys %s, missing keys %s"
                              % (name, unknown, missing))
        exempt = raw.get("worktree_roots", [])
        if not isinstance(exempt, list):
            raise PolicyError("lanes.%s.worktree_roots must be a list" % name)
        lanes[name] = {
            "repo": _policy_path(raw["repo"], "lanes.%s.repo" % name, home),
            "worktrees": _policy_path(raw["worktrees"], "lanes.%s.worktrees" % name, home),
            "worktree_roots": tuple(
                _policy_path(p, "lanes.%s.worktree_roots[%d]" % (name, i), home)
                for i, p in enumerate(exempt)),
            "also_protect": product,
            "write_here": _policy_text(raw["write_here"], "lanes.%s.write_here" % name),
            "where": _policy_text(raw["where"], "lanes.%s.where" % name),
            "git_tail": _policy_text(raw.get("git_tail", defaults["git_tail"]),
                                     "lanes.%s.git_tail" % name),
            "redirect_tail": _policy_text(
                raw.get("redirect_tail", defaults["redirect_tail"]),
                "lanes.%s.redirect_tail" % name),
        }
    return brain_root, product, lanes


BRAIN_ROOT, PRODUCT_REPOS, LANES = load_policy()


def config(lane):
    """Resolve a lane name to its config, or die loudly.

    A guard that silently no-ops on an unknown lane is worse than no guard: the
    shim would keep exiting 0 and every write would land. `KeyError` here means
    a shim and this table disagree, and the hook then fails closed at the
    caller (see `main`).
    """
    if lane not in LANES:
        raise KeyError(
            "deny-repo-writes: unknown lane %r — known lanes are %s. A shim and "
            "the lane table have diverged; the guard is NOT protecting anything "
            "until this is fixed." % (lane, ", ".join(sorted(LANES)))
        )
    cfg = dict(LANES[lane])
    cfg["lane"] = lane
    # `repo`/`worktrees` stay the lane's OWN pair — the teardown exemption and
    # the prose key on them. `roots` is every tree the lane must not write,
    # deduplicated because a lane's own repo is normally in PRODUCT_REPOS too.
    roots = [cfg["repo"], cfg["worktrees"]]
    for extra in cfg.get("also_protect", ()):
        if extra not in roots:
            roots.append(extra)
    cfg["roots"] = tuple(roots)
    # Resolved once per process, not once per token: `decide` now asks the
    # boundary question of every absolute path in a command, and re-realpath'ing
    # the two roots for each of them would be the same syscalls over and over.
    cfg["resolved_roots"] = _resolve_roots(cfg["roots"])
    return cfg


# ── DETECTION ───────────────────────────────────────────────────────────────
# Every regex below is shared by every lane. Nothing here is lane-configured,
# and nothing here was dropped from any copy.

# Mutating shell verbs. Word-anchored so `remove_stale_rows` or a path segment
# called `cp-report` cannot trip them.
MUTATORS = re.compile(
    r"(?<![\w-])("
    r"rm|mv|cp|rsync|tee|touch|mkdir|rmdir|truncate|dd|ln|chmod|chown|"
    r"install|patch|sponge|"
    # Interpreters and line editors write as readily as `cp` does, and the
    # 2026-08-20 audit got a file into the repo with `python3 -c "open(...,'w')"`
    # while `Bash(python3:*)` sat on the brainer's allowlist.
    r"ed|ex|python|python3|node|ruby|perl|php|deno|bun|osascript|xargs"
    r")(?![\w-])"
)
# sed/perl/awk only mutate with an in-place flag.
INPLACE = re.compile(r"(?<![\w-])(sed|perl|gawk|awk)\b[^|;&]*\s-i\b")
# Any redirect that creates or appends to a file (not 2>&1, not a heredoc).
REDIRECT = re.compile(r"(?<![0-9&])>{1,2}(?!&)")

# WORD-ANCHORING IS NOT COMMAND-POSITION. Measured 2026-09-10 with `cwd` inside
# the `lane-a` lane: `ls app/scripts/bin/node` and
# `cat app/docs/rm.md` were both DENIED, because `/` and `.` sit
# outside `[\w-]` — the same class `MUTATORS`'s lookaround excludes — so a path
# SEGMENT merely named after a mutator (`bin/node`, `rm.md`) satisfies the same
# boundary a real invocation does. Widening the lookaround to include `/`/`.`
# is the naive fix and it is unsafe: it would let `/usr/bin/python3 -c
# "open(...,'w')"` back through, which is exactly the hole `MUTATORS` grew
# interpreter names to close (see that comment above). The real distinction is
# POSITION, not spelling: a mutator counts only when it is the word actually
# being invoked as a command — the head of a simple command, optionally
# reached through variable assignments (`VAR=1 python3`), the `env`/`command`/
# `exec`/`xargs` wrappers, and/or a leading path (`/usr/bin/python3`) — and not
# when it is an operand of some OTHER command (`ls`, `cat`, `test -x`, `wc`).
_CMD_BOUNDARY = re.compile(r"[;&|(`\"'\n]|\$\(")
_CMD_PREFIX_ALLOWED = re.compile(
    r"^\s*(?:\w+=\S*\s+)*"
    r"(?:(?:env|command|exec|xargs(?:\s+-\S+)*)\s+)*"
    r"(?:[\w./-]*/)?$"
)


def _is_command_position(text, pos):
    """True when the match starting at `text[pos:]` is the head of a simple
    command rather than a path segment inside some other command's operand.

    Walks back from `pos` to the nearest shell-command boundary — `;&|(` `` ` ``
    `$(`, a newline, or a quote character (the last is what keeps `bash -c
    "rm ..."` denied: by the time this runs, `masked` has already blanked
    quoted DATA to spaces, so a quote surviving here is either a dash-c
    script-open or the harmless edge of a blanked-out citation) — and checks
    that everything between that boundary and `pos` is only variable
    assignments, wrapper commands, and/or a bare leading path. Anything else
    (another command's name and its own flags, e.g. `ls `, `cat `, `test -x `)
    fails the check, because those chars cannot appear in this grammar.
    """
    prefix = text[:pos]
    last_boundary_end = 0
    for m in _CMD_BOUNDARY.finditer(prefix):
        last_boundary_end = m.end()
    segment = prefix[last_boundary_end:]
    return bool(_CMD_PREFIX_ALLOWED.match(segment))
# git subcommands that change the repo or its refs.
GIT_MUTATORS = re.compile(
    r"(?<![\w-])git\b[^|;&]*?(?<![\w-])("
    r"commit|add|rm|mv|checkout|switch|restore|reset|revert|merge|rebase|"
    r"cherry-pick|apply|am|stash|push|clean|gc|prune|worktree\s+(add|remove|prune)|"
    r"branch\s+-[dDmM]|tag|config|update-ref|symbolic-ref"
    r")(?![\w-])"
)


# ── BOUNDARY: RESOLVE, THEN COMPARE ─────────────────────────────────────────
#
# The hazard this guard has already paid for once (see setup/decisions.md) is a
# string test wearing a path test's confidence. Three ways a string test is
# wrong, and all three are live on this machine:
#
#   `..`      ~/docs/../app/src is INSIDE
#             the repo and starts with neither root.
#   prefix    ~/app-scratch is OUTSIDE and
#             starts with the repo root.
#   symlink   a link anywhere pointing into a protected tree reaches it under a
#             name the guard has never heard of. The shared CLAUDE.md in the
#             brain root's parent directory is itself a symlink into this repo, so this is the local idiom, not
#             an exotic evasion.
#
# So: normalise `..` away, compare on a path-SEGMENT boundary, and resolve
# symlinks. The resolution is deliberately two-sided — candidate raw AND
# realpath'd, against root raw AND realpath'd. Today no protected root is a
# symlink (checked 2026-09-06); the day one becomes one, comparing only
# resolved-to-raw would silently stop matching. Extra candidates and extra roots
# can only ever make `_inside_any` MORE true, so this direction never opens a
# hole.


def _resolve_roots(roots):
    out = []
    for root in roots:
        if root not in out:
            out.append(root)
        try:
            real = _real(root)
        except OSError:
            continue
        if real not in out:
            out.append(real)
    return tuple(out)


def _candidates(path):
    """The forms a path may be recognised under: normalised, and resolved.

    THE REALPATH CANDIDATE IS ABSOLUTE-ONLY, and that is not fussiness. This
    hook runs as a subprocess whose own cwd has nothing to do with the shell's,
    so `os.path.realpath("x")` would resolve against the WRONG directory and
    answer a boundary question about a path that was never named. Every caller
    already passes an absolute path; this makes the requirement structural
    rather than remembered, and skipping the candidate is the safe direction —
    the normalised form is still compared.
    """
    out = [_norm(path)]
    if not _is_abs(path):
        return tuple(out)
    try:
        real = _real(path)
    except OSError:
        return tuple(out)
    if real not in out:
        out.append(real)
    return tuple(out)


def _inside_any(cfg, path):
    """True when `path` lands on or under a protected root, however spelled."""
    if not path:
        return False
    for cand in _candidates(path):
        for root in cfg["resolved_roots"]:
            if cand == root or cand.startswith(root + "/"):
                return True
    return False


# Absolute-path-looking runs in a command. Deliberately crude: it is used only
# to ASK the boundary question of a token, and `_inside_any` answers it. A token
# that is not really a path resolves to something outside the trees and changes
# nothing.
_ABS_TOKEN = re.compile(r"/[^\s;|&()<>\"']+")
_WIN_TOKEN = re.compile(r"(?<![\w])[A-Za-z]:[\\/][^\s;|&()<>\"']*")


def _resolves_protected(cfg, probe):
    """The first absolute path in `probe` that lands inside, or None.

    THE GATE ITSELF WAS A SUBSTRING TEST. Measured 2026-09-06 against all eight
    pre-unification copies, with a symlink `ro-link -> app`:

        rm -rf <ro-link>/src                        ALLOW  (all eight)
        echo probe > <ro-link>/HOLE.txt             ALLOW  (all eight)
        echo probe > .../.nope/../app/HOLE.txt   ALLOW (all eight)

    Every inner predicate had been taught to resolve, and the guard still let
    those through — because `repo in probe` decided whether any of them ran, and
    a symlinked or `..`-spelled path does not contain the root as a substring.
    Resolving the destination is pointless if the gate never opens.

    The one asymmetry worth stating: this only widens what REACHES the deny
    chain. The chain still decides, so a READ through a symlink
    (`rg -n foo <ro-link>/src`) matches no mutator and is still allowed.
    """
    seen = set()
    for token in _ABS_TOKEN.findall(probe):
        if token in seen:
            continue
        seen.add(token)
        if _inside_any(cfg, token):
            return token
    # AND AS SHELL WORDS, because `_ABS_TOKEN` stops at a space and a quote:
    # `'/var/…/My App/x'` was asked as `/var/…/My`, and a root with spaces
    # reached through a symlink (`/var` → `/private/var`) never opened the gate
    # (measured 2026-09-24, setup/tests/186). Only adds tokens to ask about.
    # AND IN WINDOWS' SPELLING, which has no leading `/` for the token above
    # to start at: `C:\\...` and `C:/...`.
    if _WINPATHS:
        for token in _WIN_TOKEN.findall(probe):
            if token in seen:
                continue
            seen.add(token)
            if _inside_any(cfg, token):
                return token
    for word in _SHELL_WORD.findall(probe):
        token = _unquote_word(word)
        if token in seen or not _is_abs(token):
            continue
        seen.add(token)
        if _inside_any(cfg, token):
            return token
    return None


def _inside_protected(cfg, path):
    """The cwd axis. Absolute paths only — a relative cwd is not resolvable.

    one lane's copy had replaced this with a bare `str.startswith`, which
    is wrong in both directions; see the measurements in this module's
    docstring. It is restored here for every lane.
    """
    if not path or not _is_abs(path):
        return False
    return _inside_any(cfg, path)


def _under_a_worktree_root(cfg, path):
    """True for a path strictly beneath one of the lane's worktree roots.

    Strictly: a root itself is not a worktree, and `REPO + "/.worktrees"` must
    never let `REPO` through. The trailing slash is what enforces both. A lane
    with no `worktree_roots` can never satisfy this, which is how the teardown
    exemption stays where it already was.
    """
    path = _canon(path)
    for root in cfg["worktree_roots"]:
        for r in (root, _real(root)):
            if path.startswith(r + "/"):
                return True
    return False


# ── NARROW EXCEPTION: tearing down a spent worktree ─────────────────────────
#
# `hw done` closes the space but leaves the worktree and its branch on disk, and
# it cannot remove them itself. That left the brainer asking the operator to run
# two git commands after every task — which is the chore this whole setup
# exists to remove, and the operator said so.
#
# Only these two forms are allowed through, and only because each one REFUSES
# by itself when the thing it is deleting still holds work:
#
#   git worktree remove <path under a worktree root>  refuses on a dirty worktree
#   git branch -d <names>                             refuses on an unmerged branch
#
# `--force`/`-f` and `branch -D` stay denied: those are the flags that turn a
# self-guarding command into a destructive one. `worktree prune` stays denied
# too — it acts on every registration at once, not on a path you named.
#
# An optional `-C <dir>` is permitted because `git worktree remove` has to run
# from inside the repository that registered the worktree, and the brainer's cwd
# is never in there. `cd <repo> && git ...` stays denied: that is chaining, and
# the rule against parsing shell to decide a delete is safe holds.
#
# GATED ON `worktree_roots` BEING NON-EMPTY. Three of the four lanes never had
# this exemption and do not get it here.
_ALLOWED_TEARDOWN = re.compile(
    r"""^\s*git\s+(-C\s+(?P<c>[^\s;|&]+)\s+)?(
          worktree\s+remove\s+(?P<wt>[^\s;|&]+)\s*
        | branch\s+-d\s+(?P<br>[^;|&]+?)\s*
        )$""",
    re.VERBOSE,
)
_FORCE = re.compile(r"(?<![\w-])(--force|-f|-D)(?![\w-])")


def _is_spent_worktree_teardown(cfg, probe):
    """True for a single, unforced teardown of a worktree under a lane root.

    Deliberately strict: one command, no chaining, no force flags. A compound
    command is rejected rather than parsed, because parsing shell to decide
    whether a delete is safe is how a guard stops being one.
    """
    if not cfg["worktree_roots"]:
        return False
    if _FORCE.search(probe):
        return False
    m = _ALLOWED_TEARDOWN.match(probe)
    if not m:
        return False
    # A `-C` may only point at the repo itself or at a worktree of it. Anywhere
    # else and this is not the teardown it is claiming to be.
    #
    # A RELATIVE PATH REFUSES THE EXEMPTION. `os.path.realpath` resolves against
    # THIS PROCESS's cwd, which is not the shell's — so `git worktree remove
    # ./spent` would be judged against a directory nobody named. Refusing is the
    # safe direction and costs the operator an absolute path.
    c = m.group("c")
    if c is not None:
        c = _expand_tilde(c.strip("\"'"))
        if not _is_abs(c):
            return False
        c = _real(c)
        if c != _real(cfg["repo"]) and c != cfg["repo"] \
                and not _under_a_worktree_root(cfg, c):
            return False
    wt = m.group("wt")
    if wt is None:
        return True  # `git branch -d` — self-guarding, refuses if unmerged.
    wt = _expand_tilde(wt.strip("\"'"))
    if not _is_abs(wt):
        return False
    return _under_a_worktree_root(cfg, _real(wt))


def _expand_tilde(p):
    if p == "~" or p.startswith("~/"):
        return _shell_home() + p[1:]
    return os.path.expanduser(p) if not _NATIVE_WINDOWS else p


# ── WHAT IS DATA AND WHAT IS CODE ───────────────────────────────────────────
#
# `rg -n "cp " <REPO>/x` is a search pattern, not an invocation of `cp`, and a
# heredoc's prose is not command text. Denying those is a FALSE POSITIVE, and a
# false positive here is not harmless — it teaches the agent to route around the
# guard, which is how a guard stops being one. Only one lane's copy carried
# this; all four lanes carry it now.
#
# The two escape hatches below are what make masking safe: a quote that IS a
# script (`bash -c "rm x"`) and a heredoc that FEEDS a shell (`bash <<EOF`) stay
# fully visible to every regex.

_HEREDOC_START = re.compile(r"<<(-)?\s*(['\"]?)(\w+)\2")

# A shell fed as an interpreter treats its argument/stdin as CODE, not data —
# masking it would hide a real `rm`/`sed -i`/redirect from every regex below.
# `bash -c "..."`, `sh -c '...'`, and `bash <<EOF` are the shape of that; `env`
# and `command` are the common wrappers that still leave the shell reachable.
_SHELL_INTERPRETERS = r"(?:bash|sh|zsh|dash|ksh|ash)"
_SHELL_WRAPPERS = r"(?:(?:env|command|exec)\s+)*"

# True when the text immediately before a quote's opening character is a
# shell interpreter invoked with `-c` — i.e. the quote IS the script.
_SHELL_DASH_C = re.compile(
    r"(?:^|[|;&(])\s*" + _SHELL_WRAPPERS +
    r"(?:[\w./-]*/)?" + _SHELL_INTERPRETERS + r"\b"
    r"(?:\s+-[\w-]+)*\s+-c\s*$"
)

# CONSUMERS WHOSE HEREDOC BODY CAN DETERMINE A DESTINATION, not just a real
# shell re-parsing it as new commands. Measured 2026-09-09, adversarially,
# against the fix below this one (heredoc bodies feeding the content-only
# gate): `xargs -I{} rm {} <<EOF` runs `rm` once per LINE of its stdin, so a
# heredoc line IS a real argument, not prose — treating it as data would have
# let `xargs -I{} rm {} <<'EOF'\n<REPO>/f\nEOF` straight past the gate.
# `patch`'s destination is the `+++ b/<path>` header INSIDE the diff it is fed,
# never an argument on its own command line, so its body is the same shape.
# `ed`/`ex` are line editors that read editing COMMANDS — including `w <path>`
# — from stdin, so their body is code by the reasoning a shell's already is.
# The general interpreters (`python`, `node`, …) are included too: nothing
# stops a script from reading its own stdin and acting on what it finds there,
# and this guard cannot parse an arbitrary script to rule that out — so their
# heredoc stays visible on the same "refuse rather than guess" footing every
# other unparseable case in this file already stands on.
_HEREDOC_BODY_IS_LIVE = (
    r"(?:bash|sh|zsh|dash|ksh|ash|xargs|patch|ed|ex|"
    r"python|python3|node|ruby|perl|php|deno|bun|osascript)"
)

# True when a heredoc-start line names one of those consumers as either the
# command consuming the heredoc directly (`bash <<EOF`) or the target of a
# pipe on that same line (`cat <<EOF | bash`) — either way the body is live.
_SHELL_HEREDOC_TARGET = re.compile(
    r"(?:^|[|;&(])\s*" + _SHELL_WRAPPERS +
    r"(?:[\w./-]*/)?" + _HEREDOC_BODY_IS_LIVE + r"\b(?=\s|<<|$)"
)


def _strip_heredoc_bodies(text):
    """Drop heredoc body lines that are DATA — keep the ones that are CODE.

    A `>` in a heredoc's prose (`- resumen > detalle`) fed to `cat` is not a
    shell redirect, and a mutator word in that prose is not a command
    invocation — so that body is dropped. But a heredoc fed to a shell
    interpreter (`bash <<EOF`, `cat <<EOF | bash`) IS command text: the shell
    will execute every line of it, so it stays in the scan. This is
    line-based, not a real shell parser: a heuristic good enough to find
    `<<DELIM` ... `DELIM`, including `<<-DELIM` (leading tabs on the closing
    line) and a quoted delimiter (`<<'EOF'`).
    """
    lines = text.split("\n")
    out = []
    i = 0
    while i < len(lines):
        line = lines[i]
        out.append(line)
        m = _HEREDOC_START.search(line)
        if not m:
            i += 1
            continue
        dash, delim = bool(m.group(1)), m.group(3)
        close_re = re.compile((r"^\t*" if dash else r"^") + re.escape(delim) + r"\s*$")
        feeds_shell = bool(_SHELL_HEREDOC_TARGET.search(line))
        i += 1
        while i < len(lines) and not close_re.match(lines[i]):
            if feeds_shell:
                out.append(lines[i])  # code the shell executes — keep it visible
            i += 1
        if i < len(lines):
            out.append(lines[i])
            i += 1
    return "\n".join(out)


def _mask_quotes(text):
    """Blank the interior of quoted strings that are DATA, not CODE.

    Blanking what is inside quotes removes string *content* from the scan while
    leaving real, unquoted command text (and the quote marks themselves, so `>`
    right before a quoted redirect target is still visible) untouched. But
    `bash -c "rm x"` is not a search pattern — the quote IS the script the shell
    runs — so a quote immediately after a shell `-c` is left unmasked.

    A TOP-LEVEL BACKSLASH ESCAPES THE NEXT CHARACTER, and leaving that out was a
    real protection loss. Measured 2026-09-06 on the first draft of this file:

        echo \\' && rm -rf <REPO>/x \\'

        lane-b DENY -> ALLOW       lane-c  DENY -> ALLOW
        setup  DENY -> ALLOW       lane-a  ALLOW -> ALLOW

    bash sees NO quote there at all — `\\'` is one escaped literal character — so
    it runs two commands and the `rm` reaches the repo. The scanner saw the `'`
    as a delimiter, opened a quoted region, and blanked ` && rm -rf <REPO>/x `
    out of the text every deny regex reads. Three lanes had no masking before
    and refused this correctly; porting lane-a's masker to them
    carried its bug along. That is precisely the levelling-down this whole
    unification exists to refuse, so the escape is honoured here the way bash
    honours it: outside quotes and inside double quotes, never inside single
    quotes (where bash does not honour it either).

    RETURNS (masked, balanced). An UNTERMINATED quote is not a string this can
    reason about: the scanner blanks everything from the opening quote to the
    end of the text, which is a general-purpose way to hide a command from every
    deny regex. Measured 2026-09-06:

        echo "unterminated <REPO>/x && rm -rf <REPO>/y     ALLOW

    bash refuses to run that (unexpected EOF), so it is not directly
    exploitable — but "the shell would have rejected it anyway" is a claim about
    another program's parser, and this guard does not get to lean on one. When
    the quotes do not balance, the caller falls back to the UNMASKED text, which
    is the same stance `_is_copy_out_of_protected` already takes when
    `shlex.split` raises: an input this cannot parse refuses the concession.
    """
    out = []
    balanced = True
    i, n = 0, len(text)
    while i < n:
        c = text[i]
        if c == "\\" and i + 1 < n:
            # Escaped: emit both characters and let NEITHER open a quote.
            out.append(c)
            out.append(text[i + 1])
            i += 2
            continue
        if c in ("'", '"'):
            quote = c
            quote_start = i
            out.append(c)
            i += 1
            start = i
            while i < n and text[i] != quote:
                if quote == '"' and text[i] == "\\" and i + 1 < n:
                    i += 2
                    continue
                i += 1
            interior = text[start:i]
            if _SHELL_DASH_C.search(text[:quote_start]):
                out.append(interior)  # code the shell runs — keep it visible
            else:
                out.append(" " * len(interior))
            if i < n:
                out.append(text[i])
                i += 1
            else:
                balanced = False  # ran off the end looking for the closer
        else:
            out.append(c)
            i += 1
    return "".join(out), balanced


# ── NARROW EXCEPTION: copying OUT of a protected tree ───────────────────────
#
# `hw done` tells the operator "Move what you need out first" before a worktree
# is reaped, and this guard denied exactly that move, stranding the artifacts in
# the one place it was just said they would not survive.
#
# The cause: the guard asked whether a mutating command NAMES a protected path,
# never WHERE in the argument list that path sits. In `cp SRC DST` only DST is
# written; a protected path in SRC is a READ, and reading is allowed.
#
# `mv` is deliberately absent from the tables below. Moving out of a protected
# tree REMOVES the source, so BOTH operands are write targets and it stays
# denied. The same reasoning excludes `rsync --remove-source-files`.
#
# Fail-closed by construction: SOURCE operands are the only tokens this never
# inspects, and a source is whatever is LEFT once argv[0], every option, every
# option value and the final destination have each been individually recognised
# and cleared. Anything unaccounted for — an unknown option, an operand that
# cannot be resolved — returns a refusal, and the command falls through to the
# deny chain exactly as it did before.

# argv[0], with any leading directory stripped.
_COPY_LIKE = re.compile(r"^(?:[^\s/]*/)*(cp|rsync|install)$")

# A metacharacter means this is not one simple command, and parsing shell to
# decide that a write is safe is how a guard stops being one. A surviving `$`
# counts too: after expansion it means a variable this hook cannot resolve.
_NOT_SIMPLE = re.compile(r"[<>|;&()`\n$]")

# Options that decide WHICH operand is the destination, or that delete the
# source. Refused outright rather than parsed: `cp -t DST SRC...` inverts the
# operand order, `install -d` turns every operand into a directory to create,
# and `rsync --remove-source-files` is an `mv` wearing a `cp`'s name.
_COPY_ORDER_FLAGS = {
    "cp": {"-t", "--target-directory"},
    "install": {"-t", "--target-directory", "-d", "--directory"},
    "rsync": {"--remove-source-files"},
}

# Per command: (self-contained short letters, value-taking short letters,
# self-contained long options, value-taking long options). This is an
# ALLOWLIST. An option not in it refuses the exemption rather than being
# guessed at, because guessing is how an option VALUE gets mistaken for a
# source operand — and an option value can be a write target
# (`rsync --backup-dir DIR`) while a source operand never is.
_COPY_FLAGS = {
    "cp": (
        "abcdfHiLlnPpRrsTuvXxZ",
        "S",
        {"archive", "attributes-only", "backup", "copy-contents", "debug",
         "dereference", "force", "interactive", "link", "no-clobber",
         "no-dereference", "no-preserve", "no-target-directory",
         "one-file-system", "parents", "preserve", "recursive", "reflink",
         "remove-destination", "sparse", "strip-trailing-slashes",
         "symbolic-link", "update", "verbose"},
        {"suffix"},
    ),
    "install": (
        "bcCDpsTvZ",
        "mogS",
        {"backup", "compare", "no-target-directory", "preserve-timestamps",
         "preserve-context", "strip", "verbose"},
        {"mode", "owner", "group", "suffix", "strip-program", "context"},
    ),
    # rsync's table is deliberately small. It is also currently inert: rsync IS
    # in MUTATORS, so an rsync command reaches the deny chain and this table
    # decides the exemption. Kept correct and conservative.
    "rsync": (
        "aAcdDgGhHiklLmnoOpPqrRsStuUvWxXzZ0",
        "efMBT",
        {"archive", "checksum", "compress", "delete", "delete-after",
         "delete-before", "delete-during", "delete-excluded", "dirs", "dry-run",
         "existing", "group", "hard-links", "human-readable", "ignore-existing",
         "ignore-times", "itemize-changes", "links", "no-perms", "numeric-ids",
         "omit-dir-times", "one-file-system", "owner", "partial", "perms",
         "progress", "prune-empty-dirs", "quiet", "recursive", "relative",
         "size-only", "sparse", "stats", "times", "update", "verbose",
         "whole-file"},
        {"exclude", "include", "exclude-from", "include-from", "files-from",
         "filter", "rsh", "rsync-path", "chmod", "chown", "timeout", "bwlimit",
         "max-size", "min-size", "block-size", "temp-dir", "backup-dir",
         "suffix", "compare-dest", "copy-dest", "link-dest", "log-file",
         "partial-dir", "info", "debug", "out-format"},
    ),
}


def _lands_outside_protected(cfg, token, cwd):
    """True only when `token` DEMONSTRABLY resolves outside every protected root.

    Anything it cannot resolve — a relative path with no cwd to anchor it, an
    rsync `host:path` remote spec — is False, i.e. treated as protected. This is
    the one direction that must never be optimistic: it is the test applied to
    the DESTINATION operand, to every option value, and (since 2026-09-06) to
    every redirect target, and a wrong True here is a real write let through.

    The leading substring test is deliberately blunt and deliberately kept: a
    token that merely CONTAINS a protected root refuses the exemption without
    further argument. It over-refuses on a sibling like
    `app-scratch`, and that is the safe direction for a write
    target — unlike the cwd axis, where the same over-reach denies a whole
    unrelated directory's worth of work and is fixed in `_inside_protected`.

    `not cwd.startswith("/")` comes from the JS copies, which had it while all
    four Python copies checked only `not cwd`. With a RELATIVE cwd, the Python
    joined and normalised to a relative path, which can never equal an absolute
    root — so it returned True ("outside") and let the write through. The JS was
    right; this is the union, not a new rule.
    """
    if any(root in token or root in _canon_text(token) for root in cfg["roots"]):
        return False
    if not _is_abs(token):
        # `host:path` / `rsync://` — a remote spec this hook cannot resolve.
        if ":" in token.split("/")[0]:
            return False
        if not cwd or not _is_abs(cwd):
            return False
        token = _join(cwd, token)
    return not _inside_any(cfg, token)


def _copy_operands(cfg, argv, name, cwd):
    """Split argv[1:] into operands, or None if anything is unrecognised.

    None means "this hook does not understand this command line", which is
    always a refusal — never an allow.
    """
    self_short, value_short, self_long, value_long = _COPY_FLAGS[name]
    order_flags = _COPY_ORDER_FLAGS[name]
    operands = []
    i, n = 1, len(argv)
    while i < n:
        tok = argv[i]
        if tok == "--":
            operands.extend(argv[i + 1:])
            return operands
        if not tok.startswith("-") or tok == "-":
            operands.append(tok)
            i += 1
            continue
        if tok.startswith("--"):
            head, _, attached = tok.partition("=")
            has_attached = "=" in tok
            if head in order_flags:
                return None
            long_name = head[2:]
            if has_attached:
                # Attached value: never confusable with an operand, but it can
                # still BE a write target, so it is checked like one.
                if long_name not in self_long and long_name not in value_long:
                    return None
                if not _lands_outside_protected(cfg, attached, cwd):
                    return None
                i += 1
                continue
            if long_name in self_long:
                i += 1
                continue
            if long_name in value_long:
                if i + 1 >= n:
                    return None
                if not _lands_outside_protected(cfg, argv[i + 1], cwd):
                    return None
                i += 2
                continue
            return None
        # Short cluster, e.g. `-Rv`, `-m644`, `-m 644`.
        if tok in order_flags:
            return None
        j = 1
        consumed_value = False
        while j < len(tok):
            letter = tok[j]
            if "-" + letter in order_flags:
                return None
            if letter in value_short:
                rest = tok[j + 1:]
                if rest:
                    if not _lands_outside_protected(cfg, rest, cwd):
                        return None
                    i += 1
                else:
                    if i + 1 >= n:
                        return None
                    if not _lands_outside_protected(cfg, argv[i + 1], cwd):
                        return None
                    i += 2
                consumed_value = True
                break
            if letter not in self_short:
                return None
            j += 1
        if not consumed_value:
            i += 1
    return operands


def _is_copy_out_of_protected(cfg, probe, cwd):
    """(allowed, note) — allowed only for a copy whose destination is outside.

    `note` explains a refusal, and is empty when the command is not copy-shaped
    at all. It goes into the deny message so a denied `cp` says WHY the
    source-vs-destination exception did not apply.
    """
    if _NOT_SIMPLE.search(probe):
        head = probe.split()[0] if probe.split() else ""
        if _COPY_LIKE.match(head):
            return False, (
                " This looks like a copy, but it is not one simple command (it "
                "contains a redirect, a pipe, a chain, a substitution or an "
                "unexpanded variable), so the source-vs-destination exception "
                "does not apply — run the copy on its own.")
        return False, ""
    try:
        argv = shlex.split(probe)
    except ValueError:
        return False, ""
    if not argv:
        return False, ""
    m = _COPY_LIKE.match(argv[0])
    if not m:
        return False, ""
    # Only an argv[0] that NAMES a path can name a protected one; a bare `cp` is
    # resolved through PATH, and resolving it against the cwd instead would
    # misreport "the binary is in the repo" for every copy run from inside one.
    if "/" in argv[0] and not _lands_outside_protected(cfg, argv[0], cwd):
        return False, " The `%s` being run is itself inside a protected tree." % argv[0]
    name = m.group(1)
    operands = _copy_operands(cfg, argv, name, cwd)
    if operands is None:
        return False, (
            " This is a `%s`, but it uses an option this guard does not parse "
            "(or one that moves the destination, like `-t`), so it cannot tell "
            "source from destination and refuses rather than guess. A plain "
            "`%s -R <src> <dst>` is exempt when only the source is protected."
            % (name, name))
    if len(operands) < 2:
        return False, (
            " This is a `%s` with fewer than two operands, so there is no "
            "destination to check." % name)
    dest = operands[-1]
    if not _lands_outside_protected(cfg, dest, cwd):
        return False, (
            " The DESTINATION operand (%s) is inside a protected tree. Copying "
            "OUT is allowed; copying IN is not." % dest)
    return True, ""


# A REDIRECT TARGET IS A SHELL WORD, NOT A RUN OF NON-SPACES. Measured
# 2026-09-24 on the first lane installed over a repo whose path has spaces
# (`~/projects/Some Project AI`): `echo x > '<repo>/README.md'` was
# ALLOWED, because `[^\s;|&()<>]+` stopped at the first space and resolved
# `'/Users/.../Financial` — outside every root.
#
# THE TARGETS ARE A UNION, and that is the safety property. The first draft
# replaced the old pattern with a word pattern, and Judgment Day (judge B,
# 2026-09-24) proved it fail-OPEN: a `>` INSIDE a quoted string started a match
# whose "word" swallowed the real redirect up to the next quote —
# `sed -n "/>/p" f > out.txt; echo "done"` from inside the repo went from deny
# to allow. So the old pattern's targets are all still asked, exactly as
# before, and a quote-aware scan ADDS the word after every `>` that is not
# inside quotes. A command can only gain targets to check, never lose one.
# deny-repo-writes.js carries the same two passes.
_WORD = r"""(?:"[^"]*"|'[^']*'|\\.|[^\s;|&()<>\\])+"""
_SHELL_WORD = re.compile(_WORD)
_BARE_REDIRECT_TARGET = re.compile(r"(?<![0-9&])>{1,2}(?!&)\s*([^\s;|&()<>]+)")


def _redirect_targets(probe):
    """Every redirect target in `probe`, unquoted: the base pattern's, plus
    the shell word after each `>` that sits outside quotes."""
    targets = [raw.strip("\"'") for raw in _BARE_REDIRECT_TARGET.findall(probe)]
    i, n, quote = 0, len(probe), None
    while i < n:
        c = probe[i]
        if quote:
            if c == quote:
                quote = None
            elif c == "\\" and quote == '"':
                i += 1
            i += 1
            continue
        if c in "\"'":
            quote = c
        elif c == "\\":
            i += 1
        elif c == ">" and not (i and (probe[i - 1] in "0123456789&")):
            j = i + 1
            if j < n and probe[j] == ">":
                j += 1
            if j < n and probe[j] == "&":
                i = j + 1
                continue
            while j < n and probe[j] in " \t":
                j += 1
            m = _SHELL_WORD.match(probe, j)
            if m:
                targets.append(_unquote_word(m.group(0)))
            i = j
            continue
        i += 1
    return targets


def _unquote_word(word):
    return re.sub(r""""([^"]*)"|'([^']*)'|\\(.)|["']""",
                  lambda m: next((g for g in m.groups() if g is not None), ""), word)


def _redirect_lands_in_protected(cfg, probe, cwd):
    """(raw_target, resolved_path) for the first redirect target that lands
    inside a protected root, or None if every target resolves outside.

    `cat app/x > brain/note.md` used to be denied: the command
    named a protected tree and contained a `>`, so reading FROM the repo INTO
    brain tripped the rule. A false positive here is not harmless — it teaches
    the agent to route around the guard.

    Every copy used to answer this with its own `t.startswith(REPO)` — a raw
    substring test on a WRITE TARGET, exactly what `_lands_outside_protected`
    exists to refuse. They are one question now, so a symlinked or `..`-laden
    redirect target is resolved rather than pattern-matched, and a relative
    target resolves against the cwd instead of being blanket-denied. When a
    target cannot be resolved, the shared predicate still falls back to denying.

    RETURNING THE RESOLVED PATH, not just a boolean, is what lets the caller
    NAME the destination it detected instead of gesturing at "the command
    names a protected root". Measured 2026-09-09: with only a boolean, the
    deny message fell back to that sentence even when the match came from
    resolving `$HOME` or a symlink — a sentence that reads like a
    literal-string test this branch does not run, and points the reader at
    the wrong thing to change.
    """
    for t in _redirect_targets(probe):
        if not _lands_outside_protected(cfg, t, cwd):
            resolved = t if _is_abs(t) else _norm(_join(cwd or ".", t))
            if _is_abs(resolved):
                resolved = _real(resolved)
            return t, resolved
    return None


# ── THE DECISION ────────────────────────────────────────────────────────────


def decide(cfg, command, cwd):
    """None to allow, or (rule, reason) to refuse.

    A pure function of (lane config, command, cwd), so a harness can drive it
    directly. `main` below is the thin part that speaks a vendor's protocol.
    """
    if not isinstance(command, str) or not command:
        return None

    # Match against an EXPANDED copy. The check used to be a literal
    # absolute-path substring test, so `~/...` and `$HOME/...`
    # sailed straight through — and the tilde form is how an agent naturally
    # writes the path, not an exotic evasion. The shell expands these after the
    # hook has already decided, so the hook has to expand them first.
    home = _shell_home()
    probe = command.replace("$HOME", home).replace("${HOME}", home)
    probe = re.sub(r"(?<![\w~])~/", lambda _m: home + "/", probe)  # a home is not a template: C:\Users is "\U"

    # `no_heredoc` drops heredoc body lines (data, not command text) but keeps
    # quotes intact, so a real redirect target that happens to be quoted is
    # still resolvable. `masked` additionally blanks quoted-string interiors,
    # so a mutator word or a `>` that only exists inside a string literal (a
    # grep pattern, a heredoc's own prose) cannot trip the command regexes.
    # Path detection (names_protected, cds_into_protected, teardown, redirect
    # TARGET resolution) stays on the unmasked text — it needs the real path,
    # quoted or not.
    no_heredoc = _strip_heredoc_bodies(probe)
    masked, balanced = _mask_quotes(no_heredoc)
    if not balanced:
        # Unbalanced quotes: the masking is not trustworthy, so do not mask.
        masked = no_heredoc

    # EVERY protected root, not just the lane's own pair. Iterating `cfg["roots"]`
    # is what makes `also_protect` reach these two literal gates; keying them on
    # `repo`/`worktrees` alone is exactly how three lanes let a write to another lane's repo
    # through while the table already listed the tree.
    roots = cfg["roots"]

    # A command is dangerous if it can write AND it can land in a protected tree,
    # either because it names one or because it is already standing in one.
    #
    # SCANNED ON `no_heredoc`, NOT `probe`. Measured 2026-09-09: a `git commit`
    # whose message was fed by `cat <<'EOF' ... EOF` (data, not code — the
    # heredoc feeds `cat`, so it never reaches a shell) cited a protected root
    # as evidence for a decisions.md entry, and scanning the raw command text
    # made that citation indistinguishable from a real destination — the
    # commit's ACTUAL destination is the cwd's own repo, never text sitting in
    # its message. A heredoc body is always DATA for this question: no verb's
    # write destination is ever spelled inside the payload it is asked to
    # write, only in its own operands or the shell's cwd. The verb regexes
    # below already read `masked`, which is heredoc-stripped for exactly this
    # reason (see `_strip_heredoc_bodies`); this gate had been left reading the
    # unstripped text, so it kept the false trigger alive even once the verb
    # match itself had stopped seeing it.
    # AND IN THE SHELL'S OWN SPELLING OF IT. A root with a space in it is
    # written `Financial\ Analyst\ AI` as often as it is quoted, and the
    # literal test above never matched that (measured 2026-09-24, the first
    # lane over such a repo: `echo x >> <repo-with-escapes>/README.md` was
    # allowed). Removing backslash escapes only ever ADDS matches.
    unescaped = re.sub(r"\\(.)", r"\1", no_heredoc)
    no_heredoc_c, unescaped_c = _canon_text(no_heredoc), _canon_text(unescaped)
    named_root = next((root for root in roots
                       if root in no_heredoc or root in unescaped
                       or root in no_heredoc_c or root in unescaped_c), None)
    names_protected = named_root is not None
    stands_in_protected = _inside_protected(cfg, cwd)
    # `cd` into a protected tree counts even when cwd is elsewhere.
    cds_pattern = r"cd\s+[\"']?(%s)" % "|".join(re.escape(r) for r in roots)
    cds_into_protected = (re.search(cds_pattern, no_heredoc)
                          or re.search(cds_pattern, no_heredoc_c))
    # And the same question asked of paths that do not SPELL a protected root —
    # a symlink into one, or a `..` traversal back into one. Skipped when a
    # literal already opened the gate, so the common case costs nothing.
    resolved_token = None
    if not names_protected:
        resolved_token = _resolves_protected(cfg, no_heredoc)

    if not (names_protected or resolved_token or stands_in_protected
            or cds_into_protected):
        return None

    # A CROSS-LANE DENIAL MUST NAME THE TREE IT IS PROTECTING. `where` is the
    # lane's own prose ("the lane-a checkout…"), and it is the wrong sentence
    # when a `setup` brainer is stopped from writing into another lane's repo. Naming the
    # actual root is the difference between a reader who understands the rule
    # and one who thinks the guard is misconfigured and routes around it.
    where = cfg["where"]
    if named_root is not None and named_root not in (cfg["repo"], cfg["worktrees"]):
        where = "another lane's protected tree (%s)" % named_root

    # Name WHICH condition tripped. A deny that only says "this command
    # mutates the repo" sends the reader to inspect the command text — and
    # when the trigger was the shell's cwd (which persists across Bash calls
    # and never appears in the command itself), there is nothing there to
    # find. That cost a brainer ~50 minutes diagnosing a phantom regression
    # on 2026-08-25 when the real cause was a leftover `cd` from earlier in
    # the session.
    triggers = []
    if stands_in_protected:
        triggers.append(
            "the shell's cwd is inside it (%s) — this is NOT in the command "
            "text above; the cwd persists across Bash calls, so an earlier "
            "`cd` is still in effect. Leave it with a `cd` run as its own "
            "call: `cd %s`" % (cwd, home))
    if names_protected:
        triggers.append("the command names a protected root (%s)" % named_root)
    if resolved_token:
        triggers.append(
            "a path in the command RESOLVES inside it (%s -> %s) even though the "
            "root does not appear literally — a symlink or a `..` traversal"
            % (resolved_token, _real(resolved_token)))
    if cds_into_protected:
        triggers.append("the command itself cd's into it")
    trigger_reason = "; and ".join(triggers)

    # Cleaning up after `hw done` is maintenance, not a repo write. Checked
    # before the git deny below, and narrow enough that nothing else fits it.
    # Inert on a lane whose `worktree_roots` is empty.
    if _is_spent_worktree_teardown(cfg, probe):
        return None

    # Copying OUT of a protected tree is a READ of it — in `cp SRC DST` only
    # DST is written. Checked here, ahead of the deny chain, and only when the
    # sole reason we got this far is that the command NAMES a protected path:
    # when the shell is standing in one, or the command cd's into one, a
    # RELATIVE destination can land inside it without ever naming it, and this
    # cannot tell.
    copy_out_ok, copy_note = _is_copy_out_of_protected(cfg, probe, cwd)
    if copy_out_ok:
        if not (stands_in_protected or cds_into_protected):
            return None
        copy_note = (
            " This copy's destination is outside the protected trees, but the "
            "shell's cwd is inside one (or the command cd's into one), so a "
            "relative operand could still land inside one without naming it. "
            "Re-run it from outside.")

    # A CONTENT-ONLY MATCH IS NOT A CONFIRMED DESTINATION. `named_root` and
    # `resolved_token` answer "does a protected path appear in this command's
    # CODE", never "is it this command's write target" — and a path can appear
    # in code as a quoted argument that is pure prose (a citation, a commit
    # message) rather than an operand. When the match text survives
    # quote-masking it sat in plain, unquoted text — the shape every real
    # operand has — and the trigger stays confident. When masking blanked it
    # away, the only reason we are here is a quoted string, and the honest
    # answer is "a protected path is CITED here", not "targeting X". (A
    # citation inside a heredoc body never reaches this point at all: it was
    # already excluded from `named_root`/`resolved_token` above.)
    content_only = (
        not stands_in_protected and not cds_into_protected
        and ((named_root is not None and named_root not in masked
              and named_root not in _canon_text(masked))
             or (resolved_token is not None and resolved_token not in masked))
    )

    if GIT_MUTATORS.search(masked):
        if content_only:
            return ("git",
                    "Blocked: this git command's text contains the path of %s "
                    "(%s), but a git subcommand's actual destination is its cwd "
                    "(or an explicit -C/--git-dir/--work-tree), never text "
                    "elsewhere in the command — and this match sits inside a "
                    "quoted argument, not one of those. I could not confirm "
                    "whether this is a real destination or a citation, and "
                    "refuse rather than guess. If you are only citing the path "
                    "as evidence, move it into a heredoc body instead of a "
                    "quoted argument (e.g. `git commit -F -` fed by "
                    "`cat <<'EOF' ... EOF`): a heredoc that does not feed a "
                    "shell is already read as data, not as this trigger."
                    % (where, trigger_reason))
        return ("git",
                "Blocked: this git subcommand mutates %s (%s). %s"
                % (where, trigger_reason, cfg["git_tail"]))
    if INPLACE.search(masked):
        if content_only:
            return ("inplace",
                    "Blocked: an in-place edit's command text contains the "
                    "path of %s (%s), sitting inside a quoted argument rather "
                    "than in the file operand itself. I could not confirm "
                    "whether this is the edited file or a citation, and "
                    "refuse rather than guess." % (where, trigger_reason))
        return ("inplace",
                "Blocked: in-place edit targeting %s (%s). The brainer is "
                "read-only there." % (where, trigger_reason))
    if REDIRECT.search(masked):
        redirect_hit = _redirect_lands_in_protected(cfg, no_heredoc, cwd)
        if redirect_hit:
            raw_target, resolved_target = redirect_hit
            return ("redirect",
                    "Blocked: shell redirect while targeting %s (%s). This "
                    "redirect's destination resolves to %s (written as "
                    "`%s` in the command). This is the exact hole that "
                    "path-based deny rules do not cover. Write to %s "
                    "instead. %s"
                    % (where, trigger_reason, resolved_target, raw_target,
                       cfg["write_here"], cfg["redirect_tail"]))
    mutator_invoked = any(
        _is_command_position(masked, m.start())
        for m in MUTATORS.finditer(masked)
    )
    if mutator_invoked:
        if content_only:
            return ("mutator",
                    "Blocked: this command's text contains the path of %s "
                    "(%s), sitting inside a quoted argument rather than in a "
                    "plain operand. That is as often a citation (evidence "
                    "pasted into a decisions.md entry) as a real destination, "
                    "and I could not confirm which. Refusing is the safe "
                    "default. If you are only citing the path, move it into a "
                    "heredoc body instead of a quoted argument: a heredoc "
                    "that does not feed a shell is already read as data, not "
                    "as this trigger. If you are instead trying to PRESERVE "
                    "data by copying it OUT of a protected tree, run `cp -R`, "
                    "`rsync -a` or `install` directly rather than through an "
                    "interpreter — those parse source from destination and "
                    "are already exempt when only the source is protected."
                    % (where, trigger_reason))
        return ("mutator",
                "Blocked: file-mutating command targeting %s (%s). The brainer "
                "is read-only there.%s" % (where, trigger_reason, copy_note))
    return None


# ── VENDOR PROTOCOLS ────────────────────────────────────────────────────────


def deny(reason):
    """Claude Code's PreToolUse refusal: JSON on stdout, exit 0."""
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": reason,
        }
    }))
    sys.exit(0)


def main(lane, stream=None):
    """Claude Code entry point. Every lane shim calls exactly this.

    The hook only covers `Bash`, and that is not an oversight: a shell redirect
    is not a path, so `Edit()`/`Write()` deny rules cannot see it and this hook
    cannot see an `Edit()`. The guard is two halves, and the other half is the
    `permissions.deny` list in each lane's `.claude/settings.json`. `0c51247`
    emptied that half and it went unnoticed for three days. Do not remove it.
    """
    try:
        payload = json.load(stream or sys.stdin)
    except Exception:
        sys.exit(0)  # Never block on a malformed payload.

    if payload.get("tool_name") != "Bash":
        sys.exit(0)

    # AN UNKNOWN LANE DENIES; IT DOES NOT CRASH. Letting `config`'s KeyError
    # propagate was the first draft, and it is fail-OPEN: Claude Code treats a
    # PreToolUse hook that exits non-zero with anything other than 2 as a
    # non-blocking error and runs the command anyway. So a shim and this table
    # disagreeing — the one way this can break — would have removed the guard
    # silently, which is the precise failure mode the whole file is about.
    try:
        cfg = config(lane)
    except KeyError as exc:
        deny("Blocked: %s Every Bash command is refused until a lane this guard "
             "knows is named, because a guard that cannot resolve its lane "
             "cannot tell a protected tree from any other directory."
             % exc.args[0])
        return

    command = payload.get("tool_input", {}).get("command", "") or ""
    cwd = payload.get("cwd", "") or ""

    # AND A CRASH INSIDE `decide` DENIES TOO. The shim already fails closed when
    # the shared module cannot be IMPORTED; nothing covered it raising at
    # DECISION time. Measured 2026-09-07 by a Judgment Day judge: make `decide`
    # raise, and the hook exits 1 with an empty stdout — which Claude Code
    # treats as a non-blocking error, so `rm -rf <repo>/src` went through with
    # no deny anywhere. A guard that cannot reach a verdict has not observed
    # that the command is safe; it has observed nothing, and the third value
    # for a guard is refuse.
    try:
        verdict = decide(cfg, command, cwd)
    except Exception as exc:  # noqa: BLE001 — ANY failure here must deny
        deny("Blocked: the %s read-only guard CRASHED while deciding "
             "(%s: %s). It reached no verdict, and a guard that reached no "
             "verdict has not established that this command is safe. Every "
             "Bash command is refused until this is fixed."
             % (lane, type(exc).__name__, exc))
        return
    if verdict:
        deny(verdict[1])
    sys.exit(0)
