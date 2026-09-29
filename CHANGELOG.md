# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) with pre-release labels.

## [0.1.0-rc.1] — unreleased

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
- MIT licence.

### Known limitations
See [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md). The ones that decide whether
this fits you: herdr only; Linux and native Windows (Git Bash) are measured
by the suite but not yet on a live herdr and agent, and WSL2 is the proven way
on Windows; Codex is not set up by the installer and
its write tool is unguarded; OpenCode background sub-agents are experimental.
