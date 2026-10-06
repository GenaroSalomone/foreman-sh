# lib/hw/reap.sh — `hw reap` and the removal `hw done` shares with it.
#
# Sourced ONCE by bin/hw (from $HW_BIN_DIR/../lib/hw/reap.sh), at the point where
# this code used to sit, so every function and global is defined exactly where it
# was. It is a library: no shebang, nothing runs on source except assignments.
# Moved verbatim from bin/hw; see the commit message for the line ranges.

# ── reap's removal: ONE body for `hw reap --apply` and `hw done` ────────────
#
# Acts on the verdict the caller JUST computed with _wt_disposition, and only on
# `safe` or `archivable`; every other verdict is a KEEP and returns 1 untouched,
# so `safe` stays an allowlist (90-a-verdict-that-could-not-be-measured.sh).
#
# `archivable` is the path that did not exist: task outputs (WT_ARCHIVABLE) are
# copied to $(_archive_root)/<lane>/<task>/, each copy compared byte for byte
# with `diff -r`, the originals removed, and the worktree's verdict asked AGAIN —
# it is removed only if it now reads `safe` by the same gates as any other.
# Then the branch (`-d`, which refuses anything git does not see as merged) and,
# on a lane with `db.provisioned`, the worktree's database: `pg_dump -Fc` into
# the same archive, checked with `pg_restore -l`, and only then dropped.
#
# Sets REAP_ARCHIVE_DIR to where anything went; empty when nothing was moved.
REAP_ARCHIVE_DIR=""
_reap_archive_dest() {  # <proj> <wt> — a path that does not exist yet
  local d; d="$(_archive_root)/$1/$(basename "$2")"
  [ ! -e "$d" ] || d="$d.$(date +%Y%m%d-%H%M%S)"
  printf '%s' "$d"
}

_reap_archive_entries() {  # <wt> <dest>; the entries on stdin, one per line
  local wt="$1" dest="$2" rel
  while IFS= read -r rel; do
    [ -n "$rel" ] || continue
    mkdir -p "$dest/$(dirname "$rel")" || return 1
    cp -pR "$wt/$rel" "$dest/$rel" 2>/dev/null || return 1
    # THE COPY IS VERIFIED, NOT ASSUMED: a truncated copy followed by a removal
    # is the one outcome worse than keeping the worktree forever.
    diff -rq "$wt/$rel" "$dest/$rel" >/dev/null 2>&1 || return 1
  done
  return 0
}

# MEASURED 2026-10-06: the three databases reap still could not drop failed at
# the DUMP, not at the drop — pg_dump 14 first on PATH against a 16.10 server dies
# with "server version mismatch", and the `2>/dev/null` below hid it behind
# "could not be verified". A tool is taken from the major the server runs.
# HW_PG_BIN_DIRS (colon list of bin dirs) replaces the candidates searched.
_pg_major() {  # <tool path> — the major its --version names, empty when unreadable
  "$1" --version 2>/dev/null | sed -n 's/.* \([0-9][0-9]*\)[. ].*/\1/p;s/.* \([0-9][0-9]*\)$/\1/p' | head -1
}
_pg_tool() {  # <tool> <server major, or empty> — a path; PATH's own when nothing matches
  local tool="$1" want="$2" d c dirs
  if [ -n "$want" ]; then
    dirs="${HW_PG_BIN_DIRS:-/opt/homebrew/opt/postgresql@$want/bin:/usr/local/opt/postgresql@$want/bin:/usr/lib/postgresql/$want/bin}"
    c="$(command -v "$tool" 2>/dev/null)" || c=""
    if [ -n "$c" ] && [ "$(_pg_major "$c")" = "$want" ]; then printf '%s' "$c"; return 0; fi
    while [ -n "$dirs" ]; do
      d="${dirs%%:*}"; case "$dirs" in *:*) dirs="${dirs#*:}" ;; *) dirs="" ;; esac
      [ -x "$d/$tool" ] && [ "$(_pg_major "$d/$tool")" = "$want" ] && { printf '%s' "$d/$tool"; return 0; }  # MUTATION-ANCHOR: 760-M01
    done
  fi
  printf '%s' "$tool"
}

