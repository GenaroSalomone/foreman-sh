# foreman-sh 0.3.1

One-command upgrades, worktrees that clean up after themselves, and a clearer `hw status`.

## In short

- **Upgrade in one command.** `foreman-sh upgrade` remembers how you installed, updates, checks and shows what is new; `hw status` tells you when a release is out.
- **Disk stays under control.** `hw done` removes a merged, clean worktree with its branch and database, and `hw status` warns before a lane piles up.
- **Live work is protected.** A worktree is locked while its executor runs, so no cleanup can touch it.
- **Know before you resume.** `hw status` shows how much context each executor has used and any ruling still waiting to be delivered.
- **Fewer false alarms.** A task closed by hand is shown as closed, not as a session that died.
- **Authorizations travel with the brief.** An `authorizes:` line hands the executor exactly what it may do remotely, so it no longer stops to ask; and sonnet is now the default model.

## What changed

17 changes since 0.3.0.

### Added
- `hw status` prints one line per lane past its worktree count, its disk use or
  the `.next`/`.turbo` the last `hw reap` summed (`reap.max_worktrees`,
  `reap.max_disk_pct`, `reap.max_build_gb`; defaults 40, 85%, 30 GB), naming
  `hw reap <lane> --apply`, and one line for tasks that finished, never
  reported, and whose brainer pane is gone.
- A worktree `hw done` or `hw reap --apply` keeps for a git reason (dirty,
  unmerged, irreplaceable) sheds its git-ignored `.next` and `.turbo`, unless a
  live pane, process or lock is in it or it is outside the lane's worktree root.
- hw locks a worktree while its executor lives (`git worktree lock`) and
  `hw done` unlocks it; `hw reap` keeps a locked worktree.
- `hw reap` records the per-worktree databases hw provisioned, and `--apply`
  dumps and drops a recorded one whose worktree is gone.
- An executor may run `hw reap <lane>` (the survey, never `--apply`).
- A brief can carry `authorizes: <destination> :: <operation> :: <handle|none>`
  (one line per entry). `hw` validates the shape, refuses it without
  `requested_by:`, checks a named Keychain handle, shows it in the manifest and
  delivers it to the executor as a quoted block that authorizes only those
  destinations and operations.
- `hw done` now leaves a `closed-by-hand` mark in the task's state directory
  (when, which flag, whether a report existed). `hw status` shows such a task
  as `closed-unreported` instead of `finished, unreported`, so a task closed on
  purpose no longer looks like a dead one. Runs closed before the mark existed
  are reclassified when read, from the `verify_run` line only `hw done`
  writes; nothing is backfilled.
- A re-tasked chain row names the last task that did report (for example
  "task 3 reported, task 4 closed before reporting").
- `foreman-sh upgrade --brain DIR` (also `install.sh --upgrade`) installs the
  newest release with the flags the brain was installed with, runs `--check`
  and prints the release's "In short". `--to VERSION` installs that one, the way
  back; `--dry-run` says what it would do and changes nothing. The flags are
  recorded by every install in `DIR/.foreman/install.json` (never a secret): the
  install's own flags (bin dir, permissions, lane, repo, base). What a lane
  declares (vendor, model, operator, floor, request rule) is read from
  `projects.json` when `upgrade` runs, so a hand edit there is kept, never
  replayed back; a brain with no record is refused, naming the flags to pass once.
  `--to` a release older than `upgrade` is not supported: that release has no
  `upgrade` to be handed the brain.
- `hw status` says in one line when a newer release exists. It asks at most
  once a day, caches the answer in `DIR/.foreman/latest-release.json`, and says
  nothing offline or in a brain with no record.
- `hw status` shows each live executor's context size, with a prompt to prefer a
  fresh executor over `hw next` or a ruling once it passes 150k tokens, and
  the number of queued rulings with the age of the oldest. `hw receipt` shows
  the same context line for an open run.

### Changed
- A Claude executor launched without `--model` now runs sonnet, and a brief
  declaring `kind: design` runs opus; `--model` still wins. The manifest says
  where the model came from.
- The README's "Upgrading" section, INSTALL.md, the Homebrew caveats and each
  release's "Upgrading from" notes say `foreman-sh upgrade` instead of "run
  install.sh again with the same flags".
- Release notes open with a person-written "In short" summary
  (`setup/releases/highlights/VERSION.md`), which is what `upgrade` prints.
- The context read behind `hw next` and `hw status` is bounded in time, so a
  stuck pane cannot hang either command.

### Fixed
- A branch squash-merged through a GitHub PR, whose paths the base changed
  again since, read `unmerged` for good. A PR merged into the lane's base whose
  head contains the branch tip is now merge evidence; an open PR, a PR merged
  elsewhere, a failing or missing `gh`, or a non-GitHub origin is none. Such a
  branch is deleted with its worktree.
- The receipt of a re-tasked executor no longer copies the previous task's
  `report_*` tokens as if they were the current task's.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.0

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
