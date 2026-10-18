package main

// hw suite — the port of cmd_suite (bin/hw): one command per lane under that lane's suite lock, so two executors
// in a shared tree cannot run the suite over each other; a lane whose suite pools itself (`suite_lock: none`) takes
// no lock and runs at once. Everything bash does it does here, in the same order and with the same words:
//
//   - the lock (_suite_lock_take, _suite_holder_alive, _suite_holder_line, _suite_lock_held_by_ancestor) and the
//     machine-wide gate (sg_take / sg_release, bin/suite-gate) are `hw done`'s own, already ported in
//     t4_done_verify.go (doneRun.suiteLockTake, doneHolderAlive, doneLockHeldByAncestor, doneGate): `hw done`'s
//     close verification and this verb were always the same acquire loop, and they stay one here too;
//   - the wrapped command is run as bash runs `"$@"`: in the foreground, stdin/stdout/stderr inherited, its
//     status passed through unchanged (128+n for a signalled child);
//   - INT and TERM are trapped only while the lock is held, as bash does (`trap ... INT`, `trap ... TERM`): bash
//     defers a trap until the foreground command ends, so a signal sent to hw alone lets the command finish, then
//     releases the lock and exits 130/143; a signal that lands while hw itself waits (the gate's poll) ends the
//     wait at once.
//
// Intentional differences: bash prints a command it cannot start as `<script>: line N: x: command not found`;
// here it is `hw: x: command not found` (the status, 127 or 126, is the same).

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func suiteUsage() string {
	return "usage: hw suite <project> [--wait-ms N] [--dry-run|--status] -- <command...>   Serialises one command per lane against that lane's suite lock, so two executors in a shared tree cannot run the suite over each other. --dry-run answers \"would this wait, and behind whom\" and spends nothing. NEVER kill a suite by command pattern: `pkill -f setup/test-hw` matches every lane's and every agent's."
}

// suiteLanePooled is _suite_lane_pooled: the lane's suite coordinates itself and takes no lock.
func suiteLanePooled(a *app, proj string) bool {
	return a.c.t.get(proj, "suite_lock") == "none" // bash anchor 139-M03 (registered on the bash source)
}

func (a *app) cmdSuite() int {
	args := a.args
	proj := ""
	if len(args) > 0 {
		proj, args = args[0], args[1:]
	}
	if proj == "" {
		a.die(suiteUsage())
	}
	proj = a.c.t.canonProject(proj)
	waitMS, dry, status := "", false, false
parse:
	for len(args) > 0 {
		switch args[0] {
		case "--wait-ms":
			if len(args) < 2 { // _need_val "$@"
				a.die(args[0] + " needs a value")
			}
			waitMS = args[1]
			args = args[2:]
		case "--dry-run":
			dry = true
			args = args[1:]
		case "--status":
			status = true
			args = args[1:]
		case "--":
			args = args[1:]
			break parse
		default:
			a.die("hw suite: unknown flag " + args[0] + " (the command to run goes after `--`)")
		}
	}
	if !a.c.t.known(proj) {
		a.die("unknown project: " + proj + " (expected: " + a.c.t.namesBar() + ")")
	}
	t5FillLaneFields(a.c.t, a.c.home) // lane_get suite_lock: the loader does not keep it

	lock := a.c.work + "/.locks/suite-" + proj
	os.MkdirAll(doneDirname(lock), 0o777)

	d := &doneRun{app: a, binDir: a.getenv("HW_CORE_BIN_DIR"), root: a.getenv("HW_CORE_ROOT")}
	if d.binDir == "" {
		d.binDir = a.c.brain + "/bin"
	}
	if d.root == "" {
		d.root = a.c.brain
	}
	s := &suiteRun{d: d, gate: &doneGate{d: d}, extra: map[string]string{}}

	if suiteLanePooled(a, proj) {
		pool := a.c.work + "/.locks/suite-pool"
		busy := 0
		slots, _ := os_glob(pool, "slot.")
		for _, p := range slots {
			if _, err := os.Stat(p); err == nil { // [ -e "$d" ]
				busy++
			}
		}
		if status || dry {
			excl := ""
			if isDir(pool + "/exclusive") {
				pid, _ := doneReadTrim(pool + "/exclusive/owner.pid")
				job, _ := doneReadTrim(pool + "/exclusive/owner.job")
				excl = fmt.Sprintf(", and a timing subject has the machine (pid %s, %s)", pid, job)
			}
			a.ok("the " + proj + " lane has no suite lock — this would run at once. Its suite takes worker slots from " + pool + ", shared by every suite on this machine: " + strconv.Itoa(busy) + " slot(s) held right now" + excl + ".")
			return 0
		}
		if len(args) == 0 {
			a.die("hw suite: nothing to run. Put the command after `--`, e.g. hw suite " + proj + " -- bash setup/test-hw")
		}
		a.info("the " + proj + " lane has no suite lock: setup/test-hw shares the machine's worker pool (" + pool + ") with every other suite and runs its timing subjects alone. Running now.")
		rc, _ := s.gatedRun("hw suite "+proj+": "+strings.Join(args, " "), args)
		s.gate.release()
		return rc
	}

	// ── answering without acting ──
	if status || dry {
		if !isDir(lock) {
			a.ok("suite lock for " + proj + " is FREE — this would acquire it immediately and wait for nothing")
			return 0
		}
		if doneHolderAlive(lock) {
			a.warn("suite lock for " + proj + " is HELD by a live runner: " + doneHolderLine(lock))
			a.info("this would WAIT for it. Do other work and come back — and never `pkill -f` the suite: that pattern matches every lane's run and every agent's, and it has already destroyed three complete runs.")
		} else {
			a.warn("suite lock for " + proj + " is an ORPHAN — " + doneHolderLine(lock) + " is no longer running, so this would reclaim it at once rather than wait behind a dead holder")
		}
		return 0
	}

	if len(args) == 0 {
		a.die("hw suite: nothing to run. Put the command after `--`, e.g. hw suite setup -- bash setup/test-hw")
	}

	// A default long enough for the thing it waits on: a suite run is ~20-23 minutes, so 45 minutes.
	if waitMS == "" {
		waitMS = a.getenv("HW_SUITE_WAIT_MS")
		if waitMS == "" {
			waitMS = "2700000"
		}
	}
	if !doneAllDigits(waitMS) {
		a.die("--wait-ms/HW_SUITE_WAIT_MS must be a positive integer in milliseconds (got: " + waitMS + ")")
	}
	wait := doneBashArith(waitMS)
	label := "hw suite " + proj + ": " + strings.Join(args, " ")

	// Already inside somebody's hold: run, do not queue behind an ancestor blocked on this very command.
	if doneLockHeldByAncestor(lock, proj, a.getenv("HW_SUITE_LOCK_HELD")) {
		a.info("the " + proj + " suite lock is already held further up this process tree — running inside that hold rather than waiting behind it")
		rc, _ := s.gatedRun(label, args)
		s.gate.release()
		return rc
	}

	token, ok := d.suiteLockTake(lock, proj, wait, strings.Join(args, " "))
	if !ok {
		a.die("the " + proj + " suite lock did not free within " + strconv.FormatInt(wait/1000, 10) + "s. It is held by a LIVE process — " + doneHolderLine(lock) + " — so this is not an orphan and reclaiming it would destroy a running suite. NOTHING was run. Look at that process and decide: wait longer with --wait-ms, or kill it BY PID if you know what it is. Never by command pattern.")
	}
	s.lock = lock

	// Released on every exit path the shell can see; INT and TERM exit (130, 143) after the lock is gone.
	sigs := make(chan os.Signal, 2)
	// A signal ignored on entry (a background job of a non-interactive shell ignores INT) cannot be trapped, in
	// bash or here: Notify would re-enable it, so it is left alone.
	for _, sg := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		if !signal.Ignored(sg) {
			signal.Notify(sigs, sg)
		}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case sg := <-sigs:
			s.mu.Lock()
			if s.running { // a trap waits for the foreground command to end
				s.pending = sg
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()
			os.RemoveAll(lock)
			s.gate.release()
			os.Exit(suiteSigStatus(sg))
		case <-stop:
		}
	}()
	released := false
	defer func() {
		if !released { // the EXIT trap: a die or a signal still releases the lock
			os.RemoveAll(lock)
			s.gate.release()
		}
		signal.Stop(sigs)
		close(stop)
		<-done
	}()
	a.ok("holding the " + proj + " suite lock (pid " + strconv.Itoa(os.Getpid()) + ")")
	s.extra["HW_SUITE_LOCK_HELD"] = proj + ":" + token
	rc, sg := s.gatedRun(label, args)
	if sg != nil {
		return suiteSigStatus(sg)
	}
	s.gate.release()
	os.RemoveAll(lock)
	released = true
	a.ok("released the " + proj + " suite lock")
	return rc
}

