package main

// browser-close in Go (bin/browser-close): close an agent-browser session and PROVE it, or say what is still open and why.
// Exit 0 verified closed, 1 a session outlived the settle window (or the observation failed), 3 Chrome still holds the profile.
// The reasons it exists are the bash header's, which `--help` prints from wrappers_browser_close_help.txt.

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed wrappers_browser_close_help.txt
var bcHelp string

const (
	bcSleepLine = 157 // bin/browser-close: the `"${BROWSER_CLOSE_SLEEP:-/bin/sleep}" "$((SETTLE_MS / 1000)).…"` line
	bcKillLine  = 205 // … and the `kill -0 "$pid"` line: bash names them when it fails
)

// bcNamespacePath is agent-browser's sanitize_session_component as the bash reproduces it: ASCII lowercased, the first
// separator of each '-'/'_' run kept, everything else a '-', no leading or trailing separator.
func bcNamespacePath(ns string) string {
	var p []byte
	lastSep := false
	for i := 0; i < len(ns); i++ {
		c := ns[i]
		switch {
		case c >= 'A' && c <= 'Z':
			p = append(p, c+'a'-'A')
			lastSep = false
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			p = append(p, c)
			lastSep = false
		case c == '-' || c == '_':
			if len(p) > 0 && !lastSep {
				p = append(p, c)
			}
			lastSep = true
		default:
			if len(p) > 0 && !lastSep {
				p = append(p, '-')
			}
			lastSep = true
		}
	}
	if n := len(p); n > 0 && (p[n-1] == '-' || p[n-1] == '_') {
		p = p[:n-1]
	}
	return string(p)
}

// bcSettleMillis is `$((SETTLE_MS))` in bash: a leading 0 is octal (and 8 or 9 then an error), and the arithmetic wraps at 64 bits.
func bcSettleMillis(s string) (int64, bool) {
	base := uint64(10)
	if len(s) > 1 && s[0] == '0' {
		base = 8
	}
	var v uint64
	for i := 0; i < len(s); i++ {
		d := uint64(s[i] - '0')
		if d >= base {
			return 0, false
		}
		v = v*base + d
	}
	return int64(v), true
}

