#!/usr/bin/env bash
# init.sh — foreman-sh init: from a fresh install to a first dispatch, in one command.
#
#   foreman-sh init                       # ~/brain, lane "demo" on a toy repository
#   foreman-sh init --yes                 # no questions: every fix below is consented to
#   foreman-sh init --dry-run             # print the plan, change nothing
#   foreman-sh init --lane myapp --repo ~/code/myapp --brain ~/brain
#
# Run through the front (`install.sh init`, which the Homebrew formula links as
# `foreman-sh`), never on its own initiative. In order:
#   1. prerequisites     git, jq 1.7+, python3, herdr, claude, rg, fd, sd (and a shell
#                        it was measured with). A missing one is NAMED with its install
#                        command and the run stops; nothing is ever installed here
#                        (`foreman-sh --with-recommended` is the asked-for way).
#   2. Claude account    the welcome, login and Bypass Permissions warning: yours, named
#   3. herdr + engram    herdr's Claude integration (needed to install) and `engram setup
#                        claude-code` (recommended) — each ASKED, or done under --yes;
#                        engram's server is checked, never started
#   4. brain + lane      install.sh does the work; a toy repository is made for the
#                        default lane. What changed is MEASURED (a fingerprint before and
#                        after), not assumed
#   5. a sample brief    examples/demo/briefs/hello.md → <brain>/<lane>/briefs/, never
#                        over a file that is there
#   6. a dry-run dispatch of that brief — the proof
#   7. what is still yours: PATH, Claude Code's first run, `brain <lane>`
#
# IDEMPOTENT: a second run changes nothing and says so. It never writes inside the
# lane's repository (the toy one is created, then left alone), never reads a
# credential and never touches an engram it was not pointed at.
#
# Exit 0: the dry-run dispatch succeeded (what is still yours is listed). Exit 1:
# something is missing, refused or failed — named. Exit 2: usage.
set -uo pipefail

SRC="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INIT_CMD="${FOREMAN_SH_INIT_CMD:-./install.sh init}"
FRONT="${INIT_CMD% init}"
BRAIN_DIR="" LANE="demo" REPO="" BIN_DIR="" YES=0 DRY=0
SAMPLE_TASK=hello

usage() {
  cat <<'EOF'
usage: init.sh [--brain DIR] [--lane NAME] [--repo PATH] [--bin-dir DIR] [--yes] [--dry-run]

  --brain DIR    where the brain lives (default: ~/brain)
  --lane NAME    the first lane (default: demo, on a toy repository made for it)
  --repo PATH    that lane's repository; required for any lane but demo, unless the lane exists
  --bin-dir DIR  where hw, brain and the invokers are linked (default: ~/.local/bin)
  --yes          answer every question yes (herdr's Claude integration, engram's setup)
  --dry-run      print the plan and change nothing
  -h, --help     this text

Nothing is installed: a missing tool is named with its install command and the run stops.
EOF
}
die() { printf 'init: %s\n' "$*" >&2; exit "${2:-1}"; }
while [ $# -gt 0 ]; do
  case "$1" in
    --brain)   [ $# -ge 2 ] || die "--brain needs a directory" 2; BRAIN_DIR="$2"; shift 2 ;;
    --lane)    [ $# -ge 2 ] || die "--lane needs a name" 2; LANE="$2"; shift 2 ;;
    --repo)    [ $# -ge 2 ] || die "--repo needs a path" 2; REPO="$2"; shift 2 ;;
    --bin-dir) [ $# -ge 2 ] || die "--bin-dir needs a directory" 2; BIN_DIR="$2"; shift 2 ;;
    --yes|-y)  YES=1; shift ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $1" 2 ;;
  esac
done
[[ "$LANE" =~ ^[a-z][a-z-]*$ ]] || die "lane name '$LANE' is not a lowercase word (a-z and '-')" 2
case "$LANE" in brain|setup|work|bin|lib|layouts|lanes) die "lane name '$LANE' is reserved by the brain's own layout" 2 ;; esac

