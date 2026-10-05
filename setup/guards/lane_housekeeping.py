#!/usr/bin/env python3
"""What a brainer's SessionStart does about the lane's leftovers.

TWO THINGS, both measured on 2026-10-01 (inventory: 559 worktrees kept, 30
tasks finished and never reported):

1. THE TASKS THAT FINISHED AND NEVER REPORTED, IN THE BRAINER'S CONTEXT.
   `hw status` already listed them — under a rule that says "run `hw status`
   when you start", which nothing runs. So the background worker runs it,
   scoped to the lane, through `HW_STATUS_ORPHANS_JSON` (never the coloured
   table), and every start puts the last saved queue, with its time, where the
   brainer reads it: `additionalContext`. Not in the hook itself: `hw status`
   took 8 to 37s, measured, and the start must not wait.

2. `hw reap <lane> --apply`, IN THE BACKGROUND. The rule "hw reap after every
   hw done" was unenforced and `hw done` cannot do it in the common path: when
   done-invoker calls it, it dies with its own tab, and the branch is unmerged
   at report time anyway. Measured with a herdr probe: nothing after
   `tab close` runs. So the reap that removes a worktree after its merge runs
   here, detached, with a time cap, and leaves ONE line saying what it removed
   and what it kept. The next SessionStart shows that line.

FAIL CLOSED. reap removes only `safe`/`archivable`, archives before removing
and re-asks the verdict after (bin/hw `_reap_worktree`). Here: a reap that
cannot start, exits non-zero or hits the cap is reported FAILED, the cap kills
its whole process group so nothing more is removed, and every removal it had
already made was individually verified by reap itself. Never two at once per
lane (a lock). Not for an executor (HW_TASK/HW_RUN set) — hw refuses it too.

Nothing here may crash the session: every failure becomes a line or silence.
"""
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import time

GUARDS = os.path.dirname(os.path.realpath(__file__))
TREE = os.path.dirname(os.path.dirname(GUARDS))
DEFAULT_TIMEOUT = 600
STATUS_TIMEOUT = 90
ANSI = re.compile(r"\x1b\[[0-9;]*m")


def _hw():
    return os.environ.get("HW_HOUSEKEEPING_HW") or os.path.join(TREE, "bin", "hw")


def _state_dir():
    if os.environ.get("HW_HOUSEKEEPING_STATE"):
        return os.environ["HW_HOUSEKEEPING_STATE"]
    root = os.environ.get("HW_BRAIN_ROOT")
    if not root:
        # The brain's place is guards.json's `brain_root`, as for the roster.
        from specialist_roster import BRAIN_ROOT
        root = os.path.expanduser(BRAIN_ROOT)
    return os.path.join(root, ".hw-housekeeping")


def _is_executor():
    return bool(os.environ.get("HW_TASK") or os.environ.get("HW_RUN"))


def _timeout():
    try:
        return max(1, int(os.environ.get("HW_BG_REAP_TIMEOUT", DEFAULT_TIMEOUT)))
    except ValueError:
        return DEFAULT_TIMEOUT


def refresh_unreported(lane):
    """Worker side: `hw status <lane>`'s orphan queue, saved for the next start.

    In the worker, not the hook: `hw status` measured 8 to 37s on 2026-10-01,
    and the session start must not wait for it."""
    state = _state_dir()
    dest = os.path.join(state, f"unreported-{lane}.json")
    env = dict(os.environ, HW_STATUS_ORPHANS_JSON=dest + ".tmp", NO_COLOR="1")
    try:
        os.unlink(dest + ".tmp")
    except OSError:
        pass
    try:
        subprocess.run([_hw(), "status", lane], env=env, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, timeout=STATUS_TIMEOUT, check=False)
        with open(dest + ".tmp", encoding="utf-8") as f:
            orphans = json.load(f)
        with open(dest + ".tmp", "w", encoding="utf-8") as f:
            json.dump({"at": time.time(), "orphans": orphans}, f)
        os.replace(dest + ".tmp", dest)
    except (OSError, ValueError, subprocess.TimeoutExpired):
        # No fresh answer: the old snapshot is removed rather than shown as current.
        for f in (dest, dest + ".tmp"):
            try:
                os.unlink(f)
            except OSError:
                pass


