# L26 inventory — the retired claims and what holds them now

Source: the per-task lists of retired anchors (`go060-s4-hw-sin-bash` and `go060-s5-tools-sin-bash`, archived under
the run archive's `setup/*/.artifacts/anchors.md`), the `Unresolved` tables: **77 rows in S4 + 13 in S5 = 90 claims** (the
"about 90" of KNOWN-LIMITATIONS L26). One row per retired claim, with its kind, the Go location, and what holds it today.

Kinds, as L26 defines them:

- **kind 1** — the wording of a message or a refusal. A mutant is worth adding only where a golden would *not* also move;
  where the golden/test already compares those bytes, the claim is **held by that golden** and needs none.
- **kind 2** — a guard on one branch of a verb, where a behavioural arm still runs the verb through `hw-core` and the Go
  has no anchor on the branch. These need a Go mutant anchored on the branch, killed by a subject or a Go test.
- **kind 3** — a defect class only bash had (an unset variable under `pipefail`, a pipeline that fails silently,
  `grep -c || echo 0`). No Go twin; needs nothing.
- **n/a** — the claim was about the *deleted bash's shape* (an extraction, a comment, an old-binary pair): there is no Go
  branch to guard and no wording to hold. Nothing to add; the row stays visible so nobody reads it as covered.

"now" is one of: a Go mutant id (added by this task, proved killed), `held by <golden|test>`, `nothing needed`, or
`owed — <the fixture/arm it still needs>`.

## Counts

| disposition | items |
|---|---|
| Go mutant added (kind 2), 0.7.0 | 12 |
| Go mutant added (kind 2), the rest of L26 | 8 |
| kind 2, still owed (each names the fixture it is missing) | 36 |
| kind 1, held by a golden or a byte-comparing test | 22 |
| kind 3 or n/a — nothing to add | 16 |
| total items | 94 |

The unit here is an **item**, not a source row: the 90 `Unresolved` rows are all walked, plus the four rows of the Held
tables that name an unanchored remainder (159-M04, 177-M02, 177-M03, 232-M05). Where one source row carried several mutants
(the 806 EXIT trap's three; 150-M01/M02) they are grouped into one item, and where one row names several arms (40's six) they
stay one item. `798-M04` appears twice in the S4 table and once here. Every source row is listed.

The 36 rows still owed are the honest remainder of kind 2: none of them has **an arm a Go mutant could be killed by** today —
neither a corpus case whose golden would move, nor a Go test that names the branch. Adding the anchor alone would be a vacuous arm,
which `setup/tests/540` and the corpus judges refuse. Each row names the fixture/arm that would make it killable, and the rows that
were delivered in 0.7.0 are in the table above; the reason a row was owed was checked, not assumed — 990's six, 64's two and 986
were owed only because nobody had looked at the corpus, which already carried the fixture.

## Added here (kind 2, proved)

