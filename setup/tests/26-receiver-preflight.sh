#!/usr/bin/env bash
# channel-send: the exit-6 dead-receiver preflight
#
# WHY THIS FILE EXISTS. The preflight in channel-send (bin/channel-send until go-060 S5, core/channel_send_http.go since) is the whole of ACP
# 0.4.3, and on 2026-08-26 four defects were injected into it — the preflight
# call deleted outright, exit 6 downgraded to 5, the membership test inverted,
# and the empty-resolved-set guard dropped — and ALL FOUR survived every gate
# in both trees. In this one the reason was simple: nothing looked at it.
# `rg 'preflightDefaultAgent|receiver preflight|default_agent' setup/test-hw
# setup/test-channel-send setup/tests/` returned ZERO hits, measured before
# this file existed. The proof that the feature worked lived in an executor's
# $HW_ARTIFACTS and died with the work directory.
#
# So: verde no es cobertura. Every claim the exit-6 contract makes gets an arm,
# and every arm is proved to BITE by re-injecting the defect it is supposed to
# notice. An arm that passes with and without the defect is decoration.
#
# HERMETIC. No `opencode` binary, no network, no fixture outside $TMP: the
# receiver is a node stub whose /config and /agent answers are chosen per
# scenario, which is the only way to produce shapes a real opencode cannot be
# asked for on demand (routes that 404, an empty resolved set, a 500 whose body
# is a well-formed agent list).
#
# THE POST IS WITNESSED RECEIVER-SIDE. "Nothing was sent" is the load-bearing
# half of exit 6, and an exit code cannot prove it — a preflight moved to AFTER
# the POST still exits 6. So every arm asserts the receiver's own POST counter,
# and the dead arms additionally assert that the probe text never entered the
# session.

. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

PORT_FILE="$TMP/preflight-port"
SERVER_PID=""
# _common.sh owns the EXIT trap for $TMP; this one is only for the stub server,
# so it must not replace that trap. Chained explicitly instead.
stop_server() {
  [ -z "$SERVER_PID" ] || kill "$SERVER_PID" 2>/dev/null || true
  [ -z "$SERVER_PID" ] || wait "$SERVER_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap stop_server EXIT

SESSION="ses_preflight"

# ── the receiver ────────────────────────────────────────────────────────────
# One server, many scenarios. The endpoint handed to channel-send is
# http://host/<mode>/<nonce>, so every invocation gets its OWN state even when
# it reuses a mode — a mutant judged against a counter another run already
# incremented is not a judgement.
node --input-type=module - "$PORT_FILE" "$SESSION" <<'NODE' &
import fs from "node:fs"
import http from "node:http"

const portFile = process.argv[2]
const session = process.argv[3]
const states = new Map()

function stateFor(key) {
  if (!states.has(key)) states.set(key, { posts: 0, messages: new Map(), log: [] })
  return states.get(key)
}

function send(res, status, body) {
  res.statusCode = status
  if (body === undefined) return res.end()
  res.setHeader("content-type", "application/json")
  res.end(JSON.stringify(body))
}

// The two answers the preflight reads, per scenario. `build` is deliberately a
// name that appears in NO config `agent` map: it is a builtin, and that is what
// makes a /config-only membership test condemn a healthy receiver.
const AGENTS = [{ name: "build", mode: "primary" }, { name: "plan", mode: "primary" }]
const CONFIG = {
  healthy: [200, { default_agent: "build" }],
  "dead-disabled": [200, { default_agent: "ghost", agent: { ghost: { disable: true } } }],
  "dead-absent": [200, { default_agent: "ghost" }],
  "no-routes": [404, { error: "unknown route" }],
  "no-default": [200, { agent: {} }],
  "empty-agents": [200, { default_agent: "ghost" }],
  "config-500": [500, { error: "boom" }],
  "agent-500-body": [200, { default_agent: "build" }],
  neither: [200, {}],
}
const AGENT = {
  healthy: [200, AGENTS],
  "dead-disabled": [200, AGENTS],
  "dead-absent": [200, AGENTS],
  "no-routes": [404, { error: "unknown route" }],
  "no-default": [200, AGENTS],
  "empty-agents": [200, []],
  "config-500": [200, AGENTS],
  // A 500 whose body is a well-formed, non-empty list that OMITS the configured
  // default. Only a preflight that checks the STATUS survives it; one that
  // trusts the body condemns a healthy receiver.
  "agent-500-body": [500, [{ name: "plan", mode: "primary" }]],
  neither: [200, []],
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://127.0.0.1")
  const parts = url.pathname.split("/").filter(Boolean)
  const [mode, nonce, ...rest] = parts
  const key = `${mode}/${nonce}`
  const path = `/${rest.join("/")}`
  const state = stateFor(key)
  if (path !== "/admin") state.log.push(`${req.method} ${path}`)

  if (path === "/admin") return send(res, 200, { posts: state.posts, log: state.log })

  if (path === "/config") {
    const [status, body] = CONFIG[mode] ?? [404, { error: "unknown mode" }]
    return send(res, status, body)
  }
  if (path === "/agent") {
    const [status, body] = AGENT[mode] ?? [404, { error: "unknown mode" }]
    return send(res, status, body)
  }
  if (path === "/path") {
    return send(res, 200, { config: "/fixture/global", directory: "/fixture/project" })
  }

  if (path === `/session/${session}` && req.method === "GET") return send(res, 200, { id: session })
  // busy is what routes the send through prompt_async, the route the preflight
  // guards; a receiver with no status entry takes the settled route instead.
  if (path === "/session/status") return send(res, 200, { [session]: { type: "busy" } })
  if (path === "/question" || path === "/permission") return send(res, 200, [])

  if (path === `/session/${session}/prompt_async` && req.method === "POST") {
    state.posts += 1
    const chunks = []
    for await (const chunk of req) chunks.push(chunk)
    const payload = JSON.parse(Buffer.concat(chunks).toString())
    state.messages.set(payload.messageID, {
      info: { id: payload.messageID, role: "user" },
      parts: payload.parts,
    })
    return send(res, 204)
  }
  const match = path.match(new RegExp(`^/session/${session}/message/(msg_[A-Za-z0-9_]+)$`))
  if (match && req.method === "GET") {
    const message = state.messages.get(match[1])
    return message ? send(res, 200, message) : send(res, 404, { error: "not found" })
  }
  if (path === `/session/${session}/message` && req.method === "GET") {
    return send(res, 200, Array.from(state.messages.values()))
  }
  send(res, 404, { error: "unknown route" })
})

