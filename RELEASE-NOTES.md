# foreman-sh 0.4.0-rc.4

`foreman-sh` executors pick their model by the job and can load code-review plugins, and the fast gate runs on Windows again.

## In short

- **Every executor delegates by model tier.** Each vendor gets a cheap explorer for reading and searching; a `kind: build` on the top tier is refused without a stated reason.
- **Review plugins per executor.** A brief's `plugins:` loads pinned thermo, ponytail or pstack into that executor only; an optional install, off by default.
- **Windows fast gate fixed.** The hardware snapshot no longer needs `SIGHUP`.
- **Contract tests faster and steady under load.** A case's world shares executables already run, instead of paying each one's first exec again.

## What changed

5 changes since 0.4.0-rc.3.

### Added
- Every executor is handed a read-only `cheap-explorer` subagent on its vendor's cheapest tier
  (Claude Code `--agents` on haiku, OpenCode an `OPENCODE_CONFIG_CONTENT` agent on luna, codex
  `-c agents.*` on luna, pi `PI_SUBAGENT_EXTRA_AGENT_DIRS` on deepseek-v4.1-flash), and the prompt `hw`
  prepends names it and says which job goes to which tier. The dry run's `explorer` line says what
  reached the launch. Nothing is written into a product repo. KNOWN-LIMITATIONS L21.
- A `kind: build` dispatch on the top tier (opus, fable) is refused unless it says why:
  `--top-tier-why "<reason>"` or a `top_tier_why:` line in the brief. The reason is in the manifest
  and the receipt.
- A brief can load plugins into its own executor: `plugins: [thermo, ponytail, pstack]` in the
  frontmatter. Claude Code gets one `--plugin-dir` per plugin, pi one `-e <plugin dir>`, and the dry run
  says `NOT APPLIED` for opencode and codex. The copies are pinned and complete under
  `setup/vendor/plugins` (upstream sha, sha256 manifest, license, `PATCH.diff`, a `build` script that
  regenerates them): hooks, skill scripts and pi extension included, so a plugin's always-on behaviour
  works natively in the one executor whose brief asks for it. Every hook command runs through
  `hw/hook-env`, which redirects what a hook writes into `$HW_ARTIFACTS/plugin-state`; nothing is
  written to `~/.claude` or `~/.pi`. Installing them is an optional step, off by
  default: `install.sh --with-review-plugins` (also `init.sh`, and `setup/vendor/plugins/install`
  in a checkout) verifies the copies against their manifests and puts them in `<brain>/plugins`;
  a name that is not installed refuses the dispatch, naming that command. The export does not
  ship the copies (KNOWN-LIMITATIONS L22).

### Fixed
- The fast gate runs on Windows again: the hardware snapshot no longer reaches
  for `signal.SIGHUP`, which Windows does not have.
- The contract tests no longer go red only under load: a case's world reuses
  executables the machine has already run, and two timing races are closed.

## Known limits

- Native Git Bash on Windows still fails subjects in a full run
  (KNOWN-LIMITATIONS, L1b); the targeted fixes were verified on Windows 11
  ARM64 only. WSL2 was not re-measured for this release, and mutation
  testing was not run on Windows.

## Upgrading from 0.4.0-rc.3

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
