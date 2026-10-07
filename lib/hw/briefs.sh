# lib/hw/briefs.sh — finished briefs leave briefs/ for briefs/archive/<yyyy-mm>/, and come back.
#
# Sourced ONCE by bin/hw (from $HW_BIN_DIR/../lib/hw/briefs.sh), after ledger. A library: no
# shebang, nothing runs on source except function definitions. bin/hw keeps only call sites:
# `_archived_brief_gate` (the launch route and `_new_brief`) and the `briefs` verb.
#
# WHY. 1685 briefs sit flat in <lane>/briefs/, and the picker, the completion and every
# listing read all of them. A finished brief is read by nobody until somebody re-launches it,
# so it moves out of the way — but a move is exactly what made `hw` launch an executor with NO
# contract and no error (audit 2026-10-07), so the move comes with its own refusal: a task
# whose brief is archived DIES, naming the archived path and the way back. It never launches
# without a brief.
#
# WHAT MOVES. A brief that is `done` by `hw ledger`, or whose run reported (a `done` marker in
# $WORK/<lane>/<task>/.hw), or has no ledger line and the history says it finished (`backfill`:
# a .hw done marker, or a merge of `task/<task>` on the base where the lane's branches live in the
# brain repo) — AND that has no live worktree. Or STALE: no ledger line, no evidence, and the brief's
# last commit in brain is more than 21 days old (a product lane merges by squash PR, so git has no
# `task/<t>` merge to find) — AND no live worktree; it goes to the month of that last commit.
# A newer brief stays never-dispatched. Never a fresh `never-dispatched`, never
# `in-progress`, never a `_`-prefixed document. "Live worktree" is read as: the lane's worktree of
# the task (`_lane_wt_dir`: {checkout}/.worktrees/<task> for a product lane, $WORK/<lane>/<task>
# for setup) or its plain work dir $WORK/<lane>/<task> exists and its newest run has not reported
# (a run that reported BLOCKED and waits for a ruling has not finished). A reported run whose directory is still there
# does not hold its brief: the brief is the contract of a task that is over.
# A brief that is untracked or modified is left where it is and named: `git mv` needs it
# committed, and archiving must not decide what an uncommitted edit means.
#
# HOW IT COMMITS. The checkout is shared and holds other agents' uncommitted work, so the moves
# go through a PRIVATE INDEX (setup/CLAUDE.md, «Commiteando en un árbol compartido»):
# `git mv` against a private GIT_INDEX_FILE renames the file on disk and in that index only;
# commit-tree builds the commit from it, update-ref is given the OLD value (a HEAD that moved
# is refused, not undone), and the real index is reset for those paths alone afterwards.
# The new tree must differ from HEAD's in exactly the paths the operation names, or nothing
# is committed and the disk is put back.
#
# THE WAY BACK. `briefs/archive/<yyyy-mm>/MANIFEST.tsv` holds `path<TAB>blob<TAB>reason` for each
# brief, the path being where it was. `hw briefs restore <lane> <task>` returns one;
# `hw briefs unarchive <lane> --manifest <file>` returns every row of a manifest, all or
# nothing. Archive then unarchive leaves a byte-identical tree (the tree object is the same).

