---
# OPTIONAL, and checked. Declare only the fields this task actually requires;
# hw compares each one against the resolved dispatch and REFUSES before it
# builds anything if they disagree. Omit the block entirely and nothing
# changes — the manifest just records that the brief declared nothing.
#
#   agent:     claude | opencode | codex
#   model:     the vendor's own form (opencode needs provider/model)
#   effort:    low | medium | high | xhigh | max (codex also ultra; pi also off | minimal) — --effort's own levels,
#              refused in --effort's own words when the vendor cannot take it. A CHECK, like model:, never a source:
#              the flag (or pi's roster default) still chooses, and a brief that says high on a dispatch that
#              resolves low is refused before anything is built. A brief that declares neither model: nor effort:
#              gets one warning naming both (never a refusal).
#   sdd:       none (the default; omit it) | speckit (opt-in, needs --sdd speckit)
#   base:      the branch the worktree is built on — declare it on any task
#              whose result is measured
#   requested_by: "<who, date: the request, quoted>"  and  asked: investigate | build
#   release: prod   a brief that writes prod data or deploys to prod. REQUIRES env-from: and env-to: (app uuids),
#              env-diff: {NAME: needed|not-needed} (one ruling per name that differs, no more), screens: [<name>, …]
#              (done needs $HW_ARTIFACTS/screens/<name>.png newer than the run start; sub-views are fine,
#              e.g. route-detail/per-driver-stops) and qa: <brief> (hw dispatches it with the release; the close waits for
#              it to report). And an authorizes: line that covers the read: the SSH host, the env list of each app and
#              printenv in each backend container, both app uuids. hw diffs the two envs at dispatch and again at close;
#              a name ruled needed that still differs refuses the close. Put the precheck that PRINTS the flags in the
#              brief's `## Testing / Verification` (a template rule, unenforced). Dispatched, never `hw next`.
#   rehearsal-of: <release brief> (+ its own env-from:) refused unless that env matches the release's env-to, or every
#              difference is ruled not-needed there.
#              What was asked for. hw refuses a dispatch without it where the lane requires one and
#              warns elsewhere; `investigate` does not implement.
#   authorizes: <destination> :: <operation> :: <Keychain handle|none>
#              A remote operation the operator authorized, one entry per `authorizes:` line (a `;` does
#              not separate entries). The third field is a Keychain handle, or `none`; a Bitwarden
#              item is not a handle: it goes in `secrets:`.
#   secrets:   [<item>, …]
#              Bitwarden items (folder `agents`) the task needs, by name, never a value; hw checks
#              each is there before building. See `## Access`.
#   placement: project-tab | legacy-space | here
#   worktree:  new | none
#   database:  own | main
#   invoker:   the brainer pane that must receive the report
#   kind:      build | explore | audit | review
#              What the task IS. A model the lane marks build-only
#              is REFUSED for explore/audit/review: exploration decides
#              what to build, and the model meant for building is the
#              wrong one to decide it. Declaring `build` also silences
#              the name heuristic for a task like `fix-the-audit-script`.
#   requires:  browser | subagents | repo | database  (comma-separated)
#              What the task NEEDS. hw warns — never refuses — when the
#              target may not have it, because it knows the vendor and not
#              what that session can reach. A warning before the launch
#              beats finding out at minute 40.
#
#   sandbox:   true — run the executor inside the macOS sandbox, as --sandbox
#              (Claude Code on macOS only; docs/THREAT-MODEL.md)
#   requires-agents: [primary:|subagent:]<name>, …
#              Agents the task cannot do without: hw refuses the dispatch
#              when the executor cannot reach one, instead of warning.
#   review:    on a `kind: build` (or no kind) EVERY value owes the independent review — the key
#              absent, judgment-day | plugins | rdd, ONE list [judgment-day, thermo, ponytail,
#              pstack, tests], or a bare `none`. The one opt-out is `review: none` PLUS a
#              `review-skip: <why>` frontmatter line (the reason lands in the manifest); a bare
#              `review: none` still owes the review and warns for one release (refused after it).
#              A task of another kind (explore, audit, review, design) owes none: no key = no review.
#              How it runs: when the build reports done, `hw review <lane> <task>` dispatches a fresh
#              claude opus reviewer (`kind: review`) on the build's head. Its tier is the size of the
#              diff in code-only lines (docs/ except REVIEW-BRIEF/BRIEF-TEMPLATE, decisions/, briefs/,
#              changelog/ and changelog fragments do not count): 0 = nothing to review; 1-400 = S: thermo +
#              test-review; over 400 = L: those plus ponytail-review and the two pstack principles.
#              What `review:` adds to the reviewer, never to the builder:
#              judgment-day (or rdd, which is retired and warns) = the reviewer runs Judgment Day:
#              write it where the diff sends mail or messages, moves money, or writes or migrates
#              production data (the brainer marks high risk; nothing guesses it); a list's plugin items
#              (thermo, ponytail, pstack, tests) and `plugins` (the scalar: the review pieces of
#              `plugins:`) join the reviewer's tier, each piece once: thermo = maintainability;
#              ponytail = senior review (correct, safe, load, tested, fast, lean); tests = foreman's
#              own test-review skill; pstack = in the reviewer, its two principles
#              (minimize-reader-load, subtract-before-you-add), not the four reviewers it carries in a
#              task's own close. The reviewer's done-invoker proves each piece from its transcript.
#              The BUILDER runs no Judgment Day and no review skill. It closes with its own OLD/NEW
#              tests (a brief that changes behaviour says so under Verification, below) and, on claude,
#              `/simplify` (proved by the Skill call; pi: pstack:deslop if loaded, unproved; codex and
#              opencode: not available). A build that owes the review launches with --keep-pane by
#              default: the pane is there for the one fix round.
#              The merge: `hw done` offers «Merge it» only for 0 code lines at the head, `review: none` +
#              `review-skip:`, a reviewer's `verdict: approve` at exactly the branch head, or the
#              brainer's `hw done <lane> <task> --accept-review "<why>"` (refused inside an executor).
#              Otherwise it prints NOT MERGEABLE. A product lane's build opens its PR as a draft.
#              A non-build task that declares `review:` keeps the old close: judgment-day orders Judgment
#              Day at the close, plugins the `plugins:` review pieces, and the executor proves each, or
#              carries `review-skip: <piece> — <why>`.
#              Any other value or item is refused before anything is built. Keep `plugins:` (below)
#              for plugins that are not review.
#   plugins:   [thermo, ponytail, pstack, tests, webguide]  (any subset)
#              Review plugins (pinned, complete: their hooks run in THIS executor only) loaded
#              into it: claude gets one --plugin-dir each (hook writes go to $HW_ARTIFACTS/plugin-state),
#              pi one -e <plugin dir> each, opencode and codex get NOT APPLIED. They are an
#              OPTIONAL install step, off by default (setup/vendor/plugins/install, or
#              install.sh --with-review-plugins): a name that is not installed is refused
#              before anything is built, naming that command. Nothing is installed globally
#              and nothing is written to ~/.claude or ~/.pi. A re-task (`hw next`) cannot add one.
#              `webguide` (Chrome's modern web guidance) is guidance loaded at build time, not a review
#              piece: name it here on a brief that touches UI, never in `review:`.
#   boundary:  <what must never cross> — the task is not done without an
#              approved design judgment (docs/INSTALL.md, Judgment Day)
#
# A declaration is a CHECK, not a source: it never supplies a flag hw would
# otherwise ask for, except `sandbox: true`, which is the flag. The brief keys
# are also in `hw help flags`.
---

