#!/usr/bin/env bash
# The two inputs to the verdict whose consumer is `git worktree remove`.
#
# `_wt_disposition` decides whether a worktree is disposable, and
# `hw reap --apply` acts on `safe` with `git worktree remove` — the command
# class that already destroyed 588K once. Two of its gates could not tell a
# measurement from a failure:
#
#   `bin/hw:5167` `_wt_occupants` — `herdr agent list 2>/dev/null | jq … || true`.
#     The comment three lines above reads "Read from herdr, NOT inferred: a
#     stray worktree is only safe to move when nothing is working in it." On any
#     herdr hiccup it WAS inferred, by exclusion, in the unsafe direction — and
#     this is the FIRST gate, checked before dirty and before merged precisely
#     so occupancy outranks everything. Same root cause as the 2026-08-21
#     incident its own comments describe: a cleanup that worked by exclusion
#     killed three executors. `cmd_revive` (`hw:4100`) had the identical shape.
#
#   `bin/hw:5393` — `[ -n "$(git -C "$wt" status --porcelain 2>/dev/null | head -1)" ]`.
#     stderr discarded, exit code thrown away by `$(...)`. A corrupted index, a
#     missing gitdir or a permissions error produces empty stdout EXACTLY as a
#     clean tree does, and the verdict walks on toward `safe`.
#
# WHAT THIS FILE PINS. Every gate is stubbed to FAIL — not to return empty, to
# FAIL — and the verdict must be `undetermined`, which is its own value with its
# own diagnosis and is never `safe`. That is the whole discipline: "nothing is
# working in it" and "we could not find out whether anything is working in it"
# are different facts, and only the first is a reason to delete a directory.
#
# The functions are extracted and driven directly, so nothing here touches a
# real worktree, a real herdr or a real `git worktree remove`.
#
# Run alone while working on this subject:
#     bash setup/tests/90-a-verdict-that-could-not-be-measured.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── EXTRACT THE TWO FUNCTIONS ──────────────────────────────────────────────
FNS="$TMP/wtfns.sh"
awk '/^WT_OCCUPANTS_OBSERVED=0$/,/^}$/' "$ROOT/bin/hw" > "$FNS"
awk '/^_wt_disposition\(\) \{$/,/^}$/' "$ROOT/bin/hw" >> "$FNS"
grep -q '_wt_occupants()' "$FNS" || fail "verdict-third-value: could not extract _wt_occupants from bin/hw"
grep -q '_wt_disposition()' "$FNS" || fail "verdict-third-value: could not extract _wt_disposition from bin/hw"
pass "both verdict functions extracted from bin/hw"

# The gates _wt_disposition calls that are not under test here. Stubbed to the
# ANSWER THAT WOULD REACH `safe`, so anything that stops short of safe stopped
# because of the gate we actually moved.
HARNESS="$TMP/harness.sh"
cat > "$HARNESS" <<'SH'
set -uo pipefail
. "$FNS"
_leased_rundir_for() { return 1; }
_wt_branch_merged()  { WT_MERGE_EVIDENCE="merged"; return 0; }
_wt_irreplaceable()  { printf ''; }
_wt_disposition /main /wt task/x main
printf 'VERDICT=%s\nWHY=%s\n' "$WT_VERDICT" "$WT_WHY"
SH

verdict() { FNS="$FNS" PATH="$TMP/gate:$PATH" bash "$HARNESS" 2>&1; }
mkdir -p "$TMP/gate"

