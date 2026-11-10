# foreman-sh 0.8.0-rc.2

foreman-sh 0.8.0-rc.2 makes the herdr desktop app a complete home for foreman: the whole cycle — open a brainer, dispatch, ruling, ask, report, done, next, outbox, wait, revive — ran live in an isolated app with real Claude, and every gap those runs found is closed.

## In short

- **Reports survive a closed brainer.** A report sent while the brainer is closed, done or blocked, is queued and delivered as it was sent when the brainer reopens; the executor follows the reopened brainer's new address, and the cockpit shows it delivered.
- **`hw revive`, `hw wait` and the cockpit work on the app.** An app run can be revived, a non-blocking wait cleans up its monitor, and the cockpit keeps one writer per brainer and sweeps dead ones.
- **The app stays in its own lane.** Brainers and executors started on the app drop an inherited `CLAUDE_CONFIG_DIR` and run identity, the start recipes use a clean environment, and an `hw` verb outside an app refuses instead of reaching a herdr server nobody named.
- **An executor that asks ends its turn.** The dispatch prompt and `ask-invoker` say the reply arrives as the next prompt, so no executor waits on a guard that refuses the wait.

## What changed

33 changes since 0.8.0-rc.1.

### Added
- `hw wait`, `hw unstick`, `brain <lane> --reset`, `--resume <id>` and `brain relaunch` work on an app terminal
  (herdr-core), through `hc`.
- `release: prod` briefs: hw diffs the staging and prod env at dispatch and at close (app env names plus container booleans),
  refuses an unruled or stale `env-diff:` ruling, requires `screens:` PNGs and a `qa:` run reported before the close, and
  refuses a rehearsal on an env that differs from the release target.

### Changed
- L29 now says what the fifth live-cycle QA measured (real claude, isolated app), and the README says the Agents list's space-name rows are the app's own mod.

### Fixed
- `hw wait`, `hw wait-one` and `hw outbox flush --to` refuse an app terminal address (`t3`, `machine/t3`) named from a
  process that is not an app terminal, and `hw revive` refuses an app run from one, instead of asking the default
  herdr or core (the person's own). One gate in the dispatch, so a new verb that names an address is covered by listing it.
- `brain <lane>` on the herdr app opens on the first try: a failed `agent.focus` is retried
  and the outbox is flushed whatever focus does.
- A report sent while the brainer is closed stays a done: `done-invoker` queues it in the
  outbox (exit 0, `queued` marker) and `--blocked` can no longer replace it.
- An executor's invoker address follows a brainer reopened as another terminal.
- `brain <lane>` no longer flushes another lane's, another run's or another herdr-core's queued report into the new brainer (a stale
  `probe:probe` entry once reached a setup brainer, which acted on it); the skipped entries are named and left where they are.
- A report delivered from the outbox now closes its executor (`hw done`, unless the run was launched with `--keep-pane` or the report
  is blocked) and leaves the transcript's REPORT line, as a direct report does.
- An executor dispatched on the herdr app records its `session_resume` (read from claude's own `sessions/<pid>.json`), so `hw revive`
  can resume it; the cheap explorer is armed through `--agents` in the terminal's argv, and the receipt reads `agent_ready: yes` once
  the brief's turn is proved.
- The cockpit's state file for an app brainer is `<core id>.<address>.json`, and a run records the core that dispatched it
  (`hc-core`): two cores no longer share `t2.json`, and a reopened brainer no longer re-points another core's runs.
- `hw revive` resumes a run of the herdr app: it starts the recorded cwd, env and resume as an `hc run` terminal (refusing a task that
  still has a live terminal), proves it with `hc get`, and reopens the run as on herdr.
- The cockpit plugin inside an app brainer reads the per-core state file (`HW_COCKPIT_CORE` in the brainer's environment), and
  `brain <lane>` no longer waits on the `hw done` of a flushed report longer than `HW_FLUSH_CLOSE_WAIT_MS` (5 s); a close still running
  goes on in its own process, with its output in `close.log`.
- `herdr-app/README.md` documents an isolated live QA (own brain copy, work root, config dir and MCP config).
- The cockpit and `hw status` list the executors that run in the herdr app again (hc 0.1.19 refuses
  `hc ls --label hw_task`; the label is now filtered on `hc get`), `hw` and `brain` resolve inside the app's
  brainer and executor terminals, and `hw done` removes the worktree of an app run when no herdr server runs.
- A `--blocked` report sent while the brainer is closed is queued like a done (exit 0, "QUEUED"), counts as a turn-end disk fact,
  and the reopen's outbox flush files it as blocked (`blocked-waiting`), so `hw ruling` resumes it instead of being refused.
- A brainer on the herdr app gets a cockpit writer for as long as its terminal lives, and a lane's space takes the lane's name
  instead of a terminal's.
- `hw done` of a `--keep-pane` run cancels its chaining-lease timer.
- A report the outbox flush delivers to an app brainer moves the executor terminal's labels to `hw_done_state=delivered` and
  `hw_invoker=<the new brainer>`; the cockpit no longer says "report undelivered" to a dead brainer for ever.
- `brain <lane>` on the default account clears an inherited `CLAUDE_CONFIG_DIR` for the brainer it starts in an app terminal, as `hw` does for
  executors, so the folder-trust and first-run checks read the account the brainer really runs under.
- A brainer started in an app terminal starts without the run identity (`HW_RUN`, `HW_TASK`, `HW_PROJECT`, `HW_WORKDIR`, …) an app launched from
  an executor's shell hands every terminal. The README recipes start the app with `env -i`, and L29's workaround names `HW_*`.
- A non-blocking `hw wait` on the app leaves no finished monitor terminal: it removes itself `HW_WAIT_MONITOR_KEEP_S` seconds (default 300)
  after its last verdict.
- The brainer's cockpit writer removes the state files of writers that did not leave (older than an hour, loop lock free).
- The cockpit panel's "no state folder" says the app reads files only under its own home; the README's isolated-QA recipe puts the work root there.
- `hw ruling`, `hw next` and `hw unstick` refuse an app terminal address (`t3`, no colon) named from a shell with no `HC_TERMINAL_ID`, naming the missing env, instead of asking the default herdr server about it.
- `ask-invoker` tells an executor to end its turn after a delivered ask (the reply arrives as its next prompt; the sleep-poll guard refuses a wait), and the shared rules say the same.
- `cockpit-state --kick-run` for an hc brainer that has left removes its state file instead of redrawing "report undelivered".
- The herdr-app README `env -i` recipes carry `USER`, `LOGNAME` and the real `~/.local/bin`; the layouts name the cockpit folder from `~`; the panel's "no state folder" line wraps instead of being clipped.
- An executor resumed after its brainer was reopened on the app can report again: the run's env follows the new brainer
  address, and `brain` re-points the lane's runs whose invoker terminal is gone.
- `hw done --blocked` is no longer refused for the turn that queued the report itself.
- `channel-send --reply-hold` waits up to `HW_HC_PROOF_MS` for the ask's delivery proof instead of refusing a reply sent too early.
- The cockpit raises no herdr alarm on an hc-only brainer, a writer removes its file when it leaves, and the plugin finds its
  state by `HC_TERMINAL_ID`.
- `hw done` cancels every chaining-lease timer of the run, not only the newest.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.8.0-rc.1

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
