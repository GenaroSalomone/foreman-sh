#!/usr/bin/env bash
# Mutate a COPY of bin/, never the live tree.
#
# WHY THIS ARM EXISTS. Verde no es cobertura. The arms in 33-killed-sender.sh
# went green the first time they ran, and a green arm proves only that code and
# expectation agree — never that the arm would notice a change. The claims here
# are exactly the kind that fail silently: a handler that cleans up and then
# falls through into the send reads like a handler and behaves like the bug, and
# it shipped that way for days after being written down as a release blocker
# (see setup/decisions.md).
#
# ONE MUTANT PER BEHAVIOURAL CLAIM. Each one below is a plausible edit — the
# line as it used to be, or the shortcut a hurried reader would write — not a
# syntax error.
#
# HOW IT ISOLATES. Each subject test file derives its own $ROOT from
# $BASH_SOURCE, so running the copy at $WORK/setup/tests/<f> makes $WORK the
# root and $WORK/bin the binaries under test. Nothing reaches ~/brain.
set -euo pipefail
ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ART="${HW_ARTIFACTS:-${TMPDIR:-/tmp}}"
WORK="$ART/killed-sender-mutants"
rm -rf "$WORK"; mkdir -p "$WORK/setup"
cp -R "$ROOT/bin" "$WORK/bin"
cp -R "$ROOT/setup/tests" "$WORK/setup/tests"
cp -R "$ROOT/setup/guards" "$WORK/setup/guards"
cp "$ROOT/setup/test-hw" "$ROOT/setup/test-channel-send" "$WORK/setup/"
PRISTINE="$WORK/pristine"; mkdir -p "$PRISTINE"
cp "$ROOT/bin/channel-send" "$ROOT/bin/state-witness.sh" "$PRISTINE/"

killed=0; survived=0

# BASELINE FIRST. A mutant "killed" by a subject that was already red proves
# nothing at all.
for subject in 33-killed-sender.sh 30-state-witness.sh 05-channel-send.sh; do
  if bash "$WORK/setup/tests/$subject" >"$WORK/baseline-$subject.txt" 2>&1; then
    printf 'ok - baseline %s is green before mutation\n' "$subject"
  else
    printf 'not ok - baseline %s is ALREADY RED, so no mutant it kills would mean anything\n' "$subject"
    exit 1
  fi
done

mutant() { # <name> <bin-file> <subject-test> <from> <to>
  local name="$1" binfile="$2" subject="$3" from="$4" to="$5"
  cp "$PRISTINE/$binfile" "$WORK/bin/$binfile"
  python3 - "$WORK/bin/$binfile" "$from" "$to" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text(); old, new = sys.argv[2:]
n = s.count(old)
if n != 1:
    raise SystemExit("expected exactly one mutation site, found %d" % n)
p.write_text(s.replace(old, new, 1))
PY
  chmod +x "$WORK/bin/$binfile"
  if bash "$WORK/setup/tests/$subject" >"$WORK/$name.txt" 2>&1; then
    printf 'not ok - mutant %s SURVIVED %s\n' "$name" "$subject"; survived=$((survived + 1))
  else
    printf 'ok - mutant %s killed by %s\n' "$name" "$subject"; killed=$((killed + 1))
  fi
  cp "$PRISTINE/$binfile" "$WORK/bin/$binfile"; chmod +x "$WORK/bin/$binfile"
}

# ── ONE DOCUMENTED EQUIVALENT MUTANT, AND WHY IT IS NOT A GAP ───────────────
#
# The first mutant written for this guard was the obvious one: turn the signal
# handler's `exit 1` into a `return`, so it cleans up and falls through into the
# send — the exact old bug. IT SURVIVED, twice (`return 1`, then `return 0`),
# and the honest reading took a third measurement rather than a third guess.
#
# MEASURED: the same fixture run against the shipped binary and against a
# version with BOTH the handler's `exit` AND its `witness_kill_child` removed
# produces BYTE-IDENTICAL output and the same rc=1, with no delivery either way:
#
#     rc=1 / sent marker: absent
#     channel-send: SIGTERM after 2s — THIS IS YOUR CALLER KILLING IT …
#     channel-send: this call was allowed to wait up to 30s …
#
# So it is a behaviourally equivalent mutant, not an uncovered claim. What makes
# a killed sender stop is the BACKGROUNDED WAIT — the mutant below, which is
# killed — because a `wait` interrupted by a trapped signal ends the wait and
# the script with it. The handler's `exit` is defence in depth on top of that.
#
# IT STAYS IN THE CODE ANYWAY, and that is a deliberate call rather than an
# oversight: it makes the contract explicit instead of resting on what bash
# happens to do after a trap handler returns, which is precisely the class of
# implicit behaviour this repository keeps being burned by. It is simply not
# claimed as tested, because it is not.

