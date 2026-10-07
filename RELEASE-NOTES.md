# foreman-sh 0.3.8

Finished briefs leave the working folder, every build proves its checks, and a session launched by foreman keeps its transcript.

## In short

- **A briefs archive per lane.** `hw briefs archive <lane>` moves finished briefs to `briefs/archive/<yyyy-mm>/` with a manifest. A brief counts as finished when the ledger, a merged `task/<t>` branch or a reported run says so. A brief with no evidence and no commit in 21 days moves as `stale`. Without `--apply` it only prints the plan, and `hw briefs restore` / `unarchive` undo it. A task whose brief is archived is refused by name instead of launching empty.
- **`checks.md` on every build.** A build leaves one row per requirement of its brief, each with a verdict and the evidence. `done-invoker` refuses a done report without it.
- **Sessions keep their transcript.** A Claude session that `brain` or `hw` launches no longer inherits the child-session marker, so the "Transcript saving is off" warning is gone and `--resume` works.
- **The installer brings every guard `hw` needs**, including the gentle home guard. A clean install can now dispatch `--sdd gentle`.

## What changed

8 changes since 0.3.7.

### Added
- `hw briefs archive <lane> [--apply]` moves a lane's finished briefs (`hw ledger`
  says done, or a run reported, with no live worktree) to
  `briefs/archive/<yyyy-mm>/` in one commit, with a `MANIFEST.tsv` per month.
  Without `--apply` it prints the plan. Briefs that are in progress, never
  dispatched, `_`-prefixed, untracked or modified stay where they are.
- `hw briefs restore <lane> <task>` and `hw briefs unarchive <lane> --manifest
  <file>` undo one brief or a whole run: the same tree, byte for byte.
- A build task now leaves `checks.md` in its artifacts: one row per requirement
  of the brief's Proof and Verification, with a verdict and its evidence.
  `done-invoker` refuses a done report when the file is missing, has no rows or
  has a row with no verdict, and the report names the counts.

### Changed
- `hw <lane> <task>` for an archived task refuses, naming the archived path and
  the restore command; `--no-brief` does not override it, `--brief <path>` runs it.
- `hw ledger` lists archived briefs too (`archived: true`) and adds `done_at`.

### Fixed
- A re-task (`hw next`) records the pane's own model, effort and vendor in the
  ledger, not the defaults of a launch that did not happen.
- `install.sh` installs `deny-gentle-real-home.py`, the guard `hw` copies into
  the gentle home. Before this, a clean install that dispatched `--sdd gentle`
  failed at that copy.
- A Claude Code session that `brain` or `hw` launches no longer inherits
  `CLAUDE_CODE_CHILD_SESSION` from a terminal multiplexer started inside another
  Claude session. The "Transcript saving is off" warning is gone, and the session
  keeps its transcript, so `--resume` and `brain relaunch` have a conversation to resume.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.7

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
