# foreman-sh 0.5.0-rc.1

Go by default: `hw` (pi included), `brain`, `channel-send` and the invokers run in Go, with bash kept one release as the way back.

## In short

- **Go is the default.** `hw`, `pi`, `brain`, `channel-send`, `ask-invoker` and `done-invoker` run on `hw-core`; `HW_IMPL=bash` (or `default=bash` in the switch file) is the rollback, removed in 0.6.0.
- **Still bash by default.** The stop hook, `hw-secret` and six small wrappers have a Go port you opt into with `HW_IMPL_<TOOL>=go`.
- **A credential goes into Bitwarden without anyone seeing it.** `hw secret put <item>` reads the value from a pipe, refuses a terminal, and never reads it back; the guard now refuses a hand-written `bw create|edit`.
- **Faster gates.** The incremental verify runs only the Go partitions a change reaches, and the Linux runner sizes its jobs to the container's memory.
- **Judges check the work against the ask.** Design briefs quote the original ask verbatim, and the product judges list the scope beyond it.

## What changed

28 changes since 0.4.0.

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

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.4.0

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
