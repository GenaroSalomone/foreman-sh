#!/usr/bin/env bash
# two full suites share one machine: a worker pool, and timing subjects alone
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process. Run it alone while working on this subject:
#
#     bash setup/tests/176-dos-suites-comparten-la-maquina.sh
#
# WHAT IS BEING GUARDED. Two full suites on one machine used to be twelve
# workers where six were measured safe, and the timing subjects went red for
# nothing (five aborted verify-for-push runs on 2026-09-22/23). The first fix
# was a lock — one suite per machine — and every verify-for-push then waited
# 20-40 minutes behind another. ../test-hw now shares ONE POOL of worker slots
# per machine among every suite, and a subject that declares
# `# suite-lane: exclusive` runs with no other job beside it, of any suite.
#
# SO THE CLAIMS ARE, each against two real runners racing:
#   · the two suites RUN TOGETHER — pooled work from both overlaps in time;
#   · together they never exceed the machine's cap;
#   · a declared subject never overlaps ANY other job, from either suite —
#     the assertion that goes red if a timing subject runs beside another;
# and the mutants that remove the exclusive and the cap are seen to break them.
#
# THE FIXTURE is two throwaway repos, standing for two worktrees, each carrying a
# copy of THIS tree's runner and subjects that stamp when they start and end
# into one shared log. The runner is driven for real, through its snapshot.
# TEST_HW_NESTED is removed for every drive: the suite exports it to this file,
# and a nested runner takes no slot by design — so without removing it nothing
# below would exercise the pool at all.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
# On Git Bash this subject once stopped with rc=1 and no line saying why: a
# command failed under set -e. The trap names it there; elsewhere, as before.
case "${OSTYPE:-}" in msys*|cygwin*) trap 'printf "not ok - 176 stopped at line %s: %s\n" "$LINENO" "$BASH_COMMAND"' ERR ;; esac

export WORK="$TMP/work"        # the pool lives under $WORK — never the machine's
POOL="$WORK/.locks/suite-pool"
mkdir -p "$WORK"
STAMPS="$TMP/stamps"
: > "$STAMPS"

# stamp_subject <repo> <file> <hold-seconds> [exclusive] — holds scale by HW_TEST_SLOW:
# the second suite must still be starting while the first one's pool runs.
stamp_subject() {
  local repo="$1" file="$2" hold="$3" lane="${4:-}" name
  name="$(basename "$repo"):${file%.sh}"
  {
    printf '%s\n' '#!/usr/bin/env bash'
    if [ -n "$lane" ]; then
      printf '%s\n' '# suite-lane: exclusive — fixture: it must run with nothing beside it'
    fi
    cat <<EOS
. "\$(dirname "\${BASH_SOURCE[0]}")/_common.sh"
now() { python3 -c 'import time; print("%.3f" % time.time())'; }
printf 'start %s %s\n' "$name" "\$(now)" >> "$STAMPS"
EOS
    if [ -n "$lane" ]; then
      printf 'sleep %s\n' "$hold"
    else
      # A pool subject stays up until POOL_EXPECT pool subjects of either suite
      # are alive at once, or all eight have started: the peak is then the
      # runner's cap, however slowly a loaded machine spawns (the fixed sleep
      # it replaces peaked at 3 of an uncapped 4 on the WSL runner, 2026-10-02).
      # The ceiling only ends a wave the runner capped below POOL_EXPECT. Only a
      # race sets POOL_EXPECT; a single suite's drive keeps the fixed hold.
      printf 'if [ -z "${POOL_EXPECT:-}" ]; then sleep %s; else\n' "$hold"
      sed "s#@STAMPS@#$STAMPS#" <<'EOS'
python3 -c '
import sys, time
f, want, cap = sys.argv[1], int(sys.argv[2]), time.time() + float(sys.argv[3])
while time.time() < cap:
    st, en = set(), set()
    for l in open(f):
        k, who, _ = l.split()
        if ":2" in who:
            (st if k == "start" else en).add(who)
    if len(st) >= 8 or len(st - en) >= want:
        break
    time.sleep(0.1)
time.sleep(0.5)
'  "@STAMPS@" "$POOL_EXPECT" "$((60 * HW_TEST_SLOW))"
fi
EOS
    fi
    cat <<EOS
printf 'end %s %s\n' "$name" "\$(now)" >> "$STAMPS"
pass "stamped $name"
EOS
  } > "$repo/setup/tests/$file"
  chmod +x "$repo/setup/tests/$file"
}

