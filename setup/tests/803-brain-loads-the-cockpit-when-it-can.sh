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
mkdir -p "$BH/bin" "$BH/lib/hw" "$BH/demo" "$BH/demo2" "$BH/cockpit/.claude-plugin" "$TMP/stub"
cp "$ROOT/lib/hw/deps.sh" "$ROOT/lib/hw/deps.conf" "$BH/lib/hw/"   # the claude floor is a row of this manifest
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
  env -i HW_CORE="$HW_CORE" PATH="$TMP/stub:/usr/bin:/bin:/usr/sbin:/sbin" HOME="$TMP/home" TMPDIR="$TMP" STUB_CALLS="$TMP/calls" \
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

# The floor is a row of lib/hw/deps.conf, not a number in bin/brain: move it and brain follows.
cp "$BH/lib/hw/deps.conf" "$TMP/deps.conf.keep"
sd '^claude\|[0-9.]+\|' 'claude|2.1.400|' "$BH/lib/hw/deps.conf"
run 2.1.300 demo; need_started "a raised floor"
[ -z "$(flag_dir)" ] && grep -q 'cockpit needs claude >= 2.1.400 (found 2.1.300)' "$TMP/out" || fail "803: brain did not follow the floor in lib/hw/deps.conf (2.1.400): $(cat "$TMP/out")"
run 2.1.400 demo; need_started "at the raised floor"
[ "$(flag_dir)" = "$BHP/cockpit" ] || fail "803: claude AT the manifest's floor was not given the cockpit"
rm "$BH/lib/hw/deps.conf"
run 2.1.400 demo; need_started "no manifest"
[ -z "$(flag_dir)" ] && grep -q 'no version floor for the cockpit' "$TMP/out" || fail "803: a tree with no manifest must open without the cockpit and say so: $(cat "$TMP/out")"
cp "$TMP/deps.conf.keep" "$BH/lib/hw/deps.conf"
pass "803: the cockpit's claude floor is read from lib/hw/deps.conf; with no manifest brain opens without the cockpit and says so"

# Mutants of bin/brain: the eight bash mutants of this subject edited bin/brain's bash, which is gone (go-060 S5: bin/brain is a shim over
# hw-core), and each assertion above is the survival control of one. Their Go twins are anchored on the Go source and killed in process by
# core/brain_mutants_test.go (rows of setup/tests/contract-hw/mutants-brain-go-launch, judged by TestBrain803*):
#   the flag is passed below the floor        go-803-M01  core/brain_launch.go
#   the opt-out is ignored                    go-803-M02  core/brain_launch.go
#   --no-cockpit is ignored                   go-803-M03  core/brain_launch.go
#   the floor message is dropped              go-803-M04  core/brain_launch.go
#   the flag goes to opencode too             go-803-M05  core/brain_launch.go
#   only the major version is compared        go-803-M06  core/brain_helpers.go
#   the flag points at a different folder     go-803-M07  core/brain_launch.go
#   the work root is not passed to the pane   go-803-M08  core/brain_launch.go
# (The old/new arm that ran the base's bash brain is gone with it: the base's brain is a shim now, and it passes the flag.)
printf 'coverage - 803: 0 mutants (the 8 bash mutants are retired; go-803-M01..M08 hold them in Go)\n'
