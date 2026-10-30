# shellcheck shell=bash
# engram-serve.sh — IS THE engram serve ON THE PORT THIS MACHINE'S STORE?
#
# SOURCED, never executed: no shebang, not executable, not symlinked into ~/bin.
# Used by bin/hw (registration at launch, renewal at every executor turn end)
# and bin/done-invoker (the report must name an observation that exists).
#
# ── WHY AN ADDRESS IS NOT AN IDENTITY ───────────────────────────────────────
#
# MEASURED 2026-09-28. From 10:47 to ~11:55 the process answering
# 127.0.0.1:7437 was not the operator's `engram serve`. An executor probing OpenCode
# ran `HOME=<scratchpad>/cphome opencode run …` with a copy of
# ~/.config/opencode; its engram plugin (plugins/engram.ts, "Try to start
# engram server if not running") found no serve on the port — the real one was
# down at that moment — and spawned `engram serve` with the SANDBOX HOME. That
# serve opened <scratchpad>/cphome/.engram/engram.db, took the default port and
# outlived its parent (ppid 1). Every `POST /sessions` hw made after that landed
# in the sandbox store and answered "created". The executors' `engram mcp`
# processes open ~/.engram/engram.db directly, never saw the row, and answered
# `unknown_session` to the `session_id` the preamble told them to pass; the
# model retried without it, fell into `manual-save-<label>` (owned by `brain`)
# and was refused. Transcripts: sacar-specs-de-tenants-del-repo 14:22:17Z,
# qr-fiscal-tipo-de-comprobante 14:29:29Z.
#
# SO THE PORT IS CHECKED FOR WHO IT IS. `engram serve` reports `instance_id` on
# GET /health, and that id is the `.instance-id` file in the data dir it opened
# (engram 2.1.0, cmd/engram/main.go `instance-id`, internal/server/server.go
# /health). The data dir the MCP processes open is `ENGRAM_DATA_DIR`, else
# `$HOME/.engram` — engram's own resolution, blank value keeps the default. If
# the two ids differ, the serve is someone else's store and nothing is written
# to it.
#
# NOTHING HERE WRITES THE STORE DIRECTLY, and nothing here starts a serve: a
# serve started from here would carry whatever HOME this process has, which is
# the exact defect above.

engram_serve_url() {
  printf '%s' "${HW_ENGRAM_URL:-http://127.0.0.1:${ENGRAM_PORT:-7437}}"
}

# The store an `engram mcp` started from this environment opens.
engram_store_dir() {
  local d="${ENGRAM_DATA_DIR:-}"
  case "$d" in *[![:space:]]*) printf '%s' "$d" ;; *) printf '%s/.engram' "${HOME:-}" ;; esac
}

# engram_serve_wait [url] [seconds] — give a serve that is starting time to answer.
# 0 once /health answers (whoever's it is: the verdict judges that), 1 after the
# timeout (default HW_ENGRAM_WAIT, else 6s). Returns 0 at once when engram is not
# installed: there is no serve to wait for, and a launch must not pay for it.
# Measured 2026-09-30, first `brain demo`: the serve was starting and the one
# probe ran before it answered, so the session was reported NOT registered.
# The wait is a deadline, not a tick count: on native Windows a curl to a
# closed localhost port takes ~1s by itself, so counted ticks made a 1s wait 5s.
engram_serve_wait() {
  local url="${1:-$(engram_serve_url)}" max="${2:-${HW_ENGRAM_WAIT:-6}}" deadline
  command -v engram >/dev/null 2>&1 || return 0
  case "$max" in ''|*[!0-9]*) max=6 ;; esac
  deadline=$((SECONDS + max))
  while ! curl -sf -m 1 "$url/health" >/dev/null 2>&1; do
    [ "$SECONDS" -lt "$deadline" ] || return 1
    sleep 0.5
  done
  return 0
}

# The probe every HTTP read of the serve goes through (core/internal/engramhealth is its Go twin). A serve under load answers
# /health in 2-3 s (measured 2026-10-29), so a short budget read a slow serve as a dead one: one try waits
# HW_ENGRAM_HEALTH_TIMEOUT seconds (default 10), a transport failure is tried twice, an HTTP status of 400 and over is an
# answer and is not retried.
engram_health_budget() {
  case "${HW_ENGRAM_HEALTH_TIMEOUT:-}" in ''|*[!0-9.]*|.|0|*.*.*) printf 10 ;; *) printf '%s' "$HW_ENGRAM_HEALTH_TIMEOUT" ;; esac
}

