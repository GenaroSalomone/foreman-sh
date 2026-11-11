# foreman-sh 0.9.0-rc.1

foreman-sh 0.9.0-rc.1 rounds out the herdr app as the place to run foreman: new panels for ports, quota and history, a cockpit that queues what waits on you, and a toolchain that is all Go but its shims.

## In short

- **Every lane dispatches from the app.** Lanes with dev servers and per-worktree databases launch from an app terminal too, their dev servers run as app terminals that `hw done` ends, and a dry run now names any refusal the launch would make.
- **More of the cockpit in the app.** Ports and machine load, account quota, run history, an attention queue of what waits on you, and issue views with chips, each one click away in the right column.
- **Safer by default.** `--sandbox` no longer exposes the vault session or the pinned verify file, and the kill and Bitwarden guards close the bypasses an audit found.
- **Briefs declare their own effort**, and `hw next` holds a re-task to the model and effort the pane was launched with, on every vendor.
- **A newcomer's install works from the docs alone**: the explorer agent installs, and the README names every first-run step.
- **Honest gates and clean release QA**: Go reds surface in every batch gate, the fast gate prints its first-exec cost, `setup/heavy` can wait for a quiet machine, and a release QA is dispatched with the release's flags and accepted however it was launched.

## What changed

99 changes since 0.8.0.

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

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.8.0

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
