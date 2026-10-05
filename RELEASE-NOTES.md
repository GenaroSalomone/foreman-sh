# foreman-sh 0.3.0

The stable release of 0.3.0-rc.1 and 0.3.0-rc.2; their changes are in
[`CHANGELOG.md`](CHANGELOG.md). Below, everything since 0.3.0-rc.2.

### Added
- A brainer can now ask about an order that reads two ways: a turn ending in
  `Ambiguous: «<the operator's words>» — <reading A> / <reading B>` (or
  `Ambiguo:`) is no longer refused as a handback by `bin/hw-stop-hook.sh`.
  The line must quote words the operator actually wrote and name two
  readings, so a decision handed back as a question is still refused.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.0-rc.2

Update your checkout and run `install.sh` again with the arguments you
installed with.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