make_repo() { # <dir> <runner-source> [bare — no stamping subjects]
  local repo="$1" runner="$2" g
  mkdir -p "$repo/setup/tests" "$repo/setup/guards"
  cp "$runner" "$repo/setup/test-hw"
  cp "$ROOT/setup/test-hw-snapshot.py" "$ROOT/setup/mutation-coverage" "$ROOT/setup/fast-gate-budget.sh" "$repo/setup/"
  cp "$ROOT/setup/tests/_common.sh" "$repo/setup/tests/"
  chmod +x "$repo/setup/test-hw" "$repo/setup/mutation-coverage"
  # On native Windows the runner reads a holder's start through bin/'s layer;
  # without it Git Bash's own ps prints nothing for -o lstart=, and a holder this
  # script recorded through the layer reads as a different, dead process. And
  # its herdr witness listens through _herdr_endpoint.py: the fallback it has
  # without it is AF_UNIX, which Windows Python lacks, so no witness starts.
  case "${OSTYPE:-}" in msys*|cygwin*)
    mkdir -p "$repo/bin" && cp "$ROOT/bin/msys-compat.sh" "$ROOT/bin/sitecustomize.py" "$repo/bin/"
    cp "$ROOT/setup/tests/_herdr_endpoint.py" "$repo/setup/tests/" ;;
  esac
  for g in test-deny-repo-writes.mjs test-deny-repo-writes-filesystem.mjs \
           test-opencode-hw-blocked-reason.mjs test-herdr-opencode-background-state.mjs; do
    printf '%s\n' '// stub: this subject is about the pool, not the guards' > "$repo/setup/guards/$g"
  done
  # The mutation arm's stand-in records WHERE it would write its mutants: the
  # real one writes under $HW_ARTIFACTS when that is set (section 7).
  printf '%s\n' '#!/usr/bin/env bash' "printf '%s\\n' \"\${HW_ARTIFACTS:-<unset>}\" >> \"$STAMPS.art\"" 'exit 0' \
    > "$repo/setup/guards/mutate-deny-repo-writes.sh"
  chmod +x "$repo/setup/guards/mutate-deny-repo-writes.sh"
  if [ "${3:-}" != bare ]; then
    stamp_subject "$repo" 10-alone.sh $((5 * HW_TEST_SLOW)) exclusive
    stamp_subject "$repo" 20-pool.sh $((2 * HW_TEST_SLOW))
    stamp_subject "$repo" 21-pool.sh $((2 * HW_TEST_SLOW))
    stamp_subject "$repo" 22-pool.sh $((2 * HW_TEST_SLOW))
    stamp_subject "$repo" 23-pool.sh $((2 * HW_TEST_SLOW))
  fi
  ( cd "$repo" && git init -q . && git config user.email t@example.invalid \
      && git config user.name t && git config commit.gpgsign false \
      && git add -A . && git commit -q -m fixture ) >/dev/null 2>&1 \
    || fail "fixture: could not build $repo"
}
# TEST_HW_SUITE_QUEUE=off: since 2026-09-30 two FULL runs queue (one per
# machine, tests/360), which would put this race one after the other. What
# this file holds is the pool beneath that queue — still what a full run shares
# with every fast gate — so its drives race two full runners with the queue off.
drive() { # <repo> [VAR=value...]
  ( cd "$1" && env -u TEST_HW_NESTED TEST_HW_SUITE_QUEUE=off HW_TEST_JOBS=2 HW_TEST_MACHINE_JOBS=3 "${@:2}" bash ./setup/test-hw 2>&1 )
}

