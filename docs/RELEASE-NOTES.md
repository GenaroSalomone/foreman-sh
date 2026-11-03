# foreman-sh 0.7.0

foreman-sh runs on Go with no bash left to switch to, a pi executor works as natively as a Claude one, and its verdicts lie less.

## In short

- **Bash is retired.** The last eight small tools (the Stop hook, `hw secret`, `hw-sandbox`, `browser-close`, `brain-archive`, `secret-unlock`, `opencode-auto`, the Codex channel hook) run their `hw-core` verb; `hw secret put` fails closed, and a leftover `HW_IMPL_<TOOL>=bash` line says once that Go runs. The way back is `foreman-sh upgrade --to 0.6.3`.
- **pi does it natively.** A pi executor calls the account's Judgment Day judges, runs the plugin review skills, fetches web pages and reads your global rules; `done-invoker` proves its reviews from its own pi session (a management call or an async launch proves nothing). Build and design dispatches default to `deepseek-v4.1-flash`, with `deepseek-v4-pro` on a model error.
- **One pi per executor.** A pi executor with `plugins:` gets a private agent dir under its run, so its packages never touch yours; a dispatch from inside one resolves your real dir. The one shared limit, `auth.json`'s lock, is stated in KNOWN-LIMITATIONS.
- **A preamble half the size.** One renderer writes the executor's prompt at launch and at `hw next` (claude 10.0 → 5.7 KB, pi 9.4 → 5.0 KB), and every lane's own executor rules reach its executors, without displacing a repo's own `.claude`.
- **Verdicts you can trust more.** The incremental verdict parses its markers instead of pattern-matching them, a pre-commit `all` still runs the subjects that guard the change, and 20 of the 90 mutation claims lost with bash are held by Go mutants again.
- **Memory that scales.** A decision-grade `mem_save` without a `topic_key` is refused with how to fix it, the memory audit measures the last 30 days, and `brain relaunch` finds the conversation of a brainer on another account.

## What changed

95 changes since 0.6.3.

### Added
- `setup/mutation/l26-inventory.md`: the 90 retired mutation claims of 0.6.0's deleted bash bodies, classified row by row — 20 now
  mutation-held (12 in 0.7.0, 8 more here), 36 guard branches still owed (each naming the fixture it needs), 22 held by a golden a
  mutant would move with it, 16 bash-only or n/a.
