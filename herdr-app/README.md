# herdr-app — the foreman cockpit and its layout, as app files

Two files for [the Herdr app](https://herdr.dev) (`~/.config/herdr-app`, guide in
its `AGENTS.md`): a mod that draws the foreman cockpit, and a personal layout
laid out like Zed: spaces and agents on the left, the terminals in the center,
the cockpit on the right and the status bar at the bottom.

The cockpit's data is the file `bin/cockpit-state` writes, under the contract in
[`cockpit/schema.json`](../cockpit/schema.json) — the same file the Claude Code
cockpit reads. Nothing here writes it, and nothing here changes it.

The mod cannot guess where that file lives, so its block says it:
`props.dir`, the work root's `.cockpit` folder. `bin/cockpit-state --print-path`
prints one writer's file inside it.

## What it shows

One section per brainer whose state file is live — named by its lane, sections
ordered by lane name — and hw's own cards inside it, in hw's order: the task and its title, its status and how long it has held
it, the vendor, model and effort with the context percentage, the commits, the
suite running for it and the children it waits on, its subagents, and — the line
that matters — what waits on the brainer: a challenge, an ask, a blocked report,
a queued ruling or no progress for longer than the writer's threshold.

Above the sections, one **queue** of what waits on the person (reports, asks, challenges, blocked), oldest first;
a click on an entry focuses that executor's pane (`pane.focus`, when the app's core holds it). Each row carries a
**context meter** (`ctx.pct`): quiet below 60 %, the warning color from 80 %, the alarm from 90 %, the percent written
once it warns; no meter when the window is unknown. A **filter** (all / needs you / working / done) narrows every
brainer's rows and is kept in the app's shared state (`foreman.cockpit.filter`, persisted in its `shared.json`). The helpers
(`waitingQueue`, `ctxLevel`, `keeps`) are in `state.js`, held by `queue.test.mjs`.

A file older than 15 s is dimmed and marked `stale`; older than 60 s, or
unreadable, or naming another schema, it is drawn as `NO STATE`. Rows are never
kept from a file that has gone bad.

**Buttons.** A queue entry carries what the person can do about it, without typing in a brainer's pane:

- an **ask** row: **Reply** (a text field and *Send*);
- a **challenge** row: **Rule** (the same), and a **blocked** row where hw says a ruling would queue: **Rule** too (the queue lists only what waits on the person, so a working row has none);
- a **reported** row: **Done**, which asks `Close <project>/<task>?` and runs only on *Confirm done*.

Each button runs exactly the argv `cockpit/verbs.txt` gives for its verb (`done`, `ruling`, `reply`, `challenge-reply`),
read from that file at run time (`props.verbs`, its path, in the layout's cockpit block): the mod keeps no second copy of
the table, and a verb the file does not list is not callable (`actions.js`, held by `actions.test.mjs`). A block with no
`props.verbs` has no buttons; one that names a file that is not the contract says `buttons off`.

**How the buttons find `hw` and `channel-send`** (2026-11-12, app-dispatch-path-and-space-drag). The app is a GUI process: its `exec` resolves a bare
name on the PATH of the login shell it starts, and the brain's `bin/` is put on that PATH by `.zshrc` (interactive shells only), so `hw` was not there
(measured: `zsh -l -c 'command -v hw'` prints nothing, and every button died with «`herdr.exec`: cannot run hw: No such file or directory»). The mod now runs
`<bin>/hw` and `<bin>/channel-send`, where `<bin>` is the `bin/` beside the `cockpit/` folder of `props.verbs` (`binDirOf` in `actions.js`: `<brain>/cockpit/verbs.txt`
→ `<brain>/bin`). `verbs.txt` is unchanged (its argv still starts with the bare word) and neither the app's nor macOS's environment is touched; a `props.verbs` that
is not a `…/cockpit/verbs.txt` path falls back to the bare name. Held by `actions.test.mjs` (a real exec under `/usr/bin:/bin`, every verb of `verbs.txt`).
There is no Dispatch control (removed 2026-11-12, it was never a v1 verb): a brief is dispatched from its brainer with `hw`. The Agents panel draws each executor's model beside
its row from the Cockpit's `model_running` (`shared("foreman.cockpit.models")`, `foreman-agents/model.js`).

**Under the app's core the route is `hc`** (2026-11-11, app-live-qa-devserver): there a brainer and an executor are core terminals, not herdr
panes, so the state file's `ask.route` says `hc` and the reply verbs run `channel-send … hc <terminal> - <text>` (`<route>` in
`verbs.txt`; `herdr` when an older writer publishes none). Before, an ask raised under the app got no button.

**The text goes on argv, not stdin** (2026-11-11). The contract's `stdin=text` verbs read the text from stdin in the Claude
cockpit mod, but the app's `exec` has no stdin (`herdr.d.ts`: `{ cwd, on, timeout }`) and runs no shell. So the app puts the text as
ONE argv word in place of the template's last `-`; with no shell, backticks, `$` and newlines arrive intact (the Issues handoff
does the same). A text that begins with `-` is refused for `hw ruling` (it reads the word as an option: "unknown option"), and a
lone `-` for `channel-send` (it means "read stdin"); the row says so and nothing is rewritten.

