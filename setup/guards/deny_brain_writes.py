#!/usr/bin/env python3
"""The inverse guard: a PRODUCT executor does not write in brain.

`deny_repo_writes` keeps every brainer read-only over the product repos. This
is the other direction. brain (`brain_root` in `guards.json`) is its
operator's own project, and an executor dispatched on a product lane — every
lane whose table entry does not say `brain_guard: false` — has no business changing it, working from it, or carrying a
path into it back to a product repo.

IT DENIES BY DEFAULT. The first version asked "is this command a write into
brain?" by parsing Bash, and three rounds of review each found a new way to
write that the parser did not know (a relative path, a variable, a heredoc
message, a push of every branch). That space is open. The question this file
asks instead is closed: "does this command NAME brain, and if it does, is that
one of the two allowed ways?"

  1. As a program of brain's bin/ run by its path — `<brain>/bin/done-invoker`
     and the rest of BIN_TOOLS, optionally behind an env assignment or a
     wrapper (`timeout 600 <brain>/bin/ask-invoker …`).
  2. As an operand of a verb from a closed list of readers — `cat`, `rg`,
     `sed` without an in-place flag … (READ_VERBS), with no output redirect
     into brain.

Anything else that names brain is refused: `cd` into it, a write tool, a
relative path that resolves into it (`../../brain/x`), a variable whose value
does (`"$OPENCODE_CONFIG"`, or one assigned earlier in the same command), a
glob or brace that expands into it, an interpreter or `git` or `cp` given a
brain path. A command whose shell stands in brain is refused whole.

"Names" is measured after the expansions the guard can see: `~`, `$HOME`,
every variable of the executor's own environment and every assignment earlier
in the command, braces, globs, `..`, symlinks. What it cannot see is code that
COMPUTES a path at run time (`$(printf …)`, `eval`, a script that builds one).
That is the stated limit: this guard stops an executor that follows a wrong
instruction, not one that works to evade it. It is not a sandbox.

LEAKS ARE JUDGED ON WHAT LEAVES, NOT ON THE COMMAND. On any `git push` (and on
`hw done`, see bin/hw), every object no remote has yet —
`git log -p --all --not --remotes`, messages included — is scanned for a
brain path. How the commit was made (`commit -m "$(cat <<EOF…)"`, `merge`,
`commit-tree` + `update-ref`, `push --all`) does not matter.

WHAT A PRODUCT EXECUTOR NEEDS FROM BRAIN, measured before designing: reading
its brief by path, and running done-invoker, ask-invoker, channel-send, hw,
browser-close and herdr-rpc by path. Their state is keyed off `HW_WORKDIR` or
`TMPDIR`, never brain, so no write exception exists.

WHO LOADS IT. Nobody by directory. `hw` registers it at launch for a product
executor only — Claude through `--plugin-dir setup/guards/brain-guard`,
opencode through `OPENCODE_CONFIG=setup/guards/brain-guard.opencode.json`,
whose plugin asks THIS file for its verdict. A `setup` executor, a brainer and
the operator never load it; a Codex executor does not either. The guard's own
files live in brain, so they are out of the executor's reach by the same rule.

FAILS CLOSED. An import failure, a crash while deciding, and a leak scan that
cannot finish all deny.
"""
import glob
import json
import os
import re
import subprocess
import sys
import time

# THE IMPORT IS GUARDED: a hook that dies with a traceback exits 1, which
# Claude Code reads as a non-blocking error — the command would run.
try:
    sys.path.insert(0, os.path.dirname(os.path.realpath(__file__)))
    import deny_repo_writes as drw  # noqa: E402
except Exception as _exc:  # noqa: BLE001 — ANY failure here must deny
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "PreToolUse", "permissionDecision": "deny",
        "permissionDecisionReason": (
            "Blocked: the brain guard could not load deny_repo_writes or its "
            "policy (%s: %s). It reached no verdict, and a guard that reached "
            "no verdict has not established that this is safe."
            % (type(_exc).__name__, _exc))}}))
    sys.exit(0)

WRITE_TOOLS = {
    # Claude Code
    "Write": ("file_path",), "Edit": ("file_path",), "MultiEdit": ("file_path",),
    "NotebookEdit": ("notebook_path",),
    # opencode
    "write": ("filePath",), "edit": ("filePath",), "multiedit": ("filePath",),
}
PATCH_TOOLS = {"patch", "apply_patch"}
BASH_TOOLS = {"Bash", "bash"}

# brain's programs a product executor runs by path, measured on the real calls
# that named brain: done-invoker, ask-invoker, browser-close, hw, herdr-rpc,
# plus channel-send, which the invokers document. Not every bin/ program is
# here: `decisions archive --apply`, for one, writes into brain.
BIN_TOOLS = {"done-invoker", "ask-invoker", "channel-send", "hw",
             "browser-close", "herdr-rpc"}

