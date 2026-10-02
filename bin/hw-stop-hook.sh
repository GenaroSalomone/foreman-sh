#!/usr/bin/env bash
#
# hw-stop-hook.sh — a Claude Code/Codex `Stop` hook that makes the harness carry
# turn-end facts an agent keeps forgetting to act on.
#
#   ~/.claude/settings.json  → hooks.Stop → bash '<this file>' stop
#   ~/.codex/hooks.json      → hooks.Stop → bash '<this file>' stop
#
# WHY THIS EXISTS
#
# `done-invoker` is opt-in: the agent has to choose to call it. Twice now an
# executor has finished its work, saved its findings to engram, and never
# called it — the pane stayed alive and idle while the brainer, which cannot
# poll, went on believing the task was running. The instruction side of this is
# already as loud as prose gets (CLAUDE.shared.md, "Closing is not optional").
# Prose was not enough. So the signal now comes from the harness, which fires
# whether or not the agent remembered anything.
#
# WHAT THIS HOOK HONESTLY CLAIMS — read this before extending it
#
# A `Stop` hook fires at the end of a TURN. It cannot tell "the task is
# finished" from "this turn is finished": an executor that has just asked the
# brainer a question, or that stopped to think, or that is one turn into a
# ten-turn job, ends a turn exactly like one that has quietly finished. So this
# hook does NOT write a done marker, does NOT prompt the brainer, and does not
# claim completion anywhere.
#
# What it establishes, and this is all it establishes:
#
#     this executor's turn ended at time T, and it has still not reported done
#
# That is precisely the signal that was missing. "Idle and silent" is a state a
# human or a brainer can act on — it is not "failed", and `hw status` words it
# that way on purpose.
#
# WHERE THE SIGNAL GOES
#
# Into `pane.report_metadata` tokens on the executor's own pane, through
# invoker-common.sh's `invoker_publish` — the same publishing path the invokers
# use, not a second copy of it. Tokens survive without a keystroke and are
# readable with `herdr agent list`; `hw status` reads them from there.
#
# THE TOKEN BUDGET IS THE REASON THIS IS A SEPARATE PUBLISH. herdr caps tokens
# at 16 keys PER REQUEST and merges into whatever the pane already has.
# ask-invoker's publish is 15/16 and done-invoker's is 16/16 —
# both exactly full, so a fixed key cannot be added inside either. A small
# publish of our own merges alongside them and takes nothing away. The three
# keys written here (`turn_state`, `turn_ended_at`, `turns`) appear in neither
# invoker's key set, so no publish deletes another's keys.
#
# GUARDS. Every wired Claude Code/Codex session on this machine runs this file.
# It has two narrow jobs: publish an unreported executor turn, and refuse an
# explicit handback from a brainer. Everything else is inert.
#
# IT MUST NEVER BREAK A SESSION: `set -e` is deliberately NOT set and an EXIT
# trap forces status 0 no matter what happens below, herdr being absent
# included.
#
set -uo pipefail
# NATIVE WINDOWS (Git Bash): bin/msys-compat.sh makes the native programs this
# file runs (Python, jq, fd, git) answer in bash's path spelling and with LF,
# and asks msys for real symlinks. Elsewhere OSTYPE never matches. A copy of this
# file with no msys-compat.sh beside it (a test fixture) runs on the layer its parent exported.
case "${OSTYPE:-}" in msys*|cygwin*) HW_MSYS_COMPAT="$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/msys-compat.sh"; [ ! -f "$HW_MSYS_COMPAT" ] || . "$HW_MSYS_COMPAT" ;; esac
trap 'exit 0' EXIT

# ── guard 1: the action this file was wired for ────────────────────────────
# Mirrors ~/.claude/hooks/herdr-agent-state.sh, which takes its event as argv[1]
# rather than trusting the payload. Anything else is not ours.
case "${1:-}" in
  stop) ;;
  *) exit 0 ;;
esac

# ── guard 2: are we inside a herdr pane at all ─────────────────────────────
[ "${HERDR_ENV:-}" = "1" ]          || exit 0
[ -n "${HERDR_SOCKET_PATH:-}" ]     || exit 0
[ -n "${HERDR_PANE_ID:-}" ]         || exit 0

