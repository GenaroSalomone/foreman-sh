#!/usr/bin/env bash
# The reply command an executor hands its brainer has to be one the brainer can
# RUN. For a codex executor it was not.
#
# MEASURED 2026-09-06 against a live codex pane (w7G:p5P, run
# 20260906-202943-67474) with bin/invoker-common.sh at 4bf66d7:
#
#   $ invoker_resolve_sender w7G:p5P
#   VENDOR=[codex] SESSION=[01a0790e-…] ENDPOINT=[~/.codex/app-server-control/app-server-control.sock]
#   $ channel-send --report --reply-hold … codex 01a0790e-… ~/.codex/…/app-server-control.sock "BRAVO"
#   Error: failed to connect to remote app server … No such file or directory (os error 2)
#   exit=1
#
# 888583c classified codex as a vendor with no native transport and made every
# route-selection site in bin/hw ask `_vendor_has_native_transport`. That fixed
# the BRAINER→EXECUTOR direction. The EXECUTOR→BRAINER direction lives in
# bin/invoker-common.sh and was left as it was: `invoker_resolve_sender` still
# fabricated the socket endpoint (and, before herdr published a session, a
# thread GUESSED from the work directory's basename), and `invoker_reply_command`
# still hand-coded `!= claude` as its route test — so the native route's four
# conditions all held and the printed command addressed a daemon that does not
# run.
#
# A previous attempt at this fix resolved its OWN pane (claude) and concluded the
# command ran. It did: the claude branch takes the herdr route. This subject
# therefore drives the resolver with the codex pane AS THE ARGUMENT, and drives
# the reply command in the codex executor's own environment, never the
# harness's.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LIB="${INVOKER_LIB_SOURCE:-$ROOT/bin/invoker-common.sh}"
HW_SRC="$ROOT/bin/hw"
[ -r "$LIB" ] || fail "no invoker library at $LIB"

# ── the harness ─────────────────────────────────────────────────────────────
# The REAL library, sourced. herdr is a stub driven by the environment so one
# harness reproduces each measured pane state; channel-send is a recorder, so
# "the command runs" is measured by running it, not by reading it.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "agent get")
    if [ -n "${FAKE_SESSION:-}" ]; then
      printf '{"result":{"agent":{"agent":"%s","cwd":"%s","agent_status":"idle","agent_session":{"value":"%s","agent":"%s"}}}}\n' \
        "$FAKE_VENDOR" "${FAKE_CWD:-/w/x}" "$FAKE_SESSION" "$FAKE_VENDOR"
    else
      printf '{"result":{"agent":{"agent":"%s","cwd":"%s","agent_status":"idle","agent_session":null}}}\n' \
        "$FAKE_VENDOR" "${FAKE_CWD:-/w/x}"
    fi
    ;;
  "pane process-info")
    if [ -n "${FAKE_PORT:-}" ]; then
      printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","%s"]}]}}}\n' "$FAKE_PORT"
    else
      printf '{"result":{"process_info":{"foreground_processes":[]}}}\n'
    fi
    ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
cat > "$TMP/bin/channel-send" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CS_LOG"
exit 0
STUB
chmod +x "$TMP/bin/herdr" "$TMP/bin/channel-send"
export CS_LOG="$TMP/cs.log"

# drive <lib> <shell snippet> — the snippet runs after the library is sourced.
# The invoker environment is the executor's own: HERDR_PANE_ID is the pane the
# functions belong to, HW_EXECUTOR_VENDOR is what hw baked into it.
drive() {
  local lib="$1"; shift
  PATH="$TMP/bin:$PATH" \
  INVOKER_PROG=probe INVOKER_BIN_DIR="$TMP/bin" \
  HERDR_PANE_ID="${PANE:-wC:p9}" HW_EXECUTOR_VENDOR="${EXEC_VENDOR:-codex}" HW_RUN="${RUN:-testrun}" \
  FAKE_VENDOR="${FAKE_VENDOR:-codex}" FAKE_SESSION="${FAKE_SESSION-01a0790e-7cf5-7183-a594-9006e28ca563}" \
  FAKE_CWD="${FAKE_CWD:-/Users/x/work/setup/guessable-basename}" FAKE_PORT="${FAKE_PORT:-}" \
  bash -c '
    set -uo pipefail
    die() { printf "die: %s\n" "$*" >&2; exit 1; }
    . "$1"; shift
    eval "$1"
  ' _ "$lib" "$1" 2>&1
}
resolve() { # <pane-argument> — prints the three variables one per line
  drive "$LIB" 'invoker_resolve_sender '"$1"'; printf "%s\n%s\n%s\n" "$INVOKER_SENDER_VENDOR" "$INVOKER_SENDER_SESSION" "$INVOKER_SENDER_ENDPOINT"'
}

