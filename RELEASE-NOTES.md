# foreman-sh 0.4.0-rc.3

`foreman-sh` tests faster: an executor's gate no longer waits behind a full suite, a push reruns only what its change can reach, and the slow contract subjects run in parallel shards.

## In short

- **Fast gates first.** A full suite yields its workers and the machine to a fast gate that is waiting, instead of holding a slot for hours.
- **A push stands on the last green suite.** It runs only the tests its change can reach, not the whole suite again.
- **The slow contract subjects run as parallel shards.** 843, 847, 860 and 870 are cut into pieces the runner schedules side by side, with the same cases.
- **A test world is cloned, not copied.** One clonefile(2) of the whole tree: 0.67 s where three copies took 31.7 s.
- **`brain relaunch` keeps background tasks alive.** It answers Claude Code's exit dialog with «Move to background and exit».

## What changed

12 changes since 0.4.0-rc.2.

### Added
- The suite's time budget is a gate: `setup/test-budget-gate` fails when a subject's budget passes
  600 s or the suite's estimate passes 45 minutes, and names the subject to split. A known debt carries
  a ceiling with the split that pays it; a ceiling only goes down.

### Changed
- `brain relaunch` no longer refuses a brainer that has background tasks: it answers Claude Code's exit dialog with
  «Move to background and exit» (selection read back before Enter), so the tasks keep running and the conversation
  resumes in the same pane. `--all` no longer skips such panes; a pane mid-turn is still skipped.
- A fast gate no longer waits behind a full suite: while a fast gate waits for a worker, a full run starts
  no new subject and gives the machine back; its timing subjects run alone only after the drain.
- `verify-for-push` stands on the last green verdict: a green incremental verdict is a base for the next
  push (at most 3 hops), so a push runs only the tests its change can reach.
- The four slow contract subjects (843, 847, 860, 870) and the anchors of 844 and 848 run as shards in
  parallel slots, with the same case lists as before.
- A contract case's world is one clonefile(2) of the whole tree instead of a copy per file (3 clones:
  0.67 s instead of 31.7 s).
- The golden regen scripts take `--grep`: only the cases naming that string are regenerated.
- The pre-commit hook runs the export check (about 2 s) before the gate, so an export leak fails before
  the minutes-long wait.

### Fixed
- A brainer no longer shows another lane's concurrent edits as its own Bash diff: Claude Code snapshots
  the whole git repo of the cwd around every Bash call, and every brainer shares one repo.
  `brain` and `hw` now launch `claude` with `CLAUDE_CODE_BASH_EDIT_DIFF=0`. A session already running keeps
  the diff until it is relaunched (KNOWN-LIMITATIONS L17).
- `done-invoker` no longer ends with "Exit code 137" after a delivered report. `hw done` closed the
  tab that held the invoker itself, which killed it; the close now runs after the invoker has
  exited, in a process of its own, so the invoker exits 0. A close that fails leaves `close-failed`
  and `close.log` beside the run's `done` marker.
- `done-invoker` and `ask-invoker` no longer print "token publication UNPROVEN" for any summary or
  question longer than one 80-character token: herdr deletes control characters and trims
  whitespace from every token value, and the invoker now sends values spelled that way, so the
  read-back proves them.
- The exit-status advice that `done-invoker`, `ask-invoker` and `channel-send` print when their output
  is piped no longer tells zsh users to read `${PIPESTATUS[0]}` (bash-only, empty under zsh); it
  offers `<verb> … ; echo rc=$?`, which reads the same in bash and zsh.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.4.0-rc.2

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
