#
# invoker-common.sh — the out-of-band signalling shared by ask-invoker and
# done-invoker. SOURCED, never executed: no shebang, not executable, and
# deliberately not symlinked into ~/bin.
#
# WHY THIS FILE EXISTS
#
# The record and the transport are separate. Every invoker first publishes
# metadata on its own pane. Delivery then follows a typed receiver address:
# OpenCode same-vendor proves exact-message persistence while busy and exact
# assistant completion while settled. Codex has NO native transport an invoker
# can address (`invoker_vendor_has_native_transport`, and setup/decisions.md
# "Codex es un vendor sin transporte nativo"), so it stays on Herdr's idle gate,
# as do mixed vendors. Claude 2.1.240 also stays on the gate:
# live peer messages were preserved but did not auto-open a turn. Accounting
# happens only after the route's receiver-owned delivery proof succeeds.
#
# WHAT WE CANNOT DO, so nobody proposes it again
#
# Turning the executor's sidebar row blocked via `pane.report_agent --state
# blocked` was proposed and tested. **That is unreachable.**
# `pane.report_agent` is inert on any pane where herdr's screen detection is
# already classifying a recognised claude agent: it returns `{"type":"ok"}` and
# moves nothing (measured over repeated polls, state never budged, in both
# directions). A follow-up probe went further and tried `pane.clear_agent_authority`
# alone, then with `report_agent` in both orders, with and without a matching
# `seq`, and `pane.release_agent` alone: every call returned ok, and with a live
# subscription to `pane.agent_status_changed` attached, **zero events fired and
# `state_change_seq` never moved**. These writes are rejected before any write
# happens, not applied-then-overwritten. There is no version of this that works
# in herdr 0.8.2, and `state_labels` is indexed BY state, so the custom sidebar
# string is unreachable by the same route.
#
# The consequence for the tokens below: they are the ONLY thing that tells
# the operator what an executor is waiting on, because the sidebar will not
# change colour for them. So they carry the question, the counter, the task and the
# delivery state in full — `herdr agent list` alone must be enough.
#
# Callers must set, before sourcing:
#   INVOKER_PROG      name used in error messages ("ask-invoker")
#   INVOKER_BIN_DIR   the real brain/bin, resolved through the ~/bin symlink
# and must define `die`.
#

# herdr-rpc is NOT on PATH — only ask-invoker, done-invoker, hw, brain and
# lint-shell are symlinked into ~/bin. It lives beside the real script.
INVOKER_RPC="$INVOKER_BIN_DIR/herdr-rpc"

# Reported as the `source` of every metadata write, so a human reading
# `agent list` knows which script wrote the tokens.
INVOKER_SOURCE="hw:$INVOKER_PROG"
INVOKER_CHANNEL_SEND="$INVOKER_BIN_DIR/channel-send"

# ── the wait budget ───────────────────────────────────────────────────────
#
# HOW THIS NUMBER WAS CHOSEN, because it is the one real design decision here.
#
# The budget is irrelevant when the brainer is idle: `wait-agent` is
# level-triggered and returns in ~0.14s. It only bites when the brainer is
# mid-turn, and then it is bounded from ABOVE by something outside herdr: the
# executor is a Claude Code agent calling us through its Bash tool, whose
# default timeout is 120s. If we block longer than that, the harness kills
# ask-invoker mid-wait — and then the question is neither delivered nor
# reported, which is the exact failure we are here to remove. 90s leaves ~30s
# of headroom for the publish, the prompt and the executor's own overhead.
#
# A brainer turn often runs longer than 90s, so TIMING OUT IS A NORMAL OUTCOME,
# not an error path bolted on. It is handled as a first-class result: the
# question is already published in herdr, the attempt is not consumed, and the
# executor is told to retry. An executor that knows it can afford to block
# longer raises the budget itself:
#
#   HW_INVOKER_WAIT_MS=540000 ask-invoker "…"     # with a 600s Bash timeout
#
INVOKER_WAIT_MS="${HW_INVOKER_WAIT_MS:-90000}"
case "$INVOKER_WAIT_MS" in
  ''|*[!0-9]*) INVOKER_WAIT_MS=90000 ;;
esac

# ── publishing ────────────────────────────────────────────────────────────

# HERDR TRUNCATES EVERY TOKEN VALUE AT 80 CHARACTERS. Measured 2026-08-20 on
# 0.8.2, and it is silent — `report_metadata` returns `{"type":"ok"}` and
# `agent list` hands back exactly 80. It is 80 CODEPOINTS, not bytes: 100 "é"
# came back as 80 chars / 160 bytes, so a chunk boundary at 80 characters never
# splits a character. The schema documents the 16-key cap and the key-name
# pattern and says nothing about this.
#
# That matters more here than it looks: these tokens are the ONLY record of what
# an executor is waiting on, and a 600-char question stored as one token would
# have lost 520 characters of itself without a word of complaint. So text goes
# in as numbered chunks — `ask`, `ask2`, … — and the caller budgets how many.
INVOKER_TOKEN_CHARS=80

# A token whose value is JSON null is DELETED by herdr. That is the only way to
# take a key away, and it is load-bearing here — see the padding below.
INVOKER_NULL=$'\001'

# invoker_chunk_tokens <prefix> <max_chunks> <text>
#
# Emits NUL-terminated `name=chunk` records for invoker_tokens_json. First chunk
# keeps the bare prefix so the common short case reads as one plain token; the
# rest are prefix2, prefix3, … which also happens to sort correctly in
# `agent list`. If the text needs more than <max_chunks>, the last chunk ends in
# an ellipsis — a reader must be able to tell a complete record from a clipped
# one, and that costs no extra key out of the 16.
#
# EVERY SLOT IS ALWAYS EMITTED, and the unused ones carry the delete sentinel.
# This is not tidiness. `report_metadata` MERGES into whatever tokens the pane
# already has: a 600-char question followed by a 40-char one left ask3…ask8 from
# the first question sitting behind the second, so anyone reconstructing the
# record read one question with five paragraphs of another glued to the end.
# Observed on the probe pane before this was added.
#
# python3, not bash: this is character-indexed slicing of arbitrary text, and
# python3 is already a hard dependency of this design (herdr-rpc is python).
invoker_chunk_tokens() {
  python3 - "$1" "$2" "$3" "$INVOKER_TOKEN_CHARS" "$INVOKER_NULL" <<'CHUNKER'
import sys

prefix, max_chunks, text, width, sentinel = (
    sys.argv[1], int(sys.argv[2]), sys.argv[3], int(sys.argv[4]), sys.argv[5])

def split(text, width):
    """Fixed-width slices, but preferring a whitespace boundary.

    LOSSLESS BY CONSTRUCTION: a reader reconstructs the value by concatenating
    the chunks in order, so the break may move but no character may be dropped
    — the whitespace stays at the end of the chunk it ended. A fixed slice cut
    mid-word, which is only cosmetic in `herdr agent list` but made the one
    surface a human actually reads harder to read than the text it carries.

    A word longer than `width` has no boundary to find, so it falls back to the
    hard cut. That is the correct outcome, not a limitation to work around.
    """
    out = []
    rest = text
    while len(rest) > width:
        head = rest[:width]
        cut = max(head.rfind(" "), head.rfind("\n"), head.rfind("\t"))
        if cut <= 0:
            cut = width          # one long token: cut it, there is nothing else
        else:
            cut += 1             # keep the separator on this chunk
        out.append(rest[:cut])
        rest = rest[cut:]
    out.append(rest)
    return out


chunks = split(text, width) if text else [""]
if len(chunks) > max_chunks:
    chunks = chunks[:max_chunks]
    chunks[-1] = chunks[-1][:width - 1] + "\u2026"
chunks += [sentinel] * (max_chunks - len(chunks))
for i, chunk in enumerate(chunks, 1):
    name = prefix if i == 1 else "%s%d" % (prefix, i)
    sys.stdout.write("%s=%s\0" % (name, chunk))
CHUNKER
}


# invoker_tokens_json k=v [k=v ...]  ->  a JSON object on stdout.
#
# Split on the FIRST `=` only, so a value may contain `=`, quotes, newlines and
# anything else a question can contain. jq does the escaping; building this
# JSON with printf is how you ship a script that breaks on the first apostrophe.
# Keys must match herdr's ^[A-Za-z0-9_-]{1,32}$ and there is a hard cap of 16 —
# both are the caller's job, and both are checked here rather than discovered as
# an opaque RPC error.
invoker_tokens_json() {
  [ $# -le 16 ] || die "internal: $# metadata tokens, herdr's limit is 16"
  local pair name
  for pair in "$@"; do
    name="${pair%%=*}"
    case "$name" in
      "$pair") die "internal: metadata token '$pair' has no '=' separator" ;;
    esac
    case "$name" in
      *[!A-Za-z0-9_-]*|'') die "internal: metadata token name '$name' is not ^[A-Za-z0-9_-]{1,32}$" ;;
    esac
    [ "${#name}" -le 32 ] || die "internal: metadata token name '$name' is over 32 chars"
  done
  # The $INVOKER_NULL sentinel becomes a real JSON null, which is how a token is
  # deleted rather than set to an empty string.
  jq -nc --arg nul "$INVOKER_NULL" --args '
    [ $ARGS.positional[]
      | (index("=")) as $i
      | (.[$i+1:]) as $v
      | { (.[:$i]): (if $v == $nul then null else $v end) } ]
    | add // {}
  ' "$@"
}

