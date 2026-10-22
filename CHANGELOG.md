# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) with pre-release labels.

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
  LaunchAgent (`setup/install-bw-unlock-agent.sh`) unlocks the vault on its own. A guard for agents
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
  `setup/retired-subjects.txt`.
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
