# foreman-sh 0.7.1

foreman-sh 0.7.1 makes pi executors hold to Claude's rules, keeps the vault unlocked, and makes the gate's red verdicts more honest.

## In short

- **The vault stays unlocked.** `secret-unlock --auto` renews a session before it expires and leaves a healthy one alone. `hw secret check` tries it once before refusing, and every outcome is a timestamped line in `~/.cache/hw/bw-unlock.log`, with bw's own words and nothing secret.
- **pi waits like Claude.** A pi executor runs a command in the background, is woken when it ends, and is refused the same sleep loops Claude is. Its subagents run at the lane's model tiers, its Judgment Day judges are proved from its own session, and pstack's children are held to its write guard.
- **`--effort` is documented as it works.** `hw help flags` names what each of the four vendors takes, and a test fails if the help and the code disagree.
- **Reds that name their cause.** Under `--keep-going` every red is re-run alone and labelled ENV-ONLY, FLAKY, STALE-AT-BASE or REAL. A busy batch lock waits instead of throwing away the run, and a broken pipe no longer reads as a vacuous mutant.
- **Safer parallel work.** `hw` warns when two live tasks touch the same files, and a nested probe can no longer overwrite its parent pane's report. A setup executor cannot write into the brainer's checkout.
- **No more notifications.** foreman sends none. Everything they said is still in the report, `hw status` and `close.log`.

## What changed

67 changes since 0.7.0.

### Added
- A pi executor's declared Judgment Day is proved, not assumed: the dispatch writes `jd_agents_t<N>`, and
  `done-invoker` refuses the close unless the session records a call to `jd-judge-a` and another to
  `jd-judge-b` (agent, task, `async:false`). The builtin `reviewer` no longer stands in for a registered judge.
- The extension a plugin's child discovers: a plugin dispatch's private pi agent directory carries
  `extensions/hw-child-guard.ts`, so pstack's `agent` children are held to the executor's write guard and
  secret-read redaction.
- A pi close proves pstack's review agents too: a foreground call to pstack's own `agent` tool (its
  `subagent_type`) counts, so `pstack:comment-sicko` can run and be held to the proof.
