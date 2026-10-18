package main

// `channel-send`: deliver one message with the requested receiver-owned receipt (bin/channel-send, 2107 lines of
// bash), ported byte for byte. Its exit-code table (0 delivered, 1 not delivered, 4 delivered but the turn failed,
// 5 unknown, 6 receiver unusable), its stdout and stderr and its on-disk effects (the delivery lock, the reply
// hold, the transcript) are the contract: callers branch on them, and setup/fixtures/verb-golden/
// channel-send-exit-codes holds them.
//
// This file: the command line, the exit guard (a non-zero exit that cannot be swallowed by a pipe), the signal
// outcomes (being killed is one of three reported states) and the route switch. The routes are in
// channel_send_herdr.go, channel_send_http.go (opencode, opencode-tui) and channel_send_claude.go (claude, codex,
// the --report-state proof). The programs the bash ran stay programs here: herdr, herdr-rpc, holdfacts,
// claude-delivery-receipt.py, claude, codex, ps, find — what it did with jq, python3's two embedded scripts and
// node's two is native.

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

//go:embed channel_send_help.txt
var channelSendHelp string

// csExit unwinds to runChannelSend with the status the process ends with.
type csExit struct{ code int }

// lockedWriter keeps a signal's message and the main flow's from interleaving mid-line.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// delivery state for the signal handler: _CS_SENT.
const (
	csNotSent  = 0
	csSent     = 1
	csInflight = 2
)

type chanSend struct {
	out, err         io.Writer
	getenv           func(string) string
	stdin            io.Reader
	stdoutTTY, inTTY bool
	binDir           string
	exitProc         func(int)
	sleep            func(seconds float64)
	a                *app // the witness's functions (t4_next_gate.go), over this process's environment

	mu       sync.Mutex // guards the fields the signal handler reads
	spoken   bool
	t0       int64
	sent     int
	target   string // _CS_TARGET: how the receiver is named in the signal's message
	budgetMS string // _CS_BUDGET_MS, "" until a gated wait picks one
	child    *os.Process
	lock     string // the delivery lock directory, once held
	lockHeld bool

	// the signal path owns the exit once it has started: a waiter that sees its child die must not report that death
	// as its own failure (the child died because onSignal killed it).
	signalled bool
	sigRC     int
	sigDone   chan struct{} // closed when onSignal has finished speaking; nil until a signal arrives
	afterKill func()        // a test seam: runs in onSignal right after the child is signalled, before it speaks

	required, deliveryKey, replyHold, reportState string
	allowTurnEnded, isReport, isRuling            bool
	holdProven, reportProven                      bool
	vendor, tgt, endpoint, message                string
	witnessDetail                                 string
	degraded                                      bool // no state-witness.sh beside this binary
	probeMS                                       int64
}

// runChannelSend is `channel-send <args>`: the exit status.
func runChannelSend(args []string, stdout, stderr io.Writer, getenv func(string) string, stdin io.Reader, stdoutTTY, stdinTTY bool) int {
	return newChanSend(stdout, stderr, getenv, stdin, stdoutTTY, stdinTTY).runGuarded(args)
}

func newChanSend(stdout, stderr io.Writer, getenv func(string) string, stdin io.Reader, stdoutTTY, stdinTTY bool) *chanSend {
	var mu sync.Mutex
	c := &chanSend{
		out: lockedWriter{&mu, stdout}, err: lockedWriter{&mu, stderr}, getenv: getenv, stdin: stdin,
		stdoutTTY: stdoutTTY, inTTY: stdinTTY, exitProc: os.Exit,
		sleep: func(s float64) { sleepSeconds(s) },
	}
	c.binDir = c.resolveBinDir()
	c.a = &app{term: newTerm(c.out, c.err, false), getenv: getenv, stdin: stdin, c: &ctx{brain: filepath.Dir(c.binDir), getenv: getenv}}
	return c
}