def unreported_context(lane):
    """The last saved queue, as context lines, or None. Never runs hw."""
    try:
        with open(os.path.join(_state_dir(), f"unreported-{lane}.json"), encoding="utf-8") as f:
            snap = json.load(f)
        orphans, at = snap.get("orphans") or [], snap.get("at")
    except (OSError, ValueError, AttributeError):
        return None
    if not orphans:
        return None
    asof = time.strftime("%d %b %H:%M", time.localtime(at)) if at else "?"
    lines = [f"## Tareas que terminaron y NUNCA reportaron ({lane}): {len(orphans)}", "",
             f"Según `hw status {lane}` a las {asof} (lo actualiza la limpieza en segundo "
             "plano de cada arranque). Nadie fue notificado: recuperá el reporte o cerralas "
             f"con `hw done {lane} <task>`.", ""]
    for o in orphans[:15]:
        when = time.strftime("%d %b %H:%M", time.localtime(o["at"])) if o.get("at") else "?"
        lines.append(f"- `{o.get('key')}` — {when} — {o.get('why', '')}")
    if len(orphans) > 15:
        lines.append(f"- … y {len(orphans) - 15} más")
    return "\n".join(lines)


def summarize(lane, output, rc, timed_out, timeout, log):
    """The one line. Built from reap's own output, never from its intent."""
    text = ANSI.sub("", output)
    removed = re.findall(r"removed (\S+)", text)
    removed = [r for r in removed if r not in ("removed,",)]
    deleted = re.findall(r"deleted (\S+)", text)
    archived = len(re.findall(r"archived .*→", text))
    m = re.search(r"(\d+) removed, (\d+) kept", text)
    kept = m.group(2) if m else "?"
    stamp = time.strftime("%Y-%m-%d %H:%M")
    gone = ", ".join(removed + [d for d in deleted if d not in removed]) or "nothing"
    if timed_out or rc != 0 or not m:
        why = f"timed out after {timeout}s" if timed_out else (
            f"exit {rc}" if rc != 0 else "no summary line")
        return (f"hw reap {lane} --apply (background, {stamp}) FAILED ({why}): it stopped there "
                f"and removed nothing more; before the stop it had removed: {gone}. Log: {log}")
    return (f"hw reap {lane} --apply (background, {stamp}): removed {gone}"
            f"{f'; archived {archived} worktree output(s)' if archived else ''}; kept {kept}. Log: {log}")


def _start_reap(cmd, lf, env):
    """Start the reap so that the cap can stop ALL of it. Returns (popen, job).

    Where os.killpg exists the reap gets a session of its own, and the group is
    what _stop_reap kills. Native Windows Python has neither os.killpg nor
    SIGKILL, and start_new_session makes no group there: the cap died on
    AttributeError, wrote no FAILED line, and the reap ran on (640 on
    windows.yml 37066966083). There the reap starts suspended inside a Job
    Object, so every process it spawns is in the job whatever its parent chain:
    an msys exec chain leaves a grandchild outside taskkill /T's tree, as
    setup/fast-gate-budget.sh measured. No WinDLL: no job, taskkill only.
    """
    k32 = None
    if not hasattr(os, "killpg"):
        try:
            import ctypes
            k32 = ctypes.WinDLL("kernel32", use_last_error=True)
        except (ImportError, AttributeError, OSError):
            k32 = None
    if k32 is None:
        return subprocess.Popen(cmd, stdout=lf, stderr=subprocess.STDOUT,
                                start_new_session=True, env=env), None
    k32.CreateJobObjectW.restype = ctypes.c_void_p
    k32.AssignProcessToJobObject.argtypes = (ctypes.c_void_p, ctypes.c_void_p)
    k32.TerminateJobObject.argtypes = (ctypes.c_void_p, ctypes.c_uint)
    job = k32.CreateJobObjectW(None, None)
    if not job:
        return subprocess.Popen(cmd, stdout=lf, stderr=subprocess.STDOUT, env=env), None
    p = subprocess.Popen(cmd, stdout=lf, stderr=subprocess.STDOUT, env=env,
                         creationflags=0x4)  # CREATE_SUSPENDED
    ntdll = ctypes.WinDLL("ntdll")
    ntdll.NtResumeProcess.argtypes = (ctypes.c_void_p,)
    if not k32.AssignProcessToJobObject(job, int(p._handle)):
        job = None
    ntdll.NtResumeProcess(int(p._handle))
    return p, ((k32, job) if job else None)


