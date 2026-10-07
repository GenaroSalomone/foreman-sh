#!/usr/bin/env bash
# Codex cannot claim an SDD flow until native entry and completion are measured.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SOURCE="${HW_SOURCE:-$ROOT/bin/hw}"
SOURCE_BIN="$(dirname "$SOURCE")"
ONLY_HANDOFF="${CODEX_REUSE_HANDOFF_ONLY:-0}"

run_launch() { env -u HW_INVOKER_PANE -u HW_CHAINING_ENABLED "$SOURCE" setup "codex-$1" --agent "$2" --model "${3:-gpt-5.3-codex}" --sdd "$1" --no-report --no-brief --dry-run 2>&1 || true; }
if [ "$ONLY_HANDOFF" != 1 ]; then
  for mode in speckit; do
    out="$(run_launch "$mode" codex)"
    case "$out" in
      *"REFUSED BEFORE BUILDING ANYTHING"*"--sdd $mode"*) pass "Codex $mode launch is refused before building" ;;
      *) fail "Codex $mode launch did not refuse honestly: $out" ;;
    esac
    case "$out" in *"dispatch setup:"*|*"agent args:"*) fail "Codex $mode reached planning/build preparation: $out" ;; esac
  done
  out="$(run_launch none codex)"
  case "$out" in *"dry run — nothing created"*) pass "Codex --sdd none remains available for direct delivery" ;; *) fail "Codex none was not allowed: $out" ;; esac
  out="$(run_launch speckit opencode openai/gpt-5.6-terra)"
  case "$out" in *"OpenCode framework dispatch REFUSED BEFORE BUILDING ANYTHING"*"--sdd speckit"*) pass "OpenCode speckit launch is refused before planning or build" ;; *) fail "OpenCode speckit launch did not refuse honestly: $out" ;; esac
  case "$out" in *"dispatch setup:"*|*"agent args:"*) fail "OpenCode speckit reached planning/build preparation: $out" ;; esac

  # M01 — make OpenCode/speckit legal in an isolated binary. The same fresh
  # launch must reach planning, proving the refusal assertion observes this guard.
  mut="$TMP/opencode-speckit-mut"; cp -R "$SOURCE_BIN" "$mut"
  MUT_HW="$mut/hw" python3 - <<'PY'
import os
p = os.environ["MUT_HW"]
s = open(p, encoding="utf-8").read()
old = '''    opencode:speckit)
      die "OpenCode framework dispatch REFUSED BEFORE BUILDING ANYTHING: --sdd $SDD_EFFECTIVE_MODE is Spec Kit, but this machine exposes Spec Kit only as Claude Code skills. Nothing was built. Use --agent claude for Spec Kit, or --sdd none for direct delivery."
      ;;'''
assert s.count(old) == 1, s.count(old)
open(p, "w", encoding="utf-8").write(s.replace(old, '    opencode:speckit) return 0 ;;'))
PY
  # A separate, unconditional gate refuses --sdd speckit before building
  # anything when no speckit-* skill is on disk, regardless of the vendor
  # pair — so proving THIS mutation reaches planning also needs a real skill
  # on disk, under a fake HOME, or that other gate would refuse it too and
  # mask the mutant surviving.
  m01home="$TMP/m01home"; mkdir -p "$m01home/.claude/skills/speckit-example"
  out="$(env -u HW_INVOKER_PANE -u HW_CHAINING_ENABLED HOME="$m01home" "$mut/hw" setup codex-mut-opencode-speckit --agent opencode --model openai/gpt-5.6-terra --sdd speckit --no-report --no-brief --fresh --dry-run 2>&1 || true)"
  case "$out" in *"dispatch setup:"*"dry run — nothing created"*) pass "mutant killed: M01 without OpenCode Speckit refusal the fresh launch reaches planning" ;; *) fail "M01 survived or misfired: $out" ;; esac
fi

