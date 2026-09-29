#!/usr/bin/env bash
# task names and --model, validated on argv before anything is built
#
# Run by ../test-hw, which sources nothing: this file is executed as its own
# bash process so its fixtures, its $TMP and its helper names cannot reach any
# other subject's. Run it alone while working on this subject:
#
#     bash setup/tests/01-task-names-and-models.sh
#
. "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

# ── task names: herdr's rule, checked on argv ───────────────────────────────
# The name must be validated on argv, before a worktree is built, dependencies
# installed, ports allocated or panes opened — a name herdr will later reject
# must fail before any of that work starts, not after.
#
# `--sdd none` on all four: this asserts about the TASK NAME, not about the
# setup lane's live framework mode, and omitting --sdd lets hw's --sdd gate
# answer first instead of the argv name check (same class of bug as
# setup/tests/02-placement-and-manifest.sh).
expect_out "task name: uppercase is refused" \
  "must start with a lowercase letter" setup Mala-Mayuscula --sdd none
expect_out "task name: a space is refused" \
  "may only contain" setup "con espacio" --sdd none
expect_out "task name: over 32 chars is refused on argv" \
  "characters; herdr agent names cap at 32" setup aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --sdd none
expect_absent "task name: a legal name is accepted" \
  "may only contain" setup legal-name_9 --sdd none

# The collation trap itself, asserted rather than assumed: if this ever starts
# failing, bash has changed and the enumerated-alphabet guard can be simplified.
# bash >= 5.0 ranges are ASCII by default (globasciiranges), so on Linux's bash 5
# the trap does not exist and the enumerated guard is merely redundant; the fold
# is a fact about macOS's bash 3.2, and is asserted only where it can occur.
if [ "${BASH_VERSINFO[0]}" -ge 5 ]; then
  pass "bash >= 5 ranges are ASCII; the enumerated guard is redundant here, and harmless"
elif case "Mala-Mayuscula" in [!a-z]*) false ;; *) true ;; esac; then
  pass "bash still folds case in [a-z] ranges — the enumerated guard is still required"
else
  fail "bash no longer folds case in [a-z]: revisit _check_task's enumerated alphabet"
fi

# ── --model is validated per vendor ─────────────────────────────────────────
expect_out "model: codex refuses provider/model before creating a pane" \
  "codex needs a bare model id" setup mp --agent codex --model openai/gpt-6-astra --sdd none
expect_out "model: codex accepts a bare model id" \
  "model=gpt-6-astra" setup mp --agent codex --model gpt-6-astra --sdd none
# `--model terra` dispatched cleanly and the claude CLI then refused to start,
# leaving an idle pane with the brief inside it and zero context.
#
# `--sdd none` throughout: these assert about --model VALIDATION, not the
# setup lane's live framework mode (same class of bug as
# setup/tests/02-placement-and-manifest.sh). None of these aliases (terra,
# future-model-alias, the argv-injection strings below) are sonnet/haiku, so
# they are not the case the comment further down carves out for 130.
expect_out "model: claude warns rather than declaring an unknown alias invalid" \
  "availability is unverified, passing it to the vendor unchanged" setup mp --agent claude --model terra --sdd none
expect_out "model: an unknown Claude alias reaches dispatch unchanged" \
  "model=future-model-alias" setup mp --agent claude --model future-model-alias --sdd none
for m in 'future --extra-flag' 'future*' '-future' 'claude-future --resume X' 'claude-future*' 'claude-future?' 'claude-future[12]' $'claude-future\t--resume'; do
  expect_out "model: unknown alias $m cannot expand into flags or paths" \
    "cannot safely pass this Claude model as one literal argument" setup mp --agent claude --model "$m" --sdd none
done
expect_out "model: claude refuses a provider/model id" \
  "opencode's form, not claude's" setup mp --agent claude --model openai/gpt-5.6-terra --sdd none
expect_out "model: opencode refuses a bare alias" \
  "needs provider/model, not a bare alias" setup mp --agent opencode --model terra --sdd none
# `--sdd none` IS PART OF THE FIXTURE, NOT DECORATION. The subject here is
# ALIAS VALIDATION -- does hw accept this spelling and carry it to the manifest.
# Since 2026-09-11 a sonnet or haiku ORCHESTRATOR is refused whenever an SDD
# mode is in force, and the setup lane's live surface is one, so without this
# flag two of these five aliases would be answered by a gate that has nothing to
# do with spelling. That refusal is real and deliberate; it is asserted where it
# belongs, in 130-a-sonnet-may-not-orchestrate-an-sdd-flow.sh.
for m in opus sonnet haiku fable claude-opus-5; do
  expect_out "model: claude accepts $m" "model=$m" setup mp --agent claude --model "$m" --sdd none
done
expect_out "model: opencode accepts a provider/model id" \
  "model=openai/gpt-5.6-terra" setup mp --agent opencode --model openai/gpt-5.6-terra --sdd none
