#!/usr/bin/env bash
# The fast/slow gate split: every subject file lands in exactly one gate, the
# split is derived from setup/test-budgets.json (never a hand-kept list), and
# a file that runs slower than its committed number cannot quietly stay fast.
#
# suite-lane: exclusive — this subject drives a NESTED fast-gate runner over its
#   own fixtures, and the fast gate refuses a subject that overran its budget
#   ON THAT RUN. Measured 2026-09-16 with fourteen jobs: the fixture subject
#   `01-fast.sh` took 4s against a 2s budget and the nested runner refused it —
#   correctly, by its own contract, and about nothing: the machine was busy.
#
# Run alone while working on this subject:
#     bash setup/tests/117-two-gates-cover-every-subject.sh
#
# Companion coverage: setup/tests/118-verify-timeout-coherence.sh (the
# _VERIFY_TIMEOUT/full-suite mismatch) and setup/tests/119-the-gate-does-not-
# break-the-push.sh (pre-push's cached-verdict mandatory-coverage mechanism —
# also 112's subject now that pre-push no longer runs the suite inline). Run
# all four:
#     for f in 117 112 118 119; do bash setup/tests/$f-*.sh; done
# suite-timing: retry-once — its nested runner refuses a fixture that overran its time budget; measured 2026-09-30:
#   red under load 11-13 from other lanes, green alone.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

REAL_HW="$ROOT/setup/test-hw"
[ -x "$REAL_HW" ] || fail "gate: $REAL_HW is missing"

# ── 1. THE REAL, COMMITTED MANIFEST COVERS EVERY FILE ON DISK ───────────────
# This is the invariant itself, checked against production data: a file that
# stopped being classified is a file that stopped running in either gate, and
# that must fail loudly rather than default silently into one side.
BUDGETS="$ROOT/setup/test-budgets.json"
[ -f "$BUDGETS" ] || fail "gate: $BUDGETS is missing — this is what setup/measure-tests.sh produces"
python3 - "$BUDGETS" "$ROOT/setup/tests" <<'PY' || exit 1
import json, pathlib, sys
budgets = json.loads(pathlib.Path(sys.argv[1]).read_text())
tests_dir = pathlib.Path(sys.argv[2])
on_disk = sorted(p.name for p in tests_dir.glob("[0-9][0-9]*-*.sh"))
listed = sorted(budgets["files"].keys())
if on_disk != listed:
    missing = sorted(set(on_disk) - set(listed))
    extra = sorted(set(listed) - set(on_disk))
    print(f"not ok - gate: budgets.json and tests/ disagree — missing={missing} extra={extra}", file=sys.stderr)
    sys.exit(1)
print(f"ok - gate: setup/test-budgets.json classifies all {len(on_disk)} subject files on disk, 1:1")
PY
threshold="$(python3 -c "import json; print(json.load(open('$BUDGETS'))['fast_gate_threshold_seconds'])")"
[ -n "$threshold" ] || fail "gate: no fast_gate_threshold_seconds in $BUDGETS — the threshold has no owner"
pass "gate: the fast gate threshold is a single named value in the manifest ($threshold s), not a hand-kept list"

# ── 2. A FIXTURE COPY OF THE RUNNER, so the mutation arms below cost tens of
# milliseconds each instead of re-running production's ~1000s suite. ────────
#
# It is a REAL copy of setup/test-hw driving REAL stub subject files through
# its REAL fast-gate machinery (the completeness check and the live per-file
# timing enforcement added alongside this file) — only the subject files and
# the budgets manifest are fixtures.
mk_gate_repo() { # (no args) → prints the fixture root
  # THE SNAPSHOT SCRIPT SCANS ONE LEVEL ABOVE `setup/`, WHOLESALE — so the
  # fixture must reproduce that shape (<root>/setup/test-hw, not
  # <root>/test-hw) or SOURCE_ROOT resolves to something OUTSIDE the fixture
  # (an ancestor $TMP shared with other fixtures, or /tmp itself) and the
  # snapshot trips on whatever unrelated bytes happen to sit there. Measured
  # here: a stray `.s.PGSQL.5432` socket file in /tmp aborted an earlier draft
  # of this fixture before it ever reached the gate logic being tested.
  local d; d="$(mktemp -d "$TMP/gaterepo-XXXXXX")/root"  # UNIQUE per call — a fixed path let one case's leftover stub files survive into the next case's completeness check, reported as spurious "unclassified" files that were really just uncleaned fixture debris.
  mkdir -p "$d/setup/tests"
  cp "$REAL_HW" "$d/setup/test-hw"
  cp "$ROOT/setup/fast-gate-budget.sh" "$d/setup/fast-gate-budget.sh"
  cp "$ROOT/setup/test-hw-snapshot.py" "$d/setup/test-hw-snapshot.py"
  cp "$ROOT/setup/mutation-coverage" "$d/setup/mutation-coverage"
  cp "$ROOT/setup/tests/_common.sh" "$d/setup/tests/_common.sh"
  ( cd "$d" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git init -q )
  printf '%s' "$d"
}
# git_commit_all <dir> — the fixture must be a real git work tree for test-hw's
# tracked/untracked accounting to take the normal path rather than the
# "NO TRACKED NUMBER" branch that only fires outside one.
git_commit_all() {
  ( cd "$1" && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE git add -A \
      && env -u GIT_DIR -u GIT_INDEX_FILE -u GIT_WORK_TREE \
        git -c user.email=t@t -c user.name=t commit -q -m fixture )
}
# stub_subject <dir> <name> <sleep-seconds>  — a subject that sleeps then passes
stub_subject() {
  cat > "$1/setup/tests/$2" <<STUB
#!/usr/bin/env bash
sleep $3
echo "ok - stub $2 ran"
STUB
  chmod +x "$1/setup/tests/$2"
}
write_budgets() { # <dir> <threshold> <name=seconds> ...
  local d="$1" threshold="$2"; shift 2
  local body="" first=1
  for pair in "$@"; do
    name="${pair%%=*}"; secs="${pair#*=}"
    [ "$first" = 1 ] || body="$body,"
    first=0
    body="$body\"$name\": {\"seconds\": $secs, \"exit\": 0}"
  done
  printf '{"fast_gate_threshold_seconds": %s, "files": {%s}}\n' "$threshold" "$body" > "$d/setup/test-budgets.json"
}

