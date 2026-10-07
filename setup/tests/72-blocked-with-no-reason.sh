#!/usr/bin/env bash
# `blocked_reason=stuck` outlives its task exactly like an ask token does, and
# nothing checked done_status for it — measured LIVE 2026-09-03 against
# w7G:p26 (setup:reuse-by-default-not-by-refusal, run 20260903-165405-26345):
# reported done_status=done/done_state=delivered, chaining lease live, footer
# idle, no question or permission anywhere on its endpoint — and herdr still
# published blocked_reason=stuck/scope=root. `hw next` recommended `hw
# unstick`; `hw wait --dry-run` read the same stale token without saying so.
# `hw unstick`'s own guard does not read done_status either, so it would not
# have refused. Its guard is otherwise correct and is NOT touched here.
#
# A fixture cannot stand in for the live proof (see 34-wait-witness.sh's own
# note on this) but it pins the branch: same shape, done_status present vs
# absent, so a regression here is caught before the next live pane is one.
#
# Run alone while working on this subject:
#     bash setup/tests/72-blocked-with-no-reason.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BIN="$TMP/bin2"
cp -R "$ROOT/bin" "$BIN"
[ -x "$BIN/hw" ] || fail "blocked-with-no-reason: the copied bin/ has no executable hw"

STUB_LOG="$TMP/log"; : > "$STUB_LOG"
export STUB_LOG

# A real .hw/<run> under a real cwd, so `hw next` can resolve a run directory
# the same way it would for a live pane — the exact prerequisite the two
# fixed message-builders (_next_busy_message, _next_settled_message) run
# behind.
WORKD="$TMP/work/setup/probe"
RUNDIR="$WORKD/.hw/20260101-000000-1"
mkdir -p "$RUNDIR"
printf 'HW_WORKDIR=%s\n' "$WORKD" > "$RUNDIR/env"
# Reuse behavior is the subject; record the direct-delivery framework contract
# so the fixture does not inherit global Speckit for its OpenCode pane.
printf '  framework   none  (chosen)\n' > "$RUNDIR/dispatch"

cat > "$BIN/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
printf '%s %s\n' "$1" "${2:-}" >> "$STUB_LOG"
if [ "$1 ${2:-}" = "call pane.read" ]; then
  printf '{"read":{"text":"an unchanging screen"}}\n'
  exit 0
fi
if [ "$1" = wait-agent ]; then
  exit "${W_WAIT_RC:-2}"
fi
printf '{"result":{}}\n'
STUB
chmod +x "$BIN/herdr-rpc"

cat > "$BIN/channel-send" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
chmod +x "$BIN/channel-send"

cat > "$TMP/bin/herdr" <<STUB
#!/usr/bin/env bash
printf 'herdr %s %s\n' "\$1" "\${2:-}" >> "$STUB_LOG"
case "\$1 \${2:-}" in
  "agent get")
    _t="\${W_TOKENS:-}"; [ -n "\$_t" ] || _t='{}'
    printf '{"result":{"agent":{"agent":"opencode","agent_status":"%s","cwd":"%s","tokens":%s}}}\n' \\
      "\${W_STATUS:-blocked}" "$WORKD" "\$_t" ;;
  "pane process-info")
    printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","49999"]}]}}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
chmod +x "$TMP/bin/herdr"

cat > "$TMP/bin/curl" <<'STUB'
#!/usr/bin/env bash
printf '[]'
STUB
chmod +x "$TMP/bin/curl"

export WITNESS_SLICE_MS=200 WITNESS_INTERVAL_S=0 WITNESS_SAMPLES=2

hw_wait() {
  "$BIN/hw" wait-one "$@" 2>&1 | sed 's/\x1b\[[0-9;]*m//g'
}
hw_next() {
  # This subject exercises stale lifecycle tokens, not inherited framework
  # selection. Make its direct-delivery contract explicit on every retask.
  "$BIN/hw" next "$@" --sdd none 2>&1 | sed 's/\x1b\[[0-9;]*m//g'
}
expect_says() {
  local label="$1" pat="$2" out="$3"
  case "$out" in *"$pat"*) pass "$label" ;; *) fail "$label — no '$pat' in: $out" ;; esac
}
expect_silent() {
  local label="$1" pat="$2" out="$3"
  case "$out" in *"$pat"*) fail "$label — '$pat' should NOT appear in: $out" ;; *) pass "$label" ;; esac
}

DONE_STUCK='{"blocked_reason":"stuck","blocked_scope":"root","done_status":"done","done_state":"delivered"}'
GENUINELY_STUCK='{"blocked_reason":"stuck","blocked_scope":"root"}'
DONE_STUCK_BLOCKED_REPORT='{"blocked_reason":"stuck","blocked_scope":"root","done_status":"blocked","done_state":"delivered"}'