| test | retired claim | Go location | now |
|---|---|---|---|
| 806 | `--resume` dropped from the relaunch | `core/brain.go:291` | go-806-M01 |
| 806 | a working pane is relaunched | `core/brain.go:273` | go-806-M02 |
| 806 | the caller's `CLAUDE_CONFIG_DIR` is the pane's | `core/brain_launch.go:348` | go-806-M03 |
| 806 | the alias is the lane name (first directory entry wins) | `core/brain_helpers.go:71` | go-806-M04 |
| 806 | a failure after `/exit` is counted as a skip | `core/brain.go:294` | go-806-M05 |
| 806 | relaunch exits 0 over a failed pane | `core/brain.go:340` | go-806-M06 |
| 806 | the cwd is not realpath'd | `core/brain_helpers.go:58` | go-806-M07 |
| 806 | the EXIT trap is gone | `core/brain.go:75` | go-806-M08 |
| 806 | the EXIT trap also holds `--reset` | `core/brain_reset.go:265` | go-806-M09 |
| 806 | the EXIT trap turns success into 75 | `core/brain.go:75` | go-806-M10 |
| 67 | the chaining default flips to retention | `core/t5_manifest.go:367` | go-67-M06 |
| 185 | the speckit phase note leaks into the `--sdd none` prompt | `core/t5l_deliver.go:51` | go-185-M01 |
| 990 | the dispatch warns when a brief names a remote marker and carries no `authorizes:` | `core/t5_brief.go:1098,1269` | 844-A170 (rest of L26) |
| 990 | the warning refuses instead of warning | `core/t5_brief.go:1269` | 844-A171 (rest of L26) |
| 990 | the manifest's `authorizes` line loses the marker | `core/t5_brief.go:1280` | 844-A173 (rest of L26) |
| 990 | the check fires although `authorizes:` is present | `core/t5_brief.go:1098` | 844-A172 (rest of L26) |
| 990 | the marker match is a bare substring (`product` trips `prod`) | `core/t5_brief.go:1146` (`remoteMarker`) | 844-A174 (rest of L26) |
| 990 | the frontmatter is scanned too | `core/t5_brief.go:1146` | 844-A175 (rest of L26) |
| 64 | the reopened state, and `task_done`/`task_reopened` on the current task's markers | `core/status.go` (~939, ~1035) | 826-G35, 826-G36 (rest of L26) |
| 986 | the manifest's pi brain-guard line | `core/t5_manifest.go:519` | 844-M51 (rest of L26) |

## kind 2 — still owed (36)

