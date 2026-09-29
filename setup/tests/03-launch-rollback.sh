#!/usr/bin/env bash
# a failed launch removes only what that launch created
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/03-launch-rollback.sh
#
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── launch rollback removes only what the launch created ────────────────────
awk '/^_launch_rollback\(\) \{/,/^\}/' "$ROOT/bin/hw" > "$TMP/rollback.sh"
rb_repo="$TMP/rb"; mkdir -p "$rb_repo"
( cd "$rb_repo" && git init -q -b main . && git config user.email t@t && git config user.name t \
  && echo hi > f && echo ".env.local" > .gitignore && git add -A && git commit -qm init )
rb_run() { # created branch_created
  bash -c '
    warn(){ :; }; info(){ :; }; ok(){ :; }
    LAUNCH_WT="'"$rb_repo"'/.worktrees/p"; LAUNCH_WT_CREATED='"$1"'
    LAUNCH_BRANCH="'"$2"'"; LAUNCH_BRANCH_CREATED='"$3"'
    source "'"$TMP"'/rollback.sh"; _launch_rollback "'"$rb_repo"'"'
}
git -C "$rb_repo" worktree add -q -b task/p "$rb_repo/.worktrees/p" main
echo secret > "$rb_repo/.worktrees/p/.env.local"
rb_run 0 task/p 0
[ -d "$rb_repo/.worktrees/p" ] || fail "rollback: removed a worktree it did not create"
[ -f "$rb_repo/.worktrees/p/.env.local" ] || fail "rollback: destroyed ignored content it did not create"
pass "rollback: a pre-existing worktree and its ignored files are untouched"

rb_run 1 task/p 1
[ -d "$rb_repo/.worktrees/p" ] && fail "rollback: left the worktree it created"
git -C "$rb_repo" show-ref --verify --quiet refs/heads/task/p && fail "rollback: left the branch it created"
pass "rollback: a worktree and branch this launch created are both removed"

git -C "$rb_repo" branch -q task/keep main
git -C "$rb_repo" worktree add -q "$rb_repo/.worktrees/p" task/keep
rb_run 1 task/keep 0
[ -d "$rb_repo/.worktrees/p" ] && fail "rollback: left the worktree it created"
git -C "$rb_repo" show-ref --verify --quiet refs/heads/task/keep || fail "rollback: deleted a branch it did not create"
pass "rollback: a branch that existed beforehand survives"
