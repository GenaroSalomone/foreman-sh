package main

// The helpers `hw next` reaches outside lib/hw/next.sh, ported under next… names. MANY OF THEM BELONG TO THE
// DISPATCH PATH (train T5): the run env reader, the run directory, the markers, the chaining lease, the
// receipt, the ledger, the lane roots. They are copied here byte for byte, from the bash each one names, so
// `hw next` can be held to its contract now; T5 owns them and dedupes these copies when it ports the launch.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// nextBinDir is $HW_BIN_DIR: the shim hands it down as HW_CORE_BIN_DIR.
func (a *app) nextBinDir() string {
	if d := a.getenv("HW_CORE_BIN_DIR"); d != "" {
		return d
	}
	return a.c.brain + "/bin"
}

// nextEnvOr is ${VAR:-def}.
func (a *app) nextEnvOr(k, def string) string {
	if v := a.getenv(k); v != "" {
		return v
	}
	return def
}

// nextOperator is $HW_OPERATOR after lane_config_load: the caller's, else the table's `operator`, else
// "the operator" (project-spaces.sh: `.operator // "the operator" | tostring`).
func (a *app) nextOperator() string {
	if v := a.getenv("HW_OPERATOR"); v != "" {
		return v
	}
	data, err := os.ReadFile(a.c.t.file)
	if err == nil {
		if vals, _ := parseStream(data); len(vals) > 0 {
			if op, ok := vals[0].get("operator"); ok && op.truthy() {
				if op.kind == vStr {
					return op.s
				}
				return op.toJSON()
			}
		}
	}
	return "the operator"
}

// ── the clock: `date`, as bash asks it ──────────────────────────────────────

// nextEpoch is $(date +%s). Asked of `date` itself, as bash asks it, so a fixed clock answers both.
func nextEpoch() int64 {
	if out, err := exec.Command("date", "+%s").Output(); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
			return n
		}
	}
	return time.Now().Unix()
}

// nextDateUTC is $(date -u '+%Y-%m-%dT%H:%M:%SZ').
func nextDateUTC() string {
	if out, err := exec.Command("date", "-u", "+%Y-%m-%dT%H:%M:%SZ").Output(); err == nil {
		return bashSub(string(out))
	}
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// ── the run env: `_run_env_value <rundir> <KEY>` (bin/runenv --bare get) ────

// nextRunEnv is _run_env_value: the value and runenv's status — 0 found, 1 absent, 2 damaged. A damaged
// file (unreadable, not UTF-8) is left to bin/runenv itself, so its words on stderr are its own; loud says
// whether that stderr reaches the terminal (bash's call sites without 2>/dev/null).
func (a *app) nextRunEnv(rundir, key string, loud bool) (string, int) {
	path := rundir + "/env"
	if isDir(path) { // runenv: a directory target means <dir>/env
		path += "/env"
	}
	data, err := os.ReadFile(path)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return "", 1
	}
	if err == nil && utf8.Valid(data) {
		kvs, perr := ParseEnv(data, true, true)
		if perr == nil {
			for _, kv := range kvs {
				if kv.Key == key {
					return kv.Value, 0
				}
			}
			return "", 1
		}
	}
	cmd := exec.Command(a.nextBinDir()+"/runenv", "--bare", "get", rundir+"/env", key)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	rc := exitStatus(cmd.Run())
	if loud {
		a.err.Write(errb.Bytes())
	}
	return out.String(), rc
}

// nextEnvVal is `$(_run_env_value … 2>/dev/null || true)`.
func (a *app) nextEnvVal(rundir, key string) string {
	v, _ := a.nextRunEnv(rundir, key, false)
	return bashSub(v)
}

// ── the run directory and its markers ───────────────────────────────────────