# Exercise the original public entry point BEFORE any new helper. Against the
# unmodified library this must fail for its codex socket route, not because the
# classifier added by the fix is absent. Keep this first for meaningful red/green.
HOLD="$TMP/pending-reply-1"; rm -f "$HOLD"
CMD="$(drive "$LIB" 'invoker_reply_command testrun:ask:1:reply answer '"$HOLD")"
case "$CMD" in
  *"--report --reply-hold $HOLD herdr wC:p9 - "*) ;;
  *) fail "a codex executor's reply command is not the herdr one: $CMD" ;;
esac
case "$CMD" in
  *" codex "*|*app-server-control*|*"--require processed"*|*"--id "*) fail "a codex executor's reply command still carries the native route: $CMD" ;;
esac
pass "a codex executor hands its brainer a herdr reply command addressed to its pane"

# ── 1. the classifier ───────────────────────────────────────────────────────
classify() { drive "$LIB" 'if invoker_vendor_has_native_transport "'"$1"'"; then echo native; else echo herdr; fi'; }
[ "$(classify opencode)" = native ] || fail "opencode lost its native transport in the invoker library"
[ "$(classify codex)" = herdr ] || fail "the invoker library still classes codex as having a native transport"
[ "$(classify claude)" = herdr ] || fail "the invoker library no longer classes claude as herdr-only"
[ "$(classify '')" = herdr ] || fail "an unresolved vendor is not classed as herdr-only"
pass "the invoker library classes codex with claude: no native transport an invoker can address"

# ── 2. resolving a codex pane fabricates no address ─────────────────────────
# The measured case: herdr HAS published the session (codex's own
# UserPromptSubmit hook wrote it). The id is real and is kept; the endpoint must
# be the herdr sentinel, never a socket nobody creates.
OUT="$(resolve wC:p9)"
V="$(printf '%s\n' "$OUT" | sed -n 1p)"; S="$(printf '%s\n' "$OUT" | sed -n 2p)"; E="$(printf '%s\n' "$OUT" | sed -n 3p)"
[ "$V" = codex ] || fail "resolving a codex pane by argument did not read its vendor from herdr: $OUT"
[ "$S" = 01a0790e-7cf5-7183-a594-9006e28ca563 ] || fail "the session herdr published for a codex pane was not kept: $OUT"
[ "$E" = - ] || fail "a codex pane still resolves to a fabricated endpoint: [$E]"
case "$OUT" in *app-server-control*) fail "the app-server-control socket is still handed out: $OUT" ;; esac
pass "a codex pane resolves to its published session and the herdr sentinel, never to the app-server socket"

# ── 3. and guesses no thread from the work directory ────────────────────────
OUT="$(FAKE_SESSION='' resolve wC:p9)"
S="$(printf '%s\n' "$OUT" | sed -n 2p)"; E="$(printf '%s\n' "$OUT" | sed -n 3p)"
[ -z "$S" ] || fail "with no published session a codex pane was given a thread id from somewhere: [$S]"
case "$OUT" in *guessable-basename*) fail "the thread id is still guessed from the work directory's basename: $OUT" ;; esac
[ "$E" = - ] || fail "a codex pane with no session resolves to endpoint [$E], not the herdr sentinel"
pass "no published session means no session — the basename guess is gone"

# ── 5. the pending-reply fact agrees with the command ───────────────────────
# channel-send refuses a --reply-hold whose route/target differ from the
# command's own, so a hold that still said codex would make even a corrected
# command unrunnable.
grep -qx 'route=herdr' "$HOLD" || fail "the pending-reply fact does not record the herdr route: $(cat "$HOLD")"
grep -qx 'target=wC:p9' "$HOLD" || fail "the pending-reply fact does not target the executor's pane: $(cat "$HOLD")"
grep -qx 'state=undelivered' "$HOLD" || fail "the pending-reply fact is not born undelivered: $(cat "$HOLD")"
pass "the pending-reply fact names the same route and target as the printed command"

