"""The checks of 797 (the cockpit state writer), one function each, runnable alone:

    python3 -I _cockpit_writer_checks.py <bin-dir> <check> [<check> …]

Prints `ok <check>` or `FAIL <check>: <why>` per check and exits 1 on any failure. The
bin dir is a parameter so 797 can run the same check against a mutated copy of the
writer and expect it to fail. Everything is built on disk in a temp dir by
_cockpit_world.py; nothing here touches herdr or the real work root.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import _cockpit_world as cw  # noqa: E402

BIN = None
TMPS = []


def tmpdir():
    d = tempfile.mkdtemp(prefix="cockpit-writer-")
    TMPS.append(d)
    return d


def state(w):
    s = cw.stdout_state(w, BIN)
    errs = cw.validate_state(s)
    assert not errs, "does not validate: " + "; ".join(errs[:3])
    return s


def row(s, task):
    for r in s["rows"]:
        if r["task"] == task:
            return r
    raise AssertionError("no row for %s in %s" % (task, [r["task"] for r in s["rows"]]))


def long_text(n):
    words = ["alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"]
    out, i = "", 0
    while len(out) < n:
        out += words[i % len(words)] + " "
        i += 1
    return out[:n].rstrip()


# ── the classes of attention, from the disk facts ────────────────────────────

def classes():
    w = cw.World(tmpdir())
    now = time.time()
    summary = long_text(300)
    done_tokens = dict(cw.chunk("sum", summary, 7), done_status="done", done_state="delivered", done_at=cw.iso(now - 90),
                       artifacts="2", engram="brain", hw_invoker=w.invoker)
    w.executor(task="chal", status="idle", holds=[dict(kind="challenge", n=1)], tokens={"ask_seq": "1/3", "ask_state": "delivered"})
    w.executor(task="asked", status="idle", holds=[dict(kind="ask", n=2)], tokens={"ask_seq": "2/3"})
    w.executor(task="asked-undelivered", status="working", holds=[dict(kind="ask", n=1, state="undelivered")])
    w.executor(task="opencode-ask", vendor="opencode", status="idle", holds=[dict(kind="ask", n=1, route="opencode", target="sess-1")])
    w.executor(task="blocked", status="idle", done=True, blocked_waiting=True,
               tokens=dict(done_tokens, done_status="blocked"))
    w.executor(task="reported", status="idle", done=True, tokens=done_tokens)
    w.executor(task="busy", status="working", tokens={"turn_state": "working", "turns": "4", "turn_ended_at": cw.iso(now - 30),
                                                      "ctx_pct": "42.5", "ctx_tokens": "85000"})
    w.executor(task="child", status="idle", tokens={"turn_state": "ended_awaiting_child", "children_running": "2 shell"})
    w.executor(task="quiet", status="idle", tokens={"turn_state": "ended_unreported"}, rulings=["fix a", "fix b"])
    w.executor(task="second", status="idle", seq=2, done=False, tokens={"done_status": "done", "done_state": "delivered",
                                                                         "done_at": cw.iso(now - 900), "sum": "task one"})
    w.executor(task="foreign", invoker="w2:p9", status="working")
    w.executor(task="nohr", with_run=False, status="working")
    w.noise(3)
    w.publish()
    s = state(w)

    assert s["invoker_pane"] == w.invoker and s["herdr"]["ok"] is True, s["herdr"]
    tasks = [r["task"] for r in s["rows"]]
    assert "foreign" not in tasks, "another brainer's executor is listed"      # 797-M03
    assert "nohr" not in tasks, "a pane with no run directory is listed"
    assert tasks[0] == "chal", "a challenge is not first: %s" % tasks
    c = row(s, "chal")
    assert c["attention"] == "challenge" and c["ask"]["kind"] == "challenge" and c["ask"]["state"] == "delivered", c["ask"]
    assert c["ask"]["seq"] == "1/3", c["ask"]
    assert isinstance(c["ask"]["pending_reply"], str) and c["ask"]["pending_reply"].endswith("pending-reply-1"), c["ask"]
    assert os.path.isfile(c["ask"]["pending_reply"]), "pending_reply is not a file that exists"
    assert c["actions"]["ruling"] is False and "ruling" in c["actions"]["why_not"], "an idle holder must not get a ruling button"
    a = row(s, "asked")
    assert a["attention"] == "ask" and a["ask"]["seq"] == "2/3", a["ask"]
    u = row(s, "asked-undelivered")
    assert u["ask"]["state"] == "undelivered" and u["ask"]["pending_reply"] is None, "pending_reply set on an undelivered hold: %s" % u["ask"]    # 797-M08
    o = row(s, "opencode-ask")
    assert o["ask"]["pending_reply"] is None, "pending_reply set on a hold on another route, which the herdr verbs would refuse"  # 797-M08
    assert o["vendor"] == "opencode"
    b = row(s, "blocked")
    assert b["attention"] == "blocked" and b["report"]["status"] == "blocked", b["report"]
    r = row(s, "reported")
    assert r["attention"] == "report" and r["report"]["status"] == "done" and r["report"]["state"] == "delivered", r["report"]
    assert r["report"]["summary"] == summary, "summary not rejoined: %r" % r["report"]["summary"]   # 797-M07
    assert r["report"]["artifacts"] == 2 and r["report"]["engram"] == "brain", r["report"]
    assert r["report"]["envelope_id"].endswith(":done") and abs(r["report"]["at"] - int((now - 90) * 1000)) < 2000, r["report"]
    bz = row(s, "busy")
    assert bz["attention"] == "working" and bz["ctx"] == {"pct": 42.5, "tokens": 85000, "source": "turn-end"}, bz["ctx"]
    assert bz["turns"] == 4 and bz["turn_state"] == "working"
    assert row(s, "quiet")["ctx"] == {"pct": None, "tokens": None, "source": None}, "ctx must be null, never a guess"
    ch = row(s, "child")
    assert ch["attention"] == "working" and ch["children_running"] == "2 shell", ch["attention"]
    q = row(s, "quiet")
    assert q["attention"] == "idle" and q["rulings_pending"]["count"] == 2 and q["rulings_pending"]["oldest_at"], q["rulings_pending"]
    sec = row(s, "second")
    assert sec["task_n"] == 2 and sec["report"] is None, "task 1's done tokens must not make task 2 a report"
    assert sec["attention"] == "idle"
    t = s["totals"]
    assert t["executors"] == len(s["rows"]) == 10 and t["omitted"] == 0
    assert (t["challenges"], t["asks"], t["blocked"], t["reports"], t["working"], t["idle"]) == (1, 3, 1, 1, 2, 2), t
    assert t["rulings_pending"] == 2, t
    order = ["challenge", "ask", "blocked", "report", "working", "idle", "gone"]
    keys = [(order.index(x["attention"]), x["attention_since"] or 0, x["id"]) for x in s["rows"]]
    assert keys == sorted(keys), "rows are out of order: %s" % keys
    assert s["rules"]["cut"] == {"running": False, "since_ms": None, "holder": None}


def done_tokens_outlive_the_task():
    """Task 1's done tokens stay on the pane while task 2 works: no `done` marker for task 2
    means no report. An UNDELIVERED report with no marker is the opposite case — stranded, and
    exactly what the person must see — and only counts when it is newer than this task."""
    w = cw.World(tmpdir())
    now = time.time()
    old = {"done_status": "done", "done_state": "delivered", "done_at": cw.iso(now - 900), "sum": "task one", "artifacts": "1"}
    w.executor(task="two", seq=2, status="idle", tokens=old)
    time.sleep(0.01)
    stranded = {"done_status": "done", "done_state": "undelivered", "done_at": cw.iso(now + 5), "sum": "never arrived", "artifacts": "0"}
    w.executor(task="stranded", seq=1, status="idle", tokens=stranded)
    stale_undelivered = {"done_status": "done", "done_state": "undelivered", "done_at": cw.iso(now - 4000), "sum": "task one lost", "artifacts": "0"}
    w.executor(task="two-undelivered-old", seq=2, status="idle", tokens=stale_undelivered)
    w.publish()
    s = state(w)
    assert row(s, "two")["report"] is None and row(s, "two")["attention"] == "idle", \
        "task 1's done tokens made task 2 a report: %s" % row(s, "two")["report"]                         # 797-M09
    assert row(s, "two-undelivered-old")["report"] is None, "an old undelivered report of task 1 is shown against task 2"
    st = row(s, "stranded")
    assert st["report"]["state"] == "undelivered" and st["attention"] == "report", "a stranded report must show: %s" % st["report"]


def row_order():
    """hw's order: attention rank, then attention_since ascending, then id."""
    w = cw.World(tmpdir())
    now = time.time()
    w.executor(task="z-idle", status="idle")
    w.executor(task="a-working", status="working")
    rep = {"done_status": "done", "done_state": "delivered", "done_at": cw.iso(now - 10), "sum": "r"}
    w.executor(task="report", status="idle", done=True, tokens=rep)
    late = w.executor(task="ask-late", status="idle", holds=[dict(kind="ask", n=1)])
    early = w.executor(task="ask-early", status="idle", holds=[dict(kind="ask", n=1)])
    w.executor(task="chal", status="idle", holds=[dict(kind="challenge", n=1)])
    os.utime(os.path.join(late["taskdir"], "pending-reply-1"), (now - 10, now - 10))
    os.utime(os.path.join(early["taskdir"], "pending-reply-1"), (now - 500, now - 500))
    w.publish()
    got = [r["task"] for r in state(w)["rows"]]
    want = ["chal", "ask-early", "ask-late", "report", "a-working", "z-idle"]
    assert got == want, "rows are not in hw's order: %s, want %s" % (got, want)                      # 797-M10