# `git` and `herdr` are replaced for the duration: each stub reads a mode from
# the environment, so one harness drives every case.
cat > "$TMP/gate/herdr" <<'STUB'
#!/usr/bin/env bash
case "${GATE_HERDR:-ok}" in
  ok)       printf '{"result":{"type":"agent_list","agents":[]}}\n' ;;
  # ZERO AGENTS WITH THE KEY OMITTED. Nobody here has observed herdr's real
  # reply when nothing is running, so both shapes are driven: `agents` must be
  # readable as "none", not as "could not read". Getting this wrong makes the
  # third value unfalsifiable — every worktree undetermined forever.
  empty)    printf '{"result":{"type":"agent_list"}}\n' ;;
  occupied) printf '{"result":{"type":"agent_list","agents":[{"pane_id":"w1:p2","agent":"claude","agent_status":"working","cwd":"/wt"}]}}\n' ;;
  # The same occupant as herdr reports it on native Windows: C:\\...\\wt.
  occupiedwin) printf '{"result":{"type":"agent_list","agents":[{"pane_id":"w1:p2","agent":"claude","agent_status":"working","cwd":"%s"}]}}\n' "$GATE_WCWD" ;;
  # THE THREE WAYS IT FAILS, and none of them is "no occupants".
  down)     printf 'herdr: could not connect to the server socket\n' >&2; exit 7 ;;
  garbage)  printf 'not json at all\n' ;;
  # Exit 0 with an error object in stdout: measured on `herdr agent read <dead
  # pane>` on 2026-09-07, and the reason a clean exit is not itself an answer.
  errorobj) printf '{"result":{"error":"pane is gone"}}\n' ;;
  # Well-formed JSON that answers a DIFFERENT question.
  wrongtype) printf '{"result":{"type":"pane_list","panes":[]}}\n' ;;
esac
STUB
chmod +x "$TMP/gate/herdr"

cat > "$TMP/gate/git" <<'STUB'
#!/usr/bin/env bash
for a in "$@"; do [ "$a" = "--porcelain" ] && porcelain=1; done
if [ "${porcelain:-0}" = 1 ]; then
  case "${GATE_GIT:-clean}" in
    clean) exit 0 ;;
    dirty) printf ' M src/a.py\n M src/b.py\n'; exit 0 ;;
    # THE ONE THAT LOOKS IDENTICAL TO CLEAN: empty stdout, non-zero exit.
    broken) printf 'fatal: not a git repository: /wt/.git\n' >&2; exit 128 ;;
  esac
fi
exit 0
STUB
chmod +x "$TMP/gate/git"

# ── 1. THE BASELINE: everything readable, nothing in the way → safe ────────
out="$(GATE_HERDR=empty GATE_GIT=clean verdict)"
case "$out" in
  *VERDICT=safe*) pass "an agent_list with the agents key OMITTED reads as none, not as unreadable" ;;
  *) fail "verdict-third-value: herdr answering agent_list with no agents key gave: $out. Nobody here has seen herdr's reply with zero agents running; if it omits the key, keying on the ARRAY would make every worktree undetermined forever — a third value nothing can ever clear, which is its own kind of useless" ;;
esac

out="$(GATE_HERDR=ok GATE_GIT=clean verdict)"
case "$out" in
  *VERDICT=safe*) pass "a worktree whose gates all answered, and answered clean, is safe" ;;
  *) fail "verdict-third-value: the clean case is no longer safe, so the rest of this file proves nothing: $out" ;;
esac

# And the gates still say their own thing when they DO answer.
out="$(GATE_HERDR=occupied GATE_GIT=clean verdict)"
case "$out" in *VERDICT=held*) pass "an observed occupant is still held" ;; *) fail "verdict-third-value: an occupied worktree gave: $out" ;; esac
# On native Windows herdr spells that cwd C:\... (measured on windows-latest);
# compared as a string it read as nobody there, and nobody there is `safe`.
case "${OSTYPE:-}" in
  msys*|cygwin*)
    out="$(GATE_WCWD="$(cygpath -w /wt | sed 's/\\/\\\\/g')" GATE_HERDR=occupiedwin GATE_GIT=clean verdict)"
    case "$out" in *VERDICT=held*) pass "an occupant herdr reports as C:\\... is still held" ;; *) fail "verdict-third-value: an occupant herdr reports as C:\\... gave: $out" ;; esac ;;
  *) printf 'skip - the C:\\ cwd herdr reports exists only on native Windows\n' ;;
esac
out="$(GATE_HERDR=ok GATE_GIT=dirty verdict)"
case "$out" in *VERDICT=dirty*) pass "an observed uncommitted change is still dirty" ;; *) fail "verdict-third-value: a dirty worktree gave: $out" ;; esac

