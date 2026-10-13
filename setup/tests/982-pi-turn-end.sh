#!/usr/bin/env bash
# pi's turn end is hw's (setup/briefs/pi-vendor-v1.md, piece 3). The adapter's
# `agent_before_settle` runs `hw executor-turn-end --hook-json` as vendor pi and,
# when hw answers {"decision":"block"} — a handback refused, or a queued ruling
# claimed — returns it as one custom_message plus `continue: true`: the pi form of
# Claude's Stop-hook block. Driven with pi's own payload shapes against a stub hw.
# OLD: no adapter exists, so nothing runs at a pi turn end (measured live too:
# docs/pi-vendor.md, turn_state never published).
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

ADAPTER="${PI_ADAPTER_SOURCE:-$ROOT/setup/pi/hw-executor.ts}"
DRIVER="$ROOT/setup/pi/test-hw-executor.mjs"
command -v node >/dev/null 2>&1 || fail "node is required to drive the pi adapter"
[ -r "$ADAPTER" ] || fail "no pi adapter at $ADAPTER — nothing runs at a pi executor's turn end"

fake="$TMP/brain"; mkdir -p "$fake/bin"
cat > "$fake/bin/hw" <<'STUB'
#!/usr/bin/env bash
printf '%s|%s|%s\n' "$*" "${HW_EXECUTOR_VENDOR:-}" "${HW_TURN_HAD_INPUT:-}" >> "$STUB_LOG"
[ "${STUB_ALLOW:-0}" = 1 ] && exit 0
printf '{"decision":"block","reason":"STUB-REASON: report with done-invoker"}\n'
exit 8
STUB
chmod +x "$fake/bin/hw"
export STUB_LOG="$TMP/hw.log"; : > "$STUB_LOG"

out="$(HW_BRAIN_ROOT="$fake" HW_TASK=t HW_RUN=r HW_WORKDIR="$TMP" HW_PROJECT=setup \
  node --experimental-strip-types --no-warnings "$DRIVER" "$ADAPTER" turn-end 2>&1)" \
  || fail "the pi turn-end adapter did not behave: $out"
printf '%s\n' "$out" | while IFS= read -r l; do case "$l" in "ok - "*) pass "${l#ok - }" ;; esac; done
grep -q '^executor-turn-end --hook-json|pi|' "$STUB_LOG" \
  || fail "the adapter did not call hw executor-turn-end --hook-json as vendor pi: $(cat "$STUB_LOG")"
pass "the adapter asks hw (executor-turn-end --hook-json, HW_EXECUTOR_VENDOR=pi) — the predicate is hw's, not the extension's"

# Outside an hw executor the turn end is inert: no hw call at all.
: > "$STUB_LOG"
HW_BRAIN_ROOT="$fake" node --experimental-strip-types --no-warnings --input-type=module -e '
  const m = await import(process.argv[1]); const h = {};
  m.default({ on(n, f) { (h[n] ??= []).push(f); }, registerMcpServer() {} });
  if (h.agent_before_settle) { console.log("registered"); process.exit(1); }' "$ADAPTER" >/dev/null 2>&1 \
  || fail "the adapter registered a turn-end handler outside an hw executor"
[ ! -s "$STUB_LOG" ] || fail "hw was called outside an hw executor"
pass "outside an hw executor (no HW_TASK/HW_RUN/HW_WORKDIR) the adapter does not touch the turn end"