def private_files():
    """The state file holds report summaries: 0600 in a 0700 directory."""
    w = cw.World(tmpdir())
    w.executor(task="a", status="working")
    w.publish()
    assert cw.run_writer(w, BIN, "--once").returncode == 0
    mode = os.stat(w.state_path()).st_mode & 0o777
    dmode = os.stat(os.path.dirname(w.state_path())).st_mode & 0o777
    assert mode == 0o600, "the state file is mode %o, not 600" % mode                                     # 797-M20
    assert dmode == 0o700, "the state directory is mode %o, not 700" % dmode


def loop_survives_a_failed_write():
    """A write that raises ends one beat, not the heartbeat."""
    w = cw.World(tmpdir())
    w.executor(task="a", status="working")
    w.panes.append({"pane_id": w.invoker, "agent_status": "idle", "cwd": w.tmp})
    w.publish()
    os.makedirs(os.path.dirname(w.state_path()))
    os.mkdir(w.state_path() + ".lock")  # a directory where the lock file goes: every write raises
    cmd = '"%s/cockpit-state" --loop 1 --bind-pane --invoker %s >/dev/null 2>%s/err & echo $!' % (BIN, w.invoker, w.tmp)
    pid = int(subprocess.run(["sh", "-c", cmd], env=w.env(), capture_output=True, text=True, timeout=20).stdout.strip())
    try:
        err = os.path.join(w.tmp, "err")
        deadline = time.time() + 15
        while time.time() < deadline and _alive(pid) and "write failed" not in open(err).read():
            time.sleep(0.1)  # the first failed beat has happened (or the writer is dead)
        time.sleep(2.5)  # and a second beat has had its chance to kill it
        assert _alive(pid), "the heartbeat died with its first failed write"                                  # 797-M21
        os.rmdir(w.state_path() + ".lock")
        deadline = time.time() + 5
        while time.time() < deadline and not os.path.exists(w.state_path()):
            time.sleep(0.2)
        assert os.path.exists(w.state_path()), "the heartbeat never recovered once the write could succeed"
    finally:
        if _alive(pid):
            os.kill(pid, 9)


