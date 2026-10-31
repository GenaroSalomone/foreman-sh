# foreman-sh 0.6.3

Executors that cannot hang the machine, cannot go quiet, and see their lane's specialists.

## In short

- **No wait without a bound, no frozen leftovers.** A background `while/until … sleep` loop needs a timeout, a counter or a deadline; a suite or heavy run ends its whole process group on exit; and the heavy slot no longer jams behind a zombie or a run that already reported.
- **A quiet executor speaks up.** An executor idle for 20 minutes after a legal turn end reports to its brainer once, and `hw status` shows it as `STALLED since HH:MM`; `hw done` reuses a green verdict of the same tree instead of re-running it.
- **Every lane sees its specialists.** `hw` links a repo-less lane's agents and skills into the executor's work directory, and a pi executor delegates to the lane's `.claude/agents` specialists as a Claude executor does.
- **One Bitwarden rule for every lane.** The shared rules carry a single table of what an executor and a brainer may run with `hw secret`, the brain guard allows what the rules prescribe, and a test drives every such command through the guard in every lane.
- **Reviews with one focus per plugin.** `thermo` judges maintainability, `ponytail` reviews like the senior who gets paged, `pstack` audits security and correctness and now also runs `blast-radius` and the `comment-sicko` agent; a piece two plugins share runs once, and `done-invoker` proves every one, agents included.
- **Fewer dead ends.** `hw revive` finds a repo lane's runs recorded in the work directory; the brain guard lets a listed program print its usage and, when it refuses, names the allowed form; and the README reads like a product, with a one-line install and indexes at the top of the long references.

## What changed

20 changes since 0.6.2.

### Added
- A claude executor can no longer write into a product repo it is not working in, whatever its lane's shape (a lane with
  no repo of its own included). The brain-guard plugin `hw` already passes to every claude executor now also runs
  `setup/guards/deny_executor_repo_writes.py`: every `product_repos` root of `guards.json` is refused, in Bash and in the
  Write/Edit tools, except the one that holds the executor's own `HW_WORKDIR`.
- An executor whose last turn ended legally (waiting on a background child, on a reply to its ask/challenge, or
  on a human-boundary) and that stays without a new turn for `HW_STALL_MIN` minutes (default 20; 0 turns it off)
  now reaches its brainer on its own: one `channel-send --report` to `HW_INVOKER_PANE` naming task, pane,
  minutes idle and what it waits on, and `hw status` shows the row as `STALLED since HH:MM`. The timer is
  `bin/stall-watch`, started by the turn-end hook in the shape of `setup/heavy`'s frozen-run watchdog (no
  daemon, no LaunchAgent); a new turn, the report or a gone pane ends it silently, and it alerts once per stall.
