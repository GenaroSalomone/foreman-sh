package main

// The small readers hw revive needs and no earlier verb did: what `jq -r` / `jq -e` say about a herdr reply or
// a receipt, the way bash splits a word list, and the quoting of printf %q. Every top-level name starts with
// `revive` so that this package, merged with the other writers of train T4, has one owner per name.

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

// reviveJqPath is `.a.b.c` on one value: null indexes to null, an object to its member (null when absent),
// and anything else is a jq error (ok == false).
func reviveJqPath(v *jval, path ...string) (*jval, bool) {
	cur := v
	for _, p := range path {
		n, ok := cur.get(p)
		if !ok {
			return nil, false
		}
		cur = n
	}
	return cur, true
}

// revivePretty is jq's default (`-r` of an array or an object): two-space indentation.
func revivePretty(v *jval, depth int, sb *strings.Builder) {
	pad := strings.Repeat("  ", depth+1)
	switch v.kind {
	case vArr:
		if len(v.arr) == 0 {
			sb.WriteString("[]")
			return
		}
		sb.WriteString("[\n")
		for i, e := range v.arr {
			sb.WriteString(pad)
			revivePretty(e, depth+1, sb)
			if i < len(v.arr)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.Repeat("  ", depth) + "]")
	case vObj:
		if len(v.obj) == 0 {
			sb.WriteString("{}")
			return
		}
		sb.WriteString("{\n")
		for i, m := range v.obj {
			sb.WriteString(pad)
			writeJSONString(sb, m.k)
			sb.WriteString(": ")
			revivePretty(m.v, depth+1, sb)
			if i < len(v.obj)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.Repeat("  ", depth) + "}")
	default:
		v.write(sb)
	}
}

// reviveRaw is one output of `jq -r`.
func reviveRaw(v *jval) string {
	if v.kind == vStr {
		return v.s
	}
	var sb strings.Builder
	revivePretty(v, 0, &sb)
	return sb.String()
}

// reviveReceiptLast is
//
//	jq -r 'select(.key==K) | .value' FILE 2>/dev/null | tail -1
//
// and the status the pipeline has under pipefail: jq's own, which in 1.7 is the status of the LAST input it
// processed (5 when that input could not be indexed) and 5 again for a syntax error. A non-zero status ends bash
// (`set -e` on the assignment), silently, with that status: the caller does the same.
func reviveReceiptLast(path, key string) (string, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 2
	}
	vals, perr := parseStream(data)
	var outs []string
	rc := 0
	for _, v := range vals {
		k, ok := v.get("key")
		if !ok {
			rc = 5 // # MUTATION-ANCHOR: 863-G17
			continue
		}
		rc = 0
		if k.kind == vStr && k.s == key {
			val, _ := v.get("value")
			outs = append(outs, reviveRaw(val))
		}
	}
	if perr != nil {
		rc = 5 // # MUTATION-ANCHOR: 863-G18
	}
	return reviveLastLine(outs), rc
}

// reviveLastLine is `tail -1` of the newline-terminated outputs, then `$(...)`'s strip of the newlines.
func reviveLastLine(outs []string) string {
	if len(outs) == 0 {
		return ""
	}
	text := strings.Join(outs, "\n")
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		text = text[i+1:]
	}
	return bashSub(text)
}

// reviveFirstTruthy is `jq -r 'A // B // … // empty'`: for each document the first path that yields something
// other than null and false, as jq 1.7 prints it. A path that cannot be indexed is an error that `//` does NOT
// swallow (measured on jq 1.7.1): that document prints nothing.
func reviveFirstTruthy(text string, paths ...[]string) string {
	vals, _ := parseStream([]byte(text))
	var outs []string
docs:
	for _, v := range vals {
		for _, p := range paths {
			cur, ok := reviveJqPath(v, p...)
			if !ok {
				continue docs
			}
			if cur.truthy() {
				outs = append(outs, reviveRaw(cur))
				continue docs
			}
		}
	}
	return bashSub(strings.Join(outs, "\n"))
}

// reviveJqTrue is `jq -e '<expr>'` as a condition, for an expression that yields a boolean: the last document
// answers, an error or a syntax error is a failure.
func reviveJqTrue(text string, expr func(*jval) (bool, bool)) bool {
	vals, perr := parseStream([]byte(text))
	if perr != nil || len(vals) == 0 {
		return false
	}
	res, ok := expr(vals[len(vals)-1])
	return ok && res
}

// reviveStrEq is `.p == "s"`: a member that is that string.
func reviveStrEq(v *jval, s string) bool { return v != nil && v.kind == vStr && v.s == s }

// reviveHead is `head -N` of a text without its final newline, as `$(printf '%s' "$x" | head -N)` leaves it.
func reviveHead(text string, n int) string {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return bashSub(strings.Join(lines, ""))
}

// reviveSplitWords is what an unquoted `$var` does to its value: split on blanks, tabs and newlines.
func reviveSplitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// reviveHasWord is `case " $s " in *" $w "*)`.
func reviveHasWord(s, w string) bool { return strings.Contains(" "+s+" ", " "+w+" ") }

// reviveQuote is bash's `printf %q`.
func reviveQuote(s string) string {
	if s == "" {
		return "''"
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte("_/.,:+=@%-", c) >= 0, c >= 0x80:
		case c == '~' && i > 0:
		default:
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// reviveRun runs a program the way bash does from a `$(...)`: stdout is what it printed, the status its own,
// stderr passed to the caller's.
func (a *app) reviveRun(stderr bool, name string, args ...string) (string, int) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if stderr {
		cmd.Stderr = os.Stderr
	}
	return out.String(), exitStatus(cmd.Run())
}

// reviveBinDir is HW_BIN_DIR: the shim hands it down as HW_CORE_BIN_DIR.
func (a *app) reviveBinDir() string {
	if d := a.getenv("HW_CORE_BIN_DIR"); d != "" {
		return d
	}
	return a.c.brain + "/bin"
}

// reviveBrainRoot is bin/hw's _brain_root: the checkout the toolchain runs from (the shim hands it down).
func (a *app) reviveBrainRoot() string {
	if d := a.getenv("HW_CORE_ROOT"); d != "" {
		return d
	}
	return a.c.brain
}
