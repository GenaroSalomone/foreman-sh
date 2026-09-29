#!/usr/bin/env bash
# pre-push required a cached verdict for the EXACT tip tree being pushed,
# with no exception. Measured 2026-09-10, four times in one day: a brainer
# runs setup/verify-for-push against a verified tree, then another lane
# commits something outside SUITE_TRIGGER_PATTERN (setup/decisions.md, a
# brief) on top before the push actually happens — the tip's tree changed,
# has no verdict of its own, and pre-push refused a push that could not
# possibly have been invalidated by what changed.
#
# The tribal workaround was pushing the verified sha explicitly, then the
# rest, in two separate pushes. This is the durable version: pre-push now
# also accepts the cached verdict of the MOST RECENT commit (walking
# local_sha's first-parent history) whose OWN change touches
# SUITE_TRIGGER_PATTERN — "the last commit that touched the pattern has a
# verdict", not "some ancestor does". Nothing capable of invalidating the
# suite changed between that commit and the tip, so its verdict still
# describes the tip's suite-relevant content.
#
# DELIBERATELY LINEAR-ONLY. A merge anywhere between the tip and the
# candidate trigger commit could bring in suite-relevant content through a
# non-first parent this walk never inspects, so a merge in that span bails
# out to the exact-tip check instead of guessing — narrower than the general
# case, on purpose (see setup/hooks/pre-push's own comment on
# ancestor_verdict_covers).
#
# Run alone while working on this subject:
#     bash setup/tests/128-an-ancestors-verdict-can-cover-a-quieter-tip.sh
#
# Companion coverage: setup/tests/119-the-gate-does-not-break-the-push.sh
# (C04/C07 were adjusted so their own fixtures stay genuinely UNcovered by
# this shortcut) and setup/tests/120-the-gate-asks-what-changed.sh (the
# range-wide "nothing triggers at all" shortcut this one does not replace).
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HOOK="$ROOT/setup/hooks/pre-push"
PATTERN_FILE="$ROOT/setup/hooks/suite-trigger-pattern.sh"
VERIFY="$ROOT/setup/verify-for-push"
[ -x "$HOOK" ] || fail "gate: $HOOK is missing or not executable"
[ -f "$PATTERN_FILE" ] || fail "gate: $PATTERN_FILE is missing — the single source of truth for the trigger pattern"
[ -x "$VERIFY" ] || fail "gate: $VERIFY is missing or not executable"
ZERO="0000000000000000000000000000000000000000"

grep -q 'ancestor_verdict_covers' "$HOOK" \
  || fail "criterion: setup/hooks/pre-push does not define an ancestor-verdict shortcut at all"
pass "criterion: pre-push defines the ancestor-verdict shortcut"

mk_repo() { # (no args) → prints the fixture root
  local d; d="$(mktemp -d "$TMP/ancrepo-XXXXXX")"
  mkdir -p "$d/setup/hooks"
  cp "$PATTERN_FILE" "$d/setup/hooks/suite-trigger-pattern.sh"
  # pre-push also sources setup/hooks/attribution-pattern.sh (added 2026-09-16,
  # shared with setup/hooks/commit-msg so the two gates cannot drift on what
  # counts as attribution). A fixture without it dies before reaching the gate.
  cp "$ROOT/setup/hooks/attribution-pattern.sh" "$d/setup/hooks/attribution-pattern.sh"
  cp "$VERIFY" "$d/setup/verify-for-push"
  cp "$HOOK" "$d/setup/hooks/pre-push"
  chmod +x "$d/setup/verify-for-push" "$d/setup/hooks/pre-push"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git init -q -b main )
  printf '%s' "$d"
}
stub_suite() { # <dir> <exit>
  cat > "$1/setup/test-hw" <<STUB
#!/usr/bin/env bash
printf '%s\\n' "42 tests passed  (TRACKED only)"
exit $2
STUB
  chmod +x "$1/setup/test-hw"
}
commit_file() { # <dir> <relpath> <content> <message>
  local d="$1"; mkdir -p "$(dirname "$d/$2")"; printf '%s\n' "$3" > "$d/$2"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git add -A \
      && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE \
        git -c user.email=t@t -c user.name=t commit -q -m "${4:-fixture}" )
}
run_verify() { # <dir>
  ( cd "$1" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/verify-for-push >/dev/null 2>&1 ) || true
}
run_push() { # <dir> <remote> <local-sha> [<remote-sha>] → "exit=<rc>|<out>"
  local d="$1" remote="$2" local_sha="$3" remote_sha="${4:-$ZERO}" rc=0 out
  out="$(cd "$d" && printf 'refs/heads/main %s refs/heads/main %s\n' "$local_sha" "$remote_sha" \
        | env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/hooks/pre-push "$remote" 2>&1)" || rc=$?
  printf 'exit=%s|%s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}

# ── C01: verified bin/hw commit, then an unrelated non-triggering commit on
# top (another lane's setup/decisions.md) — the TIP has no verdict of its
# own, but the ancestor's does, and nothing since touches the pattern. ──────
d="$(mk_repo)"; stub_suite "$d" 0
commit_file "$d" "bin/hw" "echo hw" "feat: touch bin/hw"
sha1="$(cd "$d" && git rev-parse HEAD)"
run_verify "$d"
commit_file "$d" "setup/decisions.md" "prose on top, another lane" "decide: unrelated prose"
sha2="$(cd "$d" && git rev-parse HEAD)"
r="$(run_push "$d" backup "$sha2")"
case "$r" in
  exit=0*"ancestor"*"covers it"*) pass "C01: an ancestor's verdict covers a quieter tip stacked on top of it" ;;
  *) fail "C01: a tip covered only by an ancestor's verdict was refused: $r" ;;