# ── hw wait --dry-run: settled + already reported ───────────────────────────
out="$(W_TOKENS="$DONE_STUCK" W_STATUS=blocked hw_wait wX:p1 --dry-run)" || true
expect_says "hw wait: a settled blocked/stuck pane that already reported names it" \
  "it already reported (done_status=done, done_state=delivered)" "$out"
expect_says "hw wait: and says the token is stale, not a live condition" "stale" "$out"
expect_silent "hw wait: and does NOT recommend hw unstick on a finished executor" "in-place recovery is available" "$out"

# ── hw wait --dry-run: genuinely stuck (no done token) is UNCHANGED ─────────
out="$(W_TOKENS="$GENUINELY_STUCK" W_STATUS=blocked hw_wait wX:p1 --dry-run)" || true
expect_silent "hw wait: a genuinely stuck pane (no done token) gets no already-reported note" \
  "it already reported" "$out"

# ── hw wait, the real (non-dry) settled path carries the same note ─────────
# witness_wait currently rounds its deadline to whole seconds. Three seconds
# guarantees at least two full seconds after that truncation: enough for both
# 200ms settled confirmations plus fixture/process overhead on a loaded host.
out="$(W_TOKENS="$DONE_STUCK" W_STATUS=blocked hw_wait wX:p1 --timeout-ms $((3000 * HW_TEST_SLOW)))" || true
expect_says "hw wait (real): the settled exit also carries the already-reported note" \
  "it already reported (done_status=done, done_state=delivered)" "$out"

# ── hw next (real, non-dry): delivered completion opens the gate ───────────
# `done_status=blocked` (a task that reported --blocked) counts too — this is
# STILL a finished executor, and the stale lifecycle token cannot veto reuse.
out="$(W_TOKENS="$DONE_STUCK_BLOCKED_REPORT" W_STATUS=blocked hw_next wX:p1 --wait-ms 1000 "next task")" || true
expect_says "hw next: a done_status=blocked (reported --blocked) executor opens the gate" \
  "ignoring its stale blocked/stuck lifecycle token" "$out"
expect_says "hw next: a reported-blocked executor is actually dispatched" \
  "task 2 dispatched" "$out"
expect_silent "hw next: and is not sent to hw unstick either" "in-place recovery is available" "$out"

out="$(W_TOKENS="$DONE_STUCK" W_STATUS=blocked hw_next wX:p1 --wait-ms 1000 "next task")" || true
expect_says "hw next: a blocked/stuck pane that already reported is reused, not merely diagnosed" \
  "ignoring its stale blocked/stuck lifecycle token" "$out"
expect_says "hw next: and advances the next task again" "task 3 dispatched" "$out"
expect_silent "hw next: and never recommends restarting a finished executor" "in-place recovery is available" "$out"

# `hw next`'s own earlier gate refuses "has not reported" before these
# branches are ever reached unless the task already reported (marker or
# done_status token) or `--force` overrides it — so the genuinely-stuck,
# never-reported case can only reach this message via --force, same as a real
# brainer deliberately overriding an executor it knows will never report.
out="$(W_TOKENS="$GENUINELY_STUCK" W_STATUS=blocked hw_next wX:p1 --force --wait-ms 1000 "next task")" || true
expect_says "hw next: a genuinely stuck pane (no done token, --force) keeps the hw unstick recommendation, unchanged" \
  "hw unstick" "$out"
expect_silent "hw next: and is never told it already reported when it did not" "already reported" "$out"

# ── mutation arm: unconditional, like every other subject here ─────────────
# `hw` sources siblings from its own resolved directory, so the mutant needs
# the whole bin/ copy, not a lone file — same lesson as 71's mutants.
MUTANT="$TMP/mutant/bin"
mkdir -p "$MUTANT"
cp -R "$ROOT/bin/." "$MUTANT/"
mutate_anchor 72-M01 "$(hw_lib_beside "$MUTANT")/next.sh" 'and false'
rm -f "$MUTANT/hw.bak"
chmod +x "$MUTANT/hw"
out="$(W_TOKENS="$DONE_STUCK" W_STATUS=blocked "$MUTANT/hw" next wX:p1 --wait-ms 1000 --sdd none "next task" 2>&1 \
  | sed 's/\x1b\[[0-9;]*m//g')" || true
if [[ "$out" == *"hw unstick"* ]]; then
  saw_mutant "M01 already-reported check on reason=stuck" "$out" "hw unstick"
else
  fail "M01 already-reported check on reason=stuck: mutant still suppressed hw unstick's own advice — the assertion above does not depend on this line: $out"
fi
