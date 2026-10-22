#!/usr/bin/env bash
# hw status audits the always-loaded memory indexes: one line, or nothing
#
# Claude Code loads the whole MEMORY.md of a git root's auto-memory into every
# session under it, per account. Measured 2026-10-02: the brain index carried
# 99 lines / 16961 B, five retired entries (OBSOLETA, REVERTIDA) still listed,
# and decisions.md cited a [[slug]] that lived in no store the reader could
# see. Nothing kept that index honest. bin/memory-audit is the read-only check
# `hw status` now prints; a clean store prints NOTHING.
#
# Extended 2026-10-18 (memoria-mecanismo): the audit also checks each memory file's
# `type:`/`expires:`, index <-> files, the file cap, a description that repeats a
# rule's heading or line, and engram (pending conflicts, observations without a
# topic_key) read through its CLI. Every check has its single-defect fixture
# here, and a mutant that must go silent on it (660-M06..M14). engram is a stub:
# the real store is never asked.
#
# Run alone while working on this subject:
#     bash setup/tests/660-hw-status-audita-las-memorias.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

AUDIT="$ROOT/bin/memory-audit"
[ -f "$AUDIT" ] || fail "the subject of this test does not exist: $AUDIT"

# A stub `engram`: `conflicts stats` and `export FILE` answer from STUB_* env, so
# no test reads the real store.
STUB="$TMP/engram-stub"
# the audit asks engram only where a store exists (the CLI would create one): give the stub's runs a placeholder
export ENGRAM_DATA_DIR="$TMP/engram-data"; mkdir -p "$ENGRAM_DATA_DIR"; : > "$ENGRAM_DATA_DIR/engram.db"
cat > "$STUB" <<'SH'
#!/usr/bin/env bash
case "$1" in
  conflicts)
    printf 'Conflicts Stats (project: stub)\n\n  By judgment_status:\n    pending:     %s\n    judged:      5\n\n  By relation type:\n    pending:             9\n' "${STUB_PENDING:-0}" ;;
  export)
    python3 - "$2" "${STUB_TOTAL:-100}" "${STUB_NOKEY:-0}" <<'PY'
import json, sys
path, total, nokey = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
obs = [{"id": i, "topic_key": "" if i < nokey else "k/%d" % i, "deleted_at": None} for i in range(total)]
obs.append({"id": -1, "topic_key": "", "deleted_at": "2026-01-01"})  # a deleted one never counts
json.dump({"observations": obs}, open(path, "w"))
PY
    ;;
  *) exit 2 ;;
esac
SH
chmod +x "$STUB"

# A fixture brain with one lane, and one account config dir holding its store.
fixture() {  # $1 = dir
  local r="$1/brain" c="$1/cfg"
  mkdir -p "$r/setup" "$c"
  printf '{"lanes": {"setup": {}}}\n' > "$r/projects.json"
  printf '# SETUP — decisions\n\n## 2026-10-02 — a ruling\n**Ruling:** see [[kept-memory]] and `[[:space:]]`.\n' > "$r/setup/decisions.md"
  local key; key="$(python3 -c 'import os,re,sys; print(re.sub(r"[^A-Za-z0-9]","-",os.path.realpath(sys.argv[1])))' "$r")"
  local m="$c/projects/$key/memory"; mkdir -p "$m"
  printf -- '---\nname: kept-memory\nmetadata:\n  type: feedback\n---\n\nHow the brainer works.\n' > "$m/kept-memory.md"
  printf -- '---\nname: loan\nmetadata:\n  type: project\n  expires: 2999-12-31\n---\n\nA live loan.\n' > "$m/loan.md"
  printf -- '- [Kept](kept-memory.md) — how the brainer works\n- [Loan](loan.md) — a live loan\n' > "$m/MEMORY.md"
  printf '%s' "$m"
}
run_audit() { python3 "$AUDIT" --root "$1/brain" --config-dir "$1/cfg" --engram "$STUB" "${@:2}" 2>&1; }
# one memory file, indexed, with the given frontmatter lines (and optional description)
add_memory() {  # $1 = store, $2 = slug, $3 = frontmatter lines, $4 = indexed? (1|0)
  printf -- '---\nname: %s\n%s\n---\n\nBody.\n' "$2" "$3" > "$1/$2.md"
  [ "${4:-1}" = 1 ] && printf -- '- [%s](%s.md) — x\n' "$2" "$2" >> "$1/MEMORY.md"
  return 0
}