# Other programs of brain a product executor runs, measured, one per line in
# `setup/brain-guard-programs.txt` beside this directory: they name lanes and
# systems, and this directory names none (setup/tests/195). A line is
#   <path relative to brain> [via=<interpreter>] [first=<required first arg>]
PROGRAMS_FILE = os.path.join(os.path.dirname(os.path.dirname(os.path.realpath(__file__))),
                             "brain-guard-programs.txt")
INTERPRETERS = {"python3", "python", "bash", "sh", "zsh", "node", "perl", "ruby"}


def load_programs(path=None):
    out = {"bin/" + t: (None, None) for t in BIN_TOOLS}
    try:
        lines = open(path or PROGRAMS_FILE).read().splitlines()
    except OSError:
        return out
    for line in lines:
        parts = line.split("#", 1)[0].split()
        if not parts:
            continue
        opts = dict(p.split("=", 1) for p in parts[1:] if "=" in p)
        out[parts[0].strip("/")] = (opts.get("via"), opts.get("first"))
    return out


# The readers. A verb is here only if no flag of it writes, or the flags that
# do are refused in _read_verb_ok.
READ_VERBS = {"cat", "bat", "rg", "grep", "egrep", "fgrep", "sed", "head",
              "tail", "wc", "ls", "eza", "fd", "find", "test", "[", "stat",
              "file", "jq", "echo", "printf", "realpath", "readlink",
              "dirname", "basename", "which", "diff", "cmp", "sort", "uniq",
              "cut", "tr", "tree", "du", "shasum", "md5", "nl", "column"}
# Words that run the word after them: stripped before the verb is judged.
WRAPPERS = {"env", "command", "exec", "nohup", "time", "builtin", "nice"}
# Shell words that open or close a compound command; the verb is after them.
RESERVED = {"if", "then", "else", "elif", "fi", "do", "done", "while", "until",
            "!", "{", "}", "time"}

WHY = ("brain is its operator's own project: a product executor names it only "
       "to read (cat, rg, sed -n …) or to run its bin/ tools by path "
       "(done-invoker, ask-invoker, channel-send, hw, browser-close, herdr-rpc)")
HOW = ("Persist what you need in $HW_ARTIFACTS or in engram; to use a brain "
       "file, read it (e.g. `cat <brain>/x > \"$HW_ARTIFACTS/x\"`)")

MARK = "\x00"
LIST = "\x01"  # joins the words a `for` variable takes, one at a time
LOOP_OPEN, LOOP_CLOSE = "\x02", "\x03"  # around them, so `"$d/x"` is each word + /x  # a part of a word the guard cannot know before the shell runs


def brain_config(root=None, programs=None):
    root = drw._norm(root or drw.BRAIN_ROOT)  # on Windows the one canonical spelling
    # The guard's own files are protected with brain: an executor that could
    # edit them, or the config that loads them, could switch the guard off.
    own = os.path.dirname(os.path.realpath(__file__))
    cfg = {"root": root, "roots": (root, own)}
    cfg["resolved_roots"] = drw._resolve_roots(cfg["roots"])
    cfg["brains"] = drw._resolve_roots((root,))
    # The checkout this guard runs from carries the same bin/: an executor
    # launched from a setup worktree is handed ITS done-invoker.
    cfg["own_bin"] = os.path.join(os.path.dirname(os.path.dirname(own)), "bin")
    cfg["programs"] = load_programs(programs)
    home = drw._policy_home()
    spellings = set(drw._resolve_roots((root,)))
    for s in list(spellings):
        if s.startswith(home + "/"):
            # `~/<x>`, `$HOME/<x>` and a bare `<x>` all carry this tail.
            spellings.add(s[len(home) + 1:])
    # `/+`: `$TMPDIR/x` is as often written with a `//` in it.
    cfg["spell"] = re.compile(
        r"(?<![\w.-])(?:%s)(?![\w.-])"
        % "|".join("/+".join(re.escape(p) for p in s.strip("/").split("/"))
                   for s in sorted(spellings, key=len, reverse=True)))
    return cfg


def _refuse(rule, what):
    return (rule, "Blocked (brain guard, rule '%s'): %s. The rule: %s. %s."
            % (rule, what, WHY, HOW))


def _inside(cfg, path):
    # drw._is_abs, not startswith("/"): on Windows C:\x and C:/x are absolute
    # too, and a test that knew only "/" answered them "outside" — an allow.
    return bool(path) and drw._is_abs(path) and drw._inside_any(cfg, drw._norm(path))


# ── the shell, read as far as it can be without running it ─────────────────

class Seg:
    """One simple command: its words, its redirects, its heredoc data."""
    def __init__(self):
        self.words = []       # (value, raw) — value has MARK where unknowable
        self.redirects = []   # (op, value)
        self.data = []        # heredoc / here-string bodies fed to it
        self.after_pipe = False  # its stdin is the previous segment's stdout


def _brace(word):
    """`a{b,c}d` → [abd, acd], nested and repeated; anything else as is."""
    m = re.search(r"\{([^{}]*,[^{}]*)\}", word)
    if not m:
        return [word]
    out = []
    for alt in m.group(1).split(","):
        out += _brace(word[:m.start()] + alt + word[m.end():])
    return out[:64]