# verdicts — reads the stamps of one race and prints one line per property:
#   ALONE-OK | ALONE-BROKEN <who> <who>   a declared subject overlapped a job
#   TOGETHER | APART                      pooled work from both repos overlapped
#   PEAK <n>                              most stamped jobs running at one time
verdicts() {
  python3 - "$STAMPS" "$1" "$2" <<'PY'
import sys
t = {}
for line in open(sys.argv[1]):
    kind, who, at = line.split()
    t.setdefault(who, {})[kind] = float(at)
a, b = sys.argv[2], sys.argv[3]
iv = {w: (v['start'], v['end']) for w, v in t.items() if 'start' in v and 'end' in v}
if len(iv) != len(t) or len(iv) != 10:
    print('INCOMPLETE %d of 10 subjects stamped both ends' % len(iv)); sys.exit(0)
over = lambda x, y: x[0] < y[1] and y[0] < x[1]
broken = [(w, o) for w in iv if w.endswith(':10-alone') for o in iv if o != w and over(iv[w], iv[o])]
print('ALONE-BROKEN %s %s' % broken[0] if broken else 'ALONE-OK')
pa = [iv[w] for w in iv if w.startswith(a + ':2')]
pb = [iv[w] for w in iv if w.startswith(b + ':2')]
print('TOGETHER' if any(over(x, y) for x in pa for y in pb) else 'APART')
edges = sorted([(s, 1) for s, _ in iv.values()] + [(e, -1) for _, e in iv.values()], key=lambda p: (p[0], p[1]))
n = peak = 0
for _, d in edges:
    n += d; peak = max(peak, n)
print('PEAK %d' % peak)
PY
}

# race <runner> <tag> — two worktrees start the suite together; sets RACE_*
race() {
  local runner="$1" tag="$2" a b pa pb tries=0
  a="$TMP/$tag-a"; b="$TMP/$tag-b"
  make_repo "$a" "$runner"; make_repo "$b" "$runner"
  : > "$STAMPS"
  drive "$a" POOL_EXPECT="${RACE_EXPECT:-3}" > "$TMP/$tag-a.out" &
  pa=$!
  # B starts once A's declared subject is running, so "together" is a fact,
  # not luck: without the exclusive, B's own would start beside it.
  while ! grep -q "start $tag-a:10-alone" "$STAMPS" && [ "$tries" -lt 150 ]; do sleep 0.2; tries=$((tries + 1)); done
  drive "$b" POOL_EXPECT="${RACE_EXPECT:-3}" > "$TMP/$tag-b.out" &
  pb=$!
  wait "$pa" && RACE_RC_A=0 || RACE_RC_A=$?
  wait "$pb" && RACE_RC_B=0 || RACE_RC_B=$?
  RACE_A="$(cat "$TMP/$tag-a.out")"; RACE_B="$(cat "$TMP/$tag-b.out")"
  RACE_V="$(verdicts "$tag-a" "$tag-b")"
}

# ── 1. two worktrees at once: together, capped, and the timing subject alone ──
race "$ROOT/setup/test-hw" live
[ "$RACE_RC_A" = 0 ] && [ "$RACE_RC_B" = 0 ] \
  || fail "race: a run failed (a=$RACE_RC_A b=$RACE_RC_B): $(printf '%s\n%s' "$RACE_A" "$RACE_B" | tail -8)"
case "$RACE_V" in *INCOMPLETE*) fail "race: $RACE_V" ;; esac
case "$RACE_V" in
  *ALONE-OK*) pass "race: a declared timing subject never ran beside another job, from either suite" ;;
  *) fail "race: a timing subject ran beside another job — $(printf '%s' "$RACE_V" | head -1)" ;;
esac
case "$RACE_V" in
  *TOGETHER*) pass "race: the two suites ran together — pooled work from both overlapped, neither waited for the other to finish" ;;
  *) fail "race: the two suites ran one after the other ($RACE_V)" ;;
esac
peak="$(printf '%s\n' "$RACE_V" | sed -n 's/^PEAK //p')"
[ -n "$peak" ] && [ "$peak" -le 3 ] || fail "race: $peak jobs ran at once against a machine cap of 3"
pass "race: together they never exceeded the machine's cap (peak $peak of 3)"
case "$RACE_B" in
  *"WAITING to run 10-alone.sh alone"*"pid "[0-9]*", worktree "*"live-a"*) pass "race: the waiter names who has the machine — pid and worktree" ;;
  *) fail "race: the waiter did not name who holds the machine: $(printf '%s' "$RACE_B" | head -5)" ;;
