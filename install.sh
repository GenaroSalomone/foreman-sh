#!/usr/bin/env bash
# install.sh — build a brain of your own from this checkout, in one command.
#
#   ./install.sh --brain ~/brain --lane myapp --repo ~/code/myapp
#   ./install.sh --brain ~/brain --lane other --repo ~/code/other   # add a lane
#   ./install.sh --brain ~/brain --lane oc --repo ~/code/oc --vendor opencode  # OpenCode executors
#   ./install.sh --brain ~/brain --check                            # look, write nothing
#
# In a terminal, a NEW lane is asked what it would otherwise leave at a generic
# value (--operator, --min-model, --requested-by), default shown; with no terminal,
# or with all three given as options, nothing is asked.
#
# WHAT IT WRITES, AND NOTHING ELSE:
#   <brain>/                 the mechanism (bin/, layouts/, lanes/git-worktree.sh,
#                            setup/guards/ runtime files), copied from this checkout,
#                            plus projects.json, guards.json, CLAUDE.md, and per lane
#                            <brain>/<lane>/{CLAUDE.md,decisions.md,briefs/,.claude/}
#                            and, for an OpenCode lane, <brain>/<lane>/.opencode-executor/:
#                            the executor's own OPENCODE_CONFIG_DIR (its guard)
#   <bin-dir>/               symlinks hw, brain, done-invoker, ask-invoker,
#                            channel-send, decisions → <brain>/bin/ (and
#                            opencode-auto, when the run installs an OpenCode lane)
#   <claude-config>/settings.json
#                            ONE entry merged in: the Stop hook
#                            `bash '<brain>/bin/hw-stop-hook.sh' stop`
#
# It never writes inside a lane's repository: what a lane needs persisted lives
# in the brain. It never reads or writes a credential.
#
# IDEMPOTENT: a second identical run changes no file. The mechanism is the
# installer's and is refreshed on every run; configuration the user may have
# edited (projects.json entries, guards.json, CLAUDE.md, decisions.md,
# .claude/settings.json) is created when absent and otherwise only gains what
# is missing — or the run REFUSES, naming what collides, before writing.
#
# The design, what it rules out and what would reverse it:
# setup/decisions.md, "Etapa 4: el instalador".
set -euo pipefail
# NATIVE WINDOWS (Git Bash): bin/msys-compat.sh makes the native programs this
# file runs (Python, jq, fd, git) answer in bash's path spelling and with LF,
# and asks msys for real symlinks. Elsewhere OSTYPE never matches.
case "${OSTYPE:-}" in msys*|cygwin*) . "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/bin/msys-compat.sh" ;; esac

SRC="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BRAIN_DIR="" LANE="" REPO="" BASE="" CHECK=0 VENDOR="" MODEL=""
OPERATOR="" MIN_MODEL="" REQ_BY=""   # "" = not given; --min-model / --requested-by "none" = given, none
BIN_DIR_OUT="${HOME}/.local/bin"
CLAUDE_CFG="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
LINKS=(hw brain done-invoker ask-invoker channel-send decisions)
MARKER=".brain-install.json"
# The oldest OpenCode this harness has been MEASURED against (INSTALL.md). Not
# a guess at compatibility: an older one may work, and nothing here has shown it.
OPENCODE_MIN="1.18.31"

usage() {
  cat <<'EOF'
usage: install.sh --brain DIR [--lane NAME --repo PATH [--base BRANCH] [--vendor claude|opencode [--model P/M]]]
                  [--operator NAME] [--min-model haiku|sonnet|opus|none] [--requested-by required|warn|none]
                  [--bin-dir DIR] [--check]

  --brain DIR     where your brain lives (created if absent)
  --lane NAME     a lane for one product repository (lowercase word)
  --repo PATH     that repository's checkout (a git repo; never written to)
  --base BRANCH   the branch task worktrees start from (default: the repo's current branch)
  --vendor V      who runs the lane's executors: claude (default) or opencode. The
                  brainer is Claude Code either way
  --model P/M     an opencode lane's default model, provider/model (default: opencode's own)
  --operator NAME the name hw's messages give for whoever decides (default: "the operator")
  --min-model T   the lowest Claude tier the lane launches without --below-floor-why:
                  haiku, sonnet, opus, or none (default: none)
  --requested-by M  whether every dispatch must cite the request it answers
                  (requested_by: in the brief): required, warn, or none (default: none)
  --bin-dir DIR   where hw, brain and the invokers are linked (default: ~/.local/bin)
  --check         verify prerequisites and report what a run would do; write nothing

With a terminal, a new lane is asked for each of --operator, --min-model and
--requested-by that was not given; without one nothing is asked. A lane that
exists keeps what its row says; an explicit value that differs is refused.

The Claude Code settings merged are $CLAUDE_CONFIG_DIR/settings.json when that
is set, ~/.claude/settings.json otherwise.
EOF
}

die()  { printf 'install: %s\n' "$*" >&2; exit 1; }
say()  { printf '  %s\n' "$*"; }
ok()   { printf '  ok    %s\n' "$*"; }
chg()  { printf '  +     %s\n' "$*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --brain)   [ $# -ge 2 ] || die "--brain needs a directory"; BRAIN_DIR="$2"; shift 2 ;;
    --lane)    [ $# -ge 2 ] || die "--lane needs a name"; LANE="$2"; shift 2 ;;
    --repo)    [ $# -ge 2 ] || die "--repo needs a path"; REPO="$2"; shift 2 ;;
    --base)    [ $# -ge 2 ] || die "--base needs a branch"; BASE="$2"; shift 2 ;;
    --vendor)  [ $# -ge 2 ] || die "--vendor needs claude or opencode"; VENDOR="$2"; shift 2 ;;
    --model)   [ $# -ge 2 ] || die "--model needs provider/model"; MODEL="$2"; shift 2 ;;
    --operator) [ $# -ge 2 ] || die "--operator needs a name"; OPERATOR="$2"; shift 2 ;;
    --min-model) [ $# -ge 2 ] || die "--min-model needs haiku, sonnet, opus or none"; MIN_MODEL="$2"; shift 2 ;;
    --requested-by) [ $# -ge 2 ] || die "--requested-by needs required, warn or none"; REQ_BY="$2"; shift 2 ;;
    --bin-dir) [ $# -ge 2 ] || die "--bin-dir needs a directory"; BIN_DIR_OUT="$2"; shift 2 ;;
    --check)   CHECK=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $1" ;;
  esac
done
[ -n "$BRAIN_DIR" ] || { usage >&2; die "--brain is required"; }
if [ -n "$LANE" ] || [ -n "$REPO" ]; then
  [ -n "$LANE" ] && [ -n "$REPO" ] || die "--lane and --repo go together"
  [[ "$LANE" =~ ^[a-z][a-z-]*$ ]] || die "lane name '$LANE' is not a lowercase word (a-z and '-')"
  case "$LANE" in brain|setup|work|bin|layouts|lanes) die "lane name '$LANE' is reserved by the brain's own layout" ;; esac
