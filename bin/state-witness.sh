# shellcheck shell=bash
# state-witness.sh — CAN THIS STATE STILL CHANGE?
#
# SOURCED, never executed: no shebang, not executable, not symlinked into ~/bin.
#
# WHY THIS FILE EXISTS
#
# Every gate in this toolchain waits on `agent_status`, and on this machine
# `agent_status` goes stale. Measured 2026-08-27, three panes, three different
# lies, all on one host:
#
#   w4C:p9D  agent_status=working  — delivered its report at 01:49:35Z and was
#                                   still `working` eight hours later
#   w4C:p97  blocked reason=permission scope=child — nothing left to answer
#   w4C:p9W  blocked reason=stuck scope=child — while visibly running a task
#
# A gate that cannot tell a live state from a stale one does not fail. It waits
# out its whole budget and then reports nothing. `w41:p6H` had a 1200s and a
# 600s `herdr agent wait` burning on an `idle` that could not arrive.
#
# THE PRODUCER IS NOT FIXED HERE, AND THIS MUST HOLD WITHOUT IT BEING FIXED.
# Why the lifecycle producer goes stale is somebody else's open question. This
# is the consumer side: given a state that may be a lie, decide whether it can
# still change, using evidence that does not come from the thing that lied.
#
# ── THE AUTHORITY THAT LOOKED OBVIOUS AND IS NOT ─────────────────────────────
#
# The brief for this work named the pane's own OpenCode HTTP endpoint as "the
# one thing that did not lie, every time" and pointed the fix at cross-checking
# `agent_status` against `/api/session/active`. It also, correctly, said that
# was a measurement about agreement in those cases and NOT a proof of authority,
# and said to establish what the endpoint guarantees before building on it.
#
# ESTABLISHING IT REFUTES THAT DESIGN. Measured 2026-08-27, same probe, three
# panes, literal output:
#
#   w4C:p9D  (STALE, `working`)   /api/session/active -> {"data":{}}  /permission -> []  /question -> []
#   w4C:p97  (STALE, `blocked`)   /api/session/active -> {"data":{}}  /permission -> []  /question -> []
#   w4C:pAH  (GENUINELY WORKING)  /api/session/active -> {"data":{}}  /permission -> []  /question -> []
#
# Byte-identical. `/api/session/active` is empty on a pane that is actively
# running a task. A gate built on "empty means nothing can change" would have
# declared a working executor settled and abandoned it — the exact failure this
# work exists to prevent, shipped as the fix.
#
# So the endpoint is demoted to the one job the measurement supports: proving
# that NO HUMAN PROMPT IS OUTSTANDING. `/question` and `/permission` are real,
# positive, load-bearing facts. `/api/session/active` is not consulted at all,
# because nothing establishes what its emptiness means.
#
# ── WHAT ACTUALLY DISCRIMINATES: PROGRESS OVER TIME ──────────────────────────
#
# The question a gate needs answered is not "what state is this in" — the whole
# problem is that nobody can answer that — but "is anything moving". That is
# observable without trusting any status field, and it is vendor-neutral:
#
#   3 samples of the pane's own visible screen, 2s apart, hashed:
#     w4C:p9D  d0b0338b7965a937  d0b0338b7965a937  d0b0338b7965a937   STATIC
#     w4C:p97  6b2a508a9897f88e  6b2a508a9897f88e  6b2a508a9897f88e   STATIC
#     w4C:pAH  6089d0a8e2249552  5e8cfcbfb7b67e86  079c84630c8b88a6   MOVING
#
# pAH is mislabelled `blocked` by the producer AND has an all-empty endpoint,
# and the screen still classifies it correctly. That is the property the design
# needs: it holds without knowing why the producer lies.
#
# ── AN UNREACHABLE AUTHORITY IS NOT EVIDENCE OF ANYTHING ─────────────────────
#
# This is the rule the verdict is built around, and it is here because half this
# repository's history is the same mistake: a failed precondition rendered as a
# normal terminal outcome. A cross-check that guesses when it cannot ask would
# be a new instance of it wearing the fix's clothes.
#
# So the verdicts are asymmetric on purpose:
#
#   live            SOMETHING POSITIVE was observed to move.
#   awaiting-human  a prompt was POSITIVELY read as outstanding.
#   settled         EVERY input was read successfully AND every one was negative.
#   unknown         any required input could not be read or parsed. This is not
#                   "probably live" and not "probably stale". A caller that gets
#                   `unknown` must fall back to exactly the behaviour it had
#                   before this file existed, and say the cross-check was
#                   unavailable.
#
# TWO verdicts license cutting a wait short, and both require a positive read.
# `settled` requires every reader to have succeeded and every one to have said
# no. `awaiting-human` requires the endpoint to have been asked successfully and
# to have answered YES — a real prompt is outstanding.
#
# `awaiting-human` USED TO BE COMPUTED AND THEN DISCARDED, and that was the gap.
# witness_wait folded it in with `unknown` and reset the streak, so a pane whose
# own endpoint had just CONFIRMED an outstanding question kept the full budget
# and the caller was told, 540s later, that the pane "was still working" —
# advice to retry, for a state that a retry cannot change and only a person can.
# That is the same class of error as waiting on a stale status, arrived at from
# the opposite direction: not a fact that could not be read, but a fact that was
# read, was positive, and was thrown away.
#
# Callers must set, before sourcing:
#   WITNESS_RPC   absolute path to herdr-rpc (it is not on PATH)
# and must have `herdr`, `jq`, `curl` and `shasum` available.

