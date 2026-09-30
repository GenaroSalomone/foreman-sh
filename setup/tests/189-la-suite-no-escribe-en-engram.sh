#!/usr/bin/env bash
# THE SUITE MUST NOT WRITE IN THE ENGRAM OF WHOEVER RUNS IT.
#
# `hw` registers an executor's engram session at launch with `POST /sessions`
# on `${HW_ENGRAM_URL:-http://127.0.0.1:${ENGRAM_PORT:-7437}}`. That is an
# ADDRESS: the empty HOME _common.sh gives every subject does not reach it, and
# the HW_* sweep removes HW_ENGRAM_URL, so a subject that got as far as the
# registration posted to the live `engram serve`. A session registered that way
# stays active after its temporary directory is gone, and several active
# sessions for one project make engram refuse every save that names none.
#
# WHAT THIS PROVES is an ABSENCE, so it stands next to a control, as 145 does
# for herdr. A recording stand-in plays the live engram on a free port, reached
# the way the live one is: through an inherited ENGRAM_PORT, with HW_ENGRAM_URL
# unset. C01 runs the real `_engram_register_session`, extracted from bin/hw,
# WITHOUT _common.sh and requires the POST to arrive. The claim runs the same
# driver in a child that sources _common.sh first and requires nothing to
# arrive. If the control does not reproduce, this file fails rather than
# passing on silence.
#
# NOTHING HERE TOUCHES THE REAL ENGRAM: the parent already runs under
# _common.sh, and the stand-in's port is the only engram address any child sees.
#
#     bash setup/tests/189-la-suite-no-escribe-en-engram.sh
#
# `SUBJECT_COMMON` runs the claim against another _common.sh, for the old/new
# evidence.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

COMMON="${SUBJECT_COMMON:-$ROOT/setup/tests/_common.sh}"
FRAG="$TMP/register.sh"
sed -n '/^_engram_register_session() {/,/^}/p' "$ROOT/bin/hw" > "$FRAG"
grep -q 'POST' "$FRAG" || fail "driver: _engram_register_session was not found in bin/hw"

# ── the recording stand-in ──────────────────────────────────────────────────
LOG="$TMP/engram-requests.log"; : > "$LOG"
PORTF="$TMP/engram-port"
python3 - "$LOG" "$PORTF" <<'PY' &
import http.server, socketserver, sys
log, portf = sys.argv[1], sys.argv[2]
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        with open(log, "a") as f: f.write(f"GET {self.path}\n")
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.end_headers()
        self.wfile.write(b'{"status":"ok","instance_id":"i189"}')
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0)).decode()
        with open(log, "a") as f: f.write(f"POST {self.path} {body}\n")
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.end_headers()
        self.wfile.write(b'{"id":"hw-run-189","status":"created"}')
    def log_message(self, *a): pass
# NO REVERSE LOOKUP. HTTPServer.server_bind calls socket.getfqdn on the bound
# address before the port can be written; on the GitHub macOS runner that
# lookup outlasted the whole wait below (setup/tests/390).
class S(http.server.HTTPServer):
    def server_bind(self):
        socketserver.TCPServer.server_bind(self)
        self.server_name, self.server_port = self.server_address[:2]
s = S(("127.0.0.1", 0), H)
open(portf, "w").write(str(s.server_address[1]))
s.serve_forever()
PY
SERVER=$!
trap 'kill "$SERVER" 2>/dev/null || true; wait "$SERVER" 2>/dev/null || true; rm -rf "$TMP"' EXIT
for _ in $(seq 1 100); do [ -s "$PORTF" ] && break; kill -0 "$SERVER" 2>/dev/null || break; sleep 0.1; done
[ -s "$PORTF" ] || fail "stand-in: the recording engram never bound a port$(kill -0 "$SERVER" 2>/dev/null || printf ' (its python exited)')"
PORT="$(<"$PORTF")"

# drive [common] — the launch's registration, with the live engram's address
# inherited the way a person's shell carries it. With an argument, the child
# sources that _common.sh first, as every subject does.
drive() {
  env -u HW_ENGRAM_URL -u ENGRAM_DATA_DIR ENGRAM_PORT="$PORT" HW_BIN_DIR="$ROOT/bin" FRAG="$FRAG" C="${1:-}" bash -c '
    set -euo pipefail
    [ -z "$C" ] || . "$C"
    WT=/fixture/wt; HW_WORKDIR=/fixture/wt; HW_RUN=run-189; ENGRAM_PROJECT=brain
    info() { :; }; warn() { :; }; _receipt() { :; }
    source "$FRAG"
    _engram_register_session
  ' >/dev/null 2>&1
}

# The stand-in plays the person's OWN serve: hw registers only on a serve whose
# /health instance_id is the store's (bin/engram-serve.sh), and with
# ENGRAM_DATA_DIR unset that store is $HOME/.engram.
mkdir -p "$HOME/.engram"; printf 'i189\n' > "$HOME/.engram/.instance-id"

# C01 — the control: without _common.sh the driver reaches the stand-in.
: > "$LOG"
drive || fail "C01: the driver failed outright"
case "$(<"$LOG")" in
  *"POST /sessions"*'"id":"hw-run-189"'*) pass "C01: the control reproduces — without the suite's environment the registration reaches the engram address it inherited" ;;
  *) fail "C01: the control did not reach the stand-in, so the absence below would prove nothing. Log: $(<"$LOG")" ;;
esac

# The claim — under _common.sh nothing arrives.
: > "$LOG"
drive "$COMMON" || fail "claim: the driver failed under _common.sh"
[ ! -s "$LOG" ] || fail "claim: a registration made under _common.sh reached the inherited engram address: $(<"$LOG")"
pass "claim: under _common.sh a launch's engram registration reaches no engram the person runs"

# And the CLI's store is this run's, never an inherited one.
got="$(env ENGRAM_DATA_DIR=/live/.engram bash -c '. "$1"; printf "%s|%s" "$ENGRAM_DATA_DIR" "$TMP"' _ "$COMMON")"
case "$got" in
  /live/*) fail "store: an inherited ENGRAM_DATA_DIR survives _common.sh: ${got%%|*}" ;;
  "${got#*|}"/*) pass "store: ENGRAM_DATA_DIR points inside the subject's own \$TMP" ;;
  *) fail "store: ENGRAM_DATA_DIR is not under the subject's \$TMP: $got" ;;
esac

# And the PORT a subject's own engram (a plugin's auto-started serve, or a
# bare `engram serve`) would bind is never the person's default nor inherited.
got="$(env ENGRAM_PORT=7437 bash -c '. "$1"; printf "%s" "${ENGRAM_PORT:-<unset>}"' _ "$COMMON")"
case "$got" in
  7437|'<unset>') fail "port: under _common.sh ENGRAM_PORT resolves to the person's serve port ($got): a serve a subject starts would take it" ;;
  *) pass "port: under _common.sh ENGRAM_PORT is pinned away from 7437 ($got)" ;;
esac
