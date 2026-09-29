#!/usr/bin/env bash
# 684b760 added an AI-attribution gate to setup/hooks/pre-push WITHOUT a
# dedicated test — verified by hand, four real payloads, the four runs
# pasted into that commit's own message, but no wall. This is the wall.
#
# THE RULE: "Never add Co-Authored-By or AI attribution to commits" is a
# standing hard rule. It lives in pre-push, not pre-commit or commit-msg,
# because the setup lane is repo-less and commits with the private-index
# recipe (setup/CLAUDE.md) — `git commit-tree`, which runs NO hooks at all.
# pre-push is the one gate every commit passes regardless of how it was
# built, and it is the boundary that matters: what reaches a remote.
#
# ONLY NEW COMMITS ARE CHECKED — `git rev-list <local> --not
# --remotes=<remote>`, the same reckoning setup/hooks/pre-push's own
# range_needs_suite uses for "what is actually new". 12 commits already in
# this history carry the trailer from before the rule was enforced;
# rewriting them would rewrite shared history.
#
# Run alone while working on this subject:
#     bash setup/tests/129-pre-push-refuses-ai-attribution-in-new-commits.sh
#
# Companion coverage: setup/tests/112-pre-push-allows-backup-and-github-only.sh
# (the remote-name gate, which runs before this one) and setup/tests/119 /
# 128 (the verdict gate this one must still reach after passing).
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HOOK="$ROOT/setup/hooks/pre-push"
PATTERN_FILE="$ROOT/setup/hooks/suite-trigger-pattern.sh"
VERIFY="$ROOT/setup/verify-for-push"
[ -x "$HOOK" ] || fail "gate: $HOOK is missing or not executable"
[ -f "$PATTERN_FILE" ] || fail "gate: $PATTERN_FILE is missing"
[ -x "$VERIFY" ] || fail "gate: $VERIFY is missing or not executable"
ZERO="0000000000000000000000000000000000000000"

mk_repo() { # (no args) → prints the fixture root
  local d; d="$(mktemp -d "$TMP/attrrepo-XXXXXX")"
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
commit_msg() { # <dir> <relpath> <content> <message (may be multi-line)>
  local d="$1"; mkdir -p "$(dirname "$d/$2")"; printf '%s\n' "$3" > "$d/$2"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git add -A \
      && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE \
        git -c user.email=t@t -c user.name=t commit -q -m "$4" )
}
run_verify() { ( cd "$1" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/verify-for-push >/dev/null 2>&1 ) || true; }
run_push() { # <dir> <remote> <local-sha> [<remote-sha>] → "exit=<rc>|<out>"
  local d="$1" remote="$2" local_sha="$3" remote_sha="${4:-$ZERO}" rc=0 out
  out="$(cd "$d" && printf 'refs/heads/main %s refs/heads/main %s\n' "$local_sha" "$remote_sha" \
        | env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/hooks/pre-push "$remote" 2>&1)" || rc=$?
  printf 'exit=%s|%s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}

# ── C01 (brief case 1): a NEW offending commit — rc=1, names the sha AND the
# subject. ────────────────────────────────────────────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0
commit_msg "$d" "setup/decisions.md" "prose" "$(printf 'decide: an offending change\n\nCo-Authored-By: Claude <noreply@anthropic.com>')"
sha="$(cd "$d" && git rev-parse HEAD)"
subject="$(cd "$d" && git log -1 --format=%s HEAD)"
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=1*"AI attribution"*"$sha"*"$subject"*) pass "C01: a new offending commit refuses, naming both its sha and its subject" ;;
  exit=1*"AI attribution"*) fail "C01: refused for attribution, but did not name the sha and subject as required: $r" ;;
  *) fail "C01: a new commit carrying Co-Authored-By was not refused: $r" ;;
esac
# AND IT NAMES THE LINE AND THE COLUMN, the same way commit-msg does, from the
# same formatter in setup/hooks/attribution-pattern.sh. A commit message in this
# repo can carry the trailer as QUOTED EVIDENCE and as a real trailer at the
# same time; naming the sha alone leaves the reader unable to tell which line
# the gate objected to.
case "$r" in
  *"line 3, column 1: Co-Authored-By: Claude"*) pass "C01: the refusal names the offending LINE and its column, not only the commit" ;;
  *) fail "C01: refused and named the commit, but never named the offending line and column: $r" ;;
esac

