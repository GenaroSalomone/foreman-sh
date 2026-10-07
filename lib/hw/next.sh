# lib/hw/next.sh — `hw next`.
#
# Sourced ONCE by bin/hw (from $HW_BIN_DIR/../lib/hw/next.sh), after status's
# module. It is a library: no shebang, nothing runs on source except function
# definitions. Moved VERBATIM from bin/hw (parent fcf6d56f): _next_authorization,
# _next_autoclosed_run_for_pane, _next_missing_pane_message, _next_wait_from_run,
# _next_busy_message, _next_finished_stale_stuck, _next_opencode_session_from_receipt,
# _next_settled_message, _next_message, _next_gentle_read and cmd_next.
# Stayed in bin/hw, each with a caller outside `hw next`: _next_dispatch_model and
# _next_dispatch_effort (the reuse route), _next_run_dir (dispatch, revive, status
# readers), _next_load_invoker_lib (dispatch, unstick), _run_env_value (many).
# THE SAME BLOCK FOR A RE-TASK. `hw next` delivers a NEW brief and used to carry
# the epistemic safeguard and the preamble but never the authorization, so a task
# whose brief says `authorizes:` got it on the first dispatch only. Run in a
# subshell, the way the launch path does it: the same validation, the same
# requested_by requirement and the same handle check (_check_authorizes), then
# the same block (_brief_authorization). A refusal leaves nothing delivered.
_next_authorization() {  # $1 = the new brief's path; prints the block, or nothing
  [ -f "${1:-}" ] || return 0
  (
    # CHECK_HANDLES is set by the launch path's flag parsing, which `hw next`
    # never reaches: unset here it was an unbound variable under set -u.
    BRIEF="$1"; BRIEF_DECLS_READ=0; BRIEF_DECLS=""; BRIEF_DECLARED=none; AUTHORIZES=""
    CHECK_HANDLES="${CHECK_HANDLES:-1}"
    _load_brief_decls
    # A citation already in hand (--requested-by on the launch that reached
    # here by the reuse route) outranks the brief's own, as it does at launch.
    [ -z "${REQUESTED_BY:-}" ] && REQUESTED_BY="$(printf '%s\n' "$BRIEF_DECLS" | awk -F '\t' '$1 == "requested_by" { print $2; exit }')"  # MUTATION-ANCHOR: 703-M03
    _check_authorizes
    _brief_authorization
  )
}

# A missing pane gets the chaining diagnosis only when disk proves the exact
# launch decision. This intentionally excludes old runs: absence of a key is
# not proof that the brainer chose automatic close.
#
# The default was a fourth hand-typed copy of "$WORK plus the repo-backed lane
# roots" until 2026-09-15 — missing two other lanes, same class of gap as
# INVOKER_ENV_ROOTS (bin/project-spaces.sh's all_lane_worktree_roots doc
# comment has the incident). Derives from that same function now, falling
# back to the old literal if it is unavailable for any reason.
# HW_NEXT_EVIDENCE_ROOTS still wins outright — setup/tests/12-hw-next.sh pins
# a missing root through it on purpose.
_next_autoclosed_run_for_pane() {
  local pane="$1" roots _default_roots _derived
  _default_roots="$WORK"
  if command -v all_lane_worktree_roots >/dev/null 2>&1; then
    _derived="$(all_lane_worktree_roots 2>/dev/null || true)"
    [ -n "$_derived" ] && _default_roots="$WORK
$_derived"
  fi
  roots="${HW_NEXT_EVIDENCE_ROOTS:-$_default_roots}"
  HW_NEXT_PANE="$pane" HW_NEXT_ROOTS="$roots" HW_RUNENV="$HW_BIN_DIR/runenv" python3 - <<'PYEOF' 2>/dev/null || true
import importlib.machinery, importlib.util, json, os, sys

sys.dont_write_bytecode = True  # never a __pycache__ beside bin/runenv
pane = os.environ["HW_NEXT_PANE"]
roots = [r for r in os.environ.get("HW_NEXT_ROOTS", "").split("\n") if r]

# bin/runenv is the one reader of a run's env file; loaded, not re-implemented.
_loader = importlib.machinery.SourceFileLoader("runenv", os.environ["HW_RUNENV"])
runenv = importlib.util.module_from_spec(importlib.util.spec_from_loader("runenv", _loader))
_loader.exec_module(runenv)

def env_value(path, key):
    try:
        values = runenv.read(path, lenient=True, bare=True)
    except runenv.Unusable:
        return None
    return None if values is None else values.get(key)

def receipt_pane(path):
    found = None
    try:
        lines = open(path, encoding="utf-8")
    except OSError:
        return None
    with lines:
        for line in lines:
            try:
                rec = json.loads(line)
            except ValueError:
                continue
            if rec.get("key") == "pane" and rec.get("value"):
                found = rec["value"]
    return found

for root in roots:
    if not os.path.isdir(root):
        continue
    for directory, dirs, files in os.walk(root):
        if "receipt.jsonl" not in files or os.path.basename(os.path.dirname(directory)) != ".hw":
            continue
        if receipt_pane(os.path.join(directory, "receipt.jsonl")) != pane:
            continue
        if env_value(os.path.join(directory, "env"), "HW_CHAINING_ENABLED") != "0":
            continue
        if not any(os.path.isfile(os.path.join(directory, k)) for k in ("tab", "workspace")):
            continue
        seq = "1"
        try:
            raw = open(os.path.join(directory, "task"), encoding="utf-8").read()
            digits = "".join(c for c in raw if c.isdigit())
            if digits and digits != "0":
                seq = digits
        except OSError:
            pass
        marker = (os.path.join(directory, "done") if seq == "1" else
                  os.path.join(directory, "t" + seq, "done"))
        if not os.path.isfile(marker):
            continue
        project = env_value(os.path.join(directory, "env"), "HW_PROJECT") or "unknown-project"
        task = env_value(os.path.join(directory, "env"), "HW_TASK") or "unknown-task"
        print("%s:%s (run %s, task %s)" % (project, task, os.path.basename(directory), seq))
        raise SystemExit(0)
PYEOF
}

_next_missing_pane_message() {
  local pane="$1" panes evidence
  panes="$(herdr pane list 2>/dev/null || true)"
  # If Herdr itself is unavailable or malformed, the durable record cannot prove
  # that this pane is absent now. Keep the generic two-cause diagnosis.
  printf '%s' "$panes" | jq -e '.result.panes | type == "array"' >/dev/null 2>&1 \
    || { printf 'herdr does not know pane %s (or herdr is unreachable on $HERDR_SOCKET_PATH)' "$pane"; return 0; }
  if printf '%s' "$panes" | jq -e --arg p "$pane" 'any(.result.panes[]?; .pane_id == $p)' >/dev/null 2>&1; then
    printf 'herdr pane %s exists but has no agent record, so hw cannot re-task it' "$pane"
    return 0
  fi
  evidence="$(_next_autoclosed_run_for_pane "$pane")"
  if [ -n "$evidence" ]; then
    printf 'pane %s belonged to completed %s and was launched with automatic close. Chaining is a launch-time decision: launch with `hw ... --keep-pane` when this executor must survive its report for `hw next`.' "$pane" "$evidence"
  else
    printf 'herdr does not know pane %s (or herdr is unreachable on $HERDR_SOCKET_PATH)' "$pane"
  fi
}

# _next_wait_from_run <rundir>
#
# A dispatch's environment is the durable bridge between the brainer that
# launched an executor and the brainer process that later calls `hw next`.
# Never source it: the format is deliberately narrow. bin/runenv reads it, and
# this keeps only the decimal wait value we own.
_next_wait_from_run() {
  local value
  value="$(_run_env_value "$1" HW_NEXT_WAIT_MS)" || return 1
  case "$value" in ''|*[!0-9]*) return 1 ;; esac
  printf '%s' "$value"
}

