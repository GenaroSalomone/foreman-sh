# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) with pre-release labels.

## [0.9.0-rc.1] — 2026-11-11

### Added
- Herdr app: a **Ports** panel in the right column lists every live executor with its ports (listening or not), the CPU and memory of its process tree, and the machine's load, executors and free memory against the load cap.
- The herdr app's Cockpit shows each Claude account's 5-hour and 7-day usage at the top,
  one line per account, amber from 80% and red from 95%, dimmed with its age when no session
  has refreshed it for 15 minutes. It reads what `bin/statusline-usage-tee` keeps from Claude
  Code's own status line (no credential, no Keychain).
- The Herdr app has a History panel (a fourth label in the right column): finished
  runs by lane, newest first, with each run's outcome, duration, model and commits.
  Opening one shows its timeline (the brief, each step's tools and key outputs,
  asks and rulings), the report and links to its artifacts. It is read-only and
  reads what hw already records; the session file is parsed where it lives.
- `docs/KNOWN-LIMITATIONS.md` L16 names every group of files that stays bash,
  with its reason, and the ones whose port is still open.
- A brief can declare `effort:` the way it declares `model:`: a check against the resolved dispatch (never a source), refused before anything is built when it disagrees, with the same levels and refusal words as `--effort` on claude, opencode, codex and pi. `--dry-run` lists it on the `contract` line.
- A brief that declares neither `model:` nor `effort:` gets one warning naming both and what the dispatch resolves; it is never a refusal.
- The app cockpit puts what waits on you in one queue at the top, oldest first; a click on an entry focuses that executor's pane. Each executor row carries a context meter (quiet below 60%, warning from 80%, alarm from 90%), and a filter (all / needs you / working / done) narrows every brainer's executors, remembered across restarts. In all three looks.
- `bin/herdr-isolated start|stop|ls <dir>`: an isolated Herdr app instance starts from the README recipe (clean env, mods, layouts, the N1 fix) and stops entirely — app, feeders, the detached herdr-core and its holder — found by the instance's own socket directory, never by name. It refuses a `<dir>` that is broad or contains the person's own herdr tree.
- `done-invoker` warns, naming pids and sockets, when an herdr process of an isolated instance the run started (a socket under its TMPDIR or a registered instance dir, or an herdr binary whose cwd is its work directory) is still alive at report time. A warning, not a refusal; the person's own core is never reported.
- The README shows what a prompt looks like: the one line a person types, and the
  brief the brainer writes from it, near the top of the page. The example is held
  by a test against the check `hw` runs on a brief's front matter.
- The README carries a screenshot of the herdr app running foreman, with three
  brainers and five executors in the cockpit.
- `setup/tests/1338` holds that an invoker's shell-out to `hw` uses a verb an executor may run, or strips `HW_TASK` and `HW_RUN` first.
- `setup/heavy --quiet <max-load1> -- <cmd>` holds the job (and its slot) until the machine's 1-minute load average is at or
  under the bound, saying once a minute what it reads. For a timing run that must start on a quiet machine.
- The fast gate's summary prints `first-exec assessments: N (~S s at 0.28 s each, …)`: how many first-exec checks macOS ran
  during the gate (`setup/first-exec-count`, ~2 s), the serialized cost behind a slow suite, so a fix's ceiling is known before it
  is written. The headers of `setup/test-hw` and `setup/go-test-core.sh` point at the evidence (`setup/evidence/pool-3x/README.md`).
- `setup/go-test-core.sh --base <ref> -run <TestName>` runs one focused Go test on that commit's tree and on the current one and says red, green or none on each, so a red can be told apart as the branch's or the base's without building a copy by hand.
- `setup/commit-tree` refuses a new `setup/tests/*.sh` that has no entry in `setup/suite-data/test-budgets.json`, naming the entry to add, instead of letting the gate refuse to start.
- `setup/shell-lint`, run by `setup/commit-tree` and by the pre-commit hook over the shell files of a commit: it refuses a quote that `bash -n` accepts but shellcheck does not, and a `printf … | grep -q` pipe the commit adds.
- `setup/guards/drive-guard --guard <guard.py> [--env K=V]… <file>` feeds a PreToolUse guard one payload kept in a file (a whole
  hook payload, or the bare command text of a Bash probe) and prints `deny: <reason>` or `allow`. A Bash call that spells a
  protected path is itself refused by the live guard, so a guard could not be driven from the shell; the path now travels in a file.
- `bin/herdr-isolated shot <dir> <out.png>` raises the instance's window by its pid, waits, takes the screenshot and refuses a frame
  byte-identical to the last (a stale frame). The OSC 7501 payload that registers an agent state is documented in `herdr-app/README.md`.
- Issues panel: saved views (filter and display kept apart; "Ready to dispatch" and "In flight" offered), a chip per issue for the executor working on it and its pull request, and "Hand to brainer", which delivers the issue to its lane's brainer through `channel-send`.
- `hw <lane> <task>` from a herdr app terminal now builds the lanes that have dev servers or a database as it does from a
  TUI pane: the worktree, ports and per-worktree database, and each dev server as an `hc run` terminal of its own
  (labelled with the task, plus `hw_role`), which `hw done` and `hw reap` end with the executor.

### Changed
- `bin/stall-watch` is a shim now: the stall timer runs as `hw-core stall-watch`.
  Same arguments, same alert, same marker and log.
- `hw status` reads the machine-wide suite-slots line itself instead of
  running `bin/suite-gate`; the line is byte-for-byte the same.
- `done-invoker` and `ask-invoker` read the lane table themselves (the
  operator's name, a lane's engram label, the roots a run env file may live
  under) instead of running `bin/project-spaces.sh` in a bash of their own.
  Same answers on a table the loader accepts; on a table that parses but the
  loader would refuse, they now answer from what they can read rather than
  falling back, and no longer print the loader's diagnostic (every other
  command already stops at it).
- `hw-reconcile` runs in Go (`hw-core hw-reconcile`); `bin/hw-reconcile` is a
  shim with the same name and arguments, and the same output and exit codes
  (0 nothing to report, 3 a recovery queue, 1 a failure) on the same runs. It
  is still read-only. It no longer needs `bin/state-witness.sh`,
  `bin/project-spaces.sh` or an inline `python3` program; it reads a run's env
  file itself and still runs `python3 bin/holdfacts` to validate a hold (a
  `holdfacts` or `python3` that is missing is now a failure, exit 1, not a
  queue that reads as empty). Four differences: it needs `hw-core` (from a
  checkout that has not built one it says so and exits 1); the recovery queue
  lists the runs in the order of their directory names, where the bash listed
  them in whatever order the filesystem returned them; on a lane table that
  does not load it says so in its own words, not jq's; and a run on an app
  terminal (hc) is asked through hc like the other commands, not through
  herdr only.
- `done-invoker` refuses a plain done while the task's own `checks.md` has a `fail` row: a task that says a requirement failed closes `done-invoker --blocked` (it stays alive and a ruling resumes it). `unresolved` rows are still accepted, and `--blocked` is not gated.
- Inside an hw executor, the touched gate (`HW_TEST_CHANGED="$HW_ARTIFACTS/changed" HW_TEST_GATE=fast bash setup/test-hw`) exits 75 instead of silently becoming the whole fast gate when the selector would go whole; pre-commit keeps its fallback.
- A runner waiting for a machine slot repeats its WAITING line every 300 s and names the holder's whole run age, not only its subject's.
- Inside an executor (`HW_TASK`), `setup/go-test-core.sh` holds every partition it is asked to run to the same estimate-vs-budget
  refusal as `--affected --run`: a lone partition, a `+a,b` list or a hand list over `touched_budget_seconds` exits 75 and names the
  focused run. `setup/heavy` allows an executor run ONE slot job (held or queued) at a time; a second exits 75. The short lane and
  a job started by a heavy job are not counted.
- The one-test form is `setup/heavy --short -- sh -c 'cd core && go test -count=1 -run <T> .'` (the old `… go test … ./<pkg>` form
  fails from the repo root: `core/` is its own module). The rules and the brief template ask an executor for the touched subjects plus
  a focused `go test -run`, not "touched Go partitions", which the runner refuses. The probe recipe uses a timestamp-shaped `HW_RUN`
  and tells a codex probe the `herdr pane report-metadata` line. The brief template points the leak check at `HerdrLeaks`.
