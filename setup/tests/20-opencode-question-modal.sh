#!/usr/bin/env bash
# the opencode `question` modal, and the dead end it used to be
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/20-opencode-question-modal.sh
#
# WHY THIS SUBJECT EXISTS. An opencode executor that calls its `question` tool
# opens a modal, and herdr's plugin reports `blocked`. Nothing in that plugin
# leaves `blocked` except permission.replied / question.replied /
# question.rejected, and an idle opencode emits none of them by itself — so the
# pane sits `blocked` forever and every route in this setup gates on
# `idle,done`. Measured 2026-08-25 on a disposable pane: question.asked ->
# blocked, still blocked 43s later; `pane.send_keys ["escape"]` ->
# question.rejected -> working -> session.idle -> idle, reachable again.
#
# What is asserted here is the GUARD, not the keystroke: the keystroke is
# verified end-to-end against a live pane and recorded in the commit, because
# a stub cannot prove that escape dismisses anything.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# A stub herdr-rpc that answers the three calls the helper makes, driven by
# env. `agent.get` shape is herdr-rpc's, which unwraps `result` — so the top
# level is `.agent`, NOT `.result.agent`. Getting that wrong is silent: jq
# returns empty, the helper returns 1, and the rescue never fires.
cat > "$TMP/rpc" <<'STUB'
#!/usr/bin/env bash
case "${1:-} ${2:-}" in
  "call agent.get")
    printf '{"agent":{"agent_status":"%s","agent":"%s"}}\n' \
      "${STUB_STATE:-blocked}" "${STUB_AGENT:-opencode}" ;;
  "call pane.read")
    printf '{"read":{"text":%s}}\n' "${STUB_TEXT_JSON:-\"↑↓ select  enter submit  esc dismiss\"}" ;;
  "call pane.send_keys")
    printf 'send_keys %s\n' "${3:-}" >> "${STUB_KEYS_LOG:-/dev/null}"
    echo '{"type":"ok"}' ;;
  "wait-agent"*) exit "${STUB_WAIT_RC:-0}" ;;
  *) echo '{}' ;;
esac
STUB
chmod +x "$TMP/rpc"

# The helper is a shell function, so drive it directly rather than through a
# command that would need a whole live dispatch to reach it.
clear_q() {
  # INVOKER_RPC is overridden AFTER the source, not before: invoker-common.sh
  # sets it from INVOKER_BIN_DIR unconditionally, so exporting it first is
  # silently discarded and every assertion below would hit the real herdr.
  env "$@" bash -c '
    INVOKER_BIN_DIR="$1/bin"
    . "$1/bin/invoker-common.sh" >/dev/null 2>&1 || true
    INVOKER_RPC="$2"
    invoker_clear_opencode_question wX:p1 && echo CLEARED || echo REFUSED' \
    _ "$ROOT" "$TMP/rpc"
}

keys="$TMP/keys.log"

# ── it clears the case it was written for ──────────────────────────────────
: > "$keys"
case "$(clear_q STUB_KEYS_LOG="$keys" HW_TASK=t HW_PROJECT=setup HW_RUN=r \
        HW_WORKDIR="$TMP" HERDR_PANE_ID=wX:p2 HW_INVOKER_PANE=wX:p3)" in
  *CLEARED*) pass "question modal: a blocked opencode pane showing 'esc dismiss' is cleared" ;;
  *) fail "question modal: the case the helper exists for was refused" ;;
esac
grep -q 'escape' "$keys" || fail "question modal: no escape key was sent"
pass "question modal: the key actually sent is escape"

# ── and refuses everything else ────────────────────────────────────────────
# THE THREE REFUSALS ARE THE POINT. A blind `escape` at a terminal is a
# keystroke with no owner, and on a permission prompt it means DENY.
: > "$keys"
case "$(clear_q STUB_KEYS_LOG="$keys" STUB_STATE=idle HW_TASK=t HW_PROJECT=setup \
        HW_RUN=r HW_WORKDIR="$TMP" HERDR_PANE_ID=wX:p2 HW_INVOKER_PANE=wX:p3)" in
  *REFUSED*) pass "question modal: a pane that is not blocked is left alone" ;;
  *) fail "question modal: acted on a pane that was not blocked" ;;
esac
[ -s "$keys" ] && fail "question modal: sent a key to a pane that was not blocked"
pass "question modal: no key reaches a pane that is not blocked"

: > "$keys"
case "$(clear_q STUB_KEYS_LOG="$keys" STUB_AGENT=claude HW_TASK=t HW_PROJECT=setup \
        HW_RUN=r HW_WORKDIR="$TMP" HERDR_PANE_ID=wX:p2 HW_INVOKER_PANE=wX:p3)" in
  *REFUSED*) pass "question modal: a non-opencode pane is left alone" ;;
  *) fail "question modal: acted on a pane that is not opencode" ;;
esac

# The footer is the evidence that a DISMISSIBLE modal is on screen. Without it
# the pane is blocked for some other reason — a permission prompt among them —
# and dismissing it would answer a question nobody asked.
: > "$keys"
case "$(clear_q STUB_KEYS_LOG="$keys" STUB_TEXT_JSON='"waiting for permission: allow this?"' \
        HW_TASK=t HW_PROJECT=setup HW_RUN=r HW_WORKDIR="$TMP" HERDR_PANE_ID=wX:p2 \
        HW_INVOKER_PANE=wX:p3)" in
  *REFUSED*) pass "question modal: a blocked pane with no 'esc dismiss' footer is left alone" ;;
  *) fail "question modal: dismissed a modal that was not a question" ;;
esac
[ -s "$keys" ] && fail "question modal: sent a key with no dismissible modal on screen"
pass "question modal: no key reaches a pane with no dismissible modal"

# ── sending the key is not the claim ───────────────────────────────────────
# It verifies afterwards. A helper that reported success on the send would let
# `hw next` move its counter against a pane that never became reachable.
: > "$keys"
case "$(clear_q STUB_KEYS_LOG="$keys" STUB_WAIT_RC=2 HW_TASK=t HW_PROJECT=setup \
        HW_RUN=r HW_WORKDIR="$TMP" HERDR_PANE_ID=wX:p2 HW_INVOKER_PANE=wX:p3)" in
  *REFUSED*) pass "question modal: a key that did not move the pane is not reported as cleared" ;;
  *) fail "question modal: claimed cleared without the pane leaving blocked" ;;
esac
grep -q 'escape' "$keys" || fail "question modal: the escape was not attempted at all"
pass "question modal: the escape was attempted, and its failure was reported as failure"
