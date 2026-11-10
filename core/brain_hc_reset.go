package main

// --reset, --resume and `brain relaunch` ON HERDR-CORE (herdr-app-hc-parity, item 5).
//
// An app terminal has no pane to type /exit into, so the TUI's replace-in-place does not apply. The same three
// verbs are a kill and a start: `hc kill` + `hc rm` end the brainer's terminal, and `hcStartBrainer` starts it again
// with `claude --resume <id>` (--resume) or without (--reset). The conversation the new terminal resumes is named by
// the person (--resume) or read from the terminal's own claude: claude writes <config dir>/sessions/<pid>.json
// (MEASURED 2026-11-09: pid, sessionId, cwd, status), and `hc get` names the terminal's pid.
// shortcut: a brainer is replaced, never typed into, so --self/--await-idle/--in-pane (the pane's own typing) stay refused.

import (
	"encoding/json"
	"os"
	"regexp"
	"time"
)

var reHCAddr = regexp.MustCompile(`^t[0-9]+$`)

// hcEnd ends the brainer terminal addr, refusing one that is working (a turn would be cut) unless --force says so.
func (b *brainRun) hcEnd(addr string) bool {
	if addr == b.getenv("HC_TERMINAL_ID") { // the TUI's ownPaneGate: brain does not end the terminal it runs in
		b.errf("brain: %s is the terminal brain is running in, and brain does not kill its own terminal. Nothing was sent and nothing was restarted. Run this from ANOTHER terminal.\n", addr)
		return false
	}
	m, read := b.hcGet(addr)
	if !read { // an unread terminal is not a gone one: a second brainer must not be started beside a live one
		b.errf("brain: `hc get %s` could not be read, so whether it is still running is UNKNOWN. Nothing was ended or started.\n", addr)
		return false
	}
	if hcStr(m, "state") != "live" {
		return true // already gone
	}
	st, _ := m["status"].(jmap) // the status of the read that just succeeded, not a second read that may fail
	if hcStr(st, "state") == "working" && !b.force {
		b.errf("brain: %s is working: replacing it would cut a turn in flight. Wait for it, or add --force to --reset.\n", addr)
		return false
	}
	b.hc(brainCmd{NoOut: true}, "kill", addr)
	b.hc(brainCmd{NoOut: true}, "rm", addr)
	for i := 0; i < 10; i++ {
		if _, live := hcAgentGetJSON(addr); !live {
			return true
		}
		b.sleep(500 * time.Millisecond)
	}
	b.errf("brain: %s is still live after `hc kill` and `hc rm`; nothing was started beside it.\n", addr)
	return false
}

// hcBlockerGate is the TUI's blockerGate asked on the app: a --reset ends the conversation that knows what the brainer
// dispatched, so it is refused while executors that have not reported are addressed to this terminal, unless --force.
// The live agents are the app's executors (hcRunEntries, in herdr's pane shape); an hc that cannot answer refuses too,
// since an unread list is not an empty one.
func (b *brainRun) hcBlockerGate(addr string) bool {
	_, panes, answered := hcRunEntries()
	if !answered && !b.force {
		b.errf("brain: `hc ls` could not answer, so whether executors still report to %s is UNKNOWN. Nothing was sent and nothing was restarted. --force resets anyway.\n", addr)
		return false
	}
	raw, _ := json.Marshal(map[string]any{"result": map[string]any{"panes": panes}})
	return b.blockerGate(b.proj, addr, "--reset", string(raw))
}

// hcSessionOf is the claude session id of an app terminal, "" when it cannot be established: the pid `hc get` names,
// looked up in the account's sessions folder, accepted only when that file's cwd is the terminal's own.
func (b *brainRun) hcSessionOf(addr, acctDir string) string {
	m, ok := b.hcGet(addr)
	if !ok {
		return ""
	}
	return claudeSessionOf(bjRaw(m["pid"]), hcStr(m, "cwd"), acctDir, b.getenv("CLAUDE_CONFIG_DIR"), b.home+"/.claude")
}

// claudeSessionOf is the session id in <dir>/sessions/<pid>.json for the first config dir that has one whose cwd is the terminal's
// own ("" when none): the one reader of claude's own record, shared by `brain relaunch` and dispatch's session_resume.
func claudeSessionOf(pid, cwd string, dirs ...string) string {
	if !allDigits(pid) {
		return ""
	}
	for _, d := range dirs {
		raw, err := os.ReadFile(d + "/sessions/" + pid + ".json")
		if d == "" || err != nil {
			continue
		}
		v, parsed := bjParse(string(raw))
		s, isMap := v.(jmap)
		if parsed && isMap && sameDir(hcStr(s, "cwd"), cwd) && reUUID.MatchString(hcStr(s, "sessionId")) {
			return hcStr(s, "sessionId")
		}
	}
	return ""
}

// relaunchHC is brain_relaunch for the app: each live claude brainer terminal ended and started again resuming the
// conversation it holds. A terminal that is working, or whose session cannot be established, is skipped by name.
func (b *brainRun) relaunchHC(args []string) int {
	all, dry, targets, rc, done := b.relaunchArgs(args)
	if done {
		return rc
	}
	terms, ok := b.hcTerminals("label=brain")
	if !ok {
		return 1
	}
	n, fails := 0, 0
	for _, t := range terms {
		lane := b.laneOfCwd(t.cwd)
		if !all {
			hit := false
			for _, x := range targets {
				hit = hit || x == "pane:"+t.addr || (lane != "" && x == "lane:"+lane)
			}
			if !hit {
				continue
			}
		}
		info, live := hcAgentGetJSON(t.addr)
		if t.addr == b.getenv("HC_TERMINAL_ID") {
			b.outf("skipped %s (%s): it is the terminal brain is running in\n", t.addr, lane)
			continue
		}
		if !live || jqRawOr(info, "", "result", "agent", "agent") != "claude-code" {
			continue
		}
		n++
		if lane == "" || !b.laneKnown(lane) {
			b.outf("skipped %s: its cwd (%s) is not a lane directory under %s\n", t.addr, t.cwd, b.brain)
			continue
		}
		sid := b.hcSessionOf(t.addr, b.laneAccountDir(lane))
		if sid == "" {
			b.outf("skipped %s (%s): no session id could be read for it, so there is no conversation to resume\n", t.addr, lane)
			continue
		}
		if st := jqRawOr(info, "", "result", "agent", "agent_status"); st != "idle" && st != "done" {
			b.outf("skipped %s (%s): %s — a turn may be in flight, and a relaunch would cut it\n", t.addr, lane, orStr(st, "no status"))
			continue
		}
		_, conv := b.sessionTranscript(sid, b.laneAccountDir(lane))
		how := "resuming " + sid
		if !conv {
			how = "as a NEW conversation — " + sid + " has no transcript (it never had a turn), so there is nothing to resume"
		}
		if dry {
			b.outf("would relaunch %s (%s) %s\n", t.addr, lane, how)
			continue
		}
		res := b.run(brainCmd{Name: b.binDir + "/brain", Args: []string{lane, "--resume", sid}, Merge: true})
		if res.RC != 0 {
			b.outf("FAILED %s (%s): brain refused, rc=%d: %s\n", t.addr, lane, res.RC, tail2(brTrimNL(res.Out)))
			fails++
			continue
		}
		b.outf("relaunched %s (%s) %s\n", t.addr, lane, how)
	}
	if n == 0 {
		b.outf("brain relaunch: no open claude brainer matched.\n")
	}
	if fails != 0 {
		return 1
	}
	return 0
}