say()   { printf '  %s\n' "$*"; }
ok()    { printf '  ok    %s\n' "$*"; }
chg()   { printf '  +     %s\n' "$*"; CHANGES=$((CHANGES + 1)); }
would() { printf '  would %s\n' "$*"; }
warn()  { printf '  WARN  %s\n' "$*"; }
stage() { printf '\n[%s] %s\n' "$1" "$2"; }
CHANGES=0
YOURS=""   # what is still the person's, one line each, printed last
yours() { YOURS="${YOURS}${YOURS:+
}$1"; }

abspath() { python3 -c 'import os,sys; print(os.path.realpath(os.path.expanduser(sys.argv[1])))' "$1"; }
strip_ansi() { local esc; esc="$(printf '\033')"; sed "s/${esc}\[[0-9;]*m//g"; }

# A question, asked only with a terminal; --yes answers it. No terminal and no --yes is a no.
confirm() {
  [ "$YES" = 1 ] && return 0
  local ans=""
  if [ -t 0 ]; then printf '  %s [y/N] ' "$1" >&2; IFS= read -r ans || ans=""
  elif ( : </dev/tty ) 2>/dev/null; then printf '  %s [y/N] ' "$1" >&2; IFS= read -r ans </dev/tty || ans=""
  else return 1; fi
  case "$ans" in y|Y|yes|YES) return 0 ;; esac
  return 1
}

command -v python3 >/dev/null 2>&1 || die "python3 is required to continue (macOS: xcode-select --install; Linux: apt install python3)"
[ -n "$BRAIN_DIR" ] || BRAIN_DIR="$HOME/brain"
BRAIN="$(abspath "$BRAIN_DIR")"
BIN="$(abspath "${BIN_DIR:-$HOME/.local/bin}")"
CLAUDE_CFG="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
[ -z "$REPO" ] || REPO="$(abspath "$REPO")"
case "$SRC/" in "$BRAIN/"*) die "--brain $BRAIN is (or contains) the checkout init runs from — choose another directory" ;; esac

[ "$DRY" = 0 ] || printf 'foreman-sh init — dry run: the plan, nothing is changed\n'
[ "$DRY" = 1 ] || printf 'foreman-sh init — brain %s, lane %s\n' "$BRAIN" "$LANE"

# ── 1. prerequisites ────────────────────────────────────────────────────────
stage 1/7 "prerequisites (nothing is installed here: a missing one is named with its command)"
OS="$(uname -s)"
if [ "$OS" = Linux ]; then
  ROWS="git|worktrees and branches|apt install git
python3|the guards and the JSON merges|apt install python3
jq|hw reads projects.json with it (1.7 or newer)|apt install jq, or https://jqlang.github.io/jq/download/
herdr|every brainer and executor is a herdr pane|https://herdr.dev
claude|the agent runtime|https://claude.com/claude-code
rg|hw and the guards search with it|apt install ripgrep
fd|hw finds files with it|apt install fd-find, then link fdfind as fd
sd|hw edits files with it|apt install sd
node|the OpenCode guard plugin runs in it (22.7 or newer)|https://nodejs.org/en/download"
else
  [ "$OS" = Darwin ] || warn "$OS is not supported — macOS and Linux only"
  ROWS="git|worktrees and branches|xcode-select --install
python3|the guards and the JSON merges|xcode-select --install
jq|hw reads projects.json with it (1.7 or newer)|brew install jq
herdr|every brainer and executor is a herdr pane|brew install herdr
claude|the agent runtime|brew install --cask claude-code
rg|hw and the guards search with it|brew install ripgrep
fd|hw finds files with it|brew install fd
sd|hw edits files with it|brew install sd"
fi
MISSING=0
while IFS='|' read -r cmd why how; do
  if ! type -P "$cmd" >/dev/null 2>&1; then
    printf '  MISSING %s — %s. Install: %s\n' "$cmd" "$why" "$how"; MISSING=$((MISSING + 1)); continue
  fi
  if [ "$cmd" = jq ]; then
    v="$(jq --version 2>/dev/null | sed -E 's/^jq-([0-9]+)\.([0-9]+).*/\1 \2/')"
    if [ "${v% *}" -gt 1 ] 2>/dev/null || { [ "${v% *}" -eq 1 ] 2>/dev/null && [ "${v#* }" -ge 7 ] 2>/dev/null; }; then ok "jq ${v% *}.${v#* } (>= 1.7)"
    else printf '  MISSING jq >= 1.7 — found %s. Install: %s\n' "$(jq --version 2>/dev/null)" "$how"; MISSING=$((MISSING + 1)); fi
    continue
  fi
  ok "$cmd"