# Sampling. Deliberately conservative: the cost of over-sampling is seconds, the
# cost of under-sampling is abandoning a working executor.
: "${WITNESS_SAMPLES:=3}"
: "${WITNESS_INTERVAL_S:=2}"
: "${WITNESS_READ_LINES:=40}"
: "${WITNESS_CURL_TIMEOUT_S:=5}"

# Set by witness_state. Never partially set: on any path, both are assigned.
WITNESS_VERDICT=""
WITNESS_DETAIL=""

# witness_screen_hash <pane>
#
# One hash of the pane's visible screen. Exit 1 — never a hash, never an empty
# string that a caller might compare equal to another empty string — when the
# read did not succeed. `.read.text` is checked for PRESENCE with `jq -e`, not
# read with `// empty`: a pane whose screen is legitimately blank is readable
# and must not be confused with an rpc that failed.
witness_screen_hash() {
  local pane="$1" out
  out="$("$WITNESS_RPC" call pane.read \
    "$(printf '{"pane_id":"%s","source":"visible","lines":%s}' "$pane" "$WITNESS_READ_LINES")" \
    2>/dev/null)" || return 1
  [ -n "$out" ] || return 1
  printf '%s' "$out" | jq -e 'has("read") and (.read | has("text")) and (.read.text | type == "string")' \
    >/dev/null 2>&1 || return 1
  printf '%s' "$out" | jq -j '.read.text' 2>/dev/null | shasum -a 256 | cut -c1-16
}

# witness_screen <pane>
#
# Prints "moved <hash>" or "static <hash>". Exit 1 if ANY sample failed to read:
# a witness that could not watch is not a witness that saw nothing.
witness_screen() {
  local pane="$1" first="" now_hash="" i=1
  first="$(witness_screen_hash "$pane")" || return 1
  while [ "$i" -lt "$WITNESS_SAMPLES" ]; do
    sleep "$WITNESS_INTERVAL_S"
    now_hash="$(witness_screen_hash "$pane")" || return 1
    if [ "$now_hash" != "$first" ]; then printf 'moved %s' "$first"; return 0; fi
    i=$((i + 1))
  done
  printf 'static %s' "$first"
}

# witness_endpoint <pane>
#
# The pane's own OpenCode HTTP address, from its argv — the same derivation
# invoker_resolve_sender uses, and for the same reason: the port exists only in
# the live process, never in the run env. Exit 1 when there is no endpoint to
# ask (not an opencode pane, no --port, process-info unreadable). That is an
# absent authority, which is `unknown` territory, never a negative answer.
witness_endpoint() {
  local pane="$1" info port=""
  info="$(herdr pane process-info --pane "$pane" 2>/dev/null || true)"
  [ -n "$info" ] || return 1
  # `endswith("/opencode")` as well as the bare name, matching the argv test
  # cmd_unstick already applies. invoker_resolve_sender's copy of this jq
  # accepts only the bare string, so an opencode launched by absolute path
  # resolves no endpoint there — here that would silently downgrade every such
  # pane to `unknown`, which is safe but blind, and this check is the only
  # reason a caller may act at all.
  port="$(printf '%s' "$info" | jq -r '
    first(.result.process_info.foreground_processes[]?
      | select(any(.argv[]?; type == "string"
                   and (. == "opencode" or endswith("/opencode")))) | .argv) as $argv
    | ($argv | index("--port")) as $i
    | if $i == null then empty else $argv[$i + 1] // empty end
  ' 2>/dev/null)" || port=""
  case "$port" in ''|*[!0-9]*) return 1 ;; esac
  printf 'http://127.0.0.1:%s' "$port"
}