# _next_busy_message <pane> <wait_ms> <info_json>
#
# `hw next` gates on idle,done before it will move the counter. Rejected
# because a pane is `blocked` on its OWN unanswered ask (ask_state=delivered)
# is not the same rejection as a pane genuinely mid-turn: no wait budget ever
# reaches idle,done from there, because nothing is going to move until the ask
# is answered. Advising a retry with a bigger HW_NEXT_WAIT_MS in that case is
# advice that cannot work — measured 2026-08-25, w41:p37: 90s then
# 540s spent on a pane parked on its own ask #3/3 the whole time.
#
# So this names the open ask instead of the budget. A `blocked` pane with no
# ask_state=delivered (a tool-permission prompt, say) still gets today's
# message — this is not a general replacement for it.
#
# ASK TOKENS OUTLIVE THE TASK THAT OPENED THEM. `ask_state=delivered` is
# published by the pane and STAYS published after the task closes — a task
# that ends by reporting `done_status=blocked` (a DIFFERENT blocker than the
# ask) keeps its old `ask_state=delivered` sitting right there. Measured
# 2026-08-25, twice, against w41:p37: the executor had already closed
# with `done_status=blocked` and a distinct blocker, and both a brainer and
# this tool read the stale ask token as live and pointed at answering a
# question that no longer existed. So an ask is only live if the task has NOT
# reported done — check `done_status`/`done_state` before naming the ask, and
# say plainly that the task closed when it has.
_next_busy_message() {
  local pane="$1" wait_ms="$2" info="$3"
  local st ask_state ask_seq ask_at ask_text done_status done_state
  local blocked_reason blocked_scope
  st="$(printf '%s' "$info" | jq -r '.result.agent.agent_status // empty' 2>/dev/null || true)"
  blocked_reason="$(printf '%s' "$info" | jq -r '.result.agent.tokens.blocked_reason // empty' 2>/dev/null || true)"
  blocked_scope="$(printf '%s' "$info" | jq -r '.result.agent.tokens.blocked_scope // empty' 2>/dev/null || true)"
  ask_state="$(printf '%s' "$info" | jq -r '.result.agent.tokens.ask_state // empty' 2>/dev/null || true)"
  done_status="$(printf '%s' "$info" | jq -r '.result.agent.tokens.done_status // empty' 2>/dev/null || true)"
  done_state="$(printf '%s' "$info" | jq -r '.result.agent.tokens.done_state // empty' 2>/dev/null || true)"
  if [ "$st" = blocked ] && [ "$ask_state" = delivered ] && [ "$done_status" != done ] && [ "$done_status" != blocked ]; then
    ask_seq="$(printf '%s' "$info" | jq -r '.result.agent.tokens.ask_seq // "?"' 2>/dev/null || true)"
    ask_at="$(printf '%s' "$info" | jq -r '.result.agent.tokens.ask_at // "an unknown time"' 2>/dev/null || true)"
    ask_text="$(printf '%s' "$info" | jq -r \
      '[.result.agent.tokens | .ask, .ask2, .ask3, .ask4, .ask5, .ask6, .ask7, .ask8]
       | map(select(. != null)) | join("")' 2>/dev/null || true)"
    printf '%s is blocked on its own ask %s, delivered %s and still unanswered: "%s". Re-tasking cannot reach idle,done from here no matter how large a budget you pass — hw next gates on idle,done, and a pane parked on its own ask never gets there until the ask is answered. Answer it, then retry: herdr agent get %s' \
      "$pane" "$ask_seq" "$ask_at" "$ask_text" "$pane"
  elif [ "$st" = blocked ] && [ "$ask_state" = delivered ]; then
    printf '%s already reported done (done_status=%s, done_state=%s) — the ask token you see is stale, published before that report, and there is nothing left to answer. Do not answer it: read the actual report with `herdr agent get %s` and re-task from that, or override with --force if you are deliberately replacing it.' \
      "$pane" "$done_status" "${done_state:-unknown}" "$pane"
  elif [ "$st" = blocked ] && [ "$blocked_reason" = permission ]; then
    printf '%s is blocked on a PERMISSION prompt (%s session), not on a turn that will end. Nothing was sent and the counter did not move. hw will not dismiss it for you: dismissing a permission prompt means DENY, and that is a decision about what the executor may do, not a stuck pane to rescue. Answer it — `herdr agent get %s`, and for a child prompt press ctrl+x then down to reach it — then retry.' \
      "$pane" "${blocked_scope:-root}" "$pane"
  elif [ "$st" = blocked ] && [ "$blocked_reason" = stuck ] \
       && { [ "$done_status" = done ] || [ "$done_status" = blocked ]; }; then
    # THE GAP THE ask_state=delivered BRANCH ABOVE ALREADY CLOSED, missing HERE.
    # `blocked_reason=stuck` outlives the task the same way an ask token does
    # (see the comment atop this function) — measured live 2026-09-03,
    # w7G:p26: reported done_status=done/done_state=delivered, chaining lease
    # live, footer idle, no question or permission anywhere, and herdr still
    # published blocked_reason=stuck/scope=root. `hw unstick` would have
    # RESTARTED a finished, reusable session on the strength of that stale
    # token — its own guard (blocked_reason=stuck + a corroborating settled
    # witness) does not read done_status either, so it would not have refused.
    printf '%s already reported (done_status=%s, done_state=%s) — the blocked/stuck token you see is stale, published on a pane that has finished. Do not `hw unstick` it: that restarts OpenCode for nothing left to do. Read its report with `herdr agent get %s`, then `hw next %s "<next task>"` to reuse it or `hw done <project> <task>` to close it.' \
      "$pane" "$done_status" "${done_state:-unknown}" "$pane" "$pane"
  elif [ "$st" = blocked ] && [ "$blocked_reason" = stuck ]; then
    printf '%s is blocked with NOTHING LEFT TO ANSWER: every prompt it raised has been answered or dismissed (last one in its %s session) and herdr still reports blocked. Nothing was sent and the counter did not move. No budget reaches idle from here and there is no modal for a human to clear, so do not retry with a larger HW_NEXT_WAIT_MS. A deliberate in-place recovery is available: `hw unstick %s`; it first refuses any visible question/permission, then restarts only OpenCode in the same pane (the active conversation is replaced; tab, cwd and worktree stay). If that is not acceptable, close it with `hw done` and dispatch again.' \
      "$pane" "${blocked_scope:-root}" "$pane"
  elif [ "$st" = blocked ] && [ "$blocked_reason" = error ]; then
    printf '%s is blocked on a SESSION ERROR, not on input — there is no modal to dismiss and no budget that reaches idle. Nothing was sent and the counter did not move. Read what failed with `herdr agent get %s`; if that session is unrecoverable this is a relaunch, not a re-task.' \
      "$pane" "$pane"
  else
    printf '%s was still busy after %ds (agent_status=%s), so nothing was sent and the task counter did not move. A mid-turn prompt is NOT dropped — it is acted on — but task N+1 answered inside task N is answered against the wrong state, with the counter already advanced, which is what this gate is for. Retry, or raise the budget: HW_NEXT_WAIT_MS=540000 hw next …' \
      "$pane" "$((wait_ms / 1000))" "$st"
  fi
}

