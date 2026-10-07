#!/usr/bin/env bash
# One command installs a brain of one's own, twice without harm, and refuses before it overwrites
#
# WHAT THIS HOLDS. Stage 4 of the harness opening (setup/decisions.md, «Etapa
# 4: el instalador») is `install.sh`: from this checkout, one command builds a
# new brain with a lane over a product repo, its guards, `hw` on PATH and the
# Stop hook merged into the user's Claude Code settings. This file asks it, in
# the suite's own empty HOME, over a repo whose path has spaces:
#
#   1. it names a missing prerequisite and writes nothing;
#   2. it installs, and the new brain's `hw` dispatches that lane (dry run)
#      from a bin/ that is byte-identical to this tree's;
#   3. a second identical run changes no file;
#   4. the new brainer's guard refuses a write into the repo and allows a read;
#   5. it merges into an existing settings.json, keeping what was there;
#   6. every collision — another brain's Stop hook, a lane name over another
#      repo, a foreign link on PATH, a non-empty directory — is refused with
#      nothing written;
#   7. and nothing is ever written inside the lane's repository.
#
# The live proof — a real brainer and a haiku executor reporting through
# done-invoker from a borrowed HOME — is not reproducible here and lives in the
# task's transcript; this file is the regression net for the installer itself.
#
# Run alone:  bash setup/tests/187-un-instalador-de-un-comando.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON CLAUDE_CONFIG_DIR

INSTALL="$ROOT/install.sh"
REPO="$HOME/code/My App"
mkdir -p "$(dirname "$REPO")"
( mkdir -p "$REPO" && cd "$REPO" && git init -q -b trunk && git config user.email t@t \
  && git config user.name t && echo hi > README.md && git add . && git commit -q -m base )
repo_state() { ( cd "$REPO" && git status --porcelain --untracked-files=all; git for-each-ref; git worktree list; find . -path ./.git -prune -o -type f -print | sort ) | shasum; }
REPO0="$(repo_state)"
RREPO="$(cd -P "$REPO" && pwd)"

# The prerequisites the installer looks for, as the smallest honest stand-ins:
# herdr answers only `integration status`, claude only exists.
STUBS="$TMP/prereq-bin"; mkdir -p "$STUBS"
cat > "$STUBS/herdr" <<'EOF'
#!/usr/bin/env bash
[ "$1 $2" = "integration status" ] && { echo "claude: ${STUB_INTEGRATION:-current} (x)"; exit 0; }
exec "$HW_TEST_HERDR_STUB" "$@" 2>/dev/null || exit 0
EOF
printf '#!/bin/sh\nexit 0\n' > "$STUBS/claude"
chmod +x "$STUBS/herdr" "$STUBS/claude"
HW_TEST_HERDR_STUB="$(command -v herdr || echo /usr/bin/true)"; export HW_TEST_HERDR_STUB
export PATH="$HOME/.local/bin:$STUBS:$PATH"

inst() { "$INSTALL" "$@" 2>&1; }
tree() { (cd "$HOME" && find . -path ./code -prune -o -type f -print0 | sort -z | xargs -0 shasum) | shasum; }

# ── 1. a missing prerequisite is named, and nothing is written ──────────────
out="$(STUB_INTEGRATION=missing inst --brain ~/brain --lane app --repo "$REPO" || true)"
case "$out" in *"MISSING herdr's claude integration"*"nothing was written"*) ;; *) fail "a missing prerequisite was not named: $out" ;; esac
[ ! -e "$HOME/brain" ] || fail "a run that stopped on a prerequisite created the brain"
pass "a missing prerequisite is named with its install command, and no brain is created"