# A launch request describes a fresh executor only if no reusable executor is
# already alive. Drive the real launch -> reuse -> cmd_next transaction and make
# the pane's measured vendor authoritative for framework compatibility.
handoff="$TMP/handoff"; handoff_bin="$handoff/bin"; handoff_stub="$handoff/stub-bin"
handoff_home="$handoff/home"; handoff_wd="$handoff/run"
handoff_run="$handoff_wd/.hw/20260905-140000-1"
mkdir -p "$handoff_bin" "$handoff_stub" "$handoff_home/.claude/tools" "$handoff_run"
# A leftover fixture file from when hw refused a dispatch whose entry command
# resolved to no file on disk; that gate and its own test
# (124-an-entry-command-that-does-not-exist-is-refused.sh) are gone. Kept here
# as a harmless no-op — this subject is about the codex→claude handoff below,
# not about any entry-command check.
mkdir -p "$handoff_home/.claude/commands"
: > "$handoff_home/.claude/commands/sdd-new.md"
cp -R "$SOURCE_BIN/." "$handoff_bin/"
rm -f "$handoff_bin/herdr-rpc" "$handoff_bin/channel-send"
cat > "$handoff_stub/herdr" <<'STUB'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "pane list")  printf '{"result":{"panes":[]}}\n' ;;
  "agent list") printf '{"result":{"agents":[{"pane_id":"pJD","tokens":{"hw_project":"setup","hw_task":"prior-task","hw_run":"20260905-140000-1"}}]}}\n' ;;
  "agent get")  printf '{"result":{"agent":{"cwd":"%s","agent":"%s","agent_status":"idle","tokens":{"done_status":"done"}}}}\n' "$HANDOFF_CWD" "$HANDOFF_VENDOR" ;;
  *)             printf '{"result":{}}\n' ;;
