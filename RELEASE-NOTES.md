# foreman-sh 0.6.0

Bash leaves: `hw`, `brain`, `channel-send` and the invokers are Go only, and the eight small tools run Go by default.

## In short

- **No bash rollback for the core tools.** `hw` (pi included), `brain`, `channel-send`, `ask-invoker` and `done-invoker` are short shims that exec `hw-core`; their bash bodies are gone, and `HW_IMPL=bash` or a `default=bash` line now prints a warning and runs Go.
- **If you had `default=bash`, nothing breaks:** the line is ignored with a warning, and `install.sh` says which lines of an existing switch file mean nothing since 0.6.0.
- **The small tools flip to Go.** The stop hook, `hw-secret`, `hw-sandbox` and the wrappers run Go by default; `HW_IMPL_<TOOL>=bash` or a `<tool>=bash` line is each one's way back through 0.6.x. The Go stop hook answers in-process and is about a quarter faster than the bash one.
- **A dead test job no longer hangs the suite.** A job that dies without reporting is red, named, and the run goes on.
- **A load flake no longer throws a full run away.** Each red job is run once more, alone, at the end; green alone, it is named `flaky` and the run passes, saying how many it absorbed.
- **Some mutation coverage was retired with the bash it tested;** KNOWN-LIMITATIONS says which kind of claims lost their mutant.

## What changed

35 changes since 0.5.0.

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
  first failing line) to `setup/flaky-ledger.tsv` and says so in its number:
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
  carried (89 rows of `setup/mutation-anchors.tsv`) is retired; the claim each held now rests on a Go anchor, a golden or a Go test.
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

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.5.0

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