# ── C01b: A QUOTATION IS NOT A TRAILER, and this gate is what proved it was
# being treated as one. MEASURED 2026-09-17: the detection, run over the 55
# commits this repository had not yet pushed, refused three — 222f9d7 (a real
# trailer at column 0) and c70dbe7 / 9dc5ab8, the two commits that BUILT this
# guard and paste the trailer into their own messages as evidence, indented.
# The release sat blocked behind two false positives of the guard's own making.
# The anchor is `^` now; this arm is the indented shape, driven through the real
# hook, and it must also still REACH the verdict gate rather than being waved
# through by an attribution gate that stopped looking.
d="$(mk_repo)"; stub_suite "$d" 0
commit_msg "$d" "bin/hw" "echo hw" "$(printf 'fix: prove the detection by quoting what it caught\n\nThe run printed:\n\n  Co-Authored-By: Claude <noreply@anthropic.com>\n\nwhich is the line the gate is supposed to catch.')"
sha="$(cd "$d" && git rev-parse HEAD)"
run_verify "$d"
r="$(run_push "$d" backup "$sha")"
case "$r" in
  *"AI attribution"*) fail "C01b: an INDENTED quotation of the trailer was refused as a trailer — this is the 2026-09-17 release blocker: $r" ;;
  exit=0*"proceeding"*) pass "C01b: an indented quotation passes the attribution gate and still reaches the verdict gate" ;;
  *) fail "C01b: unexpected result for a commit quoting the trailer as evidence: $r" ;;
esac

# ── C02 (brief case 2): a clean HEAD already covered by a cached verdict —
# rc=0, and the verdict gate is still REACHED (not short-circuited away). ──
d="$(mk_repo)"; stub_suite "$d" 0
commit_msg "$d" "bin/hw" "echo hw" "feat: touch bin/hw, clean message"
sha="$(cd "$d" && git rev-parse HEAD)"
run_verify "$d"
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=0*"proceeding"*) pass "C02: a clean, already-verified commit passes attribution and still reaches the verdict gate" ;;
  *) fail "C02: a clean commit with a cached verdict was not accepted, or the verdict gate was not reached: $r" ;;
esac

# ── C03 (brief case 3): a HISTORICAL offender already on the remote — rc=0,
# does not fire. Only what is NEW (unreachable from the remote's own ref) is
# ever inspected. ────────────────────────────────────────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0
commit_msg "$d" "setup/decisions.md" "old prose" "$(printf 'decide: an old, already-pushed offender\n\nCo-Authored-By: Claude <noreply@anthropic.com>')"
historical_sha="$(cd "$d" && git rev-parse HEAD)"
( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git update-ref refs/remotes/backup/main "$historical_sha" )
commit_msg "$d" "setup/decisions.md" "new prose" "decide: a clean commit on top of the historical offender"
sha="$(cd "$d" && git rev-parse HEAD)"
r="$(run_push "$d" backup "$sha" "$historical_sha")"
case "$r" in
  exit=0*) pass "C03: a historical offender already on the remote does not fire — only the new commit was inspected" ;;
  *"AI attribution"*) fail "C03 MUTATION SURVIVED: a commit already on the remote was re-inspected and refused: $r" ;;
  *) fail "C03: unexpected result for a historical offender already on the remote: $r" ;;
esac

# ── C04 (brief case 4): a clean tree with NO cached verdict — rc=1, and the
# refusal is the VERDICT gate's, not the attribution gate's silently eating
# it. Proves the two gates are independent and both load-bearing. ──────────
d="$(mk_repo)"; stub_suite "$d" 0
commit_msg "$d" "bin/hw" "echo hw" "feat: touch bin/hw, clean message, never verified"
sha="$(cd "$d" && git rev-parse HEAD)"
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C04: a clean but unverified tree still refuses at the verdict gate, which the attribution gate did not block reaching" ;;
  exit=1*"AI attribution"*) fail "C04: a clean commit was wrongly refused by the attribution gate" ;;
  *) fail "C04: an unverified clean tree was not refused: $r" ;;
esac

# ── C05 (MUTATION, PUSHED_REFS): the ref list arrives on stdin ONCE and both
# gates must read the SAME captured copy. Reverting the coverage gate's own
# loop to a fresh `$(cat)` starves it — stdin is already exhausted by the
# attribution gate's read — which fails OPEN: a push with no cached verdict
# for a pattern-touching change gets waved through as "nothing to check".
d="$(mk_repo)"; stub_suite "$d" 0
commit_msg "$d" "bin/hw" "echo hw" "feat: touch bin/hw, clean message, never verified"
sha="$(cd "$d" && git rev-parse HEAD)"
python3 - "$d/setup/hooks/pre-push" <<'PY'
import re, sys
path = sys.argv[1]
src = open(path).read()
marker = 'done <<EOF\n$PUSHED_REFS\nEOF\nif [ -n "$missing" ]'
assert marker in src, "fixture broken: could not find the coverage loop's own PUSHED_REFS terminator"
mutated = src.replace(marker, 'done <<EOF\n$(cat)\nEOF\nif [ -n "$missing" ]', 1)
assert mutated != src
open(path, "w").write(mutated)
PY
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=0*"nothing to check"*) pass "mutant killed: reverting the coverage gate's own ref-list read to a fresh \$(cat) starves it and waves an uncovered push through" ;;
  exit=1*) fail "C05 mutant: the mutated hook refused anyway — mutation setup did not reproduce the double-read bug (fixture is broken, not the fix)" ;;
  *) fail "C05 mutant: unexpected result: $r" ;;
esac
