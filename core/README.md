# hw-core

The Go half of `bin/hw`, as a pilot (trains T0, T1, T2, T3 and T5, the dispatch with its launch — `setup/decisions.md`, «El núcleo de hw migra a Go»):
`receipt`, `log`, `handle` and `help` (T0), `worktrees` and the table of `status` (T1), and the marker writers
(T3: `ruling`, `human-boundary`, `executor-turn-end`, `chaining-lease-start`/`-expire`, `outbox flush`, `output`,
`stage`), and the dispatch (T5: `hw <lane> <task> …`, its dry run and its launch), ported byte for byte from the bash they replace. Nothing else is
ported, and nothing changes for anyone unless `HW_IMPL` is set (`bin/hw`, "hw-core: the verbs that run as a
Go binary").

    HW_IMPL=bash   (default) the bash that has always run — the rollback
    HW_IMPL=go     exec hw-core for a ported verb
    HW_IMPL=shadow run both, print bash's answer, append any difference to ${XDG_STATE_HOME:-~/.local/state}/hw/shadow.jsonl (a hw-core older than core/ is not run: bash answers, with one warning)
    HW_IMPL_<VERB> overrides HW_IMPL for that verb (HW_IMPL_RECEIPT, _LOG, _HANDLE, _HELP, _STATUS, _WORKTREES, the T2 verbs below, the T3 writers: _RULING, _HUMAN_BOUNDARY, _EXECUTOR_TURN_END,
                   _CHAINING_LEASE_START, _CHAINING_LEASE_EXPIRE, _OUTBOX, _OUTPUT, _STAGE, and T5a: _DISPATCH)

`hw-core status` prints the TABLE of `hw status` (`cmd_status`, lib/hw/status.sh); `bin/hw` calls it where it called
`cmd_status` and still prints the quiet checks after it (outbox, release, gate, tabs, hygiene, rules, decisions), which
stay bash until a later train. `worktrees --quiet`, the audit `hw status` runs, goes through the same switch. In `shadow`,
`worktrees --migrate` is never run twice: bash answers alone.

Train T2 adds the read verbs that depend on T1: `reap` and `sweep` (dry runs), `reports`, `review-range`,
`review-delta`, `preflight --json`, `outbox list`, and the `wait` family (`wait`, `wait-one`, `wait-monitor`, exit
contract 2/5/6/7/8). The shim sends only the READ half of a verb to hw-core: `reap --apply`, `sweep --apply`,
`outbox flush` (T3, below) are not T2's (a `reports` that repairs is T6's, and under `shadow` it is bash alone), and under `shadow` a real wait is answered by bash alone
(running it twice would wait twice); a `--dry-run` wait is a reader and runs in both. `HW_IMPL_<VERB>` names are
the verb upper-cased with `-` as `_` (HW_IMPL_REAP, _SWEEP, _REPORTS, _REVIEW_RANGE, _REVIEW_DELTA, _PREFLIGHT,
_OUTBOX, _WAIT, _WAIT_ONE, _WAIT_MONITOR). What is not byte for byte is listed in `setup/contracts/hw-t2.md`.

`worktrees` compares an agent's cwd with a worktree path as bash does: as written, except under msys/cygwin, where
bash puts both through `realpath` (backslashes, case, `..`, links) and a native Windows hw-core does the same.
`HW_CORE_PATH_RULES=windows|posix` forces one set of rules on any system (the contract cases use it); unset, the
system decides. Shadow compares what bash and Go print without their colour, so a terminal never logs a difference.

## The marker writers (T3)

Hooks and invokers read what these verbs write (`turns`, `handback-refused`, `pending-ruling.<n>`, `chaining-lease`,
`receipt.jsonl`, the outbox's `.meta`), so the bytes are the contract (`setup/tests/840`, golden = bash's, 242 cases).
Two things differ from the readers above:

- **`shadow` never writes.** hw-core runs FIRST with `HW_CORE_READONLY=1` and `HW_CORE_PLAN=<file>`: every write goes
  through `fsx` (`t3.go`), which records it (path, bytes, mode) instead of doing it; then bash writes for real; then
  `hw-core shadow-verify <plan>` compares the plan with the files bash left, timestamps, epochs, tokens and pids
  normalised, and a difference goes to `shadow.jsonl` as `differs: ["tree"]`. `chaining-lease-expire` and `outbox
  flush`, whose effect is outside the run directory (a pane closed, a message sent), run in bash alone under shadow.
- **Some of what the bash does stays an exec.** `holdfacts` (the validity of an `ask-invoker` hold), `cockpit-state` and
  `channel-send` are Python/bash helpers on `HW_CORE_BIN_DIR`; hw-core runs them as bash did and does not port them.
  The herdr metadata publish, the engram session renewal and the pane witness are native.

`ruling` and `outbox` are brainer verbs: an executor (HW_TASK or HW_RUN set) is sent to bash, where the refusal text lives.
`outbox list` is another train's.

## The dispatch's dry run (T5)

