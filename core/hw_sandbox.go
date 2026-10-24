package main

// hw-sandbox as a verb of hw-core: a port of bin/hw-sandbox — same stdout, stderr, exit status and on-disk effects. The bash stays
// beside it as the rollback: bin/wrapper-go.sh (sourced by bin/hw-sandbox) execs `hw-core hw-sandbox "$@"` unless HW_IMPL_HW_SANDBOX
// (else the switch file's `hw-sandbox=` line) says `bash`. THE DEFAULT IS GO.
//
// The shim hands over HW_WRAPPER_ARGV0 (the bash's own $0). The bash finds the invokers it runs, and the file `arm` writes into the
// launch shim, from where it lives; the Go does the same from that path.
//
// What the bash got from python3 — the outbox, pinned by device and inode and acted on relative to a descriptor — is here too, with
// no python: hw_sandbox_outbox_unix.go opens the directory O_NOFOLLOW, checks its identity and fchdir()s into it for each act, so
// every name is resolved inside the pinned directory and never through the agent-controlled path above it. Where the platform has
// no such open (native Windows) hw_sandbox_outbox_other.go checks by lstat right before each act, as the bash's python did.
//
// Line numbers below (sbL…) are the lines of bin/hw-sandbox that raise a `${x:?}` / unbound-variable error: bash names the line,
// and the Go says the same. hw_sandbox_parity_test.go runs both, so a line that moves in the bash is a red there.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sbLHome     = 137 // ${HOME:?HOME is unset}
	sbLWorkdir  = 138 // ${HW_WORKDIR:?HW_WORKDIR is unset}
	sbLStHome   = 238 // statedir: $HOME unbound
	sbLStRundir = 237 // statedir: ${1:?rundir}
	sbLArmRun   = 354 // arm: ${1:?rundir}, ${2:?kind}
	sbLRunRun   = 379 // run: ${1:?rundir}, ${2:?executable}
	sbLBrRun    = 436 // broker: ${1:?rundir}, ${2:?the owner pid…}
	sbLRecHome  = 251 // _task_record: $HOME unbound
)

const sbHeartbeat = "broker.alive"

const sbUsage = "usage: hw-sandbox profile|supported|statedir <rundir>|arm <rundir> <kind>|run <rundir> <exe> …|broker <rundir> <owner-pid>|request <invoker> …"

type sbT struct {
	out, errw io.Writer
	argv0     string // the path the bash was started as ($0)
	binDir    string // BIN_DIR: where the real hw-sandbox lives, links followed
}

func sbMain(args []string, stdout, stderr io.Writer) int {
	argv0 := os.Getenv("HW_WRAPPER_ARGV0")
	if argv0 == "" {
		if exe, err := os.Executable(); err == nil {
			argv0 = filepath.Join(filepath.Dir(exe), "hw-sandbox")
		} else {
			argv0 = "hw-sandbox"
		}
	}
	s := &sbT{out: stdout, errw: stderr, argv0: argv0}
	s.binDir = sbSelfDir(argv0)
	if len(args) == 0 {
		return s.die(sbUsage)
	}
	rest := args[1:]
	switch args[0] {
	case "profile":
		return s.cmdProfile()
	case "statedir":
		return s.cmdStatedir(rest)
	case "armed":
		return s.cmdArmed(rest)
	case "supported":
		msg, ok := sbSupported()
		if !ok {
			s.outf("%s", msg)
			return 1
		}
		return 0
	case "arm":
		return s.cmdArm(rest)
	case "run":
		return s.cmdRun(rest)
	case "broker":
		return s.cmdBroker(rest)
	case "request":
		return s.cmdRequest(rest)
	}
	return s.die(sbUsage) // MUTATION-ANCHOR: s8-usage
}

func (s *sbT) outf(f string, a ...any) { fmt.Fprintf(s.out, f, a...) }
func (s *sbT) errf(f string, a ...any) { fmt.Fprintf(s.errw, f, a...) }
func (s *sbT) die(msg string) int      { s.errf("hw-sandbox: %s\n", msg); return 1 }
func (s *sbT) self() string            { return s.binDir + "/hw-sandbox" }
func (s *sbT) colon(line int, name, msg string) int {
	s.errf("%s: line %d: %s: %s\n", s.argv0, line, name, msg)
	return 1
}
func (s *sbT) unbound(line int, name string) int {
	s.errf("%s: line %d: %s: unbound variable\n", s.argv0, line, name)
	return 1
}

