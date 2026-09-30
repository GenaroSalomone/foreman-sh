# foreman-sh 0.1.0-rc.4

**The last release candidate before 0.1.0.** It is meant for early feedback,
and it ships with a register of what it does not do: read
[`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md) before installing. It is not a
stable release; what changes before 0.1.0 is what this candidate's feedback
finds.

## What changed since 0.1.0-rc.3

- **A report that did not arrive is kept, and said so.** `done-invoker` keeps
  each report in the run's outbox until the brainer admits it. `hw outbox`
  lists what never arrived, `hw outbox flush --to <pane>` redelivers it, and
  `brain <lane>` redelivers it on open. `hw done` refuses to close a task
  whose report is stranded. A send cut off mid-flight exits 5, "uncertain,
  do not retry", instead of a generic failure, and on herdr the receipt says
  *admitted*, not delivered.
- **A stuck or dead task is visible.** `hw status` marks a working task with
  no sign of activity for 30 minutes (`HW_STALE_MINUTES`) as `STALE`, and a
  run that never reached its first turn as `DIED-BEFORE-FIRST-TURN`.
  `hw log <lane> <task>` shows a task's asks, rulings, answers and reports.
- **An executor can run inside a sandbox, on macOS.** `hw <lane> <task>
  --sandbox` starts a Claude Code executor in a Seatbelt profile: it cannot
  write outside its work directory and a few named places, cannot reach
  herdr, Docker or the ssh-agent, and cannot read `~/.ssh`, `~/.config/gh` or
  `~/.docker`; it still reports and asks through the outbox. Opt-in, macOS
  and Claude Code only, and the network stays open
  ([`THREAT-MODEL.md`](THREAT-MODEL.md)).
- **An executor no longer holds the brainer's commands.** Run from an
  executor, `hw` refuses to re-task, rule, restart, reap, close another task
  or dispatch; the reverse guard lets it run only its own `hw` verbs and
  `channel-send` only toward its own brainer.
- **The guards close more spellings.** The brainer's guard reads a `find`
  whose `-delete` or `-exec` sits after a line continuation, an `eval`'s quoted
  words, a quote inside a comment, and `env --unset`, `-i`, `--chdir` and
  `-S`. The Codex hook refuses when it crashes, and the OpenCode plugins
  refuse arguments that are not an object.
- **The docs say where they stop.** When engram and Judgment Day become
  mandatory, that the installer does not register the Codex guard, how
  injected content reaches an agent, and how foreman compares with similar
  tools and when not to use it.
- **CI and a way to contribute.** The fast gate runs on Linux, macOS and
  Windows (Git Bash) for every push and pull request; `CONTRIBUTING.md`,
  `SECURITY.md` and issue and pull-request templates are in the repository.

## Upgrading from 0.1.0-rc.3

Update your checkout and run `install.sh` again with the arguments you
installed with. It refreshes `bin/` and the guards in your brain. Your
`setup/brain-guard-programs.txt` is kept; if it has no `bin/hw` line, the
installer adds one, because the guard now allows an executor's `hw` only
through that line. Nothing else needs to change.

The full list is in [`CHANGELOG.md`](CHANGELOG.md).

## What it is

A shell harness, built on [herdr](https://herdr.dev), for running coding agents
in two roles over your own repositories. Every brainer and executor is a herdr
pane.

- A **brainer** is a long-lived agent session per repository (a *lane*). It
  reads, plans and writes briefs. A guard stops it writing into the repository.
- An **executor** is a disposable session launched for one task, in its own git
  worktree and terminal tab. It works from the brief, commits on its own branch
  and ends by reporting to its brainer with `done-invoker`.

The two talk through a return channel. An executor that needs a decision asks
(`ask-invoker`); its final report arrives in the brainer's session without
polling. What a lane is (repository, base branch, worktree layout, default
agent) is configuration in `projects.json`; what the guards protect is
configuration in `guards.json`.

## What you can do with this release

1. Install a brain and a first lane with one command, on macOS, Linux or
   Windows (WSL2, or Git Bash natively):
   `./install.sh --brain ~/brain --lane myapp --repo ~/code/myapp`.
2. Open the brainer (`brain myapp`), write a brief, and inspect the whole
   dispatch before it runs with `hw myapp task --brief … --sdd none --dry-run`.
3. Run executors on Claude Code or OpenCode, each isolated in a worktree, and
   receive their reports in the brainer.
4. Rely on the guards to refuse a brainer's writes into your repository, and a
   product executor's writes into the brain.
5. On macOS, run a Claude Code executor inside a sandbox that confines its
   writes and control sockets: `hw myapp task … --sandbox` (opt-in; the
   network stays open).

Your repository is never written by the installer, and no credential is read.

## Try it

`examples/demo/README.md` walks the install and a dispatch on a throwaway
repository. Verify a checkout with:

```sh
HW_TEST_GATE=fast bash setup/test-hw   # fast gate
bash setup/test-hw                     # full suite
bash setup/test-channel-send
```

## Requirements

macOS, Linux, or Windows through WSL2 or Git Bash; `git`, `jq` 1.7+, `python3`; [herdr](https://herdr.dev) with its Claude
integration; [Claude Code](https://claude.com/claude-code) with its first run
finished; on Linux also `sd`, `fd`, `rg` and Node 22.7+; optionally `engram`
(memory), `fzf`, and `rg` on macOS. OpenCode 1.18.31+ is supported as an
executor agent. See `INSTALL.md`.

## What to know before you rely on it

- macOS is where it is developed. Linux is tested in a container without a
  live herdr. Windows runs under WSL2 as Linux, and natively under Git Bash
  measured on a CI runner only, not yet on a real machine. herdr only.
- Codex can be dispatched with `--sdd none` only, is not set up by the
  installer, and its own write tool is not covered by the guard.
- OpenCode background sub-agents are an OpenCode experimental feature.
- The guards catch mistaken writes; they are not a sandbox. Without
  `--sandbox`, an executor is guarded only against writing into the brain;
  with it, on macOS, its writes and control sockets are confined but the
  network is not (`THREAT-MODEL.md`).
- The test suite is hermetic (herdr stubbed), so a green suite does not prove a
  live pane worked.

Each of these is entered in the register with its scope, impact, evidence and
workaround.

## Feedback

This candidate exists to find what breaks on machines other than the one it was
built on. A failing `install.sh --check`, a `hw` dry run that names the wrong
thing, or a guard that refuses something harmless is exactly the report that is
wanted.
