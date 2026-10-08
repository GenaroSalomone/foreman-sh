#!/usr/bin/env bash
# `foreman-sh init` takes an empty HOME to a first dry-run dispatch in one command
#
# WHAT THIS HOLDS. Measured 2026-10-07 (scratch HOME, private engram port): the
# manual path from INSTALL.md to a first `hw demo hello --dry-run` is nine
# commands over three documents, one of them (`--no-report`) learned from a
# failure, and the toy repository, the sample brief's location and the PATH are
# the person's to know. `install.sh init` is that path in one command. Asked
# here, over stubs of herdr, claude, engram and brew in a HOME this run owns:
#
#   1. `--dry-run` prints the seven stages as a plan and changes nothing;
#   2. a missing prerequisite is NAMED with its install command and the run
#      stops before it writes anything — and nothing is installed (the brew stub
#      is never called);
#   3. without --yes and without a terminal, herdr's integration is asked for,
#      not done: the run stops, names the command and writes no brain;
#   4. `init --yes` builds the brain, the demo lane on a toy repository, the
#      sample brief, and ends with a dry-run dispatch of it that succeeds;
#      Claude's first run, the PATH and the git identity are named as the
#      person's, never answered;
#   5. a second run changes no file and SAYS so; a brief the person edited is
#      left as it is;
#   6. a lane other than demo needs --repo, and with one it is built.
#
# THE MUTANTS run a copy of init.sh with one line changed, inside a symlink farm of
# this tree, and are killed by text only the mutated line produces:
#   M01 the prerequisite check dropped — a missing herdr no longer stops the run;
#   M02 the sample brief is written over what is there;
#   M03 "nothing changed" is never concluded (the measured fingerprint is ignored).
#
# Run alone:  bash setup/tests/824-foreman-sh-init-lleva-de-cero-al-primer-dry-run.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON CLAUDE_CONFIG_DIR OPENCODE_CONFIG_DIR OPENCODE_EXECUTOR_CONFIG
[ -f "$ROOT/init.sh" ] || fail "init.sh is not in the tree — there is no \`init\` to test"

# ── the world: stubs ahead of a copy of PATH that has no herdr, claude, engram or brew ──
LOGS="$TMP/stub-log"; mkdir -p "$LOGS"
STUBS="$TMP/stubs"; mkdir -p "$STUBS"
cat > "$STUBS/herdr" <<'STUB'
#!/usr/bin/env bash
case "$1 $2" in
  "integration status") [ -f "$HOME/.herdr-claude-integrated" ] && echo "claude: current (stub)"; exit 0 ;;
  "integration install")
    [ -d "$HOME/.claude" ] || { echo "no ~/.claude" >&2; exit 1; }
    : > "$HOME/.herdr-claude-integrated"; echo "$*" >> "$STUB_LOGS/herdr-install"; exit 0 ;;
esac
exit 0
STUB
cat > "$STUBS/claude" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
cat > "$STUBS/engram" <<'STUB'
#!/usr/bin/env bash
if [ "$1 $2" = "setup claude-code" ]; then
  read -r ans; echo "answer=$ans" >> "$STUB_LOGS/engram-setup"
  printf '{"hasCompletedOnboarding":false,"mcpServers":{"engram":{}}}' > "$HOME/.claude.json"
fi
exit 0
STUB
cat > "$STUBS/brew" <<'STUB'
#!/usr/bin/env bash
echo "brew $*" >> "$STUB_LOGS/brew-calls"; exit 0
STUB
cat > "$STUBS/curl" <<'STUB'
#!/usr/bin/env bash
case "$*" in *"/health"*) [ "${STUB_ENGRAM:-up}" = up ] && { echo '{"status":"ok","version":"stub"}'; exit 0; } ;; esac
exit 22
STUB
chmod +x "$STUBS"/*
FARM="$TMP/farm"; mkdir -p "$FARM"   # every executable on PATH except the ones this world stubs
IFS=: read -ra _pdirs <<<"$PATH"
for _d in "${_pdirs[@]}"; do
  for _f in "$_d"/*; do _n="${_f##*/}"
    case "$_n" in herdr|claude|engram|brew|curl) continue ;; esac
    [ -e "$FARM/$_n" ] || [ ! -x "$_f" ] || ln -s "$_f" "$FARM/$_n" 2>/dev/null || true
  done
done
WORLD_PATH="$STUBS:$FARM"
NOHERDR_PATH="$TMP/stubs-no-herdr:$FARM"
mkdir -p "$TMP/stubs-no-herdr"; for _s in claude engram brew curl; do ln -s "$STUBS/$_s" "$TMP/stubs-no-herdr/$_s"; done
export STUB_LOGS="$LOGS" SHELL=/bin/zsh HW_ENGRAM_URL="http://127.0.0.1:1"   # never the real engram
export PATH="$WORLD_PATH"
REAL_HOME="$(cd -P "$HOME" && pwd)"

