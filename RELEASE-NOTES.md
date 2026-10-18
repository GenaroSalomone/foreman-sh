# foreman-sh 0.4.0-rc.5

`foreman-sh` keeps secrets in Bitwarden and out of every transcript, and its test suite runs in process.

## In short

- **Secrets from Bitwarden.** A brief names its items; `hw secret run` hands a value to one command's environment and never prints it, and the vault unlocks itself from the Keychain.
- **A guard for agents.** Reading the vault, its session or the master password directly is refused; only `hw secret run` reaches a value.
- **In-process contract and mutant tests.** Same cases, same bytes, same mutants caught, in parallel partitions under 10 minutes each.
- **Bounded Go test cache.** Mutant builds use a throwaway copy and a full run caps the cache at 4 GB.
- **macOS and Linux only.** Windows is no longer supported or tested in CI.
- **`hw reap --apply` cleans safe product-lane worktrees** that hold an absolute symlink or the injected `AGENTS.md` block.

## What changed

24 changes since 0.4.0-rc.4.

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

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.4.0-rc.4

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