- Go mutants `go-806-M01..M10` (brain relaunch, panic exits and the resume's EXIT trap), `go-67-M06` (chaining default) and
  `go-185-M01` (the speckit phase note must not leak into `--sdd none`), registered in `setup/mutation/mutation-anchors.tsv` and
  killed in process by their corpora.
- Six mutants of the brief's remote-marker guard, `844-A170..A175` (`core/t5_brief.go`), killed by the new contract cases
  `br-au-none` (a body naming `prod` with no `authorizes:`), `br-au-present` (the same body with one), `br-au-product` (a word that
  only contains `prod`) and `br-au-fm-marker` (a marker only in the frontmatter): the warning, its refusal form, its absence when
  the brief authorizes, the manifest's `— but the brief names prod`, the word boundary, and the frontmatter exclusion.
- Two mutants of `hw status`'s reopened run, `826-G35` (the table row) and `826-G36` (the row after its pane is gone), killed by the
  `status-hygiene` case whose golden already carried both `t-reopened` fixtures; `mutants-c825.jsonl` regenerated.
- One mutant of the pi brain-guard manifest line, `844-M51` (`core/t5_manifest.go`), killed by `pi-kind-build`.
- `setup/commit-tree [--onto <ref>] -m <msg>|-F <file> <path>...` commits exactly the paths you name: the tree
  and the parent come from the same ref, the mode is computed (100755 when executable), a path deleted on disk
  is removed, a diff touching a path you did not name is refused with exit 1 and nothing written, and the ref
  moves only if it still holds the commit the tree was read from. It never runs a hook.
- `setup/tests/1233-commit-tree-solo-lo-que-nombra.sh` reproduces the hand-typed recipe deleting a file that
  landed between `read-tree` and `commit-tree`, and holds the helper's behaviour against it.
- `hw status` lists a setup run that reported done until its retro is in engram
  (`topic_key: setup/retro/<task>`, exactly that key; one engram check for the whole
  list, across project labels) — a reminder, never a block. `hw retro-done <task>`
  clears it by hand.
- Executors get their own `TMPDIR` under `$HW_ARTIFACTS/tmp`, created at dispatch for
  every vendor, so fixed `/tmp/<name>` proofs cannot collide across parallel runs. The
  shared Go build cache and the shared lock directory stay shared, and `done-invoker`
  does not count `tmp/` as artifacts.
- `core/main_test.go` refuses a `go test` over core/ that selects the long in-process contracts (TestC848, TestC840,
  TestC870) outside `setup/go-test-core.sh` without their shard cut, and names the script and `--affected <file>`.
- `hw`'s dispatch preflight refuses a brief whose `## Verification` runs `go test` selecting an in-process test.
- A lane's executor rules live in `brain/<lane>/.claude/CLAUDE.md` and `hw` reaches every vendor it can: a symlinked
  `.claude/CLAUDE.md` in every worktree or work directory of the lane (Claude Code), and an
  `--append-system-prompt <file>` on the pi launch line and on `hw revive`. The dispatch manifest and the receipt print
  one `lane_rules` line naming that route, or `NOT APPLIED — <vendor> …` for opencode and codex.
- The lane's `.claude/agents`, `.claude/skills` and `.claude/CLAUDE.md` are linked in a lane with no product repo; their
  paths are named once in the checkout's `.git/info/exclude`, with no lane-specific exclusion code.

### Changed
- The eight small tools run Go with no switch: the Stop hook, `hw secret`, `hw-sandbox`, `browser-close`,
  `brain-archive`, `secret-unlock`, `opencode-auto` and `codex-channel-session-hook.sh` are now shims that exec their
  `hw-core` verb, like `channel-send` since 0.6.0. `HW_IMPL_<TOOL>=bash` or a `<tool>=bash` line is read only to say
  once that bash was removed, and Go runs; it never stops the command. The way back for one of them is
  `foreman-sh upgrade --to 0.6.3`.
- `hw secret put` is in Go: the value still comes from a pipe on stdin, travels to `bw` on its own stdin
  base64-encoded, and never in argv.
- A `mem_save` of `decision`, `architecture`, `discovery`, `bugfix`, `pattern` or `config`
  is refused by `bin/engram-label-proxy` when it carries no `topic_key`, and the refusal says
  how to add one (`mem_suggest_topic_key` proposes it); `session_summary`, `passive`, `manual`
  and every other tool pass untouched.
- `hw status`'s memory audit measures the flow, not the legacy: observations without a
  `topic_key` count only over those six types and the last 30 days (`HW_ENGRAM_NOKEY_DAYS`),
  and the line names the window it measured.
- KNOWN-LIMITATIONS L26 now carries the measured counts and points at the inventory instead of "about 90".
- `TestBrain806EachPaneKeepsItsOwnAccountNeverTheCallers`: the default-account lane's pane now runs under a NON-default
  `CLAUDE_CONFIG_DIR` (a third account directory), so the pane-reading branch `core/brain_launch.go` is the one that decides and
  `go-806-M03` is killed again on a base that carries the lane-account relaunch (`27488505`). The old world let the lane branch decide
  and the default account stay empty, which the mutant survived.
- pi's `build` and `design`/`audit` dispatch kinds default to
  `opencode-go/deepseek-v4.1-flash`, keeping the roster's thinking default, and
  fall back to `opencode-go/deepseek-v4-pro` on a model error; a brief that
  declares no kind is a build and does the same. `mechanical`, which ran the
  build model, now runs `opencode-go/deepseek-v4.1-flash` too (the 2026-10-11
  roster's row for it) and keeps its `glm-5.3-flash` fallback; `review` and
  `explore` keep the models and fallbacks they had.
- `hw`'s dispatch preflight refuses a `kind: build` brief with `review: none` unless it carries a
  `review-skip: <piece> — <why>` line. `docs/BRIEF-TEMPLATE.md` defaults `review:` to `[judgment-day]`, and its
  Verification names `c843` / `c848-1..8` for t5 goldens and mutants instead of `--affected <file>`.
- `foreman-guide` says when goldens are regenerated: at the end, after Judgment Day, from a frozen `hw-core`.
- `_skills/retro/SKILL.md` finds pi runs under `~/.pi/agent/sessions/`, not only
  `~/.claude*/projects/`.
- `setup/CLAUDE.md` names `setup/go-test-core.sh` (`<partition>` or `--affected`) as the
  only way to run Go tests; `docs/BRIEF-TEMPLATE.md` points to it.
- The invoker env-file whitelist adopts `HW_LOCK_DIR`.
- A pi executor registers the account's user-level agents (`$CLAUDE_CONFIG_DIR/agents`, default
  `~/.claude/agents`) with pi-subagents next to the lane's, so `subagent` with agent `jd-judge-a`,
  `jd-judge-b` or `jd-fix-agent` works as it does for a Claude executor. The lane's agent of the same
  name shadows the account's. The `jd-` agents are no longer filtered out of that registration.
- A pi specialist that lists `WebFetch` gets pi-web-access's `fetch_content` (the child loads that
  package), where it used to lose it. `WebSearch` stays dropped: not measured working.
