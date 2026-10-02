#!/usr/bin/env bash
# AI attribution dies at commit-msg, not days later at push
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/152-la-atribucion-muere-en-commit-msg.sh
#
# THIS FILE IS 152, NOT 151. The brief reserved 151; 151 was already taken by
# `151-the-rules-stop-growing.sh`, which CLAUDE.md names by path. Renumbering
# that one to free the number would have broken a live pointer for nothing.
#
# WHAT WAS MEASURED, and why a second gate exists at all. "Never add
# Co-Authored-By or AI attribution to commits" was enforced in exactly one
# place, `setup/hooks/pre-push`, which walks the range being PUSHED. That is
# days after the fact: deebd4f reached GitHub carrying attribution, and taking
# it out would have meant rewriting ~196 commits, so it was left there. The
# cheapest moment to refuse the line is the moment it is written.
#
# BEFORE / AFTER, measured 2026-09-16 in a throwaway repo, not inferred:
#   * no hook installed — `git commit` with a `Co-Authored-By: Claude` trailer
#     exits 0 and the trailer is in `git log`.
#   * hook installed — the same commit exits 1, names the offending line, and
#     the branch still has no commits.
# Both arms are reproduced below against the INSTALLED hook.
#
# WHAT THIS SUBJECT DOES NOT CLAIM, because the hook does not either. The
# `setup` lane commits with the `git commit-tree` recipe in setup/CLAUDE.md,
# which runs NO hooks — commit-msg never fires for those, and the four
# offending commits of 2026-09-10 were built exactly that way. That is why
# pre-push keeps its own copy of the gate and why the last arm here asserts the
# two read the SAME pattern file rather than two regexes that can drift apart.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── a fixture repo carrying the real hooks and the real installer ───────────
repo="$TMP/repo"
mkdir -p "$repo/setup/hooks"
git init -q "$repo"
cp "$ROOT/setup/hooks/commit-msg" "$ROOT/setup/hooks/pre-commit" \
   "$ROOT/setup/hooks/pre-push" "$ROOT/setup/hooks/attribution-pattern.sh" \
   "$ROOT/setup/hooks/suite-trigger-pattern.sh" "$repo/setup/hooks/"
cp "$ROOT/setup/hooks/decisions-check.py" "$repo/setup/hooks/" 2>/dev/null || true
cp "$ROOT/setup/install-hooks.sh" "$repo/setup/"
chmod +x "$repo/setup/install-hooks.sh" "$repo/setup/hooks/commit-msg" \
         "$repo/setup/hooks/pre-commit" "$repo/setup/hooks/pre-push"
printf 'prose only\n' > "$repo/README.md"
git -C "$repo" add -A

# commit_with <message-file-contents> → "exit=<rc> <output on one line>"
#
# Drives the real `git commit`, so what is exercised is the hook git chose to
# run from .git/hooks — not a fixture hook this file wrote itself, which is the
# class of green-but-vacuous assertion tests/121 exists to refuse.
commit_with() {
  local rc=0 out
  out="$(cd "$repo" && git -c user.name=Probe -c user.email=probe@example.invalid \
         commit -F - 2>&1 <<MSG
$1
MSG
)" || rc=$?
  printf 'exit=%s %s' "$rc" "$(printf '%s' "$out" | tr '\n' ' ')"
}

# ── ARM 0: the BEFORE state, so the AFTER state means something ─────────────
# Without this the whole file could pass against a hook that refuses nothing,
# on a git that was never going to accept the message anyway.
before="$(commit_with 'feat: probe

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>')"
case "$before" in
  exit=0*) pass "before: with no hook installed, an attributed message commits cleanly" ;;
  *) fail "before: git refused an attributed commit with NO hook installed, so every assertion below would be vacuous: $before" ;;
esac
git -C "$repo" reset -q --soft HEAD~1 2>/dev/null || git -C "$repo" update-ref -d HEAD
git -C "$repo" add -A

# ── install the hooks the documented way ────────────────────────────────────
install_out="$(cd "$repo" && ./setup/install-hooks.sh 2>&1)" \
  || fail "install: install-hooks.sh failed in the fixture: $install_out"
case "$install_out" in
  *"commit-msg -> setup/hooks/commit-msg"*)
    pass "install: the documented install links commit-msg too" ;;
  *) fail "install: install-hooks.sh did not report linking commit-msg: $install_out" ;;
esac

