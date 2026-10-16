package main

// The shell-shaped helpers `hw done` leans on (train T4): running a tool the way
// bash runs it, the clock bash reads (`date`, which the contract world fixes),
// the lane fields the T0 table loader does not keep, and the few jq programs
// lib/hw/done.sh pipes herdr's answers and the receipt through.

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// doneExit is the status of a finished command as `$?` reads it: 128+N for a signal.
func doneExit(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		if c := ee.ExitCode(); c >= 0 {
			return c
		}
		return 128 + 9
	}
	return 127
}

// doneTool runs a program with stderr discarded: its stdout and status.
func doneTool(dir string, env []string, name string, args ...string) (string, int) {
	r := extRun(extCall{Dir: dir, Env: env, Name: name, Args: args, Signal: true})
	return r.Out, r.RC
}

func doneCmd(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }

func doneLookPath(name string) (string, error) { return exec.LookPath(name) }

// doneCapture is `_capture <cmd>`: stdout and stderr in one stream (`$(… 2>&1)`), and the status.
func doneCapture(name string, args ...string) (string, int) {
	r := extRun(extCall{Name: name, Args: args, Merge: true, Signal: true})
	return bashSub(r.Out), r.RC
}

// doneGit is `git -C <dir> <args> 2>/dev/null`: stdout and status.
func doneGit(dir string, args ...string) (string, int) {
	return doneTool("", nil, "git", append([]string{"-C", dir}, args...)...)
}

// doneDate is `date <args>`: the clock bash asks (the contract world's `date` is fixed).
func doneDate(args ...string) (string, bool) {
	out, rc := doneTool("", nil, "date", args...)
	return bashSub(out), rc == 0
}

// doneNow is `$(date +%s)`.
func doneNow() int64 {
	if s, ok := doneDate("+%s"); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return n
		}
	}
	return time.Now().Unix()
}

// doneHeadTr is `printf '%s' "$x" | head -3 | tr '\n' ' '`: three lines, a newline kept after each but the last
// one printf left unterminated, each newline a blank.
func doneHeadTr(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		return strings.Join(lines[:n], " ") + " "
	}
	return strings.Join(lines, " ")
}

// doneAllDigits is `case $x in ""|*[!0-9]*) no ;; esac`.
func doneAllDigits(s string) bool {
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

// doneAtoi is a digit string as `[ -gt ]` and `$(( ))` read it in decimal; anything else is 0.
func doneAtoi(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// doneDigits is `tr -dc '0-9'`.
func doneDigits(s string) string { return digitsOnly(s) }

// doneTrimSpace is sed's s/^[[:space:]]*//; s/[[:space:]]*$//.
func doneTrimSpace(s string) string { return strings.Trim(s, " \t\n\v\f\r") }

// doneBasename is `basename`: trailing slashes do not count.
func doneBasename(p string) string {
	q := strings.TrimRight(p, "/")
	if q == "" {
		if p == "" {
			return ""
		}
		return "/"
	}
	if i := strings.LastIndexByte(q, '/'); i >= 0 {
		return q[i+1:]
	}
	return q
}

// doneDirname is `dirname`.
func doneDirname(p string) string {
	q := strings.TrimRight(p, "/")
	if q == "" {
		if strings.HasPrefix(p, "/") {
			return "/"
		}
		return "."
	}
	i := strings.LastIndexByte(q, '/')
	if i < 0 {
		return "."
	}
	d := strings.TrimRight(q[:i], "/")
	if d == "" {
		return "/"
	}
	return d
}

// donePhysical is `cd "$d" && pwd -P`.
func donePhysical(d string) (string, bool) {
	if !isDir(d) {
		return "", false
	}
	cmd := exec.Command("pwd", "-P")
	cmd.Dir = d
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return bashSub(string(out)), true
}

// doneFirstKey is `sed -n "s/^key=//p" file | head -1`.
func doneFirstKey(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, key+"=") {
			return l[len(key)+1:]
		}
	}
	return ""
}

// doneReadTrim is `$(cat file)`.
func doneReadTrim(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return bashSub(string(b)), true
}

// ── the lane fields T0's loader does not keep ───────────────────────────────

type doneLaneExtra struct {
	operator string
	f        map[string]map[string]string // lane -> field -> the value lane_config_load emits
}

// doneLoadExtras reads, from the table already loaded and checked, the fields `hw done` asks lane_get for that
// lanes.go does not keep: repoless, suite_lock, brain_guard, reap.copies / copies_from, and the operator.
func doneLoadExtras(file string) *doneLaneExtra {
	x := &doneLaneExtra{operator: "the operator", f: map[string]map[string]string{}}
	data, err := os.ReadFile(file)
	if err != nil {
		return x
	}
	vals, _ := parseStream(data)
	if len(vals) == 0 || vals[0].kind != vObj {
		return x
	}
	root := vals[0]
	if op := field(root, "operator"); op != nil {
		x.operator = op.tostring()
	}
	lanes := field(root, "lanes")
	if lanes == nil || lanes.kind != vObj {
		return x
	}
	for _, m := range lanes.obj {
		v := m.v
		f := map[string]string{}
		if r := field(v, "repoless"); r != nil && r.kind == vBool && r.b {
			f["repoless"] = "1"
		}
		f["suite_lock"] = orDefault(field(v, "suite_lock"), "")
		f["brain_guard"] = "1"
		if g := field(v, "brain_guard"); g != nil && g.kind == vBool && !g.b {
			f["brain_guard"] = "0"
		}
		if r := field(v, "reap"); r != nil {
			f["reap_copies"] = joinStrs(field(r, "copies"), "\n")
			f["reap_copies_from"] = orDefault(field(r, "copies_from"), "")
		}
		x.f[m.k] = f
	}
	return x
}