# _next_finished_stale_stuck <agent-info-json>
#
# `done-invoker` publishes done_status first as undelivered, and changes
# done_state to delivered only after the brainer-owned delivery is proved.  That
# pair is therefore stronger completion evidence than the lifecycle plugin's
# blocked/stuck classification, which can outlive the task that produced it.
# Keep this exception deliberately exact: permission, an unanswered ask, a
# genuine stuck pane without delivered done tokens, and a working pane must all
# continue through the normal idle/done gate.
_next_finished_stale_stuck() {
  printf '%s' "$1" | jq -e '
    .result.agent.agent_status == "blocked"
    and .result.agent.tokens.blocked_reason == "stuck"
    and ((.result.agent.tokens.done_status == "done") or (.result.agent.tokens.done_status == "blocked"))
    and .result.agent.tokens.done_state == "delivered"  # MUTATION-ANCHOR: 72-M01 # MUTATION-ANCHOR: 12-M02
  ' >/dev/null 2>&1
}

# Herdr can omit agent_session while an OpenCode pane is classified blocked.
# A stale-stuck completion still needs the native receiver-proved route: Herdr's
# fallback applies a second blocked-state gate and would reject it again.  The
# launch receipt already binds session identity to this exact pane and vendor;
# unlike endpoint recency, those three facts cannot select another conversation
# that happens to share the cwd.
_next_opencode_session_from_receipt() {
  local rundir="$1" pane="$2" sid vendor receipt_pane
  [ -r "$rundir/receipt.jsonl" ] || return 1
  sid="$(jq -sr '[.[] | select(.key == "session_id")][-1].value // empty' "$rundir/receipt.jsonl" 2>/dev/null || true)"
  vendor="$(jq -sr '[.[] | select(.key == "session_vendor")][-1].value // empty' "$rundir/receipt.jsonl" 2>/dev/null || true)"
  receipt_pane="$(jq -sr '[.[] | select(.key == "pane")][-1].value // empty' "$rundir/receipt.jsonl" 2>/dev/null || true)"
  if [[ "$sid" == ses_* ]] && [ "$vendor" = opencode ] && [ "$receipt_pane" = "$pane" ]; then
    printf '%s' "$sid"
    return 0
  fi
  return 1
}

# _next_settled_message <pane> <wait_ms> <info_json>
#
# The refusal for a gate that stopped early because the state was PROVED unable
# to change — not a timeout, and it must not read like one. The distinction is
# operational: a timeout means "retry, perhaps with a bigger budget"; this means
# "no budget will ever work, and the pane needs a decision".
#
# THE FOUR STATES STAY DISTINGUISHABLE. `blocked` once meant four things and
# collapsing any two is how this whole class started, so this branches on the
# reason token to say which of them was contradicted, and never rewrites one as
# another. What is new is only the LAST clause of each: the contradiction is
# now established, so the advice can be definite instead of "retry bigger".
_next_settled_message() {
  local pane="$1" wait_ms="$2" info="$3"
  local st reason scope done_status done_state
  st="$(printf '%s' "$info" | jq -r '.result.agent.agent_status // empty' 2>/dev/null || true)"
  reason="$(printf '%s' "$info" | jq -r '.result.agent.tokens.blocked_reason // empty' 2>/dev/null || true)"
  scope="$(printf '%s' "$info" | jq -r '.result.agent.tokens.blocked_scope // empty' 2>/dev/null || true)"
  done_status="$(printf '%s' "$info" | jq -r '.result.agent.tokens.done_status // empty' 2>/dev/null || true)"
  done_state="$(printf '%s' "$info" | jq -r '.result.agent.tokens.done_state // empty' 2>/dev/null || true)"

  printf '%s: NOTHING WAS SENT and the task counter did not move. ' "$pane"
  if [ "$st" = working ] && { [ "$done_status" = done ] || [ "$done_status" = blocked ]; }; then
    printf 'herdr calls it `working`, but it already reported (done_status=%s) and the cross-check above finds nothing running and no prompt outstanding. That `working` is STALE — the lifecycle producer never cleared it — so the state you are waiting on will not change, at any budget. This executor is finished: read its report with `herdr agent get %s` and either re-task with --force (which files the unreported task as abandoned — check first whether the report simply failed to reach you) or close it with `hw done`.' \
      "$done_status" "$pane"
  elif [ "$st" = working ]; then
    printf 'herdr calls it `working` and the cross-check above finds nothing running and no prompt outstanding, so that `working` is not a turn in progress. Do NOT raise HW_NEXT_WAIT_MS: the budget is not what is missing. Look at the pane, and if it has genuinely finished without reporting, close it with `hw done` or re-task with --force.' ''
  elif [ "$st" = blocked ] && [ "$reason" = permission ]; then
    printf 'herdr calls it blocked on a PERMISSION prompt (%s session), but its own endpoint reports no permission outstanding — that token is STALE, and there is no modal for anyone to answer. Do not go looking for a prompt to approve. `hw unstick` will not touch this either, because the signature is `permission`, not `stuck`. Read what it last did with `herdr agent get %s`, then close it with `hw done` and dispatch again.' \
      "${scope:-root}" "$pane"
  elif [ "$st" = blocked ] && [ "$reason" = question ]; then
    printf 'herdr calls it blocked on a QUESTION (%s session), but its own endpoint reports no question outstanding, and the dismissal attempt above found no modal on screen. That token is STALE. Read what it last did with `herdr agent get %s`, then close it with `hw done` and dispatch again.' \
      "${scope:-root}" "$pane"
  elif [ "$st" = blocked ] && [ "$reason" = error ]; then
    printf 'herdr calls it blocked on a SESSION ERROR and the cross-check confirms nothing is running and nothing is outstanding — so there is no modal, and no budget reaches idle. Read what failed with `herdr agent get %s`; an unrecoverable session is a relaunch, not a re-task.' \
      "$pane"
  elif [ "$st" = blocked ] && [ "$reason" = stuck ] \
       && { [ "$done_status" = done ] || [ "$done_status" = blocked ]; }; then
    # Same gap as the `working`+done_status branch above, on the OTHER token
    # that outlives its task. Measured live 2026-09-03, w7G:p26: settled
    # verdict, blocked_reason=stuck/scope=root, and done_status=done/
    # done_state=delivered — a finished, reusable executor, not a stuck one.
    # `hw unstick`'s own guard does not read done_status either, so pointing
    # here at it would have let it restart a session with nothing left to do.
    printf 'herdr calls it blocked/stuck, but it already reported (done_status=%s, done_state=%s) — that token is STALE, published on a pane that has finished, and the cross-check above only confirms nothing is CURRENTLY moving, not why. Do not `hw unstick` it. Read its report with `herdr agent get %s`, then `hw next %s "<next task>"` to reuse it or `hw done <project> <task>` to close it.' \
      "$done_status" "${done_state:-unknown}" "$pane" "$pane"
  elif [ "$st" = blocked ] && [ "$reason" = stuck ]; then
    printf 'herdr calls it blocked/stuck (%s session) and the cross-check CONFIRMS it: nothing is moving and nothing is outstanding. This is the one case `hw unstick %s` is for — it re-runs this same cross-check before it touches anything, and refuses if the pane turns out to be working. If an in-place restart is not acceptable, close it with `hw done` and dispatch again.' \
      "${scope:-root}" "$pane"
  else
    printf 'herdr calls it `%s`, and the cross-check above finds nothing moving and no prompt outstanding, so that state cannot change on its own. Raising the budget past %ds cannot help. Look at the pane and decide: `hw done` and dispatch again, or --force if you are deliberately replacing an executor that will never report.' \
      "${st:-unknown}" "$((wait_ms / 1000))"
  fi
}