esac
STUB
cat > "$handoff_bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
case "${1:-}" in wait-agent) exit 0 ;; *) printf '{"result":{}}\n' ;; esac
STUB
cat > "$handoff_bin/channel-send" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$HANDOFF_SEND_LOG"
exit 0
STUB
# The live surface is read straight off disk now (`_speckit_skills_in`, no
# framework-mode.py, no python at all): a `.claude/skills/speckit-*` directory
# under the surface hw resolves — $handoff_wd for the reuse/next routes below,
# or $handoff_home for the global fallback `_apply_sdd` checks once a mode is
# explicitly speckit. A global skill here covers the claude-handoff accept
# case below (an explicit --sdd speckit that must actually apply); it plays no
# part in any IMPLICIT mode resolution, which reads only the surface, never
# $HOME (see _resolve_effective_sdd_mode).
mkdir -p "$handoff_home/.claude/skills/speckit-example"
chmod +x "$handoff_stub/herdr" "$handoff_bin/herdr-rpc" "$handoff_bin/channel-send"
printf "HW_LAUNCH_MODE='here'\nHW_NEXT_WAIT_MS='300'\n" > "$handoff_run/env"
: > "$handoff_run/done"
# A LIVE CHAINING LEASE, since 2026-09-16: that is now the whole criterion for
# launch-time reuse, so a reported pane with no lease is not offered however it
# was launched. The codex SDD handoff this file owns REQUIRES the reuse route —
# it is the one door that lets a codex-requested flow succeed, by handing the
# brief to a live claude/opencode pane — so its candidate has to be a declared
# one. The reuse DECISION stays 71-launch-reuse-check.sh's subject.
{
  printf 'state=live\n'
  printf 'until=%s\n' "$(( $(date +%s) + 3600 ))"
  printf 'reason=fixture: declared so the codex handoff has a pane to hand to\n'
} > "$handoff_run/chaining-lease"
for pane_vendor in claude opencode; do
  for mode in speckit; do
    printf '1\n' > "$handoff_run/task"
    rm -rf "$handoff_run/t2"
    : > "$handoff/send.log"
    set +e
    out="$(HOME="$handoff_home" PATH="$handoff_stub:$PATH" HANDOFF_CWD="$handoff_wd" HANDOFF_VENDOR="$pane_vendor" HANDOFF_SEND_LOG="$handoff/send.log" "$handoff_bin/hw" setup "reuse-$pane_vendor-$mode" --agent codex --model gpt-5.3-codex --sdd "$mode" --no-report --no-brief 2>&1)"
    rc=$?
    set -e
    case "$pane_vendor:$mode" in
      opencode:speckit)
        [ "$rc" -ne 0 ] || fail "Codex-requested $mode handoff to reusable OpenCode was not refused: $out"
        case "$out" in *"OpenCode framework dispatch REFUSED BEFORE BUILDING ANYTHING"*"--sdd $mode"*) ;; *) fail "Codex-requested $mode refusal to reusable OpenCode was not explicit: $out" ;; esac
        [ ! -e "$handoff_run/t2" ] || fail "Codex-requested $mode refusal to reusable OpenCode created task metadata"
        [ ! -s "$handoff/send.log" ] || fail "Codex-requested $mode refusal to reusable OpenCode sent a task"
        pass "Codex-requested $mode refuses incompatible reusable OpenCode before task state or delivery"
        continue
        ;;
    esac
    [ "$rc" = 0 ] || fail "Codex-requested $mode launch did not hand off to reusable $pane_vendor pane: $out"
    # `dispatch setup:` NO LONGER MEANS THE FRESH PATH RAN. Since 2026-09-09 the
    # REUSE route prints a manifest of its own — before that, a dry run that
    # reused a pane printed none at all, which is what made a brainer read a
    # working feature as broken. The claim here is unchanged; its discriminator
    # moves to the `route` field, which distinguishes the two paths instead of
    # merely detecting that a manifest was printed. Same correction as
    # 71-launch-reuse-check.sh.
    case "$out" in
      *"REFUSED BEFORE BUILDING ANYTHING"*|*"route       fresh executor"*|*"1/3  work directory"*)
        fail "Codex-requested $mode handoff used fresh-launch validation/building: $out" ;;
    esac
    case "$out" in
      *"route       REUSE"*) ;;
      *) fail "Codex-requested $mode handoff printed no REUSE route, so it cannot be shown to have taken the reuse path: $out" ;;
    esac
    case "$out" in *"reused pane pJD as task 2"*"no new executor was built"*) : ;; *) fail "Codex-requested $mode handoff did not expose cmd_next reuse: $out" ;; esac
    [ "$(cat "$handoff_run/task")" = 2 ] || fail "Codex-requested $mode handoff did not advance the reusable $pane_vendor task counter"
    [ -d "$handoff_run/t2" ] || fail "Codex-requested $mode handoff did not create task-2 state"
    case "$(cat "$handoff/send.log")" in *"NEXT TASK (task 2 of this session"*) : ;; *) fail "Codex-requested $mode handoff did not send cmd_next's task-2 envelope" ;; esac
    case "$(cat "$handoff/send.log")" in
      *"run /speckit-specify for this task"*"framework mode"*"for it is speckit"*) : ;;
      *) fail "Codex-requested $mode handoff did not apply the framework on the reusable $pane_vendor surface: $(cat "$handoff/send.log")" ;;
    esac
    pass "Codex-requested $mode launch hands off through hw next to reusable $pane_vendor"
  done
done
[ "$ONLY_HANDOFF" != 1 ] || exit 0

