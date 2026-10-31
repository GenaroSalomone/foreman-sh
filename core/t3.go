package main

// Train T3: the verbs that WRITE the markers hooks and invokers read —
// ruling, human-boundary, executor-turn-end, chaining-lease-start/-expire,
// outbox flush, output and stage. The bytes they leave on disk are the
// contract (setup/contracts/hw.md §1): a marker that differs by a byte is a
// hook that reads something else.
//
// Every write goes through fsx, never through os directly, for one reason:
// SHADOW. `HW_IMPL=shadow` for a writer runs this binary FIRST with
// HW_CORE_READONLY=1 and HW_CORE_PLAN=<file>, then bash for real. In that mode
// fsx performs nothing and records what it would have done; bin/hw then asks
// `hw-core shadow-verify <plan>` whether the files bash left are the files this
// plan describes (shadowVerify). Go computes and diffs; bash writes. The run
// directory is never written twice.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func t3NowUTC() time.Time { return time.Now().UTC() }

// t3Verbs are the verbs of this train; the value is whether an executor may
// run it (bin/hw's _executor_refuse gate, in Go since executor_gate.go: runWith refuses the others, with bash's words,
// when HW_TASK or HW_RUN is set).
var t3Verbs = map[string]bool{
	"ruling": false, "human-boundary": true, "executor-turn-end": true,
	"chaining-lease-start": true, "chaining-lease-expire": true,
	"outbox-flush": false, "output": true, "stage": true,
	"shadow-verify": true, // not a verb of hw: the shim's question after a shadow run
}

func isT3Verb(v string) bool { _, ok := t3Verbs[v]; return ok }

// ── the file system, with a plan ─────────────────────────────────────────────

type fsOp struct {
	Op   string `json:"op"` // write | append | remove | rename | mkdir | symlink | scope (a directory as it was when Go finished: Data is its listing)
	Path string `json:"path"`
	To   string `json:"to,omitempty"`
	Data string `json:"data,omitempty"` // base64
	Mode uint32 `json:"mode,omitempty"`
}

type fsx struct {
	readonly bool
	planPath string
	ops      []fsOp
	watched  []string // directories whose listing goes into the plan beside those the ops name
}

func newFsx(getenv func(string) string) *fsx {
	return &fsx{readonly: getenv("HW_CORE_READONLY") != "", planPath: getenv("HW_CORE_PLAN")}
}

func (f *fsx) rec(op fsOp) {
	if f.planPath != "" {
		f.ops = append(f.ops, op)
	}
}

// watch names a directory the verb writes into even when this run planned nothing there: what bash leaves in it
// that the plan does not account for is a write Go would have missed.
func (f *fsx) watch(d string) {
	if f.readonly && f.planPath != "" {
		f.watched = append(f.watched, d)
	}
}

// t3Listing is the regular files of a directory, one `name size mtime_ns crc` line each, sorted by name. The CRC of
// the bytes (of a file up to 1 MiB; 0 above) is there because size and mtime alone miss a same-size rewrite: Linux
// stamps a file from a coarse clock (a few ms), so `turns` going from `1` to `2` inside one tick kept both.
func t3Listing(d string) string {
	ents, _ := os.ReadDir(d)
	var b strings.Builder
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		var sum uint32
		if fi.Size() <= 1<<20 {
			if data, err := os.ReadFile(filepath.Join(d, e.Name())); err == nil {
				sum = crc32.ChecksumIEEE(data)
			}
		}
		fmt.Fprintf(&b, "%s %d %d %08x\n", e.Name(), fi.Size(), fi.ModTime().UnixNano(), sum)
	}
	return b.String()
}