# ── 1. a clean store prints nothing and exits 0 ─────────────────────────────
F="$TMP/clean"; fixture "$F" >/dev/null
rc=0; out="$(run_audit "$F")" || rc=$?
[ "$rc" = 0 ] && [ -z "$out" ] || fail "clean: expected silence and exit 0, got exit $rc: $out"
pass "clean: a store with nothing to say prints nothing"

# ── 2. each finding is named, on one line, exit 3 ───────────────────────────
F="$TMP/dirty"; M="$(fixture "$F")"
printf -- '- [Old](old-rule.md) — [OBSOLETA: retired] the old way\n' >> "$M/MEMORY.md"
printf -- '---\nname: replaced\nmetadata:\n  type: feedback\n---\n\n**Reemplazada 2026-10-02** por [[kept-memory]].\n' > "$M/replaced.md"
printf -- '- [Replaced](replaced.md) — still sounds current\n' >> "$M/MEMORY.md"
printf -- '- [Ghost](ghost.md) — a line with no file\n' >> "$M/MEMORY.md"
printf '**Ruling:** cites [[nowhere-at-all]].\n' >> "$F/brain/setup/decisions.md"
rc=0; out="$(run_audit "$F")" || rc=$?
[ "$rc" = 3 ] || fail "dirty: expected exit 3, got $rc: $out"
[ "$(printf '%s\n' "$out" | grep -c .)" = 1 ] || fail "dirty: the audit printed more than one line: $out"
for needle in "old-rule (marked)" "replaced (file says superseded)" "ghost (file missing)" \
              "setup/decisions.md:5 [[nowhere-at-all]]"; do
  case "$out" in *"$needle"*) ;; *) fail "dirty: the line does not name '$needle': $out" ;; esac
done
case "$out" in
  *kept-memory*) fail "dirty: a [[slug]] that resolves to a memory file was reported: $out" ;;
  *":space:"*) fail "dirty: a regex character class was read as a [[slug]]: $out" ;;
esac
pass "dirty: one line names the marked entry, the superseded file, the missing file and the dangling slug"

out="$(run_audit "$TMP/clean" --budget-bytes 10)" || true
case "$out" in *"1 index(es) over 10 B"*) pass "budget: an index over the byte budget is named" ;;
  *) fail "budget: an index over budget was not named: $out" ;; esac

# ── 3. hw status runs it ────────────────────────────────────────────────────
awk '/^cmd_status\(\) \{/,/^\}/' "$ROOT/lib/hw/status.sh" | grep -q '"$HW_BIN_DIR/memory-audit"' \
  || fail "wiring: cmd_status does not run bin/memory-audit"
pass "wiring: hw status runs the audit after the engram label audit"

# ── mutants: each removes one check and is killed by what it printed ────────
mut() {  # $1 mutated copy, $2 fixture, rest = audit flags
  python3 "$1" --root "$2/brain" --config-dir "$2/cfg" --engram "$STUB" "${@:3}" 2>&1; printf ' [rc=%s]' "$?"
}
for a in M01 M02 M03 M04; do cp "$AUDIT" "$TMP/ma-$a"; done
mutate_anchor 660-M01 "$TMP/ma-M01" 'if False:'
mutate_anchor 660-M02 "$TMP/ma-M02" 'if False:'
mutate_anchor 660-M03 "$TMP/ma-M03" 'elif False:'
mutate_anchor 660-M04 "$TMP/ma-M04" 'if False:'
saw_mutant "660-M01 the budget is never compared" \
  "$(mut "$TMP/ma-M01" "$TMP/clean" --budget-bytes 10)" "[rc=0]"
