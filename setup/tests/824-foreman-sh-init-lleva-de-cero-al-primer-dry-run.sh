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
#   2. a missing prerequisite is NAMED with its install command; init asks
#      Homebrew for it in the same run when answered (--yes), never without an
#      answer, and a stop is ONE copyable line (--with-recommended && init);
#      one run takes a machine without herdr to the dry-run dispatch;
#   2d. git and python3 are not Homebrew's (Command Line Tools): a machine missing ONLY git is told
#      `xcode-select --install`, is not offered brew and is not sent to --with-recommended;
#   2e. without Go, init finishes and passes on install.sh's NOTE that hw runs in bash (4b below);
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
#   M03 "nothing changed" is never concluded (the measured fingerprint is ignored);
#   M04 the in-run Homebrew offer dropped — a missing herdr stops the run again;
#   M05 the Command Line Tools line dropped — a machine missing only git is not told xcode-select;
#   M06 the copyable brew line printed for what Homebrew cannot install (only git missing);
#   M07 the in-run Homebrew offer fired for what Homebrew cannot install (only git missing);
#   M08 install.sh's NOTE lines (hw runs in bash: no Go) are not passed on.
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
echo "brew $*" >> "$STUB_LOGS/brew-calls"
# `brew install herdr` puts a herdr on PATH only where the test says so (STUB_BREW_BIN)
[ "$1 $2" = "install herdr" ] && [ -n "${STUB_BREW_BIN:-}" ] && { mkdir -p "$STUB_BREW_BIN"; cp "$STUB_HERDR_SRC" "$STUB_BREW_BIN/herdr"; }
exit 0
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
export STUB_HERDR_SRC="$STUBS/herdr"
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

# ── 2. a missing prerequisite is named; with Homebrew init offers the install in the same run ──
# Answered yes (--yes) but the brew stub installs nothing: still missing, the run stops before writing,
# and the message is ONE line to copy (--with-recommended && init), not two instructions.
rc=0; out="$(PATH="$NOHERDR_PATH" init --yes)" || rc=$?
[ "$rc" = 1 ] || fail "init with no herdr must exit 1 (got $rc): $out"
case "$out" in *"MISSING herdr"*) ;; *) fail "the missing herdr is not named: $out" ;; esac
case "$out" in *"brew install herdr"*|*"https://herdr.dev"*) ;; *) fail "the missing herdr is named without its install command: $out" ;; esac
case "$out" in *"[2/7]"*) fail "the run went past the prerequisites with herdr missing: $out" ;; esac
case "$out" in *"$ROOT/install.sh --with-recommended && "*" init"*|*"--with-recommended && ./install.sh init"*) ;; *) fail "the stop does not name ONE copyable line (--with-recommended && init): $out" ;; esac
[ ! -e "$HOME/brain" ] && [ ! -e "$HOME/code/toy" ] || fail "init wrote a brain or a toy repository with a prerequisite missing"
rg -q 'brew install herdr' "$LOGS/brew-calls" 2>/dev/null || fail "init --yes did not run the Homebrew install in the same run: $(cat "$LOGS/brew-calls" 2>/dev/null)"
pass "a missing herdr is named, init --yes tries Homebrew in the same run, and when it is still missing the stop is one copyable line and writes nothing"

# 2b. no answer (no --yes, no terminal): Homebrew is NOT called, the one line is printed
rm -f "$LOGS/brew-calls"
rc=0; out="$(PATH="$NOHERDR_PATH" notty "$ROOT/install.sh" init | sed 's/\x1b\[[0-9;]*m//g')" || rc=$?
[ "$rc" = 1 ] || fail "init with no herdr and no answer must exit 1 (got $rc): $out"
[ ! -e "$LOGS/brew-calls" ] || fail "init called brew without being asked: $(cat "$LOGS/brew-calls")"
case "$out" in *"--with-recommended && "*) ;; *) fail "no copyable line without an answer: $out" ;; esac
pass "without an answer init installs nothing and prints one copyable line"

