package main

// Shadow's verify leg for a dispatch's LAUNCH (`hw-core shadow-verify --launch <plan>`, bin/hw `_core_shadow_launch`).
// The plan is core/t3.go's, with two more op kinds the launch records: `exec` {Path: program, Data: base64 of the
// argv joined by NUL, To: dir} and `symlink` {Path, To}. What is compared: every planned file write exists in
// bash's tree with the same bytes once the volatile fields are normalised, every planned symlink points where bash
// pointed it. The `exec` calls are NOT compared: bash's calls are not observable in production, so the verdict says
// nothing about them (the shim records `calls: not compared` beside the entry, never as a difference).

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	t5lReRun   = regexp.MustCompile(`[0-9]{8}-[0-9]{6}-[0-9]+`)
	t5lRePort  = regexp.MustCompile(`(--port[ =]|127\.0\.0\.1:|localhost:|PORT=)[0-9]+`)
	t5lRePane  = regexp.MustCompile(`\bshadow:[a-z]+[0-9]*\b|\bw[0-9A-Z]+:[pt][0-9A-Z]+\b|"(pane_id|tab_id|workspace_id)":"[^"]*"`)
	t5lRePid   = regexp.MustCompile(`\b(pid|PID|pgid)([=: ]+)[0-9]+`)
	t5lReStamp = regexp.MustCompile(`\.bak-[0-9]{8}-[0-9]{6}`)
	t5lReTmp   = regexp.MustCompile(`hw-shadow\.[A-Za-z0-9]+|(/private)?/var/folders/[^ "'/]+/[^ "'/]+/T`)
)

// t5lShadowNorm is t3ShadowNorm plus what two processes launching the same task differ in by construction: the
// run id (a timestamp and a pid), the port, the pane/tab/workspace ids herdr hands out, pids, temporary paths.
func t5lShadowNorm(b []byte) []byte {
	b = t3ShadowNorm(b)
	b = t5lReRun.ReplaceAll(b, []byte("@RUN@"))
	b = t5lReStamp.ReplaceAll(b, []byte(".bak-@STAMP@"))
	b = t5lRePort.ReplaceAll(b, []byte("${1}@PORT@"))
	b = t5lRePane.ReplaceAll(b, []byte("@PANE@"))
	b = t5lReBare.ReplaceAll(b, []byte("@PANE@"))
	b = t5lReCommit.ReplaceAll(b, []byte(`"brief_commit":"@SHA@"`))
	b = t5lReDeliv.ReplaceAll(b, []byte(`"delivered":"@D@"`))
	b = t5lRePid.ReplaceAll(b, []byte("${1}${2}@PID@"))
	b = t5lReTmp.ReplaceAll(b, []byte("@TMP@"))
	return b
}

// t5lVerifySymlinks checks each planned symlink against what bash left; returns the number of differences.
func t5lVerifySymlinks(out io.Writer, syms map[string]string, norm func([]byte) []byte) int {
	paths := make([]string, 0, len(syms))
	for p := range syms {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	bad := 0
	for _, p := range paths {
		got, err := os.Readlink(t5lResolve(p, norm))
		switch {
		case err != nil:
			fmt.Fprintf(out, "%s: Go linked it, bash did not\n", p)
			bad++
		case string(norm([]byte(got))) != string(norm([]byte(syms[p]))):
			fmt.Fprintf(out, "%s: link differs\n  go:   %q\n  bash: %q\n", p, syms[p], got)
			bad++
		}
	}
	return bad
}

// t5lVolatileFile is a planned file whose bytes are two processes' own: a port lock's pid. Its existence is checked, not its content.
func t5lVolatileFile(p string) bool {
	return strings.Contains(p, "/hw-ports/") && strings.HasSuffix(p, "/pid")
}

// t5lLiveCopy is a planned file that is a COPY OF A LIVE FILE: the backup _trust_dir makes of ~/.claude.json, which every
// running Claude Code session rewrites (numStartups, tipsHistory…). Go's leg and bash's read it seconds apart, so the bytes
// of two correct copies differ; that the backup exists, and is pruned the same way, is the contract (shadow-limpia-go).
func t5lLiveCopy(p string) bool {
	return strings.Contains(filepath.Base(p), ".claude.json.bak-")
}

// t5lResolve maps a planned path to the one bash left: a component that carries a run id or a backup stamp is two
// processes' own (Go's run id is not bash's), so it is found in its directory by what it is once normalised; the
// newest wins when bash left several.
func t5lResolve(p string, norm func([]byte) []byte) string {
	if _, err := os.Lstat(p); err == nil {
		return p
	}
	parts := strings.Split(p, "/")
	cur := "/"
	for i, c := range parts {
		if c == "" {
			continue
		}
		next := filepath.Join(cur, c)
		if _, err := os.Lstat(next); err == nil {
			cur = next
			continue
		}
		ents, err := os.ReadDir(cur)
		if err != nil {
			return p
		}
		want := string(norm([]byte(c)))
		var best string
		var bestT int64 = -1
		for _, e := range ents {
			if string(norm([]byte(e.Name()))) != want {
				continue
			}
			if fi, err := e.Info(); err == nil && fi.ModTime().UnixNano() > bestT {
				best, bestT = e.Name(), fi.ModTime().UnixNano()
			}
		}
		if best == "" {
			return p
		}
		cur = filepath.Join(cur, best)
		_ = i
	}
	return cur
}

var (
	t5lReBare   = regexp.MustCompile(`(?m)^[wtp][0-9A-Z]+$`) // herdr ids are base-36 upper case: w41, w7G, tX2, tZF, p1FV (measured 2026-10-12: `w7G:tZF`)
	t5lReCommit = regexp.MustCompile(`"brief_commit":"[^"]*"`)
	t5lReDeliv  = regexp.MustCompile(`"delivered":(true|false)`) // a read-only leg cannot see a delivery fail
)

var t5lReKey = regexp.MustCompile(`"key":"([^"]*)"`)

// t5lKeys is the receipt keys of a JSONL text, in order.
func t5lKeys(b []byte) []string {
	var out []string
	for _, m := range t5lReKey.FindAllSubmatch(b, -1) {
		out = append(out, string(m[1]))
	}
	return out
}

// t5lKeysSuffix: the keys Go appended are a subsequence of the keys bash's file ends with. (A read-only leg runs no child, so the
// receipts bash's `hw receipt` children append — worktree, branch, the session's vendor — are not in Go's tail.)
func t5lKeysSuffix(bash, tail []byte) bool {
	b, g := t5lKeys(bash), t5lKeys(tail)
	j := 0
	for _, k := range b {
		if j < len(g) && g[j] == k {
			j++
		}
	}
	return j == len(g)
}

// t5lEnvKeys is the variable names of a run's env file (KEY='value' lines), sorted.
func t5lEnvKeys(b []byte) []string {
	var out []string
	for _, ln := range strings.Split(string(b), "\n") {
		if ln == "" || ln[0] == '#' {
			continue
		}
		if i := strings.IndexByte(ln, '='); i > 0 {
			out = append(out, ln[:i])
		}
	}
	sort.Strings(out)
	return out
}

// t5lEnvKeysEqual: every variable Go wrote is in bash's file. (Bash may carry more: the review base is read from git in a worktree a
// read-only leg only planned.)
func t5lEnvKeysEqual(bash, goes []byte) bool {
	have := map[string]bool{}
	for _, k := range t5lEnvKeys(bash) {
		have[k] = true
	}
	for _, k := range t5lEnvKeys(goes) {
		if !have[k] {
			return false
		}
	}
	return true
}
