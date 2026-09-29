#!/usr/bin/env bash
# OpenCode dispatches name the primary that will actually execute the task.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SOURCE="${HW_SOURCE:-$ROOT/bin/hw}"
HOME_FIX="$TMP/home"
MUTANTS="$TMP/mutants"
mkdir -p "$HOME_FIX/.config/opencode" "$MUTANTS"
cat > "$HOME_FIX/.config/opencode/opencode.json" <<'JSON'
{"agent":{"direct-worker":{"mode":"primary"},"gentle-orchestrator":{"mode":"primary"},"sol-orchestrator":{"mode":"primary"}}}
JSON

# Extract and execute the real argument constructor. This keeps every case arm
# intact while replacing only the port allocator and output helpers at its edge.
extract_agent_args() {
  local source="$1" fragment="$2"
  python3 - "$source" "$fragment" <<'PY'
import sys
src = open(sys.argv[1], encoding="utf-8").read()
start = src.index("_build_agent_args() {\n")
end = src.index("\n# ── SDD mode, per task", start)
open(sys.argv[2], "w", encoding="utf-8").write(src[start:end])
PY
}

run_args() {
  local source="$1" agent="$2" sdd="$3" override="$4" output="$5"
  local model="${6:-}"
  local fragment="$TMP/args-$(basename "$source")-$agent-$sdd-${override:-empty}.sh"
  extract_agent_args "$source" "$fragment"
  if [ "$override" = __UNSET__ ]; then
    env -u HW_OPENCODE_AGENT HOME="$HOME_FIX" FRAGMENT="$fragment" AGENT_VALUE="$agent" SDD_VALUE="$sdd" MODEL_VALUE="$model" bash -c '
      set -euo pipefail
      AGENT="$AGENT_VALUE"; SDD="$SDD_VALUE"; MODEL="$MODEL_VALUE"; EFFORT=""
      _free_channel_port() { printf 51234; }
      info() { printf "INFO:%s\n" "$*" >&2; }
      warn() { printf "WARN:%s\n" "$*" >&2; }
      source "$FRAGMENT"
      _build_agent_args
      printf "ARGS=%s\nPRIMARY=%s\n" "$AGENT_ARGS" "${OPENCODE_PRIMARY_AGENT:-}"
    ' > "$output" 2>&1
  else
    env HW_OPENCODE_AGENT="$override" HOME="$HOME_FIX" FRAGMENT="$fragment" AGENT_VALUE="$agent" SDD_VALUE="$sdd" MODEL_VALUE="$model" bash -c '
      set -euo pipefail
      AGENT="$AGENT_VALUE"; SDD="$SDD_VALUE"; MODEL="$MODEL_VALUE"; EFFORT=""
      _free_channel_port() { printf 51234; }
      info() { printf "INFO:%s\n" "$*" >&2; }
      warn() { printf "WARN:%s\n" "$*" >&2; }
      source "$FRAGMENT"
      _build_agent_args
      printf "ARGS=%s\nPRIMARY=%s\n" "$AGENT_ARGS" "${OPENCODE_PRIMARY_AGENT:-}"
    ' > "$output" 2>&1
  fi
}

contains() { case "$(<"$1")" in *"$2"*) return 0 ;; *) return 1 ;; esac; }
args_line() { grep '^ARGS=' "$1" || true; }
has_agent() { case "$(args_line "$1")" in *"--agent $2"*) return 0 ;; *) return 1 ;; esac; }
has_any_agent() { case "$(args_line "$1")" in *"--agent "*) return 0 ;; *) return 1 ;; esac; }

run_args "$SOURCE" opencode none __UNSET__ "$TMP/sol-none.out" openai/gpt-5.6-sol
has_agent "$TMP/sol-none.out" sol-orchestrator && contains "$TMP/sol-none.out" 'PRIMARY=sol-orchestrator' \
  || fail "C01 framework-free Sol does not pass the orchestration primary"
pass "C01 framework-free Sol passes the explicit orchestration primary"

run_args "$SOURCE" opencode none __UNSET__ "$TMP/none.out" openai/gpt-5.6-terra
has_agent "$TMP/none.out" direct-worker && contains "$TMP/none.out" 'PRIMARY=direct-worker' \
  || fail "C01b non-Sol OpenCode lost the explicit direct-worker primary"
pass "C01b non-Sol OpenCode preserves direct-worker's inline role"

run_args "$SOURCE" opencode speckit __UNSET__ "$TMP/sol-speckit.out" openai/gpt-5.6-sol
has_agent "$TMP/sol-speckit.out" direct-worker \
  || fail "C01c Sol selection leaked into a framework phase path"
pass "C01c framework modes remain on direct-worker; SDD phase routing is untouched"

run_args "$SOURCE" opencode none '' "$TMP/empty.out"
if has_any_agent "$TMP/empty.out"; then
  fail "C03 an empty HW_OPENCODE_AGENT override still passes an agent"
fi
contains "$TMP/empty.out" 'explicitly empty' \
  || fail "C03 the empty override is not identified as an intentional opt-out"
pass "C03 HW_OPENCODE_AGENT empty remains the explicit no-agent opt-out"

run_args "$SOURCE" opencode none missing-worker "$TMP/missing.out"
if has_agent "$TMP/missing.out" missing-worker; then
  fail "C04 an undefined override was passed to OpenCode"
fi
contains "$TMP/missing.out" 'WARN:opencode wanted --agent missing-worker' &&
  contains "$TMP/missing.out" 'continuing without --agent' \
  || fail "C04 an undefined agent did not warn and continue without the flag"