func (c *chanSend) resolveBinDir() string {
	if d := c.getenv("HW_CORE_BIN_DIR"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe)
}

func (c *chanSend) env(k, def string) string {
	if v := c.getenv(k); v != "" {
		return v
	}
	return def
}

// runGuarded is the script with its two EXIT traps: every way out releases the lock and speaks the exit guard.
func (c *chanSend) runGuarded(args []string) (rc int) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(csExit)
			if !ok {
				panic(r)
			}
			rc = e.code
		}
		c.releaseLock() // # MUTATION-ANCHOR: 66-G02
		c.exitGuard(rc) // # MUTATION-ANCHOR: 66-G01
	}()
	c.body(args)
	return 0
}

func (c *chanSend) exit(rc int) { panic(csExit{rc}) }

func (c *chanSend) printf(format string, a ...any) { fmt.Fprintf(c.err, format, a...) }

// die is `die`: "channel-send: <msg>" on stderr, exit 1.
func (c *chanSend) die(msg string) {
	fmt.Fprintf(c.err, "channel-send: %s\n", msg)
	c.exit(1)
}

// ── THE EXIT CODE A PIPE SWALLOWS ───────────────────────────────────────────

func csExitMeaning(rc int) string {
	switch rc {
	case 1:
		return "NOT DELIVERED — nothing landed, nothing was lost, retry is free"
	case 4:
		return "DELIVERED and provably in the receiver, but its TURN failed — do NOT resend"
	case 5:
		return "ADMITTED, persistence UNCONFIRMED — it may already be in the receiver; do not blind-retry"
	case 6:
		return "RECEIVER UNUSABLE — nothing was sent, and retrying cannot succeed"
	}
	return "a failure, not a delivery"
}

// exitGuard is _cs_exit_guard: once per process, only for a non-zero status, only when stdout is not a terminal.
func (c *chanSend) exitGuard(rc int) {
	c.mu.Lock()
	if c.spoken {
		c.mu.Unlock()
		return
	}
	c.spoken = true
	c.mu.Unlock()
	if rc == 0 {
		return
	}
	if c.stdoutTTY { // # MUTATION-ANCHOR: 66-G04
		return
	}
	n := strconv.Itoa(rc)
	fmt.Fprintf(c.err, "channel-send: ══ MY EXIT CODE IS %s: %s ══\n", n, csExitMeaning(rc))
	fmt.Fprintf(c.err, "channel-send: my stdout is NOT a terminal, so this number can be swallowed. If you piped me — `channel-send … | tail` — then `$?` is TAIL's status (0 whenever it reads a byte), NOT mine: run me unpiped — `channel-send … ; echo rc=$?` reads the real status in bash and zsh (`${PIPESTATUS[0]}` is bash-only: zsh spells it `${pipestatus[1]}` and prints nothing for the bash spelling). If you captured me with `$(…)` or `|| rc=$?` you already have it, and it is %s. Either way the delivery fact is %s, not whatever the shell shows.\n", n, n) // # MUTATION-ANCHOR: 66-G05
}

// ── BEING KILLED IS AN OUTCOME, AND IT HAS TO BE REPORTED LIKE ONE ───────────

