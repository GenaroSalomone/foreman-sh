#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/_common.sh"
python3 - "$(cd "$(dirname "$0")/../.." && pwd)" <<'PY'
import os, pathlib, subprocess, sys, tempfile
root = pathlib.Path(sys.argv[1])
checker = pathlib.Path(os.environ.get('DECISIONS_LINTER', root/'setup/hooks/decisions-check.py'))
with tempfile.TemporaryDirectory() as tmp:
    path = pathlib.Path(tmp)/'decisions.md'
    def entry(title='Current', reverse='none', evidence='12345'):
        return f'## 2026-09-08 — {title}\n**Ruling:** Keep it short.\n**Rules out:** Long entries.\n**Reverses:** {reverse}\n**Evidence:** engram #{evidence}\n'
    def lint(text, *args):
        path.write_text(text)
        return subprocess.run(['python3',str(checker),*args,str(path)],capture_output=True,text=True)
    valid = lint(entry())
    assert valid.returncode == 0, valid.stderr
    # `Reverses:` names its target by ENGRAM ID (as of 2026-09-08 — bin/decisions
    # `entry_id`/`reverses_target`); a title, whether a stranger's ('Absent') or
    # this entry's own ('Current'), is never a resolvable value. The self-
    # reference case is now the SAME id, not the same title.
    for text in [entry().replace('2026-09-08 — ',''),entry().replace('**Evidence:** engram #12345\n',''),entry(reverse='Absent'),entry(reverse='#12345'),entry()+'extra\n'*14]:
        result = lint(text)
        assert result.returncode == 1 and result.stderr, result.stderr
        assert lint(text,'--report-only').returncode == 0
    archives = path.parent/'decisions'
    archives.mkdir()
    (archives/'2026-Q3.md').write_text(entry('Previous', evidence='99999'))
    assert lint(entry(reverse='#99999')).returncode == 0
    assert lint(entry()+'\n```md\n## not an entry\n```\n').returncode == 0
    assert lint(entry()+'extra\n'*13).returncode == 0
    # An id sitting under a malformed (undated) heading is UNKNOWN to
    # bin/decisions' own split()/entry_key, so it must not resolve here either
    # — a linter that let it resolve would pass an entry `decisions supersede`
    # itself refuses ("no entry ... carries that id").
    leak = ('## not a real date whatsoever\n**Ruling:** irrelevant.\n'
            '**Rules out:** irrelevant.\n**Reverses:** none\n'
            '**Evidence:** engram #77777\n\n' + entry(reverse='#77777'))
    assert lint(leak).returncode == 1, 'id only reachable via a malformed heading must not resolve'
    print('ok - 106 strict and report-only lint enforce dated keys size and archived reversals by engram id')
PY