fi
[ -z "$BASE" ] || [ -n "$LANE" ] || die "--base only means something with --lane"
[ -z "$VENDOR" ] || [ -n "$LANE" ] || die "--vendor only means something with --lane"
# Codex is not offered: nothing here can prove a Codex executor end to end today
# (INSTALL.md, "Limits, stated").
case "$VENDOR" in
  ""|claude|opencode) ;;
  *) die "--vendor must be claude or opencode (got: $VENDOR) — Codex lanes are not installed by this script" ;;
esac
[ -z "$MODEL" ] || [ -n "$LANE" ] || die "--model only means something with --lane"
[ -z "$MODEL" ] || [[ "$MODEL" =~ ^[^/[:space:]]+/[^[:space:]]+$ ]] || die "--model must be provider/model (got: $MODEL) — try: opencode models"

[ -z "$OPERATOR$MIN_MODEL$REQ_BY" ] || [ -n "$LANE" ] || die "--operator, --min-model and --requested-by only mean something with --lane"
valid_operator() { [ -n "$1" ] && [ "${#1}" -le 60 ] && [[ "$1" != *[[:cntrl:]]* ]]; }
[ -z "$OPERATOR" ] || valid_operator "$OPERATOR" || die "--operator must be one line of 1 to 60 characters"
case "$MIN_MODEL" in ""|haiku|sonnet|opus|none) ;; *) die "--min-model must be haiku, sonnet, opus or none (got: $MIN_MODEL)" ;; esac
case "$REQ_BY" in ""|required|warn|none) ;; *) die "--requested-by must be required, warn or none (got: $REQ_BY)" ;; esac

abspath() {  # absolute, symlinks resolved for the parts that exist
  local p
  p="$(python3 -c 'import os,sys; print(os.path.realpath(os.path.expanduser(sys.argv[1])))' "$1")" || return
  # Native Windows Python answers D:\a\b (and ends its line in \r); readlink and
  # every shell here say /d/a/b. Measured 2026-09-28: a second identical install
  # refused its own links as "another brain owns that name". msys/cygwin only.
  case "${OSTYPE:-}" in msys*|cygwin*) p="$(cygpath -u "${p%$'\r'}")" ;; esac
  printf '%s\n' "$p"
}

# A LANE THAT EXISTS KEEPS WHAT ITS ROW SAYS unless the run names otherwise.
# Re-running the plain command over a lane someone edited by hand (its model,
# its vendor) must not turn into a refusal nothing can get past: only an
# EXPLICIT --vendor/--model that differs is refused, in the plan below.
# The row's kit comes with it: a row whose vendor is opencode keeps the
# opencode_config_dir it has (or has not), instead of being compared against
# the one a new lane would get.
ROW_VENDOR="" ROW_KIT="" ROW_EXISTS="" ROW_OPERATOR="" ROW_FLOOR="" ROW_REQBY=""
adopt_lane_row() {
  local row rm
  row="$(python3 - "$(abspath "$BRAIN_DIR")/projects.json" "$LANE" <<'PY' 2>/dev/null || true
import json, sys
try:
    doc = json.load(open(sys.argv[1]))
    cur = doc["lanes"].get(sys.argv[2])
except Exception:
    sys.exit(0)
op = doc.get("operator", "")
op = op if isinstance(op, str) else ""
# \x1f, not a tab: `read` collapses runs of a whitespace IFS, and most of these are often "".
if cur is None:
    print("\x1f".join(["", "", "", "0", op, "", "", ""]))
else:
    floor = cur.get("model_floor")
    floor = floor.get("tier", "") if isinstance(floor, dict) else ""
    print("\x1f".join([cur.get("vendor", "claude"), cur.get("model", ""), cur.get("opencode_config_dir", ""),
                       "1", op, floor, cur.get("requested_by", ""), ""]))
PY
)"
  [ -n "$row" ] || return 0
  IFS=$'\x1f' read -r ROW_VENDOR rm ROW_KIT ROW_EXISTS ROW_OPERATOR ROW_FLOOR ROW_REQBY <<EOF
$row
EOF
  [ "$ROW_EXISTS" = 1 ] || return 0
  [ -n "$VENDOR" ] || VENDOR="$ROW_VENDOR"
  [ -n "$MODEL" ] || [ "$VENDOR" != "$ROW_VENDOR" ] || MODEL="$rm"
  # The lane's floor and request rule are chosen once, like its vendor: a re-run
  # that names neither keeps the row's.
  [ -n "$MIN_MODEL" ] || MIN_MODEL="${ROW_FLOOR:-none}"
  [ -n "$REQ_BY" ] || REQ_BY="${ROW_REQBY:-none}"
}

# What a NEW lane would leave generic is asked, with a terminal on both ends and
# only for what no option gave. Whatever is not asked stays as it always was: no
# operator name, no floor, no request rule.
ask() {  # <var> <prompt> <default label> <validator> — an empty answer keeps the default
  local var="$1" q="$2" def="$3" ok="$4" ans
  while :; do
    printf '  %s [%s]: ' "$q" "$def" >&2
    IFS= read -r ans || { printf '\n' >&2; return 0; }
    [ -n "$ans" ] || return 0
    if "$ok" "$ans"; then printf -v "$var" '%s' "$ans"; return 0; fi
    printf '  not valid — try again, or press Enter for the default\n' >&2
  done
}
is_tier()  { case "$1" in haiku|sonnet|opus|none) return 0 ;; esac; return 1; }
is_reqby() { case "$1" in required|warn|none) return 0 ;; esac; return 1; }
ask_new_lane() {
  [ "$CHECK" = 0 ] && [ -t 0 ] && [ -t 1 ] && [ -n "$LANE" ] && [ "$ROW_EXISTS" != 1 ] || return 0
  local need_op=0
  [ -n "$OPERATOR" ] || [ -n "$ROW_OPERATOR" ] || need_op=1
  [ "$need_op" = 1 ] || [ -z "$MIN_MODEL" ] || [ -z "$REQ_BY" ] || return 0
  printf '\nlane %s: what only you can say (Enter keeps the default shown)\n' "$LANE" >&2
  [ "$need_op" = 0 ] || ask OPERATOR "your name, as hw's messages say it" "the operator" valid_operator
  [ -n "$MIN_MODEL" ] || ask MIN_MODEL "lowest Claude tier its executors may run without a reason (haiku, sonnet, opus)" "none" is_tier
  [ -n "$REQ_BY" ] || ask REQ_BY "must every brief cite the request it answers (required, warn)" "none" is_reqby
}

