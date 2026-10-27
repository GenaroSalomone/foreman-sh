package main

// hw-secret in Go: `hw-core hw-secret <verb> …` is bin/hw-secret, the one path from Bitwarden to an executor, and it
// never prints a value. The bash is the rollback and stays beside it; setup/tests/1022 runs both on the same inputs and
// diffs them, setup/tests/1023 kills a mutant of each anchor named `<n>-G<nn>` (the bash's `<n>-M<nn>`).
//
// SAME WORDS, STREAMS, EXIT STATUS AND FILES as the bash — one place differs on purpose, and one is not reproducible:
//   - the bash needs jq (`jq is required`); this reads bw's JSON itself, so a machine with no jq works;
//   - `exec: <cmd>: not found` carries the bash's `<script path>: line N:` prefix there and does not here.
//
// The value of an item is held in one Go string, put in the CHILD's environment and exec'd over this process: it is
// never written to stdout, stderr or disk, and bw's stderr is dropped, as in the bash.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	hsAgentsFolder  = "agents"
	hsMasterService = "hw-bw-master"
)

// hsExit unwinds to runHwSecret: `die` in the bash is `exit 1` from wherever it is called.
type hsExit struct{ rc int }

// hsWorld is everything the program touches outside itself, so a test can drive it in process.
type hsWorld struct {
	out, err io.Writer
	in       io.Reader
	env      []string // the process environment; exported variables are set here, BW_SESSION too
	now      func() time.Time
	// execChild replaces the process (it returns only when it could not): the exit status to die with, and why.
	execChild func(argv, env []string) (rc int, why string)
	tty       bool // stdin is a terminal: the password prompts switch the echo off
}

func (w *hsWorld) getenv(k string) string {
	for i := len(w.env) - 1; i >= 0; i-- {
		if strings.HasPrefix(w.env[i], k+"=") {
			return w.env[i][len(k)+1:]
		}
	}
	return ""
}

func envWithout(env []string, drop func(name string) bool) []string {
	var r []string
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if !drop(name) {
			r = append(r, kv)
		}
	}
	return r
}

func envSet(env []string, k, v string) []string {
	return append(envWithout(env, func(n string) bool { return n == k }), k+"="+v)
}

func (w *hsWorld) die(msg string) {
	fmt.Fprintf(w.err, "hw secret: %s\n", msg)
	panic(hsExit{1})
}

func (w *hsWorld) note(msg string) { fmt.Fprintf(w.err, "hw secret: %s\n", msg) }

// quietly runs f with stderr thrown away: `f 2>/dev/null` of the bash.
func (w *hsWorld) quietly(f func()) {
	keep := w.err
	w.err = io.Discard
	defer func() { w.err = keep }()
	f()
}

func (w *hsWorld) secretDir() string {
	if d := w.getenv("HW_SECRET_DIR"); d != "" {
		return d
	}
	return w.getenv("HOME") + "/.cache/hw"
}
func (w *hsWorld) sessionFile() string { return w.secretDir() + "/bw-session" }
func (w *hsWorld) statusFile() string  { return w.secretDir() + "/bw-unlock-status" }

// bwPath is BW of the bash: HW_BW_BIN, else bw on PATH, else ~/.npm-global/bin/bw.
func (w *hsWorld) bwPath() string {
	if b := w.getenv("HW_BW_BIN"); b != "" {
		return b
	}
	if p := hsLookPath("bw", w.env); p != "" {
		return p
	}
	if p := w.getenv("HOME") + "/.npm-global/bin/bw"; hsIsExec(p) {
		return p
	}
	return ""
}

func (w *hsWorld) needBw() string {
	bw := w.bwPath()
	if bw == "" || !hsIsExec(bw) {
		w.die("the bw CLI is not installed (npm i -g @bitwarden/cli)")
	}
	return bw
}

// bw runs the vault CLI with its stderr dropped and returns stdout and the exit status (-1: it did not start).
func (w *hsWorld) bw(bw string, env []string, args ...string) (string, int) {
	c := exec.Command(bw, args...)
	c.Env = env
	var o bytes.Buffer
	c.Stdout = &o
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return o.String(), ee.ExitCode()
		}
		return o.String(), -1
	}
	return o.String(), 0
}

// noSession is `env -u BW_SESSION`.
func (w *hsWorld) noSession() []string {
	return envWithout(w.env, func(n string) bool { return n == "BW_SESSION" })
}

// ── the session ─────────────────────────────────────────────────────────────

