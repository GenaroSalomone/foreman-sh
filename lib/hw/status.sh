# lib/hw/status.sh — `hw status`.
#
# Sourced ONCE by bin/hw (from $HW_BIN_DIR/../lib/hw/status.sh), at the point where
# this code used to sit. It is a library: no shebang, nothing runs on source except
# function definitions. Moved VERBATIM from bin/hw (parent 2d101dd): the
# "── status" block, _status_engram_labels and cmd_status. _status_worktree_roots
# stays in bin/hw because cmd_outbox also calls it.

# ── status ──────────────────────────────────────────────────────────────────
# What is actually running — asked of herdr, not inferred from directories.
#
# THE OLD VERSION GUESSED. "A space with this label exists" was read as running
# and "no space" as never-finished, so the one case that matters — an executor
# whose pane died mid-task — looked exactly like one that had never been closed.
#
# herdr can answer it. `events.subscribe` on `pane.exited` REPLAYS the session's
# event backlog on connect, so a one-shot connection sees every pane
# that has died in this server session. No daemon, no polling, no state file:
# this deliberately replaces the `.hw/pane` file an earlier proposal wanted,
# because herdr's copy of "is that pane alive" cannot go stale and ours could.
#
# TWO LIMITS THAT SHAPE THE OUTPUT. Both are herdr's, both are load-bearing, and
# neither is papered over:
#
#   1. `pane_exited` carries only `pane_id` and `workspace_id`. There is NO exit
#      code, and `pane get` on a dead pane returns null, so there is no
#      post-mortem either. "It died" is knowable; "it failed" is not. Nothing
#      below prints "failed", and the DIED row says why it cannot.
#   2. The backlog is PER SESSION. A task started before the current herdr
#      server came up left no event in it, so silence is not evidence of health.
#      hw therefore reads the server's own start time and, when a task predates
#      it, reports `no information` with that reason rather than a clean row.
#
# Correlation and printing are one python block on purpose. The shape is
# joins-and-fallbacks over four data sources plus a directory walk, which in
# shell is a field of `$(find …)` assignments — and this very function is one of
# the five places where that construct shipped as a silent `set -e` exit. There
# is no such construct here; the guards below are all that talk to herdr.
_herdr_server_started() {
  # macOS has no procfs (Linux has one, but this path serves both) and herdr's own API reports no session start time, so
  # the honest source is the process holding the socket. Best effort: a missing
  # answer downgrades a row to "no information", it never invents one.
  local pid
  [ -n "${HERDR_SOCKET_PATH:-}" ] || return 0
  if command -v lsof >/dev/null 2>&1; then
    pid="$(lsof -t "$HERDR_SOCKET_PATH" 2>/dev/null | head -1 || true)"
  else  # Linux without lsof: fuser (psmisc) names the owner of a socket path
    pid="$(fuser "$HERDR_SOCKET_PATH" 2>/dev/null | awk '{print $1}' || true)"
  fi
  [ -n "$pid" ] || return 0
  ps -o lstart= -p "$pid" 2>/dev/null | head -1 || true
  return 0
}

_status_inputs_source() {
  cat <<'HW_STATUS_PY'
import calendar, glob, json, os, re, time

# The shell decided whether colour is wanted (C_0 is empty when it is not): the
# same NO_COLOR / not-a-terminal rule as every other line hw prints.
if os.environ.get("HW_ST_COLOUR") == "1":
    OK, WARN, ERR, DIM, B, Z = "\033[32m", "\033[33m", "\033[31m", "\033[2m", "\033[1m", "\033[0m"
else:
    OK = WARN = ERR = DIM = B = Z = ""

def blob(name):
    try:
        return json.loads(os.environ.get(name) or "{}")
    except Exception:
        return {}

herdr_ok   = os.environ.get("HW_ST_OK") == "1"
workspaces = (blob("HW_ST_WS").get("result") or {}).get("workspaces") or []
panes      = (blob("HW_ST_PANES").get("result") or {}).get("panes") or []
agents     = (blob("HW_ST_AGENTS").get("result") or {}).get("agents") or []

# pane_exited events, from the replayed backlog. A pane that exited and then had
# its id reused by a later pane would show up here as alive, which is why the
# live-pane set below is subtracted rather than trusted on its own.
exited = {}
for line in (os.environ.get("HW_ST_EXITED") or "").splitlines():
    line = line.strip()
    if not line:
        continue
    try:
        data = json.loads(line).get("data") or {}
    except Exception:
        continue
    if data.get("pane_id"):
        exited[data["pane_id"]] = data.get("workspace_id")

live_panes = {p.get("pane_id") for p in panes}
dead_by_ws = {}
for pane_id, ws in exited.items():
    if pane_id in live_panes or not ws:
        continue
    dead_by_ws.setdefault(ws, []).append(pane_id)

ws_by_label = {w.get("label"): w.get("workspace_id") for w in workspaces if w.get("label")}

# hw's own spaces are labelled `<project>:<task>` with a project hw knows. Other
# labels in the same list are the operator's own long-lived sessions, and
# reading them as tasks put phantom rows in the table.
PROJECTS = set((os.environ.get("HW_ST_PROJECTS") or "").split())

def hw_label(label):
    return label.count(":") == 1 and label.split(":", 1)[0] in PROJECTS

session_start = None
raw = (os.environ.get("HW_ST_SERVER_STARTED") or "").strip()
if raw:
    try:  # ps -o lstart= → "Thu Aug 20 11:34:37 2026", padded on single digits
        session_start = time.mktime(time.strptime(" ".join(raw.split()), "%a %b %d %H:%M:%S %Y"))
    except Exception:
        session_start = None

# ── the task index ────────────────────────────────────────────────────────
# Directories first (they outlive every space), then anything herdr knows about
# that has no directory — a --here executor lives in someone else's workspace
# and has never had one, and used not to appear in `hw status` at all.
tasks = {}

def row(project, task):
    return tasks.setdefault(f"{project}:{task}",
                            {"project": project, "task": task, "workdir": None})

work = os.environ["HW_ST_WORK"]
for d in sorted(glob.glob(os.path.join(work, "*", "*"))):
    if os.path.isdir(d):
        project, task = os.path.basename(os.path.dirname(d)), os.path.basename(d)
        row(project, task)["workdir"] = d
for spec in (os.environ.get("HW_ST_ROOTS") or "").splitlines():
    if ":" not in spec:
        continue
    root, project = spec.rsplit(":", 1)
    for d in sorted(glob.glob(os.path.join(root, "*"))):
        if os.path.isdir(d):
            row(project, os.path.basename(d))["workdir"] = d

# Agent panes, attached to a task by our own metadata tokens when they carry
# them and by the workspace label otherwise. Tokens are the better key — they
# survive a workspace being renamed and they work for --here — but they are
# written by the invokers and carry a TTL, so their ABSENCE means nothing.
agents_by_task = {}
for a in agents:
    tok = a.get("tokens") or {}
    key = None
    if tok.get("hw_project") in PROJECTS and tok.get("hw_task"):
        key = f"{tok['hw_project']}:{tok['hw_task']}"
        row(tok["hw_project"], tok["hw_task"])
    else:
        for label, ws in ws_by_label.items():
            if ws == a.get("workspace_id") and hw_label(label):
                key = label
                row(*label.split(":"))
                break
    if key:
        agents_by_task.setdefault(key, []).append(a)
HW_STATUS_PY
}

_status_liveness_source() {
  cat <<'HW_STATUS_PY'
# ── liveness, by the pane's cwd rather than by our own tokens ──────────────
#
# THIS IS THE LOAD-BEARING PART OF THIS COMMAND. Everything above keys a live
# pane to a task through something WE wrote: metadata tokens (written by the
# invokers, so absent until an executor chooses to call one, and carrying a TTL)
# or a workspace label (which a --here executor does not have, because it lives
# in someone else's workspace). An executor that never called an invoker was
# therefore invisible here, and the branch below then reported it as closed —
# while `herdr pane list` showed the pane alive. Two sources of truth about
# whether an executor is running, and the more accessible one was wrong.
#
# A pane whose cwd IS, or is under, a task's work directory IS that task,
# whether or not it ever called an invoker. That is evidence herdr keeps for us
# and cannot be stale: it is the same `pane list` a human reads. Tokens are now
# an enrichment on top of it, never the basis.
#
# Two details that decide whether the comparison works at all:
#   · the cwd herdr persists and reports is the pane's LIVE cwd, not its launch
#     cwd, so match by PREFIX — a `cd sub/deeper` must still match.
#   · os.path.realpath() resolves symlinks but does NOT normalise case, while
#     os.getcwd() does. Comparing one against the other never matches on a
#     case-insensitive filesystem, which is exactly how the product-lane worktree root
#     stayed dead for a day. canon() casefolds both sides, the same way
#     bin/invoker-common.sh's canon() does — keep them consistent.


def canon(path):
    return os.path.realpath(path).rstrip("/").casefold()


# Longest work directory first, so a task nested inside another's directory
# claims its own panes rather than its parent's.
work_index = sorted(
    ((canon(t["workdir"]), k) for k, t in tasks.items() if t["workdir"]),
    key=lambda pair: len(pair[0]), reverse=True)
task_by_name = {t["task"]: k for k, t in tasks.items()}
agent_by_pane = {a.get("pane_id"): a for a in agents if a.get("pane_id")}

panes_by_task = {}
for p in panes:
    if not p.get("pane_id"):
        continue
    key = None
    # foreground_cwd as well as cwd: herdr reports both, and they differ while a
    # child process is running somewhere else.
    for candidate in (p.get("cwd"), p.get("foreground_cwd")):
        if not candidate:
            continue
        c = canon(candidate)
        for root, k in work_index:
            if c == root or c.startswith(root + "/"):
                key = k
                break
        if key:
            break
    if key is None:
        # The second handle on the same pane, for the one case cwd cannot cover:
        # an executor that cd'd out of its work directory. `_run_here` renames
        # the pane to the task name and `agent start` names the agent the same,
        # and `pane list` DOES report that label (it is `title` that stays null,
        # which is why `hw done` still cannot find a --here pane by name).
        # Ambiguous if two projects ever use the same task name — last one wins
        # in task_by_name. cwd is tried first and is unambiguous, so this only
        # decides for a pane that has left its work directory.
        label = p.get("label")
        if label and label in task_by_name:
            key = task_by_name[label]
    if key:
        panes_by_task.setdefault(key, []).append(p)
HW_STATUS_PY
}

