// Package mux decides which multiplexer a foreman process is running under:
// the herdr TUI (`herdr`) or the herdr app's herdr-core terminals (`hc`).
//
// The environment is not proof. An app terminal inherits the environment of
// the process that launched the app (measured 2026-11-08: a terminal carried
// HERDR_ENV=1 and its launcher's HERDR_PANE_ID), and a probe wrote tokens onto
// a TUI pane that was not its own. So when both sets of variables are present,
// the nearest herdr/herdr-core ancestor in the process tree decides.
package mux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	Herdr = "herdr"
	HC    = "hc"
)

// AncestorName is the nearest multiplexer ancestor's process name; a variable
// so tests can supply a tree without spawning one.
var AncestorName = nearestName

// Backend is the multiplexer this process writes to.
//
//   - no HC_TERMINAL_ID: the herdr TUI, today's path, untouched;
//   - HC_TERMINAL_ID without HERDR_ENV=1: herdr-core;
//   - both: the nearest multiplexer ancestor decides, because HERDR_ENV is
//     inherited, not measured (P12). An unknown tree is read as herdr-core:
//     never write onto a HERDR_PANE_ID that may belong to somebody else.
func Backend(getenv func(string) string) string {
	if getenv("HC_TERMINAL_ID") == "" {
		return Herdr
	}
	if getenv("HERDR_ENV") != "1" {
		return HC
	}
	switch AncestorName() {
	case "herdr":
		return Herdr
	default:
		return HC
	}
}

// LabelName is the hc label key of a herdr token name. herdr keys are already
// free-form; a token that does not carry the hw_ namespace gets it, so the
// same fact has one key on both backends (turn_state -> hw_turn_state).
func LabelName(token string) string {
	if strings.HasPrefix(token, "hw_") {
		return token
	}
	return "hw_" + token
}

// nearestName walks the parent chain for the nearest process named herdr or
// herdr-core. P14 measured the chains: a TUI pane is `zsh -> claude -> zsh ->
// herdr -> herdr`, an hc terminal `sh -> herdr-core -> herdr-core`.
func nearestName() string {
	pid := os.Getppid()
	for pid > 1 {
		ppid, comm, ok := processOf(pid)
		if !ok {
			return ""
		}
		switch filepath.Base(strings.TrimSpace(comm)) {
		case "herdr", "herdr-core":
			return filepath.Base(strings.TrimSpace(comm))
		}
		pid = ppid
	}
	return ""
}

// processOf is `ps -o ppid=,comm= -p <pid>`.
func processOf(pid int) (int, string, bool) {
	out, err := exec.Command("ps", "-o", "ppid=", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, "", false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", false
	}
	return ppid, strings.Join(fields[1:], " "), true
}

// IsHCAddress says whether addr names an hc terminal from a process running on
// herdr-core. herdr pane ids always carry a colon (`w3Z:p2`), hc addresses never
// do (`t7`, `machine/t7`).
func IsHCAddress(getenv func(string) string, addr string) bool {
	return addr != "" && !strings.Contains(addr, ":") && Backend(getenv) == HC
}
