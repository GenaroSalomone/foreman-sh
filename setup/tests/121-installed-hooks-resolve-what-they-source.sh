#!/usr/bin/env bash
# the hook git ACTUALLY runs can resolve the files it sources
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/121-installed-hooks-resolve-what-they-source.sh
#
# WHY THIS SUBJECT EXISTS, measured 2026-09-09. deebd4f made
# setup/hooks/suite-trigger-pattern.sh the single source of truth for "what can
# invalidate the suite" and both gates source it — but `pre-push` sourced it
# relative to `dirname "${BASH_SOURCE[0]}"`. git invokes the hook as
# `.git/hooks/pre-push`, a symlink, so that dirname is `.git/hooks`, where the
# shared file is not. The first real push after that commit died with
#
#     .git/hooks/pre-push: line 44: .git/hooks/suite-trigger-pattern.sh: No such file or directory
#
# THE SUITE WAS GREEN THROUGHOUT, and that is the part worth fixing: every
# existing hook assertion drives a FIXTURE hook it wrote itself, so the subject
# of the claim was never the file git runs. A green assertion whose subject is
# not the thing in production is the class this repo keeps closing.
#
# So this subject installs the hooks the documented way and runs the INSTALLED
# one, with nothing beside it in .git/hooks.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── a fixture repo that carries the real hooks and the real installer ───────
repo="$TMP/repo"
mkdir -p "$repo/setup/hooks"
git init -q "$repo"
# commit-msg and attribution-pattern.sh joined this list on 2026-09-16: the
# installer now links three entry points, and pre-push now sources the shared
# attribution pattern the same way it sources the suite pattern — so a fixture
# missing either file would fail here for a reason that has nothing to do with
# what this subject is about.
cp "$ROOT/setup/hooks/pre-push" "$ROOT/setup/hooks/pre-commit" \
   "$ROOT/setup/hooks/commit-msg" "$ROOT/setup/hooks/attribution-pattern.sh" \
   "$ROOT/setup/hooks/suite-trigger-pattern.sh" "$repo/setup/hooks/"
cp "$ROOT/setup/hooks/decisions-check.py" "$repo/setup/hooks/" 2>/dev/null || true
cp "$ROOT/setup/install-hooks.sh" "$repo/setup/"
chmod +x "$repo/setup/install-hooks.sh" "$repo/setup/hooks/pre-push" \
         "$repo/setup/hooks/pre-commit" "$repo/setup/hooks/commit-msg"
printf 'prose only\n' > "$repo/README.md"
git -C "$repo" add -A
git -C "$repo" -c user.name=Probe -c user.email=probe@example.invalid commit -qm "fixture"
head="$(git -C "$repo" rev-parse HEAD)"
zero="0000000000000000000000000000000000000000"

install_out="$(cd "$repo" && ./setup/install-hooks.sh 2>&1)" || fail "install-hooks.sh failed in the fixture: $install_out"
case "$install_out" in
  *"pre-commit -> setup/hooks/pre-commit"*"pre-push -> setup/hooks/pre-push"*"commit-msg -> setup/hooks/commit-msg"*)
    pass "install: the documented install is a command, and it links all three entry points" ;;
  *) fail "install: install-hooks.sh did not report linking all three hooks: $install_out" ;;
esac
[ -x "$repo/.git/hooks/pre-push" ] || fail "install: .git/hooks/pre-push is not executable, so git would not run it"
# THE PRECONDITION THAT MAKES THE NEXT ASSERTION MEAN ANYTHING. If a copy of the
# shared file were sitting in .git/hooks, the hook would resolve it for the
# WRONG reason and this subject would certify the bug it exists to catch.
[ ! -e "$repo/.git/hooks/suite-trigger-pattern.sh" ] \
  || fail "install: a copy of suite-trigger-pattern.sh is in .git/hooks — the resolution assertion below would pass vacuously"
[ ! -e "$repo/.git/hooks/attribution-pattern.sh" ] \
  || fail "install: a copy of attribution-pattern.sh is in .git/hooks — pre-push would resolve it for the wrong reason"
pass "install: only the entry points are installed; nothing shared is copied beside them"

# ── the installed hook resolves what it sources ─────────────────────────────
run_installed_prepush() {
  local rc=0 out
  out="$(cd "$repo" && printf 'refs/heads/main %s refs/heads/main %s\n' "$head" "$zero" \
        | ./.git/hooks/pre-push backup "file://$TMP/remote" 2>&1)" || rc=$?
  printf 'exit=%s %s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}
verdict="$(run_installed_prepush)"
case "$verdict" in
  *"No such file or directory"*)
    fail "the INSTALLED pre-push cannot resolve the file it sources — this is the 2026-09-09 push failure: $verdict" ;;
  exit=0*"touches nothing matching SUITE_TRIGGER_PATTERN"*)
    pass "resolution: the installed pre-push sources the shared pattern and reaches a decision" ;;
  *) fail "the installed pre-push did something unexpected: $verdict" ;;
esac

# And it must still REFUSE what it is there to refuse — a resolution fix that
# quietly turned the gate off would pass the assertion above.
printf 'tools change\n' > "$repo/bin-probe"
mkdir -p "$repo/bin" && printf '#!/bin/sh\n' > "$repo/bin/thing"
git -C "$repo" add -A
git -C "$repo" -c user.name=Probe -c user.email=probe@example.invalid commit -qm "touch bin/"
head="$(git -C "$repo" rev-parse HEAD)"
gated="$(run_installed_prepush)"
case "$gated" in
  exit=1*"no cached full-suite verdict"*)
    pass "gate: a range touching bin/ is still refused without a cached verdict" ;;
  *) fail "gate: a bin/ change was not refused by the installed hook: $gated" ;;
esac

# ── MUTATION ARM ────────────────────────────────────────────────────────────
# M01 — put back the resolution that broke the push: source the shared file
# relative to the hook's own path instead of the repo root. The mutant is killed
# by the message it printed, which is the exact one the real push died with.
cp "$repo/setup/hooks/pre-push" "$TMP/pre-push.mutant"
mutate_anchor 121-M01 "$TMP/pre-push.mutant" '. "$(dirname "${BASH_SOURCE[0]}")/suite-trigger-pattern.sh"'
grep -q 'dirname "${BASH_SOURCE\[0\]}"' "$TMP/pre-push.mutant" \
  || fail "M01 was not applied — the sourcing line moved; update this arm"
cp "$TMP/pre-push.mutant" "$repo/setup/hooks/pre-push"
chmod +x "$repo/setup/hooks/pre-push"
saw_mutant "M01 pre-push sourcing relative to its own symlink" "$(run_installed_prepush)" \
  "suite-trigger-pattern.sh: No such file or directory"