esac
[ -z "$(ls -A "$POOL" 2>/dev/null)" ] || fail "race: the pool is not empty after two clean runs: $(ls -A "$POOL")"
pass "race: nothing is left in the pool after both runs"

# ── 2. the mutants: the exclusive, then the cap, each removed alone ─────────
mutant_of() { # <out>: a copy of the runner; edit it with mutate_anchor (the marker, not the prose)
  cp "$ROOT/setup/test-hw" "$1"
}
mutant_of "$TMP/mutant-excl"
mutate_anchor 176-M01 "$TMP/mutant-excl" ': mutant-no-exclusive'
race "$TMP/mutant-excl" mexcl
saw_mutant "no machine exclusive" "$RACE_V" "ALONE-BROKEN"

# The cap alone: every slot number is claimable, the exclusive still holds.
mutant_of "$TMP/mutant-cap"
mutate_anchor 176-M02 "$TMP/mutant-cap" 'while [ "$k" -lt 99 ]; do'
RACE_EXPECT=4 race "$TMP/mutant-cap" mcap
peak="$(printf '%s\n' "$RACE_V" | sed -n 's/^PEAK //p')"
[ -n "$peak" ] && [ "$peak" -gt 3 ] && saw_mutant "no machine cap" "$RACE_V" "PEAK $peak" \
  || fail "no machine cap VACUOUS: without the slot claim the peak was still ${peak:-unknown} of 3 — the cap assertion above proves nothing"

# ── 3. a dead holder does not block: its slot, its exclusive, its wait ──────
make_repo "$TMP/solo" "$ROOT/setup/test-hw"
sleep 0 >/dev/null 2>&1 & dead=$!; wait "$dead" 2>/dev/null || true
dead_holder() { # <dir>
  mkdir -p "$1"
  printf '%s\n' "$dead" > "$1/owner.pid"
  printf 'Thu Jan  1 00:00:00 1970\n' > "$1/owner.start"
  printf 'ghost.sh\n' > "$1/owner.job"
}
rm -rf "$POOL"; mkdir -p "$POOL"
dead_holder "$POOL/slot.0"; dead_holder "$POOL/slot.1"; dead_holder "$POOL/slot.2"
dead_holder "$POOL/exclusive"
printf 'Thu Jan  1 00:00:00 1970\n' > "$POOL/wait.$dead"
: > "$STAMPS"
started=$(date +%s)
out="$(drive "$TMP/solo")" && rc=0 || rc=$?
[ "$rc" = 0 ] || fail "orphan: the run behind dead holders failed ($rc): $(printf '%s' "$out" | tail -5)"
case "$out" in
  *"reclaiming an ORPHANED exclusive slot"*"pid $dead"*) pass "orphan: an exclusive whose holder is gone is reclaimed, and said out loud" ;;
  *) fail "orphan: no exclusive reclaim was reported: $(printf '%s' "$out" | head -5)" ;;
esac
case "$out" in
  *"reclaiming an ORPHANED worker slot"*"pid $dead"*) pass "orphan: a worker slot whose holder is gone is reclaimed, and said out loud" ;;
  *) fail "orphan: no slot reclaim was reported: $(printf '%s' "$out" | head -5)" ;;
esac
[ ! -e "$POOL/wait.$dead" ] || fail "orphan: a dead waiter's file survived the run"
[ $(( $(date +%s) - started )) -lt $((60 * HW_TEST_SLOW)) ] || fail "orphan: the run behind dead holders took $((60 * HW_TEST_SLOW))s or more (its holds scale by HW_TEST_SLOW, and so does this bound)"
pass "orphan: nothing dead made the run wait"