# ── 1. prerequisites ────────────────────────────────────────────────────────
printf 'prerequisites\n'
missing=0
need() {  # <command> <why> <how>
  if command -v "$1" >/dev/null 2>&1; then ok "$1"
  else printf '  MISSING %s — %s. Install: %s\n' "$1" "$2" "$3"; missing=1; fi
}
[ "$(uname -s)" = Linux ] && PKG="apt install" || PKG="brew install"
need git     "worktrees and branches"                "$([ "$(uname -s)" = Linux ] && echo "apt install git" || echo "xcode-select --install")"
need jq      "hw reads projects.json with it"        "$PKG jq"
# jq 1.6 (Debian 12, Ubuntu 22.04) exits 0 on EMPTY input under `jq -e`; 1.7 exits 4. Several
# checks read "jq -e succeeded" as "the reply was what I asked for", so an old jq is refused.
if command -v jq >/dev/null 2>&1; then
  jq_ver="$(jq --version 2>/dev/null | sed -E 's/^jq-([0-9]+)\.([0-9]+).*/\1.\2/' || true)"
  case "$jq_ver" in
    [0-9]*.[0-9]*)
      if [ "${jq_ver%%.*}" -gt 1 ] || { [ "${jq_ver%%.*}" -eq 1 ] && [ "${jq_ver#*.}" -ge 7 ]; }; then ok "jq $jq_ver (>= 1.7)"
      else printf '  MISSING jq >= 1.7 — found %s, whose `jq -e` treats empty input as success. Install: https://jqlang.github.io/jq/download/\n' "$jq_ver"; missing=1; fi ;;
  esac
fi
need python3 "the guards and the JSON merges"        "$([ "$(uname -s)" = Linux ] && echo "apt install python3" || echo "xcode-select --install")"
need herdr   "every brainer and executor is a herdr pane" "https://herdr.dev"
need claude  "the agent runtime"                     "https://claude.com/claude-code"
command -v python3 >/dev/null 2>&1 || die "python3 is required to continue"
MODEL_GIVEN="$MODEL"
[ -z "$LANE" ] || adopt_lane_row
: "${VENDOR:=claude}"
# After the row is adopted: --model over an existing opencode lane needs no --vendor.
[ -z "$MODEL_GIVEN" ] || [ "$VENDOR" = opencode ] || die "--model only means something with --vendor opencode"
# opencode-auto only for a run that installs an OpenCode lane: a Claude-only
# user who already has one of their own is not refused over a name they never use.
[ "$VENDOR" != opencode ] || LINKS+=(opencode-auto)
bash_major="${BASH_VERSINFO[0]}"
case "$(uname -s)" in
  Darwin) ok "macOS" ;;
  Linux)  ok "Linux (tested in a container; no macOS Keychain — see KNOWN-LIMITATIONS.md)" ;;
  MINGW*|MSYS*|CYGWIN*)
    # Native Windows installs, and says what is and is not proven (INSTALL.md, Windows).
    say "WARN  native Windows (Git Bash): measured by the setup suite on a CI runner with herdr and the agents stubbed; a real machine is unverified (KNOWN-LIMITATIONS.md L1b). WSL2 is the proven way"
    # The guard's hook is Windows Python run from Git Bash; an MSYS2 or Cygwin
    # python3 first on PATH makes every hook refuse (it cannot decide there).
    case "$(python3 -c 'import os, sys; print(os.name, sys.platform)' 2>/dev/null)" in
      "nt win32"*) ok "python3 is native Windows Python" ;;
      *) printf '  MISSING native Windows python3 first on PATH — found %s, and the read-only guard refuses every call under it. Install: https://www.python.org/downloads/windows/\n' "$(command -v python3 || echo none)"; missing=1 ;;
    esac
    # hw and this installer LINK; with Developer Mode off Windows refuses a
    # symlink, and MSYS's default would have copied instead, silently.
    _lt="$(mktemp -d "${TMPDIR:-/tmp}/brain-install-link.XXXXXX")"
    if : > "$_lt/a" && ln -s a "$_lt/l" 2>/dev/null && [ -L "$_lt/l" ]; then ok "symlinks"
    else printf '  MISSING symlinks — hw and this installer link files. Enable Windows Developer Mode (Settings → System → For developers), then run this again\n'; missing=1; fi
    rm -rf "$_lt" ;;
  *)      say "WARN  $(uname -s) is not supported — macOS and Linux only" ;;
esac
[ "$bash_major" -ge 3 ] || { printf '  MISSING bash >= 3\n'; missing=1; }
if command -v herdr >/dev/null 2>&1; then
  # Captured first: `| grep -q` exits on the first match, herdr takes SIGPIPE,
  # and pipefail turns an installed integration into a missing one.
  herdr_int="$(herdr integration status 2>/dev/null || true)"
  if printf '%s\n' "$herdr_int" | grep -q '^claude: current'; then
    ok "herdr's claude integration"
  else
    printf "  MISSING herdr's claude integration — hw reads an agent's state through it. Install: herdr integration install claude\n"
    missing=1
  fi
fi
if [ "$VENDOR" = opencode ]; then
  need opencode "this lane's executors run in it" "https://opencode.ai"
  if command -v opencode >/dev/null 2>&1; then
    oc_ver="$(opencode --version 2>/dev/null | head -1 | tr -d '[:space:]' || true)"
    if python3 - "$oc_ver" "$OPENCODE_MIN" <<'PY'
import re, sys
def v(s):
    m = re.fullmatch(r"v?(\d+)\.(\d+)\.(\d+)", s)
    return tuple(map(int, m.groups())) if m else None
have, want = v(sys.argv[1]), v(sys.argv[2])
sys.exit(0 if have is not None and have >= want else 1)
PY
    then ok "opencode $oc_ver (>= $OPENCODE_MIN)"
    else
      printf '  MISSING opencode >= %s — found %s. Install: npm install -g opencode-ai@latest\n' "$OPENCODE_MIN" "${oc_ver:-<no version>}"
      missing=1
    fi
  fi
  if command -v herdr >/dev/null 2>&1; then
    if printf '%s\n' "$herdr_int" | grep -q '^opencode: current'; then
      ok "herdr's opencode integration"
    else
      printf "  MISSING herdr's opencode integration — hw reads an opencode executor's state through it. Install: herdr integration install opencode\n"
      missing=1
    fi
  fi
fi
for soft in engram fzf rg; do
  if command -v "$soft" >/dev/null 2>&1; then ok "$soft"
  else
    case "$soft" in
      engram) say "WARN  engram not found — optional: persistent memory for brainers and executors (brew install gentleman-programming/tap/engram)" ;;
      fzf)    say "WARN  fzf not found — optional: only hw's interactive pickers use it (brew install fzf)" ;;
      rg)     say "WARN  rg not found — optional: the shipped rules suggest it (brew install ripgrep)" ;;
    esac
  fi