- A pi executor runs a shell command in the background and is woken in the session when it ends, with
  its exit code, its last 20 lines and the path of its whole output (`bash_background`, the equivalent
  of Claude's `run_in_background`), and stops one with `job_stop`. The command runs through pi's own
  shell operations, at most 25 run at once, and a turn that ends while one runs is a legal turn end, so
  the executor stops and is woken instead of hand-rolling a poll loop.
- The sleep-poll rule reaches pi: a `sleep` over 30 s, a `while`/`until … sleep` loop and a foreground
  `tail -f` are refused by the same guard Claude's hook asks, and the refusal names pi's own
  background tool. An unbounded loop or follow is refused in the background too, and a bounded one is
  allowed, as before.
- A pi executor's subagents carry the lane's model tiers: `pi_subagent_models` in
  `projects.json` maps a Claude agent's `model:` (`haiku`, `sonnet`, `opus`) and any
  agent the lane classifies by name to a pi model and thinking level, registered in
  pi-subagents' own `model`/`thinking` fields. Explore agents read on the cheap
  model at off/low thinking; build and review agents run a strong model at
  medium/high. The receipt's `pi_specialists` row names what each specialist runs.
- A pi executor that reads more than 3 files in one request (or 40 KB of text)
  gets ONE steer toward the explorer, through pi's native
  `sendMessage(deliverAs: "steer")`, and the receipt records it
  (`pi_exploration_budget`). It is armed only where hw injected an explorer.
- Under `--keep-going` every red subject is re-run alone with `env -i` and labelled ENV-ONLY, FLAKY, STALE-AT-BASE or
  REAL, printed beside it in the final `not ok` list; at most `HW_TEST_LABEL_MAX` (6) subjects and
  `HW_TEST_LABEL_BUDGET` (600 s), and past the bound the label is UNLABELLED, never guessed.
- `hw` warns at dispatch when the paths a brief names, or the paths this task's branch already changed, overlap the
  unmerged branch of another LIVE task of the lane: the warning names the task and the files. It never refuses.
  (2026-11-04: two live tasks edited `core/invokers/transcript_pi.go` and `setup/pi/hw-executor.ts` at once.)
- `hw --dry-run` names the thinking level a pi executor will really run at, and warns when pi clamps the request or when
  it is above the pi-roster ruling. Measured 2026-11-04 on `opencode-go/deepseek-v4.1-flash`: off/minimal→low,
  low→low, medium/high→high, **xhigh/max→max** — three real rungs, correcting the 2026-11-01 ruling in `setup/decisions.md`.
- A setup executor can no longer write in the brainer's checkout of brain: with `HW_TASK` set and `HW_PROJECT=setup`,
  the Write and Edit tools, a Bash that writes there, and a `cd` into it are refused, in the Claude hook
  (`setup/guards/deny_setup_brain_writes.py`, registered in `.claude/settings.json`) and in the pi adapter. Reads,
  brain's `bin/` tools by path and the executor's own worktree stay allowed; the brainer is not judged.
- A pi executor's prompt says which guards judge it, from where, and how old they are: the lane root's copy, never its
  worktree's. The adapter is read once at session start, so a rebase or a ruling that changes it reaches only a new
  session; the sleep-poll rule it spawns per call is read then, from that same lane root; the two global hooks are also
  per call but from `~/.claude/hooks`. The line says to report which guard judged it instead of re-running a refused
  command. Measured 2026-11-04: a run whose branch had gained the sleep-poll guard still saw a `sleep 540` pass, because
  the adapter loads at session start and from the lane root. The pi prompt sample sat at exactly its 5000 B ceiling, so
  its budget moves to 5300 for this rule, with the reason in `core/t5l_prompt_sample_test.go`, and nothing else moves.
- `~/.cache/hw/bw-unlock.log` is a timestamped line per `--auto` outcome (`ensure`, `check`);
  a rejection carries bw's own error text with passwords, session tokens and `BW_*` values redacted.

### Changed
- The shared Go build cache is capped at 12 GB instead of 4 GB, so parallel
  worktrees no longer evict each other's builds and recompile on every gate.
- pi's two Judgment Day judges travel in ONE `subagent({workflow:"parallel",…,async:false})` call, the shape
  pi-subagents' parallel resource takes, because a second `subagent` call in the same turn is refused; the
  transcript reader counts that call as both judges, and none of `workflow:true` or a script path proves anything.
- pi's closing says so itself when `jd-judge-a`/`jd-judge-b` are not on disk, or when pi-subagents is absent and
  neither a judge nor a `reviewer` can run, instead of handing over names that are not registered.
- KNOWN-LIMITATIONS lists, row by row, every executor optimisation Claude gets with pi's equivalent or the
  reason there is none.
- The shipped text names no machine-local tool of whoever built it: `hw secret` and
  `secret-unlock` say you schedule `secret-unlock --auto` yourself, on macOS, instead of
  pointing at an installer this project does not ship, and `setup/heavy` and the runner
  describe "the memory guard" rather than naming one.
- `hw help advanced` and `docs/KNOWN-LIMITATIONS.md` call `--account personal` the second
  account, which is what it is.
- Judgment Day's judges now check trust boundaries: a value a diff reads instead of
  creates — an inherited environment variable, argv, a file, a directory listing, a
  message from a peer process — is a finding when the code uses it without
  validating it, and both judges carry the rule word for word, so the pair stays
  blind and symmetric.
- The pi manifest's `pi subagents` line names the tier table a specialist's
  `model:` resolves to (and the agents the lane classifies by name), instead of
  saying the model is not carried.
- `docs/pi-vendor.md` records that a LIVE pi session changes its own thinking level and model (`/thinking`, `shift+tab`,
  RPC `set_thinking_level`/`set_model`), so effort never needs a relaunch.
- `done-invoker --check` runs every close gate (the 900-character limit, `checks.md`'s verdicts, a
  declared design boundary, the plugin review proof) and delivers nothing, so a refusal is found
  before the work is done. `done-invoker --help` prints the real usage, `--help` included anywhere
  among the flags.
- Docs: the judgment-day skill names pi's single-call parallel form for the two judges
  (`subagent(workflow: "parallel", …)`), beside Claude Code's two `Task` calls.
- Docs: the brief template orders the close — commits, Judgment Day, its fix rounds, and the ONE fast
  gate on the final tree — and says every path is relative to the worktree; `setup/CLAUDE.md` names
  the changelog fragment path, `setup/export/overlay/changes/task-<task>.md`.
- Docs: a focused `-run` test of seconds runs as `setup/heavy --short -- <cmd>`, beside a
  held slot and never in the slot queue; `setup/CLAUDE.md` and `docs/BRIEF-TEMPLATE.md`
  say so with the 120 s cap.
- `secret-unlock --auto` leaves a session with over half its life left alone and says so.
- `hw secret check` on a locked vault makes the same single `--auto` try as `hw secret ensure`
  before refusing.

### Removed
- Every notification foreman sent: the dispatch, executor done/blocked, ask,
  `hw wait` and close-FAILED announcements, and the release cut's macOS
  notification. What they said is still where it was — the brainer's report,
  the pane's metadata, `hw status` and `close.log`.

### Fixed
- `hw help flags` described `--effort` as «claude and codex only», while the
  code carries it for all four vendors: claude's `--effort`, codex's
  `-c model_reasoning_effort=`, pi's `--thinking`, and opencode's
  `reasoningEffort` overlay. The entry now says what each vendor takes.
- An in-process test derives which vendors carry `--effort` from the command
  line the code builds and fails when the help entry names a different set.
- A pi dispatch whose brief declares a plugin runs on a private agent directory
  whose settings keep `theme`; the theme's JSON is now linked read-only into
  `<private>/themes/<name>.json` (the real `themes/` directory or a package's
  `pi.themes`), so pi no longer prints `Failed to load theme "<name>"` and falls
  back to the system theme.
- `hw revive` clears the run's `done` marker when it marks it reopened: `channel-send --report-state` refuses every
  report while that marker stands, so a revived run could not report at all (2026-11-04: the executor moved it by hand).
  The earlier report stays on record in the receipt and the ledger.
- The two files this branch adds under `core/` are classified in the export manifest and listed in the gate's dependency
  map; the contract page's moved `core/invokers/done.go` citations and the export's personal names are corrected, so
  subjects 606, 815 and 1014 are green again.
- `setup/gate-select`'s touched budget registers its two mutation anchors (510-T01, 510-T02) and each has its mutant
  arm: the cheap floor removed, and the cap spending itself slowest-first.
- `hw done` and `hw reap --apply` copy a pi run's newest session jsonl to
  `archive/<lane>/<task>/.artifacts/transcript.jsonl` before removing anything, so the transcript outlives the
  worktree (2026-11-04: no session to read after a close).
- The touched budget no longer cuts the cheapest subjects: `gate-select … touched` keeps every subject at or under
  `touched_cheap_seconds` (default 15 s) and spends the cap by taking the SLOWEST candidate first, so what is cut is
  always the most expensive thing left (2026-11-04: 3-13 s subjects were skipped while slower ones ran).
- An executor can run `hw <lane> <task> --dry-run`: a dispatch rehearsal creates nothing, so both the executor gate and
  the brain guard let it through, while every other refusal stays (2026-11-04: it was refused and the executor got past
  with `env -u HW_TASK`, five calls). The flag bypasses the gate only on the DISPATCH arm: a named brainer verb keeps
  its refusal even with `--dry-run`, because `sweep --apply` and `preview release` ignore the flag and change state.
- `setup/commit-tree` refuses, with `HW_TASK` set, a repository that is not `HW_WORKDIR`: an executor commits in its
  own worktree, never in the brainer's checkout (2026-11-04: a task commit landed on `main`). The brainer, with no
  `HW_TASK`, is unaffected. `hw` also warns at dispatch when a setup brief names the brain root as a place to edit.
- `setup/commit-tree` is near-linear in the number of named paths: the macOS case-collision check lowercases each
  prefix once instead of forking `tr` per pair, and the index is filled with one `hash-object --stdin-paths` and one
  `update-index --index-info`. A commit naming 635 paths took 31 minutes and never finished; it now takes 3 s.
- `setup/heavy` runs two slots by default (`HW_HEAVY_SLOTS=1` asks for one), so a waiter is no longer held
  behind a head that cannot use a free slot: a waiter may take a free slot the head's own
  `HW_HEAVY_SLOTS` excludes, and the head keeps priority for the slots it may use.
- `setup/heavy --status` lists every slot lock that exists, not only the caller's `HW_HEAVY_SLOTS`.
- `setup/heavy` no longer floods its output with `Broken pipe` lines: the queue is read once, never piped
  into `head`.
- `setup/export/check` also judges a new file under `setup/export/overlay/` before it is tracked, so a changelog
  fragment naming a term the export gate refuses is refused by the fast gate and not first found in review.
- The sleep-poll guard refuses a `while`/`until` loop that waits on `pgrep` (a process count that matches its own shell
  and other lanes' processes), in the background and under a `timeout` too; its refusals recommend only shapes it allows,
  held by a test.
- `hw status` names every guard installed under `~/.claude/hooks` whose bytes differ from its copy in `setup/guards/`,
  with the installer that fixes it, so a Claude executor is no longer left running an old guard unnoticed.
- A pi executor's `bash` call that sets no timeout is bounded at 600 s (`HW_PI_BASH_DEFAULT_TIMEOUT_S` changes it, `0`
  turns it off); a call ended by that bound says so and points at `bash_background`. pi's own `bash` has no default
  timeout, and two foreground probes hung 83 minutes with a ruling unable to land.
- `hw ruling` delivers to a pi executor that is working right away, through pi's native steering (`herdr agent prompt`;
  pi delivers it after the tool call in flight), records it as delivered (`via=pi-steer`) and leaves nothing queued for the
  turn end. A failed send leaves the ruling queued. Claude and OpenCode keep the turn-end queue.