server.listen(0, "127.0.0.1", () => fs.writeFileSync(portFile, String(server.address().port)))
NODE
SERVER_PID=$!

for _ in $(seq 1 200); do
  [ -s "$PORT_FILE" ] && break
  sleep 0.02
done
[ -s "$PORT_FILE" ] || fail "preflight test server failed to start"
PORT="$(<"$PORT_FILE")"
BASE="http://127.0.0.1:$PORT"

# A port nothing is listening on, for the unreachability arm.
DEAD_PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));p=s.getsockname()[1];s.close();print(p)')"

NONCE=0
OUT=""; RC=0; POSTS=0; LOG=""

# run <binary> <mode> — drives one send and records (RC, OUT, POSTS, LOG).
run() {
  local binary="$1" mode="$2" endpoint
  NONCE=$((NONCE + 1))
  PROBE="PREFLIGHT_PROBE_${mode}_${NONCE}"
  if [ "$mode" = unreachable ]; then
    endpoint="http://127.0.0.1:$DEAD_PORT/unreachable/$NONCE"
  else
    endpoint="$BASE/$mode/$NONCE"
  fi
  RC=0
  set +e
  OUT="$(CHANNEL_PREFLIGHT_TIMEOUT_MS=1000 \
         CHANNEL_PERSISTENCE_TIMEOUT_MS=1500 \
         CHANNEL_DELIVERY_TIMEOUT_MS=3000 \
         "$binary" opencode "$SESSION" "$endpoint" "$PROBE" 2>&1)"
  RC=$?
  set -e
  if [ "$mode" = unreachable ]; then
    POSTS=0; LOG=""
  else
    local admin
    admin="$(curl -fsS --max-time 5 "$BASE/$mode/$NONCE/admin")" || admin='{"posts":-1,"log":[]}'
    POSTS="$(printf '%s' "$admin" | jq -r '.posts')"
    LOG="$(printf '%s' "$admin" | jq -r '.log | join(" ")')"
  fi
}

