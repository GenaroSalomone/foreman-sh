#!/usr/bin/env bash
# pre-push used to run the full suite inline (~1000-1200s) to enforce mandatory
# coverage before a push. Measured 2026-09-10, pushing FOR REAL, not invoking
# the hook by hand: that held an SSH connection to github idle long enough for
# GitHub to close it mid-push (`Connection to github.com closed by remote
# host`, then `exit 141` SIGPIPE on retry) — even though the suite itself
# passed both times. git contacts the remote FIRST to hand the pre-push hook
# its ref list over stdin, and only transfers objects AFTER the hook returns,
# so anything the hook does that takes minutes holds that connection open for
# the same minutes. `git push --no-verify github main` entered in 3 seconds:
# the transport was never broken, the hook's own cost was.
#
# So verification moved OUT of the hook: `setup/verify-for-push` runs the full
# suite against a clean HEAD and, on green, caches a verdict keyed by HEAD's
# EXACT tree at `$(git rev-parse --git-common-dir)/hw-push-verified/<tree-sha>`.
# `setup/hooks/pre-push` now only checks that file exists for every tree being
# pushed — a handful of `git rev-parse` calls and a stat, never a suite run.
#
# Run alone while working on this subject:
#     bash setup/tests/119-the-gate-does-not-break-the-push.sh
#
# Companion coverage: setup/tests/112-pre-push-allows-backup-and-github-only.sh
# (the remote-name gate, which runs BEFORE anything here) and
# setup/tests/117-two-gates-cover-every-subject.sh (C08 pins that pre-push
# never again shells out to setup/test-hw directly). Run all three:
#     for f in 112 117 119; do bash setup/tests/$f-*.sh; done
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

HOOK="$ROOT/setup/hooks/pre-push"
VERIFY="$ROOT/setup/verify-for-push"
[ -x "$HOOK" ] || fail "gate: $HOOK is missing or not executable"
[ -x "$VERIFY" ] || fail "gate: $VERIFY is missing or not executable"
ZERO="0000000000000000000000000000000000000000"

# ── A throwaway repo, with a REAL copy of both scripts under test ──────────
# `setup/test-hw` is stubbed (controllable exit/sleep) — this file is about
# the cache mechanism, not about re-running the real ~1000s suite. That
# distinction is exactly why verify-for-push and pre-push are split in the
# first place, so stubbing the expensive part here is faithful, not a shortcut
# around what is being tested.
mk_repo() { # (no args) → prints the fixture root
  local d; d="$(mktemp -d "$TMP/pushrepo-XXXXXX")"
  mkdir -p "$d/setup/hooks"
  cp "$VERIFY" "$d/setup/verify-for-push"
  # pre-push SOURCES its sibling setup/hooks/suite-trigger-pattern.sh (one
  # shared file with pre-commit, see setup/tests/120) — the fixture must keep
  # that relative layout, not a top-level "./hook".
  cp "$ROOT/setup/hooks/suite-trigger-pattern.sh" "$d/setup/hooks/suite-trigger-pattern.sh"
  # pre-push also sources setup/hooks/attribution-pattern.sh (added 2026-09-16,
  # shared with setup/hooks/commit-msg so the two gates cannot drift on what
  # counts as attribution). A fixture without it dies before reaching the gate.
  cp "$ROOT/setup/hooks/attribution-pattern.sh" "$d/setup/hooks/attribution-pattern.sh"
  cp "$HOOK" "$d/setup/hooks/pre-push"
  chmod +x "$d/setup/verify-for-push" "$d/setup/hooks/pre-push"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git init -q )
  printf '%s' "$d"
}
stub_suite() { # <dir> <exit> <sleep-seconds>
  cat > "$1/setup/test-hw" <<STUB
#!/usr/bin/env bash
sleep $3
printf '%s\\n' "\${STUB_HEADLINE:-42 tests passed  (TRACKED only)}"
exit $2
STUB
  chmod +x "$1/setup/test-hw"
}
git_commit_all() { # <dir> <message>
  ( cd "$1" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git add -A \
      && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE \
        git -c user.email=t@t -c user.name=t commit -q -m "${2:-fixture}" )
}
run_verify() { # <dir> → "exit=<rc>|<stdout+stderr, newlines squashed>"
  local d="$1" rc=0 out
  out="$(cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/verify-for-push 2>&1)" || rc=$?
  printf 'exit=%s|%s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}
run_push() { # <dir> <remote> <local-sha> [<local-ref> <remote-ref> <remote-sha>] → "exit=<rc>|<out>"
  local d="$1" remote="$2" local_sha="$3"
  local local_ref="${4:-refs/heads/main}" remote_ref="${5:-refs/heads/main}" remote_sha="${6:-$ZERO}"
  local rc=0 out
  out="$(cd "$d" && printf '%s %s %s %s\n' "$local_ref" "$local_sha" "$remote_ref" "$remote_sha" \
        | env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/hooks/pre-push "$remote" 2>&1)" || rc=$?
  printf 'exit=%s|%s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}
cache_file() { # <dir> <tree-sha>
  printf '%s/.git/hw-push-verified/%s' "$1" "$2"
}

# ── C01: no cached verdict → pre-push refuses FAST, no suite invoked ────────
d="$(mk_repo)"; stub_suite "$d" 0 0
printf 'x\n' > "$d/file.txt"; git_commit_all "$d"
sha="$(cd "$d" && git rev-parse HEAD)"
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C01: pre-push refuses a tree with no cached verdict" ;;
  *) fail "C01: expected a refusal naming the missing verdict: $r" ;;
esac

# ── C02: verify-for-push runs the suite, passes, caches; pre-push then allows
r="$(run_verify "$d")"
case "$r" in exit=0*"PASSED"*"cached for tree"*) pass "C02: verify-for-push caches a verdict on a passing suite" ;;
  *) fail "C02: verify-for-push did not report a cached pass: $r" ;; esac