- A pi executor starts with the account's `CLAUDE.md` as a context file of its run (pi's own slot for
  user instructions; nothing is written under `~/.pi`), and the receipt names the file (`pi_global_rules`).
- `done-invoker`'s pi proof counts a `subagent` call only when it ran something: no `action` (get, list,
  status, update… are management), `async:false` (pi-subagents runs async by default, and an async call's
  result only says it was launched), and agent + task for a single call, each element of `tasks`, and each
  step (and `parallel` step) of `chain`; `workflow:"review"` proves the `reviewer` agent. `workflow:true` or
  a script path proves nothing foreman can read: `review-skip` is the exit. A `chain` step without a task is
  not counted either (a stated limit).
- The pi session is accepted only when its header `cwd` is the run's workdir, and a session file that cannot
  be opened is reported as unverified instead of as a smaller proof. The refusal for a pi session names pi's
  mechanism (the `SKILL.md` read, `subagent` with agent, task and `async:false`).
- The dry run's `pi specialists` line reports the agents registered from the account's directory as well as
  the lane's, each with its directory, and appears when only the account's directory has agents.
- Known limit (measured, pi 1.4.0 / pi-subagents 0.77.0): a pi child still has no `Task` and no `mcp__*` tools. MCP for children needs background children, which
  need `@earendil-works/chord` in the host pi.
- The plugin-review refusal describes the check in vendor-neutral terms, and each vendor's exact rule is
  documented next to its reader.
- The text hw puts before the brief in every executor prompt is shorter and
  per-executor: each executor reads only the blocks that apply to it (its
  vendor, its lane, what the brief grants, how the task closes). Measured on a
  pi setup build: 9,428 -> 4,996 bytes of preamble; on a claude build with
  QA: 9,964 -> 5,693 bytes.
- A re-tasked executor (`hw next`) now gets the same text a fresh launch gets:
  a pi executor hears the pi specialists note, and a brief with `secrets:`
  gets the grants block. It also saves its findings under its own topic key
  (`<lane>/<task>-t2/findings`), so a second task no longer overwrites the
  first task's findings.
- `done-invoker` and `ask-invoker` count characters, not bytes, against their
  900 and 600 limits, so accented text is no longer refused early. The prompt
  now states the limit.
- `done-invoker --blocked` followed by a plain report now tells the executor to
  wait for `hw ruling` instead of suggesting `--retask`.
- `hw review-delta` says what to do on every refusal instead of only exiting.
- The `checks.md` reminder in `done-invoker` names what to list: one row per
  requirement of the brief's Verification section and each numbered goal.
- A setup executor's prompt again says to run the fast gate
  (`HW_TEST_GATE=fast bash setup/test-hw`) before closing unless the brief's
  Limits forbid it, and that a red it prints is the executor's to fix.
- A pi executor is told its Judgment Day judges are the registered `jd-judge-a`
  and `jd-judge-b`, and that every subagent call to a judge or plugin review
  agent carries `agent`, `task` and `async:false`, or the close does not count
  it as proof.
- `--sdd` is optional and `none` is the default: a dispatch without it runs outside any
  framework, even where Spec Kit's `speckit-*` skills are on disk (it used to refuse there
  and ask for a mode). `--sdd speckit` stays, opt-in, and is still refused where its skills
  are not on disk or the vendor is not Claude Code.
- An OpenCode or Codex dispatch without `--sdd` on a repository that has Spec Kit installed
  is no longer refused as if it had asked for Spec Kit, and an OpenCode Sol or Astra
  dispatch without `--sdd` gets its orchestrator agent, as `--sdd none` always did.
