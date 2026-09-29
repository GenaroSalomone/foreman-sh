#!/usr/bin/env bash
# The two locks in this toolchain must know WHO holds them, and must release
# only what is still theirs.
#
# WHY. Both had the same shape of hole, and both are measured.
#
#   THE INVOKER RUN LOCK (invoker-common.sh) recorded NOTHING inside itself and
#   released by PATH. Measured 2026-09-08, three probe processes driving the real
#   function against one HW_RUN: A takes the lock and stays ALIVE holding it;
#   `ls -A` on the lock prints nothing, so the only evidence about its holder is
#   its mtime; age it past `find -mmin +5` — which any invoker on a long budget
#   reaches without doing anything wrong — and B reaps it and takes it. Two live
#   holders. A then exits NORMALLY and its EXIT trap runs `rmdir <path>`,
#   deleting B's LIVE lock, and C walks in. Three invokers of one run inside the
#   read-check-act window the lock exists to hold one process in — which is the
#   double-ask and duplicate-done race, arrived at through the lock.
#
#   THE DELIVERY LOCK (channel-send) does record its holder, and then decided
#   whether that holder "is a sender" by testing `ps -o command=` for the
#   substring `channel-send`. That answers a different question from the one
#   being asked. Three ordinary things remove the string from a real sender — a
#   wrapper script in front of it, a symlink under another name, and `ps`
#   truncating its column — and each one makes the check BREAK A LIVE SENDER'S
#   LOCK, which lets two senders into one exchange: the exact failure the lock
#   exists to prevent, produced by the check meant to protect it.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── the invoker run lock ────────────────────────────────────────────────────
LIB="$ROOT/bin/invoker-common.sh"
LOCKS="$TMP/locks"; mkdir -p "$LOCKS"

# A holder that takes the real lock through the real library and then waits to
# be told to let go, so a second process can be observed against a LIVE holder.
cat > "$TMP/holder" <<'HOLDER'
#!/usr/bin/env bash
set -uo pipefail
LABEL="$1"; GO="$2"
INVOKER_PROG="probe-$LABEL"
INVOKER_BIN_DIR="$(dirname "$LIB")"
die() { printf '%s: %s\n' "$INVOKER_PROG" "$*" >&2; exit 1; }
# shellcheck disable=SC1090
. "$LIB"
invoker_run_lock
printf '%s\t%s\t%s\n' "$LABEL" "$$" "$INVOKER_RUN_LOCK" > "$GO.taken"
while [ ! -e "$GO.release" ]; do sleep 0.2; done
HOLDER
chmod +x "$TMP/holder"

export LIB
lock_of() { printf '%s/hw-invoker-%s.lock' "$LOCKS" "$1"; }
start_holder() {   # start_holder <label> <run> -> pid on stdout
  # Separate statements: `local a=$1 b=$TMP/x-$a` declares both names first
  # under bash 3.2, so the second reads an unset `a` and dies under `set -u`.
  local label="$1"
  local run="$2"
  local go="$TMP/go-$label"
  rm -f "$go.taken" "$go.release"
  env TMPDIR="$LOCKS" HW_RUN="$run" HW_WORKDIR="$TMP/wd-$run" \
    "$TMP/holder" "$label" "$go" >"$TMP/out-$label" 2>&1 &
  local pid=$!
  local i=0
  while [ ! -e "$go.taken" ] && [ "$i" -lt 60 ]; do sleep 0.2; i=$((i+1)); done
  printf '%s' "$pid"
}
release_holder() { touch "$TMP/go-$1.release"; }
# stop_holder <label> <pid> — ends a holder this file launched, on a FAILURE path.
# NOT `kill`: a failure path must not depend on the code under test. Until
# 2026-09-24 invoker_run_lock trapped TERM to release its lock and then carried
# on (the arm at the end of this file holds the fix), and on 2026-09-23 a mutant
# that failed arm 2 made `kill "$B"` leave B alive, so the bare `wait` after it
# hung the subject instead of failing it. So: release it the way it is written
# to be released, KILL it (the one signal it cannot trap), and wait for THAT pid
# only — a bare `wait` also waits for anything else.
stop_holder() {
  release_holder "$1"
  kill -KILL "$2" 2>/dev/null || true
  wait "$2" 2>/dev/null || true
}

RUN=r1
A="$(start_holder A "$RUN")"
LOCK="$(lock_of "$RUN")"
[ -d "$LOCK" ] || fail "the probe holder did not create $LOCK"

# 1. THE HOLDER IS RECORDED.
OWNER="$(cat "$LOCK/owner.pid" 2>/dev/null || true)"
case "$OWNER" in
  ''|*[!0-9]*) fail "the invoker lock records no holder pid (got '$OWNER') — the next waiter can only guess by age" ;;
  *) [ "$OWNER" = "$A" ] || fail "the invoker lock names pid $OWNER, but its holder is $A" ;;
esac
pass "invoker lock: the holder's pid is inside the lock, so staleness is a fact and not a timer"

