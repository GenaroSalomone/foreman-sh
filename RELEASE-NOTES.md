# foreman-sh 0.3.2

Suites take turns, old artifacts clean themselves up, and fewer surprises when tasks are re-dispatched.

## In short

- **Suites take turns.** Heavy test runs wait for a free slot instead of saturating the machine, a release cut goes first, and `hw status` shows who holds a slot.
- **Old artifacts clean themselves up.** `hw reap` removes the artifacts of reported tasks after 30 days and backups after 60, configurable per lane, and logs every removal.
- **Re-dispatch keeps what matters.** `hw next` carries the brief's authorization, and an executor in a lane with no specialists is told so.
- **Release notes can be rebuilt.** `notes --refold` rewrites a version already in the changelog, and the commit gate runs the goldens a change makes stale.
- **Decisions write where they run.** `decisions archive` and `supersede` touch the tree they run in, and a malformed entry is named instead of blocking the lane.
- **Fewer false reds.** Brief keys and values are read case-insensitively, and the opencode adapter waits as long as it is told.

## What changed

17 changes since 0.3.1.

### Added
- The OpenCode blocked-reason adapter reads `HW_RPC_TIMEOUT_MS` (default 2000) as how
  long it waits for one `herdr-rpc` call, so a loaded machine can be given more time
  without the adapter ever blocking a turn.
- Heavy runs take turns. The setup suite, the verification inside `hw done`,
  `hw suite` and the release cut each take one slot of a machine-wide gate
  (`bin/suite-gate`) before they start: `max(1, cpus/4)` at once, set with
  `HW_SUITE_SLOTS`. A run that waits says so once and never fails for the wait
  (`HW_SUITE_GATE_WAIT_MS`, default two hours, then it goes ahead without a slot).
  A slot held by a dead process is reclaimed, and the cut goes first without
  killing anyone. `hw status` shows the slots in use and who waits.
- `hw reap` retires the artifacts of a finished task: a work dir whose task
  reported (done, or blocked and closed), with nobody in it, becomes safe once
  every artifact is older than 30 days, and a backup (`*.tgz`, `*.tar.gz`,
  `*.tar`, or a name containing `backup`) once it is older than 60. A task that
  never reported, or has a live pane, is kept at any age. The dry run says
  «retention: 40d > 30d».
- The windows are `retention.artifact_retention_days` and
  `retention.backup_retention_days` in `projects.json` (0 disables a window).
  Every removal by retention appends a line to `.hw-reap-retention.log` under
  the brain: time, lane, path, task, age, size, reason.

### Changed
- An executor whose lane has no specialists is told so: "no specialists in this lane;
  ops-investigator / qa-tester are not available here". The line used to be omitted.
- The pre-commit test run now also runs a slow test that declares itself a golden
  (`# gate-reference:`) when a staged file is one it names. Until now such a
  golden (the dispatch output, the installer comparison) stayed stale until the
  full run after the merge; the executor's own verify already ran them.
- `setup/test-hw` lowers its parallel jobs, and says so, when the 1-minute load
  average starts above twice the cpu count (never below one; an explicit
  `HW_TEST_JOBS` is left alone).

### Fixed
- `hw status` reads a task closed by hand with `hw done` as `closed-unreported` even when it
  never ended a turn; it used to read `DIED-BEFORE-FIRST-TURN`, a death.
- A brief key written with capitals (`Kind: design`, `Boundary:`, `Deployed_Check:`) is
  now read by every reader of the frontmatter, not only the contract check. `Kind: design`
  used to give the task the sonnet default instead of opus.
- `hw next --brief` now delivers the brief's `authorizes:` block (and refuses an uncited or
  malformed one, like a first dispatch). A re-tasked executor used to work without the
  authorization its brief declared.
- `decisions` writes the worktree it is run in. Run through a PATH link to another
  checkout of the same repository, `archive` and `supersede --apply` used to
  rewrite that other checkout's files; they now use the current worktree and say
  which tree they used.
- `decisions supersede` no longer refuses a whole lane over one `Reverses` line it
  cannot read: it skips that entry with a warning that names it and moves the
  reversals that are unambiguous. `decisions check` lists the unreadable lines.
- `hw sweep` finds the runs under a lane's `.worktrees` directory. It asked `fd`,
  which skips the git-excluded `.hw/`, so a reported executor's tab there was
  never closed by `hw sweep --apply`.
- `HW_ARTIFACT_RETENTION_DAYS` and `HW_BACKUP_RETENTION_DAYS` are read as
  decimal days: `08` and `09` are eight and nine days (they used to fail as bad
  octal), and `00` disables the window like `0` (it used to be a 0-day window
  that made every finished task's artifacts safe to remove).
- A brief's `kind:` value is matched without regard to case: `Kind: Design`
  now runs opus like `kind: design`.
- `HW_RPC_TIMEOUT_MS` in the OpenCode adapter is floored and capped to what the
  runtime accepts; a fractional or huge value used to drop the publish silently.
- `setup/release/notes --refold` no longer dies with a traceback on a
  CHANGELOG whose last line is the version's heading with no final newline.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.1

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
