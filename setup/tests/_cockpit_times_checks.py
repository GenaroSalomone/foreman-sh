"""The checks of 920 (how long an executor has worked, and whether it still moves), one function each:

    python3 -I _cockpit_times_checks.py <bin-dir> <check> [<check> …]

Prints `ok <check>` or `FAIL <check>: <why>` per check and exits 1 on any failure. The bin dir is a
parameter so 920 can run the same check against a mutated copy of the writer and expect it to fail.
Run directories, worktrees (real `git` repositories) and transcripts are built on disk in a temp dir
by _cockpit_world.py; nothing here touches herdr or the real work root.
"""
import importlib.machinery
import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import _cockpit_world as cw  # noqa: E402

BIN = None
TMPS = []
H = 3600
SLOP = 5000  # ms: the writer reads the clock after the test did


def tmpdir():
    d = tempfile.mkdtemp(prefix="cockpit-times-")
    TMPS.append(d)
    return d


def state(w, **env):
    r = cw.run_writer(w, BIN, "--stdout", **env)
    assert r.returncode == 0, "cockpit-state --stdout exited %d: %s" % (r.returncode, r.stderr)
    s = json.loads(r.stdout)
    errs = cw.validate_state(s)
    assert not errs, "does not validate: " + "; ".join(errs[:3])
    return s


def row(s, task):
    for r in s["rows"]:
        if r["task"] == task:
            return r
    raise AssertionError("no row for %s in %s" % (task, [r["task"] for r in s["rows"]]))


def ms(t):
    return int(t * 1000)


def near(got, want_s, what):
    assert got is not None and abs(got - ms(want_s)) < SLOP, "%s: %s, wanted about %s (%.0f s apart)" % (
        what, got, ms(want_s), (got - ms(want_s)) / 1000 if got is not None else float("nan"))


def git(wd, *args, when=None):
    """A real git command in `wd`. `when` (epoch s) is the author and committer date, which is the time the reflog keeps."""
    env = dict(os.environ, GIT_CONFIG_GLOBAL="/dev/null", GIT_CONFIG_SYSTEM="/dev/null", GIT_CONFIG_NOSYSTEM="1",
               GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.test", GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.test")
    if when is not None:
        env["GIT_AUTHOR_DATE"] = env["GIT_COMMITTER_DATE"] = "%d +0000" % when
    r = subprocess.run(["git", "-C", wd] + list(args), env=env, capture_output=True, text=True)
    assert r.returncode == 0, "git %s: %s" % (" ".join(args), r.stderr)
    return r.stdout


def worktree(x, *, file_age, commit_age, extra_commit_age=None):
    """The executor's worktree as a real repository: one tracked file `a.txt` whose mtime is
    `file_age` seconds old, committed `commit_age` seconds ago; optionally a second commit (an
    empty one, so no file moves) `extra_commit_age` seconds ago."""
    wd = os.path.dirname(os.path.dirname(x["rundir"]))
    now = time.time()
    git(wd, "init", "-q", "-b", "task/x")
    f = os.path.join(wd, "a.txt")
    open(f, "w").write("x\n")
    os.utime(f, (now - file_age, now - file_age))
    git(wd, "add", "a.txt")
    git(wd, "commit", "-q", "-m", "first", when=now - commit_age)
    if extra_commit_age is not None:
        git(wd, "commit", "-q", "--allow-empty", "-m", "second", when=now - extra_commit_age)
    return wd


def claude_session(w, x, sid, age):
    """The run's receipt session_id, CLAUDE_CONFIG_DIR in its env, and a transcript `age` seconds old."""
    cfg = os.path.join(w.tmp, "claude-config")
    with open(os.path.join(x["rundir"], "receipt.jsonl"), "a") as fh:
        fh.write(json.dumps({"key": "session_id", "value": sid}) + "\n")
    with open(os.path.join(x["rundir"], "env"), "a") as fh:
        fh.write("CLAUDE_CONFIG_DIR='%s'\n" % cfg)
    launch = os.path.dirname(os.path.dirname(x["rundir"]))
    pdir = os.path.join(cfg, "projects", re.sub(r"[^A-Za-z0-9]", "-", launch))
    os.makedirs(pdir, exist_ok=True)
    path = os.path.join(pdir, sid + ".jsonl")
    open(path, "w").write(json.dumps({"type": "assistant", "message": {"model": "claude-sonnet-5-5", "usage": {"input_tokens": 10}}}) + "\n")
    t = time.time() - age
    os.utime(path, (t, t))
    return path


def load_writer():
    ldr = importlib.machinery.SourceFileLoader("cs_times", os.path.join(BIN, "cockpit-state"))
    mod = importlib.util.module_from_spec(importlib.util.spec_from_loader(ldr.name, ldr))
    ldr.exec_module(mod)
    return mod


