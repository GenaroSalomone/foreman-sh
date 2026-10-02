#!/usr/bin/env bash
#
# install-hooks.sh — put this repo's git hooks in .git/hooks, idempotently.
#
# WHY THIS EXISTS AS A COMMAND. The install used to be two `ln -sf` lines in
# setup/hooks/README.md, and prose cannot be run, cannot be tested, and cannot
# say whether it worked. On 2026-09-09 the push gate broke because the installed
# `pre-push` could not resolve a file it sources — and the suite stayed green,
# because the tests exercise fixture hooks rather than the installed ones.
#
# WHAT IS AND IS NOT INSTALLED. Only the ENTRY POINTS git invokes get a
# symlink — `pre-commit`, `pre-push`, and since 2026-09-16 `commit-msg`. The
# files they share — `suite-trigger-pattern.sh`, `attribution-pattern.sh`,
# `decisions-check.py` — are NOT installed and must never need to be: each hook
# resolves them through
# `git rev-parse --show-toplevel`, from the working tree, so a hook works as a
# bare symlink with nothing beside it. A shared file that had to be copied into
# .git/hooks would be a second install step, and a second install step is the
# thing that was forgotten. setup/tests/121 drives the INSTALLED hooks to hold
# that: it installs into a fixture and runs pre-push with no sibling present.
#
#   setup/install-hooks.sh            install (or repair) the hooks
#   setup/install-hooks.sh --check    say what is installed; change nothing
#
# Exit 0 installed/verified, 1 something is wrong and is named.
set -euo pipefail

CHECK_ONLY=0
case "${1:-}" in
  --check) CHECK_ONLY=1 ;;
  "") ;;
  *) echo "usage: install-hooks.sh [--check]" >&2; exit 2 ;;
esac

ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# The COMMON dir, not `.git`: in a linked worktree `.git` is a file and the
# hooks live in the main checkout's directory, which is the one git reads.
# Named refusal, not a bare assignment: under `set -e` a failing rev-parse would
# kill this script with no message, which is the one thing an installer must not
# do — the caller would read silence as "installed".
HOOKS=""
HOOKS="$(git -C "$ROOT" rev-parse --git-common-dir 2>/dev/null)" || {
  echo "install-hooks: $ROOT is not a git repository git can read, so there is no .git/hooks to install into." >&2
  exit 1
}
case "$HOOKS" in /*) ;; *) HOOKS="$ROOT/$HOOKS" ;; esac
HOOKS="$HOOKS/hooks"

# A configured core.hooksPath wins over .git/hooks, so installing there would
# be a no-op nobody could see. Say so rather than reporting success.
configured="$(git -C "$ROOT" config --get core.hooksPath || true)"
if [ -n "$configured" ]; then
  echo "install-hooks: core.hooksPath is set to '$configured', so git ignores $HOOKS." >&2
  echo "install-hooks: refusing to install where nothing would run it. Unset it, or install into that path yourself." >&2
  exit 1
fi

rc=0
# ORDER MATTERS ONLY FOR THE REPORT, and setup/tests/121 reads it: pre-commit
# and pre-push first, in that order, then anything added later.
for hook in pre-commit pre-push commit-msg; do
  src="$ROOT/setup/hooks/$hook"
  dst="$HOOKS/$hook"
  [ -f "$src" ] || { echo "install-hooks: missing $src" >&2; rc=1; continue; }
  [ -x "$src" ] || { echo "install-hooks: $src is not executable — git would not run it" >&2; rc=1; continue; }
  if [ "$CHECK_ONLY" = 1 ]; then
    if [ -x "$dst" ] && [ "$(cd -P "$(dirname "$dst")" && cd -P "$(dirname "$(readlink "$dst" 2>/dev/null || printf '%s' "$dst")")" && pwd)/$(basename "$src")" = "$src" ]; then
      echo "install-hooks: $hook installed"
    else
      echo "install-hooks: $hook is NOT installed at $dst" >&2
      rc=1
    fi
    continue
  fi
  mkdir -p "$HOOKS"
  ln -sf "../../setup/hooks/$hook" "$dst"
  [ -x "$dst" ] || { echo "install-hooks: $dst is not executable after install" >&2; rc=1; continue; }
  echo "install-hooks: $hook -> setup/hooks/$hook"
done

# THE BUDGETS MERGE DRIVER. Every branch that touches a test re-measures it into
# setup/test-budgets.json, so two branches always met there as a text conflict
# that was never a disagreement. .gitattributes binds the file to the driver
# `test-budgets`; this names the program behind it. In the repository config
# (the common one, so every worktree has it), never the user's: it is this
# repository's merge rule. A tree that does not carry the program skips it.
driver="python3 setup/merge-test-budgets %O %A %B"
if [ -f "$ROOT/setup/merge-test-budgets" ]; then
  if [ "$CHECK_ONLY" = 1 ]; then
    if [ "$(git -C "$ROOT" config --get merge.test-budgets.driver || true)" = "$driver" ]; then
      echo "install-hooks: merge driver test-budgets installed"
    else
      echo "install-hooks: merge driver test-budgets is NOT configured (git would merge setup/test-budgets.json as text)" >&2
      rc=1
    fi
  else
    git -C "$ROOT" config merge.test-budgets.name "setup/test-budgets.json, merged by key" \
      && git -C "$ROOT" config merge.test-budgets.driver "$driver" \
      && echo "install-hooks: merge driver test-budgets -> setup/merge-test-budgets" \
      || { echo "install-hooks: could not configure the test-budgets merge driver" >&2; rc=1; }
  fi
fi
exit "$rc"
