"""Native Windows Python, spoken to by Git Bash: one path spelling, LF newlines.

Python imports this module by itself at startup (`sitecustomize`) when this
directory is on PYTHONPATH, and `bin/msys-compat.sh` puts it there — under Git
Bash only. Everywhere else it is never on the path, and when it is imported
anyway it does nothing unless BOTH hold: the interpreter is native Windows
(`os.name == "nt"`) and its parent chain is Git Bash or MSYS2 (`MSYSTEM`).

WHY IT EXISTS. bash and Python disagree about the machine they share. Measured
on windows-latest (Git Bash 5.3, Python 3.12), bash says `/tmp/x` and `/c/x`;
msys rewrites those as `C:/…/AppData/Local/Temp/x` and `C:/x` in
every argument AND every environment value it hands a native program, but not
inside a heredoc or a `-c` string; and Python answers `C:\\...` with
backslashes, `\\r\\n` on stdout and in every text file it writes. So a path
bash wrote came back as a different string, and every comparison between the
two was false.

THE ONE CONVERSION. Inside Python, a path is spelled the way bash spells it.
What msys converted on the way in is converted back at startup (argv and
environment), a path is turned into its Windows spelling only where Python
hands it to Windows (open, stat, listdir, a child process that is not an msys
program), and every path Python computes (join, realpath, getcwd) comes back
in bash's spelling. The mount table is msys's own, exported by msys-compat.sh
as HW_MSYS_ROOT / HW_MSYS_TMP / HW_MSYS_TMP_LONG (from `cygpath -m`).

Also here, because it is the same boundary: stdout, stderr and text files
written are LF; a bare program name is found on PATH (CreateProcess searches
System32 first, where `bash` is WSL's launcher); a `#!` script is run through
msys bash, which honours its shebang (Windows cannot: WinError 193).
"""
import os
import sys


def _active():
    return os.name == "nt" and bool(os.environ.get("MSYSTEM"))


import re

_DRIVE = re.compile(r"^([A-Za-z]):(?:[/\\]|$)")
_MSYS_DRIVE = re.compile(r"^/([A-Za-z])(?=/|$)")
_ROOT = ""
_TMPS = []
_TMP_SHORT = ""


def _norm_win(p):
    return p.replace("\\", "/")


def load_table(env):
    """msys's mount table, as msys-compat.sh exported it (`cygpath -m`)."""
    global _ROOT, _TMPS, _TMP_SHORT
    _ROOT = _norm_win(env.get("HW_MSYS_ROOT", "")).rstrip("/")
    _TMP_SHORT = _norm_win(env.get("HW_MSYS_TMP", "")).rstrip("/")
    _TMPS = [t.rstrip("/") for t in (_norm_win(env.get("HW_MSYS_TMP", "")),
                                     _norm_win(env.get("HW_MSYS_TMP_LONG", "")),
                                     _norm_win(env.get("TEMP", ""))) if t]


def to_native(p):
    """bash's spelling -> one Windows can open (forward slashes)."""
    if not isinstance(p, str) or not p or p[0] not in "/\\" or p[:2] in ("//", "\\\\"):
        return p
    q = _norm_win(p)
    if q == "/dev/null":
        return "nul"
    m = _MSYS_DRIVE.match(q)
    if m:
        return m.group(1).upper() + ":" + (q[2:] or "/")
    if _TMPS and (q == "/tmp" or q.startswith("/tmp/")):
        return _TMPS[0] + q[4:]
    return _ROOT + q if _ROOT else p