- `setup/test-hw --keep-going` labels a red STALE-AT-BASE only when it also fails at the merge base (the check was
  inverted), finds that base in the live checkout (the runner's snapshot has no `.git`, so every red was called REAL),
  and removes the temporary base worktree it adds to the repo.
- The pre-commit hook no longer dies with exit 141 and no message on a merge that stages a file over 16 KB: it read the
  first line of each staged file with `printf | head -1` under `pipefail`, and `head` quitting early gave `printf` a
  SIGPIPE. A merge of main into a task branch could not be committed with `git commit`.
- The dispatch warning that two live tasks edit the same file also finds them when the work root is reached through a
  symlink (`/tmp` on macOS): it compared git's real worktree paths with the unresolved root and skipped every task.
- `setup/heavy` no longer reports its own `--when-idle` pause as a FROZEN tree when the pause begins while the watchdog is
  sampling the process table (the marker is now read after the sample, not before): on a loaded machine that raised a
  false FROZEN alert at once with a zero-minute threshold.
- A batch of `go test` partitions that finds the batch lock held no longer dies at once with exit 75: it waits for the
  holder, naming its pid and the bound (`HW_GO_TEST_LOCK_WAIT`, default 3600 s; `0` keeps the old refusal at once), and
  runs when the holder releases it. Measured 2026-11-04: 5 of 16 fast-gate launches ran every subject and then lost the
  whole run to this lock, while `setup/heavy` had already given the gate a slot.
- The Go tests a change touches are spelled the way they run, in `docs/BRIEF-TEMPLATE.md` and in
  `setup/go-test-core.sh --help`: `git diff --name-only main > /tmp/changed` then
  `bash setup/go-test-core.sh --affected /tmp/changed`. The briefs' `--affected <file>` named no file, and a shell read
  `<file>` as a redirect; `hw` now warns when a brief hands `--affected` a placeholder.
- A subject's mutant verdict can no longer false-red on a broken pipe. `printf … | rg -q` under `pipefail` reports the
  writer's EPIPE as the pipeline's status, so a killed mutant read as VACUOUS — measured 2026-11-04: 1150-G11 printed
  “VACUOUS — no test failed” beside `printf: write error: Broken pipe` and passed when run alone. The whole suite matches
  from a here-string now (`has_text`/`has_fixed`, `setup/tests/_common.sh`) — 74 call sites in 35 subjects, every
  `printf … | rg -q|rg -qx|rg -qi|rg -qw|rg -qF|grep -qF` of a captured output (`rg -qx` as `^…$`, `rg -qi` as `(?i)…`, a
  fixed string as `has_fixed`); the only one left is 1150's own reproduction line, and it says so. 47 was the fifth, and
  the gate lost a run to it on the same day.
- `setup/commit-tree` is linear in the paths it is handed. Its case-collision check compared every pair of every
  named arg's directory prefixes, lowercasing both sides with a `tr` fork per comparison: a 344-path commit forked about
  two million of them and ran 32 min without finishing (measured in a wait-free lane). One lowercase → first-original
  pass finds the same collisions and a 344-path commit now commits in 15.9 s; that pass keeps a TAB in a directory name
  and compares strings rather than awk's numbers, so `Ne<TAB>w`/`ne<TAB>w` and `1e5`/`1E5` are still refused
  (`tests/1233` G13b).
- A nested probe can no longer overwrite its parent pane's report. `done-invoker` and `ask-invoker` read the
  `hw_run` herdr records on the pane before publishing anything and refuse (exit 1, no token written) when it is not
  `$HW_RUN`: measured 2026-11-04, a pi probe inherited `HERDR_PANE_ID` and rewrote the live executor's
  `hw_task`/`done_state`/summary, and `hw status` read `done-NOT-delivered` for a pane that was still working. The
  probe recipe in `setup/CLAUDE.md` now says how to start one without the parent's pane environment. Old (check
  removed): rc 0 and both `pane.report_metadata` writes landed; new: rc 1, tokens intact (`pane_run_foreign`
  scenarios in `core/invokers/diff_{done,ask}_test.go`).
