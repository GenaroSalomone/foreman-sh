#!/usr/bin/env bash
# The cockpit state writer says what is on disk, atomically, fast, and dies with its pane
#
# bin/cockpit-state writes the file the brainer's cockpit mod reads (cockpit/schema.json,
# design H1–H3). Claims, each a check in _cockpit_writer_checks.py:
#
#   · classes      every attention class comes out of the DISK facts — an ask is a valid
#                  hold file (not a token that outlives its task), a report is the done
#                  marker, rows of another brainer and panes with no run are absent, ctx is
#                  null rather than a guess, rows are in hw's order, the output validates
#                  against schema.json, and an idle executor holding an ask gets a
#                  pending_reply (the reply button) and no ruling button
#   · done_tokens_outlive_the_task  task 1's done tokens are not task 2's report; a stranded
#                  (undelivered, newer) report with no marker still shows
#   · private_files  the state file is 0600 in a 0700 directory (it carries report summaries)
#   · loop_survives_a_failed_write  a write that raises ends one beat, not the heartbeat
#   · row_order    attention rank, then attention_since ascending, then id
#   · herdr_down   an RPC failure is herdr.ok:false with no rows
#   · rows_cap     64 rows at most, the lowest-ranked dropped and counted in totals.omitted
#   · size_cap     256 KiB at most, rows dropped from the end
#   · summary_rejoin  the done-invoker's 80-char chunks (trailing blanks trimmed by herdr)
#                  read back as the text they carried, soft breaks and a hard cut alike
#   · atomic       a reader never sees half a file (>= 1000 concurrent reads); every write is a new inode; seq rises by 1
#   · killed_writer  kill -9 in the middle of a write leaves a whole file, and the next writer removes the tmp it left
#   · debounce     15 events in 100 ms are at most 3 writes, and none is dropped
#   · loop_dies_with_parent  --loop leaves when the shell that started it is gone
#   · loop_binds_to_pane  the detached writer `bin/brain` starts: one per brainer, survives its
#                  launching shell and a herdr outage, leaves when the brainer's pane does
#   · ctx_from_transcript  44424 tokens = 22.2 % (probe P5), 1M by model id, sidechains skipped
#   · cut_rule     a live `release cut` slot says so; a dead holder or a reused pid does not
#   · kick_names_its_brainer  a kick whose brainer does not resolve writes nothing (never the executor's
#                  own pane) and logs it; a resolved one still kicks its brainer
#   · timing_40    --once under 300 ms for 40 executors (best of 7; the numbers are printed)
#
# and each of the writer's rules has a mutant that the checks above must kill.
#
# Run alone while working on this subject:
#     bash setup/tests/797-cockpit-state-writer.sh
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

[ -x "$ROOT/bin/cockpit-state" ] || { pass "797: this tree carries no cockpit writer"; exit 0; }
CHECKS="$ROOT/setup/tests/_cockpit_writer_checks.py"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/t797.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

out="$(python3 -I "$CHECKS" "$ROOT/bin" 2>&1)" || { printf '%s\n' "$out" >&2; fail "797: the writer's checks failed on the real tree (above)"; }
printf '%s\n' "$out"
pass "797: the writer says what is on disk — classes, caps, atomic write, debounce, loop, ctx, cut, timing"