# DROP DATABASE, once. Prints nothing and returns 0 when it is gone (an
# already-absent database counts as dropped); returns 1 with the reason on
# stdout otherwise, naming who holds it open. Never retried here.
_reap_drop_db() {  # <user> <db>
  local u="$1" db="$2" err who
  if err="$(psql -U "$u" -d postgres -q -c "DROP DATABASE \"$db\"" 2>&1)"; then return 0; fi
  case "$err" in *"does not exist"*) return 0 ;; esac  # MUTATION-ANCHOR: 760-M02
  who="$(psql -U "$u" -d postgres -Atc "select coalesce(nullif(usename,''),'?') || '/' || coalesce(nullif(application_name,''),'-') || ' x' || count(*) from pg_stat_activity where datname = '$db' group by usename, application_name" 2>/dev/null | tr '\n' ' ')"
  if [ -n "$who" ]; then printf 'open connections: %s— close them and reap again' "$who"
  else printf '%s' "$(printf '%s' "$err" | head -1)"; fi
  return 1
}

_reap_db() {  # <proj> <main> <wt> <archive dir, or empty>
  local proj="$1" main="$2" wt="$3" dest="$4" db main_db u pgd pgr pgv derr why
  [ -n "$(lane_get "$proj" db_provisioned)" ] || return 0
  db="$(basename "$wt" | tr '\-/' '__')"
  main_db="$(basename "$main" | tr '\-/' '__')"
  [ "$db" != "$main_db" ] || { warn "kept database $db — it is the main checkout's name"; return 0; }
  u="$(id -un)"
  psql -U "$u" -d postgres -Atc "select 1 from pg_database where datname = '$db'" 2>/dev/null | grep -q 1 || return 0
  [ -n "$dest" ] || dest="$(_reap_archive_dest "$proj" "$wt")"
  mkdir -p "$dest" || { warn "kept database $db — could not create $dest"; return 0; }
  pgv="$(psql -U "$u" -d postgres -Atc "show server_version_num" 2>/dev/null | awk '/^[0-9]+$/ { print int($1 / 10000) }')"
  pgd="$(_pg_tool pg_dump "$pgv")"; pgr="$(_pg_tool pg_restore "$pgv")"
  if derr="$("$pgd" -U "$u" -Fc -f "$dest/$db.dump" "$db" 2>&1 >/dev/null)" \
     && [ -s "$dest/$db.dump" ] && "$pgr" -l "$dest/$db.dump" >/dev/null 2>&1; then
    REAP_ARCHIVE_DIR="$dest"
    if why="$(_reap_drop_db "$u" "$db")"; then
      ok "database $db dumped to $dest/$db.dump and dropped"
    else
      warn "database $db dumped to $dest/$db.dump but NOT dropped — $why"
    fi
  else
    rm -f "$dest/$db.dump"
    warn "kept database $db — its pg_dump into $dest could not be verified, so it was not dropped${derr:+: $(printf '%s' "$derr" | tail -1)}"
  fi
  return 0
}

_reap_worktree() {  # <proj> <main> <wt> <branch> <base>
  local proj="$1" main="$2" wt="$3" branch="$4" base="$5" dest="" rel entries
  REAP_ARCHIVE_DIR=""
  case "$WT_VERDICT" in safe|archivable) ;; *) return 1 ;; esac
  if [ "$WT_VERDICT" = archivable ]; then
    entries="$WT_ARCHIVE"
    dest="$(_reap_archive_dest "$proj" "$wt")"
    if ! printf '%s\n' "$entries" | _reap_archive_entries "$wt" "$dest"; then
      # The partial copy goes: the originals are untouched, and a copy left
      # here would be repeated by every later reap under a new timestamp.
      _reap_rm_workdir "$dest" || true
      warn "kept $(basename "$wt") — the copy into $dest could not be verified, so nothing was removed"
      return 1
    fi
    REAP_ARCHIVE_DIR="$dest"
    ok "archived $(printf '%s' "$entries" | tr '\n' ' ')→ $dest"
    while IFS= read -r rel; do
      [ -n "$rel" ] || continue
      _reap_rm_workdir "$wt/$rel" || true
    done <<< "$entries"
    _wt_disposition "$main" "$wt" "$branch" "$base"
    if [ "$WT_VERDICT" != safe ]; then
      warn "kept $(basename "$wt") — with its outputs archived it reads $WT_VERDICT: $WT_WHY"
      return 1
    fi
  fi
  if ! git -C "$main" worktree remove "$wt" 2>/dev/null; then
    warn "could not remove $wt — run it by hand and read what git says"
    return 1
  fi
  ok "removed $(basename "$wt")"
  if _reap_delete_branch "$main" "$branch" "$base"; then
    info "deleted $branch"
  else
    warn "kept $branch — git refused a safe delete, so something is not merged after all"
  fi
  _reap_db "$proj" "$main" "$wt" "$dest"
  return 0
}

