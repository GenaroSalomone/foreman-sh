#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/_common.sh"
python3 - "$(cd "$(dirname "$0")/../.." && pwd)" <<'PY'
import dataclasses, os, pathlib, runpy, shutil, subprocess, sys, tempfile

root = pathlib.Path(sys.argv[1])
checker = pathlib.Path(os.environ.get('DECISIONS_CHECKER', root/'setup/hooks/decisions-check.py'))
policy = pathlib.Path(os.environ.get('DECISIONS_POLICY', root/'bin/decisions'))
policy_api = runpy.run_path(str(policy))


@dataclasses.dataclass(frozen=True)
class Entry:
    heading: str
    body: tuple[str, ...]

    def block(self):
        return self.heading + '\n' + '\n'.join(self.body) + '\n'

    def body_text(self):
        return '\n'.join(self.body) + '\n'


def live_entry(number):
    hour, minute = divmod(number, 60)
    return Entry(
        f'## 2026-07-02 {hour:02}:{minute:02}:00 — live entry {number:03}',
        (
            '',
            f'**Ruling:** Preserve live entry {number:03}.',
            f'**Rules out:** Dropping live entry {number:03}.',
            '**Reverses:** none',
            f'**Evidence:** engram #{20000 + number}',
            f'Identity line A for live entry {number:03}.',
            f'Identity line B for live entry {number:03}.',
            '',
        ),
    )


def archived_entry(date, title, evidence):
    return Entry(
        f'## {date} 00:00:00 — {title}',
        (
            '',
            f'**Ruling:** Preserve {title}.',
            f'**Rules out:** Deleting {title}.',
            '**Reverses:** none',
            f'**Evidence:** engram #{evidence}',
            f'Historical body for {title}.',
            '',
        ),
    )


LIVE = tuple(live_entry(i) for i in range(150))
APPEND = Entry(
    '## 2026-07-02 03:00:00 — same-commit append',
    (
        '',
        '**Ruling:** Preserve the same-commit append.',
        '**Rules out:** Treating a provable append as corruption.',
        '**Reverses:** none',
        '**Evidence:** engram #20999',
        'Identity line for the appended entry.',
        '',
    ),
)
PRIOR_Q2 = archived_entry('2026-04-01', 'prior Q2 history', 19001)
PRIOR_Q3 = archived_entry('2026-07-01', 'prior Q3 history', 19002)
LIVE_PREAMBLE = '# setup — decisions\n\n---\n\n'
POINTER_NOTE = (
    'USER POINTER NOTE FIRST LINE THAT MUST NOT BE DROPPED\n'
    '\n'
    'USER POINTER NOTE SECOND LINE THAT MUST STAY IN SEQUENCE'
)


def render_live(entries):
    return LIVE_PREAMBLE + ''.join(entry.block() for entry in entries)


def with_pointer_note(text):
    block = '\n'.join(policy_api['pointer_block']('setup'))
    return text.rstrip() + '\n' + block + POINTER_NOTE + '\n'


def render_archive(quarter, entries):
    preamble = (
        f'# setup — decisions, {quarter} (archive)\n\n'
        'Closed quarter, newest first. Same contract as `../decisions.md`: a\n'
        'decision and what it rules out are the same entry. Nothing here was\n'
        'reversed by being archived — find it with `decisions index setup --all`,\n'
        'or `rg` this directory.\n\n---\n\n'
    )
    return preamble + ''.join(entry.block() for entry in entries)


def run(repo, *args):
    return subprocess.run(args, cwd=repo, text=True, capture_output=True)


def git(repo, *args):
    result = run(repo, 'git', *args)
    assert result.returncode == 0, result.stderr
    return result.stdout


def commit(repo, message):
    return run(
        repo,
        'git', '-c', 'user.name=Probe', '-c', 'user.email=probe@example.invalid',
        'commit', '-qm', message,
    )


