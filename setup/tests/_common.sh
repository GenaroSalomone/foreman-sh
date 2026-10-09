# shellcheck shell=bash
# _common.sh — everything every subject file needs, and nothing any one of them
# owns alone. Sourced, never executed.
#
# WHY THIS SUITE EXISTS. Every fix in bin/hw on 2026-08-24 was verified by
# running ad-hoc commands in a scratchpad that is deleted at the end of the
# session. The fixes are real; the evidence for them was not kept. That is the
# same defect the fixes themselves are about — a claim whose supporting signal
# is gone.
#
# WHY IT IS A DIRECTORY. It was one 2173-line file that grew by appending, and
# every task picked a section id that looked free. Four ids collided (7e, 7f,
# 7i, 7j), the reading order ended `… 7k, 7l, 7d, 7e, 7f, 7j, 8, 9, 7i`, and
# `7h` reached 17 subsections. Two helper names were quietly defined twice
# (`_link_data_fixtures`, `die`) because everything shared one scope. Section
# ids are gone: the FILE NAME is the index, and a new subject is a new file
# rather than an id nobody can prove is unused.
#
# EVERY SUBJECT FILE IS ITS OWN BASH PROCESS. ../test-hw runs them, it does not
# source them. So each gets its own $TMP, its own trap and its own helper
# names, and one subject's fixture can no longer shadow another's.
#
# IT RUNS UNDER BASH ON PURPOSE — the shebang on each subject file, and
# ../test-hw invoking them with `bash`. Two of the traps only reproduce in
# bash 3.2, which is what every bin/ script runs under, and both looked fixed
# when checked in an interactive zsh:
#   * `[!a-z]*` accepts "Mala-Mayuscula" in bash, rejects it in zsh (collation).
#   * an unquoted "$var" holding "--worktree maybe" word-splits in bash and does
#     NOT in zsh, so a validation check silently tests one argument, not two.
#
# Hermetic. No herdr object is created, moved or closed, no worktree is built,
# no agent is launched; herdr and herdr-rpc are stubbed on PATH; every hw call
# is --dry-run; pure functions are extracted and driven directly; and HOME is a
# directory this run owns (see "A HOME THAT IS NOT THIS MACHINE'S" below). The
# last exception, 16-status.sh's real `hw status` over the live work
# directories, now runs over a fixture tree instead.
set -euo pipefail

# STRIP GIT'S AMBIENT ENVIRONMENT BEFORE ANYTHING ELSE.
#
# This is not hygiene, it is damage control, and it is here because it already
# happened. A git hook exports GIT_DIR and GIT_INDEX_FILE, and EVERY git command
# a hook-invoked child runs targets those — including `git init` inside an
# unrelated directory. The rollback fixture below does
# `( cd "$TMP/rb" && git init && git add -A && git commit )`, which reads as
# obviously scoped and is not: run from pre-commit, on 2026-08-24, it
# initialised brain's own repository, committed the fixture's two files over
# HEAD and left every real file untracked. Nothing was lost — the commit was
# never pushed and `git reset --mixed` restored it — but the working tree of a
# repo several agents write to was one `--hard` away from real damage.
#
# It is also this suite's own subject matter: "I cd'd into another directory"
# is a signal that does not carry the claim "git will operate there".
unset GIT_DIR GIT_INDEX_FILE GIT_WORK_TREE GIT_OBJECT_DIRECTORY \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_PREFIX \
      2>/dev/null || true

# AND THE CONFIG-INJECTION FAMILY, which the list above missed until 2026-09-09.
#
# `git -c <key>=<value> …` does not keep that setting to itself: git serialises
# every `-c` into GIT_CONFIG_PARAMETERS (and accepts the same injection as
# GIT_CONFIG_COUNT / GIT_CONFIG_KEY_<n> / GIT_CONFIG_VALUE_<n>), exports it, and
# EVERY descendant git process applies it — at command-line precedence, which
# beats the repository's own config. A hook is a descendant, and so is every git
# command a hook-invoked test runs inside a throwaway fixture.
#
# MEASURED 2026-09-09. `git -c core.hooksPath=setup/hooks commit` — the way this
# repo's hook gets invoked when .git/hooks is not the source — leaked
# `core.hooksPath=setup/hooks` all the way down into
# tests/105-decisions-staged-budget.sh, whose fixtures install a pre-commit hook
# at <fixture>/.git/hooks/pre-commit and then commit deliberately corrupt
# rotations to prove the guard REFUSES them. In the leaked context git looked for
# the hook at <fixture>/setup/hooks/pre-commit — a directory the fixture creates
# for other reasons and never puts a hook in — found nothing, ran no guard, and
# accepted all eight corrupt commits. The guard was never wrong; it was never
# called. Standalone the same file passes, which is what made it look like the
# suite failed "only inside the hook".
#
# The prefix sweep covers GIT_CONFIG itself, GIT_CONFIG_GLOBAL/SYSTEM, and the
# numbered KEY/VALUE pairs a fixed list cannot name.
for _v in ${!GIT_CONFIG@}; do
  unset "$_v" 2>/dev/null || true
done
unset _v 2>/dev/null || true