- `setup/go-test-core.sh --affected <file> --run` runs exactly the partitions it prints, as the batch they must be
  run as (the batch lock and the plan), instead of leaving the executor to run them one by one by hand — where the
  lock and the parallelism are skipped and a stale golden surfaced only in Judgment Day (measured 2026-11-04). The
  names are still printed, so the machine-readable use (`| paste -sd, -`) keeps working; `--help` carries the
  spelling. `tests/1100` G10 drives the fixture's `a` and `rest` through it.
- `setup/commit-tree`'s case-collision check reads the tree NUL-separated and compares in one python pass. A name
  with a TAB, a newline or a non-ASCII byte is C-quoted by `git ls-tree --name-only`, so `docs/new.txt` beside an
  existing `Docs/é.txt` was committed — the quoted line matched neither the file nor its directory (measured
  2026-11-04). `tests/1233` G17 pins the non-ASCII directory and the TAB-in-file cases; a 344-path commit still
  takes 16 s, not 32 minutes.
- The sandbox parity world is serialized by a lock (`/tmp/hw-sb-world.lock`) instead of colliding. Its fixed path
  cannot move: the state-directory hash derived from it is in the recorded transcripts under `core/testdata/sandbox/`.
  Two concurrent runs made subjects 1040/1041 red in the gate and green alone (~85 min of gates, 2026-11-04). Two
  `1040` runs started at once now pass (154 s, one waiting for the other); at HEAD without the lock the same pair is
  red on both, on different scenarios.
