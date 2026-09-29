#!/usr/bin/env python3
"""Lint rulings (strict by default); --report-only diagnoses legacy files.

--staged is the pre-commit boundary: budget failures block, format diagnostics
are advisory until the separately owned migration lands. Reads index blobs,
never unstaged decisions. Budget and heading grammar belong to bin/decisions.
"""
import argparse
from pathlib import Path, PurePosixPath
import re
import runpy
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
POLICY = runpy.run_path(str(ROOT / "bin/decisions"))
MAX_ENTRY_LINES = 18  # one lane already sustains 18; the others average 37–48.
KEYS = ("Ruling", "Rules out", "Reverses", "Evidence")


def git(*args):
    return subprocess.check_output(["git", *args])


def blob(ref, path):
    result = subprocess.run(["git", "show", f"{ref}:{path}"], capture_output=True)
    if result.returncode:
        raise RuntimeError(f"cannot read {ref}:{path}")
    return result.stdout.decode("utf-8")


def headings(text):
    """Ignore fenced examples, but retain their physical lines in entry size."""
    fence = None
    for number, line in enumerate(text.splitlines()):
        marker = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
        if fence:
            if marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence) and not marker[2].strip():
                fence = None
            continue
        if marker:
            fence = marker[1]
            continue
        if line.startswith("## "):
            yield number, line


def title(heading):
    match = POLICY["ENTRY_RE"].match(heading)
    return match.groups()[-1].strip().lstrip("—–- ") if match else None


def entry_bodies(text):
    """(heading, body_lines) for every entry `bin/decisions` itself would parse
    as one — ENTRY_RE match AND a real calendar date (`entry_key`), same as
    `split()`'s own classification. A `## ` heading that fails either check is
    UNKNOWN to bin/decisions and must not mint an id `known` can rely on: a
    body under a malformed heading citing `**Evidence:** engram #NNNNN` would
    otherwise pass this linter for an id `decisions supersede` refuses to find.
    """
    lines = text.splitlines()
    found = list(headings(text))
    for i, (start, heading) in enumerate(found):
        end = found[i + 1][0] if i + 1 < len(found) else len(lines)
        match = POLICY["ENTRY_RE"].match(heading)
        if not match or POLICY["entry_key"](match) is None:
            continue
        yield heading, lines[start:end]


def lint(path, text, sources):
    lines = text.splitlines()
    entries = list(headings(text))
    # `Reverses:` names its target by ENGRAM ID (bin/decisions, `entry_id` /
    # `reverses_target`), not by title — a title is edited after the ruling
    # settles and is not unique across a project's history. `known` must
    # therefore be resolved the same way `bin/decisions` resolves it, or the
    # linter disagrees with the tool that owns the format.
    known = {
        POLICY["entry_id"](body)
        for source in sources
        for _, body in entry_bodies(source)
    } - {None}
    errors = []
    if text.strip() and not entries:
        errors.append(f"{path}:1: no dated entry headings found")
    for i, (start, heading) in enumerate(entries):
        if heading == POLICY["POINTER_HEADING"]:
            continue
        prefix = f"{path}:{start + 1}: "
        if not title(heading):
            errors.append(prefix + "entry heading requires date and title")
        end = entries[i + 1][0] if i + 1 < len(entries) else len(lines)
        body = lines[start:end]
        while body and not body[-1].strip():
            body.pop()
        if len(body) > MAX_ENTRY_LINES:
            errors.append(prefix + f"entry has {len(body)} lines; ceiling is {MAX_ENTRY_LINES}")
        fields = {}
        fenced = False
        for line in body[1:]:
            if re.match(r"^ {0,3}(`{3,}|~{3,})", line):
                fenced = not fenced
            if fenced:
                continue
            for key in KEYS:
                match = re.fullmatch(r"\*\*" + re.escape(key) + r":\*\*\s*(.+)", line)
                if match:
                    fields[key] = match[1].strip()
        for key in KEYS:
            if not fields.get(key):
                errors.append(prefix + f"missing **{key}:** value")
        if "Evidence" in fields and not re.fullmatch(r"engram #\d+", fields["Evidence"]):
            errors.append(prefix + "Evidence must name engram #NNNNN")
        reverse = fields.get("Reverses", "none")
        if reverse.strip().lower() != POLICY["NO_REVERSE"]:
            reverse_id = POLICY["as_id"](reverse)
            own_id = POLICY["as_id"](fields.get("Evidence"))
            if reverse_id is None or reverse_id not in known or reverse_id == own_id:
                errors.append(prefix + f"Reverses target not found (or self-reference): {reverse}")
    return errors


def archive_paths(paths, parent):
    return sorted(
        path for path in paths
        if path.startswith(parent + "/decisions/") and path.endswith(".md")
    )