# AND STRIP THE INVOKER ADDRESS, for the same reason: this suite must not read
# the environment of the pane that happens to be running it. Since 2026-08-26
# hw honours an explicitly exported HW_INVOKER_PANE ahead of every heuristic
# and REFUSES one that is not live — correct against a real herdr, and fatal
# against the stub below, whose pane list is empty by design. An executor
# running `bash setup/test-hw` inherits exactly that variable from its own
# dispatch, so without this line the suite passes for a brainer and dies for
# every executor: the "green in one env, red in another" shape this directory
# exists to prevent.
# AND THE CHAINING DIRECTION, which is the same shape and was found the same
# way. `HW_CHAINING_ENABLED` is read environment-first and env-file-second by
# design (invoker-common.sh), so 23-invoker-env-file.sh's fixture — which
# rewrites the env FILE and asserts the recovered direction — is silently
# overridden by whatever the running pane exports. hw exports it on every
# executor, so `bash setup/test-hw` was green for a brainer and red at test 23
# for every executor that ran it. Measured 2026-08-26 in this directory.
# THE THIRD INSTANCE, AND WHY THIS IS NO LONGER A LIST OF NAMES TO REMOVE.
#
# This line used to read `unset HW_INVOKER_PANE HW_CHAINING_ENABLED
# HW_DONE_KEEP_PANE`. Measured 2026-09-02: `hw` exports FOUR chaining variables
# into every `--keep-pane` executor, and only ONE of them was named here. With
# `HW_CHAINING_LEASE_SECONDS` still set, `done-invoker` starts a bounded lease
# by calling `hw chaining-lease-start`; 24-done-closes-delivered.sh asserts that
# chaining invokes `hw` for nothing, and was correct to fail. So `./test-hw`
# printed 1140 ok / exit 0 for a brainer and aborted at 406 ok for a chained
# executor — the same tree, two answers. A second-vendor reviewer independently
# returned NO SHIP for subject 24 by the same mechanism, and an earlier bisect
# across five commits measured the runner's own environment rather than any of
# those commits.
#
# THE DEFECT WAS THE SHAPE, NOT THE MISSING NAME. An enumeration of what to
# REMOVE has to be extended every time `hw` grows an export — a file in another
# directory, edited by other agents, with nothing connecting the two. That
# lockstep has now failed three times in one day (HW_INVOKER_PANE,
# HW_CHAINING_ENABLED, and this one), which is not three mistakes but one
# design that produces them.
#
# So it is a PREFIX SWEEP with an inverted list: every `HW_*` in the
# environment goes, EXCEPT the handful of names this directory owns as
# deliberate inputs. The rot moves to where it is harmless — a new `hw` export
# is scrubbed the day it is invented, and only a new HARNESS hook needs a line
# here, added by whoever is already editing a file in tests/.
#
# `${!HW_@}` is bash 3.2 prefix name expansion (verified on 3.2.57, and it
# yields an empty word list rather than an error under `set -u` when nothing
# matches), so this needs no `env`/`sed` parse of values that may contain
# newlines.
#
# WHAT IS KEPT, and each one is a deliberate input rather than pane state:
#   HW_UNSTICK_BIN                 22-hw-unstick.sh:6  — points the extraction
#                                  at a mutant instead of $ROOT/bin/hw
#   HW_SOURCE                      49-required-agent-contract.sh:5 — same idea
#   HW_CHECK_LIVE_REVIEW_DRIFT     31-review-context-plugin-drift.sh:197
#   HW_CHECK_LIVE_REVIEW_BINDING   36-review-binding-producer.sh:232
# The last two GATE AN OPTIONAL LIVE ARM. Sweeping them would silently turn a
# deliberate opt-in off, which is the "an arm nobody runs is not coverage"
# failure `setup/mutation-coverage` exists to catch — a fix that caused it would
# be worse than the bug.
# HW_IMPL (and HW_CORE, the binary it routes to, and HW_SHADOW_JSONL, where shadow writes) ARE INPUTS THIS DIRECTORY OWNS, since the Go pilot (core/): `HW_IMPL=go|shadow
# bash setup/test-hw` runs the whole suite through hw-core for the verbs it has (bin/hw's
# shim). Unset it and the answer is bash's, which is the default and the rollback.
_HW_TEST_INPUTS=" HW_UNSTICK_BIN HW_SOURCE HW_CHECK_LIVE_REVIEW_DRIFT HW_CHECK_LIVE_REVIEW_BINDING HW_IMPL HW_CORE HW_SHADOW_JSONL "  # MUTATION-ANCHOR: 54-M02
for _v in ${!HW_@}; do  # MUTATION-ANCHOR: 54-M01
  case "$_HW_TEST_INPUTS" in *" $_v "*) continue ;; esac
  unset "$_v" 2>/dev/null || true
done
unset _v _HW_TEST_INPUTS 2>/dev/null || true
# The two non-HW_ names `hw` also bakes into an executor's environment. They are
# outside the prefix, so they are named — and they are pane state, not inputs:
# 09 passes ENGRAM_PROJECT explicitly where it needs one, and 13 asserts about
# the namespace by reading hw's output rather than its own environment.
unset ENGRAM_PROJECT AGENT_BROWSER_NAMESPACE 2>/dev/null || true  # MUTATION-ANCHOR: 54-M03

