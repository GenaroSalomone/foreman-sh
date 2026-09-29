#!/usr/bin/env bash
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
SOURCE="${HW_SOURCE:-$ROOT/bin/invoker-common.sh}"
awk '/^invoker_publish\(\) \{/,/^}/' "$SOURCE" > "$TMP/publish.sh"
. "$TMP/publish.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
cat > "$TMP/rpc" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$2" >> "$CALLS"
if [ "$2" = pane.report_metadata ]; then
  [ "$CASE" != write-error ] || exit 3
  printf '{"type":"ok"}\n'; exit 0
fi
[ "$2" = agent.list ] || exit 4
case "$CASE" in
  read-error) exit 3 ;;
  malformed) printf 'not json\n' ;;
  absent) printf '{"agents":[]}\n' ;;
  wrong-pane) printf '{"agents":[{"pane_id":"pOTHER","tokens":{"x":"new"}}]}\n' ;;
  wrong-value) printf '{"agents":[{"pane_id":"pTEST","tokens":{"x":"old"}}]}\n' ;;
  undeleted) printf '{"agents":[{"pane_id":"pTEST","tokens":{"x":"new","drop":"old"}}]}\n' ;;
  null-not-deleted) printf '{"agents":[{"pane_id":"pTEST","tokens":{"x":"new","drop":null}}]}\n' ;;
  duplicate) printf '{"agents":[{"pane_id":"pTEST","tokens":{"x":"new"}},{"pane_id":"pTEST","tokens":{"x":"new"}}]}\n' ;;
  *) printf '{"agents":[{"pane_id":"pTEST","tokens":{"x":"new"}}]}\n' ;;
esac
SH
chmod +x "$TMP/rpc"
INVOKER_RPC="$TMP/rpc" HERDR_PANE_ID=pTEST INVOKER_SOURCE=hw:test
export CASE CALLS="$TMP/calls"
for CASE in match read-error malformed absent wrong-pane wrong-value undeleted null-not-deleted duplicate write-error; do
  : > "$CALLS"
  rc=0; out="$(invoker_publish 60000 '{"x":"new","drop":null}' 2>&1)" || rc=$?
  case "$CASE" in
    match) [ "$rc" = 0 ] || fail "matching token read-back failed: $out" ;;
    write-error) [ "$rc" = 3 ] && [ "$(wc -l < "$CALLS" | tr -d ' ')" = 1 ] || fail "write transport error was hidden or read-back ran" ;;
    *) [ "$rc" = 4 ] || fail "metadata ack became token proof for $CASE: exit=$rc" ;;
  esac
  pass "invoker_publish $CASE has the measured publication outcome"
done