func (w *hsWorld) expire(bw string) {
	os.Remove(w.sessionFile())
	w.bw(bw, w.noSession(), "lock")
}

// sessionLoad: true = a live session, now in the environment as BW_SESSION; false = locked (the file, if any, is gone).
func (w *hsWorld) sessionLoad(bw string) bool {
	b, err := os.ReadFile(w.sessionFile())
	if err != nil {
		return false
	}
	lines := strings.SplitN(string(b), "\n", 3)
	exp := strings.TrimPrefix(lines[0], "expires=")
	tok := ""
	if len(lines) > 1 {
		tok = lines[1]
	}
	if exp == "" || strings.Trim(exp, "0123456789") != "" {
		os.Remove(w.sessionFile())
		return false
	}
	if tok == "" {
		os.Remove(w.sessionFile())
		return false
	}
	if n, err := strconv.ParseInt(exp, 10, 64); err == nil && n <= w.now().Unix() { // # MUTATION-ANCHOR: 1009-G01
		w.expire(bw)
		return false
	}
	// The token must still open the vault: someone may have run `bw lock` since.
	out, _ := w.bw(bw, envSet(w.env, "BW_SESSION", tok), "status")
	if hsStatus(out) != "unlocked" { // # MUTATION-ANCHOR: 1009-G07
		os.Remove(w.sessionFile())
		return false
	}
	w.env = envSet(w.env, "BW_SESSION", tok)
	return true
}

func hsUntil(exp string) string {
	n, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return "epoch " + exp
	}
	return time.Unix(n, 0).Format("2006-01-02 15:04")
}

// sessionWrite: the session file, 0600 in a 0700 dir, with its expiry.
func (w *hsWorld) sessionWrite(tok string, hours int64) {
	dir := w.secretDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		w.die(err.Error())
	}
	os.Chmod(dir, 0o700)
	exp := w.now().Unix() + hours*3600
	tmp, err := os.CreateTemp(dir, ".bw-session.") // 0600
	if err != nil {
		w.die(err.Error())
	}
	fmt.Fprintf(tmp, "expires=%d\n%s\n", exp, tok)
	tmp.Close()
	os.Rename(tmp.Name(), w.sessionFile()) // # MUTATION-ANCHOR: 1009-G06
	fmt.Fprintf(w.out, "unlocked until %s (%d h); the session is in %s (0600), nothing secret was printed\n", hsUntil(strconv.FormatInt(exp, 10)), hours, w.sessionFile())
}

func (w *hsWorld) autoStatus(state, why string) {
	dir := w.secretDir()
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o700)
	line := state + " " + w.now().Format("2006-01-02 15:04:05")
	if why != "" {
		line += " — " + why
	}
	os.WriteFile(w.statusFile(), []byte(line+"\n"), 0o600)
}

func (w *hsWorld) autoFail(why string) int {
	w.autoStatus("failed", why) // # MUTATION-ANCHOR: 1012-G02
	w.note("secret-unlock --auto failed: " + why)
	return 1
}

// statusReason is `sed 's/^[a-z]* [0-9: -]*// ' "$STATUS_FILE"`; ok is false when the file cannot be read.
var hsReasonRe = regexp.MustCompile(`^[a-z]* [0-9: -]*`)

func (w *hsWorld) statusReason() (string, bool) {
	b, err := os.ReadFile(w.statusFile())
	if err != nil {
		return "", false
	}
	ls := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	for i := range ls {
		ls[i] = hsReasonRe.ReplaceAllString(ls[i], "")
	}
	return trimNL(strings.Join(ls, "\n")), true
}

// ── the master password's roads ─────────────────────────────────────────────

func (w *hsWorld) needSecurity() string {
	p := hsLookPath("security", w.env)
	if p == "" {
		w.die("the master password lives in the macOS Keychain and there is no security(1) here: use plain secret-unlock")
	}
	return p
}

func (w *hsWorld) keychainArg() []string {
	if k := w.getenv("HW_BW_KEYCHAIN"); k != "" {
		return []string{k}
	}
	return nil
}

func hsEsc(s string) string {
	return regexp.MustCompile(`[\\"]`).ReplaceAllString(s, `\$0`) // # MUTATION-ANCHOR: 1012-G07
}

