#!/usr/bin/env bash
# MEASURED today: a ruling correcting a contract challenge reached its executor
# with every backticked identifier MISSING. Nothing in ask-invoker,
# done-invoker or channel-send evaluates its own text (no `eval` anywhere in
# the three) — the substitution happened in the BRAINER's own shell, before any
# of these tools ran, because the text sat inside a double-quoted argv word and
# bash expands backticks (and `$…`) inside double quotes regardless of what the
# receiving program does with the string afterward. Quoting harder on the
# sending side cannot fix this; the only fix is a way to hand over the text
# that the sending shell does not interpolate at all.
#
# WHY THIS IS MORE SERIOUS THAN A COSMETIC LOSS. If backticks are substituted,
# a ruling containing `` `something` `` EXECUTES `something` in the brainer's
# own shell. Today it cost words; the same mechanism with different text is
# accidental command execution. A channel the sending shell cannot touch at all
# removes the class, not just today's instance.
#
# THE FIX FOLLOWS AN EXISTING CONVENTION RATHER THAN INVENTING ONE:
# channel-send's own usage already documents `<endpoint|->` — a lone '-' as a
# sentinel in a positional slot. This applies the SAME sentinel to the message
# slot, in all four commands that receive free text: ask-invoker, done-invoker,
# channel-send, and `hw ruling`. A caller who writes the text with a
# single-quoted heredoc (`<<'EOF' ... EOF`, which bash does NOT expand) and
# pipes it in through a lone '-' never lets the sending shell see backticks,
# `$`, embedded quotes, or newlines as anything but bytes.
#
# THREE ERRORS, THREE MESSAGES — a message given both ways (argv text AND '-'),
# an empty stdin read, and (documented, not exercised here because it needs a
# real tty) reading '-' with nothing piped in at all. An empty message
# delivered would be worse than any one of these refusals, so each is checked.
#
# ADDITIVE, NOT REPLACING: every existing argv-words invocation must still work
# exactly as before. This is checked first, for all four commands, before any
# of the new stdin behaviour is exercised.
#
# Run alone while working on this subject:
#     bash setup/tests/135-a-message-can-cross-a-shell-that-cannot-touch-it.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

CS_SRC="$ROOT/bin/channel-send"
AI_SRC="$ROOT/bin/ask-invoker"
DI_SRC="$ROOT/bin/done-invoker"
HW_SRC="${HW_SOURCE:-$ROOT/bin/hw}"

# A payload with every character class the incident actually lost: a
# backtick pair (which bash would run as a command substitution if it ever sat
# inside a double-quoted argv word), a `$` expansion, and an embedded quote.
PAYLOAD='fix touches `deny-repo-writes.py`; cost is $0 either way, says "the brief"'

# ── codex stub: proves the BYTES, not just that something was accepted ──────
# channel-send's codex route runs `codex queue --remote ... --message "$MESSAGE"`
# directly (no shell re-interpretation of its own), so logging codex's argv
# verbatim is a faithful witness of exactly what channel-send decided MESSAGE
# was — the strongest evidence available without a live receiver.
mkdir -p "$TMP/cxbin"
cat > "$TMP/cxbin/codex" <<'STUB'
#!/usr/bin/env bash
: > "$CODEX_LOG"
for _a in "$@"; do printf '%s\n' "$_a" >> "$CODEX_LOG"; done
STUB
chmod +x "$TMP/cxbin/codex"
CODEX_LOG="$TMP/codex.log"; export CODEX_LOG

send_codex() {
  PATH="$TMP/cxbin:$PATH" "$CS_SRC" --require admitted codex fakethread /tmp/fake.sock "$@" 2>&1
}
codex_message_logged() {
  # the --message value is the token right after the literal --message
  awk '/^--message$/{getline; print; exit}' "$TMP/codex.log"
}

# ── 1. EXISTING ARGV INVOCATIONS STILL WORK, unchanged, for all four ────────
out="$(send_codex plain unquoted words)"
[ "$?" = 0 ] || fail "channel-send's ordinary argv path regressed: $out"
[ "$(codex_message_logged)" = "plain unquoted words" ] || fail "argv words were not joined into one message as before: $(codex_message_logged)"
pass "channel-send's existing argv-words message path is unchanged"

