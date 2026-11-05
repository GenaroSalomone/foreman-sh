package main

// hw-secret in Go: `hw-core hw-secret <verb> …` is bin/hw-secret, the one path from Bitwarden to an executor, and it
// never prints a value. 0.7.0 removed the bash body: this is the only implementation. setup/tests/1009 and 1012 drive
// the real bin/hw-secret through its shim, and 1077 compares it with the transcripts the bash recorded.
//
// SAME WORDS, STREAMS, EXIT STATUS AND FILES the bash answered — one place differs on purpose, and one is not
// reproducible:
//   - the bash needs jq (`jq is required`); this reads bw's JSON itself, so a machine with no jq works;
//   - `exec: <cmd>: not found` carries the bash's `<script path>: line N:` prefix there and does not here.
//
// The value of an item is held in one Go string, put in the CHILD's environment and exec'd over this process: it is
// never written to stdout, stderr or disk, and bw's stderr is dropped. `put` builds the item JSON here and sends it to
// bw's stdin base64-encoded; argv carries ids and names, never a value.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
	o, _, rc := w.bwStderr(bw, env, args...)
	return o, rc
}

// bwStderr is bw that also hands back stderr. Only --auto's unlock reads it, and only to log it through redact.
func (w *hsWorld) bwStderr(bw string, env []string, args ...string) (string, string, int) {
	c := exec.Command(bw, args...)
	c.Env = env
	var o, e bytes.Buffer
	c.Stdout, c.Stderr = &o, &e
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return o.String(), e.String(), ee.ExitCode()
		}
		return o.String(), e.String(), -1
	}
	return o.String(), e.String(), 0
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

// sessionExp: the live session file's expiry (epoch seconds), 0 when there is none.
func (w *hsWorld) sessionExp() int64 {
	b, err := os.ReadFile(w.sessionFile())
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimPrefix(strings.SplitN(string(b), "\n", 2)[0], "expires="), 10, 64)
	return n
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

// autoLog appends one timestamped line to bw-unlock.log (0600). The LaunchAgent AND every `ensure`/`check` try write it,
// so the file is the whole history of --auto; callers pass only text that went through redact.
func (w *hsWorld) autoLog(msg string) {
	dir := w.secretDir()
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o700)
	f, err := os.OpenFile(dir+"/bw-unlock.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", w.now().Format(time.RFC3339), msg)
}