# Brief: <task-name>

## Goal
<One sentence. What is true when this is done that is not true now.>

## Context
<What the agent cannot infer from the code alone: history, prior attempts,
constraints that live in someone's head.>

<One line per premise the task rests on, so a measured fact cannot pass for an assumed one:>
Premisa: <claim> | <fuente> | medida o asumida
<Before an explore brief: `decisions index <lane> --all` for the idea, so a ruling that rules it out is found first.>
<A file the brief needs the executor to open (a screenshot, a fixture) lives in `brain/<lane>/briefs/assets/` and the brief names it as `briefs/assets/<file>`: `hw` refuses the dispatch if it does not exist and copies it to `$HW_ARTIFACTS/brief-assets/`, because the brain guard blocks the executor from opening the brain path.>

## In scope
- <thing>

## Out of scope
- <what stays untouched> — <why: who owns it, or what breaks if it moves>

## Limits
- The smallest change that meets Done when: delete or reuse before adding; no new flag, alias or switch unless a Done-when rule needs it; report the diff size per item; run the reviews after the scope is final.

## Done when
<A build: first the RULES the behaviour must hold on real data, and what the feature is FOR — the judges and the executor judge against these, not only against the code. Example:>
- [ ] Rule: a past route-day is judged by the plan that existed that day, never a later one. Purpose: the audit shows what dispatch actually ran.
<A rule that can only break on real data also goes in `## Testing / Verification` as a check that runs on that data.>
- [ ] <observable, checkable condition>
- [ ] <A build with a UI change only: product-judge ACCEPT, at most 3 rounds; after the third, `done-invoker --blocked` naming what is still rejected.>

