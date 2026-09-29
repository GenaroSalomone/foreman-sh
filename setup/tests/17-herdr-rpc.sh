#!/usr/bin/env bash
# herdr-rpc wait-agent: target resolution, and a hang-up that is not a dead socket
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/17-herdr-rpc.sh
#
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── herdr-rpc wait-agent: the target is resolved, and a hang-up is not ──────
#         called a dead socket
#
# BOTH OF THESE WERE MEASURED BROKEN on 2026-08-25, against live herdr 0.8.2.
# `bin/herdr-rpc wait-agent probe idle,done` — an agent NAME, which every
# `herdr agent <verb>` accepts and which channel-send's own usage line invites
# with "<session-or-pane>" — died three times out of three with
#   herdr-rpc: cannot connect: herdr closed the connection
# and exit 3. The same call with the pane id behind that name exited 0. Nothing
# was wrong with the socket: events.wait matches on a pane_id only, herdr
# answers an absent pane_id by HANGING UP rather than returning an error, and
# the collector's failure branch then announced the socket as the cause.
# channel-send relays exit 3 as "could not wait on $TARGET", so the operator is
# told the server is down when the server is fine.
#
# The fake server below answers events.wait ONLY for the pane id that agent.get
# resolves to, and hangs up for anything else. That is what makes the first test
# real rather than decorative: delete the resolve step and the request carries
# the name as a pane_id, the server hangs up, and the test fails.
hr="$TMP/hrpc"; mkdir -p "$hr"
cat > "$hr/fake-herdr.py" <<'FAKEHERDR'
import json, os, socket, sys, threading
sys.path.insert(0, os.environ["HW_TEST_PYLIB"])
from _herdr_endpoint import listen

sock_path, mode = sys.argv[1], sys.argv[2]
RESOLVED = "wZ:p9"

def handle(conn):
    f = conn.makefile("r", encoding="utf-8", newline="\n")
    while True:
        line = f.readline()
        if not line:
            return
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            return
        rid, method = req.get("id"), req.get("method")
        params = req.get("params") or {}

        if method == "agent.get":
            if mode == "hangup-resolve":
                conn.close()
                return
            elif mode == "notfound":
                body = {"id": rid, "error": {"code": "agent_not_found",
                        "message": "agent target %s not found" % params.get("target")}}
            else:
                body = {"id": rid, "result": {"agent": {"pane_id": RESOLVED,
                        "agent_status": "working"}}}
            conn.sendall((json.dumps(body) + "\n").encode())
            continue

        if method == "events.wait":
            asked = (params.get("match_event") or {}).get("pane_id")
            if mode == "hangup" or asked != RESOLVED:
                conn.close()          # exactly what herdr does for an absent pane
                return
            body = {"id": rid, "result": {"type": "wait_matched", "event": {
                    "event": "pane_agent_status_changed", "data": {
                    "type": "pane_agent_status_changed", "pane_id": asked,
                    "agent_status": params["match_event"]["agent_status"]}}}}
            conn.sendall((json.dumps(body) + "\n").encode())
            continue

        conn.sendall((json.dumps({"id": rid, "error": {"code": "unknown_method",
                      "message": method or ""}}) + "\n").encode())

srv = listen(sock_path)
sys.stderr.write("ready\n"); sys.stderr.flush()
while True:
    c, _ = srv.accept()
    threading.Thread(target=handle, args=(c,), daemon=True).start()
FAKEHERDR

# SETS GLOBALS, PRINTS NOTHING. Two earlier shapes of this helper were wrong in
# ways worth naming, because both are the file's own subject matter:
#   * `local mode="$1" sp="$hr/$mode.sock"` — bash 3.2 does not see `mode` in
#     that same statement and dies under `set -u`. It works in zsh.
#   * returning the path via `sp="$(hr_start ok)"` — a command substitution
#     blocks until every writer to the inherited stdout closes it, and a server
#     never exits, so the suite HUNG instead of failing. And `HR_PID=$!` inside
#     a substitution is set in the subshell, so hr_stop never saw it.
hr_start() { # mode -> sets HR_SOCK and HR_PID
  HR_MODE="$1"
  HR_SOCK="$hr/$HR_MODE.sock"
  python3 "$hr/fake-herdr.py" "$HR_SOCK" "$HR_MODE" >/dev/null 2>&1 &
  HR_PID=$!
  local i=0
  while [ ! -e "$HR_SOCK" ]; do i=$((i+1)); [ "$i" -gt 100 ] && break; sleep 0.05; done
}
hr_stop() { kill "$HR_PID" 2>/dev/null || true; wait "$HR_PID" 2>/dev/null || true; }