// onSignal is _cs_on_signal: the three states, each its own words and its own status.
func (c *chanSend) onSignal(sig string) {
	c.mu.Lock()
	c.signalled = true
	if c.sigDone == nil {
		c.sigDone = make(chan struct{})
	}
	done := c.sigDone
	elapsed := nextEpoch() - c.t0
	child := c.child
	c.child = nil
	sent, target, budget := c.sent, c.target, c.budgetMS
	c.mu.Unlock()
	defer close(done)
	if child != nil {
		child.Signal(syscall.SIGTERM)
	}
	if c.afterKill != nil {
		c.afterKill()
	}
	c.releaseLock()
	rc := 1
	switch sent {
	case csSent:
		c.printf("channel-send: SIG%s after %ds, but %s ALREADY HAS THIS MESSAGE — it was sent before the signal arrived. Do NOT resend and do not fall back to another route; that is how one ruling gets delivered twice. Exiting 0 because the delivery happened.\n", sig, elapsed, target)
		rc = 0
	case csInflight:
		c.printf("channel-send: SIG%s after %ds WHILE THE SEND WAS IN FLIGHT to %s. This CANNOT say whether it landed: the delivery had been issued and its result had not come back. Do not blind-retry — read that pane first, and resend only if the message is not there.\n", sig, elapsed, target)
		rc = 5
	default:
		c.printf("channel-send: SIG%s after %ds — THIS IS YOUR CALLER KILLING IT (a Bash/harness timeout), not a channel-send failure. NOTHING WAS SENT to %s and nothing was lost: retry the identical command.\n", sig, elapsed, target)
		if budget != "" {
			ms, _ := strconv.ParseInt(budget, 10, 64)
			c.printf("channel-send: this call was allowed to wait up to %ds for %s to reach idle, so a caller timeout below %d ms will always kill it here. Either give the call that timeout, or shorten the wait: HW_INVOKER_WAIT_MS=<ms> channel-send …\n", ms/1000, target, ms+15000)
		}
	}
	c.mu.Lock()
	c.sigRC = rc
	c.mu.Unlock()
	c.exitGuard(rc) // # MUTATION-ANCHOR: 66-G03
	c.exitProc(rc)
}

// installSignals routes SIGTERM and SIGINT to onSignal; the returned func removes them.
func (c *chanSend) installSignals() func() {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan struct{})
	go func() {
		select {
		case s := <-ch:
			if s == syscall.SIGINT {
				c.onSignal("INT")
			} else {
				c.onSignal("TERM")
			}
		case <-done:
		}
	}()
	return func() { signal.Stop(ch); close(done) }
}

func (c *chanSend) setSent(v int) {
	c.mu.Lock()
	c.sent = v
	c.mu.Unlock()
}

func (c *chanSend) setTarget(t string) {
	c.mu.Lock()
	c.target = t
	c.mu.Unlock()
}

func (c *chanSend) setBudget(ms int64) {
	c.mu.Lock()
	c.budgetMS = strconv.FormatInt(ms, 10)
	c.mu.Unlock()
}

// ── the command line ────────────────────────────────────────────────────────

func (c *chanSend) body(args []string) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		io.WriteString(c.out, channelSendHelp)
		c.exit(0)
	}
	c.required = "admitted"