- `regen-grep.sh` names the third golden family, `setup/fixtures/verb-golden/`, and exits 1 while it still holds the old text.
- `hw next` holds a re-task brief's `model:` to the model the pane was launched with, the way it already held `effort:`: a disagreement is refused (nothing sent, counter unmoved) and names `--model` on a fresh executor; a level written inside the id is refused in `--effort`'s words.
- When a dispatch REUSES a live pane, the brief's `model:` is held to the pane's own recorded model (it was compared with the lane's default, which the pane does not run).
- The README and INSTALL name the git identity a dispatch commits with, the `herdr integration install claude`
  and `engram setup claude-code` steps before the first `brain`, `engram serve`, and what a dry run's
  warnings mean.
- `hw next` refuses a re-task brief that declares review plugins (`plugins:` or a `review:` list) the pane did not launch with, on claude and pi: a plugin is loaded when a session starts, so the executor was told "not installed" and closed on a `review-skip:`. Nothing is sent and the counter does not move; the refusal names the fresh dispatch (`hw <project> <task> --fresh`).
- `done-invoker` refuses a `review-skip:` for a plugin vendored in `setup/vendor/plugins` (thermo, ponytail, pstack): a declared plugin that did not run is never a skip, and the exit for one that cannot run is `--blocked`. A run whose vendor loads no plugin (codex, opencode) and a piece no vendored plugin ships keep their skip. The closing's prose and the refusal now say so.
- The export's root is tidier: the sample brief `init.sh` copies is now `docs/examples/demo/briefs/hello.md` (was `examples/demo/`), and `scripts/repo-settings.sh`, a maintainer's tool for the GitHub repository settings, is no longer exported. `shell/` and `CHANGELOG.md` stay at the root: `~/.zsh` symlinks point into `shell/`, and `CHANGELOG.md` is the one history ledger (`docs/RELEASE-NOTES.md` holds only the current release).
- The README opens with one image of what foreman-sh provides (brainer, executors,
  reviews, retro, guards, secrets, the herdr desktop app) and a section on running
  it from the app; Claude Code and pi are named as the native agents.
- The cockpit has one short section in the README instead of three overlapping
  ones; the card sample lives in REFERENCE.md only.
- The sample `install.sh --check` output in the README matches what the installer
  prints today.
- When `shellcheck` is not installed, `setup/commit-tree` and the pre-commit hook now say so on every commit (before, the notice was shown only when a commit was refused).
- `setup/go-test-core.sh --base <ref> -run <Test>` says `skipped` instead of `green` when every selected test skipped (an opt-in corpus test without its switch) and exits 1 on this tree, naming the switch.
- The refusal of an over-budget Go run inside an executor now prints, for each affected corpus, one focused command per `setup/tests/contract-hw/cases-*` file with the corpus' own case variable (`HW_INPROC_CASES`, `HW_C848_CASES`, …) and the number of cases, instead of `-run <TestName>`. The variables live in the new `CASES=` line of `setup/suite-data/go-test-partitions.txt`.
- `setup/suite-data/go-test-deps.txt`: a path mapped to a corpus is also mapped to that corpus' mutant partitions (their unmutated control run reads it), so `--affected` queues them. `rest` is unchanged.
- The exit footer of `ask-invoker` and `done-invoker` no longer says "retry the identical command" under a refusal; it points at the message above it.
- `bin/herdr-isolated start` accepts a `<dir>` of up to 59 characters (the longest socket path of an instance, 44 more, must stay under the 103 a unix socket holds) instead of 48, so an executor's own `TMPDIR` works.
- The executor sleep guard also refuses a same-line `cmd > out 2>&1 & other` (a `&` that backgrounds a command redirecting to a file, whatever follows it on the line).
- `setup/tests/1014` reads a copy of the sources taken once when it starts, so an edit made during its 15-25 minute run no longer queues behind it or mixes into a mutant.
- `~/.cache/hw/heavy.log` lines are now `start end wall rc load1=<at start>/<at end> tag cmd`: a timing quoted from the log
  says how loaded the machine was.
- The fast gate and its `.fast` verdict now say how many Go partitions ran (`go partitions Q of N`).
- Inside an hw executor, the slow subjects the touched form selects are cut at `touched_budget_seconds` even when each is quick, and the cut is printed.
- The documented changed-paths file is written against the merge base (`git diff --name-only "$(git merge-base HEAD main)"`), so a worktree behind main does not list other people's changes as its own.
- The executor sleep guard also refuses a command that ends in a shell `&` and redirects to a file, naming `run_in_background`, and reads `nice -n N` as a prefix of the command it wraps.
- A `HW_INPROC_CASES`, `HW_C848_CASES` or `HW_C870_CASES` list that names a case matching nothing now fails instead of running nothing.
- Judgment Day: a BLOCKER/CRITICAL only one judge raised, ruled `real`, reproduced by the orchestrator and introduced by the
  candidate, now enters the fix loop like a confirmed finding (it used to stay unfixed and force an ESCALATED verdict, which made
  two disjoint judges zero-recall). One ruled real but not reproduced is still not auto-fixed.
- `bin/herdr-isolated start` builds the whole QA instance: its own `HOME` and `CLAUDE_CONFIG_DIR` (a trust entry or any claude state lands
  under the instance, never in the real `~/.claude.json`), a private unreachable engram, a fixture `gh`, a stand-in `claude` first on
  every terminal's PATH (a real claude cannot sign in under another config dir: measured "Not logged in"), `<dir>/q` to run a command in
  that environment, and with `--brain` a copy of the tree with its own hw-core. The person's `~/.local/bin` is no longer on the instance's
  PATH (`hc run claude` ran the real claude through it); only `herdr-core`, `hc`, `herdr-app` and `herdr` are linked.
- The Issues block of the three layouts names the hw state folder (`props.dir`) the Cockpit already reads.
- `hw <lane> <task> --dry-run` under herdr-core prints, as ✗, every refusal the real dispatch makes there (`--sandbox`,
  a vendor other than claude, `--here`); it used to print ✓ and the launch then died.
- `hw revive` and the re-dispatch check on herdr-core read past a task's dev-server terminals when looking for its executor.

### Removed
- `bin/hw-train`: the bash body of the merge train, a second implementation of
  `core/t6_train.go` that nothing called any more. `hw train add|status|push`
  is unchanged.
- The bash bodies of the per-lane build files under `lanes/` (other than
  `git-worktree.sh`): dead copies of the Go builders. The files stay as named
  selectors; `projects.json` `build` is unchanged and `hw <lane> <task>`
  behaves the same.
- `bin/engram-serve.sh`: the launch asked the engram serve who it was by running
  that bash lib; `hw-core` now does it itself (`core/engram_serve.go`, the same
  check the turn end and `brain` already ran). The receipt line for a
  registration that failed names the POST, not a `curl`. `done-invoker` no
  longer refuses a `bin/` that lacks the file.
- The README no longer repeats, under "How it compares", what the Why table and
  the Features list already say.

### Fixed
- The Artifacts panel in the Herdr app shows an artifact's page from the top, at the panel's
  width, with no empty block above it. The picture's box was as tall as the PNG with the page
  centred inside it, so a tall page floated hundreds of pixels down; the panel now sizes the box
  from its own width and the PNG's proportions.
- `brain` no longer registers an engram session before a refusal: a refused `brain`
  ("Nothing was sent and nothing was restarted") posts nothing to engram and prints no
  engram line (the session is registered once the launch cannot be refused any more).
- A fast gate run on main itself (an empty diff against main) queues every Go partition
  instead of none, and says so.
- The app cockpit names each brainer by its lane, also when it has no executors, instead of its pane id, and the header no longer says the same thing twice. Brainers with no executors share one line of lane names instead of a section each, in all three looks. `cockpit-state` takes `--lane` (`bin/brain` passes it) and publishes `lane` in the state file, an optional field of schema 1: older files still read, named by their rows' projects.
- The herdr app's right-column switch bar no longer clips the look switcher (`C D G`) and the live count in look D (and C, G) with five panel labels: the bar is two rows, labels above, the count and the look switcher below, readable down to a 300 px column.
- The regen scripts take a per-golden-directory lock: two concurrent regens nested one copy inside the other (30 copies, ~40 min).
- A red that is its fast-gate time budget is labelled `BUDGET-STALE` (with both numbers) under `--keep-going`, not `ENV-ONLY`.
- The repo-write guard follows a plain `cd <dir>` in a compound command instead of giving up on it, so a read-only path under the
  brainer's checkout beside a `cp` in the worktree (the judgment-day skill) is no longer refused; a write there still is.
- `setup/heavy`: a pause that heavy made itself no longer raises a false FROZEN alert when it ends while the watchdog is sampling the process table.
- An app (herdr-core) brainer shows one cockpit, the side panel: `brain` no longer adds `--plugin-dir <cockpit>` to the
  claude it starts in an app terminal, and keeps the writer the side panel reads. `--no-cockpit` / `FOREMAN_COCKPIT=0`
  still stop both, as before.
- `herdr-isolated start` (and the README recipe) builds the isolated app instance from THIS tree's mods: the person's
  `foreman-*` symlinks are no longer copied, a copy that fails is reported instead of swallowed, and `start` refuses an instance
  whose app dir holds a symlink. Before, the instance ran the person's mods and the next copy of the tree's mods failed four
  times with "Not a directory" (or wrote through a link).
- A report that reaches the brainer through `hw outbox flush` (what `brain <lane>` runs when it opens) now starts the
  chaining lease of a `--keep-pane <duration>` run, from the run's own env, as done-invoker does. Before, `hw next` said
  "task 1 has no chaining authority".
- The History panel reads an app run as `done` or `blocked`, not "closed unreported": `hw done` and `hw receipt` now snapshot an
  app (herdr-core) run's report from the `hw_*` labels on its terminal into the receipt, as they did from a herdr pane's tokens.
- `hw next` ends the chaining-lease timer (a bash and its 3600 s sleep) of the lease it consumes, instead of leaving it until `hw done`.
- When a dispatch reuses a pane whose vendor is not the lane's (a codex pane under a claude lane), the brief's `effort:` is judged by the pane's vendor: `effort: ultra` on a codex pane is no longer refused with claude's levels.
- `install.sh` now copies `_agents/` into the brain, so a fresh install's dry run no longer says
  `explorer NOT INJECTED` for the `cheap-explorer` the README promises.
- The QA a `release: prod` brief dispatches from its `qa:` line now carries the release's own
  `--base`, `--sdd`, `--model`, `--effort`, `--agent`, `--account` and `--fresh`, so it launches as that
  QA brief does by hand with them. Before, a QA brief declaring `base: main` was refused ("the brief and
  the flags disagree") against the lane's own base. A flag whose key the QA brief declares with a
  different value is not carried: that declaration stays the QA's own. `hw --dry-run` now prints the
  command the QA would run.
- A release's `done-invoker` no longer refuses a QA that has reported done. The gate reads the QA's state with
  `hw ledger`, which `hw` refuses to a shell carrying `HW_TASK`/`HW_RUN` (an executor's), so the refusal went to
  stderr and the gate read an empty `done_at`, however the QA had been launched. The gate now makes that call as the
  invoker, and a `hw ledger` that fails is reported with its own words instead of as "has not reported".
- When the QA really has not reported, the refusal now says what to do: close with `done-invoker --blocked`, end the
  turn, and run `done-invoker` again only after the brainer's ruling; it no longer reads like something to retry in a loop.
- `setup/commit-tree` refuses a commit that adds a `core/*.go` with no line in
  `setup/suite-data/go-test-deps.txt`, naming the line to add: test 1014 P03 used to go
  red on main after the batch gate.
- `hw` run as `bin/hw` from a worktree (which has no `hw-core`: `core/dist/` is gitignored)
  now says, when `HW_TASK` or `HW_RUN` is set, to run the `hw` on PATH instead.
- `setup/export/clean-install-check` removes the export tree and the per-vendor HOMEs
  (about 570 MB) when every step passed, and keeps them on a FAIL; `--keep`, a no-op until now,
  keeps them on a pass too. The logs always stay.
- The fast gate is red when it queues no Go partition although the changed paths name `core/` or the Go runner, instead of green.
- `printf … | grep -q` pipes in the hooks, the runner and the release scripts no longer depend on `grep` outliving its writer under pipefail.
- `setup/export/check` decodes git's quoted paths in a `--tracked` list, so a tracked file with a non-ASCII name is not reported as unclassified.
- The `hw next` contract world refuses to replace an existing pane fixture by accident.
- `--sandbox` no longer lets the executor read the Bitwarden session token (`bw-session`), which unlocks the whole
  vault rather than the `agents` folder.
- `--sandbox` no longer lets the executor rewrite the pinned `<workdir>/.hw/<run>/verify` that `hw done` runs
  unsandboxed, nor plant or rename a run directory.
- The OpenCode Bitwarden guard now judges `secret-unlock --auto|--setup` and `hw-secret --auto|--setup` (its prefilter
  skipped them).
- The blind-kill guard refuses `pkill -u|-U <uid>` with no pattern, and sees through `env -u VAR`, `nice`, `setsid`,
  `caffeinate` and `eval`.
- `cockpit-state` no longer builds a state path from a `HW_INVOKER_PANE` that is not one path component.
- Shell restore no longer replays names hw execs or reads code from (`HW_CORE`, `HW_BW_BIN`, `HW_BIN_DIR`,
  `HW_IMPL*`, `*_ROOT`) from the executor-writable run env, and `hw revive` replays only the exact names the run env
  writer writes: `BASH_ENV`, `ZDOTDIR`, `PATH`, `DYLD_*` and any other name are refused.
- `hw status`'s agent-tools probe (`claude -p` inside each product checkout) no longer runs that repo's project hooks
  or `.mcp.json` servers: it passes `--settings '{"disableAllHooks":true}'` and `--strict-mcp-config`. A product
  repo's own MCP servers are therefore not started by the probe, so tools served only by them are reported as unknown.
- `setup/commit-tree` refuses a path argument that holds a newline (several paths in ONE argument, which is what an
  unquoted `$VAR` is in zsh), exit 1 with nothing written. It used to read it as an already-gone path and exit 0 with only
  the other named paths committed.
- `done-invoker`'s checks.md preflight line and the build prompt's checks.md note now say what the gate does: a `fail` row
  refuses a done and the task closes `--blocked`.
- A dispatch of a lane with dev servers or a database from the herdr app is no longer refused with "dev servers and
  databases need `hc run` terminals of their own" after the dry run said ✓.

## [0.8.0] — 2026-11-10

The release candidate 0.8.0-rc.2, promoted with no change.

## [0.8.0-rc.2] — 2026-11-10

### Added
- `hw wait`, `hw unstick`, `brain <lane> --reset`, `--resume <id>` and `brain relaunch` work on an app terminal
  (herdr-core), through `hc`.
- `release: prod` briefs: hw diffs the staging and prod env at dispatch and at close (app env names plus container booleans),
  refuses an unruled or stale `env-diff:` ruling, requires `screens:` PNGs and a `qa:` run reported before the close, and
  refuses a rehearsal on an env that differs from the release target.

### Changed
- L29 now says what the fifth live-cycle QA measured (real claude, isolated app), and the README says the Agents list's space-name rows are the app's own mod.

### Fixed
- `hw wait`, `hw wait-one` and `hw outbox flush --to` refuse an app terminal address (`t3`, `machine/t3`) named from a
  process that is not an app terminal, and `hw revive` refuses an app run from one, instead of asking the default
  herdr or core (the person's own). One gate in the dispatch, so a new verb that names an address is covered by listing it.
- `brain <lane>` on the herdr app opens on the first try: a failed `agent.focus` is retried
  and the outbox is flushed whatever focus does.
- A report sent while the brainer is closed stays a done: `done-invoker` queues it in the
  outbox (exit 0, `queued` marker) and `--blocked` can no longer replace it.
- An executor's invoker address follows a brainer reopened as another terminal.
- `brain <lane>` no longer flushes another lane's, another run's or another herdr-core's queued report into the new brainer (a stale
  `probe:probe` entry once reached a setup brainer, which acted on it); the skipped entries are named and left where they are.
- A report delivered from the outbox now closes its executor (`hw done`, unless the run was launched with `--keep-pane` or the report
  is blocked) and leaves the transcript's REPORT line, as a direct report does.
- An executor dispatched on the herdr app records its `session_resume` (read from claude's own `sessions/<pid>.json`), so `hw revive`
  can resume it; the cheap explorer is armed through `--agents` in the terminal's argv, and the receipt reads `agent_ready: yes` once
  the brief's turn is proved.
- The cockpit's state file for an app brainer is `<core id>.<address>.json`, and a run records the core that dispatched it
  (`hc-core`): two cores no longer share `t2.json`, and a reopened brainer no longer re-points another core's runs.
- `hw revive` resumes a run of the herdr app: it starts the recorded cwd, env and resume as an `hc run` terminal (refusing a task that
  still has a live terminal), proves it with `hc get`, and reopens the run as on herdr.
- The cockpit plugin inside an app brainer reads the per-core state file (`HW_COCKPIT_CORE` in the brainer's environment), and
  `brain <lane>` no longer waits on the `hw done` of a flushed report longer than `HW_FLUSH_CLOSE_WAIT_MS` (5 s); a close still running
  goes on in its own process, with its output in `close.log`.
- `herdr-app/README.md` documents an isolated live QA (own brain copy, work root, config dir and MCP config).
- The cockpit and `hw status` list the executors that run in the herdr app again (hc 0.1.19 refuses
  `hc ls --label hw_task`; the label is now filtered on `hc get`), `hw` and `brain` resolve inside the app's
  brainer and executor terminals, and `hw done` removes the worktree of an app run when no herdr server runs.
- A `--blocked` report sent while the brainer is closed is queued like a done (exit 0, "QUEUED"), counts as a turn-end disk fact,
  and the reopen's outbox flush files it as blocked (`blocked-waiting`), so `hw ruling` resumes it instead of being refused.
- A brainer on the herdr app gets a cockpit writer for as long as its terminal lives, and a lane's space takes the lane's name
  instead of a terminal's.
- `hw done` of a `--keep-pane` run cancels its chaining-lease timer.
- A report the outbox flush delivers to an app brainer moves the executor terminal's labels to `hw_done_state=delivered` and
  `hw_invoker=<the new brainer>`; the cockpit no longer says "report undelivered" to a dead brainer for ever.
- `brain <lane>` on the default account clears an inherited `CLAUDE_CONFIG_DIR` for the brainer it starts in an app terminal, as `hw` does for
  executors, so the folder-trust and first-run checks read the account the brainer really runs under.
- A brainer started in an app terminal starts without the run identity (`HW_RUN`, `HW_TASK`, `HW_PROJECT`, `HW_WORKDIR`, …) an app launched from
  an executor's shell hands every terminal. The README recipes start the app with `env -i`, and L29's workaround names `HW_*`.
- A non-blocking `hw wait` on the app leaves no finished monitor terminal: it removes itself `HW_WAIT_MONITOR_KEEP_S` seconds (default 300)
  after its last verdict.
- The brainer's cockpit writer removes the state files of writers that did not leave (older than an hour, loop lock free).
- The cockpit panel's "no state folder" says the app reads files only under its own home; the README's isolated-QA recipe puts the work root there.
- `hw ruling`, `hw next` and `hw unstick` refuse an app terminal address (`t3`, no colon) named from a shell with no `HC_TERMINAL_ID`, naming the missing env, instead of asking the default herdr server about it.
- `ask-invoker` tells an executor to end its turn after a delivered ask (the reply arrives as its next prompt; the sleep-poll guard refuses a wait), and the shared rules say the same.
- `cockpit-state --kick-run` for an hc brainer that has left removes its state file instead of redrawing "report undelivered".
- The herdr-app README `env -i` recipes carry `USER`, `LOGNAME` and the real `~/.local/bin`; the layouts name the cockpit folder from `~`; the panel's "no state folder" line wraps instead of being clipped.
- An executor resumed after its brainer was reopened on the app can report again: the run's env follows the new brainer
  address, and `brain` re-points the lane's runs whose invoker terminal is gone.
- `hw done --blocked` is no longer refused for the turn that queued the report itself.
- `channel-send --reply-hold` waits up to `HW_HC_PROOF_MS` for the ask's delivery proof instead of refusing a reply sent too early.
- The cockpit raises no herdr alarm on an hc-only brainer, a writer removes its file when it leaves, and the plugin finds its
  state by `HC_TERMINAL_ID`.
- `hw done` cancels every chaining-lease timer of the run, not only the newest.

## [0.8.0-rc.1] — 2026-11-09

### Added
- `setup/heavy` keeps the Mac awake while a job runs: `caffeinate -i -w <job pid>`, a user process that
  ends with the job (even if heavy is killed), no `pmset` or system setting.
- `hw help next` answers, and says how to re-task or reopen an executor: `hw next` for a live reported
  one, `hw revive` then `hw next` for a closed one, and that a hand-written `channel-send` opens no task.
- `setup/next-test-number [N]` prints the next free `setup/tests/NNNN-*.sh` number, reading main, HEAD, the working tree and every live local `task/*` branch (a collision renumbers the subject the colliding branch introduces, never another's; `docs/BRIEF-TEMPLATE.md` says so).
- `done-invoker` now also requires, for each review plugin the closing named, a row in `$HW_ARTIFACTS/checks.md` that names the skill with a verdict (pass, fail or unresolved) and its findings or "no findings" — loading a skill is not applying it. A `review-skip:` line still releases a skill.
- The Herdr app's Artifacts panel (`herdr-app/mods/foreman-artifacts`): the
  claude.ai artifacts of the Teams (default) and personal accounts, listed with
  title and last update, where a click draws the artifact's own page in the panel
  and opening it in the browser is the second action.
- `brain <lane>` run from an herdr-core (`hc`) terminal, as the herdr app starts one, opens the brainer as an hc
  terminal instead of a herdr workspace tab: labelled `label=brain`, rooted at the lane's brain directory, opted into
  the app's window with `herdr.space=<the lane's space>`, and shown with `herdr-app do agent.focus` — so `brain setup`
  puts the brainer on screen in the app. An open brainer is focused by its cwd, never built twice.
- An executor dispatched from that brainer resolves `HW_INVOKER_PANE` to its terminal: the resolver reads `hc ls` and
  `hc get`, normalised into `herdr pane list`'s shape, so the manifest's invoker line, its "verified as label=brain
  cwd=…" proof and its refusals are the same words on both backends. A failed `hc` read is reported as unverified, never
  as "no brainer".
- `channel-send hc <terminal> - <message>` delivers to an agent living in an herdr-core terminal (a brainer opened by `brain <lane>` in the herdr app, or an executor), with the same exit codes (0/1/4/5) and the same report-versus-redirect rules. Redirects wait for the receiver's `idle`/`done` status with `hc wait`; `hc send` is never given `--if` or `--force`, and a blocked receiver is refused, not typed into. Delivery is proven from the receiver's own status, `turn` and turn-end labels; a send nothing confirms within `HW_HC_PROOF_MS` (default 15000) is exit 5.
- `done-invoker` and `ask-invoker` reach a brainer in an hc terminal, and `ask-invoker` works there: its hold records route `hc` and the terminal's own address (`holdfacts` accepts it). `hw ruling` reads an hc executor with `hc get` and queues for the turn end.
- The foreman cockpit in the Herdr app: `herdr-app/mods/foreman-cockpit` draws every live brainer's
  executors — status, model and context, commits, the running suite, subagents, and what waits on that
  brainer — from the same `bin/cockpit-state` file the Claude Code cockpit reads, and
  `herdr-app/layouts/foreman.json` puts the cockpit on the left, the terminals in the center, and an
  empty column on the right for the viewer. Installed by symlink; `herdr-app/README.md` says how.
- `hw` run from a herdr-app (herdr-core) terminal launches the executor there: one `hc run --bg --json --no-parent --no-notify` terminal named for the task, labelled `hw_project`, `hw_task`, `hw_run` and `herdr.space`, with the herdr variables unset in its argv and the brief as claude's launch task, proved by `hc get`. A second dispatch of the same task finds the live terminal with `hc ls --label hw_project= --label hw_task=` and launches nothing. `--dry-run` names the route on the `placement` line; from the herdr TUI nothing changes. Claude executors on the plain lanes only for now (KNOWN-LIMITATIONS L29).
- `hw done` of an executor that runs on the herdr app (an `hc` terminal) now ends it: it kills and removes the run's terminals found by their `hw_project`/`hw_task` labels (a terminal with other labels survives), and the in-use guard reads the terminal's status and report from `hc get`. `hw reap`, `hw status` and the worktree occupancy count those terminals as occupants. A run without the `.hw/<run>/hc` record makes no `hc` call (KNOWN-LIMITATIONS L29).
- A foreman Issues panel for the Herdr app: the GitHub issues that involve you
  (authored, assigned, mentioned, involving), with a text filter, scope, state
  and repo filters, a refresh, and a preview of any issue — title, state,
  author, labels, markdown body and comments — inside the panel. Read-only
  through `gh`, with clear states when it is missing, signed out or offline.
- The Herdr app has a second look beside C: `herdr-app/themes/d-linear.json` (Linear: `#0f1011` canvas, inset `#17181c` panel, one `#26282d` line, indigo accent, yellow working, red blocked). Pick it with cmd-shift-P, `app.theme`; C stays the layout's default. `looks.test.mjs` now proves no foreman mod hard-codes a color or a look, so each renders with whichever theme is active.
- Every Herdr app look shows a `C D G` look switcher at the right end of its right-column switch bar, the shown look marked; a click runs `layout.switch {name}`. `cmd-shift-l` still cycles every layout. Held by `switcher.test.mjs` and proved in the isolated instance (19/19 per look).
- `hw next <terminal>` re-tasks an executor living in an herdr-core terminal: the gate reads `hc get` (idle/done opens it, blocked is a person's prompt, otherwise it waits with `hc wait`), keys go through `hc keys`, and the task is delivered with `channel-send hc`. The TUI path is unchanged.
- `hw outbox flush <lane> --to <terminal>` delivers through the `hc` vendor when the target is an hc address, and `brain <lane>` in the app flushes the lane's outbox to the brainer it opened or focused (waiting for a started one to report a status first).
- The read-side tools see executors that run as herdr-app (herdr-core) terminals: the cockpit state and `hw status` list the live `hc ls --label hw_task` terminals with their status and turn from `hc get`, `state-witness` falls back to `hc get`/`hc read` when herdr knows no such pane, and `stall-watch` ends when its `hc` terminal is gone instead of treating a missing `HERDR_PANE_ID` as alive. The cockpit writer bound to an app brainer no longer exits as "pane gone". Nothing changes when no `hc` terminal runs (KNOWN-LIMITATIONS L29).
- An executor running in an herdr-core (`hc`) terminal, as the herdr app starts one, publishes its turn-end and
  invoker state as `hc label` keys (`hw_turn_state`, `hw_turns`, `hw_done_state`, …), proved by an `hc get --json`
  read-back: the same fact the TUI reads from herdr tokens.
- When both variable sets are present, the nearest `herdr`/`herdr-core` ancestor decides the backend, so an app
  terminal that inherited `HERDR_ENV`/`HERDR_PANE_ID` from whoever launched the app never writes onto that pane.
- `herdr-app/mods/foreman-viewer/`: a Herdr app pane that shows one file — markdown
  rendered, code read-only with syntax highlighting and line numbers, images drawn when
  they live inside the app folder, any other format as plain text. Open it with
  `herdr-app do pane.split:right --mod foreman-viewer --props '{"file":"/abs/path"}'`;
  the README has the `fv` shell one-liner for opening a path from a terminal.
- The Herdr app has a third look beside C and D: `herdr-app/themes/g-zed.json` (Zed's One Dark: chrome `#3b414d`, docks `#2f343e`, terminal `#282c33`, one `#464b57` line, blue accent, yellow working, red blocked, One Dark's terminal ANSI). Pick it with cmd-shift-P, `app.theme`; C stays the layout's default. The mockup's Plex/Lilex fonts are not installed or applied: the layout's Geist Mono stays.
- pi has two named model rosters and one switch: `pi_roster` in `projects.json` is `go` (the paid opencode-go roster,
  exactly as before) or `free` (opencode's free models: `crouton-alpha-free` for build, design, audit, explore and the
  explorer, `beam-free` for review, each the other's fallback on a model error). It ships as `free` while the paid
  quota is spent; going back is changing that word, no code. Absent means `go`. `--model` still overrides the primary.
- The dry run names the active roster (a `pi roster` line) and the receipt records it (`pi_roster`).
- Under `free`, a real `--agent pi` dispatch (task name without a `test` or `probe` word) prints one warning: pi runs
  on free models for testing only. Nothing is refused. The lanes' `pi_subagent_models` rows name paid models and apply
  to `go` only.
- `hw` prints the opencode-go usage (rolling, weekly, monthly) as a `pi usage` line in the dispatch manifest and in
  `--dry-run`, with a WARNING at 70% or more of any window or a status other than `ok`. It never blocks and fails open.
- `deny_setup_brain_writes.py` also allows a `python3 - <<EOF` whose body only quotes the brain path as text (`write`, `replace`, `format`); `open()`, `os.*`, an assignment, an f-string or an unparseable body stay refused.
- `setup/commit-tree` warns, never refuses, when a new `setup/tests/N-*.sh` takes a number main or a local `task/*` branch already owns.
- The export's "exported tests with no measured budget" error prints the entries ready to paste into `setup/suite-data/test-budgets.json` (`setup/export/narrow_budgets.py`).
- `setup/test-hw` fails on a repeated test number not in `setup/suite-data/test-number-dups.txt` (`setup/test-number-check`). The cockpit subject is now 1272 and the pi plugin-agents subject 1273.
- `setup/tests/contract-hw/regen-grep.sh '<text>'` runs the `--grep` regeneration over t3, t4, t5 and t5l.
- `setup/pi/test-hw-executor.mjs` with no arguments prints its usage and exits 2.
- A Claude executor whose turn dies on an API error or a spent quota (StopFailure, which fires instead of Stop) now publishes `turn_state=ended_api_error` and clears `children_running`, so `channel-send --ruling` and `hw ruling` stop reading the pane as awaiting a child. `install.sh` registers `hooks.StopFailure` beside `hooks.Stop`; run it again to pick it up.
- `hw ruling` against a pane that says `ended_awaiting_child` with a `turn_ended_at` more than 10 minutes old warns "probablemente obsoleto" and names the plain `channel-send` that delivers. The ruling is queued all the same.
- The setup brain guard's python-heredoc exemption applies only to a quoted delimiter.
- `setup/heavy --status` shows `[setup]` or `[product]` on every holder and queued job, and queued jobs in the order they are served.
- A guard (`setup/guards/deny-headless-chrome-keychain.py`, installed by `setup/install/install-headless-chrome-guard.sh`) refuses a `--headless` Chrome/Chromium launch without `--use-mock-keychain` and `--user-data-dir`: without them macOS pops a Keychain dialog. It reads the command line only; a launch from inside node code is not seen.
- `setup/heavy --short` warns when it is asked to run a `go test` with no `-run`, pointing at `setup/go-test-core.sh --affected`.

### Changed
- A brief that declares Judgment Day and carries `## Goal` and `## Done when` has the close tell the executor to hand both
  sections, verbatim, to each judge beside the diff; each judge answers per Done-when rule: held / broken (file:line) /
  not judgeable from the diff, and the report lists every "not judgeable" rule. A brief without both sections gets the
  close it had.
- `docs/BRIEF-TEMPLATE.md`: `## Done when` asks for the rules the behaviour must hold on real data and what the feature
  is for (a rule that can only break on real data is also a check in `## Testing / Verification`); `## Out of scope`
  carries the smallest-change line (`setup/CLAUDE.md` points at it); `## When you finish` says a dropped item is a `pass`
  row with `dropped: <proof>`, since done-invoker accepts only pass, fail or unresolved.
- `setup/gate-select … touched` lists first the subjects that spell a changed path whole (the narrowest path first), then
  those that name only its bare word, then those reached through the closure; cheapest within each. Same set, same cut. On
  foreman-friction-1106's range 1131 and 1014 move from #230 and #229 of 236 to #19 and #18.
- The CLI cockpit's cards read clean at 66 columns: the ruling input opens only on the card whose `ruling` button was pressed (and closes on a second press or once queued), the title is one line that truncates with `…` and leaves out a brief title that only repeats the task (`Brief: <task>`), the status keeps its width, the `▰▱` glyph bar and the trailing `· unknown` are gone.
- The reasoning level of a dispatch has one door on every vendor: `--effort`. A level written inside the model id
  (`--model opencode-go/deepseek-v4.1-flash:max`, pi's own CLI spelling) is refused before anything is built, naming the
  model, the level and the `--effort <level>` to use instead, instead of being dropped while the roster's own level won.
  The refusal is one check for claude, codex, opencode and pi, on `--model`, on a brief's `model:`, and in `hw next`.
  A `:` that is part of a model id (`:batch`, `:free`) is untouched.
- The dry run's effort line reads the same on every vendor: `effort=<value>  (chosen)` when `--effort` asked, and
  `effort=<value>  (default; <source>)` when a default supplied it — the lane's declared default for that vendor
  (pi's roster, by the brief's kind) or `the vendor's own`. The values are unchanged; pi still defaults to low.
- `setup/go-test-core.sh` waits at most 300 s for the batch lock when it runs under `setup/heavy` (`HW_HEAVY_RUNNING=1`), instead of 3600 s, so a gate holding a heavy slot fails within minutes naming the holder. An explicit `HW_GO_TEST_LOCK_WAIT` still wins.
- The executor sleep/poll guard now refuses a `go test` over `core/` in the background too when it names `./...` or has no `-timeout` of at most 10m, and points to `setup/go-test-core.sh --affected "$HW_ARTIFACTS/changed" --run`.
- `hw review-range` warns on stderr that the range is EMPTY when the base is HEAD (a `base: task/<x>` brief); exit 0 and the SHA stay.
- The executor prompt, `setup/CLAUDE.md` and `docs/BRIEF-TEMPLATE.md` say plainly that an executor verifies with touched subjects and partitions only and the full fast gate is the brainer's; they also say the gate reads a snapshot taken when it starts, and use `$HW_ARTIFACTS/changed` instead of `/tmp/changed`.
- The executor prompt line now gives the touched-subjects form (`HW_TEST_CHANGED=… HW_TEST_GATE=fast bash setup/test-hw --keep-going`, in the background) and says the full gate is the brainer's.
- `checks.md` rows are split only on unescaped pipes, so a `\|` inside a code span no longer shifts the verdict column.
- The guard message and `go-test-core.sh --help` give the one-test form `setup/heavy --short -- go test -count=1 -run <T> ./<pkg>`; a red partition prints its `--- FAIL:` lines right above the `not ok` line.
- `deny-blind-process-kill.py` refuses `pkill -f` on `go-test-core.sh`, `go test`, `test-hw` and `setup/tests/` (shared between executors) unless the pattern names the caller's own worktree path; kill by PID.
- `hw ruling` writes `RULING-STEER-FAILED` (vendor, exit, herdr's reason) or `RULING-STEER-SKIPPED` (vendor, why) to the run's `transcript.log` when a ruling for a pane that could be pi is queued unsteered, so a ruling that waits 40 minutes leaves a trace. The cause of the unsteered rulings in herdr-app-viewer is UNESTABLISHED.
- The executor prompt's pi preamble budget moves 5300 -> 5400 B for the touched-subjects form.
- `setup/go-test-core.sh --affected … --run` prints `estimated N s over M partitions` first; inside an executor (`HW_TASK`) it exits 75 over `touched_budget_seconds`, naming the focused `setup/heavy --short -- go test -count=1 -run <T> ./<pkg>`.
- `hw` warns at dispatch when a setup brief's Done when or Verification orders a run of `setup/test-hw` (the touched form with `HW_TEST_CHANGED` is the one mention that passes); the executor prompt's done-invoker line names `done-invoker --check`.
- `channel-send` refuses an endpoint that starts with `--` (`channel-send herdr <pane> --report - msg` took `--report` as the endpoint and waited 90 s); the executor prompt says frictions go to `mem_save` under `<lane>/<task>/friction` with the #id in the done summary, never by `channel-send`; `docs/BRIEF-TEMPLATE.md` says a new top-level path needs its `setup/export/manifest` line in the same commit.
- `setup/commit-tree` asks `setup/export/check` over the tree it is about to commit, in seconds, and
  refuses what the check refuses. It runs no hook, so a leak used to surface only in a gate of hours.
- `setup/CLAUDE.md`: before the gate, an executor that changes a message's text searches its old text in `setup/`, `core/` and `docs/` tests.
- `setup/install/install-gh-image.sh` builds gh-image from the audited source instead of downloading a release binary:
  it fetches the pin's commit by full sha, builds it with `go.sum`-checked modules (`-mod=readonly`, no cgo, an
  environment of its own) and installs it only if its sha256 is the pin's. `--from` is now a source checkout, refused
  unless its `HEAD` is the pin's commit. The pin records the commit, the Go toolchain and the build's hash
  (darwin-arm64); the old release binary is refused by `bin/gh-attach`. Reinstall with `setup/install/install-gh-image.sh`.
- The contract goldens (`golden-t4`, `golden-t5`) store the prompt's byte count as `@N@` (`deliver @N@ bytes`, `executor, @N@ bytes)`), in `contract_norm` and its in-process twin `inprocNorm`: a prompt-text change no longer rewrites ~96 goldens whose only change was that number.
- `setup/heavy`: setup jobs may take every free slot while no product job holds a slot or waits; the reserve (slots - 1, or `HW_HEAVY_SETUP_MAX` when set) applies the moment one appears. The memory floor is untouched.
- The headless-Chrome guard now runs on the shared tokenizer `sd.tokenize` (`deny-sd-shell-var.py`, installed beside it) instead of its own shlex code, with a shared corpus `setup/guards/shell-vectors.json` run by its test; an internal error refuses. It now sees `$(...)`/backtick launches, `do`/`then`/`if` keywords, `sudo -u x`/`timeout -s SIG` option values and `open -a "Google Chrome" --args`, and `2>&1` no longer splits a complete launch.
- `--reset`, `--resume`, `--self`, `--await-idle` and `brain relaunch` have no herdr-core mapping yet: run from an app
  terminal they are refused by name, with "Nothing was sent and nothing was restarted".
- A plain herdr TUI pane is unaffected, byte for byte: with no `HC_TERMINAL_ID` the brainer takes the workspace/tab
  path it always took and `hc` is never executed.
- KNOWN-LIMITATIONS L29: delivery to the app and the ask hold now work; `hw` dispatch, `hw next`, `hw done`/`hw reap` and the outbox flush on hc still do not.
- The herdr app's cockpit shows the whole text of a title, meta, progress, report or attention line on hover, since each is clipped with `…` at the panel's width. The task name (which wraps) and the folded subagents summary line have no hover.
- A cockpit row is wired to focus its executor's terminal on click when the app's core (`hc`) holds that terminal. Today the state writer records only TUI pane ids (`w1:p11`), which the app cannot reach (KNOWN-LIMITATIONS L29), so a click does nothing yet; it starts working once an executor runs in `hc` and the writer records its terminal id.
- The Issues panel (`herdr-app/mods/foreman-issues`) says each fact once, in all three looks: a row names its repo without the org, and only when the list mixes repos (a chosen repo chip, or a group of one repo, already names it); state shows only under `Any`; dates are one relative format (`5d`, `2mo`, `1y`) instead of `13d` beside `2026-09-04`; the title takes the freed width. The header drops the panel's name (the tab or the switch label says it) and shows its count only when a filter narrows the list or the 200 cap cuts it; `reading…` is said once. D and G lose the `Open 22 · updated ↓` band. The issue viewer shows the whole title, wrapped, under its head. The rules are plain functions in `rows.js`, held by `rows.test.mjs`.
- The Issues panel (`herdr-app/mods/foreman-issues`) draws its filters in two rows instead of four under a header bar: `updated` and Refresh (and the count, when a filter narrows the list) join the search row; Work/Personal, Involving/Author/Assigned/Mentioned and Open/Closed/Any share one row, a wider gap between the groups, wrapping a whole group at a time in a narrow panel; the repo chips keep their own row. The first issue row starts 88 px higher in C and G and 92 px in D at 816 px. Every control, count, selection and copy is unchanged. Held by `rows.test.mjs` item 12.
- The Issues panel (`herdr-app/mods/foreman-issues`) names each repo once: under `All repos` the rows sit under one quiet header per repo (its short name and count — C's label, D's band and G's row from `tokens.js`), in the repo chips' order, and no row repeats the repo; with one repo shown there is no header. The `#number` sits in a fixed-width column (the longest number's), so every title starts at the same x in C and D; G already aligned its titles under the row mark. Held by `rows.test.mjs` items 10 and 11 (`byRepo`, `numWidth` in `rows.js`).
- The Issues panel splits into Work (every repo of one configured organisation) and Personal (the rest), with the repo filter inside each group, and never lists one excluded repository; both rules are data in `foreman-issues/groups.js`, held by `groups.test.mjs`.
- KNOWN-LIMITATIONS L29: `hw next` and the outbox flush work on hc; `hw done`/`hw reap` still do not.
- The Herdr app's right column switches between Cockpit, Issues and Artifacts from a row of three labels (a layout `stack` plus `layout.select`), so the issues and artifacts panels no longer need a shell command. The file viewer and palette entries stay blocked by app limits (no mod commands in the palette, no file-open hook), stated in `herdr-app/README.md`.
- The Herdr app's default look is now `c-mono-signal` (pure black and white, Geist Mono, one cyan for what waits on the user), shipped as `herdr-app/themes/c-mono-signal.json` and chosen by the layout. The previous look is the built-in `graphite` theme; switching is the app's own `app.theme` from the palette, persisted, with no code.
- Herdr app panels, v2: an artifact now shows as it renders (Chrome headless, with a mock keychain and a throwaway profile, draws the page to a cached PNG shown scrollable at the panel's width); the issue preview, an artifact's text answer and the file viewer draw markdown through one renderer (headings, lists, code, quotes, comment blocks, GitHub-colored label chips, a state dot) in the active theme's tokens.
- The Herdr app's foreman layout follows Zed's grammar: spaces and agents on the
  left, terminals in the center, the cockpit in the right column, the status bar
  at the bottom.
- The four foreman mods (cockpit, issues, artifacts, viewer) share one look from
  an identical `tokens.js`: one type scale, hairline separators instead of
  bordered cards, status as a colored dot with neutral text.
- The Herdr app's foreman looks hold WCAG AA and one "waits on you" color. The cockpit paints an ask and a report with the theme's `done` (the app's own "waits on you" status) instead of the terminal's `ansi3` and the focus `accent`: look C draws both in its signal cyan (they were yellow and white, white like working), look G draws an ask blue instead of a yellow that matched working. Theme values: C `dim` `#7a7a7a`, `done` `#00e5ff`; D `dim` `#80848c`, `done` `#7c86e8`; G `chrome` `#2f343e` (Zed's dock color, where the app paints its sidebars and the cockpit), `dim` `#9a9eab`, `blocked` `#ea858b`. `looks.test.mjs` now holds AA for every look's text on `chrome` and `background` and keeps `done` apart from `working`.
- Each Herdr app look is now a whole window, as its approved mockup draws it, not only a palette: `herdr-app/layouts/foreman.json` (C), `foreman-d.json` (D, Linear) and `foreman-g.json` (G, Zed) bring their theme, fonts (C Geist Mono; D system UI + JetBrains Mono; G IBM Plex Sans + Lilex), region sizes and paint, and replace the app's own sidebar, tab bar, pane header and status bar with foreman parts drawn per look (`mods/foreman-cockpit/chrome.js`, `props.part`/`props.look`). The cockpit, Issues and Artifacts draw each look's rows, status glyphs (■/●/○, ◐/●/○/✕), sections and callout. Switch from Settings (each layout has a preview) or `ctrl-shift-l`. Theme changes: D `surface` `#1a1b1f`, G `chrome` back to `#3b414d` and `done` `#7fb6ec`. New `~/.config/herdr-app/layouts` symlinks: `foreman-d.json`, `foreman-g.json` and the three `foreman*.png` previews.
- A plain herdr TUI pane is unaffected: with no `HC_TERMINAL_ID`, the Stop hook and the invokers take today's path,
  byte for byte.
- `docs/pi-vendor.md` and `setup/export/overlay/docs/KNOWN-LIMITATIONS.md` (L3b): the specialists row names the plugin
  agents, and the "plugin skills are not passed in a product lane" line is corrected — the plugins a brief declares ARE
  passed (`-e`, skills included), only the user-level skills are not.
- `setup/tests/1273-pi-ve-los-agentes-de-los-plugins.sh`: the dry run names the agents, the adapter registers them
  (hermetic), and a LIVE pi session (real pi, real pi-subagents, a local scripted provider — no quota) runs
  `pstack:comment-sicko` with the registration and reproduces `Unknown agent` without it; the same session asks pi for its
  loaded skills (`get_commands`), so the plugin skills are proved reachable too.
- `setup/pi/test-hw-executor.mjs` mode `plugin-agents`, `core/t6_pi_test.go` `TestPiPluginAgentDirs`,
  `core/invokers/pi_transcript_test.go` (a plugin agent through `subagent`), and the closing's golden in
  `core/t5_review_plugins_test.go`.
- **Review children get the diff as a file, on every vendor.** pi-subagents' `reviewer` declares `tools: read, grep, find, ls`
  (no shell, by design — the file belongs to the npm package), so 3 of 4 review children of herdr-app-artifacts could not run
  `git diff`. Both closings now say: write the range's diff to `$HW_ARTIFACTS/review-range.diff` and give that path to every
  review subagent and judge. The agent sentence also says to pass `async:false` where the call has that field (both judges:
  the pi proof needs it explicit). A declared plugin whose agents directory path has a space or `:` is now said at dispatch
  (`NOT LAUNCHABLE here … has a space or ':'`) instead of dropped silently. Prompt-size budgets moved for the clause.
- A product lane that declares `vendor: claude` with an `account` and no `model` now dispatches Claude Code on that account (`CLAUDE_CONFIG_DIR`), with the model chosen by the default and the `kind:` rules. One lane moved from opencode to this; `--agent opencode` still works there, and live executors keep their vendor.
- `setup/heavy` serves a queued product-lane job (anything whose lane is not `setup`) before any queued
  setup-lane job, whatever their arrival order; FIFO inside a class. The lane is `HW_HEAVY_CLASS` — a caller's own
  declaration, cooperative and unverified — else `HW_PROJECT`, else a shell standing in the setup lane's own brain checkout
  (`HW_BRAIN_ROOT`, which is all a brainer's shell has), else the `setup-` tag prefix; an unknown lane counts as product.
- Setup-lane jobs together hold at most `HW_HEAVY_SETUP_MAX` slots (default: slots minus 1, minimum 1). A budget on setup,
  not a promise to product: the cap and the slot scan both use the caller's own `HW_HEAVY_SLOTS`, so a caller asking for fewer
  slots cannot reach one setup leaves free, slot 2 needs 20% free memory, and a product waiter takes whichever slot frees first.
- A setup job queued longer than `HW_HEAVY_SETUP_STARVE_MIN` minutes (default 60) is served ahead of product jobs, inside the
  reserve.
- A brief's `## Testing / Verification` fence pins only the touched subjects and partitions, minutes long, and not the full
  fast gate: `hw done` re-runs the pinned line under a 900 s ceiling, and the gate takes longer than that. The full gate is
  still the executor's last run, written once in `setup/CLAUDE.md`.
- `docs/BRIEF-TEMPLATE.md` tells executors to run `setup/export/check` before committing and to use `setup/mutation/safe-mutate` for old/new proofs.
- The cockpit card shows no model (instead of the sentence `n/a — hw has no model reader for vendor …`) for a vendor hw cannot read a model from.
- `setup/commit-tree`'s export refusal for a new or deleted file now says to edit `setup/export/manifest`, name it in the same call, and mentions `setup/export/check --tracked FILE`.
- `docs/BRIEF-TEMPLATE.md`: an "After merge (brainer)" section for installs that point at the main checkout.
- A queued ruling reaches a working claude executor between two tool calls: `bin/hw-stop-hook.sh post-tool` (a `PostToolUse`
  hook, merged into `~/.claude/settings.json` by `install.sh`) claims the queue and answers `additionalContext`, in order, once
  (`ruling-delivered.<n>` records `via=post-tool`). The turn-end and done-invoker routes are unchanged, and `channel-send --ruling`
  still refuses a working receiver. `hw ruling --help` and `CLAUDE.shared.md` say so.
- The in-process Go partitions of a fast gate run half the cpus at once (7 of 14, two cpus each), not a third,
  and start longest first. A partition measured 0.5-0.7 cpu of wall, so four at once left most of the machine idle.
- The suite's time budget gate estimates what actually runs. Retired subjects no longer count, and the Go
  partitions do. The SUITE ceiling now bounds the worst-case fast gate (a diff that reaches core/, every partition
  queued), not the full suite.
- A green Go partition's line names its wall: `partition <name> green (...) in <N>s`.
- `go-test-core.sh --affected` names, on stderr, each changed path that queues every partition, and why:
  the deps file does not list it, or every in-process test reads it.
- The full-suite budget estimate is back under its ceiling: 863 (`las-anclas-de-revive`) was budgeted at a flat 600 s and measures 178.69 s, so the estimate falls 2933 s -> 2862 s and the SUITE ceiling is lowered 2931 -> 2870.

### Fixed
- `setup/tests/510`: three mutant needles no longer matched the selector or the runner, so the subject died before its
  last mutants ran.
- `cockpit-state` publishes `model: null` when hw could not read the model (the receipt's `model_running` of `none — <why>`); the diagnostic stays in `hw receipt` instead of being drawn as the card's model.
- `setup/heavy` no longer reclaims, as "had reported", a job that started after its run's report:
  an executor reopened by a hand-written `channel-send` keeps its gate instead of being killed (143).
- `hw done`'s pinned verification is killed whole at `HW_VERIFY_TIMEOUT_SECONDS`: the process group and
  every descendant (heavy's jobs sit in a group of their own, which `timeout` never reached), exit 124.
- The pre-commit hook's closing list of red lines ("the red lines of the gate, again") now prints when the gate's output is larger than the pipe buffer (64 KB); a `grep -q` pipe under `pipefail` swallowed it.
- The fast gate queues the Go test job (`go test` over `core/`) before the bash subjects, so a red Go test is reported in seconds instead of after the whole subject sweep.
- The Herdr app's looks C, D and G work again: each layout (`herdr-app/layouts/foreman.json`, `foreman-d.json`, `foreman-g.json`) draws the window with the app's own mods (`workspace-chrome`, `spaces`, `agents`, `tabs`, `pane`, `workspace-status`), styled by the look's theme, fonts, region sizes and paint. The premium pass's drawn stand-ins (`foreman-cockpit/chrome.js`, `props.part`) are gone: they showed empty Spaces/Agents headings, no tab bar, no space and a stub status bar. `looks.test.mjs` now fails any layout that replaces those mods. Proved in a second, isolated Herdr app (own home, app socket and core): switch/create/rename/close a space, open/switch/close a tab, agent status and click-to-focus, status bar, Cockpit/Issues/Artifacts, 19/19 per look with the look switcher below.
- Cockpit rows have no hover: the app's tooltip drew the whole report as an unstyled bar across the window. A report and what waits wrap under the row instead (`hover.test.mjs`).
- A row with no model reads clean: hw's `null` and its `n/a — hw has no model reader` are not drawn as a model; the row shows `—` (D, G) or leaves the field out (C) (`state.test.mjs`).
- `channel-send hc`'s idle gate ignored `hc wait`'s exit code, so a failing `hc wait` was re-run in a hot loop until the budget ended; it now backs off a second and lets `hc get` decide.
- The cockpit no longer repeats a task's name as its title (`Brief: <task>`),
  draws an unknown effort as `—`, or prints an empty `report:` line.
- `hw` on the herdr-core route no longer looks up, or focuses, a herdr tab or space that happens to carry the task's label.
- `cockpit-state --print-path` inside an executor (HW_TASK set) names the brainer's file from `HW_INVOKER_PANE`, not the executor's own pane, and exits 1 with a stderr line when the file does not exist (it used to print a path that never exists and exit 0).
- `bin/gh-attach`: when `hw secret check` cannot run (hw-core not built) it names the build step (`setup/build-core --native`)
  instead of calling the cookie item missing; "missing" is said only when hw reports the item is not in the folder. Its store
  hint now says to type the command first and copy the cookie last, so the clipboard still holds the cookie when it runs.
- hw's two `authorizes:` refusals teach the rules: the Keychain-handle refusal says a Bitwarden item belongs in `secrets:`,
  the "more than three fields" refusal says one `authorizes:` line per entry. `docs/BRIEF-TEMPLATE.md` documents
  `authorizes:` and `secrets:` in its frontmatter block.
- `docs/BRIEF-TEMPLATE.md` gains a `## Limits` section (after `## Out of scope`) that holds the smallest-change line, the
  heading the close prompt already calls "the brief's Limits"; `setup/CLAUDE.md` points at it.
- **pi runs the plugin reviews its brief declares, by the same names a Claude executor uses.** hw names each declared
  plugin's `agents/` directory (`HW_PI_PLUGIN_AGENT_DIRS`, `<plugin>=<dir>`) and `setup/pi/hw-executor.ts` registers every
  file in it with pi-subagents as `<plugin>:<name>`; `subagent` with `pstack:comment-sicko` now resolves instead of
  answering `Unknown agent: pstack:comment-sicko` (measured live, 2026-11-09: three pi executors in a row improvised
  around it — a generic reviewer, an inline review, a `review-skip:`). A plugin's agents are not filtered by hw's
  framework prefixes, and keep the prefix: the bare `comment-sicko` is not registered.
- **`done-invoker` proves that run like a Claude one.** The proof was already there for a pi `subagent` call that ran and
  waited (`core/invokers/transcript_pi.go`); what was missing was a name it could ever see. A `review-skip:` is no longer
  the honest exit for a plugin agent pi can run.
- **The dispatch says when a declared agent piece has no door.** Without pi-subagents the `pi specialists` line adds
  `NOT LAUNCHABLE here: the review's agent pstack:comment-sicko needs pi's subagent tool…` instead of silently listing
  nothing. With it, the line names each plugin agent and its directory.
- **The review-agent sentence asks for the foreground, on every vendor**: `Launch the review agent … in the foreground —
  no background launch (run_in_background, async:true): a background call proves the launch, not the run`. Measured
  2026-11-09 (herdr-app-viewer): a `run_in_background:true` call was refused by the closing, costing 8.5 min and two
  more cycles.
- `hw … --agent pi` with a brief that declares plugins opens on the executor instead of on pi's inventory. The private
  agent directory such a launch already gets now carries `quietStartup: true`, so pi stops printing `[Context]`,
  `[Skills]` (every one of the ~68 skills thermo, ponytail and pstack ship), `[Prompt conflicts]` and `[Extensions]` on
  the first screen; pi's own diagnostics still print when pi is quiet (`showDiagnosticsWhenQuiet`), so a skill pi had to
  skip is still named. It is a settings field, not a flag: pi has no `--quiet`.
- A plugin whose skill an earlier declared plugin already ships travels as a per-run mirror of itself with that one
  `skills/` entry left out (`$HW_PI_SKILLS_ROOT/pi-plugins/<plugin>-<hash>`, links into the installed plugin). `thermo`
  and `pstack` both ship `thermo-nuclear-code-quality-review`; pi kept the first copy and printed a `[Skill warnings]`
  block for the second, which was the whole first screen. Nothing vendored changed, and pstack keeps its own copy for a
  launch that declares it alone.
- `PONYTAIL_QUIET_STARTUP=1` is exported to a pi launch that declares `ponytail`, which stops its extension's
  `Ponytail loaded: <mode>` toast; its statusline keeps naming the mode. No pi setting reaches that line.
- A pi executor whose provider quota is spent now reports the task blocked to its brainer: the adapter runs
  `done-invoker --blocked` once, naming the quota, the branch HEAD sha and the `checks.md` counts. Before, the run ended
  with only a `provider_quota` receipt row and the brainer found the dead pane by reading it.
- `deny_setup_brain_writes.py` no longer refuses `python3 - <<EOF … EOF` followed by brain's `bin/done-invoker` or
  `ask-invoker` run by path, which its own docstring allows. A script that merely names them as an operand is still refused.
- `ask-invoker` and `done-invoker` send nothing, and say why, when an ancestor process is a headless agent (`claude -p/--print`, `pi -p/--print`, `opencode run`): a probe nested in a run inherited its `HW_RUN`/pane and spent the real run's asks. An interactive agent among the ancestors is a real executor and is not refused.
- The setup executor's brain write guard judges each `;`/`&&`/`||`/newline piece: `mkdir -p .artifacts; cat <brain>/…` is no longer denied because of the read, while a mutator that writes brain, `cd <brain> && rm …`, pipelines into `xargs`, variables and interpreters stay denied.
- The headless-Chrome guard no longer lets `chrome --he''adless …` or `$(which chrome) --headless …` through without `--use-mock-keychain` and `--user-data-dir`.
- `hw done` on a run whose own `done-invoker` already closed it says so and verifies nothing, instead of alarming about uncommitted work.
- The test suite exports a short `HW_ENGRAM_HEALTH_TIMEOUT`, so subjects that call `ask-invoker` no longer wait 10 s per try.
- The app cockpit row no longer ends in `· unknown` when the agent status is unknown.
- A test fails when a `herdr-app/layouts/*.json` leaves out one of the app's 6 built-in mods without an allowlisted reason.
- The repo-write guard no longer refuses a read-only command (`sed -n` of a file reached through a symlink into a protected tree) just because a `cp` elsewhere in the same command line does not touch that tree; a write through the symlink is still refused.
- A ruling consumed by `done-invoker` is no longer lost to a truncated read: its text stays in the run dir as `ruling-text.<n>`
  (turn end and done-invoker both), and the held report names that path on its first AND last line, so `done-invoker … | tail -1`
  still finds it.
- Seven core files had no line in `setup/suite-data/go-test-deps.txt`, which turned a check of that file red.
  They are now listed as read by every partition, the conservative choice until they can be measured.
- The suite budget estimate (2870 s) counted 60 retired subjects (10344 s) that no gate runs, and none of the
  Go partitions that every gate reaching core/ runs.

## [0.7.3] — 2026-11-06

### Added
- A pi executor has `monitor` (a long command's matching output lines are events, batched into one wake per
  second, with a mandatory timeout and an event cap) and `job_list`, beside `bash_background` and `job_stop`.
  They keep the write, secret and sleep-poll guards and the output filter.
- Judgment Day is enforced in code when a brief declares it, on pi and Claude: `done-invoker` reads the judges'
  calls from the transcript and refuses a fourth two-judge round (the first plus two re-judgments) or a ninth judge run, and a re-judgment that adds
  a finding to the first judgment's frozen ledger (unless the judge marks it `fix-caused`, or the finding is at a file its own
  prompt names); a close reporting `ESCALATED` is accepted over either breach.
- A pi executor records at session start whether a background subagent can start (`pi_background_subagents`).
  A new live probe, `setup/pi/test-live-subagent-wake.mjs`, proves the wake with chord absent: idle, mid-turn and
  compacting.
- `bin/gh-attach <owner/repo> <number> <image>... [--comment "<text>"]` attaches screenshots to a
  PR or issue as GitHub user-attachments, with no commit, and prints their markdown. It runs the
  pinned gh-image (v1.4.1, sha256 checked on every run) with the session cookie only as the
  Bitwarden item `github-user-session`, resolved by `hw secret run`; the tool gets an
  environment of its own and is never started without the token.
- `setup/install/install-gh-image.sh` installs that build under `~/.local/share/hw/tools/gh-image`
  and refuses a checksum that is not the pin's.

### Changed
- A review is opt-in: a brief with no `review:` closes with none, and `review: none` needs no `review-skip:` line on
  any `kind` (a declared review is enforced as before).
- The sleep-poll rule is narrower, on Claude and pi alike: a foreground poll loop is allowed under `timeout`
  up to 120 s, and a standalone `sleep` is refused while background work is in flight (a state pi reports; Claude's
  hook cannot see it).

### Fixed
- The archive copy of a pi run's transcript no longer depends on the worktree sitting at `work/<lane>/<task>`:
  it lands beside the task's archived artifacts, takes the session the run recorded instead of the newest one on the
  machine, and the close says where it went. A `session_file` is copied only when it is a `.jsonl` under a pi session root.

## [0.7.2] — 2026-11-05

### Fixed
- `secret-unlock --auto` no longer calls every `bw unlock` failure a rejected master password. Only bw's own
  "Invalid master password." is one; a broken node/bw, a migration error, a server or network error and a timeout
  are reported as what they are, keep a live session and retry on the next run, and page the setup brainer only when
  the session has under 60 minutes left (or none), naming the machine's fix instead of asking for the password.
  `secret-unlock --setup` keeps the stored password when bw could not judge it.
- `hw secret status` runs a no-vault `bw --version` and reports a bw that cannot start.

## [0.7.1] — 2026-11-05

### Added
- A pi executor's declared Judgment Day is proved, not assumed: the dispatch writes `jd_agents_t<N>`, and
  `done-invoker` refuses the close unless the session records a call to `jd-judge-a` and another to
  `jd-judge-b` (agent, task, `async:false`). The builtin `reviewer` no longer stands in for a registered judge.
- The extension a plugin's child discovers: a plugin dispatch's private pi agent directory carries
  `extensions/hw-child-guard.ts`, so pstack's `agent` children are held to the executor's write guard and
  secret-read redaction.
- A pi close proves pstack's review agents too: a foreground call to pstack's own `agent` tool (its
  `subagent_type`) counts, so `pstack:comment-sicko` can run and be held to the proof.
- A pi executor runs a shell command in the background and is woken in the session when it ends, with
  its exit code, its last 20 lines and the path of its whole output (`bash_background`, the equivalent
  of Claude's `run_in_background`), and stops one with `job_stop`. The command runs through pi's own
  shell operations, at most 25 run at once, and a turn that ends while one runs is a legal turn end, so
  the executor stops and is woken instead of hand-rolling a poll loop.
- The sleep-poll rule reaches pi: a `sleep` over 30 s, a `while`/`until … sleep` loop and a foreground
  `tail -f` are refused by the same guard Claude's hook asks, and the refusal names pi's own
  background tool. An unbounded loop or follow is refused in the background too, and a bounded one is
  allowed, as before.
- A pi executor's subagents carry the lane's model tiers: `pi_subagent_models` in
  `projects.json` maps a Claude agent's `model:` (`haiku`, `sonnet`, `opus`) and any
  agent the lane classifies by name to a pi model and thinking level, registered in
  pi-subagents' own `model`/`thinking` fields. Explore agents read on the cheap
  model at off/low thinking; build and review agents run a strong model at
  medium/high. The receipt's `pi_specialists` row names what each specialist runs.
- A pi executor that reads more than 3 files in one request (or 40 KB of text)
  gets ONE steer toward the explorer, through pi's native
  `sendMessage(deliverAs: "steer")`, and the receipt records it
  (`pi_exploration_budget`). It is armed only where hw injected an explorer.
- Under `--keep-going` every red subject is re-run alone with `env -i` and labelled ENV-ONLY, FLAKY, STALE-AT-BASE or
  REAL, printed beside it in the final `not ok` list; at most `HW_TEST_LABEL_MAX` (6) subjects and
  `HW_TEST_LABEL_BUDGET` (600 s), and past the bound the label is UNLABELLED, never guessed.
- `hw` warns at dispatch when the paths a brief names, or the paths this task's branch already changed, overlap the
  unmerged branch of another LIVE task of the lane: the warning names the task and the files. It never refuses.
  (2026-11-04: two live tasks edited `core/invokers/transcript_pi.go` and `setup/pi/hw-executor.ts` at once.)
- `hw --dry-run` names the thinking level a pi executor will really run at, and warns when pi clamps the request or when
  it is above the pi-roster ruling. Measured 2026-11-04 on `opencode-go/deepseek-v4.1-flash`: off/minimal→low,
  low→low, medium/high→high, **xhigh/max→max** — three real rungs, correcting the 2026-11-01 ruling in `setup/decisions.md`.
- A setup executor can no longer write in the brainer's checkout of brain: with `HW_TASK` set and `HW_PROJECT=setup`,
  the Write and Edit tools, a Bash that writes there, and a `cd` into it are refused, in the Claude hook
  (`setup/guards/deny_setup_brain_writes.py`, registered in `.claude/settings.json`) and in the pi adapter. Reads,
  brain's `bin/` tools by path and the executor's own worktree stay allowed; the brainer is not judged.
- A pi executor's prompt says which guards judge it, from where, and how old they are: the lane root's copy, never its
  worktree's. The adapter is read once at session start, so a rebase or a ruling that changes it reaches only a new
  session; the sleep-poll rule it spawns per call is read then, from that same lane root; the two global hooks are also
  per call but from `~/.claude/hooks`. The line says to report which guard judged it instead of re-running a refused
  command. Measured 2026-11-04: a run whose branch had gained the sleep-poll guard still saw a `sleep 540` pass, because
  the adapter loads at session start and from the lane root. The pi prompt sample sat at exactly its 5000 B ceiling, so
  its budget moves to 5300 for this rule, with the reason in `core/t5l_prompt_sample_test.go`, and nothing else moves.
- `~/.cache/hw/bw-unlock.log` is a timestamped line per `--auto` outcome (`ensure`, `check`);
  a rejection carries bw's own error text with passwords, session tokens and `BW_*` values redacted.

### Changed
- The shared Go build cache is capped at 12 GB instead of 4 GB, so parallel
  worktrees no longer evict each other's builds and recompile on every gate.
- pi's two Judgment Day judges travel in ONE `subagent({workflow:"parallel",…,async:false})` call, the shape
  pi-subagents' parallel resource takes, because a second `subagent` call in the same turn is refused; the
  transcript reader counts that call as both judges, and none of `workflow:true` or a script path proves anything.
- pi's closing says so itself when `jd-judge-a`/`jd-judge-b` are not on disk, or when pi-subagents is absent and
  neither a judge nor a `reviewer` can run, instead of handing over names that are not registered.
- KNOWN-LIMITATIONS lists, row by row, every executor optimisation Claude gets with pi's equivalent or the
  reason there is none.
- The shipped text names no machine-local tool of whoever built it: `hw secret` and
  `secret-unlock` say you schedule `secret-unlock --auto` yourself, on macOS, instead of
  pointing at an installer this project does not ship, and `setup/heavy` and the runner
  describe "the memory guard" rather than naming one.
- `hw help advanced` and `docs/KNOWN-LIMITATIONS.md` call `--account personal` the second
  account, which is what it is.
- Judgment Day's judges now check trust boundaries: a value a diff reads instead of
  creates — an inherited environment variable, argv, a file, a directory listing, a
  message from a peer process — is a finding when the code uses it without
  validating it, and both judges carry the rule word for word, so the pair stays
  blind and symmetric.
- The pi manifest's `pi subagents` line names the tier table a specialist's
  `model:` resolves to (and the agents the lane classifies by name), instead of
  saying the model is not carried.
- `docs/pi-vendor.md` records that a LIVE pi session changes its own thinking level and model (`/thinking`, `shift+tab`,
  RPC `set_thinking_level`/`set_model`), so effort never needs a relaunch.
- `done-invoker --check` runs every close gate (the 900-character limit, `checks.md`'s verdicts, a
  declared design boundary, the plugin review proof) and delivers nothing, so a refusal is found
  before the work is done. `done-invoker --help` prints the real usage, `--help` included anywhere
  among the flags.
- Docs: the judgment-day skill names pi's single-call parallel form for the two judges
  (`subagent(workflow: "parallel", …)`), beside Claude Code's two `Task` calls.
- Docs: the brief template orders the close — commits, Judgment Day, its fix rounds, and the ONE fast
  gate on the final tree — and says every path is relative to the worktree; `setup/CLAUDE.md` names
  the changelog fragment path, `setup/export/overlay/changes/task-<task>.md`.
- Docs: a focused `-run` test of seconds runs as `setup/heavy --short -- <cmd>`, beside a
  held slot and never in the slot queue; `setup/CLAUDE.md` and `docs/BRIEF-TEMPLATE.md`
  say so with the 120 s cap.
- `secret-unlock --auto` leaves a session with over half its life left alone and says so.
- `hw secret check` on a locked vault makes the same single `--auto` try as `hw secret ensure`
  before refusing.

### Removed
- Every notification foreman sent: the dispatch, executor done/blocked, ask,
  `hw wait` and close-FAILED announcements, and the release cut's macOS
  notification. What they said is still where it was — the brainer's report,
  the pane's metadata, `hw status` and `close.log`.

### Fixed
- `hw help flags` described `--effort` as «claude and codex only», while the
  code carries it for all four vendors: claude's `--effort`, codex's
  `-c model_reasoning_effort=`, pi's `--thinking`, and opencode's
  `reasoningEffort` overlay. The entry now says what each vendor takes.
- An in-process test derives which vendors carry `--effort` from the command
  line the code builds and fails when the help entry names a different set.
- A pi dispatch whose brief declares a plugin runs on a private agent directory
  whose settings keep `theme`; the theme's JSON is now linked read-only into
  `<private>/themes/<name>.json` (the real `themes/` directory or a package's
  `pi.themes`), so pi no longer prints `Failed to load theme "<name>"` and falls
  back to the system theme.
- `hw revive` clears the run's `done` marker when it marks it reopened: `channel-send --report-state` refuses every
  report while that marker stands, so a revived run could not report at all (2026-11-04: the executor moved it by hand).
  The earlier report stays on record in the receipt and the ledger.
- The two files this branch adds under `core/` are classified in the export manifest and listed in the gate's dependency
  map; the contract page's moved `core/invokers/done.go` citations and the export's personal names are corrected, so
  subjects 606, 815 and 1014 are green again.
- `setup/gate-select`'s touched budget registers its two mutation anchors (510-T01, 510-T02) and each has its mutant
  arm: the cheap floor removed, and the cap spending itself slowest-first.
- `hw done` and `hw reap --apply` copy a pi run's newest session jsonl to
  `archive/<lane>/<task>/.artifacts/transcript.jsonl` before removing anything, so the transcript outlives the
  worktree (2026-11-04: no session to read after a close).
- The touched budget no longer cuts the cheapest subjects: `gate-select … touched` keeps every subject at or under
  `touched_cheap_seconds` (default 15 s) and spends the cap by taking the SLOWEST candidate first, so what is cut is
  always the most expensive thing left (2026-11-04: 3-13 s subjects were skipped while slower ones ran).
- An executor can run `hw <lane> <task> --dry-run`: a dispatch rehearsal creates nothing, so both the executor gate and
  the brain guard let it through, while every other refusal stays (2026-11-04: it was refused and the executor got past
  with `env -u HW_TASK`, five calls). The flag bypasses the gate only on the DISPATCH arm: a named brainer verb keeps
  its refusal even with `--dry-run`, because `sweep --apply` and `preview release` ignore the flag and change state.
- `setup/commit-tree` refuses, with `HW_TASK` set, a repository that is not `HW_WORKDIR`: an executor commits in its
  own worktree, never in the brainer's checkout (2026-11-04: a task commit landed on `main`). The brainer, with no
  `HW_TASK`, is unaffected. `hw` also warns at dispatch when a setup brief names the brain root as a place to edit.
- `setup/commit-tree` is near-linear in the number of named paths: the macOS case-collision check lowercases each
  prefix once instead of forking `tr` per pair, and the index is filled with one `hash-object --stdin-paths` and one
  `update-index --index-info`. A commit naming 635 paths took 31 minutes and never finished; it now takes 3 s.
- `setup/heavy` runs two slots by default (`HW_HEAVY_SLOTS=1` asks for one), so a waiter is no longer held
  behind a head that cannot use a free slot: a waiter may take a free slot the head's own
  `HW_HEAVY_SLOTS` excludes, and the head keeps priority for the slots it may use.
- `setup/heavy --status` lists every slot lock that exists, not only the caller's `HW_HEAVY_SLOTS`.
- `setup/heavy` no longer floods its output with `Broken pipe` lines: the queue is read once, never piped
  into `head`.
- `setup/export/check` also judges a new file under `setup/export/overlay/` before it is tracked, so a changelog
  fragment naming a term the export gate refuses is refused by the fast gate and not first found in review.
- The sleep-poll guard refuses a `while`/`until` loop that waits on `pgrep` (a process count that matches its own shell
  and other lanes' processes), in the background and under a `timeout` too; its refusals recommend only shapes it allows,
  held by a test.
- `hw status` names every guard installed under `~/.claude/hooks` whose bytes differ from its copy in `setup/guards/`,
  with the installer that fixes it, so a Claude executor is no longer left running an old guard unnoticed.
- A pi executor's `bash` call that sets no timeout is bounded at 600 s (`HW_PI_BASH_DEFAULT_TIMEOUT_S` changes it, `0`
  turns it off); a call ended by that bound says so and points at `bash_background`. pi's own `bash` has no default
  timeout, and two foreground probes hung 83 minutes with a ruling unable to land.
- `hw ruling` delivers to a pi executor that is working right away, through pi's native steering (`herdr agent prompt`;
  pi delivers it after the tool call in flight), records it as delivered (`via=pi-steer`) and leaves nothing queued for the
  turn end. A failed send leaves the ruling queued. Claude and OpenCode keep the turn-end queue.
- `setup/test-hw --keep-going` labels a red STALE-AT-BASE only when it also fails at the merge base (the check was
  inverted), finds that base in the live checkout (the runner's snapshot has no `.git`, so every red was called REAL),
  and removes the temporary base worktree it adds to the repo.
- The pre-commit hook no longer dies with exit 141 and no message on a merge that stages a file over 16 KB: it read the
  first line of each staged file with `printf | head -1` under `pipefail`, and `head` quitting early gave `printf` a
  SIGPIPE. A merge of main into a task branch could not be committed with `git commit`.
- The dispatch warning that two live tasks edit the same file also finds them when the work root is reached through a
  symlink (`/tmp` on macOS): it compared git's real worktree paths with the unresolved root and skipped every task.
- `setup/heavy` no longer reports its own `--when-idle` pause as a FROZEN tree when the pause begins while the watchdog is
  sampling the process table (the marker is now read after the sample, not before): on a loaded machine that raised a
  false FROZEN alert at once with a zero-minute threshold.
- A batch of `go test` partitions that finds the batch lock held no longer dies at once with exit 75: it waits for the
  holder, naming its pid and the bound (`HW_GO_TEST_LOCK_WAIT`, default 3600 s; `0` keeps the old refusal at once), and
  runs when the holder releases it. Measured 2026-11-04: 5 of 16 fast-gate launches ran every subject and then lost the
  whole run to this lock, while `setup/heavy` had already given the gate a slot.
- The Go tests a change touches are spelled the way they run, in `docs/BRIEF-TEMPLATE.md` and in
  `setup/go-test-core.sh --help`: `git diff --name-only main > /tmp/changed` then
  `bash setup/go-test-core.sh --affected /tmp/changed`. The briefs' `--affected <file>` named no file, and a shell read
  `<file>` as a redirect; `hw` now warns when a brief hands `--affected` a placeholder.
- A subject's mutant verdict can no longer false-red on a broken pipe. `printf … | rg -q` under `pipefail` reports the
  writer's EPIPE as the pipeline's status, so a killed mutant read as VACUOUS — measured 2026-11-04: 1150-G11 printed
  “VACUOUS — no test failed” beside `printf: write error: Broken pipe` and passed when run alone. The whole suite matches
  from a here-string now (`has_text`/`has_fixed`, `setup/tests/_common.sh`) — 74 call sites in 35 subjects, every
  `printf … | rg -q|rg -qx|rg -qi|rg -qw|rg -qF|grep -qF` of a captured output (`rg -qx` as `^…$`, `rg -qi` as `(?i)…`, a
  fixed string as `has_fixed`); the only one left is 1150's own reproduction line, and it says so. 47 was the fifth, and
  the gate lost a run to it on the same day.
- `setup/commit-tree` is linear in the paths it is handed. Its case-collision check compared every pair of every
  named arg's directory prefixes, lowercasing both sides with a `tr` fork per comparison: a 344-path commit forked about
  two million of them and ran 32 min without finishing (measured in a wait-free lane). One lowercase → first-original
  pass finds the same collisions and a 344-path commit now commits in 15.9 s; that pass keeps a TAB in a directory name
  and compares strings rather than awk's numbers, so `Ne<TAB>w`/`ne<TAB>w` and `1e5`/`1E5` are still refused
  (`tests/1233` G13b).
- A nested probe can no longer overwrite its parent pane's report. `done-invoker` and `ask-invoker` read the
  `hw_run` herdr records on the pane before publishing anything and refuse (exit 1, no token written) when it is not
  `$HW_RUN`: measured 2026-11-04, a pi probe inherited `HERDR_PANE_ID` and rewrote the live executor's
  `hw_task`/`done_state`/summary, and `hw status` read `done-NOT-delivered` for a pane that was still working. The
  probe recipe in `setup/CLAUDE.md` now says how to start one without the parent's pane environment. Old (check
  removed): rc 0 and both `pane.report_metadata` writes landed; new: rc 1, tokens intact (`pane_run_foreign`
  scenarios in `core/invokers/diff_{done,ask}_test.go`).
- `setup/go-test-core.sh --affected <file> --run` runs exactly the partitions it prints, as the batch they must be
  run as (the batch lock and the plan), instead of leaving the executor to run them one by one by hand — where the
  lock and the parallelism are skipped and a stale golden surfaced only in Judgment Day (measured 2026-11-04). The
  names are still printed, so the machine-readable use (`| paste -sd, -`) keeps working; `--help` carries the
  spelling. `tests/1100` G10 drives the fixture's `a` and `rest` through it.
- `setup/commit-tree`'s case-collision check reads the tree NUL-separated and compares in one python pass. A name
  with a TAB, a newline or a non-ASCII byte is C-quoted by `git ls-tree --name-only`, so `docs/new.txt` beside an
  existing `Docs/é.txt` was committed — the quoted line matched neither the file nor its directory (measured
  2026-11-04). `tests/1233` G17 pins the non-ASCII directory and the TAB-in-file cases; a 344-path commit still
  takes 16 s, not 32 minutes.
- The sandbox parity world is serialized by a lock (`/tmp/hw-sb-world.lock`) instead of colliding. Its fixed path
  cannot move: the state-directory hash derived from it is in the recorded transcripts under `core/testdata/sandbox/`.
  Two concurrent runs made subjects 1040/1041 red in the gate and green alone (~85 min of gates, 2026-11-04). Two
  `1040` runs started at once now pass (154 s, one waiting for the other); at HEAD without the lock the same pair is
  red on both, on different scenarios.
- `done-invoker` warns (never refuses) when a declared Judgment Day's judges are proved but no single call launched
  them together: the transcript proof now keeps each call's group, so two serial judge calls are named. Measured
  2026-11-04 (no-notifications): the two judges ran one after the other twice, ~13 min lost each time.
  `core/invokers/pi_transcript_test.go` holds both fixtures — `pi_judges.jsonl` (serial) warns, one
  `workflow:"parallel"` call (and one Claude assistant message with both Task blocks) does not.
- The executor guard (`setup/guards/deny-executor-sleep-poll.py`, asked by Claude's PreToolUse/Bash hook and by the pi
  adapter) refuses a raw FOREGROUND `go test` whose target is `core/` — `go test ./core/...`, `cd core && go test …`, or
  a bare `go test` from core — and names `setup/go-test-core.sh` (`<partition>`, or `--affected /tmp/changed --run`)
  and the background route. Measured 2026-11-04: 8- and 10-minute `go test` runs held the turn and skipped the batch
  lock. The runner itself, a background run and a non-core target pass. `tests/1133` section j holds it, and the live
  probe (`setup/pi/test-live-gotest-guard.mjs`) shows a real pi refused where the old adapter ran the command.
- A new subject, `tests/1245`, lints every `# gate-when:` header: the regex must compile, and no alternative may
  name a directory a changed path can never be (a trailing `/`, or a fully anchored bare directory — the shape Judge A
  found on 2026-11-05, where a subject silently never ran for the files it claimed). It also runs over this tree's 37
  headers and requires zero flags.
- 1130Ai: the cut's cap is 40 s, not 20 s, so the test no longer goes red under load (measured 3 of 5 red at 20 s with eight concurrent spinners; 5 of 5 green at 40 s).
- The gate's own reds from this task are fixed: the sleep-poll guard no longer spells the lane word `cur` as a local (173), `core/invokers/panerun.go` and `setup/pi/test-live-gotest-guard.mjs` are classified in the export manifest (606), and the `done.go`/`ask.go` citations in `setup/contracts/hw.md` follow the moved lines (815).
- The serial-Judgment-Day warning groups judge calls by the assistant message, not the transcript line: Claude Code writes each tool_use of one parallel message as its own line sharing `message.id`, so a correctly parallel pair was warned as serial (Judgment Day, 2026-11-05). `pi_transcript_test.go` holds the real shape (same id: no warning; two ids: warning).

### Security
- The export's term list refuses the names of the installer and of the memory guard this
  release stopped pointing at, so a cut that leaks either name again fails the fast gate.

## [0.7.0] — 2026-11-01

### Added
- `setup/mutation/l26-inventory.md`: the 90 retired mutation claims of 0.6.0's deleted bash bodies, classified row by row — 20 now
  mutation-held (12 in 0.7.0, 8 more here), 36 guard branches still owed (each naming the fixture it needs), 22 held by a golden a
  mutant would move with it, 16 bash-only or n/a.
- Go mutants `go-806-M01..M10` (brain relaunch, panic exits and the resume's EXIT trap), `go-67-M06` (chaining default) and
  `go-185-M01` (the speckit phase note must not leak into `--sdd none`), registered in `setup/mutation/mutation-anchors.tsv` and
  killed in process by their corpora.
- Six mutants of the brief's remote-marker guard, `844-A170..A175` (`core/t5_brief.go`), killed by the new contract cases
  `br-au-none` (a body naming `prod` with no `authorizes:`), `br-au-present` (the same body with one), `br-au-product` (a word that
  only contains `prod`) and `br-au-fm-marker` (a marker only in the frontmatter): the warning, its refusal form, its absence when
  the brief authorizes, the manifest's `— but the brief names prod`, the word boundary, and the frontmatter exclusion.
- Two mutants of `hw status`'s reopened run, `826-G35` (the table row) and `826-G36` (the row after its pane is gone), killed by the
  `status-hygiene` case whose golden already carried both `t-reopened` fixtures; `mutants-c825.jsonl` regenerated.
- One mutant of the pi brain-guard manifest line, `844-M51` (`core/t5_manifest.go`), killed by `pi-kind-build`.
- `setup/commit-tree [--onto <ref>] -m <msg>|-F <file> <path>...` commits exactly the paths you name: the tree
  and the parent come from the same ref, the mode is computed (100755 when executable), a path deleted on disk
  is removed, a diff touching a path you did not name is refused with exit 1 and nothing written, and the ref
  moves only if it still holds the commit the tree was read from. It never runs a hook.
- `setup/tests/1233-commit-tree-solo-lo-que-nombra.sh` reproduces the hand-typed recipe deleting a file that
  landed between `read-tree` and `commit-tree`, and holds the helper's behaviour against it.
- `hw status` lists a setup run that reported done until its retro is in engram
  (`topic_key: setup/retro/<task>`, exactly that key; one engram check for the whole
  list, across project labels) — a reminder, never a block. `hw retro-done <task>`
  clears it by hand.
- Executors get their own `TMPDIR` under `$HW_ARTIFACTS/tmp`, created at dispatch for
  every vendor, so fixed `/tmp/<name>` proofs cannot collide across parallel runs. The
  shared Go build cache and the shared lock directory stay shared, and `done-invoker`
  does not count `tmp/` as artifacts.
- `core/main_test.go` refuses a `go test` over core/ that selects the long in-process contracts (TestC848, TestC840,
  TestC870) outside `setup/go-test-core.sh` without their shard cut, and names the script and `--affected <file>`.
- `hw`'s dispatch preflight refuses a brief whose `## Verification` runs `go test` selecting an in-process test.
- A lane's executor rules live in `brain/<lane>/.claude/CLAUDE.md` and `hw` reaches every vendor it can: a symlinked
  `.claude/CLAUDE.md` in every worktree or work directory of the lane (Claude Code), and an
  `--append-system-prompt <file>` on the pi launch line and on `hw revive`. The dispatch manifest and the receipt print
  one `lane_rules` line naming that route, or `NOT APPLIED — <vendor> …` for opencode and codex.
- The lane's `.claude/agents`, `.claude/skills` and `.claude/CLAUDE.md` are linked in a lane with no product repo; their
  paths are named once in the checkout's `.git/info/exclude`, with no lane-specific exclusion code.

### Changed
- The eight small tools run Go with no switch: the Stop hook, `hw secret`, `hw-sandbox`, `browser-close`,
  `brain-archive`, `secret-unlock`, `opencode-auto` and `codex-channel-session-hook.sh` are now shims that exec their
  `hw-core` verb, like `channel-send` since 0.6.0. `HW_IMPL_<TOOL>=bash` or a `<tool>=bash` line is read only to say
  once that bash was removed, and Go runs; it never stops the command. The way back for one of them is
  `foreman-sh upgrade --to 0.6.3`.
- `hw secret put` is in Go: the value still comes from a pipe on stdin, travels to `bw` on its own stdin
  base64-encoded, and never in argv.
- A `mem_save` of `decision`, `architecture`, `discovery`, `bugfix`, `pattern` or `config`
  is refused by `bin/engram-label-proxy` when it carries no `topic_key`, and the refusal says
  how to add one (`mem_suggest_topic_key` proposes it); `session_summary`, `passive`, `manual`
  and every other tool pass untouched.
- `hw status`'s memory audit measures the flow, not the legacy: observations without a
  `topic_key` count only over those six types and the last 30 days (`HW_ENGRAM_NOKEY_DAYS`),
  and the line names the window it measured.
- KNOWN-LIMITATIONS L26 now carries the measured counts and points at the inventory instead of "about 90".
- `TestBrain806EachPaneKeepsItsOwnAccountNeverTheCallers`: the default-account lane's pane now runs under a NON-default
  `CLAUDE_CONFIG_DIR` (a third account directory), so the pane-reading branch `core/brain_launch.go` is the one that decides and
  `go-806-M03` is killed again on a base that carries the lane-account relaunch (`27488505`). The old world let the lane branch decide
  and the default account stay empty, which the mutant survived.
- pi's `build` and `design`/`audit` dispatch kinds default to
  `opencode-go/deepseek-v4.1-flash`, keeping the roster's thinking default, and
  fall back to `opencode-go/deepseek-v4-pro` on a model error; a brief that
  declares no kind is a build and does the same. `mechanical`, which ran the
  build model, now runs `opencode-go/deepseek-v4.1-flash` too (the 2026-10-11
  roster's row for it) and keeps its `glm-5.3-flash` fallback; `review` and
  `explore` keep the models and fallbacks they had.
- `hw`'s dispatch preflight refuses a `kind: build` brief with `review: none` unless it carries a
  `review-skip: <piece> — <why>` line. `docs/BRIEF-TEMPLATE.md` defaults `review:` to `[judgment-day]`, and its
  Verification names `c843` / `c848-1..8` for t5 goldens and mutants instead of `--affected <file>`.
- `foreman-guide` says when goldens are regenerated: at the end, after Judgment Day, from a frozen `hw-core`.
- `_skills/retro/SKILL.md` finds pi runs under `~/.pi/agent/sessions/`, not only
  `~/.claude*/projects/`.
- `setup/CLAUDE.md` names `setup/go-test-core.sh` (`<partition>` or `--affected`) as the
  only way to run Go tests; `docs/BRIEF-TEMPLATE.md` points to it.
- The invoker env-file whitelist adopts `HW_LOCK_DIR`.
- A pi executor registers the account's user-level agents (`$CLAUDE_CONFIG_DIR/agents`, default
  `~/.claude/agents`) with pi-subagents next to the lane's, so `subagent` with agent `jd-judge-a`,
  `jd-judge-b` or `jd-fix-agent` works as it does for a Claude executor. The lane's agent of the same
  name shadows the account's. The `jd-` agents are no longer filtered out of that registration.
- A pi specialist that lists `WebFetch` gets pi-web-access's `fetch_content` (the child loads that
  package), where it used to lose it. `WebSearch` stays dropped: not measured working.
- A pi executor starts with the account's `CLAUDE.md` as a context file of its run (pi's own slot for
  user instructions; nothing is written under `~/.pi`), and the receipt names the file (`pi_global_rules`).
- `done-invoker`'s pi proof counts a `subagent` call only when it ran something: no `action` (get, list,
  status, update… are management), `async:false` (pi-subagents runs async by default, and an async call's
  result only says it was launched), and agent + task for a single call, each element of `tasks`, and each
  step (and `parallel` step) of `chain`; `workflow:"review"` proves the `reviewer` agent. `workflow:true` or
  a script path proves nothing foreman can read: `review-skip` is the exit. A `chain` step without a task is
  not counted either (a stated limit).
- The pi session is accepted only when its header `cwd` is the run's workdir, and a session file that cannot
  be opened is reported as unverified instead of as a smaller proof. The refusal for a pi session names pi's
  mechanism (the `SKILL.md` read, `subagent` with agent, task and `async:false`).
- The dry run's `pi specialists` line reports the agents registered from the account's directory as well as
  the lane's, each with its directory, and appears when only the account's directory has agents.
- Known limit (measured, pi 1.4.0 / pi-subagents 0.77.0): a pi child still has no `Task` and no `mcp__*` tools. MCP for children needs background children, which
  need `@earendil-works/chord` in the host pi.
- The plugin-review refusal describes the check in vendor-neutral terms, and each vendor's exact rule is
  documented next to its reader.
- The text hw puts before the brief in every executor prompt is shorter and
  per-executor: each executor reads only the blocks that apply to it (its
  vendor, its lane, what the brief grants, how the task closes). Measured on a
  pi setup build: 9,428 -> 4,996 bytes of preamble; on a claude build with
  QA: 9,964 -> 5,693 bytes.
- A re-tasked executor (`hw next`) now gets the same text a fresh launch gets:
  a pi executor hears the pi specialists note, and a brief with `secrets:`
  gets the grants block. It also saves its findings under its own topic key
  (`<lane>/<task>-t2/findings`), so a second task no longer overwrites the
  first task's findings.
- `done-invoker` and `ask-invoker` count characters, not bytes, against their
  900 and 600 limits, so accented text is no longer refused early. The prompt
  now states the limit.
- `done-invoker --blocked` followed by a plain report now tells the executor to
  wait for `hw ruling` instead of suggesting `--retask`.
- `hw review-delta` says what to do on every refusal instead of only exiting.
- The `checks.md` reminder in `done-invoker` names what to list: one row per
  requirement of the brief's Verification section and each numbered goal.
- A setup executor's prompt again says to run the fast gate
  (`HW_TEST_GATE=fast bash setup/test-hw`) before closing unless the brief's
  Limits forbid it, and that a red it prints is the executor's to fix.
- A pi executor is told its Judgment Day judges are the registered `jd-judge-a`
  and `jd-judge-b`, and that every subagent call to a judge or plugin review
  agent carries `agent`, `task` and `async:false`, or the close does not count
  it as proof.
- `--sdd` is optional and `none` is the default: a dispatch without it runs outside any
  framework, even where Spec Kit's `speckit-*` skills are on disk (it used to refuse there
  and ask for a mode). `--sdd speckit` stays, opt-in, and is still refused where its skills
  are not on disk or the vendor is not Claude Code.
- An OpenCode or Codex dispatch without `--sdd` on a repository that has Spec Kit installed
  is no longer refused as if it had asked for Spec Kit, and an OpenCode Sol or Astra
  dispatch without `--sdd` gets its orchestrator agent, as `--sdd none` always did.
- `setup/opencode-hw-*.js` starts `hw` and `herdr-rpc` through the same `execFileP` on every platform: the Git
  Bash shell lookup and its 10x timeout are gone. macOS and Linux behaviour is unchanged.

### Removed
- The bash bodies of the eight small tools, `bin/wrapper-go.sh`, and the per-tool bash switch they implemented.
  Their subjects now hold the Go against the transcripts the bash recorded (`core/testdata/wrappers/`,
  `core/testdata/sandbox/`, `setup/tests/hw-secret-parity/golden/`), and the bash-only mutation anchors left
  `setup/mutation/mutation-anchors.tsv` for their Go holders.
- The duplicate `HAND=` ports of `setup/suite-data/go-test-partitions.txt` no gate ran: `TestC847`, `TestC847Mutants`,
  `TestInProcC862_866`, `TestInProcC862_866Mutants` and `TestC843Mutants`, with their 1,400 lines of test files.
  `TestC844Contract` stays: it is the entry point the live `TestC844Mutants` spawns in the mutated binary.
- The Windows code, ruled out 2026-10-16: the `core/*_windows.go` files, `core/invokers/*_windows.go`,
  `bin/msys-compat.sh` and its source lines, `setup/mutation/mutation-exempt-native-windows.tsv` and
  `.github/workflows/windows.yml`. Subjects 208 and 609 stay: they force the Windows path model on POSIX and
  still cover branches the tool keeps.
- The "this task runs outside the framework flow" paragraph is no longer prepended to a
  `--sdd none` executor's prompt; the unattended-pane paragraph stays.
- More of the Windows code the 0.7.0 deletion left behind: `bin/sitecustomize.py` (the msys path layer)
  with its export, deps and test references, the `MINGW*|MSYS*|CYGWIN*` branch and the `cygpath` fallback in
  `install.sh` (a native Windows shell now gets the same "not supported" answer as any unsupported platform),
  the `.exe` lookup stanza of the eleven `bin/` entry points, the Windows-only CI helpers (`setup/ci/`), the
  native-Windows mutation exemption with its deleted list, and the dead `runtime.GOOS == "windows"` branches in
  `core/` and the `msys-compat` comments that outlived the file they named.
- The Windows-only runtime: `bin/herdr-rpc`'s named-pipe transport and the `os.name == "nt"` selection,
  the pipe half of `setup/tests/_herdr_endpoint.py`, the Windows branch of `bin/invoker-common.sh`'s
  `usable()`, the Git Bash special cases in `setup/guards/deny_repo_writes.py`, `setup/test-hw`'s
  `taskkill` witness cleanup, the Job Object/`taskkill` cap in `setup/guards/lane_housekeeping.py` and
  subject 640's no-`killpg` arm that tested it, the `msys`/`cygwin` branches of subjects 153, 176, 186,
  199, 217, 251 and 1123, and the comments that cited them in `_common.sh`, `install.sh`,
  `bin/engram-serve.sh` and the fragments. `setup/tests/contract-hw/native.sh` (Windows-only) and its
  `go-test-deps.txt` line are gone.
- The read-only guard refuses at import on every platform but macOS and Linux (a Cygwin/MSYS Python
  answers `os.name == "posix"` and read a `C:\…` write as a relative, allowed path), and its JavaScript
  half no longer resolves `/usr/...` through `HW_MSYS_ROOT`.
- What deliberately stays: the native-Windows path model — `core/worktrees.go`'s
  `HW_CORE_PATH_RULES=windows|posix` and the guard's spelling folding that subject 208 forces with
  `DENY_REPO_WRITES_AS_NATIVE_WINDOWS=1`. The fast-gate ceiling's own Windows branch also stays, and
  subject 609 forces it with a simulated `win32`; it is process control, not a path model.

### Fixed
- `hw secret put` fails closed: when it cannot read the existing item from the vault it stops
  with an error instead of storing the new value over it.
- `brain relaunch` carries the lane's account: a brainer whose lane declares `account: personal`
  is resumed with the lane's `CLAUDE_CONFIG_DIR` and finds its conversation, even when its pane
  shell lacks that variable (before, `brain relaunch --all` left such brainers as bare shells
  printing «No conversation found with session ID»). The transcript lookup tries the lane's
  account first.
- The incremental verdict reads a `# MUTATION-ANCHOR:` marker as a declaration only inside a real shell (or Python) comment: inside a heredoc body, inside a string that spans lines, or on a `\` continuation it is text, so the plan now pays a full run instead of trusting a change that moved the answer.
- A pre-commit `all` gate, and a `mutants-*`-only commit, now run every slow subject whose `# gate-when:` matches that path or whose `# gate-reference:` names it (a `setup/tests/_common.sh` change brings eleven, about four minutes; any `core/*.go` change brings 1014 and 540): `setup/gate-select` makes its gate-when pass before answering `all`, and `setup/test-hw` reads the `reference` line beside it (subject 540, the mutation-anchor registry).
- The mutation-anchor subject's orphan scan covers non-subject files under `setup/tests/` (`_common.sh`, the `contract-hw` helpers): an unregistered marker there is refused instead of accepted, by the full run: 540's `# gate-when:` names no `setup/tests/` helper, so a commit touching only those does not run it at pre-commit.
- A pi executor can run a pstack review agent again. pstack's `agent` tool starts its children with extension
  discovery ON, so a child found the ambient `rpiv-ask-user-question` package and its `ask_user_question` collided
  with the one the vendored pstack extension registers (`setup/vendor/plugins/pstack/pi/ask.ts`): the child died at
  startup and `pstack:comment-sicko` never ran. A pi dispatch whose brief declares `plugins:` now runs on a private
  agent directory under `$HW_ARTIFACTS/pi-agent` — the real one's files symlinked, `settings.json` copied without
  `packages` or `extensions` — so a plugin's children discover nothing ambient. `~/.pi` and a plain pi session are
  untouched, and a session with no plugin keeps the ambient directory. `core/t6_pi_shadow.go`; the dry run's
  `pi agent dir` line says which one a launch gets.
- That private directory is this run's own: a dispatch from inside another hw run no longer puts it in the parent's
  `.artifacts` (where the parent's `hw done` deleted it under the child). It carries no lock directories, and a real
  `settings.json` that is not valid JSON still gets the private one with no packages instead of the ambient directory.
- A dispatch made from inside such an executor no longer builds its private directory out of the parent's: the real
  agent directory travels beside the private one (`HW_PI_REAL_AGENT_DIR`) and `piAgentDir` prefers it, and a child
  without plugins is handed the real directory instead of inheriting the parent's (which the parent's `hw done` deletes).
  Preparing a reused private directory also drops links to names it now skips (`auth.json.lock`), which pi would
  otherwise lock through.
- `HW_PI_REAL_AGENT_DIR` is trusted only when the inherited `PI_CODING_AGENT_DIR` is a run's private directory and the
  value is a readable directory; an explicit `PI_CODING_AGENT_DIR` always wins over a stale variable. If the private
  directory cannot be made, the real directory is exported (never an unchecked value) and the warning names the
  directory pi gets. A non-pi child of a shadowed parent gets the real directory and no `HW_PI_REAL_AGENT_DIR`; a
  revive, a re-task and `hw --sandbox` resolve pi's directory the same way.
- Known limitation: the private directory links `auth.json`, but pi locks it at the link's own path (`auth-storage.js:50`, `:91-92`,
  `realpath: false`) and offers no way to point auth elsewhere (`config.js` `getAuthPath`), so the lock does not exclude
  your own pi sessions' token refresh: two simultaneous OAuth refreshes can invalidate the rotated refresh token and log
  one session out (`invalid_grant`), not only "the later write wins". The file stays one (pi writes it in place, `auth-storage.js:71`, `:147`).
  `KNOWN-LIMITATIONS.md`, L3b.
- `setup/go-test-core.sh --affected` refuses a source file (`*.go`, `*.sh`, or lines with spaces/braces/parens/
  quotes) with exit 2 naming the mistake; a real changed-paths list that reaches nothing (docs only) is still an
  empty answer, rc 0. `hw`'s brief preflight refuses the same shape in `## Verification`.
- A brief that names `brain/setup/CLAUDE.md` gets a warning that inside a setup worktree the lane file is
  `setup/CLAUDE.md`; `docs/BRIEF-TEMPLATE.md` now points at the worktree-relative path and at `setup/commit-tree`.
- `setup/tests/1101-cada-caso-de-golden-t5-lo-corre-el-gate.sh` holds that every `golden-t5` case is exercised by a
  gate partition (`c843`), so the contract's goldens cannot sit outside the gate.
- The contract worlds no longer depend on the ambient `TMPDIR`: they are made under a fixed root (the suite's `~/.cache/hw-test-<uid>`), so a long `TMPDIR` (every executor's) no longer moves the byte counts the goldens record (`c843` was 58 red).
- `regen --review` streams the diff instead of rewriting paths in a captured variable (bash 3.2 was super-linear: a 9 MB diff did not finish in 60 s, now 0.7 s), and prints `case i/N` on stderr.
- Mutant `844-R142b` guards the direction R142 lost: a global speckit install IS detected (`speckit (global)`), with the new t5 case `rt-reuse-nosdd-home`.
- The read-only guard's refusal on cygwin/msys/win32 no longer yields to `DENY_REPO_WRITES_AS_NATIVE_WINDOWS=1`
  (Python and JavaScript halves): on a supported host the variable changed nothing, and on an unsupported one it
  turned the refusal off. It only forces the native-Windows folding on macOS/Linux now (subject 208). The guard's
  test asserts the import still refuses under `sys.platform=cygwin` with the variable set.
- `setup/commit-tree`'s Python syntax check parses with `python3 -I … ast.parse` instead of
  `python3 -m py_compile`, which imported `py_compile`/`traceback` from the repo root or `PYTHONPATH` and ran them.
  It selects files like `setup/hooks/pre-commit` (`#!…python`, `#!/bin/sh`, `#!/usr/bin/env sh` too) and skips
  symlinks (subject 1233, G16).
- The in-process harness's TZ fix has a wiring guard: `TestRunGuardedHandsTheCaseItsTZ` goes through `runGuarded`
  with `time.Local` pre-set to a non-UTC zone and asserts a `TZ=UTC` case sees UTC (it fails with the
  `defer inprocTZ(getenv)()` deleted). The dead `getenv == nil` branch and the long comment are gone.
- `setup/tests/contract-hw/regen*.sh` run under the suite's `TMPDIR` (the goldens record byte counts that include
  absolute paths), `world.sh` defaults `HW_GOCACHE_DEFAULT` so a regeneration no longer dies unset, and a case (or
  set, for `regen.sh`) name nobody has exits 2 naming it instead of returning 0 having regenerated nothing
  (subject 1000, G05 and G06).
- `setup/suite-data/test-budgets.json` carries subject 1101's budget (4.39 s, measured through `setup/heavy`), so
  pre-commit stops refusing every commit for the missing entry.
- `setup/tests/contract-hw/golden-t4` is regenerated from the tree's `hw-core` for the 52 cases that had gone stale:
  the `--sdd` help and usage text (`done-usage-bare`, `preview-help-long`) and the world listing of 50 cases whose
  `tree` diffs moved by two lines. `setup/go-test-core.sh t4-1` is green again.
- The in-process contract harness gives each case its `TZ`, so a
  `ruling --dry-run` prints the world's zone and not whichever zone the
  process happened to initialise `time.Local` with first (corpus 840's three
  `rl-blocked-* --dry-run` cases differed from the golden in a whole-package
  run only).
- Corpus 870's outbox world stamps the two `alpha` worktree envelopes like
  every other run, so the listing's order no longer follows the wall-clock
  second the template was built in (`ob-all`, `ob-list`, `ob-alpha`,
  `ob-alias` and `ob-to-value` could differ from the golden in `out`).
- A pi executor's plugin review is proved from its own session (`session_vendor=pi`). A skill is a read of its
  `SKILL.md` under `<plugins>/<p>/skills/<n>/SKILL.md` (proving `p:n` and `n`) or `.claude/skills/<n>/SKILL.md`
  (proving `n`), a partial read included; an agent piece is a `subagent`/`Agent` call of that exact name. A call
  counts only once its own `toolResult` arrived without an error, so an unknown agent proves nothing. A skill named
  only in prose is not a run, and only what ran at or after the receipt line counts. The session is the file's path
  the receipt records, read only as a literal `.jsonl` regular file under pi's sessions root
  (`$PI_CODING_AGENT_DIR/sessions` when set, else `~/.pi/agent/sessions/`); other plugin layouts are not recognised
  and need `review-skip`.
- A pi executor's plugin review is no longer credited by a call that failed: a skill read or an agent launch
  counts only once pi's own result for that call arrived without an error. A session whose unknown-agent call
  failed used to prove that agent had run.
- A pi skill is proved only by the two `SKILL.md` layouts foreman uses,
  `plugins/<plugin>/skills/<name>/SKILL.md` and `.claude/skills/<name>/SKILL.md`; any other `SKILL.md` proves
  nothing. A partial read still counts.
- The pi session foreman reads is the file path the receipt records, under the executor's own pi agent directory
  (`$PI_CODING_AGENT_DIR/sessions` when set, else `~/.pi/agent/sessions`), including every run of its subagents.
- An unreadable pi session is reported as unverified instead of reading as a session that ran nothing.
- `done-invoker` and `ask-invoker` run from a brand-new worktree: they use the
  main checkout's built invokers instead of failing with "the Go invokers are
  not built".
- `done-invoker --help` and `ask-invoker --help` print their usage and exit 0
  instead of refusing "unknown option: --help".
- `hw next --brief` names the re-task by its own brief (`<lane>/<brief>-t2`) in
  the prompt header and the findings topic key, not by the first task.
- `hw next` reads a brief's `secrets:` with the same parser the launch uses; an
  item that starts with `-` is warned about and left out instead of silently
  kept.
- A lane with its own repo no longer has its `.claude` replaced: a repo's own worktree script links
  `.claude/{CLAUDE.md,agents,skills}` from its checkout and `hw` used to relink them to the brain's, so the executor
  lost the repo's specialists and skills. `hw` now removes a symlink only
  when it points under `brain/<lane>/.claude` (one it made); any other entry is kept and reported. Such a lane gets only
  `.claude/CLAUDE.md`, and only where the worktree has none; agents and skills stay linked only where they were before
  (lanes with no product repo).
- `info/exclude` names an entry only when this run made (or already owns) its link and git tracks nothing at that path,
  so new files in a tracked `.claude/skills` still show in `git status`. The patterns come from the one list of linked
  entries.
- The receipt's `lane_rules` says `KEPT (repo has its own)` when the worktree kept its `.claude/CLAUDE.md`, instead of
  `linked`.
- setup links under `setup/.claude` of its worktree (the worktree is the brain's), not the root's.
- An executor's per-run `TMPDIR` is now `<cache>/hw-run-tmp/<run>` (`$XDG_CACHE_HOME`, else
  `~/.cache`), outside every git repo: under `$HW_ARTIFACTS/tmp` a "plain directory" built
  there found the worktree's repo, which made five Go tests red inside an executor and green
  in a clean env. `hw done` and `hw reap --apply` remove the directory with the worktree, and `hw reap --apply` with a loose (plain) work dir; the path removed is the one the run recorded in `.hw/<run>/env` (when it is absolute, named for the run and under `hw-run-tmp`), so a closer with another `HOME`/`XDG_CACHE_HOME` removes the right one. The
  shared Go cache stays shared under it (`gocache_default`).
- `newCSWorld` scrubs `HW_LOCK_DIR`; `TestDiffDone` waits for its detached closer by event
  (after 60 s without it the snapshot is compared to the golden as it is, and the missing close shows as a golden diff); the 844-A88/A89/R141 mutants are anchored again; `regen
  --review` treats a closed pipe as "differs"; an empty `HOME` no longer creates `core/.cache`.
- A revive whose recorded `TMPDIR` vanished recreates it (0700) before arming the sandbox; the `/tmp/hw-run-tmp` fallback root is made 0700.
- Review fixes (rest-limpio-fix2-070): revive creates only a TMPDIR `recordedRunTmpdir` accepts (a recorded `TMPDIR='scratch'` is no longer created under hw's cwd; a failed create is warned); every work-dir removal goes through `removeWithRunTmps`, which makes the scratch writable first (Go's module cache is read-only) and warns when it cannot remove it; `/tmp/hw-run-tmp` is one const and a failed chmod of it is warned.
- Four reds on `main` before 0.7.0: mutants 844-M23 and 844-R142 and 1020-G01 were stale after `--sdd` became optional
  (fcff4221, the "say which" refusal is gone) and `kind: build` + `review: none` became a refusal (bcab9b97); each is
  re-anchored on the code that now carries its claim. `setup/commit-tree` is classified (shipped) in the export manifest.
- The names-only exemption rule in `setup/tests/46-mutation-arms-are-not-gated.sh` is asserted again off macOS:
  a row with an empty reason, or `*` where an arm id belongs, exempts nothing. Both shapes were lost with the
  removed native-Windows exemption list, though the shared `exempt_arms()` kept the rule.

## [0.6.3] — 2026-10-30

### Added
- A claude executor can no longer write into a product repo it is not working in, whatever its lane's shape (a lane with
  no repo of its own included). The brain-guard plugin `hw` already passes to every claude executor now also runs
  `setup/guards/deny_executor_repo_writes.py`: every `product_repos` root of `guards.json` is refused, in Bash and in the
  Write/Edit tools, except the one that holds the executor's own `HW_WORKDIR`.
- An executor whose last turn ended legally (waiting on a background child, on a reply to its ask/challenge, or
  on a human-boundary) and that stays without a new turn for `HW_STALL_MIN` minutes (default 20; 0 turns it off)
  now reaches its brainer on its own: one `channel-send --report` to `HW_INVOKER_PANE` naming task, pane,
  minutes idle and what it waits on, and `hw status` shows the row as `STALLED since HH:MM`. The timer is
  `bin/stall-watch`, started by the turn-end hook in the shape of `setup/heavy`'s frozen-run watchdog (no
  daemon, no LaunchAgent); a new turn, the report or a gone pane ends it silently, and it alerts once per stall.
- An executor of a lane with no product repo now gets the lane's specialists: `hw` links
  `brain/<lane>/.claude/agents` and `skills` into its work directory at dispatch, so the prompt lists them
  (a lane's `settings.json` and `hooks` are never linked).

### Changed
- A `--agent pi` executor can now delegate to the lane's specialists. pi-subagents
  reads no `.claude/agents`, so `hw` names the worktree's directory and the pi
  adapter registers each file with it at session start, translating in memory
  Claude's tool names to pi's and leaving the file's `model:` to the executor's
  model; nothing is copied or rewritten. The preamble and the dispatch manifest
  say it, and `KNOWN-LIMITATIONS.md` L3b is narrowed to what still differs
  (tools pi has no counterpart for are dropped; specialists run in the foreground).
- The executor's fast gate now also runs the slow checks that only the full suite used to run: the mutation-anchor
  check when any source file changes, the Go-cache check when a subject that builds mutants changes, and the
  in-process-suite check when a `core/` file changes. A subject can declare `# gate-when-body: <regex>` to be
  selected by the text of a changed subject.
- The README leads with a one-line install and a short "Why" table, puts the
  install variants in one place, and drops the lane-settings example and the
  full command table in favour of links to the reference.
- `KNOWN-LIMITATIONS.md` opens with an index of every entry, `THREAT-MODEL.md`
  with an "In short", `REFERENCE.md` with a jump line, and `CONTRIBUTING.md`
  with the clone and the first gate run; the gate's timing knobs have their own
  section.
- The brain guard allows a listed program that carries `first=` with no
  arguments and with `help`, `-h` or `--help`, so an agent can read its usage;
  when it refuses the first argument, it names the allowed call
  (`call it as: bash <path> new|env|… (or --help)`).
- A plugin review has one focus per plugin and every piece runs once: `thermo` maintainability,
  `ponytail` senior review, `pstack` the diff's security and correctness, `blast-radius`, the
  `comment-sicko` agent and maintainability. A piece two plugins carry (`thermo` and `pstack` both
  carry the code-quality review) runs once, from the first plugin declared; the receipt, the closing
  and the `done-invoker` proof read the same list.
- `comment-sicko` is an agent: the closing tells the executor to launch it with the Agent tool, and
  `done-invoker` proves it from the transcript by its `subagent_type`, `pstack:comment-sicko`.
- A release and a push to `main` no longer wait for the Linux full suite (78-112 minutes in Docker on a
  4 GB VM, and the 0.6.3 cut died on its own 30-minute cap inside it). The release suite is the macOS fast gate
  (`HW_TEST_GATE=fast bash setup/test-hw`): the cut passes on its verdict for the candidate's exact tree
  (`<tree>.fast`) and, with none, runs the gate itself in the background and records it, and the push to `main`
  takes the same verdict. A Linux full verdict still opens both when one exists, but nothing requires it.
- The fast-gate verdict names the implementation it measured (`impl=go|bash`), as a full one does, and a verdict of
  one is never read for the other.
- Linux is best effort: it works, it is not verified per release. `setup/linux/run-suite.sh full` stays as a tool to
  run when the machine is idle; its reds become fixes in the next release.

### Fixed
- The executor sleep-poll guard no longer lets a `while/until … sleep` loop through in the background
  without a bound: wrapped in `timeout N`/`gtimeout N`, or carrying a counter or a `$SECONDS`/`date`
  deadline. The refusal names that shape. (A foreground refusal used to push an unbounded
  loop into the background, where it waited 17 h on a lock nobody held.)
- `setup/test-hw` runs each job in its own process group and, on any exit or signal, ends the whole job
  (TERM + CONT, then KILL), so a process frozen by the memory guard no longer outlives its suite.
  `setup/heavy` does the same on INT/TERM and when its command ends.
- `setup/heavy`'s one slot no longer jams behind a holder that will never finish. A zombie (a run whose
  parent is stopped: `kill -0` succeeds on it) counts as dead, for the slot holder and for queue entries.
  A holder or queue entry whose hw run has already reported (its run directory carries the `done` marker,
  not reopened, not blocked) is reclaimed and its process tree ended, with one line in `~/.cache/hw/heavy.log`
  (`reclaimed:<tag> run <dir> had reported (pid N)`). A frozen holder whose run has not reported keeps the slot.
- `hw done` no longer re-runs a verification that already ran green on the same tree. A pinned
  `setup/heavy -- <cmd>` leaves a verdict (command and tree) in the git common dir when `<cmd>` exits 0
  and leaves the working tree as it found it, and `hw done` reads it instead of waiting in the heavy
  queue for the 900 s ceiling and closing with exit 124.
- A pinned `HW_TEST_GATE=fast bash setup/test-hw --keep-going` now finds the fast-gate verdict of its tree;
  the flag used to make the pinned line differ from the cached one, so the gate ran again.
- `hw revive` finds a run recorded in the work directory (a `--worktree none` task, or one from before its
  lane had a repo): it lists the runs under the directory it already chose, instead of resolving the
  worktree a second time and refusing "no run directory".
- The release cut's fast gate runs at `nice -n 10`, no longer under `taskpolicy -b`. Background priority puts it on the
  efficiency cores, which cost about 3.5x the CPU time for the same work (subject 103: 3.2 s of CPU at `nice`, 10.8 s at
  `-b`), so seven subjects read over their time budgets with nothing slower in them. The Linux step keeps `taskpolicy -b`.

## [0.6.2] — 2026-10-30

### Added
- `setup/mutation/safe-mutate <file>... -- <command>` runs a mutation with a safety net that needs no hook:
  the dirty tree is pinned as a ref (`git stash create`, never the shared stash stack) and every file is
  restored byte for byte on exit, uncommitted work included.
- A `foreman-guide` skill for brainers: one index of «how do I X in foreman / why is Y this way»
  with the answer in a line or two and the source (file, test, `hw help` topic or ruling) to read.
  It loads only on demand, so it costs nothing per session; a question it does not hold goes to
  `brain-explorer`, and the answer is added as a new entry.
- A brief may name files kept under `<lane>/briefs/assets/` as `briefs/assets/<file>`: `hw` refuses the
  dispatch before building anything if one does not exist, and copies each to
  `$HW_ARTIFACTS/brief-assets/`, telling the executor that path.
- `docs/BRIEF-TEMPLATE.md` carries premise lines (`Premisa: <claim> | <fuente> | medida o asumida`),
  a UI-change acceptance line, a QA section (a string only the new build carries), and a rule for
  reviewing PRs in one detached checkout each.
- A guard refuses AI attribution (`Co-Authored-By` for an AI, «Generated with Claude Code», 🤖 footers) in the body of
  `gh pr create|edit` (`--body`, `-b`, `--body-file`, `-F`, a `$(cat <<EOF)` heredoc), for Claude Code and OpenCode;
  `setup/install/install-pr-attribution-guard.sh` installs it.
- Before cutting a worktree for a product repo, `hw` fetches the base over https (gh's credential helper) and prints
  the sha it will cut from, or says the base may be STALE; the worktree's own config gets the same rewrite.
- `review:` takes one list, e.g. `review: [judgment-day, thermo, ponytail, pstack]`: hw loads each
  plugin item itself, the closing names every item, and `done-invoker` wants evidence per item.
  `plugins:` stays for non-review plugins; the scalar `review:` values keep their meaning.

### Changed
- A red pre-commit gate keeps its whole output in `<git-common-dir>/hw-precommit-last.out`, repeats its
  `not ok` lines as the last lines of its diagnosis with that path, and says to run the gate with
  `--keep-going` first so every red shows in one pass.
- The pre-commit hook warns (never refuses) when a new `setup/tests/N-*` number, or a `MUTATION-ANCHOR`
  prefix, is already taken on `main` by another file.
- `setup/measure-tests.sh` ends by running `setup/test-budget-gate` on the manifest it wrote.
- The contract-citation subject names the new line of a moved symbol.
- `hw` refuses at dispatch a brief whose `## Verification` has no command it can run, instead of only warning; a line
  starting `None:` (or `Ninguno`, `N/A`) in the section keeps it legal.
- A build whose brief declares `review: judgment-day` and `plugins:` with review skills now closes
  naming both: each plugin review skill runs over the committed range before Judgment Day, and
  the manifest and receipt say so. `review: plugins` and `review: none` are unchanged.

### Fixed
- `setup/release/cut` started inside `setup/heavy` now refuses at once instead of waiting
  for the heavy slot its own parent holds until its time cap.
- The fast gate no longer goes red when a subject overruns its wall-clock budget only under load:
  it is re-run once, alone, at the end of the run, and passes with a printed warning when it fits.
- A slow engram serve is no longer read as a dead one: `done-invoker`, `brain`, the turn-end session
  renewal and `hw`'s launch-time session registration probe the serve for 10 s per try
  (`HW_ENGRAM_HEALTH_TIMEOUT`, in seconds) and try a transport failure twice, an HTTP error once. The
  refusal now reads `no engram serve answered at <url> within <N> s (2 tries)`, so a slow serve is told
  apart from a dead one.
- `done-invoker` no longer takes the report's words as proof that a plugin review ran. For a Claude
  executor it reads the executor's own transcript and its subagents', and wants a `Skill` call (or an
  `Agent` whose `subagent_type` is) of the exact `plugin:skill` the closing named: a look-alike such as
  `pstack:thermo-nuclear-code-quality-review-subagent` no longer releases
  `thermo:thermo-nuclear-code-quality-review`. A `review-skip: <skill> — <why>` line still releases one.
  With no transcript to read (another vendor, no file) it falls back to the report, by whole namespaced
  token, and the report it sends says `unverified: transcript not reachable`.
- The receipt and the closing prompt name each review skill by its plugin (`thermo:thermo-nuclear-code-quality-review`).
- `done-invoker` refuses a done report that names none of the plugin review skills the closing
  asked for, unless it carries a `review-skip: <skill> — <why>` line.
- `hw revive` gives a revived claude executor the brief's `plugins:` again (`--plugin-dir`), as it
  already did for brain-guard.

## [0.6.1] — 2026-10-27

### Changed
- A release cut of a small change no longer pays the whole Linux suite. When no verdict covers the candidate's exact
  tree but a first-parent ancestor has a Linux full verdict, the cut runs only the jobs the diff reaches, in the same
  container, and records a one-hop incremental Linux verdict. The full suite runs once per merged batch instead.
- The Linux suite and the release cut's Linux step run under `setup/heavy` at background priority (`taskpolicy -b` on
  macOS) and are never paused. `HW_HEAVY_WHEN_IDLE=1` (or `setup/heavy --when-idle`) opts in to waiting for an idle Mac
  and pausing while it is in use; the cut's 30-minute cap does not count that time.
- After `--publish` the cut waits for the public CI of the pushed commit and ends with `RELEASE PUBLISHED, CI RED: <url>`
  and a non-zero exit when it is red.
- The repository root is tidier. The reference documents moved into `docs/` (`INSTALL.md`, `REFERENCE.md`,
  `KNOWN-LIMITATIONS.md`, `THREAT-MODEL.md`, `BRIEF-TEMPLATE.md`, `RELEASE-NOTES.md`) and the helper scripts into
  `scripts/` (`init.sh`, `install-deps.sh`, `repo-settings.sh`). `README.md`, `LICENSE`, `CHANGELOG.md`,
  `CONTRIBUTING.md`, `SECURITY.md` and `install.sh` stay at the root, so the one-line install URL and the links
  GitHub shows do not change. `foreman-sh init` and `install.sh --with-recommended` run the moved scripts as before;
  an upgrade to or from a release that still has the notes at the root reads them from either place.
- Pushing `main` takes the same Linux full verdict the release cut takes: one Linux run (`setup/linux/run-suite.sh full`,
  in Docker) covers both, and a macOS full verdict still works. The refusal now names both ways in. A `gate=fast`
  verdict never opens `main`.

### Fixed
- CI's macOS fast gate is green again: the runner installs Go before the suite,
  since `hw`, `brain` and the invokers are `hw-core` from 0.6.0 on and
  `macos-latest` ships no Go.

## [0.6.0] — 2026-10-24

### Added
- `install.sh` reads an existing `~/.config/hw/impl` and says, line by line, which lines mean nothing
  since 0.6.0 (a `default=`, a bare word, or a line for `hw`, `brain`, `channel-send` or the invokers
  set to `bash`) and which `<tool>=bash` lines for the eight small tools still run that tool on its
  bash through 0.6.x. The file is left as it is.

### Changed
- The release cut runs every cheap suite (channel-send, the clean install and
  the export's fast gate) before the Linux full suite, so a red
  cheap gate stops the cut in minutes instead of after the 42-49 minute Linux run.
- The release cut stops itself when its wall clock passes `HW_CUT_MAX_MINUTES`
  (default 30, `0` turns it off): it kills the running gate, says which gate it
  was in and how long each finished gate took, exits non-zero and sends a macOS
  notification and a `channel-send --report` to `HW_INVOKER_PANE` when set.
- Under the Go default, `hw reap --help|-h`, `hw sweep -h`, `hw --help|-h` and
  `hw <lane> <task> … --help` are answered by `hw-core`, byte for byte as bash
  answered them (text and exit code), and no longer reach a bash body.
  `sweep -h` is still a dry run that ignores the flag.
- Under the Go default, `hw <lane>` with one word (the usage page on stderr, exit
  64), an argv whose first word starts with `-` (`hw -x`, `hw -x y`) and an
  executor's `hw briefs` / `hw briefs list` are answered by `hw-core`, byte for
  byte as bash answered them (text, stream and exit code), and no longer reach a
  bash body.
- `hw-core` is required. `install.sh` refuses, before writing anything to the brain,
  when no Go 1.22 or newer is on PATH (and `--check` lists Go as missing), and a build
  that fails or that makes a binary for another architecture is a refusal, not a
  warning. The Homebrew formula depends on `go`.
- `hw`, `brain`, `channel-send`, `ask-invoker` and `done-invoker` with no `hw-core`
  stop and name `setup/build-core --native`, where they used to fall back to bash.
  A `hw-core` older than the sources in `core/` is rebuilt with that command when Go
  is on PATH, and run as it is, with one line per state, when it is not.
- The picker (`hw` with no arguments on a terminal) runs in Go.
- `brain`, `channel-send`, `ask-invoker` and `done-invoker` are shims: each keeps its name and its place in `bin/` (the interface)
  and execs its Go binary (`hw-core brain`, `hw-core channel-send`, the `invokers` binary) with the rules `bin/hw` already
  follows: the binary is `HW_CORE` / `HW_INVOKERS`, else beside the tool or under `core/dist/<os>-<arch>/`; a missing one dies
  naming `setup/build-core --native`; a stale one is rebuilt when Go is on PATH. Their command lines, output, exit codes and
  on-disk effects are the ones Go already answered with; nothing a user types changes. The way back to bash is the previous
  release, `foreman-sh upgrade --to 0.5.0`.
- The eight small tools run in Go by default: the Stop hook (`bin/hw-stop-hook.sh`, still the registered
  script, which now execs `hw-core stop-hook`, so no hook configuration changes on upgrade), `hw secret`,
  `hw-sandbox`, `browser-close`, `brain-archive`, `secret-unlock`, `opencode-auto` and
  `codex-channel-session-hook.sh`. Their bash stays as the way back through 0.6.x: `HW_IMPL_<TOOL>=bash`
  (for example `HW_IMPL_STOP_HOOK=bash`) or a `<tool>=bash` line in `~/.config/hw/impl`. Only the word `bash`
  selects it; the global `HW_IMPL` and the file's default line no longer choose for these tools.
  `hw secret put` stays on bash under every mode.
- `setup/test-hw` no longer throws a full run away for one load-dependent flake. A job that
  exits non-zero does not stop the run (up to three of them): at the end each is run once
  more, alone, with nothing else of the run beside it. Green alone, the run passes, prints
  `flaky - <subject> (red in the pool, green alone)`, appends a row (date, commit, subject,
  first failing line) to `setup/suite-data/flaky-ledger.tsv` and says so in its number:
  `N tests passed  (TRACKED only — …; FLAKES ABSORBED: n, each red in the pool and green alone)`,
  which is the headline the verdict records. Red alone, the run fails as before.
  Measured 2026-10-24: five Linux full runs of ~40 min were each red on one job, a
  different one each time, all green alone.
- Only a non-zero exit is retried. A subject that does not parse, a failed mutation arm, a
  budget refusal, a second cut by the ceiling and a herdr witness hit stay red on the spot.
- More than three reds are not retried (that is a broken tree, not load); a run without
  `--keep-going` stops at the fourth red. The first red no longer stops such a run at once:
  it goes on until the retry, so a real red costs the rest of the run before it fails.
  `HW_TEST_NO_RETRY=1` restores the old behaviour, and the fast gate never retries.
- The ledger is appended to the checkout the run started from (`HW_TEST_FLAKY_LEDGER` moves
  it); a run inside the Linux container keeps its row only in the printed `flaky -` line.
- `HW_IMPL=bash`, `HW_IMPL=shadow`, `HW_IMPL_<VERB>=bash`, and a `default=bash`, bare `bash`
  or `<verb>=bash` line in `~/.config/hw/impl` no longer choose anything for `hw`, `brain`,
  `channel-send`, `ask-invoker` and `done-invoker`: each is said once per state ("bash was
  removed in 0.6.0; running Go — delete this line") and Go runs. It never stops a command,
  because a hook calls `hw` every turn. A `<tool>=bash` line for one of the eight small
  tools (Stop hook, `hw secret`, `hw-sandbox`, the five wrappers) keeps its meaning.
- `setup/release/cut` has one gate fewer: `test-bash-rollback` is gone, and the gate numbers
  after it (and their `NN-<gate>.log` files) move down by one.
- One internal seam keeps two paths of `hw-core` working until their bash is ported:
  `HW_BASH_BODY=1` makes `bin/hw`, `brain`, `channel-send` and the invokers run their bash
  body. Only `hw-core` sets it (`resumeInBash` in `core/t3_ruling.go`, for `hw ruling` of a
  task that reported `--blocked`, and `handBackToBash` in `core/t5l_lanes.go`, for a lane
  build hook that is not under `lanes/`), and the test harness. Both callers must be ported
  before the bash bodies are deleted (S4 of `setup/briefs/go-060-plan.md`). It is not a
  setting for people.
- `bin/hw` is a shim of under 100 lines: it finds `hw-core` (the same rules as before: `HW_CORE`,
  `bin/hw-core`, `core/dist/<os>-<arch>/hw-core`; missing dies naming `setup/build-core --native`; stale is
  rebuilt when Go is on PATH) and execs it. Every bash body of `hw` is gone, so `hw` no longer has
  a second implementation to fall back on; the way back to bash is the previous release,
  `foreman-sh upgrade --to 0.5.0`.
- `hw ruling` of a task that reported `--blocked` resumes it in Go (it used to re-enter `hw`'s bash for
  that one path). Same markers, same message, same transcript line, same exit codes.
- A lane build hook that is not one of the builders under `lanes/` is refused before anything is created,
  with a message naming the cause. `hw` used to source such a file into its own bash, with every variable
  and function it had; that bash is gone, so the hook cannot run. No hook in this tree is outside `lanes/`.
- The contract goldens (`setup/tests/contract-hw/golden*`) are frozen data: nothing writes them from
  bash any more. `contract-hw/regen*.sh` regenerates from `hw-core`, and `--review` prints the diff
  without writing it.
- README, INSTALL and KNOWN-LIMITATIONS say what 0.6.0 does: Go only for `hw`, `brain`, `channel-send`
  and the invokers (the way back is `foreman-sh upgrade --to 0.5.0 --brain <brain>`), and Go by default for
  the eight small tools, each with `HW_IMPL_<TOOL>=bash` or a `<tool>=bash` line as its way back until 0.7.0.
- When an install drops a `hw-core` it cannot replace, it now says `hw` stops until a core is built, not that
  `hw` runs its bash.
- KNOWN-LIMITATIONS L26 says what the bash deletion cost in mutation coverage: about ninety claims about
  `hw`, `brain`, `channel-send` and the invokers have no Go mutant yet.
- A cached suite verdict now records which implementation it measured (`impl=go|bash`, from
  `HW_IMPL`, Go by default), and a verdict for one implementation is never reused for the
  other: not by the push hook, not by the release cut, not by the incremental job cache.
  A verdict file with no `impl=` line is read as Go.

### Removed
- `install.sh --no-core`: it is refused, "removed in 0.6.0". An install that builds a
  core still removes the `default=bash` file an older `--no-core` install wrote.
- The bash bodies of `bin/brain`, `bin/channel-send`, `bin/ask-invoker` and `bin/done-invoker` (about 5,300 lines), and with them
  the internal seam `HW_BASH_BODY=1`: setting it does nothing, for any tool.
- The suite's bash pins: `pin_bash` and `MUTANT_KEEPS_IMPL` in `setup/tests/_common.sh`. Every mutation anchor the four bodies
  carried (89 rows of `setup/mutation/mutation-anchors.tsv`) is retired; the claim each held now rests on a Go anchor, a golden or a Go test.
- `hw shadow`: it says "removed in 0.6.0" and exits 64. `lib/hw/shadow.sh`, the
  `HW_SHADOW_JSONL` log and the `shadow` mode of every tool are gone.
- The implementation switch of `bin/hw`, `bin/brain`, `bin/channel-send`, `bin/ask-invoker`
  and `bin/done-invoker`, with `bin/invoker-go.sh` and `lib/hw/core-t6.sh`.
- The `test-bash-rollback` gate of the cut, and its line in the release checklist.
- `lib/hw/status.sh`, `done.sh`, `next.sh`, `reap.sh`, `briefs.sh`, `ledger.sh` and `explorer.sh`, and
  every `cmd_*` body of `bin/hw`. `lib/hw/deps.sh` and `deps.conf` stay.
- The internal seam `HW_BASH_BODY=1` no longer does anything for `hw` (hw-core stopped setting it). It
  remains for `brain`, `channel-send`, `ask-invoker` and `done-invoker` until their bash is deleted.

### Fixed
- The Go Stop hook ran the turn end in a second `hw` process unless a switch said otherwise: an unset
  switch read as a pin to bash, so the hook cost what the bash hook costs. It now always runs the turn end
  in process.
- `setup/test-hw` no longer waits forever for a job that died without writing its
  status: a job whose process is gone and that left no `<index>.status` is reported
  `not ok - runner: setup/tests/<subject> DIED WITHOUT REPORTING` (exit 1), and the run
  ends like any other red job (it continues under `HW_TEST_KEEP_GOING`). A macOS full
  suite on main had spun 73 minutes on one such job.

## [0.5.0] — 2026-10-22

The release candidate 0.5.0-rc.2, promoted with no change.

## [0.5.0-rc.2] — 2026-10-22

### Added
- A suite job runs only when what it reads changed: a content-keyed verdict cache replays a green job
  whose inputs are unchanged, and a mutant already killed is not run again until its code moves.
  `HW_TEST_NO_CACHE=1` runs everything cold.
- `setup/heavy --short` runs a short job beside the one heavy run instead of queueing behind it.

### Changed
- Each mutant corpus compiles one shared test binary that switches mutants at run time, instead of one
  compile per mutant; an anchor that cannot be switched keeps its own compile.
- A merge commit skips the fast gate and the lane sweep at pre-commit; the Python guard vectors are
  decided in one process.

### Fixed
- The in-process tests build a fresh `hw-core` when the checkout has none, so a clean checkout no longer
  reads the "hw-core is not built" warning as a contract difference.
- The mutant-binary cache trims to its cap on Linux too: it sums file sizes instead of `du`, which counts
  the directory's own block there.
- `go test` with the in-process switch and no partition marker is refused, so a bare run cannot skip
  the partitions.

## [0.5.0-rc.1] — 2026-10-20

### Added
- `setup/e2e-cheap-executors` takes `--impl go|bash` (default `go`), pins every
  switch inside its run, builds hw-core and the invokers fresh and says per tool,
  one `IMPL` line each, which implementation answered; it also runs a real `brain`
  launch and a real `hw` dispatch of a cheap executor, and `--mutant` now breaks
  the Go done-invoker too.
- `default=<mode>` is accepted in the switch file as another spelling of the bare default line.
- `hw-sandbox` (the Seatbelt profile, `arm`, the launch wrapper `run`, the outbox broker and the
  `request` client the invokers use inside the sandbox) runs in Go as a verb of `hw-core`, on the same
  switch as the other tools: `HW_IMPL_HW_SANDBOX=go`, `HW_IMPL=go`, or a `hw-sandbox=go` (or default
  `go`) line of `~/.config/hw/impl` makes it exec `hw-core hw-sandbox` with the same argv and
  environment (`bin/wrapper-go.sh`, sourced by `bin/hw-sandbox`). The default stays bash, and the bash
  stays as the rollback. Same stdout, stderr, exit status and files, held by `setup/tests/1040` (the
  real bash against its Go twin, including a stub agent run under a real Seatbelt profile and
  reporting out through a real broker on macOS) and `setup/tests/1041` (one killed mutant at least per
  behaviour). The outbox no longer needs `python3` under Go: it is pinned by device and inode and
  acted on relative to its descriptor in the Go itself.
- `setup/heavy -- <cmd>` runs a heavy command (a subject, the fast gate, `go test` of a package) in one slot (two heavy
  runs lagged the machine; `HW_HEAVY_SLOTS=2` opens a second while memory is free), first come first served, with the holders and your position said once a minute; a dead holder is
  reclaimed, the command's exit code is kept, and each run's measured duration is appended to `~/.cache/hw/heavy.log`.
  `setup/heavy --status` prints the same once.
- `hw secret put <item>[.<field>] [--replace]` stores a credential in the Bitwarden folder
  `agents` from a pipe on stdin (`<command> | hw secret put <item>`): the login password, or a
  hidden custom field with `.<field>`. The value never reaches argv, the terminal or the
  output, which is one line, `stored …` or `replaced …`. An existing value is refused and
  `--replace` overwrites it; a terminal, a file, empty stdin and an argv value are refused.
- Judges now check the work against what was actually asked: the
  `product-judge` of each product lane and Judgment Day design mode return a required `Scope beyond the ask` section,
  listing what the work adds beyond the quoted original ask, or `none`, or
  `no original ask quoted — cannot check`. It never changes the verdict.
- Feature design and explore briefs in the product lanes carry `## The original ask (verbatim)`: the requester's words and source.
- `hw status` now audits the brainer's memory beyond its index: a memory with no `type:`, a `project`
  memory with no `expires:` date or past it, a file the index does not list, a store over 60 files, a
  memory that repeats a ruling or a `CLAUDE.md` line, and engram itself (pending conflicts, observations
  saved without a `topic_key`). A clean store still prints nothing.
- The `memory-retro` skill (`_skills/memory-retro/SKILL.md`) sorts a brainer's memories into their
  layers and judges pending engram conflicts in batches with `mem_judge`; the audit names it when
  engram's conflicts pile up.
- The brief template and the executor rules ask every report to be saved with
  `topic_key: <lane>/<task>/findings`, so a re-saved report corrects itself instead of adding a row.
- Five small runtime wrappers run in Go as verbs of `hw-core`, behind the same switch as the rest:
  `browser-close`, `brain-archive`, `secret-unlock`, `opencode-auto` and
  `codex-channel-session-hook.sh`. `HW_IMPL_<TOOL>=go`, `HW_IMPL=go`, or a `<tool>=go` (or default `go`)
  line of `~/.config/hw/impl` makes the tool exec `hw-core <tool>` with the same argv, stdin and
  environment (`bin/wrapper-go.sh`, sourced by the five). The default stays bash, and the bash stays as
  the rollback. Same stdout, stderr, exit status (browser-close's 0 closed, 1 not verified, 3 Chrome
  holds the profile) and effects, held by `setup/tests/1027` (real bash against its Go twin, in
  a world of stubs) and `setup/tests/1018` (one killed mutant at least per tool). KNOWN-LIMITATIONS L16.
- `hw-reconcile` is not ported: nothing on the runtime path calls it (an operator's detector), so it
  stays bash.
- The Stop hook in Go. `bin/hw-stop-hook.sh` execs `hw-core stop-hook` when `HW_IMPL_STOP_HOOK`,
  `HW_IMPL` or the `stop-hook=` line of `~/.config/hw/impl` says `go`, and runs the executor's turn end
  in the same process instead of starting `hw`. The bash stays as the rollback and is the default.
  The Go hook is compared with the bash byte for byte on 30 payloads (`setup/tests/1030`) and its 14
  anchors are mutated and killed in process (`setup/tests/1031`). KNOWN-LIMITATIONS L16.
- `hw secret` and `secret-unlock` in Go: `bin/hw-secret` execs `hw-core hw-secret` when
  `HW_IMPL_HW_SECRET` or `HW_IMPL` says `go`, or the switch file has a `hw-secret=go`
  line. The default stays bash (the file's default line does not reach it), and the
  bash is the rollback. Same words, streams, exit status and files as the bash, and no
  `jq` needed. See KNOWN-LIMITATIONS L16.

### Changed
- The docs say what 0.5.0 does. README, INSTALL.md and KNOWN-LIMITATIONS L16 state that `hw`, `brain`,
  `channel-send`, `ask-invoker` and `done-invoker` run in Go by default (`--agent pi` dispatches included),
  that `HW_IMPL=bash`, `HW_IMPL_<TOOL>=bash` or `default=bash` in `~/.config/hw/impl` is the way back, that
  the bash is the rollback through 0.5.x and 0.6.0 removes it, and that macOS and Linux are the supported
  systems. INSTALL.md no longer says the installer writes a switch file or that bash is the default; the
  switch file is written only by `--no-core`. KNOWN-LIMITATIONS no longer says pi stays on bash or that the
  Go core skips the `secrets:` check.
- The docs also say what is not Go by default yet: the Stop hook, `hw secret`, `secret-unlock`,
  `browser-close`, `brain-archive`, `opencode-auto`, `codex-channel-session-hook.sh` and `hw-sandbox` run
  bash unless `HW_IMPL_<TOOL>=go` (or `HW_IMPL=go`) says otherwise. KNOWN-LIMITATIONS L16.
- `hw revive` of a pi executor now runs in the Go core (`HW_IMPL=go`) instead of
  being refused there: the recorded `pi --session <id>` comes back with hw's
  extensions and skills, and the pane is held until pi's startup screens are out
  of the way, byte for byte as the bash path does. The default stays bash.
- Go is the default implementation of `hw`, `brain`, `channel-send`, `ask-invoker` and `done-invoker`: with no
  `HW_IMPL` and nothing in `~/.config/hw/impl`, each of them runs `hw-core`. The way back is one line for all of
  them: `HW_IMPL=bash` in the environment, or `default=bash` in the switch file (a `<tool>=bash` line keeps one tool
  on bash). The bash implementations stay for this release as the rollback.
- A missing or stale `hw-core` still never kills a tool: the bash answers and one line says why. `hw` and the
  invokers say it once per state of the binary, since a hook calls them every turn.
- `install.sh` no longer writes the switch file when it builds `hw-core`, because Go needs none. `--no-core`
  writes `default=bash` when there is no file, and a later install that builds a core removes exactly that file.
- The release cut measures the way back as well: after the full suite under the default it runs the fast gate with
  `HW_IMPL=bash`, a stable also runs the full Linux suite, and an end-to-end log that does not say `impl=go` is refused.
- `hw` refuses a brief whose body carries the `pgrep -f 'setup/test-hw|run-suite.sh|release/cut'` machine rule, in
  bash and in Go, naming `setup/heavy` instead: that count matched the waiters' own wait loops, and four executors
  waited on each other for about 2h17.
- The README is a tour again: tagline, quickstart, features and everyday use, with
  limitations as links into `KNOWN-LIMITATIONS.md`. Every command, brief field,
  cockpit control and lane setting moved, whole, to the new `REFERENCE.md`.
- An incremental verify (`setup/verify-for-push` after a recorded full run) queues only the in-process
  `go test` partitions its diff can reach, plus the ones red in the recorded run and the ones it never
  ran, instead of all 39: they were 90-100% of the busy seconds of every incremental run measured
  (7300-9900 s of a 1277-1799 s run). The partitions it skips keep the row the recorded run gave them,
  and a run with no partition line in its job list still queues them all.
- The Go core no longer runs `bin/state-witness.sh`: `done-invoker` claims queued rulings in Go, and
  no gate refuses or degrades when that file is missing from `brain/bin` (the bash rollback still
  needs it). The Go witness answers the same verdict, detail and exit code as the bash for every
  state the bash distinguishes, held by a differential test over one set of stubbed readers.
  KNOWN-LIMITATIONS L16.

### Fixed
- The Go `hw executor-turn-end` ignores the turn end of a pi-subagents child
  session (a payload naming another session than the registered one), as the
  bash verb does: it no longer counts it or hands the parent's task back.
- Under `HW_IMPL=go`, a brief's `secrets:` are checked, shown in the dispatch manifest and named in the
  executor's prompt, as the bash `hw` does; the Go dispatch ignored them.

### Security
- The Bitwarden guard now also refuses a hand-written `bw create` and `bw edit`: `hw secret put`
  is the way in, and it names no `bw` verb in the agent's command.

## [0.4.0] — 2026-10-18

The release candidate 0.4.0-rc.5, promoted with no change.

## [0.4.0-rc.5] — 2026-10-17

### Added
- Time is a gate at commit: a subject budgeted over 10 minutes, or a suite estimated over its
  ceiling, makes the commit red and names the subject to split.
- Secrets from Bitwarden (folder `agents`). A brief names its items in `secrets: [item, …]`; `hw`
  checks the vault and each item before building anything. A value reaches a command only through
  `hw secret run <item>[.<field>] --as VAR -- <cmd>`, into that command's environment: never printed,
  never on disk or in argv. `secret-unlock` keeps the master password in the login Keychain and a
  LaunchAgent you schedule yourself (`secret-unlock --auto`) unlocks the vault on its own. A guard for agents
  refuses `bw get|unlock|export|list|serve`, the session file and `BW_SESSION`. KNOWN-LIMITATIONS L23.
- The review a build closes with is the brief's choice, in every lane and every `--sdd` mode:
  `review: judgment-day` (the default when absent, as before), `review: plugins` (the close names only
  the review skills of the brief's `plugins:`, and no Judgment Day) or `review: none` (no review note).
  Any other value refuses the dispatch, naming the three. The dry run prints a `review` line and the
  run's receipt records it (`review`, and `review_t<N>` for a `hw next`). Before this, `hw` ordered
  Judgment Day at the close of every build and ignored a brief's `review:` outside `--sdd gentle`.
- INSTALL.md has a Secrets section: the `secrets:` brief field, `hw secret run|check|status`,
  `secret-unlock`, what you set up (`bw`, the folder `agents`) and what this release does not ship.

### Changed
- An executor's fast gate never waits behind a full run: while one waits, a full run starts no new
  test, and the subjects that need the machine alone run last.
- The push trusts the last green full run plus the subjects a change can reach; it runs the whole
  suite again only when the test runner itself changed, not because a change is large.
- One gate no longer takes the whole machine: the in-process partitions run a third of the cpus at a
  time, each with its share, and a change queues only the partitions that can see it.
- The Go test cache is trimmed to its cap at the start of every run, fast gates included.
- The contract and mutant tests run in process, in Go: the same cases, the same output bytes and the
  same mutants caught as the shell suites they replace, split in parallel partitions of under 10
  minutes each. A gate is red when a partition is skipped. The retired shell subjects are listed in
  `setup/suite-data/retired-subjects.txt`.
- The docs say which implementation runs. `HW_IMPL` takes `bash`, `go` or `shadow`; without it
  and without `~/.config/hw/impl`, `hw` runs bash, and the installer writes that file with `go`
  when it builds `hw-core`, so a machine installed with Go runs the ported verbs in Go.
  `HW_IMPL=bash` goes back. README, INSTALL and KNOWN-LIMITATIONS L16 now agree.
- KNOWN-LIMITATIONS L23 says the export ships `hw secret`, `secret-unlock` and the `secrets:`
  preflight but not the guard that refuses an agent's own `bw` calls or the unlock LaunchAgent
  installer, and that the Keychain unlock is macOS only. README names `bw` as an optional tool.

### Removed
- Windows support. CI runs on Linux and macOS only, and the docs say Windows (native, Git Bash or
  WSL2) is not supported. The Git Bash compatibility layer still ships, unmaintained.
  KNOWN-LIMITATIONS L1b.
- `--sdd gentle` (gentle-ai's ODD flow), `hw gentle-home`, the `sdd_modes` lane field, the RDD review,
  the `delivery:` and `deployed_check:` gates and the gentle-ai row of `lib/hw/deps.conf`. Old
  spellings stay as aliases that warn and fall back: `--sdd gentle` and `sdd: gentle` run
  `--sdd none`, and `review: rdd` runs `judgment-day`. `install.sh` no longer installs
  `deny-gentle-real-home.py`. A `projects.json` that still carries `sdd_modes` fails to load,
  naming the field: delete the key.

### Fixed
- The Go build cache used by the tests no longer grows without bound: mutants build in a throwaway
  clone of it, and a full run caps what is left at 4096 MB.
- `hw reap --apply` archives and removes a safe product-lane worktree that holds an absolute symlink or the
  `AGENTS.md` block the toolchain injects, instead of reporting it unsafe; `hw done`'s Go path verifies
  the archive file by file like the shell one.
- KNOWN-LIMITATIONS no longer carries the `0.1.0-rc.4` title, and README no longer lists
  `sdd_modes` among the lane fields the loader checks.
- On Linux, `hw` read a file's modification time with BSD `stat -f` first, which GNU `stat` accepts as a
  different command and exits 0: a chained task's receipt could copy the previous task's report, and
  `suite-gate` misjudged the age of a lock. GNU `stat -c` is now tried first, BSD `stat -f` second.
- The Go `channel-send`, killed while it waited on herdr, could add a "could not wait … exit 137" line
  that the shell version never prints: its own kill of the child is no longer reported as a failure.
- On Linux, `hw done --keep` and the chaining lease wrote their expiry as a raw epoch (`until_h=1791379800`)
  and `hw ruling` showed `?` for when a task blocked: they asked BSD `date -r <epoch>`, which GNU `date`
  reads as a file name. GNU `date -d @<epoch>` is now tried when `-r` fails, in `hw` and in hw-core.
- On Linux, the dispatch manifest cut the `requested` citation, the design `boundary` and the model reason
  by BYTES (GNU `cut -c`), where macOS cuts by characters: a line of accented words came out shorter and
  could end in half a character. Both now keep the first N characters of the locale.
- A task name with an accented letter (`é`) got a different refusal on Linux than on macOS: its character
  class was a range (`A-Za-z`), and whether a range holds `é` is the platform's. It is now the locale's
  letters (`[:alpha:]`), the same on both; such a name is still refused, by the herdr agent-name rule.
- On Linux, `hw reap` died on a kept worktree's build output (`.next`, `.turbo`): it read the directory's age
  with BSD `stat -f` first, which GNU `stat` answers with a file-system listing and exit 0, and the comparison
  that followed failed. GNU `stat -c` is now asked first, as everywhere else in hw.
- On Linux, the dry run's route forecast for a task named with an accented letter was written under one
  name by `hw` (GNU `tr` maps bytes: `é` became `__`) and looked for under another by hw-core (`_`); the
  name now maps characters in both, as macOS's `tr` did.
- On Linux, `done-invoker` refused a `checks.md` that is not valid UTF-8 with another message than on macOS
  and than its Go port (GNU awk read on where macOS's awk stops). Such a file is now "no rows" everywhere.

## [0.4.0-rc.4] — 2026-10-15

### Added
- Every executor is handed a read-only `cheap-explorer` subagent on its vendor's cheapest tier
  (Claude Code `--agents` on haiku, OpenCode an `OPENCODE_CONFIG_CONTENT` agent on luna, codex
  `-c agents.*` on luna, pi `PI_SUBAGENT_EXTRA_AGENT_DIRS` on deepseek-v4.1-flash), and the prompt `hw`
  prepends names it and says which job goes to which tier. The dry run's `explorer` line says what
  reached the launch. Nothing is written into a product repo. KNOWN-LIMITATIONS L21.
- A `kind: build` dispatch on the top tier (opus, fable) is refused unless it says why:
  `--top-tier-why "<reason>"` or a `top_tier_why:` line in the brief. The reason is in the manifest
  and the receipt.
- A brief can load plugins into its own executor: `plugins: [thermo, ponytail, pstack]` in the
  frontmatter. Claude Code gets one `--plugin-dir` per plugin, pi one `-e <plugin dir>`, and the dry run
  says `NOT APPLIED` for opencode and codex. The copies are pinned and complete under
  `setup/vendor/plugins` (upstream sha, sha256 manifest, license, `PATCH.diff`, a `build` script that
  regenerates them): hooks, skill scripts and pi extension included, so a plugin's always-on behaviour
  works natively in the one executor whose brief asks for it. Every hook command runs through
  `hw/hook-env`, which redirects what a hook writes into `$HW_ARTIFACTS/plugin-state`; nothing is
  written to `~/.claude` or `~/.pi`. Installing them is an optional step, off by
  default: `install.sh --with-review-plugins` (also `init.sh`, and `setup/vendor/plugins/install`
  in a checkout) verifies the copies against their manifests and puts them in `<brain>/plugins`;
  a name that is not installed refuses the dispatch, naming that command. The export does not
  ship the copies (KNOWN-LIMITATIONS L22).

### Fixed
- The fast gate runs on Windows again: the hardware snapshot no longer reaches
  for `signal.SIGHUP`, which Windows does not have.
- The contract tests no longer go red only under load: a case's world reuses
  executables the machine has already run, and two timing races are closed.

## [0.4.0-rc.3] — 2026-10-14

### Added
- The suite's time budget is a gate: `setup/test-budget-gate` fails when a subject's budget passes
  600 s or the suite's estimate passes 45 minutes, and names the subject to split. A known debt carries
  a ceiling with the split that pays it; a ceiling only goes down.

### Changed
- `brain relaunch` no longer refuses a brainer that has background tasks: it answers Claude Code's exit dialog with
  «Move to background and exit» (selection read back before Enter), so the tasks keep running and the conversation
  resumes in the same pane. `--all` no longer skips such panes; a pane mid-turn is still skipped.
- A fast gate no longer waits behind a full suite: while a fast gate waits for a worker, a full run starts
  no new subject and gives the machine back; its timing subjects run alone only after the drain.
- `verify-for-push` stands on the last green verdict: a green incremental verdict is a base for the next
  push (at most 3 hops), so a push runs only the tests its change can reach.
- The four slow contract subjects (843, 847, 860, 870) and the anchors of 844 and 848 run as shards in
  parallel slots, with the same case lists as before.
- A contract case's world is one clonefile(2) of the whole tree instead of a copy per file (3 clones:
  0.67 s instead of 31.7 s).
- The golden regen scripts take `--grep`: only the cases naming that string are regenerated.
- The pre-commit hook runs the export check (about 2 s) before the gate, so an export leak fails before
  the minutes-long wait.

### Fixed
- A brainer no longer shows another lane's concurrent edits as its own Bash diff: Claude Code snapshots
  the whole git repo of the cwd around every Bash call, and every brainer shares one repo.
  `brain` and `hw` now launch `claude` with `CLAUDE_CODE_BASH_EDIT_DIFF=0`. A session already running keeps
  the diff until it is relaunched (KNOWN-LIMITATIONS L17).
- `done-invoker` no longer ends with "Exit code 137" after a delivered report. `hw done` closed the
  tab that held the invoker itself, which killed it; the close now runs after the invoker has
  exited, in a process of its own, so the invoker exits 0. A close that fails leaves `close-failed`
  and `close.log` beside the run's `done` marker.
- `done-invoker` and `ask-invoker` no longer print "token publication UNPROVEN" for any summary or
  question longer than one 80-character token: herdr deletes control characters and trims
  whitespace from every token value, and the invoker now sends values spelled that way, so the
  read-back proves them.
- The exit-status advice that `done-invoker`, `ask-invoker` and `channel-send` print when their output
  is piped no longer tells zsh users to read `${PIPESTATUS[0]}` (bash-only, empty under zsh); it
  offers `<verb> … ; echo rc=$?`, which reads the same in bash and zsh.

## [0.4.0-rc.2] — 2026-10-14

### Changed
- The pre-commit hook says once, before it runs, that a commit with no verdict for its tree runs
  the gate now and takes minutes, and that an agent should run it in the background.

### Fixed
- On a Mac without the Command Line Tools, `/usr/bin/git` and `/usr/bin/python3` are shims that
  exit 1; `--check` printed `ok git` and `ok python3` for them and `init` died on an empty
  `--brain`. A `git` or `python3` that does not run is now MISSING, with `xcode-select --install`.
- `foreman-sh init` passes on install.sh's `NOTE` line when `hw-core` is not built (no Go, Go older
  than 1.22): it finished silently, so the person could not tell `hw` was running in bash.
- `brain relaunch` no longer types `/exit` over a draft you had not sent in the brainer's input
  box: the draft was submitted as a message and the relaunch refused with "NOT proved agent-free".
  A session with no transcript is relaunched as a new conversation instead of a resume that cannot
  work.
- `setup/gate-select` does not walk checkouts nested in the tree it selects for. With agent
  worktrees under `.claude/worktrees` the selection took 243 s; it takes 1.65 s and picks the same
  83 tests on a normal tree.
- A contract subject's fan-out fits the share of the machine its slot holds, instead of 18 workers
  on 14 CPUs when five subjects ran in six slots.
- The release-copy tests copy the tracked tree, not whatever the checkout carries untracked.

## [0.4.0-rc.1] — 2026-10-12

### Added
- pi is a native executor vendor: `hw <lane> <task> --agent pi --model <provider/id>
  --sdd none` launches it with the same return channel, subagents, processed-proof
  delivery, `--sandbox` and write guard as a Claude Code executor. Its limits are in
  KNOWN-LIMITATIONS L3b: the guards are string-level, `--sandbox` is what contains,
  there is no `CLAUDE_CONFIG_DIR`, and specialists load only from `.pi/agents`.
- `hw` warns when a brief names remote access (ssh, prod, a VPS) and carries no
  `authorizes:` line, so a request to look at a server is not read as permission to
  act on it.
- pi in a `product_repo` lane: the checkout's `.claude/skills` (minus `speckit-*`) reach pi as
  one `--skill` directory, a tracked `.pi/agents` directory is passed to pi-subagents
  (`PI_SUBAGENT_EXTRA_AGENT_DIRS`) so the executor can delegate to those specialists, and
  `--sdd gentle` is refused with its measured reason instead of a generic one.
- The Go core ships. `install.sh` builds `hw-core` with `setup/build-core` when Go 1.22 or newer is on PATH
  (`--no-core` or `FOREMAN_SH_NO_CORE=1` skips it) and writes `~/.config/hw/impl` with `go` when you have none,
  so `hw`'s ported verbs run in Go. `HW_IMPL=bash`, or `HW_IMPL_<VERB>=bash` for one verb, goes back.

### Changed
- The cockpit card shows how a task is going instead of hw's internals: the task and its
  brief's title with its state and age, the model and context, the branch's commits over its
  base with the last one's age and the suite running for it, and a `⚠` line only when something
  needs you. An action `hw` would refuse is no longer drawn, and neither is its reason.
- `foreman-sh init` offers to install what Homebrew carries in the same run (asked first;
  `--yes` answers it) and checks again, instead of stopping with exit 1 and sending you to
  `--with-recommended` and back. Without the answer, the stop prints one line to copy:
  `foreman-sh --with-recommended && foreman-sh init`.
- `--sandbox` also runs a pi executor (it was Claude Code only).
- Without Go, or with one older than 1.22, `install.sh` builds nothing, says so in one line and exits 0; `hw` runs its
  bash implementation, which answers the same. `--agent pi` dispatches stay on bash and say so.
- The README no longer calls foreman-sh a shell harness: it is bash with a Go core.

### Fixed
- The cockpit's 40-row draw test measures seven real draws: each one reads and parses the state
  file again, where the six after the first reused the parsed state.
- INSTALL.md and the README say `brain` needs herdr's server running (measured on a clean
  VM: without it, `brain demo` refuses with `server_not_running`).
- `--with-recommended` and `init` install Claude Code as the `claude-code@latest` cask. The
  stable `claude-code` cask was 2.1.287 when measured on a clean macOS VM, below the 2.1.289
  floor, so `install.sh` refused the machine right after installing it (and `claude update`
  does nothing for a Homebrew install).
- `install.sh --check` over a brain that already has a lane (the `demo` lane `init` makes)
  no longer lists "a lane for one of your repositories" as a fix: its next step is
  `brain <lane>`, and the way to add another repository is a note.
- The update command `install.sh --check` and `hw preflight` name for Claude Code follows how
  it was installed (detected from the binary): `brew upgrade --cask claude-code@latest` for the
  `claude-code@latest` cask, uninstall-and-reinstall for the stable cask that trails the floor,
  `claude update` for the native installer. It was `claude update` for all of them, which does
  nothing for a Homebrew install.
- `foreman-sh init` with only `git` or `python3` missing names `xcode-select --install` and no
  longer offers Homebrew or prints a `--with-recommended` line that cannot succeed.
- A pi executor's `bash` in a product lane is judged against the main checkout and sibling
  worktrees (it skipped the lane policy before), with its own worktree left open. The check
  is string-level (KNOWN-LIMITATIONS L3b).

## [0.3.13] — 2026-10-08

### Added
- The cockpit card of a working executor says how long it has been at it and how
  long its current turn has run (`working 3h12m · turn 41m`), and marks `no
  progress 47m`, in the attention colour, when nothing has moved for 30 minutes:
  no commit on its branch, no changed file in its worktree, no new transcript
  line. A queued ruling now shows its age (`1 ruling queued 25m`). The state file
  gains `dispatched_at`, `turn_started_at` and `last_progress_at` per row and
  `rules.no_progress_after_ms`; the worktree walk is cached for 30-60 s.
- One manifest for the versions the harness needs, `lib/hw/deps.conf`: herdr, Claude Code,
  OpenCode, Node, gentle-ai, engram and jq, each with its floor and the command that prints
  its version. `install.sh --check` and `hw preflight` read it, and a tool below its floor is
  refused with the command to update it (`dep-floor` in the preflight, only for the tools
  the dispatch uses). An old tool used to show up when a dispatch failed half way. The
  versions are cached by the binary's path, mtime and size.
- `subagent_model` per lane and `hw --subagent-model <inherit|haiku|sonnet|opus|fable|claude-*>`:
  the model a Claude Code executor's subagents run on when they name none, handed
  over as `CLAUDE_CODE_SUBAGENT_MODEL` (Claude Code only; opencode and codex are
  `NOT APPLIED`). The dry run prints it as chosen or defaulted; the receipt, the
  run env and the dispatch ledger record it. An agent that pins its own `model:`
  keeps it.

### Changed
- A brief whose frontmatter has a key `hw` does not know still dispatches, with a
  warning; the warning now suggests the nearest known key (`requieres:` →
  `requires`) and its list of known keys is the parser's own, so it includes
  `review`, which it used to leave out.
- The floors that were fixed in `brain` (Claude Code, for the cockpit), `hw` (gentle-ai) and
  `install.sh` (OpenCode, Node, jq) are read from that manifest; their behaviour is the same.
  `install.sh --check` now also reports herdr, Claude Code and engram against their floors.

### Fixed
- The cockpit card's context use is what the pane's own statusline says (tokens and
  percent of the window Claude Code really has), read with `herdr agent read` and cached
  for 10 s per pane. It showed `ctx 100%` on a 1M-window model whose statusline said 41%,
  because the transcript fallback assumed a 200k window, and `ctx n/a` on working
  executors with no turn end yet. The fallbacks (turn-end value, transcript) now give the
  tokens and a percent only when the model id names its window (`[1m]`); a card that knows
  the tokens and not the window shows `ctx 237.5k`.

## [0.3.12] — 2026-10-08

### Fixed
- INSTALL.md and the README's Quickstart now start, on macOS, with Homebrew's own
  installer (which also brings the Command Line Tools): a clean machine had no `brew`
  and the docs never said so.
- The documented `hw … --dry-run` (README, INSTALL.md, the demo) carries `--no-report`
  and says why: outside a herdr pane that `brain` opened it exited 1 with
  `HW_INVOKER_PANE UNRESOLVED`. `hw` itself is unchanged.

## [0.3.11] — 2026-10-08

### Fixed
- `brain <lane> --resume <id>` on a pane left as a bare shell (after a reboot)
  starts the brainer under the lane's account; it used to look for the
  conversation in the default account and say "No conversation found".

## [0.3.10] — 2026-10-08

### Changed
- `hw status` runs in about 1 second instead of about 20 with 15 live panes, and `hw status
  <lane>` in under 1 second. The outbox scan no longer walks every task tree, and the closing
  sections run in parallel but print in the same order. The output is byte-identical, and
  `HW_STATUS_SERIAL=1` runs them one after another.

### Fixed
- `hw status` no longer crashes on a receipt line that is a JSON list.

## [0.3.9] — 2026-10-07

### Added
- `foreman-sh init` (`./install.sh init`): from a fresh install to a first dispatch in one
  command. It checks git, jq, python3, herdr, claude, rg, fd and sd (naming a missing one with
  its install command, never installing it), builds the brain and a `demo` lane over a toy
  repository with a sample brief, checks the Claude account and engram, and ends with a dry-run
  dispatch of that brief. A second run changes nothing and says so; `--yes` answers its
  questions and `--dry-run` prints the plan.

## [0.3.8] — 2026-10-07

### Added
- `hw briefs archive <lane> [--apply]` moves a lane's finished briefs (`hw ledger`
  says done, or a run reported, with no live worktree) to
  `briefs/archive/<yyyy-mm>/` in one commit, with a `MANIFEST.tsv` per month.
  Without `--apply` it prints the plan. Briefs that are in progress, never
  dispatched, `_`-prefixed, untracked or modified stay where they are.
- `hw briefs restore <lane> <task>` and `hw briefs unarchive <lane> --manifest
  <file>` undo one brief or a whole run: the same tree, byte for byte.
- A build task now leaves `checks.md` in its artifacts: one row per requirement
  of the brief's Proof and Verification, with a verdict and its evidence.
  `done-invoker` refuses a done report when the file is missing, has no rows or
  has a row with no verdict, and the report names the counts.

### Changed
- `hw <lane> <task>` for an archived task refuses, naming the archived path and
  the restore command; `--no-brief` does not override it, `--brief <path>` runs it.
- `hw ledger` lists archived briefs too (`archived: true`) and adds `done_at`.

### Fixed
- A re-task (`hw next`) records the pane's own model, effort and vendor in the
  ledger, not the defaults of a launch that did not happen.
- `install.sh` installs `deny-gentle-real-home.py`, the guard `hw` copies into
  the gentle home. Before this, a clean install that dispatched `--sdd gentle`
  failed at that copy.
- A Claude Code session that `brain` or `hw` launches no longer inherits
  `CLAUDE_CODE_CHILD_SESSION` from a terminal multiplexer started inside another
  Claude session. The "Transcript saving is off" warning is gone, and the session
  keeps its transcript, so `--resume` and `brain relaunch` have a conversation to resume.

## [0.3.7] — 2026-10-07

### Added
- `brain relaunch [--all | <lane|pane>...] [--dry-run]` relaunches every open,
  idle Claude Code brainer in place with the cockpit mod, resuming its own
  conversation (same pane, same account, writer loop and work root restored).
  A working pane is skipped and named; below the cockpit floor it relaunches
  without the mod and says so. Executors are not relaunched.
- `brain <lane> --resume <session-id>` is the same launch for one lane.
- `hw ledger` lists each brief as never-dispatched, in-progress or done, from a
  durable ledger `hw` now writes at every real dispatch (brief and its sha,
  run, account, model, effort, vendor, base, pane). A dry run writes nothing.
- A dispatch commits its brief, and only that file, when the brain checkout is
  on main and the brief is untracked or modified.

### Changed
- The cockpit pane draws one rounded card per executor, with a coloured
  state badge, a context bar and aligned buttons, and a totals header of
  chips; a stale or dead state gets a boxed, coloured banner.
- `hw done` moved out of `bin/hw` into `lib/hw/done.sh`, the third module of
  the split of `bin/hw` by command. Behaviour is unchanged.
- `hw next` moved out of `bin/hw` into `lib/hw/next.sh`, the third module of
  the split of `bin/hw` by command. Behaviour is unchanged.
- `hw <lane> <task>` with no brief now refuses, naming the path it looked for and
  the nearest names. `--no-brief` keeps the old launch, with a one-line warning.
- The task picker shows each brief's title instead of the frontmatter's `---`,
  skips `_`-prefixed files, and reads every brief in one pass.

### Fixed
- `brain relaunch` keeps each pane's own account: the folder-trust and first-run
  checks read the pane's `CLAUDE_CONFIG_DIR`, not the caller's.
- `brain relaunch` finds a brainer whose directory is spelt through a symlink or
  another case, instead of skipping it.
- A `brain <lane> --resume` whose start failed says `FAILED resuming` and exits
  75 instead of printing `resumed` and exiting 0; `brain relaunch` reports such a
  pane as `FAILED` (not `skipped`) and exits non-zero.
- A `--sdd gentle` task with `review: rdd` is told native RDD is its only
  review: its prompt no longer also asks for Judgment Day, and carries a line
  saying not to run it. Without `review:`, Judgment Day is unchanged.

## [0.3.6] — 2026-10-06

### Added
- The brainer cockpit's contract is frozen under `cockpit/`: the state file
  and `hw preflight --json` schemas, the allowlist of verbs a panel button may
  start, and one example fixture per state class. Nothing loads it yet.
- `hw cockpit-state` writes the file the brainer's cockpit panel reads: one
  JSON document per brainer with every executor's attention class (challenge,
  ask, blocked, report, working, idle), its queued rulings and which buttons are
  valid. It reads herdr's pane list and the run directories, never a pane
  capture, so it answers in well under 300 ms where `hw status` takes seconds
  per pane. A brainer started by `brain` keeps it fresh every 5 seconds and
  the invokers refresh it within a second of a report, an ask or a ruling.
- An idle executor holding an ask or a challenge gets a reply action: the
  state names the hold file (`pending_reply`) and the two reply verbs are part
  of the cockpit contract.
- A turn end publishes the executor's context use (`ctx_pct`, `ctx_tokens`)
  for claude executors, and an ask publishes whether it is an ask or a
  challenge (`ask_kind`).
- `hw preflight --json -- <hw argv>` prints what hw's own rules say about a
  dispatch, without doing it: a warning for an `--account` other than the
  lane's, and for a dispatch while a release cut holds a suite slot.
- The brainer cockpit mod (`cockpit/`): a read-only Claude Code mod that draws a
  band and a pane from the state file `hw` writes — the executors, their
  attention order, and a stale, dead or unreachable state said as such instead
  of old data. The installer mirrors it beside `bin/`. It starts no verb yet.
- The cockpit now has buttons: each starts one `hw` verb (`done` after a
  confirmation, `ruling` with its text on stdin, `receipt`, and `verify`, which
  only pre-fills the prompt). Which buttons exist is read from the state `hw`
  writes; hw's refusal is shown as it came and the card stays. A band shows
  `hw preflight`'s verdict before a dispatch runs, and never blocks it.
- `brain` opens a Claude Code brainer with the cockpit loaded (`--plugin-dir`)
  when claude is 2.1.289 or newer. Below that it prints one line and opens
  without it. `FOREMAN_COCKPIT=0` or `brain <lane> --no-cockpit` opts out.
- The cockpit panel's reply box: an idle executor holding an ask or a challenge
  gets an input that sends the typed answer to it. The text travels on stdin,
  never in a command line.
- `hw next <pane> --run <id>` gives a run that `hw revive` reopened its next task: the counter
  advances and the new task has its own `done-invoker`, with no `--retask`. A reported run that
  was not revived is refused, naming `hw revive`.

### Changed
- The refusals of `hw done`, `hw ruling` and `hw receipt` live in one module,
  `bin/hw-actions`, that the verbs and the state writer both call, so a panel
  button is disabled exactly where the verb would refuse, with the verb's own
  words. The refusal texts are unchanged.
- `cockpit/schema.json` states that `herdr.server_started_ms` is always null
  and that the `gone` attention class is reserved, in this version.

### Fixed
- A cockpit refresh that names a brainer it cannot resolve now writes nothing
  and leaves a line in `.cockpit/skipped-kicks.log`; before, it wrote a state
  file for the pane it was running in.
- `hw next --run <id>` no longer refuses a revived run that re-reported while its chaining lease is
  live: it refuses only a reported run with no reopened marker and no live lease.

## [0.3.5] — 2026-10-06

### Added
- `hw revive <project> <task> --run <id>` revives that run instead of the latest; an unknown id names the runs that exist, a run whose session is live in a pane is refused naming the pane, and the dry run says which run was chosen or defaulted.

### Changed
- `--keep-pane` is accepted under `--sdd gentle`. `hw next` re-tasks a gentle
  pane with ODD's per-task block built from the new brief (delivery, the RDD
  bullet, the deployed check) and records `deployed_check_t<N>` for it;
  `review: rdd` needs the first brief to have said so.
- The `deployed_check:` refusal names exactly the accepted forms (absent or
  empty, `required`, `out-of-scope — <why>`) with an example, and
  `BRIEF-TEMPLATE.md` says the same.
- `requires: subagents` under `--agent claude` no longer prints the
  opencode-only `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS` warning.
- A brief that declares `requires: browser`, `deployed_check: required` (or is a
  `--sdd gentle` task without an out-of-scope deployed check) no longer leaves
  the executor with the lane note "QA is staging-only" as the last word: the
  prompt adds that the brief overrides it. Every lane, launch and `hw next`.
- `review: rdd` is native. gentle-ai 4.0.0 runs its own tool-free reviewer in process on every `review capture-result --agent`, so the executor runs each returned operation unchanged: no lens subagent, no `--input`, no candidate bytes through a file. The relay plugin, `rdd-settings.json` and the SubagentStop log are gone.
- `gentle-ai-task` gives that reviewer the login it needs under a private HOME (claude on PATH, `CLAUDE_CONFIG_DIR`, `USER`, a link to the login keychain) and writes `rdd-log.jsonl` from each capture's own answer. A home provisioned before this change fails `hw gentle-home --check`: re-provision it.
- `hw status` moved out of `bin/hw` into `lib/hw/status.sh`, the second module of
  the split of `bin/hw` by command. Behaviour is unchanged.
- The native reviewer's login reaches it through one link: the login keychain file alone, inside a real Keychains directory, removed as soon as each `review capture-result` ends (success, failure or signal). The whole Keychains directory is no longer linked.

### Fixed
- `hw revive` of a `--sdd gentle` run keeps ODD: the system prompt, both
  plugins and both settings files are rebuilt from the current gentle-home
  (with the RDD half for `review: rdd`), and an incomplete home refuses,
  pointing at `hw gentle-home --check`.
- A `--sdd gentle` brief with `review: rdd` is no longer described as
  "RDD off" by the manifest or the launch info line: both say "RDD on (native)".
- A 118 KB lens-context no longer has to be relayed by hand into a subagent prompt, which an API safeguard cut twice without leaving a receipt.
- The keychain link of a task's private home survives concurrent captures: a reviewer group (four at once) shares one home, so the link is reference-counted by live capture pid under a `mkdir` lock, created on 0 to 1 and removed on 1 to 0, and a capture killed with `-9` is pruned by the next one instead of stranding the link.
- `hw gentle-home` refreshes the guard copy inside an already-complete home when `setup/guards/deny-gentle-real-home.py` changed, instead of reporting "nothing changed".

## [0.3.4] — 2026-10-06

### Added
- `--sdd gentle` is available in a second lane (`sdd_modes` in `projects.json`),
  on the same terms as in the first: Claude only, its own worktree, no `--keep-pane`.
- `review: rdd` in a `--sdd gentle` brief opts into native RDD, run against a local
  clone because gentle-ai keeps its review store inside the repository's `.git`.
  The reviewer roles ride as a plugin with their models pinned (judgment roles on
  opus, readability on sonnet), and a local SubagentStop hook appends one line per
  role run to `$HW_ARTIFACTS/rdd-log.jsonl`; telemetry stays off.

### Changed
- `hw done`'s cached close also covers `bash setup/verify-for-push` (a full
  verdict for the exact tree is a hit; a fast verdict never satisfies a full
  pin) and a tree that differs from a full-verdict tree only in lane
  documentation, recorded as `cached — green full verdict for tree <t> (<commit>);
  differs only in lane docs: <paths>`. The lane-doc rule is now one function,
  `docs_only_diff_covers` in `setup/hooks/suite-trigger-pattern.sh`, called by
  both `pre-push` and `hw`. Dirty, red and unreadable still run the command.
- `hw done` no longer re-runs a pinned verification it can already answer from
  disk: a clean task whose HEAD is the base recorded at dispatch records
  `skipped — no change since dispatch (<sha>)`, and a clean tree with a green
  `HW_TEST_GATE=fast bash setup/test-hw` (or full) verdict records
  `cached — green verdict for tree <sha> from <when>`. A dirty tree, a red or
  unreadable verdict or any other pinned command still runs as before;
  `HW_VERIFY_FRESH=1` forces the run.
- `--sdd gentle` runs gentle-ai 4.0.0 (was 3.7.0), pinned to the four checksums of
  the release's own `checksums.txt`, whose minisign signature was verified. Only the
  ODD protocol of the home's `CLAUDE.md` reaches the executor (`odd-block.md`): 4.0.0
  also installs a 45 KB orchestrator block and five hooks, and `hw gentle-home` now
  keeps the block out and removes the hooks from gentle-ai's own home.
- `hw gentle-home` proves telemetry off from the home's own state, not from the
  wrapper's environment, and gives gentle-ai a private engram port and data dir.
- `hw reap` and the removal `hw done` shares with it moved out of `bin/hw` into
  `lib/hw/reap.sh`, the first module of a split of `bin/hw` by command. Behaviour
  is unchanged. The installer now mirrors `lib/` beside `bin/`, and `lib` is a
  reserved lane name.

### Fixed
- `setup/hooks/pre-push` no longer dies (exit 127) in a worktree whose
  `suite-trigger-pattern.sh` predates `docs_only_init`: the lane-doc cover is
  then off (fail closed, the push needs its own verdict) and one stderr line
  says why. `hw`'s `_verify_docs_cover` treats such a lib as a miss.
- `hw gentle-home` copies its two guards into `<home>/guards/` and the settings run them from there. Provisioned from a task worktree, the settings named that worktree's `setup/guards/`, so the hooks dangled once it was removed.

## [0.3.3] — 2026-10-06

### Added
- An executor's prompt on the web lane now names its own QA target
  (`http://localhost:<HW_PORT_WEB>`), also after `hw next`, so QA no longer
  falls back to staging or to another worktree's server on :3001.
- `hw reap` frees `.next`/`.turbo` of kept worktrees idle past
  `retention.build_output_days`; it logs what it removes before removing it.

### Changed
- The release cut retries the e2e gate once and names both logs when it
  stays red.

### Fixed
- `hw reap` dumps a database with the `pg_dump` of the server's major
  version, names the open connections that block a drop, and counts a
  database that is already gone as dropped.
- A `HW_PORT_WEB` inherited from the caller's environment no longer leaks
  into a `--here` launch.

## [0.3.2] — 2026-10-05

### Added
- The OpenCode blocked-reason adapter reads `HW_RPC_TIMEOUT_MS` (default 2000) as how
  long it waits for one `herdr-rpc` call, so a loaded machine can be given more time
  without the adapter ever blocking a turn.
- Heavy runs take turns. The setup suite, the verification inside `hw done`,
  `hw suite` and the release cut each take one slot of a machine-wide gate
  (`bin/suite-gate`) before they start: `max(1, cpus/4)` at once, set with
  `HW_SUITE_SLOTS`. A run that waits says so once and never fails for the wait
  (`HW_SUITE_GATE_WAIT_MS`, default two hours, then it goes ahead without a slot).
  A slot held by a dead process is reclaimed, and the cut goes first without
  killing anyone. `hw status` shows the slots in use and who waits.
- `hw reap` retires the artifacts of a finished task: a work dir whose task
  reported (done, or blocked and closed), with nobody in it, becomes safe once
  every artifact is older than 30 days, and a backup (`*.tgz`, `*.tar.gz`,
  `*.tar`, or a name containing `backup`) once it is older than 60. A task that
  never reported, or has a live pane, is kept at any age. The dry run says
  «retention: 40d > 30d».
- The windows are `retention.artifact_retention_days` and
  `retention.backup_retention_days` in `projects.json` (0 disables a window).
  Every removal by retention appends a line to `.hw-reap-retention.log` under
  the brain: time, lane, path, task, age, size, reason.

### Changed
- An executor whose lane has no specialists is told so: "no specialists in this lane;
  ops-investigator / qa-tester are not available here". The line used to be omitted.
- The pre-commit test run now also runs a slow test that declares itself a golden
  (`# gate-reference:`) when a staged file is one it names. Until now such a
  golden (the dispatch output, the installer comparison) stayed stale until the
  full run after the merge; the executor's own verify already ran them.
- `setup/test-hw` lowers its parallel jobs, and says so, when the 1-minute load
  average starts above twice the cpu count (never below one; an explicit
  `HW_TEST_JOBS` is left alone).

### Fixed
- `hw status` reads a task closed by hand with `hw done` as `closed-unreported` even when it
  never ended a turn; it used to read `DIED-BEFORE-FIRST-TURN`, a death.
- A brief key written with capitals (`Kind: design`, `Boundary:`, `Deployed_Check:`) is
  now read by every reader of the frontmatter, not only the contract check. `Kind: design`
  used to give the task the sonnet default instead of opus.
- `hw next --brief` now delivers the brief's `authorizes:` block (and refuses an uncited or
  malformed one, like a first dispatch). A re-tasked executor used to work without the
  authorization its brief declared.
- `decisions` writes the worktree it is run in. Run through a PATH link to another
  checkout of the same repository, `archive` and `supersede --apply` used to
  rewrite that other checkout's files; they now use the current worktree and say
  which tree they used.
- `decisions supersede` no longer refuses a whole lane over one `Reverses` line it
  cannot read: it skips that entry with a warning that names it and moves the
  reversals that are unambiguous. `decisions check` lists the unreadable lines.
- `hw sweep` finds the runs under a lane's `.worktrees` directory. It asked `fd`,
  which skips the git-excluded `.hw/`, so a reported executor's tab there was
  never closed by `hw sweep --apply`.
- `HW_ARTIFACT_RETENTION_DAYS` and `HW_BACKUP_RETENTION_DAYS` are read as
  decimal days: `08` and `09` are eight and nine days (they used to fail as bad
  octal), and `00` disables the window like `0` (it used to be a 0-day window
  that made every finished task's artifacts safe to remove).
- A brief's `kind:` value is matched without regard to case: `Kind: Design`
  now runs opus like `kind: design`.
- `HW_RPC_TIMEOUT_MS` in the OpenCode adapter is floored and capped to what the
  runtime accepts; a fractional or huge value used to drop the publish silently.
- `setup/release/notes --refold` no longer dies with a traceback on a
  CHANGELOG whose last line is the version's heading with no final newline.

## [0.3.1] — 2026-10-05

### Added
- `hw status` prints one line per lane past its worktree count, its disk use or
  the `.next`/`.turbo` the last `hw reap` summed (`reap.max_worktrees`,
  `reap.max_disk_pct`, `reap.max_build_gb`; defaults 40, 85%, 30 GB), naming
  `hw reap <lane> --apply`, and one line for tasks that finished, never
  reported, and whose brainer pane is gone.
- A worktree `hw done` or `hw reap --apply` keeps for a git reason (dirty,
  unmerged, irreplaceable) sheds its git-ignored `.next` and `.turbo`, unless a
  live pane, process or lock is in it or it is outside the lane's worktree root.
- hw locks a worktree while its executor lives (`git worktree lock`) and
  `hw done` unlocks it; `hw reap` keeps a locked worktree.
- `hw reap` records the per-worktree databases hw provisioned, and `--apply`
  dumps and drops a recorded one whose worktree is gone.
- An executor may run `hw reap <lane>` (the survey, never `--apply`).
- A brief can carry `authorizes: <destination> :: <operation> :: <handle|none>`
  (one line per entry). `hw` validates the shape, refuses it without
  `requested_by:`, checks a named Keychain handle, shows it in the manifest and
  delivers it to the executor as a quoted block that authorizes only those
  destinations and operations.
- `hw done` now leaves a `closed-by-hand` mark in the task's state directory
  (when, which flag, whether a report existed). `hw status` shows such a task
  as `closed-unreported` instead of `finished, unreported`, so a task closed on
  purpose no longer looks like a dead one. Runs closed before the mark existed
  are reclassified when read, from the `verify_run` line only `hw done`
  writes; nothing is backfilled.
- A re-tasked chain row names the last task that did report (for example
  "task 3 reported, task 4 closed before reporting").
- `foreman-sh upgrade --brain DIR` (also `install.sh --upgrade`) installs the
  newest release with the flags the brain was installed with, runs `--check`
  and prints the release's "In short". `--to VERSION` installs that one, the way
  back; `--dry-run` says what it would do and changes nothing. The flags are
  recorded by every install in `DIR/.foreman/install.json` (never a secret): the
  install's own flags (bin dir, permissions, lane, repo, base). What a lane
  declares (vendor, model, operator, floor, request rule) is read from
  `projects.json` when `upgrade` runs, so a hand edit there is kept, never
  replayed back; a brain with no record is refused, naming the flags to pass once.
  `--to` a release older than `upgrade` is not supported: that release has no
  `upgrade` to be handed the brain.
- `hw status` says in one line when a newer release exists. It asks at most
  once a day, caches the answer in `DIR/.foreman/latest-release.json`, and says
  nothing offline or in a brain with no record.
- `hw status` shows each live executor's context size, with a prompt to prefer a
  fresh executor over `hw next` or a ruling once it passes 150k tokens, and
  the number of queued rulings with the age of the oldest. `hw receipt` shows
  the same context line for an open run.

### Changed
- A Claude executor launched without `--model` now runs sonnet, and a brief
  declaring `kind: design` runs opus; `--model` still wins. The manifest says
  where the model came from.
- The README's "Upgrading" section, INSTALL.md, the Homebrew caveats and each
  release's "Upgrading from" notes say `foreman-sh upgrade` instead of "run
  install.sh again with the same flags".
- Release notes open with a person-written "In short" summary
  (`setup/releases/highlights/VERSION.md`), which is what `upgrade` prints.
- The context read behind `hw next` and `hw status` is bounded in time, so a
  stuck pane cannot hang either command.

### Fixed
- A branch squash-merged through a GitHub PR, whose paths the base changed
  again since, read `unmerged` for good. A PR merged into the lane's base whose
  head contains the branch tip is now merge evidence; an open PR, a PR merged
  elsewhere, a failing or missing `gh`, or a non-GitHub origin is none. Such a
  branch is deleted with its worktree.
- The receipt of a re-tasked executor no longer copies the previous task's
  `report_*` tokens as if they were the current task's.

## [0.3.0] — 2026-10-05

### Added
- A brainer can now ask about an order that reads two ways: a turn ending in
  `Ambiguous: «<the operator's words>» — <reading A> / <reading B>` (or
  `Ambiguo:`) is no longer refused as a handback by `bin/hw-stop-hook.sh`.
  The line must quote words the operator actually wrote and name two
  readings, so a decision handed back as a question is still refused.

## [0.3.0-rc.2] — 2026-10-02

### Added
- `hw status` audits the always-loaded auto-memory indexes in one line: an
  index over its byte budget, a retired entry still listed, a `[[slug]]` in
  `decisions.md` that resolves nowhere. A clean store prints nothing.

### Changed
- `done-invoker --blocked` no longer closes the executor. The report is
  delivered as before, and the pane and its session stay open, waiting.
  `hw status` shows the task as `blocked-waiting`. A run launched with
  `--keep-pane` is kept by its chaining lease instead and is not marked
  waiting: its message says so, and names `hw next` and `hw done --blocked`.
- `hw ruling <pane>` on a task that reported `--blocked` resumes that task in
  the same session: the ruling is sent at once and the executor reports again
  with its own `done-invoker`, without `--retask`. A ruling to a task that
  reported done is still refused.
- `hw reap --apply` closes a blocked executor nobody answered within
  `HW_BLOCKED_WAIT_HOURS` (default 24) with `hw done --blocked`, in every
  lane's worktree root, including the one `hw sweep` leaves out, and so does
  `hw done --blocked` by hand: the turn that sent the blocked report is not
  counted as a turn after it. A turn that ends after that one still makes
  `hw done` refuse the pane. `hw sweep` keeps a waiting executor until its
  wait ends. A success report and an explicit `hw done` close as before.
- Under `--sdd gentle` the ask cap is 4 instead of 3, so approving the phase-1
  proposal no longer costs one of the three asks; other modes stay at 3.
- Claude executors now start with Claude Code's auto-memory off
  (`CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`): a brainer's saved preferences no
  longer load into every executor; the brief carries what the task needs.
- `setup/release/notes VERSION` over a version `CHANGELOG.md` already has adds
  the new fragments to that section, each entry under its type (a missing type
  inserted in Keep a Changelog order), in `CHANGELOG.md` and in
  `RELEASE-NOTES.md`, instead of refusing and leaving the merge to be done by
  hand.

### Fixed
- `hw status` showed a delivered `--blocked` report as `reported-done`.
- An OpenCode prompt that ended in two `session.idle` events counted as two
  turns after a report, and `hw done` refused the pane for the second. The
  OpenCode adapter now tells `hw` whether the pane was given a message since
  its last idle, and a repeated idle counts nothing.
- `engram-label-proxy` sends the pane's label as `expected_project` on
  `mem_update`, which engram 3.0.0 requires, and refuses one naming another label.
- The brain guard refuses `H=$(command -v hw); $H …` with a message that says to
  call `hw` by its name or by its path, not through a variable.
- The full suite runs green again on Linux (container) and on WSL2: tests and
  harness pieces that assumed macOS were fixed (the suite image builds without
  Docker Desktop's credential store and carries zsh and herdr, a heartbeat age
  is read with GNU `stat` first, the per-test ceiling cuts on native Windows).
  Native Git Bash still fails four subjects in a full run (KNOWN-LIMITATIONS,
  L1b).

## [0.3.0-rc.1] — 2026-10-02

### Added
- `hw train add <lane> <task>` merges a task's branch into `train-<lane>`
  without touching a working tree or running pre-commit. A conflict in
  `setup/suite-data/test-budgets.json` is resolved key by key against the merge base; any
  other conflict stops the merge and names the file, the tasks on the train
  that touched it, and the incoming task that should resolve it.
- `hw train push <lane>` runs `setup/verify-for-push` on the train and moves
  `main` to it. Uncommitted edits in the checkout are kept; an edit the change
  does not apply on top of stops it before anything moves. It prints the
  `git push` commands and runs none of them.
- `hw train status <lane>` lists the tasks the train carries over `main`.
  Only a lane whose checkout is the brain repository has a train.
- `hw reap --apply` archives before it removes: a merged, clean worktree whose
  only ignored content is `.artifacts`, `qa-report`, `test-results` or
  `playwright-report` is copied to `archive/<lane>/<task>/` beside the work
  directory (`HW_ARCHIVE_ROOT`), the copy is compared with `diff -r`, and only
  then are the worktree and its branch removed. Such a worktree was kept
  forever before. On a lane with `db.provisioned` its database is dumped there
  with `pg_dump -Fc`, checked with `pg_restore -l`, and dropped.
- `hw reap --apply` deletes, with `git branch -d`, the lane's merged task
  branches that no worktree holds.
- The brainer's SessionStart hook lists the lane's tasks that finished and
  never reported, and starts `hw reap <lane> --apply` in the background with a
  time cap (`HW_BG_REAP_TIMEOUT`, 600s). It writes one line with what it
  removed and kept, shown at the next session start; a reap that fails or hits
  the cap is reported `FAILED` and stops there. Executor sessions and
  compactions do not run it; `HW_HOUSEKEEPING=0` turns it off.
- `setup/test-hw` cuts a subject that overruns its ceiling: its budget in
  `setup/suite-data/test-budgets.json` times `HW_TEST_TIMEOUT_FACTOR` (default 5), never
  under `HW_TEST_TIMEOUT_FLOOR` seconds (default 60, the whole ceiling of a
  subject with no budget). Only that subject's process group is killed; the
  run names it, with its time and ceiling, as red. `HW_TEST_TIMEOUT_FACTOR=0`
  turns it off.
- `setup/tests/650` drives `hw`'s live path against a private herdr server,
  with a shell standing in for the agent: dispatch, brief delivery, turn
  tokens, a report into the brainer pane, and `hw done` closing the tab. It
  skips, saying so, when herdr is not installed.
- The README documents every field of a lane in `projects.json`, with `brief_note` and an example, and `hw help lanes` summarizes them.

### Changed
- `hw done` waits (up to `HW_DONE_TURN_WAIT` seconds, default 60, with a progress
  line) for the turn that sent the report to end, then closes, instead of
  answering "NOT CLOSED YET, run it again". A turn that ends after the report, or
  the cap, still refuses; `--force` never waits.
- `hw done` reaps its own task after closing it, the same way: it removed
  nothing before and printed the commands instead.
- `tsconfig.tsbuildinfo` and `.astro` are regenerable: they no longer keep a
  merged worktree.
- `setup/verify-for-push` runs the full suite with `--keep-going`: a red
  no longer stops the run, every red is listed at the end, and the tree is
  still refused, so one run shows every problem instead of one per run.
- `setup/export/check` scans every file of the export, generated from the
  working tree with the new `setup/export/generate --worktree`, not only the
  overlay: a private term in an exported test now stops the fast gate instead
  of the cut. The export gate reads the export about three times faster.
- A branch's fast gate now runs every slow subject that declares
  `# gate-reference:` and names what the branch changed, past the touched
  budget: the reference-output tests for a change to `bin/hw`, and the
  installer's for one to `install.sh`.
- The per-test ceiling (`HW_TEST_TIMEOUT_FACTOR`) is spent in loaded seconds:
  with more load than cpus a wall second counts less, up to
  `HW_TEST_TIMEOUT_LOAD_MAX` (default 4) times less, so a healthy subject on a
  busy machine is no longer killed and a hung one still is.

## [0.2.0-rc.1] — 2026-10-01

### Added
- Skipping permission prompts is now a choice. `skip` stays the recommended
  default (Claude Code `--dangerously-skip-permissions`, OpenCode `--auto`,
  Codex keeps its own `approval_policy`); `ask` leaves the prompts on and pins
  Codex to `-a on-request`. Pick it per dispatch with `hw --permissions ask|skip`,
  per shell with `HW_PERMISSIONS`, or per machine with
  `install.sh --permissions ask|skip` (it writes `~/.config/hw/permissions`, and
  asks in a terminal). `brain` and a revived executor follow the same setting,
  and `hw`'s manifest has a `permissions` line saying which one applies and
  where it came from. A value that is not `ask` or `skip` is refused.
- A Homebrew formula: `brew install GenaroSalomone/tap/foreman-sh` installs
  foreman-sh with herdr, jq, rg, fd and sd, and links `foreman-sh`, which is
  `install.sh` with the same flags. Its hints name `foreman-sh`, and
  `--version` prints the packaged version.
- `install.sh --with-recommended` installs the missing required and recommended
  packages (herdr, jq, rg, fd, sd, Claude Code as a cask, engram, fzf). It runs
  one `brew install` per package and prints each command before running it.
  A failed install is named and the next one is still tried. With `--check` it
  prints the commands and runs none, and without Homebrew it installs nothing.
  Nothing is installed without the flag.
- When `brew` is on PATH and a required tool is missing, `--check` names
  `--with-recommended`.

### Changed
- `hw --help advanced` documents `--permissions`. The manifest's OpenCode
  `permissions` line moved into the common `permissions` line.
- The executor's prompt states `ask-invoker`'s limit (600 characters, 3 per
  task) and its rules drop shouted capitals for the reason behind them, after
  an audit against Anthropic's prompting guide for current Claude models.

### Fixed
- The OpenCode read-only guard now refuses OpenCode's own file tools (`write`,
  `edit`, `multiedit`, `patch`, `apply_patch`) when they would write inside a
  protected repository. Before, it judged `bash` only and left file tools to
  `permission.edit`, whose absolute-path denies OpenCode never matches for a
  file outside the worktree.
- `setup/check-machine` fails when an OpenCode agent grants `edit`, `write` or
  `bash` through the legacy `tools` map, which appends an allow-all after
  every `permission` deny for that agent.
- `curl … | bash` no longer ends in `curl: (23)`: `install.sh` drains the rest
  of the script it is read from, and only when stdin is not a terminal.
- `install-deps.sh` no longer stops after the first package when `brew` reads
  stdin (a cask prompt, a post-install): it reads its package list on its own
  file descriptor, so every package is installed or named.
- An invoker whose `project-spaces.sh` does not load, or whose `projects.json`
  is not valid JSON, says so on stderr instead of failing later without a cause.
- `setup/e2e-cheap-executors` retries `agent start` while a fresh herdr pane
  answers `agent_pane_busy`, instead of failing the run in its first second.

## [0.1.3] — 2026-10-01

### Added
- `install.sh` runs when piped from curl: it clones the last published `v*` tag
  into a temporary directory and continues from there with the same flags. A
  missing `git` or `curl`, or a repository with no published tag, stops it
  with exit 1. `--check` still writes nothing.
- Through a pipe, `install.sh` asks a new lane's questions on `/dev/tty`. With
  no terminal, or under `--check`, it prints one line naming the defaults it
  used and the flags that change them.
- `done-invoker` prints `reported — verifying before close: <command>` when it
  runs the brief's verification after the report is delivered.

### Changed
- `hw done --force` and the close after a `done-invoker --blocked` report skip
  the brief's verification and record `verify_run skipped` in the receipt.

### Fixed
- The Codex repository guard reads `apply_patch` calls: a patch whose file
  headers land in a protected tree is refused with exit 2, as a Bash write
  already was. Codex calls the guard for patches only once `~/.codex/hooks.json`
  has an `apply_patch` matcher for it; the installer does not set up Codex.

## [0.1.2] — 2026-09-30

### Added
- `bin/engram-label-proxy`: `hw` and `brain` start engram's MCP server through
  it, for Claude Code, OpenCode and Codex. A `mem_*` call whose `project` is
  not the lane's label is refused; a call without one gets the lane's label,
  taken from the working directory when the session has none; `mem_search`
  always runs across every project.
- `hw status` names observations saved under a label other than the lane's.

### Changed
- `setup/release/cut` runs the candidate, export, unclassified-file and leak
  checks before the test suites, so a manifest mistake stops the cut in
  seconds.

## [0.1.1] — 2026-09-30

### Fixed
- The repository guards (Python, JavaScript and Codex) unwrap nested `env`
  wrappers on a fixed work budget and refuse the command when it runs out,
  instead of taking time that grew with the cube of the nesting. A hook that
  timed out did not block.

## [0.1.0] — 2026-09-30

First stable release.

### Added
- `install.sh --check` names the answer to engram's allowlist question and
  lists the `PATH` export as an ordered step.
- One full test suite runs per machine at a time; a second one waits in a
  queue. The pre-commit hook runs only the subjects a commit touches
  (`setup/gate-select`).

### Changed
- README rewritten: requirements before the quickstart, a table of concepts,
  every command checked against `hw help all`, and "How it compares" dated
  and sourced to each project's own documentation. The "When not to use
  foreman" section is gone.
- `BRIEF-TEMPLATE.md` no longer carries a section written for one of the
  maintainer's own projects, and its closing step works without engram.
- `brain` waits for engram's server before registering a session, and the
  server starts without printing an error.
- An executor without Judgment Day reports it as "not installed (optional)",
  not as escalated.
- CI scales time budgets by 3 on a shared runner.

### Fixed
- The repository guards unwrap `env` and its options (`-u`, `-i`, `--chdir`,
  `-S`) before judging a command, and refuse a quoting trick that hid a write
  from the Python and JavaScript guards.
- Test 189 binds its stand-in without a reverse DNS lookup, which failed on
  the macOS CI runner.
- Test 54's mock no longer runs out a 2–3 second deadline under load.

## [0.1.0-rc.4] — 2026-09-30

Fourth pre-release, and the last candidate before 0.1.0. A report that did
not arrive is kept and redelivered instead of lost, a stuck or dead task shows
in `hw status`, an executor can run inside a kernel sandbox on macOS, and an
executor no longer holds the brainer's commands.

### Added
- `hw <lane> <task> --sandbox`, or `sandbox: true` in a brief: on macOS, a
  Claude Code executor runs inside a Seatbelt profile generated for that
  dispatch. It cannot write outside its work directory, `$HW_ARTIFACTS`, the
  temporary directories, its transcript and the engram store; every outbound
  unix socket but the system resolver's is cut (herdr, Docker, the
  ssh-agent); `~/.ssh`, `~/.config/gh` and `~/.docker` are unreadable.
  `done-invoker` and `ask-invoker` still work, through the run's outbox and a
  broker outside the sandbox. Opt-in; refused off macOS and for OpenCode and
  Codex; TCP stays open (`THREAT-MODEL.md`, `KNOWN-LIMITATIONS.md` L11).
- An outbox for reports. `done-invoker` keeps a copy of each report until the
  receiver admits it; `hw outbox` lists what never arrived, `hw outbox flush
  --to <pane>` redelivers it, `brain <lane>` redelivers on open, and `hw
  status` names what is waiting. A report whose delivery is uncertain is never
  resent on its own.
- `hw status` marks a working task that shows no sign of activity for
  `HW_STALE_MINUTES` (30) as `STALE`, naming what to look at, and a run that
  never reached its first turn as `DIED-BEFORE-FIRST-TURN` instead of hiding
  it.
- `hw log <lane> <task>`: the asks, challenges, rulings, answers and reports
  of one task, from a per-task `transcript.log`.
- Continuous integration: the fast gate on Linux and macOS, and on Windows
  under Git Bash without blocking, for every push to `main` and every pull
  request. `CONTRIBUTING.md`, `SECURITY.md`, issue templates (bug, feature,
  guard vector) and a pull-request template.
- README: "How it compares", every cell sourced, and "When not to use
  foreman".

### Changed
- `done-invoker` and `ask-invoker` exit 5 when delivery is uncertain (the
  send was cut off with the message in flight), with a message not to retry,
  instead of a generic 1. On herdr the receipt says the report was
  *admitted*; only the native routes say its delivery was proved.
- `hw done` refuses to close a task whose report never reached the brainer
  (`STRANDED`) and names how to redeliver it; `--force` discards it
  deliberately.
- An executor has an executor's commands only. Run from an executor, `hw`
  refuses `next`, `ruling`, `unstick`, `reap --apply`, `sweep --apply`,
  `preview`, `revive`, a dispatch and `done` on another task; the reverse
  guard lets an executor run `hw` only with its own verbs, and `channel-send`
  only toward its own brainer.
- The README and `KNOWN-LIMITATIONS.md` say exactly when engram and Judgment
  Day become mandatory (a task `hw` registered an engram session for; a brief
  that declares `boundary:`), and that the installer does not register the
  Codex guard.
- `THREAT-MODEL.md` covers injection: a product repository's own agent
  configuration, and a report's text arriving in the brainer's session.
- `hw help` lists `hw outbox`, `--sandbox` and the brief keys `sandbox:`,
  `requires-agents:` and `boundary:`.
- `install.sh --check` on Linux requires `sd`, `fd`, `rg` and Node 22.7 or
  newer, as the requirements always said, and names each one missing.
- The example `projects.json` and `guards.json`, the README and `INSTALL.md`
  put task worktrees in `~/work`, beside the brain, where the installer puts
  them.
- `install.sh`, run again over an existing brain, adds the executor's
  `bin/hw` line to `setup/suite-data/brain-guard-programs.txt` when it has none, and
  keeps every other line (`RELEASE-NOTES.md`, "Upgrading").

### Fixed
- The brainer's guard let through a `find` whose write action (`-delete`,
  `-exec`) came after a backslash-newline, read the quoted words of an `eval`
  as data rather than code, and let a quote inside a comment pair with one on
  the next line and hide the command between them. All three are refused, in
  the Python, JavaScript and Codex guards alike.
- The brainer's guard reads `env --unset`/`-u`, `-i`, `--chdir` and
  `-S`/`--split-string` as `env` does, so the command behind them is judged.
- The Codex hook exits 2, a refusal, when it crashes while deciding; it
  exited 1, which Codex treats as a non-blocking error.
- The OpenCode plugins refuse a tool call whose arguments are not an object.

## [0.1.0-rc.3] — 2026-09-30

Third pre-release. It fixes an installer layout in which no product executor
could run, and makes the brainer's guard read the shell.

### Fixed
- The installer put task worktrees under `<brain>/work`, inside the brain,
  where the reverse guard refused every command a product executor ran in its
  worktree. `work` now goes beside the brain; `install.sh`, `install.sh
  --check` and `hw` refuse a guarded lane whose work directory is inside the
  brain and print how to move it. An existing installation also has to run
  `install.sh --lane` again for each lane, so the guards protect the new
  work directory (`RELEASE-NOTES.md`, "Upgrading").
- The brainer's guard (Claude Code, OpenCode, Codex) let through a relative
  path (`rm -rf ../myapp/src`, `git -C ../myapp …`), a redirect after
  `cd <repo> &&`, a shell-local variable in a write target, globs, brace
  expansions, `find -delete`/`-exec` and `fd -x`. It now segments each command,
  tracks the directory each one runs in, expands its variables and braces,
  reads literal `sh -c` and `eval` bodies, and treats `find`/`fd` actions as
  writes. Same verdicts in all three guards.
- In the Python guards (the Claude Code hooks and the Codex hook), a payload
  that is not an object, or a shell call whose input is not an object, crashed
  the hook (a non-blocking error to the agent); it is now refused. Claude Code's reverse guard exited 127 without `python3`, which is
  not a refusal; it now refuses.
- `hw` without a terminal waited on an interactive picker; it now prints its
  usage and exits 2.

### Added
- `THREAT-MODEL.md`: what the guards stop, what they do not, and what the
  harness assumes.
- `install.sh --with-judgment-day` installs Judgment Day into Claude Code's
  configuration; idempotent, and it refuses rather than overwrites a file it
  did not write.
- `hw help <topic>` (`dispatch`, `flags`, `commands`, `exit-codes`,
  `recovery`, `advanced`, `all`), and `--version` on `hw`, `brain` and
  `install.sh`.

### Changed
- `install.sh --check` checks everything in one pass, lists the fixes in the
  order they must be done, and ends with one `Next step:`.
- `hw --help` is one page on standard output; `brain --help` works.
- Color is used only when output is a terminal, and never with `NO_COLOR`.
- The quickstart in `README.md`, `INSTALL.md` and `examples/demo/` runs as
  written.

## [0.1.0-rc.2] — 2026-09-29

Second pre-release. It closes a write-guard gap found after rc.1 and ships
Judgment Day.

### Fixed
- Write guards on macOS: a path that spells a protected repository with
  different letter case (`~/Code/MyApp` for `~/code/myapp`) reached the
  repository, because APFS folds case and the guard compared text. The Python
  guard (Claude Code, Codex) and the JavaScript guard (OpenCode) now compare a
  path's filesystem identity, device and inode, against each protected root.
- The OpenCode guard follows a symlink whose target does not exist yet, with
  the Python guard's verdict; before, a write through such a link into a
  protected repository was allowed. Measured on macOS, on Linux (Ubuntu 24.04
  under WSL2) and on Windows under Git Bash, before the case-variant change
  above; the guards as shipped are measured on macOS only (see
  `KNOWN-LIMITATIONS.md`, L1b).

### Added
- Judgment Day (`_skills/judgment-day/`, agents in `_agents/`): a blind review
  by two judges before a diff counts as finished. Not installed by default;
  activation is in `INSTALL.md`.
- A Codex executor's brief states the executor rules Codex cannot load from a
  file (secrets, credential handles, closing the browser, where artifacts go).

### Changed
- Every release candidate is run once end to end before it is published: a
  real herdr server, a real Claude Code executor and a real OpenCode executor
  asking, challenging and reporting through the return channel.

## [0.1.0-rc.1] — 2026-09-29

First public pre-release. It is a snapshot of a harness that has been used
day to day on real repositories; the history that produced it is not part of
this repository.

### Added
- `hw`: one command that gives each task its own git worktree, its own terminal
  tab and its own agent session, from a brief in Markdown. `--dry-run` prints
  the whole dispatch, each field marked chosen or defaulted.
- `brain`: opens the long-lived planning session for a lane.
- Return channel: `done-invoker`, `ask-invoker` and `channel-send`. An executor
  reports, asks or is redirected without anyone polling its pane.
- Write guards for Claude Code, OpenCode and Codex that refuse a brainer's
  writes into its lane's repository, plus a reverse guard that refuses a
  product-lane executor's writes into the brain.
- `install.sh`: builds a brain of your own, one lane per repository, links the
  commands, merges one Stop hook, and refuses rather than overwrites when
  configuration collides. Idempotent. `--check` writes nothing.
- Lanes are configuration (`projects.json`, `guards.json`, per-lane build
  script), not code paths in `bin/`.
- A hermetic test suite (`setup/test-hw`, `setup/test-channel-send`) with a fast
  gate, runtime budgets and mutation coverage.
- `examples/demo/`: a lane to try the install on a throwaway repository.
- MIT license.

### Known limitations
See [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md). The ones that decide whether
this fits you: herdr only; Linux and native Windows (Git Bash) are measured
by the suite but not yet on a live herdr and agent, and WSL2 is the proven way
on Windows; Codex is not set up by the installer and
its write tool is unguarded; OpenCode background subagents are experimental.