# ── C01: the fast gate runs only files classified at/under threshold ───────
d="$(mk_gate_repo)"
stub_subject "$d" "01-fast.sh" 0
stub_subject "$d" "02-slow.sh" 0   # the STUB is quick; its CLAIMED time is not
write_budgets "$d" 2 "01-fast.sh=0.1" "02-slow.sh=99"
git_commit_all "$d"
rc=0; out="$(cd "$d" && HW_TEST_GATE=fast bash ./setup/test-hw 2>&1)" || rc=$?
case "$out" in
  *"stub 01-fast.sh ran"*) : ;;
  *) fail "C01: the fast-classified file did not run: $out" ;;
esac
case "$out" in
  *"stub 02-slow.sh ran"*) fail "C01: the file classified OVER threshold ran in the fast gate — it should have been excluded" ;;
  *) pass "C01: the fast gate ran the file classified fast and excluded the one classified slow" ;;
esac
[ "$rc" = 0 ] || fail "C01: the fast gate refused a run where every included file was legitimately fast: $out"
case "$out" in *"FAST GATE:"*"1 tests passed"*) pass "C01: the fast-gate headline is labelled, and its count is scoped to what ran" ;;
  *) fail "C01: no labelled FAST GATE headline: $out" ;; esac

# ── C02 (MUTATION): a file that runs SLOWER than its committed number is
# caught on THIS run, by the clock — not trusted from the stale manifest. ───
d="$(mk_gate_repo)"
stub_subject "$d" "01-liar.sh" 3     # really sleeps 3s
write_budgets "$d" 1 "01-liar.sh=0.1"   # manifest claims 0.1s, threshold is 1s
git_commit_all "$d"
# An unsaturated machine pinned by HW_TEST_LOAD1/NCPU: the budget is judged on wall.
rc=0; out="$(cd "$d" && HW_TEST_LOAD1=0.1 HW_TEST_NCPU=8 HW_TEST_GATE=fast bash ./setup/test-hw 2>&1)" || rc=$?
case "$out$rc" in
  *"took"*"over the"*"budget"*[!0]) pass "C02: a file that actually runs over threshold is refused by live timing, regardless of what the manifest claimed" ;;
  *) fail "C02: a file that measured 3s against a 1s budget was not caught: rc=$rc out=$out" ;;
esac

# ── C03 (MUTATION, completeness): a file on disk with NO manifest entry is
# refused rather than defaulted into either gate. ───────────────────────────
d="$(mk_gate_repo)"
stub_subject "$d" "01-known.sh" 0
stub_subject "$d" "02-unclassified.sh" 0
write_budgets "$d" 2 "01-known.sh=0.1"   # 02-unclassified.sh is not listed
git_commit_all "$d"
rc=0; out="$(cd "$d" && HW_TEST_GATE=fast bash ./setup/test-hw 2>&1)" || rc=$?
case "$out$rc" in
  *"does not match"*"1:1"*[!0]) pass "C03: an unclassified file on disk refuses the fast gate rather than silently joining a gate" ;;
  *) fail "C03: an unclassified file was not caught: rc=$rc out=$out" ;;
esac