# ── 6. THE COMMAND RUNS ─────────────────────────────────────────────────────
# Not "reads right": executed, with the placeholder replaced the way a brainer
# replaces it, against the recorder. This is the delivery this subject is for.
: > "$CS_LOG"
RUN_CMD="$(printf '%s' "$CMD" | sed 's/"<your answer>"/"BRAVO"/')"
RC=0; ( eval "$RUN_CMD" ) >/dev/null 2>&1 || RC=$?
[ "$RC" = 0 ] || fail "the printed reply command exited $RC when run: $RUN_CMD"
grep -qx -- "--report --reply-hold $HOLD herdr wC:p9 - BRAVO" "$CS_LOG" \
  || fail "running the reply command did not reach channel-send with the herdr route: $(cat "$CS_LOG")"
pass "the printed reply command executes and reaches channel-send on the herdr route with the answer"

# ── 7. a ruling takes the same route ────────────────────────────────────────
rm -f "$HOLD"
CMD="$(drive "$LIB" 'invoker_reply_command testrun:challenge:1:reply ruling '"$HOLD")"
case "$CMD" in
  *"--ruling --reply-hold $HOLD herdr wC:p9 - "*) ;;
  *) fail "a codex executor's ruling command is not the herdr one: $CMD" ;;
esac
grep -qx 'intent=ruling' "$HOLD" || fail "the ruling hold does not record its intent: $(cat "$HOLD")"
pass "a contract challenge from a codex executor is answered over herdr too"

# ── 8. opencode keeps its native route ──────────────────────────────────────
rm -f "$HOLD"
CMD="$(EXEC_VENDOR=opencode FAKE_VENDOR=opencode FAKE_SESSION=ses_abc FAKE_PORT=4242 \
  drive "$LIB" 'invoker_reply_command testrun:ask:1:reply answer '"$HOLD")"
case "$CMD" in
  *"--require processed --id testrun:ask:1:reply --reply-hold $HOLD opencode ses_abc http://127.0.0.1:4242 "*) ;;
  *) fail "opencode lost its native causal reply route: $CMD" ;;
esac
pass "an opencode executor still hands its brainer the native processed-receipt command"

# ── 9. claude is unchanged ──────────────────────────────────────────────────
rm -f "$HOLD"
CMD="$(EXEC_VENDOR=claude FAKE_VENDOR=claude FAKE_SESSION=8a728e6c-d990-4064-a8b1-1682ece760d9 \
  drive "$LIB" 'invoker_reply_command testrun:ask:1:reply answer '"$HOLD")"
case "$CMD" in
  *"--report --reply-hold $HOLD herdr wC:p9 - "*) ;;
  *) fail "a claude executor's reply command changed: $CMD" ;;
esac
pass "a claude executor's reply command is the herdr one it always was"

# ── 10. one vendor list, in two files ───────────────────────────────────────
# hw loads this library lazily and 80-codex-brief-delivery.sh extracts hw's own
# classifier and drives it alone, so neither classifier can simply call the
# other. Two copies of a three-line list are acceptable only while a test says
# they agree; this is that test.
labels() { # <file> <function-name> — the case labels of a case-only classifier
  awk -v f="^${2}\\\\(\\\\) \\\\{" '$0 ~ f, /^}/' "$1" | grep -E '^\s+[^ ]+\) return [01] ;;' | sed 's/^[[:space:]]*//'
}
HW_LABELS="$(labels "$HW_SRC" _vendor_has_native_transport)"
LIB_LABELS="$(labels "$LIB" invoker_vendor_has_native_transport)"
[ -n "$HW_LABELS" ] || fail "could not extract _vendor_has_native_transport's case labels from bin/hw"
[ -n "$LIB_LABELS" ] || fail "could not extract invoker_vendor_has_native_transport's case labels from $LIB"
[ "$HW_LABELS" = "$LIB_LABELS" ] || fail "the two classifiers disagree — hw: [$HW_LABELS] library: [$LIB_LABELS]"
pass "invoker_vendor_has_native_transport and hw's _vendor_has_native_transport carry the same vendor list"

# ── 11. no hand-coded route test survives in the library ────────────────────
hand_coded="$(grep -c '\[ "\$INVOKER_SENDER_VENDOR" != claude \]' "$LIB" || true)"
[ "$hand_coded" = 0 ] \
  || fail "$hand_coded site(s) in the invoker library still hand-code != claude instead of asking the classifier"
pass "the invoker library's route selection asks the classifier, not != claude"

