#!/usr/bin/env bash
# `brain <lane>` LOADS THE COCKPIT MOD FOR A CLAUDE BRAINER THAT CAN RUN IT, AND SAYS SO WHEN IT CANNOT.
#
# The mod is a session-only plugin folder: `--plugin-dir <installed>/cockpit`,
# resolved from bin/brain's own physical path, so a foreman-sh upgrade replaces
# the tree and the mod with it and nothing is written to ~/.claude. The flag goes
# on only for the claude vendor, only with claude >= 2.1.289, and only when
# neither FOREMAN_COCKPIT=0 nor --no-cockpit is set. Below the floor it is one
# line on stderr and the brainer opens anyway; an opt-out says nothing; an
# opencode brainer is not told anything (it has no mods).
#
#     bash setup/tests/803-brain-loads-the-cockpit-when-it-can.sh
#
# BRAIN_SOURCE runs it against another bin/brain, for the old/new evidence.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BH="$TMP/tree/brain"
mkdir -p "$BH/bin" "$BH/demo" "$BH/demo2" "$BH/cockpit/.claude-plugin" "$TMP/stub"
cp "${BRAIN_SOURCE:-$ROOT/bin/brain}" "$BH/bin/brain"
cp "$ROOT/bin/engram-label-proxy" "$ROOT/bin/project-spaces.sh" "$ROOT/bin/runenv" "$ROOT/bin/msys-compat.sh" "$ROOT/bin/sitecustomize.py" "$BH/bin/"
chmod +x "$BH/bin/brain"
BHP="$(cd -P "$BH" && pwd)"
printf '{"name":"cockpit","version":"0.1.0"}\n' > "$BH/cockpit/.claude-plugin/plugin.json"
jq '.lanes.demo.space = "demo" | .lanes.demo2 = (.lanes.setup | .vendor = "opencode" | .engram = "demo2label" | .space = "demo2" | .hw_aliases = [] | .brain_aliases = [])' \
  "$ROOT/projects.json" > "$BH/projects.json"
printf '# demo\n' > "$BH/demo/CLAUDE.md"
printf '# demo2\n' > "$BH/demo2/CLAUDE.md"; printf '{}\n' > "$BH/demo2/opencode.json"

cat > "$TMP/stub/herdr" <<'STUB'
#!/usr/bin/env bash
for a in "$@"; do printf '%s\n' "$a"; done > "${STUB_CALLS:?}.$1-$2"
case "$1 $2" in
  "workspace list") printf '{"result":{"workspaces":[]}}\n' ;;
  "workspace create") printf '{"result":{"workspace":{"workspace_id":"wX"},"tab":{"tab_id":"wX:t1"},"root_pane":{"pane_id":"wX:p1"}}}\n' ;;
  "pane list")    printf '{"result":{"panes":[{"pane_id":"wX:p1","tab_id":"wX:t1","workspace_id":"wX"}]}}\n' ;;
  "agent start")  printf '{"result":{"agent":{"interactive_ready":true}}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
cat > "$TMP/stub/claude" <<'STUB'
#!/usr/bin/env bash
[ "${1:-}" = --version ] && { printf '%s (Claude Code)\n' "${STUB_CLAUDE_VERSION:?}"; exit 0; }
exit 0
STUB
chmod +x "$TMP/stub/herdr" "$TMP/stub/claude"

# run <version> <brain args...> — extra env in $ENVX. stdout+stderr of brain in $TMP/out, herdr's agent-start argv in $TMP/calls.agent-start
run() {
  local v="$1"; shift
  rm -f "$TMP"/calls.* "$TMP/out"
  # shellcheck disable=SC2086
  env -i PATH="$TMP/stub:/usr/bin:/bin:/usr/sbin:/sbin" HOME="$TMP/home" TMPDIR="$TMP" STUB_CALLS="$TMP/calls" \
      STUB_CLAUDE_VERSION="$v" ${ENVX:-} "$BH/bin/brain" "$@" > "$TMP/out" 2>&1 || true
}
flag_dir() { grep -A1 -x -- '--plugin-dir' "$TMP/calls.agent-start" 2>/dev/null | tail -1; }
cockpit_lines() { grep -c 'cockpit' "$TMP/out" || true; }
need_started() { [ -f "$TMP/calls.agent-start" ] || fail "$1: brain started no agent: $(cat "$TMP/out")"; }