**Outcomes.** A non-zero exit is shown as the command said it (its output, scrubbed of control characters) and the row stays,
with its buttons. A send cut off in flight, or channel-send's exit 5, is `unknown` (`thrown`, from `foreman-issues/handoff.js`; the
Cockpit holds a byte-identical `thrown.js`, test 1277): the row waits and offers *Dismiss*, never a blind retry. Exit 0 is *sent*:
the row is cleared only when the next state read shows it changed (another attention, another ask, or the row gone).

There is no merge button, and `verify` and `receipt` stay out of the app: `verify` pre-fills a prompt and starts no verb, and a
person reads `receipt` in a shell. Dispatching is the Issues panel's *Hand to brainer* (`foreman-issues/handoff.js`).

## Reaching each panel

| panel | how, from the app | how, from a shell |
| --- | --- | --- |
| Cockpit | the right column, `Cockpit` label (shown first) | — |
| Issues | the right column, `Issues` label | `herdr-app do tab.new --mod foreman-issues` |
| Ports | the right column, `Ports` label | `herdr-app do tab.new --mod foreman-ports --props '{"dir":"~/<work>/.cockpit"}'` |
| History | the right column, `History` label | `herdr-app do tab.new --mod foreman-history` (`--props '{"open":"<lane>/<task>"}'` opens a run) |
| File viewer | **no route inside the app** (below) | `herdr-app do pane.split:right --mod foreman-viewer --props '{"file":"/abs/path"}'` |

The right column is a `stack` (`right-body`) under a row of five labels
(`right-tabs`, the cockpit mod in its switch mode: `props.switch`; five labels since History); a label runs the
app's own `layout.select {stack, child}`. The app keeps the choice across restarts. The
underline follows the last label clicked, else the layout tree's `active` child; a stack
changed some other way (`herdr-app do layout.select …`) can leave it on the wrong label
until the next click (`layout()` is documented as the layout file, so this is unproven).

**Ports** (`foreman-ports`) lists every live executor of every lane (the cockpit state files, `props.dir`) with
its ports from the run's own `.hw/<run>/env` (`HW_PORT_WEB`, `HW_PORT_API`, `HW_AGENT_ENDPOINT`): a bright dot when
something listens there (`lsof -iTCP -sTCP:LISTEN`, read-only; listening, not an HTTP answer), a dim one when not, and the CPU
and memory of the pane's process tree plus the trees of the listeners on its ports (`ps`, each process paid once). Above
them the machine: load1 and live executors against the load cap (`CAP` in `model.js`: 10 executors, load1 40; a figure
over its cap gets the blocked dot) and free memory. It only reads; a click focuses the executor's terminal. The pure half
(`model.js`) is run by test 1272. Taken from the community mods Resources and Live ports (VladTemp27/herdr-mods, Apache-2.0): the `ps` tree sum and the `lsof -F pcn` records.

**Markdown is drawn one way** by Issues (description and comments), the file viewer and
the Files panel: `md.js` splits it into headings, lists, fenced code, quotes and
paragraphs, `mdview.js` draws each in the active theme's tokens (code on `selected`, quotes
under a hairline); inline marks and links stay with `TextView.markdown`. Issue labels are chips
in GitHub's own colors, state a dot plus text, each comment an author/date block between hairlines.
The Issues list says each fact once in every look (`foreman-issues/rows.js`): under All repos one
header per repo (name and count) and no repo on the rows, number and title in fixed columns, the state only under **Any**, one date format (`5d`, `2mo`, `1y`).

**App limits that block the rest** (measured 2026-11-09 against `herdr-app commands`,
`herdr.d.ts` and `AGENTS.md`): the command palette (cmd-shift-P) lists the app's own
commands and no mod can register one, so `Cockpit`, `Issues` and
`Open in viewer` cannot be palette entries; and the app has no file-open hook (Files
previews a file in its own panel; a terminal link is only a live terminal address). The
viewer therefore stays a shell command (`fv`, in its README).

**History** (`foreman-history`): taken from plutotw's History panel in "Herdr Main + Usage" (herdr.dev/mods) — a list of past sessions you open from the right column — and changed to read what hw records (receipts under `work/` and `archive/`, plus each product lane's own worktree root read from `projects.json` as hw resolves it — `worktree_root` with `{checkout}` from `checkout_var` else `checkout` — since every lane with its own checkout keeps its `.hw` in `<checkout>/.worktrees`; commits from git or the `archive/task/*` tags) instead of the vendors' session stores. A run still open in a pane is not listed. A product worktree that `hw done` or `hw reap` removed takes its `.hw` with it (only `.artifacts` and a few report folders are archived, `core/reap.go`), so its run is gone from every source and cannot be listed. The right column's switch bar is two rows (labels; then the live count and `C D G`), so five labels never squeeze either out, in any look, down to a 300 px column (`SWITCH_ROW1/2` in `switcher.js`, held by `switcher.test.mjs`).

## The Agents list tells a brainer from an executor

`mods/foreman-agents/` is a clone of the app's `agents` mod (the app replaces its shipped mods whole on every start, so a
change to the list is a clone, ours to keep in step with the app's updates; the three layouts place it in the `agents` block,
with `props.look`). It keeps every behaviour of the original — groups by state or space, order and `setAgentOrder`, clicks, pins,
the fold and the sub-agent tree — and changes one thing: **a row's icon says the role**, read from the terminal's labels
(`role.js`), never from a title:

| terminal | icon |
| --- | --- |
| an executor: carries `hw_task` (with `hw_run`, `hw_project`) and no `hw_role` | a hard hat (`hard-hat.svg`) |
| a brainer: named `brain-<lane>` and carries no `hw_task` | a brain (`brain.svg`) |
| anything else (a lane's dev server carries `hw_task` plus an `hw_role`) | the app's own program icon, unchanged |

Both SVGs are 24-unit line icons in `currentColor`, drawn at 16 px with the row's text color like the shipped icons. The rule is
`role.test.mjs`, run by test 1272; it fails the mutants that read `hw_run`, the name before the label, a prefix instead of
`brain-<lane>`, a title, or drop the dev-server exception. Sub-agent rows keep their program icon.

## Files shows a file the way the viewer does

`mods/foreman-files/` is a clone of the app's `files` mod (`herdr-app do mod.clone`, app 0.1.15; `mod.json` records
`cloned_from`), placed by the three layouts in the `files` block with `props.look`. The app's own Files put every file in a
read-only editor, so markdown showed as raw text (`|` tables, `**`) and long lines ran off the panel's edge. The clone keeps
the tree as the app draws it (pixel for pixel: the lazy folders, the git marks, the back arrow, the 500-per-folder and 5,000
limits, the 10-second and folder-watch refresh, remote machines) and changes what an opened file looks like:

| file | drawn as |
| --- | --- |
| markdown (`.md`, `.markdown`, `.mdown`) | rendered: headings, bold, tables, lists, quotes, fenced code (`mdview.js`), scrolling |
| code and plain text | the read-only editor with the theme's syntax colors and line numbers; **long lines wrap** |
| an image (`png jpg jpeg gif webp bmp ico svg`) on this machine | drawn, scaled to the panel |
| a file with a NUL byte | `Binary file · no preview` |
| a file that is gone, or over the 1 MiB the app reads | one line saying so |

