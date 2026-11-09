# foreman-sh 0.8.0-rc.1

foreman-sh 0.8.0-rc.1 brings the herdr desktop app to foreman. It also makes corrections reach an executor while it works, and halves the time the fast gate takes.

## In short

- **The herdr app as a cockpit.** Native mods add a Cockpit, Issues, Artifacts and a markdown viewer to herdr's app, with Linear, Zed and other looks. The Issues panel shows each fact once: rows are grouped per repo, titles are aligned and the filters take two rows.
- **A ruling reaches a working executor mid-turn.** A queued correction lands between two tool calls through a PostToolUse hook, instead of waiting for the turn to end. A consumed ruling's text stays on disk even when the output is truncated.
- **The fast gate takes half the time.** The worst case drops from 3643 s to 1556 s: Go partitions run seven at a time, longest first, and no test was removed. Contract goldens store the prompt byte count as a placeholder, so a prompt edit rewrites 28 goldens instead of 96.
- **Executors fail fast instead of waiting.** A wait that cannot progress names its holder and exits. `hw done` recognises a run that already closed, and pi stops on a spent quota with a report.

## What changed

136 changes since 0.7.3.

### Added
- `setup/heavy` keeps the Mac awake while a job runs: `caffeinate -i -w <job pid>`, a user process that
  ends with the job (even if heavy is killed), no `pmset` or system setting.
- `hw help next` answers, and says how to re-task or reopen an executor: `hw next` for a live reported
  one, `hw revive` then `hw next` for a closed one, and that a hand-written `channel-send` opens no task.