# witness_prompts <endpoint>
#
# Prints "<question_count> <permission_count>". Exit 1 unless BOTH answered and
# BOTH parsed as JSON arrays. A 500, a truncated body or an unexpected shape is
# an authority that could not be asked — it is not "no prompts outstanding".
witness_prompts() {
  local endpoint="$1" q p qn pn
  q="$(curl -s -m "$WITNESS_CURL_TIMEOUT_S" "$endpoint/question" 2>/dev/null)" || return 1
  p="$(curl -s -m "$WITNESS_CURL_TIMEOUT_S" "$endpoint/permission" 2>/dev/null)" || return 1
  qn="$(printf '%s' "$q" | jq -e 'if type == "array" then length else empty end' 2>/dev/null)" || return 1
  pn="$(printf '%s' "$p" | jq -e 'if type == "array" then length else empty end' 2>/dev/null)" || return 1
  printf '%s %s' "$qn" "$pn"
}

# witness_state <pane>
#
# Sets WITNESS_VERDICT and WITNESS_DETAIL. Always returns 0: a witness that
# cannot reach a conclusion reports `unknown`, and must never take a caller
# down with it under `set -e`.
#
# WITNESS_DETAIL is not decoration. A gate that gives up has to say which fact
# it read, from where, and what contradicted it — "timed out" is what this whole
# class of failure looked like from outside for eight hours.
witness_state() {
  local pane="$1" status screen screen_state screen_hash endpoint prompts qn pn
  local reason scope info

  info="$(herdr agent get "$pane" 2>/dev/null || true)"
  status="$(printf '%s' "$info" | jq -r '.result.agent.agent_status // empty' 2>/dev/null || true)"
  reason="$(printf '%s' "$info" | jq -r '.result.agent.tokens.blocked_reason // empty' 2>/dev/null || true)"
  scope="$(printf '%s' "$info" | jq -r '.result.agent.tokens.blocked_scope // empty' 2>/dev/null || true)"
  local claim="agent_status=${status:-unreadable}"
  [ -z "$reason" ] || claim="$claim/${reason}${scope:+:$scope}"

  # The endpoint first, because an outstanding prompt outranks everything else:
  # a pane with a real question on screen is neither stale nor busy, it is
  # waiting for a person, and that is its own answer.
  endpoint="$(witness_endpoint "$pane" || true)"
  if [ -n "$endpoint" ]; then
    prompts="$(witness_prompts "$endpoint" || true)"
  else
    prompts=""
  fi
  if [ -n "$prompts" ]; then
    qn="${prompts%% *}"; pn="${prompts##* }"
    if [ "$qn" -gt 0 ] || [ "$pn" -gt 0 ]; then
      WITNESS_VERDICT=awaiting-human
      WITNESS_DETAIL="$(printf '%s says %s, and its own endpoint %s CONFIRMS a real prompt is outstanding (/question %s, /permission %s). A person has to answer that; no budget reaches idle without one.' \
        "$pane" "$claim" "$endpoint" "$qn" "$pn")"
      return 0
    fi
  fi

  screen="$(witness_screen "$pane" || true)"
  if [ -n "$screen" ]; then
    screen_state="${screen%% *}"; screen_hash="${screen##* }"
  else
    screen_state=unreadable; screen_hash=""
  fi

  if [ "$screen_state" = moved ]; then
    WITNESS_VERDICT=live
    WITNESS_DETAIL="$(printf '%s says %s, but its own visible screen CHANGED between samples %ss apart (pane.read source=visible), so that pane is doing work whatever the status field says. This is a real wait and it keeps its full budget.' \
      "$pane" "$claim" "$WITNESS_INTERVAL_S")"
    return 0
  fi

  # From here the screen is static or unreadable, and no prompt was positively
  # read. Every remaining branch turns on WHETHER WE COULD ASK, never on a guess
  # about what the answer would have been.
  if [ "$screen_state" = unreadable ]; then
    WITNESS_VERDICT=unknown
    WITNESS_DETAIL="$(printf '%s says %s, and the cross-check could NOT be made: its visible screen could not be read (pane.read source=visible failed or returned no text field). That is not evidence the pane is live and not evidence it is stale, so the gate keeps its normal budget.' \
      "$pane" "$claim")"
    return 0
  fi
  if [ -z "$endpoint" ]; then
    WITNESS_VERDICT=unknown
    WITNESS_DETAIL="$(printf '%s says %s and its visible screen was STATIC across %s samples %ss apart (hash %s) — but it exposes no OpenCode HTTP endpoint (not opencode, or no --port in its argv), so whether a prompt is outstanding could not be established. A static screen alone does not distinguish a stale state from a pane holding a modal this check cannot see, so the gate keeps its normal budget.' \
      "$pane" "$claim" "$WITNESS_SAMPLES" "$WITNESS_INTERVAL_S" "$screen_hash")"
    return 0
  fi
  if [ -z "$prompts" ]; then
    WITNESS_VERDICT=unknown
    WITNESS_DETAIL="$(printf '%s says %s and its visible screen was STATIC across %s samples %ss apart (hash %s) — but its endpoint %s did not answer /question and /permission with parseable arrays, so an outstanding prompt could not be ruled out. An authority that cannot be asked is not an authority that said no; the gate keeps its normal budget.' \
      "$pane" "$claim" "$WITNESS_SAMPLES" "$WITNESS_INTERVAL_S" "$screen_hash" "$endpoint")"
    return 0
  fi

  WITNESS_VERDICT=settled
  WITNESS_DETAIL="$(printf '%s says %s, and every independent check CONTRADICTS it: its visible screen did not change across %s samples %ss apart (hash %s, pane.read source=visible), and its own endpoint %s reports nothing outstanding (/question %s, /permission %s). Nothing is moving and there is no prompt for anyone to answer, so that state cannot change on its own and no budget reaches idle from here.' \
    "$pane" "$claim" "$WITNESS_SAMPLES" "$WITNESS_INTERVAL_S" "$screen_hash" "$endpoint" "$qn" "$pn")"
  return 0
}