# The archived copy of <lane>:<task> (the newest month if two exist), or nothing. Returns 0.
_archived_brief_path() {
  local lane="$1" task="$2" f hit=""
  for f in "$BRAIN/$lane/briefs/archive"/*/"$task.md"; do
    [ -f "$f" ] && hit="$f"
  done
  [ -z "$hit" ] || printf '%s' "$hit"
  return 0
}

# The refusal. Called where `_infer_brief` found no flat brief; a launch of an archived task
# ends here, `--no-brief` or not: an archived task HAS a contract, and launching it without
# one is the exact failure the archive must not reopen. `--brief <path>` still runs it as is.
_archived_brief_gate() {
  local lane="$1" task="$2" hit
  hit="$(_archived_brief_path "$lane" "$task")"
  [ -n "$hit" ] || return 0
  die "$lane:$task is ARCHIVED — its brief is $hit"$'\n'"    restore it:  hw briefs restore $lane $task"$'\n'"    or run it as it is:  --brief $hit"$'\n'"    hw never launches an archived task without a brief (--no-brief does not apply to it)"  # MUTATION-ANCHOR: 812-M01
}

cmd_briefs() {
  local sub="${1:-}"
  [ $# -eq 0 ] || shift
  case "$sub" in
    archive)   _briefs_archive "$@" ;;
    restore)   _briefs_restore "$@" ;;
    unarchive) _briefs_unarchive "$@" ;;
    ""|-h|--help|help) _briefs_usage ;;
    *) die "unknown: hw briefs $sub (usage: hw briefs archive|restore|unarchive — hw briefs --help)" ;;
  esac
}

_briefs_usage() {
  cat <<'EOF'
hw briefs archive <lane> [--apply] [--list]
    Move every finished brief of the lane to <lane>/briefs/archive/<yyyy-mm>/ (the month it
    reported), in one commit, with a MANIFEST.tsv per month. Finished = `hw ledger` says done,
    or — for a brief the ledger never saw, because it is older than the ledger — the history
    says so (`backfill: …` in the reason): a run reported in $WORK/<lane>/<task>/.hw, or, in a
    lane whose checkout is the brain repo, a merge of its task branch on the base; and no live
    worktree. A brief with neither a ledger line nor such evidence whose last commit in the brain
    is more than 21 days old is `stale: …` and moves too, to the month of that commit. --list names the in-progress and never-dispatched briefs instead of counting them. Never moves a
    never-dispatched or in-progress brief, a `_` document, or one that is untracked or modified.
    Without --apply it prints the plan and changes nothing. The commit lands on the branch the
    checkout has checked out.
hw briefs restore <lane> <task>
    Move one archived brief back to <lane>/briefs/<task>.md (one commit; refuses to overwrite).
hw briefs unarchive <lane> --manifest <file> [--manifest <file> ...]
    Move back every brief a manifest lists, all or nothing, and delete the manifest.

An archived task does not launch: `hw <lane> <task>` dies naming the archived path and the
restore command — `--no-brief` does not override it; `--brief <archived path>` runs it as is.
EOF
}

# ── the plan ────────────────────────────────────────────────────────────────
# One pass: lane's flat briefs + `hw ledger --json` + the work dirs + git's idea of tracked.
# stdout, tab-separated:  move <task> <yyyy-mm> <reason>   |   skip <task> <why>
_briefs_plan() {
  local lane="$1" ljson
  ljson="$(mktemp "${TMPDIR:-/tmp}/hw-briefs-ledger.XXXXXX")" || die "briefs: cannot make a temp file"
  cmd_ledger "$lane" --json > "$ljson" 2>/dev/null || { rm -f "$ljson"; die "briefs: hw ledger failed for $lane, so no brief's state is known; nothing is planned"; }
  # Where each task's worktree would be, by the lane's own template (not a guess at $WORK/<lane>/<task>).
  local wts="$ljson.wt" f t
  : > "$wts"
  for f in "$BRAIN/$lane/briefs"/*.md; do
    [ -f "$f" ] || continue
    t="$(basename "$f" .md)"
    printf '%s\t%s\n' "$t" "$(_lane_wt_dir "$lane" "$t" 2>/dev/null || true)" >> "$wts"
  done
  # GIT EVIDENCE exists only where the lane's checkout IS the brain repo (setup): its task
  # branches are in this repo's refs. A product lane's branches live in the product repo,
  # which this reads nothing of, so for it the branch template is empty and git says nothing.
  local gtop gco gbranch="" gbase=""
  gtop="$(git -C "$BRAIN" rev-parse --show-toplevel 2>/dev/null || true)"
  gco="$(lane_checkout "$lane" 2>/dev/null || true)"
  if [ -n "$gtop" ] && [ -n "$gco" ] && [ -d "$gco" ] && [ "$(cd "$gco" && pwd -P)" = "$(cd "$gtop" && pwd -P)" ]; then
    gbranch="$(_lane_branch "$lane" "{task}" 2>/dev/null || true)"; gbase="$(_lane_base "$lane" 2>/dev/null || true)"
  fi
  python3 -I - "$BRAIN" "$lane" "${WORK:-$HOME/work}" "$ljson" "$wts" "$gbranch" "$gbase" <<'PY' || { rm -f "$ljson" "$wts"; die "briefs: the plan could not be computed"; }
import glob, json, os, re, subprocess, sys, time
brain, lane, work, ljson, wtfile, gbranch, gbase = sys.argv[1:8]
wtdirs = {}
with open(wtfile, encoding="utf-8") as fh:
    for ln in fh:
        t, _, d = ln.rstrip("\n").partition("\t")
        wtdirs[t] = d

def git(*a):
    return subprocess.run(["git", "-C", brain] + list(a), capture_output=True, text=True, check=True).stdout

prefix = git("rev-parse", "--show-prefix").strip()
tracked = set(git("ls-files", "-z", "--", lane + "/briefs").split("\0")) - {""}
dirty = set()
for ent in git("status", "--porcelain", "-z", "--untracked-files=no", "--no-renames", "--", lane + "/briefs").split("\0"):
    if len(ent) > 3:
        p = ent[3:]
        dirty.add(p[len(prefix):] if prefix and p.startswith(prefix) else p)

rows = {}
with open(ljson, encoding="utf-8") as fh:
    for ln in fh:
        try:
            r = json.loads(ln)
        except ValueError:
            continue
        if not r.get("archived"):
            rows[r.get("task")] = r

def iso(ts):
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(ts))

def hw_done(wd):
    """The newest run's reported time (epoch) if it reported and was not reopened, else None."""
    hd = os.path.join(wd, ".hw")
    try:
        runs = sorted(d for d in os.listdir(hd) if os.path.isdir(os.path.join(hd, d)))
    except OSError:
        return None
    if not runs:
        return None
    run = os.path.join(hd, runs[-1])
    tn = [d for d in os.listdir(run) if re.fullmatch(r"t[0-9]+", d) and os.path.isdir(os.path.join(run, d))]
    d = os.path.join(run, max(tn, key=lambda x: int(x[1:]))) if tn else run
    if os.path.isfile(os.path.join(d, "reopened")) or os.path.isfile(os.path.join(d, "blocked-waiting")):
        return None
    m = os.path.join(d, "done")
    return os.path.getmtime(m) if os.path.isfile(m) else None

def git_merges():
    """task -> (sha, iso): the newest merge commit on the base whose subject names the lane's
    task branch. Empty where the lane's branches are not in this repo.
    NOT EVIDENCE: a surviving branch that is an ancestor of the base. A branch cut from the
    base and never worked on is an ancestor too, and nothing in the refs tells the two apart."""
    merges = {}
    if not gbranch or not gbase:
        return merges
    try:
        git("rev-parse", "--verify", "-q", gbase + "^{commit}")
    except subprocess.CalledProcessError:
        return merges
    pre, _, post = gbranch.partition("{task}")
    rx = re.compile(r"(?<![\w./-])" + re.escape(pre) + r"([A-Za-z0-9][A-Za-z0-9._-]*)" + re.escape(post))
    for ln in git("log", gbase, "--merges", "--format=%H%x09%cI%x09%s").splitlines():
        sha, _, rest = ln.partition("\t")
        when, _, subj = rest.partition("\t")
        # git's own "Merge branch 'a' into b" merges a, not b
        m = re.match(r"Merge (?:remote-tracking )?branch '([^']+)'", subj)
        for t in rx.finditer(m.group(1) if m else subj):
            merges.setdefault(t.group(1).rstrip("."), (sha, when))
    return merges

def git_evidence(name, merges):
    """-> (reason, iso) or None."""
    if name in merges:
        sha, when = merges[name]
        return "backfill: git merge %s of %s" % (sha[:8], gbranch.replace("{task}", name)), when
    return None

gmerges = git_merges()

STALE_DAYS = 21

def last_commits():
    """brief path (relative to the brain dir) -> committer epoch of the newest commit touching it.
    One pass over the lane's briefs history, not a `git log -1` per brief."""
    out, cur = {}, None
    for ln in git("log", "--no-renames", "--relative", "--name-only", "--format=%x01%ct", "--", lane + "/briefs").splitlines():
        if ln.startswith("\x01"):
            cur = int(ln[1:])
        elif ln and cur is not None:
            out.setdefault(ln, cur)
    return out

lastc = last_commits()
now = time.time()

bdir = os.path.join(brain, lane, "briefs")
for f in sorted(glob.glob(os.path.join(bdir, "*.md"))):
    name = os.path.basename(f)[:-3]
    if name.startswith("_"):
        continue
    rel = "%s/briefs/%s.md" % (lane, name)
    row = rows.get(name)
    state = row["state"] if row else "never-dispatched"
    when, reason = None, ""
    if state == "in-progress":  # MUTATION-ANCHOR: 812-M02
        print("skip\t%s\tin-progress" % name)
        continue
    if state == "done":
        reason = "ledger: " + (row.get("evidence") or "done")
        when = row.get("done_at") or row.get("ts")
    # Every place this task's executor may live: the lane's worktree and the plain work dir.
    wds = [w for w in dict.fromkeys([wtdirs.get(name, ""), os.path.join(work, lane, name)]) if w]
    present = [w for w in wds if os.path.isdir(w)]
    held = None
    for w in present:
        h = hw_done(w)
        if h is None:
            held = None
            break
        held = max(held or 0, h)
    if state == "never-dispatched":
        # No ledger line: the ledger starts the day it was built. The history decides, and the
        # reason says so (`backfill`) and names the evidence.
        gev = None if held is not None else git_evidence(name, gmerges)  # MUTATION-ANCHOR: 813-M01
        if held is not None:
            reason, when = "backfill: .hw done marker", iso(held)
        elif gev:
            reason, when = gev
        elif rel in lastc and now - lastc[rel] > STALE_DAYS * 86400:  # MUTATION-ANCHOR: 814-M01
            # STALE: no ledger line, no evidence, and nobody has touched the brief in brain for
            # more than STALE_DAYS. A product lane merges by squash PR, so git has no `task/<t>`
            # merge to find; the age is the only signal left. The live-worktree guard below still wins.
            reason, when = "stale: no ledger entry or evidence; last commit " + time.strftime("%Y-%m-%d", time.gmtime(lastc[rel])), iso(lastc[rel])
        else:  # MUTATION-ANCHOR: 813-M02
            print("skip\t%s\tnever-dispatched" % name)
            continue  # MUTATION-ANCHOR-END: 813-M02
    if present and held is None:  # MUTATION-ANCHOR: 812-M04
        print("skip\t%s\tlive-worktree" % name)
        continue
    if not when:
        when = iso(os.path.getmtime(f))
        reason += " (month from the brief's mtime)"
    if rel not in tracked:
        print("skip\t%s\tuntracked" % name)
        continue
    if rel in dirty:
        print("skip\t%s\tmodified" % name)
        continue
    if glob.glob(os.path.join(bdir, "archive", "*", name + ".md")):
        print("skip\t%s\tname-taken" % name)
        continue
    print("move\t%s\t%s\t%s" % (name, when[:7], reason))
PY
  rm -f "$ljson" "$wts"
}

_briefs_archive() {
  local lane="" apply=0 list=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --apply) apply=1; shift ;;
      --list) list=1; shift ;;
      -h|--help) _briefs_usage; return 0 ;;
      -*) die "unknown option: $1 (usage: hw briefs archive <lane> [--apply] [--list])" ;;
      *) [ -z "$lane" ] || die "hw briefs archive takes one lane (got: $lane, $1)"; lane="$(_canon_project "$1")"; shift ;;
    esac
  done
  [ -n "$lane" ] || die "usage: hw briefs archive <lane> [--apply]"
  [ -d "$BRAIN/$lane/briefs" ] || die "no briefs directory for $lane: $BRAIN/$lane/briefs"
  git -C "$BRAIN" rev-parse --show-toplevel >/dev/null 2>&1 || die "briefs: $BRAIN is not a git checkout; archive commits what it moves"

  local plan n_move n_stale reasons="never-dispatched in-progress live-worktree untracked modified name-taken" r c
  plan="$(_briefs_plan "$lane")" || exit 1
  n_move="$(printf '%s\n' "$plan" | awk -F'\t' '$1=="move"{n++} END{print n+0}')"
  if [ "$apply" = 1 ]; then printf 'archive %s\n' "$lane"; else printf 'archive plan for %s (a dry run: add --apply to move)\n' "$lane"; fi
  printf '%s\n' "$plan" | awk -F'\t' '$1=="move"{printf "  move  %s  %s  (%s)\n", $3, $2, $4}'
  # Skips: the counts, and by name only the ones that were nearly moved.
  for r in $reasons; do
    c="$(printf '%s\n' "$plan" | awk -F'\t' -v r="$r" '$1=="skip" && $3==r{n++} END{print n+0}')"
    [ "$c" -gt 0 ] || continue
    case "$r" in
      never-dispatched|in-progress)
        if [ "$list" = 1 ]; then printf '  skip  %s %s: %s\n' "$c" "$r" "$(printf '%s\n' "$plan" | awk -F'\t' -v r="$r" '$1=="skip" && $3==r{printf "%s ", $2}')"
        else printf '  skip  %s %s\n' "$c" "$r"; fi ;;
      *) printf '  skip  %s %s: %s\n' "$c" "$r" "$(printf '%s\n' "$plan" | awk -F'\t' -v r="$r" '$1=="skip" && $3==r{printf "%s ", $2}')" ;;
    esac
  done
  n_stale="$(printf '%s\n' "$plan" | awk -F'\t' '$1=="move" && $4 ~ /^stale:/{n++} END{print n+0}')"
  if [ "$n_stale" -gt 0 ]; then printf '  %s to move (%s of them stale)\n' "$n_move" "$n_stale"; else printf '  %s to move\n' "$n_move"; fi
  [ "$n_move" -gt 0 ] || return 0
  [ "$apply" = 1 ] || return 0

  local top ops months="" manifests="" kind task ym reason
  top="$(git -C "$BRAIN" rev-parse --show-toplevel)" || die "briefs: $BRAIN has no git toplevel"; top="$(cd "$top" && pwd -P)"
  ops="$(mktemp -d "${TMPDIR:-/tmp}/hw-briefs-ops.XXXXXX")" || die "briefs: cannot make a temp directory"
  local rel_lane
  rel_lane="$(_briefs_rel "$top" "$BRAIN/$lane")" || { rm -rf "$ops"; die "briefs: $BRAIN/$lane is outside the checkout at $top"; }
  : > "$ops/ops"
  months="$(printf '%s\n' "$plan" | awk -F'\t' '$1=="move"{print $3}' | sort -u)"
  local m src dst blob
  while IFS=$'\t' read -r kind task ym reason; do
    [ "$kind" = move ] || continue
    src="$rel_lane/briefs/$task.md"; dst="$rel_lane/briefs/archive/$ym/$task.md"
    blob="$(git -C "$top" hash-object -- "$src")" || die "briefs: cannot hash $src"
    printf 'mv\t%s\t%s\n' "$src" "$dst" >> "$ops/ops"
    printf '%s\t%s\t%s\n' "$src" "$blob" "$reason" >> "$ops/rows.$ym"
  done <<<"$plan"
  for m in $months; do
    local mrel="$rel_lane/briefs/archive/$m/MANIFEST.tsv"
    { [ -f "$top/$mrel" ] && cat "$top/$mrel"; cat "$ops/rows.$m"; } > "$ops/manifest.$m"
    printf 'put\t%s\t%s\n' "$mrel" "$ops/manifest.$m" >> "$ops/ops"
    manifests="$manifests $BRAIN/$lane/briefs/archive/$m/MANIFEST.tsv"
  done
  local msg undo="hw briefs unarchive $lane"
  msg="docs(briefs): archive $n_move finished $lane briefs ($(printf '%s\n' "$months" | paste -sd, -))"
  if ! _briefs_commit_ops "$top" "$msg" "$ops/ops"; then rm -rf "$ops"; exit 1; fi
  rm -rf "$ops"
  ok "archived $n_move $lane briefs: ${BRIEFS_COMMIT:-?}"
  for m in $manifests; do undo="$undo --manifest $m"; done
  info "undo: $undo   (returns EVERY brief those manifests list, earlier runs of the month included; to undo only this run: git revert ${BRIEFS_COMMIT:-<commit>})"
}

# $1 = checkout top, $2 = an absolute path under it → the path relative to the top.
_briefs_rel() {
  local top="$1" abs="$2"
  abs="$(cd "$(dirname "$abs")" 2>/dev/null && pwd -P)/$(basename "$abs")" || return 1
  [ "${abs#"$top"/}" != "$abs" ] || return 1
  printf '%s' "${abs#"$top"/}"
}

# ── restore / unarchive ─────────────────────────────────────────────────────
_briefs_restore() {
  local lane="" task=""
  while [ $# -gt 0 ]; do
    case "$1" in
      -h|--help) _briefs_usage; return 0 ;;
      -*) die "unknown option: $1 (usage: hw briefs restore <lane> <task>)" ;;
      *) if [ -z "$lane" ]; then lane="$(_canon_project "$1")"; elif [ -z "$task" ]; then task="$1"; else die "hw briefs restore takes <lane> <task> (got extra: $1)"; fi; shift ;;
    esac
  done
  [ -n "$lane" ] && [ -n "$task" ] || die "usage: hw briefs restore <lane> <task>"
  local hit; hit="$(_archived_brief_path "$lane" "$task")"
  [ -n "$hit" ] || die "no archived brief for $lane:$task under $BRAIN/$lane/briefs/archive/"
  _briefs_move_back "$lane" "restore $lane/$task from the archive" "$hit" ""
}

_briefs_unarchive() {
  local lane="" manifests=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --manifest) _need_val "$@"; manifests+=("$2"); shift 2 ;;
      -h|--help) _briefs_usage; return 0 ;;
      -*) die "unknown option: $1 (usage: hw briefs unarchive <lane> --manifest <file>)" ;;
      *) [ -z "$lane" ] || die "hw briefs unarchive takes one lane (got: $lane, $1)"; lane="$(_canon_project "$1")"; shift ;;
    esac
  done
  [ -n "$lane" ] && [ "${#manifests[@]}" -gt 0 ] || die "usage: hw briefs unarchive <lane> --manifest <file> [--manifest <file> ...]"
  _briefs_move_back "$lane" "unarchive ${#manifests[@]} manifest(s) of $lane" "" "${manifests[@]}"
}

# The shared engine of restore and unarchive. $1 lane, $2 commit subject tail, $3 one archived
# file (restore) or "", then manifests (unarchive). ALL OR NOTHING: every problem is gathered
# before anything moves, and a problem refuses the whole run.
_briefs_move_back() {
  local lane="$1" what="$2" single="$3"; shift 3
  git -C "$BRAIN" rev-parse --show-toplevel >/dev/null 2>&1 || die "briefs: $BRAIN is not a git checkout"
  local top ops problems="" rel_lane adir
  top="$(git -C "$BRAIN" rev-parse --show-toplevel)" || die "briefs: $BRAIN has no git toplevel"; top="$(cd "$top" && pwd -P)"
  rel_lane="$(_briefs_rel "$top" "$BRAIN/$lane")" || die "briefs: $BRAIN/$lane is outside the checkout at $top"
  adir="$(cd "$BRAIN/$lane/briefs" 2>/dev/null && pwd -P)/archive" || die "no briefs directory for $lane"
  ops="$(mktemp -d "${TMPDIR:-/tmp}/hw-briefs-ops.XXXXXX")" || die "briefs: cannot make a temp directory"
  : > "$ops/ops"; : > "$ops/files"

  local mf mdir path blob reason base src dst
  if [ -n "$single" ]; then
    mdir="$(cd "$(dirname "$single")" && pwd -P)"
    blob=""
    [ ! -f "$mdir/MANIFEST.tsv" ] || blob="$(awk -F'\t' -v p="$rel_lane/briefs/$(basename "$single")" '$1==p{print $2; exit}' "$mdir/MANIFEST.tsv")"
    printf '%s\t%s\t%s\n' "$mdir/$(basename "$single")" "$mdir/MANIFEST.tsv" "${blob:-}" >> "$ops/files"
  else
    for mf in "$@"; do
      [ -f "$mf" ] || { problems="$problems"$'\n'"  manifest not found: $mf"; continue; }
      mf="$(cd "$(dirname "$mf")" && pwd -P)/$(basename "$mf")"
      case "$mf" in "$adir"/*/MANIFEST.tsv) ;; *) problems="$problems"$'\n'"  not a manifest of $lane's archive ($adir/<yyyy-mm>/MANIFEST.tsv): $mf"; continue ;; esac
      while IFS=$'\t' read -r path blob reason; do
        [ -n "$path" ] || continue
        base="$(basename "$path")"
        case "$path" in "$rel_lane/briefs/"*.md) ;; *) problems="$problems"$'\n'"  $mf lists a path outside $rel_lane/briefs/: $path"; continue ;; esac
        printf '%s\t%s\t%s\n' "$(dirname "$mf")/$base" "$mf" "$blob" >> "$ops/files"
      done < "$mf"
    done
  fi

  local manifest_rel
  while IFS=$'\t' read -r src mf blob; do
    [ -n "$src" ] || continue
    base="$(basename "$src")"
    dst="$rel_lane/briefs/$base"
    if [ ! -f "$src" ]; then problems="$problems"$'\n'"  archived file is missing: $src"; continue; fi
    git -C "$top" ls-files --error-unmatch -- "$(_briefs_rel "$top" "$src")" >/dev/null 2>&1 \
      || { problems="$problems"$'\n'"  archived file is not committed: $src"; continue; }
    if [ -e "$top/$dst" ]; then problems="$problems"$'\n'"  would overwrite: $top/$dst"; continue; fi
    if [ -n "$blob" ] && [ "$(git -C "$top" hash-object -- "$src")" != "$blob" ]; then
      warn "briefs: $base changed since it was archived (blob differs from its manifest row); it is restored as it is now"
    fi
    printf 'mv\t%s\t%s\n' "$(_briefs_rel "$top" "$src")" "$dst" >> "$ops/ops"  # MUTATION-ANCHOR: 812-M03
  done < "$ops/files"
  if [ -n "$problems" ]; then
    rm -rf "$ops"
    die "briefs: nothing was moved back; $(printf '%s' "$problems" | awk 'NF' | wc -l | tr -d ' ') problem(s):$problems"
  fi
  [ -s "$ops/ops" ] || { rm -rf "$ops"; die "briefs: nothing to move back"; }

  # The manifests: rows of what moved back are dropped; a manifest left empty is deleted.
  local mpaths m moved_paths
  mpaths="$(if [ -n "$single" ]; then printf '%s\n' "$(cd "$(dirname "$single")" && pwd -P)/MANIFEST.tsv"; else for m in "$@"; do printf '%s\n' "$(cd "$(dirname "$m")" && pwd -P)/$(basename "$m")"; done; fi | sort -u)"
  moved_paths="$(awk -F'\t' '$1=="mv"{print $3}' "$ops/ops")" || true
  local idx=0
  while IFS= read -r m; do
    [ -n "$m" ] || continue
    manifest_rel="$(_briefs_rel "$top" "$m")" || continue
    [ -f "$m" ] || continue
    idx=$((idx + 1))
    awk -F'\t' 'NR==FNR{gone[$0]=1; next} !($1 in gone)' <(printf '%s\n' "$moved_paths") "$m" > "$ops/manifest.$idx"
    if [ -s "$ops/manifest.$idx" ]; then printf 'put\t%s\t%s\n' "$manifest_rel" "$ops/manifest.$idx" >> "$ops/ops"
    else printf 'del\t%s\n' "$manifest_rel" >> "$ops/ops"; fi
  done <<<"$mpaths"

  local n; n="$(grep -c '^mv' "$ops/ops")"
  if ! _briefs_commit_ops "$top" "docs(briefs): $what" "$ops/ops"; then rm -rf "$ops"; exit 1; fi
  rm -rf "$ops"
  # Month directories the move emptied are not part of any tree; leave none behind.
  local d
  for d in $(printf '%s\n' "$mpaths" | while IFS= read -r m; do dirname "$m"; done | sort -u); do rmdir "$d" 2>/dev/null || true; done
  rmdir "$adir" 2>/dev/null || true
  ok "moved $n $lane brief(s) back: ${BRIEFS_COMMIT:-?}"
}