class Shell:
    def __init__(self, text, env):
        self.text, self.env, self.i = text, env, 0
        self.segs, self.local = [], {}

    # variables: this command's own assignments first, then the environment
    def var(self, name):
        if name in self.local:
            return self.local[name]
        if name in self.env:
            return self.env[name]
        return MARK

    def parse(self):
        self._list(end=None)
        return self.segs

    def _list(self, end):
        seg, word, raw, have = Seg(), [], [], False
        pending_redirect = None
        heredocs = []
        t = self.text

        def flush_word():
            nonlocal word, raw, have, pending_redirect
            if not have:
                return
            value, r = "".join(word), "".join(raw)
            if pending_redirect is not None:
                if pending_redirect.startswith("<<") and pending_redirect != "<<<":
                    heredocs.append((seg, r.strip("'\"").lstrip("-"), pending_redirect == "<<-"))
                elif pending_redirect == "<<<":
                    seg.data.append(value)
                else:
                    seg.redirects.append((pending_redirect, value))
                pending_redirect = None
            else:
                if not seg.words and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", r):
                    self.local[r.split("=", 1)[0]] = value.split("=", 1)[1]
                seg.words.append((value, r))
            word, raw, have = [], [], False

        piped = [False]

        def flush_seg():
            nonlocal seg
            flush_word()
            w = [r for _, r in seg.words]
            if len(w) >= 3 and w[0] == "for" and w[2] == "in":
                # `for f in <words>; do … "$f"`: the variable holds each word.
                self.local[w[1]] = LOOP_OPEN + LIST.join(v for v, _ in seg.words[3:]) + LOOP_CLOSE
                seg.words = []
            if seg.words or seg.redirects or seg.data:
                # An empty flush (`|&`, `|` then a newline) keeps the pipe
                # pending for the command that actually follows it.
                seg.after_pipe, piped[0] = piped[0], False
                self.segs.append(seg)
            seg = Seg()

        while self.i < len(t):
            c = t[self.i]
            if end and c == end:
                flush_seg(); self.i += 1; break
            if c == "\\" and self.i + 1 < len(t):
                if t[self.i + 1] == "\n":
                    self.i += 2; continue
                word.append(t[self.i + 1]); raw.append(t[self.i:self.i + 2]); have = True
                self.i += 2; continue
            if c == "'":
                j = t.find("'", self.i + 1)
                j = len(t) if j < 0 else j
                word.append(t[self.i + 1:j]); raw.append(t[self.i:j + 1]); have = True
                self.i = j + 1; continue
            if c == '"':
                self.i += 1
                start = self.i
                while self.i < len(t) and t[self.i] != '"':
                    if t[self.i] == "\\" and self.i + 1 < len(t):
                        word.append(t[self.i + 1]); self.i += 2; continue
                    if t[self.i] == "$" or t[self.i] == "`":
                        word.append(self._dollar()); continue
                    word.append(t[self.i]); self.i += 1
                raw.append(t[start - 1:self.i + 1]); have = True
                self.i += 1; continue
            if c == "$" or c == "`":
                s = self.i
                word.append(self._dollar()); raw.append(t[s:self.i]); have = True
                continue
            if c == "~" and not have:
                m = re.match(r"~([A-Za-z0-9_.-]*)(?=/|$|\s|[;&|)])", t[self.i:])
                if m:
                    word.append(os.path.expanduser("~" + m.group(1)) if m.group(1) else self.env.get("HOME") or os.path.expanduser("~"))
                    raw.append(m.group(0)); have = True; self.i += len(m.group(0)); continue
            if c in " \t":
                flush_word(); self.i += 1; continue
            if c == "#" and not have:
                nl = t.find("\n", self.i)
                self.i = len(t) if nl < 0 else nl
                continue
            if c == "\n":
                flush_seg(); self.i += 1
                for hseg, delim, dash in heredocs:
                    body = []
                    while self.i < len(t):
                        nl = t.find("\n", self.i)
                        nl = len(t) if nl < 0 else nl
                        line = t[self.i:nl]; self.i = nl + 1
                        if (line.lstrip("\t") if dash else line) == delim:
                            break
                        body.append(line)
                    hseg.data.append("\n".join(body))
                heredocs = []
                continue
            if c == "|" and t[self.i + 1:self.i + 2] == "|":
                flush_seg(); self.i += 2; continue
            if c in ";&|()":
                flush_seg()
                if c == "|":
                    piped[0] = True
                    if t[self.i + 1:self.i + 2] == "&":  # `|&` pipes stderr too
                        self.i += 1
                if c == "(":
                    self.i += 1; self._list(end=")"); continue
                self.i += 1
                continue
            m = re.match(r"(\d*)(>>|>\||&>>|&>|<>|>&|<<<|<<-|<<|<&|>|<)", t[self.i:])
            if m:
                flush_word()
                op = m.group(2)
                self.i += len(m.group(0))
                if op in (">&", "<&"):
                    # `2>&1` names a descriptor, not a file.
                    mm = re.match(r"\s*(\d+|-)", t[self.i:])
                    if mm:
                        self.i += len(mm.group(0)); continue
                    op = ">"
                while self.i < len(t) and t[self.i] in " \t":
                    self.i += 1
                pending_redirect = op
                continue
            word.append(c); raw.append(c); have = True; self.i += 1
        flush_seg()
        for hseg, _, _ in heredocs:  # a heredoc with no body line yet
            hseg.data.append(t[self.i:])

    def _dollar(self):
        """One `$…` or backtick expansion at self.i, returned as its value."""
        t = self.text
        if t[self.i] == "`":
            j = t.find("`", self.i + 1)
            j = len(t) if j < 0 else j
            Shell(t[self.i + 1:j], self.env).parse_into(self.segs, self.local)
            self.i = j + 1
            return MARK
        if t.startswith("$((", self.i):
            depth, j = 0, self.i + 1
            while j < len(t):
                if t[j] == "(":
                    depth += 1
                elif t[j] == ")":
                    depth -= 1
                    if depth == 0:
                        break
                j += 1
            self.i = j + 1
            return MARK
        if t.startswith("$(", self.i):
            sub = Shell(t, self.env)
            sub.i, sub.local = self.i + 2, self.local
            sub._list(end=")")
            self.segs += sub.segs
            self.i = sub.i
            return MARK
        m = re.match(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(?::?[-=]([^}]*))?[^}]*\}", t[self.i:])
        if m:
            self.i += len(m.group(0))
            v = self.var(m.group(1))
            return m.group(2) if v == MARK and m.group(2) is not None else v
        m = re.match(r"\$([A-Za-z_][A-Za-z0-9_]*)", t[self.i:])
        if m:
            self.i += len(m.group(0))
            return self.var(m.group(1))
        if re.match(r"\$[0-9@*#?$!-]", t[self.i:]):
            self.i += 2
            return MARK
        self.i += 1
        return "$"

    def parse_into(self, segs, local):
        self.local = local
        self._list(end=None)
        segs += self.segs