func suiteSigStatus(sg os.Signal) int {
	if sg == syscall.SIGINT {
		return 130
	}
	return 143
}

// suiteRun is what one `hw suite` run holds: the gate, the environment it hands the command, and whether a trapped
// signal is waiting for the command to end.
type suiteRun struct {
	d     *doneRun
	gate  *doneGate
	extra map[string]string
	lock  string

	mu      sync.Mutex
	running bool
	pending os.Signal
}

// gatedRun is _suite_gated_run: one slot of the machine-wide gate around the command (bin/hw sources
// bin/suite-gate when it is readable; absent, nothing gates), and the command's own status. A signal trapped while
// the command ran comes back as the second value, with the gate still held for the caller's exit path to release.
func (s *suiteRun) gatedRun(label string, argv []string) (int, os.Signal) {
	if readableFile(s.d.binDir + "/suite-gate") {
		s.gate.take(label, s.extra)
	}
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	rc := s.exec(argv)
	s.mu.Lock()
	s.running = false
	sg := s.pending
	s.mu.Unlock()
	if sg != nil {
		return rc, sg
	}
	s.gate.release()
	return rc, nil
}

// exec is `"$@"`: the command in the foreground, the terminal's streams, its status as bash reads it.
func (s *suiteRun) exec(argv []string) int {
	a := s.d.app
	c := exec.Command(argv[0], argv[1:]...)
	c.Env = doneChildEnv(s.extra)
	c.Stdin, c.Stdout, c.Stderr = a.stdin, a.out, a.err
	if err := c.Start(); err != nil {
		return suiteStartFail(a, argv[0], err)
	}
	return doneExit(c.Wait())
}

// suiteStartFail is what bash says and returns for a command it cannot start: 127 not found, 126 not runnable.
func suiteStartFail(a *app, name string, err error) int {
	why, rc := "command not found", 127
	var pe *fs.PathError
	switch {
	case errors.Is(err, fs.ErrPermission):
		why, rc = "Permission denied", 126
	case errors.As(err, &pe) && strings.Contains(name, "/"):
		why = "No such file or directory"
	}
	a.errf("hw: %s: %s\n", name, why)
	return rc
}