// flush writes the plan, once, when the verb is over: the ops, then the listing of every directory they (or watch)
// name, taken now, before bash writes anything.
func (f *fsx) flush() {
	if !f.readonly || f.planPath == "" {
		return
	}
	dirs := map[string]bool{}
	for _, d := range f.watched {
		dirs[d] = true
	}
	for _, op := range f.ops {
		dirs[filepath.Dir(op.Path)] = true
	}
	names := make([]string, 0, len(dirs))
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)
	var b bytes.Buffer
	for _, op := range f.ops {
		j, _ := json.Marshal(op)
		b.Write(j)
		b.WriteByte('\n')
	}
	for _, d := range names {
		j, _ := json.Marshal(fsOp{Op: "scope", Path: d, Data: t3B64([]byte(t3Listing(d)))})
		b.Write(j)
		b.WriteByte('\n')
	}
	os.WriteFile(f.planPath, b.Bytes(), 0o600)
}

func t3B64(d []byte) string { return base64.StdEncoding.EncodeToString(d) }

func (f *fsx) mkdirAll(p string) error {
	if f.readonly {
		f.rec(fsOp{Op: "mkdir", Path: p})
		return nil
	}
	return os.MkdirAll(p, 0o777)
}

// write is `printf … > file`: create or truncate; an existing file keeps its mode.
func (f *fsx) write(p string, data []byte, mode os.FileMode) error {
	if f.readonly {
		f.rec(fsOp{Op: "write", Path: p, Data: t3B64(data), Mode: uint32(mode)})
		return nil
	}
	return os.WriteFile(p, data, mode)
}

// appendTo is `printf … >> file`.
func (f *fsx) appendTo(p string, data []byte, mode os.FileMode) error {
	if f.readonly {
		f.rec(fsOp{Op: "append", Path: p, Data: t3B64(data), Mode: uint32(mode)})
		return nil
	}
	fh, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(data)
	return err
}

// remove is `rm -f`.
func (f *fsx) remove(p string) {
	if f.readonly {
		f.rec(fsOp{Op: "remove", Path: p})
		return
	}
	os.Remove(p)
}

// symlink is `ln -s target link`.
func (f *fsx) symlink(target, link string) error {
	if f.readonly {
		f.rec(fsOp{Op: "symlink", Path: link, To: target})
		return nil
	}
	return os.Symlink(target, link)
}

// rename is `mv`: the claim that exactly one of several racing processes wins.
func (f *fsx) rename(from, to string) error {
	if f.readonly {
		f.rec(fsOp{Op: "rename", Path: from, To: to})
		return nil
	}
	return os.Rename(from, to)
}

// ── what the verbs share ─────────────────────────────────────────────────────

type t3 struct {
	*app
	fs *fsx
}

// binDir is HW_BIN_DIR: where cockpit-state, holdfacts, channel-send and hw live.
func (t *t3) binDir() string {
	if d := t.getenv("HW_CORE_BIN_DIR"); d != "" {
		return d
	}
	return filepath.Join(t.c.brain, "bin")
}

func (a *app) cmdT3(verb string) int {
	// bin/hw, for a caller that is an executor (HW_TASK or HW_RUN set), gives HW_PROJECT and HW_TASK a `?` before
	// any verb runs; a verb that reads them sees `?`, not an empty string.
	if a.getenv("HW_TASK") != "" || a.getenv("HW_RUN") != "" {
		inner := a.getenv
		a.getenv = func(k string) string {
			if v := inner(k); v != "" || (k != "HW_PROJECT" && k != "HW_TASK") {
				return v
			}
			return "?"
		}
		a.c.getenv = a.getenv
	}
	t := &t3{app: a, fs: newFsx(a.getenv)}
	defer t.fs.flush()
	switch verb {
	case "human-boundary", "executor-turn-end", "chaining-lease-start", "output":
		if d, ok := t.executorStateDir(); ok {
			t.fs.watch(d)
		}
	}
	switch verb {
	case "human-boundary":
		return t.humanBoundary()
	case "output":
		return t.output()
	case "stage":
		return t.stage()
	case "ruling":
		return t.ruling()
	case "executor-turn-end":
		return t.executorTurnEnd()
	case "chaining-lease-start":
		return t.leaseStart()
	case "chaining-lease-expire":
		return t.leaseExpire()
	case "outbox-flush":
		return t.outboxFlush()
	case "shadow-verify":
		return t.shadowVerify()
	}
	return 64
}