def herdr_down():
    w = cw.World(tmpdir())
    w.executor(task="x")
    w.publish(fail=True)
    s = state(w)
    assert s["herdr"]["ok"] is False and s["herdr"]["error"], s["herdr"]
    assert s["rows"] == [] and s["totals"]["executors"] == 0, "rows must be empty when herdr is unreachable"


def rows_cap():
    w = cw.World(tmpdir())
    for i in range(70):
        w.executor(task="t%02d" % i, status="working")
    w.executor(task="urgent", status="idle", holds=[dict(kind="challenge", n=1)])
    w.publish()
    s = state(w)
    assert len(s["rows"]) == 64 and s["totals"]["omitted"] == 7 and s["totals"]["executors"] == 64, "rows capped at 64 but totals.omitted is wrong: %s" % s["totals"]   # 797-M12
    assert s["rows"][0]["task"] == "urgent", "a challenge must be the last row to go, so it is the first"


def size_cap():
    w = cw.World(tmpdir())
    for i in range(64):
        w.executor(task="t%02d" % i, status="working", tokens={"children_running": ("x" * 8000)})
    w.publish()
    r = cw.run_writer(w, BIN, "--once")
    assert r.returncode == 0, r.stderr
    raw = open(w.state_path(), "rb").read()
    assert len(raw) <= 256 * 1024, "state file is %d bytes, over the 256 KiB cap" % len(raw)                       # 797-M04
    d = json.loads(raw)
    assert not cw.validate_state(d)
    assert d["totals"]["omitted"] > 0 and d["totals"]["executors"] == len(d["rows"]), d["totals"]
    # the cap binds the file AS WRITTEN (indented), not a compact rendering of it: sweep the row size
    # across the band where the compact form fits and the indented one does not
    for size in range(3000, 4200, 100):
        w = cw.World(tmpdir())
        for i in range(64):
            w.executor(task="t%02d" % i, status="working", tokens={"children_running": ("x" * size)})
        w.publish()
        assert cw.run_writer(w, BIN, "--once").returncode == 0
        n = len(open(w.state_path(), "rb").read())
        assert n <= 256 * 1024, "rows of %d bytes: the written file is %d bytes, over the 256 KiB cap" % (size, n)   # 797-M04


