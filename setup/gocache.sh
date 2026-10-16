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
# The shared cache is also capped: at the end of a FULL run `gocache_trim` empties it when it is over the cap, and
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

# gocache_seed_mutants <dir> — MUT_GOCACHE, a throwaway copy of the shared cache for the mutants of one subject.
gocache_seed_mutants() {
  local dest="$1" src="${GOCACHE:-${HW_GOCACHE_DEFAULT:-}}"
  mkdir -p "$dest" || return 0
  if [ -d "$src" ]; then
    cp -cR "$src/." "$dest/" 2>/dev/null || cp -R "$src/." "$dest/" 2>/dev/null || true   # MUTATION-ANCHOR: 1004-M01
  fi
  MUT_GOCACHE="$dest"; export MUT_GOCACHE
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
  [ -d "$cache" ] || return 0
  kb="$(du -sk "$cache" 2>/dev/null | cut -f1)" || kb=""   # an unreadable entry must not abort an EXIT trap under set -e/pipefail # MUTATION-ANCHOR: 1004-M06
  case "$kb" in ''|*[!0-9]*) return 0 ;; esac
  if [ "$kb" -gt $((cap * 1024)) ]; then   # MUTATION-ANCHOR: 1004-M04
    printf 'runner: go cache %s is %s MB, over its %s MB cap — emptied\n' "$cache" "$((kb / 1024))" "$cap" >&2
    GOCACHE="$cache" go clean -cache >/dev/null 2>&1 || rm -rf "$cache" 2>/dev/null || true
  fi
  return 0
}