// executorStateDir is _executor_state_dir.
func (t *t3) executorStateDir() (string, bool) {
	wd, run := t.getenv("HW_WORKDIR"), t.getenv("HW_RUN")
	if wd == "" || run == "" {
		return "", false
	}
	return wd + "/.hw/" + run, true
}

// charLen is bash's ${#var}: characters in a UTF-8 locale, bytes otherwise.
func (t *t3) charLen(s string) int {
	loc := t.getenv("LC_ALL")
	if loc == "" {
		loc = t.getenv("LC_CTYPE")
	}
	if loc == "" {
		loc = t.getenv("LANG")
	}
	l := strings.ToLower(loc)
	if strings.Contains(l, "utf-8") || strings.Contains(l, "utf8") {
		return utf8.RuneCountInString(s)
	}
	return len(s)
}

// substr is bash's ${var:0:n}, by the same unit.
func (t *t3) substr(s string, n int) string {
	if t.charLen(s) == len(s) { // bytes, or pure ASCII
		if len(s) > n {
			return s[:n]
		}
		return s
	}
	i := 0
	for k := 0; k < n && i < len(s); k++ {
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
	}
	return s[:i]
}

// t3Digits is `tr -dc '0-9'`.
func t3Digits(s string) string { return digitsOnly(s) }

// mkTaskSeq is _chaining_current_task: the counter's digits, "1" when none or "0".
func mkTaskSeq(rundir string) string {
	n := ""
	if b, err := os.ReadFile(rundir + "/task"); err == nil {
		n = t3Digits(string(b))
	}
	if n == "" || n == "0" {
		n = "1"
	}
	return n
}

// mkTaskDir is witness_task_dir (and _run_done_marker's directory): the run
// directory for task 1, t<N>/ after it.
func mkTaskDir(rundir string) string {
	n := ""
	if b, err := os.ReadFile(rundir + "/task"); err == nil {
		n = t3Digits(string(b))
	}
	switch n {
	case "", "0", "1":
		return rundir
	}
	return rundir + "/t" + n
}

// mkReadSub is `$(cat file)`: NULs dropped, trailing newlines stripped.
func mkReadSub(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return bashSub(string(b))
}

func t3IsoNow() string { return t3NowUTC().Format("2006-01-02T15:04:05Z") }

// tick is the fixed point of the writer's volatile fields in the golden: set by
// the world, never by a user.
func t3ShadowNorm(b []byte) []byte {
	b = t3ReISO.ReplaceAll(b, []byte("@AT@"))
	b = t3ReEpoch.ReplaceAll(b, []byte("@EPOCH@"))
	b = t3ReHexTok.ReplaceAll(b, []byte("@TOKEN@"))
	b = t3RePid.ReplaceAll(b, []byte("scheduler_pid=@PID@")) // the plan cannot know the pid bash's scheduler gets
	return b
}

