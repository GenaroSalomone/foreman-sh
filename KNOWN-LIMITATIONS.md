# Known limitations — 0.1.0-rc.1

This is the register of what this pre-release does not do, or does only
partly. Each entry says its **scope** (where it applies), its **impact**, its
**evidence** (`measured` = observed by running it; `unverified` = believed, not
executed) and a **workaround** where there is one. Nothing here is a hidden
regression: a limitation that a change in this release introduced or widened
would not be listed as "inherited", and none is.

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
- **Evidence:** the setup suite on a GitHub-hosted `windows-latest` runner,
  herdr and the agents stubbed as everywhere in the suite, every subject file
  run one by one (survey mode). Git Bash: 188 of 188 pass, the guard's vector
  suites included, in 57 minutes. WSL2 (Ubuntu 24.04), same commit: 185 of
  188, and 207 passes since. The two left are not WSL2-specific: 69 needs the
  maintainer's live brain checkout above the tree and says so; and the
  OpenCode guard's vector suite, whose symlink cases point at product repos
  the runner does not have. That run found a real gap: the JavaScript half did
  not follow a symlink whose target is missing, so a write through it into a
  protected repository was allowed where the Python half refused it. This
  release closes it (the JavaScript half now follows the dangling link, with
  the Python half's verdict); the fix is `measured` on macOS; on Linux and
  Windows it is `(unverified)`. The export itself, installed on a
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
  (199: no zsh there, and herdr's pane shell is PowerShell). `unverified`: Claude Code's
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
  integration as well. Under an empty home directory that is exactly what
  `clean-install-check` meets. `measured`: the installer exits 1 with
  nothing written. Like Claude's, it is the person's step: run OpenCode once
  so its config directory exists, then `herdr integration install opencode`.
  `clean-install-check` performs both integration steps itself for that reason.

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

### L4. OpenCode background sub-agents are experimental
- **Scope:** OpenCode executors and brainers that spawn sub-agents in the
  background. The harness enables them with OpenCode's
  `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS` variable.
- **Impact:** the behaviour, and the variable itself, are OpenCode's and may
  change between its releases; the harness does not pin an OpenCode version.
  A background sub-agent's work is not something the harness's return channel
  tracks separately from its parent session.
- **Evidence:** the `background` option and its detach route were exercised
  against OpenCode 1.18.23 (`measured`). Whether the feature has left
  experimental status in a later OpenCode release is `unverified`.
- **Workaround:** dispatch without relying on background sub-agents; the
  foreground path does not depend on the variable.
- **Next decision:** revisit when OpenCode documents the feature as stable.

## Guards

### L5. The write guards are a tripwire, not a sandbox
- **Scope:** the read-only guard that stops a brainer writing into the
  repository of its lane (Claude Code hook + permission deny rules; OpenCode
  plugin; Codex hook), and the reverse guard that stops a product-lane
  executor writing into the brain.
- **Impact:** a guard decides from the tool call it is shown. It is built to
  catch a mistaken or careless write, including one written with an environment
  variable or an indirect path, but it is not a sandbox and does not contain a
  process that is trying to get around it. Specifically:
  - **Codex:** the guard covers its shell tool only. A write through Codex's
    own patch/write tool is unguarded, and the Codex configuration this harness was
    developed against runs it without a sandbox or approval prompts. Whether editing the Codex guard script
    re-raises Codex's trust dialog is unknown.
  - Where a write's destination cannot be resolved with confidence, the guard
    refuses on the command's content; it therefore may refuse a harmless
    command that merely names a protected path.
- **Evidence:** the shell and write-tool paths for Claude Code and OpenCode are
  `measured` by conformance and mutation tests plus live refusals; the Codex
  write-tool gap is `measured` as absent; the trust-hash behaviour is
  `unverified`. The reverse guard's live behaviour on OpenCode is [confirm at
  cut].
- **Workaround:** keep protected repositories on a filesystem account or mount
  that the agent user cannot write, if you need a hard boundary.

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

### L8. engram is optional for running and required for reports
- **Scope:** `done-invoker` reports from executors that `hw` registered a
  memory session for.
- **Impact:** without an `engram` server, the harness runs but reports reach no
  memory, and a completion report that names no stored observation is refused.
  A stray second `engram serve` (for example one started by an agent plugin
  under a throwaway HOME) can own the default port with the wrong store; `hw`
  and the invokers detect that by instance id and refuse to write to it, but ad
  hoc probes you run yourself are not protected.
- **Evidence:** measured.
- **Workaround:** run a probe with its own `ENGRAM_PORT` and `ENGRAM_DATA_DIR`
  under its sandbox.

## Tests

### L9. The suite is hermetic, and so it does not exercise live herdr
- **Scope:** `setup/test-hw` and `setup/test-channel-send`.
- **Impact:** every subject runs under a HOME of its own with herdr stubbed and
  every dispatch a dry run. A green suite proves the boundary each test
  observes, not that a real pane, agent and return channel worked.
- **Evidence:** by design.
- **Workaround:** the demo lane (`examples/demo/`) exercises the real path once
  by hand.
