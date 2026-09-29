#!/usr/bin/env bash
# The invokers' run-env-file lookup: rooted at $HW_WORKDIR, walking only when
# there is no $HW_WORKDIR to root it at.
#
# Run by ../test-hw as its own bash process. Run it alone while working here:
#
#     bash setup/tests/22-invoker-env-file.sh
#
# WHY THIS FILE EXISTS. `invoker_adopt_env_file` is the function that decides
# whether a pane has a brainer to report to. Run with $HW_INVOKER_PANE unset
# from a cwd inside an executor's own work directory, it can walk up, find a
# DIFFERENT run's env file, re-adopt an invoker pane from it, and deliver a
# real completion report for a task that never ran there. Nothing else in the
# suite catches that.
#
# HERMETIC. No pane, no herdr, no delivery: this drives the shell function
# directly with INVOKER_ENV_ROOTS pointed at $TMP, which is the same mechanism
# the invokers use and none of the machinery around it.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

IC="$ROOT/bin/invoker-common.sh"

# ── 0. THE APOSTROPHE TRAP, one level worse than 08-invoker-pane.sh's.
#       bash 3.2 scans a <( ... ) for its closing paren with quote tracking that
#       does NOT know the <<'ENVFIND' body is inert. Adding apostrophes to
#       comments inside that heredoc can flip the parity of the count and make
#       every call die with
#         invoker-common.sh: line 278: bad substitution: no closing `)'
#       The function is unreachable at that point, so BOTH invokers stop
#       working — and `bash -n` passes, which is why this is asserted on the
#       ASSERTED FIRST, before anything calls the function. When the parse dies
#       the function is simply gone, `adopt` fills nothing, and the two negative
#       assertions below — which expect exactly "nothing" — PASS, naming an
#       unrelated cause further down as the culprit. A precondition belongs
#       above what depends on it.
q="$(sed -n "/<<'ENVFIND'/,/^ENVFIND\$/p" "$IC" | tr -cd "'" | wc -c | tr -d ' ')"
[ -n "$q" ] || fail "invoker env: could not count apostrophes in the ENVFIND heredoc"
if [ $((q % 2)) -eq 0 ]; then
  pass "invoker env: the ENVFIND heredoc body has an even number of apostrophes ($q), so bash 3.2 can still find the closing paren"
else
  fail "invoker env: the ENVFIND heredoc body has $q apostrophes — odd, so bash 3.2 fails the whole function with 'bad substitution: no closing )'"
fi

# The driver. Sources invoker-common.sh the way an invoker does, overrides the
# roots to $TMP so no real work directory is read, calls the function, and
# prints only what it filled in.
cat > "$TMP/adopt.sh" <<'DRIVER'
#!/usr/bin/env bash
set -uo pipefail
INVOKER_PROG=test
INVOKER_BIN_DIR="$(dirname "$1")"
die() { printf 'die: %s\n' "$*"; exit 1; }
. "$1"
INVOKER_ENV_ROOTS="$2
"
invoker_adopt_env_file
printf 'PANE=%s\n' "${HW_INVOKER_PANE:-<unset>}"
printf 'CHAINING=%s\n' "${HW_CHAINING_ENABLED:-<unset>}"
printf 'FILE=%s\n' "${INVOKER_ENV_FILE:-<none>}"
DRIVER

# `mine` is one executor's work directory, `theirs` is another's under the same
# root, and `mine/sub/deeper` is the cwd a `cd` leaves behind — the exact
# geometry of the incident.
mk() { mkdir -p "$TMP/$1/.hw/$2"; printf "HW_INVOKER_PANE='%s'\nHW_TASK='%s'\nHW_CHAINING_ENABLED='%s'\n" "$3" "$1" "${4:-0}" > "$TMP/$1/.hw/$2/env"; }
mk mine   r-mine  wMINE:p1
mk theirs r-their wTHEIRS:p1
mkdir -p "$TMP/mine/sub/deeper" "$TMP/nowhere"

# A work directory OUTSIDE every root, carrying a perfectly good env file. It
# has to be real and readable or assertion 5 below is vacuous: a HW_WORKDIR that
# merely does not exist refuses for the wrong reason, and a mutant that deletes
# the root check survives unnoticed.
OUTSIDE="$(mktemp -d "${TMPDIR:-/tmp}/hw-test-outside.XXXXXX")"
trap 'rm -rf "$TMP" "$OUTSIDE"' EXIT
mkdir -p "$OUTSIDE/.hw/r-out"
printf "HW_INVOKER_PANE='wOUT:p1'\n" > "$OUTSIDE/.hw/r-out/env"

# Every call runs with NO HW_INVOKER_PANE — that is the whole question — and
# with HW_* scrubbed so this file behaves identically for a brainer and for an
# executor whose own dispatch exported them.
adopt() { # adopt <cwd> [VAR=VAL ...]
  local cwd="$1"; shift
  ( cd "$cwd" && env -u HW_INVOKER_PANE -u HW_WORKDIR -u HW_RUN -u HW_TASK \
      -u HW_PROJECT -u HW_ARTIFACTS -u ENGRAM_PROJECT -u HW_EXECUTOR_VENDOR \
      -u HW_INVOKER_VENDOR -u HW_INVOKER_SESSION -u HW_INVOKER_ENDPOINT \
      "$@" bash "$TMP/adopt.sh" "$IC" "$TMP" 2>/dev/null )
}
pane_of() { adopt "$@" | sed -n 's/^PANE=//p'; }
chaining_of() { adopt "$@" | sed -n 's/^CHAINING=//p'; }

