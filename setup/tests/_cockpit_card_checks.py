"""The checks of 970 (what the task is and how far it has come, as the writer publishes it), one function each:

    python3 -I _cockpit_card_checks.py <bin-dir> <check> [<check> …]

Prints `ok <check>` or `FAIL <check>: <why>` per check and exits 1 on any failure. The bin dir is a
parameter so 970 can run the same check against a mutated copy of the writer and expect it to fail.
Briefs, worktrees (real `git` repositories), suite-gate slots held by a real live process and a real
`setup/test-hw` process are built in a temp dir; nothing here touches herdr or the real work root, and
every process started here is a `sleep` (or a script that sleeps) in its own process group, killed by
that group's id at the end.
"""
import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import _cockpit_world as cw  # noqa: E402
from _cockpit_times_checks import git, row  # noqa: E402

BIN = None
TMPS = []
PROCS = []
SLOP = 5000


def tmpdir():
    d = tempfile.mkdtemp(prefix="cockpit-card-")
    TMPS.append(d)
    return d


def state(w, **env):
    r = cw.run_writer(w, BIN, "--stdout", **env)
    assert r.returncode == 0, "cockpit-state --stdout exited %d: %s" % (r.returncode, r.stderr)
    s = json.loads(r.stdout)
    errs = cw.validate_state(s)
    assert not errs, "does not validate: " + "; ".join(errs[:3])
    return s


def dispatch(x, **fields):
    """Lines of the run's dispatch manifest in hw's shape: `  <name>  <value>  (mark)`."""
    with open(os.path.join(x["rundir"], "dispatch"), "a") as fh:
        for k, v in fields.items():
            fh.write("  %-11s %s  (chosen)\n" % (k, v))


def receipt(x, key, value):
    with open(os.path.join(x["rundir"], "receipt.jsonl"), "a") as fh:
        fh.write(json.dumps({"key": key, "value": value, "at": cw.iso(time.time())}) + "\n")


def repo(x):
    """The executor's worktree as a real repository with one base commit on main, on a task branch; returns
    (worktree, base sha)."""
    wd = os.path.dirname(os.path.dirname(x["rundir"]))
    git(wd, "init", "-q", "-b", "main")
    open(os.path.join(wd, ".gitignore"), "w").write(".hw/\n")
    git(wd, "add", ".gitignore")
    git(wd, "commit", "-q", "-m", "base", when=time.time() - 7200)
    base = git(wd, "rev-parse", "HEAD").strip()
    git(wd, "checkout", "-q", "-b", "task/x")
    return wd, base


def sleeper(cwd=None, argv=None):
    p = subprocess.Popen(argv or ["sleep", "120"], cwd=cwd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                         start_new_session=True)  # its own group: the `sleep` a script starts goes with it
    PROCS.append(p)
    return p


def lstart(pid):
    return subprocess.run(["ps", "-o", "lstart=", "-p", str(pid)], capture_output=True, text=True).stdout.strip()


def slot(w, n, pid, task, cmd, at):
    d = os.path.join(w.work, ".locks", "suite-gate", "slot.%d" % n)
    os.makedirs(d, exist_ok=True)
    for k, v in (("pid", pid), ("start", lstart(pid)), ("task", task), ("cmd", cmd), ("at", at)):
        open(os.path.join(d, "owner." + k), "w").write("%s\n" % v)
    return d


# ── the title ────────────────────────────────────────────────────────────────

BRIEF = """---
requested_by: "someone"
# a YAML comment is not the title
asked: build
---

# Go T4: verbos escritores

## Qué pasa
# not this one either
"""


def title_after_frontmatter():
    """The title is the brief's first `# ` line after its frontmatter; no frontmatter: the first `# `; no brief: null."""
    w = cw.World(tmpdir())
    d = tmpdir()
    a = w.executor(task="fm")
    open(os.path.join(d, "fm.md"), "w").write(BRIEF)
    dispatch(a, brief=os.path.join(d, "fm.md"))
    b = w.executor(task="plain")
    open(os.path.join(d, "plain.md"), "w").write("intro\n# Plain title  \n# second\n")
    dispatch(b, brief=os.path.join(d, "plain.md"))
    w.executor(task="nobrief")
    e = w.executor(task="gone")
    dispatch(e, brief=os.path.join(d, "missing.md"))
    w.publish()
    s = state(w)
    assert row(s, "fm")["title"] == "Go T4: verbos escritores", "the title read past the frontmatter: %r" % row(s, "fm")["title"]
    assert row(s, "plain")["title"] == "Plain title", "a brief without frontmatter: %r" % row(s, "plain")["title"]
    assert row(s, "nobrief")["title"] is None and row(s, "gone")["title"] is None, \
        "no brief, no title: %r %r" % (row(s, "nobrief")["title"], row(s, "gone")["title"])


# ── the commits ──────────────────────────────────────────────────────────────