# invoker_cockpit_kick — tell the brainer's cockpit something changed (see
# bin/cockpit-state --kick: debounced, returns at once, never fails). Called after every
# publication an invoker makes, so the panel shows an envelope within a second instead of
# at the next 5 s heartbeat.
invoker_cockpit_kick() {
  [ -n "${HW_INVOKER_PANE:-}" ] || return 0
  "$INVOKER_BIN_DIR/cockpit-state" --kick --invoker "$HW_INVOKER_PANE" >/dev/null 2>&1 || true  # MUTATION-ANCHOR: 798-M01
}

# invoker_publish <ttl_ms> <tokens_json> [<state_labels_json>|clear]
#
# Writes the tokens onto the EXECUTOR's own pane ($HERDR_PANE_ID) — not the
# brainer's. The executor is the thing whose state changed, and this is the one
# pane we are certainly allowed to write to.
#
# Returns 0 only when agent.list exposes the requested token values/deletions
# on this exact pane. Write transport errors retain their status; an unproven
# read-back is 4 (degraded, not proof that the record did not land).
#
# `state_labels` is written when given even though we cannot reach
# the `blocked` state to display it — a label under a state the pane genuinely
# does enter (working/idle) costs one key and may render; nothing depends on it.
# `clear` removes labels a previous call left behind.
invoker_publish() {
  local ttl="$1" tokens="$2" labels="${3:-}" params observed rc=0
  if ! params="$(
    jq -nc \
      --arg pane "$HERDR_PANE_ID" \
      --arg src "$INVOKER_SOURCE" \
      --argjson ttl "$ttl" \
      --argjson tokens "$tokens" \
      --arg labels "$labels" \
      '{ pane_id: $pane, source: $src, ttl_ms: $ttl, tokens: $tokens }
       + ( if   $labels == ""      then {}
           elif $labels == "clear" then { clear_state_labels: true }
           else { state_labels: ($labels | fromjson) } end )'
  )"; then
    die "internal: could not build pane.report_metadata params"
  fi
  "$INVOKER_RPC" call pane.report_metadata "$params" >/dev/null 2>&1 || rc=$?
  [ "$rc" = 0 ] || return "$rc"
  observed="$("$INVOKER_RPC" call agent.list '{}' 2>/dev/null)" || {
    printf 'invoker_publish: token publication UNPROVEN: agent.list read-back failed\n' >&2
    return 4
  }
  if ! printf '%s' "$observed" | jq -e --arg pane "$HERDR_PANE_ID" --argjson expected "$tokens" '
      .error == null and (.agents | type) == "array" and
      (($expected | type) == "object") and ($expected | length) > 0 and
      ([.agents[] | select(.pane_id == $pane)] as $matches |
       ($matches | length) == 1 and ($matches[0].tokens | type) == "object" and
       ($matches[0].tokens as $actual | all($expected | to_entries[]; . as $item |
         if $item.value == null then ($actual | has($item.key)) | not
         else $actual[$item.key] == $item.value end)))' >/dev/null 2>&1; then
    printf 'invoker_publish: token publication UNPROVEN: requested state is not readable on the exact pane\n' >&2
    return 4
  fi
  return 0
}

# Describe the observation already made, never a later persistence guarantee.
invoker_publication_note() {
  if [ "$1" = 0 ]; then
    printf 'Metadata was verified by read-back before the delivery attempt.'
  else
    printf 'Metadata publication could not be established; do not rely on a token record on this pane.'
  fi
}

# ── the cwd-derived fallback ───────────────────────────────────────────────
#
# WHY THIS EXISTS. Until 2026-08-20 both invokers refused outright without
# HW_INVOKER_PANE in the environment, and after a herdr restart that is exactly
# the state a resumed executor is in: herdr restores the pane, the cwd, the agent
# and the conversation, and re-injects only `HERDR_*` — every `HW_*` is gone
# (verified with `ps -Eww` on both resumed processes: zero hits). It does not replay
# the launch command either, so there is no argv seam to rebuild them through.
#
# So the environment is now the FIRST source, not the only one. The second is
# the file `hw` writes for the run: `<workdir>/.hw/<run>/env`. It is found by
# walking UP from $PWD, because the cwd herdr persists is the pane's LIVE cwd,
# not its launch cwd — a `cd sub/deeper` was persisted verbatim in the probe.
#
# WHAT THE FILE CANNOT FIX, so nobody expects more of it than it gives:
# HW_INVOKER_PANE is a pane ID. Pane ids do survive a restart, which is why the
# file records the id and not a name — `agent_name` is NOT persisted for a
# brainer-shaped pane (post-restart `name: null`), so a name would restore as
# nothing. But a brainer that was closed and recreated has a DIFFERENT id, and
# no file can know that. The wait then fails, and herdr answers "no such pane"
# by closing the connection, which herdr-rpc reports as exit 3 — the same exit
# as "herdr is gone". The messages say both, and claim nothing more.
#
# THE ROOTS ARE WIDER HERE THAN IN THE SHELL SNIPPET, deliberately.
# brain/shell/hw-restore-env.zsh runs in every shell the operator opens and feeds
# `typeset -gx`, so it is confined to the checkout's own work root. This runs only when an
# executor deliberately calls an invoker, and it PARSES six known keys instead
# of applying a whole file, so it also covers the two worktree roots where the
# repo-backed lanes actually put their executors. Same ownership and permission
# refusals in both.
# One root per repo, inside the repo. A legacy shared worktree root outside the
# repos was retired once its last holdout migrated once its pane freed — the
# export it carries was hash-compared before and after the move. That root is
# gone from disk and from here, along with its copies in hw's sweep and
# hw-restore-env.zsh. Three doors, one rule.
#
# DERIVED FROM bin/project-spaces.sh's lane_work_bases(), not hand-copied.
# Until 2026-09-15 this was a fourth literal list — the same three roots typed
# out here, in hw-restore-env.zsh, and (for a different purpose) in bin/hw's
# `_next_autoclosed_run_for_pane`. None of the four knew the newer lanes had
# landed: an executor's `done-invoker` walked up from
# ~/projects/<lane>/.worktrees/<task>, matched no root, and died with
# `no HW_INVOKER_PANE` — reproduced against this exact literal before the fix
# (setup/briefs/el-canal-alcanza-cada-worktree.md). `all_lane_worktree_roots`
# is the one place that enumerates every project `project_space_label()` knows
# and unions their `lane_work_bases()`, so a new lane with a repo reaches the
# invoker channel the moment it gets one `lane_work_bases()` arm — no second
# edit here.
#
# Falls back to a direct read of the same table on ANY failure to source or
# derive: a broken derivation must not take the invoker channel down with it,
# and this file is sourced from a bare `done-invoker`/`ask-invoker` process
# with no guarantee the caller already loaded project-spaces.sh or that jq is
# on PATH. The fallback is python3, reads only `work` and each lane's
# `worktree_root`, expands `~` from HOME and the `{work}`/`{lane}`/`{checkout}`
# placeholders, and skips anything it cannot resolve. No table at all leaves
# $WORK, if the caller set one, or nothing: the walk then finds no file and the
# invoker's own checks decide.
_invoker_roots_from_table() {
  python3 - "${HW_PROJECTS_JSON:-${HW_BRAIN_ROOT:-$(dirname "$INVOKER_BIN_DIR")}/projects.json}" "${WORK:-}" <<'ROOTS'
import json, os, sys
path, work = sys.argv[1], sys.argv[2]
try:
    doc = json.load(open(path, encoding="utf-8"))
except FileNotFoundError:
    doc = {}
except (OSError, ValueError) as e:
    sys.stderr.write("invoker-common: the lane table %s could not be read (%s); env roots are only the work directory\n" % (path, e))
    doc = {}
if not isinstance(doc, dict):
    doc = {}
home = os.environ.get("HOME", "")
def exp(p):
    return home + p[1:] if p == "~" or p.startswith("~/") else p
if not work and isinstance(doc.get("work"), str):
    work = exp(doc["work"])
roots = [work] if work else []
lanes = doc.get("lanes") if isinstance(doc.get("lanes"), dict) else {}
for lane, v in lanes.items():
    wr = v.get("worktree_root") if isinstance(v, dict) else None
    if not isinstance(wr, str) or not wr:
        continue
    wr = exp(wr).replace("{work}", work).replace("{lane}", lane)
    if "{checkout}" in wr:
        co = v.get("checkout")
        if not isinstance(co, str) or not co:
            continue
        wr = wr.replace("{checkout}", exp(co))
    if "{" in wr or (work and (wr == work or wr.startswith(work + "/"))):
        continue
    if wr not in roots:
        roots.append(wr)
print("\n".join(roots))
ROOTS
}
INVOKER_ENV_ROOTS=""
if [ -r "$INVOKER_BIN_DIR/project-spaces.sh" ]; then
  # shellcheck source=project-spaces.sh
  # Named, not silent: a library that does not load used to leave the roots
  # empty with no trace. The fallback below still runs, so the channel lives.
  . "$INVOKER_BIN_DIR/project-spaces.sh" \
    || printf 'invoker-common: %s/project-spaces.sh did not load (exit %s); the lane roots fall back to reading the lane table directly\n' "$INVOKER_BIN_DIR" "$?" >&2
  # The lane table is brain/projects.json, one directory above this script's
  # own (or HW_BRAIN_ROOT). If it does not load, the fallback below reads it.
  if command -v lane_config_load >/dev/null 2>&1 \
     && lane_config_load "${HW_BRAIN_ROOT:-$(dirname "$INVOKER_BIN_DIR")}" 2>/dev/null \
     && command -v all_lane_worktree_roots >/dev/null 2>&1; then
    _derived_roots="$(all_lane_worktree_roots 2>/dev/null || true)"
    if [ -n "$_derived_roots" ]; then
      INVOKER_ENV_ROOTS="$WORK
