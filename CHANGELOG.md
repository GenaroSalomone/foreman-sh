# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) with pre-release labels.

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
  `setup/test-budgets.json` is resolved key by key against the merge base; any
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
  `setup/test-budgets.json` times `HW_TEST_TIMEOUT_FACTOR` (default 5), never
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
  `bin/hw` line to `setup/brain-guard-programs.txt` when it has none, and
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