# Re-tasking reads the prior dispatch contract and must stop before t2 exists.
wd="$TMP/next/wd/task"; run="$wd/.hw/20260905-135300-1"; mkdir -p "$run"
printf '1\n' > "$run/task"; : > "$run/done"
printf '  framework   speckit  (chosen)\n' > "$run/dispatch"
cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
if [ "$1 $2" = "agent get" ]; then printf '{"result":{"agent":{"cwd":"%s","agent":"%s","agent_status":"idle"}}}\n' "$NEXT_CWD" "$NEXT_VENDOR"; fi
STUB
chmod +x "$TMP/bin/herdr"
[ ! -e "$run/t2" ] || rm -rf "$run/t2"
set +e
out="$(NEXT_CWD="$wd" NEXT_VENDOR=codex PATH="$TMP/bin:$PATH" "$SOURCE" next wC:p1 restored-task 2>&1)"
rc=$?
set -e
[ "$rc" -ne 0 ] || fail "Codex inherited next was not refused: $out"
case "$out" in *"REFUSED BEFORE BUILDING ANYTHING"*) : ;; *) fail "Codex inherited next did not refuse: $out" ;; esac
[ ! -e "$run/t2" ] || fail "Codex inherited next created task metadata before refusal"
pass "Codex inherited next refusal leaves the counter and task metadata untouched"

for pair in 'codex speckit' 'opencode speckit'; do
  read -r pane_vendor mode <<< "$pair"
  rm -rf "$run/t2"
  set +e
  out="$(NEXT_CWD="$wd" NEXT_VENDOR="$pane_vendor" PATH="$TMP/bin:$PATH" "$SOURCE" next wC:p1 --sdd "$mode" explicit-target 2>&1)"
  rc=$?
  set -e
  [ "$rc" -ne 0 ] || fail "explicit $pane_vendor --sdd $mode next was not refused: $out"
  case "$out" in *"REFUSED BEFORE BUILDING ANYTHING"*"--sdd $mode"*) ;; *) fail "explicit $pane_vendor --sdd $mode next was not explicitly refused: $out" ;; esac
  [ ! -e "$run/t2" ] || fail "explicit $pane_vendor --sdd $mode next created task metadata before refusal"
done
pass "explicit Codex and OpenCode speckit next refuse before task metadata"

# A frozen pre-correction binary produced this exact historical chain: its
# OpenCode root dispatch recorded none, then an explicit task changed the live
# worktree surface to Speckit. The current root record remains none because
# next tasks have no dispatch record of their own. The raw old-binary proof is
# preserved by suspect-framework-probe; this fixture starts at its complete
# recorded state and proves the current guard reads the effective surface before
# it creates task 3 or sends it.
printf '2\n' > "$handoff_run/task"
mkdir -p "$handoff_run/t2"
printf '  framework   none  (chosen)\n' > "$handoff_run/dispatch"
: > "$handoff/send.log"
# The live surface is read straight off disk (`_speckit_skills_in`): a
# `.claude/skills/speckit-*` directory under $handoff_wd, the exact surface
# `dirname(dirname(rundir))` resolves for this fixture's project/task.
mkdir -p "$handoff_wd/.claude/skills/speckit-example"
set +e
out="$(HOME="$handoff_home" PATH="$handoff_stub:$PATH" HANDOFF_CWD="$handoff_wd" HANDOFF_VENDOR=opencode HANDOFF_SEND_LOG="$handoff/send.log" "$handoff_bin/hw" next pJD inherited-historical-none 2>&1)"
rc=$?
set -e
[ "$rc" -ne 0 ] || fail "historical OpenCode none -> Speckit retask was accepted: $out"
case "$out" in *"OpenCode framework dispatch REFUSED BEFORE BUILDING ANYTHING"*"--sdd speckit"*) ;; *) fail "historical OpenCode refusal did not identify the measured Speckit surface: $out" ;; esac
[ "$(cat "$handoff_run/task")" = 2 ] || fail "historical OpenCode refusal advanced the task counter"
[ ! -e "$handoff_run/t3" ] || fail "historical OpenCode refusal created task-3 metadata"
[ ! -s "$handoff/send.log" ] || fail "historical OpenCode refusal sent task 3"
pass "historical OpenCode none -> Speckit implicit retask refuses before task state, delivery, or framework mutation"
rm -rf "$handoff_wd/.claude"