# ── the rule ────────────────────────────────────────────────────────────────

def _names(cfg, value, cwd):
    """Does this word name brain, however it is spelled?"""
    if not value:
        return False
    if LOOP_OPEN in value and LOOP_CLOSE in value:
        pre, rest = value.split(LOOP_OPEN, 1)
        mid, post = rest.split(LOOP_CLOSE, 1)
        return any(_names(cfg, pre + v + post, cwd) for v in mid.split(LIST))
    literal = value.replace(MARK, "")
    if _spells(cfg, literal):
        return True
    # `FOO=<path>` names <path>, and so does each part of `PATH=<a>:<b>`.
    m = re.match(r"^[A-Za-z_]\w*=(.+)$", value)
    if m and any(_names(cfg, part, cwd) for part in m.group(1).split(":") if part):
        return True
    for v in _brace(value):
        if MARK in v:
            # Unknowable before the shell runs. It names brain only through
            # its known part: a relative path climbing out is refused below.
            v = v.split(MARK)[0]
            if not v:
                continue
        if not ("/" in v or v in (".", "..") or v.startswith("~") or drw._is_abs(v)) and not cwd:
            continue
        p = v if drw._is_abs(v) else (drw._join(cwd, v) if cwd else None)
        if p is None:
            if v.startswith(".."):
                return True  # relative to a directory the guard cannot know
            continue
        if _inside(cfg, p):
            return True
        if re.search(r"[*?\[]", v):
            try:
                if any(_inside(cfg, g) for g in glob.glob(p)[:256]):
                    return True
            except re.error:
                pass
    return False


def _spells(cfg, text):
    """Does this text spell brain? On Windows the roots are canonical (lower
    case, `/c/`), so the text is read folded as well as as written."""
    return bool(cfg["spell"].search(text)
                or (drw._WINPATHS and cfg["spell"].search(drw._canon_text(text))))


def _data_names(cfg, text):
    """A heredoc or here-string body: its spellings and its absolute paths."""
    text = text.replace(MARK, "")
    if _spells(cfg, text):
        return True
    return any(_inside(cfg, p) for p in re.findall(r"/[^\s'\"`;|&<>()]+", text)[:512])


def _head(seg):
    """(index of the verb among seg.words, verb) past assignments and wrappers."""
    i, words = 0, seg.words
    while i < len(words) and words[i][1] in RESERVED:
        i += 1
    while i < len(words) and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", words[i][1]):
        i += 1
    while i < len(words):
        name = os.path.basename(words[i][0])
        if name in WRAPPERS:
            i += 1
            while i < len(words) and (words[i][0].startswith("-") or re.match(r"^[A-Za-z_]\w*=", words[i][1])):
                i += 1
            continue
        if name == "timeout":
            i += 1
            while i < len(words) and words[i][0].startswith("-"):
                i += 1
            i += 1  # the duration
            continue
        break
    return i, (words[i][0] if i < len(words) else "")