# AND THE HERDR ADDRESS FAMILY — THE FOURTH INSTANCE, AND THE FIRST ONE THAT
# MUTATED SOMETHING OUTSIDE THIS SUITE.
#
# Everything above removes state that made the suite ANSWER differently in two
# environments. This block removes state that made the suite WRITE. Measured
# 2026-09-15, on a recording stand-in for the herdr socket, running each subject
# unchanged: 133-a-correction-does-not-need-a-dead-executor.sh sent SEVEN
# `pane.report_metadata` calls (six distinct) carrying the fixture's own tokens —
# `hw_run=20260911-100000-1`, a constant that names no run on this disk — onto
# `pane_id: w7G:p9F`, the LIVE PANE OF THE AGENT RUNNING THE SUITE. Six more
# subjects reached the real daemon with reads rather than writes: 12-hw-next (24
# `agent.get`), 130 (3), 44 (1) and 72 (3) the same, 16-status (4
# `events.subscribe`) and 73 (3) the same. Every other subject on disk sent
# nothing — 136, which shares the same fixture constant, among them. After this
# block all seven send nothing at all, re-measured the same way.
#
# WHAT IS PINNED BY A SUBJECT AND WHAT IS NOT, because they are different and
# only one of them survives this session. 145-a-test-run-does-not-write-on-a-
# live-pane.sh pins THE WRITE PATH: a subject-shaped driver, a control that must
# reproduce it, and the absence asserted against a recording socket. The six
# READ-ONLY subjects above are a measurement, recorded in the commit and here,
# not a standing assertion — 145 does not re-run them, and a future subject that
# starts reading a live daemon again would not fail it. Pinning that too means a
# runner-level witness rather than a subject, which is its own piece of work.
#
# AND THE SECOND RUNNER WAS MEASURED, NOT ASSUMED. `setup/test-channel-send` is
# not globbed by ../test-hw, has its own environment sweep, and is where the
# HW_* enumeration bug recurred a fourth time (54's header). Driven the same way
# on 2026-09-15 it sent the recording socket ZERO requests: it builds its own
# `$HARNESS/bin` and puts a mocked `herdr-rpc` in it, so the invokers it drives
# resolve `$INVOKER_BIN_DIR/herdr-rpc` to that mock rather than to brain/bin.
# It is protected by construction rather than by a sweep, so nothing is added
# there — but a fixture in that file that reached brain/bin directly would be
# unguarded, and that is stated rather than left to be rediscovered.
#
# THE CHAIN, and every link of it is a design decision that is individually
# correct. `hw executor-turn-end` publishes a turn record for whatever pane it
# is running in: `_executor_turn_publish` (bin/hw) gates on `HERDR_ENV=1` and a
# non-empty `HERDR_PANE_ID` and then addresses exactly that pane — right, because
# an executor is the thing whose turn ended. 133 supplies `HW_PROJECT`,
# `HW_TASK`, `HW_RUN` and `HW_WORKDIR` as fixture values and calls that command —
# right, because those are the inputs under test. Neither knows about the other,
# and the result is a test suite writing fixture constants onto a working pane.
#
# THE SYMPTOM IS ELSEWHERE AND ARRIVES LATER. The tokens outlive the run (the
# ttl is 24h), so the next legitimate report from that pane meets `hw`'s
# token/receipt cross-check (bin/hw, "report tokens on <pane> belong to run
# <run>") and the receipt loses `report_project`, `report_task`, `report_run`,
# `report_summary` and `report_status`. That guard is the only reason any of
# this was visible; it is not where the fix belongs.
#
# WHY A STUB ON $PATH DOES NOT REACH IT, which is the part that makes this its
# own block rather than one more name on the list above. The stubs below are
# found by $PATH. Every caller that matters invokes herdr-rpc by ABSOLUTE PATH —
# `$HW_BIN_DIR/herdr-rpc` (bin/hw), `$INVOKER_BIN_DIR/herdr-rpc`
# (bin/invoker-common.sh), `$BIN_DIR/herdr-rpc` (bin/channel-send,
# bin/hw-reconcile), `$SCRIPT_DIR/herdr-rpc` (bin/codex-channel-session-hook.sh)
# — because herdr-rpc is deliberately NOT on $PATH in production and must be
# found beside its caller. A $PATH entry cannot shadow a path that is already
# written out, and those files are not this directory's to edit.
#
# SO THE INTERCEPTION IS AT THE ADDRESS, NOT AT THE BINARY. Two independent
# cuts, because either alone leaves a way through:
#   1. the prefix sweep here removes the pane identity, so the gate in
#      `_executor_turn_publish` (and every `[ -n "${HERDR_PANE_ID:-}" ]` like it)
#      is closed before any address is built; and
#   2. `HERDR_SOCKET_PATH` is re-pointed, below, at a path this harness owns and
#      never creates, so a caller that builds an address anyway connects to
#      nothing. Unsetting it would work too; naming a harness path makes the
#      diagnostic say where the call went, which is the difference between a
#      confusing red and an obvious one.
#
# AN INVERTED LIST, for the same reason the HW_ sweep has one: an enumeration of
# what to REMOVE has to track herdr's own environment, which nobody here owns.
# The list is EMPTY today — no subject reads a HERDR_* it did not set on the
# command line, and several (08, 13, 122) already `env -u HERDR_PANE_ID`
# defensively. It exists so that adding a deliberate input is one word here
# rather than a redesign.
_HERDR_TEST_INPUTS=" "
for _v in ${!HERDR_@}; do  # MUTATION-ANCHOR: 145-M01a  # MUTATION-ANCHOR: 145-M02
  case "$_HERDR_TEST_INPUTS" in *" $_v "*) continue ;; esac
  unset "$_v" 2>/dev/null || true
done
unset _v _HERDR_TEST_INPUTS 2>/dev/null || true

# One directory deeper than the old single file, so two levels up, not one.
ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# $ROOT IS NOT ALWAYS THE CHECKOUT. setup/test-hw re-execs itself against a
# private byte-for-byte SNAPSHOT (test-hw-snapshot.py), and a snapshot has no
# .git — so `git -C "$ROOT" …` there fails with "not a git repository", and any
# claim about what a COMMIT contains has to be asked of the live checkout
# instead. Measured 2026-09-09: tests/112's committed-mode assertion passed
# standalone and exited 128 under the runner for exactly this reason.
#
# Use $ROOT to read bytes (that is what the snapshot exists to freeze) and
# $LIVE_ROOT only to ask git about the repository. Standalone the two are equal.
LIVE_ROOT="${TEST_HW_LIVE_ROOT:-$ROOT}"
# bin/hw sources its modules from lib/ beside its own bin/. A fixture that copies
# bin/hw (or just the one file, as every mutant does) has no lib/ beside it, so
# the suite names this checkout's: the fallback hw reads only when none is there.
export HW_LIB_DIR="$ROOT/lib"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/hw-test.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
# HW_CORE IS A SEAM, like HW_LIB_DIR above: a subject that runs hw from a COPY of bin/ has no
# core/dist beside it, and with HW_IMPL=go|shadow the shim would warn that hw-core is not built. When the
# suite is asked for an implementation other than bash, it hands the shim the hw-core it has (the built
# one, or one built here) — never a warning line the subject did not ask for.
if [ -n "${HW_IMPL:-}" ] && [ "$HW_IMPL" != bash ] && [ -z "${HW_CORE:-}" ]; then
  # the export of this suite may not carry contract-hw/: without it there is no hw-core to hand over
  if [ -f "$ROOT/setup/tests/contract-hw/world.sh" ]; then
    . "$ROOT/setup/tests/contract-hw/world.sh"
    _core="$(contract_core_bin "$TMP/hw-core-build")" || { printf 'runner: HW_IMPL=%s and the go toolchain is here, but hw-core did not build\n' "$HW_IMPL" >&2; exit 1; }
  else
    _core=""
  fi
  if [ -n "$_core" ]; then export HW_CORE="$_core"; else printf 'runner: HW_IMPL=%s but there is no hw-core to hand over (no go toolchain, or no contract-hw/ in this suite) — the shim will fall back to bash with its warning\n' "$HW_IMPL" >&2; fi
  unset _core
fi
# hw_lib_beside <bindir> — a lib/ of its own beside a copy of bin/hw, so a MUTANT of
# a moved module (lib/hw/next.sh) is mutated in a file the copy actually sources:
# hw reads the lib/ beside its bin/ first (HW_LIB_DIR above only fills in for none).
# It carries EVERYTHING in lib/hw/, not only the *.sh: deps.sh reads deps.conf beside itself and
# bin/hw dies ("lib/hw/deps.conf has no gentle-ai row") before reaching what a subject measures
# when the copy lacks it (930). A real install with a missing floor must keep failing: only the
# copy is completed, deps.sh is untouched.
# Prints the new lib/hw dir. <bindir> must have a parent the caller owns alone.
hw_lib_beside() {
  local d="$1/../lib/hw"
  rm -rf "$d"; mkdir -p "$d"; cp -R "$ROOT"/lib/hw/. "$d/"
  (cd "$d" && pwd)
}