# ── the gate ─────────────────────────────────────────────────────────────────
#
# witness_wait <pane> <total_ms>
#
#   0 = the pane reached idle,done — a prompt will actually be read
#   2 = the budget was spent and it never got there. The old outcome, kept: an
#       `unknown` cross-check must cost a caller nothing it did not already pay.
#   5 = the wait was CUT SHORT because the state was proved unable to change.
#   6 = the wait was CUT SHORT because a PERSON is being waited on: the pane's
#       own endpoint confirmed an outstanding /question or /permission. Distinct
#       from 5 because the advice is not the same. 5 says look at that pane and
#       decide what it needs; 6 says the pane already told you what it needs and
#       it is not something any sender can supply.
#
# A REAL WAIT IS STILL A WAIT. This does not shorten anything on the normal
# path: the underlying level-triggered `wait-agent` returns in ~0.14s on an idle
# pane, and a pane whose screen is moving is re-witnessed as `live` every slice
# and keeps every millisecond of its budget. What it removes is only the case
# where a gate sits on a state that has been proved incapable of changing.
#
# WHY TWO CONFIRMATIONS, AND WHY SLICES RATHER THAN ONE UP-FRONT CHECK.
# A single witness is a 4-second window. A genuinely working pane can be silent
# for 4 seconds — a long tool call between two writes to the screen — and
# abandoning it on that would be the same class of error as the bug, pointed the
# other way. Requiring the verdict twice, with a full wait slice in between,
# means a pane is only given up on after being static AND prompt-free for the
# whole span (~50s at the defaults). That is a bounded, honest cost against a
# 900s budget spent on nothing.
: "${WITNESS_SLICE_MS:=20000}"
: "${WITNESS_CONFIRMATIONS:=2}"

# WITNESS_ON_VERDICT — the name of a caller function, or empty.
#
# A LONG WAIT THAT PRINTS NOTHING IS INDISTINGUISHABLE FROM A HANG, and that is
# not a cosmetic complaint: the whole reason a brainer reached for a raw
# `herdr agent wait` with a 1800000ms budget and then sat there is that the
# alternative also looked like nothing happening. A gate may now ask to be told
# what each round decided, so it can say "still live, 340s left" instead of
# leaving a person to guess whether the process is alive.
#
# It is a CALLBACK RATHER THAN AN UNCONDITIONAL `printf` on purpose. Every
# existing caller of witness_wait — channel-send's two gates, hw next, hw
# unstick's pre-checks — is either a gate whose output contract is already
# fixed or a library used inside `$(...)`, where a stray line on stdout is not
# a progress report, it is a corrupted return value. Default empty means every
# one of them behaves exactly as it did.
#
# Called as: "$WITNESS_ON_VERDICT" <verdict> <detail> <elapsed_s> <remaining_s>
# Its exit status is IGNORED — a progress printer that fails must never be able
# to end a wait it was only observing.
: "${WITNESS_ON_VERDICT:=}"

witness_notify() {
  [ -n "${WITNESS_ON_VERDICT:-}" ] || return 0
  command -v "$WITNESS_ON_VERDICT" >/dev/null 2>&1 || return 0
  "$WITNESS_ON_VERDICT" "$1" "$2" "$3" "$4" || true
  return 0
}

