# foreman-sh 0.2.0-rc.1

A release candidate. It soaks for seven days before 0.2.0; report anything
that behaves differently from 0.1.3.

**Install with Homebrew.** The `brew install` line in the README
installs foreman-sh with herdr, jq, rg, fd and sd, and links `foreman-sh`,
which is `install.sh` with the same flags. Without the tap,
`install.sh --with-recommended` installs any missing dependency with Homebrew,
printing each command first; nothing is installed without the flag.

**Skipping permission prompts is now a choice.** `skip` stays the recommended
default. `ask` keeps the prompts on: per dispatch with
`hw --permissions ask|skip`, per shell with `HW_PERMISSIONS`, or per machine
with `install.sh --permissions ask|skip`. The manifest says which one applies
and where it came from.

Also in this release:
- The OpenCode read-only guard refuses OpenCode's own file tools inside a
  protected repository, not only `bash`.
- The installer no longer fails piped from curl (`curl: (23)`), and a `brew`
  that reads stdin no longer swallows the packages after it.
- The executor's prompt states `ask-invoker`'s limit, and its rules give their
  reason instead of shouting.

## Upgrading from 0.1.3

Update your checkout and run `install.sh` again with the arguments you
installed with. To keep permission prompts on, add `--permissions ask`.

The full list is in [`CHANGELOG.md`](CHANGELOG.md).