# The contract, as a table: with the CORRECT binary every mode must produce
# exactly this (exit code, receiver-side POST count). It is a table and not
# nine hand-written blocks so that a new mode cannot be added without stating
# what it is supposed to do.
#
#   mode             rc  posts  why
#   healthy           0    1    a resolvable default is untouched
#   dead-disabled     6    0    default declared AND disabled -> refuse, pre-POST
#   dead-absent       6    0    default declared nowhere      -> refuse, pre-POST
#   no-routes         0    1    404: an older endpoint is not evidence
#   no-default        0    1    no default_agent configured is not evidence
#   empty-agents      0    1    an empty resolved set is not evidence
#   config-500        0    1    a 500 is not evidence
#   agent-500-body    0    1    a 500 with a plausible body is still not evidence
#   neither           0    1    exposing neither value behaves exactly as before
#   unreachable       1    -    unreachability keeps its pre-existing exit code
EXPECT_RC="healthy=0 dead-disabled=6 dead-absent=6 no-routes=0 no-default=0 \
empty-agents=0 config-500=0 agent-500-body=0 neither=0 unreachable=1"

want_rc() {
  local mode="$1" pair
  for pair in $EXPECT_RC; do
    case "$pair" in "$mode="*) printf '%s' "${pair#*=}"; return 0 ;; esac
  done
  fail "no expected exit code declared for mode $mode"
}
want_posts() {
  case "$1" in dead-disabled|dead-absent|unreachable) printf '0' ;; *) printf '1' ;; esac
}

BIN="$ROOT/bin/channel-send"

# ── arms ────────────────────────────────────────────────────────────────────
echo "── the exit-6 contract, one arm per claim ──"

for mode in healthy dead-disabled dead-absent no-routes no-default \
            empty-agents config-500 agent-500-body neither unreachable; do
  run "$BIN" "$mode"
  wr="$(want_rc "$mode")"; wp="$(want_posts "$mode")"
  [ "$RC" = "$wr" ] || fail "$mode: exit $RC, expected $wr: $OUT"
  [ "$mode" = unreachable ] || [ "$POSTS" = "$wp" ] \
    || fail "$mode: receiver recorded $POSTS POST(s), expected $wp"
  pass "preflight: $mode exits $wr with $wp POST(s) at the receiver"
done

# REFUSAL HAPPENS BEFORE THE POST — asserted from the receiver's own request
# log, not from the exit code. This is the arm that a preflight moved after the
# POST cannot pass, and it is the reason exit 6 may promise "nothing landed".
echo "── the refusal is pre-POST, and it names what the operator needs ──"
for mode in dead-disabled dead-absent; do
  run "$BIN" "$mode"
  [ "$RC" = 6 ] || fail "$mode: exit $RC, expected 6"
  [ "$POSTS" = 0 ] || fail "$mode: $POSTS POST(s) reached the receiver; exit 6 promises none"
  case "$LOG" in
    *prompt_async*) fail "$mode: the request log contains a prompt_async: $LOG" ;;
  esac
  # Exactly the two GETs the design pays for, and no session lookup: the
  # refusal must precede every other layer.
  [ "$(printf '%s' "$LOG" | tr ' ' '\n' | grep -c '^/config$' || true)" = 1 ] \
    || fail "$mode: expected exactly one GET /config: $LOG"
  [ "$(printf '%s' "$LOG" | tr ' ' '\n' | grep -c '^/agent$' || true)" = 1 ] \
    || fail "$mode: expected exactly one GET /agent: $LOG"
  case "$LOG" in
    *"/session/$SESSION"*) fail "$mode: the session lookup ran before the refusal: $LOG" ;;
  esac
  # The probe text must never have entered the session.
  body="$(curl -fsS --max-time 5 "$BASE/$mode/$NONCE/session/$SESSION/message")" || body='[]'
  case "$body" in
    *"$PROBE"*) fail "$mode: the probe text IS in the session — the message was delivered" ;;
  esac
  pass "preflight: $mode refuses on two GETs, before the session lookup and the POST"

  # The message is the operator's whole diagnosis, so it is asserted, not
  # assumed: the layer, the missing agent, the endpoint, the resolved count,
  # the pre-POST promise, and the two config scopes that have to agree.
  case "$OUT" in *"receiver preflight"*) ;; *) fail "$mode: not reported at the preflight layer: $OUT" ;; esac
  case "$OUT" in *'"ghost"'*) ;; *) fail "$mode: does not name the missing agent: $OUT" ;; esac
  case "$OUT" in *"127.0.0.1:$PORT"*) ;; *) fail "$mode: does not name the endpoint: $OUT" ;; esac
  case "$OUT" in *"2 agents"*) ;; *) fail "$mode: does not name the resolved agent count: $OUT" ;; esac
  case "$OUT" in *"NO POST was"*) ;; *) fail "$mode: does not state that no POST was sent: $OUT" ;; esac
  case "$OUT" in *"cannot succeed"*) ;; *) fail "$mode: does not say a retry cannot succeed: $OUT" ;; esac
  case "$OUT" in *"config scopes that have to agree"*) ;; *) fail "$mode: does not name the config scopes: $OUT" ;; esac
  case "$OUT" in *"/fixture/global/opencode.json"*) ;; *) fail "$mode: does not name the global scope: $OUT" ;; esac
  case "$OUT" in *"/fixture/project/opencode.json"*) ;; *) fail "$mode: does not name the project scope: $OUT" ;; esac
  # And it must NOT carry the hedge it exists to replace.
  case "$OUT" in
    *"may already be in the receiver"*) fail "$mode: still carries the misleading persistence hedge: $OUT" ;;
  esac
  pass "preflight: $mode names the agent, endpoint, resolved count and both config scopes"
