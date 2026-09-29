#!/usr/bin/env bash
#
# `git -c` IS NOT SCOPED TO THE COMMAND YOU TYPED IT ON, and this suite believed
# it was until 2026-09-09.
#
# WHAT HAPPENED, measured. This repo's pre-commit is reached as
# `git -c core.hooksPath=setup/hooks commit`. git serialises every `-c` into
# GIT_CONFIG_PARAMETERS, exports it, and every descendant git applies it at
# command-line precedence — which outranks a repository's own config, so a
# fixture cannot defend itself with `git config`. The hook stripped GIT_DIR and
# GIT_INDEX_FILE from the suite (a 2026-08-24 incident) but not this family, so
# `core.hooksPath=setup/hooks` travelled into
# tests/105-decisions-staged-budget.sh, whose fixtures install a pre-commit hook
# at <fixture>/.git/hooks and then commit deliberately corrupt decisions
# rotations to prove the guard refuses them. Git looked in
# <fixture>/setup/hooks/ instead, found no hook, ran no guard, and accepted all
# eight corrupt commits. Two POSITIVE cases went green in the same run, certifying
# a guard that had not executed.
#
# THE GUARD WAS NEVER WRONG — IT WAS NEVER CALLED. That distinction is the whole
# finding, and this file pins it from the outside: a leaked core.hooksPath must
# not be able to reach a fixture's hook.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# The leak exactly as git writes it. Both spellings are accepted by git, and a
# fixed `unset` list can name the first but not the numbered second, which is
# why the real fix sweeps the GIT_CONFIG prefix.
LEAK_PARAMETERS="'core.hooksPath'='setup/hooks'"

# ── 1. the environment is actually clean after _common.sh ───────────────────
# _common.sh is what every subject sources, so it is where the strip bites.
leaked="$(GIT_CONFIG_PARAMETERS="$LEAK_PARAMETERS" \
          GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=setup/hooks \
          GIT_CONFIG_GLOBAL=/dev/null \
          bash -c '. "$1"/tests/_common.sh; env | grep "^GIT_CONFIG" || true' _ "$ROOT/setup")"
if [ -n "$leaked" ]; then
  fail "_common.sh left git config injection in the environment, so a fixture's hooks can still be redirected: $(printf '%s' "$leaked" | tr '\n' ' ')"
fi
pass "_common.sh strips the whole GIT_CONFIG family, numbered KEY/VALUE pairs included"

# ── 2. and it bites where it matters: a fixture's own hook still runs ───────
# A behavioural probe, not a source read. Build a throwaway repo whose
# .git/hooks/pre-commit refuses every commit, then commit under the leak.
fixture() { # <dir>
  local d="$1"
  mkdir -p "$d/setup/hooks"
  ( cd "$d" && git init -q . )
  printf '#!/bin/sh\necho FIXTURE-HOOK-REFUSED >&2\nexit 1\n' > "$d/.git/hooks/pre-commit"
  chmod 755 "$d/.git/hooks/pre-commit"
  printf 'x\n' > "$d/file"
  ( cd "$d" && git add file )
}

commit_under_leak() { # <dir> <strip|nostrip> → "exit=<n> <stderr>"
  local d="$1" mode="$2" out rc=0 pre=''
  [ "$mode" = strip ] && pre='. "'"$ROOT"'/setup/tests/_common.sh";'
  out="$(cd "$d" && GIT_CONFIG_PARAMETERS="$LEAK_PARAMETERS" bash -c \
    "$pre"' git -c user.name=Probe -c user.email=probe@example.invalid commit -qm probe 2>&1 1>/dev/null')" || rc=$?
  printf 'exit=%s %s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}

fixture "$TMP/guarded"
guarded="$(commit_under_leak "$TMP/guarded" strip)"
case "$guarded" in
  exit=1*FIXTURE-HOOK-REFUSED*)
    pass "under a leaked core.hooksPath, a stripped environment still reaches the fixture's own pre-commit" ;;
  exit=0*)
    fail "the fixture's pre-commit was SKIPPED even after the strip — the corrupt-commit assertions in tests/105 are vacuous again: $guarded" ;;
  *)
    fail "the guarded fixture commit did something unexpected: $guarded" ;;
esac

