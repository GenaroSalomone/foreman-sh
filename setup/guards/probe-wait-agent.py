#!/usr/bin/env python3
"""Drive the complete wait-agent executable through a real local endpoint: a
unix socket, or on Windows the named pipe herdr serves there."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading

sys.path.insert(0, os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "tests"))
from _herdr_endpoint import listen  # noqa: E402

binary = sys.argv[1]

def matched(status='idle', pane='pTEST'):
    return {'type': 'wait_matched', 'event': {'event': 'pane_agent_status_changed',
            'data': {'type': 'pane_agent_status_changed', 'pane_id': pane, 'agent_status': status}}}

def run_case(label, reply, expected, statuses='idle', stdout=''):
    with tempfile.TemporaryDirectory(prefix='wr-') as temporary:
        path = str(Path(temporary) / 's')
        server = listen(path, 4)
        server.settimeout(5)
        errors = []
        def serve():
            try:
                for _ in range(1 + len(statuses.split(','))):
                    conn, _ = server.accept()
                    with conn, conn.makefile('r') as reader:
                        request = json.loads(reader.readline())
                        if request['method'] == 'agent.get':
                            result = {'agent': {'pane_id': 'pTEST'}}
                        else:
                            assert request['method'] == 'events.wait', request
                            status = request['params']['match_event']['agent_status']
                            result = reply(status) if callable(reply) else reply
                        conn.sendall((json.dumps({'id': request['id'], 'result': result}) + '\n').encode())
            except Exception as error:
                errors.append(repr(error))
            finally:
                server.close()
        thread = threading.Thread(target=serve, daemon=True)
        thread.start()
        result = subprocess.run([sys.executable, binary, 'wait-agent', 'probe', statuses,
                                 '--socket', path, '--timeout-ms', '100'], capture_output=True, text=True, timeout=8)
        thread.join(6)
        assert not errors and not thread.is_alive(), (label, errors)
        assert result.returncode == expected, (label, 'exit', result.returncode, result.stdout, result.stderr)
        assert result.stdout == stdout, (label, result.stdout, result.stderr)
        if expected == 4:
            assert 'unproven wait match' in result.stderr, (label, result.stderr)
        print('ok - wait-agent ' + label, flush=True)

run_case('measured matching event remains success', matched(), 0, stdout='idle\n')
for label, payload in [('generic acknowledgment', {'type': 'ok'}), ('null result', None),
                       ('empty result', {}), ('array result', []),
                       ('timeout-shaped result', {'type': 'wait_timed_out'}),
                       ('wrong pane', matched(pane='pOTHER')), ('wrong status', matched('working')),
                       ('empty event', {'type': 'wait_matched', 'event': {}})]:
    run_case(label + ' is not a match', payload, 4)
wrong_event = matched()
wrong_event['event']['event'] = 'pane_output_changed'
run_case('unrelated event is not a match', wrong_event, 4)
wrong_data = matched()
wrong_data['event']['data'] = 'not an object'
run_case('malformed event data is not a match', wrong_data, 4)
run_case('a valid sibling result wins over an unproven status',
         lambda status: matched('done') if status == 'done' else {'type': 'ok'},
         0, statuses='idle,done', stdout='done\n')
