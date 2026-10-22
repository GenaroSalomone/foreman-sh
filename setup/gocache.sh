# THE GO BUILD CACHE OF THE TEST SUITE — where it lives, what a mutant may write into it, and when it is trimmed.
# Sourced by tests/_common.sh (every subject) and by test-hw (the end of a full run). Held by tests/1004.
#
# MEASURED 2026-10-15: ~/.cache/hw-test-501/hw-go-cache was 168 GB. Every mutation anchor builds a mutated copy of
# core/; that build writes ~19 MB (the compiled package and the linked binary, 6 entries, 8-9 s) under a key no
# other build ever asks for again, and Go trims only entries unused for 5 days. A full run builds ~300 mutants.
#
# So the mutants do not write into the shared cache. A subject that builds mutants calls `gocache_seed_mutants`
# right after its control build (the shared cache is warm there: std, deps, the control's own objects) and builds
# each mutant with GOCACHE="${MUT_GOCACHE:-$GOCACHE}". The seed is a clone (APFS clonefile, no bytes copied until
# written) inside the subject's own $TMP, so the mutants still hit every unmutated package and the whole lot is
# removed with the subject. The shared cache then only grows by what is built from the real tree.
#
# The shared cache is also capped. EVERY run trims it at its START (`gocache_trim_start`, below: the oldest entries go
# until it is under the cap, an entry in use never does); at the end of a FULL run `gocache_trim` empties it when it is over the cap, and
# only when no other run holds the machine (the pool of test-hw). The default directory has its own name
# (`hw-go-cache.v2`) so that the old, enormous one is never cloned; `gocache_trim` removes it the first time the
# machine is quiet.