$_derived_roots"
    fi
    unset _derived_roots
  fi
fi
# Only when the derivation gave nothing: a python3 start on every invoker call
# is time the invoker suites pay hundreds of times over.
if [ -z "$INVOKER_ENV_ROOTS" ]; then
  INVOKER_ENV_ROOTS="$(_invoker_roots_from_table)
"
fi

# Set by invoker_adopt_env_file: the file it read, and whether HW_INVOKER_PANE
# specifically came from it (which is what makes a stale pane id plausible).
INVOKER_ENV_FILE=""
INVOKER_ENV_PANE_FROM_FILE=0

# invoker_adopt_env_file
#
# Fills in ONLY the variables that are currently unset or empty. A live pane
# launched with `--env` must win over a file on disk: the file is what a pane
# that lost its environment falls back to, never an override for one that still
# has it. Always returns 0 — an absent or rejected file is not an error here,
# it just means the caller's own checks decide, exactly as before.
# BASH 3.2 RE-PARSES the ENVFIND body below as part of the <( ... ), heredoc or
# not: an odd count of apostrophes ends it early (setup/tests/23 asserts the
# count), and a comprehension like {k: v for k, v in x} is mangled by brace
# expansion into a Python SyntaxError. Plain loops, no braces with commas.
invoker_adopt_env_file() {
  local rec key val found=""
  # python3, not shell: this walks a bounded path, stats for ownership and mode,
  # and unquotes a fixed grammar. `|| true` because "no file" is the common case
  # and a non-zero exit here must not take the whole invoker down under set -e.
  while IFS= read -r -d '' rec; do
    key="${rec%%=*}"
    val="${rec#*=}"
    if [ "$key" = "__FILE__" ]; then found="$val"; continue; fi
    case "$key" in
      HW_PROJECT|HW_TASK|HW_WORKDIR|HW_RUN|HW_INVOKER_PANE|ENGRAM_PROJECT|\
      HW_ARTIFACTS|HW_EXECUTOR_VENDOR|HW_INVOKER_VENDOR|HW_INVOKER_SESSION|HW_INVOKER_ENDPOINT|\
      HW_CHAINING_ENABLED) ;;
      HW_CHAINING_LEASE_SECONDS|HW_CHAINING_LEASE_REASON|HW_CHAINING_LEASE_HOLDER) ;;
      *) continue ;;
    esac
    # Indirect expansion, so no eval touches the file's contents. The assignment
    # below evals only a whitelisted NAME plus a quoted variable reference.
    [ -n "${!key:-}" ] && continue
    eval "$key=\$val"
    export "$key"
    [ "$key" = HW_INVOKER_PANE ] && INVOKER_ENV_PANE_FROM_FILE=1
  done < <(INVOKER_ENV_ROOTS="$INVOKER_ENV_ROOTS" HW_ENV_ROOTED_AT="${HW_WORKDIR:-}" HW_ENV_ROOTED_RUN="${HW_RUN:-}" INVOKER_RUNENV="$INVOKER_BIN_DIR/runenv" python3 - <<'ENVFIND'
import importlib.machinery, importlib.util, os, stat, sys

sys.dont_write_bytecode = True

WANTED = ("HW_PROJECT", "HW_TASK", "HW_WORKDIR", "HW_RUN",
          "HW_ARTIFACTS", "HW_INVOKER_PANE", "ENGRAM_PROJECT", "HW_EXECUTOR_VENDOR",
          "HW_INVOKER_VENDOR", "HW_INVOKER_SESSION", "HW_INVOKER_ENDPOINT",
          "HW_CHAINING_ENABLED") + (
          "HW_CHAINING_LEASE_SECONDS", "HW_CHAINING_LEASE_REASON",
          "HW_CHAINING_LEASE_HOLDER")
MAX_HOPS = 12

roots = [r for r in os.environ.get("INVOKER_ENV_ROOTS", "").split("\n") if r]


def canon(p):
    """Canonical form for comparison.

    os.path.realpath() resolves symlinks but does NOT fix the case, while
    os.getcwd() DOES return the true on-disk case — so comparing one against the
    other never matches on a case-insensitive filesystem. That asymmetry killed
    a worktree root outright. Casefolding both sides is what
    makes a future case drift a non-event instead of a silent dead root.
    """
    return os.path.realpath(p).rstrip("/").casefold()


def under_a_root(path):
    p = canon(path)
    for root in roots:
        r = canon(root)
        if p == r or p.startswith(r + "/"):
            return True
    return False


def usable(path):
    """Owned by us, a regular file, and not group- or world-writable."""
    try:
        st = os.lstat(path)
    except OSError:
        return False
    if not stat.S_ISREG(st.st_mode):
        return False
    if os.name == "nt":
        # No uid and no mode bits on Windows (chmod 600 changes nothing): the
        # file sits in the tree of the user, and NTFS ACLs decide who can write.
        return True
    if st.st_uid != os.getuid():
        return False
    return not (st.st_mode & 0o022)


def candidates(directory):
    hw = os.path.join(directory, ".hw")
    try:
        runs = os.listdir(hw)
    except OSError:
        return []
    found = []
    for run in runs:
        path = os.path.join(hw, run, "env")
        if usable(path):
            try:
                found.append((os.stat(path).st_mtime, path))
            except OSError:
                pass
    # Newest run first: a work directory is reused across runs by `hw done`.
    return [p for _, p in sorted(found, reverse=True)]


def load_runenv():
    """bin/runenv is the one reader of a run env file; loaded, never copied."""
    path = os.environ.get("INVOKER_RUNENV", "")
    try:
        loader = importlib.machinery.SourceFileLoader("runenv", path)
        mod = importlib.util.module_from_spec(importlib.util.spec_from_loader("runenv", loader))
        loader.exec_module(mod)
    except Exception as e:
        sys.stderr.write("invoker-common: cannot load %s (%s), so no run env file can be read\n" % (path, e))
        sys.exit(0)
    return mod


runenv = load_runenv()


def parse(path):
    """The WANTED keys of one run env file; a file runenv refuses is named, not skipped in silence."""
    out = {}
    try:
        values = runenv.read(path, lenient=True)
    except runenv.Unusable as e:
        sys.stderr.write("invoker-common: %s\n" % e)
        return out
    for key in WANTED:
        if values is not None and key in values:
            out[key] = values[key]
    return out


def emit(path, values):
    sys.stdout.write("__FILE__=%s\0" % path)
    for key in WANTED:
        if key in values:
            sys.stdout.write("%s=%s\0" % (key, values[key]))
    sys.exit(0)


# ROOTED AT $HW_WORKDIR WHEN THERE IS ONE, AND AT $PWD ONLY WHEN THERE IS NOT.
#
# The walk up from $PWD is what makes a RESUMED executor work at all: herdr
# re-injects only HERDR_*, so HW_WORKDIR is gone with everything else and the
# cwd is the only thing left to search from. That path is untouched below.
#
# But a pane that STILL HAS HW_WORKDIR is not searching, it is being told, and
# the cwd is then a worse signal than the variable. Measured directly: an
# observed hijack, and one shape worse than it. Both with
# `env -u HW_INVOKER_PANE` and HW_WORKDIR still set:
#
#   cwd INSIDE the work directory -> adopted the run OWN HW_INVOKER_PANE, and
#     done-invoker delivered a real completion report for setup:probe-A, a task
#     that does not exist, to w4C:pT, exit 0.
#   cwd in a SIBLING directory under the same lane -> climbed OUT of it and
#     adopted A DIFFERENT TASK run file. Same hijack, blast radius of every
#     work directory under a root.
#
# Only a cwd outside every root refused, which is why the incident was found by
# cd-ing away and not by anything in the code.
#
# So when HW_WORKDIR is set the walk collapses to its root: exactly that
# directory, and when HW_RUN is set too, exactly that run file. `hw` writes the
# file at $HW_WORKDIR/.hw/$HW_RUN/env and exports both, so this is the address
# of the file rather than a search for something that resembles it. Nothing is
# adopted when that address is empty, and REFUSING IS THE POINT: the gate in
# both invokers then says "no HW_INVOKER_PANE" instead of reporting a task to a
# brainer that never dispatched it.
#
# THE ROOT CHECK STILL APPLIES to HW_WORKDIR itself. A variable is not a licence
# to read a file anywhere, and a stale HW_WORKDIR pointing outside the roots must
# refuse rather than fall back to the walk. Falling back is how the variable
# would stop meaning anything.
#
# PASSED EXPLICITLY, not read from the ambient environment, so a value that is
# set but not exported cannot silently send this down the cwd path instead.
rooted = os.environ.get("HW_ENV_ROOTED_AT", "")
if rooted:
    if under_a_root(rooted):
        run = os.environ.get("HW_ENV_ROOTED_RUN", "")
        if run:
            paths = [p for p in [os.path.join(rooted, ".hw", run, "env")] if usable(p)]
        else:
            # No run named: the work directory is still authoritative, and
            # candidates() already prefers the newest run inside it.
            paths = candidates(rooted)
        for path in paths:
            values = parse(path)
            if values:
                emit(path, values)
    sys.exit(0)

directory = os.getcwd()
for _ in range(MAX_HOPS):
    if not under_a_root(directory):
        break
    for path in candidates(directory):
        values = parse(path)
        if values:
            emit(path, values)
    parent = os.path.dirname(directory)
    if parent == directory:
        break
    directory = parent
ENVFIND
  )
  INVOKER_ENV_FILE="$found"
  return 0
}

