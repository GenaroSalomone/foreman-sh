#!/usr/bin/env bash
# bin/hw-reconcile: a read-only recovery queue for completed, undelivered work.
#
# hw-reconcile is Go (hw-core hw-reconcile, core/wrappers_reconcile.go) since bash-port-rest; bin/hw-reconcile is the shim that execs it. R01-R05
# below drive the shim as the operator does, and are the contract the bash met; the same scenarios run on hrRun in
# core/wrappers_reconcile_test.go, and the three mutants of the bash (M01-M03, which edited its text) are the Go anchors bpr4-turns, bpr4-done
# and bpr4-holdage, killed by setup/tests/1298.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

SUBJECT="$ROOT/bin/hw-reconcile"
[ -f "$SUBJECT" ] || fail "R00 bin/hw-reconcile is missing"

HARNESS="$TMP/harness"
mkdir -p "$HARNESS/bin" "$TMP/state"
cp "$SUBJECT" "$HARNESS/bin/hw-reconcile"
# bin/holdfacts is the validator of a hold; the lane table and the witness are read by hw-core itself.
cp "$ROOT/bin/holdfacts" "$HARNESS/bin/holdfacts"
cp "$ROOT/projects.json" "$HARNESS/projects.json"

cat > "$HARNESS/bin/herdr-rpc" <<STUB
#!/usr/bin/env bash
printf 'rpc %s\n' "\$*" >> "$TMP/state/calls"
case "\$*" in
  *pMove*)
    n=\$(( \$(cat "$TMP/state/move" 2>/dev/null || echo 0) + 1 ))
    printf '%s' "\$n" > "$TMP/state/move"
    printf '{"read":{"text":"tick %s"}}\n' "\$n" ;;
  *) printf '{"read":{"text":"static"}}\n' ;;
esac
STUB
chmod +x "$HARNESS/bin/herdr-rpc"

cat > "$TMP/bin/herdr" <<STUB
#!/usr/bin/env bash
printf 'herdr %s\n' "\$*" >> "$TMP/state/calls"
case "\$1 \$2" in
  "agent get")
    pane="\$3"
    case "\$pane" in
      # pane_id is in every real reply and hw-reconcile checks it: measured
      # against the live daemon 2026-09-08, \`herdr agent get <pane>\` answers
      # with 16 keys including "pane_id":"<the pane asked for>". A stub that
      # omits it is not a cheaper daemon, it is a shape the daemon never emits.
      pDone) printf '{"result":{"agent":{"pane_id":"%s","agent_status":"working","tokens":{"done_status":"done","done_state":"undelivered"}}}}\n' "\$pane" ;;
      pBlocked) printf '{"result":{"agent":{"pane_id":"%s","agent_status":"blocked","tokens":{"done_status":"blocked","done_state":"undelivered"}}}}\n' "\$pane" ;;
      pTurn|pMove) printf '{"result":{"agent":{"pane_id":"%s","agent_status":"idle","tokens":{"turn_state":"ended_unreported"}}}}\n' "\$pane" ;;
      *) exit 1 ;;
    esac ;;
  "pane process-info")
    printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","49999"]}]}}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
cat > "$TMP/bin/curl" <<'STUB'
#!/usr/bin/env bash
printf '[]\n'
STUB
chmod +x "$TMP/bin/herdr" "$TMP/bin/curl"

make_run() { # root task run pane turns
  local root="$1" task="$2" run="$3" pane="$4" turns="$5"
  local dir="$root/$task/.hw/$run"
  mkdir -p "$dir"
  printf "HW_PROJECT='setup'\nHW_TASK='%s'\nHW_RUN='%s'\nHW_WORKDIR='%s'\nHW_ARTIFACTS='%s'\nHW_INVOKER_PANE='w:pBrain'\n" \
    "$task" "$run" "$root/$task" "$root/$task" > "$dir/env"
  [ -z "$pane" ] || printf '%s\n' "{\"key\":\"pane\",\"value\":\"$pane\"}" > "$dir/receipt.jsonl"
  [ "$turns" = 0 ] || printf '%s\n' "$turns" > "$dir/turns"
  printf '%s' "$dir"
}

run_reconcile() { # binary root
  local binary="$1" root="$2" rc=0
  rm -f "$TMP/state/move"
  HW_RECONCILE_ROOTS="$root" HW_RECONCILE_NOW=2000000000 \
    HW_RECONCILE_HOLD_SECONDS=900 WITNESS_SAMPLES=2 WITNESS_INTERVAL_S=0 \
    PATH="$TMP/bin:$PATH" "$binary" 2>&1 || rc=$?
  printf '\n__RC=%s\n' "$rc"
}

# R01 — durable turn end, and every veto that prevents probe noise.
r1="$TMP/r1"; mkdir -p "$r1"
make_run "$r1" ended-positive 20260831-100000-1 '' 1 >/dev/null
printf 'finished output\n' > "$r1/ended-positive/result.txt"
done_dir="$(make_run "$r1" ended-reported 20260831-100001-2 '' 1)"; : > "$done_dir/done"
probe_dir="$(make_run "$r1" intentional-probe 20260831-100002-3 '' 1)"
printf "HW_PROJECT='setup'\nHW_TASK='intentional-probe'\nHW_RUN='20260831-100002-3'\nHW_NO_REPORT='1'\n" > "$probe_dir/env"
# An older failed attempt is not a second orphan after this task's newest run
# reported. The queue is task-oriented, not an archaeology of every attempt.
old_dir="$(make_run "$r1" retried-task 20260831-090000-4 '' 1)"
new_dir="$(make_run "$r1" retried-task 20260831-100003-5 '' 1)"; : > "$new_dir/done"
out="$(run_reconcile "$HARNESS/bin/hw-reconcile" "$r1")"
case "$out" in *"setup:ended-positive"*"ended-unreported:disk"*"__RC=3"*) : ;; *) fail "R01 ended turn was not raised: $out" ;; esac
case "$out" in *"ended-reported"*|*"intentional-probe"*|*"retried-task"*) fail "R01 a reported run, superseded attempt, or explicit probe entered the queue: $out" ;; esac
pass "R01 a durable ended turn is raised; done, superseded, and explicit --no-report runs are vetoed"

