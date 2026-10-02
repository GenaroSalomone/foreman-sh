#!/usr/bin/env bash
# A TEST RUN MUST NOT WRITE ON A PANE THAT IS DOING REAL WORK.
#
# THE INCIDENT, MEASURED 2026-09-15. Closing a task, `hw done` printed:
#
#     ! report tokens on w7G:p8Z belong to run 20260911-100000-1,
#       not 20260914-171937-6546; receipt unchanged
#
# `20260911-100000-1` names no run on this disk. It is a FIXTURE CONSTANT, used
# by 130, 133, 134 and 136. The suite had written it onto a working brainer's
# pane.
#
# HOW IT GOT THERE, and every link is individually correct:
#   * 133 supplies HW_PROJECT/HW_TASK/HW_RUN/HW_WORKDIR as fixture values and
#     calls `hw executor-turn-end` -- those are its subject matter;
#   * `_executor_turn_publish` (bin/hw) gates on HERDR_ENV=1 and a non-empty
#     HERDR_PANE_ID and publishes the turn record onto exactly that pane -- right,
#     because an executor is the thing whose turn ended;
#   * _common.sh swept HW_* and two names outside it, and never touched the
#     HERDR_* family, so the pane identity of whoever RAN the suite walked
#     straight into the fixture.
# Neither side knows about the other. The result was a test suite mutating
# production state.
#
# AND A $PATH STUB DOES NOT REACH IT. _common.sh stubs `herdr` and `herdr-rpc`
# on $PATH, which is why this looked hermetic. Every caller that matters invokes
# herdr-rpc by ABSOLUTE PATH -- `$HW_BIN_DIR/herdr-rpc` (bin/hw),
# `$INVOKER_BIN_DIR/herdr-rpc` (bin/invoker-common.sh), `$BIN_DIR/herdr-rpc`
# (bin/channel-send, bin/hw-reconcile), `$SCRIPT_DIR/herdr-rpc`
# (bin/codex-channel-session-hook.sh) -- because herdr-rpc is deliberately not on
# $PATH in production. A $PATH entry cannot shadow a path already written out,
# and those files are not this directory's to edit. So the interception is at the
# ADDRESS: the identity is swept, and $HERDR_SOCKET_PATH is re-pointed into the
# run's own $TMP.
#
# WHAT THIS SUBJECT PROVES, and it is an ABSENCE, which is the hard shape:
# a subject-shaped driver, run under an environment that is exactly a live pane's,
# reaches a RECORDING STAND-IN for the herdr socket and leaves NOTHING on it.
# An absence is only evidence next to a control, so C01 runs the identical driver
# against a harness without the sweep and REQUIRES the write to appear. If the
# control does not reproduce, this file fails rather than passing on silence.
#
# NOTHING HERE TOUCHES THE REAL DAEMON. The socket is a python stand-in bound in
# a throwaway directory; the pane id is invented; the real $HERDR_SOCKET_PATH is
# already gone by the time this file's first line runs. Run it alone:
#
#     bash setup/tests/145-a-test-run-does-not-write-on-a-live-pane.sh
#
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

CLAIMS=0
claim() { CLAIMS=$((CLAIMS + 1)); pass "$1"; }

# ── the recording stand-in ──────────────────────────────────────────────────
#
# It speaks what herdr-rpc speaks -- newline-delimited JSON, one response per
# request -- and appends every request line to a log. It is a WITNESS, never a
# stub the code under test is supposed to be satisfied by: the claim below is
# about what arrives here, not about what the caller made of the answer.
#
# BOUND IN ITS OWN SHORT DIRECTORY, not in $TMP. AF_UNIX paths are capped near
# 104 bytes and $TMPDIR on macOS is a long /var/folders/... path, so a socket
# under $TMP fails to bind with "AF_UNIX path too long" -- measured here.
REC_DIR="$(mktemp -d /tmp/hw-pane.XXXXXX)"
REC_SOCK="$REC_DIR/s"
REC_LOG="$REC_DIR/log"
REC_PID=""
trap 'if [ -n "$REC_PID" ]; then kill "$REC_PID" 2>/dev/null || true; fi; rm -rf "$REC_DIR" "$TMP"' EXIT

cat > "$REC_DIR/recorder.py" <<'PY'
import json, os, socket, sys, threading
sys.path.insert(0, os.environ["HW_TEST_PYLIB"])
from _herdr_endpoint import listen