## Testing / Verification

<PUT THE COMMAND IN A FENCE. `hw` extracts it from this section, pins it at
dispatch into `.hw/<run>/verify`, and RUNS IT ITSELF in `hw done` — the exit
code and a hash of the output go to the receipt, and `hw receipt <project>
<task>` shows them beside what was dispatched.>

```
git diff --name-only "$(git merge-base HEAD main)" > "$HW_ARTIFACTS/changed"; HW_TEST_CHANGED="$HW_ARTIFACTS/changed" HW_TEST_GATE=fast bash setup/test-hw --keep-going
```

<Step one of two, and the only line the fence pins: the touched subjects (minutes; the Go that changed is a focused `go test -run`, below),
which `hw done` re-runs under its per-line timeout. Step two, the full fast gate
`HW_TEST_GATE=fast bash setup/test-hw --keep-going`, is the BRAINER's, once, on the merge tree: the executor
does not run it. It stays out of the fence because a two-line pin loses `hw done`'s cached-verdict shortcut
and gives the hour-long gate the same 900 s ceiling. The wording and the why live in
`setup/CLAUDE.md` ("A setup executor verifies…"). The gate reads a private snapshot of the tree taken when
it starts, before its first test in every mode, so edits made after that do not touch its run: never wait
for a gate before editing.>

<A behaviour change adds one line here: «OLD: the new test fails on the old code (paste the failure); NEW: it passes on the new (paste the run)». The reviewer's tier already runs test-review (S and L); list `tests` only on a non-build close.>

<Only a fenced block or a 4-space-indented block counts. Inline backticks are
prose: in these briefs a `path` and half a sentence of `VAR=1 VAR=2` are spelled
exactly like a command, so hw does not read them — it would be executing prose.

A heavy command (a subject, the fast gate, `go test` of a package, a release cut) runs as
`setup/heavy -- <cmd>`: two slots, first come first served, the holders shown. A focused
`-run` test of seconds runs as `setup/heavy --short -- <cmd>` — the short lane goes beside a
held slot (one short job at a time, capped at 120 s), so it does not queue behind an hour-long
run. Never a longer job there: anything that can pass 120 s takes a slot. Write
that and nothing else about the machine: a `pgrep -f` count or a hand-typed lock
deadlocks the executors, and `hw` refuses a brief that carries the `pgrep` one.

The full suite already includes every Go partition (`setup/go-test-core.sh`): do not
run them before it, and never a bare `go test ./...` over `core/` with `HW_INPROC_ALL=1`
(it starts the monolithic TestC848 and never finishes; `core/main_test.go` exits 2 on it).
How to run Go tests: `setup/CLAUDE.md`, "Go tests go through the runner".
A whole partition (`setup/go-test-core.sh <name>`, `--list` names them) is the brainer's: inside an
executor the runner exits 75 for any partition, list or `--affected --run` over `touched_budget_seconds`, and
the executor runs a focused `go test -run` instead (below). For t5 goldens and t5 mutants `--affected` is no map:
it does not know a golden case, and a change that moves one reaches no partition it lists. The executor runs the
focused test of the case; the brainer names the partitions out loud — `c843` for the goldens, `c848-1` … `c848-8` for the mutants.

