# Known limitations — 0.1.0-rc.4

This is the register of what this pre-release does not do, or does only
partly. Each entry says its **scope** (where it applies), its **impact**, its
**evidence** (`measured` = observed by running it; `unverified` = believed, not
executed) and a **workaround** where there is one. Nothing here is a hidden
regression: a limitation that a change in this release introduced or widened
would not be listed as "inherited", and none is.

What the guards are built to stop, and what they are not, is laid out as a
whole in [`THREAT-MODEL.md`](THREAT-MODEL.md).

A limitation is not a promise to fix it. The maintainer owns every entry, and
unless an entry names a different next decision, it is re-examined at the next
release candidate.

## Platform

### L1. Platforms: macOS and Linux; Windows through WSL2 or native Git Bash (L1b)
- **Scope:** every command.
  - **macOS:** developed and tested here.
  - **Linux:** the installer accepts it and the whole test suite runs green in a
    Debian container (bash 5, git, jq 1.7, python3, Node 22, sd, fd, rg).
    Not exercised: a live herdr, a live Claude Code or OpenCode agent, and a
    real desktop session. Subjects that need the maintainer's product-repo
    checkouts skip themselves by name. There is no keychain backend:
    `hw handle` says so and exits 2, and a brief that names a handle is
    launched with that check skipped and a warning that the handle was not
    verified. `jq` older than 1.7 (Debian 12, Ubuntu 22.04 ship 1.6) is refused
    by the installer, because 1.6 reports empty input as success under `jq -e`.
  - **Windows:** see L1b. Through WSL2 the Linux path applies; native Git Bash
    is measured on a CI runner and, for the install, on a Windows 11 VM.
- **Impact:** on Linux, first-run behaviour with a real herdr and a real agent is
  unmeasured; expect to be the first to find what a container cannot.
- **Evidence:** macOS and Linux (container) `measured`; Windows in L1b.
- **Workaround:** on Windows, use WSL2 (L1b).

### L1b. Windows: WSL2, or native Git Bash measured on a CI runner and a VM
- **Scope:** Windows. Two ways in: WSL2, which is Linux, and native Windows
  under Git Bash (Git for Windows), the shell Claude Code uses there.
- **Impact:** under WSL2 the harness behaves as on Linux (L1). Under native Git
  Bash it runs through `bin/msys-compat.sh` and `bin/sitecustomize.py`, which
  give bash and Windows Python one spelling of a path and LF line ends; `hw`
  reaches herdr over its named pipe; and the read-only guard decides there,
  folding `/c/x`, `C:/x`, `C:\x` and their case variants to one form, reading
  `~` from the account's profile, and refusing (exit 2) when `python3` cannot
  start. What it needs: Windows Developer Mode (symlinks), a native Windows
  `python3` first on Git Bash's PATH (an MSYS2 or Cygwin Python makes the guard
  refuse every call), and `git config --system core.autocrlf false`
  (INSTALL.md, Windows).
- **0.3.0-rc.2, measured 2026-10-02:** on the maintainer's suite, a full run
  (not one subject at a time) is green on Linux (container) and on WSL2, and
  red on native Git Bash: four subjects fail there (a load-only timing subject,
  two whose Windows arms are skipped because herdr is not on Git Bash's PATH,
  and the reap's process kill), and the run outlives the CI step's 150 minutes.
  The stable 0.3.0 waits for Git Bash green.
- **Evidence:** the maintainer's full setup suite (a superset of the one this
  repository ships; the subject numbers below are its own) on a GitHub-hosted `windows-latest` runner,
  herdr and the agents stubbed as everywhere in the suite, every subject file
  run one by one (survey mode). Git Bash: 188 of 188 pass, the guard's vector
  suites included, in 57 minutes. WSL2 (Ubuntu 24.04), same commit: 185 of
  188, and 207 passes since. The two left are not WSL2-specific: 69 needs the
  maintainer's live brain checkout above the tree and says so; and the
  OpenCode guard's vector suite, whose symlink cases point at product repos
  the runner does not have. That run found a real gap: the JavaScript half did
  not follow a symlink whose target is missing, so a write through it into a
  protected repository was allowed where the Python half refused it.
  Release 0.1.0-rc.2 closed it (the JavaScript half follows the dangling link, with
  the Python half's verdict); the fix is `measured` on macOS, on Linux
  (Ubuntu 24.04 under WSL2) and on Windows under Git Bash: in one CI run the
  OpenCode guard's vector suite passes 1722 of 1722 on both, 250 lines that
  name a symlink (100 of them dangling) included, Python, JavaScript and
  parity halves alike, with the filesystem suite at 42 of 42. There is no
  native-Linux workflow: the Linux figure is WSL2's. That run predates the
  case-variant change (0.1.0-rc.2), the shell reading (0.1.0-rc.3) and the
  spellings closed in 0.1.0-rc.4 in the same guards (L5); the guards as shipped
  are `measured` on macOS and `unverified` on WSL2 and Git Bash. The export itself, installed on a
  Windows 11 ARM64 VM under Git Bash from an empty home with the real herdr
  and Claude Code: `--check`, an install, a second identical run that writes
  nothing, and `hw <lane> probe --dry-run`, `measured`. An install from an empty home
  with the real herdr binary, and `bin/herdr-rpc` against a live headless herdr
  over its pipe, `measured`. Skipped by name on Git Bash, each with its reason
  in the subject's output: the pseudo-terminal arms (no pty in Git for Windows
  or native Python: 66, and the terminal arms of 207), the chmod arms (NTFS
  under Git Bash is noacl, so `chmod 000`/`a-w` locks nothing: 18, 91, 92),
  the historic-guard comparisons (186), the committed per-lane settings that
  were rendered for another home (88, 184), and the zsh pane-shell restore
  (199: no zsh there, and herdr's pane shell is PowerShell). Mutation coverage
  exempts on Git Bash only arms listed by name, each with its reason and printed
  in the run: 186's four, whose pre-fix guards have no Windows path model.
  The live herdr subject (650: hw against a real herdr) stops on Git Bash once
  it has found herdr: in the pane hw builds the agent never starts and the pane
  opens outside the workdir, so its mutant is listed there too.
  `unverified`: Claude Code's
  `Edit`/`Write` deny rules on Windows paths; `brain --reset` detaching
  against a live herdr; and use on a real Windows machine — herdr's panes,
  Claude Code, a brainer and an executor reporting back.
- **Workaround:** use WSL2 where a measured end-to-end path matters.

### L2. herdr is a hard requirement
- **Scope:** launching any brainer or executor. Each is a herdr pane, and `hw`
  reads an agent's state through herdr's integration.
- **Impact:** without herdr running and its Claude integration installed, the
  installer stops before writing anything, and `hw` dispatches nothing.
- **Evidence:** measured. On a home directory that has never run Claude Code,
  `herdr integration install claude` itself fails ("claude directory not
  found") until Claude Code's first run has created its config directory.
- **Workaround:** run Claude Code once, then `herdr integration install claude`.
- **Same for OpenCode:** `install.sh --vendor opencode` also stops, before
  writing anything, when herdr's OpenCode integration is missing (its report
  reads `MISSING herdr's opencode integration`), and it needs Claude's
  integration as well. Under an empty home directory that is exactly what a
  clean install meets. `measured` by the maintainer's clean-install check (not
  shipped): the installer exits 1 with nothing written. Like Claude's, it is
  the person's step: run OpenCode once so its config directory exists, then
  `herdr integration install opencode`.