def _flag(args, letters, longs=()):
    """A short flag among `letters`, alone or in a cluster (`-uo`, `-HX`), or
    a long one starting with one of `longs`."""
    for a in args:
        if a.startswith("--"):
            if longs and a.startswith(tuple(longs)):
                return True
        elif re.match(r"^-[A-Za-z0-9]*[%s]" % letters, a):
            return True
    return False


# sed is a reader only for scripts made of addresses and non-writing commands.
# A closed grammar, not a search for `w`: sed accepts `w<file>`, `gw`, `-e<s>`,
# `--expression=<s>` and more, and each was a way past a search.
_SED_S = re.compile(r"s(?P<d>[^\\\n\s])(?:\\.|(?!(?P=d)).)*(?P=d)"
                    r"(?:\\.|(?!(?P=d)).)*(?P=d)[gpiIM0-9]*")
_SED_Y = re.compile(r"y(?P<d>[^\\\n\s])(?:\\.|(?!(?P=d)).)*(?P=d)(?:\\.|(?!(?P=d)).)*(?P=d)")
_SED_ADDR = re.compile(r"\\(?P<d>.)(?:\\.|(?!(?P=d)).)*(?P=d)I?|/(?:\\.|[^/])*/I?")
_SED_REST = re.compile(r"^[0-9$,;!{}\s~+pPdDqQnNlgGhHxz=]*$")


def _sed_script_reads(script):
    rest = _SED_ADDR.sub(" ", _SED_Y.sub(" ", _SED_S.sub(" ", script)))
    return bool(_SED_REST.match(rest))


def _sed_reads(args):
    scripts, i, explicit = [], 0, False
    operands = []
    while i < len(args):
        a = args[i]
        if a in ("-e", "--expression"):
            scripts.append(args[i + 1] if i + 1 < len(args) else ""); explicit = True; i += 2; continue
        if a.startswith("--expression="):
            scripts.append(a.split("=", 1)[1]); explicit = True; i += 1; continue
        if a.startswith("--"):
            if a.startswith(("--in-place", "--file")):
                return False
            i += 1; continue
        if a.startswith("-") and len(a) > 1:
            flags = a[1:]
            if re.search(r"[iIf]", flags.split("e", 1)[0]):
                return False  # in place, or a script from a file the guard cannot read
            if "e" in flags:
                attached = flags.split("e", 1)[1]
                if attached:
                    scripts.append(attached)
                else:
                    scripts.append(args[i + 1] if i + 1 < len(args) else ""); i += 1
                explicit = True
            i += 1; continue
        operands.append(a); i += 1
    if not explicit and operands:
        scripts.append(operands[0])
    return all(_sed_script_reads(sc) for sc in scripts)


def _read_verb_ok(verb, args):
    """False when a reader is asked for one of its writing flags."""
    if verb == "sed":
        return _sed_reads(args)
    if verb == "fd":
        return not _flag(args, "xX", ("--exec",))
    if verb == "find":
        return not any(a in ("-delete", "-exec", "-execdir", "-ok", "-okdir", "-fls")
                       or a.startswith("-fprint") for a in args)
    if verb == "rg":
        return not any(a.startswith("--pre") for a in args)
    if verb == "sort":
        return not _flag(args, "o", ("--output",))
    if verb == "tree":
        return not _flag(args, "o", ("--output",))
    if verb == "uniq":
        # `uniq in out` writes its second operand.
        return len([a for a in args if not a.startswith("-")]) <= 1
    return True


def _judge(cfg, seg, cwd, local=None):
    """None, or a refusal, for one simple command."""
    for op, target in seg.redirects:
        if op.startswith("<") and op not in ("<>",):
            if _names(cfg, target, cwd):
                seg.reads_brain = True
            continue
        if _names(cfg, target, cwd):
            return _refuse("write", "this command redirects its output into brain (%s)"
                           % target.replace(MARK, "…"))
    idx, verb = _head(seg)
    args = [w for w, _ in seg.words[idx + 1:]]
    prefix = seg.words[:idx]
    names_in_args = any(_names(cfg, a, cwd) for a in args)
    names_in_prefix = any(_names(cfg, w, cwd) for w, _ in prefix)
    names_verb = _names(cfg, verb, cwd)
    name = os.path.basename(verb)
    data_names = any(_data_names(cfg, d) for d in seg.data)
    prog = _program(cfg, verb, name, args, cwd)
    if prog is not None:
        rel, via = prog
        want = cfg["programs"].get(rel)
        rest = args[1:] if via else args
        if want is None or want[0] != via or (want[1] and (rest[:1] != [want[1]])):
            return _refuse("run", "`%s` is not one of the brain programs a product "
                           "executor runs (%s)" % (rel, _programs_named(cfg)))
        names_in_args = names_in_args and via is None
        names_verb = False
        if not names_in_prefix or _prefix_ok(cfg, prefix, cwd):
            return None
    if name in ("export", "declare", "typeset", "readonly", "local") and local:
        names_in_args = names_in_args or any(
            _names(cfg, local.get(a, ""), cwd) for a in args if "=" not in a)
    if not (names_in_args or names_in_prefix or names_verb or data_names
            or getattr(seg, "reads_brain", False)):
        return None
    # An assignment alone writes nothing; its value is judged where it is used.
    if not verb and not seg.redirects:
        return None
    if name in ("export", "declare", "typeset", "readonly", "local"):
        named = [a for a in args if "=" not in a and _names(cfg, local.get(a, "") if local else "", cwd)]
        if not named and all(_path_ok(cfg, a, cwd) for a in args if _names(cfg, a, cwd)):
            return None
        return _refuse("export", "this command exports a variable that points into brain, "
                       "and whatever it runs next would inherit it (only PATH may carry "
                       "brain's bin/)")
    if names_in_prefix and not _prefix_ok(cfg, prefix, cwd):
        return _refuse("env", "an environment assignment on this command points into brain (%s)"
                       % next(w for w, _ in prefix if _names(cfg, w, cwd)).replace(MARK, "…"))
    if names_verb:
        return _refuse("run", "`%s` is not one of the brain programs a product executor "
                       "runs (%s)" % (verb, _programs_named(cfg)))
    if name in ("cd", "pushd"):
        return _refuse("cd", "this command cd's into brain; work in your worktree or work "
                       "dir and read brain by path")
    if name in READ_VERBS and _read_verb_ok(name, args):
        return None
    if name == "git" and not names_in_args and data_names:
        return None  # a heredoc message: judged as a leak on push, not here
    return _refuse("verb", "`%s` is given a brain path, and only a reader may be "
                   "(cat, bat, rg, grep, sed -n, head, tail, ls, eza, fd, find, jq, diff …)"
                   % (name or verb))


