# Installing a brain of your own

One command builds a brain for your own project: a brainer that plans and
cannot write into your repository, and `hw`, which gives each task its own
executor in its own worktree that reports back with `done-invoker`.

## Prerequisites

- macOS, or Linux (Debian/Ubuntu-class, bash 5; tested in a container, see
  `KNOWN-LIMITATIONS.md` L1). Windows: WSL2, or native Windows under Git Bash,
  measured on a CI runner and a Windows 11 VM (see [Windows](#windows) below).
- `git`, `jq` 1.7 or newer, `python3`. On Linux also `sd`, `fd` and `rg` (Debian:
  `apt install sd fd-find ripgrep`, then link `fdfind` as `fd`) and Node 22.7 or newer
  (the OpenCode guard plugin is an ES module in a `.js` file).
- [herdr](https://herdr.dev), running, with its Claude Code integration:
  `herdr integration install claude`
- [Claude Code](https://claude.com/claude-code), with its first run finished (below)
- optional: `engram` (memory), `fzf` (hw's pickers), `rg`
- for an OpenCode lane: [OpenCode](https://opencode.ai) 1.18.31 or newer, and herdr's OpenCode
  integration: `herdr integration install opencode` (run `opencode` once first,
  so its config directory exists)

`./install.sh --check` names anything missing, with its install command, and
writes nothing.

## Claude Code's first run is yours

Once per account, Claude Code shows a welcome (theme, then login) and, the
first time it runs in Bypass Permissions mode, a warning you have to accept.
Every brainer and executor runs in that mode, so each of them would stop on
those screens. Only you can answer them, and the installer never answers them
for you. Do it once, in a terminal, before the first `brain`:

```sh
claude --dangerously-skip-permissions   # finish the welcome and login, accept the warning, /exit
```

`claude auth login` alone is not enough: it logs you in but does not finish
the welcome, so the next interactive `claude` shows it again. With
`CLAUDE_CONFIG_DIR` set, run it with that variable set. The installer checks
all three (welcome, login, warning) and prints this command when one is
missing. So do `hw`'s dry run and `brain`.

Other first-run screens are handled for you. `brain` and `hw` mark their
folders trusted. An executor answers the fullscreen-renderer offer with "Not
now", which changes nothing. Any other screen an executor stops on is named
in `hw`'s error, never answered.

## Memory (engram) is a declared prerequisite

For reports to reach memory, Claude Code needs an `engram` server. The
installer checks for one (a `mcpServers.engram` entry or an enabled engram
plugin) and, when it is missing, prints:

```sh
engram setup claude-code
```

It does not run that itself. That command writes your Claude Code config by
engram's own rules, which falls outside what this installer says it writes,
and it would add a second registration where the plugin already provides one.

## The command

From a checkout of this repository:

```sh
./install.sh --brain ~/brain --lane myapp --repo ~/code/myapp
```

Run it again with another `--lane`/`--repo` to add a project. Running the same
command twice changes nothing.

Then:

```sh
brain myapp                                   # open the brainer
$EDITOR ~/brain/myapp/briefs/first-task.md    # write a brief
hw myapp first-task --brief myapp/briefs/first-task.md --sdd none --dry-run
hw myapp first-task --brief myapp/briefs/first-task.md --sdd none
```

## What it asks

In a terminal, a **new** lane is asked three things it would otherwise leave
generic. Each shows its default, and Enter keeps it:

| Option | Asked as | Where it lands | Default |
|---|---|---|---|
| `--operator NAME` | your name, as `hw`'s messages say it | `projects.json` `operator` | "the operator" |
| `--min-model haiku\|sonnet\|opus\|none` | lowest Claude tier its executors may run without `--below-floor-why` | the lane's `model_floor` | none |
| `--requested-by required\|warn\|none` | whether every brief must cite the request it answers (`requested_by:`) | the lane's `requested_by` | none |

Give an option and it is not asked. Without a terminal (a script, CI, a pipe)
nothing is asked and the result is exactly what the installer made before
these options existed. A lane that already exists is never asked again and
keeps its row; naming a different value for it is refused, and so is a
different `--operator` over one already in `projects.json`. There is no
per-lane default `--effort`: `hw` takes `--effort` per dispatch only.

## Your own files in the bin directory

Every link the installer writes (`hw`, `brain`, `done-invoker`, `ask-invoker`,
`channel-send`, `decisions`, and `opencode-auto` for an OpenCode lane) is
checked before anything is written. A file or directory of yours under one of
those names, or a link that points elsewhere, stops the run naming the path,
and so does a `--bin-dir` that cannot be a directory. Nothing is overwritten.

## OpenCode executors

```sh
./install.sh --brain ~/brain --lane myapp --repo ~/code/myapp --vendor opencode --model opencode/big-pickle
brain myapp
```

`--vendor opencode` makes OpenCode the lane's executor. The brainer is still
Claude Code, and `brain <lane>` opens it without any flag (a lane that ships
an OpenCode brainer of its own, with an `opencode.json`, keeps opening that
one; `--claude` forces Claude Code, and `--opencode` still needs that file). `--model` takes
`provider/model` and is optional; without it OpenCode uses its own default.

The executor gets its own guard. The installer writes
`~/brain/<lane>/.opencode-executor/`, and `hw` passes that directory to the
executor as `OPENCODE_CONFIG_DIR`. It holds a plugin that refuses any shell
write into your repository, with a policy of its own that leaves the task's
own worktree writable. Nothing is written to `~/.config/opencode` or to the
worktree.

`opencode-auto` is linked too: `opencode --auto` with background subagents on,
the same thing `hw` launches, as a command you can type.

1.18.31 is the oldest OpenCode the harness has been measured against, so it is
the minimum the installer accepts. It was also measured on 1.18.33.

Signing OpenCode into a provider is yours: `opencode auth login`, or a model
that needs no login. The installer never reads it.

The vendor is fixed when the lane is created. Running the installer again
with another `--vendor` for the same lane is refused.

## What it installs

| Where | What |
|---|---|
| `~/brain/bin`, `layouts`, `lanes/git-worktree.sh`, `setup/guards` | the mechanism, copied from this checkout; refreshed on every run |
| `~/brain/projects.json`, `guards.json` | your lanes and what the guards protect; created, then only added to |
| `~/brain/<lane>/` | `CLAUDE.md`, `decisions.md`, `briefs/`, and `.claude/` with the read-only guard |
| `~/brain/<lane>/.opencode-executor/` | an OpenCode lane only: the executor's guard plugin and its policy |
| `~/brain/work/<lane>/<task>` | each task's git worktree (outside your repo) |
| `~/.local/bin` | links: `hw`, `brain`, `done-invoker`, `ask-invoker`, `channel-send`, `decisions`, and `opencode-auto` for an OpenCode lane |
| `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR`) | one Stop hook, merged; the previous file is kept as `settings.json.bak-brain-install` |

Nothing is written inside your repository. If something already exists and
differs, like another brain's Stop hook, a lane with that name over a
different repo, a foreign link in `~/.local/bin`, or a non-empty directory the
installer did not make, it refuses and names it before writing anything.

## Windows

Use **WSL2**: install a Linux distribution (`wsl --install`), and install and
run everything — git, jq, python3, herdr, Claude Code and this installer —
inside it, in its own filesystem (`~/`), not under `/mnt/c`.

**Native Windows, under Git Bash (Git for Windows), runs the harness, measured
on a CI runner and a Windows 11 VM.** What it needs (the installer checks the first two and
names them before it writes anything):

- **Windows Developer Mode** (Settings → System → For developers): `hw` and the
  installer create symlinks, and Windows refuses them otherwise.
- **Native Windows Python first on Git Bash's PATH** as `python3`. An MSYS2 or
  Cygwin Python makes the read-only guard refuse every call, because it cannot
  decide there. `jq`, `rg`, `fd` and `sd` as Windows builds; Git for Windows
  ships none of them, and no `rsync` either (the installer does without it).
- `git config --system core.autocrlf false`. Git for Windows ships it `true`,
  which rewrites the harness's shell scripts with CRLF.

What makes it work: every entry point in `bin/` loads `bin/msys-compat.sh`,
and through it `bin/sitecustomize.py` in every Python it starts. That layer
gives bash and Windows Python one spelling of a path and LF line ends, and it
runs `#!` scripts through Git Bash's own bash. `hw` reaches herdr over its
named pipe. The read-only guard decides on Windows: it folds `/c/x`, `C:/x`,
`C:\x` and their case variants to one form, reads `~` from the account's
profile, and when `python3` cannot start, the hook command exits 2, which
Claude Code treats as a refusal.

What has been run: the setup suite on a GitHub-hosted `windows-latest` runner
in Git Bash and in WSL2, with herdr and the agents stubbed, plus this installer
from an empty home with the real herdr binary and `bin/herdr-rpc` against a
live headless herdr, and the exported installer on a Windows 11 ARM64 VM. Not verified: a real Windows machine with herdr's panes,
Claude Code, a brainer and an executor reporting back. The numbers and every
skip are in `KNOWN-LIMITATIONS.md`, L1b.

## Judgment Day (recommended)

Judgment Day is a blind review by two judges of a diff before it counts as
finished: both read the same target, neither sees the other's findings, and only
a severe defect both confirm is fixed, in at most two rounds. It ships in
`_skills/judgment-day/` with its three agents in `_agents/`, derived from Gentle
AI: the skill under the Apache License 2.0, the agents under the MIT License
(`_skills/judgment-day/LICENSE`, `LICENSE-MIT` and `NOTICE`). The installer does not copy it; to activate it for Claude Code:

```sh
mkdir -p ~/.claude/skills ~/.claude/agents
cp -R _skills/judgment-day ~/.claude/skills/
cp _agents/jd-*.md ~/.claude/agents/
```

Then ask a session for "judgment day" over a diff. The judges use engram to
recall context when it is registered (see Memory above).

## Limits, stated

- **Claude Code or OpenCode executors.** Codex lanes are not installed: nothing
  can prove a Codex executor end to end today, so `--vendor codex` is refused.
- **An OpenCode lane's guard covers shell commands only.** It is a plugin on
  the `bash` tool. OpenCode's own edit and write tools are not refused by it.
- **An OpenCode executor gets no `--agent`** unless `~/.config/opencode/opencode.json`
  defines the one `hw` asks for (`direct-worker` by default). Without it `hw`
  warns and OpenCode runs its default agent.
- **One brain per user.** The Stop hook in your Claude Code settings belongs to
  one brain.
- **A task's worktree is a plain `git worktree add`.** Ignored files like
  `.env`, `.venv` or `node_modules` are not carried over. A repo that needs them
  gets its own `lanes/<lane>.sh` that calls its own setup script.
- **First-run screens are matched by their text.** The handling above was
  tested against Claude Code 2.1.281's screens:
  live for the welcome and for a brainer whose agent exited, and with a stub
  for the Bypass warning and the fullscreen offer. To repeat it on a new user:
  `./install.sh --check`, then `brain <lane>` *before* the command above (it
  names the waiting screen), then the command, then `hw <lane> probe --brief
  <lane>/briefs/probe.md --sdd none --model haiku`.
- **Your repo's own `.claude/settings.json` hooks must resolve.** hw refuses
  to launch into a worktree whose versioned hooks name a missing binary.
  `--allow-stale-hooks` overrides, and the override is recorded.
