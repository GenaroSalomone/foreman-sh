---
# OPTIONAL, and checked. Declare only the fields this task actually requires;
# hw compares each one against the resolved dispatch and REFUSES before it
# builds anything if they disagree. Omit the block entirely and nothing
# changes — the manifest just records that the brief declared nothing.
#
#   agent:     claude | opencode | codex
#   model:     the vendor's own form (opencode needs provider/model)
#   sdd:       speckit | none (gentle is retired: it warns and runs none)
#   base:      the branch the worktree is built on — declare it on any task
#              whose result is measured
#   requested_by: "<who, date: the request, quoted>"  and  asked: investigate | build
#              What was asked for. hw refuses a dispatch without it where the lane requires one and
#              warns elsewhere; `investigate` does not implement.
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
#              (Claude Code on macOS only; THREAT-MODEL.md)
#   requires-agents: [primary:|subagent:]<name>, …
#              Agents the task cannot do without: hw refuses the dispatch
#              when the executor cannot reach one, instead of warning.
#   review:    judgment-day | plugins | none — the review the close names, in every lane
#              and --sdd mode. judgment-day (the default when absent) orders Judgment Day
#              over the branch; plugins names only the review skills of `plugins:` (below)
#              and no Judgment Day; none adds no review note. Any other value is refused
#              before anything is built (rdd is retired: it warns and runs judgment-day).
#   plugins:   [thermo, ponytail, pstack]  (any subset)
#              Review plugins (pinned, complete: their hooks run in THIS executor only) loaded
#              into it: claude gets one --plugin-dir each (hook writes go to $HW_ARTIFACTS/plugin-state),
#              pi one -e <plugin dir> each, opencode and codex get NOT APPLIED. They are an
#              OPTIONAL install step, off by default (setup/vendor/plugins/install, or
#              install.sh --with-review-plugins): a name that is not installed is refused
#              before anything is built, naming that command. Nothing is installed globally
#              and nothing is written to ~/.claude or ~/.pi. A re-task (`hw next`) cannot add one.
#   boundary:  <what must never cross> — the task is not done without an
#              approved design judgment (INSTALL.md, Judgment Day)
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

## In scope
- <thing>

## Out of scope
- <what stays untouched> — <why: who owns it, or what breaks if it moves>

## Done when
- [ ] <observable, checkable condition>

## Testing / Verification

<PUT THE COMMAND IN A FENCE. `hw` extracts it from this section, pins it at
dispatch into `.hw/<run>/verify`, and RUNS IT ITSELF in `hw done` — the exit
code and a hash of the output go to the receipt, and `hw receipt <project>
<task>` shows them beside what was dispatched.>

```
bash setup/test-hw
```

<Only a fenced block or a 4-space-indented block counts. Inline backticks are
prose: in these briefs a `path` and half a sentence of `VAR=1 VAR=2` are spelled
exactly like a command, so hw does not read them — it would be executing prose.

A heavy command (a subject, the fast gate, `go test` of a package, a release cut) runs as
`setup/heavy -- <cmd>`: two slots, first come first served, the holders shown. Write
that and nothing else about the machine: a `pgrep -f` count or a hand-typed lock
deadlocks the executors, and `hw` refuses a brief that carries the `pgrep` one.

The full suite already includes every Go partition (`setup/go-test-core.sh`): do not
run them before it, and never a bare `go test ./...` over `core/` with `HW_INPROC_ALL=1`
(it starts the monolithic TestC848 and never finishes; `core/main_test.go` exits 2 on it).
A partition is `setup/go-test-core.sh <name>` (`--list` names them), alone only when the
brief's change can reach just that one.

Anything else here — which directions to run it in, what a fixture must pin, how
to reproduce the old behaviour — is for the executor and hw never touches it.

Leave the fence out and hw says so at dispatch and again at close: the manifest
reads `verify none — the brief has no runnable command`, and the receipt records
that nothing was measured. It is not a gate and it does not block anything. It
just means the report is the only claim there is, which is what it was on
2026-08-25 when three executors in a row shipped correct code with a
verification nobody had run.>

## Mechanism
<...and, if this task should run under a framework the directory does not
already have, say which and why: `hw <project> <task> --sdd speckit`.
Omit it and the task inherits the directory's mode, which is usually right.>
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
Without engram, the report carries them. Then `done-invoker "<one paragraph:
what you established, and the observation ids>"`. Once, last thing.

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