# 2c. Homebrew does install herdr: ONE init run goes from missing herdr to the dry-run dispatch
OLD_HOME="$HOME"; export HOME="$TMP/home-brew"; mkdir -p "$HOME"
git config --global user.name t; git config --global user.email t@t
rm -f "$LOGS/brew-calls" "$LOGS/herdr-install"
rc=0; out="$(STUB_BREW_BIN="$TMP/brew-bin" PATH="$TMP/brew-bin:$NOHERDR_PATH" init --yes)" || rc=$?
[ "$rc" = 0 ] || fail "one init --yes run must install herdr through Homebrew and finish (got $rc): $out"
rg -q 'brew install herdr' "$LOGS/brew-calls" || fail "brew was not asked for herdr"
case "$out" in *"ok    dry run: hw demo hello"*) ;; *) fail "the same run did not reach the dry-run dispatch: $out" ;; esac
case "$out" in *"MISSING herdr"*"ok    herdr"*) ;; *) fail "herdr was not reported missing and then ok in the same run: $out" ;; esac
export HOME="$OLD_HOME"; rm -f "$LOGS/herdr-install" "$LOGS/engram-setup" "$LOGS/brew-calls"
pass "one init --yes run takes a machine without herdr through Homebrew to the dry-run dispatch"

# 2d. only git is missing: Homebrew cannot install it (the Command Line Tools do). Measured on a clean
# macOS VM 2026-10-11: init offered brew for it, which installs nothing, and printed a
# `--with-recommended && init` line that cannot succeed. Now it names xcode-select and nothing else.
NOGIT="$TMP/farm-nogit"; mkdir -p "$NOGIT"
for _l in "$FARM"/*; do case "${_l##*/}" in git) ;; *) ln -s "$(readlink "$_l")" "$NOGIT/${_l##*/}" 2>/dev/null || true ;; esac; done
# The Command Line Tools are a Mac's: the machine of this scenario says Darwin, as the one it was measured on did,
# also where the suite runs on Linux (whose own hint, apt, init names there and 2d does not hold).
_real_uname="$(type -P uname)"; rm -f "$NOGIT/uname"
printf '#!/bin/sh\n[ "$1" = -s ] && [ "$#" = 1 ] && { echo Darwin; exit 0; }\nexec "%s" "$@"\n' "$_real_uname" > "$NOGIT/uname"; chmod +x "$NOGIT/uname"
rm -f "$LOGS/brew-calls"
rc=0; out="$(PATH="$STUBS:$NOGIT" init --yes)" || rc=$?
[ "$rc" = 1 ] || fail "2d: init with only git missing must exit 1 (got $rc): $out"
case "$out" in *"MISSING git"*"Install: xcode-select --install"*) ;; *) fail "2d: the missing git is not named with xcode-select --install: $out" ;; esac
case "$out" in *"Command Line Tools"*"xcode-select --install"*) ;; *) fail "2d: the stop does not say git comes from the Command Line Tools: $out" ;; esac
[ ! -e "$LOGS/brew-calls" ] || fail "2d: init called brew for a prerequisite Homebrew cannot install: $(cat "$LOGS/brew-calls")"
case "$out" in *"--with-recommended"*|*"Homebrew can install"*|*"prerequisites, again:"*) fail "2d: init sent a machine missing only git to Homebrew: $out" ;; esac
pass "2d: a machine missing only git is told xcode-select --install, is not offered brew and gets no --with-recommended line"

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

# 4b. no Go toolchain: install.sh says hw runs in bash (one NOTE line) and init, which keeps install.sh's output
# to itself, must pass it on — measured 2026-10-13 on a clean macOS VM running rc.1: init finished, said nothing
# about Go, and the person could not tell which half of hw they had.
NOGO="$TMP/farm-nogo"; mkdir -p "$NOGO"
for _l in "$FARM"/*; do case "${_l##*/}" in go) ;; *) ln -s "$(readlink "$_l")" "$NOGO/${_l##*/}" 2>/dev/null || true ;; esac; done
OLD_HOME="$HOME"; export HOME="$TMP/home-nogo"; mkdir -p "$HOME"
cp -p "$LOGS/herdr-install" "$TMP/herdr-install.4"; cp -p "$LOGS/engram-setup" "$TMP/engram-setup.4"   # section 5 counts the first run's calls
git config --global user.name t; git config --global user.email t@t
rc=0; out="$(unset FOREMAN_SH_NO_CORE; PATH="$STUBS:$NOGO" init --yes)" || rc=$?   # _common.sh sets it: the suite never builds Go
[ "$rc" = 0 ] || fail "4b: init --yes without Go must still finish (got $rc): $out"
if [ -d "$ROOT/core" ] && [ -f "$ROOT/setup/build-core" ]; then
  case "$out" in *"NOTE  no Go toolchain on PATH: hw-core is not built and hw runs its bash implementation"*) ;; *) fail "4b: init does not say hw runs in bash without Go: $out" ;; esac
  [ ! -e "$HOME/.config/hw/impl" ] || fail "4b: a switch file was written with no hw-core built"
  pass "4b: without Go init finishes and passes on install.sh's one-line NOTE that hw runs in bash"
