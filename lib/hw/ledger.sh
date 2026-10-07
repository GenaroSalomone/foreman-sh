# lib/hw/ledger.sh — the dispatch ledger, the brief commit, the picker's rows, the briefless refusal.
#
# Sourced ONCE by bin/hw (from $HW_BIN_DIR/../lib/hw/ledger.sh), after reap and status. A
# library: no shebang, nothing runs on source except function definitions. bin/hw keeps
# only call sites: `_send_brief` (ledger + brief commit), `cmd_next` (ledger), the launch
# argv (`--no-brief`, `_briefless_gate`), `_interactive` (`_picker_rows`) and the `ledger`
# verb (`cmd_ledger`).
#
# WHY A LEDGER. Nothing durable said what had been dispatched: a brief that was done and a
# brief that was never launched read the same, and 1685 briefs could not be told apart
# without opening each run directory (audit 2026-10-07). One JSONL line per real dispatch.
#
# WHERE IT LIVES: `$WORK/.hw-ledger/<lane>.jsonl` (`HW_LEDGER_DIR` overrides), and not in
# the brain repo's `.git/`. The two choices differ in what they outlive:
#   · a run directory, a worktree, `hw done`, `hw reap` — both survive them, the reason it
#     is not under `<work>/<lane>/<task>/.hw` (reap removes exactly that);
#   · a fresh clone, or a brain checkout re-made from scratch — only `$WORK` survives;
#     `.git/` goes with the clone, and the executors' work directories do not;
#   · a test or a probe with a throwaway brain — `$WORK/` is what a probe already sets,
#     and a brain with no `.git/` at all (the fixtures) still has a place to write.
# `$WORK/` is outside every repo, so it is never committed, never swept by a worktree
# operation and never lost by a branch switch. One file per lane keeps the append
# contended by one lane at a time; a line is far below PIPE_BUF, so `>>` (O_APPEND) does
# not interleave two dispatches.
#
# ARCHIVED BRIEFS (lib/hw/briefs.sh) are still listed: `hw ledger` also reads
# briefs/archive/<yyyy-mm>/, marks the row `archived`, and reports `done_at` (the done
# marker's mtime, or what the ledger recorded the first time it saw one) so the archive
# can name the month. The picker and the completion read only the flat directory.
#
# A DRY RUN WRITES NOTHING. Held here as well as at the call sites: a ledger that says a
# dispatch happened when none did is worse than none.

_ledger_dir() { printf '%s' "${HW_LEDGER_DIR:-${WORK:-$HOME/work}/.hw-ledger}"; }
_ledger_file() { printf '%s/%s.jsonl' "$(_ledger_dir)" "$1"; }

# Where the brief is committed to, for the ledger line. Set by _ledger_commit_brief.
LEDGER_BRIEF_COMMIT=""

# One line for the dispatch that is happening NOW. $1 = pane, $2 = delivery status of the
# brief (0 delivered), $3 = task sequence (1 for a launch; N for `hw next`), $4 = rundir.
# Never fails the dispatch: a ledger that cannot be written warns and moves on.
_ledger_dispatch() {
  [ "${DISPATCH_DRY:-0}" != 1 ] || return 0  # MUTATION-ANCHOR: 811-M01
  local pane="${1:-}" drc="${2:-0}" seq="${3:-1}" rundir="${4:-}" f sha="" base=""
  [ -n "${PROJ:-}" ] && [ -n "${TASK:-}" ] || return 0
  [ -n "$rundir" ] || { [ -n "${WT:-}" ] && rundir="$WT/.hw/${HW_RUN:-}"; }
  if [ -n "${BRIEF:-}" ] && [ -f "$BRIEF" ]; then sha="$(git hash-object -- "$BRIEF" 2>/dev/null || true)"; fi
  base="$(_task_base 2>/dev/null || true)"
  f="$(_ledger_file "$PROJ")"
  mkdir -p "$(dirname "$f")" 2>/dev/null || { warn "ledger: cannot create $(dirname "$f"); this dispatch is NOT recorded"; return 0; }
  jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg lane "$PROJ" --arg task "$TASK" \
    --arg run "${HW_RUN:-}" --argjson seq "$seq" \
    --arg brief "${BRIEF:-}" --arg sha "$sha" --arg commit "${LEDGER_BRIEF_COMMIT:-}" \
    --arg account "${ACCOUNT:-}" --arg model "${MODEL:-}" --arg effort "${EFFORT:-}" \
    --arg vendor "${AGENT:-}" --arg sdd "${SDD:-}" --arg base "$base" \
    --arg pane "$pane" --arg rundir "$rundir" --argjson delivered "$([ "$drc" = 0 ] && echo true || echo false)" \
    '{v:1, ts:$ts, lane:$lane, task:$task, run:$run, seq:$seq,
      brief:(if $brief=="" then null else $brief end),
      brief_sha:(if $sha=="" then null else $sha end),
      brief_commit:(if $commit=="" then null else $commit end),
      account:$account, model:$model, effort:$effort, vendor:$vendor, sdd:$sdd, base:$base,
      pane:$pane, rundir:$rundir, delivered:$delivered}' >> "$f" 2>/dev/null \
    || warn "ledger: could not append to $f; this dispatch is NOT recorded"
  return 0
}

