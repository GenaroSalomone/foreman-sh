# Contributing to foreman-sh

## Reporting

The most useful reports are the smallest ones: a failing
`install.sh --check`, a `hw --dry-run` that names the wrong thing, a guard
that refuses something harmless or lets something through. Open an issue with
the command, its full output, your platform and the versions of herdr and
your agent. The issue templates ask for exactly that.

A vulnerability is not an issue: see [`SECURITY.md`](SECURITY.md).

## Changing something

1. Open a pull request with the test that fails without your change. A
   behaviour fix ships its test; a refactor changes no expectation.
2. Run the fast gate before you send it:

   ```sh
   HW_TEST_GATE=fast bash setup/test-hw
   ```

   CI runs the same gate on Linux and macOS (and Windows under Git Bash, which
   reports but does not block). On a CI host (`CI=true` or
   `GITHUB_ACTIONS=true`) every per-subject time budget of the fast gate is
   multiplied by 3, and the runner prints that it is: a shared runner cannot
   judge wall-clock time. `HW_TEST_BUDGET_FACTOR=<n>` sets the factor
   yourself; the budgets themselves are in `setup/test-budgets.json`.
3. A change to what a guard allows or refuses comes with a vector in
   `setup/guards/` for the new case.
4. Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
   and carry no AI attribution.

## The git hooks

Do not install the git hooks in `setup/hooks/` in a clone of this repository
unless you want them: they are the maintainer's workflow (`pre-push` refuses AI
attribution in new commits and wants a cached suite verdict), though it does
not restrict which remote you push to.