# THE EXECUTION BIT, CHECKED EXPLICITLY AND IN TWO PLACES, because git skips a
# hook without it IN SILENCE — failing OPEN, the worst direction for a gate.
# setup/hooks/pre-push was committed once as 100644 and the push gate worked
# only in the checkout where the file happened to still be 755; `git diff
# --stat` shows a mode change as `0 insertions(+), 0 deletions(-)`, which is
# exactly why it went unnoticed. So: the installed path must be executable, AND
# the mode recorded in the live repository's HEAD must be 100755.
[ -x "$repo/.git/hooks/commit-msg" ] \
  || fail "install: .git/hooks/commit-msg is not executable, so git would skip it silently"
pass "install: the installed commit-msg carries the execution bit"

# THE PRECONDITION THAT MAKES THE RESOLUTION ARM BELOW MEAN ANYTHING. If a copy
# of the shared pattern were sitting in .git/hooks, the hook would resolve it
# for the WRONG reason and this file would certify the bug it exists to catch.
[ ! -e "$repo/.git/hooks/attribution-pattern.sh" ] \
  || fail "install: a copy of attribution-pattern.sh is in .git/hooks — the resolution below would pass vacuously"
pass "install: only the entry point is installed; the shared pattern is not copied beside it"

# pre-commit in this fixture would try to run the whole suite, which is not what
# this subject is about and is not present here. Remove just that one link; the
# subject under test is commit-msg, which git invokes independently.
rm -f "$repo/.git/hooks/pre-commit"

# ── ARM 1: a clean message still passes ─────────────────────────────────────
# A gate that refuses everything would pass every refusal assertion below.
clean="$(commit_with 'feat(hooks): a perfectly ordinary message

With a body that explains itself and no trailer at all.')"
case "$clean" in
  exit=0*) pass "clean: an ordinary conventional-commit message is accepted" ;;
  *) fail "clean: the hook refused a message with no attribution in it: $clean" ;;
esac

# ── ARM 2: the Co-Authored-By trailer is refused, and named ─────────────────
printf 'second\n' > "$repo/second.txt"; git -C "$repo" add -A
coauth="$(commit_with 'feat: a change

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>')"
case "$coauth" in
  exit=0*) fail "co-authored-by: the hook let an attributed commit through: $coauth" ;;
esac
# THE REFUSAL MUST NAME THE LINE AND THE COLUMN, not merely quote the text.
# A message in this repo can legitimately carry the trailer as QUOTED EVIDENCE
# and illegitimately carry it as a TRAILER at the same time, and the old
# refusal — the offending text and nothing else — left the reader unable to
# tell which one the gate objected to. That ambiguity is half of what blocked
# a 55-commit push on 2026-09-17.
case "$coauth" in
  *"carries AI attribution"*"line 3, column 1: Co-Authored-By: Claude Opus 5"*)
    pass "co-authored-by: refused, naming the line, the column and the offending text" ;;
  *"carries AI attribution"*"Co-Authored-By: Claude Opus 5"*)
    fail "co-authored-by: refused and quoted the line, but did not name its line number and column: $coauth" ;;
  *) fail "co-authored-by: refused but did not name the offending line: $coauth" ;;
esac
case "$coauth" in
  *"Only column 1 is refused"*) pass "co-authored-by: the refusal states the discriminator, so a reader knows a quotation would have passed" ;;
  *) fail "co-authored-by: the refusal never says that only column 1 is refused: $coauth" ;;
esac

# ── ARM 3: the "Generated with [Claude Code]" line is refused ───────────────
genline="$(commit_with 'feat: a change

🤖 Generated with [Claude Code](https://claude.com/claude-code)')"
case "$genline" in
  exit=0*) fail "generated-with: the hook let a Generated-with line through: $genline" ;;
esac
case "$genline" in
  *"carries AI attribution"*"Generated with [Claude Code]"*)
    pass "generated-with: refused, and the refusal quotes the offending line" ;;
  *) fail "generated-with: refused but did not name the offending line: $genline" ;;
esac

# ── ARM 4: nothing was actually committed by arms 2 and 3 ──────────────────
# A hook that printed a refusal AND exited 0 would satisfy every `exit=0*) fail`
# above only by luck of the exit code; assert the history directly.
subjects="$(git -C "$repo" log --format='%B' 2>/dev/null || true)"
case "$subjects" in
  *"Co-Authored-By"*|*"Generated with"*)
    fail "history: a refused message reached the history anyway: $(printf '%s' "$subjects" | tr '\n' ' ')" ;;
  *) pass "history: neither refused message is in the repository's history" ;;