# ── 2. one command installs, and the new brain's hw dispatches the lane ─────
out="$(inst --brain ~/brain --lane app --repo "$REPO")" || fail "install failed: $(printf '%s' "$out" | tail -5)"
B="$(cd -P "$HOME/brain" && pwd)"   # the installer records resolved paths (/var → /private/var)
LB="$(cd -P "$HOME/.local/bin" && pwd)"
diff -r "$ROOT/bin" "$B/bin" >/dev/null || fail "the new brain's bin/ is not the tree's"
for f in hw brain done-invoker ask-invoker channel-send decisions; do
  # -ef, not the link text: under Git Bash the installer and `cd -P` spell one
  # directory two ways (/tmp/x and /c/Users/<u>/AppData/Local/Temp/x).
  [ -L "$LB/$f" ] && [ "$LB/$f" -ef "$B/bin/$f" ] || fail "$f is not linked into ~/.local/bin"
done
# The same file, however spelled (the installer writes /c/... where `cd -P`
# says /tmp/... under Git Bash, as for the links above).
stop_dir="$(rg -o "bash '([^']+)/bin/hw-stop-hook\.sh' stop" -r '$1' "$HOME/.claude/settings.json" 2>/dev/null | head -1)"
[ -n "$stop_dir" ] && [ "$stop_dir/bin/hw-stop-hook.sh" -ef "$B/bin/hw-stop-hook.sh" ] || fail "the Stop hook was not merged into ~/.claude/settings.json"
export TMPDIR="$TMP"
dry="$(cd "$B" && hw app probe --sdd none --no-report --no-brief --dry-run < /dev/null 2>&1 | sed 's/\x1b\[[0-9;]*m//g' || true)"
for want in "dispatch app:probe" "worktree    $(dirname "$B")/work/app/probe  (new, own branch off trunk)" "engram      app" "agent       claude  (lane default for app)"; do
  printf '%s\n' "$dry" | rg -qF -- "$want" || fail "the new brain's dry run has no '$want': $(printf '%s' "$dry" | tail -3 | tr '\n' ' ')"
done
pass "one command installs a brain whose bin/ is this tree's, links hw and the invokers, merges the Stop hook, and \`hw app probe --dry-run\` dispatches from the table"

# ── 3. idempotent ───────────────────────────────────────────────────────────
t0="$(tree)"
out="$(inst --brain ~/brain --lane app --repo "$REPO")" || fail "the second run failed: $(printf '%s' "$out" | tail -3)"
[ "$(tree)" = "$t0" ] || fail "a second identical run changed files"
printf '%s\n' "$out" | rg -q '^  \+ ' && fail "a second identical run reported writes: $(printf '%s\n' "$out" | rg '^  \+ ')"
pass "a second identical run changes no file and reports no write"

# ── 3b. no rsync (Git for Windows ships none), and an rsync that fails ─────
# A PATH that is this one minus rsync, in a HOME of its own (a second brain in
# this HOME is a Stop-hook collision, section 6).
NORS="$TMP/no-rsync-bin"; mkdir -p "$NORS"
IFS=: read -ra _pdirs <<<"$PATH"
for _d in "${_pdirs[@]}"; do
  for _f in "$_d"/*; do _n="${_f##*/}"
    [ "$_n" = rsync ] || [ -e "$NORS/$_n" ] || [ ! -x "$_f" ] || ln -s "$_f" "$NORS/$_n" 2>/dev/null || true
  done
done
H2="$TMP/no-rsync-home"; mkdir -p "$H2"
out="$(HOME="$H2" PATH="$NORS" "$INSTALL" --brain "$H2/brain" --lane app --repo "$REPO" 2>&1)" || fail "with no rsync on PATH the install failed: $(printf '%s' "$out" | tail -3)"
diff -r "$ROOT/bin" "$H2/brain/bin" >/dev/null || fail "with no rsync on PATH the new brain's bin/ is not the tree's: $(printf '%s' "$out" | rg 'bin/')"
out="$(HOME="$H2" PATH="$NORS" "$INSTALL" --brain "$H2/brain" --lane app --repo "$REPO" 2>&1)" || fail "with no rsync the second run failed"
printf '%s\n' "$out" | rg -q '^  \+ ' && fail "with no rsync a second identical run reported writes: $(printf '%s\n' "$out" | rg '^  \+ ')"
FRS="$TMP/failing-rsync-bin"; mkdir -p "$FRS"; printf '#!/bin/sh\nexit 23\n' > "$FRS/rsync"; chmod +x "$FRS/rsync"
H3="$TMP/failing-rsync-home"; mkdir -p "$H3"
HOME="$H3" PATH="$FRS:$NORS" "$INSTALL" --brain "$H3/brain" --lane app --repo "$REPO" > "$TMP/frs.out" 2>&1 \
  && fail "an rsync that failed was reported as a successful install: $(rg 'bin/' "$TMP/frs.out")"