# ── 2. OCCUPANCY THAT COULD NOT BE READ ────────────────────────────────────
#
# Three failure shapes, because they are three different things going wrong and
# every one of them used to print nothing and be read as "nobody is there".
for mode in down garbage errorobj wrongtype; do
  out="$(GATE_HERDR=$mode GATE_GIT=clean verdict)"
  case "$out" in
    *VERDICT=safe*)
      fail "verdict-third-value: herdr '$mode' produced VERDICT=safe. An unread occupant list is not an empty one, and \`hw reap --apply\` runs \`git worktree remove\` on safe: $out" ;;
    *VERDICT=undetermined*) : ;;
    *) fail "verdict-third-value: herdr '$mode' gave neither safe nor undetermined: $out" ;;
  esac
  # AND THE DIAGNOSIS MUST NAME WHAT FAILED. A third value whose reason is
  # "something went wrong" sends the reader back to guess, which is the cost the
  # original 2>/dev/null was paying.
  case "$out" in
    *"could not read who is working in it"*) : ;;
    *) fail "verdict-third-value: herdr '$mode' did not say the occupant list was unreadable: $out" ;;
  esac
done
pass "a herdr that is down, that answers garbage, or that exits 0 with an error object all give undetermined, each naming what failed"

# ── 3. A `git status` THAT FAILED IS NOT A CLEAN TREE ──────────────────────
out="$(GATE_HERDR=ok GATE_GIT=broken verdict)"
case "$out" in
  *VERDICT=safe*)
    fail "verdict-third-value: a FAILED \`git status\` produced VERDICT=safe. Its empty stdout is byte-for-byte a clean tree's, and the consumer of safe is \`git worktree remove\`: $out" ;;
  *VERDICT=undetermined*) pass "a git status that exits non-zero gives undetermined, not clean" ;;
  *) fail "verdict-third-value: a broken git status gave: $out" ;;
esac
case "$out" in
  *"git status exited 128"*) pass "the diagnosis names the exit code the status actually returned" ;;
  *) fail "verdict-third-value: the broken-status diagnosis does not name the exit code: $out" ;;
esac

# ── 4. `undetermined` MUST NEVER REACH THE REMOVAL PATH ────────────────────
#
# The call site, because that is the property an extracted function cannot show:
# `hw reap --apply` and `hw done` both branch on `= safe`, so a new verdict
# keeps by construction — but only for as long as nobody widens the test.
#
# DRIVEN, NOT GREPPED. The first version of this check was a regex for
# `[ "$WT_VERDICT" = safe ]` and it was the defect this whole subject is about:
# a Judgment Day judge replaced the reap gate with
#     case "$WT_VERDICT" in safe|undetermined) ...
# on 2026-09-07, so `hw reap --apply` removed worktrees on `undetermined`, and
# the check still printed ok because the syntactic form it knew was simply gone.
# A `case` is the natural refactor once a verdict list has eight values.
#
# So the gate is EXECUTED instead: the real `cmd_reap` body is extracted, its
# removal driven against a stub `git`, and the question asked of behaviour —
# does a worktree get removed. Every non-safe verdict is checked, not just the
# new one, because the property is "safe is an allowlist", not "undetermined is
# special".
REAP="$TMP/reap.sh"
awk '/^cmd_reap\(\) \{$/,/^\}$/' "$ROOT/lib/hw/reap.sh" > "$REAP"
# Since 2026-10-01 the removal is `_reap_worktree`, which cmd_reap calls; a hint
# string in cmd_reap's own text used to satisfy this check by accident and went
# when the hand recipe did (setup-higiene-worktrees). So: cmd_reap must reach the
# removal, and the removal must be there to be driven.
grep -q '_reap_worktree "\$proj"' "$REAP" \
  && awk '/^_reap_worktree\(\) \{/,/^\}$/' "$ROOT/lib/hw/reap.sh" | grep -q 'worktree remove' \
  || fail "verdict-third-value: could not extract a cmd_reap that reaches a removal carrying \`worktree remove\` from lib/hw/reap.sh"

mkdir -p "$TMP/reapgate"
cat > "$TMP/reapgate/git" <<'STUB'
#!/usr/bin/env bash
# Records the fact that matters and performs nothing.
for a in "$@"; do
  case "$a" in
    remove) printf 'REMOVED
