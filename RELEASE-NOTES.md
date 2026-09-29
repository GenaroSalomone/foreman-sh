# foreman-sh 0.1.0-rc.1

**A release candidate.** It is meant for early feedback, and it ships with a
register of what it does not do: read
[`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md) before installing. It is not a
stable release; the interfaces below may still change.

## What it is

A harness for running coding agents in two roles over your own repositories.

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
finished; optionally `engram` (memory), `fzf`, `rg`. OpenCode is supported as an
executor agent. See `INSTALL.md`.

## What to know before you rely on it

- macOS is where it is developed. Linux is tested in a container without a
  live herdr. Windows runs under WSL2 as Linux, and natively under Git Bash
  measured on a CI runner only, not yet on a real machine. herdr only.
- Codex can be dispatched with `--sdd none` only, is not set up by the
  installer, and its own write tool is not covered by the guard.
- OpenCode background sub-agents are an OpenCode experimental feature.
- The guards catch mistaken writes; they are not a sandbox.
- The test suite is hermetic (herdr stubbed), so a green suite does not prove a
  live pane worked.

Each of these is entered in the register with its scope, impact, evidence and
workaround.

## Feedback

This candidate exists to find what breaks on machines other than the one it was
built on. A failing `install.sh --check`, a `hw` dry run that names the wrong
thing, or a guard that refuses something harmless is exactly the report that is
wanted.