tree="$(cd "$d" && git rev-parse HEAD^{tree})"
[ -f "$(cache_file "$d" "$tree")" ] || fail "C02: no cache file was written at hw-push-verified/$tree"
r="$(run_push "$d" backup "$sha")"
case "$r" in exit=0*"proceeding"*) pass "C02: pre-push allows the push once a verdict is cached for that exact tree" ;;
  *) fail "C02: pre-push still refused after a real cached pass: $r" ;; esac

# ── C03 (MUTATION): a FAILING suite caches NOTHING — pre-push keeps refusing ─
d="$(mk_repo)"; stub_suite "$d" 1 0
printf 'x\n' > "$d/file.txt"; git_commit_all "$d"
sha="$(cd "$d" && git rev-parse HEAD)"
r="$(run_verify "$d")"
case "$r" in exit=1*"FAILED"*"no verdict cached"*) pass "C03: verify-for-push reports failure and caches nothing on a failing suite" ;;
  *) fail "C03: a failing suite was not reported as uncached: $r" ;; esac
tree="$(cd "$d" && git rev-parse HEAD^{tree})"
[ -f "$(cache_file "$d" "$tree")" ] && fail "C03: a cache file exists despite the suite failing"
r="$(run_push "$d" backup "$sha")"
case "$r" in exit=1*) pass "C03: pre-push still refuses after a failed verify (nothing was cached)" ;;
  *) fail "C03: pre-push allowed a push whose only verify run failed: $r" ;; esac

# ── C04 (MUTATION, criterion 5): a cached verdict for ANOTHER tree does not
# authorize THIS push — even in the SAME repo, right after a real cache hit. ─
#
# THE SECOND COMMIT MUST TOUCH SUITE_TRIGGER_PATTERN ITSELF. setup/tests/128
# added a legitimate shortcut where an ANCESTOR's verdict covers a later tip
# that changes nothing pattern-matching — a plain `file.txt` v1→v2 edit is
# exactly that legitimate case now, not a mutation this test should catch.
# Touching bin/hw in the second commit makes sha2 itself the most recent
# commit that touches the pattern, so the shortcut cannot apply and this
# stays a real regression guard: an unrelated cache must not authorize it.
d="$(mk_repo)"; stub_suite "$d" 0 0
printf 'v1\n' > "$d/file.txt"; git_commit_all "$d" "v1"
sha1="$(cd "$d" && git rev-parse HEAD)"
run_verify "$d" >/dev/null
mkdir -p "$d/bin"; printf '#!/usr/bin/env bash\necho hw\n' > "$d/bin/hw"; git_commit_all "$d" "v2, touches bin/hw itself"
sha2="$(cd "$d" && git rev-parse HEAD)"
[ "$sha1" != "$sha2" ] || fail "C04: the second commit did not produce a different sha — fixture is broken"
r="$(run_push "$d" backup "$sha2")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C04: a verdict cached for sha1's tree does not authorize pushing sha2's different tree, whose own change touches the pattern" ;;
  *) fail "C04: pushing a NEW tree was allowed by an OLD tree's cached verdict: $r" ;;
esac
# and the OLD tree's cache still authorizes pushing sha1 itself, unaffected
r="$(run_push "$d" backup "$sha1")"
case "$r" in exit=0*) pass "C04: sha1's own cached verdict still authorizes pushing sha1" ;;
  *) fail "C04: sha1's cache was lost or invalidated by the later commit: $r" ;; esac