# THE SECOND CUT OF THE HERDR SWEEP ABOVE, here because it needs $TMP.
#
# herdr-rpc resolves its socket from $HERDR_SOCKET_PATH and exits 3 when it
# cannot connect — a documented outcome every caller in bin/ already handles as
# "a fact about herdr, not about the pane". Pointing it at a path inside this
# run's own $TMP, which is never created, makes every absolute-path herdr-rpc
# call a connect failure against an address this directory owns: nothing live is
# read, nothing live is written, and a call that should not have happened names
# the harness in its own error text.
export HERDR_SOCKET_PATH="$TMP/no-such-herdr.sock"  # MUTATION-ANCHOR: 145-M01b  # MUTATION-ANCHOR: 145-M03
export LC_ALL=en_US.UTF-8

# AND THE ENGRAM ADDRESS, THE SAME CUT FOR THE SAME REASON.
#
# `hw` registers every executor's engram session with `POST /sessions` on
# `${HW_ENGRAM_URL:-http://127.0.0.1:${ENGRAM_PORT:-7437}}` — an ADDRESS, not a
# path under HOME, so the empty HOME below does not reach it. The HW_* sweep
# above removes HW_ENGRAM_URL, which leaves the default: the `engram serve` of
# the person running the suite. Measured 2026-09-25 under this file: a
# registration made from a subject resolved to that live address. So it is
# re-pointed at a port nothing serves, and the path names the harness in any
# error a stray call produces; ENGRAM_PORT goes with it, since it is the second
# road to the same server. ENGRAM_DATA_DIR is pinned into this run's HOME for
# the CLI, which opens whatever store that variable names (decisions.md,
# 2026-09-21: a `--help` opened and migrated the real one).
#
# ENGRAM_PORT IS PINNED, NOT UNSET. Unset means 7437, and 7437 is the port the
# person's serve lives on. Measured 2026-09-28: an OpenCode run under a sandbox
# HOME found no serve there (the real one was down), its engram plugin spawned
# `engram serve` with that HOME, and the sandbox store held the real port for an
# hour (bin/engram-serve.sh). A subject that starts an agent with an engram
# plugin, or `engram serve` itself, now resolves port 9, which is not that port.
# Pinned by 189-la-suite-no-escribe-en-engram.sh.
export HW_ENGRAM_URL="http://127.0.0.1:9/hw-test-no-engram"
export ENGRAM_DATA_DIR="$TMP/home/.engram"
export ENGRAM_PORT=9
# brain waits for a serve to start; port 9 never will, so no subject pays the timeout
export HW_ENGRAM_WAIT=0
# NO SUBJECT STARTS THE BRAINER'S HOUSEKEEPING (setup/guards/lane_housekeeping.py):
# a SessionStart driven here would run `hw status` and detach a real
# `hw reap --apply`. 640 turns it back on, against its own fixture.
export HW_HOUSEKEEPING=0

# ── A HOME THAT IS NOT THIS MACHINE'S ──────────────────────────────────────
#
# THE SUITE READ THE MACHINE, AND A CONFIG CHANGE BROKE IT. Until 2026-09-22 a
# subject that wanted ~/.config/opencode/opencode.json, the installed herdr
# plugin, ~/.codex/hooks.json or a product repo's .claude/agents simply read the
# live one. So a change of CONFIGURATION, not of code, turned the suite red —
# measured that day with 70 and 74 — and the same tree answered differently on
# two machines, which is the one thing a suite may not do.
#
# So every subject runs with HOME pointed at an empty directory this run owns.
# What a subject needs from HOME it builds there itself, from a fixture in
# setup/fixtures/ or with the helpers below; nothing is inherited. The
# properties of the LIVE machine that used to be asserted here have not been
# dropped: they moved to setup/check-machine, which is outside this suite on
# purpose and says, per check, which subject used to carry it.
#
# XDG_CONFIG_HOME is pinned inside it, and the per-vendor config pointers are
# removed, because each is a second road to the same live state: an exported
# CLAUDE_CONFIG_DIR or CODEX_HOME points at the real ~/.claude-personal or
# ~/.codex regardless of what HOME says.
export HOME="$TMP/home"
mkdir -p "$HOME"
export XDG_CONFIG_HOME="$HOME/.config"
unset XDG_DATA_HOME XDG_STATE_HOME XDG_CACHE_HOME CLAUDE_CONFIG_DIR CODEX_HOME \
      OPENCODE_CONFIG OPENCODE_CONFIG_DIR 2>/dev/null || true
# AND NO BYTECODE CACHE UNDER IT. Apple's /usr/bin/python3 cannot write
# __pycache__ beside its own stdlib, so it writes to
# $HOME/Library/Caches/com.apple.python instead — which is now $TMP. A subject
# that leaves a detached python behind (153's arm) kept writing there while the
# EXIT trap removed $TMP, the `rm -rf` lost the race ("Directory not empty"),
# and the subject exited red. Measured 2026-09-22 under the runner. Before this
# block the same writes landed in the real ~/Library, unseen.
export PYTHONDONTWRITEBYTECODE=1
# AND NOT THE CI HOST'S LENIENCY. setup/test-hw relaxes every fast-gate budget
# where CI or GITHUB_ACTIONS is true (setup/fast-gate-budget.sh, point 6); a
# subject that drives a nested runner (217, 391) must judge its fixtures on
# the budgets it wrote, on a laptop and on a runner alike.
unset CI GITHUB_ACTIONS 2>/dev/null || true
FIXTURES="$ROOT/setup/fixtures"

# THE LANE TABLE AND THE BRAIN ROOT, pinned to what every subject was written
# against. bin/ derives its root from where the script lives and reads
# <root>/projects.json; most subjects run a COPY of bin/ under $TMP, whose
# derived root would be $TMP, with no table. So the suite names both: the root
# the old bin/ spelled (guards.json's brain_root under the home, which home_brain
# links to the tree) and the table of the tree under test. The subjects about derivation
# itself (174, 175) unset them.
# Where the brain sits under the home is the guards' own setting (guards.json's
# brain_root), so the fixture follows the configuration instead of naming a path.
# A minimal tree that copies only this file has no guards.json: it gets a neutral name.
_brain_rel="$(jq -r '.brain_root // empty' "$ROOT/guards.json" 2>/dev/null || true)"
_brain_rel="${_brain_rel:-brain-under-test}"
export HW_BRAIN_ROOT="$HOME/${_brain_rel#\~/}"
export HW_PROJECTS_JSON="$ROOT/projects.json"