# 2. AN AGED LOCK WITH A LIVE HOLDER IS NOT STALE. This is the first half of the
# measured race: the age rule reaped it while A was still holding it.
touch -t "$(date -v-10M +%Y%m%d%H%M 2>/dev/null || date -d '10 minutes ago' +%Y%m%d%H%M)" "$LOCK"
B_OUT="$TMP/b.out"
( env TMPDIR="$LOCKS" HW_RUN="$RUN" HW_WORKDIR="$TMP/wd-$RUN" \
    "$TMP/holder" B "$TMP/go-B" >"$B_OUT" 2>&1 ) & B=$!
sleep 3
if [ -e "$TMP/go-B.taken" ]; then
  release_holder A; stop_holder B "$B"
  fail "a ten-minute-old lock was reaped while its holder was alive and holding it"
fi
pass "invoker lock: a lock older than the old five-minute rule is NOT reaped while its holder is alive"
[ "$(cat "$LOCK/owner.pid" 2>/dev/null || true)" = "$A" ] \
  || fail "the waiter changed the lock's owner while A still held it"
pass "invoker lock: the waiting invoker left the live holder's lock alone"

# 3. RELEASE BY IDENTITY. Hand B the lock the only legitimate way — A lets go —
# then prove that a LATE release from a process that no longer owns it cannot
# remove B's lock. That late release is the second half of the measured race.
release_holder A
wait "$A" 2>/dev/null || true
i=0; while [ ! -e "$TMP/go-B.taken" ] && [ "$i" -lt 60 ]; do sleep 0.2; i=$((i+1)); done
[ -e "$TMP/go-B.taken" ] || { stop_holder B "$B"; fail "B never acquired the lock after A released it: $(cat "$B_OUT")"; }
B_PID="$(cut -f2 "$TMP/go-B.taken")"
[ "$(cat "$LOCK/owner.pid" 2>/dev/null || true)" = "$B_PID" ] \
  || fail "B holds the lock but it names $(cat "$LOCK/owner.pid" 2>/dev/null)"
pass "invoker lock: the successor's own pid replaces the one it reaped"

# A stale releaser: same code path, a pid that is not the recorded owner.
cat > "$TMP/late-release" <<'LATE'
#!/usr/bin/env bash
set -uo pipefail
INVOKER_PROG=probe-late
INVOKER_BIN_DIR="$(dirname "$LIB")"
die() { printf 'probe-late: %s\n' "$*" >&2; exit 1; }
# shellcheck disable=SC1090
. "$LIB"
INVOKER_RUN_LOCK="$1"
invoker_release_run_lock
LATE
chmod +x "$TMP/late-release"
env TMPDIR="$LOCKS" HW_RUN="$RUN" LIB="$LIB" "$TMP/late-release" "$LOCK" >/dev/null 2>&1 || true
[ -d "$LOCK" ] \
  || { release_holder B; fail "a release from a process that does not own the lock destroyed the live holder's lock"; }
[ "$(cat "$LOCK/owner.pid" 2>/dev/null || true)" = "$B_PID" ] \
  || { release_holder B; fail "the stale release removed the live holder's owner file"; }
pass "invoker lock: a release from a process that no longer owns it leaves the live lock alone"

# 4. AND A DEAD HOLDER IS BROKEN AT ONCE, not after five minutes.
release_holder B
wait "$B" 2>/dev/null || true
[ -d "$LOCK" ] && fail "B's own release did not remove its lock: $(ls -A "$LOCK")"
pass "invoker lock: the owner's own release does remove it"

rm -rf "$LOCK"; mkdir -p "$LOCK"
printf '%s\n' 999999 > "$LOCK/owner.pid"     # a pid that cannot be alive
t0="$(date +%s)"
C="$(start_holder C "$RUN")"
elapsed=$(( $(date +%s) - t0 ))
[ -e "$TMP/go-C.taken" ] || { stop_holder C "$C"; fail "a lock held by a dead pid was not broken: $(cat "$TMP/out-C")"; }
[ "$elapsed" -le $((10 * HW_TEST_SLOW)) ] || fail "breaking a dead holder's lock took ${elapsed}s"
case "$(cat "$TMP/out-C")" in
  *"holder is gone"*"999999"*) pass "invoker lock: a dead holder is named and broken in ${elapsed}s, not waited out" ;;
  *) fail "breaking a dead holder's lock says nothing about it: $(cat "$TMP/out-C")" ;;
esac
# WAIT FOR C TO BE GONE, NOT FOR `wait`. C was started inside `$(start_holder)`,
# so it is not this shell's child and `wait "$C"` returns at once. C's EXIT trap
# then reads its own pid, and M01 below does `rm -rf; mkdir; printf owner.pid`
# on the same path: when the trap's `rmdir` lands between that mkdir and printf,
# it removes M01's fresh lock and the subject dies on "No such file or
# directory". Measured 2026-09-23: 1 in 200 copies under twenty-way load. KILL
# is the fallback because it runs no trap, so nothing of C can race below.
release_holder C
_i=0; while kill -0 "$C" 2>/dev/null && [ "$_i" -lt 60 ]; do sleep 0.2; _i=$((_i + 1)); done
stop_holder C "$C"