**One renderer, two mods.** `filekind.js` says what a file is and `fileview.js` draws it; `foreman-viewer` and
`foreman-files` carry identical copies (the app refuses an import from outside a mod's folder) and neither `main.js` makes an
editor or markdown of its own: test 1277 fails on a differing copy or a second renderer. The viewer pane looks as it did
(its shots before and after are pixel-identical); it gained `.svelte`/`.vue` highlighting from the app's Files table, and a
file literally named `png` is no longer taken for an image. Only Files wraps long lines (`openFile(…, { wrap: true })`).

**Images.** The app draws an image only from inside its own folder (`image()`, `gpui-kit.d.ts`), so Files copies a local image
into `~/.config/herdr-app/cache/foreman-files/<crc>-<bytes>.<ext>` (`picture.js`: POSIX `cksum`, so an edited image is a new
picture and an unchanged one is never copied twice) and draws the copy. Images over
20 MiB are not copied; a copy not opened for 30 days is removed when another image is copied. An image that cannot be drawn —
on another machine, too large, no `sh` — is shown as the app's own Files shows it: an SVG as its source, anything else as a
binary file. The viewer pane is unchanged: it still draws images only from inside the app's folder.

**`props.open`** (a path inside the folder) names a file the panel shows first: nothing else opens a file without a click, and
it is how the proof shots were taken. It is opened one turn after `init`: on 0.1.15 a `readTextFile` started inside the `init`
of a block the app had just started over (new props) answered twice and then never again (13 of 13 logged runs), while one
started a turn later answered in 20 of 20; the cause is not established. `fileview.test.mjs` (run by test 1272) holds the dispatch by kind in the shared module
and through the panel's own `show()`, the image copy against a real `sh`, and `props.open`; 24 mutants of those go red.

Not changed: the header (`Files`, the back arrow, the path) and the tree. Three one-line notes now read as the viewer's do
(a missing file, a binary file, `… (truncated)` after 400 KB). A clone is ours to keep in step with the app's updates: `mod.json` names the version it was cloned from.

## Opening a path printed in a terminal

**The key: `cmd-shift-f`** (`"cmd-shift-f": "mod.open:foreman-paths"` in the `global` section of `~/.config/herdr-app/keys.json`; free in the
app's 0.1.15 defaults, where `cmd-f` is the terminal's search). It opens `mods/foreman-paths/`, a picker of the file paths on the focused
terminal's screen and the 200 lines above it, newest first: `↑`/`↓` and `Enter`, or a click; `Esc` leaves. The chosen file opens in the
Files panel, drawn as Files draws any file; a hidden panel is shown first. It is the app's counterpart of the herdr TUI's `prefix+f`
(ide-view). The app gives a mod no hook for a click on a terminal link (measured, 0.1.15), so a click on a `file://` link cannot be made to
open Files: this key is the way.

**What it reaches.** `hc read` returns the text a terminal shows, never an OSC 8 link's destination (measured, app 0.1.15;
herdrdev/herdr-app#590), so a link is found by what it prints:

| on the screen | reached |
| --- | --- |
| a `file://` URL written out, an absolute path, `~/…` | yes |
| a path relative to the terminal's cwd, or to its git root | yes |
| `path:12`, `path:12:5`, `path#L12`, `@path`, a diff's `a/path`, `key=path` | yes (the line is shown, not jumped to) |
| a link that prints only a name (`[CLAUDE.md](file:///abs/CLAUDE.md)` shows `CLAUDE.md`), the file in the cwd or its git root | yes |
| the same, the file elsewhere, and its full path also on the screen (a tool header: `Read(/abs/CLAUDE.md)`) | yes: the newest path with that name |
| the same, and no path on the screen ends in that name | no |
| a path the terminal wrapped onto two lines | no |
| `http(s)://` | no: the app opens those |

**Errors are said.** No path on the screen: `No file paths on this terminal's screen`. No terminal has the keyboard: the picker says there is
no screen to read. A `file://` URL whose file is gone is still listed, marked `missing`, and choosing it shows `No such file: <path>` in Files;
any other text that names no file is not listed.

**How.** `paths.js` (pure, held by `paths.test.mjs`) reads the tokens, the disk is asked once which of them are files (`sh`, on the terminal's
machine), and the choice goes to Files as `shared("foreman.files.open") = {path, at}`: Files takes it when it starts (a hidden block is not
loaded), when it draws, and on `foreman-files.open`, once and only while fresh (15 s), so the value the app persists never reopens a file at
the next start. Without keys: `herdr-app do mod.open --name foreman-paths`, then `foreman-paths.next`, `.prev`, `.open`, `.close`.

## Claude usage

The top of the Cockpit shows one line per Claude account: `default 5h 83% 7d 41%`, amber from 80 %,
red from 95 % (look C draws red as its one signal color), dimmed with `40m ago` when the file is older
than 15 min; a window whose reset has passed shows `—`. The source is Claude Code's own status
line: its stdin carries `rate_limits` (5-hour and 7-day `used_percentage`, `resets_at`).
`bin/statusline-usage-tee -- <the real status line command>` keeps only `rate_limits`, model and cwd in
`~/.cache/hw/claude-usage/<account>.json` (account = `CLAUDE_CONFIG_DIR`'s name; `~/.claude` is
`default`) and runs the real command with the same stdin, so its output is byte-identical. Each
account's `settings.json` `statusLine.command` runs the wrapper from the brain's `bin/` if it is
executable, else the real command alone (copies of the previous files: `settings.json.bak-quota-tee`).
The file is rewritten when a session refreshes its status line, so it lags `/usage` by one turn:
measured 2026-11-10 in a live session, the file said 12 %/24 % where `/usage` said 13 %/24 % one
request later, and the reset times matched. Ruled out: Anthropic's OAuth usage endpoint and the
Keychain token (community mod `agent-limits`, by mchlkim, reads them). Taken from it: the optional
status line wrapper and one line per agent.

## Looks

A look is a **layout plus its theme**: `layouts/foreman.json` is C, `layouts/foreman-d.json` D and
`layouts/foreman-g.json` G. Each draws the window with **the app's own working mods** —
`workspace-chrome` (brand and footer), `spaces`, `tabs`, `pane` (G's pane header) and
`workspace-status` — and, for the Agents list and the Files panel, `foreman-agents` and `foreman-files` (clones of the app's `agents` and `files`, each in its own section above). It styles them only through what the app gives a layout: the theme's colors,
the fonts, region sizes, and paint (`background`, `border`, `radius`, `padding`, `gap`, `divider`,
the pane frame). The built-in mods take no style props, and only the root's look applies
(`AGENTS.md`, "Today only the root's look applies"). The right column is the foreman cockpit,
Issues, Ports and History, each block naming its look (`props.look`).

**Switching looks:** every look shows `C D G` at the right end of the right column's switch bar,
the shown look marked (C: inverted; D and G: on `selected`); a click runs `layout.switch {name}`.
`cmd-shift-l` (the app's own key) still cycles every layout, including the app's. Also Settings
(`cmd-,`), Look; or `herdr-app do layout.switch '{"name":"foreman-g"}'`.

| layout | theme | the window (approved mockup) |
| --- | --- | --- |
| `foreman` (C, default) | `c-mono-signal` | C — one signal (`archive/setup/herdr-app-bold-look/mockups/c-mono-signal.html`): `#000`/`#fff` in Geist Mono, flush regions on one `#1f1f1f` line, pane frames as 1 px white-focused cards; one color, cyan `#00e5ff`, for what waits on the user (the cockpit's callout, the row's bar and ■, the agents list's "Needs you", the status bar's "needs you") |
| `foreman-d` (D) | `d-linear` | D — Linear (`archive/setup/herdr-app-popular-looks/mockups/d-linear.html`): the sidebar on the `#0f1011` canvas, the work and the cockpit as inset panels (8 px margin, one `#26282d` line, radius 10); system UI font, JetBrains Mono terminal; the cockpit as Linear's grouped list under an inbox card |
| `foreman-g` (G) | `g-zed` | G — Zed One Dark (`archive/setup/herdr-app-one-more-look/mockups/g-zed.html`): docks `#2f343e`, terminal `#282c33`, status bar across the bottom on `#3b414d`, flush regions split by `#464b57`; IBM Plex Sans UI, Lilex terminal; the app's `pane` header as the breadcrumb; the cockpit as tree sections |
| `graphite` | — | the app's built-in theme: the previous look, no file here |

The rule every look keeps: what waits on the user (an ask or a report: `done`; a challenge or a
blocked report: `blocked`) is the loudest thing on screen, and never the color of `working`.
`looks.test.mjs` holds it (WCAG AA for the text colors on `background`, the look's dock and
`chrome`), and fails any layout whose brand, spaces, footer, tab bar or status bar is not
the app's own mod, whose Agents list is not `foreman-agents`, whose Files panel is not `foreman-files`, or whose foreman block draws a window part (`props.part`): the premium pass's
drawn stand-ins showed empty Spaces and Agents, no tab bar, and a stub status bar (2026-11-09).

**Proved in the app, not a harness** (2026-11-09): a second, isolated Herdr instance — its own
`HERDR_APP_HOME`, and its own core through `XDG_STATE_HOME`/`XDG_RUNTIME_DIR`, both sockets
checked with `lsof` before use — with this tree's mods and layouts, ran each look through: switch,
create, rename and close a space; open, switch and close a tab; agent status (stand-in agents
reporting OSC 7501) and focusing an agent's pane; the status bar; Cockpit and Issues;
the look switcher (each of C, D, G) and the `cmd-shift-l` cycle. 19/19 per look. Each item runs the command the app's mod runs on that click; the window was not
clicked or dragged (no pointer automation on the person's display), so drag is held by the layout
(`titlebar: none`, `window_drag` on the brand and the tab bar).

**One command each** (2026-11-10): `bin/herdr-isolated start /tmp/hx` builds the instance (mods, layouts, themes, the welcome flag, the clean `env -i` with the N1 fix),
proves the app's socket is under `/tmp/hx`, and writes `/tmp/hx/env` (`. /tmp/hx/env` drives it); `bin/herdr-isolated stop /tmp/hx` ends everything that instance
started — app, feeders, the detached core (its `--hc-launchd` parent) and its `hold` child — found by the instance's own socket directory, never by name, so it cannot
match the person's core under `~/.local/state/herdr-core`. `ls` lists what it would stop. Killing the app and the feeder by pid does NOT stop the core (measured 2026-11-10:
it re-parents to pid 1 and stays; two QA runs reported with five and two of them alive), and `done-invoker` now warns, naming pids and sockets, when a run reports with one alive.
`start` builds the instance from this tree's mods (never a symlink: `find <dir>/app -type l` is asserted empty), the welcome flag, the instance's own HOME and
`CLAUDE_CONFIG_DIR`, a fixture `gh` and a stand-in `claude` (below), `<dir>/q` and, with `--brain`, a brain copy; the app starts from a clean `env -i`
(the app hands every terminal it opens the environment it was started with: from an executor's shell that is its `HW_*`). Read `bin/herdr-isolated`; it is the recipe.
`bin/herdr-isolated shot <dir> out.png` takes a screenshot (see "Working on the mod"); it raises the window for the frame and puts the app that was in front back after it. `start` and `stop` work on the physical path (`/tmp/x` is `/private/tmp/x`), and `stop` also ends any process whose command line names `<dir>/…` (a `--brain` instance's cockpit writers).

### A live QA of foreman on the app is isolated all the way down

The instance above isolates the window and the core. A QA that dispatches executors or opens `brain <lane>` ALSO needs its own tree:
the live-cycle QA of 2026-11-09 ran `brain setup` against the real brain and work trees (not copies), flushed a stale `probe:probe` outbox entry into
a real setup brainer, and that brainer wrote into the real `brain/setup/decisions.md` (herdr-app-final-gaps, G1). Nothing a probe does
may reach the real brain tree, the real work root, `~/.claude*` or `~/.claude.json`:

```sh
bin/herdr-isolated start /tmp/hfg --brain    # short dir, as above; about 12 s with the hw-core build
/tmp/hfg/q hw status                         # a command in the instance's environment: its HOME, brain copy, claude stand-in, private engram
```

`start --brain` builds all of it (it used to be this recipe by hand, about 40 tool calls): `<dir>/h` is the instance's HOME and `<dir>/claude` its
`CLAUDE_CONFIG_DIR`, so a trust entry lands under `<dir>`, never in the real `~/.claude.json`; `<dir>/brain` is a copy of the tree under test (no `.git`)
with its own hw-core, whose lane table lives under `~` and so resolves under `<dir>/h`, touching nothing real; the engram is private and unreachable
(`ENGRAM_PORT=9`, never 7437); `<dir>/bin` carries a fixture `gh` and the stand-in `claude`; `<dir>/q <command…>` runs a command in that environment
(never the caller's `HW_*`/`HERDR_*`); `<dir>/h/.local/bin` links only `herdr-core`, `hc`, `herdr-app` and `herdr` (the core puts `$HOME/.local/bin` first on a
terminal's PATH, so with the person's own in reach `hc run claude` ran the REAL claude, measured 2026-11-10). The cockpit mod reads ONLY under the app's own HOME, links
resolved (`capabilities.fs.read` in `mod.json` is NOT honoured): the work root's `.cockpit` must sit under `<dir>/h`, as the layouts' `props.dir` names it (relative to `~`).

- **Auth.** A real `claude` cannot sign in under another config dir: measured 2026-11-10, `CLAUDE_CONFIG_DIR=<empty dir> claude -p --model haiku …` answers
  `Not logged in · Please run /login` (the credentials are in the keychain; never read or copy them; a COPIED config dir was measured the same way on 2026-11-09). So a probe runs the stand-in `claude`
  first on `PATH` (`<dir>/bin/claude`), and a real one that is reached anyway runs under `<dir>/claude`, not logged in, writing nothing real. The stand-in
  writes `<config>/sessions/<pid>.json`, logs its argv and environment to `<config>/stand-in.log`, answers each line it is sent, and reports its state by OSC 7501:
  `printf '\033]7501;state=working\007'` (also `idle`, `done`, `blocked`, `error`; measured 2026-11-10: `hc get` shows `status` after exactly this, and `turn` goes up when it
  leaves `working`; a record with extra `;`, `:` or `,` fields, or a `state=` after another key, registers nothing, and a plain program, not an agent, reports none).
- **Prove it before the first dispatch.** `lsof -a -U -p <app pid>` for both sockets against the person's own inodes, `hc ls` through
  the same env showing only your terminals, and `$I/brain/projects.json` naming no path of the real brain or work trees.
- **A product-lane (dev-server) cycle** (2026-11-11, `setup/app-live-qa-devserver`): the instance's lane table resolves under `<dir>/h`, so the lane
  is a fixture. Make a git repo at the `website` lane's checkout under `<dir>/h` (`checkout` in the copy's `projects.json`, `~` resolved to `<dir>/h`; branches `main` and `develop`, an `origin` bare repo beside it,
  and a `scripts/worktree.sh new <branch> <base>` that runs `git worktree add .worktrees/<branch with / → ->`), put a stand-in `npx` in `<dir>/bin` that serves 200 on
  `--port N` (the lane starts `npx astro dev --port $HW_PORT_WEB`) and a `gh` that lists one issue of the repo `LANE_OF_REPO` (`foreman-issues/work.js`) maps to that lane. `hw` and `brain`
  detect the app's core from the shell's ancestry, so run them from a terminal INSIDE the app (`<dir>/q hc run --bg --name driver zsh -f`, then `hc send`), never
  from the QA's own shell. The stand-in runs a line that begins with `!` in its own environment, so `!ask-invoker …`, `!ask-invoker --challenge …` and
  `!done-invoker …` are the executor's, and `!hw website <task> --brief … --keep-pane 1h --why …` is the brainer's: without `--keep-pane` the report closes the
  pane and its dev server before **Done** exists to be pressed. An executor's `done-invoker` needs `$HW_ARTIFACTS/checks.md`. Ports reads `.hw/<run>/env`; History
  and Files read the run's worktree.
- **A terminal `hc run` starts is not an agent the app lists until it has a pane.** `hc run` makes a running terminal with no space and no pane, so the
  mods' `agents()` does not return it (measured 2026-11-11: an `hc run` stand-in was invisible to the Agents list). The recipe, through the instance's env
  (`$I/q`, never a bare command): `$I/q herdr-app do space.new '{"name":"qa","folder":"<dir>/h"}'` if the window has no space yet, start the terminal with
  `$I/q hc run …`, read its id from `$I/q hc ls`, then `$I/q herdr-app do terminal.keep '{"terminal":"<id>"}'` (the command's own text: "give a running terminal
  without a pane a pane"; `"place":"right"` or `"down"` splits instead of opening a tab; arguments are `herdr-app commands`'). Only then does `herdr-app ls`
  show it and the Agents list draw its row. Prove the row with `herdr-isolated shot`, never by asserting it in prose.
- **Never** run a bare `hc`, `herdr-app` or `herdr` from the QA's shell: without the env they reach the person's running app.
- Close every process of the instance with `bin/herdr-isolated stop $I`: the core and its holder are detached from the app, so closing the app's pid leaves them running.

Drive it with the same env: `herdr-app do layout.select '{"stack":"right-body","child":"issues-body"}'`,
`herdr-app do layout.resize '{"id":"cockpit","px":816}'`, `bin/herdr-isolated shot <dir> out.png`.

**The Agents list names every row after its space** (QA5 G6): the `agents` mod is the app's own (`~/.config/herdr-app/mods/agents/main.js`,
shipped by the app; `mods/foreman-agents/main.js` is its clone and draws the same rows), and its default **priority** view draws `a.space_name` on each row (`name = full ? a.space_name || label(a) : …`);
only the **Grouped by space** sort shows the tab or terminal name. The sort menu (top of the list) switches it, and the app keeps the choice. Neither
sort is changed here: the clone only adds the role icons.

What the mockups show and the app does **not** — function won each time:

1. Sample content (task names, the terminal's text) is the mockups'; the app draws live state.
2. The sidebar, tab bar, pane header and status bar are the app's mods in the look's colors and
   fonts, not the mockups' drawings: C's inverted white space row and tab, D's brand badge and
   Inbox, G's project tree (spaces → agents with chevrons), its title bar, its tab notch and its
   status-bar toggles. Each needs a `mod.clone` of the app's mod that is then ours to keep in step
   with every app update; not done. The app's Herdr brand, Settings and Add machine stay.
3. C's mockup has no right-column switch; the switch stays (approved with the panels).
4. Issues folds it into the search row (`updated`, Refresh, and the count only when a filter narrows the list), with its filters on two rows below: Work/Personal, the scope and the state on one line that wraps a whole group at a time, and the repo chips on their own.
5. D's Inter is not in the app: the UI uses the system font, as the mockup itself fell back to.
6. A brainer is named by its lane (`lane` in the state file, which `bin/brain` hands the writer; an older file falls back to its rows'
   projects, then to its pane id) and the header never repeats it. A brainer with no executors is not a section: all of them share one
   line of lane names after the sections, in C, D and G alike (eight idle brainers filled the panel with eight headers).
7. No hover anywhere in the cockpit: the app's tooltip takes a string only and draws it as wide as
   its text (a report became a bar across the window). Rows wrap their report (up to 200
   characters) and what waits (up to 300) instead; the rest, and a task's brief title, are no
   longer reachable from the cockpit: a click on the row focuses the executor's pane, where the
   whole report is.
8. No model to draw — hw's `null` (none read) or its `n/a — hw has no model reader` — reads `—` in
   D and G and is left out of C's meta line, never the sentence.

## Install

Both are installed the same way: the app reads a *symlink* to this repo, so a
saved edit here is a live edit there.

```sh
# from a checkout of this repo (any path works)
ln -sfn "$PWD/herdr-app/mods/foreman-cockpit" ~/.config/herdr-app/mods/foreman-cockpit
ln -sfn "$PWD/herdr-app/mods/foreman-agents" ~/.config/herdr-app/mods/foreman-agents   # the layouts' Agents list
ln -sfn "$PWD/herdr-app/mods/foreman-files" ~/.config/herdr-app/mods/foreman-files     # the layouts' Files panel
ln -sfn "$PWD/herdr-app/mods/foreman-spaces" ~/.config/herdr-app/mods/foreman-spaces   # the layouts' Spaces list
for l in foreman foreman-d foreman-g; do   # one layout per look, and its preview picture
  ln -sfn "$PWD/herdr-app/layouts/$l.json" ~/.config/herdr-app/layouts/$l.json
  ln -sfn "$PWD/herdr-app/layouts/$l.png" ~/.config/herdr-app/layouts/$l.png
done
ln -sfn "$PWD/herdr-app/themes/c-mono-signal.json" ~/.config/herdr-app/themes/c-mono-signal.json
ln -sfn "$PWD/herdr-app/themes/d-linear.json" ~/.config/herdr-app/themes/d-linear.json
ln -sfn "$PWD/herdr-app/themes/g-zed.json" ~/.config/herdr-app/themes/g-zed.json

herdr-app check ~/.config/herdr-app/mods/foreman-cockpit   # must pass
```

Then switch to the layout: `herdr-app do layout.switch '{"name":"foreman"}'`, or
`cmd-shift-l` cycles layouts. The layout is a `layout.clone` of `main`, so
everything main has still works: spaces, tabs, panes, the Files panel.

The mod's `mod.json` says `"place": "none"`: it appears only where a layout puts
it, never docked into someone else's layout on first sight.

Uninstall: remove the symlinks. Nothing else was written outside this repo.

## Files

| file | what it is |
| --- | --- |
| `mods/foreman-cockpit/main.js` | the mod: the cockpit per look, the file watching, the right column's switch and its live count (`foreman.cockpit`) |
| `mods/foreman-agents/` (`main.js`, `role.js` + `.test.mjs`, `brain.svg`, `hard-hat.svg`) | the Agents list: the app's `agents` mod cloned, its rows' icons say brainer or executor (run by test 1272) |
| `mods/foreman-cockpit/hover.test.mjs` | the cockpit sets no tooltip (run by test 1272) |
| `mods/foreman-cockpit/state.js` | the reader of `cockpit/schema.json`: parse, classify, format |
| `mods/foreman-cockpit/switcher.js` + `.test.mjs` | the right column's switch: which stack child shows, the C/D/G look switcher's layouts, and the layout it must match (also run by test 1272) |
| `mods/foreman-cockpit/state.test.mjs` | the reader against `cockpit/fixtures/index.json`, run by `setup/tests/1272-herdr-cockpit-mod-reads-the-contract.sh` |
| `mods/foreman-issues/md.js` + `.test.mjs`, `mdview.js` | markdown as blocks and their drawing, plus GitHub label colors as chips; identical copies in the viewer and files mods (test 1277) |
| `mods/foreman-spaces/` (`main.js`, `reorder.js` + `.test.mjs`) | the Spaces list: the app's `spaces` mod cloned, its drag-to-reorder decided by a pure reducer (run by test 1272) |
| `mods/foreman-files/` (`main.js`, `picture.js`, `fileview.test.mjs`, four SVGs) | the Files panel: the app's `files` mod cloned, an opened file drawn by kind; its test is run by test 1272 |
| `mods/foreman-viewer/filekind.js`, `fileview.js` | what a file is and how it is drawn (markdown, code, image, binary, missing); identical copies in the files mod (test 1277) |
| `mods/foreman-history/` (`main.js`, `runs.js`, `timeline.js`, `fixture.mjs`, two `.test.mjs`) | the History panel: finished runs by lane from the `.hw/<run>/receipt.jsonl` files under `work/`, `archive/` and the lanes' own worktree roots (`projects.json`), a run's timeline from its vendor session file (condensed through `jq` where it lives, never copied); read-only |
| `mods/foreman-ports/model.js` + `.test.mjs`, `main.js` | the Ports panel: parsers and caps (pure, node-tested) and its view |
| `mods/foreman-*/tokens.js` | the one look the foreman mods share (the five panels, and Files for the file it opens) (type, spacing, radius, the status dot, the header bar); identical copies, held by `setup/tests/1277-herdr-mods-share-one-look.sh` |
| `themes/c-mono-signal.json`, `themes/d-linear.json`, `themes/g-zed.json` | looks C, D and G as themes; `mods/foreman-cockpit/looks.test.mjs` holds every theme against every mod |
| `layouts/foreman.json`, `foreman-d.json`, `foreman-g.json` (+ `.png` previews) | looks C, D and G as whole windows: sidebar, terminals, the right column (Cockpit, Issues, Ports, History), status bar, all the app's own mods; the cockpit body names `props.dir` |

Issues also opens as a tab or overlay; the viewer only as a pane. Dragging the dividers resizes the regions.

To put the cockpit in a layout of your own, add a block whose content is:

```json
{ "mod": "foreman-cockpit", "props": { "dir": "~/<work>/.cockpit" } }
```

## Working on the mod

- **Edit the source, never through the `~/.config` symlink.** The symlinks point
  at the main checkout of this repo, so a save through them is live on the
  person's screen at once and lands outside any branch. Work in your worktree;
  the app shows it after the merge.
- **The look lives in `tokens.js`**, one identical copy per mod (the app refuses
  an import from outside a mod's folder). Edit one, copy it to the other three;
  test 1277 fails while they differ. Status is a dot in its theme color with
  neutral text beside it; separators are hairlines in `border`; no colored
  borders, no filled pills.
- **`herdr-app check <mod folder>` must pass**, but it renders only the first
  frame: rows that arrive asynchronously are not drawn there.
- **`herdr-app shot` can return a stale frame.** It saves the last frame the
  window rendered; a window that is not frontmost may not redraw, so the PNG can
  predate your change (two shots of an isolated instance, one before and one after a
  second space was made, were byte-identical, measured 2026-11-10). Take shots of an
  isolated instance with `bin/herdr-isolated shot <dir> <out.png>`: it raises that
  instance's window by its pid, waits, shoots, and refuses a frame equal to the last.
  A window of the person's own app is never raised by it; an agent records that shot as pending.
- **Switch the layout back** with `herdr-app do layout.switch '{"name":"main"}'`
  (or `cmd-shift-l` to cycle). If a layout breaks the window,
  `ctrl-alt-shift-escape` always restores `main` and sets the shown layout aside.

## The Spaces list drags with a reducer (2026-11-12, `setup/app-dispatch-path-and-space-drag`)

The Spaces list in the three looks is the app's own `spaces` mod, and a live drag drew nothing and moved nothing. **The cause is not established**:
measured in a sandbox, the mod's own `ordered()` pass does not undo a move of an ordinary space (`space.move` sticks, a main checkout's worktree spaces follow it;
it only puts a worktree space back under its checkout), but the pointer path could not be driven (the app takes no synthetic pointer events, and no real drag on the
person's cursor was allowed). So `mods/foreman-spaces/` is a clone (`mod.clone`, app 0.1.15; `mod.json` names it) whose one change is where a drag is decided: the
reducer `reorder.js` (pure; `reorder.test.mjs` holds it, run by test 1272), wired by `main.js` and commented line by line in its header. A drop no longer needs a mouse-move to have
put the pointer over the target (the target row's own mouse-up is the target; the shipped rule needed `over`), a row also reports the pointer by `on_hover`, the list's own
mouse-up ends a drag released between rows, and leaving the list cancels it. **Unverified live**: whether the app delivers those events mid-press is only seen with a real
drag, which the person tries after the merge. Link it: `ln -sfn "$PWD/herdr-app/mods/foreman-spaces" ~/.config/herdr-app/mods/foreman-spaces`; the layouts place it in the `spaces` block
(test 1283 counts it as `spaces`).

## Role icons on the tab bar, and the statusline's look (2026-11-12, `setup/claude-session-foreman-look`)

- **Tabs.** The tab bar is the app's `tabs` mod, which only a clone can change (`mod.clone`; the upstream limit, herdrdev/herdr-app#246, is about
  *adding* to the bar). `mods/foreman-tabs/` is that clone: a tab whose terminal is a brainer or an executor draws the brain or hard hat of the Agents list
  instead of the program icon; every other tab keeps the program's. A mod cannot import outside its own folder (`resolves outside the application
  directory`, measured), so `role.js` is a byte copy of `foreman-agents/role.js`; test 1360 holds the copy, and the three layouts show `foreman-tabs`.
  Link it like the others: `ln -sfn "$PWD/herdr-app/mods/foreman-tabs" ~/.config/herdr-app/mods/foreman-tabs`.
- **Status line.** `bin/statusline-look apply|revert|check` puts `lib/statusline/ccstatusline.json` (d-linear tokens, no labels) on both accounts, with
  ccstatusline pinned to `@2.2.27` and still wrapped by `statusline-usage-tee`; backups are `<file>.bak-foreman-look`, `revert` restores them. Test 1359.
