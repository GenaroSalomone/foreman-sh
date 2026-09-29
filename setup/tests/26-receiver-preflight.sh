#!/usr/bin/env bash
# channel-send: the exit-6 dead-receiver preflight
#
# WHY THIS FILE EXISTS. The preflight in bin/channel-send is the whole of ACP
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
# Verde no es cobertura. One mutant per behavioural claim above, each judged by
# the arm named beside it. The four marked SURVIVED-2026-08-26 are the exact
# defects that got past this gate when it had no preflight arm at all.
echo "── mutants: every arm must bite ──"

MUTANTS="$TMP/mutants"; mkdir -p "$MUTANTS"

# mutate <name> <old> <new> [<old> <new> ...] -> path to the mutated binary
#
# Takes PAIRS, because one plausible defect is not always one edit: moving the
# preflight after the POST means deleting the call from where it is and adding
# it where it should not be, and a single-substitution engine can only express
# defects that happen to be one substitution wide.
mutate() {
  local name="$1"; shift
  local out="$MUTANTS/$name"
  cp "$BIN" "$out"
  chmod +x "$out"
  python3 - "$out" "$@" <<'PY'
import sys
path, edits = sys.argv[1], sys.argv[2:]
if len(edits) % 2 != 0:
    sys.exit("mutate: edits must come in old/new pairs")
src = open(path, encoding="utf-8").read()
for old, new in zip(edits[0::2], edits[1::2]):
    count = src.count(old)
    if count != 1:
        sys.exit(f"mutation anchor found {count} times, expected 1: {old[:70]!r}")
    src = src.replace(old, new)
open(path, "w", encoding="utf-8").write(src)
PY
  printf '%s' "$out"
}

# judge <name> <mode> <old> <new> [<old> <new> ...]
#
# BASELINE FIRST, ALWAYS. A mutant that "survives" because the machine was
# loaded reads exactly like a real gap in the tests, so the unmutated binary is
# re-run on the same arm immediately beforehand and must produce the contract's
# values. If it does not, this is an ENVIRONMENT failure and is reported as
# one — never as a surviving mutant.
judge() {
  local bin wr wp brc bposts name mode
  name="$1"; mode="$2"
  # The arity is checked BEFORE the shift, not after: `shift 2` returns 1 when
  # fewer than two arguments are left and `set -e` then kills the run with
  # nothing printed — the exact silent-death shape this suite exists to make
  # impossible, and one bin/lint-shell refuses to let past.
  [ $# -ge 4 ] || fail "judge: needs a name, a mode, and at least one old/new pair"
  shift 2
  wr="$(want_rc "$mode")"; wp="$(want_posts "$mode")"

  run "$BIN" "$mode"
  brc="$RC"; bposts="$POSTS"
  { [ "$brc" = "$wr" ] && { [ "$mode" = unreachable ] || [ "$bposts" = "$wp" ]; }; } \
    || fail "ENVIRONMENT: arm $mode is unsound (unmutated binary gave exit $brc, $bposts POSTs; expected $wr, $wp) — mutant $name not judged: $OUT"

  bin="$(mutate "$name" "$@")"
  run "$bin" "$mode"
  if [ "$RC" = "$wr" ] && { [ "$mode" = unreachable ] || [ "$POSTS" = "$wp" ]; }; then
    fail "MUTANT SURVIVED: $name — arm $mode gave exit $RC with $POSTS POST(s), identical to the correct binary"
  fi
  pass "mutant killed: $name (arm $mode: exit $RC/$POSTS POSTs vs correct $wr/$wp)"
}

# SURVIVED-2026-08-26 #1 — the preflight call deleted outright.
judge preflight-call-deleted dead-disabled \
  'await preflightDefaultAgent()
await json(`/session/${session}`, "session-identity")' \
  'await json(`/session/${session}`, "session-identity")'

# SURVIVED-2026-08-26 #2 — the new fact filed under an existing code.
judge exit-6-downgraded-to-5 dead-disabled \
  '    6,
  )
}' \
  '    5,
  )
}'

# SURVIVED-2026-08-26 #3 — the membership test inverted, so healthy receivers
# are condemned. Judged by the HEALTHY arm: the false-positive guard is the
# half an inverted test destroys.
judge membership-inverted healthy \
  'if (resolved.includes(defaultAgent)) return' \
  'if (!resolved.includes(defaultAgent)) return'

# SURVIVED-2026-08-26 #4 — the empty-resolved-set guard dropped, so an endpoint
# that does not report its agents the way we assumed is condemned.
judge empty-set-guard-dropped empty-agents \
  'if (resolved.length === 0) return' \
  'if (false) return'

# The preflight never condemns anything — the other half of #3, and the shape a
# "make the test pass" edit produces.
judge preflight-never-condemns dead-disabled \
  'if (resolved.includes(defaultAgent)) return' \
  'if (true) return'

# The preflight runs AFTER the POST — deleted from the top and re-inserted once
# the message has already been admitted. It STILL EXITS 6, so the exit code
# cannot kill it and only the receiver-side POST witness can; that is precisely
# why the witness exists.
#
# The first version of this mutant moved the call one line down, past the
# session lookup, and SURVIVED — correctly, because the session lookup is not
# the POST and the defect was never actually injected. Recorded here because it
# is the failure mode a mutation suite is most likely to hide: a mutant that
# does not change behaviour proves nothing about the arm judging it.
judge preflight-after-the-post dead-disabled \
  'await preflightDefaultAgent()
await json(`/session/${session}`, "session-identity")' \
  'await json(`/session/${session}`, "session-identity")' \
  '  const deadline = Date.now() + persistenceTimeout' \
  '  await preflightDefaultAgent()
  const deadline = Date.now() + persistenceTimeout'

# The preflight reports a transport failure of its own, masking the layer that
# owns unreachability.
judge preflight-owns-transport unreachable \
  '  } catch {
    return
  }' \
  '  } catch (error) {
    fail("receiver preflight", error instanceof Error ? error.message : String(error), 6)
  }'

# A non-OK response is treated as evidence. Only the 500-with-a-plausible-body
# arm can kill this: a 404 body carries no default_agent, so the next guard
# returns anyway and the mutant is equivalent there.
judge trusts-non-ok-response agent-500-body \
  'if (!configResponse.ok || !agentResponse.ok) return' \
  'if (false) return'

# A receiver with no default_agent configured is condemned.
judge no-default-condemns no-default \
  'if (!defaultAgent) return' \
  'if (false) return'

# THE ONE-GET VERSION: decide from /config alone — "declared and not disabled" —
# instead of membership in the resolved set. This is the shape a single GET can
# support, and the HEALTHY arm is what rejects it: the commonest default,
# `build`, is a builtin declared in no config scope, so a config-only test
# condemns a perfectly healthy receiver. This is the arm that justifies the
# second GET.
judge one-get-config-only healthy \
  'if (resolved.includes(defaultAgent)) return' \
  'if (config?.agent && Object.prototype.hasOwnProperty.call(config.agent, defaultAgent) && config.agent[defaultAgent]?.disable !== true) return'
