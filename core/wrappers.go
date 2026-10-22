package main

// The small runtime wrappers of bin/, as verbs of hw-core: browser-close, brain-archive, secret-unlock, opencode-auto and
// codex-channel-session-hook. Each is a port of the bash it names — same stdout, stderr, exit status and on-disk effects —
// and each bash file stays beside it as the rollback: bin/wrapper-go.sh, sourced by the five, execs
// `hw-core <tool> "$@"` when HW_IMPL_<TOOL> (else HW_IMPL, else the switch file's `<tool>=` line) says `go`. The default is bash.
//
// The shim hands over `HW_WRAPPER_ARGV0` (the bash's own $0): the bash resolves the directory it lives in, and the files
// beside it (hw-secret, herdr-rpc, the brain root), from where it was started, so the Go does the same from that path.
//
// hw-reconcile is NOT here: nothing on the runtime path calls it (an operator's detector), so it stays bash.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// wenv is what a wrapper runs against: its streams, its environment and the path the bash was started as.
type wenv struct {
	stdout, stderr io.Writer
	stdin          io.Reader
	stdinFile      *os.File // the real stdin, for the terminal test; nil in a unit test
	getenv         func(string) string
	argv0          string
}

var wrapperTools = map[string]func(*wenv, []string) int{
	"browser-close":              bcRun,
	"brain-archive":              baRun,
	"secret-unlock":              suRun,
	"opencode-auto":              oaRun,
	"codex-channel-session-hook": chRun,
}

func isWrapperTool(name string) bool { _, ok := wrapperTools[name]; return ok }

func runWrapper(tool string, args []string, stdout, stderr io.Writer, getenv func(string) string, stdin io.Reader, stdinFile *os.File) int {
	argv0 := getenv("HW_WRAPPER_ARGV0")
	if argv0 == "" {
		argv0 = tool
	}
	return wrapperTools[tool](&wenv{stdout: stdout, stderr: stderr, stdin: stdin, stdinFile: stdinFile, getenv: getenv, argv0: argv0}, args)
}

func (w *wenv) errf(format string, a ...any) { fmt.Fprintf(w.stderr, format, a...) }
func (w *wenv) outf(format string, a ...any) { fmt.Fprintf(w.stdout, format, a...) }

// selfDir is the directory a bash tool resolves for itself by following the links of its own file
// (`while [ -L "$src" ]; … cd -P "$(dirname "$src")" && pwd`).
func (w *wenv) selfDir() string {
	src := w.argv0
	for i := 0; i < 40; i++ {
		fi, err := os.Lstat(src)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}
		dir := physDir(filepath.Dir(src))
		to, err := os.Readlink(src)
		if err != nil {
			break
		}
		if !filepath.IsAbs(to) {
			to = filepath.Join(dir, to)
		}
		src = to
	}
	return physDir(filepath.Dir(src))
}

// physDir is `cd -P "$d" && pwd`: the directory with its own links followed, or the path as given when it does not resolve.
func physDir(d string) string {
	if abs, err := filepath.Abs(d); err == nil {
		d = abs
	}
	if r, err := filepath.EvalSymlinks(d); err == nil {
		return r
	}
	return d
}

// ownDir is `cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd` with NO link of the file itself followed.
func (w *wenv) ownDir() string { return physDir(filepath.Dir(w.argv0)) }

// jqString is a string as `jq -c` prints it: UTF-8 as it is, the escapes of RFC 8259, and DEL as \u007f.
func jqString(s string) string {
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04x`, r)
		case r == utf8.RuneError:
			b.WriteString("�") // invalid UTF-8 comes out as U+FFFD, as jq does
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