# ── C05 (MUTATION, criterion 5): a DIRTY tracked tree cannot cache or reuse a
# verdict, even if the suite would pass. ────────────────────────────────────
#
# THE DIRTIED FILE MUST TOUCH SUITE_TRIGGER_PATTERN. setup/tests/127 narrowed
# this refusal to only paths that can invalidate the suite — dirtying plain
# `file.txt` is now legitimately allowed and is 127's job to cover, not a
# regression this file should catch. Dirtying bin/hw keeps this a real guard:
# tracked, pattern-matching content that is genuinely dirty must still block.
d="$(mk_repo)"; stub_suite "$d" 0 0
mkdir -p "$d/bin"; printf '#!/usr/bin/env bash\necho hw\n' > "$d/bin/hw"; git_commit_all "$d" "clean bin/hw"
sha="$(cd "$d" && git rev-parse HEAD)"
printf '#!/usr/bin/env bash\necho hw dirty\n' > "$d/bin/hw"   # modify a TRACKED, pattern-matching file, do not commit
r="$(run_verify "$d")"
case "$r" in
  exit=1*"can invalidate the suite"*) pass "C05: verify-for-push refuses to run against a dirty tracked working tree that touches SUITE_TRIGGER_PATTERN" ;;
  *) fail "C05: a dirty tracked tree touching the pattern was not refused: $r" ;;
esac
tree="$(cd "$d" && git rev-parse HEAD^{tree})"
[ -f "$(cache_file "$d" "$tree")" ] && fail "C05: a cache file was written despite the dirty refusal"
r="$(run_push "$d" backup "$sha")"
case "$r" in exit=1*) pass "C05: pre-push still refuses HEAD's tree — the dirty run cached nothing" ;;
  *) fail "C05: a dirty-tree run somehow still authorized the push: $r" ;; esac
# UNTRACKED files are a different matter — this repo is shared by several
# concurrent agents and its working tree carries other agents' unrelated
# untracked files at almost every moment (setup/CLAUDE.md); requiring those
# clean too would make verify-for-push unusable in its own home repo.
printf 'v2\n' > "$d/file.txt"; git_commit_all "$d" "v2 clean again"
sha2="$(cd "$d" && git rev-parse HEAD)"
printf 'irrelevant\n' > "$d/untracked-from-another-agent.md"
r="$(run_verify "$d")"
case "$r" in exit=0*"PASSED"*) pass "C05: an untracked file elsewhere in the tree does not block verification" ;;
  *) fail "C05: verify-for-push refused over an untracked file, which is not what makes a tree dirty here: $r" ;; esac
rm -f "$d/untracked-from-another-agent.md"

# ── C06: a DELETE (local sha all-zero) needs no cached verdict ─────────────
d="$(mk_repo)"; stub_suite "$d" 0 0
printf 'x\n' > "$d/file.txt"; git_commit_all "$d"
r="$(run_push "$d" backup "$ZERO")"
case "$r" in exit=0*) pass "C06: deleting a ref needs no cached verdict — nothing new is being published" ;;
  *) fail "C06: a pure deletion was refused: $r" ;; esac

# ── C07 (MUTATION, criterion 4): if EITHER of two refs pushed together lacks a
# cached verdict, the WHOLE push is refused, not just the uncovered one. ────
#
# other-branch's OWN new commit MUST touch SUITE_TRIGGER_PATTERN. setup/
# tests/128 added a shortcut where an ancestor's verdict covers a later,
# non-triggering commit — main's root commit here already touches
# setup/test-hw (stub_suite writes it before the first commit), so a
# file.txt-only commit on other-branch would legitimately inherit that
# verdict via the shortcut and stop being "uncovered". Touching bin/other
# keeps other-branch's tip itself the most recent trigger commit, with no
# cache of its own — genuinely uncovered, which is what this test is for.
d="$(mk_repo)"; stub_suite "$d" 0 0
printf 'x\n' > "$d/file.txt"; git_commit_all "$d"
sha_ok="$(cd "$d" && git rev-parse HEAD)"
run_verify "$d" >/dev/null
tree_ok="$(cd "$d" && git rev-parse HEAD^{tree})"
[ -f "$(cache_file "$d" "$tree_ok")" ] || fail "C07: setup fixture did not cache the first ref's verdict"
( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git checkout -qb other-branch )
mkdir -p "$d/bin"; printf '#!/usr/bin/env bash\necho other\n' > "$d/bin/other"; git_commit_all "$d" "other, unverified"
sha_bad="$(cd "$d" && git rev-parse HEAD)"
rc=0
out="$(cd "$d" && { printf 'refs/heads/main %s refs/heads/main %s\n' "$sha_ok" "$ZERO"
                    printf 'refs/heads/other %s refs/heads/other %s\n' "$sha_bad" "$ZERO"; } \
      | env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE bash ./setup/hooks/pre-push backup 2>&1)" || rc=$?
case "$out$rc" in
  *"refs/heads/other"*[!0]) pass "C07: one uncovered ref among several refuses the ENTIRE push, naming it" ;;
  *) fail "C07: a push with one covered and one uncovered ref was not fully refused: rc=$rc out=$out" ;;
