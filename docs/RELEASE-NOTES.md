# foreman 1.0.0-rc.1

foreman 1.0.0-rc.1 is the first 1.0 candidate: foreman-sh is renamed `foreman`, the 1.0 promise is written down, and a whole person-shaped cycle runs from the herdr app's buttons.

## In short

- **foreman is its name, and 1.0 says what it promises.** The repository and the Homebrew formula are now `foreman`, a 0.9.x install upgrades in place, and the README states the 1.0 surface: `hw`, `brain` and the invokers on Claude Code and pi.
- **The Cockpit acts, not just shows.** Queue rows carry **Reply**, **Rule** and **Done** buttons, Agents tells a brainer from an executor at a glance, and Files renders markdown and images the way the viewer does.
- **The app runs a product-lane cycle end to end.** Dispatch from Issues, watch ports, answer, rule, close and read the history against a lane with a dev server, proved live in an isolated app.
- **A faster, more honest batch gate.** The mutation partitions move to the release gate, every gate run is logged in `gate-runs.tsv`, known flakes are labelled, and the gate names what it did not run.
- **Reds surface at commit time.** `setup/commit-tree` runs the touched subjects of seconds before it commits, bash subjects take `--base`, and `setup/heavy` keeps the tail of every run.

## What changed

113 changes since 0.9.0-rc.1.

### Added
- In the herdr app's Agents list, each row's icon tells a brainer from an executor: a brain for a brainer
  (a terminal named `brain-<lane>` with no `hw_task`), a hard hat for an executor (one carrying `hw_task`, and no
  `hw_role`); every other terminal keeps its program icon. It is `herdr-app/mods/foreman-agents`, a clone of the app's
  `agents` mod that the three foreman layouts now place; link it into `~/.config/herdr-app/mods/` as the README says.
- `foreman-files` copies a local image it opens into `~/.config/herdr-app/cache/foreman-files/`,
  named by its content, because the app draws images only from its own folder; copies not opened
  for 30 days are removed.