init() { "$ROOT/install.sh" init "$@" 2>&1 </dev/null | sed 's/\x1b\[[0-9;]*m//g'; }   # under pipefail: install.sh's status
notty() {   # a new session: no controlling terminal for the /dev/tty probe to find
  python3 - "$@" <<'PY'
import subprocess, sys
r = subprocess.run(sys.argv[1:], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                   start_new_session=True, text=True, timeout=120)
sys.stdout.write(r.stdout); sys.exit(r.returncode)
PY
}
tree() { (cd "$HOME" && find . -path ./.cache -prune -o -type f -print0 | sort -z | xargs -0 shasum 2>/dev/null) | shasum; }

# ── 1. --dry-run: the plan, and nothing changes ─────────────────────────────
t0="$(tree)"
rc=0; out="$(init --dry-run)" || rc=$?
[ "$rc" = 0 ] || fail "init --dry-run must exit 0 with every prerequisite met (got $rc): $out"
for s in '[1/7]' '[2/7]' '[3/7]' '[4/7]' '[5/7]' '[6/7]' '[7/7]'; do
  case "$out" in *"$s"*) ;; *) fail "the plan has no stage $s: $out" ;; esac
done
case "$out" in *"would run: herdr integration install claude"*) ;; *) fail "the plan does not name herdr's integration: $out" ;; esac
case "$out" in *"would create the toy repository"*) ;; *) fail "the plan does not name the toy repository: $out" ;; esac
case "$out" in *"would run: hw demo hello --brief"*"--dry-run"*) ;; *) fail "the plan does not end with the dry-run dispatch: $out" ;; esac
case "$out" in *"git has no user.name"*) ;; *) fail "an empty HOME has no git identity and the plan does not say so: $out" ;; esac
[ "$(tree)" = "$t0" ] || fail "init --dry-run wrote files"
[ ! -e "$HOME/brain" ] && [ ! -e "$HOME/code/toy" ] || fail "init --dry-run created the brain or the toy repository"
pass "--dry-run prints the seven stages as a plan, names the git identity, and changes nothing"

# ── 2. a missing prerequisite is named, the run stops, nothing is installed ──
rc=0; out="$(PATH="$NOHERDR_PATH" init --yes)" || rc=$?
[ "$rc" = 1 ] || fail "init with no herdr must exit 1 (got $rc): $out"
case "$out" in *"MISSING herdr"*) ;; *) fail "the missing herdr is not named: $out" ;; esac
case "$out" in *"brew install herdr"*|*"https://herdr.dev"*) ;; *) fail "the missing herdr is named without its install command: $out" ;; esac
case "$out" in *"[2/7]"*) fail "the run went past the prerequisites with herdr missing: $out" ;; esac
[ ! -e "$HOME/brain" ] && [ ! -e "$HOME/code/toy" ] || fail "init wrote a brain or a toy repository with a prerequisite missing"
[ ! -e "$LOGS/brew-calls" ] || fail "init called brew: $(cat "$LOGS/brew-calls")"
pass "a missing herdr is named with its install command, the run stops before writing, and brew is never called"

# ── 3. no --yes and no terminal: the integration is asked for, not done ─────
rc=0; out="$(notty "$ROOT/install.sh" init | sed 's/\x1b\[[0-9;]*m//g')" || true
case "$out" in *"herdr integration install claude"*"init: stopped before writing the brain"*) ;; *) fail "without --yes or a terminal the run neither did nor named herdr's integration: $out" ;; esac
[ ! -e "$HOME/brain" ] && [ ! -e "$LOGS/herdr-install" ] || fail "init did the integration, or wrote the brain, without being asked"
pass "without --yes and without a terminal init stops at herdr's integration, names its command and writes nothing"

# ── 4. init --yes: brain, lane, toy repository, sample brief, dry-run dispatch ─
git config --global user.name t; git config --global user.email t@t
rc=0; out="$(init --yes)" || rc=$?
[ "$rc" = 0 ] || fail "init --yes must exit 0 (got $rc): $out"
BRIEF="$REAL_HOME/brain/demo/briefs/hello.md"
[ -s "$BRIEF" ] || fail "the sample brief was not written to $BRIEF"
[ "$(jq -r '.lanes.demo.checkout' "$HOME/brain/projects.json")" = "$REAL_HOME/code/toy" ] || fail "projects.json has no demo lane over the toy repository"
[ "$(git -C "$HOME/code/toy" rev-list --count HEAD)" = 1 ] && [ -s "$HOME/code/toy/README.md" ] || fail "the toy repository is not one commit with a README"
for l in hw brain done-invoker; do [ -L "$HOME/.local/bin/$l" ] || fail "$l is not linked in ~/.local/bin"; done
[ "$(wc -l < "$LOGS/herdr-install" | tr -d ' ')" = 1 ] || fail "herdr's integration was not installed exactly once"
[ "$(cat "$LOGS/engram-setup")" = "answer=y" ] || fail "engram setup was not run with its allowlist question answered y"
case "$out" in *"dispatch demo:hello"*) ;; *) fail "the output carries no dry-run dispatch of demo:hello: $out" ;; esac
case "$out" in *"ok    dry run: hw demo hello"*) ;; *) fail "the dry-run dispatch did not end ok: $out" ;; esac
case "$out" in *"yours: Claude Code's first run"*"claude --dangerously-skip-permissions"*) ;; *) fail "Claude Code's first run is not named as the person's: $out" ;; esac
case "$out" in *"yours: PATH:"*) ;; *) fail "the PATH is not named as the person's: $out" ;; esac
case "$out" in *"init: done"*) ;; *) fail "the first run does not say it changed things: $out" ;; esac
[ ! -e "$HOME/work/demo/hello" ] || fail "the dry run created a worktree"
pass "init --yes builds the brain, the demo lane on a toy repository and the sample brief, and ends with a dry-run dispatch that succeeds; first run and PATH are named as the person's"