def _stop_reap(p, job):
    """The cap: stop the reap and everything it started, never raise."""
    if hasattr(os, "killpg"):
        try:
            os.killpg(p.pid, signal.SIGTERM)
            p.wait(timeout=10)
        except (OSError, subprocess.TimeoutExpired):
            try:
                os.killpg(p.pid, signal.SIGKILL)
            except OSError:
                pass
        return
    if job:
        k32, handle = job
        k32.TerminateJobObject(handle, 1)
    try:
        subprocess.run(["taskkill", "/T", "/F", "/PID", str(p.pid)], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, timeout=30, check=False)
    except (OSError, subprocess.TimeoutExpired):
        pass
    try:
        p.kill()
        p.wait(timeout=10)
    except (OSError, subprocess.TimeoutExpired):
        pass


def run_reap(lane):
    """The detached worker: run reap under the cap and write the line."""
    state = _state_dir()
    os.makedirs(state, exist_ok=True)
    log = os.path.join(state, f"reap-{lane}.log")
    last = os.path.join(state, f"reap-{lane}.last")
    lock = os.path.join(state, f"reap-{lane}.lock")
    timeout = _timeout()
    try:
        os.mkdir(lock)
    except FileExistsError:
        try:
            if time.time() - os.path.getmtime(lock) < 2 * timeout:
                return 0
            os.rmdir(lock)
            os.mkdir(lock)
        except OSError:
            return 0
    try:
        refresh_unreported(lane)
        timed_out = False
        with open(log, "w", encoding="utf-8") as lf:
            try:
                p, job = _start_reap([_hw(), "reap", lane, "--apply"], lf,
                                     dict(os.environ, NO_COLOR="1"))
            except OSError as e:
                lf.write(f"could not start: {e}\n")
                rc = 127
            else:
                try:
                    rc = p.wait(timeout=timeout)
                except subprocess.TimeoutExpired:
                    timed_out = True
                    _stop_reap(p, job)
                    rc = -1
        with open(log, encoding="utf-8", errors="replace") as lf:
            out = lf.read()
        line = summarize(lane, out, rc, timed_out, timeout, log)
        with open(last + ".tmp", "w", encoding="utf-8") as f:
            f.write(line + "\n")
        os.replace(last + ".tmp", last)
        print(line)
        return 0
    finally:
        try:
            os.rmdir(lock)
        except OSError:
            pass


def start_background_reap(lane):
    """Detach the worker and return at once; the session start never waits."""
    try:
        subprocess.Popen([sys.executable, os.path.realpath(__file__), "--reap", lane],
                         stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                         stderr=subprocess.DEVNULL, start_new_session=True)
        return True
    except OSError:
        return False


def last_reap_line(lane):
    try:
        with open(os.path.join(_state_dir(), f"reap-{lane}.last"), encoding="utf-8") as f:
            return f.read().strip() or None
    except OSError:
        return None


def context(lane, source):
    """The housekeeping half of SessionStart's additionalContext, or None."""
    if lane in ("", "brain") or _is_executor() or os.environ.get("HW_HOUSEKEEPING") == "0":
        return None
    # The payload is read only now, past the gates above: a caller that is not a
    # Claude Code hook may hold stdin open, and reading it would hang.
    if callable(source):
        source = source()
    if source not in ("startup", "resume", ""):
        return None
    parts = []
    u = unreported_context(lane)
    if u:
        parts.append(u)
    prev = last_reap_line(lane)
    started = start_background_reap(lane)
    lines = [f"## Limpieza de la lane ({lane})", ""]
    if prev:
        lines.append(f"Último reap en segundo plano: {prev}")
    lines.append(f"`hw reap {lane} --apply` arrancó en segundo plano (tope {_timeout()}s)."
                 if started else
                 f"`hw reap {lane} --apply` NO pudo arrancar en segundo plano; no se borró nada.")
    parts.append("\n".join(lines))
    return "\n\n".join(parts)


if __name__ == "__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--reap":
        sys.exit(run_reap(sys.argv[2]))
    print("usage: lane_housekeeping.py --reap <lane>", file=sys.stderr)
    sys.exit(2)
