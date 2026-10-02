# foreman-sh 0.3.0-rc.1

A release candidate for 0.3.0. Report anything that behaves differently
from 0.2.0-rc.1.

### Added
- `hw train add <lane> <task>` merges a task's branch into `train-<lane>`
  without touching a working tree or running pre-commit. A conflict in
  `setup/test-budgets.json` is resolved key by key against the merge base; any
  other conflict stops the merge and names the file, the tasks on the train
  that touched it, and the incoming task that should resolve it.
- `hw train push <lane>` runs `setup/verify-for-push` on the train and moves
  `main` to it. Uncommitted edits in the checkout are kept; an edit the change
  does not apply on top of stops it before anything moves. It prints the
  `git push` commands and runs none of them.
- `hw train status <lane>` lists the tasks the train carries over `main`.
  Only a lane whose checkout is the brain repository has a train.
- `hw reap --apply` archives before it removes: a merged, clean worktree whose
  only ignored content is `.artifacts`, `qa-report`, `test-results` or
  `playwright-report` is copied to `archive/<lane>/<task>/` beside the work
  directory (`HW_ARCHIVE_ROOT`), the copy is compared with `diff -r`, and only
  then are the worktree and its branch removed. Such a worktree was kept
  forever before. On a lane with `db.provisioned` its database is dumped there
  with `pg_dump -Fc`, checked with `pg_restore -l`, and dropped.
- `hw reap --apply` deletes, with `git branch -d`, the lane's merged task
  branches that no worktree holds.
- The brainer's SessionStart hook lists the lane's tasks that finished and
  never reported, and starts `hw reap <lane> --apply` in the background with a
  time cap (`HW_BG_REAP_TIMEOUT`, 600s). It writes one line with what it
  removed and kept, shown at the next session start; a reap that fails or hits
  the cap is reported `FAILED` and stops there. Executor sessions and
  compactions do not run it; `HW_HOUSEKEEPING=0` turns it off.
- `setup/test-hw` cuts a subject that overruns its ceiling: its budget in
  `setup/test-budgets.json` times `HW_TEST_TIMEOUT_FACTOR` (default 5), never
  under `HW_TEST_TIMEOUT_FLOOR` seconds (default 60, the whole ceiling of a
  subject with no budget). Only that subject's process group is killed; the
  run names it, with its time and ceiling, as red. `HW_TEST_TIMEOUT_FACTOR=0`
  turns it off.
- `setup/tests/650` drives `hw`'s live path against a private herdr server,
  with a shell standing in for the agent: dispatch, brief delivery, turn
  tokens, a report into the brainer pane, and `hw done` closing the tab. It
  skips, saying so, when herdr is not installed.
- The README documents every field of a lane in `projects.json`, with `brief_note` and an example, and `hw help lanes` summarizes them.

### Changed
- `hw done` waits (up to `HW_DONE_TURN_WAIT` seconds, default 60, with a progress
  line) for the turn that sent the report to end, then closes, instead of
  answering "NOT CLOSED YET, run it again". A turn that ends after the report, or
  the cap, still refuses; `--force` never waits.
- `hw done` reaps its own task after closing it, the same way: it removed
  nothing before and printed the commands instead.
- `tsconfig.tsbuildinfo` and `.astro` are regenerable: they no longer keep a
  merged worktree.
- `setup/verify-for-push` runs the full suite with `--keep-going`: a red
  no longer stops the run, every red is listed at the end, and the tree is
  still refused, so one run shows every problem instead of one per run.
- `setup/export/check` scans every file of the export, generated from the
  working tree with the new `setup/export/generate --worktree`, not only the
  overlay: a private term in an exported test now stops the fast gate instead
  of the cut. The export gate reads the export about three times faster.
- A branch's fast gate now runs every slow subject that declares
  `# gate-reference:` and names what the branch changed, past the touched
  budget: the reference-output tests for a change to `bin/hw`, and the
  installer's for one to `install.sh`.
- The per-test ceiling (`HW_TEST_TIMEOUT_FACTOR`) is spent in loaded seconds:
  with more load than cpus a wall second counts less, up to
  `HW_TEST_TIMEOUT_LOAD_MAX` (default 4) times less, so a healthy subject on a
  busy machine is no longer killed and a hung one still is.

## Upgrading from 0.2.0-rc.1

Update your checkout and run `install.sh` again with the arguments you
installed with.

Every release's changes are in [`CHANGELOG.md`](CHANGELOG.md).