# engram_probe_get <url> — 0 with the body in $_eh_body; 1 when it did not answer. Either way $_eh_budget, $_eh_tries and
# $_eh_status (0: no answer came) say how it went; engram_probe_refusal turns them into the line.
engram_probe_get() {
  local raw rc
  _eh_budget="$(engram_health_budget)"; _eh_tries=0; _eh_status=0; _eh_body=""
  while [ "$_eh_tries" -lt 2 ]; do
    _eh_tries=$((_eh_tries + 1))
    raw="$(curl -s -m "$_eh_budget" -w '%{http_code}' "$1" 2>/dev/null)" && rc=0 || rc=$?
    [ "$rc" -eq 0 ] || continue
    _eh_status="${raw: -3}"; _eh_body="${raw%???}"
    [ "$_eh_status" -lt 400 ] && return 0
    return 1
  done
  return 1
}

engram_probe_refusal() {
  local n=tries
  [ "$_eh_tries" -eq 1 ] && n=try
  printf 'no engram serve answered at %s within %s s (%s %s)' "$1" "$_eh_budget" "$_eh_tries" "$n"
  [ "$_eh_status" -eq 0 ] || printf ': it answered HTTP %s' "$_eh_status"
}

# engram_serve_verdict [url] — one line on stdout, and:
#   0  the serve at url is this environment's store (instance ids match)
#   2  a serve answers, and it is ANOTHER store: never write to it
#   3  unverifiable: nothing answers, or no instance id on either side
engram_serve_verdict() {
  local url="${1:-$(engram_serve_url)}" dir want got health
  dir="$(engram_store_dir)"
  # `[ -r ]` and not `2>/dev/null` on the read: the shell reports a failed `<`
  # redirection before that `2>` applies, so a first use (no store yet) printed
  # "No such file or directory" (measured 2026-09-30, empty HOME).
  want=""
  if [ -r "$dir/.instance-id" ]; then want="$(tr -d '[:space:]' < "$dir/.instance-id" 2>/dev/null || true)"; fi
  if ! engram_probe_get "$url/health"; then
    engram_probe_refusal "$url"; return 3
  fi
  health="$_eh_body"
  got="$(printf '%s' "$health" | jq -r '.instance_id // empty' 2>/dev/null || true)"
  if [ -z "$want" ]; then
    printf 'cannot prove the serve at %s is this store: %s/.instance-id is missing' "$url" "$dir"; return 3
  fi
  if [ -z "$got" ]; then
    printf 'cannot prove the serve at %s is this store: its /health names no instance_id' "$url"; return 3
  fi
  if [ "$got" != "$want" ]; then
    printf 'FOREIGN engram serve at %s: instance %s, but this store (%s) is instance %s — a serve started under another HOME or ENGRAM_DATA_DIR holds the port' \
      "$url" "$got" "$dir" "$want"
    return 2
  fi
  printf 'engram serve at %s is this store (%s, instance %s)' "$url" "$dir" "$got"
  return 0
}

# engram_session_post <id> <project> <directory> [url] — create or renew (a
# renewal rewrites the 30-minute runtime lease; engram DOCS.md "Sessions").
# The caller has already verified the serve. 0 only when engram echoes the id.
engram_session_post() {
  local id="$1" project="$2" dir="$3" url="${4:-$(engram_serve_url)}" out
  out="$(curl -sf -m 5 -X POST "$url/sessions" -H 'Content-Type: application/json' \
        -d "$(jq -cn --arg id "$id" --arg p "$project" --arg d "$dir" '{id: $id, project: $p, directory: $d}')" 2>&1)" || {
    printf '%s' "${out:-no answer}"; return 1; }
  printf '%s' "$out" | jq -e --arg id "$id" '.id == $id' >/dev/null 2>&1 || { printf '%s' "$out"; return 1; }
  printf '%s' "$out"
}

# engram_observation_project <id> [url] — the project of a live observation, or
# nothing (missing, soft-deleted, or unreachable). GET /observations/{id}
# excludes deleted rows (store.GetObservation: deleted_at IS NULL).
engram_observation_project() {
  local id="$1" url="${2:-$(engram_serve_url)}"
  engram_probe_get "$url/observations/$id" || return 0
  printf '%s' "$_eh_body" | jq -r '.project // empty' 2>/dev/null || true
}
