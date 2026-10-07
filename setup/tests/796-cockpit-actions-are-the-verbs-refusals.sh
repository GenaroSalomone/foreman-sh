#!/usr/bin/env bash
# The cockpit's actions are the verbs' own refusals
#
# `hw ruling`, `hw done` and `hw receipt` refuse through bin/hw-actions, the
# same module the cockpit state writer imports to build each row's `actions`.
# Claims, per refusal class, against the SAME stubbed state:
#
#   · OLD/NEW: the verb as it was at e3d56b00 (before the extraction) and the
#     verb now print the same refusal and exit with the same code — ruling:
#     absent, no cwd, no run dir, reported, reporting, idle, working,
#     awaiting_child, the blocked-waiting resume; done: not reported, stranded,
#     working, turns after the report, and the allowed close; receipt: no
#     runs, no receipt, allowed;
#   · actions_for_row on the facts read from that same state says false exactly
#     where the verb refused, with the verb's own text as why_not, and true
#     where it did not;
#   · mutants of hw-actions (an idle ruling accepted, one done reason's text
#     altered, a receipt refusal dropped) turn the comparison red.
#
# Run alone while working on this subject:
#     bash setup/tests/796-cockpit-actions-are-the-verbs-refusals.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

[ -r "$ROOT/bin/hw-actions" ] || { pass "796: this tree carries no hw-actions"; exit 0; }
OLD_REV=e3d56b00
git -C "$ROOT" cat-file -e "$OLD_REV:bin/hw" 2>/dev/null \
  || { pass "796: $OLD_REV is not in this history, so there is no old verb to compare against"; exit 0; }

export WORK="$TMP/work"
RUN=20260101-000000-3

mkstub() {  # $1=bindir
  cat > "$1/herdr" <<'STUB'
case "$1 ${2:-}" in
  "agent get"*)
    [ -z "${STUB_ABSENT:-}" ] || exit 0
    printf '{"result":{"agent":{"pane_id":"pIU","cwd":"%s","agent_status":"%s","focused":false,"agent":"claude","tokens":%s}}}\n' \
      "${STUB_CWD-/x}" "${STUB_STATUS:-idle}" "${STUB_TOKENS:-null}" ;;
  "agent list") printf '{"result":{"agents":[]}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
  chmod +x "$1/herdr"
}
mkbin() {  # $1=dir $2=old|new
  mkdir -p "$1"; cp -R "$ROOT/bin/." "$1/"; mkstub "$1"
  [ "$2" = new ] || git -C "$ROOT" show "$OLD_REV:bin/hw" > "$1/hw"
  chmod +x "$1/hw"
}
OLDBIN="$TMP/old"; NEWBIN="$TMP/new"
mkbin "$OLDBIN" old; mkbin "$NEWBIN" new

# A run dir the way hw writes one. $1=task $2=flags: any of done blocked reporting
# $3=post-report turns. The pane receipt is always there.
mkrun() {
  local task="$1" flags="${2:-}" post="${3:-}" dir
  dir="$WORK/setup/$task/.hw/$RUN"
  rm -rf "$WORK/setup/$task"; mkdir -p "$dir"
  printf '{"key": "pane", "value": "pIU"}\n' > "$dir/receipt.jsonl"
  case " $flags " in *" done "*) : > "$dir/done" ;; esac
  case " $flags " in *" blocked "*) : > "$dir/blocked-waiting" ;; esac
  case " $flags " in *" reporting "*)
    printf '%s\n%s\n' "$LIVE_PID" "$(ps -o lstart= -p "$LIVE_PID" | tr -s ' ' | sed 's/^ *//; s/ *$//')" > "$dir/reporting" ;; esac
  [ -z "$post" ] || printf '1 %s\n' "$post" > "$dir/turns-post-report"
}
sleep 600 & LIVE_PID=$!
trap 'kill "$LIVE_PID" 2>/dev/null || true' EXIT

strip() { sed 's/\x1b\[[0-9;]*m//g'; }

# ── the cases: name | verb | task | status | tokens | run flags | post | extra env ──
# Each is run on OLD and NEW; mkrun is redone before each run (hw done closes).
CASES=(
  "ruling-absent|ruling|r-absent|idle|null|done||STUB_ABSENT=1"
  "ruling-no-cwd|ruling|r-nocwd|idle|null|||STUB_CWD="
  "ruling-no-rundir|ruling|r-norun|idle|null|||STUB_CWD=/nonexistent-796"
  "ruling-reported|ruling|r-reported|idle|null|done||"
  "ruling-reporting|ruling|r-reporting|working|null|reporting||"
  "ruling-idle|ruling|r-idle|idle|null|||"
  "ruling-idle-holding|ruling|r-hold|idle|{\"turn_state\":\"ended_holding\"}|||"
  "ruling-working|ruling|r-working|working|null|||"
  "ruling-awaiting-child|ruling|r-await|idle|{\"turn_state\":\"ended_awaiting_child\",\"children_running\":\"1 shell\"}|||"
  "ruling-blocked-waiting|ruling|r-blocked|idle|null|done blocked||"
  "done-not-reported|done|d-unreported|idle|null|||"
  "done-stranded|done|d-stranded|idle|{\"done_status\":\"done\",\"done_state\":\"undelivered\"}|||"
  "done-working|done|d-working|working|null|done||"
  "done-working-unreported|done|d-workun|working|null|||"
  "done-post-turns|done|d-post|idle|null|done|3|"
  "done-allowed|done|d-ok|idle|null|done||"
  "done-token-delivered|done|d-tok|idle|{\"done_status\":\"done\",\"done_state\":\"delivered\"}|||"
  "receipt-no-hw|receipt|c-nohw|idle|null|||NORUNS=1"
  "receipt-no-runs|receipt|c-noruns|idle|null|||EMPTYHW=1"
  "receipt-no-receipt|receipt|c-noreceipt|idle|null|||NORECEIPT=1"
  "receipt-allowed|receipt|c-ok|idle|null|done||"
)

