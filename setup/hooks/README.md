# Git hooks for this repo

`.git/hooks` is not versioned, so a fresh clone has none of this. Install them all:

    setup/install-hooks.sh            # install or repair
    setup/install-hooks.sh --check    # say what is installed, change nothing

It links the three entry points git invokes and nothing else. The files those
hooks SHARE — `suite-trigger-pattern.sh`, `attribution-pattern.sh`,
`decisions-check.py` — are deliberately not installed: each hook resolves them through `git rev-parse --show-toplevel`,
from the working tree, so a hook works as a bare symlink with nothing beside it.

That is not decoration. On 2026-09-09 `pre-push` sourced `suite-trigger-pattern.sh`
relative to `dirname "$BASH_SOURCE"` — which, for a hook invoked as
`.git/hooks/pre-push`, is `.git/hooks`. The first real push after that died with
`.git/hooks/suite-trigger-pattern.sh: No such file or directory`, and the suite
was green throughout, because the tests exercised fixture hooks rather than the
installed one. `setup/tests/121-installed-hooks-resolve-what-they-source.sh`
installs into a fixture and runs the installed hook with no sibling present.

**`pre-commit`** refuses a commit that mixes shared tooling (`bin/`, `shell/`,
`layouts/`, `setup/plans/`, `setup/hooks/`, `CLAUDE.shared.md`, the root `CLAUDE.md`, `README.md`,
`BRIEF-TEMPLATE.md`) with one project lane's files (`<project>/decisions*`,
`<project>/briefs/`, `<project>/CLAUDE.md`, `<project>/.claude/`,
`<project>/opencode.json`).

That combination is the signature of `git add -A` in a repo several agents write
to at once. A brainer finishing one lane's work swept another agent's
uncommitted `bin/hw` and `CLAUDE.shared.md` into a commit titled after its own
lane's merge decision. Nothing was lost, but the history says a lane's merge
decision contained a 239-line rewrite of hw's delivery path, and whoever wrote
that rewrite never got to describe it.

Stage paths, not `-A`. `--no-verify` overrides it for a change that genuinely
spans both.

## When the tooling suite fails

The failure report lists the staged paths that triggered `setup/test-hw` and
runs `bin/hw stage`. Its claimant lines are **executor testimony, never inferred
authorship**: a declared path can name an executor; `UNATTRIBUTED` means there
is no declaration, not that the hook guessed nobody wrote it.

In this shared tree, do **not** use `git commit -- <paths>` as recovery. It
commits working-tree bytes, not a private index, and can capture another
writer's edits. Use the private-index/`commit-tree` recipe in
[`setup/CLAUDE.md`](../CLAUDE.md) instead.

**`commit-msg`** refuses a message carrying AI attribution — a
`Co-Authored-By:` naming an agent vendor, or a "Generated with [Claude Code]"
line. The detection lives in `attribution-pattern.sh` and `pre-push` reads the
SAME file, so the two gates cannot silently disagree about what counts. Both
also read the same refusal formatter from it, so an offending line reads
identically at commit time and at push time.

**A TRAILER IS REFUSED; A QUOTATION IS NOT — the anchor is what tells them
apart.** MEASURED 2026-09-17, running the then-current detection over the 55
commits this repository had not yet pushed: it refused three, and one deserved
it. `222f9d7` carries a real trailer at column 0. `c70dbe7` and `9dc5ab8` are
the two commits that BUILT this guard, and they paste the trailer into their own
messages as evidence that the detection works — indented, which is how this
lane's rules say to quote evidence. The guard bit the evidence and the release
sat blocked behind two false positives of its own making.

So the pattern is anchored at `^`, not `^[[:space:]]*`: a git trailer begins its
own line at column 0, and a quotation is indented, fenced, or embedded mid-line
in a shell example. The old whitespace class was never justified by a
measurement — it arrived with the first inline version of the regex in `684b760`
— and one was looked for before it was removed: every commit in this history,
grepped for an attribution line with LEADING whitespace, returns exactly the
three hits that are those two false positives. No real indented trailer exists
anywhere in this history. UNESTABLISHED, and named rather than assumed away:
whether some tool somewhere emits an indented trailer token. None was found
here.

The narrowing errs toward refusal, deliberately — a column-0 quotation inside a
fenced block is still refused, because a false negative is worse than a false
positive. And the refusal now names the LINE and the COLUMN of what it objected
to, because a message can carry the trailer as evidence and as a trailer at the
same time and the old wording left the reader unable to tell which.

It judges the message file exactly as written, with no cleanup reasoning of its
own, with ONE exception: git's own `-v` diff. Both halves of that were paid for.

An early version stripped comment lines and truncated at any scissors-SHAPED
line, assuming git removes them; git only does that when the message is EDITED,
and `git commit -m`/`-F` gets `whitespace`, which keeps both. MEASURED
2026-09-16 on git 2.50.1: a plain `Co-Authored-By:` trailer after a
scissors-shaped comment line survived into the commit while the hook exited 0.

Discarding nothing at all fixed that and broke the other direction. Under
`git commit -v` git appends the staged diff to the message file before the hook
runs, and a unified diff prefixes an UNCHANGED context line with one space —
which the pattern's THEN-CURRENT `^[[:space:]]*` anchor accepted. Since this
repo's own files carry the example trailer as content, a `-v` commit near one of
them was refused for text that never reaches the message. Also measured, same
day. That particular route is closed twice over now: a diff context line carries
a leading space, so it cannot match a `^`-anchored pattern at all. The marker
rule below is kept anyway — it drops only lines that are both below git's own
marker and shaped like diff content, none of which can match the anchored
pattern, so keeping it cannot introduce a false negative.

So the discriminator is git's OWN three-line verbose marker (the scissors line
plus "Do not modify or remove the line above." and "Everything below it will be
ignored."), never a scissors shape alone. Only below all three, and only lines
that are unified-diff content, are dropped. A forged scissors line hides
nothing; a forged full marker hides nothing either unless the trailer is itself
shaped like a diff line. Both directions are arms in
`setup/tests/152-la-atribucion-muere-en-commit-msg.sh`, and the `-v` arms drive
a real `git commit -v`.

**A KNOWN GAP, LEFT OPEN:** a trailer prefixed with `#`
(`# Co-Authored-By: Claude …`) matches neither gate, and the 2026-09-17 anchor
narrowing did not touch it: `#` was never whitespace, so it matched neither gate
before either. Readable straight off the regex, nothing measured about it — and,
measured, under the non-interactive `whitespace` cleanup git keeps that line in
the message. Closing it would refuse a message that quotes the rule inside a
comment, which this repo's own files do constantly. Named here rather than
closed quietly, and pinned by the `hash-gap` arm in
`setup/tests/152-la-atribucion-muere-en-commit-msg.sh`.

**It does not see a `commit-tree` commit, and that is not a gap it can close.**
The `setup` lane's mandated recipe in [`setup/CLAUDE.md`](../CLAUDE.md) builds
commits with `git commit-tree`, which runs no hooks at all — the four offending
commits of 2026-09-10 were built that way and this hook would not have seen one
of them. It covers every ordinary `git commit`; `pre-push` is what covers
everything else, and the two are deliberately both live.

**`pre-push`** permits only the `backup` remote.
