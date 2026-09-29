#
# hw-restore-env.zsh — put an hw executor's environment back after a herdr crash.
#
# Symlinked to ~/.zsh/hw-restore-env.zsh and sourced from ~/.zshrc by one
# guarded line, in the same style as the bun and VPN blocks already there.
#
# WHY THIS RUNS IN EVERY SHELL
#
# herdr does not persist a pane's environment, and it does not replay the launch
# command either. On restore it SYNTHESIZES `claude --resume <session-id>` and
# types it into a fresh interactive LOGIN zsh in the pane's cwd — the original
# argv, `--model` included, is discarded. So there is no argv
# seam to rebuild `HW_*` through: the wrapper-command proposal is dead. What is
# left is this. A login zsh reads ~/.zshrc, and that was proved from the inside:
# ~/.zshrc-only functions and aliases are defined in a restored pane. So this
# snippet runs BEFORE herdr types the resume command, and the resumed executor
# starts with its environment already in place.
#
# For a DELIBERATE restart none of this is needed — `herdr update --handoff`
# restarts the server with pane processes alive, environment and dev servers
# intact. This file is for crashes and reboots.
#
# THE SAFETY RULES, because this is in the startup path of every terminal
# the user opens. Each one is a refusal, not a warning:
#
#   1. WHITELISTED ROOTS. Only the roots hw actually puts executors in are ever
#      considered: the lane table's `work`, plus each repo-backed lane's own
#      worktree root — derived from bin/project-spaces.sh's
#      `all_lane_worktree_roots()`, not a fixed list; a lane without a repo has
#      no worktree root to add. A shell anywhere else does a handful of string
#      comparisons and returns. Reading `.hw/*/env` from wherever a shell
#      happens to stand would mean any cloned repository carrying one gets a say
#      in every terminal.
#      WHY NOT THE WORK ROOT ALONE: the repo-backed lanes put their executors
#      under their own worktree roots, so a restored executor there would come
#      back without ENGRAM_PROJECT and its `mem_save` would land under a
#      directory-derived label, silently. The roots cover every lane, and that
#      is paid for with rule 7 — which keeps what this file can do small. The
#      derivation reads the same table the rest of hw reads, so a new lane
#      closes its root without an edit here.
#   7. IDENTITY VARIABLES ONLY: `HW_*` and `ENGRAM_*`. Nothing else is applied,
#      whatever the file says. A restored pane needs to know which task it is and
#      where its memory goes — that is what makes the invokers and `mem_save`
#      work. It does NOT need the lane's application variables (PORT,
#      CORS_ORIGINS, DATABASE_URL): non-agent panes come
#      back as bare `-zsh` with nothing running, so a dev server is restarted by
#      hand anyway, and that restart can source the file explicitly — which is
#      the moment a human is present to want it. Restricting the prefix here is
#      what makes rule 1 affordable: this code runs in every terminal, so the set
#      of variables it can define must be one a reader can hold in their head.
#      It also removes the one hazard the file otherwise had — rule 4 protects
#      PATH, which a login shell always has set by the time ~/.zshrc runs, but it
#      does not protect DYLD_INSERT_LIBRARIES or LD_PRELOAD, which are normally
#      unset and would therefore have been applied.
#      ONE DERIVED EXCEPTION, 2026-09-28: an opencode executor's --effort. Its
#      OPENCODE_CONFIG_CONTENT is never read from the file; it is REBUILT from
#      two HW_* fields, each validated — see _hw_restore_opencode_effort.
#   2. IT DOES NOT SOURCE THE FILE. It parses it, with no eval and no `.`, which
#      is why the writer (`_write_run_env` in brain/bin/hw) pins the grammar to
#      one `KEY='value'` per line with inner `'` as `'\''`. That grammar is also
#      what `set -a; . env; set +a` reads, so a human can still restore by hand.
#   3. OWNED BY THE USER, NOT GROUP- OR WORLD-WRITABLE. Ownership is the `U` glob
#      qualifier; the mode is checked with zstat. Either failing is a silent skip.
#   4. NEVER OVERWRITE A SET VARIABLE. A live pane launched with `--env` must
#      win over any file — the file is a fallback for a pane that lost its
#      environment, never an override for one that still has it.
#   5. UPWARD WALK, BOUNDED. The persisted cwd is the pane's LIVE cwd, not its
#      launch cwd (a `cd sub/deeper` is persisted), so a restored pane
#      can be in a subdirectory of the work directory. The walk stops at the
#      whitelisted root, at $HOME, and after 12 hops.
#   6. SILENT AND FAST. Nothing is printed on any path, ever, and nothing fails:
#      a broken ~/.zshrc breaks every new terminal.
#
# The invokers do the same lookup in python (`invoker_adopt_env_file` in
# brain/bin/invoker-common.sh) because they cannot source a zsh function. The two
# now agree on the roots; they still differ in scope, and deliberately — the
# invokers read six named keys, this reads any `HW_*`/`ENGRAM_*`, so a new HW_
# variable reaches a restored shell without editing this file. Change the grammar
# in one and the other two stop reading it: hw writes it, this parses it, the
# invokers parse it.
#

