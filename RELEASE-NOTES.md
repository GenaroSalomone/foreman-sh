# foreman-sh 0.3.6

The brainer gets a cockpit inside Claude Code, and a revived run can take a new task officially.

## In short

- **The cockpit ships inside foreman-sh as a Claude Code mod.** `brain` loads it with `--plugin-dir` from the installed tree, so an upgrade updates it and nothing is written to `~/.claude`. It needs Claude Code 2.1.289 or newer: below that, `brain` prints one line and runs without it. `FOREMAN_COCKPIT=0` or `brain --no-cockpit` turns it off, the state writer included.
- **It shows a live panel of executors and acts through the verbs you already have.** The panel shows state, context used, queued rulings and time since the last report. Report cards have buttons for `hw done`, `hw ruling`, `hw receipt`, and a reply for a held ask or challenge. A rule band shows `hw preflight` warnings before a dispatch and never denies one.
- **The mod computes nothing.** It reads a state file that `hw cockpit-state` writes from herdr tokens and run files, refreshed on every event and every 5 s. Stale or missing data is shown as such.
- **`hw preflight --json -- <hw argv>`** reports a dispatch's rule warnings without side effects: an account override, or a cut in progress.
- **`hw next <pane> --run <id>`** gives a revived run that already reported its next task, with its own `done-invoker` and no `--retask`.
- **Pilot numbers:** `hw cockpit-state --pilot` prints the median and p90 time from a report to an action, and the reports left without one.

## What changed

14 changes since 0.3.5.

### Added
- The brainer cockpit's contract is frozen under `cockpit/`: the state file
  and `hw preflight --json` schemas, the allowlist of verbs a panel button may
  start, and one example fixture per state class. Nothing loads it yet.
- `hw cockpit-state` writes the file the brainer's cockpit panel reads: one
  JSON document per brainer with every executor's attention class (challenge,
  ask, blocked, report, working, idle), its queued rulings and which buttons are
  valid. It reads herdr's pane list and the run directories, never a pane
  capture, so it answers in well under 300 ms where `hw status` takes seconds
  per pane. A brainer started by `brain` keeps it fresh every 5 seconds and
  the invokers refresh it within a second of a report, an ask or a ruling.
- An idle executor holding an ask or a challenge gets a reply action: the
  state names the hold file (`pending_reply`) and the two reply verbs are part
  of the cockpit contract.
- A turn end publishes the executor's context use (`ctx_pct`, `ctx_tokens`)
  for claude executors, and an ask publishes whether it is an ask or a
  challenge (`ask_kind`).
- `hw preflight --json -- <hw argv>` prints what hw's own rules say about a
  dispatch, without doing it: a warning for an `--account` other than the
  lane's, and for a dispatch while a release cut holds a suite slot.
- The brainer cockpit mod (`cockpit/`): a read-only Claude Code mod that draws a
  band and a pane from the state file `hw` writes — the executors, their
  attention order, and a stale, dead or unreachable state said as such instead
  of old data. The installer mirrors it beside `bin/`. It starts no verb yet.
- The cockpit now has buttons: each starts one `hw` verb (`done` after a
  confirmation, `ruling` with its text on stdin, `receipt`, and `verify`, which
  only pre-fills the prompt). Which buttons exist is read from the state `hw`
  writes; hw's refusal is shown as it came and the card stays. A band shows
  `hw preflight`'s verdict before a dispatch runs, and never blocks it.
- `brain` opens a Claude Code brainer with the cockpit loaded (`--plugin-dir`)
  when claude is 2.1.289 or newer. Below that it prints one line and opens
  without it. `FOREMAN_COCKPIT=0` or `brain <lane> --no-cockpit` opts out.
- The cockpit panel's reply box: an idle executor holding an ask or a challenge
  gets an input that sends the typed answer to it. The text travels on stdin,
  never in a command line.
- `hw next <pane> --run <id>` gives a run that `hw revive` reopened its next task: the counter
  advances and the new task has its own `done-invoker`, with no `--retask`. A reported run that
  was not revived is refused, naming `hw revive`.

### Changed
- The refusals of `hw done`, `hw ruling` and `hw receipt` live in one module,
  `bin/hw-actions`, that the verbs and the state writer both call, so a panel
  button is disabled exactly where the verb would refuse, with the verb's own
  words. The refusal texts are unchanged.
- `cockpit/schema.json` states that `herdr.server_started_ms` is always null
  and that the `gone` attention class is reserved, in this version.

### Fixed
- A cockpit refresh that names a brainer it cannot resolve now writes nothing
  and leaves a line in `.cockpit/skipped-kicks.log`; before, it wrote a state
  file for the pane it was running in.
- `hw next --run <id>` no longer refuses a revived run that re-reported while its chaining lease is
  live: it refuses only a reported run with no reopened marker and no live lease.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.5

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