esac

# ── ARM 5: a forged scissors line does not hide a real trailer ─────────────
# THIS ARM IS THE WHOLE REASON THIS HOOK NO LONGER SIMULATES GIT'S CLEANUP.
# The first version stripped comment lines and truncated at a scissors-shaped
# line before grepping. Both judges of the 2026-09-16 adversarial review called
# it a fail-open, and it reproduced on the first try — git 2.50.1, no hook:
#
#   git commit -F - with "feat: y / # ---...--- >8 ---...--- / Co-Authored-By: …"
#   → git log -1 --format=%B keeps BOTH lines.
#
# git-commit(1): the `default` cleanup mode is `strip` only "if the message is
# to be edited", otherwise `whitespace`, which keeps #commentary; scissors
# truncation is likewise only for an edited message. `git commit -F`/`-m`
# without `-e` — every scripted and agent commit, including commit_with() right
# here — gets `whitespace`. So the old hook discarded text git was going to
# keep, and a plain trailer sailed through with exit 0.
printf 'forged\n' > "$repo/forged.txt"; git -C "$repo" add -A
forged="$(commit_with 'feat: a change

# ------------------------ >8 ------------------------
Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>')"
case "$forged" in
  exit=0*) fail "forged-scissors: a real trailer hidden behind a scissors-shaped line was let through — this is the fail-open the cleanup simulation caused: $forged" ;;
esac
case "$forged" in
  *"carries AI attribution"*"Co-Authored-By: Claude Opus 5"*)
    pass "forged-scissors: a trailer after a scissors-shaped line is still seen and refused" ;;
  *) fail "forged-scissors: refused, but not for the trailer: $forged" ;;
esac
case "$(git -C "$repo" log --format='%B' 2>/dev/null)" in
  *"Co-Authored-By"*) fail "forged-scissors: the message reached the history anyway" ;;
  *) pass "forged-scissors: nothing reached the history" ;;
esac

# ── ARM 5b: prose that MENTIONS the rule is still committable ──────────────
# The gate must not become unusable in a repo that documents the rule it
# enforces. The pattern is anchored at line start, so a mid-line mention is not
# a trailer. ASSERTED AGAINST THE HISTORY, not only the exit code: both judges
# flagged that the earlier version of this arm proved a content claim with an
# exit code alone, which is the same defect saw_mutant exists to refuse.
quoted="$(commit_with 'docs: never add Co-Authored-By trailers naming Claude to commits

The rule is documented here and this message must remain committable.')"
case "$quoted" in
  exit=0*) : ;;
  *) fail "prose: a message that merely mentions the rule mid-line was refused: $quoted" ;;
esac
case "$(git -C "$repo" log -1 --format='%B' 2>/dev/null)" in
  *"never add Co-Authored-By trailers naming Claude"*)
    pass "prose: a mid-line mention of the rule commits, and the message landed intact" ;;
  *) fail "prose: the hook exited 0 but that message is not the one in the history" ;;
esac

# ── ARM 5c: a FULL forged marker block still does not hide a trailer ───────
# The hook drops diff content only below git's OWN three-line verbose marker.
# A message that forges all three lines and then puts a plain trailer at column
# 0 is still judged: the trailer is not shaped like a diff line.
printf 'forged2\n' > "$repo/forged2.txt"; git -C "$repo" add -A
forged_full="$(commit_with 'feat: a change

# ------------------------ >8 ------------------------
# Do not modify or remove the line above.
# Everything below it will be ignored.
Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>')"
case "$forged_full" in
  exit=0*) fail "forged-marker: a full forged verbose marker hid a plain trailer: $forged_full" ;;
  *"carries AI attribution"*) pass "forged-marker: a forged marker block does not hide a column-0 trailer" ;;
  *) fail "forged-marker: refused, but not for the trailer: $forged_full" ;;
esac