# gocache_default — the shared cache directory when the caller did not set GOCACHE. Subjects run with TMPDIR already
# moved off /var/folders (tests/_common.sh), test-hw does not: the rule is repeated here, so the two agree.
gocache_default() {
  local root="${TMPDIR:-/tmp}" safe
  if [ "$(uname -s)" = Darwin ]; then
    case "$(cd -P "$root" 2>/dev/null && pwd)" in
      /private/var/folders/*/T|/private/var/folders/*/T/*)
        safe="${HW_TEST_TMPROOT:-${XDG_CACHE_HOME:-$HOME/.cache}/hw-test-$(id -u)}"
        if mkdir -p "$safe" 2>/dev/null && chmod 700 "$safe" 2>/dev/null && [ -w "$safe" ]; then root="$safe"; fi ;;
    esac
  fi
  printf '%s/hw-go-cache.v2' "$root"
}

# gocache_seed_plain <src> <dest> — the seed where `cp -c` (a copy-on-write clone) is not there: GNU cp has no -c, so the
# copy is real, and a real copy of a cache of tens of GB (a full run on Linux leaves 15-20 GB, over its cap, until the trim
# at the start of the next run) outlasted the 125 s ceiling of 960 and 962 — measured 2026-10-18 in the Linux container:
# `cp -cR` dies with "invalid option -- 'c'" and `cp -R` then copies the whole cache. A cold build of core/ costs ~5 s, so
# over HW_TEST_MUTSEED_MAX_MB (default 1024) the mutant gets an empty cache instead and builds what it needs.
gocache_seed_plain() {
  local kb; kb="$(du -sk "$1" 2>/dev/null | cut -f1)"
  case "$kb" in ''|*[!0-9]*) return 0 ;; esac
  [ "$kb" -le $((${HW_TEST_MUTSEED_MAX_MB:-1024} * 1024)) ] || return 0   # MUTATION-ANCHOR: 1004-M14
  cp -R "$1/." "$2/" 2>/dev/null || true
}

# gocache_seed_mutants <dir> — MUT_GOCACHE, a throwaway copy of the shared cache for the mutants of one subject.
gocache_seed_mutants() {
  local dest="$1" src="${GOCACHE:-${HW_GOCACHE_DEFAULT:-}}"
  mkdir -p "$dest" || return 0
  if [ -d "$src" ]; then
    cp -cR "$src/." "$dest/" 2>/dev/null || gocache_seed_plain "$src" "$dest"   # MUTATION-ANCHOR: 1004-M01
  fi
  MUT_GOCACHE="$dest"; export MUT_GOCACHE
}

# gocache_trim_mutants <dir> <cap-mb> — the mutant test binaries (core/mutant_cache_test.go; setup/go-test-core.sh puts them beside the go cache) are
# ~11 MB each and a full run keeps ~300-500: over <cap-mb> the least recently used go first (a hit refreshes the mtime). Unlike the go cache, an
# entry in use may go at any time: a binary is copied out whole, and an open file survives its removal. Never fails.
gocache_trim_mutants() {
  local dir="$1" cap="$2" list total over
  case "$cap" in ''|*[!0-9]*) cap=4096 ;; esac
  [ -d "$dir" ] || return 0
  # Sizes are the files' own, not `du -sk` of the directory: on Linux du also counts the directory's own block (4 KB),
  # which APFS does not, so five 1 MB files over a 3 MB cap read as 2.004 MB over and a third file went (1103a, MEASURED Linux 2026-10-22).
  # GNU stat first: it reads BSD's -f as --file-system and exits 0.
  list="$({ stat -c '%Y %s %n' /dev/null >/dev/null 2>&1 && find "$dir" -type f -exec stat -c '%Y %s %n' {} + 2>/dev/null \
      || find "$dir" -type f -exec stat -f '%m %z %N' {} + 2>/dev/null; } | sort -n)" || list=""
  total="$(printf '%s\n' "$list" | awk '{ t += $2 } END { print t + 0 }')"
  over=$(( total - cap * 1024 * 1024 ))
  [ "$over" -gt 0 ] || return 0
  printf '%s\n' "$list" \
    | awk -v need="$over" '{ if (got >= need) exit; got += $2; sub(/^[0-9]+ [0-9]+ /, ""); printf "%s%c", $0, 0 }' \
    | xargs -0 rm -f 2>/dev/null || true
  printf 'runner: mutant binaries %s were %s MB, over their %s MB cap — least recently used removed\n' "$dir" "$((total / 1048576))" "$cap" >&2
  return 0
}

# gocache_pool_busy <pool-dir> — 0 when another runner holds a slot, the exclusive, or waits for one. A holder that
# cannot be read counts as alive: the cost of a wrong "busy" is one skipped trim, the cost of a wrong "idle" is a
# cache emptied under a build.
gocache_pool_busy() {
  local d pid
  for d in "$1"/slot.* "$1"/exclusive "$1"/wait.*; do
    [ -e "$d" ] || continue
    case "$d" in *.part) continue ;; esac
    if [ -d "$d" ]; then pid="$(cat "$d/owner.pid" 2>/dev/null || true)"; else pid="${d##*.}"; fi
    case "$pid" in ''|*[!0-9]*) return 0 ;; esac
    [ "$pid" = "$$" ] && continue
    kill -0 "$pid" 2>/dev/null && return 0   # MUTATION-ANCHOR: 1004-M02
  done
  return 1
}

# gocache_trim <cache> <cap-mb> <pool-dir> [<old-dir>…] — empty <cache> when it is over <cap-mb>, and remove the
# <old-dir>s; both only while nobody else is running. Says what it did (or why it did not) on stderr; never fails.
gocache_trim() {
  local cache="$1" cap="$2" pool="$3" old kb; shift 3
  case "$cap" in ''|*[!0-9]*) cap=4096 ;; esac
  if [ -n "$pool" ] && gocache_pool_busy "$pool"; then   # MUTATION-ANCHOR: 1004-M03
    printf 'runner: go cache not trimmed — another run holds the machine\n' >&2; return 0
  fi
  for old in "$@"; do
    [ -d "$old" ] && [ "$old" != "$cache" ] || continue
    printf 'runner: removing the old go cache %s (replaced by %s)\n' "$old" "$cache" >&2
    ( rm -rf "$old" >/dev/null 2>&1 & )
  done
  gocache_trim_mutants "$(dirname "$cache")/hw-mutant-bins.v1" "${HW_TEST_MUTCACHE_CAP_MB:-4096}"
  [ -d "$cache" ] || return 0
  kb="$(du -sk "$cache" 2>/dev/null | cut -f1)" || kb=""   # an unreadable entry must not abort an EXIT trap under set -e/pipefail # MUTATION-ANCHOR: 1004-M06
  case "$kb" in ''|*[!0-9]*) return 0 ;; esac
  if [ "$kb" -gt $((cap * 1024)) ]; then   # MUTATION-ANCHOR: 1004-M04
    printf 'runner: go cache %s is %s MB, over its %s MB cap — emptied\n' "$cache" "$((kb / 1024))" "$cap" >&2
    GOCACHE="$cache" go clean -cache >/dev/null 2>&1 || rm -rf "$cache" 2>/dev/null || true
  fi
  return 0
}

# gocache_trim_start <cache> <cap-mb> — EVERY run, at its start, brings <cache> under <cap-mb> by deleting its OLDEST
# entries (mtime). Go does NOT touch an entry each time a build uses it: cmd/go refreshes an entry's mtime only when
# it is more than an hour old (mtimeInterval = 1h in cmd/go/internal/cache), so an entry in constant use can show an
# mtime up to 60 minutes old. MEASURED 2026-10-17: a threshold of 10 minutes deleted the stdlib entries a running
# build was importing ("could not import os (open …/hw-go-cache.v2/15/…-d: no such file or directory)", m844-1/7/8,
# m848-1). So the tiers below say "unused for N minutes" and the find threshold is N plus Go's 60-minute mtime
# granularity (GOCACHE_MTIME_SLACK_MIN): an entry whose mtime is younger than 60+N minutes may be in use.
# MEASURED 2026-10-16: the end-of-full-run trim above almost never fires (a whole quiet full run is rare), so
# hw-go-cache.v2 reached 192 GB and filled the disk. Entries unused for GOCACHE_OLD_MIN (60) minutes go first; only
# if that is not enough, those unused for GOCACHE_USED_MIN (10); anything younger is in use and is never deleted (the
# cap is then soft). One trimmer at a time (a lock directory) and at most one look a minute (a stamp beside the
# cache, so the ~40 go-test-core partitions of a gate do not each walk it). Never fails. Held by tests/1004
# (S-claims `start:*`).
gocache_trim_start() {
  local cache="$1" cap="$2" stamp lock kb over tier min order=-n   # MUTATION-ANCHOR: 1004-M09
  case "$cap" in ''|*[!0-9]*) cap=4096 ;; esac
  [ -d "$cache" ] || return 0
  stamp="$cache.trimmed"; lock="$cache.trimming"
  if [ -f "$stamp" ] && [ -z "$(find "$stamp" -mmin +1 2>/dev/null)" ]; then return 0; fi
  if ! mkdir "$lock" 2>/dev/null; then
    [ -z "$(find "$lock" -maxdepth 0 -mmin +30 2>/dev/null)" ] || rmdir "$lock" 2>/dev/null || true   # a dead trimmer's lock
    return 0
  fi
  : > "$stamp" 2>/dev/null || true
  gocache_trim_mutants "$(dirname "$cache")/hw-mutant-bins.v1" "${HW_TEST_MUTCACHE_CAP_MB:-4096}"   # trimmed with the cache, on the same look
  kb="$(du -sk "$cache" 2>/dev/null | cut -f1)" || kb=""
  case "$kb" in ''|*[!0-9]*) rmdir "$lock" 2>/dev/null || true; return 0 ;; esac
  if [ "$kb" -gt $((cap * 1024)) ]; then   # MUTATION-ANCHOR: 1004-M08
    printf 'runner: go cache %s is %s MB, over its %s MB cap — deleting its oldest entries\n' "$cache" "$((kb / 1024))" "$cap" >&2
    for tier in "${GOCACHE_OLD_MIN:-60}" "${GOCACHE_USED_MIN:-10}"; do   # MUTATION-ANCHOR: 1004-M10
      over=$(( (kb - cap * 1024) * 1024 ))
      min=$((tier + ${GOCACHE_MTIME_SLACK_MIN:-60}))   # MUTATION-ANCHOR: 1004-M13
      # GNU stat first: it reads BSD's -f as --file-system and exits 0, so asking -f first fed the sort garbage on Linux
      { stat -c '%Y %s %n' /dev/null >/dev/null 2>&1 && find "$cache" -type f -mmin "+$min" -exec stat -c '%Y %s %n' {} + 2>/dev/null \
          || find "$cache" -type f -mmin "+$min" -exec stat -f '%m %z %N' {} + 2>/dev/null; } \
        | sort "$order" \
        | awk -v need="$over" '{ if (got >= need) exit; got += $2; sub(/^[0-9]+ [0-9]+ /, ""); printf "%s%c", $0, 0 }' \
        | xargs -0 rm -f 2>/dev/null || true
      kb="$(du -sk "$cache" 2>/dev/null | cut -f1)" || kb=""
      case "$kb" in ''|*[!0-9]*) break ;; esac
      [ "$kb" -gt $((cap * 1024)) ] || break
    done
  fi
  rmdir "$lock" 2>/dev/null || true
  return 0
}