- `setup/opencode-hw-*.js` starts `hw` and `herdr-rpc` through the same `execFileP` on every platform: the Git
  Bash shell lookup and its 10x timeout are gone. macOS and Linux behaviour is unchanged.

### Removed
- The bash bodies of the eight small tools, `bin/wrapper-go.sh`, and the per-tool bash switch they implemented.
  Their subjects now hold the Go against the transcripts the bash recorded (`core/testdata/wrappers/`,
  `core/testdata/sandbox/`, `setup/tests/hw-secret-parity/golden/`), and the bash-only mutation anchors left
  `setup/mutation/mutation-anchors.tsv` for their Go holders.
- The duplicate `HAND=` ports of `setup/suite-data/go-test-partitions.txt` no gate ran: `TestC847`, `TestC847Mutants`,
  `TestInProcC862_866`, `TestInProcC862_866Mutants` and `TestC843Mutants`, with their 1,400 lines of test files.
  `TestC844Contract` stays: it is the entry point the live `TestC844Mutants` spawns in the mutated binary.
- The Windows code, ruled out 2026-10-16: the `core/*_windows.go` files, `core/invokers/*_windows.go`,
  `bin/msys-compat.sh` and its source lines, `setup/mutation/mutation-exempt-native-windows.tsv` and
  `.github/workflows/windows.yml`. Subjects 208 and 609 stay: they force the Windows path model on POSIX and
  still cover branches the tool keeps.
- The "this task runs outside the framework flow" paragraph is no longer prepended to a
  `--sdd none` executor's prompt; the unattended-pane paragraph stays.
- More of the Windows code the 0.7.0 deletion left behind: `bin/sitecustomize.py` (the msys path layer)
  with its export, deps and test references, the `MINGW*|MSYS*|CYGWIN*` branch and the `cygpath` fallback in
  `install.sh` (a native Windows shell now gets the same "not supported" answer as any unsupported platform),
  the `.exe` lookup stanza of the eleven `bin/` entry points, the Windows-only CI helpers (`setup/ci/`), the
  native-Windows mutation exemption with its deleted list, and the dead `runtime.GOOS == "windows"` branches in
  `core/` and the `msys-compat` comments that outlived the file they named.
- The Windows-only runtime: `bin/herdr-rpc`'s named-pipe transport and the `os.name == "nt"` selection,
  the pipe half of `setup/tests/_herdr_endpoint.py`, the Windows branch of `bin/invoker-common.sh`'s
  `usable()`, the Git Bash special cases in `setup/guards/deny_repo_writes.py`, `setup/test-hw`'s
  `taskkill` witness cleanup, the Job Object/`taskkill` cap in `setup/guards/lane_housekeeping.py` and
  subject 640's no-`killpg` arm that tested it, the `msys`/`cygwin` branches of subjects 153, 176, 186,
  199, 217, 251 and 1123, and the comments that cited them in `_common.sh`, `install.sh`,
  `bin/engram-serve.sh` and the fragments. `setup/tests/contract-hw/native.sh` (Windows-only) and its
  `go-test-deps.txt` line are gone.
- The read-only guard refuses at import on every platform but macOS and Linux (a Cygwin/MSYS Python
  answers `os.name == "posix"` and read a `C:\…` write as a relative, allowed path), and its JavaScript
  half no longer resolves `/usr/...` through `HW_MSYS_ROOT`.
- What deliberately stays: the native-Windows path model — `core/worktrees.go`'s
  `HW_CORE_PATH_RULES=windows|posix` and the guard's spelling folding that subject 208 forces with
  `DENY_REPO_WRITES_AS_NATIVE_WINDOWS=1`. The fast-gate ceiling's own Windows branch also stays, and
  subject 609 forces it with a simulated `win32`; it is process control, not a path model.

### Fixed
- `hw secret put` fails closed: when it cannot read the existing item from the vault it stops
  with an error instead of storing the new value over it.
- `brain relaunch` carries the lane's account: a brainer whose lane declares `account: personal`
  is resumed with the lane's `CLAUDE_CONFIG_DIR` and finds its conversation, even when its pane
  shell lacks that variable (before, `brain relaunch --all` left such brainers as bare shells
  printing «No conversation found with session ID»). The transcript lookup tries the lane's
  account first.
