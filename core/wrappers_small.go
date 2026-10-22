package main

// secret-unlock, opencode-auto and codex-channel-session-hook: the three wrappers that do one thing and leave.
// The line numbers below are the `exec` / herdr-rpc lines of the bash files: a shell names the line of a failed exec
// (`bin/opencode-auto: line 39: exec: opencode: not found`) and the Go says the same. wrappers_parity_test.go runs both
// against a missing program, so a line that moves in the bash is a red here.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	suExecLine = 22 // bin/secret-unlock: the `exec …/hw-secret unlock "$@"`
	oaExecLine = 46 // bin/opencode-auto: the `exec opencode --auto …`
	chRPC1Line = 66 // bin/codex-channel-session-hook.sh: the pane.report_agent_session call
	chRPC2Line = 75 // … and the agent.get call
	oaUsage    = "Usage: opencode-auto [opencode options] --resume <session-id>\n"
)

// bashIsLegacy: the bash the wrappers run under (`#!/usr/bin/env bash`, the first one on PATH) is older than 4: macOS's /bin/bash 3.2. A
// bash 4+ (5.2.15 measured in the Linux container, 2026-10-19) answers differently where a program cannot be started: bashExecFail and
// bcSleep say how. Asked once, of that bash itself; with none to ask, the platform's: only macOS ships 3.2.
var bashIsLegacy = sync.OnceValue(func() bool {
	if out, err := exec.Command("bash", "-c", `printf %s "${BASH_VERSINFO[0]}"`).Output(); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			return n < 4
		}
	}
	return runtime.GOOS == "darwin"
})

// bashExecFail is what bash 3.2 prints, and exits with, when `exec <name>` cannot start the program: one line for a name looked up
// on PATH, and for a path the two lines it prints (the failed open, then the exec's own).
func (w *wenv) bashExecFail(line int, name string, err error) int {
	reason := "cannot execute"
	switch {
	case errors.Is(err, exec.ErrNotFound) || (errors.Is(err, os.ErrNotExist) && !strings.Contains(name, "/")):
		w.errf("%s: line %d: exec: %s: not found\n", w.argv0, line, name)
		return 127
	case errors.Is(err, os.ErrNotExist):
		if !bashIsLegacy() { // bash 4+: the one line of the failed open, and the 127 of "not found"
			w.errf("%s: line %d: %s: No such file or directory\n", w.argv0, line, name)
			return 127
		}
		reason = "No such file or directory"
	case errors.Is(err, os.ErrPermission):
		reason = "Permission denied"
	default:
		reason = err.Error()
	}
	w.errf("%s: line %d: %s: %s\n", w.argv0, line, name, reason)
	w.errf("%s: line %d: exec: %s: cannot execute: %s\n", w.argv0, line, name, reason)
	return 126
}

// ── secret-unlock ────────────────────────────────────────────────────────────

// suRun is `exec <dir>/hw-secret unlock "$@"`: the logic lives in hw-secret, the one owner of the session file.
func suRun(w *wenv, args []string) int {
	hs := w.selfDir() + "/hw-secret"
	argv := append([]string{hs, "unlock"}, args...) // MUTATION-ANCHOR: s6-su-args
	err := execReplace(hs, argv, os.Environ())
	return w.bashExecFail(suExecLine, hs, err)
}

// ── opencode-auto ────────────────────────────────────────────────────────────

// oaRun is opencode with permissions auto-approved and background subagents on; `--resume <id>` is `--session <id>`.
func oaRun(w *wenv, args []string) int {
	var out []string
	resumeSeen := false
	for i := 0; i < len(args); {
		if args[i] == "--resume" {
			if resumeSeen || len(args)-i < 2 || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") { // MUTATION-ANCHOR: s6-oa-resume
				w.errf("%s", oaUsage)
				return 2
			}
			out = append(out, "--session", args[i+1])
			resumeSeen = true
			i += 2
			continue
		}
		out = append(out, args[i])
		i++
	}
	env := append(os.Environ(), "OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true")
	path, err := exec.LookPath("opencode")
	if err != nil {
		return w.bashExecFail(oaExecLine, "opencode", err)
	}
	err = execReplace(path, append([]string{"opencode", "--auto"}, out...), env)
	return w.bashExecFail(oaExecLine, "opencode", err)
}

// ── codex-channel-session-hook ───────────────────────────────────────────────