def commits_over_base():
    """The branch's commits over the base hw recorded at dispatch, and the newest one's time; 0 and null on a
    fresh branch; null where there is no repository."""
    w = cw.World(tmpdir())
    now = time.time()
    a = w.executor(task="two")
    wd, base = repo(a)
    receipt(a, "review_base", base)
    git(wd, "commit", "-q", "--allow-empty", "-m", "one", when=now - 3600)
    git(wd, "commit", "-q", "--allow-empty", "-m", "two", when=now - 37 * 60)
    b = w.executor(task="fresh")
    _, base_b = repo(b)
    dispatch(b, base="main")  # no review_base: the manifest's base name
    w.executor(task="norepo")
    w.publish()
    s = state(w)
    c = row(s, "two")["commits"]
    assert c is not None and c["count"] == 2, "two commits on the branch over its base, got %r" % (c,)
    assert abs(c["last_at"] - int((now - 37 * 60) * 1000)) < SLOP, "the last commit was 37 min ago, got %r" % (c,)
    f = row(s, "fresh")["commits"]
    assert f == {"count": 0, "last_at": None}, "a branch with nothing over its base: %r" % (f,)
    assert row(s, "norepo")["commits"] is None, "no repository, no count: %r" % (row(s, "norepo")["commits"],)


def commits_cache():
    """A commit made inside the TTL is counted on the next write: the cache is keyed by the HEAD reflog."""
    w = cw.World(tmpdir())
    a = w.executor(task="moving")
    wd, base = repo(a)
    receipt(a, "review_base", base)
    git(wd, "commit", "-q", "--allow-empty", "-m", "one")
    w.publish()
    env = {"HW_COCKPIT_COMMITS_TTL_S": "3600"}
    first = cw.run_writer(w, BIN, "--once", **env)
    assert first.returncode == 0, first.stderr
    s1 = json.load(open(w.state_path()))
    assert row(s1, "moving")["commits"]["count"] == 1, "one commit: %r" % (row(s1, "moving")["commits"],)
    git(wd, "commit", "-q", "--allow-empty", "-m", "two")
    assert cw.run_writer(w, BIN, "--once", **env).returncode == 0
    s2 = json.load(open(w.state_path()))
    assert row(s2, "moving")["commits"]["count"] == 2, \
        "a new commit is counted within the TTL: %r" % (row(s2, "moving")["commits"],)


# ── the gate ─────────────────────────────────────────────────────────────────

def gate_from_slot():
    """A live suite-gate slot owned by the task is its gate, since the slot's owner.at; a slot of another task,
    or of a dead holder, is not; `hw done verification of <task>` is the task's too."""
    w = cw.World(tmpdir())
    at = int(time.time()) - 12 * 60
    mine = w.executor(task="mine")
    other = w.executor(task="other")
    done = w.executor(task="verified")
    w.executor(task="dead")
    live = sleeper()
    slot(w, 0, live.pid, "mine", "setup/test-hw (fast)", at)
    slot(w, 1, live.pid, "someone-else", "setup/test-hw (full)", at)
    slot(w, 2, live.pid, "", "hw done verification of verified", at + 60)
    gone = subprocess.Popen(["true"])
    gone.wait()
    slot(w, 3, gone.pid, "dead", "setup/test-hw (fast)", at)
    w.publish()
    s = state(w)
    g = row(s, "mine")["gate"]
    assert g == {"since": at * 1000, "what": "setup/test-hw (fast)"}, "the task's own slot is its gate: %r" % (g,)
    assert row(s, "other")["gate"] is None, "a slot of another task is not this one's gate: %r" % (row(s, "other")["gate"],)
    assert row(s, "verified")["gate"] == {"since": (at + 60) * 1000, "what": "hw done verification of verified"}, \
        "hw done's verification of the task is its gate: %r" % (row(s, "verified")["gate"],)
    assert row(s, "dead")["gate"] is None, "a dead holder runs nothing: %r" % (row(s, "dead")["gate"],)
    del other, done


def gate_from_process():
    """No slot: a running `setup/test-hw` whose cwd is inside the worktree is the task's gate; one elsewhere is not."""
    w = cw.World(tmpdir())
    here = w.executor(task="here")
    away = w.executor(task="away")
    wd = os.path.dirname(os.path.dirname(here["rundir"]))
    os.makedirs(os.path.join(wd, "setup"), exist_ok=True)
    script = os.path.join(wd, "setup", "test-hw")
    open(script, "w").write("#!/bin/sh\nsleep 120\n")  # no exec: the shell stays `bash setup/test-hw`, as a run does
    os.chmod(script, 0o755)
    p = sleeper(cwd=wd, argv=["bash", "setup/test-hw"])
    time.sleep(0.3)
    w.publish()
    s = state(w, HW_COCKPIT_PROCS_TTL_S="0")
    # first: any other `setup/test-hw` on the machine (a suite running this very check) is another tree's
    assert row(s, "away")["gate"] is None, "a run in another tree is not this task's gate: %r" % (row(s, "away")["gate"],)
    g = row(s, "here")["gate"]
    assert g is not None and g["what"] == "setup/test-hw", "a setup/test-hw running in the worktree is its gate: %r" % (g,)
    assert abs(g["since"] - int(time.time() * 1000)) < 10_000, "it began a moment ago: %r" % (g,)
    del away, p


CHECKS = {f.__name__: f for f in (title_after_frontmatter, commits_over_base, commits_cache, gate_from_slot, gate_from_process)}

if __name__ == "__main__":
    BIN = os.path.abspath(sys.argv[1])
    names = sys.argv[2:] or list(CHECKS)
    failed = 0
    try:
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
    finally:
        for p in PROCS:
            try:
                os.killpg(p.pid, signal.SIGTERM)
                p.wait(timeout=5)
            except (OSError, subprocess.SubprocessError):
                pass
        for d in TMPS:
            shutil.rmtree(d, ignore_errors=True)
    sys.exit(1 if failed else 0)