def _same_dir(a, b):
    if drw._WINPATHS:
        return drw._real(a) == drw._real(b)
    return os.path.realpath(a) == b


def _rel(cfg, path):
    """`path` relative to brain, or None when it is not inside it."""
    # The roots are canonical on Windows (drw._canon), so the path is too.
    real = drw._real(path) if drw._WINPATHS else os.path.realpath(path)
    for r in cfg["brains"]:
        if real.startswith(r + "/"):
            return real[len(r) + 1:]
    return None


def _program(cfg, verb, name, args, cwd):
    """(path relative to brain, interpreter or None) when this command runs a
    program that lives in brain — by path, by a bare name brain's bin/ holds,
    or as a script handed to an interpreter — else None."""
    if not verb or MARK in verb:
        return None
    if "/" in verb:
        full = verb if verb.startswith("/") else os.path.join(cwd or "/", verb)
        if _same_dir(os.path.dirname(full), cfg["own_bin"]):
            return "bin/" + name, None
        rel = _rel(cfg, full)
        return (rel, None) if rel else None
    if name in INTERPRETERS:
        script = next((a for a in args if not a.startswith("-")), None)
        if script and MARK not in script and "/" in script:
            rel = _rel(cfg, script if script.startswith("/") else os.path.join(cwd or "/", script))
            if rel:
                return rel, name
        return None
    for d in [os.path.join(r, "bin") for r in cfg["brains"]] + [cfg["own_bin"]]:
        if os.path.isfile(os.path.join(d, verb)):
            return "bin/" + verb, None
    return None


def _programs_named(cfg):
    return ", ".join(sorted("bin/" + t for t in BIN_TOOLS)) + " and the ones in setup/brain-guard-programs.txt"


def _path_ok(cfg, word, cwd):
    """`PATH=…` whose every part inside brain is brain's own bin/."""
    if not word.startswith("PATH="):
        return False
    for part in word[5:].split(":"):
        real = drw._real(part) if drw._WINPATHS else os.path.realpath(part)
        if _names(cfg, part, cwd) and real not in {
                os.path.join(r, "bin") if not drw._WINPATHS else r + "/bin" for r in cfg["brains"]}:
            return False
    return True


def _prefix_ok(cfg, prefix, cwd):
    return all(_path_ok(cfg, w, cwd) for w, _ in prefix if _names(cfg, w, cwd))


def _seg_names(cfg, seg, cwd, local=None):
    return (any(_names(cfg, w, cwd) for w, _ in seg.words)
            or any(_names(cfg, t, cwd) for _, t in seg.redirects)
            or any(_data_names(cfg, d) for d in seg.data))


def _effective_cwds(cfg, segs, cwd):
    """The directory each segment runs in, as far as `cd` can be followed.
    None once a cd went somewhere the guard cannot resolve."""
    out, at = [], cwd
    for seg in segs:
        out.append(at)
        idx, verb = _head(seg)
        name = os.path.basename(verb)
        if name in ("cd", "pushd"):
            args = [w for w, _ in seg.words[idx + 1:] if not w.startswith("-") or w == "-"]
            target = args[0] if args else (os.environ.get("HOME") or "~")
            if target == "-" or MARK in target or at is None:
                at = None
            else:
                at = os.path.normpath(target if target.startswith("/") else os.path.join(at, target))
        elif name == "popd":
            at = None
    out.append(at)  # where the shell is left standing
    return out


# ── leaks: what leaves, not how it was made ─────────────────────────────────

LEAK_BUDGET = 40  # seconds; past it the scan denies rather than guess


