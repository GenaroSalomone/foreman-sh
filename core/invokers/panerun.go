package invokers

// A CALL THAT WRITES PANE TOKENS MUST BE THE RUN THAT OWNS THE PANE.
//
// MEASURED 2026-11-04 (retro of retro-fixes-rc2): a pi probe started inside an
// executor's pane inherited HERDR_PANE_ID, so its done-invoker published onto
// its PARENT's pane — hw_task, done_state and the summary were rewritten, and
// `hw status` showed the live executor as `done-NOT-delivered`. The invoker
// only checked that HERDR_PANE_ID was non-empty.
//
// herdr records the run on the pane as the hw_run token; a call whose HW_RUN is
// not that value is a nested process talking about somebody else's pane. It is
// refused before anything is written. An unreadable pane (herdr down, no agent
// on it yet, a stub) is NOT a mismatch: the check refuses only what it can
// prove, so a probe against a stub and a first publish are unchanged.

// paneRunToken is the hw_run herdr records on this call's pane: "" when it
// cannot be read (herdr unreachable, no such pane, no token).
func (c *Ctx) paneRunToken() string {
	pane := c.get("HERDR_PANE_ID")
	if pane == "" {
		return ""
	}
	raw, rc := c.out(c.BinDir+"/herdr-rpc", "call", "agent.list", "{}")
	if rc != 0 || raw == "" {
		return ""
	}
	doc, ok := decodeDoc(raw)
	if !ok {
		return ""
	}
	top, ok := doc.(map[string]any)
	if !ok {
		return ""
	}
	result, _ := top["result"].(map[string]any)
	agents, ok := top["agents"].([]any)
	if !ok && result != nil {
		agents, ok = result["agents"].([]any)
	}
	if !ok {
		return ""
	}
	for _, a := range agents {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		if p, _ := m["pane_id"].(string); p != pane {
			continue
		}
		tokens, ok := m["tokens"].(map[string]any)
		if !ok {
			return ""
		}
		run, _ := tokens["hw_run"].(string)
		return run
	}
	return ""
}

// refuseForeignPaneRun is the check done-invoker and ask-invoker make before
// publishing anything: the pane they are about to write is this run's pane.
func (c *Ctx) refuseForeignPaneRun() error {
	want := c.get("HW_RUN")
	got := c.paneRunToken()
	if got == "" || got == want {
		return nil
	}
	return die("REFUSING before any pane token was written: this pane (%s) belongs to hw run %q, and this call's HW_RUN is %q. A nested process — a probe, a subagent shell, a test started inside another executor's pane — inherited HERDR_PANE_ID and would rewrite the RUNNING executor's hw_task/done_state/summary on its own pane. That is how a live executor came to read `done-NOT-delivered` while it was still working (measured 2026-11-04, retro-fixes-rc2). NOTHING was sent or marked. If this is your own probe, start it in a herdr workspace of its own with no HERDR_PANE_ID/HW_INVOKER_PANE from the parent pane (setup/CLAUDE.md, \"For a target that must look like a real executor\"), then report from there.", c.get("HERDR_PANE_ID"), got, want)
}