setup_case() {  # $1=verb $2=task $3=flags $4=post $5=extra
  mkrun "$2" "$3" "$4"
  case "$5" in
    NORUNS=1) rm -rf "$WORK/setup/$2/.hw" ;;
    EMPTYHW=1) rm -rf "$WORK/setup/$2/.hw"; mkdir -p "$WORK/setup/$2/.hw" ;;
    NORECEIPT=1) rm -f "$WORK/setup/$2/.hw/$RUN/receipt.jsonl" ;;
  esac
}
verb_argv() {  # $1=verb $2=task
  case "$1" in
    ruling) printf '%s\n' ruling pIU "a correction" --dry-run ;;
    done) printf '%s\n' done setup "$2" ;;
    receipt) printf '%s\n' receipt setup "$2" ;;
  esac
}

# One case on one bindir: rebuild the state, run the verb, print the filtered
# output (the refusal lines, the exit code, and the close marker).
run_case() {  # $1=bindir $2=case-spec
  local name verbk task status tokens flags post extra argv=() a out cwd absent=""
  IFS='|' read -r name verbk task status tokens flags post extra <<< "$2"
  setup_case "$verbk" "$task" "$flags" "$post" "$extra"
  cwd="$WORK/setup/$task"
  case "$extra" in STUB_CWD=*) cwd="${extra#STUB_CWD=}" ;; STUB_ABSENT=1) absent=1 ;; esac
  while IFS= read -r a; do argv+=("$a"); done < <(verb_argv "$verbk" "$task")
  out="$(set +e; STUB_STATUS="$status" STUB_TOKENS="$tokens" STUB_CWD="$cwd" STUB_ABSENT="$absent" \
           PATH="$1:$PATH" HW_DONE_TURN_WAIT=1 HW_DONE_TURN_POLL=1 "$1/hw" "${argv[@]}" 2>&1 | strip; printf 'rc=%s\n' "${PIPESTATUS[0]}")"
  case "$verbk" in
    receipt) out="$(printf '%s\n' "$out" | rg '✗|!|rc=' || true)" ;;
    done) out="$(printf '%s\n' "$out" | rg '✗|!|rc=|ports released with the panes' || true)" ;;
  esac
  printf '%s' "$out"
}
run_all() {  # $1=bindir
  local c name
  for c in "${CASES[@]}"; do
    name="${c%%|*}"
    printf '%s\t%s\n' "$name" "$(run_case "$1" "$c" | python3 -c 'import sys;print(sys.stdin.read().encode("unicode_escape").decode())')"
  done
}

# The actions side: facts read from the SAME state, per case, against the
# output of the verb itself (OLD bin) on that state.
actions_json() {  # $1=bindir $2=wt $3=status $4=tokens $5=cwd
  python3 -I - "$1/hw-actions" "$2" "$3" "$4" "$5" <<'PY'
import importlib.machinery, importlib.util, json, sys
path, wt, status, tokens, cwd = sys.argv[1:6]
loader = importlib.machinery.SourceFileLoader("hw_actions", path)
spec = importlib.util.spec_from_loader("hw_actions", loader)
m = importlib.util.module_from_spec(spec); loader.exec_module(m)
tok = (json.loads(tokens) or {}) if tokens else {}
tok.update({"hw_project": "setup", "hw_task": wt.rsplit("/", 1)[-1]})
item = {"pane_id": "pIU", "agent_status": status, "cwd": cwd, "tokens": tok}
print(json.dumps(m.actions_for_row(m.facts_from_pane(item))))
PY
}
check_actions() {  # $1=bindir
  local c name verbk task status tokens flags post extra out act cwd bad=0 rc=0
  for c in "${CASES[@]}"; do
    IFS='|' read -r name verbk task status tokens flags post extra <<< "$c"
    out="$(run_case "$OLDBIN" "$c")"          # also rebuilds the state
    cwd="$WORK/setup/$task"; case "$extra" in STUB_CWD=*) cwd="${extra#STUB_CWD=}" ;; esac
    [ "$extra" != STUB_ABSENT=1 ] || status=""
    # a --dry-run ruling that would queue prints "would queue"; a resume "would RESUME"
    act="$(actions_json "$1" "$WORK/setup/$task" "$status" "$tokens" "$cwd")" || { echo "actions: $name could not be computed"; bad=1; continue; }
    python3 -I - "$name" "$verbk" "$act" "$out" <<'PY' || bad=1
