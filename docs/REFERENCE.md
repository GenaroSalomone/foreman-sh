# foreman reference

Every command, brief field, framework, cockpit control and lane setting, in one
place. The [README](../README.md) is the tour; this is the lookup.

Jump to: [Commands](#commands) · [Frameworks](#frameworks---sdd) · [Model tiers](#delegating-by-model-tier) · [Cockpit](#the-cockpit) · [Layout](#layout) · [Lane settings](#customizing-a-lane-projectsjson) · [What 1.0 promises](#what-10-promises).

## Commands

| Command | What it does |
|---|---|
| `brain <lane>` | Open the lane's brainer. |
| `hw <lane> <task> --brief <path> --sdd none` | Launch an executor. Add `--dry-run` to preview. |
| `hw` | With no arguments, an [fzf](https://github.com/junegunn/fzf) picker over every lane's briefs, plus a `+ new brief` row. Needs `fzf`. |
| `hw status` | What is running, what reported and what is left over, and one line when the brainer's memory has drifted (a stale or duplicated memory, engram's conflicts piling up) that names the `memory-retro` skill. |
| `hw ledger [<lane>] [--brief <path>\|--task <t>] [--state <s>] [--json]` | Every brief as never-dispatched, in-progress or done, from the ledger `hw` writes at each dispatch. When the brain is a git repository on `main` (the installer does not make it one), `hw` also commits the brief at dispatch if it is untracked or changed. `hw <lane> <task>` with no brief is refused, naming the path it looked for; `--no-brief` launches without one on purpose. |
| brief frontmatter `review: judgment-day\|plugins\|none` | What the task's review is, in every lane and `--sdd` mode. A `kind: build` (or no kind) owes the independent review for every value, the key absent included: when it reports done, `hw review <lane> <task>` dispatches a fresh claude opus reviewer on its head (tier by code-only lines: 0 none, 1-400 thermo + test-review, more adds ponytail-review and two pstack principles). `judgment-day` (or a list naming it) adds Judgment Day, run by the reviewer; a list's plugin items, or `plugins`, add the brief's review pieces to the reviewer's tier. The one opt-out is `review: none` with a `review-skip: <why>` line (a bare `none` still owes the review and warns for one release). The builder runs no Judgment Day: it closes with its own tests and, on claude, `/simplify`. `hw done` offers the merge only for an approved review at the branch head, an opt-out, or a brainer's `--accept-review "<why>"`. A task of another kind owes no review: absent means no review there, and a declared `judgment-day` or `plugins` still orders its close (on pi and Claude `done-invoker` holds a Judgment Day close to its protocol from the transcript). Any other value refuses the dispatch; `rdd` is retired and warns into `judgment-day`. The dry run prints it in a `review` line and the run's receipt records it. |
| brief frontmatter `secrets: [<item>[.<field>], …]` | Bitwarden items the task needs, by name (folder `agents`). Before anything is built `hw` checks that the vault is unlocked and each item exists, and refuses naming what is missing. The executor reaches a value only through `hw secret run <item>[.<field>] --as VAR -- <cmd>`, which puts it in `<cmd>`'s environment and nowhere else. See *Secrets* below. |
| brief frontmatter `plugins: [<name>, …]` | Load installed review plugins (an optional install step, off by default: `install.sh --with-review-plugins`) into that executor only: Claude Code gets one `--plugin-dir` each, pi one `-e <plugin dir>` each (the plugins' hooks run natively in that executor only), and the dry run says `NOT APPLIED` for opencode and codex. Nothing is installed globally; what a hook writes goes to `$HW_ARTIFACTS/plugin-state`, never `~/.claude` or `~/.pi`, and a name that is not installed refuses the dispatch, naming the command that installs it. The export ships no plugin (KNOWN-LIMITATIONS L22). |
| `hw briefs archive <lane> [--apply]` | Move the lane's finished briefs to `briefs/archive/<yyyy-mm>/` (a plan unless `--apply`). An archived task does not launch: `hw` names the archived path and `hw briefs restore <lane> <task>`; `hw briefs unarchive <lane> --manifest <file>` undoes a whole run. |
| `hw log <lane> <task>` | What a task asked and reported. |
| `hw ruling <pane> "<correction>"` | Correct an executor that is still working, or resume one that reported `--blocked`. |
| `hw next <pane> --brief <path>` | Give the next task to an executor launched with `--keep-pane`. |
| `hw done <lane> <task>` | Run the brief's verification and close the task's tab. Waits up to 60 s for the report's own turn to end. A merged, clean task is then reaped like `hw reap --apply`. |
| `hw preflight --json -- <hw argv>` | Read-only verdict on a dispatch you have not run: `{"level":"ok\|warn\|block","rules":[…]}`. Two rules today: an `--account` other than the lane's, and a release cut holding the suite. Exits 0 whatever the level; only a malformed call exits 1. |
| `brain relaunch (--all\|<lane>…) [--dry-run]` | Reopen idle Claude Code brainers with the cockpit, keeping each one's conversation, pane and account. A working pane is skipped and named. |
| `hw reap [<lane>]` | List merged worktrees and task branches that are safe to remove. `--apply` removes them, archiving `.artifacts`/`qa-report` to `archive/<lane>/<task>/` beside the work directory first. Dirty, unmerged or occupied work is never removed. Also lists executors kept after a `--blocked` report and left unanswered for 24h; `--apply` closes them. |
| `hw train add <lane> <task>` | Merge a reported task's branch into `train-<lane>`. Only a `setup/suite-data/test-budgets.json` conflict is resolved; any other is named. |
| `hw train push <lane>` | Run the full suite on the train, then move `main` to it, keeping uncommitted edits in the checkout. Pushes nothing. |

`<pane>` is the executor's herdr pane, as `hw status` lists it. `hw help all`
lists every command and flag; `hw help <topic>` shows one part.

## Frameworks (`--sdd`)

`none` is the default and `--sdd` is optional: a dispatch without it runs outside any
framework, even where Spec Kit is installed. To use another methodology your
repository already has, name it in the brief.

| Mode | Use it for | Needs on the machine |
|---|---|---|
| `none` (default) | Any task; the brief is the whole contract. Who designs and who builds is the brief's `kind:`, not a mode. | Nothing. |
| `speckit` (opt-in) | A task that should go through Spec Kit's phases. | A `speckit-*` skill in the repository's `.claude/skills` or in `~/.claude/skills`. Without one, `hw` refuses before building anything. Claude Code only. |

There is no other mode. `--sdd gentle` (and `sdd: gentle` in a brief) is gone:
it warns and runs `--sdd none`.
Spec Kit is a third-party project that foreman integrates with, under its
own licence. foreman is not part of it, and no endorsement is implied:

- [Spec Kit](https://github.com/github/spec-kit), MIT licence.

## Delegating by model tier

Every executor, on every vendor, is handed a read-only `cheap-explorer` subagent on its
vendor's cheapest tier (Claude Code: haiku; OpenCode and codex: `gpt-5.6-luna`; pi:
`deepseek-v4.1-flash`), and the prompt `hw` prepends names it and says which job goes where:
search and mapping to the explorer, edits and tests to a mid tier, design and the verdict to
the executor. The role lives in the brain (`_agents/cheap-explorer.md`) and reaches each
vendor through its own per-launch mechanism, so nothing is written into a product repo. The
dry run's `explorer` line says what reached the launch, or `NOT INJECTED` and why; a brief
can require it with `requires-agents: subagent:cheap-explorer`.

A `kind: build` task refuses the top tier (opus, fable) unless it says why: pass
`--top-tier-why "<reason>"`, or put a `top_tier_why:` line in the brief's frontmatter. The
prompt asks for delegation and nothing forces it, and the ceiling reaches only models `hw` can
rank (KNOWN-LIMITATIONS L21).

## The cockpit

A brainer opened by `brain` runs with a cockpit mod: a card per executor, and a
band above the prompt that totals them and names the one that needs you (an
unanswered `ask-invoker`, say). `/cockpit` opens the full pane. A card says how
the task is going, top to bottom:

```
task-05 · Add retries to the importer        ● working 3h12m · turn 41m
claude · claude-sonnet-5-5 · high · ctx 41% ▰▰▱▱▱▱
2 commits · last 37m ago · gate running 12m
⚠ no progress 47m · 1 ruling queued 25m
                                                   [ receipt ]
```

The task and its brief's title (the brief's first `# ` heading, after its
frontmatter), with its state and how long it has held it; the harness, model,
effort and context; the commits of its branch over the base it was dispatched
on, how long ago the last one landed, and the suite running for it (its
suite-gate slot, or a `setup/test-hw` running in its worktree). The `⚠` line
appears only when something needs you: no progress, a queued ruling, an open
ask or challenge, a blocked report. A working executor that has not moved for 30
minutes (no commit on its branch, no file changed in its worktree, no new line
in its transcript) says `no progress 47m` and turns yellow, so a stuck run does
not look like a busy one. The turn's start is the previous turn's end, so a turn
that began after the executor sat idle is shown longer than it is. The project
is named beside the task only when the cards span more than one. Each control runs a command
that already exists, so it does nothing the command itself would refuse:

| Control | Runs |
|---|---|
| `receipt` | `hw receipt <lane> <task>` |
| `done` | `hw done <lane> <task>`, after you confirm |
| `ruling` (a text box) | `hw ruling <pane> -`, with the text you type |
| `reply` (a text box, on an open ask) | `channel-send`, back to the executor |
| `verify` | Fills the prompt with a request to check the report against its evidence; runs no verb. |

A control `hw` would refuse is not drawn at all; its reason stays in the state file and in
`hw receipt`, never on the card. The band also shows
the verdict of `hw preflight` for the dispatch your brainer is about to run, and only
displays it. The cockpit needs Claude Code 2.1.289 or newer: below that, `brain`
says so and opens without it. Turn it off with `FOREMAN_COCKPIT=0` or
`brain <lane> --no-cockpit`.

`brain <lane> --resume <id>` reopens a brainer's own conversation with the
cockpit and exits 75 when it did not start. `brain relaunch --all` does the same for
every idle brainer pane (or name lanes), in place, under the account (`CLAUDE_CONFIG_DIR`) the
pane already had; `--dry-run` previews it. A pane whose agent is working, or
that has no session to resume, is skipped and named; one that was stopped and
did not come back is reported `FAILED` and the command exits 1. Executors are
never relaunched.

## Layout

For contributors: `bin/hw` is the dispatcher and holds the launch path. Five
modules under `lib/hw/` hold the verbs that grew past it, and `bin/hw` sources
each one: `reap.sh` (`hw reap` and the removal `hw done` shares), `status.sh`
(`hw status`), `done.sh` (`hw done` and the close-time verification), `next.sh`
(`hw next`) and `ledger.sh` (the dispatch ledger, the brief commit, the picker
rows, the no-brief refusal and `hw ledger`). They are libraries with no
shebang: run `hw`, never the module. `install.sh` copies `lib/` into the brain,
and `hw` stops with a message naming the module when one is missing. The
cockpit is `cockpit/` (the mod) plus `bin/cockpit-state` and `bin/hw-actions`,
which write the state it reads and decide which buttons are live.
The Go core is `core/` (one `main` package, standard library only), built by
`setup/build-core`; `bin/hw`, `brain`, `channel-send` and the invokers hand their
work to it, with no switch to bash (0.6.0 removed it; a `HW_IMPL=bash` is said once and
ignored). A fix to a ported verb goes to `core/`; the bash that is still in the tree is
frozen dead code, and goes when the last two callers in `hw-core` are ported.

## Customizing a lane (`projects.json`)

`projects.json` at the brain root is the one place a lane's settings live.
`hw`, `brain` and the invokers read it once per process, and a lane you add
there needs no change to `bin/`. An unknown key, or a value of a shape the
loader checks (`requested_by`, `suite_lock`, `artifacts`, `model_floor`,
`model_pins`, `ports`), makes the whole file fail to load:
nothing is read from it. Other values are not checked.
`hw help lanes` prints this table in short form.

Paths accept a leading `~` (your home). `{brain}` is the brain root, `{work}`
the work directory (`work`, else `root`/work), `{checkout}` the lane's checkout, `{lane}` its
name and `{task}` the task name.

| Field | Type | What it does | Default |
|---|---|---|---|
| `space` | string | The herdr workspace label for the lane. Required. | none: the file fails to load |
| `org` | string | The organisation the lane works for: a key of the top-level `orgs`. A `product_repo` lane's `checkout` must sit under that org's directory or the file fails to load; a rule that applies to one organisation reads `org`. Every lane of your table should name one (`docs/layout.md`). | none |
| `engram` | string | The engram project label the lane's memory is filed under. | `brain` |
| `vendor` | string | The agent `brain <lane>` and `hw` default to: `claude` or `opencode`. Not validated at load. | `claude` |
| `model` | string | The model executors get when the dispatch passes no `--model`. | empty: the vendor's own |
| `model_floor` | object | `tier` (`haiku`, `sonnet` or `opus`) is the lowest Claude tier `hw` launches without `--below-floor-why`; `accepts` lists off-ladder model ids the lane admits. | no floor |
| `model_pins` | object | Maps an alias (`haiku`, `sonnet`, `opus`, `fable`) to the full `claude-*` id `hw` passes in its place. | the alias as typed |
| `subagent_model` | string | The model a Claude Code executor's subagents run on when they name none: `inherit`, an alias (`haiku`, `sonnet`, `opus`, `fable`) or a `claude-*` id. `hw` exports it as `CLAUDE_CODE_SUBAGENT_MODEL`, to Claude Code only (the manifest says `NOT APPLIED` for the others); `--subagent-model` overrides it per dispatch. An agent that pins its own `model:` keeps it. | `inherit` |
| `pi_subagent_models` | object | A pi executor's profile: `haiku`, `sonnet` and `opus` map a Claude agent's `model:` to `{model: a pi provider/id, thinking: a pi level}`, and an entry keyed by an agent's name outranks what its file says (`default` is not a key: `subagent_model` decides it, and `model: inherit` takes no thinking). Explore agents read cheap at off/low, build and review agents run strong at medium/high; the receipt's `pi_specialists` row names what each runs. | the pi roster's tiers in `core/t6_pi.go` |
| `pi_roster` | string | Which models a pi executor runs: `go` (opencode-go, paid; the lanes' `pi_subagent_models` apply) or `free` (opencode's free models; pi for testing only, and the dry run warns so). Switching back is changing the word. | `go` |
| `account` | string | The Claude Code account the lane's sessions run under: `default`, or `personal`, the second account. | `default` |
| `artifacts` | string | Where executors may publish claude.ai artifacts: `deny`, `default` or `personal`. | the lane's `account` |
| `requested_by` | string | `required` refuses a dispatch whose brief cites no request; `warn` says so and launches. | off |
| `brief_note` | string | Text `hw` appends to the preamble of every executor prompt of this lane. See below. | none |
| `qa_target` | boolean | `true`: when `hw` allocates the executor a web port, its prompt (and `hw next`) names `http://localhost:<port>` as the QA target. | off |
| `suite_lock` | string | `none` lets `hw suite` and a task's close verification run at once; `lane` serializes them. | `lane` |
| `checkout` | path | The lane's repository checkout. | none: no checkout |
| `checkout_var` | string | An upper-case environment variable that overrides `checkout` when set. | none |
| `base` | string | The branch a task's worktree is built from, and the base a review is measured against. | none |
| `base_ref_prefix` | string | Prefix of the ref the base is read from; `""` means a local branch. | `origin/` |
| `branch` | string | The branch name template for a task. | `task/{task}` |
| `worktree_root` | path | The directory that holds the lane's worktrees. | none |
| `worktree` | path | One task's worktree. | none |
| `build` | path | The script, relative to `projects.json`, that builds a worktree (`lanes/<lane>.sh`). A lane with a worktree and no `build` refuses to launch. | none |
| `repoless` | boolean | The lane has no repository: no checkout, no worktree. | `false` |
| `product_repo` | boolean | The checkout is a product repository; framework and agent lookups read it. | `false` |
| `agents_from` | string | Where a task's `.claude/agents/` come from: `checkout` or `base`. | none |
| `brain_guard` | boolean | `false` stops `hw` from loading the brain-write guard into the lane's executors, the one that keeps a product executor from writing in the brain. | `true` |
| `opencode_config_dir` | path | An OpenCode config layer for the lane's executors; it must contain `plugin/deny-repo-writes.js` or `hw` refuses to launch. | none |
| `sweep` | boolean | `false` leaves the lane's worktree root out of `hw sweep`. | `true` |
| `reap` | object | `copies`: patterns of files that are copies of the main checkout's, so `hw reap` does not count them as unsaved work. `copies_from`: the worktree script whose own copy list extends them. `max_worktrees` / `max_disk_pct` / `max_build_gb`: past any, `hw status` prints one line naming `hw reap <lane> --apply` (defaults 40, 85 and 30 GB of `.next`/`.turbo` as the last `hw reap` summed them; `HW_HYGIENE_MAX_WORKTREES` / `_DISK_PCT` / `_BUILD_GB` override). | none |
| `deps` | object | `line`: the sentence the dry run prints about dependencies. `contention`: `true` when a task's install can break other tasks. | none |
| `db` | object | `provisioned` (one database per worktree), `line`, `no_worktree` and `no_db_inert`: what the dry run says about the database. | none |
| `devserver` | object | `start`: `always` starts a dev server on every task, `opt-in` only with `--dev-server`. `line` and `off` are the text the dry run prints. | no dev server |
| `ports` | object | Base port per name, such as `{"web": 3100}`. | none |
| `hw_aliases` | array | Other names `hw <lane>` accepts. | none |
| `brain_aliases` | array | Other names `brain <lane>` accepts. | none |
| `hint_aliases` | array | The aliases the refusal messages print beside the lane name. | none |

At the top level of the file: `root` (the agents root, the one place it is
written: brain is `root`/brain, work `root`/work, the archive `root`/archive;
`bin/move-root` changes it), `work` (required when there is no `root`),
`orgs` (each organisation's directory, where its lanes' product checkouts
live), `lanes` (required),
`operator` (the person messages tell an agent to leave a decision to),
`survey_order` (the order `hw reap` and `hw worktrees` walk the lanes),
`retention` (how long `hw reap` keeps the artifacts of a task that reported and
its backups: `artifact_retention_days`, 30, and `backup_retention_days`, 60;
0 disables a window), `metrics_direct` and `comment`.

### `brief_note`: one rule for every executor of a lane

A brief says what one task needs. A rule that holds for every task of a lane,
such as a pull request template, would otherwise be copied into each brief and
forgotten in one of them. Put it in `brief_note` instead: `hw` appends it to
the preamble of every prompt it builds for that lane, including a task sent
with `hw next`.

```json
"myapp": {
  "space": "myapp",
  "checkout": "~/code/myapp",
  "base": "main",
  "brief_note": "Open every pull request with .github/pull_request_template.md and keep its headings as they are. Do not add sections of your own."
}
```

`hw <lane> <task> --dry-run` shows the note inside the prompt. The note is
plain text, one string, with no tokens expanded.

## What 1.0 promises

From `1.0.0` the interface is stable and [SemVer](https://semver.org) counts it: a
change that breaks a working setup takes a new major version, a new flag, field or
subcommand a minor, and a fix a patch. The promise covers:

- `hw` and `brain`: their subcommands and flags, and what each one does;
- the invokers (`ask-invoker`, `done-invoker`, `channel-send`): their arguments and exit codes;
- the brief format: its front matter keys and the values `hw` accepts;
- the install: `install.sh`, the `foreman-setup` command Homebrew links, and the layout it produces;
- the herdr app: foreman's mods and layouts for it, and the `hc` backend (herdr-core's command line) it runs on.

**Deprecated, and outside the promise: Codex and OpenCode.** They keep working, and
either can be removed without a major version. Claude Code and pi are the agents the
promise is made for.

**The name.** Up to 0.9.x the project was `foreman-sh`; from `1.0.0-rc.1` it is
`foreman`: the repository, the formula (`foreman` in the tap)
and the environment variables (`FOREMAN_SH_*` became `FOREMAN_*`). The installer
command is `foreman-setup`. There is no `foreman` executable, because
homebrew-core already has a different formula with that name.