o="$(mut "$TMP/ma-M02" "$F")"
case "$o" in *"old-rule (marked)"*) fail "660-M02 VACUOUS: the mutant still reported the marked entry: $o" ;;
  *"ghost (file missing)"*) pass "mutant killed: 660-M02 retirement markers ignored (saw: the other findings, no marked entry)" ;;
  *) fail "660-M02 VACUOUS: the mutant reported nothing at all: $o" ;; esac
o="$(mut "$TMP/ma-M03" "$F")"
case "$o" in *"file says superseded"*) fail "660-M03 VACUOUS: the mutant still read the banner: $o" ;;
  *"ghost (file missing)"*) pass "mutant killed: 660-M03 superseded banner ignored (saw: the other findings, no banner)" ;;
  *) fail "660-M03 VACUOUS: the mutant reported nothing at all: $o" ;; esac
o="$(mut "$TMP/ma-M04" "$F")"
case "$o" in *"nowhere-at-all"*) fail "660-M04 VACUOUS: the mutant still reported the slug: $o" ;;
  *"ghost (file missing)"*) pass "mutant killed: 660-M04 dangling slugs ignored (saw: the other findings, no slug)" ;;
  *) fail "660-M04 VACUOUS: the mutant reported nothing at all: $o" ;; esac
cp "$ROOT/lib/hw/status.sh" "$TMP/m05-hw"; mutate_anchor 660-M05 "$TMP/m05-hw" ':'
case "$(awk '/^cmd_status\(\) \{/,/^\}/' "$TMP/m05-hw")" in
  *memory-audit\"*) fail "660-M05 VACUOUS: the mutant still runs the audit" ;;
  *"--audit"*) pass "mutant killed: 660-M05 hw status without the audit (saw: cmd_status with no memory-audit call)" ;;
  *) fail "660-M05 VACUOUS: the mutant's cmd_status could not be read" ;;
esac

# ── 4. the checks added 2026-10-18, each on a store with ONE defect ─────────
# solo <dir> <slug> <frontmatter> [indexed]: a clean fixture plus one memory file.
solo() { local d="$TMP/solo-$1"; local m; m="$(fixture "$d")"; [ $# -ge 3 ] && add_memory "$m" "$2" "$3" "${4:-1}"; printf '%s' "$d"; }
expect_solo() {  # $1 label, $2 dir, $3 needle, rest = env assignments for the run
  local lbl="$1" d="$2" n="$3"; shift 3
  local rc=0 o; o="$(env "$@" python3 "$AUDIT" --root "$d/brain" --config-dir "$d/cfg" --engram "$STUB" 2>&1)" || rc=$?
  [ "$rc" = 3 ] || fail "$lbl: expected exit 3, got $rc: $o"
  [ "$(printf '%s\n' "$o" | grep -c .)" = 1 ] || fail "$lbl: more than one line: $o"
  case "$o" in *"$n"*) pass "$lbl: named ($n)" ;; *) fail "$lbl: the line does not name '$n': $o" ;; esac
}
mutsolo() {  # $1 anchor id (its mutant copy is already at $TMP/mx-<id>), $2 dir, rest = env assignments; prints the mutant's output and rc
  local id="$1" d="$2"; shift 2
  env "$@" python3 "$TMP/mx-$id" --root "$d/brain" --config-dir "$d/cfg" --engram "${MUT_ENGRAM:-$STUB}" 2>&1; printf ' [rc=%s]' "$?"
}