rg -q 'rsync could not mirror' "$TMP/frs.out" || fail "a failed rsync was not named: $(tail -3 "$TMP/frs.out")"
pass "with no rsync the mirror is made anyway, byte-identical and idempotent, and a failed rsync stops the install instead of reading as up to date"

# ── 4. the new brainer's guard ──────────────────────────────────────────────
verdict() {  # <hook> <command>
  python3 - "$1" "$2" "$B/app" <<'PY'
import json, subprocess, sys
p = subprocess.run([sys.argv[1]], input=json.dumps({"tool_name": "Bash", "tool_input": {"command": sys.argv[2]}, "cwd": sys.argv[3]}), capture_output=True, text=True)
print("deny" if '"deny"' in p.stdout else "allow")
PY
}
for hook in "$B/app/.claude/hooks/deny-repo-writes.py" "$B/.claude/hooks/deny-repo-writes.py"; do
  [ "$(verdict "$hook" "echo x > '$REPO/README.md'")" = deny ] || fail "$hook allowed a redirect into the repo"
  [ "$(verdict "$hook" "rm -rf '$REPO'")" = deny ] || fail "$hook allowed rm -rf of the repo"
  [ "$(verdict "$hook" "git -C '$REPO' log -1")" = allow ] || fail "$hook refused a read-only git log"
  [ "$(verdict "$hook" "echo idea >> $B/app/decisions.md")" = allow ] || fail "$hook refused a write into the brain"
done
jq -e --arg r "Edit(/$RREPO/**)" '.permissions.deny | index($r)' "$B/app/.claude/settings.json" >/dev/null \
  || fail "the lane's settings.json has no Edit deny over the repo"
pass "the lane's and the root's guards refuse writes into the repo, allow reads and brain writes, and the Edit deny half is there"

# ── 5. an existing settings.json is merged, not replaced ────────────────────
C3="$TMP/cfg3"; mkdir -p "$C3"
printf '{"model":"sonnet","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo mine"}]}]}}\n' > "$C3/settings.json"
CLAUDE_CONFIG_DIR="$C3" inst --brain ~/brain >/dev/null || fail "installing into a CLAUDE_CONFIG_DIR with a settings.json failed"
jq -e '.model == "sonnet" and (.hooks.PreToolUse[0].hooks[0].command == "echo mine") and (.hooks.Stop | length == 1)' "$C3/settings.json" >/dev/null \
  || fail "the merge lost what was there: $(cat "$C3/settings.json")"
pass "a user's settings.json keeps its keys and hooks and gains exactly one Stop hook"