# ── 5. a second run changes nothing and says so ─────────────────────────────
t1="$(tree)"
rc=0; out="$(init --yes)" || rc=$?
[ "$rc" = 0 ] || fail "the second init --yes must exit 0 (got $rc): $out"
[ "$(tree)" = "$t1" ] || fail "the second init changed files"
case "$out" in *"init: nothing to change"*) ;; *) fail "the second run does not say it changed nothing: $out" ;; esac
case "$out" in *"were already in place: nothing changed"*) ;; *) fail "the second run does not say the brain was already in place: $out" ;; esac
case "$out" in *"(left as it is)"*) ;; *) fail "the second run does not say the sample brief was left alone: $out" ;; esac
[ "$(wc -l < "$LOGS/herdr-install" | tr -d ' ')" = 1 ] && [ "$(wc -l < "$LOGS/engram-setup" | tr -d ' ')" = 1 ] || fail "the second run redid herdr's integration or engram's setup"
pass "a second init --yes changes no file, redoes no setup, and says there was nothing to change"

# ── the mutants: one changed line each, in a copy of init.sh inside a symlink farm ──
MUT="$TMP/mut"; mkdir -p "$MUT"
for e in "$ROOT"/* "$ROOT"/.[!.]*; do n="${e##*/}"; case "$n" in init.sh|.git) continue ;; esac; [ -e "$e" ] && ln -s "$e" "$MUT/$n"; done
minit() { "$MUT/install.sh" init "$@" 2>&1 </dev/null | sed 's/\x1b\[[0-9;]*m//g'; }

# M01 — the prerequisite check dropped: the unmutated run above stopped before [2/7]
cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M01 "$MUT/init.sh" 'if false; then'   # was: if [ "$MISSING" -gt 0 ]; then
saw_mutant "M01 the prerequisite check dropped" "$(PATH="$NOHERDR_PATH" minit --yes)" "[2/7] the Claude account"

# M02 — the sample brief is written over what is there: the unmutated second run said "(left as it is)"
cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M02 "$MUT/init.sh" 'if false; then ok "x"'   # was: if [ -f "$BRIEF" ]; then ok "… (left as it is)"
saw_mutant "M02 the sample brief overwritten on a re-run" "$(minit --yes)" "+     $BRIEF" "init: done — 1 change(s)"

# M03 — the measured fingerprint ignored: the unmutated second run said "were already in place"
cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M03 "$MUT/init.sh" 'if false; then ok "x"'   # was: if [ "$before" = "$after" ]; then ok "… nothing changed"
saw_mutant "M03 nothing-changed never concluded" "$(minit --yes)" "(links in " "init: done — 1 change(s)"

# ── 6. what the person edited is left alone; another lane needs its repository ─
printf 'my own brief\n' > "$BRIEF"
out="$(init --yes)" || true
[ "$(cat "$BRIEF")" = "my own brief" ] || fail "init wrote over a brief the person edited"
pass "a brief the person edited is left as it is"
rc=0; out="$(init --yes --lane myapp)" || rc=$?
[ "$rc" = 1 ] || fail "a lane other than demo with no --repo must exit 1 (got $rc): $out"
case "$out" in *"needs --repo"*) ;; *) fail "the missing --repo is not named: $out" ;; esac
home_repo myapp-repo main
rc=0; out="$(init --yes --lane myapp --repo "$HOME/myapp-repo")" || rc=$?
[ "$rc" = 0 ] || fail "init --lane myapp --repo must exit 0 (got $rc): $out"
[ "$(jq -r '.lanes.myapp.checkout' "$HOME/brain/projects.json")" = "$REAL_HOME/myapp-repo" ] || fail "projects.json has no myapp lane"
case "$out" in *"dispatch myapp:hello"*) ;; *) fail "the sample brief does not dispatch on lane myapp: $out" ;; esac
pass "a lane other than demo needs --repo, and with one it is built and its sample brief dispatches"

printf 'coverage - 824: 6 live claims, 3 mutants (3 must be named)\n'
