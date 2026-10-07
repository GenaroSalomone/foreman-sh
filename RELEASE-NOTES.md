# foreman-sh 0.3.7

Every dispatch is now recorded and every task needs a brief, brainers relaunch in place with the cockpit, and `bin/hw` starts splitting by command.

## In short

- **A durable dispatch ledger.** `hw` records every real dispatch: the brief and its sha, run, account, model, effort, vendor, base and pane. `hw ledger` tells you which briefs are never-dispatched, in progress or done.
- **No brief, no launch.** `hw <lane> <task>` without a brief refuses and names the nearest briefs; `--no-brief` keeps the old launch with a loud warning. The task picker shows each brief's real title, and it got about 30 times faster.
- **`brain relaunch --all` and `brain <lane> --resume <id>`** relaunch idle brainers in place with the cockpit mod. Each keeps its conversation, pane and account; a working pane is skipped and named.
- **A cleaner cockpit:** one rounded card per executor, with a state badge, a context bar and aligned buttons.
- **One review per gentle task.** A `--sdd gentle` task with `review: rdd` runs native RDD only, without also running Judgment Day.
- **`hw done` and `hw next` live in `lib/hw/`.** Behaviour is unchanged, and `bin/hw` is about 1900 lines shorter.

## What changed

13 changes since 0.3.6.

### Added
- `brain relaunch [--all | <lane|pane>...] [--dry-run]` relaunches every open,
  idle Claude Code brainer in place with the cockpit mod, resuming its own
  conversation (same pane, same account, writer loop and work root restored).
  A working pane is skipped and named; below the cockpit floor it relaunches
  without the mod and says so. Executors are not relaunched.
- `brain <lane> --resume <session-id>` is the same launch for one lane.
- `hw ledger` lists each brief as never-dispatched, in-progress or done, from a
  durable ledger `hw` now writes at every real dispatch (brief and its sha,
  run, account, model, effort, vendor, base, pane). A dry run writes nothing.
- A dispatch commits its brief, and only that file, when the brain checkout is
  on main and the brief is untracked or modified.

### Changed
- The cockpit pane draws one rounded card per executor, with a coloured
  state badge, a context bar and aligned buttons, and a totals header of
  chips; a stale or dead state gets a boxed, coloured banner.
- `hw done` moved out of `bin/hw` into `lib/hw/done.sh`, the third module of
  the split of `bin/hw` by command. Behaviour is unchanged.
- `hw next` moved out of `bin/hw` into `lib/hw/next.sh`, the third module of
  the split of `bin/hw` by command. Behaviour is unchanged.
- `hw <lane> <task>` with no brief now refuses, naming the path it looked for and
  the nearest names. `--no-brief` keeps the old launch, with a one-line warning.
- The task picker shows each brief's title instead of the frontmatter's `---`,
  skips `_`-prefixed files, and reads every brief in one pass.

### Fixed
- `brain relaunch` keeps each pane's own account: the folder-trust and first-run
  checks read the pane's `CLAUDE_CONFIG_DIR`, not the caller's.
- `brain relaunch` finds a brainer whose directory is spelt through a symlink or
  another case, instead of skipping it.
- A `brain <lane> --resume` whose start failed says `FAILED resuming` and exits
  75 instead of printing `resumed` and exiting 0; `brain relaunch` reports such a
  pane as `FAILED` (not `skipped`) and exits non-zero.
- A `--sdd gentle` task with `review: rdd` is told native RDD is its only
  review: its prompt no longer also asks for Judgment Day, and carries a line
  saying not to run it. Without `review:`, Judgment Day is unchanged.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.6

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