out="$(env -u HW_INVOKER_PANE "$AI_SRC" "a plain question" 2>&1 || true)"
case "$out" in *"no HW_INVOKER_PANE"*) pass "ask-invoker's existing argv-words question path still reaches its normal refusal" ;; *) fail "ask-invoker's argv path regressed: $out" ;; esac

out="$(env -u HW_INVOKER_PANE "$DI_SRC" "a plain summary" 2>&1 || true)"
case "$out" in *"no HW_INVOKER_PANE"*) pass "done-invoker's existing argv-words summary path still reaches its normal refusal" ;; *) fail "done-invoker's argv path regressed: $out" ;; esac

out="$(env -u HW_INVOKER_PANE "$HW_SRC" ruling nopane "a plain ruling" 2>&1 || true)"
case "$out" in *"herdr reports no cwd"*) pass "hw ruling's existing argv-words text path still reaches its normal refusal" ;; *) fail "hw ruling's argv path regressed: $out" ;; esac

# ── 2. '-' DELIVERS THE EXACT BYTES, backticks and '$' untouched ────────────
out="$(printf '%s' "$PAYLOAD" | send_codex -)"
[ "$?" = 0 ] || fail "channel-send's stdin path failed to deliver: $out"
[ "$(codex_message_logged)" = "$PAYLOAD" ] || fail "the message read from stdin does not match what was piped in — got: $(codex_message_logged)"
pass "channel-send delivers a backtick-and-\$-bearing message byte-for-byte when given as '-'"

out="$(printf '%s' "$PAYLOAD" | env -u HW_INVOKER_PANE "$AI_SRC" - 2>&1 || true)"
case "$out" in
  *"no HW_INVOKER_PANE"*) pass "ask-invoker accepts '-' and the backtick/\$ payload survives past its own checks" ;;
  *) fail "ask-invoker's '-' path did not reach the normal refusal (payload may have been mangled or rejected): $out" ;;
esac

out="$(printf '%s' "$PAYLOAD" | env -u HW_INVOKER_PANE "$DI_SRC" - 2>&1 || true)"
case "$out" in
  *"no HW_INVOKER_PANE"*) pass "done-invoker accepts '-' and the backtick/\$ payload survives past its own checks" ;;
  *) fail "done-invoker's '-' path did not reach the normal refusal (payload may have been mangled or rejected): $out" ;;
esac

out="$(printf '%s' "$PAYLOAD" | env -u HW_INVOKER_PANE "$HW_SRC" ruling nopane - 2>&1 || true)"
case "$out" in
  *"herdr reports no cwd"*) pass "hw ruling accepts '-' and the backtick/\$ payload survives past its own checks" ;;
  *) fail "hw ruling's '-' path did not reach the normal refusal (payload may have been mangled or rejected): $out" ;;
esac

# ── 3. THE THREE DISTINCT ERRORS, for all four commands ─────────────────────

# 3a. empty stdin
out="$(printf '' | send_codex -)" && rc=0 || rc=$?
[ "$rc" != 0 ] || fail "channel-send accepted an empty stdin message"
case "$out" in *"stdin for '-' produced no bytes"*) pass "channel-send names an empty '-' read distinctly" ;; *) fail "channel-send's empty-stdin refusal is missing or generic: $out" ;; esac

out="$(printf '' | env -u HW_INVOKER_PANE "$AI_SRC" - 2>&1 || true)"
case "$out" in *"stdin for '-' produced no bytes"*) pass "ask-invoker names an empty '-' read distinctly" ;; *) fail "ask-invoker's empty-stdin refusal is missing or generic: $out" ;; esac

out="$(printf '' | env -u HW_INVOKER_PANE "$DI_SRC" - 2>&1 || true)"
case "$out" in *"stdin for '-' produced no bytes"*) pass "done-invoker names an empty '-' read distinctly" ;; *) fail "done-invoker's empty-stdin refusal is missing or generic: $out" ;; esac