### L2b. Native install: Homebrew on macOS; a pipe elsewhere
- **Scope:** `brew install GenaroSalomone/tap/foreman-sh` and
  `install.sh --with-recommended`.
- **Impact:** the formula cannot depend on Claude Code, which is a cask, so
  Claude Code is one more step: `foreman-sh --with-recommended` or
  `brew install --cask claude-code@latest`. On Linux there is no native package. The
  piped installer is the way, and `--with-recommended` works only where
  Homebrew is on PATH. Without it, it names https://brew.sh and installs nothing.
  On macOS, `install.sh --check` does not name `rg`, `fd` and `sd`, which `hw`
  calls. The formula and `--with-recommended` install them.
- **Evidence:** measured on macOS with Homebrew 7.0.7. The formula passes
  `brew audit --strict`, installs from a local tap with `--build-from-source`,
  its test assertions hold, and the installed `foreman-sh --check` runs. The
  dependencies were not reinstalled, so that run used `--ignore-dependencies`.
  A real `--with-recommended` run and the published tap were measured on a clean macOS 26.6
  VM (tart, no Homebrew, no Command Line Tools, empty HOME), through `init` in one run:
  Homebrew + CLT 145 s and 367 s (two runs, network-bound), `brew install foreman-sh`
  14-24 s, `init --yes` 49 s (claude cask, engram, fzf, integrations, brain, dry run),
  a second `init` 2 s. That run found the `claude-code` cask below the version floor
  (hence `claude-code@latest`). Measured later on the same image: `engram serve` starts inside
  the VM and answers `/health` in 0.17 s (its own port 7437), and the TUI `herdr` attaches over
  `ssh -t` (first paint 0.4 s; it needs a terminal that answers the TUI's queries: a bare
  pty with no emulator draws nothing) and reattaches to the same session. Not measured: Claude
  Code's login and first run (they need a person's credentials), Linux with Homebrew; the
  suite also drives `--with-recommended` against a stub `brew`.
- **Workaround:** the piped or cloned `install.sh`, as before; `--check` names
  every package with its install command.

## The Go core

### L16. The ported verbs run in Go; pi launches and everything not ported stay on bash
- **Scope:** every `hw` verb, through the switch in `bin/hw`.
- **Impact:**
  - **What runs in Go.** `install.sh` builds `hw-core` when Go 1.22 or newer is on
    PATH and writes `~/.config/hw/impl` as `go` when that file is absent (an
    existing one is left as it is). The ported verbs then run in `hw-core`:
    `receipt`, `log`, `handle`, `help`, `worktrees`, the table of `status`, the
    read half of `reap`, `sweep` and `reports`, `review-range`, `review-delta`,
    `preflight --json`, `outbox list`, the `wait` family, the marker writers
    (`ruling`, `human-boundary`, `executor-turn-end`, `chaining-lease-start` and
    `-expire`, `outbox flush`, `output`, `stage`) and the dispatch
    (`hw <lane> <task> …`, its dry run and its launch), and the large writers: `done`,
    `next`, `revive`, `preview`, `reap --apply` and `sweep --apply`. Anything else is bash.
    An executor's own `hw` calls and `shadow` mode never run a writer in Go.
    `HW_IMPL=bash` goes back for every verb, `HW_IMPL_<VERB>=bash` for one.
  - **No Go, a Go older than 1.22, or `--no-core`:** nothing is built and `hw` runs
    its bash implementation: the same answers. Speed differs by verb: measured here, `hw worktrees` takes about 0.5 s in Go and 1.6 s in bash, and `hw help` about 0.14 s in Go and 0.07 s in bash.
  - **pi stays on bash.** `--agent pi` dispatches are not in `hw-core`; they run
    the bash and say so on stderr.
  - **Lane hooks.** The export carries the builders of git-worktree lanes only. A
    lane hook of any other name is handed to bash by `hw-core`.
  - **A missing or stale `hw-core`** (absent, or older than `core/`'s sources)
    gives bash and one warning, never a failure.
  - **Bash is not deleted.** It is the rollback and stays frozen: a fix to a
    ported verb goes to `core/`, so the bash of a ported verb does not receive it.
- **Evidence:** `measured`: the setup suite runs under both implementations, and
  the golden contract compares stdout, stderr, exit status and the resulting tree
  byte for byte; the binary is reproducible (the same sha256 on two builds).
  `unverified`: any long live shadow on a user's machine (the maintainer's was
  short, and no figure from it is given here), and Windows: `hw-core.exe` is built
  by the same builder, but use on a real Windows machine is not measured (L1b).
- **Workaround:** `HW_IMPL=bash`, or `--no-core` at install, to run the bash that
  predates the core.

## Agents

### L3. Codex is not set up by the installer, and is a reduced dispatch target
- **Scope:** `--agent codex`.
- **Impact:** the installer configures Claude Code and OpenCode only. A Codex
  executor can be dispatched with `--sdd none` only; its first launch needs a
  person to clear two dialogs (hook review and a model-migration prompt); and
  the delivery of a brief to it can be shown as *admitted* but not as
  *processed*, unlike the other two agents.
- **Evidence:** measured (first launch, dialogs and delivery receipts).
- **Workaround:** use Claude Code or OpenCode. If you do run Codex, see L5 for
  what its guard does not cover.