def to_msys(p, tmp=True):
    """A Windows spelling -> bash's (the inverse of to_native). tmp=False gives
    the PHYSICAL spelling, the one `pwd -P` prints (/c/Users/<u>/AppData/...),
    not the /tmp mount: what realpath answers, as /private/var on macOS.
    tmp="argv" is the exact inverse of msys's own conversion: it spells /tmp/x
    with the short 8.3 name and /c/Users/<u>/.../Temp/x with the long one, so
    only the short spelling is /tmp (both, where the two names are one)."""
    if p == "nul":
        return "/dev/null"
    if not isinstance(p, str) or not _DRIVE.match(p):
        return p
    q = _norm_win(p)
    low = q.lower()
    if tmp == "argv":
        tmps = [_TMP_SHORT] if _TMP_SHORT else _TMPS
    else:
        tmps = _TMPS if tmp else ()
    for t in tmps:
        tl = t.lower()
        if low == tl or low.startswith(tl + "/"):
            return "/tmp" + q[len(t):]
    if _ROOT:
        rl = _ROOT.lower()
        if low == rl or low.startswith(rl + "/"):
            return q[len(_ROOT):] or "/"
    return "/" + q[0].lower() + (q[2:] if len(q) > 3 else "")


# msys writes an upper-case drive; a lower-case one was never its conversion.
_OPT = re.compile(r"^(--?[\w.-]+=)([A-Z]:/.*)$")


def back(a):
    """One argument or value msys converted on its way in, converted back."""
    if isinstance(a, str):
        if re.match(r"^[A-Z]:/", a):
            return to_msys(a, tmp="argv")
        m = _OPT.match(a)
        if m:
            return m.group(1) + to_msys(m.group(2), tmp="argv")
    return a


# ONCE PER PROCESS. A fixture that carries its own copy of bin/ puts a second
# sitecustomize.py on PYTHONPATH; applying the layer twice nests every wrapper,
# and the two copies chaining to each other nest them without end (84 on
# windows-latest). The first copy loaded is the layer; any other is skipped.
_FIRST = not getattr(sys, "_hw_msys_layer", False)
sys._hw_msys_layer = True

