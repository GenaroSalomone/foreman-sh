package main

// hw-core: the verbs of bin/hw that run as a Go binary. T0 is the pilot:
// receipt, log, handle and help. Every one is a port of the bash it names,
// byte for byte; the contract is setup/contracts/hw.md.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type app struct {
	*term
	c      *ctx
	args   []string
	getenv func(string) string
	stdin  io.Reader
}

func realGetenv(k string) string { return os.Getenv(k) }

// brainRoot is bin/hw's BRAIN: HW_BRAIN_ROOT (the shim passes the root it
// resolved), else found from where this binary lives.
func brainRoot(getenv func(string) string) string {
	if b := getenv("HW_BRAIN_ROOT"); b != "" {
		return b
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	d := filepath.Dir(exe)
	for _, up := range []string{"..", filepath.Join("..", "..", "..")} {
		cand := filepath.Clean(filepath.Join(d, up))
		if _, err := os.Stat(filepath.Join(cand, "projects.json")); err == nil {
			return cand
		}
	}
	return filepath.Clean(filepath.Join(d, ".."))
}

// run is main without the process: it returns the exit status.
func run(args []string, stdout, stderr *os.File) (rc int) {
	if len(args) > 0 && args[0] == "channel-send" { // the delivery transport (channel_send.go): raw bytes, no colour, no lane table
		return runChannelSend(args[1:], stdout, stderr, realGetenv, os.Stdin, isTerminal(stdout), isTerminal(os.Stdin))
	}
	color := getenvColor(realGetenv, stdout, stderr)
	if color {
		enableVT(stdout)
		enableVT(stderr)
	}
	return runWith(args, stdout, stderr, color, realGetenv, os.Stdin)
}

func getenvColor(getenv func(string) string, out, err *os.File) bool {
	// bin/hw runs `worktrees --quiet` as a held job of `hw status` (its stdout is a file until the replay), so
	// the terminal it is looking at is not the one this process sees: HW_CORE_COLOR=1 hands down that bash
	// started on a terminal, and NO_COLOR and TERM=dumb still turn it off.
	if getenv("HW_CORE_COLOR") == "1" {
		return getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
	}
	return getenv("NO_COLOR") == "" && getenv("TERM") != "dumb" && isTerminal(out) && isTerminal(err) // # MUTATION-ANCHOR: 816-G23
}

func runWith(args []string, stdout, stderr io.Writer, color bool, getenv func(string) string, stdin io.Reader) (rc int) {
	t := newTerm(stdout, stderr, color)
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(exitNow); ok {
				rc = e.code
				return
			}
			panic(r)
		}
	}()
	if len(args) == 0 {
		fmt.Fprintln(stderr, "hw-core: no verb (receipt, log, handle, help, status, status-full, executor-gate, worktrees, review-range, review-delta, preflight, outbox, reports, sweep, reap, wait, wait-one, wait-monitor, dispatch, done, next, revive, preview, and the T3 writers)")
		return 64
	}
	verb := args[0]
	switch verb {
	case "receipt", "log", "handle", "help", "status", "worktrees", "executor-gate":
	case "review-range", "review-delta", "preflight", "outbox", "reports", "sweep", "reap", "wait", "wait-one", "wait-monitor", "status-full": // T2; status-full is all of `hw status`
		for _, tool := range []string{"herdr", "git", "jq"} { // bin/hw's `need herdr; need git; need jq`, before any verb
			if _, err := exec.LookPath(tool); err != nil {
				t.die("missing required command: " + tool)
			}
		}
	default:
		if isT3Verb(verb) { // train T3, the marker writers (t3.go)
			break
		}
		if verb == "dispatch" { // train T5, the dispatch: its dry run (t5.go) and its launch (t5l*.go)
			for _, tool := range []string{"herdr", "git", "jq"} { // bin/hw:696 `need herdr; need git; need jq`, before any verb
				if _, err := exec.LookPath(tool); err != nil {
					t.die("missing required command: " + tool)
				}
			}
			break
		}
		if isT4Verb(verb) { // train T4, the large writers (t4.go)
			break
		}
		if isT6Verb(verb) { // train T6, the verbs that were bash-only (t6.go)
			for _, tool := range []string{"herdr", "git", "jq"} { // bin/hw:696 `need herdr; need git; need jq`, before any verb
				if _, err := exec.LookPath(tool); err != nil {
					t.die("missing required command: " + tool)
				}
			}
			break
		}
		fmt.Fprintf(stderr, "hw-core: %s is not a verb of hw-core (receipt, log, handle, help, status, status-full, executor-gate, worktrees, review-range, review-delta, preflight, outbox, reports, sweep, reap, wait, wait-one, wait-monitor, dispatch, done, next, revive, preview, and the T3 writers: ruling, human-boundary, executor-turn-end, chaining-lease-start, chaining-lease-expire, outbox-flush, output, stage)\n", verb)
		return 64
	}
	if verb == "status" || verb == "worktrees" { // bin/hw's `need herdr`, not an empty "herdr unreachable: "
		if _, err := exec.LookPath("herdr"); err != nil {
			t.die("missing required command: herdr")
		}
	}
	if verb == "-V" || verb == "--version" { // bin/hw:749, before the lane table: it answers from a brain whose table is broken
		return cmdVersion(t, brainRoot(getenv))
	}
	home := getenv("HOME")
	if home == "" && runtime.GOOS == "windows" {
		home = getenv("USERPROFILE")
	}
	brain := brainRoot(getenv)
	file := getenv("HW_PROJECTS_JSON")
	if file == "" {
		file = filepath.Join(brain, "projects.json")
	}
	tab, errs := loadTable(file, home)
	if tab == nil {
		for _, l := range errs {
			fmt.Fprintln(stderr, l)
		}
		t.die("the lane table did not load, so no lane is known — fix " + brain + "/projects.json (the reason is above)")
	}
	work := getenv("WORK")
	if work == "" {
		work = tab.work
	}
	a := &app{term: t, args: args[1:], getenv: getenv, stdin: stdin}
	a.c = &ctx{t: tab, brain: brain, work: work, home: home, getenv: getenv}
	// An executor does not have the brainer's verbs (bin/hw's gate, which comes after its lane table): refused here, in Go,
	// for every verb hw-core answers, and for the ones it does not by the shim's `executor-gate` call (executor_gate.go).
	if verb == "executor-gate" {
		a.executorGate(a.args)
		return 0
	}
	a.executorGate(gateArgv(verb, a.args))
	switch verb {
	case "receipt":
		a.cmdReceiptArgs()
	case "log":
		a.cmdLogArgs()
	case "handle":
		return a.cmdHandle()
	case "help":
		a.cmdHelp()
	case "status":
		return a.cmdStatusArgs()
	case "status-full":
		return a.cmdStatusFull()
	case "worktrees":
		return a.cmdWorktrees()
	case "review-range":
		return a.cmdReviewRange()
	case "review-delta":
		return a.cmdReviewDelta()
	case "preflight":
		return a.cmdPreflight()
	case "outbox":
		return a.cmdOutbox()
	case "reports":
		return a.cmdReports()
	case "reap": // T2 surveys; T4 applies (bin/hw sends a call with --apply here only for the writer)
		if hasApply(a.args) {
			return a.cmdReapApply()
		}
		return a.cmdReap()
	case "sweep":
		if hasApply(a.args) {
			return a.cmdSweepApply()
		}
		return a.cmdSweep()
	case "wait-one":
		return a.cmdWaitOne()
	case "wait-monitor":
		return a.cmdWaitMonitor()
	case "wait":
		return a.cmdWait()
	case "dispatch":
		return a.cmdDispatch()
	default: // a verb of T3, T4 or T6 (checked above)
		if isT3Verb(verb) {
			return a.cmdT3(verb)
		}
		if isT6Verb(verb) {
			return a.cmdT6(verb)
		}
		return a.cmdT4(verb)
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