done
if [ "$missing" = 1 ]; then
  [ "$CHECK" = 1 ] && { say "a real run would stop here"; exit 1; }
  die "install the missing prerequisites above and run this again; nothing was written"
fi
ask_new_lane
: "${MIN_MODEL:=none}" "${REQ_BY:=none}"

# ── 1b. what is the person's, said BEFORE they meet it ─────────────────────
# Measured 2026-09-24 on a new macOS user: every tool worked, and each first
# stopped on a screen Claude Code shows once per account. Two are a person's to
# answer — the login (inside the welcome) and the Bypass Permissions warning —
# and this installer answers neither: it names them, with the one command that
# clears both. It never writes the keys that record them.
# shellcheck source=bin/project-spaces.sh
. "$SRC/bin/project-spaces.sh"
printf '\nfirst run of Claude Code in this account (yours to do)\n'
FIRST_RUN="$(claude_first_run_pending "${CLAUDE_CONFIG_DIR:-}")"
login_state="$(claude auth status --json 2>/dev/null | jq -r 'if .loggedIn == true then "yes" elif .loggedIn == false then "no" else "" end' 2>/dev/null || true)"
[ "$login_state" != no ] || FIRST_RUN="login${FIRST_RUN:+
$FIRST_RUN}"
case "$FIRST_RUN" in *onboarding*) ;; *) ok "welcome completed" ;; esac
case "$FIRST_RUN" in *login*) ;; *) if [ "$login_state" = yes ]; then ok "logged in"; fi ;; esac
case "$FIRST_RUN" in *bypass*) ;; *) ok "Bypass Permissions warning accepted" ;; esac
case "$FIRST_RUN" in *onboarding*) say "PENDING the welcome (theme, then login) was never completed — \`claude auth login\` alone does not complete it, and the first interactive claude would show it again" ;; esac
case "$FIRST_RUN" in *login*) say "PENDING not logged in" ;; esac
case "$FIRST_RUN" in *bypass*) say "PENDING the Bypass Permissions warning was never accepted — every brainer and executor starts in that mode" ;; esac
[ -z "$FIRST_RUN" ] || say "  do it once, in a terminal: $(claude_first_run_command "${CLAUDE_CONFIG_DIR:-}")   — finish the welcome and login, accept the warning, /exit"

# engram: DECLARED, NOT INSTALLED. Wiring it into Claude Code is engram's own
# `engram setup claude-code`, which writes into the user's Claude config by its
# own rules — outside the list of files this installer says it writes, and a
# second registration where the engram plugin already provides one. So the
# check is here, with the command; the choice to run it is the person's.
ENGRAM_WIRED=""
if command -v engram >/dev/null 2>&1; then
  ENGRAM_WIRED="$(CFG_JSON="$(claude_account_json "${CLAUDE_CONFIG_DIR:-}")" CFG_SETTINGS="$CLAUDE_CFG/settings.json" python3 -c '
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
  if [ "$ENGRAM_WIRED" = yes ]; then ok "engram is wired into Claude Code"
  else say "PENDING engram is installed but Claude Code has no engram server, so reports never reach memory — run: engram setup claude-code"; fi
fi

BRAIN="$(abspath "$BRAIN_DIR")"
[ -z "$REPO" ] || REPO="$(abspath "$REPO")"
BIN_OUT="$(abspath "$BIN_DIR_OUT")"
# The source checkout is itself a brain, with its own table: installing onto
# it (or around it) would merge a new lane into ITS projects.json.
case "$SRC/" in "$BRAIN/"*) die "--brain $BRAIN is (or contains) the checkout this installer runs from — choose a new directory" ;; esac

# ── 2. every refusal, before the first write ────────────────────────────────
printf '\nchecks\n'
if [ -e "$BRAIN" ] && [ ! -f "$BRAIN/$MARKER" ] && [ -n "$(ls -A "$BRAIN" 2>/dev/null)" ]; then
  die "$BRAIN exists, is not empty, and was not made by this installer (no $MARKER) — choose another --brain; nothing was written"
fi
if [ -n "$REPO" ]; then
  [ -d "$REPO" ] || die "--repo $REPO does not exist"
  git -C "$REPO" rev-parse --git-dir >/dev/null 2>&1 || die "--repo $REPO is not a git repository"
  case "$REPO/" in "$BRAIN/"*) die "the repo cannot live inside the brain" ;; esac
  case "$BRAIN/" in "$REPO/"*) die "the brain cannot live inside the repo $REPO: nothing of the harness is written into a product repository" ;; esac
  if [ -z "$BASE" ]; then
    BASE="$(git -C "$REPO" symbolic-ref --short HEAD 2>/dev/null)" \
      || die "$REPO has a detached HEAD; name the branch tasks start from with --base"
  fi
  git -C "$REPO" rev-parse --verify --quiet "refs/heads/$BASE" >/dev/null \
    || die "branch '$BASE' does not exist in $REPO"
  ok "repo $REPO (base $BASE)"
fi

# The JSON half of the plan: decide everything, write nothing yet.
PLAN="$(mktemp "${TMPDIR:-/tmp}/brain-install.XXXXXX")"
trap 'rm -f "$PLAN"' EXIT
STOP_CMD="bash '$BRAIN/bin/hw-stop-hook.sh' stop"
OC_KIT=""
if [ -n "$ROW_VENDOR" ] && [ "$VENDOR" = "$ROW_VENDOR" ]; then OC_KIT="$ROW_KIT"
elif [ "$VENDOR" = opencode ]; then OC_KIT="$BRAIN/$LANE/.opencode-executor"; fi
python3 - "$BRAIN" "$LANE" "$REPO" "$BASE" "$CLAUDE_CFG/settings.json" "$STOP_CMD" "$PLAN" "$VENDOR" "$MODEL" "$OC_KIT" "$OPERATOR" "$MIN_MODEL" "$REQ_BY" <<'PY'
import json, os, sys
brain, lane, repo, base, settings, stop_cmd, plan, vendor, model, oc_kit, operator, min_model, req_by = sys.argv[1:14]
out = {"refuse": [], "notes": []}
BAD = object()   # unparseable — distinct from a file whose JSON is literally `false`

def load(path):
    if not os.path.exists(path):
        return None
    try:
        with open(path) as f:
            return json.load(f)
    except Exception as e:
        out["refuse"].append("%s is not valid JSON (%s) — fix or move it; nothing was written" % (path, e))
        return BAD