# witness_rpc_wait <pane> <ms>
#
# One `wait-agent`, run so that A SIGNAL REACHES THE CALLER WHILE IT IS WAITING.
#
# It used to be a plain foreground call, and that is what made every gate in
# this toolchain unkillable. Bash does not run a trap while a foreground child
# is running: it records the signal and handles it after the child returns. With
# an executor's 540s budget that means a SIGTERM is ignored for up to nine
# minutes — measured 2026-08-27 against a real parked executor: SIGTERM at t=8s,
# the process still waiting at t=11s, and when the target finally reached idle
# the script ran ON PAST its own handler and DELIVERED THE MESSAGE, 26 seconds
# after the caller had killed it, printing nothing at all. From the caller's
# side that is a hang, then a `Terminated: 15` with no diagnosis; from the
# receiver's side it is a message that arrived. Both halves of that pair are
# wrong, and both come from this one line.
#
# Backgrounded plus `wait`: the `wait` BUILTIN is interruptible, so a trapped
# signal runs its handler at once. The child is killed on the way out, because a
# handler that exits without it would leave an orphaned rpc holding the socket —
# which is the shape of orphan this file's callers already have to detect.
witness_rpc_wait() {
  local pane="$1" ms="$2" child rc=0
  "$WITNESS_RPC" wait-agent "$pane" idle,done --timeout-ms "$ms" >/dev/null 2>&1 &
  child=$!
  WITNESS_RPC_CHILD="$child"
  wait "$child" || rc=$?
  WITNESS_RPC_CHILD=""
  return "$rc"
}

# Set while witness_rpc_wait has a child in flight, so a caller's signal handler
# can take it down with it. Empty at every other moment.
WITNESS_RPC_CHILD=""
witness_kill_child() {
  [ -n "${WITNESS_RPC_CHILD:-}" ] || return 0
  kill "$WITNESS_RPC_CHILD" 2>/dev/null || true
  WITNESS_RPC_CHILD=""
  return 0
}

witness_wait() {
  local pane="$1" total_ms="$2" deadline slice_ms now remaining_ms rc
  local settled_streak=0 human_streak=0

  WITNESS_VERDICT=""
  WITNESS_DETAIL=""

  # A budget smaller than one slice is a caller that has already decided it will
  # not wait long. Do not spend witness time on top of it.
  if [ "$total_ms" -le "$WITNESS_SLICE_MS" ]; then
    rc=0
    witness_rpc_wait "$pane" "$total_ms" || rc=$?
    return "$rc"
  fi

  deadline=$(( $(date +%s) + total_ms / 1000 ))
  while :; do
    now="$(date +%s)"
    remaining_ms=$(( (deadline - now) * 1000 ))
    [ "$remaining_ms" -gt 0 ] || return 2
    slice_ms="$WITNESS_SLICE_MS"
    [ "$slice_ms" -le "$remaining_ms" ] || slice_ms="$remaining_ms"

    rc=0
    witness_rpc_wait "$pane" "$slice_ms" || rc=$?

    # ONLY A TIMEOUT IS WORTH ANOTHER SLICE, and this is load-bearing rather
    # than tidy. herdr-rpc distinguishes its outcomes: 0 matched, 2 timed out,
    # 3 could not connect, 4 the RPC itself errored (the pane is gone). Looping
    # on 3 or 4 would spin as fast as the socket can refuse — burning the whole
    # budget in a hot loop on a dead multiplexer, and returning a code the
    # caller could no longer tell apart from a real timeout. Those two are
    # facts about the transport, not about the pane, and they are passed
    # straight back exactly as the raw wait used to.
    case "$rc" in
      0) return 0 ;;
      2) ;;
      *) return "$rc" ;;
    esac

    # Spend nothing on a witness whose answer can no longer change the outcome:
    # if the budget is gone, this is a plain timeout and must read as one.
    [ "$(date +%s)" -lt "$deadline" ] || return 2

    witness_state "$pane"
    witness_notify "$WITNESS_VERDICT" "$WITNESS_DETAIL" \
      "$(( $(date +%s) - (deadline - total_ms / 1000) ))" \
      "$(( deadline - $(date +%s) ))"
    case "$WITNESS_VERDICT" in
      settled)
        settled_streak=$((settled_streak + 1))
        human_streak=0
        [ "$settled_streak" -lt "$WITNESS_CONFIRMATIONS" ] || return 5
        ;;
      # SAME BAR AS `settled`, DELIBERATELY. A person can answer a prompt while
      # this is watching, and abandoning a wait on a single 4-second window
      # would be the mirror of the bug: giving up on a pane that was about to
      # move. Two confirmations with a full slice between them means the prompt
      # has been outstanding for the whole span (~50s at the defaults) before
      # anything is refused — and the refusal costs nothing, because nothing is
      # sent and the caller may retry the instant the person answers.
      awaiting-human)
        human_streak=$((human_streak + 1))
        settled_streak=0
        [ "$human_streak" -lt "$WITNESS_CONFIRMATIONS" ] || return 6
        ;;
      # live and unknown reset both. `unknown` resets for the same reason it
      # never triggers anything: it is not half a finding, it is the absence of
      # one.
      *) settled_streak=0; human_streak=0 ;;
    esac
  done
}

