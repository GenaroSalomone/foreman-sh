# foreman-sh

**Plan with one agent. Build with many. Keep your repository out of the planner's hands.**

[![CI](https://github.com/GenaroSalomone/foreman-sh/actions/workflows/ci.yml/badge.svg)](https://github.com/GenaroSalomone/foreman-sh/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

foreman-sh runs coding agents in two roles, on top of [herdr](https://herdr.dev):

- a **brainer**, the planner, that reads your code, plans the work and writes
  briefs, but cannot touch your repository;
- **executors** that each take one brief, work in a git worktree of their own
  and report back when they are done.

You talk to the brainer. It sends out the work, and the results come back to it.

```mermaid
sequenceDiagram
    actor You
    participant B as Brainer
    participant E as Executor
    You->>B: describe the work
    B->>B: write a brief
    B->>E: hw myapp fix-login --brief … (own worktree and branch)
    E->>E: work and commit
    E->>B: report
    B->>You: what was done, and what is next
```

**Install on macOS:** `brew install GenaroSalomone/tap/foreman-sh && foreman-sh init`
builds a brain with a demo lane and ends with a dry-run dispatch. Linux, step by
step and requirements: [Quickstart](#quickstart). Not related to Ruby's `foreman`.

## Why

Put several coding agents on one repository and three things go wrong. Each has a
mechanism here:

| What goes wrong | What foreman-sh does |
|---|---|
| The planner starts editing, and changes land that nobody reviewed. | A guard refuses the brainer's writes to your repository. |
| Agents step on each other in a shared checkout. | One worktree and one branch per task. |
| Results get lost in a pane nobody is watching. | A report lands in the brainer's session by itself. |

## Concepts

| Term | Meaning |
|---|---|
| **Brain** | The directory the installer creates (`~/brain` here). It holds foreman-sh's commands, its configuration and one folder per lane, outside every repository. |
| **Lane** | One of your repositories, together with the brainer that plans for it. |
| **Brainer** | A long-lived Claude Code session for a lane, opened with `brain <lane>`. A guard refuses its writes to the repository. |
| **Executor** | A disposable agent session, launched with `hw`. It works in its own worktree and branch and ends each task with a report. |
| **Brief** | The executor's instructions, as a Markdown file: goal, scope and the command that proves the work is done. |

## Features

- **A planner that cannot write.** The installer never writes inside your
  repositories and never reads a credential.
- **One worktree and one branch per task**, outside your checkout, with its own
  database where the lane provisions one.
- **Reports that arrive.** `done-invoker` puts the executor's report in the
  brainer's session, with no polling; `ask-invoker` asks it a question and waits.
- **Briefs that are checked.** A brief's front matter is validated against the
  dispatch before anything is built, and `hw done` runs the brief's verification
  command itself and records its exit code.
- **Your agent, your model.** Claude Code, OpenCode, pi and Codex
  ([L3](docs/KNOWN-LIMITATIONS.md)) run a lane's executors; each one gets a
  `cheap-explorer` subagent on its vendor's cheapest tier.
- **Frameworks when you want them.** `--sdd none` makes the brief the whole
  contract; `--sdd speckit` goes through [Spec Kit](https://github.com/github/spec-kit)'s phases.
- **Secrets as Bitwarden items.** A brief names `secrets: [item, …]`; the value
  reaches the executor through `hw secret run`, never as text in a brief or a prompt.
- **A cockpit.** A card per executor, and a band above the brainer's prompt that
  names the one that needs you.
- **Cleanup that is safe.** `hw reap` removes the worktree, branch and database
  of a task that is merged and clean, and nothing else.
- **A Go core.** `hw`, `brain`, `channel-send` and the invokers run in `hw-core`,
  which the installer builds ([INSTALL.md](docs/INSTALL.md#the-go-core-and-the-way-back),
  [L16](docs/KNOWN-LIMITATIONS.md)).

## Requirements

- **macOS or Linux** (Debian/Ubuntu-class, bash 5 for the shell parts); Linux works, not verified per release
  ([L1](docs/KNOWN-LIMITATIONS.md)).
- **[herdr](https://herdr.dev)**, running, with its Claude Code integration:
  `herdr integration install claude`.
- **[Claude Code](https://claude.com/claude-code)**, with its first run finished:
  run `claude --dangerously-skip-permissions`, complete the welcome and login,
  accept the warning, then `/exit`. `install.sh --permissions ask` leaves the
  permission prompts on ([INSTALL.md](docs/INSTALL.md#permissions-skip-recommended-or-ask)).
- `git`, `jq` 1.7 or newer, `python3`, `rg`, `fd` and `sd`; on Linux, Node 22.7
  or newer.
- **Go 1.22 or newer**, which `install.sh` uses to build `hw-core`. `install.sh --check`
  lists it as missing and names https://go.dev/dl.

On macOS, Homebrew installs all of these but Claude Code's first run. Each tool
has a minimum version, kept in `lib/hw/deps.conf`; `install.sh --check` and
`hw preflight` name the command that updates one
([INSTALL.md](docs/INSTALL.md), [L15](docs/KNOWN-LIMITATIONS.md)).

Recommended and optional:

- **[engram](https://github.com/Gentleman-Programming/engram)**, recommended: it
  gives the brainer memory across sessions, and without it nothing is saved
  between them ([L8](docs/KNOWN-LIMITATIONS.md)). Install it with
  `brew install gentleman-programming/tap/engram && engram setup claude-code`
  and answer `y` when it asks about the allowlist.
- **Judgment Day**, a blind review of a diff by two judges
  ([L10](docs/KNOWN-LIMITATIONS.md)); **Bitwarden CLI (`bw`)**, for a brief that
  declares `secrets:`; **[OpenCode](https://opencode.ai)** 1.18.31 or newer and
  **pi**, to run a lane's executors
  ([INSTALL.md](docs/INSTALL.md#opencode-executors), [L3b](docs/KNOWN-LIMITATIONS.md)).

## Quickstart

### 1. Homebrew, on macOS

Every `brew` command below needs [Homebrew](https://brew.sh) (skip this if
`brew --version` already answers). Its installer also installs Apple's Command
Line Tools:

```sh
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"   # brew.sh's own installer; it also installs the Command Line Tools
```

It ends by printing two `eval "$(…/brew shellenv)"` lines: add them to your shell
profile and open a new terminal.

### 2. Install

On macOS, one command builds a brain, a `demo` lane over a toy repository and a
sample brief, and ends with a dry-run dispatch:

```sh
brew install GenaroSalomone/tap/foreman-sh
foreman-sh init             # checks what you have, builds ~/brain with a demo lane, ends with a dry-run dispatch
foreman-sh init --dry-run   # the plan only; --yes answers its questions
```

`init` names each missing tool with its install command and offers to install
what Homebrew carries. A second run changes nothing and says so
([INSTALL.md](docs/INSTALL.md#quick-start-foreman-sh-init)). For your own
repository instead of the demo, `foreman-sh` takes the installer's flags
(`foreman-sh --with-recommended` installs Claude Code, engram and fzf):

```sh
foreman-sh --brain ~/brain --lane myapp --repo ~/code/myapp   # creates your brain and a first lane
export PATH="$HOME/.local/bin:$PATH"                          # puts hw and brain on your PATH
brain myapp                                                   # opens the brainer
```

On Linux, or without Homebrew, the installer runs from a pipe:

```sh
curl -fsSL https://raw.githubusercontent.com/GenaroSalomone/foreman-sh/main/install.sh | bash -s -- --brain ~/brain --check   # checks everything, writes nothing
curl -fsSL https://raw.githubusercontent.com/GenaroSalomone/foreman-sh/main/install.sh | bash -s -- --brain ~/brain --lane myapp --repo ~/code/myapp   # creates your brain and a first lane
```

To read the installer first, clone the repository and run it from there:

```sh
git clone https://github.com/GenaroSalomone/foreman-sh && cd foreman-sh
./install.sh --brain ~/brain --check
./install.sh --brain ~/brain --lane myapp --repo ~/code/myapp
```

Add the `PATH` line to your shell profile so it survives a new terminal.
`--check` lists what is left, in order, and ends with the next command to run;
this is the head of a healthy one:

```text
$ ./install.sh --brain ~/brain --check
prerequisites
  ok    git
  ok    jq 1.7.1 (>= 1.7)
  ok    herdr
  ok    claude
  ok    go 1.26 (builds hw-core)
  ok    herdr's claude integration

first run of Claude Code in this account (yours to do)
  ok    welcome completed
  ok    logged in
  ok    Bypass Permissions warning accepted
```

### 3. Try it on a toy repository

```sh
git init ~/code/toy && echo "toy: a throwaway repository for trying foreman-sh." > ~/code/toy/README.md
git -C ~/code/toy add README.md && git -C ~/code/toy commit -m init
./install.sh --brain ~/brain --lane demo --repo ~/code/toy
cp examples/demo/briefs/hello.md ~/brain/demo/briefs/
herdr   # once, in a terminal: `brain` needs herdr's server (without it: server_not_running)
brain demo
```

Then, in the brainer (type it with Claude Code's `!` prefix, or ask the brainer
to do it):

```sh
hw demo hello --brief ~/brain/demo/briefs/hello.md --sdd none --no-report --dry-run   # shows the plan, creates nothing
hw demo hello --brief ~/brain/demo/briefs/hello.md --sdd none             # launches it
```

`--no-report` is for the preview only, outside a pane `brain` opened
([L7](docs/KNOWN-LIMITATIONS.md)); the launch line has none. An executor opens in a
new herdr tab, writes `HELLO.md`, commits it on branch `task/hello`, and its
report arrives in the brainer's session.

### Upgrading

```sh
foreman-sh upgrade --brain ~/brain              # the newest release, with the flags you installed with
foreman-sh upgrade --brain ~/brain --dry-run    # says what it would do and changes nothing; --to 0.3.0 goes back
```

`upgrade` replays the flags the install recorded, runs `--check` and prints the
new release's "In short"; `hw status` tells you when a newer release exists
([INSTALL.md](docs/INSTALL.md#the-command)).

## How it works

1. **Brief.** You describe the work; the brainer writes a brief.
2. **Dispatch.** `hw` creates a worktree and branch outside your checkout,
   opens a herdr tab, starts the agent and makes sure the brief arrived.
3. **Work.** The executor works and commits on its branch. If it needs a
   decision, it asks the brainer with `ask-invoker` and waits for the answer.
4. **Report.** The executor finishes with `done-invoker`. The report arrives
   in the brainer as a message, with no polling.
5. **Close.** `hw done` runs the brief's verification, records the result and
   closes the tab. Merging is always up to you; once a task is merged, its
   worktree, branch and database are removed by `hw reap`, which the brainer's
   session start runs in the background, after archiving the task's outputs.

## Everyday use

| Command | What it does |
|---|---|
| `brain <lane>` | Open the lane's brainer. |
| `hw <lane> <task> --brief <path> --sdd none` | Launch an executor. Add `--dry-run` to preview. |
| `hw` | A picker over every lane's briefs, plus a `+ new brief` row (needs [fzf](https://github.com/junegunn/fzf)). |
| `hw status` | What is running, what reported and what is left over. |
| `hw ledger [<lane>]` | Every brief as never-dispatched, in-progress or done. |
| `hw ruling <pane> "<correction>"` | Correct an executor that is still working, or resume one that reported `--blocked`. |
| `hw next <pane> --brief <path>` | Give the next task to an executor launched with `--keep-pane`. |
| `hw done <lane> <task>` | Run the brief's verification and close the task's tab. |
| `hw reap [<lane>]` | List merged worktrees and task branches that are safe to remove; `--apply` removes them. |
| `hw train add\|push <lane> …` | Merge reported tasks into `train-<lane>`, then run the suite and move `main` to it. |

A brief's front matter also takes `review:`, `secrets:` and `plugins:`. `hw help all` lists every
command and flag, `hw help <topic>` shows one part, and [REFERENCE.md](docs/REFERENCE.md) has
each row in full, with the frameworks, the cockpit and every lane setting. `<pane>` is the executor's herdr pane, as
`hw status` lists it.

### Secrets (Bitwarden)

A credential an executor needs is a Bitwarden item in the folder `agents`, named
in the brief's `secrets: [item, …]`. `hw` checks the vault and each item before
it builds anything, and the executor reaches the value only through
`hw secret run <item> --as VAR -- <cmd>`: it lands in `<cmd>`'s environment and
is not printed or written ([INSTALL.md](docs/INSTALL.md#secrets-bitwarden),
[L23](docs/KNOWN-LIMITATIONS.md)).

### The cockpit

A brainer opened by `brain` runs with a cockpit mod: a card per executor, and a
band above the prompt that names the one that needs you. `/cockpit` opens the
full pane. A card, top to bottom:

```text
task-05 · Add retries to the importer        ● working 3h12m · turn 41m
claude · claude-sonnet-5-5 · high · ctx 41% ▰▰▱▱▱▱
2 commits · last 37m ago · gate running 12m
⚠ no progress 47m · 1 ruling queued 25m
                                                   [ receipt ]
```

Each control runs a command that already exists. Turn the cockpit off with
`FOREMAN_COCKPIT=0` or `brain <lane> --no-cockpit`; every card line is in
[REFERENCE.md](docs/REFERENCE.md#the-cockpit).

### Customize a lane

`projects.json` at the brain root is the one place a lane's settings live, and
`brief_note` there adds a rule to every executor prompt of that lane. Every field is
in [REFERENCE.md](docs/REFERENCE.md#customizing-a-lane-projectsjson).

## How it compares

| | Isolation | Guards the brainer | Agents | UI |
|---|---|---|---|---|
| **foreman-sh** | worktree and branch per task | yes | Claude Code, OpenCode, pi, Codex ([L3](docs/KNOWN-LIMITATIONS.md)) | herdr panes |
| [claude-squad](https://github.com/smtg-ai/claude-squad) | tmux and worktree per agent | — | Claude Code, Codex, Gemini, Aider | TUI |
| [Conductor](https://www.conductor.build) | isolated workspaces | — | Claude Code, Codex, Cursor | macOS app |
| [uzi](https://github.com/devflowinc/uzi) | worktree per agent | — | Claude Code, Codex and others | CLI and tmux |
| [container-use](https://github.com/dagger/container-use) | container per agent | — | any agent through MCP | terminal and web |
| [vibe-kanban](https://github.com/BloopAI/vibe-kanban) | branch per task | — | Claude Code, Codex, Gemini and others | web kanban |
| [Claude Code agent teams](https://code.claude.com/docs/en/agent-teams) | shared checkout | plan approval | Claude Code | panel or tmux |

Compared from each project's own documentation, September 2026.

What foreman-sh adds on top of isolation:

- the brainer cannot write to your code;
- a brief's front matter is checked against the dispatch before anything is
  built;
- `hw done` runs the brief's verification command itself and records its exit
  code in the receipt.

## Learn more

- [`INSTALL.md`](docs/INSTALL.md): every install option, OpenCode and engram.
- [`REFERENCE.md`](docs/REFERENCE.md): every command, brief field, cockpit control and lane setting.
- [`BRIEF-TEMPLATE.md`](docs/BRIEF-TEMPLATE.md): what a brief can contain.
- [`THREAT-MODEL.md`](docs/THREAT-MODEL.md): what the guards stop, what they do not
  stop and the opt-in macOS `--sandbox`.
- [`KNOWN-LIMITATIONS.md`](docs/KNOWN-LIMITATIONS.md): what this release does not
  do, or does only partly.
- [`CHANGELOG.md`](CHANGELOG.md): what changed in each release.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`SECURITY.md`](SECURITY.md).

## License

MIT. See [`LICENSE`](LICENSE).