# `-d` first. A squash- or PR-merged branch is one git can never see as merged,
# so `-d` refused it and every such branch outlived its worktree; `-D` only when
# the merge evidence, asked again right here, is one of those two.
_reap_delete_branch() {  # <main> <branch> <base>
  git -C "$1" branch -d "$2" >/dev/null 2>&1 && return 0
  WT_MERGE_EVIDENCE=""
  _wt_branch_merged "$1" "$2" "$3" && [ -n "$WT_MERGE_EVIDENCE" ] || return 1
  git -C "$1" branch -D "$2" >/dev/null 2>&1
}

# ── shed: the weight of a worktree that stays ─────────────────────────────
#
# A KEPT worktree loses only what its install or build rebuilds: git-ignored
# directories named in WT_SHED. A fixed list, not a lane field: the names are
# the framework's build output, not the lane's, and git saying "ignored" is the
# per-repo half. NOT node_modules: MEASURED 2026-10-04, pnpm on
# APFS clones it — 140 GiB by `du` freed ~0 by `df` — while `.next` (0.4–5.6 GB
# a worktree) freed exactly what `du` said. Removing node_modules buys nothing
# and costs the next reuse a `pnpm install`.
#
# Never under a live process: a dev server's cwd is the worktree, and `.next`
# is what it serves. `lsof` failing is a no. And only hw's own worktrees, under
# the lane's canonical root: `git worktree list` also holds the main checkout
# and worktrees other tools made (a Codex app's, measured in a product lane).
WT_SHED=".next .turbo ${HW_WT_SHED:-}"
# Which KEEP verdicts may shed: the ones about git. Not held, leased or
# undetermined (somebody may be using it, or we could not tell), not detached
# or not-a-worktree (not this tool's to decide about).
_wt_sheddable() { case "$WT_VERDICT" in dirty|unmerged|irreplaceable) return 0 ;; esac; return 1; }
_wt_shed() {  # <proj> <wt> — prints what it removed; returns 1 when it removed nothing
  local wt="$2" real root cwds ignored rel kb=0 k gone=""
  real="$(cd "$wt" 2>/dev/null && pwd -P)" || return 1
  root="$(_wt_canonical_root "$1")" && root="$(cd "$root" 2>/dev/null && pwd -P)" || return 1
  case "$real" in "$root"/*) ;; *) return 1 ;; esac
  cwds="$(lsof -a -d cwd -Fn 2>/dev/null)" || [ -n "$cwds" ] || return 1
  printf '%s\n' "$cwds" | awk -v d="n$real" '$0 == d || index($0, d "/") == 1 { f = 1 } END { exit !f }' && return 1
  ignored="$(git -C "$wt" status --porcelain --ignored 2>/dev/null)" || return 1
  while IFS= read -r rel; do
    rel="${rel%/}"
    case " $WT_SHED " in *" ${rel##*/} "*) ;; *) continue ;; esac
    [ -d "$wt/$rel" ] && [ ! -L "$wt/$rel" ] || continue
    k="$(du -sk "$wt/$rel" 2>/dev/null | awk '{print $1}')"
    rm -rf "${wt:?}/$rel" 2>/dev/null || continue
    kb=$((kb + ${k:-0})); gone="$gone $rel"
  done < <(printf '%s\n' "$ignored" | sed -n 's/^!! //p')
  [ -n "$gone" ] || return 1
  printf '%s (%s MB)' "${gone# }" "$((kb / 1024))"
}

