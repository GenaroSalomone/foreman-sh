#!/usr/bin/env bash
# The ask cap must leave a reportable exit instead of forcing a silent turn.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
need_invokers   # this subject runs ask-invoker, which is Go since go-060 S2 and has no core/dist beside a copy of bin/

HARNESS="$TMP/harness"; WORK="$TMP/work"; mkdir -p "$HARNESS/bin" "$WORK/.hw/run"
cp "$ROOT/bin/ask-invoker" "$ROOT/bin/invoker-common.sh" "$ROOT/bin/runenv" "$HARNESS/bin/"
cat > "$HARNESS/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
cat > "$HARNESS/bin/channel-send" <<'STUB'
#!/usr/bin/env bash
exit 99
STUB
chmod +x "$HARNESS/bin/ask-invoker" "$HARNESS/bin/herdr-rpc" "$HARNESS/bin/channel-send"
printf '3' > "$WORK/.hw/run/ask-count"

run_cap() {
  HW_INVOKER_PANE=wB:p1 HERDR_PANE_ID=wT:p1 HW_TASK=probe HW_PROJECT=setup \
    HW_WORKDIR="$WORK" HW_RUN=run "$1" "one more question" 2>&1
}

out="$(run_cap "$HARNESS/bin/ask-invoker")" && rc=0 || rc=$?
[ "$rc" = 1 ] || fail "C01 capped ask expected exit 1, got $rc: $out"
case "$out" in *'done-invoker --blocked "<the unresolved question and what would unblock it>"'*'cap never requires a silent turn'*) pass "C01 the ask cap names the blocked-report legal exit" ;; *) fail "C01 ask cap created a silent dead end: $out" ;; esac

# The one bash mutant of this subject (45-M01: the cap refusal stops naming the legal exit) edited bin/ask-invoker's bash, which is gone
# (go-060 S5). The Go holds it as core/invokers/mutants_test.go 45-M01 (ask.go: "The cap never requires a silent turn." becomes a refusal
# with no exit), killed by TestDiffAsk/ask_cap; C01 above is the same claim, driven on the shim and the binary.
printf 'mapping - C01 ask-cap legal exit (45-M01 lives in core/invokers/mutants_test.go)\n'

# The cap is 3 for every run. It was 4 under gentle (the phase-1 approval rode an ask) until gentle
# left hw (setup/decisions.md, 2026-10-16): a run whose dispatch record still says gentle is capped
# at 3 like any other, and the 4th ask is refused.
printf '  framework   gentle  (chosen)\n' > "$WORK/.hw/run/dispatch"
printf '3' > "$WORK/.hw/run/ask-count"
gout="$(run_cap "$HARNESS/bin/ask-invoker")" && grc=0 || grc=$?
case "$gout" in *'already asked the brainer 3 times on this task (limit 3)'*) pass "G01 a run recorded as gentle is capped at 3 (rc=$grc)" ;; *) fail "G01 the retired gentle cap of 4 is still honoured: $gout" ;; esac
printf '  framework   none  (chosen)\n' > "$WORK/.hw/run/dispatch"
printf '3' > "$WORK/.hw/run/ask-count"
nout="$(run_cap "$HARNESS/bin/ask-invoker")" && nrc=0 || nrc=$?
case "$nout" in *'already asked the brainer 3 times on this task (limit 3)'*) pass "G02 a none run stays capped at 3" ;; *) fail "G02 the cap moved: $nout" ;; esac
printf 'coverage - 1 behavior claim, 0 mutants (the bash mutant is retired with the bash body)\n'
