# foreman-sh 0.4.0-rc.2

`foreman-sh` starts on a bare Mac without the Command Line Tools, restarts a brainer without losing what you were typing, and the commit gate stops running into its own checkouts.

## In short

- **A Mac without the Command Line Tools installs.** The `git` and `python3` shims that exit 1 count as missing, `--check` says so with `xcode-select --install`, and `init` no longer dies on an empty `--brain`.
- **`init` tells you when `hw` runs in bash.** With no Go (or one older than 1.22) it passes on the installer's one-line note instead of finishing silently.
- **`brain relaunch` keeps your unsent draft.** It no longer types `/exit` over it, and a session that never produced a transcript comes back as a new conversation instead of a failed resume.
- **The test selector is 150 times faster beside agent worktrees.** `setup/gate-select` skips nested checkouts: 243 s down to 1.65 s on a tree with five of them, same selection.
- **The commit hook says the gate takes minutes.** One line before the wait, so an agent runs the commit in the background instead of dying at its tool timeout.

## What changed

7 changes since 0.4.0-rc.1.

### Changed
- The pre-commit hook says once, before it runs, that a commit with no verdict for its tree runs
  the gate now and takes minutes, and that an agent should run it in the background.

### Fixed
- On a Mac without the Command Line Tools, `/usr/bin/git` and `/usr/bin/python3` are shims that
  exit 1; `--check` printed `ok git` and `ok python3` for them and `init` died on an empty
  `--brain`. A `git` or `python3` that does not run is now MISSING, with `xcode-select --install`.
- `foreman-sh init` passes on install.sh's `NOTE` line when `hw-core` is not built (no Go, Go older
  than 1.22): it finished silently, so the person could not tell `hw` was running in bash.
- `brain relaunch` no longer types `/exit` over a draft you had not sent in the brainer's input
  box: the draft was submitted as a message and the relaunch refused with "NOT proved agent-free".
  A session with no transcript is relaunched as a new conversation instead of a resume that cannot
  work.
- `setup/gate-select` does not walk checkouts nested in the tree it selects for. With agent
  worktrees under `.claude/worktrees` the selection took 243 s; it takes 1.65 s and picks the same
  83 tests on a normal tree.
- A contract subject's fan-out fits the share of the machine its slot holds, instead of 18 workers
  on 14 CPUs when five subjects ran in six slots.
- The release-copy tests copy the tracked tree, not whatever the checkout carries untracked.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.4.0-rc.1

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
