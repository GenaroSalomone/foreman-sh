#!/usr/bin/env python3
"""Did a SendMessage actually land in the RECEIVER's own transcript?

WHY THIS IS A SEPARATE FILE. `bin/channel-send`'s claude route used to prove
delivery with `[ "$RESULT" = CHANNEL_SENT ]` — a language model printing a
string it had been told to print. Nothing inspected the tool_result, because
without `--output-format stream-json` the tool calls are invisible to the
caller. Exit 0 on that route is the fact that a ruling reached a held executor
or that a report reached a brainer, and CLAUDE.shared.md tells the operator in
bold not to resend on exit 0 — so a false success closed the recovery move too.

THE OBSERVATION THAT REPLACES IT. Claude Code writes a delivered SendMessage
into the receiver's own transcript at
`~/.claude/projects/<slug>/<sessionId>.jsonl`. Measured 2026-09-07 against a
live receiver, two records carry the payload:

    {"type": "queue-operation", "operation": "enqueue", "content": "<payload>"}
    {"type": "attachment", "attachment": {"type": "queued_command",
                                          "prompt": "<payload>"}}

The first is the message entering the receiver's queue; the second is it
entering a turn. Either is the RECEIVER reporting on itself, which is what a
receipt has to be.

THE SLUG IS NOT RECOMPUTED. The directory name is Claude Code's transform of
the receiver's cwd (`/` and `.` both become `-`), and guessing it wrong would
read as a non-delivery — the same class of mistake this file exists to remove.
The session id is unique, so the file is found by globbing for it.

THE WATERMARK, and it is what makes this a receipt for THIS delivery. The
transcript is append-only and a ruling is near-boilerplate ("the prohibition
stands, because …"), so scanning it from byte 0 would accept the record left by
an EARLIER send of the same text — including the one that just returned exit 5
and is being retried. The subject emitted a signal, but not the one being
asserted about, which is this file's own failure mode one level in. So the
caller passes `CHANNEL_SINCE`: the byte offset the transcript had BEFORE the
send, and only records after it count. Raised by a Judgment Day judge on
2026-09-07 with a working reproduction.

`CHANNEL_SINCE` is required. Not defaulted to 0 — a missing watermark is the
caller forgetting to take one, and silently scanning the whole file would be
exactly the false receipt this exists to prevent.

EXIT CODES, and the third one is the point:
    0   the payload bytes are in the receiver's transcript. Prints how it
        matched (`exact` or `contained`) and the file it was found in.
    1   NOT OBSERVED within the budget. This is deliberately not called a
        non-delivery: stdout says nothing, and stderr names WHICH of the two
        undetermined cases it is — `no-transcript` (no file for that session id
        exists yet) or `not-found` (the file exists and does not carry it).
        The caller turns this into exit 5, "may be in the receiver".

Usage (all through the environment, so no payload ever reaches a command line):
    CHANNEL_TARGET=<receiver session id> \
    CHANNEL_MESSAGE=<payload> \
    CHANNEL_LAND_MS=<budget> \
    CHANNEL_SINCE=<transcript byte offsets before the send, or "none"> \
    claude-delivery-receipt.py

`--mark` prints the watermark to take before sending: one `<path>\t<size>` line
per transcript that exists for the session, or nothing when none does yet.
"""
import glob
import json
import os
import sys
import time

# NATIVE WINDOWS (Git Bash): bin/msys-compat.sh puts bin/ on PYTHONPATH for the
# entry points it starts; run by hand, this file loads bin/sitecustomize.py
# itself (a no-op when Python already did). Elsewhere nothing happens.
if os.name == "nt":
    sys.path.insert(0, os.path.dirname(os.path.realpath(__file__)))
    import sitecustomize  # noqa: E402,F401

TRANSCRIPTS = "~/.claude/projects/*/%s.jsonl"