# This help claim is paired with the transaction immediately above: historical
# launch none must not override the measured incompatible live surface.
help_out="$("$SOURCE" help all 2>&1)"
case "$help_out" in
  *"effective live worktree surface is measured and incompatible vendor/mode"*"pairs are refused before task creation or delivery."*"only a fallback when that measurement yields no mode, not authoritative."*) ;;
  *) fail "next help does not describe the measured live-surface precedence: $help_out" ;;
esac
case "$help_out" in *"keeps whatever mode it was launched with"*) fail "next help promises historical launch precedence" ;; esac
pass "next help describes the live-surface refusal proved by the historical transaction"

# Missing measurement retains the existing historical fallback. The status
# sentence describes precedence, never claiming that a live read succeeded.
# No skill directory exists anywhere at this point in the fixture (the
# previous case cleaned up its own), so the live surface yields no mode at
# all and the dry run must fall back honestly rather than claim a measurement.
printf '  framework   none  (chosen)\n' > "$handoff_run/dispatch"
out="$(HOME="$handoff_home" PATH="$handoff_stub:$PATH" HANDOFF_CWD="$handoff_wd" HANDOFF_VENDOR=opencode HANDOFF_SEND_LOG="$handoff/send.log" "$handoff_bin/hw" next pJD --dry-run dry-fallback-mode 2>&1)"
case "$out" in
  *"compatibility checked using the effective mode"*"launch record as fallback when no live mode is obtained"*) ;;
  *) fail "next fallback dry run lacks an honest compatibility explanation: $out" ;;
esac
case "$out" in *"measured the effective live"*) fail "next fallback claims a live measurement: $out" ;; esac
[ "$(cat "$handoff_run/task")" = 2 ] && [ ! -e "$handoff_run/t3" ] || fail "fallback dry run changed task state"
pass "next fallback dry run does not claim a live measurement when no live surface is on disk"

# An explicit contract remains authoritative: none deliberately leaves the
# surface alone. It is the only explicit override compatible with an OpenCode
# pane — any orchestrated --sdd is refused for OpenCode outright — and it may
# not be rejected merely because Speckit was measured before this transition.
explicit_mode=none
printf '2\n' > "$handoff_run/task"; rm -rf "$handoff_run/t3"
: > "$handoff/send.log"
mkdir -p "$handoff_wd/.claude/skills/speckit-example"
set +e
out="$(HOME="$handoff_home" PATH="$handoff_stub:$PATH" HANDOFF_CWD="$handoff_wd" HANDOFF_VENDOR=opencode HANDOFF_SEND_LOG="$handoff/send.log" "$handoff_bin/hw" next pJD --sdd "$explicit_mode" "explicit-$explicit_mode" 2>&1)"
rc=$?
set -e
[ "$rc" = 0 ] || fail "explicit OpenCode --sdd $explicit_mode was rejected after a live Speckit surface: $out"
[ "$(cat "$handoff_run/task")" = 3 ] && [ -d "$handoff_run/t3" ] && [ -s "$handoff/send.log" ] || fail "explicit OpenCode --sdd $explicit_mode did not create and deliver task 3"
pass "explicit OpenCode --sdd none remains valid after a live Speckit surface"

# M02 — reverse _resolve_effective_sdd_mode's precedence: inherited first,
# live surface second. The historical none-dispatch fixture, with a live
# Speckit skill still on disk at $handoff_wd from the case above, must then
# send task 3 on the strength of the stale "none" dispatch record, proving the
# live-first lookup in the real function is causal rather than a
# diagnostic-only read.
mut="$TMP/stale-framework-mut"; cp -R "$handoff_bin" "$mut"
MUT_HW="$mut/hw" python3 - <<'PY'
import os
p = os.environ["MUT_HW"]
s = open(p, encoding="utf-8").read()
old = '''  mode="$requested"
  if [ -z "$mode" ]; then
    # A re-task may inherit a root dispatch record from before an explicit
    # re-task changed this pane's surface. Prefer what is on disk for THIS
    # directory — a machine-wide install alone does not put a directory in
    # play; the recorded value is only the fallback when the disk says nothing.
    _speckit_skills_in "$surface" >/dev/null && mode=speckit
    [ -n "$mode" ] || mode="$inherited"
  fi'''