# ── THE WAIT MUST NOT GO BACK TO THE FOREGROUND ─────────────────────────────
#
# The plain foreground call is what made every gate in this toolchain unkillable
# for up to 540s. It is also the obvious "simplification" a later reader makes.
mutant wait-back-in-foreground state-witness.sh 33-killed-sender.sh \
  '  "$WITNESS_RPC" wait-agent "$pane" idle,done --timeout-ms "$ms" >/dev/null 2>&1 &
  child=$!' \
  '  "$WITNESS_RPC" wait-agent "$pane" idle,done --timeout-ms "$ms" >/dev/null 2>&1
  child=$$'

# ── THE THREE KILL STATES ARE THREE, NOT TWO ────────────────────────────────
#
# Collapsing `inflight` into "nothing was sent" is the tempting simplification,
# and it is the one that makes a caller resend a ruling the receiver may already
# be holding.
mutant inflight-claims-nothing-sent channel-send 33-killed-sender.sh \
  '  if [ "$_CS_SENT" = inflight ]; then' \
  '  if [ "$_CS_SENT" = never-happens ]; then'

# And the flag must be raised BEFORE the send is issued, not after. Setting it
# afterwards leaves the whole in-flight window reporting "nothing was sent".
mutant sent-flag-set-too-late channel-send 33-killed-sender.sh \
  '    _CS_SENT=inflight
    herdr agent prompt' \
  '    herdr agent prompt'

# ── THE DIAGNOSIS IS THE DELIVERABLE ────────────────────────────────────────
#
# A kill that exits silently is the original incident with a tidier exit code.
mutant kill-is-silent channel-send 33-killed-sender.sh \
  "printf 'channel-send: SIG%s after %ss — THIS IS YOUR CALLER KILLING IT" \
  "true 'channel-send: SIG%s after %ss — THIS IS YOUR CALLER KILLING IT"

# ── THE BUDGET NOTICE NAMES THE NUMBER ──────────────────────────────────────
#
# Reverting to "give the Bash call a timeout that matches" is a one-line edit
# that reads as harmless prose and puts the derivation back on the caller.
mutant notice-stops-naming-the-timeout channel-send 33-killed-sender.sh \
  "    printf 'channel-send: THIS CALL CAN THEREFORE TAKE UP TO %ss." \
  "    printf 'channel-send: give the Bash call a timeout that matches. %s"

# ── ALIVE IS NOT "IS A SENDER" ──────────────────────────────────────────────
#
# Dropping the identity check restores the measured 545s wait on a lock held by
# a `sleep`, with a message that calls it another sender.
mutant lock-identity-not-checked channel-send 33-killed-sender.sh \
  '              *) _dead=2 ;;' \
  '              *) : ;;'

# THE OTHER DIRECTION, and it is the one that would quietly remove the lock.
# Treating an unreadable holder as a non-sender breaks live locks the moment ps
# is unavailable — two concurrent senders to one pane, which is the ordering
# failure the lock exists to prevent.
mutant unreadable-holder-treated-as-dead channel-send 33-killed-sender.sh \
  "              '') ;;                        # could not ask: keep waiting" \
  "              '') _dead=2 ;;"

# And a real sender's lock must survive: a check that broke every lock would
# pass the positive arm and remove the mechanism.
mutant real-sender-lock-broken channel-send 33-killed-sender.sh \
  '              *channel-send*) ;;            # a real sender: keep waiting' \
  '              *channel-send*) _dead=2 ;;'

# ── AWAITING-HUMAN IS ITS OWN OUTCOME ───────────────────────────────────────
#
# It was computed and discarded once already. Folding it back into the default
# branch is precisely that regression, and it is invisible from the outside: the
# caller just waits its whole budget again.
mutant awaiting-human-discarded state-witness.sh 30-state-witness.sh \
  '      awaiting-human)
        human_streak=$((human_streak + 1))' \
  '      awaiting-human-never)
        human_streak=$((human_streak + 1))'

# One confirmation is not two. The streak is the whole safety margin against
# refusing a pane whose prompt a person is about to answer.
mutant awaiting-human-refuses-on-one state-witness.sh 30-state-witness.sh \
  '        [ "$human_streak" -lt "$WITNESS_CONFIRMATIONS" ] || return 6' \
  '        return 6'

# THE TWO STREAKS MUST NOT SHARE A COUNTER. With one counter a pane that was
# never twice-anything still reaches a refusal, and reports whichever verdict
# landed last — a fact it never established.
mutant streaks-share-one-counter state-witness.sh 30-state-witness.sh \
  '        human_streak=$((human_streak + 1))
        settled_streak=0' \
  '        human_streak=$((settled_streak + human_streak + 1))
        settled_streak=$human_streak'

# And 6 must not become 5: they send the caller to opposite places. 5 says look
# at that pane and decide what it needs; 6 says the pane already said what it
# needs, and it is not something any sender can supply.
mutant awaiting-human-reported-as-settled state-witness.sh 30-state-witness.sh \
  '        [ "$human_streak" -lt "$WITNESS_CONFIRMATIONS" ] || return 6
        ;;' \
  '        [ "$human_streak" -lt "$WITNESS_CONFIRMATIONS" ] || return 5
        ;;'

printf '\n%s killed, %s survived\n' "$killed" "$survived"
[ "$survived" = 0 ] || exit 1