# projects.json: a lane that exists must point at the same repo.
pj = load(os.path.join(brain, "projects.json"))
if lane and isinstance(pj, dict):
    cur = pj.get("lanes", {}).get(lane)
    if cur is not None and cur.get("checkout") != repo:
        out["refuse"].append("lane '%s' already exists in %s/projects.json with checkout %s, not %s — pick another lane name"
                             % (lane, brain, cur.get("checkout"), repo))
    # The vendor is chosen once, when the lane is created: a second run that
    # names another one would otherwise report success and change nothing.
    elif cur is not None:
        floor = cur.get("model_floor")
        have = (cur.get("vendor", "claude"), cur.get("model", ""), cur.get("opencode_config_dir", ""))
        want = (vendor, model, oc_kit)
        have_rules = ((floor.get("tier") if isinstance(floor, dict) else None) or "none", cur.get("requested_by") or "none")
        if have_rules != (min_model, req_by):
            out["refuse"].append("lane '%s' already exists in %s/projects.json with min-model=%s requested-by=%s, not min-model=%s requested-by=%s — edit projects.json by hand to change them"
                                 % (lane, brain, have_rules[0], have_rules[1], min_model, req_by))
        if have != want:
            out["refuse"].append("lane '%s' already exists in %s/projects.json as vendor=%s model=%s opencode_config_dir=%s, not vendor=%s model=%s opencode_config_dir=%s — edit projects.json by hand to change it, or pick another lane name"
                                 % (lane, brain, have[0], have[1] or "<default>", have[2] or "<none>", want[0], want[1] or "<default>", want[2] or "<none>"))
    # The operator is the table's, said once: a name given now over a different one is
    # not silently kept nor silently replaced.
    if operator and isinstance(pj.get("operator"), str) and pj["operator"] != operator:
        out["refuse"].append("%s/projects.json already names the operator %r, not %r — edit it by hand to change it"
                             % (brain, pj["operator"], operator))
    for other, v in pj.get("lanes", {}).items():
        if other != lane and lane in (v.get("hw_aliases", []) + v.get("brain_aliases", [])):
            out["refuse"].append("lane name '%s' is already an alias of lane '%s'" % (lane, other))

gj = load(os.path.join(brain, "guards.json"))
if lane and isinstance(gj, dict):
    cur = gj.get("lanes", {}).get(lane)
    if cur is not None and cur.get("repo") != repo:
        out["refuse"].append("lane '%s' already exists in %s/guards.json protecting %s, not %s"
                             % (lane, brain, cur.get("repo"), repo))

# The user's Claude Code settings: merge one Stop hook, or refuse naming the clash.
st = load(settings)
if isinstance(st, dict):
    hooks = st.get("hooks", {})
    if not isinstance(hooks, dict) or not isinstance(hooks.get("Stop", []), list):
        out["refuse"].append("%s: `hooks.Stop` is not a list — merge by hand: add a Stop hook running %s" % (settings, stop_cmd))
    else:
        cmds = [h.get("command", "") for g in hooks.get("Stop", []) if isinstance(g, dict)
                for h in g.get("hooks", []) if isinstance(h, dict)]
        theirs = [c for c in cmds if "hw-stop-hook.sh" in c and c != stop_cmd]
        if theirs:
            out["refuse"].append("%s already runs another brain's Stop hook (%s) — one user, one brain; remove it or install into that brain"
                                 % (settings, theirs[0]))
        out["stop_present"] = stop_cmd in cmds
elif st is None:
    out["stop_present"] = False
elif st is not BAD:
    out["refuse"].append("%s is valid JSON but not an object — fix or move it" % settings)

# The brain's own .claude/settings.json files are merged later: a shape the
# merge cannot handle is refused HERE, before the first write, not as a
# traceback halfway through.
# "brain" always: the merge pass adds the root lane whenever a lane is given.
lanes = sorted(set(gj.get("lanes", {}) if isinstance(gj, dict) else []) | ({lane, "brain"} if lane else set()))
for name in lanes:
    path = os.path.join(brain if name == "brain" else os.path.join(brain, name), ".claude", "settings.json")
    doc = load(path)
    if doc is None or doc is BAD:
        continue
    pre = doc.get("hooks", {}).get("PreToolUse", []) if isinstance(doc, dict) and isinstance(doc.get("hooks", {}), dict) else None
    deny = doc.get("permissions", {}).get("deny", []) if isinstance(doc, dict) and isinstance(doc.get("permissions", {}), dict) else None
    if not isinstance(pre, list) or not all(isinstance(g, dict) and isinstance(g.get("hooks", []), list) and all(isinstance(h, dict) for h in g.get("hooks", [])) for g in pre) \
            or not isinstance(deny, list):
        out["refuse"].append("%s does not have the shape this installer merges into (an object with a hooks.PreToolUse list and a permissions.deny list) — fix or move it" % path)
json.dump(out, open(plan, "w"))
PY
refusals="$(jq -r '.refuse[]' "$PLAN")"
[ -z "$refusals" ] || { printf '%s\n' "$refusals" | sed 's/^/  REFUSED /' >&2; die "nothing was written"; }
d="$BIN_OUT"
while [ ! -e "$d" ] && [ "$d" != / ]; do d="$(dirname "$d")"; done
[ -d "$d" ] || die "$d is not a directory, so $BIN_OUT cannot be created — choose another --bin-dir. Nothing was written"
for l in "${LINKS[@]}"; do
  t="$BIN_OUT/$l"
  if [ -L "$t" ]; then
    # On native Windows msys answers a link's target in its own spelling: one
    # under %TEMP% reads back as /tmp/..., not the /c/... this installer wrote.
    # There the question is whether it is the same file; elsewhere, the text.
    [ "$(readlink "$t")" = "$BRAIN/bin/$l" ] \
      || { case "${OSTYPE:-}" in msys*|cygwin*) [ "$(readlink "$t")" -ef "$BRAIN/bin/$l" ] ;; *) false ;; esac; } \
      || die "$t already links to $(readlink "$t"), not $BRAIN/bin/$l — another brain owns that name; remove it or choose --bin-dir. Nothing was written"
  elif [ -e "$t" ]; then
    die "$t exists and is not a link this installer made — remove it or choose --bin-dir. Nothing was written"
  fi
done
ok "no collision with an existing brain, lane, link or settings entry"

if [ "$CHECK" = 1 ]; then
  printf '\n--check: nothing written. A run would install into %s' "$BRAIN"
  [ -z "$LANE" ] || printf ', lane %s over %s (base %s)' "$LANE" "$REPO" "$BASE"
  printf '\n'
  exit 0
fi