// masterStore: the password goes in on STDIN of `security -i`, as a quoted command: never in argv.
func (w *hsWorld) masterStore(sec, pw string) bool {
	user := w.getenv("USER")
	if user == "" {
		o, _ := exec.Command("id", "-un").Output()
		user = trimNL(string(o))
	}
	kc := ""
	if k := w.getenv("HW_BW_KEYCHAIN"); k != "" {
		kc = ` "` + hsEsc(k) + `"`
	}
	cmd := fmt.Sprintf("add-generic-password -U -s %s -a %s -w \"%s\"%s\n", hsMasterService, user, hsEsc(pw), kc)
	c := exec.Command(sec, "-i") // # MUTATION-ANCHOR: 1012-G05
	c.Env = w.env
	c.Stdin = strings.NewReader(cmd)
	c.Run()
	meta := exec.Command(sec, append([]string{"find-generic-password", "-s", hsMasterService}, w.keychainArg()...)...) // metadata only
	meta.Env = w.env
	return meta.Run() == nil
}

func (w *hsWorld) masterRead(sec string) string {
	c := exec.Command(sec, append([]string{"find-generic-password", "-s", hsMasterService, "-w"}, w.keychainArg()...)...)
	c.Env = w.env
	o, err := c.Output()
	if err != nil {
		return ""
	}
	return trimNL(string(o))
}

func (w *hsWorld) masterForget(sec string) {
	c := exec.Command(sec, append([]string{"delete-generic-password", "-s", hsMasterService}, w.keychainArg()...)...)
	c.Env = w.env
	c.Run()
}

// cmdAuto: the Keychain item → bw unlock --passwordenv → the session file. It returns the status of the bash's.
func (w *hsWorld) cmdAuto(hours int64) int {
	bw := w.needBw()
	sec := w.needSecurity()
	out, _ := w.bw(bw, w.noSession(), "status")
	if hsStatus(out) == "unauthenticated" {
		return w.autoFail("bw is not logged in (bw login, once)")
	}
	pw := w.masterRead(sec)
	if pw == "" {
		return w.autoFail("no " + hsMasterService + " item in the Keychain (secret-unlock --setup, once)")
	}
	tokOut, rc := w.bw(bw, envSet(w.noSession(), "BW_HW_MASTER", pw), "unlock", "--passwordenv", "BW_HW_MASTER", "--raw") // # MUTATION-ANCHOR: 1012-G01
	tok := trimNL(tokOut)
	if rc != 0 {
		tok = ""
	}
	pw = ""
	if tok == "" {
		return w.autoFail("bw rejected the stored master password")
	}
	keep := w.out
	w.out = io.Discard
	w.sessionWrite(tok, hours)
	w.out = keep
	w.autoStatus("ok", "")
	fmt.Fprintln(w.out, "unlocked (auto)")
	return 0
}

// readLine is `IFS= read -r` (or `-rs`: the echo off when stdin is a terminal).
func (w *hsWorld) readLine(secret bool) string {
	if secret && w.tty {
		off := exec.Command("stty", "-echo")
		off.Stdin = os.Stdin
		if off.Run() == nil {
			defer func() {
				on := exec.Command("stty", "echo")
				on.Stdin = os.Stdin
				on.Run()
			}()
		}
	}
	var b []byte
	one := make([]byte, 1)
	for {
		n, err := w.in.Read(one)
		if n == 1 {
			if one[0] == '\n' {
				break
			}
			b = append(b, one[0])
		}
		if err != nil {
			break
		}
	}
	return string(b)
}

// cmdSetup: once, a person, a plain terminal.
func (w *hsWorld) cmdSetup() int {
	w.needBw()
	sec := w.needSecurity()
	fmt.Fprint(w.err, "Bitwarden master password (not echoed): ")
	pw := w.readLine(true)
	fmt.Fprint(w.err, "\n")
	fmt.Fprint(w.err, "Again: ")
	pw2 := w.readLine(true)
	fmt.Fprint(w.err, "\n")
	if pw == "" {
		w.die("an empty password was not stored")
	}
	if pw != pw2 { // # MUTATION-ANCHOR: 1012-G04
		w.die("the two entries differ: nothing was stored")
	}
	if !w.masterStore(sec, pw) {
		w.die("the Keychain refused the item")
	}
	pw, pw2 = "", ""
	keep := w.out
	w.out = io.Discard
	rc := w.cmdAuto(12)
	w.out = keep
	if rc != 0 {
		w.masterForget(sec) // # MUTATION-ANCHOR: 1012-G03
		why, _ := w.statusReason()
		w.die("bw did not accept that password (or is not logged in): the Keychain item was removed. " + why)
	}
	fmt.Fprintf(w.out, "stored as the Keychain service %s and unlocked once; the LaunchAgent (setup/install/install-bw-unlock-agent.sh) keeps it unlocked\n", hsMasterService)
	return 0
}