# ── 4. a LIVE holder: the run waits and names it; a nested runner does not ──
sleep $((60 * HW_TEST_SLOW)) >/dev/null 2>&1 < /dev/null & holder=$!   # detached from our pipes
live_holder() { # <dir> <job>
  rm -rf "$1"; mkdir -p "$1"
  printf '%s\n' "$holder" > "$1/owner.pid"
  ps -o lstart= -p "$holder" | tr -s ' ' > "$1/owner.start"
  date +%s > "$1/owner.at"
  printf '%s\n' "$TMP/elsewhere" > "$1/owner.root"
  printf '%s\n' "$2" > "$1/owner.job"
}
rm -rf "$POOL"; mkdir -p "$POOL"
live_holder "$POOL/slot.1" 99-somebody-elses.sh
: > "$STAMPS"
drive "$TMP/solo" > "$TMP/late.out" 2>&1 &
late=$!
tries=0
while ! grep -q "machine is draining" "$TMP/late.out" && [ "$tries" -lt $((300 * HW_TEST_SLOW)) ]; do sleep 0.2; tries=$((tries + 1)); done
grep -q "machine is draining" "$TMP/late.out" \
  || { { kill "$late" 2>/dev/null || true; }; fail "late run: it never waited for the machine to drain: $(tail -5 "$TMP/late.out")"; }
case "$(cat "$TMP/late.out")" in
  *"will run alone at its end"*) pass "late run: arriving on a busy machine, its timing subject is deferred to its end" ;;
  *) { kill "$late" 2>/dev/null || true; }; fail "late run: the deferral was not said: $(head -5 "$TMP/late.out")" ;;
esac
case "$(cat "$TMP/late.out")" in
  *"still running: [pid $holder, worktree $TMP/elsewhere, running 99-somebody-elses.sh"*) pass "late run: the drain names the job it is waiting for" ;;
  *) { kill "$late" 2>/dev/null || true; }; fail "late run: the drain did not name the live holder: $(tail -5 "$TMP/late.out")" ;;
esac
grep -q "start solo:10-alone" "$STAMPS" && { { kill "$late" 2>/dev/null || true; }; fail "late run: the timing subject started while a live job held a slot"; }
pass "late run: the timing subject does not start while another suite's job is running"
out="$(drive "$TMP/solo" TEST_HW_NESTED=1)" && rc=0 || rc=$?
[ "$rc" = 0 ] || fail "nested: a nested runner did not run past the pool ($rc): $(printf '%s' "$out" | tail -4)"
pass "nested: a runner started from inside the suite takes no slot and waits for nothing"
[ "$(cat "$POOL/slot.1/owner.pid")" = "$holder" ] || fail "live holder: another run touched its slot"
pass "live holder: runs around it leave its slot as they found it"
rm -rf "$POOL/slot.1"
wait "$late" && rc=0 || rc=$?
[ "$rc" = 0 ] || fail "late run: once the holder left, the run did not finish green ($rc): $(tail -5 "$TMP/late.out")"
python3 - "$STAMPS" <<'PY' || fail "late run: the timing subject did not run after every pooled subject"
import sys
t = {}
for line in open(sys.argv[1]):
    kind, who, at = line.split(); t[(kind, who)] = float(at)
alone = t[('start', 'solo:10-alone')]
sys.exit(0 if all(at <= alone for (k, w), at in t.items() if k == 'end' and w != 'solo:10-alone') else 1)
PY
pass "late run: the timing subject ran last, after the machine drained"
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true

# ── 5. a subject run alone never reaches the pool ───────────────────────────
rm -rf "$POOL"; mkdir -p "$POOL"
( cd "$TMP/solo" && env -u TEST_HW_NESTED bash setup/tests/20-pool.sh >/dev/null 2>&1 ) \
  || fail "lone subject: the subject failed when run alone"
[ -z "$(ls -A "$POOL")" ] || fail "lone subject: running one subject touched the pool"
pass "lone subject: bash setup/tests/NN-*.sh takes no slot"

