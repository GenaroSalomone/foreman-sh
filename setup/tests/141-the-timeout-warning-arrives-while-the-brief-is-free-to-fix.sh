#!/usr/bin/env bash
# hw said the right thing about the close timeout, 900 seconds too late
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/141-the-timeout-warning-arrives-while-the-brief-is-free-to-fix.sh
#
# THE DEFECT. A brief put `bash setup/test-hw` in its `## Verification` block.
# `hw done` hung past seven minutes and died `VERIFICATION FAILED exit 124` —
# the full suite is ~1000s against a 900s ceiling.
#
# The advice was never missing. `hw done` DID name `HW_TEST_GATE=fast bash
# setup/test-hw` in that failure, and setup/tests/118 has guarded that warning
# since it was written. What was wrong is WHEN it arrives: at close, with the
# work already done and the 900 seconds already spent. Confirmed with
# `--dry-run` on the same brief — the dispatch manifest printed the verification
# block and said nothing at all about the timeout or the gate.
#
# So this subject is about the MOMENT, not the message. The one reader who can
# still fix the brief for nothing is the brainer looking at a dispatch before it
# launches, and that reader was the one never told. 118 keeps the close-side
# warning; this keeps the dispatch-side one.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# Briefs are read by path, and `_brief_verify_cmds` reads fenced code under a
# Testing/Verification heading — so the fixtures are real briefs, not strings.
mkdir -p "$TMP/briefs"
brief() {  # <name> <verification command>
  printf '# probe\n\nDo the thing.\n\n## Verification\n\n```\n%s\n```\n' "$2" \
    > "$TMP/briefs/$1.md"
  printf '%s' "$TMP/briefs/$1.md"
}
FULL="$(brief full 'bash setup/test-hw')"
FAST="$(brief fast 'HW_TEST_GATE=fast bash setup/test-hw')"
OTHER="$(brief other 'bash setup/tests/22-hw-unstick.sh')"

# ── 1. the dispatch says it, in the manifest a human reads ─────────────────
out="$(hw_dry setup probe-full --brief "$FULL" --sdd none || true)"
case "$out" in
  *"WILL TIME OUT AT CLOSE"*"~1000s"*"HW_TEST_GATE=fast"*)
    pass "dispatch: a verify block naming the full suite is flagged ON THE MANIFEST LINE, beside the command" ;;
  *) fail "dispatch: the manifest printed the verify command with nothing about the ${_VT:-900}s ceiling it will hit: $(printf '%s' "$out" | grep -i verify)" ;;
esac
case "$out" in
  *"hw runs it itself at close"*"exit 124"*)
    pass "dispatch: and again as a warning, naming the exit code it will actually produce" ;;
  *) fail "dispatch: no warning beside the manifest — a line inside a dim block is easy to scroll past: $out" ;;
esac
# THE FIX IS NAMED, not just the problem. A warning that does not say what to
# write instead sends the reader back to the same brief with the same question.
case "$out" in
  *"Fix the brief now"*"HW_TEST_GATE=fast bash setup/test-hw"*)
    pass "dispatch: the warning names the exact line to put in the brief instead" ;;
  *) fail "dispatch: the warning describes the problem and not the fix: $out" ;;
esac
# AND IT SAYS WHY THE FULL SUITE IS NOT BEING LOST — the push hook still pays
# for it. Without that the advice reads as "check less", which it is not.
case "$out" in
  *"mandatory at push"*"pre-push"*)
    pass "dispatch: it says where full-suite coverage still happens, so the advice is not 'check less'" ;;
  *) fail "dispatch: the warning does not say that the full suite is still mandatory at push: $out" ;;
esac

# ── 2. NO FALSE POSITIVES, which is what makes the warning readable ────────
out="$(hw_dry setup probe-fast --brief "$FAST" --sdd none || true)"
case "$out" in
  *"WILL TIME OUT AT CLOSE"*|*"names the full suite without"*)
    fail "dispatch: a brief that already names HW_TEST_GATE=fast was warned about anyway: $out" ;;
  *) pass "dispatch: a verify block that already names the fast gate is not warned about" ;;
esac
out="$(hw_dry setup probe-other --brief "$OTHER" --sdd none || true)"
case "$out" in
  *"WILL TIME OUT AT CLOSE"*|*"names the full suite without"*)
    fail "dispatch: an unrelated verify command triggered the full-suite warning: $out" ;;
  *) pass "dispatch: an unrelated verify command is not warned about" ;;
esac
out="$(hw_dry setup probe-nobrief --sdd none || true)"
case "$out" in
  *"WILL TIME OUT AT CLOSE"*) fail "dispatch: a dispatch with no brief at all was warned about: $out" ;;
  *) pass "dispatch: no brief means nothing to warn about" ;;
esac

# ── 3. EVERY PINNED COMMAND IS CHECKED, not only the first ─────────────────
# The manifest NAMES the first command and summarises the rest as `(+N more)`.
# A full-suite line sitting second is exactly as expensive at close and exactly
# as invisible on the manifest, so it has to be found by the check rather than
# by the line.
printf '# probe\n\n## Verification\n\n```\nbash setup/tests/22-hw-unstick.sh\nbash setup/test-hw\n```\n' \
  > "$TMP/briefs/second.md"
out="$(hw_dry setup probe-second --brief "$TMP/briefs/second.md" --sdd none || true)"
case "$out" in
  *"(+1 more)"*) : ;;
  *) fail "dispatch: the two-command fixture did not produce the (+N more) shape this case depends on: $out" ;;
esac
case "$out" in
  *"WILL TIME OUT AT CLOSE"*)
    pass "dispatch: a full-suite line in SECOND position is found, though the manifest names only the first" ;;
  *) fail "dispatch: only the first pinned command was checked, so a second full-suite line reaches the close unannounced: $out" ;;
esac

# (go-060 S4: the structural check that ONE bash function answered at both moments, and mutant M01 that disabled the dispatch-side
# call in a copy of bin/hw, went with the bash body. The dispatch-side claim is held by Go anchor 844-141-M01 in core/t5_manifest.go;
# the close side keeps its own detector, doneNamesFullSuite, so the two moments are no longer one function.)
