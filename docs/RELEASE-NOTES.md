# foreman-sh 0.7.2

foreman-sh 0.7.2 stops asking for the Bitwarden master password when bw fails for any other reason.

## In short

- **Only a real wrong password asks a person.** `secret-unlock --auto` treats only bw's own "Invalid master password." as a rejection. A broken node or bw, a migration error, a server or network error and a timeout are reported as what they are. They keep a live session and retry on the next run.
- **Alerts name the machine's fix.** The setup brainer is paged only when the session has under 60 minutes left, or none, and the page says what the machine needs (reinstall node or bw), not "type the password again". `secret-unlock --setup` keeps the stored password when bw could not judge it.
- **A bw that cannot start is visible.** `hw secret status` runs a no-vault `bw --version` and reports it.

## What changed

2 changes since 0.7.1.

### Fixed
- `secret-unlock --auto` no longer calls every `bw unlock` failure a rejected master password. Only bw's own
  "Invalid master password." is one; a broken node/bw, a migration error, a server or network error and a timeout
  are reported as what they are, keep a live session and retry on the next run, and page the setup brainer only when
  the session has under 60 minutes left (or none), naming the machine's fix instead of asking for the password.
  `secret-unlock --setup` keeps the stored password when bw could not judge it.
- `hw secret status` runs a no-vault `bw --version` and reports a bw that cannot start.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.7.1

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