_status_runs_source() {
  cat <<'HW_STATUS_PY'
def run_is_here(rundir):
    """Was this run `--here`, or did it get a space of its own?

    hw MUST know the shape, because a --here executor has no workspace BY
    DESIGN — `_run_here` splits the current pane and creates none — and the
    branch below used to read that absence as "the space is gone", reporting a
    live executor as closed without reporting. That is the bug this answers.

    `_write_run_env` states the shape outright as HW_LAUNCH_MODE=here|tab|space.
    It used to be INFERRED from the presence of HW_INVOKER_PANE, which was
    --here only — and that inference died the moment space mode started
    carrying the brainer pane too, so that the return channel would work in
    the default mode at all. An inferred discriminator that silently reads
    every space run as --here is worse than no discriminator, because the
    branch below then claims the opposite of the truth.

    No new file: this still lives in the env file that already exists, and it
    is a launch-time fact about a run that already happened, so it cannot go
    stale the way a copy of live state does (setup/decisions.md).

    True / False, or None when there is no readable env file at all (a run
    whose directory could not be written) — None means "do not claim".
    """
    try:
        with open(os.path.join(rundir, "env")) as fh:
            body = fh.read()
    except OSError:
        return None
    for line in body.split("\n"):
        if line.startswith("HW_LAUNCH_MODE="):
            return "here" in line
    # Runs written before HW_LAUNCH_MODE existed: back then HW_INVOKER_PANE was
    # --here only, so on those files the old inference is still exactly right —
    # including its False, which correctly reads a legacy space run.
    return any(line.startswith("HW_INVOKER_PANE=") for line in body.split("\n"))


def run_owns(rundir, kind):
    """The tab or workspace this run CREATED, from the ownership record `hw done`
    already relies on to decide what it is allowed to close. Placement is read
    from this and never from "it has no dedicated workspace" — a task tab has no
    workspace of its own BY DESIGN, and inferring `--here` from that absence is
    how a three-pane tab in workspace `<lane>` came to be printed as a split
    pane. It also works for runs written before HW_LAUNCH_MODE grew a `tab`
    value, which is every Grid Compact run."""
    try:
        with open(os.path.join(rundir, kind)) as fh:
            return fh.read().strip() or None
    except OSError:
        return None


def _placement_gone(here_run, own_tab, own_ws):
    """What closed, named from what the run owned. `space closed` was printed for
    every non---here task, including the tabs, which is the same false
    attribution one level down."""
    if own_tab:
        return f"tab {own_tab} closed"
    if own_ws:
        return f"workspace {own_ws} closed"
    if here_run:
        return "pane gone (--here)"
    return "its placement record is gone, so what closed is not recorded"


def run_mode_of(rundir):
    """`here`, `tab`, `space`, or None when the env file cannot be read or
    predates the field. Separate from run_is_here() because that answers a
    yes/no and the interesting distinction is now three-way.

    Reads through _env_field, which is defined further down beside
    invoker_pane_of instead of here. That placement is not taste: the test
    harness extracts these functions with `awk` ranges anchored on `^def <name>(`
    lines, and setup/tests/16-status.sh deliberately stops `run_owns ..
    run_mode_of` short of this function while `task_seq .. runs_of` carries
    invoker_pane_of. A helper both callers need has to land inside that second
    window or 16-status.sh dies with a NameError, so the forward reference is
    the price of not rewriting anchors that already work."""
    return _env_field(rundir, "HW_LAUNCH_MODE")


def task_seq(rundir):
    """Which task this executor is on. See invoker-common.sh: a run is an
    EXECUTOR, and the brainer re-tasks a living one with `hw next`, so the run
    directory holds task 1 and t2/, t3/ … hold the ones after it."""
    try:
        with open(os.path.join(rundir, "task")) as fh:
            n = int("".join(c for c in fh.read() if c.isdigit()) or 1)
    except (OSError, ValueError):
        return 1
    return n if n > 0 else 1


def task_state_dir(rundir):
    n = task_seq(rundir)
    return rundir if n == 1 else os.path.join(rundir, "t%d" % n)


def task_reopened(rundir):
    """`hw revive` brought this run back after it reported: `reopened` sits
    beside `done`, and until the revived executor reports again (which clears
    it) the task is OPEN, whatever the older marker says."""
    d = task_state_dir(rundir)
    return os.path.isfile(os.path.join(d, "done")) and os.path.isfile(os.path.join(d, "reopened"))


def task_done(rundir):
    """Did the CURRENT task report? Task 3 being open is not task 2's marker.
    A reopened run is not done: its report predates the pane hw revive gave it."""
    d = task_state_dir(rundir)
    return os.path.isfile(os.path.join(d, "done")) and not os.path.isfile(os.path.join(d, "reopened"))


def closed_by_hand(rundir):
    """Was the CURRENT task closed with `hw done` before it reported? A dict
    {at, by, reported, legacy} or None.

    `hw done` writes `closed-by-hand` in the task's state dir. Runs closed
    before it existed have no such file; the one measured trace they carry is a
    `verify_run` line in receipt.jsonl, which only `hw done` writes, dated after
    this task was dispatched (the `task` file `hw next` wrote; task 1 has no
    lower bound). A hand close that left no verify_run (no pinned command)
    stays unclassified: absence of the trace proves nothing."""
    p = os.path.join(task_state_dir(rundir), "closed-by-hand")
    try:
        with open(p) as fh:
            kv = dict(l.strip().split("=", 1) for l in fh if "=" in l)
        return {"at": int(kv.get("at") or 0) or None, "by": kv.get("by") or "none",
                "reported": kv.get("reported") == "yes", "legacy": False}
    except (OSError, ValueError):
        pass
    try:
        floor = os.path.getmtime(os.path.join(rundir, "task")) if task_seq(rundir) > 1 else 0
        at = None
        with open(os.path.join(rundir, "receipt.jsonl")) as fh:
            for line in fh:
                try:
                    r = json.loads(line)
                    if r.get("key") != "verify_run":
                        continue
                    t = calendar.timegm(time.strptime(r.get("at") or "", "%Y-%m-%dT%H:%M:%SZ"))
                except Exception:
                    continue
                if t >= floor:  # MUTATION-ANCHOR: 700-M05
                    at = t
        if at is not None:
            return {"at": at, "by": "unknown", "reported": False, "legacy": True}
    except OSError:
        pass
    return None


def prior_reported(rundir):
    """The last task BEFORE the current one whose done marker exists, or None.
    Only meaningful for a chain (`hw next` made the current task N > 1)."""
    for k in range(task_seq(rundir) - 1, 0, -1):
        d = rundir if k == 1 else os.path.join(rundir, "t%d" % k)
        if os.path.isfile(os.path.join(d, "done")):
            return k
    return None


def blocked_since(rundir):
    """When the CURRENT task's --blocked report landed, if its executor is still
    kept waiting for a ruling (`blocked-waiting`, written by done-invoker);
    None otherwise. A marker without a readable `since` uses its mtime."""
    p = os.path.join(task_state_dir(rundir), "blocked-waiting")
    if not os.path.isfile(p):
        return None
    try:
        with open(p) as fh:
            for line in fh:
                if line.startswith("since="):
                    return int(line.split("=", 1)[1].strip())
    except (OSError, ValueError):
        pass
    try:
        return int(os.path.getmtime(p))
    except OSError:
        return None


def blocked_wait_hours():
    h = os.environ.get("HW_BLOCKED_WAIT_HOURS", "24")
    return int(h) if h.isdigit() else 24


def task_resumed(rundir):
    """`hw ruling` resumed this task after a blocked report, and it has not
    reported since: open again, whatever the pane's old done tokens say."""
    d = task_state_dir(rundir)
    return os.path.isfile(os.path.join(d, "blocked-resumed")) and not task_done(rundir)


def turns_of(rundir):
    """How many of this run's TURNS the harness watched end. Written by
    bin/hw-stop-hook.sh (claude, codex) and the opencode `hw-turn-ended` plugin at
    the end of every turn, before the agent chooses anything — so unlike the
    metadata tokens it does not depend on the executor cooperating, and unlike
    them it is a FILE, so it outlives the pane. Measured 2026-08-26: closing a
    workspace makes `pane.get` return pane_not_found and drops the agent from
    `agent list`, taking every token with it. This number is what is left.

    0 means NOTHING, and the orphan section below is built on that: either the
    run predates its vendor's adapter (claude 2026-08-20, opencode 08-22, codex
    08-23), or the executor died before finishing a turn, or nobody wired one.
    Absence of the file is never evidence that no turn ended."""
    try:
        with open(os.path.join(rundir, "turns")) as fh:
            n = int("".join(c for c in fh.read() if c.isdigit()) or 0)
    except (OSError, ValueError):
        return 0
    return n if n > 0 else 0


def turns_at(rundir):
    """When the last turn ended, as the mtime of the counter the hook rewrites
    each time. The token `turn_ended_at` carries the same fact and dies with the
    pane; this is the copy that survives it."""
    try:
        return os.path.getmtime(os.path.join(rundir, "turns"))
    except OSError:
        return None


def has_env(rundir):
    """Whether hw wrote a run env file at all. A run with none (hand-made probe,
    or a launch that died before _write_run_env) cannot be asked whether it had a
    return channel, and must not be reported as if it could."""
    return os.path.isfile(os.path.join(rundir, "env"))


def _env_field(rundir, key):
    """One field of a run env file, or None. Read by bin/runenv, the one reader
    of that file (hw, hw-reconcile and the invokers all ask it), `--bare` so a
    hand-made run with unquoted values reads as before.

    THE SINGLE READER FOR BOTH FIELDS: run_mode_of and invoker_pane_of both come
    through here, so one mutant on this function kills both callers.

    An empty value reads as None rather than "": to every caller here, "the key
    is absent" and "the key is empty" are the same fact. A file runenv cannot
    read is named once on stderr and also reads as None, never as a run that
    sets nothing without a word."""
    import importlib.machinery, importlib.util, sys
    mod = _env_field.__dict__.get("runenv")
    if mod is None:
        sys.dont_write_bytecode = True
        loader = importlib.machinery.SourceFileLoader("runenv", os.environ["HW_ST_RUNENV"])
        mod = importlib.util.module_from_spec(importlib.util.spec_from_loader("runenv", loader))
        loader.exec_module(mod)
        _env_field.runenv = mod
    try:
        values = mod.read(rundir, lenient=True, bare=True)
    except mod.Unusable as e:
        sys.stderr.write("hw status: %s\n" % e)
        return None
    return (values or {}).get(key) or None


def invoker_pane_of(rundir):
    """The brainer pane this run was told to report to, from the env file hw
    wrote at dispatch. This is the whole reason the orphan section can say WHY a
    task never reported instead of only that it did not.

    None means the key is absent, and that is a HARD fact about the run, not a
    gap: `done-invoker` refuses at its `no HW_INVOKER_PANE` gate without it — before it
    publishes anything. Measured 2026-08-26 against a real herdr pane: the gate
    refusal leaves ZERO tokens, while a DELIVERY refusal (dead brainer pane,
    timeout) publishes done_status/done_state=undelivered/sum first. Two
    refusals, opposite evidence; only the second is readable off the pane, and
    only while that pane is alive."""
    return _env_field(rundir, "HW_INVOKER_PANE")


def receipt_pane(rundir):
    """The executor's OWN pane, as MEASURED at launch and recorded in
    receipt.jsonl — not the pane hw meant to use. If it is still open, the report
    a refused done-invoker printed into it is still there to read."""
    pane = None
    try:
        with open(os.path.join(rundir, "receipt.jsonl")) as fh:
            for line in fh:
                line = line.strip()
                if not line:
                    continue
                try:
                    rec = json.loads(line)
                except ValueError:
                    continue
                if rec.get("key") == "pane" and rec.get("value"):
                    pane = rec["value"]
    except OSError:
        return None
    return pane


def receipt_value(rundir, key):
    """The last non-empty value receipt.jsonl recorded for `key`, or None."""
    found = None
    try:
        with open(os.path.join(rundir, "receipt.jsonl")) as fh:
            for line in fh:
                try:
                    rec = json.loads(line)
                except ValueError:
                    continue
                if rec.get("key") == key and rec.get("value"):
                    found = rec["value"]
    except OSError:
        return None
    return found

def chaining_lease_of(rundir):
    """Bounded retention record. Shape only; the detached action is authoritative."""
    path = os.path.join(rundir, "chaining-lease")
    vals = {}
    try:
        with open(path) as fh:
            for line in fh:
                if "=" in line:
                    k, v = line.rstrip("\n").split("=", 1); vals[k] = v
    except OSError:
        return None
    try: until = int(vals.get("until", ""))
    except ValueError: until = 0
    vals["live"] = vals.get("state") == "live" and time.time() < until
    vals["expired"] = vals.get("state") == "live" and time.time() >= until or vals.get("state") == "expired-cleanup"
    return vals


def runs_of(workdir):
    """Every .hw run directory, newest first, with its start time and markers."""
    out = []
    if not workdir:
        return out
    hw = os.path.join(workdir, ".hw")
    # `.hw/done` with no run directory is the pre-HW_RUN layout; still read it.
    if os.path.isfile(os.path.join(hw, "done")):
        out.append({"name": "(legacy)", "done": True, "task": 1,
                    "started": None, "here": None, "mode": None,
                    "own_tab": None, "own_ws": None})
    for d in sorted(glob.glob(os.path.join(hw, "*")), reverse=True):
        if not os.path.isdir(d):
            continue
        name = os.path.basename(d)
        started = None
        m = re.match(r"^(\d{8}-\d{6})", name)
        if m:
            try:
                started = time.mktime(time.strptime(m.group(1), "%Y%m%d-%H%M%S"))
            except Exception:
                started = None
        if started is None:
            try:
                started = os.path.getmtime(d)
            except OSError:
                started = None
        out.append({"name": name, "dir": d, "done": task_done(d), "task": task_seq(d),
                    "reopened": task_reopened(d),
                    "closed": closed_by_hand(d), "prior_reported": prior_reported(d),
                    "blocked_since": blocked_since(d), "resumed": task_resumed(d),
                    "started": started, "here": run_is_here(d),
                    "mode": run_mode_of(d),
                    "turns": turns_of(d), "turns_at": turns_at(d),
                    "invoker": invoker_pane_of(d), "no_report": _env_field(d, "HW_NO_REPORT") == "1", "has_env": has_env(d),
                    "exec_pane": receipt_pane(d),
                    "model_running": receipt_value(d, "model_running"),
                    "model_verdict": receipt_value(d, "model_verdict"),
                    "chaining_lease": chaining_lease_of(d),
                    "own_tab": run_owns(d, "tab"),
                    "own_ws": run_owns(d, "workspace")})
    return out


def pending_rulings_of(rundir):
    """(count, oldest mtime) of the rulings queued on a run and not yet
    delivered: `pending-ruling` plus `pending-ruling.<n>`, the same files
    ruling_queue_pending (state-witness.sh) reads. (0, None) when none."""
    if not rundir:
        return 0, None
    files = []
    for f in glob.glob(os.path.join(rundir, "pending-ruling*")):
        n = os.path.basename(f)[len("pending-ruling"):]
        if (n == "" or re.fullmatch(r"\.\d+", n)) and os.path.isfile(f):
            files.append(f)
    ages = []
    for f in files:
        try:
            ages.append(os.path.getmtime(f))
        except OSError:
            pass
    return len(ages), (min(ages) if ages else None)


def pane_ctx_parse(text):
    """(tokens, percent|None) from the last statusline row of a pane capture
    (`Model: … | Ctx: <n> | …`), or None when there is none. The row is found by
    `Ctx:`, NOT by `Ctx Used:`: a narrow pane cuts the footer to
    `Ctx: 237.5k | Ctx Use...`, so the percent is optional (measured on live
    executors). Non-breaking spaces normalised; ONE physical line; no row is
    None, never zero."""
    rows = [l for l in (text or "").replace("\u00a0", " ").splitlines() if re.search(r"Ctx: *[0-9]+(\.[0-9]+)?[kKmM]? *\|", l)]  # MUTATION-ANCHOR: 690-M03
    if not rows:
        return None
    m = re.search(r"Ctx: *([0-9.]+)([kKmM]?)", rows[-1])
    u = re.search(r"Ctx Used: *([0-9.]+)", rows[-1])
    if not m:
        return None
    mult = {"k": 1000, "m": 1000000}.get(m.group(2).lower(), 1)
    try:
        return int(float(m.group(1)) * mult), (u.group(1) if u else None)
    except ValueError:   # `Ctx: 1.2.3`: not a measurement, never a crash
        return None


def pane_ctx_read(pane, timeout):
    """One bounded `herdr agent read`: the parsed context, or None for
    unreadable / no statusline / timed out."""
    import subprocess
    try:
        r = subprocess.run(["herdr", "agent", "read", pane, "--source", "visible"],
                           capture_output=True, text=True, errors="replace", timeout=timeout)  # MUTATION-ANCHOR: 690-M04
    except (OSError, subprocess.SubprocessError):
        return None
    return pane_ctx_parse(r.stdout) if r.returncode == 0 else None


def pane_ctx_many(panes, timeout=None):
    """{pane: (tokens, pct) | None}, all panes read in parallel so the whole
    status costs one timeout at worst, not one per executor."""
    panes = sorted({p for p in panes if p})
    if not panes:
        return {}
    if timeout is None:
        try:
            timeout = float(os.environ.get("HW_STATUS_CTX_TIMEOUT") or 2)
        except ValueError:
            timeout = 2.0
    from concurrent.futures import ThreadPoolExecutor
    with ThreadPoolExecutor(max_workers=min(8, len(panes))) as ex:
        return dict(zip(panes, ex.map(lambda p: pane_ctx_read(p, timeout), panes)))
HW_STATUS_PY
}