' >> "$REAP_WITNESS"; exit 0 ;;
  esac
done
exit 0
STUB
chmod +x "$TMP/reapgate/git"

# The smallest harness that reaches the gate: the real `if` from cmd_reap, with
# WT_VERDICT forced and everything around it stubbed to a no-op.
GATE="$TMP/gate.sh"
python3 - "$ROOT/lib/hw/reap.sh" "$GATE" <<'PY' || fail "verdict-third-value: could not isolate cmd_reap's removal gate"
import re, sys
src = open(sys.argv[1], encoding="utf-8").read()
start = src.find("      _wt_disposition \"$main\" \"$wt\" \"$branch\" \"$base\"")
if start < 0:
    raise SystemExit("anchor not found")
end = src.find("\n      else", start)
if end < 0:
    raise SystemExit("gate end not found")
gate = src[start:end]
gate = gate.replace('_wt_disposition "$main" "$wt" "$branch" "$base"',
                    'WT_VERDICT="$FORCE_VERDICT"; WT_WHY=forced')
# Since 2026-10-01 the removal itself is `_reap_worktree`, shared with `hw done`:
# its real body comes too, so the gate is still what acts. Its archive and
# database halves are stubbed to refuse — they are 640's subject, not this one.
m = re.search(r"^_reap_worktree\(\) \{.*?^\}$", src, re.S | re.M)
if not m:
    raise SystemExit("_reap_worktree not found")
helpers = (m.group(0) + "\n"
           "_reap_archive_dest(){ printf /nonexistent; }\n"
           "_reap_archive_entries(){ return 1; }\n"
           "_reap_rm_workdir(){ return 1; }\n"
           "_reap_db(){ :; }\n"
           "_wt_disposition(){ :; }\n")
open(sys.argv[2], "w", encoding="utf-8").write(
    "set -uo pipefail\n"
    "proj=demo; total_removed=0\n"
    + helpers +
    "total_safe=0; apply=1; main=/main; wt=/wt; branch=task/x; base=main\n"
    "ok(){ :; }; info(){ :; }; warn(){ :; }\n"
    "C_DIM=; C_0=\n"
    # The slice ends inside the `if [ "$WT_VERDICT" = safe ]`, at the `else`
    # that begins the KEEP arm. Close it so the gate is a runnable program; the
    # keep arm only prints, so cutting it changes nothing being measured.
    + gate + "\nfi\n")
PY

reap_removes() { # <verdict> -> "REMOVED" | ""
  : > "$TMP/witness"
  FORCE_VERDICT="$1" REAP_WITNESS="$TMP/witness" \
    PATH="$TMP/reapgate:$PATH" bash "$GATE" >/dev/null 2>&1 || true
  tr -d '\n' < "$TMP/witness"
}

[ "$(reap_removes safe)" = REMOVED ] \
  || fail "verdict-third-value: the extracted reap gate does not remove on \`safe\`, so the rest of this section proves nothing"
pass "the extracted reap gate really removes a worktree on safe"

for verdict in undetermined held leased detached dirty unmerged irreplaceable a-verdict-invented-next-year; do
  [ -z "$(reap_removes "$verdict")" ] \
    || fail "verdict-third-value: \`hw reap --apply\` ran \`git worktree remove\` on WT_VERDICT=$verdict. \`safe\` must be an ALLOWLIST — that is what keeps a verdict nobody has written yet on the KEEP side. This is the command class that destroyed 588K once."
done
pass "reap removes on safe and on nothing else — including a verdict that does not exist yet"

# ── 5. cmd_revive ASKS THE SAME QUESTION THE SAME WAY ──────────────────────
#
# It had the identical `2>/dev/null | jq … || true` shape, guarding the "one
# directory, one agent" rule. A failed read there does not merely keep a stale
# worktree — it CREATES the shared-tree collision the worktree model exists to
# prevent.
#
# DRIVEN, NOT READ. The first version of this section checked cmd_revive's
# SOURCE TEXT for three tokens, and a Judgment Day judge killed it on
# 2026-09-07 with a mutant that kept every token and neutralised the guard:
#     [ "$WT_OCCUPANTS_OBSERVED" = 1 ] || true \
#       || _revive_die "could not read ..."
# The guard was dead, `hw revive` would revive into a directory whose occupant
# list could not be read, and the section still printed ok. A test that reads
# the words instead of running the code is the defect this file is about.
REVIVE="$TMP/revive.sh"
python3 - "$ROOT/bin/hw" "$REVIVE" <<'PY' || fail "verdict-third-value: could not isolate cmd_revive's occupancy gate"
import sys
src = open(sys.argv[1], encoding="utf-8").read()
start = src.find("  # One directory, one agent.")
if start < 0:
    raise SystemExit("cmd_revive's one-directory-one-agent comment is gone")
