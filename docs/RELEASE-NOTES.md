# foreman-sh 0.6.2

Reviews that are proved, guards that travel, and a slow memory server that no longer reads as a dead one.

## In short

- **Plugin reviews are proved from the transcript.** A done report naming plugin review skills is refused unless the executor's own transcript (or a subagent's) shows each one ran, matched by its exact namespaced name; without a reachable transcript the check falls back and says "unverified".
- **A slow engram is not a dead one.** The health probe waits 10 s and retries once on a transport failure, and a refusal says how long it waited.
- **Briefs carry their own assets and a runnable Verification.** Brief assets reach the executor, and a brief whose Verification has no command to run is refused before anything is built.
- **PR attribution is guarded too.** `gh pr create` and `gh pr edit` bodies carrying AI attribution are refused, with the same pattern the commit gates use.
- **A foreman guide for every brainer.** The `foreman-guide` skill answers "how do I X / why is Y" from sources, loaded only on demand.

## What changed

20 changes since 0.6.1.

### Added
- `setup/mutation/safe-mutate <file>... -- <command>` runs a mutation with a safety net that needs no hook:
  the dirty tree is pinned as a ref (`git stash create`, never the shared stash stack) and every file is
  restored byte for byte on exit, uncommitted work included.
- A `foreman-guide` skill for brainers: one index of «how do I X in foreman / why is Y this way»
  with the answer in a line or two and the source (file, test, `hw help` topic or ruling) to read.
  It loads only on demand, so it costs nothing per session; a question it does not hold goes to
  `brain-explorer`, and the answer is added as a new entry.
- A brief may name files kept under `<lane>/briefs/assets/` as `briefs/assets/<file>`: `hw` refuses the
  dispatch before building anything if one does not exist, and copies each to
  `$HW_ARTIFACTS/brief-assets/`, telling the executor that path.
- `docs/BRIEF-TEMPLATE.md` carries premise lines (`Premisa: <claim> | <fuente> | medida o asumida`),
  a UI-change acceptance line, a QA section (a string only the new build carries), and a rule for
  reviewing PRs in one detached checkout each.
- A guard refuses AI attribution (`Co-Authored-By` for an AI, «Generated with Claude Code», 🤖 footers) in the body of
  `gh pr create|edit` (`--body`, `-b`, `--body-file`, `-F`, a `$(cat <<EOF)` heredoc), for Claude Code and OpenCode;
  `setup/install/install-pr-attribution-guard.sh` installs it.
- Before cutting a worktree for a product repo, `hw` fetches the base over https (gh's credential helper) and prints
  the sha it will cut from, or says the base may be STALE; the worktree's own config gets the same rewrite.
- `review:` takes one list, e.g. `review: [judgment-day, thermo, ponytail, pstack]`: hw loads each
  plugin item itself, the closing names every item, and `done-invoker` wants evidence per item.
  `plugins:` stays for non-review plugins; the scalar `review:` values keep their meaning.

### Changed
- A red pre-commit gate keeps its whole output in `<git-common-dir>/hw-precommit-last.out`, repeats its
  `not ok` lines as the last lines of its diagnosis with that path, and says to run the gate with
  `--keep-going` first so every red shows in one pass.
- The pre-commit hook warns (never refuses) when a new `setup/tests/N-*` number, or a `MUTATION-ANCHOR`
  prefix, is already taken on `main` by another file.
- `setup/measure-tests.sh` ends by running `setup/test-budget-gate` on the manifest it wrote.
- The contract-citation subject names the new line of a moved symbol.
- `hw` refuses at dispatch a brief whose `## Verification` has no command it can run, instead of only warning; a line
  starting `None:` (or `Ninguno`, `N/A`) in the section keeps it legal.
- A build whose brief declares `review: judgment-day` and `plugins:` with review skills now closes
  naming both: each plugin review skill runs over the committed range before Judgment Day, and
  the manifest and receipt say so. `review: plugins` and `review: none` are unchanged.

### Fixed
- `setup/release/cut` started inside `setup/heavy` now refuses at once instead of waiting
  for the heavy slot its own parent holds until its time cap.
- The fast gate no longer goes red when a subject overruns its wall-clock budget only under load:
  it is re-run once, alone, at the end of the run, and passes with a printed warning when it fits.
- A slow engram serve is no longer read as a dead one: `done-invoker`, `brain`, the turn-end session
  renewal and `hw`'s launch-time session registration probe the serve for 10 s per try
  (`HW_ENGRAM_HEALTH_TIMEOUT`, in seconds) and try a transport failure twice, an HTTP error once. The
  refusal now reads `no engram serve answered at <url> within <N> s (2 tries)`, so a slow serve is told
  apart from a dead one.
- `done-invoker` no longer takes the report's words as proof that a plugin review ran. For a Claude
  executor it reads the executor's own transcript and its subagents', and wants a `Skill` call (or an
  `Agent` whose `subagent_type` is) of the exact `plugin:skill` the closing named: a look-alike such as
  `pstack:thermo-nuclear-code-quality-review-subagent` no longer releases
  `thermo:thermo-nuclear-code-quality-review`. A `review-skip: <skill> — <why>` line still releases one.
  With no transcript to read (another vendor, no file) it falls back to the report, by whole namespaced
  token, and the report it sends says `unverified: transcript not reachable`.
- The receipt and the closing prompt name each review skill by its plugin (`thermo:thermo-nuclear-code-quality-review`).
- `done-invoker` refuses a done report that names none of the plugin review skills the closing
  asked for, unless it carries a `review-skip: <skill> — <why>` line.
- `hw revive` gives a revived claude executor the brief's `plugins:` again (`--plugin-dir`), as it
  already did for brain-guard.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.6.1

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