def summary_rejoin():
    """What done-invoker publishes (80-char chunks, whitespace kept then trimmed by herdr)
    reads back as the text it carried, for soft breaks AND a hard cut inside a long word."""
    w = cw.World(tmpdir())
    text = ("The four preview rows were written with the value of the running container: " + "/opt/demo/very/long/path/" * 4 +
            "/report.html (section 'Controls', index updated) — Top 3: (1) typing a date changes Date but not the table")
    toks = dict(cw.chunk("sum", text, 7), done_status="done", done_state="delivered", done_at=cw.iso(time.time()),
                artifacts="0", engram="brain")
    w.executor(task="r", done=True, tokens=toks)
    w.publish()
    got = row(state(w), "r")["report"]["summary"]
    assert got == text[:len(got)] and len(got) == len(text), "rejoined %r want %r" % (got, text)    # 797-M07


def atomic():
    """A reader never sees half a file, and each write is a new inode (tmp + replace)."""
    w = cw.World(tmpdir())
    for i in range(30):
        w.executor(task="t%02d" % i, status="working")
    w.publish()
    path = w.state_path()
    assert cw.run_writer(w, BIN, "--once").returncode == 0
    stop, bad, seen, inodes = threading.Event(), [], [0], set()

    def reader():
        while not stop.is_set():
            try:
                raw = open(path, "rb").read()
                inodes.add(os.stat(path).st_ino)
            except OSError:
                continue
            seen[0] += 1
            try:
                d = json.loads(raw)
                if d["schema"] != 1:
                    bad.append("schema")
            except ValueError:
                bad.append("unparsable %d bytes" % len(raw))

    th = threading.Thread(target=reader)
    th.start()
    writes = 0
    while writes < 120 or (seen[0] < 1000 and writes < 800):  # at least 120 writes and 1000 concurrent reads
        assert cw.run_writer(w, BIN, "--once").returncode == 0
        writes += 1
    stop.set()
    th.join()
    assert seen[0] >= 1000, "only %d reads happened during %d writes" % (seen[0], writes)
    assert not bad, "a reader saw a partial file: %s (after %d reads)" % (bad[:3], seen[0])
    assert len(inodes) > 20, "the file was rewritten in place (%d inodes in %d writes)" % (len(inodes), writes)       # 797-M01
    sys.stderr.write("  atomic: %d concurrent reads over %d writes, %d inodes, none partial\n" % (seen[0], writes, len(inodes)))
    seqs = []
    for _ in range(3):
        cw.run_writer(w, BIN, "--once")
        seqs.append(json.load(open(path))["writer"]["seq"])
    assert seqs == [seqs[0], seqs[0] + 1, seqs[0] + 2], "seq does not rise by one per write: %s" % seqs