# ── MUTATION ARM ────────────────────────────────────────────────────────────
# Remove the strip — the exact state this repo was in before 2026-09-09 — and
# the same fixture's hook must vanish. The mutant is killed by what it printed:
# a commit that SUCCEEDED where the guarded one was refused.
fixture "$TMP/unguarded"
unguarded="$(commit_under_leak "$TMP/unguarded" nostrip)"
saw_mutant "M01 unstripped GIT_CONFIG_PARAMETERS" "$unguarded" "exit=0"
case "$unguarded" in
  *FIXTURE-HOOK-REFUSED*)
    fail "M01 did not reproduce the leak: the fixture's hook ran even without the strip, so this file proves nothing about the strip — $unguarded" ;;
esac
pass "mutant killed: without the strip the leaked core.hooksPath silently skips the fixture's hook and the commit is accepted ($unguarded)"

# ── 3. the hook's own second lock, driven rather than read ──────────────────
# setup/hooks/pre-commit already stripped GIT_DIR and friends from the suite it
# launches; it now sweeps the GIT_CONFIG prefix too. That duplicate matters
# because it covers anything the suite runs before _common.sh is sourced, and an
# `env -u` list cannot name GIT_CONFIG_KEY_<n>. Prove it by giving the hook a
# stub suite that reports what it actually received.
HOOKDIR="$TMP/hooklock"
mkdir -p "$HOOKDIR/setup/hooks" "$HOOKDIR/bin"
( cd "$HOOKDIR" && git init -q . )
# pre-commit sources its sibling setup/hooks/suite-trigger-pattern.sh via
# $(git rev-parse --show-toplevel) — unconditionally, before it can even
# decide whether staged paths trigger the suite (see setup/tests/120) — so a
# fixture copying pre-commit standalone needs that sibling present too.
cp "$ROOT/setup/hooks/suite-trigger-pattern.sh" "$HOOKDIR/setup/hooks/suite-trigger-pattern.sh"
cp "$ROOT/setup/hooks/pre-commit" "$HOOKDIR/hook"
cat > "$HOOKDIR/setup/test-hw" <<'STUB'
#!/usr/bin/env bash
env | grep '^GIT_CONFIG' | sed 's/^/SUITE-SAW /' || true
printf 'not ok - deliberate stub failure so the hook prints what the suite saw\n'
exit 1
STUB
chmod +x "$HOOKDIR/hook" "$HOOKDIR/setup/test-hw"
# TWO trigger paths, not one. A single-path fixture cannot see how the list is
# JOINED, and the first version of that join was `paste -sd', '` — which reads
# its argument as a cyclic list of single-character delimiters, so two paths
# came out "a,b" with no space and three alternated "," and " ". A judge found
# it on 2026-09-09 against a fixture that only ever staged one path, which is
# exactly the assertion this file was entitled to make and had not.
printf 'x\n' > "$HOOKDIR/bin/tool"
mkdir -p "$HOOKDIR/setup/guards"
printf 'x\n' > "$HOOKDIR/setup/guards/probe.py"
( cd "$HOOKDIR" && git add bin/tool setup/guards/probe.py )
lock_out="$(cd "$HOOKDIR" && GIT_CONFIG_PARAMETERS="$LEAK_PARAMETERS" \
  GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=setup/hooks \
  HOME="$HOOKDIR/home" bash ./hook 2>&1 || true)"
case "$lock_out" in
  *'deliberate stub failure'*) : ;;
  *) fail "115: the stub suite never ran, so nothing was established about the hook's env — $lock_out" ;;
esac
case "$lock_out" in
  *SUITE-SAW*)
    fail "the hook handed git config injection to the suite it launches: $(printf '%s' "$lock_out" | grep SUITE-SAW | tr '\n' ' ')" ;;
  *)
    pass "setup/hooks/pre-commit strips the GIT_CONFIG family from the suite it launches (second lock, driven)" ;;
esac

# And the headline it prints must name the staged path that actually triggered
# the suite. It said "this commit changes bin/." unconditionally for as long as
# the trigger has matched six path shapes; on 2026-09-09 a commit staging only
# setup/tests/112 was told it changed bin/.
case "$lock_out" in
  *'failed after '*'and this commit changes bin/tool, setup/guards/probe.py, which triggers it.'*)
    pass "the hook's failure headline names every staged path that triggered the suite, joined readably" ;;
  *'and this commit changes bin/.'*)
    fail "the hook still claims 'this commit changes bin/.' without reading the staged set: $lock_out" ;;
  *'bin/tool,setup/guards/probe.py'*)
    fail "the headline joined its trigger paths with a bare comma — the delimiter was read as a cyclic single-character list, not as the string ', ': $lock_out" ;;
  *)
    fail "the hook's failure headline did not name its trigger paths: $lock_out" ;;
esac