def fixture(pointer_note=False):
    temporary = tempfile.TemporaryDirectory()
    repo = pathlib.Path(temporary.name)
    git(repo, 'init', '-q')
    (repo/'setup/hooks').mkdir(parents=True)
    (repo/'setup/decisions').mkdir()
    (repo/'bin').mkdir()
    shutil.copy(policy, repo/'bin/decisions')
    shutil.copy(checker, repo/'setup/hooks/decisions-check.py')
    live_text = render_live(LIVE)
    if pointer_note:
        live_text = with_pointer_note(live_text)
    (repo/'setup/decisions.md').write_text(live_text)
    (repo/'setup/decisions/2026-Q2.md').write_text(render_archive('2026-Q2', (PRIOR_Q2,)))
    (repo/'setup/decisions/2026-Q3.md').write_text(render_archive('2026-Q3', (PRIOR_Q3,)))
    git(repo, 'add', 'bin/decisions', 'setup/hooks/decisions-check.py',
        'setup/decisions.md', 'setup/decisions/2026-Q2.md',
        'setup/decisions/2026-Q3.md')
    baseline = commit(repo, 'baseline')
    assert baseline.returncode == 0, baseline.stderr
    # THE SUBJECT UNDER TEST IS THE REAL HOOK, REACHED THROUGH A WRAPPER THAT
    # SAYS IT WAS REACHED.
    #
    # Every negative case below concludes "the guard refuses this" from a
    # non-zero `git commit`, and every positive case concludes "the guard
    # permits this" from a zero. Neither reading is entitled to assume the hook
    # was consulted at all — and on 2026-09-09 it was not. `git -c
    # core.hooksPath=setup/hooks commit` (how this repo's pre-commit gets
    # invoked) exports that setting to every descendant git through
    # GIT_CONFIG_PARAMETERS, at command-line precedence, so these fixtures went
    # looking for their hook in <fixture>/setup/hooks/ — a directory this
    # function creates for decisions-check.py and never puts a hook in. No hook
    # ran, all eight corrupt commits were accepted, and the two positive cases
    # PASSED, silently certifying a guard that had not executed.
    #
    # The suite strips the whole GIT_CONFIG* family now, which removes that
    # cause. This wrapper removes the CLASS: the marker is emitted by the hook
    # invocation itself, so any future reason the hook does not run — a hooks
    # path, a mode bit, a git version, a fixture typo — turns every assertion in
    # this file into a named failure instead of a green line.
    # pre-commit now SOURCES its sibling setup/hooks/suite-trigger-pattern.sh
    # (one shared file with pre-push — see setup/tests/120) instead of
    # carrying its own inline copy of the trigger pattern. That sourcing is
    # unconditional (it has to run BEFORE the hook can even decide whether a
    # commit's staged paths match), so every fixture that copies pre-commit
    # standalone needs this sibling present too, or EVERY commit through it
    # fails on a missing-file error — including the decisions-only commits
    # this file's own fixtures are actually about. Copying the CURRENT
    # checkout's copy is harmless for the DECISIONS_OLD_REF branch below (an
    # old pre-commit that predates this sourcing simply never reads it).
    shutil.copy(root/'setup/hooks/suite-trigger-pattern.sh',
                repo/'setup/hooks/suite-trigger-pattern.sh')
    real = repo/'.git/hooks/pre-commit.real'
    if os.environ.get('DECISIONS_OLD_REF'):
        real.write_bytes(subprocess.check_output(
            ['git', 'show', os.environ['DECISIONS_OLD_REF'] + ':setup/hooks/pre-commit'],
            cwd=root,
        ))
    else:
        shutil.copy(root/'setup/hooks/pre-commit', real)
    real.chmod(0o755)
    hook = repo/'.git/hooks/pre-commit'
    hook.write_text(
        '#!/bin/sh\n'
        f'echo {HOOK_RAN} >&2\n'
        f'exec bash "{real}" "$@"\n'
    )
    hook.chmod(0o755)
    return temporary, repo


def rotate(repo):
    result = run(repo, 'python3', 'bin/decisions', 'archive', 'setup',
                 '--keep', '140', '--apply')
    assert result.returncode == 0, result.stdout + result.stderr
    assert len((repo/'setup/decisions.md').read_text().splitlines()) > 1200


def swap(text, first, second):
    token = '\nJD105-SWAP-TOKEN\n'
    assert token not in text and text.count(first) == 1 and text.count(second) == 1
    return text.replace(first, token).replace(second, first).replace(token, second)


HOOK_RAN = 'JD105-FIXTURE-PRE-COMMIT-RAN'

failures = []


def exercise(name, mutation, should_accept, diagnostic=None, pointer_note=False):
    temporary, repo = fixture(pointer_note=pointer_note)
    try:
        mutation(repo)
        result = commit(repo, name)
        print(f'{name} commit exit: {result.returncode}')
        if result.stderr.strip():
            print(result.stderr.strip())
        if HOOK_RAN not in result.stderr:
            # VACUOUS, and that outranks the verdict. Whatever the exit code
            # says, nothing was established about the guard, so this must not
            # read as either a pass or an ordinary assertion failure.
            failures.append(
                f'{name}: VACUOUS — the fixture pre-commit hook never ran '
                f'(no {HOOK_RAN} in stderr), so the commit exit code '
                f'{result.returncode} says nothing about the guard'
            )
        elif should_accept and result.returncode != 0:
            failures.append(f'{name}: valid commit was rejected')
        elif not should_accept and result.returncode == 0:
            failures.append(f'{name}: corrupt commit was unexpectedly accepted')
        elif diagnostic and diagnostic not in result.stderr:
            failures.append(f'{name}: rejection did not diagnose {diagnostic!r}')
        else:
            print(f'ok - {name}')
    finally:
        temporary.cleanup()