parse:
	for {
		first := ""
		if len(args) > 0 {
			first = args[0]
		}
		switch first {
		case "--require":
			if len(args) < 2 {
				c.die("--require needs admitted or processed")
			}
			c.required = args[1]
			args = args[2:]
		case "--allow-turn-ended":
			c.allowTurnEnded = true
			args = args[1:]
		case "--report":
			c.isReport = true
			args = args[1:]
		case "--report-state":
			if len(args) < 2 {
				c.die("--report-state needs the absolute task state directory written by hw")
			}
			c.reportState = args[1]
			args = args[2:]
		case "--ruling":
			c.isRuling = true // # MUTATION-ANCHOR: 41-G02
			args = args[1:]
		case "--reply-hold":
			if len(args) < 2 {
				c.die("--reply-hold needs the absolute pending-reply file written by ask-invoker")
			}
			c.replyHold = args[1]
			args = args[2:]
		case "--id":
			if len(args) < 2 {
				c.die("--id needs a stable logical delivery id")
			}
			c.deliveryKey = args[1]
			if c.deliveryKey == "" {
				c.die("--id must not be empty")
			}
			args = args[2:]
		default:
			break parse
		}
	}
	if c.required != "admitted" && c.required != "processed" {
		c.die("--require must be admitted or processed (got: " + c.required + ")")
	}
	if c.isReport && c.isRuling {
		c.die("--report and --ruling are different delivery contracts and cannot be combined")
	}
	if c.allowTurnEnded && c.isRuling {
		c.die("--allow-turn-ended cannot make a ruling ordered; use --ruling alone")
	}
	if len(args) < 4 {
		c.die("usage: channel-send [--require admitted|processed] [--id logical-id] [--allow-turn-ended] [--report|--ruling] [--report-state absolute-directory] [--reply-hold absolute-file] <claude|opencode|opencode-tui|codex|herdr> <session-or-pane-or-directory> <endpoint|-> <message|-> (a lone '-' for the message reads it from stdin — the way for text with backticks, $, quotes or newlines that argv would mangle)")
	}
	c.vendor, c.tgt, c.endpoint = args[0], args[1], args[2]
	args = args[3:]
	// MESSAGE VIA STDIN: a lone '-' in the message slot.
	if args[0] == "-" { // # MUTATION-ANCHOR: 135-G01
		if len(args) != 1 {
			c.die("message given twice: as '-' for stdin AND as more argv text after it — pass it ONE way, not both")
		}
		if c.inTTY {
			c.die("message is '-' (read from stdin), but stdin is a terminal — pipe or redirect the text in, e.g. channel-send ... - - <<'EOF'")
		}
		b, err := io.ReadAll(c.stdin)
		if err != nil {
			c.die("could not read the message from stdin")
		}
		c.message = bashSub(string(b))
		if c.message == "" {
			c.die("stdin for '-' produced no bytes — an empty message delivered would be worse than this refusal")
		}
	} else {
		c.message = strings.Join(args, " ")
		if c.message == "" {
			c.die("message is empty")
		}
	}
	c.degraded = !csReadable(filepath.Join(c.binDir, "state-witness.sh"))
	c.probeMS = 30000
	c.checkHold()
	c.checkReportState()
	c.setTarget(c.tgt)
	c.t0 = nextEpoch() // _CS_T0
	stop := c.installSignals()
	defer stop()
	c.sanitizeProbe()
	c.route()
	c.clearReplyHold()
}

// checkHold is the --reply-hold block: bin/holdfacts is the one validator of a hold file.
func (c *chanSend) checkHold() {
	if c.replyHold == "" {
		return
	}
	if !strings.HasPrefix(c.replyHold, "/") {
		c.die("--reply-hold must be an absolute path")
	}
	if !c.isReport && !c.isRuling {
		c.die("--reply-hold is valid only for a declared --report answer or --ruling")
	}
	intent := "answer"
	if c.isRuling {
		intent = "ruling"
	}
	hargs := []string{"match", c.replyHold, "--intent", intent, "--route", c.vendor, "--target", c.tgt}
	if c.deliveryKey != "" {
		hargs = append(hargs, "--logical-id", c.deliveryKey)
	}
	r := extRun(extCall{Name: c.binDir + "/holdfacts", Args: hargs, Env: childEnv(), Merge: true})
	herr := bashSub(r.Out)
	if r.RC > 1 {
		herr = fmt.Sprintf("holdfacts did not run (exit %d): %s", r.RC, herr)
	}
	if herr != "" {
		c.die("--reply-hold REFUSED before delivery: " + herr + ". NOTHING was sent.")
	}
	c.holdProven = true
}

// clearReplyHold is _clear_reply_hold: the answer or ruling is delivered, so its text goes to the transcript and
// the hold goes away.
func (c *chanSend) clearReplyHold() {
	if !c.holdProven {
		return
	}
	if !c.degraded { // `type transcript_append` is true only when state-witness.sh was sourced
		envelope := c.deliveryKey
		if envelope == "" {
			if b, err := os.ReadFile(c.replyHold); err == nil {
				for _, l := range strings.Split(string(b), "\n") { // awk -F= '$1 == "logical_id" { print $2; exit }'
					if f := strings.Split(l, "="); f[0] == "logical_id" {
						if len(f) > 1 {
							envelope = f[1]
						}
						break
					}
				}
			}
		}
		if envelope == "" {
			envelope = "-"
		}
		kind := "ANSWER"
		if c.isRuling {
			kind = "RULING"
		}
		csTranscriptAppend(filepath.Dir(c.replyHold), kind, envelope, c.message)
	}
	os.Remove(c.replyHold)
}