# ── ONE ANSWER TO "WHERE IS THIS EXECUTOR", FOR EVERY CALLER THAT GATES A RULING ─
#
# THE DEFECT, 2026-09-28, two lanes the same day. `hw ruling` said "w41:p127 is
# done, not working" about a pane herdr showed `working` with
# turn_state=ended_awaiting_child; a minute later the same command queued. On
# the setup lane `hw ruling` said idle while `channel-send` said working about
# one pane, and on a chained executor's task 2 it said "has already reported
# this task" — reading task 1's marker.
#
# Three readers, three samplings of `agent_status`, one of them keyed on the
# wrong task. `agent_status` is the field this file already calls a liar, and
# for a pane awaiting a background child it OSCILLATES by construction: every
# post from the child opens and closes a turn (measured 2026-09-09 on w84:p1,
# see channel-send `_awaiting_child_warning`). Two commands that each sample it
# once will disagree, and whichever refuses names the other as the way out.
#
# So there is one classifier, and both `hw ruling` and `channel-send --ruling`
# call it. It reads ONE `herdr agent get` and the run's disk markers, and
# prints one line separated by \037 (unit separator) — NOT tabs: `read` with a
# whitespace IFS collapses empty fields, and turn_state is often empty:
#
#   <verdict> <agent_status> <turn_state> <children_running> <run_dir> <task> <cwd>
#
# The verdicts, in the order they are decided — the first that holds wins:
#
#   absent          herdr knows no such pane.
#   reporting       the CURRENT task's done-invoker is running right now (its
#                   `reporting` marker names a live process).
#   reported        the CURRENT task has a done marker and no `reopened`.
#   awaiting_child  the pane's own Stop hook published ended_awaiting_child
#                   with a child named running. `agent_status` is NOT read for
#                   this: it flaps here, and the next turn end is guaranteed —
#                   the child's completion re-invokes the executor.
#   working         anything else that is not idle/done.
#   idle            agent_status idle or done.
#
# "The CURRENT task" is `<run>/task` — task 1 is the run dir, task N is t<N>/ —
# the same rule as bin/hw `_run_done_marker` and invoker-common
# `invoker_state_dir`. A pane with no hw run directory skips the three disk
# verdicts and is classified on herdr alone, which is what both callers did
# before this existed.
#
# Exit 0 always; the verdict is the answer.
witness_run_dir() {  # <cwd> [<run id from the pane's hw_run token>]
  local d="$1" run="${2:-}" hops=0 newest
  while [ "$hops" -lt 12 ] && [ -n "$d" ] && [ "$d" != / ]; do
    if [ -d "$d/.hw" ]; then
      # THE PANE'S OWN RUN FIRST. A --here work dir can hold several runs, and
      # "newest" is a guess about which one this pane is; its token is not.
      if [ -n "$run" ] && [ -d "$d/.hw/$run" ]; then printf '%s' "$d/.hw/$run"; return 0; fi
      newest="$(fd -H -t d -d 1 '^2[0-9]{7}-[0-9]{6}-[0-9]+$' "$d/.hw" 2>/dev/null | sort -r | head -1 || true)"
      if [ -n "$newest" ]; then printf '%s' "${newest%/}"; return 0; fi
    fi
    d="$(dirname "$d")"
    hops=$((hops + 1))
  done
  return 1
}

witness_task_dir() {  # <run_dir> — the current task's state dir
  local n=""
  [ -r "$1/task" ] && n="$(tr -dc '0-9' < "$1/task" 2>/dev/null || true)"
  case "$n" in ''|0|1) printf '%s' "$1" ;; *) printf '%s/t%s' "$1" "$n" ;; esac
}

# A `reporting` marker is `<pid>\n<ps lstart>` written by done-invoker. It is
# live only while that exact process is: a crashed done-invoker leaves a file,
# and a file alone must not refuse corrections forever.
witness_reporting_live() {  # <marker>
  local pid start now
  [ -r "$1" ] || return 1
  pid="$(sed -n 1p "$1" 2>/dev/null || true)"
  start="$(sed -n 2p "$1" 2>/dev/null || true)"
  case "$pid" in ''|*[!0-9]*) return 1 ;; esac
  kill -0 "$pid" 2>/dev/null || return 1
  now="$(ps -o lstart= -p "$pid" 2>/dev/null | tr -s ' ' | sed 's/^ *//; s/ *$//' || true)"
  [ "$now" = "$start" ]
}

