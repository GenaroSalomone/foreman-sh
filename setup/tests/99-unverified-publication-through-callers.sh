#!/usr/bin/env bash
# Execute complete invokers through publication AND every delivery outcome.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
python3 - "${TIER1_CALLER_SOURCE_ROOT:-$ROOT}" "$TMP" <<'PY'
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

from concurrent.futures import ThreadPoolExecutor

source, temporary = map(Path, sys.argv[1:])
base_env = {k:v for k,v in os.environ.items() if not k.startswith('HW_')}
outcomes = [('herdr', n) for n in (1,2,3,4,5,6,0)] + [('opencode', n) for n in (1,2,3,5,6,0,4)]

# THE 84 CASES RUN FOUR AT A TIME AND ARE REPORTED IN ORDER. Each case owns
# its directory, its bin/ copy, its TMPDIR and its environment, so none can see
# another. The per-case 10s cap is unchanged, and the first failing case in the
# original order is the one reported.
#
# THE STUBS ARE WRITTEN ONCE AND LINKED INTO EVERY CASE. They are the same
# bytes in all 84 cases — each reads CASE, PUBLICATION and DELIVERY from its
# environment — and on this machine the FIRST exec of a newly written file
# costs 1.2-2s (measured 2026-09-23; a link to a file already executed costs
# 3ms). Written per case, that was four first-execs a case and most of this
# file's wall clock. Each is executed once here, so every link is warm.
scripts = {
    'channel-send': '#!/bin/bash\nprintf "%s\\n" "$*" > "$CASE/channel"\nexit "$DELIVERY"\n',
    'hw': '#!/bin/bash\nprintf close > "$CASE/closed"\n',
    'herdr': '#!/bin/bash\nprintf "%s\\n" "$*" >> "$CASE/herdr"\nprintf \'{"result":{}}\\n\'\n',
    'herdr-rpc': '''#!/bin/bash
case "$2" in
  pane.report_metadata) printf '%s' "$3" > "$CASE/metadata"; printf '{"type":"ok"}\n' ;;
  agent.list)
    case "$PUBLICATION" in
      read-error) exit 3 ;;
      mismatch) printf '{"agents":[]}\n' ;;
      verified) jq '{agents:[{pane_id:.pane_id,tokens:(.tokens | with_entries(select(.value != null)))}]}' "$CASE/metadata" ;;
    esac ;;
esac
'''}
STUBS = tuple(scripts)
stubs = temporary / 'stubs'
stubs.mkdir()
for name, body in scripts.items():
    (stubs / name).write_text(body)
    (stubs / name).chmod(0o755)
    warm = temporary / 'stubs-warm'
    warm.mkdir(exist_ok=True)
    subprocess.run([str(stubs / name)], env=dict(base_env, CASE=str(warm), DELIVERY='0', PUBLICATION='mismatch'),
                   capture_output=True, timeout=30 * int(os.environ.get('HW_TEST_SLOW', '1')))

def run_case(publication, caller, route, delivery):
            label = f'{caller}/{publication}/{route}/{delivery}'
            case = temporary / label
            binary = case / 'bin'
            state = case / 'work/.hw/run'
            artifacts = case / 'artifacts'
            binary.mkdir(parents=True)
            state.mkdir(parents=True)
            artifacts.mkdir()
            # ITS OWN TMPDIR, because the invoker's run lock lives there
            # ($TMPDIR/hw-invoker-<run>.lock) and every case is HW_RUN=run: a
            # shared TMPDIR serialises the cases on that lock, and makes this
            # file contend with any other subject using the same run name.
            tmp = case / 'tmp'
            tmp.mkdir()
            (state / 'ask-count').write_text('1')
            for name in ('ask-invoker', 'done-invoker', 'invoker-common.sh', 'state-witness.sh'):
                shutil.copy2(source / 'bin' / name, binary / name)
            for name in STUBS:
                os.link(stubs / name, binary / name)
            env = dict(base_env, PATH=str(binary)+':'+base_env['PATH'], CASE=str(case),
                       PUBLICATION=publication, DELIVERY=str(delivery), HW_INVOKER_PANE='pB',
                       HERDR_PANE_ID='pE', HW_TASK='probe', HW_PROJECT='setup', HW_RUN='run',
                       HW_WORKDIR=str(case/'work'), HW_ARTIFACTS=str(artifacts),
                       HW_EXECUTOR_VENDOR='opencode' if route == 'opencode' else 'claude',
                       HW_INVOKER_VENDOR='opencode' if route == 'opencode' else 'claude',
                       HW_INVOKER_SESSION='ses_fixture', HW_INVOKER_ENDPOINT='http://127.0.0.1:1',
                       TMPDIR=str(tmp))
            result = subprocess.run(['bash', str(binary/caller), 'Probe message; evidence #1'], env=env,
                                    cwd=case/'work', capture_output=True, text=True, timeout=10 * int(os.environ.get('HW_TEST_SLOW', '1')))
            output = result.stdout + result.stderr
            assert (case/'channel').is_file(), (label, 'caller never reached delivery', output)
            success = delivery == 0 or (route == 'opencode' and delivery == 4)
            # channel-send exit 5 (landing unconfirmed) is the invokers' own exit 5,
            # on every route: collapsed into 1 it read as "not delivered, retry".
            assert result.returncode == (0 if success else 5 if delivery == 5 else 1), (label, result.returncode, output)
            if not success:
                note = ('Metadata was verified by read-back before the delivery attempt.' if publication == 'verified'
                        else 'Metadata publication could not be established; do not rely on a token record on this pane.')
                assert note in output, (label, 'caller lost publication evidence', output)
                if publication != 'verified':
                    notices = (case/'herdr').read_text()
                    assert not re.search(r'remains recorded|[Ii][Ss] recorded on this pane|question published on', output+notices), (label, 'false durable record claim', output, notices)
                assert not (state/'done').exists() and not (case/'closed').exists(), (label, 'failure marked or closed task')
                assert (state/'ask-count').read_text() == '1', (label, 'failure consumed ask')
            elif caller == 'ask-invoker':
                assert (state/'ask-count').read_text() == '2', (label, 'delivered ask not counted')
            else:
                assert (state/'done').is_file() and (case/'closed').is_file(), (label, 'delivered report not finalized')
            return 'ok - full caller '+label+' preserves publication and delivery facts'

cases = [(publication, caller, route, delivery)
         for publication in ('mismatch', 'read-error', 'verified')
         for caller in ('ask-invoker', 'done-invoker')
         for route, delivery in outcomes]
with ThreadPoolExecutor(max_workers=4) as pool:
    futures = [pool.submit(run_case, *c) for c in cases]
    for future in futures:
        print(future.result(), flush=True)
PY