// nextRunDir is _next_run_dir: the newest run at or above cwd, twelve hops at most.
func nextRunDir(cwd string) (string, bool) {
	d := cwd
	for hops := 0; hops < 12 && d != "" && d != "/"; hops++ {
		if isDir(d + "/.hw") {
			if ids := RunIDs(d + "/.hw"); len(ids) > 0 {
				return d + "/.hw/" + ids[0], true
			}
		}
		d = filepath.Dir(d)
	}
	return "", false
}

// nextTaskDigits is `tr -dc '0-9' < <rundir>/task` when the file is readable, else "".
func nextTaskDigits(rundir string) string {
	b, err := os.ReadFile(rundir + "/task")
	if err != nil {
		return ""
	}
	return digitsOnly(string(b))
}

// nextDoneMarker is _run_done_marker: task 1 keeps the run directory, task N is t<N>/.
func nextDoneMarker(rundir string) string {
	switch seq := nextTaskDigits(rundir); seq {
	case "", "0", "1":
		return rundir + "/done"
	default:
		return rundir + "/t" + seq + "/done"
	}
}

// nextReopenedMarker is _run_reopened_marker: beside the done marker.
func nextReopenedMarker(rundir string) string {
	return strings.TrimSuffix(nextDoneMarker(rundir), "/done") + "/reopened"
}

// nextCurrentTask is _chaining_current_task.
func nextCurrentTask(rundir string) string {
	n := nextTaskDigits(rundir)
	if n == "" || n == "0" {
		n = "1"
	}
	return n
}

// nextDispatchFramework is _run_dispatch_framework: the second word of the first `  framework ` line.
func nextDispatchFramework(rundir string) string {
	b, err := os.ReadFile(rundir + "/dispatch")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "  framework") && len(l) > len("  framework") && nextAwkSpace(l[len("  framework")]) {
			f := strings.Fields(l)
			if len(f) >= 2 {
				return f[1]
			}
			return ""
		}
	}
	return ""
}

func nextAwkSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

// nextDispatchModel is _next_dispatch_model: the value after `model=` on the first such line, a trailing
// `  (mark)` anchored at its end taken off.
func nextDispatchModel(rundir string) string {
	b, err := os.ReadFile(rundir + "/dispatch")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		t := strings.TrimLeft(l, " \t\v\f\r")
		if !strings.HasPrefix(t, "model=") {
			continue
		}
		line := t[len("model="):]
		if m := nextReMark.FindStringIndex(line); m != nil { // # MUTATION-ANCHOR: 862-G01
			line = line[:m[0]]
		}
		return line
	}
	return ""
}

// ── the chaining lease ──────────────────────────────────────────────────────

// nextLeaseField is _chaining_lease_field: the LAST `key=` line's value.
func nextLeaseField(rundir, key string) string {
	b, err := os.ReadFile(rundir + "/chaining-lease")
	if err != nil {
		return ""
	}
	v := ""
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if strings.HasPrefix(l, key+"=") {
			v = l[len(key)+1:]
		}
	}
	return v
}

// nextLeaseEligibility is _chaining_lease_eligibility: live, consumed, expired, missing or mismatched.
func nextLeaseEligibility(rundir, seq string) string {
	if !readableFile(rundir + "/chaining-lease") {
		return "missing"
	}
	token, task := nextLeaseField(rundir, "token"), nextLeaseField(rundir, "task")
	state, until := nextLeaseField(rundir, "state"), nextLeaseField(rundir, "until")
	if token == "" {
		return "mismatched"
	}
	switch state {
	case "consumed":
		return "consumed"
	case "expired", "expired-cleanup":
		return "expired"
	case "live":
	default:
		return "mismatched"
	}
	if task != seq { // # MUTATION-ANCHOR: 862-G02
		return "mismatched"
	}
	if until == "" || strings.Trim(until, "0123456789") != "" {
		return "mismatched"
	}
	// `[ "$(date +%s)" -lt "$until" ]`: test reads decimal, and a number it cannot hold is an error (expired)
	u, err := strconv.ParseInt(until, 10, 64)
	if err == nil && nextEpoch() < u { // # MUTATION-ANCHOR: 862-G03
		return "live"
	}
	return "expired"
}

