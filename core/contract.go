package main

// The run directory, as the files hw writes into it and the programs that read
// them agree on it (setup/contracts/hw.md §1). The library T1 and later trains
// build on; in T0 receipt, log and the snapshot use the discovery, the markers
// and the receipt line, and `ParseEnv` is held to bin/runenv by golden fixtures.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// RunIDPattern is the name of a run directory: YYYYMMDD-HHMMSS-PID
// (bin/hw:7694, the fd pattern every discovery uses).
var RunIDPattern = regexp.MustCompile(`^2[0-9]{7}-[0-9]{6}-[0-9]+$`)

// Markers are the zero-byte files whose presence is a fact about a run
// (bin/hw, bin/done-invoker, bin/invoker-common.sh).
var Markers = []string{"done", "reporting", "reopened", "blocked-waiting", "blocked-resumed", "delivery-uncertain"}

// RunIDs lists the run directories under a task's .hw directory, newest name
// first, as `fd -H -t d -d 1 <RunIDPattern> .hw | sort -r` does.
func RunIDs(hwDir string) []string {
	ents, err := os.ReadDir(hwDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !RunIDPattern.MatchString(e.Name()) {
			continue
		}
		if st, err := os.Stat(hwDir + "/" + e.Name()); err == nil && st.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

// HasMarker is `[ -f <rundir>/<marker> ]`.
func HasMarker(runDir, marker string) bool { return isFile(runDir + "/" + marker) }

// ── the env file: KEY='value', one per line (bin/runenv is the one reader) ───

// KV is one entry of an env file, in the order the key first appeared.
type KV struct{ Key, Value string }

// ErrEnvUnusable is runenv's exit 2: the file exists but is outside the
// grammar or cannot be decoded. It is never "absent".
var ErrEnvUnusable = errors.New("env file unusable")

var (
	reEnvLine = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)='(.*)'$`)
	reEnvBare = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
)

// pyIsSpace is Python's str.isspace for one rune.
func pyIsSpace(r rune) bool {
	switch {
	case r >= 0x09 && r <= 0x0d, r >= 0x1c && r <= 0x20, r == 0x85, r == 0xa0, r == 0x1680,
		r >= 0x2000 && r <= 0x200a, r == 0x2028, r == 0x2029, r == 0x202f, r == 0x205f, r == 0x3000:
		return true
	}
	return false
}

func pyStrip(s string) string  { return strings.TrimFunc(s, pyIsSpace) }
func pyLStrip(s string) string { return strings.TrimLeftFunc(s, pyIsSpace) }

// pyLines splits the way Python's text mode reads a file: \n, \r\n and a lone
// \r all end a line (universal newlines), and the terminator is dropped.
func pyLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			out = append(out, s[start:i])
			start = i + 1
		case '\r':
			out = append(out, s[start:i])
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// ParseEnv reads an env file the way bin/runenv does. Strict by default: a line
// outside the grammar is ErrEnvUnusable. lenient skips such a line; bare also
// takes a bare KEY=value, trimmed. A key repeated keeps its first position and
// its last value, as sourcing the file would.
func ParseEnv(data []byte, lenient, bare bool) ([]KV, error) {
	var out []KV
	set := func(k, v string) {
		for i := range out {
			if out[i].Key == k {
				out[i].Value = v
				return
			}
		}
		out = append(out, KV{k, v})
	}
	if bare {
		lenient = true
	}
	for n, line := range pyLines(string(data)) {
		if pyStrip(line) == "" || strings.HasPrefix(pyLStrip(line), "#") {
			continue
		}
		if m := reEnvLine.FindStringSubmatch(line); m != nil {
			set(m[1], strings.ReplaceAll(m[2], `'\''`, `'`))
			continue
		}
		if bare {
			if b := reEnvBare.FindStringSubmatch(line); b != nil {
				set(b[1], pyStrip(b[2]))
			}
			continue
		}
		if lenient {
			continue
		}
		return nil, fmt.Errorf("%w: line %d is not KEY='value': %q", ErrEnvUnusable, n+1, line)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: not UTF-8", ErrEnvUnusable)
	}
	return out, nil
}

// DumpNul is `runenv dump --nul`: KEY=value NUL records.
func DumpNul(kvs []KV) []byte {
	var b bytes.Buffer
	for _, kv := range kvs {
		b.WriteString(kv.Key + "=" + kv.Value + "\x00")
	}
	return b.Bytes()
}

// EncodeEnv writes the grammar: one KEY='value' per line in key order, an inner
// ' as '\” , an empty value never written, and a value with a newline, or a
// name outside [A-Za-z0-9_], skipped (the writer's own rules, bin/hw:2676-2760).
func EncodeEnv(values map[string]string) []byte {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	for _, k := range keys {
		v := values[k]
		if v == "" || strings.Contains(v, "\n") || !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(k) {
			continue
		}
		b.WriteString(k + "='" + strings.ReplaceAll(v, "'", `'\''`) + "'\n")
	}
	return b.Bytes()
}