# lane_harness <hw> <out.sh> <fn>... — the named functions extracted from <hw>,
# behind the lane table they read: project-spaces.sh from the same bin/, loaded
# from $HW_PROJECTS_JSON. A lane fact that used to be a literal in the function
# is a table value now, so an extraction without the table proves nothing.
lane_harness() {
  local hw="$1" out="$2" fn; shift 2
  { lane_preamble "$hw"
    for fn in "$@"; do sed -n "/^$fn() {/,/^}/p" "$hw"; done; } > "$out"
}

# lane_preamble [<hw>] — the two lines an extracted harness needs first:
# project-spaces.sh from <hw>'s bin/ (the tree's by default) and the table.
lane_preamble() {
  printf '. %q\nlane_config_load %q >/dev/null || exit 97\n' \
    "$(dirname "${1:-$ROOT/bin/hw}")/project-spaces.sh" "$HW_BRAIN_ROOT"
}

# table_mutant <out.json> <python on `t`> — the lane table under test with one
# edit. A subject whose mutant used to rewrite a `case` arm in bin/hw now
# rewrites the table, since that is where the value lives; run the subject's
# binary with HW_PROJECTS_JSON=<out.json>.
table_mutant() {
  python3 - "$HW_PROJECTS_JSON" "$1" "$2" <<'PY'
import json, sys
t = json.load(open(sys.argv[1]))
exec(sys.argv[3])
json.dump(t, open(sys.argv[2], "w"), indent=2)
PY
}

# lane_build_file <lane> — the lane's build hook in the tree under test: the
# file projects.json's `build` names, relative to the table. A lane's build path
# lived in a `case` arm in bin/hw until stage 3; a subject that reads or mutates
# it reads this file now. Empty for a lane with no hook.
lane_build_file() {
  local b
  b="$(jq -r --arg l "$1" '.lanes[$l].build // empty' "$HW_PROJECTS_JSON")"
  [ -n "$b" ] || return 0
  printf '%s/%s' "$(dirname "$HW_PROJECTS_JSON")" "$b"
}

# home_brain — put the tree UNDER TEST where bin/ expects to find brain.
#
# bin/hw, bin/brain and bin/invoker-common.sh resolve brain to $HW_BRAIN_ROOT
# (see 171's ratchet). Before this block that resolved to the MAIN checkout, so a subject run from a worktree exercised its
# own bin/hw against another tree's lanes and briefs. The link points at $ROOT —
# the snapshot under ../test-hw, the checkout when run alone — so it is the code
# being tested, not state of the machine. Making bin/ take the root as input is
# stage 2's; this suite does not touch bin/.
home_brain() {
  mkdir -p "$(dirname "$HW_BRAIN_ROOT")"
  [ -e "$HW_BRAIN_ROOT" ] || ln -s "$ROOT" "$HW_BRAIN_ROOT"
}

# home_repo <path under HOME> [<branch>] — a one-commit git repository where
# bin/ expects a product checkout (project-spaces.sh's *_MAIN defaults). It
# stands in for the live checkout the subject used to read; add what the
# subject needs to it after calling this.
home_repo() {
  local dir="$HOME/$1" branch="${2:-main}"
  mkdir -p "$dir"
  ( cd "$dir" && git init -q -b "$branch" && git config user.email t@t \
    && git config user.name t && git commit -q --allow-empty -m base )
}

# home_opencode_config [<fixture>] — the global OpenCode config, from a fixture.
# Default: setup/fixtures/opencode.json, a snapshot of the agent section
# of the live file taken on 2026-09-22 (the date 70 and 74 went red on it).
home_opencode_config() {
  local src="${1:-$FIXTURES/opencode.json}"
  mkdir -p "$XDG_CONFIG_HOME/opencode"
  cp "$src" "$XDG_CONFIG_HOME/opencode/opencode.json"
}

# The runner sums the `ok -` lines across every subject file, so no file keeps
# a count of its own — a per-file counter could only ever report a fragment.
pass() { printf 'ok - %s\n' "$1"; }
fail() { printf 'not ok - %s\n' "$1" >&2; exit 1; }

# link_tool <program path> <dir> [<name>] — one program on a PATH of single links.
# Windows looks a program's DLLs up beside the LINK, so a lone link to bash.exe
# dies on `error while loading shared libraries`; under msys/cygwin the DLLs of
# the program's own directory are linked in beside it.
link_tool() {
  local p="$1" d="$2" n="${3:-${1##*/}}" dll
  ln -sf "$p" "$d/$n" || return 1
  case "${OSTYPE:-}" in msys*|cygwin*)
    for dll in "${p%/*}"/*.dll; do
      if [ -e "$dll" ] && [ ! -e "$d/${dll##*/}" ]; then ln -s "$dll" "$d/${dll##*/}" 2>/dev/null || true; fi
    done ;;
  esac
}

# saw_mutant <arm> <output> <needle> [<needle>...]
#
# A MUTANT IS KILLED BY WHAT IT SAID, NEVER BY WHAT IT DID NOT SAY. Measured
# 2026-09-01 (8f5c8ef): 45's M01 copied bin/ask-invoker without
# the library it sources, died on `cannot find invoker-common.sh` before reaching
# the mutated line, and its arm — `*'original text'*) fail ;; *) pass "mutant
# killed"` — read that silence as a kill. Absence of the original is exactly what
# a mutant that never ran produces, so an arm built on absence certifies every
# broken mutant.
#
# This is the positive shape: the needles are text that ONLY the mutated code
# produces (the string the mutation inserted, the diagnosis the mutated branch
# prints, the field the mutant now omits being NAMED as omitted by the caller),
# and at least one must be in the mutant's output. A mutant that could not run
# fails here as VACUOUS, with the tail of what it actually printed, instead of
# passing as dead. Kill lines it prints carry the `mutant killed:` marker, so
# setup/mutation-coverage counts them as exercised arms.
saw_mutant() {
  local arm="$1" out="$2"; shift 2
  [ $# -ge 1 ] || fail "$arm: saw_mutant needs at least one needle — a kill with no evidence is the defect this helper exists to refuse"
  local n
  for n in "$@"; do
    case "$out" in *"$n"*) pass "mutant killed: $arm (saw: $n)"; return 0 ;; esac
  done
  fail "$arm VACUOUS: none of the mutant's own evidence [$*] is in its output, so the mutated line may never have run — tail: $(printf '%s' "$out" | tail -3 | tr '\n' ' ')"
}

