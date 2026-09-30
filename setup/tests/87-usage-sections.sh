#!/usr/bin/env bash
# Each help section independently preserves its captured literal stdout bytes.
#
# THE GOLDEN IS A SNAPSHOT, SO A DELIBERATE HELP CHANGE UPDATES IT IN THE SAME
# COMMIT, and the fixture diff is the record of what changed. Its value is not
# that the bytes never move -- it is that they never move BY ACCIDENT: a stray
# edit, a reformat, or a section that quietly stopped rendering all fail here.
# Regenerate the same way this file reads it, section by section through the
# `_usage_*` functions with the lane table loaded, never by piping `hw --help`:
#
#     python3 - <<'REGEN'
#     import pathlib, re, subprocess
#     src = pathlib.Path("bin/hw").read_text(); out = ""
#     pre = 'BRAIN=$PWD; . bin/project-spaces.sh; lane_config_load "$BRAIN" || exit 9\n'
#     for n in ['launch','placement','flags','commands','recovery','exits','projects','environment','advanced']:
#         m = re.search(r'^_usage_' + n + r"\(\) \{\n.*?^}", src, re.M | re.S)
#         out += subprocess.run(['bash','-c', pre + m.group() + '\n_usage_' + n],
#                               capture_output=True, text=True).stdout
#     pathlib.Path("setup/fixtures/hw-help-before.txt").write_text(out)
#     REGEN
#
# THE LANE LISTS IN THE HELP ARE READ FROM projects.json (each lane's command,
# its non-default vendor, account and branch, the lane bases, the lanes that
# provision a database or opt into a dev server, the PROJECTS list), so the
# golden is taken against the repository's own table. A lane added there moves
# the golden, and that is the point: the diff shows what the help now says.
#
# Then READ `git diff setup/fixtures/hw-help-before.txt` before committing: if
# it carries a line you did not mean to write, the golden just absorbed drift
# instead of catching it. Last moved 2026-09-30: hw outbox, --sandbox and the brief keys, +22.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
python3 - "${HELP_SUBJECT:-$ROOT/bin/hw}" "$ROOT/setup/fixtures/hw-help-before.txt" ${HELP_TABLE:+"$HELP_TABLE"} <<'PY'
import pathlib, re, subprocess, sys
source = pathlib.Path(sys.argv[1]).read_text()
golden = pathlib.Path(sys.argv[2]).read_text()
root = str(pathlib.Path(sys.argv[1]).resolve().parent.parent)
table = sys.argv[3] if len(sys.argv) > 3 else root + '/projects.json'
pre = ('BRAIN=' + root + '; HW_PROJECTS_JSON=' + table + '; . ' + root + '/bin/project-spaces.sh; '
       'lane_config_load "$BRAIN" || exit 9\n')
names = ['launch','placement','flags','commands','recovery','exits','projects','environment','advanced']
markers = ['PLACEMENT AND ISOLATION', 'FLAGS\n', 'OTHER COMMANDS\n', 'RECOVERY AND CLEANUP\n', 'EXIT STATUS\n', 'PROJECTS\n', 'ENVIRONMENT\n', 'ADVANCED\n']
positions = [0] + [golden.index(marker) for marker in markers] + [len(golden)]
definitions = {}
combined = ''
for name, start, end in zip(names, positions, positions[1:]):
    match = re.search(r'^_usage_' + name + r"\(\) \{\n.*?^}", source, re.M | re.S)
    assert match, 'missing independent usage section: ' + name
    definitions[name] = match.group()
    result = subprocess.run(['bash','-c',pre + match.group() + '\n_usage_' + name], capture_output=True, text=True)
    assert result.returncode == 0 and result.stderr == '', (name,result)
    assert result.stdout == golden[start:end], (name,repr(result.stdout),repr(golden[start:end]))
    combined += result.stdout
    print('ok - isolated usage ' + name + ' preserves captured stdout bytes and exit status')
assert combined == golden
wrapper = re.search(r'^_usage_all\(\) \{\n.*?^}', source, re.M | re.S)
assert wrapper
# On stdin, not in argv: every section at once is over Windows' 32767-character
# command line, and CreateProcess refuses it before bash starts.
# Bytes, so a Windows text pipe cannot turn its newlines into CRLF on the way.
result = subprocess.run(['bash','-s'], input=(pre + '\n'.join(definitions.values()) + '\n' + wrapper.group() + '\n_usage_all\n').encode(), capture_output=True)
result.stdout, result.stderr = result.stdout.decode(), result.stderr.decode()
if not (result.returncode == 0 and result.stderr == '' and result.stdout == golden):
    import difflib
    d = list(difflib.unified_diff(golden.splitlines(), result.stdout.splitlines(), 'golden', 'hw', n=0))[:12]
    raise SystemExit('usage differs from the golden (rc=%s, stderr=%r):\n%s' % (result.returncode, result.stderr[:200], '\n'.join(d)))
PY