pass "C04 an undefined agent warns and continues without --agent"

run_args "$SOURCE" claude none __UNSET__ "$TMP/claude.out"
run_args "$SOURCE" codex none __UNSET__ "$TMP/codex.out"
if has_any_agent "$TMP/claude.out" || has_any_agent "$TMP/codex.out"; then
  fail "C05 the OpenCode primary selection leaked into Claude or Codex"
fi
pass "C05 Claude and Codex argument construction is untouched"

# The full dry-run proves the selected identity reaches the review-facing
# manifest, not only the process argv assembled by the extracted function.
manifest="$(HOME="$HOME_FIX" "$ROOT/bin/hw" setup oc-primary-probe --agent opencode --model openai/gpt-5.6-sol --sdd none --no-report --dry-run 2>&1 || true)"
case "$manifest" in
  *"primary     sol-orchestrator"*"explicit --agent passed by hw"*) ;;
  *) fail "C06 the dry-run manifest does not name sol-orchestrator as the explicit primary" ;;
esac
pass "C06 the Sol dry-run manifest names the explicit orchestration primary"

run_args "$SOURCE" opencode none gentle-orchestrator "$TMP/override-role.out"
contains "$TMP/override-role.out" 'INFO:opencode → --agent gentle-orchestrator (explicit HW_OPENCODE_AGENT override)' &&
  ! contains "$TMP/override-role.out" '(direct worker; no orchestration)' \
  || fail "C07 an explicit override was mislabeled with the SDD-derived default role"
pass "C07 an explicit HW_OPENCODE_AGENT override is labeled as an override without inferring its role"

mutate() {
  local name="$1"
  local old="$2"
  local new="$3"
  local output="$MUTANTS/$name"
  python3 - "$SOURCE" "$output" "$old" "$new" <<'PY'
import sys
src, dst, old, new = sys.argv[1:]
text = open(src, encoding="utf-8").read()
if text.count(old) != 1:
    raise SystemExit("mutation anchor count is not one: %r (%d)" % (old[:80], text.count(old)))
open(dst, "w", encoding="utf-8").write(text.replace(old, new))
PY
  chmod +x "$output"
  printf '%s' "$output"
}

# Mutation arms are ungated: setup/test-hw must execute every declared proof.
M1="$(mutate m01 'default_oc_agent="sol-orchestrator"' 'default_oc_agent="direct-worker"')" || fail "M01 could not be built"
run_args "$M1" opencode none __UNSET__ "$TMP/m01.out" openai/gpt-5.6-sol
has_agent "$TMP/m01.out" sol-orchestrator && fail "M01 SURVIVED: Sol may regress to inline execution"
pass "mutant killed: M01 replacing the Sol orchestrator makes C01 fail"

M3="$(mutate m03 '[ "${HW_OPENCODE_AGENT+x}" = x ]' '[ -n "${HW_OPENCODE_AGENT:-}" ]')" || fail "M03 could not be built"
run_args "$M3" opencode none '' "$TMP/m03.out"
has_any_agent "$TMP/m03.out" || fail "M03 SURVIVED: empty override still disabled the agent after :- mutation"
pass "mutant killed: M03 treating empty as unset destroys the explicit opt-out"

M4="$(mutate m04 'if jq -e --arg a "$oc_agent"' 'if false && jq -e --arg a "$oc_agent"')" || fail "M04 could not be built"
run_args "$M4" opencode none __UNSET__ "$TMP/m04.out" openai/gpt-5.6-sol
has_agent "$TMP/m04.out" sol-orchestrator && fail "M04 SURVIVED: bypassing config verification still passed the agent"
pass "mutant killed: M04 rejecting the configured agent prevents C01 from passing"

M5="$(mutate m05 'warn "opencode wanted --agent $oc_agent ($oc_role), but it is not defined in $oc_conf"' ': # warning removed')" || fail "M05 could not be built"
run_args "$M5" opencode none missing-worker "$TMP/m05.out"
contains "$TMP/m05.out" 'WARN:opencode wanted --agent missing-worker' && fail "M05 SURVIVED: removed warning remained visible"
pass "mutant killed: M05 removing the undefined-agent warning makes C04 fail"

M6="$(mutate m06 \
  $'  OPENCODE_PRIMARY_AGENT=""\n  case "$AGENT" in' \
  $'  OPENCODE_PRIMARY_AGENT=""\n  [ "$AGENT" = claude ] && AGENT=opencode\n  case "$AGENT" in')" || fail "M06 could not be built"
run_args "$M6" claude none __UNSET__ "$TMP/m06-claude.out"
contains "$TMP/m06-claude.out" '--agent direct-worker' || fail "M06 SURVIVED: widening the OpenCode arm did not contaminate Claude"
pass "mutant killed: M06 widening the OpenCode case arm contaminates Claude, which C05 refuses"

M7="$(mutate m07 'oc_role="explicit HW_OPENCODE_AGENT override"' ': # keep the SDD-derived default role')" || fail "M07 could not be built"
run_args "$M7" opencode none gentle-orchestrator "$TMP/m07.out"
contains "$TMP/m07.out" '(direct worker; no orchestration)' \
  || fail "M07 SURVIVED: dropping override labeling did not restore the false SDD-derived role"
pass "mutant killed: M07 dropping override labeling restores the false SDD-derived role, which C07 refuses"

printf 'mapping - C01↔M01/M04 · C01b direct preservation · C01c framework isolation · C03↔M03 · C04↔M05 · C05↔M06 · C06 full manifest · C07↔M07 override role\n'
printf 'coverage - 8 behavior claims, 6 dedicated mutants killed\n'