# ── build output of an idle kept worktree ───────────────────────────────────
#
# MEASURED 2026-10-06: 31 GB of `.next`/`.turbo` (47 GB the day before) under
# worktrees that stay for a git reason. `_wt_shed` only runs on --apply, with no
# age, and says nothing in the dry run. This class has a window
# (`retention.build_output_days`, HW_BUILD_OUTPUT_DAYS, 7; 0 disables) and lists
# what it would free. ONLY the two build directories, git-ignored, never a
# source. THE BIAS IS KEEP: a window that cannot be read, a worktree outside
# the lane's root, a pane or process in it, an unread occupant list, a log that
# cannot be written, any file inside the window — all keep it.
WT_BUILD_DIRS=""; WT_BUILD_KB=0; WT_BUILD_AGE=""
_wt_build_idle() {  # <proj> <wt> — 0 when its build output is idle past the window; sets WT_BUILD_*
  local proj="$1" wt="$2" win real root cwds d dirs="" kb=0 m newest=0 now f act
  WT_BUILD_DIRS=""; WT_BUILD_KB=0; WT_BUILD_AGE=""
  win="$(_retention_days 7 "${HW_BUILD_OUTPUT_DAYS:-7}")"
  [ "$win" != 0 ] || return 1
  # An ALLOWLIST, as _wt_sheddable: a live pane reads `held`, an unread occupant
  # list `undetermined`, a live executor's lock `locked` (invisible to lsof when
  # idle), a plain directory `not-a-worktree` — none is this class's to free.
  case "$WT_VERDICT" in dirty|unmerged|irreplaceable) ;; *) return 1 ;; esac  # MUTATION-ANCHOR: 761-M03
  real="$(cd "$wt" 2>/dev/null && pwd -P)" || return 1
  root="$(_wt_canonical_root "$proj")" && root="$(cd "$root" 2>/dev/null && pwd -P)" || return 1
  case "$real" in "$root"/*) : ;; *) return 1 ;; esac
  cwds="$(lsof -Fn -a -d cwd 2>/dev/null)" || [ -n "$cwds" ] || return 1  # MUTATION-ANCHOR: 761-M05
  printf '%s\n' "$cwds" | awk -v d="n$real" '$0 == d || index($0, d "/") == 1 { f = 1 } END { exit !f }' && return 1
  while IFS= read -r d; do
    d="${d%/}"
    [ -n "$d" ] && [ -d "$d" ] && [ ! -L "$d" ] || continue
    git -C "$wt" check-ignore -q "$d" 2>/dev/null || continue
    act="$(find "$d" -type f -mtime "-$win" -print -quit 2>/dev/null)" && [ -z "$act" ] || return 1  # MUTATION-ANCHOR: 761-M04
    dirs="$dirs$d"$'\n'
    kb=$((kb + $(du -sk "$d" 2>/dev/null | awk '{print $1 + 0}')))
    m="$(stat -f %m "$d" 2>/dev/null || stat -c %Y "$d" 2>/dev/null)" || return 1
    [ "$m" -le "$newest" ] || newest="$m"
  done < <(fd -H -I -t d -d 4 -E node_modules '^\.(next|turbo)$' "$wt" 2>/dev/null)
  [ -n "$dirs" ] || return 1
  # git's own last touch counts too: a commit or checkout is activity. The reflog,
  # NOT the index: `git status` (this very survey) rewrites the index whenever a
  # stat changed, so its mtime would read every worktree as active.
  for f in "$(git -C "$wt" rev-parse --git-path logs/HEAD 2>/dev/null)"; do
    [ -n "$f" ] && [ -e "$f" ] || continue
    case "$f" in /*) ;; *) f="$wt/$f" ;; esac
    act="$(find "$f" -mtime "-$win" -print 2>/dev/null)" && [ -z "$act" ] || return 1
  done
  now="$(date +%s)"
  WT_BUILD_DIRS="$dirs"; WT_BUILD_KB="$kb"; WT_BUILD_AGE="$(( (now - newest) / 86400 ))"
  [ "$WT_BUILD_AGE" -ge "$win" ] || WT_BUILD_AGE="$win"
  return 0
}

# The lane's task branches no worktree holds any more, merged into the base.
# Measured 2026-10-01: 50 such branches across three
# repositories — what is left when a worktree was removed by hand. Only the lane's own
# task-branch namespace (`_lane_branch`), never another branch; `-d` only.
_reap_orphan_branches() {  # <proj> <main> <base> — prints them, one per line
  local proj="$1" main="$2" base="$3" prefix held b
  prefix="$(_lane_branch "$proj" X)"; prefix="${prefix%X}"
  [ -n "$prefix" ] && [ "$prefix" != "$(_lane_branch "$proj" X)" ] || return 0
  held="$(git -C "$main" worktree list --porcelain 2>/dev/null | sed -n 's:^branch refs/heads/::p')" || return 0
  while IFS= read -r b; do
    [ -n "$b" ] || continue
    printf '%s\n' "$held" | grep -qxF "$b" && continue
    _wt_branch_merged "$main" "$b" "$base" && printf '%s\n' "$b"
  done < <(git -C "$main" for-each-ref --format='%(refname:short)' "refs/heads/$prefix" 2>/dev/null)
  return 0
}

cmd_reap() {
  local apply=0 proj_filter=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --apply) apply=1; shift ;;
      -h|--help) echo "usage: hw reap [<project>] [--apply] — surveys the lane's git worktrees, its merged task branches no worktree holds, AND the loose work dirs beside them ($WORK/<lane>). --apply removes what is safe; a merged worktree whose only ignored content is .artifacts/qa-report/test-results/playwright-report has it copied to $(_archive_root)/<lane>/<task>/ and verified first, and a db.provisioned lane's database is pg_dump'ed there before it is dropped. Dirty, unmerged and occupied stay. Retention: a loose work dir whose task REPORTED (done, or blocked and closed) and not occupied is safe once every artifact is older than artifact_retention_days (30), a backup (*.tgz, *.tar.gz, *.tar, or a name containing backup) older than backup_retention_days (60) — both in projects.json under retention, 0 disables a window, HW_ARTIFACT_RETENTION_DAYS/HW_BACKUP_RETENTION_DAYS override; the dry run says «retention: Nd > 30d», and every removal by retention appends a line (time, lane, path, task, age, size, reason) to $(_reap_log_file) (HW_REAP_LOG). A task that never reported or has a live pane is kept at any age. A kept worktree idle past build_output_days (7; 0 disables) has its .next/.turbo freed (dry run lists them with sizes; --apply logs each); never one with a live pane or process in it. It also closes an executor kept after a --blocked report and left unanswered past HW_BLOCKED_WAIT_HOURS (default 24)." >&2; return 0 ;;
      -*) die "unknown option: $1" ;;
      *) proj_filter="$(_canon_project "$1")"; shift ;;
    esac
  done

  local proj main base total_safe=0 total_kept=0 total_removed=0 lane_safe0 lane_removed0
  local build_kb_total=0 bwin bd bkb
  while IFS= read -r proj; do
    [ -z "$proj_filter" ] || [ "$proj_filter" = "$proj" ] || continue
    main="$(_wt_main "$proj")" || continue
    [ -d "$main" ] || continue
    base="$(_lane_base "$proj")"
    : "${base:=main}"
    printf '\n  %s%s%s  (base %s)\n' "$C_B" "$proj" "$C_0" "$base"
    lane_safe0="$total_safe"; lane_removed0="$total_removed"
    local seed_from
    seed_from="$(lane_get "$proj" reap_copies_from)" || seed_from=""
    if [ -n "$seed_from" ]; then
      _wt_seed_patterns "$main/$seed_from" >/dev/null 2>&1 || true
      info "$WT_SEED_NOTE"
    fi

    local wt branch shed
    while IFS= read -r wt; do
      [ -n "$wt" ] || continue
      # THE MAIN CHECKOUT IS GIT'S FIRST ENTRY, dropped above by position. A
      # string compare with the lane's spelling of it missed whenever the two
      # spellings differ (a /var -> /private/var temp dir, a doubled slash): the
      # lane golden then listed the main checkout as `safe`, recipe and all,
      # with `branch -d` of the base. Kept as a second check.
      [ "$wt" = "$main" ] && continue
      branch="$(git -C "$wt" symbolic-ref --short HEAD 2>/dev/null || echo detached)"
      # a database this worktree's own runs recorded, before anything removes it
      if [ -n "$(lane_get "$proj" db_provisioned)" ]; then
        local rdb
        for rdb in $(cat "$wt"/.hw/2*/receipt.jsonl 2>/dev/null | jq -r 'select(.key=="database") | .value' 2>/dev/null | sort -u); do
          _db_ledger_add "$proj" "$rdb" "$wt"
        done
      fi
      _wt_disposition "$main" "$wt" "$branch" "$base"
      if [ "$WT_VERDICT" = safe ] || [ "$WT_VERDICT" = archivable ]; then
        total_safe=$((total_safe + 1))
        if [ "$apply" = 1 ]; then
          if _reap_worktree "$proj" "$main" "$wt" "$branch" "$base"; then
            total_removed=$((total_removed + 1))
          fi
        else
          # NO HAND RECIPE: `--apply` is the one path, and the `branch -d` this
          # printed is refused by git for a squash- or PR-merged branch.
          ok "$(basename "$wt")  ${C_DIM}$WT_WHY${C_0}"
          [ "$WT_VERDICT" != archivable ] \
            || printf '      %sarchive to %s/%s/%s, then remove%s\n' "$C_DIM" "$(_archive_root)" "$proj" "$(basename "$wt")" "$C_0"
        fi
      else
        total_kept=$((total_kept + 1))
        printf '  %s!%s %-42s %s%s: %s%s\n' "$C_WARN" "$C_0" "$(basename "$wt")" \
          "$C_DIM" "$WT_VERDICT" "$WT_WHY" "$C_0"
        if _wt_build_idle "$proj" "$wt"; then
          bwin="$(_retention_days 7 "${HW_BUILD_OUTPUT_DAYS:-7}")"
          WD_RET_AGE="$WT_BUILD_AGE"; WD_WHY="build output retention: ${WT_BUILD_AGE}d > ${bwin}d"
          if [ "$apply" != 1 ]; then
            build_kb_total=$((build_kb_total + WT_BUILD_KB))
            printf '      %sbuild output of %s idle %sd > %sd, would free %s MB: %s%s\n' "$C_DIM" "$(basename "$wt")" "$WT_BUILD_AGE" "$bwin" "$((WT_BUILD_KB / 1024))" \
              "$(printf '%s' "$WT_BUILD_DIRS" | sed "s|^$wt/||" | tr '\n' ' ')" "$C_0"
          elif _reap_log_ready; then
            build_kb_total=$((build_kb_total + WT_BUILD_KB))
            while IFS= read -r bd; do
              [ -n "$bd" ] || continue
              bkb="$(du -sk "$bd" 2>/dev/null | awk '{print $1 + 0}')"
              _reap_log_retention "$proj" "$bd" "$bkb" && rm -rf "${bd:?}" 2>/dev/null || true
            done <<< "$WT_BUILD_DIRS"
            printf '      %sbuild output of %s idle %sd > %sd, freed %s MB: %s%s\n' "$C_DIM" "$(basename "$wt")" "$WT_BUILD_AGE" "$bwin" "$((WT_BUILD_KB / 1024))" \
              "$(printf '%s' "$WT_BUILD_DIRS" | sed "s|^$wt/||" | tr '\n' ' ')" "$C_0"
          else
            warn "kept the build output of $(basename "$wt") — $(_reap_log_file) cannot be written, and an unlogged removal is the one this log forbids"
          fi
        fi
        if [ "$apply" = 1 ] && _wt_sheddable; then
          shed="$(_wt_shed "$proj" "$wt")" && printf '      %skept, and shed %s%s\n' "$C_DIM" "$shed" "$C_0"
        fi
      fi
    done < <(git -C "$main" worktree list --porcelain 2>/dev/null | rg '^worktree ' | sd '^worktree ' '' | tail -n +2)

    # ── the merged task branches no worktree holds ────────────────────────
    local ob
    while IFS= read -r ob; do
      [ -n "$ob" ] || continue
      total_safe=$((total_safe + 1))
      if [ "$apply" = 1 ]; then
        if _reap_delete_branch "$main" "$ob" "$base"; then
          total_removed=$((total_removed + 1))
          ok "deleted $ob  ${C_DIM}(merged, no worktree)${C_0}"
        else
          warn "kept $ob — git refused a safe delete, so git does not see it as merged"
        fi
      else
        ok "$ob  ${C_DIM}branch merged into $base, no worktree holds it${C_0}"
      fi
    done < <(_reap_orphan_branches "$proj" "$main" "$base")

    # ── second pass: the loose work dirs ──────────────────────────────────
    #
    # SEPARATE PASS, SEPARATE VOCABULARY, ON PURPOSE. It is not a widening of
    # the loop above: the loop above asks git what exists and this one asks the
    # filesystem, the two inventories overlap on the `setup` lane only (where
    # both live under $WORK/setup), and `_workdir_disposition` hands anything
    # git owns straight back with the verdict `worktree` rather than ruling on
    # it a second time. Nothing about how a WORKTREE is classified moves.
    #
    # $WORK/<lane> IS THE WHOLE TREE IT LOOKS AT — the `lane_work_bases` arm
    # that is not a repo's `.worktrees`, which is exactly where a `--here` or a
    # `--worktree none` task lands. No new root, no new lane.
    local wd wd_root="$WORK/$proj" loose_seen=0
    if [ -d "$wd_root" ]; then
      while IFS= read -r wd; do
        [ -n "$wd" ] || continue
        _workdir_disposition "$proj" "$wd"
        [ "$WD_VERDICT" = worktree ] && continue
        loose_seen=$((loose_seen + 1))
        if [ "$WD_VERDICT" = safe ]; then
          total_safe=$((total_safe + 1))
          if [ "$apply" = 1 ]; then
            local ret_kb=""
            if [ -n "$WD_RET_AGE" ]; then
              if ! _reap_log_ready; then
                warn "kept $(basename "$wd") — it is past retention but $(_reap_log_file) cannot be written, and a removal by retention is never unlogged"
                continue
              fi
              ret_kb="$(du -sk "$wd" 2>/dev/null | awk '{ print $1 + 0 }')"
            fi
            if _reap_rm_workdir "$wd"; then
              total_removed=$((total_removed + 1))
              [ -z "$WD_RET_AGE" ] || _reap_log_retention "$proj" "$wd" "${ret_kb:-0}" \
                || warn "removed $wd by retention but could not append to $(_reap_log_file) — record it by hand: ${WD_RET_AGE}d, ${ret_kb:-0}KB, $WD_WHY"
              ok "removed $(basename "$wd")  ${C_DIM}(loose work dir${WD_RET_AGE:+, $WD_WHY})${C_0}"
            else
              warn "could not remove $wd — it read as safe and the delete failed, which is the one outcome worse than keeping it. Run it by hand and read what rm says."
            fi
          else
            ok "$(basename "$wd")  ${C_DIM}loose work dir: $WD_WHY${C_0}"
          fi
        else
          total_kept=$((total_kept + 1))
          printf '  %s!%s %-42s %s%s: %s%s\n' "$C_WARN" "$C_0" "$(basename "$wd")" \
            "$C_DIM" "$WD_VERDICT" "$WD_WHY" "$C_0"
        fi
      done < <(fd -H -t d -d 1 . "$wd_root" 2>/dev/null | sd '/$' '' | sort)
    fi
    [ "$loose_seen" != 0 ] || info "no loose work dirs under $wd_root"

    # ── the databases hw recorded whose worktree is gone ──────────────────
    local lf ldb lwt keep_rows=""
    lf="$(_db_ledger "$proj")"
    if [ -n "$(lane_get "$proj" db_provisioned)" ] && [ -r "$lf" ]; then
      while IFS=$'\t' read -r ldb lwt; do
        [ -n "$ldb" ] && [ -n "$lwt" ] || continue
        if [ -d "$lwt" ]; then keep_rows="$keep_rows$ldb"$'\t'"$lwt"$'\n'; continue; fi
        if ! psql -U "$(id -un)" -d postgres -Atc "select 1 from pg_database where datname = '$ldb'" 2>/dev/null | grep -q 1; then
          continue   # already gone: the row goes too
        fi
        if [ "$ldb" != "$(basename "$lwt" | tr '\-/' '__')" ]; then
          warn "kept database $ldb — recorded for $lwt, whose name does not derive it; drop it by hand if it is disposable"
          keep_rows="$keep_rows$ldb"$'\t'"$lwt"$'\n'; continue
        fi
        total_safe=$((total_safe + 1))
        if [ "$apply" = 1 ]; then
          _reap_db "$proj" "$main" "$lwt" ""
          if psql -U "$(id -un)" -d postgres -Atc "select 1 from pg_database where datname = '$ldb'" 2>/dev/null | grep -q 1; then
            keep_rows="$keep_rows$ldb"$'\t'"$lwt"$'\n'
          else
            total_removed=$((total_removed + 1))
          fi
        else
          ok "database $ldb  ${C_DIM}its worktree $(basename "$lwt") is gone; pg_dump into the archive, then drop${C_0}"
          keep_rows="$keep_rows$ldb"$'\t'"$lwt"$'\n'
        fi
      done < "$lf"
      [ "$apply" != 1 ] || printf '%s' "$keep_rows" > "$lf" 2>/dev/null || true
    fi

    # ── build output: the weight `df` actually gets back ──────────────────
    # `.next`/`.turbo` only: node_modules on APFS is clones,
    # so `du` over the tree overstates the cost ~2x. Cached for `hw status`.
    local broot bkb=0   # =0: a bare `local` again does not reset the last lane's value
    if broot="$(_wt_canonical_root "$proj")" && [ -d "$broot" ]; then
      bkb="$(fd -H -I -t d -d 4 -E node_modules '^\.(next|turbo)$' "$broot" 2>/dev/null \
        | while IFS= read -r d; do du -sk "$d" 2>/dev/null; done | awk '{ s += $1 } END { print s + 0 }')"
      info "build output (.next/.turbo) under $broot: $(( ${bkb:-0} / 1048576 )) GB"
    fi
    # What `hw status` reads instead of repeating this survey: <build KB>
    # <epoch> <disposable>. The lane's own disposable count, so only the lane
    # that has something to reap says so.
    printf '%s %s %s\n' "${bkb:-0}" "$(date +%s)" "$(( total_safe - lane_safe0 - (total_removed - lane_removed0) ))" > "$BRAIN/.hw-reap-$proj" 2>/dev/null || true
  done < <(_repo_lanes)

  # ── third pass: executors that reported BLOCKED and are still waiting ──
  #
  # done-invoker keeps a blocked executor's pane so `hw ruling` can resume the
  # same session. One nobody answers is closed here, once it has waited
  # HW_BLOCKED_WAIT_HOURS (default 24) — never before, whatever --apply says.
  local rd mk left bproj btask bpane seen_b=0 hours
  hours="$(_blocked_wait_hours)"
  while IFS= read -r rd; do
    [ -n "$rd" ] || continue
    mk="$(_run_blocked_marker "$rd")"
    [ -f "$mk" ] || continue
    bproj="$(_blocked_field "$mk" project)"; btask="$(_blocked_field "$mk" task)"; bpane="$(_blocked_field "$mk" pane)"
    [ -n "$bproj" ] && [ -n "$btask" ] || continue
    [ -z "$proj_filter" ] || [ "$proj_filter" = "$bproj" ] || continue
    if [ "$seen_b" = 0 ]; then
      printf '\n  %sblocked, waiting for a ruling%s  (closed after %sh unanswered — HW_BLOCKED_WAIT_HOURS)\n' "$C_B" "$C_0" "$hours"
      seen_b=1
    fi
    left="$(_blocked_seconds_left "$mk")"
    if [ "$left" -gt 0 ]; then
      printf '  %s·%s %-42s %swaiting, %dh%02dm left — hw ruling %s "<the unblock>" resumes it%s\n' \
        "$C_DIM" "$C_0" "$bproj:$btask" "$C_DIM" "$((left / 3600))" "$((left % 3600 / 60))" "${bpane:-<pane>}" "$C_0"
    elif [ "$apply" = 1 ]; then
      if "$HW_BIN_DIR/hw" done "$bproj" "$btask" --blocked; then
        ok "closed $bproj:$btask — reported BLOCKED and unanswered for over ${hours}h"
      else
        warn "could not close $bproj:$btask (blocked, unanswered for over ${hours}h) — run hw done $bproj $btask --blocked by hand and read what it says"
      fi
    else
      ok "$bproj:$btask  ${C_DIM}reported BLOCKED, unanswered for over ${hours}h${C_0}"
      printf '      %shw done %s %s --blocked%s\n' "$C_DIM" "$bproj" "$btask" "$C_0"
    fi
  done < <(_hw_run_dirs --every-lane)


  printf '\n'
  [ "$build_kb_total" = 0 ] || info "build output of idle kept worktrees: $((build_kb_total / 1024)) MB $([ "$apply" = 1 ] && echo freed || echo "would be freed by --apply") (build_output_days)"
  if [ "$apply" = 1 ]; then
    ok "$total_removed removed, $total_kept kept$([ "$total_removed" = "$total_safe" ] || printf ' — %s disposable could not be removed, read the lines above' "$((total_safe - total_removed))")"
  else
    info "$total_safe safe to remove, $total_kept kept. Add --apply to remove the safe ones."
    [ "$total_kept" = 0 ] || info "nothing kept is ever removed by --apply, whatever the reason says"
    [ "$total_kept" = 0 ] || info "an ignored path that IS disposable here: HW_WT_REGENERABLE='<name> <name>' hw reap"
  fi
}

