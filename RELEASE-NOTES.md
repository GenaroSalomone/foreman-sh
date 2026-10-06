# foreman-sh 0.3.3

Fewer false reds in the release cut, web-lane executors test their own build, and idle worktrees give their disk back.

## In short

- **The cut retries a flaky e2e once.** Gate 10 reruns a red end-to-end run a single time and names both logs, and the e2e run takes a suite slot so it does not race other suites.
- **Web-lane QA points at the executor's own port.** The executor's prompt names its QA target (`http://localhost:$HW_PORT_WEB`), also on `hw next`, and a stale port inherited by `--here` is cleared at lane entry.
- **Idle worktrees free their build output.** `hw reap` removes `.next`/`.turbo` from kept worktrees idle past `retention.build_output_days` (default 7), logging every removal.
- **Databases drop reliably.** Reap dumps with the `pg_dump` matching the server's major version and names the connections that block a drop.
- **The setup lane runs on the personal account,** as declared in `projects.json`.

## What changed

5 changes since 0.3.2.

### Added
- An executor's prompt on the web lane now names its own QA target
  (`http://localhost:<HW_PORT_WEB>`), also after `hw next`, so QA no longer
  falls back to staging or to another worktree's server on :3001.
- `hw reap` frees `.next`/`.turbo` of kept worktrees idle past
  `retention.build_output_days`; it logs what it removes before removing it.

### Changed
- The release cut retries the e2e gate once and names both logs when it
  stays red.

### Fixed
- `hw reap` dumps a database with the `pg_dump` of the server's major
  version, names the open connections that block a drop, and counts a
  database that is already gone as dropped.
- A `HW_PORT_WEB` inherited from the caller's environment no longer leaks
  into a `--here` launch.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.2

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