The installer does not register the Codex guard: nothing writes
`~/.codex/hooks.json`. The guard script ships in `setup/guards/`, and until you
add it to Codex's `PreToolUse` hook for the shell tool, a Codex session is
unguarded. A Codex brainer of a lane whose repository is the brain itself, once
it leaves the brain directory, is unguarded even with the hook registered.

### L3b. pi is a dispatch target; its guard reads text, and `--sandbox` is what contains
- **Scope:** `--agent pi`.
- **Impact:**
  - **The guards are string-level.** The write and secret-read guards read the
    text of a tool call and decide which paths it *names*; a shell has more
    spellings than any reader closes (a path assembled at run time, a program
    that opens a secret without naming it). "The guard refused it" does not mean
    "the secret cannot be read": what contains a pi executor is `--sandbox`,
    which refuses reads of `~/.ssh`, `~/.config/gh` and `~/.docker` at the OS
    level and nothing else. Run an executor that must not see a secret under
    `--sandbox`, with the secret out of its reach on the host.
  - **No account switch.** `CLAUDE_CONFIG_DIR` is Claude Code's: for pi the
    manifest says `NOT APPLIED`.
  - **Specialists only through `.pi/agents`.** pi-subagents reads
    `~/.pi/agent/agents`, `~/.agents`, `<repo>/.pi/agents` and `<repo>/.agents`,
    never `.claude/agents`, so a pi executor cannot delegate to a Claude
    specialist; the brief's preamble says so. Where the lane's checkout tracks a
    `.pi/agents` directory, `hw` passes it to pi-subagents
    (`PI_SUBAGENT_EXTRA_AGENT_DIRS`) and the executor can delegate to those; a
    specialist file that pins its own `model:` needs the call to pass the
    executor's model.
  - **In a product lane, pi is not at parity on skills.** Of the skills a Claude
    executor loads, pi gets `judgment-day`, `agent-browser` when the brief says
    `requires: browser`, and the checkout's `.claude/skills` (minus `speckit-*`)
    as one directory; the user-level and plugin skills are not passed.
  - **The bash guard in a product lane is string-level too.** It refuses a
    command that names the main checkout or a sibling worktree, and leaves the
    executor's own worktree open. A relative path that climbs out of the worktree
    (`cd ../other && …`), a path built at run time, or a symlink inside the
    worktree that points into the main checkout is not seen. `--sandbox` is what
    contains it: under the profile the OS refuses every write outside the
    worktree, its artifacts and the temporary directories.
  - **`--sdd gentle` (and `speckit`) is refused for pi**, before anything is
    built, with its reason: gentle's guard hook and its plugin skills are wired
    into Claude Code only (L14), and nothing carries them to pi. pi runs
    `--sdd none`.
  - Everything else the V1 list named is closed (processed-proof delivery,
    pi-subagents with the guard as a required child extension, model and thinking
    readers, sandbox, the brain guard on product lanes, cockpit context tokens,
    guard on every tool, explicit skills, turn-end session identity, model-error
    fallback, startup screens). Residuals, none of them a missing mechanism:
    under `--sandbox` herdr's socket is denied, so a sandboxed pi publishes no
    `agent_session` through herdr; `--thinking off` runs `low` on
    `deepseek-v4.1-flash` (pi clamps it, and hw reports the divergence); the
    exact body the real opencode-go returns for a spent plan is `unverified`, and
    there is no pre-dispatch usage gate (the cap is shared dollars: a spent plan
    ends the run as `provider_quota`, and the fallback model is for model errors
    only).
- **Evidence:** `measured` live on pi 1.1.0, and by setup tests 980–989 with their
  mutants.
- **Workaround:** `--sdd none`, or Claude Code where a framework flow is needed.