- The incremental verdict reads a `# MUTATION-ANCHOR:` marker as a declaration only inside a real shell (or Python) comment: inside a heredoc body, inside a string that spans lines, or on a `\` continuation it is text, so the plan now pays a full run instead of trusting a change that moved the answer.
- A pre-commit `all` gate, and a `mutants-*`-only commit, now run every slow subject whose `# gate-when:` matches that path or whose `# gate-reference:` names it (a `setup/tests/_common.sh` change brings eleven, about four minutes; any `core/*.go` change brings 1014 and 540): `setup/gate-select` makes its gate-when pass before answering `all`, and `setup/test-hw` reads the `reference` line beside it (subject 540, the mutation-anchor registry).
- The mutation-anchor subject's orphan scan covers non-subject files under `setup/tests/` (`_common.sh`, the `contract-hw` helpers): an unregistered marker there is refused instead of accepted, by the full run: 540's `# gate-when:` names no `setup/tests/` helper, so a commit touching only those does not run it at pre-commit.
- A pi executor can run a pstack review agent again. pstack's `agent` tool starts its children with extension
  discovery ON, so a child found the ambient `rpiv-ask-user-question` package and its `ask_user_question` collided
  with the one the vendored pstack extension registers (`setup/vendor/plugins/pstack/pi/ask.ts`): the child died at
  startup and `pstack:comment-sicko` never ran. A pi dispatch whose brief declares `plugins:` now runs on a private
  agent directory under `$HW_ARTIFACTS/pi-agent` — the real one's files symlinked, `settings.json` copied without
  `packages` or `extensions` — so a plugin's children discover nothing ambient. `~/.pi` and a plain pi session are
  untouched, and a session with no plugin keeps the ambient directory. `core/t6_pi_shadow.go`; the dry run's
  `pi agent dir` line says which one a launch gets.
- That private directory is this run's own: a dispatch from inside another hw run no longer puts it in the parent's
  `.artifacts` (where the parent's `hw done` deleted it under the child). It carries no lock directories, and a real
  `settings.json` that is not valid JSON still gets the private one with no packages instead of the ambient directory.
- A dispatch made from inside such an executor no longer builds its private directory out of the parent's: the real
  agent directory travels beside the private one (`HW_PI_REAL_AGENT_DIR`) and `piAgentDir` prefers it, and a child
  without plugins is handed the real directory instead of inheriting the parent's (which the parent's `hw done` deletes).
  Preparing a reused private directory also drops links to names it now skips (`auth.json.lock`), which pi would
  otherwise lock through.
- `HW_PI_REAL_AGENT_DIR` is trusted only when the inherited `PI_CODING_AGENT_DIR` is a run's private directory and the
  value is a readable directory; an explicit `PI_CODING_AGENT_DIR` always wins over a stale variable. If the private
  directory cannot be made, the real directory is exported (never an unchecked value) and the warning names the
  directory pi gets. A non-pi child of a shadowed parent gets the real directory and no `HW_PI_REAL_AGENT_DIR`; a
  revive, a re-task and `hw --sandbox` resolve pi's directory the same way.
- Known limitation: the private directory links `auth.json`, but pi locks it at the link's own path (`auth-storage.js:50`, `:91-92`,
  `realpath: false`) and offers no way to point auth elsewhere (`config.js` `getAuthPath`), so the lock does not exclude
  your own pi sessions' token refresh: two simultaneous OAuth refreshes can invalidate the rotated refresh token and log
  one session out (`invalid_grant`), not only "the later write wins". The file stays one (pi writes it in place, `auth-storage.js:71`, `:147`).
  `KNOWN-LIMITATIONS.md`, L3b.
- `setup/go-test-core.sh --affected` refuses a source file (`*.go`, `*.sh`, or lines with spaces/braces/parens/
  quotes) with exit 2 naming the mistake; a real changed-paths list that reaches nothing (docs only) is still an
  empty answer, rc 0. `hw`'s brief preflight refuses the same shape in `## Verification`.
- A brief that names `brain/setup/CLAUDE.md` gets a warning that inside a setup worktree the lane file is
  `setup/CLAUDE.md`; `docs/BRIEF-TEMPLATE.md` now points at the worktree-relative path and at `setup/commit-tree`.
- `setup/tests/1101-cada-caso-de-golden-t5-lo-corre-el-gate.sh` holds that every `golden-t5` case is exercised by a
  gate partition (`c843`), so the contract's goldens cannot sit outside the gate.