end = src.find("\n  PROJ=", start)
if end < 0:
    raise SystemExit("could not find the end of the occupancy gate")
# WRAPPED IN A FUNCTION, so `local` keeps meaning `local`. Stubbing `local` out
# to run the slice at top level also neutered `local dir="$1"` inside the real
# `_wt_occupants` — the harness broke the code it was measuring.
open(sys.argv[2], "w", encoding="utf-8").write(
    "set -uo pipefail\n"
    '_revive_die(){ printf "REFUSED: %s\\n" "$*"; exit 9; }\n'
    "revive_gate() {\n"
    '  local cwd=/wt sid=""\n'
    + src[start:end]
    + '\n}\nrevive_gate\nprintf "PROCEEDED\\n"\n')
PY

# `_wt_occupants` is what the gate consults, so the real one is supplied and its
# dependency — herdr — is what gets stubbed to fail.
{ awk '/^WT_OCCUPANTS=""$/,/^}$/' "$ROOT/bin/hw"; cat "$REVIVE"; } > "$TMP/revive-run.sh"

# `|| true` for the reason _common.sh spells out: a REFUSED run exits 9, pipefail
# propagates it, and under `set -e` the ASSIGNMENT dies before the check on the
# next line ever runs — the suite would exit silently with no `not ok`.
revive_says() { # <herdr mode> -> REFUSED:... | PROCEEDED
  GATE_HERDR="$1" PATH="$TMP/gate:$PATH" bash "$TMP/revive-run.sh" 2>&1 | tr -d '\n' || true
}

out="$(revive_says ok)"
case "$out" in
  PROCEEDED) pass "revive proceeds when the occupant list is readable and empty" ;;
  *) fail "verdict-third-value: the extracted revive gate does not proceed on a clean read, so the rest proves nothing: $out" ;;
esac

out="$(revive_says occupied)"
case "$out" in
  REFUSED*already\ running*) pass "revive refuses when an agent is observed in the directory" ;;
  *) fail "verdict-third-value: revive did not refuse an OBSERVED occupant: $out" ;;
esac

for mode in down garbage errorobj wrongtype; do
  out="$(revive_says "$mode")"
  case "$out" in
    PROCEEDED)
      fail "verdict-third-value: with herdr '$mode', \`hw revive\` PROCEEDED into the target directory. An unreadable occupant list is not an empty one, and here a failed read does not leave a stale worktree — it CREATES the one-directory-two-agents collision the worktree model exists to prevent, and that leaves no trace in any diff." ;;
    REFUSED*could\ not\ read*) : ;;
    *) fail "verdict-third-value: revive with herdr '$mode' gave: $out" ;;
  esac
done
pass "revive refuses, naming what failed, when the occupant list cannot be read at all"

# ── 6. `_wt_irreplaceable` — THE OTHER INPUT TO THE SAME VERDICT ───────────
#
# `_wt_disposition` asks this function what would be LOST if the worktree went.
# An empty answer means "nothing git cannot rebuild", which is one gate from
# `safe`, which `hw reap --apply` hands to `git worktree remove`. So every way
# this function can come back empty WITHOUT having looked is the same 588K.
#
# WHY THIS SECTION EXISTS AT ALL. Both Judgment Day judges killed the first
# attempt at covering it on 2026-09-07 with the same move: revert the fix to the
# old one-liner AND APPEND THE ALLOWLIST MARKER. `bin/lint-shell` then reports
# nothing, and the only assertion that existed — "bin/ carries no unmeasured
# findings", in test 95 — passed while the destructive bug was fully back. That
# is a claim about the linter's opinion of a line's TEXT, defeated by one
# trailing comment. What the function DOES needs its own driver, and this is it.
IRR="$TMP/irr.sh"
awk '/^_wt_irreplaceable\(\) \{$/,/^\}$/' "$ROOT/bin/hw" > "$IRR"
grep -q '_wt_irreplaceable()' "$IRR" \
  || fail "verdict-third-value: could not extract _wt_irreplaceable from bin/hw"