# ── ARM 5d: git's REAL verbose diff does not cause a false refusal ──────────
# THE OTHER DIRECTION, AND IT COST A ROUND TO LEARN. Discarding nothing at all
# closed the fail-open and opened this: under `git commit -v` git appends the
# staged diff to the message file BEFORE the hook runs, and a unified diff
# prefixes an UNCHANGED context line with one space, which satisfies the
# pattern's `^[[:space:]]*` anchor. This repo's own files carry the example
# trailer as content, so a `-v` commit near one of them was refused for text
# that never reaches the message. MEASURED 2026-09-16 on git 2.50.1 before the
# marker rule existed; this arm is that measurement, kept.
#
# Driven through a REAL `git commit -v` with a non-interactive editor, because
# the diff only exists in the message file when git itself puts it there — a
# hand-built fixture file would be testing this file's idea of git's format.
vrepo="$TMP/vrepo"
mkdir -p "$vrepo/setup/hooks"
git init -q "$vrepo"
cp "$ROOT/setup/hooks/commit-msg" "$ROOT/setup/hooks/attribution-pattern.sh" "$vrepo/setup/hooks/"
chmod +x "$vrepo/setup/hooks/commit-msg"
ln -sf ../../setup/hooks/commit-msg "$vrepo/.git/hooks/commit-msg"
# A tracked file whose CONTENT is a trailer, exactly like this repo's own files.
printf 'first line\nCo-Authored-By: Claude Opus 5 <noreply@anthropic.com>\nthird line\n' > "$vrepo/doc.md"
git -C "$vrepo" add -A
git -C "$vrepo" -c user.name=Probe -c user.email=probe@example.invalid commit -q -m base --no-verify
# Edit a NEIGHBOURING line so the trailer becomes unchanged diff CONTEXT.
printf 'first line CHANGED\nCo-Authored-By: Claude Opus 5 <noreply@anthropic.com>\nthird line\n' > "$vrepo/doc.md"
git -C "$vrepo" add -A
vrc=0
vout="$(cd "$vrepo" && GIT_EDITOR='sed -i.bak 1s/.*/fix:\ an\ edit\ beside\ a\ quoted\ trailer/' \
        git -c user.name=Probe -c user.email=probe@example.invalid commit -v 2>&1)" || vrc=$?
if [ "$vrc" -ne 0 ]; then
  fail "verbose: a legitimate \`git commit -v\` was refused because the trailer appears as diff CONTEXT — exit=$vrc: $(printf '%s' "$vout" | tr '\n' ' ')"
fi
case "$(git -C "$vrepo" log -1 --format='%s' 2>/dev/null)" in
  "fix: an edit beside a quoted trailer")
    pass "verbose: git's own -v diff is not judged, so an edit beside a quoted trailer commits" ;;
  *) fail "verbose: the -v commit exited 0 but is not the commit in the history: $(git -C "$vrepo" log -1 --format='%s' 2>/dev/null)" ;;
esac
# AND THE GATE IS STILL SHUT ON THAT PATH — a fix that simply stopped judging
# -v messages would pass the arm above.
printf 'tail\n' >> "$vrepo/doc.md"; git -C "$vrepo" add -A
vrc2=0
( cd "$vrepo" && GIT_EDITOR='sed -i.bak 1s|.*|feat:\ x\\\n\\\nCo-Authored-By:\ Claude\ Opus\ 5\ <noreply@anthropic.com>|' \
  git -c user.name=Probe -c user.email=probe@example.invalid commit -v >/dev/null 2>&1 ) || vrc2=$?
if [ "$vrc2" -eq 0 ]; then
  fail "verbose: a REAL trailer typed into a -v message was accepted — the -v path is no longer gated"
fi
pass "verbose: a real trailer typed into a -v message is still refused"

# ── ARM 5e: the known `#`-prefixed gap, asserted as the gap it is ───────────
# NOT a passing feature — a documented hole. `ATTRIBUTION_PATTERN` anchors on
# optional whitespace, so `# Co-Authored-By: …` matches neither this hook nor
# pre-push, and under non-interactive `whitespace` cleanup git KEEPS that line.
# The pattern is byte-identical to the one that was inline in pre-push, so
# widening it changes pre-push too and would refuse a message quoting the rule
# inside a comment. This arm pins today's behavior so that closing the gap later
# is a deliberate, visible change and not a surprise.
printf 'hash\n' > "$repo/hash.txt"; git -C "$repo" add -A
hashed="$(commit_with 'feat: a change

# Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>')"
case "$hashed" in
  exit=0*) pass "hash-gap (KNOWN, OPEN): a '#'-prefixed trailer is NOT caught — pinned, not endorsed" ;;
  *) fail "hash-gap: behavior changed. If the pattern was deliberately widened, update this arm and setup/hooks/README.md together: $hashed" ;;
esac

