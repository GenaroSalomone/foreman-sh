# foreman-sh 0.3.5

Receipt-driven review now runs gentle-ai's own native path, gentle tasks can be revived and re-tasked, and `hw status` moves into its own module.

## In short

- **Native RDD.** Each `review capture-result` runs exactly as gentle-ai returns it, and gentle-ai starts its own tool-free reviewer. No subagent relay and no pasted context: a 143 KB review context completes, and gentle-ai's own budget refuses what is too large. Only for a capture, the reviewer gets the executor's account through a link to the login keychain file. The link is reference-counted across concurrent reviewers and removed when the last one ends. Every capture is logged to `$HW_ARTIFACTS/rdd-log.jsonl` from gentle-ai's answer.
- **gentle tasks keep their mode.** `hw revive` rebuilds the framework args from the current gentle home. `hw revive --run <id>` revives a chosen run and refuses one that is still live. `--keep-pane` works under `--sdd gentle`, and `hw next` re-injects the mode.
- **Truthful dispatch text.** The operator lines say "RDD on (native)" when it is on. The `deployed_check` refusal names the accepted forms. A brief that asks for a browser or a deployed check overrides the lane's "QA is staging-only" note. Claude dispatches no longer warn about an opencode-only flag.
- **The gentle home refreshes its guard copies** when their source changes.
- **`hw status` lives in `lib/hw/status.sh`.** Output is byte-identical; `bin/hw` is about 1,600 lines shorter.

## What changed

14 changes since 0.3.4.

### Added
- `hw revive <project> <task> --run <id>` revives that run instead of the latest; an unknown id names the runs that exist, a run whose session is live in a pane is refused naming the pane, and the dry run says which run was chosen or defaulted.

### Changed
- `--keep-pane` is accepted under `--sdd gentle`. `hw next` re-tasks a gentle
  pane with ODD's per-task block built from the new brief (delivery, the RDD
  bullet, the deployed check) and records `deployed_check_t<N>` for it;
  `review: rdd` needs the first brief to have said so.
- The `deployed_check:` refusal names exactly the accepted forms (absent or
  empty, `required`, `out-of-scope — <why>`) with an example, and
  `BRIEF-TEMPLATE.md` says the same.
- `requires: subagents` under `--agent claude` no longer prints the
  opencode-only `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS` warning.
- A brief that declares `requires: browser`, `deployed_check: required` (or is a
  `--sdd gentle` task without an out-of-scope deployed check) no longer leaves
  the executor with the lane note "QA is staging-only" as the last word: the
  prompt adds that the brief overrides it. Every lane, launch and `hw next`.
- `review: rdd` is native. gentle-ai 4.0.0 runs its own tool-free reviewer in process on every `review capture-result --agent`, so the executor runs each returned operation unchanged: no lens subagent, no `--input`, no candidate bytes through a file. The relay plugin, `rdd-settings.json` and the SubagentStop log are gone.
- `gentle-ai-task` gives that reviewer the login it needs under a private HOME (claude on PATH, `CLAUDE_CONFIG_DIR`, `USER`, a link to the login keychain) and writes `rdd-log.jsonl` from each capture's own answer. A home provisioned before this change fails `hw gentle-home --check`: re-provision it.
- `hw status` moved out of `bin/hw` into `lib/hw/status.sh`, the second module of
  the split of `bin/hw` by command. Behaviour is unchanged.
- The native reviewer's login reaches it through one link: the login keychain file alone, inside a real Keychains directory, removed as soon as each `review capture-result` ends (success, failure or signal). The whole Keychains directory is no longer linked.

### Fixed
- `hw revive` of a `--sdd gentle` run keeps ODD: the system prompt, both
  plugins and both settings files are rebuilt from the current gentle-home
  (with the RDD half for `review: rdd`), and an incomplete home refuses,
  pointing at `hw gentle-home --check`.
- A `--sdd gentle` brief with `review: rdd` is no longer described as
  "RDD off" by the manifest or the launch info line: both say "RDD on (native)".
- A 118 KB lens-context no longer has to be relayed by hand into a subagent prompt, which an API safeguard cut twice without leaving a receipt.
- The keychain link of a task's private home survives concurrent captures: a reviewer group (four at once) shares one home, so the link is reference-counted by live capture pid under a `mkdir` lock, created on 0 to 1 and removed on 1 to 0, and a capture killed with `-9` is pruned by the next one instead of stranding the link.
- `hw gentle-home` refreshes the guard copy inside an already-complete home when `setup/guards/deny-gentle-real-home.py` changed, instead of reporting "nothing changed".

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.3.4

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