def carried(record):
    """Every string in `record` that would be the delivered payload."""
    if record.get("type") == "queue-operation" and record.get("operation") == "enqueue":
        yield record.get("content")
    if record.get("type") == "attachment":
        attachment = record.get("attachment") or {}
        if attachment.get("type") == "queued_command":
            yield attachment.get("prompt")


def watermark(pattern):
    """`{path: size}` for every transcript that exists right now."""
    marks = {}
    for path in glob.glob(pattern):
        try:
            marks[path] = os.path.getsize(path)
        except OSError:
            continue
    return marks


def scan(path, message, since):
    """`exact`, `contained`, or None — how this file carries `message`.

    Reading starts at `since`, the size this file had before the send. A record
    already there is somebody else's receipt, or this delivery's own previous
    attempt.
    """
    try:
        handle = open(path, encoding="utf-8")
    except OSError:
        return None
    with handle:
        # A transcript that SHRANK was rotated or replaced under us. Its offsets
        # no longer mean what they meant, so the honest move is to read none of
        # it and let the budget expire into the undetermined branch.
        try:
            if os.path.getsize(path) < since:
                return None
        except OSError:
            return None
        if since:
            handle.seek(since)
        for line in handle:
            if not line.strip():
                continue
            try:
                record = json.loads(line)
            except ValueError:
                continue
            for value in carried(record):
                if not isinstance(value, str):
                    continue
                body = value.strip()
                if body == message:
                    return "exact"
                # A harness that wraps the payload still put the receiver's copy
                # of these bytes on the receiver's disk, which is the fact being
                # established. Reported distinctly so a reader can tell.
                if message and message in body:
                    return "contained"
    return None


def parse_since(raw):
    """`<path>\t<size>` lines back into a dict. "none" is an empty watermark."""
    marks = {}
    if raw.strip() in ("", "none"):
        return marks
    for line in raw.splitlines():
        if not line.strip():
            continue
        path, _, size = line.rpartition("\t")
        try:
            marks[path] = int(size)
        except ValueError:
            continue
    return marks


def main():
    # `.get`, not `[...]`: a KeyError here would be raised BEFORE the guard on
    # the next line, so the diagnosis this file is careful to write would never
    # print — the caller would see a traceback and fold it into exit 5, and
    # "the prover broke" and "the receiver may have it" would read the same.
    target = os.environ.get("CHANNEL_TARGET", "")
    if not target:
        print("no receiver session id was given", file=sys.stderr)
        return 1
    pattern = os.path.expanduser(TRANSCRIPTS % target)

    if "--mark" in sys.argv[1:]:
        for path, size in sorted(watermark(pattern).items()):
            print("%s\t%d" % (path, size))
        return 0

    message = os.environ.get("CHANNEL_MESSAGE", "").strip()
    if not message:
        print("no payload was given (CHANNEL_MESSAGE): there is nothing to look "
              "for, and an empty needle would match the first record it read.",
              file=sys.stderr)
        return 1
    budget_ms = int(os.environ.get("CHANNEL_LAND_MS") or 20000)
    if "CHANNEL_SINCE" not in os.environ:
        print("no watermark was given (CHANNEL_SINCE): without one this would "
              "accept a record left by an EARLIER send of the same bytes, which "
              "is the false receipt this check exists to prevent. Take one with "
              "`--mark` before sending.", file=sys.stderr)
        return 1
    since = parse_since(os.environ["CHANNEL_SINCE"])

    deadline = time.time() + budget_ms / 1000.0
    saw_a_file = False
    while True:
        for path in glob.glob(pattern):
            saw_a_file = True
            how = scan(path, message, since.get(path, 0))
            if how:
                print("%s match in %s" % (how, os.path.basename(path)))
                return 0
        if time.time() >= deadline:
            break
        time.sleep(0.4)

    # NAME WHICH UNDETERMINED CASE IT IS. "No transcript for that session id"
    # and "the transcript exists and does not carry it" call for different
    # moves, and collapsing them is how an unknown becomes a false negative.
    print("no-transcript" if not saw_a_file else "not-found", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