// sbSelfDir is BIN_DIR: the directory the bash resolves for itself by following the links of its own file.
func sbSelfDir(argv0 string) string {
	src := argv0
	for i := 0; i < 40; i++ {
		fi, err := os.Lstat(src)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}
		dir := sbPhys(sbDirname(src))
		to, err := os.Readlink(src)
		if err != nil {
			break
		}
		if !strings.HasPrefix(to, "/") {
			to = dir + "/" + to
		}
		src = to
	}
	return sbPhys(sbDirname(src))
}

// sbPhys is `cd -P "$d" && pwd`: the directory with its own links followed, or the path as given when it does not resolve.
func sbPhys(d string) string {
	if abs, err := filepath.Abs(d); err == nil {
		d = abs
	}
	if r, err := filepath.EvalSymlinks(d); err == nil {
		return r
	}
	return d
}

// sbDirname and sbBasename are dirname(1) and basename(1): trailing slashes dropped, no lexical clean-up of `..`.
func sbDirname(p string) string {
	t := strings.TrimRight(p, "/")
	if t == "" {
		if p == "" {
			return "."
		}
		return "/"
	}
	i := strings.LastIndex(t, "/")
	if i < 0 {
		return "."
	}
	d := strings.TrimRight(t[:i], "/")
	if d == "" {
		return "/"
	}
	return d
}

func sbBasename(p string) string {
	t := strings.TrimRight(p, "/")
	if t == "" {
		if p == "" {
			return ""
		}
		return "/"
	}
	if i := strings.LastIndex(t, "/"); i >= 0 {
		return t[i+1:]
	}
	return t
}

func sbIsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// sbCanon is _canon: the path resolved through its nearest existing ancestor, so the answer is the same before and after the leaf
// is created (the state directory's name is a hash of it). Seatbelt compares RESOLVED paths.
func sbCanon(p string) string {
	rest := ""
	for !sbIsDir(p) && p != "/" && p != "" {
		rest = "/" + sbBasename(p) + rest
		p = sbDirname(p)
	}
	base := sbPhys(p) // MUTATION-ANCHOR: s8-canon
	if p == "" {
		base = sbPhys(".")
	}
	base = strings.TrimSuffix(base, "/")
	if base+rest == "" {
		return "/"
	}
	return base + rest
}

// sbLit is _lit: a path is spliced into the profile as a string literal; anything that could end the literal or start an escape is
// refused, never escaped.
func sbLit(p string) (string, string) {
	switch {
	case strings.ContainsAny(p, "\"\\\n"): // MUTATION-ANCHOR: s8-lit
		return "", "path not representable in a profile: " + p
	case strings.HasPrefix(p, "/"):
		return `"` + p + `"`, ""
	}
	return "", "path is not absolute: " + p
}