# MUTANT M01: release by PATH, which is what shipped. The stale releaser above
# then destroys the live successor's lock.
# python3, not sed: these mutations contain `||`, which is also the delimiter
# these seds were using, and the escaping is exactly where a mutant quietly
# stops applying and its arm starts certifying nothing.
python3 - "$LIB" "$TMP/m01.sh" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = '  [ "$owner" = "$$" ] || return 0\n'
assert s.count(old) == 1, "the identity check in invoker_release_run_lock moved"
open(dst, "w").write(s.replace(old, '  : # MUTANT: release by path\n', 1))
PYMUT
grep -q 'MUTANT: release by path' "$TMP/m01.sh" || fail "M01 did not apply"
rm -rf "$LOCK"; mkdir -p "$LOCK"; printf 'ownedbysomeoneelse\n' > "$LOCK/owner.pid"
printf '%s\n' 4242 > "$LOCK/owner.pid"
env TMPDIR="$LOCKS" LIB="$TMP/m01.sh" "$TMP/late-release" "$LOCK" >/dev/null 2>&1 || true
if [ -d "$LOCK" ]; then
  fail "M01 VACUOUS: releasing by path did not remove a lock owned by pid 4242, so the identity check is not what stops it"
fi
pass "mutant killed: M01 invoker_release_run_lock removes a lock owned by another process"

# MUTANT M02: go back to the age rule with no owner check, so a live holder's
# lock is reaped. The mutant's own stderr is the evidence.
python3 - "$LIB" "$TMP/m02.sh" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = ('        if ! kill -0 "$owner" 2>/dev/null \\\n'
       '           && _invoker_reap_run_lock "$lock" "$owner" "$owner_start"; then\n')
assert s.count(old) == 1, "the liveness check in invoker_run_lock moved"
new = ('        if [ -n "$(find "$lock" -maxdepth 0 -mmin +5 2>/dev/null || true)" ]; then\n'
       '          printf \'%s: MUTANT reaped by age\\n\' "${INVOKER_PROG:-invoker}" >&2\n'
       '          rm -f "$lock/owner.pid" "$lock/owner.start" 2>/dev/null || true\n'
       '          rmdir "$lock" 2>/dev/null || true\n')
open(dst, "w").write(s.replace(old, new, 1))
PYMUT
grep -q 'MUTANT reaped by age' "$TMP/m02.sh" || fail "M02 did not apply"
RUN2=r2
LOCK2="$(lock_of "$RUN2")"
rm -rf "$LOCK2"
A2="$(LIB="$TMP/m02.sh" start_holder A2 "$RUN2")"
[ -e "$TMP/go-A2.taken" ] || { fail "M02's holder never took the lock: $(cat "$TMP/out-A2")"; }
touch -t "$(date -v-10M +%Y%m%d%H%M 2>/dev/null || date -d '10 minutes ago' +%Y%m%d%H%M)" "$LOCK2"
env TMPDIR="$LOCKS" HW_RUN="$RUN2" HW_WORKDIR="$TMP/wd-$RUN2" LIB="$TMP/m02.sh" \
  "$TMP/holder" B2 "$TMP/go-B2" >"$TMP/out-B2" 2>&1 & B2=$!
# The mutant's line appears on its first pass over a pre-aged lock, so the wait
# is for that line, bounded at the 3s the arm used to sleep flat.
_i=0
while [ "$_i" -lt 30 ] && ! grep -q 'MUTANT reaped by age' "$TMP/out-B2" 2>/dev/null; do sleep 0.1; _i=$((_i + 1)); done
saw_mutant "M02 the lock is reaped by age with a live holder" \
  "$(cat "$TMP/out-B2" 2>/dev/null)" "MUTANT reaped by age"
kill "$B2" 2>/dev/null || true
release_holder A2; release_holder B2; wait 2>/dev/null || true

