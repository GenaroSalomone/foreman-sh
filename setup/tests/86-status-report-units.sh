#!/usr/bin/env bash
# Each extracted renderer runs alone; no other report may be invoked.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
python3 - "$ROOT/bin/hw" <<'PY'
import ast, contextlib, glob, io, json, os, re, sys, time
source = open(sys.argv[1]).read()
providers = re.findall(r"^_status_\w+_source\(\) \{\n  cat <<'HW_STATUS_PY'\n(.*?)\nHW_STATUS_PY\n}", source, re.M | re.S)
assert providers, 'missing isolated status report providers'
tree = ast.parse('\n'.join(providers))
definitions = [n for n in tree.body if isinstance(n, ast.FunctionDef)]
colors = dict(zip(('OK','WARN','ERR','DIM','B','Z'), ('\033[32m','\033[33m','\033[31m','\033[2m','\033[1m','\033[0m')))
names = ['spaces','tasks','cleanup','orphans','caveats']
def run(name, args, configure=None):
    ns = dict(glob=glob, json=json, os=os, re=re, time=time, **colors)
    ns['SKIP_DIRS'] = {'.hw','.git','node_modules','.venv','.next','dist','__pycache__'}
    exec(compile(ast.Module(body=definitions, type_ignores=[]), '<status-units>', 'exec'), ns)
    def forbidden(*_):
        raise AssertionError('another report ran during isolated ' + name)
    for other in names:
        if other != name: ns['status_report_' + other] = forbidden
    if configure: configure(ns)
    output = io.StringIO()
    with contextlib.redirect_stdout(output):
        result = ns['status_report_' + name](*args)
    return output.getvalue(), result

out, _ = run('spaces', ([{'workspace_id':'w1','label':'setup'},{'workspace_id':'w2','label':'setup:legacy'}], True))
assert out.startswith('\n\033[1mspaces\033[0m\n  w1  setup\n'), repr(out)
assert 'w2  setup:legacy' in out and 'DRIFT' in out
assert run('spaces', ([], False))[0] == '\n\033[1mspaces\033[0m\n  \033[2m·\033[0m unknown — herdr unreachable\n'
print('ok - isolated status spaces preserves headings, order, drift and unreachable output')

args = ({},{},{},{},{},{},None,True)
out, result = run('tasks', args)
assert out == '\n\033[1mtasks\033[0m\n  \033[2m·\033[0m no work directories, no worktrees, no task panes\n', repr(out)
assert result == (set(), [], []), repr(result)
os.environ['HW_STATUS_ALL'] = '1'
args = ({'setup:quiet':{'project':'setup','task':'quiet','workdir':None}},{},{},{},{},{},None,True)
out, result = run('tasks', args)
assert 'setup:quiet' in out and 'no information' in out, repr(out)
assert isinstance(result[0], set) and result[1:] == ([], []), repr(result)
print('ok - isolated status tasks renders empty and historical rows and returns report queues')

assert run('cleanup', ([],))[0] == ''
out, _ = run('cleanup', ([{'key':'setup:done','kind':'tab','id':'w1:t2','panes':['w1:p3']}],))
assert 'reported done, not closed' in out and 'hw done setup done' in out and 'w1:p3' in out
assert 'finished, never reported' not in out
print('ok - isolated status cleanup renders only the durable cleanup queue')

assert run('orphans', ([], []))[0] == ''
seen = []
def configure(ns):
    ns['orphan_why'] = lambda o, panes: seen.append(panes) or 'measured reason'
    ns['shape_of'] = lambda path: '3 artifacts'
orphan = dict(key='setup:lost',at=None,turns=1,live_panes=[],exec_pane='w1:p9',workdir='/fixture',session_id='',session_vendor='',session_resume='')
out, _ = run('orphans', ([orphan],[{'pane_id':'w1:p1'},{'pane_id':'w2:p2'}]), configure)
assert seen == [{'w1:p1','w2:p2'}], seen
assert 'finished, never reported' in out and 'measured reason' in out and '3 artifacts in /fixture' in out
print('ok - isolated status orphans uses the full pane set and preserves recovery guidance')

out, _ = run('caveats', (False, set()))
assert 'herdr is unreachable' in out and 'restarting herdr?' in out and out.endswith('\n\n')
out, _ = run('caveats', (True, {'exit','session','orphan','turn'}))
assert 'what this cannot tell you' in out and 'pane_exited' in out and '`hw wait`' in out
assert out.index('what this cannot tell you') < out.index('restarting herdr?')
print('ok - isolated status caveats preserves all limitations and the unconditional footer')
PY