# ── ARM 5f: AN INDENTED QUOTATION COMMITS — the defect that blocked a release ─
# MEASURED 2026-09-17, running the THEN-CURRENT detection over the 55 commits
# this repository had not yet pushed. It refused three, and exactly one of them
# deserved it:
#
#     222f9d7  a REAL trailer, column 0.                    refused, rightly
#     c70dbe7  the same text quoted, indented two spaces.   refused, WRONGLY
#     9dc5ab8  the same text quoted in a shell example.     refused, WRONGLY
#
# c70dbe7 and 9dc5ab8 are the two commits that BUILT this guard: they paste the
# trailer into their own messages as evidence that the detection works, which is
# what this lane's rules demand, and the detection bit the evidence. The push
# sat blocked behind two false positives of the guard's own making.
#
# The fix is the anchor — `^` rather than `^[[:space:]]*`. This arm is the
# c70dbe7 shape, and it asserts against the HISTORY, not only the exit code:
# a hook that exits 0 without committing would satisfy an exit-code check.
printf 'indented\n' > "$repo/indented.txt"; git -C "$repo" add -A
indented="$(commit_with 'fix: prove the detection, quoting the trailer as evidence

Running it over the offending commit printed:

  Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>

which is the line the gate is supposed to catch.')"
case "$indented" in
  exit=0*) : ;;
  *) fail "quote-indented: an INDENTED quotation of the trailer was refused — this is the false positive that blocked the 2026-09-17 push: $indented" ;;
esac
case "$(git -C "$repo" log -1 --format='%B' 2>/dev/null)" in
  *"  Co-Authored-By: Claude Opus 5"*)
    pass "quote-indented: an indented quotation commits, and the indentation survived into the stored message" ;;
  *) fail "quote-indented: the hook exited 0 but the indented evidence is not in the stored message — git did not keep it, so this arm proves nothing: $(git -C "$repo" log -1 --format='%B' | tr '\n' ' ')" ;;
esac

# ── ARM 5g: a trailer embedded MID-LINE in a shell example commits ──────────
# The 9dc5ab8 shape. Not the same case as 5f: here the attribution text is not
# at the start of the line at all, so it is the anchor ITSELF — rather than the
# whitespace class — that has to let it through.
printf 'shellq\n' > "$repo/shellq.txt"; git -C "$repo" add -A
shellq="$(commit_with 'fix: document the rebuild recipe

The recipe in the refusal reads:

      git log --format=%B -1 <sha> | rg -v "Co-Authored-By: Claude" | git commit-tree')"
case "$shellq" in
  exit=0*) : ;;
  *) fail "quote-shell: a trailer embedded inside a shell example was refused: $shellq" ;;
esac
case "$(git -C "$repo" log -1 --format='%B' 2>/dev/null)" in
  *'rg -v "Co-Authored-By: Claude"'*)
    pass "quote-shell: a mid-line trailer inside a shell example commits, and the example landed intact" ;;
  *) fail "quote-shell: the hook exited 0 but that message is not the one in the history" ;;
esac

# ── ARM 5h: lowercase is still refused ─────────────────────────────────────
# The gate is applied with `grep -i` by BOTH callers. A narrowing of the anchor
# must not quietly become a narrowing of the case-folding too.
printf 'lower\n' > "$repo/lower.txt"; git -C "$repo" add -A
lower="$(commit_with 'feat: a change

co-authored-by: claude opus 5 <noreply@anthropic.com>')"
case "$lower" in
  exit=0*) fail "lowercase: an all-lowercase trailer was let through — the detection is no longer case-insensitive: $lower" ;;
  *"line 3, column 1: co-authored-by: claude opus 5"*)
    pass "lowercase: an all-lowercase trailer at column 0 is refused and named" ;;
  *) fail "lowercase: refused, but not for the trailer: $lower" ;;
esac

# ── ARM 5i: the trailer as the LAST line, with no trailing newline ──────────
# The shape a script produces with `printf '%s'` rather than `%s\n`, and the one
# a line-oriented gate is most likely to drop: grep still emits a final line
# without a terminator, but a hand-rolled `while read` would silently discard
# it. `commit_with` cannot express this — its heredoc always terminates the last
# line — so this arm writes the message file itself.
printf 'nonl\n' > "$repo/nonl.txt"; git -C "$repo" add -A
printf 'feat: a change\n\nCo-Authored-By: Claude Opus 5 <noreply@anthropic.com>' > "$TMP/nonl.msg"
[ "$(tail -c1 "$TMP/nonl.msg" | wc -l | tr -d ' ')" = "0" ] \
  || fail "no-final-newline: the fixture message ends in a newline, so this arm would test the ordinary case"
