# foreman-sh 0.1.0-rc.3

**A release candidate.** It is meant for early feedback, and it ships with a
register of what it does not do: read
[`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md) before installing. It is not a
stable release; the interfaces below may still change.

## What changed since 0.1.0-rc.2

- **A product executor can work in its worktree again.** The installer put
  task worktrees under `<brain>/work`, inside the brain, where the reverse
  guard refused every command an executor ran: on a new installation no
  product executor could do anything. `work` now goes beside the brain, and
  `install.sh`, `install.sh --check` and `hw` refuse a guarded lane whose work
  directory is inside the brain, naming how to move it.
- **The brainer's guard reads the shell.** A relative path
  (`rm -rf ../myapp/src`), a `cd` earlier in the same command, a shell-local
  variable, a glob, a brace expansion, `find -delete`/`-exec`, `fd -x` and a
  literal `sh -c` or `eval` body are now resolved before deciding, with the
  same verdicts in the Python, JavaScript and Codex guards.
- **The Python guards fail closed on bad input.** In the Claude Code hooks
  and the Codex hook, a payload that is not an object is refused instead of
  crashing the hook, a crash the agent does not treat as a refusal;
  Claude Code's reverse guard refuses when `python3` is missing, where it
  exited 127, which Claude Code does not treat as a refusal.
- **`install.sh --check` names every fix in one pass**, in the order they
  must be done, and ends with one `Next step:`. `--with-judgment-day` installs
  Judgment Day, and the README's quickstart is copyable as written.
- **The commands follow the usual CLI conventions.** `hw --help` is one page
  with `hw help <topic>` for the rest; `hw`, `brain` and `install.sh` answer
  `--help` and `--version`; `hw` without a terminal prints its usage instead of
  waiting on a picker; colour honours `NO_COLOR` and is off when output is not
  a terminal.
- **A threat model** ([`THREAT-MODEL.md`](THREAT-MODEL.md)) states what the
  guards stop, what they do not, and what the harness assumes.

## Upgrading from 0.1.0-rc.2

An installation made by 0.1.0-rc.2 keeps its worktrees in `<brain>/work`, and
this release refuses it until that changes:

1. Finish or `hw done` every open task, so no worktree is left in
   `<brain>/work`; then remove it (and run `git worktree prune` in each
   product repository).
2. Set `"work"` in `<brain>/projects.json` to a directory outside the brain.
3. Run `install.sh --brain <brain> --lane <lane> --repo <path>` again for
   **every lane**. This is what adds the new worktree directory to
   `guards.json` and to the brainers' deny rules; until it runs, the brainer's
   guard does not protect worktrees under the new work directory.

The refusal prints the exact paths.

The full list is in [`CHANGELOG.md`](CHANGELOG.md).

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
- The guards catch mistaken writes; they are not a sandbox, and an executor
  is guarded only against writing into the brain (`THREAT-MODEL.md`).
- The test suite is hermetic (herdr stubbed), so a green suite does not prove a
  live pane worked.

Each of these is entered in the register with its scope, impact, evidence and
workaround.

## Feedback

This candidate exists to find what breaks on machines other than the one it was
built on. A failing `install.sh --check`, a `hw` dry run that names the wrong
thing, or a guard that refuses something harmless is exactly the report that is
wanted.