func (w *hsWorld) cmdUnlock(args []string) int {
	hours := "12"
	mode := "interactive"
	for len(args) > 0 {
		switch args[0] {
		case "--hours":
			if len(args) < 2 {
				w.die("--hours takes a number")
			}
			hours = args[1]
			args = args[2:]
		case "--lock":
			mode = "lock"
			args = args[1:]
		case "--auto":
			mode = "auto"
			args = args[1:]
		case "--setup":
			mode = "setup"
			args = args[1:]
		case "-h", "--help":
			fmt.Fprintf(w.out, "usage: secret-unlock [--hours N]   unlock Bitwarden (bw prompts for the master password); default 12 h\n       secret-unlock --lock       lock the vault and drop the session file\n       secret-unlock --setup      ONCE, a person: store the master password in the Keychain (service %s)\n       secret-unlock --auto       unlock from that item, no prompt (the LaunchAgent runs this every 11 h)\n", hsMasterService)
			return 0
		default:
			w.die("unknown option: " + args[0] + " (secret-unlock --help)")
		}
	}
	bw := w.needBw()
	if mode == "lock" {
		w.bw(bw, w.noSession(), "lock")
		os.Remove(w.sessionFile())
		fmt.Fprint(w.out, "locked: vault locked, session file removed\n")
		return 0
	}
	h, err := strconv.ParseInt(hours, 10, 64)
	if hours == "" || strings.Trim(hours, "0123456789") != "" || hours == "0" || (err == nil && h > 168) || err != nil {
		w.die("--hours takes a whole number of hours, 1 to 168 (got: " + hours + ")")
	}
	switch mode {
	case "auto":
		return w.cmdAuto(h)
	case "setup":
		return w.cmdSetup()
	}
	out, _ := w.bw(bw, w.noSession(), "status")
	if hsStatus(out) == "unauthenticated" {
		w.die("bw is not logged in: run `bw login` once, then secret-unlock")
	}
	// bw prompts for the master password on the terminal itself and, with --raw, prints only the token on stdout.
	c := exec.Command(bw, "unlock", "--raw")
	c.Env = w.noSession()
	c.Stdin, c.Stderr = os.Stdin, w.err
	var o bytes.Buffer
	c.Stdout = &o
	if c.Run() != nil {
		w.die("bw unlock failed (wrong master password?)")
	}
	tok := trimNL(o.String())
	if tok == "" {
		w.die("bw unlock returned no session")
	}
	w.sessionWrite(tok, h)
	return 0
}

// ── resolving an item inside the folder `agents` ────────────────────────────

func (w *hsWorld) folderID(bw string) string {
	out, _ := w.bw(bw, w.env, "list", "folders", "--search", hsAgentsFolder)
	return hsFirstID(out, hsAgentsFolder)
}

// itemID: the id of the item NAME in the folder; 0 found, 1 absent, 3 ambiguous (two items, one name).
func (w *hsWorld) itemID(bw, name string) (string, int) {
	fid := w.folderID(bw)
	if fid == "" {
		return "", 1
	}
	listing, _ := w.bw(bw, w.env, "list", "items", "--folderid", fid, "--search", name) // # MUTATION-ANCHOR: 1009-G02
	ids := hsIDs(listing, name)
	if ids == "" {
		return "", 1
	}
	if strings.Contains(ids, "\n") { // # MUTATION-ANCHOR: 1009-G08
		return "", 3
	}
	return ids, 0
}

type hsRes struct{ id, field, name string }

// resolve: the whole spec is tried as an item name first; failing that, the text after its LAST dot is the field.
func (w *hsWorld) resolve(bw, spec string) (hsRes, int) {
	r := hsRes{field: "password", name: spec}
	id, rc := w.itemID(bw, spec)
	r.id = id
	if rc == 1 && strings.Contains(spec, ".") {
		i := strings.LastIndex(spec, ".")
		r.name, r.field = spec[:i], spec[i+1:]
		if r.name == "" || r.field == "" {
			return r, 1
		}
		r.id, rc = w.itemID(bw, r.name)
	}
	return r, rc
}