**Scope: T5a (the dry run) and T5b (the launch: the worktree, the database, the pane, the brief delivered).** Under
`HW_IMPL=go` (or `HW_IMPL_DISPATCH=go`) `bin/hw` execs `hw-core dispatch <argv…>` for ANY dispatch, with or without
`--dry-run`; hw-core decides what the argv is, so a `--dry-run` that is only an option's value is parsed as the value
and the dispatch is a launch. The launch's programs (herdr, git, jq, psql, the agent) are children of hw-core, run
through one function (`t5l_exec.go`), as they were children of bash.

`hw <lane> <task> … --dry-run` prints the manifest a brainer reads before it pays for an executor, then the warnings
and refusals a launch would raise, and ends. What the dry run writes is the route
forecast (`${TMPDIR:-/tmp}/hw-route-forecast-<lane>__<task>`, read by the launch that follows); under `shadow`
bash answers for real and hw-core runs after it with `HW_CORE_READONLY=1` and writes nothing. The run id (a
timestamp and a pid) and the port opencode is told (the kernel's pick) differ between two processes by
construction: the contract and `shadow.jsonl` read them as `@RUN@` and `@PORT@`. Under `shadow` bash answers into a
file, not the terminal it was started on, so what it prints has no colour (the comparison reads both without it);
`go` and `bash` colour as they always did. A shell error bash prints about its own arithmetic (`[: …: integer
expression expected`) is not in the contract and hw-core does not reproduce it.

**A launch under `shadow` is a writer, not a pair of answers** (it is not run twice: it makes a worktree, a database,
a pane). hw-core runs FIRST with `HW_CORE_READONLY=1` and `HW_CORE_PLAN=<file>`; its reads (herdr, git) are real, the
world not yet being changed; its mutating calls are recorded and answered with the synthetic reply the launch code
carries; its file writes go through `fsx` and are recorded. Then bash launches for real, and `hw-core shadow-verify
--launch <plan>` holds the plan against what bash left: every planned file exists with the same bytes (run ids, ports,
pane ids, pids, tokens, timestamps and temporary paths read as placeholders), every planned symlink points where bash
pointed it, and what a child wrote beside them is not a difference. The plan's `exec` ops (a program, its argv joined
by NUL, its directory) are NOT compared: bash's calls are not observable in production, so `shadow.jsonl` says
`"calls":"not compared"` beside `differs`. bash's stdout and stderr are not captured (the launch is long and its
user watches it), so they are not compared either: the verdict is the exit status and the files. Test 849 holds this
shim; the byte comparison of the launch's answer is the contract world's (847).

The reuse route (a live, leased executor in the lane) prints the same manifest and then hands the brief's dry run to
`<HW_CORE_BIN_DIR>/hw next … --dry-run`, a child process, as bash called `cmd_next`.

