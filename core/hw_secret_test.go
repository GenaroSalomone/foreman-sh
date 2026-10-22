package main

// hw-secret in Go, in process. The throwaways are the same the bash is tested with: the bw stub of
// setup/tests/fixtures/bw-stub (which needs jq) and the security(1) stub of setup/tests/hw-secret-parity (python3). No
// real vault, no Keychain; every value here is a fixture. The mutants of setup/tests/1078 are judged by the
// TestHwSecret* tests of this file (and by nothing else: they build with this file alone).

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type hsT struct {
	t                *testing.T
	w                *hsWorld
	out, err         *bytes.Buffer
	home, state, sec string
	execs            [][]string // argv of each exec the program asked for
	envs             [][]string
	stubBin          string
}

func hsCopy(t *testing.T, from, to string) {
	b, err := os.ReadFile(from)
	if err != nil {
		t.Skipf("no %s here: %v", from, err)
	}
	if err := os.WriteFile(to, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func newHsT(t *testing.T) *hsT {
	t.Helper()
	for _, tool := range []string{"jq", "python3"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s: the stubs need it", tool)
		}
	}
	dir := t.TempDir()
	h := &hsT{t: t, home: filepath.Join(dir, "home"), state: filepath.Join(dir, "bwstate"), sec: filepath.Join(dir, "secstate"), stubBin: filepath.Join(dir, "bin")}
	for _, d := range []string{h.home, h.state, h.sec, h.stubBin} {
		os.MkdirAll(d, 0o755)
	}
	hsCopy(t, "../setup/tests/fixtures/bw-stub/bw", filepath.Join(h.stubBin, "bw"))
	hsCopy(t, "../setup/tests/hw-secret-parity/security", filepath.Join(h.stubBin, "security"))
	os.WriteFile(filepath.Join(h.state, "locked"), nil, 0o644)
	h.out, h.err = &bytes.Buffer{}, &bytes.Buffer{}
	h.w = &hsWorld{
		out: h.out, err: h.err, in: strings.NewReader(""), now: time.Now,
		env: []string{
			"HOME=" + h.home, "PATH=" + h.stubBin + ":" + os.Getenv("PATH"), "USER=t",
			"HW_BW_BIN=" + filepath.Join(h.stubBin, "bw"), "BW_STUB_STATE=" + h.state, "SEC_STATE=" + h.sec,
			"HW_BW_KEYCHAIN=" + filepath.Join(dir, "never.keychain-db"),
		},
		execChild: func(argv, env []string) (int, string) {
			h.execs = append(h.execs, argv)
			h.envs = append(h.envs, env)
			return 0, ""
		},
	}
	return h
}

func (h *hsT) run(args ...string) int {
	h.out.Reset()
	h.err.Reset()
	return runHwSecret(args, h.w)
}

func (h *hsT) sessionPath() string { return filepath.Join(h.home, ".cache", "hw", "bw-session") }

// session writes the file an earlier unlock left, expiring offset seconds from now, and opens (offset > 0) the vault.
func (h *hsT) session(offset int64, tok string) {
	if err := os.MkdirAll(filepath.Dir(h.sessionPath()), 0o700); err != nil {
		h.t.Fatal(err)
	}
	os.WriteFile(h.sessionPath(), []byte("expires="+strconv.FormatInt(time.Now().Unix()+offset, 10)+"\n"+tok+"\n"), 0o600)
}

func (h *hsT) unlocked() {
	os.Remove(filepath.Join(h.state, "locked"))
	h.session(43200, "TOK-STUB-123")
}

func (h *hsT) bwCalls() string {
	b, _ := os.ReadFile(filepath.Join(h.state, "calls"))
	return string(b)
}

func (h *hsT) secCalls() string {
	b, _ := os.ReadFile(filepath.Join(h.sec, "log"))
	return string(b)
}

func (h *hsT) item() (string, bool) {
	b, err := os.ReadFile(filepath.Join(h.sec, "item"))
	return string(b), err == nil
}

func (h *hsT) mustRC(want int, args ...string) {
	h.t.Helper()
	if rc := h.run(args...); rc != want {
		h.t.Fatalf("hw secret %v exited %d, not %d\nstdout: %s\nstderr: %s", args, rc, want, h.out, h.err)
	}
}

func (h *hsT) envOf(i int) map[string]string {
	m := map[string]string{}
	for _, kv := range h.envs[i] {
		if j := strings.IndexByte(kv, '='); j >= 0 {
			m[kv[:j]] = kv[j+1:]
		}
	}
	return m
}

// ── run: the value reaches the child and nobody else ────────────────────────

func TestHwSecretRunDelivers(t *testing.T) {
	h := newHsT(t)
	h.unlocked()
	h.w.env = append(h.w.env, "BW_CLIENTSECRET=cs", "KEEP_ME=1")
	h.mustRC(0, "run", "github-token", "--as", "GH", "--", "the-command", "arg")
	if strings.Contains(h.out.String()+h.err.String(), "S3CRET") {
		t.Fatalf("the value leaked to hw secret run's own output: %q %q", h.out, h.err)
	}
	if h.out.Len() != 0 {
		t.Fatalf("hw secret run printed on stdout: %q", h.out)
	}
	if len(h.execs) != 1 || strings.Join(h.execs[0], " ") != "the-command arg" {
		t.Fatalf("the command was not exec'd as given: %v", h.execs)
	}
	env := h.envOf(0)
	if env["GH"] != "S3CRET-GH" {
		t.Fatalf("the value did not reach the child's environment: %q", env["GH"])
	}
	for k := range env {
		if strings.HasPrefix(k, "BW_") {
			t.Fatalf("the child inherited %s", k)
		}
	}
	if env["KEEP_ME"] != "1" {
		t.Fatalf("the rest of the environment did not reach the child")
	}
	if strings.Contains(h.bwCalls(), "S3CRET") || strings.Contains(h.bwCalls(), "TOK-STUB") {
		t.Fatalf("bw's argv carried a value or the token: %s", h.bwCalls())
	}
}

func TestHwSecretRunFields(t *testing.T) {
	h := newHsT(t)
	h.unlocked()
	h.mustRC(0, "run", "github-token.username", "--as", "U", "--item", "github-token.api_key", "--as", "K", "--item", "github-token.notes", "--as", "N",
		"--item", "github-token.uri", "--as", "P", "--item", "github-token.totp", "--as", "T", "--item", "db.prod", "--as", "D", "--", "c")
	env := h.envOf(0)
	for k, want := range map[string]string{"U": "ghuser", "K": "S3CRET-API", "N": "S3CRET-NOTE", "P": "https://github.com", "T": "123456", "D": "S3CRET-DB"} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
}

func TestHwSecretRunRefusesVariables(t *testing.T) {
	for _, v := range []string{"PATH", "HOME", "IFS", "ENV", "BASH_ENV", "SHELLOPTS", "BASHOPTS", "PS4", "BW_SESSION", "BW_X", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES"} {
		h := newHsT(t)
		h.unlocked()
		if rc := h.run("run", "github-token", "--as", v, "--", "c"); rc == 0 || len(h.execs) != 0 {
			t.Fatalf("--as %s was accepted (rc=%d, execs=%v)", v, rc, h.execs)
		}
		if !strings.Contains(h.err.String(), "is refused: a secret may not replace that variable") {
			t.Fatalf("--as %s did not end in the refusal: %q", v, h.err)
		}
	}
	for _, v := range []string{"9X", "A-B", "", "A B"} {
		h := newHsT(t)
		h.unlocked()
		if rc := h.run("run", "github-token", "--as", v, "--", "c"); rc != 1 || len(h.execs) != 0 || !strings.Contains(h.err.String(), "is not an environment variable name") {
			t.Fatalf("--as %q: rc=%d execs=%v err=%q", v, rc, h.execs, h.err)
		}
	}
	h := newHsT(t)
	h.unlocked()
	if rc := h.run("run", "github-token", "--as", "X", "--item", "github-token", "--as", "X", "--", "c"); rc != 1 || !strings.Contains(h.err.String(), "is used twice") {
		t.Fatalf("the same variable twice was accepted: rc=%d %q", rc, h.err)
	}
}

func TestHwSecretRunRefusals(t *testing.T) {
	cases := []struct {
		name, want string
		args       []string
		lock       bool
	}{
		{"absent", "no-such-item is not in the Bitwarden folder agents", []string{"no-such-item"}, false},
		{"ambiguous", "AMBIGUOUS: two items in the folder agents carry the name dup", []string{"dup"}, false},
		{"empty", "empty-item.password is absent or empty", []string{"empty-item"}, false},
		{"badfield", "github-token.nosuchfield is absent or empty", []string{"github-token.nosuchfield"}, false},
		{"locked", "the vault is LOCKED or its session expired", []string{"github-token"}, true},
	}
	for _, c := range cases {
		h := newHsT(t)
		if !c.lock {
			h.unlocked()
		}
		args := append([]string{"run"}, c.args...)
		args = append(args, "--as", "X", "--", "c")
		if rc := h.run(args...); rc != 1 {
			t.Fatalf("%s: rc=%d, not 1", c.name, rc)
		}
		if len(h.execs) != 0 {
			t.Fatalf("%s ran the child", c.name)
		}
		if !strings.Contains(h.err.String(), c.want) {
			t.Fatalf("%s: stderr %q does not carry %q", c.name, h.err, c.want)
		}
	}
}

func TestHwSecretRunMalformed(t *testing.T) {
	for _, args := range [][]string{
		{"run", "github-token", "--", "c"},
		{"run", "--as", "X", "--", "c"},
		{"run", "github-token", "--as", "X"},
		{"run"},
		{"run", "--item"},
		{"run", "github-token", "--as"},
		{"run", "github-token", "--as", "X", "--bogus", "--", "c"},
		{"run", "github-token", "--as", "X", "second", "--as", "Y", "--", "c"},
		{"run", "--item", "a", "--item", "b", "--as", "X", "--", "c"},
		{"run", "-x"},
		{"run", "--item", "a\nb", "--as", "X", "--", "c"},
		{"run", "--item", "", "--as", "X", "--", "c"},
	} {
		h := newHsT(t)
		h.unlocked()
		if rc := h.run(args...); rc != 1 || len(h.execs) != 0 {
			t.Errorf("%q: rc=%d execs=%v", args, rc, h.execs)
		}
	}
}

// ── the folder, ambiguity, the session ──────────────────────────────────────

func TestHwSecretFolderOnly(t *testing.T) {
	h := newHsT(t)
	h.unlocked()
	if rc := h.run("run", "personal-bank", "--as", "B", "--", "c"); rc != 1 || len(h.execs) != 0 {
		t.Fatalf("an item OUTSIDE the folder agents resolved (rc=%d, execs=%v)", rc, h.execs)
	}
	if !strings.Contains(h.err.String(), "personal-bank is not in the Bitwarden folder agents") {
		t.Fatalf("the refusal does not name the item: %q", h.err)
	}
	if rc := h.run("check", "personal-bank"); rc != 1 || !strings.Contains(h.out.String(), "ABSENT") {
		t.Fatalf("check let an item outside the folder through: rc=%d %q", rc, h.out)
	}
}

func TestHwSecretAmbiguous(t *testing.T) {
	h := newHsT(t)
	h.unlocked()
	if rc := h.run("check", "dup"); rc != 1 || !strings.Contains(h.out.String(), "AMBIGUOUS") {
		t.Fatalf("check of two items with one name: rc=%d %q", rc, h.out)
	}
	if rc := h.run("run", "dup", "--as", "X", "--", "c"); rc != 1 || !strings.Contains(h.err.String(), "AMBIGUOUS") {
		t.Fatalf("run of two items with one name: rc=%d %q", rc, h.err)
	}
}

func TestHwSecretSessionExpiry(t *testing.T) {
	h := newHsT(t)
	os.Remove(filepath.Join(h.state, "locked"))
	h.session(-5, "TOK-STUB-123")
	if rc := h.run("status"); rc != 1 || h.out.String() != "locked\n" {
		t.Fatalf("an EXPIRED session was honoured: rc=%d %q", rc, h.out)
	}
	if _, err := os.Stat(h.sessionPath()); err == nil {
		t.Fatalf("an expired session file was kept")
	}
	if !strings.Contains(h.bwCalls(), "\nlock\n") && !strings.HasPrefix(h.bwCalls(), "lock\n") {
		t.Fatalf("whoever finds the file expired did not run bw lock: %q", h.bwCalls())
	}
}

func TestHwSecretSessionHonoured(t *testing.T) {
	h := newHsT(t)
	h.session(43200, "TOK-STUB-123") // the vault is locked behind the file's back
	if rc := h.run("status"); rc != 1 || h.out.String() != "locked\n" {
		t.Fatalf("a token the vault no longer honours was kept: rc=%d %q", rc, h.out)
	}
	if _, err := os.Stat(h.sessionPath()); err == nil {
		t.Fatalf("the dead session file was kept")
	}
	h.unlocked()
	if rc := h.run("status"); rc != 0 || !strings.HasPrefix(h.out.String(), "unlocked until ") {
		t.Fatalf("a live session was not honoured: rc=%d %q", rc, h.out)
	}
}

func TestHwSecretSessionFileShapes(t *testing.T) {
	for name, body := range map[string]string{
		"nondigits": "expires=soon\nTOK-STUB-123\n",
		"notoken":   "expires=" + strconv.FormatInt(time.Now().Unix()+600, 10) + "\n",
		"empty":     "",
	} {
		h := newHsT(t)
		os.Remove(filepath.Join(h.state, "locked"))
		os.MkdirAll(filepath.Dir(h.sessionPath()), 0o700)
		os.WriteFile(h.sessionPath(), []byte(body), 0o600)
		if rc := h.run("status"); rc != 1 || h.out.String() != "locked\n" {
			t.Errorf("%s: rc=%d %q", name, rc, h.out)
		}
		if _, err := os.Stat(h.sessionPath()); err == nil {
			t.Errorf("%s: the malformed session file was kept", name)
		}
	}
	// the expiry may come without its `expires=` prefix
	h := newHsT(t)
	os.Remove(filepath.Join(h.state, "locked"))
	os.MkdirAll(filepath.Dir(h.sessionPath()), 0o700)
	os.WriteFile(h.sessionPath(), []byte(strconv.FormatInt(time.Now().Unix()+7200, 10)+"\nTOK-STUB-123\n"), 0o600)
	h.mustRC(0, "status")
}

func TestHwSecretSessionMode(t *testing.T) {
	h := newHsT(t)
	h.mustRC(0, "unlock")
	st, err := os.Stat(h.sessionPath())
	if err != nil {
		t.Fatalf("unlock wrote no session file: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("the session file is mode %o, not 600", st.Mode().Perm())
	}
	if d, _ := os.Stat(filepath.Dir(h.sessionPath())); d.Mode().Perm() != 0o700 {
		t.Fatalf("the session dir is mode %o, not 700", d.Mode().Perm())
	}
	if strings.Contains(h.out.String(), "TOK-STUB") {
		t.Fatalf("unlock PRINTED the token: %q", h.out)
	}
	b, _ := os.ReadFile(h.sessionPath())
	ls := strings.Split(string(b), "\n")
	exp, _ := strconv.ParseInt(strings.TrimPrefix(ls[0], "expires="), 10, 64)
	if d := exp - time.Now().Unix(); d < 12*3600-60 || d > 12*3600+60 || ls[1] != "TOK-STUB-123" {
		t.Fatalf("the session is not 12 h of the token: %q", b)
	}
}

func TestHwSecretUnlockFlags(t *testing.T) {
	for _, bad := range []string{"0", "abc", "999", "169", "-3", ""} {
		h := newHsT(t)
		if rc := h.run("unlock", "--hours", bad); rc != 1 || !strings.Contains(h.err.String(), "--hours takes a whole number of hours, 1 to 168 (got: "+bad+")") {
			t.Errorf("--hours %q: rc=%d %q", bad, rc, h.err)
		}
		if _, err := os.Stat(h.sessionPath()); err == nil {
			t.Errorf("--hours %q wrote a session", bad)
		}
	}
	h := newHsT(t)
	h.mustRC(0, "unlock", "--hours", "1")
	b, _ := os.ReadFile(h.sessionPath())
	exp, _ := strconv.ParseInt(strings.TrimPrefix(strings.SplitN(string(b), "\n", 2)[0], "expires="), 10, 64)
	if d := exp - time.Now().Unix(); d < 3600-60 || d > 3600+60 {
		t.Fatalf("--hours 1 did not set a one-hour expiry (%d s)", d)
	}
	h = newHsT(t)
	os.Create(filepath.Join(h.state, "unauth"))
	if rc := h.run("unlock"); rc != 1 || !strings.Contains(h.err.String(), "bw login") {
		t.Fatalf("an unauthenticated bw: rc=%d %q", rc, h.err)
	}
	h = newHsT(t)
	h.unlocked()
	h.mustRC(0, "unlock", "--lock")
	if _, err := os.Stat(h.sessionPath()); err == nil || !strings.Contains(h.bwCalls(), "lock\n") || h.out.String() != "locked: vault locked, session file removed\n" {
		t.Fatalf("--lock: %q %q", h.out, h.bwCalls())
	}
	h = newHsT(t)
	h.mustRC(0, "unlock", "--hours", "3", "--help")
	if !strings.HasPrefix(h.out.String(), "usage: secret-unlock [--hours N]") {
		t.Fatalf("--help: %q", h.out)
	}
	if rc := h.run("unlock", "--bogus"); rc != 1 || h.err.String() != "hw secret: unknown option: --bogus (secret-unlock --help)\n" {
		t.Fatalf("an unknown option: rc=%d %q", rc, h.err)
	}
}

// ── check, the verbs ────────────────────────────────────────────────────────

func TestHwSecretCheck(t *testing.T) {
	h := newHsT(t)
	h.unlocked()
	h.mustRC(0, "check", "github-token", "db.prod", "github-token.api_key", "github-token.totp")
	if strings.Count(h.out.String(), " — present in agents\n") != 4 || strings.Contains(h.out.String(), "S3CRET") || !strings.HasSuffix(h.out.String(), "  only presence was checked — no value was read, printed or logged.\n") {
		t.Fatalf("check of present items: %q", h.out)
	}
	if rc := h.run("check", "github-token", "no-such-item"); rc != 1 || !strings.Contains(h.out.String(), "  !!  no-such-item — ABSENT from the Bitwarden folder agents\n") || strings.Contains(h.out.String(), "only presence") {
		t.Fatalf("check of an absent item: rc=%d %q", rc, h.out)
	}
	if rc := h.run("check", "github-token.nosuchfield"); rc != 1 || !strings.Contains(h.out.String(), `its field "nosuchfield" is absent or empty`) {
		t.Fatalf("check of a missing field: rc=%d %q", rc, h.out)
	}
	if rc := h.run("check", "empty-item"); rc != 1 {
		t.Fatalf("check of an empty field: rc=%d", rc)
	}
	if rc := h.run("check"); rc != 1 || !strings.Contains(h.err.String(), "usage: hw secret check") {
		t.Fatalf("check with no item: rc=%d %q", rc, h.err)
	}
	if rc := h.run("check", "github-token", "-x"); rc != 1 || h.out.String() != "  ok  github-token — present in agents\n" || !strings.Contains(h.err.String(), "not an item name: -x") {
		t.Fatalf("a bad name after a good one: rc=%d out=%q err=%q", rc, h.out, h.err)
	}
	h = newHsT(t)
	if rc := h.run("check", "github-token"); rc != 2 || !strings.Contains(h.err.String(), "the vault is LOCKED, so nothing can be checked") {
		t.Fatalf("a locked vault is neither present nor absent: rc=%d %q", rc, h.err)
	}
}

func TestHwSecretHelpAndVerbs(t *testing.T) {
	h := newHsT(t)
	usage := "usage: hw secret run <item>[.<field>] --as VAR [--item … --as …] -- <cmd…>\n       <cmd that prints a value> | hw secret put <item>[.<field>] [--replace]\n       hw secret check <item>[.<field>]…\n       hw secret status | ensure\n       secret-unlock [--hours N | --lock]   (a person, in a terminal, once a day)\n"
	for _, a := range [][]string{{}, {"--help"}, {"-h"}, {"help"}} {
		h.mustRC(0, a...)
		if h.out.String() != usage {
			t.Fatalf("%v: %q", a, h.out)
		}
	}
	if rc := h.run("frobnicate"); rc != 1 || h.err.String() != "hw secret: unknown verb: frobnicate (run, put, check, status, ensure)\n" {
		t.Fatalf("an unknown verb: rc=%d %q", rc, h.err)
	}
	h.w.env = append(h.w.env, "HW_BW_BIN="+filepath.Join(h.home, "no-such-bw"))
	if rc := h.run("status"); rc != 1 || h.err.String() != "hw secret: the bw CLI is not installed (npm i -g @bitwarden/cli)\n" {
		t.Fatalf("no bw: rc=%d %q", rc, h.err)
	}
}

// ── the Keychain flows: --auto, --setup, ensure ─────────────────────────────

func TestHwSecretAutoPasswordOffArgv(t *testing.T) {
	h := newHsT(t)
	os.WriteFile(filepath.Join(h.sec, "item"), []byte("MASTER-STUB-PW"), 0o644)
	h.mustRC(0, "unlock", "--auto")
	if strings.Contains(h.bwCalls(), "MASTER-STUB-PW") {
		t.Fatalf("the password reached bw's ARGV: %s", h.bwCalls())
	}
	if !strings.Contains(h.bwCalls(), "--passwordenv BW_HW_MASTER") {
		t.Fatalf("bw was not given the password through --passwordenv: %s", h.bwCalls())
	}
	if h.out.String() != "unlocked (auto)\n" || strings.Contains(h.out.String()+h.err.String(), "MASTER-STUB-PW") {
		t.Fatalf("--auto printed: %q %q", h.out, h.err)
	}
	st, err := os.Stat(h.sessionPath())
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("--auto did not leave a 0600 session: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(h.home, ".cache", "hw", "bw-unlock-status"))
	if !strings.HasPrefix(string(b), "ok ") {
		t.Fatalf("--auto left no ok status: %q", b)
	}
}

func TestHwSecretAutoFailureStatus(t *testing.T) {
	h := newHsT(t)
	if rc := h.run("unlock", "--auto"); rc != 1 {
		t.Fatalf("--auto with NO item: rc=%d", rc)
	}
	b, _ := os.ReadFile(filepath.Join(h.home, ".cache", "hw", "bw-unlock-status"))
	if !strings.HasPrefix(string(b), "failed ") || !strings.Contains(string(b), "secret-unlock --setup") {
		t.Fatalf("the failure left no readable status naming --setup: %q", b)
	}
	if ents, _ := os.ReadDir(filepath.Join(h.home, ".cache", "hw")); len(ents) != 1 {
		t.Fatalf("a failed --auto left more than the status file: %v", ents)
	}
	if !strings.Contains(h.err.String(), "secret-unlock --auto failed: no hw-bw-master item in the Keychain (secret-unlock --setup, once)") {
		t.Fatalf("stderr: %q", h.err)
	}
	// a stored password the vault no longer accepts, and a bw that is not logged in
	h = newHsT(t)
	os.WriteFile(filepath.Join(h.sec, "item"), []byte("MASTER-STUB-PW"), 0o644)
	h.w.env = append(h.w.env, "BW_STUB_MASTER=CHANGED")
	if rc := h.run("unlock", "--auto"); rc != 1 || !strings.Contains(h.err.String(), "bw rejected the stored master password") {
		t.Fatalf("a changed master password: rc=%d %q", rc, h.err)
	}
	h = newHsT(t)
	os.WriteFile(filepath.Join(h.sec, "item"), []byte("MASTER-STUB-PW"), 0o644)
	os.Create(filepath.Join(h.state, "unauth"))
	if rc := h.run("unlock", "--auto"); rc != 1 || !strings.Contains(h.err.String(), "bw is not logged in (bw login, once)") {
		t.Fatalf("an unauthenticated bw: rc=%d %q", rc, h.err)
	}
}

func (h *hsT) setup(pw, pw2 string) int {
	h.w.in = strings.NewReader(pw + "\n" + pw2 + "\n")
	return h.run("unlock", "--setup")
}

func TestHwSecretSetupPasswordOffArgv(t *testing.T) {
	h := newHsT(t)
	if rc := h.setup("MASTER-STUB-PW", "MASTER-STUB-PW"); rc != 0 {
		t.Fatalf("--setup failed: %q", h.err)
	}
	if strings.Contains(h.secCalls(), "MASTER-STUB-PW") {
		t.Fatalf("the password reached security's ARGV: %s", h.secCalls())
	}
	if pw, ok := h.item(); !ok || pw != "MASTER-STUB-PW" {
		t.Fatalf("--setup stored no item (%q, %v)", pw, ok)
	}
	if st, err := os.Stat(h.sessionPath()); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("--setup did not leave a 0600 session (it proves the password with one unlock)")
	}
	if strings.Contains(h.out.String()+h.err.String(), "MASTER-STUB-PW") || strings.Contains(h.out.String()+h.err.String(), "TOK-STUB") {
		t.Fatalf("--setup PRINTED a secret: %q %q", h.out, h.err)
	}
	if !strings.HasPrefix(h.out.String(), "stored as the Keychain service hw-bw-master and unlocked once;") {
		t.Fatalf("stdout: %q", h.out)
	}
	if want := "Bitwarden master password (not echoed): \nAgain: \n"; h.err.String() != want {
		t.Fatalf("the prompts: %q", h.err)
	}
}

func TestHwSecretSetupTypo(t *testing.T) {
	h := newHsT(t)
	if rc := h.setup("MASTER-STUB-PW", "MASTER-STUB-TYPO"); rc != 1 || !strings.Contains(h.err.String(), "the two entries differ: nothing was stored") {
		t.Fatalf("two different entries: rc=%d %q", rc, h.err)
	}
	if _, ok := h.item(); ok {
		t.Fatalf("two different entries stored an item")
	}
	if _, err := os.Stat(h.sessionPath()); err == nil {
		t.Fatalf("two different entries left a session")
	}
	if rc := h.setup("", ""); rc != 1 || !strings.Contains(h.err.String(), "an empty password was not stored") {
		t.Fatalf("an empty password: rc=%d %q", rc, h.err)
	}
}

func TestHwSecretSetupRejected(t *testing.T) {
	h := newHsT(t)
	if rc := h.setup("WRONG-PW", "WRONG-PW"); rc != 1 {
		t.Fatalf("a password bw rejects: rc=%d", rc)
	}
	if _, ok := h.item(); ok {
		t.Fatalf("a password bw REJECTED stayed in the Keychain")
	}
	if _, err := os.Stat(h.sessionPath()); err == nil {
		t.Fatalf("a rejected password left a session")
	}
	if !strings.Contains(h.err.String(), "bw did not accept that password (or is not logged in): the Keychain item was removed. — bw rejected the stored master password") {
		t.Fatalf("stderr: %q", h.err)
	}
}

func TestHwSecretSetupMetachars(t *testing.T) {
	h := newHsT(t)
	pw := `p"w\x $y#!`
	h.w.env = append(h.w.env, "BW_STUB_MASTER="+pw)
	if rc := h.setup(pw, pw); rc != 0 {
		t.Fatalf("a password with quotes, backslash, dollar and bang did not round-trip through the Keychain: %q", h.err)
	}
	if got, _ := h.item(); got != pw {
		t.Fatalf("the stored password is %q, not %q", got, pw)
	}
}

func TestHwSecretEnsureTriesOnce(t *testing.T) {
	h := newHsT(t)
	os.WriteFile(filepath.Join(h.sec, "item"), []byte("MASTER-STUB-PW"), 0o644)
	h.mustRC(0, "ensure")
	if n := strings.Count(h.bwCalls(), "unlock "); n != 1 {
		t.Fatalf("ensure did not try --auto EXACTLY once: %q", h.bwCalls())
	}
	before := h.bwCalls()
	h.mustRC(0, "ensure")
	if strings.Count(h.bwCalls(), "unlock ") != strings.Count(before, "unlock ") {
		t.Fatalf("ensure unlocked an already-unlocked vault")
	}
	h = newHsT(t)
	if rc := h.run("ensure"); rc != 1 || !strings.Contains(h.err.String(), "A person, once: secret-unlock --setup") || !strings.Contains(h.err.String(), "no hw-bw-master item in the Keychain") {
		t.Fatalf("with no item ensure must fail naming the one-time step: rc=%d %q", rc, h.err)
	}
	h = newHsT(t)
	os.WriteFile(filepath.Join(h.sec, "item"), []byte("MASTER-STUB-PW"), 0o644)
	if rc := h.run("run", "github-token", "--as", "GH", "--", "c"); rc != 1 || strings.Contains(h.bwCalls(), "unlock ") {
		t.Fatalf("hw secret run unlocked the vault on its own (rc=%d, calls: %s)", rc, h.bwCalls())
	}
}

// ── the jq the bash had, read in Go ─────────────────────────────────────────

func TestHwSecretJSON(t *testing.T) {
	if hsStatus(`{"status":"unlocked"}`) != "unlocked" || hsStatus(`{"status":null}`) != "" || hsStatus(`[]`) != "" || hsStatus("not json") != "" || hsStatus(`{"status":false}`) != "" {
		t.Fatal("hsStatus")
	}
	folders := `[{"id":"f1","name":"personal"},{"id":"f2","name":"agents"},{"id":"f3","name":"agents"}]`
	if hsFirstID(folders, "agents") != "f2" || hsFirstID(folders, "none") != "" || hsFirstID(`["x"]`, "agents") != "" || hsFirstID(`[null,{"id":"z","name":"agents"}]`, "agents") != "z" {
		t.Fatal("hsFirstID")
	}
	if hsIDs(`[{"id":"a","name":"x"},{"id":"b","name":"y"}]`, "x") != "a" || hsIDs(`[{"id":"a","name":"x"},{"id":"b","name":"x"}]`, "x") != "a\nb" || hsIDs(`[]`, "x") != "" || hsIDs(`[{"id":"a","name":"x"},"oops"]`, "x") != "a" {
		t.Fatal("hsIDs")
	}
	item := `{"id":"i","login":{"username":"u","password":"p","uris":[{"uri":"https://h"}]},"notes":"n\n\n","fields":[{"name":"k","value":"v"},{"name":"z","value":false},{"name":"n","value":7}]}`
	for field, want := range map[string]string{"password": "p", "username": "u", "uri": "https://h", "notes": "n\n\n", "k": "v", "z": "", "n": "7", "absent": ""} {
		if got, ok := hsFieldRaw(item, field); !ok || got != want {
			t.Errorf("hsFieldRaw %s = %q,%v want %q", field, got, ok, want)
		}
	}
	if _, ok := hsFieldRaw(`{"login":"oops"}`, "password"); ok {
		t.Error("a type error is an error")
	}
	if hsFieldPresent(item, "z") || !hsFieldPresent(item, "n") || !hsFieldPresent(item, "notes") || hsFieldPresent(item, "absent") || hsFieldPresent(`{"login":{"password":""}}`, "password") || hsFieldPresent("x", "password") {
		t.Error("hsFieldPresent")
	}
	if got, _ := hsFieldRaw(`{"fields":[{"name":"o","value":{"a":1,"b":[]}}]}`, "o"); got != "{\n  \"a\": 1,\n  \"b\": []\n}" {
		t.Errorf("an object value prints as jq -r does: %q", got)
	}
}

func TestHwSecretLookPathAndExec(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tool"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(dir, "plain"), []byte("x"), 0o644)
	env := []string{"PATH=" + dir}
	if hsLookPath("tool", env) != dir+"/tool" || hsLookPath("plain", env) != "" || hsLookPath("nothing", env) != "" {
		t.Fatal("hsLookPath")
	}
	if rc, why := hsExecProcess([]string{"nothing"}, env); rc != 127 || why != "not found" {
		t.Fatalf("a command not found: %d %q", rc, why)
	}
	if rc, why := hsExecProcess([]string{dir + "/plain"}, env); rc != 126 || !strings.Contains(why, "Permission denied") {
		t.Fatalf("a file that cannot run: %d %q", rc, why)
	}
	if rc, _ := hsExecProcess([]string{dir}, env); rc != 126 {
		t.Fatalf("a directory: %d", rc)
	}
}
