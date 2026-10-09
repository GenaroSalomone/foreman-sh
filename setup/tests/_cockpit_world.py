"""A fake world for the cockpit tests: run directories on disk, a `herdr` stub that
answers `pane list` with the panes of those runs, and the schema check.

Not a test (the leading underscore keeps the runner off it). Used by 797 (the writer),
798 (the writer against a real herdr probe workspace) and 799 (the mod-facing timing).

The run directories are built the way hw builds them — `<work>/<lane>/<task>/.hw/<run>/env`
with its `KEY='value'` grammar, `task` for the counter, `done`, `blocked-waiting`,
`pending-reply-<n>` holds in the grammar bin/holdfacts validates, `pending-ruling.<n>` —
so the writer reads real file shapes, not a mock of its own reading.
"""
import json
import os
import re
import stat
import subprocess
import sys
import time

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
SCHEMA = json.load(open(os.path.join(ROOT, "cockpit", "schema.json")))
DEFS = SCHEMA["$defs"]


def check(v, sc, path):
    """The subset of JSON Schema schema.json uses. Returns a list of errors."""
    if "$ref" in sc:
        return check(v, DEFS[sc["$ref"].split("/")[-1]], path)
    errs = []
    if "oneOf" in sc:
        ok = [s for s in sc["oneOf"] if not check(v, s, path)]
        return [] if len(ok) == 1 else ["%s: matches %d of oneOf" % (path, len(ok))]
    if "const" in sc and v != sc["const"]:
        return ["%s: != %r" % (path, sc["const"])]
    if "enum" in sc and v not in sc["enum"]:
        return ["%s: %r not in enum" % (path, v)]
    t = sc.get("type")
    if t:
        ts = t if isinstance(t, list) else [t]
        py = {"object": dict, "array": list, "string": str, "boolean": bool, "null": type(None),
              "number": (int, float), "integer": int}
        ok = any(isinstance(v, py[x]) and not (x in ("integer", "number") and isinstance(v, bool)) for x in ts)
        if not ok:
            return ["%s: type %s, wanted %s" % (path, type(v).__name__, ts)]
    if isinstance(v, (int, float)) and not isinstance(v, bool):
        if "minimum" in sc and v < sc["minimum"]:
            errs.append("%s: below minimum" % path)
        if "maximum" in sc and v > sc["maximum"]:
            errs.append("%s: above maximum" % path)
    if isinstance(v, str):
        if "maxLength" in sc and len(v) > sc["maxLength"]:
            errs.append("%s: too long" % path)
        if "pattern" in sc and not re.search(sc["pattern"], v):
            errs.append("%s: pattern" % path)
    if isinstance(v, list):
        if "maxItems" in sc and len(v) > sc["maxItems"]:
            errs.append("%s: too many items" % path)
        if "items" in sc:
            for i, x in enumerate(v):
                errs += check(x, sc["items"], "%s[%d]" % (path, i))
    if isinstance(v, dict):
        for k in sc.get("required", []):
            if k not in v:
                errs.append("%s: missing %s" % (path, k))
        props = sc.get("properties", {})
        if sc.get("additionalProperties") is False:
            for k in v:
                if k not in props:
                    errs.append("%s: unexpected %s" % (path, k))
        for k, x in v.items():
            if k in props:
                errs += check(x, props[k], path + "." + k)
    for a in sc.get("allOf", []):
        i, th = a.get("if"), a.get("then")
        if i is not None and not check(v, i, path) and th is not None:
            errs += check(v, th, path)
    return errs


def validate_state(d):
    return check(d, DEFS["state"], "state")


def iso(ts):
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(ts))


def chunk(prefix, text, n, width=80):
    """What invoker_chunk_tokens publishes and herdr stores: whitespace-preferring slices of
    `width`, trailing whitespace trimmed by herdr, `prefix`, `prefix2` …"""
    out, rest = [], text
    while len(rest) > width:
        head = rest[:width]
        cut = max(head.rfind(" "), head.rfind("\n"), head.rfind("\t"))
        cut = width if cut <= 0 else cut + 1
        out.append(rest[:cut])
        rest = rest[cut:]
    out.append(rest)
    return {(prefix if i == 0 else "%s%d" % (prefix, i + 1)): c.rstrip() for i, c in enumerate(out[:n])}