// fieldValue is the only place a value is read, into the string the caller puts in a child's environment.
func (w *hsWorld) fieldValue(bw, id, field string) string {
	if field == "totp" {
		out, rc := w.bw(bw, w.env, "get", "totp", id)
		if rc != 0 {
			return ""
		}
		return trimNL(out)
	}
	out, rc := w.bw(bw, w.env, "get", "item", id)
	if rc != 0 {
		return ""
	}
	v, ok := hsFieldRaw(out, field)
	if !ok {
		return ""
	}
	return trimNL(v)
}

func hsValidSpec(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && !strings.Contains(s, "\n")
}

// ── hw secret check <item>… ─────────────────────────────────────────────────

func (w *hsWorld) cmdCheck(args []string) int {
	if len(args) < 1 {
		w.die("usage: hw secret check <item>[.<field>]…")
	}
	bw := w.needBw()
	if !w.sessionLoad(bw) {
		w.note("the vault is LOCKED, so nothing can be checked: run secret-unlock in a terminal")
		return 2
	}
	rc := 0
	for _, spec := range args {
		if !hsValidSpec(spec) {
			w.die("not an item name: " + spec)
		}
		res, r := w.resolve(bw, spec)
		switch r {
		case 0:
			// presence of the item AND of the field
			present := false
			if res.field == "totp" {
				_, c := w.bw(bw, w.env, "get", "totp", res.id)
				present = c == 0
			} else if out, c := w.bw(bw, w.env, "get", "item", res.id); c == 0 {
				present = hsFieldPresent(out, res.field)
			}
			if present {
				fmt.Fprintf(w.out, "  ok  %s — present in %s\n", spec, hsAgentsFolder)
			} else {
				fmt.Fprintf(w.out, "  !!  %s — the item is there but its field \"%s\" is absent or empty\n", spec, res.field)
				rc = 1
			}
		case 3:
			fmt.Fprintf(w.out, "  !!  %s — AMBIGUOUS: two items in %s carry that name\n", spec, hsAgentsFolder)
			rc = 1
		default:
			fmt.Fprintf(w.out, "  !!  %s — ABSENT from the Bitwarden folder %s\n", spec, hsAgentsFolder)
			rc = 1
		}
	}
	if rc == 0 {
		fmt.Fprint(w.out, "  only presence was checked — no value was read, printed or logged.\n")
	}
	return rc
}

// ── hw secret status | ensure ───────────────────────────────────────────────

func (w *hsWorld) cmdStatus() int {
	bw := w.needBw()
	if !w.sessionLoad(bw) {
		fmt.Fprint(w.out, "locked\n")
		return 1
	}
	b, _ := os.ReadFile(w.sessionFile())
	exp := strings.TrimPrefix(strings.SplitN(string(b), "\n", 2)[0], "expires=")
	fmt.Fprintf(w.out, "unlocked until %s\n", hsUntil(exp))
	return 0
}

// cmdEnsure: unlocked, or ONE `secret-unlock --auto` try (the Keychain's master password, no prompt) before refusing.
func (w *hsWorld) cmdEnsure() int {
	bw := w.needBw()
	if w.sessionLoad(bw) {
		return 0
	}
	w.note("the vault is locked: one secret-unlock --auto try (the master password comes from the Keychain)")
	keep := w.out
	w.out = io.Discard
	w.quietly(func() { w.cmdAuto(12) }) // # MUTATION-ANCHOR: 1012-G06
	w.out = keep
	if w.sessionLoad(bw) {
		return 0
	}
	why, ok := w.statusReason()
	if !ok {
		why = "no reason recorded"
	}
	w.note("the vault is still locked (" + why + "). A person, once: secret-unlock --setup")
	return 1
}

// ── hw secret run ───────────────────────────────────────────────────────────

const hsRunUsage = "usage: hw secret run <item>[.<field>] --as VAR [--item <item> --as VAR …] -- <cmd…>"

