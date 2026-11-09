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

A file older than 15 s is dimmed and marked `stale`; older than 60 s, or
unreadable, or naming another schema, it is drawn as `NO STATE`. Rows are never
kept from a file that has gone bad.

What it does not do: the buttons. `verify`, `receipt`, `done`, a ruling and a
reply all run verbs (`cockpit/verbs.txt`) with a person's text on stdin; this mod
only reads, so every card is a display of what `hw` decided, and the person still
answers in the brainer's own pane.

## Reaching each panel

| panel | how, from the app | how, from a shell |
| --- | --- | --- |
| Cockpit | the right column, `Cockpit` label (shown first) | — |
| Issues | the right column, `Issues` label | `herdr-app do tab.new --mod foreman-issues` |
| Artifacts | the right column, `Artifacts` label | `herdr-app do tab.new --mod foreman-artifacts` |
| File viewer | **no route inside the app** (below) | `herdr-app do pane.split:right --mod foreman-viewer --props '{"file":"/abs/path"}'` |

The right column is a `stack` (`right-body`) under a row of three labels
(`right-tabs`, the cockpit mod in its switch mode: `props.switch`); a label runs the
app's own `layout.select {stack, child}`. The app keeps the choice across restarts. The
underline follows the last label clicked, else the layout tree's `active` child; a stack
changed some other way (`herdr-app do layout.select …`) can leave it on the wrong label
until the next click (`layout()` is documented as the layout file, so this is unproven).

**Artifacts are drawn as the page renders.** Reading an artifact saves its HTML as before;
the mod then starts Chrome headless (Google Chrome, Chromium, or Playwright's
`chromium_headless_shell`) through `exec`, measures the page at 1000 css px wide and shoots
it at 2x into `~/.config/herdr-app/cache/artifacts/<id>-<version>.png` (the app draws images
only from inside its folder; the version is a hash of the page, so an edited artifact is a
new picture and an unchanged one is not drawn twice). The panel shows it with `image()` at
the panel's width, scrollable. Every launch passes `--use-mock-keychain
--password-store=basic` and its own throwaway `--user-data-dir` under `TMPDIR`, so it never
touches the macOS keychain or the person's Chrome profile (`render.test.mjs` asserts it).
Measured: 4.3 s for a real 3504 px artifact; pages taller than 7000 css px are cut there. No
Chrome, or a failed render, falls back to the page's text with the reason named. The `browser`
button stays. `gpui-kit.d.ts` has no web view, so this is the in-app route. Unverified until
the merge: how `image()` sizes itself with only `w_full()` (the app was not opened).

**Markdown is drawn one way** by Issues (description and comments), Artifacts (an answer with
no page) and the file viewer: `md.js` splits it into headings, lists, fenced code, quotes and
paragraphs, `mdview.js` draws each in the active theme's tokens (code on `selected`, quotes
under a hairline); inline marks and links stay with `TextView.markdown`. Issue labels are chips
in GitHub's own colors, state a dot plus text, each comment an author/date block between hairlines.
The Issues list says each fact once in every look (`foreman-issues/rows.js`): under All repos one
header per repo (name and count) and no repo on the rows, number and title in fixed columns, the state only under **Any**, one date format (`5d`, `2mo`, `1y`).

**App limits that block the rest** (measured 2026-11-09 against `herdr-app commands`,
`herdr.d.ts` and `AGENTS.md`): the command palette (cmd-shift-P) lists the app's own
commands and no mod can register one, so `Cockpit`, `Issues`, `Artifacts` and
`Open in viewer` cannot be palette entries; and the app has no file-open hook (Files
previews a file in its own panel; a terminal link is only a live terminal address). The
viewer therefore stays a shell command (`fv`, in its README).

## Looks

A look is a **layout plus its theme**: `layouts/foreman.json` is C, `layouts/foreman-d.json` D and
`layouts/foreman-g.json` G. Each draws the window with **the app's own working mods** —
`workspace-chrome` (brand and footer), `spaces`, `agents`, `tabs`, `pane` (G's pane header) and
`workspace-status` — and styles them only through what the app gives a layout: the theme's colors,
the fonts, region sizes, and paint (`background`, `border`, `radius`, `padding`, `gap`, `divider`,
the pane frame). The built-in mods take no style props, and only the root's look applies
(`AGENTS.md`, "Today only the root's look applies"). The right column is the foreman cockpit,
Issues and Artifacts, each block naming its look (`props.look`).

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
`chrome`), and fails any layout whose brand, spaces, agents, footer, tab bar or status bar is not
the app's own mod, or whose foreman block draws a window part (`props.part`): the premium pass's
drawn stand-ins showed empty Spaces and Agents, no tab bar, and a stub status bar (2026-11-09).

**Proved in the app, not a harness** (2026-11-09): a second, isolated Herdr instance — its own
`HERDR_APP_HOME`, and its own core through `XDG_STATE_HOME`/`XDG_RUNTIME_DIR`, both sockets
checked with `lsof` before use — with this tree's mods and layouts, ran each look through: switch,
create, rename and close a space; open, switch and close a tab; agent status (stand-in agents
reporting OSC 7501) and focusing an agent's pane; the status bar; Cockpit, Issues and Artifacts;
the look switcher (each of C, D, G) and the `cmd-shift-l` cycle. 19/19 per look. Each item runs the command the app's mod runs on that click; the window was not
clicked or dragged (no pointer automation on the person's display), so drag is held by the layout
(`titlebar: none`, `window_drag` on the brand and the tab bar).

The isolated instance, as run for the Issues shots (Herdr 0.1.14):

```sh
I=/tmp/hin2   # short: under $TMPDIR the core's holder socket passes the 104-char limit
mkdir -p $I/{app,state,config,run,bin,zdot}
cp -R herdr-app/{mods,layouts,themes} $I/app/   # plus the app's built-in mods, as in ~/.config/herdr-app/mods
echo '{"frame.welcomed":"skipped"}' > $I/app/shared.json   # overlay.close does not dismiss the welcome
# gh has no auth there (XDG_CONFIG_HOME is isolated): a fixture `gh` in $I/bin, never real credentials.
# exec takes PATH from the login shell, not from this env, so prepend $I/bin there:
printf 'source ~/.zshrc >/dev/null 2>&1\nexport PATH=%s/bin:$PATH\n' $I > $I/zdot/.zshrc
printf 'export PATH=%s/bin:$PATH\n' $I > $I/zdot/.zlogin
env ZDOTDIR=$I/zdot HERDR_APP_HOME=$I/app XDG_STATE_HOME=$I/state XDG_CONFIG_HOME=$I/config \
  XDG_RUNTIME_DIR=$I/run HERDR_TELEMETRY=0 /Applications/Herdr.app/Contents/MacOS/Herdr &