# _next_message <nextseq> <entry_line> <text> <rundir> <brief_path> <wtdir>
#
# THE RE-TASK CARRIES THE PREAMBLE TOO, and until 2026-09-21 it carried none of
# it. RE-MEASURED 2026-09-21 by the selector `_brief_preamble` names: 158
# re-task deliveries across 89 session transcripts, 0 of them carrying one byte
# of the preamble or the epistemic safeguard, and 8 arriving after a
# compaction boundary.
#
# WHAT IS *NOT* CARRIED, because `hw next` already owns it: the framework entry.
# $entry_line is this route's own answer to what the launch says with its
# "OUTSIDE THE FRAMEWORK FLOW" paragraph. Copying the launch MESSAGE would drag
# it in; this copies the fragments whose absence was measured, and nothing else.
#
# BUILT FROM THE RUN ENV, NEVER FROM THE BRAINER'S. `hw next` runs in the
# brainer's process, so $ENGRAM_PROJECT in this shell is the brainer's and would
# be a lie in the executor's prompt. The value the executor actually inherited
# was written once by `_write_run_env` at launch.
_next_message() {
  local nextseq="$1" entry_line="$2" text="$3" rundir="$4" brief_path="$5" wtdir="$6"
  local authorization="${7:-}"
  local preamble epistemic
  preamble="$(
    BRIEF="$brief_path"
    HW_WORKDIR="$wtdir"
    ENGRAM_PROJECT="$(_run_env_value "$rundir" ENGRAM_PROJECT 2>/dev/null || printf brain)"  # MUTATION-ANCHOR: 12-M04
    # THE BASE, READ BACK RATHER THAN RECOMPUTED, for the reason the header of
    # this block already gives: `hw next` runs in the BRAINER process, where
    # neither PROJ nor a worktree path belongs to the executor. It is also the
    # correct value on a re-task — a merge-base does not move when task 1
    # commits, so task 2 is still reviewed from the same branch point, which is
    # the whole point of a boundary. ABSENT stays absent: a run hw could not
    # compute a base for gets no paragraph here either.
    #
    # NO APOSTROPHES IN THESE COMMENTS, and that is not style. This block is the
    # body of a `"$( ... )"` command substitution, and bash does NOT treat `#`
    # as starting a comment there: the first draft wrote "the BRAINER's process"
    # and the parser carried that quote forward until it died 60 lines later on
    # an unrelated `case`, naming a file and a construct with nothing wrong.
    HW_REVIEW_BASE_REF="$(_run_env_value "$rundir" HW_REVIEW_BASE_REF 2>/dev/null || true)"
    HW_REVIEW_BASE_BRANCH="$(_run_env_value "$rundir" HW_REVIEW_BASE_BRANCH 2>/dev/null || true)"
    HW_EXECUTOR_VENDOR="$(_run_env_value "$rundir" HW_EXECUTOR_VENDOR 2>/dev/null || true)"
    HW_PORT_WEB="$(_run_env_value "$rundir" HW_PORT_WEB 2>/dev/null || true)"  # MUTATION-ANCHOR: 750-M03
    # THE LANE THIS RUN WAS ACTUALLY LAUNCHED FOR, read back the same way
    # ENGRAM_PROJECT is above — never the brainer's own $PROJ, which on a
    # --report-to re-task belongs to a different lane entirely. This is what
    # lets _brief_preamble's one-lane-only policy line survive a
    # `hw next` re-task instead of silently dropping it.
    PROJ="$(_run_env_value "$rundir" HW_PROJECT 2>/dev/null || true)"  # MUTATION-ANCHOR: 171-M07a
    # Tasks of a gentle pane default to a required deployed check, as _brief_qa_override reads it.
    if [ "$(_run_dispatch_framework "$rundir")" = gentle ]; then HW_GENTLE_PANE=1; else HW_GENTLE_PANE=0; fi
    # The engram session registered at launch, read back from the receipt. A
    # value starting with none is a failed registration, and says nothing.
    HW_ENGRAM_SESSION="$(jq -r 'select(.key=="engram_session") | .value' "$rundir/receipt.jsonl" 2>/dev/null | tail -1 || true)"
    # An `if`, not a `case`: an unbalanced `)` inside this substitution breaks bash 3.2.
    if [ "${HW_ENGRAM_SESSION#none}" != "$HW_ENGRAM_SESSION" ]; then HW_ENGRAM_SESSION=""; fi
    _brief_preamble
  )"
  epistemic="$(_brief_epistemic)"
  # Before the brief text, after the preamble: the order the launch gives it.
  [ -z "$authorization" ] || text="$(printf '%s\n\n---\n\n%s' "$authorization" "$text")"  # MUTATION-ANCHOR: 703-M01
  # ONE printf, because `$( )` strips trailing newlines: a separator built at the
  # end of its own substitution vanishes and two blocks run together. Same
  # reason _send_brief keeps its separators inside the format string.
  printf 'NEXT TASK (task %s of this session — the previous one is closed and reported).\nYour ask budget and your done-invoker are reset for it: report this one with\ndone-invoker when it is finished, exactly as you did the last.\n\n%s---\n\n%s\n\n---\n\n%s\n\n---\n\n%s' \
    "$nextseq" "$entry_line" "$epistemic" "$preamble" "$text"  # MUTATION-ANCHOR: 12-M03
}

# What a gentle pane's re-task is told, built from the NEW brief by the same
# gate the launch runs (so a bad delivery:, review: or deployed_check: refuses
# the re-task before anything is sent). Sets GENTLE_DELIVERY, GENTLE_REVIEW and
# GENTLE_DEPLOYED_CHECK in the caller.
_next_gentle_read() {  # $1 = run dir, $2 = brief path
  local rundir="$1" nbrief="$2" why d
  [ -n "$nbrief" ] \
    || die "hw next on a gentle pane REFUSED: ODD needs the next task as a brief (--brief <path>) that fixes its delivery strategy, because nobody answers in an executor's pane. Nothing was sent."
  BRIEF="$nbrief"; BRIEF_DECLS="$(_brief_declares "$nbrief")" || BRIEF_DECLS=""
  _gentle_brief_gate
  d="$(gentle_home_dir)"
  why="$(_gentle_home_ready "$d")" \
    || die "hw next on a gentle pane REFUSED: gentle-ai's own home is not ready ($why). Nothing was sent. Provision it with: hw gentle-home"
  # RDD is native (gentle-ai 4.0.0 reviews in process): no plugin or settings ride
  # on the launch args, so any gentle pane can be re-tasked into review: rdd.
}