sock_path, log_path = sys.argv[1], sys.argv[2]
srv = listen(sock_path, 64)
lock = threading.Lock()

def serve(conn):
    f = conn.makefile("r", encoding="utf-8", newline="\n")
    for raw in f:
        raw = raw.strip()
        if not raw:
            continue
        with lock:
            with open(log_path, "a", encoding="utf-8") as lg:
                lg.write(raw + "\n")
                lg.flush()
        try:
            req = json.loads(raw)
        except ValueError:
            continue
        try:
            conn.sendall((json.dumps({"id": req.get("id"), "result": {}}) + "\n").encode("utf-8"))
        except OSError:
            break
    conn.close()

while True:
    c, _ = srv.accept()
    threading.Thread(target=serve, args=(c,), daemon=True).start()
PY

python3 "$REC_DIR/recorder.py" "$REC_SOCK" "$REC_LOG" >/dev/null 2>&1 &
REC_PID=$!
# DISOWNED so the shell does not print "Terminated: 15" over the last assertion
# when the trap kills it. It is still ours to kill: $REC_PID outlives the job
# table entry.
disown %% 2>/dev/null || true
# BOUNDED, and it fails LOUDLY. A recorder that never bound would make every
# absence claim below pass on nothing.
_waited=0
while [ ! -e "$REC_SOCK" ]; do
  _waited=$((_waited + 1))
  [ "$_waited" -le 100 ] || fail "the recording socket never bound at $REC_SOCK — every claim in this file would have passed on silence"
  sleep 0.1
done
kill -0 "$REC_PID" 2>/dev/null || fail "the recorder died before the first case"

# ── the environment a live pane actually has ────────────────────────────────
#
# Taken from a real executor's own environment, with the ids invented and one
# name nobody has coined. HERDR_FUTURE_KNOB is what separates a prefix sweep
# from a fourth round of the enumeration bug 54 documents: an allowlist cannot
# know it.
LIVE_PANE=wLIVE:pRUNNER
pollute() { # pollute <command...>
  env HERDR_ENV=1 \
      HERDR_PANE_ID="$LIVE_PANE" \
      HERDR_SOCKET_PATH="$REC_SOCK" \
      HERDR_TAB_ID=wLIVE:t1 \
      HERDR_WORKSPACE_ID=wLIVE \
      HERDR_BIN_PATH=/nonexistent/herdr \
      HERDR_FUTURE_KNOB=1 \
      "$@"
}

# ── the driver: a miniature subject ─────────────────────────────────────────
#
# It does what every subject file does and nothing else: source a harness, build
# a fixture, drive a binary. `hw executor-turn-end` is the binary the incident
# came through, and the fixture is 133's own run id -- the constant that appeared
# on the live pane.
#
# THE MARKER IS WHAT MAKES THE ABSENCE READABLE. Written after the call returns,
# it separates "the harness stopped the write" from "the driver never ran", which
# an empty log cannot tell apart on its own.
FIXTURE_RUN=20260911-100000-1
cat > "$TMP/driver.sh" <<'DRV'
#!/usr/bin/env bash
. "$1"                       # the harness under test, by absolute path
wd="$TMP/fixture"
mkdir -p "$wd/.hw/$FIXTURE_RUN"
printf '1\n' > "$wd/.hw/$FIXTURE_RUN/task"
HW_TASK=fixture-task HW_PROJECT=setup HW_RUN="$FIXTURE_RUN" HW_WORKDIR="$wd" \
  "$ROOT/bin/hw" executor-turn-end --hook-json >/dev/null 2>&1 || true
printf 'DRIVER-REACHED-THE-END\n' > "$DRIVER_MARKER"
DRV

drive() { # drive <harness _common.sh> <label>
  : > "$REC_LOG"
  rm -f "$TMP/marker-$2"
  FIXTURE_RUN="$FIXTURE_RUN" DRIVER_MARKER="$TMP/marker-$2" \
    pollute bash "$TMP/driver.sh" "$1" >/dev/null 2>&1 || true
  [ -e "$TMP/marker-$2" ] \
    || fail "$2: the driver did not reach its own last line, so nothing it did or did not send means anything"
  DRIVE_LOG="$(cat "$REC_LOG" 2>/dev/null || true)"
}

