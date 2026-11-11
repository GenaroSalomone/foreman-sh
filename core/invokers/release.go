package invokers

// The close half of the release: prod standard (hw's half: core/t5_release.go). At close the env is diffed AGAIN from the same
// two sources, the screens must exist and be newer than the run start, and the QA brief hw dispatched with the release must have
// reported done. The spec is what the dispatch pinned in the receipt; nothing here is read from the brief again.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"hwcore/internal/releasediff"
)

// ReleaseGate returns a one-line note for --check, or a refusal.
func (c *Ctx) ReleaseGate(raw, artifacts string) (string, error) {
	var sp releasediff.Spec
	if err := json.Unmarshal([]byte(raw), &sp); err != nil {
		return "", die("the receipt's release record is unreadable (%v). NOTHING was sent or marked.", err)
	}
	diff, err := releasediff.Diff(releasediff.FromEnv(c.BinDir+"/.."), sp.From, sp.To)
	if err != nil {
		return "", die("cannot re-diff the env at close: %v. NOTHING was sent or marked; there is no names-only fallback.", err)
	}
	if err := releasediff.CheckClose(diff, sp.Ruled); err != nil {
		return "", die("%v. NOTHING was sent or marked.", err)
	}
	if err := releasediff.CheckScreens(artifacts+"/screens", sp.Screens, time.Unix(sp.Start, 0)); err != nil {
		return "", die("%v. NOTHING was sent or marked.", err)
	}
	// the QA must have reported AFTER this release started: an earlier report (another release, a rehearsal) is not this one's QA
	row, err := c.qaLedgerRow(sp.QALane, sp.QATask)
	if err != nil {
		return "", die("cannot read the state of the release's QA run (%s:%s): %v. NOTHING was sent or marked. This is not \"the QA has not reported\": ask your brainer with ask-invoker \"cannot read the QA's state: <that error>\" and end your turn; do not re-run done-invoker until it answers.", sp.QALane, sp.QATask, err)
	}
	if at, err := time.Parse(time.RFC3339, row.DoneAt); err != nil || !at.After(time.Unix(sp.Start, 0)) {
		return "", die("the release's QA run (%s:%s) has not reported done since this release started (done_at %q, start %s). NOTHING was sent or marked. Do not re-run done-invoker or poll for it: it reads the same state each time. Close this task now with done-invoker --blocked \"waiting on QA %s:%s\" and end your turn; your brainer's ruling resumes you when the QA reports, and only then run done-invoker again.", sp.QALane, sp.QATask, row.DoneAt, time.Unix(sp.Start, 0).UTC().Format(time.RFC3339), sp.QALane, sp.QATask)
	}
	return "release: env re-diffed, " + itoa(len(sp.Screens)) + " screens, QA " + sp.QALane + ":" + sp.QATask + " reported", nil
}

// qaLedgerRow is `hw ledger <lane> --task <task> --state done --json`: the row `hw status` shows as done for the QA, however
// that QA was launched (by the release's dispatch or by hand). hw refuses its brainer verbs to a shell where HW_TASK/HW_RUN are
// set, and `ledger` is one of them, so the call is made as the invoker, not as the executor: without that the refusal went to
// stderr, stdout stayed empty and a QA that had reported read as "done_at empty" (measured 2026-11-10). A command that
// fails is an error here, never an empty row.
func (c *Ctx) qaLedgerRow(lane, task string) (row struct {
	DoneAt string `json:"done_at"`
}, err error) {
	cmd := exec.Command(c.BinDir+"/hw", "ledger", lane, "--task", task, "--state", "done", "--json")
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HW_TASK=") && !strings.HasPrefix(kv, "HW_RUN=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()
	if runErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return row, fmt.Errorf("hw ledger failed (%v): %s", runErr, msg)
		}
		return row, fmt.Errorf("hw ledger failed (%v)", runErr)
	}
	json.Unmarshal([]byte(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]), &row)
	return row, nil
}
