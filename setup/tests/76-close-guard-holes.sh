#!/usr/bin/env bash
# Namespace/source verification and lint-shell's real errexit shapes.
set -euo pipefail

. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

live_pid=""
holder=""
trap '[ -z "${live_pid:-}" ] || kill "$live_pid" 2>/dev/null || true; [ -z "${holder:-}" ] || kill "$holder" 2>/dev/null || true; rm -rf "$TMP"' EXIT

cat > "$TMP/fakeab" <<'ABEOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKEAB_LOG"
case "$*" in
  *"session list"*)
    # Model the measured destructive-read contract without invoking the CLI.
    if [ -n "${FAKEAB_PRUNE_DIR:-}" ]; then rm -f "$FAKEAB_PRUNE_DIR"/*.pid; fi
    if [ -n "${FAKEAB_STUCK:-}" ]; then printf 'Active sessions:\n  stuck-one\n'; else printf 'No active sessions\n'; fi
    ;;
  *) printf 'Browser closed\n' ;;
esac
ABEOF
chmod +x "$TMP/fakeab"
export FAKEAB_LOG="$TMP/fakeab.log"

run_close() {
  set +e
  out="$("$@" 2>&1)"
  rc=$?
  set -e
}

# Missing/empty namespaces fail before the fake CLI can touch its default.
: > "$FAKEAB_LOG"
run_close env -u AGENT_BROWSER_NAMESPACE BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" \
  "$ROOT/bin/browser-close" --settle-ms 0
[ "$rc" = 1 ] || fail "browser-close: missing namespace exited $rc, expected 1"
case "$out" in *"no AGENT_BROWSER_NAMESPACE"*) pass "browser-close: an unset namespace refuses before the default namespace can be closed" ;; *) fail "browser-close: missing namespace said: $out" ;; esac
[ ! -s "$FAKEAB_LOG" ] || fail "browser-close: missing namespace called the fake CLI"

: > "$FAKEAB_LOG"
run_close env AGENT_BROWSER_NAMESPACE= BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" \
  "$ROOT/bin/browser-close" --settle-ms 0
[ "$rc" = 1 ] || fail "browser-close: empty namespace exited $rc, expected 1"
[ ! -s "$FAKEAB_LOG" ] || fail "browser-close: empty namespace called the fake CLI"
pass "browser-close: an empty namespace also refuses before CLI activity"

# The pinned upstream source falls back beyond HOME through platform APIs. This
# shell deliberately refuses that unsupported branch rather than guessing it.
: > "$FAKEAB_LOG"
run_close env -u AGENT_BROWSER_SOCKET_DIR -u XDG_RUNTIME_DIR -u HOME \
  AGENT_BROWSER_NAMESPACE=hw-fixture BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" \
  "$ROOT/bin/browser-close" --settle-ms 0
[ "$rc" = 1 ] || fail "browser-close: source-unavailable environment exited $rc, expected 1"
case "$out" in *"cannot establish agent-browser's sidecar root"*) ;; *) fail "browser-close: source-unavailable diagnostic: $out" ;; esac
[ ! -s "$FAKEAB_LOG" ] || fail "browser-close: source-unavailable environment called the fake CLI"
pass "browser-close: only the unsupported platform-home fallback refuses"

sidecars="$TMP/sidecars"
mkdir -p "$sidecars/namespaces/hw-fixture/run"
: > "$FAKEAB_LOG"
run_close env AGENT_BROWSER_NAMESPACE=hw-fixture AGENT_BROWSER_SOCKET_DIR="$sidecars" \
  XDG_RUNTIME_DIR="$TMP/wrong-runtime" HOME="$TMP/wrong-home" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --session chosen --settle-ms 0
[ "$rc" = 0 ] || fail "browser-close: explicit-root clean close exited $rc: $out"
case "$out" in *"no profile was checked"*) ;; *) fail "browser-close: profileless output hid its scope: $out" ;; esac
case "$(<"$FAKEAB_LOG")" in *"--session chosen close"*"session list"*) ;; *) fail "browser-close: chosen session did not reach both fake operations" ;; esac
pass "browser-close: explicit socket root wins and a profileless success names what was not checked"

# Namespace sanitation is source-defined: lowercase, collapse separator runs,
# and map other ASCII characters to one hyphen.
sanitized_root="$TMP/sanitized"
mkdir -p "$sanitized_root/namespaces/worktree-one/run"
run_close env AGENT_BROWSER_NAMESPACE='Worktree:  One--' AGENT_BROWSER_SOCKET_DIR="$sanitized_root" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --settle-ms 0
[ "$rc" = 0 ] || fail "browser-close: source-defined namespace sanitation exited $rc: $out"
pass "browser-close: namespace sanitation matches pinned agent-browser source"

mkdir -p "$TMP/runtime/agent-browser/namespaces/hw-xdg/run"
run_close env -u AGENT_BROWSER_SOCKET_DIR AGENT_BROWSER_NAMESPACE=hw-xdg XDG_RUNTIME_DIR="$TMP/runtime" \
  HOME="$TMP/wrong-home" BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" \
  "$ROOT/bin/browser-close" --settle-ms 0
[ "$rc" = 0 ] || fail "browser-close: XDG root exited $rc: $out"

mkdir -p "$TMP/home/.agent-browser/namespaces/hw-home/run"
run_close env -u AGENT_BROWSER_SOCKET_DIR -u XDG_RUNTIME_DIR AGENT_BROWSER_NAMESPACE=hw-home HOME="$TMP/home" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --settle-ms 0
[ "$rc" = 0 ] || fail "browser-close: HOME root exited $rc: $out"
pass "browser-close: XDG and HOME fallbacks follow pinned source precedence"

# Snapshot a real harmless process before the fake list prunes its PID file.
/bin/sleep 30 & live_pid=$!
printf '%s\n' "$live_pid" > "$sidecars/namespaces/hw-fixture/run/chosen.pid"
run_close env AGENT_BROWSER_NAMESPACE=hw-fixture AGENT_BROWSER_SOCKET_DIR="$sidecars" \
  FAKEAB_PRUNE_DIR="$sidecars/namespaces/hw-fixture/run" BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" \
  "$ROOT/bin/browser-close" --session chosen --settle-ms 0
kill "$live_pid" 2>/dev/null || true
wait "$live_pid" 2>/dev/null || true
live_pid=""
[ "$rc" = 1 ] || fail "browser-close: live PID followed by pruning exited $rc, expected 1: $out"
case "$out" in *"independent sidecar snapshot"*) ;; *) fail "browser-close: live PID diagnostic: $out" ;; esac
[ ! -e "$sidecars/namespaces/hw-fixture/run/chosen.pid" ] || fail "browser-close: fake list did not model PID pruning"
pass "browser-close: a live external PID observed before a pruning list prevents exit 0"

printf '999999\n' > "$sidecars/namespaces/hw-fixture/run/stale.pid"
run_close env AGENT_BROWSER_NAMESPACE=hw-fixture AGENT_BROWSER_SOCKET_DIR="$sidecars" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --settle-ms 0
rm -f "$sidecars/namespaces/hw-fixture/run/stale.pid"
[ "$rc" = 0 ] || fail "browser-close: dead PID exited $rc: $out"
pass "browser-close: a dead stale PID is observed but does not claim a live daemon"

# The standalone dashboard has its own stop lifecycle and is not a session
# closed by close --all. Its live sidecar therefore cannot disprove that close.
/bin/sleep 30 & live_pid=$!
printf '%s\n' "$live_pid" > "$sidecars/namespaces/hw-fixture/run/dashboard.pid"
run_close env AGENT_BROWSER_NAMESPACE=hw-fixture AGENT_BROWSER_SOCKET_DIR="$sidecars" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --settle-ms 0
kill "$live_pid" 2>/dev/null || true
wait "$live_pid" 2>/dev/null || true
live_pid=""
rm -f "$sidecars/namespaces/hw-fixture/run/dashboard.pid"
[ "$rc" = 0 ] || fail "browser-close: standalone dashboard PID exited $rc: $out"
pass "browser-close: close --all ignores the standalone dashboard sidecar"

# An inaccessible/malformed root cannot be converted into proof of absence.
: > "$TMP/root-is-a-file"
run_close env AGENT_BROWSER_NAMESPACE=hw-fixture AGENT_BROWSER_SOCKET_DIR="$TMP/root-is-a-file" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --settle-ms 0
[ "$rc" = 1 ] || fail "browser-close: failed sidecar inspection exited $rc, expected 1"
case "$out" in *"independent sidecar observation failed"*) pass "browser-close: failed external inspection cannot produce a verified-success claim" ;; *) fail "browser-close: inspection failure diagnostic: $out" ;; esac

# Existing registered-session exit 1 and profile-holder exit 3 keep priority.
export FAKEAB_STUCK=1
run_close env AGENT_BROWSER_NAMESPACE=hw-fixture AGENT_BROWSER_SOCKET_DIR="$sidecars" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --settle-ms 0
unset FAKEAB_STUCK
[ "$rc" = 1 ] || fail "browser-close: registered session exited $rc, expected 1"
case "$out" in *"DO NOT relaunch"*) ;; *) fail "browser-close: registered-session diagnostic: $out" ;; esac
pass "browser-close: a session surviving list still exits 1 and forbids relaunch"

bash -c "exec -a 'chrome --user-data-dir=$TMP/held' /bin/sleep 30" & holder=$!
run_close env AGENT_BROWSER_NAMESPACE=hw-fixture AGENT_BROWSER_SOCKET_DIR="$sidecars" \
  BROWSER_CLOSE_AGENT_BROWSER="$TMP/fakeab" "$ROOT/bin/browser-close" --profile "$TMP/held" --settle-ms 0
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
holder=""
[ "$rc" = 3 ] || fail "browser-close: held profile exited $rc, expected 3: $out"
case "$out" in *"will not kill it"*) pass "browser-close: a profile holder remains exit 3 and is never killed by browser-close" ;; *) fail "browser-close: held-profile diagnostic: $out" ;; esac

# Every original command class remains a detector, in quoted assignments. The
# exact regression adds unquoted and middle-pipeline forms under active options.
unsafe="$TMP/lint-unsafe.sh"
cat > "$unsafe" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
GREP="$(grep absent input)"
FIND="$(find missing)"
RG="$(rg absent input)"
JQ="$(jq -r . missing.json)"
HERDR="$(herdr agent list)"
ENGRAM="$(engram status)"
GIT="$(git rev-parse HEAD)"
LS="$(ls missing)"
PGREP="$(pgrep Chrome)"
AWK="$(awk 'BEGIN {exit 1}')"
SED="$(sed -n 1p missing)"
UNQUOTED=$(rg absent input)
COUNT=$(ps ax | rg Chrome | wc -l)
ADJACENT="$(rg absent input)"
printf ok || true
PLURAL="$([ "${COUNT:-0}" = 1 ] && printf '')"
case "${1:-}" in
  --unsafe) shift 2 ;;
esac
EOF
set +e
lint_out="$("$ROOT/bin/lint-shell" "$unsafe" 2>&1)"
lint_rc=$?
set -e
[ "$lint_rc" = 1 ] || fail "lint-shell: unsafe fixture exited $lint_rc: $lint_out"
[ "$(printf '%s\n' "$lint_out" | grep -c 'assignment may inherit' || true)" = 14 ] \
  || fail "lint-shell: expected 14 assignment findings: $lint_out"
case "$lint_out" in *":18: \$( [ … ]"*) ;; *) fail "lint-shell: conditional substitution was not classified: $lint_out" ;; esac
case "$lint_out" in *":20: shift 2 returns"*) ;; *) fail "lint-shell: unsafe shift was not classified: $lint_out" ;; esac
pass "lint-shell: every original class plus quoted, unquoted, middle-pipeline, conditional, and shift hazards are detected"

safe="$TMP/lint-safe.sh"
cat > "$safe" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
OUTER="$(rg absent input)" || outer_rc=$?
INNER="$(rg absent input || true)"
CONTINUED="$(herdr agent list)" \
  || continued_rc=$?
if CONDITIONAL="$(git rev-parse HEAD)"; then :; fi
set +o pipefail
NO_PIPEFAIL=$(ps ax | rg Chrome | wc -l)
set -o pipefail
AWK_FILTER="$(printf '%s\n' value | awk '{print $1}')"
SED_FILTER="$(printf '%s\n' value | sed -n 1p)"
set +e
NO_ERREXIT="$(jq -r . missing.json)"
set -e
PLURAL="$([ "${COUNT:-0}" = 1 ] && printf '' || printf 's')"
masked_status() { local LOCAL_VALUE="$(rg absent input)"; printf '%s' "$LOCAL_VALUE"; }
LITERAL='EXAMPLE=$(rg absent input)'
grep -E 'EXAMPLE=\$\(rg' /dev/null || true
cat <<'HEREDOC'
HEREDOC_ASSIGNMENT="$(git rev-parse HEAD)"
HEREDOC
case "${1:-}" in
  --safe)
    _need_val "$@"
    case "$2" in *) : ;; esac
    shift 2
    ;;
esac
EOF
safe_out="$("$ROOT/bin/lint-shell" "$safe" 2>&1)" || fail "lint-shell: handled/context fixture was noisy: $safe_out"
"$ROOT/bin/lint-shell" "$ROOT/bin/lint-shell" >/dev/null || fail "lint-shell: self diagnostics were treated as executing code"
pass "lint-shell: real handlers, inactive options, conditional context, literals, regex examples, heredocs, and guarded shifts stay quiet"

run_lint_hazard() {
  local fixture="$1" label="$2"
  set +e
  probe_lint_out="$("$ROOT/bin/lint-shell" "$fixture" 2>&1)"
  probe_lint_rc=$?
  set -e
  [ "$probe_lint_rc" = 1 ] || fail "lint-shell: $label fixture exited $probe_lint_rc: $probe_lint_out"
  [ "$(printf '%s\n' "$probe_lint_out" | grep -c 'assignment may inherit' || true)" = 1 ] \
    || fail "lint-shell: $label fixture did not produce exactly one assignment finding: $probe_lint_out"
}

run_runtime_abort() {
  local fixture="$1" label="$2"
  set +e
  probe_runtime_out="$(bash "$fixture" 2>&1)"
  probe_runtime_rc=$?
  set -e
  [ "$probe_runtime_rc" = 1 ] || fail "lint-shell: $label Bash runtime exited $probe_runtime_rc, expected 1: $probe_runtime_out"
  case "$probe_runtime_out" in *SURVIVED*) fail "lint-shell: $label Bash runtime reached its survival marker" ;; esac
}

cat > "$TMP/option-function.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
unused() {
  set +e
  set +o pipefail
}
VALUE="$(printf 'input\n' | grep -q ZZNOPE | wc -l)"
printf 'SURVIVED\n'
EOF
run_lint_hazard "$TMP/option-function.sh" "uncalled function option scope"
run_runtime_abort "$TMP/option-function.sh" "uncalled function option scope"
pass "lint-shell: options changed in an uncalled multiline function do not leak into its parent"

cat > "$TMP/option-subshell.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
(
  set +e
  set +o pipefail
)
VALUE="$(printf 'input\n' | grep -q ZZNOPE | wc -l)"
printf 'SURVIVED\n'
EOF
run_lint_hazard "$TMP/option-subshell.sh" "subshell option scope"
run_runtime_abort "$TMP/option-subshell.sh" "subshell option scope"
pass "lint-shell: options changed in a subshell do not leak into its parent"

cat > "$TMP/quoted-heredoc-prose.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' 'usage: cat <<EOF'
VALUE="$(grep -q ZZNOPE /dev/null)"
printf 'SURVIVED\n'
EOF
run_lint_hazard "$TMP/quoted-heredoc-prose.sh" "quoted heredoc prose"
run_runtime_abort "$TMP/quoted-heredoc-prose.sh" "quoted heredoc prose"
pass "lint-shell: a quoted heredoc example does not hide following executable code"

cat > "$TMP/hyphenated-heredoc.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat <<'END-DATA'
BODY="$(grep -q ZZNOPE /dev/null)"
END-DATA
VALUE="$(grep -q ZZNOPE /dev/null)"
printf 'SURVIVED\n'
EOF
run_lint_hazard "$TMP/hyphenated-heredoc.sh" "hyphenated heredoc delimiter"
run_runtime_abort "$TMP/hyphenated-heredoc.sh" "hyphenated heredoc delimiter"
pass "lint-shell: a quoted hyphenated heredoc masks only its body and closes on its full delimiter"

cat > "$TMP/or-false.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
VALUE="$(grep -q ZZNOPE /dev/null || false)"
printf 'SURVIVED\n'
EOF
run_lint_hazard "$TMP/or-false.sh" "internal or-false handler"
run_runtime_abort "$TMP/or-false.sh" "internal or-false handler"
pass "lint-shell: an internal || false does not prove a successful substitution"

cat > "$TMP/adjacent-redirection.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
VALUE="$(grep</dev/null -q ZZNOPE)"
printf 'SURVIVED\n'
EOF
run_lint_hazard "$TMP/adjacent-redirection.sh" "adjacent command redirection"
run_runtime_abort "$TMP/adjacent-redirection.sh" "adjacent command redirection"
pass "lint-shell: a shell redirection is recognized as a command-word boundary"

cat > "$TMP/declaration-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
probe() { local value="$(false)"; printf 'SURVIVED\n'; }
probe
EOF
declaration_out="$(bash "$TMP/declaration-probe.sh")" || fail "lint-shell: local declaration did not mask substitution status"
[ "$declaration_out" = SURVIVED ] || fail "lint-shell: local declaration probe said: $declaration_out"
pass "lint-shell: declaration builtins are excluded because Bash measures the builtin status, not the substitution"

# The pipeline behavior itself is measured, not inferred from the scanner.
cat > "$TMP/abort-probe.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
COUNT=$(printf 'a\nb\n' | rg ZZNOPE | wc -l)
printf 'UNREACHABLE\n'
EOF
set +e
abort_out="$(bash "$TMP/abort-probe.sh" 2>&1)"
abort_rc=$?
set -e
[ "$abort_rc" = 1 ] || fail "lint-shell: real pipefail probe exited $abort_rc, expected 1"
case "$abort_out" in *UNREACHABLE*) fail "lint-shell: real pipefail probe reached past the assignment" ;; esac
pass "lint-shell: a real middle-stage no-match aborts an errexit assignment before its next statement"

# Run the requested full-bin lint in this auto-discovered subject. Existing
# consumer debt is locked to a line-number-independent semantic fingerprint:
# unrelated insertions may move a finding, but any finding text/source change
# fails this subject rather than hiding behind an accepted exit 1.
set +e
bin_files=(); for f in "$ROOT/bin"/*; do [ -f "$f" ] && bin_files+=("$f"); done  # a stray __pycache__ is not a subject
bin_lint_out="$("$ROOT/bin/lint-shell" "${bin_files[@]}" 2>&1)"
bin_lint_rc=$?
set -e
[ "$bin_lint_rc" = 1 ] || fail "lint-shell: expected the explicit consumer-debt exit 1, got $bin_lint_rc: $bin_lint_out"
brain_archive_count="$(printf '%s\n' "$bin_lint_out" | grep -c "^$ROOT/bin/brain-archive:" || true)"
hw_count="$(printf '%s\n' "$bin_lint_out" | grep -c "^$ROOT/bin/hw:" || true)"
reconcile_count="$(printf '%s\n' "$bin_lint_out" | grep -c "^$ROOT/bin/hw-reconcile:" || true)"
# THIS NUMBER IS A RATCHET AND IT ONLY GOES DOWN. hw went 10 -> 9 on 2026-09-07:
# `_wt_disposition`'s second `n="$(git -C "$wt" status --porcelain 2>/dev/null
# | wc -l …)"` was one of the accepted debts, and the fix that made a FAILED
# `git status` produce `undetermined` instead of `safe` removed the whole
# re-read. Paying a debt down must not read as a regression; ADDING one must.
[ "$brain_archive_count:$hw_count:$reconcile_count" = "1:9:11" ] \
  || fail "lint-shell: full-bin finding distribution changed (brain-archive:hw:hw-reconcile=$brain_archive_count:$hw_count:$reconcile_count)"
# THE SUMMARY LINE IS NOT A FINDING, and hashing it made this ratchet fire on
# things that are not findings. `lint-shell` now ends with "N finding(s) in M
# file(s) read" — and M varies with how many files are in bin/, so ADDING A
# BINARY would change this hash with the finding set byte-identical. Measured
# 2026-09-07 when it did exactly that: the whole diff of old against new was
# that one line. The count is not lost — the per-file distribution assertion
# directly above is what pins it, and it is the assertion that means something.
normalized="$(printf '%s\n' "$bin_lint_out" \
  | grep -v '^[0-9]* finding(s)' \
  | sed -E "s#^$ROOT/bin/([^:]+):[0-9]+:#bin/\\1:#")"
actual_hash="$(printf '%s\n' "$normalized" | shasum -a 256 | cut -d ' ' -f1)"
# TRIAGED TWICE, and this is what triage means here: the old set and the new one
# are diffed IN FULL before the hash moves.
#   2026-09-07 (a): one line left it —
#     bin/hw: n="$(git -C "$wt" status --porcelain 2>/dev/null | wc -l …)"
#   nothing was added. 22 -> 21.
#   2026-09-07 (b): the FINDING SET DID NOT CHANGE AT ALL. The whole diff was
#   lint-shell's summary line gaining "in M file(s) read"; with that line now
#   excluded above, the old and new normalized texts hash identically. The hash
#   below moved only because the exclusion changed what is hashed.
[ "$actual_hash" = 809b9c8d7d31e28ef708225dffbaa99ebb70b8d69ae4520e31fa99b6c756fb62 ] \
  || fail "lint-shell: full-bin semantic finding set changed (sha256=$actual_hash) and needs fresh triage: $bin_lint_out"
pass "lint-shell: full bin/* scan is automatic and its 21 active-errexit consumer findings are exact, not an untriaged dump"
