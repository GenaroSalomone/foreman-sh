#!/usr/bin/env bash
# Closing: a non-zero close is not evidence that the target is absent.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# Extract the production helpers with only their output dependencies. Driving
# these directly keeps every target synthetic: this test never closes a live
# pane, tab or workspace.
awk '/^_HERDR_CLOSE_REASON=/,/^# ── One space, one call:/' "$ROOT/bin/hw" | sed '$d' > "$TMP/close-fns.sh"
cat > "$TMP/drive.sh" <<EOF
#!/usr/bin/env bash
set -uo pipefail
ok() { printf 'OK:%s\n' "\$1"; }
info() { printf 'INFO:%s\n' "\$1"; }
warn() { printf 'WARN:%s\n' "\$1"; }
_capture() { local out rc=0; out="\$("\$@" 2>&1)" || rc=\$?; printf '%s' "\$out"; return \$rc; }
. "$TMP/close-fns.sh"
_close_herdr_object "\$@"
EOF
chmod +x "$TMP/drive.sh"

cat > "$TMP/bin/herdr" <<'STUB'
#!/usr/bin/env bash
kind="${1:-}"; action="${2:-}"
if [ "$action" = close ]; then
  printf '%s\n' "${STUB_CLOSE_REASON:-refused by fixture}" >&2
  exit "${STUB_CLOSE_RC:-42}"
fi
if [ "$action" = list ]; then
  [ "${STUB_LIST_RC:-0}" = 0 ] || { printf '%s\n' "${STUB_LIST_REASON:-server unavailable}" >&2; exit "$STUB_LIST_RC"; }
  case "$kind" in
    pane)      printf '{"result":{"panes":%s}}\n' "${STUB_OBJECTS:-[]}" ;;
    tab)       printf '{"result":{"tabs":%s}}\n' "${STUB_OBJECTS:-[]}" ;;
    workspace) printf '{"result":{"workspaces":%s}}\n' "${STUB_OBJECTS:-[]}" ;;
  esac
fi
STUB
chmod +x "$TMP/bin/herdr"

run_close() {
  local rc=0
  env "$@" "$TMP/drive.sh" tab w9:t9 "tab closed" "tab is already gone" > "$TMP/out" 2>&1 || rc=$?
  printf '%s' "$rc"
}

# The motivating behavior: close fails and the exact tab remains in a fresh,
# parseable list. This must be a loud failure carrying the close reason.
rc="$(run_close STUB_OBJECTS='[{"tab_id":"w9:t9"}]' STUB_CLOSE_REASON='permission denied')"
[ "$rc" = 1 ] || fail "close: a refused close against a tab that still exists exited $rc, not 1"
grep -q 'WARN:CLOSE FAILED: tab w9:t9 still exists' "$TMP/out" \
  || fail "close: the surviving target was not reported as a failure"
grep -q 'permission denied' "$TMP/out" || fail "close: the provider reason was discarded"
if grep -q 'already gone' "$TMP/out"; then fail "close: a surviving tab was still called already gone"; fi
pass "close: a non-zero close against a tab that still exists is a failure, with the provider reason"

# `already gone` has one license: the follow-up list succeeded, parsed and did
# not contain the exact id.
rc="$(run_close STUB_OBJECTS='[]')"
[ "$rc" = 0 ] || fail "close: measured absence exited $rc"
grep -q 'INFO:tab is already gone' "$TMP/out" || fail "close: measured absence lost the already-gone verdict"
pass "close: already gone is said only after a successful list measures the exact id absent"

# Herdr down after a failed close cannot prove either presence or absence. It is
# another failure, not an excuse to infer absence.
rc="$(run_close STUB_LIST_RC=7 STUB_LIST_REASON='socket unavailable')"
[ "$rc" = 1 ] || fail "close: an unverifiable close exited $rc, not 1"
grep -q 'absence could not be verified: list exited 7: socket unavailable' "$TMP/out" \
  || fail "close: the failed verification reason was not named"
if grep -q 'already gone' "$TMP/out"; then fail "close: herdr being unavailable was called absence"; fi
pass "close: an unavailable verifier is a named failure, never inferred absence"

# Sibling audit. These are the only close sites in cmd_done: leased agent pane,
# owned task tab, and legacy workspace. Sweep is the batch recovery path. Every
# one must route through the same measured helper; no direct suppressed close is
# allowed to reappear.
done_body="$(awk '/^cmd_done\(\)/,/^}$/ ' "$ROOT/lib/hw/done.sh")"
[ "$(printf '%s' "$done_body" | grep -c '_close_herdr_object' || true)" = 3 ] \
  || fail "close audit: cmd_done does not route exactly pane, tab and workspace through the measured helper"
if printf '%s' "$done_body" | grep -E 'herdr (pane|tab|workspace) close .*dev/null' >/dev/null; then
  fail "close audit: cmd_done regained a directly suppressed herdr close"
fi
sweep_body="$(awk '/^cmd_sweep\(\)/,/^}$/ ' "$ROOT/bin/hw")"
grep -q '_close_herdr_object' <<<"$sweep_body" || fail "close audit: sweep still owns an unmeasured close path"
pass "close audit: leased pane, task tab, legacy workspace and batch sweep share measured close handling"

# MUTANT: restore the old inference by forcing the post-failure probe to say
# absent. The same live-target fixture must then reproduce the false reassurance,
# proving the first assertion is coupled to the fix rather than merely green.
sed 's/_herdr_object_exists "\$kind" "\$id" || probe_rc=\$?/_herdr_object_exists "\$kind" "\$id" >\/dev\/null 2>\&1; probe_rc=1/' \
  "$TMP/close-fns.sh" > "$TMP/close-fns.mut.sh"
sed "s|$TMP/close-fns.sh|$TMP/close-fns.mut.sh|" "$TMP/drive.sh" > "$TMP/drive.mut.sh"
chmod +x "$TMP/drive.mut.sh"
mut="$(STUB_OBJECTS='[{"tab_id":"w9:t9"}]' "$TMP/drive.mut.sh" tab w9:t9 closed 'tab is already gone' 2>&1 || true)"
case "$mut" in
  *'INFO:tab is already gone'*) pass "close mutation: restoring failure-means-absence reproduces the false already-gone verdict" ;;
  *) fail "close mutation: the old inference did not reproduce, so the regression test is not coupled to it" ;;
esac