esac

# ── C02 (MUTATION): the tip's OWN change touching the pattern is NOT covered
# by an older, unrelated verdict — the shortcut names the LAST trigger
# commit, never just "any ancestor". ────────────────────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0
commit_file "$d" "bin/hw" "echo hw" "feat: touch bin/hw"
sha1="$(cd "$d" && git rev-parse HEAD)"
run_verify "$d"
commit_file "$d" "bin/other" "echo other" "feat: a second, unverified pattern-touching commit"
sha2="$(cd "$d" && git rev-parse HEAD)"
r="$(run_push "$d" backup "$sha2")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C02: a tip whose OWN change touches the pattern is not waved through by an older ancestor's verdict" ;;
  exit=0*) fail "C02 MUTATION SURVIVED: a change capable of invalidating the suite passed unverified, covered only by a stale ancestor verdict" ;;
  *) fail "C02: unexpected result: $r" ;;
esac

# ── C03 (MUTATION): a MERGE between the tip and the candidate trigger commit
# bails out to the exact-tip check instead of guessing through it. ─────────
d="$(mk_repo)"; stub_suite "$d" 0
commit_file "$d" "bin/hw" "echo hw" "feat: touch bin/hw"
base="$(cd "$d" && git rev-parse HEAD)"
run_verify "$d"
( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git checkout -qb side )
commit_file "$d" "setup/decisions.md" "side branch prose" "decide: side"
( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git checkout -q main )
commit_file "$d" "setup/briefs/x.md" "main branch prose" "brief: main"
( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git -c user.email=t@t -c user.name=t merge -q --no-ff side -m "merge: side into main" )
sha_merge="$(cd "$d" && git rev-parse HEAD)"
r="$(run_push "$d" backup "$sha_merge")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C03: a merge in the walk bails out to the exact-tip check rather than guessing through it" ;;
  exit=0*) fail "C03 MUTATION SURVIVED: a merge commit was silently walked through and covered by an ancestor's verdict" ;;
  *) fail "C03: unexpected result for a merge in the walk: $r" ;;
esac

# ── C04: the shortcut requires the TRIGGER commit's OWN verdict — a verdict
# that was never cached at all still refuses, even though the walk correctly
# identifies the same trigger commit. ───────────────────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0
commit_file "$d" "bin/hw" "echo hw" "feat: touch bin/hw, never verified"
sha1="$(cd "$d" && git rev-parse HEAD)"
commit_file "$d" "setup/decisions.md" "prose on top of an unverified trigger commit" "decide: prose"
sha2="$(cd "$d" && git rev-parse HEAD)"
r="$(run_push "$d" backup "$sha2")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C04: the shortcut still refuses when the trigger commit it finds has no cached verdict of its own" ;;
  *) fail "C04: a push was accepted despite the trigger commit itself never having a cached verdict: $r" ;;
esac
