#!/usr/bin/env bash
# setup/verify-for-push refused ANY dirty tracked file, regardless of whether
# that file could ever affect the suite. Measured 2026-09-10, four times in
# one day: this repo is shared by five lanes appending to setup/decisions.md,
# briefs, and registry caches continuously, and a dirty decisions.md refused
# verify-for-push one step from the end of an unrelated task.
#
# The fix narrows the SCOPE of the dirty check to SUITE_TRIGGER_PATTERN
# (setup/hooks/suite-trigger-pattern.sh — the same file pre-commit and
# pre-push both source): a dirty tracked file outside that pattern cannot
# change what the suite reads, so a verdict cached against HEAD's tree stays
# honest regardless of it. A dirty tracked file that DOES match the pattern
# still refuses, exactly as before — the check itself does not go away, only
# its scope narrows.
#
# Run alone while working on this subject:
#     bash setup/tests/127-a-quiet-tracked-dirt-does-not-block-verify.sh
#
# Companion coverage: setup/tests/119-the-gate-does-not-break-the-push.sh
# (C05's own dirty-tree guard now dirties a PATTERN-matching path so this
# fix does not silently defang it) and setup/tests/120-the-gate-asks-what-
# changed.sh (the same pattern, applied to pre-push's own range decision).
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

VERIFY="$ROOT/setup/verify-for-push"
PATTERN_FILE="$ROOT/setup/hooks/suite-trigger-pattern.sh"
[ -x "$VERIFY" ] || fail "gate: $VERIFY is missing or not executable"
[ -f "$PATTERN_FILE" ] || fail "gate: $PATTERN_FILE is missing — the single source of truth for the trigger pattern"

grep -qF 'suite-trigger-pattern.sh' "$VERIFY" \
  || fail "criterion: setup/verify-for-push does not source setup/hooks/suite-trigger-pattern.sh"
pass "criterion: verify-for-push sources the one shared trigger-pattern file"

mk_repo() { # (no args) → prints the fixture root
  local d; d="$(mktemp -d "$TMP/verifyrepo-XXXXXX")"
  mkdir -p "$d/setup/hooks"
  cp "$PATTERN_FILE" "$d/setup/hooks/suite-trigger-pattern.sh"
  cp "$VERIFY" "$d/setup/verify-for-push"
  chmod +x "$d/setup/verify-for-push"
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
commit_all() { # <dir> <message>
  ( cd "$1" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git add -A \
      && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE \
        git -c user.email=t@t -c user.name=t commit -q -m "${2:-fixture}" )
}
run_verify() { # <dir> → "exit=<rc>|<out>"
  local d="$1" rc=0 out
  out="$(cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/verify-for-push 2>&1)" || rc=$?
  printf 'exit=%s|%s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}

# ── C01: a dirty tracked file OUTSIDE SUITE_TRIGGER_PATTERN does not block
# verification — a real run, not a stubbed skip. ────────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0
mkdir -p "$d/setup"; printf 'an entry\n' > "$d/setup/decisions.md"; commit_all "$d" "decide: base"
printf 'an entry
appended while another lane is mid-verify\n' > "$d/setup/decisions.md"   # dirty, tracked, OUTSIDE the pattern
r="$(run_verify "$d")"
case "$r" in
  exit=0*"PASSED"*) pass "C01: a dirty setup/decisions.md (outside SUITE_TRIGGER_PATTERN) does not block verify-for-push" ;;
  *) fail "C01: a non-triggering dirty file blocked verification: $r" ;;
esac

# ── C02 (MUTATION): a dirty tracked file INSIDE SUITE_TRIGGER_PATTERN still
# refuses, exactly as before the narrowing. ─────────────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0
mkdir -p "$d/bin"; printf '#!/usr/bin/env bash\necho hw\n' > "$d/bin/hw"; commit_all "$d" "feat: clean bin/hw"
tree_before="$(cd "$d" && git rev-parse HEAD^{tree})"
printf '#!/usr/bin/env bash\necho hw dirty\n' > "$d/bin/hw"   # dirty, tracked, INSIDE the pattern
r="$(run_verify "$d")"
case "$r" in
  exit=1*"can invalidate the suite"*) pass "C02: a dirty bin/hw (matches SUITE_TRIGGER_PATTERN) still refuses verify-for-push" ;;
  exit=1*) fail "C02: refused, but not for the stated reason (scope check may have regressed to something else): $r" ;;
  *) fail "C02 MUTATION SURVIVED: a dirty pattern-matching file was allowed through: $r" ;;
esac
[ ! -f "$(cd "$d" && git rev-parse --git-common-dir)/hw-push-verified/$tree_before" ] \
  || fail "C02: a cache file was written despite the pattern-matching dirty refusal"

# ── C03 (MUTATION, adversarial-review shape): a git failure while computing
# the dirty diff must refuse, never be read as "nothing dirty matters". ─────
d="$(mk_repo)"; stub_suite "$d" 0
mkdir -p "$d/setup"; printf 'an entry\n' > "$d/setup/decisions.md"; commit_all "$d" "decide: base"
printf 'dirty\n' > "$d/setup/decisions.md"
rm -f "$d/setup/hooks/suite-trigger-pattern.sh"
r="$(run_verify "$d")"
case "$r" in
  exit=0*) fail "C03 MUTATION SURVIVED: a missing pattern file let a dirty tree pass verification: $r" ;;
  exit=1*) pass "mutant killed: a missing pattern file refuses loudly (bash aborts sourcing it under set -e) instead of silently waving a dirty tree through" ;;
  *) fail "C03: unexpected result for a missing pattern file: $r" ;;
esac

# ── C04: an untracked file remains completely outside this check, unchanged
# by the narrowing (unaffected regression guard). ───────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0
mkdir -p "$d/setup"; printf 'an entry\n' > "$d/setup/decisions.md"; commit_all "$d" "decide: base"
printf 'irrelevant\n' > "$d/untracked-from-another-agent.md"
r="$(run_verify "$d")"
case "$r" in
  exit=0*"PASSED"*) pass "C04: an untracked file elsewhere in the tree still does not block verification" ;;
  *) fail "C04: an untracked file wrongly blocked verification after the narrowing: $r" ;;
esac