def valid_with_append(repo):
    rotate(repo)
    with (repo/'setup/decisions.md').open('a') as stream:
        stream.write(APPEND.block())
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


def delete_prior_archive(repo):
    rotate(repo)
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')
    git(repo, 'rm', '-q', 'setup/decisions/2026-Q2.md')


def shuffle_migrated_bodies(repo):
    rotate(repo)
    archive = repo/'setup/decisions/2026-Q3.md'
    archive.write_text(swap(
        archive.read_text(),
        '\n'.join(LIVE[0].body[1:-1]),
        '\n'.join(PRIOR_Q3.body[1:-1]),
    ))
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


def reorder_migrated_entries(repo):
    rotate(repo)
    archive = repo/'setup/decisions/2026-Q3.md'
    archive.write_text(swap(archive.read_text(), LIVE[0].block(), LIVE[1].block()))
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


def fence_migrated_entries(repo):
    rotate(repo)
    archive = repo/'setup/decisions/2026-Q3.md'
    text = archive.read_text()
    first = text.index(LIVE[9].heading)
    prior = text.index(PRIOR_Q3.heading)
    archive.write_text(text[:first] + '```markdown\n' + text[first:prior] + '```\n\n' + text[prior:])
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


def rotate_newest_instead(repo):
    rotate(repo)
    live = repo/'setup/decisions.md'
    pointer = live.read_text()
    pointer = pointer[pointer.index('<!-- decisions:archive-pointer -->'):]
    live.write_text(render_live(LIVE[:140]) + pointer)
    archive = repo/'setup/decisions/2026-Q3.md'
    text = archive.read_text()
    preamble = text[:text.index(LIVE[9].heading)]
    archive.write_text(
        preamble
        + ''.join(entry.block() for entry in reversed(LIVE[140:]))
        + PRIOR_Q3.block()
    )
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


def change_retained_body(repo):
    rotate(repo)
    live = repo/'setup/decisions.md'
    live.write_text(swap(live.read_text(), LIVE[20].body_text(), LIVE[21].body_text()))
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


def leave_archive_unstaged(repo):
    rotate(repo)
    git(repo, 'add', 'setup/decisions.md')


def drop_pointer_note(repo):
    rotate(repo)
    live = repo/'setup/decisions.md'
    text = live.read_text()
    preserved = POINTER_NOTE in text
    print('pointer note after policy rotation:', 'preserved' if preserved else 'dropped')
    if preserved:
        text = text.replace(POINTER_NOTE + '\n', '', 1)
        live.write_text(text)
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


def preserve_pointer_note(repo):
    rotate(repo)
    live = repo/'setup/decisions.md'
    text = live.read_text()
    preserved = POINTER_NOTE in text
    print('pointer note after policy rotation:', 'preserved' if preserved else 'dropped; restored')
    if not preserved:
        with live.open('a') as stream:
            stream.write(POINTER_NOTE + '\n')
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')


exercise('valid policy rotation with generated pointer and append', valid_with_append, True)
exercise('prior archive deletion', delete_prior_archive, False, 'archive history')
exercise('migrated and prior bodies under different headings', shuffle_migrated_bodies, False, 'heading/body')
exercise('migrated heading sequence reordered', reorder_migrated_entries, False, 'sequence')
exercise('migrated headings hidden in a fence', fence_migrated_entries, False, 'sequence')
exercise('newest entries archived instead of oldest', rotate_newest_instead, False, 'newest suffix')
exercise('retained live body changed', change_retained_body, False, 'heading/body')
exercise('archive left unstaged', leave_archive_unstaged, False, 'archive history')
exercise('pointer-region user note preserved', preserve_pointer_note, True,
         pointer_note=True)
exercise('pointer-region user note dropped', drop_pointer_note, False, 'pointer history',
         pointer_note=True)

if failures:
    for failure in failures:
        print('not ok -', failure)
    raise SystemExit(1)
print('ok - 105 staged budget proves exact rotation identity, sequence, and archive history')
PY
