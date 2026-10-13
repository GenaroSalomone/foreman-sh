package main

// hw chaining-lease-start and chaining-lease-expire (bin/hw cmd_chaining_lease_*):
// the lease a `--keep-pane` executor holds between its report and the next task.
// `chaining-lease` is a key=value file in the run directory; a detached scheduler
// calls `hw chaining-lease-expire` when it runs out, and that closes the pane the
// run owns. Both are internal verbs of done-invoker and the scheduler.

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// t3LeaseField is _chaining_lease_field: the value of the LAST `key=` line.
func t3LeaseField(rundir, key string) string {
	b, err := os.ReadFile(rundir + "/chaining-lease")
	if err != nil {
		return ""
	}
	v := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, key+"=") {
			v = l[len(key)+1:]
		}
	}
	return v
}

// t3LeaseTaskMarker is _chaining_task_marker.
func t3LeaseTaskMarker(rundir, seq string) string {
	if n, err := strconv.Atoi(seq); err == nil && n > 1 {
		return rundir + "/t" + seq + "/done"
	}
	return rundir + "/done"
}

// t3Arith is `$((n))` of a digit string: a leading 0 makes it octal, and a digit 8 or 9 in it is an error.
func t3Arith(s string) (int64, bool) {
	if len(s) > 1 && s[0] == '0' {
		v, err := strconv.ParseInt(s, 8, 64)
		return v, err == nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

func (t *t3) leaseStart() int {
	rundir := t.getenv("HW_WORKDIR") + "/.hw/" + t.getenv("HW_RUN")
	seq := mkTaskSeq(rundir)
	seconds, reason := t.getenv("HW_CHAINING_LEASE_SECONDS"), t.getenv("HW_CHAINING_LEASE_REASON")
	holder := t.getenv("HW_CHAINING_LEASE_HOLDER")
	if holder == "" {
		holder = t.getenv("HW_INVOKER_PANE")
	}
	if holder == "" {
		holder = "unknown"
	}
	if seconds == "" || strings.Trim(seconds, "0123456789") != "" {
		t.die("delivered, but chaining lease duration is absent or invalid")
	}
	if reason == "" {
		t.die("delivered, but chaining lease reason is absent")
	}
	if !isFile(t3LeaseTaskMarker(rundir, seq)) { // # MUTATION-ANCHOR: 841-L01
		t.die("refusing to start a chaining lease before task " + seq + " has a done marker")
	}
	var tok [16]byte
	if _, err := rand.Read(tok[:]); err != nil {
		t.die("could not create chaining lease identity")
	}
	token := hex.EncodeToString(tok[:])
	secs, ok := t3Arith(seconds)
	if !ok {
		return 1 // bash: the arithmetic error ends the verb
	}
	now := time.Now().Unix()
	until := now + secs
	untilH := time.Unix(until, 0).Format("2006-01-02T15:04:05-0700")
	by := t.getenv("HERDR_PANE_ID")
	if by == "" {
		by = "unknown"
	}
	lease := rundir + "/chaining-lease"
	body := t3Lines(
		"token="+token,                        // # MUTATION-ANCHOR: 841-L10
		"task="+seq,                           // # MUTATION-ANCHOR: 841-L11
		"until="+strconv.FormatInt(until, 10), // # MUTATION-ANCHOR: 841-L12
		"until_h="+untilH,                     // # MUTATION-ANCHOR: 841-L13
		"reason="+reason,                      // # MUTATION-ANCHOR: 841-L14
		"holder="+holder,                      // # MUTATION-ANCHOR: 841-L15
		"by="+by,                              // # MUTATION-ANCHOR: 841-L16
		"created="+t3IsoNow(),                 // # MUTATION-ANCHOR: 841-L17
		"state=live",                          // # MUTATION-ANCHOR: 841-L18
	)
	if err := t.fs.write(lease, body, 0o666); err != nil {
		t.die("could not write " + lease)
	}
	pid := 0
	if t.fs.readonly {
		// shadow: bash starts the scheduler; the plan records the line it will append, with a stand-in pid
		t.fs.appendTo(lease, []byte("scheduler_pid=0\n"), 0o666)
		t.fs.write(rundir+"/chaining-lease-expiry.log", nil, 0o666) // the scheduler's log, which bash creates empty
	} else {
		logf, err := os.OpenFile(rundir+"/chaining-lease-expiry.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			return 1
		}
		cmd := exec.Command("nohup", "bash", "-c", `sleep "$1"; exec "$2" chaining-lease-expire "$3" "$4" "$5"`, "_",
			seconds, t.binDir()+"/hw", rundir, token, seq)
		cmd.Stdout, cmd.Stderr = logf, logf
		if devnull, err := os.Open(os.DevNull); err == nil { // an async list has no stdin
			cmd.Stdin = devnull
			defer devnull.Close()
		}
		if err := cmd.Start(); err != nil {
			os.Remove(lease)
			logf.Close()
			t.die("could not schedule chaining lease expiry")
		}
		logf.Close()
		pid = cmd.Process.Pid
		time.Sleep(0) // the scheduler is a child of the hook: it is not waited for
		if !t3PidAlive(pid) {
			os.Remove(lease)
			t.die("could not schedule chaining lease expiry")
		}
		if err := t.fs.appendTo(lease, []byte("scheduler_pid="+strconv.Itoa(pid)+"\n"), 0o666); err != nil { // # MUTATION-ANCHOR: 841-L03
			cmd.Process.Kill()
			os.Remove(lease)
			t.die("could not record chaining lease scheduler")
		}
		cmd.Process.Release()
	}
	t.ok("chaining lease active until " + untilH + " — holder " + holder + "; expiry closes the owned pane")
	return 0
}

// ── expire ───────────────────────────────────────────────────────────────────

// t3LeaseLockPath is _chaining_lock_path: basename's newline goes through `tr -c` too.
func t3LeaseLockPath(rundir string, getenv func(string) string) string {
	base := rundir[strings.LastIndexByte(strings.TrimRight(rundir, "/"), '/')+1:]
	var sb strings.Builder
	for _, r := range base + "\n" {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	tmp := getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	return tmp + "/hw-invoker-" + sb.String() + ".lock"
}

func (t *t3) leaseLockTake(rundir string) (string, bool) {
	lock := t3LeaseLockPath(rundir, t.getenv)
	deadline := time.Now().Add(120 * time.Second)
	for os.Mkdir(lock, 0o777) != nil {
		if st, err := os.Stat(lock); err == nil && time.Since(st.ModTime()) > 5*time.Minute {
			os.Remove(lock)
			continue
		}
		if !time.Now().Before(deadline) {
			return "", false
		}
		time.Sleep(time.Second)
	}
	return lock, true
}

func (t *t3) leaseExpire() int {
	if len(t.args) != 3 {
		t.die("internal chaining-lease-expire needs rundir token task")
	}
	rundir, token, seq := t.args[0], t.args[1], t.args[2]
	lease := rundir + "/chaining-lease"
	lock, ok := t.leaseLockTake(rundir)
	if !ok {
		t.die("could not acquire the run lock to expire " + lease)
	}
	release := func() { os.Remove(lock) }
	// # MUTATION-ANCHOR: 841-L04
	if t3LeaseField(rundir, "token") != token || t3LeaseField(rundir, "task") != seq ||
		mkTaskSeq(rundir) != seq || !isFile(t3LeaseTaskMarker(rundir, seq)) {
		release()
		return 0
	}
	t.fs.appendTo(lease, []byte("state=expired-cleanup\n"), 0o666)     // # MUTATION-ANCHOR: 841-E01
	t.fs.appendTo(lease, []byte("expired_at="+t3IsoNow()+"\n"), 0o666) // # MUTATION-ANCHOR: 841-E02
	kind, id := "", ""
	if b, err := os.ReadFile(rundir + "/tab"); err == nil {
		kind, id = "tab", strings.Join(strings.Fields(string(b)), "")
	} else if b, err := os.ReadFile(rundir + "/workspace"); err == nil {
		kind, id = "workspace", strings.Join(strings.Fields(string(b)), "")
	} else {
		t.fs.appendTo(lease, []byte("cleanup=no-owned-container\n"), 0o666) // # MUTATION-ANCHOR: 841-E05
		release()
		return 0
	}
	if t.closeHerdrObject(kind, id, "closed expired chaining "+kind+" "+id, kind+" "+id+" already gone") { // # MUTATION-ANCHOR: 841-L05
		t.fs.appendTo(lease, []byte("cleanup=closed\n"), 0o666) // # MUTATION-ANCHOR: 841-E03
		release()
		return 0
	}
	t.fs.appendTo(lease, []byte("cleanup=failed\n"), 0o666) // # MUTATION-ANCHOR: 841-E04
	release()
	return 1
}

// closeHerdrObject is _close_herdr_object: close it, or prove it is already gone. A
// failed close is not evidence of absence — only a parseable list that omits the id is.
func (t *t3) closeHerdrObject(kind, id, success, absent string) bool {
	h := herdrClient{getenv: t.getenv}
	out, rc := h.capture(kind, "close", id)
	if rc == 0 {
		t.ok(success)
		return true
	}
	out = bashSub(out)
	reason := t3Head3TR(out)
	if reason == "" {
		reason = "no reason returned"
	}
	exists, why := t.herdrObjectExists(kind, id)
	switch exists {
	case 0:
		t.warn("CLOSE FAILED: " + kind + " " + id + " still exists (close exit " + strconv.Itoa(rc) + ": " + reason + ")")
		return false
	case 1:
		t.info(absent)
		return true
	}
	t.warn("CLOSE FAILED: " + kind + " " + id + " could not be closed (exit " + strconv.Itoa(rc) + ": " + reason + ")")
	t.warn("absence could not be verified: " + why)
	return false
}

// herdrObjectExists is _herdr_object_exists: 0 it is there, 1 it is not, 2 unknown (and why).
func (t *t3) herdrObjectExists(kind, id string) (int, string) {
	var array, field string
	switch kind {
	case "pane":
		array, field = "panes", "pane_id"
	case "tab":
		array, field = "tabs", "tab_id"
	case "workspace":
		array, field = "workspaces", "workspace_id"
	default:
		return 2, "unknown herdr object kind: " + kind
	}
	out, rc := herdrClient{getenv: t.getenv}.capture(kind, "list")
	out = bashSub(out)
	if rc != 0 {
		return 2, "list exited " + strconv.Itoa(rc) + ": " + t3Head3TR(out)
	}
	vals, _ := parseStream([]byte(out))
	var list *jval
	for _, v := range vals {
		r, ok := v.get("result")
		if !ok {
			break
		}
		a, ok := r.get(array)
		if ok && a.kind == vArr {
			list = a
		} else {
			list = nil
		}
	}
	if list == nil {
		return 2, "list returned no parseable .result." + array + " array"
	}
	for _, e := range list.arr {
		f, ok := e.get(field)
		if !ok {
			return 1, "" // jq errors on a non-object element: `-e` exits non-zero, which reads as absent
		}
		if f.kind == vStr && f.s == id {
			return 0, ""
		}
	}
	return 1, ""
}

// t3Head3TR is `printf '%s' "$out" | head -3 | tr '\n' ' '`: the first three lines, each
// newline a space — a newline that ended the third line included.
func t3Head3TR(out string) string {
	lines := strings.Split(out, "\n")
	if len(lines) > 3 {
		return strings.Join(lines[:3], " ") + " "
	}
	return strings.Join(lines, " ")
}