- `setup/next-test-number [N]` prints the next free `setup/tests/NNNN-*.sh` number, reading main, HEAD, the working tree and every live local `task/*` branch (a collision renumbers the subject the colliding branch introduces, never another's; `docs/BRIEF-TEMPLATE.md` says so).
- `done-invoker` now also requires, for each review plugin the closing named, a row in `$HW_ARTIFACTS/checks.md` that names the skill with a verdict (pass, fail or unresolved) and its findings or "no findings" — loading a skill is not applying it. A `review-skip:` line still releases a skill.
- The Herdr app's Artifacts panel (`herdr-app/mods/foreman-artifacts`): the
  claude.ai artifacts of the Teams (default) and personal accounts, listed with
  title and last update, where a click draws the artifact's own page in the panel
  and opening it in the browser is the second action.
- `brain <lane>` run from an herdr-core (`hc`) terminal, as the herdr app starts one, opens the brainer as an hc
  terminal instead of a herdr workspace tab: labelled `label=brain`, rooted at the lane's brain directory, opted into
  the app's window with `herdr.space=<the lane's space>`, and shown with `herdr-app do agent.focus` — so `brain setup`
  puts the brainer on screen in the app. An open brainer is focused by its cwd, never built twice.
- An executor dispatched from that brainer resolves `HW_INVOKER_PANE` to its terminal: the resolver reads `hc ls` and
  `hc get`, normalised into `herdr pane list`'s shape, so the manifest's invoker line, its "verified as label=brain
  cwd=…" proof and its refusals are the same words on both backends. A failed `hc` read is reported as unverified, never
  as "no brainer".
- `channel-send hc <terminal> - <message>` delivers to an agent living in an herdr-core terminal (a brainer opened by `brain <lane>` in the herdr app, or an executor), with the same exit codes (0/1/4/5) and the same report-versus-redirect rules. Redirects wait for the receiver's `idle`/`done` status with `hc wait`; `hc send` is never given `--if` or `--force`, and a blocked receiver is refused, not typed into. Delivery is proven from the receiver's own status, `turn` and turn-end labels; a send nothing confirms within `HW_HC_PROOF_MS` (default 15000) is exit 5.
- `done-invoker` and `ask-invoker` reach a brainer in an hc terminal, and `ask-invoker` works there: its hold records route `hc` and the terminal's own address (`holdfacts` accepts it). `hw ruling` reads an hc executor with `hc get` and queues for the turn end.
- The foreman cockpit in the Herdr app: `herdr-app/mods/foreman-cockpit` draws every live brainer's
  executors — status, model and context, commits, the running suite, subagents, and what waits on that
  brainer — from the same `bin/cockpit-state` file the Claude Code cockpit reads, and
  `herdr-app/layouts/foreman.json` puts the cockpit on the left, the terminals in the center, and an
  empty column on the right for the viewer. Installed by symlink; `herdr-app/README.md` says how.
- `hw` run from a herdr-app (herdr-core) terminal launches the executor there: one `hc run --bg --json --no-parent --no-notify` terminal named for the task, labelled `hw_project`, `hw_task`, `hw_run` and `herdr.space`, with the herdr variables unset in its argv and the brief as claude's launch task, proved by `hc get`. A second dispatch of the same task finds the live terminal with `hc ls --label hw_project= --label hw_task=` and launches nothing. `--dry-run` names the route on the `placement` line; from the herdr TUI nothing changes. Claude executors on the plain lanes only for now (KNOWN-LIMITATIONS L29).
- `hw done` of an executor that runs on the herdr app (an `hc` terminal) now ends it: it kills and removes the run's terminals found by their `hw_project`/`hw_task` labels (a terminal with other labels survives), and the in-use guard reads the terminal's status and report from `hc get`. `hw reap`, `hw status` and the worktree occupancy count those terminals as occupants. A run without the `.hw/<run>/hc` record makes no `hc` call (KNOWN-LIMITATIONS L29).
- A foreman Issues panel for the Herdr app: the GitHub issues that involve you
  (authored, assigned, mentioned, involving), with a text filter, scope, state
  and repo filters, a refresh, and a preview of any issue — title, state,
  author, labels, markdown body and comments — inside the panel. Read-only
  through `gh`, with clear states when it is missing, signed out or offline.
- The Herdr app has a second look beside C: `herdr-app/themes/d-linear.json` (Linear: `#0f1011` canvas, inset `#17181c` panel, one `#26282d` line, indigo accent, yellow working, red blocked). Pick it with cmd-shift-P, `app.theme`; C stays the layout's default. `looks.test.mjs` now proves no foreman mod hard-codes a color or a look, so each renders with whichever theme is active.
- Every Herdr app look shows a `C D G` look switcher at the right end of its right-column switch bar, the shown look marked; a click runs `layout.switch {name}`. `cmd-shift-l` still cycles every layout. Held by `switcher.test.mjs` and proved in the isolated instance (19/19 per look).
- `hw next <terminal>` re-tasks an executor living in an herdr-core terminal: the gate reads `hc get` (idle/done opens it, blocked is a person's prompt, otherwise it waits with `hc wait`), keys go through `hc keys`, and the task is delivered with `channel-send hc`. The TUI path is unchanged.
- `hw outbox flush <lane> --to <terminal>` delivers through the `hc` vendor when the target is an hc address, and `brain <lane>` in the app flushes the lane's outbox to the brainer it opened or focused (waiting for a started one to report a status first).
- The read-side tools see executors that run as herdr-app (herdr-core) terminals: the cockpit state and `hw status` list the live `hc ls --label hw_task` terminals with their status and turn from `hc get`, `state-witness` falls back to `hc get`/`hc read` when herdr knows no such pane, and `stall-watch` ends when its `hc` terminal is gone instead of treating a missing `HERDR_PANE_ID` as alive. The cockpit writer bound to an app brainer no longer exits as "pane gone". Nothing changes when no `hc` terminal runs (KNOWN-LIMITATIONS L29).
- An executor running in an herdr-core (`hc`) terminal, as the herdr app starts one, publishes its turn-end and
  invoker state as `hc label` keys (`hw_turn_state`, `hw_turns`, `hw_done_state`, …), proved by an `hc get --json`
  read-back: the same fact the TUI reads from herdr tokens.
- When both variable sets are present, the nearest `herdr`/`herdr-core` ancestor decides the backend, so an app
  terminal that inherited `HERDR_ENV`/`HERDR_PANE_ID` from whoever launched the app never writes onto that pane.
- `herdr-app/mods/foreman-viewer/`: a Herdr app pane that shows one file — markdown
  rendered, code read-only with syntax highlighting and line numbers, images drawn when
  they live inside the app folder, any other format as plain text. Open it with
  `herdr-app do pane.split:right --mod foreman-viewer --props '{"file":"/abs/path"}'`;
  the README has the `fv` shell one-liner for opening a path from a terminal.
- The Herdr app has a third look beside C and D: `herdr-app/themes/g-zed.json` (Zed's One Dark: chrome `#3b414d`, docks `#2f343e`, terminal `#282c33`, one `#464b57` line, blue accent, yellow working, red blocked, One Dark's terminal ANSI). Pick it with cmd-shift-P, `app.theme`; C stays the layout's default. The mockup's Plex/Lilex fonts are not installed or applied: the layout's Geist Mono stays.
- pi has two named model rosters and one switch: `pi_roster` in `projects.json` is `go` (the paid opencode-go roster,
  exactly as before) or `free` (opencode's free models: `crouton-alpha-free` for build, design, audit, explore and the
  explorer, `beam-free` for review, each the other's fallback on a model error). It ships as `free` while the paid
  quota is spent; going back is changing that word, no code. Absent means `go`. `--model` still overrides the primary.
- The dry run names the active roster (a `pi roster` line) and the receipt records it (`pi_roster`).
- Under `free`, a real `--agent pi` dispatch (task name without a `test` or `probe` word) prints one warning: pi runs
  on free models for testing only. Nothing is refused. The lanes' `pi_subagent_models` rows name paid models and apply
  to `go` only.
- `hw` prints the opencode-go usage (rolling, weekly, monthly) as a `pi usage` line in the dispatch manifest and in
  `--dry-run`, with a WARNING at 70% or more of any window or a status other than `ok`. It never blocks and fails open.
- `deny_setup_brain_writes.py` also allows a `python3 - <<EOF` whose body only quotes the brain path as text (`write`, `replace`, `format`); `open()`, `os.*`, an assignment, an f-string or an unparseable body stay refused.
- `setup/commit-tree` warns, never refuses, when a new `setup/tests/N-*.sh` takes a number main or a local `task/*` branch already owns.
- The export's "exported tests with no measured budget" error prints the entries ready to paste into `setup/suite-data/test-budgets.json` (`setup/export/narrow_budgets.py`).
- `setup/test-hw` fails on a repeated test number not in `setup/suite-data/test-number-dups.txt` (`setup/test-number-check`). The cockpit subject is now 1272 and the pi plugin-agents subject 1273.
- `setup/tests/contract-hw/regen-grep.sh '<text>'` runs the `--grep` regeneration over t3, t4, t5 and t5l.
- `setup/pi/test-hw-executor.mjs` with no arguments prints its usage and exits 2.
- A Claude executor whose turn dies on an API error or a spent quota (StopFailure, which fires instead of Stop) now publishes `turn_state=ended_api_error` and clears `children_running`, so `channel-send --ruling` and `hw ruling` stop reading the pane as awaiting a child. `install.sh` registers `hooks.StopFailure` beside `hooks.Stop`; run it again to pick it up.
- `hw ruling` against a pane that says `ended_awaiting_child` with a `turn_ended_at` more than 10 minutes old warns "probablemente obsoleto" and names the plain `channel-send` that delivers. The ruling is queued all the same.
- The setup brain guard's python-heredoc exemption applies only to a quoted delimiter.
- `setup/heavy --status` shows `[setup]` or `[product]` on every holder and queued job, and queued jobs in the order they are served.
- A guard (`setup/guards/deny-headless-chrome-keychain.py`, installed by `setup/install/install-headless-chrome-guard.sh`) refuses a `--headless` Chrome/Chromium launch without `--use-mock-keychain` and `--user-data-dir`: without them macOS pops a Keychain dialog. It reads the command line only; a launch from inside node code is not seen.
- `setup/heavy --short` warns when it is asked to run a `go test` with no `-run`, pointing at `setup/go-test-core.sh --affected`.

### Changed
- A brief that declares Judgment Day and carries `## Goal` and `## Done when` has the close tell the executor to hand both
  sections, verbatim, to each judge beside the diff; each judge answers per Done-when rule: held / broken (file:line) /
  not judgeable from the diff, and the report lists every "not judgeable" rule. A brief without both sections gets the
  close it had.
- `docs/BRIEF-TEMPLATE.md`: `## Done when` asks for the rules the behaviour must hold on real data and what the feature
  is for (a rule that can only break on real data is also a check in `## Testing / Verification`); `## Out of scope`
  carries the smallest-change line (`setup/CLAUDE.md` points at it); `## When you finish` says a dropped item is a `pass`
  row with `dropped: <proof>`, since done-invoker accepts only pass, fail or unresolved.
- `setup/gate-select … touched` lists first the subjects that spell a changed path whole (the narrowest path first), then
  those that name only its bare word, then those reached through the closure; cheapest within each. Same set, same cut. On
  foreman-friction-1106's range 1131 and 1014 move from #230 and #229 of 236 to #19 and #18.
- The CLI cockpit's cards read clean at 66 columns: the ruling input opens only on the card whose `ruling` button was pressed (and closes on a second press or once queued), the title is one line that truncates with `…` and leaves out a brief title that only repeats the task (`Brief: <task>`), the status keeps its width, the `▰▱` glyph bar and the trailing `· unknown` are gone.
- The reasoning level of a dispatch has one door on every vendor: `--effort`. A level written inside the model id
  (`--model opencode-go/deepseek-v4.1-flash:max`, pi's own CLI spelling) is refused before anything is built, naming the
  model, the level and the `--effort <level>` to use instead, instead of being dropped while the roster's own level won.
  The refusal is one check for claude, codex, opencode and pi, on `--model`, on a brief's `model:`, and in `hw next`.
  A `:` that is part of a model id (`:batch`, `:free`) is untouched.
- The dry run's effort line reads the same on every vendor: `effort=<value>  (chosen)` when `--effort` asked, and
  `effort=<value>  (default; <source>)` when a default supplied it — the lane's declared default for that vendor
  (pi's roster, by the brief's kind) or `the vendor's own`. The values are unchanged; pi still defaults to low.
- `setup/go-test-core.sh` waits at most 300 s for the batch lock when it runs under `setup/heavy` (`HW_HEAVY_RUNNING=1`), instead of 3600 s, so a gate holding a heavy slot fails within minutes naming the holder. An explicit `HW_GO_TEST_LOCK_WAIT` still wins.
- The executor sleep/poll guard now refuses a `go test` over `core/` in the background too when it names `./...` or has no `-timeout` of at most 10m, and points to `setup/go-test-core.sh --affected "$HW_ARTIFACTS/changed" --run`.
- `hw review-range` warns on stderr that the range is EMPTY when the base is HEAD (a `base: task/<x>` brief); exit 0 and the SHA stay.
- The executor prompt, `setup/CLAUDE.md` and `docs/BRIEF-TEMPLATE.md` say plainly that an executor verifies with touched subjects and partitions only and the full fast gate is the brainer's; they also say the gate reads a snapshot taken when it starts, and use `$HW_ARTIFACTS/changed` instead of `/tmp/changed`.
- The executor prompt line now gives the touched-subjects form (`HW_TEST_CHANGED=… HW_TEST_GATE=fast bash setup/test-hw --keep-going`, in the background) and says the full gate is the brainer's.
- `checks.md` rows are split only on unescaped pipes, so a `\|` inside a code span no longer shifts the verdict column.
- The guard message and `go-test-core.sh --help` give the one-test form `setup/heavy --short -- go test -count=1 -run <T> ./<pkg>`; a red partition prints its `--- FAIL:` lines right above the `not ok` line.
- `deny-blind-process-kill.py` refuses `pkill -f` on `go-test-core.sh`, `go test`, `test-hw` and `setup/tests/` (shared between executors) unless the pattern names the caller's own worktree path; kill by PID.
- `hw ruling` writes `RULING-STEER-FAILED` (vendor, exit, herdr's reason) or `RULING-STEER-SKIPPED` (vendor, why) to the run's `transcript.log` when a ruling for a pane that could be pi is queued unsteered, so a ruling that waits 40 minutes leaves a trace. The cause of the unsteered rulings in herdr-app-viewer is UNESTABLISHED.
- The executor prompt's pi preamble budget moves 5300 -> 5400 B for the touched-subjects form.
- `setup/go-test-core.sh --affected … --run` prints `estimated N s over M partitions` first; inside an executor (`HW_TASK`) it exits 75 over `touched_budget_seconds`, naming the focused `setup/heavy --short -- go test -count=1 -run <T> ./<pkg>`.
- `hw` warns at dispatch when a setup brief's Done when or Verification orders a run of `setup/test-hw` (the touched form with `HW_TEST_CHANGED` is the one mention that passes); the executor prompt's done-invoker line names `done-invoker --check`.
- `channel-send` refuses an endpoint that starts with `--` (`channel-send herdr <pane> --report - msg` took `--report` as the endpoint and waited 90 s); the executor prompt says frictions go to `mem_save` under `<lane>/<task>/friction` with the #id in the done summary, never by `channel-send`; `docs/BRIEF-TEMPLATE.md` says a new top-level path needs its `setup/export/manifest` line in the same commit.
- `setup/commit-tree` asks `setup/export/check` over the tree it is about to commit, in seconds, and
  refuses what the check refuses. It runs no hook, so a leak used to surface only in a gate of hours.
- `setup/CLAUDE.md`: before the gate, an executor that changes a message's text searches its old text in `setup/`, `core/` and `docs/` tests.
- `setup/install/install-gh-image.sh` builds gh-image from the audited source instead of downloading a release binary:
  it fetches the pin's commit by full sha, builds it with `go.sum`-checked modules (`-mod=readonly`, no cgo, an
  environment of its own) and installs it only if its sha256 is the pin's. `--from` is now a source checkout, refused
  unless its `HEAD` is the pin's commit. The pin records the commit, the Go toolchain and the build's hash
  (darwin-arm64); the old release binary is refused by `bin/gh-attach`. Reinstall with `setup/install/install-gh-image.sh`.
- The contract goldens (`golden-t4`, `golden-t5`) store the prompt's byte count as `@N@` ("deliver @N@ bytes", "executor, @N@ bytes)"), in `contract_norm` and its in-process twin `inprocNorm`: a prompt-text change no longer rewrites ~96 goldens whose only change was that number.
- `setup/heavy`: setup jobs may take every free slot while no product job holds a slot or waits; the reserve (slots - 1, or `HW_HEAVY_SETUP_MAX` when set) applies the moment one appears. The memory floor is untouched.
- The headless-Chrome guard now runs on the shared tokenizer `sd.tokenize` (`deny-sd-shell-var.py`, installed beside it) instead of its own shlex code, with a shared corpus `setup/guards/shell-vectors.json` run by its test; an internal error refuses. It now sees `$(...)`/backtick launches, `do`/`then`/`if` keywords, `sudo -u x`/`timeout -s SIG` option values and `open -a "Google Chrome" --args`, and `2>&1` no longer splits a complete launch.
- `--reset`, `--resume`, `--self`, `--await-idle` and `brain relaunch` have no herdr-core mapping yet: run from an app
  terminal they are refused by name, with "Nothing was sent and nothing was restarted".
- A plain herdr TUI pane is unaffected, byte for byte: with no `HC_TERMINAL_ID` the brainer takes the workspace/tab
  path it always took and `hc` is never executed.
- KNOWN-LIMITATIONS L29: delivery to the app and the ask hold now work; `hw` dispatch, `hw next`, `hw done`/`hw reap` and the outbox flush on hc still do not.
- The herdr app's cockpit shows the whole text of a title, meta, progress, report or attention line on hover, since each is clipped with `…` at the panel's width. The task name (which wraps) and the folded subagents summary line have no hover.
- A cockpit row is wired to focus its executor's terminal on click when the app's core (`hc`) holds that terminal. Today the state writer records only TUI pane ids (`w1:p11`), which the app cannot reach (KNOWN-LIMITATIONS L29), so a click does nothing yet; it starts working once an executor runs in `hc` and the writer records its terminal id.
- The Issues panel (`herdr-app/mods/foreman-issues`) says each fact once, in all three looks: a row names its repo without the org, and only when the list mixes repos (a chosen repo chip, or a group of one repo, already names it); state shows only under `Any`; dates are one relative format (`5d`, `2mo`, `1y`) instead of `13d` beside `2026-09-04`; the title takes the freed width. The header drops the panel's name (the tab or the switch label says it) and shows its count only when a filter narrows the list or the 200 cap cuts it; `reading…` is said once. D and G lose the `Open 22 · updated ↓` band. The issue viewer shows the whole title, wrapped, under its head. The rules are plain functions in `rows.js`, held by `rows.test.mjs`.
- The Issues panel (`herdr-app/mods/foreman-issues`) draws its filters in two rows instead of four under a header bar: `updated` and Refresh (and the count, when a filter narrows the list) join the search row; Work/Personal, Involving/Author/Assigned/Mentioned and Open/Closed/Any share one row, a wider gap between the groups, wrapping a whole group at a time in a narrow panel; the repo chips keep their own row. The first issue row starts 88 px higher in C and G and 92 px in D at 816 px. Every control, count, selection and copy is unchanged. Held by `rows.test.mjs` item 12.
- The Issues panel (`herdr-app/mods/foreman-issues`) names each repo once: under `All repos` the rows sit under one quiet header per repo (its short name and count — C's label, D's band and G's row from `tokens.js`), in the repo chips' order, and no row repeats the repo; with one repo shown there is no header. The `#number` sits in a fixed-width column (the longest number's), so every title starts at the same x in C and D; G already aligned its titles under the row mark. Held by `rows.test.mjs` items 10 and 11 (`byRepo`, `numWidth` in `rows.js`).
- The Issues panel splits into Work (every repo of one configured organisation) and Personal (the rest), with the repo filter inside each group, and never lists one excluded repository; both rules are data in `foreman-issues/groups.js`, held by `groups.test.mjs`.
- KNOWN-LIMITATIONS L29: `hw next` and the outbox flush work on hc; `hw done`/`hw reap` still do not.
- The Herdr app's right column switches between Cockpit, Issues and Artifacts from a row of three labels (a layout `stack` plus `layout.select`), so the issues and artifacts panels no longer need a shell command. The file viewer and palette entries stay blocked by app limits (no mod commands in the palette, no file-open hook), stated in `herdr-app/README.md`.
- The Herdr app's default look is now `c-mono-signal` (pure black and white, Geist Mono, one cyan for what waits on the user), shipped as `herdr-app/themes/c-mono-signal.json` and chosen by the layout. The previous look is the built-in `graphite` theme; switching is the app's own `app.theme` from the palette, persisted, with no code.
- Herdr app panels, v2: an artifact now shows as it renders (Chrome headless, with a mock keychain and a throwaway profile, draws the page to a cached PNG shown scrollable at the panel's width); the issue preview, an artifact's text answer and the file viewer draw markdown through one renderer (headings, lists, code, quotes, comment blocks, GitHub-colored label chips, a state dot) in the active theme's tokens.
- The Herdr app's foreman layout follows Zed's grammar: spaces and agents on the
  left, terminals in the center, the cockpit in the right column, the status bar
  at the bottom.
- The four foreman mods (cockpit, issues, artifacts, viewer) share one look from
  an identical `tokens.js`: one type scale, hairline separators instead of
  bordered cards, status as a colored dot with neutral text.
- The Herdr app's foreman looks hold WCAG AA and one "waits on you" color. The cockpit paints an ask and a report with the theme's `done` (the app's own "waits on you" status) instead of the terminal's `ansi3` and the focus `accent`: look C draws both in its signal cyan (they were yellow and white, white like working), look G draws an ask blue instead of a yellow that matched working. Theme values: C `dim` `#7a7a7a`, `done` `#00e5ff`; D `dim` `#80848c`, `done` `#7c86e8`; G `chrome` `#2f343e` (Zed's dock color, where the app paints its sidebars and the cockpit), `dim` `#9a9eab`, `blocked` `#ea858b`. `looks.test.mjs` now holds AA for every look's text on `chrome` and `background` and keeps `done` apart from `working`.
- Each Herdr app look is now a whole window, as its approved mockup draws it, not only a palette: `herdr-app/layouts/foreman.json` (C), `foreman-d.json` (D, Linear) and `foreman-g.json` (G, Zed) bring their theme, fonts (C Geist Mono; D system UI + JetBrains Mono; G IBM Plex Sans + Lilex), region sizes and paint, and replace the app's own sidebar, tab bar, pane header and status bar with foreman parts drawn per look (`mods/foreman-cockpit/chrome.js`, `props.part`/`props.look`). The cockpit, Issues and Artifacts draw each look's rows, status glyphs (■/●/○, ◐/●/○/✕), sections and callout. Switch from Settings (each layout has a preview) or `ctrl-shift-l`. Theme changes: D `surface` `#1a1b1f`, G `chrome` back to `#3b414d` and `done` `#7fb6ec`. New `~/.config/herdr-app/layouts` symlinks: `foreman-d.json`, `foreman-g.json` and the three `foreman*.png` previews.
- A plain herdr TUI pane is unaffected: with no `HC_TERMINAL_ID`, the Stop hook and the invokers take today's path,
  byte for byte.
- `docs/pi-vendor.md` and `setup/export/overlay/docs/KNOWN-LIMITATIONS.md` (L3b): the specialists row names the plugin
  agents, and the "plugin skills are not passed in a product lane" line is corrected — the plugins a brief declares ARE
  passed (`-e`, skills included), only the user-level skills are not.
- `setup/tests/1273-pi-ve-los-agentes-de-los-plugins.sh`: the dry run names the agents, the adapter registers them
  (hermetic), and a LIVE pi session (real pi, real pi-subagents, a local scripted provider — no quota) runs
  `pstack:comment-sicko` with the registration and reproduces `Unknown agent` without it; the same session asks pi for its
  loaded skills (`get_commands`), so the plugin skills are proved reachable too.
- `setup/pi/test-hw-executor.mjs` mode `plugin-agents`, `core/t6_pi_test.go` `TestPiPluginAgentDirs`,
  `core/invokers/pi_transcript_test.go` (a plugin agent through `subagent`), and the closing's golden in
  `core/t5_review_plugins_test.go`.
- **Review children get the diff as a file, on every vendor.** pi-subagents' `reviewer` declares `tools: read, grep, find, ls`
  (no shell, by design — the file belongs to the npm package), so 3 of 4 review children of herdr-app-artifacts could not run
  `git diff`. Both closings now say: write the range's diff to `$HW_ARTIFACTS/review-range.diff` and give that path to every
  review subagent and judge. The agent sentence also says to pass `async:false` where the call has that field (both judges:
  the pi proof needs it explicit). A declared plugin whose agents directory path has a space or `:` is now said at dispatch
  (`NOT LAUNCHABLE here … has a space or ':'`) instead of dropped silently. Prompt-size budgets moved for the clause.
- A product lane that declares `vendor: claude` with an `account` and no `model` now dispatches Claude Code on that account (`CLAUDE_CONFIG_DIR`), with the model chosen by the default and the `kind:` rules. One lane moved from opencode to this; `--agent opencode` still works there, and live executors keep their vendor.
- `setup/heavy` serves a queued product-lane job (anything whose lane is not `setup`) before any queued
  setup-lane job, whatever their arrival order; FIFO inside a class. The lane is `HW_HEAVY_CLASS` — a caller's own
  declaration, cooperative and unverified — else `HW_PROJECT`, else a shell standing in the setup lane's own brain checkout
  (`HW_BRAIN_ROOT`, which is all a brainer's shell has), else the `setup-` tag prefix; an unknown lane counts as product.
- Setup-lane jobs together hold at most `HW_HEAVY_SETUP_MAX` slots (default: slots minus 1, minimum 1). A budget on setup,
  not a promise to product: the cap and the slot scan both use the caller's own `HW_HEAVY_SLOTS`, so a caller asking for fewer
  slots cannot reach one setup leaves free, slot 2 needs 20% free memory, and a product waiter takes whichever slot frees first.
- A setup job queued longer than `HW_HEAVY_SETUP_STARVE_MIN` minutes (default 60) is served ahead of product jobs, inside the
  reserve.
- A brief's `## Testing / Verification` fence pins only the touched subjects and partitions, minutes long, and not the full
  fast gate: `hw done` re-runs the pinned line under a 900 s ceiling, and the gate takes longer than that. The full gate is
  still the executor's last run, written once in `setup/CLAUDE.md`.
- `docs/BRIEF-TEMPLATE.md` tells executors to run `setup/export/check` before committing and to use `setup/mutation/safe-mutate` for old/new proofs.
- The cockpit card shows no model (instead of the sentence `n/a — hw has no model reader for vendor …`) for a vendor hw cannot read a model from.
- `setup/commit-tree`'s export refusal for a new or deleted file now says to edit `setup/export/manifest`, name it in the same call, and mentions `setup/export/check --tracked FILE`.
- `docs/BRIEF-TEMPLATE.md`: an "After merge (brainer)" section for installs that point at the main checkout.
- A queued ruling reaches a working claude executor between two tool calls: `bin/hw-stop-hook.sh post-tool` (a `PostToolUse`
  hook, merged into `~/.claude/settings.json` by `install.sh`) claims the queue and answers `additionalContext`, in order, once
  (`ruling-delivered.<n>` records `via=post-tool`). The turn-end and done-invoker routes are unchanged, and `channel-send --ruling`
  still refuses a working receiver. `hw ruling --help` and `CLAUDE.shared.md` say so.
- The in-process Go partitions of a fast gate run half the cpus at once (7 of 14, two cpus each), not a third,
  and start longest first. A partition measured 0.5-0.7 cpu of wall, so four at once left most of the machine idle.
- The suite's time budget gate estimates what actually runs. Retired subjects no longer count, and the Go
  partitions do. The SUITE ceiling now bounds the worst-case fast gate (a diff that reaches core/, every partition
  queued), not the full suite.
- A green Go partition's line names its wall: `partition <name> green (...) in <N>s`.
- `go-test-core.sh --affected` names, on stderr, each changed path that queues every partition, and why:
  the deps file does not list it, or every in-process test reads it.
- The full-suite budget estimate is back under its ceiling: 863 (`las-anclas-de-revive`) was budgeted at a flat 600 s and measures 178.69 s, so the estimate falls 2933 s -> 2862 s and the SUITE ceiling is lowered 2931 -> 2870.

### Fixed
- `setup/tests/510`: three mutant needles no longer matched the selector or the runner, so the subject died before its
  last mutants ran.
- `cockpit-state` publishes `model: null` when hw could not read the model (the receipt's `model_running` of `none — <why>`); the diagnostic stays in `hw receipt` instead of being drawn as the card's model.
- `setup/heavy` no longer reclaims, as "had reported", a job that started after its run's report:
  an executor reopened by a hand-written `channel-send` keeps its gate instead of being killed (143).
- `hw done`'s pinned verification is killed whole at `HW_VERIFY_TIMEOUT_SECONDS`: the process group and
  every descendant (heavy's jobs sit in a group of their own, which `timeout` never reached), exit 124.
- The pre-commit hook's closing list of red lines ("the red lines of the gate, again") now prints when the gate's output is larger than the pipe buffer (64 KB); a `grep -q` pipe under `pipefail` swallowed it.
- The fast gate queues the Go test job (`go test` over `core/`) before the bash subjects, so a red Go test is reported in seconds instead of after the whole subject sweep.
- The Herdr app's looks C, D and G work again: each layout (`herdr-app/layouts/foreman.json`, `foreman-d.json`, `foreman-g.json`) draws the window with the app's own mods (`workspace-chrome`, `spaces`, `agents`, `tabs`, `pane`, `workspace-status`), styled by the look's theme, fonts, region sizes and paint. The premium pass's drawn stand-ins (`foreman-cockpit/chrome.js`, `props.part`) are gone: they showed empty Spaces/Agents headings, no tab bar, no space and a stub status bar. `looks.test.mjs` now fails any layout that replaces those mods. Proved in a second, isolated Herdr app (own home, app socket and core): switch/create/rename/close a space, open/switch/close a tab, agent status and click-to-focus, status bar, Cockpit/Issues/Artifacts, 19/19 per look with the look switcher below.
- Cockpit rows have no hover: the app's tooltip drew the whole report as an unstyled bar across the window. A report and what waits wrap under the row instead (`hover.test.mjs`).
- A row with no model reads clean: hw's `null` and its `n/a — hw has no model reader` are not drawn as a model; the row shows `—` (D, G) or leaves the field out (C) (`state.test.mjs`).
- `channel-send hc`'s idle gate ignored `hc wait`'s exit code, so a failing `hc wait` was re-run in a hot loop until the budget ended; it now backs off a second and lets `hc get` decide.
- The cockpit no longer repeats a task's name as its title (`Brief: <task>`),
  draws an unknown effort as `—`, or prints an empty `report:` line.
- `hw` on the herdr-core route no longer looks up, or focuses, a herdr tab or space that happens to carry the task's label.
- `cockpit-state --print-path` inside an executor (HW_TASK set) names the brainer's file from `HW_INVOKER_PANE`, not the executor's own pane, and exits 1 with a stderr line when the file does not exist (it used to print a path that never exists and exit 0).
- `bin/gh-attach`: when `hw secret check` cannot run (hw-core not built) it names the build step (`setup/build-core --native`)
  instead of calling the cookie item missing; "missing" is said only when hw reports the item is not in the folder. Its store
  hint now says to type the command first and copy the cookie last, so the clipboard still holds the cookie when it runs.
- hw's two `authorizes:` refusals teach the rules: the Keychain-handle refusal says a Bitwarden item belongs in `secrets:`,
  the "more than three fields" refusal says one `authorizes:` line per entry. `docs/BRIEF-TEMPLATE.md` documents
  `authorizes:` and `secrets:` in its frontmatter block.
- `docs/BRIEF-TEMPLATE.md` gains a `## Limits` section (after `## Out of scope`) that holds the smallest-change line, the
  heading the close prompt already calls "the brief's Limits"; `setup/CLAUDE.md` points at it.
- **pi runs the plugin reviews its brief declares, by the same names a Claude executor uses.** hw names each declared
  plugin's `agents/` directory (`HW_PI_PLUGIN_AGENT_DIRS`, `<plugin>=<dir>`) and `setup/pi/hw-executor.ts` registers every
  file in it with pi-subagents as `<plugin>:<name>`; `subagent` with `pstack:comment-sicko` now resolves instead of
  answering `Unknown agent: pstack:comment-sicko` (measured live, 2026-11-09: three pi executors in a row improvised
  around it — a generic reviewer, an inline review, a `review-skip:`). A plugin's agents are not filtered by hw's
  framework prefixes, and keep the prefix: the bare `comment-sicko` is not registered.
- **`done-invoker` proves that run like a Claude one.** The proof was already there for a pi `subagent` call that ran and
  waited (`core/invokers/transcript_pi.go`); what was missing was a name it could ever see. A `review-skip:` is no longer
  the honest exit for a plugin agent pi can run.
- **The dispatch says when a declared agent piece has no door.** Without pi-subagents the `pi specialists` line adds
  `NOT LAUNCHABLE here: the review's agent pstack:comment-sicko needs pi's subagent tool…` instead of silently listing
  nothing. With it, the line names each plugin agent and its directory.
- **The review-agent sentence asks for the foreground, on every vendor**: `Launch the review agent … in the foreground —
  no background launch (run_in_background, async:true): a background call proves the launch, not the run`. Measured
  2026-11-09 (herdr-app-viewer): a `run_in_background:true` call was refused by the closing, costing 8.5 min and two
  more cycles.
- `hw … --agent pi` with a brief that declares plugins opens on the executor instead of on pi's inventory. The private
  agent directory such a launch already gets now carries `quietStartup: true`, so pi stops printing `[Context]`,
  `[Skills]` (every one of the ~68 skills thermo, ponytail and pstack ship), `[Prompt conflicts]` and `[Extensions]` on
  the first screen; pi's own diagnostics still print when pi is quiet (`showDiagnosticsWhenQuiet`), so a skill pi had to
  skip is still named. It is a settings field, not a flag: pi has no `--quiet`.
- A plugin whose skill an earlier declared plugin already ships travels as a per-run mirror of itself with that one
  `skills/` entry left out (`$HW_PI_SKILLS_ROOT/pi-plugins/<plugin>-<hash>`, links into the installed plugin). `thermo`
  and `pstack` both ship `thermo-nuclear-code-quality-review`; pi kept the first copy and printed a `[Skill warnings]`
  block for the second, which was the whole first screen. Nothing vendored changed, and pstack keeps its own copy for a
  launch that declares it alone.
- `PONYTAIL_QUIET_STARTUP=1` is exported to a pi launch that declares `ponytail`, which stops its extension's
  `Ponytail loaded: <mode>` toast; its statusline keeps naming the mode. No pi setting reaches that line.
- A pi executor whose provider quota is spent now reports the task blocked to its brainer: the adapter runs
  `done-invoker --blocked` once, naming the quota, the branch HEAD sha and the `checks.md` counts. Before, the run ended
  with only a `provider_quota` receipt row and the brainer found the dead pane by reading it.
- `deny_setup_brain_writes.py` no longer refuses `python3 - <<EOF … EOF` followed by brain's `bin/done-invoker` or
  `ask-invoker` run by path, which its own docstring allows. A script that merely names them as an operand is still refused.
- `ask-invoker` and `done-invoker` send nothing, and say why, when an ancestor process is a headless agent (`claude -p/--print`, `pi -p/--print`, `opencode run`): a probe nested in a run inherited its `HW_RUN`/pane and spent the real run's asks. An interactive agent among the ancestors is a real executor and is not refused.
- The setup executor's brain write guard judges each `;`/`&&`/`||`/newline piece: `mkdir -p .artifacts; cat <brain>/…` is no longer denied because of the read, while a mutator that writes brain, `cd <brain> && rm …`, pipelines into `xargs`, variables and interpreters stay denied.
- The headless-Chrome guard no longer lets `chrome --he''adless …` or `$(which chrome) --headless …` through without `--use-mock-keychain` and `--user-data-dir`.
- `hw done` on a run whose own `done-invoker` already closed it says so and verifies nothing, instead of alarming about uncommitted work.
- The test suite exports a short `HW_ENGRAM_HEALTH_TIMEOUT`, so subjects that call `ask-invoker` no longer wait 10 s per try.
- The app cockpit row no longer ends in `· unknown` when the agent status is unknown.
- A test fails when a `herdr-app/layouts/*.json` leaves out one of the app's 6 built-in mods without an allowlisted reason.
- The repo-write guard no longer refuses a read-only command (`sed -n` of a file reached through a symlink into a protected tree) just because a `cp` elsewhere in the same command line does not touch that tree; a write through the symlink is still refused.
- A ruling consumed by `done-invoker` is no longer lost to a truncated read: its text stays in the run dir as `ruling-text.<n>`
  (turn end and done-invoker both), and the held report names that path on its first AND last line, so `done-invoker … | tail -1`
  still finds it.
- Seven core files had no line in `setup/suite-data/go-test-deps.txt`, which turned a check of that file red.
  They are now listed as read by every partition, the conservative choice until they can be measured.
- The suite budget estimate (2870 s) counted 60 retired subjects (10344 s) that no gate runs, and none of the
  Go partitions that every gate reaching core/ runs.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.7.3

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