# A sentence for the messages that name a pane id, added only when that id came
# out of the env file rather than the environment. It is the honest limit of
# this whole mechanism, so it is said where it matters and nowhere else.
invoker_pane_source_note() {
  [ "$INVOKER_ENV_PANE_FROM_FILE" = 1 ] || return 0
  printf ' That pane id came from %s, which was written before this pane lost its environment: pane ids do survive a herdr restart, but a brainer pane that was closed and recreated has a different id and no file can know that. herdr answers "no such pane" by closing the connection, so it is indistinguishable from "herdr is gone" — this cannot be narrowed further from here.' \
    "$INVOKER_ENV_FILE"
}

# ── per-task state inside one run ─────────────────────────────────────────
#
# A run is an EXECUTOR, not a task. The brainer's normal loop is: dispatch,
# executor reports done, dispatch again to the same living session — respawning
# a whole space to hand over the next task throws away the context that made the
# session worth keeping. But every budget in this design (the ask cap, the
# once-only done marker) was keyed on the run directory, so the second task on a
# living executor could never report: done-invoker refused it as a duplicate.
#
# The task number lives in a FILE, not the environment. The executor's HW_RUN is
# fixed in its process environment at launch and no later write can change it —
# invoker_adopt_env_file only fills variables that are unset. A file is read on
# every invocation, so `hw next` can advance the counter under a running agent.
#
# Task 1 keeps the run directory itself. That is deliberate: every live run
# predates this file, and a layout change that needed migrating would strand
# them. `t2/`, `t3/` … are additions, not a new scheme.
invoker_run_dir() {
  printf '%s/.hw/%s' "${HW_WORKDIR:-${TMPDIR:-/tmp}}" "${HW_RUN:-norun}"
}

invoker_task_seq() {
  local n="" f
  f="$(invoker_run_dir)/task"
  # `[ -r ]` first: the redirect is the SHELL's, so `2>/dev/null` on tr never
  # sees it and every task-1 invocation printed "No such file or directory".
  # Task 1 has no counter file by design, so that is the common case.
  [ -r "$f" ] && n="$(tr -dc '0-9' < "$f" 2>/dev/null || true)"
  case "$n" in ''|0) n=1 ;; esac
  printf '%s' "$n"
}

# ── the outbox: a report is written down BEFORE it is sent ───────────────────
#
# done-invoker used to hold the report only in memory and in herdr tokens (24h
# TTL, capped at 560 characters). With the brainer's pane closed or renamed the
# send died, the only full copy was the executor's own transcript, and nothing
# would ever offer it to the brainer again. Now the full envelope is a file
# under `<run state dir>/outbox/`, keyed by ENVELOPE ID, and it is removed only
# once the receiver admitted it. `hw outbox` lists it; `hw outbox flush` (run by
# `brain <lane>` on open) redelivers it.
#
# DEDUPLICATION IS BY ENVELOPE ID, ON THE SENDER'S SIDE: one file per id, so a
# retried done-invoker overwrites rather than adds, and a delivered id has no
# file left to redeliver. An id that ended `uncertain` (channel-send exit 5: it
# may already be in the receiver) is NEVER auto-redelivered — that is the one
# case where a redelivery is a duplicate. The redelivered text carries the same
# envelope id, which is how a receiver can recognise a repeat.
invoker_outbox_dir() { printf '%s/outbox' "$(invoker_state_dir)"; }
invoker_outbox_key() { printf '%s' "$1" | tr -c 'A-Za-z0-9._-' '_'; }
invoker_outbox_write_meta() {  # $1=envelope $2=state $3=status(optional)
  local d key status="${3:-}"
  d="$(invoker_outbox_dir)"; key="$(invoker_outbox_key "$1")"
  [ -n "$status" ] || status="$(sed -n 's/^status=//p' "$d/$key.meta" 2>/dev/null | head -1)"
  printf 'envelope=%s\nproject=%s\ntask=%s\nrun=%s\nstatus=%s\nstate=%s\nat=%s\npid=%s\n' \
    "$1" "${HW_PROJECT:-}" "${HW_TASK:-}" "${HW_RUN:-norun}" "$status" "$2" \
    "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$$" > "$d/$key.meta"
}
invoker_outbox_put() {  # $1=envelope $2=message $3=status (done|blocked)
  local d key
  local state=pending
  d="$(invoker_outbox_dir)"; key="$(invoker_outbox_key "$1")"
  mkdir -p "$d" 2>/dev/null || return 1
  # A re-run after an exit 5 must not launder `uncertain` back into `pending`:
  # that is the one state a redelivery would duplicate.
  ! grep -q '^state=uncertain' "$d/$key.meta" 2>/dev/null || state=uncertain
  printf '%s' "$2" > "$d/$key.msg.tmp" 2>/dev/null && mv "$d/$key.msg.tmp" "$d/$key.msg" || return 1
  invoker_outbox_write_meta "$1" "$state" "$3" || return 1
}
invoker_outbox_settle() {  # $1=envelope — admitted: nothing left to redeliver
  local d key
  d="$(invoker_outbox_dir)"; key="$(invoker_outbox_key "$1")"
  rm -f "$d/$key.msg" "$d/$key.meta" 2>/dev/null || true
}
# channel-send exit 5: the message MAY be in the receiver. Say so on disk.
invoker_delivery_uncertain() {  # $1=envelope $2=route
  local d key
  d="$(invoker_outbox_dir)"; key="$(invoker_outbox_key "$1")"
  printf 'envelope=%s\nroute=%s\nat=%s\nchannel_send_exit=5\n' "$1" "$2" \
    "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" > "$(invoker_state_dir)/delivery-uncertain" 2>/dev/null || true
  [ ! -f "$d/$key.meta" ] || invoker_outbox_write_meta "$1" uncertain || true
}

invoker_state_dir() {
  local run_dir seq
  run_dir="$(invoker_run_dir)"
  seq="$(invoker_task_seq)"
  if [ "$seq" -gt 1 ] 2>/dev/null; then printf '%s/t%s' "$run_dir" "$seq"
  else printf '%s' "$run_dir"; fi
}

