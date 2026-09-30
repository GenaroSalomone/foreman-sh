# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) with pre-release labels.

## [0.1.1] — 2026-09-30

### Fixed
- The repository guards (Python, JavaScript and Codex) unwrap nested `env`
  wrappers on a fixed work budget and refuse the command when it runs out,
  instead of taking time that grew with the cube of the nesting. A hook that
  timed out did not block.

## [0.1.0] — 2026-09-30

First stable release.

### Added
- `install.sh --check` names the answer to engram's allowlist question and
  lists the `PATH` export as an ordered step.
- One full test suite runs per machine at a time; a second one waits in a
  queue. The pre-commit hook runs only the subjects a commit touches
  (`setup/gate-select`).

### Changed
- README rewritten: requirements before the quickstart, a table of concepts,
  every command checked against `hw help all`, and "How it compares" dated
  and sourced to each project's own documentation. The "When not to use
  foreman" section is gone.
- `BRIEF-TEMPLATE.md` no longer carries a section written for one of the
  maintainer's own projects, and its closing step works without engram.
- `brain` waits for engram's server before registering a session, and the
  server starts without printing an error.
- An executor without Judgment Day reports it as "not installed (optional)",
  not as escalated.
- CI scales time budgets by 3 on a shared runner.

### Fixed
- The repository guards unwrap `env` and its options (`-u`, `-i`, `--chdir`,
  `-S`) before judging a command, and refuse a quoting trick that hid a write
  from the Python and JavaScript guards.
- Test 189 binds its stand-in without a reverse DNS lookup, which failed on
  the macOS CI runner.
- Test 54's mock no longer runs out a 2–3 second deadline under load.

## [0.1.0-rc.4] — 2026-09-30

Fourth pre-release, and the last candidate before 0.1.0. A report that did
not arrive is kept and redelivered instead of lost, a stuck or dead task shows
in `hw status`, an executor can run inside a kernel sandbox on macOS, and an
executor no longer holds the brainer's commands.

### Added
- `hw <lane> <task> --sandbox`, or `sandbox: true` in a brief: on macOS, a
  Claude Code executor runs inside a Seatbelt profile generated for that
  dispatch. It cannot write outside its work directory, `$HW_ARTIFACTS`, the
  temporary directories, its transcript and the engram store; every outbound
  unix socket but the system resolver's is cut (herdr, Docker, the
  ssh-agent); `~/.ssh`, `~/.config/gh` and `~/.docker` are unreadable.
  `done-invoker` and `ask-invoker` still work, through the run's outbox and a
  broker outside the sandbox. Opt-in; refused off macOS and for OpenCode and
  Codex; TCP stays open (`THREAT-MODEL.md`, `KNOWN-LIMITATIONS.md` L11).
- An outbox for reports. `done-invoker` keeps a copy of each report until the
  receiver admits it; `hw outbox` lists what never arrived, `hw outbox flush
  --to <pane>` redelivers it, `brain <lane>` redelivers on open, and `hw
  status` names what is waiting. A report whose delivery is uncertain is never
  resent on its own.
- `hw status` marks a working task that shows no sign of activity for
  `HW_STALE_MINUTES` (30) as `STALE`, naming what to look at, and a run that
  never reached its first turn as `DIED-BEFORE-FIRST-TURN` instead of hiding
  it.
- `hw log <lane> <task>`: the asks, challenges, rulings, answers and reports
  of one task, from a per-task `transcript.log`.
- Continuous integration: the fast gate on Linux and macOS, and on Windows
  under Git Bash without blocking, for every push to `main` and every pull
  request. `CONTRIBUTING.md`, `SECURITY.md`, issue templates (bug, feature,
  guard vector) and a pull-request template.
- README: "How it compares", every cell sourced, and "When not to use
  foreman".

### Changed
- `done-invoker` and `ask-invoker` exit 5 when delivery is uncertain (the
  send was cut off with the message in flight), with a message not to retry,
  instead of a generic 1. On herdr the receipt says the report was
  *admitted*; only the native routes say its delivery was proved.
- `hw done` refuses to close a task whose report never reached the brainer
  (`STRANDED`) and names how to redeliver it; `--force` discards it
  deliberately.
- An executor has an executor's commands only. Run from an executor, `hw`
  refuses `next`, `ruling`, `unstick`, `reap --apply`, `sweep --apply`,
  `preview`, `revive`, a dispatch and `done` on another task; the reverse
  guard lets an executor run `hw` only with its own verbs, and `channel-send`
  only toward its own brainer.
- The README and `KNOWN-LIMITATIONS.md` say exactly when engram and Judgment
  Day become mandatory (a task `hw` registered an engram session for; a brief
  that declares `boundary:`), and that the installer does not register the
  Codex guard.
- `THREAT-MODEL.md` covers injection: a product repository's own agent
  configuration, and a report's text arriving in the brainer's session.
- `hw help` lists `hw outbox`, `--sandbox` and the brief keys `sandbox:`,
  `requires-agents:` and `boundary:`.
- `install.sh --check` on Linux requires `sd`, `fd`, `rg` and Node 22.7 or
  newer, as the requirements always said, and names each one missing.
- The example `projects.json` and `guards.json`, the README and `INSTALL.md`
  put task worktrees in `~/work`, beside the brain, where the installer puts
  them.
- `install.sh`, run again over an existing brain, adds the executor's
  `bin/hw` line to `setup/brain-guard-programs.txt` when it has none, and
  keeps every other line (`RELEASE-NOTES.md`, "Upgrading").

### Fixed
- The brainer's guard let through a `find` whose write action (`-delete`,
  `-exec`) came after a backslash-newline, read the quoted words of an `eval`
  as data rather than code, and let a quote inside a comment pair with one on
  the next line and hide the command between them. All three are refused, in
  the Python, JavaScript and Codex guards alike.
