package invokers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// A HEADLESS AGENT NESTED IN A RUN IS NOT THAT RUN'S EXECUTOR.
//
// MEASURED 2026-11-09 (batch-1109-pm): an executor ran `claude -p … -- BRIEFWORD`
// as a probe. The child inherited HW_RUN, HW_INVOKER_PANE and HERDR_PANE_ID,
// took "BRIEFWORD" for its brief, and its ask-invoker reached the brainer and
// spent one of the REAL run's asks. refuseForeignPaneRun cannot see it: the
// HW_RUN is the same. What differs is the process tree — a real executor is an
// interactive agent, and a headless one (claude -p/--print, pi -p/--print,
// opencode run) among the ancestors is a probe or a subagent shell.

var interpreters = map[string]bool{"node": true, "bun": true, "bash": true, "sh": true, "python3": true, "env": true}

// headlessAgent reports whether one process's argv is a headless agent. Only
// argv[0] (or argv[1] behind an interpreter) names the program: a shell whose
// -c text merely MENTIONS `claude -p` is not one.
// shortcut: paths with spaces split wrongly in `ps` output; harmless, it only misses a match.
func headlessAgent(argv string) bool {
	f := strings.Fields(argv)
	if len(f) > 1 && interpreters[filepath.Base(f[0])] {
		f = f[1:]
	}
	if len(f) == 0 {
		return false
	}
	flags := f[1:]
	for i, a := range flags {
		if a == "--" || a == "--prompt" { // what follows is the brief (opencode's TUI takes it as --prompt): prose, not flags
			flags = flags[:i]
			break
		}
	}
	has := func(want ...string) bool {
		for _, a := range flags {
			for _, w := range want {
				if a == w {
					return true
				}
			}
		}
		return false
	}
	switch filepath.Base(f[0]) {
	case "claude", "pi":
		return has("-p", "--print")
	case "opencode":
		return has("run") // shortcut: a flag VALUE spelled `run` also matches; harmless, such a probe is nested anyway
	}
	return false
}

// headlessAncestor is the argv of the nearest headless agent above this process, "" when none.
func headlessAncestor() string {
	pid := os.Getppid()
	for depth := 0; pid > 1 && depth < 64; depth++ {
		b, err := exec.Command("/bin/ps", "-o", "ppid=,command=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return ""
		}
		line := strings.TrimSpace(string(b))
		ppid, cmd, _ := strings.Cut(line, " ")
		if headlessAgent(strings.TrimSpace(cmd)) {
			return strings.TrimSpace(cmd)
		}
		next, err := strconv.Atoi(ppid)
		if err != nil || next == pid {
			return ""
		}
		pid = next
	}
	return ""
}

// refuseNestedHeadless is checked before anything is locked, counted or sent.
func (c *Ctx) refuseNestedHeadless() error {
	if a := headlessAncestor(); a != "" {
		if len(a) > 80 {
			a = a[:80] + "…"
		}
		return die("REFUSING before anything was sent: an ancestor of this call is a headless agent (%s), so this is a probe or a subagent shell nested in a run, not that run's executor — its inherited HW_RUN/HW_INVOKER_PANE/HERDR_PANE_ID are the parent's. NOTHING was sent, no ask was spent and no marker written. Report from the real executor pane, or start the probe with `env -u HW_RUN -u HW_INVOKER_PANE -u HERDR_PANE_ID` and a stub brain (setup/CLAUDE.md).", a)
	}
	return nil
}
