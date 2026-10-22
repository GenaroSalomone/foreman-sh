---
name: memory-retro
description: Use when `hw status` prints an `engram: … run the memory-retro skill` line, when bin/memory-audit names memories (type/expires, orphans, repeats a rule), or when the operator asks for a retro of the memories. Sorts the auto-memory into its layers, judges pending engram conflicts in batches with mem_judge, proposes rulings, and reports counts before and after. Trigger words - memory retro, retro de memorias, pending conflicts, conflictos pendientes.
---

# Memory retro — keep the layers apart, and the conflicts judged

The layers, and the rule "one fact, one owner": a fact about the system lives in
a ruling (`<lane>/decisions.md`) or a `CLAUDE.md`; how a brainer works lives in
Claude's auto-memory; evidence lives in engram; none is copied into another.
The deterministic half is `bin/memory-audit` (read-only, in `hw status`); this
skill is the judgment it cannot make.

You propose rulings and judge conflicts. You do **not** edit the auto-memory
files, delete observations or touch `~/.engram/engram.db`: the brainer edits its
own memory, and engram is reached only through its MCP and CLI.

## 1. Count first

```bash
bin/memory-audit                                   # what the audit names today
engram conflicts stats --project brain             # pending / judged, by relation
engram stats --project brain                       # observations in total
```

Write down: pending conflicts, observations without `topic_key` (the audit line
gives the percentage), memory files, audit findings. These are the "before".

## 2. The auto-memory against the layers

Read the stores the audit names (`~/.claude*/projects/<the git root, every non-alphanumeric character turned into ->/memory/`).
Read each flagged file and its description only; do not read all of them.

| the memory says | it belongs in | proposal |
|---|---|---|
| a fact about the system (account, model, mechanism, what is ruled out) | a ruling in `<lane>/decisions.md`, or the `CLAUDE.md` that enforces it | write the ruling; the memory becomes a pointer or is retired |
| how the operator wants the brainer to work | the auto-memory | keep; merge into the existing file that holds the topic |
| a measurement, a report | engram, with a `topic_key` | `mem_save` it; retire the memory |
| a `project` fact | auto-memory with `expires:` | add the date, or retire when past it |

The audit's "repeats a rule" line is a lead, not a verdict: open both sides and
confirm they say the same thing before proposing to retire either.

## 3. Pending conflicts, in batches of at most 50

```bash
engram conflicts list --project brain --status pending --limit 50
engram conflicts show <id>          # titles and obs sync ids only, never the bodies
engram export <file> --project brain   # sync_id -> numeric #id, content, topic_key
```

`list` and `show` carry no body: to read a pair, map its `obs-…` sync ids to
numeric ids through the export (a scratch file, never committed) and read the
content there or with `mem_get_observation`.

`id` is for `show`; the judgment id for `mem_judge` is the row's `sync_id`
(`rel-<hex>`). One `mem_judge` per row, never a verdict reused across rows.

| the pair | relation |
|---|---|
| two reports that touch the same area but say different things | `related` |
| a newer observation replaces an older one on the same fact | `supersedes` (and the older one is not deleted) |
| both true, different scope (lane, account, date) | `scoped` |
| both true, nothing to reconcile | `compatible` |
| a scan false positive: unrelated subjects | `not_conflict` |
| they contradict and neither replaces the other | `conflicts_with` — a ruling is needed |

Give a one-line `reason` naming what you compared. `confidence` under 0.7, or
`supersedes`/`conflicts_with` on a `decision`/`architecture`/`policy`
observation, goes to the brainer instead of being judged: list those and leave
them pending. Most of the backlog is a `findings` row against an unrelated
earlier one; that is `not_conflict`, and it should be quick. Do not judge a pair
you did not read: a verdict from a title is a guess recorded as a fact.

A judged `not_conflict` leaves the pending count without showing up under a
relation in `conflicts stats`; count it from your own verdict list.

Stop after the batch. Re-run the counts; offer the next batch, do not loop.

## 4. Why the backlog grows — fix the writer, not the backlog

A "findings" row that never upserts is a save without `topic_key`. Report how
many of the batch's sources had none; the cure is the executor instructions
(`CLAUDE.shared.md`, `BRIEF-TEMPLATE.md`: `topic_key: <lane>/<task>/findings`),
not more judging. A cause you did not measure stays `UNESTABLISHED`.

## 5. Output

Short, in the language the operator writes to you:

    antes → después: pendientes <a>→<b>, sin topic_key <x>%→<y>%, hallazgos de la auditoría <n>→<m>
    juzgados: <k> (not_conflict <…>, related <…>, supersedes <…>, scoped <…>, compatible <…>)
    para el brainer: <conflicts left pending because they are his call, with ids>
    propuestas: <memory → destino (ruling / engram / retire), one line each>

Then `mem_save` once: title `memory retro (<date>)`, type `discovery`,
`topic_key: setup/memory-retro/<date>`, omitting `project`. End with
`Not requested: <the rulings and the memory edits you proposed>`.
