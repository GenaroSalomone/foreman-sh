# foreman-sh 0.1.3

**Install with one command.** `install.sh` can now be piped from curl: it
fetches the last published release (never `main`) and runs from there, with the
same flags. When it creates a lane it asks its questions on the terminal even
through the pipe, and with no terminal it prints the defaults it used and the
flags that change them. The command is in the README's Quickstart.

Also in this release: `done-invoker` says when it is running the brief's
verification after the report is delivered, and `hw done --force` and a
blocked report no longer run that verification again. The Codex guard now
also refuses an `apply_patch` into a protected tree.

## Upgrading from 0.1.2

Update your checkout and run `install.sh` again with the arguments you
installed with, or run the Quickstart command. Nothing else needs to change.

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
   Windows through WSL2:
   `curl -fsSL …/install.sh | bash -s -- --brain ~/brain --lane myapp --repo ~/code/myapp`
   (the full command is in the README's Quickstart).
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

macOS or Linux (on Windows, WSL2); `git`, `jq` 1.7 or newer and `python3`;
[herdr](https://herdr.dev) with its Claude Code integration;
[Claude Code](https://claude.com/claude-code) with its first run finished; on
Linux also `sd`, `fd`, `rg` and Node 22.7 or newer. engram is recommended for
memory. OpenCode 1.18.31 or newer can run executors. See `INSTALL.md`.

## What to know before you rely on it

- macOS is where it is developed. Linux is tested in a container without a
  live herdr. Windows runs under WSL2 as Linux, and natively under Git Bash
  measured on a CI runner only, not yet on a real machine. herdr only.
- Codex can be dispatched with `--sdd none` only, is not set up by the
  installer, and its own write tool is not covered by the guard.
- OpenCode background subagents are an OpenCode experimental feature.
- The guards catch mistaken writes; they are not a sandbox. Without
  `--sandbox`, an executor is guarded only against writing into the brain;
  with it, on macOS, its writes and control sockets are confined but the
  network is not (`THREAT-MODEL.md`).
- The test suite is hermetic (herdr stubbed), so a green suite does not prove a
  live pane worked.

Each of these is entered in the register with its scope, impact, evidence and
workaround.

## Feedback

A failing `install.sh --check`, a `hw` dry run that names the wrong thing, or a
guard that refuses something harmless is exactly the report that is wanted:
open an issue.
