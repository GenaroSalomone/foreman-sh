---
kind: review
model: opus
review: [{{review_list}}]
review_of: {{review_of}}
requested_by: "{{requested_by}}"
asked: {{asked}}
sdd: none
---

# Independent review of {{lane}}:{{task}} @ {{head}}

This file is the template `hw review` fills (core/t6_review.go): `{{slot}}` words are its slots. You are a fresh session, not
the builder's. Judge the bytes at `{{head}}`; change nothing and post nothing to GitHub.

## What you review

- Branch head: `{{head}}` (check `git rev-parse HEAD` in your worktree: it must print exactly that, or close `--blocked`).
- Range: `{{base}}..{{head}}` — {{code_lines}} code-only line(s), tier **{{tier}}**. `hw review-range` prints an empty range
  here, because this worktree is cut at the reviewed SHA: use the range above.
- Intent: the build's brief, {{build_brief}}. Read it for what was asked, never for what was done.
- The builder's `checks.md`, PR body and test results are **claims, not evidence**. Re-derive what you rely on: run the
  tests yourself when one decides a finding, and read the files outside the diff that the diff depends on.
- Read `git diff {{base}}..{{head}}` once, then whatever else of the tree you need. Not the builder's transcript.

## What you run, in this one session

Each with the Skill tool under its full name; done-invoker reads your transcript for each call:

{{skills}}

{{jd}}

If you run Judgment Day: **a binary (an image, a font, a compiled file) is checked by you, never by a judge.** The diff counts it as
owed, so open it yourself and write what you saw in `checks.md`; keep every binary out of a judge's target paths, since a judge
cannot judge bytes and a coverage relaunch spent on one is wasted. And write the judges' prompts with **intent, not reassurance**:
say what the code is meant to do, never that a value or behaviour is fine ("it sends real mail once the token is changed from
the test value" tells the judge not to look at the very defect the test value is).

A skill that is not among your available skills means this pane was launched without its plugin: close with
`done-invoker --blocked` naming it, never as "not installed". The principle skills are guidance, not reports: apply them to the
diff and say in `checks.md` what they changed in your verdict.

## What you also look for

Beyond the skills, every one of these is a defect class this system has already shipped:

1. Side effects (mail, HTTP, files) inside a transaction, or before the record that says they happened is committed.
2. Retry, rerun and a crash between send and record: does one of them lose work or do it twice?
3. Date and time-zone boundaries: Monday and Sunday, month end, UTC against local time.
4. One rule defined in two places that must agree.
5. Tests that call a real service, or that mirror the implementation instead of its behaviour.
6. A test or placeholder value (an API test token, an example sender, placeholder goals) that would ship to real users and make
   the code report work as done.
7. Tests that reach the network through a client the test guard does not patch (`urllib` when the guard patches `requests`).
8. User-visible text driven by placeholder or default data.
9. If the diff sends external messages, moves money, writes live data or migrates data and the build did not declare
   Judgment Day, that is a `should` finding.

## What you leave

**`$HW_ARTIFACTS/review.md`**, first line exactly:

    verdict: approve | changes-needed  head: {{head}}

then one row per finding: `id | blocker|should|nit|follow-up | file:line | failure scenario | fix direction`.

- A `blocker` is caused by this diff and carries a failure scenario and a file:line. A defect that was already there is a
  `follow-up` row, and it never blocks. `approve` with a `blocker` row, or `changes-needed` with none, is wrong.

**`$HW_ARTIFACTS/checks.md`**, `requirement | verdict | evidence | correction`:

- one row per piece above, named in full (`thermo:thermo-nuclear-code-quality-review`): verdict `pass` means *applied*,
  evidence is what it found, counted; a reviewer that found defects has done its job, so never `fail` a piece for that.
- one row per changed function, with the one-thing-per-function verdict: `simple`, `split`, `merge` or `inline`, and an action
  (`none` for `simple`):

        function | what it does | verdict | action

  The functions hw found in the diff (`$HW_ARTIFACTS/functions.txt`, name and path):

{{functions}}

  A row for a function this list missed is welcome.

Then `done-invoker "<verdict, the head, how many blockers, the engram ids>"`. Save the findings to engram with
`topic_key: {{lane}}/{{task}}-review/findings`.