nonl_rc=0
nonl_out="$(cd "$repo" && git -c user.name=Probe -c user.email=probe@example.invalid \
            commit -F "$TMP/nonl.msg" 2>&1)" || nonl_rc=$?
case "exit=$nonl_rc $(printf '%s' "$nonl_out" | tr '\n' ' ')" in
  exit=0*) fail "no-final-newline: a trailer written as the last line with no trailing newline was let through" ;;
  *"line 3, column 1: Co-Authored-By: Claude Opus 5"*)
    pass "no-final-newline: a trailer as the unterminated last line is still seen, refused and named" ;;
  *) fail "no-final-newline: refused, but not for the trailer: exit=$nonl_rc $(printf '%s' "$nonl_out" | tr '\n' ' ')" ;;
esac

# ── ARM 5j: the anchor's SHAPE, because the refusal states the column as a
# constant ─────────────────────────────────────────────────────────────────
# `attribution_offending_lines` prints "column 1" without measuring it, which is
# true only while the pattern is anchored at `^` with nothing optional before
# the alternation. This arm is what makes that constant safe: widen the anchor
# and this fails, pointing at the comment that explains why.
. "$ROOT/setup/hooks/attribution-pattern.sh"
case "$ATTRIBUTION_PATTERN" in
  '^('*) pass "anchor: ATTRIBUTION_PATTERN begins at column 0 with nothing optional before it, so \"column 1\" is true by construction" ;;
  *) fail "anchor: ATTRIBUTION_PATTERN no longer starts with '^(' — the refusal's 'column 1' is now a lie. It reads: $ATTRIBUTION_PATTERN" ;;
esac
case "$ATTRIBUTION_PATTERN" in
  *'[[:space:]]'*) fail "anchor: ATTRIBUTION_PATTERN carries a whitespace class again — an indented QUOTATION will be refused as a trailer, which is the 2026-09-17 release blocker. If this was deliberate, update ARM 5f, 5g and attribution-pattern.sh together." ;;
  *) pass "anchor: no whitespace class survives in the pattern, so indentation still distinguishes a quotation from a trailer" ;;
esac

# ── ARM 5k: THE REAL COMMITS, not a reconstruction of them ─────────────────
# Arms 5f and 5g are the SHAPES. This one is the three actual commits the
# measurement was taken on, read out of this repository's own history, so the
# claim "the release is unblocked" is asserted against the artifacts that
# blocked it rather than against fixtures that resemble them.
#
# SKIPPED, LOUDLY, WHERE THE HISTORY IS NOT REACHABLE — a clone without these
# objects must not silently turn this arm into a pass.
_real_refused="222f9d7"   # a real trailer, column 0
_real_quoted="c70dbe7 9dc5ab8"  # the two commits that built this guard
if git -C "$LIVE_ROOT" cat-file -e "$_real_refused^{commit}" 2>/dev/null; then
  _hits="$(git -C "$LIVE_ROOT" log -1 --format='%B' "$_real_refused" | attribution_offending_lines)"
  case "$_hits" in
    *"column 1: Co-Authored-By: Claude"*) pass "real-commits: $_real_refused, a genuine column-0 trailer, is still refused and named" ;;
    *) fail "real-commits: $_real_refused carries a real trailer and the detection no longer sees it — this is a fail-OPEN: '$_hits'" ;;
  esac
  for _c in $_real_quoted; do
    _hits="$(git -C "$LIVE_ROOT" log -1 --format='%B' "$_c" | attribution_offending_lines)"
    [ -z "$_hits" ] \
      || fail "real-commits: $_c quotes the trailer as EVIDENCE and is refused again — the 2026-09-17 release blocker is back: $_hits"
  done
  pass "real-commits: c70dbe7 and 9dc5ab8, which quote the trailer as their own evidence, pass"
else
  pass "real-commits: SKIPPED — $_real_refused is not reachable from this checkout, so the real-history regression cannot be asserted here"
fi

# ── ARM 6: one detection, not two ──────────────────────────────────────────
# The brief's own instruction: reuse pre-push's detection, because if the two
# differ one of them is lying. Assert structurally, at the source: neither hook
# may carry its own inline copy of the regex.
for hook in commit-msg pre-push; do
  grep -q 'ATTRIBUTION_PATTERN' "$ROOT/setup/hooks/$hook" \
    || fail "shared: setup/hooks/$hook does not reference ATTRIBUTION_PATTERN"
  grep -q "co-authored-by:\.\*(claude" "$ROOT/setup/hooks/$hook" \
    && fail "shared: setup/hooks/$hook still carries its own inline copy of the attribution regex — the two gates can now drift"
