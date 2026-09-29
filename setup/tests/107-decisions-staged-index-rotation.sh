#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/_common.sh"
python3 - "$(cd "$(dirname "$0")/../.." && pwd)" <<'PY'
import os, pathlib, shutil, subprocess, sys, tempfile

root = pathlib.Path(sys.argv[1])
policy = pathlib.Path(os.environ.get('DECISIONS_POLICY', root/'bin/decisions'))


def require(condition, message):
    if not condition:
        print('not ok -', message)
        raise SystemExit(1)


def run(repo, *args):
    return subprocess.run(args, cwd=repo, text=True, capture_output=True)


def git(repo, *args):
    result = run(repo, 'git', *args)
    require(result.returncode == 0, result.stderr.strip() or 'git command failed')
    return result.stdout


def commit(repo, message):
    return run(
        repo,
        'git', '-c', 'user.name=Probe', '-c', 'user.email=probe@example.invalid',
        'commit', '-qm', message,
    )


def entry(number):
    hour, minute = divmod(number, 60)
    return (
        f'## 2026-07-02 {hour:02}:{minute:02}:00 — staged entry {number:03}\n'
        '\n'
        f'**Ruling:** Preserve staged entry {number:03}.\n'
        f'**Rules out:** Dropping staged entry {number:03}.\n'
        '**Reverses:** none\n'
        f'**Evidence:** engram #{22000 + number}\n'
        f'Identity line A for staged entry {number:03}.\n'
        f'Identity line B for staged entry {number:03}.\n'
        '\n'
    )


preamble = '# setup — decisions\n\n---\n\n'
baseline_text = preamble + ''.join(entry(number) for number in range(132))
staged_text = baseline_text + entry(132)
require(len(baseline_text.splitlines()) <= 1200, 'baseline fixture already exceeds 1200 lines')
require(len(staged_text.splitlines()) > 1200, 'staged append fixture does not exceed 1200 lines')

with tempfile.TemporaryDirectory() as temporary:
    repo = pathlib.Path(temporary)
    git(repo, 'init', '-q')
    (repo/'setup/hooks').mkdir(parents=True)
    (repo/'bin').mkdir()
    shutil.copy(policy, repo/'bin/decisions')
    shutil.copy(root/'setup/hooks/decisions-check.py', repo/'setup/hooks/decisions-check.py')
    live = repo/'setup/decisions.md'
    live.write_text(baseline_text)
    git(repo, 'add', 'bin/decisions', 'setup/hooks/decisions-check.py', 'setup/decisions.md')
    initial = commit(repo, 'baseline')
    require(initial.returncode == 0, initial.stderr)
    baseline_head = git(repo, 'rev-parse', 'HEAD').strip()

    # pre-commit now sources its sibling setup/hooks/suite-trigger-pattern.sh
    # unconditionally (see setup/tests/120) — a fixture that copies pre-commit
    # standalone needs that sibling present too, or every commit through it
    # fails on a missing-file error, regardless of what is staged.
    shutil.copy(root/'setup/hooks/suite-trigger-pattern.sh',
                repo/'setup/hooks/suite-trigger-pattern.sh')
    hook = repo/'.git/hooks/pre-commit'
    old_ref = os.environ.get('DECISIONS_OLD_REF')
    if old_ref:
        hook.write_bytes(subprocess.check_output(
            ['git', 'show', old_ref + ':setup/hooks/pre-commit'], cwd=root,
        ))
    else:
        shutil.copy(root/'setup/hooks/pre-commit', hook)
    hook.chmod(0o755)

    live.write_text(staged_text)
    git(repo, 'add', 'setup/decisions.md')
    indexed = git(repo, 'show', ':setup/decisions.md')
    live.write_text('unstaged short working file\n')

    rejected = commit(repo, 'append without rotation')
    print('staged over-budget append commit exit:', rejected.returncode)
    if rejected.stderr.strip():
        print(rejected.stderr.strip())
    require(rejected.returncode != 0,
            'over-budget staged append was accepted through an unstaged short working file')
    require('exceeds 1200' in rejected.stderr,
            'over-budget staged append was rejected without the decisions budget diagnosis')
    require(git(repo, 'rev-parse', 'HEAD').strip() == baseline_head,
            'rejected append advanced HEAD, so the pending commit was lost')
    require(live.read_text() == 'unstaged short working file\n',
            'the hook read or rewrote the unstaged working file')
    print('ok - staged index stays authoritative over an unstaged short working file')

    live.write_text(indexed)
    rotation = run(repo, 'python3', 'bin/decisions', 'archive', 'setup', '--apply')
    print('actual rotation exit:', rotation.returncode)
    if rotation.stdout.strip():
        print(rotation.stdout.strip())
    if rotation.stderr.strip():
        print(rotation.stderr.strip())
    require(rotation.returncode == 0, 'actual decisions rotation failed')
    archive = repo/'setup/decisions/2026-Q3.md'
    require(archive.exists(), 'actual rotation did not create the expected archive')
    git(repo, 'add', 'setup/decisions.md', 'setup/decisions/2026-Q3.md')

    accepted = commit(repo, 'append with rotation')
    print('same pending commit after rotation exit:', accepted.returncode)
    if accepted.stderr.strip():
        print(accepted.stderr.strip())
    require(accepted.returncode == 0, accepted.stderr.strip() or 'rotated commit was rejected')
    require(git(repo, 'rev-parse', 'HEAD^').strip() == baseline_head,
            'rotation did not complete the same pending commit over the baseline')
    committed_live = git(repo, 'show', 'HEAD:setup/decisions.md')
    committed_archive = git(repo, 'show', 'HEAD:setup/decisions/2026-Q3.md')
    require('staged entry 132' in committed_live,
            'the newest staged append did not remain in the committed live file')
    require('staged entry 000' not in committed_live and 'staged entry 000' in committed_archive,
            'the actual rotation did not move the oldest entry into the committed archive')
    print('ok - 107 rejected the staged append, then accepted that same commit after actual rotation')
PY