# A harness tree whose _common.sh can be patched without touching the real one.
# $ROOT is derived from _common.sh's own path, so the copy needs the two
# directories above it and a bin/ to find `hw` through -- symlinked, because the
# binaries are not what is being mutated.
make_harness() { # make_harness <name>; sets HARNESS
  local dir="$TMP/harness-$1"
  mkdir -p "$dir/setup/tests"
  ln -sfn "$ROOT/bin" "$dir/bin"
  ln -sfn "$ROOT/projects.json" "$dir/projects.json"   # the lane table bin/ reads
  cp "$ROOT/setup/tests/_common.sh" "$dir/setup/tests/_common.sh"
  HARNESS="$dir/setup/tests/_common.sh"
}

# The two anchors are the lines of setup/tests/_common.sh that carry a
# `# MUTATION-ANCHOR: 145-Mnn` marker (the sweep loop, the socket re-point): a
# mutant edits the line a marker declares, not the prose of it, and mutate_anchor
# dies when the marker is gone.

# ── C01. THE CONTROL: without the sweep, the write lands on the live pane ────
#
# The pre-fix harness exactly: no HERDR_* sweep, no re-pointed socket. This is
# the incident, reproduced in a directory, and it is what makes C02's silence
# evidence instead of an assumption.
make_harness prefix
mutate_anchor 145-M01a "$HARNESS" 'printf "PREFIX-NO-HERDR-SWEEP\n" >&2; for _v in ; do'
mutate_anchor 145-M01b "$HARNESS" ': # socket left where the caller pointed it'
drive "$HARNESS" control
case "$DRIVE_LOG" in
  *'"pane_id": "'"$LIVE_PANE"'"'*) : ;;
  *) fail "C01 the control did not reproduce: a harness with no HERDR sweep sent nothing to the recorder, so the absence asserted below proves nothing — log: $(printf '%s' "$DRIVE_LOG" | head -2 | tr '\n' ' ')" ;;
esac
case "$DRIVE_LOG" in
  *pane.report_metadata*"$FIXTURE_RUN"*) claim "C01 CONTROL: with no HERDR sweep a subject-shaped driver writes pane.report_metadata carrying the fixture run onto the live pane — the incident" ;;
  *) fail "C01 the control reached the pane but not with the fixture tokens: $(printf '%s' "$DRIVE_LOG" | head -2 | tr '\n' ' ')" ;;
esac

# ── C02. THE CLAIM: with the real harness, the live pane is never addressed ──
drive "$ROOT/setup/tests/_common.sh" real
case "$DRIVE_LOG" in
  '') claim "C02 with the real harness the same driver sends the recorder NOTHING — no write, and no read either" ;;
  *) fail "C02 the suite still reached the socket: $(printf '%s' "$DRIVE_LOG" | head -3 | tr '\n' ' ')" ;;
esac
case "$DRIVE_LOG" in
  *"$LIVE_PANE"*) fail "C02b the live pane id reached the socket" ;;
  *) claim "C02b and the live pane id appears nowhere on the wire" ;;
esac

# ── C03. the first cut: the identity is gone, invented names included ───────
cat > "$TMP/probe.sh" <<'PROBE'
#!/usr/bin/env bash
. "$1"
for n in HERDR_ENV HERDR_PANE_ID HERDR_TAB_ID HERDR_WORKSPACE_ID \
         HERDR_BIN_PATH HERDR_FUTURE_KNOB; do
  eval "v=\${$n:-<swept>}"
  printf '%s=%s\n' "$n" "$v"
done
printf 'SOCKET=%s\n' "${HERDR_SOCKET_PATH:-<swept>}"
printf 'TMP=%s\n' "$TMP"
PROBE

PROBE_OUT="$(pollute bash "$TMP/probe.sh" "$ROOT/setup/tests/_common.sh" 2>&1)"
survivors="$(printf '%s\n' "$PROBE_OUT" | grep -v '=<swept>$' | grep -v '^SOCKET=' | grep -v '^TMP=' || true)"
[ -z "$survivors" ] \
  || fail "C03 a live pane's herdr identity survived _common.sh: $(printf '%s' "$survivors" | tr '\n' ' ')"
claim "C03 every HERDR_* a live pane exports is swept, including an invented HERDR_FUTURE_KNOB"

# ── C04. the second cut: the address belongs to the harness ─────────────────
#
# Asserted SEPARATELY from C03 because it defends a different thing: C03 closes
# the gates that read a pane id, C04 closes the road for a caller that builds an
# address anyway. The path must be inside this run's own $TMP and must not exist.
probe_tmp="$(printf '%s\n' "$PROBE_OUT" | sed -n 's/^TMP=//p')"
probe_socket="$(printf '%s\n' "$PROBE_OUT" | sed -n 's/^SOCKET=//p')"
[ -n "$probe_tmp" ] && [ -n "$probe_socket" ] \
  || fail "C04 the probe reported no socket or no TMP: $PROBE_OUT"
