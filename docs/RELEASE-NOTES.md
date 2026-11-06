# foreman-sh 0.7.3

foreman-sh 0.7.3 makes a review the brief's choice and enforces Judgment Day in code when a brief asks for it.

## In short

- **A review is opt-in.** A brief with no `review:` closes with none, and `review: none` needs no justification line. A declared review is enforced as before.
- **Judgment Day is enforced, not just proved.** `done-invoker` reads the judges' calls from the transcript, on Claude and pi, and refuses a re-judgment that adds new findings or a run past the round budget.
- **pi waits like Claude.** A pi executor gets `monitor` (line events with a mandatory timeout) and `job_list`, and the sleep-poll rule is narrower and the same on both runtimes.
- **A pi run's transcript reaches the archive** wherever its worktree sat, and the close says where it went.

## What changed

8 changes since 0.7.2.

### Added
- A pi executor has `monitor` (a long command's matching output lines are events, batched into one wake per
  second, with a mandatory timeout and an event cap) and `job_list`, beside `bash_background` and `job_stop`.
  They keep the write, secret and sleep-poll guards and the output filter.
- Judgment Day is enforced in code when a brief declares it, on pi and Claude: `done-invoker` reads the judges'
  calls from the transcript and refuses a fourth two-judge round (the first plus two re-judgments) or a ninth judge run, and a re-judgment that adds
  a finding to the first judgment's frozen ledger (unless the judge marks it `fix-caused`, or the finding is at a file its own
  prompt names); a close reporting `ESCALATED` is accepted over either breach.
- A pi executor records at session start whether a background subagent can start (`pi_background_subagents`).
  A new live probe, `setup/pi/test-live-subagent-wake.mjs`, proves the wake with chord absent: idle, mid-turn and
  compacting.
- `bin/gh-attach <owner/repo> <number> <image>... [--comment "<text>"]` attaches screenshots to a
  PR or issue as GitHub user-attachments, with no commit, and prints their markdown. It runs the
  pinned gh-image (v1.4.1, sha256 checked on every run) with the session cookie only as the
  Bitwarden item `github-user-session`, resolved by `hw secret run`; the tool gets an
  environment of its own and is never started without the token.
- `setup/install/install-gh-image.sh` installs that build under `~/.local/share/hw/tools/gh-image`
  and refuses a checksum that is not the pin's.

### Changed
- A review is opt-in: a brief with no `review:` closes with none, and `review: none` needs no `review-skip:` line on
  any `kind` (a declared review is enforced as before).
- The sleep-poll rule is narrower, on Claude and pi alike: a foreground poll loop is allowed under `timeout`
  up to 120 s, and a standalone `sleep` is refused while background work is in flight (a state pi reports; Claude's
  hook cannot see it).

### Fixed
- The archive copy of a pi run's transcript no longer depends on the worktree sitting at `work/<lane>/<task>`:
  it lands beside the task's archived artifacts, takes the session the run recorded instead of the newest one on the
  machine, and the close says where it went. A `session_file` is copied only when it is a `.jsonl` under a pi session root.

## Known limits

- macOS and Linux only. Windows, native or under WSL2, is not supported or
  tested (KNOWN-LIMITATIONS, L1b).
- The Go core is on by default when the installer builds it; `HW_IMPL=bash`
  goes back. Every limit, with its evidence, is in
  [`KNOWN-LIMITATIONS.md`](KNOWN-LIMITATIONS.md).

## Upgrading from 0.7.2

Run `foreman-sh upgrade --brain DIR`. A brain installed before `upgrade`
existed has no record of its flags yet: run your install command once more,
and `upgrade` works from then on.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