run 2.1.291 demo; need_started "2.1.291"
[ "$(flag_dir)" = "$BHP/cockpit" ] || fail "a claude brainer above the floor was not launched with --plugin-dir $BHP/cockpit: $(tr '\n' ' ' < "$TMP/calls.agent-start")"
[ "$(cockpit_lines)" = 0 ] || fail "brain said something about the cockpit when it loaded it: $(cat "$TMP/out")"
pass "803: claude 2.1.291 gets --plugin-dir <installed>/cockpit, silently"
# The mod finds its state file from the pane's environment, and `$WORK` is a shell variable that is never
# exported: the pane is created with the work root as HW_COCKPIT_WORK (measured live, the mod said "no state path").
want_work="$(jq -r '.work' "$BH/projects.json" | sed "s#^~#$TMP/home#")"
[ -f "$TMP/calls.workspace-create" ] || fail "803: brain created no workspace: $(cat "$TMP/out")"
grep -qx -- "HW_COCKPIT_WORK=$want_work" "$TMP/calls.workspace-create" \
  || fail "803: the brainer's pane was not created with HW_COCKPIT_WORK=$want_work: $(tr '\n' ' ' < "$TMP/calls.workspace-create")"
pass "803: the brainer's pane carries the work root as HW_COCKPIT_WORK, so the mod finds its state file"

run 2.1.289 demo; need_started "2.1.289"
[ "$(flag_dir)" = "$BHP/cockpit" ] || fail "claude exactly at the floor did not get the flag"
pass "803: the floor itself loads it"

run 2.1.288 demo; need_started "2.1.288"
[ -z "$(flag_dir)" ] || fail "claude below the floor was given the flag: $(tr '\n' ' ' < "$TMP/calls.agent-start")"
[ "$(cockpit_lines)" = 1 ] || fail "below the floor brain must print exactly one cockpit line, printed $(cockpit_lines): $(cat "$TMP/out")"
grep -qF 'cockpit needs claude >= 2.1.289 (found 2.1.288); running without it' "$TMP/out" || fail "the line does not name the floor and the version found: $(cat "$TMP/out")"
pass "803: claude 2.1.288 opens without the flag and says why in one line"

run 2.0.5 demo; need_started "2.0.5"
[ -z "$(flag_dir)" ] || fail "a lower major version was given the flag"
run 2.1.1000 demo; need_started "2.1.1000"
[ "$(flag_dir)" = "$BHP/cockpit" ] || fail "2.1.1000 (a longer patch) was refused: versions are compared as numbers, not text"
pass "803: versions compare as numbers"

ENVX="FOREMAN_COCKPIT=0" run 2.1.291 demo; need_started "FOREMAN_COCKPIT=0"
[ -z "$(flag_dir)" ] && [ "$(cockpit_lines)" = 0 ] || fail "FOREMAN_COCKPIT=0 did not opt out silently: $(tr '\n' ' ' < "$TMP/calls.agent-start") :: $(cat "$TMP/out")"
run 2.1.291 demo --no-cockpit; need_started "--no-cockpit"
[ -z "$(flag_dir)" ] && [ "$(cockpit_lines)" = 0 ] || fail "--no-cockpit did not opt out silently: $(cat "$TMP/out")"
ENVX="FOREMAN_COCKPIT=0" run 2.1.200 demo --no-cockpit; need_started "opt-out below the floor"
[ "$(cockpit_lines)" = 0 ] || fail "an opt-out below the floor still printed the floor line: $(cat "$TMP/out")"
pass "803: FOREMAN_COCKPIT=0 and --no-cockpit omit the flag and say nothing"