- The contract worlds no longer depend on the ambient `TMPDIR`: they are made under a fixed root (the suite's `~/.cache/hw-test-<uid>`), so a long `TMPDIR` (every executor's) no longer moves the byte counts the goldens record (`c843` was 58 red).
- `regen --review` streams the diff instead of rewriting paths in a captured variable (bash 3.2 was super-linear: a 9 MB diff did not finish in 60 s, now 0.7 s), and prints `case i/N` on stderr.
- Mutant `844-R142b` guards the direction R142 lost: a global speckit install IS detected (`speckit (global)`), with the new t5 case `rt-reuse-nosdd-home`.
- The read-only guard's refusal on cygwin/msys/win32 no longer yields to `DENY_REPO_WRITES_AS_NATIVE_WINDOWS=1`
  (Python and JavaScript halves): on a supported host the variable changed nothing, and on an unsupported one it
  turned the refusal off. It only forces the native-Windows folding on macOS/Linux now (subject 208). The guard's
  test asserts the import still refuses under `sys.platform=cygwin` with the variable set.
- `setup/commit-tree`'s Python syntax check parses with `python3 -I … ast.parse` instead of
  `python3 -m py_compile`, which imported `py_compile`/`traceback` from the repo root or `PYTHONPATH` and ran them.
  It selects files like `setup/hooks/pre-commit` (`#!…python`, `#!/bin/sh`, `#!/usr/bin/env sh` too) and skips
  symlinks (subject 1233, G16).
- The in-process harness's TZ fix has a wiring guard: `TestRunGuardedHandsTheCaseItsTZ` goes through `runGuarded`
  with `time.Local` pre-set to a non-UTC zone and asserts a `TZ=UTC` case sees UTC (it fails with the
  `defer inprocTZ(getenv)()` deleted). The dead `getenv == nil` branch and the long comment are gone.
- `setup/tests/contract-hw/regen*.sh` run under the suite's `TMPDIR` (the goldens record byte counts that include
  absolute paths), `world.sh` defaults `HW_GOCACHE_DEFAULT` so a regeneration no longer dies unset, and a case (or
  set, for `regen.sh`) name nobody has exits 2 naming it instead of returning 0 having regenerated nothing
  (subject 1000, G05 and G06).
- `setup/suite-data/test-budgets.json` carries subject 1101's budget (4.39 s, measured through `setup/heavy`), so
  pre-commit stops refusing every commit for the missing entry.
- `setup/tests/contract-hw/golden-t4` is regenerated from the tree's `hw-core` for the 52 cases that had gone stale:
  the `--sdd` help and usage text (`done-usage-bare`, `preview-help-long`) and the world listing of 50 cases whose
  `tree` diffs moved by two lines. `setup/go-test-core.sh t4-1` is green again.
- The in-process contract harness gives each case its `TZ`, so a
  `ruling --dry-run` prints the world's zone and not whichever zone the
  process happened to initialise `time.Local` with first (corpus 840's three
  `rl-blocked-* --dry-run` cases differed from the golden in a whole-package
  run only).
- Corpus 870's outbox world stamps the two `alpha` worktree envelopes like
  every other run, so the listing's order no longer follows the wall-clock
  second the template was built in (`ob-all`, `ob-list`, `ob-alpha`,
  `ob-alias` and `ob-to-value` could differ from the golden in `out`).
- A pi executor's plugin review is proved from its own session (`session_vendor=pi`). A skill is a read of its
  `SKILL.md` under `<plugins>/<p>/skills/<n>/SKILL.md` (proving `p:n` and `n`) or `.claude/skills/<n>/SKILL.md`
  (proving `n`), a partial read included; an agent piece is a `subagent`/`Agent` call of that exact name. A call
  counts only once its own `toolResult` arrived without an error, so an unknown agent proves nothing. A skill named
  only in prose is not a run, and only what ran at or after the receipt line counts. The session is the file's path
  the receipt records, read only as a literal `.jsonl` regular file under pi's sessions root
  (`$PI_CODING_AGENT_DIR/sessions` when set, else `~/.pi/agent/sessions/`); other plugin layouts are not recognised
  and need `review-skip`.
- A pi executor's plugin review is no longer credited by a call that failed: a skill read or an agent launch
  counts only once pi's own result for that call arrived without an error. A session whose unknown-agent call
  failed used to prove that agent had run.
