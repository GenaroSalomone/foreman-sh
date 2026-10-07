# lib/hw/done.sh — `hw done`.
#
# Sourced ONCE by bin/hw (from $HW_BIN_DIR/../lib/hw/done.sh), right after
# status.sh. It is a library: no shebang, nothing runs on source except function
# and variable definitions. Moved VERBATIM from bin/hw (parent fcf6d56f): the
# verification hw runs at close (_verify_dispatch_base .. _verify_at_close), the
# "a pane has an owner" guard (_done_*), _DONE_VERIFY_SECONDS and cmd_done.
# Stayed in bin/hw: _verify_names_full_suite (_dispatch_manifest also calls it),
# _VERIFY_TIMEOUT (the dispatch manifest and the suite lock read it) and
# _snapshot_done_report (cmd_receipt and the outbox repair call it too).

# ── the verification hw runs itself ─────────────────────────────────────────
#
# THE REPORT STOPS BEING THE EVIDENCE.
#
# On 2026-08-25 three executors in a row delivered correct code and a false
# process, and all three reports had to be corrected by hand. One ran the
# pre-commit, saw it red, and pushed past it with `env -u ... git commit`. One
# reported "Post-fix inherited: 173 tests passed" — the suite failed in the
# direction it claimed to have run, and it had never been run that way. One
# called a port complete because the diff "was the whole fix"; it had a hole.
#
# The pattern is not the model. It is that a report is PROSE, and prose has no
# cost of lying: writing "173 tests passed" costs less than running it, and far
# less than running it in both directions. The only defence was a human
# re-measuring everything by hand, which is what cost that session.
#
# So hw runs it. The commands come from the brief, pinned at dispatch into
# `.hw/<run>/verify` before the agent existed; hw executes them here, in the
# task's own directory, and writes the exit code and a hash of the output to
# the receipt. The agent is not asked and its report is not read — an executor
# that types "all green" produces no line here, and a missing line is now the
# visible thing it always should have been.
#
# NOT A GATE. A failing verification is recorded and said out loud; it does not
# block the close, invent a lifecycle state, or hold the worktree. The human
# decides what a red means. hw's job is that the red exists at all.
# A CLOSE THAT HAS ALREADY BEEN MEASURED DOES NOT MEASURE AGAIN.
#
# MEASURED 2026-10-06 on five stale setup tasks: verification 67, 176, 294, 396
# and 901s, everything else 0-1s. The pinned command is nearly always
# `HW_TEST_GATE=fast bash setup/test-hw`, and that suite already leaves a
# verdict keyed by the exact tree (`<git-common-dir>/hw-push-verified/<tree>.fast`,
# or `<tree>` for a full run), the cache pre-push reads. Re-running it for a
# tree that has a green verdict re-measures a fact on disk; re-running it for a
# task that never changed anything measures the base, not the task.
#
# Two shortcuts, both only on a CLEAN checkout, both a receipt line that says
# which one — never a silent pass:
#   skipped — HEAD is the review_base recorded at dispatch: nothing to verify.
#   cached  — the pin is exactly the fast gate, the full suite or verify-for-push and a green
#             verdict names HEAD's tree — or a FULL green one names a tree that
#             differs from it only in lane docs (_verify_docs_cover).
# EVERY DOUBT IS A MISS, and a miss runs the command as before: not a git
# checkout, a dirty tree (tracked or untracked), no recorded base, a pin that is
# more than one of those two commands, a verdict that cannot be read, names
# another tree, carries the wrong gate or says anything failed.
# HW_VERIFY_FRESH=1 forces the run.
_verify_dispatch_base() {  # $1 = run directory -> the review_base sha recorded at dispatch, or nothing
  jq -r 'select(.key=="review_base") | .value' "$1/receipt.jsonl" 2>/dev/null \
    | tail -1 | rg -x '[0-9a-f]{40}' || true
}
_verify_cached_verdict() {  # $1 = common dir, $2 = tree, $3 = command -> prints "<tree> <when>" on a green hit
  local dir="$1/hw-push-verified" tree="$2" cmd="$3" f gate
  local -a cands=()
  case "$cmd" in
    "HW_TEST_GATE=fast bash setup/test-hw") cands=("$dir/$tree.fast" "$dir/$tree") ;;
    "bash setup/test-hw"|"bash setup/verify-for-push") cands=("$dir/$tree") ;;  # MUTATION-ANCHOR: 770-M04
    *) return 1 ;;
  esac
  for f in "${cands[@]}"; do
    [ -r "$f" ] || continue
    rg -qx "tree=$tree" "$f" 2>/dev/null || continue
    # RED OR UNSAID IS NOT GREEN: only a green run is ever written, but a file
    # that says otherwise is not read as a pass.
    if rg -qi '^(headline|result|status)=.*(fail|\bred\b)' "$f" 2>/dev/null; then continue; fi  # MUTATION-ANCHOR: 770-M03
    gate=full; rg -qx 'gate=fast' "$f" 2>/dev/null && gate=fast
    case "$f:$gate" in
      *.fast:fast) ;;
      *.fast:*)    continue ;;
      *:fast)      continue ;;
    esac
    printf '%s %s' "$tree" "$(sed -n 's/^verified_at=//p' "$f" | head -1)"
    return 0
  done
  return 1
}
# THE SECOND COVER: a tree with no verdict of its own that differs from a FULL
# green one only in lane documentation. MEASURED 2026-10-06: `hw done` ran
# `bash setup/verify-for-push` for 901s and died 124 on a tree that pre-push
# itself accepts, "differs only in lane documentation from tree 2dae21…". The
# rule is setup/hooks/suite-trigger-pattern.sh's docs_only_diff_covers, the same
# function pre-push calls — not a second copy. Only a full verdict is a base
# (a fast one never covers anything here), the tree is already known clean, and
# every doubt — no lib, no table, an unreadable verdict, a path outside the
# list — is a miss.
# prints "<verdict tree>\t<commit>\t<paths>" on a hit
_verify_docs_cover() {  # $1 = worktree, $2 = common dir, $3 = tree
  local lib="${BRAIN:-${HW_BRAIN_ROOT:-}}/setup/hooks/suite-trigger-pattern.sh"
  [ -r "$lib" ] || return 1
  (
    cd "$1" || exit 1
    . "$lib" || exit 1
    [ -n "${SUITE_TRIGGER_PATTERN:-}" ] || exit 1
    declare -F docs_only_init >/dev/null && declare -F docs_only_diff_covers >/dev/null || exit 1  # a lib that predates the cover is a miss
    docs_only_init "$(git rev-parse --show-toplevel)" || exit 1
    for f in "$2/hw-push-verified"/*; do
      v="$(basename "$f")"
      case "$v" in *[!0-9a-f]*|'') continue ;; esac
      [ "${#v}" -eq 40 ] || continue
      [ -r "$f" ] || continue
      grep -qxF "tree=$v" "$f" 2>/dev/null || continue
      grep -qxF "gate=fast" "$f" 2>/dev/null && continue
      if grep -qiE '^(headline|result|status)=.*(fail|\bred\b)' "$f" 2>/dev/null; then continue; fi
      git cat-file -e "$v^{tree}" 2>/dev/null || continue
      paths="$(docs_only_diff_covers "$v" "$3")" || continue  # MUTATION-ANCHOR: 770-M05
      printf '%s\t%s\t%s' "$v" "$(sed -n 's/^commit=//p' "$f" | head -1)" "$paths"
      exit 0
    done
    exit 1
  )
}
_verify_shortcut() {  # $1 = run directory, $2 = worktree, $3 = pin file. 0 = decided and receipted, 1 = run it
  local rundir="$1" wd="$2" pin="$3" head base dirty tree common cmd hit lines
  [ -z "${HW_VERIFY_FRESH:-}" ] || return 1
  [ -e "$wd/.git" ] || return 1
  head="$(git -C "$wd" rev-parse --verify -q 'HEAD^{commit}' 2>/dev/null)" || return 1
  [ -n "$head" ] || return 1
  dirty="$(git -C "$wd" status --porcelain --untracked-files=normal 2>/dev/null)" || return 1
  [ -z "$dirty" ] || return 1  # MUTATION-ANCHOR: 770-M02
  base="$(_verify_dispatch_base "$rundir")"
  if [ -n "$base" ] && [ "$head" = "$base" ]; then  # MUTATION-ANCHOR: 770-M01
    _receipt_into "$rundir" verify_run "skipped — no change since dispatch ($base)" \
      "HEAD == review_base recorded at dispatch and git status is clean in $wd; the pinned verification was not run"
    info "verification skipped — no change since dispatch ($base); nothing here claims more than that"
    return 0
  fi
  lines="$(rg -c '\S' "$pin" 2>/dev/null || true)"
  [ "$lines" = 1 ] || return 1
  cmd="$(rg '\S' "$pin" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//' || true)"
  tree="$(git -C "$wd" rev-parse --verify -q 'HEAD^{tree}' 2>/dev/null)" || return 1
  common="$(git -C "$wd" rev-parse --git-common-dir 2>/dev/null)" || return 1
  case "$common" in /*) ;; *) common="$wd/$common" ;; esac
  if ! hit="$(_verify_cached_verdict "$common" "$tree" "$cmd")"; then
    case "$cmd" in
      "HW_TEST_GATE=fast bash setup/test-hw"|"bash setup/test-hw"|"bash setup/verify-for-push") ;;
      *) return 1 ;;
    esac
    local cover vt vc vp
    cover="$(_verify_docs_cover "$wd" "$common" "$tree")" || return 1
    IFS=$'\t' read -r vt vc vp <<<"$cover"
    _receipt_into "$rundir" verify_run \
      "cached — green full verdict for tree $vt ($vc); differs only in lane docs: $vp" \
      "$common/hw-push-verified/$vt is a full green verdict; HEAD's tree $tree differs from it only in lane documentation (docs_only_diff_covers), $wd is clean; $cmd was not re-run"
    ok "VERIFIED (cached)  green full verdict for tree $vt ($vc); differs only in lane docs: $vp  $cmd"
    return 0
  fi
  _receipt_into "$rundir" verify_run \
    "cached — green verdict for tree ${hit% *} from ${hit#* }" \
    "$common/hw-push-verified/${hit% *}[.fast] named HEAD's tree, $wd is clean; $cmd was not re-run"
  ok "VERIFIED (cached)  green verdict for tree ${hit% *} from ${hit#* }  $cmd"
  return 0
}


_verify_at_close() {  # $1 = run directory, $2 = directory to run in, $3/$4 = project/task
  local rundir="$1" workdir="$2" vproj="${3:-}" vtask="${4:-}" pin n=0 rc out hash log to=""
  local verifydir="$workdir"
  # SETUP USED TO BE REDIRECTED TO THE MAIN CHECKOUT HERE, and that redirect
  # expired on 2026-09-16. It existed because a setup work directory held
  # artifacts and no code, so `bash setup/test-hw` had nothing to run; now that
  # directory IS a worktree of brain and running the pinned verification
  # anywhere else would measure a tree the task never touched — the exact
  # "green on the wrong base" failure the --base field was added to stop.
  #
  # The redirect survives for a setup task that is NOT a worktree: a
  # `--worktree none` dispatch, or one of the plain directories every setup task
  # before that date left behind. There the old reason still holds exactly.
  if lane_worktree_is_workdir "$vproj" && [ ! -e "$workdir/.git" ]; then
    verifydir="$(_lane_main_checkout "$vproj")"
    [ -n "$verifydir" ] || verifydir="${BRAIN:-${HW_BRAIN_ROOT:-}}"  # an extracted harness may not set BRAIN
  fi
  # `fd` hands back a trailing slash, which made every recorded log path read
  # `.hw/<run>//verify-1.log`. A path in a receipt is meant to be pasted.
  rundir="${rundir%/}"
  [ -n "$rundir" ] && [ -d "$rundir" ] || return 0
  # A CLOSE THAT NOBODY NEEDS VERIFIED SAYS SO, in the receipt. `--force` is what
  # a human reaches for when a close looks stuck, and paying the whole
  # verification again made it the slowest way out; a `--blocked` report has no
  # finished work to verify. Both are decided by the caller, not inferred here.
  if [ -n "${DONE_VERIFY_SKIP:-}" ]; then
    _receipt_into "$rundir" verify_run "skipped (${DONE_VERIFY_SKIP})" \
      "hw done $DONE_VERIFY_SKIP: the pinned verification was not run"
    info "verification skipped (${DONE_VERIFY_SKIP}) — nothing here claims this task was verified"
    return 0
  fi
  pin="$rundir/verify"
  # NO PIN AT ALL is a different fact from AN EMPTY PIN, and only the second is
  # worth a receipt: runs that predate this have no file, and inventing a
  # measurement for them would be the defect this function exists to remove.
  [ -f "$pin" ] || return 0
  if [ ! -s "$pin" ]; then
    _receipt_into "$rundir" verify_run \
      "none — the brief declared no runnable command" \
      "$pin, pinned from the brief at dispatch, is empty"
    warn "NOT VERIFIED: this task's brief declared no command hw could run"
    warn "nothing here says the work was checked — the report is the only claim there is"
    return 0
  fi
  [ -d "$verifydir" ] || {
    _receipt_into "$rundir" verify_run \
      "not run — $verifydir does not exist" "test -d $verifydir at hw done"
    warn "NOT VERIFIED: $verifydir is gone, so the pinned verification could not run"
    return 0
  }
  # BEFORE THE LOCK AND THE MACHINE SLOT: a close that is decided from disk has
  # nothing to wait for. Only where the measured tree IS the task's worktree —
  # the main-checkout redirect above measures a tree this task never owned.
  if [ "$verifydir" = "$workdir" ] && declare -F _verify_shortcut >/dev/null 2>&1 \
     && _verify_shortcut "$rundir" "$workdir" "$pin"; then
    info "measured: hw receipt $vproj $vtask"
    return 0
  fi
  # `hw done` may be called by the brainer, not the executor. Reconstruct the
  # task-owned artifacts location from the same project/workdir rule used at
  # dispatch, so an unset (or stale) caller environment cannot become a false
  # verification failure. Verification commands currently pin no executor-only
  # HW_* inputs beyond this durable path.
  HW_ARTIFACTS="$(_artifacts_dir_for "$vproj" "$workdir")"
  export HW_ARTIFACTS
  # A verification that hangs would hang `hw done`, and hw done is how a task
  # leaves the board. Bounded, and the bound is stated in the source line.
  # `${_VERIFY_TIMEOUT:-900}`, not the bare variable: an empty duration makes
  # `timeout` exit 125 without ever running the command, and 125 would have been
  # recorded as the verification's own exit code — a measurement of nothing,
  # indistinguishable from a real failure. Caught by driving this function
  # outside bin/hw, which is how the suite drives everything.
  local secs="${_VERIFY_TIMEOUT:-900}"
  if command -v timeout >/dev/null 2>&1; then to="timeout $secs"
  elif command -v gtimeout >/dev/null 2>&1; then to="gtimeout $secs"; fi
  # ── AND IT RUNS BEHIND THE LANE'S SUITE LOCK, the same one `hw suite` takes ──
  #
  # MEASURED: it did not. `hw suite <lane> -- <cmd>`
  # existed, took a per-lane lock and reclaimed a dead holder's orphan, and the
  # close verification ran its pinned command raw beside it. The command a brief
  # pins is very often that lane's own suite — so two `hw done`s in one lane, or
  # one close while somebody runs the suite by hand, ran over each other, on a
  # lane where several executors share ONE tree and ONE index. A lock a human
  # gets and the tool does not is a lock with a hole in it.
  #
  # NOT A GATE, AND THE BUDGET SAYS SO. `_suite_lock_take` never dies; a lane
  # busy for the whole budget is recorded as NOT VERIFIED and the close
  # continues, because `hw done` is how a task leaves the board and a lock must
  # never be what keeps it on. The budget defaults to the verification's own
  # timeout — waiting longer for a turn than the command is allowed to take is
  # its own absurdity — and is injectable for the same reason every other clock
  # in this file is.
  #
  # WHAT MUST NOT BE BROKEN, because it is not obvious and it was measured:
  # a suite that has been orphaned (ppid=1) can still reach green and still have
  # its verdict recovered, because its output goes to a real file. So a lock
  # whose only question is "is a process alive" loses that case.
  # `_suite_holder_alive` already answers a narrower question — pid AND process
  # start time, with the claim window aged rather than assumed — and this reuses
  # it unchanged rather than inventing a second liveness rule here.
  #
  # A LOCK THAT CANNOT BE CREATED IS NOT CONTENTION, and the difference is the
  # whole point of this block. `_suite_lock_take` waits out its budget on ANY
  # `mkdir` failure, and an unwritable parent fails the same way a held lock
  # does — so without this branch a broken `$WORK` would be reported as
  # "somebody else is running the suite", which is a fact hw would be inventing.
  # Named, and the verification runs unlocked rather than not at all: an
  # unprovable lock is a reason to say so, not a reason to measure nothing.
  #
  # THE CEILING, STATED RATHER THAN IMPLIED. The comment above says this
  # function is bounded; with a lock in front of it the bound is now
  # `vwait` PLUS `secs` per pinned command, not `secs`. Default 900 + 900. That
  # is deliberate — a lane suite runs ~1000s, so a wait much shorter than one
  # run would mean a close almost never gets its turn — and it is not a silent
  # hang: `_suite_lock_take` announces the wait ONCE, names the holder and
  # prints the budget. `HW_VERIFY_SUITE_WAIT_MS` moves it.
  #
  # THE $WORK CHECK COMES FIRST, and the order is the whole point.
  # `_suite_lock_dir` dereferences `$WORK` bare, so resolving the path before
  # checking it would abort the process under `set -u` instead of reaching the
  # unserialised fallback this branch exists to provide. Raised by a blind
  # adversarial reviewer on this candidate; today `cmd_done` happens to
  # dereference `$WORK` earlier anyway, which makes it unreachable rather than
  # correct, and unreachable is not a property to build on.
  local vlock="" vheld=0 vwait vlock_prev="${HW_SUITE_LOCK_HELD:-}"
  vwait="${HW_VERIFY_SUITE_WAIT_MS:-$(( secs * 1000 ))}"
  # SAID BEFORE ANY WAIT, INCLUDING THE LOCK'S: this is the step that makes
  # `hw done` slow, and nothing has closed while it runs. A caller whose tool gives
  # up at 120s should know what it is waiting for, that the pane is still open,
  # and both bounds.
  info "hw done is now running this task's verification — the slow step; NOTHING is closed until it ends. Ceiling ${secs}s per command, plus up to $(( vwait / 1000 ))s first if another run holds the lane's suite lock, plus up to ${HW_SUITE_GATE_WAIT_MS:-7200000}ms for a machine suite slot (bin/suite-gate; never a failure, it runs without one past that)."
  if _suite_lane_pooled "$vproj"; then
    info "the $vproj lane has no suite lock: its suite shares the machine's worker pool with every other suite and runs its timing subjects alone — see _suite_lane_pooled. This verification takes a machine suite slot and may wait for one."
  elif [ -z "${WORK:-}" ]; then
    warn "the $vproj suite lock could not be located — WORK is unset, so this verification runs UNSERIALISED"
    warn "if somebody else is running that lane's suite right now, these two are on the same tree"
  else
    vlock="$(_suite_lock_dir "$vproj")"  # MUTATION-ANCHOR: 142-M01
    if ! mkdir -p "$(dirname "$vlock")" 2>/dev/null; then
      warn "the $vproj suite lock could not be created under $WORK — this verification runs UNSERIALISED"
      warn "if somebody else is running that lane's suite right now, these two are on the same tree"
      vlock=""
    fi
  fi
  if [ -z "$vlock" ]; then
    :
  elif _suite_lock_held_by_ancestor "$vlock" "$vproj"; then
    # Already inside somebody's hold — see _suite_lock_held_by_ancestor.
    info "the $vproj suite lock is already held further up this process tree; this verification runs inside that hold"
    vlock=""
  elif _suite_lock_take "$vlock" "$vproj" "$vwait" "hw done verification of ${vtask:-<unnamed task>}"; then
    vheld=1
    # THE INT/TERM ARMS EXIT, and that is not decoration. A trap whose body
    # does not exit turns a delivered signal into "run this, then carry on
    # from where you were" — so Ctrl-C during a locked verification would
    # release the lock and let the close continue, which is the opposite of
    # what the person pressing it asked for. Raised by both blind reviewers on
    # this candidate. 130/143 are the conventional 128+signal codes.
    # shellcheck disable=SC2064
    trap "rm -rf '$vlock' 2>/dev/null || true; sg_release 2>/dev/null || true" EXIT
    # shellcheck disable=SC2064
    trap "rm -rf '$vlock' 2>/dev/null || true; exit 130" INT
    # shellcheck disable=SC2064
    trap "rm -rf '$vlock' 2>/dev/null || true; exit 143" TERM
    # THE MARKER A NESTED `hw suite` READS, exported only while the lock is
    # really held. See _suite_lock_held_by_ancestor for why a pid rides along.
    export HW_SUITE_LOCK_HELD="$vproj:$SUITE_LOCK_TOKEN"
    ok "holding the $vproj suite lock for this close's verification (pid $$)"
  else
    _receipt_into "$rundir" verify_run \
      "not run — the $vproj suite lock was held by a live runner for the whole $(( vwait / 1000 ))s budget" \
      "hw done waited for $vlock and did not get it; $(_suite_holder_line "$vlock")"
    warn "NOT VERIFIED: the $vproj suite lock is held by a live runner — $(_suite_holder_line "$vlock")"
    warn "hw done will NOT run this task's verification over somebody else's suite. Nothing here says the work was checked."
    warn "when that runner is done: hw suite $vproj -- <the command in $pin>"
    info "measured: hw receipt $vproj $vtask"
    return 0
  fi
  # ONE SLOT OF THE MACHINE-WIDE GATE FOR THE WHOLE VERIFICATION, taken AFTER
  # the lane lock (always that order) and never failing the close: a wait past
  # HW_SUITE_GATE_WAIT_MS runs the commands without a slot and says so.
  if declare -F sg_take >/dev/null 2>&1; then SG_WORK="${WORK:-}" sg_take "hw done verification of ${vtask:-<unnamed task>}" || true; fi  # MUTATION-ANCHOR: 730-M01
  while IFS= read -r cmd; do
    [ -n "$cmd" ] || continue
    n=$((n + 1))
    log="$rundir/verify-$n.log"
    # THE ONE COMMAND SHAPE THIS TIMEOUT CANNOT SERVE: the full suite, named
    # without the fast gate. `secs` above is sized for the fast gate (or a
    # brief's own quick check); the full suite is ~1000s of real work and
    # belongs to `setup/hooks/pre-push`, mandatorily, not to a synchronous
    # `hw done` step. Warn rather than refuse — a human may have a reason —
    # but say it where the mismatch is about to be paid for, not just in a
    # comment nobody reads until it times out.
    if _verify_names_full_suite "$cmd"; then
      warn "verify command names the full suite without HW_TEST_GATE=fast — that is ~1000s against a ${secs}s timeout, and it will read as a failed verification, not a slow one. Full-suite coverage is already mandatory at push (setup/hooks/pre-push); point this brief's verify at the fast gate instead: HW_TEST_GATE=fast bash setup/test-hw"
      warn "this was also said at dispatch, where the brief could still be edited for free"
    fi
    info "verifying: $cmd"
    rc=0
    out="$( cd "$verifydir" && $to bash -c "$cmd" 2>&1 )" || rc=$?
    printf '%s\n' "$out" > "$log" 2>/dev/null || true
    hash="$(printf '%s' "$out" | shasum -a 256 | cut -d' ' -f1)"
    _receipt_into "$rundir" verify_run \
      "exit $rc  sha256 $hash  $cmd" \
      "hw done ran: (cd $verifydir && $to bash -c '$cmd'); output in $log"
    if [ "$rc" = 0 ]; then
      ok "VERIFIED  exit 0  $cmd"
    else
      warn "VERIFICATION FAILED  exit $rc  $cmd"
      warn "the receipt records the failure; nothing here claims this task was verified"
      warn "the output is in $log"
    fi
  done < "$pin"
  if declare -F sg_release >/dev/null 2>&1; then sg_release; fi
  if [ "$vheld" = 1 ]; then
    rm -rf "$vlock" 2>/dev/null || true
    trap - EXIT INT TERM
    # RESTORED, not merely cleared: hw done may itself have been launched from
    # inside somebody's hold, and dropping the marker would let the NEXT nested
    # call claim a lock its ancestor is holding.
    if [ -n "$vlock_prev" ]; then export HW_SUITE_LOCK_HELD="$vlock_prev"
    else unset HW_SUITE_LOCK_HELD; fi
    ok "released the $vproj suite lock"
  fi
  info "measured: hw receipt $vproj $vtask"
  return 0
}

# ── a pane has an owner, and hw done must establish it before closing ───────
#
# THE INCIDENT. A brainer ran `hw done` on a
# pane THE OPERATOR WAS USING AT THAT MOMENT. The session went with it. It came back
# through `hw revive` — and nobody in the room knew that command existed until
# after the panic, which is the other half of the defect: an emergency exit
# announced after the fire is not an exit.
#
# WHAT hw KNEW AND DID NOT ASK. `hw done` closed the tab on the strength of a
# label matching `<proj>:<task>`. Whether anything was happening inside it was
# never a question — not whether the task had reported, not whether a turn was
# running, not whether turns had ended in there that hw never dispatched.
#
# THE ORIGINAL PRESCRIPTION FOR THIS GUARD WAS `turns`, AND IT DOES NOT WORK.
# Stated here because the next reader will reach for it too. `turns` counts
# turn-ENDS (`cmd_executor_turn_end`), not hw dispatches: the turn-end hook
# itself returns 8 and forces another one, so one brief routinely produces
# several. MEASURED over the 189 `.hw/<run>/turns` files under $WORK:
# 118 of them are at 2 or more with the task counter still at 1, the largest at
# 233. "turns hw did not dispatch" = turns − dispatches would have refused ~62%
# of every close ever made, and a guard that needs `--force` everywhere is a
# guard nobody reads. Worse, the counter FROZE at the report (that branch
# returned before incrementing), so the post-report window — the incident's own
# window — produced no turns at all. The counter was wrong in both directions.
#
# SO THE FACTS THIS READS ARE THE ONES THAT DISCRIMINATE, three of them, each
# named separately in the refusal because they mean different things:
#
#   · NO REPORT. Derived by `_report_evidence`, the same function `hw next`
#     uses — one derivation of "reported", two callers. A second answer to that
#     question is what produced the 2026-08-24 contradiction it was written for.
#   · agent_status=working. A turn is running RIGHT NOW and the close would cut
#     it mid-work. hw dispatched none of it: after a report `hw next` is the
#     only re-task route and it advances the task counter first.
#   · turns that ended AFTER the report. Counted since 2026-09-16 beside
#     `turns`, on disk at `.hw/<run>/turns-post-report` and republished as the
#     `turns_post_report` token. Disk first, token second — the same two-signal
#     discipline as the report evidence, because a token carries a TTL and a
#     file does not.
#
# `focused` IS EVIDENCE, NEVER A TRIGGER. It says a human is LOOKING at the
# pane, and exactly one pane on the machine can hold it. "Nobody is looking"
# is not "nobody is using it" — a human who switched tabs for thirty seconds
# would be silently overruled by a guard that keyed on it. So it is reported
# when true, and it never decides.
#
# `--force` is the one exit, and it is meant to cost a sentence of thought:
# every refusal below names it, and taking it says the closer established
# ownership some other way.
_done_pane_for_run() {  # $1=rundir
  [ -r "$1/receipt.jsonl" ] || return 0
  jq -r 'select(.key=="pane") | .value' "$1/receipt.jsonl" 2>/dev/null | tail -1 || true
}
# Turns that ended in a pane after the report of task $2, disk first and the
# token as fallback (see the guard's comment above). Empty when none.
_done_post_report_turns() {  # $1=rundir $2=seq $3=herdr agent-get json
  local rundir="$1" seq="$2" info="$3" post="" rec rec_seq tok tok_seq
  if [ -r "$rundir/turns-post-report" ]; then
    rec="$(cat "$rundir/turns-post-report" 2>/dev/null || true)"
    rec_seq="${rec%% *}"
    [ "$rec_seq" = "$seq" ] && post="$(printf '%s' "${rec##* }" | tr -dc '0-9' || true)"
  else
    tok="$(printf '%s' "$info" | jq -r '.result.agent.tokens.turns_post_report // empty' 2>/dev/null || true)"
    tok_seq="${tok%% *}"
    [ -n "$tok" ] && [ "$tok_seq" = "$seq" ] && post="$(printf '%s' "${tok##* }" | tr -dc '0-9' || true)"
  fi
  case "$post" in ''|0) post="" ;; esac
  printf '%s' "$post"
}
_done_in_use_guard() {  # $1=project $2=task $3=rundir
  local proj="$1" task="$2" rundir="$3"
  local pane info status focus post ev seq reasons="" revive_note _has_agent=0
  if [ "${DONE_FORCE:-0}" = 1 ]; then
    warn "--force: closing $proj:$task WITHOUT establishing that its pane is free"
    return 0
  fi
  # Each of these is "hw has nothing to read", not "the pane is free", and they
  # are deliberately not refusals: hw done must still close a task whose run
  # record, pane receipt or pane itself is gone, which is most old tasks.
  [ -n "$rundir" ] || { info "no hw run record for $proj:$task, so there is no pane to establish ownership of"; return 0; }
  pane="$(_done_pane_for_run "$rundir")"
  [ -n "$pane" ] || { info "run $(basename "$rundir") recorded no pane receipt, so hw cannot tell whose pane this was"; return 0; }
  # A SELF-CLOSE IS NOT AN INTERRUPTION, AND THIS GUARD WOULD HAVE BROKEN EVERY
  # ONE OF THEM. MEASURED 2026-09-16, on the executor writing this: `done-invoker`
  # closes its own pane by calling `hw done <proj> <task>` from INSIDE it
  # (bin/done-invoker, both the lease-fallback and the ordinary close), and at
  # that instant herdr reports that pane `agent_status=working` — because the
  # turn running done-invoker IS a turn:
  #
  #     pane w7G:p9Y -> {"agent_status":"working","focused":true}
  #
  # So the `working` arm below would refuse every non-chained close, done-invoker
  # would take its delivered-but-not-closed exit, and delivered task tabs would
  # accumulate open forever — the exact failure that path exists to prevent,
  # reintroduced by the guard meant to protect panes.
  #
  # SO THE EXEMPTION IS SCOPED TO THE ARMS A SELF-CLOSE CAUSES, and only those:
  # `working` and the post-report turn count. It does NOT cover the unreported
  # arm.
  #
  # THE FIRST VERSION OF THIS FIX RETURNED UNCONDITIONALLY, and that was wrong.
  # Caught by a blind judge in the scoped re-judgment and reproduced before it
  # was accepted: `HERDR_PANE_ID` is baked into EVERY pane at spawn, so the
  # exemption keyed on "somebody is closing their own pane", not on
  # "done-invoker is closing after reporting". An agent abandoning its task and
  # running `hw done <proj> <task>` by hand, with no report ever written, closed
  # silently — while the identical close from any other pane was refused. That
  # defeats the guard's primary arm for the one caller most able to skip it.
  #
  # `done-invoker` is unaffected: it writes the done marker BEFORE it ever
  # reaches the close, so the unreported arm has nothing to fire on there.
  #
  # AND setup/tests/24-done-closes-delivered.sh DID NOT CATCH THE ORIGINAL
  # REGRESSION EITHER, which is the more useful half. Its herdr stub is
  # `exit 0` — it answers nothing, so the guard sees no agent object and
  # abstains, and the test passed for the wrong reason. setup/CLAUDE.md's own
  # rule: the stubbed path is not the path.
  local self_close=0
  if [ -n "${HERDR_PANE_ID:-}" ] && [ "$pane" = "$HERDR_PANE_ID" ]; then  # MUTATION-ANCHOR: 150-M05
    self_close=1
  fi
  # AND AN UNANSWERED HERDR IS SAID OUT LOUD, NOT SWALLOWED. Blind judge A: a
  # transient herdr failure while a human is working in the pane leaves `info`
  # empty, the guard abstained SILENTLY, and the close went through — the
  # incident's own failure mode reopened by a different trigger.
  #
  # It stays a WARNING and not a refusal, deliberately. `hw done` must keep
  # closing tasks whose pane is genuinely long gone, which is most old tasks,
  # and this call cannot tell "herdr did not answer" from "there is no such
  # agent". What is fixed is the SILENCE: a close that could not establish
  # ownership now says so, instead of reading like one that established it.
  # ASKED OF THE STRUCTURE, NOT OF THE TEXT. This was a substring match for
  # `"agent"` on the raw reply, which any unrelated JSON carrying that word
  # would satisfy and any reshaped herdr reply could break. Scoped re-judgment.
  info="$(herdr agent get "$pane" 2>/dev/null || true)"
  _has_agent=0
  printf '%s' "$info" | jq -e '.result.agent' >/dev/null 2>&1 && _has_agent=1
  if [ -z "$info" ] || [ "$_has_agent" = 0 ]; then
    warn "could NOT establish whether pane $pane is free: herdr returned nothing usable for it. It may be long gone, or herdr may simply not have answered — this close cannot tell those apart, and is proceeding WITHOUT the check."
    return 0
  fi

  seq="$(_chaining_current_task "$rundir")"
  ev="$(_report_evidence "$pane" "$rundir")"
  status="$(printf '%s' "$info" | jq -r '.result.agent.agent_status // empty' 2>/dev/null || true)"
  focus="$(printf '%s' "$info" | jq -r '.result.agent.focused // empty' 2>/dev/null || true)"
  # DISK FIRST. The file is written by the executor's own turn-end hook and
  # outlives the token, which carries a 24h TTL.
  # THE RECORD IS `<seq> <count>`, and the seq is load-bearing: a chained
  # session's task 2 must not be refused over task 1's post-report turns. A
  # record whose seq is not this task's says nothing about this task.
  # THE FILE IS AUTHORITATIVE WHEN IT EXISTS, and the token is only a fallback
  # for when the run directory is gone. That ordering is load-bearing: a
  # chained session's record reads `1 3` while task 2 is being closed, the seqs
  # do not match — and falling through to the token there would hand `hw done`
  # task 1's stale count and refuse task 2's ordinary close, which is the very
  # bug the seq exists to prevent, rebuilt one line lower.
  post="$(_done_post_report_turns "$rundir" "$seq" "$info")"
  # A COUNT OF 1 WHILE THE PANE STILL SAYS `working` IS THE REPORT'S OWN TURN: its
  # Stop hook has already counted it (the marker exists by then — measured with a
  # real claude and `hw executor-turn-end`) and herdr has not caught up. It is
  # "finishing", not "somebody is talking to it"; 2+ is still the latter.
  if [ "$post" = 1 ] && [ "$status" = working ] && [ "$self_close" = 0 ]; then post=""; fi

  # `done_state=undelivered` IS A REPORT THAT NEVER REACHED THE BRAINER, NOT ONE
  # THAT DID. done-invoker publishes it before it tries to deliver and rewrites
  # it to `delivered` only after the receiver admitted the envelope, so the
  # token alone says "the executor finished", never "the brainer heard" — and
  # the pane is the only copy of the transport failure. Closing it here filed a
  # STRANDED report as reported. The marker still wins: `_report_evidence`
  # returns `marker` first, so a delivered report never lands in this arm.
  # THE REASONS (and `stranded`) ARE COMPUTED IN bin/hw-actions, shared with the
  # cockpit's state writer, so its `done` button is valid by this predicate.
  local stranded=0 _rc=0
  reasons="$("$HW_BIN_DIR/hw-actions" done-reasons "$ev" "$status" "$self_close" "$post" "$seq" "$pane" "$(_run_done_marker "$rundir")")" || _rc=$?
  [ "$_rc" != 2 ] || stranded=1
  [ "$_rc" -le 2 ] || die "hw-actions done-reasons failed (exit $_rc) — nothing was closed"
  [ -n "$reasons" ] || return 0

  # REPORTED AND STILL WORKING, WITH NOBODY TALKING TO IT, IS A PANE FINISHING,
  # NOT A PANE IN USE. Measured 2026-09-28/29: right after `done-invoker` the
  # executor's pane still runs the turn that sent the report, herdr says
  # `working`, and `hw done` answered "nothing was closed. Establish who owns
  # that pane" — the wording for a stranger's session — to a brainer who only
  # needed to run the same command a few seconds later. The refusal stands (the
  # turn is real and the close would cut it); what changes is that it says which
  # of the two it is. Any other reason (no report, turns after the report)
  # keeps the ownership wording: those ARE somebody else's.
  local finishing=0
  if [ "$ev" != none ] && [ "$stranded" = 0 ] && [ "$status" = working ] && [ "$self_close" = 0 ] && [ -z "$post" ] && [ "$focus" != true ]; then
    finishing=1
  fi
  # THE ONLY REASON IS THE TURN THAT SENT THE REPORT: WAIT FOR IT, DO NOT HAND
  # THE RETRY BACK. Measured 2026-10-01, five times: the brainer re-ran `hw done`
  # by hand a few seconds later and it closed. Poll the same herdr answer; close
  # when the pane stops working with no turn after the report; refuse as before
  # when the cap passes (HW_DONE_TURN_WAIT, default 60s) or a turn ENDS after the
  # report (somebody is talking to it). --force never reaches here.
  if [ "$finishing" = 1 ]; then
    local cap="${HW_DONE_TURN_WAIT:-60}" poll="${HW_DONE_TURN_POLL:-1}" t0 ninfo
    case "$cap" in ''|*[!0-9]*) cap=60 ;; esac
    case "$poll" in ''|*[!0-9.]*|*.*.*) poll=1 ;; esac
    t0=$(date +%s)
    info "$pane reported and is finishing the turn that sent it — waiting up to ${cap}s for it to end before closing $proj:$task"
    while [ $(( $(date +%s) - t0 )) -lt "$cap" ]; do
      sleep "$poll"
      ninfo="$(herdr agent get "$pane" 2>/dev/null || true)"
      if ! printf '%s' "$ninfo" | jq -e '.result.agent' >/dev/null 2>&1; then
        # Same stance as an unanswered herdr at the top of the guard: say it, proceed.
        warn "herdr stopped answering for $pane while hw waited for its last turn — it may be gone; closing WITHOUT the rest of the check."
        return 0
      fi
      info="$ninfo"
      status="$(printf '%s' "$info" | jq -r '.result.agent.agent_status // empty' 2>/dev/null || true)"
      post="$(_done_post_report_turns "$rundir" "$seq" "$info")"
      # THE REPORT'S OWN TURN END IS COUNTED BY THE STOP HOOK AS `<seq> 1` (the
      # marker exists by then — measured with `hw executor-turn-end`), so 1 is
      # the turn we are waiting for, not somebody talking to the pane. A second
      # turn end (2+) is. A 1 with the pane working again is just waited out.
      if [ -n "$post" ] && [ "$post" -gt 1 ]; then
        reasons="  · $post turn(s) ended in $pane AFTER its report, and hw dispatched none of them. Somebody is talking to this pane."
        finishing=0
        break
      fi
      if [ "$status" != working ]; then
        info "$pane finished the turn that sent the report — closing $proj:$task"
        return 0
      fi
    done
  fi
  if [ "$finishing" = 1 ]; then
    warn "hw done REFUSED for $proj:$task — its pane has REPORTED and is STILL FINISHING its last turn:"
  else
    warn "hw done REFUSED for $proj:$task — this pane is in use:"
  fi
  printf '%s\n' "$reasons" | while IFS= read -r _r; do [ -z "$_r" ] || warn "$_r"; done
  _report_evidence_explain "$pane" "$ev" "$(_run_done_marker "$rundir")"
  if [ "$focus" = true ]; then
    warn "  · and it is FOCUSED: a human has this pane on screen right now. (Evidence only — an unfocused pane can be just as much in use, so this never decides on its own.)"
  fi
  # BEFORE THE CLOSE, NEVER AFTER. See the section comment: the recovery door is
  # worth nothing if it is announced once the session is already gone.
  revive_note="$(_done_revive_line "$proj" "$task" "$rundir")"
  [ -z "$revive_note" ] || info "$revive_note"
  if [ "$finishing" = 1 ]; then
    die "NOT CLOSED YET — nothing was closed: the report is in and hw sees no turn after it, so this is most likely the turn that sent it ending (hw already waited HW_DONE_TURN_WAIT seconds for it and it is still working, so somebody is talking to it — hw status). Run the same command again: hw done $proj $task   (--force would cut that turn)"
  fi
  if [ "$stranded" = 1 ]; then
    die "nothing was closed: the report is STRANDED. Redeliver it first (\`hw outbox flush $proj --to <brainer-pane>\`; do NOT re-run done-invoker if its exit was 5 — the report may already be there); close it deliberately, discarding the undelivered report, with: hw done $proj $task --force"
  fi
  die "nothing was closed. Establish who owns that pane — look at it — and then either let it finish, or close it deliberately with: hw done $proj $task --force"
}
# The recovery command, with the one precondition that decides whether it will
# work stated rather than implied. `cmd_revive` refuses a run whose receipt
# recorded no `session_resume` (58 of setup's 143 receipts are in that state),
# and a brainer reading `hw revive` as a promise in the moment of panic is
# exactly the wrong time to find that out.
_done_revive_line() {  # $1=project $2=task $3=rundir
  local rundir="$3"
  if [ -n "$rundir" ] && [ -r "$rundir/receipt.jsonl" ] \
     && grep -q '"key": *"session_resume"' "$rundir/receipt.jsonl" 2>/dev/null; then
    printf 'if this closes and you wanted it back: hw revive %s %s — run %s recorded a session_resume, so the conversation is addressable' \
      "$1" "$2" "$(basename "$rundir")"
    return 0
  fi
  printf 'NOTE: hw revive %s %s will NOT bring this back — %s recorded no session_resume, so the conversation is not addressable and closing is final for it' \
    "$1" "$2" "${rundir:+$(basename "$rundir")}"
}

# WHAT `hw done` SAYS ABOUT ITSELF: WHAT IT DID, AND WHERE THE SECONDS WENT.
# Measured 2026-09-28/29: the close itself is milliseconds (herdr answers in
# ~10ms, the brain-leak scan ~0.15s); the seconds are `_verify_at_close`, which
# runs the brief's own ## Verification command (unless a clean tree already
# has a green verdict or never left its dispatch base: see _verify_shortcut) — the fast gate ~55s, a full
# suite up to 900s, plus up to 900s waiting for the lane's suite lock — BEFORE
# anything closes. A caller whose tool gives up at 120s could not tell that from
# a hang, and could not tell a close that happened from one that did not. So
# `_done_outcome` names one of three facts the moment the close is decided, and
# `cmd_done` ends with the clock split into the step that took it and the rest.
_DONE_VERIFY_SECONDS=0
_done_outcome() {  # $1=project $2=task $3=what herdr was asked to close, or "" when nothing was there
  case "${_HERDR_CLOSE_RESULT:-}" in
    closed) ok "CLOSED: $3 — this call closed it ($1:$2)" ;;
    absent) info "NOTHING TO CLOSE: $3 was already gone when hw asked — this call closed nothing, and that is not a failure ($1:$2)" ;;
    failed) warn "NOT CLOSED — FAILED: $3 may still be RUNNING; this call did not close it and exits non-zero ($1:$2). Look before anything else: hw status" ;;
    *)      info "NOTHING TO CLOSE: no open tab or space labelled $1:$2 — closed earlier or never opened, so this call closed nothing, and that is not a failure" ;;
  esac
}
# NO WRAPPER, ON PURPOSE. A function called on the left of `||` (or in an `if`)
# runs with errexit OFF for its whole body and everything it calls — a wrapper
# `_body || _rc=$?` silently turned every unguarded failure in here (a `local x=$(die…)`
# in the lease path, a failed `_lease_write`) into "carry on", one of which closes
# the pane. Found by both Judgment Day judges. So the clock is read at the top
# of cmd_done and printed by `_done_took` at each return.
_done_took() {
  local total=$(( $(date +%s) - ${_DONE_T0:-$(date +%s)} ))
  if [ "$total" -ge 5 ]; then
    info "hw done took ${total}s: verification ${_DONE_VERIFY_SECONDS}s, everything else $(( total - _DONE_VERIFY_SECONDS ))s"
  fi
}
# `closed-by-hand` beside the task's done marker: when, by which flag, and
# whether a report existed. Written once — a repeated `hw done` (sweeps do) must
# not move the first close, and a marker on a task that DID report must say so.
_done_mark_closed_by_hand() {  # $1 = run directory
  local rundir="${1%/}" mark by=none reported=no
  [ -n "$rundir" ] && [ -d "$rundir" ] || return 0
  mark="$(dirname "$(_run_done_marker "$rundir")")/closed-by-hand"
  [ ! -e "$mark" ] || return 0
  [ "${DONE_BLOCKED:-0}" = 1 ] && by=--blocked
  [ "${DONE_FORCE:-0}" = 1 ] && by=--force
  [ -f "$(_run_done_marker "$rundir")" ] && reported=yes
  mkdir -p "$(dirname "$mark")" 2>/dev/null || return 0
  printf 'at=%s\nby=%s\nreported=%s\n' "$(date +%s)" "$by" "$reported" > "$mark" 2>/dev/null || true  # MUTATION-ANCHOR: 700-M01
}

cmd_done() {
  local proj="$1" task="$2" main wt branch close_failed=0
  _DONE_T0="$(date +%s)"; _DONE_VERIFY_SECONDS=0
  lane_known "$proj" || die "unknown project: $proj (expected: $(lane_names_bar))"
  main="$(_lane_main_checkout "$proj")"
  wt="$(_lane_wt_dir "$proj" "$task")"
  [ -n "$wt" ] || wt="$WORK/$proj/$task"
  branch="$(_lane_branch "$proj" "$task")"  # MUTATION-ANCHOR: 140-M06

  # Capture first: close verification can run for 900 seconds, and the live pane
  # is the only source for an old report that went to the wrong brainer.
  local done_rundir
  done_rundir="$(_task_rundirs "$proj" "$task" | head -1)"
  [ -z "$done_rundir" ] || _snapshot_done_report "$done_rundir"
  # BEFORE ANYTHING IS CLOSED OR CLEARED — before the reopened marker is
  # removed, before the 900s close verification, and long before a tab goes. A
  # refusal arriving after any of those has already spent what it exists to save.
  #
  # NOT "before anything is touched", which is what this said until the scoped
  # re-judgment called it: `_snapshot_done_report` above already ran, and it
  # appends to the run's receipt. That snapshot is idempotent and informational
  # and it deliberately stays first — the live pane is the only source for an
  # old report, and a 900s verification would lose it — but the claim was
  # wider than the code and is narrowed rather than left standing.
  _done_in_use_guard "$proj" "$task" "$done_rundir"
  # A run `hw revive` reopened leaves the board again when it is closed, whether
  # its executor reported a second time or a human closed it: the marker is a
  # statement that a live pane is expected, and after this there is none.
  if [ -n "$done_rundir" ] && [ -f "$(_run_reopened_marker "$done_rundir")" ]; then
    rm -f "$(_run_reopened_marker "$done_rundir")" 2>/dev/null || true
    info "run was reopened by hw revive; closing it clears that marker"
  fi

  # BEFORE ANYTHING IS CLOSED. The verification runs in the task's directory and
  # every close path below reaches an end — including `--keep-preview`, which
  # returns early. Running it first means the measurement exists whichever way
  # this task leaves.
  local _v0; _v0="$(date +%s)"
  _verify_at_close "$(_task_rundirs "$proj" "$task" | head -1)" "$wt" "$proj" "$task"
  _DONE_VERIFY_SECONDS=$(( $(date +%s) - _v0 ))
  # And before anything closes: a product task does not leave with a commit that
  # carries a brain path. The push guard sees the same scan, but a branch the
  # operator pushes or merges after this never passes through it.
  PROJ="$proj" _brain_leak_at_close "$proj" "$wt" "$branch"
  # A blocked executor kept waiting for a ruling stops waiting when it is closed:
  # the marker moves aside (`blocked-closed`) so sweep, reap and status stop
  # treating a closed run as one that can still be resumed. AFTER THE
  # VERIFICATION AND THE LEAK SCAN, not before: the scan can refuse the close
  # and a verification of up to 900s can be interrupted, and a close stopped
  # there had already moved the marker — a live pane nobody could resume and
  # nothing would expire.
  if [ -n "$done_rundir" ] && [ -f "$(_run_blocked_marker "$done_rundir")" ]; then
    mv -f "$(_run_blocked_marker "$done_rundir")" "$(dirname "$(_run_blocked_marker "$done_rundir")")/blocked-closed" 2>/dev/null || true
    info "it had reported BLOCKED and was waiting for a ruling; closing it ends the wait"
  fi

  # A HAND CLOSE LEAVES ITS OWN MARK. Without it `hw status` read a task closed
  # here and a task whose executor died as the same `finished, unreported`
  # (an audit found 19 of 24 such rows were hand closes). Written after
  # every refusal above, so a close that did not happen leaves nothing.
  _done_mark_closed_by_hand "$done_rundir"
  # TAB FIRST, AND A TAB RECORD CAN NEVER CLOSE A WORKSPACE. hw has closed other
  # people's agents before — a cleanup that worked by exclusion once
  # killed a live brainer's own executors — and the answer then was
  # that nothing is closed without an ownership record naming it. Tabs inherit
  # that rule exactly: `.hw/<run>/tab` licenses a `tab close` and nothing else,
  # `.hw/<run>/workspace` licenses a `workspace close` and nothing else. Runs that
  # predate tab mode have only the second, so this is compatible by construction.
  local rundir_tab="" tab="" ws=""
  rundir_tab="$(_latest_run_record "$proj" "$task" tab)"
  if [ -n "$rundir_tab" ]; then
    tab="$rundir_tab"
  else
    tab="$(_tab_id_by_label "$proj:$task")"
  fi
  # UNDER A LEASE, THE AGENT GOES AND THE PREVIEW STAYS. Closing the tab takes
  # the dev pane with it, which is exactly the coupling the lease exists to
  # break: the executor is finished, the preview is not.
  if [ "${KEEP_PREVIEW:-0}" = 1 ]; then
    local lease_run agent_pane secs
    lease_run="$(_task_rundirs "$proj" "$task" | head -1)"
    [ -n "$lease_run" ] || die "no hw run directory for $proj:$task, so there is nothing to write a lease against."
    secs="$(_lease_seconds "$KEEP_FOR")"
    agent_pane="$(jq -r 'select(.key=="pane") | .value' "$lease_run/receipt.jsonl" 2>/dev/null | tail -1)"
    if [ -n "$agent_pane" ]; then
      _close_herdr_object pane "$agent_pane" \
        "agent pane $agent_pane closed — the executor is released" \
        "agent pane $agent_pane is already gone" || return 1
    else
      warn "no pane receipt for this run, so the agent pane could not be identified; nothing was closed"
    fi
    _lease_write "$lease_run" "$secs" "$KEEP_WHY"
    ok "preview LEASED until $(_lease_field "$lease_run" until_h) — tab $tab, database and ports kept"
    info "why: $KEEP_WHY"
    [ -z "${HW_PORT_WEB:-}" ] || info "preview: http://127.0.0.1:$HW_PORT_WEB"
    info "release it with: hw preview release $proj $task   (then hw done $proj $task --drop-db)"
    info "hw reap will refuse this worktree while the lease holds, and hw status shows it expiring"
    _done_took
    return 0
  fi
  # THE EXIT IS ANNOUNCED BEFORE THE FIRE, NOT AFTER. On 2026-09-16 a close
  # that turned out to be a mistake was recoverable the whole time and nobody
  # in the room knew the command. It costs one line, and the only moment it is
  # useful is the moment before.
  _done_revive_pre="$(_done_revive_line "$proj" "$task" "$done_rundir")"
  [ -z "$_done_revive_pre" ] || info "$_done_revive_pre"  # MUTATION-ANCHOR: 150-M03
  _HERDR_CLOSE_RESULT=""
  if [ -n "$tab" ]; then
    _close_herdr_object tab "$tab" \
      "tab $tab closed ($proj:$task)" \
      "tab $tab is already gone" || close_failed=1
    _done_outcome "$proj" "$task" "tab $tab"
  else
    ws="$(_ws_id_by_label "$proj:$task")"
    if [ -n "$ws" ]; then
      _close_herdr_object workspace "$ws" \
        "space $ws closed" \
        "space $ws is already gone" || close_failed=1
      _done_outcome "$proj" "$task" "space $ws"
    else
      info "no open tab or space labelled $proj:$task"
      _done_outcome "$proj" "$task" ""
    fi
  fi

  # The pane is gone: tell the cockpit now, not at the next heartbeat.
  [ -z "$done_rundir" ] || "$HW_BIN_DIR/cockpit-state" --kick --rundir "$done_rundir" >/dev/null 2>&1 || true

  # A repo-less task has no branch to merge and no ports to free. What it does
  # have is a directory of artifacts, which is the whole output — reported,
  # never removed.
  if _is_repoless "$proj"; then
    if [ -d "$wt" ]; then
      local n sz
      # Exclude .hw: those are hw's own control files, not the task's output.
      # Reporting them as artifacts made an empty task look like it produced two.
      n="$(find "$wt" -type f -not -path '*/.hw/*' 2>/dev/null | wc -l | tr -d ' ' || true)"; : "${n:=0}"
      sz="$(du -sh "$wt" 2>/dev/null | awk '{print $1}')"
      if [ "$n" = 0 ]; then
        info "no artifacts in $wt (correct when the output was a report)"
      else
        warn "artifacts KEPT in $wt ($n files, $sz) — hw never deletes them"
      fi
    else
      info "no work directory at $wt"
    fi
    printf "\n  ${C_B}Nothing to merge:${C_0} %s has no repository.\n  Move anything worth keeping out of %s yourself.\n\n" "$proj" "$wt"
    _done_took
    return "$close_failed"
  fi

  # Ports are freed by the dev servers dying with their panes; nothing to unbind.
  info "ports released with the panes"

  # The database outlives the space on purpose: it holds whatever state the task
  # produced. hw reports it and, only when asked, drops it — dropping by default
  # would silently destroy work whose only copy is in that database. Which lane
  # has one is the table's `db.provisioned`; every other lane gets the same
  # one line, spelled with its own name.
  local done_db_live=""
  if [ -n "$(lane_get "$proj" db_provisioned)" ]; then
    local db; db="$(basename "$wt" | tr '\-/' '__')"
    if psql -U "$(id -un)" -d postgres -Atc "select 1 from pg_database where datname = '$db'" 2>/dev/null | grep -q 1; then
      local size; size="$(psql -U "$(id -un)" -d postgres -Atc "select pg_size_pretty(pg_database_size('$db'))" 2>/dev/null)"
      if [ "$DROP_DB" = 1 ]; then
        if psql -U "$(id -un)" -d postgres -q -c "DROP DATABASE \"$db\"" 2>/dev/null; then
          ok "database $db dropped (was $size)"
        else
          warn "could not drop $db — open connections? try again once the panes are gone"
        fi
      else
        # Decided below, with the worktree: a disposable worktree takes its
        # database with it (pg_dump into the archive first, see _reap_db).
        done_db_live="$db ($size)"
      fi
    else
      info "no database named $db"
    fi
  else
    info "$proj has no per-worktree database to release"
  fi

  BASE_BRANCH="$(_lane_base "$proj")"; : "${BASE_BRANCH:=main}"

  # THE MERGE IS PRINTED, never run: merging is the brainer's call. Removing a
  # `safe` worktree is not printed — it is done, below (the operator, 2026-10-04).
  #
  # AND THE MERGE IS NOT PRINTED BLINDLY. This block used to end with
  # `git worktree remove $wt` on every done, whatever the worktree held. For
  # a product lane's coords-reconcile that line destroys `_coords-export/` —
  # 588K of raw.json and four CSVs, the 754 rows that went to Mike — because the
  # directory is git-ignored and `git status` therefore says clean. A command
  # printed under the heading "clean up" is a command that gets pasted.
  # NOTHING TO MERGE IF NOTHING WAS BRANCHED. A `--worktree none` task runs in a
  # repo-less work directory and never creates `task/<name>`, and printing a
  # merge for a branch that does not exist is the same defect as printing
  # `git worktree remove` for a worktree that holds unreproducible data: a
  # command under a confident heading gets pasted. Asked of git, not inferred
  # from the flags — the run that created this task may not be the one closing it.
  # A BRANCH EXISTING IS NOT WORK BEING ON IT. The check above asked git whether
  # the ref exists and stopped there — so on 2026-08-25 `hw done <lane>
  # classic-stage-impl` printed a full merge recipe for `task/classic-stage-impl`
  # while that branch was at develop with ZERO commits and the entire day of work
  # sat as 39 uncommitted files in the worktree. Paste that recipe and it merges
  # nothing, cleanly, and you walk away believing the work landed. That is the
  # same defect the existence check was added to fix, one level down: a confident
  # heading over a command whose signal does not support it.
  # COUNT AGAINST origin/$BASE_BRANCH WHEN IT EXISTS, AND NAME WHICHEVER REF WAS
  # USED. This used to run "$BASE_BRANCH..$branch" against the LOCAL checkout
  # only — on a READ-ONLY task whose local main was six days stale, that
  # printed "11 commits — merge it" while origin/main..task/<x> was empty.
  # "11 commits" with no subject is an unverifiable claim; naming the ref is
  # the fix, not a flag to pick one.
  local ahead=0 ahead_ref="$BASE_BRANCH" ahead_note=""
  if git -C "$main" show-ref --verify --quiet "refs/heads/$branch" 2>/dev/null; then
    if git -C "$main" show-ref --verify --quiet "refs/remotes/origin/$BASE_BRANCH" 2>/dev/null; then
      ahead_ref="origin/$BASE_BRANCH"
      local fetch_head fetch_epoch
      # --git-path prints a path RELATIVE TO $main, not to this process's cwd
      # or an absolute path — unprefixed, `[ -f "$fetch_head" ]` checked the
      # wrong directory and silently found nothing.
      fetch_head="$(git -C "$main" rev-parse --git-path FETCH_HEAD 2>/dev/null || true)"
      [ -z "$fetch_head" ] || fetch_head="$main/$fetch_head"
      if [ -n "$fetch_head" ] && [ -f "$fetch_head" ]; then
        fetch_epoch="$(date -r "$fetch_head" +%s 2>/dev/null || echo 0)"
        if [ "$fetch_epoch" -gt 0 ]; then
          ahead_note=" (origin fetched $(( ($(date +%s) - fetch_epoch) / 3600 ))h ago)"
        fi
      fi
    else
      ahead_note=" (no origin/$BASE_BRANCH — counted against the local checkout, which may be behind origin)"
    fi
    ahead="$(git -C "$main" rev-list --count "$ahead_ref..$branch" 2>/dev/null || echo 0)"
    case "$ahead" in ''|*[!0-9]*) ahead=0 ;; esac
  fi
  local dirty=0
  # `grep -c` prints its count AND exits 1 when that count is zero, so
  # `|| echo 0` appended a second "0" and a CLEAN worktree made the test below
  # print `[: integer expression expected` — reproduced 2026-09-01 against an
  # empty repo with HEAD's line: dirty=$'0\n0'. The count is already there; only
  # the exit needs swallowing, and anything that is still not a number is 0.
  # THE EXIT CODE, NOT JUST THE COUNT. `git status | grep -c '^'` prints 0 for a
  # FAILED status exactly as for a clean tree, and `$dirty` decides whether the
  # "THE WORK IS NOT ON THE BRANCH" warning below is printed at all. A zero
  # nobody measured suppresses the warning that exists to stop work being lost.
  local dirty_rc=0
  if [ -d "$wt" ]; then
    dirty="$(git -C "$wt" status --porcelain 2>/dev/null)" || dirty_rc=$?
    if [ "$dirty_rc" -ne 0 ]; then
      dirty=unknown
    else
      dirty="$(printf '%s' "$dirty" | grep -c '^' || true)"
    fi
  fi
  case "$dirty" in unknown) ;; ''|*[!0-9]*) dirty=0 ;; esac
  # THE RECIPE NAMES ONLY REMOTES THE LANE ACTUALLY HAS.
  #
  # MEASURED 2026-09-16, on the first setup close that reached this block: brain
  # has NO `origin` — its remotes are `backup` (a local bare repo) and `github`
  # (the sanitized export) — and the recipe printed `git fetch origin` and
  # `git pull` under the heading "Merge it:". Pasted, that is
  # `fatal: 'origin' does not appear to be a git repository`, on the one command
  # a brainer runs to land a task's work. The block was unreachable for setup
  # until the lane left REPOLESS in this same change, so it is a defect this
  # change introduced rather than one it exposed.
  #
  # THE FACT IS ALREADY COMPUTED ten lines up: `$ahead_ref` is
  # `origin/$BASE_BRANCH` when that remote-tracking ref exists and the bare
  # local branch when it does not. Asking the same question a second way is how
  # two answers start to disagree.
  local sync_lines=""
  case "$ahead_ref" in
    origin/*) sync_lines="  git fetch origin"$'\n'"  git checkout $BASE_BRANCH"$'\n'"  git pull" ;;
    *)        sync_lines="  git checkout $BASE_BRANCH"$'\n'"  ${C_DIM}# no origin in $main — nothing to fetch or pull${C_0}" ;;
  esac
  if git -C "$main" show-ref --verify --quiet "refs/heads/$branch" 2>/dev/null && [ "$ahead" -gt 0 ]; then
    cat <<EOF

${C_B}Merge it:${C_0}

  cd $main
$sync_lines
  git merge --no-ff $branch          # $ahead commit(s) against $ahead_ref$ahead_note — or open a PR and squash-merge it

EOF
  elif git -C "$main" show-ref --verify --quiet "refs/heads/$branch" 2>/dev/null; then
    printf '\n'
    warn "NOTHING TO MERGE: $branch exists but is level with $ahead_ref — 0 commits.$ahead_note"
    if [ "$dirty" = unknown ]; then
      warn "and whether the worktree holds uncommitted changes could NOT be read (git status failed). That is not the same as none — look before you remove anything."
    elif [ "$dirty" -gt 0 ]; then
      warn "and the worktree holds $dirty uncommitted change(s). THE WORK IS NOT ON THE BRANCH."
      warn "It exists in exactly one place: $wt"
      warn "Commit it there before anything else, or it is one \`git worktree remove\` from gone:"
      printf '\n  git -C %s add -A && git -C %s commit -m "<what this is>"\n\n' "$wt" "$wt"
      warn "\`hw reap\` already refuses this worktree while it is dirty, so nothing is lost right now."
    else
      info "the worktree is clean too, so this task produced no committed change"
    fi
  else
    printf '\n'
    info "no branch $branch in $main — nothing to merge"
    info "that is expected for a --worktree none task: it ran in a work directory, not a worktree"
  fi
  # ASK GIT WHERE IT IS, don't assume the canonical path. coords-reconcile still
  # lives at the pre-2026-08-22 root outside the repo, so the assumed path did not
  # exist and this printed "already gone" about a worktree holding 588K of export.
  # A wrong reassurance is worse than a wrong command.
  if [ ! -d "$wt" ] && [ -n "$branch" ]; then
    local found
    found="$(git -C "$main" worktree list --porcelain 2>/dev/null \
      | awk -v b="refs/heads/$branch" '/^worktree /{w=$2} /^branch /{if ($2==b) print w}' | head -1)"
    if [ -n "$found" ] && [ -d "$found" ]; then
      wt="$found"
      warn "its worktree is not at the canonical root: $wt"
    fi
  fi
  if [ ! -d "$wt" ]; then
    printf '  %sno worktree for %s — already removed, or it never had one%s\n\n' "$C_DIM" "$branch" "$C_0"
    [ -z "$done_db_live" ] || warn "database $done_db_live is STILL LIVE — drop it with: hw done $proj $task --drop-db"
    _done_took
    return "$close_failed"
  fi
  # THE BRANCH THE WORKTREE IS ON, not the one its task name derives. Now that
  # this path removes, a worktree left detached (or switched) with commits of
  # its own would read `safe` against a merged `task/<name>` and lose them to
  # `git worktree remove` — the gate `hw reap` already has by reading HEAD.
  # Only when git can read the tree: the lost-.git case still needs the
  # derived name (see _wt_disposition).
  local wt_branch="$branch"
  if git -C "$wt" rev-parse --git-dir >/dev/null 2>&1; then
    wt_branch="$(git -C "$wt" symbolic-ref --short HEAD 2>/dev/null || echo detached)"
  fi
  # Its executor is gone (the tab closed above): the lock hw took at launch
  # goes with it. Only hw's own — a lock somebody else took stays, and reads
  # `locked` below.
  local _lr
  if _lr="$(_wt_lock_reason "$main" "$wt")" && [ "${_lr#hw }" != "$_lr" ]; then
    git -C "$main" worktree unlock "$wt" >/dev/null 2>&1 && info "unlocked $(basename "$wt") — its executor is closed"
  fi
  _wt_disposition "$main" "$wt" "$wt_branch" "${BASE_BRANCH:-main}"
  if [ "$WT_VERDICT" = safe ] || [ "$WT_VERDICT" = archivable ]; then
    # `hw done` REAPS ITS OWN TASK. It used to print the two commands below and
    # run neither, and "hw reap after every hw done" was a rule nobody ran:
    # measured 2026-10-01, 16 worktrees `safe` for weeks in one lane and 191
    # more held only by their outputs. Same body as `hw reap --apply`, same
    # gates, re-asked after the archive.
    #
    # `safe` answers "does git need this directory". It does not answer "does a
    # recorded session need it": `hw revive` repeats the cwd from the receipt
    # and refuses when it is gone, so removing it is also the act that makes
    # the session unrevivable. Said before it happens.
    if [ -n "$done_rundir" ] && [ -r "$done_rundir/receipt.jsonl" ] \
       && [ -n "$(jq -r 'select(.key=="session_resume") | .value' "$done_rundir/receipt.jsonl" 2>/dev/null | tail -1)" ]; then
      info "removing the worktree also ends \`hw revive $proj $task\` for this session"
    fi
    printf '%sReaping it — %s%s\n' "$C_B" "$C_0" "$WT_WHY"
    if _reap_worktree "$proj" "$main" "$wt" "$wt_branch" "${BASE_BRANCH:-main}"; then
      [ -z "$REAP_ARCHIVE_DIR" ] || info "its outputs are in $REAP_ARCHIVE_DIR"
      case "$ahead_ref" in
        origin/*) info "a pushed branch stays on origin: git -C $main push origin --delete $branch" ;;
      esac
      done_db_live=""
    fi
  else
    printf '  %s%s DO NOT REMOVE THIS WORKTREE YET%s\n' "$C_WARN" "!" "$C_0"
    printf '      %s%s%s\n' "$C_DIM" "$WT_WHY" "$C_0"
    local shed
    if _wt_sheddable && shed="$(_wt_shed "$proj" "$wt")"; then
      printf '      %sKept, and shed what its install rebuilds: %s%s\n' "$C_DIM" "$shed" "$C_0"
    fi
    case "$WT_VERDICT" in
      irreplaceable)
        printf '      %sgit status reports this worktree clean and it is not — those paths are%s\n' "$C_DIM" "$C_0"
        printf '      %signored, so `git worktree remove` would delete them with no warning.%s\n' "$C_DIM" "$C_0"
        printf '      %sMove what you need out first, then re-run `hw done` or `hw reap`.%s\n' "$C_DIM" "$C_0" ;;
      unmerged)
        printf '      %sMerge it first (above), then `hw reap %s`.%s\n' "$C_DIM" "$proj" "$C_0" ;;
      dirty)
        printf '      %sCommit or discard those changes, then `hw reap %s`.%s\n' "$C_DIM" "$proj" "$C_0" ;;
      held)
        printf '      %sThat pane is still sitting in it. Nothing to do until it moves.%s\n' "$C_DIM" "$C_0" ;;
      not-a-worktree)
        printf '      %sNothing to remove and nothing to merge: no branch was ever created for it.%s\n' "$C_DIM" "$C_0"
        printf '      %sIts contents are the task output. Move what is worth keeping, then delete it by hand.%s\n' "$C_DIM" "$C_0" ;;
      undetermined)
        printf '      %sThis is NOT a verdict that the worktree is needed — it is the absence of one.%s\n' "$C_DIM" "$C_0"
        printf '      %sFix what could not be read, then re-run. Do not remove it on the strength of%s\n' "$C_DIM" "$C_0"
        printf '      %sa check that did not run: that is how 588K went once.%s\n' "$C_DIM" "$C_0" ;;
      locked)
        printf '      %sSomebody holds it; unlock it once nobody does, then `hw reap %s`.%s\n' "$C_DIM" "$proj" "$C_0" ;;
      leased)
        printf '      %sSomebody is still going to look at this preview. `hw preview` shows who and until when;%s\n' "$C_DIM" "$C_0"
        printf '      %s`hw preview release %s <task>` ends it early.%s\n' "$C_DIM" "$proj" "$C_0" ;;
    esac
    printf '      %sSurvey every worktree the same way: hw reap %s%s\n\n' "$C_DIM" "$proj" "$C_0"
  fi
  [ -z "$done_db_live" ] || warn "database $done_db_live is STILL LIVE — drop it with: hw done $proj $task --drop-db"
  _done_took
  return "$close_failed"
}