def killed_writer():
    """kill -9 in the middle of a write leaves the old file or the new one, never half of either,
    and the tmp file it leaves behind is cleaned by the next writer. The kill is aimed: the
    writer is watched until its tmp file appears (it exists only between the open and the rename)
    and killed that instant, so the window under test is the one that matters."""
    import glob as _glob
    import signal
    w = cw.World(tmpdir())
    for i in range(30):
        w.executor(task="t%02d" % i, status="working")
    w.publish()
    path = w.state_path()
    assert cw.run_writer(w, BIN, "--once").returncode == 0
    killed_in_window = 0
    for _ in range(60):
        for t in _glob.glob(path + ".tmp.*"):  # only THIS writer's tmp may trigger the kill
            os.unlink(t)
        p = subprocess.Popen([os.path.join(BIN, "cockpit-state"), "--once"], env=w.env(), stdout=subprocess.DEVNULL,
                             stderr=subprocess.DEVNULL)
        deadline = time.time() + 3
        while p.poll() is None and time.time() < deadline and not _glob.glob(path + ".tmp.*"):
            pass
        if p.poll() is None:
            p.send_signal(signal.SIGKILL)
        p.wait()
        if _glob.glob(path + ".tmp.*"):
            killed_in_window += 1
        d = json.load(open(path))  # raises on a partial file
        assert d["schema"] == 1 and not cw.validate_state(d), "the file is not a valid state after a kill -9"
    assert killed_in_window >= 3, "only %d of 60 writers were caught between tmp and rename: the check proves nothing" % killed_in_window
    open(path + ".tmp.99999999", "w").write("{")  # a killed writer's leftover, whatever the last kill did
    assert cw.run_writer(w, BIN, "--once").returncode == 0
    left = [f for f in os.listdir(os.path.dirname(path)) if ".tmp." in f]
    assert not left, "tmp files of killed writers survive the next write: %s" % left                          # 797-M22
    sys.stderr.write("  killed_writer: %d of 60 writers killed between tmp and rename, the file was always whole\n" % killed_in_window)


