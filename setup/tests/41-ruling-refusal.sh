#!/usr/bin/env bash
# A ruling is declared by its caller and refuses unsafe mid-work injection.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

BIN="$TMP/bin2"; STUB="$TMP/stub"; mkdir -p "$BIN" "$STUB"
cp "$ROOT"/bin/* "$BIN/" 2>/dev/null || true
LOG="$TMP/log"; SENT="$TMP/sent"; export LOG SENT

cat > "$STUB/herdr" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$LOG"
case "$1 $2" in
  "agent get") printf '{"result":{"agent":{"agent":"opencode","agent_status":"%s","agent_session":{"value":"ses_executor"}}}}\n' "${RULING_STATE:-working}" ;;
  "pane process-info") printf '{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","49999"]}]}}}\n' ;;
  "agent prompt") printf 'sent\n' >> "$SENT"; printf '{"result":{}}\n' ;;
  *) printf '{"result":{}}\n' ;;
esac
STUB
cat > "$BIN/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
printf '%s %s\n' "$1" "${2:-}" >> "$LOG"
if [ "$1" = wait-agent ]; then exit "${RULING_WAIT_RC:-2}"; fi
printf '{"result":{}}\n'
STUB
chmod +x "$STUB/herdr" "$BIN/herdr-rpc"

run_ruling() {
  : > "$LOG"; : > "$SENT"
  PATH="$STUB:$PATH" HW_INVOKER_WAIT_MS=300 \
    RULING_STATE="${RULING_STATE:-working}" RULING_WAIT_RC="${RULING_WAIT_RC:-2}" \
    "$1" --ruling herdr wT:p1 - "a ruling" 2>&1
}

run_redirect() {
  : > "$LOG"; : > "$SENT"
  PATH="$STUB:$PATH" HW_INVOKER_WAIT_MS=300 \
    RULING_STATE=working RULING_WAIT_RC=2 \
    "$1" herdr wT:p1 - "generic redirect" 2>&1
}

ruling_out="$(run_ruling "$BIN/channel-send")" && ruling_rc=0 || ruling_rc=$?
[ "$ruling_rc" = 1 ] || fail "C01 working ruling expected refusal exit 1, got $ruling_rc: $ruling_out"
case "$ruling_out" in
  # THE CONTINUATION IS WHAT THIS ARM JUDGES. Wording that reads as "finish or
  # `hw done`" risks an executor closing its whole session over a small
  # correction, so the needles are the three exits themselves rather than a
  # sentence naming only some of them. `hw done` must appear as the thing NOT
  # to do -- a refusal that stops naming it is a regression this arm has to
  # catch.
  *"--ruling REFUSED before delivery"*"NOTHING was sent"*"hw ruling"*"hw next"*"DO NOT run"*"hw done"*"Do not substitute --report"*)
    pass "C01 a declared ruling refuses immediately, names the queue first, and names hw done as the trap" ;;
  *) fail "C01 ruling refusal omitted its reason or continuation: $ruling_out" ;;
esac
[ "$(grep -c '^wait-agent ' "$LOG" || true)" = 0 ] || fail "C01 ruling entered the ordinary wait instead of refusing"
[ ! -s "$SENT" ] || fail "C01 ruling refusal injected text"

# The marker is narrow. An undeclared generic redirect retains the ordinary
# gate; making every redirect refuse would change a different contract.
redirect_out="$(run_redirect "$BIN/channel-send")" && redirect_rc=0 || redirect_rc=$?
[ "$redirect_rc" = 1 ] || fail "C02 busy generic redirect expected its existing gate refusal"
[ "$(grep -c '^wait-agent ' "$LOG" || true)" -ge 1 ] || fail "C02 generic redirect no longer used its gate"
case "$redirect_out" in *"--ruling REFUSED"*) fail "C02 generic redirect was misclassified as a ruling" ;; *) pass "C02 generic redirects remain gated and are never inferred from message text" ;; esac

# Native OpenCode is the route a real challenge reply commonly resolves. Its
# own /session/status is the structural liveness source, and refusal precedes
# prompt_async. Count POSTs rather than trusting prose.
PORT_FILE="$TMP/port"; SERVER_PID=""
node --input-type=module - "$PORT_FILE" <<'NODE' &
import fs from "node:fs"
import http from "node:http"
let posts = 0
const target = "ses_executor"
const server = http.createServer((req, res) => {
  const send = (code, body) => { res.statusCode = code; res.setHeader("content-type", "application/json"); res.end(JSON.stringify(body)) }
  if (req.url === "/config") return send(200, {})
  if (req.url === "/agent") return send(200, [])
  if (req.url === `/session/${target}`) return send(200, { id: target })
  if (req.url === "/session/status") return send(200, { [target]: { type: "busy" } })
  if (req.url === "/question" || req.url === "/permission") return send(200, [])
  if (req.url === "/admin") return send(200, { posts })
  if (req.url === `/session/${target}/prompt_async`) { posts += 1; return send(204, undefined) }
  return send(404, {})
})
server.listen(0, "127.0.0.1", () => fs.writeFileSync(process.argv[2], String(server.address().port)))
NODE
SERVER_PID=$!
for _ in $(seq 1 100); do [ -s "$PORT_FILE" ] && break; sleep 0.02; done
[ -s "$PORT_FILE" ] || fail "C03 native ruling server did not start"
PORT="$(cat "$PORT_FILE")"
native_out="$(CHANNEL_DELIVERY_TIMEOUT_MS=500 "$BIN/channel-send" --ruling --require processed --id ruling-1 \
  opencode ses_executor "http://127.0.0.1:$PORT" "native ruling" 2>&1)" && native_rc=0 || native_rc=$?
[ "$native_rc" = 1 ] || fail "C03 native busy ruling expected exit 1, got $native_rc: $native_out"
case "$native_out" in *"ruling safety"*"--ruling REFUSED"*"hw next"*) pass "C03 native OpenCode refuses the same unsafe ruling before POST" ;; *) fail "C03 native refusal was not actionable: $native_out" ;; esac
posts="$(curl -fsS "http://127.0.0.1:$PORT/admin" | jq -r .posts)"
[ "$posts" = 0 ] || fail "C03 native ruling made $posts prompt_async POST(s)"

# Run the real challenge caller with transport replaced only by a recorder. The
# opaque ruling text is never inspected; ask-invoker passes a structural intent
# argument and the generated reply command carries --ruling.
HARNESS="$TMP/harness"; mkdir -p "$HARNESS/bin" "$HARNESS/work/.hw/run"
cp "$ROOT/bin/ask-invoker" "$ROOT/bin/invoker-common.sh" "$ROOT/bin/runenv" "$HARNESS/bin/"
cat > "$HARNESS/bin/channel-send" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$DELIVERY_RECORD"
exit 0
STUB
cat > "$HARNESS/bin/herdr-rpc" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
chmod +x "$HARNESS/bin/channel-send" "$HARNESS/bin/herdr-rpc"

run_challenge() {
  local ask="$1" work="$2" record="$3"
  mkdir -p "$work/.hw/run"
  PATH="$STUB:$PATH" DELIVERY_RECORD="$record" \
    HW_EXECUTOR_VENDOR=claude HW_INVOKER_PANE=wB:p1 HERDR_PANE_ID=wT:p1 \
    HW_TASK=probe HW_PROJECT=setup HW_WORKDIR="$work" HW_RUN=run \
    "$ask" --challenge "brief forbids X :: runtime proves Y" >/dev/null 2>&1
}
run_challenge "$HARNESS/bin/ask-invoker" "$HARNESS/work" "$TMP/challenge-delivery"
case "$(cat "$TMP/challenge-delivery")" in *"--ruling"*"--reply-hold"*"<your ruling>"*) pass "C04 the known challenge-reply caller declares ruling intent and its pending hold explicitly" ;; *) fail "C04 challenge reply command lacks --ruling/--reply-hold: $(cat "$TMP/challenge-delivery")" ;; esac

# The refusal is a boundary, not a ban. At a clean idle/done target the same
# explicit ruling follows the ordinary receipt path and is delivered once.
idle_out="$(RULING_STATE=idle RULING_WAIT_RC=0 run_ruling "$BIN/channel-send")" && idle_rc=0 || idle_rc=$?
[ "$idle_rc" = 0 ] || fail "C05 idle ruling expected delivery exit 0, got $idle_rc: $idle_out"
[ "$(grep -c '^sent$' "$SENT" || true)" = 1 ] || fail "C05 idle ruling did not deliver exactly once"
[ "$(grep -c '^wait-agent ' "$LOG" || true)" = 1 ] || fail "C05 idle ruling bypassed its ordered boundary check"
pass "C05 a ruling at a clean idle boundary is delivered exactly once"

# MUTATION ARMS RUN UNCONDITIONALLY. They used to sit behind
# `if [ "${RULING_MUTATION_TEST:-0}" = 1 ]`, which `setup/test-hw` never set, so a 909-ok run
# exercised none of them. The stated reason was "committed bytes only"; the arms
# copy from $ROOT, the working tree, like every ungated arm here. See
# setup/tests/46-mutation-arms-are-not-gated.sh and setup/mutation-coverage.
  # M01 removes the immediate refusal. The same working target then reaches the
  # ordinary wait, proving the test distinguishes the policy from a timeout.
  M1="$TMP/m1"; mkdir -p "$M1"; cp "$BIN"/* "$M1/" 2>/dev/null || true
  mutate_anchor 41-M01 "$M1/channel-send" 'if false; then'
  chmod +x "$M1/channel-send"; cp "$BIN/herdr-rpc" "$M1/herdr-rpc"
  mout="$(run_ruling "$M1/channel-send" || true)"
  [ "$(grep -c '^wait-agent ' "$LOG" || true)" -ge 1 ] || fail "M01 SURVIVED: ruling still refused before the wait"
  pass "mutant killed: M01 sends a declared ruling into the ordinary redirect gate"

  # M02 makes ruling the default. A generic redirect is then refused before its
  # ordinary gate, proving C02 is about preserved behavior rather than prose.
  M2="$TMP/m2"; mkdir -p "$M2"; cp "$BIN"/* "$M2/" 2>/dev/null || true
  mutate_anchor 41-M02 "$M2/channel-send" 'IS_RULING=1'
  chmod +x "$M2/channel-send"; cp "$BIN/herdr-rpc" "$M2/herdr-rpc"
  mredirect="$(run_redirect "$M2/channel-send" || true)"
  [ "$(grep -c '^wait-agent ' "$LOG" || true)" = 0 ] || fail "M02 SURVIVED: generic redirect still used its ordinary gate"
  case "$mredirect" in *"--ruling REFUSED"*) pass "mutant killed: M02 classifies every generic redirect as a ruling" ;; *) fail "M02 environment did not expose ruling-by-default: $mredirect" ;; esac

  # M03 removes the native busy pre-POST refusal. The same server then records
  # prompt_async, which C03 explicitly forbids.
  M3="$TMP/m3"; mkdir -p "$M3"; cp "$BIN"/* "$M3/" 2>/dev/null || true
  mutate_anchor 41-M03 "$M3/channel-send" 'if (false && !holdProven && (pending.length || (status && ["busy", "retry"].includes(status.type)))) {'
  chmod +x "$M3/channel-send"
  CHANNEL_DELIVERY_TIMEOUT_MS=500 CHANNEL_PERSISTENCE_TIMEOUT_MS=100 \
    "$M3/channel-send" --ruling --require processed --id ruling-mutant \
      opencode ses_executor "http://127.0.0.1:$PORT" "native mutant ruling" >/dev/null 2>&1 || true
  mposts="$(curl -fsS "http://127.0.0.1:$PORT/admin" | jq -r .posts)"
  [ "$mposts" -gt 0 ] || fail "M03 SURVIVED: native busy ruling still made no prompt_async POST"
  pass "mutant killed: M03 removes native OpenCode pre-POST ruling refusal"

  # M04 disconnects the known challenge caller from the marker. The resulting
  # delivery still succeeds, but its reply command silently becomes an answer.
  M4="$TMP/m4"; mkdir -p "$M4"; cp "$HARNESS/bin"/* "$M4/"
  mutate_anchor 41-M04 "$M4/ask-invoker" 'REPLY_COMMAND="$(invoker_reply_command "$ENVELOPE_ID:reply" answer "$PENDING_REPLY")"'
  chmod +x "$M4/ask-invoker"
  run_challenge "$M4/ask-invoker" "$TMP/m4-work" "$TMP/m4-delivery"
  # KILLED BY WHAT THE MUTANT SAID. `invoker_reply_command` prints two different
  # commands, not one command with a flag missing: intent `ruling` prints
  # `--ruling --reply-hold … "<your ruling>"`, intent `answer` prints
  # `--report --reply-hold … "<your answer>"` (bin/invoker-common.sh). So the
  # downgraded reply has text of its OWN — the mutant's delivery carries
  # `--report --reply-hold … "<your answer>"`. The old shape was
  # `*"--ruling"*) fail ;; *) pass`, which certifies the kill from the ABSENCE of
  # the flag: a challenge that never got as far as printing a reply command has
  # no `--ruling` either and passed it too.
  case "$(cat "$TMP/m4-delivery")" in
    *'"<your answer>"'*) pass "mutant killed: M04 downgrades the challenge reply to an ordinary answer" ;;
    *"--ruling"*) fail "M04 SURVIVED: challenge still declared ruling" ;;
    *) fail "M04 VACUOUS: the delivery carries neither reply command, so invoker_reply_command may never have run — got: $(cat "$TMP/m4-delivery" | tail -3 | tr '\n' ' ')" ;;
  esac

  # M05 rejects even a clean idle target, turning the safety boundary into a
  # blanket ban. C05 requires one ordered delivery and kills that regression.
  M5="$TMP/m5"; mkdir -p "$M5"; cp "$BIN"/* "$M5/" 2>/dev/null || true
  mutate_anchor 41-M05 "$M5/channel-send" 'never) return 0 ;;'
  chmod +x "$M5/channel-send"; cp "$BIN/herdr-rpc" "$M5/herdr-rpc"
  midle="$(RULING_STATE=idle RULING_WAIT_RC=0 run_ruling "$M5/channel-send" || true)"
  [ ! -s "$SENT" ] || fail "M05 SURVIVED: idle ruling still delivered"
  case "$midle" in *"--ruling REFUSED"*) pass "mutant killed: M05 turns the clean idle boundary into a blanket refusal" ;; *) fail "M05 environment did not expose idle refusal: $midle" ;; esac

  printf 'mapping - C01↔M01 Herdr working refusal; C02↔M02 generic redirect preservation; C03↔M03 native pre-POST refusal; C04↔M04 challenge wiring; C05↔M05 clean-boundary delivery\n'
  printf 'coverage - 5 behavior claims, 5 dedicated ruling mutants killed\n'

[ -z "$SERVER_PID" ] || { kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; }
