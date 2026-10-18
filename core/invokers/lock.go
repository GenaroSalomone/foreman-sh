package invokers

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// procStart is _invoker_proc_start: `ps -o lstart= -p PID` with runs of spaces
// squeezed and the ends trimmed ("" when ps says nothing).
func (c *Ctx) procStart(pid string) string {
	out, _ := c.out("ps", "-o", "lstart=", "-p", pid)
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		for strings.Contains(l, "  ") {
			l = strings.ReplaceAll(l, "  ", " ")
		}
		lines[i] = strings.Trim(l, " ")
	}
	return strings.Join(lines, "\n")
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

func (c *Ctx) runLockPath() string {
	key := []byte(c.or("HW_RUN", "norun"))
	for i, b := range key {
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '.' || b == '-') {
			key[i] = '_'
		}
	}
	return c.tmpdir() + "/hw-invoker-" + string(key) + ".lock"
}

// olderThanMinute is `find <dir> -maxdepth 0 -mmin +1`: ok is false when find
// could not ask (it failed), old when it printed the path.
func (c *Ctx) olderThanMinute(path string) (old, ok bool) {
	st, err := os.Lstat(path)
	if err != nil {
		return false, false
	}
	return c.Now().Unix()-st.ModTime().Unix() > 60, true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// releaseRunLock is invoker_release_run_lock: BY IDENTITY, never by path.
func (c *Ctx) releaseRunLock() {
	lock := c.runLock
	if lock == "" {
		return
	}
	if readTrim(lock+"/owner.pid") != itoa(c.Pid) { // 101-M01
		return
	}
	os.Remove(lock + "/owner.pid")
	os.Remove(lock + "/owner.start")
	os.Remove(lock)
}

// reapRunLock is _invoker_reap_run_lock: true when it removed the lock it had
// judged dead, while the holder it judged is still the one on disk.
func (c *Ctx) reapRunLock(lock, seenOwner, seenStart string) bool {
	reap := lock + ".reap"
	rc := false
	if err := os.Mkdir(reap, 0o777); err != nil {
		if old, ok := c.olderThanMinute(reap); ok && old {
			os.Remove(reap)
		}
		return false
	}
	nowOwner := readTrim(lock + "/owner.pid")
	nowStart := readTrim(lock + "/owner.start")
	if nowOwner == seenOwner && nowStart == seenStart { // 101-M06
		if !isDigits(seenOwner) {
			if old, ok := c.olderThanMinute(lock); ok && old {
				rc = true
			}
		} else {
			rc = true
		}
	}
	if rc {
		os.Remove(lock + "/owner.pid")
		os.Remove(lock + "/owner.start")
		os.Remove(lock)
	}
	os.Remove(reap)
	return rc
}

// armLockSignals is invoker_arm_lock_signals; the process wires the signals to
// signalExit (130 for INT, 143 for TERM). (101-M05)
func (c *Ctx) armLockSignals() {}

// runLockAcquire is invoker_run_lock: one invoker at a time per run.
func (c *Ctx) runLockAcquire() error {
	lock := c.runLockPath()
	deadline := c.Now().Add(c.LockWait)
	pid := itoa(c.Pid)
	for {
		if err := os.Mkdir(lock, 0o777); err == nil {
			os.WriteFile(lock+"/owner.pid", []byte(pid+"\n"), 0o666)
			start := c.procStart(pid)
			if start != "" {
				start += "\n"
			}
			os.WriteFile(lock+"/owner.start", []byte(start), 0o666)
			c.runLock = lock
			c.armLockSignals()
			return nil
		}
		owner := readTrim(lock + "/owner.pid")
		switch {
		case !isDigits(owner):
			old, ok := c.olderThanMinute(lock)
			if ok && old {
				claimStart := readTrim(lock + "/owner.start")
				if c.reapRunLock(lock, owner, claimStart) { // 101-M07
					c.errf("%s: breaking an invoker lock that records no holder (%s, older than a minute). Nothing has been read or sent yet.\n", c.Prog, lock)
					continue
				}
			}
		case owner == pid:
			c.runLock = lock
			return nil
		default:
			ownerStart := readTrim(lock + "/owner.start") // 101-M04
			if ownerStart != "" {
				live := c.procStart(owner)
				if live != "" && live != ownerStart && c.reapRunLock(lock, owner, ownerStart) {
					c.errf("%s: breaking an invoker lock whose holder pid was REUSED (%s, pid %s started %s, the recorded holder started %s). The invoker that took this lock is gone.\n", c.Prog, lock, owner, live, ownerStart)
					continue
				}
			}
			if !c.pidAlive(owner) && c.reapRunLock(lock, owner, ownerStart) { // 101-M02
				c.errf("%s: breaking an invoker lock whose holder is gone (%s, pid %s). That is a crashed or killed invoker, not a busy one.\n", c.Prog, lock, owner)
				continue
			}
		}
		if !c.Now().Before(deadline) {
			who := owner
			if who == "" {
				who = "none recorded"
			}
			return die("another invoker for this run has held %s for over 120s (pid %s, still alive). Nothing was sent. Retry.", lock, who)
		}
		c.Sleep(time.Second)
	}
}

// pidAlive is `kill -0`: true only when it succeeds (EPERM is a failure there too).
func (c *Ctx) pidAlive(pid string) bool {
	n := 0
	for i := 0; i < len(pid); i++ {
		n = n*10 + int(pid[i]-'0')
		if n > 1<<30 {
			return false
		}
	}
	return kill0(n)
}

var _ = syscall.Getpid
var _ = filepath.Join