if _active() and _FIRST:
    import builtins
    import io
    import ntpath
    import shutil
    import subprocess

    load_table(os.environ)
    if not _ROOT:
        _b = _norm_win(shutil.which("bash") or "")
        if _b.lower().endswith("/usr/bin/bash.exe"):
            _ROOT = _b[: -len("/usr/bin/bash.exe")]

    def _fs(p):
        if isinstance(p, os.PathLike):
            p = os.fspath(p)
        return to_native(p) if isinstance(p, str) else p

    # ── what msys converted on the way in, converted back ──────────────────
    sys.argv[1:] = [back(a) for a in sys.argv[1:]]
    for _k, _v in list(os.environ.items()):
        # msys writes a converted value with forward slashes; Windows' own
        # values use backslashes and are left alone. HOME, PWD and OLDPWD are
        # the exception: msys converts them to backslashes, and they are bash's.
        if (re.match(r"^[A-Z]:/", _v) and ";" not in _v) or (_k in ("HOME", "PWD", "OLDPWD") and _DRIVE.match(_v)):
            os.environ[_k] = to_msys(_v, tmp="argv")

    # ── LF out ──────────────────────────────────────────────────────────────
    for _s in (sys.stdout, sys.stderr):
        try:
            _s.reconfigure(newline="\n")
        except (AttributeError, ValueError, io.UnsupportedOperation):
            pass

    _open = io.open

    def _open_lf(file, mode="r", buffering=-1, encoding=None, errors=None, newline=None, closefd=True, opener=None):
        if isinstance(file, (str, os.PathLike)):
            file = _fs(file)
        if newline is None and "b" not in mode and any(c in mode for c in "wax+"):
            newline = "\n"
        return _open(file, mode, buffering, encoding, errors, newline, closefd, opener)

    builtins.open = _open_lf
    io.open = _open_lf

    # ── paths handed to Windows ─────────────────────────────────────────────
    def _wrap1(fn):
        def w(path, *a, **k):
            return fn(_fs(path), *a, **k)
        w.__name__ = getattr(fn, "__name__", "w")
        w.__wrapped__ = fn
        return w

    def _wrap2(fn):
        def w(src, dst, *a, **k):
            return fn(_fs(src), _fs(dst), *a, **k)
        w.__name__ = getattr(fn, "__name__", "w")
        w.__wrapped__ = fn
        return w

    for _n in ("open", "stat", "lstat", "mkdir", "makedirs", "rmdir", "remove", "unlink",
               "chdir", "chmod", "utime", "access", "truncate", "removedirs"):
        if hasattr(os, _n):
            setattr(os, _n, _wrap1(getattr(os, _n)))
    for _n in ("rename", "replace", "link"):
        setattr(os, _n, _wrap2(getattr(os, _n)))

    _listdir, _scandir, _readlink, _symlink = os.listdir, os.scandir, os.readlink, os.symlink

    def _listdir_w(path="."):
        return _listdir(_fs(path))

    def _scandir_w(path="."):
        return _scandir(_fs(path))

    def _readlink_w(path, *a, **k):
        return to_msys(_readlink(_fs(path), *a, **k))

    def _symlink_w(src, dst, *a, **k):
        return _symlink(_fs(src) if isinstance(src, str) and src.startswith("/") else src, _fs(dst), *a, **k)

    os.listdir, os.scandir, os.readlink, os.symlink = _listdir_w, _scandir_w, _readlink_w, _symlink_w

    _getcwd = os.getcwd
    os.getcwd = lambda: to_msys(_getcwd())

    # ── paths Python computes, answered in bash's spelling ──────────────────
    # A module of its own, not ntpath patched in place: ntpath's functions call
    # each other through its globals, and they must keep Windows' spelling.
    import posixpath
    import types
    _nt = ntpath
    P = types.ModuleType("os.path", _nt.__doc__)
    P.__dict__.update({k: v for k, v in _nt.__dict__.items() if not k.startswith("__")})

    def _fwd(p):
        return p.replace("\\", "/") if isinstance(p, str) else p

    def _rooted(p):
        return isinstance(p, str) and p[:1] in "/\\" and p[:2] not in ("//", "\\\\")

    def join(a, *p):
        return _fwd(_nt.join(a, *p))

    def normpath(p):
        p = os.fspath(p)
        return posixpath.normpath(_fwd(p)) if _rooted(p) else _fwd(_nt.normpath(p))

    def isabs(p):
        p = os.fspath(p)
        return _rooted(p) or (isinstance(p, str) and bool(_DRIVE.match(p))) or _nt.isabs(p)

    def abspath(p):
        p = os.fspath(p)
        if not isinstance(p, str):
            return _nt.abspath(p)
        if not isabs(p):
            p = join(os.getcwd(), p)
        return normpath(to_msys(p))

    def realpath(p, *a, **k):
        p = os.fspath(p)
        if not isinstance(p, str):
            return _nt.realpath(p, *a, **k)
        return to_msys(_fwd(_nt.realpath(to_native(abspath(p)), *a, **k)), tmp=False)

    def relpath(path, start=None):
        path = abspath(path)
        start = abspath(start if start is not None else os.curdir)
        if path[:1] == "/" and start[:1] == "/":
            return posixpath.relpath(path, start)
        return _fwd(_nt.relpath(path, start))

    def expanduser(p):
        p = os.fspath(p)
        home = os.environ.get("HOME")
        if isinstance(p, str) and home and (p == "~" or p.startswith(("~/", "~\\"))):
            return home.rstrip("/") + _fwd(p[1:])
        return _nt.expanduser(p)

    def dirname(p):
        return _fwd(_nt.dirname(p))

    def split(p):
        h, t = _nt.split(p)
        return _fwd(h), t

    for _n, _f in (("join", join), ("normpath", normpath), ("isabs", isabs), ("abspath", abspath),
                   ("realpath", realpath), ("relpath", relpath), ("expanduser", expanduser),
                   ("dirname", dirname), ("split", split)):
        setattr(P, _n, _f)
    for _n in ("exists", "lexists", "isfile", "isdir", "islink", "ismount", "getsize",
               "getmtime", "getatime", "getctime", "isjunction"):
        if hasattr(_nt, _n):
            setattr(P, _n, _wrap1(getattr(_nt, _n)))
    P.samefile = _wrap2(_nt.samefile)
    # The separator every path above is spelled with: code that composes or
    # tests a path with os.sep (`endswith(os.sep + "decisions")`) must agree.
    # Windows accepts either, so a path built this way still opens.
    P.sep, P.altsep = "/", "\\"
    os.sep, os.altsep = "/", "\\"
    os.path = P
    sys.modules["os.path"] = P

    # pathlib keeps the real ntpath as WindowsPath's flavour, so `Path(x)`
    # would resolve `/tmp/x` against the current drive (`D:\tmp\x`). Under Git
    # Bash `Path()` builds a posix-flavoured path instead: its string is bash's
    # spelling, and its filesystem calls go through the os functions above.
    import pathlib

    class _MsysPath(pathlib.Path, pathlib.PurePosixPath):
        __slots__ = ()

        # What it is built from may be Windows' spelling (`__file__` is
        # D:\\a\\x.py), which a posix path would read as one relative name.
        # 3.12+ parses in __init__, older ones in __new__.
        if pathlib.PurePath.__init__ is not object.__init__:
            def __init__(self, *args, **kwargs):
                super().__init__(*(to_msys(a) if isinstance(a, str) else a for a in args), **kwargs)
        else:
            def __new__(cls, *args, **kwargs):
                return super().__new__(cls, *(to_msys(a) if isinstance(a, str) else a for a in args), **kwargs)

        # A posix flavour resolves with posixpath.realpath, which keeps the
        # /tmp mount; os.path.realpath above answers what `pwd -P` does.
        def resolve(self, strict=False):
            if strict:
                os.stat(self)
            return type(self)(os.path.realpath(os.fspath(self)))

    # What Path() instantiates when os.name == "nt"; 3.13+ looks it up in pathlib._local.
    for _m in (pathlib, sys.modules.get("pathlib._local")):
        if _m is not None:
            _m.WindowsPath = _MsysPath

    # shutil.copyfile/copy2 end in _winapi.CopyFile2, which takes the path as given.
    import _winapi
    if hasattr(_winapi, "CopyFile2"):
        _copyfile2 = _winapi.CopyFile2

        def _copyfile2_fs(src, dst, flags, *a):
            return _copyfile2(_fs(src), _fs(dst), flags, *a)

        _winapi.CopyFile2 = _copyfile2_fs

    # runpy, py_compile and the import system read source through open_code.
    import _io
    _open_code = _io.open_code

    def _open_code_fs(path):
        return _open_code(_fs(path))

    _io.open_code = _open_code_fs
    io.open_code = _open_code_fs

    import tempfile
    try:
        tempfile.tempdir = to_msys(_fwd(tempfile.gettempdir()))
    except Exception:  # noqa: BLE001 — a temp dir is looked up again on use
        pass

    # ── imports from a path in bash's spelling ──────────────────────────────
    import importlib.machinery as _mach

    _hook = _mach.FileFinder.path_hook(
        (_mach.ExtensionFileLoader, _mach.EXTENSION_SUFFIXES),
        (_mach.SourceFileLoader, _mach.SOURCE_SUFFIXES),
        (_mach.SourcelessFileLoader, _mach.BYTECODE_SUFFIXES))

    def _msys_path_hook(entry):
        if isinstance(entry, str) and entry[:1] == "/" and entry[:2] != "//":
            return _hook(to_native(entry))
        raise ImportError("not an msys path")

    sys.path_hooks.insert(0, _msys_path_hook)
    sys.path_importer_cache.clear()

    # A loader built on a path (py_compile, SourceFileLoader(name, path))
    # stats and reads it through the nt module, which takes it as given.
    import importlib._bootstrap_external as _bx
    _fl_init = _bx.FileLoader.__init__

    def _fl_init_fs(self, fullname, path):
        _fl_init(self, fullname, _fs(path))

    _bx.FileLoader.__init__ = _fl_init_fs

    # py_compile hands the path to the loader's methods again, as given.
    for _cls, _meth in ((_bx.FileLoader, "get_data"), (_bx.SourceFileLoader, "path_stats")):
        def _mk(orig):
            def w(self, path):
                return orig(self, _fs(path))
            return w
        setattr(_cls, _meth, _mk(getattr(_cls, _meth)))

    # ...and writes the .pyc through _write_atomic, which opens with nt.open.
    _wa = _bx._write_atomic
    _bx._write_atomic = lambda path, data, mode=0o666: _wa(_fs(path), data, mode)

    import importlib.util as _iu
    _sffl = _iu.spec_from_file_location

    def _spec_from_file_location(name, location=None, *a, **k):
        return _sffl(name, _fs(location) if location is not None else None, *a, **k)

    _iu.spec_from_file_location = _spec_from_file_location

    # ── child processes ─────────────────────────────────────────────────────
    _PATHEXT = tuple(e.lower() for e in os.environ.get("PATHEXT", ".COM;.EXE;.BAT;.CMD").split(";") if e)

    def _is_msys_program(native):
        low = _norm_win(native).lower()
        return bool(_ROOT) and low.startswith(_ROOT.lower() + "/usr/bin/")

    def _to_native_arg(a):
        if isinstance(a, str):
            if a[:1] == "/" and a[:2] != "//":
                return to_native(a)
            m = re.match(r"^(--?[\w.-]+=)(/[^/].*)$", a)
            if m:
                return m.group(1) + to_native(m.group(2))
        return a

    _Popen_init = subprocess.Popen.__init__

    def _popen_init(self, args, *a, **k):
        if k.get("cwd") is not None:
            k["cwd"] = _fs(k["cwd"])
        if not k.get("shell") and isinstance(args, (list, tuple)) and args:
            args = [os.fspath(x) if isinstance(x, os.PathLike) else x for x in args]
            prog = k.get("executable") or args[0]
            found = None
            if isinstance(prog, str):
                if "/" in prog or "\\" in prog:
                    found = to_native(prog)
                else:
                    found = shutil.which(prog)
            if found:
                is_exe = _norm_win(found).lower().endswith(_PATHEXT)
                script = False
                if not is_exe:
                    try:
                        with _open(found, "rb") as fh:
                            script = fh.read(2) == b"#!"
                    except OSError:
                        script = False
                if script:
                    # Windows cannot run a #! file; msys bash's exec honours it.
                    bash = shutil.which("bash") if not _ROOT else _ROOT + "/usr/bin/bash.exe"
                    args = [bash, "-c", 'exec "$0" "$@"', to_msys(_norm_win(found))] + list(args[1:])
                    k.pop("executable", None)
                elif is_exe and not _is_msys_program(found):
                    args = [found] + [_to_native_arg(x) for x in args[1:]]
                    env = k.get("env")
                    src = os.environ if env is None else env
                    k["env"] = {kk: (_to_native_arg(v) if isinstance(v, str) and ":" not in v else v)
                                for kk, v in src.items()}
                else:
                    args = [found] + list(args[1:])
        _Popen_init(self, args, *a, **k)
        for s in (self.stdin,):
            if isinstance(s, io.TextIOWrapper):
                try:
                    s.reconfigure(newline="\n")
                except (ValueError, io.UnsupportedOperation):
                    pass

    subprocess.Popen.__init__ = _popen_init

# ── a sitecustomize further down the path still runs ────────────────────────
def _chain():
    import importlib.machinery
    import importlib.util
    here = os.path.dirname(os.path.abspath(__file__))
    for p in sys.path:
        if not p or os.path.abspath(p) == here:
            continue
        spec = importlib.machinery.PathFinder.find_spec("sitecustomize", [p])
        if not (spec and spec.loader and spec.origin):
            continue
        try:
            with open(spec.origin, "rb") as fh:
                if b"_hw_msys_layer" in fh.read():
                    continue  # another copy of this file, not someone else's
        except OSError:
            continue
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return


try:
    if _FIRST:
        _chain()
except Exception:  # noqa: BLE001 — another package's sitecustomize is not ours to break on
    pass