def pointer_history(text, project):
    """Return non-generated pointer-region text with exact line bytes and order."""
    regions = []
    region = None
    in_pointer = False
    fence = None
    for raw in text.splitlines(keepends=True):
        line = raw.rstrip("\r\n")
        marker = POLICY["FENCE_RE"].match(line)
        if fence is not None:
            if marker and marker.group(1)[0] == fence[0] and len(marker.group(1)) >= len(fence):
                fence = None
        elif marker:
            fence = marker.group(1)

        if fence is None and POLICY["H2_RE"].match(line):
            match = POLICY["ENTRY_RE"].match(line)
            key = POLICY["entry_key"](match) if match else None
            if key is not None:
                if region is not None:
                    regions.append(region)
                    region = None
                in_pointer = False
                continue
            if in_pointer and line.strip() == POLICY["POINTER_HEADING"]:
                region.append(raw)
                continue
            if region is not None:
                regions.append(region)
                region = None
            in_pointer = False
            continue

        if fence is None and POLICY["POINTER_MARK"] in line:
            if region is not None:
                regions.append(region)
            region = [raw]
            in_pointer = True
            continue
        if in_pointer:
            region.append(raw)
    if region is not None:
        regions.append(region)

    generated = []
    if project is not None:
        block = POLICY["pointer_block"](project)
        generated = block[block.index(POLICY["POINTER_MARK"]):]

    history = []
    for found in regions:
        lines = [raw.rstrip("\r\n") for raw in found]
        consumed = 0
        if generated and lines[:len(generated)] == generated:
            consumed = len(generated)
        elif generated and generated[-1] == "" and lines[:len(generated) - 1] == generated[:-1]:
            consumed = len(generated) - 1
        remainder = tuple(found[consumed:])
        if remainder:
            history.append(remainder)
    return tuple(history)


def parse_snapshot(files):
    """Parse git blobs with bin/decisions' exact split and ordering rules."""
    with tempfile.TemporaryDirectory(prefix="decisions-check-") as temporary:
        root = Path(temporary)
        for name, text in files.items():
            target = root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(text, encoding="utf-8")

        policy_globals = POLICY["split"].__globals__
        policy_root = policy_globals["ROOT"]
        policy_globals["ROOT"] = str(root)
        try:
            parsed = {}
            for name, text in files.items():
                source = root / name
                split = POLICY["split"](str(source))
                if len(split) not in (3, 4):
                    raise RuntimeError(
                        f"bin/decisions split returned {len(split)} values; expected 3 or 4"
                    )
                preamble, entries, _unknown = split[:3]
                parts = PurePosixPath(name).parts
                project = parts[0] if parts and parts[0] in POLICY["PROJECTS"] else None
                carried = pointer_history(text, project)

                def record(entry):
                    key, heading, body = entry
                    return key, heading, POLICY["canon"](body)

                parsed[name] = {
                    "text": text,
                    "preamble": tuple(preamble),
                    "file_entries": tuple(record(entry) for entry in entries),
                    "ordered_entries": tuple(
                        record(entry) for entry in POLICY["in_order"](str(source), entries)
                    ),
                    "carried": tuple(carried),
                }
            return parsed
        finally:
            policy_globals["ROOT"] = policy_root


def sequence_problem(expected, actual):
    for position, (wanted, found) in enumerate(zip(expected, actual), 1):
        if wanted == found:
            continue
        if wanted[1] == found[1]:
            return f"heading/body identity changed at {wanted[1]}"
        return (
            f"heading sequence differs at position {position}: "
            f"expected {wanted[1]!r}, staged {found[1]!r}"
        )
    if len(expected) != len(actual):
        return f"heading sequence has {len(actual)} entries; expected {len(expected)}"
    return None


