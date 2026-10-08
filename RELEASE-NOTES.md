# foreman-sh 0.3.11

A brainer resumed after a reboot comes back under its own account.

## In short

- **`brain <lane> --resume <id>` keeps the lane's account on a bare shell.** After a reboot the pane comes back without `CLAUDE_CONFIG_DIR`, and the resume used to look in the default account and fail with "No conversation found". It now exports the lane's verified account first.
- **An account it cannot verify is still refused**, before anything is exported or started.
- **`brain relaunch` and default-account lanes are unchanged.** A pane whose shell already carries the right account is left as it is.

## What changed

1 change since 0.3.10.

### Fixed
- `brain <lane> --resume <id>` on a pane left as a bare shell (after a reboot)
  starts the brainer under the lane's account; it used to look for the
  conversation in the default account and say "No conversation found".

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.10

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