# Envelope ids have to be unique across the WHOLE session, not within one task.
# Both budgets reset when the counter moves, so task 2's first ask would reuse
# task 1's envelope id and the brainer's "process this envelope even if your
# inbox coalesced it" instruction would be pointing at a collision. The task
# segment is omitted for task 1 so every envelope id already in flight keeps its
# spelling.
invoker_envelope_prefix() {
  local seq
  seq="$(invoker_task_seq)"
  if [ "$seq" -gt 1 ] 2>/dev/null; then printf '%s:t%s' "${HW_RUN:-norun}" "$seq"
  else printf '%s' "${HW_RUN:-norun}"; fi
}

# ONE INVOKER AT A TIME PER RUN. Both invokers are read-check-act on state that
# only they write, with the act far from the read:
#
#   ask-invoker:  COUNT=$(count asks)  ...  ASK_SEQ=$((COUNT+1))  ... write
#   done-invoker: [ -f done ] && die   ...  deliver              ... : > done
#
# An agent can issue parallel Bash calls, so two invokers from the SAME run can
# interleave: two asks both read COUNT=2 against a cap of 3 and both proceed,
# spending four asks and colliding on one envelope id; two dones both pass the
# marker check and the brainer is told twice about one task. Neither is as likely
# as the cross-executor race on a shared brainer pane — that one needs only two
# executors finishing in the same second — but the fix is the same primitive and
# it costs one directory.
#
# Held for the whole invocation, because the window IS the whole invocation.
# Keyed on the run so different executors never wait on each other.
#
# THE HOLDER GOES INSIDE, AND THE RELEASE CHECKS IT. Neither used to be true,
# and the two omissions compose into the exact race the lock exists to prevent.
# MEASURED 2026-09-08, three probe processes driving this function through the
# real library, one HW_RUN:
#
#   A takes the lock and stays alive holding it. The lock is an EMPTY directory,
#   so the only evidence about its holder is its mtime — `ls -A` on it printed
#   nothing. Age it past `find -mmin +5`, which any invoker on a long budget
#   reaches without doing anything wrong, and B reaps it and takes it: TWO LIVE
#   HOLDERS. A then exits normally, and the EXIT trap runs `rmdir <path>` — by
#   PATH, so it deletes B's LIVE lock. C then walks straight in while B is still
#   inside its window. Three invokers of one run in the region that exists to
#   hold one.
#
# So the pid is written inside, `kill -0` decides staleness on the first pass
# instead of a five-minute timer, and the release removes the directory only
# while it still names us. `channel-send`'s delivery lock has worked this way
# since 2026-08-25; these two are now the same design instead of two.
# A PID IS NOT AN IDENTITY; A PID PLUS ITS START TIME IS. This half was
# MISSING until 2026-09-08, when both Judgment Day judges pointed out —
# independently, and correctly — that the commit claiming these two locks are
# now one design had given the delivery lock `owner.start` and left this one
# recording a bare pid. A claim the diff does not support is a finding, so this
# is the diff catching up with the claim rather than the claim being softened.
#
# The trailing `|| true` is load-bearing, not decoration: under
# `set -euo pipefail` a failing `ps` kills the whole pipeline and takes the
# assignment with it, and the caller dies with no diagnosis. Measured while
# writing the delivery-lock half of this same design.
_invoker_proc_start() { ps -o lstart= -p "$1" 2>/dev/null | tr -s ' ' | sed 's/^ *//; s/ *$//' || true; }

invoker_run_lock_path() {
  local key
  key="$(printf '%s' "${HW_RUN:-norun}" | tr -c 'A-Za-z0-9_.-' '_')"
  printf '%s/hw-invoker-%s.lock' "${TMPDIR:-/tmp}" "$key"
}

# RELEASE BY IDENTITY, NEVER BY PATH. A lock that no longer names us was reaped
# by a successor which is holding it right now, and removing it would hand a
# third invoker the same window. Returning quietly is the whole point: there is
# nothing of OURS left to release, and saying so with a stale `rmdir` is what
# broke the successor.
invoker_release_run_lock() {
  local lock owner
  lock="${INVOKER_RUN_LOCK:-}"
  [ -n "$lock" ] || return 0
  owner="$(cat "$lock/owner.pid" 2>/dev/null || true)"
  [ "$owner" = "$$" ] || return 0  # MUTATION-ANCHOR: 101-M01
  rm -f "$lock/owner.pid" "$lock/owner.start" 2>/dev/null || true
  rmdir "$lock" 2>/dev/null || true
  return 0
}

# BREAKING A LOCK IS A CLAIM ON IT, SO IT IS SERIALISED AND RE-CHECKED. A
# waiter reads owner.pid, then decides. Between the two the holder can release
# cleanly and exit, and a successor can take the lock: the waiter's `kill -0`
# on the pid it read then fails, and removing the lock BY PATH deletes the
# successor's live one. Measured 2026-09-24 by replaying that interleaving
# through this library: the waiter printed "holder is gone", took the lock, and
# the successor was still inside its window — two live holders.
#
# So a waiter that has judged a lock dead does not remove it. It takes
# `<lock>.reap` — one waiter at a time — re-reads the holder's pid AND start
# time, and removes the lock only while they are still the ones it judged. A
# (pid, start) that is dead stays dead, so an unchanged identity is the whole
# proof, and nothing else removes a dead holder's lock while `.reap` is held.
# A lock that recorded no holder, or bytes that are not a pid, is judged by age
# instead, so its age is asked again inside. Returns 0 when it removed the lock, 1 when it did not; the
# caller then waits as it would for any busy holder.
#
# The reap lock is held for a few syscalls, so one older than a minute belongs
# to a waiter that died inside them and is removed. That removal is by age and
# is NOT identity-checked; its window needs a waiter killed inside those
# syscalls, and it is stated here rather than implied away.
_invoker_reap_run_lock() { # <lock> <owner as read> <start as read>
  local lock="$1" seen_owner="$2" seen_start="$3" reap="$1.reap" rc=1 age_probe now_owner now_start
  if ! mkdir "$reap" 2>/dev/null; then
    age_probe="$(find "$reap" -maxdepth 0 -mmin +1 2>/dev/null)" || age_probe=''
    [ -z "$age_probe" ] || rmdir "$reap" 2>/dev/null || true
    return 1
  fi
  # An unreadable file reads as empty. Against a holder that WAS recorded that
  # is a mismatch, so the lock is left alone and the waiter waits.
  now_owner="$(cat "$lock/owner.pid" 2>/dev/null || true)"
  now_start="$(cat "$lock/owner.start" 2>/dev/null || true)"
  if [ "$now_owner" = "$seen_owner" ] && [ "$now_start" = "$seen_start" ]; then  # MUTATION-ANCHOR: 101-M06
    # A recorded pid was judged dead; anything else — nothing, or bytes that
    # are not a pid — was judged by age, so the age is asked again.
    case "$seen_owner" in
      ''|*[!0-9]*)
        age_probe="$(find "$lock" -maxdepth 0 -mmin +1 2>/dev/null)" || age_probe=''
        [ -z "$age_probe" ] || rc=0
        ;;
      *) rc=0 ;;
    esac
  fi
  if [ "$rc" = 0 ]; then
    rm -f "$lock/owner.pid" "$lock/owner.start" 2>/dev/null || true
    rmdir "$lock" 2>/dev/null || true
  fi
  rmdir "$reap" 2>/dev/null || true
  return "$rc"
}

# A SIGNAL ENDS THE INVOCATION; IT DOES NOT MERELY DROP THE LOCK. This used to
# be `trap invoker_release_run_lock EXIT INT TERM`, and a trap that does not
# `exit` REPLACES the signal's default action: the TERMed invoker released its
# lock, returned from the handler and carried on with its read-check-act — now
# outside the region that exists to serialise it. Measured 2026-09-24 with the
# real done-invoker: TERMed while it held the lock, it made its next herdr-rpc
# call with the lock already gone. So the handler releases AND exits, with the
# shell's own 128+signal status; the EXIT trap then runs again, and releasing
# twice is a no-op because the release goes by identity.
#
# Both callers re-arm the EXIT trap after taking the lock, and they call THIS for
# the signals rather than spelling the trap themselves, so there is one copy.
invoker_arm_lock_signals() {
  trap 'invoker_release_run_lock; exit 130' INT
  trap 'invoker_release_run_lock; exit 143' TERM  # MUTATION-ANCHOR: 101-M05
}

