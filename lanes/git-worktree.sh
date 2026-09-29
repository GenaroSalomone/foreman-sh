# shellcheck shell=bash
# lanes/git-worktree.sh — the build for a lane whose repo has no worktree
# tooling of its own: a plain `git worktree add`, no ports, no database, no
# dev server, and the two-pane layout.
#
# SOURCED BY bin/hw, never executed, exactly like every other lanes/<lane>.sh:
# projects.json names this file as the lane's `build`, and it runs at hw's top
# level with every variable hw has set, so `exit` and `die` end the launch and
# `local` is an error here.
#
# WHO USES IT. The lanes `install.sh --lane` creates for a new brain. The five
# lanes of the original installation each keep their own file, because each
# delegates to its repo's own worktree script (scripts/new-worktree.sh,
# local/wt.sh, …) — the shared rules forbid a bare `git worktree add` there.
#
# THE BARE-WORKTREE TRAP IS STATED, NOT SOLVED. A bare add carries only what
# git tracks: an ignored `.env`, a `.venv` or a `node_modules` in the checkout
# is NOT in the task's worktree. A repo that needs them gets its own
# lanes/<lane>.sh that calls its own replication script, which is what the
# original lanes do. This file says so on every launch instead of pretending.
#
# NO `git fetch`: the base is whatever `base_ref_prefix` + `base` name in the
# table, local by default, so a repo with no remote builds too.
MAIN="$(lane_checkout "$PROJ")"; BASE="$(_task_base)"
WT="$(_lane_wt_dir "$PROJ" "$TASK")"
[ -n "$MAIN" ] && [ -d "$MAIN" ] || die "checkout not found: ${MAIN:-<none in projects.json>}"
git -C "$MAIN" rev-parse --git-dir >/dev/null 2>&1 || die "$MAIN is not a git repository — this build adds a git worktree to it"

step "1/4  worktree"
if [ -e "$WT/.git" ]; then
  ok "worktree already exists, reusing: $WT (HEAD $(git -C "$WT" symbolic-ref --short HEAD 2>/dev/null || echo detached))"
elif [ -e "$WT" ]; then
  die "$WT exists and is not a git worktree — dispatch under another task name, or move it aside. Nothing was created."
else
  _gw_branch_existed=0
  git -C "$MAIN" show-ref --verify --quiet "refs/heads/$BRANCH" 2>/dev/null \
    && _gw_branch_existed=1
  mkdir -p "$(dirname "$WT")"
  if [ "$_gw_branch_existed" = 1 ]; then
    git -C "$MAIN" worktree add "$WT" "$BRANCH" >/dev/null
    LAUNCH_WT="$WT"; LAUNCH_WT_CREATED=1
    info "branch $BRANCH already existed — worktree added on it, so a rollback will not delete it"
  else
    git -C "$MAIN" worktree add -b "$BRANCH" "$WT" "$BASE" >/dev/null
    LAUNCH_WT="$WT"; LAUNCH_WT_CREATED=1
    LAUNCH_BRANCH="$BRANCH"; LAUNCH_BRANCH_CREATED=1
  fi
  ok "worktree $WT on $BRANCH (from $BASE)"
fi
[ -e "$WT/.git" ] || die "worktree creation reported success but $WT is not a worktree"
_receipt_worktree "$WT"
_refuse_if_stale_hooks "$WT" "$MAIN" "$BASE"

step "2/4  local config"
info "plain git worktree: only what git tracks is here — an ignored .env, .venv or node_modules in $MAIN was NOT carried over"
[ "$WANT_DB" = 0 ] && info "--no-db has no effect here: this lane provisions no database"

if [ "$HERE" = 1 ]; then _run_here "$WT"; exit 0; fi

_trust_dir "$WT"
_apply_sdd "$WT" "$SDD" || true

step "3/4  space + layout"
_export_hw_env "$WT"
case "$(_artifacts_dir_for "$PROJ" "$WT")" in "$WT") ;; *) _ensure_artifacts_dir "$WT" ;; esac
_build_space "$LAYOUT_DIR/repoless.json"
P_AGENT="$(_pane_by_label "$LAYOUT_OUT" agent)"
P_SHELL="$(_pane_by_label "$LAYOUT_OUT" shell)"
[ -n "$P_AGENT" ] && [ -n "$P_SHELL" ] \
  || die "layout applied but a labelled pane is missing (agent=$P_AGENT shell=$P_SHELL) — check $LAYOUT_DIR/repoless.json"
ok "agent=$P_AGENT  shell=$P_SHELL"

step "4/4  agent"
_agent_step

printf "\n  ${C_B}%s${C_0}  %s\n  branch %s off %s  ·  no ports, no database\n\n" "$LABEL" "$WT" "$BRANCH" "$BASE"
