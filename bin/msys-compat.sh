# shellcheck shell=bash
# msys-compat.sh — sourced by every entry point in bin/, by install.sh and by the
# setup suite, on Windows only (OSTYPE msys/cygwin). It changes nothing else.
#
# Git Bash runs native Windows programs — Python, jq, fd, git — and msys
# rewrites every argument and environment value that looks like a path on its
# way into one: `/tmp/x` reaches it as `C:/…/AppData/Local/Temp/x`
# (measured on windows-latest). What the program prints back is in THAT
# spelling, with `\r\n` line ends, so a path bash wrote came back as a different
# string. This file is the one place that conversion is undone:
#
#   - Python: bin/sitecustomize.py, put on PYTHONPATH here, converts at
#     Python's own boundary (argv, environment, open/stat, child processes)
#     and writes LF.
#   - jq, fd, git: the functions below. jq gets -b (LF) and its --arg values
#     unconverted; fd's and git's paths come back in bash's spelling.
#
# Both halves read one mount table, msys's own (`cygpath -m`), exported once.
# A Cygwin shell (no MSYSTEM) has POSIX tools and gets only the first block.

case " ${MSYS:-} " in *" winsymlinks:nativestrict "*) ;; *) export MSYS="winsymlinks:nativestrict${MSYS:+ $MSYS}" ;; esac
export PYTHONUTF8=1
# bin/ is on PYTHONPATH (sitecustomize.py): no __pycache__ written into it.
export PYTHONDONTWRITEBYTECODE=1
[ -n "${MSYSTEM:-}" ] || return 0

: "${HW_MSYS_ROOT:=$(cygpath -m /)}" "${HW_MSYS_TMP:=$(cygpath -m /tmp)}" "${HW_MSYS_TMP_LONG:=$(cygpath -m -l /tmp)}"
export HW_MSYS_ROOT HW_MSYS_TMP HW_MSYS_TMP_LONG
_hw_msys_py="$(cygpath -w "$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)")"
case ";${PYTHONPATH:-};" in *";$_hw_msys_py;"*) ;; *) export PYTHONPATH="$_hw_msys_py${PYTHONPATH:+;$PYTHONPATH}" ;; esac
unset _hw_msys_py
# The programs the functions below stand in front of, as PATH found them the
# first time this ran (exported, so a child that sources this again keeps them).
# A test that puts its own `ps` or `jq` first on PATH must get IT: a function
# outranks every PATH entry, so each function defers to anything that is not
# the program recorded here.
for _hw_t in jq fd git ps pgrep; do
  eval ': "${HW_MSYS_REAL_'"$_hw_t"'=$(type -P '"$_hw_t"' || true)}"; export HW_MSYS_REAL_'"$_hw_t"
done
unset _hw_t
_msys_stubbed() {  # NAME — true when PATH now finds another NAME than the recorded one
  local now real
  now="$(type -P "$1" || true)"; eval "real=\${HW_MSYS_REAL_$1:-}"
  # Identity, not spelling: /bin/ps and /usr/bin/ps are one file under Git Bash,
  # and a PATH that finds the other spelling is not a stub.
  [ -n "$now" ] && [ "$now" != "$real" ] && ! [ "$now" -ef "$real" ]
}