def debounce():
    """Fifteen events in one burst make at most three writes, and the LAST write sees an
    event that arrived after the first one (trailing edge, not dropped). The burst is
    fired from ONE process (the kick call itself takes microseconds; a fresh interpreter
    per event would spread the burst over seconds on a loaded machine and test the load)."""
    import importlib.machinery
    import importlib.util
    w = cw.World(tmpdir())
    w.executor(task="a", status="working")
    w.publish()
    path = w.state_path()
    cw.run_writer(w, BIN, "--once")
    base = json.load(open(path))["writer"]["seq"]
    os.environ.update(w.env())
    loader = importlib.machinery.SourceFileLoader("cs", os.path.join(BIN, "cockpit-state"))
    mod = importlib.util.module_from_spec(importlib.util.spec_from_loader("cs", loader))
    loader.exec_module(mod)
    for i in range(15):
        mod.kick(w.invoker, path)
        if i == 3:
            w.executor(task="late", status="working")
            w.publish()
    deadline = time.time() + 20
    while time.time() < deadline and os.path.isdir(path + ".kick"):
        time.sleep(0.1)
    time.sleep(1.0)
    d = json.load(open(path))
    writes = d["writer"]["seq"] - base
    assert 1 <= writes <= 3, "%d writes for 15 events" % writes                                              # 797-M02
    assert any(r["task"] == "late" for r in d["rows"]), "the event that arrived mid-burst never reached the file"


def kick_names_its_brainer():
    """An executor's kick names the brainer it reports to. When that name does not resolve
    (an empty --invoker, a run env with no HW_INVOKER_PANE) the kick must NOT fall back to the
    environment's pane (HERDR_PANE_ID, which in an executor is its own): it writes nothing and
    leaves a line in skipped-kicks.log. A run that does name its brainer still kicks that one."""
    w = cw.World(tmpdir())
    other = "w1:p77"  # the executor's own pane, as its environment says
    ex = w.executor(task="a", status="working")
    w.publish()
    env = {"HERDR_PANE_ID": other}
    env_no = os.path.join(ex["rundir"], "env")
    with open(env_no) as fh:
        keep = [l for l in fh if "HW_INVOKER_PANE" not in l]
    own = os.path.join(w.work, ".cockpit", other + ".json")
    log = os.path.join(w.work, ".cockpit", "skipped-kicks.log")

    def settle(path):
        end = time.time() + 3
        while time.time() < end and not os.path.exists(path):
            time.sleep(0.1)
        time.sleep(0.6)

    for label, args, mutate in (("an empty --invoker", ["--kick", "--invoker", ""], False),
                                ("a run env without the invoker", ["--kick", "--rundir", ex["rundir"]], True)):
        if mutate:
            open(env_no, "w").writelines(keep)
        r = subprocess.run([os.path.join(BIN, "cockpit-state")] + args, env=w.env(HW_COCKPIT_INVOKER="", HERDR_PANE_ID=other, **{}),
                           capture_output=True, text=True, timeout=20)
        assert r.returncode == 0, "%s: a kick never fails (exit %d)" % (label, r.returncode)
        settle(own)
        assert not os.path.exists(own), "%s: the kick fell back to the executor's own pane (%s)" % (label, own)    # 797-M23
        assert os.path.exists(log) and "kick skipped" in open(log).read(), "%s: nothing was logged" % label
    # a run that names its brainer still kicks that brainer, and only that one
    ex2 = w.executor(task="b", status="working")
    w.publish()
    r = subprocess.run([os.path.join(BIN, "cockpit-state"), "--kick", "--rundir", ex2["rundir"]], env=w.env(HERDR_PANE_ID=other),
                       capture_output=True, text=True, timeout=20)
    assert r.returncode == 0
    settle(w.state_path())
    assert os.path.exists(w.state_path()), "a kick from a run that names its brainer wrote nothing"
    assert not os.path.exists(own), "the kick of a resolved run also wrote the executor's own pane"


def _gone_within(pid, seconds):
    deadline = time.time() + seconds
    while time.time() < deadline:
        try:
            os.kill(pid, 0)
        except OSError:
            return True
        time.sleep(0.2)
    os.kill(pid, 9)
    return False