D="$(solo untyped untyped 'description: x')"
expect_solo "type: a memory with no type:" "$D" "untyped (no type)"
cp "$AUDIT" "$TMP/mx-660-M06"; mutate_anchor 660-M06 "$TMP/mx-660-M06" 'if False:'
saw_mutant "660-M06 a memory with no type: goes unnoticed" "$(mutsolo 660-M06 "$D")" "[rc=0]"

D="$(solo noexp proj 'metadata:
  type: project')"
expect_solo "expires: a project memory with no expires:" "$D" "proj (project without expires)"
cp "$AUDIT" "$TMP/mx-660-M07"; mutate_anchor 660-M07 "$TMP/mx-660-M07" 'if False:'
saw_mutant "660-M07 a project memory with no expires: goes unnoticed" "$(mutsolo 660-M07 "$D")" "[rc=0]"

D="$(solo expired gone 'metadata:
  type: project
  expires: 2020-01-01')"
expect_solo "expires: a project memory past its date" "$D" "gone (expired 2020-01-01)"
cp "$AUDIT" "$TMP/mx-660-M08"; mutate_anchor 660-M08 "$TMP/mx-660-M08" 'if False:'
saw_mutant "660-M08 an expired project memory goes unnoticed" "$(mutsolo 660-M08 "$D")" "[rc=0]"

D="$(solo orphan stray 'metadata:
  type: feedback' 0)"
expect_solo "index: a file the index does not list" "$D" "stray"
cp "$AUDIT" "$TMP/mx-660-M13"; mutate_anchor 660-M13 "$TMP/mx-660-M13" 'if False:'
saw_mutant "660-M13 a file missing from the index goes unnoticed" "$(mutsolo 660-M13 "$D")" "[rc=0]"

D="$(solo cap)"
expect_solo "cap: a store over the file cap" "$D" "over 1 memory files" HW_MEMORY_FILE_CAP=1
cp "$AUDIT" "$TMP/mx-660-M12"; mutate_anchor 660-M12 "$TMP/mx-660-M12" 'if False:'
saw_mutant "660-M12 a store over the file cap goes unnoticed" "$(mutsolo 660-M12 "$D" HW_MEMORY_FILE_CAP=1)" "[rc=0]"

D="$(solo twin twin 'description: "executors always run under the personal account, never the default"
metadata:
  type: feedback')"
printf '\n## 2026-10-03 — executors always run under the personal account and never the default\n' >> "$D/brain/setup/decisions.md"
expect_solo "overlap: a description that repeats a ruling heading" "$D" "twin ~ setup/decisions.md:"
cp "$AUDIT" "$TMP/mx-660-M09"; mutate_anchor 660-M09 "$TMP/mx-660-M09" 'if False:'
saw_mutant "660-M09 a memory that repeats a ruling goes unnoticed" "$(mutsolo 660-M09 "$D")" "[rc=0]"
# the control: a description that shares only a word or two with it is silent
D2="$(solo twin2 other 'description: "the personal account is unrelated to this note about tabs"
metadata:
  type: feedback')"
printf '\n## 2026-10-03 — executors always run under the personal account and never the default\n' >> "$D2/brain/setup/decisions.md"
rc=0; out="$(run_audit "$D2")" || rc=$?
[ "$rc" = 0 ] && [ -z "$out" ] || fail "overlap control: two shared words were reported as a duplicate: $out"
pass "overlap control: a description sharing a couple of words with a heading is silent"

D="$(solo pending)"
expect_solo "engram: pending conflicts over the threshold" "$D" "500 pending conflicts" STUB_PENDING=500
case "$(env STUB_PENDING=500 python3 "$AUDIT" --root "$D/brain" --config-dir "$D/cfg" --engram "$STUB")" in
  *memory-retro*) pass "engram: the line names the memory-retro skill" ;; *) fail "engram: the line does not name the skill" ;; esac
cp "$AUDIT" "$TMP/mx-660-M10"; mutate_anchor 660-M10 "$TMP/mx-660-M10" 'if False:'
saw_mutant "660-M10 pending conflicts are never compared" "$(mutsolo 660-M10 "$D" STUB_PENDING=500)" "[rc=0]"

