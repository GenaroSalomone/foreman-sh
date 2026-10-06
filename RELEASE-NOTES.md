# foreman-sh 0.3.4

Closing an unchanged task takes seconds, gentle-ai 4.0.0 runs with per-task review, and `bin/hw` starts splitting into modules.

## In short

- **`hw done` reuses a verdict it already has.** If HEAD is still the dispatch base and the tree is clean, or the exact tree (or one that differs only in lane documentation) already holds a green verdict of the pinned kind, the close skips the re-run and the receipt names the verdict. A fast verdict never satisfies a full pin; a dirty tree is never a hit.
- **pre-push survives an older worktree.** A worktree whose library predates the lane-doc cover pushes with the cover off and one line saying why, instead of dying.
- **gentle-ai 4.0.0 for `--sdd gentle`,** signature- and checksum-verified, available to more lanes. `review: rdd` turns receipt-driven review on in a private home per task, so concurrent tasks never switch it off under each other; the shared home stays untouched. The home's guards run from copies inside it.
- **`hw reap` lives in `lib/hw/reap.sh`.** `bin/hw` sources its modules from `lib/` beside it; install and export ship `lib/`. Output is byte-identical.

## What changed

9 changes since 0.3.3.

### Added
- `--sdd gentle` is available in a second lane (`sdd_modes` in `projects.json`),
  on the same terms as in the first: Claude only, its own worktree, no `--keep-pane`.
- `review: rdd` in a `--sdd gentle` brief opts into native RDD, run against a local
  clone because gentle-ai keeps its review store inside the repository's `.git`.
  The reviewer roles ride as a plugin with their models pinned (judgment roles on
  opus, readability on sonnet), and a local SubagentStop hook appends one line per
  role run to `$HW_ARTIFACTS/rdd-log.jsonl`; telemetry stays off.

### Changed
- `hw done`'s cached close also covers `bash setup/verify-for-push` (a full
  verdict for the exact tree is a hit; a fast verdict never satisfies a full
  pin) and a tree that differs from a full-verdict tree only in lane
  documentation, recorded as `cached — green full verdict for tree <t> (<commit>);
  differs only in lane docs: <paths>`. The lane-doc rule is now one function,
  `docs_only_diff_covers` in `setup/hooks/suite-trigger-pattern.sh`, called by
  both `pre-push` and `hw`. Dirty, red and unreadable still run the command.
- `hw done` no longer re-runs a pinned verification it can already answer from
  disk: a clean task whose HEAD is the base recorded at dispatch records
  `skipped — no change since dispatch (<sha>)`, and a clean tree with a green
  `HW_TEST_GATE=fast bash setup/test-hw` (or full) verdict records
  `cached — green verdict for tree <sha> from <when>`. A dirty tree, a red or
  unreadable verdict or any other pinned command still runs as before;
  `HW_VERIFY_FRESH=1` forces the run.
- `--sdd gentle` runs gentle-ai 4.0.0 (was 3.7.0), pinned to the four checksums of
  the release's own `checksums.txt`, whose minisign signature was verified. Only the
  ODD protocol of the home's `CLAUDE.md` reaches the executor (`odd-block.md`): 4.0.0
  also installs a 45 KB orchestrator block and five hooks, and `hw gentle-home` now
  keeps the block out and removes the hooks from gentle-ai's own home.
- `hw gentle-home` proves telemetry off from the home's own state, not from the
  wrapper's environment, and gives gentle-ai a private engram port and data dir.
- `hw reap` and the removal `hw done` shares with it moved out of `bin/hw` into
  `lib/hw/reap.sh`, the first module of a split of `bin/hw` by command. Behaviour
  is unchanged. The installer now mirrors `lib/` beside `bin/`, and `lib` is a
  reserved lane name.

### Fixed
- `setup/hooks/pre-push` no longer dies (exit 127) in a worktree whose
  `suite-trigger-pattern.sh` predates `docs_only_init`: the lane-doc cover is
  then off (fail closed, the push needs its own verdict) and one stderr line
  says why. `hw`'s `_verify_docs_cover` treats such a lib as a miss.
- `hw gentle-home` copies its two guards into `<home>/guards/` and the settings run them from there. Provisioned from a task worktree, the settings named that worktree's `setup/guards/`, so the hooks dangled once it was removed.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.3

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