- The brainer's guard reads `env --unset`/`-u`, `-i`, `--chdir` and
  `-S`/`--split-string` as `env` does, so the command behind them is judged.
- The Codex hook exits 2, a refusal, when it crashes while deciding; it
  exited 1, which Codex treats as a non-blocking error.
- The OpenCode plugins refuse a tool call whose arguments are not an object.

## [0.1.0-rc.3] — 2026-09-30

Third pre-release. It fixes an installer layout in which no product executor
could run, and makes the brainer's guard read the shell.

### Fixed
- The installer put task worktrees under `<brain>/work`, inside the brain,
  where the reverse guard refused every command a product executor ran in its
  worktree. `work` now goes beside the brain; `install.sh`, `install.sh
  --check` and `hw` refuse a guarded lane whose work directory is inside the
  brain and print how to move it. An existing installation also has to run
  `install.sh --lane` again for each lane, so the guards protect the new
  work directory (`RELEASE-NOTES.md`, "Upgrading").
- The brainer's guard (Claude Code, OpenCode, Codex) let through a relative
  path (`rm -rf ../myapp/src`, `git -C ../myapp …`), a redirect after
  `cd <repo> &&`, a shell-local variable in a write target, globs, brace
  expansions, `find -delete`/`-exec` and `fd -x`. It now segments each command,
  tracks the directory each one runs in, expands its variables and braces,
  reads literal `sh -c` and `eval` bodies, and treats `find`/`fd` actions as
  writes. Same verdicts in all three guards.
- In the Python guards (the Claude Code hooks and the Codex hook), a payload
  that is not an object, or a shell call whose input is not an object, crashed
  the hook (a non-blocking error to the agent); it is now refused. Claude Code's reverse guard exited 127 without `python3`, which is
  not a refusal; it now refuses.
- `hw` without a terminal waited on an interactive picker; it now prints its
  usage and exits 2.

### Added
- `THREAT-MODEL.md`: what the guards stop, what they do not, and what the
  harness assumes.
- `install.sh --with-judgment-day` installs Judgment Day into Claude Code's
  configuration; idempotent, and it refuses rather than overwrites a file it
  did not write.
- `hw help <topic>` (`dispatch`, `flags`, `commands`, `exit-codes`,
  `recovery`, `advanced`, `all`), and `--version` on `hw`, `brain` and
  `install.sh`.

### Changed
- `install.sh --check` checks everything in one pass, lists the fixes in the
  order they must be done, and ends with one `Next step:`.
- `hw --help` is one page on standard output; `brain --help` works.
- Color is used only when output is a terminal, and never with `NO_COLOR`.
- The quickstart in `README.md`, `INSTALL.md` and `examples/demo/` runs as
  written.

## [0.1.0-rc.2] — 2026-09-29

Second pre-release. It closes a write-guard gap found after rc.1 and ships
Judgment Day.

### Fixed
- Write guards on macOS: a path that spells a protected repository with
  different letter case (`~/Code/MyApp` for `~/code/myapp`) reached the
  repository, because APFS folds case and the guard compared text. The Python
  guard (Claude Code, Codex) and the JavaScript guard (OpenCode) now compare a
  path's filesystem identity, device and inode, against each protected root.
- The OpenCode guard follows a symlink whose target does not exist yet, with
  the Python guard's verdict; before, a write through such a link into a
  protected repository was allowed. Measured on macOS, on Linux (Ubuntu 24.04
  under WSL2) and on Windows under Git Bash, before the case-variant change
  above; the guards as shipped are measured on macOS only (see
  `KNOWN-LIMITATIONS.md`, L1b).

### Added
- Judgment Day (`_skills/judgment-day/`, agents in `_agents/`): a blind review
  by two judges before a diff counts as finished. Not installed by default;
  activation is in `INSTALL.md`.
- A Codex executor's brief states the executor rules Codex cannot load from a
  file (secrets, credential handles, closing the browser, where artifacts go).

### Changed
- Every release candidate is run once end to end before it is published: a
  real herdr server, a real Claude Code executor and a real OpenCode executor
  asking, challenging and reporting through the return channel.

## [0.1.0-rc.1] — 2026-09-29

First public pre-release. It is a snapshot of a harness that has been used
day to day on real repositories; the history that produced it is not part of
this repository.

### Added
- `hw`: one command that gives each task its own git worktree, its own terminal
  tab and its own agent session, from a brief in Markdown. `--dry-run` prints
  the whole dispatch, each field marked chosen or defaulted.
- `brain`: opens the long-lived planning session for a lane.
- Return channel: `done-invoker`, `ask-invoker` and `channel-send`. An executor
  reports, asks or is redirected without anyone polling its pane.
- Write guards for Claude Code, OpenCode and Codex that refuse a brainer's
  writes into its lane's repository, plus a reverse guard that refuses a
  product-lane executor's writes into the brain.
- `install.sh`: builds a brain of your own, one lane per repository, links the
  commands, merges one Stop hook, and refuses rather than overwrites when
  configuration collides. Idempotent. `--check` writes nothing.
- Lanes are configuration (`projects.json`, `guards.json`, per-lane build
  script), not code paths in `bin/`.
- A hermetic test suite (`setup/test-hw`, `setup/test-channel-send`) with a fast
  gate, runtime budgets and mutation coverage.
- `examples/demo/`: a lane to try the install on a throwaway repository.
- MIT license.

### Known limitations
See [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md). The ones that decide whether
this fits you: herdr only; Linux and native Windows (Git Bash) are measured
by the suite but not yet on a live herdr and agent, and WSL2 is the proven way
on Windows; Codex is not set up by the installer and
its write tool is unguarded; OpenCode background subagents are experimental.
