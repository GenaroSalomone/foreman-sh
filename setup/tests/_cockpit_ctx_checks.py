"""The checks of 922 (a card's context use comes from the pane's statusline), one function each:

    python3 -I _cockpit_ctx_checks.py <bin-dir> <check> [<check> …]

Prints `ok <check>` or `FAIL <check>: <why>` per check and exits 1 on any failure. The bin dir is a
parameter so 922 can run the same check against a mutated copy of the writer and expect it to fail.

The statusline is Claude Code's own account of its context (`Model: … | Ctx: 283.8k | Ctx Used:
28.0% | …`), the source `hw status` reads (lib/hw/status.sh, pane_ctx_parse). The stub herdr of
_cockpit_world.py answers `agent read <pane>` with World.screens[pane]; every call is logged.
"""
import json
import os
import re
import shutil
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import _cockpit_world as cw  # noqa: E402

BIN = None
TMPS = []
EMPTY = {"pct": None, "tokens": None, "source": None}


def tmpdir():
    d = tempfile.mkdtemp(prefix="cockpit-ctx-")
    TMPS.append(d)
    return d


def state(w, **env):
    r = cw.run_writer(w, BIN, "--stdout", **env)
    assert r.returncode == 0, "cockpit-state --stdout exited %d: %s" % (r.returncode, r.stderr)
    s = json.loads(r.stdout)
    errs = cw.validate_state(s)
    assert not errs, "does not validate: " + "; ".join(errs[:3])
    return s


def ctx(s, task):
    for r in s["rows"]:
        if r["task"] == task:
            return r["ctx"]
    raise AssertionError("no row for %s" % task)


def line(tokens, used, model="Sonnet 5.5"):
    """What the statusline of a claude pane prints, with the pane's other rows around it."""
    used_part = "" if used is None else " | Ctx Used: %s%%" % used
    return "some output\n\n  Model: %s | Ctx: %s%s | ⎇ task/x | (+0,-0)\n  ⏵⏵ bypass permissions on\n" % (model, tokens, used_part)


def usage_line(tokens, model):
    return json.dumps({"type": "assistant", "message": {"model": model, "usage": {"input_tokens": tokens}}}) + "\n"


def transcript(w, x, sid, tokens, model):
    cfg = os.path.join(w.tmp, "claude-config")
    with open(os.path.join(x["rundir"], "receipt.jsonl"), "a") as fh:
        fh.write(json.dumps({"key": "session_id", "value": sid}) + "\n")
    with open(os.path.join(x["rundir"], "env"), "a") as fh:
        fh.write("CLAUDE_CONFIG_DIR='%s'\n" % cfg)
    launch = os.path.dirname(os.path.dirname(x["rundir"]))
    pdir = os.path.join(cfg, "projects", re.sub(r"[^A-Za-z0-9]", "-", launch))
    os.makedirs(pdir, exist_ok=True)
    open(os.path.join(pdir, sid + ".jsonl"), "w").write(usage_line(tokens, model))


# ── which number the card shows ──────────────────────────────────────────────

def statusline_is_the_source():
    """The statusline's tokens and percent win over a turn-end value and over the transcript, on a working card or an idle one."""
    w = cw.World(tmpdir())
    now = time.time()
    wrong = {"turn_state": "ended_unreported", "turn_ended_at": cw.iso(now - 600), "ctx_pct": "100.0", "ctx_tokens": "200000"}
    a = w.executor(task="working", status="working", tokens=wrong, screen=line("283.8k", "28.0"))
    transcript(w, a, "33333333-aaaa-bbbb-cccc-000000000001", 250000, "claude-sonnet-5-5")
    w.executor(task="idle", status="idle", tokens=wrong, screen=line("1.2M", "41.0"))
    w.executor(task="small", status="working", screen=line("900", "0.1"))
    w.publish()
    s = state(w)
    assert ctx(s, "working") == {"pct": 28.0, "tokens": 283800, "source": "statusline"}, \
        "a card with a statusline must show the statusline, not the turn-end 100%%: %s" % ctx(s, "working")
    assert ctx(s, "idle") == {"pct": 41.0, "tokens": 1200000, "source": "statusline"}, ctx(s, "idle")
    assert ctx(s, "small") == {"pct": 0.1, "tokens": 900, "source": "statusline"}, ctx(s, "small")


def unknown_percent_is_tokens_only():
    """A narrow pane cuts the footer to `Ctx: 237.5k | Ctx Use…`: the tokens are known, the percent is not, and nothing is guessed."""
    w = cw.World(tmpdir())
    w.executor(task="narrow", status="working", screen="  Model: Sonnet 5.5 | Ctx: 237.5k | Ctx Use...\n")
    w.publish()
    got = ctx(state(w), "narrow")
    assert got == {"pct": None, "tokens": 237500, "source": "statusline"}, "tokens without a percent: %s" % got