# Read one env file. 0 = the file passed the checks and was read, so the walk
# stops here: the NEAREST env file wins outright and no parent one is layered on
# top of it. Non-zero = it was rejected (wrong owner, writable, unreadable) and
# the walk should keep going up.
_hw_restore_env_file() {
  emulate -L zsh
  local f=$1 line key val
  local -a st

  zmodload -F zsh/stat b:zstat 2>/dev/null || return 1
  zstat -A st +mode -- $f 2>/dev/null || return 1
  # Group- or world-writable. A file anyone else can write is an injection point
  # into every shell the user opens, so it is not read at all.
  #
  # `8#22`, NOT `0022`. **zsh does not read a leading zero as octal** unless
  # OCTAL_ZEROES is set — bash does, and this line was written in bash's dialect.
  # `0022` therefore evaluated as decimal 22, and 0644 & 22 = 4, so this check
  # REJECTED EVERY FILE hw writes (umask 022 → 0644) while accepting 0600. The
  # whole cold-restart restore path was dead, and rule 6 says nothing is ever
  # printed, so it was dead in silence. Found 2026-08-24 while verifying the
  # worktree move — the restore never worked from either root.
  (( st[1] & 8#22 )) && return 1

  while IFS= read -r line; do
    [[ -z $line || $line == '#'* ]] && continue
    key=${line%%=*}
    [[ $key == $line ]] && continue                    # no '=' — not a line we wrote
    [[ -n ${key//[A-Za-z0-9_]/} || $key == [0-9]* ]] && continue
    # Rule 7: identity only. Enforced on the KEY, before the value is even read,
    # so nothing else in the file can ever reach the environment.
    [[ $key == HW_* || $key == ENGRAM_* ]] || continue
    # Rule 4: a variable the pane already has wins. ${(P)key+x} is the indirect
    # form of ${VAR+x} — set-but-empty counts as set, which is correct.
    [[ -n ${(P)key+x} ]] && continue
    val=${line#*=}
    [[ $val == \'*\' ]] || continue                    # not our grammar; skip it
    val=${val[2,-2]}
    val=${val//\'\\\'\'/\'}                            # '\'' -> '
    typeset -gx -- "$key=$val"
  done < $f

  _hw_restore_opencode_effort
  return 0
}

# THE ONE NON-IDENTITY VARIABLE, AND IT IS BUILT, NEVER READ. hw carries an
# opencode executor's --effort as OPENCODE_CONFIG_CONTENT, a whole config layer
# (MCP commands, plugins) — exactly what rule 7 keeps out of every shell. So the
# file's own OPENCODE_CONFIG_CONTENT line is ignored, and the one shape hw
# writes, {"agent":{<name>:{"reasoningEffort":<effort>}}}, is rebuilt here from
# HW_OPENCODE_EFFORT (an enum) and HW_OPENCODE_EFFORT_AGENT ([A-Za-z0-9._-]).
# Anything else in either field builds nothing. Rule 4 still holds: a pane that
# kept its own OPENCODE_CONFIG_CONTENT wins.
_hw_restore_opencode_effort() {
  emulate -L zsh
  [[ -n ${OPENCODE_CONFIG_CONTENT+x} ]] && return 0
  [[ ${HW_OPENCODE_EFFORT-} == (low|medium|high|xhigh|max) ]] || return 0
  [[ -n ${HW_OPENCODE_EFFORT_AGENT-} && -z ${HW_OPENCODE_EFFORT_AGENT//[A-Za-z0-9._-]/} ]] || return 0
  typeset -gx OPENCODE_CONFIG_CONTENT="{\"agent\":{\"$HW_OPENCODE_EFFORT_AGENT\":{\"reasoningEffort\":\"$HW_OPENCODE_EFFORT\"}}}"
}

# Walk up from $PWD inside the whitelisted root and apply the newest env file
# found. Callable by hand, which is how it was verified.
_hw_restore_env() {
  emulate -L zsh
  # Rule 1: the roots hw actually launches executors in.
  #
  # DERIVED FROM brain/bin/project-spaces.sh's all_lane_worktree_roots(), not
  # hand-copied. That file is plain case/printf/functions — no bashisms — so it
  # sources cleanly into this zsh (same output under `bash -c` and `zsh -c`).
  # The invokers derive the same roots, so the three readers agree by
  # construction rather than by three hand-typed copies.
  #
  # `${(f)...}` splits the function's newline-separated output into an array —
  # zsh's own idiom, since plain word-splitting on an unquoted expansion does
  # NOT happen here (no SH_WORD_SPLIT). Falls back to the brain's sibling
  # `work` alone on ANY failure to source or on empty output: this runs in every
  # terminal the user opens, and a broken derivation must not silently disable
  # the whole file.
  #
  # brain is where THIS file lives, links followed (~/.zsh/hw-restore-env.zsh
  # links to brain/shell/), and project-spaces.sh reads its lane table from
  # <brain>/projects.json. The work root is the table's `work`: WORK is unset in
  # the subshell so a stray WORK in the terminal cannot move it.
  local -a roots
  local _brain=${${(%):-%x}:A:h:h}
  local _psf=$_brain/bin/project-spaces.sh
  if [[ -r $_psf ]]; then
    local _derived
    _derived="$(
      emulate -L sh
      unset WORK
      . $_psf
      lane_config_load "$_brain" >/dev/null 2>&1 \
        && typeset -f all_lane_worktree_roots >/dev/null 2>&1 \
        && [ -n "$WORK" ] && printf '%s\n' "$WORK" && all_lane_worktree_roots
    )" 2>/dev/null
    [[ -n $_derived ]] && roots=("${(@f)_derived}")
  fi
  (( ${#roots} == 0 )) && roots=( ${_brain:h}/work )
  # Compare CANONICAL paths on both sides. `:A` resolves symlinks and, on a
  # case-insensitive filesystem, returns the true on-disk case — which plain
  # $PWD does not. herdr canonicalizes a pane's cwd, so a root spelled with the
  # wrong case matches nothing and the whole root goes silently dead. That is
  # what happens to a root whose literal spelling says `Projects` while the disk
  # says `projects`: every comparison misses, and a restored executor there
  # comes back with no environment and no error.
  local root= r cpwd=${PWD:A}
  for r in $roots; do
    r=${r:A}
    if [[ $cpwd == $r/* || $cpwd == $r ]]; then root=$r; break; fi
  done
  # The whole cost of this file for a shell that is not under one of them.
  [[ -n $root ]] || return 0

  local dir=$PWD f
  local -i hops=0
  while (( hops < 12 )); do
    # N nullglob, . regular files only, U owned by us, om newest first.
    for f in $dir/.hw/*/env(N.Uom); do
      _hw_restore_env_file $f && return 0
    done
    [[ $dir == $root || $dir == $HOME || $dir == / ]] && break
    dir=${dir:h}
    (( hops++ ))
  done
  return 0
}

_hw_restore_env