def rotation_errors(path, old_files, new_files):
    """Prove an over-budget commit is one exact oldest-prefix rotation."""
    if path not in old_files:
        return ["HEAD has no live history from which a rotation can be proved"]

    parent = str(PurePosixPath(path).parent)
    project = PurePosixPath(parent).name
    old_archives = set(old_files) - {path}
    new_archives = set(new_files) - {path}
    missing_history = sorted(old_archives - new_archives)
    if missing_history:
        return ["archive history was deleted: " + ", ".join(missing_history)]

    old = parse_snapshot(old_files)
    new = parse_snapshot(new_files)
    old_live = old[path]
    new_live = new[path]
    if old_live["preamble"] != new_live["preamble"]:
        return ["live preamble changed while rotating history"]
    if old_live["carried"] != new_live["carried"]:
        return ["non-generated live pointer history changed while rotating"]

    old_entries = old_live["ordered_entries"]
    staged_entries = new_live["file_entries"]
    old_headings = {
        entry[1]
        for parsed in old.values()
        for entry in parsed["ordered_entries"]
    }
    candidates = []
    for cut in range(1, len(old_entries)):
        retained = old_entries[cut:]
        if staged_entries[:len(retained)] != retained:
            continue
        appended = staged_entries[len(retained):]
        if len(set(appended)) != len(appended):
            continue
        if any(entry[1] in old_headings for entry in appended):
            continue
        if tuple(sorted(retained + appended, key=lambda entry: entry[0])) != retained + appended:
            continue
        candidates.append((cut, appended))

    if len(candidates) != 1:
        old_heading_sequence = tuple(entry[1] for entry in old_entries)
        staged_heading_sequence = tuple(entry[1] for entry in staged_entries)
        for cut in range(1, len(old_entries)):
            retained_headings = old_heading_sequence[cut:]
            if staged_heading_sequence[:len(retained_headings)] == retained_headings:
                return [
                    "live heading/body identities changed; retained entries must be exact"
                ]
        return [
            "staged live headings are not one exact newest suffix of HEAD; "
            "only the oldest prefix may move"
        ]

    cut, _appended = candidates[0]
    moved = old_entries[:cut]
    buckets = {}
    for entry in moved:
        quarter = POLICY["quarter"](entry[0])
        target = str(PurePosixPath(parent) / "decisions" / f"{quarter}.md")
        buckets.setdefault(target, []).append(entry)

    expected_archives = old_archives | set(buckets)
    if new_archives != expected_archives:
        missing = sorted(expected_archives - new_archives)
        extra = sorted(new_archives - expected_archives)
        details = []
        if missing:
            details.append("missing " + ", ".join(missing))
        if extra:
            details.append("unexpected " + ", ".join(extra))
        return ["archive history set does not match the proved rotation: " + "; ".join(details)]

    for archive in sorted(expected_archives):
        if archive not in buckets:
            if new_files[archive] != old_files[archive]:
                return [f"untouched archive history changed: {archive}"]
            continue

        previous = old.get(archive)
        existing = previous["ordered_entries"] if previous else ()
        chronological = tuple(sorted(existing + tuple(buckets[archive]), key=lambda entry: entry[0]))
        expected_file_entries = tuple(reversed(chronological))
        problem = sequence_problem(expected_file_entries, new[archive]["file_entries"])
        if problem:
            return [f"archive history {archive} {problem}; exact heading/body order is required"]

        quarter = PurePosixPath(archive).stem
        expected_preamble = (
            previous["preamble"]
            if previous
            else tuple(POLICY["archive_preamble"](project, quarter))
        )
        if new[archive]["preamble"] != expected_preamble:
            return [f"archive history preamble changed: {archive}"]
        if previous and new[archive]["carried"] != previous["carried"]:
            return [f"non-entry archive history changed: {archive}"]
    return []


def staged():
    paths = set(filter(None, git("ls-files", "-z").decode().split("\0")))
    changed = set(filter(None, git("diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z").decode().split("\0")))
    old_paths = None
    failed = False
    for path in sorted(changed):
        if not path.endswith("/decisions.md"):
            continue
        text = blob("", path)
        parent = str(PurePosixPath(path).parent)
        archives = archive_paths(paths, parent)
        sources = [text] + [blob("", p) for p in archives]
        for error in lint(path, text, sources):
            print("decisions lint (advisory): " + error, file=sys.stderr)
        size = len(text.splitlines())
        if size <= POLICY["WARN_LINES"]:
            continue
        if old_paths is None:
            has_head = subprocess.run(
                ["git", "rev-parse", "--verify", "HEAD"], capture_output=True
            ).returncode == 0
            old_paths = set(
                git("ls-tree", "-r", "--name-only", "HEAD").decode().splitlines()
            ) if has_head else set()
        old_archives = archive_paths(old_paths, parent)
        old_files = ({path: blob("HEAD", path)} if path in old_paths else {})
        old_files.update({archive: blob("HEAD", archive) for archive in old_archives})
        new_files = {path: text}
        new_files.update({archive: blob("", archive) for archive in archives})
        problems = rotation_errors(path, old_files, new_files)
        if problems:
            for problem in problems:
                print(f"pre-commit: {path}: cannot prove rotation: {problem}", file=sys.stderr)
            print(f"pre-commit: {path}: {size} lines exceeds {POLICY['WARN_LINES']}; rotate in this commit (stage live file and preserved archive).", file=sys.stderr)
            failed = True
    return int(failed)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--staged", action="store_true")
    parser.add_argument(
        "--report-only", action="store_true",
        help="print findings but always exit 0 — diagnostic, never blocking "
             "(see the module docstring: format is advisory, not a gate)",
    )
    parser.add_argument("paths", nargs="*")
    args = parser.parse_args()
    if args.staged:
        return staged()
    if not args.paths:
        parser.error("name at least one decisions.md file")
    errors = []
    for name in args.paths:
        path = Path(name)
        text = path.read_text()
        sources = [text] + [p.read_text() for p in (path.parent / "decisions").glob("*.md")]
        errors.extend(lint(name, text, sources))
    for error in errors:
        print(error, file=sys.stderr)
    # Exiting 0 here with errors printed is INTENTIONAL when --report-only is
    # given: this mode diagnoses without blocking (module docstring, and the
    # flag's own --help). Without the flag, a direct-paths call is strict and
    # exits 1 on any lint error — that is the caller who wants $? to mean
    # something.
    return int(bool(errors) and not args.report_only)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, RuntimeError, UnicodeError, subprocess.CalledProcessError) as error:
        print(f"decisions check: {error}", file=sys.stderr)
        sys.exit(1)