- A pi skill is proved only by the two `SKILL.md` layouts foreman uses,
  `plugins/<plugin>/skills/<name>/SKILL.md` and `.claude/skills/<name>/SKILL.md`; any other `SKILL.md` proves
  nothing. A partial read still counts.
- The pi session foreman reads is the file path the receipt records, under the executor's own pi agent directory
  (`$PI_CODING_AGENT_DIR/sessions` when set, else `~/.pi/agent/sessions`), including every run of its subagents.
- An unreadable pi session is reported as unverified instead of reading as a session that ran nothing.
- `done-invoker` and `ask-invoker` run from a brand-new worktree: they use the
  main checkout's built invokers instead of failing with "the Go invokers are
  not built".
- `done-invoker --help` and `ask-invoker --help` print their usage and exit 0
  instead of refusing "unknown option: --help".
- `hw next --brief` names the re-task by its own brief (`<lane>/<brief>-t2`) in
  the prompt header and the findings topic key, not by the first task.
- `hw next` reads a brief's `secrets:` with the same parser the launch uses; an
  item that starts with `-` is warned about and left out instead of silently
  kept.
- A lane with its own repo no longer has its `.claude` replaced: a repo's own worktree script links
  `.claude/{CLAUDE.md,agents,skills}` from its checkout and `hw` used to relink them to the brain's, so the executor
  lost the repo's specialists and skills. `hw` now removes a symlink only
  when it points under `brain/<lane>/.claude` (one it made); any other entry is kept and reported. Such a lane gets only
  `.claude/CLAUDE.md`, and only where the worktree has none; agents and skills stay linked only where they were before
  (lanes with no product repo).
- `info/exclude` names an entry only when this run made (or already owns) its link and git tracks nothing at that path,
  so new files in a tracked `.claude/skills` still show in `git status`. The patterns come from the one list of linked
  entries.
- The receipt's `lane_rules` says `KEPT (repo has its own)` when the worktree kept its `.claude/CLAUDE.md`, instead of
  `linked`.
- setup links under `setup/.claude` of its worktree (the worktree is the brain's), not the root's.
- An executor's per-run `TMPDIR` is now `<cache>/hw-run-tmp/<run>` (`$XDG_CACHE_HOME`, else
  `~/.cache`), outside every git repo: under `$HW_ARTIFACTS/tmp` a "plain directory" built
  there found the worktree's repo, which made five Go tests red inside an executor and green
  in a clean env. `hw done` and `hw reap --apply` remove the directory with the worktree, and `hw reap --apply` with a loose (plain) work dir; the path removed is the one the run recorded in `.hw/<run>/env` (when it is absolute, named for the run and under `hw-run-tmp`), so a closer with another `HOME`/`XDG_CACHE_HOME` removes the right one. The
  shared Go cache stays shared under it (`gocache_default`).
- `newCSWorld` scrubs `HW_LOCK_DIR`; `TestDiffDone` waits for its detached closer by event
  (after 60 s without it the snapshot is compared to the golden as it is, and the missing close shows as a golden diff); the 844-A88/A89/R141 mutants are anchored again; `regen
  --review` treats a closed pipe as "differs"; an empty `HOME` no longer creates `core/.cache`.
- A revive whose recorded `TMPDIR` vanished recreates it (0700) before arming the sandbox; the `/tmp/hw-run-tmp` fallback root is made 0700.
- Review fixes (rest-limpio-fix2-070): revive creates only a TMPDIR `recordedRunTmpdir` accepts (a recorded `TMPDIR='scratch'` is no longer created under hw's cwd; a failed create is warned); every work-dir removal goes through `removeWithRunTmps`, which makes the scratch writable first (Go's module cache is read-only) and warns when it cannot remove it; `/tmp/hw-run-tmp` is one const and a failed chmod of it is warned.
- Four reds on `main` before 0.7.0: mutants 844-M23 and 844-R142 and 1020-G01 were stale after `--sdd` became optional
  (fcff4221, the "say which" refusal is gone) and `kind: build` + `review: none` became a refusal (bcab9b97); each is
  re-anchored on the code that now carries its claim. `setup/commit-tree` is classified (shipped) in the export manifest.
- The names-only exemption rule in `setup/tests/46-mutation-arms-are-not-gated.sh` is asserted again off macOS:
  a row with an empty reason, or `*` where an arm id belongs, exempts nothing. Both shapes were lost with the
  removed native-Windows exemption list, though the shared `exempt_arms()` kept the rule.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.6.3

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