# ── 1. the incident: HW_WORKDIR set, cwd inside it, but the named run has no
#       env file. Adopting ANYTHING here is how a task that does not exist gets
#       reported as finished.
got="$(pane_of "$TMP/mine/sub/deeper" HW_WORKDIR="$TMP/mine" HW_RUN=r-probe)"
case "$got" in
  '<unset>') pass "invoker env: a run whose own env file is absent adopts nothing, even standing inside a work directory that has one" ;;
  *) fail "invoker env: adopted '$got' for run r-probe, which has no env file — a report would go to the wrong invoker" ;;
esac

# ── 2. the shape worse than the incident: cwd in a directory with no .hw of its
#       own, so the old walk climbed OUT of it and into a DIFFERENT task's run.
got="$(pane_of "$TMP/nowhere" HW_WORKDIR="$TMP/mine" HW_RUN=r-probe)"
case "$got" in
  '<unset>') pass "invoker env: the walk cannot climb out of \$HW_WORKDIR into another task's run" ;;
  *) fail "invoker env: adopted '$got' from outside \$HW_WORKDIR — the return channel is still cwd-hijackable" ;;
esac

# ── 3. the fallback that is the whole reason the file exists must still work:
#       HW_RUN names this run, its file is there, the pane is missing.
got="$(pane_of "$TMP/nowhere" HW_WORKDIR="$TMP/mine" HW_RUN=r-mine)"
case "$got" in
  wMINE:p1) pass "invoker env: a pane that lost only HW_INVOKER_PANE still recovers it from its OWN run's file" ;;
  *) fail "invoker env: rooting the walk broke the legitimate fallback — got '$got', wanted wMINE:p1" ;;
esac
got="$(chaining_of "$TMP/nowhere" HW_WORKDIR="$TMP/mine" HW_RUN=r-mine)"
[ "$got" = 0 ] \
  && pass "invoker env: automatic-close direction survives restart recovery" \
  || fail "invoker env: recovered chaining direction '$got', wanted 0"
python3 - "$TMP/mine/.hw/r-mine/env" <<'PYEOF'
import sys
p = sys.argv[1]
s = open(p).read().replace("HW_CHAINING_ENABLED='0'", "HW_CHAINING_ENABLED='1'")
open(p, "w").write(s)
PYEOF
got="$(chaining_of "$TMP/nowhere" HW_WORKDIR="$TMP/mine" HW_RUN=r-mine)"
[ "$got" = 1 ] \
  && pass "invoker env: enabled chaining survives restart recovery" \
  || fail "invoker env: recovered chaining direction '$got', wanted 1"

# ── 4. a RESUMED executor has no HW_* at all, so there is nothing to
#       root at and the walk from $PWD is the only thing left. UNCHANGED.
got="$(pane_of "$TMP/mine/sub/deeper")"
case "$got" in
  wMINE:p1) pass "invoker env: with no \$HW_WORKDIR to root at, the walk up from \$PWD still finds the run env file" ;;
  *) fail "invoker env: a resumed executor (no HW_*) lost its walk-up — got '$got', wanted wMINE:p1" ;;
esac

# ── 5. a stale or wrong HW_WORKDIR must REFUSE, not fall back to the cwd walk.
#       Falling back is how the variable would stop meaning anything: the pane
#       would be told one thing and read another.
#       Both halves are asserted at once by standing in a cwd whose walk WOULD
#       succeed (wMINE:p1) while HW_WORKDIR names a real, readable, out-of-root
#       run file (wOUT:p1). Either failure mode names itself in the output.
got="$(pane_of "$TMP/mine/sub/deeper" HW_WORKDIR="$OUTSIDE" HW_RUN=r-out)"
case "$got" in
  '<unset>') pass "invoker env: an \$HW_WORKDIR outside every root refuses outright, and does not fall back to \$PWD either" ;;
  wOUT:p1)   fail "invoker env: read an env file outside every root — \$HW_WORKDIR is being treated as a licence to read anywhere" ;;
  *)         fail "invoker env: an out-of-root \$HW_WORKDIR fell back to the cwd walk and adopted '$got'" ;;
esac

# ── 6. HW_WORKDIR without HW_RUN: the directory is still authoritative, and the
#       newest run inside it is the answer. A work directory is reused across
#       runs by `hw done`, so "newest" is the existing rule, not a new one.
got="$(pane_of "$TMP/nowhere" HW_WORKDIR="$TMP/theirs")"
case "$got" in
  wTHEIRS:p1) pass "invoker env: \$HW_WORKDIR with no \$HW_RUN reads the newest run in THAT directory" ;;
  *) fail "invoker env: \$HW_WORKDIR alone did not resolve — got '$got', wanted wTHEIRS:p1" ;;
esac

# ── 7. the value must reach python EXPLICITLY. Read from the ambient
#       environment instead, a HW_WORKDIR that is set but not exported would
#       send this silently back down the cwd path — the defect restored by a
#       variable nobody notices is missing.
grep -q 'HW_ENV_ROOTED_AT="${HW_WORKDIR:-}"' "$IC" \
  || fail "invoker env: HW_ENV_ROOTED_AT is not passed on the python invocation, so an unexported HW_WORKDIR silently takes the cwd path"
grep -q 'HW_ENV_ROOTED_RUN="${HW_RUN:-}"' "$IC" \
  || fail "invoker env: HW_ENV_ROOTED_RUN is not passed on the python invocation"
pass "invoker env: the root and the run are handed to python on the invocation, not read from the ambient environment"
