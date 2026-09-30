---
# OPTIONAL, and checked. Declare only the fields this task actually requires;
# hw compares each one against the resolved dispatch and REFUSES before it
# builds anything if they disagree. Omit the block entirely and nothing
# changes — the manifest just records that the brief declared nothing.
#
#   agent:     claude | opencode | codex
#   model:     the vendor's own form (opencode needs provider/model)
#   sdd:       speckit | none | gentle (gentle: only where the lane declares it)
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
#              What the task IS. A build-only model (openai/gpt-5.6-sol)
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
# A declaration is a CHECK, not a source: it never supplies a flag hw would
# otherwise ask for. Other keys hw reads (decision, delivery, boundary,
# requires-agents) are in `hw help flags`.
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
- <thing the agent must NOT touch>

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

Anything else here — which directions to run it in, what a fixture must pin, how
to reproduce the old behaviour — is for the executor and hw never touches it.

Leave the fence out and hw says so at dispatch and again at close: the manifest
reads `verify none — the brief has no runnable command`, and the receipt records
that nothing was measured. It is not a gate and it does not block anything. It
just means the report is the only claim there is, which is what it was on
2026-08-25 when three executors in a row shipped correct code with a
verification nobody had run.>

## Retroactividad — una línea, obligatoria si el cambio toca un cálculo

> Borrá esta sección sólo si la tarea no cambia ningún número ni ninguna salida.
> Si la cambia, respondela: **¿este cambio toca datos ya guardados, sí o no?**

Un cambio sobre algo que se **computa en vivo** es retroactivo solo: cualquier
fecha pasada que se abra después sale con el código nuevo, sin backfill.

Un cambio sobre algo **persistido** deja filas viejas desactualizadas, y hay que
decir **qué rango recomputar**.

Por qué está acá y no en la cabeza de nadie: en un mismo producto pueden convivir
las dos cosas — el detalle de paradas se recalcula en vivo y no guarda nada, mientras
`route_daily_report` persiste una fila por fecha y ruta. Y esa tabla **se escribe
recién cuando alguien abre la fecha**, así que ya es un mosaico de días
calculados con versiones distintas del código, antes de cualquier cambio nuevo.

El antecedente: en el PR #289 un filtro **reescribía reportes pasados en
silencio**, y lo encontró el gate del brief, no un test. Una suposición sobre
retroactividad no se ve en ningún diff.

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
- <what it is> — handle `<handle>` (checked with `hw handle <handle>`)
<A global hook blocks every Keychain secret read, so a PREEXISTING stored
credential is unreachable today: do not prescribe the Keychain read as if it
works (see Secrets in the shared rules file). A credential the task GENERATES
is fine. If there is no handle for something the task needs, say so here — that
is a gap, and the executor reports it blocked rather than asking a person to
paste a secret into a pane.>

## When you finish
`mem_save` the findings (`<task> — findings (<date>)`, type `discovery`) and, if
any, the open questions (type `manual`). No `findings.md` — engram is the one
store, and `hw` has already pinned your label. Then `done-invoker "<one
paragraph: what you established, and the observation ids>"`. Once, last thing.

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
