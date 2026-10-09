# shellcheck shell=bash
# shellcheck disable=SC2034  # DEPS_* are read by whoever sourced this file
# The version floors of the outside tools, and the one check against them.
#
# Sourced by bin/hw (`hw preflight`, gentle-ai's pin), bin/brain (the cockpit's
# claude floor) and install.sh (--check). The numbers live in deps.conf beside this
# file; nothing else in the tree keeps one. Bash 3.2 (macOS): no associative
# arrays, no ${x,,}.
#
#   deps_row <name>             DEPS_MIN DEPS_MATCH DEPS_CMD DEPS_NEEDS DEPS_BASIS DEPS_UPDATE
#   deps_names                  every tool in the manifest, one per line
#   deps_version_at_least A B   A >= B, numerically, part by part
#   deps_status <name> [<bin>]  DEPS_STATE = ok | below | absent | unknown, DEPS_VERSION
#   deps_message <name>         the sentence for a state that is not ok
#
# THE VERSION IS CACHED by the binary's path, mtime and size
# ($XDG_CACHE_HOME/hw/deps/<name>): asking a tool its version costs a process, and
# `hw` should not pay one per tool per command. A replaced binary has another
# mtime or size, so it is asked again; the FLOOR is never cached, it is compared
# on every read, so editing deps.conf takes effect at once. DEPS_NO_CACHE=1 turns the
# cache off (install.sh --check, which writes nothing).

_DEPS_DIR="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPS_CONF="${HW_DEPS_CONF:-$_DEPS_DIR/deps.conf}"

deps_names() {
  local n rest
  [ -r "$DEPS_CONF" ] || return 1
  while IFS='|' read -r n rest; do
    case "$n" in ''|'#'*) continue ;; esac
    printf '%s\n' "$n"
  done < "$DEPS_CONF"
}

deps_row() {
  local n
  DEPS_MIN="" DEPS_MATCH="" DEPS_CMD="" DEPS_NEEDS="" DEPS_BASIS="" DEPS_UPDATE=""
  [ -r "$DEPS_CONF" ] || return 1
  while IFS='|' read -r n DEPS_MIN DEPS_MATCH DEPS_CMD DEPS_NEEDS DEPS_BASIS DEPS_UPDATE; do
    case "$n" in ''|'#'*) continue ;; esac
    [ "$n" = "$1" ] && return 0
  done < "$DEPS_CONF"
  DEPS_MIN="" DEPS_MATCH="" DEPS_CMD="" DEPS_NEEDS="" DEPS_BASIS="" DEPS_UPDATE=""
  return 1
}

# <a> <b>: a >= b, three numeric parts, a missing part is 0. NOT a string
# comparison: 2.1.289 is newer than 2.1.30.
deps_version_at_least() {
  local a b i x y
  IFS=. read -r -a a <<< "$1"
  IFS=. read -r -a b <<< "$2"
  for i in 0 1 2; do
    x="${a[i]:-0}"; y="${b[i]:-0}"
    [ "$((10#$x))" -gt "$((10#$y))" ] && return 0
    [ "$((10#$x))" -lt "$((10#$y))" ] && return 1
  done
  return 0
}

# The first dotted version in a tool's output: "2.1.294 (Claude Code)", "v22.19.0",
# "jq-1.7.1-apple", "herdr 0.9.1". Prints nothing when there is none.
deps_parse_version() {
  local re='([0-9]+\.[0-9]+(\.[0-9]+)?)'
  [[ "$1" =~ $re ]] && printf '%s' "${BASH_REMATCH[1]}"
  return 0
}

# What a replaced binary changes: mtime and size (npm normalises the mtime of the
# files it unpacks, so the size is the second witness). Prints nothing, and fails,
# for a path stat cannot read.
_deps_stat_key() {
  stat -Lc '%Y-%s' "$1" 2>/dev/null || stat -Lf '%m-%z' "$1" 2>/dev/null
}

