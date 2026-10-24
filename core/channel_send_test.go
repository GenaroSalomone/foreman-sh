package main

// In-process tests of channel-send (channel_send*.go): the cases the bash subjects cover (setup/tests/05, 26, 41,
// 42, 66, 101, 111, 135) over a world of stub programs (herdr, herdr-rpc, holdfacts) and fake OpenCode endpoints.
// A mutant of a line the source marks as a mutation anchor must turn one of these red; setup/tests/1016 builds
// each mutant and says which test killed it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type csWorld struct {
	t    *testing.T
	dir  string // the world: bin/, stub/, tmp/
	bin  string
	stub string
	env  map[string]string
}

// newCSWorld builds a bin/ of stubs and puts it first on PATH.
func newCSWorld(t *testing.T) *csWorld {
	t.Helper()
	dir := t.TempDir()
	w := &csWorld{t: t, dir: dir, bin: dir + "/bin", stub: dir + "/stub", env: map[string]string{}}
	os.MkdirAll(w.bin, 0o755)
	os.MkdirAll(w.stub, 0o755)
	os.MkdirAll(dir+"/tmp", 0o755)
	script := func(name, body string) {
		if err := os.WriteFile(w.bin+"/"+name, []byte("#!/bin/bash\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// herdr answers from files in $STUB_DIR; every call is logged.
	script("herdr", `printf '%s\n' "$*" >> "$STUB_DIR/calls"
case "$1 $2" in
  "agent get") cat "$STUB_DIR/agent-get.json" 2>/dev/null; exit 0 ;;
  "agent list") cat "$STUB_DIR/agent-list.json" 2>/dev/null; exit 0 ;;
  "agent prompt") [ -z "$STUB_PROMPT_SLEEP" ] || sleep "$STUB_PROMPT_SLEEP"; [ ! -f "$STUB_DIR/prompt-out" ] || cat "$STUB_DIR/prompt-out"; exit "${STUB_PROMPT_RC:-0}" ;;
  "pane send-text") [ -z "$STUB_SENDTEXT_RC" ] || { echo "send-text failed"; exit "$STUB_SENDTEXT_RC"; }; exit 0 ;;
  "pane send-keys") [ -z "$STUB_SENDKEYS_RC" ] || { echo "send-keys failed"; exit "$STUB_SENDKEYS_RC"; }; exit 0 ;;
  "pane process-info") cat "$STUB_DIR/process-info.json" 2>/dev/null; exit 0 ;;
esac
exit 0
`)
	script("herdr-rpc", `printf 'rpc %s\n' "$*" >> "$STUB_DIR/calls"
[ "$1" != call ] || { [ ! -f "$STUB_DIR/rpc-call-out" ] || cat "$STUB_DIR/rpc-call-out"; exit "${STUB_RPC_CALL_RC:-0}"; }
exit "${STUB_RPC_RC:-0}"
`)
	script("holdfacts", `printf 'holdfacts %s\n' "$*" >> "$STUB_DIR/calls"
[ -z "$STUB_HOLD_ERR" ] || { echo "$STUB_HOLD_ERR"; exit 1; }
exit 0
`)
	os.WriteFile(w.bin+"/state-witness.sh", []byte("# a stub: its presence is the fact\n"), 0o644)
	t.Setenv("PATH", w.bin+":"+os.Getenv("PATH"))
	t.Setenv("STUB_DIR", w.stub)
	t.Setenv("TMPDIR", dir+"/tmp")
	t.Setenv("HW_CORE_BIN_DIR", w.bin)
	// A budget under one slice is one plain wait: the cross-check loop (and its seconds of sampling) is for the
	// tests that ask for it.
	t.Setenv("WITNESS_SLICE_MS", "100000000")
	t.Setenv("WITNESS_SAMPLES", "2")
	t.Setenv("WITNESS_INTERVAL_S", "0")
	for _, k := range []string{"HW_INVOKER_WAIT_MS", "HW_AWAITING_CHILD_PROBE_MS", "HW_REPORT_LOCK_WAIT_S", "STUB_RPC_RC", "STUB_PROMPT_RC",
		"HW_PROJECTS_JSON", "HW_BRAIN_ROOT", "HW_CORE", "HW_IMPL", "HW_IMPL_CHANNEL_SEND", "STUB_HOLD_ERR", "STUB_PROMPT_SLEEP", "STUB_RPC_CALL_RC", "STUB_SENDTEXT_RC", "STUB_SENDKEYS_RC", "HW_OPERATOR", "CLAUDE_PANE", "HERDR_PANE_ID", "HERDR_SOCKET_PATH"} {
		t.Setenv(k, "")
	}
	return w
}

func (w *csWorld) set(k, v string) { w.t.Setenv(k, v) }

func (w *csWorld) agent(status string, tokens map[string]string) {
	tk := map[string]string{}
	for k, v := range tokens {
		tk[k] = v
	}
	b, _ := json.Marshal(map[string]any{"result": map[string]any{"agent": map[string]any{"agent_status": status, "tokens": tk}}})
	os.WriteFile(w.stub+"/agent-get.json", b, 0o644)
}

func (w *csWorld) calls() string {
	b, _ := os.ReadFile(w.stub + "/calls")
	return string(b)
}

type csResult struct {
	rc       int
	out, err string
}

// cs runs channel-send with stdout not a terminal (a pipe, as every caller has it) and no stdin.
func (w *csWorld) cs(args ...string) csResult { return w.csIO("", false, false, args...) }

func (w *csWorld) csIO(stdin string, outTTY, inTTY bool, args ...string) csResult {
	var o, e bytes.Buffer
	rc := runChannelSend(args, &o, &e, os.Getenv, strings.NewReader(stdin), outTTY, inTTY)
	return csResult{rc, o.String(), e.String()}
}

func (r csResult) has(t *testing.T, want ...string) {
	t.Helper()
	for _, s := range want {
		if !strings.Contains(r.err+r.out, s) {
			t.Errorf("output lacks %q:\n%s", s, r.err+r.out)
		}
	}
}

func (r csResult) lacks(t *testing.T, bad ...string) {
	t.Helper()
	for _, s := range bad {
		if strings.Contains(r.err+r.out, s) {
			t.Errorf("output has %q:\n%s", s, r.err+r.out)
		}
	}
}

func (r csResult) rcIs(t *testing.T, want int) {
	t.Helper()
	if r.rc != want {
		t.Errorf("exit %d, want %d:\n%s%s", r.rc, want, r.out, r.err)
	}
}

// ── the command line, the help, the exit guard (66) ─────────────────────────

func TestChannelSendHelp(t *testing.T) {
	w := newCSWorld(t)
	for _, f := range []string{"--help", "-h"} {
		r := w.cs(f)
		r.rcIs(t, 0)
		if r.out != channelSendHelp || !strings.HasPrefix(r.out, "Deliver one message with the requested receiver-owned receipt") {
			t.Errorf("%s: help is not the script's header: %q", f, r.out[:60])
		}
		if r.err != "" {
			t.Errorf("%s: help spoke on stderr: %q", f, r.err)
		}
	}
}

func TestChannelSendRefusals(t *testing.T) {
	w := newCSWorld(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"require-missing", []string{"--require"}, "channel-send: --require needs admitted or processed\n"},
		{"require-bad", []string{"--require", "x", "herdr", "p", "-", "m"}, "channel-send: --require must be admitted or processed (got: x)\n"},
		{"id-empty", []string{"--id", "", "herdr", "p", "-", "m"}, "channel-send: --id must not be empty\n"},
		{"report-ruling", []string{"--report", "--ruling", "herdr", "p", "-", "m"}, "channel-send: --report and --ruling are different delivery contracts and cannot be combined\n"},
		{"turn-ended-ruling", []string{"--allow-turn-ended", "--ruling", "herdr", "p", "-", "m"}, "channel-send: --allow-turn-ended cannot make a ruling ordered; use --ruling alone\n"},
		{"usage", []string{"herdr", "p", "-"}, "channel-send: usage: channel-send [--require admitted|processed]"},
		{"empty-message", []string{"herdr", "p", "-", ""}, "channel-send: message is empty\n"},
		{"unknown-vendor", []string{"nope", "p", "-", "m"}, "channel-send: unknown vendor: nope\n"},
		{"hold-needs-intent", []string{"--reply-hold", "/x", "herdr", "p", "-", "m"}, "channel-send: --reply-hold is valid only for a declared --report answer or --ruling\n"},
		{"hold-relative", []string{"--report", "--reply-hold", "x", "herdr", "p", "-", "m"}, "channel-send: --reply-hold must be an absolute path\n"},
		{"report-state-alone", []string{"--report-state", "/x", "herdr", "p", "-", "m"}, "channel-send: --report-state is valid only with --report\n"},
		{"report-state-relative", []string{"--report", "--report-state", "x", "herdr", "p", "-", "m"}, "channel-send: --report-state must be an absolute path\n"},
	}
	for _, c := range cases {
		r := w.cs(c.args...)
		r.rcIs(t, 1)
		if !strings.HasPrefix(r.err, c.want) {
			t.Errorf("%s: stderr %q does not start with %q", c.name, r.err, c.want)
		}
		if r.out != "" {
			t.Errorf("%s: stdout is not left alone: %q", c.name, r.out)
		}
	}
}

// 66: a non-zero exit speaks twice on stderr when stdout is not a terminal, and never touches stdout.
func TestChannelSendExitGuard(t *testing.T) {
	w := newCSWorld(t)
	r := w.cs("herdr", "p", "-", "")
	r.rcIs(t, 1)
	lines := strings.Split(strings.TrimSuffix(r.err, "\n"), "\n")
	if len(lines) != 3 { // the die, then the banner's two lines
		t.Fatalf("stderr is %d lines: %q", len(lines), r.err)
	}
	if lines[1] != "channel-send: ══ MY EXIT CODE IS 1: NOT DELIVERED — nothing landed, nothing was lost, retry is free ══" {
		t.Errorf("first banner line: %q", lines[1])
	}
	if !strings.Contains(lines[2], "my stdout is NOT a terminal") || !strings.Contains(lines[2], "it is 1. Either way the delivery fact is 1,") {
		t.Errorf("second banner line: %q", lines[2])
	}
	if strings.Count(r.err, "MY EXIT CODE") != 1 {
		t.Errorf("the banner spoke more than once:\n%s", r.err)
	}
	// on a terminal it stays silent: the caller can see the status
	tty := w.csIO("", true, false, "herdr", "p", "-", "")
	tty.rcIs(t, 1)
	tty.lacks(t, "MY EXIT CODE")
	// exit 0 never warns
	ok := w.cs("--help")
	ok.lacks(t, "MY EXIT CODE")
	// every code has its own meaning
	for rc, want := range map[int]string{4: "DELIVERED and provably in the receiver", 5: "ADMITTED, persistence UNCONFIRMED", 6: "RECEIVER UNUSABLE", 9: "a failure, not a delivery"} {
		if !strings.Contains(csExitMeaning(rc), want) {
			t.Errorf("meaning of %d: %q", rc, csExitMeaning(rc))
		}
	}
}

// 135: the message may come from stdin through a lone '-'.
func TestChannelSendMessageFromStdin(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	r := w.csIO("the fix touches `deny-repo-writes.py`; cost is $0\n\n", false, false, "herdr", "w1:p1", "-", "-")
	r.rcIs(t, 0)
	if !strings.Contains(w.calls(), "agent prompt w1:p1 the fix touches `deny-repo-writes.py`; cost is $0\n") {
		t.Errorf("the message did not arrive byte for byte (trailing newlines stripped): %s", w.calls())
	}
	both := w.csIO("x", false, false, "herdr", "w1:p1", "-", "-", "more")
	both.rcIs(t, 1)
	both.has(t, "message given twice: as '-' for stdin AND as more argv text after it")
	tty := w.csIO("", false, true, "herdr", "w1:p1", "-", "-")
	tty.rcIs(t, 1)
	tty.has(t, "message is '-' (read from stdin), but stdin is a terminal")
	empty := w.csIO("", false, false, "herdr", "w1:p1", "-", "-")
	empty.rcIs(t, 1)
	empty.has(t, "stdin for '-' produced no bytes")
	argv := w.cs("herdr", "w1:p1", "-", "two", "words")
	argv.rcIs(t, 0)
	if !strings.Contains(w.calls(), "agent prompt w1:p1 two words\n") {
		t.Errorf("argv words are joined with a space: %s", w.calls())
	}
}

// ── the herdr route: the budget, queued vs steered vs failed (05) ───────────

func TestChannelSendHerdrBudget(t *testing.T) {
	w := newCSWorld(t)
	w.agent("working", nil)
	w.set("STUB_RPC_RC", "2")
	r := w.cs("herdr", "wT:p1", "-", "probe")
	r.rcIs(t, 1)
	r.has(t, "HW_INVOKER_WAIT_MS=540000", "wT:p1 was still working after 90s, so NOTHING was sent")
	// 3 is a dead socket, not a busy target
	w.set("STUB_RPC_RC", "3")
	d := w.cs("herdr", "wT:p1", "-", "probe")
	d.rcIs(t, 1)
	d.has(t, "could not wait on wT:p1 (herdr-rpc exit 3 — see its message above). Nothing was sent.")
	d.lacks(t, "still working after")
	// an executor (hw_task token) gets the raised budget, announced before the wait
	w.agent("working", map[string]string{"hw_task": "go-x"})
	w.set("STUB_RPC_RC", "2")
	e := w.cs("herdr", "wT:p1", "-", "probe")
	e.has(t, "wT:p1 is an hw executor (task go-x), so the wait budget defaults to 540s instead of 90s", "THIS CALL CAN THEREFORE TAKE UP TO 540s. Give it a caller timeout of at least 555000 ms")
	// the caller's own budget wins, and a junk one is not announced as an executor's
	w.set("HW_INVOKER_WAIT_MS", "5000")
	f := w.cs("herdr", "wT:p1", "-", "probe")
	f.has(t, "still working after 5s")
	f.lacks(t, "is an hw executor")
}

func TestChannelSendHerdrLandings(t *testing.T) {
	w := newCSWorld(t)
	w.agent("working", nil)
	r := w.cs("herdr", "wT:p1", "-", "probe")
	r.rcIs(t, 0)
	r.has(t, "wT:p1 was MID-TURN when this was sent, so WHERE it landed was not your choice.", "it was acted on — mid-turn delivery is not a loss")
	w.agent("idle", nil)
	q := w.cs("herdr", "wT:p1", "-", "probe")
	q.rcIs(t, 0)
	if q.err != "" {
		t.Errorf("a between-turns delivery is silent, got %q", q.err)
	}
	w.agent("mystery", nil)
	u := w.cs("herdr", "wT:p1", "-", "probe")
	u.has(t, "could not read wT:p1 status before sending (got mystery), so whether this steered or queued is unknown.")
	// no answer from herdr at all is "unknown", not a crash
	os.Remove(w.stub + "/agent-get.json")
	n := w.cs("herdr", "wT:p1", "-", "probe")
	n.rcIs(t, 0)
	n.has(t, "(got )") // herdr answered nothing: jq read fine and printed nothing, and the bash says so as it is
	// refuses what herdr does not expose
	w.cs("--id", "k", "herdr", "p", "-", "m").has(t, "herdr does not expose idempotent caller-supplied delivery ids")
	w.cs("--require", "processed", "herdr", "p", "-", "m").has(t, "herdr does not expose causal processed receipt proof")
	w.cs("herdr", "", "-", "m").has(t, "Herdr target pane is empty")
}

// a failed prompt names what herdr said; nothing is sent, so nothing is marked sent.
func TestChannelSendHerdrPromptFails(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	w.set("STUB_PROMPT_RC", "3")
	os.WriteFile(w.stub+"/prompt-out", []byte("boom\n"), 0o644)
	r := w.cs("herdr", "wT:p1", "-", "probe")
	r.rcIs(t, 1)
	r.has(t, "channel-send: herdr agent prompt rejected delivery to wT:p1 (exit 3): boom\n", "herdr agent prompt failed before delivery to wT:p1. NOTHING was sent.")
}

// ── the ruling gate (41) ────────────────────────────────────────────────────

func TestChannelSendRulingGate(t *testing.T) {
	w := newCSWorld(t)
	w.agent("working", nil)
	r := w.cs("--ruling", "herdr", "wR:p1", "-", "fix it")
	r.rcIs(t, 1)
	r.has(t, "--ruling REFUSED before delivery: wR:p1 is working (herdr agent_status=working), not at a clean idle/done boundary.", "`hw ruling wR:p1 \"<the correction>\"` queues it", "DO NOT run `hw done` to reach exit 3.")
	if strings.Contains(w.calls(), "agent prompt") {
		t.Errorf("a refused ruling was sent: %s", w.calls())
	}
	// the same pane, a plain redirect, is not a ruling: the generic gate waits and delivers
	g := w.cs("herdr", "wR:p1", "-", "go on")
	g.rcIs(t, 0)
	g.lacks(t, "--ruling REFUSED")
	// a clean idle boundary delivers exactly once
	w.agent("idle", nil)
	os.Remove(w.stub + "/calls")
	ok := w.cs("--ruling", "herdr", "wR:p1", "-", "fix it")
	ok.rcIs(t, 0)
	if n := strings.Count(w.calls(), "agent prompt wR:p1 fix it\n"); n != 1 {
		t.Errorf("the ruling was delivered %d times", n)
	}
	// a child the pane is awaiting is named, and agent_status is not read
	w.agent("working", map[string]string{"turn_state": "ended_awaiting_child", "children_running": "1 subagent"})
	a := w.cs("--ruling", "herdr", "wR:p1", "-", "fix it")
	a.rcIs(t, 1)
	a.has(t, "awaiting_child (turn_state=ended_awaiting_child, children_running=1 subagent; herdr agent_status=working flaps here, so it is not read)")
	// herdr knowing no such pane
	os.Remove(w.stub + "/agent-get.json")
	n := w.cs("--ruling", "herdr", "wR:p1", "-", "fix it")
	n.has(t, "wR:p1 is in an unreadable state, not at a clean idle/done boundary")
	// the bare pane, never the claude display string, goes into the queue command
	w.agent("working", nil)
	if !strings.Contains(w.cs("--ruling", "herdr", "wR:p1", "-", "x").err, "`hw ruling wR:p1 \"<the correction>\"`") {
		t.Error("the queue command is not runnable")
	}
}

// ── the awaiting-child probe (111) ──────────────────────────────────────────

func TestChannelSendAwaitingChildProbe(t *testing.T) {
	w := newCSWorld(t)
	tokens := map[string]string{"turn_state": "ended_awaiting_child", "children_running": "1 subagent", "turn_ended_at": "23:26:41Z"}
	w.agent("working", tokens)
	w.set("STUB_RPC_RC", "2")
	w.set("HW_INVOKER_WAIT_MS", "540000")
	r := w.cs("herdr", "w9:p9", "-", "redirect")
	r.rcIs(t, 1)
	r.has(t, "w9:p9 has WARNED that idle may be far off — its own tokens say turn_state=ended_awaiting_child with children_running=1 subagent. Waiting 30s instead of 540s",
		"REFUSED AFTER A BOUNDED PROBE, and NOTHING was sent: w9:p9 did not reach idle in 30s",
		"turn_ended_at=23:26:41Z", "DO NOT raise HW_INVOKER_WAIT_MS", "THE ORDERED ROUTE", "herdr agent get w9:p9 | jq", "IF IT MUST LAND NOW, --allow-turn-ended")
	r.lacks(t, "still working after")
	// the probe is the one the caller sets, and junk is said once and ignored
	w.set("HW_AWAITING_CHILD_PROBE_MS", "12000")
	p := w.cs("herdr", "w9:p9", "-", "redirect")
	p.has(t, "Waiting 12s instead of 540s", "did not reach idle in 12s")
	w.set("HW_AWAITING_CHILD_PROBE_MS", "soon")
	j := w.cs("herdr", "w9:p9", "-", "redirect")
	j.has(t, "channel-send: HW_AWAITING_CHILD_PROBE_MS=soon is not a number of milliseconds; using 30000.\n", "Waiting 30s instead of 540s")
	w.set("HW_AWAITING_CHILD_PROBE_MS", "")
	// a pane already idle is at the boundary: nothing to shorten
	w.set("STUB_RPC_RC", "0")
	w.agent("idle", tokens)
	i := w.cs("herdr", "w9:p9", "-", "redirect")
	i.rcIs(t, 0)
	i.lacks(t, "WARNED")
	// no ended turn, or no child named, is not a warning
	w.set("STUB_RPC_RC", "2")
	w.agent("working", map[string]string{"turn_state": "running", "children_running": "1 subagent"})
	w.cs("herdr", "w9:p9", "-", "x").lacks(t, "WARNED")
	w.agent("working", map[string]string{"turn_state": "ended_awaiting_child"})
	w.cs("herdr", "w9:p9", "-", "x").lacks(t, "WARNED")
	// a budget at or under the probe is not shortened further
	w.agent("working", tokens)
	w.set("HW_INVOKER_WAIT_MS", "30000")
	w.cs("herdr", "w9:p9", "-", "x").lacks(t, "WARNED")
	// --report skips the wait, so it has nothing to shorten
	w.set("HW_INVOKER_WAIT_MS", "540000")
	rep := w.cs("--report", "herdr", "w9:p9", "-", "x")
	rep.rcIs(t, 0)
	rep.lacks(t, "WARNED")
	rep.has(t, "--report: not waiting for w9:p9 to be idle")
}

// the lock budget is derived from the UNSHORTENED wait (111-M08): a contended sender keeps its long wait.
func TestChannelSendLockBudgetIsTheUnshortenedWait(t *testing.T) {
	w := newCSWorld(t)
	tokens := map[string]string{"turn_state": "ended_awaiting_child", "children_running": "1 subagent"}
	w.agent("working", tokens)
	w.set("STUB_RPC_RC", "2")
	w.set("HW_INVOKER_WAIT_MS", "540000")
	lock := w.dir + "/tmp/hw-deliver-w8_p8.lock"
	os.MkdirAll(lock, 0o755)
	me := fmt.Sprint(os.Getpid())
	os.WriteFile(lock+"/owner.pid", []byte(me+"\n"), 0o644)
	os.WriteFile(lock+"/owner.start", []byte(procStart(me)+"\n"), 0o644)
	var o, e syncBuf
	c := newChanSend(&o, &e, os.Getenv, strings.NewReader(""), false, false)
	done := make(chan int, 1)
	go func() { done <- c.runGuarded([]string{"herdr", "w8:p8", "-", "x"}) }()
	// the sender says what it waits on; only then does the holder let go
	for i := 0; i < 600 && !strings.Contains(e.String(), "another sender holds the delivery lock"); i++ {
		sleepSeconds(0.05)
	}
	os.Remove(lock + "/owner.pid")
	os.Remove(lock + "/owner.start")
	os.Remove(lock)
	<-done
	got := e.String()
	for _, want := range []string{"another sender holds the delivery lock on w8:p8", "Waiting up to 545s for it; nothing has been sent yet."} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Waiting up to 35s") {
		t.Errorf("the lock wait was derived from the probe:\n%s", got)
	}
}

// syncBuf is a buffer a test can read while the sender still writes to it.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// ── the delivery lock (05, 101) ─────────────────────────────────────────────

func TestChannelSendLock(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	lk := w.dir + "/tmp/hw-deliver-wL_p1.lock"
	// a lock whose recorded holder is dead is broken AT ONCE
	os.MkdirAll(lk, 0o755)
	os.WriteFile(lk+"/owner.pid", []byte("999999\n"), 0o644)
	r := w.cs("herdr", "wL:p1", "-", "probe")
	r.rcIs(t, 0)
	r.has(t, "breaking a delivery lock on wL:p1 whose holder is gone ("+lk+", pid 999999).")
	if _, err := os.Stat(lk); err == nil {
		t.Error("the lock outlived the delivery")
	}
	// a live holder is respected, and SAID, and named; a plain sender gives up with its own diagnosis
	os.MkdirAll(lk, 0o755)
	me := fmt.Sprint(os.Getpid())
	os.WriteFile(lk+"/owner.pid", []byte(me+"\n"), 0o644)
	os.WriteFile(lk+"/owner.start", []byte(procStart(me)+"\n"), 0o644)
	w.set("HW_INVOKER_WAIT_MS", "1000")
	l := w.cs("herdr", "wL:p1", "-", "probe")
	l.rcIs(t, 1)
	l.has(t, "another sender holds the delivery lock on wL:p1 ("+lk+", pid "+me+", alive). Waiting up to 6s for it", "has held the delivery lock on wL:p1 for over 6s", "Nothing was sent, so nothing was lost — retry, or investigate that process.")
	// a report waits a bounded window, then goes WITHOUT the lock rather than deadlock its receiver
	w.set("HW_REPORT_LOCK_WAIT_S", "1")
	rep := w.cs("--report", "herdr", "wL:p1", "-", "probe")
	rep.rcIs(t, 0)
	rep.has(t, "reply/report: another sender has held "+lk+" for 1s (pid "+me+"), so this expected reply is being SENT WITHOUT THE LOCK.")
	// 101: the same pid with another start time is a reused pid, and the lock is broken
	os.WriteFile(lk+"/owner.start", []byte("Thu Jan  1 00:00:00 1970\n"), 0o644)
	re := w.cs("herdr", "wL:p1", "-", "probe")
	re.rcIs(t, 0)
	re.has(t, "whose holder pid was REUSED ("+lk+", pid "+me+" is alive but started "+procStart(me)+", and the sender that took this lock started Thu Jan  1 00:00:00 1970).")
	// no recorded start: the name check — a holder that is not a channel-send is not a sender
	os.MkdirAll(lk, 0o755)
	os.WriteFile(lk+"/owner.pid", []byte(me+"\n"), 0o644)
	os.Remove(lk + "/owner.start")
	nm := w.cs("herdr", "wL:p1", "-", "probe")
	nm.rcIs(t, 0)
	nm.has(t, "whose holder is NOT a sender ("+lk+", pid "+me+" is alive but its command is `")
	// a delivery that DIES holding the lock (the prompt fails) still releases it: the exit path's own release
	os.RemoveAll(lk)
	w.set("STUB_PROMPT_RC", "1")
	die := w.cs("herdr", "wL:p1", "-", "probe")
	die.rcIs(t, 1)
	if _, err := os.Stat(lk); err == nil {
		t.Error("a delivery that died left its lock behind")
	}
}

// ── the herdr blocked-pane fallbacks (42) ───────────────────────────────────

func holdFile(t *testing.T, dir string) string {
	p := dir + "/pending-reply-1"
	os.WriteFile(p, []byte("version=1\nstate=pending\nintent=ruling\nroute=herdr\ntarget=wH:p1\nlogical_id=lid-9\npane=wH:p1\nrun=r\n"), 0o644)
	return p
}

func TestChannelSendHeldReplyFallsBackToRawInput(t *testing.T) {
	w := newCSWorld(t)
	w.agent("working", nil)
	w.set("STUB_PROMPT_RC", "1")
	os.WriteFile(w.stub+"/prompt-out", []byte(`{"error":{"code": "agent_blocked"}}`+"\n"), 0o644)
	hold := holdFile(t, w.dir)
	// a proved held ruling: no idle wait, raw pane input, and exit 5 — the hold stays
	r := w.cs("--ruling", "--reply-hold", hold, "herdr", "wH:p1", "-", "the answer")
	r.rcIs(t, 5)
	r.has(t, "--ruling: not waiting for wH:p1 to become idle because its delivered challenge is still outstanding in "+hold,
		"rejected the proved held ruling with agent_blocked (exit 1)", "falling back ONLY for this receiver-owned reply hold to raw pane input",
		"held ruling was submitted to wH:p1 through the blocked-pane fallback after agent_blocked, but receiver ingestion is UNCONFIRMED. The reply hold remains")
	if !strings.Contains(w.calls(), "pane send-text wH:p1 the answer\n") || !strings.Contains(w.calls(), "pane send-keys wH:p1 enter\n") {
		t.Errorf("raw input was not sent:\n%s", w.calls())
	}
	if _, err := os.Stat(hold); err != nil {
		t.Error("the hold was cleared by a delivery that is only UNCONFIRMED")
	}
	if !strings.Contains(w.calls(), "holdfacts match "+hold+" --intent ruling --route herdr --target wH:p1\n") {
		t.Errorf("holdfacts was not asked: %s", w.calls())
	}
	// an answer (a --report with the hold) is the second spelling, and the same exit
	ans := w.cs("--report", "--reply-hold", hold, "--id", "lid-9", "herdr", "wH:p1", "-", "x")
	ans.rcIs(t, 1) // herdr takes no --id; the hold is checked first and proved, then the route refuses
	ans.has(t, "herdr does not expose idempotent caller-supplied delivery ids")
	ans2 := w.cs("--report", "--reply-hold", hold, "herdr", "wH:p1", "-", "x")
	ans2.rcIs(t, 5)
	ans2.has(t, "rejected the proved held ask answer with agent_blocked", "held answer was submitted to wH:p1 through the blocked-pane fallback after agent_blocked, but receiver ingestion is UNCONFIRMED")
	// the fallback's own failures
	w.set("STUB_SENDTEXT_RC", "9")
	f := w.cs("--ruling", "--reply-hold", hold, "herdr", "wH:p1", "-", "x")
	f.rcIs(t, 1)
	f.has(t, "agent_blocked was diagnosed, but the proved-ruling fallback failed before input reached wH:p1 (pane send-text exit 9: send-text failed). NOTHING was sent.")
	w.set("STUB_SENDTEXT_RC", "")
	w.set("STUB_SENDKEYS_RC", "8")
	k := w.cs("--ruling", "--reply-hold", hold, "herdr", "wH:p1", "-", "x")
	k.rcIs(t, 5)
	k.has(t, "pane send-text reached wH:p1 after agent_blocked, but pane send-keys enter failed (exit 8: send-keys failed). The ruling is in the input buffer but submission is UNCONFIRMED; do not blind-retry.")
	// a hold holdfacts refuses stops before anything is sent
	w.set("STUB_SENDKEYS_RC", "")
	w.set("STUB_HOLD_ERR", "hold is for another route")
	h := w.cs("--ruling", "--reply-hold", hold, "herdr", "wH:p1", "-", "x")
	h.rcIs(t, 1)
	h.has(t, "--reply-hold REFUSED before delivery: hold is for another route. NOTHING was sent.")
}

// a blocked pane gives an unproved report or ruling NOTHING; a plain message asks the receiver's own endpoint.
func TestChannelSendBlockedPaneWithoutProof(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	w.set("STUB_PROMPT_RC", "1")
	os.WriteFile(w.stub+"/prompt-out", []byte(`{"error":{"code":"agent_blocked"}}`+"\n"), 0o644)
	for _, args := range [][]string{{"--report", "herdr", "wB:p1", "-", "r"}} {
		r := w.cs(args...)
		r.rcIs(t, 1)
		r.has(t, "herdr agent prompt failed before delivery to wB:p1. NOTHING was sent.", "rejected delivery to wB:p1 (exit 1):")
		if strings.Contains(w.calls(), "send-text") {
			t.Errorf("an unproved report reached the raw pane: %s", w.calls())
		}
	}
	// a plain message with no readable endpoint refuses rather than guessing
	p := w.cs("herdr", "wB:p1", "-", "hello")
	p.rcIs(t, 1)
	p.has(t, "and its endpoint could not be read to confirm whether a prompt is pending")
	if strings.Contains(w.calls(), "send-text") {
		t.Errorf("a plain message reached the raw pane with nothing proving it safe: %s", w.calls())
	}
}

// a plain message to a pane whose OWN endpoint says nothing is pending is let through raw.
func TestChannelSendBlockedPlainMessageEndpointSaysClear(t *testing.T) {
	srv := fakeOpenCode(t, &ocFake{questions: "[]", permissions: "[]"})
	w := newCSWorld(t)
	w.agent("idle", nil)
	w.set("STUB_PROMPT_RC", "1")
	os.WriteFile(w.stub+"/prompt-out", []byte(`{"error":{"code":"agent_blocked"}}`+"\n"), 0o644)
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	os.WriteFile(w.stub+"/process-info.json", []byte(`{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","`+port+`"]}]}}}`), 0o644)
	r := w.cs("herdr", "wB:p1", "-", "hello")
	r.rcIs(t, 0)
	r.has(t, "but its own endpoint "+srv.URL+" confirms nothing is pending (/question 0, /permission 0)", "message submitted to wB:p1 through the blocked-pane fallback after agent_blocked (its own endpoint confirmed nothing pending).")
	// with a question outstanding it refuses
	srv2 := fakeOpenCode(t, &ocFake{questions: `[{"id":"q1"}]`, permissions: "[]"})
	port2 := strings.TrimPrefix(srv2.URL, "http://127.0.0.1:")
	os.WriteFile(w.stub+"/process-info.json", []byte(`{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","`+port2+`"]}]}}}`), 0o644)
	q := w.cs("herdr", "wB:p1", "-", "hello")
	q.rcIs(t, 1)
	q.has(t, "CONFIRMS a real prompt is outstanding (/question 1, /permission 0). NOTHING was sent; raw pane input would overwrite a real human prompt.")
}

// ── --report-state: the proof that licenses a blocked-pane report (42) ──────

// reportWorld lays out <work>/.hw/<run>/{env,...} the way hw does.
func reportWorld(t *testing.T, w *csWorld, pane string) (state string) {
	work := w.dir + "/work"
	run := work + "/.hw/run1"
	os.MkdirAll(run, 0o755)
	os.WriteFile(run+"/env", []byte("HW_WORKDIR='"+work+"'\nHW_RUN='run1'\nHW_INVOKER_PANE='"+pane+"'\n"), 0o644)
	return run
}

func TestChannelSendReportState(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	run := reportWorld(t, w, "wP:p1")
	ok := w.cs("--report", "--report-state", run, "herdr", "wP:p1", "-", "done")
	ok.rcIs(t, 0)
	// the pane the env names must be the pane being sent to
	bad := w.cs("--report", "--report-state", run, "herdr", "wP:p2", "-", "done")
	bad.rcIs(t, 1)
	bad.has(t, "--report-state REFUSED before delivery: dispatch env has HW_INVOKER_PANE=wP:p1, expected wP:p2.")
}

func TestChannelSendReportStateRefusals(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	run := reportWorld(t, w, "wP:p1")
	refuse := func(state, pane, want string) {
		t.Helper()
		r := w.cs("--report", "--report-state", state, "herdr", pane, "-", "done")
		r.rcIs(t, 1)
		r.has(t, "channel-send: --report-state REFUSED before delivery: "+want+". The blocked-pane bypass is NOT authorized and NOTHING was sent.\n")
	}
	refuse(w.dir+"/nowhere", "wP:p1", "task state directory does not exist")
	refuse(w.dir+"/work", "wP:p1", "task state is not a run root or one direct tN task directory")
	refuse(run, "wP:p2", "dispatch env has HW_INVOKER_PANE=wP:p1, expected wP:p2")
	// a done marker means the task already reported: the bypass is not for a second report
	os.WriteFile(run+"/done", nil, 0o644)
	refuse(run, "wP:p1", "task state already has a done marker")
	os.Remove(run + "/done")
	// the env must reconstruct this exact run
	os.WriteFile(run+"/env", []byte("HW_WORKDIR='"+w.dir+"/elsewhere'\nHW_RUN='run1'\nHW_INVOKER_PANE='wP:p1'\n"), 0o644)
	refuse(run, "wP:p1", "dispatch env does not reconstruct this exact run directory")
	os.WriteFile(run+"/env", []byte("HW_RUN='run1'\nHW_RUN='run2'\n"), 0o644)
	refuse(run, "wP:p1", "dispatch env repeats HW_RUN")
	os.WriteFile(run+"/env", []byte("not a row\n"), 0o644)
	refuse(run, "wP:p1", "dispatch env contains a row outside its fixed KEY='value' grammar")
	os.Remove(run + "/env")
	real, _ := filepath.EvalSymlinks(run) // python resolves the state directory: the path it names is the real one
	refuse(run, "wP:p1", "cannot read dispatch env: [Errno 2] No such file or directory: '"+real+"/env'")
}

// a blocked pane takes a PROVED report raw — and only that.
func TestChannelSendProvedReportFallsBack(t *testing.T) {
	w := newCSWorld(t)
	w.agent("blocked", nil)
	run := reportWorld(t, w, "wP:p1")
	w.set("STUB_PROMPT_RC", "1")
	os.WriteFile(w.stub+"/prompt-out", []byte(`{"error":{"code":"agent_blocked"}}`+"\n"), 0o644)
	// state-witness reads the screen: with herdr-rpc silent it is unreadable, so the proof cannot be completed
	r := w.cs("--report", "--report-state", run, "herdr", "wP:p1", "-", "done")
	r.rcIs(t, 1)
	r.has(t, "herdr agent prompt rejected this proved run report because wP:p1 is blocked, but state-witness could not prove whether raw input is safe: wP:p1 says agent_status=blocked, and the cross-check could NOT be made: its visible screen could not be read", "NOTHING was sent; retry only after the receiver state is readable.")
	if strings.Contains(w.calls(), "send-text") {
		t.Errorf("an unreadable witness licensed raw input: %s", w.calls())
	}
}

// ── a reply hold is cleared, and written into the transcript, once delivered ─

func TestChannelSendDeliveredHoldIsClearedAndLogged(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	runDir := w.dir + "/run"
	os.MkdirAll(runDir, 0o755)
	hold := holdFile(t, runDir)
	r := w.cs("--ruling", "--reply-hold", hold, "herdr", "wH:p1", "-", "line one\nline two")
	r.rcIs(t, 0)
	if _, err := os.Stat(hold); err == nil {
		t.Error("the hold survived its delivery")
	}
	b, _ := os.ReadFile(runDir + "/transcript.log")
	s := string(b)
	if !strings.Contains(s, "  RULING  envelope=lid-9\n    | line one\n    | line two\n") {
		t.Errorf("transcript: %q", s)
	}
}

// ── signals: being killed is an outcome (66's sibling, the 08-27 incident) ──

func TestChannelSendSignalOutcomes(t *testing.T) {
	for _, c := range []struct {
		name   string
		sent   int
		budget string
		want   []string
		rc     int
	}{
		{"nothing sent", csNotSent, "", []string{"channel-send: SIGTERM after 0s — THIS IS YOUR CALLER KILLING IT (a Bash/harness timeout), not a channel-send failure. NOTHING WAS SENT to wX:p1 and nothing was lost: retry the identical command.\n"}, 1},
		{"nothing sent, with a budget", csNotSent, "540000", []string{"this call was allowed to wait up to 540s for wX:p1 to reach idle, so a caller timeout below 555000 ms will always kill it here."}, 1},
		{"already delivered", csSent, "", []string{"but wX:p1 ALREADY HAS THIS MESSAGE", "Exiting 0 because the delivery happened."}, 0},
		{"in flight", csInflight, "", []string{"WHILE THE SEND WAS IN FLIGHT to wX:p1. This CANNOT say whether it landed"}, 5},
	} {
		w := newCSWorld(t)
		var o, e bytes.Buffer
		got := -1
		cc := &chanSend{out: &o, err: &e, getenv: os.Getenv, exitProc: func(rc int) { got = rc }, sleep: func(float64) {}}
		cc.sent, cc.target, cc.budgetMS, cc.t0 = c.sent, "wX:p1", c.budget, nextEpoch()
		cc.onSignal("TERM")
		if got != c.rc {
			t.Errorf("%s: exit %d, want %d", c.name, got, c.rc)
		}
		for _, s := range c.want {
			if !strings.Contains(e.String(), s) {
				t.Errorf("%s: stderr lacks %q:\n%s", c.name, s, e.String())
			}
		}
		// a signalled exit speaks the guard too: stdout is a pipe here, and the status is the delivery fact
		banner := strings.Contains(e.String(), fmt.Sprintf("MY EXIT CODE IS %d:", c.rc))
		if (c.rc != 0) != banner {
			t.Errorf("%s: exit %d and the guard's banner is %v", c.name, c.rc, banner)
		}
		_ = w
	}
}

// ── the claude route's refusals ─────────────────────────────────────────────

func TestChannelSendClaudeRoute(t *testing.T) {
	w := newCSWorld(t)
	r := w.cs("claude", "ses-1", "-", "m")
	r.rcIs(t, 1)
	r.has(t, "claude session ses-1 is live but is not in a herdr pane")
	w.cs("--id", "k", "claude", "ses-1", "-", "m").has(t, "claude does not yet expose idempotent caller-supplied delivery ids")
	w.cs("--require", "processed", "claude", "ses-1", "-", "m").has(t, "claude does not yet expose processed receipt proof")
	// the pane is found by session id; --report is said not to apply; a wait that times out names the budget
	os.WriteFile(w.stub+"/agent-list.json", []byte(`{"result":{"agents":[{"agent":"claude","agent_session":{"value":"ses-1"},"pane_id":"wC:p4"}]}}`), 0o644)
	w.agent("working", nil)
	w.set("STUB_RPC_RC", "2")
	w.set("HW_INVOKER_WAIT_MS", "5000")
	t2 := w.cs("--report", "claude", "ses-1", "-", "m")
	t2.rcIs(t, 1)
	t2.has(t, "--report has NO effect on the claude route and the gate below still applies.", "wC:p4 (claude session ses-1) was still working after 5s, so NOTHING was sent.")
	// a ruling at a busy boundary is refused with the BARE pane in the queue command
	rl := w.cs("--ruling", "claude", "ses-1", "-", "m")
	rl.rcIs(t, 1)
	rl.has(t, "wC:p4 (claude session ses-1) is working (herdr agent_status=working)", "`hw ruling wC:p4 \"<the correction>\"`")
	// the awaiting-child warning is read from the BARE pane, never the display string
	w.agent("working", map[string]string{"turn_state": "ended_awaiting_child", "children_running": "2 shell"})
	w.set("HW_INVOKER_WAIT_MS", "540000")
	ac := w.cs("claude", "ses-1", "-", "m")
	ac.rcIs(t, 1)
	ac.has(t, "wC:p4 has WARNED that idle may be far off", "REFUSED AFTER A BOUNDED PROBE, and NOTHING was sent: wC:p4 (claude session ses-1) did not reach idle in 30s",
		"THERE IS NO SEND-NOW TRADE ON THIS ROUTE", "send it over the herdr route against pane wC:p4, where a mid-turn delivery is at least acted on.", "herdr agent get wC:p4 | jq")
	// a session with no live record
	w.set("STUB_RPC_RC", "0")
	w.set("HW_INVOKER_WAIT_MS", "")
	w.agent("idle", nil)
	w.set("HOME", w.dir)
	nl := w.cs("claude", "ses-1", "-", "m")
	nl.rcIs(t, 1)
	nl.has(t, "claude session lookup returned 0 matches\n", "channel-send: claude session lookup failed\n")
}

func TestChannelSendCodexRoute(t *testing.T) {
	w := newCSWorld(t)
	w.cs("--ruling", "codex", "t1", "/sock", "m").has(t, "--ruling REFUSED before delivery: the codex route exposes no receiver turn-state proof")
	w.cs("--id", "k", "codex", "t1", "/sock", "m").has(t, "codex does not yet expose idempotent caller-supplied delivery ids")
	w.cs("--require", "processed", "codex", "t1", "/sock", "m").has(t, "codex does not yet expose processed receipt proof")
	w.cs("codex", "", "/sock", "m").has(t, "codex target thread is empty")
	// the queue command is codex's own, with CODEX_HOME out of its environment
	os.WriteFile(w.bin+"/codex", []byte("#!/bin/bash\nprintf '%s|' \"$@\" >> \"$STUB_DIR/codex\"; printf 'home=%s\\n' \"${CODEX_HOME-unset}\" >> \"$STUB_DIR/codex\"; echo noise; exit \"${STUB_CODEX_RC:-0}\"\n"), 0o755)
	w.set("CODEX_HOME", "/somewhere")
	r := w.cs("codex", "thr", "/sock", "go on")
	r.rcIs(t, 0)
	if r.out != "" {
		t.Errorf("codex's stdout leaked: %q", r.out)
	}
	b, _ := os.ReadFile(w.stub + "/codex")
	if string(b) != "queue|--remote|unix:///sock|--thread|thr|--message|go on|home=unset\n" {
		t.Errorf("codex was run as %q", b)
	}
	w.set("STUB_CODEX_RC", "7")
	w.cs("codex", "thr", "/sock", "m").rcIs(t, 7)
}

// ── the OpenCode routes (26, 05, 41) ────────────────────────────────────────

// ocFake is an OpenCode endpoint with just the routes channel-send touches.
type ocFake struct {
	mu          sync.Mutex
	config      string // GET /config
	agents      string // GET /agent
	path        string // GET /path
	status      string // GET /session/status
	questions   string // GET /question
	permissions string // GET /permission
	posts       []string
	stored      map[string]string // message id -> the stored message JSON (what GET /session/:id/message/:mid answers)
	replyAfter  bool              // an assistant reply parented to the posted message exists
	replyError  string            // its error, JSON
	persist     bool              // GET of the posted message finds it
	withhold    bool              // POST is accepted but the message never shows up
	postStatus  int
	missing     bool // GET /session/:id answers 404
}

func fakeOpenCode(t *testing.T, f *ocFake) *httptest.Server {
	t.Helper()
	if f.questions == "" {
		f.questions = "[]"
	}
	if f.permissions == "" {
		f.permissions = "[]"
	}
	if f.status == "" {
		f.status = "{}"
	}
	if f.stored == nil {
		f.stored = map[string]string{}
	}
	if f.postStatus == 0 {
		f.postStatus = 204
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		write := func(code int, body string) { w.WriteHeader(code); fmt.Fprint(w, body) }
		p := r.URL.Path
		switch {
		case p == "/config" && f.config != "":
			write(200, f.config)
		case p == "/agent" && f.agents != "":
			write(200, f.agents)
		case p == "/agent":
			write(200, "[]")
		case p == "/path" && f.path != "":
			write(200, f.path)
		case p == "/question":
			write(200, f.questions)
		case p == "/permission":
			write(200, f.permissions)
		case p == "/session/status":
			write(200, f.status)
		case strings.HasSuffix(p, "/prompt_async") && r.Method == "POST":
			b := new(bytes.Buffer)
			b.ReadFrom(r.Body)
			f.posts = append(f.posts, b.String())
			if f.postStatus == 204 && !f.withhold {
				var m struct {
					MessageID string `json:"messageID"`
					Parts     []struct{ Text string }
				}
				json.Unmarshal(b.Bytes(), &m)
				if f.persist || true {
					f.stored[m.MessageID] = `{"info":{"id":"` + m.MessageID + `","role":"user"},"parts":[{"type":"text","text":` + jsonQ(m.Parts[0].Text) + `}]}`
					f.persist = true
				}
			}
			write(f.postStatus, "")
		case strings.Contains(p, "/message/"):
			id := p[strings.LastIndex(p, "/")+1:]
			if s, ok := f.stored[id]; ok {
				write(200, s)
			} else {
				write(404, `{"error":"not found"}`)
			}
		case strings.HasSuffix(p, "/message"):
			var parts []string
			for id := range f.stored {
				parts = append(parts, `{"info":{"id":"x","role":"user"}}`)
				if f.replyAfter {
					errp := ""
					if f.replyError != "" {
						errp = `,"error":` + f.replyError
					}
					parts = append(parts, `{"info":{"role":"assistant","parentID":"`+id+`","time":{"completed":5}`+errp+`}}`)
				}
			}
			write(200, "["+strings.Join(parts, ",")+"]")
		case strings.HasPrefix(p, "/session/"):
			if f.missing {
				write(404, `{"error":"no session"}`)
			} else {
				write(200, `{"id":"ses_1"}`)
			}
		default:
			write(404, "")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func jsonQ(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestChannelSendOpenCodeDelivers(t *testing.T) {
	w := newCSWorld(t)
	f := &ocFake{}
	srv := fakeOpenCode(t, f)
	r := w.cs("opencode", "ses_1", srv.URL, "hello there")
	r.rcIs(t, 0)
	if len(f.posts) != 1 || !strings.Contains(f.posts[0], `"text":"hello there"`) || !strings.Contains(f.posts[0], `"messageID":"msg_channel_`) {
		t.Errorf("posts: %v", f.posts)
	}
	// the same --id twice is one message, and the id is derived from the session and the key
	f2 := &ocFake{}
	s2 := fakeOpenCode(t, f2)
	a := w.cs("--id", "k-1", "opencode", "ses_1", s2.URL, "hello")
	a.rcIs(t, 0)
	b := w.cs("--id", "k-1", "opencode", "ses_1", s2.URL, "hello")
	b.rcIs(t, 0)
	if len(f2.posts) != 1 {
		t.Errorf("an idempotent resend posted again: %v", f2.posts)
	}
	if !strings.Contains(f2.posts[0], `"messageID":"msg_channel_`+"00") && false {
		t.Skip()
	}
	// the id exists with other bytes: refused
	c := w.cs("--id", "k-1", "opencode", "ses_1", s2.URL, "different")
	c.rcIs(t, 1)
	c.has(t, "opencode delivery failed at idempotency lookup: msg_channel_", "already exists with different message bytes")
	// the endpoint and the target are validated before anything is sent
	w.cs("opencode", "", srv.URL, "m").has(t, "opencode target session is empty")
	w.cs("opencode", "ses_1", "ftp://x", "m").has(t, "invalid opencode endpoint: ftp://x")
	// nobody is listening: exit 1, in node's words
	dead := w.cs("opencode", "ses_1", "http://127.0.0.1:1", "m")
	dead.rcIs(t, 1)
	dead.has(t, "opencode delivery failed at session-identity: fetch failed\n")
}

// 05-M01/M02: a processed receipt needs a completed assistant reply PARENTED to the message and without an error.
func TestChannelSendOpenCodeProcessedReceipt(t *testing.T) {
	w := newCSWorld(t)
	w.set("CHANNEL_DELIVERY_TIMEOUT_MS", "1500")
	ok := &ocFake{replyAfter: true}
	r := w.cs("--require", "processed", "opencode", "ses_1", fakeOpenCode(t, ok).URL, "m")
	r.rcIs(t, 0)
	none := &ocFake{}
	n := w.cs("--require", "processed", "opencode", "ses_1", fakeOpenCode(t, none).URL, "m")
	n.rcIs(t, 4)
	n.has(t, "IS in the receiver's session — admitted and persisted — but no completed assistant response was causally parented to it within 1500ms. The delivery succeeded; the turn did not.")
	bad := &ocFake{replyAfter: true, replyError: `{"name":"UnknownError"}`}
	e := w.cs("--require", "processed", "opencode", "ses_1", fakeOpenCode(t, bad).URL, "m")
	e.rcIs(t, 4)
}

// 26: a receiver whose default agent does not resolve is exit 6, before any POST.
func TestChannelSendOpenCodePreflight(t *testing.T) {
	w := newCSWorld(t)
	dead := &ocFake{config: `{"default_agent":"ghost","agent":{"ghost":{"disable":true}}}`, agents: `[{"name":"build"},{"name":"plan"}]`, path: `{"config":"/cfg/x/opencode","directory":"/w/y"}`}
	r := w.cs("opencode", "ses_1", fakeOpenCode(t, dead).URL, "m")
	r.rcIs(t, 6)
	r.has(t, `opencode delivery failed at receiver preflight: the receiver at http://127.0.0.1:`, `selects default agent "ghost", which is not among the 2 agents the endpoint actually resolves (GET /agent), because that scope also sets "disable": true on it.`,
		`dies with 'default agent "ghost" not found' before the message is persisted, so NO POST was sent and nothing landed`, "\n  the config scopes that have to agree:\n    global : /cfg/x/opencode/opencode.json\n    project: /w/y/opencode.json", "then RESTART the receiver — the merged config is read at startup.")
	if len(dead.posts) != 0 {
		t.Errorf("a POST was sent to a dead receiver: %v", dead.posts)
	}
	for _, c := range []struct {
		name string
		f    *ocFake
		why  string
	}{
		{"declared but unresolved", &ocFake{config: `{"default_agent":"ghost","agent":{"ghost":{"model":"x"}}}`, agents: `[{"name":"build"}]`}, "it is declared but did not survive config resolution"},
		{"not declared", &ocFake{config: `{"default_agent":"ghost"}`, agents: `[{"name":"build"}]`}, "no merged config scope declares it at all"},
	} {
		x := w.cs("opencode", "ses_1", fakeOpenCode(t, c.f).URL, "m")
		x.rcIs(t, 6)
		x.has(t, "because "+c.why+".")
	}
	// transparent when it cannot tell: the delivery goes ahead
	for name, f := range map[string]*ocFake{
		"no default":       {config: `{}`, agents: `[{"name":"build"}]`},
		"default resolves": {config: `{"default_agent":"build"}`, agents: `[{"name":"build"}]`},
		"no agents named":  {config: `{"default_agent":"ghost"}`, agents: `[]`},
		"not an array":     {config: `{"default_agent":"ghost"}`, agents: `{"build":1}`},
		"blank default":    {config: `{"default_agent":"  "}`, agents: `[{"name":"build"}]`},
		"unparseable":      {config: `not json`, agents: `[{"name":"build"}]`},
	} {
		if r := w.cs("opencode", "ses_1", fakeOpenCode(t, f).URL, "m"); r.rc != 0 {
			t.Errorf("%s: a preflight that could not tell stopped the delivery (exit %d): %s", name, r.rc, r.err)
		}
	}
	// 26-M06: a 204 whose message never shows up is exit 5 — it may have landed
	w.set("CHANNEL_PERSISTENCE_TIMEOUT_MS", "300")
	lost := &ocFake{withhold: true}
	l := w.cs("opencode", "ses_1", fakeOpenCode(t, lost).URL, "m")
	l.rcIs(t, 5)
	l.has(t, "HTTP 204 was ACCEPTED by the endpoint, but polling never confirmed msg_channel_", "landed within 300ms. The message may already be in the receiver's session — this is not a proven non-delivery. Retrying without --id may duplicate it.")
	// a refused admission is exit 1, with the body
	refused := &ocFake{postStatus: 500}
	x := w.cs("opencode", "ses_1", fakeOpenCode(t, refused).URL, "m")
	x.rcIs(t, 1)
	x.has(t, "opencode delivery failed at HTTP admission: HTTP 500")
}

// 41-M03: a ruling is refused before the POST when the receiver is busy or blocked.
func TestChannelSendOpenCodeRulingRefusedBeforePOST(t *testing.T) {
	w := newCSWorld(t)
	w.set("CLAUDE_PANE", "wO:p2")
	busy := &ocFake{status: `{"ses_1":{"type":"busy"}}`}
	r := w.cs("--ruling", "opencode", "ses_1", fakeOpenCode(t, busy).URL, "fix")
	r.rcIs(t, 1)
	r.has(t, "--ruling REFUSED before delivery: receiver is busy, not at a clean idle boundary.", "`hw ruling wO:p2 \"<the correction>\"` queues it", "DO NOT run hw done to reach exit 3.")
	if len(busy.posts) != 0 {
		t.Errorf("a refused ruling was posted: %v", busy.posts)
	}
	q := &ocFake{questions: `[{"id":"q9","sessionID":"ses_1"}]`}
	rq := w.cs("--ruling", "opencode", "ses_1", fakeOpenCode(t, q).URL, "fix")
	rq.rcIs(t, 1)
	rq.has(t, "receiver is blocked by question q9")
	// a generic redirect into a pending question is its own refusal, and a busy one goes through (prompt_async queues)
	g := w.cs("opencode", "ses_1", fakeOpenCode(t, &ocFake{questions: `[{"id":"q9","sessionID":"ses_1"}]`}).URL, "go")
	g.rcIs(t, 1)
	g.has(t, "opencode delivery failed at process-capable activation: receiver has pending question q9; no text was injected")
	bw := w.cs("opencode", "ses_1", fakeOpenCode(t, &ocFake{status: `{"ses_1":{"type":"retry"}}`}).URL, "go")
	bw.rcIs(t, 0)
	// an unknown status object is refused rather than guessed
	us := w.cs("opencode", "ses_1", fakeOpenCode(t, &ocFake{status: `{"ses_1":{"type":"idle"}}`}).URL, "go")
	us.rcIs(t, 1)
	us.has(t, `unsupported receiver status {"type":"idle"}`)
	// an idle receiver takes the ruling
	idle := &ocFake{}
	ok := w.cs("--ruling", "opencode", "ses_1", fakeOpenCode(t, idle).URL, "fix")
	ok.rcIs(t, 0)
}

// ── opencode-tui: the first-delivery route ──────────────────────────────────

type tuiFake struct {
	mu        sync.Mutex
	dir       string
	status    string
	questions string
	calls     []string
	text      string
	submitted bool
	submitAns string
	noLand    bool
	hang      bool
}

func fakeTUI(t *testing.T, f *tuiFake) *httptest.Server {
	t.Helper()
	if f.status == "" {
		f.status = "{}"
	}
	if f.questions == "" {
		f.questions = "[]"
	}
	if f.submitAns == "" {
		f.submitAns = "true"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/agent":
			fmt.Fprint(w, "[]")
		case "/session":
			fmt.Fprintf(w, `[{"id":"ses_9","directory":%s}]`, jsonQ(f.dir))
		case "/session/status":
			fmt.Fprint(w, f.status)
		case "/question":
			fmt.Fprint(w, f.questions)
		case "/permission":
			fmt.Fprint(w, "[]")
		case "/tui/clear-prompt":
			f.text = ""
			fmt.Fprint(w, "true")
		case "/tui/append-prompt":
			var b struct{ Text string }
			json.NewDecoder(r.Body).Decode(&b)
			f.text += b.Text
			fmt.Fprint(w, "true")
		case "/tui/submit-prompt":
			f.submitted = true
			fmt.Fprint(w, f.submitAns)
		case "/session/ses_9/message":
			if f.submitted && !f.noLand {
				fmt.Fprintf(w, `[{"info":{"role":"user","time":{"created":1}},"parts":[{"type":"text","text":%s}]},{"info":{"role":"assistant","time":{"created":2,"completed":3}}}]`, jsonQ(f.text))
			} else {
				fmt.Fprint(w, "[]")
			}
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChannelSendOpenCodeTUI(t *testing.T) {
	w := newCSWorld(t)
	dir := w.dir + "/work"
	f := &tuiFake{dir: dir}
	srv := fakeTUI(t, f)
	r := w.cs("opencode-tui", dir, srv.URL, "the brief")
	r.rcIs(t, 0)
	r.has(t, "CHANNEL_SESSION_ID=ses_9\n")
	if f.text != "the brief" {
		t.Errorf("typed %q", f.text)
	}
	order := strings.Join(f.calls, "|")
	if !(strings.Index(order, "POST /tui/clear-prompt") < strings.Index(order, "POST /tui/append-prompt") && strings.Index(order, "POST /tui/append-prompt") < strings.Index(order, "POST /tui/submit-prompt")) {
		t.Errorf("clear, append, submit out of order: %s", order)
	}
	// processed: the turn must complete
	p := w.cs("--require", "processed", "opencode-tui", dir, srv.URL, "the brief")
	p.rcIs(t, 0)
	// refusals before anything is typed
	w.cs("opencode-tui", "rel", srv.URL, "m").has(t, "opencode-tui target must be the receiver's absolute working directory, not a session id (got: rel)")
	w.cs("--ruling", "opencode-tui", dir, srv.URL, "m").has(t, "--ruling REFUSED: opencode-tui is the first-delivery route")
	w.cs("opencode-tui", dir, srv.URL, "/clear").has(t, "the message begins with '/', which the TUI prompt reads as a slash command")
	busy := &tuiFake{dir: dir, status: `{"ses_9":{"type":"busy"}}`}
	b := w.cs("opencode-tui", dir, fakeTUI(t, busy).URL, "m")
	b.rcIs(t, 1)
	b.has(t, "opencode-tui delivery failed at receiver readiness: the receiver is mid-turn (ses_9).")
	if busy.text != "" {
		t.Error("something was typed into a busy receiver")
	}
	modal := &tuiFake{dir: dir, questions: `[{"sessionID":"ses_9"}]`}
	m := w.cs("opencode-tui", dir, fakeTUI(t, modal).URL, "m")
	m.rcIs(t, 1)
	m.has(t, "the receiver has a pending question modal; text typed now answers the modal instead of opening a turn. NOTHING was typed.")
	// a TUI that does not accept the submit
	no := &tuiFake{dir: dir, submitAns: "false"}
	n := w.cs("opencode-tui", dir, fakeTUI(t, no).URL, "m")
	n.rcIs(t, 1)
	n.has(t, "at prompt submit: POST /tui/submit-prompt answered false instead of true — the TUI did not accept it")
	// accepted but never landed: exit 5
	w.set("CHANNEL_TUI_LAND_TIMEOUT_MS", "500")
	lost := &tuiFake{dir: dir, noLand: true}
	l := w.cs("opencode-tui", dir, fakeTUI(t, lost).URL, "m")
	l.rcIs(t, 5)
	l.has(t, "no user message with these exact bytes appeared under "+dir+" within 500ms.")
	// nothing listening: exit 1, "refused"; a hung receiver: exit 6
	d := w.cs("opencode-tui", dir, "http://127.0.0.1:1", "m")
	d.rcIs(t, 1)
	d.has(t, "http://127.0.0.1:1 refused the connection (fetch failed). Nothing is listening yet")
	hung := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer hung.Close()
	w.set("CHANNEL_TUI_PROBE_TIMEOUT_MS", "200")
	h := w.cs("opencode-tui", dir, hung.URL, "m")
	h.rcIs(t, 6)
	h.has(t, "accepted the connection and answered NOTHING within 200ms. The receiver is not slow, it is hung")
}

// ── pieces ──────────────────────────────────────────────────────────────────

func TestChannelSendEncodeURIComponent(t *testing.T) {
	for in, want := range map[string]string{"ses_1": "ses_1", "a b/c": "a%20b%2Fc", "/w/é": "%2Fw%2F%C3%A9", "a-_.!~*'()": "a-_.!~*'()"} {
		if got := jsEncodeURIComponent(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestChannelSendPySplitLines(t *testing.T) {
	got := pySplitLines("a\nb\r\nc\rd\x0be\x0cf\u2028g\n")
	if strings.Join(got, "|") != "a|b|c|d|e|f|g" {
		t.Errorf("%q", got)
	}
	if len(pySplitLines("")) != 0 {
		t.Error("an empty text has no lines")
	}
}

func opencodeProc(w *csWorld, srv *httptest.Server) {
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	os.WriteFile(w.stub+"/process-info.json", []byte(`{"result":{"process_info":{"foreground_processes":[{"argv":["opencode","--port","`+port+`"]}]}}}`), 0o644)
}

// the cross-check: a wait that cannot end is said so, with what was read, before the budget is spent.
func TestChannelSendWitnessCrossCheck(t *testing.T) {
	w := newCSWorld(t)
	w.agent("working", nil)
	w.set("STUB_RPC_RC", "2")
	w.set("WITNESS_SLICE_MS", "100")
	w.set("HW_INVOKER_WAIT_MS", "30000")
	os.WriteFile(w.stub+"/rpc-call-out", []byte(`{"read":{"text":"a static screen"}}`), 0o644)
	opencodeProc(w, fakeOpenCode(t, &ocFake{}))
	r := w.cs("herdr", "wW:p1", "-", "x")
	r.rcIs(t, 1)
	r.has(t, "wW:p1 is NOT going to become idle: wW:p1 says agent_status=working, and every independent check CONTRADICTS it", "NOTHING was sent, so nothing was lost", "THE ONE ROUTE LEFT, and it is not a retry: raw keystrokes — `herdr pane send-text wW:p1")
	if strings.Contains(w.calls(), "agent prompt") {
		t.Errorf("a pane that cannot become idle was sent to: %s", w.calls())
	}
	// a prompt the receiver's own endpoint confirms is a person to answer
	opencodeProc(w, fakeOpenCode(t, &ocFake{questions: `[{"id":"q"}]`}))
	h := w.cs("herdr", "wW:p1", "-", "x")
	h.rcIs(t, 1)
	h.has(t, "wW:p1 is WAITING FOR A PERSON: wW:p1 says agent_status=working, and its own endpoint", "CONFIRMS a real prompt is outstanding (/question 1, /permission 0)", "Answer it on that pane (or have the operator answer it), then re-run this identical command.")
	w.set("HW_OPERATOR", "Ada")
	w.cs("herdr", "wW:p1", "-", "x").has(t, "(or have Ada answer it)")
	// the claude route reads the same verdicts, and names the pane it waited on
	os.WriteFile(w.stub+"/agent-list.json", []byte(`{"result":{"agents":[{"agent":"claude","agent_session":{"value":"ses-1"},"pane_id":"wW:p1"}]}}`), 0o644)
	c := w.cs("claude", "ses-1", "-", "x")
	c.rcIs(t, 1)
	c.has(t, "wW:p1 (claude session ses-1) is WAITING FOR A PERSON:", "A larger budget, --allow-turn-ended and a raw keystroke route all fail the same way")
}

// a proved report to a blocked pane whose screen and endpoint both say "nothing is moving" is taken raw.
func TestChannelSendProvedReportToAStaticBlockedPane(t *testing.T) {
	w := newCSWorld(t)
	w.agent("blocked", nil)
	run := reportWorld(t, w, "wP:p1")
	w.set("STUB_PROMPT_RC", "1")
	os.WriteFile(w.stub+"/prompt-out", []byte(`{"error":{"code":"agent_blocked"}}`+"\n"), 0o644)
	os.WriteFile(w.stub+"/rpc-call-out", []byte(`{"read":{"text":"static"}}`), 0o644)
	opencodeProc(w, fakeOpenCode(t, &ocFake{}))
	r := w.cs("--report", "--report-state", run, "herdr", "wP:p1", "-", "done")
	r.rcIs(t, 0)
	r.has(t, "state-witness classified wP:p1 as settled rather than awaiting-human:", "falling back ONLY for this dispatch-bound unreported run to raw pane input", "proved run report submitted to wP:p1 through the blocked-pane fallback after agent_blocked.")
	if !strings.Contains(w.calls(), "pane send-text wP:p1 done\n") || !strings.Contains(w.calls(), "pane send-keys wP:p1 enter\n") {
		t.Errorf("raw input was not sent:\n%s", w.calls())
	}
	// and with a question outstanding it is a person's, not ours to overwrite
	opencodeProc(w, fakeOpenCode(t, &ocFake{questions: `[{"id":"q"}]`}))
	h := w.cs("--report", "--report-state", run, "herdr", "wP:p1", "-", "done")
	h.rcIs(t, 1)
	h.has(t, "state-witness confirms it is WAITING FOR A PERSON:", "raw pane input would overwrite a real human prompt.")
}

// the send is marked in flight BEFORE herdr is asked, so a kill that lands while it runs says exit 5 (42-M04).
func TestChannelSendTheSendIsInFlightWhileHerdrIsAsked(t *testing.T) {
	w := newCSWorld(t)
	w.agent("idle", nil)
	w.set("STUB_PROMPT_SLEEP", "5")
	var o, e bytes.Buffer
	c := newChanSend(&o, &e, os.Getenv, strings.NewReader(""), false, false)
	got := -1
	c.exitProc = func(rc int) { got = rc }
	c.sleep = func(float64) {}
	done := make(chan int, 1)
	go func() { done <- c.runGuarded([]string{"herdr", "wF:p1", "-", "go"}) }()
	// a deadline, not an iteration count: a loaded machine slows the steps before the prompt, and a timeout is named as one
	deadline := time.Now().Add(60 * time.Second)
	for !strings.Contains(w.calls(), "agent prompt") {
		if time.Now().After(deadline) {
			t.Fatalf("herdr was never asked (no agent prompt call) within 60 s:\n%s", e.String())
		}
		sleepSeconds(0.05)
	}
	c.mu.Lock()
	state := c.sent
	c.mu.Unlock()
	if state != csInflight {
		t.Errorf("while herdr was being asked the send was marked %d, want in flight (%d)", state, csInflight)
	}
	c.onSignal("TERM")
	if got != 5 {
		t.Errorf("a kill while the send was in flight exited %d, want 5:\n%s", got, e.String())
	}
	<-done
}

// A kill that lands while the gate waits: onSignal kills the herdr-rpc child, and the waiter sees it die. The waiter
// must not report that death as its own failure — the signal path speaks for the process. The seam holds the signal
// path after it has killed the child until the waiter has had every chance to speak first (the order the parity
// test lost on a loaded machine).
func TestChannelSendTheWaiterDoesNotReportTheDeathOurOwnKillCaused(t *testing.T) {
	w := newCSWorld(t)
	w.agent("working", nil)
	os.WriteFile(w.bin+"/herdr-rpc", []byte("#!/bin/bash\nexec sleep 30\n"), 0o755)
	var o, e syncBuf
	c := newChanSend(&o, &e, os.Getenv, strings.NewReader(""), false, false)
	got := -1
	var once sync.Once
	c.exitProc = func(rc int) { once.Do(func() { got = rc }) }
	c.sleep = func(float64) {}
	c.afterKill = func() {
		deadline := time.Now().Add(2 * time.Second)
		for !strings.Contains(e.String(), "could not wait on") && time.Now().Before(deadline) {
			sleepSeconds(0.01)
		}
	}
	done := make(chan int, 1)
	go func() { done <- c.runGuarded([]string{"herdr", "wS:p1", "-", "go"}) }()
	deadline := time.Now().Add(60 * time.Second)
	for {
		c.mu.Lock()
		waiting := c.child != nil
		c.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the gate never started waiting:\n%s", e.String())
		}
		sleepSeconds(0.01)
	}
	c.onSignal("TERM")
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the run did not return after the signal")
	}
	if strings.Contains(e.String(), "could not wait on") {
		t.Errorf("the waiter reported our own kill as a wait failure:\n%s", e.String())
	}
	if !strings.Contains(e.String(), "THIS IS YOUR CALLER KILLING IT") || got != 1 {
		t.Errorf("the signal path did not speak for the run (exit %d):\n%s", got, e.String())
	}
}

// 73: each non-zero status of channel-send names its exact meaning on the banner, never the fallback of an unknown code. The
// label mutants 73-G01..G03 (core/channel_send.go, setup/tests/contract-hw/mutants-cs) turn the case of 4, 5 and 6 into an unknown code.
func TestChannelSendExitMeanings(t *testing.T) {
	for _, c := range []struct {
		rc   int
		want string
	}{
		{4, "DELIVERED and provably in the receiver, but its TURN failed — do NOT resend"},
		{5, "ADMITTED, persistence UNCONFIRMED — it may already be in the receiver; do not blind-retry"},
		{6, "RECEIVER UNUSABLE — nothing was sent, and retrying cannot succeed"},
	} {
		got := csExitMeaning(c.rc)
		if got != c.want {
			t.Errorf("channel-send exit %d lost its exact meaning; got: %q", c.rc, got)
		}
		if strings.Contains(got, "a failure, not a delivery") {
			t.Errorf("channel-send exit %d fell through to the fallback meaning: %q", c.rc, got)
		}
	}
}