done
pass "shared: commit-msg and pre-push both read attribution-pattern.sh and neither inlines the regex"

# And the shared file is resolved from the repo root, not from the hook's own
# dirname — the 2026-09-09 failure that killed a real push.
grep -q 'git rev-parse --show-toplevel' "$ROOT/setup/hooks/commit-msg" \
  || fail "shared: commit-msg never asks git for the repository root"
grep -qE '^\. "\$(_root|\(git rev-parse --show-toplevel\))/setup/hooks/attribution-pattern\.sh"' "$ROOT/setup/hooks/commit-msg" \
  || fail "shared: commit-msg does not source attribution-pattern.sh from a repo-root-resolved path"
grep -q 'dirname "${BASH_SOURCE\[0\]}".*attribution-pattern' "$ROOT/setup/hooks/commit-msg" \
  && fail "shared: commit-msg resolves the shared pattern relative to its own symlink — this is the 2026-09-09 push failure"
pass "shared: commit-msg resolves the shared pattern from the repo root, so a bare symlink works"

# ── ARM 7: the committed mode, asked of git rather than of the filesystem ───
# $ROOT can be a snapshot with no .git (see _common.sh); ask the live checkout.
mode="$(git -C "$LIVE_ROOT" ls-tree HEAD -- setup/hooks/commit-msg 2>/dev/null | awk '{print $1}')"
case "$mode" in
  100755) pass "mode: setup/hooks/commit-msg is committed as 100755, so a fresh clone gets a hook git will run" ;;
  "") fail "mode: setup/hooks/commit-msg is not in HEAD yet — commit it, and commit it executable" ;;
  *) fail "mode: setup/hooks/commit-msg is committed as $mode — git skips a hook without the execution bit IN SILENCE" ;;
esac

# ── MUTATION ARM ───────────────────────────────────────────────────────────
# M01 — turn the refusal into a warning: the hook still prints everything it
# printed before and then exits 0. This is the shape the execution-bit incident
# had (a gate that is present and does nothing), and no assertion above that
# reads only the hook's TEXT would catch it — which is why arm 2 checks the exit
# code separately from the message, and arm 4 checks the history.
#
# SURVIVAL CONTROL: the unmutated hook refused this exact message in arm 2 with
# exit=1, asserted there. The mutant is killed by the commit LANDING.
# The mutation is one token: the refusal branch's own `exit 1` becomes `exit 0`.
# Targeted by its POSITION (the line right after the heredoc-style block's
# `} >&2`), because the file has other `  exit 1` lines in its precondition
# guards and mutating one of those would test something else.
cp "$repo/setup/hooks/commit-msg" "$TMP/commit-msg.mutant"
mutate_anchor 152-M01 "$TMP/commit-msg.mutant" 'exit 0'
bash -n "$TMP/commit-msg.mutant" \
  || fail "M01 produced a syntactically broken hook — a mutant that cannot run proves nothing"
cp "$TMP/commit-msg.mutant" "$repo/setup/hooks/commit-msg"
chmod +x "$repo/setup/hooks/commit-msg"
printf 'third\n' > "$repo/third.txt"; git -C "$repo" add -A
mutant_out="$(commit_with 'feat: a change

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>')"
# The needle is the mutant's OWN signature: the refusal text AND a zero exit,
# a combination the healthy hook cannot produce — it prints that text only on
# the path that exits 1.
saw_mutant "M01 commit-msg warns instead of refusing" "$mutant_out" "exit=0 commit-msg: refusing"
# And the mutant really did let it land, which is what makes the arm about
# behavior rather than about a string.
case "$(git -C "$repo" log --format='%B' -1 2>/dev/null)" in
  *"Co-Authored-By"*) pass "M01: the mutant's commit reached the history, so the exit code is what holds this gate shut" ;;
  *) fail "M01: the mutant exited 0 but nothing was committed — the arm is not measuring what it claims" ;;
esac

# M01 mutated the HOOK. The two mutants below mutate the shared PATTERN, so the
# healthy hook has to be back in place first — otherwise they would be run
# against a gate that already exits 0 for everything and would pass vacuously.
cp "$ROOT/setup/hooks/commit-msg" "$repo/setup/hooks/commit-msg"
chmod +x "$repo/setup/hooks/commit-msg"
printf 'restored\n' > "$repo/restored.txt"; git -C "$repo" add -A
restored="$(commit_with 'chore: the healthy hook is back

  Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>')"
