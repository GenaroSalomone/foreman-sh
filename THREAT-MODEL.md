# Threat model

What foreman-sh's guards are built to stop, what they are not, and what the
harness assumes about the machine it runs on. The limitations register,
[`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md), carries the measured detail
(L5 for the guards).

## What the harness assumes

- One operator, one machine, one OS account. Every brainer and executor runs
  as that account, with the agent's permission prompts turned off
  (`--dangerously-skip-permissions` for Claude Code, the equivalent elsewhere).
- The agents are **cooperative but fallible**: they follow instructions, and
  sometimes follow wrong ones. They are not assumed to be adversarial.
- The operator trusts the content of the product repositories, the briefs, and
  the agent runtimes (Claude Code, OpenCode, Codex, herdr) and what they send
  to a hook.

## What the guards stop — a tripwire

| Actor | Guard | Stops |
|---|---|---|
| Brainer (Claude Code) | the repo-write hook plus `Edit`/`Write` deny rules | a write or a git mutation inside a protected repository or one of its live worktrees (the ones `guards.json` names), spelled with an absolute or relative path, `~`, `$HOME`, an exported or a shell-local variable, a symlink chain, `..`, `//`, `/./`, a letter-case variant, a glob or a brace expansion, or made after a `cd` into it earlier in the same command; `find -delete`/`-exec` and `fd -x`; a command nested in `sh -c` or `eval` with a literal body |
| Brainer (OpenCode) | the same guard ported to JavaScript | the same verdicts, held by a shared vector suite run against both |
| Brainer (Codex) | the same guard as a Codex hook | the same, for Codex's shell tool only, and only once the hook is registered in Codex's own configuration — the installer does not do it |
| Product executor (Claude Code, OpenCode) | the reverse guard | naming the brain except to read it or run its `bin/` tools; a brain path in an unpushed commit |
| An executor whose lane's repository is the brain itself; any Codex executor | none | — |

**Fail-closed, where it is built to.** The guards refuse on a missing or
malformed `guards.json` and a missing shared module. The Python guards (the
Claude Code hooks and the Codex hook) refuse a payload that is not an object
or a shell call whose input is not an object; the Claude Code brainer hook
also refuses when it crashes while deciding, and the OpenCode plugins refuse
on any crash. The reverse guard, on Claude Code and on OpenCode, refuses when
there is no `python3` to run it. Not covered: input that is not JSON at all
names no tool and is allowed; the Codex hook, if it crashes while deciding,
exits 1 rather than with its refusal code 2; and the OpenCode
brainer plugin allows a shell call whose arguments are not an object.

## What they do not stop

- **Brainer:** a path computed at run time — `eval` or `sh -c` over a
  substitution, `$(…)`, a script, a `while read` loop — and any write through
  Codex's own patch/write tool.
- **Executor:** everything outside the brain: the home directory and its
  dotfiles, the agents' own configuration, `~/.local/bin`, LaunchAgents,
  crontab, `git config --global`, other repositories, `git push --force` or
  `--delete`, `branch -D`, the keychain, the network.
- **Executor → control plane:** the brain's `bin/` tools are allowed to an
  executor so it can report, and they are the same tools the brainer uses:
  an executor can run `hw` verbs and `channel-send` to any pane.
- **Content:** a product repository's own versioned agent configuration
  (Claude Code settings and hooks, OpenCode plugins, `CLAUDE.md`) loads in the
  executor with the operator's rights. An executor's report text arrives as a
  prompt in the brainer's session.

## Stated non-goals

Containing an agent that works to get around the guards; protecting secrets
readable by the account; protecting remotes the account can push to.

## A hard boundary, if you need one

It is only as strong as the operating system: a separate account or a mount
the agent cannot write, or a sandbox or container around the agent.