func bcRun(w *wenv, args []string) int {
	die := func(m string) int { w.errf("browser-close: %s\n", m); return 1 }
	note := func(m string) { w.errf("browser-close: %s\n", m) }

	session := ""
	profile := w.getenv("HW_BROWSER_PROFILE")
	settle := "1500"
	for len(args) > 0 {
		switch args[0] {
		case "--session":
			if len(args) < 2 {
				return die("--session needs a name")
			}
			session, args = args[1], args[2:]
		case "--profile":
			if len(args) < 2 {
				return die("--profile needs a name or path")
			}
			profile, args = args[1], args[2:]
		case "--settle-ms":
			if len(args) < 2 {
				return die("--settle-ms needs a number")
			}
			settle, args = args[1], args[2:]
		case "-h", "--help":
			w.outf("%s", bcHelp)
			return 0
		default:
			return die("unknown option: " + args[0] + " (takes --session, --profile, --settle-ms)")
		}
	}
	if settle == "" || strings.Trim(settle, "0123456789") != "" {
		return die("--settle-ms must be a whole number of milliseconds (got: " + settle + ")")
	}

	ab := w.getenv("BROWSER_CLOSE_AGENT_BROWSER")
	if ab == "" {
		ab = "agent-browser"
	}

	// An unset namespace makes the CLI operate on its default namespace: refusing is safer than closing the wrong daemon.
	namespace := w.getenv("AGENT_BROWSER_NAMESPACE")
	if namespace == "" {
		return die("no AGENT_BROWSER_NAMESPACE — refusing to close the default namespace and report success")
	}
	for i := 0; i < len(namespace); i++ { // grep -q '[^ -~]', line by line: a newline is not a character of any line
		if c := namespace[i]; c != '\n' && (c < ' ' || c > '~') {
			return die("AGENT_BROWSER_NAMESPACE contains non-ASCII characters whose lowercase sidecar path cannot be reproduced safely")
		}
	}
	if session != "" && strings.Trim(session, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return die("--session must contain only ASCII letters, numbers, hyphens, or underscores")
	}

	// agent-browser 0.31.2's get_socket_dir: a non-empty override, then a non-empty XDG runtime root, then HOME.
	var sidecarBase string
	switch {
	case w.getenv("AGENT_BROWSER_SOCKET_DIR") != "":
		sidecarBase = w.getenv("AGENT_BROWSER_SOCKET_DIR")
	case w.getenv("XDG_RUNTIME_DIR") != "":
		sidecarBase = w.getenv("XDG_RUNTIME_DIR") + "/agent-browser"
	case w.getenv("HOME") != "":
		sidecarBase = w.getenv("HOME") + "/.agent-browser"
	default:
		return die("cannot establish agent-browser's sidecar root: AGENT_BROWSER_SOCKET_DIR, XDG_RUNTIME_DIR, and HOME are all empty")
	}

	nsPath := bcNamespacePath(namespace)
	if nsPath == "" {
		return die("AGENT_BROWSER_NAMESPACE sanitizes to an empty namespace; refusing the default sidecar root")
	}
	sidecarRoot := sidecarBase + "/namespaces/" + nsPath + "/run"
	if profile != "" && !strings.HasPrefix(profile, "/") && w.getenv("HOME") == "" {
		return die("a relative --profile needs HOME to resolve agent-browser's shared profile path")
	}
	abPath, err := exec.LookPath(ab)
	if err != nil {
		return die("no agent-browser in PATH, so there is nothing to close and nothing to verify")
	}

	// ── 1. ask it to close ── the output is recorded, never believed
	var closeArgs []string
	if session != "" {
		closeArgs = []string{"--session", session, "close"}
	} else {
		closeArgs = []string{"close", "--all"}
	}
	closeOut := bcCapture(abPath, closeArgs...)

	// ── 2. let the registration daemon die ──
	// An arithmetic error in the argument skips that one command and the script goes on (bash 3.2, set -e or not).
	if ms, ok := bcSettleMillis(settle); !ok {
		w.errf("%s: line %d: %s: value too great for base (error token is \"%s\")\n", w.argv0, bcSleepLine, settle, settle)
	} else if rc := w.bcSleep(strconv.FormatInt(ms/1000, 10) + "." + bcPad3(ms%1000)); rc != 0 {
		return rc
	}

	// ── 3. read the signals that carry the fact ── before `session list`: list/info prune stale sidecars
	liveSidecars, inspectionError := "", ""
	var pidFiles []string
	if bcIsDir(sidecarRoot) {
		if !accessOK(sidecarRoot, 4) || !accessOK(sidecarRoot, 1) {
			inspectionError = sidecarRoot + " is not readable and searchable"
		} else if session != "" {
			pidFiles = []string{sidecarRoot + "/" + session + ".pid"}
		} else {
			ents, _ := os.ReadDir(sidecarRoot)
			for _, e := range ents {
				if n := e.Name(); strings.HasSuffix(n, ".pid") && !strings.HasPrefix(n, ".") {
					pidFiles = append(pidFiles, sidecarRoot+"/"+n)
				}
			}
			pidFiles = sortLocale(pidFiles, false)
		}
	} else {
		// Absence is evidence only when an existing ancestor is inspectable.
		ancestor := sidecarRoot
		for !bcExists(ancestor) && ancestor != "/" && ancestor != "." {
			if i := strings.LastIndex(ancestor, "/"); i >= 0 {
				ancestor = ancestor[:i]
				if ancestor == "" {
					ancestor = "/"
				}
			} else {
				ancestor = "."
			}
		}
		if !bcIsDir(ancestor) || !accessOK(ancestor, 4) || !accessOK(ancestor, 1) {
			inspectionError = "cannot verify that absent sidecar root " + sidecarRoot + " is reachable from " + ancestor
		}
	}
	for _, pf := range pidFiles {
		if fi, err := os.Stat(pf); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if filepath.Base(pf) == "dashboard.pid" {
			continue
		}
		if !accessOK(pf, 4) {
			inspectionError = "cannot read sidecar PID file " + pf
			continue
		}
		raw, err := os.ReadFile(pf)
		if err != nil {
			inspectionError = "could not inspect sidecar PID file " + pf
			continue
		}
		pid := trimNL(string(raw))
		if pid == "" || strings.Trim(pid, "0123456789") != "" {
			continue
		}
		n, perr := strconv.Atoi(pid)
		if perr != nil {
			inspectionError = "could not establish liveness for pid " + pid + " from " + pf + ": " + w.argv0 + ": line " + strconv.Itoa(bcKillLine) + ": kill: " + pid + ": arguments must be process or job IDs"
			continue
		}
		alive, why := kill0(n)
		switch {
		case alive:
			name := strings.TrimSuffix(filepath.Base(pf), ".pid")
			if liveSidecars != "" {
				liveSidecars += ", "
			}
			liveSidecars += name + " (pid " + pid + ")"
		case why == "No such process":
		default:
			inspectionError = "could not establish liveness for pid " + pid + " from " + pf + ": " + w.argv0 + ": line " + strconv.Itoa(bcKillLine) + ": kill: (" + pid + ") - " + why
		}
	}

	listOut := bcCapture(abPath, "session", "list")
	stillListed := ""
	if !strings.Contains(listOut, "No active sessions") {
		// sed '1d' | tr -d ' →' | tr '\n' ' ' | sed 's/ *$//'
		rest := strings.Split(listOut, "\n")[1:]
		for i, l := range rest {
			rest[i] = strings.NewReplacer(" ", "", "→", "").Replace(l)
		}
		stillListed = strings.TrimRight(strings.Join(rest, " "), " ")
	}

	// The profile's own Chrome, matched on user-data-dir — not on "Chrome for Testing" alone.
	chromePids, udd := "", ""
	if profile != "" {
		if strings.HasPrefix(profile, "/") {
			udd = profile
		} else {
			udd = w.getenv("HOME") + "/.agent-browser/profiles/" + profile
			note("'" + profile + "' is a shared profile name, not a path: every task in the project uses " + udd + ".")
			note("Chrome locks it, so the second task to launch loses — measured. Prefer $HW_BROWSER_PROFILE, or no profile at all (the default is a fresh ephemeral one).")
		}
		if out, err := exec.Command("pgrep", "-f", "--", "--user-data-dir="+udd).Output(); err == nil || len(out) > 0 {
			chromePids = strings.TrimRight(strings.ReplaceAll(string(out), "\n", " "), " ")
		}
	}

	// ── 4. report what was OBSERVED ──
	said := closeOut
	if said == "" {
		said = "<no output>"
	}
	if stillListed != "" {
		note("agent-browser said: " + said)
		note("but " + settle + "ms later these sessions are still registered: " + stillListed)
		note("that is past the settle window, so it is not the usual phantom — the close did not take.")
		note("DO NOT relaunch into a new session: each launch restores the shared profile's windows.")
		return 1
	}
	if inspectionError != "" {
		note("agent-browser said: " + said)
		note("but the independent sidecar observation failed: " + inspectionError)
		note("the registry read may prune state, so this close is not verified. DO NOT relaunch.")
		return 1
	}
	if liveSidecars != "" { // MUTATION-ANCHOR: s6-bc-live
		note("agent-browser said: " + said)
		note("but the independent sidecar snapshot before session list found live daemon(s): " + liveSidecars)
		note("the session registry may prune stale state; this close is not verified. DO NOT relaunch.")
		return 1
	}
	if chromePids != "" {
		note("no sessions are registered, but Chrome is still holding " + udd + " (pids: " + chromePids + ").")
		note("that is a live browser owned by something else — another task on this profile, or a")
		note("window a human opened. It is NOT this session's leak and this script will not kill it.")
		return 3 // MUTATION-ANCHOR: s6-bc-exit3
	}
	w.outf("closed and verified: no live daemon PID observed before registry pruning, and no registered session")
	if profile != "" {
		w.outf(", and no Chrome holding %s", udd)
	} else {
		w.outf("; no profile was checked")
	}
	w.outf("\n")
	return 0
}