new = '''  mode="$requested"
  if [ -z "$mode" ]; then
    mode="$inherited"
    [ -n "$mode" ] || { _speckit_skills_in "$surface" >/dev/null && mode=speckit; }
  fi'''
assert s.count(old) == 1, s.count(old)
open(p, "w", encoding="utf-8").write(s.replace(old, new))
PY
printf '2\n' > "$handoff_run/task"; rm -rf "$handoff_run/t3"
printf '  framework   none  (chosen)\n' > "$handoff_run/dispatch"
: > "$handoff/send.log"
mkdir -p "$handoff_wd/.claude/skills/speckit-example"
set +e
out="$(HOME="$handoff_home" PATH="$handoff_stub:$PATH" HANDOFF_CWD="$handoff_wd" HANDOFF_VENDOR=opencode HANDOFF_SEND_LOG="$handoff/send.log" "$mut/hw" next pJD inherited-historical-none 2>&1)"
rc=$?
set -e
if [ "$rc" = 0 ] && [ "$(cat "$handoff_run/task")" = 3 ] && [ -d "$handoff_run/t3" ] && [ -s "$handoff/send.log" ]; then
  pass "mutant killed: M02 stale dispatch-first resolution sends OpenCode task 3 without measuring the live Speckit surface"
else
  fail "M02 survived or misfired: rc=$rc out=$out"
fi
rm -rf "$handoff_wd/.claude"

# The Codex transport must reject unsupported stronger receipts before queueing.
cs="$TMP/cs"; mkdir -p "$cs/bin"; cp "$SOURCE_BIN/channel-send" "$cs/bin/"
cat > "$cs/bin/codex" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CODEX_QUEUE_LOG"
STUB
chmod +x "$cs/bin/codex"; : > "$TMP/queue.log"
for flags in '--id r1' '--require processed'; do
  out="$(CODEX_QUEUE_LOG="$TMP/queue.log" PATH="$cs/bin:$PATH" "$cs/bin/channel-send" $flags codex thread socket message 2>&1 || true)"
  case "$out" in *"does not yet expose"*) : ;; *) fail "Codex transport accepted unsupported $flags: $out" ;; esac
done
[ ! -s "$TMP/queue.log" ] || fail "Codex queue ran despite unsupported receipt/id: $(cat "$TMP/queue.log")"
pass "Codex transport rejects id and processed contracts before queueing"

# Drive the real readiness function against its three observed buffers.
awk '/^_codex_startup_ready\(\) \{/,/^}/' "$SOURCE" > "$TMP/ready.sh"
ready() { READY_SCREEN="$1" READY_LOG="$TMP/ready.log" bash -c 'herdr(){ [ "$1 $2" = "agent read" ] && { printf "%s" "$READY_SCREEN"; return; }; printf "%s\n" "$*" >> "$READY_LOG"; }; source "$1"; _codex_startup_ready c' _ "$TMP/ready.sh"; }
: > "$TMP/ready.log"; ready 'Do you trust the contents of this directory' || fail "trust dialog was not handled"
grep -q 'agent send-keys c enter' "$TMP/ready.log" || fail "trust dialog did not receive enter"
: > "$TMP/ready.log"; ready 'Hooks need review' && fail "hook trust dialog was accepted"
: > "$TMP/ready.log"; ready 'Choose how you'\''d like Codex to proceed' && fail "model dialog was accepted"
: > "$TMP/ready.log"; ready 'normal prompt' || fail "normal Codex prompt was rejected"
[ ! -s "$TMP/ready.log" ] || fail "normal prompt received startup keys"
pass "Codex readiness handles trust, refuses blocking dialogs, and leaves a normal prompt untouched"
