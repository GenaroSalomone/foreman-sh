# foreman-sh 0.5.0-rc.2

Go by default: `hw` (pi included), `brain`, `channel-send` and the invokers run in Go, with bash kept one release as the way back.

## In short

- **Go is the default.** `hw`, `pi`, `brain`, `channel-send`, `ask-invoker` and `done-invoker` run on `hw-core`; `HW_IMPL=bash` (or `default=bash` in the switch file) is the rollback, removed in 0.6.0.
- **Still bash by default.** The stop hook, `hw-secret` and six small wrappers have a Go port you opt into with `HW_IMPL_<TOOL>=go`.
- **A credential goes into Bitwarden without anyone seeing it.** `hw secret put <item>` reads the value from a pipe, refuses a terminal, and never reads it back; the guard now refuses a hand-written `bw create|edit`.
- **A suite job runs only when what it reads changed.** A content-keyed verdict cache replays a green whose inputs are unchanged, and the incremental verify runs only the Go partitions a change reaches; `HW_TEST_NO_CACHE=1` runs cold.
- **Mutants compile once per corpus.** Each mutant corpus builds one shared test binary that switches mutants at run time, instead of one compile per mutant.
- **Judges check the work against the ask.** Design briefs quote the original ask verbatim, and the product judges list the scope beyond it.

## What changed

7 changes since 0.5.0-rc.1.

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

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.5.0-rc.1

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
