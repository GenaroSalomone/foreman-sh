#!/usr/bin/env bash
# A card's context use is what the pane's statusline says, and no window is ever assumed
#
# Measured 2026-10-08 on the operator's screen: the card said ctx 100% on a sonnet-5-5 whose statusline said 41%
# (the writer read the transcript and assumed a 200k window on a 1M one), and ctx n/a on executors that
# were working. bin/cockpit-state now reads the pane's own statusline (`herdr agent read <pane>`, the
# source `hw status` uses) as the primary source; the turn-end value and the transcript are the fallback
# and give tokens, with a percent only for a model id that names its window (`[1m]`). The checks are in
# _cockpit_ctx_checks.py:
#
#   · statusline_is_the_source        tokens AND percent from the statusline win over a turn-end 100%
#                                     and over a transcript, on a working card and an idle one
#   · unknown_percent_is_tokens_only  `Ctx: 237.5k | Ctx Use…` (a narrow pane): tokens, percent null
#   · no_window_is_assumed            283.8k tokens of sonnet-5-5 from a transcript: no percent; a `[1m]` id
#                                     has one; a turn-end value with no percent shows its tokens
#   · falls_back_when_the_pane_cannot_be_read  no statusline / a failing read: the old sources; an opencode
#                                     pane's screen is not a claude statusline
#   · reads_are_cached                a pane is read once per TTL, shared by the writers; --stdout writes none
#   · read_timeout_is_bounded         eight hanging reads cost one timeout, and the cards fall back
#   · brainers_do_not_share_reads_or_cache  a writer reads only its own brainer's panes; two brainers, two caches
#
# and each rule has a mutant of the writer that these checks must kill, naming the check that does.
#
# Run alone while working on this subject:
#     bash setup/tests/922-cockpit-ctx-from-the-statusline.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

[ -x "$ROOT/bin/cockpit-state" ] || { pass "922: this tree carries no cockpit writer"; exit 0; }
CHECKS="$ROOT/setup/tests/_cockpit_ctx_checks.py"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/t922.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

out="$(python3 -I "$CHECKS" "$ROOT/bin" 2>&1)" || { printf '%s\n' "$out" >&2; fail "922: the ctx checks failed on the real tree (above)"; }
printf '%s\n' "$out"
pass "922: the statusline is the source of a card's context; the fallbacks assume no window; reads are cached and bounded"

mutant_dir() {  # <id> — a COPY of the writer and the tools it loads; sets MD
  MD="$TMP/m-$1"
  mkdir -p "$MD"
  cp "$ROOT"/bin/cockpit-state "$ROOT"/bin/runenv "$ROOT"/bin/holdfacts "$ROOT"/bin/hw-actions "$MD/"
}
kill_mutant() {  # <id> <check> <needle>… — the check, run on the mutated copy, must fail with the mutant's own evidence
  local id="$1" check="$2" res rc=0; shift 2
  res="$(python3 -I "$CHECKS" "$MD" "$check" 2>&1)" || rc=$?
  [ "$rc" -ne 0 ] || fail "922: mutant $id SURVIVED — check $check passed on a writer with the rule broken"
  saw_mutant "$id $check" "$res" "$@"
}
mutant_dir 922-M01; mutate_anchor 922-M01 "$MD/cockpit-state" 'if False:'; kill_mutant 922-M01 statusline_is_the_source "must show the statusline"
mutant_dir 922-M02; mutate_anchor 922-M02 "$MD/cockpit-state" 'window = 200_000'; kill_mutant 922-M02 no_window_is_assumed "a window was assumed"
mutant_dir 922-M03; mutate_anchor 922-M03 "$MD/cockpit-state" 'if False:'; kill_mutant 922-M03 reads_are_cached "cache ignored"
mutant_dir 922-M04; mutate_anchor 922-M04 "$MD/cockpit-state" 'return (int(float(m.group(1)) * mult), None)'; kill_mutant 922-M04 statusline_is_the_source "must show the statusline"
mutant_dir 922-M05; mutate_anchor 922-M05 "$MD/cockpit-state" 'out, _ = p.communicate()'; kill_mutant 922-M05 read_timeout_is_bounded "TimeoutExpired" "took"
mutant_dir 922-M06; mutate_anchor 922-M06 "$MD/cockpit-state" 'sl = statuslines([p.get("pane_id") for p in claude_panes], now)'; kill_mutant 922-M06 brainers_do_not_share_reads_or_cache "must not be read"
mutant_dir 922-M07; mutate_anchor 922-M07 "$MD/cockpit-state" 'cache_file = os.path.join(os.path.dirname(path), "shared.cache")'; kill_mutant 922-M07 brainers_do_not_share_reads_or_cache "was overwritten"
pass "922: 7 mutants of the writer, each killed by the check that names its rule"