The Go a change touches, as ONE focused test (a whole partition is the brainer's), run in the background:

```
setup/heavy --short -- sh -c 'cd core && go test -count=1 -run <TestName> .'
```

A cold `go test` (low memory, a fresh build cache) can pass the short lane's 120 s cap and exits 124 there: rerun it without `--short`. `--max <s>` only lowers the cap (above 120 it is clamped).

What `--affected` reaches, spelled so it runs (only while the estimate is under the budget; over it, the line above):

```
git diff --name-only "$(git merge-base HEAD main)" > "$HW_ARTIFACTS/changed"
bash setup/go-test-core.sh --affected "$HW_ARTIFACTS/changed" --run
```

`--affected` takes a FILE OF CHANGED PATHS, one per line — never a source file, and never a
placeholder. Written `--affected <file>` it is not the interface: `<file>` names nothing on disk,
and a shell reads the angle brackets as a redirect. Handed a real source file it refuses (exit 2);
`setup/go-test-core.sh --help` carries the same interface (its example still spells the changed file as `/tmp/changed`; use `$HW_ARTIFACTS/changed`).

Verify with the touched subjects and a focused `go test -run` only; the full fast gate is the brainer's, once on the merge tree
(`setup/CLAUDE.md`, "A setup executor verifies…"). Not repeated here.

Before the gate, also run `bash setup/test-budget-gate` — it answers a new subject's missing 1:1 budget entry in seconds,
where the gate would spend a heavy slot to say the same thing. When
a commit's hook is the red, its full gate output is in `$(git rev-parse --git-common-dir)/hw-precommit-last.out`.
Run `setup/export/check` before `setup/commit-tree`: it refuses an unclassified manifest entry or a private term in seconds, instead of at the commit.
For an old/new proof, edit through `setup/mutation/safe-mutate <file> -- <cmd>` (restores the file byte for byte), not `git show HEAD:x > x` by hand.

A brief about TIME (a budget, a wait, a speed-up) names a deterministic proxy first — a count of
processes, calls or files — and a wall-clock number only as the second measure: seconds move with the
machine's load, a count does not.
A timing task starts its run with `setup/heavy --quiet <max-load1> -- <cmd>`, records the load1 `heavy.log` prints for it, and runs a control (the base commit) in the same hour.

Anything else here — which directions to run it in, what a fixture must pin, how
to reproduce the old behaviour — is for the executor and hw never touches it.

Leave the fence out and hw says so at dispatch and again at close: the manifest
reads `verify none — the brief has no runnable command`, and the receipt records
that nothing was measured. It is not a gate and it does not block anything. It
just means the report is the only claim there is, which is what it was on
2026-08-25 when three executors in a row shipped correct code with a
verification nobody had run.>

## Committing
<Commit per work unit as you go, and write the changelog fragment
(`setup/export/overlay/changes/task-<task>.md`) in the same commit as the behaviour.
Every path you name is RELATIVE TO THE WORKTREE, and the worktree is the only tree
you commit in — a path under the brain root names the brainer's checkout, which
is not yours to edit.

ORDER WHEN THE CLOSE HAS A JUDGMENT DAY (a non-build task that declared it; a build's runs in its
reviewer): commit, then the judges, then the fix rounds, and only then the touched-subject verification — on the FINAL tree. (The full fast gate is the
brainer's.)

A NEW TOP-LEVEL PATH (a directory or file at the repo root) needs its `include`/`exclude` line in `setup/export/manifest` in the SAME commit: `setup/commit-tree` runs `setup/export/check` and refuses an unclassified path.

Commit with `setup/commit-tree`: it takes the tree and the parent from the same ref, refuses a diff that
touches a path you did not name, and runs no hook (but it does ask `setup/export/check`, in seconds, over the tree it commits). `setup/commit-tree -m "<msg>" <path>...` on your
branch; `--onto <base>` to commit onto a branch that is not HEAD; `-F <file>` for a long message. Never
`git add -A`, never `git commit` in a shared tree, never `--no-verify`. The lane rules are `<lane>/CLAUDE.md`
RELATIVE TO THE REPO ROOT — inside a setup worktree that is `setup/CLAUDE.md`; the brain root's own copy is the
same file under its lane directory, and it does not exist in the worktree.

A mutant runs through `setup/mutation/safe-mutate` and must compile: it fails on the assertion, not on `go build`.
A "costs no visible time" claim is proved with 6 interleaved old/new pairs, comparing medians (say the load average).
A brief that dispatches pi first runs `pi -p --model <m> "say ok"`; if it fails, that leg is marked UNVERIFIED.

A new `setup/tests/NNNN-*.sh` takes its number from `setup/next-test-number` (it reads main and the live `task/*`
branches), and the file is named in your first commit. A collision still happening renumbers the subject THIS branch
introduces, never someone else's.>

## QA
<A QA in an isolated Herdr app starts it with `bin/herdr-isolated start <short dir under /tmp>` and ends it with `bin/herdr-isolated stop <dir>` before it reports: closing the app's pid leaves its herdr-core running, and done-invoker warns about it. To CHECK for a leak do not `pgrep` by name: the leaked core's argv is `hc --hc-launchd`, so a name match finds nothing. The detector is `HerdrLeaks` in `core/invokers/herdr_leak.go` (what done-invoker runs): it ties a process to the run by the sockets and cwd under the instance dir. By hand, after `bin/herdr-isolated stop <dir>`: `lsof -nP -U | rg <dir>` listing nothing.>
<A herdr-core proof (a claim about the app, `hc`, the cockpit, or a `brain`/`hw` run inside the app) names three lines, because an executor that is not told how to reach the app guesses and the guess is the person's own: `Mechanism: bin/herdr-isolated start <short dir under /tmp> --brain`, then every command through `<dir>/q …` (never a bare `hc`, `herdr-app` or `herdr`). `Precheck: lsof -a -U -p <app pid>` showing its sockets under `<dir>` and not the person's, and `<dir>/q hc ls` listing only your own terminals; a terminal started with `hc run` is not in the Agents list until `<dir>/q herdr-app do terminal.keep '{"terminal":"<id>"}'` gives it a pane (recipe in herdr-app/README.md). `If it fails: report blocked` and stop with `bin/herdr-isolated stop <dir>`: never a fake marked pass, never a stand-in answer typed as if the app had given it, never the person's app or core.>
<A browser or deployed-build check: QA is handed one string that only the new build carries (a commit sha, or a marker in the bundle) and FAILS if it is not there. A 200 alone does not prove the new build.>

## Reviewing PRs
<One detached checkout per PR (`git worktree add --detach <dir> <ref>`); never review inside the executor's own worktree.>

## Mechanism
<...and, only if this task should run Spec Kit's flow, say why: `--sdd speckit`.
Omit it and the task runs outside any framework (none, the default).>
<How this work is actually done. Name it exactly, do not describe it.>
- Tool / MCP / skill: `<exact name as invoked>`
- Precheck, run FIRST before any real work: `<the cheapest call that proves it is live>`
- If the precheck fails: report blocked and stop. <Name the substitutes that are
  NOT acceptable, so nobody improvises one.>

<Capabilities are per-session. A brainer proving something works in its own pane
proves nothing about yours — that is why this section exists.>

## Access
<Every credential this task needs, as a HANDLE, never a value:>
- <what it is> — Bitwarden item `<item>` in the folder `agents`, declared in the
  frontmatter as `secrets: [<item>]`; the executor reaches it only as
  `hw secret run <item> --as VAR -- <cmd>` (hw checks it is there before building)
- <what it is> — Keychain handle `<handle>` (checked with `hw handle <handle>`)
<A global hook blocks every Keychain secret read, so a PREEXISTING Keychain
credential is unreachable: put it in Bitwarden instead, and do not prescribe the
Keychain read as if it works. A credential the task GENERATES
is fine. If there is no handle for something the task needs, say so here — that
is a gap, and the executor reports it blocked rather than asking a person to
paste a secret into a pane.>

## When you finish
If engram is set up, `mem_save` the findings (`<task> — findings (<date>)`,
type `discovery`, `topic_key: <lane>/<task>/findings`) and, if any, the open
questions (type `manual`, `topic_key: <lane>/<task>/open-questions`). No
`findings.md`: engram is the one store, and `hw` has already pinned your label.
Without engram, the report carries them. An item the task drops is a `pass` row in checks.md with `dropped: <proof>` in its evidence column: done-invoker accepts only pass, fail or unresolved, and refuses a DROPPED verdict. Then `done-invoker "<one paragraph:
what you established, and the observation ids>"` — **at most 900 characters**, counted as characters,
with no code fences, and each observation named as `#<id>` (the number `mem_save` returned; a summary
that names none is refused, and one whose ids are not under this lane's project is refused too). Run `done-invoker --check "<the same paragraph>"` first when the close is a
build: it runs the same gates (the 900 limit, checks.md's verdicts, the design boundary, the plugin
review proof) and delivers nothing, so a refusal costs no work. Once, last thing.

## After merge (brainer)
<Installs that point at the main checkout (a `~/.config` symlink to the brainer's checkout, `install-*.sh` run from it) cannot exist before the merge: list them here for the brainer to run after it. Never ask an executor to run them, and do not widen a guard for it.>

## If you get blocked
Report it blocked. Do not find a way around it, and close with
`done-invoker --blocked "<what stopped you>"` so the brainer knows to stop
waiting.
One question to the brainer is allowed: `ask-invoker "<question>"` — three per
task, one paragraph, no pasted output. Only for what the brainer can answer:
this brief contradicts what you found, two sources disagree, the work left the
scope. Credentials and authorization are not that; those are the owner's.

## Notes
<Links to brain/<project>/decisions.md entries, ticket ids, anything else.>