var (
	hsBWAssign = regexp.MustCompile(`(?i)\bBW_[A-Z0-9_]*\s*[=:]\s*\S+`)
	hsSession  = regexp.MustCompile(`(?i)--session\s+\S+`)
	hsLongTok  = regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`)
)

// redact makes bw's stderr safe for the log: the given secrets and the value (4+ bytes) of every BW_* variable in the environment are
// replaced first (whatever their shape), then BW_*=… assignments, --session arguments and any long token-looking run;
// whitespace collapses to single spaces and the text stops at 300 runes.
func (w *hsWorld) redact(s string, secrets ...string) string {
	for _, e := range w.env {
		if k, v, ok := strings.Cut(e, "="); ok && strings.HasPrefix(k, "BW_") && len(v) >= 4 {
			secrets = append(secrets, v)
		}
	}
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	s = hsBWAssign.ReplaceAllString(s, "BW_…=[redacted]")
	s = hsSession.ReplaceAllString(s, "--session [redacted]")
	s = hsLongTok.ReplaceAllString(s, "[redacted]")
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return s
}

func (w *hsWorld) autoFail(why string, detail ...string) int {
	w.autoStatus("failed", why) // # MUTATION-ANCHOR: 1012-G02
	w.note("secret-unlock --auto failed: " + why)
	log := "failed: " + why
	if len(detail) > 0 && detail[0] != "" {
		log += " (bw said: " + detail[0] + ")"
	}
	w.autoLog(log)
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
	tokOut, errOut, rc := w.bwStderr(bw, envSet(w.noSession(), "BW_HW_MASTER", pw), "unlock", "--passwordenv", "BW_HW_MASTER", "--raw") // # MUTATION-ANCHOR: 1012-G01
	tok := trimNL(tokOut)
	if rc != 0 {
		tok = ""
	}
	if tok == "" {
		return w.autoFail("bw rejected the stored master password", w.redact(errOut, pw, tokOut))
	}
	pw = ""
	keep := w.out
	w.out = io.Discard
	w.sessionWrite(tok, hours)
	w.out = keep
	w.autoStatus("ok", "")
	w.autoLog(fmt.Sprintf("ok: unlocked, new %d h session", hours))
	fmt.Fprintln(w.out, "unlocked (auto)")
	return 0
}

// autoFresh: --auto from the LaunchAgent runs every hour, and a live session with more than half its life left needs no
// `bw unlock`: it says so, and logs it. Anything shorter-lived is renewed, so a session never reaches a night's sleep near its end.
func (w *hsWorld) autoFresh(bw string, hours int64) bool {
	if !w.sessionLoad(bw) {
		return false
	}
	left := w.sessionExp() - w.now().Unix()
	if left <= hours*3600/2 { // the installer's hourly slot renews inside this margin
		return false
	}
	w.autoStatus("ok", "")
	msg := fmt.Sprintf("noop: already unlocked, %d min left (renews under %d min); no bw unlock", left/60, hours*60/2)
	w.autoLog(msg)
	fmt.Fprintln(w.out, msg)
	return true
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
	fmt.Fprintf(w.out, "stored as the Keychain service %s and unlocked once; a scheduled `secret-unlock --auto` keeps it unlocked\n", hsMasterService)
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
			fmt.Fprintf(w.out, "usage: secret-unlock [--hours N]   unlock Bitwarden (bw prompts for the master password); default 12 h\n       secret-unlock --lock       lock the vault and drop the session file\n       secret-unlock --setup      ONCE, a person: store the master password in the Keychain (service %s)\n       secret-unlock --auto       unlock from that item, no prompt (a job you schedule runs this hourly; a session with over half its life left is left alone)\n", hsMasterService)
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
		rc := 0
		if !w.autoFresh(bw, h) {
			rc = w.cmdAuto(h)
		}
		w.vaultAlert(rc)
		return rc
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
	if !w.sessionOrAuto(bw) {
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

// sessionOrAuto: a live session, or ONE `secret-unlock --auto` try (the Keychain's master password, no prompt). `ensure`
// and `check` share it, so no verb refuses a locked vault the Keychain could have opened.
func (w *hsWorld) sessionOrAuto(bw string) bool {
	if w.sessionLoad(bw) {
		return true
	}
	w.note("the vault is locked: one secret-unlock --auto try (the master password comes from the Keychain)")
	keep := w.out
	w.out = io.Discard
	w.quietly(func() { w.cmdAuto(12) }) // # MUTATION-ANCHOR: 1012-G06
	w.out = keep
	return w.sessionLoad(bw)
}

// cmdEnsure: unlocked, or ONE `secret-unlock --auto` try before refusing.
func (w *hsWorld) cmdEnsure() int {
	bw := w.needBw()
	if w.sessionOrAuto(bw) {
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

// ── hw secret put, the way in ───────────────────────────────────────────────

const hsPutUsage = "usage: <command that prints the value> | hw secret put <item>[.<field>] [--replace]   (the value comes from a pipe on stdin, never from argv)"

// hsStdinTerminal is `[ -t 0 ]`; a reader that is not a file (an in-process test) is not a terminal.
func (w *hsWorld) hsStdinTerminal() bool {
	f, ok := w.in.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// hsStdinPipe is `[ -p /dev/stdin ]`: a FIFO or a socket, never a regular file and never /dev/null's char device.
func (w *hsWorld) hsStdinPipe() bool {
	f, ok := w.in.(*os.File)
	if !ok {
		return true
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeNamedPipe != 0 || fi.Mode()&os.ModeSocket != 0
}

// bwStdin is `printf '%s' "$enc" | bw …`, with stderr dropped, as the bash's writes are.
func (w *hsWorld) bwStdin(bw, input string, env []string, args ...string) int {
	c := exec.Command(bw, args...)
	c.Env = env
	c.Stdin = strings.NewReader(input)
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return -1
	}
	return 0
}

func hsItemJSON(s string) map[string]any {
	var o map[string]any
	if json.Unmarshal([]byte(s), &o) != nil {
		return nil
	}
	return o
}

func hsLoginOf(o map[string]any) (map[string]any, bool) {
	l, ok := o["login"]
	if !ok || l == nil {
		return nil, false
	}
	m, ok := l.(map[string]any)
	return m, ok
}

func hsHasField(o map[string]any, field string) bool {
	fs, _ := o["fields"].([]any)
	for _, f := range fs {
		if m, ok := f.(map[string]any); ok {
			if n, _ := m["name"].(string); n == field {
				return true
			}
		}
	}
	return false
}

func (w *hsWorld) cmdPut(args []string) int {
	spec, replace := "", false
	for _, a := range args {
		switch {
		case a == "--replace":
			replace = true
		case strings.HasPrefix(a, "-"):
			w.die("unknown option (put takes only --replace; a value is never an argument: pipe it on stdin)")
		case spec != "":
			w.die("a second argument: a value never travels in argv, pipe it on stdin (put takes the item and --replace)")
		default:
			spec = a
		}
	}
	if spec == "" {
		w.die(hsPutUsage)
	}
	if !hsValidSpec(spec) {
		w.die("not an item name: " + spec)
	}
	// stdin must be a pipe: nobody pastes a secret into a pane, and a file or /dev/null is not a producer
	if w.hsStdinTerminal() {
		w.die("stdin is a terminal: the value comes from a pipe (<command> | hw secret put " + spec + "), never typed or pasted into a pane")
	}
	if !w.hsStdinPipe() { // # MUTATION-ANCHOR: 1009-G11
		w.die("stdin is not a pipe: the value comes from <command> | hw secret put " + spec)
	}
	b, _ := io.ReadAll(w.in)
	value := strings.TrimSuffix(string(b), "\n") // # MUTATION-ANCHOR: 1009-G12
	if value == "" { // # MUTATION-ANCHOR: 1009-G13
		w.die("stdin was empty: nothing was stored (did the command before the pipe fail?)")
	}
	bw := w.needBw()
	if !w.sessionLoad(bw) {
		w.die("the vault is LOCKED or its session expired: the brainer unlocks it (secret-unlock); report this blocked")
	}
	folder := w.folderID(bw)
	if folder == "" {
		w.die("there is no Bitwarden folder " + hsAgentsFolder + ": a person creates it once in the vault")
	}
	id, name, field := "", spec, "password"
	res, rc := w.resolve(bw, spec)
	switch rc {
	case 0:
		id, name, field = res.id, res.name, res.field
	case 3:
		w.die("AMBIGUOUS: two items in the folder " + hsAgentsFolder + " carry the name " + spec)
	default:
		// no such item: a dotted spec names <item>.<field>, a plain one the password
		if i := strings.LastIndex(spec, "."); i >= 0 {
			name, field = spec[:i], spec[i+1:]
			if name == "" || field == "" {
				w.die("not an item name: " + spec)
			}
		}
		id, rc = w.itemID(bw, name)
		if rc == 3 {
			w.die("AMBIGUOUS: two items in the folder " + hsAgentsFolder + " carry the name " + name)
		}
	}
	switch field {
	case "username", "notes", "uri", "totp":
		w.die(field + " is a built-in field, not a secret: put stores the password or a custom hidden field")
	}
	verb, have := "stored", false
	obj := map[string]any(nil)
	if id != "" {
		if out, c := w.bw(bw, w.env, "get", "item", id); c == 0 {
			obj = hsItemJSON(out)
		}
		// the edit sends the whole item back: without it, a write would wipe the rest
		if obj == nil { // # MUTATION-ANCHOR: 1009-G15
			w.die("bw could not read " + name + ", so nothing was changed (is the vault still unlocked? hw secret status)")
		}
		if field == "password" {
			login, ok := hsLoginOf(obj)
			if !ok {
				w.die(name + " is not a login item: put cannot give it a password")
			}
			if p, _ := login["password"].(string); p != "" {
				have = true
			}
		} else {
			have = hsHasField(obj, field)
		}
		if have {
			if !replace { // # MUTATION-ANCHOR: 1009-G14
				w.die(spec + " already exists: nothing was changed (--replace overwrites it)")
			}
			verb = "replaced"
		}
	}
	var out map[string]any
	if id == "" {
		out = map[string]any{
			"organizationId": nil, "collectionIds": nil, "folderId": folder, "type": 1,
			"name": name, "notes": nil, "favorite": false, "reprompt": 0,
			"login":  map[string]any{"uris": []any{}, "username": nil, "password": nil, "totp": nil},
			"fields": []any{},
		}
	} else {
		out = obj
		if out == nil {
			out = map[string]any{}
		}
	}
	if field == "password" {
		login, _ := hsLoginOf(out)
		if login == nil {
			login = map[string]any{}
			out["login"] = login
		}
		login["password"] = value
	} else {
		var fields []any
		if fs, ok := out["fields"].([]any); ok {
			for _, f := range fs {
				if m, ok := f.(map[string]any); ok {
					if n, _ := m["name"].(string); n == field {
						continue
					}
				}
				fields = append(fields, f)
			}
		}
		out["fields"] = append(fields, map[string]any{"name": field, "value": value, "type": 1})
	}
	js, err := json.Marshal(out)
	if err != nil {
		w.die("bw refused the write, so nothing was changed (is the vault still unlocked? hw secret status)")
	}
	enc := base64.StdEncoding.EncodeToString(js)
	rc = -1
	if id != "" {
		rc = w.bwStdin(bw, enc, w.env, "edit", "item", id)
	} else {
		rc = w.bwStdin(bw, enc, w.env, "create", "item")
	}
	if rc != 0 { // # MUTATION-ANCHOR: 1009-G16
		w.die("bw refused the write, so nothing was changed (is the vault still unlocked? hw secret status)")
	}
	fmt.Fprintf(w.out, "%s %s\n", verb, spec)
	return 0
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
	case "put":
		return w.cmdPut(args)
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

// ── the alert: a person must learn the vault is going to lock ───────────────

const hsAlertWindow = 2 * 3600 // a session ending within this is reported even when the last --auto succeeded

func (w *hsWorld) alertFile(name string) string { return w.secretDir() + "/" + name }

func (w *hsWorld) alertLog(msg string) {
	os.MkdirAll(w.secretDir(), 0o700)
	if f, err := os.OpenFile(w.alertFile("vault-alerts.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintf(f, "%s %s\n", w.now().Format(time.RFC3339), msg)
		f.Close()
	}
}

// vaultAlert runs after the LaunchAgent's --auto (never after ensure/check, which a dispatch drives). A failed last
// outcome, or a session ending within two hours, is ONE alert per episode: the state file names the kind being alerted,
// `hw status` prints it, and it is removed (and a log line says so) the first run that finds the vault healthy again.
func (w *hsWorld) vaultAlert(rc int) {
	left := w.sessionExp() - w.now().Unix()
	kind, detail := "", ""
	switch {
	case rc != 0:
		why, _ := w.statusReason()
		kind, detail = "failed", "--auto failed: "+why
	case left > 0 && left <= hsAlertWindow:
		kind, detail = "expiring", fmt.Sprintf("session ends in %d min", left/60)
	}
	state, sent := w.alertFile("vault-alert-state"), w.alertFile("vault-alert-sent")
	prev, _ := os.ReadFile(state)
	if kind == "" {
		if len(prev) > 0 {
			os.Remove(state)
			os.Remove(sent)
			w.alertLog("recovered: the vault is unlocked with a healthy session")
		}
		return
	}
	if got, _ := os.ReadFile(sent); string(got) == kind {
		return // this episode was already reported
	}
	if strings.HasPrefix(string(prev), kind+" ") {
		w.alertLog("retrying the report of the open " + kind + " alert") // the last send never landed
	} else {
		line := kind + " " + w.now().Format("2006-01-02 15:04:05") + " — " + detail
		os.MkdirAll(w.secretDir(), 0o700)
		os.WriteFile(state, []byte(line+"\n"), 0o600)
		os.Remove(sent)
		w.alertLog("ALERT " + detail)
	}
	lane := w.getenv("HW_VAULT_ALERT_TO")
	if lane == "" {
		lane = "setup"
	}
	pane := w.brainerPane(lane)
	if pane == "" {
		w.alertLog("send skipped: no single herdr pane labelled brain in the " + lane + " lane directory")
		return
	}
	msg := "vault: " + detail + ". Log: " + w.alertFile("vault-alerts.log") + ". A person, once, in a terminal: secret-unlock (or secret-unlock --setup)."
	cs := w.getenv("HW_CHANNEL_SEND")
	if cs == "" {
		cs = hsLookPath("channel-send", w.env)
	}
	if cs == "" {
		w.alertLog("send skipped: no channel-send")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second) // channel-send's own wait for a busy pane is 90 s
	defer cancel()
	c := exec.CommandContext(ctx, cs, "--report", "herdr", pane, "-", "-")
	c.Env = w.alertEnv()
	c.Stdin = strings.NewReader(msg)
	if err := c.Run(); err != nil {
		w.alertLog("send to " + pane + " failed: " + err.Error()) // the log and the status line stay the record
		return
	}
	os.WriteFile(sent, []byte(kind), 0o600)
	w.alertLog("reported to " + pane)
}

// alertEnv is what herdr and channel-send get: no vault session (they never need it), and the herdr socket a LaunchAgent
// does not inherit (bin/herdr-rpc exits 3 without HERDR_SOCKET_PATH).
func (w *hsWorld) alertEnv() []string {
	env := w.noSession()
	if w.getenv("HERDR_SOCKET_PATH") == "" {
		env = envSet(env, "HERDR_SOCKET_PATH", w.getenv("HOME")+"/.config/herdr/herdr.sock")
	}
	return env
}

// brainerPane: the one herdr pane labelled brain whose cwd is the lane's brain directory, "" when none or many. Pane ids
// change with every relaunch, so it is resolved at send time with brainerVerdict, the predicate bin/brain launches by.
func (w *hsWorld) brainerPane(lane string) string {
	root := w.getenv("HW_BRAIN_ROOT")
	if root == "" { // the agent's plist sets it; a bare run has no brain to look in
		return ""
	}
	hp := hsLookPath("herdr", w.env)
	if hp == "" {
		return ""
	}
	c := exec.Command(hp, "pane", "list")
	c.Env = w.alertEnv()
	out, err := c.Output()
	if err != nil {
		return ""
	}
	v, ok := bjParse(string(out))
	if !ok {
		return ""
	}
	list, _ := bjGet(v, "result", "panes").([]any)
	var hits []string
	for _, e := range list {
		if p, isMap := e.(jmap); isMap {
			if id, isStr := p["pane_id"].(string); isStr && brainerVerdict(string(out), id, root+"/"+lane) == "ok" {
				hits = append(hits, id)
			}
		}
	}
	if len(hits) != 1 {
		return ""
	}
	return hits[0]
}