out="$(printf '' | env -u HW_INVOKER_PANE "$HW_SRC" ruling nopane - 2>&1 || true)"
case "$out" in *"stdin for '-' produced no bytes"*) pass "hw ruling names an empty '-' read distinctly" ;; *) fail "hw ruling's empty-stdin refusal is missing or generic: $out" ;; esac

# 3b. text given BOTH ways — argv words AND '-'
out="$(printf 'x' | send_codex - "extra words")" && rc=0 || rc=$?
[ "$rc" != 0 ] || fail "channel-send accepted a message given both as argv text and '-'"
case "$out" in *"given twice"*) pass "channel-send refuses a message given both ways, distinctly from the empty case" ;; *) fail "channel-send's double-channel refusal is missing or reuses the empty-stdin wording: $out" ;; esac

out="$(printf 'x' | env -u HW_INVOKER_PANE "$AI_SRC" - "extra words" 2>&1 || true)"
case "$out" in *"given on argv AND as '-'"*) pass "ask-invoker refuses a question given both ways, distinctly from the empty case" ;; *) fail "ask-invoker's double-channel refusal is missing or reuses the empty-stdin wording: $out" ;; esac

out="$(printf 'x' | env -u HW_INVOKER_PANE "$DI_SRC" - "extra words" 2>&1 || true)"
case "$out" in *"given on argv AND as '-'"*) pass "done-invoker refuses a summary given both ways, distinctly from the empty case" ;; *) fail "done-invoker's double-channel refusal is missing or reuses the empty-stdin wording: $out" ;; esac

out="$(printf 'x' | env -u HW_INVOKER_PANE "$HW_SRC" ruling nopane - "extra words" 2>&1 || true)"
case "$out" in *"given on argv AND as '-'"*) pass "hw ruling refuses a correction given both ways, distinctly from the empty case" ;; *) fail "hw ruling's double-channel refusal is missing or reuses the empty-stdin wording: $out" ;; esac

# ── mutants ───────────────────────────────────────────────────────────────────
# M01 — channel-send's '-' branch is dropped, so a lone '-' is treated as
# ordinary argv text (the literal string "-") instead of a stdin read. The
# byte-for-byte assertion above must then fail, proving it exercises real
# stdin plumbing and not just a string comparison somewhere else.
mut="$TMP/m01"; mkdir -p "$mut"; cp "$CS_SRC" "$mut/channel-send"
mutate_anchor 135-M01 "$mut/channel-send" 'if false; then'
chmod +x "$mut/channel-send"
: > "$TMP/codex.log"
PATH="$TMP/cxbin:$PATH" "$mut/channel-send" --require admitted codex fakethread /tmp/fake.sock - \
  <<<"$PAYLOAD" >/dev/null 2>&1 || true
# WITH THE '-' BRANCH DISABLED, "-" IS TREATED AS ORDINARY ARGV TEXT: MESSAGE
# becomes the literal one-character string "-" instead of the piped payload.
# EXACT equality, not substring: the real payload also contains a hyphen
# (deny-repo-writes.py), so a substring check on '-' would pass whether or not
# the mutant ran, which is exactly the vacuous-mutant shape saw_mutant exists
# to refuse.
m01_logged="$(codex_message_logged)"
if [ "$m01_logged" = "-" ]; then
  pass "mutant killed: M01 (saw: the literal argv word '-' sent as the message, instead of the piped payload)"
else
  fail "M01 VACUOUS: expected the disabled stdin branch to send the literal '-', got: $m01_logged"
fi

# M02 — done-invoker's double-channel check is dropped, so text given both ways
# is silently accepted with only the stdin half kept — proving assertion 3b's
# done-invoker arm is load-bearing rather than incidentally true.
mut="$TMP/m02"; mkdir -p "$mut"; cp "$DI_SRC" "$mut/done-invoker"
cp "$ROOT/bin/invoker-common.sh" "$mut/invoker-common.sh"; cp "$ROOT/bin/runenv" "$mut/runenv"
mutate_anchor 135-M02 "$mut/done-invoker" ''
chmod +x "$mut/done-invoker"
out="$(printf 'from stdin' | env -u HW_INVOKER_PANE "$mut/done-invoker" - "from argv" 2>&1 || true)"
saw_mutant "M02" "$out" "no HW_INVOKER_PANE"