def loop_dies_with_parent():
    """Two ways to outlive the pane, both refused: the launching shell is already gone when the
    writer starts (python takes ~100 ms to boot), and it goes while the writer is running."""
    w = cw.World(tmpdir())
    w.executor(task="a", status="working")
    w.publish()
    cmd = '"%s/cockpit-state" --loop 1 >/dev/null 2>&1 & echo $!' % BIN
    out = subprocess.run(["sh", "-c", cmd], env=w.env(), capture_output=True, text=True, timeout=20)
    assert _gone_within(int(out.stdout.strip()), 6), "the --loop writer started by a shell that exited at once is still running"   # 797-M17
    out = subprocess.run(["sh", "-c", cmd + "; sleep 2"], env=w.env(), capture_output=True, text=True, timeout=30)
    assert _gone_within(int(out.stdout.strip()), 6), "the --loop writer outlived the shell that started it"                       # 797-M11


def _alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except OSError:
        return False


def loop_binds_to_pane():
    """`bin/brain` starts the heartbeat and exits; claude lives in the PANE. So the writer is
    detached, there is ONE per brainer, it outlives the shell that started it, survives a
    herdr outage, and leaves when the brainer's pane does."""
    w = cw.World(tmpdir())
    w.executor(task="a", status="working")
    w.panes.append({"pane_id": w.invoker, "agent_status": "idle", "cwd": w.tmp})
    w.publish()
    cmd = '"%s/cockpit-state" --loop 1 --bind-pane --invoker %s >/dev/null 2>&1 & echo $!' % (BIN, w.invoker)
    first = int(subprocess.run(["sh", "-c", cmd], env=w.env(), capture_output=True, text=True, timeout=20).stdout.strip())
    try:
        deadline = time.time() + 10
        while time.time() < deadline and not os.path.exists(w.state_path()):
            time.sleep(0.2)
        assert os.path.exists(w.state_path()), "the detached writer wrote nothing"
        time.sleep(2.5)
        assert _alive(first), "the --bind-pane writer died with the shell that started it"
        second = int(subprocess.run(["sh", "-c", cmd], env=w.env(), capture_output=True, text=True, timeout=20).stdout.strip())
        gone = False
        for _ in range(40):
            if not _alive(second):
                gone = True
                break
            time.sleep(0.2)
        if not gone:
            os.kill(second, 9)
        assert gone, "a second writer for the same brainer keeps running"                                   # 797-M19
        w.publish(fail=True)  # herdr unreachable: an outage is not a closed pane
        time.sleep(2.5)
        assert _alive(first), "the writer left when herdr stopped answering"
        assert json.load(open(w.state_path()))["herdr"]["ok"] is False, "the outage is not written"
        w.publish()
        w.panes[:] = [p for p in w.panes if p["pane_id"] != w.invoker]  # the brainer's pane closes
        w.publish()
        assert _gone_within(first, 8), "the writer outlived its brainer's pane"                             # 797-M18
    finally:
        if _alive(first):
            os.kill(first, 9)


def ctx_from_transcript():
    d = tmpdir()
    p = os.path.join(d, "t.jsonl")
    with open(p, "w") as fh:
        fh.write(json.dumps({"type": "assistant", "message": {"model": "claude-sonnet-5-5", "usage": {"input_tokens": 10, "cache_read_input_tokens": 1000, "cache_creation_input_tokens": 90}}}) + "\n")
        fh.write(json.dumps({"type": "assistant", "isSidechain": True, "message": {"usage": {"input_tokens": 190000}}}) + "\n")
        fh.write(json.dumps({"type": "assistant", "message": {"model": "claude-sonnet-5-5", "usage": {"input_tokens": 4424, "cache_read_input_tokens": 40000, "cache_creation_input_tokens": 0}}}) + "\n")
        fh.write(json.dumps({"type": "user", "message": {"content": "x"}}) + "\n")
    r = subprocess.run([os.path.join(BIN, "cockpit-state"), "ctx", p], capture_output=True, text=True)
    assert r.returncode == 0 and json.loads(r.stdout) == {"pct": 22.2, "tokens": 44424}, r.stdout + r.stderr   # P5: 44424 → 22 %
    big = os.path.join(d, "m.jsonl")
    open(big, "w").write(json.dumps({"type": "assistant", "message": {"model": "claude-opus-4-7[1m]", "usage": {"input_tokens": 100000}}}) + "\n")
    r = subprocess.run([os.path.join(BIN, "cockpit-state"), "ctx", big], capture_output=True, text=True)
    assert json.loads(r.stdout) == {"pct": 10.0, "tokens": 100000}, "the [1m] window is not honoured: %s" % r.stdout
    none = os.path.join(d, "n.jsonl")
    open(none, "w").write('{"type":"user"}\n')
    assert subprocess.run([os.path.join(BIN, "cockpit-state"), "ctx", none], capture_output=True).returncode == 1
    assert subprocess.run([os.path.join(BIN, "cockpit-state"), "ctx", os.path.join(d, "absent")], capture_output=True).returncode == 1