func bcPad3(n int64) string {
	s := strconv.FormatInt(n, 10)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// bcCapture is `out="$("$prog" args… 2>&1)" || true`: both streams in the order written, trailing newlines gone.
func bcCapture(prog string, args ...string) string {
	out, _ := exec.Command(prog, args...).CombinedOutput()
	return trimNL(string(out))
}

// bcSleep is the settle: BROWSER_CLOSE_SLEEP (a test's stub) or /bin/sleep, given `S.mmm`; set -e makes its failure ours.
func (w *wenv) bcSleep(secs string) int {
	prog := w.getenv("BROWSER_CLOSE_SLEEP")
	if prog == "" {
		prog = "/bin/sleep"
		if _, err := os.Stat(prog); err != nil { // no /bin/sleep (Windows): wait in-process
			f, _ := strconv.ParseFloat(secs, 64)
			time.Sleep(time.Duration(f * float64(time.Second)))
			return 0
		}
	}
	cmd := exec.Command(prog, secs)
	cmd.Stdout, cmd.Stderr = w.stdout, w.stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if _, isExit := err.(*exec.ExitError); isExit {
		return exitCodeOf(err)
	}
	w.errf("%s: line %d: %s: No such file or directory\n", w.argv0, bcSleepLine, prog)
	if !bashIsLegacy() {
		return 127 // bash 4+ (5.2.15 measured on Linux): set -e leaves with the status of the failed command
	}
	return 1 // bash 3.2: set -e leaves with 1 here, not the 127 of the failed command (measured)
}

func bcIsDir(p string) bool  { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
func bcExists(p string) bool { _, err := os.Stat(p); return err == nil }