# ── the mutants: each rule, broken in a COPY of the writer, must be caught by its check ──
mutant_dir() {  # <id> — a COPY of the writer and the tools it loads; sets MD
  MD="$TMP/m-$1"
  mkdir -p "$MD"
  cp "$ROOT"/bin/cockpit-state "$ROOT"/bin/runenv "$ROOT"/bin/holdfacts "$ROOT"/bin/hw-actions "$MD/"
}
kill_mutant() {  # <id> <check> <needle>… — the check, run on the mutated copy, must fail with the mutant's own evidence
  local id="$1" check="$2" res rc=0; shift 2
  res="$(python3 -I "$CHECKS" "$MD" "$check" 2>&1)" || rc=$?
  [ "$rc" -ne 0 ] || fail "797: mutant $id SURVIVED — check $check passed on a writer with the rule broken"
  saw_mutant "$id $check" "$res" "$@"
}
mutant_dir 797-M01; mutate_anchor 797-M01 "$MD/cockpit-state" 'open(path, "w").write(body)'; kill_mutant 797-M01 atomic "rewritten in place" "a reader saw a partial file"
mutant_dir 797-M02; mutate_anchor 797-M02 "$MD/cockpit-state" 'pass'; kill_mutant 797-M02 debounce "writes for 15 events"
mutant_dir 797-M03; mutate_anchor 797-M03 "$MD/cockpit-state" 'if False:'; kill_mutant 797-M03 classes "another brainer's executor is listed"
mutant_dir 797-M04; mutate_anchor 797-M04 "$MD/cockpit-state" 'break'; kill_mutant 797-M04 size_cap "over the 256 KiB cap"
mutant_dir 797-M07; mutate_anchor 797-M07 "$MD/cockpit-state" 'if i < len(present) - 1:'; kill_mutant 797-M07 summary_rejoin "rejoined"
mutant_dir 797-M08; mutate_anchor 797-M08 "$MD/cockpit-state" 'valid = True'; kill_mutant 797-M08 classes "pending_reply set on"
mutant_dir 797-M09; mutate_anchor 797-M09 "$MD/cockpit-state" 'has_marker = True'; kill_mutant 797-M09 done_tokens_outlive_the_task "made task 2 a report"
mutant_dir 797-M10; mutate_anchor 797-M10 "$MD/cockpit-state" 'return (0, 0, r["id"])'; kill_mutant 797-M10 row_order "not in hw's order"
mutant_dir 797-M11; mutate_anchor 797-M11 "$MD/cockpit-state" 'if False:'; kill_mutant 797-M11 loop_dies_with_parent "outlived the shell"
mutant_dir 797-M17; mutate_anchor 797-M17 "$MD/cockpit-state" 'if False:'; kill_mutant 797-M17 loop_dies_with_parent "exited at once"
mutant_dir 797-M18; mutate_anchor 797-M18 "$MD/cockpit-state" 'if False:'; kill_mutant 797-M18 loop_binds_to_pane "outlived its brainer's pane"
mutant_dir 797-M19; mutate_anchor 797-M19 "$MD/cockpit-state" 'pass'; kill_mutant 797-M19 loop_binds_to_pane "second writer"
mutant_dir 797-M20; mutate_anchor 797-M20 "$MD/cockpit-state" 'with open(tmp, "w", encoding="utf-8") as fh:'; kill_mutant 797-M20 private_files "is mode 644"
mutant_dir 797-M21; mutate_anchor 797-M21 "$MD/cockpit-state" 'except ZeroDivisionError as e:'; kill_mutant 797-M21 loop_survives_a_failed_write "died with its first failed write"
mutant_dir 797-M22; mutate_anchor 797-M22 "$MD/cockpit-state" 'pass'; kill_mutant 797-M22 killed_writer "tmp files of killed writers survive"
mutant_dir 797-M12; mutate_anchor 797-M12 "$MD/cockpit-state" 'omitted = 0'; kill_mutant 797-M12 rows_cap "totals.omitted is wrong"
mutant_dir 797-M15; mutate_anchor 797-M15 "$MD/cockpit-state" 'window = WINDOW_DEFAULT'; kill_mutant 797-M15 ctx_from_transcript "window is not honoured"
mutant_dir 797-M23; mutate_anchor 797-M23 "$MD/cockpit-state" 'if False:'; kill_mutant 797-M23 kick_names_its_brainer "fell back to the executor's own pane"
mutant_dir 797-M16; mutate_anchor 797-M16 "$MD/cockpit-state" 'if False:'; kill_mutant 797-M16 cut_rule "reused pid"
