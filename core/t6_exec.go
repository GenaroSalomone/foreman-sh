package main

// `hw cockpit-state`, `hw secret` and `--version` (bin/hw:749, :13470, :13479). The first two are an `exec` of a
// program on HW_BIN_DIR: hw-core runs it as its child with the same stdio and answers its status; the programs
// themselves (bin/cockpit-state, bin/hw-secret) are not ported.

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdExecHelper is `exec "$HW_BIN_DIR/<prog>" "$@"`.
func (a *app) cmdExecHelper(prog string) int {
	path := filepath.Join(a.binDir(), prog)
	if _, err := os.Stat(path); err != nil { // bash's exec: "<path>: No such file or directory", status 127
		a.errf("%s/hw: line 0: exec: %s: not found\n", a.binDir(), path)
		return 127
	}
	cmd := exec.Command(path, a.args...)
	cmd.Stdin = a.stdin
	cmd.Stdout = a.out
	cmd.Stderr = a.err
	if f, ok := a.stdin.(*os.File); ok {
		cmd.Stdin = f
	}
	return exitStatus(cmd.Run())
}

// cmdVersion is `printf 'hw %s\n' "$(cli_version "$BRAIN")"`.
func cmdVersion(t *term, brain string) int {
	t.printf("hw %s\n", cliVersion(brain))
	return 0
}

// cliVersion is cli_version (bin/project-spaces.sh): the install record's version, else its commit's first seven
// characters, else the exact tag or the short HEAD of the root, else "unknown".
func cliVersion(root string) string {
	v := ""
	if data, err := os.ReadFile(filepath.Join(root, ".brain-install.json")); err == nil {
		var m map[string]any
		if json.Unmarshal(data, &m) == nil {
			if s, ok := m["version"].(string); ok && s != "" { // jq: `.version // …`: null and false fall through
				v = s
			} else if c, ok := m["commit"].(string); ok {
				v = runeSlice(c, 7)
			}
		}
	}
	if v == "" || v == "null" {
		v = ""
		if r := extRun(extCall{Name: "git", Args: []string{"-C", root, "describe", "--tags", "--exact-match"}}); r.RC == 0 {
			v = strings.TrimRight(r.Out, "\n")
		}
		if v == "" {
			if r := extRun(extCall{Name: "git", Args: []string{"-C", root, "rev-parse", "--short", "HEAD"}}); r.RC == 0 {
				v = strings.TrimRight(r.Out, "\n")
			}
		}
	}
	if v == "" {
		return "unknown"
	}
	return v
}

func runeSlice(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

var _ = io.Discard