# The same line for a re-task (`hw next`, and the launch route that reuses a pane): the lane
# and the task come from the run, the brief from the call. Locals shadow the launch globals
# for the length of the call and nothing else.
_ledger_next() {  # $1 pane  $2 rundir  $3 task seq  $4 brief path ("" when the task came as text)
  local PROJ TASK BRIEF="${4:-}" DISPATCH_DRY=0
  # THE RUN IS THE DIRECTORY'S NAME. HW_RUN is unset in a brainer's `hw next`, and a line with
  # run "" made every re-task of one task number in a lane the same (run, seq) key, so one
  # run's done marker read as done for all of them (judgment day, both judges).
  local HW_RUN; HW_RUN="$(basename "$2")"  # MUTATION-ANCHOR: 811-M06
  PROJ="$(_run_env_value "$2" HW_PROJECT 2>/dev/null || true)"
  TASK="$(_run_env_value "$2" HW_TASK 2>/dev/null || true)"
  [ -n "$BRIEF" ] && TASK="$(basename "$BRIEF" .md)"
  [ -n "$PROJ" ] && [ -n "$TASK" ] || return 0
  # THE SESSION'S OWN MODEL, EFFORT AND VENDOR. `hw next` re-tasks a pane that already
  # runs: the globals here are the CLI's defaults for a launch that did not happen, so
  # the line said "sonnet" for a pane that was opus (judgment day, registro-de-despachos).
  # Read from the run's `dispatch` file and env the way cmd_next and the reuse-route
  # refusals read them. Unreadable stays empty: no value beats the wrong one.
  local MODEL EFFORT AGENT  # MUTATION-ANCHOR: 811-M07
  MODEL="$(_next_dispatch_model "$2" 2>/dev/null || true)"
  EFFORT="$(_next_dispatch_effort "$2" 2>/dev/null || true)"
  AGENT="$(_run_env_value "$2" HW_EXECUTOR_VENDOR 2>/dev/null || true)"  # MUTATION-ANCHOR-END: 811-M07
  _ledger_commit_brief
  _ledger_dispatch "$1" 0 "$3" "$2"
}