| test | retired claim | Go location | owed because |
|---|---|---|---|
| 806 | the lane is a raw prefix of the cwd | `core/brain_helpers.go:54-78` | the parent check is redundant beside the `SameFile` loop; no fixture pane fails a prefix mutant |
| 806 | the lane is matched by spelling, not identity | `core/brain_helpers.go:71-77` | the fixture's case-insensitive arm is skipped on macOS; no pane fails a spelling mutant |
| 806 | success is printed after a failed start | `core/brain.go:294-340` | the same test's no-`relaunched` assertion is held by go-806-M05/M06; an anchor of its own has no arm |
| 18 | a git that cannot answer leaves reap UNDETERMINED (`rc != 128`) | `core/reap.go:886`, `core/t4_reap_work.go:123` | no case drives a git stub exiting 127 on the reap path |
| 40 | set-wait: monitor focus, N-1 targets, concurrency, three witness-repetition arms (6 claims) | `core/wait.go` | the six arms run through the shim in test 40, but no corpus case carries a `wait` golden the mutant would move |
| 53 | a brief-body agent is only taken for `mode: subagent` | `core/t5_brief_agents.go:562` | no Go test isolates the mode test and no case golden moves on it |
| 70 | the turn-end predicate ignores its payload | `core/t3_turnend.go:101` (`t3LiveChildren`) | M04's hook arm covers forwarding, not the predicate; no case isolates it |
| 83 | `verifyAtClose` decides its verifydir (lane checkout when the worktree is the workdir) | `core/t4_done_verify.go:305` | no Go test or golden names the choice |
| 08 | the invoker pane is resolved once per dispatch (`invokerPaneInit`) | `core/t5_reuse_invoker.go:243` | `TestRtInvokerInitAndValidate` compares against `bin/hw` and is stale |
| 118 | `HW_VERIFY_TIMEOUT_SECONDS` bounds the pinned command | `core/t4_done_verify.go:343-360` | no golden sets the variable (done-e-vok/vfail do not) |
| 134 | a run with a done marker is skipped by `depsContendingOthers` | `core/t5_manifest.go:122` | arm exists in test 134 §4 through the shim; no corpus case has deps + a done marker |
| 142 | the close's suite lock closes over INT/TERM as 130/143 | `core/t4_done_verify.go:402-411` | no Go test on the done path (TestT6SuiteOrphan covers `hw suite`) |
| 202 | a table-only lane's artifacts dir and worktree decision | `core/t4_reap_work.go:22`, `core/t4_done_verify.go:281` | no table-only-lane fixture drives either decision |
| 640 | the reap manifest drops its readlink target | `core/t4_reap_apply.go` (`reapApplyTree`) | test 640's `CP_SPOIL=link` arm runs through `hw-core`; no corpus case holds the manifest line |
| 640 | the reap manifest drops its cksum/cmp | `core/t4_reap_apply.go` (`reapApplyTree`) | same, `CP_SPOIL=file` |
| 660 | `hw status` runs the memory audit (`startAudits`) | `core/status.go:113,216` | the claim is a grep of the Go source, not an arm |
| 670 | PR merge evidence (`prList`, `prMerged`) | `core/t4_reap_wt.go:299,335` | the denials are positive checks in test 670; no corpus case carries a gh-stub golden |
| 71 | the failed-delivery refusal in `hw next` | `core/t4_next_run.go` | positive assertions in test 71; a T4 corpus row needs an `hw next` case whose delivery fails |
| 730 | `hw done`'s verification takes the machine suite gate | `core/t4_done_verify.go:427` | the arm is a structural grep plus an `sd` text mutant, not a behavioural one |
| 770 | a dirty tree skips a cached verdict | `core/t4_done_verify.go:182-184` | no read test; needs a done case with a dirty worktree + a green verdict |
| 770 | a fast verdict never covers a full pin | `core/t4_done_verify.go:47-51,72-74` | no read test for the candidate selection or the isFast/gate mismatch |
| 770 | the docs-only drift cover | `core/t4_done_verify.go:129` (`doneDocsCoverScript`) | `done-z-vcached` may cover the hit; its code-drift negatives are unread |
| 785 | a revived run is judged by its spent lease in the dry run | `core/t4_next.go:165` | N02 runs through the shim; no T4 case golden moves on the dry run alone |
| 785 | the live-lease clause (not the whole refusal) | `core/t4_next.go:165` | 862-G05 anchors the line but kills the whole refusal; N06 runs through the shim |
| 798 | `hw ruling` kicks the cockpit (`cockpitKick`) | `core/t3_ruling.go:323` | the only arm needs a real herdr, and it is a live assertion, not a golden |
| 900 | the preflight checks every manifest row | `core/deps.go:34` (`depsRows`) | D03/D04 notice it through the real preflight; no corpus case holds a preflight golden |
| 991 | the pi start prepares the project skills (`piProjectSkillsPrepare`) | `core/t5l_agent.go:332` | no test of the wiring; `TestPiProjectSkillsPrepare` holds the prepare only |
| 1003 | the opencode explorer overlay | `core/t5_explorer.go` (`explorerResolve`, `opencodeOverlay`) | held by a positive dry-run assertion and golden br-ag-oc-ok, not by a corpus case |
| 1003 | claude's explorer injection decision | `core/t5_explorer.go:155` | positive assertion; no case golden moves on the decision alone |
| 1003 | pi's explorer reaches the agent dir | `core/t5_explorer.go` (pi arm) | positive assertion only |
| 1003 | `checkTopTierCeiling` | `core/t5_explorer.go` / `core/t5.go` | positive assertion in test 1003; no ceiling case golden exists |
| 1003 | `--top-tier-why` is parsed | `core/t5.go:281` | positive assertion only |
| 1003 | requires-agents knows the injected explorer | `core/t5_manifest.go` | positive assertion only |
| 1003 | `modelIsTopTier` decides the top tier | `core/t5_explorer.go` | the retired marker lived in `bin/project-spaces.sh`; no Go test names the function |
| 150 | a pane has an owner (M01, M02) | — | the only arms mutated `bin/hw-actions`, which is no longer read by `hw`; a Go anchor needs a test of the owner rule first |
| 800 | a preflight side effect would be caught | `core/preflight.go` | P08's snapshot arm was removed with the bash; no Go analogue |

## kind 1 — held by a golden or a byte-comparing test