# ── C04 (MUTATION, the other half of completeness): a STALE manifest entry
# for a file that no longer exists on disk is refused too. ─────────────────
d="$(mk_gate_repo)"
stub_subject "$d" "01-known.sh" 0
write_budgets "$d" 2 "01-known.sh=0.1" "02-deleted.sh=0.1"
git_commit_all "$d"
rc=0; out="$(cd "$d" && HW_TEST_GATE=fast bash ./setup/test-hw 2>&1)" || rc=$?
case "$out$rc" in
  *"does not match"*"1:1"*[!0]) pass "C04: a manifest entry for a deleted file refuses the fast gate rather than being silently ignored" ;;
  *) fail "C04: a stale manifest entry was not caught: rc=$rc out=$out" ;;
esac

# ── C05: HW_TEST_GATE=full (the default, and what pre-push uses) is
# UNCHANGED — every file runs, and the guard block still runs too. ─────────
d="$(mk_gate_repo)"
mkdir -p "$d/setup/guards"
for _g in test-deny-repo-writes.mjs test-deny-repo-writes-filesystem.mjs \
          test-opencode-hw-blocked-reason.mjs test-herdr-opencode-background-state.mjs; do
  printf 'console.log("ok - stub %s ran");\n' "$_g" > "$d/setup/guards/$_g"
done
cat > "$d/setup/guards/mutate-deny-repo-writes.sh" <<'STUB'
#!/usr/bin/env bash
echo "ok - stub guard ran"
STUB
chmod +x "$d/setup/guards/mutate-deny-repo-writes.sh"
stub_subject "$d" "01-fast.sh" 0
stub_subject "$d" "02-slow.sh" 0
write_budgets "$d" 2 "01-fast.sh=0.1" "02-slow.sh=99"
git_commit_all "$d"
rc=0; out="$(cd "$d" && bash ./setup/test-hw 2>&1)" || rc=$?
case "$out" in
  *"stub 01-fast.sh ran"*"stub 02-slow.sh ran"*) : ;;
  *) fail "C05: default (full) gate did not run every subject file: $out" ;;
esac
case "$out" in
  *"stub guard ran"*) pass "C05: default (full) gate still runs the read-only guard's mutation arms, and a fast-classified file's own budget does not gate it" ;;
  *) fail "C05: the guard block did not run under HW_TEST_GATE=full (default): $out" ;;
esac
case "$out" in *"FAST GATE:"*) fail "C05: the full gate printed the fast-gate-only headline label" ;; *) : ;; esac
[ "$rc" = 0 ] || fail "C05: the full gate refused a run where every file was legitimate: $out"

# ── C06 (MUTATION): an invalid HW_TEST_GATE value is refused, not silently
# treated as full or fast. ──────────────────────────────────────────────────
d="$(mk_gate_repo)"
stub_subject "$d" "01-fast.sh" 0
write_budgets "$d" 2 "01-fast.sh=0.1"
git_commit_all "$d"
rc=0; out="$(cd "$d" && HW_TEST_GATE=bogus bash ./setup/test-hw 2>&1)" || rc=$?
case "$out$rc" in
  *"must be full or fast"*[!0]) pass "C06: an unknown HW_TEST_GATE value is refused" ;;
  *) fail "C06: an unknown HW_TEST_GATE value was not refused: rc=$rc out=$out" ;;
esac

# ── C07: pre-commit passes HW_TEST_GATE=fast to test-hw ────────────────────
# Checked against the REAL hook, not a fixture — the fixtures above prove the
# mechanism works, this proves the mechanism is WIRED.
grep -q 'HW_TEST_GATE=fast bash "\$root/setup/test-hw"' "$ROOT/setup/hooks/pre-commit" \
  || fail "C07: setup/hooks/pre-commit no longer passes HW_TEST_GATE=fast to test-hw"
pass "C07: pre-commit's suite invocation is wired to the fast gate"

# ── C08: pre-push does NOT invoke test-hw at all ────────────────────────────
# setup/tests/119-the-gate-does-not-break-the-push.sh owns pre-push's mandatory-
# coverage guarantee in full (the cached-verdict mechanism, tree-exactness,
# dirty-tree refusal, and the timing bound) — measured 2026-09-10 that running
# the suite INSIDE pre-push holds a remote socket open long enough for GitHub to
# close it mid-push, so that inline invocation this file used to check for is
# gone by design, not by omission. What belongs HERE, in the file that owns
# "every subject file is in exactly one gate", is the one structural fact that
# would silently reopen that hole: pre-push must never again shell out to
# setup/test-hw directly. If it does, the full suite is back on the push path
# and setup/tests/119's whole premise is moot.
grep -qF 'bash "$root/setup/test-hw"' "$ROOT/setup/hooks/pre-push" \
  && fail "C08: setup/hooks/pre-push invokes setup/test-hw directly again — that is the exact regression setup/tests/119 exists to prevent (a slow hook holding a remote socket open)"
pass "C08: pre-push does not shell out to setup/test-hw — coverage is enforced by a cached verdict, checked in setup/tests/119"