### L22. `plugins:` loads pinned plugins into claude and pi executors only, and the export ships none
- **Scope:** the brief field `plugins: [<name>, …]`.
- **Impact:**
  - **Where it applies.** Claude Code gets one `--plugin-dir` per plugin and pi one `-e <plugin dir>` (the package's
    skills and its extension). opencode and codex get `NOT APPLIED` in the dry run and in the receipt, and hw does
    not touch them.
  - **The hooks run, in that executor only.** The vendored copies are complete. Every hook command goes through
    `hw/hook-env`, which points `CLAUDE_CONFIG_DIR` and `XDG_CONFIG_HOME` at `$HW_ARTIFACTS/plugin-state/<plugin>`
    for that process, so nothing is written to `~/.claude` or `~/.config` and ponytail's statusline nudge (it asks the
    model to edit `settings.json`) is switched off. Measured with a real haiku session each: ponytail's hook answers
    `PONYTAIL MODE ACTIVE` and pstack's `You have pstack.` with the plugin, nothing without it, and the real HOME is
    unchanged. pstack's `SessionStart` mandate (route a multi-file task to `poteto-mode`) is the plugin's own behaviour
    and competes with the brief as the contract: declare pstack only when that is wanted. pi has no Claude hooks: its
    equivalent is the extension, which `-e` loads (pstack's adds the `agent`, `send_message`, `list_agents`,
    `stop_agent`, `ask_user_question` and `schedule_wakeup` tools; ponytail's answers `PONYTAIL MODE ACTIVE`).
  - **Neutralised, on purpose.** pstack's `poteto-mode/scripts/bootstrap.ts` refuses `bun install` (network, writes
    `node_modules` into the plugin) unless `HW_PLUGIN_ALLOW_INSTALL=1`, so `watch-pr`, `ship-pr` and `orch` do not run
    in an executor; the skills that name `~/.claude` name the plugin state instead. Not redirected: ponytail's pi
    command `/ponytail default <mode>` writes `$XDG_CONFIG_HOME/ponytail/config.json` when a person types it, and
    pstack's `agent` tool may create worktrees under the repository's `.claude/worktrees`.
  - **A re-task cannot add them.** A plugin is loaded when a session starts: the reuse route says `NOT APPLIED`
    and the brainer launches with `--fresh`, and `hw next` ignores the field. A revived pane (`hw revive`) is not
    given them again.
  - **pi stays on bash.** The pi half exists in the bash implementation only; the Go core carries the field for
    claude, opencode and codex and sends a pi dispatch to bash, as before (L16).
  - **Plugin state counts as artifacts.** A hook's state lives under `$HW_ARTIFACTS/plugin-state`, and `done-invoker`
    counts every file under `$HW_ARTIFACTS` (only `.hw/` is excluded): a report-only executor that loaded ponytail
    reports `Artifacts: 2 file(s)` (the nudge flag and `.ponytail-active`) instead of none. Found by both Judgment Day
    judges; not fixed here (it is `done-invoker`'s count, outside this change): exclude `plugin-state` there, or move
    the state under `.hw/`.
  - **`verify` checks the hook shapes the pinned copies use.** It scans `hooks/hooks.json` and the file a string
    `"hooks"` in `plugin.json` names; an inline hooks object or an array of paths in `plugin.json`, a malformed hooks
    file or a missing `jq` read as clean, and a command that merely mentions `hw/hook-env` passes. The manifest still
    catches any byte changed after `build`, so this matters only when a pin is updated: read `PATCH.diff`.
  - **Installing them is optional and off by default.** `hw` reads only `<brain>/plugins/`, which
    `install.sh --with-review-plugins` (or `bash setup/vendor/plugins/install` in a checkout) fills after verifying
    the pinned copies; without it a brief with `plugins:` is refused, naming that command. An install is never
    active by itself and never writes to `~/.claude` or `~/.pi`.
  - **The export ships no plugin.** The copies carry third-party text and its licenses; `setup/vendor/` is
    excluded from the export, so there `--with-review-plugins` refuses and `plugins:` refuses every name until
    you put one at `<brain>/plugins/<name>/` with a `.claude-plugin/plugin.json`.
  - **Cost.** pstack puts ≈12.5 KB of skill descriptions in the executor's context for as long as it runs.
- **Evidence:** `measured` for the dispatch lines, the receipt, the launch flags and the refusals (setup tests 843,
  844, 847, 848 and 979, bash against Go on the same goldens, with a mutant on each decision), and for a real
  executor of each vendor that sees a plugin's skill when its brief asks and does not when it does not, with
  `~/.claude` and `~/.pi` unchanged. `unverified`: that a plugin improves any task; that is a question for a
  measurement of its own.
- **Workaround:** none needed; declare only the plugins the task wants.

### L4. OpenCode background subagents are experimental
- **Scope:** OpenCode executors and brainers that spawn subagents in the
  background. The harness enables them with OpenCode's
  `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS` variable.
- **Impact:** the behaviour, and the variable itself, are OpenCode's and may
  change between its releases. The installer requires OpenCode 1.18.31 or
  newer and pins no upper version.
  A background sub-agent's work is not something the harness's return channel
  tracks separately from its parent session.
- **Evidence:** the `background` option and its detach route were exercised
  against OpenCode 1.18.23 (`measured`), before the installer's minimum rose to
  1.18.31; they are not re-measured on 1.18.31 or later (`unverified`). Whether the feature has left
  experimental status in a later OpenCode release is `unverified`.
- **Workaround:** dispatch without relying on background subagents; the
  foreground path does not depend on the variable.
- **Next decision:** revisit when OpenCode documents the feature as stable.

### L21. Every executor is handed a cheap explorer; the prompt asks for it, nothing forces it
- **Scope:** `hw <lane> <task>` on every vendor, and the `kind: build` ceiling.
- **Impact:**
  - **What `hw` does.** It hands each executor a read-only subagent named `cheap-explorer`
    on its vendor's cheapest tier, through the vendor's own per-launch mechanism and never
    by writing into a product repo: Claude Code `--agents` (haiku), OpenCode an `agent` entry
    in the `OPENCODE_CONFIG_CONTENT` overlay (`gpt-5.6-luna`, `reasoningEffort low`), codex
    `-c agents.cheap-explorer.*` plus `setup/codex/agents/cheap-explorer.toml`
    (`gpt-5.6-luna`, effort low, read-only sandbox), pi `setup/pi/agents` added to
    `PI_SUBAGENT_EXTRA_AGENT_DIRS` (`deepseek-v4.1-flash`, thinking off). The dry run's
    `explorer` line says what reached the launch, or `NOT INJECTED` and why (a pi executor
    whose pi-subagents is not loaded has no subagent tool, and says so). A re-task through
    `hw next` states the tier table only if the run's receipt says the session was launched
    with the explorer.
  - **The prompt asks; nothing forces.** The instruction to hand reading to the explorer is
    text in the executor's prompt. An executor that reads the files itself is not stopped,
    and `hw` does not count its reads.
  - **The top-tier ceiling reaches only models `hw` can rank.** A `kind: build` dispatch on
    the Claude ladder's top (names containing `opus` or `fable`, on the last path segment) is
    refused without `--top-tier-why` or the brief's `top_tier_why:`. `openai/gpt-5.6-sol` (a
    build-only pin by its lane's own decision), `gpt-6-astra`, `deepseek-v4-pro` and other
    non-Claude models are not ranked, so the ceiling does not apply to them: `UNESTABLISHED`,
    not guessed. An undeclared kind is not judged (a claude executor defaults to sonnet);
    a reused pane runs what it already runs.
  - **Cheapest means the vendor's own description.** `luna` is the cheapest tier by the
    descriptions in this machine's `~/.config/opencode/opencode.json` and `~/.codex/config.toml`;
    no pricing is on disk, so the order luna < terra < sol is not a cost measurement. Those
    two vendors' model ids are constants in `lib/hw/explorer.sh`, not read from the account.
  - **pi's probe did not include the write guard.** The pi probe loaded pi-subagents and the
    extra agent directory without `hw-executor.ts`, so that the explorer's children pass through the
    same guard as the executor's (the adapter registers itself as a required child extension) is
    the documented mechanism, not something that probe exercised.
  - **A crash restore drops OpenCode's explorer.** `shell/hw-restore-env.zsh` rebuilds only the
    `--effort` overlay of `OPENCODE_CONFIG_CONTENT` (it never copies a config layer out of a
    file), so a shell restored after a crash has the engram proxy and the explorer missing
    until the pane is revived; `hw revive` replays the run's recorded environment, which
    carries the whole overlay. `unverified` live.
  - **Claude Code's `--agents` JSON is not on the launch line.** `hw` writes it to
    `.hw/<run>/explorer-agents.json` and types a short shell function into the pane with the
    session env (`claude() { command claude --agents "$(cat <file>)" "$@"; }`), because the launch
    line is typed and a pane whose shell is still starting reads it in the tty's canonical mode,
    which holds 1023 bytes: the old 2310-byte line left claude unstarted (measured 2026-10-15,
    opus; a typed line of 1017 bytes started 3/3, 1023 bytes and up 0/3). The function stays
    defined in that pane's shell after claude exits, so a `claude` typed there later carries the
    explorer too.
- **Evidence:** `measured` by real probes on 2026-10-14 (scratch repo, private `ENGRAM_PORT`):
  a mid- or top-tier primary asked where a class is defined delegated to `cheap-explorer`
  and the child ran on `claude-haiku-5-5` (Claude Code, headless and through `herdr agent
  start`), `openai/gpt-5.6-luna` (OpenCode 1.19.0, the task's metadata), `gpt-5.6-luna` at effort low
  (codex 0.153.4, the child rollout's `agent_role` and turn context) and
  `opencode-go/deepseek-v4.1-flash` (pi 1.1.0, pi-subagents 0.77.0); and by
  `setup/tests/1003-cada-ejecutor-delega-por-modelo.sh` with its mutants.
- **Workaround:** for a task that must not be read by the top tier, say so in the brief
  (`requires-agents: subagent:cheap-explorer`) and check the transcript; for a build that needs
  the top tier, pass `--top-tier-why`.

### L14. `--sdd speckit` and `--sdd gentle` are Claude Code only, and gentle needs a download
- **Scope:** dispatches with `--sdd speckit` or `--sdd gentle`, and `hw gentle-home`.
- **Impact:** `hw` refuses both modes for a Codex or OpenCode executor before
  building anything. `gentle` also refuses `--here`, needs a lane that lists it
  in `sdd_modes` (added by hand to `projects.json`) and needs the home that
  `hw gentle-home` downloads from gentle-ai's GitHub releases. Only macOS and
  Linux on arm64 or amd64 have a pinned checksum; any other platform has no
  gentle home. The pin is gentle-ai 4.0.0. The installer copies
  `setup/guards/deny-gentle-real-home.py` into the brain, where `hw gentle-home`
  takes it from (setup test 819); the live download and everything after it have
  not been run from an installed brain. The cockpit is a separate gate:
  it needs Claude Code 2.1.289 or newer, and `brain` opens without it below that.
- **Evidence:** `measured` for the gentle refusals and the checksum check by the
  setup suite (`setup/tests/190`), for the Spec Kit refusals by `75` and `64`
  and for the cockpit floor by `803`, all from a checkout's `bin/hw`;
  the suite feeds `hw gentle-home` a local tarball (`HW_GENTLE_TARBALL`), so the live download is `unverified`.
- **Workaround:** use `--sdd none` with the methodology named in the brief.

### L17. A Claude Code session started before bashEditDiff was turned off keeps showing other lanes' changes
- **Scope:** Claude Code 2.1.299 sessions (brainers and executors) that were already running when `brain`/`hw`
  began setting `CLAUDE_CODE_BASH_EDIT_DIFF=0`, and any `claude` you start by hand under a repo shared by
  several lanes (the brain checkout), or in a pane restored after a herdr crash (the restore replays only
  `HW_*` and `ENGRAM_*`).
- **Impact:** Claude Code snapshots the whole git repo of the cwd before and after every Bash call and
  shows every change in that window as the command's own, so a brainer's pane (and its transcript) displays a
  file another lane wrote meanwhile. It is the UI and the transcript only: it never enters the model's
  context. A session only reads the variable at start.
- **Evidence:** `measured` with a real `claude -p --model opus` and a concurrent writer (variable unset: the
  concurrent files are in `toolUseResult.bashEditDiff`; `=0` and `=false`: none). It is model-gated:
  the same probe on `--model haiku` shows nothing with the variable unset.
- **Workaround:** `brain relaunch <lane>` (or start the session again) so the launch sets it, or
  `export CLAUDE_CODE_BASH_EDIT_DIFF=0` before `claude`.

### L20. `brain relaunch` moves a pane's background tasks out of the session; the resumed claude does not list them
- **Scope:** a claude brainer that has background Bash tasks when `brain relaunch` (or `brain <lane> --resume`) replaces it.
- **Impact:** the exit dialog is answered with «Move to background and exit», so the tasks keep running (reparented to
  init) and the conversation resumes in the same pane, but the resumed claude does not show them as its own and
  cannot read their output or stop them from the UI; they end on their own, or by `kill`.
- **Evidence:** `measured` 2026-10-14 with a real `claude --model haiku`: option 1 («Exit and stop tasks») left no process;
  option 2 left the task running and `claude --resume <session>` returned in the pane with its status bar not listing the shell.
  `setup/tests/891-*.sh` drives the keys against a stub with a real process tree.
- **Workaround:** finish or stop the tasks in the pane before relaunching, if you need to keep supervising them.

## Guards

### L5. The write guards are a tripwire, not a sandbox
- **Scope:** the read-only guard that stops a brainer writing into the
  repository of its lane (Claude Code hook + permission deny rules; OpenCode
  plugin; Codex hook), and the reverse guard that stops a product-lane
  executor writing into the brain.
- **Impact:** a guard decides from the tool call it is shown. It is built to
  catch a mistaken or careless write, including one written with an environment
  variable or an indirect path, but it is not a sandbox and does not contain a
  process that is trying to get around it. It runs as the same OS account as
  the agent it watches, and it sees only the tool calls the agent reports to
  it. Specifically:
  - **Codex:** the guard script reads both its shell tool and `apply_patch`, but
    until the `apply_patch` matcher is registered in `~/.codex/hooks.json`, a write
    through Codex's own patch tool is unguarded, and the Codex configuration this harness was
    developed against runs it without a sandbox or approval prompts. Whether editing the Codex guard script
    re-raises Codex's trust dialog is unknown.
  - **OpenCode:** the plugin judges `bash` and the file tools `write`, `edit`,
    `multiedit`, `patch` and `apply_patch` by absolute path. A file-writing
    tool outside that list (a new built-in, an MCP server's) is not judged,
    and `permission.edit` is no fallback: OpenCode 1.18.34 matches it against
    the path relative to the worktree, so an absolute deny never matches a
    file outside it. If a lane's shim cannot load the shared guard, it
    refuses every `bash` call but not the file tools.
  - Where a write's destination cannot be resolved with confidence, the guard
    refuses on the command's content; it therefore may refuse a harmless
    command that merely names a protected path.
  - A path computed at run time (`$(printf …)`, `eval` or `sh -c` over a
    substitution, a script, a `while read` loop) is resolved by neither
    guard: the brainer's guard cannot see the path, and the reverse guard
    (executors) does not follow a script's or a substitution's write either.
    The brainer's guard does follow a relative path, a `cd` earlier in
    the same command, a shell-local variable, a glob, a brace expansion,
    `find -delete`/`-exec`, `fd -x`, and a literal `sh -c` or `eval` body.
  - An executor is guarded only against writing into the brain. Everything
    else the account can reach — its home directory, other repositories, the
    network, `git push --force` — is open to it, unless it runs under
    `--sandbox` (L11). Of the brain's own `bin/` tools it may run the ones it
    needs to report: its own `hw` verbs, and `channel-send` only toward its
    own brainer; `hw` itself also refuses the brainer's verbs to an executor.
    The guard reads the command it is shown, so a script gets past it; `hw`'s
    check reads the executor's environment, so unsetting `HW_TASK` and
    `HW_RUN` gets past that one, and the guard refuses that spelling.
  - Letter case: on a case-insensitive filesystem (macOS APFS by default) a
    path is matched to a protected repository by its filesystem identity, so
    `~/Code/MyApp` is refused like `~/code/myapp`. A protected root that does
    not exist yet has no identity to compare, and a different-case spelling of
    it is compared as text only.
  - Very long commands: the guards stop unwrapping nested `env` wrappers after
    a fixed amount of work and refuse the command. A quoted command of about
    50 KB still takes the guard about 9 seconds to read (`measured`), well
    inside the hook timeout.
- **Evidence:** the shell and write-tool paths for Claude Code and OpenCode are
  `measured` by conformance and mutation tests plus live refusals; the Codex
  write-tool gap is `measured` as absent; the trust-hash behaviour is
  `unverified`. The reverse guard was probed live on Claude Code and on
  OpenCode executors, which refused writes into the brain, a `cd` into it,
  relative paths and a path read from an environment variable (`measured`).
  The relative-path, `cd`, variable, glob, brace and `find`/`fd` cases are
  `measured` by a vector suite that runs every case through the Python, the
  JavaScript and the Codex guard, each case red on the previous release's
  guards and green on these, and by mutation arms that turn each piece off.
  A malformed payload and a missing `python3` are refused (`measured`); input
  that is not JSON names no tool and is allowed.
  The case-variant identity check is `measured` on macOS (APFS) for all three
  guards. On Windows under Git Bash the guards do not use it: they fold case
  in the path's text instead (L1b). On a case-insensitive Linux mount the
  identity check is `unverified`.
- **Workaround:** keep protected repositories on a filesystem account or mount
  that the agent user cannot write, if you need a hard boundary.

### L23. Bitwarden secrets: the session token and the master password sit where the user's own processes can read them
- **Scope:** `secrets:` in a brief, `hw secret run|check|ensure`, `secret-unlock`
  and the global guard `deny-bitwarden-secrets.py` (Claude, Codex, OpenCode).
- **Impact:** the master password (`secret-unlock --setup`) is kept in the login
  Keychain as the service `hw-bw-master`, readable by any process of the user that
  passes the item's ACL, and a LaunchAgent unlocks from it at load and every 11 h.
  The guard cuts AGENTS (a Keychain read of it, `secret-unlock --auto|--setup`), not
  a process made on purpose. `~/.cache/hw/bw-session` (0600) holds a
  token that opens the WHOLE vault, not only the folder `agents`: `hw secret run`
  refuses items outside that folder, but a process running as the same user can
  read the file and ask bw for anything. The guard refuses `bw get|unlock|export|
  list|serve`, the file's name and `BW_SESSION` in an agent's command, and judges
  the command's TEXT: `B=bw; $B get x`, a script that calls bw, or a value already
  in a child's environment and printed on purpose (`hw secret run … -- sh -c 'echo
  $V'`) are not caught. Like L5 it is a tripwire for the careless, not a sandbox,
  and a quoted mention (a commit message spelling `bw get`) is refused too: write
  it through a heredoc. Codex and Claude settings carry the registration by hand.
  `hw next` re-tasks and the Go core (`HW_IMPL=go|shadow`, L16) do not run the
  `secrets:` preflight.
- **Evidence:** the resolver, the guard and the preflight are `measured` against a
  throwaway `bw` stub (tests 1009-1011); a run against a real vault is `unverified`
  here and belongs to the operator who unlocks it.
- **Workaround:** keep in the folder `agents` only what an agent may be trusted
  with, lock with `secret-unlock --lock` when done, and use the default 12 h expiry
  (`--hours N` shortens it).

## First run

### L6. Claude Code's first-run screens are yours
- **Scope:** the first `brain` or `hw` launch under a given Claude Code
  account.
- **Impact:** the welcome (theme, then login) and the Bypass Permissions
  warning cannot be answered by a script, and every brainer and executor starts
  in that mode, so each would stop on them. The installer detects and names
  them; it never answers them.
- **Evidence:** measured on a fresh macOS user.
- **Workaround:** run `claude --dangerously-skip-permissions` once, accept,
  `/exit`. `claude auth login` alone does not complete the welcome.

### L12. With `--permissions ask`, an unattended executor waits on its prompts
- **Scope:** any dispatch that resolves to `ask` (`hw --permissions ask`,
  `HW_PERMISSIONS=ask`, or `install.sh --permissions ask`).
- **Impact:** Claude Code and Codex are launched with their prompts on, and
  an executor's pane has no one watching it. `hw` has no detector for a Claude
  Code or Codex permission dialog: the pane stays where the dialog is until a
  person answers it in the pane. For OpenCode the existing state witness
  reports `blocked reason=permission`. Not measured live against Claude Code
  or Codex here: what the manifest says about waiting follows from what the
  flags mean, and the arguments were checked in `--dry-run`.
- **Workaround:** keep `skip` (the default) for unattended work, or answer the
  prompt in the executor's pane. `--permissions` cannot change a pane that is
  already running.

### L7. `hw … --dry-run` from a plain terminal stops at the return channel
- **Scope:** running `hw <lane> <task> --dry-run` outside a brainer pane, for
  example from the terminal the installer ran in.
- **Impact:** the dry run ends with `HW_INVOKER_PANE is UNRESOLVED` and no
  manifest. `--no-report` prints the manifest, at the cost of a dispatch that
  has nowhere to report.
- **Evidence:** measured (clean-home run of the export).
- **Workaround:** open the brainer with `brain <lane>` first, or add
  `--no-report` for a dry run.

## Memory

### L8. engram is optional for running and required for reports it registered
- **Scope:** `done-invoker` reports from executors that `hw` registered a
  memory session for (receipt `engram_session` is an `hw-…` id). A run without
  a registered session, and any `--blocked` report, is not held to it.
- **Impact:** without an `engram` server, `hw` registers no session: the
  harness runs, reports reach the brainer but no memory, and nothing is
  refused. With a registered session, a completion report that names no stored
  observation as `#<id>` under the lane's project is refused.
  A stray second `engram serve` (for example one started by an agent plugin
  under a throwaway HOME) can own the default port with the wrong store; `hw`
  and the invokers detect that by instance id and refuse to write to it, but ad
  hoc probes you run yourself are not protected.
- **Evidence:** measured.
- **Workaround:** run a probe with its own `ENGRAM_PORT` and `ENGRAM_DATA_DIR`
  under its sandbox.

## Tests

### L9. The suite runs live herdr, but never a real agent
- **Scope:** `setup/test-hw` and `setup/test-channel-send`.
- **Impact:** every subject but one runs under a HOME of its own with herdr
  stubbed and every dispatch a dry run. `setup/tests/650` drives the live path
  against a private herdr server: a real `hw` dispatch, the brief through
  `herdr agent prompt`, the Stop hook's turn tokens, `done-invoker` through
  `channel-send --report` into a brainer pane, and `hw done` closing the tab.
  The agent in both panes is a shell stand-in, so nothing about the Claude Code
  or OpenCode runtimes themselves is exercised: their startup dialogs, the
  session id herdr reads from them, or the screen herdr reads their state from.
  Without herdr installed, 650 skips and says so.
- **Evidence:** measured; 650 kills a dispatch that records the tab id it asked
  `layout.apply` for instead of the one herdr answered — `hw done` then reports
  "nothing to close" and leaves the executor's tab open — a defect no stubbed
  subject sees, because none of them parses a real `layout.apply` reply.
- **Workaround:** the demo lane (`examples/demo/`) exercises the real path once
  by hand. Before each release candidate is published the maintainer runs one
  end-to-end pass with a real herdr server and real Claude Code and OpenCode
  executors (ask, challenge and report through the return channel); that run
  is not part of this repository.

## Review

### L10. Judgment Day is optional, except for a brief that declares `boundary:`
- **Scope:** briefs with `boundary:` in their front-matter whose `kind:` is
  `build` or undeclared. For `explore`, `audit` and `review`, `hw` warns and
  records no gate.
- **Impact:** `done-invoker` refuses a completion whose `design-judgment.md`
  is missing, names another boundary, points at a changed design or does not end
  in `JUDGMENT: APPROVED`. Producing it honestly needs the judgment-day skill
  (`install.sh --with-judgment-day`); the check reads the file and cannot prove
  the judgment ran or that it preceded the code. Any other brief is never held
  to it.
- **Evidence:** measured.
- **Workaround:** leave `boundary:` out of a brief until the skill is active.

## Sandbox

### L11. `--sandbox` is macOS-only, opt-in, Claude Code and pi only, and leaves TCP open
- **Scope:** `hw <lane> <task> --sandbox`, or `sandbox: true` in a brief — the
  Seatbelt profile around one executor (see `THREAT-MODEL.md`).
- **Impact:**
  - **Platform and vendor.** Seatbelt exists only on macOS, and Apple marks
    `sandbox-exec` deprecated. Off macOS, and with OpenCode or Codex, the
    dispatch is refused before anything is built. OpenCode keeps the plugins
    other sessions run under `~/.cache/opencode`, so its profile would have to
    open a path this one refuses; neither vendor has been measured under it.
  - **Not confined:** the network (TCP, including every service on the host);
    `git push --force`/`--delete` (use server-side branch protection); reads
    outside `~/.ssh`, `~/.config/gh` and `~/.docker`; the keychain (not
    measured under this profile).
  - **Writable from inside:** the engram store (every lane's memory — the
    executor's report is a `mem_save`, and the engram MCP server opens the
    database directly), `/private/tmp` and `$TMPDIR` (shared with the
    operator's other sessions), `<workdir>/.hw/` (the run's own state), and
    the product repository's shared `objects`, `refs`, `logs` and
    `packed-refs`, which a worktree's commits need: a sandboxed executor can
    still move or delete any branch of that repository (`git branch -D`,
    `git update-ref`), though not its hooks or config.
  - **Chrome** needs `--no-sandbox` inside: a sandbox cannot start inside
    another.
  - **Breaks inside:** `ps` (setuid) is refused, so shell start-up files that
    call it print an error (measured); `git gc` (measured while the profile
    was designed) and anything else writing outside the worktree's slice of
    `.git`; a push over SSH; any tool that writes a
    shared cache under the home directory (package managers included) unless
    it is pointed at `$TMPDIR`.
  - **The stdin form** (`done-invoker -`, `ask-invoker -`) is refused
    inside: stdin does not cross the broker. Pass the text as the argument.
  - **Nothing removes a run's state directory** under
    `~/.local/state/hw-sandbox/` (a profile, a shim, a log: a few KB each),
    nor a task's arming record: a later launch of the same `<lane>:<task>`
    without `--sandbox` is revived with it.
  - **A report whose pane closes under it** is carried to the end by the
    broker, but its caller is gone, so the answer stays unread in the run's
    outbox (`<id>.out`/`.err`/`.rc`).
- **Evidence:** `measured` by a live end-to-end dispatch on the maintainer's
  machine (the script is not shipped: it runs real agents on real accounts) —
  a temporary lane, with a brainer and an executor in an isolated herdr
  session: brief read, workdir written, ten probes outside it (writes, the
  herdr socket, `~/.ssh`) refused, an ask answered back into the sandbox, a `mem_save` into a private
  engram, a `done` report delivered, and the broker winding down after the
  close. The profile, the refusal off macOS, and the outbox broker are held
  by the maintainer's test suite with mutants (not shipped). OpenCode and
  Codex under the profile: `unverified`. Chrome under it: measured while
  the profile was designed, not by this dispatch.
- **Workaround:** off macOS, run executors under a separate OS account or in a
  container if you need a hard boundary; for TCP, a host firewall.

## Cleanup

### L13. The background reap needs the lane's SessionStart hook, and closes only what is merged
- **Scope:** `hw reap --apply`, `hw done`, the brainer's SessionStart.
- **Impact:**
  - The background `hw reap <lane> --apply` and the list of tasks that never
    reported come from `setup/guards/lane_housekeeping.py`, called by the
    lane's SessionStart hook. The installer copies the module but does not
    register a SessionStart hook in a lane's `.claude/settings.json`; a lane
    without one gets neither, and `hw reap` stays a command you run.
  - `hw done` called by `done-invoker` runs after the invoker has exited, in a
    process of its own, so it now finishes (before 2026-10-14 it was killed by
    its own close and reaped nothing). The branch is usually not merged yet,
    though, so the worktree stays: a task is removed by `hw done` or the next
    background reap once it is merged and clean. A close that fails leaves
    `close-failed` and `close.log` beside the run's `done` marker.
  - What reap never removes: a dirty or unmerged worktree, one an agent sits
    in, a leased preview, and any ignored file outside `.artifacts`,
    `qa-report`, `test-results` and `playwright-report` (credentials, notes,
    exports). Those keep their worktree until you move them.
  - A branch merged by squash is removed with its worktree only when
    `git branch -d` agrees it is merged; otherwise reap says so and keeps it.
  - A database whose worktree is already gone is not dumped or dropped: with
    no worktree there is no merge to check.
  - The archive is never pruned.
- **Evidence:** `measured` by `setup/tests/640-la-mugre-no-se-acumula.sh`
  (old/new against the previous binary) and one run on a disposable clone.
- **Workaround:** register the hook (`.claude/hooks/session-start-*.py`
  calling `specialist_roster.main(<lane>)`), or run `hw reap <lane> --apply`
  after merging.

## Versions

### L15. Version floors: two of them are a baseline, not a measurement
- **Scope:** `install.sh --check` and `hw preflight`, for herdr, Claude Code, OpenCode,
  Node, gentle-ai, engram and jq. The floors are rows of `lib/hw/deps.conf`, the one
  place any script reads them from.
- **Impact:** a tool below its floor is refused (`MISSING … Install: <update command>`
  by `--check`; a `dep-floor` rule at level `block` from `hw preflight`, only for the
  tools the dispatch uses). `hw <lane> <task>` itself does not check them: the
  preflight is what a brainer, or the cockpit's rule band, runs first. Claude Code
  2.1.289, OpenCode 1.18.31, Node 22.7, jq 1.7 and gentle-ai 4.0.0 were already
  demanded by a script and moved there unchanged (gentle-ai is an exact pin: a newer one
  is refused too). herdr 0.9.1 and engram 3.0.0 are the versions that were installed when
  the manifest was written, because nothing had named a floor for them.
- **Evidence:** `measured` by `setup/tests/900-los-pisos-de-dependencias-viven-en-un-manifiesto.sh`
  (old and new preflight against a stub below the floor, five mutants). `unverified`: that
  herdr 0.9.0 or engram 2.9 fail; the two baseline floors may refuse a version that works.
- **Workaround:** run the update command in the message. To accept an older tool,
  lower its row in `lib/hw/deps.conf` (the install owns that file, and `upgrade` restores it).
- **Cache:** the version is remembered by the binary's path, mtime and size in
  `$XDG_CACHE_HOME/hw/deps` (default `~/.cache/hw/deps`). A binary replaced in place
  with the same size within the same second would be read as the old one. So would a
  launcher or version-manager shim (volta, asdf, mise, an npm launcher script) whose own file
  does not change when the tool behind it does: after upgrading through one, a preflight can
  keep reporting the old version until you delete that directory (`rm -rf ~/.cache/hw/deps`).
  `unverified` for any given tool; `measured` only on the direct binaries of this machine.


### L18. The incremental verdict trusts a marker-only runner change, and 540 may be skipped by two gate paths
- **Scope:** `setup/verdict-incremental` (a runner file whose diff is only `# MUTATION-ANCHOR: <id>`
  markers stays incremental) and `setup/gate-select` (`# gate-when:` on the slow subject 540).
- **Impact:** (1) the marker strip is textual: a mode-only change, a marker glued to code (`a#MARKER`), or one
  inside a `\` continuation or heredoc is not recognised as marker-only, so it falls back to a full run
  (safe) or, for a marker the strip wrongly reads as a comment, is trusted; 540 is forced into the plan
  for every dropped runner path, which catches an unregistered or duplicate marker in a file outside
  `setup/tests/`; 540's orphan scan skips `setup/tests/`, so an unregistered id added to `tests/_common.sh`
  or another non-subject file there is accepted until the registry is next checked. (2) `gate-select`
  returns `all` before its gate-when pass, so a `mutants-*`-only commit (the RUNNER regex) or a pre-commit `all`
  does not select the slow 540; a tsv-only commit gets it as `reference`, not as a suite.
- **Evidence:** `measured` by `setup/tests/820-el-veredicto-incremental.sh` (marker-only runner change forces 540;
  mutants 820-M04, M05, M06) and `setup/tests/999-*.sh`. `unverified`: that any real commit hits the
  `all`-before-gate-when path without also touching a path 540 names.
- **Workaround:** run `bash setup/tests/540-las-anclas-de-mutacion-existen.sh` (about 5 s) before pushing a change to
  markers or to `mutants-*`; the full suite still runs 540 on every release cut.

### L19. A chained incremental verdict compounds gate-select's blind spot, and fast gates outrank a full run
- **Scope:** `setup/verdict-incremental` + `setup/verify-for-push` (a green incremental run leaves its own record, so
  the next tree can start from it, up to `HW_VERDICT_MAX_CHAIN` = 3 hops off a real full run) and the pool in
  `setup/test-hw` (a fast gate's worker request outranks a full run's next subject and its timing subjects).
- **Impact:** (1) a job a diff reached only through a path built at runtime (gate-select's blind spot) keeps the
  row it had; with a chain that row can be inherited across up to three diffs instead of one. The release cut
  still takes ONE hop (`verdict-incremental chain`), so a release is never built on a chain. (2) While fast gates
  keep arriving, a full run starts nothing new whenever one waits for a worker and its timing subjects wait for a
  quiet moment, so the full run takes longer in proportion; nothing is killed. A fast subject that is already running
  is still waited for, and a full run's timing subject that is already running is too.
- **Evidence:** `measured` by `setup/tests/820-el-veredicto-incremental.sh` (chain section; mutants 820-M07, M08, M09) and
  `setup/tests/964-las-compuertas-rapidas-no-esperan-una-corrida-completa.sh` (R1-R3; mutants 964-M01..M03).
  `unverified`: how often the chain hides a real regression on production trees, and the full run's delay under
  a sustained queue of fast gates.
- **Workaround:** `bash setup/verify-for-push --full` runs every job and starts a new chain;
  `HW_VERDICT_MAX_CHAIN=1` restores one hop.
