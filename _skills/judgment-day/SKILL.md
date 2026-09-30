---
name: judgment-day
description: "Trigger: judgment day, dual review, adversarial review, juzgar, juzgar el diseño, design review. Run explicit blind dual review with at most two scoped fix/re-judgment rounds, or judge a design before its code exists (design mode)."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.8"
---

## Activation Contract

Load only when the user explicitly requests Judgment Day or equivalent dual/adversarial review for a concrete target. Judgment Day is a standalone developer tool: judges run whenever asked, on any runtime, and need no review transaction, runtime identity, or delivery-receipt machinery to start.

## Two Modes

- **Code mode** (the default, and everything below unless it says otherwise): the target is a diff or a set of files.
- **Design mode**: the target is ONE design document, judged before its code exists — the boundary it protects, the input space it decides over, and how it would be broken. Use it for guards, gates, parsers and any security boundary, and whenever a brief declares `boundary:`. Its prompt, severity table, protocol and verdict file are in [references/design-mode.md](references/design-mode.md); read it before starting. It has no fix actor and no fix rounds: an `ESCALATED` design is revised by its author and judged again as a new target, at most twice per task.

## Hard Rules

- Resolve matching project skills before starting and pass the same paths to both judges (`jd-judge-a`, `jd-judge-b`) and the fix actor (`jd-fix-agent`).
- Build one complete immutable target, then launch two blind read-only judges in parallel with identical scope and criteria.
- Each judge returns one neutral findings result and terminates. Wait for both; never accept a partial judgment.
- Two-judge agreement is the corroboration mechanism; no third refuter is launched.
- Only the parent orchestrator merges/persists findings, launches the fix actor (`jd-fix-agent`), and launches scoped re-judgment.
- Fix only severe findings confirmed by both judges and caused by the candidate; a `causal_disposition: pre-existing` defect is `info` and never blocks. WARNING/SUGGESTION rows remain `info`.
- A BLOCKER/CRITICAL only one judge reports is a severe suspect. Before the verdict, the orchestrator verifies each one against the source, read-only, and rules it `real`, `refuted` or `unverifiable` with the evidence. A `real` suspect is still not auto-fixed and makes the verdict `ESCALATED` when the candidate introduced it; a `real` one already at the base stays `info` like any `pre-existing` defect; the second judge's silence is not a refutation.
- Permit at most two fix rounds and two scoped re-judgments. Re-judgment sees only the frozen ledger plus fix delta and may record fix-caused defects.
- A rebase after a judgment is not a new target. Compare the own diff as judged with the one now, file by file (`hw review-delta <judged-base> <judged-head>` inside an hw executor; per-file `git patch-id --stable` elsewhere): an unchanged diff keeps the verdict; otherwise run a scoped re-judgment over the changed files only, which does not reset the fix-round budget. Re-judge the full range only when the comparison cannot be made.
- The only terminal verdicts are `APPROVED | ESCALATED`; never reset or extend an exhausted round budget.
- A judgment carries no delivery authority: it satisfies no commit, push, PR, or release gate. Delivery remains under ordinary repository policy.

## Decision Gates

| Condition | Action |
|---|---|
| Target unclear | Ask one scope question and stop. |
| Both judges confirm severe finding | Ask the human before round-one correction; then use the bounded fix actor. Inside an hw executor the question goes through `ask-invoker`, and is skipped when the brief already authorizes corrections. |
| One judge reports it | Record suspect; do not auto-fix. If BLOCKER/CRITICAL, verify it against the source before the verdict. |
| Judges contradict | Escalate for explicit human decision. |
| Scoped re-judgment fails before round two | Parent may launch the final bounded fix round. |
| Any issue remains after round two | Escalate and stop. |

## Execution Steps

1. Build the complete immutable target and freeze the scope both judges will inspect.
2. Launch both read-only judges in parallel (`jd-judge-a`, `jd-judge-b`) against the same immutable target.
   A background judge re-invokes you when it finishes. Do not wait with `ScheduleWakeup`, `sleep` or polling: end the turn or do other read-only work until both results arrive. `ScheduleWakeup` belongs to `/loop` and refuses a call without its `prompt`.
3. Merge findings into the frozen ledger and persist it through the selected artifact store. Verify every severe suspect against the source and record its ruling in the ledger.
4. Ask the human before round-one correction (inside an hw executor: `ask-invoker`, or skip it when the brief already authorizes corrections); run the fix actor (`jd-fix-agent`) only for confirmed severe IDs.
5. Run both judges again (`jd-judge-a`, `jd-judge-b`) only over the frozen ledger plus immutable fix delta.
6. Repeat once at most, then run independent final verification and return the terminal verdict.

## Output Contract

Return target identity, round, confirmed/suspect/contradiction/INFO counts, every severe suspect listed with its location, severity, reporting judge and the orchestrator's ruling (`real | refuted | unverifiable`) — never as a count alone, correction work units, scoped re-judgment result, artifact references, skill resolution, and exactly one final `JUDGMENT: APPROVED ✅` or `JUDGMENT: ESCALATED ⚠️`.

## References

- [references/prompts-and-formats.md](references/prompts-and-formats.md) — compact judge/fix prompts and verdict shape.
- [references/design-mode.md](references/design-mode.md) — design mode: target, judge questions, severity, protocol, and the verdict file `done-invoker` checks.