case "$probe_socket" in
  "$probe_tmp"/*) : ;;
  *) fail "C04 \$HERDR_SOCKET_PATH is not inside the run's own TMP: $probe_socket" ;;
esac
[ "$probe_socket" != "$REC_SOCK" ] \
  || fail "C04 \$HERDR_SOCKET_PATH is still the address the caller exported"
[ ! -e "$probe_socket" ] \
  || fail "C04 the harness's socket path EXISTS, so a call could connect to it: $probe_socket"
claim "C04 \$HERDR_SOCKET_PATH is re-pointed at a path inside this run's TMP that is never created"

# ── MUTANTS ─────────────────────────────────────────────────────────────────
#
# Each kills on the PROGRAM DOING SOMETHING DIFFERENT — a request arriving that
# should not have, a value surviving that should have gone — never on a printf
# announcing that the patch applied. The markers below are vacuity guards: they
# prove the mutated line RAN, and are checked in addition to, never instead of,
# the behavioural needle. Each mutant patches a COPY; the real _common.sh is
# never touched.

# M01 — both cuts reverted, which is the harness the incident happened on. This
# is C01's control, re-asserted here so mutation-coverage counts the arm.
drive "$TMP/harness-prefix/setup/tests/_common.sh" m01
saw_mutant "M01 reverts both cuts and the fixture run id lands on a live pane again" \
  "$DRIVE_LOG" "\"pane_id\": \"$LIVE_PANE\"" 'pane.report_metadata'

# M02 — only the identity sweep reverted; the address cut stays. The recorder
# stays silent either way, so this arm is killed by the SURVIVING IDENTITY, which
# is the thing the sweep exists to remove. Without it a later "simplification"
# could drop the sweep and every absence claim above would still pass.
make_harness m02
mutate_anchor 145-M02 "$HARNESS" 'printf "M02-IDENTITY-KEPT\n" >&2; for _v in ; do'
out="$(pollute bash "$TMP/probe.sh" "$HARNESS" 2>&1 || true)"
case "$out" in
  *M02-IDENTITY-KEPT*) : ;;
  *) fail "M02 VACUOUS: the mutated line never ran — out: $(printf '%s' "$out" | tail -3 | tr '\n' ' ')" ;;
esac
saw_mutant "M02 drops the identity sweep, and a live pane's id survives into every fixture" "$out" \
  "HERDR_PANE_ID=$LIVE_PANE" "HERDR_FUTURE_KNOB=1"

# M03 — only the address re-point reverted; the sweep stays. This one does NOT
# leak: the sweep has already removed $HERDR_SOCKET_PATH, so herdr-rpc dies on
# "HERDR_SOCKET_PATH is not set" and reaches nothing either way. What the
# re-point buys is stated rather than implied, and it is what this arm measures:
# an address the HARNESS owns instead of no address at all. A caller that
# supplies its own default socket path — herdr-rpc's `--socket` flag is one, and
# a future caller with a built-in default is another — walks past an absence and
# stops at a dead path inside this run's $TMP, whose error text names the
# harness. Killed by the address being merely ABSENT, which is the state the
# re-point exists to replace.
make_harness m03
mutate_anchor 145-M03 "$HARNESS" 'printf "M03-ADDRESS-KEPT\n" >&2'
out="$(pollute bash "$TMP/probe.sh" "$HARNESS" 2>&1 || true)"
case "$out" in
  *M03-ADDRESS-KEPT*) : ;;
  *) fail "M03 VACUOUS: the mutated line never ran — out: $(printf '%s' "$out" | tail -3 | tr '\n' ' ')" ;;
esac
case "$out" in
  *"SOCKET=$REC_SOCK"*) fail "M03 SURVIVED differently: with the re-point dropped the caller's own live socket survived the sweep as well — the sweep is not removing HERDR_SOCKET_PATH at all" ;;
esac
saw_mutant "M03 drops the address re-point, leaving no address for a caller that carries its own default" "$out" \
  'SOCKET=<swept>'

[ "$CLAIMS" -ge 1 ] || fail "no behaviour claims were made"
printf 'coverage - %s behaviour claims, %s dedicated production mutants\n' "$CLAIMS" 3
