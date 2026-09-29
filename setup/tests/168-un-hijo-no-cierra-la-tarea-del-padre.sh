#!/usr/bin/env bash
# A `claude -p` started by an executor inherits HW_RUN, and its turn end must
# not be judged as the executor's: the Stop hook acts only for the session hw
# registered for the run.
#
#     bash setup/tests/168-un-hijo-no-cierra-la-tarea-del-padre.sh
#
# The real hook runs; `hw` beside it is a stub that records being called, which
# is the whole decision under test (the verdict itself is 43's subject).
# `SUBJECT_HOOK` runs the same assertions against another hook, for old/new.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BIN="$TMP/bin"; mkdir -p "$BIN" "$TMP/work/.hw/run"
cp "${SUBJECT_HOOK:-$ROOT/bin/hw-stop-hook.sh}" "$BIN/hw-stop-hook.sh"
cat > "$BIN/hw" <<EOF
#!/usr/bin/env bash
printf 'called %s\n' "\$*" >> "$TMP/hw.calls"
printf '{"decision":"block","reason":"run done-invoker"}\n'
EOF
chmod +x "$BIN/hw"
RECEIPT="$TMP/work/.hw/run/receipt.jsonl"

stop() {  # $1 = payload session_id
  : > "$TMP/hw.calls"
  printf '{"session_id":"%s","hook_event_name":"Stop","last_assistant_message":"x"}' "$1" \
    | HERDR_ENV=1 HERDR_SOCKET_PATH=/nonexistent HERDR_PANE_ID=wT:p1 \
      HW_TASK=probe HW_RUN=run HW_WORKDIR="$TMP/work" \
      bash "$BIN/hw-stop-hook.sh" stop
}

printf '{"key":"session_id","value":"executor-session"}\n' > "$RECEIPT"

out="$(stop child-session)"
[ ! -s "$TMP/hw.calls" ] && [ -z "$out" ] \
  || fail "hook: a child session inheriting HW_RUN was told to close the parent's task: $out"
pass "hook: a nested session's turn end is not judged as the executor's"

out="$(stop executor-session)"
[ -s "$TMP/hw.calls" ] && case "$out" in *done-invoker*) true ;; *) false ;; esac \
  || fail "hook: the registered executor session no longer gets its turn-end verdict"
pass "hook: the registered executor session still gets its turn-end verdict"

# No registered session (herdr published none in time): the old behaviour, so
# the closing is not silenced for every such run.
printf '{"key":"session_id","value":"none — claude published no agent_session within 15s"}\n' > "$RECEIPT"
stop anything >/dev/null
[ -s "$TMP/hw.calls" ] || fail "hook: a run with no registered session lost its turn-end verdict"
pass "hook: with no registered session the hook acts as before"
