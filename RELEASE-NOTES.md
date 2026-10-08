# foreman-sh 0.3.10

`hw status` answers in about a second.

## In short

- **`hw status` is about 17 times faster.** With 15 live panes it takes about 1.2 s instead of 20 s, and `hw status <lane>` takes 0.9 s instead of 3.6 s. The output is byte for byte what it was.
- **The cause was one scan, not the language.** An outbox search walked every task tree. It now stops at the depth where reports live, and the closing sections run in parallel and print in order.
- **A receipt line that is a JSON list** no longer crashes `hw status`.

## What changed

2 changes since 0.3.9.

### Changed
- `hw status` runs in about 1 second instead of about 20 with 15 live panes, and `hw status
  <lane>` in under 1 second. The outbox scan no longer walks every task tree, and the closing
  sections run in parallel but print in the same order. The output is byte-identical, and
  `HW_STATUS_SERIAL=1` runs them one after another.

### Fixed
- `hw status` no longer crashes on a receipt line that is a JSON list.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.9

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