- A Files block can name the file it shows first with `props.open`.
- `setup/env-prefix-check` (run by subject 1356) fails when an environment prefix that `bin/` or `core/` reads is neither swept in `setup/tests/_common.sh` nor on `setup/suite-data/env-prefix-allowlist.tsv` with a reason. It would have caught the `HC_*` leak that lasted about 2.7 days.
- `setup/test-hw --base <ref> <subject>...` answers «is this red the branch's or the base's?» for bash subjects the way `go-test-core.sh --base` does for Go: one fresh clone at `<ref>`, each named `setup/tests` subject run there and on this tree under `env -i`, `red`/`green`/`none` for each, exit = this tree's.
- In the herdr app's Cockpit, the queue's rows have buttons: **Reply** on an ask, **Rule** on a challenge (or a queued
  ruling), **Done** on a reported task (it asks first). Each runs the argv `cockpit/verbs.txt` gives, read from that file
  (`props.verbs` in the layout's cockpit block); a failed verb shows its output and the row stays, a cut-off send shows
  `unknown`, and a row clears only once the next state read shows the change. There is no merge button.
- Every whole fast gate run from a linked worktree (the brainer's batch gate) appends one row
  to the main checkout's `setup/suite-data/gate-runs.tsv`: date, tree, the `HW_GATE_BATCH` label
  if set, result (`green`, `red`, `known-flake`, `killed`), the first failing job and the wall
  seconds. The first-try rate of the batch gates is now a query. A gate run in the main
  checkout itself says it leaves no row: a row in the tree under test would change what the
  next gate diffs.
- A red subject listed in `setup/suite-data/flaky-ledger.tsv` that is red at the merge base
  too is printed `known flake` and recorded `known-flake` in `gate-runs.tsv`; the gate itself
  stays red.
- Herdr app: `cmd-shift-f` (`mod.open:foreman-paths`) lists the file paths on the focused terminal's screen, newest first,
  and opens the chosen one in the Files panel; a missing file and a screen with no paths say so. `herdr-app/README.md`
  names which printed links it reaches.
- `hw <lane> <task> --agent pi|opencode|codex` now launches from a terminal of the herdr app, as it does from a herdr TUI
  pane: one `hc run` terminal running the vendor's own binary with the brief as its launch task. Only `--sandbox` and `--here`
  are still refused there.
- **`--sandbox` in the herdr app.** A sandboxed dispatch from an app terminal was refused ("the executor would run
  UNCONFINED"). The run is now armed before `hc run` and the shim's directory goes first on the terminal's `PATH`; the
  broker's heartbeat proves the agent runs inside the profile, and a terminal without one is killed. `hw revive` arms a
  sandboxed run again the same way instead of refusing it.
- **A brainer in the herdr app can reset itself: `brain <lane> --reset --self`.** It was refused ("has no herdr-core
  mapping yet"). The brainer arms the reset and a detached process waits for that turn to end, ends the terminal and
  starts a new one; the lane's open runs are repointed to the new terminal. `--force` and the unreported-executors
  gate work as in a herdr pane.
- `projects.json` names the agents root once, as `root`: brain is `<root>/brain`, work `<root>/work`
  (unless `work` is set), the archive `<root>/archive`. `guards.json` spells the root `{brain}` and
  `{work}`, resolved from the `projects.json` beside it; the guards protect exactly what they did.
- `projects.json` names organisations: `orgs` maps each to the directory its product checkouts live
  under, and every lane names its `org`. A table where a lane names an unknown org, or a
  `product_repo` checkout outside its org's directory, does not load. Organisation-only rules read
  `org`, never a list of lane names.
- `bin/move-root --to <dir> [--dry-run]` moves the agents root (`brain/`, `work/`, `archive/`) by rename
  and repairs what names it: git worktree links and remotes, every account's `settings.json` and
  `.claude.json`, `~/.codex/hooks.json` and `config.toml`, herdr, herdr-app and hc state, LaunchAgents,
  hw's run state, symlinks into the trees and `<root>/CLAUDE.md`, the accounts' project directories,
  and the brain's root-bearing files (committed). It checks the engram labels do not change, refuses
  while any process stands in the trees or herdr/hc is running, and journals every step;
  `bin/move-root --undo` puts it back.
- `bin/move-root --audit`: a hard-coded `<root>/brain|work` outside history, or a root-bearing file on
  another root, is red (`setup/tests/1361`).
- `docs/layout.md` defines each piece once (agents root, brain, work, archive, lane, org, product
  checkout), where each kind of file lives, how to add a lane and choose its org, and how to move the root.
- The Cockpit of the herdr app has a **Dispatch…** control under each brainer: it lists the briefs of that lane that were
  never dispatched, prints the `hw <lane> <task> --dry-run` manifest, and runs `hw <lane> <task> --report-to <the brainer>`
  only after a confirmation. Its three verbs are in `cockpit/dispatch-verbs.txt`, beside the frozen `verbs.txt`.
- The Agents panel of the herdr app shows each executor's model beside its row, from the same `model_running` the Cockpit row reads.
- `hw review <lane> <task> [--dry-run]`: the independent review of a build. It measures the branch's diff (code-only lines: only `decisions/`, `briefs/`, changelog fragments and `docs/` are prose; `.claude/**`, `CLAUDE.md`, `_agents/`, `_skills/`, `plugins/`, `*.golden`, `testdata/` and binaries count), picks tier 0, S (1-400 lines) or L (more), plus Judgment Day when the build's brief lists `judgment-day`, writes the reviewer's brief from `docs/REVIEW-BRIEF.md` and dispatches a fresh claude opus executor on `task/<task>-review-<short sha>`, cut at the reviewed SHA (a product lane is cut from `origin/task/<task>` and refuses a head that branch is not at). The build's receipt gains `review_run`, `review_head` and `review_tier`; the reviewer's run gets `functions.txt` (the functions the diff changed). Tier 0 dispatches nothing and records `review_head`.
- A build's dispatch writes `review_owed=1` to its receipt (not for `review: none` with a `review-skip:` reason, nor for a kind that is not a build), so a forgotten or failed `hw review` is never read as "nothing owed".
- A `kind: review` brief with `review_of: <lane>:<task>@<sha>` gets its own gate keys at dispatch (`plugin_review_t1`, `jd_agents_t1` when Judgment Day is listed, `checks_required_t1`, `review_of_t1`), so done-invoker proves each review piece from the reviewer's transcript. `review-skip` and `review_of` are known frontmatter keys.
- `hw done <lane> <task> --accept-review "<why>"`: a brainer's override of the review gate, recorded in the receipt as
  `review_accepted`. Refused when `HW_TASK` or `HW_RUN` is set, so an executor cannot use it.
- `hw review <lane> <task> --at <sha>`: a blind review of a past head, for benchmarking (a PR before its fixes). The SHA must resolve to a commit in the lane's checkout; the reviewer runs on `task/<task>-review-at-<short sha>`, cut locally at it, so nothing is pushed and no remote ref is needed (a product lane's `origin/task/<task>` comparison is skipped: the tree is cut at the stamped SHA itself, and the reviewer's close gate, `HEAD` equal to the reviewed SHA, is unchanged). The base is the merge-base of the SHA with the lane's base, as before. The build's receipt is not touched (no `review_head`, `review_tier` or `review_run`), so a past-head verdict can never satisfy `hw done`'s merge gate; the dry run says so. `hw help advanced` lists it.
- An independent reviewer's `done-invoker` is held to its review: when the run's receipt has `review_of_t<n>` (written by `hw review`'s dispatch), `review.md` must exist, its first line must be `verdict: approve|changes-needed  head: <sha>` with the reviewed SHA, every row `id | blocker|should|nit|follow-up | file:line | …` must carry a known severity and a file:line, `approve` may carry no `blocker` row and `changes-needed` needs one, and the function table in `checks.md` (`function | what it does | verdict | action`, verdict `simple|split|merge|inline`) must have a row for every function in `functions.txt`. The reviewer's worktree `git rev-parse HEAD` must equal the reviewed SHA, so the bytes it read are the bytes the verdict names.
- `checks.md` of a reviewer may hold that function table: the build's checks gate skips it for a run with `review_of_t<n>`.
- `bin/statusline-look`: the Claude Code status line in foreman's look (`Fable 5.2 · 9.0% · branch · +0 -0`, theme colors, no labels), the same pinned ccstatusline on both accounts, with backups and `revert`.
- `herdr-app/mods/foreman-tabs`: brainer and executor tabs draw the brain and hard-hat icons in the tab bar.
- `setup/commit-tree` refuses a new `setup/tests` subject whose number another subject of the tree already holds (`setup/test-number-check`, the check `setup/test-hw` runs first), naming `setup/next-test-number`.
- `setup/test-hw` repeats the touched subjects the budget cut (`SKIPPED, names your diff and was NOT run: …`) in the last lines of the run, uncapped, so a `tail` shows them.
- `docs/BRIEF-TEMPLATE.md`: a mutant goes through `setup/mutation/safe-mutate` and must compile; a "costs no visible time" claim is 6 interleaved old/new pairs, medians; a pi leg starts with `pi -p --model <m> "say ok"` and is UNVERIFIED if it fails.
- `hw status <lane>` now says when a lane holds 25 or more finished briefs:
  `N finished briefs → hw briefs archive <lane> --apply`. The count is what that verb would move, so a
  fresh archive leaves the status quiet.
- A brief can ask for a review of its tests: `review: [..., tests]` loads foreman's own `test-review`
  skill (plugin `tests`), whose pass criterion is a test that fails on the old code and passes on the new,
  OLD/NEW output pasted. The close is held to it like any review plugin: a Skill call in the transcript and
  a `test-review` row in `checks.md`, and no `review-skip:` for it. Pages for Go, vitest with Testing
  Library and pytest with FastAPI are picked by the diff's file types.
- `webguide`, a vendored plugin: Chrome's Modern Web Guidance (`GoogleChrome/modern-web-guidance`, Apache-2.0, pinned at `4dd9683`). A brief that touches UI loads it with `plugins: [webguide]` after `setup/vendor/plugins/install`; it is not a review piece, and the lane's `CLAUDE.md` wins when its guidance disagrees.

### Changed
- The herdr app's Files panel shows a file the way the file viewer does: markdown is rendered
  (headings, bold, tables, code blocks), code and plain text keep their syntax colors and wrap
  long lines, and a local image is drawn. The three foreman layouts place `foreman-files`, a
  clone of the app's `files` mod, with the tree unchanged; link it with
  `ln -sfn "$PWD/herdr-app/mods/foreman-files" ~/.config/herdr-app/mods/foreman-files`.
- `bin/herdr-isolated start --brain` also puts `cockpit/verbs.txt` and `projects.json` under the brain root the layouts name inside `<dir>/h` (the
  layouts and the History mod read them from the instance's HOME; without them the Cockpit said "buttons off" and History
  listed no product lane). Its stand-in `claude` treats a launch prompt as a turn and runs a line that begins with `!` in its
  own environment, so a QA can ask, challenge, report and dispatch as an executor or brainer. `herdr-app/README.md` has the
  product-lane (dev-server) cycle: a fixture lane under `<dir>/h`, a stand-in `npx` that serves 200 on `HW_PORT_WEB`, and
  `--keep-pane` so the report row stays for **Done**.
- In the herdr app, `brain <lane>` opens its tab named `brain` and `hw <lane> <task>`
  opens its tab named after the task, instead of the app's numbers («1», «2»).
  An executor's dev-server terminals keep the app's numbering, and a tab a person
  already named is left alone.
- foreman-sh is now `foreman`: the repository, the Homebrew formula (`foreman`
  in the tap) and the environment variables
  (`FOREMAN_SH_*` became `FOREMAN_*`). The installer command is `foreman-setup`,
  and nothing links an executable called `foreman`, because homebrew-core already
  has a formula with that name.
- An install made by 0.9.x upgrades to this release with `foreman-setup upgrade
  --brain DIR`. The three variables a 0.9.x installer hands to the new one
  (`FOREMAN_SH_VERSION`, `FOREMAN_SH_INSTALLED_FROM`, `FOREMAN_SH_UPGRADE_TARGET`)
  are still read. The tap records the rename in `formula_renames.json`, and the formula
  keeps a deprecated `foreman-sh` command so a 0.9.x install's own `foreman-sh upgrade`
  completes.
- The README and the reference state what 1.0 promises: `hw`, `brain`, the invokers,
  the brief format, the install and the herdr app, with its mods, layouts and the
  `hc` backend. A change that breaks a working setup takes a new major version.
- An edit to `setup/mutation/mutation-anchors.tsv` no longer selects every subject that builds a mutant: `mutate_anchor` (in `_common.sh`) names the table, so the selector read it as named by every caller, and one row pulled 12 slow subjects (2285 s) into a touched list with no red of the branch's. `setup/gate-select` now selects, for the table, only the subjects whose own text spells it (and 540, which judges it); the 28 `# gate-when:` lines that listed it lose it.
- The sleep/poll guard no longer refuses a bounded `tail -f … | grep --line-buffered` in the `Monitor` tool: a Monitor command is judged as a background run bounded by its `timeout_ms`. Without a numeric `timeout_ms` an unbounded follow or loop is still refused, and the refusal names `timeout_ms` as the form that passes. Nothing is loosened for `Bash`.
- `setup/heavy` keeps the last 40 lines of every run's output (stdout and stderr, the short lane too) in `~/.cache/hw/heavy-out/<tag>-<timestamp>.log`, and the `heavy.log` line now ends in ` out=<that file>`: a red of a batch gate can be classified after the fact. The caller still receives both streams, apart. Files older than 14 days are removed.
- `setup/heavy -- bash setup/tests/<n>-*.sh` (a lone subject, nothing else on the line) takes the short lane by itself when `suite-data/test-budgets.json` budgets it at 120 s or less, instead of queueing behind a frozen holder. A subject over the cap, one the table does not list, or a line with another word queues as before.
- The runner's `ENV-ONLY` label (a red subject that passes alone under `env -i`) now names the variable that turns it red again: each group of the gate's exported variables (the name up to its first `_`) is put back alone into the clean run, and the label reads `ENV-ONLY (red again when only HC_* is put back …)`, the variable's own name for a lone one, or says that no single group flips it and how many were tried.
- `setup/commit-tree` now runs, before it commits, the subjects `setup/gate-select` names for the commit's paths whose budget is 20 s or less (cheapest first, up to 180 budgeted seconds), refuses the commit when one is red (the ref unmoved, the subject and its first `not ok` line named), and lists the slower ones as `NOT RUN here`. Each subject runs in its own session under a wall limit (its budget, at least 20 s) and the whole check under a 150 s allowance: a subject that never returns (the memory guard freezes a tree under 26% free memory) is killed and listed as `FROZEN/TIMEOUT`, never counted as red, and the rest are not started. A red used to surface 30 minutes to 29 hours later, in a batch gate.
- `setup/go-test-core.sh --bless <patterns>` inside an executor narrows itself instead of exiting 75: it runs only the cases whose golden holds a line a pattern matches, through each corpus' own `HW_*_CASES` selector (estimate: partition seconds x the share of cases kept, held to `touched_budget_seconds`). When it cannot narrow (a NEW line is in no golden, a matching case no cases-* file names, or the narrowed run is still over the budget) it exits 75 and prints the focused commands and the `golden-bless` step to run by hand. Outside an executor the whole batch runs as before.
- `cockpit/verbs.txt`: the app's `exec` has no stdin, so for the `stdin=text` verbs the app passes the person's text as one
  argv word in place of the lone `-` (no shell, nothing expanded); a text that `hw` or `channel-send` would read as a flag
  is refused with a message, never rewritten. The Claude cockpit mod still uses stdin.
- The mutation partitions (`m844`, `m848`, `m840`, `m826`, `m861`-`m866`, `m871`, `mbrain`: 21 of the 40 Go partitions, about half the batch's Go work) leave the fast gate and
  run in the release gate: the partition table's `RELEASE=` line names them (`go-test-core.sh --release`), the fast gate in every form queues none and lists the ones a diff
  reaches as `DEFERRED, names your diff: mutation partitions …`, and `setup/release/cut` runs them as one batch after the release suite and refuses the cut when one is red.
  No mutant is dropped; the full run still queues them. The worst-case fast gate estimate falls from 1253 s to 818 s.
- The fast gate names what it did not run: a slow subject that names the diff and was left out
  is listed in the last lines as `DEFERRED, names your diff: <n> -> bash setup/tests/<file>`.
- An executor's touched form that would queue Go partitions over `touched_budget_seconds` exits
  75 before any subject runs, naming the focused `go test -run` form; the refusal for a path no
  subject names now prints a loop that runs the reference subjects, and the executor's touched
  run says how to check a freeze (`ps -o state=`), since it runs outside `setup/heavy`.
- `mutate_anchor` fails at its first use, naming `setup/mutation/mutation-anchors.tsv`, for an
  anchor id the registry does not list (it failed only at subject 540, the end of the gate).
- Herdr app cockpit: the «N reports to read» block folds from its header (kept across restarts; a new report reopens it), and a report's summary line shows only when it says more than «Done».
- Herdr app Agents, «Grouped by space»: the brainer is the first row of its space, its executors follow; a group's fold is kept across restarts.
- **What the herdr app still refuses says why.** `brain --in-pane`, `hw … --here` and `hw revive` of a pi, opencode or
  codex run on the app used to say "no herdr-core mapping yet: this step…" or name a state that was no longer true
  ("only claude executors are launched"); each now gives the measured reason and the command that does the job there.
- The readers of the lane table (`bin/project-spaces.sh`, hw-core, the label proxy, cockpit-state,
  brainer-metrics, suite-gate, the invokers (Go and their shell fallback), the suite runner, `install.sh`, the zsh
  completion) take `work` from `root` when the table does not set it.
- The rules, docs, skills and scripts say `<brain>` and `<work>` instead of one machine's paths; an
  organisation-only rule names the `org` it applies to.
- The README has a "Tools foreman works with" table: every tool foreman installs, pins, launches or reads,
  whether it is required, and its version floor or pinned commit, including the `tests` review piece and `webguide`.
- The README's diagram is a 1920×1080 architecture picture (`docs/foreman-architecture.png`, source
  `docs/foreman-architecture.html`): brainer, `hw`, executors, the return channel, engram, review pieces, `setup/heavy` and the way to a merge.
- A build owes the independent review by default: every `review:` value on a `kind: build` (or no kind) owes it, whether the key is absent, `judgment-day`, `rdd`, `plugins`, a list or a bare `none`. The one opt-out is `review: none` WITH a `review-skip: <why>` line. A bare `review: none` still owes the review and warns for one release (it is refused after that).
- The builder no longer runs Judgment Day or a review skill. Its closing is: commit per unit, its own OLD/NEW tests, and `/simplify` on claude (proved by the Skill call `simplify`, receipt `plugin_review_t<n>`; pi: `pstack:deslop` when loaded, unproved; codex and opencode: not available). The reviewer runs Judgment Day when the brief declares `judgment-day`, `rdd` or a list naming it.
- `hw review` adds the plugin items of the build's `review:` list to the reviewer's tier, and `review: plugins` adds the brief's `plugins:` (S4); `rdd` is accepted in a list as Judgment Day.
- A build that owes the review launches with `--keep-pane` by default (a pane is needed for the one fix round); the manifest says it was the default. The manifest's `review` line names the independent review for every build.
- The docs now say what the independent review is: every `kind: build` owes it whatever `review:` says (only `review: none` + `review-skip:` opts out), `hw review` picks the tier by code-only lines (0 / S 1-400 / L +ponytail and two pstack principles / +Judgment Day when the brief names it), the builder closes with OLD/NEW tests and `/simplify` and runs no Judgment Day, and `hw done` offers «Merge it» only for an approved head or the brainer's `--accept-review`. Updated: the `review:` paragraph of `BRIEF-TEMPLATE.md`, `INSTALL.md`, `REFERENCE.md`, and the brainer rule that still called the review opt-in.
- `hw review --at <sha>` (a benchmark of a past head, never the merge gate) is documented, with its one known gap: on an already-merged commit the base is the current base, so it counts 0 lines and prints tier 0 (KNOWN-LIMITATIONS L34).
- `hw help flags` says what `review:` means now (a build owes the independent review whatever it says; `review: none` opts out only with `review-skip:`), instead of «a review is opt-in». The export README's «Reviews you opt into» bullet says the same, and two of its feature bullets were joined onto one line each to stay inside test 1024's 380-line budget (no wording dropped).
- `hw done` no longer prints «Merge it» for every branch with a commit ahead. It recomputes the code-only lines at the branch head
  and offers the merge only for an approval at that head (the reviewer's `review.md`), a diff with no code, a recorded
  `review-skip:`, or `--accept-review "<why>"`. Otherwise it prints `NOT MERGEABLE: review <verdict> at <sha>, head is <sha>`
  and why. A missing verdict is a denial, never «nothing owed». Owed follows the diff, not the brief's kind, and a review run's
  own branch is never offered.
- On a product lane `hw done` compares the reviewed SHA with the PR's `headRefOid` (read-only `gh pr view`) and prints
  `gh pr ready <n>` only for a draft PR at that head. Builds open their PRs with `gh pr create --draft`.
- The three foreman layouts show `foreman-tabs` instead of the shipped `tabs` mod.
- The two accounts' `statusLine` no longer differ (`@2.2.27` vs `@latest`).
- `setup/heavy --short` says, on its exit-124 kill, to rerun without `--short` (a cold `go test` takes a slot) and that `--max <s>` only lowers the 120 s cap; the printed focused-run forms (`setup/go-test-core.sh`, `docs/BRIEF-TEMPLATE.md`) say the same.
- The Claude status line says the context once, `▓▓░░░░░░░░ 170k/1M`, and colors the bar by use:
  muted below 60%, amber below 85%, red above.
- Its git counts are the branch's commits against `main`, and absent on `main` itself, instead of the
  working tree's uncommitted changes.
- The refusals for a bad `review:` list and for `review: plugins` with no review plugin now name `tests`
  among the plugins that carry a review skill.

### Deprecated
- Codex and OpenCode are outside the 1.0 promise. They keep working, and either
  can be removed without a major version.

### Removed
- The Cockpit's **Dispatch…** control and its `cockpit/dispatch-verbs.txt`. A brief is dispatched from its
  brainer with `hw <lane> <task>`.
- The herdr app's **Artifacts** panel (`foreman-artifacts`): its mod, its entry in the right column's switch
  and stack in the three looks (C, D, G), its tests and its README sections.

### Fixed
- `herdr-app/mods/foreman-cockpit/looks.test.mjs` (run by test 1272) no longer asserts QA-recipe strings that the README
  moved into `bin/herdr-isolated`.
- In the herdr app (the `hc` route, where a brainer and an executor are core terminals, not herdr panes), the Cockpit now
  offers **Reply** and **Rule**: the state file's `ask` carries the hold's `route` (`herdr` or `hc`, null with
  `pending_reply`) and `cockpit/verbs.txt` hands it to `channel-send` as `<route>` where it said `herdr`. Before, the writer
  set `pending_reply` only for a herdr-route hold, so an ask or a challenge raised under the app showed no button.
- The Issues hand-off finds the lane's brainer under the app's core: when `herdr pane list` has none it lists `hc ls --json
  --label label=brain` and delivers with `channel-send hc <terminal>`. Before, it answered "The brainers could not be listed:
  no herdr server is running". A refused send now says its cause, not channel-send's closing "my stdout is NOT a terminal" line.
- `hw done` no longer refuses a report as STRANDED when its task branch is already merged into
  the lane's base by ancestry: a `done-invoker` exit 5 (delivery unconfirmed) whose report the brainer
  received and merged is treated as delivered. A branch still at its start point proves nothing and stays refused.
- `herdr-isolated stop` ends every process whose command line names the instance directory (the cockpit
  writers of a `--brain` instance, which hold no socket and used to respawn a core under it), and
  TERMs again on every wait round instead of leaving a respawn to the KILL.
- `herdr-isolated start` canonicalizes `<dir>` to its physical path, so the app's home and the pane cwd agree
  (`/tmp/x` is `/private/tmp/x`).
- `herdr-isolated shot` puts the app that was in front back in front once it has taken the frame.
- `setup/commit-tree` refuses a commit that names the suite's time-budget table (`test-budgets.json`, its ceilings,
  the retired list or the partition table) when `setup/test-budget-gate` finds the new tree's table over budget, and
  `setup/test-budget-gate` now prints the headroom (full suite and fast gate) on green as well as on red.
- `setup/release/cut` with no `--out` and no `HW_ARTIFACTS` writes its evidence to `~/.cache/hw/release-<VERSION>-<n>`
  (the next free `n`) instead of a mktemp directory, and prints `evidence: <dir>` as its first line and its last.
- `done-invoker --check` with no summary prints a usage line that names `--check`, and its exit footer says
  `CHECK REFUSED` (a rehearsal delivers nothing by design) instead of `NOT DELIVERED`.
- `hw`'s dispatch manifest, for a setup `kind: build` brief with no runnable Verification fence, shows
  `verify      ! setup build with no fence: paste the BRIEF-TEMPLATE Done-when fence` instead of a plain `none —` line.
- `hw done` and `hw reap` no longer list `gpui-kit.d.ts` and `jsconfig.json` (ignored, regenerable) as «DO NOT REMOVE THIS WORKTREE YET». The regenerable list was copied in three places (`core/reap.go`, `core/t4_done_wt.go`, `core/t4_reap_wt.go`); it is now one constant in `core/wt_regenerable.go` that names both files.
- `setup/test-budget-gate` leaves the release-gate partitions (the `RELEASE=` line of `go-test-partitions.txt`: the `m*` mutation partitions) out of its worst-case fast-gate estimate; the full-suite estimate still counts them. Measured on this tree: 1252 s before, 817 s after.
- `HEAVY_ONLY=<F..> bash setup/tests/1040-…` runs only the claims it names; it ran every mutant
  after them.
- A gate run in the main checkout no longer reads the runner's own `gate-runs.tsv` rows as a changed path (it narrowed the
  in-process suite to 0 Go partitions); a full run in an executor ignores `HW_TEST_CHANGED` again instead of hitting the touched
  form's refusal.
- The close after a report now ends the herdr app terminal of a `--worktree none` task in a repo lane:
  `hw done` looked for its run record only under the lane's worktree path, found none, searched herdr's tabs and said
  "NOTHING TO CLOSE" with exit 0, so the terminal stayed open and the report piled up in the cockpit.
- The close that `done-invoker` starts after a report no longer exits 0 when it found no tab, space or terminal to close:
  it says "NOT CLOSED — NOTHING FOUND" and exits non-zero, so the task keeps the `close-failed` file `hw status` shows.
  A hand-run `hw done` on a task that opened nothing is unchanged.
- `done-invoker` and `ask-invoker` no longer refuse an opencode executor started with `--prompt <brief>`: the nested-headless
  guard read the word `run` inside the brief as `opencode run`.
- An opencode executor's turn-end plugin publishes in a herdr-app terminal (`HC_TERMINAL_ID`), not only in a herdr TUI pane.
- Herdr app dispatch (`hc run`) now records `model_running` in the receipt, as the TUI route does, so `bin/cockpit-state`
  and the cockpit row show `vendor · model · effort · ctx` for app-dispatched executors.
- **The stall alert and `setup/heavy`'s FROZEN alert reach a brainer in the herdr app.** Both run
  `channel-send --report herdr "$HW_INVOKER_PANE"`, and an app brainer's address is a terminal (`t24`) that herdr does not
  know: every alert since the move to the app ended in a log as "did not reach pane t24 (channel-send exit 1)".
  `channel-send` now sends the `herdr` word through `hc` when the target is an app terminal and the caller runs on
  herdr-core; a herdr pane id (it carries a colon) and every caller in a herdr TUI keep the herdr route. The vault alert
  uses the same command but still finds the brainer through herdr only (L29).
- **`hw status` for executors in the herdr app.** It read "context: not measured" for every app executor (it asked herdr
  for a terminal herdr does not have) and now reads the terminal's screen with `hc read`; a finished app run read "its
  placement record is gone, so what closed is not recorded" and now names its terminal ("app terminal t4 closed"); a
  stalled or stale app executor is to be read with `hc read <terminal>`, not `herdr pane read`.
- **A `--keep-pane` lease on the herdr app expires.** Its expiry closed nothing for an app run
  (`cleanup=no-owned-container`), so the terminal stayed open for ever; it now ends the run's terminals as `hw done` does.
  The lease also names the terminal that started it instead of `by=unknown`, and an expiry ends terminals only while
  its own run's terminal is still one of the task's.
- **`hw unstick` on an app terminal that names no agent** refused with "t5 is , not OpenCode"; the sentence now reads.
- The Rule button of the herdr app now sends: `hw ruling t5 …` run by the app's own `exec` (no `HC_TERMINAL_ID`, but the app's
  `HERDR_APP_HOME` and `XDG_RUNTIME_DIR`) was refused as "not running as a terminal of an app". A process the app started is now on
  the app's backend; a process with a herdr TUI ancestor, or without those variables, is still refused.
- The answer to an executor's ask in the herdr app now marks the ask `hw_ask_state=answered`; `hc get` kept `delivered` after the reply.
- `done-invoker` reads a background Judgment Day judge's answer: an Agent launched with `run_in_background` answers its tool call only with "Async agent launched", and its findings arrive as a `<task-notification>` (a `user` line, or a `queued_command` attachment). The protocol check used to say «not checked — the judges' results carry no readable findings» over a full ledger; it now reads the notification (once, however many lines repeat it) and holds a background re-judgment to the frozen ledger like a foreground one.
- `hw review --at <sha>` no longer says «review_owed=1 still stands in the build's receipt» when the dispatch fails: a past head never touches that receipt. It says so and prints `hw review <lane> <task> --at <sha>` to re-run.
- `docs/REVIEW-BRIEF.md`: the reviewer checks binaries itself and keeps them out of a judge's target paths; judge prompts carry intent, not reassurance (a test value is not "fine"); three more defect classes (a test or placeholder value reaching production, tests reaching the network through an unpatched client, user-visible text driven by placeholder data).
- A test run can no longer label a live herdr-core terminal: `setup/test-channel-send` puts a tripwire `hc`
  first on PATH (a call is recorded and refused, and fails the file), so the brainer's terminal is no longer
  drawn as an executor with the fixtures of that test.
- The herdr app's Cockpit buttons (Reply, Rule, Done) run `hw` and `channel-send` from the brain's `bin/`.
  They died with «cannot run hw: No such file or directory» because the app's PATH did not carry it.
- The Spaces list drags with a reducer (`foreman-spaces`, a clone of the app's `spaces`): a drop no longer needs a mouse-move
  to have reported the target, and leaving the list cancels the drag. The live pointer path is unverified.
- The gate's snapshot wrapper (`setup/test-hw-snapshot.py`) reads the runner's output as bytes. A subject that
  printed bytes that are not UTF-8 raised `UnicodeDecodeError` in the wrapper, which removed the private
  snapshot while the runner kept going, so every later bash subject read «does not parse».
- The release cut no longer re-runs the fast gate for the commit that only folds the release notes: a
  candidate that differs from a first-parent ancestor with a green fast-gate verdict only in the changelog,
  the release notes and the removed fragments reuses that verdict, and the gate line says which tree and paths.
- A partition of the in-process suite whose only failure is go test's `-timeout` panic, in a run whose process
  tree was seen stopped (the memory guard's freeze), is run once more instead of turning a frozen machine into
  a red cut.
- `bin/statusline-usage-tee` hands the status line command the last 8 MB of a long transcript (`HW_STATUSLINE_TAIL_MB`, 0 turns it off) instead of the whole file: ccstatusline read a 473 MB transcript on every refresh, at 2.8 GB RSS and 4.3 s each; now 178 MB and 0.56 s, same output.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.9.0-rc.1

Run `foreman-setup upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