# ── 6. collisions are refused before any write ──────────────────────────────
t0="$(tree)"
refused() {  # <label> <needle> <args...>
  local label="$1" needle="$2" o; shift 2
  o="$("$@" 2>&1 || true)"
  case "$o" in *"$needle"*) ;; *) fail "$label was not refused with '$needle': $(printf '%s' "$o" | tail -3)" ;; esac
  [ "$(tree)" = "$t0" ] || fail "$label wrote something before refusing"
}
mkdir -p "$HOME/code/other" && ( cd "$HOME/code/other" && git init -q -b main && git -c user.email=t@t -c user.name=t commit -q --allow-empty -m b )
refused "a lane name over another repo" "already exists" "$INSTALL" --brain ~/brain --lane app --repo "$HOME/code/other"
C2="$TMP/cfg2"; mkdir -p "$C2"
printf '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"bash /x/bin/hw-stop-hook.sh stop"}]}]}}\n' > "$C2/settings.json"
refused "another brain's Stop hook" "another brain's Stop hook" env CLAUDE_CONFIG_DIR="$C2" "$INSTALL" --brain ~/brain --lane other --repo "$HOME/code/other"
printf '{ nope' > "$C2/settings.json"
refused "an unparseable settings.json" "is not valid JSON" env CLAUDE_CONFIG_DIR="$C2" "$INSTALL" --brain ~/brain --lane other --repo "$HOME/code/other"
# Valid JSON that is not an object, and a brainer's own settings in a shape the
# merge cannot handle: both used to crash AFTER the mechanism was written
# (Judgment Day warnings, 2026-09-24). Now refused before the first write.
printf '[]\n' > "$C2/settings.json"
refused "a settings.json that is not an object" "not an object" env CLAUDE_CONFIG_DIR="$C2" "$INSTALL" --brain ~/brain --lane other --repo "$HOME/code/other"
cp "$B/app/.claude/settings.json" "$TMP/app-settings.saved"
printf '{"hooks":{"PreToolUse":"nope"}}\n' > "$B/app/.claude/settings.json"
t0="$(tree)"
refused "a brainer settings.json in a shape the merge cannot handle" "does not have the shape" "$INSTALL" --brain ~/brain --lane other --repo "$HOME/code/other"
cp "$TMP/app-settings.saved" "$B/app/.claude/settings.json"
t0="$(tree)"
L="$TMP/links"; mkdir -p "$L"; ln -s /elsewhere/hw "$L/hw"
refused "a foreign link on PATH" "another brain owns that name" "$INSTALL" --brain ~/brain --lane other --repo "$HOME/code/other" --bin-dir "$L"
mkdir -p "$TMP/notmine" && echo keep > "$TMP/notmine/f"
refused "a non-empty directory" "was not made by this installer" "$INSTALL" --brain "$TMP/notmine" --lane other --repo "$HOME/code/other"
refused "the source checkout itself" "is (or contains) the checkout" "$INSTALL" --brain "$ROOT" --lane other --repo "$HOME/code/other"
pass "a lane over another repo, another brain's Stop hook, bad or non-object JSON, a brainer settings file of the wrong shape, a foreign link and a foreign directory are each refused with nothing written"

# ── 7. the repo was never written ───────────────────────────────────────────
[ "$(repo_state)" = "$REPO0" ] || fail "the lane's repository changed during installation"
pass "the lane's repository — files, status, refs and worktrees — is byte-identical after every run"

# ── mutants ─────────────────────────────────────────────────────────────────
# M01 — an installer that ignores the Stop-hook clash overwrites the user's
# settings: named by the refusal it would have printed.
# The copy is a source tree of its own, because install.sh finds what it
# installs beside itself; nothing is written into $ROOT.
MS="$TMP/mutant-src"; mkdir -p "$MS/lanes" "$MS/setup"
cp -R "$ROOT/bin" "$ROOT/layouts" "$ROOT/lib" "$MS/"; cp "$ROOT/lanes/git-worktree.sh" "$MS/lanes/"; cp -R "$ROOT/setup/guards" "$MS/setup/"; cp "$ROOT/setup/brain-guard-programs.txt" "$MS/setup/"
cp "$INSTALL" "$MS/install.sh"; chmod +x "$MS/install.sh"
mutate_anchor 187-M01 "$MS/install.sh" 'theirs = []'
printf '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"bash /x/bin/hw-stop-hook.sh stop"}]}]}}\n' > "$C2/settings.json"
out="$(env CLAUDE_CONFIG_DIR="$C2" "$MS/install.sh" --brain "$TMP/m01-brain" --lane other --repo "$HOME/code/other" --bin-dir "$TMP/m01-bin" 2>&1 || true)"
saw_mutant "M01 the Stop-hook clash check removed" "$(jq -r '[.hooks.Stop[].hooks[].command] | length' "$C2/settings.json" 2>&1) stop hooks after: $out" "2 stop hooks after"

printf 'coverage - 186: 7 live claims, 1 mutant (1 must be named)\n'
