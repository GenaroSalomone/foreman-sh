# foreman-sh 0.6.1

A faster release path, a tidier repository root, and the macOS CI fix.

## In short

- **A small release no longer pays the whole Linux suite.** The cut runs only the jobs the diff reaches over an ancestor's Linux full verdict; the full suite runs once per merged batch.
- **Heavy work is never paused.** The Linux suite and the cut run at background priority; waiting for an idle Mac is opt-in (`HW_HEAVY_WHEN_IDLE=1`).
- **One Linux run opens both the cut and the push to `main`.**
- **Reference docs live in `docs/`, helper scripts in `scripts/`.** The install URL does not change.
- **CI is green on macOS.** The workflow installs Go from `core/go.mod` before the fast gate.

## What changed

6 changes since 0.6.0.

### Changed
- A release cut of a small change no longer pays the whole Linux suite. When no verdict covers the candidate's exact
  tree but a first-parent ancestor has a Linux full verdict, the cut runs only the jobs the diff reaches, in the same
  container, and records a one-hop incremental Linux verdict. The full suite runs once per merged batch instead.
- The Linux suite and the release cut's Linux step run under `setup/heavy` at background priority (`taskpolicy -b` on
  macOS) and are never paused. `HW_HEAVY_WHEN_IDLE=1` (or `setup/heavy --when-idle`) opts in to waiting for an idle Mac
  and pausing while it is in use; the cut's 30-minute cap does not count that time.
- After `--publish` the cut waits for the public CI of the pushed commit and ends with `RELEASE PUBLISHED, CI RED: <url>`
  and a non-zero exit when it is red.
- The repository root is tidier. The reference documents moved into `docs/` (`INSTALL.md`, `REFERENCE.md`,
  `KNOWN-LIMITATIONS.md`, `THREAT-MODEL.md`, `BRIEF-TEMPLATE.md`, `RELEASE-NOTES.md`) and the helper scripts into
  `scripts/` (`init.sh`, `install-deps.sh`, `repo-settings.sh`). `README.md`, `LICENSE`, `CHANGELOG.md`,
  `CONTRIBUTING.md`, `SECURITY.md` and `install.sh` stay at the root, so the one-line install URL and the links
  GitHub shows do not change. `foreman-sh init` and `install.sh --with-recommended` run the moved scripts as before;
  an upgrade to or from a release that still has the notes at the root reads them from either place.
- Pushing `main` takes the same Linux full verdict the release cut takes: one Linux run (`setup/linux/run-suite.sh full`,
  in Docker) covers both, and a macOS full verdict still works. The refusal now names both ways in. A `gate=fast`
  verdict never opens `main`.

### Fixed
- CI's macOS fast gate is green again: the runner installs Go before the suite,
  since `hw`, `brain` and the invokers are `hw-core` from 0.6.0 on and
  `macos-latest` ships no Go.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.6.0

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