var nextReMark = regexp.MustCompile(`  \([^()]*\)$`)

// nextLeaseIneligible is _chaining_lease_ineligible_message.
func nextLeaseIneligible(state, seq, rundir string) string {
	switch state {
	case "consumed":
		return "task " + seq + " chaining authority was spent by a prior successful hw next. NOTHING was sent or touched; the task counter did NOT move."
	case "expired":
		at := ""
		if u := nextLeaseField(rundir, "until_h"); u != "" {
			at = " at " + u
		}
		return "task " + seq + " chaining authority expired" + at + ". NOTHING was sent or touched; the task counter did NOT move."
	case "missing":
		return "task " + seq + " has no chaining authority. NOTHING was sent or touched; the task counter did NOT move."
	}
	return "task " + seq + " chaining authority does not match its current task-bound lease. NOTHING was sent or touched; the task counter did NOT move."
}

// nextLockPath is _chaining_lock_path: $TMPDIR/hw-invoker-<run, every byte outside [A-Za-z0-9_.-] as _>.lock
// (basename's newline goes through `tr -c` too).
func (a *app) nextLockPath(rundir string) string {
	var sb strings.Builder
	for _, c := range []byte(filepath.Base(rundir) + "\n") {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '.', c == '-':
			sb.WriteByte(c)
		default:
			sb.WriteByte('_')
		}
	}
	return a.nextEnvOr("TMPDIR", "/tmp") + "/hw-invoker-" + sb.String() + ".lock"
}

// nextLockTake is _chaining_lock_take: mkdir the lock, a stale one (5 min) broken, 120 s at most.
func (a *app) nextLockTake(rundir string) (string, bool) {
	lock := a.nextLockPath(rundir)
	deadline := nextEpoch() + 120
	for os.Mkdir(lock, 0o777) != nil {
		if st, err := os.Stat(lock); err == nil && time.Since(st.ModTime()) > 5*time.Minute {
			os.Remove(lock)
			continue
		}
		if nextEpoch() >= deadline {
			return "", false
		}
		time.Sleep(time.Second)
	}
	return lock, true
}

// nextArith is `$((n))` of a digit string: a leading 0 makes it octal.
func nextArith(s string) (int64, bool) {
	if len(s) > 1 && s[0] == '0' {
		v, err := strconv.ParseInt(s, 8, 64)
		return v, err == nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

// ── the lane roots: WORK plus all_lane_worktree_roots ───────────────────────

// nextLaneRoots is all_lane_worktree_roots: every lane's lane_work_bases in survey order, minus what is
// under $WORK.
func (a *app) nextLaneRoots() []string {
	work := a.c.work
	var out []string
	for _, p := range a.c.t.survey {
		if p == "" || !a.c.t.known(p) {
			continue
		}
		root := a.c.lanePath(p, "worktree_root", "")
		var bases string
		if root == "" || root == work+"/"+p {
			bases = work + "/" + p
		} else {
			bases = root + " " + work + "/" + p
		}
		for _, b := range strings.Split(bases, " ") {
			if b == "" || strings.HasPrefix(b, work+"/") {
				continue
			}
			out = append(out, b)
		}
	}
	return out
}

// ── small shapes of the shell ───────────────────────────────────────────────

// nextWc is `printf '%s' "$x" | wc -c | tr -d ' '`.
func nextWc(s string) string { return strconv.Itoa(len(s)) }

func nextSha16(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}

// nextSortedUnique is `sort -u` of lines, byte order.
func nextSortedUnique(lines []string) []string {
	sort.Strings(lines)
	var out []string
	for i, l := range lines {
		if i == 0 || l != lines[i-1] {
			out = append(out, l)
		}
	}
	return out
}