# ── 3. the mechanism ────────────────────────────────────────────────────────
printf '\nmechanism → %s\n' "$BRAIN"
mkdir -p "$BRAIN/setup/guards" "$BRAIN/lanes" "$BRAIN/work"
sync_dir() {  # <src dir> <dst dir> — the installer owns dst: mirror it
  local out n
  # Git for Windows ships no rsync: the same mirror in Python (checksum, mode,
  # links, deletions), counted the same way. A mirror that failed is fatal:
  # counting its empty output once said "up to date" over a brain with no bin/.
  if command -v rsync >/dev/null 2>&1; then
    out="$(rsync -a --delete --checksum --itemize-changes "$1/" "$2/")" || die "rsync could not mirror $1 into $2"
    n="$(printf '%s\n' "$out" | grep -c '^[<>c*]' || true)"
  else
    n="$(python3 - "$1" "$2" <<'PY'
import filecmp, os, shutil, stat, sys
src, dst = sys.argv[1], sys.argv[2]
n = 0


def drop(b):
    if os.path.isdir(b) and not os.path.islink(b):
        shutil.rmtree(b)
    else:
        os.unlink(b)


os.makedirs(dst, exist_ok=True)
for d, dirs, files in os.walk(src):
    t = os.path.normpath(os.path.join(dst, os.path.relpath(d, src)))
    for name in sorted(dirs + files):
        a, b = os.path.join(d, name), os.path.join(t, name)
        if os.path.islink(a):
            if os.path.islink(b) and os.readlink(b) == os.readlink(a):
                continue
            if os.path.lexists(b):
                drop(b)
            os.symlink(os.readlink(a), b)
            n += 1
        elif os.path.isdir(a):
            if os.path.lexists(b) and (os.path.islink(b) or not os.path.isdir(b)):
                drop(b)
            os.makedirs(b, exist_ok=True)
        else:
            same = (os.path.isfile(b) and not os.path.islink(b) and filecmp.cmp(a, b, shallow=False)
                    and stat.S_IMODE(os.stat(a).st_mode) == stat.S_IMODE(os.stat(b).st_mode))
            if not same:
                if os.path.lexists(b):
                    drop(b)
                shutil.copy2(a, b)
                n += 1
    dirs[:] = [x for x in dirs if not os.path.islink(os.path.join(d, x))]
# Top-down, then reversed: a bottom-up walk takes its subdirectories from each
# entry's own path, which on native Windows is Windows' spelling, not the one
# dst was given in, and every file under a subdirectory then looked orphaned
# (deleted, and copied back on the next run: 187 and 198 on windows-latest).
for d, dirs, files in reversed(list(os.walk(dst))):
    rel = os.path.relpath(d, dst)
    for name in dirs + files:
        if not os.path.lexists(os.path.join(src, rel, name)):
            drop(os.path.join(d, name))
            n += 1
print(n)
PY
)" || die "could not mirror $1 into $2 (no rsync on PATH, and the Python mirror failed)"
  fi
  [ "$n" = 0 ] && ok "$(basename "$2")/ up to date" || chg "$(basename "$2")/ — $n file(s) written"
}
put() {  # <src file> <dst file>
  if cmp -s "$1" "$2"; then ok "${2#"$BRAIN"/}"
  else cp -p "$1" "$2"; chg "${2#"$BRAIN"/}"; fi
}
sync_dir "$SRC/bin" "$BRAIN/bin"
sync_dir "$SRC/layouts" "$BRAIN/layouts"
put "$SRC/lanes/git-worktree.sh" "$BRAIN/lanes/git-worktree.sh"
for g in deny_repo_writes.py deny-repo-writes.js deny-repo-writes-codex.py specialist_roster.py; do
  put "$SRC/setup/guards/$g" "$BRAIN/setup/guards/$g"
done
# The brain guard: a product lane's executor is not launched without it, so it
# is mechanism like the read-only guard. Its list of programs is the operator's
# to extend, so it is written once and never overwritten.
for g in deny_brain_writes.py deny-brain-writes.js brain-guard.opencode.json; do
  put "$SRC/setup/guards/$g" "$BRAIN/setup/guards/$g"
done
sync_dir "$SRC/setup/guards/brain-guard" "$BRAIN/setup/guards/brain-guard"
if [ -f "$BRAIN/setup/brain-guard-programs.txt" ]; then ok "setup/brain-guard-programs.txt (yours, kept)"
else put "$SRC/setup/brain-guard-programs.txt" "$BRAIN/setup/brain-guard-programs.txt"; fi
# Every OpenCode executor's copy of the guard module is mechanism too, so a
# guard fix reaches it on ANY run — not only on one that names its lane.
if [ -f "$BRAIN/projects.json" ]; then
  while IFS= read -r kit; do
    [ -d "$kit/setup/guards" ] || continue
    case "$kit/" in "$BRAIN/"*) put "$SRC/setup/guards/deny-repo-writes.js" "$kit/setup/guards/deny-repo-writes.js" ;; esac
  done < <(jq -r '.lanes[] | .opencode_config_dir // empty' "$BRAIN/projects.json")
fi
src_sha="$(git -C "$SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
marker_new="$(printf '{\n  "installed_from": "%s",\n  "commit": "%s"\n}\n' "$SRC" "$src_sha")"
if [ "$(cat "$BRAIN/$MARKER" 2>/dev/null)" = "$marker_new" ]; then ok "$MARKER"
else printf '%s\n' "$marker_new" > "$BRAIN/$MARKER"; chg "$MARKER (commit $src_sha)"; fi

write_once() {  # <dst> — body on stdin; never overwrites what the user may have edited
  if [ -e "$1" ]; then cat >/dev/null; ok "${1#"$BRAIN"/} (kept)"
  else mkdir -p "$(dirname "$1")"; cat > "$1"; chg "${1#"$BRAIN"/}"; fi
}

write_once "$BRAIN/CLAUDE.md" <<'EOF'
# Rules for every agent under this brain

- A brainer (`brain <lane>`) thinks and delegates; it does not write in a
  product repository. Its read-only guard enforces that for Bash, and its
  `.claude/settings.json` deny list for Edit/Write.
- An executor (`hw <lane> <task> --brief <lane>/briefs/<task>.md --sdd none`)
  works in its own worktree and ends every task with `done-invoker "<summary>"`
  (or `done-invoker --blocked "<why>"`). Those are shell commands, not tools.
- `<lane>/decisions.md` holds what was decided and what it rules out.
- Nothing of this brain is committed into a product repository.
EOF
write_once "$BRAIN/.gitignore" <<'EOF'
work/
.hw-*
*.bak-*
EOF

# ── 4. the lane ─────────────────────────────────────────────────────────────
shim() {  # <lane> — the per-lane Claude hook; the logic lives in setup/guards
  cat <<EOF
#!/usr/bin/env python3
"""The '$1' brainer's read-only guard. Generated by install.sh; the logic is
setup/guards/deny_repo_writes.py and the policy guards.json, both at the brain
root. A module that does not load DENIES every Bash command: Claude Code runs a
command whose PreToolUse hook merely crashed."""
import json
import pathlib
import sys

LANE = "$1"


def _deny(reason):
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "PreToolUse", "permissionDecision": "deny",
        "permissionDecisionReason": reason}}))
    raise SystemExit(0)