cmd_next() {
  local pane="" force=0 brief="" text="" dry=0 wait_ms="" next_run=""
  # THE MODE MUST BE ABLE TO CROSS A RE-TASK. Hit twice on one lane: a
  # framework flow was re-tasked into a session hw had launched `--sdd none`, and it
  # worked only because the orchestrator still happened to hold the flow state.
  # `--dry-run` said nothing about it either. A per-task property that cannot be
  # set on the next task is not per-task.
  local next_sdd=""
  pane="${1:-}"; shift || true
  [ -n "$pane" ] || die "usage: hw next <executor-pane> [--run <revived-run-id>] [--sdd <mode>] [--force] [--dry-run] \"<next task>\" | --brief <path>  (flags may appear before or after the task text). Exit 0 means DELIVERED and the counter advanced; every refusal — a gate that would not open, an unknown pane, a task that has not reported — exits 1 and moves nothing."
  while [ $# -gt 0 ]; do
    case "$1" in
      --brief) _need_val "$@"; brief="$2"; shift 2 ;;
      --force) force=1; shift ;;
      # Everything except the send and the counter bump. Worth keeping past the
      # first test: re-tasking a live executor is the one operation here whose
      # mistakes cost somebody else's session.
      --dry-run) dry=1; shift ;;
      --wait-ms) _need_val "$@"; wait_ms="$2"; shift 2 ;;
      --sdd) _need_val "$@"; next_sdd="$2"; shift 2 ;;
      # A REVIVED RUN, named. The pane's cwd resolves to the NEWEST run under
      # .hw, and a run `hw revive --run <id>` brought back need not be it.
      --run) _need_val "$@"; next_run="$2"; shift 2 ;;
      # Same override as the launch path's, reachable here too: `hw next`
      # hands a brief to an ALREADY-EXISTING worktree, which is exactly the
      # case _refuse_if_stale_hooks exists for. Forwarded by the automatic
      # reuse route below (`_reuse_args`) when the outer launch carried it.
      --allow-stale-hooks) ALLOW_STALE_HOOKS=1; shift ;;
      --) shift
          if [ $# -gt 0 ]; then
            if [ -n "$text" ]; then text="$text $*"; else text="$*"; fi
          fi
          break ;;
      -*) die "unknown option: $1" ;;
      # Accumulate and keep looping — a flag is valid in ANY position relative
      # to the task text, not just before it. `hw next <pane> "task" --dry-run`
      # used to swallow `--dry-run` into the text and dispatch for real.
      # MUTATION-ANCHOR: 12-M01
      *)  if [ -n "$text" ]; then text="$text $1"; else text="$1"; fi
          shift ;;
      # MUTATION-ANCHOR-END: 12-M01
    esac
  done

  if [ -n "$brief" ]; then
    [ -z "$text" ] || die "pass a brief or a task text, not both"
    [ -r "$brief" ] || die "cannot read brief: $brief"
    text="$(cat "$brief")"
  fi
  [ -n "$text" ] || die "the next task is empty — there is nothing to send"
  # ONCE, before anything is sent or even gated: the dry run prints the size of
  # the message this block is part of, and a refused authorization refuses both.
  local next_auth=""
  next_auth="$(_next_authorization "$brief")" || die "the re-task brief's authorization was refused above. NOTHING was sent."  # MUTATION-ANCHOR: 703-M02
  case "$next_sdd" in
    ""|speckit|none|gentle) ;;
    *) die "--sdd must be speckit, none or gentle (got: $next_sdd)" ;;
  esac

  _next_load_invoker_lib

  local info cwd vendor status
  info="$(herdr agent get "$pane" 2>/dev/null || true)"
  [ -n "$info" ] || die "$(_next_missing_pane_message "$pane")"
  cwd="$(printf '%s' "$info" | jq -r '.result.agent.cwd // empty' 2>/dev/null || true)"
  vendor="$(printf '%s' "$info" | jq -r '.result.agent.agent // empty' 2>/dev/null || true)"
  status="$(printf '%s' "$info" | jq -r '.result.agent.agent_status // empty' 2>/dev/null || true)"
  [ -n "$cwd" ] || die "herdr reports no cwd for $pane, so hw cannot find its run directory"

  local rundir
  rundir="$(_next_run_dir "$cwd")" \
    || die "no hw run directory at or above $cwd — that pane was not launched by hw, so it has no task state to advance"
  # A REVIVED RUN THAT REPORTED CAN BE RE-TASKED, and only a revived one. Its
  # chaining lease was spent or expired with the first launch, so the lease gate
  # refused it and the brainer had to send task 2 with `channel-send --ruling`,
  # leaving the executor `done-invoker --retask` (measured 2026-10-06
  # on a revived executor in another lane; evidence is in engram). `hw revive` marks the
  # run `reopened`: that marker IS the authority here, in place of a lease.
  # `--run` says "this is the revived run", so a reported run without the marker
  # is refused naming the way back instead of falling into the lease gate.
  local next_reopened=0
  if [ -n "$next_run" ]; then
    case "$next_run" in */*|.*) die "--run takes a run id (a directory name under .hw/), not a path: $next_run" ;; esac
    [ -d "$(dirname "$rundir")/$next_run" ] \
      || die "no run '$next_run' beside $(basename "$rundir") under $(dirname "$rundir"). NOTHING was sent."
    rundir="$(dirname "$rundir")/$next_run"
    # A revived run that re-reported has no reopened marker (the report cleared it) but
    # starts a chaining lease: a live lease is authority too, so task 3 is not refused.
    local next_run_lease=""
    next_run_lease="$(_chaining_lease_eligibility "$rundir" "$(_chaining_current_task "$rundir")")"  # MUTATION-ANCHOR: 785-M03
    if [ -f "$(_run_done_marker "$rundir")" ] && [ ! -f "$(_run_reopened_marker "$rundir")" ] && [ "$next_run_lease" != live ]; then  # MUTATION-ANCHOR: 785-M01
      die "run $next_run of $pane reported and was not revived (no reopened marker), so \`hw next --run\` has no authority to re-task it. NOTHING was sent and the counter did NOT move. Revive it first: hw revive $(_run_env_value "$rundir" HW_PROJECT 2>/dev/null || printf '<project>') $(_run_env_value "$rundir" HW_TASK 2>/dev/null || printf '<task>') --run $next_run — then re-run this. A run still on its chaining lease needs no --run."
    fi
  fi
  # The marker path follows the task counter, so it is read once, before the counter moves.
  local next_reopened_marker; next_reopened_marker="$(_run_reopened_marker "$rundir")"
  [ ! -f "$next_reopened_marker" ] || [ ! -f "$(_run_done_marker "$rundir")" ] || next_reopened=1

  # Validate the effective framework against the measured alive pane before the
  # counter, task directory, surface, or delivery can move. Launch routing finds
  # reuse before validating a requested fresh vendor; that does not make an
  # incompatible framework safe to apply to the pane it actually selected.
  local next_wtdir prior_framework
  next_wtdir="$(dirname "$(dirname "$rundir")")"
  [ -d "$next_wtdir" ] || next_wtdir="$cwd"
  # A re-task hands a brief to a worktree that ALREADY EXISTS — the exact
  # shape of the branch-predates-the-cleanup problem this guards against — so
  # it is checked here too, not just at a fresh launch. No $MAIN/$BASE is
  # known in this call shape (`hw next <pane> "task"` names neither), so the
  # remedy text falls back to naming the mechanism generically rather than a
  # command this function cannot verify applies.
  _refuse_if_stale_hooks "$next_wtdir" "" ""
  # `_receipt` inside that call is a no-op here: $WT/$HW_RUN are the FRESH-
  # launch route's globals and are unset on this route. $rundir is this route's
  # own equivalent, so the override is recorded into it directly instead.
  [ -z "$STALE_HOOKS_ALLOWED_FILES" ] \
    || _receipt_into "$rundir" stale_hooks_allowed "$STALE_HOOKS_ALLOWED_FILES" "--allow-stale-hooks (hw next)"
  prior_framework="$(_run_dispatch_framework "$rundir")"
  # A GENTLE PANE STAYS GENTLE. ODD is in its system prompt for the whole
  # session, so every task in it is an ODD task: what is per task (delivery, the
  # RDD bullet, the deployed check) is read from the NEW brief and re-injected
  # as the message's lead block. `--sdd gentle` on a pane that was not launched
  # gentle cannot work (ODD enters only at launch), and another mode on one that
  # was would leave ODD asking an unattended pane its questions.
  local next_gentle=0
  if [ "$prior_framework" = gentle ]; then
    case "$next_sdd" in ""|gentle) next_gentle=1; next_sdd="" ;;  # MUTATION-ANCHOR: 781-M06
      *) die "hw next --sdd $next_sdd REFUSED: $pane was launched --sdd gentle, so ODD is in its system prompt for the whole session and cannot be turned off for one task. Nothing was sent. Re-task it as gentle (omit --sdd) or launch a fresh executor." ;;
    esac
  elif [ "$next_sdd" = gentle ]; then
    die "hw next --sdd gentle REFUSED: ODD enters Claude's system prompt at launch, and $pane was not launched --sdd gentle (its framework is ${prior_framework:-undetermined}). Nothing was sent. Launch it fresh: hw <project> <task> --sdd gentle --agent claude --keep-pane --why \"...\""
  fi
  local next_gentle_block=""
  if [ "$next_gentle" = 1 ]; then
    _next_gentle_read "$rundir" "$brief"
    # PROJ is the RUN's lane (the RDD bullet names its base), never the brainer's.
    next_gentle_block="$(PROJ="$(_run_env_value "$rundir" HW_PROJECT 2>/dev/null || true)"; _gentle_preamble)"
  fi
  _validate_sdd_vendor_pair "$vendor" "$next_sdd" "$next_wtdir" "$prior_framework"
  # A re-task cannot change the pane's model, so the launch record is the only
  # place the orchestrator's model exists by the time an SDD mode is applied to
  # it. A pane launched sonnet/none and later handed --sdd speckit is the same
  # inversion the launch gate refuses, arriving by the one door that has no
  # --model flag at all.
  _validate_sdd_model_pair "$(_next_dispatch_model "$rundir")" "this pane was launched with that model; hw next cannot change it"

  # BEFORE the gates that cost time, so `--dry-run` answers the question a
  # brainer actually has — "is this pane still worth reusing?" — without
  # sending anything.
  _report_pane_context "$pane"

  # Precedence is deliberate: an explicit call-site setting wins for an
  # exceptional handoff, then the target's launch contract, then 90s only for
  # legacy runs that predate the contract. HW_INVOKER_WAIT_MS is unrelated: it
  # gates executor -> brainer delivery and must not silently tune this gate.
  if [ -z "$wait_ms" ]; then
    wait_ms="${HW_NEXT_WAIT_MS:-}"
    [ -n "$wait_ms" ] || wait_ms="$(_next_wait_from_run "$rundir" || true)"
    [ -n "$wait_ms" ] || wait_ms=90000
  fi
  case "$wait_ms" in ''|*[!0-9]*) die "--wait-ms/HW_NEXT_WAIT_MS must be a positive integer in milliseconds (got: $wait_ms)" ;; esac

  local seq="" marker
  [ -r "$rundir/task" ] && seq="$(tr -dc '0-9' < "$rundir/task" 2>/dev/null || true)"
  case "$seq" in ''|0) seq=1 ;; esac
  marker="$(_run_done_marker "$rundir")"

  # An unreported task is not a finished one. Replacing it silently would file
  # its work as never-dispatched and leave the brainer believing both landed.
  #
  # THE DERIVATION MOVED to `_report_evidence` on 2026-09-16, unchanged, so
  # `hw done` could refuse an in-use pane on the SAME two signals rather than
  # inventing a second answer to the same question. Its comment carries the
  # incident this shape came from; only the refusal sentence is owned here,
  # because re-tasking and closing are different things to be stopped from.
  if [ ! -f "$marker" ] && [ "$force" != 1 ]; then
    _next_ev="$(_report_evidence "$pane" "$rundir")"
    case "$_next_ev" in
      token*)
        _report_evidence_explain "$pane" "$_next_ev" "$marker"
        info "proceeding without --force; read its report with: herdr agent get $pane"
        ;;
      *)
        die "task $seq of $(basename "$rundir") has not reported: no $marker, and $pane publishes no done token either — so there is no evidence it finished, from either source. Wait for its done-invoker, or override with --force if you are deliberately re-tasking an executor that will never report."
        ;;
    esac
  fi

  # channel-send, not `herdr agent prompt`: the receipt is the point. Resolved
  # BEFORE the gate, so a dry run proves the route too — which route an executor
  # gets is the part that differs per harness, so it is the part worth being
  # able to check without sending anything.
  local route target endpoint
  invoker_resolve_sender "$pane"
  if _next_finished_stale_stuck "$info" \
     && [ "$INVOKER_SENDER_VENDOR" = opencode ] \
     && [ -z "$INVOKER_SENDER_SESSION" ] \
     && [ -n "$INVOKER_SENDER_ENDPOINT" ]; then
    INVOKER_SENDER_SESSION="$(_next_opencode_session_from_receipt "$rundir" "$pane" || true)"
  fi
  if _vendor_has_native_transport "${INVOKER_SENDER_VENDOR:-}" \
     && [ -n "$INVOKER_SENDER_SESSION" ] \
     && [ -n "$INVOKER_SENDER_ENDPOINT" ]; then
    route="$INVOKER_SENDER_VENDOR"
    target="$INVOKER_SENDER_SESSION"
    endpoint="$INVOKER_SENDER_ENDPOINT"
  else
    route=herdr; target="$pane"; endpoint=-
  fi

  # A DRY RUN NEVER ENTERS THE DELIVERY GATE, and never touches the pane.
  #
  # It used to sit AFTER the wait, so `hw next <pane> --dry-run "…"` inherited
  # the full budget against a pane that would never change and printed nothing
  # for over three minutes before being killed — measured 2026-08-27 against
  # w4C:p9W, exit 144. A command whose entire purpose is to tell you what would
  # happen must not be the slowest way to find out, and "the gate would not
  # open" is the single most useful thing it can report.
  #
  # It also used to run AFTER invoker_clear_opencode_question, which DISMISSES A
  # MODAL. A dry run that answers a question on the executor's behalf is worse
  # than one that waits.
  #
  # So the gate is described, not entered: the witness reads the same facts the
  # gate would, read-only, and says which way it would go.
  if [ "$dry" = 1 ]; then
    local chaining_seconds_dry="" chaining_state_dry=""
    chaining_seconds_dry="$(_run_env_value "$rundir" HW_CHAINING_LEASE_SECONDS || true)"
    if [ -n "$chaining_seconds_dry" ] && [ "$next_reopened" = 0 ]; then  # MUTATION-ANCHOR: 785-M02
      chaining_state_dry="$(_chaining_lease_eligibility "$rundir" "$seq")"
      if [ "$chaining_state_dry" != live ]; then # MUTATION-ANCHOR: 37-M18a
        die "$(_chaining_lease_ineligible_message "$chaining_state_dry" "$seq" "$rundir")"
      fi
    fi
    local nextseq_dry="$((seq + 1))"
    [ "$next_gentle" = 0 ] || info "gentle pane: task $nextseq_dry carries ODD's per-task block from $brief — $(_gentle_rdd_word), delivery $GENTLE_DELIVERY, deployed check $GENTLE_DEPLOYED_CHECK"
    ok "dry run: $pane is reachable, on task $seq of $(basename "$rundir") — NOTHING was sent and the pane was not touched"
    info "route $route -> $target${endpoint:+ (endpoint $endpoint)}"
    info "would create $rundir/t$nextseq_dry and deliver $(printf '%s' "$(_next_message "$nextseq_dry" "${next_gentle_block:+$next_gentle_block$'\n\n'}" "$text" "$rundir" "$brief" "$next_wtdir" "$next_auth")" | wc -c | tr -d ' ') bytes as task $nextseq_dry ($(printf '%s' "$text" | wc -c | tr -d ' ') of them the task text; the rest is the preamble hw prepends, minus any framework entry line)"
    [ "$next_reopened" = 0 ] || info "run was revived (reopened): hw revive is the authority for this re-task, so its spent chaining lease is not consulted; task $nextseq_dry gets its own done-invoker with no --retask"
    if [ -n "$chaining_seconds_dry" ] && [ "$next_reopened" = 0 ]; then
      info "this one use would be consumed only after successful delivery; dry-run lease state is volatile and was not touched"
    fi
    if _next_finished_stale_stuck "$info"; then
      ok "the delivery gate WOULD OPEN: $pane has authoritative delivered completion tokens; its blocked/stuck lifecycle token is stale (budget ${wait_ms}ms, unspent)"
    else
      case "$status" in
        idle|done)
          ok "the delivery gate WOULD OPEN: herdr reports $pane $status, so a prompt would be read now (budget ${wait_ms}ms, unspent)"
            ;;
          *)
            info "herdr reports $pane $status, so the gate would wait (budget ${wait_ms}ms). Cross-checking whether that state can still change — this does not send, dismiss or wait:"
            witness_state "$pane"
            case "$WITNESS_VERDICT" in
              live)
                info "gate verdict: WOULD WAIT, and correctly. $WITNESS_DETAIL"
                ;;
              awaiting-human)
                warn "gate verdict: WOULD NOT OPEN. $WITNESS_DETAIL"
                ;;
              settled)
                warn "gate verdict: WOULD NOT OPEN, and no budget would help. $WITNESS_DETAIL"
                warn "a real run would refuse here after confirming that twice, instead of spending ${wait_ms}ms on it"
                ;;
              *)
                info "gate verdict: UNDETERMINED. $WITNESS_DETAIL"
                ;;
            esac
            ;;
      esac
    fi
    if [ -n "$next_sdd" ]; then
      local dry_entry
      dry_entry="$(_framework_entry "$next_sdd")"
      info "would set SDD mode $next_sdd on the worktree, entry ${dry_entry:-<none for this mode>}"
    else
      info "SDD surface: unchanged — compatibility checked using the effective mode; live surface takes precedence, with the launch record as fallback when no live mode is obtained"
    fi
    return 0
  fi

  # Gate BEFORE the counter moves: a bump that is not followed by a delivered
  # prompt would leave the executor's invokers pointing at a task it was never
  # given, and its next done-invoker would write a marker for nothing.
  # BEFORE the wait, not after it. A pane reading `blocked` is never going to
  # reach idle on its own — that is the whole finding — so spending the budget
  # first only delays the rescue by however long the caller was willing to
  # wait. Measured: 94s to deliver with the attempt after the wait, ~4s with it
  # before. The helper re-checks `blocked` itself and returns immediately on
  # anything else, so calling it here costs one agent.get on the normal path.
  # witness_wait, not invoker_wait_for_brainer: same gate, same budget, same
  # `idle,done` condition, plus the one thing the raw wait cannot do — notice
  # that it is waiting on a state which has been PROVED unable to change, and
  # stop. Exit 5 is that, and only that. Exit 2 is still an ordinary timeout,
  # which is what an `unknown` cross-check must cost: nothing extra.
  local gate_rc=0
  if _next_finished_stale_stuck "$info"; then
    info "$pane already reported with delivered completion tokens; ignoring its stale blocked/stuck lifecycle token and re-tasking without --force."
  else
    if invoker_clear_opencode_question "$pane"; then
      info "$pane was blocked on its own opencode question modal; dismissed it (the executor sees question.rejected) and the pane reached idle. Re-tasking now."
    fi
    witness_wait "$pane" "$wait_ms" || gate_rc=$?
  fi
  if [ "$gate_rc" != 0 ]; then
    # One rescue before the die, and only one shape of rescue. An opencode
    # executor parked on its own `question` modal reads `blocked` and STAYS
    # there — the plugin has no event left to publish, so no budget and no
    # retry reaches idle. That is the dead end this whole gate used to have no
    # move for, and pressing one key is the move. `--force` is NOT: it files
    # the task as abandoned to get past a pane that is merely waiting.
    if invoker_clear_opencode_question "$pane"; then
      info "$pane was blocked on its own opencode question modal; dismissed it (the executor sees question.rejected) and the pane reached idle. Re-tasking now."
    else
      local busy_info
      busy_info="$(herdr agent get "$pane" 2>/dev/null || true)"
      # A GATE THAT GIVES UP SAYS WHICH FACT IT READ AND WHAT CONTRADICTED IT.
      # For eight hours this class of failure looked like "timed out" from
      # outside, which is why nobody could act on it. WITNESS_DETAIL names the
      # reader, the source and the disagreement; the existing message still
      # carries the state-specific advice, so both are printed.
      # A PANE WAITING FOR A PERSON IS NOT A BUSY PANE, and the advice is not
      # the same. Without this branch gate_rc 6 fell through to the generic
      # busy message, which tells the caller to wait or retry — for a state that
      # only a human answering a prompt can move. The witness has already read
      # that pane's own endpoint and had it CONFIRM an outstanding
      # /question or /permission, so say that, and say the one thing that is
      # never the answer here: --force files an unreported task as abandoned to
      # get past a pane that is merely waiting for someone to press a key.
      if _next_finished_stale_stuck "$busy_info"; then
        info "$pane reported while the gate was checking it; delivered completion tokens now prove blocked/stuck is stale. Re-tasking without --force."
        # Route selection happened before the wait. Re-resolve it from the
        # completion snapshot so an OpenCode session omitted while blocked can
        # use its exact launch-receipt binding instead of falling into Herdr's
        # second blocked-state gate.
        invoker_resolve_sender "$pane"
        if [ "$INVOKER_SENDER_VENDOR" = opencode ] \
           && [ -z "$INVOKER_SENDER_SESSION" ] \
           && [ -n "$INVOKER_SENDER_ENDPOINT" ]; then
          INVOKER_SENDER_SESSION="$(_next_opencode_session_from_receipt "$rundir" "$pane" || true)"
        fi
        if _vendor_has_native_transport "${INVOKER_SENDER_VENDOR:-}" \
           && [ -n "$INVOKER_SENDER_SESSION" ] \
           && [ -n "$INVOKER_SENDER_ENDPOINT" ]; then
          route="$INVOKER_SENDER_VENDOR"
          target="$INVOKER_SENDER_SESSION"
          endpoint="$INVOKER_SENDER_ENDPOINT"
        else
          route=herdr; target="$pane"; endpoint=-
        fi
      else
        if [ "$gate_rc" = 6 ]; then
          warn "$WITNESS_DETAIL"
          die "$pane is waiting for a PERSON, not for time: its own endpoint confirms a prompt is outstanding (see above). The task counter did NOT move and nothing was sent. Answer that prompt on the pane — or have ${HW_OPERATOR:-the operator} answer it — and re-run this identical hw next. Do not raise the budget and do not reach for --force: neither answers a question."
        fi
        if [ "$gate_rc" = 5 ]; then
          warn "$WITNESS_DETAIL"
          die "$(_next_settled_message "$pane" "$wait_ms" "$busy_info")"
        fi
        [ -z "${WITNESS_DETAIL:-}" ] || warn "cross-check: $WITNESS_DETAIL"
        die "$(_next_busy_message "$pane" "$wait_ms" "$busy_info")"
      fi
    fi
  fi

  local nextseq="$((seq + 1))"
  mkdir -p "$rundir/t$nextseq" || die "cannot create $rundir/t$nextseq"

  # The mode is set on the WORKTREE before the brief is sent, in that order: an
  # entry command that arrives before its framework is loaded is an instruction
  # the executor cannot follow.
  local entry_line=""
  if [ -n "$next_sdd" ]; then
    local entry
    entry="$(_framework_entry "$next_sdd")"
    _apply_sdd "$next_wtdir" "$next_sdd" \
      || die "--sdd $next_sdd has no speckit-* skill on disk for $next_wtdir, so nothing was sent and the task counter did not move. Install Spec Kit where it is used, or re-task with --sdd none."
    if [ -n "$entry" ]; then
      entry_line="$(printf 'FIRST ACTION, before anything else: run %s for this task. The framework mode\nfor it is %s and what follows is its input, not a licence to start editing.\n\n' "$entry" "$next_sdd")"
      # A plain assignment, not `$( )`: command substitution strips trailing
      # newlines, and _next_message prints this directly before its `---`.
      [ "$next_sdd" != speckit ] || entry_line="$entry_line"$'\n\n'"$(_speckit_phase_note)"$'\n\n'
    else
      entry_line="$(printf 'Framework mode for this task: %s. hw forces no entry command.\n\n' "$next_sdd")"
    fi
  fi

  # A gentle pane's lead block: what ODD fixes for THIS task, from its own brief.
  # An assignment, not `$( )` around the whole thing: a trailing blank line is
  # part of the block and command substitution strips it.
  [ "$next_gentle" = 0 ] || entry_line="$next_gentle_block"$'\n\n'  # MUTATION-ANCHOR: 781-M04

  # The executor is told its own bookkeeping changed, because its budgets did —
  # and, since 2026-09-21, it is told the rest of what a launch tells one.
  local msg
  msg="$(_next_message "$nextseq" "$entry_line" "$text" "$rundir" "$brief" "$next_wtdir" "$next_auth")"

  # New bounded-chaining runs have a lease only after proved report delivery.
  # Hold the SAME per-run lock as done-invoker and the detached expiry action
  # across delivery, lease consumption, and counter advance. Expiry therefore
  # cannot close between those acts, and a failed send leaves the lease intact.
  local chaining_seconds="" chaining_locked=0
  chaining_seconds="$(_run_env_value "$rundir" HW_CHAINING_LEASE_SECONDS || true)"
  [ "$next_reopened" = 0 ] || chaining_seconds=""
  if [ -n "$chaining_seconds" ]; then
    _chaining_lock_take "$rundir" || die "could not acquire the run/task lock before consuming its chaining lease"
    chaining_locked=1
    local chaining_state
    chaining_state="$(_chaining_lease_eligibility "$rundir" "$seq")"
    if [ "$chaining_state" != live ]; then
      _chaining_lock_release
      die "$(_chaining_lease_ineligible_message "$chaining_state" "$seq" "$rundir")"
    fi
  fi

  local send_rc=0  # MUTATION-ANCHOR: 05-M05
  if [ "$route" = opencode ]; then
    "$HW_BIN_DIR/channel-send" --require processed --id "$(basename "$rundir"):t$nextseq" \
      "$route" "$target" "$endpoint" "$msg" >/dev/null || send_rc=$?
  else
    "$HW_BIN_DIR/channel-send" "$route" "$target" "$endpoint" "$msg" >/dev/null || send_rc=$?
  fi
  # EXIT 4 IS NOT A FAILED DELIVERY. channel-send returns it only after the
  # message is proven present in the receiver's session; what did not finish is
  # the TURN. Rolling the counter back there would be the worse of the two
  # errors: the executor holds task N+1 while its invokers still call it N, so
  # its report would be filed against the task it no longer has.
  #
  # This is the incident that made the distinction necessary. On 2026-08-25 the
  # old synchronous route returned `HTTP 500 UnknownError` re-tasking
  # a product-lane task; hw called it undelivered, rolled back, and
  # the orchestrator — reasonably, given what hw told it — concluded the
  # executor was unreachable and closed it. Nothing proved the message had NOT
  # landed. Now the two outcomes have different exit codes and different words.
  if [ "$send_rc" = 4 ] && [ "$route" = opencode ]; then  # MUTATION-ANCHOR: 05-M04
    warn "$pane ACCEPTED task $nextseq — it is in its session — but the turn it started did not complete (see the message above)."
    warn "that is a turn failure, not a delivery failure, so the counter advances: rolling it back would leave the executor holding a task its invokers do not know about."
    info "check what it did with: herdr agent read $pane --source recent-unwrapped"
  elif [ "$send_rc" != 0 ]; then
    rmdir "$rundir/t$nextseq" 2>/dev/null || true
    [ "$chaining_locked" = 0 ] || _chaining_lock_release
    die "delivery to $pane failed via $route (exit $send_rc). The task counter was NOT advanced, so the executor is still on task $seq and its next done-invoker still refers to the task it actually ran. The pane is untouched and still usable — a failed delivery is not a reason to close an executor; fix the transport and retry."  # MUTATION-ANCHOR: 71-M03
  fi

  # Only now. The counter is the executor's contract with its own invokers, and
  # advancing it before proven delivery is how a task gets a budget but no brief.
  if [ "$chaining_locked" = 1 ]; then
    # MUTATION-ANCHOR: 37-M18
    printf 'state=consumed\nconsumed_at=%s\nconsumed_by=%s\n' \
      "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${HERDR_PANE_ID:-unknown}" >> "$(_chaining_lease_path "$rundir")" \
      || { _chaining_lock_release; die "task delivered, but its chaining lease could not be marked consumed; refusing to advance the counter"; }
    # MUTATION-ANCHOR-END: 37-M18
  fi
  # THE NEW TASK'S BOUNDARY, when its brief declares one — keyed by the task
  # number, so done-invoker holds task N+1 to its own brief and not to task 1's.
  # After proven delivery and with the counter, never before: a failed send
  # must not leave a gate for a task that never arrived.
  # The SAME gate _next_message's preamble ran — inherited TASK_KIND and all —
  # so the executor is never held to a judgment its prompt did not ask for (on
  # the reuse route a name-inferred or `asked: investigate` kind is in scope).
  if [ -n "$brief" ] && BRIEF="$brief" _design_gate_applies; then
    _receipt_into "$rundir" "design_boundary_t$nextseq" "$(_brief_boundary "$brief")" \
      "brief frontmatter boundary: in $brief at hw next — done-invoker requires an APPROVED design judgment"
  fi
  # THE DEPLOYED CHECK OF A GENTLE RE-TASK, keyed by task for the same reason.
  if [ "$next_gentle" = 1 ]; then
    local dc_src="brief frontmatter deployed_check: in $brief at hw next (required when absent) — done-invoker holds a done to it"
    _receipt_into "$rundir" "deployed_check_t$nextseq" "$GENTLE_DEPLOYED_CHECK" "$dc_src"  # MUTATION-ANCHOR: 781-M05
  fi
  # MUTATION-ANCHOR: 37-M19
  printf '%s\n' "$nextseq" > "$rundir/task" \
    || { [ "$chaining_locked" = 0 ] || _chaining_lock_release; die "delivered, but could not write $rundir/task — the executor is on task $nextseq and its invokers still think $seq. Write that file by hand before it reports."; }
  # MUTATION-ANCHOR-END: 37-M19
  # The reopened marker belongs to the task it reopened; task N+1 owes its own
  # report and must not inherit the licence (done-invoker clears it the same way).
  [ "$next_reopened" = 0 ] || rm -f "$next_reopened_marker" 2>/dev/null || true
  [ "$chaining_locked" = 0 ] || _chaining_lock_release
  _ledger_next "$pane" "$rundir" "$nextseq" "$brief"
  ok "task $nextseq dispatched to $pane via $route (${vendor:-unknown} executor, $(printf '%s' "$msg" | wc -c | tr -d ' ') bytes)"
  info "state for it: $rundir/t$nextseq — fresh ask budget, no done marker"
  return 0
}

