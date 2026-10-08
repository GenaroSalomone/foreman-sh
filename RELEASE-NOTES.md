# foreman-sh 0.3.12

A new Mac reaches `foreman-sh init` by following INSTALL.md as written.

## In short

- **INSTALL.md starts with Homebrew.** Measured on a clean macOS 26.6 VM, the docs never said to install Homebrew first; INSTALL.md, the README quick start and the demo README now open with Homebrew's official installer.
- **Every documented `hw … --dry-run` passes as written.** Outside a `brain` pane the old line exited 1 with `HW_INVOKER_PANE UNRESOLVED`; the docs now carry `--no-report`, with one line on why.
- **Measured from zero:** on the clean VM, five commands and 44 s from the brew formula to a finished `foreman-sh init --yes`; a second `init` changes nothing. Homebrew itself came preinstalled in the image, so its install time is not in that number.

## What changed

2 changes since 0.3.11.

### Fixed
- INSTALL.md and the README's Quickstart now start, on macOS, with Homebrew's own
  installer (which also brings the Command Line Tools): a clean machine had no `brew`
  and the docs never said so.
- The documented `hw … --dry-run` (README, INSTALL.md, the demo) carries `--no-report`
  and says why: outside a herdr pane that `brain` opened it exited 1 with
  `HW_INVOKER_PANE UNRESOLVED`. `hw` itself is unchanged.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.11

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