# _msys_w PATH — sets REPLY to Windows' spelling of bash's (forward slashes).
_msys_w() {
  REPLY="$1"
  case "$1" in
    //*|"") ;;
    /[a-zA-Z]) REPLY="${1:1:1}"; REPLY="${REPLY^^}:/" ;;
    /[a-zA-Z]/*) REPLY="${1:1:1}"; REPLY="${REPLY^^}:${1:2}" ;;
    /tmp|/tmp/*) REPLY="${HW_MSYS_TMP%/}${1#/tmp}" ;;
    /*) REPLY="${HW_MSYS_ROOT%/}$1" ;;
  esac
}

# _msys_u PATH — sets REPLY to bash's spelling of a Windows one (the inverse).
_msys_u() {
  REPLY="$1"
  case "$1" in [A-Za-z]:[/\\]*|[A-Za-z]:) ;; *) return 0 ;; esac
  local q="${1//\\//}" low t
  low="${q,,}"
  for t in "${HW_MSYS_TMP%/}" "${HW_MSYS_TMP_LONG%/}"; do
    [ -n "$t" ] || continue
    case "$low/" in "${t,,}/"*) REPLY="/tmp${q:${#t}}"; return 0 ;; esac
  done
  t="${HW_MSYS_ROOT%/}"
  if [ -n "$t" ]; then
    case "$low/" in "${t,,}/"*) REPLY="${q:${#t}}"; REPLY="${REPLY:-/}"; return 0 ;; esac
  fi
  REPLY="${q:0:1}"; REPLY="/${REPLY,,}${q:2}"
  [ "${#q}" -gt 3 ] || REPLY="${REPLY%/}"
}

# jq: LF output (-b), and only the operands that are files converted — a
# `--arg NAME VALUE` reaches jq exactly as written.
jq() {
  if _msys_stubbed jq; then command jq "$@"; return; fi
  local a=() x filter=0 values=0
  while [ $# -gt 0 ]; do
    x="$1"; shift
    if [ "$values" = 1 ] && [ "$filter" = 1 ]; then a+=("$x"); continue; fi
    case "$x" in
      --arg|--argjson) a+=("$x" "${1-}" "${2-}"); shift 2 || shift $# ;;
      --slurpfile|--rawfile) _msys_w "${2-}"; a+=("$x" "${1-}" "$REPLY"); shift 2 || shift $# ;;
      -f|--from-file) _msys_w "${1-}"; a+=("$x" "$REPLY"); filter=1; shift || true ;;
      -L|--indent) _msys_w "${1-}"; a+=("$x" "$REPLY"); shift || true ;;
      --args|--jsonargs) a+=("$x"); values=1 ;;
      --) a+=("$x") ;;
      -*) a+=("$x") ;;
      *) if [ "$filter" = 0 ]; then filter=1; a+=("$x"); else _msys_w "$x"; a+=("$REPLY"); fi ;;
    esac
  done
  MSYS2_ARG_CONV_EXCL='*' command jq -b "${a[@]}"
}

# _msys_u_lines — every line of stdin that is a Windows path, in bash's spelling.
_msys_u_lines() {
  local l
  while IFS= read -r l || [ -n "$l" ]; do _msys_u "$l"; printf '%s\n' "$REPLY"; done
}

# fd prints the root it was given, and msys gave it the Windows spelling.
fd() {
  if _msys_stubbed fd; then command fd "$@"; return; fi
  local o rc=0 x
  for x in "$@"; do case "$x" in -0|--print0|-x|--exec|-X|--exec-batch) command fd "$@"; return ;; esac; done
  o="$(command fd "$@")" || rc=$?
  [ -z "$o" ] || _msys_u_lines <<<"$o"
  return "$rc"
}

# git is a native program too: its paths (rev-parse, worktree list) come back
# in bash's spelling; every other subcommand is untouched.
git() {
  if _msys_stubbed git; then command git "$@"; return; fi
  local x sub="" skip=0
  for x in "$@"; do
    if [ "$skip" = 1 ]; then skip=0; continue; fi
    case "$x" in
      -C|-c|--git-dir|--work-tree|--namespace|--exec-path|--config-env) skip=1 ;;
      -*) ;;
      *) sub="$x"; break ;;
    esac
  done
  case "$sub" in rev-parse|worktree) ;; *) command git "$@"; return ;; esac
  local o rc=0 l
  o="$(command git "$@")" || rc=$?
  [ -n "$o" ] || return "$rc"
  while IFS= read -r l || [ -n "$l" ]; do
    case "$l" in
      "worktree "[A-Za-z]:*) _msys_u "${l#worktree }"; printf 'worktree %s\n' "$REPLY" ;;
      [A-Za-z]:[/\\]*)
        if [[ "$l" =~ ^(.*[^ ])(\ +[0-9a-f]{7,64}( .*)?|\ +\(bare\))$ ]]; then
          _msys_u "${BASH_REMATCH[1]}"; printf '%s%s\n' "$REPLY" "${BASH_REMATCH[2]}"
        else _msys_u "$l"; printf '%s\n' "$REPLY"; fi ;;
      *) printf '%s\n' "$l" ;;
    esac
  done <<<"$o"
  return "$rc"
}

# ps -o lstart=|command= -p PID and pgrep -P PPID | -f TEXT, answered from msys's
# /proc: Git for Windows' ps has no -o and there is no pgrep (measured). Only
# these shapes are what bin/ and the suite ask; anything else is the real ps.
# lstart is an IDENTITY here (the process's start tick), which is all its callers
# compare: a pid that was reused has another one.
_msys_stat() {  # PID — sets REPLY_STAT to the fields after "(comm)"
  local l
  { IFS= read -r l < "/proc/$1/stat"; } 2>/dev/null || return 1
  read -r -a REPLY_STAT <<<"${l##*) }"
}
ps() {
  if _msys_stubbed ps; then command ps "$@"; return; fi
  local fmt="" pid="" a=("$@")
  if [ $# -eq 4 ] && [ "$1" = -o ] && [ "$3" = -p ]; then fmt="$2" pid="$4"
  elif [ $# -eq 4 ] && [ "$1" = -p ] && [ "$3" = -o ]; then fmt="$4" pid="$2"
  fi
  case "$fmt" in
    lstart=) _msys_stat "$pid" || return 1; printf 'msys-start %s\n' "${REPLY_STAT[19]}" ;;
    command=|args=) [ -r "/proc/$pid/cmdline" ] || return 1
      tr '\0' ' ' < "/proc/$pid/cmdline" | sed 's/ $//'; printf '\n' ;;
    *) command ps "${a[@]}" ;;
  esac
}
pgrep() {
  if _msys_stubbed pgrep; then command pgrep "$@"; return; fi
  local d p hit=1 cmd
  case "${1:-}" in
    -P) for d in /proc/[0-9]*; do p="${d#/proc/}"
          _msys_stat "$p" && [ "${REPLY_STAT[1]}" = "$2" ] && { printf '%s\n' "$p"; hit=0; }
        done ;;
    -f) shift; [ "${1:-}" = -- ] && shift
        for d in /proc/[0-9]*; do p="${d#/proc/}"
          [ "$p" = "$$" ] || [ "$p" = "${BASHPID:-}" ] && continue
          cmd="$(tr '\0' ' ' < "$d/cmdline" 2>/dev/null)" || continue
          case "$cmd" in *"$1"*) printf '%s\n' "$p"; hit=0 ;; esac
        done ;;
    *) printf 'pgrep: only -P and -f are provided under Git Bash\n' >&2; return 2 ;;
  esac
  return "$hit"
}

export -f _msys_w _msys_u _msys_u_lines _msys_stubbed jq fd git _msys_stat ps pgrep