# A SECOND WAY TO PASS THIS HELPER'S OWN NEEDLE CHECK AND STILL PROVE NOTHING.
# MEASURED 2026-09-11 (third review of setup/tests/136-...sh, its own
# subject): five of eight mutants in that file inserted a `printf` MARKER
# ahead of the guard they disabled —
#
#     printf "MUTANT-MODEL-GUARD-SKIPPED\n" >&2
#     if false; then
#       _reuse_flag_decision "$MODEL" "$REUSE_MODEL"
#
# — and `saw_mutant` found the marker and called it killed. The marker prints
# on EVERY invocation of that binary, whether the guard it precedes still
# works or not: a fake binary carrying the identical `printf` ahead of the
# UNCHANGED guard produces the marker AND the correct refusal side by side.
# The needle is real text the mutated file produces, so `saw_mutant` cannot
# tell this apart from a real kill by looking at the needle alone — a marker
# proves the file was edited, never that the edit changed what the program
# does. The sixth mutant in the same file had the same defect in reverse: its
# marker sat inside a branch (`if [ -z "$measured" ]`) the HEALTHY binary
# also takes, so the marker appeared regardless of the mutation too.
#
# THE FIX IS NOT A DIFFERENT HELPER, it is the discipline `saw_mutant`'s own
# needles were always meant to enforce, made explicit for callers building a
# marker-shaped mutant instead of the "mutation removed/altered real text"
# shape the doc above already covers:
#
#   1. SURVIVAL CONTROL FIRST. Run the UNMUTATED binary against the exact
#      fixture the mutant will use, and assert (with `pass`/`fail`, not
#      `saw_mutant`) that the behavior the mutant is supposed to break is
#      actually present. If this step cannot be written, the mutant is
#      testing nothing and must be redesigned before it is written at all.
#   2. THEN MUTATE, and kill on the ABSENCE of that same behavior, or the
#      PRESENCE of whatever incorrect behavior replaces it — text produced
#      by the PROGRAM DOING SOMETHING DIFFERENT, never text a `printf`
#      inserted next to the change asserts unconditionally. `saw_mutant`'s
#      needle in this shape is the wrong OUTPUT the mutant produces (a
#      refusal that should have fired but did not; a success line where a
#      `die` belongs) — not a string whose only job is to announce that the
#      patch applied.
#
# A mutant that cannot be killed this way — because the healthy binary
# cannot be shown taking a different path on the same fixture — is not a
# weaker test of the same thing; it is evidence for nothing, and the fix is
# to choose a different fixture or a different mutation, not to add a marker.

# WHY EVERY `x="$(grep ... )"` IN THIS FILE ENDS IN `|| true`.
#
# This script runs under `set -euo pipefail`. A grep that finds nothing exits 1,
# pipefail makes the whole pipeline fail, and the ASSIGNMENT then kills the
# script — before the `[ -n "$x" ] || fail "..."` written on the next line ever
# runs. The suite exits 1, prints no `not ok`, and `pre-commit` surfaces only
# lines starting with `not ok`, so a real regression appears as silence.
#
# Found 2026-08-25 by mutation-testing the lease guard: removing it left the
# suite "passing" as far as anything anyone reads. Nine assertions were in that
# shape. If you add another, end it in `|| true` and let the explicit check be
# the thing that speaks.

# ── stubs ───────────────────────────────────────────────────────────────────
# hw reads herdr to describe live state. A dry run must not depend on a live
# session, and must never mutate one.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  # the version floor check (lib/hw/deps.sh) asks every tool its version; a stub that answered
  # with a JSON object would read as "no readable version" and raise a warn in every preflight
  "--version ") echo "herdr 9.9.9" ;;
  "workspace list") echo '{"result":{"workspaces":[]}}' ;;
  "tab list")       echo '{"result":{"tabs":[]}}' ;;
  # OVERRIDABLE, default EMPTY — the default is what every other subject file
  # already relies on. It became overridable when the dispatch manifest started
  # resolving the invoker pane: an empty list can only ever produce UNRESOLVED,
  # so the resolvable half of that manifest line was untestable through hw.
  "pane list")      printf '{"result":{"panes":%s}}\n' "${STUB_PANES:-[]}" ;;
  # `type` IS PART OF THE REAL REPLY, measured on the live daemon 2026-09-07:
  # `.result` is `{"type":"agent_list","agents":[...]}`. The stub omitted it,
  # which made it a weaker answer than herdr's own — and a reader that needs to
  # tell "herdr answered this question" from "herdr answered something else"
  # could not be tested at all. `hw`'s occupancy gate keys on it now.
  "agent list")     printf '{"result":{"type":"agent_list","agents":%s}}\n' "${STUB_AGENTS:-[]}" ;;
  "agent prompt")   # WITNESS THE LOCK AS IT IS AT SEND TIME. The owner pid only
                    # exists while the lock is held, and the lock is released
                    # before channel-send exits, so nothing outside can observe
                    # it afterwards. Without this the "writes its pid" fix had
                    # no test: removing the write left the suite green.
                    [ -n "${STUB_LOCK_WITNESS:-}" ] && \
                      cat "${TMPDIR:-/tmp}/hw-deliver-wL_p1.lock/owner.pid" \
                        > "$STUB_LOCK_WITNESS" 2>/dev/null || true
                    echo '{"result":{}}' ;;
  "agent get")      _tok="${STUB_TOKENS-}"; [ -n "$_tok" ] || _tok='{}'
                    printf '{"result":{"agent":{"agent_status":"%s","cwd":"%s","tokens":%s}}}\n' \
                      "${STUB_STATUS:-idle}" "${STUB_CWD:-}" "$_tok" ;;
  *) echo '{"result":{}}' ;;
esac
STUB
chmod +x "$TMP/bin/herdr"
cat > "$TMP/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
exit "${STUB_RPC_RC:-0}"
STUB
chmod +x "$TMP/bin/herdr-rpc"
# A verifier that says yes. setup resolves the `personal` account (projects.json,
# 2026-10-06), and `hw` refuses an account `claude-personal --check` cannot
# prove; under the isolated HOME of a subject it proves nothing, and every
# `hw setup` dry run died before printing its manifest. Whether the account
# refusal works is 122's subject, which puts its own stub ahead of this one.
cat > "$TMP/bin/claude-personal" <<'STUB'
#!/usr/bin/env bash
[ "${1:-}" = --check ] && printf '%s\n' "$HOME/.claude-personal"
exit 0
STUB
chmod +x "$TMP/bin/claude-personal"
export PATH="$TMP/bin:$PATH"

