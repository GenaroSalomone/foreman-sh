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
  `brew install --cask claude-code`. On Linux there is no native package. The
  piped installer is the way, and `--with-recommended` works only where
  Homebrew is on PATH. Without it, it names https://brew.sh and installs nothing.
  On macOS, `install.sh --check` does not name `rg`, `fd` and `sd`, which `hw`
  calls. The formula and `--with-recommended` install them.
- **Evidence:** measured on macOS with Homebrew 7.0.7. The formula passes
  `brew audit --strict`, installs from a local tap with `--build-from-source`,
  its test assertions hold, and the installed `foreman-sh --check` runs. The
  dependencies were not reinstalled, so that run used `--ignore-dependencies`.
  The published tap, a real `--with-recommended` run, and Linux with Homebrew
  are `unverified`: the suite drives `--with-recommended` against a stub `brew`.
- **Workaround:** the piped or cloned `install.sh`, as before; `--check` names
  every package with its install command.

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

### L11. `--sandbox` is macOS-only, opt-in, Claude Code only, and leaves TCP open
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
  - `hw done` called by `done-invoker` closes the executor's own tab and ends
    with it, so it reaps nothing; the branch is usually not merged yet anyway.
    A task is removed by the next background reap after its merge.
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