esac

# ── C08: pre-push is FAST, always — cache hit, cache miss, and a stub suite
# that WOULD be slow if pre-push ever ran it (it must not). ─────────────────
d="$(mk_repo)"; stub_suite "$d" 0 $((5 * HW_TEST_SLOW))   # 5s (x the platform factor) if invoked — pre-push must never invoke it
printf 'x\n' > "$d/file.txt"; git_commit_all "$d"
sha="$(cd "$d" && git rev-parse HEAD)"
t0=$(date +%s); run_push "$d" backup "$sha" >/dev/null; t1=$(date +%s)
miss_elapsed=$(( t1 - t0 ))
[ "$miss_elapsed" -lt $((3 * HW_TEST_SLOW)) ] || fail "C08: pre-push took ${miss_elapsed}s on a cache MISS — it must never approach the suite's own runtime"
pass "C08: pre-push's cache-miss refusal is fast (${miss_elapsed}s)"
run_verify "$d" >/dev/null 2>&1 || true   # this one DOES take ~5s — that cost is verify-for-push's, not the hook's
t0=$(date +%s); run_push "$d" backup "$sha" >/dev/null; t1=$(date +%s)
hit_elapsed=$(( t1 - t0 ))
[ "$hit_elapsed" -lt $((3 * HW_TEST_SLOW)) ] || fail "C08: pre-push took ${hit_elapsed}s on a cache HIT — a cached verdict must not cost anything close to a suite run"
pass "C08: pre-push's cache-hit approval is fast (${hit_elapsed}s) — the 5s the stub suite would sleep never happens inside the hook"

# ── C09 (MUTATION): a hook that skips the cache check entirely is not coverage
# — prove the check is load-bearing by removing it and showing an uncached
# push gets through. ─────────────────────────────────────────────────────────
d="$(mk_repo)"; stub_suite "$d" 0 0
printf 'x\n' > "$d/file.txt"; git_commit_all "$d"
sha="$(cd "$d" && git rev-parse HEAD)"
python3 - "$d/setup/hooks/pre-push" <<'PY'
import re, sys
path = sys.argv[1]
src = open(path).read()
start = src.index('cache_dir="$(git rev-parse --git-common-dir)')
mutated = src[:start]  # guts the whole coverage-check block through EOF
open(path, "w").write(mutated)
PY
grep -q 'cache_dir=' "$d/setup/hooks/pre-push" && fail "C09 mutant: the coverage-check block was not removed — mutation setup is broken"
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=0*) pass "mutant killed: removing the cache-check block lets an uncached push through — the check is load-bearing, not decorative" ;;
  *) fail "C09 mutant: removing the check should have made the push succeed trivially (proving the check normally refuses it), but got: $r" ;;
esac

# ── C10 (MUTATION): the cache check is CONTENT, not merely EXISTENCE ───────
# Found by adversarial review 2026-09-10: a bare `[ -f "$cache_dir/$tree" ]`
# would trust ANY file at the right path/name — a zero-byte file, a stray
# `touch`, a write caught mid-flight — as if it were a real verdict from
# verify-for-push. Prove the real hook does NOT do that: an empty file at
# the exact right path, under the exact right name, must still be refused.
d="$(mk_repo)"; stub_suite "$d" 0 0
printf 'x\n' > "$d/file.txt"; git_commit_all "$d"
sha="$(cd "$d" && git rev-parse HEAD)"
tree="$(cd "$d" && git rev-parse HEAD^{tree})"
mkdir -p "$d/.git/hw-push-verified"
: > "$d/.git/hw-push-verified/$tree"   # exists, right name, EMPTY — not a real verdict
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C10: an empty file at the right cache path is refused — existence alone is not a verdict" ;;
  *) fail "C10: an empty placeholder file at the right path was accepted as a real verdict: $r" ;;
esac
# and content naming a DIFFERENT tree, sitting under the file for THIS one, is
# refused the same way — the filename alone was never the whole check either.
printf 'commit=deadbeef\ntree=0000000000000000000000000000000000000000\n' > "$d/.git/hw-push-verified/$tree"
r="$(run_push "$d" backup "$sha")"
case "$r" in
  exit=1*"no cached full-suite verdict"*) pass "C10: a cache file whose OWN content names a different tree is refused, even under the right filename" ;;
  *) fail "C10: a cache file with mismatched internal content was accepted: $r" ;;
esac
# the real thing, from a real verify-for-push run, still works
rm -f "$d/.git/hw-push-verified/$tree"
run_verify "$d" >/dev/null
r="$(run_push "$d" backup "$sha")"
case "$r" in exit=0*) pass "C10: a genuine verify-for-push verdict still authorizes the push" ;;
  *) fail "C10: a real cached verdict was rejected after tightening the check: $r" ;; esac