# ── channel-send's delivery lock ────────────────────────────────────────────
# A COMPLETE COPY, because channel-send resolves its neighbours from its own
# directory. Same pattern as 33-killed-sender.sh.
cs="$TMP/cs"; mkdir -p "$cs/bin" "$cs/tmp"
cp "$ROOT/bin/channel-send" "$ROOT/bin/state-witness.sh" "$cs/bin/"
cat > "$cs/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
sleep "${STUB_WAIT_S:-0}"
exit "${STUB_RPC_RC:-0}"
STUB
cat > "$cs/bin/herdr" <<'STUB'
#!/usr/bin/env bash
echo '{"result":{}}'
STUB
chmod +x "$cs/bin"/*

lock="$cs/tmp/hw-deliver-wT_p1.lock"
cs_probe() {   # -> the probe's output; it must not block for a real budget
  env PATH="$cs/bin:$PATH" TMPDIR="$cs/tmp" STUB_WAIT_S=0 STUB_RPC_RC=3 \
    HW_INVOKER_WAIT_MS=2000 "$cs/bin/channel-send" herdr wT:p1 - "probe" 2>&1 || true
}
# A 2s BUDGET IS A 7s LOCK WAIT (budget + channel-send's fixed 5s). The arms
# that wait on a live holder read what the loop says on its first pass and then
# watch it keep the lock until it gives up; at 20000 that was 25s per arm for
# the same observation.

# 5. A REAL SENDER UNDER A NAME `ps` DOES NOT SPELL `channel-send` MUST NOT LOSE
# ITS LOCK. This is the defect: the holder is a genuine sender invoked through a
# symlink, so its command line carries the LINK's name.
# A SYMLINK UNDER ANOTHER NAME, not a wrapper that `exec`s. `exec` replaces the
# process image, so `ps -o command=` still spells `channel-send` and the old
# check survives it — checked while writing this file. A symlink changes what
# `ps` reports (it shows the path the script was invoked through) without
# changing which program is running, which is precisely the case that makes
# "does the command line contain this string" the wrong question.
ln -sf "$cs/bin/channel-send" "$cs/bin/deliver"
[ -x "$cs/bin/deliver" ] || fail "the symlinked sender is not executable"
rm -rf "$lock"; mkdir -p "$lock"
env PATH="$cs/bin:$PATH" TMPDIR="$cs/tmp" STUB_WAIT_S=60 STUB_RPC_RC=2 \
  HW_INVOKER_WAIT_MS=60000 "$cs/bin/deliver" herdr wT:p9 - "holder" >/dev/null 2>&1 &
holder=$!
# WAIT FOR THE FACT, NOT FOR A DURATION. This was `sleep 1`, and one second is a
# bet on the scheduler, not a property of the code: this repository runs several
# executors and whole test suites at once, and on 2026-09-09 this assertion
# failed three times in a row while two suites and two review subagents were
# running — then passed, unchanged, on a quiet machine. Both channel-send
# versions either side of that day's commit pass in isolation, so what the arm
# had started measuring was the load average.
#
# A test that fails under load and passes idle teaches everyone to re-run it,
# which is how a real regression gets waved through. Poll for the file the
# sender is supposed to write, with a bound, and say which of the two failures
# happened.
holder_start="$cs/tmp/hw-deliver-wT_p9.lock/owner.start"
waited=0
while [ ! -s "$holder_start" ] && [ "$waited" -lt 100 ]; do
  sleep 0.2; waited=$((waited + 1))
done
# Its own lock is on wT:p9; its pid goes into the wT:p1 lock, which is the
# situation the identity check has to read correctly.
printf '%s\n' "$holder" > "$lock/owner.pid"
if [ ! -s "$holder_start" ] && ! kill -0 "$holder" 2>/dev/null; then
  fail "the backgrounded sender exited before recording anything — it never reached the lock, so this arm measured nothing"
fi
cp "$holder_start" "$lock/owner.start" 2>/dev/null \
  || fail "the sender recorded no owner.start after $((waited / 5))s, so identity still rests on its name"
pass "delivery lock: a sender records WHEN it started, not only its pid"
# The recorded start is the wrapper's exec'd process — the same process — so it
# must match, and the name must be irrelevant.
OUT="$(cs_probe)"
kill -TERM "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
case "$OUT" in
  *"holder is gone"*) fail "the fixture's holder died, so this arm proved nothing: $OUT" ;;
  *"NOT a sender"*) fail "a live sender behind a symlink lost its lock to the command-name check: $OUT" ;;
  *"pid was REUSED"*) fail "a live sender behind a symlink was called a reused pid: $OUT" ;;
  *"another sender holds"*) pass "delivery lock: a real sender invoked through a symlink keeps its lock — identity is the process, not the name" ;;
  *) fail "unexpected behaviour against a symlinked sender's lock: $OUT" ;;
esac
rm -rf "$lock" "$cs/tmp/hw-deliver-wT_p9.lock"

# 6. AND A REUSED PID IS BROKEN AT ONCE, positively rather than by inference
# from a name. Same pid, a start time that is not the one recorded.
rm -rf "$lock"; mkdir -p "$lock"
sleep 120 & impostor=$!
printf '%s\n' "$impostor" > "$lock/owner.pid"
printf 'Thu Jan  1 00:00:00 1970\n' > "$lock/owner.start"
t0="$(date +%s)"
OUT="$(cs_probe)"
elapsed=$(( $(date +%s) - t0 ))
kill "$impostor" 2>/dev/null || true
rm -rf "$lock"
case "$OUT" in
  *"pid was REUSED"*) pass "delivery lock: a pid whose start time is not the recorded one is named as reused" ;;
  *) fail "a reused pid was not identified: $OUT" ;;
esac
case "$OUT" in
  *"1970"*) pass "delivery lock: it prints both start times, so the operator can see why" ;;
  *) fail "the reused-pid message does not show the times it compared: $OUT" ;;
esac
[ "$elapsed" -le $((10 * HW_TEST_SLOW)) ] \
  && pass "delivery lock: it breaks that lock in ${elapsed}s instead of spending the budget" \
  || fail "it spent ${elapsed}s on a lock whose holder pid was reused"

# 7. UNREADABLE IS STILL NOT A NEGATIVE ANSWER — the asymmetry every check in
# this toolchain rests on. `ps` unavailable must leave the lock alone.
cat > "$cs/bin/ps" <<'STUB'
#!/usr/bin/env bash
exit 1
STUB
chmod +x "$cs/bin/ps"
rm -rf "$lock"; mkdir -p "$lock"
sleep 120 & impostor=$!
printf '%s\n' "$impostor" > "$lock/owner.pid"
printf 'Thu Jan  1 00:00:00 1970\n' > "$lock/owner.start"
OUT="$(cs_probe)"
kill "$impostor" 2>/dev/null || true
rm -rf "$lock"; rm -f "$cs/bin/ps"
case "$OUT" in
  *"pid was REUSED"*) fail "it broke a lock on a holder whose start time it could not read: $OUT" ;;
  *"NOT a sender"*) fail "it broke a lock on a holder it could not identify at all: $OUT" ;;
  *"another sender holds"*) pass "delivery lock: a holder ps cannot describe is left alone — unreadable is not a negative answer" ;;
  *) fail "unexpected behaviour when ps is unavailable: $OUT" ;;
esac

# MUTANT M03: identity by command name only, which is what shipped. The
# symlinked sender in arm 5 then loses its lock.
python3 - "$ROOT/bin/channel-send" "$cs/bin/channel-send" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = '            _owner_start_was="$(cat "$_lock/owner.start" 2>/dev/null || true)"\n'
assert s.count(old) == 1, "the recorded start time is read somewhere else now"
open(dst, "w").write(s.replace(old, '            _owner_start_was="" # MUTANT: name only\n', 1))
PYMUT
grep -q 'MUTANT: name only' "$cs/bin/channel-send" || fail "M03 did not apply"
chmod +x "$cs/bin/channel-send"
rm -rf "$lock"; mkdir -p "$lock"
env PATH="$cs/bin:$PATH" TMPDIR="$cs/tmp" STUB_WAIT_S=60 STUB_RPC_RC=2 \
  HW_INVOKER_WAIT_MS=60000 "$cs/bin/deliver" herdr wT:p9 - "holder" >/dev/null 2>&1 &
holder=$!
# Same bounded wait as the arm above, and for a sharper reason here: this `cp`
# ends in `|| true`, so under load the mutant's lock would silently have NO
# recorded start time — and the arm would then be judging a lock that is missing
# the very field M03 is about.
waited=0
while [ ! -s "$cs/tmp/hw-deliver-wT_p9.lock/owner.start" ] && [ "$waited" -lt 100 ]; do
  sleep 0.2; waited=$((waited + 1))
done
printf '%s\n' "$holder" > "$lock/owner.pid"
cp "$cs/tmp/hw-deliver-wT_p9.lock/owner.start" "$lock/owner.start" 2>/dev/null \
  || fail "M03 could not stage a recorded start time, so this arm would judge a lock without the field it is about"
OUT="$(cs_probe)"
kill -TERM "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
rm -rf "$lock" "$cs/tmp/hw-deliver-wT_p9.lock"
saw_mutant "M03 identity rests on the command name, so a symlinked sender loses its lock" \
  "$OUT" "NOT a sender"

# ── the pid-reuse half, added 2026-09-08 after Judgment Day ─────────────────
#
# BOTH judges raised this independently: the commit that gave the delivery lock
# `owner.start` claimed the two locks were "now one design", and this one still
# recorded a bare pid. `kill -0` says only that SOME process holds that number
# now — between a crashed invoker and this moment the number can have been
# handed to anything, and the waiter then spent its whole 120s budget and
# reported "pid N, still alive" about a process that was never an invoker.
#
# The probe CALLS the waiter. `late-release` above only releases, so it never
# reaches the claim loop where this decision lives.
cat > "$TMP/take-lock" <<'TAKE'
#!/usr/bin/env bash
set -uo pipefail
INVOKER_PROG=probe-take
INVOKER_BIN_DIR="$(dirname "$LIB")"
die() { printf 'probe-take: %s\n' "$*" >&2; exit 1; }
# shellcheck disable=SC1090
. "$LIB"
invoker_run_lock
printf 'TOOK-THE-LOCK\n'
TAKE
chmod +x "$TMP/take-lock"
# PAID ONCE, OUTSIDE THE BUDGET. On this machine the first exec of a newly
# written file costs 0.3-2s and queues when the machine is busy; the second
# costs ~25ms. take3 below gives the healthy waiter 3s and arm 1 requires rc=0
# inside them, so an unwarmed first exec was spending that budget on the file
# being new. Measured 2026-09-23 under twenty concurrent copies: arm 1 took
# 0.34-3.05s and 3 of 20 went red with rc=124 on the HEALTHY library. The warm-up
# takes a lock nobody holds under its own run, so it measures nothing and must
# print TOOK-THE-LOCK, or the probe itself is broken and every arm below with it.
case "$(env TMPDIR="$LOCKS" HW_RUN=warm HW_WORKDIR="$TMP/wd-warm" LIB="$LIB" "$TMP/take-lock" 2>&1)" in
  *TOOK-THE-LOCK*) : ;;
  *) fail "the take-lock probe could not take a free lock, so no arm below would mean anything" ;;
esac
RUN3=r3
LOCK3="$(lock_of "$RUN3")"
plant() { # plant <start-file-contents|-> ; a pid that IS alive: this shell
  rm -rf "$LOCK3"; mkdir -p "$LOCK3"
  printf '%s\n' "$$" > "$LOCK3/owner.pid"
  if [ "$1" = - ]; then
    :
  elif [ "$1" = real ]; then
    # THE SUBJECT'S OWN HELPER, through the same probe that uses it. Sourcing
    # $LIB from this shell needs INVOKER_BIN_DIR and a die(), and a subshell
    # that fails silently gave an empty file and a control that proved nothing.
    # Re-implementing `ps -o lstart=` here instead would let the two formats
    # drift and the control would go quietly vacuous.
    env LIB="$LIB" bash -c '
      set -uo pipefail
      INVOKER_PROG=probe-start
      INVOKER_BIN_DIR="$(dirname "$LIB")"
      die() { exit 1; }
      . "$LIB"
      _invoker_proc_start "$1"
    ' _ "$$" > "$LOCK3/owner.start" 2>/dev/null || true
    [ -s "$LOCK3/owner.start" ] \
      || fail "the negative control could not record a real start time, so it would prove nothing"
  else
    printf '%s\n' "$1" > "$LOCK3/owner.start"
  fi
}
take3() { # -> "rc=<n>" then the probe's output
  # BOUNDED, and the bound is evidence. A waiter that decides to WAIT holds for
  # 120s, so `timeout` is what ends it — and rc=124 is then a positive
  # observation of "it chose to wait", which is the only thing that
  # distinguishes a mutant that ignores the start time from one that reads it.
  # Without the code, waiting looks like silence, and silence is not a needle.
  # 3s is three passes of the lock loop's 1s poll: a waiter that was going to
  # break the lock has done it on the first. HW_TEST_SLOW stretches it where
  # starting the probe alone takes seconds (Git Bash), still far under 120s.
  local out rc=0
  out="$(env TMPDIR="$LOCKS" HW_RUN="$RUN3" HW_WORKDIR="$TMP/wd-$RUN3" LIB="${1:-$LIB}" \
    timeout $((3 * HW_TEST_SLOW)) "$TMP/take-lock" 2>&1)" || rc=$?
  printf 'rc=%s\n%s\n' "$rc" "$out"
}

# 1. A live pid whose RECORDED start time cannot be its own: that is reuse.
plant 'Thu Jan  1 00:00:00 1970'
OUT="$(take3)"
case "$OUT" in
  *"pid was REUSED"*) pass "a live pid whose recorded start time does not match is broken as a reused pid, not waited on" ;;
  *"still alive"*)    fail "the waiter spent its budget on a reused pid and called it a busy holder: $OUT" ;;
  *) fail "the reuse case produced neither verdict: $OUT" ;;
esac
case "$OUT" in
  rc=0*) : ;;
  *) fail "the reuse case did not exit cleanly — rc says it waited or died: $OUT" ;;
esac
case "$OUT" in
  *TOOK-THE-LOCK*) pass "and having broken it, the waiter goes on to take the lock (rc=0, no timeout)" ;;
  *) fail "the reuse was diagnosed but the lock was never taken: $OUT" ;;
esac

# 2. THE NEGATIVE CONTROL, which is what makes arm 1 mean anything: a MATCHING
#    start time is a real holder. Observed positively — the lock is still there
#    and still names the same holder — not by the absence of a word.
plant real
OUT="$(take3)"
case "$OUT" in
  *"pid was REUSED"*) fail "a MATCHING start time was called a reused pid — every live holder would lose its lock: $OUT" ;;
  *TOOK-THE-LOCK*)    fail "the waiter took a lock held by a live, matching holder: $OUT" ;;
esac
[ -d "$LOCK3" ] && [ "$(cat "$LOCK3/owner.pid" 2>/dev/null || true)" = "$$" ] \
  && pass "a matching start time is a real holder: the lock survives and still names it" \
  || fail "the live holder's lock was destroyed or renamed by a waiter"

# 3. An absent start time is not evidence of reuse. A lock from a version that
#    recorded none must fall through to the kill -0 rule, not be broken on
#    sight — the same principle as the unreadable age probe above.
plant -
OUT="$(take3)"
case "$OUT" in
  *"pid was REUSED"*) fail "a lock with NO recorded start time was declared reused — an absent record is not evidence: $OUT" ;;
  *TOOK-THE-LOCK*)    fail "a lock with no recorded start time was broken by a waiter: $OUT" ;;
esac
[ -d "$LOCK3" ] \
  && pass "an absent start time falls through to kill -0 instead of being read as reuse" \
  || fail "the no-start-time lock was removed"

# M04 — the start time is written and never consulted: the state the commit
# message described as already fixed. Dies on arm 1.
# python3, not sed: the line contains `||`, which is also sed's delimiter here.
# Third time in this task; it is written down so it is the last.
python3 - "$ROOT/bin/invoker-common.sh" "$TMP/m04.sh" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = '        owner_start="$(cat "$lock/owner.start" 2>/dev/null || true)"\n'
assert old in s, "M04 anchor missing"
open(dst, "w").write(s.replace(old, '        owner_start=""\n', 1))
PYMUT
grep -q '^        owner_start=""$' "$TMP/m04.sh" || fail "M04 did not apply"
bash -n "$TMP/m04.sh" || fail "M04 produced a syntactically invalid mutant, which kills nothing"
plant 'Thu Jan  1 00:00:00 1970'
# rc=124 is `timeout`'s: the mutant WAITED on the reused pid the healthy binary
# breaks in rc=0. Arm 1 above positively observed the healthy rc=0 on this same
# fixture, so the two are a real pair.
saw_mutant "M04 the start time is written and never read, so a reused pid is waited on as busy" \
  "$(take3 "$TMP/m04.sh")" "rc=124"
rm -rf "$LOCK3"

# ── a signal ends the invocation, it does not just drop the lock ─────────────
#
# The trap used to be `invoker_release_run_lock` on EXIT INT TERM. A trap that
# does not `exit` REPLACES the signal's default action, so a TERMed holder
# released its lock and carried on with its read-check-act outside the lock.
# Measured 2026-09-24 with the real done-invoker: TERMed while holding the lock,
# it made its next herdr-rpc call with the lock already gone.
#
# Observed positively on both sides: the healthy holder is GONE and its lock
# with it; the mutant is still ALIVE with no lock. The poll is bounded because
# the mutant never dies on its own — the bound ends the observation, it is not a
# budget the healthy side races against.
term_arm() { # term_arm <label> [lib] -> "gone" | "alive-lock=<yes|no>"
  local label="$1" pid lock i=0
  local run="rt-$label"
  lock="$(lock_of "$run")"; rm -rf "$lock"
  pid="$(LIB="${2:-$LIB}" start_holder "$label" "$run")"
  [ -e "$TMP/go-$label.taken" ] || { stop_holder "$label" "$pid"; fail "the TERM arm's holder $label never took the lock: $(cat "$TMP/out-$label" 2>/dev/null)"; }
  kill -TERM "$pid" 2>/dev/null || true
  while kill -0 "$pid" 2>/dev/null && [ "$i" -lt 25 ]; do sleep 0.2; i=$((i + 1)); done
  if kill -0 "$pid" 2>/dev/null; then
    printf 'alive-lock=%s\n' "$([ -d "$lock" ] && echo yes || echo no)"
  else
    printf 'gone-lock=%s\n' "$([ -d "$lock" ] && echo yes || echo no)"
  fi
  stop_holder "$label" "$pid"
  rm -rf "$lock"
}
case "$(term_arm T1)" in
  gone-lock=no) pass "a TERMed lock holder exits and its lock goes with it" ;;
  alive-lock=no) fail "a TERMed lock holder released its lock and KEPT RUNNING — it would go on to deliver outside the lock" ;;
  *) fail "a TERMed lock holder ended in an unexpected state" ;;
esac

# M05 — the trap releases and does not exit, which is what shipped.
python3 - "$LIB" "$TMP/m05.sh" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = "  trap 'invoker_release_run_lock; exit 143' TERM\n"
assert s.count(old) == 1, "the TERM trap in invoker_arm_lock_signals moved"
open(dst, "w").write(s.replace(old, "  trap 'invoker_release_run_lock' TERM\n", 1))
PYMUT
grep -q "^  trap 'invoker_release_run_lock' TERM$" "$TMP/m05.sh" || fail "M05 did not apply"
saw_mutant "M05 the TERM trap releases the lock and carries on" "$(term_arm T2 "$TMP/m05.sh")" "alive-lock=no"

# ── a waiter that judged a holder dead must not remove its successor's lock ───
#
# The waiter reads owner.pid, then probes it with `kill -0`. Between the two the
# holder can release cleanly and exit and a successor can take the lock; the
# probe then fails on the pid that WAS the holder, and removing the lock by path
# deletes the successor's live one. Measured 2026-09-24: two live holders.
#
# The interleaving is forced, not hoped for. The waiter below is the real
# library with ONE thing injected: a shell function named `kill` that, on the
# first `kill -0 <A>`, lets A go and waits until A is dead and S holds the lock.
# Every line of lock code that runs is the library's own.
cat > "$TMP/race-waiter" <<'WAITER'
#!/usr/bin/env bash
set -uo pipefail
INVOKER_PROG=probe-race
INVOKER_BIN_DIR="$(dirname "$LIB")"
die() { printf 'probe-race: %s\n' "$*" >&2; exit 1; }
# shellcheck disable=SC1090
. "$LIB"
kill() {
  if [ "${1:-}" = -0 ] && [ "${2:-}" = "$RACE_A" ] && [ ! -e "$RACE_DIR/injected" ]; then
    : > "$RACE_DIR/injected"
    touch "$RACE_DIR/go-RA.release"
    while builtin kill -0 "$RACE_A" 2>/dev/null; do sleep 0.05; done
    while [ ! -e "$RACE_DIR/go-RS.taken" ]; do sleep 0.05; done
    : > "$RACE_DIR/interleaved"
  fi
  builtin kill "$@"
}
invoker_run_lock
printf 'WAITER-TOOK-THE-LOCK\n'
WAITER
chmod +x "$TMP/race-waiter"
race_arm() { # race_arm [lib] -> "S=<alive|dead> W=<took|waited> rc=<n> race=<forced|missed>" then the waiter's output
  local lib="${1:-$LIB}" run=rrace ra rs out rc=0
  local lock; lock="$(lock_of "$run")"
  rm -rf "$lock" "$lock.reap" "$TMP/injected" "$TMP/interleaved" "$TMP/go-RS.taken" "$TMP/go-RS.release"
  ra="$(LIB="$lib" start_holder RA "$run")"
  [ -e "$TMP/go-RA.taken" ] || { stop_holder RA "$ra"; fail "the race arm's first holder never took the lock"; }
  # S starts waiting behind A; it takes the lock on its first 1s poll after A goes.
  env TMPDIR="$LOCKS" HW_RUN="$run" HW_WORKDIR="$TMP/wd-$run" LIB="$lib" \
    "$TMP/holder" RS "$TMP/go-RS" >"$TMP/out-RS" 2>&1 & rs=$!
  # Bounded because the healthy waiter WAITS on S, which is the correct answer;
  # rc=124 is that observation. The mutant takes the lock long before the bound.
  out="$(env TMPDIR="$LOCKS" HW_RUN="$run" HW_WORKDIR="$TMP/wd-$run" LIB="$lib" \
    RACE_A="$ra" RACE_DIR="$TMP" timeout $((8 * HW_TEST_SLOW)) "$TMP/race-waiter" 2>&1)" || rc=$?
  # Not `case` inside `$( )`: bash 3.2 reads the pattern's `)` as the end of it.
  # race=forced says the interleaving really happened: A was gone and S held the
  # lock BEFORE the waiter's probe ran. Without it a slow machine could time the
  # waiter out inside the injection and pass this arm having measured nothing.
  local s_state=dead w_state=waited forced=missed
  [ -e "$TMP/interleaved" ] && forced=forced
  if builtin kill -0 "$rs" 2>/dev/null && [ -e "$TMP/go-RS.taken" ]; then s_state=alive; fi
  case "$out" in *WAITER-TOOK-THE-LOCK*) w_state=took ;; esac
  printf 'S=%s W=%s rc=%s race=%s\n%s\n' "$s_state" "$w_state" "$rc" "$forced" "$out"
  stop_holder RA "$ra"; stop_holder RS "$rs"
  rm -rf "$lock" "$lock.reap" "$TMP/injected"
}
OUT="$(race_arm)"
case "$OUT" in
  "S=alive W=waited rc=124 race=forced"*) pass "a waiter whose holder left while it looked does not break the successor's live lock" ;;
  *"S=alive W=took"*) fail "TWO LIVE HOLDERS: the waiter broke the successor's lock after its holder released: $OUT" ;;
  *) fail "the race arm ended in a state it does not name: $OUT" ;;
esac

# M06 — the reap re-checks nothing, which is what shipped: judging a holder dead
# is enough to remove whatever lock is at that path now.
python3 - "$LIB" "$TMP/m06.sh" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = '  if [ "$now_owner" = "$seen_owner" ] && [ "$now_start" = "$seen_start" ]; then\n'
assert s.count(old) == 1, "the identity check in _invoker_reap_run_lock moved"
s = s.replace(old, '  if true; then\n', 1)
open(dst, "w").write(s)
PYMUT
bash -n "$TMP/m06.sh" || fail "M06 produced a syntactically invalid mutant, which kills nothing"
saw_mutant "M06 a waiter removes the lock at the path without re-checking who holds it" \
  "$(race_arm "$TMP/m06.sh")" "S=alive W=took rc=0 race=forced"

# ── a lock whose owner.pid is not a pid is still judged by age ───────────────
#
# The no-holder branch takes both an empty owner.pid and bytes that are not a
# pid. Judge B caught, 2026-09-24, that the reap was handed '' as the owner it
# had seen, so the garbage re-read inside never matched and the lock could never
# be broken: every invoker of that run waited 120s and failed. The base broke
# it after a minute, which is what this arm holds.
garbage_arm() { # garbage_arm [lib] -> "rc=<n>" then the probe's output
  local lock; lock="$(lock_of rgarbage)"; rm -rf "$lock" "$lock.reap"
  mkdir -p "$lock"; printf 'garbage\n' > "$lock/owner.pid"
  touch -t "$(date -v-3M +%Y%m%d%H%M 2>/dev/null || date -d '3 minutes ago' +%Y%m%d%H%M)" "$lock"
  local out rc=0
  out="$(env TMPDIR="$LOCKS" HW_RUN=rgarbage HW_WORKDIR="$TMP/wd-rgarbage" LIB="${1:-$LIB}" \
    timeout $((5 * HW_TEST_SLOW)) "$TMP/take-lock" 2>&1)" || rc=$?
  printf 'rc=%s\n%s\n' "$rc" "$out"
  rm -rf "$lock" "$lock.reap"
}
case "$(garbage_arm)" in
  rc=0*"records no holder"*TOOK-THE-LOCK*) pass "an aged lock whose owner.pid is not a pid is broken by age, not waited on forever" ;;
  *) fail "an aged lock with a non-numeric owner.pid was not broken: $(garbage_arm)" ;;
esac

# M07 — the no-holder branch hands the reap '' instead of the owner it read.
python3 - "$LIB" "$TMP/m07.sh" <<'PYMUT'
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
old = '            if _invoker_reap_run_lock "$lock" "$owner" "$claim_start"; then\n'
assert s.count(old) == 1, "the no-holder reap call moved"
open(dst, "w").write(s.replace(old, '            if _invoker_reap_run_lock "$lock" \'\' "$claim_start"; then\n', 1))
PYMUT
grep -q "_invoker_reap_run_lock \"\$lock\" '' " "$TMP/m07.sh" || fail "M07 did not apply"
saw_mutant "M07 a non-numeric owner.pid is compared against '' and never broken" "$(garbage_arm "$TMP/m07.sh")" "rc=124"