lsof -a -U -p $! | grep sock   # $I/app/herdr-app.sock, never ~/.config/herdr-app/herdr-app.sock
```

Drive it with the same env: `herdr-app do layout.select '{"stack":"right-body","child":"issues-body"}'`,
`herdr-app do layout.resize '{"id":"cockpit","px":816}'`, `herdr-app shot out.png`.

What the mockups show and the app does **not** — function won each time:

1. Sample content (task names, the terminal's text) is the mockups'; the app draws live state.
2. The sidebar, tab bar, pane header and status bar are the app's mods in the look's colors and
   fonts, not the mockups' drawings: C's inverted white space row and tab, D's brand badge and
   Inbox, G's project tree (spaces → agents with chevrons), its title bar, its tab notch and its
   status-bar toggles. Each needs a `mod.clone` of the app's mod that is then ours to keep in step
   with every app update; not done. The app's Herdr brand, Settings and Add machine stay.
3. C's mockup has no Issues/Artifacts switch; the switch stays (approved with the panels).
4. Artifacts keeps its own header bar (`updated`, Refresh, accounts). Issues folds it into the search row (`updated`, Refresh, and the count only when a filter narrows the list), with its filters on two rows below: Work/Personal, the scope and the state on one line that wraps a whole group at a time, and the repo chips on their own.
5. D's Inter is not in the app: the UI uses the system font, as the mockup itself fell back to.
6. D and G draw a brainer with no executors as its own section (`No executors.` / a folded row),
   as their mockups do; C hides it.
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
| `mods/foreman-cockpit/hover.test.mjs` | the cockpit sets no tooltip (run by test 1272) |
| `mods/foreman-cockpit/state.js` | the reader of `cockpit/schema.json`: parse, classify, format |
| `mods/foreman-cockpit/switcher.js` + `.test.mjs` | the right column's switch: which stack child shows, the C/D/G look switcher's layouts, and the layout it must match (also run by test 1272) |
| `mods/foreman-cockpit/state.test.mjs` | the reader against `cockpit/fixtures/index.json`, run by `setup/tests/1272-herdr-cockpit-mod-reads-the-contract.sh` |
| `mods/foreman-artifacts/render.js` + `.test.mjs` | an artifact page to a cached PNG through Chrome headless; the test draws a real page and asserts the keychain-free flags |
| `mods/foreman-issues/md.js` + `.test.mjs`, `mdview.js` | markdown as blocks and their drawing, plus GitHub label colors as chips; identical copies in the artifacts and viewer mods (test 1277) |
| `mods/foreman-*/tokens.js` | the one look all four foreman mods share (type, spacing, radius, the status dot, the header bar); identical copies, held by `setup/tests/1277-herdr-mods-share-one-look.sh` |
| `themes/c-mono-signal.json`, `themes/d-linear.json`, `themes/g-zed.json` | looks C, D and G as themes; `mods/foreman-cockpit/looks.test.mjs` holds every theme against every mod |
| `layouts/foreman.json`, `foreman-d.json`, `foreman-g.json` (+ `.png` previews) | looks C, D and G as whole windows: sidebar, terminals, the right column (Cockpit, Issues, Artifacts), status bar, all the app's own mods; the cockpit body names `props.dir` |

Issues and artifacts also open as tabs or overlays; the viewer only as a pane. Dragging the dividers resizes the regions.

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
  predate your change. Bring Herdr to the front yourself before trusting a shot;
  an agent records it as pending instead.
- **Switch the layout back** with `herdr-app do layout.switch '{"name":"main"}'`
  (or `cmd-shift-l` to cycle). If a layout breaks the window,
  `ctrl-alt-shift-escape` always restores `main` and sets the shown layout aside.