def _git(top, args, left):
    r = subprocess.run(["git", "-C", top] + args, capture_output=True, text=True,
                       errors="replace", timeout=max(1, left))
    if r.returncode:
        raise RuntimeError("git %s: %s" % (args[0], (r.stderr or r.stdout).strip()[:200]))
    return r.stdout


def scan_unpushed(cfg, top, refs, deadline=None):
    """(ref, commit, line) for the first brain path in the objects reachable
    from `refs` that no remote has — diffs (merges against their first parent)
    and messages — or None. Raises when git fails or runs out of time: a scan
    that did not finish has not found the objects clean."""
    deadline = deadline or time.time() + LEAK_BUDGET
    for ref in refs:
        out = _git(top, ["log", "-p", "--diff-merges=first-parent", "--no-color",
                         "--no-ext-diff", "--format=@@commit %H%n%B", ref, "--not",
                         "--remotes"], deadline - time.time())
        commit, in_diff = "?", False
        for line in out.splitlines():
            if line.startswith("@@commit "):
                commit, in_diff = line[9:21], False
                continue
            if line.startswith("diff --git "):
                in_diff = True
                continue
            if in_diff and not (line.startswith("+") and not line.startswith("+++")):
                continue
            if _spells(cfg, line):
                return ref, commit, line.strip()[:160]
    return None


def _pushes(seg):
    words = [os.path.basename(w) if i == 0 else w for i, (w, _) in enumerate(seg.words)]
    if "git" in words and "push" in words[words.index("git") + 1:]:
        return "git"
    if "gh" in words and "pr" in words:
        return "gh"
    return None


def _repo_dirs(seg, cwd):
    dirs = [cwd] if cwd else []
    words = [w for w, _ in seg.words]
    for i, w in enumerate(words):
        if w == "-C" and i + 1 < len(words) and MARK not in words[i + 1]:
            d = words[i + 1]
            dirs.append(d if d.startswith("/") else os.path.join(cwd or "/", d))
    return dirs


def _pushed_refs(seg, top, left):
    """What this push sends: the local refs `git push --dry-run --porcelain`
    lists with the same arguments (without running the repo's own pre-push
    hook) — the list a pre-push hook reads on stdin,
    which a guard outside the repo cannot install a hook to read. A push the
    guard cannot replay (an argument it cannot know) is scanned as HEAD plus
    every local branch, the widest thing it could send."""
    words = [w for w, _ in seg.words]
    i = words.index("push") if "push" in words else -1
    if i < 0 or any(MARK in w for w in words[i + 1:]):
        return ["HEAD", "--branches"]
    out = _git(top, ["push", "--dry-run", "--porcelain", "--no-verify"] + words[i + 1:], left)
    refs = []
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) >= 2 and ":" in parts[1] and parts[0].strip() != "-":
            src = parts[1].split(":", 1)[0]
            if src:
                refs.append(src)
    return refs


def _leak(cfg, segs, cwds):
    deadline = time.time() + LEAK_BUDGET
    seen = set()
    for seg, cwd in zip(segs, cwds):
        kind = _pushes(seg)
        if not kind:
            continue
        for d in _repo_dirs(seg, cwd):
            if not os.path.isdir(d):
                continue
            r = subprocess.run(["git", "-C", d, "rev-parse", "--show-toplevel"],
                               capture_output=True, text=True, timeout=10)
            top = r.stdout.strip()
            if r.returncode or not top or top in seen or _inside(cfg, top):
                continue
            seen.add(top)
            try:
                refs = (_pushed_refs(seg, top, deadline - time.time()) if kind == "git"
                        else ["HEAD"])
                hit = scan_unpushed(cfg, top, refs, deadline)
            except (subprocess.TimeoutExpired, RuntimeError, OSError) as exc:
                return _refuse("leak", "what this push would send, in %s, could not be "
                               "scanned (%s), so it is not known to be clean" % (top, exc))
            if hit:
                return _refuse("leak", "this push sends %s, and its commit %s, which no "
                               "remote has, names brain (%s). Rewrite that commit before "
                               "pushing" % hit)
    return None


# ── the decision ────────────────────────────────────────────────────────────

def _patch_files(text):
    return re.findall(r"^\*\*\* (?:Add|Update|Delete) File: (.+)$|^\*\*\* Move to: (.+)$",
                      text or "", re.M)