func (x *doneLaneExtra) get(lane, key string) string {
	if f, ok := x.f[lane]; ok {
		return f[key]
	}
	return ""
}

// ── jq, as lib/hw/done.sh pipes text through it ─────────────────────────────

// doneJqPath walks .a.b.c of one value as jq does: ok false is an error (indexing what is not an object).
func doneJqPath(v *jval, path ...string) (*jval, bool) {
	cur := v
	for _, p := range path {
		var ok bool
		if cur, ok = cur.get(p); !ok {
			return nil, false
		}
	}
	return cur, true
}

// doneJqE is `printf '%s' "$text" | jq -e '<path>' >/dev/null`: the status of the last output, as a success.
func doneJqE(text string, path ...string) bool {
	vals, perr := parseStream([]byte(text))
	st := 4
	for _, v := range vals {
		cur, ok := doneJqPath(v, path...)
		if !ok {
			st = 5
			continue
		}
		if cur.truthy() {
			st = 0
		} else {
			st = 1
		}
	}
	if perr != nil {
		return false
	}
	return st == 0
}

// doneJqRawEmpty is `jq -r '<path> // empty'` as `$( )` keeps it.
func doneJqRawEmpty(text string, path ...string) string { return jqRawOr(text, "", path...) }

// doneJqRawOr is `jq -r '<path> // "<def>"'`: per input value, the default where the path is null or false.
func doneJqRawOr(text, def string, path ...string) string {
	vals, _ := parseStream([]byte(text))
	var outs []string
	for _, v := range vals {
		cur, ok := doneJqPath(v, path...)
		if !ok {
			continue
		}
		if !cur.truthy() {
			outs = append(outs, def)
			continue
		}
		outs = append(outs, cur.raw())
	}
	return bashSub(strings.Join(outs, "\n"))
}

// doneReceiptLast is `jq -r 'select(.key=="<key>") | .value' <receipt> | tail -1`: the newest value's last line.
// rc is jq's status: 2 for a file it cannot read or a stream that stops parsing, 5 when a value could not be
// indexed, 0 otherwise (the pipeline status under pipefail, for the one caller that does not `|| true` it).
func doneReceiptLast(receipt, key string) (string, int) {
	data, err := os.ReadFile(receipt)
	if err != nil {
		return "", 2
	}
	vals, perr := parseStream(data)
	rc := 0
	var outs []string
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok {
			rc = 5
			continue
		}
		if k.kind == vStr && k.s == key {
			val, ok := v.get("value")
			if !ok {
				rc = 5
				continue
			}
			outs = append(outs, val.raw())
		}
	}
	if perr != nil {
		rc = 2
	}
	return bashSub(lastLine(outs)), rc
}

// doneJqSelectFirst is
//
//	jq -r --arg l "$label" '.result.<array>[] | select(.label == $l) | .<field>' | head -1
func doneJqSelectFirst(text, array, label, fieldName string) string {
	vals, _ := parseStream([]byte(text))
	var outs []string
	for _, v := range vals {
		arr, ok := doneJqPath(v, "result", array)
		if !ok || (arr.kind != vArr && arr.kind != vObj) {
			continue // jq: cannot iterate — this value ends in an error
		}
		elems := arr.arr
		if arr.kind == vObj {
			elems = nil
			for _, m := range arr.obj {
				elems = append(elems, m.v)
			}
		}
		for _, e := range elems {
			l, ok := e.get("label")
			if !ok {
				break
			}
			if l.kind == vStr && l.s == label {
				f, ok := e.get(fieldName)
				if !ok {
					break
				}
				outs = append(outs, f.raw())
			}
		}
	}
	if len(outs) == 0 {
		return ""
	}
	first := strings.Join(outs, "\n")
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	return first
}

// doneListHas is the probe of _herdr_object_exists: 2 when the list is not an answer, 1 when it lacks the id.
func doneListHas(text, array, fieldName, id string) int {
	vals, perr := parseStream([]byte(text))
	// jq -e '.result[$a] | type == "array"'
	isArr := 4
	for _, v := range vals {
		cur, ok := doneJqPath(v, "result", array)
		if !ok {
			isArr = 5
			continue
		}
		if cur.kind == vArr {
			isArr = 0
		} else {
			isArr = 1
		}
	}
	if perr != nil || isArr != 0 {
		return 2
	}
	// jq -e 'any(.result[$a][]; .[$f] == $id)'
	st := 4
	for _, v := range vals {
		cur, ok := doneJqPath(v, "result", array)
		if !ok || cur.kind != vArr {
			st = 5
			continue
		}
		found, bad := false, false
		for _, e := range cur.arr {
			x, ok := e.get(fieldName)
			if !ok {
				bad = true
				break
			}
			if x.kind == vStr && x.s == id {
				found = true
				break
			}
		}
		switch {
		case found:
			st = 0
		case bad:
			st = 5
		default:
			st = 1
		}
	}
	if st == 0 {
		return 0
	}
	return 1
}

// doneLinesOf is `while IFS= read -r line` over a file: every newline-terminated line (a last line without
// one is not read).
func doneLinesOf(data string) []string {
	var out []string
	for {
		i := strings.IndexByte(data, '\n')
		if i < 0 {
			return out
		}
		out = append(out, data[:i])
		data = data[i+1:]
	}
}

var doneReNonSpace = regexp.MustCompile(`\S`)
