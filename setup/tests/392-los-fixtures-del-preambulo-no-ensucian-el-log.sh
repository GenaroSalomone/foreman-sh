#!/usr/bin/env bash
# THE PREAMBLE FIXTURES DO NOT LEAVE "command not found" IN THE LOG.
#
# Measured 2026-09-30 in the first public CI run (36747604152): the log carried
# "lane_get: command not found" from the preamble fixtures of 166, 185 and 47
# and "grep: .../CLAUDE.shared.md: No such file or directory" from 183. The
# same lines appear on brain (27 and 205 too): nothing the export removed was
# needed. 27, 47, 166, 185 and 205 render the preamble from a fragment of
# bin/hw that calls lane_get (205 also info and _receipt_model) without
# stubbing it, and `|| brief_note=""` swallowed the failure, so every subject
# passed while shouting. 183 grepped a file the export does not carry, and the
# grep's error read as a pass.
#
# Each subject runs in a tree of its own that carries only what it needs —
# setup/tests/_common.sh, the subject, bin/ and setup/fixtures/, and no
# CLAUDE.shared.md, as in the export — and its stderr must carry neither
# message.
#
#     bash setup/tests/392-los-fixtures-del-preambulo-no-ensucian-el-log.sh
#
# SUBJECT_TESTS runs the subjects from another setup/tests, for the old/new
# evidence.
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

TESTS_DIR="${SUBJECT_TESTS:-$ROOT/setup/tests}"
ran=0
for t in 27-brief-epistemic-safeguard.sh 47-unattended-gate-stall.sh 166-el-cierre-es-judgment-day.sh 183-un-fix-prueba-el-rojo.sh \
         185-el-analyze-corre-en-contexto-limpio.sh 205-el-diseno-se-juzga-antes-del-codigo.sh; do
  [ -f "$TESTS_DIR/$t" ] || continue
  ran=$((ran + 1))
  tree="$(mktemp -d "$TMP/tree-XXXXXX")"; mkdir -p "$tree/setup/tests" "$tree/bin"
  cp "$TESTS_DIR/_common.sh" "$TESTS_DIR/$t" "$tree/setup/tests/"
  cp -R "$ROOT/bin/." "$tree/bin/"
  [ ! -d "$ROOT/setup/fixtures" ] || cp -R "$ROOT/setup/fixtures" "$tree/setup/"
  rc=0; bash "$tree/setup/tests/$t" > "$tree/out" 2> "$tree/err" || rc=$?
  [ "$rc" -eq 0 ] || fail "$t: the subject itself failed (exit $rc): $(grep -m3 'not ok' "$tree/out" "$tree/err")"
  if grep -E 'command not found|No such file or directory' "$tree/err" > "$tree/noise"; then
    fail "$t: its stderr carries $(wc -l < "$tree/noise" | tr -d ' ') noise line(s), first: $(head -n 1 "$tree/noise")"
  fi
  pass "$t: passes with a clean stderr, in a tree with no CLAUDE.shared.md"
done
[ "$ran" -gt 0 ] || fail "no preamble subject found under $TESTS_DIR"