def no_window_is_assumed():
    """Without a statusline the fallbacks give tokens, and a percent only when the model id names its window ([1m])."""
    w = cw.World(tmpdir())
    a = w.executor(task="plain", status="working")
    transcript(w, a, "33333333-aaaa-bbbb-cccc-000000000002", 283800, "claude-sonnet-5-5")
    b = w.executor(task="named", status="working")
    transcript(w, b, "33333333-aaaa-bbbb-cccc-000000000003", 100000, "claude-opus-4-7[1m]")
    w.executor(task="turnend", status="idle", tokens={"turn_state": "ended_unreported", "ctx_pct": "null", "ctx_tokens": "283800"})
    w.publish()
    s = state(w)
    assert ctx(s, "plain") == {"pct": None, "tokens": 283800, "source": "live"}, \
        "a window was assumed for a model that does not name one: %s" % ctx(s, "plain")
    assert ctx(s, "named") == {"pct": 10.0, "tokens": 100000, "source": "live"}, ctx(s, "named")
    assert ctx(s, "turnend") == {"pct": None, "tokens": 283800, "source": "turn-end"}, \
        "a turn-end value with no percent must show its tokens: %s" % ctx(s, "turnend")


def falls_back_when_the_pane_cannot_be_read():
    """No statusline on the screen, a failing read, or a non-claude pane: the old sources, never a statusline guess."""
    w = cw.World(tmpdir())
    tok = {"turn_state": "ended_unreported", "ctx_pct": "42.5", "ctx_tokens": "85000"}
    w.executor(task="blank", status="idle", tokens=tok, screen="a shell prompt, no statusline here\n")
    w.executor(task="unreadable", status="idle", tokens=tok)                       # the read fails
    w.executor(task="opencode", vendor="opencode", status="working", screen=line("500k", "50.0"))
    w.publish()
    s = state(w)
    for t in ("blank", "unreadable"):
        assert ctx(s, t) == {"pct": 42.5, "tokens": 85000, "source": "turn-end"}, "%s: %s" % (t, ctx(s, t))
    assert ctx(s, "opencode") == EMPTY, "an opencode pane's screen is not a claude statusline: %s" % ctx(s, "opencode")


# ── what it costs ────────────────────────────────────────────────────────────

def reads_are_cached():
    """Panes are read once per TTL, shared by the writers through a file; `--stdout` writes none."""
    w = cw.World(tmpdir())
    for i in range(6):
        w.executor(task="t%d" % i, status="working", screen=line("%dk" % (100 + i), "10.0"))
    w.publish()
    cw.run_writer(w, BIN, "--stdout")
    assert not os.path.exists(os.path.join(w.work, ".cockpit")), "--stdout created something"
    first = w.reads()
    assert first == 6, "the first write must read each claude pane once: %d" % first
    assert cw.run_writer(w, BIN, "--once").returncode == 0
    n1 = w.reads() - first
    assert n1 == 6, "the first --once read %d panes, not 6" % n1
    assert cw.run_writer(w, BIN, "--once").returncode == 0                      # another process, inside the TTL
    n2 = w.reads() - first - n1
    assert n2 == 0, "panes read again inside the TTL: %d reads (cache ignored)" % n2
    assert cw.run_writer(w, BIN, "--once", HW_COCKPIT_CTX_TTL_S="0").returncode == 0
    n3 = w.reads() - first - n1 - n2
    assert n3 == 6, "with the TTL at 0 every pane is read again: %d" % n3


def read_timeout_is_bounded():
    """A herdr that hangs on `agent read` costs one timeout for all panes, not one per pane, and the card falls back."""
    w = cw.World(tmpdir())
    for i in range(8):
        w.executor(task="t%d" % i, status="idle", tokens={"turn_state": "ended_unreported", "ctx_pct": "5.0", "ctx_tokens": "10000"},
                   screen=line("1M", "9.0"))
    w.publish(hang_reads=True)
    t0 = time.time()
    r = cw.run_writer(w, BIN, "--stdout", HW_COCKPIT_CTX_TIMEOUT_S="1")
    took = time.time() - t0
    assert r.returncode == 0, r.stderr
    s = json.loads(r.stdout)
    assert all(row["ctx"]["source"] == "turn-end" for row in s["rows"]), "a read that timed out must fall back: %s" % [row["ctx"] for row in s["rows"]]
    assert took < 6, "8 hanging reads took %.1f s: they are not read in parallel, or the timeout is not honoured" % took


def brainers_do_not_share_reads_or_cache():
    """A writer reads only its own brainer's claude panes, and two brainers' caches are two files: neither drops the other's entries."""
    w = cw.World(tmpdir())
    w.executor(task="mine", status="working", screen=line("100k", "10.0"))
    w.executor(task="theirs", invoker="w2:p9", status="working", screen=line("200k", "20.0"))
    w.publish()
    assert cw.run_writer(w, BIN, "--once").returncode == 0
    assert w.reads() == 1, "a writer read %d panes: another brainer's executor must not be read" % w.reads()          # 922-M06
    other = dict(HW_COCKPIT_INVOKER="w2:p9")
    assert cw.run_writer(w, BIN, "--once", **other).returncode == 0
    assert w.reads() == 2, "the other brainer's writer must read its own pane once: %d" % w.reads()
    assert cw.run_writer(w, BIN, "--once").returncode == 0
    assert w.reads() == 2, "the first brainer's cache was overwritten by the second's writer (%d reads)" % w.reads()    # 922-M07


CHECKS = {f.__name__: f for f in (statusline_is_the_source, unknown_percent_is_tokens_only, no_window_is_assumed,
                                  falls_back_when_the_pane_cannot_be_read, reads_are_cached, read_timeout_is_bounded, brainers_do_not_share_reads_or_cache)}

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