# ── guard 3: read the completed turn once ──────────────────────────────────
# Stop input carries `last_assistant_message`; unlike transcript_path, Claude's
# documented contract says it includes the just-finished response. Subagent
# events are ignored before either branch acts.
HOOK_INPUT="$(head -c 262144 2>/dev/null || true)"
case "$HOOK_INPUT" in
  *'"agent_id"'*) exit 0 ;;
esac

# ── a brainer's engram session stays selectable ────────────────────────────
# bin/brain registers `brainer-<label>` and hands the pane BRAIN_ENGRAM_SESSION;
# POST /sessions carries a 30-minute lease, after which engram stops selecting
# it and an unlabelled mem_save falls into manual-save-<label> (see bin/brain).
# Renewed here at every brainer turn end, only on a serve proven to be this
# machine's store. Executors renew through `hw executor-turn-end` instead.
# Silent and bounded: this hook must never break or slow a session noticeably.
if [ -z "${HW_RUN:-}" ] && [ -n "${BRAIN_ENGRAM_SESSION:-}" ] && [ -n "${ENGRAM_PROJECT:-}" ]; then
  _eb="${BASH_SOURCE[0]}"
  while [ -L "$_eb" ]; do
    _ed="$(cd -P "$(dirname "$_eb")" && pwd)" || break
    _eb="$(readlink "$_eb")" || break
    case "$_eb" in /*) ;; *) _eb="$_ed/$_eb" ;; esac
  done
  _eb="$(cd -P "$(dirname "$_eb")" 2>/dev/null && pwd)"
  # shellcheck source=engram-serve.sh
  # `[ -r ]` first: bash exits outright on a `.` of a missing file, even here.
  if [ -n "$_eb" ] && [ -r "$_eb/engram-serve.sh" ] && . "$_eb/engram-serve.sh" \
     && engram_serve_verdict >/dev/null 2>&1; then
    engram_session_post "$BRAIN_ENGRAM_SESSION" "$ENGRAM_PROJECT" "$PWD" >/dev/null 2>&1 || true
  fi
fi

# ── brainer handback refusal ───────────────────────────────────────────────
# A brainer has Herdr identity and the brain memory pin, but no hw run. Limit
# detection to the lanes' brainer directories: this must not turn a phrase in an
# ordinary terminal into control flow. The detector reads only the FINAL
# paragraph and only concrete handback forms. It deliberately does not guess
# whether a decision belongs to the operator.
if [ -z "${HW_RUN:-}" ] && [ "${ENGRAM_PROJECT:-}" = "brain" ]; then
  command -v python3 >/dev/null 2>&1 || exit 0
  # The brainer directories are brain/<lane> for every lane projects.json
  # knows, and brain is this script's own root (HW_BRAIN_ROOT overrides it, as
  # it does for bin/hw). A table that does not load disables this refusal, like
  # every other failure in this hook: it must never break a session.
  _src="${BASH_SOURCE[0]}"
  while [ -L "$_src" ]; do
    _dir="$(cd -P "$(dirname "$_src")" && pwd)" || exit 0
    _src="$(readlink "$_src")" || exit 0
    case "$_src" in /*) ;; *) _src="$_dir/$_src" ;; esac
  done
  _bin="$(cd -P "$(dirname "$_src")" && pwd)" || exit 0
  _brain="${HW_BRAIN_ROOT:-$(dirname "$_bin")}"
  # shellcheck source=project-spaces.sh
  . "$_bin/project-spaces.sh" 2>/dev/null || exit 0
  lane_config_load "$_brain" 2>/dev/null || exit 0
  [ -n "${HW_LANES:-}" ] || exit 0
  # The event goes through a file, not fd 3: a native Windows python3 (Git
  # Bash) inherits only 0-2, and open(3) failing reads as "not a handback".
  _hw_stop_event="$(mktemp "${TMPDIR:-/tmp}/hw-stop-event.XXXXXX")" || exit 0
  printf '%s' "$HOOK_INPUT" > "$_hw_stop_event"
  HW_STOP_EVENT="$_hw_stop_event" HW_STOP_BRAIN_ROOT="$_brain" HW_STOP_LANES="$HW_LANES" HW_STOP_OPERATOR="$HW_OPERATOR" \
  python3 - <<'PY' 2>/dev/null || true
import json
import os
import re
import sys

try:
    with open(os.environ["HW_STOP_EVENT"]) as hook_input:
        event = json.load(hook_input)
except Exception:
    raise SystemExit(0)

cwd = event.get("cwd", "")
brain_root = os.environ.get("HW_STOP_BRAIN_ROOT", "")
lanes = os.environ.get("HW_STOP_LANES", "").split()
def in_lane(root, path):
    return bool(re.fullmatch(
        re.escape(root) + r"/(?:" + "|".join(map(re.escape, lanes)) + r")(?:/.*)?", path))
# On native Windows the same directory reaches here spelled C:\..., /c/...,
# /tmp/... or through a resolved symlink, so it is also compared resolved.
# Elsewhere the spelling given is the one compared, as it always was.
if not brain_root or not lanes or not (in_lane(brain_root, cwd) or (
        os.name == "nt" and cwd and in_lane(
            os.path.realpath(brain_root).rstrip("/").casefold(),
            os.path.realpath(cwd).rstrip("/").casefold()))):
    raise SystemExit(0)

message = event.get("last_assistant_message")
if not isinstance(message, str) or not message.strip():
    raise SystemExit(0)

paragraphs = [part.strip() for part in re.split(r"\n\s*\n", message) if part.strip()]
if not paragraphs:
    raise SystemExit(0)
final = paragraphs[-1]

# This exact declaration is the safe exit for a genuinely human decision. It
# forces the response to name the boundary rather than merely re-label work as
# the user's. The shared rule defines the accepted boundaries.
if re.search(r"(?im)^human boundary:\s+\S", final):
    raise SystemExit(0)

# The other legal exit: a PROPOSAL of work the operator did not ask for. Launching it
# is what the brainer rule forbids (brain/CLAUDE.md, "Decide, then act"), so a
# turn that proposes and waits is obeying the rule, not handing work back.
# Before 2026-09-28 the only exit was "Human boundary:", which forced a brainer
# holding an unrequested fix to either launch it or invent a boundary. Same
# shape as the line above: it must name what is proposed.
if re.search(r"(?im)^(?:not requested|no pedido):\s+\S", final):
    raise SystemExit(0)

# Ignore examples and quoted material. Detection is about what the brainer is
# handing back, not text it is reviewing.
candidate = re.sub(r"```.*?```", " ", final, flags=re.S)
candidate = re.sub(r"`[^`]*`", " ", candidate)
candidate = re.sub(r'"[^"\n]*"', " ", candidate)
candidate = " ".join(candidate.casefold().split())

patterns = (
    r"\bdecime si (?:querés|quieres) que\b",
    r"\bavisame si (?:querés|quieres) que\b",
    r"\b(?:querés|quieres) que\b",
    r"\bsigue(?:n)? siendo (?:tuyo|tuya|tuyos|tuyas|suyo|suya|suyos|suyas)\b",
    r"\bnecesita(?:n)? (?:un |una )?dueñ[oa]\b",
    r"\blet me know if you want me to\b",
    r"\btell me if you want me to\b",
    r"\bdo you want me to\b",
    r"\bwould you like me to\b",
    r"\bshould i\b",
    r"\bremains? yours\b",
    r"\bneeds? an? owner\b",
)
if not any(re.search(pattern, candidate) for pattern in patterns):
    raise SystemExit(0)

operator = os.environ.get("HW_STOP_OPERATOR") or "the operator"
reason = (
    "A mechanical handback signature appeared in your final paragraph. "
    f"Do not ask {operator} to choose or own reversible, scoped work: decide and act now. "
    "If action crosses a genuine human boundary (money, a third party, an irreversible "
    "or outward action, or conflicting priorities), do not act; end with "
    f"'Human boundary: <specific boundary and why only {operator} can decide>.' "
    f"If the work is something {operator} did not ask for, do not launch it either: "
    "propose it and end with 'Not requested: <what you propose, and that it was not asked for>.'"
)
json.dump({"decision": "block", "reason": reason}, sys.stdout, separators=(",", ":"))
sys.stdout.write("\n")
PY
  rm -f "$_hw_stop_event"
  exit 0
fi

# ── guard 4: are we an hw EXECUTOR ─────────────────────────────────────────
# HW_RUN is what makes an executor run addressable — the ask cap and the done
# marker both hang off it, not off the directory.
[ -n "${HW_TASK:-}" ]    || exit 0
[ -n "${HW_RUN:-}" ]     || exit 0
[ -n "${HW_WORKDIR:-}" ] || exit 0

# ── guard 5: has this run's CURRENT task already reported ──────────────────
# The same marker done-invoker writes, in the same place: task 1 is the run
# dir, task N (after `hw next`) is t<N>/. Keyed on task 1's marker alone, this
# exited on EVERY turn end of a chained task 2+, so executor-turn-end never ran
# there: no handback judgement and no delivery of a queued `hw ruling`.
# MEASURED 2026-09-28 on probe w80:p1, task 2 — see setup/decisions.md.
STATE_DIR="$HW_WORKDIR/.hw/$HW_RUN"
_task_seq=""
[ -r "$STATE_DIR/task" ] && _task_seq="$(tr -dc '0-9' < "$STATE_DIR/task" 2>/dev/null || true)"
case "$_task_seq" in ''|0|1) _task_done="$STATE_DIR/done" ;; *) _task_done="$STATE_DIR/t$_task_seq/done" ;; esac
[ -f "$_task_done" ] && exit 0  # MUTATION-ANCHOR: 203-M03

command -v python3 >/dev/null 2>&1 || exit 0
command -v jq      >/dev/null 2>&1 || exit 0

# ── guard 6: is THIS session the executor hw registered for the run ────────
# HW_RUN is inherited by every descendant, so it identifies a RUN, not a
# session. MEASURED 2026-09-22: a `claude -p` an executor started as a helper
# ended its own turn, found HW_RUN in its environment, was told to run
# done-invoker, and closed the PARENT's task with it. The session hw recorded
# at launch (`session_id` in the run's receipt, read back from herdr's
# agent_session) is the one executor this hook speaks to; a payload naming any
# other session is a child, and a child's turn is not the executor's.
#
# ONLY A REGISTERED SESSION NARROWS IT. A run whose receipt holds no session id
# (herdr published none in time, recorded as `none — …`) keeps the old
# behaviour: silencing the closing for every such run would trade a rare
# misfire for a common silent stop, the failure this hook exists to prevent.
REGISTERED_SID="$(jq -r 'select(.key=="session_id") | .value' \
  "$STATE_DIR/receipt.jsonl" 2>/dev/null | tail -1)"
PAYLOAD_SID="$(printf '%s' "$HOOK_INPUT" | jq -r '.session_id // empty' 2>/dev/null)"
case "$REGISTERED_SID" in
  ""|none*) ;;
  *) [ -n "$PAYLOAD_SID" ] && [ "$PAYLOAD_SID" != "$REGISTERED_SID" ] && exit 0 ;;
esac

# ── the vendor-neutral turn-end decision ───────────────────────────────────
# Same symlink-safe resolution as the invokers. The predicate lives in hw so
# Claude, Codex and OpenCode classify the same disk facts. This hook can bind
# the returned JSON as a native Stop refusal; OpenCode cannot, so hw wait binds
# the same recorded verdict at the control-plane boundary.
_src="${BASH_SOURCE[0]}"
while [ -L "$_src" ]; do
  _dir="$(cd -P "$(dirname "$_src")" && pwd)" || exit 0
  _src="$(readlink "$_src")" || exit 0
  case "$_src" in /*) ;; *) _src="$_dir/$_src" ;; esac
done
INVOKER_BIN_DIR="$(cd -P "$(dirname "$_src")" && pwd)" || exit 0
[ -x "$INVOKER_BIN_DIR/hw" ] || exit 0
# The vendor payload names the background children still running, and that is
# the one legal turn end no disk fact carries. hw does the classifying, so
# Claude, Codex and OpenCode cannot disagree about it; a vendor that sends no
# payload simply has no fourth case, exactly as before.
HW_TURN_END_PAYLOAD="$HOOK_INPUT" "$INVOKER_BIN_DIR/hw" executor-turn-end --hook-json 2>/dev/null || true # MUTATION-ANCHOR: 43-M06

exit 0