# A subject's fake herdr listens through _herdr_endpoint.py (a unix socket, or
# the named pipe herdr serves on Windows); its scripts import it from here.
export HW_TEST_PYLIB="$ROOT/setup/tests"
# The account's own home — what the guards read as `~` (the password database;
# on native Windows the profile directory) — for a subject that expands a
# policy path the way the guard does. Never $HOME, which is the fixture's.
case "${OSTYPE:-}" in
  msys*|cygwin*) HW_TEST_ACCOUNT_HOME="$(cygpath -u "$USERPROFILE")" ;;
  *) HW_TEST_ACCOUNT_HOME="$(python3 -c 'import os, pwd; print(pwd.getpwuid(os.getuid()).pw_dir)')" ;;
esac
export HW_TEST_ACCOUNT_HOME

# NATIVE WINDOWS (Git Bash), what the subjects' OWN python, jq, git and hashing
# need — bin/'s entry points source the same msys-compat.sh for themselves. A
# fixture repository is made under a HOME of its own, so it reads only the
# system git config, whose core.autocrlf must be false (INSTALL.md; the CI
# sets it). It is not pinned here: a GIT_CONFIG_* variable is what 115
# forbids. Git for Windows has sha256sum but no shasum, which the subjects call bare (default SHA-1) and as `-a 256`.
# Elsewhere OSTYPE never matches and nothing changes.
# HW_TEST_SLOW scales the wall-clock bounds tuned on macOS and Linux: under Git
# Bash every process start costs ~10x (measured: a witness round ~5s where the
# bound allowed 3s). It lengthens budgets; it never removes an assertion.
export HW_TEST_SLOW=1
case "${OSTYPE:-}" in msys*|cygwin*)
  export HW_TEST_SLOW=10
  # A fixture tree that carries only _common.sh has no bin/: the environment half.
  if [ -f "$ROOT/bin/msys-compat.sh" ]; then . "$ROOT/bin/msys-compat.sh"
  else export PYTHONUTF8=1 MSYS="winsymlinks:nativestrict${MSYS:+ $MSYS}"; fi
  ;;
esac
# WIN_ENV: what `env -i` must still pass on Git Bash. It is not hygiene there:
# without TEMP a msys runtime started past a native process (a detached reset)
# mounts /tmp somewhere else, and without MSYSTEM bin/'s layer stays off.
# Pass it as "${WIN_ENV[@]+"${WIN_ENV[@]}"}"; elsewhere it is empty.
WIN_ENV=()
case "${OSTYPE:-}" in msys*|cygwin*)
  for _v in TEMP SYSTEMROOT WINDIR USERPROFILE MSYSTEM; do [ -z "${!_v:-}" ] || WIN_ENV+=("$_v=${!_v}"); done
  [ -z "${TEMP:-}" ] || WIN_ENV+=("TMP=$TEMP")
  unset _v ;;
esac
if ! command -v shasum >/dev/null 2>&1 && command -v sha256sum >/dev/null 2>&1; then
  cat > "$TMP/bin/shasum" <<'STUB'
#!/usr/bin/env bash
a=1; case "${1:-}" in -a) a="${2:-1}"; set -- "${@:3}" ;; esac
exec "sha${a}sum" "$@"
STUB
  chmod +x "$TMP/bin/shasum"
fi

# Most subjects test a dispatch field unrelated to reporting. They deliberately
# use the operator's fire-and-forget opt-out, so the new report-channel gate does
# not turn every old fixture with an empty pane list into a different test. A
# report-specific subject calls hw_dry_report and supplies a live brainer pane.
#
# THE SAME MOVE FOR THE CITED REQUEST (2026-09-28). A lane with requested_by: required refuses a dispatch
# that cites no request, and a fixture about placement or ports is not about
# that. So these helpers cite one, appended LAST so it overrides any in "$@";
# the subject that asserts on the citation itself,
# setup/tests/191-lanzar-solo-lo-pedido.sh, uses hw_dry_uncited. Its model-floor
# fixtures get no such pass either: they cite their own request via REQ.
HW_TEST_CITATION=(--requested-by "setup test suite fixture")
#
# AND FOR THE BRIEF (2026-10-07). A launch with no brief is refused unless `--no-brief`
# says it is on purpose, and most fixtures dispatch a task name no brief exists for: the
# fixture is about placement or ports, not about the contract. `--no-brief` only lifts the
# refusal — a brief that IS found is still read — so these helpers pass it; the subject
# that asserts on the refusal itself, setup/tests/811-*, calls hw directly.
HW_TEST_NO_BRIEF=(--no-brief)
hw_dry() { "$ROOT/bin/hw" "$@" "${HW_TEST_CITATION[@]}" "${HW_TEST_NO_BRIEF[@]}" --no-report --dry-run 2>&1 | sed 's/\x1b\[[0-9;]*m//g'; }
hw_dry_report() { "$ROOT/bin/hw" "$@" "${HW_TEST_CITATION[@]}" "${HW_TEST_NO_BRIEF[@]}" --dry-run 2>&1 | sed 's/\x1b\[[0-9;]*m//g'; }
hw_dry_uncited() { "$ROOT/bin/hw" "$@" "${HW_TEST_NO_BRIEF[@]}" --no-report --dry-run 2>&1 | sed 's/\x1b\[[0-9;]*m//g'; }

# expect_out <label> <pattern> <hw args...>
expect_out() {
  local label="$1" pat="$2"; shift 2
  local out; out="$(hw_dry "$@" || true)"
  case "$out" in *"$pat"*) pass "$label" ;; *) fail "$label — no '$pat' in: $(printf '%s' "$out" | tail -3)" ;; esac
}
# expect_absent <label> <pattern> <hw args...>
expect_absent() {
  local label="$1" pat="$2"; shift 2
  local out; out="$(hw_dry "$@" || true)"
  case "$out" in *"$pat"*) fail "$label — '$pat' should NOT appear" ;; *) pass "$label" ;; esac
}

