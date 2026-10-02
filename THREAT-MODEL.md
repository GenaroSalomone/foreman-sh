# Threat model

What foreman-sh's guards are built to stop, what they are not, and what the
harness assumes about the machine it runs on. The limitations register,
[`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md), carries the measured detail
(L5 for the guards).

## What the harness assumes

- One operator, one machine, one OS account. Every brainer and executor runs
  as that account, with the agent's permission prompts turned off by default
  (`--dangerously-skip-permissions` for Claude Code, `--auto` for OpenCode).
  That is the recommended setting, not a fixed one: `hw --permissions ask`
  leaves the prompts on, and the guards below do not depend on it.
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
| Brainer (Codex) | the same guard as a Codex hook | the same, for Codex's shell tool only, and only once the hook is registered in Codex's own configuration — the installer does not do it. A Codex brainer of the lane whose repository is the brain, once it leaves the brain, has no guard |
| Product executor (Claude Code, OpenCode) | the reverse guard | naming the brain except to read it or run its `bin/` tools; a brainer's `hw` verb, or `channel-send` to any pane but its own brainer's; a brain path in an unpushed commit |
| Executor under `--sandbox` (Claude Code, macOS) | a Seatbelt profile, enforced by the kernel | a write outside its work directory and a few named places, every control socket, and reading three credential directories (the `--sandbox` section below) |
| An executor whose lane's repository is the brain itself; any Codex executor | none | — |

**Fail-closed, where it is built to.** The guards refuse on a missing or
malformed `guards.json` and a missing shared module. The Python guards (the
Claude Code hooks and the Codex hook) refuse a payload that is not an object
or a shell call whose input is not an object; the OpenCode plugins refuse a
shell call (the reverse guard's also a write call) whose arguments are not an
object. The Claude Code
brainer hook and the Codex hook also refuse when they crash while deciding
(Codex's refusal is exit 2; any other exit is a non-blocking error there), and
the OpenCode plugins refuse on any crash. The reverse guard, on Claude Code
and on OpenCode, refuses when there is no `python3` to run it. Not covered:
input that is not JSON at all names no tool and is allowed.

## What they do not stop

- **Brainer:** a path computed at run time — `eval` or `sh -c` over a
  substitution, `$(…)`, a script, a `while read` loop — and any write through
  Codex's own patch/write tool.
- **Executor:** everything outside the brain: the home directory and its
  dotfiles, the agents' own configuration, `~/.local/bin`, LaunchAgents,
  crontab, `git config --global`, other repositories, `git push --force` or
  `--delete`, `branch -D`, the keychain, the network.
- **Executor → control plane:** the brain's `bin/` tools are allowed to an
  executor so it can report. `hw` refuses it the brainer's verbs (re-task,
  rule, restart, reap, close another task, dispatch), and the reverse guard
  allows only its own `hw` verbs and `channel-send` toward its own brainer;
  the guard reads the command it is shown, so a script gets past it, and
  `hw`'s check reads the executor's environment, so unsetting `HW_TASK` and
  `HW_RUN` gets past that one (the guard refuses that spelling). A Codex
  executor has no guard at all. Under `--sandbox` the
  kernel holds the line: herdr's socket is out of reach, and only its own
  run's `done-invoker` and `ask-invoker` cross, through the broker.
- **Content:** a product repository's own versioned agent configuration
  (Claude Code settings and hooks, OpenCode plugins, `CLAUDE.md`) loads in the
  executor with the operator's rights. An executor's report text arrives as a
  prompt in the brainer's session.

## Injection: content that carries instructions

Two routes put text or code in front of an agent that runs without permission
prompts.

**A product repository's own content.** An executor works in a worktree of the
repository, so whatever the repository versions loads with the operator's
rights: Claude Code hooks in `.claude/settings.json`, OpenCode plugins,
`CLAUDE.md` and other instruction files. A change merged into the repository
therefore runs on the operator's machine at the next dispatch, and text in a
file can steer the executor toward anything the executor is allowed to do
(see the executor row above: the account's whole reach outside the brain).
*Mitigation today:* `hw` reads a worktree's versioned hook settings before it
hands over a brief, but only to refuse a hook that names a binary missing from
`PATH`; it does not judge what a hook does, and there is no trust prompt for a
repository's hooks. Merging stays with the operator, who is the only review of
that content.

**A report that returns to the brainer.** The text of a `done-invoker` report
arrives as a prompt in the brainer's session, which also runs without
permission prompts and holds every `hw` verb. An executor that read hostile
content can pass it on inside its report. *Mitigation today:* `done-invoker`'s
report is capped in length and forbidden code fences, but an executor can also
reach the brainer through `channel-send`, which has no such limit, so the limit is friction and not a barrier; and the brainer's own guard still
refuses its writes into protected repositories; nothing filters or marks the
report's text as untrusted, and the brainer can still dispatch, close and
message other panes. Treat a report as data from a session that may have read
untrusted files.

Not done: prompting before a repository's versioned hooks load, refusing or
warning when a worktree carries them, and isolating the executor from the
account's reach by default. `--sandbox` confines an executor's writes and
control sockets on macOS when asked for; its network and its reads stay open
(below).

## `--sandbox`: a kernel boundary for one executor (macOS, opt-in)

`hw <lane> <task> --sandbox`, or `sandbox: true` in the brief's frontmatter,
starts a Claude Code executor inside a Seatbelt (`sandbox-exec`) profile that
`hw` generates for that dispatch. It is not the default: without it, nothing
changes. It is refused off macOS and for any vendor but Claude Code, because
those are the only conditions it was measured under.

**What it stops, enforced by the kernel rather than by a hook reading the
command** (`measured` by a live end-to-end dispatch and by the maintainer's
test suite; neither is shipped, since the dispatch runs real agents on real
accounts):

- writes anywhere but the executor's work directory (and its git worktree),
  `$HW_ARTIFACTS`, `$TMPDIR` and `/private/tmp`, its own transcript directory
  and the engram store: the home directory, dotfiles, the agent's settings,
  hooks and plugins, `~/.local/bin`, other repositories, and the main
  checkout's `.git/config` and `.git/hooks` are refused;
- every outbound unix socket except the system resolver's — so herdr (no
  typing into other panes), Docker (no `docker run -v $HOME`) and the
  ssh-agent (no push over SSH) are out of reach;
- reading `~/.ssh`, `~/.config/gh` and `~/.docker`.

The executor still reports and asks: `done-invoker` and `ask-invoker` write
their arguments into the run's outbox, and a broker outside the sandbox,
started with the executor and ended with it, runs the real invoker for that
run only. No other command crosses, and the request carries no environment.
The broker and the launch keep their own files — the profile, the heartbeat,
requests in flight — in a per-run directory under
`~/.local/state/hw-sandbox/` that the profile does not open, and answer into
the outbox only by renaming into the directory it pinned at start, so a link
the executor plants there — for an answer, or for the outbox itself — is
replaced or refused, never written or read through. A revived task is
re-armed from a record keyed by its lane and task name in that same place,
which the executor can neither delete nor hide by renaming its run
directories.

**What it does not stop:**

- **TCP.** The network is open: the internet, and every service on the host —
  engram's HTTP port, databases, dev servers. A push over HTTPS with a
  credential the executor can reach is not refused.
- **`git push --force` and `--delete`.** No local profile tells a forced push
  from a normal one without breaking the normal one. Cover them with branch
  protection on the server.
- **Chrome's own sandbox.** A sandbox cannot start inside another, so a
  browser the executor launches must run with `--no-sandbox`; it is then
  contained by this profile instead of by its own (`measured` while the
  profile was designed; not re-measured by the dispatch above).
- **Reads.** Apart from the three credential directories, the executor reads
  whatever the account can: other repositories and `.env` files. Keychain
  access under the profile is not measured.
- **What it is allowed to write.** The engram store holds every lane's
  memory; `/private/tmp` and `$TMPDIR` are shared with the operator's other
  sessions; `<workdir>/.hw/` holds the run's own state (ask count, done
  marker, return address), as it does without the sandbox.
- **The repository's branches.** A worktree's commits land in the product
  repository's shared `.git`, so its `objects`, `refs`, `logs` and
  `packed-refs` stay writable: a sandboxed executor can move or delete any
  branch of that repository (`git branch -D`, `git update-ref`), though not
  its hooks or config.

## Stated non-goals

Containing an agent that works to get around the guards; protecting secrets
readable by the account; protecting remotes the account can push to.

## A hard boundary, if you need one

It is only as strong as the operating system: a separate account or a mount
the agent cannot write, or a sandbox or container around the agent. On macOS,
`--sandbox` (above) is that sandbox for an executor's writes and its control
sockets; it does not confine the network.