done <<EOF
$ROWS
EOF
case "${SHELL##*/}" in
  bash|zsh) ok "shell ${SHELL##*/}" ;;
  *) warn "your login shell is '${SHELL:-unset}': foreman-sh is measured with zsh and bash (chsh -s /bin/zsh switches); hw itself runs under bash either way" ;;
esac
if [ -z "$(git config --global --get user.name 2>/dev/null)" ] || [ -z "$(git config --global --get user.email 2>/dev/null)" ]; then
  warn "git has no user.name/user.email — an executor's commit fails without them. Set them: git config --global user.name 'Your Name' && git config --global user.email you@example.com"
  yours "git identity: git config --global user.name 'Your Name' && git config --global user.email you@example.com"
fi
if [ "$MISSING" -gt 0 ]; then   # MUTATION-ANCHOR: 824-M01
  if type -P brew >/dev/null 2>&1; then
    printf '\n  Homebrew can install what it carries, one `brew install` each, printed before it runs: %s --with-recommended\n' "$FRONT"
  fi
  printf '\ninit: %d prerequisite(s) missing — nothing was written. Install them (commands above) and run `%s` again.\n' "$MISSING" "$INIT_CMD" >&2
  exit 1
fi

# From here on the project's own helpers are available (the same ones install.sh and hw use).
# shellcheck source=bin/project-spaces.sh
. "$SRC/bin/project-spaces.sh"

# ── 2. the Claude account ───────────────────────────────────────────────────
stage 2/7 "the Claude account (the welcome, login and warning are yours: init names them, never answers them)"
FIRST_RUN="$(claude_first_run_pending "${CLAUDE_CONFIG_DIR:-}")"
login_state="$(claude auth status --json 2>/dev/null | jq -r 'if .loggedIn == true then "yes" elif .loggedIn == false then "no" else "" end' 2>/dev/null || true)"
[ "$login_state" != no ] || FIRST_RUN="login${FIRST_RUN:+
$FIRST_RUN}"
case "$FIRST_RUN" in *onboarding*) say "PENDING the welcome (theme, then login) was never completed" ;; *) ok "welcome completed" ;; esac
case "$FIRST_RUN" in *login*) say "PENDING not logged in" ;; *) [ "$login_state" != yes ] || ok "logged in" ;; esac
case "$FIRST_RUN" in *bypass*) say "PENDING the Bypass Permissions warning was never accepted" ;; *) ok "Bypass Permissions warning accepted" ;; esac
[ -z "$FIRST_RUN" ] || yours "Claude Code's first run, in a terminal: $(claude_first_run_command "${CLAUDE_CONFIG_DIR:-}")   (finish the welcome and login, accept the warning, /exit)"

# ── 3. herdr's integration, and engram ──────────────────────────────────────
stage 3/7 "herdr's Claude integration (needed) and engram (recommended)"
BLOCKED=""
herdr_int="$(herdr integration status 2>/dev/null || true)"
if printf '%s\n' "$herdr_int" | grep -q '^claude: current'; then
  ok "herdr's claude integration"
elif [ "$DRY" = 1 ]; then
  would "run: herdr integration install claude   (asks first; --yes answers it; creates $CLAUDE_CFG if Claude Code has not yet)"
elif confirm "install herdr's Claude integration (herdr integration install claude)?"; then
  mkdir -p "$CLAUDE_CFG"
  if herdr integration install claude >/dev/null 2>&1; then chg "herdr integration install claude"
  else BLOCKED="herdr integration install claude failed — run it yourself to see why, then run \`$INIT_CMD\` again"; fi