else
  pass "4b: skipped — this tree carries no Go core, so install.sh has no NOTE to pass on"
fi
export HOME="$OLD_HOME"; cp -p "$TMP/herdr-install.4" "$LOGS/herdr-install"; cp -p "$TMP/engram-setup.4" "$LOGS/engram-setup"

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

# M04 — the in-run Homebrew offer dropped: the unmutated 2c run reached the dry-run dispatch
cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M04 "$MUT/init.sh" 'if false; then'   # was: if [ "$MISSING" -gt 0 ] && type -P brew ... && [ "$DRY" = 0 ]; then
saw_mutant "M04 the Homebrew offer dropped" "$(HOME="$TMP/home-brew-m" STUB_BREW_BIN="$TMP/brew-bin-m" PATH="$TMP/brew-bin-m:$NOHERDR_PATH" minit --yes)" "prerequisite(s) missing"

# M05 — the Command Line Tools line dropped: the unmutated 2d run said "Command Line Tools"
cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M05 "$MUT/init.sh" 'if false; then'   # was: if [ "$CLT_MISSING" -gt 0 ]; then
# positive evidence, not absence: the run REACHED the stop (so the mutated line ran), and the hint is gone
m5="$(PATH="$STUBS:$NOGIT" minit --yes || true)"
case "$m5" in *"prerequisite(s) missing"*) case "$m5" in *"Command Line Tools"*) m5=CLT-LINE-STILL-THERE ;; *) m5=CLT-LINE-GONE ;; esac ;; *) m5=RUN-NEVER-REACHED-THE-STOP ;; esac
saw_mutant "M05 the Command Line Tools line dropped" "$m5" "CLT-LINE-GONE"

# M06 — the copyable brew line printed even when only git is missing: the unmutated 2d run had none
cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M06 "$MUT/init.sh" '  if type -P brew >/dev/null 2>&1; then'   # was: if [ "$((MISSING - CLT_MISSING))" -gt 0 ] && type -P brew ...
saw_mutant "M06 the brew line for what brew cannot install" "$(PATH="$STUBS:$NOGIT" minit --yes)" "--with-recommended &&"

# M07 — the in-run offer fired by MISSING alone: the unmutated 2d run never asked
cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M04 "$MUT/init.sh" 'if [ "$MISSING" -gt 0 ] && type -P brew >/dev/null 2>&1 && [ "$DRY" = 0 ]; then'   # was: [ "$((MISSING - CLT_MISSING))" -gt 0 ]
saw_mutant "M07 the offer fired for what brew cannot install" "$(PATH="$STUBS:$NOGIT" minit --yes)" "prerequisites, again:"

# M08 — install.sh's NOTE lines dropped: the unmutated 4b run carried the no-Go NOTE
if [ -d "$ROOT/core" ] && [ -f "$ROOT/setup/build-core" ]; then
  cp "$ROOT/init.sh" "$MUT/init.sh"; mutate_anchor 824-M08 "$MUT/init.sh" '  :'   # was: ... sed -n 's/^  NOTE  //p' | while ... say "NOTE  $_note"
  m8="$(unset FOREMAN_SH_NO_CORE; HOME="$TMP/home-nogo-m" PATH="$STUBS:$NOGO" minit --yes || true)"
  # positive evidence: the run REACHED its end (so the mutated line ran) and the NOTE is gone
  case "$m8" in *"init: done"*) case "$m8" in *"NOTE  no Go toolchain"*) m8=NOTE-STILL-THERE ;; *) m8=NOTE-GONE ;; esac ;; *) m8=RUN-NEVER-FINISHED ;; esac
  saw_mutant "M08 install.sh's NOTE not passed on" "$m8" "NOTE-GONE"
fi

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

printf 'coverage - 824: 11 live claims, 8 mutants (8 must be named)\n'