var (
	t3ReISO    = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(Z|[+-][0-9]{4})`)
	t3ReEpoch  = regexp.MustCompile(`\b1[0-9]{9}\b`)
	t3ReHexTok = regexp.MustCompile(`\b[0-9a-f]{32}\b`)
	t3RePid    = regexp.MustCompile(`scheduler_pid=[0-9]+`)
)

// shadowVerify is the shim's question after a shadow run of a writer:
// `hw-core shadow-verify <plan>`. It replays the plan per path and compares
// the result with what bash left (volatile fields — timestamps, epochs, pids,
// the lease token — normalised, and only those). One line per difference; 1
// when there is any.
func (t *t3) shadowVerify() int {
	args, launch := t.args, false
	if len(args) == 2 && args[0] == "--launch" { // the plan of a dispatch's launch (t5l_shadow.go)
		args, launch = args[1:], true
	}
	if len(args) != 1 {
		fmt.Fprintln(t.err, "usage: hw-core shadow-verify [--launch] <plan>")
		return 64
	}
	norm := t3ShadowNorm
	if launch {
		norm = t5lShadowNorm
	}
	syms := map[string]string{}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return 0 // no plan: the Go half had nothing to say (it died before writing one)
	}
	type state struct {
		known   bool // the first op fixed the whole content (write/remove), not a tail
		absent  bool
		content []byte
	}
	order := []string{}
	st := map[string]*state{}
	owned := map[string]bool{} // paths the plan itself creates: their run id / stamp is Go's own, so bash's file is found by what it is once normalised
	scopes := map[string]string{}
	scopeOrder := []string{}
	get := func(p string) *state {
		s, ok := st[p]
		if !ok {
			s = &state{}
			st[p] = s
			order = append(order, p)
		}
		return s
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var op fsOp
		if err := dec.Decode(&op); err != nil {
			if err != io.EOF {
				fmt.Fprintln(t.out, "plan: unreadable: "+err.Error())
				return 1
			}
			break
		}
		raw, _ := base64.StdEncoding.DecodeString(op.Data)
		switch op.Op {
		case "scope":
			scopes[op.Path] = string(raw)
			scopeOrder = append(scopeOrder, op.Path)
		case "write":
			s := get(op.Path)
			owned[op.Path] = true
			s.known, s.absent, s.content = true, false, raw
		case "append":
			s := get(op.Path)
			owned[op.Path] = true
			s.content = append(s.content, raw...)
			if !s.known && s.content == nil {
				s.content = []byte{}
			}
		case "remove":
			s := get(op.Path)
			s.known, s.absent, s.content = true, true, nil
		case "rename":
			// a claim: the source is gone. Its destination is a temp name that bash
			// removes too, so only the source's absence is checked.
			s := get(op.Path)
			s.known, s.absent, s.content = true, true, nil
		case "mkdir":
			s := get(op.Path)
			owned[op.Path] = true
			s.known, s.absent, s.content = true, false, nil
			s.content = nil
		case "symlink":
			owned[op.Path] = true
			syms[op.Path] = op.To
		case "exec": // a call: bash's are not observable in production, so they are not compared (t5l_shadow.go)
		}
	}
	bad := 0
	// A read-only Go leg goes on where bash stopped (it cannot see a mutating call fail): what bash never got to write is not a difference.
	bashStopped := launch && t.getenv("HW_SHADOW_BASH_RC") != "" && t.getenv("HW_SHADOW_BASH_RC") != "0"
	// what bash left in a directory Go was working in, that Go's plan does not account for
	for _, d := range scopeOrder {
		if launch { // children (runenv, cockpit-state, git) write beside the launch's own files: only what was planned is checked
			break
		}
		was := map[string]string{}
		for _, l := range strings.Split(scopes[d], "\n") {
			if i := strings.IndexByte(l, ' '); i > 0 {
				was[l[:i]] = l[i+1:]
			}
		}
		now := map[string]string{}
		for _, l := range strings.Split(t3Listing(d), "\n") {
			if i := strings.IndexByte(l, ' '); i > 0 {
				now[l[:i]] = l[i+1:]
			}
		}
		var all []string
		for n := range was {
			all = append(all, n)
		}
		for n := range now {
			if _, ok := was[n]; !ok {
				all = append(all, n)
			}
		}
		sort.Strings(all)
		for _, n := range all {
			if _, planned := st[d+"/"+n]; planned {
				continue
			}
			a, hadBefore := was[n]
			b, existsNow := now[n]
			switch {
			case !hadBefore:
				fmt.Fprintf(t.out, "%s/%s: bash wrote it, Go did not plan it\n", d, n)
				bad++
			case !existsNow:
				fmt.Fprintf(t.out, "%s/%s: bash removed it, Go did not plan it\n", d, n)
				bad++
			case a != b:
				fmt.Fprintf(t.out, "%s/%s: bash changed it, Go did not plan it\n", d, n)
				bad++
			}
		}
	}
	for _, p := range order {
		s := st[p]
		if launch {
			if t5lVolatileFile(p) {
				if _, e := os.Stat(t5lResolve(p, norm)); e != nil && !s.absent {
					fmt.Fprintf(t.out, "%s: Go wrote it, bash did not\n", p)
					bad++
				}
				continue
			}
			// A path the plan did not create is a name that already existed (a backup Go prunes, a file it removes): it is
			// looked up AS IS. Resolving it by its normalised name would find the newest sibling instead, and report a
			// file bash correctly removed as one it "left" (measured 2026-10-12: `.claude.json.bak-<stamp>`).
			if t5lGoOwns(owned, p) {
				p = t5lResolve(p, norm)
			}
		}
		got, err := os.ReadFile(p)
		exists := err == nil
		if fi, e := os.Stat(p); e == nil && fi.IsDir() && !s.absent {
			continue // a mkdir: it exists
		}
		switch {
		case s.absent && exists:
			fmt.Fprintf(t.out, "%s: Go removed it, bash left it\n", p)
			bad++
		case s.absent:
		case !exists:
			if bashStopped {
				continue
			}
			fmt.Fprintf(t.out, "%s: Go wrote it, bash did not\n", p)
			bad++
		case s.known && launch && t5lLiveCopy(p): // exists (checked above); its bytes are a live file's, read at two moments
		case s.known:
			if launch && strings.Contains(p, "/.hw/") && strings.HasSuffix(p, "/env") { // values carry ports, panes and paths of two processes: the names are the contract
				if !t5lEnvKeysEqual(got, s.content) {
					fmt.Fprintf(t.out, "%s: the variables differ\n  go:   %v\n  bash: %v\n", p, t5lEnvKeys(s.content), t5lEnvKeys(got))
					bad++
				}
				continue
			}
			if !bytes.Equal(norm(got), norm(s.content)) {
				fmt.Fprintf(t.out, "%s: contents differ\n  go:   %q\n  bash: %q\n", p, norm(s.content), norm(got))
				bad++
			}
		default: // an append to what was there: the tail must be ours
			if launch && bashStopped && (strings.HasSuffix(p, "/receipt.jsonl") || strings.HasSuffix(p, "/env")) {
				continue
			}
			if launch && strings.HasSuffix(p, "/receipt.jsonl") { // the values come from live herdr/engram answers a read-only leg only guesses: the keys, in order, are the contract
				if !t5lKeysSuffix(got, s.content) {
					fmt.Fprintf(t.out, "%s: the appended keys differ\n  go:   %v\n  bash: %v\n", p, t5lKeys(s.content), t5lKeys(got))
					bad++
				}
				continue
			}
			if !bytes.HasSuffix(norm(got), norm(s.content)) {
				fmt.Fprintf(t.out, "%s: the appended tail differs\n  go:   %q\n  bash: %q\n", p, norm(s.content), norm(got))
				bad++
			}
		}
	}
	bad += t5lVerifySymlinks(t.out, syms, norm)
	if bad > 0 {
		return 1
	}
	return 0
}

// t3Lines is the text of a marker, one field to a line: each argument and its newline, in order.
func t3Lines(fields ...string) []byte {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// t5lGoOwns: the plan itself creates p, or a directory above it.
func t5lGoOwns(owned map[string]bool, p string) bool {
	for q := p; q != "/" && q != "." && q != ""; q = filepath.Dir(q) {
		if owned[q] {
			return true
		}
	}
	return false
}