else
  BLOCKED="herdr's Claude integration is missing and install.sh needs it — run: herdr integration install claude   (or let init: \`$INIT_CMD --yes\`)"
fi
[ -z "$BLOCKED" ] || { printf '  MISSING %s\n' "$BLOCKED"; printf '\ninit: stopped before writing the brain.\n' >&2; exit 1; }

ENGRAM_URL="${HW_ENGRAM_URL:-http://127.0.0.1:${ENGRAM_PORT:-7437}}"
if type -P engram >/dev/null 2>&1; then
  wired="$(CFG_JSON="$(claude_account_json "${CLAUDE_CONFIG_DIR:-}")" CFG_SETTINGS="$CLAUDE_CFG/settings.json" python3 -c '
import json, os
def load(p):
    try:
        d = json.load(open(p)); return d if isinstance(d, dict) else {}
    except Exception:
        return {}
g, s = load(os.environ["CFG_JSON"]), load(os.environ["CFG_SETTINGS"])
servers = g.get("mcpServers") if isinstance(g.get("mcpServers"), dict) else {}
plugins = s.get("enabledPlugins") if isinstance(s.get("enabledPlugins"), dict) else {}
print("yes" if "engram" in servers or any(k.split("@")[0] == "engram" and v is True for k, v in plugins.items()) else "no")
' 2>/dev/null || echo no)"
  if [ "$wired" = yes ]; then ok "engram is wired into Claude Code"
  elif [ "$DRY" = 1 ]; then would "run: engram setup claude-code   (asks first; answers its allowlist question y)"
  elif confirm "wire engram into Claude Code (engram setup claude-code; its allowlist question is answered y)?"; then
    if printf 'y\n' | engram setup claude-code >/dev/null 2>&1; then chg "engram setup claude-code"
    else warn "engram setup claude-code failed — run it yourself to see why"; yours "engram setup claude-code   (answer y to its allowlist question)"; fi
  else
    say "PENDING engram is installed but Claude Code has no engram server, so reports never reach memory"
    yours "engram setup claude-code   (answer y to its allowlist question)"
  fi
  if health="$(curl -fsS --max-time 2 "$ENGRAM_URL/health" 2>/dev/null)" && printf '%s' "$health" | jq -e '.status == "ok"' >/dev/null 2>&1; then
    ok "engram serve answers at $ENGRAM_URL (version $(printf '%s' "$health" | jq -r '.version // "?"'))"
  else
    warn "engram serve does not answer at $ENGRAM_URL — memory is not saved until it runs (init never starts it): engram serve"
    yours "engram serve   (in its own terminal; init does not start it)"
  fi
else
  warn "engram not found — recommended: without it nothing is saved between sessions ($([ "$OS" = Linux ] && echo "https://github.com/Gentleman-Programming/engram/blob/main/docs/INSTALLATION.md" || echo "brew install gentleman-programming/tap/engram"), then engram setup claude-code)"
fi

# ── 4. the brain and the first lane ─────────────────────────────────────────
stage 4/7 "the brain and lane $LANE"
PJ="$BRAIN/projects.json"
EXISTING=""
[ ! -f "$PJ" ] || EXISTING="$(jq -r --arg l "$LANE" '.lanes[$l].checkout // empty' "$PJ" 2>/dev/null || true)"
TOY=0
if [ -z "$REPO" ]; then
  if [ -n "$EXISTING" ]; then REPO="$EXISTING"
  elif [ "$LANE" = demo ]; then REPO="$(abspath "$HOME/code/toy")"; TOY=1
  else die "lane '$LANE' needs --repo PATH: the repository it plans for (only the demo lane gets a toy one)"; fi