# ── the brief is committed when it is dispatched ────────────────────────────
# 695 briefs sat untracked until they were swept by hand (95a5a396), and about 300 a week
# keep appearing. The dispatch is the moment a brief becomes a contract, so it is the
# moment it is committed — ONE FILE, by the private-index recipe (setup/CLAUDE.md,
# «Commiteando en un árbol compartido»), because the checkout is shared and holds other
# agents' uncommitted work.
#
# commit-tree and not `git commit`: no pre-commit hook, so the window in which HEAD can
# move under the tree being written is a second, not seven minutes — and update-ref is
# given the OLD value, so if HEAD did move it refuses instead of undoing what landed.
# Only on `main`: on any other branch the brief would be committed to somebody's feature.
# Never fails the dispatch.
_ledger_commit_brief() {
  LEDGER_BRIEF_COMMIT=""
  [ "${DISPATCH_DRY:-0}" != 1 ] || return 0
  [ -n "${BRIEF:-}" ] && [ -f "$BRIEF" ] || return 0
  local top rel abs branch old oldtree idx blob mode newtree c changed dirty
  abs="$(cd "$(dirname "$BRIEF")" 2>/dev/null && pwd -P)/$(basename "$BRIEF")" || return 0
  case "$abs" in
    "$(cd "$BRAIN" 2>/dev/null && pwd -P)"/*/briefs/*.md) ;;
    *) return 0 ;;
  esac
  top="$(git -C "$BRAIN" rev-parse --show-toplevel 2>/dev/null)" || return 0
  top="$(cd "$top" && pwd -P)"
  rel="${abs#"$top"/}"
  [ "$rel" != "$abs" ] || return 0
  # Tracked and unmodified: nothing to commit; its commit is the last one that touched it.
  dirty="$(git -C "$top" status --porcelain -- "$rel" 2>/dev/null)" || return 0  # a failed status is not "clean"
  if [ -z "$dirty" ]; then
    LEDGER_BRIEF_COMMIT="$(git -C "$top" log -1 --format=%H -- "$rel" 2>/dev/null || true)"
    return 0
  fi
  branch="$(git -C "$top" symbolic-ref --short -q HEAD 2>/dev/null || true)"
  if [ "$branch" != main ]; then
    warn "brief $rel is not committed and the brain checkout is on '${branch:-a detached HEAD}', not main: left uncommitted"
    return 0
  fi
  old="$(git -C "$top" rev-parse -q --verify refs/heads/main 2>/dev/null)" || return 0
  idx="$(mktemp "${TMPDIR:-/tmp}/hw-brief-idx.XXXXXX")" || return 0
  rm -f "$idx"
  oldtree="$old^{tree}"
  if ! blob="$(git -C "$top" hash-object -w -- "$rel" 2>/dev/null)"; then rm -f "$idx"; warn "brief commit: could not hash $rel"; return 0; fi
  mode=100644; [ -x "$abs" ] && mode=100755
  local built=0
  if GIT_INDEX_FILE="$idx" git -C "$top" read-tree "$old" 2>/dev/null; then
    GIT_INDEX_FILE="$idx" git -C "$top" update-index --add --cacheinfo "$mode,$blob,$rel" 2>/dev/null && built=1  # MUTATION-ANCHOR: 811-M02
  fi
  if [ "$built" = 1 ]; then newtree="$(GIT_INDEX_FILE="$idx" git -C "$top" write-tree 2>/dev/null)" || built=0; fi
  if [ "$built" != 1 ]; then
    rm -f "$idx"; warn "brief commit: could not build the private index for $rel; left uncommitted"; return 0
  fi
  rm -f "$idx"
  # The one safety that matters: the new tree differs from HEAD's in that path and no other.
  changed="$(git -C "$top" diff-tree -r --name-only "$oldtree" "$newtree" 2>/dev/null || true)"  # a failed diff-tree reads as "differs": REFUSED below
  if [ "$changed" != "$rel" ]; then  # MUTATION-ANCHOR: 811-M05
    warn "brief commit REFUSED: the tree it built differs from HEAD in more than $rel; left uncommitted"
    return 0
  fi
  if ! c="$(git -C "$top" commit-tree "$newtree" -p "$old" -m "docs(briefs): ${PROJ:-lane}/${TASK:-task} brief, committed at dispatch" 2>/dev/null)"; then
    warn "brief commit: commit-tree failed (is user.name/user.email set?); $rel left uncommitted"; return 0
  fi
  if ! git -C "$top" update-ref refs/heads/main "$c" "$old" 2>/dev/null; then
    warn "brief commit: main moved while the commit was being written; $rel left uncommitted (nothing was undone)"
    return 0
  fi
  # The real index must not keep a stale idea of this path (it would show as staged-deleted).
  git -C "$top" reset -q -- "$rel" 2>/dev/null || warn "brief commit: committed, but 'git reset -- $rel' failed on the real index; run it by hand"
  LEDGER_BRIEF_COMMIT="$c"
  ok "brief committed at dispatch: ${c:0:8} ($rel)"
  return 0
}

# ── no brief is a refusal, not a normal run ─────────────────────────────────
# A brief that moved or a typo launched an executor with no contract and no error.
# `--no-brief` is the deliberate way to do it and says so in one line.
_briefless_gate() {
  [ -z "${BRIEF:-}" ] || return 0
  if [ "${NO_BRIEF:-0}" = 1 ]; then
    warn "launching $PROJ:$TASK WITHOUT a brief (--no-brief): the executor has no contract, no verification and no authorization to quote"
    return 0
  fi
  local want="$BRAIN/$PROJ/briefs/$TASK.md" near hint msg
  near="$(_briefless_nearest "$PROJ" "$TASK" | tr '\n' ' ')"
  hint="$(_briefless_archive_hint "$PROJ" "$TASK")"
  msg="no brief for $PROJ:$TASK — looked for $want"
  [ -z "$near" ] || msg="$msg"$'\n'"    nearest names in $PROJ/briefs: $near"
  [ -z "$hint" ] || msg="$msg"$'\n'"    $hint"
  die "$msg"$'\n'"    point at one with --brief <path>, or launch on purpose without a contract with --no-brief"  # MUTATION-ANCHOR: 811-M04
}

# The names closest to the task, fuzzy, across this lane's briefs (live and archived).
_briefless_nearest() {
  python3 - "$BRAIN" "$1" "$2" <<'PY' 2>/dev/null || true
import difflib, glob, os, sys
brain, lane, task = sys.argv[1:4]
names = []
for pat in ("%s/%s/briefs/*.md", "%s/%s/briefs/*/*.md"):
    for f in glob.glob(pat % (brain, lane)):
        n = os.path.basename(f)[:-3]
        if not n.startswith("_"):
            names.append(n)
for n in difflib.get_close_matches(task, sorted(set(names)), n=5, cutoff=0.5):
    print(n)
PY
}