# ── what the card is told ────────────────────────────────────────────────────

def times_published():
    """A working row carries when its run began, when its turn began and when it last moved."""
    w = cw.World(tmpdir())
    now = time.time()
    turn_end = {"turn_state": "ended_unreported", "turn_ended_at": cw.iso(now - 2460)}
    busy = w.executor(task="busy", status="working", tokens=turn_end, dispatched_at=now - (3 * H + 12 * 60))
    worktree(busy, file_age=2 * H, commit_age=2 * H)
    first = w.executor(task="first", status="working", dispatched_at=now - 900)           # no turn has ended
    worktree(first, file_age=2 * H, commit_age=2 * H)
    idle = w.executor(task="idle", status="idle", tokens=turn_end, dispatched_at=now - H)
    worktree(idle, file_age=60, commit_age=60)
    w.publish()
    s = state(w)
    b = row(s, "busy")
    near(b["dispatched_at"], now - (3 * H + 12 * 60), "busy.dispatched_at (the first line of the receipt)")
    near(b["turn_started_at"], now - 2460, "busy.turn_started_at (the previous turn's end)")
    f = row(s, "first")
    assert f["turn_started_at"] == f["dispatched_at"], "a first turn began with the run: %s vs %s" % (f["turn_started_at"], f["dispatched_at"])
    i = row(s, "idle")
    near(i["dispatched_at"], now - H, "idle.dispatched_at")
    assert i["turn_started_at"] is None and i["last_progress_at"] is None, \
        "a row that is not working has no turn and no progress to show: %s" % ((i["turn_started_at"], i["last_progress_at"]),)


def dispatched_fallbacks():
    """No `at` on the receipt: the dispatch file's mtime, then the env file's."""
    w = cw.World(tmpdir())
    now = time.time()
    a = w.executor(task="withdispatch", status="working", effort="high")
    os.utime(os.path.join(a["rundir"], "dispatch"), (now - 7200, now - 7200))
    b = w.executor(task="envonly", status="working")
    os.utime(os.path.join(b["rundir"], "env"), (now - 5400, now - 5400))
    w.publish()
    s = state(w)
    near(row(s, "withdispatch")["dispatched_at"], now - 7200, "dispatched_at from the dispatch file")
    near(row(s, "envonly")["dispatched_at"], now - 5400, "dispatched_at from the env file")


def last_progress():
    """The newest of the last commit, the newest file in the worktree and the transcript, never .git or .hw."""
    w = cw.World(tmpdir())
    now = time.time()
    kw = dict(status="working", dispatched_at=now - 6 * H, tokens={"turn_state": "ended_unreported", "turn_ended_at": cw.iso(now - 3 * H)})
    c = w.executor(task="commitonly", **kw)
    worktree(c, file_age=2 * H, commit_age=2 * H, extra_commit_age=300)          # files old, a commit 5 min ago
    f = w.executor(task="fileonly", **kw)
    worktree(f, file_age=200, commit_age=2 * H)                                    # a file edited 200 s ago, no new commit
    t = w.executor(task="transcript", **kw)
    worktree(t, file_age=2 * H, commit_age=2 * H)
    claude_session(w, t, "22222222-aaaa-bbbb-cccc-000000000001", 60)
    q = w.executor(task="quiet", **kw)                                             # nothing moved for two hours...
    wd = worktree(q, file_age=2 * H, commit_age=2 * H)
    os.makedirs(os.path.join(wd, "node_modules", "x"))
    open(os.path.join(wd, "node_modules", "x", "i.js"), "w").write("x")            # ...but caches and tool state are being written now
    open(os.path.join(q["rundir"], "scratch"), "w").write("x")
    n = w.executor(task="nothing", **kw)                                           # no repository, no file: dispatch is all there is
    w.publish()
    s = state(w)
    near(row(s, "commitonly")["last_progress_at"], now - 300, "a commit on the branch is progress though no file moved")
    near(row(s, "fileonly")["last_progress_at"], now - 200, "the newest file in the worktree is progress")
    near(row(s, "transcript")["last_progress_at"], now - 60, "the transcript's last write is progress")
    near(row(s, "quiet")["last_progress_at"], now - 2 * H, ".git, .hw and node_modules are not progress")
    nr = row(s, "nothing")
    assert nr["last_progress_at"] == nr["dispatched_at"], "with no sign of work, progress is the dispatch: %s" % nr["last_progress_at"]