run 2.1.291 demo2; need_started "opencode"
[ -z "$(flag_dir)" ] && [ "$(cockpit_lines)" = 0 ] || fail "an opencode brainer was given the cockpit or told about it: $(tr '\n' ' ' < "$TMP/calls.agent-start") :: $(cat "$TMP/out")"
run 2.1.100 demo2; need_started "opencode below the floor"
[ "$(cockpit_lines)" = 0 ] || fail "an opencode brainer was told about the claude floor: $(cat "$TMP/out")"
pass "803: an opencode brainer gets no flag and no message"

mv "$BH/cockpit" "$BH/cockpit.away"
run 2.1.291 demo; need_started "no mod on disk"
[ -z "$(flag_dir)" ] && [ "$(cockpit_lines)" = 1 ] || fail "with no cockpit folder brain must open without the flag and print one line: $(cat "$TMP/out")"
mv "$BH/cockpit.away" "$BH/cockpit"
pass "803: a tree with no cockpit folder opens without it and says so"

# Old/new: the base's brain never passes the flag. Skipped once the base has it (after the merge).
old="$TMP/old-brain"
if git -C "$ROOT" show "$(git -C "$ROOT" merge-base HEAD main 2>/dev/null)":bin/brain > "$old" 2>/dev/null && ! grep -q -- '--plugin-dir' "$old"; then
  cp "$BH/bin/brain" "$TMP/new-brain"; cp "$old" "$BH/bin/brain"; chmod +x "$BH/bin/brain"
  run 2.1.291 demo; need_started "old brain"
  [ -z "$(flag_dir)" ] || fail "the old brain passed --plugin-dir"
  cp "$TMP/new-brain" "$BH/bin/brain"
  pass "803: old/new — the base's brain passes no flag at 2.1.291; this one does"
fi

# Mutants of bin/brain: each must turn one of the assertions above red.
mut() {  # <name> <from> <to> <needle in output of a failing run>
  cp "$ROOT/bin/brain" "$TMP/mut-brain"
  python3 -I - "$TMP/mut-brain" "$2" "$3" <<'PY' || fail "803: the mutant '$1' targets text that is not in bin/brain"
import sys
p, a, b = sys.argv[1:4]
s = open(p).read()
if a not in s: sys.exit(1)
open(p, "w").write(s.replace(a, b, 1))
PY
  chmod +x "$TMP/mut-brain"
  ( BRAIN_SOURCE="$TMP/mut-brain" bash "$0" --as-mutant ) >"$TMP/mut.out" 2>&1 && fail "803: mutant '$1' survived"
  return 0
}
if [ "${1:-}" != --as-mutant ] && [ -z "${BRAIN_SOURCE:-}" ]; then
  mut "the flag is passed below the floor" '! _version_at_least "$cockpit_ver" "$cockpit_floor"' 'false'
  mut "the opt-out is ignored" '[ "${FOREMAN_COCKPIT:-1}" != 0 ]; then
    cockpit_dir' 'true; then
    cockpit_dir'
  mut "--no-cockpit is ignored" 'if [ "$NO_COCKPIT" = 0 ] && [ "${FOREMAN_COCKPIT:-1}" != 0 ]; then' 'if [ "${FOREMAN_COCKPIT:-1}" != 0 ]; then'
  mut "the floor message is dropped" 'echo "brain: cockpit needs claude' ': "brain: cockpit needs claude'
  mut "the flag goes to opencode too" 'agent_args=(-- --auto --port "$opencode_port")' 'agent_args=(-- --auto --port "$opencode_port" --plugin-dir "$BRAIN_BIN/../cockpit")'
  mut "only the major version is compared" 'for i in 0 1 2; do' 'for i in 0; do'
  mut "the flag points at a different folder" 'agent_args+=(--plugin-dir "$cockpit_dir")' 'agent_args+=(--plugin-dir "$BRAIN_BIN")'
  mut "the work root is not passed to the pane" '[ "$kind" != claude ] || [ -z "${WORK:-}" ] ||' 'true ||'
  pass "803: 8 mutants of bin/brain, each killed"
fi
