#!/usr/bin/env bash
# OpenCode dispatches name the primary that will actually execute the task.
#
# Driven through the dry run of `hw` (the shim, then hw-core): the argv hw builds for opencode is printed on the
# `agent args:` line and the chosen primary on the manifest's `primary` line. (Until go-060 S4 this extracted
# `_build_agent_args` from bin/hw and mutated the text of that copy; the mutants' claims now sit on hw-core's
# anchors 844-B51..B57 and the vd-oc-* goldens of contract-hw.)
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HOME_FIX="$TMP/home"
mkdir -p "$HOME_FIX/.config/opencode"
cat > "$HOME_FIX/.config/opencode/opencode.json" <<'JSON'
{"agent":{"direct-worker":{"mode":"primary"},"gentle-orchestrator":{"mode":"primary"},"sol-orchestrator":{"mode":"primary"}}}
JSON

# run_args <agent> <sdd: none|unset> <override: __UNSET__|value (may be empty)> <output file> [model]
run_args() {
  local agent="$1" sdd="$2" override="$3" output="$4" model="${5:-}"
  local args=(setup oc-primary-probe --agent "$agent")
  [ -z "$model" ] || args+=(--model "$model")
  [ "$sdd" != none ] || args+=(--sdd none)
  (
    if [ "$override" = __UNSET__ ]; then unset HW_OPENCODE_AGENT; else export HW_OPENCODE_AGENT="$override"; fi
    export HOME="$HOME_FIX"
    hw_dry "${args[@]}"
  ) > "$output" 2>&1 || true
}

contains() { case "$(<"$1")" in *"$2"*) return 0 ;; *) return 1 ;; esac; }
args_line() { rg '· agent args:' "$1" || true; }
has_agent() { case "$(args_line "$1")" in *"--agent $2"*) return 0 ;; *) return 1 ;; esac; }
has_any_agent() { case "$(args_line "$1")" in *"--agent "*) return 0 ;; *) return 1 ;; esac; }

run_args opencode none __UNSET__ "$TMP/sol-none.out" openai/gpt-5.6-sol
has_agent "$TMP/sol-none.out" sol-orchestrator && contains "$TMP/sol-none.out" 'primary     sol-orchestrator' \
  || fail "C01 framework-free Sol does not pass the orchestration primary: $(<"$TMP/sol-none.out")"
pass "C01 framework-free Sol passes the explicit orchestration primary"

run_args opencode none __UNSET__ "$TMP/none.out" openai/gpt-5.6-terra
has_agent "$TMP/none.out" direct-worker && contains "$TMP/none.out" 'primary     direct-worker' \
  || fail "C01b non-Sol OpenCode lost the explicit direct-worker primary: $(<"$TMP/none.out")"
pass "C01b non-Sol OpenCode preserves direct-worker's inline role"

# no --sdd none: the framework is not "none", so the Sol selection must not apply
run_args opencode unset __UNSET__ "$TMP/sol-speckit.out" openai/gpt-5.6-sol
has_agent "$TMP/sol-speckit.out" direct-worker \
  || fail "C01c Sol selection leaked into a framework phase path: $(<"$TMP/sol-speckit.out")"
pass "C01c framework modes remain on direct-worker; SDD phase routing is untouched"

run_args opencode none '' "$TMP/empty.out"
if has_any_agent "$TMP/empty.out"; then
  fail "C03 an empty HW_OPENCODE_AGENT override still passes an agent"
fi
contains "$TMP/empty.out" 'explicitly empty' \
  || fail "C03 the empty override is not identified as an intentional opt-out: $(<"$TMP/empty.out")"
pass "C03 HW_OPENCODE_AGENT empty remains the explicit no-agent opt-out"

run_args opencode none missing-worker "$TMP/missing.out"
if has_agent "$TMP/missing.out" missing-worker; then
  fail "C04 an undefined override was passed to OpenCode"
fi
contains "$TMP/missing.out" 'opencode wanted --agent missing-worker' &&
  contains "$TMP/missing.out" 'continuing without --agent' \
  || fail "C04 an undefined agent did not warn and continue without the flag: $(<"$TMP/missing.out")"
pass "C04 an undefined agent warns and continues without --agent"

run_args claude none __UNSET__ "$TMP/claude.out"
# codex prints an `agent args:` line only when it has args: a model gives it one, so "no --agent" is read off a line that exists
run_args codex none __UNSET__ "$TMP/codex.out" gpt-5.5
if has_any_agent "$TMP/claude.out" || has_any_agent "$TMP/codex.out"; then
  fail "C05 the OpenCode primary selection leaked into Claude or Codex"
fi
[ -n "$(args_line "$TMP/claude.out")" ] && [ -n "$(args_line "$TMP/codex.out")" ] \
  || fail "C05 the dry run printed no agent args line for claude or codex, so nothing was checked: $(<"$TMP/claude.out")"
pass "C05 Claude and Codex argument construction is untouched"

# The full dry-run proves the selected identity reaches the review-facing manifest.
manifest="$(HOME="$HOME_FIX" "$ROOT/bin/hw" setup oc-primary-probe --agent opencode --model openai/gpt-5.6-sol --sdd none --no-report --no-brief --dry-run 2>&1 || true)"
case "$manifest" in
  *"primary     sol-orchestrator"*"explicit --agent passed by hw"*) ;;
  *) fail "C06 the dry-run manifest does not name sol-orchestrator as the explicit primary" ;;
esac
pass "C06 the Sol dry-run manifest names the explicit orchestration primary"

run_args opencode none gentle-orchestrator "$TMP/override-role.out"
contains "$TMP/override-role.out" 'opencode → --agent gentle-orchestrator (explicit HW_OPENCODE_AGENT override)' &&
  ! contains "$TMP/override-role.out" '(direct worker; no orchestration)' \
  || fail "C07 an explicit override was mislabeled with the SDD-derived default role: $(<"$TMP/override-role.out")"
pass "C07 an explicit HW_OPENCODE_AGENT override is labeled as an override without inferring its role"

printf 'mapping - C01 Sol primary · C01b direct preservation · C01c framework isolation · C03 empty opt-out · C04 undefined agent · C05 claude/codex untouched · C06 full manifest · C07 override role\n'
printf 'coverage - 8 behavior claims; the bash text mutants M01-M07 are hw-core anchors 844-B51..B57 (contract-hw mutants-t5-vendor)\n'