def threshold_is_one_constant():
    """rules.no_progress_after_ms is the 30 min the brief asks for; the test environment can lower it."""
    w = cw.World(tmpdir())
    w.executor(task="a", status="working")
    w.publish()
    assert state(w)["rules"]["no_progress_after_ms"] == 30 * 60 * 1000, state(w)["rules"]
    got = state(w, HW_COCKPIT_NO_PROGRESS_S="90")["rules"]["no_progress_after_ms"]
    assert got == 90000, "the threshold is not the writer's single constant: %s" % got


# ── what it costs ────────────────────────────────────────────────────────────

def progress_cache():
    """A worktree is walked once per TTL, not once per beat; the cache is by the time it was walked."""
    mod = load_writer()
    d = tmpdir()
    for i in range(40):
        os.makedirs(os.path.join(d, "d%d" % i))
        open(os.path.join(d, "d%d" % i, "f.txt"), "w").write("x")
    old = time.time() - 7200
    for root, _, files in os.walk(d):
        for f in files:
            os.utime(os.path.join(root, f), (old, old))
    cache = {}
    t0 = ms(time.time())
    first = mod.newest_mtime_ms(d, cache, t0)
    near(first, old, "the first walk")
    fresh = os.path.join(d, "d3", "new.txt")
    open(fresh, "w").write("x")
    again = mod.newest_mtime_ms(d, cache, t0 + 1000)
    assert again == first, "a worktree walked a second ago was walked again (cache ignored): %s != %s" % (again, first)
    later = mod.newest_mtime_ms(d, cache, t0 + 10 * 60 * 1000)
    near(later, time.time(), "a walk past the TTL")
    assert later > first, "a walk past the TTL did not see the new file"


def progress_cache_is_shared_by_writers():
    """`--kick` and the heartbeat are different processes: the cache is a file beside the state file, and `--stdout` writes none."""
    w = cw.World(tmpdir())
    now = time.time()
    x = w.executor(task="busy", status="working", dispatched_at=now - 6 * H)
    wd = worktree(x, file_age=2 * H, commit_age=2 * H)
    w.publish()
    r = cw.run_writer(w, BIN, "--stdout")
    assert r.returncode == 0, r.stderr
    assert not os.path.exists(os.path.join(w.work, ".cockpit")), "--stdout created something (the cache file?)"
    r = cw.run_writer(w, BIN, "--once")
    assert r.returncode == 0, r.stderr
    s1 = json.load(open(w.state_path()))
    near(row(s1, "busy")["last_progress_at"], now - 2 * H, "first write")
    open(os.path.join(wd, "b.txt"), "w").write("new")                       # progress right after the first walk
    r = cw.run_writer(w, BIN, "--once")                                     # another process, inside the TTL
    s2 = json.load(open(w.state_path()))
    near(row(s2, "busy")["last_progress_at"], now - 2 * H, "second write inside the TTL")
    r = cw.run_writer(w, BIN, "--once", HW_COCKPIT_PROGRESS_TTL_S="0")
    s3 = json.load(open(w.state_path()))
    near(row(s3, "busy")["last_progress_at"], time.time(), "write with the TTL at 0")


def walk_cost():
    """What a walk costs, measured: a 3000-file tree. Printed; the budget is generous (a walk happens once per TTL per worktree)."""
    mod = load_writer()
    d = tmpdir()
    for i in range(60):
        p = os.path.join(d, "pkg%02d" % i)
        os.makedirs(p)
        for j in range(50):
            open(os.path.join(p, "f%02d.txt" % j), "w").write("x")
    best = 1e9
    for _ in range(3):
        t0 = time.perf_counter()
        v = mod.newest_mtime_ms(d, {}, ms(time.time()))
        best = min(best, (time.perf_counter() - t0) * 1000)
    assert v is not None
    sys.stderr.write("  walk cost: 3000 files in %d dirs, best of 3 %.0f ms (cached beats cost one dict lookup)\n" % (60, best))
    assert best < 1500, "a 3000-file walk took %.0f ms" % best


CHECKS = {f.__name__: f for f in (times_published, dispatched_fallbacks, last_progress, threshold_is_one_constant, progress_cache,
                                  progress_cache_is_shared_by_writers, walk_cost)}

if __name__ == "__main__":
    BIN = os.path.abspath(sys.argv[1])
    names = sys.argv[2:] or list(CHECKS)
    failed = 0
    for n in names:
        try:
            CHECKS[n]()
            print("ok %s" % n)
        except AssertionError as e:
            failed += 1
            print("FAIL %s: %s" % (n, e))
        except Exception as e:  # a crash is a failure with its type named, never a pass
            failed += 1
            print("FAIL %s: %s: %s" % (n, type(e).__name__, e))
    for d in TMPS:
        shutil.rmtree(d, ignore_errors=True)
    sys.exit(1 if failed else 0)
