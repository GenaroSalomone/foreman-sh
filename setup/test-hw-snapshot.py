#!/usr/bin/env python3
"""Run test-hw on a private, measured copy, never on changing checkout bytes.

This is optimistic drift detection, NOT an atomic filesystem snapshot. No lock
can exclude writers that do not participate. The before/copy/after manifests
must agree before tests start; another writer can then change the original
without changing the measured candidate. HOME, operator state under the real
brain directory, and external symlink targets remain shared/live dependencies,
as do the explicitly live checks in subject 69. This narrows concurrent-source
contamination; it does NOT close concurrent-live-state contamination.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile


def inventory(root):
    """Include ignored files, new subjects, dependencies and symlink objects.

    Git metadata is the only omission: it must never refer to the original
    index/worktree. Symlink targets outside the checkout are not followed.
    """
    result = {}
    for parent, dirs, files in os.walk(root, followlinks=False):
        dirs[:] = sorted(d for d in dirs if d != '.git')
        for name in sorted(dirs + [f for f in files if f != '.git']):
            path = Path(parent) / name
            rel = str(path.relative_to(root))
            info = path.lstat()
            mode = stat.S_IMODE(info.st_mode)
            if path.is_symlink():
                value = ['link', mode, os.readlink(path)]
            elif stat.S_ISDIR(info.st_mode):
                value = ['dir', mode]
            elif stat.S_ISREG(info.st_mode):
                with path.open('rb') as source:
                    digest = hashlib.file_digest(source, 'sha256').hexdigest() if hasattr(hashlib, 'file_digest') else hashlib.sha256(source.read()).hexdigest()
                after = path.stat()
                # Reading may update atime; it is not a source-byte change.
                stable = ('st_dev', 'st_ino', 'st_mode', 'st_size', 'st_mtime_ns', 'st_ctime_ns')
                if any(getattr(after, key) != getattr(info, key) for key in stable):
                    raise RuntimeError('source changed while reading: ' + rel)
                value = ['file', mode, digest]
            else:
                raise RuntimeError('unsupported source entry (not copied): ' + rel)
            result[rel] = value
    return result


def tracking(root):
    try:
        proc = subprocess.run(['git', '-C', str(root), 'ls-files', '-z'],
                              stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    except FileNotFoundError:
        return {'usable': False, 'tracked': []}
    return {'usable': proc.returncode == 0,
            'tracked': sorted(os.fsdecode(p) for p in proc.stdout.split(b'\0') if p) if proc.returncode == 0 else []}


def main():
    root = Path(sys.argv[1]).resolve()
    with tempfile.TemporaryDirectory(prefix='hw-source-') as temporary:
        base = Path(temporary).resolve()
        if base == root or root in base.parents:
            raise RuntimeError('temporary snapshot directory is inside the source checkout; set TMPDIR outside it')
        candidate = base / 'brain'
        before = inventory(root)
        tracked = tracking(root)
        shutil.copytree(root, candidate, symlinks=True,
                        ignore=lambda _parent, names: ['.git'] if '.git' in names else [])
        frozen = inventory(candidate)
        after = inventory(root)
        if before != frozen or before != after or tracked != tracking(root):
            other = frozen if before != frozen else after
            diff = sorted(k for k in set(before) | set(other) if before.get(k) != other.get(k))[:3]
            raise RuntimeError('source or tracking changed during capture; retry from a quiescent checkout'
                               + (' (first differences: %s)' % ', '.join(
                                   '%s %r != %r' % (k, before.get(k), other.get(k)) for k in diff) if diff else ''))
        manifest = base / 'tracking.json'
        manifest.write_text(json.dumps(tracked))
        identity = hashlib.sha256(json.dumps(frozen, sort_keys=True).encode()).hexdigest()
        print('# SOURCE SNAPSHOT sha256=' + identity + ' (checkout bytes; explicit live checks remain live)', flush=True)
        env = dict(os.environ, TEST_HW_SNAPSHOT_ROOT=str(candidate),
                   TEST_HW_TRACKING_MANIFEST=str(manifest), TEST_HW_LIVE_ROOT=str(root),
                   TEST_HW_GIT_USABLE='1' if tracked['usable'] else '0',
                   PYTHONDONTWRITEBYTECODE='1')
        # Copies retain their modes: subjects copy these bytes into writable
        # mutant fixtures. Isolation is a private copy, not a claim of a
        # hostile-process sandbox. Verify it again before releasing a headline.
        proc = subprocess.Popen(['bash', str(candidate / 'setup/test-hw')],
                                cwd=candidate, env=env, stdout=subprocess.PIPE,
                                text=True)
        pending = []
        for line in proc.stdout:
            if pending or re.match(r'^(?:[0-9]+ tests passed|NO TRACKED NUMBER)', line):
                pending.append(line)
            else:
                print(line, end='', flush=True)
        rc = proc.wait()
        if frozen != inventory(candidate):
            raise RuntimeError('private candidate changed during execution; no summary is valid')
        if rc == 0:
            print(''.join(pending), end='', flush=True)
        return rc


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, RuntimeError, shutil.Error) as error:
        print('not ok - runner snapshot: ' + str(error), file=sys.stderr)
        if isinstance(error, OSError):
            # Not one of the refusals above: say where it came from.
            import traceback
            traceback.print_exc(file=sys.stderr)
        print('\nRUN ABORTED — no number for this run. See stderr for the failure.')
        sys.exit(1)