# ── mutants ─────────────────────────────────────────────────────────────────
# Each puts one measured defect back and must be caught by the assertion that
# exists for it. `mutate <name> <old> <new>` writes the mutant and points LIB at
# it; the assertions above are re-run through the same drive.
mutate() {
  local name="$1"; MUT="$TMP/lib-$name"; cp "$ROOT/bin/invoker-common.sh" "$MUT"
  MUT_LIB="$MUT" MUT_OLD="$2" MUT_NEW="$3" python3 - <<'PY' || fail "mutant $1 could not be written — the source no longer contains the line it mutates"
import os
p = os.environ["MUT_LIB"]
s = open(p, encoding="utf-8").read()
old, new = os.environ["MUT_OLD"], os.environ["MUT_NEW"]
assert s.count(old) == 1, (old, s.count(old))
open(p, "w", encoding="utf-8").write(s.replace(old, new))
PY
}

# M01 — codex gets a native transport back in the classifier. The resolver
# still hands it the herdr sentinel as endpoint, which is NON-EMPTY: the reply
# command must therefore go native, which is the bug in its 2026-09-06 shape.
mutate M01 '    ""|claude|codex) return 1 ;;' '    ""|claude) return 1 ;;'
rm -f "$HOLD"
CMD="$(drive "$MUT" 'invoker_reply_command testrun:ask:1:reply answer '"$HOLD")"
case "$CMD" in
  *" codex 01a0790e-7cf5-7183-a594-9006e28ca563 - "*) pass "mutant killed: M01 a native codex in the classifier sends the reply to the codex route again" ;;
  *) fail "M01 survived: the classifier no longer decides the reply route — got: $CMD" ;;
esac

# M02 — the fabricated socket endpoint comes back. The classifier still keeps
# the reply on herdr, so this is caught where the address is handed out, not
# where it is used: nothing may invent an address, used or not.
mutate M02 '    claude|codex) INVOKER_SENDER_ENDPOINT="-" ;;' \
  '    claude) INVOKER_SENDER_ENDPOINT="-" ;;
    codex) INVOKER_SENDER_ENDPOINT="$HOME/.codex/app-server-control/app-server-control.sock" ;;'
OUT="$(drive "$MUT" 'invoker_resolve_sender wC:p9; printf "%s\n" "$INVOKER_SENDER_ENDPOINT"')"
case "$OUT" in
  *app-server-control*) pass "mutant killed: M02 the fabricated socket endpoint is caught at the resolver" ;;
  *) fail "M02 survived or misfired: the mutant resolved [$OUT]" ;;
esac

# M03 — the reply command goes back to `!= claude`. With the fixed resolver the
# endpoint is `-`, non-empty, so codex is native again exactly as before.
mutate M03 '  if invoker_vendor_has_native_transport "$INVOKER_SENDER_VENDOR" \' \
  '  if [ -n "$INVOKER_SENDER_VENDOR" ] && [ "$INVOKER_SENDER_VENDOR" != claude ] \'
rm -f "$HOLD"
CMD="$(drive "$MUT" 'invoker_reply_command testrun:ask:1:reply answer '"$HOLD")"
case "$CMD" in
  *" codex 01a0790e-7cf5-7183-a594-9006e28ca563 - "*) pass "mutant killed: M03 a hand-coded != claude puts codex back on the native route" ;;
  *) fail "M03 survived: != claude no longer changes the route — got: $CMD" ;;
esac

# M04 — the basename guess comes back for a session-less codex pane.
mutate M04 '    claude|codex) INVOKER_SENDER_ENDPOINT="-" ;;' \
  '    codex)
      [ -n "$INVOKER_SENDER_SESSION" ] || INVOKER_SENDER_SESSION="$(printf "%s" "$info" | jq -r ".result.agent.cwd // empty | split(\"/\")[-1]" 2>/dev/null || true)"
      INVOKER_SENDER_ENDPOINT="-" ;;
    claude) INVOKER_SENDER_ENDPOINT="-" ;;'
OUT="$(FAKE_SESSION='' drive "$MUT" 'invoker_resolve_sender wC:p9; printf "%s\n" "$INVOKER_SENDER_SESSION"')"
case "$OUT" in
  *guessable-basename*) pass "mutant killed: M04 a thread guessed from the work directory is caught" ;;
  *) fail "M04 survived or misfired: the mutant resolved session [$OUT]" ;;
esac
