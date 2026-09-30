#!/usr/bin/env bash
# A RECORDING STAND-IN BINDS WITHOUT WAITING FOR A REVERSE LOOKUP.
#
# Measured 2026-09-30: the first public CI run (36747604152, macos-latest)
# failed 189 with "stand-in: the recording engram never bound a port", and no
# traceback. http.server.HTTPServer.server_bind calls socket.getfqdn on the
# bound address before the stand-in can write its port; locally that lookup
# answers from the resolver in milliseconds (getfqdn("127.0.0.1") returns
# 1.0.0.127.in-addr.arpa, so it DOES go out), on the runner it outlasted the
# 5s wait. Reproduced here by making the lookup slow: the old 189 fails with
# exactly that message. That the runner's resolver is what was slow is the
# inference that fits (no traceback, a silent wait); the lookup is removed, so
# the stand-in no longer depends on it either way.
#
# Each engram stand-in subject (189, and 196 where the tree carries it) runs
# under a python whose gethostbyaddr sleeps 8s and records that it was called.
#
#     bash setup/tests/390-un-stand-in-http-no-espera-al-dns.sh
#
# SUBJECT_TESTS runs the subjects from another setup/tests, for the old/new
# evidence.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

TESTS_DIR="${SUBJECT_TESTS:-$ROOT/setup/tests}"
SLOW="$TMP/slowdns"; mkdir -p "$SLOW"
CALLS="$TMP/reverse-lookups"; : > "$CALLS"
cat > "$SLOW/sitecustomize.py" <<'PY'
import os, socket, time
_real = socket.gethostbyaddr
def _slow(addr):
    with open(os.environ["HW390_CALLS"], "a") as f: f.write(f"{addr}\n")
    time.sleep(8)
    return _real(addr)
socket.gethostbyaddr = _slow
PY

# C01 — the control: the slow resolver is live in a child python.
: > "$CALLS"
HW390_CALLS="$CALLS" PYTHONPATH="$SLOW" python3 -c 'import socket; socket.gethostbyaddr.__name__ == "_slow" or exit(1)' \
  || fail "C01: the slow resolver did not load through PYTHONPATH — every claim below would pass on silence"
pass "C01: a child python under PYTHONPATH gets the slow reverse lookup"

ran=0
for t in 189-la-suite-no-escribe-en-engram.sh 196-mem-save-no-cae-en-manual-save.sh; do
  [ -f "$TESTS_DIR/$t" ] || continue
  ran=$((ran + 1))
  : > "$CALLS"
  rc=0; out="$(HW390_CALLS="$CALLS" PYTHONPATH="$SLOW${PYTHONPATH:+:$PYTHONPATH}" bash "$TESTS_DIR/$t" 2>&1)" || rc=$?
  [ "$rc" -eq 0 ] || fail "$t: under a slow reverse lookup the subject failed (exit $rc): $(printf '%s\n' "$out" | grep -m3 'not ok' || printf '%s' "$out" | tail -n 3)"
  [ ! -s "$CALLS" ] || fail "$t: its stand-in still made a reverse lookup: $(tr '\n' ' ' < "$CALLS")"
  pass "$t: its stand-in binds and answers with no reverse lookup"
done
[ "$ran" -gt 0 ] || fail "no engram stand-in subject found under $TESTS_DIR"