mkdir -p "$TMP/irrgate" "$TMP/irrwt/keepme"
printf 'real bytes\n' > "$TMP/irrwt/keepme/data.bin"
# `git status --ignored` reports the tree; the rest of the pipeline is real.
cat > "$TMP/irrgate/git" <<'STUB'
#!/usr/bin/env bash
case "${GATE_GIT_IGNORED:-ok}" in
  ok)     printf '!! keepme/\n' ;;
  broken) printf 'fatal: not a git repository\n' >&2; exit 128 ;;
esac
STUB
cat > "$TMP/irrgate/fd" <<'STUB'
#!/usr/bin/env bash
case "${GATE_FD:-ok}" in
  ok)     printf '%s/data.bin\n' "${@: -1}" ;;
  broken) printf 'fd: permission denied\n' >&2; exit 3 ;;
esac
STUB
cat > "$TMP/irrgate/rg" <<'STUB'
#!/usr/bin/env bash
case "${GATE_RG:-ok}" in
  ok)     grep -E "$1" || true ;;
  broken) printf 'rg: not found\n' >&2; exit 127 ;;
esac
STUB
cat > "$TMP/irrgate/sd" <<'STUB'
#!/usr/bin/env bash
sed "s|$1|$2|"
STUB
chmod +x "$TMP/irrgate"/*

keeps() { # → what _wt_irreplaceable printed, on one line
  PATH="$TMP/irrgate:$PATH" bash -c "
    set -uo pipefail
    WT_REGENERABLE=''
    _wt_copy_of_main(){ return 1; }
    . '$IRR'
    _wt_irreplaceable '$TMP/irrwt'" 2>/dev/null | tr '\n' ' '
}

# The baseline, or the rest of the section proves nothing.
out="$(GATE_GIT_IGNORED=ok GATE_FD=ok GATE_RG=ok keeps)"
case "$out" in
  *keepme*) pass "a git-ignored directory holding real files is reported irreplaceable" ;;
  *) fail "verdict-third-value: the baseline does not report an ignored directory with files in it: [$out]" ;;
esac

# (a) `fd` FAILS — the inner gate. This is the instance the lint rule found.
out="$(GATE_GIT_IGNORED=ok GATE_FD=broken GATE_RG=ok keeps)"
case "$out" in
  "") fail "verdict-third-value: with \`fd\` failing, _wt_irreplaceable reported NOTHING. An unreadable directory is not an empty one, and an empty answer here walks the verdict to \`safe\` — which \`hw reap --apply\` hands to \`git worktree remove\`" ;;
  *keepme*) pass "an ignored directory that could not be READ is still reported, not dropped" ;;
  *) fail "verdict-third-value: fd failing gave: [$out]" ;;
esac

# (b) `git status --ignored` FAILS — the outer gate, one line above (a). Both
# judges pointed out that fixing the inner one left this open, and it fails the
# same way into the same command.
out="$(GATE_GIT_IGNORED=broken GATE_FD=ok GATE_RG=ok keeps)"
[ -n "$out" ] \
  || fail "verdict-third-value: with \`git status --ignored\` failing, the whole list came back empty and the worktree reads as holding nothing irreplaceable"
pass "an ignored-file list that could not be read is reported, not treated as no ignored files"

# (c) A MISSING TOOL in the pipeline. `rg` and `sd` are not POSIX; a machine
# without them would silently report every worktree disposable.
out="$(GATE_GIT_IGNORED=ok GATE_FD=ok GATE_RG=broken keeps)"
[ -n "$out" ] \
  || fail "verdict-third-value: with \`rg\` missing, _wt_irreplaceable reported nothing — a machine without rg would call every worktree safe to delete"
pass "a missing tool in the pipeline does not read as an empty ignored list"