The port is in four areas, one file family each, all methods of the dispatch state `dsp` (`t5_state.go`; a bash
function `_check_brief_contract` is `checkBriefContract`): the flow and the manifest (`t5.go`, `t5_manifest.go`,
`t5_lanefields.go`: the lane fields the T0/T1 loader left out, with lane_config_load's defaults), the brief and its
contract (`t5_brief*.go`), vendors, models, accounts and guards (`t5_vendor*.go`), and sdd, reuse discovery, the
route forecast and the invoker (`t5_reuse*.go`, `t5_sdd*.go`). Tests: 843 (contract, ~520 cases, bash/Go/shadow),
844 (one mutant per decision), 845 (the shim), 846 (the `bin/hw: line N:` of the two shell complaints the dry run imitates: hw-core finds the line in `bin/hw` by its anchoring text, and 843 normalizes the number away, so only 846 notices a stale one). The cases are `contract-hw/cases-t5*`, the goldens `golden-t5/`
(`contract-hw/regen-t5.sh`, bash only).

A hw-core that is not built is not an error: `bin/hw` warns once and runs bash. `setup/build-core` builds it
(five targets, reproducible, with sha256 sums) into `core/dist/<os>-<arch>/`, where the shim finds it.

## Why `core/` and not the brain root

A `go.mod` at the root would make every `go` command walk a tree of three thousand tracked files, every
lane's worktrees under `work/` and whatever a lane keeps in `node_modules`; a module of its own bounds what
`go build ./...` and `go test ./...` can see to this directory, gives `dist/` a home that `.gitignore` can name
in one line, and lets the export leave the pilot out with one `exclude core/` (`setup/export/manifest`).

## Rules of this module

- Standard library only, `CGO_ENABLED=0`. The one place the library does not reach is the terminal test
  (`isatty_*.go`), written with `syscall` per system, which is what the library itself does.
- The text is the bash's. `help/*.txt` are the heredocs of `_usage_*` taken verbatim; a better sentence breaks
  the suite, which matches strings. The contract is `setup/contracts/hw.md`; the goldens are written by bash only.
- A fix to a ported verb goes here, not to the frozen bash (`setup/decisions.md`). The mutation anchors are
  `// # MUTATION-ANCHOR: 816-G<nn>` on the line a mutant replaces (`setup/tests/816-*`; T3: `841-*`, `842-N01`; T5: `844-M<nn>` the flow and manifest, `844-A<nn>` brief, `844-B<nn>` vendors, `844-R<nn>` reuse/sdd/invoker, and the bash id of a line ported from bin/hw behind `844-` (`844-37-M02`); the rows are `contract-hw/mutants-t5-*`, `setup/tests/844-*`).
- `contract.go` is the library later trains build on: run-directory discovery, the markers, and the `env`
  grammar held to `bin/runenv` by `go test`.

## Layout

    app.go       entry, the lane table's lookups, the process's exit
    lanes.go     projects.json, loaded and checked as lane_config_load does
    receipt.go   receipt and log
    snapshot.go  the report snapshot and the context line (both read the pane through the herdr CLI)
    herdr.go     the herdr client: the CLI reads (parallel, bounded) and the NDJSON socket (the pane.exited backlog)
    status.go    the table of `hw status` and the sections after it; statusdata.go: what it reads of each run directory
    worktrees.go worktrees, and who is working in a stray one
    pyport.go    the Python behaviours status.sh leans on (int, float, %g, repr, json.dump, glob, realpath)
    reap.go      reap (dry run): worktrees by disposition, loose work dirs, orphan branches, the database ledger, build output
    sweep.go     sweep (dry run), with markers.go: the done / reopened / blocked markers and the leases of a run
    reports.go   reports; review.go review-range and review-delta; outbox.go outbox list; preflight.go preflight --json
    wait.go      wait, wait-one, wait-monitor: the exit contract (rcTimeout 2, rcSettled 5, rcHuman 6, rcStranded 7, rcRefused 8)
    sortlocale.go the locale order bash sorts and globs by, asked of `sort` itself
    handle.go    handle
    help.go      help (and fold)
    jsonjq.go    JSON as jq reads and writes it: the stream, the numbers, the escapes
    term.go      info / warn / die, colour
    t3*.go       the marker writers: t3.go (dispatch, the fsx write plan, shadow-verify), t3_ruling.go, t3_turnend.go,
                 t3_lease.go, t3_outbox.go, t3_simple.go (human-boundary, output, stage), t3_pid_*.go
    t5*.go       the dispatch's dry run: t5.go (flow, flags), t5_state.go, t5_manifest.go, t5_lanefields.go,
                 t5_brief*.go, t5_vendor*.go, t5_reuse*.go, t5_sdd*.go
    contract.go  the run directory and the env file

## The verbs that were bash-only (T6)

`train`, `suite`, `unstick`, `ledger` and `briefs` run in Go (`t6_<verb>.go`), as do the repairing form of
`reports` and `worktrees --migrate` (which only lacked a shim route), `cockpit-state` and `secret` (hw-core runs
`bin/cockpit-state` and `bin/hw-secret` as its child with the same stdio, as bash `exec`'d them) and `-V`/`--version`
(answered before the lane table loads). The switch is the same (`HW_IMPL`, `HW_IMPL_<VERB>`; `HW_IMPL_VERSION` for `-V`),
and `lib/hw/core-t6.sh` — sourced by `bin/hw` right after the T2 switch — holds it, so adding it moved no line of `bin/hw`
that `setup/contracts/hw.md` cites. Under `shadow` these verbs answer in bash alone (they write: a merge, a restarted pane,
an archived brief), and an executor (HW_TASK/HW_RUN set) goes to bash for `train`, `unstick` and `briefs`, where
`_executor_refuse` words the refusal. The mutation anchors of their bash stay registered on the bash source: the Go ports
are held by `TestT6*` (in-process, `core/t6_*_test.go`), and a Go-side anchor would be a second marker of an id the registry
keeps once.

## channel-send (the delivery transport)

`hw-core channel-send <args>` is `bin/channel-send` in Go (`channel_send*.go`): the same command line, stdout, stderr,
exit codes (0, 1, 4, 5, 6 — the table `--help` prints) and on-disk effects (the delivery lock, the reply hold, the
task transcript). `bin/channel-send` keeps its bash as the rollback and execs hw-core when `HW_IMPL_CHANNEL_SEND` (else
`HW_IMPL`) is `go`; `shadow` and unset run the bash. The programs it ran it still runs (`herdr`, `herdr-rpc`,
`holdfacts`, `claude-delivery-receipt.py`, `claude`, `codex`, `ps`, `find`); what it did with `jq`, two `python3`
heredocs and two `node` scripts is native here. `setup/tests/1015` runs both on the same inputs and diffs them;
`setup/tests/1016` kills a mutant of each mutation anchor (named `<n>-G<nn>`, the bash's `<n>-M<nn>`) with
`channel_send_test.go`. Not byte for byte, and said so: the message of an unparseable JSON body (node's own parser
text, imitated for the two common shapes) and the order of the session ids in the opencode-tui busy refusal
(the endpoint's in node, sorted here).