witness_executor_state() {  # <pane>
  local pane="$1" info status state children cwd run rundir="" taskdir="" seq=1 verdict
  info="$(herdr agent get "$pane" 2>/dev/null || true)"
  status="$(printf '%s' "$info" | jq -r '.result.agent.agent_status // empty' 2>/dev/null || true)"
  if [ -z "$info" ] || [ -z "$status" ]; then
    printf 'absent\037\037\037\037\037\037\n'; return 0
  fi
  state="$(printf '%s' "$info" | jq -r '.result.agent.tokens.turn_state // empty' 2>/dev/null || true)"
  children="$(printf '%s' "$info" | jq -r '.result.agent.tokens.children_running // empty' 2>/dev/null || true)"
  cwd="$(printf '%s' "$info" | jq -r '.result.agent.cwd // empty' 2>/dev/null || true)"
  run="$(printf '%s' "$info" | jq -r '.result.agent.tokens.hw_run // empty' 2>/dev/null || true)"
  [ -z "$cwd" ] || rundir="$(witness_run_dir "$cwd" "$run" || true)"
  if [ -n "$rundir" ]; then
    taskdir="$(witness_task_dir "$rundir")"
    case "$taskdir" in "$rundir"/t*) seq="${taskdir##*/t}" ;; esac
  fi
  if [ -n "$taskdir" ] && witness_reporting_live "$taskdir/reporting"; then
    verdict=reporting
  elif [ -n "$taskdir" ] && [ -f "$taskdir/done" ] && [ ! -f "$taskdir/reopened" ]; then
    verdict=reported
  elif [ "$state" = ended_awaiting_child ] && [ -n "$children" ]; then
    verdict=awaiting_child
  else
    case "$status" in idle|done) verdict=idle ;; *) verdict=working ;; esac
  fi
  printf '%s\037%s\037%s\037%s\037%s\037%s\037%s\n' "$verdict" "$status" "$state" "$children" "$rundir" "$seq" "$cwd"
}

# ── THE RULING QUEUE ─────────────────────────────────────────────────────────
#
# `hw ruling` used to hold ONE correction and refuse a second while the first
# was undelivered, because two corrections merged into one blob lose which came
# first, and an overwrite loses one of them. MEASURED 2026-09-28/29, a product lane's
# release: with phases in parallel the brainer had to chain background waits to
# send migration renumberings one after another.
#
# The guarantee was ORDER and IDENTITY, not "one", so the queue keeps both:
# every ruling is its own file, `pending-ruling.<n>`, with <n> taken by a
# no-clobber `ln` so two concurrent senders cannot share a number; delivery is
# oldest first, each one labelled with its place; and each gets its own record,
# `ruling-delivered.<n>`. A <n> is never reused while its claim or its record
# is on disk, so a record always names one ruling.
#
# `pending-ruling` with no suffix is still read, as the oldest entry: it is
# what an hw from before the queue writes into a run it launched.
#
# THE CAP. A queue against an executor that never reaches a turn end grows
# without anyone noticing, and a brainer that has sent five unseen corrections
# is rewriting the brief, not correcting it. Only rulings AHEAD of the new one
# count, and they can only leave by being delivered, so two concurrent senders
# at the edge can both be refused but can never both slip past it.
: "${RULING_QUEUE_CAP:=5}"

ruling_queue_pending() {  # <run_dir> — pending rulings, oldest first, one path per line
  local d="$1" f n
  [ -f "$d/pending-ruling" ] && printf '%s\n' "$d/pending-ruling"
  for f in "$d"/pending-ruling.*; do
    [ -f "$f" ] || continue
    n="${f##*/pending-ruling.}"
    case "$n" in ''|*[!0-9]*) continue ;; esac
    printf '%s\t%s\n' "$((10#$n))" "$f"
  done | sort -n -k1,1 | cut -f2-
  return 0
}

ruling_queue_next_seq() {  # <run_dir>
  local d="$1" f n max=0
  for f in "$d"/pending-ruling.* "$d"/.pending-ruling.claimed.* "$d"/ruling-delivered.*; do
    [ -e "$f" ] || continue
    case "$f" in
      */.pending-ruling.claimed.*) n="${f##*/.pending-ruling.claimed.}"; n="${n%%.*}" ;;
      *) n="${f##*.}" ;;
    esac
    case "$n" in ''|*[!0-9]*) continue ;; esac
    [ "$((10#$n))" -le "$max" ] || max="$((10#$n))"
  done
  printf '%s' "$((max + 1))"
}