try:
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[$2] / "setup" / "guards"))
    from deny_repo_writes import main
except Exception as exc:  # noqa: BLE001 — any failure here must deny
    try:
        payload = json.load(sys.stdin)
    except Exception:
        raise SystemExit(0)
    if payload.get("tool_name") != "Bash":
        raise SystemExit(0)
    _deny("Blocked: the %s read-only guard could not load setup/guards/deny_repo_writes.py "
          "(%s: %s). Every Bash command is refused until it loads." % (LANE, type(exc).__name__, exc))

if __name__ == "__main__":
    main(LANE)
EOF
}
put_text() {  # <dst> — body on stdin; installer-owned, refreshed when it differs
  local tmp; tmp="$(mktemp)"; cat > "$tmp"
  if cmp -s "$tmp" "$1"; then rm -f "$tmp"; ok "${1#"$BRAIN"/}"
  else mkdir -p "$(dirname "$1")"; mv "$tmp" "$1"; chg "${1#"$BRAIN"/}"; fi
}

if [ -n "$LANE" ]; then
  printf '\nlane %s → %s\n' "$LANE" "$REPO"
  mkdir -p "$BRAIN/$LANE/briefs"
  write_once "$BRAIN/$LANE/CLAUDE.md" <<EOF
# $LANE

