# foreman-sh 0.3.9

A fresh install now reaches its first dispatch with one command.

## In short

- **`foreman-sh init`** (`./install.sh init`) checks the prerequisites and names the install command for any that are missing, without installing them. It builds the brain and a `demo` lane over a toy repository with a sample brief, checks the Claude account and engram, and ends with a dry-run dispatch of that brief.
- **Safe to run twice.** A second run changes nothing and says so in about 2 seconds. `--yes` answers its questions and `--dry-run` prints the plan only.
- **Fewer steps.** The manual path was 9 commands with one failure; `init --yes` is one command with none. The clean-install check now runs `init` twice from an empty HOME.

## What changed

1 change since 0.3.8.

### Added
- `foreman-sh init` (`./install.sh init`): from a fresh install to a first dispatch in one
  command. It checks git, jq, python3, herdr, claude, rg, fd and sd (naming a missing one with
  its install command, never installing it), builds the brain and a `demo` lane over a toy
  repository with a sample brief, checks the Claude account and engram, and ends with a dry-run
  dispatch of that brief. A second run changes nothing and says so; `--yes` answers its
  questions and `--dry-run` prints the plan.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.8

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
