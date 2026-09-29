#!/usr/bin/env bash
# A new lane is added in projects.json, not in bin/
#
# WHAT THIS HOLDS. Before the second stage of the harness opening, adding a lane
# meant an arm in a dozen `case` statements across bin/hw, bin/brain and
# bin/project-spaces.sh. The claim now is that a lane which exists only in
# projects.json — here a fictitious `demo`, over a git repository this file
# creates — dispatches with `hw demo <task> --dry-run`, with every manifest
# field taken from the table, and that bin/ was not touched to get there.
#
# THE TREE IS A COPY, and that is the point rather than a convenience: bin/ is
# copied byte for byte, only the copy's projects.json gains `demo`, and the
# copy's own bin/hw is run from where it lies. So this also proves the brain
# root is derived from the script's location — nothing tells the copy where it
# is — and that the real table in the tree never carries `demo`.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
unset HW_BRAIN_ROOT HW_PROJECTS_JSON   # this subject is about the derivation itself

B="$TMP/brain"
mkdir -p "$B"
cp -R "$ROOT/bin" "$ROOT/layouts" "$B/"
# The brain guard launches with every product executor; without it the
# launch would refuse on the guard before it reached the build path.
mkdir -p "$B/setup"; cp -R "$ROOT/setup/guards" "$B/setup/"
home_repo demo-repo trunk
python3 - "$ROOT/projects.json" "$B/projects.json" "$HOME/demo-repo" <<'PY'
import json, sys
t = json.load(open(sys.argv[1]))
assert "demo" not in t["lanes"], "the real table must not carry demo"
t["lanes"]["demo"] = {
    "product_repo": True,
    "hw_aliases": ["dm"],
    "brain_aliases": ["dm"],
    "space": "demo-space",
    "engram": "demo-memory",
    "vendor": "claude",
    "model": "",
    "account": "default",
    "checkout": sys.argv[3],
    "base": "trunk",
    "base_ref_prefix": "",
    "branch": "feature/{task}",
    "worktree_root": "{checkout}/.wt",
    "worktree": "{checkout}/.wt/{task}",
}
json.dump(t, open(sys.argv[2], "w"), indent=2)
PY
mkdir -p "$B/demo/briefs"; : > "$B/demo/CLAUDE.md"
printf '# demo brief\n' > "$B/demo/briefs/probe.md"

diff -r "$ROOT/bin" "$B/bin" >/dev/null || fail "the copy's bin/ differs from the tree's, so nothing below is about configuration alone"
pass "the copy's bin/ is byte-identical to the tree's; only its projects.json differs"

export TMPDIR="$TMP"          # the route forecast lands here, not in the real TMPDIR
dry() { "$B/bin/hw" "$@" 2>&1 < /dev/null | sed 's/\x1b\[[0-9;]*m//g' || true; }

# ── 1. the demo lane dispatches, and every field is the table's ─────────────
out="$(dry demo probe --dry-run --no-report --sdd none)"
field() { printf '%s\n' "$out" | rg -F -- "$1" >/dev/null || fail "demo's manifest has no '$1': $(printf '%s' "$out" | tail -4 | tr '\n' ' ')"; }
field "dry run — nothing created"
field "dispatch demo:probe"
field "worktree    $HOME/demo-repo/.wt/probe  (new, own branch off trunk)"
field "base        trunk  (lane default)"
field "engram      demo-memory"
field "placement   own tab in space demo-space"
field "agent       claude  (lane default for demo)"
pass "hw demo probe --dry-run prints a full manifest: worktree, base, engram label, space and vendor all from projects.json"

# The brief was found under the COPY's root, which nothing named: the root was
# derived from where the copy's bin/hw lives.
field "brief       $B/demo/briefs/probe.md"
pass "the brain root is the copy's own, derived from the script's location with no override"

# The lane's alias is the table's too.
case "$(dry dm probe --dry-run --no-report --sdd none)" in
  *"dispatch demo:probe"*) pass "hw dm resolves to demo through the table's hw_aliases" ;;
  *) fail "the demo alias did not resolve: $(dry dm probe --dry-run --no-report --sdd none | tail -2)" ;;
esac

# A demo worktree is not built by a bin/ that has no build arm for it: the
# launch refuses before creating anything instead of falling through.
herdr_stub_note="the herdr stub answers every call, so only hw's own refusal can stop this"
real="$("$B/bin/hw" demo probe --no-report --sdd none --fresh 2>&1 < /dev/null | sed 's/\x1b\[[0-9;]*m//g' || true)"
case "$real" in
  *"demo has a worktree in projects.json but no build path in bin/hw yet"*)
    [ ! -e "$HOME/demo-repo/.wt/probe" ] || fail "the refusal left a worktree behind"
    pass "a non-dry demo launch refuses, naming the missing build path, and creates no worktree ($herdr_stub_note)" ;;
  *) fail "a non-dry demo launch did not refuse as expected: $(printf '%s' "$real" | tail -3 | tr '\n' ' ')" ;;
esac

# ── 2. brain knows the lane from the same table ─────────────────────────────
case "$("$B/bin/brain" dm --force 2>&1 || true)" in
  "--force means nothing without --reset") pass "brain dm is accepted as a lane (it reaches flag validation, not 'unknown brainer')" ;;
  *) fail "brain did not accept the demo alias: $("$B/bin/brain" dm --force 2>&1 || true)" ;;
esac

# ── mutants ───────────────────────────────────────────────────────────────
# M01 — the same copy, with demo taken out of the table again: the lane is
# unknown, which proves section 1 was the table's doing.
python3 - "$ROOT/projects.json" "$B/projects.json" <<'PY'
import shutil, sys
shutil.copy(sys.argv[1], sys.argv[2])
PY
saw_mutant "M01 demo removed from the copy's projects.json" \
  "$(dry demo probe --dry-run --no-report --sdd none)" "unknown project: demo"

# M02 — a table that does not parse loads nothing, and hw says so instead of
# dispatching with no lanes.
printf '{ "lanes": ' > "$B/projects.json"
saw_mutant "M02 an unparseable projects.json" \
  "$(dry setup probe --dry-run --no-report --sdd none)" "the lane table did not load"

# M03 — a lane name that would be spliced into a variable name is refused at
# load, not eval'd.
printf '{"work":"~/w","lanes":{"a;b":{"space":"x"}}}' > "$B/projects.json"
saw_mutant "M03 a lane name that is not a lowercase word" \
  "$(dry setup probe --dry-run --no-report --sdd none)" "the lane table did not load"

printf 'coverage - 175: 6 live claims, 3 mutants (3 must be named)\n'
