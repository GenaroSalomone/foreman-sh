# foreman-sh 0.3.13

The cockpit tells you how long each executor has worked and which one stopped, and the harness checks its tool versions before a dispatch fails half way.

## In short

- **Time and progress on every working card.** `working 3h12m · turn 41m`, a `no progress 47m` mark in the attention colour after 30 minutes with no commit, file change or transcript line, and the age of each queued ruling.
- **The card's context use is the statusline's.** It used to show `ctx 100%` on a 1M-window model whose statusline said 41%, and `ctx n/a` on executors still in their first turn.
- **One manifest of tool floors, `lib/hw/deps.conf`.** `install.sh --check` and `hw preflight` refuse a tool below its floor (herdr, Claude Code, OpenCode, Node, gentle-ai, engram, jq) and print the command to update it, instead of a dispatch failing half way.
- **Subagents on a cheaper model, per lane.** `subagent_model` and `hw --subagent-model` hand a Claude Code executor's subagents their model (`CLAUDE_CODE_SUBAGENT_MODEL`); an agent that pins its own model keeps it.
- **A brief key typo gets a suggestion.** `requieres:` warns `did you mean requires`, and the known-keys list is the parser's own.

## What changed

6 changes since 0.3.12.

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

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.12

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