_status_recovery_source() {
  cat <<'HW_STATUS_PY'
# ── the queue that had no name: finished, and never reported ───────────────
#
# WHAT WAS MISSING. `hw status` had a word for an executor that reported
# (`reported-done`), for one that reported and could not deliver
# (`done-NOT-delivered`), for one sitting idle with its turn ended
# (`idle, silent`) and for one whose space is gone (`no information`). It had no
# word for the one that FINISHED, could not report, and whose agent then went
# away — and that is not a rare shape. Three executors hit it on 2026-08-26
# alone, and a sweep of all 83 work directories found nine.
#
# WHY THE EXISTING ROWS GET IT BACKWARDS, which is worse than silence. herdr
# drops the AGENT record when the agent process exits, but the PANE survives —
# the tab is still open holding a bare shell. All three of 2026-08-26 were in
# exactly that state, and `hw status` printed them `process-live`, "pane X live,
# no agent detected in it". Healthy-looking, and the opposite of true.
#
# So the discriminator is the AGENT, not the pane, and the evidence is on DISK,
# because every token died with the agent record (measured, see
# invoker_pane_of). Three signals, and all three have to agree:
#
#   1. no `done` marker for the run's CURRENT task seq — it never reported
#   2. `turns` >= 1 — the harness watched at least one turn of this run END
#   3. no live agent in the work directory — it is not running now
#
# THE `turns` GATE IS WHAT STOPS THIS CRYING WOLF, and it is deliberately a
# positive test. A work directory with no done marker is NOT an orphan: it may
# be running (1 and 3 exclude it), it may have been abandoned before it ever
# finished a turn, or it may predate the turn adapter entirely. Those come back
# None here and keep whatever the table already said. Verified over all 83
# directories: 9 orphans, 4 running, 7 unclassifiable, and ZERO tasks that had
# reported.
def orphan_of(newest, has_agent):
    """Did this task finish and never reach a brainer? A dict, or None.

    None is the honest answer for everything the three signals cannot settle,
    and it is the common one. `why` is the part a human acts on: it says whether
    reporting was even POSSIBLE, which is a fact about the dispatch rather than
    about the executor's diligence."""
    if newest is None or has_agent:
        return None
    if newest.get("done"):
        return None
    if newest.get("turns", 0) < 1:
        return None
    return {"run": newest.get("name"), "turns": newest["turns"],
            "at": newest.get("turns_at"), "invoker": newest.get("invoker"),
            "no_report": newest.get("no_report"),
            "has_env": newest.get("has_env"), "exec_pane": newest.get("exec_pane")}


def unreported_row(newest):
    """The state and notes for a task with no report and no agent, when it is
    one `hw done` closed by hand (A) or a chained task whose earlier tasks DID
    report (B). (state, notes) or None when neither fact is on disk.

    Closed by hand is a different fact from a death, and `finished, unreported`
    said the same thing for both."""
    if not newest:
        return None
    cl, prior, n = newest.get("closed"), newest.get("prior_reported"), newest.get("task", 1)
    notes = []
    if prior and n > 1:  # MUTATION-ANCHOR: 700-M04
        notes.append("task %d reported, task %d %s before reporting"
                     % (prior, n, "closed" if cl else "ended"))
    if not cl:
        return None if not notes else ("finished, unreported", notes)
    when = time.strftime("%d %b %H:%M", time.localtime(cl["at"])) if cl.get("at") else "an unrecorded time"
    if cl.get("legacy"):
        notes.append("closed by hand: an `hw done` verification (receipt) at %s, and no report — "
                     "read from the receipt, this run predates the closed-by-hand mark" % when)
    else:
        notes.append("closed by hand with `hw done` (%s) at %s, %s"
                     % (cl["by"] if cl["by"] != "none" else "no flag", when,
                        "after a report existed" if cl["reported"] else "no report existed"))
    notes.append("not a death: the task was closed on purpose without done-invoker — its work and engram findings may still be there")
    return ("closed-unreported", notes)  # MUTATION-ANCHOR: 700-M02


def orphan_why(orph, live_pane_ids):
    """Why the report never landed, from the run's own dispatch record.

    Three answers, and they are not the same problem. The first is hw's or the
    brainer's, not the executor's: without HW_INVOKER_PANE `done-invoker` dies at
    its gate, so a perfectly diligent executor is refused and nothing is
    published. Seven of the nine found on 2026-08-26 were this."""
    if not orph["has_env"]:
        return ("no run env file, so hw cannot say whether it had a return "
                "channel at all")
    if orph.get("no_report"):
        return ("this dispatch deliberately used --no-report (fire-and-forget), so no "
                "brainer report was requested")
    if not orph["invoker"]:
        return ("no HW_INVOKER_PANE in its run env — done-invoker refuses at the "
                "gate (`no HW_INVOKER_PANE` in bin/done-invoker) and publishes nothing, so this was "
                "never the executor's to fix")
    if orph["invoker"] not in live_pane_ids:
        return ("its brainer's pane %s is gone, so done-invoker had nowhere to "
                "deliver" % orph["invoker"])
    return ("its brainer %s is still live, so nothing stopped it — done-invoker "
            "was simply never called" % orph["invoker"])


def cleanup_backlog_of(newest, has_agent, live_panes, own_tab, own_ws):
    """A reported task whose owned container still exists, or None.

    The current task must have a durable done marker, its agent must be gone,
    and an ownership record must name the tab or workspace still holding the
    shell. Those gates exclude every finished-unreported recovery pane by
    construction."""
    if newest is None or not newest.get("done") or has_agent or not live_panes:
        return None
    lease = newest.get("chaining_lease")
    if lease and lease.get("live"):
        return None
    if own_tab:
        return {"kind": "tab", "id": own_tab,
                "panes": [p.get("pane_id") for p in live_panes if p.get("pane_id")]}
    if own_ws:
        return {"kind": "workspace", "id": own_ws,
                "panes": [p.get("pane_id") for p in live_panes if p.get("pane_id")]}
    return None


SKIP_DIRS = {".hw", ".git", "node_modules", ".venv", ".next", "dist", "__pycache__"}

def shape_of(workdir):
    """One phrase describing what is on disk, and it is not the same phrase for
    both kinds of task. A repo-less work directory holds the task's OUTPUT, so
    the file count is the interesting number. A worktree holds a checkout, and
    counting it said `7191 artifacts` for a task that had produced none —
    a number that is both wrong and slow to compute."""
    if not workdir:
        return "no work directory"
    if os.path.exists(os.path.join(workdir, ".git")):
        return "worktree"
    n = 0
    for base, dirs, files in os.walk(workdir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        n += len(files)
    return f"{n} artifact" + ("" if n == 1 else "s")


# ── a task that is `working` and has shown no sign of life ───────────────────
#
# herdr's `working` goes stale on this machine (state-witness.sh: one pane said
# `working` eight hours after it reported), so an executor that hangs, or that
# is waiting on something nobody will answer, read `working` forever. There is
# no daemon and no history to ask, so STALE is computed at read time from what
# the disk and the process table already hold: the last turn end, the newest
# mtime in the run's own state, the newest file in the work directory, and the
# newest process started with its cwd in it. If none is newer than the
# threshold, the row says so. It is a place to LOOK, not a verdict: what it
# cannot see (pane output; one child that has simply been running a long time)
# is named in the caveats.
def stale_minutes():
    try:
        m = float(os.environ.get("HW_STALE_MINUTES") or 30)
    except ValueError:
        return 30.0
    return m if m > 0 else 30.0


def status_now():
    try:
        return float(os.environ.get("HW_ST_NOW") or time.time())
    except ValueError:
        return time.time()


def run_state_activity(rundir):
    """Newest mtime among the run's own state files, and its t<N>/ directories'.
    Asks, rulings, transcript lines and turn counters all land here."""
    newest = None
    dirs = [rundir] + sorted(glob.glob(os.path.join(rundir, "t[0-9]*")))
    for d in dirs:
        try:
            names = os.listdir(d)
        except OSError:
            continue
        for name in names:
            try:
                m = os.lstat(os.path.join(d, name)).st_mtime
            except OSError:
                continue
            if newest is None or m > newest:
                newest = m
    return newest


def tree_activity(workdir, cutoff):
    """Newest file mtime under the work directory (SKIP_DIRS pruned), returning
    as soon as one is newer than `cutoff`: the question is only whether anything
    moved inside the window, so an active tree costs a handful of stats."""
    newest = None
    for base, dirs, files in os.walk(workdir):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for f in files:
            try:
                m = os.lstat(os.path.join(base, f)).st_mtime
            except OSError:
                continue
            if newest is None or m > newest:
                newest = m
                if newest >= cutoff:
                    return newest
    return newest


def process_activity(workdir, now):
    """When the newest process with its cwd under `workdir` was STARTED, or None.
    The agent and its MCP servers live there from the first minute, so only a
    START inside the window says anything: a test run, a build, a shell the
    executor opened. None also when ps/lsof cannot be asked."""
    import subprocess
    root = canon(workdir)
    try:
        out = subprocess.run(["lsof", "-a", "-d", "cwd", "-Fpn"], capture_output=True,
                             text=True, timeout=10).stdout
        ps = subprocess.run(["ps", "-axo", "pid=,etime="], capture_output=True,
                            text=True, timeout=10).stdout
    except Exception:
        return None
    pids, pid = set(), None
    for line in out.splitlines():
        if line.startswith("p"):
            pid = line[1:]
        elif line.startswith("n") and pid:
            c = canon(line[1:])
            if c == root or c.startswith(root + "/"):
                pids.add(pid)
    newest = None
    for line in ps.splitlines():
        parts = line.split()
        if len(parts) != 2 or parts[0] not in pids:
            continue
        et, days = parts[1], 0
        if "-" in et:
            d, et = et.split("-", 1)
            days = int(d)
        try:
            sec = [int(x) for x in et.split(":")]
        except ValueError:
            continue
        while len(sec) < 3:
            sec.insert(0, 0)
        started = now - (days * 86400 + sec[0] * 3600 + sec[1] * 60 + sec[2])
        if newest is None or started > newest:
            newest = started
    return newest


def stale_of(newest, workdir, now, threshold_min):
    """None when the task is not stale or hw cannot tell; otherwise the quiet
    time in seconds and the last sign of life it found (when, what).

    Cheap signals first and the dearer ones only while every earlier one is
    still older than the window. No signal at all is None: hw does not claim
    what it cannot place in time."""
    if not newest or not newest.get("dir"):
        return None
    cutoff = now - threshold_min * 60
    signs = []

    def seen(ts, what):
        if ts is not None:
            signs.append((ts, what))
        return ts is not None and ts >= cutoff

    if seen(newest.get("started"), "its launch") \
       or seen(newest.get("turns_at"), "its last turn end") \
       or seen(run_state_activity(newest["dir"]), "its run state (asks, rulings, transcript)"):
        return None
    if workdir and os.path.isdir(workdir):
        if seen(tree_activity(workdir, cutoff), "a file in its work directory"):
            return None
        if seen(process_activity(workdir, now), "a process started in its work directory"):
            return None
    if not signs:
        return None
    ts, what = max(signs)
    return {"quiet": now - ts, "at": ts, "what": what}
HW_STATUS_PY
}

_status_report_spaces_source() {
  cat <<'HW_STATUS_PY'
def status_report_spaces(workspaces, herdr_ok):
    print(f"\n{B}spaces{Z}")
    if not workspaces:
        print(f"  {DIM}·{Z} none" if herdr_ok else f"  {DIM}·{Z} unknown — herdr unreachable")
    for w in workspaces:
        label = w.get("label") or "-"
        drift = (f"  {WARN}DRIFT{Z} — legacy per-task workspace; expected a tab "
                 "in the project's bare-label space") if ":" in label else ""
        print(f"  {w.get('workspace_id')}  {label}{drift}")
HW_STATUS_PY
}

_status_report_tasks_source() {
  cat <<'HW_STATUS_PY'
def status_report_tasks(tasks, ws_by_label, agents_by_task, panes_by_task, agent_by_pane, dead_by_ws, session_start, herdr_ok):
    print(f"\n{B}tasks{Z}")
    if not tasks:
        print(f"  {DIM}·{Z} no work directories, no worktrees, no task panes")

    limits = set()
    STATUS_PROJECT = os.environ.get("HW_STATUS_PROJECT") or ""
    STATUS_ALL = os.environ.get("HW_STATUS_ALL") == "1"
    _suppressed = 0
    orphans = []
    cleanup_backlog = []
    # CONTEXT SIZE of each live executor (a 400k pane was once resumed unseen),
    # read in parallel and bounded, once, before the rows print.
    _ctx_panes = []
    for _k in tasks:
        if STATUS_PROJECT and not _k.startswith(STATUS_PROJECT + ":"):
            continue
        # The same agent the row will name: agents_by_task first, then the
        # agent of a live pane in the task's directory.
        _ags = list(agents_by_task.get(_k) or [])
        for _p in panes_by_task.get(_k) or []:
            _a = agent_by_pane.get(_p.get("pane_id"))
            if _a and _a.get("pane_id") not in {x.get("pane_id") for x in _ags}:
                _ags.append(_a)
        if _ags and _ags[0].get("pane_id"):
            _ctx_panes.append(_ags[0]["pane_id"])
    ctx_by_pane = pane_ctx_many(_ctx_panes) if herdr_ok else {}
    try:
        CTX_FRESH = int(os.environ.get("HW_CTX_PREFER_FRESH_TOKENS") or 150000)
    except ValueError:
        CTX_FRESH = 150000
    for key in sorted(tasks):
        if STATUS_PROJECT and not key.startswith(STATUS_PROJECT + ":"):
            continue
        t = tasks[key]
        ws = ws_by_label.get(key)
        # list(), not the stored list: appending below would otherwise mutate
        # agents_by_task and make a second read of this task see different evidence.
        mine = list(agents_by_task.get(key) or [])
        live_panes = panes_by_task.get(key) or []
        seen_panes = {a.get("pane_id") for a in mine}
        for p in live_panes:
            a = agent_by_pane.get(p.get("pane_id"))
            if a and a.get("pane_id") not in seen_panes:
                mine.append(a)
                seen_panes.add(a.get("pane_id"))
        runs = runs_of(t["workdir"])
        newest = runs[0] if runs else None
        here_run = newest["here"] if newest else None
        run_mode = newest.get("mode") if newest else None
        own_tab = newest.get("own_tab") if newest else None
        own_ws = newest.get("own_ws") if newest else None
        dead = dead_by_ws.get(ws) or [] if ws else []

        tok = (mine[0].get("tokens") or {}) if mine else {}
        # Read every token defensively. A task can predate the invoker that writes
        # them, and every token carries a TTL — so a missing `done` is "no
        # information", never "not done".
        # `done_status` is the flag done-invoker actually writes, and its value is
        # `done` or `blocked` — a blocked executor stopped, it did not finish, so it
        # must not be read as done. `hw_done`/`done` are accepted too in case a
        # future writer picks the shorter name.
        done_status = tok.get("done_status")
        # `done_state` is the OTHER half of the fact and status never read it.
        # done-invoker publishes the tokens BEFORE it tries to deliver
        # (`publish_done undelivered`) and rewrites them to `delivered` only after the
        # receiver proves it got the message; the `.hw/<run>/done` file is written at
        # that same later point. So the tokens answer "the executor said it finished"
        # and the file answers "the brainer was actually told" — two different facts
        # under one word. A product-lane brainer hit the gap on 2026-08-24:
        # `hw status` said reported-blocked while `hw next` said "has not reported",
        # and both were right. Its executor had called done-invoker and the delivery
        # timed out against a mid-turn brainer at the 90s default. Naming the two
        # states apart costs nothing; unifying them would be a design change.
        done_state = tok.get("done_state")
        done_tok = (done_status == "done") or bool(tok.get("hw_done") or tok.get("done"))
        blocked_tok = done_status == "blocked"
        blocked_now = blocked_tok or bool(newest and newest.get("blocked_since"))
        done_disk = any(r["done"] for r in runs)
        # The NEWEST run's marker only. `hw done` keeps a repo-less work directory
        # and the next run reuses it, so "any run ever reported" must not be allowed
        # to speak for the run that is live right now — a fresh executor would read
        # as already finished. done_disk stays as-is for the closed-space branches,
        # where the question is "did this task ever report", not "did this run".
        done_newest = bool(newest and newest["done"])
        # Written by the agent's turn-end adapter, so it fires whether or not the
        # agent remembered anything. It claims exactly one thing:
        # this executor's turn ended at that time and it has still not reported. It
        # is NOT a completion — a Stop hook cannot tell "the task is finished" from
        # "this turn is finished" — so it never outranks a real done/blocked report.
        turn_ended = tok.get("turn_state") == "ended_unreported"
        turn_at = tok.get("turn_ended_at") or "?"
        turns = tok.get("turns") or "?"
        # `bool(mine)` — the AGENT, never the pane. A finished executor leaves its
        # tab open with a bare shell in it, and reading that as alive is precisely
        # how three of these came to be printed `process-live`.
        orph = orphan_of(newest, bool(mine))
        if orph:
            orph["key"] = key
            orph["workdir"] = t["workdir"]
            orph["live_panes"] = [p.get("pane_id") for p in live_panes]
            orphans.append(orph)
        cleanup = cleanup_backlog_of(newest, bool(mine), live_panes, own_tab, own_ws)
        if cleanup:
            cleanup["key"] = key
            cleanup_backlog.append(cleanup)

        notes = []
        chain = newest.get("chaining_lease") if newest else None
        if mine or live_panes:
            a = mine[0] if mine else None
            mark, state, tint = "●", "process-live", OK
            if a:
                agent_status = a.get("agent_status") or "unknown"
                if agent_status in ("working", "retry"):
                    state = "working"
                elif agent_status in ("idle", "done"):
                    state = "idle"
                elif agent_status == "blocked":
                    state = "blocked"
                detail = f"{a.get('agent') or 'agent'} {agent_status} in {a.get('pane_id')}"
            else:
                # Alive on herdr's own evidence, with nothing herdr classifies as an
                # agent in it: a bare shell, or an agent that exited and left the
                # shell behind. Still running as far as the process tree goes.
                detail = "pane " + ", ".join(
                    p.get("pane_id") for p in live_panes) + " live, no agent detected in it"
            if not ws:
                # NAME WHAT IT ACTUALLY IS. Both branches used to say `--here`, so a
                # run that was not --here was still labelled --here — the else was
                # reached by every TAB task, which has a tab rather than a workspace
                # of its own and therefore trips `not ws` by design. Reported by the
                # product-lane orchestrator as "hw status labels some tabs as --here"; it
                # was labelling all of them.
                if own_tab:
                    detail += f" (tab {own_tab} in the project's space — a tab has no workspace of its own, by design)"
                elif here_run:
                    detail += " (--here: no space of its own, by design)"
                elif run_mode == "tab":
                    detail += " (a tab in the project's space, not a workspace of its own)"
                else:
                    detail += " (no workspace of its own, and no --here record either)"
            notes.append(detail)
            # THE MODEL THAT RUNS, as the receipt measured it from the session
            # (not the alias hw was asked for). A divergence is not decoration.
            if newest and newest.get("model_running"):
                mv = newest.get("model_verdict") or ""
                if mv.startswith("DIVERGENCE"):
                    notes.append(f"model {newest['model_running']} — {mv}")
                    limits.add("model")
                else:
                    notes.append(f"model {newest['model_running']} (measured)")
            if dead:
                notes.append(f"another pane in {ws} exited: {', '.join(dead)}")
            if tok.get("ask_seq"):
                notes.append(f"ask {tok['ask_seq']} {tok.get('ask_state') or ''}".strip())
            if a and a.get("pane_id") in ctx_by_pane:
                _c = ctx_by_pane[a["pane_id"]]
                if _c is None:
                    notes.append("context: not measured (unreadable, no statusline, or timed out) — not a fresh session")
                else:
                    _n = f"context {_c[0]} tokens used" + (f" ({_c[1]}% of the window)" if _c[1] else "")
                    if _c[0] > CTX_FRESH:  # MUTATION-ANCHOR: 690-M01
                        _n += " → prefer a fresh executor over hw next or a ruling"
                    notes.append(_n)
            _nr, _oldest = pending_rulings_of((newest or {}).get("dir"))
            if _nr:  # MUTATION-ANCHOR: 690-M02
                _age = max(0, int(time.time() - _oldest)) if _oldest else 0
                _human = f"{_age}s" if _age < 120 else (f"{_age // 60} min" if _age < 7200 else f"{_age // 3600} h")
                notes.append(f"{_nr} ruling{'s' if _nr != 1 else ''} queued, not delivered — oldest {_human} old; "
                             "it lands at the executor's next turn end")
            # THE NAMED BROKEN STATE. `done_state=undelivered` says the report
            # exists and did not land; `turn_state=ended_unreported` says the turn
            # that would have retried the delivery is over. Either alone is
            # ordinary — a healthy executor publishes `ended_unreported` routinely
            # (measured on w30:p54, mid-work, turns=6, because a background-subagent
            # notification opens a turn with no prompt), and `undelivered` alone is
            # a delivery still in flight. TOGETHER they are neither, and nothing
            # inside that pane is going to fix it.
            #
            # The pair was ALREADY half-named: the `done-NOT-delivered` and
            # `blocked-NOT-delivered` rows below have been WARN since they were
            # written. What was missing is the second half — those branches consume
            # `done_tok`/`blocked_tok` and the `elif turn_ended` below is therefore
            # unreachable from them, so a row carrying BOTH facts printed only the
            # first and read as a delivery that might still land. It cannot.
            stranded = turn_ended and done_state == "undelivered" and not done_newest
            def strand_note():
                notes.append(
                    f"turn {turns} ended {turn_at} and the delivery had already failed — "
                    "nothing in that pane is going to retry it")
                notes.append(
                    "ENDED AND NEVER REPORTED: read its tokens (sum/sum2/…) and its engram "
                    "observations, then `hw done` it. A larger budget and `hw unstick` both "
                    "answer a question nobody is asking — the work finished, the delivery did not")
                limits.add("turn")

            if newest and newest.get("reopened"):
                # `hw revive` gave a reported run a new pane. The old report stands
                # and a new one is owed; the row must not read as done (it is not,
                # any more) nor as a fresh launch (it has a report behind it).
                mark, state, tint = "●", "reopened", OK
                notes.append("reopened by hw revive after reporting done — a new done-invoker is expected, without --retask")
            elif newest and newest.get("resumed"):
                # `hw ruling` answered a blocked report and the task is open again
                # in the same session; the pane's old `blocked` tokens predate it.
                mark, state, tint = "●", "resumed", OK
                notes.append("resumed by hw ruling after a BLOCKED report — same task, same session; a new done-invoker is expected")
            # A BLOCKED REPORT ALSO WRITES `done` (it was delivered), so the disk
            # marker alone cannot say which ending happened; `blocked-waiting` or
            # the blocked token can, and either one outranks it.
            elif (done_tok or done_newest) and not blocked_now:
                # A real report always outranks the turn hook: it knows a turn
                # ended, done-invoker knows the task did.
                if done_state == "undelivered" and not done_newest:
                    if stranded:
                        mark, state, tint = "!", "done-STRANDED", WARN
                    else:
                        mark, state, tint = "!", "done-NOT-delivered", WARN
                else:
                    mark, state, tint = "✓", "reported-done", OK
                    if chain and chain.get("live"):
                        state = "chaining-retained"
                    elif chain and chain.get("expired"):
                        mark, state, tint = "!", "chaining-expired", WARN
                if chain and chain.get("live"):
                    notes.append("reported done; chaining retention is LIVE until %s — holder %s: %s" % (chain.get("until_h", "?"), chain.get("holder", "?"), chain.get("reason", "?")))
                elif chain and chain.get("expired"):
                    notes.append("reported done; chaining lease EXPIRED, cleanup=%s" % chain.get("cleanup", "pending"))
                else:
                    notes.append("reported done, pane still open")
                if stranded:
                    strand_note()
            elif blocked_now:
                if done_state == "undelivered" and not done_newest:
                    if stranded:
                        mark, state, tint = "!", "blocked-STRANDED", WARN
                    else:
                        mark, state, tint = "!", "blocked-NOT-delivered", WARN
                elif newest and newest.get("blocked_since"):
                    mark, state, tint = "!", "blocked-waiting", WARN
                else:
                    mark, state, tint = "!", "reported-blocked", WARN
                if state == "blocked-waiting":
                    until = newest["blocked_since"] + blocked_wait_hours() * 3600
                    notes.append("reported BLOCKED and is WAITING for a ruling since %s — pane and session kept"
                                 % time.strftime("%d %b %H:%M", time.localtime(newest["blocked_since"])))
                    notes.append("answer it: `hw ruling %s \"<the unblock>\"` resumes the same task; `hw done %s --blocked` closes it; "
                                 "`hw reap --apply` closes it unanswered after %s (HW_BLOCKED_WAIT_HOURS=%d)"
                                 % ((a or {}).get("pane_id") or "<pane>", key.replace(":", " ", 1),
                                    time.strftime("%d %b %H:%M", time.localtime(until)), blocked_wait_hours()))
                else:
                    notes.append("reported BLOCKED and stopped, pane still open")
                if stranded:
                    strand_note()
            elif turn_ended:
                # THE STATE THE CHANNEL USED TO HAVE NO WORD FOR. Not running (it
                # stopped), not done (it never said so), not failed (nothing here
                # says it failed). Idle and silent, and the honest wording matters:
                # this is the shape a brainer must go and look at, not conclude from.
                status = (a.get("agent_status") if a else "") or ""
                if status in ("", "idle", "done", "unknown"):
                    mark, state, tint = "!", "idle, silent", WARN
                    notes.append(f"turn {turns} ended {turn_at} and done-invoker was never called")
                    notes.append("idle and silent — that is not failed and not done; read its engram findings or ask it in its pane")
                    limits.add("turn")
                else:
                    notes.append(f"turn {turns} ended {turn_at} unreported, and it is {status} again since")
            elif orph:
                # THE TABLE MUST NOT CONTRADICT THE SECTION BELOW. Reached when the
                # pane is still open and nothing in it is an agent any more: no
                # tokens survive that, so every branch above is blind and the row
                # used to read `process-live`. The pane being open is what makes it
                # RECOVERABLE, not what makes it running.
                mark, state, tint = "!", "finished, unreported", WARN
                notes.append("its agent is gone and the pane is a bare shell — see `finished, never reported` below")
                limits.add("orphan")
                _ur = unreported_row(newest)
                if _ur:
                    state = _ur[0]
                    notes.extend(_ur[1])
            if state == "working" and newest and t["workdir"]:
                quiet = stale_of(newest, t["workdir"], status_now(), stale_minutes())
                if quiet:
                    mark, state, tint = "!", "STALE", WARN
                    notes.append("working by herdr's word, but no turn end, file change or new process for %d min "
                                 "(last sign: %s at %s); threshold %g min, HW_STALE_MINUTES changes it"
                                 % (quiet["quiet"] // 60, quiet["what"],
                                    time.strftime("%d %b %H:%M", time.localtime(quiet["at"])),
                                    stale_minutes()))
                    notes.append("look at: `herdr pane read %s` (what it is doing), `hw log %s` (what was said), "
                                 "its engram findings; it may be waiting on something, hung, or in one long step"
                                 % ((a or {}).get("pane_id") or "<pane>", key.replace(":", " ", 1)))
                    limits.add("stale")
        elif ws and dead:
            mark, state, tint = "✗", "DIED", ERR
            notes.append(f"space {ws} is open with no agent, and {', '.join(dead)} exited")
            notes.append("herdr reports no exit code, so this says died, not failed")
            limits.add("exit")
        elif ws:
            mark, state, tint = "!", "no agent", WARN
            notes.append(f"space {ws} is open but nothing in it is an agent — never started, or released")
        elif newest and newest.get("reopened"):
            mark, state, tint = "!", "reopened, gone", WARN
            notes.append("hw revive reopened this run and its pane is gone with no new report — `hw done` it, or `hw revive` it again")
        elif blocked_tok:
            mark, state, tint = "!", "blocked", WARN
            notes.append("the executor reported BLOCKED and stopped, and its "
                         + _placement_gone(here_run, own_tab, own_ws))
        elif done_tok or done_disk:
            mark, state, tint = "✓", "done", OK
            notes.append("reported done, " + _placement_gone(here_run, own_tab, own_ws))
        elif orph:
            # Nothing of this task is left running anywhere. This row used to
            # collapse into the `no information` count, which is the honest verdict
            # only while nothing places the task in time — and `turns` does.
            mark, state, tint = "!", "finished, unreported", WARN
            notes.append("its %s and no turn since — see `finished, never reported` below"
                         % _placement_gone(here_run, own_tab, own_ws))
            limits.add("orphan")
            _ur = unreported_row(newest)
            if _ur:
                state = _ur[0]
                notes.extend(_ur[1])
        elif (newest and not newest.get("done") and newest.get("turns", 0) < 1
              and session_start is not None and newest.get("started") is not None
              and newest["started"] >= session_start):
            # THE ROW THAT WAS HIDDEN. Started in this herdr session, no pane, no
            # agent, no report — and the harness never watched one turn end. That
            # is an executor gone before it finished its first turn, and it used
            # to fall through to `no information`, which `hw status` collapses to
            # a count. It needs the same visibility as a death the space reports.
            _ur = unreported_row(newest) if newest.get("closed") else None  # MUTATION-ANCHOR: 260-M01
            if _ur:
                # `hw done` closed it on purpose before a first turn ended; the
                # mark says so, and a closed task is not a death.
                mark, state, tint = "!", _ur[0], WARN
                notes.append("its %s, and no turn end was ever recorded"
                             % _placement_gone(here_run, own_tab, own_ws))
                notes.extend(_ur[1])
                limits.add("first-turn")
            else:
                mark, state, tint = "✗", "DIED-BEFORE-FIRST-TURN", ERR
                notes.append("its %s, and no turn end was ever recorded — it died or was closed before finishing one "
                             "(`hw done` closes a task without a report too, and leaves no mark of it)"
                             % _placement_gone(here_run, own_tab, own_ws))
                notes.append("look at: `hw log %s`, its engram findings, .hw/%s/ in its work directory; "
                             "if it was closed on purpose, `hw done` leaves this row in place for the rest of this herdr session; otherwise dispatch it again"
                             % (key.replace(":", " ", 1), newest.get("name")))
                limits.add("first-turn")
        else:
            mark, state, tint = "?", "no information", DIM
            started = newest["started"] if newest else None
            if session_start is None:
                notes.append("no space, no done marker, and hw could not read the herdr session's start time — the backlog's silence proves nothing")
                limits.add("session")
            elif started is None:
                notes.append("no space, no done marker, and no .hw run record — nothing places this task in time, so the backlog cannot answer for it")
                limits.add("session")
            elif started < session_start:
                notes.append("last run started " + time.strftime("%d %b %H:%M", time.localtime(started))
                             + ", before this herdr session began "
                             + time.strftime("%d %b %H:%M", time.localtime(session_start)))
                notes.append("the pane_exited backlog is per session, so it holds nothing about this task")
                limits.add("session")
            elif here_run:
                # A --here run never had a space, so its absence proves nothing and
                # this branch used to lie about it. What IS true: no pane anywhere in
                # `herdr pane list` sits in this work directory or carries this task's
                # label, and no done marker was written.
                notes.append("a --here run, so it never had a space of its own — and no live pane is in its work directory")
                notes.append("it ended without reporting: no done marker, and nothing left to ask")
            else:
                _what = ("tab %s" % own_tab) if own_tab else \
                        (("workspace %s" % own_ws) if own_ws else "space")
                notes.append("started in this herdr session, and its %s is gone with no done marker — closed without reporting" % _what)
                if own_tab:
                    notes.append("a done marker only appears once the brainer is TOLD; if this executor reported while you were mid-turn, the delivery failed and the work may be complete — check its engram findings before calling it abandoned")
            # No `turn_state` note here on purpose: the adapter publishes onto the
            # executor's own pane, and herdr drops an agent record when its pane dies.
            # So a turn token can only ever be read while the pane is alive, which is
            # the branch above. Nothing to say here that would not be invented.

        counts = [shape_of(t["workdir"])]
        if t["workdir"]:
            counts.append(f"{len(runs)} run" + ("" if len(runs) == 1 else "s"))
            # Only when it is not 1. A re-tasked executor is the interesting case and
            # every legacy run is task 1, so printing it always would be noise.
            if newest and newest.get("task", 1) > 1:
                counts.append(f"task {newest['task']}")

        # Padded on the PLAIN text: an f-string width counts the colour escapes as
        # characters and every coloured column came out short by nine.
        # ljust() only pads, it never separates: a key at or over the column width
        # butts straight against the state and the row reads as one token
        # (`<lane>:<task>no information`).
        # Guarantee the gap instead of trusting the width.
        # COLLAPSE THE DEAD HISTORY. `no information` is truthful and it was 25 of
        # 27 rows, which buries the one task that is actually running. It is never
        # actionable on its own — a task hw cannot place in time stays that way — so
        # it becomes a count, and `--all` brings every row back.
        if state == "no information" and not STATUS_ALL:
            _suppressed += 1
            continue
        print(f"  {tint}{mark}{Z} {key.ljust(38) if len(key) < 38 else key + '  '}{tint}{state.ljust(16)}{Z}  {DIM}{' · '.join(counts)}{Z}")
        for note in notes:
            print(f"      {DIM}{note}{Z}")

    if _suppressed:
        print(f"  {DIM}·{Z} {_suppressed} older task(s) with no information, hidden — run {DIM}hw status --all{Z} for them")
    return limits, orphans, cleanup_backlog
HW_STATUS_PY
}

_status_report_cleanup_source() {
  cat <<'HW_STATUS_PY'
def status_report_cleanup(cleanup_backlog):
    # A REPORTED TASK AND A CLOSED TAB ARE TWO DIFFERENT EVENTS. `done-invoker`
    # writes the marker and reports; only the later brainer-owned `hw done` closes
    # the owned container. The ordinary row said "pane still open", but a sentence
    # distributed across dozens of green rows is not a recoverable queue. This is
    # the queue. Its done-marker gate is why preserved unreported panes cannot enter.
    if cleanup_backlog:
        print(f"\n{B}reported done, not closed{Z}  {DIM}— cleanup backlog; every row has a durable done marker{Z}")
        for c in cleanup_backlog:
            panes_text = ", ".join(c["panes"]) if c["panes"] else "no listed pane"
            command = c["key"].replace(":", " ", 1)
            print(f"  {WARN}!{Z} {c['key'].ljust(38) if len(c['key']) < 38 else c['key'] + '  '}"
                  f"{WARN}{c['kind']} {c['id']} still exists{Z}")
            print(f"      {DIM}pane(s): {panes_text}; close this one with `hw done {command}`{Z}")
        print(f"  {DIM}·{Z} {DIM}batch recovery: `hw sweep --apply` closes only owned tabs/workspaces whose current task has a done marker{Z}")
        print(f"  {DIM}·{Z} {DIM}finished-unreported panes are excluded by that marker gate and are never touched{Z}")
HW_STATUS_PY
}

_status_report_orphans_source() {
  cat <<'HW_STATUS_PY'
def status_report_orphans(orphans, panes):
    # ── finished, never reported ───────────────────────────────────────────────
    #
    # Printed as its own section rather than left in the table because the table is
    # read for "what is running" and this queue is read for "what do I have to go
    # and recover". Each row answers the three questions a human needs before it can
    # decide to recover the report or drop it: WHEN it went quiet, WHY it could not
    # report, and WHERE the report still is.
    if orphans:
        # NOT the loop's `live_panes`, which the task loop rebinds to one task's
        # panes on every iteration and leaves holding the last task's. This is the
        # whole live set, which is what "is that brainer still there" asks about.
        all_live_panes = {p.get("pane_id") for p in panes if p.get("pane_id")}
        print(f"\n{B}finished, never reported{Z}  {DIM}— the turn ended, the agent is gone, no brainer was told{Z}")
        for o in sorted(orphans, key=lambda x: x["at"] or 0, reverse=True):
            when = (time.strftime("%d %b %H:%M", time.localtime(o["at"]))
                    if o["at"] else "at an unrecorded time")
            turns_word = "turn" if o["turns"] == 1 else "turns"
            print(f"  {WARN}!{Z} {o['key'].ljust(38) if len(o['key']) < 38 else o['key'] + '  '}"
                  f"{WARN}{('%d %s, last ended %s' % (o['turns'], turns_word, when)).ljust(16)}{Z}")
            print(f"      {DIM}why it never reported: {orphan_why(o, all_live_panes)}{Z}")
            # WHERE THE REPORT IS. A refused done-invoker prints its summary into the
            # executor's own pane, so a pane that is still open is the cheapest place
            # to recover it — and this is the case that looked healthy in the table.
            # Once the pane is gone the artifacts and engram are all that is left,
            # and saying so is what lets a human decide to drop it.
            alive = [p for p in (o["live_panes"] or []) if p]
            if alive:
                print(f"      {DIM}report: pane {', '.join(alive)} is still open — the summary it could not "
                      f"send is in that pane{Z}")
            elif o["exec_pane"]:
                sid = o.get("session_id") or ""
                if sid and not sid.startswith("none"):
                    print(f"      {DIM}report: its pane {o['exec_pane']} is gone, but the conversation is NOT "
                          f"lost — session {sid} ({o.get('session_vendor') or 'unknown vendor'}); "
                          f"{o.get('session_resume') or 'see receipt.jsonl'}{Z}")
                else:
                    print(f"      {DIM}report: its pane {o['exec_pane']} (from receipt.jsonl) is gone and no "
                          f"session id was recorded, so the pane copy is lost{Z}")
            else:
                print(f"      {DIM}report: no pane, and no pane recorded in receipt.jsonl{Z}")
            # The label this lane's executors were given, exactly as hw exports
            # it (ENGRAM_PROJECT, from _engram_label): "lane=label" per line in
            # HW_ST_ENGRAM. A lane missing there is named as itself.
            lane = o["key"].split(":", 1)[0]
            engram = dict(l.split("=", 1) for l in (os.environ.get("HW_ST_ENGRAM") or "").splitlines()
                          if "=" in l).get(lane, lane)
            print(f"      {DIM}also: {shape_of(o['workdir'])} in {o['workdir']}, and whatever it saved to "
                  f"engram under `{engram}`{Z}")
        print(f"  {DIM}·{Z} {DIM}recover or drop each one, then `hw done <project> <task>` takes it off the board{Z}")
HW_STATUS_PY
}

_status_report_caveats_source() {
  cat <<'HW_STATUS_PY'
def status_report_caveats(herdr_ok, limits):
    if not herdr_ok:
        print(f"\n  {WARN}!{Z} herdr is unreachable, so every row above is disk evidence only:")
        print(f"      {DIM}a done marker still means done; nothing else on this list means anything{Z}")
    elif limits:
        print(f"\n  {DIM}what this cannot tell you{Z}")
        if "exit" in limits:
            print(f"      {DIM}pane_exited carries a pane id and nothing else — no exit code, and a dead{Z}")
            print(f"      {DIM}pane returns null from `pane get`. A DIED row means the process is gone.{Z}")
            print(f"      {DIM}Whether it succeeded, failed or was killed is not recorded anywhere.{Z}")
        if "session" in limits:
            print(f"      {DIM}the event backlog starts when the herdr server does. For a task older{Z}")
            print(f"      {DIM}than that, hw says `no information` rather than reporting it as fine.{Z}")
        if "orphan" in limits:
            print(f"      {DIM}`finished, unreported` is three disk facts agreeing: no done marker, at least{Z}")
            print(f"      {DIM}one turn END watched by the harness, and no live agent. It cannot tell a task{Z}")
            print(f"      {DIM}that finished from one killed after a turn — a Stop hook never could. What it{Z}")
            print(f"      {DIM}does establish is that nothing is running and nobody was told.{Z}")
        if "stale" in limits:
            print(f"      {DIM}`STALE` is computed when you read it, from the last turn end, the run's{Z}")
            print(f"      {DIM}state files, the newest file in its work directory and the newest process{Z}")
            print(f"      {DIM}started there. herdr exposes no last-output time, so pane output is not a{Z}")
            print(f"      {DIM}signal, and one long step (a test run that started earlier) is not seen.{Z}")
        if "first-turn" in limits:
            print(f"      {DIM}`DIED-BEFORE-FIRST-TURN` is: started in this herdr session, pane gone, no done{Z}")
            print(f"      {DIM}marker, no turn end ever counted. A `hw done` with no report reads the same.{Z}")
        if "model" in limits:
            print(f"      {DIM}`model` is what the executor's own session record says answered, read at{Z}")
            print(f"      {DIM}launch. A divergence is against what hw handed the vendor (the lane's pin{Z}")
            print(f"      {DIM}when there is one). It is not re-read afterwards: `/model` inside the pane{Z}")
            print(f"      {DIM}changes what runs and this row will not know.{Z}")
        if "turn" in limits:
            print(f"      {DIM}`idle, silent` comes from the agent's turn-end adapter, which fires at the end{Z}")
            print(f"      {DIM}of a TURN. It cannot tell a finished task from a finished turn, so it says{Z}")
            print(f"      {DIM}only that the turn ended and done-invoker was not called. The executor may{Z}")
            print(f"      {DIM}be waiting on an ask-invoker reply, or thinking, or quietly finished.{Z}")
            print(f"      {DIM}`done-STRANDED` / `blocked-STRANDED` is the OTHER shape and it is not{Z}")
            print(f"      {DIM}ambiguous: the executor DID publish a report, the delivery did NOT land,{Z}")
            print(f"      {DIM}and its turn is over — so nothing inside that pane is going to retry it.{Z}")
            print(f"      {DIM}`hw wait` on such a pane exits 7 without starting a wait, for the same{Z}")
            print(f"      {DIM}reason: no budget reaches idle from there.{Z}")

    # `hw status` is what you run before touching the server, so this is where the
    # restart rule belongs. A cold restart resumes the agents and loses everything
    # else: every dev-server pane comes back as a bare shell, and no pane keeps its
    # environment (herdr re-injects only HERDR_*). `--handoff` keeps the pane
    # PROCESSES alive across the restart.
    print()
    print(f"  {DIM}restarting herdr? use `herdr update --handoff` — it keeps pane processes alive{Z}")
    print(f"  {DIM}(environment, dev servers and --model intact). A cold restart resumes the{Z}")
    print(f"  {DIM}agents only, from .hw/<run>/env; the dev servers come back as bare shells.{Z}")
    print()
HW_STATUS_PY
}

_status_python_source() {
  _status_inputs_source
  _status_liveness_source
  _status_runs_source
  _status_recovery_source
  _status_report_spaces_source
  _status_report_tasks_source
  _status_report_cleanup_source
  _status_report_orphans_source
  _status_report_caveats_source
  cat <<'HW_STATUS_PY'
status_report_spaces(workspaces, herdr_ok)
limits, orphans, cleanup_backlog = status_report_tasks(
    tasks, ws_by_label, agents_by_task, panes_by_task, agent_by_pane,
    dead_by_ws, session_start, herdr_ok)
status_report_cleanup(cleanup_backlog)
status_report_orphans(orphans, panes)
# For the brainer's SessionStart (setup/guards/lane_housekeeping.py): the same
# queue, machine-readable, so the hook does not parse the coloured table.
if os.environ.get("HW_STATUS_ORPHANS_JSON"):
    _live = {p.get("pane_id") for p in panes if p.get("pane_id")}
    with open(os.environ["HW_STATUS_ORPHANS_JSON"], "w", encoding="utf-8") as _f:
        json.dump([{"key": o["key"], "at": o.get("at"), "turns": o.get("turns"),
                    "why": orphan_why(o, _live)} for o in orphans], _f)
status_report_caveats(herdr_ok, limits)
HW_STATUS_PY
}
# "<lane>=<engram label>" per line — what _engram_label exports to each lane's
# executors, so `hw status` names the label the work was actually saved under.
_status_engram_labels() {
  local p
  for p in $HW_LANES; do printf '%s=%s\n' "$p" "$(_engram_label "$p")"; done
}

cmd_status() {
  local ws_json panes_json agents_json exited_ndjson server_started herdr_ok=1

  ws_json="$(_capture herdr workspace list)" || { herdr_ok=0; warn "herdr unreachable: $(printf '%s' "$ws_json" | head -1)"; ws_json='{}'; }
  if [ "$herdr_ok" = 1 ]; then
    # STDOUT ONLY. `_capture` exists to turn a failure's stderr into a readable
    # diagnostic, which is exactly what ws_json above wants — but these two are
    # PARSED, and `_capture` runs its command with `2>&1`, so any notice herdr
    # ever writes to stderr is prepended to the JSON and every downstream jq
    # silently yields nothing. `hw status` would then report an empty world with
    # no error at all. Same defect the invoker-pane resolver had.
    panes_json="$(herdr pane list 2>/dev/null)"  || panes_json='{}'
    agents_json="$(herdr agent list 2>/dev/null)" || agents_json='{}'
    # One connection, drain the replayed backlog, exit. --idle-ms is raised over
    # herdr-rpc's 300ms default because this backlog can be long and a status
    # command can afford to wait; --timeout-ms caps it so `hw status` always
    # returns. Exit 2 means "nothing arrived", which is a legitimate answer.
    exited_ndjson="$("$HERDR_RPC" subscribe pane.exited --drain --idle-ms 600 --timeout-ms 8000 2>/dev/null || true)"
  else
    panes_json='{}'; agents_json='{}'; exited_ndjson=""
  fi
  server_started="$(_herdr_server_started)"

  _status_python_source |
  HW_ST_OK="$herdr_ok" HW_ST_RUNENV="$HW_BIN_DIR/runenv" \
  HW_ST_WS="$ws_json" HW_ST_PANES="$panes_json" HW_ST_AGENTS="$agents_json" \
  HW_ST_EXITED="$exited_ndjson" HW_ST_SERVER_STARTED="$server_started" \
  HW_ST_COLOUR="$([ -n "$C_0" ] && echo 1 || echo 0)" \
  HW_ST_WORK="$WORK" HW_ST_PROJECTS="$HW_LANES" \
  HW_ST_ROOTS="$(_status_worktree_roots)" \
  HW_ST_ENGRAM="$(_status_engram_labels)" \
  python3 -
  # THE LABELS THE PROXY DID NOT HOLD (design §5): read-only, one line, never a
  # failed status. Nonzero is a finding (3) or an unreadable store (1).
  python3 "$(_engram_proxy_bin)" --audit 2>&1 || true
  # THE ALWAYS-LOADED MEMORY INDEXES, same contract: read-only, one line, and
  # nothing at all when there is nothing to say (docs/brief-and-memory.md § Memory).
  python3 "$HW_BIN_DIR/memory-audit" 2>&1 || true  # MUTATION-ANCHOR: 660-M05
}