The brainer for \`$REPO\`. Read-only there: plan, write briefs in
\`briefs/\`, dispatch with \`hw $LANE <task> --brief $LANE/briefs/<task>.md --sdd none\`,
and record what was decided in \`decisions.md\`.
EOF
  write_once "$BRAIN/$LANE/decisions.md" <<EOF
# $LANE — decisions

Append-only. Each entry: the ruling, what it rules out, what would reverse it.
EOF
fi

# projects.json, guards.json and every .claude/settings.json in one python
# pass, each written only when its content changes.
python3 - "$BRAIN" "$LANE" "$REPO" "$BASE" "$VENDOR" "$MODEL" "$OC_KIT" "$OPERATOR" "$MIN_MODEL" "$REQ_BY" <<'PY'
import json, os, sys
brain, lane, repo, base, vendor, model, oc_kit, operator, min_model, req_by = sys.argv[1:11]
work = os.path.join(brain, "work")

def load(path, default):
    try:
        with open(path) as f:
            return json.load(f)
    except FileNotFoundError:
        return default

def save(path, doc):
    text = json.dumps(doc, indent=2) + "\n"
    try:
        if open(path).read() == text:
            print("  ok    %s" % os.path.relpath(path, brain)); return
    except FileNotFoundError:
        pass
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    open(tmp, "w").write(text); os.replace(tmp, path)
    print("  +     %s" % os.path.relpath(path, brain))

pj_path, gj_path = os.path.join(brain, "projects.json"), os.path.join(brain, "guards.json")
pj = load(pj_path, None)
gj = load(gj_path, None)
if not lane:
    if pj is None:
        print("  --    no lane yet: projects.json and guards.json are written with the first --lane")
    sys.exit(0)

if pj is None:
    pj = {"comment": ["The lanes this brain dispatches. Written by install.sh; see hw --help."],
          "work": work, "lanes": {}}
entry = {
    "product_repo": True, "hw_aliases": [], "brain_aliases": [],
    "space": lane, "engram": lane, "vendor": vendor, "model": model, "account": "default",
    "checkout": repo, "base": base, "base_ref_prefix": "",
    "branch": "task/{task}",
    "worktree_root": "{work}/{lane}", "worktree": "{work}/{lane}/{task}",
    "build": "lanes/git-worktree.sh",
    "db": {"line": "none — this lane provisions no database", "no_db_inert": True},
}
if oc_kit:
    entry["opencode_config_dir"] = oc_kit
if min_model != "none":
    entry["model_floor"] = {"tier": min_model}
if req_by != "none":
    entry["requested_by"] = req_by
pj["lanes"].setdefault(lane, entry)
# Only when the table names no one: an operator already there is the user's.
if operator and "operator" not in pj:
    pj["operator"] = operator
save(pj_path, pj)

wt = os.path.join(work, lane)
if gj is None:
    gj = {"comment": ["What the read-only guards protect. Written by install.sh.",
                      "Absolute paths, so the policy does not follow $HOME."],
          "brain_root": brain, "product_repos": [],
          "defaults": {
              "git_tail": "The brainer is read-only there — only the task's executor changes repo state, in its own worktree. Read-only git (log/status/diff/show/rev-list) is allowed.",
              "redirect_tail": "If you were only reading and the '>' is part of a search pattern, keep the tree's absolute path out of the command. This check resolves the redirect's actual target before deciding, so respelling the path does not change the answer."},
          "lanes": {}}
if repo not in gj["product_repos"]:
    gj["product_repos"].append(repo)
if wt not in gj["product_repos"]:
    gj["product_repos"].append(wt)
gj["lanes"].setdefault("brain", {
    "repo": repo, "worktrees": wt, "write_here": brain + "/<lane>/",
    "where": "a product repo or a live worktree"})
gj["lanes"].setdefault(lane, {
    "repo": repo, "worktrees": wt, "write_here": "%s/%s/" % (brain, lane),
    "where": "the %s repo or a live %s worktree" % (lane, lane)})
save(gj_path, gj)

# Each brainer's .claude/settings.json: the Bash hook plus the deny half the
# hook cannot see (Edit/Write), over every protected root. Merged, never replaced.
roots = gj["product_repos"]
def merge_settings(dirpath):
    path = os.path.join(dirpath, ".claude", "settings.json")
    st = load(path, {})
    perm = st.setdefault("permissions", {})
    deny = perm.setdefault("deny", [])
    for r in roots:
        for rule in ("Edit(/%s/**)" % r, "Read(/%s/**/.env*)" % r):
            if rule not in deny:
                deny.append(rule)
    pre = st.setdefault("hooks", {}).setdefault("PreToolUse", [])
    # `|| exit 2`: a hook that cannot run at all (no python3 — exit 127) is a
    # NON-blocking error to Claude Code, i.e. an allow. 2 is its refusal.
    bare = "$CLAUDE_PROJECT_DIR/.claude/hooks/deny-repo-writes.py"
    cmd = bare + " || exit 2"
    for g in pre:
        for h in g.get("hooks", []):
            if h.get("command") == bare:
                h["command"] = cmd
    if not any(h.get("command") == cmd for g in pre for h in g.get("hooks", [])):
        pre.append({"matcher": "Bash", "hooks": [{"type": "command", "command": cmd}]})
    save(path, st)
for name in gj["lanes"]:
    merge_settings(brain if name == "brain" else os.path.join(brain, name))
PY

if [ -n "$LANE" ]; then
  for name in $(jq -r '.lanes | keys[]' "$BRAIN/guards.json"); do
    if [ "$name" = brain ]; then shim brain 2 | put_text "$BRAIN/.claude/hooks/deny-repo-writes.py"; chmod 755 "$BRAIN/.claude/hooks/deny-repo-writes.py"
    else shim "$name" 3 | put_text "$BRAIN/$name/.claude/hooks/deny-repo-writes.py"; chmod 755 "$BRAIN/$name/.claude/hooks/deny-repo-writes.py"; fi
  done
fi

# ── 4b. an OpenCode lane: the executor's own config directory ──────────────
# hw hands it to the executor as OPENCODE_CONFIG_DIR (projects.json's
# `opencode_config_dir`), which opencode loads like a project `.opencode/`.
# So the guard reaches the executor without a byte in ~/.config/opencode or in
# the worktree. Its policy is the executor's, not the brainer's: the lane's
# checkout is protected, the task's own worktree is not — the brainer's
# guards.json protects work/<lane> too, and would refuse every command an
# executor runs there. The shared module is COPIED into the kit because it
# finds its policy three directories above itself.
if [ -n "$OC_KIT" ]; then
  printf '\nopencode executor → %s\n' "${OC_KIT#"$BRAIN"/}"
  mkdir -p "$OC_KIT/setup/guards" "$OC_KIT/plugin"
  put "$SRC/setup/guards/deny-repo-writes.js" "$OC_KIT/setup/guards/deny-repo-writes.js"
  python3 - "$BRAIN" "$LANE" "$REPO" <<'PY' | put_text "$OC_KIT/guards.json"
import json, sys
brain, lane, repo = sys.argv[1:4]
tail = ("An executor writes only in its own worktree; the %s checkout is read-only to it. "
        "Read-only git (log/status/diff/show/rev-list) is allowed." % lane)
where = "the %s checkout (an executor writes only in its own worktree)" % lane
pane = {"repo": repo, "worktrees": repo, "write_here": "your task's own worktree", "where": where}
print(json.dumps({
    "comment": ["What the %s OPENCODE EXECUTOR's guard protects. Written by install.sh;"
                " the brainer's policy is guards.json at the brain root." % lane],
    "brain_root": brain, "product_repos": [repo],
    "defaults": {"git_tail": tail,
                 "redirect_tail": "If you were only reading and the '>' is part of a search pattern, keep the checkout's absolute path out of the command."},
    "lanes": {"brain": pane, lane: pane}}, indent=2))
PY
  cat <<EOF | put_text "$OC_KIT/plugin/deny-repo-writes.js"
// The '$LANE' opencode EXECUTOR's guard. Generated by install.sh; the logic is
// ../setup/guards/deny-repo-writes.js and the policy ../guards.json.
// The import is dynamic and guarded: a module that does not load becomes a
// throw on every bash call — opencode's one refusal — never a missing guard.
const LANE = "$LANE";
const SHARED = "../setup/guards/deny-repo-writes.js";

export const DenyRepoWrites = async (pluginInput) => {
  try {
    const { makeDenyRepoWrites } = await import(SHARED);
    return await makeDenyRepoWrites(LANE)(pluginInput);
  } catch (error) {
    const why = String(error?.message ?? error);
    return {
      "tool.execute.before": async (input) => {
        if (input.tool !== "bash") return;
        throw new Error(
          "Blocked: the $LANE executor guard could not load its shared module (" +
          SHARED + "): " + why + ". Every bash command is refused until it loads.");
      },
    };
  }
};
EOF
fi

# ── 5. the user's side: PATH links and one Claude Code Stop hook ────────────
printf '\nuser config\n'
mkdir -p "$BIN_OUT"
for l in "${LINKS[@]}"; do
  if [ -L "$BIN_OUT/$l" ]; then ok "$BIN_OUT/$l"
  else ln -s "$BRAIN/bin/$l" "$BIN_OUT/$l"; chg "$BIN_OUT/$l → $BRAIN/bin/$l"; fi
done
case ":$PATH:" in *":$BIN_OUT:"*) ;; *) say "WARN  $BIN_OUT is not on your PATH — add it, then restart herdr so its panes inherit it" ;; esac

if [ "$(jq -r '.stop_present' "$PLAN")" = true ]; then
  ok "$CLAUDE_CFG/settings.json already runs this brain's Stop hook"
else
  mkdir -p "$CLAUDE_CFG"
  [ ! -f "$CLAUDE_CFG/settings.json" ] || cp -p "$CLAUDE_CFG/settings.json" "$CLAUDE_CFG/settings.json.bak-brain-install"
  python3 - "$CLAUDE_CFG/settings.json" "$STOP_CMD" <<'PY'
import json, os, sys
path, cmd = sys.argv[1:3]
try:
    st = json.load(open(path))
except FileNotFoundError:
    st = {}
st.setdefault("hooks", {}).setdefault("Stop", []).append(
    {"hooks": [{"type": "command", "command": cmd}]})
tmp = path + ".tmp"
open(tmp, "w").write(json.dumps(st, indent=2) + "\n"); os.replace(tmp, path)
PY
  chg "$CLAUDE_CFG/settings.json — Stop hook merged (previous file kept as settings.json.bak-brain-install)"
fi

printf '\ninstalled: %s\n' "$BRAIN"
if [ -n "$FIRST_RUN" ]; then
  printf '\nfirst, yours — Claude Code has not finished its first run in this account (%s):\n' "$(printf '%s' "$FIRST_RUN" | tr '\n' ' ' | sed 's/ $//')"
  printf '  %s   # finish the welcome and login, accept the Bypass Permissions warning, /exit\n' "$(claude_first_run_command "${CLAUDE_CONFIG_DIR:-}")"
fi
[ "$ENGRAM_WIRED" != no ] || printf '\nand for memory: engram setup claude-code\n'
if [ "$VENDOR" = opencode ]; then
  printf '\nand for the %s executors: opencode signs into its own provider, and that login is yours\n' "$LANE"
  printf '  (opencode auth login, or a --model that needs none). This installer never reads it.\n'
fi
if [ -n "$LANE" ]; then
  cat <<EOF

next:
  brain $LANE                                     # open the $LANE brainer
  \$EDITOR $BRAIN/$LANE/briefs/<task>.md
  hw $LANE <task> --brief $BRAIN/$LANE/briefs/<task>.md --sdd none --dry-run
EOF
else
  printf '\nnext: ./install.sh --brain %s --lane <name> --repo <path>\n' "$BRAIN"
fi