// chFail is _hook_fail: the message on stderr, a line in the durable per-pane file, and exit 1.
func (w *wenv) chFail(msg string) int {
	w.errf("codex-channel-session-hook: %s\n", msg)
	if pane := w.getenv("HERDR_PANE_ID"); pane != "" {
		tmp := w.getenv("TMPDIR")
		if tmp == "" {
			tmp = "/tmp"
		}
		san := []byte(pane)
		for i, c := range san {
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-') {
				san[i] = '_'
			}
		}
		if f, err := os.OpenFile(tmp+"/hw-codex-session-"+string(san)+".err", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666); err == nil {
			io.WriteString(f, time.Now().UTC().Format("2006-01-02T15:04:05Z")+"\t"+msg+"\n")
			f.Close()
		}
	}
	return 1
}

// jqStrings is `jq -r '<path> // empty'` over EVERY JSON value of raw, as the shell holds the output afterwards (a command
// substitution: no NUL, no trailing newline). A value that is null, false or absent prints nothing, a document that is not
// an object on the way prints nothing (jq says so on stderr and goes on), and a document that does not parse ends the input.
func jqStrings(raw []byte, path ...string) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var lines []string
	for {
		var cur json.RawMessage
		if dec.Decode(&cur) != nil {
			break
		}
		if v, ok := jqPick(cur, path); ok {
			lines = append(lines, v)
		}
	}
	return trimNL(strings.ReplaceAll(strings.Join(lines, "\n"), "\x00", ""))
}

func jqPick(cur json.RawMessage, path []string) (string, bool) {
	for _, key := range path {
		var obj map[string]json.RawMessage
		if json.Unmarshal(cur, &obj) != nil {
			return "", false
		}
		v, ok := obj[key]
		if !ok {
			return "", false
		}
		cur = v
	}
	s := strings.TrimSpace(string(cur))
	switch {
	case s == "null" || s == "false":
		return "", false
	case strings.HasPrefix(s, `"`):
		var out string
		if json.Unmarshal(cur, &out) != nil {
			return "", false
		}
		return out, true
	case strings.HasPrefix(s, "{") || strings.HasPrefix(s, "["):
		var b bytes.Buffer
		json.Indent(&b, cur, "", "  ")
		return b.String(), true
	}
	return s, true // true, a number: as written
}

// headTwoJoined is `printf '%s' "$out" | head -2 | tr '\n' ' '`, command-substituted.
func headTwoJoined(out string) string {
	lines := strings.Split(out, "\n")
	if len(lines) >= 3 {
		return lines[0] + " " + lines[1] + " "
	}
	return strings.Join(lines, " ")
}

// chRPC is `herdr-rpc call <method> <params> 2>&1`, command-substituted: the output and whether it exited 0.
func (w *wenv) chRPC(dir string, line int, method, params string) (string, bool) {
	rpc := dir + "/herdr-rpc"
	cmd := exec.Command(rpc, "call", method, params)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) { // it never started: the shell's own words, in the captured stream
			msg := "No such file or directory"
			if errors.Is(err, os.ErrPermission) {
				msg = "Permission denied"
			}
			return w.argv0 + ": line " + strconv.Itoa(line) + ": " + rpc + ": " + msg, false
		}
	}
	return trimNL(strings.ReplaceAll(string(out), "\x00", "")), err == nil
}

// chRun reports Codex's thread id to herdr as soon as a user prompt is submitted, and reads the mapping back.
func chRun(w *wenv, args []string) int {
	if len(args) == 0 || args[0] != "session" || w.getenv("HERDR_ENV") != "1" || w.getenv("HERDR_PANE_ID") == "" || w.getenv("HERDR_SOCKET_PATH") == "" {
		return 0
	}
	pane := w.getenv("HERDR_PANE_ID")
	in, _ := io.ReadAll(io.LimitReader(w.stdin, 262144))
	session := jqStrings(in, "session_id")
	if session == "" {
		return 0
	}
	dir := w.ownDir()
	params := `{"pane_id":` + jqString(pane) + `,"source":"hw:codex-channel","agent":"codex","seq":` + strconv.FormatInt(time.Now().UnixNano(), 10) + `,"agent_session_id":` + jqString(session) + `}`
	rpcOut, ok := w.chRPC(dir, chRPC1Line, "pane.report_agent_session", params)
	if !ok {
		return w.chFail("pane.report_agent_session failed for pane " + pane + " session " + session + ": " + headTwoJoined(rpcOut))
	}
	readOut, ok := w.chRPC(dir, chRPC2Line, "agent.get", `{"target":`+jqString(pane)+`}`)
	if !ok {
		return w.chFail("published the session for pane " + pane + " but could not read it back: " + headTwoJoined(readOut))
	}
	landed := jqStrings([]byte(readOut), "agent", "agent_session", "value")
	if landed != session { // MUTATION-ANCHOR: s6-ch-readback
		shown := landed
		if shown == "" {
			shown = "none"
		}
		return w.chFail("pane " + pane + " reports session '" + shown + "', not the '" + session + "' just published — the mapping did not land")
	}
	return 0
}
