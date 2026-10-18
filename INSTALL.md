# Installing a brain of your own

One command builds a brain for your own project: a brainer that plans and
cannot write into your repository, and `hw`, which gives each task its own
executor in its own worktree that reports back with `done-invoker`.

## Prerequisites

On macOS, first install [Homebrew](https://brew.sh), which needs Apple's Command Line
Tools (its installer asks for them, so one command covers both; skip it if
`brew --version` already answers). It then prints two `eval "$(…/brew shellenv)"`
lines for your shell profile: add them and open a new terminal.

```sh
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

- macOS, or Linux (Debian/Ubuntu-class, bash 5; tested in a container, see
  `KNOWN-LIMITATIONS.md` L1). Windows is not supported (see [Windows](#windows) below).
- `git`, `jq` 1.7 or newer, `python3`, `sd`, `fd` and `rg` (`hw` calls all three).
  On Linux (Debian: `apt install sd fd-find ripgrep`, then link `fdfind` as `fd`)
  also Node 22.7 or newer (the OpenCode guard plugin is an ES module in a `.js` file).
  On macOS, `--check` does not name `sd`, `fd` and `rg` yet; the Homebrew formula
  and `--with-recommended` install them.
- [herdr](https://herdr.dev), running, with its Claude Code integration:
  `herdr integration install claude`
- [Claude Code](https://claude.com/claude-code), with its first run finished (below)
- recommended: `engram` (memory, see [Memory](#memory-engram-is-recommended)); optional: `fzf` (the picker behind a bare `hw`)
- optional: [Go](https://go.dev/dl) 1.22 or newer, which lets the installer build `hw-core`, the Go half
  of `hw` (see [The Go core and the way back](#the-go-core-and-the-way-back)); without it `hw` runs in
  bash, and `--check` does not list it as missing
- optional: the Bitwarden CLI (`npm i -g @bitwarden/cli`), only for a brief that declares `secrets:`
  (see [Secrets](#secrets-bitwarden))
- for an OpenCode lane: [OpenCode](https://opencode.ai) 1.18.31 or newer, and herdr's OpenCode
  integration: `herdr integration install opencode` (run `opencode` once first,
  so its config directory exists)

Every version floor above (herdr, Claude Code, OpenCode, Node, jq and engram)
is one row of `lib/hw/deps.conf`. `--check` prints each installed version against its floor and
refuses one below it with the command to update it; `hw preflight --json -- <dispatch>` does the
same for the tools that dispatch uses (rule `dep-floor`). A tool that is not installed is reported
where it is needed, as before. The installed versions are remembered in `~/.cache/hw/deps`.

`./install.sh --brain ~/brain --check` evaluates all of it in one pass and
writes nothing. It names anything missing with its install command, then lists
the fixes in the order they must be done (tools, Claude Code's first run,
herdr's integrations, engram, a lane) and ends with one line, `Next step: …`.
Run it again after each fix.

## Installing the dependencies: `--with-recommended`

`install.sh --with-recommended` installs what is missing, required and
recommended, through Homebrew: one `brew install` per package, each printed
before it runs. A failed one is named and the rest are still tried. It exits 0
only when nothing is left missing.

| Package | Kind | Command it runs |
|---|---|---|
| herdr, jq (1.7 or newer), rg, fd, sd | required | `brew install herdr`, `brew install jq`, `brew install ripgrep`, `brew install fd`, `brew install sd` |
| Claude Code | required | `brew install --cask claude-code@latest` (macOS; see below) |
| engram | recommended | `brew install gentleman-programming/tap/engram` |
| fzf | recommended | `brew install fzf` |

What Homebrew does not install here is named with its own method and never
run: `git` and `python3` (`xcode-select --install` on macOS, your distribution's
packages on Linux), and Claude Code on Linux (https://claude.com/claude-code).
Without Homebrew it installs nothing, names https://brew.sh and exits 1. On
Linux the list also has Node (`brew install node`).

Claude Code is the `claude-code@latest` cask on purpose: measured on a clean macOS
VM, the `claude-code` cask (stable channel) was 2.1.287, below the 2.1.289 floor
(`lib/hw/deps.conf`), so `install.sh` refused a machine `--with-recommended` had just
set up. `claude update` does nothing for a Homebrew install (it prints a tip), so the update
command `--check` and `hw preflight` print for Claude Code is detected from where the binary
lives, not assumed: the `claude-code@latest` cask gets `brew upgrade --cask claude-code@latest`,
the stable cask gets `brew uninstall --cask claude-code && brew install --cask claude-code@latest`,
and any other install (the native installer) gets `claude update`. Measured on a clean macOS VM:
the stable cask (2.1.287) refused by `--check` with the second command, which then
installed 2.1.296 in 18 s.

`foreman-sh init` offers Homebrew only for what Homebrew carries. If only `git` or `python3`
is missing it names `xcode-select --install` (Apple's Command Line Tools) and sends you
to nothing else. A `git` or `python3` that does not run counts as missing: on a Mac without
the Command Line Tools, `/usr/bin/git` and `/usr/bin/python3` exist as shims that print
"No developer tools were found" and exit 1 (`--check` and `init` both say so, measured on a clean macOS VM).

It installs packages and configures nothing. Claude Code's first run,
`herdr integration install claude` and `engram setup claude-code` stay yours;
`--check` names them. OpenCode is not in the list: its Homebrew formula was
1.18.30 when measured, below the 1.18.31 this harness was measured against, so
an OpenCode lane keeps `npm install -g opencode-ai@latest`.

```sh
./install.sh --with-recommended --check   # prints the commands, runs none
./install.sh --with-recommended           # runs them
```

With `--brain` it continues into the install once the packages are done. With
`--check` it runs nothing. When `brew` is on your PATH and a required tool is
missing, `--check` points at this flag.

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
now", which changes nothing. The brainer is yours to answer: on Claude Code's
first start it may stop on that same offer; press `Esc`, and `brain` says when
it is waiting on it. Any other screen an executor stops on is named
in `hw`'s error, never answered.

## `~/.local/bin` on your PATH

`hw`, `brain` and the invokers are linked into `~/.local/bin` (or `--bin-dir`).
If that directory is not on your `PATH` they are not found and `brain demo`
fails. The installer warns and `--check` lists the fix: add
`export PATH="$HOME/.local/bin:$PATH"` to your shell profile, open a new
terminal and restart herdr, so its panes inherit it.

## Memory (engram) is recommended

engram is recommended, not required, and becomes mandatory only per task: when
`hw` registered an engram session for an executor (receipt `engram_session` is
an `hw-…` id), `done-invoker` refuses a completion that names no stored
observation as `#<id>`. With no registered session, or with `--blocked`, it
asks for none. Without engram the harness runs and each
report still reaches the brainer's session, but nothing is saved to memory:
later sessions start cold. For reports to reach memory, Claude Code needs an
`engram` server (`brew install gentleman-programming/tap/engram`, or engram's
[installation guide](https://github.com/Gentleman-Programming/engram/blob/main/docs/INSTALLATION.md)).
The installer checks for one (a `mcpServers.engram` entry or an enabled engram
plugin) and, when it is missing, prints:

```sh
engram setup claude-code
```

engram asks `Add to allowlist? (y/N)`: answer `y`. It lists only engram's own
memory tools in Claude Code's permissions, so saving to memory never stops to ask.

It does not run that itself. That command writes your Claude Code config by
engram's own rules, which falls outside what this installer says it writes,
and it would add a second registration where the plugin already provides one.

## Quick start: `foreman-sh init`

One command takes a fresh install to a first dispatch:

```sh
foreman-sh init             # from a checkout: ./install.sh init
foreman-sh init --dry-run   # prints the plan and changes nothing
foreman-sh init --yes       # answers its questions yes
```

It checks `git`, `jq`, `python3`, `herdr`, `claude`, `rg`, `fd` and `sd`: a missing
one is named with its install command. With Homebrew on PATH `init` then asks
whether to install what Homebrew carries (`--with-recommended`, above; `--yes`
answers yes) and checks again in the same run, so a machine without `claude` needs
one `init`, not `init`, `--with-recommended`, `init`. Without that answer, or if
something is still missing, it stops having written nothing and prints one line to
copy: `foreman-sh --with-recommended && foreman-sh init`. It then offers herdr's
Claude integration and `engram setup claude-code` (it asks first, and `--yes` answers
for you), builds `~/brain` with a `demo` lane over a toy repository at `~/code/toy`
(`--lane NAME --repo PATH` for your own), copies a sample brief to
`~/brain/demo/briefs/hello.md` and ends with `hw demo hello … --dry-run`. Claude
Code's first run and your PATH stay yours: `init` names them. Without a Go toolchain
(1.22 or newer) it says in one `NOTE` line that `hw` runs in bash. A second run changes
nothing and says so. `--brain DIR` and `--bin-dir DIR` move what it writes.

## The command

On macOS, the Homebrew formula is the shortest way. It installs foreman-sh
with herdr, jq, rg, fd and sd, and links `foreman-sh`, which is this
`install.sh` with the same flags:

```sh
brew install GenaroSalomone/tap/foreman-sh
foreman-sh --brain ~/brain --lane myapp --repo ~/code/myapp
```

The brain keeps its own copy of the mechanism. To refresh it, run
`foreman-sh upgrade --brain ~/brain`: it takes the newest release (`brew upgrade
foreman-sh`), reinstalls with the flags recorded in `~/brain/.foreman/install.json`,
runs `--check` and prints the release's "In short". `--to VERSION` installs that
one (the way back) and `--dry-run` changes nothing. Claude Code is a
cask, so the formula cannot depend on it: `foreman-sh --with-recommended`
installs it (above).

Without Homebrew, it is one line on a machine without the repository. It clones the latest published
release tag into a temporary directory, runs that `install.sh` with your
arguments and removes the directory (`git` and `curl` are required):

```sh
curl -fsSL https://raw.githubusercontent.com/GenaroSalomone/foreman-sh/main/install.sh | bash -s -- --brain ~/brain --lane myapp --repo ~/code/myapp
```

Or from a checkout, as the README's Quickstart shows:

```sh
./install.sh --brain ~/brain --lane myapp --repo ~/code/myapp
```

Run it again with another `--lane`/`--repo` to add a project. Running the same
command twice changes nothing.

Then:

```sh
brain myapp                                   # open the brainer
$EDITOR ~/brain/myapp/briefs/first-task.md    # write a brief
hw myapp first-task --brief ~/brain/myapp/briefs/first-task.md --sdd none --no-report --dry-run
hw myapp first-task --brief ~/brain/myapp/briefs/first-task.md --sdd none
```

`brain` needs herdr's server up: start `herdr` once in a terminal first (measured on a
clean VM with the headless `herdr server`; without one, `brain` refuses with
`server_not_running`, naming `herdr`).

The dry run carries `--no-report` because, outside a herdr pane that `brain` opened, `hw`
exits 1 with `HW_INVOKER_PANE UNRESOLVED` (an executor launched there could not report
back). The launch line has none: run it from the brainer.

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

## Permissions: skip (recommended) or ask

`hw` and `brain` launch agents with their permission prompts skipped: Claude
Code with `--dangerously-skip-permissions`, OpenCode with `--auto`. Codex gets
no flag and keeps its own `approval_policy`. That is **recommended** because
an executor runs unattended, and it is a choice, not a requirement:

```sh
./install.sh --brain ~/brain --permissions ask   # or skip
hw <lane> <task> --permissions ask               # one dispatch
HW_PERMISSIONS=ask hw <lane> <task>              # one shell
```

The first form writes `~/.config/hw/permissions` (under `$XDG_CONFIG_HOME`),
one word. In a terminal the installer asks once when nothing is set; with no
terminal, a pipe, or `--check`, it asks nothing and writes nothing. The order
is the flag, then `HW_PERMISSIONS`, then that file, then `skip`, and the
manifest's `permissions` line shows which one won. Anything other than `ask`
or `skip` is refused.

The cost of `ask`: Claude Code and Codex then stop on every prompt, and
`hw` cannot answer it or detect it (OpenCode shows it as `blocked
reason=permission`). Codex is pinned to `-a on-request`. A running pane keeps
what it launched with. The Bypass Permissions warning in "first run" above
matters only for `skip`.

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

A brainer already open without the cockpit mod picks it up without losing its
conversation: `brain relaunch --all` (or `brain relaunch <lane>…`, `--dry-run` to
preview) relaunches each idle Claude Code brainer in its own pane, resuming its
own session and its own account (`CLAUDE_CONFIG_DIR`); a working pane is skipped
and named, and a pane whose agent was stopped but did not come back is reported
`FAILED` with a non-zero exit. `brain <lane> --resume <id>` is the single-lane
form (exit 75 when it did not start). A pane is matched to its lane by directory
identity, under the lane's canonical name in `projects.json`: a pane standing in an
alias path (a symlink to the lane directory) or in a symlink to the lane is that lane.
Executors are not relaunched.

The cockpit mod needs Claude Code 2.1.289 or newer: below that, `brain` prints
`cockpit needs claude >= 2.1.289` and opens without it. To open a brainer without
it on purpose, set `FOREMAN_COCKPIT=0` or pass `brain <lane> --no-cockpit`. What
it shows and what its buttons run are in the README.

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

## Frameworks for `--sdd`

`hw` asks `--sdd none|speckit` on every dispatch where a framework is in
play. `none` needs nothing. `speckit` is a third-party project the installer
does not install, and it is refused before anything is built when it is not
there:

| Mode | What the machine needs |
|---|---|
| `speckit` | [Spec Kit](https://github.com/github/spec-kit) (MIT), with a `speckit-*` skill in the repository's `.claude/skills` or in `~/.claude/skills`. Claude Code only: Codex and OpenCode dispatches are refused. |

foreman-sh is not part of that project and implies no endorsement by it.

There is no other mode: `--sdd gentle` and `sdd: gentle` in a brief are gone and
warn, then run `--sdd none`; `hw gentle-home` no longer exists; and the
`sdd_modes` lane field is not a field any more, so a `projects.json` that still
carries it fails to load (`unknown lane field(s)`): delete the key. A
`review: rdd` brief warns and closes with Judgment Day. What a task closes with
is the brief's `review:` (`judgment-day`, the default; `plugins`; `none`), below.

## Dispatching: briefs, the ledger and `hw preflight`

`hw <lane> <task>` reads the brief at `~/brain/<lane>/briefs/<task>.md`, or the
one `--brief` names. With neither it stops, naming the path it looked for and
the nearest brief names; `--no-brief` launches without a contract on purpose,
and warns that the executor has no verification. For a brief under
`~/brain/<lane>/briefs/`, `hw` commits it at dispatch when the brain is a git repository on `main` (the
installer does not make it one) and the brief is untracked or changed; on another
branch it warns and leaves it. Each real
dispatch also appends a line to the ledger, which `hw ledger` reads (see the
table below). A bare `hw` opens an `fzf` picker over every lane's briefs.

`hw preflight --json -- <hw arguments>` prints what `hw` would warn or refuse
about a dispatch you have not run, as `{"level":"ok|warn|block","rules":[…]}`.
It is read-only and exits 0 whatever the level (a malformed call exits 1).

### Plugins for one executor: `plugins:`

A brief can ask for third-party code-quality plugins with `plugins: [thermo, ponytail, pstack]`
in its frontmatter. `hw` loads them into **that executor only**: a Claude Code executor gets one
`--plugin-dir` per plugin (that session; the plugins' hooks run natively in it), a pi executor one
`-e <plugin dir>` per plugin (that run: skills and extension); opencode and codex are told `NOT APPLIED`
in the dry run. Nothing is installed globally; what a hook writes goes to `$HW_ARTIFACTS/plugin-state`,
never to `~/.claude` or `~/.pi`; the receipt of the run records the field.

**Installing them is an optional step, off by default.** The plugins are pinned copies under
`setup/vendor/plugins/<name>/` (the upstream sha, a sha256 manifest and the license are in each;
`setup/vendor/plugins/verify` checks them offline). Installing them only copies them, after checking
every file against its manifest, into `<brain>/plugins/`, where a brief that declares `plugins:` can
find them; nothing is written to `~/.claude` or `~/.pi` and nothing becomes active by itself. Without
the step, a brief with `plugins:` is refused before anything is built, and the refusal names the
command that installs them and lists what is installed.

```bash
./install.sh --brain ~/brain --with-review-plugins         # a new or existing brain (also: init --with-review-plugins)
bash setup/vendor/plugins/install                          # a brain that IS a checkout of this repository: <checkout>/plugins
./install.sh --brain ~/brain --check                       # says whether they are installed ("Review plugins (optional)")
```

`upgrade` replays the flag once it was used. Their hooks are **not** vendored, so none runs. **The
export does not ship the copies**, so there `--with-review-plugins` refuses; to use the field in your
own brain, put a plugin at `<brain>/plugins/<name>/` with a `.claude-plugin/plugin.json` and its
skills. A re-task with `hw next` cannot add plugins to a live executor.

## What it installs

| Where | What |
|---|---|
| `~/brain/bin`, `lib/hw`, `layouts`, `lanes/git-worktree.sh`, `setup/guards`, `cockpit` | the mechanism, copied from this checkout; refreshed on every run. `bin/hw` loads its modules from `lib/hw` (`reap`, `status`, `done`, `next`, `ledger`) and stops naming one that is missing |
| `~/brain/core/dist/<os>-<arch>/hw-core` | only with Go 1.22 or newer on PATH and without `--no-core`: `hw`'s Go half, built from this checkout (see [The Go core and the way back](#the-go-core-and-the-way-back)) |
| `~/.config/hw/impl` | the same condition, and only when the file does not exist: one word, `go` (`$XDG_CONFIG_HOME` and `HW_IMPL_FILE` are honoured) |
| `~/brain/projects.json`, `guards.json` | your lanes and what the guards protect; created, then only added to |
| `~/brain/<lane>/` | `CLAUDE.md`, `decisions.md`, `briefs/`, and `.claude/` with the read-only guard |
| `~/brain/<lane>/.opencode-executor/` | an OpenCode lane only: the executor's guard plugin and its policy |
| `~/work/<lane>/<task>` | each task's git worktree: beside the brain, never inside it (the brain guard would refuse every command an executor ran there), and outside your repo. Set by `"work"` in `projects.json` |
| `~/.config/hw/permissions` | only with `--permissions` or an answer in a terminal: `ask` or `skip` (see Permissions above) |
| `~/work/.hw-ledger/<lane>.jsonl` | the dispatch ledger `hw ledger` reads: one line per real dispatch (brief and its sha, run, account, model, effort, vendor, base, pane). Outside every worktree, so `hw reap` never removes it; `HW_LEDGER_DIR` moves it |
| `~/brain/<lane>/briefs/archive/<yyyy-mm>/` | what `hw briefs archive --apply` moves a finished brief into, with a `MANIFEST.tsv` (path, blob, reason) that `hw briefs unarchive` reads. Inside the brain, committed; `hw <lane> <task>` refuses an archived task instead of launching it with no brief |
| `~/archive/<lane>/<task>/` | what `hw reap --apply` and `hw done` keep of a merged task before removing its worktree: its `.artifacts`, `qa-report`, `test-results`, `playwright-report`, and a `pg_dump -Fc` of its database on a lane with `db.provisioned`. Beside `"work"`; `HW_ARCHIVE_ROOT` moves it |
| `~/.local/bin` | links: `hw`, `brain`, `done-invoker`, `ask-invoker`, `channel-send`, `decisions`, and `opencode-auto` for an OpenCode lane |
| `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR`) | one Stop hook, merged; the previous file is kept as `settings.json.bak-brain-install` |
| `~/.claude/skills/judgment-day/`, `~/.claude/agents/jd-*.md` | only with `--with-judgment-day` (see Judgment Day below) |

Nothing is written inside your repository. If something already exists and
differs, like another brain's Stop hook, a lane with that name over a
different repo, a foreign link in `~/.local/bin`, or a non-empty directory the
installer did not make, it refuses and names it before writing anything.

## The Go core and the way back

`hw` has two halves: its bash implementation (`bin/hw` and `lib/hw`) and
`hw-core`, a Go binary that runs the ported verbs. Both give the same answers;
speed differs by verb (measured on one machine: `hw worktrees` about 0.5 s in Go against 1.6 s in bash, `hw help` about 0.14 s against 0.07 s). Go is optional.

- **What is built.** When a Go toolchain 1.22 or newer is on PATH, the installer
  builds `hw-core` with `setup/build-core --native` into
  `~/brain/core/dist/<os>-<arch>/hw-core`, where `hw` looks for it. The build is
  reproducible (the same bytes every run), so a second run writes nothing.
- **The switch file.** The installer also writes `~/.config/hw/impl`
  (`$XDG_CONFIG_HOME` and `HW_IMPL_FILE` are honoured) containing `go`, only when
  that file does not exist. One that exists is never touched. With the file at
  `go` and `hw-core` built, the ported verbs run in Go. **Without that file and
  without `HW_IMPL`, `hw` runs bash:** the default in `bin/hw` is bash, and the Go
  default of an installed machine comes from the file the installer wrote.
- **The way back to bash.** `HW_IMPL=bash` switches every verb for that command or
  shell; `HW_IMPL_<VERB>=bash` switches one verb. To make it permanent, edit or
  remove the switch file. A `hw-core` that is missing, or older than the sources in
  `core/`, makes `hw` fall back to bash with one warning.
- **Without Go.** With no Go on PATH, or one older than 1.22, or with `--no-core`
  (or `FOREMAN_SH_NO_CORE=1`), nothing is built and the installer exits 0; all but
  the environment variable print one `NOTE` line. A `go` that does not answer `go version`
  counts as unreadable, with its own `NOTE`. A `hw-core` an earlier install left is removed. No switch file is written and `hw` runs in bash: same
  answers. `--check` never lists Go as missing. Install Go and run
  the installer again to move the ported verbs to Go.
- **Which one runs.** `HW_IMPL` takes `bash`, `go` or `shadow`; any other value
  in the environment stops the command (in the switch file it warns and runs
  bash). For one verb, the first of these that says something wins:
  `HW_IMPL_<VERB>`, `HW_IMPL`, a `<verb>=<mode>` line of the switch file, the
  file's one-word default, and then bash. The ported verbs run in Go when
  `hw-core` is built and the answer is `go`; for any verb that answers `bash`
  they run in bash. `shadow` runs both, prints bash's answer and logs every
  difference to `~/.local/state/hw/shadow.jsonl` (`hw shadow --help`): a
  diagnostic for whoever reports a difference, not a mode to live in.
- **`--agent pi`.** A dispatch with `--agent pi` stays on bash and prints one line
  saying so. The export carries the builders of git-worktree lanes only; a lane
  hook of another name is sourced by bash.

## Secrets (Bitwarden)

A task that needs a credential names Bitwarden items in its brief
(`secrets: [github-token, stripe-key.password]`, folder `agents`) instead of
carrying a value. Before anything is built, `hw` checks that the vault is
unlocked (a locked one gets one `secret-unlock --auto` try, which works only on
macOS with the master password kept in the login Keychain) and that every item
exists; a missing one refuses the dispatch by name. `--no-handle-check` skips
the check and says so. The executor is told to reach a value only as

```text
hw secret run <item>[.<field>] --as VAR [--item <item> --as VAR …] -- <cmd…>
```

which puts it in `<cmd>`'s environment: never printed, never written to disk,
never in `argv`. `hw secret check <item>…` says present or absent, and
`hw secret status` says locked or unlocked.

What you set up: the `bw` CLI, logged in once (`bw login`), and a vault folder
named `agents` that holds only what an agent may use. Then, in a terminal and
once a day, `secret-unlock` (the master password is asked by `bw`; the session
token goes to `~/.cache/hw/bw-session`, mode 0600, for 12 hours by default, and
`secret-unlock --lock` drops it). On macOS, `secret-unlock --setup` stores the
master password in the login Keychain so a dispatch can unlock by itself;
nothing else here reads it.

What this release does **not** ship: the global guard that refuses an agent's
own `bw get|unlock|export|list|serve` and the unlock LaunchAgent installer.
Without the guard, `hw secret run` is the sanctioned path and nothing forces an
agent onto it, and the session token opens the whole vault, not just the folder.
See `KNOWN-LIMITATIONS.md` L23.

## Windows

Not supported. foreman-sh runs on macOS and Linux only; nothing is tested on
Windows, native or under Git Bash. WSL2 is a Linux, so the Linux path may work
there, but it is not tested either (`KNOWN-LIMITATIONS.md`, L1b).

## Judgment Day (optional)

Nothing requires it, with one exception: a brief that declares `boundary:` is
not accepted as done without an `APPROVED` design judgment, which needs this
skill (see KNOWN-LIMITATIONS.md, L10).

Judgment Day is a blind review by two judges of a diff before it counts as
finished: both read the same target, neither sees the other's findings, and only
a severe defect both confirm is fixed, in at most two rounds. It ships in
`_skills/judgment-day/` with its three agents in `_agents/`, derived from Gentle
AI: the skill under the Apache License 2.0, the agents under the MIT License
(`_skills/judgment-day/LICENSE`, `LICENSE-MIT` and `NOTICE`). To activate it for
Claude Code:

```sh
./install.sh --brain ~/brain --with-judgment-day
```

It copies the skill into `$CLAUDE_CONFIG_DIR/skills/judgment-day/` and the
three agents into `$CLAUDE_CONFIG_DIR/agents/` (`~/.claude/` when unset), and
does nothing else with them. It is idempotent: a second run changes no file,
and a newer checkout updates the copies an earlier run made. It has the same
collision rule as the links: a file of yours under one of those names, one that
differs from this checkout's copy and that no earlier run made, or a symlink
that points anywhere but at an identical copy (one that does is left alone and
never written through), stops the run naming the path before anything is
written. `--check` says on one line whether Judgment Day is active, and with
`--with-judgment-day` also reports any collision.

Then ask a session for "judgment day" over a diff. The judges use engram to
recall context when it is registered (see Memory above).

**Which review a build closes with is the brief's choice**, in every lane and
every `--sdd` mode: `review: judgment-day` (the default when the key is absent)
orders Judgment Day over the branch at the close; `review: plugins` names only
the review skills of the brief's `plugins:` (`thermo`, `ponytail`, `pstack`) and
no Judgment Day; `review: none` adds no review note. Any other value refuses the
dispatch, naming those three. The choice is printed in the dispatch manifest and
recorded in the run's receipt.

## Limits, stated

- **Claude Code or OpenCode executors.** pi is dispatched with `--agent pi`, not installed
  by this script (`herdr integration install pi`; L3b). Codex lanes are not installed: nothing
  can prove a Codex executor end to end today, so `--vendor codex` is refused.
  The installer does not write `~/.codex/hooks.json` either, so a Codex session
  is unguarded until you register the Codex guard yourself (L3, L5).
- **An OpenCode lane's guard is a plugin, not `permission.edit`.** It judges
  the `bash` tool and OpenCode's file tools (`write`, `edit`, `multiedit`,
  `patch`, `apply_patch`) by absolute path. A lane's `permission.edit` deny on
  a protected repo does not hold by itself: OpenCode 1.18.34 matches edit rules
  against the path relative to the worktree, so an absolute pattern never
  matches a file outside it. Any other tool that writes files is not judged.
- **No OpenCode agent may carry `tools: {edit|write|bash: true}`.** That
  legacy map appends an allow-all rule after every deny in the config, so
  `permission` denies stop applying to that agent. Grant tools through
  `permission` instead; `setup/check-machine` checks the live config.
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
  ~/brain/<lane>/briefs/probe.md --sdd none --model haiku`.
- **The sandbox is macOS, and Claude Code or pi, only.** `hw <lane> <task> --sandbox`
  (or `sandbox: true` in a brief) runs an executor inside a Seatbelt profile;
  the installer sets nothing up for it, and it is refused elsewhere. What it
  confines and what it leaves open: `THREAT-MODEL.md` and `KNOWN-LIMITATIONS.md`
  (L11).
- **Your repo's own `.claude/settings.json` hooks must resolve.** hw refuses
  to launch into a worktree whose versioned hooks name a missing binary.
  `--allow-stale-hooks` overrides, and the override is recorded.
