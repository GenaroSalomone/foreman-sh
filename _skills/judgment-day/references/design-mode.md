# Judgment Day — Design Mode

Design mode judges an APPROACH before any code exists: the boundary it protects,
the input space it decides over, and how it would be broken. It exists because a
code-mode Judgment Day on a guard whose approach cannot close does not converge:
every fix is correct and opens a neighbour, and the round budget runs out on the
symptom while the cause is the mechanism.

## The Target: one design document

One file, frozen by its sha256, that states five things. A design missing any of
them is returned to its author before judging (the Decision Gate "target
unclear"), because a judge would have to invent the missing part:

1. **Boundary** — what must never cross, in one sentence.
2. **Mechanism** — how the decision is made (parse, match, allowlist, check the
   object, sandbox…), and where it runs.
3. **Input space** — what the mechanism decides over, and whether the author
   believes it is closed.
4. **Default and exceptions** — what an input nobody anticipated gets, and every
   carve-out with its reason.
5. **Threat model** — who crosses the boundary and how: a cooperative agent
   erring in its ordinary idioms, or an adversary evading on purpose; and what
   is declared out of scope, with the reason (another layer owns it, the input
   cannot occur, the cost is accepted).

Without the threat model every guard over an open space fails: some input
always evades a text-level check, and "a hostile agent can run a script" is true
of every hook ever written. Measured on the first probes (setup/decisions.md,
"Judgment Day juzga el diseño"): with no threat model stated, the judges
rejected the adopted designs as firmly as the failed ones. The threat model is
what makes the verdict discriminate, and it is judged too — see below.

The document is the whole target. Judges do not read the code (there is none, or
it is not the subject), nor the history of the task.

## What the judges answer

- **The threat model itself.** Does it fit the boundary's stated purpose? An
  out-of-scope declaration that excludes what the boundary exists to stop — or
  that excludes the ORDINARY idioms of the agents in scope (a heredoc commit
  message, a relative path, a `cd`) — is a CRITICAL, not a scope.
- **Open or closed.** Is the decided input space enumerable (a fixed list, a
  grammar the mechanism fully owns), or open (a general language such as a
  shell, URLs, free text, paths, anything with quoting, variables, substitution
  or indirection)?
- **Default.** Does an unanticipated input fall on the protected side (deny) or
  the exposed side (allow)? An exception list on top of a deny is closed; a
  deny list on top of an allow over an open space is not.
- **Bypass classes, not instances,** within the declared threat model. Name each
  CLASS of input that crosses the boundary, with one concrete example. "A relative path" is a class;
  `../../x` is its example.
- **Convergence.** For each class: does a finite patch close it, or does the
  patch create a neighbouring class (a new escape, a new carve-out, a new
  syntax to recognise)? Say which.
- **The closed alternative.** When the approach cannot close, name the one that
  can — deny by default with a closed allowlist, checking the object that leaves
  instead of the command that makes it, a boundary the OS enforces — and its
  cost (false refusals, what legitimate use it breaks).

## Severity in design mode

| Severity | Means |
|---|---|
| BLOCKER | The approach cannot hold its boundary WITHIN ITS THREAT MODEL: an open input space decided by an allow-by-default rule or an enumerated deny list, with at least one in-scope bypass class no bounded patch closes. |
| CRITICAL | A bypass class the design does not address but CAN close within its own approach. |
| WARNING | Cost: false refusals, operability, performance, a legitimate use broken. A bypass class the threat model legitimately excludes is reported here, once. |
| SUGGESTION | Anything else. |

`causal_disposition` is always `introduced`: a design has no base. `location` is
`<design file>:<line>` or `<design file>:§<section>`.

## Protocol differences from code mode

- **No fix actor, and no fix rounds.** The code-mode round budget is untouched
  and does not apply: a design is revised by its author, not patched by
  `jd-fix-agent`.
- **One judgment per design.** Both judges, blind, in parallel, same as code
  mode. Corroboration is still two-judge agreement, and a severe finding only
  one judge raised is still verified by the orchestrator (`real | refuted |
  unverifiable`) before the verdict.
- **A revised design is a new target**, with a new sha256 and a fresh judgment.
  A third design judgment in one task is not run: two rejected approaches mean
  the choice of approach goes to whoever owns the task (the brainer, via
  `ask-invoker` inside an executor).
- **Verdict — the approach, not the patch list.** A bypass class is
  *confirmed* when both judges raise it as BLOCKER or CRITICAL (the same class,
  whatever the severity each gave it). Then:
  - `ESCALATED` when a confirmed class was rated BLOCKER by at least one judge,
    or a BLOCKER only one judge raised is ruled `real` by the orchestrator. The
    approach cannot hold; the verdict carries every such class and, where a
    judge named it, the closed alternative.
  - `APPROVED` otherwise. Every confirmed CRITICAL — a class the approach CAN
    close — is listed in the verdict file as a **required amendment**: the code
    must close it, and code-mode Judgment Day at the close is where that is
    checked. Bounded fixes are what code mode converges on; an approach that
    cannot close is what it does not.
  Measured on the probes in setup/decisions.md ("Judgment Day juzga el
  diseño"): this is the line that separated the two designs that failed from
  the two that were adopted. Approving only designs with no CRITICAL at all
  rejected all four.
- **ESCALATED stops the code.** Inside an hw executor the task reports with
  `done-invoker --blocked`, naming the classes; writing the code anyway is
  exactly the failure this mode exists to prevent.

## Design Judge Prompt

```markdown
You are blind Judge {A|B} in Judgment Day DESIGN MODE. There is no code to
review: judge the approach.

Target: {design file path} sha256 {hash}. Read ONLY that file.
Skills to load: {resolved SKILL.md paths}, and _skills/judgment-day/references/design-mode.md.

Answer, from the document alone: does the threat model fit the boundary's
purpose (an exclusion of what the boundary exists to stop, or of the in-scope
agents' ordinary idioms, is CRITICAL); is the decided input space open or
closed; what does an unanticipated input get (deny or allow); every bypass
CLASS within the threat model, each with one concrete example input (a class the
threat model legitimately excludes is a WARNING, once); for each class, whether
a bounded patch closes it or opens a neighbour; and, if the approach cannot
close, the closed alternative and its cost. Severity per design-mode.md:
BLOCKER means the approach itself cannot hold the boundary within its threat
model.

Do not read code, history, other tasks, or memory about this target: the
judgment is of the document as written. Run one exhaustive read-only sweep.

Return one JSON object and no prose:

{"findings":[{"location":"design.md:§Mechanism","severity":"BLOCKER","claim":"bypass class and why no bounded patch closes it","evidence_class":"reasoned","causal_disposition":"introduced","proof_refs":["concrete example input","the closed alternative, if any"]}],"evidence":["what was inspected"]}

The only allowed top-level fields are `findings` and `evidence`; the only
finding fields are `location`, `severity`, `claim`, `evidence_class`,
`causal_disposition`, `proof_refs`. Return `{"findings":[],"evidence":[...]}`
when the design holds, then terminate.
```

## The Design Verdict File

When a brief declares `boundary:`, hw records it and `done-invoker` refuses a
DONE report until `$HW_ARTIFACTS/design-judgment.md` shows the design was judged
and approved. The file is plain text; these lines are what is checked:

```
boundary: <exactly the brief's boundary: value>
design: <path of the design document, absolute or relative to $HW_ARTIFACTS>
design_sha256: <sha256 of that file as judged>
amendment: <one line per confirmed CRITICAL the code must close>
JUDGMENT: APPROVED
```

The last `JUDGMENT:` line is the verdict. The design file must still hash to
`design_sha256`, so the verdict cannot be about a different document. What
`done-invoker` cannot check is ORDER — that the judgment came before the code;
that is the executor's to keep and the report's to state. A `--blocked` report
is never gated: an ESCALATED design is reported that way.
