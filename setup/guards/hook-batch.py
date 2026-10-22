#!/usr/bin/env python3
"""Run a guard's real hook file for many payloads in ONE process.

The conformance suite (`test-deny-repo-writes.mjs`) used to spawn `python3 <hook>`
once per vector, ~2000 interpreters per run, and the mutation arm
(`mutate-deny-repo-writes.sh`) repeated that for each of 77 mutants. This file is
the same hook, minus the interpreter start-up.

THERE IS NO SECOND IMPLEMENTATION HERE. Each request names a hook FILE — a lane
shim or `deny-repo-writes-codex.py` — and this runs that file with `runpy`, as
`__main__`, with its stdin, stdout, stderr, environment, working directory and
`sys.path` set to what a fresh `python3 <hook>` would have had. So the shim's
lane name, its guarded import, `main`'s payload checks, `decide`, and each
vendor's refusal protocol (Claude: JSON on stdout, exit 0; Codex: reason on
stderr, exit 2) all run exactly as they do in a pane. What this does NOT cover is
the process: interpreter start-up, `sys.path[0]` resolution by the OS, the exit
status the kernel reports. The suite keeps a fixed sample of vectors on a real
`spawnSync("python3", [hook])` for that, and asserts the sample's answers equal
this file's.

PROTOCOL: one JSON object per stdin line,
    {"hook": <path>, "stdin": <payload text>, "env": <object|null>, "cwd": <dir|null>}
one JSON object per stdout line, in order,
    {"status": <exit code>, "stdout": <text>, "stderr": <text>}
`env` null keeps this process's environment. An exception that escapes the hook
is what python would have printed and exited 1 on: status 1, traceback on stderr.

A guard module is imported ONCE per process (`sys.modules`), so a batch must not
mix hooks from two different guard copies: the suite starts one process per copy.
"""
import io
import json
import os
import runpy
import sys
import traceback


def run(request):
    hook = request["hook"]
    saved = (sys.stdin, sys.stdout, sys.stderr, list(sys.path), sys.argv,
             dict(os.environ), os.getcwd())
    out, err = io.StringIO(), io.StringIO()
    status = 0
    try:
        sys.stdin, sys.stdout, sys.stderr = io.StringIO(request["stdin"]), out, err
        sys.argv = [hook]
        # `python3 <script>` puts the script's directory first on the path.
        sys.path.insert(0, os.path.dirname(os.path.abspath(hook)))
        if request.get("env") is not None:
            os.environ.clear()
            os.environ.update(request["env"])
        if request.get("cwd"):
            os.chdir(request["cwd"])
        try:
            runpy.run_path(hook, run_name="__main__")
        except SystemExit as exit_:
            code = exit_.code
            if code is None:
                status = 0
            elif isinstance(code, int):
                status = code
            else:
                print(code, file=err)
                status = 1
        except BaseException:  # noqa: BLE001 — what an uncaught exception does to python
            traceback.print_exc(file=err)
            status = 1
    finally:
        sys.stdin, sys.stdout, sys.stderr, sys.path[:], sys.argv = (
            saved[0], saved[1], saved[2], saved[3], saved[4])
        os.environ.clear()
        os.environ.update(saved[5])
        os.chdir(saved[6])
    return {"status": status, "stdout": out.getvalue(), "stderr": err.getvalue()}


def main():
    for line in sys.stdin:
        if not line.strip():
            continue
        sys.stdout.write(json.dumps(run(json.loads(line))) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
