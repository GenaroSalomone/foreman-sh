package invokers

// The close half of the release: prod standard (hw's half: core/t5_release.go). At close the env is diffed AGAIN from the same
// two sources, the screens must exist and be newer than the run start, and the QA brief hw dispatched with the release must have
// reported done. The spec is what the dispatch pinned in the receipt; nothing here is read from the brief again.

import (
	"encoding/json"
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
	out, _ := exec.Command(c.BinDir+"/hw", "ledger", sp.QALane, "--task", sp.QATask, "--state", "done", "--json").Output()
	var row struct {
		DoneAt string `json:"done_at"`
	}
	json.Unmarshal([]byte(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]), &row)
	if at, err := time.Parse(time.RFC3339, row.DoneAt); err != nil || !at.After(time.Unix(sp.Start, 0)) {
		return "", die("the release's QA run (%s:%s) has not reported done since this release started (done_at %q, start %s). NOTHING was sent or marked.", sp.QALane, sp.QATask, row.DoneAt, time.Unix(sp.Start, 0).UTC().Format(time.RFC3339))
	}
	return "release: env re-diffed, " + itoa(len(sp.Screens)) + " screens, QA " + sp.QALane + ":" + sp.QATask + " reported", nil
}