class World:
    def __init__(self, tmp, invoker="w1:p1"):
        self.tmp, self.invoker = tmp, invoker
        self.work = os.path.join(tmp, "work")
        self.panes = []
        self.counter = 0
        os.makedirs(self.work, exist_ok=True)
        self.herdr = os.path.join(tmp, "herdr")
        self.panes_json = os.path.join(tmp, "panes.json")
        self.screens_dir = os.path.join(tmp, "screens")   # what `herdr agent read <pane>` prints, one file per pane
        self.reads_log = os.path.join(tmp, "reads.log")    # one line per `agent read` the stub answered

    # ── building ──
    def executor(self, project="demo", task=None, *, pane=None, run=None, seq=1, vendor="claude", status="idle",
                 tokens=None, invoker=None, done=False, blocked_waiting=False, reopened=False, holds=(),
                 rulings=(), model="claude-sonnet-5-5", cwd=None, with_run=True, effort=None, dispatched_at=None, screen=None):
        self.counter += 1
        n = self.counter
        task = task or "task-%02d" % n
        pane = pane or "w1:p%d" % (10 + n)
        run = run or "20261006-1000%02d-%d" % (n % 60, 1000 + n)
        wd = cwd or os.path.join(self.work, project, task)
        rd = os.path.join(wd, ".hw", run)
        if with_run:
            os.makedirs(rd, exist_ok=True)
            env = {"HW_EXECUTOR_VENDOR": vendor, "HW_INVOKER_PANE": invoker or self.invoker, "HW_PROJECT": project,
                   "HW_TASK": task, "HW_RUN": run}
            with open(os.path.join(rd, "env"), "w") as fh:
                fh.write("# written by hw\n" + "".join("%s='%s'\n" % kv for kv in env.items()))
            with open(os.path.join(rd, "receipt.jsonl"), "w") as fh:
                # real receipts carry the time of each fact: the first one is when the run began
                line = {"key": "model_running", "value": model}
                if dispatched_at is not None:
                    line["at"] = iso(dispatched_at)
                fh.write(json.dumps(line) + "\n")
            if effort is not None:  # the dispatch file's shape: `effort=<v>  (mark)`
                open(os.path.join(rd, "dispatch"), "w").write("model=%s  (chosen)\n  effort=%s  (chosen)\n" % (model, effort))
            if seq > 1:
                open(os.path.join(rd, "task"), "w").write("%d\n" % seq)
            td = rd if seq == 1 else os.path.join(rd, "t%d" % seq)
            os.makedirs(td, exist_ok=True)
            if done:
                open(os.path.join(td, "done"), "w").write("done\n")
            if reopened:
                open(os.path.join(td, "reopened"), "w").write("\n")
            if blocked_waiting:
                open(os.path.join(td, "blocked-waiting"), "w").write("since=%d\n" % int(time.time()))
            for h in holds:
                self.hold(td, pane, run, **h)
            for i, text in enumerate(rulings):
                open(os.path.join(rd, "pending-ruling" + ("" if i == 0 else ".%d" % (i + 1))), "w").write(text + "\n")
        p = {"pane_id": pane, "agent": vendor, "agent_status": status, "cwd": wd, "tokens": dict(tokens or {})}
        p["tokens"].update({"hw_project": project, "hw_task": task, "hw_run": run})
        self.panes.append(p)
        if screen is not None:
            os.makedirs(self.screens_dir, exist_ok=True)
            open(os.path.join(self.screens_dir, pane.replace(":", "_")), "w").write(screen)
        return {"pane": pane, "run": run, "rundir": rd, "taskdir": (rd if seq == 1 else os.path.join(rd, "t%d" % seq)),
                "task": task, "project": project, "pane_item": p}

    def hold(self, taskdir, pane, run, kind="ask", n=1, state="delivered", route="herdr", target=None):
        intent = "answer" if kind == "ask" else "ruling"
        lid = "%s:%s:%d:reply" % (run, kind, n)
        rows = ["version=1", "state=" + state, "intent=" + intent, "route=" + route, "target=" + (target or pane),
                "logical_id=" + lid, "pane=" + pane, "run=" + run]
        path = os.path.join(taskdir, "pending-reply-%d" % n)
        open(path, "w").write("\n".join(rows) + "\n")
        return path

    def noise(self, n):
        """Panes that are not executors of this brainer: shells, other brainers, no tokens."""
        for i in range(n):
            self.panes.append({"pane_id": "w9:p%d" % i, "agent_status": "unknown", "cwd": self.tmp})

    # ── the herdr stub ──
    def publish(self, fail=False, hang_reads=False):
        json.dump({"id": "cli:pane:list", "result": {"panes": self.panes}}, open(self.panes_json, "w"))
        body = "echo boom >&2; exit 1\n" if fail else 'cat "%s"\n' % self.panes_json
        # `herdr agent read <pane> --source visible`: the pane's screen, or exit 1 when it has none
        reads = ('if [ "$1" = agent ] && [ "$2" = read ]; then echo "$3" >> "%s"; %s'
                 'f="%s/$(echo "$3" | tr : _)"; [ -f "$f" ] && cat "$f" || exit 1; exit 0; fi\n'
                 % (self.reads_log, "sleep 30; " if hang_reads else "", self.screens_dir))
        with open(self.herdr, "w") as fh:
            fh.write("#!/bin/sh\n" + reads + body)
        os.chmod(self.herdr, os.stat(self.herdr).st_mode | stat.S_IXUSR)

    def reads(self):
        """How many `agent read` calls the stub has answered so far."""
        try:
            return len(open(self.reads_log).read().split())
        except OSError:
            return 0

    def env(self, **extra):
        e = dict(os.environ)
        e.update({"HW_COCKPIT_HERDR": self.herdr, "HW_COCKPIT_WORK": self.work, "HW_COCKPIT_INVOKER": self.invoker})
        e.pop("HW_COCKPIT_STATE", None)
        e.update(extra)
        return e

    def state_path(self):
        return os.path.join(self.work, ".cockpit", self.invoker + ".json")


def run_writer(world, bin_dir, *args, timeout=30, **env):
    return subprocess.run([os.path.join(bin_dir, "cockpit-state")] + list(args), env=world.env(**env),
                          capture_output=True, text=True, timeout=timeout)


def stdout_state(world, bin_dir):
    r = run_writer(world, bin_dir, "--stdout")
    if r.returncode != 0:
        sys.exit("cockpit-state --stdout exited %d: %s" % (r.returncode, r.stderr))
    return json.loads(r.stdout)
