#!/usr/bin/env bash
# A PANE REOPENED BY --reset OR REVIVE CARRIES THE COCKPIT'S WORK ROOT, AND AN OPT-OUT STARTS NO WRITER
#
# T5's judges flagged two things it left open:
#   1. `herdr agent start` on an existing pane does not re-apply the pane's env, so a pane opened before
#      HW_COCKPIT_WORK existed (or without it) was reset/revived with no work root and the mod said
#      "writer down (no state path)" with the writer healthy. brain now exports it into the pane's shell
#      before it starts claude there.
#   2. the heartbeat (`cockpit-state --loop`) started whatever FOREMAN_COCKPIT or --no-cockpit said: an
#      opt-out brainer had a writer nobody reads.
#
#     bash setup/tests/804-a-reopened-brainer-keeps-its-cockpit.sh
#
# BRAIN_SOURCE runs it against another bin/brain, for the old/new evidence.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BH="$TMP/tree/brain"
mkdir -p "$BH/bin" "$BH/setup" "$TMP/stub" "$TMP/state"
cp "$TMP/bin/claude-personal" "$TMP/stub/claude-personal"
cp "${BRAIN_SOURCE:-$ROOT/bin/brain}" "$BH/bin/brain"
cp "$ROOT/bin/engram-label-proxy" "$ROOT/bin/project-spaces.sh" "$ROOT/bin/runenv" "$ROOT/bin/msys-compat.sh" "$ROOT/bin/sitecustomize.py" "$BH/bin/"
cp "$ROOT/projects.json" "$BH/"
chmod +x "$BH/bin/brain"
printf '# setup brainer\n' > "$BH/setup/CLAUDE.md"
LANE_DIR="$BH/setup"
SPACE="$(jq -r '.lanes.setup.space' "$BH/projects.json")"
# the writer, as a recorder: brain only has to START it
cat > "$BH/bin/cockpit-state" <<'W'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${STUB_WRITER:?}"
W
chmod +x "$BH/bin/cockpit-state"

cat > "$TMP/stub/herdr" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${STUB_CALLS:?}"
S="${STUB_STATE:?}"
case "$1 $2" in
  "workspace list") printf '{"result":{"workspaces":[{"workspace_id":"wX","label":"%s"}]}}\n' "${STUB_SPACE:?}" ;;
  "tab list")       printf '{"result":{"tabs":[{"tab_id":"wX:t1","workspace_id":"wX","label":"brainer"}]}}\n' ;;
  "pane list")      cat "$S/panes.json" ;;
  "agent get")      printf '{"error":{"code":"agent_not_found","message":"agent target not found"}}\n' >&2; exit 1 ;;
  "agent start")    printf '{"result":{"agent":{"interactive_ready":true}}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
cat > "$TMP/stub/claude" <<'STUB'
#!/usr/bin/env bash
[ "${1:-}" = --version ] && { printf '2.1.291 (Claude Code)\n'; exit 0; }
exit 0
STUB
chmod +x "$TMP/stub/herdr" "$TMP/stub/claude"
printf '{"result":{"panes":[{"pane_id":"wX:p1","tab_id":"wX:t1","workspace_id":"wX","label":"brain","cwd":"%s","agent":"","agent_status":"unknown"}]}}' \
  "$LANE_DIR" > "$TMP/state/panes.json"

run() {  # <label> <env assignments...> -- <brain args...>
  local label="$1"; shift
  local -a envs=()
  while [ $# -gt 0 ] && [ "$1" != -- ]; do envs+=("$1"); shift; done
  shift || true
  : > "$TMP/calls-$label"; rm -f "$TMP/writer-$label"
  env -i PATH="$TMP/stub:/usr/bin:/bin:/usr/sbin:/sbin" HOME="$TMP/home" TMPDIR="$TMP" \
      STUB_SPACE="$SPACE" STUB_CALLS="$TMP/calls-$label" STUB_STATE="$TMP/state" STUB_WRITER="$TMP/writer-$label" \
      "${envs[@]+"${envs[@]}"}" "$BH/bin/brain" setup "$@" > "$TMP/out-$label" 2>&1 || true
  sleep 0.5   # the writer is detached
}
want_work="$(jq -r '.work' "$BH/projects.json" | sd '^~' "$TMP/home")"
export_line="pane run wX:p1 export HW_COCKPIT_WORK=$want_work"
check_export() {  # <label> <what>
  grep -qxF -- "$export_line" "$TMP/calls-$1" \
    || fail "804: $2 did not export HW_COCKPIT_WORK=$want_work into the pane: $(tr '\n' '|' < "$TMP/calls-$1") :: $(cat "$TMP/out-$1")"
  local e s
  e="$(rg -n -m1 -xF -- "$export_line" "$TMP/calls-$1" | cut -d: -f1)"
  s="$(rg -n -m1 '^agent start ' "$TMP/calls-$1" | cut -d: -f1)"
  [ -n "$s" ] && [ "$e" -lt "$s" ] || fail "804: $2 exported after (or without) starting claude, so claude would not inherit it"
}

run reset "STUB_GET=absent" -- --reset
check_export reset "--reset"
pass "804: --reset puts HW_COCKPIT_WORK into the pane's shell before claude starts"

run revive -- 
check_export revive "a revive (the agent exited, a bare shell)"
pass "804: a revive does too"

# the heartbeat obeys the opt-out
run on -- --reset
[ -s "$TMP/writer-on" ] || fail "804: with the cockpit on, brain started no writer: $(cat "$TMP/out-on")"
run off1 "FOREMAN_COCKPIT=0" -- --reset
[ ! -e "$TMP/writer-off1" ] || fail "804: FOREMAN_COCKPIT=0 still started the writer loop: $(cat "$TMP/writer-off1")"
run off2 -- --reset --no-cockpit
[ ! -e "$TMP/writer-off2" ] || fail "804: --no-cockpit still started the writer loop: $(cat "$TMP/writer-off2")"
pass "804: FOREMAN_COCKPIT=0 and --no-cockpit start no writer; the default does"

mut() {  # <name> <from> <to>
  cp "$ROOT/bin/brain" "$TMP/mut-brain"
  python3 -I - "$TMP/mut-brain" "$2" "$3" <<'PY' || fail "804: the mutant '$1' targets text that is not in bin/brain"
import sys
p, a, b = sys.argv[1:4]
s = open(p).read()
if a not in s: sys.exit(1)
open(p, "w").write(s.replace(a, b, 1))
PY
  chmod +x "$TMP/mut-brain"
  ( BRAIN_SOURCE="$TMP/mut-brain" bash "$0" --as-mutant ) >"$TMP/mut.out" 2>&1 && fail "804: mutant '$1' survived"
  return 0
}
if [ "${1:-}" != --as-mutant ] && [ -z "${BRAIN_SOURCE:-}" ]; then
  mut "reset/revive do not export the work root" 'herdr pane run "$pane" "export HW_COCKPIT_WORK=' 'herdr pane list "$pane" "export HW_COCKPIT_WORK='
  mut "the writer ignores the opt-out" '[ "$NO_COCKPIT" = 0 ] && [ "${FOREMAN_COCKPIT:-1}" != 0 ] || return 0' ':'
  pass "804: 2 mutants of bin/brain, each killed"
fi
