# foreman-sh 0.3.0-rc.2

A release candidate for 0.3.0. Report anything that behaves differently
from 0.3.0-rc.1.

### Added
- `hw status` audits the always-loaded auto-memory indexes in one line: an
  index over its byte budget, a retired entry still listed, a `[[slug]]` in
  `decisions.md` that resolves nowhere. A clean store prints nothing.

### Changed
- `done-invoker --blocked` no longer closes the executor. The report is
  delivered as before, and the pane and its session stay open, waiting.
  `hw status` shows the task as `blocked-waiting`. A run launched with
  `--keep-pane` is kept by its chaining lease instead and is not marked
  waiting: its message says so, and names `hw next` and `hw done --blocked`.
- `hw ruling <pane>` on a task that reported `--blocked` resumes that task in
  the same session: the ruling is sent at once and the executor reports again
  with its own `done-invoker`, without `--retask`. A ruling to a task that
  reported done is still refused.
- `hw reap --apply` closes a blocked executor nobody answered within
  `HW_BLOCKED_WAIT_HOURS` (default 24) with `hw done --blocked`, in every
  lane's worktree root, including the one `hw sweep` leaves out, and so does
  `hw done --blocked` by hand: the turn that sent the blocked report is not
  counted as a turn after it. A turn that ends after that one still makes
  `hw done` refuse the pane. `hw sweep` keeps a waiting executor until its
  wait ends. A success report and an explicit `hw done` close as before.
- Under `--sdd gentle` the ask cap is 4 instead of 3, so approving the phase-1
  proposal no longer costs one of the three asks; other modes stay at 3.
- Claude executors now start with Claude Code's auto-memory off
  (`CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`): a brainer's saved preferences no
  longer load into every executor; the brief carries what the task needs.
- `setup/release/notes VERSION` over a version `CHANGELOG.md` already has adds
  the new fragments to that section, each entry under its type (a missing type
  inserted in Keep a Changelog order), in `CHANGELOG.md` and in
  `RELEASE-NOTES.md`, instead of refusing and leaving the merge to be done by
  hand.

### Fixed
- `hw status` showed a delivered `--blocked` report as `reported-done`.
- An OpenCode prompt that ended in two `session.idle` events counted as two
  turns after a report, and `hw done` refused the pane for the second. The
  OpenCode adapter now tells `hw` whether the pane was given a message since
  its last idle, and a repeated idle counts nothing.
- `engram-label-proxy` sends the pane's label as `expected_project` on
  `mem_update`, which engram 3.0.0 requires, and refuses one naming another label.
- The brain guard refuses `H=$(command -v hw); $H …` with a message that says to
  call `hw` by its name or by its path, not through a variable.
- The full suite runs green again on Linux (container) and on WSL2: tests and
  harness pieces that assumed macOS were fixed (the suite image builds without
  Docker Desktop's credential store and carries zsh and herdr, a heartbeat age
  is read with GNU `stat` first, the per-test ceiling cuts on native Windows).
  Native Git Bash still fails four subjects in a full run (KNOWN-LIMITATIONS,
  L1b).

## Upgrading from 0.3.0-rc.1

Update your checkout and run `install.sh` again with the arguments you
installed with.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