# ── 6. a job that addresses the pane the run was started from turns it RED ──
# The witness in ../test-hw stands where the live pane's address was, so that
# the one thing the pool no longer serialises — two runs' worth of jobs, each
# free to talk to "its" pane — is watched on every full run. The fixture job
# below does not source _common.sh, the way the guard jobs do not, and says a
# `pane.report_metadata` to whatever HERDR_SOCKET_PATH it inherited.
reach_subject() { # <repo>
  cat > "$1/setup/tests/30-reach.sh" <<'EOS'
#!/usr/bin/env bash
# deliberately NOT sourcing _common.sh: it addresses the pane it was started from
python3 -c 'import os, sys
sys.path.insert(0, os.environ.get("HW_TEST_PYLIB", ""))
try:
    from _herdr_endpoint import connect
except ImportError:
    import socket
    def connect(p):
        c = socket.socket(socket.AF_UNIX); c.connect(p); return c
s = connect(os.environ["HERDR_SOCKET_PATH"])
s.sendall(b"{\"method\":\"pane.report_metadata\",\"pane_id\":\"%s\"}\n" % os.environ.get("HERDR_PANE_ID", "").encode())
s.close()' 2>/dev/null || true
printf 'ok - reached for the pane it was started from\n'
EOS
  chmod +x "$1/setup/tests/30-reach.sh"
  ( cd "$1" && git add -A . && git commit -q -m reach ) >/dev/null 2>&1 || fail "fixture: could not add the reaching subject"
}
make_repo "$TMP/reach" "$ROOT/setup/test-hw" bare; reach_subject "$TMP/reach"
rm -rf "$POOL"
out="$(drive "$TMP/reach")" && rc=0 || rc=$?
case "$rc:$out" in
  0:*) fail "witness: a job that addressed its pane left the run green: $(printf '%s' "$out" | tail -4)" ;;
  *"reached the herdr WITNESS socket"*"pane.report_metadata"*"wWITNESS:pWITNESS"*)
    pass "witness: a job that addresses the pane the run was started from turns the whole run red, and the report quotes what it sent" ;;
  *) fail "witness: the run failed, but not on the witness ($rc): $(printf '%s' "$out" | tail -4)" ;;
esac
mutant_of "$TMP/mutant-witness"
mutate_anchor 176-M03 "$TMP/mutant-witness" ': mutant-no-witness'
make_repo "$TMP/reach-m" "$TMP/mutant-witness" bare; reach_subject "$TMP/reach-m"
rm -rf "$POOL"
out="$(drive "$TMP/reach-m")" && rc=0 || rc=$?
[ "$rc" = 0 ] || fail "no witness VACUOUS: without the witness the run still failed ($rc), so the red above may not be the witness's: $(printf '%s' "$out" | tail -3)"
saw_mutant "no witness" "$out" "tests passed"

# ── 7. two suites' guard jobs never share the caller's artifacts directory ──
# MEASURED 2026-09-28, in the second double verify-for-push: both runs were
# started from one executor, both inherited its HW_ARTIFACTS, and
# setup/guards/mutate-deny-repo-writes.sh writes its mutants under
# $HW_ARTIFACTS/mutants when that is set — so the two mutation arms shared one
# directory, and each run died on `rm: .../mutants: Directory not empty`. The
# lock had kept them apart; the pool does not, so the runner must.
rm -rf "$POOL"; : > "$STAMPS.art"
export HW_ARTIFACTS="$TMP/the-callers-artifacts"
race "$ROOT/setup/test-hw" arts
unset HW_ARTIFACTS
[ "$RACE_RC_A" = 0 ] && [ "$RACE_RC_B" = 0 ] || fail "artifacts: a run failed (a=$RACE_RC_A b=$RACE_RC_B)"
python3 - "$STAMPS.art" "$TMP/the-callers-artifacts" <<'PY' || fail "artifacts: the guard jobs of two suites shared a directory, or used the caller's: $(tr '\n' ' ' < "$STAMPS.art")"
import sys
seen = [l.strip() for l in open(sys.argv[1]) if l.strip()]
sys.exit(0 if len(seen) == 2 and len(set(seen)) == 2 and sys.argv[2] not in seen and "<unset>" not in seen else 1)
PY
pass "artifacts: each suite's guard mutation arm gets an artifacts directory of its own, never the caller's"
