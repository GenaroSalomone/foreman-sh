# foreman-sh 0.4.0-rc.1

`hw`'s ported verbs now run in Go by default, pi can run a task's executor beside Claude Code and OpenCode, and the first run on a new machine needs fewer round trips.

## In short

- **The Go core ships, and runs by default.** `install.sh` builds `hw-core` when Go 1.22 or newer is on PATH and writes `~/.config/hw/impl` (only if you have none), so the ported verbs run in Go. Without Go it says so in one line and `hw` runs its bash implementation, which answers the same. `HW_IMPL=bash`, or `HW_IMPL_<VERB>=bash` for one verb, goes back.
- **pi as a native executor vendor.** `hw <lane> <task> --agent pi --model <provider/id> --sdd none` launches pi with the same return channel, subagents, processed-proof delivery, `--sandbox` and write guard as a Claude Code executor. In a product lane it also loads the checkout's skills and tracked `.pi/agents` specialists, and its `bash` is refused when it names the main checkout or a sibling worktree.
- **A request to look is not a permission to act.** `hw` warns when a brief names remote access (ssh, prod, a VPS) and carries no `authorizes:` line.
- **`foreman-sh init` installs what Homebrew carries in the same run.** It asks first (`--yes` answers it) and checks again, instead of stopping and sending you to `--with-recommended` and back.
- **The update command names your install.** `install.sh --check` and `hw preflight` print the upgrade that fits how Claude Code was installed, not `claude update` for every case.
- **The cockpit card says how a task is going.** Its title and state, model and context, the branch's commits over its base, and a `⚠` line only when something needs you.

## What changed

16 changes since 0.3.13.

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

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.13

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
