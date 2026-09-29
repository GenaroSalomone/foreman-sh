# layouts — one JSON document per lane, applied in a single `layout.apply`

`hw` used to build a task space incrementally: `pane split`, `pane split`,
`pane rename`, `pane rename`, `--env` repeated per pane. Every one of those was
a separate round trip, and every one was a place where a `set -e` trap or a
herdr hiccup could leave a half-built space that `hw done` would then have to
clean up. `layout.apply` (API-only — there is no `herdr layout` CLI, hence
`bin/herdr-rpc`) takes the whole tree in one request, so the structure is either
there or it is not.

Each file here is the `layout.apply` params for one lane. `hw` renders it and
sends it; nothing else reads them.

## The contract with `hw`

* Strings may contain `${NAME}` placeholders. `hw` substitutes them **after the
  JSON has been parsed**, so a value containing a quote or a backslash cannot
  break the document. A placeholder `hw` does not set is an error, not an empty
  string.
* Any key whose name starts with `_` is stripped before the request is sent.
  That is how these documents carry comments, which JSON otherwise cannot.
* Every `pane` node gets the executor environment (`HW_PROJECT`, `HW_TASK`,
  `HW_WORKDIR`, `HW_RUN`, `ENGRAM_PROJECT`) merged into its `env` by `hw`. Do
  not list those here — they would drift the day `hw` adds one. List only the
  variables that are specific to this pane, such as a port.
* `tab_id` is `${HW_TAB}`, the tab of the workspace `hw` just created.
  **`layout.apply` with a `tab_id` REPLACES that tab**: the tab and its panes
  are destroyed and rebuilt, and the result carries a new `tab_id`. That is
  wanted here — the workspace's initial pane is exactly what we are replacing —
  but it means these documents must describe the whole tab, never a fragment.

## Why no `command` on the dev-server panes

`layout.apply` accepts a `command` argv per pane, and `hw` deliberately does not
use it for dev servers. A pane whose process IS the command dies with the
command: one Ctrl-C on `pnpm dev` and the pane vanishes, taking the stack trace
with it. Verified on a probe pane — `send-keys C-c` on a `command:` pane left
`pane_not_found`. So the panes are created as plain shells with the right `cwd`
and `env`, and `hw` starts the servers with `pane run` afterwards, which is what
makes Ctrl-C-then-up-arrow keep working.

The agent pane is empty here for a different reason: it must be started by
`herdr agent start`, which is what writes `managed_agent_kind` into
`~/.config/herdr/session.json` (verified: the executor pane
has both `managed_agent_kind` and `agent_session`, and non-agent panes persist no
`command` at all). A `claude` launched as a bare `command` here would be an
ordinary pane to herdr: absent from `agent list`, invisible to `agent prompt`,
and **(unverified)** almost certainly restored as a bare shell. So `hw` keeps
`agent start` and uses `layout.apply` only for the structure around it.

P6 HOOK, not built here: if `hw` ever writes `<workdir>/.hw/<run>/env` so a
resumed pane can reconstruct its environment, the dev and shell panes are where
a `command` that sources it would go. Left alone deliberately — the restart
experiment that P6 depends on had not run when this was written.
