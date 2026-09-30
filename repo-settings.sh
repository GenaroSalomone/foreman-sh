#!/usr/bin/env bash
# The repository's description, homepage and topics, as `gh repo edit`
# commands. Run once by the maintainer after publishing; needs an authenticated
# `gh` with admin on the repository.
#
#   bash repo-settings.sh [OWNER/NAME]   # default: the repository of the cwd
set -euo pipefail

REPO="${1:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}"

gh repo edit "$REPO" \
  --description "Plan with one agent, build with many: a shell harness, built on herdr, that keeps your repository out of the planner's hands." \
  --enable-issues \
  --enable-discussions=false \
  --enable-wiki=false

for topic in coding-agents claude-code ai-agents git-worktree shell bash \
             developer-tools multi-agent agent-guardrails herdr; do
  gh repo edit "$REPO" --add-topic "$topic"
done