- An executor of a lane with no product repo now gets the lane's specialists: `hw` links
  `brain/<lane>/.claude/agents` and `skills` into its work directory at dispatch, so the prompt lists them
  (a lane's `settings.json` and `hooks` are never linked).

### Changed
- A `--agent pi` executor can now delegate to the lane's specialists. pi-subagents
  reads no `.claude/agents`, so `hw` names the worktree's directory and the pi
  adapter registers each file with it at session start, translating in memory
  Claude's tool names to pi's and leaving the file's `model:` to the executor's
  model; nothing is copied or rewritten. The preamble and the dispatch manifest
  say it, and `KNOWN-LIMITATIONS.md` L3b is narrowed to what still differs
  (tools pi has no counterpart for are dropped; specialists run in the foreground).
- The executor's fast gate now also runs the slow checks that only the full suite used to run: the mutation-anchor
  check when any source file changes, the Go-cache check when a subject that builds mutants changes, and the
  in-process-suite check when a `core/` file changes. A subject can declare `# gate-when-body: <regex>` to be
  selected by the text of a changed subject.
- The README leads with a one-line install and a short "Why" table, puts the
  install variants in one place, and drops the lane-settings example and the
  full command table in favour of links to the reference.
- `KNOWN-LIMITATIONS.md` opens with an index of every entry, `THREAT-MODEL.md`
  with an "In short", `REFERENCE.md` with a jump line, and `CONTRIBUTING.md`
  with the clone and the first gate run; the gate's timing knobs have their own
  section.
- The brain guard allows a listed program that carries `first=` with no
  arguments and with `help`, `-h` or `--help`, so an agent can read its usage;
  when it refuses the first argument, it names the allowed call
  (`call it as: bash <path> new|env|… (or --help)`).
- A plugin review has one focus per plugin and every piece runs once: `thermo` maintainability,
  `ponytail` senior review, `pstack` the diff's security and correctness, `blast-radius`, the
  `comment-sicko` agent and maintainability. A piece two plugins carry (`thermo` and `pstack` both
  carry the code-quality review) runs once, from the first plugin declared; the receipt, the closing
  and the `done-invoker` proof read the same list.
- `comment-sicko` is an agent: the closing tells the executor to launch it with the Agent tool, and
  `done-invoker` proves it from the transcript by its `subagent_type`, `pstack:comment-sicko`.
- A release and a push to `main` no longer wait for the Linux full suite (78-112 minutes in Docker on a
  4 GB VM, and the 0.6.3 cut died on its own 30-minute cap inside it). The release suite is the macOS fast gate
  (`HW_TEST_GATE=fast bash setup/test-hw`): the cut passes on its verdict for the candidate's exact tree
  (`<tree>.fast`) and, with none, runs the gate itself in the background and records it, and the push to `main`
  takes the same verdict. A Linux full verdict still opens both when one exists, but nothing requires it.
- The fast-gate verdict names the implementation it measured (`impl=go|bash`), as a full one does, and a verdict of
  one is never read for the other.
- Linux is best effort: it works, it is not verified per release. `setup/linux/run-suite.sh full` stays as a tool to
  run when the machine is idle; its reds become fixes in the next release.

### Fixed
- The executor sleep-poll guard no longer lets a `while/until … sleep` loop through in the background
  without a bound: wrapped in `timeout N`/`gtimeout N`, or carrying a counter or a `$SECONDS`/`date`
  deadline. The refusal names that shape. (A foreground refusal used to push an unbounded
  loop into the background, where it waited 17 h on a lock nobody held.)
- `setup/test-hw` runs each job in its own process group and, on any exit or signal, ends the whole job
  (TERM + CONT, then KILL), so a process frozen by the memory guard no longer outlives its suite.
  `setup/heavy` does the same on INT/TERM and when its command ends.
- `setup/heavy`'s one slot no longer jams behind a holder that will never finish. A zombie (a run whose
  parent is stopped: `kill -0` succeeds on it) counts as dead, for the slot holder and for queue entries.
  A holder or queue entry whose hw run has already reported (its run directory carries the `done` marker,
  not reopened, not blocked) is reclaimed and its process tree ended, with one line in `~/.cache/hw/heavy.log`
  (`reclaimed:<tag> run <dir> had reported (pid N)`). A frozen holder whose run has not reported keeps the slot.
- `hw done` no longer re-runs a verification that already ran green on the same tree. A pinned
  `setup/heavy -- <cmd>` leaves a verdict (command and tree) in the git common dir when `<cmd>` exits 0
  and leaves the working tree as it found it, and `hw done` reads it instead of waiting in the heavy
  queue for the 900 s ceiling and closing with exit 124.
- A pinned `HW_TEST_GATE=fast bash setup/test-hw --keep-going` now finds the fast-gate verdict of its tree;
  the flag used to make the pinned line differ from the cached one, so the gate ran again.
- `hw revive` finds a run recorded in the work directory (a `--worktree none` task, or one from before its
  lane had a repo): it lists the runs under the directory it already chose, instead of resolving the
  worktree a second time and refusing "no run directory".
- The release cut's fast gate runs at `nice -n 10`, no longer under `taskpolicy -b`. Background priority puts it on the
  efficiency cores, which cost about 3.5x the CPU time for the same work (subject 103: 3.2 s of CPU at `nice`, 10.8 s at
  `-b`), so seven subjects read over their time budgets with nothing slower in them. The Linux step keeps `taskpolicy -b`.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.6.2

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