done

# WHY declared+disabled and declared-nowhere are DIFFERENT arms: they are
# distinct diagnoses, and a single membership test that reported one for both
# would send the operator to the wrong config scope.
run "$BIN" dead-disabled
case "$OUT" in *'"disable": true'*) ;; *) fail "dead-disabled: does not say the scope disabled it: $OUT" ;; esac
pass "preflight: a declared-and-disabled default is diagnosed as disabled"
run "$BIN" dead-absent
case "$OUT" in *"no merged config scope declares it at all"*) ;; *) fail "dead-absent: does not say the default is declared nowhere: $OUT" ;; esac
pass "preflight: a default declared nowhere is diagnosed as undeclared"

# Unreachability is the negative control: the preflight may condemn or stay
# silent, never acquire a transport failure of its own.
run "$BIN" unreachable
[ "$RC" = 1 ] || fail "unreachable: exit $RC, expected 1"
case "$OUT" in *"session-identity"*) ;; *) fail "unreachable: not reported by the pre-existing layer: $OUT" ;; esac
case "$OUT" in *"receiver preflight"*) fail "unreachable: the preflight claimed a transport failure: $OUT" ;; esac
pass "preflight: an unreachable endpoint is still exit 1 at session-identity, never 6"

# ── mutants ─────────────────────────────────────────────────────────────────
# THE TEN MUTANTS OF THE BASH PREFLIGHT WENT WITH IT (go-060 S5: bin/channel-send is a shim and its embedded node program is gone).
# Each one edited a line of that program; the preflight is core/channel_send_http.go now, and its decisions carry the anchors
# 26-G01..G09 that setup/tests/1016 kills in process, each judged by the Go twin of the arms above (TestChannelSendOpenCodePreflight,
# core/channel_send_test.go, which also asserts the receiver's POST counter stays 0 on a dead receiver):
#   M01 preflight call deleted -> 26-G01       M02 exit 6 filed under 5 -> 26-G02      M03, M05, M10 membership test -> 26-G03 (one line in Go)
#   M04 empty-resolved-set guard dropped -> 26-G04    M06 preflight after the POST -> the POST witness of the same test, and 26-G06
#   M07 preflight owns transport -> 26-G07     M08 trusts a non-OK response -> 26-G08    M09 no default condemns -> 26-G09
# The arms above still run the whole contract through the shim, byte for byte, against the node stub receiver.
printf 'coverage - 26: the exit-6 contract through the shim, 0 mutants (the 10 bash mutants are retired; their Go anchors 26-G01..G09, no G05, are killed by 1016)\n'