def decide(cfg, tool, tool_input, cwd, env=None):
    """None to allow, or (rule, reason) to refuse."""
    env = dict(os.environ if env is None else env)
    tool_input = tool_input or {}
    # On Windows the caller may spell its cwd as a drive path with forward slashes (OpenCode passes its
    # session directory that way). Every relative word is joined onto it, and
    # a join that does not start with "/" is not a path to _inside: a relative
    # redirect out of the worktree into brain would pass. One spelling first.
    if drw._WINPATHS and cwd:
        cwd = drw._norm(cwd)
    if tool in WRITE_TOOLS or tool in PATCH_TOOLS:
        paths = [tool_input.get(k) for k in WRITE_TOOLS.get(tool, ())]
        if tool in PATCH_TOOLS:
            paths = [next(p for p in pair if p).strip() for pair in
                     _patch_files(tool_input.get("patchText") or tool_input.get("patch") or "")]
        for p in paths:
            if isinstance(p, str) and p:
                full = os.path.expanduser(p)
                full = full if drw._is_abs(full) else drw._join(cwd or "/", full)
                if _inside(cfg, drw._norm(full)):
                    return _refuse("write", "%s targets %s, inside brain" % (tool, p))
        return None
    if tool not in BASH_TOOLS:
        return None
    command = tool_input.get("command") or ""
    if not isinstance(command, str) or not command:
        return None
    shell = Shell(command, env)
    segs = shell.parse()
    cwds = _effective_cwds(cfg, segs, cwd)
    if cwd and _inside(cfg, cwd):
        # The one way out: a command that only cd's somewhere outside brain.
        # A redirect opens its file HERE, before the cd runs, so it may carry none.
        if segs and cwds[-1] and not _inside(cfg, cwds[-1]) and all(
                os.path.basename(_head(g)[1]) in ("cd", "pushd") and not g.redirects
                and not g.data for g in segs):
            return None
        return _refuse("cwd", "the shell's cwd is inside brain (%s); leave it with a "
                       "command that is only `cd <your worktree or work dir>`" % cwd)
    fed = False  # an earlier stage of this pipeline named brain
    for i, seg in enumerate(segs):
        here = cwds[i]
        verdict = _judge(cfg, seg, here, shell.local)
        if verdict:
            return verdict
        if cwds[i + 1] and _inside(cfg, cwds[i + 1]):
            return _refuse("cd", "this command cd's into brain (%s); work in your worktree "
                           "or work dir and read brain by path" % cwds[i + 1])
        name = os.path.basename(_head(seg)[1])
        if seg.after_pipe and fed and name in ("xargs", "parallel"):
            return _refuse("pipe", "`%s` is fed brain paths by the command before it, and "
                           "only a reader may be given them" % name)
        # Anything earlier in the same pipeline, not just the stage before.
        fed = (fed and seg.after_pipe) or _seg_names(cfg, seg, here, shell.local)
    return _leak(cfg, segs, cwds[:-1])


def main(stream=None, root=None):
    try:
        payload = json.load(stream or sys.stdin)
    except Exception:
        sys.exit(0)
    # A payload of the wrong shape refuses (audit F12, 2026-09-29): a Bash
    # `tool_input` that was null read as an empty command and was allowed.
    if not isinstance(payload, dict):
        drw.deny("Blocked: the brain guard was handed a payload that is not a "
                 "JSON object (%s), so it cannot tell which tool is running. "
                 "Refused rather than guessed." % type(payload).__name__)
        return
    tool = payload.get("tool_name", "")
    if tool in BASH_TOOLS | set(WRITE_TOOLS) | PATCH_TOOLS \
            and not isinstance(payload.get("tool_input"), dict):
        drw.deny("Blocked: the brain guard was handed a %s call whose tool_input "
                 "is not an object (%s), so it cannot read what it does. Refused "
                 "rather than guessed." % (tool, type(payload.get("tool_input")).__name__))
        return
    try:
        verdict = decide(brain_config(root), payload.get("tool_name", ""),
                         payload.get("tool_input", {}), payload.get("cwd", "") or "")
    except Exception as exc:  # noqa: BLE001 — ANY failure here must deny
        drw.deny("Blocked: the brain guard CRASHED while deciding (%s: %s). It "
                 "reached no verdict, and a guard that reached no verdict has "
                 "not established that this is safe." % (type(exc).__name__, exc))
        return
    if verdict:
        drw.deny(verdict[1])
    sys.exit(0)


def scan_main(argv, root=None):
    """`deny_brain_writes.py --scan <dir> [<branch>]`: hw done's leak check
    over what the task can carry out — HEAD and its branch. Exit 0 clean,
    1 leak (named on stdout), 2 could not tell."""
    cfg = brain_config(root)
    try:
        top = subprocess.run(["git", "-C", argv[0], "rev-parse", "--show-toplevel"],
                             capture_output=True, text=True, timeout=10).stdout.strip()
        if not top:
            return 0
        refs = ["HEAD"]
        if len(argv) > 1 and argv[1] and subprocess.run(
                ["git", "-C", top, "rev-parse", "--verify", "-q", "refs/heads/" + argv[1]],
                capture_output=True).returncode == 0:
            refs.append("refs/heads/" + argv[1])
        hit = scan_unpushed(cfg, top, refs)
    except Exception as exc:  # noqa: BLE001
        print("could not scan %s: %s: %s" % (argv[0], type(exc).__name__, exc))
        return 2
    if hit:
        print("%s carries commit %s, which names brain: %s" % hit)
        return 1
    return 0


if __name__ == "__main__":
    root = os.environ.get("HW_BRAIN_GUARD_ROOT") or None
    if sys.argv[1:2] == ["--scan"]:
        sys.exit(scan_main(sys.argv[2:], root))
    main(root=root)
