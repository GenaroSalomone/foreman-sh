#!/usr/bin/env python3
"""Prove tier-1 evidence checks with isolated, post-commit executable mutants."""
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
archive = subprocess.check_output(['git', '-C', str(repo), 'archive', sha, 'bin', 'setup/tests', 'setup/guards', 'setup/fixtures'])
old_hw = subprocess.check_output(['git', '-C', str(repo), 'show', 'b879de0:bin/hw'], text=True)
env = {k: v for k, v in os.environ.items() if not k.startswith(('HW_', 'GIT_'))}

def once(s, old, new):
    assert s.count(old) == 1, ('anchor count', old, s.count(old))
    return s.replace(old, new, 1)

def old_process_gate(s):
    pattern = r'  local i process .*?\n  \[ "\$has_opencode" = 0 \].*?\n'
    old = re.search(pattern, old_hw, re.S).group()
    return once(s, re.search(pattern, s, re.S).group(), old)

def weak_admission(s):
    body = re.search(r'^_codex_admit\(\) \{\n.*?^}', s, re.M | re.S).group()
    return once(s, body, body.rsplit('  return 1', 1)[0] + '  return 0\n}')

mutations = [
    ('wait-ack', 'bin/herdr-rpc', '98-wait-agent-requires-a-matching-event.sh',
     lambda s: re.sub(r'            # A successful RPC exchange.*?(?=        except RpcError as e:)', '            results.put(("ok", status, res))\n', s, count=1, flags=re.S),
     'generic acknowledgment is not a match'),
    ('wait-pane', 'bin/herdr-rpc', '98-wait-agent-requires-a-matching-event.sh',
     lambda s: once(s, 'data.get("pane_id") == pane_id', 'True'), 'wrong pane is not a match'),
    ('wait-status', 'bin/herdr-rpc', '98-wait-agent-requires-a-matching-event.sh',
     lambda s: once(s, 'data.get("agent_status") == status', 'True'), 'wrong status is not a match'),
    ('unstick-failed-read', 'bin/hw', '22-hw-unstick.sh', old_process_gate,
     'restarted without observing process exit (stop-read-error)'),
    ('publish-ack', 'bin/invoker-common.sh', '99-publish-requires-readable-tokens.sh',
     lambda s: once(s, '  observed="$("$INVOKER_RPC" call agent.list', '  return 0\n  observed="$("$INVOKER_RPC" call agent.list'),
     'metadata ack became token proof for read-error: exit=0'),
    ('publish-pane', 'bin/invoker-common.sh', '99-publish-requires-readable-tokens.sh',
     lambda s: once(s, 'select(.pane_id == $pane)', 'select(true)'), 'metadata ack became token proof for wrong-pane: exit=0'),
    ('publish-deletion', 'bin/invoker-common.sh', '99-publish-requires-readable-tokens.sh',
     lambda s: once(s, '($actual | has($item.key)) | not', 'true'), 'metadata ack became token proof for undeleted: exit=0'),
    ('codex-weak-admission', 'bin/hw', '80-codex-brief-delivery.sh', weak_admission,
     'existing Codex thread became proof of this delivery'),
    ('codex-late-dialog', 'bin/hw', '80-codex-brief-delivery.sh',
     lambda s: once(s, '*agent_prompt_stalled*)\n      if [ "$pane_vendor" = codex ]; then', '*agent_prompt_stalled*)\n      if false; then'),
     'a newly appeared Codex dialog received a replay'),
    ('codex-receipt-fabrication', 'bin/hw', '80-codex-brief-delivery.sh',
     lambda s: once(s, '  codex_unproven_reason="$reason"', '  if [ "${r_rc:-0}" != 0 ]; then\n    _receipt brief_admitted "unproven — herdr measured a turn" "mutant fabricated observation"\n  fi\n  codex_unproven_reason="$reason"'),
     'replay wrote duplicate admission rows (reused/1)'),
    ('ask-caller-record-claim', 'bin/ask-invoker', '99-unverified-publication-through-callers.sh',
     lambda s: once(s, 'PUBLICATION_NOTE="$(invoker_publication_note "$PUB_RC")"', 'PUBLICATION_NOTE="$(invoker_publication_note "$PUB_RC")"\n[ "$PUB_RC" = 0 ] || PUBLICATION_NOTE="The question remains recorded on this pane."'),
     'The question remains recorded on this pane.'),
    ('done-caller-record-claim', 'bin/done-invoker', '99-unverified-publication-through-callers.sh',
     lambda s: once(s, 'PUBLICATION_NOTE="$(invoker_publication_note "$PUB_RC")"', 'PUBLICATION_NOTE="$(invoker_publication_note "$PUB_RC")"\n[ "$PUB_RC" = 0 ] || PUBLICATION_NOTE="The completion remains recorded on this pane."'),
     'The completion remains recorded on this pane.'),
]
with tempfile.TemporaryDirectory(prefix='tier1-mutants-') as temporary:
    for label, target, subject, mutate, witness in [('baseline', None, None, None, None)] + mutations:
        root = Path(temporary) / label
        root.mkdir()
        with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
            tar.extractall(root)
        if mutate:
            path = root / target
            source = path.read_text()
            changed = mutate(source)
            assert changed != source, ('no mutation', label)
            path.write_text(changed)
        for name in [subject] if subject else sorted({m[2] for m in mutations}):
            run = subprocess.run(['bash', str(root / 'setup/tests' / name)], cwd=root, env=env,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            assert (run.returncode != 0 and witness in run.stdout) if mutate else run.returncode == 0, (label, run.returncode, run.stdout)
        print(('KILLED ' if mutate else 'PASS ') + label + ' @ ' + sha, flush=True)