- `done-invoker` warns (never refuses) when a declared Judgment Day's judges are proved but no single call launched
  them together: the transcript proof now keeps each call's group, so two serial judge calls are named. Measured
  2026-11-04 (no-notifications): the two judges ran one after the other twice, ~13 min lost each time.
  `core/invokers/pi_transcript_test.go` holds both fixtures — `pi_judges.jsonl` (serial) warns, one
  `workflow:"parallel"` call (and one Claude assistant message with both Task blocks) does not.
- The executor guard (`setup/guards/deny-executor-sleep-poll.py`, asked by Claude's PreToolUse/Bash hook and by the pi
  adapter) refuses a raw FOREGROUND `go test` whose target is `core/` — `go test ./core/...`, `cd core && go test …`, or
  a bare `go test` from core — and names `setup/go-test-core.sh` (`<partition>`, or `--affected /tmp/changed --run`)
  and the background route. Measured 2026-11-04: 8- and 10-minute `go test` runs held the turn and skipped the batch
  lock. The runner itself, a background run and a non-core target pass. `tests/1133` section j holds it, and the live
  probe (`setup/pi/test-live-gotest-guard.mjs`) shows a real pi refused where the old adapter ran the command.
- A new subject, `tests/1245`, lints every `# gate-when:` header: the regex must compile, and no alternative may
  name a directory a changed path can never be (a trailing `/`, or a fully anchored bare directory — the shape Judge A
  found on 2026-11-05, where a subject silently never ran for the files it claimed). It also runs over this tree's 37
  headers and requires zero flags.
- 1130Ai: the cut's cap is 40 s, not 20 s, so the test no longer goes red under load (measured 3 of 5 red at 20 s with eight concurrent spinners; 5 of 5 green at 40 s).
- The gate's own reds from this task are fixed: the sleep-poll guard no longer spells the lane word `cur` as a local (173), `core/invokers/panerun.go` and `setup/pi/test-live-gotest-guard.mjs` are classified in the export manifest (606), and the `done.go`/`ask.go` citations in `setup/contracts/hw.md` follow the moved lines (815).
- The serial-Judgment-Day warning groups judge calls by the assistant message, not the transcript line: Claude Code writes each tool_use of one parallel message as its own line sharing `message.id`, so a correctly parallel pair was warned as serial (Judgment Day, 2026-11-05). `pi_transcript_test.go` holds the real shape (same id: no warning; two ids: warning).

### Security
- The export's term list refuses the names of the installer and of the memory guard this
  release stopped pointing at, so a cut that leaks either name again fails the fast gate.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.7.0

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