fi
case "$REPO/" in "$BRAIN/"*) die "the repository $REPO cannot live inside the brain" ;; esac
if [ "$TOY" = 1 ]; then
  if git -C "$REPO" rev-parse --git-dir >/dev/null 2>&1; then ok "toy repository $REPO"
  elif [ -e "$REPO" ] && [ -n "$(ls -A "$REPO" 2>/dev/null)" ]; then die "$REPO exists, is not empty and is not a git repository — pass --repo for the repository you mean"
  elif [ "$DRY" = 1 ]; then would "create the toy repository $REPO (a one-line README, one commit)"
  else
    mkdir -p "$REPO" \
      && git init -q -b main "$REPO" \
      && printf 'toy: a throwaway repository for trying foreman-sh.\n' > "$REPO/README.md" \
      && git -C "$REPO" add README.md \
      && git -C "$REPO" -c user.name=foreman-sh -c user.email=foreman-sh@localhost commit -q -m init \
      || die "could not create the toy repository $REPO"
    chg "toy repository $REPO"
  fi
fi

fingerprint() {  # the brain, its links, Claude's settings and the permissions file: what install.sh writes
  {
    [ ! -d "$BRAIN" ] || (cd "$BRAIN" && find . -type f -print0 | sort -z | xargs -0 shasum 2>/dev/null)
    local l
    for l in hw brain done-invoker ask-invoker channel-send decisions; do
      [ ! -L "$BIN/$l" ] || printf '%s -> %s\n' "$l" "$(readlink "$BIN/$l")"
    done
    [ ! -f "$CLAUDE_CFG/settings.json" ] || shasum "$CLAUDE_CFG/settings.json"
    [ ! -f "$(hw_permissions_file)" ] || shasum "$(hw_permissions_file)"
  } 2>/dev/null | shasum
}
ARGS=(--brain "$BRAIN" --lane "$LANE" --repo "$REPO" --bin-dir "$BIN")
# What a NEW lane would be asked is answered here, so init asks nothing install.sh would: an existing
# lane keeps its row, and an explicit value that differs from it is refused.
if [ -z "$EXISTING" ]; then
  OP="the operator"
  [ ! -f "$PJ" ] || OP="$(jq -r '.operator // "the operator"' "$PJ" 2>/dev/null || echo "the operator")"
  ARGS+=(--operator "$OP" --min-model none --requested-by none)
fi
# install.sh runs here without a terminal on its output, so it asks nothing: the permissions choice stays
# what it is (the machine file, else the recommended skip) and is named below, never rewritten.
hw_permissions_resolve "" || die "$PERMISSIONS_ERR"
if [ "$DRY" = 1 ]; then
  if [ -f "$BRAIN/.brain-install.json" ] && [ -n "$EXISTING" ]; then ok "brain $BRAIN has lane $LANE → $EXISTING (install.sh would refresh the mechanism and change nothing else)"
  else would "run: install.sh $(printf '%q ' "${ARGS[@]}")"; fi
else
  LOG="$(mktemp "${TMPDIR:-/tmp}/foreman-init.XXXXXX")"
  trap 'rm -f "$LOG"' EXIT
  before="$(fingerprint)"
  bash "$SRC/install.sh" "${ARGS[@]}" >"$LOG" 2>&1 </dev/null; rc=$?
  if [ "$rc" != 0 ]; then
    strip_ansi < "$LOG" | sed 's/^/        /'
    printf '\ninit: install.sh failed (exit %s) — the output above says why.\n' "$rc" >&2; exit 1
  fi
  after="$(fingerprint)"
  say "permissions: $PERMISSIONS ($PERMISSIONS_SRC) — change with: hw --permissions ask|skip"
  if [ "$before" = "$after" ]; then ok "brain $BRAIN and lane $LANE → $REPO were already in place: nothing changed"   # MUTATION-ANCHOR: 824-M03
  else chg "brain $BRAIN, lane $LANE → $REPO (links in $BIN)"; fi
fi

# ── 5. the sample brief ─────────────────────────────────────────────────────
stage 5/7 "a sample brief"
BRIEF="$BRAIN/$LANE/briefs/$SAMPLE_TASK.md"
TEMPLATE=""
for t in "$SRC/examples/demo/briefs/hello.md" "$SRC/setup/export/overlay/examples/demo/briefs/hello.md"; do
  [ -f "$t" ] && { TEMPLATE="$t"; break; }