func hsVarName(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		letter := c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func hsRefusedVar(v string) bool {
	switch v {
	case "PATH", "HOME", "IFS", "ENV", "BASH_ENV", "SHELLOPTS", "BASHOPTS", "PS4":
		return true
	}
	return strings.HasPrefix(v, "BW_") || strings.HasPrefix(v, "LD_") || strings.HasPrefix(v, "DYLD_")
}

func (w *hsWorld) cmdRun(args []string) int {
	pending := ""
	var specs, vars, values []string
	for len(args) > 0 {
		a := args[0]
		if a == "--" {
			args = args[1:]
			break
		}
		switch {
		case a == "--item":
			if len(args) < 2 {
				w.die(hsRunUsage)
			}
			if pending != "" {
				w.die("the item \"" + pending + "\" has no --as")
			}
			pending = args[1]
			args = args[2:]
		case a == "--as":
			if len(args) < 2 {
				w.die(hsRunUsage)
			}
			if pending == "" {
				w.die("--as " + args[1] + " has no item before it")
			}
			specs, vars, pending = append(specs, pending), append(vars, args[1]), ""
			args = args[2:]
		case strings.HasPrefix(a, "-"):
			w.die("unknown option: " + a)
		default:
			if pending != "" || len(specs) != 0 {
				w.die("unexpected argument \"" + a + "\": a second item is --item <item> --as VAR")
			}
			pending = a
			args = args[1:]
		}
	}
	if pending != "" {
		w.die("the item \"" + pending + "\" has no --as")
	}
	if len(specs) < 1 || len(args) < 1 {
		w.die(hsRunUsage)
	}
	for i, spec := range specs {
		v := vars[i]
		if !hsValidSpec(spec) {
			w.die("not an item name: " + spec)
		}
		if !hsVarName(v) {
			w.die("--as " + v + " is not an environment variable name")
		}
		if hsRefusedVar(v) { // # MUTATION-ANCHOR: 1009-G05
			w.die("--as " + v + " is refused: a secret may not replace that variable")
		}
		for _, p := range vars[:i] {
			if p == v {
				w.die("--as " + v + " is used twice")
			}
		}
	}
	bw := w.needBw()
	if !w.sessionLoad(bw) { // # MUTATION-ANCHOR: 1009-G09
		w.die("the vault is LOCKED or its session expired: the brainer unlocks it (secret-unlock); report this blocked")
	}
	for _, spec := range specs {
		res, rc := w.resolve(bw, spec)
		switch rc {
		case 0:
		case 3:
			w.die("AMBIGUOUS: two items in the folder " + hsAgentsFolder + " carry the name " + spec)
		default:
			w.die(spec + " is not in the Bitwarden folder " + hsAgentsFolder)
		}
		v := w.fieldValue(bw, res.id, res.field)
		if v == "" { // # MUTATION-ANCHOR: 1009-G10
			w.die(res.name + "." + res.field + " is absent or empty")
		}
		values = append(values, v) // # MUTATION-ANCHOR: 1009-G04
	}
	// The child gets the values and NOT the vault: every BW_* goes first.
	env := envWithout(w.env, func(n string) bool { return strings.HasPrefix(n, "BW_") }) // # MUTATION-ANCHOR: 1009-G03
	for i, v := range vars {
		env = envSet(env, v, values[i])
	}
	rc, why := w.execChild(args, env)
	if why != "" { // it returns only when it could not exec
		fmt.Fprintf(w.err, "hw secret: exec: %s: %s\n", args[0], why)
	}
	return rc
}

// ── the program ─────────────────────────────────────────────────────────────

func runHwSecret(args []string, w *hsWorld) (rc int) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(hsExit)
			if !ok {
				panic(r)
			}
			rc = e.rc
		}
	}()
	verb := ""
	if len(args) > 0 {
		verb = args[0]
		args = args[1:]
	}
	switch verb {
	case "run":
		return w.cmdRun(args)
	case "check":
		return w.cmdCheck(args)
	case "status":
		return w.cmdStatus()
	case "ensure":
		return w.cmdEnsure()
	case "unlock":
		return w.cmdUnlock(args)
	case "", "-h", "--help", "help":
		fmt.Fprint(w.out, "usage: hw secret run <item>[.<field>] --as VAR [--item … --as …] -- <cmd…>\n       <cmd that prints a value> | hw secret put <item>[.<field>] [--replace]\n       hw secret check <item>[.<field>]…\n       hw secret status | ensure\n       secret-unlock [--hours N | --lock]   (a person, in a terminal, once a day)\n")
		return 0
	}
	w.die("unknown verb: " + verb + " (run, put, check, status, ensure)")
	return 1
}

// hwSecretMain is the hw-core entry: the real process, with the bash's `umask 077` (a file the child creates is 0600).
func hwSecretMain(args []string, stdout, stderr *os.File) int {
	hsUmask077()
	return runHwSecret(args, &hsWorld{
		out: stdout, err: stderr, in: os.Stdin, env: os.Environ(), now: time.Now,
		execChild: hsExecProcess, tty: isTerminal(os.Stdin),
	})
}