// csTranscriptAppend is transcript_append (bin/state-witness.sh): best effort, always.
func csTranscriptAppend(d, kind, envelope, text string) {
	if d == "" {
		return
	}
	base := filepath.Base(d)
	if len(base) > 1 && base[0] == 't' && strings.Trim(base[1:], "0123456789") == "" {
		d = filepath.Dir(d)
	}
	if !isDir(d) {
		return
	}
	body := "    | " + strings.ReplaceAll(text+"\n", "\n", "\n    | ")
	body = strings.TrimSuffix(body, "    | ")
	body = strings.TrimSuffix(body, "\n")
	ts := nextDateUTC()
	f, err := os.OpenFile(d+"/transcript.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s  envelope=%s\n%s\n", ts, kind, envelope, body)
}

// operator is _cs_operator: the person a message tells the caller to leave a prompt to.
func (c *chanSend) operator() string {
	name := c.getenv("HW_OPERATOR")
	if name == "" {
		file := c.getenv("HW_PROJECTS_JSON")
		if file == "" {
			root := c.getenv("HW_BRAIN_ROOT")
			if root == "" {
				root = filepath.Dir(c.binDir)
			}
			file = root + "/projects.json"
		}
		if b, err := os.ReadFile(file); err == nil {
			name = jqRawOr(string(b), "", "operator")
		}
	}
	if name == "" {
		return "the operator"
	}
	return name
}

// sanitizeProbe is HW_AWAITING_CHILD_PROBE_MS, validated: a junk value falls back to the default and says so.
func (c *chanSend) sanitizeProbe() {
	v := c.getenv("HW_AWAITING_CHILD_PROBE_MS")
	c.probeMS = 30000
	if v == "" {
		return
	}
	if strings.Trim(v, "0123456789") != "" {
		c.printf("channel-send: HW_AWAITING_CHILD_PROBE_MS=%s is not a number of milliseconds; using 30000.\n", v)
		return
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n == 0 {
		return
	}
	c.probeMS = n
}

// route is the `case "$VENDOR"` switch.
func (c *chanSend) route() {
	switch c.vendor {
	case "opencode-tui":
		c.routeOpencodeTUI()
	case "opencode":
		c.routeOpencode()
	case "codex":
		c.routeCodex()
	case "claude":
		c.routeClaude()
	case "herdr":
		c.routeHerdr()
	default:
		c.die("unknown vendor: " + c.vendor)
	}
}

// ── helpers shared by the routes ────────────────────────────────────────────

func (c *chanSend) herdr() herdrClient { return herdrClient{getenv: c.getenv} }

// csRun runs a program the way the bash ran it in the background and `wait`ed: its process is known to the
// signal handler while it runs. stdout is kept when keep, otherwise dropped; stderr is dropped.
func (c *chanSend) csRPCWait(pane string, ms int64) int {
	cmd := exec.Command(c.binDir+"/herdr-rpc", "wait-agent", pane, "idle,done", "--timeout-ms", strconv.FormatInt(ms, 10))
	cmd.Env = childEnv()
	if err := cmd.Start(); err != nil {
		return 127
	}
	c.mu.Lock()
	c.child = cmd.Process
	c.mu.Unlock()
	err := cmd.Wait()
	c.mu.Lock()
	c.child = nil
	signalled, done := c.signalled, c.sigDone
	c.mu.Unlock()
	if signalled {
		// onSignal killed the child and is speaking for this process: its death is not a wait failure to report.
		// In the binary the process is gone before this returns; in process the run ends with the signal's status.
		<-done
		c.mu.Lock()
		rc := c.sigRC
		c.mu.Unlock()
		c.exit(rc)
	}
	return exitStatus(err)
}

func csReadable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}