| test | retired claim | held by |
|---|---|---|
| 232 | `CLAUDE_CODE_SUBAGENT_MODEL_FORCE` is never set | test 232's own grep of `core/*.go` (a text assertion, not an arm) |
| 159 | `decision` is a known frontmatter key | golden `br-fm-decision` (the warning would show) |
| 177 | the committed agent tree is indexed by `name:` | golden `br-ag-theta-claude` |
| 177 | `<taskDir>/opencode.json` is read at status 1 vs 0 | golden `br-ag-oc-badmode` |
| 232 | the receipt row records `subagent_model` | golden-t5l `l-gw-subagent-model` (`tree`) |
| 38 | a bare command is refused | `TestBrMeasuredEvidence` (byte-compared message; `core/t5_brief_test.go:134`) |
| 38 | a bare path is refused | same test, same `else` |
| 491 | `hw next`'s dry run states the mode it would set | reference `next_task_dry_*` |
| 491 | `hw revive`'s dry run says the run would be reopened | reference `revive_*_dry` |
| 491 | `hw ruling`'s dry run states another cap | reference `ruling_dry_working` |
| 492 | a reported run's turn end is not a handback | reference `te_reported` / `te_reopened` |
| 492 | the `--force` wording | reference `done_unreported`, `core/t4_done_guard.go:235` |
| 492 | the unknown-lane wording | reference `done_unknown_lane` |
| 493 | `done-invoker` reads channel-send's 5 as retry | golden `done_invoker_rc5.out` |
| 750 | the QA line carries the dev-server port | golden-t5l `m-c-note` |
| 750 | `hw next` re-reads the run's port | golden-t4 `next-send-lane-note` |
| 811 | the re-task ledger line's run name | goldens `next-send-idle`, `next-sdd-speckit` (`tree`) |
| 811 | the re-task ledger line's model/effort/vendor | same goldens + `TestNextLedgerRecordsTheSubagentModel` |
| 1003 | the tier note in the launch prompt | golden-t5l launch `calls` (`DELEGATE BY MODEL TIER`) |
| 155 | the base-ref prefix is empty unless declared | `TestLaneGetters` + the table assertion |
| 155 | a lane with no checkout is skipped by the surveys | the table assertion on every lane's checkout |
| 05 | the strong-sender messages | test 05's own byte assertions through `hw-core` (`core/t5l_deliver.go`) |

## kind 3 / n/a — nothing to add

| test | retired claim | why |
|---|---|---|
| 140 | the suite-lock comment text | comment text of the deleted bash |
| 52 | the `\|\| true` pipefail guard on the receipt-less read | no pipeline in Go; `receiptLastJQ` |
| 64 | the dirty count's `\|\| echo 0` | `grep -c` integer error, bash-only |
| 90 | `_wt_irreplaceable`'s fd/git-ignored failures | no pipeline to fail silently in Go |
| 141 | one detector at both moments | Go has two detectors, each unit-tested; a structural claim |
| 827 | parallel == serial (`HW_STATUS_SERIAL`) | `hw-core` has no serial mode; replaced by determinism |
| 930 | `hw help flags` dying on a missing deps.conf | `core/deps.go` returns no rows; the bash-only startup death has no twin |
| 68 | `${VAR+set}` vs `${VAR:-}` | bash spelling; Go has one condition (`done.go`, `ask.go`) |
| 703 | the brief's `requested_by` replaces the flag's | Go has no preset-citation path |
| 700 | the old-binary pair | the old binary needs deleted `lib/hw/*.sh`; cannot run |
| 155 | the executor PATH carries no worktree `bin/` | removed: nothing to assert |
| 155 | the removal recipe names the lost-`.git` disposition | `core/reap.go disposition` builds it; no golden names it |
| 03 | `launchRollback`'s `!launchWTCreated` arm | no golden or test of its own |
| 86 | each status report runs in isolation | a claim about the deleted bash/python extraction |
| 14 | the leased verdict is decided before dirty/unmerged | a code-order claim; no golden has a dirty+leased worktree |
| 1011 | `briefSecretsNote` | no Go test or golden |