import json, sys
name, key, act, out = sys.argv[1:5]
a = json.loads(act)
ok, why = a[key], a["why_not"].get(key)
if key == "ruling":
    went = "would queue" in out or "would RESUME" in out
elif key == "done":
    went = "ports released with the panes" in out
else:
    went = "none carries a receipt" not in out and "✗" not in out
if ok != went:
    print("actions: %s: %s=%s but the verb %s" % (name, key, ok, "went through" if went else "refused")); sys.exit(1)
if not ok:
    # The marker path inside a done reason is spelled by the caller (hw done
    # builds the run dir with a trailing slash); the text is otherwise the same.
    flat = out.replace("//", "/")
    for line in why.split("\n"):
        if line.strip().replace("//", "/") not in flat:
            print("actions: %s: why_not line not in the verb's output: %r" % (name, line)); sys.exit(1)
PY
  done
  return $bad
}

# ── OLD/NEW on the real bytes ───────────────────────────────────────────────
old_out="$(run_all "$OLDBIN")"
new_out="$(run_all "$NEWBIN")"
[ -n "$old_out" ] || fail "796: the old verbs produced nothing — the stubs are not reaching them"
if [ "$old_out" = "$new_out" ]; then
  pass "796: OLD ($OLD_REV) and NEW verbs print the same refusal and exit with the same code in all ${#CASES[@]} cases"
else
  diff <(printf '%s\n' "$old_out") <(printf '%s\n' "$new_out") >&2 || true
  fail "796: the verbs' refusals changed with the extraction (diff above)"
fi
# The classes really are distinct outcomes, not twenty copies of one refusal.
for needle in "has already reported task" "is reporting task" "is idle (herdr" "herdr knows no pane" \
              "reports no cwd" "no hw run directory" "would RESUME" "would queue a ruling" \
              "has NOT reported" "is STRANDED" "is WORKING right now" "turn(s) ended in" \
              "ports released with the panes" "no hw runs for" "no runs under" "none carries a receipt"; do
  case "$old_out" in *"$needle"*) ;; *) fail "796: no case exercised «$needle» — a refusal class is untested" ;; esac
done
pass "796: every refusal class is exercised"

act_report="$(check_actions "$NEWBIN")" && pass "796: actions_for_row says false exactly where the verb refused, with the verb's own text as why_not" \
  || { printf '%s\n' "$act_report" >&2; fail "796: actions_for_row disagrees with the verb (see above)"; }

# ── mutants of hw-actions: each must turn the comparison red ────────────────
mutate_file() {  # $1=name $2=python replace expr: old\0new
  local d="$TMP/mut-$1"; [ -d "$d" ] || mkbin "$d" new
  python3 - "$d/hw-actions" "$2" "$3" <<'PY' || fail "796: mutant $1 did not apply"
import sys
p, a, b = sys.argv[1:4]
s = open(p).read()
if s.count(a) != 1: sys.exit(1)
open(p, "w").write(s.replace(a, b))
PY
  printf '%s' "$d"
}
caught() {  # $1=name $2=bindir
  local out
  out="$(run_all "$2")"
  if [ "$out" != "$old_out" ]; then return 0; fi
  check_actions "$2" >/dev/null 2>&1 || return 0
  return 1
}
# The idle mutant is two edits to one copy: the idle refusal goes and idle joins the accepted states.
mutate_file idle-ok 'if verdict == "idle":
        return' 'if verdict == "idle" and False:
        return' >/dev/null
caught idle-accepted "$(mutate_file idle-ok 'if verdict in ("working", "awaiting_child"):' 'if verdict in ("working", "awaiting_child", "idle"):')" \
  && pass "796: mutant — an idle executor's ruling stops being refused: red" || fail "796: mutant 'idle ruling accepted' stayed green"
caught reason-text "$(mutate_file reason-text 'Somebody is talking to this pane.' 'Somebody talks to this pane.')" \
  && pass "796: mutant — a done reason's text altered: red" || fail "796: mutant 'done reason text altered' stayed green"
caught receipt-dropped "$(mutate_file receipt-dropped 'return "no runs under %s/.hw" % wt, True' 'return None, False')" \
  && pass "796: mutant — a receipt refusal dropped: red" || fail "796: mutant 'receipt refusal dropped' stayed green"
caught blocked-resume "$(mutate_file blocked-resume 'if verdict == "reported" and blocked_waiting:' 'if False:')" \
  && pass "796: mutant — the blocked-waiting resume stops being allowed: red" || fail "796: mutant 'blocked resume refused' stayed green"
pass "796-cockpit-actions-are-the-verbs-refusals: all subjects observed"