# mutate_anchor <id> <file> <replacement>
#
# A MUTANT IS ANCHORED ON A MARKER THE SOURCE DECLARES, NEVER ON ITS PROSE.
# Measured 2026-10-01: 78's M06 matched the literal text of the guard's verdict
# line, so a refactor that left the behaviour alone (the verdict line gained a
# branch) turned the suite red with no defect anywhere. The source now carries
# `# MUTATION-ANCHOR: <id>` on the line a mutant replaces; `mutate_anchor` finds
# that line in <file> (a copy the caller owns), and replaces the WHOLE line with
# <replacement>, each of its lines prefixed with the anchor line's indent. A block
# is anchored by a second `# MUTATION-ANCHOR-END: <id>` line: the span from one to
# the other, both included, is what is replaced. The
# id must occur exactly once or the call dies: a mutant that did not apply is a
# vacuous arm, never a kill. `setup/mutation-anchors.tsv` registers every id and
# the source file that carries it; `setup/tests/540-*` holds the registry honest.
mutate_anchor() {
  local id="$1" file="$2" repl="$3"
  [ $# -eq 3 ] || fail "mutate_anchor <id> <file> <replacement>"
  python3 - "$id" "$file" "$repl" <<'PYANCHOR' || fail "mutate_anchor $id: anchor not applied in $file"
import pathlib, re, sys
anchor_id, path, repl = sys.argv[1:4]
p = pathlib.Path(path)
lines = p.read_text().split("\n")
pat = re.compile(r"MUTATION-ANCHOR: " + re.escape(anchor_id) + r"(?![\w-])")
hits = [i for i, l in enumerate(lines) if pat.search(l)]
if len(hits) != 1:
    sys.exit("anchor %r matched %d lines" % (anchor_id, len(hits)))
i = hits[0]
j = i
endpat = re.compile(r"MUTATION-ANCHOR-END: " + re.escape(anchor_id) + r"(?![\w-])")
ends = [k for k, l in enumerate(lines) if endpat.search(l)]
if ends:
    if len(ends) != 1 or ends[0] < i:
        sys.exit("anchor %r has a misplaced or repeated END marker" % anchor_id)
    j = ends[0]
indent = re.match(r"\s*", lines[i]).group(0)
lines[i:j + 1] = [indent + r if r else r for r in repl.split("\n")]
p.write_text("\n".join(lines))
PYANCHOR
}

# par_run <function-or-command> [args...]   /   par_wait
#
# INDEPENDENT CHECKS OF ONE SUBJECT RUN SIDE BY SIDE, replayed in the order they were started.
# A subject that kills N mutants one after another pays the sum of N runs; each mutant owns its own
# copy and its own output, so nothing about them needs to be serial (806 did this by hand on
# 2026-10-07: twelve mutants took 275s one by one and 99s side by side). `par_run` forks a
# subshell, so the `fail` (an `exit 1`) inside it ends that job only; its stdout and stderr are
# kept, and `par_wait` replays them in start order — the same lines a serial run prints, in the
# same order, which is what setup/mutation-coverage reads — and fails the subject, naming the
# job, if any of them did. At most HW_TEST_PAR (default 4) are in flight: a subject is one pooled
# job, and the pool counts jobs, not the processes a job starts.
#
# A job must not write to a path another job writes, and must not read anything a later job
# makes. A command that edits the caller's shell state (a variable the rest of the subject reads)
# is not a job: the subshell keeps that edit to itself.
PAR_N=0; PAR_PIDS=(); PAR_INFLIGHT=()
par_run() {
  local n cap="${HW_TEST_PAR:-4}"
  case "$cap" in ""|*[!0-9]*|0) cap=4 ;; esac
  PAR_DIR="${PAR_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/par.XXXXXX")}"
  while [ "${#PAR_INFLIGHT[@]}" -ge "$cap" ]; do
    wait "${PAR_INFLIGHT[0]}" 2>/dev/null || true
    PAR_INFLIGHT=(${PAR_INFLIGHT[@]+"${PAR_INFLIGHT[@]:1}"})
  done
  PAR_N=$((PAR_N + 1)); n="$PAR_N"
  printf '%s' "$*" > "$PAR_DIR/$n.what"
  ( set +e; ( set -e; "$@" ); printf '%s' "$?" > "$PAR_DIR/$n.rc" ) > "$PAR_DIR/$n.out" 2> "$PAR_DIR/$n.err" &
  PAR_PIDS[$n]=$!
  PAR_INFLIGHT+=("$!")
}
par_wait() {
  local n=1 rc what pid
  while [ "$n" -le "$PAR_N" ]; do
    wait "${PAR_PIDS[$n]}" 2>/dev/null || true
    cat "$PAR_DIR/$n.out"
    cat "$PAR_DIR/$n.err" >&2
    rc="$(cat "$PAR_DIR/$n.rc" 2>/dev/null || echo 'no status')"
    if [ "$rc" != 0 ]; then
      what="$(cat "$PAR_DIR/$n.what" 2>/dev/null || true)"
      for pid in ${PAR_PIDS[@]+"${PAR_PIDS[@]}"}; do pkill -P "$pid" 2>/dev/null || true; kill "$pid" 2>/dev/null || true; done
      rm -rf "$PAR_DIR"
      fail "par: job $n ($what) ended with status $rc"
    fi
    n=$((n + 1))
  done
  rm -rf "$PAR_DIR"; PAR_DIR=""; PAR_N=0; PAR_PIDS=(); PAR_INFLIGHT=()
}

# load_scale [<cap>] — how many times slower than an idle machine a wall-clock assertion should be
# allowed to run RIGHT NOW: 1 on a quiet machine, up to <cap> (default 3) on a busy one. The larger of
# the load rule (ceil(load1 / cpus), the 1-minute average) and the CPU probe of setup/fast-gate-budget.sh
# (wall / cpu of a CPU-bound loop, rounded at 1.3x like the gate: 1.29 is 1, 1.30 is 2, what the scheduler is giving this process now), because the
# average lags a burst by a minute: setup/tests/733. A subject scales the LIMIT of an elapsed-time
# assertion with this and keeps the limit under the budget the defect would burn, so the assertion
# still tells the defect from the load. HW_TEST_LOAD1 / HW_TEST_NCPU / HW_TEST_PROBE inject the readings.
load_scale() {
  local cap="${1:-3}" lc l c p s=1
  . "$ROOT/setup/fast-gate-budget.sh" 2>/dev/null || { printf '1'; return 0; }
  lc="$(fg_load_ncpu)"; l="${lc% *}"; c="${lc#* }"
  case "$l:$c" in ''|*[!0-9:]*|:*|*:) ;; *) [ "$c" -gt 0 ] && s=$(( (l + 100 * c - 1) / (100 * c) )) ;; esac
  p="$(fg_probe_x100)"
  case "$p" in ''|*[!0-9]*) ;; *) [ $(( (p + 99) / 100 )) -le "$s" ] || s=$(( (p + 70) / 100 )) ;; esac
  [ "$s" -ge 1 ] || s=1
  [ "$s" -le "$cap" ] || s="$cap"
  printf '%s' "$s"
}