done
[ -n "$TEMPLATE" ] || die "the sample brief (examples/demo/briefs/hello.md) is not in $SRC"
if [ -f "$BRIEF" ]; then ok "$BRIEF (left as it is)"   # MUTATION-ANCHOR: 824-M02
elif [ "$DRY" = 1 ]; then would "copy the sample brief to $BRIEF"
else
  mkdir -p "$(dirname "$BRIEF")" && cp "$TEMPLATE" "$BRIEF" || die "could not write $BRIEF"
  chg "$BRIEF"
fi

# ── 6. the proof: a dry-run dispatch of that brief ──────────────────────────
stage 6/7 "a dry-run dispatch of the sample brief"
HW="$BIN/hw"
NO_REPORT=""
if [ "$DRY" = 1 ]; then
  would "run: hw $LANE $SAMPLE_TASK --brief $BRIEF --sdd none --dry-run"
else
  dispatch() { (cd "$BRAIN/$LANE" && PATH="$BIN:$PATH" "$HW" "$LANE" "$SAMPLE_TASK" --brief "$BRIEF" --sdd none --dry-run "$@" </dev/null 2>&1) | strip_ansi; }
  rc=0; out="$(dispatch)" || rc=$?
  # From a plain terminal there is no brainer pane to report to (KNOWN-LIMITATIONS L7): the plan is the
  # same, minus the return channel, which `brain $LANE` supplies for the real dispatch.
  if [ "$rc" != 0 ]; then
    case "$out" in *"HW_INVOKER_PANE is UNRESOLVED"*) rc=0; out="$(dispatch --no-report)" || rc=$?; NO_REPORT=1 ;; esac
  fi
  if [ "$rc" = 0 ] && printf '%s\n' "$out" | grep -q 'dry run — nothing created'; then
    printf '%s\n' "$out" | grep -E '^  dispatch |^    (agent|worktree|base|brief|verify) ' | sed 's/^/  /'
    ok "dry run: hw $LANE $SAMPLE_TASK${NO_REPORT:+ (--no-report: a plain terminal has no pane to report to; \`brain $LANE\` has)}"
  else
    printf '%s\n' "$out" | sed 's/^/        /'
    printf '\ninit: the dry-run dispatch failed (exit %s) — the output above says why.\n' "$rc" >&2; exit 1
  fi
fi

# ── 7. what is still yours ──────────────────────────────────────────────────
stage 7/7 "what is left"
on_path=0
_IFS="$IFS"; IFS=:
for e in $PATH; do [ -n "$e" ] && [ "$(abspath "$e")" = "$BIN" ] && { on_path=1; break; }; done
IFS="$_IFS"
if [ "$on_path" = 1 ]; then ok "$BIN is on your PATH"
else
  p="$BIN"; h="$(abspath "$HOME")"; case "$p" in "$h"/*) p="\$HOME/${p#"$h"/}" ;; esac
  warn "$BIN is not on your PATH — brain and hw will not be found until it is"
  yours "PATH: add it to your shell profile, open a new terminal, then restart herdr so its panes inherit it:   export PATH=\"$p:\$PATH\""
fi
if [ -n "$YOURS" ]; then printf '%s\n' "$YOURS" | sed 's/^/  yours: /'; else ok "nothing of yours is pending"; fi

printf '\n'
if [ "$DRY" = 1 ]; then
  printf 'init: dry run — nothing was changed. Run `%s` to do it.\n' "$INIT_CMD"
elif [ "$CHANGES" = 0 ]; then
  printf 'init: nothing to change — everything was already in place.\n'
else
  printf 'init: done — %d change(s).\n' "$CHANGES"
fi
printf 'Next: brain %s   # opens the brainer; there, hw %s %s --brief %s --sdd none\n' "$LANE" "$LANE" "$SAMPLE_TASK" "$BRIEF"
