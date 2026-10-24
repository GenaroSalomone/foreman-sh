#!/usr/bin/env bash
# Drive `hw unstick` against a Herdr-shaped process. The live owned probe proves
# terminal behavior; this covers every refusal and byte-preserving restart argv.
#
# Until go-060 S4 this extracted `cmd_unstick` from bin/hw and drove it with a stub herdr, herdr-rpc and curl, the REAL
# bin/state-witness.sh as the cross-check. hw has no bash body now: `unstick` is hw-core's (core/t6_unstick.go) and the same
# stubs, the same stuck-token signature and the same claims run there, in core/t6_unstick_test.go, one test per claim family.
# This subject runs them and requires each by name, so a renamed or dropped claim is red here and not a silent skip:
#   · TestT6UnstickTokenRefusalsSendNoKeys   every refusal sends no control key; a second OpenCode process is never authorized
#   · TestT6UnstickWitnessIsTheGate          the token signature is necessary and NOT sufficient: a live pane, an unknown
#                                            reader, a question or permission awaiting are refused on the cross-check alone,
#                                            naming the reader; a corroborated stuck pane still recovers ("cross-check CONFIRMS")
#   · TestT6UnstickSecondWitnessBeforeTheStop  a pane that wakes up after the first cross-check is re-checked before Ctrl-C
#   · TestT6UnstickPromptEvidence/Refusals   visible text is not prompt evidence at either read gate
#   · TestT6UnstickFailsClosedBeforeAnyRestart  unreadable screen, wrapper argv, state changed after argv capture, a restart
#                                            name owned by another pane, a malformed registry: all refuse before Ctrl-C
#   · TestT6UnstickScopes                    root scope recovers with no child navigation; child scope opens the subagent view
#   · TestT6UnstickPostRestartStates         working is accepted after the restart, blocked is not claimed recovered
#   · TestT6UnstickRestartArgv               direct executable argv restarts, whitespace and newlines survive
#   · TestT6UnstickLock                      same-pane contention across TMPDIRs, a proven-dead lock is reclaimed, released
#   · TestT6UnstickNamesWhatItReplaces / ReceiptFallback / ReceiptLast / SessionMismatchClearsNothing
#                                            the outgoing session id is recorded BEFORE Ctrl-C, named in the output with the
#                                            `opencode export` command, repeated after the restart; no session is recorded as
#                                            an absence; with no run dir the output says it is the only place; a refusal
#                                            records nothing
#   · TestT6UnstickRegistryCollision / ForegroundArgs / ProcessState / ProcessExitMustBeObserved / Usage
#                                            the pure readers and the wait for the old process to exit
# Run alone while working on this subject:
#     bash setup/tests/22-hw-unstick.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

command -v go >/dev/null 2>&1 || fail "no go toolchain: hw-core's unstick cannot be driven, and a skipped claim is not a passed one"
TESTS="TestT6UnstickUsage TestT6UnstickTokenRefusalsSendNoKeys TestT6UnstickWitnessIsTheGate TestT6UnstickSecondWitnessBeforeTheStop
 TestT6UnstickPromptEvidence TestT6UnstickPromptRefusals TestT6UnstickFailsClosedBeforeAnyRestart TestT6UnstickScopes
 TestT6UnstickProcessExitMustBeObserved TestT6UnstickPostRestartStates TestT6UnstickRestartArgv TestT6UnstickSessionMismatchClearsNothing
 TestT6UnstickReceiptFallback TestT6UnstickNamesWhatItReplaces TestT6UnstickLock TestT6UnstickForegroundArgs TestT6UnstickProcessState
 TestT6UnstickRegistryCollision TestT6UnstickReceiptLast"
regex="^($(printf '%s\n' $TESTS | paste -sd'|' -))\$"
out="$(cd "$ROOT/core" && GOCACHE="${GOCACHE:-$HW_GOCACHE_DEFAULT}" GOPATH="$TMP/gopath" CGO_ENABLED=0 GOTOOLCHAIN=local \
  go test -count=1 -timeout 600s -v -run "$regex" . 2>&1)" \
  || fail "hw unstick no longer refuses, records or recovers as claimed: $(printf '%s\n' "$out" | tail -40)"
for t in $TESTS; do
  printf '%s\n' "$out" | grep -qF -- "--- PASS: $t " || fail "hw unstick: the case $t did not run and pass: $(printf '%s\n' "$out" | tail -20)"
  pass "hw unstick: $t (hw-core)"
done