# One line per removal by retention, in a log under the brain that git ignores
# (`.hw-reap-*`): ISO time, lane, path, task, age, size in KB, reason. Tab
# separated. HW_REAP_LOG moves it. `_reap_log_ready` is asked BEFORE the delete:
# an unlogged removal is the one this log exists to forbid.
_reap_log_file() { printf '%s' "${HW_REAP_LOG:-$BRAIN/.hw-reap-retention.log}"; }
_reap_log_ready() { : >> "$(_reap_log_file)" 2>/dev/null; }
_reap_log_retention() {  # <lane> <path> <size KB>   (age and reason from WD_*)
  printf '%s\t%s\t%s\t%s\t%sd\t%sKB\t%s\n' "$(date +%Y-%m-%dT%H:%M:%S%z)" "$1" "$2" "$(basename "$2")" \
    "$WD_RET_AGE" "$3" "$WD_WHY" >> "$(_reap_log_file)"
}

# Remove a loose work dir, read-only candidates included.
#
# THE GOTCHA, MEASURED on the same 2026-09-21 sweep: eight of the 259 refused
# `rm -rf` with Permission denied. `setup/mutation-coverage` leaves its
# candidates mode 0444 and their directories without `u+w`, so the parent cannot
# unlink them. A verdict of `safe` that then fails to delete is worse than a
# retention: it is a promise the tool did not keep, and nothing tells the
# operator which of the two happened. So the chmod is part of the removal, and
# the removal still reports its own failure rather than assuming success.
_reap_rm_workdir() {  # <dir>
  local dir="$1"
  chmod -R u+w "$dir" >/dev/null 2>&1 || true
  rm -rf "$dir" 2>/dev/null || return 1
  [ ! -e "$dir" ] || return 1
  return 0
}
