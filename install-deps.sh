#!/usr/bin/env bash
# install-deps.sh — install what foreman-sh needs and recommends, through Homebrew,
# one at a time, each command printed before it runs.
#
#   ./install.sh --with-recommended            # install what is missing
#   ./install.sh --with-recommended --check    # print the commands, run none
#
# Run by install.sh, never on its own initiative: nothing is installed unless the
# person asked with --with-recommended. --plan (what install.sh --check passes)
# prints the same list and runs nothing.
#
# WHAT IT INSTALLS: packages, and only through Homebrew (macOS, or Linux with
# Homebrew on PATH). What Homebrew does not carry is NAMED with its own official
# method and never run here: git and python3 (the Command Line Tools on macOS, the
# distribution's packages on Linux), Claude Code on Linux. It configures nothing:
# Claude Code's first run, `herdr integration install claude` and
# `engram setup claude-code` stay the person's (`install.sh --check` names them).
#
# Exit 0: nothing left missing. Exit 1: something failed, still missing, or needs
# the person — every one of them named.
set -euo pipefail

PLAN=0
case "${1:-}" in
  --plan) PLAN=1 ;;
  "") ;;
  *) printf 'install-deps: unknown argument: %s\n' "$1" >&2; exit 2 ;;
esac
OS="$(uname -s)"

# kind, name, command on PATH, brew arguments ("" = not through Homebrew), why, and
# how when not through Homebrew — joined by \x1f, not a tab: `read` collapses runs
# of a whitespace IFS, and the brew arguments are often "".
# Required first, in the order install.sh --check names them; then recommended.
# rg, fd and sd are required on macOS too: hw calls all three (bin/hw).
row() { local IFS=$'\x1f'; printf '%s\n' "$*"; }
deps() {
  local claude_brew="--cask claude-code" claude_how=""
  if [ "$OS" = Linux ]; then
    claude_brew=""; claude_how="https://claude.com/claude-code (the cask is macOS only)"
  fi
  row required git git "" "worktrees and branches" \
    "$([ "$OS" = Linux ] && echo "your distribution's package (apt install git)" || echo "xcode-select --install")"
  row required python3 python3 "" "the guards and the JSON merges" \
    "$([ "$OS" = Linux ] && echo "your distribution's package (apt install python3)" || echo "xcode-select --install")"
  row required jq jq jq "hw reads projects.json with it (1.7 or newer)" ""
  row required herdr herdr herdr "every brainer and executor is a herdr pane" ""
  row required claude claude "$claude_brew" "the agent runtime" "$claude_how"
  row required ripgrep rg ripgrep "hw and the guards search with it" ""
  row required fd fd fd "hw finds files with it" ""
  row required sd sd sd "hw and the lane scripts edit files with it" ""
  [ "$OS" != Linux ] || row required node node node "the OpenCode guard plugin is an ES module (22.7 or newer)" ""
  row recommended engram engram gentleman-programming/tap/engram "memory across sessions; without it nothing is saved between them" ""
  row recommended fzf fzf fzf "hw's interactive pickers" ""
}

# A present jq older than 1.7 counts as missing: its `jq -e` treats empty input as
# success (install.sh refuses it for the same reason).
present() {  # <command>
  # type -P: under Git Bash msys-compat exports functions named jq, fd, git, ps
  # and pgrep, which `command -v` reports as installed (571, windows.yml 37045495692).
  type -P "$1" >/dev/null 2>&1 || return 1
  if [ "$1" = jq ]; then
    local v
    v="$(jq --version 2>/dev/null | sed -E 's/^jq-([0-9]+)\.([0-9]+).*/\1 \2/')"
    [ "${v% *}" -gt 1 ] 2>/dev/null || { [ "${v% *}" -eq 1 ] 2>/dev/null && [ "${v#* }" -ge 7 ] 2>/dev/null; }
  fi
}

printf 'dependencies (%s)\n' "$([ "$PLAN" = 1 ] && echo "--check: the commands are printed, none is run" || echo "each command is printed, then run")"
BREW=""
type -P brew >/dev/null 2>&1 && BREW=brew
installed="" failed="" yours="" todo=0
# The rows come in on fd 3: brew keeps the terminal's stdin (a cask may prompt),
# and nothing it reads can eat the rows still to come.
while IFS=$'\x1f' read -r -u 3 kind name cmd brew_args why how; do
  if present "$cmd"; then printf '  ok    %s\n' "$name"; continue; fi
  if [ -z "$brew_args" ]; then
    printf '  YOURS %s (%s) — %s: %s\n' "$name" "$kind" "$why" "$how"
    yours="$yours $name"; continue
  fi
  todo=1
  if [ -z "$BREW" ]; then
    printf '  MISSING %s (%s) — %s: brew install %s\n' "$name" "$kind" "$why" "$brew_args"
    failed="$failed $name"; continue
  fi
  printf '  + brew install %s   # %s, %s\n' "$brew_args" "$name" "$kind"
  [ "$PLAN" = 0 ] || continue
  # shellcheck disable=SC2086  # brew_args is a word list on purpose (--cask claude-code)
  if HOMEBREW_NO_AUTO_UPDATE="${HOMEBREW_NO_AUTO_UPDATE:-1}" brew install $brew_args; then
    hash -r
    if present "$cmd"; then installed="$installed $name"
    else printf '  WARN  brew installed %s, but %s is not on PATH (or is still too old) — put %s/bin first on PATH\n' "$name" "$cmd" "$(brew --prefix)"; failed="$failed $name"; fi
  else
    printf '  FAILED brew install %s — the output above says why; the next one is still tried\n' "$brew_args"
    failed="$failed $name"
  fi
done 3< <(deps)

if [ -z "$BREW" ] && [ "$todo" = 1 ]; then
  printf '\nHomebrew is not on PATH, so nothing was installed. Install it from https://brew.sh, or install the packages above with your system'"'"'s package manager (install.sh --check names them)\n'
fi
[ -z "$installed" ] || printf '\ninstalled:%s\n' "$installed"
[ -z "$failed" ] || printf '\nstill missing:%s\n' "$failed"
[ -z "$yours" ] || printf '\nyours to install (not through Homebrew here):%s\n' "$yours"
if [ "$PLAN" = 1 ]; then [ "$todo" = 0 ] && [ -z "$yours" ]; exit; fi
[ -z "$failed" ] && [ -z "$yours" ]