# ── the commit, by private index ────────────────────────────────────────────
# $1 = checkout top, $2 = commit subject, $3 = an ops file, one per line, tab-separated:
#   mv <src> <dst>        git mv, in the private index and on disk
#   put <rel> <file>      write <file> to <rel> and add it
#   del <rel>             remove <rel> from disk and the index
# Sets BRIEFS_COMMIT. Returns 1 (and leaves the disk as it found it) on any failure.
_briefs_commit_ops() {
  local top="$1" subject="$2" opsfile="$3" idx old branch oldtree newtree c expect changed
  BRIEFS_COMMIT=""
  branch="$(git -C "$top" symbolic-ref --short -q HEAD 2>/dev/null || true)"
  [ -n "$branch" ] || { warn "briefs: HEAD of $top is detached; nothing was moved"; return 1; }
  old="$(git -C "$top" rev-parse -q --verify "refs/heads/$branch" 2>/dev/null)" || { warn "briefs: cannot read $branch"; return 1; }
  oldtree="$old^{tree}"
  idx="$(mktemp "${TMPDIR:-/tmp}/hw-briefs-idx.XXXXXX")" || return 1
  rm -f "$idx"
  local bk; bk="$(mktemp -d "${TMPDIR:-/tmp}/hw-briefs-bk.XXXXXX")" || return 1
  GIT_INDEX_FILE="$idx" git -C "$top" read-tree "$old" 2>/dev/null || { rm -rf "$idx" "$bk"; warn "briefs: cannot read HEAD into a private index"; return 1; }

  local op a b i=0 failed=0 undo="$bk/undo"
  : > "$undo"; : > "$bk/paths"
  # An interrupt part-way through leaves files moved and nothing committed: put the disk back.
  # shellcheck disable=SC2064
  trap "_briefs_undo '$top' '$undo'; rm -rf '$idx' '$bk'; trap - INT TERM; exit 130" INT TERM
  while IFS=$'\t' read -r op a b; do
    i=$((i + 1))
    case "$op" in
      mv)
        mkdir -p "$(dirname "$top/$b")"
        if GIT_INDEX_FILE="$idx" git -C "$top" mv -- "$a" "$b" >/dev/null 2>"$bk/err"; then
          printf 'mv\t%s\t%s\n' "$b" "$a" >> "$undo"; printf '%s\n%s\n' "$a" "$b" >> "$bk/paths"
        else warn "briefs: git mv $a failed: $(cat "$bk/err")"; failed=1; break; fi ;;
      put)
        if [ -e "$top/$a" ]; then cp -p "$top/$a" "$bk/orig.$i"; printf 'restore\t%s\t%s\n' "$a" "$bk/orig.$i" >> "$undo"
        else printf 'rm\t%s\n' "$a" >> "$undo"; fi
        mkdir -p "$(dirname "$top/$a")"
        if cp "$b" "$top/$a" && GIT_INDEX_FILE="$idx" git -C "$top" add -- "$a" >/dev/null 2>&1; then printf '%s\n' "$a" >> "$bk/paths"
        else warn "briefs: could not write $a"; failed=1; break; fi ;;
      del)
        cp -p "$top/$a" "$bk/orig.$i" 2>/dev/null && printf 'restore\t%s\t%s\n' "$a" "$bk/orig.$i" >> "$undo"
        if GIT_INDEX_FILE="$idx" git -C "$top" rm -q -f -- "$a" >/dev/null 2>&1; then printf '%s\n' "$a" >> "$bk/paths"
        else warn "briefs: could not remove $a"; failed=1; break; fi ;;
    esac
  done < "$opsfile"

  if [ "$failed" = 0 ]; then
    newtree="$(GIT_INDEX_FILE="$idx" git -C "$top" write-tree 2>/dev/null)" || failed=1
  fi
  if [ "$failed" = 0 ]; then
    # The one safety that matters: the tree differs from HEAD's in the named paths and no other.
    expect="$(sort -u "$bk/paths")"
    changed="$(git -C "$top" diff-tree -r --no-renames --name-only "$oldtree" "$newtree" 2>/dev/null | sort -u)" || true
    if [ -n "$(comm -23 <(printf '%s\n' "$changed") <(printf '%s\n' "$expect"))" ]; then
      warn "briefs: REFUSED — the tree it built differs from HEAD's in more than the paths it names; nothing was committed"
      failed=1
    fi
  fi
  if [ "$failed" = 0 ]; then
    if ! c="$(git -C "$top" commit-tree "$newtree" -p "$old" -m "$subject" 2>/dev/null)"; then warn "briefs: commit-tree failed (is user.name/user.email set?)"; failed=1
    elif ! git -C "$top" update-ref "refs/heads/$branch" "$c" "$old" 2>/dev/null; then warn "briefs: $branch moved while the commit was being written; nothing was undone upstream, and the disk is put back"; failed=1
    fi
  fi
  if [ "$failed" != 0 ]; then
    trap - INT TERM
    _briefs_undo "$top" "$undo"
    rm -rf "$idx" "$bk"
    return 1
  fi
  trap - INT TERM   # committed: from here nothing is undone
  # The real index keeps no stale idea of these paths (it would show them as staged renames).
  # shellcheck disable=SC2046
  git -C "$top" reset -q -- $(sort -u "$bk/paths" | tr '\n' ' ') 2>/dev/null || warn "briefs: committed, but 'git reset' of the moved paths failed on the real index; run it by hand"
  rm -rf "$idx" "$bk"
  BRIEFS_COMMIT="${c:0:8}"
  return 0
}

# Put the disk back, last operation first.
_briefs_undo() {
  local top="$1" undo="$2" op a b
  local rev; rev="$(awk '{a[NR]=$0} END{for(i=NR;i>=1;i--)print a[i]}' "$undo")"
  while IFS=$'\t' read -r op a b; do
    case "$op" in
      mv) mkdir -p "$(dirname "$top/$b")"; mv "$top/$a" "$top/$b" 2>/dev/null || warn "briefs: could not put $b back" ;;
      rm) rm -f "$top/$a" ;;
      restore) mkdir -p "$(dirname "$top/$a")"; cp -p "$b" "$top/$a" 2>/dev/null || warn "briefs: could not put $a back" ;;
    esac
  done <<<"$rev"
}
