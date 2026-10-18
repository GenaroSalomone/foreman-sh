# foreman-sh 0.4.0

The first stable 0.4: a Go core behind `HW_IMPL`, a review the brief chooses, secrets from Bitwarden, and a suite green on macOS and Linux.

## In short

- **A Go core.** `hw-core` runs the ported verbs in Go; `HW_IMPL=bash|go|shadow` picks the implementation, and the installer turns Go on when it builds it.
- **The brief picks its review.** `review: judgment-day|plugins|none`; gentle-ai's flow and the RDD review are gone, and old spellings warn and fall back.
- **Secrets from Bitwarden.** A brief names its items; `hw secret run` hands a value to one command's environment and never prints it.
- **macOS and Linux, measured.** The whole suite runs green on both, and a dozen GNU-versus-BSD differences in `hw` were fixed on the way; Windows is not supported.
- **In-process tests.** Contract and mutant corpora run inside `go test`, in parallel partitions, with a bounded build cache.

## What changed

64 changes since 0.3.13 (0.4.0-rc.1, rc.2, rc.3, rc.4 and rc.5 folded in).

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
- The suite's time budget is a gate: `setup/test-budget-gate` fails when a subject's budget passes
  600 s or the suite's estimate passes 45 minutes, and names the subject to split. A known debt carries
  a ceiling with the split that pays it; a ceiling only goes down.
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
- The pre-commit hook says once, before it runs, that a commit with no verdict for its tree runs
  the gate now and takes minutes, and that an agent should run it in the background.
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
- The fast gate runs on Windows again: the hardware snapshot no longer reaches
  for `signal.SIGHUP`, which Windows does not have.
- The contract tests no longer go red only under load: a case's world reuses
  executables the machine has already run, and two timing races are closed.
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

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.4.0-rc.5

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
