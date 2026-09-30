# foreman-sh 0.1.0

**The first stable release.** It follows four release candidates and fixes
what a clean install of the last one found. What it does not do, or does only
partly, is listed in [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## What changed since 0.1.0-rc.4

- **The first run tells you what to do.** `install.sh --check` says to answer
  `y` to engram's allowlist question, and lists adding `~/.local/bin` to your
  `PATH` as a step in order instead of a warning. `brain` waits for engram's
  server before registering the session, and the server no longer prints an
  error on first use. An executor without Judgment Day reports it as
  "not installed (optional)", not as escalated.
- **The guards close more spellings.** `env -u NAME`, `env -i` and the other
  `env` options no longer hide a write into a protected repository, and a
  quoting trick that got a write past the Python and JavaScript guards is
  refused.
- **The public CI is green** on Linux and macOS, and on Windows (Git Bash)
  with its time budgets scaled for a shared runner.
- **The README was rewritten**: requirements first, a table of concepts, every
  command checked against `hw help all`, and a dated comparison with similar
  tools. `BRIEF-TEMPLATE.md` no longer carries a section written for one of
  the maintainer's own projects.
- **Faster to work on.** The full suite runs in about 8 minutes instead of 30,
  only one full suite runs per machine at a time, and the pre-commit hook runs
  only the tests a commit touches.

## Upgrading from 0.1.0-rc.4

Update your checkout and run `install.sh` again with the arguments you
installed with. Nothing else needs to change.

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