hr_start ok
rc=0; out="$("$ROOT/bin/herdr-rpc" wait-agent some-agent-name idle --socket "$HR_SOCK" --timeout-ms 3000 2>&1)" || rc=$?
[ "$rc" = 0 ] || fail "herdr-rpc: an agent NAME still does not resolve (exit $rc): $out"
[ "$out" = idle ] || fail "herdr-rpc: resolved but did not print the matched status: $out"
pass "herdr-rpc: wait-agent resolves an agent name to its pane before waiting"
hr_stop

hr_start notfound
rc=0; out="$("$ROOT/bin/herdr-rpc" wait-agent ghost idle --socket "$HR_SOCK" --timeout-ms 3000 2>&1)" || rc=$?
case "$rc:$out" in
  3:*|*"cannot connect"*) fail "herdr-rpc: an unresolvable target is still blamed on the socket: $out" ;;
  4:*agent_not_found*)    pass "herdr-rpc: an unresolvable target names agent_not_found, not the socket" ;;
  *) fail "herdr-rpc: unexpected verdict for an unresolvable target (exit $rc): $out" ;;
esac
hr_stop

hr_start hangup-resolve
rc=0; out="$("$ROOT/bin/herdr-rpc" wait-agent arbitrary-target idle --socket "$HR_SOCK" --timeout-ms 3000 2>&1)" || rc=$?
[ "$rc" = 4 ] || fail "herdr-rpc: an agent.get hang-up is not exit 4 (exit $rc): $out"
case "$out" in
  *"cannot connect"*) fail "herdr-rpc: an agent.get hang-up is still blamed on the socket: $out" ;;
  *"hung up"*"server is up"*"resolution"*"failed"*) pass "herdr-rpc: an agent.get hang-up says the server is up but target resolution failed" ;;
  *) fail "herdr-rpc: an agent.get hang-up does not explain the failed target resolution: $out" ;;
esac
hr_stop

hr_start hangup
rc=0; out="$("$ROOT/bin/herdr-rpc" wait-agent some-agent-name idle --socket "$HR_SOCK" --timeout-ms 3000 2>&1)" || rc=$?
case "$rc:$out" in
  3:*|*"cannot connect"*) fail "herdr-rpc: a mid-request hang-up is still reported as a dead socket: $out" ;;
  4:*"hung up"*)          pass "herdr-rpc: a mid-request hang-up is reported as itself, not as a dead socket" ;;
  *) fail "herdr-rpc: unexpected verdict for a hang-up (exit $rc): $out" ;;
esac
hr_stop

# `rc=0; out=... || rc=$?` and not `out=...; rc=$?`: under `set -e` a failing
# command substitution aborts the script before rc is ever read. Written the
# wrong way first, and the suite died with exit 4 and no `not ok` line — the
# same shape as the nine assertions that killed this suite before their own
# check on 2026-08-24.
# The negative case for all three: a socket that really is absent MUST still be
# exit 3, or the distinction above bought nothing.
rc=0; out="$("$ROOT/bin/herdr-rpc" wait-agent some-agent-name idle --socket "$hr/absent.sock" --timeout-ms 1000 2>&1)" || rc=$?
case "$rc:$out" in
  3:*"cannot connect"*) pass "herdr-rpc: a genuinely absent socket is still exit 3, cannot connect" ;;
  *) fail "herdr-rpc: an absent socket no longer reports as unreachable (exit $rc): $out" ;;
esac