invoker_run_lock() {
  local lock deadline owner age_probe owner_start live_start claim_start
  lock="$(invoker_run_lock_path)"
  deadline=$(( $(date +%s) + 120 ))
  while :; do
    if mkdir "$lock" 2>/dev/null; then
      # mkdir is the atomic claim; the pid is written immediately after it, so a
      # waiter that arrives inside that window sees no owner file. That case is
      # age-guarded below rather than treated as stale on sight.
      printf '%s\n' "$$" > "$lock/owner.pid" 2>/dev/null || true
      _invoker_proc_start "$$" > "$lock/owner.start" 2>/dev/null || true
      INVOKER_RUN_LOCK="$lock"
      trap 'invoker_release_run_lock' EXIT
      invoker_arm_lock_signals
      return 0
    fi
    owner="$(cat "$lock/owner.pid" 2>/dev/null || true)"
    case "$owner" in
      ''|*[!0-9]*)
        # Either a claim in flight, or a lock left behind by a version that
        # recorded nothing. Give the claim window a minute, then fall back to
        # the old age rule — which is all that version ever gave anyone.
        #
        # AND THE PROBE'S FAILURE IS NOT ITS ANSWER. `[ -n "$(find … || true)" ]`
        # reads a FAILED find and a find that said "not old enough" as the same
        # empty string — which is the defect this whole change is about, so it is
        # not going in the fix for it. `lint-shell` caught it here, correctly.
        # Three values, and the unreadable one waits: reaping a lock we could not
        # age would let two invokers into the same window, while waiting costs a
        # bounded budget that already has its own diagnosis.
        age_probe="$(find "$lock" -maxdepth 0 -mmin +1 2>/dev/null)" || age_probe=__unreadable__
        case "$age_probe" in
          __unreadable__) : ;;   # could not ask: the lock stays LIVE and we wait
          '') : ;;               # asked, and it is not old enough yet: wait
          *)
            claim_start="$(cat "$lock/owner.start" 2>/dev/null || true)"
            if _invoker_reap_run_lock "$lock" "$owner" "$claim_start"; then  # MUTATION-ANCHOR: 101-M07
              printf '%s: breaking an invoker lock that records no holder (%s, older than a minute). Nothing has been read or sent yet.\n' \
                "${INVOKER_PROG:-invoker}" "$lock" >&2
              continue
            fi
            ;;
        esac
        ;;
      "$$")
        # Already ours. Waiting for ourselves would spend the whole budget and
        # then blame another invoker for it.
        INVOKER_RUN_LOCK="$lock"
        return 0
        ;;
      *)
        # A REUSED PID IS NOT A BUSY HOLDER. `kill -0` says only that SOME
        # process has this number now; between a crashed invoker and this
        # moment the number can have been handed to anything. Without this the
        # waiter spent the full 120s and then reported "pid N, still alive"
        # about a process that was never an invoker.
        #
        # Only when a start time was recorded. A lock from a version that wrote
        # none falls through to `kill -0` alone, which is what that version
        # gave anyone anyway — an absent record is not evidence of reuse.
        owner_start="$(cat "$lock/owner.start" 2>/dev/null || true)"  # MUTATION-ANCHOR: 101-M04
        if [ -n "$owner_start" ]; then
          live_start="$(_invoker_proc_start "$owner")"
          if [ -n "$live_start" ] && [ "$live_start" != "$owner_start" ] \
             && _invoker_reap_run_lock "$lock" "$owner" "$owner_start"; then
            printf '%s: breaking an invoker lock whose holder pid was REUSED (%s, pid %s started %s, the recorded holder started %s). The invoker that took this lock is gone.\n' \
              "${INVOKER_PROG:-invoker}" "$lock" "$owner" "$live_start" "$owner_start" >&2
            continue
          fi
        fi
        # MUTATION-ANCHOR: 101-M02
        if ! kill -0 "$owner" 2>/dev/null \
           && _invoker_reap_run_lock "$lock" "$owner" "$owner_start"; then
        # MUTATION-ANCHOR-END: 101-M02
          printf '%s: breaking an invoker lock whose holder is gone (%s, pid %s). That is a crashed or killed invoker, not a busy one.\n' \
            "${INVOKER_PROG:-invoker}" "$lock" "$owner" >&2
          continue
        fi
        ;;
    esac
    if [ "$(date +%s)" -ge "$deadline" ]; then
      printf '%s: another invoker for this run has held %s for over 120s (pid %s, still alive). Nothing was sent. Retry.\n' \
        "${INVOKER_PROG:-invoker}" "$lock" "${owner:-none recorded}" >&2
      exit 1
    fi
    sleep 1
  done
}

# ── receiver-owned inbox routing ──────────────────────────────────────────

invoker_route() {
  if [ -n "${HW_EXECUTOR_VENDOR:-}" ] \
     && [ "$HW_EXECUTOR_VENDOR" = "${HW_INVOKER_VENDOR:-}" ] \
     && invoker_vendor_has_native_transport "$HW_EXECUTOR_VENDOR" \
     && [ -n "${HW_INVOKER_SESSION:-}" ] \
     && [ -n "${HW_INVOKER_ENDPOINT:-}" ]; then
    printf '%s' "$HW_EXECUTOR_VENDOR"
  else
    printf 'herdr'
  fi
}

# --report ON THE HERDR ROUTE, AND IT IS A STATEMENT OF FACT ABOUT THIS
# DIRECTION, NOT A CONVENIENCE. Everything that reaches this function is an
# executor speaking to its brainer: done-invoker's completion or blocker, and
# ask-invoker's question or contract challenge. None of them tries to steer work
# the brainer already has in flight — they are the thing the brainer is waiting
# for, and the brainer reads them whenever its loop gets there. channel-send's
# idle gate is for the opposite direction (a redirect, `hw next`), and applying
# it here deadlocked a brainer against its own executor on 2026-08-27: the
# brainer was `working` because it was inside `hw wait` FOR THAT REPORT, and the
# report's sender sat 6m04s in the gate that state creates. See the --report
# block in bin/channel-send for the measurement.
#
# The native routes take no flag because they have no gate: opencode delivers
# into a session over a causal channel and the receiver's turn boundary never
# enters it. `invoker_route` never returns claude — the one route where
# --report is deliberately not honoured — so no call from here can silently rely
# on a flag that route ignores.
invoker_deliver() {
  local message="$1" report_state="${2:-}" route
  route="$(invoker_route)"
  if [ "$route" = herdr ]; then
    if [ -n "$report_state" ]; then
      "$INVOKER_CHANNEL_SEND" --report --report-state "$report_state" herdr "$HW_INVOKER_PANE" - "$message"
    else
      "$INVOKER_CHANNEL_SEND" --report herdr "$HW_INVOKER_PANE" - "$message"
    fi
  else
    "$INVOKER_CHANNEL_SEND" "$route" "$HW_INVOKER_SESSION" "$HW_INVOKER_ENDPOINT" "$message"
  fi
}

# Which vendors have a native transport an invoker can address. MIRRORS
# `_vendor_has_native_transport` in bin/hw, and 81-codex-reply-command.sh
# asserts the two lists agree: hw loads this library lazily and its own
# classifier is extracted and driven alone by 80-codex-brief-delivery.sh, so
# neither can simply call the other.
#
# WHY CODEX IS HERE, measured 2026-09-06 (setup/decisions.md, "Codex es un
# vendor sin transporte nativo"): `codex queue` resolves against an app-server
# daemon (`codex remote-control start`) that does not run on this machine, and
# a TUI started by `herdr agent start` registers with no daemon, so there is no
# address at which a codex session can be reached natively. The herdr route
# works on codex, and since 888583c it is the one hw uses towards a codex
# executor; this is the same answer in the other direction.
invoker_vendor_has_native_transport() {
  case "${1:-}" in
    ""|claude|codex) return 1 ;;
    *) return 0 ;;
  esac
}