expect_solo "engram: observations without topic_key over the threshold" "$D" "60% of 100 observations without topic_key" STUB_NOKEY=60
cp "$AUDIT" "$TMP/mx-660-M11"; mutate_anchor 660-M11 "$TMP/mx-660-M11" 'if False:'
saw_mutant "660-M11 the topic_key fraction is never compared" "$(mutsolo 660-M11 "$D" STUB_NOKEY=60)" "[rc=0]"

rc=0; out="$(env STUB_PENDING=100 STUB_NOKEY=30 python3 "$AUDIT" --root "$D/brain" --config-dir "$D/cfg" --engram "$STUB" 2>&1)" || rc=$?
[ "$rc" = 0 ] && [ -z "$out" ] || fail "engram boundary: exactly at the thresholds should be silent: $out"
pass "engram boundary: exactly 100 pending and 30% without topic_key is silent"

D="$(solo down)"
# an engram that is installed but fails: one line, no crash
printf '#!/bin/sh\nexit 1\n' > "$TMP/engram-broken"; chmod +x "$TMP/engram-broken"
rc=0; out="$(python3 "$AUDIT" --root "$D/brain" --config-dir "$D/cfg" --engram "$TMP/engram-broken" 2>&1)" || rc=$?
case "$out" in *Traceback*) fail "engram down: the audit crashed: $out" ;; esac
case "$out" in *"engram did not answer"*) pass "engram down: one line says engram did not answer, no crash" ;;
  *) fail "engram down: the line does not say so: $out" ;; esac
cp "$AUDIT" "$TMP/mx-660-M14"; mutate_anchor 660-M14 "$TMP/mx-660-M14" 'return None'
saw_mutant "660-M14 an engram that does not answer goes unsaid" \
  "$(MUT_ENGRAM="$TMP/engram-broken" mutsolo 660-M14 "$D")" "[rc=0]"

# no engram on this machine at all (a foreman-sh install without it): silent, not a drift
rc=0; out="$(python3 "$AUDIT" --root "$D/brain" --config-dir "$D/cfg" --engram "$TMP/no-such-engram" 2>&1)" || rc=$?
[ "$rc" = 0 ] && [ -z "$out" ] || fail "engram absent: a machine with no engram binary should be silent: $out"
pass "engram absent: no engram binary is silent, so an install without engram sees nothing"
cp "$AUDIT" "$TMP/mx-660-M15"; mutate_anchor 660-M15 "$TMP/mx-660-M15" 'return "engram did not answer"'
saw_mutant "660-M15 a machine with no engram starts reporting it" \
  "$(MUT_ENGRAM="$TMP/no-such-engram" mutsolo 660-M15 "$D")" "engram did not answer"

# a machine whose engram never created a store: the CLI would create one, so the audit does not ask
mkdir -p "$TMP/no-store"
rc=0; out="$(env ENGRAM_DATA_DIR="$TMP/no-store" STUB_PENDING=500 python3 "$AUDIT" --root "$D/brain" --config-dir "$D/cfg" --engram "$STUB" 2>&1)" || rc=$?
[ "$rc" = 0 ] && [ -z "$out" ] || fail "engram no store: an audit on a machine with no engram store should be silent: $out"
[ ! -e "$TMP/no-store/engram.db" ] || fail "engram no store: the audit left an engram.db behind"
pass "engram no store: silent, and no engram.db is created by the audit"
cp "$AUDIT" "$TMP/mx-660-M16"; mutate_anchor 660-M16 "$TMP/mx-660-M16" 'if False:'
saw_mutant "660-M16 the audit asks engram where it has no store" \
  "$(mutsolo 660-M16 "$D" ENGRAM_DATA_DIR="$TMP/no-store" STUB_PENDING=500)" "500 pending conflicts"