# Where a moved brief may be: any subdirectory of the lane's briefs holding <task>.md, or a
# git history that once had it.
_briefless_archive_hint() {
  local lane="$1" task="$2" hit
  hit="$(fd -t f -g "$task.md" "$BRAIN/$lane/briefs" 2>/dev/null | head -1 || true)"
  if [ -n "$hit" ]; then
    printf 'it exists at %s (moved into a subfolder?): pass it with --brief' "$hit"; return 0
  fi
  hit="$(git -C "$BRAIN" log -1 --format=%h --diff-filter=D --name-only -- "$lane/briefs/$task.md" "$lane/briefs/*/$task.md" 2>/dev/null | head -1 || true)"
  [ -z "$hit" ] || printf 'it was deleted or moved in git history: git -C %s log --all --stat -- %s/briefs/*%s.md' "$BRAIN" "$lane" "$task"
  return 0
}

# ── the picker's rows: one pass over every brief ────────────────────────────
# Was: three forks per brief (grep, sed, cut) — 5.2 s on 1685 — and the first non-empty
# line, which is `---` for 1465 of 1685 briefs because they open with frontmatter.
# Now: lane<TAB>task<TAB>title, from the `# title`, else the first prose line, after the
# frontmatter. `_`-prefixed files are not tasks.
_picker_rows() {
  python3 - "$BRAIN" $HW_LANES <<'PY'
import glob, os, sys
brain, lanes = sys.argv[1], sys.argv[2:]
def title(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            lines = fh.read().splitlines()
    except OSError:
        return "(unreadable)"
    i = 0
    while i < len(lines) and not lines[i].strip():
        i += 1
    if i < len(lines) and lines[i].strip() == "---":  # MUTATION-ANCHOR: 811-M03
        j = i + 1
        while j < len(lines) and lines[j].strip() != "---":
            j += 1
        i = j + 1 if j < len(lines) else len(lines)
    prose, fence = "", False
    for ln in lines[i:]:
        s = ln.strip()
        if s.startswith("```"):
            fence = not fence
            continue
        if fence or not s:
            continue
        if s.startswith("#"):
            t = s.lstrip("#").strip()
            if t:
                return t[:60]
            continue
        if not prose and s != "---":
            prose = s
    return (prose or "(empty)")[:60]
for lane in lanes:
    for f in sorted(glob.glob(os.path.join(brain, lane, "briefs", "*.md"))):
        name = os.path.basename(f)[:-3]
        if name.startswith("_"):
            continue
        print("%s\t%s\t%s" % (lane, name, title(f).replace("\t", " ")))
PY
}

# ── hw ledger ───────────────────────────────────────────────────────────────
# `hw ledger [<lane>] [--brief <path>|--task <t>] [--state <s>] [--json]`
# For each brief: never-dispatched, in-progress or done — from the ledger plus the run's
# `done` marker. A done marker is also WRITTEN BACK into the ledger the first time it is
# seen, so reaping the worktree later cannot turn `done` into a question. A dispatch whose
# run directory is gone and was never seen reporting reads `done` with the evidence
# `workdir removed`: hw done and hw reap are the only things that remove it.
cmd_ledger() {
  local lane="" brief="" task="" state="" json=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --brief) _need_val "$@"; brief="$2"; shift 2 ;;
      --task)  _need_val "$@"; task="$2"; shift 2 ;;
      --state) _need_val "$@"; state="$2"; shift 2 ;;
      --json)  json=1; shift ;;
      -h|--help) cat <<'EOF'
hw ledger [<lane>] [--brief <path> | --task <task>] [--state <state>] [--json]

  One row per brief (and per dispatch with no brief): never-dispatched, in-progress or done,
  from the dispatch ledger ($WORK/.hw-ledger/<lane>.jsonl) plus the run's done marker.
  --state never-dispatched|in-progress|done   keep only that state
  --json                                      one JSON object per row
EOF
        return 0 ;;
      -*) die "unknown option: $1 (usage: hw ledger [<lane>] [--brief <path>|--task <t>] [--state <s>] [--json])" ;;
      *) [ -z "$lane" ] || die "hw ledger takes one lane (got: $lane, $1)"; lane="$(_canon_project "$1")"; shift ;;
    esac
  done
  case "$state" in ""|never-dispatched|in-progress|done) ;; *) die "--state must be never-dispatched, in-progress or done (got: $state)" ;; esac
  [ -z "$brief" ] || [ -z "$task" ] || die "--brief and --task name the same row two ways; pass one"
  if [ -n "$brief" ]; then
    [ -f "$brief" ] || die "no such brief: $brief"
    brief="$(cd "$(dirname "$brief")" && pwd -P)/$(basename "$brief")"
  fi
  python3 -I -c "$(_ledger_python_source)" "$BRAIN" "$(_ledger_dir)" "$lane" "$brief" "$task" "$state" "$json" $HW_LANES
}

_ledger_python_source() {
  cat <<'PY'
import glob, json, os, sys, time
brain, ldir, only_lane, only_brief, only_task, only_state, as_json = sys.argv[1:8]
lanes = [only_lane] if only_lane else sys.argv[8:]
as_json = as_json == "1"
brain = os.path.realpath(brain)

def iso(ts):
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(ts))

def load(lane):
    rows, seen_done = [], {}
    try:
        fh = open(os.path.join(ldir, lane + ".jsonl"), encoding="utf-8")
    except OSError:
        return rows, seen_done, None
    with fh:
        for ln in fh:
            try:
                r = json.loads(ln)
            except ValueError:
                continue
            if r.get("event") == "done":
                seen_done[(r.get("run"), r.get("seq", 1))] = r.get("done_at") or r.get("ts")
            elif "task" in r:
                rows.append(r)
    return rows, seen_done, os.path.join(ldir, lane + ".jsonl")

def judge(r, seen, lane, pending):
    """-> (state, evidence, done_at)"""
    key = (r.get("run"), r.get("seq", 1))
    rd, seq = r.get("rundir") or "", r.get("seq", 1)
    d = rd if seq <= 1 else os.path.join(rd, "t%d" % seq)
    if rd and os.path.isfile(os.path.join(d, "reopened")):
        return "in-progress", "reopened", None
    if rd and os.path.isfile(os.path.join(d, "done")):
        at = iso(os.path.getmtime(os.path.join(d, "done")))
        if key not in seen:
            pending.append({"v": 1, "event": "done", "lane": lane, "task": r.get("task"), "run": key[0],
                            "seq": key[1], "ts": iso(time.time()), "done_at": at})
            seen[key] = at
        return "done", "done marker", at
    if key in seen:
        return "done", "reported (recorded earlier)", seen[key]
    if rd and not os.path.isdir(rd):
        return "done", "workdir removed", None
    return "in-progress", "no done marker", None

out = []
for lane in lanes:
    rows, seen, path = load(lane)
    pending = []
    by_task = {}
    for r in rows:
        by_task.setdefault(r["task"], []).append(r)
    listed = set()
    bdir = os.path.join(brain, lane, "briefs")
    files = sorted(glob.glob(os.path.join(bdir, "*.md")))
    # An archived brief (briefs/archive/<yyyy-mm>/) is still a brief: its state is still told.
    # A flat file of the same name is the live one and hides it.
    afiles = sorted(glob.glob(os.path.join(bdir, "archive", "*", "*.md")))
    flat = set(os.path.basename(f)[:-3] for f in files)
    for f in files + afiles:
        name = os.path.basename(f)[:-3]
        archived = f in afiles
        if name.startswith("_") or (archived and (name in flat or name in listed)):
            continue
        real = os.path.realpath(f)
        if only_brief and real != only_brief:
            continue
        if only_task and name != only_task:
            continue
        hist = by_task.get(name, [])
        listed.add(name)
        if not hist:
            row = {"lane": lane, "task": name, "state": "never-dispatched", "brief": f}
            if archived:
                row["archived"] = True
            out.append(row)
            continue
        last = hist[-1]
        st, ev, at = judge(last, seen, lane, pending)
        row = {"lane": lane, "task": name, "state": st, "evidence": ev, "brief": f, "dispatches": len(hist),
               "run": last.get("run"), "ts": last.get("ts"), "done_at": at, "model": last.get("model"),
               "vendor": last.get("vendor"), "account": last.get("account"), "effort": last.get("effort"),
               "brief_sha": last.get("brief_sha"), "brief_commit": last.get("brief_commit"),
               "pane": last.get("pane")}
        if archived:
            row["archived"] = True
        out.append(row)
    # dispatches that point at no brief in this lane's briefs/ (--no-brief, --brief elsewhere)
    for name, hist in by_task.items():
        if name in listed or only_brief:
            continue
        if only_task and name != only_task:
            continue
        last = hist[-1]
        st, ev, at = judge(last, seen, lane, pending)
        out.append({"lane": lane, "task": name, "state": st, "evidence": ev, "brief": last.get("brief"),
                    "dispatches": len(hist), "run": last.get("run"), "ts": last.get("ts"), "done_at": at,
                    "model": last.get("model"), "vendor": last.get("vendor"), "account": last.get("account"),
                    "effort": last.get("effort"), "brief_sha": last.get("brief_sha"),
                    "brief_commit": last.get("brief_commit"), "pane": last.get("pane")})
    if pending and path:
        try:
            with open(path, "a", encoding="utf-8") as fh:
                for p in pending:
                    fh.write(json.dumps(p, separators=(",", ":")) + "\n")
        except OSError:
            pass

if only_state:
    out = [o for o in out if o["state"] == only_state]
if as_json:
    for o in out:
        print(json.dumps(o, separators=(",", ":")))
else:
    for o in out:
        extra = ""
        if o["state"] != "never-dispatched":
            extra = "  %s  %s  %s" % ((o.get("ts") or "")[:16], o.get("model") or o.get("vendor") or "-", o.get("run") or "")
        print("%-16s %-14s %s%s%s" % (o["state"], o["lane"], o["task"], extra, "  (archived)" if o.get("archived") else ""))
PY
}