# Resolve the executor's own native address for the reply instruction carried
# inside an ask. This is live state: session ids do not exist when hw first
# writes the run env, and OpenCode's actual port lives in the process argv.
# The optional argument names ANOTHER pane. hw uses it to address an executor
# for a follow-up task; with no argument it resolves this pane, which is what
# both invokers want and how it was originally written.
invoker_resolve_sender() {
  local info process port="" pane="${1:-${HERDR_PANE_ID:-}}"
  if [ -n "${1:-}" ]; then
    # A named pane is somebody else's: nothing about OUR vendor applies to it.
    INVOKER_SENDER_VENDOR=""
  else
    INVOKER_SENDER_VENDOR="${HW_EXECUTOR_VENDOR:-}"
  fi
  INVOKER_SENDER_SESSION=""
  INVOKER_SENDER_ENDPOINT=""
  [ -n "$pane" ] || return 0
  info="$(herdr agent get "$pane" 2>/dev/null || true)"
  INVOKER_SENDER_SESSION="$(printf '%s' "$info" | jq -r '.result.agent.agent_session.value // empty' 2>/dev/null || true)"
  [ -n "$INVOKER_SENDER_VENDOR" ] || \
    INVOKER_SENDER_VENDOR="$(printf '%s' "$info" | jq -r '.result.agent.agent // empty' 2>/dev/null || true)"
  case "$INVOKER_SENDER_VENDOR" in
    opencode)
      process="$(herdr pane process-info --pane "$pane" 2>/dev/null || true)"
      port="$(printf '%s' "$process" | jq -r '
        first(.result.process_info.foreground_processes[]?
          | select(any(.argv[]?; . == "opencode")) | .argv) as $argv
        | ($argv | index("--port")) as $i
        | if $i == null then empty else $argv[$i + 1] // empty end
      ' 2>/dev/null || true)"
      # if/else, not `[ -n ] && …`: this is the LAST command of the branch, so
      # its exit status is the function's, and an opencode pane with no --port
      # in its argv made invoker_resolve_sender return 1. Under `set -e` that
      # aborts invoker_reply_command before it prints anything — the same
      # silent-exit shape this repo keeps writing. Measured: all three live
      # opencode panes resolve with no port.
      if [ -n "$port" ]; then INVOKER_SENDER_ENDPOINT="http://127.0.0.1:$port"; fi
      ;;
    # NO CODEX BRANCH OF ITS OWN, AND THAT IS THE FIX. This used to hand every
    # codex pane `~/.codex/app-server-control/app-server-control.sock` as its
    # endpoint and, while herdr had not yet published agent_session, a thread id
    # GUESSED from the work directory's basename. Both were measured false on
    # 2026-09-06 against a live pane: the socket is created by a daemon that
    # does not run, and `codex queue --thread <basename>` answers "No active
    # session found matching". `invoker_reply_command` then saw a vendor that
    # was not claude, a session and an endpoint — the native route's conditions
    # — and printed `channel-send … codex <thread> <socket>`, which fails with
    # "No such file or directory" when the brainer runs it. The session herdr
    # DOES publish (codex's own UserPromptSubmit hook writes it) is real and is
    # kept above, so receipts can name the thread. The endpoint is the herdr
    # sentinel, as for claude: nothing here may invent an address.
    claude|codex) INVOKER_SENDER_ENDPOINT="-" ;;
  esac
  # Explicit, so no future branch can make "could not resolve an endpoint" into
  # a non-zero exit that kills the caller. Not resolving is a normal outcome:
  # the herdr route exists for exactly that.
  return 0
}

