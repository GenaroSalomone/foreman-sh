# Known limitations

This is the register of what this release does not do, or does only
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

## At a glance

Every entry, by section; the sections below give scope, impact, evidence and workaround.

**Platform**

- [L1. macOS and Linux only](#l1-platforms-macos-and-linux-only-windows-l1b)
- [L1b. Windows is not supported](#l1b-windows-is-not-supported)
- [L2. herdr is required](#l2-herdr-is-a-hard-requirement)
- [L29. The herdr app (herdr-core) carries turn-end tokens, the brainer and the executor launch](#l29-the-herdr-app-herdr-core-carries-turn-end-tokens-the-brainer-and-the-executor-launch)
- [L33. The herdr app: no script can press a mod button, a report card outlives its pane by about a second, and the QA stand-in hides three real behaviours](#l33-the-herdr-app-no-script-can-press-a-mod-button-a-report-card-outlives-its-pane-by-about-a-second-and-the-qa-stand-in-hides-three-real-behaviours)
- [L2b. Homebrew on macOS, a pipe elsewhere](#l2b-native-install-homebrew-on-macos-a-pipe-elsewhere)

**The Go core**

- [L16. The Go core and the tools still on bash](#l16-hw-brain-channel-send-and-the-invokers-run-in-go-with-no-switch-and-so-do-the-eight-small-tools-whatever-is-not-ported-stays-on-bash)

**Agents**

- [L3. Codex is a reduced dispatch target](#l3-codex-is-not-set-up-by-the-installer-and-is-a-reduced-dispatch-target)
- [L3b. pi: its guard reads text](#l3b-pi-is-a-dispatch-target-its-guard-reads-text-and---sandbox-is-what-contains)
- [L22. `plugins:` reach claude and pi only](#l22-plugins-loads-pinned-plugins-into-claude-and-pi-executors-only-and-the-export-ships-none)
- [L4. OpenCode background subagents are experimental](#l4-opencode-background-subagents-are-experimental)
- [L21. The cheap explorer is asked for, not forced](#l21-every-executor-is-handed-a-cheap-explorer-the-prompt-asks-for-it-nothing-forces-it)
- [L27. Lane-local executor rules reach Claude Code and pi only](#l27-lane-local-executor-rules-reach-claude-code-and-pi-only)
- [L14. `--sdd speckit` is Claude Code only](#l14---sdd-speckit-is-claude-code-only)
- [L17. A Claude Code session can show other lanes' changes](#l17-a-claude-code-session-started-before-basheditdiff-was-turned-off-keeps-showing-other-lanes-changes)
- [L20. `brain relaunch` and background tasks](#l20-brain-relaunch-moves-a-panes-background-tasks-out-of-the-session-the-resumed-claude-does-not-list-them)

**Guards**

- [L5. The guards are a tripwire, not a sandbox](#l5-the-write-guards-are-a-tripwire-not-a-sandbox)
- [L23. Bitwarden session token and master password](#l23-bitwarden-secrets-the-session-token-and-the-master-password-sit-where-the-users-own-processes-can-read-them)

**First run**

- [L6. Claude Code's first run is yours](#l6-claude-codes-first-run-screens-are-yours)
- [L12. `--permissions ask` makes unattended executors wait](#l12-with---permissions-ask-an-unattended-executor-waits-on-its-prompts)
- [L7. `--dry-run` outside a brainer pane](#l7-hw----dry-run-from-a-plain-terminal-stops-at-the-return-channel)

**Memory**

- [L8. engram is optional, and what is lost without it](#l8-engram-is-optional-for-running-and-required-for-reports-it-registered)

**Tests**

- [L9. The suite never runs a real agent](#l9-the-suite-runs-live-herdr-but-never-a-real-agent)
- [L24. A replayed suite job can miss an unread file](#l24-a-suite-job-whose-inputs-did-not-change-is-replayed-from-its-last-green-so-a-change-in-a-file-it-never-read-is-unseen-until-a-cold-run)
- [L26. Some claims have no Go mutant yet](#l26-some-claims-about-hw-brain-channel-send-and-the-invokers-lost-their-mutation-test-when-the-bash-left-and-have-no-go-mutant-yet)

**Review**

- [L10. Judgment Day is optional, except for `boundary:`](#l10-judgment-day-is-optional-except-for-a-brief-that-declares-boundary)
- [L34. `hw review --at <sha>` on an already-merged commit measures against the current base, counts 0 code lines and dispatches nothing](#l34-hw-review---at-sha-on-an-already-merged-commit-measures-against-the-current-base-counts-0-code-lines-and-dispatches-nothing)

**Sandbox**

- [L11. `--sandbox` is macOS-only and opt-in](#l11---sandbox-is-macos-only-opt-in-claude-code-and-pi-only-and-leaves-tcp-open)
- [L31. Security-audit leads left open](#l31-security-audit-leads-left-open-every-agent-runs-as-the-same-uid-so-the-guards-are-behavioural-and---sandbox-is-the-only-enforced-boundary)

**Cleanup**

- [L13. Background reap needs the SessionStart hook](#l13-the-background-reap-needs-the-lanes-sessionstart-hook-and-closes-only-what-is-merged)

**Versions**

- [L15. Two version floors are a baseline](#l15-version-floors-two-of-them-are-a-baseline-not-a-measurement)
- [L18. The marker-only strip and 540's orphan scan](#l18-the-marker-only-strip-pays-a-full-run-for-the-shapes-it-does-not-parse-and-540s-orphan-scan-stops-at-a-subjects-own-body)
- [L19. A chained incremental verdict](#l19-a-chained-incremental-verdict-compounds-gate-selects-blind-spot-and-fast-gates-outrank-a-full-run)
- [L25. `setup/heavy` reports a frozen run, it does not unfreeze it](#l25-setupheavy-reports-a-frozen-or-starved-run-it-does-not-unfreeze-it)

## Platform

### L1. Platforms: macOS and Linux only (Windows: L1b)
- **Scope:** every command.
  - **macOS:** developed and tested here.
  - **Linux:** works, not verified per release: a release stands on the macOS fast gate and the real end-to-end run, and the Linux suite is run when the maintainer has the time. The installer accepts it and the whole test suite has run green in a
    Debian container (bash 5, git, jq 1.7, python3, Node 22, sd, fd, rg).
    Not exercised: a live herdr, a live Claude Code or OpenCode agent, and a
    real desktop session. Subjects that need the maintainer's product-repo
    checkouts skip themselves by name. There is no keychain backend:
    `hw handle` says so and exits 2, and a brief that names a handle is
    launched with that check skipped and a warning that the handle was not
    verified. `jq` older than 1.7 (Debian 12, Ubuntu 22.04 ship 1.6) is refused
    by the installer, because 1.6 reports empty input as success under `jq -e`.
  - **Windows:** not supported (L1b).
- **Impact:** on Linux, first-run behaviour with a real herdr and a real agent is
  unmeasured; expect to be the first to find what a container cannot.
- **Evidence:** macOS and Linux (container) `measured`.
- **Workaround:** none on Windows (L1b).

### L1b. Windows was removed in 0.7.0
- **Scope:** Windows, native, under Git Bash or under WSL2.
- **Impact:** the Windows code was deleted before the 0.7.0 freeze: the
  `_windows.go` build files, `bin/msys-compat.sh` and its source lines, the
  native-Windows mutation exemption and the Windows CI workflow. There is no
  Windows path to maintain. WSL2 is a Linux, so L1's Linux path may work
  there; it is `unverified`.
- **Evidence:** none kept current; the last Windows CI job (0.4.0-rc.4) was red.
- **Workaround:** use a macOS or Linux machine.

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

### L29. The herdr app (herdr-core) carries turn-end tokens, the brainer and the executor launch
- **Scope:** an executor running in an herdr-core (`hc`) terminal, as the herdr app starts one, and `brain <lane>`
  run from that terminal; a TUI brainer is unchanged. The app is inside the [1.0 promise](REFERENCE.md#what-10-promises): what this limit lists is what it does not cover yet, and changing its interface after 1.0 takes a major version.
- **Impact:** this release publishes the executor's turn-end and invoker state as `hc` labels (`hw_turn_state`,
  `hw_turns`, `hw_done_state`, …), proved by an `hc get --json` read-back, and chooses the backend by the nearest
  `herdr`/`herdr-core` process ancestor, so an app terminal that inherited `HERDR_ENV`/`HERDR_PANE_ID` from whoever
  launched the app never writes onto that pane. `brain <lane>` in an app terminal now opens the brainer there — an hc
  terminal labelled `label=brain` in the lane's brain directory, opted into the window with `herdr.space` — and `hw`
  resolves `HW_INVOKER_PANE` to it (`hc ls`/`hc get`, normalised into `herdr pane list`'s shape). `hw` run from an app
  terminal now launches the executor there too: one `hc run --bg --no-parent --no-notify` terminal named for the task,
  labelled `hw_project`/`hw_task`/`hw_run` and `herdr.space=<the lane's space>`, with the herdr variables unset and the
  brief as claude's launch task (proved by `hc get`: `status=working` or a turn past 0); a second dispatch of the same
  task finds it by `hc ls --label hw_project= --label hw_task=` and launches nothing. Claude executors on every lane are launched, the lanes with a dev server or a
  database included: the worktree, ports and per-worktree database are built as from a TUI pane, and each dev server is its own
  `hc run` terminal, rendered from the lane's layout (cwd and env), named `<task>-<role>`, carrying the task's
  `hw_project`/`hw_task`/`hw_run` plus `hw_role=<the pane's label>`, so `hw done` and `hw reap` end it with the executor and
  nothing takes it for the executor (a task's executor is the terminal with no `hw_role`: `hc ls` carries no labels, so dispatch
  and `hw revive` read one `hc get` per terminal; a dev terminal already live for the task is kept, not doubled). A read-only
  dispatch (`--worktree none --no-db`, or a lane whose dev server is opt-in, without it) starts none. `--agent pi`, `opencode` and
  `codex` launch the same way, each as its own binary with the brief as its launch task (claude, pi and codex after `--`, opencode as
  `--prompt`), measured 2026-11-12 on the app: the brief arrives, `hw status` sees the terminal, `done-invoker` reaches the brainer and
  `hw done` closes it. What differs from claude: pi and codex record no session id (only claude writes `sessions/<pid>.json`), opencode's
  is read from its own server (`--port`), which is also what proves its brief arrived (it reports no status for `hc get`); codex stops at
  its own first-run "Hooks need review" dialog until a person clears it, as on a TUI pane (`hc read <terminal>`); pi's turn-end publishes
  through its extension, which needs a model that answers (all of pi's `-free` models answered 403/410/400 that day, with or without the app);
  opencode's `hw-turn-ended.js` plugin only publishes turn tokens on the app once the installed plugin is this tree's. `--here`
  is refused with its reason (hw starts an app executor as its own `hc run` terminal and has no pane of the caller's to split; `--worktree none --no-db` is the same no-worktree run in a tab), and the dry run prints it as a ✗ (one function, `hcRefusal`, which the launch asks
  again). `--sandbox` is launched (L11): the run is armed before `hc run`, the shim's directory is the first entry of the terminal's `PATH`,
  the broker's heartbeat is the proof, and a terminal with no heartbeat inside 10 s is killed (`hc kill`) and the dispatch dies; `hw revive`
  arms a sandboxed run again the same way. Inside the profile the core's socket is denied like herdr's, so a sandboxed executor on the app
  publishes no turn-end label and each turn end waits out `hc label`'s 10 s (measured 2026-11-12: `hc ls` inside answers
  "core did not stop or start within 15 s"); its reports and asks cross through the broker; the cheap explorer's arming is skipped (claude's `--agents` JSON), and the receipt records no session id for claude-less vendors. A dev server runs as `sh -c <cmd>` with the dispatching shell's `PATH`, not the
  user's interactive shell: a tool only an interactive profile puts on `PATH` is not found, and the server's terminal ends when its
  process exits (`hc read <terminal>` while it is listed, `hc ls --all` after). `hw done` of that run kills and removes its terminals by label (`hc ls --label hw_project= --label hw_task=`, then `hc kill` and `hc rm`: a terminal with other labels is never touched; dispatch records the run as `.hw/<run>/hc`, and a run without that record makes no `hc` call), the in-use guard reads the terminal's status and report labels from `hc get`, and `hw reap`/`hw status`/the worktree occupancy count an hc run's live terminals as occupants of its worktree (an unreadable `hc ls` is "unobserved", never "free").
  `channel-send hc <terminal> - <message>` delivers to an hc terminal (a brainer `brain <lane>` opened in the app, or an
  executor): redirects wait for the receiver's `idle`/`done` status with `hc wait`, `--report` and a held `--ruling`
  reply do not, `hc send` is never given `--if` (it refuses a finished Claude) or `--force` (it may answer a dialog), and
  there is no raw-keystroke fallback for a blocked receiver (exit 1, a person's prompt). Delivery is proven from the
  receiver — its status word, its `turn`, or the turn-end labels moved after the send — so a send nothing confirms inside
  `HW_HC_PROOF_MS` (15 s) is exit 5, and a receiver whose turn ends in `error` is exit 4. `done-invoker` and
  `ask-invoker` use it whenever `HW_INVOKER_PANE` is an hc address (no `:`) and they run on herdr-core; an executor on
  the app with a TUI brainer keeps the herdr route. `ask-invoker` works on hc: its hold records route `hc` and the
  terminal's own address, which turn-end and `holdfacts` check. `hw ruling <terminal>` reads an hc executor with `hc get`
  and queues; the turn-end hook delivers, as on herdr. `hw next <terminal>` works on hc: the gate reads `hc get`
  (idle/done opens it, blocked is a person's prompt, anything else waits with `hc wait` up to the budget), keys go through `hc keys`, and the
  task is delivered with `channel-send hc`; there is no endpoint or screen witness for an app terminal, so the "settled, no
  budget helps" refusal does not exist there. `hc wait`'s own exit code is not interpreted (its meanings are unmeasured): a failed wait backs off a
  second and `hc get` decides. `hw outbox flush <lane> --to <terminal>` delivers with the `hc` vendor when the target is an hc address, and
  `brain <lane>` in the app runs it once a started brainer has reported a status; a report that lands that way also moves the executor terminal's labels to `hw_done_state=delivered` and `hw_invoker=<the new brainer>`, or the cockpit would say "report undelivered" to a dead brainer for ever. The brainer has a cockpit writer (`cockpit-state --loop 5 --bind-pane --invoker <terminal>`), which also removes the state files of writers that did not leave (older than an hour, loop lock free). `brain <lane> --reset` and `--resume <id>` replace the brainer by `hc kill` + `hc rm` and a new `hc run` (a working brainer is refused unless `--reset --force`); `brain relaunch [--all|<lane|terminal>…] [--dry-run]` does that for each idle claude brainer terminal, reading its conversation from claude's own `<config dir>/sessions/<pid>.json` (the pid is `hc get`'s) and skipping by name a terminal whose session it cannot read. `brain <lane> --reset --self` works in the app since 2026-11-12: the brainer arms its own reset, a detached process (`--reset --await-idle <terminal>`, in a session of its own) waits for that turn to end and then ends the terminal and starts a new one, so the brainer's address changes and the lane's open runs are repointed to it (measured: a process that called `setsid` survives `hc kill` of the terminal it was started from). `--in-pane` types into the shell under a herdr pane and stays refused, with that reason and the app's own command (`brain <lane> --resume <id>`, `brain relaunch`); `brain relaunch` in the TUI does not see a brainer living in the app. hc labels do
  not expire (no TTL) and `clear` is not applied on hc. `hw` puts `brain/bin` first on the `PATH` of the brainer and the executor it starts in an app terminal (the caller's `PATH` follows it), so `hw` and `brain` resolve there. The read side sees app executors: the cockpit state (`bin/cockpit-state`) and `hw status` list
  the live `hc ls` terminals that carry an `hw_task` label (hc 0.1.19 refuses a bare `--label hw_task`, and `hc ls` carries no labels, so each costs one `hc get`) beside herdr's panes (status and turn from `hc get`), `state-witness` reads
  `hc get`/`hc read` when herdr knows no such pane, and `stall-watch` asks `hc get` whether the terminal is live. Its alert and `setup/heavy`'s FROZEN alert name the brainer as
  `channel-send --report herdr "$HW_INVOKER_PANE"`: `channel-send` sends that through `hc` when the pane is an app terminal and the caller runs on herdr-core
  (measured live 2026-11-12; before it every such alert ended in a log as "did not reach pane t24 (channel-send exit 1)"); `hw`
  looks for and focuses no herdr tab or space on the app route. A cockpit row of an app terminal has no statusline context (it falls back to the
  transcript); `hw status` reads an app executor's context from its screen with `hc read <terminal> --lines 40`, as `hw receipt` does, names a
  finished app run's terminal ("app terminal t4 closed") from the run's `.hw/<run>/hc` record, and tells you to read a stalled one with `hc read`.
  A `--keep-pane` lease on the app records the terminal that started it (`by=`), and its expiry ends the run's terminals by label (`hc kill`, `hc rm`),
  as `hw done` does (before 2026-11-12 it recorded `cleanup=no-owned-container` and left the terminal open). `hw wait <terminal>` reads the app terminal with `hc get`/`hc read`/`hc wait` (it ends at the turn boundary, and its non-blocking form starts `hw wait-monitor` in an `hc run` terminal of its own instead of a herdr tab (its screen stays readable with `hc read` after it exits, and the terminal removes itself `HW_WAIT_MONITOR_KEEP_S` seconds, default 300, after its last verdict); with no process-info endpoint for an app terminal its cross-check says "unknown", never "settled"), `hw unstick <terminal>` reads it the same way and refuses a non-OpenCode terminal by name as on herdr; restarting a stuck OpenCode inside a kept hc terminal is `unverified` and refused by name, with `hc kill` and `brain <lane> --resume` as the way out. `hw done` and `hw reap` judge a worktree whose run is recorded as hc by `hc` alone when no herdr server is running (`server_not_running`); a worktree with no hc record keeps the refusal.
  **Still open on the app (inventory of 2026-11-12, each measured on an isolated instance or read at its line):** `hw sweep` lists and closes only herdr
  tabs and workspaces, so it answers "no run recorded a workspace id" for app runs (`hw done` and `hw reap` are what end an app run's terminals);
  the vault alert (`hw secret`) finds the lane's brainer with `herdr pane list` and runs from a LaunchAgent with no app terminal in its environment, so it
  reaches a TUI brainer only (the alert stays in `vault-alerts.log` and on `hw status`); `hw revive --help` still says "in a new tab"; on a machine
  with no herdr server `hw status` opens with "herdr unreachable" and ends with the `herdr update --handoff` tip, which do not apply there; the
  unreported-executors refusal of `brain --reset` says "the pane id SURVIVES a reset", and on the app the brainer's address changes (the runs are
  repointed instead); `hw ruling` says "herdr agent_status=" for an app terminal's status; `hw next` records `consumed_by=unknown` on the app; the detached arm of `brain --reset --self` does not re-read which terminal is the lane's brainer after its wait (the TUI arm does), so a brainer opened by hand during that wait can end up beside the new one.
- **Evidence:** `measured` — `hc label`/`hc get` round-trip, the Stop hook firing in an hc terminal, the ancestry rule
  (design 0b P5, P7, P9, P12, P14; setup test 1261, the cockpit mod 1272), and the live brainer in the app shown by `hc ls --label
  label=brain`, `hc get` and `herdr-app ls`, with `hw … --dry-run` printing `invoker t1b (resolved by env; verified as
  label=brain cwd=…)` (setup tests 1262 and 1263), the launch argv and the dry-run route (core `TestHCDispatch*`), and the `hc` route, the invokers' reports, the ask hold and `hw ruling`
  against a fake `hc` that records argv (setup test 1264; core/invokers `TestHC*`), the read tools by a fake `hc` (core `TestHCRun*`, `TestHCDispatchNeverFocusesAHerdrSpace`, setup tests 1278 and 1266), and `hw next`, the outbox flush and the wait backoff the same way (setup tests 1279, 1262), and `hw done`/occupancy against a stateful fake `hc` (core `TestDoneRunEndsTheHCTerminal`, `TestHCOccupancy`; red on the old code). Against a live core (herdr app 0.1.14, hc 0.1.19, isolated instance, real claude haiku, main e0ff921f1, 2026-11-10; the fifth live-cycle QA): `brain <lane>` in an app terminal, `hw` dispatch with the brief proved by `hc get`, `hw ruling` delivered mid-turn by the PostToolUse hook, `ask-invoker` and its reply, `done-invoker` reaching the brainer, `hw done` of a kept terminal, `hw next` on a `--keep-pane` run, the outbox flush on reopening the brainer, `hw wait` (blocking and non-blocking), `brain --reset`/`--resume`/`relaunch` and the cockpit, Issues and Artifacts panels all `measured` live. The four suspicions this entry once listed were refuted there: the brainer's `PATH` starts with `brain/bin`, `hw wait`/`hw unstick` work on hc (`hw unstick` refusing a non-OpenCode terminal by name), the brainer has a cockpit writer, and `--reset`/`--resume`/`relaunch` restart it. The sixth live-cycle QA (main 253a6eadd, real claude haiku, 2026-11-10) repeated all of it and measured `hw revive` of an app run live too. Not measured live: the Artifacts panel (the isolated home has no signed-in claude), and the remaining slices (design §5 and §6b).
- **Workaround:** keep in the TUI what the app refuses by name (`--here`, and the `brain` flags that type into a pane). Do not start the app from inside a herdr pane **or an hw executor's shell**: the core gives
  every terminal the launcher's environment until it restarts, and that is herdr's variables (`HERDR_ENV`, `HERDR_PANE_ID`, …) AND the executor's
  `HW_*` (measured: `HW_INVOKER_PANE`, `HW_RUN`, `HW_TASK` in every app terminal). Start the app with `env -i` and only what it needs
  (`herdr-app/README.md`); a brainer `brain <lane>` opens there starts without the run identity (`HW_RUN`, `HW_TASK`, `HW_PROJECT`, …).
  The cockpit mod reads only under the app's own `$HOME`, links resolved, and `mod.json` cannot widen that: keep the work root under `~`.

### L2b. Native install: Homebrew on macOS; a pipe elsewhere
- **Scope:** `brew install GenaroSalomone/tap/foreman` and
  `install.sh --with-recommended`.
- **Impact:** the formula cannot depend on Claude Code, which is a cask, so
  Claude Code is one more step: `foreman-setup --with-recommended` or
  `brew install --cask claude-code@latest`. On Linux there is no native package. The
  piped installer is the way, and `--with-recommended` works only where
  Homebrew is on PATH. Without it, it names https://brew.sh and installs nothing.
  On macOS, `install.sh --check` does not name `rg`, `fd` and `sd`, which `hw`
  calls. The formula and `--with-recommended` install them.
- **Evidence:** measured on macOS with Homebrew 7.0.7. The formula passes
  `brew audit --strict`, installs from a local tap with `--build-from-source`,
  its test assertions hold, and the installed `foreman-setup --check` runs. The
  dependencies were not reinstalled, so that run used `--ignore-dependencies`.
  A real `--with-recommended` run and the published tap were measured on a clean macOS 26.6
  VM (tart, no Homebrew, no Command Line Tools, empty HOME), through `init` in one run:
  Homebrew + CLT 145 s and 367 s (two runs, network-bound), `brew install foreman`
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

### L16. `hw`, `brain`, `channel-send` and the invokers run in Go, with no switch; and so do the eight small tools; whatever is not ported stays on bash
- **Scope:** every `hw` verb, `brain`, `channel-send`, `ask-invoker` and `done-invoker`, with no switch to bash (0.6.0 removed it); and eight small tools — the five wrappers, `hw-sandbox`, the Stop hook and `hw secret` — whose bash was removed in 0.7.0, so they run Go with no switch either. macOS and Linux only (L1, L1b).
- **Impact:**
  - **There is no switch.** `hw`, `brain`, `channel-send`, `ask-invoker` and `done-invoker` run
    `hw-core`, and 0.6.0 removed the bash that `HW_IMPL` used to choose. `HW_IMPL`,
    `HW_IMPL_<VERB>` (or `HW_IMPL_BRAIN`, `HW_IMPL_CHANNEL_SEND`, `HW_IMPL_ASK_INVOKER`, `HW_IMPL_DONE_INVOKER`) set to `bash` or `shadow`, and the same words in
    `~/.config/hw/impl` (`default=bash`, a bare `bash` line, `<verb>=bash`), are read only to say
    so: one line per state, "bash was removed; running Go — delete this line", and Go
    runs. It never stops the command: a hook calls `hw` every turn. `hw shadow` is gone (it
    says so and exits 64). A `HW_IMPL_<TOOL>=bash` or `<tool>=bash` line for one of the eight
    small tools below is retired the same way since 0.7.0, and Go runs. The way back to bash is
    `foreman-setup upgrade --to 0.6.3` (for `hw` itself, 0.5.0 was the last release with its bash). `bin/hw` is a shim that finds `hw-core` and execs it:
    it has no bash body, and a lane build hook outside `lanes/` is refused (hw used to source such
    a file into its own bash, and that bash is gone; a builder is Go now). `brain`, `channel-send`,
    `ask-invoker` and `done-invoker` are shims too: each keeps its name and execs its Go binary.
    The internal seam that ran a bash body (`HW_BASH_BODY=1`) is gone with them.
  - **What runs in Go.** `install.sh` builds `hw-core` with Go 1.22 or newer (required) and writes
    no switch file, since Go needs none (an install that builds a core removes the `default=bash`
    file an older `--no-core` install wrote, when it is byte for byte that one; any other existing file
    is left as it is; a `bash` line in it draws the one-line warning above at run time). A `hw-core` that is missing stops the tool, naming `setup/build-core --native`;
    a stale one is rebuilt when Go is on PATH. The ported verbs run in `hw-core`:
    `receipt`, `log`, `handle`, `help`, `worktrees`, the table of `status`, the
    read half of `reap`, `sweep` and `reports`, `review-range`, `review-delta`,
    `preflight --json`, `outbox list`, the `wait` family, the marker writers
    (`ruling`, `human-boundary`, `executor-turn-end`, `chaining-lease-start` and
    `-expire`, `outbox flush`, `output`, `stage`) and the dispatch
    (`hw <lane> <task> …`, its dry run and its launch, `--agent pi` included), and the large writers: `done`,
    `next`, `revive`, `preview`, `reap --apply` and `sweep --apply`. Anything else is bash, and each group that stays is named, with its reason, in "What stays bash, and why" below.
  - **What stays bash, and why.** A file stays only for a reason that is not "it works"; one line per group:
    - **Launchers** (`hw`, `brain`, `channel-send`, `ask-invoker`, `done-invoker`, `hw-stop-hook.sh`, `hw-secret`, `hw-sandbox`, `browser-close`, `brain-archive`, `secret-unlock`, `opencode-auto`, `codex-channel-session-hook.sh`, `stall-watch`, with `core-rebuild` and `impl-retired.sh`): each finds or rebuilds `hw-core` and execs it, so it has to run before `hw-core` exists. `stall-watch` is one now: its body moved to Go.
    - **Installers and bootstrap** (`install.sh`, `scripts/`, `lib/hw/deps.sh`, `setup/build-core`, `setup/check-machine`, `setup/install/`): they run before Go exists or bring it.
    - **Git hooks** (`setup/hooks/`) and `setup/commit-tree`: git runs the hook file itself in clones where `hw-core` may be unbuilt, and they are the gate over `hw-core`'s own changes.
    - **The suite runner and its helpers** (`setup/test-hw`, `setup/heavy`, `bin/suite-gate`, `bin/invoker-common.sh`, the mutation, measurement and release tooling): they build and test `hw-core`, so they cannot depend on it; `bin/invoker-common.sh` is sourced only by the test harness. `hw status` no longer runs `bin/suite-gate`: the slots line is read in Go.
    - **Glue to an external CLI** (`gh-attach`, `herdr-isolated`, `statusline-usage-tee`, the Bitwarden MCP starter): the work is the external tool's, and Go would add a rewrite and nothing else.
    - **`bin/lint-shell`** lints the bash that stays; it has no Go consumer.
    - **Lane selectors** (the per-lane build files under `lanes/` other than `git-worktree.sh`): a name and a header, no body. `projects.json` `build` names the file and `hw-core` reads its basename (`core/t5l_lanes.go`) to pick the Go builder; the bash that used to be there was a dead copy. `lanes/git-worktree.sh` keeps its body: `install.sh --lane` writes it into a new brain before Go exists.
    - **`bin/state-witness.sh`** has no runtime consumer left (every gate, `hw wait`, `hw next`, `channel-send` and now `hw-reconcile` run the witness in Go); it stays as the **oracle** the Go witness is held to: `core/witness_parity_test.go` and the subjects 30, 203, 33 and 1266 run it beside the Go on the same screens, and `setup/guards/mutate-state-witness.sh` mutates it. Deleting it means freezing those answers as goldens first, as `core/brain_parity_test.go` did for the 0.5.x brain; that is its own change.
    - **Not ported yet, and said so:** one Go call site that still runs `bin/project-spaces.sh`: `hw-core brain` asks the loader for the words of a lane table that does not load (jq's own diagnostic, which differs from one jq version to the next and is pinned by `TestBrainParityD3LaneTableDiagnostics`). `done-invoker` and `ask-invoker` no longer run it: they read the lane table themselves (`core/invokers/lanetable.go`). The file itself stays for `install.sh` and `scripts/init.sh`, which read the table before Go exists. Each of the rest is a duplicate of Go that is waiting for its own change, not a decision to keep bash.
  - **The Stop hook** (`bin/hw-stop-hook.sh`, run at every turn end of every wired
    Claude Code and Codex session) runs in Go by default (0.6.0): the registered script stays bash and execs
    `hw-core stop-hook`, so no hook configuration changes on upgrade. It runs the turn end in
    process instead of starting a second `hw`. 0.7.0 removed its bash body: a `HW_IMPL_STOP_HOOK=bash`
    or `stop-hook=bash` line is said once and never selects anything. A `hw-core` that is not built
    names `setup/build-core --native` and the hook exits 0 — it must never break a session. The Go
    does not need `python3` or `jq` on PATH.
  - **`brain`.** `bin/brain` runs in Go (`hw-core brain`). Where the Go answers differently from the bash of 0.5.x on the same input, the
    difference is one of two, both pinned by `core/brain_parity_test.go` (frozen goldens of the 0.5.x bash, which no longer ships): a
    projects.json that does not load prints the words of the bash loader (jq's,
    which differ between jq versions) because `hw-core brain` asks `bash` and
    `bin/project-spaces.sh` for them — without them, Go's own wording; and a
    `bin/hw` or a `herdr` that is missing or not executable makes the bash print
    its shell diagnostic (`…/brain: line N: …: Permission denied`) into the
    message of the step that failed, where Go prints an empty line (the exit
    status and every other byte are the same).
  - **No Go, or a Go older than 1.22:** `install.sh` refuses before writing anything, and `--no-core`
    is refused too (removed in 0.6.0). hw-core is required: there is no longer a Go-less install.
  - **Eight small tools run in Go, with no switch (0.7.0).** The five wrappers below, `hw-sandbox`, the Stop hook and
    `hw secret` are shims that exec their `hw-core` verb. Their bash bodies are gone, and `bin/wrapper-go.sh` with them:
    `HW_IMPL_<TOOL>=bash` or a `<tool>=bash` line in `~/.config/hw/impl` is read only to say, once per state, that bash
    was removed and to run Go; it never stops the command. The way back for one of these tools is
    `foreman-setup upgrade --to 0.6.3`.
  - **Five small wrappers run in Go.** `browser-close`,
    `brain-archive`, `secret-unlock`, `opencode-auto` and `codex-channel-session-hook.sh`
    are shims that exec `hw-core <tool>`; the transcripts the bash left behind
    (`core/testdata/wrappers/`) hold the Go to what it answered (`setup/tests/1027`).
    A `hw-core` that is not built names `setup/build-core --native` and exits 1; one older than
    `core/` is rebuilt by `bin/core-rebuild` when a toolchain is on PATH. `hw-reconcile` is a shim too
    (`core/wrappers_reconcile.go`; nothing on the runtime path calls it; it needs `hw-core`, which the bash did not, lists its findings by directory name and words a lane table that does not load in its own words). Three differences, all at the edge of the shell:
    a failed `exec` is worded as bash 3.2 words it (other bash versions word it differently), the
    hook's sequence number is the clock in nanoseconds with no `python3` needed (the bash exits 0
    without one), and `jq`'s printing of a non-string `session_id` is imitated for strings,
    numbers and booleans only.
  - **pi runs in Go too.** `--agent pi` dispatches (dry run and launch) are in
    `hw-core` under `go`, held byte for byte against the bash by the `pi-*`
    goldens; nothing falls back to bash and nothing says so.
  - **Lane hooks.** The export carries the builders of git-worktree lanes only. A
    lane `build` hook of any other name is refused before anything is created: `hw` has no bash left to source it into
    (dispatch with `--worktree none` for a plain work directory).
  - **`hw secret` / `secret-unlock`.** `bin/hw-secret` (and `bin/secret-unlock`, which execs
    it) runs in Go (`hw-core hw-secret`) with no switch since 0.7.0. `put` was ported in 0.7.0 too,
    so the one writer of the vault is one implementation. The Go answers the same command line with the
    same stdout, stderr, exit status and files (`setup/tests/1077` diffs 84 scenarios
    against the bash's recorded answers; `1009` and `1012` drive the real tool through its shim) and
    keeps the guard contract (`1010`): `hw secret run <item>[.<field>] --as VAR -- <cmd>`
    is the one road to a value, `BW_*` never reaches the child, and no value is printed,
    written or put in argv. Two differences, both on purpose or unavoidable: the Go reads
    bw's JSON itself, so it works on a machine with no `jq` (the bash dies with
    `jq is required`); and `exec: <cmd>: not found` has no `<script>: line N:` prefix
    in Go, nor does the diagnostic for a command given as a path that cannot run (the bash's
    wording and status for that are its version's: 3.2 and 5 differ; Go says
    `cannot execute: <why>` and exits 126, or 127 for a missing file). A leading-zero `--hours` (`010`) is read as decimal in Go and as octal by the
    bash's arithmetic, and a value that is a JSON object or array prints with jq's
    indentation but not its number normalisation. The 17 mutation anchors of the Go are
    killed by `setup/tests/1078`.
  - **A missing or stale `hw-core`** (absent, or older than `core/`'s sources)
    gives bash and one warning, never a failure.
  - **The state witness is Go.** The cross-check every gate runs before it gives up
    (`witness_state`, `witness_wait`) and the ruling queue `done-invoker` claims
    (`ruling_queue_claim_all`) run in the Go core with no `bash -c` and no
    `bin/state-witness.sh` beside the binary; a `brain/bin` without that file no
    longer refuses a gate or degrades `channel-send` and `ask-invoker`. The file stays
    in `bin/` (its bash is still run by tests and by the stop hook's bash path; the four tools' bash bodies are gone since 0.6.0). The Go core carries the witness
    in more than one place (`hw wait`, `hw next` and `channel-send` each have a copy
    that answers the same bytes); folding them into one function is not done.
  - **`channel-send` and its suite.** `channel-send` runs in Go. Its subject
    (`setup/tests/05-channel-send.sh`) sets `WITNESS_SLICE_MS` large because, against a stub `herdr-rpc` that answers
    `wait-agent` at once, the Go state witness spins; the cause is not established.
  - **Bash is not entirely gone from the tree.** The bash bodies of `hw`, `brain`, `channel-send`, `ask-invoker` and
    `done-invoker` are gone (each is a shim that execs its Go binary), and so are `lib/hw/{status,done,next,reap,briefs,ledger,explorer}.sh`;
    `bin/invoker-common.sh` is left in `bin/` with no program of the four tools reading it, kept by the tests that still source it
    (101, 798, 81). A fix to a ported verb goes to `core/`.
- **Evidence:** `measured`: the golden contract compares stdout, stderr, exit status and the
  resulting tree byte for byte; the binary is reproducible (the same sha256 on two builds).
  Windows was removed in 0.7.0 and no Windows binary is built (L1b).
- **Workaround:** none inside 0.6.0: `hw-core` is required (`setup/build-core --native`, Go 1.22 or newer).
  To get the bash back, install the previous release: `foreman-setup upgrade --to 0.5.0 --brain <brain>`.

## Agents

### L3. Codex is not set up by the installer, and is a reduced dispatch target
- **Scope:** `--agent codex`. **Deprecated** since 1.0: outside the [1.0 promise](REFERENCE.md#what-10-promises), it keeps working and can be removed without a major version.
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
  - **Specialists run on pi's model and pi's tools.** pi-subagents reads no
    `.claude/agents`, so `hw` names the worktree's directory and the account's
    user-level one (`$CLAUDE_CONFIG_DIR/agents`, default `~/.claude/agents`, where
    `jd-judge-a`, `jd-judge-b` and `jd-fix-agent` live) in
    `HW_PI_CLAUDE_AGENT_DIRS`, and the adapter registers each file with
    pi-subagents when the session starts, translated in memory (nothing is
    copied or rewritten): a pi executor delegates to the same specialists and
    judges a Claude one sees (the lane's file shadows the account's of the same
    name). The file's `model:` is resolved, not dropped: the lane's
    `pi_subagent_models` in `projects.json` maps the alias (and, by name, an agent
    the lane classifies) to a pi model and thinking level, registered in
    pi-subagents' own fields, and the `pi_specialists` row says what each runs.
    What does not carry over: the tools pi has no
    counterpart for, dropped and named in the receipt's `pi_specialists` row (`WebSearch`, `Task` and `mcp__*`):
    `Task` (a child of a child: measured, `allowNestedSubagents` and
    `tools: subagent` both leave the child without a `subagent` tool) and `mcp__*`
    (the judges' `mem_search`: pi-subagents hands a child an extension-registered
    MCP server only when it runs in the background; since pi-subagents 0.78.0 a
    background child starts on a pi 1.2+ WITHOUT `@earendil-works/chord` (measured,
    `setup/pi/test-live-subagent-wake.mjs`), and the `pi_background_subagents`
    receipt row says at session start whether one can; where it cannot, a judge
    reviews from the code alone). `WebFetch` carries over as pi-web-access's `fetch_content` when that
    package is installed; `WebSearch` does not (a child given `web_search` answered
    that it had no such tool, cause not found). Specialists
    run in the foreground. A checkout that tracks `.pi/agents` keeps its own copies
    for the names they cover; those pin a model the call must override.
  - **The account's global rules reach pi as a context file.** pi loads
    `<agent dir>/CLAUDE.md` and the ones in the working directory's ancestors, not
    the account's `~/.claude/CLAUDE.md`. The adapter adds `$CLAUDE_CONFIG_DIR/CLAUDE.md`
    to the run's context files in `before_agent_start` (pi's own slot for user
    instructions; read, not copied, nothing under `~/.pi`) and writes a
    `pi_global_rules` receipt row. A pi-subagents child does not get it.
  - **Plugin skills are read, not invoked.** pi has no Skill tool: the plugin
    package (`-e`) puts its skills in the executor's catalogue under their bare
    names, and the model runs one by reading its `SKILL.md` (or `/skill:<name>`).
    The plugins' AGENTS are registered with pi-subagents under the name a Claude
    executor reads from `--plugin-dir`: hw names each declared plugin's `agents/`
    directory (`HW_PI_PLUGIN_AGENT_DIRS`, `<plugin>=<dir>`) and the adapter
    registers every file in it as `<plugin>:<name>`, so `subagent` with
    `pstack:comment-sicko` resolves instead of answering `Unknown agent`
    (measured 2026-11-09; `setup/tests/1273`). pstack's own pi extension also
    supplies an `agent` tool for the same names. Where pi-subagents is not
    loaded, the dispatch says the declared agent piece cannot be launched
    instead of dropping it.
    `done-invoker` proves a pi executor's review skills from its pi session
    (a `SKILL.md` read, an `agent`/`subagent` call), not from the report text.
  - **In a product lane, pi is not at parity on user-level skills.** Of the skills
    a Claude executor loads, pi gets `judgment-day`, `agent-browser` when the brief
    says `requires: browser`, and the checkout's `.claude/skills` (minus
    `speckit-*`) as one directory; the user-level skills are not passed. The
    plugins the brief declares ARE passed, as packages (`-e`, skills included),
    exactly as in a repoless lane (`golden-t5/pi-plugins-one`).
  - **The bash guard in a product lane is string-level too.** It refuses a
    command that names the main checkout or a sibling worktree, and leaves the
    executor's own worktree open. A relative path that climbs out of the worktree
    (`cd ../other && …`), a path built at run time, or a symlink inside the
    worktree that points into the main checkout is not seen. `--sandbox` is what
    contains it: under the profile the OS refuses every write outside the
    worktree, its artifacts and the temporary directories.
  - **A pi executor with `plugins:` does not share `auth.json`'s lock with your
    own pi sessions.** Its private agent directory links `auth.json` (one real
    file: pi writes it in place with `writeFileSync`, `core/auth-storage.js:71`
    and `:147`, so a token refresh lands in `~/.pi/agent/auth.json`), but pi
    locks `<agent dir>/auth.json.lock` with `realpath: false`
    (`auth-storage.js:50`, `:91-92`) and takes `auth.json`'s location only from
    the agent directory (`config.js` `getAgentDir`, `getAuthPath`): no setting
    or variable moves it. The lock is the private directory's, so an executor's
    OAuth refresh (`openai-codex`) and one of yours can run at the same moment.
    Nothing is invented to cover it; if both refresh at once the later write
    wins, and the refresh token the loser rotated is invalidated: that session
    is logged out (`invalid_grant`) and needs `/login` again.
  - **`--sdd speckit` is refused for pi**, before anything is built, with its
    reason (L14). pi runs `--sdd none`; `--sdd gentle` is retired and runs as it.
  - Everything else the V1 list named is closed (processed-proof delivery,
    pi-subagents with the guard as a required child extension, model and thinking
    readers, sandbox, the brain guard on product lanes, cockpit context tokens,
    guard on every tool, explicit skills, turn-end session identity, model-error
    fallback, startup screens). Residuals, none of them a missing mechanism:
    under `--sandbox` herdr's socket is denied, so a sandboxed pi publishes no
    `agent_session` through herdr; `--thinking off` runs `low` on
    `deepseek-v4.1-flash` (pi clamps it, and hw reports the divergence); the
    exact body the real opencode-go returns for a spent plan is `unverified`, and
    there is no pre-dispatch usage gate, only a manifest warning (the cap is shared dollars: a spent plan
    ends the run as `provider_quota`, reported blocked, and the fallback model is for model errors
    only).
  - **Optimisations: claude has one, pi's equivalent or why not (measured 2026-11-04).** Each row names the
    mechanism, where hw applies it, and what holds it. A row marked `pending` is another task's and says so.
    - **Subagent model.** claude: `CLAUDE_CODE_SUBAGENT_MODEL` exported at launch (`core/t5l_env.go:332`), recorded as
      `subagent_model`. pi: the variable is `NOT APPLIED` (`core/t5l_env.go:73-85`), but the outcome is not — the
      lane's `pi_subagent_models` profile resolves each definition's `model:` (an alias, never a Claude id) to a pi
      model and thinking level, and hw-executor registers both in pi-subagents' own fields
      (`setup/pi/hw-executor.ts:815-880`); the receipt's `pi_specialists` row says what each one runs
      (`docs/pi-vendor.md:29`).
    - **Explorer (cheap tier).** claude: `cheap-explorer` through `--agents` (`core/t5_explorer.go:226`, L21). pi: the
      same definition in `setup/pi/agents`, handed over as `PI_SUBAGENT_EXTRA_AGENT_DIRS` on `HW_EXPLORER_MODEL_PI`
      (`core/t6_pi.go:105-118`, `:181-199`); only where pi-subagents is loaded, and the prompt says so when it is not.
    - **Effort / thinking.** claude: `--effort` (`core/t5_vendor_agent.go:145`). pi: `--thinking` by kind
      (`core/t6_pi.go:425`), and pi CLAMPS a level the model does not support (`off` runs `low` on
      `deepseek-v4.1-flash`); the receipt's `thinking_verdict` says DIVERGENCE instead of claiming the asked level
      (`core/t6_pi_launch.go:348`).
    - **Guards.** claude: `--permission-mode` plus the deny hooks (`setup/guards/deny-repo-writes.py`,
      `~/.claude/hooks/`). pi: the adapter's `tool_call` runs the SAME `decide`/`insideProtected`
      (`setup/pi/hw-executor.ts` `judge`; setup test 983), a secret-read guard (989), the brain guard (988), and every
      child: pi-subagents' through its required-child API, pstack's through the private directory's own extension
      (983, 2026-11-04).
    - **Review proof.** claude: a `Skill` or `Agent` tool_use of the exact name (`core/invokers/transcript.go`). pi: a
      read of the skill's `SKILL.md` in its measured layout, or a waited `subagent`/`Agent`/pstack foreground
      `agent` call (`core/invokers/transcript_pi.go`); a declared Judgment Day is TWO judges, proved from ONE
      `workflow:"parallel"` call naming both, or one call each (setup test 1150's `jd_judges_*` goldens).
    - **Plugin hooks.** claude: one `--plugin-dir` per plugin, and every hook command runs through `hw/hook-env` with
      its state in `$HW_ARTIFACTS/plugin-state` (`core/t5_plugins.go:107-132`, L22). pi: `-e <plugin>` loads the
      plugin's extension and skills; pi HAS no hook system, so the extension is the equivalent (pstack's tools,
      ponytail's answer), and there is no hook state to redirect.
    - **Account.** claude: `CLAUDE_CONFIG_DIR`, and `hw` refuses an account not at parity. pi: `NOT APPLIED` (above).
    - **Waiting on a background command.** claude: the Agent tool's `run_in_background` and its completion notice.
      pi: the same guarantee, pi-native — `bash_background` starts a command and returns a job id, its exit wakes the
      session ONCE with the exit code, the last 20 lines and the log path, and `job_stop` ends it
      (`setup/pi/hw-executor.ts:927-1199`, registered only in a non-child session). The sleep-poll guard refuses a
      poll loop in both, `bash_background` included. It is not Claude's flag: pi has no `run_in_background`. Owning
      task `setup/pi-background-wait`, merged 2026-11-04; setup test 1235 and `docs/pi-vendor.md` "The background
      shell". `monitor` (a command's matching output lines are events, batched, with a MANDATORY timeout and an event
      cap) and `job_list` complete it (`pi-adopt-gentle`, 2026-11-05; test 1251). The sleep rule is narrower on both
      runtimes: a foreground poll loop passes under `timeout <= 120 s`, and a standalone `sleep` is refused while
      background work is in flight — a state only pi can see (it sends `active_work`; Claude's PreToolUse payload has
      none, so there the second half never applies: a stated limit; test 1133 k).
    - **Theme and settings.** A plugin dispatch runs pi on the private agent directory; its `settings.json` keeps
      every field except discovery. The theme that `theme` names is linked READ-ONLY into
      `<private>/themes/<name>.json` — from the real `themes/` or from a package's `pi.themes`/`themes/` — so pi no
      longer falls back with `Failed to load theme "<name>"` (measured 2026-11-04; `core/t6_pi_shadow.go`
      `piShadowThemeFile`). Owning task `setup/pi-theme-and-models`, merged 2026-11-04.
    - **Vendor-neutral, so no row of its own:** the ports and the dev server, the per-worktree database,
      `harness`/`browser-close`, the engram label proxy, the processed-brief proof, `checks.md`, the engram ids, the
      sandbox, and the turn end (claude's Stop hook and pi's `agent_before_settle` run the same
      `hw executor-turn-end`, test 982).

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
  - **pi runs in Go.** `--agent pi` dispatches are in `hw-core` (L16).
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
  - **A pi plugin's children get a private agent directory.** pstack's `agent` tool starts its children with
    extension discovery ON, so they would find the ambient `rpiv-ask-user-question` package, whose
    `ask_user_question` collides with the one pstack's extension registers (`pstack/pi/ask.ts`), and the child dies
    at startup. A pi dispatch whose brief declares a plugin therefore runs with `PI_CODING_AGENT_DIR` pointing at
    `$HW_ARTIFACTS/pi-agent`: symlinks to the real agent directory (auth, models, trust, npm, bin, sessions — never
    its `extensions/`, which a subagent must not load) and a `settings.json` copied without `packages` or
    `extensions`. The `theme` the real settings name is kept, and its JSON is linked read-only into
    `<private>/themes/<name>.json` — from the real `themes/` directory or from a package's `pi.themes`/`themes/`
    (measured 2026-11-04: without that link pi printed `Failed to load theme "catppuccin-mocha"` and fell back to
    the system theme, while the same theme's colours appeared with it). `~/.pi` is not written and a plain pi
    session is untouched; the top-level session is unchanged because `-ne` already loaded nothing ambient. The
    private directory's own `extensions/` carries ONE file, the
    child guard (`hw-child-guard.ts`, a re-export of `setup/pi/hw-child-guard.ts`): a child that discovers it is
    held to the same `judge` and secret-read redaction the executor is, without hw-executor's default factory (which
    would claim the child as the root session). Measured live with a pstack child: with the file absent it ran
    `echo x > <product repo>/__probe__/x` and bash failed on the missing directory; with the file present the same
    call was refused by the guard. pstack's agents are reachable through pstack's own `agent` tool, not through
    pi-subagents' `subagent` roster.
  - **The export ships no plugin.** The copies carry third-party text and its licenses; `setup/vendor/` is
    excluded from the export, so there `--with-review-plugins` refuses and `plugins:` refuses every name until
    you put one at `<brain>/plugins/<name>/` with a `.claude-plugin/plugin.json`.
  - **The review proof reads the executor's own session — claude, or pi.** `done-invoker` proves each review skill the
    closing named from that session (and its subagents'): on claude a `Skill`/`Agent` tool_use of the exact name;
    on pi a read of the skill's `SKILL.md` (under the layouts below) or a `subagent`/`Agent` call naming that
    agent, counted only once the call's own `toolResult` arrived without an error. A mention in prose is not a
    call, and only what ran at or after the receipt line counts. pi's receipt records the session file's path, read
    as a literal file under pi's sessions root (`$PI_CODING_AGENT_DIR/sessions` when set, else
    `~/.pi/agent/sessions/`). Without a reachable session the check falls back to the report's whole-token names
    and says `unverified: transcript not reachable`; an unknown vendor gets the same fallback.
  - **pi: only two `SKILL.md` layouts prove a skill.** A read of `<plugins>/<p>/skills/<n>/SKILL.md` proves `p:n`
    and `n`, and `.claude/skills/<n>/SKILL.md` (project or user) proves `n`; a partial read (`offset`/`limit`)
    still counts. Any other `<x>/SKILL.md` proves nothing, and a plugin laid out differently is released with
    `review-skip: <skill> — <why>`.
  - **A plugin vendored in `setup/vendor/plugins` is never skipped.** `review-skip:` for thermo, ponytail or pstack is
    refused (the exit for one that cannot run is `--blocked`, so the brainer dispatches a fresh executor that loads it);
    only a piece no vendored plugin ships, or a run whose vendor loads no plugin (`plugins` receipt `NOT APPLIED`), keeps it.
    `hw next` holds the other half: a re-task brief declaring a plugin the pane did not launch with is refused before
    anything is sent (claude and pi; a plugin is loaded when a session starts).
  - **A pi close that DECLARES Judgment Day is held to the two judges.** The receipt carries `jd_agents_t<N>` when the
    brief declares `review: judgment-day` (or a list naming it), the lane is pi, and BOTH `jd-judge-a` and
    `jd-judge-b` exist in the directories pi registers (the lane's `.claude/agents`, then the account's);
    `done-invoker` then refuses a done report unless the pi session (and its subagents') records a `subagent` call to
    EACH judge with agent, task and `async:false`. The builtin `reviewer` is not a substitute where those agents are
    registered: a measured session used it in a judge's slot, and a proof naming one name is not proof of two blind
    judges. Where the judges are not on disk the dispatch says so itself ("jd-judge-a and jd-judge-b are not on disk
    for pi"), no row is written and the close is not held to them; an UNDECLARED (optional) Judgment Day writes no
    row either, because skipping it is permitted. Claude's closing is unchanged (no such gate).
  - **Cost.** pstack puts ≈12.5 KB of skill descriptions in the executor's context for as long as it runs.
- **Evidence:** `measured` for the dispatch lines, the receipt, the launch flags and the refusals (setup tests 843,
  844, 847, 848 and 979, bash against Go on the same goldens, with a mutant on each decision), and for a real
  executor of each vendor that sees a plugin's skill when its brief asks and does not when it does not, with
  `~/.claude` and `~/.pi` unchanged. The child guard and the two judges are held by setup test 983 (the driver runs
  `setup/pi/hw-child-guard.ts` as a child would) and hw-core's tests (`TestJdAgentsReceipt*`, `TestJdFactsByVendor`,
  `TestJudgmentDayGateRequiresBothJudges`, and the four `jd_judges_*` scenarios in `TestDiffDone`, whose goldens fail
  when the gate's condition is reverted); the live pstack child is in that test's header. `unverified`: that a plugin
  improves any task; that is a question for a measurement of its own.
- **Workaround:** none needed; declare only the plugins the task wants.

### L27. Lane-local executor rules reach Claude Code and pi only
- **Scope:** `brain/<lane>/.claude/CLAUDE.md`, the executor rules a lane keeps out of its product repo.
- **Impact:** hw links the file into every worktree or work directory of the lane as `.claude/CLAUDE.md` for a Claude
  Code executor, and appends it with `--append-system-prompt` for a pi one (fresh launch and `hw revive`). An opencode
  or codex executor gets neither: the manifest's and the receipt's `lane_rules` line says
  `NOT APPLIED — <vendor> …`. The lane's `.claude/agents` and `.claude/skills` are linked only in a lane with no
  product repo; a lane with its own repo keeps its repo's own, gets only
  `.claude/CLAUDE.md`, and only where the worktree has none (the receipt says `KEPT (repo has its own)`). Only Claude
  Code reads them; pi reaches the brain's agents through its own extension, and opencode/codex through theirs.
- **Evidence:** `measured` — pi 1.1.0 in a throwaway HOME/PI_* quoted the appended file's first heading with the flag
  and answered NONE without it; the link and the info/exclude are held by hw-core's tests (setup tests 1220, 1222) with
  a mutant on each decision.
- **Workaround:** a rule every vendor must obey belongs in the brief (or in `CLAUDE.shared.md` for the agents that read
  it); a lane-local rule is in force for Claude Code and pi only.

### L4. OpenCode background subagents are experimental
- **Scope:** OpenCode executors and brainers that spawn subagents in the
  background. **OpenCode is deprecated** since 1.0: outside the [1.0 promise](REFERENCE.md#what-10-promises), it keeps working and can be removed without a major version. The harness enables them with OpenCode's
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

### L14. `--sdd speckit` is Claude Code only
- **Scope:** dispatches with `--sdd speckit`.
- **Impact:** `hw` refuses the mode for a Codex or OpenCode executor before
  building anything. `--sdd gentle` and `sdd: gentle` are gone: they warn and run
  `--sdd none`, and `review: rdd` warns and closes with Judgment Day. The
  `sdd_modes` lane field went with them: a `projects.json` that still has it
  fails to load, naming the field, until you delete the key.
  The cockpit is a separate gate: it needs Claude Code 2.1.289 or newer, and
  `brain` opens without it below that.
- **Evidence:** `measured` for the Spec Kit refusals by `75` and `64`, for the
  retired spellings by `165`, and for the cockpit floor by `803`, all from a
  checkout's `bin/hw`.
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
  - An executor is guarded only against writing into the brain, and a Claude
    Code executor also against writing into any `product_repos` root of
    `guards.json` other than the one that holds its own `HW_WORKDIR` (so it may
    still write its own repository's checkout, not only its worktree; an
    OpenCode or Codex executor has no such guard). Everything
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
- **Scope:** `secrets:` in a brief, `hw secret run|put|check|status|ensure`,
  `secret-unlock` and, where it is installed, the global guard
  `deny-bitwarden-secrets.py` (Claude, Codex, OpenCode). **This export ships
  `hw secret`, `secret-unlock` and the `secrets:` preflight, and does not ship the
  guard or a scheduled unlock** (you set up the job that runs `secret-unlock --auto`): on a
  machine installed from it nothing refuses an agent's own `bw` call, so
  `hw secret run` is the sanctioned path and not an enforced one, and the vault
  is unlocked by a person with `secret-unlock`. The Keychain half
  (`secret-unlock --setup|--auto`, which a dispatch tries once on a locked vault)
  is macOS only; elsewhere the vault has to be unlocked before the dispatch.
  Needs the `bw` CLI, which the installer does not install.
- **Impact:** the master password (`secret-unlock --setup`) is kept in the login
  Keychain as the service `hw-bw-master`, readable by any process of the user that
  passes the item's ACL, and a scheduled `secret-unlock --auto` unlocks from it at load and every hour (and renews a session with under 6 h left).
  Where the guard is installed it cuts AGENTS (a Keychain read of it,
  `secret-unlock --auto|--setup`), not a process made on purpose. `~/.cache/hw/bw-session` (0600) holds a
  token that opens the WHOLE vault, not only the folder `agents`: `hw secret run`
  refuses items outside that folder, but a process running as the same user can
  read the file and ask bw for anything. The guard refuses `bw get|unlock|export|
  list|serve|create|edit`, the file's name and `BW_SESSION` in an agent's command, and judges
  the command's TEXT: `B=bw; $B get x`, a script that calls bw, or a value already
  in a child's environment and printed on purpose (`hw secret run … -- sh -c 'echo
  $V'`) are not caught. Like L5 it is a tripwire for the careless, not a sandbox,
  and a quoted mention (a commit message spelling `bw get`) is refused too: write
  it through a heredoc. Codex and Claude settings carry the registration by hand,
  and a machine without the guard has none of this tripwire.
  `hw next` re-tasks do not run the `secrets:` preflight.
  `hw secret put` is the way in: the value comes from a pipe on stdin, reaches bw on bw's
  stdin (never argv) and jq in jq's environment, and put prints one line. It cannot read a
  value back, but it can OVERWRITE one: `--replace` on an existing item of the folder
  `agents` destroys the old value, and the guard cannot tell an intended replace from a
  mistaken one (it judges the command text, and `put` names no bw verb). A put that
  creates an item needs the folder `agents` to exist; a dotted name that matches no item
  is read as `<item>.<field>`, so a new item whose own name has a dot cannot be created by put.
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
- **Workaround:** the demo lane (`docs/examples/demo/`) exercises the real path once
  by hand. Before each release candidate is published the maintainer runs one
  end-to-end pass with a real herdr server and real Claude Code and OpenCode
  executors (ask, challenge and report through the return channel); that run
  is not part of this repository.

### L24. A suite job whose inputs did not change is replayed from its last green, so a change in a file it never read is unseen until a cold run
- **Scope:** `setup/test-hw` with `setup/verdict-cache` (every subject, guard and go-test partition) and the per-mutant lookup in
  `core/mutant_verdict_test.go` (the in-process mutant corpora m826, m840, m844, m848, m861-866, m871).
- **Impact:** a job's green is stored under the sha256 of its own file, the bytes of every file it reads (a subject: `setup/suite-data/subject-deps.txt`,
  a partition: `setup/suite-data/go-test-deps.txt`, both measured by access time, not guessed), the names in the directories it read, the toolchain and
  an allowlist of `HW_*` knobs, and replayed when those bytes come back; only greens are stored, written atomically. What is traded away:
  (1) a change in ANOTHER file that would make a job fail, or let an old mutant survive, is not seen, because that file was never in the
  job's reads; (2) a mutant's verdict is keyed by its anchor file, its test files, its cases and goldens and the toolchain only, so a change
  elsewhere in `core/` or in the world the cases run in leaves its kill cached; (3) a subject that reads outside the checkout (the network,
  the installed claude) must say `# cache: never` in its header — seven do, and one of them was found on 2026-11-01 — and one that does
  not is cached on bytes that do not hold its answer. **Nothing measures that, and a new subject that reads the world without the
  declaration is still not refused.** A text rule cannot stand in for the measurement: `$HOME`/`~`, a URL that is not loopback, `ssh` and
  an agent CLI by absolute path flag 90 of the 522 subjects, and the narrowest sane version of it still flags 33 — of which 32 are a stub
  they build or an assertion on a path string, and 1 was real: the subject that asks the PATH for the second
  account's CLI (`setup/tests/122-the-account-an-executor-inherits.sh`, `claude-personal` at line 129), which it
  now declares. Every `$HOME`/`~` in the rest is the run's own temp home (`tests/_common.sh`
  pins it, with `XDG_CONFIG_HOME` and the per-vendor pointers), so it is not the machine's;
  (4) a subject nobody measured is keyed to the whole tree, which is today's behaviour, never a smaller one: until
  `python3 setup/measure/subject-deps.py` has run, only a byte-identical tree hits. Measured 2026-11-01: 4 subjects took 50 s with
  `--jobs 3` (one red in its clone, left unmeasured), so the 522 at that rate are about an hour of machine time — over the half-hour a
  single task is allowed — and the file still ships unmeasured.
- **Evidence:** `measured` by `setup/tests/1110-un-job-corre-solo-si-cambio-lo-que-lee.sh` on stub subjects and a stub in-process suite (second
  identical run spawns only the `# cache: never` job; an edited read file, an added file in a read directory and an edited partition file miss;
  a red is never stored; `HW_TEST_NO_CACHE=1` runs cold) and by `TestMutantVerdictKeepsAKillByItsInputs`. `unverified`: the whole suite's
  hit rate on a real release, and the measured `subject-deps.txt` (the file ships unmeasured).
- **Workaround:** `HW_TEST_NO_CACHE=1 bash setup/test-hw` runs every job cold. Run it once per release cut, on Linux (it is not enforced by
  the cut). A subject you know reads the world — the installed agent, the network, `$HOME` — gets `# cache: never` in its header by hand:
  until something measures it, that declaration is the only thing between a green and the bytes it never read.
  `HW_VERDICT_CACHE=<dir>` moves the store, `off` disables it; entries unused for 30 days are pruned.

### L26. Some claims about `hw`, `brain`, `channel-send` and the invokers lost their mutation test when the bash left, and have no Go mutant yet
- **Scope:** the mutation-anchor registry (`setup/mutation/mutation-anchors.tsv`) and the subjects that killed mutants of the bash bodies of `hw`, `brain`,
  `channel-send`, `ask-invoker` and `done-invoker`, which 0.6.0 deleted.
- **Impact:** a mutant of a bash body cannot be applied to code that is gone. Most of those claims moved to the Go side, where a Go mutant, a
  golden case or a unit test holds them; about 90 did not, and for them the claim is still asserted but no longer by a mutant that proves
  the assertion can fail. They are of three kinds: the wording of a message or a refusal (held by a golden that compares the bytes, which a
  changed golden would also move); a guard on one branch of a verb (a dirty tree skips a cached verdict, a lock is taken, a lease is judged live
  or spent), where a behavioural arm still runs the verb through `hw-core` and the Go has no anchor on the branch; and a defect class that
  only bash had (an unset variable under `pipefail`, a pipeline that fails silently), which has no Go twin and needs none. A regression in
  the first two kinds is caught only if some arm exercises the exact case; one in the third has no Go code to regress.
- **Evidence:** `measured`: the per-task lists of the retired anchors (go-060 S4 and S5) say, row by row, what holds each claim and which have
  no Go holder; `setup/mutation/l26-inventory.md` classifies all 90 of those rows (94 items: the 806 EXIT-trap row is three mutants, and
  798-M04 was listed twice). `measured` by that inventory and by running each new mutant: 20 claims now have a Go mutant killed in process
  (`go-806-M01..M10` by `TestBrainMutants` over `contract-hw/mutants-brain-go-806`, `go-67-M06` by `TestC844Mutants`, `go-185-M01` by
  `TestC848Mutants`, `844-A170..A175` and `844-M51` by `TestC844Mutants`, `826-G35/G36` by `TestInProcC825Mutants` and test 826);
  22 are kind 1 held by a golden or a byte-comparing test a mutant would move with it; 16 are bash-only defect classes
  or claims about the deleted bash's shape and need none. **36 kind 2 rows are still owed**, each naming in the inventory the fixture or arm
  it is missing (a dirty-tree done case, a table-only lane, a git stub exiting 127, …): no arm exists that
  a Go mutant on those branches could be killed by, so anchoring one today would be vacuous. `unverified`: that those 36 branches are
  exercised by their subjects at all.
- **Workaround:** none needed to use 0.6.0. A change to a ported verb's branch (in `core/`) should add its Go anchor in the same change; the
  registry's check (`setup/tests/540`) refuses a registered id no subject uses. `setup/mutation/l26-inventory.md` is the list to work from;
  every owed row names what would make its mutant killable.

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

### L34. `hw review --at <sha>` on an already-merged commit measures against the current base, counts 0 code lines and dispatches nothing
- **Scope:** `hw review <lane> <task> --at <sha>` (the benchmark form: a review of a past head) when `<sha>` is already an ancestor of the lane's base, which is the case for any merged build.
- **Impact:** the diff base is the merge-base of the SHA with the lane's base. For a commit already in the base that merge-base is the SHA itself, so the range is empty: `hw review setup review-past-head --at f77d35df7 --dry-run` prints `0 code-only line(s)` and tier `0`, and no reviewer is dispatched. `--at` works as designed only on a head the base does not contain yet (a branch before its merge). It never touches the build's receipt either way, so it cannot satisfy `hw done`'s merge gate.
- **Evidence:** `measured` by the dry run above, on the commit that introduced `--at`; the engram topic `setup/review-docs/findings` carries the output. The design did not cover it (`setup/review-past-head/findings`).
- **Workaround:** none in `hw` today. Review a branch before it merges, or read the merged range by hand with `git diff <parent>..<sha>`.

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
  Node, engram and jq. The floors are rows of `lib/hw/deps.conf`, the one
  place any script reads them from.
- **Impact:** a tool below its floor is refused (`MISSING … Install: <update command>`
  by `--check`; a `dep-floor` rule at level `block` from `hw preflight`, only for the
  tools the dispatch uses). `hw <lane> <task>` itself does not check them: the
  preflight is what a brainer, or the cockpit's rule band, runs first. Claude Code
  2.1.289, OpenCode 1.18.31, Node 22.7 and jq 1.7 were already
  demanded by a script and moved there unchanged. herdr 0.9.1 and engram 3.0.0 are the versions that were installed when
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


### L18. The marker-only strip pays a full run for the shapes it does not parse, and 540's orphan scan stops at a subject's own body
- **Scope:** `setup/verdict-incremental` (a runner file whose diff is only `# MUTATION-ANCHOR: <id>` markers stays
  incremental), `setup/gate-select` (its `all` answer) and the orphan scan of `setup/tests/540-*`.
- **Impact:** (1) the strip reads the shell that carries a marker instead of matching it: quotes that span lines,
  `$( )` and `${ }` inside a double-quoted string, heredoc bodies and `\` continuations, plus Python's own
  tokenizer for a Python runner. Only a `#` it is certain begins a comment is stripped, and a file whose quoting
  does not close is compared as it is, so the direction of any doubt is a full run, never a trusted one. What is
  left: it is an approximation of bash, so a shape it mis-reads in the trusting direction is possible in
  principle, and a marker that sits inside an embedded awk or Python heredoc, or on a continuation line, is now
  TEXT and costs a full run where it used to be trusted (`setup/test-hw` carries two, `mutation-coverage` and
  `go-test-core.sh` three each). (2) `gate-select` runs its `# gate-when:` pass before printing `all`, and
  `setup/test-hw` reads the `reference` lines that come beside it, so a `mutants-*`-only commit and a pre-commit
  `all` run every slow subject whose `gate-when` matches a changed path (540 for a `core/`, `bin/` or `mutants-*`
  change, 1014 for a `core/*.go` one) or whose `# gate-reference:` names one (a `_common.sh` change: eleven in
  all, seven of them by gate-reference, about four minutes). What is left: a FAST subject's `gate-when` is still not printed beside `all` (the whole
  fast gate runs anyway). (3) 540's orphan scan covers every non-subject file under `setup/tests/` — `_common.sh`,
  the `contract-hw` helpers — so an unregistered id there is refused when 540 runs; its `gate-when` names none of
  those helpers, so a commit touching only them meets the refusal in the full run, not at pre-commit. What is left: a marker inside a SUBJECT's own
  body is a template (`862-Gnn`) and is not judged, and `setup/tests/fixtures/` are copies of other files.
- **Evidence:** `measured` by `setup/tests/820-el-veredicto-incremental.sh` (a marker inside a heredoc body, inside
  a multi-line quoted string and on a continuation line each give a full run; mutant 820-M12 is the old textual
  strip and the heredoc arm kills it) and `setup/tests/999-*.sh` (`all` carries `reference 540-*`; 999-M05 restores
  the early return, 999-M06 drops the line, 999-M07 makes `setup/test-hw` ignore it — each killed) and
  `setup/tests/540-*` (M11: an unregistered marker in `setup/tests/_common.sh` is reported; old: the scan skipped
  the whole directory and reported nothing). `unverified`: that no real commit reaches a shape the lexer mis-reads,
  and how much a release pays for the markers it now refuses (12 of the 148 marker lines on the runner's 23 files).
- **Workaround:** none needed. `bash setup/verify-for-push --full` when a marker-only change was trusted and you
  want the certainty the parse declined to give; `bash setup/tests/540-las-anclas-de-mutacion-existen.sh` (about 5 s)
  is still the cheap check before pushing a change to markers or to `mutants-*`.

### L19. A chained incremental verdict compounds gate-select's blind spot, and fast gates outrank a full run
- **Scope:** `setup/verdict-incremental` + `setup/verify-for-push` (a green incremental run leaves its own record, so
  the next tree can start from it, up to `HW_VERDICT_MAX_CHAIN` = 3 hops off a real full run; a record at the cap
  is no base, the plan re-anchors on the real full run over the whole diff, and a big diff or a far base is no
  reason for a full run any more) and the pool in
  `setup/test-hw` (a fast gate's worker request outranks a full run's next subject, and a full run's timing subjects
  always run last, alone).
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

### L25. `setup/heavy` reports a frozen or starved run; it does not unfreeze it
- **Scope:** `setup/heavy` on a machine where something SIGSTOPs heavy trees under memory pressure (a memory-pressure
  guard local to that machine; nothing in this repo installs it).
- **Impact:** (1) the preflight warns when free memory is under `HW_HEAVY_RESUME_FREE` (26, a copy of that guard's
  resume line, not read from it — if the guard's thresholds move, this number does not follow) and starts anyway;
  `HW_HEAVY_REQUIRE_FREE=1` makes it refuse with exit 75. (2) A watchdog reads the holder's process tree every
  `HW_HEAVY_WATCH` seconds (60) and calls it frozen when more than half of it is in state `T`; after
  `HW_HEAVY_FROZEN_ALERT_MIN` minutes (20) it writes one line to `heavy-alerts.log` and, if `HW_INVOKER_PANE` is set,
  one `channel-send --report` per frozen stretch. A tree stopped by anything else (a debugger, `Ctrl-Z`) alerts the same
  way, and the text says "most likely" the guard. A run frozen under 20 minutes, an alert whose delivery fails
  (the failure is a line in `heavy-alerts.log`), and a `setup/heavy` that is itself stopped, raise nothing.
  (3) A holder or a waiter whose hw run has REPORTED is reclaimed and its tree ended, but only a job that started
  (owner file's epoch, or the queue ticket's mtime) at or before the run's `done` marker: a job that started after it
  belongs to an executor reopened by a hand-written `channel-send`, and is left alone. The comparison is in whole
  seconds, so a job started in the very second of the report is still reclaimed. (4) While a job runs it holds
  `caffeinate -i -w <job pid>` (PreventUserIdleSystemSleep, a user process, no `pmset`); a machine without `caffeinate` does nothing. **No executor-level hold exists:** an executor outside a heavy
  job relies on its vendor — Claude Code was measured holding its own `caffeinate -i -t 300`; pi, OpenCode and Codex are
  `unestablished`. Closing the lid is not covered by `-i` (not measured here).
- **Evidence:** `measured` with stubbed `memory_pressure`, `ps`, clock and `channel-send` by
  `setup/tests/1040-una-corrida-pesada-a-la-vez.sh` (F12-F16; mutants H09-H13). `unverified`: against the real guard
  freezing a real run (nothing was frozen for real), and `ps` state letters on Linux.
- **Evidence:** also `setup/tests/1040-una-corrida-pesada-a-la-vez.sh` F28 (mutant H21) and F29 (mutant H22,
  macOS: `pmset -g assertions`).
- **Workaround:** `setup/heavy --status`; free memory (the guard resumes the run over its resume line).

### L28. `secret-unlock --auto` tells a wrong password from a broken tool by bw's words, and a broken bw is caught at the next hourly run, not at update time
- **Scope:** `core/hw_secret_classify.go`, `core/hw_secret.go` (`cmdAuto`, `vaultAlert`, `cmdStatus`).
- **Impact:** only bw's own answer `Invalid master password.` (`@bitwarden/cli` 2026.10.1, `build/locales/en/messages.json:77`) is a rejection; every other failure is a tool, migration, network, timeout or unknown failure that keeps a live session and pages only under 60 min of session left (or with no session). (1) A bw release that rewords that message, or a non-English locale, would make a real rejection read as "unknown" (retried and paged under the tool wording, never silently dropped) until `hsRejectedRe` follows it. (2) No update hook exists to run `bw --version` at brew/npm upgrade time: `hw secret status` runs it, and the hourly `--auto` meets a broken bw within the hour. (3) A user whose account verifies the password on the server gets `Invalid master password.` for any server error on that path, so such an error still reads as a rejection.
- **Evidence:** `measured` with stub bw binaries reproducing the dyld crash and the 404 migration error (`core/hw_secret_classify_test.go`, `setup/tests/1012` A07). `unverified`: against a real bw release other than 2026.10.1.
- **Workaround:** `hw secret status` after a brew/npm upgrade; `~/.cache/hw/bw-unlock.log` names every outcome.

### L30. The `release: prod` env diff compares names and booleans only, screens prove existence, and the flag precheck is a template rule
- **Scope:** `core/internal/releasediff`, `core/t5_release.go`, `core/invokers/release.go`, `docs/BRIEF-TEMPLATE.md`.
- **Impact:** (1) the platform API returns no values, so non-boolean values (URLs, numbers, strings) are compared by presence only: a different value under the same name is invisible. Only values that are exactly true/false/0/1 inside the backend container are compared. (2) A screen is a PNG that exists and is newer than the run start; it does not prove the PNG shows the right view or the right data. (3) That the brief's own precheck prints the flags is a template rule, unenforced. (4) Only containers named `backend-<app uuid>-<digits>` count; preview deployments (`-pr-N`) are ignored, and zero or two matches refuse. (5) The close re-diffs and refuses only names ruled `needed`; a new unruled difference after dispatch is not refused at close. (6) A `release: prod` brief re-tasked with `hw next` is refused, not gated. (7) The QA must report after the release's dispatch (the ledger's `done_at`), but the ledger's `done` also covers a `--blocked` report and a removed workdir, so a blocked QA satisfies the close. (8) The env-diff commands come from the environment before `setup/release-prod.env`, so an executor can stub them when it runs done-invoker.
- **Evidence:** `measured` against stubs (`setup/tests/1286`) and once live against the two RO apps (2026-11-09).
- **Workaround:** name the non-boolean values a release depends on in `## Testing / Verification` and print them from the host; a person opens each screen.

### L31. Security-audit leads left open: every agent runs as the same uid, so the guards are behavioural and `--sandbox` is the only enforced boundary
- **Scope:** the twelve leads the 2026-11-10 security audit left as "needs validation" and that were not fixed with the others (the audit's numbering; the nine that were executed are fixed and have tests). All of them assume an executor, or a prompt-injected one, that can write its own run directory or worktree.
- **Impact:** each line is the lead, the open fact that would settle it, and why it is bounded:
  - **#5** the brain guard may not resolve a relative path inside a quoted `sh -c` / `python -c` string (`deny_brain_writes.py` `_names`); open: a run of `sh -c 'rm ../../../brain/x'` from a product executor. A same-uid shell reaches brain anyway, so this is a tripwire gap, not a boundary.
  - **#6** the brainer's read-only guard lists mutator verbs and omits `curl -o`, `tar -x`, `unzip`, `ditto`, `sqlite3`, `awk`/`sed` writes (`deny_repo_writes.py` `MUTATORS`); open: whether the brainer's lane lets those binaries run. A closed verb list is a tripwire by design (L5).
  - **#8** the issue viewer passes third-party GitHub markdown to the app's `TextView.markdown` with no scheme or image filter; open: what the herdr app does with a clicked link or a remote image, which is app code outside this repository.
  - **#9** `hw outbox flush` trusts the executor-writable `.meta`/`.msg` and runs `hw done` on the task the meta names (`core/t3_outbox.go` `outboxForeign` binds lane and run, not task); open: whether a forged meta for another task of the lane closes it. An unsandboxed executor can already send a report as itself.
  - **#11** `hw done` archives a pi transcript through `newestSession`, which accepts a symlink named `*.jsonl` that `copyFile` follows (`core/transcript_archive.go`); open: whether a sandboxed pi executor's link to a read-denied file lands in the archive. The executor cannot read the archive.
  - **#12** operator-side writers into the executor-writable run dir (`brain` repoint, `hw next`, `hw ruling`) write by path with no `O_NOFOLLOW`; open: whether a planted symlink makes the operator write outside the sandbox's write set.
  - **#14** operator-side `git -C <worktree>` reads a gitdir/config the sandbox leaves writable (`<wt>/.git`, `commondir`); open: whether a redirected gitdir with `core.fsmonitor` runs under `hw done`.
  - **#15** the sandbox grants the whole common `objects`/`refs`/`logs`, so a sandboxed executor can move any branch (see L11), and `hw train push`, `hw done`/`reap` and new worktrees trust local refs as provenance; open: whether a forged merge on the base makes an unmerged sibling removable or runs a nominated `verify-for-push`.
  - **#17** `hw status` and `hw-reconcile` print run-dir values (receipt `pane`, `model_running`) with raw control sequences; open: which sequences the operator's terminal honours. Terminal spoofing only.
  - **#18** an executor can write the chaining-lease and done marker that make its pane the next dispatch's reuse target (`core/t4_next_util.go`); open: nothing beyond the printed `REUSE WARNING`; `--fresh` avoids it. It delivers a later brief to a pane the same uid already controls.
  - **#20** `hw revive` re-arms the sandbox with the cwd of the last `session_resume` line of the executor-writable `receipt.jsonl` (`core/t4_revive.go`), cross-checked only against the executor-writable env; open: whether a forged line widens the write scope across a revive. The operator runs `hw revive` and the cwd is printed.
  - **#21** `hw sweep --apply` closes the herdr tab or workspace whose id a run-dir file names (`core/t4_sweep.go`); open: whether a forged id plus a done marker closes another pane. Availability only; an unsandboxed executor can call `herdr` itself.
- **Evidence:** `unverified`: source-confirmed by the audit, not executed here. The nine leads that were executed are in the changelog entry of this release.
- **Workaround:** run an executor that must not be trusted under `--sandbox` (L11) and read what `hw done`, `hw revive`, `hw sweep` and `hw outbox flush` print before accepting it.

### L32. `bin/move-root` repairs the places it knows; a quiet window is the operator's, and a task branch keeps its old root
- **Scope:** `bin/move-root` (the move, `--undo`, `--audit`), `projects.json` `root`, `guards.json` `{brain}`/`{work}`.
- **Impact:** (1) The move refuses while any process stands in `brain/`, `work/` or `archive/` (brainers included) and while herdr, herdr-app or hc answers on its socket: the window is every agent stopped and `hc server stop`, from a plain terminal. (2) Not repaired, by design or not yet: the vendors' own session stores keyed by path (opencode, pi, Codex `sessions/`), the cwd recorded inside transcripts and the archive's receipts (history), herdr's `session-snapshots/`, untracked files inside the brain, the run state of a product lane's task (`<checkout>/.worktrees/<task>/.hw/`, inside a product checkout the move does not write; a run there keeps naming the old brain until it ends), a symlink deeper than the depth scanned (work 6, brain 5, a product checkout's `.worktrees` 4) or a relative one from outside into the trees, and run-state files over 8 MB. (3) A task branch made before the move carries `"root"` on the old path until it is rebased onto the move's commit; its guards resolve `{brain}` to a directory that no longer exists, so `deny_setup_brain_writes` protects nothing from that worktree. (4) `--undo` restores a file byte for byte only if nothing touched it since the move, and skips, naming it, one removed since (`hw done`, `hw reap`); a file changed since (Claude Code rewrites `.claude.json` often) is reversed by substitution, and said. (5) A rewritten LaunchAgent needs `launchctl bootout`/`bootstrap`; the command prints them and does not run them. (6) With the agents root moved out of an org's directory, a session in a non-lane directory of the old root (an org's shared docs, say) is no longer "under the org root" for `bin/engram-label-proxy`, so it relays instead of refusing an unlabelled write. (7) `--audit` reads the spellings `/<dir>/brain|work` and, in root-bearing files, home-rooted ones; a path built at run time from pieces, or spelled as Claude's project-directory slug (`-<home>-<root>-brain…`, which step 5 renames), is invisible to it.
- **Evidence:** `measured` on HOME-like scratch trees: `setup/tests/1362` (move, every repair, the commit, `--audit`, `--undo` byte for byte, the live-process refusal and its mutant) and the brief's end-to-end proof (a clone of the brain with worktrees, re-homed copies of the accounts' settings, herdr/hc state and run state). The real move has not been run.
- **Workaround:** run `bin/move-root --to <dir> --dry-run` first and read every line; after the move, rebase or finish the task branches it lists, reload the LaunchAgents it prints, and run `bin/move-root --audit`.

### L33. The herdr app: no script can press a mod button, a report card outlives its pane by about a second, and the QA stand-in hides three real behaviours
- **Scope:** the herdr app's foreman mods (Cockpit, Agents) and `bin/herdr-isolated`, as the rc.1 live QA (2026-11-12, `website` and `setup` fixture lanes) measured them.
- **Impact:** (1) `herdr-app commands` lists no `<mod>.<command>`, so no QA can press a mod button (Reply, Rule, Done, Dispatch, the fold): the QA runs the exact argv the button runs through the app's own `exec()` (a throwaway mod in the isolated app) and looks at the cockpit with `herdr-app shot`; the click handler itself stays unmeasured. (2) A run dispatched without `--keep-pane` is closed about a second after its report (`done-invoker` closes the executor and its dev terminal), so its report card is on screen for about that long and neither the fold nor **Done** can apply to it; `--keep-pane <ttl>` keeps it for the person to read. By design. (3) The isolated app runs a stand-in `claude`: it has no `Stop` hook (a ruling queued for the turn end is delivered by `done-invoker`, not at the turn end), writes no transcript (`model_running` reads `none`, so a model on a row is shown by a simulated receipt line), and answers slowly enough that `done-invoker` can exit 5 while the stand-in brainer is still echoing; its `npx` is a python server. A real claude cannot sign in under the instance's config dir.
- **Evidence:** `measured` in the isolated app (`rc1-app-live-qa`, `rc1-app-qa-fixes`; engram topic `setup/rc1-app-live-qa/findings`).
- **Workaround:** prove a click by its argv and its effect (`herdr-app shot`, `hc get`), dispatch with `--keep-pane` when the report must be read in the cockpit, and treat the Stop-hook delivery and the real model reader as established only by their own tests.