// sbRe is _re: the path as an anchored regex, for the few rules that need a suffix.
func sbRe(p string) string {
	var b strings.Builder
	for _, r := range p {
		if strings.ContainsRune(`][\.^$*+?(){}|`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sbSedAlnum is `sed 's/[^A-Za-z0-9]/-/g'`: a character per byte in the C locale, per rune in a UTF-8 one.
func sbSedAlnum(p string) string {
	loc := os.Getenv("LC_ALL")
	if loc == "" {
		loc = os.Getenv("LC_CTYPE")
	}
	if loc == "" {
		loc = os.Getenv("LANG")
	}
	utf := strings.Contains(strings.ToLower(strings.ReplaceAll(loc, "-", "")), "utf8")
	var b strings.Builder
	keep := func(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' } // MUTATION-ANCHOR: s8-proj
	if utf {
		for _, r := range p {
			if keep(r) {
				b.WriteRune(r)
			} else {
				b.WriteByte('-')
			}
		}
		return b.String()
	}
	for i := 0; i < len(p); i++ {
		if keep(rune(p[i])) {
			b.WriteByte(p[i])
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func sbPiAgentDir(home string) string {
	d := os.Getenv("PI_CODING_AGENT_DIR")
	switch {
	case d == "":
		d = home + "/.pi/agent"
	case d == "~":
		d = home
	case strings.HasPrefix(d, "~/"): // MUTATION-ANCHOR: s8-pi-tilde
		d = home + "/" + d[2:]
	}
	return sbCanon(d)
}

// sbPiSessionDirName is `-- + the absolute cwd without its leading slash, every / and : turned into '-', + --`.
func sbPiSessionDirName(p string) string {
	return "--" + strings.NewReplacer("/", "-", ":", "-").Replace(strings.TrimPrefix(p, "/")) + "--" // MUTATION-ANCHOR: s8-pi-name
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// sbGit runs git in dir, its stdout without the trailing newlines; ok is its exit status being 0.
func sbGit(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\n"), err == nil
}

// sbCanonIn is `cd "$dir" && _canon "$p"`: a relative p is the one git printed, relative to dir.
func sbCanonIn(dir, p string) string {
	if !strings.HasPrefix(p, "/") {
		p = strings.TrimRight(dir, "/") + "/" + p
	}
	return sbCanon(p)
}

// ── the profile ──────────────────────────────────────────────────────────────

func (s *sbT) cmdProfile() int {
	text, rc := s.profile(envOr("HW_SANDBOX_KIND", "claude"))
	s.outf("%s", text)
	return rc
}

// profile is cmd_profile: the text printed (all of it, or what was printed before a die) and the exit status.
func (s *sbT) profile(kind string) (string, int) {
	h := os.Getenv("HOME")
	if h == "" {
		return "", s.colon(sbLHome, "HOME", "HOME is unset")
	}
	home := sbCanon(h)
	w := os.Getenv("HW_WORKDIR")
	if w == "" {
		return "", s.colon(sbLWorkdir, "HW_WORKDIR", "HW_WORKDIR is unset")
	}
	wd := sbCanon(w)
	art := envOr("HW_ARTIFACTS", wd+"/.artifacts")
	_ = os.MkdirAll(art, 0o777)
	art = sbCanon(art)
	tmp := sbCanon(envOr("TMPDIR", "/tmp"))
	cfg := sbCanon(envOr("CLAUDE_CONFIG_DIR", home+"/.claude"))
	engram := sbCanon(envOr("ENGRAM_DATA_DIR", home+"/.engram"))
	projname := sbSedAlnum(wd)
	piagent := sbPiAgentDir(home)
	pisess := sbPiSessionDirName(wd)
	var wt, common, gitdir string
	if _, ok := sbGit(wd, "rev-parse", "--is-inside-work-tree"); ok {
		top, _ := sbGit(wd, "rev-parse", "--show-toplevel")
		wt = sbCanon(top)
		c, _ := sbGit(wd, "rev-parse", "--git-common-dir")
		common = sbCanonIn(wd, c)
		g, _ := sbGit(wd, "rev-parse", "--git-dir")
		gitdir = sbCanonIn(wd, g)
	}
	// EVERY PATH IS CHECKED HERE, before a byte is printed.
	for _, p := range []string{home, wd, art, tmp, cfg, piagent, engram, wt, common, gitdir} {
		if p == "" {
			continue
		}
		if _, e := sbLit(p); e != "" {
			return "", s.die(e)
		}
	}
	lit := func(p string) string { l, _ := sbLit(p); return l }
	var b strings.Builder
	pf := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	pf(";; hw-sandbox hard profile — generated per dispatch; see bin/hw-sandbox\n")
	pf("(version 1)\n(allow default)\n(deny file-write*)\n")
	pf("(allow file-write*\n")
	pf("  (subpath %s)\n", lit(wd))
	if wt != "" && wt != wd {
		pf("  (subpath %s)\n", lit(wt))
	}
	pf("  (subpath %s)\n  (subpath %s)\n", lit(art), lit(tmp))
	pf("  (subpath \"/private/tmp\")\n")
	pf("  (literal \"/dev/null\") (literal \"/dev/tty\") (regex #\"^/dev/ttys[0-9]+$\") (literal \"/dev/ptmx\") (literal \"/dev/dtracehelper\")\n")
	pf("  (subpath %s)\n", lit(engram))
	if kind != "claude" && kind != "pi" { // MUTATION-ANCHOR: s8-kind
		return b.String(), s.die("no measured sandbox profile for " + kind + " (claude and pi only)")
	}
	if kind == "claude" {
		pf("  (subpath %s)\n", lit(cfg+"/projects/"+projname))
	} else {
		pf("  (subpath %s)\n", lit(piagent+"/sessions/"+pisess))
		pf("  (subpath %s) (subpath %s) (subpath %s)\n", lit(piagent+"/settings.json.lock"), lit(piagent+"/auth.json.lock"), lit(piagent+"/models-store.json.lock"))
	}
	pf(")\n")
	if common != "" && common != gitdir { // MUTATION-ANCHOR: s8-gitslice
		pf(";; the linked worktree's slice of the common .git — never hooks/ or config\n")
		pf("(allow file-write*\n")
		pf("  (subpath %s) (subpath %s) (subpath %s) (subpath %s)\n", lit(common+"/objects"), lit(common+"/refs"), lit(common+"/logs"), lit(gitdir))
		pf("  (regex #\"^%s/packed-refs(\\.lock|\\.new)?$\"))\n", sbRe(common))
	}
	pf(";; control sockets run code outside the sandbox: herdr, docker, ssh-agent\n")
	pf("(deny network-outbound (remote unix-socket))\n")
	pf("(allow network-outbound (remote unix-socket (path-literal \"/private/var/run/mDNSResponder\")))\n")
	pf(";; credentials that reach other machines\n")
	pf("(deny file-read* (subpath %s) (subpath %s) (subpath %s))\n", lit(home+"/.ssh"), lit(home+"/.config/gh"), lit(home+"/.docker"))
	return b.String(), 0
}

// ── the platform ─────────────────────────────────────────────────────────────

// sbSupported is cmd_supported: the sentence it prints (with its newline) when the sandbox is not available, and whether it is.
func sbSupported() (string, bool) {
	osn := os.Getenv("HW_SANDBOX_UNAME")
	if osn == "" {
		osn = sbUname()
	}
	if osn != "Darwin" { // MUTATION-ANCHOR: s8-darwin
		return fmt.Sprintf("the sandbox is Seatbelt (sandbox-exec), which exists only on macOS; this is %s. Dispatch without --sandbox, or from a Mac.\n", osn), false
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil && !sbExecutable("/usr/bin/sandbox-exec") {
		return "sandbox-exec is not on this Mac (expected /usr/bin/sandbox-exec). Dispatch without --sandbox.\n", false
	}
	return "", true
}

func sbExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && sbAccess(p, 1)
}

// ── state directory and the armed record ─────────────────────────────────────

func sbSha24(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:24] // MUTATION-ANCHOR: s8-key
}

func (s *sbT) stateRoot(line int) (string, int) {
	if r := os.Getenv("HW_SANDBOX_STATE_ROOT"); r != "" {
		return r, 0
	}
	home, ok := os.LookupEnv("HOME")
	if !ok {
		return "", s.unbound(line, "HOME")
	}
	return home + "/.local/state/hw-sandbox", 0
}

// stateDir is cmd_statedir: the state directory of a run, creating nothing.
func (s *sbT) stateDir(rundir string, line int) (string, int) {
	root, rc := s.stateRoot(line)
	if rc != 0 {
		return "", rc
	}
	return root + "/" + sbSha24(strings.TrimRight(sbCanon(rundir), "\n")), 0
}

func (s *sbT) cmdStatedir(a []string) int {
	if len(a) < 1 || a[0] == "" {
		return s.colon(sbLStRundir, "1", "rundir")
	}
	st, rc := s.stateDir(a[0], sbLStHome)
	if rc != 0 {
		return rc
	}
	s.outf("%s", st)
	return 0
}

func (s *sbT) taskRecord(lane, task string) (string, int) {
	root, rc := s.stateRoot(sbLRecHome)
	if rc != 0 {
		return "", rc
	}
	return root + "/tasks/" + sbSha24(lane+":"+task), 0
}

// cmdArmed: 0 armed, 1 never, 2 cannot tell — never 1 by accident.
func (s *sbT) cmdArmed(a []string) int {
	if len(a) < 2 || a[0] == "" || a[1] == "" {
		s.errf("hw-sandbox: armed needs <lane> <task>\n")
		return 2
	}
	r, rc := s.taskRecord(a[0], a[1])
	if rc != 0 {
		return 2
	}
	if _, err := os.Stat(r); err == nil { // MUTATION-ANCHOR: s8-armed
		return 0
	}
	return 1
}

// ── arming a pane ────────────────────────────────────────────────────────────

// sbQuote is `printf %q`: backslash before anything but the characters bash leaves bare, $'…' for a control character.
func sbQuote(p string) string {
	if p == "" {
		return "''"
	}
	if strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		var b strings.Builder
		b.WriteString("$'")
		for _, r := range p {
			switch {
			case r == '\n':
				b.WriteString(`\n`)
			case r == '\t':
				b.WriteString(`\t`)
			case r == '\r':
				b.WriteString(`\r`)
			case r == 0x1b:
				b.WriteString(`\E`)
			case r == '\'' || r == '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case r < 0x20 || r == 0x7f:
				fmt.Fprintf(&b, `\%03o`, r)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteString("'")
		return b.String()
	}
	var b strings.Builder
	for _, r := range p {
		if r >= 0x80 || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/.-_+=:@%~#", r) { // MUTATION-ANCHOR: s8-quote
			b.WriteRune(r)
		} else {
			b.WriteByte('\\')
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sbCommandV is `command -v <kind>` against the PATH given: the first executable file named kind, as dir/kind.
func sbCommandV(kind, path string) string {
	if strings.Contains(kind, "/") {
		if sbExecutable(kind) {
			return kind
		}
		return ""
	}
	for _, d := range strings.Split(path, ":") {
		if d == "" {
			d = "."
		}
		if p := d + "/" + kind; sbExecutable(p) {
			return p
		}
	}
	return ""
}

func (s *sbT) cmdArm(a []string) int {
	if len(a) < 1 || a[0] == "" {
		return s.colon(sbLArmRun, "1", "rundir")
	}
	if len(a) < 2 || a[1] == "" {
		return s.colon(sbLArmRun, "2", "kind")
	}
	rundir, kind := a[0], a[1]
	if msg, ok := sbSupported(); !ok { // MUTATION-ANCHOR: s8-arm-supported
		return s.die(strings.TrimRight(msg, "\n"))
	}
	st, rc := s.stateDir(rundir, sbLStHome)
	if rc != 0 {
		return rc
	}
	_ = os.RemoveAll(st) // MUTATION-ANCHOR: s8-rebuild
	if err := os.MkdirAll(filepath.Dir(st), 0o777); err != nil {
		s.errf("mkdir: %s: %v\n", filepath.Dir(st), sbErrText(err))
		return 1
	}
	if err := os.MkdirAll(rundir+"/outbox", 0o777); err != nil {
		s.errf("mkdir: %s: %v\n", rundir+"/outbox", sbErrText(err))
		return 1
	}
	failed := false
	for _, d := range []string{st, st + "/bin", st + "/work"} { // ( umask 077; mkdir "$st" "$st/bin" "$st/work" ): every one is tried, and each failure is mkdir's own line
		if err := sbMkdirPrivate(d); err != nil { // MUTATION-ANCHOR: s8-private
			name := d
			if errors.Is(err, os.ErrNotExist) { // BSD mkdir names the missing parent, not the child
				name = filepath.Dir(d)
			}
			s.errf("mkdir: %s: %s\n", name, sbErrText(err))
			failed = true
		}
	}
	if failed {
		return s.die("cannot create the state directory " + st)
	}
	var keep []string
	for _, d := range strings.Split(os.Getenv("PATH"), ":") {
		if d != st+"/bin" {
			keep = append(keep, d)
		}
	}
	real := sbCommandV(kind, strings.Join(keep, ":"))
	if real == "" {
		return s.die("cannot find the " + kind + " executable on PATH")
	}
	prof, prc := s.profile(kind)
	_ = os.WriteFile(st+"/profile.sb", []byte(prof), 0o666)
	if prc != 0 {
		return prc
	}
	if len(a) >= 4 && a[2] != "" && a[3] != "" {
		rec, rc := s.taskRecord(a[2], a[3])
		if rc != 0 {
			return rc
		}
		if sbMkdirAllPrivate(filepath.Dir(rec)) != nil || os.WriteFile(rec, []byte(rundir+"\n"), 0o666) != nil { // MUTATION-ANCHOR: s8-record
			return s.die(fmt.Sprintf("cannot record that %s:%s was armed (%s)", a[2], a[3], rec))
		}
	}
	shim := "#!/bin/bash\n" +
		"# written by hw-sandbox arm: the " + kind + " this pane starts runs in the sandbox.\n" +
		"exec " + sbQuote(s.self()) + " run " + sbQuote(rundir) + " " + sbQuote(real) + " \"$@\"\n" // MUTATION-ANCHOR: s8-shim
	if err := os.WriteFile(st+"/bin/"+kind, []byte(shim), 0o666); err != nil {
		s.errf("%s: line %d: %s/bin/%s: %s\n", s.argv0, 0, st, kind, sbErrText(err))
		return 1
	}
	_ = os.Chmod(st+"/bin/"+kind, 0o755)
	s.outf("%s\n", st+"/bin")
	return 0
}

// ── the launch wrapper ───────────────────────────────────────────────────────

func (s *sbT) cmdRun(a []string) int {
	if len(a) < 1 || a[0] == "" {
		return s.colon(sbLRunRun, "1", "rundir")
	}
	if len(a) < 2 || a[1] == "" {
		return s.colon(sbLRunRun, "2", "executable")
	}
	rundir, real, rest := a[0], a[1], a[2:]
	st, rc := s.stateDir(rundir, sbLStHome)
	if rc != 0 {
		return rc
	}
	if !sbAccess(st+"/profile.sb", 4) {
		return s.die("no profile at " + st + "/profile.sb — run hw-sandbox arm first")
	}
	if err := os.MkdirAll(rundir+"/outbox", 0o777); err != nil {
		s.errf("mkdir: %s: %v\n", rundir+"/outbox", sbErrText(err))
		return 1
	}
	// THE BROKER IS THE LAUNCH WRAPPER'S OWN CHILD, in a session of its own, started BEFORE the sandbox so it is outside the profile.
	sbStartBroker(s, rundir, st)                                                                               // MUTATION-ANCHOR: s8-broker
	cmd := exec.Command("/usr/bin/sandbox-exec", append([]string{"-f", st + "/profile.sb", real}, rest...)...) // MUTATION-ANCHOR: s8-run-args
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "HW_SANDBOX=1", "HW_SANDBOX_OUTBOX="+rundir+"/outbox", "HW_SANDBOX_STATE="+st, "DISABLE_AUTOUPDATER=1") // MUTATION-ANCHOR: s8-run-env
	return sbWaitChild(s, cmd)
}

// ── the broker (outside) ─────────────────────────────────────────────────────

var sbRidRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func (s *sbT) cmdBroker(a []string) int {
	if len(a) < 1 || a[0] == "" {
		return s.colon(sbLBrRun, "1", "rundir")
	}
	if len(a) < 2 || a[1] == "" {
		return s.colon(sbLBrRun, "2", "the owner pid: a broker nobody owns would never exit")
	}
	rundir, owner := a[0], a[1]
	ob := rundir + "/outbox"
	if err := os.MkdirAll(ob, 0o777); err != nil {
		s.errf("mkdir: %s: %v\n", ob, sbErrText(err))
		return 1
	}
	// every path this process acts on from now on is absolute: the outbox is entered by fchdir, and a relative name would be read there
	if abs, err := filepath.Abs(ob); err == nil {
		ob = abs
	}
	if wd := os.Getenv("HW_WORKDIR"); wd != "" && !strings.HasPrefix(wd, "/") {
		if abs, err := filepath.Abs(wd); err == nil {
			os.Setenv("HW_WORKDIR", abs)
		}
	}
	o, err := sbPin(ob)
	if err != nil {
		s.errf("%v\n", err)
		return s.die("cannot pin the outbox " + ob)
	}
	st, rc := s.stateDir(rundir, sbLStHome)
	if rc != 0 {
		return rc
	}
	if abs, err := filepath.Abs(st); err == nil {
		st = abs
	}
	w := st + "/work"
	if !sbIsDir(w) || sbIsLink(st) || sbIsLink(w) {
		return s.die("no state directory at " + st + " — run hw-sandbox arm first")
	}
	sbIgnoreHupTerm()                       // MUTATION-ANCHOR: s8-sigign
	defer os.Remove(st + "/" + sbHeartbeat) // MUTATION-ANCHOR: s8-hb-remove
	poll := 500 * time.Millisecond
	if v := os.Getenv("HW_SANDBOX_POLL"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			poll = time.Duration(f * float64(time.Second))
		}
	}
	var wg sync.WaitGroup
	for {
		if !sbOwnerAlive(owner) { // MUTATION-ANCHOR: s8-owner
			wg.Wait() // MUTATION-ANCHOR: s8-wait
			return 0
		}
		if f, err := os.OpenFile(st+"/"+sbHeartbeat, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666); err == nil { // MUTATION-ANCHOR: s8-hb
			f.Close()
		} else {
			s.errf("%s: line %d: %s/%s: %s\n", s.argv0, 0, st, sbHeartbeat, sbErrText(err))
			wg.Wait()
			return 1
		}
		for _, id := range o.claim(w) {
			id := id
			wg.Add(1)
			go func() { defer wg.Done(); s.serveOne(o, w, id) }()
		}
		time.Sleep(poll)
	}
}

func sbIsLink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// serveOne is _serve_one: take the request's argv, run the invoker it names — or refuse — and hand back .out, .err and, last, .rc.
func (s *sbT) serveOne(o *sbOutbox, w, id string) {
	b, _ := os.ReadFile(w + "/" + id + ".claimed")
	os.Remove(w + "/" + id + ".claimed")
	var argv []string // `read -r -d ''`: only a NUL-terminated word counts
	for len(b) > 0 {
		i := bytes.IndexByte(b, 0)
		if i < 0 {
			break
		}
		argv = append(argv, string(b[:i]))
		b = b[i+1:]
	}
	verb := ""
	if len(argv) > 0 {
		verb = argv[0]
	}
	rc := 0
	switch verb {
	case "done-invoker", "ask-invoker": // MUTATION-ANCHOR: s8-verbs
		rc = s.runInvoker(w, id, verb, argv[1:])
	default:
		os.WriteFile(w+"/"+id+".err", []byte("hw-sandbox broker: refused: that is not an invoker the sandbox hands out (only done-invoker and ask-invoker)\n"), 0o666)
		os.WriteFile(w+"/"+id+".out", nil, 0o666)
		rc = 1
	}
	os.WriteFile(w+"/"+id+".rc", []byte(strconv.Itoa(rc)+"\n"), 0o666)
	o.deliver(w+"/"+id+".out", id+".out")
	o.deliver(w+"/"+id+".err", id+".err")
	o.deliver(w+"/"+id+".rc", id+".rc")
}

// runInvoker runs the REAL invoker from the brain checkout that launched this dispatch, under the environment the pane had at
// launch (minus the three sandbox variables), on /dev/null, and writes its stdout and stderr to the state dir. Its exit status.
func (s *sbT) runInvoker(w, id, verb string, args []string) int {
	out, _ := os.OpenFile(w+"/"+id+".out", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	errf, _ := os.OpenFile(w+"/"+id+".err", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	defer out.Close()
	defer errf.Close()
	path := s.binDir + "/" + verb
	cmd := exec.Command(path, args...)
	cmd.Stdout, cmd.Stderr = out, errf
	if d := envOr("HW_WORKDIR", "."); sbIsDir(d) && sbAccess(d, 1) {
		cmd.Dir = d // MUTATION-ANCHOR: s8-cwd
	}
	for _, e := range os.Environ() {
		if k := e[:strings.IndexByte(e+"=", '=')]; k != "HW_SANDBOX" && k != "HW_SANDBOX_OUTBOX" && k != "HW_SANDBOX_STATE" { // MUTATION-ANCHOR: s8-envu
			cmd.Env = append(cmd.Env, e)
		}
	}
	if err := cmd.Start(); err != nil { // env(1)'s own words and statuses
		switch {
		case errors.Is(err, os.ErrPermission):
			fmt.Fprintf(errf, "env: %s: Permission denied\n", path)
			return 126
		default:
			fmt.Fprintf(errf, "env: %s: No such file or directory\n", path) // MUTATION-ANCHOR: s8-envmiss
			return 127
		}
	}
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	return sbExitStatus(cmd.ProcessState)
}

// ── the client (inside) ──────────────────────────────────────────────────────

func (s *sbT) cmdRequest(a []string) int {
	ob, st := os.Getenv("HW_SANDBOX_OUTBOX"), os.Getenv("HW_SANDBOX_STATE")
	verb := ""
	if len(a) > 0 {
		verb = a[0]
	}
	if ob == "" || st == "" {
		return s.die("HW_SANDBOX_OUTBOX/HW_SANDBOX_STATE are unset — this is not a sandboxed executor")
	}
	if !sbIsDir(ob) {
		return s.die("the outbox " + ob + " does not exist")
	}
	// stdin does not cross (the broker runs the invoker on /dev/null), so the `-` form would fail with a misleading message.
	if len(a) > 1 {
		for _, x := range a[1:] {
			if x == "-" { // MUTATION-ANCHOR: s8-stdin
				return s.die("the stdin form (" + verb + " -) cannot cross the sandbox: pass the paragraph as the argument. NOTHING was sent.")
			}
		}
	}
	stale := 20
	if v := os.Getenv("HW_SANDBOX_STALE_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			stale = n
		}
	}
	id := fmt.Sprintf("%d-%d-%d", time.Now().Unix(), os.Getpid(), rand.Intn(32768))
	var req bytes.Buffer
	for _, x := range a {
		req.WriteString(x)
		req.WriteByte(0)
	}
	if len(a) == 0 {
		req.WriteByte(0) // printf '%s\0' with no argument still prints the format once
	}
	if err := os.WriteFile(ob+"/"+id+".req.tmp", req.Bytes(), 0o666); err != nil {
		s.errf("%s: line %d: %s: %s\n", s.argv0, 0, ob+"/"+id+".req.tmp", sbErrText(err))
		return 1
	}
	os.Rename(ob+"/"+id+".req.tmp", ob+"/"+id+".req")
	for !sbIsFile(ob + "/" + id + ".rc") {
		if age, ok := sbAge(st + "/" + sbHeartbeat); !ok || age > stale { // MUTATION-ANCHOR: s8-stale
			if sbIsFile(ob + "/" + id + ".req") {
				os.Remove(ob + "/" + id + ".req") // MUTATION-ANCHOR: s8-withdraw
				return s.die(fmt.Sprintf("the sandbox broker is not running (no heartbeat in %s for over %ds), so %s could not leave the sandbox. NOTHING was sent. Report this pane as blocked to the operator: the broker is started by the launch wrapper, so the agent must be relaunched.", st, stale, verb))
			}
			return s.die("the sandbox broker stopped while running " + verb + ", so whether it was delivered is UNKNOWN. Do NOT re-run it; the brainer's pane or `hw outbox` says whether it landed.")
		}
		time.Sleep(300 * time.Millisecond)
	}
	if b, err := os.ReadFile(ob + "/" + id + ".out"); err == nil {
		s.out.Write(b)
	}
	if b, err := os.ReadFile(ob + "/" + id + ".err"); err == nil {
		s.errw.Write(b) // MUTATION-ANCHOR: s8-replay-err
	}
	rcb, _ := os.ReadFile(ob + "/" + id + ".rc")
	var digits []byte
	for _, c := range rcb {
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	for _, e := range []string{".out", ".err", ".rc", ".req"} {
		os.Remove(ob + "/" + id + e)
	}
	rc := 1 // `exit "${rc:-1}"`; a status of any length wraps modulo 256 like any other
	if len(digits) > 0 {
		rc = 0
		for _, c := range digits {
			rc = (rc*10 + int(c-'0')) % 256 // MUTATION-ANCHOR: s8-rc
		}
	}
	return rc
}

func sbIsFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// sbAge is _age: seconds since the file was modified; ok is false when it does not exist (`[ -f ]`).
func sbAge(p string) (int, bool) {
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return 0, false
	}
	return int(time.Now().Unix() - fi.ModTime().Unix()), true
}

func sbErrText(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	t := err.Error()
	if t == "" {
		return t
	}
	return strings.ToUpper(t[:1]) + t[1:]
}
