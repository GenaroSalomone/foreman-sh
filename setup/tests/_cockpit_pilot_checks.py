"""Checks of the pilot recorder (bin/cockpit-state): argv[1] is the bin dir under test.
Used by 805. Exits non-zero with the evidence on the first failed check."""
import json
import os
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from _cockpit_world import World, run_writer  # noqa: E402

BIN = sys.argv[1]


def die(msg):
    print("FAIL: " + msg)
    sys.exit(1)


def lines(path):
    try:
        return [json.loads(l) for l in open(path).read().split("\n") if l]
    except OSError:
        return []


def build(tmp):
    w = World(tmp)
    w.executor(task="rep", pane="w1:p11", done=True)
    w.executor(task="ask", pane="w1:p12", holds=[{"kind": "ask"}])
    w.executor(task="chal", pane="w1:p13", holds=[{"kind": "challenge"}])
    w.executor(task="busy", pane="w1:p14")
    w.publish()
    return w


def once(w, **env):
    r = run_writer(w, BIN, "--once", **env)
    if r.returncode != 0:
        die("--once exited %d: %s" % (r.returncode, r.stderr))
    return json.load(open(w.state_path()))


def check_records():
    with tempfile.TemporaryDirectory() as tmp:
        w = build(tmp)
        st = once(w)
        ev = os.path.join(w.work, ".cockpit", "cockpit-events.jsonl")
        got = lines(ev)
        want = {}
        for r in st["rows"]:
            if r["report"]:
                want[(r["pane"], r["task"], "report")] = r["report"]["at"]
            if r["ask"]:
                want[(r["pane"], r["task"], r["ask"]["kind"])] = r["ask"]["at"]
        if sorted(want) != [("w1:p11", "rep", "report"), ("w1:p12", "ask", "ask"), ("w1:p13", "chal", "challenge")]:
            die("the fixture did not produce a report, an ask and a challenge: %r" % sorted(want))
        have = {(e["pane"], e["task"], e["kind"]): e["at"] for e in got}
        if have != want or len(got) != 3 or any(set(e) != {"at", "pane", "task", "kind"} for e in got):
            die("events are not one {at,pane,task,kind} line per new envelope: %r vs %r" % (got, want))
        once(w)
        once(w)
        if len(lines(ev)) != 3:
            die("a repeated write re-recorded envelopes already in the file: %d lines" % len(lines(ev)))
        if os.stat(ev).st_mode & 0o777 != 0o600:
            die("the events file is mode %o, not 600" % (os.stat(ev).st_mode & 0o777))
        w.executor(task="late", pane="w1:p15", done=True)
        w.publish()
        once(w)
        if len(lines(ev)) != 4:
            die("a NEW envelope was not appended: %d lines" % len(lines(ev)))
    print("ok records: one line per new report/ask/challenge, deduplicated, 0600")


def check_rotates():
    with tempfile.TemporaryDirectory() as tmp:
        w = build(tmp)
        cd = os.path.join(w.work, ".cockpit")
        os.makedirs(cd, exist_ok=True)
        ev = os.path.join(cd, "cockpit-events.jsonl")
        filler = json.dumps({"at": 1, "pane": "w0:p0", "task": "old", "kind": "report"}) + "\n"
        open(ev, "w").write(filler * (1024 * 1024 // len(filler) + 1))
        once(w)
        if not os.path.exists(ev + ".1") or os.path.getsize(ev + ".1") < 1024 * 1024:
            die("the full events file was not rotated to .1")
        if len(lines(ev)) != 3:
            die("after rotation the file should hold only the new envelopes: %d lines" % len(lines(ev)))
        # and never inside a repo: the state dir is the work root's .cockpit
        if os.path.exists(os.path.join(cd, ".git")):
            die("events live in a repo")
    print("ok rotates: at 1 MiB the file moves to .1 and a new one starts")


def check_pilot():
    with tempfile.TemporaryDirectory() as tmp:
        w = World(tmp)
        cd = os.path.join(w.work, ".cockpit")
        os.makedirs(cd)
        now = 1_790_000_000_000
        T = 1_789_000_000_000
        ev, ac = [], []
        for i, d in enumerate([10, 20, 30, 100]):
            ev.append({"at": T + i * 1000, "pane": "w1:p%d" % (20 + i), "task": "t%d" % i, "kind": "report"})
            ac.append({"at": T + i * 1000 + d * 1000, "pane": "w1:p%d" % (20 + i), "verb": "done"})
        # an action that PRECEDES its envelope does not answer it
        ev.append({"at": now - 31 * 60000, "pane": "w1:p30", "task": "missed", "kind": "ask"})
        ac.append({"at": now - 40 * 60000, "pane": "w1:p30", "verb": "ruling"})
        ev.append({"at": now - 5 * 60000, "pane": "w1:p31", "task": "recent", "kind": "challenge"})
        open(os.path.join(cd, "cockpit-events.jsonl"), "w").write("".join(json.dumps(e) + "\n" for e in ev))
        open(os.path.join(cd, "cockpit-actions.jsonl"), "w").write("".join(json.dumps(a) + "\n" for a in ac) + "not json\n")
        r = run_writer(w, BIN, "--pilot", HW_COCKPIT_NOW_MS=str(now))
        out = r.stdout
        if r.returncode != 0:
            die("--pilot exited %d: %s" % (r.returncode, r.stderr))
        if "acted on 4 envelopes: median 20.0s, p90 100.0s" not in out:
            die("median/p90 wrong: %r" % out)
        if "no action after 30 min: 1" not in out or "w1:p30 missed ask (31 min ago)" not in out:
            die("the missed envelope is not reported: %r" % out)
        if "recent" in out:
            die("a 5-minute-old envelope was called missed: %r" % out)
        r = run_writer(World(os.path.join(tmp, "x")), BIN, "--pilot")
        if r.returncode != 0 or "acted on 0 envelopes" not in r.stdout:
            die("an empty state dir should print zero, not fail: rc=%d %r %r" % (r.returncode, r.stdout, r.stderr))
    print("ok pilot: median 20.0s, p90 100.0s, one missed after 30 min")


CHECKS = {"records": check_records, "rotates": check_rotates, "pilot": check_pilot}
for name in (sys.argv[2:] or CHECKS):
    CHECKS[name]()
