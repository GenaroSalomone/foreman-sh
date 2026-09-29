#!/usr/bin/env bash
# hw's own directories never reach the repo, and nothing else is hidden.
#
# `_ensure_artifacts_dir` writes `.artifacts/` and `.hw/` into
# `.git/info/exclude` — the one writable non-versioned path inside a repo. Until
# 2026-09-22 it also wrote `/odd/`, for the task documents an always-on ODD
# instruction made agents create at the worktree root. That instruction left
# this machine with Gentle AI, so hw stopped hiding a path nobody writes: an
# exclusion without a writer only blinds git to a product directory of that name.
#
# The EFFECT is driven, not the text: a real worktree, real files, `git status`.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── extract the production function, so the test drives the real bytes ────────
FN="$TMP/fn.sh"
awk '/^_ensure_artifacts_dir\(\) \{$/,/^\}$/' "$ROOT/bin/hw" > "$FN"
[ -s "$FN" ] || fail "fixture: could not extract _ensure_artifacts_dir from bin/hw"
# A bare `^}$` proves only that SOME closing brace was captured, not the matching one: if the
# body ever grows a nested `{ ...; }` group or a heredoc with a bare `}` line, the awk range ends
# early and the test would validate a truncated copy of production. Require the function's own
# last statement, and require the extract to parse.
grep -q '^}$' "$FN" || fail "fixture: extraction did not capture the closing brace"
grep -q '^  return 0$' "$FN" || fail "fixture: extraction truncated before the function's last statement"
bash -n "$FN" || fail "fixture: the extracted function does not parse"

# `info` is hw's own printer; the function under test is the subject, not it.
info() { :; }

# shellcheck disable=SC1090
. "$FN"

# ── fixture: a real repository with a real linked worktree ───────────────────
REPO="$TMP/repo"
mkdir -p "$REPO"
git -C "$REPO" init -q
git -C "$REPO" config user.email t@t
git -C "$REPO" config user.name t
printf 'x\n' > "$REPO/tracked.txt"
git -C "$REPO" add tracked.txt
git -C "$REPO" commit -qm init
WT="$TMP/wt"
git -C "$REPO" worktree add -q -b probe "$WT"

_ensure_artifacts_dir "$WT"

EX="$(git -C "$WT" rev-parse --git-path info/exclude)"
case "$EX" in /*) ;; *) EX="$WT/$EX" ;; esac

# ── C01 hw's two directories are excluded, and nothing named odd is ─────────
for line in '.artifacts/' '.hw/'; do
  grep -qxF "$line" "$EX" || fail "C01 $line is missing from the exclude file ($EX)"
done
grep -qE '^/?odd/$' "$EX" && fail "C01 hw still writes an odd/ exclusion after ODD was retired"
pass "C01 a fresh worktree excludes .artifacts/ and .hw/, and no longer odd/"

# ── C02 the effect: hw's files are invisible, an odd/ directory is not ───────
printf 'a\n' > "$WT/.artifacts/report.txt"; mkdir -p "$WT/.hw/run"; printf 'b\n' > "$WT/.hw/run/env"
mkdir -p "$WT/odd/tasks"; printf '# x\n' > "$WT/odd/tasks/f.md"
st="$(git -C "$WT" status --porcelain)"
case "$st" in *.artifacts*|*.hw*) fail "C02 hw's own files showed up in git status: $st" ;; esac
case "$st" in *odd/*) ;; *) fail "C02 an odd/ directory is still hidden from git: '$st'" ;; esac
pass "C02 .artifacts/ and .hw/ stay out of git status; an odd/ directory is visible like any other"

# ── C03 a real change to a tracked file is still reported ────────────────────
printf 'y\n' >> "$WT/tracked.txt"
git -C "$WT" status --porcelain | grep -q 'tracked.txt' \
  || fail "C03 the exclusion hid a tracked product change"
pass "C03 a real change to a tracked file is still reported"

# ── C04 one write covers a SIBLING worktree: info/exclude is in the common dir
SIB="$TMP/sibling"
git -C "$REPO" worktree add -q -b sibling "$SIB"
mkdir -p "$SIB/.artifacts"; printf 'c\n' > "$SIB/.artifacts/x.txt"
case "$(git -C "$SIB" status --porcelain)" in
  *.artifacts*) fail "C04 a sibling worktree reported .artifacts/" ;;
esac
pass "C04 the same write covers a sibling worktree"