def cut_rule():
    w = cw.World(tmpdir())
    w.executor(task="a")
    slot = os.path.join(w.work, ".locks", "suite-gate", "slot.0")
    os.makedirs(slot)
    me = subprocess.Popen(["sleep", "30"])
    try:
        start = subprocess.run(["ps", "-o", "lstart=", "-p", str(me.pid)], capture_output=True, text=True).stdout
        for name, val in (("owner.pid", str(me.pid)), ("owner.start", " ".join(start.split())), ("owner.task", "cut-task"),
                          ("owner.cmd", "release cut 9.9.9"), ("owner.at", "1790000000")):
            open(os.path.join(slot, name), "w").write(val + "\n")
        w.publish()
        c = state(w)["rules"]["cut"]
        assert c == {"running": True, "since_ms": 1790000000000, "holder": "cut-task"}, c
        open(os.path.join(slot, "owner.cmd"), "w").write("hw done verification of x\n")
        assert state(w)["rules"]["cut"]["running"] is False, "a verification slot is not a cut"
        open(os.path.join(slot, "owner.cmd"), "w").write("release cut 9.9.9\n")
        open(os.path.join(slot, "owner.start"), "w").write("Thu Jan  1 00:00:00 1970\n")
        assert state(w)["rules"]["cut"]["running"] is False, "a reused pid (another start time) is not the holder"   # 797-M16
        open(os.path.join(slot, "owner.start"), "w").write(" ".join(start.split()) + "\n")
        me.kill()
        me.wait()
        assert state(w)["rules"]["cut"]["running"] is False, "a dead holder keeps nothing"
    finally:
        me.kill()


def timing_40():
    """Design H1: under 300 ms for 40 panes. Best of 7, because the machine is shared and a
    single run can land on a load spike; the median and worst are printed too."""
    w = cw.World(tmpdir())
    for i in range(40):
        w.executor(task="t%02d" % i, status=("working" if i % 3 else "idle"), tokens={"turn_state": "ended_holding"},
                   holds=([dict(kind="ask", n=1)] if i % 5 == 0 else []), rulings=(["x"] if i % 7 == 0 else []))
    w.noise(10)
    w.publish()
    ts = []
    for _ in range(7):
        t0 = time.perf_counter()
        r = cw.run_writer(w, BIN, "--once")
        ts.append((time.perf_counter() - t0) * 1000)
        assert r.returncode == 0, r.stderr
    ts.sort()
    sys.stderr.write("  timing: 40 executors + 10 other panes, --once wall ms: best %.0f median %.0f worst %.0f\n" % (ts[0], ts[3], ts[-1]))
    size = os.path.getsize(w.state_path())
    assert size <= 256 * 1024, size
    assert ts[0] < 300, "best of 7 is %.0f ms, the budget is 300" % ts[0]


CHECKS = {f.__name__: f for f in (classes, done_tokens_outlive_the_task, row_order, private_files, killed_writer, loop_survives_a_failed_write, herdr_down, rows_cap, size_cap, summary_rejoin, atomic, debounce,
                                  loop_dies_with_parent, loop_binds_to_pane, ctx_from_transcript, cut_rule, timing_40, kick_names_its_brainer)}

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