invoker_reply_command() {
  # NO SAME-VENDOR TERM HERE, and removing it is a fix, not a relaxation.
  #
  # This used to require `$INVOKER_SENDER_VENDOR = ${HW_INVOKER_VENDOR:-}` —
  # copied from `invoker_route`, where it is load-bearing for a different
  # reason: that direction sends to the BRAINER using the EXECUTOR's vendor as
  # the route, so the two must match or it would speak the wrong protocol. This
  # direction is the reverse. The reply is run by the brainer and addressed to
  # the executor's OWN session and endpoint, both resolved directly a line
  # above. What the brainer happens to be is irrelevant to an HTTP POST at the
  # executor's port.
  #
  # And the term was not merely redundant, it was UNSATISFIABLE: hw records
  # HW_EXECUTOR_VENDOR and HW_INVOKER_PANE in `.hw/<run>/env` and has never
  # recorded HW_INVOKER_VENDOR, so the comparison was always against the empty
  # string and this branch was dead for every vendor.
  #
  # MEASURED COST, 2026-08-25: a product-lane task raised a
  # contract challenge. This function printed a reply command on the `herdr`
  # route; the brainer ran it and sat 9 minutes in the idle gate, twice, while
  # that executor's own OpenCode endpoint answered 200 on 127.0.0.1:49437 the
  # whole time — a route with no gate that returns a causal `processed` receipt
  # in ~200ms. Reproduced with the executor's exact environment: without
  # HW_INVOKER_VENDOR the command is `herdr`, with it the command is native.
  #
  # STILL OPEN, deliberately not fixed here: `invoker_route` (executor ->
  # brainer) is starved of the same missing variable, and there the same-vendor
  # term IS the right condition. Fixing that means hw recording the brainer's
  # vendor at dispatch — a launch-path change with its own test surface, and it
  # would not have helped this incident anyway, because that brainer was
  # launched without --port and has no native endpoint at all.
  local logical_id="${1:-}" intent="${2:-answer}" hold_file="${3:-}" route target endpoint ruling_flag=""
  case "$intent" in answer) ;; ruling) ruling_flag="--ruling " ;; *) die "internal: unknown reply intent $intent" ;; esac
  invoker_resolve_sender
  # THE CLASSIFIER, NOT `!= claude`. The hand-coded test is how codex reached
  # this branch: it is not claude, and the resolver used to give it a session
  # and an endpoint. bin/hw's five route-selection sites stopped asking
  # `!= claude` in 888583c; this is the sixth, in the other direction, and the
  # one a codex executor's every ask goes through.
  if invoker_vendor_has_native_transport "$INVOKER_SENDER_VENDOR" \
     && [ -n "$INVOKER_SENDER_SESSION" ] \
     && [ -n "$INVOKER_SENDER_ENDPOINT" ]; then
    route="$INVOKER_SENDER_VENDOR"
    target="$INVOKER_SENDER_SESSION"
    endpoint="$INVOKER_SENDER_ENDPOINT"
  else
    route=herdr
    target="$HERDR_PANE_ID"
    endpoint=-
  fi
  if [ -n "$hold_file" ]; then
    case "$hold_file" in /*) ;; *) die "internal: pending reply file must be absolute" ;; esac
    umask 077
    {
      printf 'version=1\nstate=undelivered\nintent=%s\n' "$intent"
      printf 'route=%s\ntarget=%s\nlogical_id=%s\n' "$route" "$target" "$logical_id"
      printf 'pane=%s\nrun=%s\n' "$HERDR_PANE_ID" "${HW_RUN:-norun}"
    } > "$hold_file" || die "could not record pending reply fact at $hold_file"
  fi
  if [ "$route" = opencode ]; then
    [ -n "$logical_id" ] || die "internal: OpenCode reply command needs a stable envelope id"
    if [ -n "$hold_file" ]; then
      printf '%q %s--require processed --id %q --reply-hold %q %q %q %q "<your answer>"' \
        "$INVOKER_CHANNEL_SEND" "$ruling_flag" "$logical_id" "$hold_file" "$route" "$target" "$endpoint"
    else
      printf '%q %s--require processed --id %q %q %q %q "<your answer>"' \
        "$INVOKER_CHANNEL_SEND" "$ruling_flag" "$logical_id" "$route" "$target" "$endpoint"
    fi
  else
    # --report ON THE ANSWER TOO, AND THIS IS THE OTHER HALF OF THE SAME BUG.
    # An executor holding for a ruling cannot receive it, because the delivery
    # gate reads holding as busy — observed repeatedly from the executor's side
    # before anyone noticed it was two-way.
    # An answer to an ask is the mirror image of a report: the receiver asked
    # for it and is holding, so it is not steering work in flight, and refusing
    # it because the receiver is busy holding is the same category error. The
    # printed command is the brainer's, so the flag has to be IN the command —
    # a brainer that has to remember to add it is a brainer that will not.
    if [ "$intent" = ruling ]; then
      if [ -n "$hold_file" ]; then
        printf '%q --ruling --reply-hold %q %q %q %q "<your ruling>"' \
          "$INVOKER_CHANNEL_SEND" "$hold_file" "$route" "$target" "$endpoint"
      else
        printf '%q --ruling %q %q %q "<your ruling>"' \
          "$INVOKER_CHANNEL_SEND" "$route" "$target" "$endpoint"
      fi
    else
      if [ -n "$hold_file" ]; then
        printf '%q --report --reply-hold %q %q %q %q "<your answer>"' \
          "$INVOKER_CHANNEL_SEND" "$hold_file" "$route" "$target" "$endpoint"
      else
        printf '%q --report %q %q %q "<your answer>"' \
          "$INVOKER_CHANNEL_SEND" "$route" "$target" "$endpoint"
      fi
    fi
  fi
}

# ── the gate ──────────────────────────────────────────────────────────────

# invoker_wait_for_brainer <pane_id> <timeout_ms>
#
# 0 = the pane is idle or done, so a prompt will actually be read.
# 2 = it never got there in the budget — DO NOT PROMPT.
# 3 = herdr is not reachable at all. A different outcome, and worth saying so:
#     "the brainer is busy" and "the multiplexer is gone" call for different
#     things from the executor.
# 4 = the RPC itself errored (e.g. the pane no longer exists).
#
# `done` is accepted alongside `idle` because herdr derives `done` from an idle
# pane whose completion a human has not looked at yet — it is idle, plus a
# notification. Refusing it would strand every executor whose brainer finished
# a turn and was not watched.
invoker_wait_for_brainer() {
  "$INVOKER_RPC" wait-agent "$1" idle,done --timeout-ms "$2" >/dev/null 2>&1
}

# ── the dead end the gate used to have no move for ─────────────────────────
#
# `blocked` IS NOT ONE STATE. Measured 2026-08-26 against a live opencode pane
# with the herdr integration plugin at version 10:
#
#   event              scope   pane goes   the parent pane SHOWS
#   ------------------ ------- ----------- ----------------------------------
#   question.asked     root    blocked     the modal, `esc dismiss` on screen
#   permission.asked   root    blocked     the prompt, on screen
#   session.error      root    blocked     nothing
#   question.asked     child   blocked     NOTHING. The modal is behind
#   permission.asked   child   blocked     `ctrl+x down view subagents`
#   session.error      child   (nothing)   the managed plugin has no child
#                                          entry for it, so it is swallowed
#
# Those five want three different answers, and for four years' worth of this
# codebase's habits the tempting move is to tell them apart by reading the
# terminal. That does not work, and the measurement says why:
#
#   * A CHILD question modal leaves the parent showing only
#     `✓ Worker Task (background)` and `ctrl+x down view subagents`.
#     `esc dismiss` is NOT in the visible buffer, so the screen test below
#     refuses — correctly, given what it can see, and uselessly.
#   * `escape` sent to the parent does not reach that modal either. Measured:
#     send_keys escape returned ok and the pane stayed blocked. `ctrl+x`,
#     `down`, then `escape` does clear it.
#   * `esc dismiss` landed on line 39 of a 40-line read in the ROOT case, and
#     had scrolled out of that window minutes later on the same unchanged
#     blocked state. The screen test is not even stable for the case it was
#     written for.
#
# So the reason is published at the source instead — by
# `setup/opencode-hw-blocked-reason.js`, an adapter beside herdr's managed
# plugin — as two pane tokens, and read here:
#
#   blocked_reason  question | permission | error | stuck
#   blocked_scope   root | child
#
# `stuck` is the one that had no name before. Measured 2026-08-26: a child's
# question was rejected, the child and then the root session went idle, the
# modal left the screen — and herdr went on reporting `blocked`, with typed
# input not reaching the prompt box. Nothing was left for anyone to answer and
# no budget could reach idle. A consumer told `stuck` can say "relaunch"
# instead of "still busy, retry with a bigger budget", which is advice that
# could never have worked.
#
# The token decides WHAT the pane is waiting on. The screen is still consulted
# before any keystroke, but only as an interlock — "the modal I am about to
# dismiss is really there" — never as the classifier. A pane with no token at
# all (adapter not installed, or an older run) falls back to exactly the
# behaviour that shipped in 3cad1d4, so nothing that worked stops working.

# invoker_blocked_reason <pane_id>
#
# Prints "<reason> <scope>" for a blocked opencode pane, or nothing when the
# pane is not blocked, is not opencode, or carries no token. Never guesses:
# absent tokens print absent, and the caller decides what to do about it.
invoker_blocked_reason() {
  local info reason scope
  info="$("$INVOKER_RPC" call agent.get "$(printf '{"target":"%s"}' "$1")" 2>/dev/null || true)"
  [ -n "$info" ] || return 1
  # THE REPLY IS VALIDATED ONCE, HERE. Each field read below discarded jq's
  # stderr, so malformed JSON produced an empty string and read as "this pane
  # is not blocked" — a failure to parse rendered as a negative answer. Found
  # 2026-09-07 by the `unmeasured` lint rule. With the document proven parseable
  # first, an empty field below is a genuine absence, which is what the
  # docstring above already promises ("absent tokens print absent").
  printf '%s' "$info" | jq -e . >/dev/null 2>&1 || return 1
  [ "$(printf '%s' "$info" | jq -r '.agent.agent_status // empty')" = blocked ] || return 1
  [ "$(printf '%s' "$info" | jq -r '.agent.agent // empty')" = opencode ] || return 1
  reason="$(printf '%s' "$info" | jq -r '.agent.tokens.blocked_reason // empty' 2>/dev/null || true)"
  scope="$(printf '%s' "$info" | jq -r '.agent.tokens.blocked_scope // empty' 2>/dev/null || true)"
  [ -n "$reason" ] || return 1
  printf '%s %s' "$reason" "${scope:-root}"
}

# invoker_clear_opencode_question <pane_id>
#
# An opencode agent parked on a `question` modal reads `blocked` and STAYS
# there: nothing in the managed plugin leaves `blocked` except
# `permission.replied`, `question.replied` or `question.rejected`, and an idle
# opencode emits none of them on its own. So the pane sits blocked forever and
# no budget reaches it. Pressing the modal's own `esc dismiss` is the move.
#
# WHAT IT DELIBERATELY WILL NOT DO, because a blind keystroke at a terminal is
# a keystroke with no owner:
#   * it acts only on an `opencode` pane herdr calls `blocked`;
#   * it dismisses a QUESTION only. A `permission` prompt is left exactly as it
#     was, because dismissing one means DENY — that is a decision, not a
#     rescue. A `session.error` is left too: there is no modal to dismiss and
#     the pane is not waiting on input at all.
#   * it requires the modal's own `esc dismiss` footer to be on screen at the
#     moment it types, even when a token already said `question`;
#   * it verifies afterwards. Sending the key is not the claim; leaving
#     `blocked` is.
#
# 0 = a question modal was dismissed and the pane left `blocked`
# 1 = there was nothing of that shape to clear, or the key did not move it
# 2 = the pane is blocked for a reason that must NOT be auto-dismissed
#     (a permission prompt, or a session error). Distinct from 1 because the
#     caller can say so instead of reporting a generic busy pane.
invoker_clear_opencode_question() {
  local pane="$1" info state agent text rs reason scope

  info="$("$INVOKER_RPC" call agent.get "$(printf '{"target":"%s"}' "$pane")" 2>/dev/null || true)"
  [ -n "$info" ] || return 1
  state="$(printf '%s' "$info" | jq -r '.agent.agent_status // empty' 2>/dev/null || true)"
  agent="$(printf '%s' "$info" | jq -r '.agent.agent // empty' 2>/dev/null || true)"
  [ "$state" = blocked ] || return 1
  [ "$agent" = opencode ] || return 1

  reason="$(printf '%s' "$info" | jq -r '.agent.tokens.blocked_reason // empty' 2>/dev/null || true)"
  scope="$(printf '%s' "$info" | jq -r '.agent.tokens.blocked_scope // empty' 2>/dev/null || true)"
  [ -n "$scope" ] || scope=root

  case "$reason" in
    permission|error|stuck) return 2 ;;
    question) : ;;
    "") : ;;   # no adapter: fall through to the screen, as before
    *)  return 1 ;;   # a reason this version does not know: refuse, never guess
  esac

  # A CHILD modal is not on the parent's screen. `ctrl+x down` is the pane's
  # own `view subagents` binding, and it brings the modal into the buffer so
  # the interlock below can see it and `escape` can reach it. Only ever sent
  # when a token positively said the prompt belongs to a child — never
  # speculatively, because on a pane that is NOT showing a subagent list those
  # keys go somewhere else.
  if [ "$scope" = child ] && [ "$reason" = question ]; then
    "$INVOKER_RPC" call pane.send_keys \
      "$(printf '{"pane_id":"%s","keys":["ctrl+x","down"]}' "$pane")" >/dev/null 2>&1 || return 1
    # The subagent view has to paint before the interlock reads it.
    sleep "${INVOKER_SUBAGENT_VIEW_SETTLE_S:-2}"
  fi

  text="$("$INVOKER_RPC" call pane.read \
    "$(printf '{"pane_id":"%s","source":"visible","lines":%s}' "$pane" "${INVOKER_MODAL_READ_LINES:-80}")" 2>/dev/null \
    | jq -r '.read.text // empty' 2>/dev/null || true)"
  case "$text" in
    *"esc dismiss"*) : ;;
    *) return 1 ;;
  esac

  "$INVOKER_RPC" call pane.send_keys \
    "$(printf '{"pane_id":"%s","keys":["escape"]}' "$pane")" >/dev/null 2>&1 || return 1

  # The dismissal travels question.rejected -> working -> session.idle, so the
  # pane passes THROUGH `working` on its way out. Waiting for `idle,done` is
  # the only check that is not racing that transit.
  "$INVOKER_RPC" wait-agent "$pane" idle,done --timeout-ms "${INVOKER_UNBLOCK_WAIT_MS:-15000}" \
    >/dev/null 2>&1 || return 1
  return 0
}