case "$restored" in
  exit=0*) pass "M-setup: the healthy hook is restored and still accepts an indented quotation" ;;
  *) fail "M-setup: restoring the healthy hook did not restore its behavior, so the pattern mutants below would prove nothing: $restored" ;;
esac

# M02 — PUT THE OLD ANCHOR BACK. `^` becomes `^[[:space:]]*` in the shared
# pattern, which is byte-for-byte the detection that blocked the 2026-09-17
# push. The healthy gate accepts the indented quotation (ARM 5f, asserted with
# the history, and again by M-setup just above); the mutant must refuse it.
#
# The kill needle is the gate DOING SOMETHING DIFFERENT — a refusal that names
# the quoted line — not a marker announcing that the patch applied.
cp "$ROOT/setup/hooks/attribution-pattern.sh" "$TMP/pattern.m02"
mutate_anchor 152-M02 "$TMP/pattern.m02" $'ATTRIBUTION_PATTERN=\'^[[:space:]]*(co-authored-by:.*(claude|anthropic|gpt|codex|opencode|copilot)|generated with \\[?claude|🤖 generated)\''
grep -q '\^\[\[:space:\]\]\*(' "$TMP/pattern.m02" \
  || fail "M02 was not applied — ATTRIBUTION_PATTERN's assignment no longer has the shape this arm edits; update the arm"
bash -n "$TMP/pattern.m02" || fail "M02 produced a syntactically broken pattern file — a mutant that cannot be sourced proves nothing"
cp "$TMP/pattern.m02" "$repo/setup/hooks/attribution-pattern.sh"
printf 'm02\n' > "$repo/m02.txt"; git -C "$repo" add -A
m02_out="$(commit_with 'fix: prove the detection, quoting the trailer as evidence

Running it over the offending commit printed:

  Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>

which is the line the gate is supposed to catch.')"
saw_mutant "M02 the old whitespace-tolerant anchor is restored" "$m02_out" \
  "line 5, column 1:   Co-Authored-By: Claude Opus 5"
case "$m02_out" in
  exit=0*) fail "M02: the mutant exited 0 — the old anchor did not reproduce the false positive, so this arm is not measuring the fix" ;;
  exit=1*) pass "M02: the mutant refused an indented quotation, which is exactly the release blocker the anchor change removes" ;;
  *) fail "M02: the mutant neither exited 0 nor refused — it may have died before reaching the mutated pattern: $m02_out" ;;
esac

# M03 — REMOVE THE ANCHOR ALTOGETHER, the other direction: `^(` becomes `(`, so
# the detection matches anywhere on a line. The healthy gate accepts the
# mid-line shell example (ARM 5g); the mutant must refuse it.
cp "$ROOT/setup/hooks/attribution-pattern.sh" "$TMP/pattern.m03"
mutate_anchor 152-M03 "$TMP/pattern.m03" $'ATTRIBUTION_PATTERN=\'(co-authored-by:.*(claude|anthropic|gpt|codex|opencode|copilot)|generated with \\[?claude|🤖 generated)\''
grep -qE "^ATTRIBUTION_PATTERN='\(co-authored-by" "$TMP/pattern.m03" \
  || fail "M03 was not applied — update this arm"
bash -n "$TMP/pattern.m03" || fail "M03 produced a syntactically broken pattern file"
cp "$TMP/pattern.m03" "$repo/setup/hooks/attribution-pattern.sh"
printf 'm03\n' > "$repo/m03.txt"; git -C "$repo" add -A
m03_out="$(commit_with 'fix: document the rebuild recipe

The recipe in the refusal reads:

      git log --format=%B -1 <sha> | rg -v "Co-Authored-By: Claude" | git commit-tree')"
saw_mutant "M03 the column-0 anchor is removed" "$m03_out" \
  'line 5, column 1:       git log --format=%B -1 <sha> | rg -v "Co-Authored-By: Claude"'
case "$m03_out" in
  exit=0*) fail "M03: the mutant exited 0 — an unanchored pattern did not refuse a mid-line mention, so the anchor is not what holds arm 5g" ;;
  exit=1*) pass "M03: the mutant refused a trailer quoted mid-line in a shell example, so the anchor is what lets that commit through" ;;
  *) fail "M03: the mutant neither exited 0 nor refused — it may have died before reaching the mutated pattern: $m03_out" ;;
esac
cp "$ROOT/setup/hooks/attribution-pattern.sh" "$repo/setup/hooks/attribution-pattern.sh"