# Queue <tmp> (a file beside the queue) as the newest ruling, consuming it.
# Prints "<path>\t<position>". Returns 3 when the cap refuses it (nothing is
# queued) and 1 when no name could be written.
ruling_queue_add() {  # <run_dir> <tmp>
  local d="$1" tmp="$2" n tries=0 ahead=0 f m
  n="$(ruling_queue_next_seq "$d")"
  until ln "$tmp" "$d/pending-ruling.$n" 2>/dev/null; do
    n=$((n + 1)); tries=$((tries + 1))
    [ "$tries" -lt 50 ] || { rm -f "$tmp"; return 1; }
  done
  rm -f "$tmp"
  while IFS= read -r f; do
    m="${f##*/pending-ruling}"; m="${m#.}"
    if [ -z "$m" ] || [ "$((10#$m))" -lt "$n" ]; then ahead=$((ahead + 1)); fi
  done < <(ruling_queue_pending "$d")
  if [ "$ahead" -ge "$RULING_QUEUE_CAP" ] \
     && mv "$d/pending-ruling.$n" "$d/.pending-ruling.retracted.$n.$$" 2>/dev/null; then
    rm -f "$d/.pending-ruling.retracted.$n.$$"
    return 3
  fi
  printf '%s\t%s' "$d/pending-ruling.$n" "$((ahead + 1))"
}

# Claim every pending ruling, oldest first, each with `mv` (done-invoker, a
# turn end and a sender's retraction race for the same names; exactly one `mv`
# wins each). Consumed BEFORE it is handed over: a crash loses a ruling rather
# than repeating it. Prints the delivery; returns 1 when there was none.
ruling_queue_claim_all() {  # <run_dir> <via>
  local d="$1" via="$2" f n claim text at count=0 first="" body=""
  at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  while IFS= read -r f; do
    n="${f##*/pending-ruling}"; n="${n#.}"; [ -n "$n" ] || n=legacy
    claim="$d/.pending-ruling.claimed.$n.$$"
    mv "$f" "$claim" 2>/dev/null || continue
    text="$(cat "$claim" 2>/dev/null || true)"
    if [ -n "$text" ]; then
      count=$((count + 1))
      [ "$n" = legacy ] || printf 'at=%s\nvia=%s\n' "$at" "$via" > "$d/ruling-delivered.$n" 2>/dev/null || true
      transcript_append "$d" "RULING-DELIVERED" "ruling:$n via=$via" "$text"
      [ "$count" -gt 1 ] || first="$text"
      body="${body}── ruling ${count} ──
$text

"
    fi
    rm -f "$claim" 2>/dev/null || true
  done < <(ruling_queue_pending "$d")
  [ "$count" -gt 0 ] || return 1
  printf 'at=%s\nvia=%s\ncount=%s\n' "$at" "$via" "$count" > "$d/ruling-delivered" 2>/dev/null || true
  if [ "$count" = 1 ]; then
    printf '%s' "$first"
  else
    printf '%s RULINGS, in the order your brainer sent them. Where a later one disagrees with an earlier one, the later one stands.\n\n%s' "$count" "${body%$'\n\n'}"
  fi
}

# ── THE TASK TRANSCRIPT ──────────────────────────────────────────────────────
#
# `<rundir>/transcript.log`: what was SAID between an executor and its brainer,
# append-only, with a timestamp and the envelope it travelled in. Until this
# existed the text of an ask, a ruling or a report lived in two panes' scrollback
# and in tokens that expire after an hour, so "what did the brainer tell it?" had
# no answer once either pane was gone. `hw log <lane> <task>` reads it.
#
# BEST EFFORT, ALWAYS. A log line lost must never cost a delivery, so this
# returns 0 whatever happens. A task past the first keeps its state in `t<N>/`;
# the log stays in the run directory itself, one file for the whole executor.
transcript_append() {  # <state-or-run dir> <kind> <envelope> <text>
  local d="${1:-}" kind="${2:-}" envelope="${3:-}" text="${4:-}" base body
  [ -n "$d" ] || return 0
  base="${d##*/}"
  case "$base" in t[0-9]*) case "${base#t}" in *[!0-9]*) ;; *) d="${d%/*}" ;; esac ;; esac
  [ -d "$d" ] || return 0
  body="$(printf '%s\n' "$text" | sed 's/^/    | /')"
  printf '%s  %s  envelope=%s\n%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$kind" "${envelope:--}" "$body" \
    >> "$d/transcript.log" 2>/dev/null || true
  return 0
}