# <name> [<binary>]: sets DEPS_VERSION and DEPS_STATE, and returns 0 only for ok.
#   absent   the binary is not there (no PATH entry, no such file); not a violation
#   unknown  the binary answered and no version could be read from the answer
#   below    older than the floor (match=min) or not the pinned one (match=exact)
deps_status() {
  local name="$1" bin="${2:-}" cmd0 key cdir cfile c_bin c_key c_cmd c_ver out ver tab
  DEPS_VERSION="" DEPS_STATE=unknown
  deps_row "$name" || { DEPS_STATE=unknown; return 2; }
  # shellcheck disable=SC2086  # the command is a word list on purpose
  set -- $DEPS_CMD
  cmd0="$1"; shift
  [ -n "$bin" ] || bin="$(type -P "$cmd0" 2>/dev/null || true)"
  if [ -z "$bin" ] || [ ! -x "$bin" ]; then DEPS_STATE=absent; return 1; fi
  tab="$(printf '\t')"
  key="$(_deps_stat_key "$bin" || true)"
  cdir="${XDG_CACHE_HOME:-${HOME:-/nonexistent}/.cache}/hw/deps"
  cfile="$cdir/$name"
  ver=""
  [ -z "${DEPS_NO_CACHE:-}" ] || key=""   # no key: nothing read, nothing written
  if [ -n "$key" ] && [ -r "$cfile" ]; then
    IFS="$tab" read -r c_bin c_key c_cmd c_ver < "$cfile" || true
    [ "$c_bin" = "$bin" ] && [ "$c_key" = "$key" ] && [ "$c_cmd" = "$DEPS_CMD" ] && ver="$c_ver"
  fi
  if [ -z "$ver" ]; then
    # </dev/null: a tool that reads stdin must not wait on the caller's. The ceiling
    # is only there where `timeout` is (stock macOS has none).
    if type -P timeout >/dev/null 2>&1; then out="$(timeout 10 "$bin" "$@" 2>&1 </dev/null | head -5 || true)"
    else out="$("$bin" "$@" 2>&1 </dev/null | head -5 || true)"; fi
    ver="$(deps_parse_version "$out")"
    if [ -z "$ver" ]; then DEPS_STATE=unknown; return 2; fi
    if [ -n "$key" ] && mkdir -p "$cdir" 2>/dev/null; then
      printf '%s\t%s\t%s\t%s\n' "$bin" "$key" "$DEPS_CMD" "$ver" > "$cfile.$$" 2>/dev/null \
        && mv "$cfile.$$" "$cfile" 2>/dev/null || rm -f "$cfile.$$" 2>/dev/null || true
    fi
  fi
  DEPS_VERSION="$ver"
  if [ "$DEPS_MATCH" = exact ]; then
    if deps_version_at_least "$ver" "$DEPS_MIN" && deps_version_at_least "$DEPS_MIN" "$ver"; then DEPS_STATE=ok; return 0; fi
  elif deps_version_at_least "$ver" "$DEPS_MIN"; then DEPS_STATE=ok; return 0; fi
  DEPS_STATE=below
  return 1
}

# The sentence for the state deps_status left (after it ran for this name).
deps_message() {
  case "$DEPS_STATE" in
    below)
      if [ "$DEPS_MATCH" = exact ]; then
        printf '%s %s is not the pinned %s; fix: %s' "$1" "$DEPS_VERSION" "$DEPS_MIN" "$DEPS_UPDATE"
      else
        printf '%s %s is below the floor %s; update: %s' "$1" "$DEPS_VERSION" "$DEPS_MIN" "$DEPS_UPDATE"
      fi ;;
    unknown) printf "could not read %s's version from \`%s\`" "$1" "$DEPS_CMD" ;;
    absent)  printf '%s is not installed' "$1" ;;
    *)       printf '%s %s (>= %s)' "$1" "$DEPS_VERSION" "$DEPS_MIN" ;;
  esac
}