# R02 — exact live turn token is only actionable after the shared witness says
# the pane is no longer moving. This is why hw-reconcile sources state-witness.
r2="$TMP/r2"; mkdir -p "$r2"
make_run "$r2" settled-turn 20260831-110000-1 pTurn 0 >/dev/null
make_run "$r2" moving-turn 20260831-110001-2 pMove 0 >/dev/null
out="$(run_reconcile "$HARNESS/bin/hw-reconcile" "$r2")"
case "$out" in *"setup:settled-turn"*"ended-unreported:settled"*) : ;; *) fail "R02 settled live turn was not raised through the witness: $out" ;; esac
case "$out" in *"moving-turn"*) fail "R02 a moving pane was called orphaned: $out" ;; esac
pass "R02 ended_unreported uses state-witness: settled is raised and moving is vetoed"

# R03 — done_status=done is completion evidence even when delivery and disk lost.
r3="$TMP/r3"; mkdir -p "$r3"
make_run "$r3" done-token 20260831-120000-1 pDone 0 >/dev/null
make_run "$r3" blocked-token 20260831-120001-2 pBlocked 0 >/dev/null
out="$(run_reconcile "$HARNESS/bin/hw-reconcile" "$r3")"
case "$out" in *"setup:done-token"*"done-token:undelivered"*) : ;; *) fail "R03 done token without disk marker was not raised: $out" ;; esac
case "$out" in *"blocked-token"*) fail "R03 blocked was promoted to completed work: $out" ;; esac
pass "R03 done_status=done without a done marker is raised; done_status=blocked is not"

# R04 — delivered holds become findings only after the configured threshold.
r4="$TMP/r4"; mkdir -p "$r4"
old_dir="$(make_run "$r4" old-hold 20260831-130000-1 '' 0)"
fresh_dir="$(make_run "$r4" fresh-hold 20260831-130001-2 '' 0)"
answered_dir="$(make_run "$r4" answered-hold 20260831-130002-3 '' 0)"
hold() { printf 'version=1\nstate=%s\nintent=ruling\nroute=herdr\ntarget=pX\nlogical_id=%s:challenge:1:reply\npane=pX\nrun=%s\n' "$1" "$2" "$2"; }
hold delivered 20260831-130000-1 > "$old_dir/pending-reply-1"
hold delivered 20260831-130001-2 > "$fresh_dir/pending-reply-1"
hold answered 20260831-130002-3 > "$answered_dir/pending-reply-1"
touch -t 202001010000 "$old_dir/pending-reply-1"
touch -t 204001010000 "$fresh_dir/pending-reply-1" "$answered_dir/pending-reply-1"
out="$(run_reconcile "$HARNESS/bin/hw-reconcile" "$r4")"
case "$out" in *"setup:old-hold"*"stale-hold:pending-reply-1:ruling"*) : ;; *) fail "R04 aged delivered hold was not raised: $out" ;; esac
case "$out" in *"fresh-hold"*|*"answered-hold"*) fail "R04 fresh or answered hold entered the queue: $out" ;; esac
pass "R04 only a delivered pending reply older than the threshold is raised"

# The safety boundary is command-level, not prose: every observed herdr/RPC call
# must be a read used by agent inspection or state-witness.
bad_calls="$(grep -Ev '^herdr (agent get|pane process-info)|^rpc call pane.read' "$TMP/state/calls" 2>/dev/null || true)"
[ -z "$bad_calls" ] || fail "R05 reconciler used a pane-mutating route: $bad_calls"
pass "R05 reconciliation sends no prompt, key, or pane mutation; its observed calls are reads only"

# Mutation arms: M01-M03 edited the bash text of the detector (MUTATION_ANCHOR_TURNS, _DONE_STATUS, _HOLD_AGE). They are three Go anchors now,
# and a Go anchor is killed by the Go test that holds its claim (setup/tests/1298 builds each mutant of core/ and runs R01, R03 and R04
# there); here the three are proved to be where the detector is.
go_anchor_killed bpr4-turns core/wrappers_reconcile.go "a durable ended turn (turns > 0, report-worthy, no live agent readable) is raised"
go_anchor_killed bpr4-done core/wrappers_reconcile.go "done_status=done on the live pane is completion evidence"
go_anchor_killed bpr4-holdage core/wrappers_reconcile.go "a delivered hold is raised only once it is older than the threshold"
go_anchor_killed bpr4-live core/wrappers_reconcile.go "the witness is asked only when the pane is readable and the run ended unreported"
go_anchor_killed bpr4-brief core/wrappers_reconcile.go "a brief-backed run on a lane with a repository is raised with no artifact"
go_anchor_killed bpr4-readable core/wrappers_reconcile.go "an agent record that names another pane is not the live reading"

printf 'coverage - 5 behavior claims, 6 dedicated detector anchors held on the Go\n'
