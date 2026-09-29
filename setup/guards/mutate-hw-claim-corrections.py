#!/usr/bin/env python3
"""Run bounded claim regressions on disposable copies of a committed tree."""
import io
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile

repo = Path(__file__).resolve().parents[2]
ref = sys.argv[1] if len(sys.argv) == 2 else 'HEAD'
sha = subprocess.check_output(['git', '-C', str(repo), 'rev-parse', ref + '^{commit}'], text=True).strip()
archive = subprocess.check_output(['git', '-C', str(repo), 'archive', sha, 'bin', 'setup/tests', 'setup/fixtures'])
env = {key: value for key, value in os.environ.items() if not key.startswith(('HW_', 'GIT_'))}

def replace_once(text, old, new):
    assert text.count(old) == 1, ('mutation anchor count', old, text.count(old))
    return text.replace(old, new, 1)

dry = '      info "SDD surface: unchanged — compatibility checked using the effective mode; live surface takes precedence, with the launch record as fallback when no live mode is obtained"'
mutations = [
    ('alias-rejection', '01-task-names-and-models.sh',
     lambda s: replace_once(s, 'warn "claude model', 'die "MUTANT rejects unknown alias: claude model'),
     'MUTANT rejects unknown alias'),
    ('recognition-as-proof', '97-recognition-is-not-runtime-readiness.sh',
     lambda s: re.sub(r"true:idle\|true:working\|true:done\) printf .*? ;;", "true:idle|true:working|true:done) printf 'yes' ;;", s, count=1),
     'recognition became runtime proof for idle: yes'),
    ('unknown-receipt-as-success', '97-recognition-is-not-runtime-readiness.sh',
     lambda s: replace_once(s, 'unknown*|unreadable) warn "UNESTABLISHED  agent: $AGENT ($m)"; unestablished=1 ;;', 'unknown*|unreadable) : ;;'),
     'receipt: every measured value matches'),
    ('fallback-as-live-measurement', '75-codex-framework-refusal.sh',
     lambda s: replace_once(s, dry, '      if [ -f "$FM" ]; then\n' + dry + '\n      else\n        info "SDD surface: unchanged — measured the effective live worktree surface"\n      fi'),
     'next fallback dry run lacks an honest compatibility explanation'),
    ('prefix-bypasses-argv-safety', '01-task-names-and-models.sh',
     lambda s: replace_once(s, '        case "$MODEL" in\n          -*|', '        case "$MODEL" in\n          claude-*) ;;\n          -*|'),
     'model: unknown alias claude-future --resume X cannot expand'),
    ('help-promises-launch-mode', '75-codex-framework-refusal.sh',
     lambda s: replace_once(s, '        effective live worktree surface is measured and incompatible vendor/mode', '        executor keeps whatever mode it was launched with'),
     'next help does not describe the measured live-surface precedence'),
    ('launch-announces-ready', '97-recognition-is-not-runtime-readiness.sh',
     lambda s: replace_once(s, '    info "$AGENT registered in $pane; vendor runtime readiness is unestablished"', '    ok "$AGENT ready in $pane"'),
     'claude ready in pTEST'),
]
with tempfile.TemporaryDirectory(prefix='hw-claims-mutants-') as temporary:
    for label, subject, mutate, witness in [('baseline', None, None, None)] + mutations:
        root = Path(temporary) / label
        root.mkdir()
        with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
            # Git-generated archive from the explicitly selected trusted commit.
            tar.extractall(root)
        binary = root / 'bin/hw'
        if mutate:
            original = binary.read_text()
            changed = mutate(original)
            assert changed != original, ('mutation did not apply', label)
            binary.write_text(changed)
        subjects = [subject] if subject else sorted({row[1] for row in mutations})
        for name in subjects:
            result = subprocess.run(['bash', str(root / 'setup/tests' / name)], cwd=root,
                                    env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            if not mutate:
                assert result.returncode == 0, (name, result.stdout)
            else:
                assert result.returncode != 0 and witness in result.stdout, (label, result.returncode, result.stdout)
        print(('KILLED ' if mutate else 'PASS ') + label + ' @ ' + sha, flush=True)
